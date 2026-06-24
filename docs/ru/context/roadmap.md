**[English](../../en/context/roadmap.md)** | Русский

# Датасейф S3 Roadmap

Author: **Трачук Илья** | Last updated: 2026-06-22

Status legend: **done** | **partial** | **in-progress** | **planned**

> **Поставка 2026-06:** фазы 1–4 в основном реализованы (`20b8518` → `6299610`). GHCR+SBOM+cosign, WebAuthn, Object Lock XML, LDAP sync worker, federation ListObjectsV2 proxy, Postgres HA tooling, STS (привязка к пользователю), NATS, nightly feature-audit CI, встроенный Swagger UI. См. [дорожную карту конкурентоспособности](../../specs/roadmap/README.md).

> Blocks 16–25 are available to **all licensed self-hosted deployments** (not admin-only or enterprise-gated). Navigation exposes features by role; administrators have full access.

---

## Feature Audit Improvements (2026-06-17)

Prioritized backlog from [feature audit 2026-06-17](../../testing/feature-audit-report.md). Source: internal improvement suggestions derived from audit.

### P0 — Critical / Reliability

| ID | Category | Item | Effort | Status | Notes |
|----|----------|------|--------|--------|-------|
| AUD-01 | Ops | Rebuild and publish `storage-server` Docker image after each release; CI build/push on `main` | M | planned | Local build OK with `ensure-docker-pull-proxy.cmd`; binary overlay documented |
| AUD-02 | Ops | Document Docker registry proxy (`scripts/ensure-docker-pull-proxy.cmd`, `127.0.0.1:10801`) or auto-disable when proxy down | S | **done** | README §Локальная разработка; proxy wait loop in script |
| AUD-03 | Testing | Consistent nullable FK handling — audit all postgres INSERT/UPDATE for empty-string vs `NULL` | M | partial | `team_id` fix **done**; add integration test with `TEST_POSTGRES_DSN` in CI |

### P1 — UX / API

| ID | Category | Item | Effort | Status | Notes |
|----|----------|------|--------|--------|-------|
| AUD-04 | Testing | External logging audit coverage — delivery checks, not just PUT 200 | M | **done** | ES `_search`, Loki; audit script 93 checks |
| AUD-05 | UX | Folder delete API — accept `prefix` as query param on DELETE (in addition to JSON body) | S | **done** | Query `?prefix=&recursive=true` |
| AUD-06 | UX | Bucket create response — always include `visibility` in POST response | S | done | In latest source; ensure deployed image |
| AUD-07 | Integrations | Share link API — document `expires_in_sec` (not `expires_in`) in OpenAPI/user guide | S | **done** | Audit script + user guide |
| OPENAPI-01 | Integrations | OpenAPI 3.1 spec + live `/api/v1/openapi.json` + Swagger UI | M | **done** | [openapi-roadmap.md](openapi-roadmap.md), `docs/api/openapi.yaml` |
| AUD-08 | UX | User bucket list empty state — clear 409/500 when create fails (e.g. FK error) | S | planned | Avoid silent empty list |
| AUD-09 | UX | OIDC login button — in-console status when Keycloak issuer unreachable (browser vs server) | M | planned | Better SSO diagnostics |
| AUD-23 | UX | Elasticsearch sink `token` field — UI placeholder "API key (base64)" not "bearer token" | S | **done** | Basic auth username/password + API key in Admin Settings |

### P2 — Operations / Monitoring

| ID | Category | Item | Effort | Status | Notes |
|----|----------|------|--------|--------|-------|
| AUD-10 | Ops | Grafana `datasafe-overview` — pre-provision Prometheus scrape on fresh install; import JSON in repo | M | partial | Prometheus scrapes `storage-server:9000`; metric `datasafe_http_requests_total` verified in audit |
| AUD-11 | Ops | Re-enable storage-server Docker healthcheck (wget/curl on `/healthz`) | S | **done** | `docker-compose.yml` wget healthcheck |
| AUD-12 | UX | Gateway replication visibility — UI indicator when remote S3 bucket policy adjusted for public-read | S | planned | Server-side logic exists |
| AUD-13 | Integrations | LDAP sync on login — document first LDAP login may require admin sync if `sync_on_login` off | S | **done** | User guide §7; audit enables `sync_on_login` |

### P3 — Nice to Have

| ID | Category | Item | Effort | Status | Notes |
|----|----------|------|--------|--------|-------|
| AUD-14 | Testing | Automated OIDC E2E — Playwright against Keycloak test container | L | partial | `scripts/oidc-browser-e2e.mjs`; API path in audit script |
| AUD-15 | Testing | Tenant viewer/member E2E in `feature-audit-test.ps1` | M | planned | Currently Go tests only |
| AUD-16 | UX | Recursive folder delete confirmation — mirror API `object_count` on conflict in UI | S | planned | Explicit confirm dialog |
| AUD-17 | Security | MFA admin enforcement — guide admin through enroll on first login (`mfa_setup_required`) | M | planned | When `require_admin_mfa` on |
| AUD-18 | Testing | Trash restore E2E — upload→delete→restore cycle in audit script | S | planned | Currently lists trash only |
| AUD-19 | Testing | Versioning / object versions UI test in audit script | S | planned | API exists |
| AUD-20 | Testing | Webhook delivery retry — verify delivery log + retry under load | M | planned | Audit only created config |
| AUD-21 | Ops | Cross-platform binary smoke test — Linux binary in `alpine:3.20` before publish | M | planned | Catch mount/startup issues |
| AUD-22 | Ops | Log sink delivery errors — surface failed HTTP responses; optional admin diagnostics via `EmitTestRecord` | M | planned | Errors swallowed in async goroutines |

### Audit fixes already merged

| Item | Status | Notes |
|------|--------|-------|
| Loki sink nanosecond timestamp | **done** | `log_sinks.go`; verified with `datasafe-log-loki` |
| Elasticsearch ApiKey auth (`token` field) | **done** | `Authorization: ApiKey`, not Bearer |
| `team_id` nullable FK on bucket create | **done** | List buckets RBAC fixed |
| External logging multi-sink fan-out | **done** | Syslog, Loki, ES 8.11, Webhook verified on latest source |
| Tenant-scoped bucket names + grants | **done** | [TZ](../specs/tenant-bucket-isolation-tz.md); `(tenant_id, name)` uniqueness; tenant admin Access tab |
| Elasticsearch basic auth in settings | **done** | Username/password in Admin Settings; `validateLoggingConfig` |

---

## Architecture Strengthening (CE TZ)

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| A.1 | Dual metadata backend | **done** | `STORAGE_METADATA_BACKEND=bolt\|postgres`; `metadata.Open(cfg)` |
| A.2 | PostgreSQL metadata store | **done** | pgx/v5, embedded migrations, full MetadataStore |
| A.3 | migrate-boltdb command | **done** | `storage-server migrate-boltdb` with integrity report |
| A.4 | Performance review doc | **done** | [performance-review.md](performance-review.md) |
| A.5 | STS AssumeRole | **done** | `POST /api/v1/sts/assume-role`; session tokens в SigV4; credentials привязаны к аутентифицированному пользователю |
| A.6 | Event notifications | **partial** | Webhook + **NATS** (`STORAGE_NATS_URL`); Kafka/RabbitMQ в планах |
| A.7 | SSE-S3 | **partial** | AES-256-GCM через `STORAGE_SSE_MASTER_KEY` |
| A.8 | Cluster health probe | **done** | Live `/healthz` каждые 10s; метрики кластера |
| A.9 | Object Lock API subset | **done** | Retention GET/PUT XML; governance/compliance modes |
| A.10 | Postgres trigram search | **done** | pg_trgm when postgres backend |
| A.11 | Extended metrics | **done** | objects, versions, replication queue, webhooks, multipart |

---

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 1.1 | Bucket Versioning | **partial** | S3 PUT/GET `?versioning`; delete markers; console Versions tab |
| 1.2 | Object Browser | **done** | Folders, breadcrumbs, drag-drop, bulk ops, metadata sidebar |
| 1.3 | Lifecycle Rules UI | **done** | Bucket Lifecycle tab + S3 XML |
| 1.4 | Presigned URLs / Share | **done** | `POST /api/v1/presign` |
| 1.5 | Bucket Quotas per user | **done** | User/bucket quotas; net delta on overwrite |

---

## Priority 2

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 2.6 | Корзина (Soft Delete) | **done** | `.datasafe-trash`; restore/purge; 7/30/90d auto-purge |
| 2.7 | Bucket Policies UI | **done** | JSON + visual builder |
| 2.8 | Audit Log | **partial** | Download + policy events; filters |
| 2.9 | API Tokens | **done** | Console token CRUD (`ds_*`) |
| 2.10 | Webhooks | **done** | Delivery log + retry |

---

## Priority 3

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 3.11 | Multipart Upload UI | **done** | Chunked upload ≥64MB |
| 3.12 | Search Engine | **done** | Global search bar |
| 3.13 | Tags | **done** | Bucket/object tags |
| 3.14 | Object Metadata UI | **done** | Content-Type, custom metadata |
| 3.15 | Favorites | **done** | Pin buckets/folders |

---

## Platform Roadmap (16–25)

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 16 | LDAP / Active Directory | **partial** | Config + test + sync + **scheduled sync worker** + login + tenant group sync E2E |
| 17 | OIDC / SSO | **partial** | Generic OIDC; issuer check; callback + Keycloak groups claim |
| 18 | MFA (TOTP + WebAuthn) | **done** | Enroll/verify/recovery; WebAuthn passkeys; optional admin MFA policy |
| 19 | Immutable Buckets (WORM) | **done** | Object Lock XML; retention presets; delete blocked until expiry |
| 20 | Legal Hold | **done** | Object flag; blocks delete; UI toggle |
| 21 | DataSafeS3 Gateway | **done** | Async queue worker; PUT/DELETE/COPY events; health metrics |
| 22 | DataSafeS3 Federation | **partial** | Cluster registry; **GetObject + ListObjectsV2** prefix proxy; sync worker |
| 23 | Horizontal scaling | **partial** | HA compose/Helm, failover scripts, erasure 2+1 MVP; no auto orchestrator |
| 24 | Storage Classes | **partial** | Hot/Warm/Cold metadata; **transition API** между классами |
| 25 | Multi-Tenant | **partial** | Tenant CRUD; bucket visibility by owner_id + team_id |

### Role matrix (navigation & API)

| Section / API | administrator | tenant_admin | operator | user |
|---------------|---------------|--------------|----------|------|
| Profile / MFA | yes | yes | yes | yes |
| Buckets (own + team + tenant) | all | tenant scope | all | own + team + tenant |
| Tenants (members, Access tab) | all tenants | managed tenants only | no | no |
| Federation / Cluster | yes | no | no | no |
| Gateway | yes | no | no | no |
| Users / Settings / Activity | yes | no | no | no |
| LDAP / OIDC / Cluster config | yes (Settings) | no | no | no |

---

## Verification

```cmd
go test ./...
scripts\build-console.cmd
scripts\dev-docker-local-binary.cmd
```

Console: Profile; Federation + Cluster (admin); Gateway, Tenants (admin); Settings LDAP/OIDC/MFA/Cluster; object legal hold in metadata panel.

---

## Summary counts (Feature Audit backlog)

| Priority | Total | done | partial | planned |
|----------|-------|------|---------|---------|
| P0 | 3 | 1 | 1 | 1 |
| P1 | 7 | 4 | 1 | 2 |
| P2 | 4 | 2 | 1 | 1 |
| P3 | 9 | 0 | 1 | 8 |
| **Audit fixes (merged)** | 4 | 4 | 0 | 0 |
| **Total audit items** | **23** | **5** | **4** | **14** |
