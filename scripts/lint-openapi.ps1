#Requires -Version 5.1
<#
.SYNOPSIS
  Lint docs/api/openapi.yaml with Spectral (optional).

.DESCRIPTION
  Uses npx @stoplight/spectral-cli when Node.js is available.
  Skips with exit 0 if npx is not installed (document manual lint in docs/api/README.md).
#>
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$SpecPath = "docs/api/openapi.yaml"
)

$ErrorActionPreference = "Stop"
$fullSpec = Join-Path $RepoRoot $SpecPath
if (-not (Test-Path $fullSpec)) {
    Write-Error "Spec not found: $fullSpec"
    exit 1
}

$npx = Get-Command npx -ErrorAction SilentlyContinue
if (-not $npx) {
    Write-Warning "npx not found; skip Spectral lint. Install Node.js or run manually: npx @stoplight/spectral-cli lint $SpecPath"
    exit 0
}

Push-Location $RepoRoot
try {
    Write-Host "Spectral lint $SpecPath ..."
    npx --yes @stoplight/spectral-cli@6.11.1 lint $SpecPath
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
