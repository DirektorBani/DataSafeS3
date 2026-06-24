**[English](../../en/context/openapi-roadmap.md)** | Русский

# Roadmap OpenAPI

Автор: **Трачук Илья** | Обновлено: 2026-06-19

Легенда: **done** | **partial** | **planned**

## Обзор

OpenAPI описывает **Community Integration API** — стабильное JSON REST подмножество `/api/v1/*` для интеграторов с токенами `ds_*`.

**Вне Swagger:** admin/console-only маршруты (см. `openapi-full.yaml`), S3 XML API (AWS SDK на порту 9000).

| Результат | Статус | Примечания |
|-----------|--------|------------|
| Community-спека `docs/api/openapi.yaml` | **done** | ~51 операция, без admin |
| Swagger UI `/api/v1/docs` | **done** | Только Community, `persistAuthorization` |
| BearerAPIToken | **done** | Без JWT/admin login в спеке |
| Drift check (подмножество server.go) | **done** | `go test -run OpenAPI` |
| Руководство Swagger EN/RU | **done** | [docs/ru/api/swagger.md](../api/swagger.md) |
| Ссылка в консоли | **done** | Community API (Swagger) |

## Фазы

### Community Integration API (**done** 2026-06-19)

health, me, бакеты/объекты, multipart, ключи, presign, usage, shares, tokens, search, favorites, trash, tags, lifecycle.

**Аудитория:** интеграторы `ds_*`, автоматизация.

**Auth:** только API-токен в OpenAPI; bootstrap через веб-консоль.

### Admin / полный REST API (**openapi-full.yaml**)

Пользователи, системные настройки, webhooks, **tenants, gateway, federation** — всё входит в Community self-hosted. Описано в [`docs/api/openapi-full.yaml`](../../api/openapi-full.yaml); консоль или полная спека. В Swagger UI не публикуется.

### S3 API (**planned**)

SigV4 + XML — отдельное руководство совместимости.

## Сопровождение

1. Маршрут Community в `server.go`
2. Обновить `tools/gen-openapi-yaml.go` (`isCommunityOp`)
3. `go run tools/gen-openapi-yaml.go`
4. `go test ./internal/api/... -run OpenAPI`
5. Пересобрать `storage-server`

См. [docs/api/README.md](../../api/README.md), [Swagger guide](../api/swagger.md).
