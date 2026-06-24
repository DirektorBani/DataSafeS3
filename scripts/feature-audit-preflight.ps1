# Reset admin MFA/quota state before feature audit (optional hygiene).
param([string]$BaseUrl = 'http://localhost:8080')

$ErrorActionPreference = 'Stop'

function Invoke-DS {
    param([string]$Method, [string]$Url, [hashtable]$Headers = @{}, [string]$Body = $null)
    $tmp = $null
    $curlArgs = @('-s', '-w', "`n%{http_code}", '-X', $Method, $Url)
    foreach ($k in $Headers.Keys) { $curlArgs += @('-H', "${k}: $($Headers[$k])") }
    if ($null -ne $Body) {
        $tmp = [System.IO.Path]::GetTempFileName()
        [System.IO.File]::WriteAllText($tmp, $Body, [System.Text.UTF8Encoding]::new($false))
        $curlArgs += @('-H', 'Content-Type: application/json', '--data-binary', "@$tmp")
    }
    try {
        $out = (& curl.exe @curlArgs | Out-String).Trim()
    } finally {
        if ($tmp) { Remove-Item -Force -ErrorAction SilentlyContinue $tmp }
    }
    $lines = $out -split "`n"
    $code = [int]$lines[-1]
    $text = ($lines[0..($lines.Count-2)] -join "`n").Trim()
    $json = $null
    if ($text) { try { $json = $text | ConvertFrom-Json } catch {} }
    return @{ Code=$code; Body=$text; Json=$json }
}

if ($env:AUDIT_RESET_ADMIN -ne '1') {
    Write-Host 'AUDIT_RESET_ADMIN not set; skipping preflight reset.'
    exit 0
}

Write-Host '=== DataSafeS3 audit preflight ===' -ForegroundColor Cyan

$login = Invoke-DS POST "$BaseUrl/api/v1/admin/login" -Body '{"username":"admin","password":"admin"}'
if (-not $login.Json.token) {
    Write-Host "Admin login failed (HTTP $($login.Code)); preflight skipped."
    exit 0
}
$adminH = @{ Authorization = "Bearer $($login.Json.token)" }

$users = Invoke-DS GET "$BaseUrl/api/v1/users" -Headers $adminH
$admin = @($users.Json.users) | Where-Object { $_.username -eq 'admin' } | Select-Object -First 1
if ($admin) {
    $body = "{`"username`":`"admin`",`"email`":`"$($admin.email)`",`"role`":`"$($admin.role)`",`"status`":`"$($admin.status)`",`"max_size_bytes`":0,`"max_objects`":0}"
    $ur = Invoke-DS PUT "$BaseUrl/api/v1/users/$($admin.id)" -Headers $adminH -Body $body
    Write-Host "Admin quota reset: HTTP $($ur.Code)"
}

$sys = Invoke-DS GET "$BaseUrl/api/v1/settings/system" -Headers $adminH
if ($sys.Code -eq 200) {
    $cfg = $sys.Json
    if ($cfg.mfa) {
        $cfg.mfa.require_admin_mfa = $false
    }
    Invoke-DS PUT "$BaseUrl/api/v1/settings/system" -Headers $adminH -Body ($cfg | ConvertTo-Json -Depth 20 -Compress) | Out-Null
    Write-Host 'Admin MFA policy disabled for audit.'
}

Write-Host 'Preflight complete.'
