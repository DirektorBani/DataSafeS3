@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "CONTAINER=datasafe-keycloak-test"
set "VOLUME=datasafe-keycloak-data"
set "REALM=%CD%\docs\integrations\keycloak-test\datasafe-realm.json"
set "THEMES=%CD%\docs\integrations\keycloak-test\themes"
set "PORT=8180"
set "KC_ADMIN=admin"
set "KC_ADMIN_PASSWORD=admin"
set "COMPOSE_NET=datasafe_default"

if not exist "%REALM%" (
  echo Realm JSON not found: %REALM%
  exit /b 1
)

docker inspect %CONTAINER% >nul 2>&1
if not errorlevel 1 (
  echo Container %CONTAINER% already exists.
  docker start %CONTAINER% >nul 2>&1
  if errorlevel 1 (
    echo Failed to start %CONTAINER%.
    exit /b 1
  )
  echo Started existing container %CONTAINER%.
  goto :ready
)

echo Starting %CONTAINER% (Keycloak behind same-origin edge proxy on :8080)...
rem Hostname env (not CLI flags) so public URLs are http://localhost:8080/realms/...
rem Direct admin UI remains on :8180 for ops.
docker run -d ^
  --name %CONTAINER% ^
  -p %PORT%:8080 ^
  -e KEYCLOAK_ADMIN=%KC_ADMIN% ^
  -e KEYCLOAK_ADMIN_PASSWORD=%KC_ADMIN_PASSWORD% ^
  -e KC_HTTP_ENABLED=true ^
  -e KC_PROXY_HEADERS=xforwarded ^
  -e KC_HOSTNAME=localhost ^
  -e KC_HOSTNAME_PORT=8080 ^
  -e KC_HOSTNAME_STRICT=false ^
  -v %VOLUME%:/opt/keycloak/data ^
  -v "%REALM%:/opt/keycloak/data/import/datasafe-realm.json:ro" ^
  -v "%THEMES%:/opt/keycloak/themes:ro" ^
  quay.io/keycloak/keycloak:26.0.7 start-dev --import-realm
if errorlevel 1 exit /b 1

:ready
docker network inspect %COMPOSE_NET% >nul 2>&1
if not errorlevel 1 (
  docker network connect %COMPOSE_NET% %CONTAINER% >nul 2>&1
)

echo.
echo Keycloak admin (direct):   http://localhost:%PORT%/admin  %KC_ADMIN% / %KC_ADMIN_PASSWORD%
echo Issuer (edge / same URL):  http://localhost:8080/realms/datasafe
echo Issuer (direct):           http://localhost:%PORT%/realms/datasafe
echo Client ID:                 datasafe-console
echo OAuth2 Proxy client:       datasafe-oauth2-proxy
echo Test user:                 ssouser / password
echo.
echo See docs\integrations\ldap-keycloak-standalone.md
