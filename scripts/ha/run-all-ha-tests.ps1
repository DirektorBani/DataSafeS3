# Unified HA / erasure / site-replication test runner (Windows lab).
param(
    [switch]$FreshVolumes,
    [switch]$SkipBuild,
    [switch]$SkipGoTests,
    [switch]$KeepStacks
)
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

$matrix = [System.Collections.Generic.List[object]]::new()
$failCount = 0

function Add-Result($Scenario, $Script, $Status, $Notes = "") {
    $script:matrix.Add([PSCustomObject]@{
        Scenario = $Scenario; Script = $Script; Status = $Status; Notes = $Notes
    })
    if ($Status -eq "FAIL") { $script:failCount++ }
    $icon = switch ($Status) { "PASS" { "[PASS]" } "SKIP" { "[SKIP]" } default { "[FAIL]" } }
    Write-Host "$icon $Scenario :: $Script $(if ($Notes) { "- $Notes" })"
}

function Run-Step($Scenario, $ScriptPath, [string[]]$ScriptArgs = @()) {
    $name = Split-Path $ScriptPath -Leaf
    Write-Host ""
    Write-Host "========== $Scenario ($name) =========="
    try {
        & $ScriptPath @ScriptArgs
        if ($LASTEXITCODE -ne 0) {
            Add-Result $Scenario $name "FAIL" "exit=$LASTEXITCODE"
            return $false
        }
        Add-Result $Scenario $name "PASS"
        return $true
    } catch {
        Add-Result $Scenario $name "FAIL" $_.Exception.Message
        return $false
    }
}

function Stop-Project($Project) {
    Write-Host "[cleanup] docker compose -p $Project down"
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    try {
        docker compose -p $Project down --remove-orphans 2>&1 | Out-Null
    } finally {
        $ErrorActionPreference = $prev
    }
}

Write-Host "=== DataSafe HA test suite ==="
Write-Host "FreshVolumes=$FreshVolumes SkipBuild=$SkipBuild"
Write-Host "Port note: HA/site-A use :8082/:9002; main dev stack uses :8080/:9000"
Write-Host ""

# Go unit tests
if (-not $SkipGoTests) {
    Write-Host "========== Go tests (ha + erasure) =========="
    go test ./internal/ha/... ./internal/storage/erasure/...
    if ($LASTEXITCODE -ne 0) {
        Add-Result "Go unit" "go test ha/erasure" "FAIL" "exit=$LASTEXITCODE"
    } else {
        Add-Result "Go unit" "go test ha/erasure" "PASS"
    }
}

$haDir = $PSScriptRoot
$fresh = @{}
if ($FreshVolumes) { $fresh["FreshVolumes"] = $true }
$skip = @{}
if ($SkipBuild) { $skip["SkipBuild"] = $true }

# A. HA cluster lab
Stop-Project "datasafe-erasure"
Stop-Project "datasafe-a"
Stop-Project "datasafe-b"
Run-Step "A HA cluster start" (Join-Path $haDir "start-ha-stack.ps1") (@($fresh.Keys | ForEach-Object { "-$_" }) + @($skip.Keys | ForEach-Object { "-$_" }))
Run-Step "A HA cluster test" (Join-Path $haDir "test-ha-cluster.ps1") @()

if (-not $KeepStacks) { Stop-Project "datasafe-ha" }

# B. Erasure backend
Run-Step "B Erasure stack start" (Join-Path $haDir "start-erasure-stack.ps1") (@($fresh.Keys | ForEach-Object { "-$_" }) + @($skip.Keys | ForEach-Object { "-$_" }))
Run-Step "B Erasure backend test" (Join-Path $haDir "test-erasure-backend.ps1") @()

if (-not $KeepStacks) { Stop-Project "datasafe-erasure" }

# C. Site replication two-stack
Run-Step "C Site repl lab start" (Join-Path $haDir "start-site-replication-lab.ps1") (@($fresh.Keys | ForEach-Object { "-$_" }) + @($skip.Keys | ForEach-Object { "-$_" }))
Run-Step "C Site replication E2E" (Join-Path $haDir "test-site-replication.ps1") @()

# F. Feature-audit site replication slice
Run-Step "F Feature-audit site repl" (Join-Path $haDir "test-feature-audit-site-repl.ps1") @()
Remove-Item Env:DATASAFE_SITE_REPL_E2E, Env:DATASAFE_SITE_REPL_PEER_CONSOLE, Env:DATASAFE_SITE_REPL_PEER_S3 -ErrorAction SilentlyContinue

if (-not $KeepStacks) {
    Stop-Project "datasafe-a"
    Stop-Project "datasafe-b"
}

Write-Host ""
Write-Host "=== HA test matrix ==="
$matrix | Format-Table -AutoSize
Write-Host "Total FAIL: $failCount"
if ($failCount -gt 0) { exit 1 }
exit 0
