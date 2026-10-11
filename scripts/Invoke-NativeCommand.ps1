# Runs a native executable without letting stderr become a terminating error.
# Windows PowerShell 5.1, with $ErrorActionPreference = 'Stop', turns redirected
# native stderr into a NativeCommandError (docker compose progress does this).
# PowerShell 7.3+ can also turn a non-zero exit into an error. Success is only
# $LASTEXITCODE. Stderr is returned as plain strings.

function Invoke-NativeCommand {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true, Position = 0)]
        [string]$FilePath,
        [Parameter(Position = 1)]
        [object[]]$ArgumentList
    )

    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $restoreNative = $false
    $prevNative = $false
    $nativeVar = Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue
    if ($null -ne $nativeVar) {
        $prevNative = [bool]$PSNativeCommandUseErrorActionPreference
        $PSNativeCommandUseErrorActionPreference = $false
        $restoreNative = $true
    }

    $code = 1
    $merged = @()
    try {
        $flat = New-Object System.Collections.Generic.List[object]
        foreach ($item in @($ArgumentList)) {
            if ($null -eq $item) { continue }
            if ($item -is [System.Array]) {
                foreach ($inner in $item) {
                    if ($null -ne $inner) { [void]$flat.Add($inner) }
                }
            } else {
                [void]$flat.Add($item)
            }
        }
        $argv = @($flat.ToArray())
        if ($argv.Count -eq 0) {
            $merged = @(& $FilePath 2>&1)
        } else {
            $merged = @(& $FilePath @argv 2>&1)
        }
        if ($null -ne $LASTEXITCODE) { $code = [int]$LASTEXITCODE }
    } finally {
        $ErrorActionPreference = $prevEap
        if ($restoreNative) {
            $PSNativeCommandUseErrorActionPreference = $prevNative
        }
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $stdout = New-Object System.Collections.Generic.List[string]
    $stderr = New-Object System.Collections.Generic.List[string]
    foreach ($item in @($merged)) {
        if ($null -eq $item) { continue }
        $text = $null
        $isErr = $false
        if ($item -is [System.Management.Automation.ErrorRecord]) {
            $isErr = $true
            if ($item.TargetObject -is [string] -and -not [string]::IsNullOrEmpty([string]$item.TargetObject)) {
                $text = [string]$item.TargetObject
            } else {
                $text = [string]$item.Exception.Message
            }
        } else {
            $text = [string]$item
        }
        [void]$lines.Add($text)
        if ($isErr) { [void]$stderr.Add($text) } else { [void]$stdout.Add($text) }
    }

    $global:LASTEXITCODE = $code
    return [pscustomobject]@{
        ExitCode = $code
        Lines    = @($lines.ToArray())
        Stdout   = @($stdout.ToArray())
        Stderr   = @($stderr.ToArray())
    }
}
