# Bootstrap HashiCorp Vault (dev mode) and seed KV-v2 secrets for DataSafeS3 local testing.
# Run from repository root. Requires Docker.
param(
    [string]$ComposeProject = "datasafe"
)
$ErrorActionPreference = "Stop"

$Root = Split-Path (Split-Path (Split-Path $PSScriptRoot -Parent) -Parent) -Parent
Set-Location $Root

$compose = @(
    "compose", "-p", $ComposeProject,
    "-f", "docker-compose.yml",
    "-f", "docker-compose.vault.yml"
)
if (Test-Path "docker-compose.local-data.yml") {
    $compose += @("-f", "docker-compose.local-data.yml")
}

function Write-Step([string]$msg) { Write-Host "[vault-dev] $msg" }

Write-Step "Starting Vault (dev mode)..."
& docker @compose --profile vault up -d vault

Write-Step "Seeding KV-v2 secrets (vault-init)..."
& docker @compose --profile vault up vault-init

Write-Step "Done. Bring up the full stack:"
Write-Host "  docker $($compose -join ' ') --profile vault up -d"
Write-Host "Or run: pwsh -File scripts/vault/smoke-vault-integration.ps1"
Write-Host "Vault UI/API: http://localhost:8200 (root token: root — dev only)"
