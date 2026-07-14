#Requires -Version 5.1
<#
.SYNOPSIS
  Compare object counts (and sample sizes) between MinIO-compatible source and DataSafeS3 dest.

.NOTES
  Secrets: -SourceSecret/-DestSecret or env SOURCE_SECRET/DEST_SECRET. Never logged.
  Live mode needs AWS CLI v2 on PATH.
#>
[CmdletBinding()]
param(
  [string]$SourceEndpoint = "",
  [string]$SourceKey = "",
  [string]$SourceSecret = "",
  [string]$DestEndpoint = "",
  [string]$DestKey = "",
  [string]$DestSecret = "",
  [Parameter(Mandatory = $true)][string]$Bucket,
  [int]$SampleSize = 20,
  [switch]$DryRun,
  [switch]$AllowCountDrift,
  [string]$Region = "us-east-1"
)

$ErrorActionPreference = "Stop"

function Write-Check([string]$Name, [string]$Status, [string]$Detail = "") {
  if ($Detail) {
    Write-Host ("[{0}] {1} - {2}" -f $Status, $Name, $Detail)
  }
  else {
    Write-Host ("[{0}] {1}" -f $Status, $Name)
  }
}

function Get-Secret([string]$ParamVal, [string]$EnvName) {
  if ($ParamVal) { return $ParamVal }
  $v = [Environment]::GetEnvironmentVariable($EnvName)
  if ($v) { return $v }
  return ""
}

$SourceSecret = Get-Secret $SourceSecret "SOURCE_SECRET"
$DestSecret = Get-Secret $DestSecret "DEST_SECRET"

Write-Host "=== minio-cutover-smoke ==="
Write-Host "Bucket: $Bucket"
Write-Host "SourceEndpoint: $SourceEndpoint"
Write-Host "DestEndpoint:   $DestEndpoint"
Write-Host "SampleSize: $SampleSize  DryRun: $DryRun  AllowCountDrift: $AllowCountDrift"
Write-Host ""

if ($DryRun) {
  Write-Check "Plan" "DRYRUN" "ListBuckets reachability (both endpoints)"
  Write-Check "Plan" "DRYRUN" "Bucket exists on source and dest: $Bucket"
  Write-Check "Plan" "DRYRUN" "Compare ListObjectsV2 object counts"
  Write-Check "Plan" "DRYRUN" "Sample up to $SampleSize keys for ContentLength match"
  Write-Check "Plan" "DRYRUN" "ETag compare skipped by default (multipart caveat)"
  Write-Host ""
  Write-Host "PASS dry-run (no AWS calls)"
  exit 0
}

if (-not $SourceEndpoint -or -not $DestEndpoint) {
  Write-Check "Args" "FAIL" "SourceEndpoint and DestEndpoint required unless -DryRun"
  exit 2
}
if (-not $SourceKey -or -not $SourceSecret -or -not $DestKey -or -not $DestSecret) {
  Write-Check "Args" "FAIL" "Keys/secrets required (params or SOURCE_SECRET/DEST_SECRET)"
  exit 2
}

$aws = Get-Command aws -ErrorAction SilentlyContinue
if (-not $aws) {
  Write-Check "AWS CLI" "FAIL" "aws not on PATH - install AWS CLI v2 or use -DryRun"
  exit 3
}
Write-Check "AWS CLI" "PASS" $aws.Source

function Invoke-S3Json {
  param(
    [string]$Endpoint,
    [string]$AccessKey,
    [string]$Secret,
    [string[]]$AwsArgs
  )
  $prevKey = $env:AWS_ACCESS_KEY_ID
  $prevSecret = $env:AWS_SECRET_ACCESS_KEY
  $prevRegion = $env:AWS_DEFAULT_REGION
  $env:AWS_ACCESS_KEY_ID = $AccessKey
  $env:AWS_SECRET_ACCESS_KEY = $Secret
  $env:AWS_DEFAULT_REGION = $Region
  try {
    $out = & aws --endpoint-url $Endpoint @AwsArgs 2>&1
    if ($LASTEXITCODE -ne 0) {
      throw "aws failed (exit $LASTEXITCODE): $out"
    }
    return ($out | Out-String)
  }
  finally {
    $env:AWS_ACCESS_KEY_ID = $prevKey
    $env:AWS_SECRET_ACCESS_KEY = $prevSecret
    $env:AWS_DEFAULT_REGION = $prevRegion
  }
}

function Test-ListBuckets([string]$Endpoint, [string]$AccessKey, [string]$Secret, [string]$Label) {
  try {
    $null = Invoke-S3Json -Endpoint $Endpoint -AccessKey $AccessKey -Secret $Secret -AwsArgs @("s3api", "list-buckets", "--output", "json")
    Write-Check "ListBuckets $Label" "PASS" $Endpoint
    return $true
  }
  catch {
    Write-Check "ListBuckets $Label" "FAIL" $_.Exception.Message
    return $false
  }
}

function Get-ObjectCount([string]$Endpoint, [string]$AccessKey, [string]$Secret) {
  $token = $null
  $count = 0
  $samples = New-Object System.Collections.Generic.List[object]
  do {
    $args = @("s3api", "list-objects-v2", "--bucket", $Bucket, "--output", "json", "--max-keys", "1000")
    if ($token) { $args += @("--continuation-token", $token) }
    $raw = Invoke-S3Json -Endpoint $Endpoint -AccessKey $AccessKey -Secret $Secret -AwsArgs $args
    $json = $raw | ConvertFrom-Json
    if ($json.Contents) {
      foreach ($o in @($json.Contents)) {
        $count++
        if ($samples.Count -lt $SampleSize) {
          $samples.Add([pscustomobject]@{ Key = $o.Key; Size = [int64]$o.Size })
        }
      }
    }
    if ($json.IsTruncated -eq $true -or $json.IsTruncated -eq "True") {
      $token = $json.NextContinuationToken
    }
    else {
      $token = $null
    }
  } while ($token)
  return @{ Count = $count; Samples = $samples }
}

$ok = $true
if (-not (Test-ListBuckets $SourceEndpoint $SourceKey $SourceSecret "source")) { $ok = $false }
if (-not (Test-ListBuckets $DestEndpoint $DestKey $DestSecret "dest")) { $ok = $false }
if (-not $ok) { exit 1 }

try {
  $null = Invoke-S3Json -Endpoint $SourceEndpoint -AccessKey $SourceKey -Secret $SourceSecret -AwsArgs @("s3api", "head-bucket", "--bucket", $Bucket)
  Write-Check "HeadBucket source" "PASS" $Bucket
}
catch {
  Write-Check "HeadBucket source" "FAIL" $_.Exception.Message
  exit 1
}
try {
  $null = Invoke-S3Json -Endpoint $DestEndpoint -AccessKey $DestKey -Secret $DestSecret -AwsArgs @("s3api", "head-bucket", "--bucket", $Bucket)
  Write-Check "HeadBucket dest" "PASS" $Bucket
}
catch {
  Write-Check "HeadBucket dest" "FAIL" $_.Exception.Message
  exit 1
}

$src = Get-ObjectCount $SourceEndpoint $SourceKey $SourceSecret
$dst = Get-ObjectCount $DestEndpoint $DestKey $DestSecret
Write-Check "Object count" "INFO" ("source={0} dest={1}" -f $src.Count, $dst.Count)

if ($src.Count -ne $dst.Count) {
  if ($AllowCountDrift) {
    Write-Check "Object count match" "WARN" "counts differ; -AllowCountDrift set"
  }
  else {
    Write-Check "Object count match" "FAIL" "counts differ"
    $ok = $false
  }
}
else {
  Write-Check "Object count match" "PASS" "equal"
}

$checked = 0
$mismatched = 0
foreach ($s in $src.Samples) {
  try {
    $raw = Invoke-S3Json -Endpoint $DestEndpoint -AccessKey $DestKey -Secret $DestSecret -AwsArgs @(
      "s3api", "head-object", "--bucket", $Bucket, "--key", $s.Key, "--output", "json"
    )
    $meta = $raw | ConvertFrom-Json
    $destSize = [int64]$meta.ContentLength
    $checked++
    if ($destSize -ne $s.Size) {
      $mismatched++
      Write-Check "Sample size" "FAIL" ("key={0} source={1} dest={2}" -f $s.Key, $s.Size, $destSize)
      $ok = $false
    }
  }
  catch {
    $mismatched++
    Write-Check "Sample head" "FAIL" ("key={0}: {1}" -f $s.Key, $_.Exception.Message)
    $ok = $false
  }
}
if ($checked -gt 0 -and $mismatched -eq 0) {
  Write-Check "Sample sizes" "PASS" ("checked={0}" -f $checked)
}
elseif ($checked -eq 0) {
  Write-Check "Sample sizes" "PASS" "no objects to sample"
}

Write-Host ""
if ($ok) {
  Write-Host "PASS minio-cutover-smoke"
  exit 0
}
Write-Host "FAIL minio-cutover-smoke"
exit 1
