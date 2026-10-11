# Proves Invoke-NativeCommand keeps a native stderr write from becoming a
# terminating error when this process is itself redirected (*> log) and
# $ErrorActionPreference is Stop. Exit 0 is success. No Docker required.
$ErrorActionPreference = 'Stop'

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $here '..\Invoke-NativeCommand.ps1')

$cmd = $env:ComSpec
if ([string]::IsNullOrWhiteSpace($cmd)) {
    throw 'ComSpec is not set; cmd.exe is required for this test'
}

$savedNative = $null
$haveNativePref = $null -ne (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue)
if ($haveNativePref) {
    $savedNative = [bool]$PSNativeCommandUseErrorActionPreference
    # 7.4 defaults this to true. The helper must tolerate that and restore it.
    $PSNativeCommandUseErrorActionPreference = $true
}

try {
    $ok = Invoke-NativeCommand -FilePath $cmd -ArgumentList @('/c', 'echo NATIVE_STDERR_OK 1>&2& exit /b 0')
} catch {
    throw "stderr on a zero exit became terminating: $($_.Exception.Message)"
}

if ($ok.ExitCode -ne 0) { throw "expected exit 0, got $($ok.ExitCode)" }
if ($global:LASTEXITCODE -ne 0) { throw "LASTEXITCODE expected 0, got $global:LASTEXITCODE" }
$joined = @($ok.Lines) -join "`n"
if ($joined -notmatch 'NATIVE_STDERR_OK') {
    throw "stderr was not captured as text: [$joined]"
}
foreach ($line in @($ok.Lines)) {
    if ($line -is [System.Management.Automation.ErrorRecord]) {
        throw 'stderr line remained an ErrorRecord'
    }
}
Write-Output $joined

try {
    $bad = Invoke-NativeCommand -FilePath $cmd -ArgumentList @('/c', 'echo NATIVE_STDERR_FAIL 1>&2& exit /b 3')
} catch {
    throw "non-zero native exit became terminating: $($_.Exception.Message)"
}
if ($bad.ExitCode -ne 3) { throw "expected exit 3, got $($bad.ExitCode)" }
if ($global:LASTEXITCODE -ne 3) { throw "LASTEXITCODE expected 3, got $global:LASTEXITCODE" }
$failText = @($bad.Lines) -join "`n"
if ($failText -notmatch 'NATIVE_STDERR_FAIL') {
    throw "fail stderr was not captured as text: [$failText]"
}

if ($haveNativePref -and ([bool]$PSNativeCommandUseErrorActionPreference -ne $true)) {
    throw 'helper did not restore PSNativeCommandUseErrorActionPreference'
}
if ($null -ne $savedNative) {
    $PSNativeCommandUseErrorActionPreference = $savedNative
}

Write-Output 'invoke-native-command ok'
exit 0
