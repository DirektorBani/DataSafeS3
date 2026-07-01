# ТЗ: Шифрование чувствительных полей метаданных (field encryption) — v1.0.3

**Версия:** 1.0  
**Дата:** 2026-06-30  
**Статус:** Спецификация (реализация — следующий этап)  
**Релиз:** v1.0.3 (расширение security surface; не входило в consensus must-ship 2026-06-29)

---

## 0. Контекст релиза 1.0.3

### Уже сделано / в работе (не дублировать)

| Область | Статус | Связь с field encryption |
|---------|--------|--------------------------|
| CI e2e-smoke, AUD-03 Postgres FK, SSRF tests/docs | must-ship trust-patch | Независимо |
| Vault **env injection** (sidecar → `STORAGE_*`) | WIP / ops pattern | Управляет **bootstrap-секретами процесса**, не полями БД |
| Security-status panel (`GET /settings/security-status`) | stretch | Расширить полем `field_encryption_enabled` / `active_kek_id` (без значений ключей) |
| TOTP at-rest (`users.totp_secret`, префикс `aes:`) | v1.0.2 | Отдельная схема (AES-GCM от JWT/MFA key); **не** объединять в phase 1 — миграция TOTP → envelope в phase 2 |

### Эта работа (новое)

Opt-in шифрование **выбранных полей метаданных** в Postgres и Bolt: envelope encryption (X25519 ECDH + AES-256-GCM). Защита от утечки дампа БД без приватного KEK.

**Артефакты планирования (этот PR):** ТЗ, миграция `012_field_encryption`, локальные скрипты `scripts/crypto/`. **Полный runtime-код — отдельная задача dev-агента.**

---

## 1. Цели

1. Чувствительные значения в метаданных (access keys, gateway credentials, фрагменты `system_config`) хранятся **зашифрованными at-rest** при включённом флаге.
2. Единый **wire format** для Postgres (TEXT/JSONB) и Bolt (JSON string fields).
3. **Версионирование KEK** (`kek_id`): ротация без массовой миграции; lazy re-encrypt при записи; decrypt по любому не-retired ключу.
4. **Обратная совместимость:** существующие plaintext-значения читаются без ошибок; новые записи шифруются только при `STORAGE_FIELD_ENCRYPTION_ENABLED=true`.
5. Community Edition: KEK из env / файла. Enterprise (phase 2): опционально Vault Transit / HSM — **вне scope phase 1**.

## 2. Non-goals (phase 1)

- Шифрование объектов на диске (SSE-S3 уже отдельно).
- Hot reload KEK без рестарта процесса.
- In-app HashiCorp Vault SDK для KEK (см. assessment → v1.2.0 для bootstrap secrets).
- Шифрование `password_hash`, `api_tokens.token_hash`, `users.totp_secret` (TOTP — legacy `aes:` до phase 2).
- `shared_links.token` — plaintext в phase 1; **phase 2:** migrate to hash-only lookup (как API tokens).
- Transparent column-level encryption Postgres (TDE) — только application-layer.
- License gates / Enterprise-only базовое шифрование полей.

---

## 3. Threat model

### Защищаем

| Угроза | Митигация |
|--------|-----------|
| Утечка дампа Postgres / копии Bolt-файла | Ciphertext + wrapped DEK; без KEK private key plaintext недоступен |
| SQL injection с exfil SELECT | То же; атакующий видит `enc:v1:...` blobs |
| Бэкап метаданных на S3/NAS | Ciphertext at-rest |

### Не защищаем (explicit)

| Угроза | Примечание |
|--------|------------|
| Компрометация running `storage-server` | KEK в памяти/env; RCE = полный доступ |
| Компрометация KEK private key | Оператор обязан хранить KEK вне БД (Vault Agent, K8s Secret, HSM) |
| Insider с admin API | Admin читает расшифрованные значения через штатные API |
| Replay / tamper без AEAD | AES-GCM + AAD с `kek_id` и field path |

### Assumptions

- KEK private key **никогда** не пишется в метаданные, audit logs, `GET /settings/*`.
- Prod: `STORAGE_DEV=false` + `STORAGE_STRICT_SECRETS=true` + отказ старта без KEK при enabled=true.

---

## 4. Криптодизайн

### 4.1. Алгоритмы

| Компонент | Алгоритм |
|-----------|----------|
| KEK | X25519 (Curve25519 DH), 32-byte seed / 32-byte public |
| DEK | Случайные 32 байта на операцию encrypt |
| Payload | AES-256-GCM |
| KDF wrap | ECDH(shared_secret) → SHA-256 → AES-256-GCM для wrap DEK |
| Nonce payload | 12 байт random (стандарт GCM) |
| Nonce wrap | 12 байт random |

Использовать `crypto/ecdh` (Go 1.20+) для X25519. Не изобретать custom KDF beyond SHA-256(shared_secret) для phase 1.

### 4.2. Wire format (canonical)

Префикс **`enc:v1:`** — единственный маркер зашифрованного значения (TEXT columns и string leaf в JSON).

```
enc:v1:<kek_id>:<nonce_b64>:<ciphertext_b64>:<ephemeral_pub_b64>:<wrapped_dek_b64>
```

| Сегмент | Описание |
|---------|----------|
| `v1` | Версия формата (не путать с `kek_id`) |
| `kek_id` | Стабильный идентификатор KEK, напр. `kek-2026-06-30-a` (URL-safe, без `:`) |
| `nonce_b64` | StdEncoding Base64, 12 байт nonce для payload GCM |
| `ciphertext_b64` | AES-GCM ciphertext payload (plaintext UTF-8 string) |
| `ephemeral_pub_b64` | Ephemeral X25519 public key (32 bytes raw → b64) для ECDH с KEK public |
| `wrapped_dek_b64` | AES-GCM(DEK) с ключом из ECDH(KEK_priv, ephemeral_pub); nonce внутри blob или prepended — **зафиксировать в коде:** prepend 12-byte nonce к wrapped blob перед b64 |

**AAD (associated data)** для payload GCM: `v1|<kek_id>|<field_path>` где `field_path` — канонический путь, напр. `access_keys.secret_key`, `gateway_connections.secret_key`, `system_config.ldap.bind_password`.

**Encrypt flow:**

1. Generate ephemeral X25519 keypair `E`.
2. `shared = ECDH(E_priv, KEK_pub_from_registry)`.
3. `wrap_key = SHA256(shared)`.
4. Generate random DEK (32 bytes).
5. `wrapped_dek = AES-GCM_encrypt(wrap_key, DEK)`.
6. `ciphertext = AES-GCM_encrypt(DEK, plaintext, AAD=field_path)`.
7. Serialize wire string.

**Decrypt flow:**

1. If not `strings.HasPrefix(val, "enc:v1:")` → return val (legacy plaintext).
2. Parse segments; load KEK public by `kek_id` from registry; try private keys from env map `STORAGE_FIELD_ENCRYPTION_KEK_KEYS` (multi-key JSON or `<kek_id>=<b64priv>`).
3. ECDH + unwrap DEK + decrypt payload.

### 4.3. Связь с существующим TOTP crypto

`internal/auth/totp.go` использует префикс `aes:` и симметричный ключ из JWT/MFA env. **Phase 1 не меняет TOTP.** Phase 2 (optional): унификация на `enc:v1:` + envelope.

---

## 5. Версионирование эллиптических ключей (KEK)

### 5.1. Реестр ключей

**Postgres:** таблица `encryption_key_registry` (миграция `012_field_encryption.up.sql`).

**Bolt:** doc-only в phase 1 — хранить записи реестра в bucket `config`, ключ `encryption_key_registry` (JSON array). **Не добавлять** новый bucket в `init()` до реализации store-слоя; при реализации предпочесть `config` subkey для минимального diff.

| Поле | Назначение |
|------|------------|
| `kek_id` | Уникальный строковый ID (оператор задаёт при генерации) |
| `algorithm` | `x25519-aes256-gcm` |
| `public_key` | 32 bytes raw public (Postgres BYTEA) |
| `is_active` | Ровно один `true` среди non-retired |
| `created_at` | Создание |
| `rotated_at` | Когда ключ сменил статус active → inactive |
| `retired_at` | NULL = ключ ещё может decrypt; NOT NULL = decrypt запрещён (emergency only) |

**Private keys не в БД.** Только public metadata в registry.

### 5.2. Env / конфигурация процесса

| Переменная | Обязательность | Описание |
|------------|----------------|----------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | opt-in | `true` — encrypt on write + decrypt on read |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | if enabled (single-key MVP) | Base64 raw 32-byte X25519 private seed |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | if enabled | Должен совпадать с active row в registry |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | optional rotation | JSON `{"kek-id-a":"b64priv","kek-id-b":"b64priv"}` — все non-retired private keys для decrypt |

Public keys для decrypt-by-id без env: из `encryption_key_registry` (нужны только для audit/verify; decrypt использует private из env).

### 5.3. Bootstrap / first-run

1. Оператор генерирует пару (`scripts/crypto/generate-kek.sh`).
2. При первом старте с `ENABLED=true`: если registry пуст — **auto-register** public key + `kek_id` из env (idempotent).
3. Если registry не пуст, но env KEK не matches active → **fail startup** (no silent mismatch).

### 5.4. Ротация KEK (lazy re-encrypt)

**Процедура (оператор + `scripts/crypto/rotate-kek.sh` skeleton):**

1. Сгенерировать новую пару `kek-new` (`generate-kek`).
2. `INSERT` public в registry; `UPDATE` старый `is_active=false`, `rotated_at=now()`.
3. Добавить **оба** private key в `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS`; установить `ACTIVE_KEK_ID=kek-new`.
4. Rolling restart storage-server.
5. **Lazy re-encrypt:** при любом `PutAccessKey`, `PutGatewayConnection`, `PutSystemConfig` — если поле decrypts with old `kek_id`, re-encrypt with active KEK before save.
6. Опциональный фоновый job (phase 1.1): admin endpoint `POST /api/v1/admin/encryption/rewrap` — batch re-encrypt (rate-limited).
7. После 100% rewrap (метрика `field_encryption_legacy_kek_count`) — удалить старый private из env; через N дней `retired_at=now()` на старом ключе.

**Multi-key decrypt:** при decrypt перебрать private keys из env map по `kek_id` из blob; если `retired_at` set и политика strict — reject.

### 5.5. Генерация keypairs

- `scripts/crypto/generate-kek.sh` / `.ps1`: OpenSSL `genpkey -algorithm X25519` или Go helper.
- Output: `data/keys/kek-v1.key` (PEM), `data/keys/kek-v1.pub` (PEM) — каталог `data/` уже в `.gitignore`.
- `kek_id` suggestion: `kek-` + ISO date + `-` + short random, напр. `kek-20260630-x7k2`.

---

## 6. Поля для шифрования (phase 1)

### 6.1. Encrypt

| Store | Field / path | Column / JSON path |
|-------|--------------|-------------------|
| Access keys | `secret_key` | `access_keys.secret_key` |
| Access keys | `session_token` | `access_keys.session_token` (STS, migration 009) |
| Gateway | `access_key` | `gateway_connections.access_key` |
| Gateway | `secret_key` | `gateway_connections.secret_key` |
| System config | LDAP bind | `system_config` → `ldap.bind_password` |
| System config | OIDC | `system_config` → `oidc.client_secret` |
| System config | External S3 | `system_config` → `external_s3.secret_access_key` |
| System config | Logging sinks | `logging.elasticsearch.password`, `logging.elasticsearch.token`, `logging.loki.token`, `logging.webhook.token` |

Для JSONB: шифровать **только leaf string values**; при `PutSystemConfig` encrypt paths before marshal; при `Get` decrypt after unmarshal. Не шифровать entire JSON blob (сохраняет query/index semantics).

### 6.2. Explicitly NOT encrypted (phase 1)

| Field | Reason |
|-------|--------|
| `users.password_hash` | bcrypt/argon verify needs hash form |
| `api_tokens.token_hash` | lookup by hash |
| `users.totp_secret` | separate `aes:` scheme until phase 2 |
| `shared_links.token` | lookup by token; **phase 2:** store hash only |
| Non-secret config (URLs, booleans, usernames) | no benefit |

---

## 7. Миграция схемы и данных

### 7.1. Postgres

- **Новая таблица:** `encryption_key_registry` — см. `012_field_encryption.up.sql`.
- **Существующие колонки не менять тип** — значения остаются TEXT/JSONB; зашифрованные получают префикс `enc:v1:`.
- Nullable: `session_token` уже nullable; `secret_key` NOT NULL — пустая строка запрещена, ciphertext всегда non-empty string.

### 7.2. Bolt

- Те же string values в JSON records (`AccessKeyRecord`, `GatewayConnectionRecord`, `SystemConfig`).
- Реестр KEK: `config` bucket, key `encryption_key_registry` (JSON) — реализовать в store layer.

### 7.3. Backward compatibility

| Read | Write (enabled=false) | Write (enabled=true) |
|------|----------------------|----------------------|
| Plaintext passthrough | Plaintext | `enc:v1:...` |
| `enc:v1:` decrypt | Passthrough ciphertext unchanged | Re-encrypt if old kek_id |

### 7.4. Bulk migration

Phase 1 **без** offline migration job. Plaintext остаётся до первой записи или optional rewrap endpoint.

---

## 8. Изменения API / store layer

### 8.1. Новый пакет

`internal/security/fieldenc/`:

- `Encrypt(fieldPath, plaintext, activeKEK) (string, error)`
- `Decrypt(fieldPath, stored) (string, error)`
- `IsEncrypted(stored) bool`
- `RewrapIfNeeded(fieldPath, stored, activeKEK) (string, bool, error)` — lazy rotation
- Unit tests: roundtrip, wrong AAD, unknown kek_id, plaintext passthrough

### 8.2. Metadata interface

Обёртка **внутри** store implementations (не в handlers):

| Method file | Change |
|-------------|--------|
| `internal/metadata/postgres/access_multipart.go` | encrypt/decrypt `secret_key`, `session_token` in Put/Get/List |
| `internal/metadata/postgres/enterprise.go` | gateway `access_key`, `secret_key` |
| `internal/metadata/postgres/config.go` | system_config JSON path encrypt/decrypt |
| `internal/metadata/store.go` (Bolt) | mirror same fields |
| `internal/metadata/config.go` | helper `EncryptSystemConfigPaths` / `DecryptSystemConfigPaths` |

Inject `*fieldenc.Service` via `metadata.Open*` options or lazy init from env in `postgres.Open` / Bolt open.

### 8.3. Startup (`cmd/storage-server/main.go`)

- После load env, before store open: init fieldenc from env + sync registry row.
- If `ENABLED=true` and missing private key → log fatal.

### 8.4. Security status API

Extend `GET /api/v1/settings/security-status`:

```json
{
  "field_encryption": {
    "enabled": true,
    "active_kek_id": "kek-20260630-x7k2",
    "registry_count": 2,
    "legacy_plaintext_fields_estimate": 0
  }
}
```

No secret material. Counts only.

### 8.5. OpenAPI

Document env vars in operations guide; **не** expose encrypt/decrypt API for arbitrary fields.

---

## 9. Edition placement (PO note)

**PO score (field encryption bundle): ~6.2 / 10** — умеренная CE-ширина, compliance driver для self-hosted, средний engineering cost.

| Capability | Edition | Phase |
|------------|---------|-------|
| Field encryption with env/file KEK | **Community** | 1.0.3 |
| KEK via Vault Agent → env (same as bootstrap secrets) | **Community** (ops) | 1.0.3 docs |
| Vault Transit wrap/unwrap for KEK | **Enterprise** (optional) | 2.0+ |
| HSM / centralized key ceremony | **Enterprise** | 2.0+ |

**Без license gates** для базового `STORAGE_FIELD_ENCRYPTION_ENABLED`.

---

## 10. Тесты (обязательные)

### Unit

- `internal/security/fieldenc/*_test.go` — roundtrip, AAD tamper, invalid blob, multi-key decrypt
- System config path matrix (ldap, oidc, external_s3, logging tokens)

### Integration

- Postgres: PutAccessKey → DB contains `enc:v1:` → Get decrypts
- Bolt: same
- Enabled=false: plaintext roundtrip unchanged
- Migration 012 applies cleanly on empty + existing DB

### Regression

- `go test ./...` PASS
- STS session token flow still works with encrypted `session_token`
- Gateway connection test connection with encrypted creds
- S3 SigV4 auth with encrypted access key secret

### CI note

Tests use ephemeral KEK from `t.Setenv`; no committed keys.

---

## 11. Критерии приёмки

- [ ] `STORAGE_FIELD_ENCRYPTION_ENABLED=false` (default) — поведение идентично v1.0.2 для всех API.
- [ ] `ENABLED=true` + valid KEK — новые access keys / gateway / system_config secrets stored as `enc:v1:` in Postgres and Bolt.
- [ ] DB dump без private KEK не раскрывает plaintext (manual verify: `psql` SELECT shows enc blob).
- [ ] Legacy plaintext rows читаются и при update перешифровываются (lazy).
- [ ] Ротация: два KEK в env, decrypt old + encrypt new on write; registry reflects active key.
- [ ] Startup fails if enabled but KEK missing or `ACTIVE_KEK_ID` mismatch registry.
- [ ] `encryption_key_registry` migration up/down idempotent.
- [ ] `scripts/crypto/generate-kek.*` produces keys under `data/keys/`; README warns never commit.
- [ ] security-status reports enabled/kek_id without secrets.
- [ ] EN/RU ops guide section (short) — separate docs task or same PR if dev agent scope allows.

---

## 12. Разбивка задач для dev-агента (порядок)

1. **Миграция 012** — apply in `postgres` store embed; verify `go test` migration version.
2. **`internal/security/fieldenc`** — crypto primitives + wire format parser + tests.
3. **Registry store** — Postgres CRUD for `encryption_key_registry`; Bolt `config/encryption_key_registry` JSON.
4. **Startup wiring** — env parsing, registry bootstrap, inject into metadata stores.
5. **Postgres store hooks** — access_keys, gateway_connections, system_config paths.
6. **Bolt store hooks** — mirror postgres behavior.
7. **security-status** — extend handler + test.
8. **Lazy rewrap** — on Put* methods; optional admin rewrap endpoint (stretch).
9. **Integration tests** — Postgres + Bolt with `TEST_POSTGRES_DSN`.
10. **Docs EN/RU** — operations-guide section «Field encryption»; upgrade.md note for 1.0.3.
11. **CHANGELOG** — runtime feature entry under `[1.0.3]` when code ships.

---

## 13. Ссылки

- Consensus scope: `_local/publishing/release-1.0.3-consensus-scope-2026-06-29.md`
- Vault assessment: `_local/publishing/release-1.0.3-vault-assessment-2026-06-29.md`
- TOTP crypto (reference): `internal/auth/totp.go`
- Access keys schema: `internal/metadata/postgres/migrations/001_init.up.sql`, `009_list_perf_sts.up.sql`
- Local KEK tooling: `scripts/crypto/README.md`

---

*Спецификация v1.0 · 2026-06-30 · planning artifact only.*
