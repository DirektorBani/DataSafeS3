English | **[Русский](../ru/encryption.md)**

# Object encryption

This is encryption of object bytes. It is not [field encryption](field-encryption.md) of metadata secrets, and it is not [Vault env injection](secrets-vault.md).

Three states:

| State | Header / bucket default | Key |
|-------|-------------------------|-----|
| Plaintext | none | none. The bytes on disk are the bytes the client sent |
| SSE-S3 | `AES256` | `STORAGE_SSE_MASTER_KEY` on every node. Vault is not called |
| SSE-KMS | `aws:kms` and an optional key id | Vault Transit when `STORAGE_KMS_PROVIDER=vault-transit` and an address are set. Until then, the same server key wraps the data key and the stored provider stays `local` |

A bucket with no default, and a PUT with no encryption header, stays plaintext. SSE-S3 from 1.5.0 is classified once and then read as plaintext. See [upgrade.md](upgrade.md#upgrading-to-v160).

Turning encryption on is **Settings →** the bucket **Settings** tab, or S3 `PutBucketEncryption`. On Postgres or HA the console asks you to confirm that every node is already 1.6.0. A live 1.5.0 peer makes that change HTTP 409 `cluster upgrade in progress`. Finish the rolling upgrade first.

**Settings → Vault (KMS)** is the opt-in Transit client: address, AppRole or token, test, and per-node status. Policies, cache TTL, and what crypto-shred does not erase are in [kms-vault.md](kms-vault.md).

**Encrypt existing objects**, **Rotate**, and **Destroy key** are on the same bucket card. Destroy asks for the bucket name. A later read of any object in that bucket, a share link, and a presigned GET are HTTP 403 `Ключ шифрования уничтожен, файл недоступен навсегда`.

Grafana row **KMS** (availability by node, latency, DEK cache, jobs) is in [monitoring.md](monitoring.md#kms). Those Vault series are absent until Vault is enabled.

## Known limit: local GET does not check the file

When the object file is already on this node's disk, S3 `GET` returns those bytes. It does not compare them with the stored ETag or size. A corrupted local copy is HTTP 200. A copy accepted from a peer is checked against the metadata ETag before it is stored. This local read path is not. Tracked, not fixed in 1.6.0.

<a id="aistor-free"></a>

## Comparison with MinIO AIStor Free

MinIO OSS under AGPLv3 is frozen: the vendor says that codebase is no longer maintained ([pricing](https://www.min.io/pricing)). AIStor is the current product and requires a license ([licenses](https://docs.min.io/aistor/operations/licenses/)).

The license page, not the pricing summary, is the feature list. It says AIStor Free is a single-node license and that these are Enterprise Lite / Enterprise only, refused on Free with `XMinioPaidTierLicenseRequired`:

- distributed (multi-node) deployments
- replication (site, bucket, or batch)
- encryption at rest, including a write that requests server-side encryption, `PutBucketEncryption`, creating a KMS key, and a key-rotate batch job

The pricing page describes Free as a single-node deployment at no charge and Enterprise as the multi-node line. Where that page says "full feature set" and the license page lists encryption and replication as paid, the license page is the one that names the refusal.

Open-source KES is deprecated. The maintained KES fork and MinIO KMS are enterprise-licensed ([legacy key management](https://docs.min.io/kms/legacy-key-management/)).

DataSafeS3 1.6.0 is not that product. SSE-S3 and SSE-KMS run without an AIStor license. Vault Transit is opt-in. A single node with no Vault and no `STORAGE_KMS_*` does not open a KMS connection. HA is the existing Postgres leader lease, not AIStor site replication. This is not a claim of API parity with MinIO or AIStor. The copy path is [migrate-from-minio.md](migrate-from-minio.md).
