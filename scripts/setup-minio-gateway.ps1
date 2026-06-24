param(
  [string]$BaseUrl = 'http://localhost:8080',
  [string]$AdminUser = 'admin',
  [string]$AdminPass = 'admin',
  [string]$ConnectionName = 'External S3 Test',
  [string]$MinioEndpoint = 'http://host.docker.internal:9100',
  [string]$RemoteBucket = 'replica-test',
  [string]$SourceBucket = ''
)

function Invoke-CurlJson {
  param([string]$Method, [string]$Url, [hashtable]$Headers = @{}, [string]$Body = $null)
  $tmp = $null
  $curlArgs = @('-s', '-X', $Method, $Url)
  foreach ($k in $Headers.Keys) { $curlArgs += @('-H', "${k}: $($Headers[$k])") }
  if ($null -ne $Body) {
    $tmp = [System.IO.Path]::GetTempFileName()
    [System.IO.File]::WriteAllText($tmp, $Body, [System.Text.UTF8Encoding]::new($false))
    $curlArgs += @('-H', 'Content-Type: application/json', '--data-binary', "@$tmp")
  }
  try {
    $text = (& curl.exe @curlArgs | Out-String).Trim()
  } finally {
    if ($tmp) { Remove-Item -Force -ErrorAction SilentlyContinue $tmp }
  }
  if (-not $text) { throw "Empty response from $Url" }
  try { return $text | ConvertFrom-Json } catch { throw "Invalid JSON from ${Url}: $text" }
}

$loginBody = "{`"username`":`"$AdminUser`",`"password`":`"$AdminPass`"}"
$login = Invoke-CurlJson -Method POST -Url "$BaseUrl/api/v1/admin/login" -Body $loginBody
if (-not $login.token) { throw "Login failed: $($login | ConvertTo-Json -Compress)" }
$auth = @{ Authorization = "Bearer $($login.token)" }

$list = Invoke-CurlJson -Method GET -Url "$BaseUrl/api/v1/gateway/connections" -Headers $auth
$conn = $null
if ($list.connections) {
  $conn = @($list.connections) | Where-Object { $_.name -eq $ConnectionName } | Select-Object -First 1
}
if (-not $conn) {
  $body = "{`"name`":`"$ConnectionName`",`"endpoint`":`"$MinioEndpoint`",`"region`":`"us-east-1`",`"access_key`":`"minioadmin`",`"secret_key`":`"minioadmin`",`"path_style`":true,`"tls_verify`":false}"
  $created = Invoke-CurlJson -Method POST -Url "$BaseUrl/api/v1/gateway/connections" -Headers $auth -Body $body
  $conn = $created.connection
  Write-Host "Created connection $($conn.id) ($ConnectionName)"
} else {
  Write-Host "Using existing connection $($conn.id) ($ConnectionName)"
}

$test = Invoke-CurlJson -Method POST -Url "$BaseUrl/api/v1/gateway/connections/$($conn.id)/test" -Headers $auth
if (-not $test.ok) { throw "Connection test failed: $($test | ConvertTo-Json -Compress)" }
Write-Host "Connection test: $($test.message)"

if (-not $SourceBucket) {
  $buckets = Invoke-CurlJson -Method GET -Url "$BaseUrl/api/v1/buckets" -Headers $auth
  if ($buckets.buckets -and @($buckets.buckets).Count -gt 0) {
    $SourceBucket = @($buckets.buckets)[0].name
    Write-Host "Auto-selected source bucket: $SourceBucket"
  } else {
    $SourceBucket = 'my-data'
    Write-Host "No local buckets found; default source bucket name: $SourceBucket"
  }
}

$rules = Invoke-CurlJson -Method GET -Url "$BaseUrl/api/v1/gateway/replication" -Headers $auth
$rule = $null
if ($rules.rules) {
  $rule = @($rules.rules) | Where-Object {
    $_.source_bucket -eq $SourceBucket -and $_.dest_connection_id -eq $conn.id -and $_.dest_bucket -eq $RemoteBucket
  } | Select-Object -First 1
}
if (-not $rule) {
  $ruleBody = "{`"source_bucket`":`"$SourceBucket`",`"dest_connection_id`":`"$($conn.id)`",`"dest_bucket`":`"$RemoteBucket`"}"
  $createdRule = Invoke-CurlJson -Method POST -Url "$BaseUrl/api/v1/gateway/replication" -Headers $auth -Body $ruleBody
  $rule = $createdRule.rule
  Write-Host "Created replication rule $($rule.id): $SourceBucket -> $RemoteBucket"
} else {
  Write-Host "Replication rule already exists: $($rule.id) ($SourceBucket -> $RemoteBucket)"
}

$health = Invoke-CurlJson -Method GET -Url "$BaseUrl/api/v1/gateway/health" -Headers $auth
Write-Host "Gateway health: replication_errors=$($health.replication_errors) queue_pending=$($health.queue_pending) rules_total=$($health.rules_total)"
Write-Host "Connection ID: $($conn.id)"
