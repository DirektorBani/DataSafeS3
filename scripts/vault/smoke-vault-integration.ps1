# Vault profile integration smoke (Windows).
# Usage:
#   $env:DATASAFE_DATA_ROOT = 'D:/datasafe-data'
#   pwsh -File scripts/vault/smoke-vault-integration.ps1
param(
    [switch]$NoCompose,
    [switch]$SkipPostgres
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
Set-Location $Root

$env:VAULT_PROFILE = '1'
if (-not $env:TEST_VAULT_ADDR) { $env:TEST_VAULT_ADDR = 'http://127.0.0.1:8200' }
if (-not $env:STORAGE_URL) { $env:STORAGE_URL = 'http://127.0.0.1:9000' }

$compose = @(
    'compose', '-p', 'datasafe-vault',
    '-f', 'docker-compose.yml',
    '-f', 'docker-compose.vault.yml'
)
if ($env:DATASAFE_DATA_ROOT) {
    $compose += @('-f', 'docker-compose.local-data.yml', '-f', 'docker-compose.local-binary.yml')
}
$profiles = @('--profile', 'vault')
if (-not $SkipPostgres) {
    $profiles += @('--profile', 'postgres')
    $env:STORAGE_METADATA_BACKEND = 'postgres'
}

if (-not $NoCompose) {
    Write-Host 'Starting Vault integration stack...'
    & docker @compose @profiles up -d --wait vault vault-init vault-agent
    if (-not $SkipPostgres) {
        & docker @compose @profiles up -d --wait postgres
    }
    & docker @compose @profiles up -d --wait storage-server
}

node scripts/vault/test-vault-integration.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
