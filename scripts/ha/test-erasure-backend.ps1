# Erasure backend smoke test (requires ha-erasure compose stack)
param(
    [string]$BaseUrl = "http://127.0.0.1:8082"
)
$ErrorActionPreference = "Stop"

function Record($Step, $Status, $Notes) {
    $icon = if ($Status -eq "PASS") { "[PASS]" } else { "[FAIL]" }
    Write-Host "$icon $Step - $Notes"
    if ($Status -ne "PASS") { exit 1 }
}

function Ensure-SetupComplete {
    $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
    if (-not $login.token) { throw "login failed" }
    $status = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status"
    if ($status.initial_setup_completed) { return $login.token }
    $h = @{ Authorization = "Bearer $($login.token)" }
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h -ContentType "application/json" -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/complete" -Method POST -Headers $h | Out-Null
    $login2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"Admin123!"}'
    if ($login2.token) {
        $h2 = @{ Authorization = "Bearer $($login2.token)" }
        Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h2 -ContentType "application/json" -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
        $login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
    }
    return $login.token
}

try {
    $h = Invoke-RestMethod -Uri "$BaseUrl/healthz"
    Record "healthz object_backend=erasure" $(if ($h.object_backend -eq "erasure") { "PASS" } else { "FAIL" }) "backend=$($h.object_backend)"
    Record "healthz erasure_degraded=false" $(if ($h.erasure_degraded -eq $false) { "PASS" } else { "FAIL" }) "degraded=$($h.erasure_degraded)"

    $tok = Ensure-SetupComplete
    $auth = @{ Authorization = "Bearer $tok" }
    $bucket = "erasure-test-$(Get-Random)"
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$bucket" -Method POST -Headers $auth -ContentType "application/json" -Body '{"visibility":"private"}' | Out-Null
    $payload = "erasure payload $(Get-Date -Format o)"
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/test.txt" -Method PUT -Headers $auth -ContentType "text/plain" -Body $payload | Out-Null
    $got = Invoke-WebRequest -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/test.txt" -Headers $auth -UseBasicParsing
    $gotBody = if ($got.Content -is [byte[]]) { [System.Text.Encoding]::UTF8.GetString($got.Content) } else { [string]$got.Content }
    Record "PUT/GET object" $(if ($got.StatusCode -eq 200 -and $gotBody -eq $payload) { "PASS" } else { "FAIL" }) "status=$($got.StatusCode)"
    & "$PSScriptRoot\migrate-fs-to-erasure.ps1" -DryRun -BaseUrl $BaseUrl | Out-Null
    Record "migrate-fs-to-erasure dry-run" "PASS" "ok"
    Write-Host "Erasure backend smoke OK"
} catch {
    Record "Erasure backend smoke" "FAIL" $_.Exception.Message
    exit 1
}
