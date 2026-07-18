param(
  [string]$Account = '',
  [string]$Password = '',
  [string]$BaseURL = 'http://127.0.0.1:8080',
  [switch]$RunRealExport
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-Api {
  param(
    [string]$Url,
    [string]$Method,
    [hashtable]$Headers,
    $Body = $null
  )

  $params = @{
    Uri     = $Url
    Method  = $Method
    Headers = $Headers
  }
  if ($null -ne $Body) {
    $params.ContentType = 'application/json'
    $params.Body = ($Body | ConvertTo-Json -Depth 8)
  }
  Invoke-RestMethod @params
}

if (-not $RunRealExport) {
  [pscustomobject]@{
    run_real_export = $false
    note = 'Set -RunRealExport to submit a real export task.'
  } | ConvertTo-Json -Depth 4
  exit 0
}

if ([string]::IsNullOrWhiteSpace($Account) -or [string]::IsNullOrWhiteSpace($Password)) {
  throw 'Account and Password are required when -RunRealExport is set.'
}

$login = Invoke-RestMethod -Uri "$BaseURL/api/admin/v1/auth/login" -Method Post -ContentType 'application/json' -Body (@{
  login_type = 'password'
  login_account = $Account
  password = $Password
} | ConvertTo-Json -Depth 4) -Headers @{
  platform = 'admin'
  'device-id' = 'codex-export-smoke'
  'X-Admin-Client-Variant' = 'desktop'
}

$token = $login.data.access_token
if ([string]::IsNullOrWhiteSpace($token)) {
  throw "Login did not return access_token: $($login | ConvertTo-Json -Depth 8)"
}
$headers = @{
  platform = 'admin'
  'device-id' = 'codex-export-smoke'
  'X-Admin-Client-Variant' = 'desktop'
  Authorization = "Bearer $token"
}

$users = Invoke-Api "$BaseURL/api/admin/v1/users?current_page=1&page_size=1" 'Get' $headers
$firstUser = $users.data.list | Select-Object -First 1
if ($null -eq $firstUser -or $null -eq $firstUser.id) {
  throw 'No user available for export smoke.'
}

$submit = Invoke-Api "$BaseURL/api/admin/v1/users/export" 'Post' $headers @{ ids = @([int64]$firstUser.id) }

$task = $null
for ($i = 0; $i -lt 20; $i++) {
  Start-Sleep -Seconds 2
  $list = Invoke-Api "$BaseURL/api/admin/v1/export-tasks?current_page=1&page_size=10&kind=user_list" 'Get' $headers
  $task = @($list.data.list) | Select-Object -First 1
  if ($null -ne $task -and ($task.status -eq 2 -or $task.status -eq 3)) {
    break
  }
}

if ($null -eq $task) {
  throw 'Export task was not observed in export task list.'
}

[pscustomobject]@{
  run_real_export   = $true
  submitted_message = if ($submit.PSObject.Properties.Name -contains 'message') { $submit.message } else { $submit.msg }
  submit_code       = $submit.code
  task_id           = $task.id
  status            = $task.status
  file_name         = $task.file_name
  row_count         = $task.row_count
  file_url_present  = -not [string]::IsNullOrWhiteSpace($task.file_url)
} | ConvertTo-Json -Depth 4
