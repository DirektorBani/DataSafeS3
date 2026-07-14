English | **[Русский](../../ru/context/project-status.md)**

# Project Status

**Last updated:** 2026-07-14 · **Current release:** [v1.2.0](https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.2.0)

## Summary

DataSafeS3 **Community Edition v1.2.0** is the current release: **immutable backup golden path** (Object Lock `retention_mode` + versioning Suspend in console, EN/RU use-case), AUD-16 folder `object_count` UX, and prior v1.1.1 MinIO migration kit. Core platform: S3-compatible API, web console (EN/RU/DE/FR), PostgreSQL/Bolt metadata, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway replication, federation MVP, Teams, metrics-token protection, share-token hashing, and HA v2 lab tooling.

**v1.1.0** burns down trust-debt C01-C21 and ships **HA v2 CE lab foundation**: erasure backend, Postgres leader lock, site replication, and local validation scripts. This HA scope is explicitly **lab / not production multi-AZ**: no automatic failover orchestrator, no Patroni-certified production cluster, and no petabyte 4+2 multi-host promise.

Prior patch **v1.0.3**: opt-in metadata field encryption, Vault Agent env injection ops pattern, CI/Postgres regression hardening, and a **Security** panel in admin Settings.

## Feature maturity (CE)

| Area | Status | Notes |
|------|--------|-------|
| S3 API (SigV4, multipart, versioning, presign) | **Shipped** | Port 9000 |
| Web console + Admin JSON API | **Shipped** | Caddy :8080 |
| PostgreSQL metadata + read replica routing | **Shipped** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Shipped** | OIDC exchange flow (v1.0.2+); issuer unreachable warning (AUD-09) |
| MFA / WebAuthn | **Shipped** | TOTP + passkeys |
| Object Lock (WORM) | **Shipped** | XML API + console; `retention_mode` GOVERNANCE/COMPLIANCE (v1.2.0) |
| Gateway replication | **Shipped** | External S3 target |
| Federation | **Partial (MVP)** | GetObject + ListObjectsV2 proxy |
| Teams admin API + console | **Shipped** | Admin → Teams, user assignment, OpenAPI full spec |
| Metrics bearer token | **Shipped** | `STORAGE_METRICS_TOKEN` protects `/metrics` when set |
| HA v2 lab foundation | **Shipped (lab)** | Erasure, leader lock, site replication; not production multi-AZ |
| HA (Postgres failover scripts, read-only standby) | **Partial** | Manual promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab foundation** | `STORAGE_OBJECT_BACKEND=erasure`; production multi-AZ remains future hardening |
| Supply chain (SBOM + cosign) | **Shipped** | Both images on release tags (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Shipped** | Community Integration API scope |
| File collaboration (phases 1–3) | **Shipped** | Home bucket, grants, share links, desktop sync |
| Security hardening (v1.0.2+) | **Shipped** | SSRF policy, rate limits, security-status API |
| Metadata field encryption (v1.0.3) | **Shipped (opt-in)** | `STORAGE_FIELD_ENCRYPTION_*`, migration `012` — [field-encryption.md](../operations-guide/en/field-encryption.md) |
| Vault secrets injection (v1.0.3) | **Shipped (ops)** | Agent sidecar → `STORAGE_*` env — [secrets-vault.md](../operations-guide/en/secrets-vault.md) |

## Test gates (last verified)

| Gate | Result | When |
|------|--------|------|
| `go test ./...` | PASS | 2026-07-04 release prep |
| Feature-audit | PASS | 112 PASS / 0 FAIL / 1 SKIP, 2026-07-04 |
| Playwright 7 specs | PASS | 9 tests across 7 spec files, 2026-07-04 |
| HA lab Option B | PASS | `run-all-ha-tests.ps1 -FreshVolumes -SkipBuild`, 0 FAIL |

## Documentation

- Bilingual guides under `docs/`; **v1.1.0** upgrade (EN/RU), HA lab docs, pen-test prep, CHANGELOG — 2026-07-04.
- Roadmap audit items: [roadmap.md](./roadmap.md).
- Architecture: [architecture.md](./architecture.md).

## Out of scope for CE (future)

Mobile (Flutter/PWA), Kafka event sink, automatic failover orchestrator, production multi-AZ erasure tier, Vault Transit in-process KEK (Enterprise phase 2).

## v1.1.0 shipped scope

Charter: trust-debt burn-down + OSS growth — internal TZ under `D:\datasafe_tz\specs\v1.1.0\` (see [docs/specs/README.md](../specs/README.md)).

| Item | Status |
|------|--------|
| Remove `STORAGE_OUTBOUND_HTTP_ALLOW` | **Shipped** |
| `STORAGE_METRICS_TOKEN` on `/metrics` | **Shipped** |
| Upgrade guide EN/RU + CHANGELOG | **Shipped** |
| Playwright CI ≥6 specs (P1) | **Done** |
| Teams admin REST + UI (P1) | **Done** |
| OIDC Keycloak E2E nightly (P1) | **Done** |
| MFA admin wizard AUD-17 (P1) | **Done** |
| Feature-audit AUD-15/18 (P1) | **Done** |
| CONTRIBUTING + ref-arch script (P1) | **Done** |
| Pen-test prep doc (P1) | **Done** |
| Grafana panel smoke AUD-10 (P2) | **Done** |
| SDK examples + GHCR main publish (P2) | **Done** |
| de/fr getting-started stubs (P2) | **Done** |
| Share link hash-only C21 (P2) | **Done** |
| OpenAPI teams routes (C06-7) | **Done** |
| HA v2 CE lab foundation | **Shipped (lab)** |

---

[Documentation index](../README.md) · [Roadmap](./roadmap.md) · [CHANGELOG](../../../CHANGELOG.md)
