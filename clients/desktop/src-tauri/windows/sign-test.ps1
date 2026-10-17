# Test Authenticode signature for Desktop 1.0 CI.
# The certificate is created in the Windows job and is not a production
# publisher. Q4-3 replaces it. Without DATASAFE_TEST_PFX the build stays
# unsigned so a local Windows build still works.
param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$Path
)
$ErrorActionPreference = "Stop"
if (-not $env:DATASAFE_TEST_PFX) {
  Write-Host "test signature skipped"
  exit 0
}
if (-not (Test-Path -LiteralPath $Path)) {
  throw "file to sign was not found"
}
$tool = Get-ChildItem -Path "C:\Program Files (x86)\Windows Kits\10\bin" -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
  Where-Object { $_.FullName -like "*\x64\signtool.exe" } |
  Sort-Object FullName -Descending |
  Select-Object -First 1
if (-not $tool) {
  throw "signtool.exe was not found"
}
& $tool.FullName sign /fd SHA256 /f $env:DATASAFE_TEST_PFX /p $env:DATASAFE_TEST_PFX_PASSWORD $Path
exit $LASTEXITCODE
