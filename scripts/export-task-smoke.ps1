param(
  [string]$Account = '',
  [string]$Password = '',
  [string]$BaseURL = 'http://127.0.0.1:8080',
  [switch]$RunRealExport
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-RedisAddr() {
  if (-not [string]::IsNullOrWhiteSpace($env:REDIS_ADDR)) { return $env:REDIS_ADDR }

  $hostName = if ([string]::IsNullOrWhiteSpace($env:REDIS_HOST)) { '127.0.0.1' } else { $env:REDIS_HOST }
  $port = if ([string]::IsNullOrWhiteSpace($env:REDIS_PORT)) { '6380' } else { $env:REDIS_PORT }
  return "$hostName`:$port"
}

function Get-RedisDB() {
  if ([string]::IsNullOrWhiteSpace($env:REDIS_DB)) { return '0' }
  return $env:REDIS_DB
}

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

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$secretReader = Join-Path $backendRoot '.tmp/read-captcha-secret.go'
New-Item -ItemType Directory -Force (Join-Path $backendRoot '.tmp') | Out-Null

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
    fmt.Fprintln(os.Stderr, "usage: read-captcha-secret <captcha-id>")
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

  value, err := client.Get(context.Background(), "captcha:slide:"+os.Args[1]).Result()
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }

  fmt.Print(value)
}
"@ | Set-Content -LiteralPath $secretReader -Encoding UTF8

$captcha = Invoke-RestMethod -Uri "$BaseURL/api/admin/v1/auth/captcha" -Method Get
if ($captcha.code -ne 0 -or $null -eq $captcha.data) {
  throw "Captcha request failed: $($captcha | ConvertTo-Json -Depth 8)"
}

$captchaID = [string]$captcha.data.captcha_id
$env:REDIS_ADDR = Get-RedisAddr
$env:REDIS_DB = Get-RedisDB
$secretJson = go run $secretReader $captchaID
if ($LASTEXITCODE -ne 0) {
  throw "Failed to read captcha secret for $captchaID"
}
$secret = $secretJson | ConvertFrom-Json
$captchaAnswer = @{
  x = [int]$secret.answer.x
  y = [int]$secret.answer.y
}

$login = Invoke-RestMethod -Uri "$BaseURL/api/admin/v1/auth/login" -Method Post -ContentType 'application/json' -Body (@{
  login_type = 'password'
  login_account = $Account
  password = $Password
  captcha_id = $captchaID
  captcha_answer = $captchaAnswer
} | ConvertTo-Json -Depth 6) -Headers @{
  platform = 'admin'
  'device-id' = 'codex-export-smoke'
}

$token = $login.data.access_token
if ([string]::IsNullOrWhiteSpace($token)) {
  throw "Login did not return access_token: $($login | ConvertTo-Json -Depth 8)"
}
$headers = @{
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
