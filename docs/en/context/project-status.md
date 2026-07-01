English | **[Русский](../../ru/context/project-status.md)**

# Project Status

**Last updated:** 2026-06-30 · **Current release:** [v1.0.3](https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.3)

## Summary

DataSafeS3 **Community Edition v1.0.3** is the current release: S3-compatible API, web console (EN/RU/DE/FR), PostgreSQL/Bolt metadata, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway replication, federation MVP, HA tooling, and supply-chain artifacts (GHCR images, SBOM, cosign).

**v1.0.3** adds **opt-in metadata field encryption** (CE, no license gate), optional **Vault Agent env injection** ops pattern, CI/Postgres regression hardening, and a **Security** panel in admin Settings. Default installs behave like v1.0.2 until field encryption is enabled.

Prior patch **v1.0.2** (security): SSRF outbound policy, OIDC `exchange_code` flow, login rate limits, `security-status` API for weak secrets.

## Feature maturity (CE)

| Area | Status | Notes |
|------|--------|-------|
| S3 API (SigV4, multipart, versioning, presign) | **Shipped** | Port 9000 |
| Web console + Admin JSON API | **Shipped** | Caddy :8080 |
| PostgreSQL metadata + read replica routing | **Shipped** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Shipped** | OIDC exchange flow (v1.0.2+); issuer unreachable warning (AUD-09) |
| MFA / WebAuthn | **Shipped** | TOTP + passkeys |
| Object Lock (WORM) | **Shipped** | XML API + console |
| Gateway replication | **Shipped** | External S3 target |
| Federation | **Partial (MVP)** | GetObject + ListObjectsV2 proxy |
| HA (Postgres failover scripts, read-only standby) | **Partial** | Manual promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab MVP** | Not production multi-AZ |
| Supply chain (SBOM + cosign) | **Shipped** | Both images on release tags (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Shipped** | Community Integration API scope |
| File collaboration (phases 1–3) | **Shipped** | Home bucket, grants, share links, desktop sync |
| Security hardening (v1.0.2+) | **Shipped** | SSRF policy, rate limits, security-status API |
| Metadata field encryption (v1.0.3) | **Shipped (opt-in)** | `STORAGE_FIELD_ENCRYPTION_*`, migration `012` — [field-encryption.md](../operations-guide/en/field-encryption.md) |
| Vault secrets injection (v1.0.3) | **Shipped (ops)** | Agent sidecar → `STORAGE_*` env — [secrets-vault.md](../operations-guide/en/secrets-vault.md) |

## Test gates (last verified)

| Gate | Result | When |
|------|--------|------|
| `go test ./...` | PASS | 2026-06-30 v1.0.3 campaign |
| Feature-audit | PASS | 2026-06-30 regression |
| Playwright e2e-smoke | PASS | CI `smoke.spec.ts` on Postgres profile |
| Postgres FK integration | PASS | `TestNullableFK_team_id` with `TEST_POSTGRES_DSN` |

## Documentation

- Bilingual guides under `docs/`; **v1.0.3** upgrade (EN/RU), field encryption, Vault injection, CHANGELOG — 2026-06-30.
- Roadmap audit items: [roadmap.md](./roadmap.md).
- Architecture: [architecture.md](./architecture.md).

## Out of scope for CE (planned 1.1.0+)

Removal of `STORAGE_OUTBOUND_HTTP_ALLOW` escape hatch (scheduled v1.1.0), mobile (Flutter/PWA), Kafka event sink, automatic failover orchestrator, production erasure tier, Vault Transit in-process KEK (Enterprise phase 2).

---

[Documentation index](../README.md) · [Roadmap](./roadmap.md) · [CHANGELOG](../../../CHANGELOG.md)
