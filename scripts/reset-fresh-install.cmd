@echo off
REM Reset DataSafeS3 to fresh install state. See reset-fresh-install.ps1 for details.
setlocal
cd /d "%~dp0.."
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0reset-fresh-install.ps1" %*
endlocal
