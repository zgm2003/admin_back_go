param(
  [string]$Account = $env:SMOKE_LOGIN_ACCOUNT,
  [string]$Password = $env:SMOKE_LOGIN_PASSWORD,
  [string]$HTTPAddr = '127.0.0.1:18081',
  [string]$BasicHTTPAddr = '127.0.0.1:18080',
  [string]$Platform = 'admin',
  [string]$DeviceID = 'codex-full-smoke'
)

$ErrorActionPreference = 'Stop'

$BackendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Set-Location $BackendRoot

function Set-EnvIfMissing([string]$Name, [string]$Value) {
  if ([string]::IsNullOrWhiteSpace($Value)) { return }
  if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($Name, 'Process'))) {
    [Environment]::SetEnvironmentVariable($Name, $Value, 'Process')
  }
}

function Import-DotEnv([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path)) { return }

  Get-Content -LiteralPath $Path | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq '' -or $line.StartsWith('#')) { return }

    $idx = $line.IndexOf('=')
    if ($idx -le 0) { return }

    $name = $line.Substring(0, $idx).Trim()
    $value = $line.Substring($idx + 1).Trim().Trim('"').Trim("'")
    Set-EnvIfMissing $name $value
  }
}

function Get-RedisAddr() {
  if (-not [string]::IsNullOrWhiteSpace($env:REDIS_ADDR)) { return $env:REDIS_ADDR }

  $hostName = if ([string]::IsNullOrWhiteSpace($env:REDIS_HOST)) { '127.0.0.1' } else { $env:REDIS_HOST }
  $port = if ([string]::IsNullOrWhiteSpace($env:REDIS_PORT)) { '6379' } else { $env:REDIS_PORT }
  return "$hostName`:$port"
}

function Get-RedisDB() {
  if ([string]::IsNullOrWhiteSpace($env:REDIS_DB)) { return '0' }
  return $env:REDIS_DB
}

function Wait-Health([string]$BaseURL) {
  for ($i = 0; $i -lt 30; $i++) {
    try {
      Invoke-RestMethod "$BaseURL/health" -TimeoutSec 2 | Out-Null
      return
    } catch {
      if ($i -eq 29) { throw }
      Start-Sleep -Milliseconds 500
    }
  }
}

function Assert-PortFree([string]$Addr) {
  $port = ($Addr -split ':')[-1]
  $listener = netstat -ano | Select-String ":$port\s+.*LISTENING"
  if ($listener) {
    throw "Port $port is already listening. Stop the existing process first, then rerun full smoke."
  }
}

function Assert-ApiOK($Response, [string]$Label) {
  if ($Response.code -ne 0) {
    throw "$Label failed: $($Response | ConvertTo-Json -Depth 12)"
  }
}

function Get-ObjectArray($Value) {
  if ($null -eq $Value) { return @() }
  return @($Value)
}

function Test-RoutePath($Routes, [string]$Path) {
  foreach ($route in (Get-ObjectArray $Routes)) {
    if ($route.path -eq $Path) { return $true }
  }
  return $false
}

function Test-HasProperty($Value, [string]$Name) {
  if ($null -eq $Value) { return $false }
  return @($Value.PSObject.Properties.Name) -contains $Name
}

function Test-JsonArray($Value) {
  if ($null -eq $Value) { return $false }
  return $Value -is [System.Array]
}

function Get-MaxOperationLogID($Response) {
  [int64]$maxID = 0
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    $id = [int64]$item.id
    if ($id -gt $maxID) { $maxID = $id }
  }
  return $maxID
}

function Invoke-JsonRequestAllowFailure([string]$Method, [string]$URL, [hashtable]$Headers, $Body) {
  $jsonBody = $Body | ConvertTo-Json -Depth 8

  try {
    return Invoke-RestMethod $URL `
      -Method $Method `
      -Headers $Headers `
      -ContentType 'application/json' `
      -Body $jsonBody `
      -TimeoutSec 10
  } catch {
    $response = $_.Exception.Response
    if ($null -eq $response) { throw }

    $text = [string]$_.ErrorDetails.Message
    if ([string]::IsNullOrWhiteSpace($text) -and $response -is [System.Net.Http.HttpResponseMessage]) {
      try {
        $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
      } catch {
        $text = ''
      }
    }

    if ([string]::IsNullOrWhiteSpace($text) -and -not ($response -is [System.Net.Http.HttpResponseMessage])) {
      $stream = $response.GetResponseStream()
      if ($null -eq $stream) { throw }

      $reader = New-Object System.IO.StreamReader($stream)
      try {
        $text = $reader.ReadToEnd()
      } finally {
        $reader.Dispose()
      }
    }

    if ([string]::IsNullOrWhiteSpace($text)) { throw }
    return $text | ConvertFrom-Json
  }
}

function Assert-ApiFailureCode($Response, [string]$Label, [int]$ExpectedCode = 100) {
  if ($Response.code -ne $ExpectedCode) {
    throw "$Label expected code=$ExpectedCode, got: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [int]$Response.code
}

function Get-OperationLogList([string]$BaseURL, [hashtable]$Headers, [string]$Action) {
  $actionQuery = [uri]::EscapeDataString($Action)
  return Invoke-RestMethod "$BaseURL/api/admin/v1/operation-logs?current_page=1&page_size=20&action=$actionQuery" `
    -Headers $Headers `
    -TimeoutSec 10
}

function Wait-NewOperationLog([string]$BaseURL, [hashtable]$Headers, [string]$Action, [int64]$AfterID) {
  for ($i = 0; $i -lt 20; $i++) {
    $logs = Get-OperationLogList $BaseURL $Headers $Action
    Assert-ApiOK $logs "operation log list for $Action"

    foreach ($item in (Get-ObjectArray $logs.data.list)) {
      if ([int64]$item.id -gt $AfterID -and [string]$item.action -eq $Action) {
        return $item
      }
    }

    Start-Sleep -Milliseconds 300
  }

  throw "operation log action=$Action was not written after id=$AfterID"
}

function Test-QueueMonitorItemShape($Item) {
  if ($null -eq $Item) { return $false }
  if ([string]::IsNullOrWhiteSpace([string]$Item.name)) { return $false }
  if ([string]::IsNullOrWhiteSpace([string]$Item.label)) { return $false }
  if ([string]::IsNullOrWhiteSpace([string]$Item.group)) { return $false }

  foreach ($field in @('waiting', 'delayed', 'failed', 'pending', 'active', 'scheduled', 'retry', 'archived', 'completed', 'processed', 'failed_today', 'processed_total', 'failed_total', 'latency_ms')) {
    if ($null -eq $Item.$field) { return $false }
  }

  if ($null -eq $Item.paused) { return $false }
  return $true
}

function Assert-QueueMonitorList($Response) {
  Assert-ApiOK $Response 'queue monitor list'

  $items = Get-ObjectArray $Response.data
  if ($items.Count -le 0) {
    throw "queue monitor list returned no queues: $($Response | ConvertTo-Json -Depth 12)"
  }

  $critical = $null
  foreach ($item in $items) {
    if (-not (Test-QueueMonitorItemShape $item)) {
      throw "queue monitor item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]$item.name -eq 'critical') {
      $critical = $item
    }
  }

  if ($null -eq $critical) {
    throw "queue monitor list missing critical queue: $($Response | ConvertTo-Json -Depth 12)"
  }

  return $items.Count
}

function Assert-QueueFailedList($Response) {
  Assert-ApiOK $Response 'queue monitor failed list'

  if ($null -eq $Response.data.page) {
    throw "queue monitor failed list missing page: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.list) {
    throw "queue monitor failed list missing list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([string]::IsNullOrWhiteSpace([string]$item.id)) {
      throw "queue monitor failed task missing id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.state)) {
      throw "queue monitor failed task missing state: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [int64]$Response.data.page.total
}

function Assert-SystemSettingInit($Response) {
  Assert-ApiOK $Response 'system settings init'

  if ($null -eq $Response.data.dict) {
    throw "system settings init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $options = Get-ObjectArray $Response.data.dict.system_setting_value_type_arr
  if ($options.Count -ne 4) {
    throw "system settings value type dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  $values = @($options | ForEach-Object { [int]$_.value })
  foreach ($expected in @(1, 2, 3, 4)) {
    if (-not ($values -contains $expected)) {
      throw "system settings value type dict missing value=${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return $options.Count
}

function Assert-SystemSettingList($Response) {
  Assert-ApiOK $Response 'system settings list'

  if ($null -eq $Response.data.page) {
    throw "system settings list missing page: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.list) {
    throw "system settings list missing list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "system settings item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.setting_key)) {
      throw "system settings item missing setting_key: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.value_type -or [string]::IsNullOrWhiteSpace([string]$item.value_type_name)) {
      throw "system settings item missing value type fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.status -or [string]::IsNullOrWhiteSpace([string]$item.status_name)) {
      throw "system settings item missing status fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-SystemLogInit($Response) {
  Assert-ApiOK $Response 'system log init'

  if ($null -eq $Response.data.dict) {
    throw "system log init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $levels = Get-ObjectArray $Response.data.dict.log_level_arr
  $tails = Get-ObjectArray $Response.data.dict.log_tail_arr
  if ($levels.Count -ne 5) {
    throw "system log level dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($tails.Count -ne 5) {
    throw "system log tail dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($expected in @('DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL')) {
    if (-not (@($levels | ForEach-Object { [string]$_.value }) -contains $expected)) {
      throw "system log level dict missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    LevelCount = $levels.Count
    TailCount = $tails.Count
  }
}

function Assert-SystemLogFiles($Response) {
  Assert-ApiOK $Response 'system log files'

  if ($null -eq $Response.data.list) {
    throw "system log files missing list: $($Response | ConvertTo-Json -Depth 12)"
  }

  $items = Get-ObjectArray $Response.data.list
  foreach ($item in $items) {
    if ([string]::IsNullOrWhiteSpace([string]$item.name)) {
      throw "system log file missing name: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.size -or [int64]$item.size -lt 0) {
      throw "system log file invalid size: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.size_human) -or [string]::IsNullOrWhiteSpace([string]$item.mtime)) {
      throw "system log file metadata incomplete: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    Count = $items.Count
    FirstName = if ($items.Count -gt 0) { [string]$items[0].name } else { $null }
  }
}

function Invoke-SystemLogLinesProbe([string]$BaseURL, [hashtable]$Headers, [string]$Filename) {
  if ([string]::IsNullOrWhiteSpace($Filename)) {
    return [pscustomobject]@{
      Status = 'skipped_no_log_files'
      Code = $null
      Filename = $null
      Total = 0
    }
  }

  $encodedName = [uri]::EscapeDataString($Filename)
  $response = Invoke-RestMethod "$BaseURL/api/admin/v1/system-logs/files/$encodedName/lines?tail=20" `
    -Headers $Headers `
    -TimeoutSec 10
  Assert-ApiOK $response 'system log lines'

  if ([string]::IsNullOrWhiteSpace([string]$response.data.filename)) {
    throw "system log lines missing filename: $($response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $response.data.total -or $null -eq $response.data.lines) {
    throw "system log lines missing total/list: $($response | ConvertTo-Json -Depth 12)"
  }

  foreach ($line in (Get-ObjectArray $response.data.lines)) {
    if ($null -eq $line.number -or [int]$line.number -le 0) {
      throw "system log line invalid number: $($line | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $line.level -or $null -eq $line.content) {
      throw "system log line missing level/content: $($line | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    Status = 'passed'
    Code = [int]$response.code
    Filename = [string]$response.data.filename
    Total = [int]$response.data.total
  }
}

function Assert-UploadDriverInit($Response) {
  Assert-ApiOK $Response 'upload driver init'

  $options = Get-ObjectArray $Response.data.dict.upload_driver_arr
  if ($options.Count -lt 2) {
    throw "upload driver dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  $values = @($options | ForEach-Object { [string]$_.value })
  foreach ($expected in @('cos', 'oss')) {
    if (-not ($values -contains $expected)) {
      throw "upload driver dict missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }
  return $options.Count
}

function Assert-UploadDriverList($Response) {
  Assert-ApiOK $Response 'upload driver list'
  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "upload driver list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.driver)) {
      throw "upload driver item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -ne $item.secret_id_enc -or $null -ne $item.secret_key_enc -or $null -ne $item.secret_id -or $null -ne $item.secret_key) {
      throw "upload driver list leaked secret fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }
  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-UploadRuleInit($Response) {
  Assert-ApiOK $Response 'upload rule init'
  $imageExts = Get-ObjectArray $Response.data.dict.upload_image_ext_arr
  $fileExts = Get-ObjectArray $Response.data.dict.upload_file_ext_arr
  if ($imageExts.Count -le 0 -or $fileExts.Count -le 0) {
    throw "upload rule init missing ext dicts: $($Response | ConvertTo-Json -Depth 12)"
  }
  return [pscustomobject]@{
    ImageExtCount = $imageExts.Count
    FileExtCount = $fileExts.Count
  }
}

function Assert-UploadRuleList($Response) {
  Assert-ApiOK $Response 'upload rule list'
  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "upload rule list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "upload rule item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.image_exts -or $null -eq $item.file_exts) {
      throw "upload rule item missing ext arrays: $($item | ConvertTo-Json -Depth 12)"
    }
  }
  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-UploadSettingInit($Response) {
  Assert-ApiOK $Response 'upload setting init'
  if ($null -eq $Response.data.dict.common_status_arr) {
    throw "upload setting init missing status dict: $($Response | ConvertTo-Json -Depth 12)"
  }
  $statusOptions = Get-ObjectArray $Response.data.dict.common_status_arr
  if ($statusOptions.Count -ne 2) {
    throw "upload setting status dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  return [pscustomobject]@{
    DriverDictCount = (Get-ObjectArray $Response.data.dict.upload_driver_list).Count
    RuleDictCount = (Get-ObjectArray $Response.data.dict.upload_rule_list).Count
    StatusDictCount = $statusOptions.Count
  }
}

function Assert-UploadSettingList($Response) {
  Assert-ApiOK $Response 'upload setting list'
  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "upload setting list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or $null -eq $item.status -or [string]::IsNullOrWhiteSpace([string]$item.status_name)) {
      throw "upload setting item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
  }
  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-PaymentChannelInit($Response) {
  Assert-ApiOK $Response 'payment channel init'

  if ($null -eq $Response.data.dict) {
    throw "payment channel init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $statuses = Get-ObjectArray $Response.data.dict.common_status_arr
  if ($statuses.Count -ne 2) {
    throw "payment channel init status dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    StatusCount = $statuses.Count
  }
}

function Assert-PaymentChannelList($Response) {
  Assert-ApiOK $Response 'payment channel list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "payment channel list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.name)) {
      throw "payment channel item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -ne $item.app_private_key -or $null -ne $item.app_private_key_enc) {
      throw "payment channel list leaked private key fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-PaymentOrderInit($Response) {
  Assert-ApiOK $Response 'payment order init'
  if ($null -eq $Response.data.dict) {
    throw "payment order init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }
  return [pscustomobject]@{
    DictKeys = (Get-ObjectArray $Response.data.dict.PSObject.Properties).Count
  }
}

function Assert-PaymentOrderList($Response) {
  Assert-ApiOK $Response 'payment order list'
  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "payment order list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "payment order item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
  }
  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-PaymentEventList($Response) {
  Assert-ApiOK $Response 'payment event list'
  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "payment event list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "payment event item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
  }
  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-UsersInitPaymentRoutes($Response) {
  Assert-ApiOK $Response 'users init payment route gate'
  $payPresent = Test-RoutePath $Response.data.router '/pay'
  $retiredWalletPresent = Test-RoutePath $Response.data.router '/wallet'
  $channelPresent = Test-RoutePath $Response.data.router '/payment/channel'
  $orderPresent = Test-RoutePath $Response.data.router '/payment/order'
  $eventPresent = Test-RoutePath $Response.data.router '/payment/event'
  if ($payPresent -or $retiredWalletPresent -or -not $channelPresent -or -not $orderPresent -or -not $eventPresent) {
    throw "users/init payment route gate mismatch: /pay=$payPresent /wallet=$retiredWalletPresent /payment/channel=$channelPresent /payment/order=$orderPresent /payment/event=$eventPresent"
  }
  return [pscustomobject]@{
    PayPresent = $payPresent
    RetiredWalletPresent = $retiredWalletPresent
    ChannelPresent = $channelPresent
    OrderPresent = $orderPresent
    EventPresent = $eventPresent
  }
}

function Assert-UsersInitAIRoutes($Response) {
  Assert-ApiOK $Response 'users init AI route gate'

  $goodsPresent = Test-RoutePath $Response.data.router '/ai/goods'
  $cinePresent = Test-RoutePath $Response.data.router '/ai/cine'
  $modelsPresent = Test-RoutePath $Response.data.router '/ai/models'
  $toolsPresent = Test-RoutePath $Response.data.router '/ai/tools'
  $promptsPresent = Test-RoutePath $Response.data.router '/ai/prompts'

  if ($goodsPresent) {
    throw 'users init still returns retired AI goods route /ai/goods'
  }
  if ($cinePresent) {
    throw 'users init still returns retired AI cine route /ai/cine'
  }
  if (-not $modelsPresent) {
    throw 'users init missing retained AI config route /ai/models'
  }
  if (-not $toolsPresent) {
    throw 'users init missing retained AI config route /ai/tools'
  }
  if (-not $promptsPresent) {
    throw 'users init missing retained AI config route /ai/prompts'
  }

  return [pscustomobject]@{
    GoodsPresent = $goodsPresent
    CinePresent = $cinePresent
    ModelsPresent = $modelsPresent
    ToolsPresent = $toolsPresent
    PromptsPresent = $promptsPresent
  }
}

function Assert-AIModelInit($Response) {
  Assert-ApiOK $Response 'AI model init'

  if ($null -eq $Response.data.dict) {
    throw "AI model init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $drivers = Get-ObjectArray $Response.data.dict.ai_driver_arr
  $statuses = Get-ObjectArray $Response.data.dict.common_status_arr
  if ($drivers.Count -lt 1 -or $statuses.Count -ne 2) {
    throw "AI model init dict mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    DriverCount = $drivers.Count
    StatusCount = $statuses.Count
  }
}

function Assert-AIModelList($Response) {
  Assert-ApiOK $Response 'AI model list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI model list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  $secretLeak = $false
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.name) -or [string]::IsNullOrWhiteSpace([string]$item.driver)) {
      throw "AI model item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ((Test-HasProperty $item 'api_key') -or (Test-HasProperty $item 'api_key_enc')) {
      $secretLeak = $true
    }
  }

  if ($secretLeak) {
    throw "AI model list leaked api_key/api_key_enc fields: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
    SecretLeak = $secretLeak
  }
}

function Assert-AIToolInit($Response) {
  Assert-ApiOK $Response 'AI tool init'

  if ($null -eq $Response.data.dict) {
    throw "AI tool init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $executors = Get-ObjectArray $Response.data.dict.ai_executor_type_arr
  $statuses = Get-ObjectArray $Response.data.dict.common_status_arr
  if ($executors.Count -ne 3 -or $statuses.Count -ne 2) {
    throw "AI tool init dict mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ExecutorCount = $executors.Count
    StatusCount = $statuses.Count
  }
}

function Assert-AIToolList($Response) {
  Assert-ApiOK $Response 'AI tool list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI tool list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.name) -or [string]::IsNullOrWhiteSpace([string]$item.code)) {
      throw "AI tool item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-AIToolAgentOptions($Response) {
  Assert-ApiOK $Response 'AI tool agent options'

  if ($null -eq $Response.data.bound_tool_ids -or $null -eq $Response.data.all_tools) {
    throw "AI tool agent options missing fields: $($Response | ConvertTo-Json -Depth 12)"
  }

  $retiredPresent = $false
  foreach ($item in (Get-ObjectArray $Response.data.all_tools)) {
    if ([string]$item.code -eq 'cine_generate_keyframe') {
      $retiredPresent = $true
    }
  }

  if ($retiredPresent) {
    throw "AI tool agent options returned retired cine_generate_keyframe: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    OptionCount = (Get-ObjectArray $Response.data.all_tools).Count
    RetiredCinePresent = $retiredPresent
  }
}

function Assert-AIPromptList($Response) {
  Assert-ApiOK $Response 'AI prompt list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI prompt list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "AI prompt item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if (-not (Test-JsonArray $item.tags)) {
      throw "AI prompt item tags must be an array: $($item | ConvertTo-Json -Depth 12)"
    }
    if (-not (Test-JsonArray $item.variables)) {
      throw "AI prompt item variables must be an array: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
    TagsArrays = $true
    VariablesArrays = $true
  }
}

function Assert-AIPromptDetail($Response) {
  Assert-ApiOK $Response 'AI prompt detail'

  if ([int64]$Response.data.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$Response.data.title)) {
    throw "AI prompt detail shape mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  if (-not (Test-JsonArray $Response.data.tags)) {
    throw "AI prompt detail tags must be an array: $($Response | ConvertTo-Json -Depth 12)"
  }
  if (-not (Test-JsonArray $Response.data.variables)) {
    throw "AI prompt detail variables must be an array: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ID = [int64]$Response.data.id
    TagsArrays = $true
    VariablesArrays = $true
  }
}

function Assert-AIAgentInit($Response) {
  Assert-ApiOK $Response 'AI agent init'

  if ($null -eq $Response.data.dict) {
    throw "AI agent init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $scenes = Get-ObjectArray $Response.data.dict.ai_scene_arr
  $retiredScenePresent = $false
  foreach ($item in $scenes) {
    if (@('goods_script', 'cine_project', 'cine_keyframe') -contains [string]$item.value) {
      $retiredScenePresent = $true
    }
  }
  if ($retiredScenePresent) {
    throw "AI agent init returned retired scene options: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    SceneCount = $scenes.Count
    RetiredScenePresent = $retiredScenePresent
  }
}

function Assert-AIAgentList($Response) {
  Assert-ApiOK $Response 'AI agent list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI agent list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-AIKnowledgeInit($Response) {
  Assert-ApiOK $Response 'AI knowledge init'

  if ($null -eq $Response.data.dict) {
    throw "AI knowledge init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $sources = Get-ObjectArray $Response.data.dict.ai_knowledge_source_type_arr
  $sourceValues = @($sources | ForEach-Object { [string]$_.value })
  foreach ($expected in @('manual', 'text')) {
    if (-not ($sourceValues -contains $expected)) {
      throw "AI knowledge source type missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    SourceTypeCount = $sources.Count
  }
}

function Assert-AIKnowledgeList($Response) {
  Assert-ApiOK $Response 'AI knowledge list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI knowledge list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-AIConversationList($Response) {
  Assert-ApiOK $Response 'AI conversation list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI conversation list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-AIRunInit($Response) {
  Assert-ApiOK $Response 'AI run init'

  if ($null -eq $Response.data.dict) {
    throw "AI run init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }
  $statuses = Get-ObjectArray $Response.data.dict.run_status_arr
  $values = @($statuses | ForEach-Object { [int]$_.value })
  foreach ($expected in @(1, 2, 3, 4)) {
    if (-not ($values -contains $expected)) {
      throw "AI run status dict missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    StatusCount = $statuses.Count
  }
}

function Assert-AIRunList($Response) {
  Assert-ApiOK $Response 'AI run list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "AI run list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-AIRunStats($Response) {
  Assert-ApiOK $Response 'AI run stats'

  if ($null -eq $Response.data.summary) {
    throw "AI run stats missing summary: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    TotalRuns = [int64]$Response.data.summary.total_runs
    FailRuns = [int64]$Response.data.summary.fail_runs
  }
}

function Assert-UserSessionPageInit($Response) {
  Assert-ApiOK $Response 'user session page-init'

  if ($null -eq $Response.data.dict) {
    throw "user session page-init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $platforms = Get-ObjectArray $Response.data.dict.platformArr
  $statuses = Get-ObjectArray $Response.data.dict.statusArr
  if ($platforms.Count -ne 2 -or $statuses.Count -ne 3) {
    throw "user session page-init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  $platformValues = @($platforms | ForEach-Object { [string]$_.value })
  foreach ($expected in @('admin', 'app')) {
    if (-not ($platformValues -contains $expected)) {
      throw "user session page-init platform missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  $statusValues = @($statuses | ForEach-Object { [string]$_.value })
  foreach ($expected in @('active', 'expired', 'revoked')) {
    if (-not ($statusValues -contains $expected)) {
      throw "user session page-init status missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    PlatformCount = $platforms.Count
    StatusCount = $statuses.Count
  }
}

function Assert-UserSessionList($Response) {
  Assert-ApiOK $Response 'user session list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "user session list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [int64]$item.user_id -le 0) {
      throw "user session item missing valid ids: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.platform) -or [string]::IsNullOrWhiteSpace([string]$item.platform_name)) {
      throw "user session item missing platform fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]$item.status -notin @('active', 'expired', 'revoked')) {
      throw "user session item invalid status: $($item | ConvertTo-Json -Depth 12)"
    }
    $fieldNames = @($item.PSObject.Properties.Name)
    foreach ($forbidden in @('access_token_hash', 'refresh_token_hash')) {
      if ($fieldNames -contains $forbidden) {
        throw "user session list leaked forbidden field ${forbidden}: $($item | ConvertTo-Json -Depth 12)"
      }
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-UserSessionStats($Response) {
  Assert-ApiOK $Response 'user session stats'

  if ($null -eq $Response.data.platform_distribution) {
    throw "user session stats missing platform_distribution: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.total_active) {
    throw "user session stats missing total_active: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ([int64]$Response.data.total_active -lt 0) {
    throw "user session stats total_active cannot be negative: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($platform in @('admin', 'app')) {
    if ($null -eq $Response.data.platform_distribution.$platform) {
      throw "user session stats missing platform ${platform}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    TotalActive = [int64]$Response.data.total_active
    Admin = [int64]$Response.data.platform_distribution.admin
    App = [int64]$Response.data.platform_distribution.app
  }
}

function Get-QuickEntryPermissionIDs($Response) {
  $ids = @()
  if ($null -eq $Response -or $null -eq $Response.data) { return $ids }
  foreach ($item in (Get-ObjectArray $Response.data.quick_entry)) {
    if ($null -ne $item.permission_id -and [int64]$item.permission_id -gt 0) {
      $ids += [int64]$item.permission_id
    }
  }
  return $ids
}

function Get-FirstPagePermissionID($Items) {
  foreach ($item in (Get-ObjectArray $Items)) {
    if ($null -eq $item) { continue }
    if ([int]$item.type -eq 2 -and [int64]$item.id -gt 0) {
      return [int64]$item.id
    }
    $childID = Get-FirstPagePermissionID $item.children
    if ($childID -gt 0) {
      return $childID
    }
  }
  return 0
}

function Assert-QuickEntrySave($Response, [int64[]]$ExpectedIDs, [string]$Label) {
  Assert-ApiOK $Response $Label
  if ($null -eq $Response.data.quick_entry) {
    throw "$Label missing quick_entry: $($Response | ConvertTo-Json -Depth 12)"
  }

  $entries = Get-ObjectArray $Response.data.quick_entry
  if ($entries.Count -ne $ExpectedIDs.Count) {
    throw "$Label quick_entry count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  for ($i = 0; $i -lt $ExpectedIDs.Count; $i++) {
    if ([int64]$entries[$i].permission_id -ne [int64]$ExpectedIDs[$i]) {
      throw "$Label quick_entry order mismatch: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    Count = $entries.Count
  }
}

function Resolve-QuickEntryCandidateID($UsersInitResponse) {
  $routeMenuIDs = New-Object System.Collections.Generic.HashSet[int64]
  foreach ($route in (Get-ObjectArray $UsersInitResponse.data.router)) {
    if ($null -eq $route.meta -or $null -eq $route.meta.menuId) { continue }
    [int64]$menuID = 0
    if ([int64]::TryParse([string]$route.meta.menuId, [ref]$menuID) -and $menuID -gt 0) {
      [void]$routeMenuIDs.Add($menuID)
    }
  }

  foreach ($menuID in $routeMenuIDs) {
    return [int64]$menuID
  }
  return Get-FirstPagePermissionID $UsersInitResponse.data.permissions
}

function Invoke-QuickEntryRoundTripProbe([string]$BaseURL, [hashtable]$Headers, $UsersInitResponse) {
  [int64[]]$originalIDs = @(Get-QuickEntryPermissionIDs $UsersInitResponse)
  $candidateID = Resolve-QuickEntryCandidateID $UsersInitResponse
  if ($candidateID -le 0) {
    return [pscustomobject]@{
      Status = 'skipped_no_page_permission'
      SaveCode = -1
      SaveCount = -1
      InitRoundTrip = $true
      RestoreCode = -1
    }
  }

  $restoreCode = -1
  $status = 'passed'
  $saveCode = -1
  $saveCount = -1
  $roundTrip = $false
  try {
    $save = Invoke-JsonRequestAllowFailure 'Put' "$BaseURL/api/admin/v1/users/me/quick-entries" $Headers @{
      permission_ids = @($candidateID)
    }
    $saveSummary = Assert-QuickEntrySave $save @($candidateID) 'users quick-entry save'
    $saveCode = $save.code
    $saveCount = $saveSummary.Count

    $afterInit = Invoke-RestMethod "$BaseURL/api/admin/v1/users/init" `
      -Headers $Headers `
      -TimeoutSec 10
    Assert-ApiOK $afterInit 'users init after quick-entry save'
    $afterIDs = @(Get-QuickEntryPermissionIDs $afterInit)
    $roundTrip = ($afterIDs.Count -eq 1 -and [int64]$afterIDs[0] -eq [int64]$candidateID)
    if (-not $roundTrip) {
      throw "users/init quick_entry did not reflect saved entry: $($afterInit | ConvertTo-Json -Depth 12)"
    }
  } finally {
    try {
      $restore = Invoke-JsonRequestAllowFailure 'Put' "$BaseURL/api/admin/v1/users/me/quick-entries" $Headers @{
        permission_ids = @($originalIDs)
      }
      Assert-ApiOK $restore 'users quick-entry restore'
      $restoreCode = $restore.code
    } catch {
      $status = 'restore_failed'
      Write-Host "Failed to restore users quick-entry for current smoke user"
    }
  }

  if ($status -ne 'passed') {
    throw "users quick-entry probe did not restore original state: $status"
  }

  return [pscustomobject]@{
    Status = $status
    SaveCode = $saveCode
    SaveCount = $saveCount
    InitRoundTrip = $roundTrip
    RestoreCode = $restoreCode
  }
}

function Assert-UserLoginLogPageInit($Response) {
  Assert-ApiOK $Response 'user login log page-init'
  if ($null -eq $Response.data.dict) {
    throw "user login log page-init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }
  $platforms = Get-ObjectArray $Response.data.dict.platformArr
  $loginTypes = Get-ObjectArray $Response.data.dict.login_type_arr
  if ($platforms.Count -lt 2 -or $loginTypes.Count -lt 1) {
    throw "user login log page-init dict mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  return [pscustomobject]@{
    PlatformCount = $platforms.Count
    LoginTypeCount = $loginTypes.Count
  }
}

function Assert-UserLoginLogList($Response) {
  Assert-ApiOK $Response 'user login log list'
  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "user login log list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "user login log item missing id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.login_account)) {
      throw "user login log item missing login_account: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.login_type) -or [string]::IsNullOrWhiteSpace([string]$item.platform)) {
      throw "user login log item missing login_type/platform: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.is_success) {
      throw "user login log item missing is_success: $($item | ConvertTo-Json -Depth 12)"
    }
  }
  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-UserSessionCurrentRevokeBlocked([string]$BaseURL, [hashtable]$Headers, $SessionListResponse) {
  $currentID = 0
  $currentDeviceID = [string]$Headers['device-id']
  foreach ($item in (Get-ObjectArray $SessionListResponse.data.list)) {
    if ([string]$item.device_id -eq $currentDeviceID -and [string]$item.status -eq 'active') {
      $currentID = [int64]$item.id
      break
    }
  }
  if ($currentID -le 0) {
    $wideList = Invoke-RestMethod "$BaseURL/api/admin/v1/user-sessions?current_page=1&page_size=100" `
      -Headers $Headers `
      -TimeoutSec 10
    Assert-ApiOK $wideList 'user session wide list for current revoke probe'
    foreach ($item in (Get-ObjectArray $wideList.data.list)) {
      if ([string]$item.device_id -eq $currentDeviceID -and [string]$item.status -eq 'active') {
        $currentID = [int64]$item.id
        break
      }
    }
    if ($currentID -le 0) {
      throw "current smoke session was not found for anti-kick probe"
    }
  }

  $response = Invoke-JsonRequestAllowFailure 'Patch' "$BaseURL/api/admin/v1/user-sessions/$currentID/revoke" $Headers @{}
  $code = Assert-ApiFailureCode $response 'user session current revoke probe'
  return [pscustomobject]@{
    Status = 'passed'
    CurrentID = $currentID
    Blocked = $true
    Code = $code
  }
}

function Invoke-UploadConfigWriteProbe([string]$BaseURL, [hashtable]$Headers, [string]$Suffix) {
  if ([string]::IsNullOrWhiteSpace($env:VAULT_KEY)) {
    return [pscustomobject]@{
      Status = 'skipped_no_vault_key'
      DriverID = 0
      RuleID = 0
      SettingID = 0
    }
  }

  [int64]$driverID = 0
  [int64]$ruleID = 0
  [int64]$settingID = 0

  try {
    $driverBody = @{
      driver = 'cos'
      secret_id = "codex-secret-id-$Suffix"
      secret_key = "codex-secret-key-$Suffix"
      bucket = "codex-full-smoke-$Suffix"
      region = 'ap-nanjing'
      appid = '1314'
      endpoint = ''
      bucket_domain = ''
      role_arn = ''
    } | ConvertTo-Json -Depth 8

    $driver = Invoke-RestMethod "$BaseURL/api/admin/v1/upload-drivers" `
      -Method Post `
      -Headers $Headers `
      -ContentType 'application/json' `
      -Body $driverBody `
      -TimeoutSec 10
    Assert-ApiOK $driver 'upload driver write probe create'
    $driverID = [int64]$driver.data.id
    if ($driverID -le 0) { throw "upload driver write probe returned invalid id: $($driver | ConvertTo-Json -Depth 12)" }

    $ruleBody = @{
      title = "Codex Full Smoke Upload Rule $Suffix"
      max_size_mb = 1
      image_exts = @('png')
      file_exts = @('pdf')
    } | ConvertTo-Json -Depth 8

    $rule = Invoke-RestMethod "$BaseURL/api/admin/v1/upload-rules" `
      -Method Post `
      -Headers $Headers `
      -ContentType 'application/json' `
      -Body $ruleBody `
      -TimeoutSec 10
    Assert-ApiOK $rule 'upload rule write probe create'
    $ruleID = [int64]$rule.data.id
    if ($ruleID -le 0) { throw "upload rule write probe returned invalid id: $($rule | ConvertTo-Json -Depth 12)" }

    $settingBody = @{
      driver_id = $driverID
      rule_id = $ruleID
      status = 2
      remark = "codex full smoke disabled setting $Suffix"
    } | ConvertTo-Json -Depth 8

    $setting = Invoke-RestMethod "$BaseURL/api/admin/v1/upload-settings" `
      -Method Post `
      -Headers $Headers `
      -ContentType 'application/json' `
      -Body $settingBody `
      -TimeoutSec 10
    Assert-ApiOK $setting 'upload setting write probe create'
    $settingID = [int64]$setting.data.id
    if ($settingID -le 0) { throw "upload setting write probe returned invalid id: $($setting | ConvertTo-Json -Depth 12)" }

    $settingList = Invoke-RestMethod "$BaseURL/api/admin/v1/upload-settings?current_page=1&page_size=20&driver_id=$driverID&rule_id=$ruleID" `
      -Headers $Headers `
      -TimeoutSec 10
    Assert-ApiOK $settingList 'upload setting write probe verify list'
    $matched = $false
    foreach ($item in (Get-ObjectArray $settingList.data.list)) {
      if ([int64]$item.id -eq $settingID -and [int]$item.status -eq 2) {
        $matched = $true
      }
    }
    if (-not $matched) {
      throw "upload setting write probe row not found as disabled: $($settingList | ConvertTo-Json -Depth 12)"
    }

    return [pscustomobject]@{
      Status = 'ok'
      DriverID = $driverID
      RuleID = $ruleID
      SettingID = $settingID
    }
  } finally {
    if ($settingID -gt 0) {
      try {
        Invoke-RestMethod "$BaseURL/api/admin/v1/upload-settings/$settingID" -Method Delete -Headers $Headers -TimeoutSec 5 | Out-Null
        $settingID = 0
      } catch {
        Write-Host "Failed to cleanup upload setting id=$settingID"
      }
    }
    if ($ruleID -gt 0) {
      try {
        Invoke-RestMethod "$BaseURL/api/admin/v1/upload-rules/$ruleID" -Method Delete -Headers $Headers -TimeoutSec 5 | Out-Null
        $ruleID = 0
      } catch {
        Write-Host "Failed to cleanup upload rule id=$ruleID"
      }
    }
    if ($driverID -gt 0) {
      try {
        Invoke-RestMethod "$BaseURL/api/admin/v1/upload-drivers/$driverID" -Method Delete -Headers $Headers -TimeoutSec 5 | Out-Null
        $driverID = 0
      } catch {
        Write-Host "Failed to cleanup upload driver id=$driverID"
      }
    }
  }
}

function Invoke-UploadTokenProbe([string]$BaseURL, [hashtable]$Headers) {
  if ($env:COS_STS_ENABLED -ne 'true') {
    return [pscustomobject]@{
      Status = 'skipped_cos_sts_disabled'
      Code = $null
      Provider = $null
      Key = $null
    }
  }

  $body = @{
    folder = 'images'
    file_name = 'smoke.png'
    file_size = 1024
    file_kind = 'image'
  } | ConvertTo-Json -Depth 4

  $response = Invoke-RestMethod "$BaseURL/api/admin/v1/upload-tokens" `
    -Method Post `
    -Headers $Headers `
    -ContentType 'application/json' `
    -Body $body `
    -TimeoutSec 15

  Assert-ApiOK $response 'upload token probe'
  if ($response.data.provider -ne 'cos') {
    throw "upload token provider mismatch: $($response | ConvertTo-Json -Depth 12)"
  }
  if (-not ([string]$response.data.key).StartsWith('images/')) {
    throw "upload token key mismatch: $($response | ConvertTo-Json -Depth 12)"
  }
  if ([string]::IsNullOrWhiteSpace([string]$response.data.credentials.tmp_secret_id) `
    -or [string]::IsNullOrWhiteSpace([string]$response.data.credentials.tmp_secret_key) `
    -or [string]::IsNullOrWhiteSpace([string]$response.data.credentials.session_token)) {
    throw "upload token credentials shape mismatch: $($response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    Status = 'passed'
    Code = $response.code
    Provider = $response.data.provider
    Key = $response.data.key
  }
}

function Assert-ProfilePayload($Response, [string]$Label) {
  Assert-ApiOK $Response $Label

  if ($null -eq $Response.data.profile) {
    throw "$Label missing profile: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.dict) {
    throw "$Label missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ([int64]$Response.data.profile.user_id -le 0) {
    throw "$Label profile missing user_id: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.profile.address_id) {
    throw "$Label profile missing address_id: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -ne $Response.data.profile.address) {
    throw "$Label leaked legacy address alias: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ((Get-ObjectArray $Response.data.dict.sexArr).Count -ne 3) {
    throw "$Label sex dict mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  $verifyTypes = @(Get-ObjectArray $Response.data.dict.verify_type_arr | ForEach-Object { [string]$_.value })
  foreach ($expected in @('password', 'code')) {
    if (-not ($verifyTypes -contains $expected)) {
      throw "$Label verify type dict missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }
}

function Assert-ProfileUpdateOperationLog([string]$BaseURL, [hashtable]$Headers, [int64]$AfterID) {
  $createdLog = Wait-NewOperationLog $BaseURL $Headers '编辑个人资料' $AfterID
  $requestData = $createdLog.request_data | ConvertFrom-Json
  if ($requestData.module -ne 'profile' -or $requestData.action -ne 'update_profile') {
    throw "profile operation log metadata mismatch: $($createdLog.request_data)"
  }
  return [pscustomobject]@{
    ID = [int64]$createdLog.id
    Module = [string]$requestData.module
    Action = [string]$requestData.action
  }
}

function Assert-AccountSecurityFailureProbe([string]$BaseURL, [hashtable]$Headers) {
  $wrongPassword = Invoke-JsonRequestAllowFailure 'Put' "$BaseURL/api/admin/v1/profile/security/password" $Headers @{
    verify_type = 'password'
    old_password = 'codex-wrong-old-password'
    new_password = 'codex-smoke-new-password'
    confirm_password = 'codex-smoke-new-password'
  }
  $wrongPasswordCode = Assert-ApiFailureCode $wrongPassword 'account security wrong old password probe'

  $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  $invalidEmail = Invoke-JsonRequestAllowFailure 'Put' "$BaseURL/api/admin/v1/profile/security/email" $Headers @{
    email = "codex-invalid-$suffix@example.com"
    code = '000000'
  }
  $invalidEmailCode = Assert-ApiFailureCode $invalidEmail 'account security invalid email code probe'

  $invalidPhone = Invoke-JsonRequestAllowFailure 'Put' "$BaseURL/api/admin/v1/profile/security/phone" $Headers @{
    phone = '15671628272'
    code = '000000'
  }
  $invalidPhoneCode = Assert-ApiFailureCode $invalidPhone 'account security invalid phone code probe'

  return [pscustomobject]@{
    WrongOldPasswordCode = $wrongPasswordCode
    InvalidEmailCode = $invalidEmailCode
    InvalidPhoneCode = $invalidPhoneCode
  }
}

function Assert-NotificationInit($Response) {
  Assert-ApiOK $Response 'notification init'

  if ($null -eq $Response.data.dict) {
    throw "notification init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $types = Get-ObjectArray $Response.data.dict.notification_type_arr
  $levels = Get-ObjectArray $Response.data.dict.notification_level_arr
  $readStatuses = Get-ObjectArray $Response.data.dict.notification_read_status_arr
  if ($types.Count -ne 4 -or $levels.Count -ne 2 -or $readStatuses.Count -ne 2) {
    throw "notification init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    TypeCount = $types.Count
    LevelCount = $levels.Count
    ReadStatusCount = $readStatuses.Count
  }
}

function Assert-NotificationList($Response) {
  Assert-ApiOK $Response 'notification list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "notification list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "notification item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "notification item missing title: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.type -or [string]::IsNullOrWhiteSpace([string]$item.type_text)) {
      throw "notification item missing type fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.level -or [string]::IsNullOrWhiteSpace([string]$item.level_text)) {
      throw "notification item missing level fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.is_read) {
      throw "notification item missing is_read: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-NotificationUnreadCount($Response) {
  Assert-ApiOK $Response 'notification unread-count'

  if ($null -eq $Response.data.count -or [int64]$Response.data.count -lt 0) {
    throw "notification unread-count shape mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [int64]$Response.data.count
}

function Assert-NotificationTaskInit($Response) {
  Assert-ApiOK $Response 'notification task init'

  if ($null -eq $Response.data.dict) {
    throw "notification task init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $types = Get-ObjectArray $Response.data.dict.notification_type_arr
  $levels = Get-ObjectArray $Response.data.dict.notification_level_arr
  $targets = Get-ObjectArray $Response.data.dict.notification_target_type_arr
  $statuses = Get-ObjectArray $Response.data.dict.notification_task_status_arr
  $platforms = Get-ObjectArray $Response.data.dict.platformArr
  if ($types.Count -ne 4 -or $levels.Count -ne 2 -or $targets.Count -ne 3 -or $statuses.Count -ne 4 -or $platforms.Count -lt 3) {
    throw "notification task init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ([string]$platforms[0].value -ne 'all') {
    throw "notification task platform all must be first: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    TypeCount = $types.Count
    LevelCount = $levels.Count
    TargetTypeCount = $targets.Count
    StatusCount = $statuses.Count
    PlatformCount = $platforms.Count
  }
}

function Assert-NotificationTaskStatusCount($Response) {
  Assert-ApiOK $Response 'notification task status-count'

  $items = Get-ObjectArray $Response.data
  if ($items.Count -ne 4) {
    throw "notification task status-count count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in $items) {
    if ($null -eq $item.value -or [string]::IsNullOrWhiteSpace([string]$item.label) -or $null -eq $item.num) {
      throw "notification task status-count item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([int64]$item.num -lt 0) {
      throw "notification task status-count num cannot be negative: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return $items.Count
}

function Assert-NotificationTaskList($Response) {
  Assert-ApiOK $Response 'notification task list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "notification task list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "notification task item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "notification task item missing title: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.status -or [string]::IsNullOrWhiteSpace([string]$item.status_text)) {
      throw "notification task item missing status fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.target_type -or [string]::IsNullOrWhiteSpace([string]$item.target_type_text)) {
      throw "notification task item missing target fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.total_count -or $null -eq $item.sent_count) {
      throw "notification task item missing progress fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-CronTaskInit($Response) {
  Assert-ApiOK $Response 'cron task init'

  if ($null -eq $Response.data.dict) {
    throw "cron task init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $presets = Get-ObjectArray $Response.data.dict.cron_preset_arr
  $statuses = Get-ObjectArray $Response.data.dict.cron_task_status_arr
  $registryStatuses = Get-ObjectArray $Response.data.dict.cron_task_registry_status_arr
  $logStatuses = Get-ObjectArray $Response.data.dict.cron_task_log_status_arr
  if ($presets.Count -le 0 -or $statuses.Count -ne 2 -or $registryStatuses.Count -ne 4 -or $logStatuses.Count -ne 3) {
    throw "cron task init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    PresetCount = $presets.Count
    StatusCount = $statuses.Count
    RegistryStatusCount = $registryStatuses.Count
    LogStatusCount = $logStatuses.Count
  }
}

function Assert-CronTaskList($Response) {
  Assert-ApiOK $Response 'cron task list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "cron task list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  $registeredNotification = $false
  $registeredPaymentCloseExpired = $false
  $registeredPaymentSyncPending = $false
  $registeredAIRunTimeout = $false
  $aiRunTimeoutTaskType = ''
  $missingLegacy = $false
  $firstID = 0
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "cron task item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.name) -or [string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "cron task item missing name/title: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.registry_status) -or [string]::IsNullOrWhiteSpace([string]$item.registry_status_text)) {
      throw "cron task item missing registry status fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($firstID -eq 0) { $firstID = [int64]$item.id }
    if ([string]$item.name -eq 'notification_task_scheduler' -and [string]$item.registry_status -eq 'registered') {
      if ([string]$item.registry_task_type -ne 'notification:dispatch-due:v1' -or [string]$item.handler -ne 'notification:dispatch-due:v1') {
        throw "notification cron task must expose Go task type instead of legacy PHP handler: $($item | ConvertTo-Json -Depth 12)"
      }
      $registeredNotification = $true
    }
    if ([string]$item.name -eq 'payment_close_expired_order' -and [string]$item.registry_status -eq 'registered') {
      if ([string]$item.registry_task_type -ne 'payment:close-expired-order:v1' -or [string]$item.handler -ne 'payment:close-expired-order:v1') {
        throw "payment close-expired cron task must expose payment task type instead of legacy pay handler: $($item | ConvertTo-Json -Depth 12)"
      }
      $registeredPaymentCloseExpired = $true
    }
    if ([string]$item.name -eq 'payment_sync_pending_order' -and [string]$item.registry_status -eq 'registered') {
      if ([string]$item.registry_task_type -ne 'payment:sync-pending-order:v1' -or [string]$item.handler -ne 'payment:sync-pending-order:v1') {
        throw "payment sync-pending cron task must expose payment task type instead of legacy pay handler: $($item | ConvertTo-Json -Depth 12)"
      }
      $registeredPaymentSyncPending = $true
    }
    if ([string]$item.name -eq 'ai_run_timeout' -and [string]$item.registry_status -eq 'registered') {
      if ([string]$item.registry_task_type -ne 'ai:run-timeout:v1' -or [string]$item.handler -ne 'ai:run-timeout:v1') {
        throw "AI run timeout cron task must expose Go task type instead of legacy PHP handler: $($item | ConvertTo-Json -Depth 12)"
      }
      $registeredAIRunTimeout = $true
      $aiRunTimeoutTaskType = [string]$item.registry_task_type
    }
    if ([string]$item.registry_status -eq 'missing') {
      $missingLegacy = $true
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
    NotificationRegistered = $registeredNotification
    PaymentCloseExpiredRegistered = $registeredPaymentCloseExpired
    PaymentSyncPendingRegistered = $registeredPaymentSyncPending
    AIRunTimeoutRegistered = $registeredAIRunTimeout
    AIRunTimeoutTaskType = $aiRunTimeoutTaskType
    MissingLegacyPresent = $missingLegacy
    FirstID = $firstID
  }
}

function Assert-CronTaskLogs($Response) {
  Assert-ApiOK $Response 'cron task logs'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "cron task logs missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "cron task log item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.status -or [string]::IsNullOrWhiteSpace([string]$item.status_name)) {
      throw "cron task log item missing status fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}


function Assert-ClientVersionInit($Response) {
  Assert-ApiOK $Response 'client version page-init'

  if ($null -eq $Response.data.dict) {
    throw "client version page-init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $platforms = Get-ObjectArray $Response.data.dict.client_version_platform_arr
  $yesNo = Get-ObjectArray $Response.data.dict.common_yes_no_arr
  if ($platforms.Count -lt 2 -or $yesNo.Count -ne 2) {
    throw "client version page-init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  $platformValues = @($platforms | ForEach-Object { [string]$_.value })
  foreach ($expected in @('windows-x86_64', 'darwin-x86_64')) {
    if (-not ($platformValues -contains $expected)) {
      throw "client version platform dict missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  $yesNoValues = @($yesNo | ForEach-Object { [int]$_.value })
  foreach ($expected in @(1, 2)) {
    if (-not ($yesNoValues -contains $expected)) {
      throw "client version yes/no dict missing ${expected}: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    PlatformCount = $platforms.Count
    YesNoCount = $yesNo.Count
  }
}

function Assert-ClientVersionList($Response) {
  Assert-ApiOK $Response 'client version list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "client version list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0) {
      throw "client version item missing valid id: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.version)) {
      throw "client version item missing version: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]$item.platform -ne 'windows-x86_64' -and [string]$item.platform -ne 'darwin-x86_64') {
      throw "client version item invalid platform: $($item | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$item.platform_name)) {
      throw "client version item missing platform_name: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.is_latest -or $null -eq $item.force_update) {
      throw "client version item missing state fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-ClientVersionUpdateJSON($Response) {
  Assert-ApiOK $Response 'client version update-json'

  $data = $Response.data
  if ($null -eq $data) {
    throw "client version update-json missing data: $($Response | ConvertTo-Json -Depth 12)"
  }

  $items = Get-ObjectArray $data
  if ($items.Count -eq 0) {
    return [pscustomobject]@{
      Shape = 'empty'
      PlatformCount = 0
      Version = ''
    }
  }

  if ([string]::IsNullOrWhiteSpace([string]$data.version)) {
    throw "client version update-json missing version: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $data.platforms) {
    throw "client version update-json missing platforms: $($Response | ConvertTo-Json -Depth 12)"
  }

  $platformCount = 0
  foreach ($property in $data.platforms.PSObject.Properties) {
    $platformCount++
    if ([string]::IsNullOrWhiteSpace([string]$property.Value.url) -or [string]::IsNullOrWhiteSpace([string]$property.Value.signature)) {
      throw "client version update-json platform payload mismatch: $($Response | ConvertTo-Json -Depth 12)"
    }
  }
  if ($platformCount -le 0) {
    throw "client version update-json platforms empty: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    Shape = 'manifest'
    PlatformCount = $platformCount
    Version = [string]$data.version
  }
}

function Invoke-BasicSmoke() {
  $basicOutput = & powershell -ExecutionPolicy Bypass -File .\scripts\basic-admin-smoke.ps1 `
    -Account $Account `
    -Password $Password `
    -HTTPAddr $BasicHTTPAddr `
    -Platform $Platform `
    -DeviceID "$DeviceID-basic" 2>&1

  if ($LASTEXITCODE -ne 0) {
    throw "basic smoke failed: $($basicOutput | Out-String)"
  }

  $text = ($basicOutput | Out-String).Trim()
  if ([string]::IsNullOrWhiteSpace($text)) {
    throw 'basic smoke returned empty output'
  }

  return $text | ConvertFrom-Json
}

function New-SmokePermission([string]$BaseURL, [hashtable]$Headers, [string]$Suffix) {
  $body = @{
    platform = $Platform
    type = 1
    name = "Codex Full Smoke OperationLog DIR $Suffix"
    parent_id = 0
    icon = ''
    path = ''
    component = ''
    i18n_key = "menu.codex_full_smoke_operation_log_$Suffix"
    code = ''
    sort = 999
    show_menu = 2
  } | ConvertTo-Json -Depth 8

  $response = Invoke-RestMethod "$BaseURL/api/admin/v1/permissions" `
    -Method Post `
    -Headers $Headers `
    -ContentType 'application/json' `
    -Body $body `
    -TimeoutSec 10

  Assert-ApiOK $response 'operation log smoke permission create'
  if ($response.data.id -le 0) {
    throw "permission create returned invalid id: $($response | ConvertTo-Json -Depth 12)"
  }
  return [int64]$response.data.id
}

Import-DotEnv (Join-Path $BackendRoot '.env')

if ([string]::IsNullOrWhiteSpace($Account) -or [string]::IsNullOrWhiteSpace($Password)) {
  throw 'Set SMOKE_LOGIN_ACCOUNT and SMOKE_LOGIN_PASSWORD, or pass -Account and -Password.'
}

Assert-PortFree $BasicHTTPAddr
Assert-PortFree $HTTPAddr

New-Item -ItemType Directory -Force .tmp | Out-Null

$serverExe = '.tmp/admin-api-full-smoke.exe'
$secretReader = '.tmp/read-full-smoke-captcha-secret.go'
$outLog = '.tmp/full-admin-smoke-out.log'
$errLog = '.tmp/full-admin-smoke-err.log'
$completed = $false
$baseURL = $null
$proc = $null
$authHeaders = $null
$createdPermissionID = 0
$operationLogRowID = 0

Remove-Item -Force $serverExe, $secretReader, $outLog, $errLog -ErrorAction SilentlyContinue

try {
  $basicSummary = Invoke-BasicSmoke

  go build -o $serverExe ./cmd/admin-api

  $env:HTTP_ADDR = $HTTPAddr
  $proc = Start-Process -FilePath (Resolve-Path $serverExe) `
    -PassThru `
    -WindowStyle Hidden `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog

  $baseURL = "http://$HTTPAddr"
  Wait-Health $baseURL

  $captcha = Invoke-RestMethod "$baseURL/api/admin/v1/auth/captcha" -TimeoutSec 10
  Assert-ApiOK $captcha 'full smoke captcha'

  @"
package main

import (
  "context"
  "fmt"
  "os"
  "strconv"

  "github.com/redis/go-redis/v9"
)

func main() {
  if len(os.Args) != 2 {
    fmt.Fprintln(os.Stderr, "usage: read-full-smoke-captcha-secret <captcha-id>")
    os.Exit(2)
  }

  db, err := strconv.Atoi(os.Getenv("REDIS_DB"))
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(2)
  }

  client := redis.NewClient(&redis.Options{
    Addr:     os.Getenv("REDIS_ADDR"),
    Password: os.Getenv("REDIS_PASSWORD"),
    DB:       db,
  })
  defer client.Close()

  prefix := os.Getenv("CAPTCHA_REDIS_PREFIX")
  if prefix == "" {
    prefix = "captcha:slide:"
  }

  value, err := client.Get(context.Background(), prefix+os.Args[1]).Result()
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }

  fmt.Print(value)
}
"@ | Set-Content -LiteralPath $secretReader -Encoding UTF8

  $env:REDIS_ADDR = Get-RedisAddr
  $env:REDIS_DB = Get-RedisDB
  if ([string]::IsNullOrWhiteSpace($env:CAPTCHA_REDIS_PREFIX)) {
    $env:CAPTCHA_REDIS_PREFIX = 'captcha:slide:'
  }

  $secretJson = go run $secretReader $captcha.data.captcha_id
  $secret = $secretJson | ConvertFrom-Json

  $loginBody = @{
    login_account = $Account
    login_type = 'password'
    password = $Password
    captcha_id = $captcha.data.captcha_id
    captcha_answer = @{
      x = [int]$secret.answer.x
      y = [int]$secret.answer.y
    }
  } | ConvertTo-Json -Depth 8

  $login = Invoke-RestMethod "$baseURL/api/admin/v1/auth/login" `
    -Method Post `
    -Headers @{ platform = $Platform; 'device-id' = $DeviceID } `
    -ContentType 'application/json' `
    -Body $loginBody `
    -TimeoutSec 10

  Assert-ApiOK $login 'full smoke login'
  if ([string]::IsNullOrWhiteSpace($login.data.access_token)) {
    throw "full smoke login returned empty access token"
  }

  $authHeaders = @{
    platform = $Platform
    'device-id' = $DeviceID
    Authorization = "Bearer $($login.data.access_token)"
  }

  $me = Invoke-RestMethod "$baseURL/api/admin/v1/users/me" -Headers $authHeaders -TimeoutSec 10
  Assert-ApiOK $me 'full smoke users me'
  Clear-UserButtonCache ([int64]$me.data.user_id) $Platform

  $usersInit = Invoke-RestMethod "$baseURL/api/admin/v1/users/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $usersInitAIRouteSummary = Assert-UsersInitAIRoutes $usersInit
  $usersInitPaymentRouteSummary = Assert-UsersInitPaymentRoutes $usersInit

  $quickEntryProbe = Invoke-QuickEntryRoundTripProbe $baseURL $authHeaders $usersInit

  $userLoginLogPageInit = Invoke-RestMethod "$baseURL/api/admin/v1/users/login-logs/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $userLoginLogPageInitSummary = Assert-UserLoginLogPageInit $userLoginLogPageInit

  $userLoginLogList = Invoke-RestMethod "$baseURL/api/admin/v1/users/login-logs?current_page=1&page_size=10&login_account=$([uri]::EscapeDataString($Account))" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $userLoginLogListSummary = Assert-UserLoginLogList $userLoginLogList

  $userSessionPageInit = Invoke-RestMethod "$baseURL/api/admin/v1/user-sessions/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $userSessionPageInitSummary = Assert-UserSessionPageInit $userSessionPageInit

  $userSessionList = Invoke-RestMethod "$baseURL/api/admin/v1/user-sessions?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $userSessionListSummary = Assert-UserSessionList $userSessionList

  $userSessionStats = Invoke-RestMethod "$baseURL/api/admin/v1/user-sessions/stats" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $userSessionStatsSummary = Assert-UserSessionStats $userSessionStats
  $userSessionCurrentRevokeProbe = Assert-UserSessionCurrentRevokeBlocked $baseURL $authHeaders $userSessionList

  $queueMonitorList = Invoke-RestMethod "$baseURL/api/admin/v1/queue-monitor" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $queueMonitorQueueCount = Assert-QueueMonitorList $queueMonitorList

  $queueMonitorFailed = Invoke-RestMethod "$baseURL/api/admin/v1/queue-monitor/failed?queue=critical&current_page=1&page_size=5" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $queueMonitorFailedTotal = Assert-QueueFailedList $queueMonitorFailed

  $queueMonitorUI = Invoke-WebRequest "$baseURL/api/admin/v1/queue-monitor-ui" `
    -Method Head `
    -Headers $authHeaders `
    -TimeoutSec 10
  if ($queueMonitorUI.StatusCode -ne 200) {
    throw "queue monitor UI HEAD returned status $($queueMonitorUI.StatusCode)"
  }

  $systemLogInit = Invoke-RestMethod "$baseURL/api/admin/v1/system-logs/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $systemLogInitSummary = Assert-SystemLogInit $systemLogInit

  $systemLogFiles = Invoke-RestMethod "$baseURL/api/admin/v1/system-logs/files" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $systemLogFilesSummary = Assert-SystemLogFiles $systemLogFiles
  $systemLogLinesProbe = Invoke-SystemLogLinesProbe $baseURL $authHeaders $systemLogFilesSummary.FirstName

  $systemSettingInit = Invoke-RestMethod "$baseURL/api/admin/v1/system-settings/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $systemSettingValueTypeCount = Assert-SystemSettingInit $systemSettingInit

  $systemSettingList = Invoke-RestMethod "$baseURL/api/admin/v1/system-settings?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $systemSettingListSummary = Assert-SystemSettingList $systemSettingList

  $clientVersionInit = Invoke-RestMethod "$baseURL/api/admin/v1/client-versions/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $clientVersionInitSummary = Assert-ClientVersionInit $clientVersionInit

  $clientVersionList = Invoke-RestMethod "$baseURL/api/admin/v1/client-versions?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $clientVersionListSummary = Assert-ClientVersionList $clientVersionList

  $clientVersionUpdateJson = Invoke-RestMethod "$baseURL/api/admin/v1/client-versions/update-json?platform=windows-x86_64" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $clientVersionUpdateJsonSummary = Assert-ClientVersionUpdateJSON $clientVersionUpdateJson

  $uploadDriverInit = Invoke-RestMethod "$baseURL/api/admin/v1/upload-drivers/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $uploadDriverDictCount = Assert-UploadDriverInit $uploadDriverInit

  $uploadDriverList = Invoke-RestMethod "$baseURL/api/admin/v1/upload-drivers?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $uploadDriverListSummary = Assert-UploadDriverList $uploadDriverList

  $uploadRuleInit = Invoke-RestMethod "$baseURL/api/admin/v1/upload-rules/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $uploadRuleDictSummary = Assert-UploadRuleInit $uploadRuleInit

  $uploadRuleList = Invoke-RestMethod "$baseURL/api/admin/v1/upload-rules?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $uploadRuleListSummary = Assert-UploadRuleList $uploadRuleList

  $uploadSettingInit = Invoke-RestMethod "$baseURL/api/admin/v1/upload-settings/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $uploadSettingDictSummary = Assert-UploadSettingInit $uploadSettingInit

  $uploadSettingList = Invoke-RestMethod "$baseURL/api/admin/v1/upload-settings?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $uploadSettingListSummary = Assert-UploadSettingList $uploadSettingList

  $paymentChannelInit = Invoke-RestMethod "$baseURL/api/admin/v1/payment/channels/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $paymentChannelInitSummary = Assert-PaymentChannelInit $paymentChannelInit

  $paymentChannelList = Invoke-RestMethod "$baseURL/api/admin/v1/payment/channels?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $paymentChannelListSummary = Assert-PaymentChannelList $paymentChannelList

  $aiModelInit = Invoke-RestMethod "$baseURL/api/admin/v1/ai-models/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiModelInitSummary = Assert-AIModelInit $aiModelInit

  $aiModelList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-models?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiModelListSummary = Assert-AIModelList $aiModelList

  $aiToolInit = Invoke-RestMethod "$baseURL/api/admin/v1/ai-tools/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiToolInitSummary = Assert-AIToolInit $aiToolInit

  $aiToolList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-tools?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiToolListSummary = Assert-AIToolList $aiToolList

  $aiToolAgentOptions = Invoke-RestMethod "$baseURL/api/admin/v1/ai-tools/agent-options" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiToolAgentOptionsSummary = Assert-AIToolAgentOptions $aiToolAgentOptions

  $aiPromptList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-prompts?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiPromptListSummary = Assert-AIPromptList $aiPromptList
  $aiPromptDetailCode = $null
  $aiPromptDetailID = 0
  $aiPromptDetailSummary = $null
  $aiPromptRows = Get-ObjectArray $aiPromptList.data.list
  if ($aiPromptRows.Count -gt 0) {
    $firstAiPrompt = $aiPromptRows[0]
    $aiPromptDetail = Invoke-RestMethod "$baseURL/api/admin/v1/ai-prompts/$($firstAiPrompt.id)" `
      -Headers $authHeaders `
      -TimeoutSec 10
    $aiPromptDetailSummary = Assert-AIPromptDetail $aiPromptDetail
    $aiPromptDetailCode = $aiPromptDetail.code
    $aiPromptDetailID = $aiPromptDetailSummary.ID
  }

  $aiAgentInit = Invoke-RestMethod "$baseURL/api/admin/v1/ai-agents/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiAgentInitSummary = Assert-AIAgentInit $aiAgentInit

  $aiAgentList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-agents?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiAgentListSummary = Assert-AIAgentList $aiAgentList

  $aiKnowledgeInit = Invoke-RestMethod "$baseURL/api/admin/v1/ai-knowledge-bases/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiKnowledgeInitSummary = Assert-AIKnowledgeInit $aiKnowledgeInit

  $aiKnowledgeList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-knowledge-bases?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiKnowledgeListSummary = Assert-AIKnowledgeList $aiKnowledgeList

  $aiConversationList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-conversations?current_page=1&page_size=5" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiConversationListSummary = Assert-AIConversationList $aiConversationList

  $aiRunInit = Invoke-RestMethod "$baseURL/api/admin/v1/ai-runs/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiRunInitSummary = Assert-AIRunInit $aiRunInit

  $aiRunList = Invoke-RestMethod "$baseURL/api/admin/v1/ai-runs?current_page=1&page_size=5" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiRunListSummary = Assert-AIRunList $aiRunList

  $aiRunStats = Invoke-RestMethod "$baseURL/api/admin/v1/ai-runs/stats" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $aiRunStatsSummary = Assert-AIRunStats $aiRunStats


  $paymentOrderInit = Invoke-RestMethod "$baseURL/api/admin/v1/payment/orders/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $paymentOrderInitSummary = Assert-PaymentOrderInit $paymentOrderInit

  $paymentOrderList = Invoke-RestMethod "$baseURL/api/admin/v1/payment/orders?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $paymentOrderListSummary = Assert-PaymentOrderList $paymentOrderList

  $paymentEventList = Invoke-RestMethod "$baseURL/api/admin/v1/payment/events?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $paymentEventListSummary = Assert-PaymentEventList $paymentEventList

  $uploadWriteProbe = Invoke-UploadConfigWriteProbe $baseURL $authHeaders ([string][DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
  $uploadTokenProbe = Invoke-UploadTokenProbe $baseURL $authHeaders

  $profile = Invoke-RestMethod "$baseURL/api/admin/v1/profile" `
    -Headers $authHeaders `
    -TimeoutSec 10
  Assert-ProfilePayload $profile 'profile read'
  $profileUserID = [int64]$profile.data.profile.user_id

  $targetProfile = Invoke-RestMethod "$baseURL/api/admin/v1/users/$profileUserID/profile" `
    -Headers $authHeaders `
    -TimeoutSec 10
  Assert-ProfilePayload $targetProfile 'target profile read'

  $profileBeforeLogs = Get-OperationLogList $baseURL $authHeaders '编辑个人资料'
  Assert-ApiOK $profileBeforeLogs 'profile operation log before list'
  $profileBeforeMaxID = Get-MaxOperationLogID $profileBeforeLogs

  $profileUpdateBody = @{
    username = [string]$profile.data.profile.username
    avatar = [string]$profile.data.profile.avatar
    sex = [int]$profile.data.profile.sex
    birthday = if ([string]::IsNullOrWhiteSpace([string]$profile.data.profile.birthday)) { $null } else { [string]$profile.data.profile.birthday }
    address_id = [int64]$profile.data.profile.address_id
    detail_address = [string]$profile.data.profile.detail_address
    bio = [string]$profile.data.profile.bio
  } | ConvertTo-Json -Depth 8

  $profileUpdate = Invoke-RestMethod "$baseURL/api/admin/v1/profile" `
    -Method Put `
    -Headers $authHeaders `
    -ContentType 'application/json' `
    -Body $profileUpdateBody `
    -TimeoutSec 10
  Assert-ApiOK $profileUpdate 'profile safe self update'
  $profileOperationLog = Assert-ProfileUpdateOperationLog $baseURL $authHeaders $profileBeforeMaxID

  $accountSecurityProbe = Assert-AccountSecurityFailureProbe $baseURL $authHeaders

  $notificationInit = Invoke-RestMethod "$baseURL/api/admin/v1/notifications/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $notificationInitSummary = Assert-NotificationInit $notificationInit

  $notificationList = Invoke-RestMethod "$baseURL/api/admin/v1/notifications?current_page=1&page_size=5" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $notificationListSummary = Assert-NotificationList $notificationList

  $notificationUnreadCount = Invoke-RestMethod "$baseURL/api/admin/v1/notifications/unread-count" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $notificationUnreadTotal = Assert-NotificationUnreadCount $notificationUnreadCount

  $notificationTaskInit = Invoke-RestMethod "$baseURL/api/admin/v1/notification-tasks/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $notificationTaskInitSummary = Assert-NotificationTaskInit $notificationTaskInit

  $notificationTaskStatusCount = Invoke-RestMethod "$baseURL/api/admin/v1/notification-tasks/status-count" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $notificationTaskStatusCountTotal = Assert-NotificationTaskStatusCount $notificationTaskStatusCount

  $notificationTaskList = Invoke-RestMethod "$baseURL/api/admin/v1/notification-tasks?current_page=1&page_size=5" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $notificationTaskListSummary = Assert-NotificationTaskList $notificationTaskList

  $cronTaskInit = Invoke-RestMethod "$baseURL/api/admin/v1/cron-tasks/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $cronTaskInitSummary = Assert-CronTaskInit $cronTaskInit

  $cronTaskList = Invoke-RestMethod "$baseURL/api/admin/v1/cron-tasks?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $cronTaskListSummary = Assert-CronTaskList $cronTaskList

  $cronTaskLogsCode = $null
  $cronTaskLogsSummary = [pscustomobject]@{ ListCount = 0; Total = 0 }
  if ($cronTaskListSummary.FirstID -gt 0) {
    $cronTaskLogs = Invoke-RestMethod "$baseURL/api/admin/v1/cron-tasks/$($cronTaskListSummary.FirstID)/logs?current_page=1&page_size=5" `
      -Headers $authHeaders `
      -TimeoutSec 10
    $cronTaskLogsCode = $cronTaskLogs.code
    $cronTaskLogsSummary = Assert-CronTaskLogs $cronTaskLogs
  }

  $operationLogInit = Invoke-RestMethod "$baseURL/api/admin/v1/operation-logs/init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  Assert-ApiOK $operationLogInit 'operation log init'

  $beforeLogs = Get-OperationLogList $baseURL $authHeaders '新增权限'
  Assert-ApiOK $beforeLogs 'operation log before list'
  $beforeMaxID = Get-MaxOperationLogID $beforeLogs

  $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
  $createdPermissionID = New-SmokePermission $baseURL $authHeaders ([string]$suffix)

  $createdLog = Wait-NewOperationLog $baseURL $authHeaders '新增权限' $beforeMaxID
  $operationLogRowID = [int64]$createdLog.id
  $requestData = $createdLog.request_data | ConvertFrom-Json
  if ($requestData.module -ne 'permission' -or $requestData.action -ne 'create') {
    throw "operation log request_data metadata mismatch: $($createdLog.request_data)"
  }

  $deleteOperationLog = Invoke-RestMethod "$baseURL/api/admin/v1/operation-logs/$operationLogRowID" `
    -Method Delete `
    -Headers $authHeaders `
    -TimeoutSec 10
  Assert-ApiOK $deleteOperationLog 'operation log delete'

  $afterDeleteLogs = Get-OperationLogList $baseURL $authHeaders '新增权限'
  Assert-ApiOK $afterDeleteLogs 'operation log after delete list'
  $deletedAbsent = $true
  foreach ($item in (Get-ObjectArray $afterDeleteLogs.data.list)) {
    if ([int64]$item.id -eq $operationLogRowID) {
      $deletedAbsent = $false
    }
  }
  if (-not $deletedAbsent) {
    throw "operation log row id=$operationLogRowID still appears after delete"
  }

  $permissionDelete = Invoke-RestMethod "$baseURL/api/admin/v1/permissions/$createdPermissionID" `
    -Method Delete `
    -Headers $authHeaders `
    -TimeoutSec 10
  Assert-ApiOK $permissionDelete 'operation log smoke permission cleanup'
  $createdPermissionID = 0

  $logout = Invoke-RestMethod "$baseURL/api/admin/v1/auth/logout" -Method Post -Headers $authHeaders -TimeoutSec 10
  Assert-ApiOK $logout 'full smoke logout'

  $summary = [ordered]@{
    basic = $basicSummary
    users_quick_entry_save_status = $quickEntryProbe.Status
    users_quick_entry_save_code = $quickEntryProbe.SaveCode
    users_quick_entry_save_count = $quickEntryProbe.SaveCount
    users_quick_entry_init_round_trip = $quickEntryProbe.InitRoundTrip
    users_quick_entry_restore_code = $quickEntryProbe.RestoreCode
    users_login_log_init_code = $userLoginLogPageInit.code
    users_login_log_platform_dict_count = $userLoginLogPageInitSummary.PlatformCount
    users_login_log_type_dict_count = $userLoginLogPageInitSummary.LoginTypeCount
    users_login_log_list_code = $userLoginLogList.code
    users_login_log_list_count = $userLoginLogListSummary.ListCount
    users_login_log_total = $userLoginLogListSummary.Total
    user_session_page_init_code = $userSessionPageInit.code
    user_session_platform_dict_count = $userSessionPageInitSummary.PlatformCount
    user_session_status_dict_count = $userSessionPageInitSummary.StatusCount
    user_session_list_code = $userSessionList.code
    user_session_list_count = $userSessionListSummary.ListCount
    user_session_total = $userSessionListSummary.Total
    user_session_stats_code = $userSessionStats.code
    user_session_total_active = $userSessionStatsSummary.TotalActive
    user_session_active_admin = $userSessionStatsSummary.Admin
    user_session_active_app = $userSessionStatsSummary.App
    user_session_current_revoke_probe = $userSessionCurrentRevokeProbe.Status
    user_session_current_revoke_blocked = $userSessionCurrentRevokeProbe.Blocked
    user_session_current_revoke_code = $userSessionCurrentRevokeProbe.Code
    user_session_token_hash_leak = $false
    queue_monitor_list_code = $queueMonitorList.code
    queue_monitor_queue_count = $queueMonitorQueueCount
    queue_monitor_failed_code = $queueMonitorFailed.code
    queue_monitor_failed_total = $queueMonitorFailedTotal
    queue_monitor_ui_status = $queueMonitorUI.StatusCode
    system_log_init_code = $systemLogInit.code
    system_log_level_count = $systemLogInitSummary.LevelCount
    system_log_tail_count = $systemLogInitSummary.TailCount
    system_log_files_code = $systemLogFiles.code
    system_log_file_count = $systemLogFilesSummary.Count
    system_log_lines_probe = $systemLogLinesProbe.Status
    system_log_lines_code = $systemLogLinesProbe.Code
    system_log_lines_filename = $systemLogLinesProbe.Filename
    system_log_lines_total = $systemLogLinesProbe.Total
    system_setting_init_code = $systemSettingInit.code
    system_setting_value_type_count = $systemSettingValueTypeCount
    system_setting_list_code = $systemSettingList.code
    system_setting_list_count = $systemSettingListSummary.ListCount
    system_setting_total = $systemSettingListSummary.Total
    client_version_init_code = $clientVersionInit.code
    client_version_platform_dict_count = $clientVersionInitSummary.PlatformCount
    client_version_yes_no_dict_count = $clientVersionInitSummary.YesNoCount
    client_version_list_code = $clientVersionList.code
    client_version_list_count = $clientVersionListSummary.ListCount
    client_version_total = $clientVersionListSummary.Total
    client_version_update_json_code = $clientVersionUpdateJson.code
    client_version_update_json_shape = $clientVersionUpdateJsonSummary.Shape
    client_version_update_json_platform_count = $clientVersionUpdateJsonSummary.PlatformCount
    client_version_update_json_version = $clientVersionUpdateJsonSummary.Version
    upload_driver_init_code = $uploadDriverInit.code
    upload_driver_dict_count = $uploadDriverDictCount
    upload_driver_list_code = $uploadDriverList.code
    upload_driver_list_count = $uploadDriverListSummary.ListCount
    upload_driver_total = $uploadDriverListSummary.Total
    upload_rule_init_code = $uploadRuleInit.code
    upload_rule_image_ext_count = $uploadRuleDictSummary.ImageExtCount
    upload_rule_file_ext_count = $uploadRuleDictSummary.FileExtCount
    upload_rule_list_code = $uploadRuleList.code
    upload_rule_list_count = $uploadRuleListSummary.ListCount
    upload_rule_total = $uploadRuleListSummary.Total
    upload_setting_init_code = $uploadSettingInit.code
    upload_setting_driver_dict_count = $uploadSettingDictSummary.DriverDictCount
    upload_setting_rule_dict_count = $uploadSettingDictSummary.RuleDictCount
    upload_setting_status_dict_count = $uploadSettingDictSummary.StatusDictCount
    upload_setting_list_code = $uploadSettingList.code
    upload_setting_list_count = $uploadSettingListSummary.ListCount
    upload_setting_total = $uploadSettingListSummary.Total
    payment_channel_init_code = $paymentChannelInit.code
    payment_channel_status_count = $paymentChannelInitSummary.StatusCount
    payment_channel_list_code = $paymentChannelList.code
    payment_channel_list_count = $paymentChannelListSummary.ListCount
    payment_channel_total = $paymentChannelListSummary.Total
    ai_model_init_code = $aiModelInit.code
    ai_model_driver_dict_count = $aiModelInitSummary.DriverCount
    ai_model_status_dict_count = $aiModelInitSummary.StatusCount
    ai_model_list_code = $aiModelList.code
    ai_model_list_count = $aiModelListSummary.ListCount
    ai_model_total = $aiModelListSummary.Total
    ai_model_secret_leak = $aiModelListSummary.SecretLeak
    ai_tool_init_code = $aiToolInit.code
    ai_tool_executor_dict_count = $aiToolInitSummary.ExecutorCount
    ai_tool_status_dict_count = $aiToolInitSummary.StatusCount
    ai_tool_list_code = $aiToolList.code
    ai_tool_list_count = $aiToolListSummary.ListCount
    ai_tool_total = $aiToolListSummary.Total
    ai_tool_agent_options_code = $aiToolAgentOptions.code
    ai_tool_agent_options_count = $aiToolAgentOptionsSummary.OptionCount
    ai_tool_retired_cine_present = $aiToolAgentOptionsSummary.RetiredCinePresent
    ai_prompt_list_code = $aiPromptList.code
    ai_prompt_list_count = $aiPromptListSummary.ListCount
    ai_prompt_total = $aiPromptListSummary.Total
    ai_prompt_tags_arrays = $aiPromptListSummary.TagsArrays
    ai_prompt_variables_arrays = $aiPromptListSummary.VariablesArrays
    ai_prompt_detail_code = $aiPromptDetailCode
    ai_prompt_detail_id = $aiPromptDetailID
    ai_prompt_detail_tags_arrays = if ($null -eq $aiPromptDetailSummary) { $true } else { $aiPromptDetailSummary.TagsArrays }
    ai_prompt_detail_variables_arrays = if ($null -eq $aiPromptDetailSummary) { $true } else { $aiPromptDetailSummary.VariablesArrays }
    ai_agent_init_code = $aiAgentInit.code
    ai_agent_scene_dict_count = $aiAgentInitSummary.SceneCount
    ai_agent_retired_scene_present = $aiAgentInitSummary.RetiredScenePresent
    ai_agent_list_code = $aiAgentList.code
    ai_agent_list_count = $aiAgentListSummary.ListCount
    ai_agent_total = $aiAgentListSummary.Total
    ai_knowledge_init_code = $aiKnowledgeInit.code
    ai_knowledge_source_type_count = $aiKnowledgeInitSummary.SourceTypeCount
    ai_knowledge_list_code = $aiKnowledgeList.code
    ai_knowledge_list_count = $aiKnowledgeListSummary.ListCount
    ai_knowledge_total = $aiKnowledgeListSummary.Total
    ai_conversation_list_code = $aiConversationList.code
    ai_conversation_list_count = $aiConversationListSummary.ListCount
    ai_conversation_total = $aiConversationListSummary.Total
    ai_run_init_code = $aiRunInit.code
    ai_run_status_dict_count = $aiRunInitSummary.StatusCount
    ai_run_list_code = $aiRunList.code
    ai_run_list_count = $aiRunListSummary.ListCount
    ai_run_total = $aiRunListSummary.Total
    ai_run_stats_code = $aiRunStats.code
    ai_run_stats_total = $aiRunStatsSummary.TotalRuns
    ai_run_stats_fail = $aiRunStatsSummary.FailRuns
    ai_goods_route_present = $usersInitAIRouteSummary.GoodsPresent
    ai_cine_route_present = $usersInitAIRouteSummary.CinePresent
    ai_models_route_present = $usersInitAIRouteSummary.ModelsPresent
    ai_tools_route_present = $usersInitAIRouteSummary.ToolsPresent
    ai_prompts_route_present = $usersInitAIRouteSummary.PromptsPresent
    payment_route_pay_present = $usersInitPaymentRouteSummary.PayPresent
    payment_route_retired_wallet_present = $usersInitPaymentRouteSummary.RetiredWalletPresent
    payment_route_channel_present = $usersInitPaymentRouteSummary.ChannelPresent
    payment_route_order_present = $usersInitPaymentRouteSummary.OrderPresent
    payment_route_event_present = $usersInitPaymentRouteSummary.EventPresent
    payment_order_init_code = $paymentOrderInit.code
    payment_order_dict_keys = $paymentOrderInitSummary.DictKeys
    payment_order_list_code = $paymentOrderList.code
    payment_order_list_count = $paymentOrderListSummary.ListCount
    payment_order_total = $paymentOrderListSummary.Total
    payment_event_list_code = $paymentEventList.code
    payment_event_list_count = $paymentEventListSummary.ListCount
    payment_event_total = $paymentEventListSummary.Total
    upload_write_probe = $uploadWriteProbe.Status
    upload_write_probe_driver_id = $uploadWriteProbe.DriverID
    upload_write_probe_rule_id = $uploadWriteProbe.RuleID
    upload_write_probe_setting_id = $uploadWriteProbe.SettingID
    upload_token_probe = $uploadTokenProbe.Status
    upload_token_code = $uploadTokenProbe.Code
    upload_token_provider = $uploadTokenProbe.Provider
    upload_token_key = $uploadTokenProbe.Key
    profile_read_code = $profile.code
    profile_user_id = $profileUserID
    profile_is_self = $profile.data.profile.is_self
    target_profile_read_code = $targetProfile.code
    profile_update_code = $profileUpdate.code
    profile_operation_log_id = $profileOperationLog.ID
    profile_operation_log_module = $profileOperationLog.Module
    profile_operation_log_action = $profileOperationLog.Action
    account_security_wrong_old_password_code = $accountSecurityProbe.WrongOldPasswordCode
    account_security_invalid_email_code = $accountSecurityProbe.InvalidEmailCode
    account_security_invalid_phone_code = $accountSecurityProbe.InvalidPhoneCode
    notification_init_code = $notificationInit.code
    notification_type_count = $notificationInitSummary.TypeCount
    notification_level_count = $notificationInitSummary.LevelCount
    notification_read_status_count = $notificationInitSummary.ReadStatusCount
    notification_list_code = $notificationList.code
    notification_list_count = $notificationListSummary.ListCount
    notification_total = $notificationListSummary.Total
    notification_unread_count_code = $notificationUnreadCount.code
    notification_unread_total = $notificationUnreadTotal
    notification_task_init_code = $notificationTaskInit.code
    notification_task_type_count = $notificationTaskInitSummary.TypeCount
    notification_task_level_count = $notificationTaskInitSummary.LevelCount
    notification_task_target_type_count = $notificationTaskInitSummary.TargetTypeCount
    notification_task_status_dict_count = $notificationTaskInitSummary.StatusCount
    notification_task_platform_count = $notificationTaskInitSummary.PlatformCount
    notification_task_status_count_code = $notificationTaskStatusCount.code
    notification_task_status_count_items = $notificationTaskStatusCountTotal
    notification_task_list_code = $notificationTaskList.code
    notification_task_list_count = $notificationTaskListSummary.ListCount
    notification_task_total = $notificationTaskListSummary.Total
    cron_task_init_code = $cronTaskInit.code
    cron_task_preset_count = $cronTaskInitSummary.PresetCount
    cron_task_registry_status_count = $cronTaskInitSummary.RegistryStatusCount
    cron_task_list_code = $cronTaskList.code
    cron_task_list_count = $cronTaskListSummary.ListCount
    cron_task_total = $cronTaskListSummary.Total
    cron_task_notification_registered = $cronTaskListSummary.NotificationRegistered
    cron_task_payment_close_expired_registered = $cronTaskListSummary.PaymentCloseExpiredRegistered
    cron_task_payment_sync_pending_registered = $cronTaskListSummary.PaymentSyncPendingRegistered
    cron_task_ai_run_timeout_registered = $cronTaskListSummary.AIRunTimeoutRegistered
    cron_task_ai_run_timeout_type = $cronTaskListSummary.AIRunTimeoutTaskType
    cron_task_missing_legacy_present = $cronTaskListSummary.MissingLegacyPresent
    cron_task_logs_code = $cronTaskLogsCode
    cron_task_logs_count = $cronTaskLogsSummary.ListCount
    cron_task_logs_total = $cronTaskLogsSummary.Total
    operation_log_init_code = $operationLogInit.code
    operation_log_before_max_id = $beforeMaxID
    operation_log_created_row_id = $operationLogRowID
    operation_log_created_action = $createdLog.action
    operation_log_created_module = $requestData.module
    operation_log_created_route_action = $requestData.action
    operation_log_delete_code = $deleteOperationLog.code
    operation_log_deleted_absent = $deletedAbsent
    permission_cleanup_code = $permissionDelete.code
    logout_code = $logout.code
  }

  $completed = $true
  $summary | ConvertTo-Json -Depth 8
} finally {
  if ($createdPermissionID -gt 0 -and $authHeaders -and $baseURL) {
    try {
      Invoke-RestMethod "$baseURL/api/admin/v1/permissions/$createdPermissionID" `
        -Method Delete `
        -Headers $authHeaders `
        -TimeoutSec 5 | Out-Null
    } catch {
      Write-Host "Failed to cleanup full smoke permission id=$createdPermissionID"
    }
  }

  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force
  }

  Start-Sleep -Milliseconds 300
  Remove-Item -Force $serverExe, $secretReader -ErrorAction SilentlyContinue

  if ($completed) {
    Remove-Item -Force $outLog, $errLog -ErrorAction SilentlyContinue
  } else {
    Write-Host "Full smoke logs kept: $outLog $errLog"
  }
}
