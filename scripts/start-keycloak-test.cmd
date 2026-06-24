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
  goto :done
)

echo Starting %CONTAINER% (Keycloak dev mode)...
docker run -d ^
  --name %CONTAINER% ^
  -p %PORT%:8080 ^
  -e KEYCLOAK_ADMIN=%KC_ADMIN% ^
  -e KEYCLOAK_ADMIN_PASSWORD=%KC_ADMIN_PASSWORD% ^
  -v %VOLUME%:/opt/keycloak/data ^
  -v "%REALM%:/opt/keycloak/data/import/datasafe-realm.json:ro" ^
  -v "%THEMES%:/opt/keycloak/themes:ro" ^
  quay.io/keycloak/keycloak:26.0.7 start-dev --import-realm
if errorlevel 1 exit /b 1

:done
echo.
echo Keycloak admin:       %KC_ADMIN% / %KC_ADMIN_PASSWORD%
echo Issuer (host):        http://localhost:%PORT%/realms/datasafe
echo Issuer (Docker):      http://host.docker.internal:%PORT%/realms/datasafe
echo Client ID:            datasafe-console
echo Client secret:        datasafe-console-secret
echo Redirect URI:         http://localhost:8080/api/v1/auth/oidc/callback
echo Test user:            ssouser / password
echo.
echo See docs\integrations\ldap-keycloak-standalone.md
