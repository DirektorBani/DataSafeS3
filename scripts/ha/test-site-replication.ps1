# Site replication E2E — two DataSafeS3 stacks (site A -> site B).
param(
    [string]$SourceUrl = "http://127.0.0.1:8082",
    [string]$PeerConsoleUrl = "http://127.0.0.1:9082",
    [string]$PeerS3Endpoint = "http://host.docker.internal:9193",
    [string]$AccessKey = "datasafe",
    [string]$SecretKey = "datasafesecret",
    [int]$MaxWaitSec = 90
)
$ErrorActionPreference = "Stop"
$results = [System.Collections.Generic.List[object]]::new()

function Record($Step, $Status, $Notes) {
    $results.Add([PSCustomObject]@{ Step = $Step; Status = $Status; Notes = $Notes })
    $icon = if ($Status -eq "PASS") { "[PASS]" } elseif ($Status -eq "SKIP") { "[SKIP]" } else { "[FAIL]" }
    Write-Host "$icon $Step - $Notes"
}

function Login-Admin([string]$BaseUrl) {
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
            if ($r.token) { return $r.token }
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "no token from $BaseUrl"
}

function Ensure-Setup([string]$BaseUrl) {
    $tok = $null
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
            if ($r.token) { $tok = $r.token; break }
        } catch {}
        Start-Sleep -Seconds 2
    }
    if (-not $tok) { throw "no token from $BaseUrl" }
    $status = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status"
    if ($status.initial_setup_completed) { return $tok }
    $h = @{ Authorization = "Bearer $tok" }
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h -ContentType "application/json" -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/complete" -Method POST -Headers $h | Out-Null
    $r2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"Admin123!"}'
    if ($r2.token) {
        $h2 = @{ Authorization = "Bearer $($r2.token)" }
        Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h2 -ContentType "application/json" -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
        return (Login-Admin $BaseUrl)
    }
    return $tok
}

try {
    $srcTok = Ensure-Setup $SourceUrl
    $peerTok = Ensure-Setup $PeerConsoleUrl
    $srcH = @{ Authorization = "Bearer $srcTok" }
    $peerH = @{ Authorization = "Bearer $peerTok" }

    # 1. Register peer on source
    $peerBody = @{
        name = "site-b-lab"
        endpoint = $PeerS3Endpoint.TrimEnd("/")
        access_key = $AccessKey
        secret_key = $SecretKey
        enabled = $true
    } | ConvertTo-Json
    $peerResp = Invoke-RestMethod -Uri "$SourceUrl/api/v1/site-replication/peers" -Method POST -Headers $srcH -ContentType "application/json" -Body $peerBody
    $peerId = $peerResp.peer.id
    Record "Register site replication peer" $(if ($peerId) { "PASS" } else { "FAIL" }) "id=$peerId"

    # 2. Create rule + dest bucket on peer
    $ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $srcBucket = "site-repl-src-$ts"
    $destBucket = "site-repl-dst-$ts"
    Invoke-RestMethod -Uri "$SourceUrl/api/v1/buckets/$srcBucket" -Method POST -Headers $srcH -ContentType "application/json" -Body '{"visibility":"private"}' | Out-Null
    Invoke-RestMethod -Uri "$PeerConsoleUrl/api/v1/buckets/$destBucket" -Method POST -Headers $peerH -ContentType "application/json" -Body '{"visibility":"private"}' | Out-Null

    $ruleBody = @{
        peer_id = $peerId
        source_bucket = $srcBucket
        dest_bucket = $destBucket
        direction = "one-way"
        enabled = $true
    } | ConvertTo-Json
    $ruleResp = Invoke-RestMethod -Uri "$SourceUrl/api/v1/site-replication/rules" -Method POST -Headers $srcH -ContentType "application/json" -Body $ruleBody
    $ruleId = $ruleResp.rule.id
    Record "Create site replication rule" $(if ($ruleId) { "PASS" } else { "FAIL" }) "$srcBucket -> $destBucket"

    # 3. PUT object on source
    $key = "repl-probe.txt"
    $payload = "site-repl-ok-$ts"
    Invoke-RestMethod -Uri "$SourceUrl/api/v1/buckets/$srcBucket/objects/$key" -Method PUT -Headers $srcH -ContentType "text/plain" -Body $payload | Out-Null
    Record "PUT object on source" "PASS" $key

    # 4. Wait for queue drain
    $drained = $false
    for ($i = 0; $i -lt $MaxWaitSec; $i += 3) {
        Start-Sleep -Seconds 3
        $st = Invoke-RestMethod -Uri "$SourceUrl/api/v1/site-replication/status" -Headers $srcH
        if ($st.pending_count -eq 0) { $drained = $true; break }
    }
    Record "Replication queue drained" $(if ($drained) { "PASS" } else { "FAIL" }) "pending=$($st.pending_count) lag_s=$($st.lag_seconds)"

    # 5. Object visible on peer
    $bodyOk = $false
    for ($j = 1; $j -le 10; $j++) {
        try {
            $dl = Invoke-WebRequest -Uri "$PeerConsoleUrl/api/v1/buckets/$destBucket/objects/$key" -Headers $peerH -UseBasicParsing
            $got = if ($dl.Content -is [byte[]]) {
                [System.Text.Encoding]::UTF8.GetString($dl.Content)
            } else {
                [string]$dl.Content
            }
            if ($got -eq $payload) { $bodyOk = $true; break }
        } catch {}
        Start-Sleep -Seconds 2
    }
    Record "Object on peer site" $(if ($bodyOk) { "PASS" } else { "FAIL" }) "dest=$destBucket"

    # Cleanup peer (optional)
    if ($peerId) {
        Invoke-RestMethod -Uri "$SourceUrl/api/v1/site-replication/peers/$peerId" -Method DELETE -Headers $srcH | Out-Null
    }
} catch {
    Record "Site replication E2E" "FAIL" $_.Exception.Message
}

$fail = @($results | Where-Object { $_.Status -eq "FAIL" })
Write-Host ""
Write-Host "Site replication summary: $($results.Count) steps, $($fail.Count) failed"
$results | Format-Table -AutoSize
if ($fail.Count -gt 0) { exit 1 }
exit 0
