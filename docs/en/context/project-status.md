English | **[Русский](../../ru/context/project-status.md)**

# Project Status

**Last updated:** 2026-06-28 · **Current release:** v1.0.1 (tag)

## Summary

DataSafeS3 **Community Edition v1.0.1** is the current public release (patch after v1.0.0): S3-compatible API, web console (EN/RU/DE/FR), PostgreSQL/Bolt metadata, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway replication, federation MVP, HA tooling, and supply-chain artifacts (GHCR images, SBOM, cosign).

Patch **v1.0.1** shipped supply-chain hygiene (console SBOM, GitHub Release artifacts), documentation sync, and AUD-08 bucket list error UX � no new product capabilities.

## Feature maturity (CE)

| Area | Status | Notes |
|------|--------|-------|
| S3 API (SigV4, multipart, versioning, presign) | **Shipped** | Port 9000 |
| Web console + Admin JSON API | **Shipped** | Caddy :8080 |
| PostgreSQL metadata + read replica routing | **Shipped** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Shipped** | OIDC issuer unreachable → in-console warning (AUD-09) |
| MFA / WebAuthn | **Shipped** | TOTP + passkeys |
| Object Lock (WORM) | **Shipped** | XML API + console |
| Gateway replication | **Shipped** | External S3 target |
| Federation | **Partial (MVP)** | GetObject + ListObjectsV2 proxy |
| HA (Postgres failover scripts, read-only standby) | **Partial** | Manual promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab MVP** | Not production multi-AZ |
| Supply chain (SBOM + cosign) | **Shipped** | Both images on release tags (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Shipped** | Community Integration API scope |
| File collaboration (phases 1–3) | **Shipped** | Home bucket, grants, share links |

## Test gates (last verified)

| Gate | Result | When |
|------|--------|------|
| `go test ./...` | PASS | 2026-06-28 v1.0.1 campaign |
| Feature-audit | 101 PASS / 2 SKIP | 2026-06-28 regression |
| Playwright E2E | 6/6 PASS | 2026-06-28 (6 specs) |

## Documentation

- Bilingual guides: 35 EN = 35 RU markdown files under `docs/`.
- Roadmap audit items: [roadmap.md](./roadmap.md) — AUD-08/09 closed in v1.0.1 scope.
- Architecture: [architecture.md](./architecture.md) · [competitiveness roadmap](../../specs/roadmap/README.md).

## Out of scope for CE (planned 1.1.0+)

Mobile (Flutter/PWA), Kafka event sink, automatic failover orchestrator, production erasure tier, full de/fr documentation, CI publish on every `main` push.

---

[Documentation index](../README.md) · [Roadmap](./roadmap.md) · [Architecture](./architecture.md)
