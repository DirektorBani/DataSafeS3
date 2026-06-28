# РЎР±СЂРѕСЃ DataSafeS3 Рє СЃРѕСЃС‚РѕСЏРЅРёСЋ В«С‡РёСЃС‚Р°СЏ СѓСЃС‚Р°РЅРѕРІРєР°В»
#
# РЈРґР°Р»СЏРµС‚ РјРµС‚Р°РґР°РЅРЅС‹Рµ (BoltDB РёР»Рё С‚РѕРј PostgreSQL), РѕР±СЉРµРєС‚С‹ РЅР° РґРёСЃРєРµ Рё РїРµСЂРµР·Р°РїСѓСЃРєР°РµС‚ СЃС‚РµРє.
# РСЃРїРѕР»СЊР·СѓР№С‚Рµ РїРµСЂРµРґ РїСЂРѕРІРµСЂРєРѕР№ РјР°СЃС‚РµСЂР° РїРµСЂРІРёС‡РЅРѕР№ РЅР°СЃС‚СЂРѕР№РєРё (initial setup wizard).
#
# BoltDB (РїСЂРѕС„РёР»СЊ РїРѕ СѓРјРѕР»С‡Р°РЅРёСЋ):
#   - СѓРґР°Р»СЏРµС‚ ./data/metadata.db Рё ./data/objects/
#
# PostgreSQL (РїСЂРѕС„РёР»СЊ postgres):
#   - docker compose --profile postgres down -v  (С‚РѕРј postgres-data)
#   - Р·Р°С‚РµРј up Р·Р°РЅРѕРІРѕ
#
# РџСЂРёРјРµСЂС‹:
#   .\scripts\reset-fresh-install.ps1 -Postgres
#   .\scripts\reset-fresh-install.ps1 -DataDir .\data-local-test -Postgres -ProjectName datasafe
#
# Р”Р»СЏ local-binary overlay (Windows dev) overlay РїРѕРґРєР»СЋС‡Р°РµС‚СЃСЏ Р°РІС‚РѕРјР°С‚РёС‡РµСЃРєРё, РµСЃР»Рё РµСЃС‚СЊ
# docker-compose.local-binary.yml. РџСЂРѕРµРєС‚ Compose: -ProjectName РёР»Рё COMPOSE_PROJECT_NAME,
# РёРЅР°С‡Рµ РѕРїСЂРµРґРµР»СЏРµС‚СЃСЏ РїРѕ Р·Р°РїСѓС‰РµРЅРЅРѕРјСѓ storage-server, РёРЅР°С‡Рµ РёРјСЏ РёР· compose (datasafe).

param(
    [string]$DataDir = ".\data",
    [switch]$Postgres,
    [switch]$NoComposeDown,
    [string]$ProjectName = "",
    [switch]$NoLocalBinary
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not (Test-Path (Join-Path $root ".env")) -and (Test-Path (Join-Path $root ".env.example"))) {
    Copy-Item (Join-Path $root ".env.example") (Join-Path $root ".env")
}
if (-not $env:DATASAFE_DATA_ROOT) { $env:DATASAFE_DATA_ROOT = "D:/datasafe-data" }

Write-Host "==> DataSafeS3 fresh install reset" -ForegroundColor Cyan
Write-Host "    Data dir: $DataDir"

function Get-ComposeBaseArgs {
    param([string]$Project)
    $args = @("-f", "docker-compose.yml")
    $localData = Join-Path $root "docker-compose.local-data.yml"
    if (Test-Path $localData) {
        $args += @("-f", "docker-compose.local-data.yml")
    }
    $localBinary = Join-Path $root "docker-compose.local-binary.yml"
    if (-not $NoLocalBinary -and (Test-Path $localBinary)) {
        $args += @("-f", "docker-compose.local-binary.yml")
    }
    if ($Project) {
        $args = @("-p", $Project) + $args
    }
    return ,$args
}

function Resolve-ComposeProjectName {
    if ($ProjectName) { return $ProjectName }
    if ($env:COMPOSE_PROJECT_NAME) { return $env:COMPOSE_PROJECT_NAME }
    $lines = docker compose ls --format json 2>$null
    if ($LASTEXITCODE -eq 0 -and $lines) {
        foreach ($line in ($lines | Where-Object { $_ })) {
            try {
                $row = $line | ConvertFrom-Json
                if ($row.Name -and ($row.Status -match "running")) {
                    return $row.Name
                }
            } catch { }
        }
    }
    return "datasafe"
}

$composeProject = Resolve-ComposeProjectName
$composeBase = Get-ComposeBaseArgs -Project $composeProject
Write-Host "    Compose project: $composeProject"

if (-not $NoComposeDown) {
    if ($Postgres) {
        Write-Host "==> Stopping compose stack (postgres profile) and removing volumes..."
        docker compose @composeBase --profile postgres down -v
    } else {
        Write-Host "==> Stopping compose stack..."
        docker compose @composeBase down -v
    }
}

$resolvedData = Resolve-Path -ErrorAction SilentlyContinue $DataDir
if (-not $resolvedData) {
    $resolvedData = Join-Path $root ($DataDir -replace '^\./', '')
}

if (Test-Path $resolvedData) {
    $meta = Join-Path $resolvedData "metadata.db"
    $objects = Join-Path $resolvedData "objects"
    if (Test-Path $meta) {
        Remove-Item -Force $meta
        Write-Host "    Removed $meta"
    }
    if (Test-Path $objects) {
        Remove-Item -Recurse -Force $objects
        Write-Host "    Removed $objects"
    }
} else {
    Write-Host "    Data directory not found (ok for first run): $resolvedData"
}

if ($Postgres) {
    Write-Host "==> Starting stack with postgres profile..."
    docker compose @composeBase --profile postgres up -d --build
    if ($LASTEXITCODE -ne 0) {
        Write-Host "    Build failed; starting without rebuild (local-binary overlay)..." -ForegroundColor Yellow
        docker compose @composeBase --profile postgres up -d
    }
} else {
    Write-Host "==> Starting stack..."
    docker compose @composeBase up -d --build
    if ($LASTEXITCODE -ne 0) {
        Write-Host "    Build failed; starting without rebuild..." -ForegroundColor Yellow
        docker compose @composeBase up -d
    }
}

function Invoke-SetupCurl {
    param([string]$Method, [string]$Url, [string]$Token = "", [string]$Body = $null)
    $args = @('-s', '-w', "`n%{http_code}", '-X', $Method, $Url)
    if ($Token) { $args += @('-H', "Authorization: Bearer $Token") }
    $tmp = $null
    if ($null -ne $Body) {
        $tmp = [System.IO.Path]::GetTempFileName()
        [System.IO.File]::WriteAllText($tmp, $Body, [System.Text.UTF8Encoding]::new($false))
        $args += @('-H', 'Content-Type: application/json', '--data-binary', "@$tmp")
    }
    try {
        return (& curl.exe @args | Out-String).Trim()
    } finally {
        if ($tmp) { Remove-Item -Force -ErrorAction SilentlyContinue $tmp }
    }
}

function Wait-AndCompleteSetup {
    param([string]$BaseUrl = "http://localhost:8080")
    Write-Host "==> Waiting for API and completing initial setup (audit-ready)..."
    $tok = $null
    for ($i = 0; $i -lt 45; $i++) {
        $out = Invoke-SetupCurl POST "$BaseUrl/api/v1/admin/login" -Body '{"username":"admin","password":"admin"}'
        $lines = $out -split "`n"
        if ([int]$lines[-1] -eq 200) {
            $text = ($lines[0..($lines.Count-2)] -join "`n")
            $tok = ($text | ConvertFrom-Json).token
            if ($tok) { break }
        }
        Start-Sleep -Seconds 2
    }
    if (-not $tok) {
        Write-Host "    API not ready; complete setup manually at $BaseUrl" -ForegroundColor Yellow
        return
    }

    $status = (Invoke-SetupCurl GET "$BaseUrl/api/v1/setup/status") -split "`n" | Select-Object -First 1 | ConvertFrom-Json
    if ($status.initial_setup_completed) {
        Write-Host "    Setup already completed."
        return
    }

    Invoke-SetupCurl POST "$BaseUrl/api/v1/me/password" -Token $tok -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
    Invoke-SetupCurl POST "$BaseUrl/api/v1/setup/complete" -Token $tok | Out-Null

    $out2 = Invoke-SetupCurl POST "$BaseUrl/api/v1/admin/login" -Body '{"username":"admin","password":"Admin123!"}'
    $lines2 = $out2 -split "`n"
    $tok2 = (($lines2[0..($lines2.Count-2)] -join "`n") | ConvertFrom-Json).token
    if ($tok2) {
        Invoke-SetupCurl POST "$BaseUrl/api/v1/me/password" -Token $tok2 -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
    }
    Write-Host "    Initial setup completed; admin password restored to admin (dev)."
}

Wait-AndCompleteSetup

Write-Host ""
Write-Host "Done. Open http://localhost:8080 - login admin/admin (setup complete)." -ForegroundColor Green
