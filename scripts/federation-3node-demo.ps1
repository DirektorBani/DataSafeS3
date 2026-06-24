# Federation 3-node demo smoke (Gateway federation pattern)
# Verifies federation registry API and optional peer connectivity.
param(
  [string]$BaseUrl = "http://localhost:8080/api/v1",
  [string]$AdminUser = "admin",
  [string]$AdminPass = "admin"
)

$ErrorActionPreference = "Stop"

function Write-Step($msg) { Write-Host "[federation-demo] $msg" }

Write-Step "Logging in as $AdminUser"
$loginBody = @{ username = $AdminUser; password = $AdminPass } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$BaseUrl/admin/login" -Method POST -ContentType "application/json" -Body $loginBody
if (-not $login.token) {
  if ($login.mfa_required) {
    Write-Step "SKIP: MFA required - set require_admin_mfa=false for demo or use enrolled admin"
    exit 0
  }
  throw "Login failed"
}
$headers = @{ Authorization = "Bearer $($login.token)" }

Write-Step "Listing federation clusters"
$clusters = Invoke-RestMethod -Uri "$BaseUrl/federation/clusters" -Headers $headers
$count = @($clusters.clusters).Count
Write-Step "Registered peers: $count"

if ($count -eq 0) {
  Write-Step "No peers registered - registering local health endpoint as demo peer"
  $peerBody = @{
    id = "demo-local"
    name = "Local node"
    endpoint = "http://localhost:9000"
    region = "local"
    capabilities = @("read", "list")
  } | ConvertTo-Json
  Invoke-RestMethod -Uri "$BaseUrl/federation/clusters" -Method POST -Headers $headers -ContentType "application/json" -Body $peerBody | Out-Null
  $clusters = Invoke-RestMethod -Uri "$BaseUrl/federation/clusters" -Headers $headers
}

foreach ($c in $clusters.clusters) {
  Write-Step "Testing connectivity: $($c.name) ($($c.endpoint))"
  $test = Invoke-RestMethod -Uri "$BaseUrl/federation/clusters/$($c.id)/test" -Method POST -Headers $headers
  Write-Step "  status=$($test.status) detail=$($test.detail)"
}

Write-Step "PASS: federation 3-node demo smoke completed"
exit 0
