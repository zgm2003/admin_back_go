param(
  [string]$Account = $env:SMOKE_LOGIN_ACCOUNT,
  [string]$Password = $env:SMOKE_LOGIN_PASSWORD,
  [string]$HTTPAddr = '127.0.0.1:18080',
  [string]$Platform = 'admin',
  [string]$DeviceID = 'codex-smoke'
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
    throw "Port $port is already listening. Stop the existing process first, then rerun smoke."
  }
}

Import-DotEnv (Join-Path $BackendRoot '.env')

if ([string]::IsNullOrWhiteSpace($Account) -or [string]::IsNullOrWhiteSpace($Password)) {
  throw 'Set SMOKE_LOGIN_ACCOUNT and SMOKE_LOGIN_PASSWORD, or pass -Account and -Password.'
}

Assert-PortFree $HTTPAddr

New-Item -ItemType Directory -Force .tmp | Out-Null
$serverExe = '.tmp/admin-api-smoke.exe'
$secretReader = '.tmp/read-captcha-secret.go'
$outLog = '.tmp/basic-admin-smoke-out.log'
$errLog = '.tmp/basic-admin-smoke-err.log'
$completed = $false
$createdPermissionID = $null
$authHeaders = $null
$baseURL = $null

Remove-Item -Force $serverExe, $secretReader, $outLog, $errLog -ErrorAction SilentlyContinue

go build -o $serverExe ./cmd/admin-api

$env:HTTP_ADDR = $HTTPAddr
$proc = Start-Process -FilePath (Resolve-Path $serverExe) `
  -PassThru `
  -WindowStyle Hidden `
  -RedirectStandardOutput $outLog `
  -RedirectStandardError $errLog

try {
  $baseURL = "http://$HTTPAddr"
  Wait-Health $baseURL

  $ready = Invoke-RestMethod "$baseURL/ready" -TimeoutSec 5
  $loginConfig = Invoke-RestMethod "$baseURL/api/admin/v1/auth/login-config" `
    -Headers @{ platform = $Platform } `
    -TimeoutSec 5
  $captcha = Invoke-RestMethod "$baseURL/api/admin/v1/auth/captcha" -TimeoutSec 10

  if ($captcha.code -ne 0 -or [string]::IsNullOrWhiteSpace($captcha.data.captcha_id)) {
    throw "captcha endpoint failed: $($captcha | ConvertTo-Json -Depth 8)"
  }

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

  $loginHeaders = @{
    platform = $Platform
    'device-id' = $DeviceID
  }

  $login = Invoke-RestMethod "$baseURL/api/admin/v1/auth/login" `
    -Method Post `
    -Headers $loginHeaders `
    -ContentType 'application/json' `
    -Body $loginBody `
    -TimeoutSec 10

  if ($login.code -ne 0 -or [string]::IsNullOrWhiteSpace($login.data.access_token)) {
    throw "login failed: $($login | ConvertTo-Json -Depth 8)"
  }

  $authHeaders = @{
    platform = $Platform
    'device-id' = $DeviceID
    Authorization = "Bearer $($login.data.access_token)"
  }

  $me = Invoke-RestMethod "$baseURL/api/admin/v1/users/me" -Headers $authHeaders -TimeoutSec 10
  $init = Invoke-RestMethod "$baseURL/api/admin/v1/users/init" -Headers $authHeaders -TimeoutSec 10
  $authPlatformInit = Invoke-RestMethod "$baseURL/api/admin/v1/auth-platforms/init" -Headers $authHeaders -TimeoutSec 10
  $authPlatformList = Invoke-RestMethod "$baseURL/api/admin/v1/auth-platforms?current_page=1&page_size=50" -Headers $authHeaders -TimeoutSec 10
  $permissionSuffix = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
  $permissionBody = @{
    platform = $Platform
    type = 1
    name = "Codex Smoke $permissionSuffix"
    parent_id = 0
    icon = ''
    path = ''
    component = ''
    i18n_key = "menu.codex_smoke_$permissionSuffix"
    code = ''
    sort = 999
    show_menu = 2
  } | ConvertTo-Json -Depth 8
  $permissionCreate = Invoke-RestMethod "$baseURL/api/admin/v1/permissions" `
    -Method Post `
    -Headers $authHeaders `
    -ContentType 'application/json' `
    -Body $permissionBody `
    -TimeoutSec 10

  if ($permissionCreate.code -ne 0 -or $permissionCreate.data.id -le 0) {
    throw "permission create failed: $($permissionCreate | ConvertTo-Json -Depth 8)"
  }

  $createdPermissionID = [int64]$permissionCreate.data.id
  $permissionDelete = Invoke-RestMethod "$baseURL/api/admin/v1/permissions/$createdPermissionID" `
    -Method Delete `
    -Headers $authHeaders `
    -TimeoutSec 10

  if ($permissionDelete.code -ne 0) {
    throw "permission delete failed: $($permissionDelete | ConvertTo-Json -Depth 8)"
  }
  $createdPermissionID = $null

  $logout = Invoke-RestMethod "$baseURL/api/admin/v1/auth/logout" -Method Post -Headers $authHeaders -TimeoutSec 10

  $summary = [ordered]@{
    ready_code = $ready.code
    login_config_code = $loginConfig.code
    captcha_code = $captcha.code
    captcha_type = $captcha.data.captcha_type
    login_code = $login.code
    access_token_present = -not [string]::IsNullOrWhiteSpace($login.data.access_token)
    me_code = $me.code
    init_code = $init.code
    router_count = @($init.data.router).Count
    button_code_count = @($init.data.buttonCodes).Count
    auth_platform_init_code = $authPlatformInit.code
    auth_platform_captcha_dict_count = @($authPlatformInit.data.dict.auth_platform_captcha_type_arr).Count
    auth_platform_list_code = $authPlatformList.code
    auth_platform_count = @($authPlatformList.data.list).Count
    permission_create_code = $permissionCreate.code
    permission_delete_code = $permissionDelete.code
    logout_code = $logout.code
  }

  $completed = $true
  $summary | ConvertTo-Json -Depth 6
} finally {
  if ($createdPermissionID -and $authHeaders -and $baseURL) {
    try {
      Invoke-RestMethod "$baseURL/api/admin/v1/permissions/$createdPermissionID" `
        -Method Delete `
        -Headers $authHeaders `
        -TimeoutSec 5 | Out-Null
    } catch {
      Write-Host "Failed to cleanup smoke permission id=$createdPermissionID"
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
    Write-Host "Smoke logs kept: $outLog $errLog"
  }
}
