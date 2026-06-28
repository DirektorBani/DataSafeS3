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
TAG=v1.0.2
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

Для локального Loki на `http://localhost:3100` оставьте `STORAGE_DEV=true` или задайте `STORAGE_OUTBOUND_HTTP_ALLOW=true` (временно — пересмотрите до v1.1.0). Overlay `docker-compose.audit.yml` ослабляет outbound и поднимает лимит login для feature-audit — только dev/CI, не production.

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
export TAG=v1.0.2   # или сборка из исходников
docker compose --profile postgres pull   # при образах GHCR
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # или pull datasafe-console:v1.0.2
docker compose --profile postgres up -d
```

Проверьте cosign с `TAG=v1.0.2` (ниже), затем smoke-test: локальный login, OIDC (если есть), один outbound (webhook или log sink).

## Чеклист

- [ ] Backup метаданных и объектов
- [ ] Проверить changelog / миграции
- [ ] Тест на staging
- [ ] Пересобрать консоль при изменении UI: `scripts\build-console.cmd`
- [ ] v1.0.2: обновить server **и** console вместе для OIDC
- [ ] v1.0.2: проверить outbound URL и `STORAGE_RATE_LIMIT_LOGIN` для автоматизации
