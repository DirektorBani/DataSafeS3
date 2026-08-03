#Requires -Version 5.1
<#
.SYNOPSIS
  Download Alpine 3.20 apk + pip wheels on the HOST (bypasses broken Docker HTTPS proxy).
#>
param(
  [string]$AlpineVersion = "3.20",
  [string]$OutRoot = ""
)

$ErrorActionPreference = "Stop"
$Root = Split-Path (Split-Path (Split-Path $PSScriptRoot -Parent) -Parent) -Parent
if (-not $OutRoot) { $OutRoot = Join-Path $Root "deploy\cluster\lab" }
# PSScriptRoot = .../scripts/cluster/lab -> repo is 3 levels up
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
if (-not $OutRoot) { $OutRoot = Join-Path $Root "deploy\cluster\lab" }
$apkDir = Join-Path $OutRoot "offline-apk"
$pipDir = Join-Path $OutRoot "offline-pip"
New-Item -ItemType Directory -Force -Path $apkDir, $pipDir | Out-Null

function Get-MirrorBase {
  $mirrors = @(
    "https://dl-cdn.alpinelinux.org/alpine/v$AlpineVersion",
    "https://mirror.yandex.ru/mirrors/alpine/v$AlpineVersion"
  )
  foreach ($m in $mirrors) {
    try {
      $r = Invoke-WebRequest -Uri "$m/main/x86_64/APKINDEX.tar.gz" -Method Head -UseBasicParsing -TimeoutSec 15
      if ($r.StatusCode -ge 200 -and $r.StatusCode -lt 400) { return $m }
    } catch {}
  }
  throw "no alpine mirror reachable from host"
}

function Expand-ApkIndex([string]$tarPath, [string]$destFile) {
  $tmp = Join-Path $env:TEMP ("apkidx-" + [guid]::NewGuid().ToString("n"))
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  try {
    tar -xzf $tarPath -C $tmp
    if (-not (Test-Path (Join-Path $tmp "APKINDEX"))) { throw "APKINDEX missing in $tarPath" }
    Copy-Item (Join-Path $tmp "APKINDEX") $destFile -Force
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
}

function Read-ApkIndex([string]$path) {
  $map = @{}
  $raw = [IO.File]::ReadAllText($path) -replace "`r", ""
  foreach ($block in ($raw -split "`n`n")) {
    if ([string]::IsNullOrWhiteSpace($block)) { continue }
    $pkg = @{}
    foreach ($line in ($block -split "`n")) {
      $i = $line.IndexOf(':')
      if ($i -lt 1) { continue }
      # Case-sensitive: P/V/D are package metadata; lowercase p: is "provides" and must not overwrite P.
      $k = $line.Substring(0, $i)
      if ($k -ceq 'P' -or $k -ceq 'V' -or $k -ceq 'D' -or $k -ceq 'A' -or $k -ceq 'S') {
        $pkg[$k] = $line.Substring($i + 1)
      }
    }
    if ($pkg.ContainsKey('P') -and $pkg['P']) {
      $map[$pkg['P']] = $pkg
    }
  }
  return $map
}

function Resolve-Deps($maps, [string[]]$roots) {
  $need = New-Object 'System.Collections.Generic.HashSet[string]'
  $q = New-Object System.Collections.Queue
  foreach ($r in $roots) { [void]$q.Enqueue($r) }
  while ($q.Count -gt 0) {
    $name = [string]$q.Dequeue()
    if ([string]::IsNullOrWhiteSpace($name)) { continue }
    if ($name.StartsWith("so:") -or $name.StartsWith("pc:") -or $name.StartsWith("cmd:")) { continue }
    $name = ($name -split '[<>=!~]', 2)[0]
    if (-not $need.Add($name)) { continue }
    $pkg = $null
    foreach ($m in $maps) {
      if ($m.ContainsKey($name)) { $pkg = $m[$name]; break }
    }
    if (-not $pkg) { continue }
    if ($pkg.ContainsKey('D') -and $pkg['D']) {
      foreach ($d in ($pkg['D'] -split '\s+')) {
        if ($d) { [void]$q.Enqueue($d) }
      }
    }
  }
  return @($need)
}

$mirror = Get-MirrorBase
Write-Host "  mirror: $mirror"
$mainIdx = Join-Path $apkDir "APKINDEX-main"
$commIdx = Join-Path $apkDir "APKINDEX-community"
curl.exe -fsSL "$mirror/main/x86_64/APKINDEX.tar.gz" -o "$apkDir\APKINDEX-main.tar.gz"
curl.exe -fsSL "$mirror/community/x86_64/APKINDEX.tar.gz" -o "$apkDir\APKINDEX-community.tar.gz"
Expand-ApkIndex "$apkDir\APKINDEX-main.tar.gz" $mainIdx
Expand-ApkIndex "$apkDir\APKINDEX-community.tar.gz" $commIdx
$mainMap = Read-ApkIndex $mainIdx
$commMap = Read-ApkIndex $commIdx
Write-Host ("  index packages: main={0} community={1}" -f $mainMap.Count, $commMap.Count)
$maps = @($mainMap, $commMap)

function Find-PkgName([string[]]$candidates) {
  foreach ($c in $candidates) {
    foreach ($m in $maps) {
      if ($m.ContainsKey($c)) { return $c }
    }
  }
  return $null
}

$want = @(
  @("openssh"),
  @("sudo"),
  @("curl"),
  @("ca-certificates"),
  @("python3"),
  @("py3-pip"),
  @("py3-setuptools"),
  @("py3-wheel"),
  @("haproxy"),
  @("keepalived"),
  @("postgresql16", "postgresql15", "postgresql"),
  @("postgresql16-contrib", "postgresql15-contrib", "postgresql-contrib"),
  @("etcd"),
  @("procps-ng", "procps"),
  @("iproute2"),
  @("iputils"),
  @("bash"),
  @("shadow"),
  @("libcap"),
  @("iptables"),
  @("nfs-utils")
)

$resolvedRoots = @()
foreach ($cands in $want) {
  $hit = Find-PkgName $cands
  if ($hit) { $resolvedRoots += $hit }
  else { Write-Warning ("missing package candidates: " + ($cands -join ", ")) }
}
Write-Host ("  roots: " + ($resolvedRoots -join ", "))

$pkgs = Resolve-Deps $maps $resolvedRoots
Write-Host ("  packages to fetch: " + $pkgs.Count)

$ok = 0; $fail = 0
foreach ($name in ($pkgs | Sort-Object)) {
  $pkg = $null
  $repo = $null
  if ($mainMap.ContainsKey($name)) { $pkg = $mainMap[$name]; $repo = "main" }
  elseif ($commMap.ContainsKey($name)) { $pkg = $commMap[$name]; $repo = "community" }
  if (-not $pkg) { continue }
  $file = "{0}-{1}.apk" -f $pkg['P'], $pkg['V']
  $url = "{0}/{1}/x86_64/{2}" -f $mirror, $repo, $file
  $dest = Join-Path $apkDir $file
  if ((Test-Path $dest) -and ((Get-Item $dest).Length -gt 32)) { $ok++; continue }
  Write-Host "  fetch $file"
  try {
    curl.exe -fsSL $url -o $dest
    if ((Get-Item $dest).Length -lt 32) { throw "tiny file" }
    $ok++
  } catch {
    Write-Warning "failed $url"
    $fail++
    Remove-Item $dest -ErrorAction SilentlyContinue
  }
}
Write-Host "  apk ok=$ok fail=$fail"

$pipOk = $false
foreach ($cmd in @("python", "py")) {
  $py = Get-Command $cmd -ErrorAction SilentlyContinue
  if (-not $py) { continue }
  Write-Host "  pip download via $($py.Source)"
  & $py.Source -m pip download "patroni[etcd3]" -d $pipDir
  if ($LASTEXITCODE -eq 0) { $pipOk = $true; break }
}
if (-not $pipOk) {
  Write-Warning "pip download failed - Dockerfile.offline will try online pip (may fail under proxy)"
}

Set-Content -LiteralPath (Join-Path $OutRoot "offline-bundle.stamp") -Value ("fetched " + (Get-Date -Format o)) -Encoding ascii
Write-Host "  [OK] offline bundle under $OutRoot"
if ($fail -gt 0 -or $ok -lt 20) { Write-Warning "apk fetch looks incomplete (ok=$ok fail=$fail)"; exit 1 }
exit 0
