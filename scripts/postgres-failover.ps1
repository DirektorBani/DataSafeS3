# Promote PostgreSQL standby and repoint DataSafeS3 DSN (Community Edition HA drill helper)
param(
  [string]$StandbyContainer = "datasafe-postgres-standby",
  [string]$PrimaryDSN = "postgres://datasafe:datasafe@postgres:5432/datasafe?sslmode=disable",
  [string]$ComposeProject = "cursor_p",
  [switch]$DryRun
)
$ErrorActionPreference = "Stop"

function Write-Step($msg) { Write-Host "[postgres-failover] $msg" }

Write-Step "Setting STORAGE_READ_ONLY on primary storage-server (stop writes)"
if (-not $DryRun) {
  docker compose -p $ComposeProject exec -T storage-server sh -c 'export STORAGE_READ_ONLY=true' 2>$null
  docker compose -p $ComposeProject stop storage-server 2>$null
}

Write-Step "Promoting standby PostgreSQL"
$promote = @"
SELECT pg_promote(wait := false);
"@
if ($DryRun) {
  Write-Step "DRY RUN: would run pg_promote on $StandbyContainer"
} else {
  docker exec $StandbyContainer psql -U datasafe -d datasafe -c $promote
}

Write-Step "Waiting for promoted node to accept connections"
for ($i = 1; $i -le 30; $i++) {
  $ok = docker exec $StandbyContainer pg_isready -U datasafe -d datasafe 2>$null
  if ($LASTEXITCODE -eq 0) { break }
  Start-Sleep -Seconds 2
}

Write-Step "Update STORAGE_POSTGRES_DSN to former standby and restart storage-server"
Write-Host "Set STORAGE_POSTGRES_DSN to promoted host DSN (example): postgres://datasafe:datasafe@<new-primary>:5432/datasafe?sslmode=disable"
Write-Host "Clear STORAGE_POSTGRES_READ_REPLICA_DSN until replica rebuilt."

Write-Step "Health wait"
if (-not $DryRun) {
  docker compose -p $ComposeProject up -d storage-server
  for ($i = 1; $i -le 60; $i++) {
    try {
      $h = Invoke-RestMethod -Uri "http://127.0.0.1:8080/healthz" -TimeoutSec 3
      if ($h.status -eq "ok") { Write-Step "PASS: /healthz ok"; exit 0 }
    } catch {}
    Start-Sleep -Seconds 2
  }
  Write-Error "Health check failed after failover"
  exit 1
}
Write-Step "DRY RUN complete"
