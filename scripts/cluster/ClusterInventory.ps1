#Requires -Version 5.1
<#
.SYNOPSIS
  Cluster inventory validation (Wave 1).
#>

function ConvertTo-ClusterInventoryObject {
  param([Parameter(Mandatory)][string]$Json)
  return ($Json | ConvertFrom-Json)
}

function Test-ClusterInventory {
  <#
  .SYNOPSIS
    Validate cluster inventory for Wave 1.
  .OUTPUTS
    PSCustomObject: @{ Ok = bool; Errors = string[] }
  #>
  param(
    [Parameter(Mandatory)]$Inventory
  )
  $errors = New-Object System.Collections.Generic.List[string]
  if (-not $Inventory) {
    $errors.Add("inventory is empty")
    return [pscustomobject]@{ Ok = $false; Errors = @($errors) }
  }
  $nodes = @($Inventory.nodes)
  if ($nodes.Count -lt 3) {
    $errors.Add("need at least 3 nodes, got $($nodes.Count)")
  }
  $ips = New-Object System.Collections.Generic.List[string]
  $i = 0
  foreach ($n in $nodes) {
    $i++
    $ip = [string]$n.ip
    if ([string]::IsNullOrWhiteSpace($ip)) {
      $errors.Add("node[$i]: missing ip")
      continue
    }
    $ip = $ip.Trim()
    if ($ip -eq '127.0.0.1' -or $ip -eq '::1' -or $ip -eq 'localhost') {
      $errors.Add("node[$i]: loopback address not allowed in cluster inventory ($ip)")
    }
    if ($ips -contains $ip) {
      $errors.Add("duplicate ip: $ip")
    } else {
      $ips.Add($ip) | Out-Null
    }
    $sshPort = 22
    if ($null -ne $n.ssh_port -and "$($n.ssh_port)" -ne '') {
      try { $sshPort = [int]$n.ssh_port } catch { $errors.Add("node[$i]: invalid ssh_port") }
    }
    if ($sshPort -lt 1 -or $sshPort -gt 65535) {
      $errors.Add("node[$i]: ssh_port out of range")
    }
  }
  $mode = [string]$Inventory.ssh_mode
  if ($mode -ne 'P' -and $mode -ne 'K') {
    $errors.Add("ssh_mode must be P or K (got '$mode')")
  }
  # Security: password must never appear in inventory object
  $raw = ($Inventory | ConvertTo-Json -Depth 8 -Compress)
  if ($raw -match '(?i)"password"\s*:' -or $raw -match '(?i)"root_password"\s*:') {
    $errors.Add("inventory must not contain password fields")
  }
  return [pscustomobject]@{ Ok = ($errors.Count -eq 0); Errors = @($errors) }
}

function New-ClusterInventorySkeleton {
  param(
    [Parameter(Mandatory)][string[]]$Ips,
    [ValidateSet('P','K')][string]$SshMode = 'P',
    [string]$SshUser = 'root'
  )
  $nodes = foreach ($ip in $Ips) {
    [pscustomobject]@{
      ip       = $ip.Trim()
      ssh_port = 22
      roles    = @('storage','postgres','etcd','lb')
    }
  }
  return [pscustomobject]@{
    version      = 1
    ssh_mode     = $SshMode
    ssh_user     = $SshUser
    layout       = 'production'  # 4+2 default; 'dev' = 2+1
    vip_mode     = 'subnet'      # or 'dns'
    nodes        = @($nodes)
    # passwords NEVER stored here
  }
}

function Save-ClusterInventory {
  param(
    [Parameter(Mandatory)]$Inventory,
    [Parameter(Mandatory)][string]$Path
  )
  $check = Test-ClusterInventory -Inventory $Inventory
  if (-not $check.Ok) {
    throw ("invalid inventory: " + ($check.Errors -join '; '))
  }
  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path $dir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }
  ($Inventory | ConvertTo-Json -Depth 8) | Set-Content -LiteralPath $Path -Encoding utf8
}

function Get-ClusterStateDir {
  $userHome = $env:USERPROFILE
  if (-not $userHome) { $userHome = $env:HOME }
  return (Join-Path $userHome ".datasafe-cluster")
}
