**[English](../../en/api/swagger.md)** | Русский

# Swagger UI — Community Integration API

**Автор:** Трачук Илья · **Обновлено:** 2026-06-28

## Что такое Swagger UI (и чем не является)

| Swagger UI **это** | Swagger UI **не это** |
|--------------------|------------------------|
| Интерактивный обозреватель **Community Integration API** | Панель администратора или замена веб-консоли |
| Живая документация из `openapi.json` | Полный перечень всех маршрутов `/api/v1/*` |
| Try-it-out для интеграторов с токенами `ds_*` | Форма входа с JWT администратора |
| Экспортируемый OpenAPI 3.1 для Postman / Insomnia | Документация S3 XML API (SigV4) |

Все функции текущей **Community**-редакции описаны в этой спеке. **Admin-only** маршруты (пользователи, системные настройки, webhooks, tenants, gateway, federation) намеренно исключены из Swagger — используйте **веб-консоль** или [`openapi-full.yaml`](../../api/openapi-full.yaml).

## URL

| Ресурс | URL (локально) |
|--------|----------------|
| **Swagger UI** | http://localhost:8080/api/v1/docs |
| **OpenAPI JSON** | http://localhost:8080/api/v1/openapi.json |
| **OpenAPI YAML** | http://localhost:8080/api/v1/openapi.yaml |
| **Файл в репозитории** | [docs/api/openapi.yaml](../../api/openapi.yaml) |

Порт **8080** — origin консоли (Caddy). Те же пути доступны напрямую на **9000** через storage-server.

Ресурсы Swagger UI **встроены в `storage-server`** (`internal/openapi/swagger-ui-dist/`), поэтому `/api/v1/docs` работает без CDN. Caddy ослабляет Content-Security-Policy для маршрута документации, чтобы скрипты и стили стабильно загружались в браузере.

## Аутентификация

Интеграции используют **API-токены** с префиксом `ds_`, а не JWT администратора.

### 1. Bootstrap (человек, один раз)

1. Откройте веб-консоль → войдите под своей учётной записью.
2. **Access → API tokens → Create**.
3. Имя, срок, scopes → **скопируйте токен сразу** (показывается один раз).

Путь в консоли: **Access → API tokens → Create** — колонки: Name, Created, Expires, Scopes.

### 2. Authorize в Swagger UI

1. Откройте `/api/v1/docs`.
2. **Authorize** (иконка замка, справа сверху).
3. Введите `ds_ваш_токен` (Swagger добавит префикс `Bearer`).
4. Авторизация сохраняется в сессии браузера.

В диалоге Authorize достаточно вставить сам токен — префикс `Bearer` вводить не нужно.

### 3. Запросы к защищённым endpoint

```http
GET /api/v1/me HTTP/1.1
Host: localhost:8080
Authorization: Bearer ds_xxxxxxxx
```

Публичные endpoint (`GET /health`, метаданные/скачивание share) **без** токена — в спеке `security: []`.

## Безопасность

| Рекомендация | Зачем |
|--------------|-------|
| Не публикуйте токены в чатах и тикетах | Полный доступ от имени пользователя |
| Ротация токенов | Ограничение ущерба при утечке |
| Минимальные scopes | Меньше прав у автоматизации |
| HTTPS в production | Защита токена в transit |
| Не коммитьте токены в git | Secret store / CI secrets |

## Community Integration API vs Admin API

- **Community Integration API (эта спека):** бакеты, объекты, ключи, presign, usage, shares, tokens, search, trash и т.д. — Swagger UI `/api/v1/docs`.
- **Admin-only маршруты** (пользователи, системные настройки, webhooks, tenants, gateway, federation): входят в **Community** self-hosted; описаны в [`openapi-full.yaml`](../../api/openapi-full.yaml), управление через консоль или полную спеку — **не** в Swagger UI.
- **S3 XML API:** AWS SigV4 на порту 9000 — AWS SDK; вне OpenAPI.

## Эндпоинты безопасности v1.0.2+ (full spec)

Swagger UI намеренно описывает только **Integration API** (без `/auth/*` и admin settings). Маршруты безопасности — в [`openapi-full.yaml`](../../api/openapi-full.yaml):

| Метод | Путь | С | Назначение |
|-------|------|---|------------|
| `POST` | `/auth/oidc/exchange` | v1.0.2 | Обмен `exchange_code` на JWT сессии (вместо `?token=` в URL) |
| `GET` | `/settings/security-status` | v1.0.2 | Диагностика: `weak_secrets` |
| `GET` | `/settings/security-status` | v1.0.3 | Тот же маршрут; в ответе блок `field_encryption` |

Для pre-flight после обновления — full spec или curl. SSO через exchange при server и console v1.0.2+. **v1.0.3:** Admin → Settings → **Security** дублирует posture из `security-status`.

## Экспорт в Postman / Insomnia

1. Скачайте `GET /api/v1/openapi.json`.
2. **Postman:** Import → URL или файл.
3. **Insomnia:** Import → From URL.
4. Auth коллекции: **Bearer Token** → `ds_*`.

## Регенерация, lint, drift

См. [docs/api/README.md](../../api/README.md):

```cmd
go run tools/gen-openapi-yaml.go
go test ./internal/api/... -run OpenAPI -count=1
powershell -File scripts\openapi-drift-check.ps1
powershell -File scripts\lint-openapi.ps1
```

После изменений спеки пересоберите `storage-server`.

## Ограничения

| Ограничение | Альтернатива |
|-------------|--------------|
| S3 XML API не в OpenAPI | AWS SDK + access keys на **9000** — [руководство §3](../user-guide/README.md#3-ключи-доступа-api-токены-и-квоты) |
| Admin-маршруты не описаны | Раздел **Администрирование** консоли |
| OIDC redirect | Вход через консоль; для автоматизации — токены |
| `/metrics` | Prometheus scrape |

## См. также

- [Руководство — REST API и OpenAPI](../user-guide/README.md#rest-api-и-openapi)
- [Roadmap OpenAPI](../context/openapi-roadmap.md)
