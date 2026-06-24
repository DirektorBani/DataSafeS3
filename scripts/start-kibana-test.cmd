@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0.."

set "ES_CONTAINER=datasafe-elasticsearch-test"
set "CONTAINER=datasafe-kibana-test"
set "NETWORK=datasafe_default"
set "ES_PORT=19200"
set "KIBANA_PORT=5601"
set "ES_PASSWORD=ElasticTest123!"
set "TOKEN_NAME=kibana-dev"
set "IMAGE=docker.elastic.co/kibana/kibana:8.11.0"

docker inspect %ES_CONTAINER% >nul 2>&1
if errorlevel 1 (
  echo Elasticsearch container %ES_CONTAINER% not found. Run scripts\start-elasticsearch-test.cmd first.
  exit /b 1
)

docker start %ES_CONTAINER% >nul 2>&1

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

echo Creating Kibana service account token (%TOKEN_NAME%)...
for /f "usebackq delims=" %%T in (`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; $p='%ES_PASSWORD%'; $h='http://localhost:%ES_PORT%'; $n='%TOKEN_NAME%'; $pair='elastic:'+$p; $bytes=[Text.Encoding]::ASCII.GetBytes($pair); $auth=[Convert]::ToBase64String($bytes); $hdr=@{Authorization=('Basic '+$auth)}; try { Invoke-RestMethod -Method Delete -Uri ($h+'/_security/service/elastic/kibana/credential/token/'+$n) -Headers $hdr | Out-Null } catch {}; $r=Invoke-RestMethod -Method Post -Uri ($h+'/_security/service/elastic/kibana/credential/token/'+$n) -Headers $hdr; Write-Output $r.token.value"`) do set "KIBANA_TOKEN=%%T"

if not defined KIBANA_TOKEN (
  echo Failed to obtain Kibana service account token. Is Elasticsearch up on port %ES_PORT%?
  exit /b 1
)

echo Starting %CONTAINER% (Kibana 8.11)...
docker run -d ^
  --name %CONTAINER% ^
  --network %NETWORK% ^
  -p %KIBANA_PORT%:5601 ^
  -e "ELASTICSEARCH_HOSTS=http://%ES_CONTAINER%:9200" ^
  -e "ELASTICSEARCH_SERVICEACCOUNTTOKEN=%KIBANA_TOKEN%" ^
  %IMAGE%
if errorlevel 1 exit /b 1

:done
echo.
echo Kibana UI:              http://localhost:%KIBANA_PORT%
echo Login (browser):        elastic / %ES_PASSWORD%
echo Elasticsearch (Docker): http://%ES_CONTAINER%:9200
echo.
echo Discover: Stack Management -^> Data Views -^> Create -^> index pattern datasafe-logs* , time field time
echo Or run:                    scripts\setup-kibana-logs.cmd
echo Useful fields: time, level, msg, method, path, status, duration_ms, remote
