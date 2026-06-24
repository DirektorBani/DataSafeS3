English | **[Русский](../ru/oidc.md)**

# OIDC / SSO

Single sign-on via OpenID Connect (Keycloak and compatible IdPs).

## Configuration

**Admin → Settings → System → OIDC**

- Issuer URL, client ID, client secret
- Redirect URI: `http://your-host:8080/login`
- Optional: password grant for legacy apps

## Login flow

```mermaid
sequenceDiagram
  participant U as User browser
  participant C as Console
  participant IdP as Keycloak/OIDC
  U->>C: Click SSO login
  C->>IdP: Authorization redirect
  IdP-->>U: Login + consent
  IdP-->>C: Callback with code
  C->>C: Exchange for JWT session
```

## Test realm

Sample Keycloak realm: [docs/integrations/keycloak-test/](../../integrations/keycloak-test/)

## Full guide

[User guide — LDAP and SSO](../../en/user-guide/README.md#7-ldap-and-sso-keycloak)
