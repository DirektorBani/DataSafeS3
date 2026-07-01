# Field encryption KEK tooling

Local X25519 key generation for **field encryption** (metadata secrets at rest).  
**Dev / staging only** unless you inject keys via Vault Agent or K8s Secret in production.

Private keys **must never** be committed. Output goes to `data/keys/` (covered by repo `.gitignore` → `data/`).

Full operator guide: [field-encryption.md](../../docs/operations-guide/en/field-encryption.md) (EN) · [RU](../../docs/operations-guide/ru/field-encryption.md)

---

## Quick start — generate a KEK

### Linux / macOS / Git Bash

```bash
./scripts/crypto/generate-kek.sh
```

### Windows (PowerShell)

```powershell
.\scripts\crypto\generate-kek.ps1
```

Optional overrides:

```bash
DATASAFE_KEK_ID=kek-prod-a ./scripts/crypto/generate-kek.sh
DATASAFE_KEK_DIR=/secure/keys ./scripts/crypto/generate-kek.sh
```

The script:

1. Creates `data/keys/` with mode `700` (bash).
2. Generates X25519 PEM pair via OpenSSL (`genpkey -algorithm X25519`).
3. Prints base64 **raw 32-byte** private seed and public key for env / registry.
4. Refuses to overwrite existing files (set `DATASAFE_KEK_ID` or remove old PEMs).

Example output (values are illustrative):

```text
=== DataSafeS3 field encryption KEK (local only) ===
kek_id suggestion: kek-20260630-a3f1
Private PEM: .../data/keys/kek-20260630-a3f1.key
Public PEM:  .../data/keys/kek-20260630-a3f1.pub

Add to .env (NEVER commit):
STORAGE_FIELD_ENCRYPTION_ENABLED=true
STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=kek-20260630-a3f1
STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=<base64>
```

---

## Step-by-step — enable field encryption

### 1. Generate the keypair

```bash
./scripts/crypto/generate-kek.sh
```

Copy the three `STORAGE_FIELD_ENCRYPTION_*` lines from the script output.

### 2. Add variables to `.env`

```env
STORAGE_FIELD_ENCRYPTION_ENABLED=true
STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=kek-20260630-a3f1
STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=<paste base64 from script>
```

Production: also set `STORAGE_DEV=false` and `STORAGE_STRICT_SECRETS=true`.  
With encryption enabled, the server **refuses to start** if the private key or active `kek_id` is missing or mismatched with the registry.

### 3. Restart `storage-server`

```bash
docker compose --profile postgres up -d storage-server
# or: go run ./cmd/storage-server   (native dev)
```

On first start with an empty `encryption_key_registry`, the server **auto-registers** the public key and `kek_id` from env (idempotent).

### 4. Verify via security-status

```bash
TOKEN=$(curl -s -X POST http://localhost:9000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_ADMIN_PASSWORD"}' | jq -r .token)

curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:9000/api/v1/settings/security-status | jq .
```

Expect (when runtime is enabled):

```json
"field_encryption": {
  "enabled": true,
  "active_kek_id": "kek-20260630-a3f1",
  "registry_count": 1,
  "legacy_plaintext_fields_estimate": 0
}
```

No secret material is returned. Same data appears in **Admin → Settings → Security** in the console.

### 5. Confirm ciphertext in metadata (Postgres)

Create or update an access key, then inspect the row:

```sql
SELECT access_key_id, left(secret_key, 20) AS secret_prefix
FROM access_keys
ORDER BY created_at DESC
LIMIT 1;
```

Encrypted values start with `enc:v1:`.

---

## Environment variables

| Variable | Required when enabled | Description |
|----------|----------------------|-------------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | — | `true` = encrypt on write, decrypt on read. Default `false` (v1.0.2 behaviour). |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | yes | Must match the single active row in `encryption_key_registry`. |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | yes (single-key) | Base64 raw 32-byte X25519 private seed. |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | rotation | JSON map `{"old-id":"b64","new-id":"b64"}` — all non-retired private keys for decrypt during rotation. |

Public keys live in the metadata registry only; private keys stay in env / Vault / K8s Secret.

---

## KEK rotation (lazy re-encrypt)

No offline migration job in v1.0.3. Existing plaintext is re-encrypted **on the next write** to that field.

Checklist script (manual steps until admin rewrap API ships):

```bash
OLD_KEK_ID=kek-20260630-a3f1 NEW_KEK_ID=kek-20260701-b2c4 ./scripts/crypto/rotate-kek.sh
```

Summary:

1. Generate new pair: `DATASAFE_KEK_ID=kek-20260701-b2c4 ./scripts/crypto/generate-kek.sh`
2. Insert new public key in `encryption_key_registry`; deactivate old key (`is_active=false`, `rotated_at=NOW()`).
3. Set env with **both** private keys in `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS`; point `ACTIVE_KEK_ID` at the new key.
4. Rolling restart all `storage-server` instances.
5. Touch records (update access keys, gateway, settings) or call `POST /api/v1/admin/encryption/rewrap` when available.
6. When `legacy_plaintext_fields_estimate` → 0, remove the old private key from env; later set `retired_at` on the old registry row.

See [field-encryption.md § Rotation](../../docs/operations-guide/en/field-encryption.md#kek-rotation-lazy-re-encrypt) and [TZ §5.4](../../docs/specs/field-encryption-1.0.3-tz.md).

---

## Edition note (honest CE positioning)

| Capability | Edition | v1.0.3 |
|------------|---------|--------|
| Field encryption with env / file KEK | **Community** | yes, opt-in, no license gate |
| KEK via Vault Agent → env | **Community** (ops pattern) | documented |
| Vault Transit wrap/unwrap for KEK | **Enterprise** | phase 2+ |
| HSM / centralized key ceremony | **Enterprise** | phase 2+ |

---

## Security

- Do not store `.key` files in git, DB backups, ticket screenshots, or chat.
- Running process holds the KEK in memory — RCE on `storage-server` bypasses at-rest protection.
- DB dump without the private KEK shows `enc:v1:…` blobs only.
- Admin API still returns decrypted values to authorized admins (by design).

Spec: [docs/specs/field-encryption-1.0.3-tz.md](../../docs/specs/field-encryption-1.0.3-tz.md)

---

## Быстрый старт (RU)

```bash
./scripts/crypto/generate-kek.sh
# или: .\scripts\crypto\generate-kek.ps1
```

Скрипт создаёт PEM в `data/keys/`, выводит строки для `.env`. После `STORAGE_FIELD_ENCRYPTION_ENABLED=true` и рестарта сервер регистрирует public key в `encryption_key_registry`, новые секреты пишутся с префиксом `enc:v1:`. Проверка: `GET /api/v1/settings/security-status` и Admin → Settings → Security.

Ротация: `./scripts/crypto/rotate-kek.sh` (чеклист). Подробно: [field-encryption.md (RU)](../../docs/operations-guide/ru/field-encryption.md).
