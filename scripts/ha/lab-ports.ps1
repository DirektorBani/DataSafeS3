# Shared reader for HA-lab env files. Scripts use the file when it exists and
# fall back to the stand's default ports, so a copied .env is honored.
function Read-LabEnv {
    param([string]$Path)
    $map = @{}
    if (-not (Test-Path -LiteralPath $Path)) { return $map }
    foreach ($line in Get-Content -LiteralPath $Path) {
        if ($line -match '^\s*#' -or $line -match '^\s*$') { continue }
        if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
            $map[$Matches[1]] = $Matches[2].Trim().Trim('"').Trim("'")
        }
    }
    return $map
}

function Get-LabPort {
    param(
        [hashtable]$Map,
        [string]$Key,
        [string]$Default
    )
    if ($null -ne $Map -and $Map.ContainsKey($Key) -and $Map[$Key]) {
        return [string]$Map[$Key]
    }
    return $Default
}
