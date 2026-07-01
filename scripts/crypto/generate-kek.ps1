# Generate a local X25519 KEK keypair for field encryption dev/testing.
# NEVER commit private keys. Output directory data/keys/ is gitignored via data/.
$ErrorActionPreference = "Stop"

function Fail($msg) { Write-Error $msg; exit 1 }

if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) {
    Fail "openssl is required (Git for Windows or OpenSSL on PATH)"
}

$Root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$KeyDir = if ($env:DATASAFE_KEK_DIR) { $env:DATASAFE_KEK_DIR } else { Join-Path $Root "data\keys" }
$rand = (& openssl rand -hex 2).Trim()
$KekId = if ($env:DATASAFE_KEK_ID) { $env:DATASAFE_KEK_ID } else { "kek-$(Get-Date -Format 'yyyyMMdd')-$rand" }
$KeyFile = Join-Path $KeyDir "$KekId.key"
$PubFile = Join-Path $KeyDir "$KekId.pub"

New-Item -ItemType Directory -Force -Path $KeyDir | Out-Null

if ((Test-Path $KeyFile) -or (Test-Path $PubFile)) {
    Fail "Refusing to overwrite existing keys in $KeyDir (set `$env:DATASAFE_KEK_ID or remove files)"
}

& openssl genpkey -algorithm X25519 -out $KeyFile
& openssl pkey -in $KeyFile -pubout -out $PubFile

$privDer = & openssl pkey -in $KeyFile -outform DER
$pubDer = & openssl pkey -pubin -in $PubFile -outform DER
$privRaw = $privDer[-32..-1]
$pubRaw = $pubDer[-32..-1]
$privB64 = [Convert]::ToBase64String([byte[]]$privRaw)
$pubB64 = [Convert]::ToBase64String([byte[]]$pubRaw)

Write-Host ""
Write-Host "=== DataSafeS3 field encryption KEK (local only) ==="
Write-Host "kek_id suggestion: $KekId"
Write-Host "Private PEM: $KeyFile"
Write-Host "Public PEM:  $PubFile"
Write-Host ""
Write-Host "Add to .env (NEVER commit):"
Write-Host "STORAGE_FIELD_ENCRYPTION_ENABLED=true"
Write-Host "STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=$KekId"
Write-Host "STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=$privB64"
Write-Host ""
Write-Host "Public key (base64 raw 32 bytes, for registry bootstrap):"
Write-Host "STORAGE_FIELD_ENCRYPTION_KEK_PUBLIC_KEY=$pubB64"
Write-Host ""
Write-Host "Rotation: keep old private keys in STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS JSON until rewrap complete."
