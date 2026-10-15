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
| `STORAGE_KMS_VAULT_TOKEN_FILE` | empty | Token file. Alternative to AppRole. The console can store a token; the environment form is a file. A token Vault will not renew (a dev-mode root token answers 400 `token not renewable`) is still used. HTTP 403, or 400 for any other reason, rejects it. |
| `STORAGE_KMS_VAULT_ADMIN_ROLE_ID` / `_SECRET_ID` / `_SECRET_ID_FILE` | empty | Separate AppRole for rotate and delete. |
| `STORAGE_KMS_DEFAULT_KEY` | `datasafe-<tenant>-<bucket>` | Default key name. When `STORAGE_KMS_PROBE_KEY` is empty, the connection test encrypts this key and does not create `datasafe-connection-test`. |
| `STORAGE_KMS_PROBE_KEY` | empty | Existing Transit key for **Test connection**. Encrypt and decrypt only. Delete is not used. |
| `STORAGE_KMS_KEY_ALLOWLIST` | `datasafe-*` | Names a caller may request. |
| `STORAGE_KMS_DEK_CACHE_TTL` | `5m` | How long an unwrapped data key stays in memory. |
| `STORAGE_KMS_POLL_INTERVAL` | `2s` | How often the node reads `kms_keys.generation`, probes Vault, and reloads shared settings. A Postgres follower reloads a console save within 2 seconds. |
| `STORAGE_KMS_TIMEOUT` | `5s` | One Vault call. |

The client is `net/http`. There is no Vault SDK in the server. A node dials only after the provider and an address are both present. Shared settings live in metadata (`system_config.kms`). The secret id and token are sealed at rest even when `STORAGE_FIELD_ENCRYPTION_ENABLED` is false: AES-GCM with the prefix `kse:v1`, under a key derived from `STORAGE_SSE_MASTER_KEY`, or from `STORAGE_JWT_SECRET` when the master key is empty. When field encryption is on, those two fields are stored as `enc:v1` instead. `GET /api/v1/settings/system`, `GET /api/v1/kms/settings`, Activity export, `/healthz`, and logs do not return them. A copy of `metadata.db` or of the Postgres `system_config` row does not contain the plaintext. A saved secret is shown as set, and the field accepts a replacement only.

`PUT /api/v1/kms/settings` encrypts and decrypts one value before it stores the address. A failed probe is HTTP 400, or HTTP 408 when Vault does not answer, and the previous settings stay. Clearing the address does not probe. The console leaves **Save** disabled until **Test connection** has succeeded for the current form. **Test connection** encrypts and decrypts one value. It uses `STORAGE_KMS_PROBE_KEY`, or `STORAGE_KMS_DEFAULT_KEY` when the probe key is empty. With neither set, it creates `datasafe-connection-test` if that key is missing and leaves it. The node policy does not need delete on that key. Delete stays on the admin policy, which crypto-shred uses.

A Postgres node reloads shared Vault settings on the 2 second poll (`STORAGE_KMS_POLL_INTERVAL` when that is shorter). A single Bolt node applies a save on that process immediately and does not run the follower poll.

Each node can keep its own AppRole in the environment. That secret is not written back to the shared config.

Lab overlay (dev mode, root token, not for production): `deploy/compose/docker-compose.vault-kms.yml`. Policies: `deploy/vault/policies/datasafe-kms.hcl` (encrypt, decrypt, rewrap, and create/read on `datasafe-*`; no delete) and `datasafe-kms-admin.hcl` (rotate, delete, and `deletion_allowed`). A key this server creates has `deletion_allowed` set at creation. Without that flag Vault refuses to delete the key, and crypto-shred records the error on the job.

## When Vault is down

A new SSE-KMS object is wrapped before anything is written. If Vault is down and the data key is not in the cache, the write is HTTP 503 `KMS unavailable` and nothing is stored. A read of an SSE-KMS object misses the same way. A cache hit inside the TTL still decrypts. Plaintext buckets and SSE-S3 do not call Vault.

A share link for an SSE-KMS object returns HTTP 503 with `Файл временно недоступен, попробуйте позже`. A plaintext share link is unchanged.

`/healthz` adds `kms.provider` and `kms.available`. It reports the latest probe and does not open a new Vault call on that request. A probe that finishes after a newer one is ignored, so a recovered node does not stay `available: false`. The console object download returns the same HTTP 503 `KMS unavailable` as S3 when Vault is down. The gauge `datasafe_kms_available` is published only while Vault is enabled. Alert `DataSafeKMSUnavailable` in `deploy/docker/prometheus/alerts.yml` fires when that gauge stays 0 for 1 minute.

Activity writes `kms_unavailable` and `kms_recovered` on the transition. The resource is the node id. The row has no secret.

The console page is **Settings → Vault (KMS)** (`/admin/settings/kms`). **Test connection** calls `POST /api/v1/kms/test`. After an address is saved, the bucket card enables **Key from Vault**. The node table reads heartbeats from the last 10 minutes (Postgres migration `026`).

Key rotation, rewrap, and crypto-shred run only on the leader. Progress is a row in `kms_jobs` (Postgres migration `029`). `POST /api/v1/kms/keys/{id}/rotate` stores a new key version and increments `generation`. New objects use that version immediately. Existing objects stay readable until **Encrypt existing objects** (`POST /api/v1/buckets/{bucket}/encryption/rewrap`). That job rewraps a data key that is already at an older version without reading the object bytes, and it encrypts a plaintext object by streaming it once (the plaintext and the ciphertext are not both held in memory). Encrypting a plaintext object replaces its ETag with the MD5 of the ciphertext. A second pass over an object already at the target version does nothing. Activity records `kms_job_started` when an administrator starts the job (user and bucket) and `kms_job_finished` or `kms_job_failed` when it ends (`done`, `total`, `errors`). The job row `holder_id` is this node's id (`STORAGE_NODE_ID`, or the HA node id). **Destroy key** (`POST /api/v1/buckets/{bucket}/encryption/destroy`) is limited to a cluster administrator, a tenant administrator of that bucket, or a user with an explicit bucket-admin grant. The owner of the bucket is not enough when their role is `user`, and an operator is refused (HTTP 403). The call asks for the bucket name and requires a step-up token when the MFA policy is on. The key is marked `destroyed`, `generation` increases, and a job deletes wrapped data keys. A local server key has no remote key to delete: the job erases wrapped data keys in the current metadata only. An older backup of that metadata still contains them. The console destroy dialog says so. A read of any object in that bucket is then HTTP 403 `Ключ шифрования уничтожен, файл недоступен навсегда`, including a share link and a presigned GET. A destroyed share publishes `ShareDownloaded` with `result=key_destroyed`. A successful share or presigned decrypt records `key_id` and `key_version` in Activity and the event. `GET /api/v1/kms/jobs/{id}` is the progress the console polls. `GET /api/v1/kms/jobs/{id}/report` is the JSON report. The jobs counter `datasafe_kms_jobs_objects_total` is registered when the first object is finished, not at process start.

A Vault Transit key is created with `deletion_allowed` set, which Transit requires before `DELETE`. Crypto-shred calls that delete with the admin AppRole (`STORAGE_KMS_VAULT_ADMIN_*`) when one is set. If Vault refuses the delete, the bucket key is still marked `destroyed` and wrapped data keys are still cleared, and the job field `vault_delete_error` plus the console danger zone show the Vault error. `GET /api/v1/public/share/{token}` after destroy is HTTP 403 `Ключ шифрования уничтожен, файл недоступен навсегда` (the share page shows that sentence). A follower that is not the leader forwards a write, or returns HTTP 503 `not cluster leader` with `Retry-After`. The same 503 is returned when the metadata commit hits a read-only transaction (SQLSTATE 25006) or the Postgres connection drops during a leader change. That used to be HTTP 500 `InternalError`.

A write that reaches a node whose process has already been killed has no HTTP status: the client sees a connection reset and retries.
