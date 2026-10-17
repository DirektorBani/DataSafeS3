# After the NSIS bundle: reject a datasafe-desktop.exe that imports a DLL
# the installer does not ship, then install /S, start that exe with a system
# PATH, and uninstall /S. A gnullvm link imports WebView2Loader.dll and
# libunwind.dll; neither is a Windows system DLL, and the Start Menu launch
# then fails. This script is the check CI was missing.
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

function Invoke-Timed {
    param([string]$File, [string[]]$ArgumentList, [int]$Seconds)
    $proc = Start-Process -FilePath $File -ArgumentList $ArgumentList -PassThru
    if (-not $proc.WaitForExit($Seconds * 1000)) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        throw "timed out after ${Seconds}s: $File"
    }
    return $proc.ExitCode
}

function Stop-ProcessTree {
    param([int]$ProcessId)
    $children = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$ProcessId" -ErrorAction SilentlyContinue)
    foreach ($child in $children) {
        Stop-ProcessTree -ProcessId $child.ProcessId
    }
    Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
}

function Get-PeDependents {
    param([string]$Exe)
    $vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\Installer\vswhere.exe"
    $dumpbin = $null
    if (Test-Path -LiteralPath $vswhere) {
        $dumpbin = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -find "VC\Tools\MSVC\*\bin\Hostx64\x64\dumpbin.exe" | Select-Object -First 1
    }
    $text = $null
    if ($dumpbin) {
        Write-Host "imports: dumpbin /dependents"
        $binDir = Split-Path -Parent $dumpbin
        $oldPath = $env:PATH
        $env:PATH = "$binDir;$oldPath"
        try {
            $text = (& $dumpbin /dependents $Exe | Out-String)
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
        $text = (& $objdump -p $Exe | Out-String)
        if ($LASTEXITCODE -ne 0) {
            throw "llvm-objdump failed ($LASTEXITCODE)"
        }
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
    $found = Get-ChildItem -Path $env:LOCALAPPDATA -Filter "datasafe-desktop.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($found) {
        return $found.DirectoryName
    }
    throw "installed datasafe-desktop.exe was not found"
}

function Trust-TestSigner {
    $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -like "*DataSafeS3 Desktop Test*" } | Select-Object -First 1
    if (-not $cert) { return }
    foreach ($name in @("TrustedPublisher", "Root")) {
        $store = New-Object System.Security.Cryptography.X509Certificates.X509Store($name, "CurrentUser")
        $store.Open("ReadWrite")
        $store.Add($cert)
        $store.Close()
    }
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
    Write-Host "launch: $launched"
    $app = [Diagnostics.Process]::Start($psi)
    Start-Sleep -Seconds 10
    if ($app.HasExited) {
        throw "datasafe-desktop.exe exited $($app.ExitCode) within 10s"
    }
    Write-Host "still running after 10s pid $($app.Id)"
    Stop-ProcessTree -ProcessId $app.Id
} finally {
    if ($installed -and $installDir) {
        try {
            $installRoot = $installDir.TrimEnd('\') + '\'
            Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
                $_.ExecutablePath -and $_.ExecutablePath.StartsWith($installRoot, [StringComparison]::OrdinalIgnoreCase)
            } | ForEach-Object {
                Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
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
