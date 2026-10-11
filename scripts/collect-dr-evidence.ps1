#Requires -Version 5.1
<#
.SYNOPSIS
  DR evidence folder and zip after a scripted promote.

.DESCRIPTION
  Same report as scripts/collect-dr-evidence.sh: summary.md (EN), evidence.json,
  and a zip. Operator evidence, not a certification. -Lab records healthz for
  the SSH lab and exits 0 when status is ok.

.EXAMPLE
  .\scripts\collect-dr-evidence.ps1 -Lab -BaseUrl http://127.0.0.1:9010 -OutDir .\dr-evidence
#>
param(
    [Parameter(Mandatory = $true)][string]$OutDir,
    [string]$NewUrl = "",
    [string]$OldUrl = "",
    [string]$BaseUrl = "",
    [string]$PgNew = "",
    [string]$PgOld = "",
    [string]$TimingFile = "",
    [string]$KeysFile = "",
    [string]$WebhookLog = "",
    [string]$Bucket = "dr-drill",
    [string]$VersionFile = "",
    [switch]$Lab
)
$ErrorActionPreference = "Stop"
$bash = Get-Command bash -ErrorAction SilentlyContinue
if ($bash) {
    $sh = Join-Path $PSScriptRoot "collect-dr-evidence.sh"
    $args = @($sh, "--out", $OutDir, "--bucket", $Bucket)
    if ($Lab) { $args += "--lab" }
    if ($BaseUrl) { $args += @("--base-url", $BaseUrl) }
    if ($NewUrl) { $args += @("--new-url", $NewUrl) }
    if ($OldUrl) { $args += @("--old-url", $OldUrl) }
    if ($PgNew) { $args += @("--pg-new", $PgNew) }
    if ($PgOld) { $args += @("--pg-old", $PgOld) }
    if ($TimingFile) { $args += @("--timing-file", $TimingFile) }
    if ($KeysFile) { $args += @("--keys-file", $KeysFile) }
    if ($WebhookLog) { $args += @("--webhook-log", $WebhookLog) }
    if ($VersionFile) { $args += @("--version-file", $VersionFile) }
    & bash @args
    exit $LASTEXITCODE
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
if (-not $VersionFile) {
    $candidate = Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path "VERSION"
    if (Test-Path $candidate) { $VersionFile = $candidate }
}
$version = if ($VersionFile -and (Test-Path $VersionFile)) { (Get-Content -Raw $VersionFile).Trim() } else { "unknown" }
$now = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$url = if ($Lab) { $BaseUrl } else { $NewUrl }
if (-not $url) { Write-Error "NewUrl or -Lab -BaseUrl is required"; exit 1 }
$health = $null
$ok = $false
try {
    $health = Invoke-RestMethod -Uri ($url.TrimEnd("/") + "/healthz") -TimeoutSec 8
    $ok = ($health.status -eq "ok")
} catch {
    $ok = $false
}
$result = if ($ok) { "pass" } else { "fail" }
$summary = @"
# DR evidence

Generated: $now
Result: $($result.ToUpper())

## Promote

- mode: $(if ($Lab) { 'SSH lab health check' } else { 'promote' })
- url: $url
- healthz: $(if ($ok) { 'ok' } else { 'failed' })
- VERSION file: $version

## What this is

Scripted promote of a Postgres standby, with the old primary fenced so it refuses writes.
It is not automatic multi-AZ failover.

Windows PowerShell without bash writes this health summary. Git bash runs scripts/collect-dr-evidence.sh, which also checks lag, the outbox, and object write/read.
"@
Set-Content -Encoding utf8 (Join-Path $OutDir "summary.md") $summary
$json = @{
    generated_at = $now
    mode         = $(if ($Lab) { "lab" } else { "promote" })
    result       = $result
    healthz      = $health
    versions     = @{ version_file = $version }
    notes        = @("Scripted promote is not automatic multi-AZ failover.")
} | ConvertTo-Json -Depth 6
Set-Content -Encoding utf8 (Join-Path $OutDir "evidence.json") $json
$zip = Join-Path (Split-Path -Parent $OutDir) ((Split-Path -Leaf $OutDir) + ".zip")
if (Test-Path $zip) { Remove-Item $zip -Force }
Compress-Archive -Path (Join-Path $OutDir "*") -DestinationPath $zip
Write-Host "evidence: $OutDir"
Write-Host "zip: $zip"
if (-not $ok) { exit 5 }
exit 0
