param([string]$BaseUrl = "http://127.0.0.1:8080")
$ErrorActionPreference = "Stop"
Invoke-RestMethod -Uri "$BaseUrl/healthz" | Out-Null
Write-Host "Corporate files arch smoke: stack healthy (LDAP/share flows in full audit)"
exit 0
