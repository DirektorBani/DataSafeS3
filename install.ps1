#Requires -Version 5.1
<#
.SYNOPSIS
  Interactive DataSafeS3 installer (Windows / PowerShell).

.EXAMPLE
  .\install.ps1
  .\install.ps1 -Yes -Profiles core,postgres,monitoring,data
#>
param(
  [switch]$Yes,
  [string]$Tag = "v1.3.0",
  [string]$DataRoot = "",
  [string]$Profiles = "",
  [string]$ProjectName = "datasafe",
  [switch]$DryRun,
  [switch]$Cluster,
  [switch]$Help
)

$ErrorActionPreference = "Stop"
$Root = if ($PSScriptRoot) { $PSScriptRoot } else { Get-Location }
Set-Location $Root

function Write-Title([string]$t) {
  Write-Host ""
  Write-Host "=== $t ===" -ForegroundColor Cyan
}
function Write-Ok([string]$m) { Write-Host "  [OK] $m" -ForegroundColor Green }
function Write-Warn([string]$m) { Write-Host "  [!!] $m" -ForegroundColor Yellow }
function Write-Err([string]$m) { Write-Host "  [XX] $m" -ForegroundColor Red }
function Write-Info([string]$m) { Write-Host "  $m" }

if ($Help) {
  Write-Host @"
DataSafeS3 interactive installer

  .\install.ps1
  .\install.ps1 -Yes -Profiles core,postgres,monitoring,data
  .\install.ps1 -DryRun -Yes -Profiles core,postgres,data
  .\install.ps1 -Tag v1.3.0 -DataRoot D:/datasafe-data
  .\install.ps1 -Cluster -DryRun

Profiles: core, postgres, monitoring, data, binary, identity
Cluster Wave 1: inventory + SSH mode [P]/[K] (Apply/Patroni = Wave 2)
Docs: docs/getting-started/en/installer.md
"@
  exit 0
}

# --- option model ---
$script:Opts = [ordered]@{
  core       = @{ On = $true;  Locked = $true;  Label = "Core (console + S3 API)" }
  postgres   = @{ On = $true;  Locked = $false; Label = "PostgreSQL metadata" }
  monitoring = @{ On = $true;  Locked = $false; Label = "Monitoring (Prometheus + Grafana)" }
  data       = @{ On = $true;  Locked = $false; Label = "Persist data on host (local-data overlay)" }
  binary     = @{ On = $false; Locked = $false; Label = "Build from source (local Linux binary)" }
  identity   = @{ On = $false; Locked = $false; Label = "Identity lab (LDAP + Keycloak)" }
}
$script:OptOrder = @("core", "postgres", "monitoring", "data", "binary", "identity")

function Set-ProfilesFromString([string]$raw) {
  if ([string]::IsNullOrWhiteSpace($raw)) { return }
  foreach ($k in $script:OptOrder) {
    if (-not $script:Opts[$k].Locked) { $script:Opts[$k].On = $false }
  }
  foreach ($p in ($raw -split "," | ForEach-Object { $_.Trim().ToLowerInvariant() } | Where-Object { $_ })) {
    if (-not $script:Opts.Contains($p)) { throw "Unknown profile: $p" }
    $script:Opts[$p].On = $true
  }
  $script:Opts["core"].On = $true
}

function Get-DefaultDataRoot {
  if ($DataRoot) { return ($DataRoot -replace '\\', '/') }
  if (Test-Path "D:\") { return "D:/datasafe-data" }
  $home = $env:USERPROFILE -replace '\\', '/'
  return "$home/datasafe-data"
}

function Test-CommandExists([string]$Name) {
  return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Test-PortFree([int]$Port) {
  try {
    $c = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    return -not $c
  } catch {
    try {
      $l = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
      $l.Start(); $l.Stop()
      return $true
    } catch { return $false }
  }
}

function Show-Detect {
  Write-Title "1) System"
  $os = [System.Environment]::OSVersion.VersionString
  $arch = $env:PROCESSOR_ARCHITECTURE
  try {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
  } catch {}
  Write-Ok "OS: $os ($arch)"
  Write-Ok "Shell: PowerShell $($PSVersionTable.PSVersion)"
  Write-Ok "Repo: $Root"
}

function Ensure-Docker {
  Write-Title "2) Prerequisites"
  $dockerOk = $false
  if (Test-CommandExists "docker") {
    try {
      docker info 2>$null | Out-Null
      if ($LASTEXITCODE -eq 0) {
        $ver = (docker version --format '{{.Server.Version}}' 2>$null)
        Write-Ok "Docker Engine $ver"
        $dockerOk = $true
      } else {
        Write-Warn "Docker CLI found but engine not reachable (is Docker Desktop running?)"
      }
    } catch {
      Write-Warn "Docker CLI found but engine not reachable"
    }
  } else {
    Write-Err "Docker not found"
  }

  if (-not $dockerOk) {
    if ($Yes) { throw "Docker is required. Install Docker Desktop and re-run." }
    Write-Host ""
    Write-Host "  Docker is required. Choose:" -ForegroundColor Yellow
    Write-Host "    [1] Open download page (Docker Desktop)"
    Write-Host "    [2] Try winget install Docker.DockerDesktop"
    Write-Host "    [3] Exit and install manually"
    $c = Read-Host "  Choice"
    switch ($c) {
      "1" {
        Start-Process "https://docs.docker.com/desktop/setup/install/windows-install/"
        throw "Install Docker Desktop, start it, then re-run .\install.ps1"
      }
      "2" {
        if (-not (Test-CommandExists "winget")) { throw "winget not available. Use option 1 or install Docker manually." }
        Write-Info "Running: winget install -e --id Docker.DockerDesktop"
        winget install -e --id Docker.DockerDesktop
        throw "After Docker Desktop finishes installing, start it and re-run .\install.ps1"
      }
      default { throw "Aborted: Docker required." }
    }
  }

  $composeOk = $false
  docker compose version 2>$null | Out-Null
  if ($LASTEXITCODE -eq 0) {
    Write-Ok "docker compose available"
    $composeOk = $true
  } else {
    Write-Err "docker compose plugin missing"
    if ($Yes) { throw "docker compose required" }
    Write-Warn "Update Docker Desktop so the Compose V2 plugin is included."
    throw "Aborted: docker compose required."
  }

  foreach ($port in @(8080, 9000, 3000)) {
    if (Test-PortFree $port) { Write-Ok "Port $port free" }
    else { Write-Warn "Port $port is in use - compose may fail to bind" }
  }
  return $composeOk
}

function Enforce-OptDeps {
  $script:Opts["core"].On = $true
}

function Set-OptSelection([int[]]$indices) {
  # Replace selection: listed items ON, others OFF (core always stays ON).
  foreach ($k in $script:OptOrder) {
    if (-not $script:Opts[$k].Locked) { $script:Opts[$k].On = $false }
  }
  foreach ($n in $indices) {
    if ($n -lt 1 -or $n -gt $script:OptOrder.Count) {
      Write-Warn "Skip out-of-range: $n"
      continue
    }
    $script:Opts[$script:OptOrder[$n - 1]].On = $true
  }
  Enforce-OptDeps
}

function Toggle-Opt([string]$k) {
  if ($script:Opts[$k].Locked) {
    Write-Warn "$($script:Opts[$k].Label) is required and stays enabled."
    return
  }
  $script:Opts[$k].On = -not $script:Opts[$k].On
  Enforce-OptDeps
}

function Normalize-MenuKey([string]$ans) {
  # Latin + Russian lookalikes (Cyrillic Es looks like C; De like Y confirmation "Da").
  if ([string]::IsNullOrWhiteSpace($ans)) { return "" }
  $t = $ans.Trim()
  if ($t.Length -eq 1) {
    $code = [int][char]$t[0]
    # C/c (67/99), Cyrillic ES/es (0x421/0x441)
    if ($code -eq 67 -or $code -eq 99 -or $code -eq 0x421 -or $code -eq 0x441) { return 'C' }
    # A/a (65/97), Cyrillic A/a (0x410/0x430)
    if ($code -eq 65 -or $code -eq 97 -or $code -eq 0x410 -or $code -eq 0x430) { return 'A' }
    # R/r (82/114), Cyrillic ER/er (0x420/0x440) - recommended
    if ($code -eq 82 -or $code -eq 114 -or $code -eq 0x420 -or $code -eq 0x440) { return 'R' }
    # Q/q
    if ($code -eq 81 -or $code -eq 113) { return 'Q' }
    # Y/y, Cyrillic DE/de (Da)
    if ($code -eq 89 -or $code -eq 121 -or $code -eq 0x414 -or $code -eq 0x434) { return 'Y' }
    # N/n, Cyrillic EN/en (Net)
    if ($code -eq 78 -or $code -eq 110 -or $code -eq 0x41D -or $code -eq 0x43D) { return 'N' }
  }
  return $t
}

function Find-LocalServerImage {
  $candidates = @(
    "datasafe-storage-server:latest",
    "ghcr.io/direktorbani/datasafe-storage-server:v1.1.0",
    "ghcr.io/direktorbani/datasafe-storage-server:v1.0.3",
    "ghcr.io/direktorbani/datasafe-storage-server:v1.0.2",
    "ghcr.io/direktorbani/datasafe-storage-server:v1.0.0",
    "datasafe-storage-server:v1.0.0"
  )
  foreach ($img in $candidates) {
    docker image inspect $img 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { return $img }
  }
  $listed = @(docker images --format "{{.Repository}}:{{.Tag}}" 2>$null | Where-Object {
    $_ -match 'datasafe-storage-server' -and $_ -notmatch '<none>'
  })
  if ($listed.Count -gt 0) { return $listed[0] }
  return $null
}

function Ensure-PullProxy {
  $proxyCmd = Join-Path $Root "scripts\ensure-docker-pull-proxy.cmd"
  if (Test-Path $proxyCmd) {
    Write-Info "Checking Docker registry proxy workaround..."
    & cmd /c "`"$proxyCmd`""
  }
}

function Write-OptMenu {
  Write-Host ""
  $i = 1
  foreach ($k in $script:OptOrder) {
    $o = $script:Opts[$k]
    $mark = if ($o.On) { "x" } else { " " }
    $lock = if ($o.Locked) { " (required)" } else { "" }
    Write-Host ("  [{0}] {1}. {2}{3}" -f $mark, $i, $o.Label, $lock)
    $i++
  }
  $sel = @()
  foreach ($k in $script:OptOrder) {
    if ($script:Opts[$k].On) { $sel += $k }
  }
  Write-Host ("  --> selected: " + ($sel -join ", ")) -ForegroundColor DarkGray
}

function Get-SelectedProfileNames {
  $sel = @()
  foreach ($k in $script:OptOrder) {
    if ($script:Opts[$k].On) { $sel += $k }
  }
  return $sel
}

function Write-SelectionConfirmed {
  $sel = Get-SelectedProfileNames
  Write-Ok ("Confirmed selection: " + ($sel -join ", "))
}

function Show-Menu {
  Write-Title "3) What to install"
  Write-Host "  How to choose:"
  Write-Host "    5           toggle one item"
  Write-Host "    1,2,3,4     SET selection and continue (core always on)"
  Write-Host "    R           recommended (1-4) and continue"
  Write-Host "    A           all options and continue"
  Write-Host "    C           confirm current selection and continue"
  Write-Host "    Q           quit"
  Write-OptMenu
  while ($true) {
    Write-Host ""
    $raw = (Read-Host "  Select").Trim()
    if ([string]::IsNullOrWhiteSpace($raw)) {
      Write-OptMenu
      continue
    }
    $ans = Normalize-MenuKey $raw
    if ($ans -eq 'Q') { throw "Aborted by user." }
    if ($ans -eq 'C') {
      Enforce-OptDeps
      Write-SelectionConfirmed
      break
    }
    if ($ans -eq 'R') {
      Set-OptSelection @(1, 2, 3, 4)
      Write-SelectionConfirmed
      break
    }
    if ($ans -eq 'A') {
      $all = @(1..$script:OptOrder.Count)
      Set-OptSelection $all
      Write-SelectionConfirmed
      break
    }
    if ($raw -match '^[\d,\s;]+$') {
      $parts = @($raw -split '[\s,;]+' | Where-Object { $_ -ne "" })
      $nums = @()
      foreach ($p in $parts) {
        if ($p -notmatch '^\d+$') { Write-Warn "Not a number: $p"; continue }
        $nums += [int]$p
      }
      if ($nums.Count -eq 0) {
        Write-Warn "No valid numbers."
        continue
      }
      if ($nums.Count -eq 1) {
        Toggle-Opt $script:OptOrder[$nums[0] - 1]
        Write-OptMenu
        continue
      }
      # Multi-number SET = final selection → confirm and leave menu (no second Select)
      Set-OptSelection $nums
      Write-SelectionConfirmed
      break
    }
    Write-Warn "Unknown input. Examples: 5  |  1,2,3,4  |  R  |  A  |  C"
  }
}

function Show-Plan([string]$dataRootPath) {
  Write-Title "4) Summary"
  $enabled = @()
  foreach ($k in $script:OptOrder) {
    if ($script:Opts[$k].On) { $enabled += $k }
  }
  Write-Info ("Profiles: " + ($enabled -join ", "))
  Write-Info "Project:  $ProjectName"
  Write-Info "Data:     $dataRootPath"
  if ($script:Opts["binary"].On) {
    Write-Info "Images:   local build (storage-server-linux + web/console/dist)"
  } else {
    Write-Info "Images:   ghcr.io/direktorbani/datasafe-storage-server:$Tag (+ console)"
  }
  $files = @("docker-compose.yml")
  if ($script:Opts["data"].On) { $files += "docker-compose.local-data.yml" }
  if ($script:Opts["binary"].On) { $files += "deploy/compose/docker-compose.local-binary.yml" }
  Write-Info ("Compose:  " + ($files -join " + "))
  Write-Host ""
  if (-not $Yes) {
    $ok = Normalize-MenuKey (Read-Host "  Proceed with installation? [Y/n]")
    if ($ok -eq 'N') { throw "Aborted by user." }
  }
}

function Set-EnvKey([string]$path, [string]$key, [string]$value) {
  $lines = @(Get-Content -LiteralPath $path -ErrorAction Stop)
  $found = $false
  $out = foreach ($line in $lines) {
    if ($line -match ("^\s*" + [regex]::Escape($key) + "\s*=")) {
      $found = $true
      "$key=$value"
    } else { $line }
  }
  if (-not $found) { $out += "$key=$value" }
  Set-Content -LiteralPath $path -Value $out -Encoding utf8
}

function Ensure-EnvFile([string]$dataRootPath) {
  $envPath = Join-Path $Root ".env"
  if (-not (Test-Path $envPath)) {
    Copy-Item (Join-Path $Root ".env.example") $envPath
    Write-Ok "Created .env from .env.example"
  } else {
    Write-Ok ".env already exists (keys will be patched, not replaced wholesale)"
  }
  Set-EnvKey $envPath "DATASAFE_DATA_ROOT" $dataRootPath
  if ($script:Opts["postgres"].On) {
    Set-EnvKey $envPath "STORAGE_METADATA_BACKEND" "postgres"
  }
  if ($script:Opts["binary"].On) {
    $localImg = Find-LocalServerImage
    if (-not $localImg) {
      throw @"
No local storage-server image found, and binary mode must not rebuild (Docker Hub/GHCR pull is broken via proxy 127.0.0.1:10801).
Fix: run scripts\ensure-docker-pull-proxy.cmd, or pull once:
  docker pull ghcr.io/direktorbani/datasafe-storage-server:v1.1.0
Then re-run installer with binary selected.
"@
    }
    Set-EnvKey $envPath "DATASAFE_SERVER_IMAGE" $localImg
    Write-Ok "Binary mode base image: $localImg (no docker build)"
  } else {
    Set-EnvKey $envPath "DATASAFE_SERVER_IMAGE" "ghcr.io/direktorbani/datasafe-storage-server:$Tag"
    Set-EnvKey $envPath "DATASAFE_CONSOLE_IMAGE" "ghcr.io/direktorbani/datasafe-console:$Tag"
  }
  if ($script:Opts["identity"].On) {
    # Docker network DNS after start-*-test.cmd joins datasafe_default; host.docker.internal as fallback.
    Set-EnvKey $envPath "STORAGE_LDAP_ENABLED" "true"
    Set-EnvKey $envPath "STORAGE_LDAP_URL" "ldap://datasafe-ldap-test:389"
    Set-EnvKey $envPath "STORAGE_LDAP_BIND_DN" "cn=admin,dc=datasafe,dc=local"
    Set-EnvKey $envPath "STORAGE_LDAP_BIND_PASSWORD" "ldapadmin"
    Set-EnvKey $envPath "STORAGE_LDAP_BASE_DN" "ou=users,dc=datasafe,dc=local"
    Set-EnvKey $envPath "STORAGE_LDAP_GROUP_DN" "ou=groups,dc=datasafe,dc=local"
    Set-EnvKey $envPath "STORAGE_LDAP_REQUIRE_TLS" "false"
    Set-EnvKey $envPath "STORAGE_OIDC_ENABLED" "true"
    Set-EnvKey $envPath "STORAGE_OIDC_ISSUER" "http://localhost:8180/realms/datasafe"
    Set-EnvKey $envPath "STORAGE_OIDC_INTERNAL_ISSUER" "http://datasafe-keycloak-test:8080/realms/datasafe"
    Set-EnvKey $envPath "STORAGE_OIDC_CLIENT_ID" "datasafe-console"
    Set-EnvKey $envPath "STORAGE_OIDC_CLIENT_SECRET" "datasafe-console-secret"
    Set-EnvKey $envPath "STORAGE_OIDC_REDIRECT_URL" "http://localhost:8080/api/v1/auth/oidc/callback"
    Set-EnvKey $envPath "STORAGE_OIDC_ROPC_ENABLED" "true"
    Write-Ok "Identity lab env keys patched (LDAP + OIDC test defaults)"
  }
}

function Connect-IdentitySidecars {
  $net = "${ProjectName}_default"
  foreach ($c in @("datasafe-ldap-test", "datasafe-keycloak-test")) {
    $running = docker ps --filter "name=^/${c}$" --format "{{.Names}}" 2>$null
    if (-not $running) {
      Write-Warn "$c not running - start with scripts\start-ldap-test.cmd / start-keycloak-test.cmd"
      continue
    }
    # Already attached is OK; docker may write to stderr (ErrorAction Stop would abort install).
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    docker network connect $net $c 2>&1 | Out-Null
    $ErrorActionPreference = $prev
    Write-Ok "$c on network $net (or already attached)"
  }
}

function Enable-IdentitySettingsViaApi {
  Write-Title "Identity settings"
  $api = "http://127.0.0.1:9000"
  $tmp = Join-Path $env:TEMP ("datasafe-id-" + [guid]::NewGuid().ToString("n"))
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  try {
    $loginBody = '{"username":"admin","password":"admin"}'
    $loginPath = Join-Path $tmp "login.json"
    [System.IO.File]::WriteAllText($loginPath, $loginBody)
    $loginRaw = & curl.exe -s -S -X POST "$api/api/v1/admin/login" -H "Content-Type: application/json" --data-binary "@$loginPath"
    $login = $loginRaw | ConvertFrom-Json
    if (-not $login.token) {
      Write-Warn "admin login failed - finish setup wizard, then configure LDAP/OIDC in Settings"
      Write-Warn ("response: " + $loginRaw)
      return
    }
    $token = [string]$login.token
    $sysPath = Join-Path $tmp "system.json"
    & curl.exe -s -S "$api/api/v1/settings/system" -H "Authorization: Bearer $token" -o $sysPath
    if (-not (Test-Path $sysPath) -or ((Get-Item $sysPath).Length -lt 2)) {
      Write-Warn "GET /settings/system failed"
      return
    }
    $cfg = Get-Content $sysPath -Raw -Encoding utf8 | ConvertFrom-Json
    $cfg.ldap = [pscustomobject]@{
      enabled               = $true
      url                   = "ldap://datasafe-ldap-test:389"
      bind_dn               = "cn=admin,dc=datasafe,dc=local"
      bind_password         = "ldapadmin"
      base_dn               = "ou=users,dc=datasafe,dc=local"
      group_dn              = "ou=groups,dc=datasafe,dc=local"
      user_attr             = "cn"
      sync_on_login         = $true
      sync_interval_minutes = 60
    }
    $cfg.oidc = [pscustomobject]@{
      enabled         = $true
      issuer          = "http://localhost:8180/realms/datasafe"
      internal_issuer = "http://datasafe-keycloak-test:8080/realms/datasafe"
      client_id       = "datasafe-console"
      client_secret   = "datasafe-console-secret"
      redirect_url    = "http://localhost:8080/api/v1/auth/oidc/callback"
      groups_claim    = "groups"
    }
    $putPath = Join-Path $tmp "put.json"
    [System.IO.File]::WriteAllText($putPath, ($cfg | ConvertTo-Json -Depth 30 -Compress))
    $putOut = Join-Path $tmp "put-out.json"
    $putCode = & curl.exe -s -o $putOut -w "%{http_code}" -X PUT "$api/api/v1/settings/system" -H "Authorization: Bearer $token" -H "Content-Type: application/json" --data-binary "@$putPath"
    if ($putCode -ne "200") {
      Write-Warn "PUT settings/system HTTP $putCode - check console Settings manually"
      Get-Content $putOut -ErrorAction SilentlyContinue | Select-Object -First 5
      return
    }
    Write-Ok "LDAP + OIDC enabled in system settings"

    $testPath = Join-Path $tmp "ldap-test.json"
    [System.IO.File]::WriteAllText($testPath, "{}")
    $ldapTest = & curl.exe -s -w "|HTTP:%{http_code}" -X POST "$api/api/v1/settings/ldap/test" -H "Authorization: Bearer $token" -H "Content-Type: application/json" --data-binary "@$testPath"
    Write-Info ("LDAP test: " + $ldapTest)

    $ldapLogin = Join-Path $tmp "ldap-login.json"
    [System.IO.File]::WriteAllText($ldapLogin, '{"username":"ldapuser","password":"password"}')
    $ldapOut = Join-Path $tmp "ldap-login-out.json"
    $ldapCode = & curl.exe -s -o $ldapOut -w "%{http_code}" -X POST "$api/api/v1/admin/login" -H "Content-Type: application/json" --data-binary "@$ldapLogin"
    if ($ldapCode -eq "200") { Write-Ok "LDAP login ldapuser OK" }
    else { Write-Warn "LDAP login HTTP $ldapCode (may need sync or setup_required)" }

    $oidcCfg = & curl.exe -s "$api/api/v1/auth/oidc/config"
    if ($oidcCfg -match '"enabled"\s*:\s*true') { Write-Ok "OIDC config enabled for SSO button" }
    else { Write-Warn ("OIDC config: " + $oidcCfg) }

    $ropc = Join-Path $tmp "ropc.json"
    [System.IO.File]::WriteAllText($ropc, '{"username":"ssouser","password":"password"}')
    $ropcOut = Join-Path $tmp "ropc-out.json"
    $ropcCode = & curl.exe -s -o $ropcOut -w "%{http_code}" -X POST "$api/api/v1/auth/oidc/password-login" -H "Content-Type: application/json" --data-binary "@$ropc"
    if ($ropcCode -eq "200") { Write-Ok "OIDC ROPC ssouser OK" }
    else { Write-Warn "OIDC ROPC HTTP $ropcCode - ensure Keycloak ready and STORAGE_OIDC_ROPC_ENABLED=true" }
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
}

function New-DataDirs([string]$dataRootPath) {
  $win = $dataRootPath -replace '/', '\'
  New-Item -ItemType Directory -Force -Path "$win\storage", "$win\postgres" | Out-Null
  Write-Ok "Data directories under $dataRootPath"
}

function Build-LocalBinary {
  Write-Title "Build from source"
  Write-Info "Building Linux storage-server..."
  $env:CGO_ENABLED = "0"
  $env:GOOS = "linux"
  $env:GOARCH = "amd64"
  go build -trimpath -ldflags="-s -w" -o deploy\docker\storage-server-linux .\cmd\storage-server
  if ($LASTEXITCODE -ne 0) { throw "go build failed" }
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Write-Ok "deploy/docker/storage-server-linux"
  if (-not (Test-Path "web\console\dist\index.html")) {
    Write-Info "Building web console..."
    & cmd /c "scripts\build-console.cmd"
    if ($LASTEXITCODE -ne 0) { throw "console build failed" }
  } else {
    Write-Ok "web/console/dist already present"
  }
}

function Start-IdentityLab {
  Write-Title "Identity lab"
  Write-Info "Starting LDAP..."
  & cmd /c "scripts\start-ldap-test.cmd"
  Write-Info "Starting Keycloak..."
  & cmd /c "scripts\start-keycloak-test.cmd"
  Write-Ok "LDAP + Keycloak sidecars requested (see script output for URLs)"
}

function Get-ComposeArgs {
  $args = @("compose", "-p", $ProjectName, "-f", "docker-compose.yml")
  if ($script:Opts["data"].On) { $args += @("-f", "docker-compose.local-data.yml") }
  if ($script:Opts["binary"].On) { $args += @("-f", "deploy/compose/docker-compose.local-binary.yml") }
  if ($script:Opts["postgres"].On) { $args += @("--profile", "postgres") }
  return ,$args
}

function Get-ComposeServices {
  $services = @("storage-server", "caddy")
  if ($script:Opts["postgres"].On) { $services = @("postgres") + $services }
  if ($script:Opts["monitoring"].On) { $services += @("prometheus", "grafana") }
  return ,$services
}

function Invoke-ComposeUp {
  Write-Title "5) Apply"
  $args = Get-ComposeArgs
  $services = Get-ComposeServices

  if ($DryRun) {
    Write-Info ("DRY-RUN docker " + ($args -join " ") + " config")
    & docker @args config --quiet
    if ($LASTEXITCODE -ne 0) { throw "docker compose config failed" }
    Write-Ok "Compose config valid"
    Write-Info ("Would up -d: " + ($services -join ", "))
    return
  }

  Ensure-PullProxy

  # Binary mode: never rebuild (avoids golang pull via broken WinHTTP proxy).
  $upArgs = $args + @("up", "-d")
  if ($script:Opts["binary"].On) {
    $upArgs += "--no-build"
    Write-Info "Using --no-build (local binary overlay)"
  }
  $upArgs += $services

  Write-Info ("docker " + ($upArgs -join " "))
  & docker @upArgs
  if ($LASTEXITCODE -ne 0) {
    throw @"
docker compose up failed.
If the error mentions 127.0.0.1:10801 or registry pull/build:
  1) scripts\ensure-docker-pull-proxy.cmd
  2) or: netsh winhttp reset proxy  (Admin) + restart Docker Desktop
  3) or re-run installer WITH option 5 (local binary) after a local image exists
     (you already have datasafe-storage-server images locally)
"@
  }
  Write-Ok "Compose up requested"
}

function Wait-Healthy {
  Write-Title "6) Verify"
  $url = "http://127.0.0.1:8080/healthz"
  Write-Info "Waiting for $url ..."
  for ($i = 1; $i -le 90; $i++) {
    try {
      $r = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3
      if ($r.StatusCode -eq 200) {
        Write-Ok "Console edge healthy"
        return
      }
    } catch {}
    Start-Sleep -Seconds 2
  }
  Write-Warn "healthz not ready yet - check: docker compose -p datasafe ps / logs"
}

function Show-Done {
  Write-Title "Done"
  Write-Host ""
  Write-Host "  URLs" -ForegroundColor Cyan
  Write-Host "  Console:  http://localhost:8080"
  Write-Host "  S3 API:   http://localhost:9000"
  if ($script:Opts["monitoring"].On) {
    Write-Host "  Grafana:  http://localhost:3000"
  }
  if ($script:Opts["identity"].On) {
    Write-Host "  Keycloak: http://localhost:8180/admin"
    Write-Host "  LDAP:     ldap://localhost:389"
  }
  Write-Host ""
  Write-Host "  Pre-created users / passwords" -ForegroundColor Cyan
  Write-Host "  Console (local):     admin / admin          (change on first login)"
  if ($script:Opts["monitoring"].On) {
    Write-Host "  Grafana:             admin / admin"
  }
  if ($script:Opts["identity"].On) {
    Write-Host "  Keycloak admin:      admin / admin"
    Write-Host "  LDAP bind DN:        cn=admin,dc=datasafe,dc=local / ldapadmin"
    Write-Host "  LDAP test user:      ldapuser / password"
    Write-Host "  LDAP test admin:     ldapadmin / password"
    Write-Host "  SSO / Keycloak user: ssouser / password"
  }
  Write-Host ""
  Write-Host "  Next: open console -> change password -> finish setup wizard."
  Write-Host "  Docs: docs/getting-started/en/onboarding.md"
  Write-Host ""
}

# --- main ---
try {
  Show-Detect
  [void](Ensure-Docker)
  $dataRootPath = Get-DefaultDataRoot

  $installMode = 'single'
  if ($Cluster) {
    $installMode = 'cluster'
  } elseif (-not $Yes) {
    . (Join-Path $Root "scripts\cluster\ClusterWizard.ps1")
    $installMode = Read-ClusterModeChoice
  }

  if ($installMode -eq 'cluster') {
    . (Join-Path $Root "scripts\cluster\ClusterWizard.ps1")
    Write-Title "Cluster mode (Wave 1)"
    Write-Info "Patroni / NFS / multi-LB Apply ships in Wave 2 - this wave collects inventory securely."
    if ($Yes -and -not $DryRun) {
      throw "Non-interactive Cluster Apply is Wave 2. Use: .\install.ps1 -Cluster -DryRun (after providing inventory via wizard interactively), or run scripts\tests\cluster-installer-w1.ps1"
    }
    if ($Yes -and $DryRun) {
      Write-Warn "DryRun + -Yes: run Wave 1+2 schema asserts; interactive wizard skipped"
      & (Join-Path $Root "scripts\tests\cluster-installer-w1.ps1")
      if ($LASTEXITCODE -ne 0) { throw "cluster Wave 1 asserts failed" }
      & (Join-Path $Root "scripts\tests\cluster-installer-w2.ps1")
      if ($LASTEXITCODE -ne 0) { throw "cluster Wave 2 asserts failed" }
      Write-Title "DryRun complete"
      Write-Ok "Cluster Wave 1+2 validation OK"
      exit 0
    }
    [void](Invoke-ClusterWizardWave1 -DryRun:$DryRun)
    exit 0
  }

  if ($Yes) {
    if ($Profiles) { Set-ProfilesFromString $Profiles }
    else { Set-ProfilesFromString "core,postgres,monitoring,data" }
  } else {
    if ($Profiles) { Set-ProfilesFromString $Profiles }
    Show-Menu
    Write-Host ""
    Write-Host "  Where to store data on the host (Postgres + object files):"
    $dr = Read-Host "  Data root [$dataRootPath]"
    if (-not [string]::IsNullOrWhiteSpace($dr)) { $dataRootPath = ($dr.Trim() -replace '\\', '/') }
  }

  Show-Plan $dataRootPath
  if ($DryRun) {
    Write-Warn "DryRun: skip .env write, builds, sidecars, and compose up - validating compose only"
  } else {
    Ensure-EnvFile $dataRootPath
    if ($script:Opts["data"].On) { New-DataDirs $dataRootPath }
    if ($script:Opts["binary"].On) { Build-LocalBinary }
    if ($script:Opts["identity"].On) { Start-IdentityLab }
  }
  # Still need Opts set for compose args; for DryRun+binary overlay, binary file must exist or config may warn
  if ($DryRun -and $script:Opts["binary"].On -and -not (Test-Path "deploy\docker\storage-server-linux")) {
    Write-Warn "local-binary overlay selected but storage-server-linux missing - config may still parse"
  }
  Invoke-ComposeUp
  if (-not $DryRun) {
    if ($script:Opts["identity"].On) { Connect-IdentitySidecars }
    Wait-Healthy
    if ($script:Opts["identity"].On) { Enable-IdentitySettingsViaApi }
    Show-Done
  } else {
    Write-Title "DryRun complete"
    Write-Ok "Configuration validated"
  }
  exit 0
} catch {
  Write-Err $_.Exception.Message
  exit 1
}
