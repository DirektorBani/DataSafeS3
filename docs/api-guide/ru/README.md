**[English](../en/README.md)** | Русский

# Руководство по API

DataSafeS3 предоставляет два API из `storage-server`:

| API | Базовый URL | Аутентификация |
|-----|-------------|----------------|
| **Admin JSON** | `/api/v1/` | JWT Bearer (вход в консоль или API token `ds_*`) |
| **S3 XML** | `/` | AWS Signature Version 4 |

## Быстрые ссылки

- [Аутентификация](authentication.md)
- [Примеры curl](curl-examples.md)
- **Примеры кода:** [Go (S3 SDK)](examples/go/list_buckets.go) · [Python (Admin JWT)](../en/examples/python/admin_list_buckets.py)
- [Swagger UI](http://localhost:8080/api/v1/docs) (Integration API)
- [OpenAPI community](../../api/openapi.yaml)
- [OpenAPI full Admin](../../api/openapi-full.yaml)
- [Swagger guide](../../ru/api/swagger.md)

## Проверка работоспособности (без аутентификации)

```http
GET /api/v1/health
GET /metrics
```
