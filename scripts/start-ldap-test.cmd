@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "CONTAINER=datasafe-ldap-test"
set "VOLUME_DATA=datasafe-ldap-data"
set "VOLUME_CONFIG=datasafe-ldap-config"
set "LDAP_DOMAIN=datasafe.local"
set "LDAP_ADMIN_PASSWORD=ldapadmin"
set "BOOTSTRAP=%CD%\docs\integrations\ldap-test\bootstrap.ldif"

if not exist "%BOOTSTRAP%" (
  echo Bootstrap LDIF not found: %BOOTSTRAP%
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

echo Starting %CONTAINER% (osixia/openldap)...
docker run -d ^
  --name %CONTAINER% ^
  -p 389:389 ^
  -p 636:636 ^
  -e LDAP_ORGANISATION=DataSafeS3 ^
  -e LDAP_DOMAIN=%LDAP_DOMAIN% ^
  -e LDAP_ADMIN_PASSWORD=%LDAP_ADMIN_PASSWORD% ^
  -v %VOLUME_DATA%:/var/lib/ldap ^
  -v %VOLUME_CONFIG%:/etc/ldap/slapd.d ^
  -v "%BOOTSTRAP%:/container/service/sldif/custom/50-datasafe-users.ldif:ro" ^
  osixia/openldap:1.5.0
if errorlevel 1 exit /b 1

:done
echo Waiting for LDAP and applying bootstrap LDIF (idempotent)...
set /a LDAP_WAIT=0
:wait_ldap
docker exec %CONTAINER% ldapsearch -x -H ldap://localhost -D "cn=admin,dc=datasafe,dc=local" -w %LDAP_ADMIN_PASSWORD% -b "dc=datasafe,dc=local" -s base "(objectClass=*)" >nul 2>&1
if errorlevel 1 (
  set /a LDAP_WAIT+=1
  if !LDAP_WAIT! GEQ 30 (
    echo LDAP did not become ready in time.
    exit /b 1
  )
  timeout /t 2 /nobreak >nul
  goto :wait_ldap
)
docker exec %CONTAINER% ldapadd -x -c -D "cn=admin,dc=datasafe,dc=local" -w %LDAP_ADMIN_PASSWORD% -f /container/service/sldif/custom/50-datasafe-users.ldif >nul 2>&1

echo.
echo LDAP URL (host):      ldap://localhost:389
echo LDAP URL (Docker):    ldap://host.docker.internal:389
echo Bind DN:              cn=admin,dc=datasafe,dc=local
echo Bind password:        %LDAP_ADMIN_PASSWORD%
echo Base DN:              ou=users,dc=datasafe,dc=local
echo Test user:            ldapuser / password
echo.
echo See docs\integrations\ldap-keycloak-standalone.md
