English | **[Русский](../../ru/api/openapi-full.md)**

# Full REST API — OpenAPI 3.1

Complete machine-readable specification for **all** JSON routes under `/api/v1` (excluding S3 XML on port 9000).

| Artifact | Location |
|----------|----------|
| **OpenAPI YAML** | [docs/api/openapi-full.yaml](../../api/openapi-full.yaml) |
| **Community subset** (Swagger UI) | [openapi.yaml](../../api/openapi.yaml) · `GET /api/v1/docs` |
| **Generator** | `go run tools/gen-openapi-yaml.go` |

## Two specs, one codebase

| Spec | Audience | Auth | Served live |
|------|----------|------|-------------|
| **Community Integration API** | Integrators, scripts | `ds_*` tokens | `GET /api/v1/openapi.json` |
| **Full Admin API** | Operators, auditors, codegen | `ds_*` + JWT | Repository file only |

S3 XML API (SigV4) is documented in the [user guide](../user-guide/README.md) — not in OpenAPI.

## OpenAPI best practices used

- **OpenAPI 3.1.0** with `info.contact`, `info.license`, `summary`
- **`operationId`** on every operation (stable for codegen)
- **Reusable components**: `parameters`, `schemas`, `responses`, `examples`
- **Security schemes**: `BearerAPIToken`, `BearerJWT` (full spec)
- **Standard errors**: `400`, `401`, `403`, `404` via `$ref`
- **Tags** grouped by domain (Buckets, Admin, Gateway, Tenants, …)
- **Drift tests**: `go test ./internal/api/... -run OpenAPI`
- **Spectral lint**: `powershell -File scripts/lint-openapi.ps1`

## Regenerate after route changes

```cmd
go run tools/gen-openapi-yaml.go
go test ./internal/api/... -run OpenAPI -count=1
powershell -File scripts\openapi-drift-check.ps1
powershell -File scripts\lint-openapi.ps1
```

Edit route inventory in `tools/gen-openapi-yaml.go` (`operations` slice), not YAML by hand.

## Import to Postman / Insomnia

1. Use `docs/api/openapi-full.yaml` (or Community `openapi.yaml`).
2. Set collection auth: **Bearer Token** → `ds_...` or JWT from login.

## See also

- [Swagger UI guide](swagger.md)
- [OpenAPI roadmap](../context/openapi-roadmap.md)
- [Scope proposal](../../testing/openapi-scope-proposal.md)
