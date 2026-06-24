@echo off
setlocal
cd /d "%~dp0.."

call scripts\ensure-docker-pull-proxy.cmd

echo Building Linux storage-server binary from current source...
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o deploy\docker\storage-server-linux .\cmd\storage-server
if errorlevel 1 exit /b 1

echo Building web console static assets for Caddy...
call scripts\build-console.cmd
if errorlevel 1 exit /b 1

echo Rebuilding storage-server image (entrypoint fixes /data permissions for UID 65532)...
docker compose --profile postgres -f docker-compose.yml -f docker-compose.local-binary.yml build storage-server
if errorlevel 1 exit /b 1

echo Ensuring /data volume is owned by UID 65532 (matches runtime user; was mismatched with USER nobody / 65534)...
docker run --rm -v datasafe_storage-data:/data --user root alpine:3.20 chown -R 65532:65532 /data

echo Starting stack with postgres profile and locally built binary...
docker compose --profile postgres -f docker-compose.yml -f docker-compose.local-binary.yml up -d postgres storage-server --no-deps
if errorlevel 1 exit /b 1
docker compose --profile postgres -f docker-compose.yml -f docker-compose.local-binary.yml up -d caddy prometheus grafana --no-deps
if errorlevel 1 exit /b 1

echo.
echo Waiting for admin login endpoint...
set /a tries=0
:wait_loop
curl -sf -X POST http://localhost:8080/api/v1/admin/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin\"}" >nul 2>&1
if not errorlevel 1 goto done
set /a tries+=1
if %tries% geq 30 (
  echo Admin login did not become ready in time.
  docker compose ps
  exit /b 1
)
timeout /t 2 /nobreak >nul
goto wait_loop

:done
echo.
docker compose ps
echo.
echo Admin login URL: http://localhost:8080/api/v1/admin/login
echo Console:         http://localhost:8080/
echo Grafana:         http://localhost:3000/
echo Prometheus:      http://localhost:9090/
echo.
echo Test:
echo   curl -X POST http://localhost:8080/api/v1/admin/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin\"}"
