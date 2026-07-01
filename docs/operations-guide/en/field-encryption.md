English | **[Русский](../ru/field-encryption.md)**

# Field encryption (metadata at rest)

**v1.0.3 · Community Edition · opt-in**

Application-layer envelope encryption for selected **metadata secrets** in Postgres and Bolt. Protects against plaintext exposure in a database dump or stolen metadata backup when the **KEK private key** is stored outside the DB.

This is **not** object (SSE-S3) encryption, **not** full-database TDE, and **not** a substitute for securing the running `storage-server` process.

## Edition placement

| Capability | Edition | v1.0.3 |
|------------|---------|--------|
| Field encryption with env / file KEK | **Community** | yes — no license gate |
| KEK via [Vault Agent](secrets-vault.md) → env | **Community** (ops) | same injection pattern as bootstrap secrets |
| Vault Transit wrap/unwrap for KEK | **Enterprise** | planned phase 2+ |
| HSM / formal key ceremony | **Enterprise** | planned phase 2+ |

Default installs behave like v1.0.2 until you set `STORAGE_FIELD_ENCRYPTION_ENABLED=true`.

## What is encrypted

When enabled, new and updated values for these paths are stored with wire prefix `enc:v1:`:

| Area | Field path |
|------|------------|
| Access keys | `access_keys.secret_key`, `access_keys.session_token` |
| Gateway connections | `gateway_connections.access_key`, `gateway_connections.secret_key` |
| System config (JSON leaves) | `ldap.bind_password`, `oidc.client_secret`, `external_s3.secret_access_key`, logging tokens/passwords |

**Not encrypted in v1.0.3:** `password_hash`, API token hashes, TOTP secrets (`aes:` scheme), `shared_links.token`, non-secret config (URLs, booleans, usernames).

Design reference: [field-encryption-1.0.3-tz.md](../../specs/field-encryption-1.0.3-tz.md).

## Threat model (summary)

| Protected | Not protected |
|-----------|---------------|
| Postgres / Bolt dump without KEK private key | Compromised running server (KEK in memory/env) |
| SQL injection exfil of ciphertext columns | Insider with admin API (decrypted via normal APIs) |
| Metadata backup on NAS/S3 | Leaked KEK private key |

## Enable field encryption

### 1. Generate a KEK (X25519)

Requires OpenSSL 3+ with X25519 support.

```bash
./scripts/crypto/generate-kek.sh
```

Windows:

```powershell
.\scripts\crypto\generate-kek.ps1
```

Output: PEM files under `data/keys/` (gitignored) and env-ready base64 values.  
Optional: `DATASAFE_KEK_ID=my-kek-id ./scripts/crypto/generate-kek.sh`.

Step-by-step examples: [scripts/crypto/README.md](../../../scripts/crypto/README.md).

### 2. Configure environment

| Variable | Required | Description |
|----------|----------|-------------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | — | `true` to enable. Default `false`. |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | if enabled | Stable ID, e.g. `kek-20260630-a3f1`. Must match active registry row. |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | if enabled | Base64 raw 32-byte X25519 private seed. |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | rotation | JSON `{"old-id":"b64priv","new-id":"b64priv"}` for multi-key decrypt. |

Example `.env` fragment (never commit):

```env
STORAGE_FIELD_ENCRYPTION_ENABLED=true
STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=kek-20260630-a3f1
STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=<base64-from-generate-kek>
STORAGE_DEV=false
STORAGE_STRICT_SECRETS=true
```

With `ENABLED=true`, startup **fails** if the private key is missing or `ACTIVE_KEK_ID` does not match the active registry entry.

### 3. Registry bootstrap

Public KEK metadata is stored in metadata, not the private key:

- **Postgres:** table `encryption_key_registry` (migration `012_field_encryption`).
- **Bolt:** JSON document at `config` / `encryption_key_registry`.

On **first start** with encryption enabled and an **empty registry**, the server auto-registers the public key derived from env (idempotent). If the registry already has an active key that does not match env → **fail startup** (no silent mismatch).

Manual inspect (Postgres):

```sql
SELECT kek_id, is_active, algorithm, octet_length(public_key) AS pub_len,
       created_at, rotated_at, retired_at
FROM encryption_key_registry
ORDER BY created_at;
```

### 4. Restart and verify

```bash
docker compose --profile postgres up -d storage-server
```

**Security status** (no secrets in response):

```bash
TOKEN=$(curl -s -X POST http://localhost:9000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_PASSWORD"}' | jq -r .token)

curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:9000/api/v1/settings/security-status | jq .field_encryption
```

Expected shape:

```json
{
  "enabled": true,
  "active_kek_id": "kek-20260630-a3f1",
  "registry_count": 1,
  "legacy_plaintext_fields_estimate": 0
}
```

Also available in the console: **Admin → Settings → Security**.

Confirm ciphertext after creating an access key:

```sql
SELECT left(secret_key, 12) AS prefix FROM access_keys ORDER BY created_at DESC LIMIT 1;
-- prefix: enc:v1:...
```

## Vault Agent — inject KEK private key

DataSafeS3 has **no in-app Vault SDK** for KEK. Use the same env-injection pattern as JWT and Postgres passwords: Vault Agent renders `STORAGE_FIELD_ENCRYPTION_*` into a sourced env file before `storage-server` starts.

Example KV v2 path `secret/datasafe/field-encryption`:

```json
{
  "enabled": "true",
  "active_kek_id": "kek-20260630-a3f1",
  "kek_private_key": "<base64 raw 32-byte seed>"
}
```

Agent template snippet:

```gotemplate
{{- with secret "secret/data/datasafe/field-encryption" -}}
STORAGE_FIELD_ENCRYPTION_ENABLED={{ .Data.data.enabled }}
STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID={{ .Data.data.active_kek_id }}
STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY={{ .Data.data.kek_private_key }}
{{- end }}
```

During rotation, add `kek_private_keys` to Vault and map to `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS`. See [secrets-vault.md](secrets-vault.md) for Compose, Helm, and K8s Injector layouts.

**Vault Transit** for KEK wrap/unwrap is **Enterprise phase 2** — not required for Community deployments that inject the raw private seed via Agent.

## KEK rotation (lazy re-encrypt)

v1.0.3 uses **lazy** re-encryption: plaintext legacy rows stay readable until the next write to that field. No mandatory offline migration.

Operator checklist:

```bash
OLD_KEK_ID=kek-20260630-a3f1 NEW_KEK_ID=kek-20260701-b2c4 ./scripts/crypto/rotate-kek.sh
```

Procedure:

1. **Generate** new pair: `DATASAFE_KEK_ID=kek-20260701-b2c4 ./scripts/crypto/generate-kek.sh`
2. **Register** new public key in `encryption_key_registry`; set old row `is_active=false`, `rotated_at=NOW()`.
3. **Env:** set `ACTIVE_KEK_ID` to the new ID; put both private keys in `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS`.
4. **Rolling restart** all `storage-server` replicas.
5. **Re-wrap:** update access keys, gateway credentials, or system settings via API/console (each write re-encrypts with the active KEK if the blob used an older `kek_id`). Optional batch: `POST /api/v1/admin/encryption/rewrap` when shipped.
6. **Verify** `legacy_plaintext_fields_estimate` → 0 in security-status.
7. **Remove** old private key from env / Vault.
8. **Retire** old registry row: `UPDATE encryption_key_registry SET retired_at=NOW() WHERE kek_id='...'`.

After `retired_at` is set, decrypt with that `kek_id` is rejected under strict policy.

## Upgrade from v1.0.2

- Default unchanged: `STORAGE_FIELD_ENCRYPTION_ENABLED=false`.
- Migration `012_field_encryption` applies automatically on Postgres startup.
- Enabling encryption is a deliberate ops step (generate KEK → env → restart). Plan a maintenance window if you enable on an existing deployment with many secrets — lazy re-encrypt spreads across normal writes.

See [upgrade.md § Upgrading to v1.0.3](upgrade.md#upgrading-to-v103).

## Related docs

- [scripts/crypto/README.md](../../../scripts/crypto/README.md) — key generation examples
- [secrets-vault.md](secrets-vault.md) — Vault Agent env injection
- [security-self-assessment.md](security-self-assessment.md) — control matrix
- [Spec](../../specs/field-encryption-1.0.3-tz.md) — wire format, crypto design, acceptance criteria
