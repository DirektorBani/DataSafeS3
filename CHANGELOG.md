# Changelog

All notable changes to DataSafeS3 are documented in this file.

## [1.0.1] - 2026-06-28

Patch release: supply-chain hygiene, documentation sync, and minor UX fixes. No new user-facing capabilities.

### Added

- SBOM (Syft CycloneDX) for **both** release images (`storage-server` and `console`).
- GitHub Release job attaches SBOM artifacts and generates release notes on version tags.
- Operator guide: `cosign verify` steps for GHCR images (EN/RU — SECURITY.md, getting-started, operations guide).

### Changed

- Docker Compose / `.env.example` / Helm examples default to `v1.0.1` GHCR tags.
- `SECURITY.md` — real security contact (`trachyk.i@gmail.com`) and GitHub Security Advisories.
- Project status and roadmap: AUD-09 (OIDC issuer unreachable) marked **done**; AUD-08 bucket list error surfacing.
- Compose project name normalization (`datasafe`) and expanded `.gitignore` (from post-v1.0.0 maintenance).
- Swagger guide: placeholder screenshots replaced with text-only steps.

### Fixed

- **AUD-08** — Buckets page shows API errors instead of a silent empty list; bucket create returns 409 only for name conflicts (500 for server/metadata errors).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.1`, `ghcr.io/direktorbani/datasafe-console:v1.0.1`.

## [1.0.0] - 2026-06-24

First public **DataSafeS3 Community Edition** release.

### Highlights

- **S3-compatible API** — buckets, objects, multipart, versioning, presigned URLs, STS session tokens, Object Lock (WORM), storage classes, and gateway replication.
- **Web console** — object browser, admin settings, tenants and groups, MFA/WebAuthn, OIDC/LDAP SSO, setup wizard, EN/RU i18n.
- **Metadata** — Bolt (dev) and PostgreSQL (production); tenant-scoped buckets and RBAC.
- **HA & operations** — Docker Compose and Helm charts, Postgres failover automation, read-replica routing, federation sync, Prometheus/Grafana dashboards.
- **Collaboration** — shared links, file collaboration (Phases 1–3), audit and webhooks.
- **Security & supply chain** — cosign image signing, govulncheck in CI, OpenAPI 3.1 spec and drift tests.
- **Documentation** — bilingual user/admin guides, API guides, and deployment cookbooks.

Container images: `ghcr.io/direktorbani/datasafe-storage-server:v1.0.0`, `ghcr.io/direktorbani/datasafe-console:v1.0.0`.

[1.0.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.1
[1.0.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.0
