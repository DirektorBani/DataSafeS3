# Quarterly DR drill checklist automation (Community Edition — manual failover pattern)
param(
  [string]$BaseUrl = "http://127.0.0.1:8080",
  [string]$ComposeProject = "datasafe",
  [switch]$SkipFailover
)
$ErrorActionPreference = "Stop"
$results = @()

function Test-Step($name, [scriptblock]$fn) {
  try {
    & $fn
    $script:results += [pscustomobject]@{ Step = $name; Status = "PASS" }
    Write-Host "[dr-drill] PASS: $name" -ForegroundColor Green
  } catch {
    $script:results += [pscustomobject]@{ Step = $name; Status = "FAIL"; Detail = $_.Exception.Message }
    Write-Host "[dr-drill] FAIL: $name — $($_.Exception.Message)" -ForegroundColor Red
  }
}

Test-Step "Core stack health" {
  $h = Invoke-RestMethod -Uri "$BaseUrl/healthz"
  if ($h.status -ne "ok") { throw "healthz not ok" }
}

Test-Step "Postgres metadata (when configured)" {
  $h = Invoke-RestMethod -Uri "$BaseUrl/healthz"
  if ($null -ne $h.postgres_ok -and $h.postgres_ok -eq $false) { throw "postgres_ok=false" }
}

Test-Step "Read-only standby profile exists" {
  if (-not (Test-Path "docker-compose.ha.yml")) { throw "docker-compose.ha.yml missing" }
}

Test-Step "Backup smoke (list buckets)" {
  $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
  $tok = $login.token
  $b = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Headers @{ Authorization = "Bearer $tok" }
  if ($null -eq $b.buckets) { throw "no buckets response" }
}

if (-not $SkipFailover) {
  Test-Step "Failover script dry-run" {
    & "$PSScriptRoot\postgres-failover.ps1" -ComposeProject $ComposeProject -DryRun | Out-Null
  }
}

Test-Step "Federation demo smoke" {
  if (Test-Path "$PSScriptRoot\federation-3node-demo.ps1") {
    & "$PSScriptRoot\federation-3node-demo.ps1" -BaseUrl "$BaseUrl/api/v1" | Out-Null
  }
}

$fail = @($results | Where-Object { $_.Status -eq "FAIL" })
Write-Host ""
Write-Host "DR drill summary: $($results.Count) steps, $($fail.Count) failed"
$results | Format-Table -AutoSize
if ($fail.Count -gt 0) { exit 1 }
exit 0
