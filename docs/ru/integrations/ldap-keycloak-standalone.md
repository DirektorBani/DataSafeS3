**[English](../../en/integrations/ldap-keycloak-standalone.md)** | Русский

# LDAP и SSO (Keycloak) для тестирования DataSafeS3

Отдельные Docker-контейнеры **вне** `docker-compose.yml` — только для локальной интеграции с DataSafeS3 (Датасейф S3). Данные сохраняются в named volumes.

> **RF-safe:** образы тянутся из публичных реестров; после первого `docker pull` работа возможна офлайн. Секреты ниже — **только для dev/test**, не для production.

## Что ожидает DataSafeS3

### LDAP (Admin → Settings → LDAP / Active Directory)

| Поле в UI | JSON / env | Описание |
|-----------|------------|----------|
| Enable LDAP | `ldap.enabled`, `STORAGE_LDAP_ENABLED` | Включить вход и синхронизацию |
| URL | `ldap.url`, `STORAGE_LDAP_URL` | `ldap://host:389` или `ldaps://host:636` |
| Bind DN | `ldap.bind_dn`, `STORAGE_LDAP_BIND_DN` | Учётная запись для поиска пользователей |
| Bind password | `ldap.bind_password`, `STORAGE_LDAP_BIND_PASSWORD` | Пароль bind |
| Base DN | `ldap.base_dn`, `STORAGE_LDAP_BASE_DN` | Корень поиска пользователей |
| Group DN | `ldap.group_dn` | База поиска групп (`groupOfNames` / `member`) — для openldap-test: `ou=groups,dc=datasafe,dc=local` |
| Group attribute | `ldap.group_attr` | Для AD: `memberOf` на записи пользователя |

Дополнительно в API: `user_attr` (по умолчанию `cn`), `group_role_map`, `sync_on_login`, `sync_interval_minutes` (фоновая синхронизация; `0` отключает; при включении LDAP в UI по умолчанию **60**).

**Поиск пользователя:** `(user_attr=username)`, по умолчанию `(cn=username)` в `Base DN`.

**API:** `POST /api/v1/settings/ldap/test`, `POST /api/v1/settings/ldap/sync` (только admin). Фоновый worker вызывает sync по заданному интервалу, когда LDAP включён.

### OIDC / SSO (Admin → Settings → OIDC / SSO)

| Поле в UI | JSON / env | Описание |
|-----------|------------|----------|
| Enable SSO | `oidc.enabled`, `STORAGE_OIDC_ENABLED` | Кнопка «Sign in with SSO» на `/login` |
| Issuer URL | `oidc.issuer`, `STORAGE_OIDC_ISSUER` | Публичный issuer (браузер, redirect на Keycloak) |
| Internal issuer URL | `oidc.internal_issuer`, `STORAGE_OIDC_INTERNAL_ISSUER` | Issuer для **серверных** вызовов из Docker (token exchange, userinfo). Опционально — если пусто и issuer содержит `localhost`, DataSafeS3 в контейнере подставит `host.docker.internal` автоматически |
| Client ID | `oidc.client_id`, `STORAGE_OIDC_CLIENT_ID` | OAuth2 client id |
| Client secret | `oidc.client_secret`, `STORAGE_OIDC_CLIENT_SECRET` | Секрет confidential client |
| Redirect URL | `oidc.redirect_url`, `STORAGE_OIDC_REDIRECT_URL` | Callback DataSafeS3 |
| Groups claim | `oidc.groups_claim` | Имя claim с группами (по умолчанию `groups`) |

**Redirect URL для консоли на порту 8080:**

```text
http://localhost:8080/api/v1/auth/oidc/callback
```

**Поток SSO (v1.0.2+):** `/login` → `GET /api/v1/auth/oidc/login` → IdP → callback сервера → редирект на `/login?exchange_code=…&auth_source=oidc` → консоль вызывает `POST /api/v1/auth/oidc/exchange` → JWT в теле ответа. Старый query-параметр `?token=` устарел и удалён в v1.0.2.

**Автотест (ROPC):** `POST /api/v1/auth/oidc/password-login` с `{"username":"ssouser","password":"password"}` — по умолчанию отключён в production (`STORAGE_OIDC_ROPC_ENABLED=false`); включайте только на test IdP с `directAccessGrantsEnabled` в Keycloak.

**Тестовая группа Keycloak:** realm import включает группу `datasafe-users` и mapper **Group Membership** → claim `groups`; пользователь `ssouser` состоит в этой группе. После изменения `datasafe-realm.json` удалите volume: `docker volume rm datasafe-keycloak-data` и пересоздайте контейнер.

**Сессия SSO:** для пользователей OIDC JWT живёт **15 минут**; сервер хранит связь с access token Keycloak и проверяет его через introspection (кэш ~30 с). Сразу после успешного callback introspection **не вызывается** (~30 с) — иначе первый запрос `/me` мог получить 401 и вернуть пользователя на `/login`. Если сессия в Keycloak завершена (logout в Admin Console, end session), следующий API-запрос после истечения кэша вернёт **401**, консоль очистит токен и перенаправит на `/login`. Кнопка **Выйти** в консоли вызывает `POST /api/v1/admin/logout` и для SSO — redirect на Keycloak `end_session_endpoint`.

---

## 1. LDAP (osixia/openldap)

### Быстрый старт (скрипт)

```cmd
scripts\start-ldap-test.cmd
```

### Однострочник `docker run`

```cmd
docker run -d --name datasafe-ldap-test -p 389:389 -p 636:636 -e LDAP_ORGANISATION=DataSafeS3 -e LDAP_DOMAIN=datasafe.local -e LDAP_ADMIN_PASSWORD=ldapadmin -v datasafe-ldap-data:/var/lib/ldap -v datasafe-ldap-config:/etc/ldap/slapd.d -v "%CD%\docs\integrations\ldap-test\bootstrap.ldif:/container/service/sldif/custom/50-datasafe-users.ldif:ro" osixia/openldap:1.5.0
```

Bootstrap LDIF: `docs/integrations/ldap-test/bootstrap.ldif` — OU `users`, `groups`, пользователи `ldapuser`, `ldapadmin`, группа `datasafe-users`.

### Значения для DataSafeS3

| Параметр | С хоста (ldapsearch) | DataSafeS3 в Docker (`storage-server`) |
|----------|----------------------|--------------------------------------|
| URL | `ldap://localhost:389` | `ldap://host.docker.internal:389` |
| Bind DN | `cn=admin,dc=datasafe,dc=local` | то же |
| Bind password | `ldapadmin` | то же |
| Base DN | `ou=users,dc=datasafe,dc=local` | то же |
| User attr | `cn` (по умолчанию) | то же |
| Фильтр входа | `(cn=<username>)` | то же |

**Тестовые пользователи:** `ldapuser` / `password`, `ldapadmin` / `password`.

### Проверка LDAP

```cmd
docker ps --filter name=datasafe-ldap-test
```

С хоста (если установлен OpenLDAP client):

```cmd
ldapsearch -x -H ldap://localhost:389 -D "cn=admin,dc=datasafe,dc=local" -w ldapadmin -b "ou=users,dc=datasafe,dc=local" "(cn=ldapuser)"
```

Через временный контейнер:

```cmd
docker run --rm osixia/openldap:1.5.0 ldapsearch -x -H ldap://host.docker.internal:389 -D "cn=admin,dc=datasafe,dc=local" -w ldapadmin -b "ou=users,dc=datasafe,dc=local" "(cn=ldapuser)"
```

---

## 2. SSO — Keycloak (OIDC)

### Быстрый старт (скрипт)

```cmd
scripts\start-keycloak-test.cmd
```

Первый запуск Keycloak может занять 30–60 секунд.

### Однострочник `docker run`

```cmd
docker run -d --name datasafe-keycloak-test -p 8180:8080 -e KEYCLOAK_ADMIN=admin -e KEYCLOAK_ADMIN_PASSWORD=admin -v datasafe-keycloak-data:/opt/keycloak/data -v "%CD%\docs\integrations\keycloak-test\datasafe-realm.json:/opt/keycloak/data/import/datasafe-realm.json:ro" -v "%CD%\docs\integrations\keycloak-test\themes:/opt/keycloak/themes:ro" quay.io/keycloak/keycloak:26.0.7 start-dev --import-realm
```

Realm `datasafe`, client `datasafe-console`, пользователь `ssouser` — см. `docs/integrations/keycloak-test/datasafe-realm.json`.

### Тема входа DataSafeS3

Кастомная тема `datasafe` монтируется из `docs/integrations/keycloak-test/themes/` и задаётся в realm import (`loginTheme: datasafe`). Стили повторяют тёмную палитру консоли (`web/console/src/index.css`): фон `hsl(222.2 84% 4.9%)`, акцент `hsl(217.2 91.2% 59.8%)`, карточка входа max-width 420px как shadcn Card, логотип и заголовок **Датасейф S3** (`login/login.ftl` + `resources/css/datasafe.css`).

После изменения CSS или `theme.properties` пересоздайте контейнер (volume realm не перезаписывается автоматически):

```cmd
docker stop datasafe-keycloak-test
docker rm datasafe-keycloak-test
scripts\start-keycloak-test.cmd
```

Проверка страницы входа (откройте в браузере):

```text
http://localhost:8180/realms/datasafe/protocol/openid-connect/auth?client_id=datasafe-console&redirect_uri=http://localhost:8080/&response_type=code&scope=openid
```

Account console (тот же login theme): http://localhost:8180/realms/datasafe/account

CSS темы: `docs/integrations/keycloak-test/themes/datasafe/login/resources/css/datasafe.css`

### Значения для DataSafeS3

| Параметр | Браузер / curl с хоста | DataSafeS3 в Docker (server-side) |
|----------|------------------------|-------------------------------|
| Issuer URL (public) | `http://localhost:8180/realms/datasafe` | то же (для redirect пользователя) |
| Internal issuer URL | — | `http://host.docker.internal:8180/realms/datasafe` |
| Client ID | `datasafe-console` | то же |
| Client secret | `datasafe-console-secret` | то же |
| Redirect URL | `http://localhost:8080/api/v1/auth/oidc/callback` | то же (URL, который видит браузер) |

> **Критично для Docker:** `storage-server` обменивает authorization code на token **из контейнера**, не из браузера. Issuer с `localhost:8180` в well-known Keycloak указывает token endpoint на `[::1]:8180` — из контейнера это недоступно. Задайте **Internal issuer** = `http://host.docker.internal:8180/realms/datasafe` или только public issuer с `localhost` — сервер подставит `host.docker.internal` автоматически (если запущен в Docker).

**Keycloak Admin Console:** http://localhost:8180/admin — `admin` / `admin`.

**Тестовый пользователь SSO:** `ssouser` / `password`.

### Проверка OIDC discovery

```cmd
curl -s http://localhost:8180/realms/datasafe/.well-known/openid-configuration
```

Ожидается JSON с `issuer`, `authorization_endpoint`, `token_endpoint`.

---

## 3. Настройка в DataSafeS3

1. Запустите основной стек DataSafeS3 (`docker compose up` или локальный dev).
2. Запустите LDAP и Keycloak скриптами выше.
3. Войдите как **admin** → **Settings**.
4. **LDAP:** заполните таблицу выше. Если `storage-server` в Docker, URL может быть `ldap://localhost:389` (авто-перезапись на `host.docker.internal`) или явно `ldap://host.docker.internal:389`. Включите LDAP → **Сохранить** → **Проверить подключение** → **Синхронизировать пользователей** (опционально).
5. **OIDC:** Public issuer `http://localhost:8180/realms/datasafe`, internal issuer `http://host.docker.internal:8180/realms/datasafe`, client id/secret, redirect URL → включите SSO → **Save**.
6. Откройте `/login` в режиме инкognito → **Sign in with SSO** → войдите как `ssouser`.

### Пример env для `storage-server` (опционально, начальные значения)

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

UI-настройки в Bolt перекрывают пустые env-поля при сохранении.

### Test Connection через API

```cmd
curl -s -X POST http://localhost:8080/api/v1/settings/ldap/test ^
  -H "Authorization: Bearer <ADMIN_JWT>" ^
  -H "Content-Type: application/json" ^
  -d "{\"url\":\"ldap://host.docker.internal:389\",\"bind_dn\":\"cn=admin,dc=datasafe,dc=local\",\"bind_password\":\"ldapadmin\",\"base_dn\":\"ou=users,dc=datasafe,dc=local\"}"
```

---

## 4. Остановка

```cmd
scripts\stop-ldap-keycloak-test.cmd
```

Удалить контейнеры **и** volumes (полный сброс LDAP/Keycloak):

```cmd
scripts\stop-ldap-keycloak-test.cmd --remove
```

---

## Устранение неполадок

### `host.docker.internal` из контейнера `storage-server`

DataSafeS3 подключается к LDAP и OIDC **с сервера**, не из браузера. Если контейнер не резолвит `host.docker.internal`:

- **Docker Desktop (Windows/macOS):** имя обычно работает из коробки.
- **Linux:** добавьте в `docker-compose.yml` для `storage-server`:
  ```yaml
  extra_hosts:
    - "host.docker.internal:host-gateway"
  ```
- Альтернатива: IP хоста в URL (`ldap://172.17.0.1:389`).

Проверка из контейнера DataSafeS3:

```cmd
docker compose exec storage-server wget -qO- http://host.docker.internal:8180/realms/datasafe/.well-known/openid-configuration
```

### LDAP: bootstrap не применился

Скрипт `start-ldap-test.cmd` после старта контейнера выполняет `ldapadd -c` для `bootstrap.ldif` (идемпотентно). Если пользователей нет вручную:

```cmd
docker exec datasafe-ldap-test ldapadd -x -c -D "cn=admin,dc=datasafe,dc=local" -w ldapadmin -f /container/service/sldif/custom/50-datasafe-users.ldif
```

После смены `bootstrap.ldif` на существующем volume:

```cmd
scripts\stop-ldap-keycloak-test.cmd --remove
scripts\start-ldap-test.cmd
```

### Keycloak: realm не импортировался

Аналогично — удалите volume `datasafe-keycloak-data` и пересоздайте контейнер. Либо создайте realm вручную в Admin Console по таблице выше.

### Keycloak: тема `datasafe` не применилась

Realm import (`--import-realm`) выполняется **только при первом создании** volume. Если контейнер уже существовал с `datasafe-keycloak-data`, поля вроде `loginTheme` из `datasafe-realm.json` не обновятся автоматически.

**Варианты:**

1. Полный сброс: `scripts\stop-ldap-keycloak-test.cmd --remove`, затем `scripts\start-keycloak-test.cmd`.
2. Вручную в Admin Console: Realm **datasafe** → **Themes** → Login theme = `datasafe`.
3. CLI:
   ```cmd
   docker exec datasafe-keycloak-test /opt/keycloak/bin/kcadm.sh config credentials --server http://localhost:8080 --realm master --user admin --password admin
   docker exec datasafe-keycloak-test /opt/keycloak/bin/kcadm.sh update realms/datasafe -s loginTheme=datasafe -s displayName=Датасейф S3
   ```

### OIDC: `token exchange failed: dial tcp [::1]:8180: connect: connection refused`

**Причина:** Issuer или token endpoint указывает на `localhost` / `127.0.0.1`. Браузер после логина в Keycloak возвращает code на DataSafeS3, но **обмен code→token** выполняет `storage-server` внутри Docker — для него `localhost` это сам контейнер, не Keycloak на хосте.

**Симптом в JSON:**

```json
{"error":"token exchange failed: Post \"http://localhost:8180/realms/datasafe/protocol/openid-connect/token\": dial tcp [::1]:8180: connect: connection refused"}
```

**Исправление:**

1. Admin → Settings → OIDC: **Issuer URL** = `http://localhost:8180/realms/datasafe` (для браузера).
2. **Internal issuer URL** = `http://host.docker.internal:8180/realms/datasafe` (для token exchange).
3. Либо оставьте internal issuer пустым — при issuer с `localhost` DataSafeS3 в Docker подставит `host.docker.internal` автоматически.
4. Keycloak well-known может рекламировать `localhost` в `token_endpoint` — DataSafeS3 переписывает host на internal issuer при обмене token.

Проверка из контейнера:

```cmd
docker compose exec storage-server wget -qO- http://host.docker.internal:8180/realms/datasafe/.well-known/openid-configuration
```

### OIDC: redirect mismatch

Redirect в Keycloak client и в DataSafeS3 должен **точно** совпадать с зарегистрированным URI. Для dev достаточно `http://localhost:8080/*` в Keycloak и полного callback в DataSafeS3.

### Порты заняты

LDAP: `-p 1389:389` и URL `ldap://host.docker.internal:1389`. Keycloak: `-p 8081:8080` и issuer `http://host.docker.internal:8081/realms/datasafe`.

### LDAP bind / Test connection failed

- Проверьте Bind DN и пароль (`ldapadmin`).
- Убедитесь, что Base DN — `ou=users,dc=datasafe,dc=local`, а логин пользователя — `cn` (`ldapuser`, не email).

### LDAP: `dial tcp [::1]:389: connect: connection refused` (Network Error)

`storage-server` в Docker не может достучаться до `localhost:389` — для контейнера это сам контейнер, а не хост.

**Решение (один из вариантов):**

1. Оставить в UI `ldap://localhost:389` — DataSafeS3 автоматически перезаписывает loopback на `host.docker.internal` при подключении из контейнера (как для OIDC internal issuer).
2. Явно указать `ldap://host.docker.internal:389` в Settings → LDAP URL.

Проверка из контейнера:

```cmd
docker compose exec storage-server wget -qO- ldap://host.docker.internal:389
```

Убедитесь, что LDAP-контейнер запущен: `docker ps --filter name=datasafe-ldap-test`.

---

## Сводная таблица интеграции

| Сервис | Контейнер | Порты | DataSafeS3 (Docker) |
|--------|-----------|-------|-------------------|
| LDAP | `datasafe-ldap-test` | 389, 636 | `ldap://host.docker.internal:389` |
| Keycloak | `datasafe-keycloak-test` | 8180→8080 | `http://host.docker.internal:8180/realms/datasafe` |

| Учётка | Логин | Пароль | Назначение |
|--------|-------|--------|------------|
| LDAP admin bind | `cn=admin,dc=datasafe,dc=local` | `ldapadmin` | Bind DN в Settings |
| LDAP user | `ldapuser` | `password` | Вход через LDAP |
| Keycloak admin | `admin` | `admin` | Admin Console |
| SSO user | `ssouser` | `password` | Sign in with SSO |
