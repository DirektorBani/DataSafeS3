# Security self-assessment (lightweight)

English | **[Русский](../ru/security-self-assessment.md)**

Internal summary for enterprise reviewers — **not** a third-party penetration test certificate.

## Scope

DataSafeS3 Community Edition `storage-server` + console, Apache-2.0, single-tenant typical deploy.

## Controls implemented

| Area | Status | Evidence |
|------|--------|----------|
| Authentication | JWT admin, SigV4 S3, WebAuthn/TOTP MFA | feature-audit B6–B10 |
| Authorization | RBAC, bucket policies, tenant grants | feature-audit C12–C16 |
| Audit trail | Activity log, share audit events | Admin → Activity |
| Transport | TLS via ingress/Caddy (operator-provided) | deployment docs |
| Secrets | Env / K8s secrets, no keys in image layers | Helm secrets |
| Supply chain | SBOM + Cosign on release tags | `.github/workflows/release.yml` |
| Vulnerability scanning | `govulncheck` in CI | `.github/workflows/ci.yml` |
| Disclosure process | SECURITY.md | repository root |

## HA / DR (Community — full)

| Control | Status |
|---------|--------|
| Postgres streaming replication docs + scripts | `scripts/postgres-failover.*`, `scripts/dr-drill.ps1` |
| Read-only storage-server standby | `STORAGE_READ_ONLY`, `docker-compose.ha.yml` |
| Replication lag alerting | Grafana `PostgresReplicationLagHigh` |

## Residual risks

- Manual failover (no K8s operator auto-failover)
- Erasure coding MVP is lab-scale, not petabyte parity
- STS scoped credentials — user-bound session tokens; not full IAM role federation

## Recommended external review

Annual third-party penetration test on reference 2-node deployment before customer-facing «enterprise ready» claims.
