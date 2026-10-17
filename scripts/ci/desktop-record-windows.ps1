# Build the current storage-server and run contract/record.sh under Git bash.
# Git for Windows must not treat temp-file paths as excluded from MSYS conversion.
# Windows PowerShell 5.1 runs this file. No server source is modified.
$ErrorActionPreference = "Stop"
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -Scope Global -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $repo

$bash = Join-Path $env:ProgramFiles "Git\bin\bash.exe"
if (-not (Test-Path -LiteralPath $bash)) {
    throw "Git bash was not found at $bash"
}

$port = 19171
$base = "http://127.0.0.1:$port"
$work = Join-Path $env:RUNNER_TEMP "datasafe-record"
if (Test-Path -LiteralPath $work) {
    Remove-Item -LiteralPath $work -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $work | Out-Null
$data = Join-Path $work "data"
New-Item -ItemType Directory -Force -Path $data | Out-Null
$exe = Join-Path $work "storage-server.exe"

Write-Host "go build $exe"
& go build -o $exe ./cmd/storage-server
if ($LASTEXITCODE -ne 0) {
    throw "go build ./cmd/storage-server failed ($LASTEXITCODE)"
}

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $exe
$psi.WorkingDirectory = $work
$psi.UseShellExecute = $false
$psi.EnvironmentVariables["STORAGE_ADDR"] = ":$port"
$psi.EnvironmentVariables["STORAGE_DATA_DIR"] = $data
$psi.EnvironmentVariables["STORAGE_DEV"] = "true"
$psi.EnvironmentVariables["STORAGE_LOG_LEVEL"] = "info"
$psi.EnvironmentVariables["STORAGE_REGION"] = "us-east-1"
$psi.EnvironmentVariables["STORAGE_ACCESS_KEY"] = "datasafe"
$psi.EnvironmentVariables["STORAGE_SECRET_KEY"] = "datasafesecret"
$psi.EnvironmentVariables["STORAGE_ADMIN_USER"] = "admin"
$psi.EnvironmentVariables["STORAGE_ADMIN_PASSWORD"] = "admin"
$psi.EnvironmentVariables["STORAGE_JWT_SECRET"] = "datasafe-contract"
$psi.EnvironmentVariables["STORAGE_AUTO_HOME_BUCKET"] = "false"
$server = [Diagnostics.Process]::Start($psi)

function Stop-Server {
    if ($server -and -not $server.HasExited) {
        & "$env:SystemRoot\System32\taskkill.exe" /F /T /PID $server.Id | Out-Null
        $deadline = [DateTime]::UtcNow.AddSeconds(15)
        while (-not $server.HasExited -and [DateTime]::UtcNow -lt $deadline) {
            Start-Sleep -Milliseconds 200
        }
    }
}

try {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    $up = $false
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($server.HasExited) {
            throw "storage-server exited $($server.ExitCode) before healthz"
        }
        try {
            $health = Invoke-WebRequest -Uri "$base/healthz" -UseBasicParsing -TimeoutSec 2
            if ($health.StatusCode -eq 200) {
                $up = $true
                break
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $up) {
        throw "storage-server did not answer $base/healthz"
    }

    function Invoke-Api {
        param($Method, $Path, $Token, $JsonBody)
        $headers = @{}
        if ($Token) {
            $headers["Authorization"] = "Bearer $Token"
        }
        $params = @{
            Method      = $Method
            Uri         = "$base$Path"
            Headers     = $headers
            ContentType = "application/json"
        }
        if ($JsonBody) {
            $params["Body"] = $JsonBody
        }
        return Invoke-RestMethod @params
    }

    function Get-Token([string]$Password) {
        $body = '{"username":"admin","password":"' + $Password + '"}'
        $login = Invoke-Api -Method POST -Path "/api/v1/admin/login" -JsonBody $body
        if (-not $login.token) {
            throw "login did not return a token"
        }
        return [string]$login.token
    }

    $token = Get-Token "admin"
    Invoke-Api -Method POST -Path "/api/v1/me/password" -Token $token -JsonBody '{"current_password":"admin","new_password":"admin-e2e-temp-1"}' | Out-Null
    $token = Get-Token "admin-e2e-temp-1"
    Invoke-Api -Method POST -Path "/api/v1/me/password" -Token $token -JsonBody '{"current_password":"admin-e2e-temp-1","new_password":"admin"}' | Out-Null
    $token = Get-Token "admin"
    Invoke-Api -Method POST -Path "/api/v1/setup/complete" -Token $token -JsonBody "{}" | Out-Null
    Write-Host "setup complete"

    $contract = Join-Path $work "contract"
    New-Item -ItemType Directory -Force -Path $contract | Out-Null
    Copy-Item -LiteralPath (Join-Path $repo "clients\desktop\contract\record.sh") -Destination (Join-Path $contract "record.sh")
    $wrapper = ([System.IO.Path]::GetFullPath((Join-Path $repo "scripts\ci\desktop-record-gitbash.sh"))) -replace '\\', '/'
    $contractMsys = ([System.IO.Path]::GetFullPath($contract)) -replace '\\', '/'
    Write-Host "record.sh via $bash"
    & $bash $wrapper $contractMsys $base "1.6.0"
    if ($LASTEXITCODE -ne 0) {
        throw "record.sh exited $LASTEXITCODE"
    }

    $loginPath = Join-Path $contract "fixtures\1.6.0\login.json"
    if (-not (Test-Path -LiteralPath $loginPath)) {
        throw "record.sh did not write $loginPath"
    }
    $login = Get-Content -Raw -LiteralPath $loginPath | ConvertFrom-Json
    if ($login.status -ne 200) {
        throw "recorded login status $($login.status)"
    }
    if ($login.body.token -ne "<masked>") {
        throw "recorded login token was not masked"
    }
    if ($null -eq $login.body.PSObject.Properties["mfa_enabled"]) {
        throw "recorded login body has no mfa_enabled"
    }
    if ([string]::IsNullOrEmpty([string]$login.body.user_id)) {
        throw "recorded login body has no user_id"
    }
    $rawLogin = [System.IO.File]::ReadAllBytes($loginPath)
    foreach ($byte in $rawLogin) {
        if ($byte -eq 13) {
            throw "record.sh wrote CR in $loginPath"
        }
    }
    Write-Host "record.sh exit 0; login token masked; user_id present; fixtures are LF"
} finally {
    Stop-Server
}
