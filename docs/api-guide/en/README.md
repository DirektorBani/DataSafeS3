English | **[Русский](../ru/README.md)**

# API guide

DataSafeS3 exposes two APIs from `storage-server`:

| API | Base | Auth |
|-----|------|------|
| **Admin JSON** | `/api/v1/` | JWT Bearer (console login or API token `ds_*`) |
| **S3 XML** | `/` | AWS Signature Version 4 |

## Quick links

- [Authentication](authentication.md)
- [curl examples](curl-examples.md)
- [Swagger UI](http://localhost:8080/api/v1/docs) (Integration API)
- [OpenAPI community spec](../../api/openapi.yaml)
- [OpenAPI full Admin spec](../../api/openapi-full.yaml)
- [Swagger guide](../../en/api/swagger.md)

## Health (no auth)

```http
GET /api/v1/health
GET /metrics
```
