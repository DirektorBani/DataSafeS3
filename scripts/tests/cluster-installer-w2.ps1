#Requires -Version 5.1
<#
.SYNOPSIS
  Wave 2 assert script: plan, render, security gate, DryRun Apply (no live SSH).
.EXAMPLE
  powershell -NoProfile -File scripts\tests\cluster-installer-w2.ps1
#>
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $Root
. ".\scripts\cluster\ClusterInventory.ps1"
. ".\scripts\cluster\ClusterPlan.ps1"
. ".\scripts\cluster\ClusterRender.ps1"
. ".\scripts\cluster\ClusterApply.ps1"
. ".\scripts\cluster\ClusterSsh.ps1"

$failed = 0
function Assert-True([bool]$cond, [string]$msg) {
  if ($cond) { Write-Host "  PASS $msg" -ForegroundColor Green }
  else { Write-Host "  FAIL $msg" -ForegroundColor Red; $script:failed++ }
}

Write-Host "=== cluster-installer-w2 ==="

$inv = New-ClusterInventorySkeleton -Ips @('10.0.0.1','10.0.0.2','10.0.0.3') -SshMode K -SshUser datasafes3
$inv.layout = 'production'
$inv.vip_mode = 'subnet'

$plan = Expand-ClusterPlan -Inventory $inv -VipS3 '10.0.0.10' -VipConsole '10.0.0.11' -VipPostgres '10.0.0.12'
Assert-True ($plan.shards.Count -eq 6) "4+2 plan has 6 shards"
Assert-True ($plan.leader_ip -eq '10.0.0.1') "default leader is first node"

$pc = Test-ClusterPlan -Plan $plan
Assert-True $pc.Ok "plan validation ok"

$devInv = New-ClusterInventorySkeleton -Ips @('10.0.0.1','10.0.0.2','10.0.0.3') -SshMode P
$devInv.layout = 'dev'
$devPlan = Expand-ClusterPlan -Inventory $devInv -VipS3 '10.0.0.10' -VipConsole '10.0.0.11' -VipPostgres '10.0.0.12'
Assert-True ($devPlan.shards.Count -eq 3) "2+1 plan has 3 shards"

$outDir = Join-Path $env:TEMP "datasafe-cluster-w2-render"
$secPath = Join-Path $env:TEMP "datasafe-cluster-w2-secrets.json"
$secrets = Save-ClusterSecrets -Path $secPath -Force
$render = Invoke-ClusterRenderWave2 -Plan $plan -OutDir $outDir -Secrets $secrets -RepoRoot $Root -RedactSecrets
Assert-True $render.Ok "render ok"
Assert-True (Test-Path (Join-Path $outDir "lb\haproxy.cfg")) "haproxy.cfg rendered"
Assert-True (Test-Path (Join-Path $outDir "lb\keepalived-s3.conf")) "keepalived-s3 rendered"
Assert-True (Test-Path (Join-Path $outDir "nodes\10.0.0.1\patroni.yml")) "patroni.yml rendered"
Assert-True (Test-Path (Join-Path $outDir "nodes\10.0.0.1\etcd.env")) "etcd.env rendered"
Assert-True (Test-Path (Join-Path $outDir "nfs\mount-shards-leader.sh")) "nfs mount script rendered"
Assert-True (Test-Path (Join-Path $outDir "bootstrap\sudoers-datasafes3")) "sudoers template copied"

$hap = Get-Content -Raw (Join-Path $outDir "lb\haproxy.cfg")
Assert-True ($hap -match 'frontend fe_s3') "haproxy has fe_s3"
Assert-True ($hap -match 'frontend fe_console') "haproxy has fe_console"
Assert-True ($hap -match 'frontend fe_postgres') "haproxy has fe_postgres"
Assert-True ($hap -match '10\.0\.0\.1:9000') "haproxy S3 -> leader"

$exports = Get-ChildItem (Join-Path $outDir "nfs") -Filter "exports.*"
Assert-True ($exports.Count -ge 1) "nfs exports files present"
$expText = ($exports | ForEach-Object { Get-Content -Raw $_.FullName }) -join "`n"
Assert-True ($expText -notmatch '(?m)\s\*(\s|$)') "nfs exports have no *"
Assert-True ($expText -notmatch '0\.0\.0\.0/0') "nfs exports have no 0.0.0.0/0"

$sudo = Get-Content -Raw (Join-Path $outDir "bootstrap\sudoers-datasafes3")
Assert-True ($sudo -notmatch 'NOPASSWD:\s*ALL') "sudoers not NOPASSWD:ALL"

$pat = Get-Content -Raw (Join-Path $outDir "nodes\10.0.0.1\patroni.yml")
Assert-True ($pat -notmatch '0\.0\.0\.0/0') "patroni pg_hba not world-open"
Assert-True ($pat -match 'REDACTED_PG_SUPER') "DryRun redacts pg super password"

$gate = Test-ClusterRenderedSecurity -OutDir $outDir
Assert-True $gate.Ok "security gate on render tree"

# evil: inject world export and expect gate fail
$evilDir = Join-Path $env:TEMP "datasafe-cluster-w2-evil"
New-Item -ItemType Directory -Force -Path $evilDir | Out-Null
Set-Content -LiteralPath (Join-Path $evilDir "exports.evil") -Value "/data *(rw,sync)" -Encoding utf8
$bad = Test-ClusterRenderedSecurity -OutDir $evilDir
Assert-True (-not $bad.Ok) "security gate rejects NFS *"

$ssh = Invoke-ClusterSsh -TargetHost '10.0.0.1' -User datasafes3 -SshMode K -RemoteCommand 'true' -DryRun
Assert-True $ssh.DryRun "ssh DryRun ok"
Assert-True ($ssh.Command -notmatch '(?i)password') "ssh plan has no password"

$apply = Invoke-ClusterApplyWave2 -Inventory $inv -RepoRoot $Root -DryRun `
  -VipS3 '10.0.0.10' -VipConsole '10.0.0.11' -VipPostgres '10.0.0.12'
Assert-True $apply.Ok "Apply DryRun ok"
Assert-True ($apply.Steps.Count -ge 6) "Apply has step plan"
Assert-True (Test-Path $apply.OutDir) "Apply wrote generated dir"

# push script present
Assert-True (Test-Path ".\scripts\cluster\cluster_push_apply.sh") "cluster_push_apply.sh present"
Assert-True (Test-Path ".\scripts\cluster\remote\apply-node.sh") "apply-node.sh present"
Assert-True (Test-Path ".\scripts\cluster\remote\install-packages.sh") "install-packages.sh present"

# mode P without identity must throw
$invP = New-ClusterInventorySkeleton -Ips @('10.0.0.1','10.0.0.2','10.0.0.3') -SshMode P
$threw = $false
try {
  [void](Invoke-ClusterApplyWave2 -Inventory $invP -RepoRoot $Root -Apply -SkipHealth)
} catch {
  $threw = $true
}
Assert-True $threw "Apply mode P without IdentityFile refused"

# cleanup
Remove-Item $outDir,$evilDir,$secPath -Recurse -Force -ErrorAction SilentlyContinue
if ($apply.OutDir -and (Test-Path $apply.OutDir)) {
  Remove-Item $apply.OutDir -Recurse -Force -ErrorAction SilentlyContinue
}

Assert-True (Test-Path ".\deploy\cluster\templates\haproxy\haproxy.cfg.tmpl") "template pack present"
Assert-True (Test-Path ".\scripts\cluster\SECURITY.md") "SECURITY.md present"
Assert-True (Test-Path ".\deploy\docker\grafana\dashboards\datasafe-cluster.json") "grafana cluster dashboard"
Assert-True (Test-Path ".\scripts\cluster\bootstrap_keys_p.sh") "bootstrap_keys_p.sh present"
Assert-True ((Get-Content ".\scripts\cluster\remote\apply-node.sh" -Raw) -match 'last-apply\.env') "apply-node idempotent stamp"

if ($failed -gt 0) {
  Write-Host "FAILED: $failed" -ForegroundColor Red
  exit 1
}
Write-Host "ALL PASS" -ForegroundColor Green
exit 0
