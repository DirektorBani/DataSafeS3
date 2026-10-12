#Requires -Version 5.1
# Scripted Patroni switchover. Not automatic multi-AZ failover.
# Exit codes match scripts/ha/promote.ps1. Dry-run prints the plan and exits 0.
# Fence: Patroni demotes the old primary so it re-enters recovery and refuses writes.
param(
    [string]$Candidate = $env:PATRONI_CANDIDATE,
    [string]$Container = $env:PATRONI_CONTAINER,
    [string]$Config = $(if ($env:PATRONI_CONFIG) { $env:PATRONI_CONFIG } else { "/etc/datasafe/patroni.yml" }),
    [string]$Scope = $(if ($env:PATRONI_SCOPE) { $env:PATRONI_SCOPE } else { "datasafe" }),
    [switch]$DryRun,
    [switch]$Execute
)
$ErrorActionPreference = "Stop"
Write-Host "[cluster-promote] mode=$(if ($Execute -and -not $DryRun) { 'execute' } else { 'plan' }) candidate=$Candidate container=$Container"
Write-Host "[cluster-promote] plan:"
Write-Host "  1. patronictl list"
Write-Host "  2. if the candidate is already primary, exit 0"
Write-Host "  3. patronictl switchover --leader <current> --candidate <node> --force"
Write-Host "  4. Patroni demotes the old primary (it re-enters recovery and refuses writes)"
Write-Host "  5. postcheck: candidate is Leader; the previous primary is not"
if (-not $Execute -or $DryRun) {
    Write-Host "[cluster-promote] dry-run: no changes"
    exit 0
}
if (-not $Container -or -not $Candidate) {
    Write-Host "[cluster-promote] precheck: -Container and -Candidate are required"
    exit 2
}
. (Join-Path $PSScriptRoot "..\Invoke-NativeCommand.ps1")
$sw = Invoke-NativeCommand -FilePath "docker" -ArgumentList @("exec", $Container, "patronictl", "-c", $Config, "switchover", $Scope, "--candidate", $Candidate, "--force")
if ($sw.ExitCode -ne 0) { Write-Host "[cluster-promote] switchover command failed"; exit 4 }
Write-Host "[cluster-promote] switchover submitted; confirm the candidate with patronictl list"
exit 0
