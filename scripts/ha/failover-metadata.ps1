# Metadata HA failover helper - promote postgres standby and clear leader lock
param(
    [string]$ComposeProject = "datasafe-ha",
    [string]$StandbyContainer = "",
    [string]$PgPrimaryPort = "5442",
    [string]$PgStandbyPort = "5443",
    [string]$PgUser = "datasafe",
    [string]$PgPassword = "datasafe",
    [string]$PgDB = "datasafe",
    [switch]$DryRun,
    [switch]$Execute
)
$ErrorActionPreference = "Stop"
if (-not $StandbyContainer) { $StandbyContainer = "${ComposeProject}-postgres-standby-1" }

Write-Host "=== DataSafe metadata failover $(if ($DryRun) { '(dry-run)' } elseif ($Execute) { '(execute)' } else { '(plan)' }) ==="
Write-Host "1. Promote postgres standby on port $PgStandbyPort (pg_ctl promote; lab: scripts/postgres-failover.ps1)"
Write-Host "2. Update STORAGE_POSTGRES_DSN to point at new primary"
Write-Host "3. Clear leader lock on new primary:"
$sql = "DELETE FROM ha_leader_lock WHERE lock_id='storage-server';"
Write-Host "   psql: $sql"
Write-Host "4. Restart storage-server with STORAGE_HA_ENABLED=true and unique STORAGE_NODE_ID"
Write-Host "5. Verify /healthz shows is_leader=true"

$failoverScript = Join-Path $PSScriptRoot "..\postgres-failover.ps1"
if ($DryRun -and (Test-Path $failoverScript)) {
    & $failoverScript -ComposeProject $ComposeProject -StandbyContainer $StandbyContainer -DryRun | Out-Null
    Write-Host "[failover-metadata] DRY RUN complete"
    exit 0
}
if ($Execute -and (Test-Path $failoverScript)) {
    Write-Host "Running postgres-failover.ps1 (live)..."
    & $failoverScript -ComposeProject $ComposeProject -StandbyContainer $StandbyContainer
    exit $LASTEXITCODE
}
Write-Host "[failover-metadata] Plan only - pass -DryRun or -Execute to invoke postgres-failover.ps1"
