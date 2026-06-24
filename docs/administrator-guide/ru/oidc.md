**[English](../en/oidc.md)** | Русский

# OIDC / SSO

Единый вход через OpenID Connect (Keycloak и совместимые IdP).

## Настройка

**Admin → Settings → System → OIDC**

- Issuer URL, client ID, client secret
- Redirect URI: `http://your-host:8080/login`
- Опционально: password grant для legacy-приложений

## Поток входа

```mermaid
sequenceDiagram
  participant U as Браузер
  participant C as Консоль
  participant IdP as Keycloak/OIDC
  U->>C: SSO login
  C->>IdP: Authorization redirect
  IdP-->>U: Вход + consent
  IdP-->>C: Callback с code
  C->>C: Обмен на JWT сессию
```

## Тестовый realm

Пример Keycloak: [docs/integrations/keycloak-test/](../../integrations/keycloak-test/)

## Полное руководство

[Руководство пользователя — LDAP и SSO](../../ru/user-guide/README.md)
