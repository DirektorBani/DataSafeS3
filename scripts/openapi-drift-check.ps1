#Requires -Version 5.1
<#
.SYNOPSIS
  Verify Community OpenAPI spec: no admin paths, embedded copy in sync, subset of server routes.

.DESCRIPTION
  CI-friendly wrapper around go test -run OpenAPI.
  Exit 1 when the community spec drifts or contains admin-only paths.
#>
param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
)

$ErrorActionPreference = "Stop"
Push-Location $RepoRoot
try {
    Write-Host "OpenAPI community spec check (no admin paths, server subset)..."
    go test ./internal/api/... -run "TestOpenAPI" -count=1
    if ($LASTEXITCODE -ne 0) {
        Write-Error "OpenAPI check failed (exit $LASTEXITCODE)"
        exit $LASTEXITCODE
    }
    Write-Host "OK: community OpenAPI spec is valid."
} finally {
    Pop-Location
}
