**[English](../en/field-encryption.md)** | Русский

# Шифрование полей метаданных (field encryption)

**v1.0.3 · Community Edition · opt-in**

Application-layer envelope encryption для выбранных **секретов метаданных** в Postgres и Bolt. Защищает от plaintext в дампе БД или украденном backup метаданных, если **приватный KEK** хранится вне БД.

Это **не** шифрование объектов (SSE-S3), **не** TDE всей БД и **не** замена hardening процесса `storage-server`.

## Размещение по edition

| Возможность | Edition | v1.0.3 |
|-------------|---------|--------|
| Field encryption с KEK из env / файла | **Community** | да — без license gate |
| KEK через [Vault Agent](secrets-vault.md) → env | **Community** (ops) | тот же паттерн, что bootstrap-секреты |
| Vault Transit wrap/unwrap для KEK | **Enterprise** | phase 2+ |
| HSM / формальная церемония ключей | **Enterprise** | phase 2+ |

По умолчанию поведение как в v1.0.2, пока не задано `STORAGE_FIELD_ENCRYPTION_ENABLED=true`.

## Что шифруется

При включении новые и обновлённые значения получают префикс `enc:v1:`:

| Область | Путь поля |
|---------|-----------|
| Access keys | `access_keys.secret_key`, `access_keys.session_token` |
| Gateway | `gateway_connections.access_key`, `gateway_connections.secret_key` |
| System config (JSON leaves) | `ldap.bind_password`, `oidc.client_secret`, `external_s3.secret_access_key`, токены/пароли logging |

**Не шифруется в v1.0.3:** `password_hash`, хеши API-токенов, TOTP (`aes:`), `shared_links.token`, несекретный config.

Спецификация: [field-encryption-1.0.3-tz.md](../../specs/field-encryption-1.0.3-tz.md).

## Threat model (кратко)

| Защищаем | Не защищаем |
|----------|-------------|
| Дамп Postgres/Bolt без приватного KEK | Компрометация running server (KEK в памяти/env) |
| SQL injection → ciphertext | Insider с admin API |
| Backup метаданных на NAS/S3 | Утечка приватного KEK |

## Включение field encryption

### 1. Генерация KEK (X25519)

Нужен OpenSSL 3+ с X25519.

```bash
./scripts/crypto/generate-kek.sh
```

Windows:

```powershell
.\scripts\crypto\generate-kek.ps1
```

Результат: PEM в `data/keys/` (gitignore) и base64 для env.  
Опционально: `DATASAFE_KEK_ID=my-kek-id ./scripts/crypto/generate-kek.sh`.

Примеры по шагам: [scripts/crypto/README.md](../../../scripts/crypto/README.md).

### 2. Переменные окружения

| Переменная | Обязательность | Описание |
|------------|----------------|----------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | — | `true` — включить. По умолчанию `false`. |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | при enabled | Стабильный ID, напр. `kek-20260630-a3f1`. Должен совпадать с active в registry. |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | при enabled | Base64 raw 32-byte X25519 private seed. |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | ротация | JSON `{"old-id":"b64","new-id":"b64"}` для multi-key decrypt. |

Фрагмент `.env` (не коммитить):

```env
STORAGE_FIELD_ENCRYPTION_ENABLED=true
STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=kek-20260630-a3f1
STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=<base64 из generate-kek>
STORAGE_DEV=false
STORAGE_STRICT_SECRETS=true
```

При `ENABLED=true` старт **падает**, если нет приватного ключа или `ACTIVE_KEK_ID` не совпадает с active в registry.

### 3. Bootstrap registry

Публичные метаданные KEK — в метаданных, приватный ключ — только в env:

- **Postgres:** таблица `encryption_key_registry` (миграция `012_field_encryption`).
- **Bolt:** JSON `config` / `encryption_key_registry`.

При **первом старте** с enabled и **пустым registry** сервер idempotent регистрирует public key из env. Если active в registry не совпадает с env → **fail startup**.

Проверка (Postgres):

```sql
SELECT kek_id, is_active, algorithm, octet_length(public_key) AS pub_len,
       created_at, rotated_at, retired_at
FROM encryption_key_registry
ORDER BY created_at;
```

### 4. Рестарт и проверка

```bash
docker compose --profile postgres up -d storage-server
```

**Security status** (без секретов в ответе):

```bash
TOKEN=$(curl -s -X POST http://localhost:9000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_PASSWORD"}' | jq -r .token)

curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:9000/api/v1/settings/security-status | jq .field_encryption
```

Ожидаемая форма:

```json
{
  "enabled": true,
  "active_kek_id": "kek-20260630-a3f1",
  "registry_count": 1,
  "legacy_plaintext_fields_estimate": 0
}
```

Также: **Admin → Settings → Security** в консоли.

После создания access key:

```sql
SELECT left(secret_key, 12) AS prefix FROM access_keys ORDER BY created_at DESC LIMIT 1;
-- prefix: enc:v1:...
```

## Vault Agent — инъекция приватного KEK

В приложении **нет Vault SDK** для KEK. Используйте env-injection как для JWT и пароля Postgres: Agent рендерит `STORAGE_FIELD_ENCRYPTION_*` в файл env до старта `storage-server`.

Пример KV v2 `secret/datasafe/field-encryption`:

```json
{
  "enabled": "true",
  "active_kek_id": "kek-20260630-a3f1",
  "kek_private_key": "<base64 raw 32-byte seed>"
}
```

Фрагмент шаблона Agent:

```gotemplate
{{- with secret "secret/data/datasafe/field-encryption" -}}
STORAGE_FIELD_ENCRYPTION_ENABLED={{ .Data.data.enabled }}
STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID={{ .Data.data.active_kek_id }}
STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY={{ .Data.data.kek_private_key }}
{{- end }}
```

При ротации добавьте `kek_private_keys` в Vault → `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS`. Compose, Helm, Injector: [secrets-vault.md](secrets-vault.md).

**Vault Transit** для KEK — **Enterprise phase 2**; для Community достаточно инъекции raw seed через Agent.

## Ротация KEK (lazy re-encrypt)

В v1.0.3 **lazy** re-encrypt: legacy plaintext читается до следующей записи поля. Offline migration не обязателен.

Чеклист:

```bash
OLD_KEK_ID=kek-20260630-a3f1 NEW_KEK_ID=kek-20260701-b2c4 ./scripts/crypto/rotate-kek.sh
```

Процедура:

1. **Сгенерировать** пару: `DATASAFE_KEK_ID=kek-20260701-b2c4 ./scripts/crypto/generate-kek.sh`
2. **Зарегистрировать** public key; у старого `is_active=false`, `rotated_at=NOW()`.
3. **Env:** новый `ACTIVE_KEK_ID`; оба private key в `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS`.
4. **Rolling restart** всех реплик `storage-server`.
5. **Re-wrap:** обновить access keys, gateway, settings через API/консоль. Опционально: `POST /api/v1/admin/encryption/rewrap`.
6. **Проверить** `legacy_plaintext_fields_estimate` → 0 в security-status.
7. **Удалить** старый private key из env / Vault.
8. **Retire:** `UPDATE encryption_key_registry SET retired_at=NOW() WHERE kek_id='...'`.

После `retired_at` decrypt с этим `kek_id` запрещён (strict policy).

## Обновление с v1.0.2

- По умолчанию без изменений: `STORAGE_FIELD_ENCRYPTION_ENABLED=false`.
- Миграция `012_field_encryption` применяется при старте Postgres backend.
- Включение — осознанный шаг ops (KEK → env → restart). На больших инсталляциях lazy re-encrypt распределяется по обычным записям.

См. [upgrade.md § Обновление до v1.0.3](upgrade.md#обновление-до-v103).

## Связанные документы

- [scripts/crypto/README.md](../../../scripts/crypto/README.md)
- [secrets-vault.md](secrets-vault.md)
- [security-self-assessment.md](security-self-assessment.md)
- [Спецификация](../../specs/field-encryption-1.0.3-tz.md)
