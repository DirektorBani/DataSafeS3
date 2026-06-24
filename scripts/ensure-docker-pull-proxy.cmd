@echo off
setlocal
cd /d "%~dp0.."

REM When WinHTTP still points at a dead local proxy (127.0.0.1:10801), Docker pulls fail.
REM Start a tiny direct forward proxy on that port so Desktop can reach registries.

for /f "tokens=2,*" %%A in ('netsh winhttp show proxy ^| findstr /i "??????-?????? Proxy"') do set WINHTTP_PROXY=%%B
if /i not "%WINHTTP_PROXY%"=="127.0.0.1:10801" exit /b 0

powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort 10801 -State Listen -ErrorAction SilentlyContinue) { exit 0 } else { exit 1 }"
if not errorlevel 1 exit /b 0

echo Starting direct pull proxy on 127.0.0.1:10801 (stale WinHTTP proxy workaround)...
start "" /B node "%~dp0local-direct-proxy.js" >nul 2>&1
set /a proxy_tries=0
:wait_proxy
powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort 10801 -State Listen -ErrorAction SilentlyContinue) { exit 0 } else { exit 1 }"
if not errorlevel 1 goto proxy_ready
set /a proxy_tries+=1
if %proxy_tries% geq 10 (
  echo WARNING: direct pull proxy did not start on 127.0.0.1:10801.
  echo Use scripts\dev-docker-local-binary.cmd or reset WinHTTP: netsh winhttp reset proxy
  exit /b 0
)
timeout /t 1 /nobreak >nul
goto wait_proxy
:proxy_ready
exit /b 0
