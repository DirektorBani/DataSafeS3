#Requires -Version 5.1
# Fetch packages that provide so:* deps for already-downloaded apks.
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$apkDir = Join-Path $Root "deploy\cluster\lab\offline-apk"
$mirror = "https://dl-cdn.alpinelinux.org/alpine/v3.20"

function Load-IndexProvides([string]$path, [string]$repo) {
  $raw = [IO.File]::ReadAllText($path) -replace "`r", ""
  $pkgOf = @{}
  foreach ($block in ($raw -split "`n`n")) {
    if ($block -notmatch '(?m)^P:([^\n]+)') { continue }
    $pname = $Matches[1]
    if ($block -notmatch '(?m)^V:([^\n]+)') { continue }
    $ver = $Matches[1]
    $pkgOf[$pname] = @{ Name = $pname; Ver = $ver; Repo = $repo; Provides = @(); Depends = @() }
    foreach ($line in ($block -split "`n")) {
      if ($line.StartsWith("p:")) {
        $pkgOf[$pname].Provides = @($line.Substring(2) -split "\s+" | ForEach-Object { ($_ -split "=", 2)[0] })
      }
      if ($line.StartsWith("D:")) {
        $pkgOf[$pname].Depends = @($line.Substring(2) -split "\s+")
      }
    }
  }
  return $pkgOf
}

$all = @{}
foreach ($kv in (Load-IndexProvides (Join-Path $apkDir "APKINDEX-main") "main").GetEnumerator()) { $all[$kv.Key] = $kv.Value }
foreach ($kv in (Load-IndexProvides (Join-Path $apkDir "APKINDEX-community") "community").GetEnumerator()) {
  if (-not $all.ContainsKey($kv.Key)) { $all[$kv.Key] = $kv.Value }
}

$provideToPkg = @{}
foreach ($pkg in $all.Values) {
  foreach ($p in $pkg.Provides) {
    if (-not $provideToPkg.ContainsKey($p)) { $provideToPkg[$p] = $pkg }
  }
  # package name itself also provides itself
  if (-not $provideToPkg.ContainsKey($pkg.Name)) { $provideToPkg[$pkg.Name] = $pkg }
}

function Resolve-Name([string]$dep) {
  if ($dep.StartsWith("/") -or $dep.StartsWith("cmd:")) { return $null }
  $base = ($dep -split "[<>=!~]", 2)[0]
  if ($provideToPkg.ContainsKey($base)) { return $provideToPkg[$base] }
  if ($provideToPkg.ContainsKey($dep)) { return $provideToPkg[$dep] }
  return $null
}

# Seeds: every package we already have an .apk for (by parsing filename Name-Ver.apk is hard);
# instead seed from known roots + iterate deps until closure, downloading missing.
$seeds = @(
  "openssh","sudo","curl","ca-certificates","python3","py3-pip","py3-setuptools","py3-wheel",
  "haproxy","keepalived","postgresql16","postgresql16-contrib","procps-ng","iproute2","iputils",
  "bash","shadow","libcap","iptables","nfs-utils","gcc","musl-dev","python3-dev","linux-headers",
  "pkgconf","g++","libffi-dev","openssl-dev","make","openssh-server","openssh-client-common",
  "openssh-keygen","openssh-sftp-server","openssh-server-common"
)

$need = New-Object 'System.Collections.Generic.HashSet[string]'
$q = New-Object System.Collections.Queue
foreach ($s in $seeds) { if ($all.ContainsKey($s)) { [void]$q.Enqueue($s) } }

while ($q.Count -gt 0) {
  $n = [string]$q.Dequeue()
  if (-not $need.Add($n)) { continue }
  if (-not $all.ContainsKey($n)) { continue }
  foreach ($d in $all[$n].Depends) {
    $prov = Resolve-Name $d
    if ($prov) { [void]$q.Enqueue($prov.Name) }
  }
}

Write-Host ("closure packages: " + $need.Count)
$ok = 0; $fail = 0
foreach ($n in ($need | Sort-Object)) {
  $pkg = $all[$n]
  $file = "{0}-{1}.apk" -f $pkg.Name, $pkg.Ver
  $dest = Join-Path $apkDir $file
  if ((Test-Path $dest) -and (Get-Item $dest).Length -gt 32) { $ok++; continue }
  $url = "{0}/{1}/x86_64/{2}" -f $mirror, $pkg.Repo, $file
  Write-Host "  fetch $file"
  try {
    curl.exe -fsSL $url -o $dest
    if ((Get-Item $dest).Length -lt 32) { throw "tiny" }
    $ok++
  } catch {
    Write-Warning "fail $file"
    $fail++
    Remove-Item $dest -EA SilentlyContinue
  }
}
Write-Host "apk ok=$ok fail=$fail totalFiles=$((Get-ChildItem $apkDir\*.apk).Count)"
if ($fail -gt 0) { exit 1 }
exit 0
