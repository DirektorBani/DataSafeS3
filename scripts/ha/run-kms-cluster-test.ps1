# One-command cluster KMS gate on Windows. Git Bash runs the shell script.
# Ports stay off :9000 and :9001.
$ErrorActionPreference = "Stop"
$bash = Get-Command bash -ErrorAction SilentlyContinue
if (-not $bash) {
  throw "bash is required (Git Bash)"
}
$script = Join-Path $PSScriptRoot "run-kms-cluster-test.sh"
& bash $script
if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}
