# Regression test: install.ps1 must set "ldap"/"oidc" on a settings object that does not have them.
# Run: pwsh -NoProfile -File scripts/tests/install-ps1-json.tests.ps1
$ErrorActionPreference = "Stop"
$src = Get-Content (Join-Path $PSScriptRoot "../../install.ps1") -Raw
if ($src -notmatch 'function Set-JsonProperty') { throw "Set-JsonProperty missing in install.ps1" }
if ($src -match '(?m)^\s*\$cfg\.(ldap|oidc)\s*=') { throw 'install.ps1 still assigns $cfg.ldap / $cfg.oidc directly' }
function Set-JsonProperty($Object, [string]$Name, $Value) {
  $Object | Add-Member -NotePropertyName $Name -NotePropertyValue $Value -Force
}
$cfg = '{"server":{"addr":":9000"}}' | ConvertFrom-Json
Set-JsonProperty $cfg "ldap" ([pscustomobject]@{ enabled = $true })
Set-JsonProperty $cfg "oidc" ([pscustomobject]@{ enabled = $true })
if (-not $cfg.ldap.enabled -or -not $cfg.oidc.enabled) { throw "properties not set" }
Write-Host "OK"
