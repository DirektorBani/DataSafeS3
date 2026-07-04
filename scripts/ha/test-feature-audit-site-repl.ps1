# Site-replication slice of feature-audit (no full 93-test run).
param(
    [string]$BaseUrl = "http://127.0.0.1:8082",
    [string]$S3Url = "http://127.0.0.1:9002"
)
$ErrorActionPreference = "Stop"
$env:DATASAFE_SITE_REPL_E2E = "1"
$env:DATASAFE_SITE_REPL_PEER_CONSOLE = "http://127.0.0.1:9082"
$env:DATASAFE_SITE_REPL_PEER_S3 = "http://host.docker.internal:9193"

$out = & (Join-Path $PSScriptRoot "..\feature-audit-test.ps1") -BaseUrl $BaseUrl -S3Url $S3Url *>&1 | Out-String
$srLines = @($out -split "`n" | Where-Object { $_ -match "Site replication" })
$srPass = @($srLines | Where-Object { $_ -match "\[PASS\]" }).Count
$srSkip = @($srLines | Where-Object { $_ -match "\[SKIP\]" }).Count
$srFail = @($srLines | Where-Object { $_ -match "\[FAIL\]" }).Count
Write-Host ($srLines -join "`n")
Write-Host "Site replication audit: pass=$srPass skip=$srSkip fail=$srFail"
if ($srFail -gt 0 -or $srPass -lt 3) { exit 1 }
exit 0
