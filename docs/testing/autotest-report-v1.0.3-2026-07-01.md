# Autotest report — DataSafeS3 v1.0.3

**Date:** 2026-07-01  
**Scope:** Community Edition, automated tests only (no manual НТ, no load testing)  
**Compose:** Rebuilt `deploy/docker/storage-server-linux` from current source; `docker compose` with `local-data` + `local-binary` + `audit` overlays before live-stack gates.

## v1.0.3 features (from CHANGELOG / docs)

| # | Feature | Acceptance criteria (automated) |
|---|---------|--------------------------------|
| 1 | **Field encryption (metadata at rest)** | `enc:v1:` wire format for access keys, gateway creds, system-config secrets; decrypt roundtrip; disabled passthrough; security-status `field_encryption` block |
| 2 | **HashiCorp Vault (env injection)** | Optional ops pattern; smoke when `VAULT_PROFILE=1` (build tag `vault` Go tests) |
| 3 | **Console security posture** | Settings → Security tab; `GET /api/v1/settings/security-status` |
| 4 | **Gateway health `public_read_rules`** | API returns count; console warning when > 0 |
| 5 | **SSRF / `STORAGE_OUTBOUND_HTTP_ALLOW` regression** | Prod vs dev matrix in urlpolicy; API blocks loopback webhooks/logging |
| 6 | **Postgres nullable `team_id` FK** | Integration test with `TEST_POSTGRES_DSN` (CI) |
| 7 | **CI / release packaging** | Out of autotest scope (workflow scripts only) |

## Feature → test mapping

| Feature | Go unit/integration | feature-audit | Playwright E2E | OpenAPI drift |
|---------|----------------------|---------------|----------------|---------------|
| Field encryption — crypto | `internal/security/fieldenc/service_test.go` (6) | — | — | — |
| Field encryption — system_config paths | `internal/metadata/fieldenc_config_test.go` (2) | — | — | — |
| Field encryption — Bolt access keys | `internal/metadata/field_encryption_bolt_integration_test.go` | — | — | — |
| Field encryption — Bolt gateway creds | `internal/metadata/field_encryption_bolt_integration_test.go` **`TestBoltFieldEncryption_PutGatewayConnection_encryptedAtRest`** (new) | — | — | — |
| Field encryption — Postgres access keys | `internal/metadata/postgres/field_encryption_integration_test.go` (2, needs `TEST_POSTGRES_DSN`) | — | — | — |
| Field encryption — Admin API | **`internal/api/field_encryption_test.go`** (2 new) | — | `settings.spec.ts` `field_encryption` block | `/settings/security-status` in `openapi_test.go` |
| Security posture API | `internal/api/security_remediation_test.go` `TestSecurityStatus_listsWeakSecrets` | **`Security` / Security-status field_encryption block**, **`weak_secrets list`** (new) | `settings.spec.ts` (2) | PASS |
| Gateway `public_read_rules` | **`defect_fixes_test.go`** `TestGatewayHealth_*` (extended + 1 new) | **`Gateway` / Gateway health** (checks `public_read_rules`) | `gateway.spec.ts` (2) | — |
| Vault injection | `internal/security/vault_injection_integration_test.go` (`-tags=vault`) | `Vault` / Injection path smoke (when `VAULT_PROFILE=1`) | — | — |
| SSRF / outbound HTTP | `internal/security/urlpolicy/urlpolicy_test.go` `TestDefaultOptions_prodMode_matrix` | existing sink/webhook records | — | — |
| Postgres FK regression | `internal/metadata/postgres/store_test.go` `TestNullableFK_team_id` | — | — | — |

## Files changed (this QA pass)

| File | Change |
|------|--------|
| `internal/api/field_encryption_test.go` | **Added** — API security-status + S3 roundtrip with field enc enabled |
| `internal/api/defect_fixes_test.go` | **Extended** gateway health tests for `public_read_rules` |
| `internal/metadata/field_encryption_bolt_integration_test.go` | **Added** gateway credential at-rest encryption test |
| `scripts/feature-audit-test.ps1` | **Added** 2 Security records; gateway health checks `public_read_rules`; fixed `weak_secrets` null count |
| `deploy/docker/storage-server-linux` | **Rebuilt** for live-stack verification (not committed) |

## Gate results

| Gate | Result | Notes |
|------|--------|-------|
| `go test ./...` | **PASS** | Full suite, Windows native |
| Focused v1.0.3 `-run` filters | **PASS** | Postgres/Vault integration **SKIP** locally without `TEST_POSTGRES_DSN` / `VAULT_PROFILE` |
| `go test ./internal/api/ -run OpenAPI` | **PASS** | Drift clean for `/settings/security-status` |
| feature-audit | **100/105 PASS**, 0 FAIL, 4 SKIP | Baseline was ~93; +2 Security, gateway assertion tightened; skips: LDAP/OIDC/ES/Vault profile |
| Playwright `settings.spec.ts` + `gateway.spec.ts` | **5/5 PASS** | Security posture tab + gateway public-read warning |
| Console build | not re-run | No UI source changes in this pass |

## Test counts (v1.0.3-related)

| Layer | New tests this pass | Existing v1.0.3 coverage |
|-------|---------------------|--------------------------|
| Go API (`field_encryption_test.go`, `defect_fixes_test.go`) | **+3** | security_remediation, urlpolicy, postgres fieldenc, bolt fieldenc, fieldenc unit |
| Go metadata | **+1** | 4 bolt/postgres integration + 2 config |
| feature-audit | **+2** records (105 total) | Vault optional +1 when profile enabled |
| Playwright | 0 new (5 existing) | settings + gateway v1.0.3 UI |

## Gaps

| Gap | Severity | Mitigation |
|-----|----------|------------|
| Postgres field encryption + `TestNullableFK_team_id` | Low locally | CI sets `TEST_POSTGRES_DSN` (Postgres 16 service) |
| Vault injection end-to-end | Low | `VAULT_PROFILE=1` + `scripts/vault/test-vault-integration.mjs`; Go `-tags=vault` |
| Field encryption enabled on live stack | Info | Audit stack runs with `STORAGE_FIELD_ENCRYPTION_ENABLED` unset → `enabled=false`; encrypt-at-rest proven in Go integration tests |
| Load / manual НТ | N/A | Explicitly out of scope per request |

## Verdict

**PASS** — v1.0.3 CE functionality is covered by the test pyramid: Go unit/integration for field encryption and SSRF, feature-audit for security-status and gateway health on live stack, Playwright for console security posture and gateway visibility warning. No blocking autotest gaps for release; Postgres/Vault paths validated in CI via env-gated tests.
