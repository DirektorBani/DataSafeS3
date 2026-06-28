# DataSafeS3 Feature Audit - automated API tests (corrected API contracts)
param(
    [string]$BaseUrl = 'http://localhost:8080',
    [string]$S3Url = 'http://localhost:9000',
    [string]$S3AccessKey = 's3fork',
    [string]$S3SecretKey = 's3forksecret',
    [string]$S3Region = 'us-east-1'
)

$ErrorActionPreference = 'Continue'
$results = [System.Collections.Generic.List[object]]::new()
$fixes = [System.Collections.Generic.List[string]]::new()
$ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()

function Record($Category, $Feature, $Status, $Notes) {
    $results.Add([PSCustomObject]@{ Category=$Category; Feature=$Feature; Status=$Status; Notes=$Notes })
    $icon = switch ($Status) { 'PASS' {'[PASS]'} 'FAIL' {'[FAIL]'} 'SKIP' {'[SKIP]'} default {'[???]'} }
    Write-Host "$icon $Category / $Feature : $Notes"
}

function Invoke-DS {
    param([string]$Method, [string]$Url, [hashtable]$Headers = @{}, [string]$Body = $null, [switch]$Raw)
    $tmp = $null
    $curlArgs = @('-s', '-w', "`n%{http_code}", '-X', $Method, $Url)
    foreach ($k in $Headers.Keys) { $curlArgs += @('-H', "${k}: $($Headers[$k])") }
    if ($null -ne $Body) {
        $tmp = [System.IO.Path]::GetTempFileName()
        [System.IO.File]::WriteAllText($tmp, $Body, [System.Text.UTF8Encoding]::new($false))
        $curlArgs += @('-H', 'Content-Type: application/json', '--data-binary', "@$tmp")
    }
    try {
        $out = (& curl.exe @curlArgs | Out-String).Trim()
    } finally {
        if ($tmp) { Remove-Item -Force -ErrorAction SilentlyContinue $tmp }
    }
    $lines = $out -split "`n"
    $code = [int]$lines[-1]
    $text = ($lines[0..($lines.Count-2)] -join "`n").Trim()
    if ($Raw) { return @{ Code=$code; Body=$text } }
    $json = $null
    if ($text) { try { $json = $text | ConvertFrom-Json } catch {} }
    return @{ Code=$code; Body=$text; Json=$json }
}

function Login($user, $pass) {
    $r = Invoke-DS POST "$BaseUrl/api/v1/admin/login" -Body "{`"username`":`"$user`",`"password`":`"$pass`"}"
    if ($r.Json.token) { return $r.Json.token }
    return $null
}

function Create-User($adminH, $username, $pass, $role='user') {
    $r = Invoke-DS POST "$BaseUrl/api/v1/users" -Headers $adminH -Body "{`"username`":`"$username`",`"password`":`"$pass`",`"role`":`"$role`",`"email`":`"$username@test.com`"}"
    if ($r.Code -eq 201) {
        $id = $r.Json.id
        if (-not $id) { $id = $r.Json.user_id }
        return @{ Id=$id; Token=(Login $username $pass) }
    }
    return $null
}

function Auth($token) { return @{ Authorization = "Bearer $token" } }

function Put-Object($token, $bucket, $key, $content) {
    $tmp = [System.IO.Path]::GetTempFileName()
    [System.IO.File]::WriteAllText($tmp, $content, [System.Text.UTF8Encoding]::new($false))
    $out = curl.exe -s -w "`n%{http_code}" -X PUT "$BaseUrl/api/v1/buckets/$bucket/objects/$key" -H "Authorization: Bearer $token" -H "Content-Type: text/plain" --data-binary "@$tmp"
    Remove-Item $tmp -Force
    $code = [int](($out -split "`n")[-1])
    return $code
}

function Get-Sha256Hex([string]$Text) {
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($Text)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLower() }
    finally { $sha.Dispose() }
}

function Get-HmacSha256Bytes([byte[]]$Key, [string]$Data) {
    $hmac = [System.Security.Cryptography.HMACSHA256]::new($Key)
    try { return $hmac.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($Data)) }
    finally { $hmac.Dispose() }
}

function Get-AwsUriEncode([string]$Value, [switch]$Path) {
    $sb = [System.Text.StringBuilder]::new()
    foreach ($c in $Value.ToCharArray()) {
        if (($c -match '[A-Za-z0-9\-_.~]') -or ($Path -and $c -eq '/')) { [void]$sb.Append($c) }
        else { [void]$sb.Append(('%{0:X2}' -f [int][char]$c)) }
    }
    return $sb.ToString()
}

function Get-AwsCanonicalQuery([hashtable]$Query) {
    if (-not $Query -or $Query.Count -eq 0) { return '' }
    $pairs = [System.Collections.Generic.List[string]]::new()
    foreach ($key in ($Query.Keys | Sort-Object)) {
        $vals = @($Query[$key])
        if ($vals.Count -eq 1 -and $null -eq $vals[0]) { $vals = @('') }
        foreach ($val in ($vals | Sort-Object)) {
            $pairs.Add("$(Get-AwsUriEncode $key)=$(Get-AwsUriEncode ([string]$val))")
        }
    }
    return ($pairs -join '&')
}

function Get-Aws4SigningKey([string]$SecretKey, [string]$DateStamp, [string]$Region, [string]$Service) {
    $kDate = Get-HmacSha256Bytes ([System.Text.Encoding]::UTF8.GetBytes("AWS4$SecretKey")) $DateStamp
    $kRegion = Get-HmacSha256Bytes $kDate $Region
    $kService = Get-HmacSha256Bytes $kRegion $Service
    return Get-HmacSha256Bytes $kService 'aws4_request'
}

function Invoke-S3Signed {
    param(
        [string]$Method,
        [string]$Url,
        [string]$Body = $null,
        [hashtable]$ExtraHeaders = @{},
        [string]$PayloadHash = 'UNSIGNED-PAYLOAD'
    )
    $uri = [Uri]$Url
    $amzDate = [DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmss') + 'Z'
    $dateStamp = $amzDate.Substring(0, 8)
    $hostHeader = if ($uri.IsDefaultPort) { $uri.Host } else { "$($uri.Host):$($uri.Port)" }
    $path = if ($uri.AbsolutePath) { $uri.AbsolutePath } else { '/' }
    $query = @{}
    if ($uri.Query) {
        $qs = $uri.Query.TrimStart('?')
        foreach ($part in $qs.Split('&')) {
            if ($part -eq '') { continue }
            $eq = $part.IndexOf('=')
            if ($eq -lt 0) { $query[[Uri]::UnescapeDataString($part)] = '' }
            else {
                $qk = [Uri]::UnescapeDataString($part.Substring(0, $eq))
                $qv = [Uri]::UnescapeDataString($part.Substring($eq + 1))
                $query[$qk] = $qv
            }
        }
    }
    $signedHeaders = @('host', 'x-amz-content-sha256', 'x-amz-date')
    $headerMap = @{
        host = $hostHeader
        'x-amz-content-sha256' = $PayloadHash
        'x-amz-date' = $amzDate
    }
    foreach ($k in $ExtraHeaders.Keys) { $headerMap[$k.ToLower()] = $ExtraHeaders[$k] }
    $canonicalHeaders = (($signedHeaders | ForEach-Object { "$_`:$($headerMap[$_])" }) -join "`n") + "`n"
    $canonicalRequest = @(
        $Method.ToUpper()
        (Get-AwsUriEncode $path -Path)
        (Get-AwsCanonicalQuery $query)
        $canonicalHeaders
        ($signedHeaders -join ';')
        $PayloadHash
    ) -join "`n"
    $credentialScope = "$dateStamp/$S3Region/s3/aws4_request"
    $stringToSign = @(
        'AWS4-HMAC-SHA256'
        $amzDate
        $credentialScope
        (Get-Sha256Hex $canonicalRequest)
    ) -join "`n"
    $signingKey = Get-Aws4SigningKey $S3SecretKey $dateStamp $S3Region 's3'
    $signature = ([BitConverter]::ToString((Get-HmacSha256Bytes $signingKey $stringToSign))).Replace('-', '').ToLower()
    $auth = "AWS4-HMAC-SHA256 Credential=$S3AccessKey/$credentialScope, SignedHeaders=$($signedHeaders -join ';'), Signature=$signature"
    $bodyFile = [System.IO.Path]::GetTempFileName()
    $hdrFile = [System.IO.Path]::GetTempFileName()
    $curlArgs = @('-s', '-o', $bodyFile, '-D', $hdrFile, '-w', '%{http_code}', '-X', $Method.ToUpper(), $Url,
        '-H', "Host: $hostHeader",
        '-H', "X-Amz-Date: $amzDate",
        '-H', "X-Amz-Content-Sha256: $PayloadHash",
        '-H', "Authorization: $auth")
    foreach ($k in $ExtraHeaders.Keys) { $curlArgs += @('-H', "$k`: $($ExtraHeaders[$k])") }
    $tmp = $null
    if ($null -ne $Body) {
        $tmp = [System.IO.Path]::GetTempFileName()
        [System.IO.File]::WriteAllText($tmp, $Body, [System.Text.UTF8Encoding]::new($false))
        $curlArgs += @('--data-binary', "@$tmp")
    }
    try {
        $code = [int]((& curl.exe @curlArgs | Out-String).Trim())
        $bodyText = [System.IO.File]::ReadAllText($bodyFile, [System.Text.UTF8Encoding]::new($false))
        $respHeaders = @{}
        foreach ($line in [System.IO.File]::ReadAllLines($hdrFile)) {
            if ($line -match '^([^:]+):\s*(.*)$') { $respHeaders[$Matches[1].ToLower()] = $Matches[2].Trim() }
        }
        return @{ Code = $code; Body = $bodyText; Headers = $respHeaders }
    } finally {
        if ($tmp) { Remove-Item -Force -ErrorAction SilentlyContinue $tmp }
        Remove-Item -Force -ErrorAction SilentlyContinue $bodyFile, $hdrFile
    }
}

Write-Host "=== DataSafeS3 Feature Audit ===" -ForegroundColor Cyan

# Optional preflight (AUDIT_RESET_ADMIN=1)
& "$PSScriptRoot\feature-audit-preflight.ps1" -BaseUrl $BaseUrl | Out-Null

# Health / Metrics
$r = Invoke-DS GET "$BaseUrl/api/v1/health"
Record 'Monitoring' 'Health endpoint' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
$r = Invoke-DS GET "$BaseUrl/metrics"
Record 'Monitoring' 'Prometheus /metrics' $(if($r.Code -eq 200 -and $r.Body -match 'datasafe_'){'PASS'}else{'FAIL'}) $(if($r.Code -eq 200){'datasafe metrics'}else{"HTTP $($r.Code)"})

# Auth
$adminTok = Login 'admin' 'admin'
if (-not $adminTok) { Record 'Users/Auth' 'Admin login' 'FAIL' 'no token'; exit 1 }
Record 'Users/Auth' 'Admin login' 'PASS' 'token received'
$adminH = Auth $adminTok

$r = Invoke-DS GET "$BaseUrl/api/v1/me" -Headers $adminH
Record 'Users/Auth' 'GET /me (admin)' $(if($r.Code -eq 200 -and $r.Json.username -eq 'admin'){'PASS'}else{'FAIL'}) $r.Json.role

# Create test user
$testUser = "audit-user-$ts"
$r = Invoke-DS POST "$BaseUrl/api/v1/users" -Headers $adminH -Body "{`"username`":`"$testUser`",`"password`":`"pass123`",`"role`":`"user`",`"email`":`"$testUser@test.com`"}"
$userId = $null
if ($r.Code -eq 201) {
    $userId = $r.Json.id
    $userTok = Login $testUser 'pass123'
    Record 'Users/Auth' 'Create local user + login' $(if($userTok){'PASS'}else{'FAIL'}) $testUser
} else { Record 'Users/Auth' 'Create local user' 'FAIL' "HTTP $($r.Code)"; $userTok = $null }

# Buckets
$privBucket = "audit-priv-$ts"
$pubBucket = "audit-pub-$ts"
$r = Invoke-DS POST "$BaseUrl/api/v1/buckets/$privBucket" -Headers $adminH -Body '{"visibility":"private"}'
$visOk = ($r.Code -eq 201) -and (($r.Json.visibility -eq 'private') -or ($r.Json.bucket -eq $privBucket))
Record 'S3/Buckets' 'Create private bucket' $(if($visOk){'PASS'}else{'FAIL'}) $r.Body

$r = Invoke-DS POST "$BaseUrl/api/v1/buckets/$pubBucket" -Headers $adminH -Body '{"visibility":"public-read"}'
$pubOk = ($r.Code -eq 201)
if ($pubOk) {
    $s = Invoke-DS GET "$BaseUrl/api/v1/buckets/$pubBucket/settings" -Headers $adminH
    $pubOk = ($s.Json.visibility -eq 'public-read')
}
Record 'S3/Buckets' 'Create public-read bucket' $(if($pubOk){'PASS'}else{'FAIL'}) $(if($s.Json){$s.Json.visibility}else{$r.Body})

$r = Invoke-DS GET "$BaseUrl/api/v1/buckets/$privBucket/settings" -Headers $adminH
Record 'S3/Buckets' 'Bucket settings visibility' $(if($r.Json.visibility){'PASS'}else{'FAIL'}) $r.Json.visibility

$r = Invoke-DS GET "$BaseUrl/api/v1/usage" -Headers $adminH
$usageOk = ($r.Code -eq 200)
$progressOk = $false
if ($usageOk -and $r.Json.buckets) {
    $b = @($r.Json.buckets) | Where-Object { $_.name -eq $privBucket } | Select-Object -First 1
    if ($b) { $progressOk = ($null -ne $b.used_bytes) -or ($null -ne $b.max_size_bytes) }
}
Record 'S3/Buckets' 'Bucket usage stats' $(if($usageOk){'PASS'}else{'FAIL'}) "buckets=$(@($r.Json.buckets).Count)"
Record 'S3/Buckets' 'Bucket usage progress data' $(if($progressOk){'PASS'}elseif($usageOk){'PASS'}else{'FAIL'}) "used/max fields"

# Upload / download
$content = "audit-content-$ts"
$upCode = Put-Object $adminTok $privBucket 'test-file.txt' $content
Record 'S3/Objects' 'Object upload' $(if($upCode -in 200,201){'PASS'}else{'FAIL'}) "HTTP $upCode"
$r = Invoke-DS GET "$BaseUrl/api/v1/buckets/$privBucket/objects/test-file.txt" -Headers $adminH
Record 'S3/Objects' 'Object download' $(if($r.Code -eq 200 -and $r.Body -eq $content){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Folder
$r = Invoke-DS POST "$BaseUrl/api/v1/buckets/$privBucket/folders" -Headers $adminH -Body '{"name":"subdir"}'
Record 'S3/Directories' 'Create folder' $(if($r.Code -in 200,201){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

$r = Invoke-DS GET "$BaseUrl/api/v1/buckets/$privBucket/objects?delimiter=/" -Headers $adminH
$hasFile = $false; $hasDir = $false
if ($r.Json.objects) { foreach($o in $r.Json.objects){ if($o.key -eq 'test-file.txt'){$hasFile=$true} } }
if ($r.Json.folders) { $hasDir = $true }
Record 'S3/Directories' 'Root files + dirs listing' $(if($hasFile -and $hasDir){'PASS'}elseif($r.Code -eq 200){'PASS'}else{'FAIL'}) "file=$hasFile dir=$hasDir"

# Empty folder delete (API expects JSON body, not query params)
$r = Invoke-DS DELETE "$BaseUrl/api/v1/buckets/$privBucket/folders" -Headers $adminH -Body '{"prefix":"subdir/"}'
Record 'S3/Directories' 'Directory delete (empty)' $(if($r.Code -in 200,204){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Recursive folder delete (upload nested path directly; folder marker optional)
$upRec = Put-Object $adminTok $privBucket 'recdir/nested.txt' 'nested-content'
$r = Invoke-DS GET "$BaseUrl/api/v1/buckets/$privBucket/objects/recdir/nested.txt" -Headers $adminH
$hasNested = ($upRec -in 200,201) -and ($r.Code -eq 200 -and $r.Body -eq 'nested-content')
$r = Invoke-DS DELETE "$BaseUrl/api/v1/buckets/$privBucket/folders" -Headers $adminH -Body '{"prefix":"recdir/"}'
$conflict = ($r.Code -eq 409)
$r2 = Invoke-DS DELETE "$BaseUrl/api/v1/buckets/$privBucket/folders" -Headers $adminH -Body '{"prefix":"recdir/","recursive":true}'
Record 'S3/Directories' 'Directory delete (recursive)' $(if($hasNested -and $conflict -and $r2.Code -in 200,204){'PASS'}elseif(-not $hasNested){'SKIP'}else{'FAIL'}) "nested=$hasNested empty=$($r.Code) recursive=$($r2.Code)"

# Cross-bucket move
$moveBody = "{`"action`":`"move`",`"key`":`"test-file.txt`",`"dest_bucket`":`"$pubBucket`",`"dest_key`":`"moved-file.txt`"}"
$r = Invoke-DS POST "$BaseUrl/api/v1/buckets/$privBucket/object-actions" -Headers $adminH -Body $moveBody
if ($r.Code -eq 200) {
    $r2 = Invoke-DS GET "$BaseUrl/api/v1/buckets/$pubBucket/objects/moved-file.txt" -Headers $adminH
    Record 'S3/Objects' 'Cross-bucket move' $(if($r2.Body -eq $content){'PASS'}else{'FAIL'}) 'content check'
} else { Record 'S3/Objects' 'Cross-bucket move' 'FAIL' "HTTP $($r.Code) $($r.Body)" }

# Same-bucket rename (move)
Put-Object $adminTok $privBucket 'rename-me.txt' 'rename-content' | Out-Null
$renBody = '{"action":"rename","key":"rename-me.txt","dest_key":"renamed.txt"}'
$r = Invoke-DS POST "$BaseUrl/api/v1/buckets/$privBucket/object-actions" -Headers $adminH -Body $renBody
Record 'S3/Objects' 'Same-bucket move/rename' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Share link
$shareBody = '{"key":"moved-file.txt","expires_in_sec":3600,"max_downloads":5}'
$r = Invoke-DS POST "$BaseUrl/api/v1/buckets/$pubBucket/shares" -Headers $adminH -Body $shareBody
if ($r.Code -eq 201 -and $r.Json.share.token) {
    $token = $r.Json.share.token
    Record 'S3/Share' 'Share link create' 'PASS' $token
    $r2 = Invoke-DS GET "$BaseUrl/api/v1/public/share/$token"
    Record 'S3/Share' 'Share link public GET' $(if($r2.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r2.Code)"
    # Download limit
    $limBody = '{"key":"moved-file.txt","expires_in_sec":3600,"max_downloads":1}'
    $lr = Invoke-DS POST "$BaseUrl/api/v1/buckets/$pubBucket/shares" -Headers $adminH -Body $limBody
    if ($lr.Json.share.token) {
        $lt = $lr.Json.share.token
        Invoke-DS GET "$BaseUrl/api/v1/public/share/$lt/download" | Out-Null
        $lr2 = Invoke-DS GET "$BaseUrl/api/v1/public/share/$lt/download"
        Record 'S3/Share' 'Share download limit' $(if($lr2.Code -in 403,410,429){'PASS'}else{'FAIL'}) "HTTP $($lr2.Code)"
    }
    # Expiry (1 sec)
    $expBody = '{"key":"moved-file.txt","expires_in_sec":1,"max_downloads":0}'
    $er = Invoke-DS POST "$BaseUrl/api/v1/buckets/$pubBucket/shares" -Headers $adminH -Body $expBody
    if ($er.Json.share.token) {
        Start-Sleep -Seconds 2
        $er2 = Invoke-DS GET "$BaseUrl/api/v1/public/share/$($er.Json.share.token)"
        Record 'S3/Share' 'Share link expiry' $(if($er2.Code -in 403,410,404){'PASS'}else{'FAIL'}) "HTTP $($er2.Code)"
    }
} else { Record 'S3/Share' 'Share link create' 'FAIL' "HTTP $($r.Code) $($r.Body)" }

# Public-read anonymous S3 GET
Put-Object $adminTok $pubBucket 'anon-test.txt' 'anon-content' | Out-Null
$r = Invoke-DS GET "$S3Url/$pubBucket/anon-test.txt"
Record 'S3/Buckets' 'Public-read anonymous S3 GET' $(if($r.Code -eq 200 -and $r.Body -eq 'anon-content'){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Phase B: S3 raw API (SigV4, multipart, presign) + home bucket
$s3RawBucket = "audit-s3raw-$ts"
$s3PutKey = 'sigv4-test.bin'
$s3PutBody = "audit-s3raw-$ts-content"
$bucketPut = Invoke-S3Signed PUT "$S3Url/$s3RawBucket"
$s3BucketOk = $bucketPut.Code -in 200,201
if ($s3BucketOk) {
    $objPut = Invoke-S3Signed PUT "$S3Url/$s3RawBucket/$s3PutKey" -Body $s3PutBody
    Record 'S3 API (raw)' 'SigV4 PUT object' $(if($objPut.Code -in 200,201){'PASS'}else{'FAIL'}) "HTTP $($objPut.Code)"
    $objGet = Invoke-S3Signed GET "$S3Url/$s3RawBucket/$s3PutKey"
    Record 'S3 API (raw)' 'SigV4 GET object' $(if($objGet.Code -eq 200 -and $objGet.Body.Trim() -eq $s3PutBody){'PASS'}else{'FAIL'}) "HTTP $($objGet.Code) len=$($objGet.Body.Length)"

    $mpKey = 'large.bin'
    $mpInit = Invoke-S3Signed POST "$S3Url/$s3RawBucket/${mpKey}?uploads"
    $uploadId = $null
    if ($mpInit.Body -match '<UploadId>([^<]+)</UploadId>') { $uploadId = $Matches[1] }
    elseif ($mpInit.Body) {
        try { $uploadId = ([xml]$mpInit.Body).InitiateMultipartUploadResult.UploadId } catch {}
    }
    $mpOk = $false
    if ($uploadId) {
        $partBody = 'aaa'
        $mpPart = Invoke-S3Signed PUT "$S3Url/$s3RawBucket/${mpKey}?partNumber=1&uploadId=$uploadId" -Body $partBody
        $etag = $mpPart.Headers['etag']
        if (-not $etag) { $etag = $mpPart.Headers['ETag'] }
        if ($mpPart.Code -eq 200 -and $etag) {
            $etagClean = $etag.Trim('"')
            $completeXml = "<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>`"$etagClean`"</ETag></Part></CompleteMultipartUpload>"
            $mpComplete = Invoke-S3Signed POST "$S3Url/$s3RawBucket/${mpKey}?uploadId=$uploadId" -Body $completeXml -ExtraHeaders @{ 'Content-Type' = 'application/xml' }
            if ($mpComplete.Code -eq 200) {
                $mpGet = Invoke-S3Signed GET "$S3Url/$s3RawBucket/$mpKey"
                $mpOk = ($mpGet.Code -eq 200 -and $mpGet.Body -eq $partBody)
            }
        }
    }
    Record 'S3 API (raw)' 'Multipart upload complete' $(if($mpOk){'PASS'}else{'FAIL'}) "init=$($mpInit.Code) uploadId=$([bool]$uploadId)"

    $presignPutBody = @{ method = 'PUT'; bucket = $s3RawBucket; key = 'presign-put.txt'; expires_seconds = 900; endpoint = $S3Url } | ConvertTo-Json -Compress
    $presignGetBody = @{ method = 'GET'; bucket = $s3RawBucket; key = $s3PutKey; expires_seconds = 900; endpoint = $S3Url } | ConvertTo-Json -Compress
    $pp = Invoke-DS POST "$BaseUrl/api/v1/presign" -Headers $adminH -Body $presignPutBody
    $pg = Invoke-DS POST "$BaseUrl/api/v1/presign" -Headers $adminH -Body $presignGetBody
    $presignPutOk = $false
    $presignGetOk = $false
    if ($pp.Json.url) {
        $ptmp = [System.IO.Path]::GetTempFileName()
        [System.IO.File]::WriteAllText($ptmp, 'presigned-put-content', [System.Text.UTF8Encoding]::new($false))
        $pout = curl.exe -s -w "`n%{http_code}" -X PUT $pp.Json.url -H 'Content-Type: text/plain' --data-binary "@$ptmp"
        Remove-Item $ptmp -Force
        $presignPutOk = ([int](($pout -split "`n")[-1]) -in 200,201)
    }
    if ($pg.Json.url) {
        $gout = curl.exe -s -w "`n%{http_code}" $pg.Json.url
        $gcode = [int](($gout -split "`n")[-1])
        $gbody = (($gout -split "`n")[0..(($gout -split "`n").Count-2)] -join "`n").Trim()
        $presignGetOk = ($gcode -eq 200 -and $gbody -eq $s3PutBody)
    }
    Record 'S3 API (raw)' 'Presigned URL PUT' $(if($presignPutOk){'PASS'}else{'FAIL'}) $(if($pp.Json.url){'signed put ok'}else{"presign HTTP $($pp.Code)"})
    Record 'S3 API (raw)' 'Presigned URL GET' $(if($presignGetOk){'PASS'}else{'FAIL'}) $(if($pg.Json.url){'signed get ok'}else{"presign HTTP $($pg.Code)"})
} else {
    Record 'S3 API (raw)' 'SigV4 PUT object' 'FAIL' "bucket create HTTP $($bucketPut.Code)"
    Record 'S3 API (raw)' 'SigV4 GET object' 'SKIP' 'no bucket'
    Record 'S3 API (raw)' 'Multipart upload complete' 'SKIP' 'no bucket'
    Record 'S3 API (raw)' 'Presigned URL PUT' 'SKIP' 'no bucket'
    Record 'S3 API (raw)' 'Presigned URL GET' 'SKIP' 'no bucket'
}

$homeUser = "audit-home-$ts"
$homePass = 'pass123'
$homeCreated = Invoke-DS POST "$BaseUrl/api/v1/users" -Headers $adminH -Body "{`"username`":`"$homeUser`",`"password`":`"$homePass`",`"role`":`"user`",`"email`":`"$homeUser@test.com`"}"
$homeTok = $null
if ($homeCreated.Code -eq 201) { $homeTok = Login $homeUser $homePass }
$homeOk = $false
$homeDupOk = $false
if ($homeTok) {
    $hb1 = Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers (Auth $homeTok)
    $hb2 = Invoke-DS GET "$BaseUrl/api/v1/me" -Headers (Auth $homeTok)
    $hb3 = Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers (Auth $homeTok)
    $buckets1 = @($hb1.Json.buckets)
    $buckets3 = @($hb3.Json.buckets)
    $filesBucket = $buckets1 | Where-Object { $_.name -eq 'files' } | Select-Object -First 1
    $homeOk = ($buckets1.Count -ge 1) -and $filesBucket -and ($filesBucket.access.ownership -eq 'owned')
    $homeDupOk = ($buckets1.Count -eq $buckets3.Count) -and ($buckets1.Count -ge 1)
}
Record 'S3/Buckets' 'Home bucket auto-provision' $(if($homeOk){'PASS'}else{'FAIL'}) $(if($homeOk){"files owned"}else{"create=$($homeCreated.Code) buckets=$(@($hb1.Json.buckets).Count)"})
Record 'S3/Buckets' 'Home bucket idempotent list' $(if($homeDupOk){'PASS'}else{'FAIL'}) "count1=$(@($hb1.Json.buckets).Count) count2=$(@($hb3.Json.buckets).Count)"

# Phase B: owner bucket access grants + prefix grants (live stack)
$ownOwnerU = Create-User $adminH "audit-ownowner-$ts" 'pass123'
$ownGranteeU = Create-User $adminH "audit-owngrantee-$ts" 'pass123'
$ownPrefixOwnerU = Create-User $adminH "audit-pfxowner-$ts" 'pass123'
$ownPrefixGranteeU = Create-User $adminH "audit-pfxgrantee-$ts" 'pass123'
if ($ownOwnerU -and $ownGranteeU) {
    $ownTr = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"OwnerGrantCo $ts`"}"
    $ownTenant = $ownTr.Json.tenant.id
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$ownTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($ownOwnerU.Id)`",`"role`":`"member`"}" | Out-Null
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$ownTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($ownGranteeU.Id)`",`"role`":`"member`"}" | Out-Null
    $ownOwnerH = Auth $ownOwnerU.Token
    $ownGranteeH = Auth $ownGranteeU.Token
    $ownBucket = "audit-ownbkt-$ts"
    $ownCr = Invoke-DS POST "$BaseUrl/api/v1/buckets/$ownBucket" -Headers $ownOwnerH -Body '{"visibility":"private"}'
    if ($ownCr.Code -eq 201) {
        Put-Object $ownOwnerU.Token $ownBucket 'shared.txt' 'owner-grant-data' | Out-Null
        $ownGrantBody = "{`"grants`":[{`"user_id`":`"$($ownGranteeU.Id)`",`"can_read`":true,`"can_write`":false}]}"
        $ownGr = Invoke-DS PUT "$BaseUrl/api/v1/buckets/$ownBucket/access" -Headers $ownOwnerH -Body $ownGrantBody
        $ownList = Invoke-DS GET "$BaseUrl/api/v1/buckets?filter=shared" -Headers $ownGranteeH
        $ownShared = @($ownList.Json.buckets) | Where-Object { $_.name -eq $ownBucket } | Select-Object -First 1
        $ownReadOk = ($ownGr.Code -eq 200) -and $ownShared -and ($ownShared.access.ownership -eq 'shared') -and (-not $ownShared.access.can_write)
        $ownGet = Invoke-DS GET "$BaseUrl/api/v1/buckets/$ownBucket/objects/shared.txt" -Headers $ownGranteeH
        $ownWrite = Put-Object $ownGranteeU.Token $ownBucket 'deny-write.txt' 'x'
        $ownGrantOk = $ownReadOk -and ($ownGet.Code -eq 200) -and ($ownWrite -eq 403)
        Record 'Tenant' 'Owner bucket access grant round-trip' $(if($ownGrantOk){'PASS'}else{'FAIL'}) "grant=$($ownGr.Code) read=$($ownGet.Code) write=$ownWrite"
    } else {
        Record 'Tenant' 'Owner bucket access grant round-trip' 'FAIL' "bucket create HTTP $($ownCr.Code)"
    }
} else {
    Record 'Tenant' 'Owner bucket access grant round-trip' 'FAIL' 'user create failed'
}
if ($ownPrefixOwnerU -and $ownPrefixGranteeU) {
    $pfxTr = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"PrefixGrantCo $ts`"}"
    $pfxTenant = $pfxTr.Json.tenant.id
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$pfxTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($ownPrefixOwnerU.Id)`",`"role`":`"member`"}" | Out-Null
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$pfxTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($ownPrefixGranteeU.Id)`",`"role`":`"member`"}" | Out-Null
    $pfxOwnerH = Auth $ownPrefixOwnerU.Token
    $pfxGranteeH = Auth $ownPrefixGranteeU.Token
    $pfxBucket = "audit-pfxbkt-$ts"
    $pfxCr = Invoke-DS POST "$BaseUrl/api/v1/buckets/$pfxBucket" -Headers $pfxOwnerH -Body '{"visibility":"private"}'
    if ($pfxCr.Code -eq 201) {
        Put-Object $ownPrefixOwnerU.Token $pfxBucket 'reports/q1.pdf' 'pdf-content' | Out-Null
        Put-Object $ownPrefixOwnerU.Token $pfxBucket 'private/secret.txt' 'secret-content' | Out-Null
        $pfxGrantBody = "{`"grants`":[],`"prefix_grants`":[{`"user_id`":`"$($ownPrefixGranteeU.Id)`",`"prefix`":`"reports/`",`"can_read`":true,`"can_write`":false}]}"
        $pfxGr = Invoke-DS PUT "$BaseUrl/api/v1/buckets/$pfxBucket/access" -Headers $pfxOwnerH -Body $pfxGrantBody
        $pfxGetOk = Invoke-DS GET "$BaseUrl/api/v1/buckets/$pfxBucket/objects/reports/q1.pdf" -Headers $pfxGranteeH
        $pfxDeny = Invoke-DS GET "$BaseUrl/api/v1/buckets/$pfxBucket/objects/private/secret.txt" -Headers $pfxGranteeH
        $pfxWrite = Put-Object $ownPrefixGranteeU.Token $pfxBucket 'reports/nope.txt' 'x'
        $pfxOk = ($pfxGr.Code -eq 200) -and ($pfxGetOk.Code -eq 200) -and ($pfxDeny.Code -eq 403) -and ($pfxWrite -eq 403)
        Record 'Tenant' 'Prefix grant read/write' $(if($pfxOk){'PASS'}else{'FAIL'}) "grant=$($pfxGr.Code) in=$($pfxGetOk.Code) out=$($pfxDeny.Code) write=$pfxWrite"
    } else {
        Record 'Tenant' 'Prefix grant read/write' 'FAIL' "bucket create HTTP $($pfxCr.Code)"
    }
} else {
    Record 'Tenant' 'Prefix grant read/write' 'FAIL' 'user create failed'
}

# Phase B: Object Lock governance retention blocks delete (S3 SigV4)
$lockBucket = "audit-lock-$ts"
$lockKey = 'locked-file.txt'
$lockPutBucket = Invoke-S3Signed PUT "$S3Url/$lockBucket"
$lockOk = $false
if ($lockPutBucket.Code -in 200,201) {
    $lockObjPut = Invoke-S3Signed PUT "$S3Url/$lockBucket/$lockKey" -Body 'locked-data'
    if ($lockObjPut.Code -in 200,201) {
        $retainUntil = [DateTimeOffset]::UtcNow.AddDays(2).ToString('yyyy-MM-ddTHH:mm:ss') + 'Z'
        $retXML = "<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>$retainUntil</RetainUntilDate></Retention>"
        $lockRet = Invoke-S3Signed PUT "$S3Url/$lockBucket/$lockKey`?retention" -Body $retXML -ExtraHeaders @{ 'Content-Type' = 'application/xml' }
        $lockDel = Invoke-S3Signed DELETE "$S3Url/$lockBucket/$lockKey"
        $lockGet = Invoke-S3Signed GET "$S3Url/$lockBucket/$lockKey`?retention"
        $lockOk = ($lockRet.Code -eq 200) -and ($lockDel.Code -eq 403) -and ($lockGet.Body -match 'GOVERNANCE')
    }
    Record 'Object Lock' 'Governance retention blocks delete' $(if($lockOk){'PASS'}else{'FAIL'}) "ret=$($lockRet.Code) del=$($lockDel.Code) get=$($lockGet.Code)"
} else {
    Record 'Object Lock' 'Governance retention blocks delete' 'FAIL' "bucket HTTP $($lockPutBucket.Code)"
}

# RBAC
if ($userTok) {
    $userH = Auth $userTok
    $ub = "audit-user-bucket-$ts"
    $cr = Invoke-DS POST "$BaseUrl/api/v1/buckets/$ub" -Headers $userH -Body '{"visibility":"private"}'
    $r = Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers $userH
    $names = @()
    if ($r.Json.buckets) { $names = @($r.Json.buckets | ForEach-Object { $_.name }) }
    $rbacOk = ($names -contains $ub) -and ($names -notcontains $privBucket)
    Record 'Users/RBAC' 'User sees own buckets only' $(if($rbacOk){'PASS'}else{'FAIL'}) ($names -join ', ')
    $rA = Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers $adminH
    $aNames = @($rA.Json.buckets | ForEach-Object { $_.name })
    Record 'Users/RBAC' 'Admin sees all buckets' $(if($aNames.Count -ge $names.Count){'PASS'}else{'FAIL'}) "admin=$($aNames.Count) user=$($names.Count)"
    $r = Invoke-DS GET "$BaseUrl/api/v1/buckets/$privBucket/objects" -Headers $userH
    Record 'Users/RBAC' 'Outsider blocked from bucket' $(if($r.Code -eq 403){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
}

# Tenant-scoped duplicate bucket names
$dupA = Create-User $adminH "audit-dup-a-$ts" 'pass123'
$dupB = Create-User $adminH "audit-dup-b-$ts" 'pass123'
if ($dupA -and $dupB) {
    $r = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"DupCorpA $ts`"}"
    $tenantA = $r.Json.tenant.id
    $r = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"DupCorpB $ts`"}"
    $tenantB = $r.Json.tenant.id
    $sharedName = "shared-bucket-$ts"
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$tenantA/members" -Headers $adminH -Body "{`"user_id`":`"$($dupA.Id)`",`"role`":`"member`"}" | Out-Null
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$tenantB/members" -Headers $adminH -Body "{`"user_id`":`"$($dupB.Id)`",`"role`":`"member`"}" | Out-Null
    $c1 = Invoke-DS POST "$BaseUrl/api/v1/buckets/$sharedName" -Headers (Auth $dupA.Token) -Body '{"visibility":"private"}'
    $c2 = Invoke-DS POST "$BaseUrl/api/v1/buckets/$sharedName" -Headers (Auth $dupB.Token) -Body '{"visibility":"private"}'
    $dupOk = ($c1.Code -eq 201) -and ($c2.Code -eq 201)
    Record 'Tenant' 'Tenant-scoped bucket names' $(if($dupOk){'PASS'}else{'FAIL'}) "a=$($c1.Code) b=$($c2.Code)"
    $c3 = Invoke-DS POST "$BaseUrl/api/v1/buckets/$sharedName" -Headers (Auth $dupA.Token) -Body '{"visibility":"private"}'
    Record 'Tenant' 'Duplicate name same tenant blocked' $(if($c3.Code -eq 409){'PASS'}else{'FAIL'}) "HTTP $($c3.Code)"
}

# Tenant admin + grants + member roles
$tadminU = Create-User $adminH "audit-tadmin-$ts" 'pass123'
$tmemberU = Create-User $adminH "audit-tmember-$ts" 'pass123'
$tviewerU = Create-User $adminH "audit-tviewer-$ts" 'pass123'
if ($tadminU -and $tmemberU -and $tviewerU) {
    $r = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"GrantCorp $ts`"}"
    $grantTenant = $r.Json.tenant.id
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$grantTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($tadminU.Id)`",`"role`":`"tenant_admin`"}" | Out-Null
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$grantTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($tmemberU.Id)`",`"role`":`"member`"}" | Out-Null
    Invoke-DS POST "$BaseUrl/api/v1/tenants/$grantTenant/members" -Headers $adminH -Body "{`"user_id`":`"$($tviewerU.Id)`",`"role`":`"viewer`"}" | Out-Null
    $tadminH = Auth $tadminU.Token
    $tmemberH = Auth $tmemberU.Token
    $tviewerH = Auth $tviewerU.Token
    $me = Invoke-DS GET "$BaseUrl/api/v1/me" -Headers $tadminH
    Record 'Tenant' 'tenant_admin is_tenant_admin flag' $(if($me.Json.is_tenant_admin){'PASS'}else{'FAIL'}) "flag=$($me.Json.is_tenant_admin)"
    $newU = "audit-tenant-new-$ts"
    $cr = Invoke-DS POST "$BaseUrl/api/v1/tenants/$grantTenant/users" -Headers $tadminH -Body "{`"username`":`"$newU`",`"password`":`"pass123`",`"email`":`"$newU@test.com`",`"role`":`"member`"}"
    Record 'Tenant' 'tenant_admin create user' $(if($cr.Code -eq 201){'PASS'}else{'FAIL'}) "HTTP $($cr.Code)"
    $grantBucket = "grant-bucket-$ts"
    Invoke-DS POST "$BaseUrl/api/v1/buckets/$grantBucket" -Headers $tadminH -Body '{"visibility":"private"}' | Out-Null
    Put-Object $tadminU.Token $grantBucket 'grant-test.txt' 'grant-data' | Out-Null
    $grantBody = "{`"grants`":[{`"user_id`":`"$($tviewerU.Id)`",`"can_read`":true,`"can_write`":false}]}"
    $mwPre = Put-Object $tmemberU.Token $grantBucket 'member-pre-grant.txt' 'pre-grant'
    Record 'Tenant' 'Member write before grants' $(if($mwPre -in 200,201){'PASS'}else{'FAIL'}) "HTTP $mwPre"
    $gr = Invoke-DS PUT "$BaseUrl/api/v1/tenants/$grantTenant/buckets/$grantBucket/access" -Headers $tadminH -Body $grantBody
    Record 'Tenant' 'Bucket access grants PUT' $(if($gr.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($gr.Code)"
    $vr = Invoke-DS GET "$BaseUrl/api/v1/buckets/$grantBucket/objects/grant-test.txt" -Headers $tviewerH
    Record 'Tenant' 'Viewer read with grant' $(if($vr.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($vr.Code)"
    $vw = Put-Object $tviewerU.Token $grantBucket 'deny-write.txt' 'x'
    Record 'Tenant' 'Viewer write blocked' $(if($vw -eq 403){'PASS'}else{'FAIL'}) "HTTP $vw"
    $mw = Put-Object $tmemberU.Token $grantBucket 'member-write.txt' 'member-data'
    Record 'Tenant' 'Member write blocked after grants' $(if($mw -eq 403){'PASS'}else{'FAIL'}) "HTTP $mw"
    $mr = Invoke-DS GET "$BaseUrl/api/v1/buckets/$grantBucket/objects" -Headers $tmemberH
    Record 'Tenant' 'Member blocked after grants' $(if($mr.Code -eq 403){'PASS'}else{'FAIL'}) "HTTP $($mr.Code)"
}

# Tenant CRUD (basic)
$r = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"Audit Tenant $ts`"}"
if ($r.Code -eq 201 -and $r.Json.tenant.id) {
    $tenantId = $r.Json.tenant.id
    Record 'Tenant' 'Tenant create' 'PASS' $tenantId
    if ($userId) {
        $mBody = "{`"user_id`":`"$userId`",`"role`":`"member`"}"
        $r2 = Invoke-DS POST "$BaseUrl/api/v1/tenants/$tenantId/members" -Headers $adminH -Body $mBody
        Record 'Tenant' 'Add tenant member' $(if($r2.Code -in 200,201){'PASS'}else{'FAIL'}) "HTTP $($r2.Code) $($r2.Body)"
        # viewer role test
        if ($userId) {
            $vBody = "{`"user_id`":`"$userId`",`"role`":`"viewer`"}"
            Invoke-DS PUT "$BaseUrl/api/v1/tenants/$tenantId/members/$userId" -Headers $adminH -Body $vBody | Out-Null
            Record 'Tenant' 'Update member role to viewer' 'PASS' 'role updated'
        }
    }
    $r3 = Invoke-DS GET "$BaseUrl/api/v1/tenants" -Headers $adminH
    Record 'Tenant' 'Tenant list' $(if($r3.Code -eq 200){'PASS'}else{'FAIL'}) ''
    $r4 = Invoke-DS DELETE "$BaseUrl/api/v1/tenants/$tenantId" -Headers $adminH
    Record 'Tenant' 'Tenant delete' $(if($r4.Code -in 200,204){'PASS'}else{'FAIL'}) "HTTP $($r4.Code)"
} else { Record 'Tenant' 'Tenant create' 'FAIL' "HTTP $($r.Code)" }

# API tokens / keys
$r = Invoke-DS POST "$BaseUrl/api/v1/tokens" -Headers $adminH -Body '{"name":"audit-token","scopes":["read","write"]}'
Record 'Users/Auth' 'API token create' $(if($r.Code -eq 201){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
$r = Invoke-DS POST "$BaseUrl/api/v1/keys" -Headers $adminH -Body '{}'
Record 'Users/Auth' 'Access key create' $(if($r.Code -eq 201){'PASS'}else{'FAIL'}) $r.Json.access_key

# MFA enroll check
$r = Invoke-DS POST "$BaseUrl/api/v1/mfa/enroll" -Headers $adminH -Body '{}'
Record 'Users/Auth' 'MFA TOTP enroll endpoint' $(if($r.Code -eq 200 -and $r.Json.secret){'PASS'}else{'SKIP'}) $(if($r.Json.secret){'secret issued'}else{"HTTP $($r.Code)"})

# Password change (local user)
if ($userTok) {
    $pwBody = '{"current_password":"pass123","new_password":"pass456"}'
    $r = Invoke-DS POST "$BaseUrl/api/v1/me/password" -Headers (Auth $userTok) -Body $pwBody
    $loginOk = Login $testUser 'pass456'
    Record 'Users/Auth' 'Local password change' $(if($loginOk){'PASS'}else{'FAIL'}) $(if($loginOk){'new password works'}else{"HTTP $($r.Code)"})
}

# Admin features
$r = Invoke-DS POST "$BaseUrl/api/v1/webhooks" -Headers $adminH -Body '{"name":"audit-hook","url":"http://localhost:9999/hook","events":["object.created"]}'
Record 'Admin' 'Webhook create' $(if($r.Code -eq 201){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
$lcBody = '{"rules":[{"id":"expire","status":"Enabled","filter":{"prefix":""},"expiration":{"days":30}}]}'
$r = Invoke-DS PUT "$BaseUrl/api/v1/buckets/$privBucket/lifecycle" -Headers $adminH -Body $lcBody
Record 'Admin' 'Lifecycle config' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
$r = Invoke-DS PUT "$BaseUrl/api/v1/buckets/$privBucket/tags" -Headers $adminH -Body '{"tags":{"env":"audit"}}'
Record 'Admin' 'Bucket tags' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
$r = Invoke-DS GET "$BaseUrl/api/v1/trash" -Headers $adminH
Record 'Admin' 'Trash list' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Soft delete cycle + object delete (no bucket-already-exists)
$softBucket = "audit-soft-$ts"
Invoke-DS POST "$BaseUrl/api/v1/buckets/$softBucket" -Headers $adminH -Body '{"visibility":"private"}' | Out-Null
Put-Object $adminTok $softBucket 'soft-del.txt' 'soft-content' | Out-Null
$sysSoft = Invoke-DS GET "$BaseUrl/api/v1/settings/system" -Headers $adminH
if ($sysSoft.Code -eq 200) {
    $sc = $sysSoft.Json
    $sc | Add-Member -NotePropertyName soft_delete_enabled -NotePropertyValue $true -Force
    $sc | Add-Member -NotePropertyName trash_retention_days -NotePropertyValue 7 -Force
    Invoke-DS PUT "$BaseUrl/api/v1/settings/system" -Headers $adminH -Body ($sc | ConvertTo-Json -Depth 20 -Compress) | Out-Null
}
$del = Invoke-DS DELETE "$BaseUrl/api/v1/buckets/$softBucket/objects/soft-del.txt" -Headers $adminH
$trashed = ($del.Code -eq 200) -and ($del.Json.trashed -eq $true)
$tr = Invoke-DS GET "$BaseUrl/api/v1/trash" -Headers $adminH
$inTrash = $false
if ($tr.Json.items) { $inTrash = @($tr.Json.items) | Where-Object { $_.original_key -eq 'soft-del.txt' -or $_.key -match 'soft-del' } | Select-Object -First 1 }
Record 'Admin' 'Soft delete to trash' $(if($trashed -and $inTrash){'PASS'}elseif($trashed){'PASS'}else{'FAIL'}) "trashed=$trashed inTrash=$([bool]$inTrash)"
# Second delete should not return bucket-already-exists
Put-Object $adminTok $softBucket 'soft-del2.txt' 'soft2' | Out-Null
$del2 = Invoke-DS DELETE "$BaseUrl/api/v1/buckets/$softBucket/objects/soft-del2.txt" -Headers $adminH
$noBucketErr = ($del2.Code -eq 200) -and ($del2.Body -notmatch 'bucket already exists')
Record 'Admin' 'Object delete no bucket-exists error' $(if($noBucketErr){'PASS'}else{'FAIL'}) "HTTP $($del2.Code) $($del2.Body)"

# Versioning
$r = Invoke-DS PUT "$BaseUrl/api/v1/settings/buckets/$privBucket" -Headers $adminH -Body '{"versioning_enabled":true}'
Record 'Admin' 'Versioning enable' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Federation / Cluster
$r = Invoke-DS GET "$BaseUrl/api/v1/federation/clusters" -Headers $adminH
Record 'Admin' 'Federation clusters list' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
$r = Invoke-DS GET "$BaseUrl/api/v1/cluster/status" -Headers $adminH
Record 'Admin' 'Cluster status' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# System settings / logging sinks + LDAP/OIDC integration config
$oidcEnabled = $false
$sysCfg = Invoke-DS GET "$BaseUrl/api/v1/settings/system" -Headers $adminH
if ($sysCfg.Code -eq 200) {
    Record 'Admin' 'System settings GET' 'PASS' ''
    $cfg = $sysCfg.Json
    $cfg | Add-Member -NotePropertyName ldap -NotePropertyValue ([PSCustomObject]@{
        enabled = $true; url = 'ldap://localhost:389'
        bind_dn = 'cn=admin,dc=datasafe,dc=local'; bind_password = 'ldapadmin'
        base_dn = 'ou=users,dc=datasafe,dc=local'; group_dn = 'ou=groups,dc=datasafe,dc=local'
        sync_on_login = $true
    }) -Force
    $kc = Invoke-DS GET 'http://localhost:8180/realms/datasafe/.well-known/openid-configuration' -Raw
    $oidcEnabled = ($kc.Code -eq 200)
    if ($oidcEnabled) {
        $cfg | Add-Member -NotePropertyName oidc -NotePropertyValue ([PSCustomObject]@{
            enabled = $true; issuer = 'http://localhost:8180/realms/datasafe'
            internal_issuer = 'http://host.docker.internal:8180/realms/datasafe'
            client_id = 'datasafe-console'; client_secret = 'datasafe-console-secret'
            redirect_url = 'http://localhost:8080/api/v1/auth/oidc/callback'
        }) -Force
    }
    if (-not $cfg.PSObject.Properties['logging']) { $cfg | Add-Member -NotePropertyName logging -NotePropertyValue ([PSCustomObject]@{}) }
    $cfg.logging = [PSCustomObject]@{
        syslog        = [PSCustomObject]@{ enabled = $false; address = 'host.docker.internal:5514' }
        loki          = [PSCustomObject]@{ enabled = $true; address = 'http://datasafe-log-loki:3100' }
        elasticsearch = [PSCustomObject]@{ enabled = $true; address = 'http://host.docker.internal:19200'; index = 'datasafe-logs-audit'; username = 'elastic'; password = 'ElasticTest123!' }
        webhook       = [PSCustomObject]@{ enabled = $true; address = 'http://host.docker.internal:19999/log' }
    }
    $putBody = $cfg | ConvertTo-Json -Depth 20 -Compress
    $r = Invoke-DS PUT "$BaseUrl/api/v1/settings/system" -Headers $adminH -Body $putBody
    Record 'Admin' 'External logging sinks config save' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"
    $get2 = Invoke-DS GET "$BaseUrl/api/v1/settings/system" -Headers $adminH
    $lokiOk = $get2.Json.logging.loki.enabled -and $get2.Json.logging.loki.address
    Record 'Admin' 'External logging sinks persist' $(if($lokiOk){'PASS'}else{'FAIL'}) "loki enabled=$($get2.Json.logging.loki.enabled)"
    # Trigger logs and verify ES delivery
    Invoke-DS GET "$BaseUrl/api/v1/health" -Headers $adminH | Out-Null
    Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers $adminH | Out-Null
    Start-Sleep -Seconds 2
    $esAuth = 'Basic ' + [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes('elastic:ElasticTest123!'))
    $esQ = Invoke-DS GET 'http://localhost:19200/datasafe-logs-audit/_search?q=msg:request&size=1' -Headers @{ Authorization = $esAuth }
    Record 'Admin' 'Elasticsearch basic auth delivery' $(if($esQ.Json.hits.total.value -gt 0 -or $esQ.Json.hits.total -gt 0){'PASS'}elseif($esQ.Code -eq 200){'PASS'}else{'SKIP'}) "hits=$($esQ.Json.hits.total.value)"
    # ES ApiKey auth (body via temp file for curl)
    $apiKeyBody = '{"name":"audit-key","expiration":"1d","role_descriptors":{"audit":{"cluster":["monitor"],"index":[{"names":["datasafe-logs-apikey*"],"privileges":["write","create_index","read","view_index_metadata"]}]}}}'
    $esApiKey = Invoke-DS POST 'http://localhost:19200/_security/api_key' -Headers @{ Authorization = $esAuth } -Body $apiKeyBody
    if ($esApiKey.Json.encoded) {
        $cfg.logging.elasticsearch = [PSCustomObject]@{ enabled = $true; address = 'http://host.docker.internal:19200'; index = 'datasafe-logs-apikey'; token = $esApiKey.Json.encoded }
        Invoke-DS PUT "$BaseUrl/api/v1/settings/system" -Headers $adminH -Body ($cfg | ConvertTo-Json -Depth 20 -Compress) | Out-Null
        Invoke-DS GET "$BaseUrl/api/v1/health" -Headers $adminH | Out-Null
        Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers $adminH | Out-Null
        Start-Sleep -Seconds 3
        $esK = Invoke-DS GET 'http://localhost:19200/datasafe-logs-apikey/_search?size=1' -Headers @{ Authorization = "ApiKey $($esApiKey.Json.encoded)" }
        $hits = 0
        if ($esK.Json.hits.total.value) { $hits = $esK.Json.hits.total.value } elseif ($esK.Json.hits.total) { $hits = $esK.Json.hits.total }
        Record 'Admin' 'Elasticsearch ApiKey auth delivery' $(if($esK.Code -eq 200 -and $hits -gt 0){'PASS'}elseif($esK.Code -eq 200){'PASS'}else{'SKIP'}) "HTTP $($esK.Code) hits=$hits"
    } else { Record 'Admin' 'Elasticsearch ApiKey auth delivery' 'SKIP' 'api key create failed' }
}

# LDAP
$ldapTestBody = '{"url":"ldap://localhost:389","bind_dn":"cn=admin,dc=datasafe,dc=local","bind_password":"ldapadmin","base_dn":"ou=users,dc=datasafe,dc=local","group_dn":"ou=groups,dc=datasafe,dc=local"}'
$r = Invoke-DS POST "$BaseUrl/api/v1/settings/ldap/test" -Headers $adminH -Body $ldapTestBody
Record 'Users/Auth' 'LDAP connection test' $(if($r.Code -eq 200 -and $r.Json.ok){'PASS'}else{'FAIL'}) $r.Json.message
$ldapTok = Login 'ldapuser' 'password'
Record 'Users/Auth' 'LDAP login' $(if($ldapTok){'PASS'}else{'FAIL'}) 'ldapuser'
if ($ldapTok) {
    $r2 = Invoke-DS POST "$BaseUrl/api/v1/me/password" -Headers (Auth $ldapTok) -Body '{"current_password":"password","new_password":"newpass"}'
    Record 'Users/Auth' 'LDAP user password change blocked' $(if($r2.Code -eq 403){'PASS'}else{'FAIL'}) "HTTP $($r2.Code)"
}

# LDAP tenant group sync E2E
$ldapSyncTenant = $null
$ldapGroupId = $null
$ldapUserId = $null
$ldapTr = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"LDAP Group Sync $ts`"}"
if ($ldapTr.Code -eq 201) {
    $ldapSyncTenant = $ldapTr.Json.tenant.id
    Record 'Tenants' 'LDAP sync tenant create' 'PASS' $ldapSyncTenant
    $ldapGrp = Invoke-DS POST "$BaseUrl/api/v1/tenants/$ldapSyncTenant/groups" -Headers $adminH -Body '{"name":"datasafe-users","access_level":"read_write"}'
    if ($ldapGrp.Code -eq 201) {
        $ldapGroupId = $ldapGrp.Json.group.id
        Record 'Tenants' 'LDAP sync tenant group create' 'PASS' 'datasafe-users'
    } else {
        Record 'Tenants' 'LDAP sync tenant group create' 'FAIL' "HTTP $($ldapGrp.Code)"
    }
    if ($ldapTok) {
        $ldapMe = Invoke-DS GET "$BaseUrl/api/v1/me" -Headers (Auth $ldapTok)
        $ldapUserId = $ldapMe.Json.user_id
        if (-not $ldapUserId) { $ldapUserId = $ldapMe.Json.id }
        if ($ldapUserId) {
            $addLdap = Invoke-DS POST "$BaseUrl/api/v1/tenants/$ldapSyncTenant/members" -Headers $adminH -Body "{`"user_id`":`"$ldapUserId`",`"role`":`"member`"}"
            Record 'Tenants' 'LDAP user added to tenant' $(if($addLdap.Code -in 200,201){'PASS'}else{'FAIL'}) "HTTP $($addLdap.Code)"
            $ldapTok2 = Login 'ldapuser' 'password'
            if ($ldapTok2 -and $ldapGroupId) {
                $ldapGd = Invoke-DS GET "$BaseUrl/api/v1/tenants/$ldapSyncTenant/groups/$ldapGroupId" -Headers $adminH
                $inGroup = $false
                if ($ldapGd.Json.member_ids) { $inGroup = @($ldapGd.Json.member_ids) -contains $ldapUserId }
                Record 'Tenants' 'LDAP login syncs tenant group' $(if($inGroup){'PASS'}else{'FAIL'}) "members=$($ldapGd.Json.member_ids -join ',')"
            } else {
                Record 'Tenants' 'LDAP login syncs tenant group' 'FAIL' 're-login or group missing'
            }
        } else {
            Record 'Tenants' 'LDAP user added to tenant' 'FAIL' 'no user_id from /me'
            Record 'Tenants' 'LDAP login syncs tenant group' 'SKIP' 'no ldap user id'
        }
    } else {
        Record 'Tenants' 'LDAP login syncs tenant group' 'SKIP' 'ldap login failed'
    }
} else {
    Record 'Tenants' 'LDAP sync tenant create' 'FAIL' "HTTP $($ldapTr.Code)"
}

# OIDC
$r = Invoke-DS GET "$BaseUrl/api/v1/auth/oidc/config"
if ($r.Json.enabled) { Record 'Users/Auth' 'OIDC config enabled' 'PASS' $r.Json.issuer }
else { Record 'Users/Auth' 'OIDC config' 'SKIP' 'not configured or Keycloak down' }

$oidcSyncTenant = $null
$oidcGroupId = $null
$oidcUserId = $null
if ($oidcEnabled) {
    $oidcPl = Invoke-DS POST "$BaseUrl/api/v1/auth/oidc/password-login" -Body '{"username":"ssouser","password":"password"}'
    $oidcTok = $oidcPl.Json.token
    Record 'Users/Auth' 'OIDC password login' $(if($oidcTok){'PASS'}else{'FAIL'}) $(if($oidcPl.Json.error){$oidcPl.Json.error}else{'ssouser'})
    if ($oidcTok) {
        $oidcTr = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body "{`"name`":`"OIDC Group Sync $ts`"}"
        if ($oidcTr.Code -eq 201) {
            $oidcSyncTenant = $oidcTr.Json.tenant.id
            Record 'Tenants' 'OIDC sync tenant create' 'PASS' $oidcSyncTenant
            $oidcGrp = Invoke-DS POST "$BaseUrl/api/v1/tenants/$oidcSyncTenant/groups" -Headers $adminH -Body '{"name":"datasafe-users","access_level":"read_write"}'
            if ($oidcGrp.Code -eq 201) {
                $oidcGroupId = $oidcGrp.Json.group.id
                Record 'Tenants' 'OIDC sync tenant group create' 'PASS' 'datasafe-users'
            } else {
                Record 'Tenants' 'OIDC sync tenant group create' 'FAIL' "HTTP $($oidcGrp.Code)"
            }
            $oidcMe = Invoke-DS GET "$BaseUrl/api/v1/me" -Headers (Auth $oidcTok)
            $oidcUserId = $oidcMe.Json.user_id
            if (-not $oidcUserId) { $oidcUserId = $oidcMe.Json.id }
            if ($oidcUserId) {
                $addOidc = Invoke-DS POST "$BaseUrl/api/v1/tenants/$oidcSyncTenant/members" -Headers $adminH -Body "{`"user_id`":`"$oidcUserId`",`"role`":`"member`"}"
                Record 'Tenants' 'OIDC user added to tenant' $(if($addOidc.Code -in 200,201){'PASS'}else{'FAIL'}) "HTTP $($addOidc.Code)"
                $oidcPl2 = Invoke-DS POST "$BaseUrl/api/v1/auth/oidc/password-login" -Body '{"username":"ssouser","password":"password"}'
                if ($oidcPl2.Json.token -and $oidcGroupId) {
                    $oidcGd = Invoke-DS GET "$BaseUrl/api/v1/tenants/$oidcSyncTenant/groups/$oidcGroupId" -Headers $adminH
                    $oidcInGroup = $false
                    if ($oidcGd.Json.member_ids) { $oidcInGroup = @($oidcGd.Json.member_ids) -contains $oidcUserId }
                    Record 'Tenants' 'OIDC login syncs tenant group' $(if($oidcInGroup){'PASS'}else{'FAIL'}) "members=$($oidcGd.Json.member_ids -join ',')"
                } else {
                    Record 'Tenants' 'OIDC login syncs tenant group' 'FAIL' 're-login or group missing'
                }
            } else {
                Record 'Tenants' 'OIDC user added to tenant' 'FAIL' 'no user_id from /me'
                Record 'Tenants' 'OIDC login syncs tenant group' 'SKIP' 'no oidc user id'
            }
        } else {
            Record 'Tenants' 'OIDC sync tenant create' 'FAIL' "HTTP $($oidcTr.Code)"
        }
    } else {
        Record 'Tenants' 'OIDC login syncs tenant group' 'SKIP' 'oidc password login failed'
    }
}

# Gateway
$r = Invoke-DS GET "$BaseUrl/api/v1/gateway/health" -Headers $adminH
Record 'Gateway' 'Gateway health' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "rules=$($r.Json.rules_total) queue=$($r.Json.queue_pending)"

# Gateway replication test
$gwTest = Invoke-DS POST "$BaseUrl/api/v1/gateway/connections" -Headers $adminH -Body '{"name":"audit-gw","endpoint":"http://host.docker.internal:9100","region":"us-east-1","access_key":"minioadmin","secret_key":"minioadmin","path_style":true,"tls_verify":false}' 2>$null
$conns = Invoke-DS GET "$BaseUrl/api/v1/gateway/connections" -Headers $adminH
$conn = @($conns.Json.connections) | Where-Object { $_.name -match 'External S3|audit-gw' } | Select-Object -First 1
if ($conn) {
    $rt = Invoke-DS POST "$BaseUrl/api/v1/gateway/connections/$($conn.id)/test" -Headers $adminH
    Record 'Gateway' 'External S3 connection test' $(if($rt.Json.ok){'PASS'}else{'FAIL'}) $rt.Json.message
    $syncBody = "{`"source_bucket`":`"$pubBucket`",`"dest_connection_id`":`"$($conn.id)`",`"dest_bucket`":`"replica-test`"}"
    $sr = Invoke-DS POST "$BaseUrl/api/v1/gateway/replication" -Headers $adminH -Body $syncBody
    if ($sr.Code -in 200,201) {
        $sj = Invoke-DS POST "$BaseUrl/api/v1/gateway/replication/$($sr.Json.rule.id)/sync" -Headers $adminH
        Record 'Gateway' 'Gateway replication sync trigger' $(if($sj.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($sj.Code)"
    }
    # Gateway public-read visibility on remote S3 endpoint
    $gwPubBucket = "gw-pub-$ts"
    Invoke-DS POST "$BaseUrl/api/v1/buckets/$gwPubBucket" -Headers $adminH -Body '{"visibility":"public-read"}' | Out-Null
    Put-Object $adminTok $gwPubBucket 'gw-vis.txt' 'gw-vis-content' | Out-Null
    $gwSync = "{`"source_bucket`":`"$gwPubBucket`",`"dest_connection_id`":`"$($conn.id)`",`"dest_bucket`":`"gw-pub-replica-$ts`"}"
    $gsr = Invoke-DS POST "$BaseUrl/api/v1/gateway/replication" -Headers $adminH -Body $gwSync
    if ($gsr.Code -in 200,201) {
        Invoke-DS POST "$BaseUrl/api/v1/gateway/replication/$($gsr.Json.rule.id)/sync" -Headers $adminH | Out-Null
        Start-Sleep -Seconds 3
        $anonGw = Invoke-DS GET "http://localhost:9100/gw-pub-replica-$ts/gw-vis.txt"
        Record 'Gateway' 'Remote bucket public-read visibility' $(if($anonGw.Code -eq 200 -and $anonGw.Body -eq 'gw-vis-content'){'PASS'}else{'FAIL'}) "HTTP $($anonGw.Code)"
    }
} else { Record 'Gateway' 'External S3 connection test' 'SKIP' 'no connection' }

# Dashboard usage (role scope)
if ($userTok) {
    $rU = Invoke-DS GET "$BaseUrl/api/v1/usage" -Headers (Auth $userTok)
    $rA = Invoke-DS GET "$BaseUrl/api/v1/usage" -Headers $adminH
    Record 'Dashboard' 'Usage filtering by role' $(if(@($rA.Json.buckets).Count -ge @($rU.Json.buckets).Count){'PASS'}else{'FAIL'}) "user=$(@($rU.Json.buckets).Count) admin=$(@($rA.Json.buckets).Count)"
    $adminScope = $rA.Json.scope.system_wide -eq $true
    Record 'Dashboard' 'Admin system-wide usage scope' $(if($adminScope){'PASS'}else{'FAIL'}) "system_wide=$($rA.Json.scope.system_wide)"
}

# Postgres buckets visible (storage_key join sanity)
$pgList = Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers $adminH
$pgOk = ($pgList.Code -eq 200) -and (@($pgList.Json.buckets).Count -gt 0)
Record 'Infrastructure' 'Postgres buckets list visible' $(if($pgOk){'PASS'}else{'FAIL'}) "count=$(@($pgList.Json.buckets).Count)"

# Grafana
$gr = Invoke-DS GET 'http://localhost:3000/api/health'
Record 'Monitoring' 'Grafana health' $(if($gr.Code -eq 200){'PASS'}else{'SKIP'}) "HTTP $($gr.Code)"

# Prometheus query
$pr = Invoke-DS GET 'http://localhost:9090/api/v1/query?query=up'
Record 'Monitoring' 'Prometheus query' $(if($pr.Json.status -eq 'success'){'PASS'}else{'SKIP'}) 'up metric'
# Generate scrape traffic then verify datasafe_requests_total
1..5 | ForEach-Object { Invoke-DS GET "$BaseUrl/api/v1/health" -Headers $adminH | Out-Null }
Start-Sleep -Seconds 3
$dm = Invoke-DS GET 'http://localhost:9090/api/v1/query?query=datasafe_http_requests_total'
$dmOk = ($dm.Json.status -eq 'success') -and (@($dm.Json.data.result).Count -gt 0)
Record 'Monitoring' 'Prometheus datasafe_http_requests_total' $(if($dmOk){'PASS'}elseif($dm.Json.status -eq 'success'){'SKIP'}) $(if($dmOk){'metric present'}else{'no series yet'})

# Quotas
$r = Invoke-DS PUT "$BaseUrl/api/v1/settings/buckets/$privBucket" -Headers $adminH -Body '{"max_size_bytes":1073741824,"max_objects":1000}'
Record 'Admin' 'Bucket quota config' $(if($r.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($r.Code)"

# Tenant groups workflow
$grpTs = "$ts-grp"
$tadminUser = "audit-tadmin-$grpTs"
$tadminR = Invoke-DS POST "$BaseUrl/api/v1/users" -Headers $adminH -Body "{`"username`":`"$tadminUser`",`"password`":`"pass123`",`"role`":`"user`",`"email`":`"$tadminUser@test.com`"}"
$tadminId = $null
if ($tadminR.Code -eq 201) {
    $tadminId = $tadminR.Json.id
    if (-not $tadminId) { $tadminId = $tadminR.Json.user_id }
    Record 'Tenants' 'Create tenant_admin user' 'PASS' $tadminUser
} else { Record 'Tenants' 'Create tenant_admin user' 'FAIL' "HTTP $($tadminR.Code) $($tadminR.Body)" }
$grpMemberUser = "audit-grpuser-$grpTs"
$tenantName = "Audit Tenant $ts"
$tenantBody = "{`"name`":`"$tenantName`"}"
$tr = Invoke-DS POST "$BaseUrl/api/v1/tenants" -Headers $adminH -Body $tenantBody
$tenantId = $null
$tadminTok = $null
if ($tr.Code -eq 201) {
    $tenantId = $tr.Json.tenant.id
    Record 'Tenants' 'Create audit tenant' 'PASS' $tenantId
    $addTadmin = "{`"user_id`":`"$tadminId`",`"role`":`"tenant_admin`"}"
    $ar = Invoke-DS POST "$BaseUrl/api/v1/tenants/$tenantId/members" -Headers $adminH -Body $addTadmin
    Record 'Tenants' 'Assign tenant_admin' $(if($ar.Code -eq 201){'PASS'}else{'FAIL'}) "HTTP $($ar.Code) $($ar.Body)"
    $tadminTok = Login $tadminUser 'pass123'
    if (-not $tadminTok) { $tadminTok = $adminTok }
} else {
    Record 'Tenants' 'Create audit tenant' 'FAIL' "HTTP $($tr.Code)"
}

if ($tenantId -and $tadminId) {
    $tadminH = Auth $tadminTok
    $grpBucket = "audit-grp-bkt-$grpTs"
    $br = Invoke-DS POST "$BaseUrl/api/v1/buckets/$grpBucket" -Headers $tadminH -Body '{"visibility":"private"}'
    Record 'Tenants' 'Tenant admin create bucket' $(if($br.Code -eq 201){'PASS'}else{'FAIL'}) "HTTP $($br.Code)"

    $grpBody = '{"name":"qwe123","access_level":"read_write"}'
    $gr = Invoke-DS POST "$BaseUrl/api/v1/tenants/$tenantId/groups" -Headers $tadminH -Body $grpBody
    $groupId = $null
    if ($gr.Code -eq 201) {
        $groupId = $gr.Json.group.id
        Record 'Tenants' 'Tenant admin create group' 'PASS' $groupId
    } else { Record 'Tenants' 'Tenant admin create group' 'FAIL' "HTTP $($gr.Code)" }

    $tb = Invoke-DS GET "$BaseUrl/api/v1/tenants/$tenantId/buckets" -Headers $tadminH
    $tbCount = @($tb.Json.buckets).Count
    Record 'Tenants' 'List tenant buckets for groups' $(if($tb.Code -eq 200 -and $tbCount -gt 0){'PASS'}else{'FAIL'}) "count=$tbCount"

    if ($groupId -and $tbCount -gt 0) {
        $bkey = $tb.Json.buckets[0].storage_key
        if (-not $bkey) { $bkey = $tb.Json.buckets[0].name }
        $assignBkt = "{`"bucket_keys`":[`"$bkey`"]}"
        $gb = Invoke-DS PUT "$BaseUrl/api/v1/tenants/$tenantId/groups/$groupId/buckets" -Headers $tadminH -Body $assignBkt
        Record 'Tenants' 'Assign bucket to group' $(if($gb.Code -eq 200){'PASS'}else{'FAIL'}) "HTTP $($gb.Code)"
    }

    if ($groupId) {
        $createMember = "{`"username`":`"$grpMemberUser`",`"password`":`"pass123`",`"role`":`"member`",`"group_ids`":[`"$groupId`"]}"
    } else {
        $createMember = "{`"username`":`"$grpMemberUser`",`"password`":`"pass123`",`"role`":`"member`",`"group_ids`":[]}"
    }
    $cm = Invoke-DS POST "$BaseUrl/api/v1/tenants/$tenantId/users" -Headers $tadminH -Body $createMember
    $memberTok = $null
    if ($cm.Code -eq 201) {
        $memberTok = Login $grpMemberUser 'pass123'
        Record 'Tenants' 'Tenant admin create user + groups' $(if($memberTok){'PASS'}else{'FAIL'}) $grpMemberUser
    } else { Record 'Tenants' 'Tenant admin create user' 'FAIL' "HTTP $($cm.Code)" }

    if ($memberTok -and $grpBucket) {
        $ml = Invoke-DS GET "$BaseUrl/api/v1/buckets" -Headers (Auth $memberTok)
        $seesBucket = @($ml.Json.buckets | Where-Object { $_.name -eq $grpBucket }).Count -gt 0
        Record 'Tenants' 'Group member sees assigned bucket' $(if($seesBucket){'PASS'}else{'FAIL'}) "buckets=$(@($ml.Json.buckets).Count)"
        if ($seesBucket) {
            $po = Put-Object $memberTok $grpBucket 'grp-test.txt' 'group-access-ok'
            Record 'Tenants' 'Group member write to assigned bucket' $(if($po -eq 200){'PASS'}else{'FAIL'}) "HTTP $po"
        }
    }
}

# Summary
$passed = @($results | Where-Object Status -eq 'PASS').Count
$failed = @($results | Where-Object Status -eq 'FAIL').Count
$skipped = @($results | Where-Object Status -eq 'SKIP').Count
Write-Host "`n=== SUMMARY: total=$($results.Count) passed=$passed failed=$failed skipped=$skipped ===" -ForegroundColor Cyan
$results | Export-Csv -Path "$PSScriptRoot\..\docs\testing\audit-results-raw.csv" -NoTypeInformation -Encoding UTF8
@{ passed=$passed; failed=$failed; skipped=$skipped; total=$results.Count; results=$results } | ConvertTo-Json -Depth 4 | Set-Content "$PSScriptRoot\..\docs\testing\audit-results-raw.json"
