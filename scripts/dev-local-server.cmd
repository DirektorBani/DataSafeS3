@echo off
setlocal
cd /d "%~dp0.."

if not exist .env (
  if exist .env.example copy /Y .env.example .env >nul
)

set STORAGE_ADDR=:9000
set STORAGE_LOG_LEVEL=info
set STORAGE_DATA_DIR=.\data
set STORAGE_REGION=us-east-1
set STORAGE_ACCESS_KEY=datasafe
set STORAGE_SECRET_KEY=datasafesecret
set STORAGE_ADMIN_USER=admin
set STORAGE_ADMIN_PASSWORD=admin
set STORAGE_JWT_SECRET=datasafe-jwt-secret

if not exist "%STORAGE_DATA_DIR%" mkdir "%STORAGE_DATA_DIR%"

echo Starting storage-server on http://localhost:9000
echo Data dir: %STORAGE_DATA_DIR%
echo.
go run ./cmd/storage-server
