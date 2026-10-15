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

That layout is **format 0**. It cannot be detected by sniffing: plaintext can start with any bytes, including a future magic.

1.5.0 does not persist those blobs. `PutObject` passes the caller’s plaintext `Content-Length` into the backend, and the backend rejects the write when the sealed body is longer (`internal/storage/fs.go` size check, and the same check in the erasure backend). A 1.5.0 data directory written with `STORAGE_SSE_MASTER_KEY` set has no successful SSE objects. A directory written without the key is plaintext and has no per-object flag.

1.6.0 writes a versioned envelope (format 1) for new encrypted objects. It reads a format-0 blob that authenticates with the server key and stores that result on the object. `STORAGE_SSE_LEGACY_FORMAT0` does not force that reader onto every unmarked object.

## Decision

### Marking objects written by 1.5.0

1. When the server key is set, an unmarked body is classified once. The only header recognized without that classification is the format-1 magic (`DSSE`, version 1). After a marker is stored, reads use the marker and do not try again.
2. On the first start, the node records a KEK fingerprint (SHA-256 of the derived AES key, hex). The fingerprint is not the key. A later start with a different `STORAGE_SSE_MASTER_KEY` refuses to start. The error text contains `SSE master key fingerprint mismatch`. A start with the variable cleared after a fingerprint was stored also refuses to start. That error says the key is missing and to set `STORAGE_SSE_MASTER_KEY` to the original key.
3. `kms_state.legacy_sse_objects` is **not** derived from the object count. 1.5.0 did not store an encryption bit on the object or the bucket, and format 0 has no magic. A format-1 header (`DSSE` and version 1) without a wrapped DEK is an error, not a format-0 decrypt. Every other unmarked object is classified once, at startup in the background and again on the first read if the marker is still absent. Classification tries authenticated decryption with the server key and streams the body. A recorded size below 28 bytes cannot be format 0 and is marked plaintext without reading it into memory. A blob that authenticates is marked format 0 (`sse_algorithm=AES256`, `plain_size` set). Any other result is marked `plaintext: true`. The write is conditional (`encryption IS NULL` on Postgres, the same check in one Bolt transaction), so a restart and a second HA node are idempotent and do not replace a marker. Reads after that use the marker. Plaintext is streamed from the file. Format 0 is decrypted, and a later change to those bytes is HTTP 500 `cipher: message authentication failed`. A body that fails authentication before a marker exists is not treated as ciphertext: 1.5.0 left no record that it was encrypted, and a failed tag is not that record. A 1.5.0 SSE object changed before that first successful check is therefore stored as plaintext and served as the bytes on disk. Fail closed only for an object the marker, or a format-1 record, says is encrypted. The first read of a large unmarked object authenticates the whole file before the response (about 11s per 2GB). The background walk does the same work. Progress is logged and counted in `datasafe_kms_legacy_classify_objects_total`; a read of 32MB or more sets `datasafe_kms_legacy_classify_current_bytes`. The series appear on the first classified object, not at process start.
4. `STORAGE_SSE_LEGACY_FORMAT0=true` is recorded on `kms_state` and does not force the format-0 reader. Classification is the same with the variable set or unset. A flag stored by an earlier 1.6.0 build that turned it on because objects already existed is cleared on the next start unless the variable is set. The fingerprint is kept, so starting with a different key, or with the key cleared, still refuses to start.
5. Objects written after the first 1.6.0 start are marked explicitly, including plaintext (`plaintext: true`). A format-0 body that 1.6.0 itself writes during a mixed-cluster upgrade carries that metadata and does not need the legacy flag.

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
| `key_provider` | `local` or `vault-transit` |
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

Encryption is no longer “the process has a key, so encrypt everything”. A new object is encrypted when the request sends `x-amz-server-side-encryption` or the bucket default says so. `AES256` is the local server key and does not call Vault. `aws:kms` uses Vault Transit when `STORAGE_KMS_PROVIDER=vault-transit` and an address are set. Without that, `aws:kms` still wraps with the local KEK and the stored provider stays `local`. `aws:kms` is the algorithm that blocks anonymous access. With no header and no bucket default, the object stays plaintext even if `STORAGE_SSE_MASTER_KEY` is set. That key is still required to read format 0 and to write any new SSE-S3 object.

`public-read` together with bucket default `aws:kms` is rejected with HTTP 409. An anonymous GET or HEAD of an object whose `sse_algorithm` is `aws:kms` is HTTP 403 `объект зашифрован ключом KMS, публичный доступ невозможен`. SSE-S3 (`AES256`) stays anonymously readable when Block Public Access is off, as in 1.5.0.

A share link or presigned URL for any sealed object (`AES256` and `aws:kms`) is decrypted only after the server checks again that the link creator or the signing key’s owner still exists, is active, and can read the object. A plaintext object is not checked again; that link works until it expires or reaches its download limit. Destroyed keys return HTTP 403 `Ключ шифрования уничтожен, файл недоступен навсегда`. An SSE-KMS share whose data key is not cached while Vault is down returns HTTP 503 `Файл временно недоступен, попробуйте позже`.

### Cluster

The fingerprint lives in shared metadata (Postgres in a cluster, Bolt on one node). Every node must present the same master key or it will not start.

A node writes a heartbeat (`cluster_node_heartbeats`) when `STORAGE_SSE_MASTER_KEY` is set, or when it uses Postgres or the HA leader lease. With no key on Bolt it does not create `kms_state` and does not start that goroutine. The advertised version is the linked build, and at least 1.6.0: an empty, `dev`, or pre-1.6 stamp is reported as 1.6.0, including `GET /healthz`. The image build still reads `VERSION` (1.5.0) when the Docker argument is `dev`; that stamp is this binary, not a 1.5.0 process, and must not block its own bucket-encryption change.

1.5.0 does not write this heartbeat. HTTP 409 `cluster upgrade in progress` is returned while a 1.6.0 node sees a live 1.5.0 peer. The HA leader renews `ha_leader_lock` (`lock_id=storage-server`, `holder_id`, `acquired_at`, `expires_at`). An unexpired lease whose holder has no 1.6.0 heartbeat is that leader. A follower retries the same `INSERT`. The `ON CONFLICT` `WHERE` does not match while the lease is held, so the row stays with the leader. Migration `030` records `NEW.holder_id` in `ha_leader_attempts` from a `BEFORE INSERT` trigger, which runs even when that update is skipped. A recent attempt with no 1.6.0 heartbeat is an old peer. An unknown version is treated as older than 1.6.0. `cluster.nodes` is not required. Nodes in that registry still answer `GET /healthz` with `"version":"1.5.0"` while they are healthy, and that answer blocks. An unreachable registry entry does not. A 1.5.0 leader runs 1.5.0 code and accepts `PutBucketEncryption` itself when the call reaches that process, so the leader must not be left last. On Postgres or HA the console requires the checkbox “All nodes are upgraded to 1.6.0” before the admin PUT. S3 `PutBucketEncryption` has no checkbox and uses the same 409 rules. While the gate is on, new SSE-S3 objects are format 0 so a 1.5.0 reader with the same master key can open them. An all-1.6.0 cluster has heartbeats at 1.6.0 or newer for every recent attempt, so it uses format 1.

Turning bucket encryption off is the dangerous action `encryption_disable` and needs a step-up token on the admin API. S3 `DeleteBucketEncryption` keeps the E00 access-key rule (`created_with_mfa`).

## Consequences

- A format-0 blob stays readable without a rewrite when the server key can open it, including when `STORAGE_SSE_LEGACY_FORMAT0` is unset, and when the object row already carries format-0 metadata. Ciphertext this key can open is not returned as the body. Classification stores one marker per object. Zip, gzip, jpeg, and other unmarked bytes 1.5.0 stored without the key stay readable on Bolt and Postgres, with or without the flag, and those reads stream. An object marked format 0 fails closed after a later change. A format-1 object is authenticated before HTTP 200. A changed byte is HTTP 500 `cipher: message authentication failed` and the connection is closed. The flag does not force every unmarked object through the format-0 reader.
- New encrypted objects are larger than the plaintext by the header and per-chunk overhead. `Content-Length` is the plaintext size.
- Vault Transit is opt-in. The client is `net/http` and is created only when the provider is `vault-transit` and an address is set. AES256 stays on the local KEK. The DEK cache (LRU, TTL, generation poll) and fail-closed SSE-KMS (HTTP 503 `KMS unavailable` on a cache miss while Vault is down) ship with that client. Rewrap and crypto-shred are leader-only jobs. The cursor and counters live in `kms_jobs` (Postgres migration `029`). A repeated rewrap of an object already at the target key version is a no-op. Crypto-shred marks the key `destroyed` and a later read of any object in the bucket is HTTP 403. Destroy is cluster admin, tenant admin, or an explicit bucket-admin grant. Encrypt-existing streams plaintext. A failed Transit delete is stored on the job. `holder_id` is the node id.
- Postgres migration `025_object_encryption` is additive (`bucket_encryption`, `kms_keys`, `kms_state`, heartbeat rows, `objects.encryption`, `multipart_uploads.encryption`). Migration `026_kms_node_status` adds Vault status columns on `cluster_node_heartbeats`. Migration `030_ha_leader_attempts` records every `ha_leader_lock` insert so a 1.5.0 follower is visible without `cluster.nodes`.
