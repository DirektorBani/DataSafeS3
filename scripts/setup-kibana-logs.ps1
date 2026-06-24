param(
    [string]$KibanaUrl = 'http://localhost:5601',
    [string]$EsUrl = 'http://localhost:19200',
    [string]$Username = 'elastic',
    [string]$Password = 'ElasticTest123!',
    [string]$DataViewTitle = 'datasafe-logs*',
    [string]$TimeField = 'time'
)

$ErrorActionPreference = 'Stop'

$pair = "${Username}:${Password}"
$auth = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($pair))
$kbHeaders = @{
    Authorization  = "Basic $auth"
    'kbn-xsrf'     = 'true'
    'Content-Type' = 'application/json'
}
$esHeaders = @{ Authorization = "Basic $auth" }

function Get-DataViewId {
    param([string]$Title)
    $list = Invoke-RestMethod -Uri "$KibanaUrl/api/data_views" -Headers $kbHeaders
    $match = $list.data_view | Where-Object { $_.title -eq $Title } | Select-Object -First 1
    if ($match) { return $match.id }
    return $null
}

Write-Host "Elasticsearch indices matching $DataViewTitle"
$catUrl = "$EsUrl/_cat/indices?v&h=index,docs.count,store.size"
$indices = curl.exe -s -u "${Username}:${Password}" $catUrl | Select-String 'datasafe-logs'
if ($indices) { $indices | ForEach-Object { Write-Host $_.Line.Trim() } } else { Write-Host '(no matching indices)' }

$dataViewId = Get-DataViewId -Title $DataViewTitle
if (-not $dataViewId) {
    Write-Host "Creating data view $DataViewTitle (time field: $TimeField)..."
    $body = @{
        data_view = @{
            title         = $DataViewTitle
            name          = 'DataSafeS3 Logs'
            timeFieldName = $TimeField
        }
    } | ConvertTo-Json -Depth 5 -Compress
    $dv = Invoke-RestMethod -Method Post -Uri "$KibanaUrl/api/data_views/data_view" -Headers $kbHeaders -Body $body
    $dataViewId = $dv.data_view.id
    Write-Host "Created data view id=$dataViewId"
} else {
    Write-Host "Data view already exists id=$dataViewId"
}

Write-Host "Setting default data view..."
$defaultBody = @{ data_view_id = $dataViewId; force = $true } | ConvertTo-Json -Compress
try {
    Invoke-RestMethod -Method Post -Uri "$KibanaUrl/api/data_views/default" -Headers $kbHeaders -Body $defaultBody | Out-Null
} catch {
    Write-Host "Warning: could not set default data view: $_"
}

Write-Host "Ensuring saved search 'DataSafeS3 Logs'..."
$searchSource = @{
    query        = @{ query = ''; language = 'kuery' }
    filter       = @()
    indexRefName = 'kibanaSavedObjectMeta.searchSourceJSON.index'
} | ConvertTo-Json -Compress
$searchBody = @{
    attributes = @{
        title                 = 'DataSafeS3 Logs'
        columns               = @('time', 'level', 'msg', 'method', 'path', 'status', 'duration_ms', 'remote')
        sort                  = @(@($TimeField, 'desc'))
        kibanaSavedObjectMeta = @{
            searchSourceJSON = $searchSource
        }
    }
    references = @(
        @{
            name = 'kibanaSavedObjectMeta.searchSourceJSON.index'
            type = 'index-pattern'
            id   = $dataViewId
        }
    )
} | ConvertTo-Json -Depth 10 -Compress
try {
    Invoke-RestMethod -Method Post -Uri "$KibanaUrl/api/saved_objects/search/datasafe-logs-recent?overwrite=true" -Headers $kbHeaders -Body $searchBody | Out-Null
    Write-Host "Saved search datasafe-logs-recent ready"
} catch {
    Write-Host "Warning: saved search setup failed: $_"
}

$discoverUrl = "$KibanaUrl/app/discover#/?_g=(filters:!(),refreshInterval:(pause:!t,value:60000),time:(from:now-24h,to:now))&_a=(columns:!(time,level,msg,method,path,status,duration_ms,remote),dataViewId:'$dataViewId',filters:!(),interval:auto,query:(language:kuery,query:''),sort:!(!('$TimeField',desc)))"
$savedSearchUrl = "$KibanaUrl/app/discover#/view/datasafe-logs-recent?_g=(time:(from:now-24h,to:now))"

Write-Host ""
Write-Host "Data view id:     $dataViewId"
Write-Host "Discover (logs):  $discoverUrl"
Write-Host "Saved search:     $savedSearchUrl"
Write-Host "Login:            $Username / $Password"
Write-Host "Tip: set time range to Last 24 hours if empty."
