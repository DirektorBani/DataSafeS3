# Build datasafe-sync sidecar for Tauri bundling (Windows)
# Run from repository root: .\scripts\build-datasafe-sync.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot

$OutDir = Join-Path $Root "clients\desktop\src-tauri\binaries"
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Name = "datasafe-sync-x86_64-pc-windows-msvc.exe" },
    @{ GOOS = "linux"; GOARCH = "amd64"; Name = "datasafe-sync-x86_64-unknown-linux-gnu" },
    @{ GOOS = "darwin"; GOARCH = "amd64"; Name = "datasafe-sync-x86_64-apple-darwin" },
    @{ GOOS = "darwin"; GOARCH = "arm64"; Name = "datasafe-sync-aarch64-apple-darwin" }
)

Push-Location $Root
try {
    foreach ($t in $targets) {
        $env:GOOS = $t.GOOS
        $env:GOARCH = $t.GOARCH
        $dest = Join-Path $OutDir $t.Name
        Write-Host "Building $($t.Name)..."
        go build -o $dest ./cmd/datasafe-sync
    }
    Write-Host "Sidecars written to $OutDir"
} finally {
    Pop-Location
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}
