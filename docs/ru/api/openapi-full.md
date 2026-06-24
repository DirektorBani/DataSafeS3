**[English](../../en/api/openapi-full.md)** | Русский

# Полный REST API — OpenAPI 3.1

Полная спецификация **всех** JSON-маршрутов `/api/v1` (без S3 XML на порту 9000).

| Артефакт | Путь |
|----------|------|
| **OpenAPI YAML** | [docs/api/openapi-full.yaml](../../api/openapi-full.yaml) |
| **Community subset** (Swagger UI) | [openapi.yaml](../../api/openapi.yaml) · `GET /api/v1/docs` |
| **Генератор** | `go run tools/gen-openapi-yaml.go` |

## Две спеки, один код

| Спека | Аудитория | Auth | В рантайме |
|-------|-----------|------|------------|
| **Community Integration API** | Интеграторы | токены `ds_*` | `GET /api/v1/openapi.json` |
| **Full Admin API** | Операторы, аудит | `ds_*` + JWT | только файл в репозитории |

S3 XML API (SigV4) — в [руководстве пользователя](../user-guide/README.md), не в OpenAPI.

## Best practices

- OpenAPI **3.1.0**, contact, license, summary
- Уникальный **operationId** на каждую операцию
- Переиспользуемые **components** (schemas, responses, parameters)
- Схемы безопасности **BearerAPIToken** и **BearerJWT** (полная спека)
- Стандартные ответы **400/401/403/404**
- Теги по доменам (Buckets, Admin, Gateway, Tenants, …)
- Тесты дрейфа: `go test ./internal/api/... -run OpenAPI`
- Lint: `scripts/lint-openapi.ps1` (Spectral)

## Регенерация

```cmd
go run tools/gen-openapi-yaml.go
go test ./internal/api/... -run OpenAPI -count=1
powershell -File scripts\openapi-drift-check.ps1
```

Меняйте маршруты в `tools/gen-openapi-yaml.go`, не в YAML вручную.

## См. также

- [Swagger UI](swagger.md)
- [Roadmap OpenAPI](../context/openapi-roadmap.md)
