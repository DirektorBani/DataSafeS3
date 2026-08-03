#Requires -Version 5.1
<#
.SYNOPSIS
  Wave 2 Apply orchestrator. Default DryRun; -Apply pushes via cluster_push_apply.sh (Git Bash) or native SSH.
#>

. "$PSScriptRoot\ClusterRender.ps1"
. "$PSScriptRoot\ClusterSsh.ps1"

function New-ClusterApplySteps {
  param([Parameter(Mandatory)]$Plan)
  $steps = @(
    "1. Bootstrap datasafes3 + scoped sudoers + keepalived caps",
    "2. install-packages.sh (etcd, Patroni, Postgres, HAProxy, keepalived, NFS)",
    "3. Deploy etcd env + Patroni on all nodes",
    "4. NFSv4 export shard paths; mount on leader (C1)",
    "5. Deploy storage-server on leader (operator follow-up if image not on node)",
    "6. HAProxy multi-frontend on nodes",
    "7. keepalived VIPs (or DNS)",
    "8. health-gates.sh"
  )
  return $steps
}

function Get-ClusterBashExe {
  foreach ($c in @(
      "${env:ProgramFiles}\Git\bin\bash.exe",
      "${env:ProgramFiles(x86)}\Git\bin\bash.exe",
      "bash"
    )) {
    if ($c -eq 'bash') {
      $cmd = Get-Command bash -ErrorAction SilentlyContinue
      if ($cmd) { return $cmd.Source }
    } elseif (Test-Path -LiteralPath $c) {
      return $c
    }
  }
  return $null
}

function Invoke-ClusterApplyWave2 {
  param(
    [Parameter(Mandatory)]$Inventory,
    [string]$RepoRoot = "",
    [string]$VipS3 = "10.0.0.10",
    [string]$VipConsole = "10.0.0.11",
    [string]$VipPostgres = "10.0.0.12",
    [string]$LeaderIp = "",
    [ValidateSet('nfs','byo','sshfs')][string]$ShardMount = 'nfs',
    [string]$Interface = "eth0",
    [string]$IdentityFile = "",
    [switch]$DryRun,
    [switch]$Apply,
    [switch]$AllowEmptyVips,
    [switch]$SkipHealth
  )

  if ($Apply -and $DryRun) {
    throw "use either -DryRun or -Apply, not both"
  }
  if (-not $Apply) { $DryRun = $true }

  if (-not $RepoRoot) {
    $RepoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
  }

  $plan = Expand-ClusterPlan -Inventory $Inventory -VipS3 $VipS3 -VipConsole $VipConsole `
    -VipPostgres $VipPostgres -LeaderIp $LeaderIp -ShardMount $ShardMount -Interface $Interface

  if ($DryRun -and ([string]::IsNullOrWhiteSpace([string]$plan.vips.s3))) {
    $AllowEmptyVips = $true
  }

  $sshMode = [string]$Inventory.ssh_mode
  if ($Apply -and $sshMode -eq 'P' -and [string]::IsNullOrWhiteSpace($IdentityFile)) {
    throw "Apply with ssh_mode=P requires -IdentityFile after datasafes3 keys exist (password never on argv)"
  }

  $state = Get-ClusterStateDir
  $genId = "wave2-" + (Get-Date -Format 'yyyyMMddHHmmss')
  $outDir = Join-Path $state "generated\$genId"
  $secretsPath = Join-Path $state "secrets\cluster-secrets.json"

  if ($DryRun) {
    $secretsPath = Join-Path $env:TEMP "datasafe-cluster-secrets-dryrun.json"
    $secrets = Save-ClusterSecrets -Path $secretsPath -Force
  } else {
    $secrets = Save-ClusterSecrets -Path $secretsPath
  }

  try {
    $render = Invoke-ClusterRenderWave2 -Plan $plan -OutDir $outDir -Secrets $secrets `
      -RepoRoot $RepoRoot -AllowEmptyVips:$AllowEmptyVips -RedactSecrets:$DryRun

    $steps = New-ClusterApplySteps -Plan $plan
    $bash = Get-ClusterBashExe
    $pushScript = Join-Path $PSScriptRoot "cluster_push_apply.sh"
    $invPath = Join-Path $state "inventory-wave1.json"
    if (-not (Test-Path $invPath)) {
      Save-ClusterInventory -Inventory $Inventory -Path $invPath
    }

    $sshPlanItems = @()
    $user = [string]$Inventory.ssh_user
    if ([string]::IsNullOrWhiteSpace($user)) {
      $user = if ($sshMode -eq 'P') { 'root' } else { 'datasafes3' }
    }

    # Always record DryRun SSH plan lines for QA
    foreach ($ip in @($plan.node_ips)) {
      $r = Invoke-ClusterSsh -TargetHost $ip -User $user -SshMode $sshMode `
        -IdentityFile $IdentityFile -RemoteCommand "echo datasafe-w2-preflight" -DryRun
      $sshPlanItems += $r
    }

    if ($bash -and (Test-Path $pushScript)) {
      $bashArgs = @(
        $pushScript,
        "--bundle", ($outDir -replace '\\', '/'),
        "--inventory", ($invPath -replace '\\', '/'),
        "--leader-ip", [string]$plan.leader_ip,
        "--vip-mode", [string]$plan.vip_mode,
        "--interface", $Interface
      )
      if ($IdentityFile) { $bashArgs += @("--identity", ($IdentityFile -replace '\\', '/')) }
      if ($SkipHealth) { $bashArgs += "--skip-health" }
      if ($DryRun) { $bashArgs += "--dry-run" }

      Write-Host "  Using bash push: $bash" -ForegroundColor DarkGray
      & $bash $bashArgs
      if ($LASTEXITCODE -ne 0) { throw "cluster_push_apply.sh failed with exit $LASTEXITCODE" }
    } else {
      if ($Apply) {
        throw "Git Bash not found; install Git for Windows or run scripts/cluster/cluster_apply_w2.sh --apply from WSL"
      }
      Write-Host "  [!!] bash not found: SSH push DryRun lines only (native)" -ForegroundColor Yellow
    }

    if ($DryRun -and (Test-Path -LiteralPath $secretsPath)) {
      Remove-Item -LiteralPath $secretsPath -Force -ErrorAction SilentlyContinue
    }

    return [pscustomobject]@{
      Ok       = $true
      DryRun   = [bool]$DryRun
      OutDir   = $outDir
      Steps    = $steps
      SshPlan  = $sshPlanItems
      Warnings = @($render.Warnings)
      Plan     = $plan
    }
  } catch {
    if ($DryRun -and (Test-Path -LiteralPath $secretsPath)) {
      Remove-Item -LiteralPath $secretsPath -Force -ErrorAction SilentlyContinue
    }
    throw
  }
}
