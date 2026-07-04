# Gateway (external S3 / MinIO) + Federation peers for HA lab (project datasafe-ha).
param(
    [string]$ConsoleUrl = "http://127.0.0.1:8082",
    [string]$AdminUser = "admin",
    [string]$AdminPass = "admin",
    [string]$MinioHostPort = "9100",
    [string]$MinioEndpoint = "http://host.docker.internal:9100",
    [string]$ConnectionName = "External S3 Test",
    [string]$RemoteBucket = "replica-test",
    [string]$SourceBucket = "ha-demo",
    [string]$HaProject = "datasafe-ha",
    [switch]$SkipMinioStart,
    [switch]$RegisterClusterB,
    [string]$ClusterBName = "Remote Cluster B",
    [string]$ClusterBEndpoint = "http://datasafe-b-storage-server-1:9000"
)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $Root

$MinioContainer = "datasafe-minio-test"
$MinioConsolePort = "9101"

function Write-HaStep($msg) { Write-Host "[ha-integrations] $msg" -ForegroundColor Cyan }

function Register-FederationPeer {
    param([hashtable]$Peer, [array]$ExistingList, [hashtable]$AuthHeaders, [string]$BaseConsoleUrl)
    $p = $Peer
    $match = $ExistingList | Where-Object { $_.name -eq $p.name } | Select-Object -First 1
    if ($match -and $match.endpoint -eq $p.endpoint) {
        Write-HaStep "Federation peer already registered: $($p.name)"
        return
    }
    if ($match) {
        Write-HaStep "Updating federation peer endpoint: $($p.name)"
        Invoke-RestMethod -Uri "$BaseConsoleUrl/api/v1/federation/clusters/$($match.id)" -Method DELETE -Headers $AuthHeaders | Out-Null
    }
    $body = @{
        name = $p.name
        endpoint = $p.endpoint
        region = $p.region
        capabilities = @("read", "list")
    } | ConvertTo-Json -Compress
    $created = Invoke-RestMethod -Uri "$BaseConsoleUrl/api/v1/federation/clusters" -Method POST -Headers $AuthHeaders -ContentType "application/json" -Body $body
    Write-HaStep "Registered federation peer: $($created.cluster.name) -> $($created.cluster.endpoint)"
}

if (-not $SkipMinioStart) {
    Write-HaStep "Starting external S3 (MinIO) on :$MinioHostPort ..."
    $startMinio = Join-Path $Root "scripts\start-minio-test.cmd"
    if (-not (Test-Path $startMinio)) { throw "Missing $startMinio" }
    cmd /c "`"$startMinio`""
    if ($LASTEXITCODE -ne 0) { throw "start-minio-test.cmd failed" }

    $net = "${HaProject}_default"
    docker network inspect $net 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
        $inspect = docker inspect $MinioContainer 2>$null | ConvertFrom-Json
        $onNet = $false
        if ($inspect -and $inspect[0].NetworkSettings.Networks.PSObject.Properties.Name -contains $net) {
            $onNet = $true
        }
        if (-not $onNet) {
            Write-HaStep "Attaching $MinioContainer to Docker network $net"
            docker network connect $net $MinioContainer 2>$null | Out-Null
        }
    }
}

Write-HaStep "Ensuring remote bucket $RemoteBucket exists on MinIO ..."
$mc = @(
    "run", "--rm", "--entrypoint", "sh", "minio/mc:latest", "-c",
    "mc alias set test $MinioEndpoint minioadmin minioadmin && mc mb test/$RemoteBucket --ignore-existing"
)
$env:HTTP_PROXY = ""; $env:HTTPS_PROXY = ""; $env:http_proxy = ""; $env:https_proxy = ""
docker @mc 2>$null | Out-Null

Write-HaStep "Ensuring local source bucket $SourceBucket on HA primary ..."
$loginBody = @{ username = $AdminUser; password = $AdminPass } | ConvertTo-Json -Compress
$login = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body $loginBody
if (-not $login.token) { throw "Login failed at $ConsoleUrl" }
$auth = @{ Authorization = "Bearer $($login.token)" }

try {
    Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/buckets/$SourceBucket" -Method POST -Headers $auth | Out-Null
    Write-HaStep "Created bucket $SourceBucket"
} catch {
    if ($_.Exception.Response.StatusCode.value__ -eq 409) {
        Write-HaStep "Bucket $SourceBucket already exists"
    } else {
        throw
    }
}

Write-HaStep "Configuring Gateway replication (local -> external S3) ..."
$gwScript = Join-Path $Root "scripts\setup-minio-gateway.ps1"
& powershell -NoProfile -ExecutionPolicy Bypass -File $gwScript `
    -BaseUrl $ConsoleUrl `
    -AdminUser $AdminUser `
    -AdminPass $AdminPass `
    -ConnectionName $ConnectionName `
    -MinioEndpoint $MinioEndpoint `
    -RemoteBucket $RemoteBucket `
    -SourceBucket $SourceBucket
if ($LASTEXITCODE -ne 0) { throw "setup-minio-gateway.ps1 failed" }

Write-HaStep "Registering Federation peers (HA standbys) ..."
$peers = @(
    @{
        name = "HA Standby 1 (read API)"
        endpoint = "http://storage-server-standby:9001"
        region = "us-east-1"
    },
    @{
        name = "HA Standby 2 (read API)"
        endpoint = "http://storage-server-standby-2:9003"
        region = "us-east-1"
    }
)

$existing = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/federation/clusters" -Headers $auth
$existingList = @()
if ($existing.clusters) { $existingList = @($existing.clusters) }

foreach ($p in $peers) {
    Register-FederationPeer -Peer $p -ExistingList $existingList -AuthHeaders $auth -BaseConsoleUrl $ConsoleUrl
}

if ($RegisterClusterB) {
    Write-HaStep "Registering remote DataSafe cluster B ..."
    Register-FederationPeer -Peer @{
        name = $ClusterBName
        endpoint = $ClusterBEndpoint
        region = "us-east-1"
    } -ExistingList $existingList -AuthHeaders $auth -BaseConsoleUrl $ConsoleUrl
    $existing = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/federation/clusters" -Headers $auth
    if ($existing.clusters) { $existingList = @($existing.clusters) }
}

$clusters = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/federation/clusters" -Headers $auth
foreach ($c in @($clusters.clusters)) {
    try {
        $test = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/federation/clusters/$($c.id)/test" -Method POST -Headers $auth
        Write-HaStep "Federation test [$($c.name)]: status=$($test.status) detail=$($test.detail)"
    } catch {
        Write-HaStep "Federation test [$($c.name)]: FAIL - $($_.Exception.Message)"
    }
}

$health = Invoke-RestMethod -Uri "$ConsoleUrl/api/v1/gateway/health" -Headers $auth
Write-Host ""
Write-HaStep "Integration summary:"
Write-Host "  Console Gateway:    $ConsoleUrl  (Administration / Gateway)"
Write-Host "  Console Federation: $ConsoleUrl  (Administration / Federation)"
Write-Host "  External S3 API:    http://127.0.0.1:$MinioHostPort  (minioadmin/minioadmin)"
Write-Host "  External S3 UI:     http://127.0.0.1:$MinioConsolePort"
Write-Host "  Replication rule:   $SourceBucket to $RemoteBucket via $ConnectionName"
Write-Host "  Gateway health:     replication_errors=$($health.replication_errors) queue_pending=$($health.queue_pending) rules_total=$($health.rules_total)"
if ($RegisterClusterB) {
    Write-Host "  Cluster B console:  http://127.0.0.1:9082  (admin/admin)"
    Write-Host "  Cluster B API:      http://127.0.0.1:9193"
}
Write-Host ""
Write-HaStep "Upload a file to bucket $SourceBucket, then verify MinIO bucket $RemoteBucket or Gateway Sync Jobs."
