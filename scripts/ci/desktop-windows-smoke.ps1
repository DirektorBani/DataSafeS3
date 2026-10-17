# After the NSIS bundle: reject a datasafe-desktop.exe that imports a DLL
# the installer does not ship, then install /S, start that exe with a system
# PATH, and uninstall /S. A gnullvm link imports WebView2Loader.dll and
# libunwind.dll; neither is a Windows system DLL, and the Start Menu launch
# then fails.
#
# Before the install, seed Credential Manager through the separate ci-keyring
# crate (it is not a binary of the app, so NSIS does not ship it) and assert
# datasafe-agent.exe --purge-secrets removes those entries and leaves an
# unrelated generic credential. After install, the file list must be exactly
# datasafe-desktop.exe, uninstall.exe, and binaries\datasafe-agent.exe.
#
# DATASAFE_UIA_FIXTURE=1 shows the Objects and Buckets listboxes without a
# sign-in. UI Automation must return a selection from each list. Chromium
# only puts item-like roles (listbox option, treeitem) into GetSelection.
#
# Windows PowerShell 5.1 runs this file. WaitForExit(int) and an unbounded
# LOCALAPPDATA walk have hung this step past its own timeouts, so every
# external process is polled and then taskkill /F /T.
$ErrorActionPreference = "Stop"
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -Scope Global -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $repo

$built = Join-Path $repo "clients\desktop\src-tauri\target\release\datasafe-desktop.exe"
if (-not (Test-Path -LiteralPath $built)) {
    throw "datasafe-desktop.exe was not built at $built"
}
$setup = Get-ChildItem -Path (Join-Path $repo "clients\desktop\src-tauri\target\release\bundle\nsis") -Filter *.exe | Select-Object -First 1
if (-not $setup) {
    throw "NSIS installer not found"
}
$agent = Join-Path $repo "clients\desktop\src-tauri\binaries\datasafe-agent.exe"
if (-not (Test-Path -LiteralPath $agent)) {
    throw "datasafe-agent.exe was not built at $agent"
}

function Stop-ProcessTree {
    param([int]$ProcessId)
    # taskkill writes "not found" to stderr and exits 128 when the process
    # has already ended. PowerShell turns that stderr into a failing error
    # under ErrorAction Stop. Merge it into stdout and ignore a missing pid.
    $out = & taskkill.exe /F /T /PID $ProcessId 2>&1
    $text = ($out | Out-String).Trim()
    if ($LASTEXITCODE -eq 0 -or $text -match "not found") {
        return
    }
    if ($text) {
        Write-Host $text
    }
}

function Wait-OrKill {
    param([System.Diagnostics.Process]$Proc, [int]$Seconds, [string]$Label)
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    while (-not $Proc.HasExited) {
        if ([DateTime]::UtcNow -gt $deadline) {
            Stop-ProcessTree -ProcessId $Proc.Id
            throw "timed out after ${Seconds}s: $Label"
        }
        Start-Sleep -Milliseconds 250
    }
}

function Quote-Arg {
    param([string]$Text)
    if ($Text -match '[\s"]') {
        return '"' + ($Text -replace '"', '\"') + '"'
    }
    return $Text
}

function Invoke-Captured {
    param([string]$File, [string[]]$ArgumentList, [int]$Seconds)
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $File
    $quoted = @()
    foreach ($arg in $ArgumentList) {
        $quoted += (Quote-Arg -Text $arg)
    }
    $psi.Arguments = ($quoted -join " ")
    $psi.WorkingDirectory = $repo
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.CreateNoWindow = $true
    $proc = New-Object System.Diagnostics.Process
    $proc.StartInfo = $psi
    if (-not $proc.Start()) {
        throw "failed to start $File"
    }
    $outTask = $proc.StandardOutput.ReadToEndAsync()
    $errTask = $proc.StandardError.ReadToEndAsync()
    Wait-OrKill -Proc $proc -Seconds $Seconds -Label $File
    $outTask.Wait(5000) | Out-Null
    $errTask.Wait(5000) | Out-Null
    return @{
        Code = $proc.ExitCode
        Text = ($outTask.Result + "`n" + $errTask.Result).Trim()
    }
}

function Invoke-Timed {
    param([string]$File, [string[]]$ArgumentList, [int]$Seconds)
    $proc = Start-Process -FilePath $File -ArgumentList $ArgumentList -PassThru -WindowStyle Hidden
    Wait-OrKill -Proc $proc -Seconds $Seconds -Label $File
    return $proc.ExitCode
}

function Get-PeDependents {
    param([string]$Exe)
    $vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\Installer\vswhere.exe"
    $dumpbin = $null
    if (Test-Path -LiteralPath $vswhere) {
        $found = Invoke-Captured -File $vswhere -ArgumentList @(
            "-latest", "-products", "*", "-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
            "-find", "VC\Tools\MSVC\*\bin\Hostx64\x64\dumpbin.exe"
        ) -Seconds 30
        $dumpbin = ($found.Text -split "`r?`n" | Where-Object { $_ -like "*dumpbin.exe" } | Select-Object -First 1)
        if ($dumpbin) { $dumpbin = $dumpbin.Trim() }
    }
    $text = $null
    if ($dumpbin -and (Test-Path -LiteralPath $dumpbin)) {
        Write-Host "imports: dumpbin /dependents"
        $binDir = Split-Path -Parent $dumpbin
        $oldPath = $env:PATH
        $env:PATH = "$binDir;$oldPath"
        try {
            $ran = Invoke-Captured -File $dumpbin -ArgumentList @("/dependents", $Exe) -Seconds 60
            $text = $ran.Text
        } finally {
            $env:PATH = $oldPath
        }
        if ($text -match "fatal error LNK" -or $text -notmatch '(?i)\.dll') {
            Write-Host $text
            $text = $null
        }
    }
    if (-not $text) {
        $objdump = $null
        $cmd = Get-Command llvm-objdump -ErrorAction SilentlyContinue
        if ($cmd) { $objdump = $cmd.Source }
        if (-not $objdump) {
            $fallback = "C:\Program Files\LLVM\bin\llvm-objdump.exe"
            if (Test-Path -LiteralPath $fallback) { $objdump = $fallback }
        }
        if (-not $objdump) {
            throw "neither dumpbin nor llvm-objdump is available"
        }
        Write-Host "imports: llvm-objdump -p"
        $ran = Invoke-Captured -File $objdump -ArgumentList @("-p", $Exe) -Seconds 60
        if ($ran.Code -ne 0) {
            Write-Host $ran.Text
            throw "llvm-objdump failed ($($ran.Code))"
        }
        $text = $ran.Text
    }
    $seen = @{}
    $names = @()
    foreach ($match in [regex]::Matches($text, '(?im)^\s+([A-Za-z0-9_.+-]+\.dll)\s*$')) {
        $dll = $match.Groups[1].Value
        if (-not $seen.ContainsKey($dll)) {
            $seen[$dll] = $true
            $names += $dll
        }
    }
    foreach ($match in [regex]::Matches($text, '(?i)DLL Name:\s*(\S+\.dll)')) {
        $dll = $match.Groups[1].Value
        if (-not $seen.ContainsKey($dll)) {
            $seen[$dll] = $true
            $names += $dll
        }
    }
    if ($names.Count -eq 0) {
        Write-Host $text
        throw "no DLL dependents parsed from $Exe"
    }
    return $names
}

function Test-SystemDll {
    param([string]$Name)
    if ($Name -match '^(?i)(api|ext)-ms-') {
        return $true
    }
    foreach ($dir in @("System32", "SysWOW64")) {
        $path = Join-Path $env:SystemRoot (Join-Path $dir $Name)
        if (Test-Path -LiteralPath $path) {
            return $true
        }
    }
    return $false
}

function Assert-ShippedImports {
    param([string]$Exe, [string]$InstallDir)
    $shipped = @{}
    Get-ChildItem -LiteralPath $InstallDir -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
        $shipped[$_.Name] = $true
    }
    $missing = @()
    foreach ($dll in (Get-PeDependents -Exe $Exe)) {
        if (Test-SystemDll -Name $dll) {
            continue
        }
        if ($shipped.ContainsKey($dll)) {
            Write-Host "shipped non-system DLL: $dll"
            continue
        }
        $missing += $dll
    }
    if ($missing.Count -gt 0) {
        $named = $missing | Where-Object { $_ -match '^(?i)(WebView2Loader|libunwind)\.dll$' }
        if ($named) {
            Write-Host "refusing non-system DLL not shipped by the installer: $($named -join ', ')"
        }
        throw "datasafe-desktop.exe imports non-system DLLs the installer does not ship: $($missing -join ', ')"
    }
    Write-Host "imports ok"
}

function Find-InstallDir {
    $known = Join-Path $env:LOCALAPPDATA "DataSafeS3 Desktop"
    $knownExe = Join-Path $known "datasafe-desktop.exe"
    if (Test-Path -LiteralPath $knownExe) {
        return $known
    }
    $keys = @()
    foreach ($root in @(
            "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall",
            "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall"
        )) {
        if (Test-Path $root) {
            $keys += Get-ChildItem $root
        }
    }
    foreach ($key in $keys) {
        $prop = Get-ItemProperty $key.PSPath
        if ($prop.DisplayName -notlike "*DataSafeS3 Desktop*") { continue }
        $loc = "$($prop.InstallLocation)".Trim().Trim('"')
        if ($loc -and (Test-Path -LiteralPath (Join-Path $loc "datasafe-desktop.exe"))) {
            return $loc.TrimEnd('\')
        }
    }
    throw "installed datasafe-desktop.exe was not found under $known or the uninstall registry"
}

function Assert-InstallFiles {
    param([string]$Dir)
    $root = $Dir.TrimEnd('\')
    $required = @("datasafe-desktop.exe", "uninstall.exe", "binaries\datasafe-agent.exe")
    $found = @(Get-ChildItem -LiteralPath $root -Recurse -File | ForEach-Object {
            $_.FullName.Substring($root.Length).TrimStart('\')
        })
    Write-Host "installed files ($($found.Count)):"
    foreach ($rel in ($found | Sort-Object)) {
        Write-Host "  $rel"
    }
    $have = @{}
    foreach ($rel in $found) {
        $have[$rel.ToLowerInvariant()] = $true
    }
    foreach ($name in $required) {
        if (-not $have.ContainsKey($name.ToLowerInvariant())) {
            throw "installer is missing $name"
        }
    }
    foreach ($rel in $found) {
        $ok = $false
        foreach ($name in $required) {
            if ($rel.Equals($name, [System.StringComparison]::OrdinalIgnoreCase)) {
                $ok = $true
            }
        }
        if (-not $ok) {
            throw "installer shipped an unexpected file: $rel"
        }
    }
}

function Trust-TestSigner {
    # CurrentUser\Root prompts for consent and blocks this non-interactive
    # runner. TrustedPublisher does not. The exe is started directly, so a
    # missing trust must not stop the launch check.
    Write-Host "trust signer"
    $job = Start-Job -ScriptBlock {
        $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -like "*DataSafeS3 Desktop Test*" } | Select-Object -First 1
        if (-not $cert) { return }
        $store = New-Object System.Security.Cryptography.X509Certificates.X509Store("TrustedPublisher", "CurrentUser")
        $store.Open("ReadWrite")
        $store.Add($cert)
        $store.Close()
    }
    if (-not (Wait-Job $job -Timeout 15)) {
        Stop-Job $job -ErrorAction SilentlyContinue
        Write-Host "trust signer skipped"
        return
    }
    Receive-Job $job -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $job -Force -ErrorAction SilentlyContinue
}

function Get-KeyringTool {
    $tool = Join-Path $repo "clients\desktop\ci-keyring\target\release\ci-keyring.exe"
    Write-Host "building ci-keyring"
    $cargo = (Get-Command cargo -ErrorAction Stop).Source
    $builtTool = Invoke-Captured -File $cargo -ArgumentList @(
        "build", "--release", "--manifest-path", "clients/desktop/ci-keyring/Cargo.toml"
    ) -Seconds 300
    if ($builtTool.Code -ne 0 -or -not (Test-Path -LiteralPath $tool)) {
        Write-Host $builtTool.Text
        throw "ci-keyring build failed ($($builtTool.Code))"
    }
    return $tool
}

function Test-GenericCredential {
    param([string]$Target)
    if (-not ("DsCredRead" -as [type])) {
        $src = "using System; using System.Runtime.InteropServices; public static class DsCredRead { [DllImport(`"advapi32.dll`", CharSet = CharSet.Unicode, SetLastError = true)] public static extern bool CredRead(string target, uint type, uint flags, out IntPtr credential); [DllImport(`"advapi32.dll`")] public static extern void CredFree(IntPtr credential); public static bool Exists(string target) { IntPtr credential; if (!CredRead(target, 1, 0, out credential)) return false; CredFree(credential); return true; } }"
        Add-Type -TypeDefinition $src
    }
    return [DsCredRead]::Exists($Target)
}

function Test-PurgeSecrets {
    $tool = Get-KeyringTool
    $id = "ci" + [guid]::NewGuid().ToString("N")
    $control = "ci-control-$id.not-datasafe"
    Write-Host "purge probe id $id"
    try {
        $seeded = Invoke-Captured -File $tool -ArgumentList @("seed", $id) -Seconds 60
        if ($seeded.Code -ne 0) {
            Write-Host $seeded.Text
            throw "keyring seed failed ($($seeded.Code))"
        }
        $present = Invoke-Captured -File $tool -ArgumentList @("expect-present", $id) -Seconds 60
        if ($present.Code -ne 0) {
            Write-Host $present.Text
            throw "keyring entries were not readable before purge"
        }
        $cmdkey = Join-Path $env:SystemRoot "System32\cmdkey.exe"
        $added = Invoke-Captured -File $cmdkey -ArgumentList @("/generic:$control", "/user:ci", "/pass:ci-unrelated") -Seconds 30
        if ($added.Code -ne 0) {
            Write-Host $added.Text
            throw "cmdkey could not add the unrelated credential"
        }
        if (-not (Test-GenericCredential -Target $control)) {
            throw "unrelated credential $control was not stored"
        }
        $purged = Invoke-Captured -File $agent -ArgumentList @("--purge-secrets") -Seconds 60
        Write-Host "purge-secrets exit $($purged.Code): $($purged.Text)"
        if ($purged.Code -ne 0) {
            throw "purge-secrets exited $($purged.Code)"
        }
        if ($purged.Text -notmatch "removed ([1-9][0-9]*)") {
            throw "purge-secrets did not report a removed credential: $($purged.Text)"
        }
        $after = Invoke-Captured -File $tool -ArgumentList @("expect-purged", $id) -Seconds 60
        if ($after.Code -ne 0) {
            Write-Host $after.Text
            throw "desktop credentials were not purged"
        }
        if (-not (Test-GenericCredential -Target $control)) {
            throw "purge-secrets removed the unrelated credential $control"
        }
        Write-Host "purge-secrets removed desktop credentials and left $control"
    } finally {
        try {
            Invoke-Captured -File $tool -ArgumentList @("cleanup", $id) -Seconds 60 | Out-Null
        } catch {
            Write-Host "keyring cleanup: $_"
        }
        try {
            $cmdkey = Join-Path $env:SystemRoot "System32\cmdkey.exe"
            Invoke-Captured -File $cmdkey -ArgumentList @("/delete:$control") -Seconds 30 | Out-Null
        } catch {
            Write-Host "credential cleanup: $_"
        }
    }
}

Test-PurgeSecrets

function Assert-UiaSelection {
    Add-Type -AssemblyName UIAutomationClient
    Add-Type -AssemblyName UIAutomationTypes
    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $windowCond = New-Object System.Windows.Automation.AndCondition(
        (New-Object System.Windows.Automation.PropertyCondition(
            [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
            [System.Windows.Automation.ControlType]::Window)),
        (New-Object System.Windows.Automation.PropertyCondition(
            [System.Windows.Automation.AutomationElement]::NameProperty,
            "DataSafeS3 Desktop"))
    )
    $deadline = [DateTime]::UtcNow.AddSeconds(25)
    $last = "window not found"
    while ([DateTime]::UtcNow -lt $deadline) {
        $window = $root.FindFirst([System.Windows.Automation.TreeScope]::Children, $windowCond)
        if ($window) {
            $ready = $true
            foreach ($label in @("Objects", "Buckets")) {
                $listCond = New-Object System.Windows.Automation.AndCondition(
                    (New-Object System.Windows.Automation.PropertyCondition(
                        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
                        [System.Windows.Automation.ControlType]::List)),
                    (New-Object System.Windows.Automation.PropertyCondition(
                        [System.Windows.Automation.AutomationElement]::NameProperty,
                        $label))
                )
                $list = $window.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $listCond)
                if (-not $list) {
                    $ready = $false
                    $last = "$label list not found"
                    break
                }
                try {
                    $raw = $list.GetCurrentPattern([System.Windows.Automation.SelectionPattern]::Pattern)
                } catch {
                    $ready = $false
                    $last = "$label has no Selection pattern: $_"
                    break
                }
                $pattern = [System.Windows.Automation.SelectionPattern]$raw
                $selected = @($pattern.Current.GetSelection())
                Write-Host "UIA $label GetSelection count $($selected.Count)"
                if ($selected.Count -lt 1) {
                    $ready = $false
                    $last = "$label GetSelection is empty"
                    break
                }
            }
            if ($ready) {
                Write-Host "UIA Selection returned the selected option on Objects and Buckets"
                return
            }
        }
        Start-Sleep -Milliseconds 500
    }
    throw "UIA Selection check failed: $last"
}

$installed = $false
$installDir = $null
$uninstallError = $null
try {
    Write-Host "NSIS install: $($setup.FullName)"
    $code = Invoke-Timed -File $setup.FullName -ArgumentList @("/S") -Seconds 180
    if ($code -ne 0) {
        throw "NSIS install exited $code"
    }
    $installed = $true
    $installDir = Find-InstallDir
    Assert-InstallFiles -Dir $installDir
    $launched = Join-Path $installDir "datasafe-desktop.exe"
    if (-not (Test-Path -LiteralPath $launched)) {
        throw "installer did not copy datasafe-desktop.exe to $installDir"
    }
    Assert-ShippedImports -Exe $built -InstallDir $installDir
    Trust-TestSigner
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $launched
    $psi.WorkingDirectory = $installDir
    $psi.UseShellExecute = $false
    $psi.EnvironmentVariables["PATH"] = "$env:SystemRoot\System32;$env:SystemRoot"
    $psi.EnvironmentVariables["DATASAFE_UIA_FIXTURE"] = "1"
    Write-Host "launch: $launched"
    $app = [Diagnostics.Process]::Start($psi)
    $alive = $true
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($app.HasExited) {
            $alive = $false
            break
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $alive) {
        throw "datasafe-desktop.exe exited $($app.ExitCode) within 10s"
    }
    if ($app.HasExited) {
        throw "datasafe-desktop.exe exited $($app.ExitCode) within 10s"
    }
    Write-Host "still running after 10s pid $($app.Id)"
    Assert-UiaSelection
    $agentDeadline = [DateTime]::UtcNow.AddSeconds(15)
    while ([DateTime]::UtcNow -lt $agentDeadline) {
        if (@(Get-Process -Name "datasafe-agent" -ErrorAction SilentlyContinue).Count -ge 1) {
            break
        }
        Start-Sleep -Milliseconds 250
    }
    Write-Host "second launch: $launched"
    $second = [Diagnostics.Process]::Start($psi)
    $secondDeadline = [DateTime]::UtcNow.AddSeconds(15)
    $oneDesktop = $false
    $oneAgent = $false
    while ([DateTime]::UtcNow -lt $secondDeadline) {
        $desktopCount = @(Get-Process -Name "datasafe-desktop" -ErrorAction SilentlyContinue).Count
        $agentCount = @(Get-Process -Name "datasafe-agent" -ErrorAction SilentlyContinue).Count
        if ($second.HasExited -and $desktopCount -eq 1 -and $agentCount -eq 1) {
            $oneDesktop = $true
            $oneAgent = $true
            break
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $second.HasExited) {
        throw "second datasafe-desktop.exe is still running pid $($second.Id)"
    }
    $desktops = @(Get-Process -Name "datasafe-desktop" -ErrorAction SilentlyContinue)
    $agents = @(Get-Process -Name "datasafe-agent" -ErrorAction SilentlyContinue)
    if (-not $oneDesktop -or $desktops.Count -ne 1) {
        throw "expected one datasafe-desktop.exe, found $($desktops.Count)"
    }
    if (-not $oneAgent -or $agents.Count -ne 1) {
        throw "expected one datasafe-agent.exe, found $($agents.Count)"
    }
    Write-Host "second launch exited; one desktop pid $($desktops[0].Id) and one agent pid $($agents[0].Id)"
    Stop-ProcessTree -ProcessId $app.Id
} finally {
    if ($installed -and $installDir) {
        try {
            Get-Process -Name "datasafe-desktop", "datasafe-agent" -ErrorAction SilentlyContinue | ForEach-Object {
                Stop-ProcessTree -ProcessId $_.Id
            }
            Start-Sleep -Seconds 2
            $uninstaller = Join-Path $installDir "uninstall.exe"
            if (-not (Test-Path -LiteralPath $uninstaller)) {
                throw "uninstall.exe is missing from $installDir"
            }
            Write-Host "NSIS uninstall: $uninstaller /S"
            $code = Invoke-Timed -File $uninstaller -ArgumentList @("/S") -Seconds 180
            if ($code -ne 0) {
                throw "NSIS uninstall exited $code"
            }
            if (Test-Path -LiteralPath (Join-Path $installDir "datasafe-desktop.exe")) {
                throw "datasafe-desktop.exe is still installed after /S"
            }
            Write-Host "uninstalled"
        } catch {
            $uninstallError = $_
            Write-Host "uninstall failed: $_"
        }
    }
}
if ($uninstallError) {
    throw $uninstallError
}
