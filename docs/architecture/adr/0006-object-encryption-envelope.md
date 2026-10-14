# ADR 0006: Object encryption envelope

**Status:** Accepted  
**Date:** 2026-10-13  
**Related:** E01 · TZ §4.1, §4.7, §4.10 · `internal/storage/sse.go` (1.5.0) · migration `025_object_encryption`

## Context

1.5.0 SSE-S3 is process-wide. `STORAGE_SSE_MASTER_KEY` derives one AES-256 key with `SHA-256(master string)`. `internal/storage/sse.go` seals the **entire object** in one AES-GCM operation. The file on disk is:

```
nonce (12 bytes) || ciphertext || GCM tag (16 bytes)
```

There is no magic, no version byte, and no per-object field. `ObjectRecord` does not say whether the bytes are encrypted. `BucketRecord.DefaultEncryption` is stored in Bolt JSON and is **not** a Postgres column, and `putObject` does not read it. If the process has a cipher, every successful write is ciphertext and every read is decrypted. If it does not, every object is plaintext.

That layout is **format 0**. It cannot be detected by sniffing: plaintext can start with any bytes, including a future magic. A data directory that was written both with and without the master key cannot be split after the fact.

1.6.0 must read those objects with no operator step (`key_provider=local`, `key_version=0`) and must write a versioned envelope (format 1) that can hold a per-object DEK.

## Decision

### Marking objects written by 1.5.0

1.6.0 does **not** sniff payload bytes.
2. On the first start, the node records a KEK fingerprint (SHA-256 of the derived AES key, hex). The fingerprint is not the key. A later start with a different `STORAGE_SSE_MASTER_KEY`, or with the variable cleared after a fingerprint was stored, refuses to start. The error text contains `SSE master key fingerprint mismatch`.
3. If that first start already has a fingerprint **and** at least one object, `kms_state.legacy_sse_objects` is set. Every object whose encryption metadata is absent is then **format 0**: `key_provider=local`, `key_version=0`, `enc_format_version=0`, empty `wrapped_dek` (the KEK is the data key). Objects written after that are marked explicitly, including plaintext (`plaintext: true`), so they are not decrypted as format 0.
4. If the first start has no master key, `legacy_sse_objects` stays false. Adding the key later records the fingerprint and does **not** flip the legacy flag: those older objects stay plaintext.
5. Operators who mixed encrypted and plaintext objects in one data directory under 1.5.0 cannot be repaired automatically. 1.5.0 stored no flag. The upgrade assumes the deployment invariant: the master key was either always set or always unset for that data directory.

### Format 1 (what 1.6.0 writes)

```
offset  size  field
0       4     magic "DSSE"
4       1     version = 1
5       1     flags = 0
6       2     reserved = 0
8       4     chunk size, uint32 big-endian (plaintext bytes, default 65536)
12      4     reserved = 0

then, until the plaintext is exhausted, each chunk:
  uint32 big-endian length of (ciphertext || tag)
  nonce (12 bytes)
  AES-256-GCM ciphertext and tag
```

Additional authenticated data for every chunk is the 16-byte header. A chunk cannot be moved under a different header. The chunk framing is self-delimiting, so a multipart upload can use **one DEK** and still concatenate parts: part 1 carries the header, later parts are chunk frames only, and the completed object is one format-1 body. 1.6.0 reads format 0 and format 1. It writes format 1 for new encrypted objects.

The DEK is 32 random bytes. It is stored only wrapped: AES-256-GCM under the local KEK, nonce prepended. Metadata fields on the object and on each version:

| Field | Meaning |
| --- | --- |
| `key_provider` | `local` or, later, `vault-transit` |
| `key_id` | `datasafe-<tenant>-<bucket>` unless the caller passed an allow-listed name |
| `key_version` | 0 for format 0; 1+ for wrapped DEKs |
| `wrapped_dek` | base64, empty for format 0 |
| `alg` | `AES-256-GCM` |
| `enc_format_version` | 0 or 1 |
| `sse_algorithm` | `AES256` or `aws:kms` |
| `plain_size` | plaintext length returned as `Content-Length` |

`ObjectRecord.Size` for format 1 is the plaintext size. Format 0 rows keep the 1.5.0 stored size (ciphertext). Readers report plaintext length (`size - 28`) for those rows. The clear DEK is never written to disk, logs, Activity, API responses, or the outbox.

Local KEK versions: version 0 is `SHA-256(master string)`, the same bytes 1.5.0 used, and it only decrypts format 0. Version 1+ is HKDF-SHA256 of that key (`salt=datasafe-sse-kek`, `info=vN`) and only wraps DEKs. Rotation (E03) bumps the version. The same master still unwraps older versions.

### When a new object is encrypted

Encryption is no longer “the process has a key, so encrypt everything”. A new object is encrypted when the request sends `x-amz-server-side-encryption` or the bucket default says so. `AES256` is the local server key. `aws:kms` is the same local KEK in E01 (Vault arrives in E02) and is the algorithm that blocks anonymous access. With no header and no bucket default, the object stays plaintext even if `STORAGE_SSE_MASTER_KEY` is set. That key is still required to read format 0 and to write any new SSE object.

`public-read` together with bucket default `aws:kms` is rejected with HTTP 409. An anonymous GET or HEAD of an object whose `sse_algorithm` is `aws:kms` is HTTP 403 `объект зашифрован ключом KMS, публичный доступ невозможен`. SSE-S3 (`AES256`) stays anonymously readable when Block Public Access is off, as in 1.5.0.

A share link or presigned URL for an `aws:kms` object is decrypted only after the server checks again that the link creator or the signing key’s owner still exists, is active, and can read the object. Destroyed keys return HTTP 403 `Ключ шифрования уничтожен, файл недоступен навсегда`.

### Cluster

The fingerprint lives in shared metadata (Postgres in a cluster, Bolt on one node). Every node must present the same master key or it will not start.

1.6.0 nodes write a heartbeat (`cluster_node_heartbeats`) with their version. If any fresh heartbeat is numerically below 1.6.0, SSE-KMS and bucket-encryption changes return HTTP 409 `cluster upgrade in progress`, and new SSE-S3 objects are written as format 0 so a 1.5.0 reader with the same master key can open them. 1.5.0 itself does not write this heartbeat, so a rolling upgrade is safe only after every node is 1.6.0. An all-1.6.0 cluster has no row below 1.6.0 and uses format 1.

Turning bucket encryption off is the dangerous action `encryption_disable` and needs a step-up token on the admin API. S3 `DeleteBucketEncryption` keeps the E00 access-key rule (`created_with_mfa`).

## Consequences

- Format 0 objects stay readable without a rewrite.
- New encrypted objects are larger than the plaintext by the header and per-chunk overhead. `Content-Length` is the plaintext size.
- Vault Transit, DEK cache invalidation, rewrap, and crypto-shred jobs are E02 and E03. The `kms.Provider` methods exist; E01 implements `local`.
- Postgres migration `025_object_encryption` is additive (`bucket_encryption`, `kms_keys`, `kms_state`, heartbeat rows, `objects.encryption`, `multipart_uploads.encryption`).
