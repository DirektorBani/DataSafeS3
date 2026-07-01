**[English](../en/upgrade.md)** | Русский

# Обновление

## Docker Compose

```bash
git pull
docker compose --profile postgres build storage-server
docker compose --profile postgres up -d
```

С local binary overlay (Windows dev):

```cmd
scripts\dev-docker-local-binary.cmd
```

## Миграции

Миграции PostgreSQL выполняются автоматически при старте `storage-server` (`internal/metadata/postgres/migrations/`).

## Откат

1. Остановить стек
2. Восстановить предыдущий binary/image и backup данных
3. Запустить стек

## Проверка образов релиза (cosign)

Перед обновлением на тег проверьте подписи GHCR (см. [SECURITY.md](../../../SECURITY.md)):

```bash
export COSIGN_EXPERIMENTAL=1
TAG=v1.0.3
cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

SBOM прикреплены к каждому [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Обновление до v1.0.2

Релиз **v1.0.2** — security patch. Новых продуктовых функций нет, но меняются дефолты и поток аутентификации. Заложите короткое окно обслуживания и обновляйте **storage-server и console вместе** — сервер v1.0.2 со старой консолью (или наоборот) ломает OIDC-вход.

### Зачем этот патч

В прежних версиях JWT сессии OIDC попадал в URL редиректа (`?token=…`). Токен мог утечь через историю браузера, логи прокси или Referer. В v1.0.2 вместо него одноразовый `exchange_code`, который консоль обменивает через POST. Отдельно сервер проверяет каждый исходящий HTTP (log sinks, webhooks, тесты hook, gateway) — защита от SSRF во внутренние сети. На login-endpoint добавлен rate limit по IP; в production явнее предупреждаем о дефолтных секретах.

### OIDC callback (ломает смешанные версии)

Redirect URI у IdP не меняется (`/api/v1/auth/oidc/callback`). После входа браузер получает `/login?exchange_code=…&auth_source=oidc` вместо `?token=…`. Консоль вызывает `POST /api/v1/auth/oidc/exchange` и сохраняет JWT из тела ответа. Обновите оба образа до следующего SSO-логина. Собственные клиенты, парсившие `?token=`, переходят на exchange (см. [`openapi-full.yaml`](../../api/openapi-full.yaml)).

### Исходящие URL (политика SSRF)

В production (`STORAGE_DEV` не задан или false) server-initiated HTTP — только **публичный HTTPS**. Plain `http://`, loopback и RFC1918 отклоняются, пока явно не ослабите политику.

Для локального Loki на `http://localhost:3100` оставьте `STORAGE_DEV=true` или задайте `STORAGE_OUTBOUND_HTTP_ALLOW=true` (временный escape hatch — см. [планируемый sunset](#planned-deprecation-storage_outbound_http_allow) ниже). Overlay `docker-compose.audit.yml` ослабляет outbound и поднимает лимит login для feature-audit — только dev/CI, не production.

### Planned deprecation: `STORAGE_OUTBOUND_HTTP_ALLOW`

| Релиз | Изменение |
|-------|-----------|
| **v1.0.2** | Строгая политика исходящих URL (`internal/security/urlpolicy`); escape hatch через `STORAGE_OUTBOUND_HTTP_ALLOW=true` |
| **v1.0.3** | Зафиксирован timeline sunset (этот раздел); в production по умолчанию `false` |
| **v1.1.0** | **`STORAGE_OUTBOUND_HTTP_ALLOW` удаляется** — `STORAGE_DEV=true` только на non-production стеках, либо публичные **HTTPS** endpoint'ы |

**До v1.1.0:** проверьте compose, Helm и `.env` на `STORAGE_OUTBOUND_HTTP_ALLOW=true`. Где возможно — переведите интеграции на HTTPS; для локального Loki/Elasticsearch используйте `STORAGE_DEV=true` только в dev/CI overlay (`docker-compose.audit.yml`, не production).

**Чеклист production:** не задавайте `STORAGE_OUTBOUND_HTTP_ALLOW` (или оставьте `false`); sinks и webhooks — `https://` на публичные хосты; после обновления — `GET /api/v1/settings/security-status`.

### Rate limit на login

По умолчанию **10** попыток login с IP за минуту (`STORAGE_RATE_LIMIT_LOGIN`, окно `STORAGE_RATE_LIMIT_WINDOW`, по умолчанию `1m`). CI и нагрузочные скрипты могут получить 429 — поднимите лимит в test overlay (`docker-compose.audit.yml`) или добавьте backoff в автоматизацию.

### Новые и изменённые переменные окружения

Сверьте `.env.example` и смените всё, что осталось на dev-дефолтах:

| Переменная | Default (prod) | Заметка оператору |
|------------|----------------|-------------------|
| `STORAGE_OUTBOUND_HTTP_ALLOW` | `false` | Разрешить non-HTTPS outbound (dev/Loki) |
| `STORAGE_OIDC_ROPC_ENABLED` | `false` | Resource-owner password grant; только для test IdP |
| `STORAGE_LDAP_REQUIRE_TLS` | `true` | Отклоняет `ldap://` в настройках LDAP |
| `STORAGE_MFA_ENCRYPTION_KEY` | (fallback на JWT secret) | Отдельный ключ шифрования MFA |
| `STORAGE_CORS_ALLOWED_ORIGINS` | (пусто) | Origins браузера для консоли, через запятую |
| `STORAGE_RATE_LIMIT_LOGIN` | `10` | Попыток auth с IP за окно |
| `STORAGE_RATE_LIMIT_WINDOW` | `1m` | Окно sliding window для login |
| `STORAGE_STRICT_SECRETS` | `false` | При `true` — отказ старта при дефолтных секретах |

После обновления: `GET /api/v1/settings/security-status` (admin JWT) покажет слабые env vars.

### Шаги обновления

```bash
git pull
export TAG=v1.0.3   # или сборка из исходников
docker compose --profile postgres pull   # при образах GHCR
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # или pull datasafe-console:v1.0.3
docker compose --profile postgres up -d
```

Проверьте cosign с `TAG=v1.0.3` (ниже), затем smoke-test: локальный login, OIDC (если есть), один outbound (webhook или log sink).

## Обновление до v1.0.3

Релиз **v1.0.3** — trust-and-quality patch (CI smoke, Postgres FK regression, SSRF-тесты, опциональный Vault ops pattern). **Field encryption** — новая **opt-in** возможность; по умолчанию поведение как v1.0.2.

### Field encryption (опционально)

| Тема | Действие |
|------|----------|
| По умолчанию | `STORAGE_FIELD_ENCRYPTION_ENABLED=false` — при апгрейде ничего не менять |
| Postgres | Миграция `012_field_encryption` автоматически (таблица `encryption_key_registry`) |
| Включение | KEK → env → restart; см. [field-encryption.md](field-encryption.md) |
| Проверка | `GET /api/v1/settings/security-status` → блок `field_encryption`; Admin → Settings → Security |
| Vault | Agent может инжектить `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` как bootstrap-секреты ([secrets-vault.md](secrets-vault.md)) |

**Community Edition:** KEK из env/файла, без license gate. Vault Transit / HSM — Enterprise phase 2.

Скрипты: [scripts/crypto/README.md](../../../scripts/crypto/README.md).

### Переменные v1.0.3 (field encryption)

| Переменная | Default | Заметка |
|------------|---------|---------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | `false` | `true` — шифрование выбранных секретов метаданных |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | (не задан) | Обязателен при enabled; совпадение с active в registry |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | (не задан) | Base64 raw X25519 private seed |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | (не задан) | JSON map для ротации |

### Шаги обновления (v1.0.3)

```bash
git pull
export TAG=v1.0.3
docker compose --profile postgres pull   # при образах GHCR
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # или pull datasafe-console:v1.0.3
docker compose --profile postgres up -d
```

Проверьте cosign с `TAG=v1.0.3`, затем `GET /api/v1/settings/security-status` и Admin → Settings → Security. Field encryption — только по чеклисту [field-encryption.md](field-encryption.md).

## Чеклист

- [ ] Backup метаданных и объектов
- [ ] Проверить changelog / миграции
- [ ] Тест на staging
- [ ] Пересобрать консоль при изменении UI: `scripts\build-console.cmd`
- [ ] v1.0.2: обновить server **и** console вместе для OIDC
- [ ] v1.0.2: проверить outbound URL и `STORAGE_RATE_LIMIT_LOGIN` для автоматизации
- [ ] v1.0.3 (опционально): включить [field encryption](field-encryption.md) — KEK, env, security-status
