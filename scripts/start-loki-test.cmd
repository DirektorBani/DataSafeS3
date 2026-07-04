@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "CONTAINER=datasafe-log-loki"
set "NETWORK=datasafe_default"
set "PORT=3100"
set "IMAGE=grafana/loki:2.9.4"

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

docker network inspect %NETWORK% >nul 2>&1
if errorlevel 1 (
  echo Docker network %NETWORK% not found. Start datasafe compose stack first.
  exit /b 1
)

echo Starting %CONTAINER% (Loki for external logging audit)...
docker run -d ^
  --name %CONTAINER% ^
  --network %NETWORK% ^
  -p %PORT%:3100 ^
  %IMAGE% -config.file=/etc/loki/local-config.yaml
if errorlevel 1 exit /b 1

:done
echo Loki ready URL (host): http://localhost:%PORT%/ready
echo Loki address (in stack): http://%CONTAINER%:3100
exit /b 0
