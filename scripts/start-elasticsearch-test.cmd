@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "CONTAINER=datasafe-elasticsearch-test"
set "NETWORK=datasafe_default"
set "PORT=19200"
set "ES_PASSWORD=ElasticTest123!"
set "IMAGE=docker.elastic.co/elasticsearch/elasticsearch:8.11.0"

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

echo Starting %CONTAINER% (single-node Elasticsearch 8.11)...
docker run -d ^
  --name %CONTAINER% ^
  --network %NETWORK% ^
  -p %PORT%:9200 ^
  -e discovery.type=single-node ^
  -e xpack.security.enabled=true ^
  -e ELASTIC_PASSWORD=%ES_PASSWORD% ^
  -e "ES_JAVA_OPTS=-Xms512m -Xmx512m" ^
  %IMAGE%
if errorlevel 1 exit /b 1

:done
echo.
echo Elasticsearch URL (host):  http://localhost:%PORT%
echo Elasticsearch URL (Docker):  http://%CONTAINER%:9200
echo User / password:             elastic / %ES_PASSWORD%
echo Log indices:                 datasafe-logs, datasafe-logs-auth, datasafe-logs-bearer
echo.
echo Run scripts\start-kibana-test.cmd for the Discover UI.
