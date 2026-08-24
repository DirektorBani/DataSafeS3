#Requires -Version 5.1
<#
.SYNOPSIS
  Collect a Governance Evidence Pack folder (operator checklist helper).

.DESCRIPTION
  Downloads storage inventory CSV and activity export (CSV/JSON) via Admin API,
  optionally captures bucket settings JSON for Object Lock evidence.

  This is an operator helper вЂ” not certified compliance tooling.

.PARAMETER BaseUrl
  Storage Admin API base (default http://127.0.0.1:9000).

.PARAMETER Token
  JWT from POST /api/v1/admin/login (or set $env:DATASAFE_ADMIN_TOKEN).

.PARAMETER Bucket
  Source bucket for inventory (required).

.PARAMETER Prefix
  Optional object prefix filter.

.PARAMETER OutDir
  Output directory (default .\evidence-pack-<timestamp>).

.PARAMETER Period
  Activity period filter: 24h | 7d | 30d | all (default 30d).

.EXAMPLE
  $tok = (Invoke-RestMethod -Method POST "$BaseUrl/api/v1/admin/login" -ContentType application/json -Body '{"username":"admin","password":"admin"}').token
  .\scripts\collect-evidence-pack.ps1 -BaseUrl http://127.0.0.1:9000 -Token $tok -Bucket backups -Prefix prod/
#>
[CmdletBinding()]
param(
  [string]$BaseUrl = $(if ($env:DATASAFE_BASE_URL) { $env:DATASAFE_BASE_URL } else { "http://127.0.0.1:9000" }),
  [string]$Token = $env:DATASAFE_ADMIN_TOKEN,
  [Parameter(Mandatory = $true)][string]$Bucket,
  [string]$Prefix = "",
  [string]$OutDir = "",
  [ValidateSet("24h", "7d", "30d", "all")][string]$Period = "30d",
  [string]$DestBucket = "",
  [string]$DestKey = ""
)

$ErrorActionPreference = "Stop"
$BaseUrl = $BaseUrl.TrimEnd("/")

if ([string]::IsNullOrWhiteSpace($Token)) {
  throw "Token required: pass -Token or set DATASAFE_ADMIN_TOKEN (admin JWT)."
}

if ([string]::IsNullOrWhiteSpace($OutDir)) {
  $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
  $OutDir = Join-Path (Get-Location) "evidence-pack-$stamp"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$headers = @{ Authorization = "Bearer $Token" }

Write-Host "Evidence pack в†’ $OutDir"
Write-Host "BaseUrl=$BaseUrl bucket=$Bucket period=$Period"

# 1) Bucket settings (Lock / retention)
try {
  $settings = Invoke-RestMethod -Method GET -Uri "$BaseUrl/api/v1/buckets/$([uri]::EscapeDataString($Bucket))/settings" -Headers $headers
  $settings | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $OutDir "bucket-settings-$Bucket.json")
  Write-Host "Wrote bucket-settings-$Bucket.json"
} catch {
  Write-Warning "Could not fetch bucket settings: $_"
}

# 2) Inventory job + download
$invBody = @{
  bucket = $Bucket
  format = "csv"
}
if ($Prefix) { $invBody.prefix = $Prefix }
if ($DestBucket) {
  $invBody.dest_bucket = $DestBucket
  if ($DestKey) { $invBody.dest_key = $DestKey }
}
$invJson = $invBody | ConvertTo-Json -Compress
$invJob = Invoke-RestMethod -Method POST -Uri "$BaseUrl/api/v1/inventory/jobs" -Headers $headers -ContentType "application/json" -Body $invJson
if ($invJob.status -ne "completed") {
  throw "Inventory job failed: status=$($invJob.status) error=$($invJob.error)"
}
$invPath = Join-Path $OutDir "inventory-$Bucket.csv"
Invoke-WebRequest -Method GET -Uri "$BaseUrl/api/v1/inventory/jobs/$($invJob.id)/download" -Headers $headers -OutFile $invPath | Out-Null
Write-Host "Wrote inventory-$Bucket.csv (objects=$($invJob.object_count) truncated=$($invJob.truncated))"

# 3) Activity export CSV + JSON
$actQs = "format=csv&period=$Period&bucket=$([uri]::EscapeDataString($Bucket))"
$actCsv = Join-Path $OutDir "activity-$Bucket.csv"
Invoke-WebRequest -Method GET -Uri "$BaseUrl/api/v1/activity/export?$actQs" -Headers $headers -OutFile $actCsv | Out-Null
Write-Host "Wrote activity-$Bucket.csv"

$actQsJson = "format=json&period=$Period&bucket=$([uri]::EscapeDataString($Bucket))"
$actJsonPath = Join-Path $OutDir "activity-$Bucket.json"
Invoke-WebRequest -Method GET -Uri "$BaseUrl/api/v1/activity/export?$actQsJson" -Headers $headers -OutFile $actJsonPath | Out-Null
Write-Host "Wrote activity-$Bucket.json"

# 4) README for the folder
$readme = @"
# Governance evidence pack

Generated: $(Get-Date -Format o)
Bucket: $Bucket
Prefix: $Prefix
Period: $Period
BaseUrl: $BaseUrl

Contents:
- bucket-settings-$Bucket.json вЂ” Object Lock / retention flags (if API allowed)
- inventory-$Bucket.csv вЂ” object listing (Admin inventory; not AWS Athena)
- activity-$Bucket.csv / .json вЂ” filtered activity trail export

Honesty:
- Operator evidence only вЂ” not ISO/SOC certification
- Activity trail is not a WORM journal
- Inventory jobs are in-memory until process restart (dest-bucket copies persist)

See docs/use-cases/en/governance-evidence.md
"@
Set-Content -Encoding utf8 (Join-Path $OutDir "README.txt") -Value $readme

# Optional zip next to the folder
$zipPath = "$OutDir.zip"
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path (Join-Path $OutDir "*") -DestinationPath $zipPath -Force
Write-Host "Zip: $zipPath"
Write-Host "Done."
