English | **[Русский](../ru/kms-vault.md)**

# Vault Transit (object keys)

This is optional object encryption. It is not [env injection](secrets-vault.md). Leave it unset and the process does not open a Vault connection, start a poller, or register KMS metrics.

SSE-S3 (`AES256`) always uses `STORAGE_SSE_MASTER_KEY`. It does not call Vault. A bucket with no default, and a request with no encryption header, stays plaintext.

`aws:kms` uses Vault only when the provider is `vault-transit` and an address is set (environment or **Settings → Vault (KMS)**). Until then, `aws:kms` still wraps with the local server key and the stored provider stays `local`.

## When the client starts

Both of these must be set. Either source is enough for each field. An environment value wins and the console shows **Set by an environment variable**.

| Variable | Default | Role |
|----------|---------|------|
| `STORAGE_KMS_PROVIDER` | empty | `vault-transit` turns the client on. Anything else, including empty, does not. |
| `STORAGE_KMS_VAULT_ADDR` | empty | Vault address. A console-saved address counts when this is empty. |
| `STORAGE_KMS_VAULT_NAMESPACE` | empty | Enterprise namespace. |
| `STORAGE_KMS_VAULT_TRANSIT_MOUNT` | `transit` | Transit mount path. |
| `STORAGE_KMS_VAULT_CACERT` | empty | PEM text, or a path to a PEM file on that node. |
| `STORAGE_KMS_VAULT_ROLE_ID` | empty | AppRole role id for this node. |
| `STORAGE_KMS_VAULT_SECRET_ID` or `_SECRET_ID_FILE` | empty | AppRole secret. A per-node value overrides the secret saved in shared metadata. |
| `STORAGE_KMS_VAULT_TOKEN_FILE` | empty | Token file. Alternative to AppRole. The console can store a token; the environment form is a file. |
| `STORAGE_KMS_VAULT_ADMIN_ROLE_ID` / `_SECRET_ID` / `_SECRET_ID_FILE` | empty | Separate AppRole for rotate and delete. |
| `STORAGE_KMS_DEFAULT_KEY` | `datasafe-<tenant>-<bucket>` | Default key name. |
| `STORAGE_KMS_KEY_ALLOWLIST` | `datasafe-*` | Names a caller may request. |
| `STORAGE_KMS_DEK_CACHE_TTL` | `5m` | How long an unwrapped data key stays in memory. |
| `STORAGE_KMS_POLL_INTERVAL` | `2s` | How often the node reads `kms_keys.generation` and drops stale cache entries. |
| `STORAGE_KMS_TIMEOUT` | `5s` | One Vault call. |

The client is `net/http`. There is no Vault SDK in the server. A node dials only after the provider and an address are both present. Shared settings live in metadata (`system_config.kms`). The secret id and token are field-encrypted. `GET /api/v1/kms/settings`, `/healthz`, Activity, and logs do not return those secrets. A saved secret is shown as set, and the field accepts a replacement only.

Each node can keep its own AppRole in the environment. That secret is not written back to the shared config.

Lab overlay (dev mode, root token, not for production): `deploy/compose/docker-compose.vault-kms.yml`. Policies: `deploy/vault/policies/datasafe-kms.hcl` (encrypt, decrypt, rewrap, create/read on `datasafe-*`) and `datasafe-kms-admin.hcl` (rotate, delete, and the deletion flag).

## When Vault is down

A new SSE-KMS object is wrapped before anything is written. If Vault is down and the data key is not in the cache, the write is HTTP 503 `KMS unavailable` and nothing is stored. A read of an SSE-KMS object misses the same way. A cache hit inside the TTL still decrypts. Plaintext buckets and SSE-S3 do not call Vault.

A share link for an SSE-KMS object returns HTTP 503 with `Файл временно недоступен, попробуйте позже`. A plaintext share link is unchanged.

`/healthz` adds `kms.provider` and `kms.available`. It does not probe Vault on that request. The gauge `datasafe_kms_available` is published only while Vault is enabled. Alert `DataSafeKMSUnavailable` in `deploy/docker/prometheus/alerts.yml` fires when that gauge stays 0 for 1 minute.

Activity writes `kms_unavailable` and `kms_recovered` on the transition. The resource is the node id. The row has no secret.

The console page is **Settings → Vault (KMS)** (`/admin/settings/kms`). **Test connection** calls `POST /api/v1/kms/test`. After an address is saved, the bucket card enables **Key from Vault**. The node table reads heartbeats from the last 10 minutes (Postgres migration `026`).

Rewrap and crypto-shred jobs are not in this step.
