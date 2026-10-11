#Requires -Version 5.1
# Scripted Postgres promote. Not automatic multi-AZ failover. A re-run is safe.
# Exit codes match scripts/ha/promote.sh and internal/ha/promote:
#   0  done, already in that state, or dry-run / plan
#   1  usage
#   2  precheck
#   3  fence failed
#   4  promote failed
#   5  postcheck failed
# Fence: ALTER SYSTEM SET default_transaction_read_only=on on the named old
# primary. That file is not replicated. The storage-server drops its leader
# lease when renew fails, so HTTP writes stop without waiting for the TTL.
param(
    [string]$PrimaryContainer = "",
    [string]$StandbyContainer = "",
    [string]$PgUser = "datasafe",
    [string]$PgDB = "datasafe",
    [int]$MaxLagBytes = 1048576,
    [string]$TimingFile = "",
    [switch]$DryRun,
    [switch]$Execute
)
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "..\Invoke-NativeCommand.ps1")

function Show-Plan {
    Write-Host "[promote] mode=$(if ($Execute -and -not $DryRun) { 'execute' } else { 'plan' }) primary=$PrimaryContainer standby=$StandbyContainer max_lag_bytes=$MaxLagBytes"
    Write-Host "[promote] plan:"
    Write-Host "  1. measure replication lag; refuse when the old primary is up and lag is above the limit"
    Write-Host "  2. fence PrimaryContainer with default_transaction_read_only=on (ALTER SYSTEM, not replicated)"
    Write-Host "  3. pg_promote StandbyContainer when it is still in recovery"
    Write-Host "  4. on a fresh promote, DELETE ha_leader_lock; a re-run leaves a live lease"
    Write-Host "  5. postcheck: old primary refuses CREATE TABLE; new primary is writable and not in recovery"
    Write-Host "[promote] lease: a failed leader renew marks that storage-server process not leader immediately"
}

function Invoke-Psql([string]$Container, [string]$Sql) {
    return Invoke-NativeCommand -FilePath "docker" -ArgumentList @("exec", $Container, "psql", "-U", $PgUser, "-d", $PgDB, "-v", "ON_ERROR_STOP=1", "-Atqc", $Sql)
}

function Test-Running([string]$Container) {
    if (-not $Container) { return $false }
    $r = Invoke-NativeCommand -FilePath "docker" -ArgumentList @("inspect", "-f", "{{.State.Running}}", $Container)
    return ($r.ExitCode -eq 0 -and (($r.Stdout -join "").Trim() -eq "true"))
}

function Get-Recovery([string]$Container) {
    $r = Invoke-Psql $Container "SELECT pg_is_in_recovery();"
    return (($r.Stdout -join "").Trim() -eq "t")
}

function Test-Fenced([string]$Container) {
    if (-not (Test-Running $Container)) { return $true }
    if (Get-Recovery $Container) { return $true }
    $ro = Invoke-Psql $Container "SELECT current_setting('transaction_read_only');"
    if ((($ro.Stdout -join "").Trim()) -ne "on") { return $false }
    $create = Invoke-Psql $Container "CREATE TABLE dr_fence_probe(id int);"
    if ($create.ExitCode -eq 0) {
        Invoke-Psql $Container "DROP TABLE IF EXISTS dr_fence_probe;" | Out-Null
        return $false
    }
    return $true
}

Show-Plan
if (-not $Execute -or $DryRun) {
    Write-Host "[promote] dry-run: no changes"
    exit 0
}
if (-not $PrimaryContainer -or -not $StandbyContainer) {
    Write-Host "promote: -Execute requires -PrimaryContainer and -StandbyContainer"
    exit 1
}
if ($PrimaryContainer -eq $StandbyContainer) {
    Write-Host "promote: primary and standby are the same container"
    exit 1
}
if (-not (Test-Running $StandbyContainer)) {
    Write-Host "[promote] precheck: standby $StandbyContainer is not running"
    exit 2
}

$already = -not (Get-Recovery $StandbyContainer)
if (-not $already) {
    if (Test-Running $PrimaryContainer) {
        $lag = Invoke-Psql $PrimaryContainer "SELECT COALESCE((SELECT MAX(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn))::bigint FROM pg_stat_replication), -1);"
        $lagN = 0
        [void][int]::TryParse((($lag.Stdout -join "").Trim()), [ref]$lagN)
        if ($lag.ExitCode -ne 0 -or $lagN -lt 0) {
            Write-Host "[promote] precheck: no streaming replica visible from $PrimaryContainer"
            exit 2
        }
        if ($lagN -gt $MaxLagBytes) {
            Write-Host "[promote] precheck: lag $lagN bytes is above $MaxLagBytes"
            exit 2
        }
        Write-Host "[promote] lag before promote: $lagN bytes"
    } else {
        Write-Host "[promote] old primary is down; lag is unknown and promote continues"
    }
} else {
    Write-Host "[promote] standby is already a primary; promote will be skipped"
}

if (-not (Test-Fenced $PrimaryContainer)) {
    Write-Host "[promote] fencing ${PrimaryContainer}: default_transaction_read_only=on"
    $alter = Invoke-Psql $PrimaryContainer "ALTER SYSTEM SET default_transaction_read_only = on;"
    if ($alter.ExitCode -ne 0) { Write-Host "[promote] fence failed: ALTER SYSTEM"; exit 3 }
    $reload = Invoke-Psql $PrimaryContainer "SELECT pg_reload_conf();"
    if ($reload.ExitCode -ne 0) { Write-Host "[promote] fence failed: reload"; exit 3 }
    Invoke-Psql $PrimaryContainer "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid() AND backend_type = 'client backend';" | Out-Null
    if (-not (Test-Fenced $PrimaryContainer)) {
        Write-Host "[promote] fence failed: $PrimaryContainer still accepts writes"
        exit 3
    }
} else {
    Write-Host "[promote] old primary $PrimaryContainer is already fenced"
}

if (Get-Recovery $StandbyContainer) {
    Write-Host "[promote] promoting $StandbyContainer"
    $prom = Invoke-Psql $StandbyContainer "SELECT pg_promote(wait := true);"
    if ($prom.ExitCode -ne 0) { Write-Host "[promote] pg_promote failed"; exit 4 }
    $left = $false
    for ($i = 0; $i -lt 30; $i++) {
        if (-not (Get-Recovery $StandbyContainer)) { $left = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $left) { Write-Host "[promote] standby still in recovery"; exit 4 }
}

if ($already) {
    Write-Host "[promote] re-run leaves a live ha_leader_lock on $StandbyContainer in place"
} else {
    $clear = Invoke-Psql $StandbyContainer "DO `$`$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'ha_leader_lock') THEN DELETE FROM ha_leader_lock WHERE lock_id = 'storage-server'; END IF; END `$`$;"
    if ($clear.ExitCode -ne 0) { Write-Host "[promote] postcheck: could not clear ha_leader_lock"; exit 5 }
}
$write = Invoke-Psql $StandbyContainer "CREATE TABLE dr_fence_probe(id int);"
if ($write.ExitCode -ne 0) { Write-Host "[promote] postcheck: new primary refused a write"; exit 5 }
Invoke-Psql $StandbyContainer "DROP TABLE IF EXISTS dr_fence_probe;" | Out-Null
if (-not (Test-Fenced $PrimaryContainer)) { Write-Host "[promote] postcheck: old primary is still writable"; exit 3 }
Write-Host "[promote] postcheck ok"
if ($TimingFile) {
    $now = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    "{`"finished_at`":`"$now`",`"idempotent_rerun`":$($already.ToString().ToLower())}" | Set-Content -Encoding utf8 $TimingFile
}
exit 0
