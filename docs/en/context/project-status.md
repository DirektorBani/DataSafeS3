English | **[???????](../../ru/context/project-status.md)**

# Project Status

**Last updated:** 2026-06-28 ? **Current release:** v1.0.2 (tagged; security patch)

## Summary

DataSafeS3 **Community Edition v1.0.2** is the current release (security patch after v1.0.1): S3-compatible API, web console (EN/RU/DE/FR), PostgreSQL/Bolt metadata, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway replication, federation MVP, HA tooling, and supply-chain artifacts (GHCR images, SBOM, cosign).

Patch **v1.0.2** closes Community security remediation (SSRF outbound policy, OIDC exchange_code flow, login rate limits, secrets diagnostics) ? no new product capabilities beyond hardening and operator UX.

## Feature maturity (CE)

| Area | Status | Notes |
|------|--------|-------|
| S3 API (SigV4, multipart, versioning, presign) | **Shipped** | Port 9000 |
| Web console + Admin JSON API | **Shipped** | Caddy :8080 |
| PostgreSQL metadata + read replica routing | **Shipped** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Shipped** | OIDC exchange flow (v1.0.2); issuer unreachable warning (AUD-09) |
| MFA / WebAuthn | **Shipped** | TOTP + passkeys |
| Object Lock (WORM) | **Shipped** | XML API + console |
| Gateway replication | **Shipped** | External S3 target |
| Federation | **Partial (MVP)** | GetObject + ListObjectsV2 proxy |
| HA (Postgres failover scripts, read-only standby) | **Partial** | Manual promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab MVP** | Not production multi-AZ |
| Supply chain (SBOM + cosign) | **Shipped** | Both images on release tags (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Shipped** | Community Integration API scope |
| File collaboration (phases 1?3) | **Shipped** | Home bucket, grants, share links |
| Security hardening (v1.0.2) | **Shipped** | SSRF policy, rate limits, security-status API |

## Test gates (last verified)

| Gate | Result | When |
|------|--------|------|
| `go test ./...` | PASS | 2026-06-28 v1.0.2 campaign |
| Feature-audit | 101 PASS / 0 FAIL / 2 SKIP | 2026-06-28 regression |
| Playwright E2E (security) | 2/2 PASS | 2026-06-28 post console build |

## Documentation

- Bilingual guides: 35 EN = 35 RU markdown files under `docs/`.
- v1.0.2 upgrade guide (EN/RU), OpenAPI full spec sync, CHANGELOG migration section ? completed 2026-06-28.
- Roadmap audit items: [roadmap.md](./roadmap.md) ? AUD-08/09 closed in v1.0.1 scope.
- Architecture: [architecture.md](./architecture.md) ? [competitiveness roadmap](../../specs/roadmap/README.md).

## Out of scope for CE (planned 1.1.0+)

Mobile (Flutter/PWA), Kafka event sink, automatic failover orchestrator, production erasure tier, full de/fr documentation, CI publish on every `main` push.

---

[Documentation index](../README.md) ? [Roadmap](./roadmap.md) ? [Architecture](./architecture.md)
