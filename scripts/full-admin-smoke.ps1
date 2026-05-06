param(
  [string]$Account = $env:SMOKE_LOGIN_ACCOUNT,
  [string]$Password = $env:SMOKE_LOGIN_PASSWORD,
  [string]$HTTPAddr = '127.0.0.1:18081',
  [string]$BasicHTTPAddr = '127.0.0.1:18080',
  [string]$Platform = 'admin',
  [string]$DeviceID = 'codex-full-smoke',
  [switch]$EnablePaymentRuntimeProbe
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

function Assert-PayChannelInit($Response) {
  Assert-ApiOK $Response 'pay channel init'

  if ($null -eq $Response.data.dict) {
    throw "pay channel init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $channels = Get-ObjectArray $Response.data.dict.channel_arr
  $methods = Get-ObjectArray $Response.data.dict.pay_method_arr
  $statuses = Get-ObjectArray $Response.data.dict.common_status_arr
  if ($channels.Count -ne 2 -or $methods.Count -ne 6 -or $statuses.Count -ne 2) {
    throw "pay channel init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ChannelCount = $channels.Count
    MethodCount = $methods.Count
    StatusCount = $statuses.Count
  }
}

function Assert-PayChannelList($Response) {
  Assert-ApiOK $Response 'pay channel list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "pay channel list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.name)) {
      throw "pay channel item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.supported_methods -or [string]::IsNullOrWhiteSpace([string]$item.supported_methods_text)) {
      throw "pay channel item missing supported method fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -ne $item.app_private_key -or $null -ne $item.app_private_key_enc) {
      throw "pay channel list leaked private key fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-PayRuntimeChannelReady($Response) {
  Assert-ApiOK $Response 'pay runtime channel readiness'

  $rows = Get-ObjectArray $Response.data.list
  foreach ($item in $rows) {
    if ([int]$item.channel -ne 2) { continue }
    if ([string]$item.status_name -ne '启用') { continue }
    if ([string]::IsNullOrWhiteSpace([string]$item.public_cert_path) -or [string]::IsNullOrWhiteSpace([string]$item.platform_cert_path) -or [string]::IsNullOrWhiteSpace([string]$item.root_cert_path)) {
      throw "enabled Alipay channel missing cert path fields: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -ne $item.app_private_key -or $null -ne $item.app_private_key_enc) {
      throw "pay runtime channel readiness leaked private key fields: $($item | ConvertTo-Json -Depth 12)"
    }
    return [pscustomobject]@{
      Status = 'ready'
      ChannelID = [int64]$item.id
      SupportedMethodsCount = (Get-ObjectArray $item.supported_methods).Count
    }
  }

  return [pscustomobject]@{
    Status = 'skipped_no_enabled_alipay_channel'
    ChannelID = 0
    SupportedMethodsCount = 0
  }
}

function Assert-PayTransactionInit($Response) {
  Assert-ApiOK $Response 'pay transaction init'

  if ($null -eq $Response.data.dict) {
    throw "pay transaction init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $channels = Get-ObjectArray $Response.data.dict.channel_arr
  $statuses = Get-ObjectArray $Response.data.dict.txn_status_arr
  if ($channels.Count -ne 2 -or $statuses.Count -ne 5) {
    throw "pay transaction init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ChannelCount = $channels.Count
    StatusCount = $statuses.Count
  }
}

function Assert-PayTransactionList($Response) {
  Assert-ApiOK $Response 'pay transaction list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "pay transaction list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.transaction_no) -or [string]::IsNullOrWhiteSpace([string]$item.order_no)) {
      throw "pay transaction item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -ne $item.app_private_key -or $null -ne $item.app_private_key_enc) {
      throw "pay transaction list leaked private key fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-PayTransactionDetail($Response) {
  Assert-ApiOK $Response 'pay transaction detail'

  if ($null -eq $Response.data.transaction -or [int64]$Response.data.transaction.id -le 0) {
    throw "pay transaction detail missing transaction: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.transaction.channel_resp -or $null -eq $Response.data.transaction.raw_notify) {
    throw "pay transaction detail missing json payload objects: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -ne $Response.data.channel) {
    if ($null -ne $Response.data.channel.app_private_key -or $null -ne $Response.data.channel.app_private_key_enc) {
      throw "pay transaction detail leaked private key fields: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ID = [int64]$Response.data.transaction.id
  }
}

function Assert-PayOrderInit($Response) {
  Assert-ApiOK $Response 'pay order init'

  if ($null -eq $Response.data.dict) {
    throw "pay order init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $orderTypes = Get-ObjectArray $Response.data.dict.order_type_arr
  $payStatuses = Get-ObjectArray $Response.data.dict.pay_status_arr
  $bizStatuses = Get-ObjectArray $Response.data.dict.biz_status_arr
  $rechargePresets = Get-ObjectArray $Response.data.dict.recharge_preset_arr
  if ($orderTypes.Count -ne 3 -or $payStatuses.Count -ne 5 -or $bizStatuses.Count -ne 6 -or $rechargePresets.Count -ne 6) {
    throw "pay order init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    OrderTypeCount = $orderTypes.Count
    PayStatusCount = $payStatuses.Count
    BizStatusCount = $bizStatuses.Count
    RechargePresetCount = $rechargePresets.Count
  }
}

function Assert-PayOrderStatusCount($Response) {
  Assert-ApiOK $Response 'pay order status count'

  $items = Get-ObjectArray $Response.data
  if ($items.Count -ne 5) {
    throw "pay order status count item count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in $items) {
    if ([string]::IsNullOrWhiteSpace([string]$item.label) -or $null -eq $item.value -or $null -eq $item.count) {
      throw "pay order status count item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return $items.Count
}

function Assert-PayOrderList($Response) {
  Assert-ApiOK $Response 'pay order list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "pay order list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.order_no) -or [string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "pay order item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.order_type_text -or $null -eq $item.pay_status_text -or $null -eq $item.biz_status_text) {
      throw "pay order item missing label fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-PayOrderDetail($Response) {
  Assert-ApiOK $Response 'pay order detail'

  if ($null -eq $Response.data.order -or [int64]$Response.data.order.id -le 0) {
    throw "pay order detail missing order: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.items) {
    throw "pay order detail missing items: $($Response | ConvertTo-Json -Depth 12)"
  }
  if ($null -eq $Response.data.order.extra) {
    throw "pay order detail missing extra object: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    ID = [int64]$Response.data.order.id
    PayStatus = [int]$Response.data.order.pay_status
    AdminRemark = [string]$Response.data.order.admin_remark
  }
}

function Assert-CurrentUserWalletSummary($Response) {
  Assert-ApiOK $Response 'current-user wallet summary'

  if ($Response.data.wallet_exists -notin @(1, 2)) {
    throw "current-user wallet summary wallet_exists mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($field in @('balance', 'frozen', 'total_recharge', 'total_consume')) {
    if ($null -eq $Response.data.$field) {
      throw "current-user wallet summary missing money field $field`: $($Response | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    WalletExists = [int]$Response.data.wallet_exists
    Balance = [int]$Response.data.balance
    Frozen = [int]$Response.data.frozen
  }
}

function Assert-CurrentUserWalletBills($Response) {
  Assert-ApiOK $Response 'current-user wallet bills'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "current-user wallet bills missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.biz_action_no)) {
      throw "current-user wallet bill item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.type_text -or $null -eq $item.available_delta -or $null -eq $item.balance_before -or $null -eq $item.balance_after) {
      throw "current-user wallet bill item missing label/money fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-CurrentUserRechargeOrders($Response) {
  Assert-ApiOK $Response 'current-user recharge orders'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "current-user recharge orders missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }
  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.order_no) -or [string]::IsNullOrWhiteSpace([string]$item.title)) {
      throw "current-user recharge order item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.pay_status_text -or $null -eq $item.biz_status_text) {
      throw "current-user recharge order item missing status text: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-WalletInit($Response) {
  Assert-ApiOK $Response 'wallet init'

  if ($null -eq $Response.data.dict) {
    throw "wallet init missing dict: $($Response | ConvertTo-Json -Depth 12)"
  }

  $walletTypes = Get-ObjectArray $Response.data.dict.wallet_type_arr
  $walletSources = Get-ObjectArray $Response.data.dict.wallet_source_arr
  if ($walletTypes.Count -ne 3 -or $walletSources.Count -ne 3) {
    throw "wallet init dict count mismatch: $($Response | ConvertTo-Json -Depth 12)"
  }

  return [pscustomobject]@{
    WalletTypeCount = $walletTypes.Count
    WalletSourceCount = $walletSources.Count
  }
}

function Assert-WalletList($Response) {
  Assert-ApiOK $Response 'wallet list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "wallet list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [int64]$item.user_id -le 0) {
      throw "wallet item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    foreach ($field in @('balance', 'frozen', 'total_recharge', 'total_consume')) {
      if ($null -eq $item.$field) {
        throw "wallet item missing money field $field`: $($item | ConvertTo-Json -Depth 12)"
      }
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Assert-WalletTransactionList($Response) {
  Assert-ApiOK $Response 'wallet transaction list'

  if ($null -eq $Response.data.page -or $null -eq $Response.data.list) {
    throw "wallet transaction list missing page/list: $($Response | ConvertTo-Json -Depth 12)"
  }

  foreach ($item in (Get-ObjectArray $Response.data.list)) {
    if ([int64]$item.id -le 0 -or [int64]$item.user_id -le 0 -or [string]::IsNullOrWhiteSpace([string]$item.biz_action_no)) {
      throw "wallet transaction item shape mismatch: $($item | ConvertTo-Json -Depth 12)"
    }
    if ($null -eq $item.type_text -or $null -eq $item.available_delta -or $null -eq $item.balance_before -or $null -eq $item.balance_after) {
      throw "wallet transaction item missing label/money fields: $($item | ConvertTo-Json -Depth 12)"
    }
  }

  return [pscustomobject]@{
    ListCount = (Get-ObjectArray $Response.data.list).Count
    Total = [int64]$Response.data.page.total
  }
}

function Get-WalletBalance([string]$BaseURL, [hashtable]$Headers, [int64]$UserID) {
  $response = Invoke-RestMethod "$BaseURL/api/admin/v1/wallets?current_page=1&page_size=1&user_id=$UserID" `
    -Headers $Headers `
    -TimeoutSec 10
  Assert-ApiOK $response 'wallet balance readback'

  $rows = Get-ObjectArray $response.data.list
  if ($rows.Count -eq 0) {
    throw "wallet balance readback missing user_id=$UserID"
  }

  return [int]$rows[0].balance
}

function Invoke-WalletAdjustmentProbe([string]$BaseURL, [hashtable]$Headers, $WalletRow) {
  if ($null -eq $WalletRow -or [int64]$WalletRow.user_id -le 0) {
    return [pscustomobject]@{
      Status = 'skipped_no_wallet_rows'
      UserID = 0
      PlusCode = $null
      DuplicateSameTransaction = $false
      Restored = $false
      PlusTransactionID = 0
      MinusTransactionID = 0
    }
  }

  $userID = [int64]$WalletRow.user_id
  $originalBalance = [int]$WalletRow.balance
  $suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  $plusBody = @{
    user_id = $userID
    delta = 100
    reason = "codex-full-smoke-adjust-plus-$suffix"
    idempotency_key = "codex-full-smoke-plus-$suffix"
  }
  $minusBody = @{
    user_id = $userID
    delta = -100
    reason = "codex-full-smoke-adjust-restore-$suffix"
    idempotency_key = "codex-full-smoke-minus-$suffix"
  }
  $restored = $false
  $plus = $null
  $minus = $null

  try {
    $plus = Invoke-JsonRequestAllowFailure 'Post' "$BaseURL/api/admin/v1/wallet-adjustments" $Headers $plusBody
    Assert-ApiOK $plus 'wallet adjustment plus'
    if ([int]$plus.data.balance_before -ne $originalBalance -or [int]$plus.data.balance_after -ne ($originalBalance + 100)) {
      throw "wallet adjustment plus balance mismatch: original=$originalBalance response=$($plus | ConvertTo-Json -Depth 12)"
    }

    $duplicate = Invoke-JsonRequestAllowFailure 'Post' "$BaseURL/api/admin/v1/wallet-adjustments" $Headers $plusBody
    Assert-ApiOK $duplicate 'wallet adjustment duplicate'
    $duplicateSameTransaction = [int64]$duplicate.data.transaction_id -eq [int64]$plus.data.transaction_id
    if (-not $duplicateSameTransaction) {
      throw "wallet adjustment duplicate returned different transaction: first=$($plus.data.transaction_id) duplicate=$($duplicate.data.transaction_id)"
    }

    $balanceAfterDuplicate = Get-WalletBalance $BaseURL $Headers $userID
    if ($balanceAfterDuplicate -ne ($originalBalance + 100)) {
      throw "wallet adjustment duplicate mutated balance: expected=$($originalBalance + 100) actual=$balanceAfterDuplicate"
    }

    $minus = Invoke-JsonRequestAllowFailure 'Post' "$BaseURL/api/admin/v1/wallet-adjustments" $Headers $minusBody
    Assert-ApiOK $minus 'wallet adjustment restore'
    $finalBalance = Get-WalletBalance $BaseURL $Headers $userID
    $restored = $finalBalance -eq $originalBalance
    if (-not $restored) {
      throw "wallet adjustment restore failed: original=$originalBalance final=$finalBalance"
    }

    return [pscustomobject]@{
      Status = 'ok'
      UserID = $userID
      PlusCode = [int]$plus.code
      DuplicateSameTransaction = $duplicateSameTransaction
      Restored = $restored
      PlusTransactionID = [int64]$plus.data.transaction_id
      MinusTransactionID = [int64]$minus.data.transaction_id
    }
  } catch {
    if ($null -ne $plus -and -not $restored) {
      try {
        $restoreAttempt = Invoke-JsonRequestAllowFailure 'Post' "$BaseURL/api/admin/v1/wallet-adjustments" $Headers $minusBody
        Assert-ApiOK $restoreAttempt 'wallet adjustment restore after failure'
        $restored = (Get-WalletBalance $BaseURL $Headers $userID) -eq $originalBalance
      } catch {
        # Keep the original failure below; the final hard failure still shows restore status.
      }
    }
    throw "wallet adjustment probe failed; restored=$restored; original=$originalBalance; error=$($_.Exception.Message)"
  }
}

function Invoke-PaymentRuntimeProbe([string]$BaseURL, [hashtable]$Headers, $ChannelReady) {
  if (-not $EnablePaymentRuntimeProbe) {
    return [pscustomobject]@{
      Status = 'skipped_flag_disabled'
      OrderCode = $null
      AttemptCode = $null
      ResultCode = $null
      CancelCode = $null
      CleanupStatus = 'not_created'
      ChannelID = [int64]$ChannelReady.ChannelID
      OrderNo = ''
      TransactionNo = ''
      PayDataMode = ''
      PayDataHasContent = $false
    }
  }
  if ([int64]$ChannelReady.ChannelID -le 0) {
    return [pscustomobject]@{
      Status = 'skipped_no_enabled_alipay_channel'
      OrderCode = $null
      AttemptCode = $null
      ResultCode = $null
      CancelCode = $null
      CleanupStatus = 'not_created'
      ChannelID = 0
      OrderNo = ''
      TransactionNo = ''
      PayDataMode = ''
      PayDataHasContent = $false
    }
  }

  $order = $null
  $attempt = $null
  $result = $null
  $cancel = $null
  $orderNo = ''
  $transactionNo = ''
  $payDataMode = ''
  $payDataHasContent = $false
  $cleanupStatus = 'not_created'
  $failure = $null

  try {
    $orderBody = @{
      amount = 1000
      pay_method = 'web'
      channel_id = [int64]$ChannelReady.ChannelID
    }
    $order = Invoke-JsonRequestAllowFailure 'Post' "$BaseURL/api/admin/v1/recharge-orders" $Headers $orderBody
    Assert-ApiOK $order 'payment runtime recharge order create'
    if ([string]::IsNullOrWhiteSpace([string]$order.data.order_no) -or [int]$order.data.pay_amount -ne 1000) {
      throw "payment runtime recharge order shape mismatch: $($order | ConvertTo-Json -Depth 12)"
    }
    $orderNo = [string]$order.data.order_no
    $cleanupStatus = 'created_not_cleaned'

    $attemptBody = @{
      pay_method = 'web'
      return_url = "$BaseURL/__codex-payment-return"
    }
    $attempt = Invoke-JsonRequestAllowFailure 'Post' "$BaseURL/api/admin/v1/recharge-orders/$orderNo/pay-attempts" $Headers $attemptBody
    Assert-ApiOK $attempt 'payment runtime pay attempt create'
    if ([string]::IsNullOrWhiteSpace([string]$attempt.data.transaction_no) -or $null -eq $attempt.data.pay_data) {
      throw "payment runtime pay attempt shape mismatch: $($attempt | ConvertTo-Json -Depth 12)"
    }
    if ([string]::IsNullOrWhiteSpace([string]$attempt.data.pay_data.content)) {
      throw "payment runtime pay attempt returned empty pay_data.content: $($attempt | ConvertTo-Json -Depth 12)"
    }
    $transactionNo = [string]$attempt.data.transaction_no
    $payDataMode = [string]$attempt.data.pay_data.mode
    $payDataHasContent = -not [string]::IsNullOrWhiteSpace([string]$attempt.data.pay_data.content)

    $result = Invoke-RestMethod "$BaseURL/api/admin/v1/recharge-orders/$orderNo/result" `
      -Headers $Headers `
      -TimeoutSec 10
    Assert-ApiOK $result 'payment runtime query result'
    if ([string]$result.data.order_no -ne $orderNo) {
      throw "payment runtime query result order mismatch: expected=$orderNo response=$($result | ConvertTo-Json -Depth 12)"
    }
  } catch {
    $failure = $_
  }

  if (-not [string]::IsNullOrWhiteSpace($orderNo)) {
    try {
      $cancel = Invoke-JsonRequestAllowFailure 'Patch' "$BaseURL/api/admin/v1/recharge-orders/$orderNo/cancel" $Headers @{ reason = 'codex full smoke cleanup' }
      Assert-ApiOK $cancel 'payment runtime smoke order cancel'
      $cleanupStatus = 'cancelled'
    } catch {
      $cleanupStatus = "cancel_failed: $($_.Exception.Message)"
      if ($null -eq $failure) {
        $failure = $_
      }
    }
  }

  if ($null -ne $failure) {
    throw "payment runtime probe failed; cleanup=$cleanupStatus; error=$($failure.Exception.Message)"
  }

  return [pscustomobject]@{
    Status = 'ok'
    OrderCode = [int]$order.code
    AttemptCode = [int]$attempt.code
    ResultCode = [int]$result.code
    CancelCode = [int]$cancel.code
    CleanupStatus = $cleanupStatus
    ChannelID = [int64]$ChannelReady.ChannelID
    OrderNo = $orderNo
    TransactionNo = $transactionNo
    PayDataMode = $payDataMode
    PayDataHasContent = $payDataHasContent
  }
}

function Invoke-PayOrderRemarkProbe([string]$BaseURL, [hashtable]$Headers, $OrderDetailSummary) {
  if ($null -eq $OrderDetailSummary -or [int64]$OrderDetailSummary.ID -le 0) {
    return [pscustomobject]@{
      Status = 'skipped_no_orders'
      Code = $null
      OrderID = 0
    }
  }

  $orderID = [int64]$OrderDetailSummary.ID
  $originalRemark = [string]$OrderDetailSummary.AdminRemark
  $newRemark = "codex smoke remark $([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
  $restoreRemark = if ([string]::IsNullOrWhiteSpace($originalRemark)) { 'codex smoke restored blank remark' } else { $originalRemark }

  $update = Invoke-JsonRequestAllowFailure 'Patch' "$BaseURL/api/admin/v1/pay-orders/$orderID/remark" $Headers @{ remark = $newRemark }
  Assert-ApiOK $update 'pay order remark update'

  $restore = Invoke-JsonRequestAllowFailure 'Patch' "$BaseURL/api/admin/v1/pay-orders/$orderID/remark" $Headers @{ remark = $restoreRemark }
  Assert-ApiOK $restore 'pay order remark restore'

  return [pscustomobject]@{
    Status = 'passed'
    Code = $update.code
    OrderID = $orderID
    RestoredOriginalBlank = [string]::IsNullOrWhiteSpace($originalRemark)
  }
}

function Invoke-PayOrderCloseProbe([string]$BaseURL, [hashtable]$Headers) {
  $pendingList = Invoke-RestMethod "$BaseURL/api/admin/v1/pay-orders?current_page=1&page_size=1&pay_status=1" `
    -Headers $Headers `
    -TimeoutSec 10
  Assert-ApiOK $pendingList 'pay order close probe pending list'

  $rows = Get-ObjectArray $pendingList.data.list
  if ($rows.Count -eq 0) {
    $payingList = Invoke-RestMethod "$BaseURL/api/admin/v1/pay-orders?current_page=1&page_size=1&pay_status=2" `
      -Headers $Headers `
      -TimeoutSec 10
    Assert-ApiOK $payingList 'pay order close probe paying list'
    $rows = Get-ObjectArray $payingList.data.list
  }

  if ($rows.Count -eq 0) {
    return [pscustomobject]@{
      Status = 'skipped_no_pending_or_paying_orders'
      Code = $null
      OrderID = 0
    }
  }

  $orderID = [int64]$rows[0].id
  $response = Invoke-JsonRequestAllowFailure 'Patch' "$BaseURL/api/admin/v1/pay-orders/$orderID/close" $Headers @{ reason = 'codex full smoke local close' }
  Assert-ApiOK $response 'pay order close probe'

  return [pscustomobject]@{
    Status = 'passed'
    Code = $response.code
    OrderID = $orderID
  }
}

function Clear-UserButtonCache([int64]$UserID, [string]$CachePlatform) {
  if ($UserID -le 0 -or [string]::IsNullOrWhiteSpace($CachePlatform)) { return }

  $cacheClearer = '.tmp/clear-user-button-cache.go'
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
  if len(os.Args) != 3 {
    fmt.Fprintln(os.Stderr, "usage: clear-user-button-cache <user-id> <platform>")
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

  key := "auth_perm_uid_" + os.Args[1] + "_" + os.Args[2] + "_rbac_page_grants"
  if err := client.Del(context.Background(), key).Err(); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
}
"@ | Set-Content -LiteralPath $cacheClearer -Encoding UTF8

  $env:REDIS_ADDR = Get-RedisAddr
  $env:REDIS_DB = Get-RedisDB
  go run $cacheClearer ([string]$UserID) $CachePlatform
  if ($LASTEXITCODE -ne 0) {
    throw "failed to clear RBAC button cache for user=$UserID platform=$CachePlatform"
  }
  Remove-Item -Force $cacheClearer -ErrorAction SilentlyContinue
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

  $payChannelInit = Invoke-RestMethod "$baseURL/api/admin/v1/pay-channels/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payChannelInitSummary = Assert-PayChannelInit $payChannelInit

  $payChannelList = Invoke-RestMethod "$baseURL/api/admin/v1/pay-channels?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payChannelListSummary = Assert-PayChannelList $payChannelList
  $payRuntimeChannelReady = Assert-PayRuntimeChannelReady $payChannelList

  $currentUserWalletSummary = Invoke-RestMethod "$baseURL/api/admin/v1/wallet/summary" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $currentUserWalletSummaryResult = Assert-CurrentUserWalletSummary $currentUserWalletSummary

  $currentUserWalletBills = Invoke-RestMethod "$baseURL/api/admin/v1/wallet/bills?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $currentUserWalletBillsSummary = Assert-CurrentUserWalletBills $currentUserWalletBills

  $currentUserRechargeOrders = Invoke-RestMethod "$baseURL/api/admin/v1/recharge-orders?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $currentUserRechargeOrdersSummary = Assert-CurrentUserRechargeOrders $currentUserRechargeOrders

  $payTransactionInit = Invoke-RestMethod "$baseURL/api/admin/v1/pay-transactions/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payTransactionInitSummary = Assert-PayTransactionInit $payTransactionInit

  $payTransactionList = Invoke-RestMethod "$baseURL/api/admin/v1/pay-transactions?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payTransactionListSummary = Assert-PayTransactionList $payTransactionList
  $payTransactionDetailCode = $null
  $payTransactionDetailID = 0
  $payTransactionRows = Get-ObjectArray $payTransactionList.data.list
  if ($payTransactionRows.Count -gt 0) {
    $firstPayTransaction = $payTransactionRows[0]
    $payTransactionDetail = Invoke-RestMethod "$baseURL/api/admin/v1/pay-transactions/$($firstPayTransaction.id)" `
      -Headers $authHeaders `
      -TimeoutSec 10
    $payTransactionDetailSummary = Assert-PayTransactionDetail $payTransactionDetail
    $payTransactionDetailCode = $payTransactionDetail.code
    $payTransactionDetailID = $payTransactionDetailSummary.ID
  }

  $payOrderInit = Invoke-RestMethod "$baseURL/api/admin/v1/pay-orders/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payOrderInitSummary = Assert-PayOrderInit $payOrderInit

  $payOrderStatusCount = Invoke-RestMethod "$baseURL/api/admin/v1/pay-orders/status-count" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payOrderStatusCountItems = Assert-PayOrderStatusCount $payOrderStatusCount

  $payOrderList = Invoke-RestMethod "$baseURL/api/admin/v1/pay-orders?current_page=1&page_size=20" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $payOrderListSummary = Assert-PayOrderList $payOrderList
  $payOrderDetailCode = $null
  $payOrderDetailID = 0
  $payOrderDetailSummary = $null
  $payOrderRows = Get-ObjectArray $payOrderList.data.list
  if ($payOrderRows.Count -gt 0) {
    $firstPayOrder = $payOrderRows[0]
    $payOrderDetail = Invoke-RestMethod "$baseURL/api/admin/v1/pay-orders/$($firstPayOrder.id)" `
      -Headers $authHeaders `
      -TimeoutSec 10
    $payOrderDetailSummary = Assert-PayOrderDetail $payOrderDetail
    $payOrderDetailCode = $payOrderDetail.code
    $payOrderDetailID = $payOrderDetailSummary.ID
  }
  $payOrderRemarkProbe = Invoke-PayOrderRemarkProbe $baseURL $authHeaders $payOrderDetailSummary
  $payOrderCloseProbe = Invoke-PayOrderCloseProbe $baseURL $authHeaders
  $paymentRuntimeProbe = Invoke-PaymentRuntimeProbe $baseURL $authHeaders $payRuntimeChannelReady

  $walletInit = Invoke-RestMethod "$baseURL/api/admin/v1/wallets/page-init" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $walletInitSummary = Assert-WalletInit $walletInit

  $walletList = Invoke-RestMethod "$baseURL/api/admin/v1/wallets?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $walletListSummary = Assert-WalletList $walletList

  $walletTransactionList = Invoke-RestMethod "$baseURL/api/admin/v1/wallet-transactions?current_page=1&page_size=10" `
    -Headers $authHeaders `
    -TimeoutSec 10
  $walletTransactionListSummary = Assert-WalletTransactionList $walletTransactionList
  $walletAdjustmentBeforeLogs = Get-OperationLogList $baseURL $authHeaders '钱包调账'
  Assert-ApiOK $walletAdjustmentBeforeLogs 'wallet adjustment operation log before list'
  $walletAdjustmentBeforeMaxID = Get-MaxOperationLogID $walletAdjustmentBeforeLogs
  $walletRows = Get-ObjectArray $walletList.data.list
  $walletAdjustmentProbe = if ($walletRows.Count -gt 0) {
    Invoke-WalletAdjustmentProbe $baseURL $authHeaders $walletRows[0]
  } else {
    [pscustomobject]@{
      Status = 'skipped_no_wallet_rows'
      UserID = 0
      PlusCode = $null
      DuplicateSameTransaction = $false
      Restored = $false
      PlusTransactionID = 0
      MinusTransactionID = 0
    }
  }
  $walletAdjustmentOperationLog = if ($walletAdjustmentProbe.Status -eq 'ok') {
    Wait-NewOperationLog $baseURL $authHeaders '钱包调账' $walletAdjustmentBeforeMaxID
  } else {
    $null
  }

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
    pay_channel_init_code = $payChannelInit.code
    pay_channel_dict_count = $payChannelInitSummary.ChannelCount
    pay_channel_method_count = $payChannelInitSummary.MethodCount
    pay_channel_status_count = $payChannelInitSummary.StatusCount
    pay_channel_list_code = $payChannelList.code
    pay_channel_list_count = $payChannelListSummary.ListCount
    pay_channel_total = $payChannelListSummary.Total
    pay_runtime_channel_status = $payRuntimeChannelReady.Status
    pay_runtime_channel_id = $payRuntimeChannelReady.ChannelID
    pay_runtime_channel_supported_methods_count = $payRuntimeChannelReady.SupportedMethodsCount
    pay_runtime_wallet_summary_code = $currentUserWalletSummary.code
    pay_runtime_wallet_exists = $currentUserWalletSummaryResult.WalletExists
    pay_runtime_wallet_balance = $currentUserWalletSummaryResult.Balance
    pay_runtime_wallet_frozen = $currentUserWalletSummaryResult.Frozen
    pay_runtime_wallet_bills_code = $currentUserWalletBills.code
    pay_runtime_wallet_bills_count = $currentUserWalletBillsSummary.ListCount
    pay_runtime_wallet_bills_total = $currentUserWalletBillsSummary.Total
    pay_runtime_recharge_orders_code = $currentUserRechargeOrders.code
    pay_runtime_recharge_orders_count = $currentUserRechargeOrdersSummary.ListCount
    pay_runtime_recharge_orders_total = $currentUserRechargeOrdersSummary.Total
    pay_runtime_probe_status = $paymentRuntimeProbe.Status
    pay_runtime_probe_order_code = $paymentRuntimeProbe.OrderCode
    pay_runtime_probe_attempt_code = $paymentRuntimeProbe.AttemptCode
    pay_runtime_probe_result_code = $paymentRuntimeProbe.ResultCode
    pay_runtime_probe_cancel_code = $paymentRuntimeProbe.CancelCode
    pay_runtime_probe_cleanup_status = $paymentRuntimeProbe.CleanupStatus
    pay_runtime_probe_order_no = $paymentRuntimeProbe.OrderNo
    pay_runtime_probe_transaction_no = $paymentRuntimeProbe.TransactionNo
    pay_runtime_probe_pay_data_mode = $paymentRuntimeProbe.PayDataMode
    pay_runtime_probe_pay_data_has_content = $paymentRuntimeProbe.PayDataHasContent
    pay_transaction_init_code = $payTransactionInit.code
    pay_transaction_channel_dict_count = $payTransactionInitSummary.ChannelCount
    pay_transaction_status_dict_count = $payTransactionInitSummary.StatusCount
    pay_transaction_list_code = $payTransactionList.code
    pay_transaction_list_count = $payTransactionListSummary.ListCount
    pay_transaction_total = $payTransactionListSummary.Total
    pay_transaction_detail_code = $payTransactionDetailCode
    pay_transaction_detail_id = $payTransactionDetailID
    pay_order_init_code = $payOrderInit.code
    pay_order_type_dict_count = $payOrderInitSummary.OrderTypeCount
    pay_order_pay_status_dict_count = $payOrderInitSummary.PayStatusCount
    pay_order_biz_status_dict_count = $payOrderInitSummary.BizStatusCount
    pay_order_recharge_preset_dict_count = $payOrderInitSummary.RechargePresetCount
    pay_order_status_count_code = $payOrderStatusCount.code
    pay_order_status_count_items = $payOrderStatusCountItems
    pay_order_list_code = $payOrderList.code
    pay_order_list_count = $payOrderListSummary.ListCount
    pay_order_total = $payOrderListSummary.Total
    pay_order_detail_code = $payOrderDetailCode
    pay_order_detail_id = $payOrderDetailID
    pay_order_remark_probe = $payOrderRemarkProbe.Status
    pay_order_remark_probe_order_id = $payOrderRemarkProbe.OrderID
    pay_order_remark_probe_restored_original_blank = $payOrderRemarkProbe.RestoredOriginalBlank
    pay_order_close_probe = $payOrderCloseProbe.Status
    pay_order_close_probe_order_id = $payOrderCloseProbe.OrderID
    wallet_init_code = $walletInit.code
    wallet_type_dict_count = $walletInitSummary.WalletTypeCount
    wallet_source_dict_count = $walletInitSummary.WalletSourceCount
    wallet_list_code = $walletList.code
    wallet_list_count = $walletListSummary.ListCount
    wallet_total = $walletListSummary.Total
    wallet_transaction_list_code = $walletTransactionList.code
    wallet_transaction_list_count = $walletTransactionListSummary.ListCount
    wallet_transaction_total = $walletTransactionListSummary.Total
    wallet_adjustment_status = $walletAdjustmentProbe.Status
    wallet_adjustment_user_id = $walletAdjustmentProbe.UserID
    wallet_adjustment_plus_code = $walletAdjustmentProbe.PlusCode
    wallet_adjustment_duplicate_same_transaction = $walletAdjustmentProbe.DuplicateSameTransaction
    wallet_adjustment_restored = $walletAdjustmentProbe.Restored
    wallet_adjustment_plus_transaction_id = $walletAdjustmentProbe.PlusTransactionID
    wallet_adjustment_minus_transaction_id = $walletAdjustmentProbe.MinusTransactionID
    wallet_adjustment_operation_log_id = if ($null -eq $walletAdjustmentOperationLog) { 0 } else { [int64]$walletAdjustmentOperationLog.id }
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
