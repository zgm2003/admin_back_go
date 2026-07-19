param(
  [string]$Account = '',
  [string]$Password = '',
  [string]$BaseURL = 'http://127.0.0.1:8080',
  [string]$Origin = 'http://localhost:5173',
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

function Assert-CredentialResponse($Response, [string]$Label) {
  $fields = @($Response.data.PSObject.Properties.Name)
  if ($Response.code -ne 0 -or $fields.Count -ne 2 -or $fields -notcontains 'access_token' -or $fields -notcontains 'expires_in') {
    throw "$Label violated the closed Browser credential contract"
  }
  if ([string]::IsNullOrWhiteSpace([string]$Response.data.access_token) -or [int64]$Response.data.expires_in -le 0) {
    throw "$Label returned an invalid Browser credential"
  }
}

function Get-RefreshCookieHeader($Session, [string]$BaseURL) {
  $builder = [System.UriBuilder]::new([uri]$BaseURL)
  $builder.Scheme = 'https'
  $builder.Path = '/api/admin/v1/auth/refresh'
  $cookie = @($Session.Cookies.GetCookies($builder.Uri) | Where-Object { $_.Name -eq '__Secure-admin_refresh' }) | Select-Object -First 1
  if ($null -eq $cookie -or [string]::IsNullOrWhiteSpace($cookie.Value)) {
    throw 'Browser refresh Cookie was not issued'
  }
  return "$($cookie.Name)=$($cookie.Value)"
}

$browserSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$credentialHeaders = @{
  Origin = $Origin
  platform = 'admin'
  'device-id' = 'codex-export-smoke'
}

$login = Invoke-RestMethod -Uri "$BaseURL/api/admin/v1/auth/login" -Method Post -ContentType 'application/json' -Body (@{
  login_type = 'password'
  login_account = $Account
  password = $Password
} | ConvertTo-Json -Depth 4) -Headers $credentialHeaders -WebSession $browserSession

Assert-CredentialResponse $login 'export smoke login'
$refreshHeaders = $credentialHeaders.Clone()
$refreshHeaders.Cookie = Get-RefreshCookieHeader $browserSession $BaseURL
$refresh = Invoke-RestMethod -Uri "$BaseURL/api/admin/v1/auth/refresh" -Method Post -Headers $refreshHeaders -WebSession $browserSession
Assert-CredentialResponse $refresh 'export smoke refresh'
$token = $refresh.data.access_token
$headers = @{
  Origin = $Origin
  platform = 'admin'
  'device-id' = 'codex-export-smoke'
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

Invoke-RestMethod -Uri "$BaseURL/api/admin/v1/auth/logout" -Method Post -Headers $headers -WebSession $browserSession | Out-Null

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
