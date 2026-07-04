# Second DataSafe cluster for HA lab federation (project datasafe-b, ports 9082/9193).
param(
    [switch]$SkipBuild,
    [switch]$FreshVolumes,
    [switch]$ConnectToHaNetwork
)
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

$Project = "datasafe-b"
$EnvFile = ".env.site-b"
if (-not (Test-Path $EnvFile)) {
    Copy-Item ".env.site-b.example" $EnvFile
    Write-Host "[ha-cluster-b] Created $EnvFile from example"
}

$DataRoot = "D:/datasafe-data-site-b"
foreach ($line in Get-Content $EnvFile) {
    if ($line -match '^\s*DATASAFE_DATA_ROOT=(.+)$') { $DataRoot = $Matches[1].Trim(); break }
}
$DataRootWin = $DataRoot -replace '/', '\'
New-Item -ItemType Directory -Force -Path "$DataRootWin\storage", "$DataRootWin\postgres" | Out-Null

$Compose = @(
    "compose", "-p", $Project,
    "--profile", "postgres",
    "-f", "docker-compose.yml",
    "-f", "docker-compose.local-data.yml",
    "-f", "docker-compose.local-binary.yml",
    "-f", "docker-compose.site-b.yml",
    "--env-file", $EnvFile
)

if ($FreshVolumes) {
    Write-Host "[ha-cluster-b] Fresh volumes..."
    & docker @Compose down -v --remove-orphans
    Remove-Item -Recurse -Force "$DataRootWin\storage\*", "$DataRootWin\postgres\*" -ErrorAction SilentlyContinue
}

if (-not $SkipBuild) {
    Write-Host "[ha-cluster-b] Building linux storage-server binary..."
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags="-s -w" -o deploy/docker/storage-server-linux ./cmd/storage-server
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

$env:DATASAFE_SERVER_IMAGE = "ghcr.io/direktorbani/datasafe-storage-server:v1.0.3"
$env:STORAGE_RATE_LIMIT_LOGIN = "500"
$env:HTTP_PROXY = ""; $env:HTTPS_PROXY = ""

Write-Host "[ha-cluster-b] Starting cluster B (postgres, storage-server, caddy)..."
& docker @Compose up -d postgres storage-server caddy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$ConsoleUrl = "http://127.0.0.1:9082"
$ApiUrl = "http://127.0.0.1:9193"
Write-Host "[ha-cluster-b] Waiting for $ConsoleUrl ..."
for ($i = 1; $i -le 90; $i++) {
    try {
        $h = Invoke-RestMethod -Uri "$ConsoleUrl/healthz" -TimeoutSec 3
        if ($h.status -eq "ok") { break }
    } catch {}
    if ($i -eq 90) { Write-Error "Cluster B healthz timeout"; exit 1 }
    Start-Sleep -Seconds 2
}

# Setup wizard if needed
try {
    $login = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
    if ($login.token) {
        $status = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/setup/status"
        if (-not $status.initial_setup_completed) {
            Write-Host "[ha-cluster-b] Completing initial setup..."
            $h = @{ Authorization = "Bearer $($login.token)" }
            Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/me/password" -Method POST -Headers $h -ContentType "application/json" -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
            Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/setup/complete" -Method POST -Headers $h | Out-Null
            $login2 = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"Admin123!"}'
            if ($login2.token) {
                $h2 = @{ Authorization = "Bearer $($login2.token)" }
                Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/me/password" -Method POST -Headers $h2 -ContentType "application/json" -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
            }
        }
    }
} catch {
    Write-Host "[ha-cluster-b] Setup wizard skip: $($_.Exception.Message)"
}

if ($ConnectToHaNetwork) {
    $haNet = "datasafe-ha_default"
    $container = "${Project}-storage-server-1"
    docker network inspect $haNet 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
        $inspect = docker inspect $container 2>$null | ConvertFrom-Json
        $onNet = $false
        if ($inspect -and $inspect[0].NetworkSettings.Networks.PSObject.Properties.Name -contains $haNet) {
            $onNet = $true
        }
        if (-not $onNet) {
            Write-Host "[ha-cluster-b] Connecting $container to $haNet ..."
            docker network connect $haNet $container
        }
    }
}

Write-Host ""
Write-Host "[ha-cluster-b] Cluster B ready:"
Write-Host "  Console:  $ConsoleUrl  (admin/admin)"
Write-Host "  S3/API:   $ApiUrl"
Write-Host "  Postgres: localhost:5542"
Write-Host "  Project:  $Project"
Write-Host ""
Write-Host "Register in HA Federation: scripts\ha\setup-ha-integrations.ps1 -RegisterClusterB"
