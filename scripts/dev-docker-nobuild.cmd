@echo off
setlocal
cd /d "%~dp0.."

if not exist .env (
  if exist .env.example copy /Y .env.example .env >nul
)

echo Building web console static assets for Caddy...
call scripts\build-console.cmd
if errorlevel 1 exit /b 1

echo Starting Docker Compose stack without rebuilding images...
echo (Use this when docker compose up -d --build fails due to proxy/registry issues.)
echo.

docker compose up -d
if errorlevel 1 exit /b 1

echo.
echo Waiting for storage-server to listen on :9000...
set /a tries=0
:wait_loop
curl -sf http://localhost:9000/healthz >nul 2>&1
if not errorlevel 1 goto wait_console
set /a tries+=1
if %tries% geq 30 (
  echo storage-server did not become ready in time.
  docker compose ps
  exit /b 1
)
timeout /t 2 /nobreak >nul
goto wait_loop

:wait_console
echo Waiting for console on http://localhost:8080/ ...
set tries=0
:console_loop
curl -sf -o nul -w "%%{http_code}" http://localhost:8080/ | findstr /b "200" >nul 2>&1
if not errorlevel 1 goto done
set /a tries+=1
if %tries% geq 30 (
  echo Console did not return HTTP 200 in time.
  docker compose ps
  docker compose logs --tail=20 caddy
  exit /b 1
)
timeout /t 2 /nobreak >nul
goto console_loop

:done
echo.
docker compose ps
echo.
echo Console (via Caddy): http://localhost:8080/
echo S3 API (direct):      http://localhost:9000/
echo.
echo Note: storage-server may show "unhealthy" if the cached image lacks wget.
echo       The service still works; Caddy serves pre-built console from web\console\dist.
