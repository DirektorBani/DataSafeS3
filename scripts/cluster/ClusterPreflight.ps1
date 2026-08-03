#Requires -Version 5.1
<#
.SYNOPSIS
  Cluster preflight helpers (Wave 1).
#>

function Test-ClusterTcpPort {
  param(
    [Parameter(Mandatory)][string]$TargetHost,
    [Parameter(Mandatory)][int]$Port,
    [int]$TimeoutMs = 3000
  )
  try {
    $client = New-Object System.Net.Sockets.TcpClient
    $iar = $client.BeginConnect($TargetHost, $Port, $null, $null)
    $ok = $iar.AsyncWaitHandle.WaitOne($TimeoutMs, $false)
    if (-not $ok) {
      try { $client.Close() } catch {}
      return $false
    }
    $client.EndConnect($iar)
    $client.Close()
    return $true
  } catch {
    return $false
  }
}

function Invoke-ClusterPreflightWave1 {
  <#
  .SYNOPSIS
    Wave 1 preflight: TCP to ssh ports; optional SSH BatchMode for mode K.
  #>
  param(
    [Parameter(Mandatory)]$Inventory,
    [switch]$SkipLiveNetwork,
    [string]$IdentityFile = ""
  )
  $errors = New-Object System.Collections.Generic.List[string]
  $warnings = New-Object System.Collections.Generic.List[string]
  $check = Test-ClusterInventory -Inventory $Inventory
  if (-not $check.Ok) {
    foreach ($e in $check.Errors) { $errors.Add($e) }
    return [pscustomobject]@{ Ok = $false; Errors = @($errors); Warnings = @($warnings) }
  }

  if ($SkipLiveNetwork) {
    $warnings.Add("SkipLiveNetwork: TCP/SSH checks skipped")
    return [pscustomobject]@{ Ok = $true; Errors = @(); Warnings = @($warnings) }
  }

  foreach ($n in @($Inventory.nodes)) {
    $ip = [string]$n.ip
    $port = 22
    if ($null -ne $n.ssh_port) { $port = [int]$n.ssh_port }
    if (-not (Test-ClusterTcpPort -TargetHost $ip -Port $port)) {
      $errors.Add("TCP $ip`:$port not reachable")
      continue
    }
    if ($Inventory.ssh_mode -eq 'K') {
      $user = [string]$Inventory.ssh_user
      if ([string]::IsNullOrWhiteSpace($user)) { $user = 'datasafes3' }
      if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
        $errors.Add("ssh client not found for BatchMode check")
        continue
      }
      $sshArgs = @(
        '-o', 'BatchMode=yes',
        '-o', 'ConnectTimeout=5',
        '-o', 'StrictHostKeyChecking=yes',
        '-p', "$port",
        "${user}@${ip}",
        'true'
      )
      if ($IdentityFile -and (Test-Path -LiteralPath $IdentityFile)) {
        $sshArgs = @('-i', $IdentityFile) + $sshArgs
      }
      & ssh @sshArgs 2>$null
      if ($LASTEXITCODE -ne 0) {
        $errors.Add("SSH BatchMode failed for ${user}@${ip}:$port (fix keys / known_hosts)")
      }
    }
  }

  if ($Inventory.ssh_mode -eq 'P') {
    $warnings.Add("Mode P: password used only in-memory at bootstrap (Wave 2 Apply); Wave 1 does not send password")
  }

  return [pscustomobject]@{
    Ok       = ($errors.Count -eq 0)
    Errors   = @($errors)
    Warnings = @($warnings)
  }
}
