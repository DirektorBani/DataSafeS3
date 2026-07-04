@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "CONTAINER=datasafe-minio-test"
set "PORT=9100"
set "DATA=%TEMP%\datasafe-minio-test-data"

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

if not exist "%DATA%" mkdir "%DATA%"

echo Starting %CONTAINER% (MinIO for Gateway audit on port %PORT%)...
docker run -d --name %CONTAINER% ^
  -p %PORT%:9000 ^
  -e MINIO_ROOT_USER=minioadmin ^
  -e MINIO_ROOT_PASSWORD=minioadmin ^
  -v "%DATA%:/data" ^
  minio/minio server /data --console-address ":9001"
if errorlevel 1 exit /b 1

:done
echo MinIO test endpoint: http://localhost:%PORT% (minioadmin / minioadmin)
exit /b 0
