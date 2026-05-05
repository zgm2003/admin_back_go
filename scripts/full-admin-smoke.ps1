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
