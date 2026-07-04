# Re-upload all objects via Admin API so erasure backend rewrites shards (maintenance window).
param(
    [string]$BaseUrl = "http://127.0.0.1:8082",
    [switch]$DryRun
)
$ErrorActionPreference = "Stop"

for ($i = 0; $i -lt 30; $i++) {
    try {
        $hz = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 3
        if ($hz.status -eq "ok") { break }
    } catch {}
    Start-Sleep -Seconds 2
}

function Login-Admin {
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $r = Invoke-RestMethod -Uri "$BaseUrl/api/v1/admin/login" -Method POST -ContentType "application/json" -Body '{"username":"admin","password":"admin"}' -ErrorAction Stop
            if ($r.token) { return [string]$r.token }
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "login failed after retries"
}

$tok = Login-Admin
$h = @{ Authorization = "Bearer $tok" }
$list = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets" -Headers $h
$buckets = @($list.buckets | ForEach-Object { $_.name })
$total = 0
$migrated = 0

foreach ($bucket in $buckets) {
    if ($bucket -match '^\.' -or $bucket -eq '.datasafe-trash') { continue }
    $objs = Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$bucket/objects" -Headers $h
    foreach ($obj in @($objs.objects)) {
        if ($obj.is_delete_marker) { continue }
        $total++
        if ($DryRun) { continue }
        $dl = Invoke-WebRequest -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/$($obj.key)" -Headers $h -UseBasicParsing
        $ct = if ($dl.Headers["Content-Type"]) { $dl.Headers["Content-Type"] } else { "application/octet-stream" }
        Invoke-RestMethod -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/$($obj.key)" -Method PUT -Headers $h -ContentType $ct -Body $dl.Content | Out-Null
        $migrated++
        if ($migrated % 50 -eq 0) { Write-Host "  migrated $migrated / $total ..." }
    }
}

if ($DryRun) {
    Write-Host "Dry run: would re-upload $total objects via API (storage-server must use STORAGE_OBJECT_BACKEND=erasure)."
} else {
    Write-Host "Migrated $migrated objects across $($buckets.Count) buckets."
    Write-Host "Verify: GET /healthz object_backend=erasure; check datasafe_erasure_degraded_shard_sets=0"
}
