# Trusted-cluster pairing + object replication smoke between HA (site A) and cluster B.
# Note: object replication requires peer endpoints reachable from inside the storage-server
# container (use host.docker.internal or docker network aliases, not 127.0.0.1 host ports).
param(
    [string]$PrimaryConsole = "http://127.0.0.1:8082",
    [string]$PeerConsole = "http://127.0.0.1:9082",
    [int]$WaitSeconds = 60,
    [switch]$PairingOnly
)
$ErrorActionPreference = "Stop"

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers = @{},
        [object]$BodyObj = $null
    )
    $params = @{
        Uri         = $Url
        Method      = $Method
        Headers     = $Headers
        ContentType = "application/json"
    }
    if ($null -ne $BodyObj) {
        $params.Body = ($BodyObj | ConvertTo-Json -Compress)
    }
    try {
        $resp = Invoke-WebRequest @params -UseBasicParsing
        $json = $null
        if ($resp.Content) {
            try { $json = $resp.Content | ConvertFrom-Json } catch {}
        }
        return [PSCustomObject]@{ Code = [int]$resp.StatusCode; Body = $resp.Content; Json = $json }
    } catch {
        $resp = $_.Exception.Response
        if ($null -eq $resp) { throw $_ }
        $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
        $body = $reader.ReadToEnd()
        $json = $null
        if ($body) { try { $json = $body | ConvertFrom-Json } catch {} }
        return [PSCustomObject]@{ Code = [int]$resp.StatusCode; Body = $body; Json = $json }
    }
}

function Login([string]$Base) {
    $r = Invoke-Api POST "$Base/api/v1/admin/login" -BodyObj @{ username = "admin"; password = "admin" }
    if ($r.Code -ne 200 -or -not $r.Json.token) { throw "login failed on $Base ($($r.Code)) $($r.Body)" }
    return $r.Json.token
}

Write-Host "=== Trusted cluster pairing E2E ==="
Write-Host "Primary: $PrimaryConsole  Peer: $PeerConsole"

$primaryTok = Login $PrimaryConsole
$peerTok = Login $PeerConsole
$primaryH = @{ Authorization = "Bearer $primaryTok" }
$peerH = @{ Authorization = "Bearer $peerTok" }

$primaryList = Invoke-Api GET "$PrimaryConsole/api/v1/clusters" -Headers $primaryH
$remote = @($primaryList.Json.clusters | Where-Object { -not $_.is_local -and $_.active -ne $false })[0]
if (-not $remote) {
    Write-Host '[pair] No remote cluster on primary - generating pairing code on primary'
    $code = Invoke-Api POST "$PrimaryConsole/api/v1/clusters/pairing-codes" -Headers $primaryH
    if ($code.Code -ne 201) { throw "pairing code create failed $($code.Code) $($code.Body)" }
    $primaryApi = ($PrimaryConsole -replace ':8082', ':9002')
    $join = Invoke-Api POST "$PeerConsole/api/v1/clusters/pair/join" -Headers $peerH -BodyObj @{
        initiator_url = $primaryApi
        token         = $code.Json.token
        name          = "cluster-b-lab"
    }
    if ($join.Code -ne 200) { throw "pair join failed $($join.Code) $($join.Body)" }
    Start-Sleep -Seconds 5
    $primaryList = Invoke-Api GET "$PrimaryConsole/api/v1/clusters" -Headers $primaryH
    $remote = @($primaryList.Json.clusters | Where-Object { -not $_.is_local -and $_.active -ne $false })[0]
}
if (-not $remote) { throw "remote trusted cluster still missing after pairing attempt" }
Write-Host ('[pair] Remote cluster id=' + $remote.id + ' name=' + $remote.name)
if ($PairingOnly) {
    Write-Host '=== PASS (pairing only) ==='
    exit 0
}

$endpoint = [string]$remote.endpoint
if ($endpoint -match '127\.0\.0\.1|localhost') {
    Write-Host "[repl] SKIP: peer endpoint $endpoint is not reachable from inside Docker containers"
    Write-Host '=== PASS (pairing verified; repl skipped - fix STORAGE_CLUSTER_ENDPOINT for container routing) ==='
    exit 0
}

$ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$srcBucket = "tc-src-$ts"
$dstBucket = "tc-dst-$ts"
Invoke-Api POST "$PrimaryConsole/api/v1/buckets/$srcBucket" -Headers $primaryH -BodyObj @{ visibility = "private" } | Out-Null
Invoke-Api POST "$PeerConsole/api/v1/buckets/$dstBucket" -Headers $peerH -BodyObj @{ visibility = "private" } | Out-Null

$rule = Invoke-Api POST "$PrimaryConsole/api/v1/clusters/$($remote.id)/replication-rules" -Headers $primaryH -BodyObj @{
    source_bucket = $srcBucket
    dest_bucket   = $dstBucket
    direction     = "one-way"
}
if ($rule.Code -notin 200, 201) { throw "create repl rule failed $($rule.Code) $($rule.Body)" }
$ruleId = $rule.Json.rule.id
Write-Host ('[repl] Rule ' + $ruleId + ' ' + $srcBucket + ' to ' + $dstBucket + ' on peer')

$key = "probe-$ts.txt"
$payload = "trusted-cluster-e2e-$ts"
$put = Invoke-Api PUT "$PrimaryConsole/api/v1/buckets/$srcBucket/objects/$key" -Headers $primaryH -BodyObj $payload
if ($put.Code -notin 200, 201) {
    # object upload uses raw body, not JSON
    $tmp = Join-Path $env:TEMP "tc-repl-$ts.txt"
    Set-Content -Path $tmp -Value $payload -NoNewline
    $curlArgs = @(
        "-s", "-w", "`n%{http_code}", "-X", "PUT",
        "$PrimaryConsole/api/v1/buckets/$srcBucket/objects/$key",
        "-H", "Authorization: Bearer $primaryTok",
        "-H", "Content-Type: text/plain",
        "--data-binary", "@$tmp"
    )
    $out = curl.exe @curlArgs
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    $lines = $out -split "`n"
    $code = [int]$lines[-1]
    if ($code -notin 200, 201) { throw "object upload failed HTTP $code" }
}

$found = $false
for ($i = 0; $i -lt $WaitSeconds; $i++) {
    Start-Sleep -Seconds 1
    $obj = Invoke-Api GET "$PeerConsole/api/v1/buckets/$dstBucket/objects/$key" -Headers $peerH
    if ($obj.Code -eq 200) { $found = $true; break }
}
if ($ruleId) {
    Invoke-Api DELETE "$PrimaryConsole/api/v1/clusters/$($remote.id)/replication-rules/$ruleId" -Headers $primaryH | Out-Null
}
if (-not $found) { throw "object not replicated to peer within ${WaitSeconds}s" }
Write-Host '[repl] Object replicated OK'
Write-Host '=== PASS ==='
