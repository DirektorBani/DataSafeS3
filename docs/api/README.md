# DataSafeS3 OpenAPI — Community Integration API

Hand-written OpenAPI **3.1** specifications for the JSON REST API (`/api/v1/*`).

| Artifact | Path |
|----------|------|
| **Community spec** (Swagger UI, integrators) | [`openapi.yaml`](openapi.yaml) |
| **Full Admin API** (all routes, operators) | [`openapi-full.yaml`](openapi-full.yaml) |
| Embedded copy (served by server) | [`internal/openapi/openapi.yaml`](../../internal/openapi/openapi.yaml) |
| Live JSON | `GET /api/v1/openapi.json` |
| Live YAML | `GET /api/v1/openapi.yaml` |
| Swagger UI | [`/api/v1/docs`](http://localhost:8080/api/v1/docs) |
| Guides (EN/RU) | [docs/en/api/swagger.md](../en/api/swagger.md) · [openapi-full.md](../en/api/openapi-full.md) |
| Spectral rules | [`.spectral.yaml`](../../.spectral.yaml) |

**Not in OpenAPI:** S3 XML API (SigV4 on port 9000) — use AWS SDKs.

## Regenerate both specs

After editing `tools/gen-openapi-yaml.go` (`operations` inventory):

```cmd
go run tools/gen-openapi-yaml.go
```

Writes `openapi.yaml`, `internal/openapi/openapi.yaml`, and `openapi-full.yaml`.

Rebuild and restart `storage-server`:

```cmd
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o deploy\docker\storage-server-linux .\cmd\storage-server
docker compose up -d storage-server --no-deps
```

## Drift check

Verifies: no admin paths in spec, community paths are subset of `server.go`, embedded copy matches docs:

```cmd
go test ./internal/api/... -run OpenAPI -count=1
powershell -File scripts\openapi-drift-check.ps1
```

## Lint (Spectral)

```cmd
powershell -File scripts\lint-openapi.ps1
powershell -File scripts\lint-openapi.ps1 -SpecPath docs/api/openapi-full.yaml
```

## Authentication

| Client | Auth |
|--------|------|
| Integrations / Swagger Try-it-out | `Authorization: Bearer ds_...` (API token from console) |
| Human bootstrap | Sign in to web console → Access → API tokens → Create |

JWT from admin login is **not** documented in OpenAPI — use console for human sessions, tokens for automation.

## Scope (Community)

| Included | Excluded |
|----------|----------|
| health, me, buckets/objects, multipart, keys, presign | `/admin/*`, `/users`, `/settings/system`, webhooks |
| usage, shares, public share, tokens | tenants, gateway, federation, cluster, LDAP admin |
| search, favorites, trash, tags, lifecycle (user-scoped) | bucket policy (admin), `/metrics`, S3 XML |

See [openapi-roadmap.md](../en/context/openapi-roadmap.md) for scope and maintenance.
