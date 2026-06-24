param([string]$BaseUrl = "http://127.0.0.1:8080")
$ErrorActionPreference = "Stop"
Invoke-RestMethod -Uri "$BaseUrl/healthz" | Out-Null
Write-Host "K8s-adjacent arch smoke: API reachable"
exit 0
