**[English](../../testing/oidc-e2e.md)** | Русский

# OIDC Keycloak E2E (nightly / опционально)

Реальный browser SSO с тестовым realm Keycloak **не** является PR gate. Запуск по расписанию и вручную при доступном Keycloak.

## Когда запускается

| Триггер | Workflow | Spec |
|---------|----------|------|
| PR / CI `main` | [ci.yml](../../../.github/workflows/ci.yml) | 7 regression specs — **без** OIDC Keycloak |
| Nightly (пн 03:00 UTC) + manual | [e2e-oidc.yml](../../../.github/workflows/e2e-oidc.yml) | `e2e/security-oidc-keycloak.spec.ts` |

## Локальный запуск

1. Keycloak: `scripts\start-keycloak-test.cmd`.
2. Stack на `http://127.0.0.1:8080` с настроенным OIDC.
3. `E2E_OIDC_KEYCLOAK=1`, `KEYCLOAK_URL=http://127.0.0.1:8180`, затем `npx playwright test e2e/security-oidc-keycloak.spec.ts`.

Spec с тегом `@nightly` **пропускается** без `E2E_OIDC_KEYCLOAK=1`.

## Политика по флейкам

- **Не блокирует merge** — зависимость от тайминга Keycloak, redirect и cookies.
- **Повторы:** re-run nightly вручную; не добавлять Keycloak в PR CI без стабильного IdP fixture.
- **Падения:** разбор в nightly; исправлять до релиза, но не откатывать несвязанные PR только из‑за optional OIDC red.
