@echo off
setlocal
cd /d "%~dp0.."

set "STOP=%~1"
if /I "%STOP%"=="--remove" goto :remove

echo Stopping test containers...
docker stop datasafe-ldap-test datasafe-keycloak-test >nul 2>&1
echo Stopped datasafe-ldap-test and datasafe-keycloak-test (volumes preserved).
echo To remove containers and volumes: %~nx0 --remove
exit /b 0

:remove
echo Stopping and removing test containers...
docker rm -f datasafe-ldap-test datasafe-keycloak-test >nul 2>&1
docker volume rm datasafe-ldap-data datasafe-ldap-config datasafe-keycloak-data >nul 2>&1
echo Removed containers and named volumes.
exit /b 0
