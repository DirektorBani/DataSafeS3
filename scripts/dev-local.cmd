@echo off
setlocal
cd /d "%~dp0.."

echo DataSafeS3 local dev (no Docker)
echo.
echo This opens two windows:
echo   1. storage-server  - http://localhost:9000
echo   2. web console     - http://localhost:5173
echo.
echo Sign in with admin / admin
echo.

start "DataSafeS3 storage-server" cmd /k "%~dp0dev-local-server.cmd"
timeout /t 2 /nobreak >nul
start "DataSafeS3 web console" cmd /k "%~dp0dev-local-console.cmd"

echo Started. Close the server windows to stop.
