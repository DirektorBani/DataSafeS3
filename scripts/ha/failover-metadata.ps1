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
Write-Host "Exit codes: 0 ok or already done, 1 usage, 2 precheck, 3 fence, 4 promote, 5 postcheck"
Write-Host "1. Fence the old primary (default_transaction_read_only=on, not replicated)"
Write-Host "2. Promote postgres standby ($StandbyContainer) when it is still in recovery"
Write-Host "3. Point STORAGE_POSTGRES_DSN at the new primary and restart that storage node"
Write-Host "4. On a fresh promote, clear ha_leader_lock so the new node can acquire it. A re-run leaves a live lease."
Write-Host "5. Verify /healthz is_leader=true on the new node and the old node refuses writes"

$promote = Join-Path $PSScriptRoot "promote.ps1"
if ($DryRun -or -not $Execute) {
    & $promote -PrimaryContainer "" -StandbyContainer $StandbyContainer -DryRun
    exit $LASTEXITCODE
}
Write-Host "[failover-metadata] executing promote.ps1"
& $promote -PrimaryContainer $env:PRIMARY_CONTAINER -StandbyContainer $StandbyContainer -Execute
exit $LASTEXITCODE
