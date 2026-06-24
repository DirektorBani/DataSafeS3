English | **[Русский](../../ru/context/openapi-roadmap.md)**

# OpenAPI Roadmap

Author: **Ilya Trachuk** | Last updated: 2026-06-19

Status legend: **done** | **partial** | **planned**

## Overview

OpenAPI documents the **Community Integration API** — the semver-stable JSON REST subset at `/api/v1/*` for `ds_*` integrators.

**Excluded from Swagger:** Admin/console-only routes (see `openapi-full.yaml`), S3 XML API (AWS SDK on port 9000).

| Deliverable | Status | Notes |
|-------------|--------|-------|
| Community spec `docs/api/openapi.yaml` (3.1) | **done** | ~51 operations, no admin paths |
| `GET /api/v1/openapi.json` | **done** | Embedded YAML → JSON |
| Swagger UI `/api/v1/docs` | **done** | Community only, `persistAuthorization` |
| BearerAPIToken security | **done** | No JWT/admin login in spec |
| Drift check (subset of server.go) | **done** | `go test -run OpenAPI` |
| Swagger guide EN/RU | **done** | [docs/en/api/swagger.md](../api/swagger.md) |
| Console link | **done** | Administrator settings → Community API (Swagger) |

## Phases

### Community Integration API (**done** 2026-06-19)

health, me, buckets/objects, multipart, keys, presign, usage, shares, tokens, search, favorites, trash, tags, lifecycle (user-scoped).

**Audience:** `ds_*` integrators, automation, third-party apps.

**Auth:** API token only in OpenAPI; human bootstrap via web console.

### Admin / full REST API (**openapi-full.yaml**)

Users, system settings, webhooks, activity, bucket policy, **tenants, gateway, federation** — all shipped in Community self-hosted. Documented in [`docs/api/openapi-full.yaml`](../../api/openapi-full.yaml); use web console or the full spec. Not published in Swagger UI by design.

### S3 API (**planned** separate doc)

SigV4 + XML — poor OpenAPI fit; AWS SDK compatibility guide instead.

### Tooling

| Approach | Decision | Status |
|----------|----------|--------|
| Hand-written community spec | **Yes** | **done** |
| `tools/gen-openapi-yaml.go` filter | **Yes** | **done** |
| CI Spectral on PR | partial | Script ready |
| SDK generation | **Future** | After community schema stability |

## Maintenance

1. Add community route in `internal/api/server.go`
2. Update `tools/gen-openapi-yaml.go` (ensure `isCommunityOp` includes it)
3. `go run tools/gen-openapi-yaml.go`
4. `go test ./internal/api/... -run OpenAPI`
5. Rebuild `storage-server`

## References

- [Swagger guide](../api/swagger.md)
- [docs/api/README.md](../../api/README.md)
- Scope proposal: [docs/testing/openapi-scope-proposal.md](../../testing/openapi-scope-proposal.md)
