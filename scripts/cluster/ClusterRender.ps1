#Requires -Version 5.1
<#
.SYNOPSIS
  Render Wave 2 configs from plan + templates (no remote SSH).
#>

. "$PSScriptRoot\ClusterPlan.ps1"

function Get-ClusterTemplateRoot {
  param([string]$RepoRoot = "")
  if (-not $RepoRoot) {
    $RepoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
  }
  return (Join-Path $RepoRoot "deploy\cluster\templates")
}

function Expand-ClusterTemplate {
  param(
    [Parameter(Mandatory)][string]$TemplateText,
    [Parameter(Mandatory)][hashtable]$Vars
  )
  $out = $TemplateText
  foreach ($k in $Vars.Keys) {
    $out = $out.Replace("{{$k}}", [string]$Vars[$k])
  }
  if ($out -match '\{\{[A-Z0-9_]+\}\}') {
    throw "unresolved template placeholders remain"
  }
  return $out
}

function Test-ClusterRenderedSecurity {
  <#
  .SYNOPSIS
    Security gate on rendered tree: no NFS wildcard clients, no unlimited sudo, no world pg_hba.
  #>
  param([Parameter(Mandatory)][string]$OutDir)
  $errors = New-Object System.Collections.Generic.List[string]
  Get-ChildItem -LiteralPath $OutDir -Recurse -File | ForEach-Object {
    $raw = Get-Content -LiteralPath $_.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $raw) { return }
    # Strip comments for pattern checks (avoid false positives in docs lines)
    $text = ($raw -split "`n" | Where-Object { $_ -notmatch '^\s*#' }) -join "`n"
    $rel = $_.FullName.Substring($OutDir.Length).TrimStart('\','/')
    if ($rel -match 'exports') {
      if ($text -match '(?m)^\s*/\S+\s+[^\n]*\*' -or $text -match '0\.0\.0\.0/0') {
        $errors.Add("NFS/export allows world: $rel")
      }
    }
    if ($rel -match 'sudoers' -and $text -match 'NOPASSWD:\s*ALL\b') {
      $errors.Add("sudoers NOPASSWD:ALL forbidden: $rel")
    }
    if ($rel -match 'patroni' -and $text -match '0\.0\.0\.0/0') {
      $errors.Add("patroni pg_hba must not use 0.0.0.0/0: $rel")
    }
  }
  return [pscustomobject]@{ Ok = ($errors.Count -eq 0); Errors = @($errors) }
}

function Invoke-ClusterRenderWave2 {
  <#
  .SYNOPSIS
    Render etcd/patroni/haproxy/keepalived/nfs/bootstrap into OutDir.
  #>
  param(
    [Parameter(Mandatory)]$Plan,
    [Parameter(Mandatory)][string]$OutDir,
    [Parameter(Mandatory)]$Secrets,
    [string]$RepoRoot = "",
    [switch]$AllowEmptyVips,
    [switch]$RedactSecrets
  )

  $planCheck = Test-ClusterPlan -Plan $Plan
  if (-not $planCheck.Ok) {
    # Allow empty VIPs for DryRun placeholder render
    $onlyVip = @($planCheck.Errors | Where-Object { $_ -match '^vip\.' })
    $other = @($planCheck.Errors | Where-Object { $_ -notmatch '^vip\.' })
    if ($other.Count -gt 0) {
      throw ("plan invalid: " + ($other -join '; '))
    }
    if ($onlyVip.Count -gt 0 -and -not $AllowEmptyVips) {
      throw ("plan invalid: " + ($onlyVip -join '; '))
    }
  }

  $tmplRoot = Get-ClusterTemplateRoot -RepoRoot $RepoRoot
  if (-not (Test-Path $tmplRoot)) { throw "templates not found: $tmplRoot" }

  $pgSuper = [string]$Secrets.pg_super_password
  $pgRepl = [string]$Secrets.pg_repl_password
  $auth = [string]$Secrets.keepalived_auth
  if ($RedactSecrets) {
    $pgSuper = 'REDACTED_PG_SUPER'
    $pgRepl = 'REDACTED_PG_REPL'
    $auth = 'REDACTED'
  }

  if (Test-Path $OutDir) { Remove-Item -LiteralPath $OutDir -Recurse -Force }
  New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

  $ips = @($Plan.node_ips)
  $etcdCluster = for ($i = 0; $i -lt $ips.Count; $i++) {
    "etcd$i=http://$($ips[$i]):2380"
  }
  $etcdInitial = $etcdCluster -join ','
  $etcdHosts = ($ips | ForEach-Object { "${_}:2379" }) -join ','

  $clusterCidr = if ($Plan.cluster_cidr -eq 'host-list') {
    ($ips | ForEach-Object { "$_/32" }) -join ','
  } else {
    [string]$Plan.cluster_cidr
  }
  $pgHbaCidr = if ($clusterCidr -match ',') {
    $parts = $Plan.leader_ip -split '\.'
    if ($parts.Count -eq 4) { "$($parts[0]).$($parts[1]).$($parts[2]).0/24" } else { '10.0.0.0/24' }
  } else { $clusterCidr }

  # Per-node etcd + patroni
  for ($i = 0; $i -lt $ips.Count; $i++) {
    $ip = $ips[$i]
    $nodeDir = Join-Path $OutDir "nodes\$ip"
    New-Item -ItemType Directory -Force -Path $nodeDir | Out-Null

    $etcdT = Get-Content -LiteralPath (Join-Path $tmplRoot "etcd\etcd.env.tmpl") -Raw
    $etcdOut = Expand-ClusterTemplate -TemplateText $etcdT -Vars @{
      ETCD_NAME            = "etcd$i"
      NODE_IP              = $ip
      ETCD_INITIAL_CLUSTER = $etcdInitial
    }
    Set-Content -LiteralPath (Join-Path $nodeDir "etcd.env") -Value $etcdOut -Encoding utf8

    $patT = Get-Content -LiteralPath (Join-Path $tmplRoot "patroni\patroni.yml.tmpl") -Raw
    $patOut = Expand-ClusterTemplate -TemplateText $patT -Vars @{
      PATRONI_NAME      = "pg$i"
      NODE_IP           = $ip
      ETCD_HOSTS        = $etcdHosts
      PG_REPL_PASSWORD  = $pgRepl
      PG_SUPER_PASSWORD = $pgSuper
      CLUSTER_CIDR      = $pgHbaCidr
    }
    Set-Content -LiteralPath (Join-Path $nodeDir "patroni.yml") -Value $patOut -Encoding utf8
  }

  # HAProxy (shared)
  $consoleServers = ($ips | ForEach-Object { "  server console_$_ ${_}:8080 check port 8080" }) -join "`n"
  $pgServers = ($ips | ForEach-Object { "  server pg_$_ ${_}:5432 check port 8008" }) -join "`n"
  $hapT = Get-Content -LiteralPath (Join-Path $tmplRoot "haproxy\haproxy.cfg.tmpl") -Raw
  $hapOut = Expand-ClusterTemplate -TemplateText $hapT -Vars @{
    LEADER_IP        = [string]$Plan.leader_ip
    VIP_S3           = [string]$Plan.vips.s3
    VIP_CONSOLE      = [string]$Plan.vips.console
    VIP_PG           = [string]$Plan.vips.postgres
    CONSOLE_SERVERS  = $consoleServers
    POSTGRES_SERVERS = $pgServers
  }
  $lbDir = Join-Path $OutDir "lb"
  New-Item -ItemType Directory -Force -Path $lbDir | Out-Null
  Set-Content -LiteralPath (Join-Path $lbDir "haproxy.cfg") -Value $hapOut -Encoding utf8

  # keepalived per VIP role
  $vipMap = @{
    s3       = @{ Vrid = 51; Vip = [string]$Plan.vips.s3; Check = '/usr/bin/curl -sf http://127.0.0.1:9000/healthz' }
    console  = @{ Vrid = 52; Vip = [string]$Plan.vips.console; Check = '/usr/bin/curl -sf http://127.0.0.1:8080/healthz' }
    postgres = @{ Vrid = 53; Vip = [string]$Plan.vips.postgres; Check = '/usr/bin/curl -sf http://127.0.0.1:8008/primary' }
  }
  $kvT = Get-Content -LiteralPath (Join-Path $tmplRoot "keepalived\keepalived.conf.tmpl") -Raw
  $pri = 100
  $unicastBlock = ""
  if ([string]$Plan.vip_mode -eq 'subnet') {
    $peerLines = ($ips | ForEach-Object { "    $_" }) -join "`n"
    $unicastBlock = "  unicast_src_ip __NODE_IP__`n  unicast_peer {`n$peerLines`n  }`n"
  }
  foreach ($role in @('s3','console','postgres')) {
    $meta = $vipMap[$role]
    $vip = $meta.Vip
    if ([string]::IsNullOrWhiteSpace($vip)) { $vip = "0.0.0.0" }
    $kvOut = Expand-ClusterTemplate -TemplateText $kvT -Vars @{
      VIP_ROLE      = $role
      CHECK_SCRIPT  = [string]$meta.Check
      VRRP_STATE    = 'BACKUP'
      INTERFACE     = [string]$Plan.interface
      VRID          = [string]$meta.Vrid
      PRIORITY      = [string]$pri
      AUTH_PASS     = $auth
      VIP_ADDRESS   = $vip
      UNICAST_BLOCK = $unicastBlock
    }
    Set-Content -LiteralPath (Join-Path $lbDir "keepalived-$role.conf") -Value $kvOut -Encoding utf8
    $pri += 10
  }

  # NFS exports: per shard owner node - only cluster IPs as clients
  $nfsDir = Join-Path $OutDir "nfs"
  New-Item -ItemType Directory -Force -Path $nfsDir | Out-Null
  $byNode = @{}
  foreach ($s in @($Plan.shards)) {
    $nip = [string]$s.node_ip
    if (-not $byNode.ContainsKey($nip)) { $byNode[$nip] = New-Object System.Collections.Generic.List[string] }
    $clients = ($ips | ForEach-Object {
      # export readable by leader primarily; allow all cluster nodes
      "$_(rw,sync,no_subtree_check,root_squash)"
    }) -join ' '
    $byNode[$nip].Add("$($s.local_path) $clients") | Out-Null
  }
  $expT = Get-Content -LiteralPath (Join-Path $tmplRoot "nfs\exports.tmpl") -Raw
  foreach ($nip in $byNode.Keys) {
    $lines = ($byNode[$nip] -join "`n")
    $expOut = Expand-ClusterTemplate -TemplateText $expT -Vars @{ EXPORT_LINES = $lines }
    Set-Content -LiteralPath (Join-Path $nfsDir "exports.$nip") -Value $expOut -Encoding utf8
  }

  $mountLines = @($Plan.shards | ForEach-Object {
    "mkdir -p `"$($_.mount_path)`"`n" +
    "mount -t nfs4 $($_.node_ip):$($_.local_path) `"$($_.mount_path)`" || true"
  }) -join "`n"
  $mntT = Get-Content -LiteralPath (Join-Path $tmplRoot "nfs\mount-shards.sh.tmpl") -Raw
  $mntOut = Expand-ClusterTemplate -TemplateText $mntT -Vars @{ MOUNT_LINES = $mountLines }
  Set-Content -LiteralPath (Join-Path $nfsDir "mount-shards-leader.sh") -Value $mntOut -Encoding utf8

  # bootstrap copies
  $bootDir = Join-Path $OutDir "bootstrap"
  New-Item -ItemType Directory -Force -Path $bootDir | Out-Null
  Copy-Item (Join-Path $tmplRoot "bootstrap\*") $bootDir -Force

  # plan.json (no secrets)
  $planPublic = [pscustomobject]@{
    version     = $Plan.version
    leader_ip   = $Plan.leader_ip
    layout      = $Plan.layout
    shard_mount = $Plan.shard_mount
    vip_mode    = $Plan.vip_mode
    vips        = $Plan.vips
    shards      = $Plan.shards
    node_ips    = $Plan.node_ips
    interface   = $Plan.interface
  }
  ($planPublic | ConvertTo-Json -Depth 8) | Set-Content -LiteralPath (Join-Path $OutDir "plan.json") -Encoding utf8

  $sec = Test-ClusterRenderedSecurity -OutDir $OutDir
  if (-not $sec.Ok) {
    throw ("security gate failed: " + ($sec.Errors -join '; '))
  }

  return [pscustomobject]@{
    Ok      = $true
    OutDir  = $OutDir
    Warnings = @($planCheck.Warnings)
  }
}
