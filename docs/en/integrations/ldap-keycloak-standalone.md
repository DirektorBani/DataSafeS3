English | **[Русский](../../ru/integrations/ldap-keycloak-standalone.md)**

# LDAP and SSO (Keycloak) for DataSafeS3 Testing

Standalone Docker containers **outside** `docker-compose.yml` — for local DataSafeS3 integration only. Data is persisted in named volumes.

> **RF-safe:** images are pulled from public registries; after the first `docker pull`, offline operation is possible. Secrets below are **dev/test only**, not for production.

## What DataSafeS3 Expects

### LDAP (Admin → Settings → LDAP / Active Directory)

| UI field | JSON / env | Description |
|-----------|------------|----------|
| Enable LDAP | `ldap.enabled`, `STORAGE_LDAP_ENABLED` | Enable login and sync |
| URL | `ldap.url`, `STORAGE_LDAP_URL` | `ldap://host:389` or `ldaps://host:636` |
| Bind DN | `ldap.bind_dn`, `STORAGE_LDAP_BIND_DN` | Service account for user search |
| Bind password | `ldap.bind_password`, `STORAGE_LDAP_BIND_PASSWORD` | Bind password |
| Base DN | `ldap.base_dn`, `STORAGE_LDAP_BASE_DN` | Root for user search |
| Group DN | `ldap.group_dn` | Group search base (`groupOfNames` / `member`) — for openldap-test: `ou=groups,dc=datasafe,dc=local` |
| Group attribute | `ldap.group_attr` | For AD: `memberOf` on user record |

Additional API fields: `user_attr` (default `cn`), `group_role_map`, `sync_on_login`, `sync_interval_minutes` (background sync; `0` disables; default **60** when LDAP is enabled).

**User lookup:** `(user_attr=username)`, default `(cn=username)` under `Base DN`.

**API:** `POST /api/v1/settings/ldap/test`, `POST /api/v1/settings/ldap/sync` (admin only). A background worker runs `ldap/sync` on the configured interval when LDAP is enabled.

### OIDC / SSO (Admin → Settings → OIDC / SSO)

| UI field | JSON / env | Description |
|-----------|------------|----------|
| Enable SSO | `oidc.enabled`, `STORAGE_OIDC_ENABLED` | "Sign in with SSO" button on `/login` |
| Issuer URL | `oidc.issuer`, `STORAGE_OIDC_ISSUER` | Public issuer (browser, redirect to Keycloak) |
| Internal issuer URL | `oidc.internal_issuer`, `STORAGE_OIDC_INTERNAL_ISSUER` | Issuer for **server-side** calls from Docker (token exchange, userinfo). Optional — if empty and issuer contains `localhost`, DataSafeS3 in a container substitutes `host.docker.internal` automatically |
| Client ID | `oidc.client_id`, `STORAGE_OIDC_CLIENT_ID` | OAuth2 client id |
| Client secret | `oidc.client_secret`, `STORAGE_OIDC_CLIENT_SECRET` | Confidential client secret |
| Redirect URL | `oidc.redirect_url`, `STORAGE_OIDC_REDIRECT_URL` | DataSafeS3 callback |
| Groups claim | `oidc.groups_claim` | Claim name for groups (default `groups`) |

**Redirect URL for console on port 8080:**

```text
http://localhost:8080/api/v1/auth/oidc/callback
```

**SSO flow:** `/login` → `GET /api/v1/auth/oidc/login` → IdP → callback → JWT in query `?token=…&auth_source=oidc`.

**Automated test (ROPC):** `POST /api/v1/auth/oidc/password-login` with `{"username":"ssouser","password":"password"}` — same group sync path as callback (requires `directAccessGrantsEnabled` on client in Keycloak).

**Keycloak test group:** realm import includes group `datasafe-users` and mapper **Group Membership** → claim `groups`; user `ssouser` is a member. After changing `datasafe-realm.json`, delete volume: `docker volume rm datasafe-keycloak-data` and recreate the container.

**SSO session:** for OIDC users JWT lives **15 minutes**; server stores link to Keycloak access token and validates via introspection (cache ~30 s). Immediately after successful callback introspection is **not called** (~30 s) — otherwise first `/me` request could get 401 and redirect user to `/login`. If session in Keycloak ends (logout in Admin Console, end session), next API request after cache expiry returns **401**, console clears token and redirects to `/login`. **Sign out** button calls `POST /api/v1/admin/logout` and for SSO — redirect to Keycloak `end_session_endpoint`.

---

## 1. LDAP (osixia/openldap)

### Quick Start (script)

```cmd
scripts\start-ldap-test.cmd
```

### One-liner `docker run`

```cmd
docker run -d --name datasafe-ldap-test -p 389:389 -p 636:636 -e LDAP_ORGANISATION=DataSafeS3 -e LDAP_DOMAIN=datasafe.local -e LDAP_ADMIN_PASSWORD=ldapadmin -v datasafe-ldap-data:/var/lib/ldap -v datasafe-ldap-config:/etc/ldap/slapd.d -v "%CD%\docs\integrations\ldap-test\bootstrap.ldif:/container/service/sldif/custom/50-datasafe-users.ldif:ro" osixia/openldap:1.5.0
```

Bootstrap LDIF: `docs/integrations/ldap-test/bootstrap.ldif` — OU `users`, `groups`, users `ldapuser`, `ldapadmin`, group `datasafe-users`.

### Values for DataSafeS3

| Parameter | From host (ldapsearch) | DataSafeS3 in Docker (`storage-server`) |
|----------|----------------------|--------------------------------------|
| URL | `ldap://localhost:389` | `ldap://host.docker.internal:389` |
| Bind DN | `cn=admin,dc=datasafe,dc=local` | same |
| Bind password | `ldapadmin` | same |
| Base DN | `ou=users,dc=datasafe,dc=local` | same |
| User attr | `cn` (default) | same |
| Login filter | `(cn=<username>)` | same |

**Test users:** `ldapuser` / `password`, `ldapadmin` / `password`.

### LDAP Verification

```cmd
docker ps --filter name=datasafe-ldap-test
```

From host (if OpenLDAP client installed):

```cmd
ldapsearch -x -H ldap://localhost:389 -D "cn=admin,dc=datasafe,dc=local" -w ldapadmin -b "ou=users,dc=datasafe,dc=local" "(cn=ldapuser)"
```

Via temporary container:

```cmd
docker run --rm osixia/openldap:1.5.0 ldapsearch -x -H ldap://host.docker.internal:389 -D "cn=admin,dc=datasafe,dc=local" -w ldapadmin -b "ou=users,dc=datasafe,dc=local" "(cn=ldapuser)"
```

---

## 2. SSO — Keycloak (OIDC)

### Quick Start (script)

```cmd
scripts\start-keycloak-test.cmd
```

First Keycloak startup may take 30–60 seconds.

### One-liner `docker run`

```cmd
docker run -d --name datasafe-keycloak-test -p 8180:8080 -e KEYCLOAK_ADMIN=admin -e KEYCLOAK_ADMIN_PASSWORD=admin -v datasafe-keycloak-data:/opt/keycloak/data -v "%CD%\docs\integrations\keycloak-test\datasafe-realm.json:/opt/keycloak/data/import/datasafe-realm.json:ro" -v "%CD%\docs\integrations\keycloak-test\themes:/opt/keycloak/themes:ro" quay.io/keycloak/keycloak:26.0.7 start-dev --import-realm
```

Realm `datasafe`, client `datasafe-console`, user `ssouser` — see `docs/integrations/keycloak-test/datasafe-realm.json`.

### DataSafeS3 Login Theme

Custom theme `datasafe` is mounted from `docs/integrations/keycloak-test/themes/` and set in realm import (`loginTheme: datasafe`). Styles match the console dark palette (`web/console/src/index.css`): background `hsl(222.2 84% 4.9%)`, accent `hsl(217.2 91.2% 59.8%)`, login card max-width 420px like shadcn Card, logo and title **DataSafeS3** (`login/login.ftl` + `resources/css/datasafe.css`).

After changing CSS or `theme.properties`, recreate the container (realm volume is not overwritten automatically):

```cmd
docker stop datasafe-keycloak-test
docker rm datasafe-keycloak-test
scripts\start-keycloak-test.cmd
```

Verify login page (open in browser):

```text
http://localhost:8180/realms/datasafe/protocol/openid-connect/auth?client_id=datasafe-console&redirect_uri=http://localhost:8080/&response_type=code&scope=openid
```

Account console (same login theme): http://localhost:8180/realms/datasafe/account

Theme CSS: `docs/integrations/keycloak-test/themes/datasafe/login/resources/css/datasafe.css`

### Values for DataSafeS3

| Parameter | Browser / curl from host | DataSafeS3 in Docker (server-side) |
|----------|------------------------|-------------------------------|
| Issuer URL (public) | `http://localhost:8180/realms/datasafe` | same (for user redirect) |
| Internal issuer URL | — | `http://host.docker.internal:8180/realms/datasafe` |
| Client ID | `datasafe-console` | same |
| Client secret | `datasafe-console-secret` | same |
| Redirect URL | `http://localhost:8080/api/v1/auth/oidc/callback` | same (URL seen by browser) |

> **Critical for Docker:** `storage-server` exchanges authorization code for token **from the container**, not from the browser. Issuer with `localhost:8180` in Keycloak well-known points token endpoint to `[::1]:8180` — unreachable from the container. Set **Internal issuer** = `http://host.docker.internal:8180/realms/datasafe` or only public issuer with `localhost` — server substitutes `host.docker.internal` automatically (when running in Docker).

**Keycloak Admin Console:** http://localhost:8180/admin — `admin` / `admin`.

**SSO test user:** `ssouser` / `password`.

### OIDC Discovery Verification

```cmd
curl -s http://localhost:8180/realms/datasafe/.well-known/openid-configuration
```

Expect JSON with `issuer`, `authorization_endpoint`, `token_endpoint`.

---

## 3. DataSafeS3 Configuration

1. Start main DataSafeS3 stack (`docker compose up` or local dev).
2. Start LDAP and Keycloak with scripts above.
3. Sign in as **admin** → **Settings**.
4. **LDAP:** fill table above. If `storage-server` in Docker, URL can be `ldap://localhost:389` (auto-rewrite to `host.docker.internal`) or explicitly `ldap://host.docker.internal:389`. Enable LDAP → **Save** → **Test connection** → **Sync users** (optional).
5. **OIDC:** Public issuer `http://localhost:8180/realms/datasafe`, internal issuer `http://host.docker.internal:8180/realms/datasafe`, client id/secret, redirect URL → enable SSO → **Save**.
6. Open `/login` in incognito → **Sign in with SSO** → sign in as `ssouser`.

### Example env for `storage-server` (optional, initial values)

```env
STORAGE_LDAP_ENABLED=true
STORAGE_LDAP_URL=ldap://host.docker.internal:389
STORAGE_LDAP_BIND_DN=cn=admin,dc=datasafe,dc=local
STORAGE_LDAP_BIND_PASSWORD=ldapadmin
STORAGE_LDAP_BASE_DN=ou=users,dc=datasafe,dc=local
# STORAGE_LDAP_GROUP_DN=ou=groups,dc=datasafe,dc=local

STORAGE_OIDC_ENABLED=true
STORAGE_OIDC_ISSUER=http://localhost:8180/realms/datasafe
STORAGE_OIDC_INTERNAL_ISSUER=http://host.docker.internal:8180/realms/datasafe
STORAGE_OIDC_CLIENT_ID=datasafe-console
STORAGE_OIDC_CLIENT_SECRET=datasafe-console-secret
STORAGE_OIDC_REDIRECT_URL=http://localhost:8080/api/v1/auth/oidc/callback
```

UI settings in Bolt override empty env fields on save.

### Test Connection via API

```cmd
curl -s -X POST http://localhost:8080/api/v1/settings/ldap/test ^
  -H "Authorization: Bearer <ADMIN_JWT>" ^
  -H "Content-Type: application/json" ^
  -d "{\"url\":\"ldap://host.docker.internal:389\",\"bind_dn\":\"cn=admin,dc=datasafe,dc=local\",\"bind_password\":\"ldapadmin\",\"base_dn\":\"ou=users,dc=datasafe,dc=local\"}"
```

---

## 4. Shutdown

```cmd
scripts\stop-ldap-keycloak-test.cmd
```

Remove containers **and** volumes (full LDAP/Keycloak reset):

```cmd
scripts\stop-ldap-keycloak-test.cmd --remove
```

---

## 5. Troubleshooting

### `host.docker.internal` from `storage-server` container

DataSafeS3 connects to LDAP and OIDC **from the server**, not from the browser. If the container cannot resolve `host.docker.internal`:

- **Docker Desktop (Windows/macOS):** name usually works out of the box.
- **Linux:** add to `docker-compose.yml` for `storage-server`:
  ```yaml
  extra_hosts:
    - "host.docker.internal:host-gateway"
  ```
- Alternative: host IP in URL (`ldap://172.17.0.1:389`).

Verify from DataSafeS3 container:

```cmd
docker compose exec storage-server wget -qO- http://host.docker.internal:8180/realms/datasafe/.well-known/openid-configuration
```

### LDAP: bootstrap not applied

Script `start-ldap-test.cmd` runs `ldapadd -c` for `bootstrap.ldif` after container start (idempotent). If users are missing manually:

```cmd
docker exec datasafe-ldap-test ldapadd -x -c -D "cn=admin,dc=datasafe,dc=local" -w ldapadmin -f /container/service/sldif/custom/50-datasafe-users.ldif
```

After changing `bootstrap.ldif` on existing volume:

```cmd
scripts\stop-ldap-keycloak-test.cmd --remove
scripts\start-ldap-test.cmd
```

### Keycloak: realm not imported

Similarly — delete volume `datasafe-keycloak-data` and recreate container. Or create realm manually in Admin Console per table above.

### Keycloak: `datasafe` theme not applied

Realm import (`--import-realm`) runs **only on first volume creation**. If container already existed with `datasafe-keycloak-data`, fields like `loginTheme` from `datasafe-realm.json` are not updated automatically.

**Options:**

1. Full reset: `scripts\stop-ldap-keycloak-test.cmd --remove`, then `scripts\start-keycloak-test.cmd`.
2. Manually in Admin Console: Realm **datasafe** → **Themes** → Login theme = `datasafe`.
3. CLI:
   ```cmd
   docker exec datasafe-keycloak-test /opt/keycloak/bin/kcadm.sh config credentials --server http://localhost:8080 --realm master --user admin --password admin
   docker exec datasafe-keycloak-test /opt/keycloak/bin/kcadm.sh update realms/datasafe -s loginTheme=datasafe -s displayName=DataSafeS3
   ```

### OIDC: `token exchange failed: dial tcp [::1]:8180: connect: connection refused`

**Cause:** Issuer or token endpoint points to `localhost` / `127.0.0.1`. Browser returns code to DataSafeS3 after Keycloak login, but **code→token exchange** runs inside Docker `storage-server` — for it `localhost` is the container itself, not Keycloak on the host.

**Symptom in JSON:**

```json
{"error":"token exchange failed: Post \"http://localhost:8180/realms/datasafe/protocol/openid-connect/token\": dial tcp [::1]:8180: connect: connection refused"}
```

**Fix:**

1. Admin → Settings → OIDC: **Issuer URL** = `http://localhost:8180/realms/datasafe` (for browser).
2. **Internal issuer URL** = `http://host.docker.internal:8180/realms/datasafe` (for token exchange).
3. Or leave internal issuer empty — with issuer containing `localhost`, DataSafeS3 in Docker substitutes `host.docker.internal` automatically.
4. Keycloak well-known may advertise `localhost` in `token_endpoint` — DataSafeS3 rewrites host to internal issuer during token exchange.

Verify from container:

```cmd
docker compose exec storage-server wget -qO- http://host.docker.internal:8180/realms/datasafe/.well-known/openid-configuration
```

### OIDC: redirect mismatch

Redirect in Keycloak client and DataSafeS3 must **exactly** match registered URI. For dev, `http://localhost:8080/*` in Keycloak and full callback in DataSafeS3 is sufficient.

### Ports in use

LDAP: `-p 1389:389` and URL `ldap://host.docker.internal:1389`. Keycloak: `-p 8081:8080` and issuer `http://host.docker.internal:8081/realms/datasafe`.

### LDAP bind / Test connection failed

- Check Bind DN and password (`ldapadmin`).
- Ensure Base DN is `ou=users,dc=datasafe,dc=local` and login username is `cn` (`ldapuser`, not email).

### LDAP: `dial tcp [::1]:389: connect: connection refused` (Network Error)

`storage-server` in Docker cannot reach `localhost:389` — for the container that is itself, not the host.

**Solution (one of):**

1. Leave `ldap://localhost:389` in UI — DataSafeS3 automatically rewrites loopback to `host.docker.internal` when connecting from container (like OIDC internal issuer).
2. Explicitly set `ldap://host.docker.internal:389` in Settings → LDAP URL.

Verify from container:

```cmd
docker compose exec storage-server wget -qO- ldap://host.docker.internal:389
```

Ensure LDAP container is running: `docker ps --filter name=datasafe-ldap-test`.

---

## Integration Summary

| Service | Container | Ports | DataSafeS3 (Docker) |
|--------|-----------|-------|-------------------|
| LDAP | `datasafe-ldap-test` | 389, 636 | `ldap://host.docker.internal:389` |
| Keycloak | `datasafe-keycloak-test` | 8180→8080 | `http://host.docker.internal:8180/realms/datasafe` |

| Account | Login | Password | Purpose |
|--------|-------|--------|------------|
| LDAP admin bind | `cn=admin,dc=datasafe,dc=local` | `ldapadmin` | Bind DN in Settings |
| LDAP user | `ldapuser` | `password` | LDAP login |
| Keycloak admin | `admin` | `admin` | Admin Console |
| SSO user | `ssouser` | `password` | Sign in with SSO |
