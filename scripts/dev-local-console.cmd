@echo off
setlocal
cd /d "%~dp0..\web\console"

if not exist node_modules (
  echo Installing npm dependencies...
  call npm install
  if errorlevel 1 exit /b 1
)

echo Starting web console on http://localhost:5173
echo API requests are proxied to http://localhost:9000 (see vite.config.ts)
echo.
call npm run dev -- --host 0.0.0.0 --port 5173
