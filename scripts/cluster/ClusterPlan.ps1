#Requires -Version 5.1
<#
.SYNOPSIS
  Build Wave 2 cluster plan (VIPs, shards, leader) from Wave 1 inventory.
#>

. "$PSScriptRoot\ClusterInventory.ps1"

function New-ClusterSecretString {
  param([int]$Length = 24)
  $bytes = New-Object byte[] $Length
  $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $rng.GetBytes($bytes)
  } finally {
    $rng.Dispose()
  }
  # alphanumeric only (keepalived auth_pass max 8 chars historically - truncate later)
  $alphabet = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  $chars = for ($i = 0; $i -lt $Length; $i++) {
    $alphabet[$bytes[$i] % $alphabet.Length]
  }
  return (-join $chars)
}

function Expand-ClusterPlan {
  <#
  .SYNOPSIS
    Derive leader, VIPs (optional placeholders), compact shard map from inventory.
  #>
  param(
    [Parameter(Mandatory)]$Inventory,
    [string]$VipS3 = "",
    [string]$VipConsole = "",
    [string]$VipPostgres = "",
    [ValidateSet('nfs','byo','sshfs')][string]$ShardMount = 'nfs',
    [string]$LeaderIp = "",
    [string]$Interface = "eth0",
    [string]$ClusterCidr = ""
  )
  $check = Test-ClusterInventory -Inventory $Inventory
  if (-not $check.Ok) {
    throw ("invalid inventory: " + ($check.Errors -join '; '))
  }

  $nodes = @($Inventory.nodes)
  $ips = @($nodes | ForEach-Object { [string]$_.ip })
  if ([string]::IsNullOrWhiteSpace($LeaderIp)) {
    $LeaderIp = $ips[0]
  }
  if ($ips -notcontains $LeaderIp) {
    throw "leader_ip $LeaderIp is not in inventory nodes"
  }

  $layout = [string]$Inventory.layout
  if ([string]::IsNullOrWhiteSpace($layout)) { $layout = 'production' }
  $needShards = if ($layout -eq 'dev') { 3 } else { 6 }

  # Compact: round-robin shards across nodes (3 VM x 2 disks for 4+2)
  $shardList = @()
  for ($i = 0; $i -lt $needShards; $i++) {
    $nodeIp = $ips[$i % $ips.Count]
    $shardList += [pscustomobject]@{
      id         = $i
      node_ip    = $nodeIp
      local_path = "/var/lib/datasafe/nfs-export/shard$i"
      mount_path = "/var/lib/datasafe/erasure/shard$i"
    }
  }
  $shards = $shardList

  if ([string]::IsNullOrWhiteSpace($ClusterCidr)) {
    # host-level allow list built at render time from node IPs
    $ClusterCidr = "host-list"
  }

  $vipMode = [string]$Inventory.vip_mode
  if ([string]::IsNullOrWhiteSpace($vipMode)) { $vipMode = 'subnet' }

  return [pscustomobject]@{
    version       = 2
    inventory     = $Inventory
    leader_ip     = $LeaderIp
    layout        = $layout
    shard_mount   = $ShardMount
    interface     = $Interface
    cluster_cidr  = $ClusterCidr
    vip_mode      = $vipMode
    vips          = [pscustomobject]@{
      s3       = $VipS3
      console  = $VipConsole
      postgres = $VipPostgres
    }
    shards        = $shards
    node_ips      = $ips
  }
}

function Test-ClusterPlan {
  param([Parameter(Mandatory)]$Plan)
  $errors = New-Object System.Collections.Generic.List[string]
  if (-not $Plan) {
    return [pscustomobject]@{ Ok = $false; Errors = @('plan empty') }
  }
  $need = if ($Plan.layout -eq 'dev') { 3 } else { 6 }
  if (@($Plan.shards).Count -lt $need) {
    $errors.Add("layout $($Plan.layout) needs >=$need shards, got $(@($Plan.shards).Count)")
  }
  if ([string]::IsNullOrWhiteSpace([string]$Plan.leader_ip)) {
    $errors.Add("leader_ip missing")
  }
  if ($Plan.vip_mode -eq 'subnet') {
    foreach ($k in @('s3','console','postgres')) {
      $v = [string]$Plan.vips.$k
      if ([string]::IsNullOrWhiteSpace($v)) {
        $errors.Add("vip.$k required when vip_mode=subnet (or use DryRun placeholders)")
      }
    }
  }
  # soft-warn style: all shards on <=2 nodes for 4+2
  if ($Plan.layout -eq 'production') {
    $distinct = @($Plan.shards | ForEach-Object { $_.node_ip } | Select-Object -Unique)
    if ($distinct.Count -lt 3) {
      $errors.Add("soft-fail-as-warn: 4+2 shards span only $($distinct.Count) node(s) - weak disk anti-affinity")
    }
  }
  # Treat soft-fail-as-warn as warnings in Apply; for Test we separate
  $hard = @($errors | Where-Object { $_ -notmatch '^soft-fail-as-warn:' })
  $warn = @($errors | Where-Object { $_ -match '^soft-fail-as-warn:' } | ForEach-Object { $_ -replace '^soft-fail-as-warn:\s*','' })
  return [pscustomobject]@{
    Ok       = ($hard.Count -eq 0)
    Errors   = $hard
    Warnings = $warn
  }
}

function Save-ClusterSecrets {
  <#
  .SYNOPSIS
    Write secrets file (mode best-effort 600). Never log contents.
  #>
  param(
    [Parameter(Mandatory)][string]$Path,
    [switch]$Force
  )
  if ((Test-Path -LiteralPath $Path) -and -not $Force) {
    return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
  }
  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path $dir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }
  $obj = [pscustomobject]@{
    version           = 1
    pg_super_password = (New-ClusterSecretString 32)
    pg_repl_password  = (New-ClusterSecretString 32)
    keepalived_auth   = ((New-ClusterSecretString 16).Substring(0, 8))
    generated_utc     = (Get-Date).ToUniversalTime().ToString('o')
  }
  ($obj | ConvertTo-Json) | Set-Content -LiteralPath $Path -Encoding utf8
  if ($IsLinux -or $IsMacOS) {
    & chmod 600 $Path 2>$null
  }
  return $obj
}
