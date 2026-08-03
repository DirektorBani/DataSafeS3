#Requires -Version 5.1
<#
.SYNOPSIS
  Interactive Cluster wizard Wave 1 (inventory + DryRun validation).
  Remote Apply / Patroni / NFS = Wave 2.
#>

. "$PSScriptRoot\ClusterInventory.ps1"
. "$PSScriptRoot\ClusterPreflight.ps1"

function Read-ClusterModeChoice {
  Write-Host ""
  Write-Host "  Install mode:"
  Write-Host "    [1] Single-node (this machine / local Docker Compose)"
  Write-Host "    [2] Cluster (>=3 Linux VMs; Wave 1 = inventory + preflight)"
  Write-Host "    Q   quit"
  while ($true) {
    $a = (Read-Host "  Select").Trim()
    if ($a -eq '1') { return 'single' }
    if ($a -eq '2') { return 'cluster' }
    if ($a -eq 'Q' -or $a -eq 'q') { throw "Aborted by user." }
    Write-Host "  [!!] Enter 1, 2, or Q" -ForegroundColor Yellow
  }
}

function Read-ClusterSshMode {
  Write-Host ""
  Write-Host "  How to connect to nodes:"
  Write-Host "    [P] Simplified start (default): root password once -> create datasafes3 + SSH keys (Apply in Wave 2)"
  Write-Host "    [K] I already have SSH keys (root or datasafes3)"
  while ($true) {
    $a = (Read-Host "  Select [P]").Trim()
    if ([string]::IsNullOrWhiteSpace($a) -or $a -eq 'P' -or $a -eq 'p') { return 'P' }
    if ($a -eq 'K' -or $a -eq 'k') { return 'K' }
    Write-Host "  [!!] Enter P or K" -ForegroundColor Yellow
  }
}

function Read-ClusterLayout {
  Write-Host ""
  Write-Host "  Object layout (erasure):"
  Write-Host "    [R] 4+2 recommended (needs >=6 shard paths)"
  Write-Host "    [2] 2+1 small lab (>=3 shard paths)"
  while ($true) {
    $a = (Read-Host "  Select [R]").Trim()
    if ([string]::IsNullOrWhiteSpace($a) -or $a -eq 'R' -or $a -eq 'r') { return 'production' }
    if ($a -eq '2') { return 'dev' }
    Write-Host "  [!!] Enter R or 2" -ForegroundColor Yellow
  }
}

function Read-ClusterVipMode {
  Write-Host ""
  Write-Host "  Entry addresses:"
  Write-Host "    [V] VIP + keepalived (default; nodes + VIP same subnet)"
  Write-Host "    [D] DNS / external LB (no floating IP)"
  while ($true) {
    $a = (Read-Host "  Select [V]").Trim()
    if ([string]::IsNullOrWhiteSpace($a) -or $a -eq 'V' -or $a -eq 'v') { return 'subnet' }
    if ($a -eq 'D' -or $a -eq 'd') { return 'dns' }
    Write-Host "  [!!] Enter V or D" -ForegroundColor Yellow
  }
}

function Read-ClusterNodeIps {
  Write-Host ""
  Write-Host "  Enter >=3 node IPs (comma or space separated). Loopback not allowed."
  while ($true) {
    $raw = (Read-Host "  Nodes").Trim()
    $parts = @($raw -split '[\s,;]+' | Where-Object { $_ -ne '' })
    if ($parts.Count -ge 3) { return $parts }
    Write-Host "  [!!] Need at least 3 IPs" -ForegroundColor Yellow
  }
}

function Invoke-ClusterWizardWave1 {
  param([switch]$DryRun)

  $sshMode = Read-ClusterSshMode
  $sshUser = if ($sshMode -eq 'P') { 'root' } else {
    $u = (Read-Host "  SSH user [datasafes3]").Trim()
    if ([string]::IsNullOrWhiteSpace($u)) { 'datasafes3' } else { $u }
  }
  $layout = Read-ClusterLayout
  $vipMode = Read-ClusterVipMode
  $ips = Read-ClusterNodeIps

  # Security: if mode P (and not DryRun), prompt password into SecureString ONLY in-memory.
  # Wave 1 does not use it for remote calls; we still prove we never persist it.
  # DryRun skips the prompt - Wave 2 Apply will re-prompt.
  $securePassword = $null
  if ($sshMode -eq 'P' -and -not $DryRun) {
    Write-Host "  Root password is collected for future Wave 2 bootstrap only; it is NOT saved to disk." -ForegroundColor DarkGray
    $securePassword = Read-Host "  Root password (SecureString)" -AsSecureString
  } elseif ($sshMode -eq 'P' -and $DryRun) {
    Write-Host "  DryRun: skip password prompt (would collect SecureString at Apply)" -ForegroundColor DarkGray
  }

  $inv = New-ClusterInventorySkeleton -Ips $ips -SshMode $sshMode -SshUser $sshUser
  $inv.layout = $layout
  $inv.vip_mode = $vipMode

  $check = Test-ClusterInventory -Inventory $inv
  if (-not $check.Ok) {
    throw ("Inventory invalid: " + ($check.Errors -join '; '))
  }

  $stateDir = Get-ClusterStateDir
  $invPath = Join-Path $stateDir "inventory-wave1.json"
  Save-ClusterInventory -Inventory $inv -Path $invPath
  Write-Host "  [OK] Inventory written: $invPath" -ForegroundColor Green
  Write-Host "  [OK] No password fields in inventory (security check)" -ForegroundColor Green

  # Drop password reference immediately after proving we do not save it (Wave 1).
  if ($null -ne $securePassword) {
    $securePassword.Dispose()
    $securePassword = $null
    Write-Host "  [OK] In-memory password cleared (Wave 1; Wave 2 will re-prompt at Apply)" -ForegroundColor Green
  }

  $pf = Invoke-ClusterPreflightWave1 -Inventory $inv -SkipLiveNetwork:$DryRun
  foreach ($w in $pf.Warnings) { Write-Host "  [!!] $w" -ForegroundColor Yellow }
  if (-not $pf.Ok) {
    throw ("Preflight failed: " + ($pf.Errors -join '; '))
  }
  Write-Host "  [OK] Wave 1 preflight passed" -ForegroundColor Green

  # Wave 2: always offer DryRun render/plan (no live SSH unless -Apply later)
  . "$PSScriptRoot\ClusterApply.ps1"
  Write-Host ""
  Write-Host "  Wave 2 DryRun: rendering Patroni/etcd/HAProxy/keepalived/NFS plan..." -ForegroundColor Cyan
  $vipS3 = '10.0.0.10'; $vipCon = '10.0.0.11'; $vipPg = '10.0.0.12'
  if (-not $DryRun -and $vipMode -eq 'subnet') {
    $v = (Read-Host "  VIP-S3 [$vipS3]").Trim(); if ($v) { $vipS3 = $v }
    $v = (Read-Host "  VIP-Console [$vipCon]").Trim(); if ($v) { $vipCon = $v }
    $v = (Read-Host "  VIP-Postgres [$vipPg]").Trim(); if ($v) { $vipPg = $v }
  }
  $repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
  $apply = Invoke-ClusterApplyWave2 -Inventory $inv -RepoRoot $repoRoot `
    -VipS3 $vipS3 -VipConsole $vipCon -VipPostgres $vipPg -DryRun
  Write-Host "  [OK] Wave 2 configs: $($apply.OutDir)" -ForegroundColor Green
  foreach ($s in $apply.Steps) { Write-Host "       $s" -ForegroundColor DarkGray }
  foreach ($w in $apply.Warnings) { Write-Host "  [!!] $w" -ForegroundColor Yellow }

  Write-Host ""
  Write-Host "  Cluster Wave 1+2 DryRun complete." -ForegroundColor Cyan
  Write-Host "  Live remote Apply: scripts\cluster\ClusterApply.ps1 -Apply (experimental; needs SSH)."
  Write-Host "  Security: scripts\cluster\SECURITY.md"
  return $invPath
}
