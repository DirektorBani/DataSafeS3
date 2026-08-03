# Start two-stack site replication lab (site A = source, site B = peer).
param(
    [switch]$SkipBuild,
    [switch]$FreshVolumes
)
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

foreach ($pair in @(
    @{ File = ".env.site-a"; Example = "deploy/compose/env/.env.site-a.example" },
    @{ File = ".env.site-b"; Example = "deploy/compose/env/.env.site-b.example" }
)) {
    if (-not (Test-Path $pair.File)) {
        Copy-Item $pair.Example $pair.File
        Write-Host "[site-repl] Created $($pair.File) from $($pair.Example)"
    }
}

foreach ($line in Get-Content ".env.site-a") {
    if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') {
        $rootA = ($Matches[1].Trim() -replace '/', '\')
        New-Item -ItemType Directory -Force -Path "$rootA\storage", "$rootA\postgres" | Out-Null
        break
    }
}
foreach ($line in Get-Content ".env.site-b") {
    if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') {
        $rootB = ($Matches[1].Trim() -replace '/', '\')
        New-Item -ItemType Directory -Force -Path "$rootB\storage", "$rootB\postgres" | Out-Null
        break
    }
}

function ComposeArgs([string]$Project, [string]$EnvFile, [string[]]$ExtraFiles) {
    $args = @(
        "compose", "-p", $Project,
        "--profile", "postgres",
        "-f", "docker-compose.yml",
        "-f", "docker-compose.local-data.yml",
        "-f", "deploy/compose/docker-compose.local-binary.yml"
    )
    foreach ($ef in $ExtraFiles) { $args += @("-f", $ef) }
    $args += @("--env-file", $EnvFile)
    return ,$args
}

$ComposeA = ComposeArgs "datasafe-a" ".env.site-a" @("deploy/compose/docker-compose.site-repl-lab.yml")
$ComposeB = ComposeArgs "datasafe-b" ".env.site-b" @("deploy/compose/docker-compose.site-b.yml")

if ($FreshVolumes) {
    & docker @ComposeA down -v --remove-orphans
    & docker @ComposeB down -v --remove-orphans
    foreach ($line in Get-Content ".env.site-a") {
        if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') {
            $rootA = ($Matches[1].Trim() -replace '/', '\')
            Remove-Item -Recurse -Force "$rootA\storage\*", "$rootA\postgres\*" -ErrorAction SilentlyContinue
            break
        }
    }
    foreach ($line in Get-Content ".env.site-b") {
        if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') {
            $rootB = ($Matches[1].Trim() -replace '/', '\')
            Remove-Item -Recurse -Force "$rootB\storage\*", "$rootB\postgres\*" -ErrorAction SilentlyContinue
            break
        }
    }
}

if (-not $SkipBuild) {
    Write-Host "[site-repl] Building linux storage-server binary..."
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags="-s -w" -o deploy/docker/storage-server-linux ./cmd/storage-server
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

$env:STORAGE_RATE_LIMIT_LOGIN = "500"
Write-Host "[site-repl] Starting site B (peer)..."
& docker @ComposeB up -d postgres storage-server caddy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "[site-repl] Starting site A (source + replication worker)..."
& docker @ComposeA up -d postgres storage-server caddy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

function Complete-InitialSetup {
    param([string]$BaseUrl)
    $tok = $null
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
            if ($r.token) { $tok = $r.token; break }
        } catch {}
        Start-Sleep -Seconds 2
    }
    if (-not $tok) { throw "login failed for $BaseUrl" }
    $status = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status"
    if ($status.initial_setup_completed) { return }
    Write-Host "[site-repl] Completing initial setup on $BaseUrl ..."
    $h = @{ Authorization = "Bearer $tok" }
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h -ContentType "application/json" -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/complete" -Method POST -Headers $h | Out-Null
    $r2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"Admin123!"}'
    if ($r2.token) {
        $h2 = @{ Authorization = "Bearer $($r2.token)" }
        Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h2 -ContentType "application/json" -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
    }
}

$BaseA = "http://127.0.0.1:8082"
$BaseB = "http://127.0.0.1:9082"
foreach ($pair in @(@($BaseA, "A"), @($BaseB, "B"))) {
    $url = $pair[0]; $label = $pair[1]
    Write-Host "[site-repl] Waiting for site $label $url ..."
    for ($i = 1; $i -le 60; $i++) {
        try {
            $h = Invoke-RestMethod -Uri "$url/healthz" -TimeoutSec 3
            if ($h.status -eq "ok") { break }
        } catch {}
        if ($i -eq 60) { Write-Error "Site $label healthz timeout"; exit 1 }
        Start-Sleep -Seconds 2
    }
    Complete-InitialSetup -BaseUrl $url
}

Write-Host ""
Write-Host "[site-repl] Lab ready:"
Write-Host "  Site A (source):  $BaseA  S3 direct http://127.0.0.1:9002"
Write-Host "  Site B (peer):    $BaseB  S3 direct http://127.0.0.1:9193"
Write-Host ""
Write-Host "Run: scripts\ha\test-site-replication.ps1"
