# Start erasure-backend lab stack (single storage-server + 6 shard volumes).
param(
    [switch]$SkipBuild,
    [switch]$FreshVolumes
)
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

$Project = "datasafe-erasure"
$EnvFile = Join-Path $Root ".env.erasure"
if (-not (Test-Path $EnvFile)) {
    $example = Join-Path $Root "deploy/compose/env/.env.erasure.example"
    if (Test-Path $example) {
        Copy-Item $example $EnvFile
    } elseif (Test-Path (Join-Path $Root "deploy/compose/env/.env.ha.example")) {
        Copy-Item (Join-Path $Root "deploy/compose/env/.env.ha.example") $EnvFile
    }
    Write-Host "[erasure] Created $EnvFile"
}

$DataRoot = "D:/datasafe-data-erasure"
foreach ($line in Get-Content $EnvFile) {
    if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') { $DataRoot = $Matches[1].Trim(); break }
}
$DataRootWin = $DataRoot -replace '/', '\'
New-Item -ItemType Directory -Force -Path "$DataRootWin\storage", "$DataRootWin\postgres" | Out-Null

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
    Write-Host "[erasure] Completing initial setup wizard..."
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
    "-f", "docker-compose.yml",
    "-f", "docker-compose.local-data.yml",
    "-f", "deploy/compose/docker-compose.local-binary.yml",
    "-f", "deploy/compose/docker-compose.ha-erasure.yml",
    "--env-file", ".env.erasure"
)

if ($FreshVolumes) {
    Write-Host "[erasure] Stopping and removing erasure volumes..."
    & docker @Compose down -v --remove-orphans
    Remove-Item -Recurse -Force "$DataRootWin\storage\*", "$DataRootWin\postgres\*" -ErrorAction SilentlyContinue
}

if (-not $SkipBuild) {
    Write-Host "[erasure] Building linux storage-server binary..."
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags="-s -w" -o deploy/docker/storage-server-linux ./cmd/storage-server
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    docker run --rm -v "${DataRoot}/storage:/data" --user root alpine:3.20 chown -R 65532:65532 /data 2>$null | Out-Null
}

$env:STORAGE_RATE_LIMIT_LOGIN = "500"
Write-Host "[erasure] Starting erasure stack..."
& docker @Compose up -d postgres storage-server caddy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$BaseUrl = "http://127.0.0.1:8082"
Write-Host "[erasure] Waiting for $BaseUrl ..."
for ($i = 1; $i -le 90; $i++) {
    try {
        $h = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 3
        if ($h.status -eq "ok") { break }
    } catch {}
    if ($i -eq 90) { Write-Error "Erasure stack healthz timeout"; exit 1 }
    Start-Sleep -Seconds 2
}

Complete-InitialSetup -BaseUrl $BaseUrl

Write-Host ""
Write-Host "[erasure] Stack ready: $BaseUrl (admin/admin), object_backend=erasure"
Write-Host "Run: scripts\ha\test-erasure-backend.ps1"
