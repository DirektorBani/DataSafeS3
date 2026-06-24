# Changelog

All notable changes to DataSafeS3 are documented in this file.

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

[1.0.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.0
