# Site-replication slice of feature-audit (no full 93-test run).
param(
    [string]$BaseUrl = "",
    [string]$S3Url = ""
)
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "lab-ports.ps1")
$labRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$siteAPorts = Read-LabEnv (Join-Path $labRoot ".env.site-a")
$siteBPorts = Read-LabEnv (Join-Path $labRoot ".env.site-b")
if (-not $BaseUrl) { $BaseUrl = "http://127.0.0.1:$(Get-LabPort $siteAPorts "PUBLIC_HTTP_PORT" "8086")" }
if (-not $S3Url) { $S3Url = "http://127.0.0.1:$(Get-LabPort $siteAPorts "SITE_A_STORAGE_PORT" "9006")" }
$env:DATASAFE_SITE_REPL_E2E = "1"
$env:DATASAFE_SITE_REPL_PEER_CONSOLE = "http://127.0.0.1:$(Get-LabPort $siteBPorts "SITE_B_HTTP_PORT" "9082")"
$env:DATASAFE_SITE_REPL_PEER_S3 = "http://host.docker.internal:$(Get-LabPort $siteBPorts "SITE_B_STORAGE_PORT" "9193")"

$out = & (Join-Path $PSScriptRoot "..\feature-audit-test.ps1") -BaseUrl $BaseUrl -S3Url $S3Url *>&1 | Out-String
$srLines = @($out -split "`n" | Where-Object { $_ -match "Site replication" })
$srPass = @($srLines | Where-Object { $_ -match "\[PASS\]" }).Count
$srSkip = @($srLines | Where-Object { $_ -match "\[SKIP\]" }).Count
$srFail = @($srLines | Where-Object { $_ -match "\[FAIL\]" }).Count
Write-Host ($srLines -join "`n")
Write-Host "Site replication audit: pass=$srPass skip=$srSkip fail=$srFail"
if ($srFail -gt 0 -or $srPass -lt 3) { exit 1 }
exit 0
