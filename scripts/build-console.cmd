@echo off
setlocal
cd /d "%~dp0..\web\console"

if not exist node_modules (
  echo Installing npm dependencies...
  call npm install
  if errorlevel 1 (
    echo npm install failed. Fix proxy/registry on the host, then retry.
    exit /b 1
  )
)

echo Building console static assets (web\console\dist)...
call npm run build
if errorlevel 1 (
  echo npm run build failed.
  exit /b 1
)

echo Console built: web\console\dist
