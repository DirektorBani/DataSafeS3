**[Русский](../../ru/testing/oidc-e2e.md)** | English

# OIDC Keycloak E2E (nightly / optional)

A real browser SSO against a test Keycloak realm is not a pull-request gate. It runs on a schedule and by hand when Keycloak is up.

## When it runs

| Trigger | Workflow | Spec |
|---------|----------|------|
| PR / CI on `main` | [ci.yml](../../../.github/workflows/ci.yml) | regression specs, without OIDC Keycloak |
| Nightly (Monday 03:00 UTC) and manual | [e2e-oidc.yml](../../../.github/workflows/e2e-oidc.yml) | `e2e/security-oidc-keycloak.spec.ts` |

## Local run

1. Keycloak: `scripts\start-keycloak-test.cmd`.
2. Stack on `http://127.0.0.1:8080` with OIDC configured.
3. `E2E_OIDC_KEYCLOAK=1`, `KEYCLOAK_URL=http://127.0.0.1:8180`, then `npx playwright test e2e/security-oidc-keycloak.spec.ts`.

A spec tagged `@nightly` is skipped unless `E2E_OIDC_KEYCLOAK=1`.

## Flakes

- It does not block a merge. Timing depends on Keycloak, the redirect, and cookies.
- Re-run the nightly job by hand. Do not add Keycloak to pull-request CI without a stable identity fixture.
- Investigate failures on the nightly run and fix them before a release. Do not revert an unrelated pull request only because this optional check is red.
