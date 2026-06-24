English | **[Русский](../../ru/api/swagger.md)**

# Swagger UI — Community Integration API

**Author:** Ilya Trachuk · **Last updated:** 2026-06-22

## What Swagger UI is (and is not)

| Swagger UI **is** | Swagger UI **is not** |
|-------------------|------------------------|
| Interactive explorer for the **Community Integration API** | Admin panel or web console replacement |
| Live documentation from `openapi.json` | Full surface of every `/api/v1/*` route |
| Try-it-out for integrators with `ds_*` tokens | Login form for administrator JWT |
| Exportable OpenAPI 3.1 for Postman / Insomnia | Documentation for S3 XML API (SigV4) |

All features shipped today in DataSafeS3 Community are covered by this spec. **Admin-only** routes (users, system settings, webhooks, tenants, gateway, federation) are intentionally excluded from Swagger — use the **web console** or [`openapi-full.yaml`](../../api/openapi-full.yaml).

## URLs

| Resource | URL (local default) |
|----------|---------------------|
| **Swagger UI** | http://localhost:8080/api/v1/docs |
| **OpenAPI JSON** | http://localhost:8080/api/v1/openapi.json |
| **OpenAPI YAML** | http://localhost:8080/api/v1/openapi.yaml |
| **Spec in repo** | [docs/api/openapi.yaml](../../api/openapi.yaml) |

Port **8080** is the console origin (Caddy). You can also hit storage-server directly on **9000** with the same paths.

Swagger UI assets are **bundled inside `storage-server`** (`internal/openapi/swagger-ui-dist/`) so `/api/v1/docs` works without a CDN. Caddy applies a relaxed Content-Security-Policy on the docs route so scripts and styles load reliably in the browser.

## Authentication workflow

Integrations use **API tokens** with prefix `ds_`, not administrator JWT.

### 1. Bootstrap (human, one-time)

1. Open the web console → sign in with your user account.
2. Go to **Access → API tokens → Create**.
3. Set name, expiry, and scopes → **copy the token immediately** (shown once).

![Console Access → API tokens — placeholder for screenshot]

### 2. Authorize in Swagger UI

1. Open `/api/v1/docs`.
2. Click **Authorize**.
3. Enter `ds_your_token_here` (Swagger adds the `Bearer` prefix).
4. Authorization persists in the browser session (`persistAuthorization: true`).

![Swagger Authorize dialog — placeholder for screenshot]

### 3. Call protected endpoints

```http
GET /api/v1/me HTTP/1.1
Host: localhost:8080
Authorization: Bearer ds_xxxxxxxx
```

Public endpoints (`GET /health`, public share metadata/download) require **no** token — they show `security: []` in the spec.

## Security best practices

| Practice | Why |
|----------|-----|
| Never paste tokens in public chats, tickets, or screenshots | Tokens grant full API access as your user |
| Rotate tokens on schedule or after team changes | Limits blast radius of a leak |
| Use least-privilege scopes when creating tokens | Restrict what automation can do |
| Prefer HTTPS in production | Protects tokens in transit |
| Do not commit tokens to git or CI logs | Use secret stores instead |

## Community Integration API vs Admin API

- **Community Integration API (this spec):** buckets, objects, keys, presign, usage, shares, tokens, search, trash, etc. — served at Swagger UI `/api/v1/docs`.
- **Admin-only routes** (users, system settings, webhooks, tenants, gateway, federation): shipped in **Community** self-hosted builds; documented in [`openapi-full.yaml`](../../api/openapi-full.yaml), managed via web console or full spec — **not** in Swagger UI.
- **S3 XML API:** AWS SigV4 on port 9000 — use AWS SDKs; not in OpenAPI.

## Export for Postman / Insomnia

1. Download `GET /api/v1/openapi.json`.
2. **Postman:** Import → Link or file → paste URL or upload JSON.
3. **Insomnia:** Application → Import → From URL → `http://localhost:8080/api/v1/openapi.json`.
4. Set collection auth to **Bearer Token** and paste your `ds_*` value.

## Regenerating, linting, drift check

See [docs/api/README.md](../../api/README.md):

```cmd
go run tools/gen-openapi-yaml.go
go test ./internal/api/... -run OpenAPI -count=1
powershell -File scripts\openapi-drift-check.ps1
powershell -File scripts\lint-openapi.ps1
```

Rebuild `storage-server` after spec changes so the embedded copy updates.

## Limitations

| Limitation | Alternative |
|------------|-------------|
| S3 XML API not in OpenAPI | AWS SDK + access keys on port **9000** — see [User guide §3](../user-guide/README.md#3-access-keys-api-tokens-and-quotas) |
| Admin routes not documented | Web console **Administrator** section |
| OIDC browser redirect flow | Human sign-in via console; tokens for automation |
| Prometheus `/metrics` | Direct scrape, not JSON REST |

## Related docs

- [User guide — REST API & OpenAPI](../user-guide/README.md#rest-api--openapi)
- [OpenAPI roadmap](../context/openapi-roadmap.md)
- [Scope proposal](../../testing/openapi-scope-proposal.md)
