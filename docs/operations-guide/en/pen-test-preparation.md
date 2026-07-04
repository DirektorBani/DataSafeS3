English | **[Русский](../ru/pen-test-preparation.md)**

# Penetration test preparation (internal)

**Not a certification.** Checklist for engaging a third-party assessor or running an internal red-team exercise against DataSafeS3 CE.

## Scope document

| Item | Include |
|------|---------|
| **Version** | Tagged release (e.g. v1.1.0) + console image digest |
| **Surface** | `storage-server` :9000 (S3 + Admin API), console :8080, optional Postgres |
| **Out of scope** | Keycloak/LDAP test containers unless explicitly in scope |
| **Rules** | No production data; use dedicated lab VPC / compose stack |

## Environment baseline

1. Deploy from [upgrade guide](upgrade.md) with **non-default secrets** (`STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, `STORAGE_ADMIN_PASSWORD`).
2. Set `STORAGE_METRICS_TOKEN` and verify `/metrics` returns 401 without bearer.
3. Confirm `STORAGE_OUTBOUND_HTTP_ALLOW` is **not** set (removed v1.1.0); production path uses HTTPS outbound only.
4. Run `GET /api/v1/settings/security-status` — zero `weak_secrets`.
5. Attach [security self-assessment](security-self-assessment.md) and latest feature-audit summary.

## Test accounts

| Role | Purpose |
|------|---------|
| `administrator` | Full admin API + console |
| `user` | RBAC negative tests |
| `tenant_admin` / `member` / `viewer` | Tenant grant matrix (AUD-15) |
| OIDC `ssouser` | SSO path (optional) |

Provide credentials in a sealed channel; rotate after assessment.

## Priority attack scenarios

1. **Auth** — JWT forgery, OIDC exchange replay, MFA bypass, login rate limit evasion
2. **SSRF** — log sinks, webhooks, hook test URLs (private IP, metadata 169.254.169.254)
3. **IDOR** — cross-tenant bucket/object access, share tokens
4. **S3** — path traversal in object keys, policy bypass, presigned URL tampering
5. **Info disclosure** — unauthenticated `/metrics`, security-status leakage
6. **Supply chain** — cosign verify release images, SBOM review

## Evidence to collect

- `go test ./...` and feature-audit PASS output
- Playwright CI job link (6 specs)
- CHANGELOG security section for target version
- Network diagram (Caddy → storage-server → Postgres)

## Post-assessment

1. Triage findings in GitHub Security Advisories private draft.
2. Map to `docs/en/context/roadmap.md` AUD-* items.
3. Update [SECURITY.md](../../../SECURITY.md) only after coordinated disclosure.

## Related

- [SECURITY.md](../../../SECURITY.md)
- [v1.1.0 TZ](../../specs/v1.1.0-trust-debt-oss-growth-tz.md)
- [Field encryption](field-encryption.md)
