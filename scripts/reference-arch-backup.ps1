# Reference architecture: backup landing zone smoke (requires running stack)
param([string]$BaseUrl = "http://127.0.0.1:8080")
$ErrorActionPreference = "Stop"
$r = Invoke-RestMethod -Uri "$BaseUrl/healthz" -Method Get
if ($r.status -ne "ok") { throw "health check failed" }
Write-Host "Backup arch smoke: stack healthy"
exit 0
