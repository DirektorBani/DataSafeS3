#Requires -Version 5.1
<#
.SYNOPSIS
  SSH helpers for cluster Apply. Never put passwords on argv or in logs.
#>

function Invoke-ClusterSsh {
  <#
  .SYNOPSIS
    Run remote command. Mode K: BatchMode + identity. Mode P: requires SSH_ASKPASS setup by caller.
  #>
  param(
    [Parameter(Mandatory)][string]$TargetHost,
    [int]$Port = 22,
    [Parameter(Mandatory)][string]$User,
    [Parameter(Mandatory)][string]$RemoteCommand,
    [string]$IdentityFile = "",
    [ValidateSet('P','K')][string]$SshMode = 'K',
    [switch]$DryRun
  )
  $sshArgs = @(
    '-o', 'StrictHostKeyChecking=yes',
    '-o', 'ConnectTimeout=10',
    '-p', "$Port",
    "${User}@${TargetHost}",
    $RemoteCommand
  )
  if ($SshMode -eq 'K') {
    $sshArgs = @('-o', 'BatchMode=yes') + $sshArgs
  }
  if ($IdentityFile -and (Test-Path -LiteralPath $IdentityFile)) {
    $sshArgs = @('-i', $IdentityFile) + $sshArgs
  }
  if ($DryRun) {
    # Redact nothing needed - password never on argv
    return [pscustomobject]@{
      Ok      = $true
      DryRun  = $true
      Command = ("ssh " + ($sshArgs -join ' '))
      ExitCode = 0
    }
  }
  if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
    throw "ssh client not found"
  }
  $prev = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  & ssh @sshArgs
  $code = $LASTEXITCODE
  $ErrorActionPreference = $prev
  return [pscustomobject]@{
    Ok       = ($code -eq 0)
    DryRun   = $false
    Command  = ("ssh " + ($sshArgs -join ' '))
    ExitCode = $code
  }
}

function Copy-ClusterSshFile {
  param(
    [Parameter(Mandatory)][string]$LocalPath,
    [Parameter(Mandatory)][string]$TargetHost,
    [int]$Port = 22,
    [Parameter(Mandatory)][string]$User,
    [Parameter(Mandatory)][string]$RemotePath,
    [string]$IdentityFile = "",
    [ValidateSet('P','K')][string]$SshMode = 'K',
    [switch]$DryRun
  )
  $scpArgs = @(
    '-o', 'StrictHostKeyChecking=yes',
    '-P', "$Port",
    $LocalPath,
    "${User}@${TargetHost}:${RemotePath}"
  )
  if ($SshMode -eq 'K') {
    $scpArgs = @('-o', 'BatchMode=yes') + $scpArgs
  }
  if ($IdentityFile -and (Test-Path -LiteralPath $IdentityFile)) {
    $scpArgs = @('-i', $IdentityFile) + $scpArgs
  }
  if ($DryRun) {
    return [pscustomobject]@{ Ok = $true; DryRun = $true; Command = ("scp " + ($scpArgs -join ' ')) }
  }
  & scp @scpArgs
  return [pscustomobject]@{ Ok = ($LASTEXITCODE -eq 0); DryRun = $false; ExitCode = $LASTEXITCODE }
}
