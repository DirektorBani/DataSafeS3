# Changelog

All notable changes to DataSafeS3 are documented in this file.

## [1.0.2] - 2026-06-28

Security patch: SSRF hardening, production secrets guidance, object key validation, OIDC exchange flow, rate limiting, and supply-chain updates.

### Security

- **SSRF** - Outbound URL policy (`internal/security/urlpolicy`) for log sinks, hook tests, webhooks, and bucket notifications (DS-SEC-003, DS-SEC-004, DS-SEC-027).
- **OIDC** - Callback redirects with one-time `exchange_code` instead of JWT in query string; `POST /api/v1/auth/oidc/exchange` (DS-SEC-005).
- **OIDC ROPC** - `STORAGE_OIDC_ROPC_ENABLED` gates `POST /api/v1/auth/oidc/password-login` (DS-SEC-013).
- **Object keys** - `ValidateObjectKey` rejects path traversal and control characters on S3 PUT/GET/DELETE (DS-SEC-024).
- **CORS** - Configurable allowlist via `STORAGE_CORS_ALLOWED_ORIGINS` (DS-SEC-001).
- **Rate limiting** - Login endpoints limited per IP (`STORAGE_RATE_LIMIT_LOGIN`, `STORAGE_RATE_LIMIT_WINDOW`) (DS-SEC-002).
- **MFA** - Optional dedicated `STORAGE_MFA_ENCRYPTION_KEY` with JWT secret fallback (DS-SEC-026).
- **LDAP** - `STORAGE_LDAP_REQUIRE_TLS=true` rejects `ldap://` URLs in settings (DS-SEC-025).
- **Secrets** - Helm `values-production.yaml` hardening template; `GET /api/v1/settings/security-status` lists weak env vars (DS-SEC-006).
- **Go** - Toolchain bumped to Go 1.26.4; `govulncheck` in CI (DS-SEC-007).

### Changed

- Console login handles `?exchange_code=` from OIDC callback (`?token=` deprecated).
- Operator docs (EN/RU): outbound URL policy, LDAP TLS, security hardening checklist, upgrade guide v1.0.2.

### Migration

Upgrading from v1.0.1 requires a **paired** storage-server and console update. OIDC IdP redirect URIs stay the same, but the browser no longer receives a JWT in the URL; the console redeems a one-time code via `POST /api/v1/auth/oidc/exchange`. Review outbound integration URLs (Loki, Elasticsearch, webhooks): production now requires public HTTPS unless `STORAGE_OUTBOUND_HTTP_ALLOW=true` or `STORAGE_DEV=true` for local dev. Login automation may need a higher `STORAGE_RATE_LIMIT_LOGIN` or retry logic (default 10/min per IP). Rotate `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, and `STORAGE_ADMIN_PASSWORD` before production; use `GET /api/v1/settings/security-status` or set `STORAGE_STRICT_SECRETS=true` to fail fast on defaults. See [upgrade guide](docs/operations-guide/en/upgrade.md#upgrading-to-v102) (EN/RU).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.2`, `ghcr.io/direktorbani/datasafe-console:v1.0.2`.

## [1.0.1] - 2026-06-28

Patch release: supply-chain hygiene, documentation sync, and minor UX fixes. No new user-facing capabilities.

### Added

- SBOM (Syft CycloneDX) for **both** release images (`storage-server` and `console`).
- GitHub Release job attaches SBOM artifacts and generates release notes on version tags.
- Operator guide: `cosign verify` steps for GHCR images (EN/RU; SECURITY.md, getting-started, operations guide).

### Changed

- Docker Compose / `.env.example` / Helm examples default to `v1.0.1` GHCR tags.
- `SECURITY.md` - real security contact (`trachyk.i@gmail.com`) and GitHub Security Advisories.
- Project status and roadmap: AUD-09 (OIDC issuer unreachable) marked **done**; AUD-08 bucket list error surfacing.
- Compose project name normalization (`datasafe`) and expanded `.gitignore` (from post-v1.0.0 maintenance).
- Swagger guide: placeholder screenshots replaced with text-only steps.

### Fixed

- **AUD-08** - Buckets page shows API errors instead of a silent empty list; bucket create returns 409 only for name conflicts (500 for server/metadata errors).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.1`, `ghcr.io/direktorbani/datasafe-console:v1.0.1`.

## [1.0.0] - 2026-06-24

First public **DataSafeS3 Community Edition** release.

### Highlights

- **S3-compatible API** - buckets, objects, multipart, versioning, presigned URLs, STS session tokens, Object Lock (WORM), storage classes, and gateway replication.
- **Web console** - object browser, admin settings, tenants and groups, MFA/WebAuthn, OIDC/LDAP SSO, setup wizard, EN/RU i18n.
- **Metadata** - Bolt (dev) and PostgreSQL (production); tenant-scoped buckets and RBAC.
- **HA & operations** - Docker Compose and Helm charts, Postgres failover automation, read-replica routing, federation sync, Prometheus/Grafana dashboards.
- **Collaboration** - shared links, file collaboration (Phases 1-3), audit and webhooks.
- **Security & supply chain** - cosign image signing, govulncheck in CI, OpenAPI 3.1 spec and drift tests.
- **Documentation** - bilingual user/admin guides, API guides, and deployment cookbooks.

Container images: `ghcr.io/direktorbani/datasafe-storage-server:v1.0.0`, `ghcr.io/direktorbani/datasafe-console:v1.0.0`.

[1.0.2]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.2
[1.0.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.1
[1.0.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.0
