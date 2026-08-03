#Requires -Version 5.1
<#
.SYNOPSIS
  Wave 1 assert script for cluster installer (no live SSH required).
.EXAMPLE
  powershell -NoProfile -File scripts\tests\cluster-installer-w1.ps1
#>
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $Root
. ".\scripts\cluster\ClusterInventory.ps1"
. ".\scripts\cluster\ClusterPreflight.ps1"

$failed = 0
function Assert-True([bool]$cond, [string]$msg) {
  if ($cond) { Write-Host "  PASS $msg" -ForegroundColor Green }
  else { Write-Host "  FAIL $msg" -ForegroundColor Red; $script:failed++ }
}

Write-Host "=== cluster-installer-w1 ==="

# valid inventory
$good = New-ClusterInventorySkeleton -Ips @('10.0.0.1','10.0.0.2','10.0.0.3') -SshMode P
$r = Test-ClusterInventory -Inventory $good
Assert-True $r.Ok "valid 3-node inventory"

# too few
$bad = New-ClusterInventorySkeleton -Ips @('10.0.0.1','10.0.0.2') -SshMode P
$r = Test-ClusterInventory -Inventory $bad
Assert-True (-not $r.Ok) "reject <3 nodes"

# loopback
$bad2 = New-ClusterInventorySkeleton -Ips @('127.0.0.1','10.0.0.2','10.0.0.3') -SshMode K
$r = Test-ClusterInventory -Inventory $bad2
Assert-True (-not $r.Ok) "reject loopback"

# duplicate
$bad3 = New-ClusterInventorySkeleton -Ips @('10.0.0.1','10.0.0.1','10.0.0.3') -SshMode P
$r = Test-ClusterInventory -Inventory $bad3
Assert-True (-not $r.Ok) "reject duplicate ip"

# password field forbidden
$evil = $good | ConvertTo-Json | ConvertFrom-Json
$evil | Add-Member -NotePropertyName password -NotePropertyValue 'secret' -Force
$r = Test-ClusterInventory -Inventory $evil
Assert-True (-not $r.Ok) "reject password field in inventory"

# save + reload must not invent secrets
$tmp = Join-Path $env:TEMP "datasafe-cluster-w1-test.json"
Save-ClusterInventory -Inventory $good -Path $tmp
$disk = Get-Content -Raw $tmp
Assert-True ($disk -notmatch '(?i)password') "saved inventory has no password"
Remove-Item $tmp -Force

# DryRun preflight skip network
$pf = Invoke-ClusterPreflightWave1 -Inventory $good -SkipLiveNetwork
Assert-True $pf.Ok "preflight SkipLiveNetwork ok"

# TCP helper against closed port on localhost (likely fail - assert function returns bool)
$tcp = Test-ClusterTcpPort -TargetHost '127.0.0.1' -Port 1 -TimeoutMs 500
Assert-True ($tcp -is [bool]) "Test-ClusterTcpPort returns bool"

# SECURITY.md present
Assert-True (Test-Path ".\scripts\cluster\SECURITY.md") "SECURITY.md exists"

if ($failed -gt 0) {
  Write-Host "FAILED: $failed" -ForegroundColor Red
  exit 1
}
Write-Host "ALL PASS" -ForegroundColor Green
exit 0
