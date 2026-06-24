@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "BASE_URL=http://localhost:8080"
set "ADMIN_USER=admin"
set "ADMIN_PASS=admin"
if not "%STORAGE_ADMIN_USER%"=="" set "ADMIN_USER=%STORAGE_ADMIN_USER%"
if not "%STORAGE_ADMIN_PASSWORD%"=="" set "ADMIN_PASS=%STORAGE_ADMIN_PASSWORD%"

set "MINIO_ENDPOINT=http://host.docker.internal:9100"
set "CONN_NAME=External S3 Test"
set "REMOTE_BUCKET=replica-test"
if not "%GATEWAY_SOURCE_BUCKET%"=="" set "SOURCE_BUCKET=%GATEWAY_SOURCE_BUCKET%"

echo Ensuring remote S3 bucket %REMOTE_BUCKET% exists (no data wipe)...
docker run --rm --entrypoint sh minio/mc:latest -c "mc alias set test http://host.docker.internal:9100 minioadmin minioadmin && mc mb test/%REMOTE_BUCKET% --ignore-existing" >nul 2>&1

echo Configuring Gateway connection and replication via DataSafeS3 API...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-minio-gateway.ps1" -BaseUrl "%BASE_URL%" -AdminUser "%ADMIN_USER%" -AdminPass "%ADMIN_PASS%" -ConnectionName "%CONN_NAME%" -MinioEndpoint "%MINIO_ENDPOINT%" -RemoteBucket "%REMOTE_BUCKET%" -SourceBucket "%SOURCE_BUCKET%"
if errorlevel 1 exit /b 1

echo Done.
