# Start DataSafeS3 HA lab stack (Windows) — separate project/ports from main `datasafe` dev.
param(
    [switch]$SkipBuild,
    [switch]$FreshVolumes,
    [switch]$WithIntegrations,
    [switch]$WithClusterB
)
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

$Project = "datasafe-ha"
$EnvFile = Join-Path $Root ".env.ha"
if (-not (Test-Path $EnvFile)) {
    Copy-Item (Join-Path $Root "deploy/compose/env/.env.ha.example") $EnvFile
    Write-Host "Created $EnvFile from deploy/compose/env/.env.ha.example"
}

$DataRoot = "D:/datasafe-data-ha"
foreach ($line in Get-Content $EnvFile) {
    if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') { $DataRoot = $Matches[1].Trim(); break }
}
$DataRootWin = $DataRoot -replace '/', '\'
$StorageDirs = @("storage-primary", "storage-standby-1", "storage-standby-2")
foreach ($d in $StorageDirs) {
    New-Item -ItemType Directory -Force -Path "$DataRootWin\$d" | Out-Null
}
New-Item -ItemType Directory -Force -Path "$DataRootWin\postgres", "$DataRootWin\postgres-standby" | Out-Null

$hbaInit = Join-Path $Root "deploy\docker\postgres\init\02-replication-hba.sh"
$replScript = Join-Path $Root "deploy\docker\object-replicator.sh"
foreach ($sh in @($hbaInit, $replScript)) {
    if (-not (Test-Path $sh)) { continue }
    $raw = Get-Content -Raw $sh
    if ($raw -match "`r") {
        [System.IO.File]::WriteAllText($sh, ($raw -replace "`r`n", "`n" -replace "`r", "`n"), [System.Text.UTF8Encoding]::new($false))
    }
}

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
    if (-not $tok) { return }
    $status = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status"
    if ($status.initial_setup_completed) { return }
    Write-Host "[ha] Completing initial setup wizard..."
    $h = @{ Authorization = "Bearer $tok" }
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h -ContentType "application/json" -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/complete" -Method POST -Headers $h | Out-Null
    $r2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"Admin123!"}'
    if ($r2.token) {
        $h2 = @{ Authorization = "Bearer $($r2.token)" }
        Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h2 -ContentType "application/json" -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
    }
}

$Compose = @(
    "compose", "-p", $Project,
    "--profile", "postgres",
    "--profile", "ha-postgres",
    "--profile", "ha-standby",
    "-f", "docker-compose.yml",
    "-f", "docker-compose.local-data.yml",
    "-f", "deploy/compose/docker-compose.local-binary.yml",
    "-f", "deploy/compose/docker-compose.ha.yml",
    "-f", "deploy/compose/docker-compose.ha-local.yml",
    "--env-file", ".env.ha"
)

if ($FreshVolumes) {
    Write-Host "[ha] Stopping and removing HA volumes..."
    & docker @Compose down -v --remove-orphans
    Remove-Item -Recurse -Force "$DataRootWin\storage-primary\*", "$DataRootWin\storage-standby-1\*", "$DataRootWin\storage-standby-2\*", "$DataRootWin\postgres\*", "$DataRootWin\postgres-standby\*" -ErrorAction SilentlyContinue
}

if (-not $SkipBuild) {
    Write-Host "[ha] Building linux storage-server binary..."
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags="-s -w" -o deploy/docker/storage-server-linux ./cmd/storage-server
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    if (Test-Path "web\console\package.json") {
        if (-not (Test-Path "web\console\dist\index.html")) {
            Write-Host "[ha] Building console (first time)..."
            Push-Location web\console
            npm ci 2>$null; if ($LASTEXITCODE -ne 0) { npm install }
            npm run build
            Pop-Location
        }
    }
    Write-Host "[ha] Fixing storage dir ownership for UID 65532..."
    foreach ($d in $StorageDirs) {
        docker run --rm -v "${DataRoot}/${d}:/data" --user root alpine:3.20 chown -R 65532:65532 /data 2>$null | Out-Null
    }
}

$env:STORAGE_RATE_LIMIT_LOGIN = "500"
Write-Host "[ha] Starting HA stack (2x postgres, 3x storage-server, object-replicator)..."
& docker @Compose up -d postgres postgres-standby storage-server object-replicator storage-server-standby storage-server-standby-2 caddy prometheus grafana
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$BaseUrl = "http://127.0.0.1:8082"
$StandbyUrls = @("http://127.0.0.1:9001", "http://127.0.0.1:9003")
Write-Host "[ha] Waiting for primary console $BaseUrl ..."
for ($i = 1; $i -le 90; $i++) {
    try {
        $h = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 3
        if ($h.status -eq "ok") { break }
    } catch {}
    if ($i -eq 90) { Write-Error "Primary healthz timeout"; exit 1 }
    Start-Sleep -Seconds 2
}

Complete-InitialSetup -BaseUrl $BaseUrl

foreach ($StandbyUrl in $StandbyUrls) {
    Write-Host "[ha] Waiting for standby $StandbyUrl ..."
    for ($i = 1; $i -le 60; $i++) {
        try {
            $h = Invoke-RestMethod -Uri "$StandbyUrl/healthz" -TimeoutSec 3
            if ($h.status -eq "ok") { break }
        } catch {}
        if ($i -eq 60) { Write-Error "Standby healthz timeout: $StandbyUrl"; exit 1 }
        Start-Sleep -Seconds 2
    }
}

Write-Host ""
Write-Host "[ha] Stack ready:"
Write-Host "  Primary console:  $BaseUrl  (admin/admin)"
Write-Host "  Primary S3/API:   http://127.0.0.1:9002"
Write-Host "  Standby read API: $($StandbyUrls -join ', ')"
Write-Host "  Postgres primary: localhost:5442"
Write-Host "  Postgres standby: localhost:5443"
Write-Host ""
Write-Host "Run: scripts\ha\test-ha-cluster.ps1"
if ($WithIntegrations) {
    Write-Host "[ha] Configuring Gateway (external S3) + Federation peers..."
    & (Join-Path $PSScriptRoot "setup-ha-integrations.ps1") -ConsoleUrl $BaseUrl -SkipMinioStart
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
if ($WithClusterB) {
    Write-Host "[ha] Starting remote cluster B + federation registration..."
    & (Join-Path $PSScriptRoot "start-ha-cluster-b.ps1") -SkipBuild -ConnectToHaNetwork
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & (Join-Path $PSScriptRoot "setup-ha-integrations.ps1") -ConsoleUrl $BaseUrl -SkipMinioStart -RegisterClusterB
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
