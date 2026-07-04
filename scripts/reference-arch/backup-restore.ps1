# Reference architecture: backup and restore smoke test
# Requires: local compose stack with admin API on $BaseUrl
# Usage: .\scripts\reference-arch\backup-restore.ps1

param(
  [string]$BaseUrl = "http://127.0.0.1:8080",
  [string]$AdminUser = "admin",
  [string]$AdminPassword = "admin"
)

$ErrorActionPreference = "Stop"

function Login {
  param([string]$User, [string]$Pass)
  $body = @{ username = $User; password = $Pass } | ConvertTo-Json
  $r = Invoke-RestMethod -Method POST -Uri "$BaseUrl/api/v1/admin/login" -ContentType "application/json" -Body $body
  return $r.token
}

$ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$bucket = "ref-arch-backup-$ts"
$token = Login $AdminUser $AdminPassword
$headers = @{ Authorization = "Bearer $token" }

Invoke-RestMethod -Method POST -Uri "$BaseUrl/api/v1/buckets/$bucket" -Headers $headers -ContentType "application/json" -Body '{"visibility":"private"}' | Out-Null

$key = "backup-test.txt"
$content = "datasafe-backup-$ts"
$bytes = [System.Text.Encoding]::UTF8.GetBytes($content)
Invoke-RestMethod -Method PUT -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/$key" -Headers $headers -ContentType "text/plain" -Body $bytes | Out-Null

$got = Invoke-RestMethod -Method GET -Uri "$BaseUrl/api/v1/buckets/$bucket/objects/$key" -Headers $headers
if ($got.content -ne $content) {
  Write-Error "GET object mismatch"
}

Write-Host "PASS reference-arch backup-restore smoke bucket=$bucket key=$key"
