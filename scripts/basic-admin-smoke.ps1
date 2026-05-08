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

function Assert-ApiOK($Response, [string]$Label) {
  if ($Response.code -ne 0) {
    throw "$Label failed: $($Response | ConvertTo-Json -Depth 12)"
  }
}

function New-SmokePermission([string]$BaseURL, [hashtable]$Headers, [hashtable]$Body, [string]$Label) {
  $response = Invoke-RestMethod "$BaseURL/api/admin/v1/permissions" `
    -Method Post `
    -Headers $Headers `
    -ContentType 'application/json' `
    -Body ($Body | ConvertTo-Json -Depth 8) `
    -TimeoutSec 10

  Assert-ApiOK $response $Label
  if ($response.data.id -le 0) {
    throw "$Label returned invalid id: $($response | ConvertTo-Json -Depth 12)"
  }
  return [int64]$response.data.id
}

function Get-ObjectArray($Value) {
  if ($null -eq $Value) { return @() }
  return @($Value)
}

function Get-Int64Array($Value) {
  $result = New-Object System.Collections.Generic.List[Int64]
  foreach ($item in (Get-ObjectArray $Value)) {
    if ($null -eq $item) { continue }
    $result.Add([int64]$item)
  }
  return @($result.ToArray())
}

function Merge-Int64Array([Int64[]]$Left, [Int64[]]$Right) {
  $seen = @{}
  $result = New-Object System.Collections.Generic.List[Int64]
  foreach ($item in @($Left + $Right)) {
    if ($item -le 0) { continue }
    $key = [string]$item
    if ($seen.ContainsKey($key)) { continue }
    $seen[$key] = $true
    $result.Add($item)
  }
  return @($result.ToArray() | Sort-Object)
}

function Test-RoutePath($Routes, [string]$Path) {
  foreach ($route in (Get-ObjectArray $Routes)) {
    if ($route.path -eq $Path) { return $true }
  }
  return $false
}

function Test-RouteViewKey($Routes, [string]$ViewKey) {
  foreach ($route in (Get-ObjectArray $Routes)) {
    if ($route.view_key -eq $ViewKey) { return $true }
  }
  return $false
}

function Test-StringListContains($Values, [string]$Expected) {
  foreach ($value in (Get-ObjectArray $Values)) {
    if ([string]$value -eq $Expected) { return $true }
  }
  return $false
}



Import-DotEnv (Join-Path $BackendRoot '.env')

if ([string]::IsNullOrWhiteSpace($Account) -or [string]::IsNullOrWhiteSpace($Password)) {
  throw 'Set SMOKE_LOGIN_ACCOUNT and SMOKE_LOGIN_PASSWORD, or pass -Account and -Password.'
}

Assert-PortFree $HTTPAddr

New-Item -ItemType Directory -Force .tmp | Out-Null
$serverExe = '.tmp/admin-api-smoke.exe'
$workerExe = '.tmp/admin-worker-smoke.exe'
$secretReader = '.tmp/read-captcha-secret.go'
$loginLogReader = '.tmp/read-login-log-count.go'
$realtimeSmoke = '.tmp/realtime-smoke.go'
$outLog = '.tmp/basic-admin-smoke-out.log'
$errLog = '.tmp/basic-admin-smoke-err.log'
$workerOutLog = '.tmp/basic-admin-worker-smoke-out.log'
$workerErrLog = '.tmp/basic-admin-worker-smoke-err.log'
$completed = $false
$authHeaders = $null
$baseURL = $null
$workerProc = $null
$createdPermissionIDs = New-Object System.Collections.Generic.List[Int64]
$smokeRole = $null
$originalRolePermissionIDs = $null
$roleRestored = $false

Remove-Item -Force $serverExe, $workerExe, $secretReader, $loginLogReader, $realtimeSmoke, $outLog, $errLog, $workerOutLog, $workerErrLog -ErrorAction SilentlyContinue

go build -o $serverExe ./cmd/admin-api
go build -o $workerExe ./cmd/admin-worker

$env:HTTP_ADDR = $HTTPAddr
$workerProc = Start-Process -FilePath (Resolve-Path $workerExe) `
  -PassThru `
  -WindowStyle Hidden `
  -RedirectStandardOutput $workerOutLog `
  -RedirectStandardError $workerErrLog

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

  $verifyCode = if ([string]::IsNullOrWhiteSpace($env:VERIFY_CODE_DEV_CODE)) { '123456' } else { $env:VERIFY_CODE_DEV_CODE }
  $sendCodeBody = @{
    account = $Account
    scene = 'login'
  } | ConvertTo-Json -Depth 4
  $sendCode = Invoke-RestMethod "$baseURL/api/admin/v1/auth/send-code" `
    -Method Post `
    -ContentType 'application/json' `
    -Body $sendCodeBody `
    -TimeoutSec 10

  if ($sendCode.code -ne 0) {
    throw "send-code failed: $($sendCode | ConvertTo-Json -Depth 8)"
  }

  $codeLoginType = if ($Account -match '^[^@\s]+@[^@\s]+\.[^@\s]+$') { 'email' } else { 'phone' }
  $codeLoginBody = @{
    login_account = $Account
    login_type = $codeLoginType
    code = $verifyCode
  } | ConvertTo-Json -Depth 4
  $codeLogin = Invoke-RestMethod "$baseURL/api/admin/v1/auth/login" `
    -Method Post `
    -Headers @{ platform = $Platform; 'device-id' = "$($DeviceID)-code" } `
    -ContentType 'application/json' `
    -Body $codeLoginBody `
    -TimeoutSec 10

  if ($codeLogin.code -ne 0 -or [string]::IsNullOrWhiteSpace($codeLogin.data.access_token)) {
    throw "code login failed: $($codeLogin | ConvertTo-Json -Depth 8)"
  }

  Invoke-RestMethod "$baseURL/api/admin/v1/auth/logout" `
    -Method Post `
    -Headers @{ platform = $Platform; 'device-id' = "$($DeviceID)-code"; Authorization = "Bearer $($codeLogin.data.access_token)" } `
    -TimeoutSec 10 | Out-Null

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

  @"
package main

import (
  "encoding/json"
  "fmt"
  "net/http"
  "net/url"
  "os"
  "strings"
  "time"

  "github.com/gorilla/websocket"
)

type Envelope struct {
  Type      string         ``json:"type"``
  RequestID string         ``json:"request_id,omitempty"``
  Data      map[string]any ``json:"data"``
}

func main() {
  if len(os.Args) != 5 {
    fmt.Fprintln(os.Stderr, "usage: realtime-smoke <base-url> <access-token> <platform> <device-id>")
    os.Exit(2)
  }

  base, err := url.Parse(os.Args[1])
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(2)
  }
  switch base.Scheme {
  case "https":
    base.Scheme = "wss"
  default:
    base.Scheme = "ws"
  }
  base.Path = "/api/admin/v1/realtime/ws"
  base.RawQuery = ""

  headers := http.Header{}
  headers.Set("Authorization", "Bearer "+os.Args[2])
  headers.Set("platform", os.Args[3])
  headers.Set("device-id", os.Args[4])

  conn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial(base.String(), headers)
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  defer conn.Close()

  if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }

  var connected Envelope
  if err := conn.ReadJSON(&connected); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  if connected.Type != "realtime.connected.v1" || connected.Data["platform"] != os.Args[3] {
    b, _ := json.Marshal(connected)
    fmt.Fprintf(os.Stderr, "unexpected connected event: %s\n", b)
    os.Exit(1)
  }

  if err := conn.WriteJSON(Envelope{
    Type:      "realtime.ping.v1",
    RequestID: "smoke-realtime-ping",
    Data:      map[string]any{},
  }); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }

  var pong Envelope
  if err := conn.ReadJSON(&pong); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  if pong.Type != "realtime.pong.v1" || pong.RequestID != "smoke-realtime-ping" || strings.TrimSpace(fmt.Sprint(pong.Data["server_time"])) == "" {
    b, _ := json.Marshal(pong)
    fmt.Fprintf(os.Stderr, "unexpected pong event: %s\n", b)
    os.Exit(1)
  }

  summary := map[string]any{
    "connected_type": connected.Type,
    "pong_type": pong.Type,
    "heartbeat_interval_ms": connected.Data["heartbeat_interval_ms"],
  }
  if err := json.NewEncoder(os.Stdout).Encode(summary); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
}
"@ | Set-Content -LiteralPath $realtimeSmoke -Encoding UTF8

  $realtimeOutput = go run $realtimeSmoke $baseURL $login.data.access_token $Platform $DeviceID 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "realtime websocket smoke failed: $($realtimeOutput | Out-String)"
  }
  $realtime = ($realtimeOutput | Out-String).Trim() | ConvertFrom-Json

  @"
package main

import (
  "context"
  "database/sql"
  "fmt"
  "os"
  "time"

  "admin_back_go/internal/config"

  _ "github.com/go-sql-driver/mysql"
)

func main() {
  if len(os.Args) != 3 {
    fmt.Fprintln(os.Stderr, "usage: read-login-log-count <account> <platform>")
    os.Exit(2)
  }

  cfg := config.Load()
  if cfg.MySQL.DSN == "" {
    fmt.Fprintln(os.Stderr, "MYSQL_DSN is empty")
    os.Exit(2)
  }

  db, err := sql.Open("mysql", cfg.MySQL.DSN)
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  defer db.Close()

  ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
  defer cancel()

  var count int
  query := "SELECT COUNT(*) FROM users_login_log WHERE login_account = ? AND platform = ? AND is_del = 2 AND created_at >= DATE_SUB(NOW(), INTERVAL 5 MINUTE)"
  err = db.QueryRowContext(ctx, query, os.Args[1], os.Args[2]).Scan(&count)
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  fmt.Print(count)
}
"@ | Set-Content -LiteralPath $loginLogReader -Encoding UTF8

  $loginLogCount = 0
  for ($i = 0; $i -lt 20; $i++) {
    $loginLogCount = [int](go run $loginLogReader $Account $Platform)
    if ($loginLogCount -ge 2) { break }
    Start-Sleep -Milliseconds 300
  }
  if ($loginLogCount -lt 2) {
    throw "login log count is too low, expected >= 2 within 5 minutes, got $loginLogCount"
  }

  $me = Invoke-RestMethod "$baseURL/api/admin/v1/users/me" -Headers $authHeaders -TimeoutSec 10
  $init = Invoke-RestMethod "$baseURL/api/admin/v1/users/init" -Headers $authHeaders -TimeoutSec 10
  $usersPageInit = Invoke-RestMethod "$baseURL/api/admin/v1/users/page-init" -Headers $authHeaders -TimeoutSec 10
  $usersList = Invoke-RestMethod "$baseURL/api/admin/v1/users?current_page=1&page_size=10" -Headers $authHeaders -TimeoutSec 10
  $authPlatformInit = Invoke-RestMethod "$baseURL/api/admin/v1/auth-platforms/init" -Headers $authHeaders -TimeoutSec 10
  $authPlatformList = Invoke-RestMethod "$baseURL/api/admin/v1/auth-platforms?current_page=1&page_size=50" -Headers $authHeaders -TimeoutSec 10
  Assert-ApiOK $init 'users init'
  Assert-ApiOK $usersPageInit 'users page-init'
  Assert-ApiOK $usersList 'users list'
  if (-not (Test-RoutePath $init.data.router '/system/clientVersion')) {
    throw 'users init missing canonical client version route path /system/clientVersion; run database migration 20260507_client_version_permission_route_cleanup.sql'
  }
  if (-not (Test-RouteViewKey $init.data.router 'system/clientVersion')) {
    throw 'users init missing canonical client version view_key system/clientVersion; run database migration 20260507_client_version_permission_route_cleanup.sql'
  }
  if (Test-RoutePath $init.data.router '/ai/goods') {
    throw 'users init still returns retired AI goods route /ai/goods; run database migration 20260508_remove_ai_goods_cine_modules.sql and clear operator-side caches'
  }
  if (Test-RoutePath $init.data.router '/ai/cine') {
    throw 'users init still returns retired AI cine route /ai/cine; run database migration 20260508_remove_ai_goods_cine_modules.sql and clear operator-side caches'
  }
  if (-not (Test-RoutePath $init.data.router '/ai/models')) {
    throw 'users init missing retained AI core route /ai/models'
  }
  if (-not (Test-RoutePath $init.data.router '/ai/chat')) {
    throw 'users init missing retained AI core route /ai/chat'
  }
  $permissionSuffix = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
  $dirBody = @{
    platform = $Platform
    type = 1
    name = "Codex Smoke DIR $permissionSuffix"
    parent_id = 0
    icon = ''
    path = ''
    component = ''
    i18n_key = "menu.codex_smoke_dir_$permissionSuffix"
    code = ''
    sort = 999
    show_menu = 2
  }
  $dirPermissionID = New-SmokePermission $baseURL $authHeaders $dirBody 'permission dir create'
  $createdPermissionIDs.Add($dirPermissionID) | Out-Null

  $smokePagePath = "/codex-smoke/$permissionSuffix/page"
  $pageBody = @{
    platform = $Platform
    type = 2
    name = "Codex Smoke PAGE $permissionSuffix"
    parent_id = $dirPermissionID
    icon = ''
    path = $smokePagePath
    component = "/codex-smoke/$permissionSuffix/page/index"
    i18n_key = "menu.codex_smoke_page_$permissionSuffix"
    code = ''
    sort = 999
    show_menu = 2
  }
  $pagePermissionID = New-SmokePermission $baseURL $authHeaders $pageBody 'permission page create'
  $createdPermissionIDs.Add($pagePermissionID) | Out-Null

  $smokeButtonCode = "codex_smoke_button_$permissionSuffix"
  $buttonBody = @{
    platform = $Platform
    type = 3
    name = "Codex Smoke BUTTON $permissionSuffix"
    parent_id = $pagePermissionID
    icon = ''
    path = ''
    component = ''
    i18n_key = ''
    code = $smokeButtonCode
    sort = 999
    show_menu = 2
  }
  $buttonPermissionID = New-SmokePermission $baseURL $authHeaders $buttonBody 'permission button create'
  $createdPermissionIDs.Add($buttonPermissionID) | Out-Null

  $roleName = [uri]::EscapeDataString([string]$me.data.role_name)
  $roleList = Invoke-RestMethod "$baseURL/api/admin/v1/roles?current_page=1&page_size=50&name=$roleName" -Headers $authHeaders -TimeoutSec 10
  Assert-ApiOK $roleList 'role list'
  $smokeRole = @(Get-ObjectArray $roleList.data.list | Where-Object { $_.name -eq $me.data.role_name } | Select-Object -First 1)[0]
  if ($null -eq $smokeRole) {
    throw "cannot find smoke account role by name: $($me.data.role_name)"
  }
  $originalRolePermissionIDs = Get-Int64Array $smokeRole.permission_id
  $nextRolePermissionIDs = Merge-Int64Array $originalRolePermissionIDs @($pagePermissionID, $buttonPermissionID)
  $roleUpdateBody = @{
    name = $smokeRole.name
    permission_id = $nextRolePermissionIDs
  } | ConvertTo-Json -Depth 8
  $roleUpdate = Invoke-RestMethod "$baseURL/api/admin/v1/roles/$($smokeRole.id)" `
    -Method Put `
    -Headers $authHeaders `
    -ContentType 'application/json' `
    -Body $roleUpdateBody `
    -TimeoutSec 10
  Assert-ApiOK $roleUpdate 'role permission grant'

  $rbacInit = Invoke-RestMethod "$baseURL/api/admin/v1/users/init" -Headers $authHeaders -TimeoutSec 10
  Assert-ApiOK $rbacInit 'users init after rbac grant'
  if (-not (Test-RoutePath $rbacInit.data.router $smokePagePath)) {
    throw "RBAC smoke route missing after page/button grant: $smokePagePath"
  }
  if (-not (Test-StringListContains $rbacInit.data.buttonCodes $smokeButtonCode)) {
    throw "RBAC smoke button code missing after grant: $smokeButtonCode"
  }

  $restoreBody = @{
    name = $smokeRole.name
    permission_id = $originalRolePermissionIDs
  } | ConvertTo-Json -Depth 8
  $roleRestore = Invoke-RestMethod "$baseURL/api/admin/v1/roles/$($smokeRole.id)" `
    -Method Put `
    -Headers $authHeaders `
    -ContentType 'application/json' `
    -Body $restoreBody `
    -TimeoutSec 10
  Assert-ApiOK $roleRestore 'role permission restore'
  $roleRestored = $true

  $permissionDeleteBody = @{ ids = @($createdPermissionIDs.ToArray()) } | ConvertTo-Json -Depth 8
  $permissionDelete = Invoke-RestMethod "$baseURL/api/admin/v1/permissions" `
    -Method Delete `
    -Headers $authHeaders `
    -ContentType 'application/json' `
    -Body $permissionDeleteBody `
    -TimeoutSec 10
  Assert-ApiOK $permissionDelete 'permission subtree delete'
  $createdPermissionIDs.Clear()

  $logout = Invoke-RestMethod "$baseURL/api/admin/v1/auth/logout" -Method Post -Headers $authHeaders -TimeoutSec 10

  $summary = [ordered]@{
    ready_code = $ready.code
    login_config_code = $loginConfig.code
    login_config_types = (@($loginConfig.data.login_type_arr) | ForEach-Object { $_.value }) -join ','
    send_code_code = $sendCode.code
    code_login_code = $codeLogin.code
    code_login_type = $codeLoginType
    captcha_code = $captcha.code
    captcha_type = $captcha.data.captcha_type
    login_code = $login.code
    access_token_present = -not [string]::IsNullOrWhiteSpace($login.data.access_token)
    login_log_count = $loginLogCount
    realtime_connected_type = $realtime.connected_type
    realtime_pong_type = $realtime.pong_type
    realtime_heartbeat_interval_ms = $realtime.heartbeat_interval_ms
    me_code = $me.code
    init_code = $init.code
    router_count = @($init.data.router).Count
    button_code_count = @($init.data.buttonCodes).Count
    ai_goods_route_present = Test-RoutePath $init.data.router '/ai/goods'
    ai_cine_route_present = Test-RoutePath $init.data.router '/ai/cine'
    ai_models_route_present = Test-RoutePath $init.data.router '/ai/models'
    ai_chat_route_present = Test-RoutePath $init.data.router '/ai/chat'
    users_page_init_code = $usersPageInit.code
    users_role_dict_count = @($usersPageInit.data.dict.roleArr).Count
    users_address_tree_count = @($usersPageInit.data.dict.auth_address_tree).Count
    users_list_code = $usersList.code
    users_list_count = @($usersList.data.list).Count
    auth_platform_init_code = $authPlatformInit.code
    auth_platform_captcha_dict_count = @($authPlatformInit.data.dict.auth_platform_captcha_type_arr).Count
    auth_platform_list_code = $authPlatformList.code
    auth_platform_count = @($authPlatformList.data.list).Count
    rbac_smoke_role_id = $smokeRole.id
    rbac_smoke_page_route_present = Test-RoutePath $rbacInit.data.router $smokePagePath
    rbac_smoke_button_code_present = Test-StringListContains $rbacInit.data.buttonCodes $smokeButtonCode
    rbac_smoke_role_restored = $roleRestored
    permission_create_code = 0
    permission_delete_code = $permissionDelete.code
    logout_code = $logout.code
  }

  $completed = $true
  $summary | ConvertTo-Json -Depth 6
} finally {
  if (-not $roleRestored -and $smokeRole -and $originalRolePermissionIDs -and $authHeaders -and $baseURL) {
    try {
      $restoreBody = @{
        name = $smokeRole.name
        permission_id = $originalRolePermissionIDs
      } | ConvertTo-Json -Depth 8
      Invoke-RestMethod "$baseURL/api/admin/v1/roles/$($smokeRole.id)" `
        -Method Put `
        -Headers $authHeaders `
        -ContentType 'application/json' `
        -Body $restoreBody `
        -TimeoutSec 5 | Out-Null
    } catch {
      Write-Host "Failed to restore smoke role id=$($smokeRole.id)"
    }
  }

  if ($createdPermissionIDs.Count -gt 0 -and $authHeaders -and $baseURL) {
    try {
      $cleanupBody = @{ ids = @($createdPermissionIDs.ToArray()) } | ConvertTo-Json -Depth 8
      Invoke-RestMethod "$baseURL/api/admin/v1/permissions" `
        -Method Delete `
        -Headers $authHeaders `
        -ContentType 'application/json' `
        -Body $cleanupBody `
        -TimeoutSec 5 | Out-Null
    } catch {
      Write-Host "Failed to cleanup smoke permission ids=$($createdPermissionIDs.ToArray() -join ',')"
    }
  }

  if ($proc -and -not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force
  }
  if ($workerProc -and -not $workerProc.HasExited) {
    Stop-Process -Id $workerProc.Id -Force
  }

  Start-Sleep -Milliseconds 300
  Remove-Item -Force $serverExe, $workerExe, $secretReader, $loginLogReader, $realtimeSmoke -ErrorAction SilentlyContinue

  if ($completed) {
    Remove-Item -Force $outLog, $errLog, $workerOutLog, $workerErrLog -ErrorAction SilentlyContinue
  } else {
    Write-Host "Smoke logs kept: $outLog $errLog $workerOutLog $workerErrLog"
  }
}
