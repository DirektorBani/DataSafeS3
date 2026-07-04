# HA cluster verification - run after scripts\ha\start-ha-stack.ps1
param(
    [string]$BaseUrl = "http://127.0.0.1:8082",
    [string]$PrimaryDirect = "http://127.0.0.1:9002",
    [string[]]$StandbyUrls = @("http://127.0.0.1:9001", "http://127.0.0.1:9003"),
    [string]$ComposeProject = "datasafe-ha",
    [string]$PgPrimaryPort = "5442",
    [string]$PgStandbyPort = "5443"
)
$ErrorActionPreference = "Stop"
$results = [System.Collections.Generic.List[object]]::new()

function Record($Step, $Status, $Notes) {
    $results.Add([PSCustomObject]@{ Step = $Step; Status = $Status; Notes = $Notes })
    $icon = if ($Status -eq "PASS") { "[PASS]" } elseif ($Status -eq "SKIP") { "[SKIP]" } else { "[FAIL]" }
    Write-Host "$icon $Step - $Notes"
}

function Login-Admin {
    $r = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}'
    if (-not $r.token) { throw "no token" }
    return $r.token
}

function Ensure-SetupComplete {
    $tok = Login-Admin
    $status = Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/status"
    if ($status.initial_setup_completed) { return }
    $h = @{ Authorization = "Bearer $tok" }
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h -ContentType "application/json" -Body '{"current_password":"admin","new_password":"Admin123!"}' | Out-Null
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/setup/complete" -Method POST -Headers $h | Out-Null
    $r2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"Admin123!"}'
    if ($r2.token) {
        $h2 = @{ Authorization = "Bearer $($r2.token)" }
        Invoke-RestMethod -Uri "$BaseUrl/api/v1/me/password" -Method POST -Headers $h2 -ContentType "application/json" -Body '{"current_password":"Admin123!","new_password":"admin"}' | Out-Null
    }
}

Ensure-SetupComplete | Out-Null

# 0. Topology: 3 storage + 2 postgres containers
try {
    $storageCnt = (docker ps --filter "name=${ComposeProject}-storage-server" --format "{{.Names}}" | Measure-Object).Count
    $pgCnt = (docker ps --filter "name=${ComposeProject}-postgres" --format "{{.Names}}" | Measure-Object).Count
    $status = if ($storageCnt -ge 3 -and $pgCnt -ge 2) { "PASS" } else { "FAIL" }
    Record "Topology 3 storage + 2 postgres" $status "storage=$storageCnt postgres=$pgCnt"
} catch {
    Record "Topology 3 storage + 2 postgres" "FAIL" $_.Exception.Message
}

# 1. Primary health + replication lag field
try {
    $h = Invoke-RestMethod -Uri "$PrimaryDirect/healthz"
    $status = if ($h.status -eq "ok" -and $h.read_only_mode -eq $false) { "PASS" } else { "FAIL" }
    Record "Primary /healthz + postgres" $status "read_only=$($h.read_only_mode) lag_s=$($h.replication_lag_s)"
} catch {
    Record "Primary /healthz + postgres" "FAIL" $_.Exception.Message
}

# 2. Standby read-only health (each node)
$idx = 1
foreach ($StandbyUrl in $StandbyUrls) {
    try {
        $h = Invoke-RestMethod -Uri "$StandbyUrl/healthz"
        $status = if ($h.status -eq "ok" -and $h.read_only_mode -eq $true) { "PASS" } else { "FAIL" }
        Record "Standby $idx /healthz read-only" $status "$StandbyUrl read_only=$($h.read_only_mode)"
    } catch {
        Record "Standby $idx /healthz read-only" "FAIL" $_.Exception.Message
    }
    $idx++
}

# 3. Postgres streaming replication
try {
    $sql = "SELECT count(*) FROM pg_stat_replication WHERE state='streaming';"
    $out = docker exec "${ComposeProject}-postgres-1" psql -U datasafe -d datasafe -tAc $sql 2>&1
    $cnt = [int]($out.ToString().Trim())
    $status = if ($cnt -ge 1) { "PASS" } else { "FAIL" }
    Record "Postgres pg_stat_replication" $status "streaming=$cnt"
} catch {
    Record "Postgres pg_stat_replication" "FAIL" $_.Exception.Message
}

# 4. Standby in recovery
try {
    $sql = 'SELECT pg_is_in_recovery();'
    $recovery = docker exec "${ComposeProject}-postgres-standby-1" psql -U datasafe -d datasafe -tAc $sql 2>&1
    $status = if ($recovery.ToString().Trim() -eq "t") { "PASS" } else { "FAIL" }
    Record "Postgres standby in recovery" $status $recovery
} catch {
    Record "Postgres standby in recovery" "FAIL" $_.Exception.Message
}

# 4b. Separate object volumes (not shared mount)
try {
    function Get-DataMount([string]$Container) {
        $json = docker inspect $Container --format '{{json .Mounts}}'
        foreach ($m in @($json | ConvertFrom-Json)) {
            if ($m.Destination -eq '/data') { return [string]$m.Source }
        }
        return $null
    }
    $primaryMount = Get-DataMount "${ComposeProject}-storage-server-1"
    $standby1Mount = Get-DataMount "${ComposeProject}-storage-server-standby-1"
    $standby2Mount = Get-DataMount "${ComposeProject}-storage-server-standby-2-1"
    $distinct = ($primaryMount -ne $standby1Mount) -and ($primaryMount -ne $standby2Mount) -and ($standby1Mount -ne $standby2Mount)
    $status = if ($distinct -and $primaryMount) { "PASS" } else { "FAIL" }
    Record "Separate storage object volumes" $status "primary=$primaryMount standby1=$standby1Mount"
} catch {
    Record "Separate storage object volumes" "FAIL" $_.Exception.Message
}

# 4c. Object replicator sidecar
try {
    $rep = docker ps --filter "name=${ComposeProject}-object-replicator" --format "{{.Status}}"
    $status = if ($rep -match "Up") { "PASS" } else { "FAIL" }
    Record "object-replicator running" $status $rep
} catch {
    Record "object-replicator running" "FAIL" $_.Exception.Message
}

# 5. Write on primary, object copy to standbys, read via API
$bucket = "ha-test-$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
$key = "probe.txt"
$payload = "ha-cluster-ok"
try {
    $tok = Login-Admin
    $h = @{ Authorization = "Bearer $tok" }
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$bucket" -Method POST -Headers $h -ContentType "application/json" -Body '{"visibility":"private"}' | Out-Null
    Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/$key" -Method PUT -Headers $h -ContentType "text/plain" -Body $payload | Out-Null

    $findOnPrimary = docker exec "${ComposeProject}-storage-server-1" sh -c "find /data/objects/buckets -path '*${bucket}*' -name '${key}' -print | head -1" 2>&1
    $objectRelPath = $findOnPrimary.ToString().Trim()
    $statusPrimary = if ($objectRelPath) { "PASS" } else { "FAIL" }
    Record "Object file on primary volume" $statusPrimary $objectRelPath

    $synced = $false
    if ($objectRelPath) {
        for ($i = 1; $i -le 20; $i++) {
            $s1 = "$(docker exec "${ComposeProject}-storage-server-standby-1" sh -c "test -f '$objectRelPath' && echo yes" 2>&1)".Trim()
            $s2 = "$(docker exec "${ComposeProject}-storage-server-standby-2-1" sh -c "test -f '$objectRelPath' && echo yes" 2>&1)".Trim()
            if ($s1 -eq "yes" -and $s2 -eq "yes") { $synced = $true; break }
            Start-Sleep -Seconds 2
        }
    }
    Record "Object replicated to standby volumes" $(if ($synced) { "PASS" } else { "FAIL" }) "max_wait_s=40"

    $idx = 1
    foreach ($StandbyUrl in $StandbyUrls) {
        $bodyOk = $false
        for ($j = 1; $j -le 15; $j++) {
            try {
                $dl = Invoke-WebRequest -Uri "$StandbyUrl/api/v1/buckets/$bucket/objects/$key" -Headers $h -UseBasicParsing
                if ($dl.Content -eq $payload) { $bodyOk = $true; break }
            } catch {}
            Start-Sleep -Seconds 2
        }
        $status = if ($bodyOk) { "PASS" } else { "FAIL" }
        Record "Read object via standby $idx" $status $(if ($bodyOk) { "ok" } else { "timeout" })
        $idx++
    }
} catch {
    Record "Write primary / replicate / read standbys" "FAIL" $_.Exception.Message
}

# 6. Each standby rejects write
$idx = 1
foreach ($StandbyUrl in $StandbyUrls) {
    try {
        $tok = Login-Admin
        $h = @{ Authorization = "Bearer $tok" }
        try {
            Invoke-WebRequest -Uri "$StandbyUrl/api/v1/buckets/$bucket/objects/deny-$idx.txt" -Method PUT -Headers $h -ContentType "text/plain" -Body "x" -UseBasicParsing | Out-Null
            Record "Standby $idx write blocked (503)" "FAIL" "expected 503"
        } catch {
            $code = $_.Exception.Response.StatusCode.value__
            $status = if ($code -eq 503) { "PASS" } else { "FAIL" }
            Record "Standby $idx write blocked (503)" $status "HTTP $code"
        }
    } catch {
        Record "Standby $idx write blocked (503)" "FAIL" $_.Exception.Message
    }
    $idx++
}

# 7. Read replica routing smoke (list buckets on primary with replica DSN configured)
try {
    $tok = Login-Admin
    $h = @{ Authorization = "Bearer $tok" }
    $list = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Headers $h
    $has = @($list.buckets | ForEach-Object { $_.name }) -contains $bucket
    $status = if ($has) { "PASS" } else { "FAIL" }
    Record "Primary list buckets (metadata)" $status "found_ha_bucket=$has"
} catch {
    Record "Primary list buckets (metadata)" "FAIL" $_.Exception.Message
}

# 8. Failover script dry-run
try {
    & "$PSScriptRoot\..\postgres-failover.ps1" -ComposeProject $ComposeProject -StandbyContainer "${ComposeProject}-postgres-standby-1" -DryRun | Out-Null
    Record "postgres-failover.ps1 dry-run" "PASS" "script ok"
} catch {
    Record "postgres-failover.ps1 dry-run" "FAIL" $_.Exception.Message
}

# 8b. Metadata failover helper dry-run
try {
    & "$PSScriptRoot\failover-metadata.ps1" -ComposeProject $ComposeProject -StandbyContainer "${ComposeProject}-postgres-standby-1" -DryRun | Out-Null
    Record "failover-metadata.ps1 dry-run" "PASS" "script ok"
} catch {
    Record "failover-metadata.ps1 dry-run" "FAIL" $_.Exception.Message
}

# 9. Healthz HA / object backend fields
try {
    $hz = Invoke-RestMethod -Uri "$PrimaryDirect/healthz"
    $ok = $hz.object_backend -ne $null
    Record "healthz object_backend field" $(if ($ok) { "PASS" } else { "FAIL" }) "backend=$($hz.object_backend)"
    if ($hz.ha_enabled) {
        Record "healthz is_leader field" $(if ($null -ne $hz.is_leader) { "PASS" } else { "FAIL" }) "is_leader=$($hz.is_leader)"
    } else {
        Record "healthz is_leader field" "SKIP" "STORAGE_HA_ENABLED not set"
    }
} catch {
    Record "healthz extended fields" "FAIL" $_.Exception.Message
}

# 10. FS to erasure migrator dry-run smoke
try {
    & "$PSScriptRoot\migrate-fs-to-erasure.ps1" -DryRun -BaseUrl $BaseUrl | Out-Null
    Record "migrate-fs-to-erasure.ps1 dry-run" "PASS" "ok"
} catch {
    Record "migrate-fs-to-erasure.ps1 dry-run" "FAIL" $_.Exception.Message
}

$fail = @($results | Where-Object { $_.Status -eq "FAIL" })
Write-Host ""
Write-Host "HA test summary: $($results.Count) steps, $($fail.Count) failed"
$results | Format-Table -AutoSize
if ($fail.Count -gt 0) { exit 1 }
exit 0
