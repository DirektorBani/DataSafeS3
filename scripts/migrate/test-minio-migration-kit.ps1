#Requires -Version 5.1
<#
.SYNOPSIS
  Autotest suite for A1 MinIO migration kit (docs, spine, DryRun smoke).

.EXAMPLE
  powershell -NoProfile -File scripts/migrate/test-minio-migration-kit.ps1
#>
[CmdletBinding()]
param(
  [switch]$SkipGo,
  [switch]$SkipDryRun
)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

$script:Pass = 0
$script:Fail = 0
$script:Skip = 0

function Record([string]$Name, [string]$Status, [string]$Notes = "") {
  $line = "[{0}] {1}" -f $Status, $Name
  if ($Notes) { $line += " - $Notes" }
  Write-Host $line
  switch ($Status) {
    "PASS" { $script:Pass++ }
    "FAIL" { $script:Fail++ }
    "SKIP" { $script:Skip++ }
    default { }
  }
}

Write-Host "=== test-minio-migration-kit ==="
Write-Host "Root: $Root"
Write-Host ""

# --- Artifact presence ---
$artifacts = @(
  "docs\operations-guide\en\migrate-from-minio.md",
  "docs\operations-guide\ru\migrate-from-minio.md",
  "docs\operations-guide\en\examples\rclone-minio-to-datasafe.conf",
  "scripts\migrate\minio-cutover-smoke.ps1",
  "scripts\migrate\README.md",
  "docs\architecture\adr\0001-migration-kit.md",
  "docs\architecture\adr\README.md",
  "internal\migrate\checklist.go",
  "internal\events\sink.go",
  "internal\inventory\types.go",
  "CHANGELOG.md"
)
foreach ($rel in $artifacts) {
  $p = Join-Path $Root $rel
  if (Test-Path -LiteralPath $p) {
    Record "artifact $rel" "PASS"
  }
  else {
    Record "artifact $rel" "FAIL" "missing"
  }
}

# --- Guide honesty (EN) ---
$enGuide = Join-Path $Root "docs\operations-guide\en\migrate-from-minio.md"
$enText = Get-Content -LiteralPath $enGuide -Raw
if ($enText -match "not a drop-in" -and $enText -match "rclone" -and $enText -match "Honesty") {
  Record "EN guide honesty block" "PASS"
}
else {
  Record "EN guide honesty block" "FAIL" "missing honesty/rclone markers"
}

$ruGuide = Join-Path $Root "docs\operations-guide\ru\migrate-from-minio.md"
$ruText = Get-Content -LiteralPath $ruGuide -Raw
if ($ruText -match "rclone" -and ($ruText -match "Честно" -or $ruText -match "не заявление")) {
  Record "RU guide honesty block" "PASS"
}
else {
  Record "RU guide honesty block" "FAIL"
}

# --- CHANGELOG ---
$cl = Get-Content -LiteralPath (Join-Path $Root "CHANGELOG.md") -Raw
if ($cl -match "\#\# \[1\.1\.1\]" -and $cl -match "MinIO migration kit") {
  Record "CHANGELOG 1.1.1 section" "PASS"
}
else {
  Record "CHANGELOG 1.1.1 section" "FAIL"
}

# --- README link ---
$readme = Get-Content -LiteralPath (Join-Path $Root "README.md") -Raw
if ($readme -match "migrate-from-minio") {
  Record "README links migrate-from-minio" "PASS"
}
else {
  Record "README links migrate-from-minio" "FAIL"
}

# --- storage-cli migrate checklist ---
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
  Record "storage-cli migrate checklist" "SKIP" "go not on PATH"
}
else {
  $cliOut = & go run ./cmd/storage-cli migrate checklist minio 2>&1 | Out-String
  if ($LASTEXITCODE -eq 0 -and $cliOut -match "minio-cutover-smoke" -and $cliOut -match "Cutover") {
    Record "storage-cli migrate checklist" "PASS"
  }
  else {
    Record "storage-cli migrate checklist" "FAIL" "exit $LASTEXITCODE"
  }
}

# --- Go unit tests ---
if ($SkipGo) {
  Record "go test migrate/events/inventory" "SKIP" "-SkipGo"
}
else {
  $go = Get-Command go -ErrorAction SilentlyContinue
  if (-not $go) {
    Record "go test migrate/events/inventory" "SKIP" "go not on PATH"
  }
  else {
    & go test ./internal/migrate/... ./internal/events/... ./internal/inventory/... ./internal/ha/promote/...
    if ($LASTEXITCODE -eq 0) {
      Record "go test migrate/events/inventory" "PASS"
    }
    else {
      Record "go test migrate/events/inventory" "FAIL" "exit $LASTEXITCODE"
    }
  }
}

# --- DryRun smoke ---
if ($SkipDryRun) {
  Record "minio-cutover-smoke DryRun" "SKIP" "-SkipDryRun"
}
else {
  $smoke = Join-Path $Root "scripts\migrate\minio-cutover-smoke.ps1"
  & powershell -NoProfile -File $smoke -DryRun `
    -SourceEndpoint "http://127.0.0.1:9001" `
    -DestEndpoint "http://127.0.0.1:9000" `
    -Bucket "demo"
  if ($LASTEXITCODE -eq 0) {
    Record "minio-cutover-smoke DryRun" "PASS"
  }
  else {
    Record "minio-cutover-smoke DryRun" "FAIL" "exit $LASTEXITCODE"
  }
}

# --- Live optional ---
if ($env:MIGRATE_LIVE -eq "1") {
  if (-not $env:SOURCE_ENDPOINT -or -not $env:DEST_ENDPOINT -or -not $env:MIGRATE_BUCKET) {
    Record "minio-cutover-smoke live" "SKIP" "set SOURCE_ENDPOINT DEST_ENDPOINT MIGRATE_BUCKET SOURCE_KEY DEST_KEY SOURCE_SECRET DEST_SECRET"
  }
  else {
    & powershell -NoProfile -File (Join-Path $Root "scripts\migrate\minio-cutover-smoke.ps1") `
      -SourceEndpoint $env:SOURCE_ENDPOINT -SourceKey $env:SOURCE_KEY `
      -DestEndpoint $env:DEST_ENDPOINT -DestKey $env:DEST_KEY `
      -Bucket $env:MIGRATE_BUCKET -SampleSize 10
    if ($LASTEXITCODE -eq 0) {
      Record "minio-cutover-smoke live" "PASS"
    }
    else {
      Record "minio-cutover-smoke live" "FAIL" "exit $LASTEXITCODE"
    }
  }
}
else {
  Record "minio-cutover-smoke live" "SKIP" "set MIGRATE_LIVE=1 for two-endpoint lab"
}

Write-Host ""
Write-Host ("SUMMARY PASS={0} FAIL={1} SKIP={2}" -f $script:Pass, $script:Fail, $script:Skip)
if ($script:Fail -gt 0) {
  exit 1
}
Write-Host "PASS test-minio-migration-kit"
exit 0
