# Reproducible list/upload benchmark (reference hardware)
param(
  [string]$Endpoint = "http://127.0.0.1:9000",
  [string]$Bucket = "bench-bucket-$(Get-Date -Format 'yyyyMMddHHmmss')",
  [int]$ObjectCount = 1000,
  [switch]$JsonOut
)
$ErrorActionPreference = "Stop"
$env:AWS_ACCESS_KEY_ID = "datasafe"
$env:AWS_SECRET_ACCESS_KEY = "datasafesecret"
$env:AWS_DEFAULT_REGION = "us-east-1"

function Invoke-AwsS3($args) {
  & aws --endpoint-url $Endpoint s3 @args 2>&1
}

Write-Host "Benchmark: endpoint=$Endpoint bucket=$Bucket objects=$ObjectCount"

Invoke-AwsS3 @("mb", "s3://$Bucket") | Out-Null

$uploadStart = Get-Date
for ($i = 0; $i -lt $ObjectCount; $i++) {
  $key = "obj-$i.dat"
  $tmp = [System.IO.Path]::GetTempFileName()
  Set-Content -Path $tmp -Value ("x" * 1024) -NoNewline
  Invoke-AwsS3 @("cp", $tmp, "s3://$Bucket/$key") | Out-Null
  Remove-Item $tmp -Force
}
$uploadMs = ((Get-Date) - $uploadStart).TotalMilliseconds

$listStart = Get-Date
for ($r = 0; $r -lt 5; $r++) {
  Invoke-AwsS3 @("ls", "s3://$Bucket", "--recursive") | Out-Null
}
$listMs = ((Get-Date) - $listStart).TotalMilliseconds / 5

$result = [ordered]@{
  timestamp_utc = (Get-Date).ToUniversalTime().ToString("o")
  endpoint      = $Endpoint
  bucket        = $Bucket
  object_count  = $ObjectCount
  upload_ms_total = [math]::Round($uploadMs, 2)
  list_ms_p50_approx = [math]::Round($listMs, 2)
  note          = "Reference bench; not a SLA claim"
}

if ($JsonOut) {
  $result | ConvertTo-Json -Depth 4
} else {
  Write-Host ($result | ConvertTo-Json -Depth 4)
}

Invoke-AwsS3 @("rb", "s3://$Bucket", "--force") | Out-Null
exit 0
