[CmdletBinding()]
param([string]$DockerCommand = 'docker')

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$backendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$stateCompose = Join-Path $backendRoot 'deploy\docker-state\docker-compose.yml'
$runtimeEnv = Join-Path $backendRoot 'deploy\docker-first\admin-go.env'
$docker = (Get-Command -Name $DockerCommand -ErrorAction Stop | Select-Object -First 1).Source

function Invoke-NativeLines([string]$Executable, [string[]]$Arguments, [string]$Operation) {
  $output = @(& $Executable @Arguments 2>$null | ForEach-Object { $_.ToString() })
  if ($LASTEXITCODE -ne 0) { throw "$Operation failed" }
  return $output
}

function Read-RuntimeEnvironment {
  if (-not (Test-Path -LiteralPath $runtimeEnv -PathType Leaf)) { throw 'Docker runtime env is missing' }
  $values = @{}
  foreach ($raw in [IO.File]::ReadAllLines($runtimeEnv, [Text.Encoding]::UTF8)) {
    $line = $raw.Trim()
    if ($line.Length -eq 0 -or $line.StartsWith('#')) { continue }
    $separator = $line.IndexOf('=')
    if ($separator -gt 0) { $values[$line.Substring(0, $separator).Trim()] = $line.Substring($separator + 1) }
  }
  return $values
}

function Get-StateContainer([string]$Service) {
  $ids = @(Invoke-NativeLines $docker @('compose', '-f', $stateCompose, 'ps', '-q', $Service) "resolve $Service container")
  if ($ids.Count -ne 1 -or $ids[0] -notmatch '^[0-9a-f]{64}$') { throw "$Service state container is not uniquely running" }
  $id = $ids[0]
  $project = @(Invoke-NativeLines $docker @('inspect', '--format', '{{index .Config.Labels "com.docker.compose.project"}}', $id) "inspect $Service project")
  $labelService = @(Invoke-NativeLines $docker @('inspect', '--format', '{{index .Config.Labels "com.docker.compose.service"}}', $id) "inspect $Service label")
  $health = @(Invoke-NativeLines $docker @('inspect', '--format', '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}', $id) "inspect $Service health")
  if ($project.Count -ne 1 -or $project[0] -cne 'admin-state' -or $labelService.Count -ne 1 -or $labelService[0] -cne $Service -or $health.Count -ne 1 -or $health[0] -cne 'healthy') {
    throw "$Service state container identity or health is invalid"
  }
  return $id
}

function Invoke-MySQL([string]$SQL, [string]$Operation) {
  $shell = 'MYSQL_PWD="$(cat /run/secrets/mysql_root_password)" exec mysql --protocol=socket --user=root --default-character-set=utf8mb4 --batch --skip-column-names --raw --database=admin --execute="$1"'
  return @(Invoke-NativeLines $docker @('exec', $script:MySQLContainer, 'sh', '-lc', $shell, 'browser-only', $SQL) $Operation)
}

function Invoke-Redis([string[]]$Arguments, [string]$Operation) {
  $password = if ($script:RuntimeValues.ContainsKey('REDIS_PASSWORD')) { [string]$script:RuntimeValues['REDIS_PASSWORD'] } else { '' }
  if ([string]::IsNullOrEmpty($password)) {
    $output = @(& $docker exec $script:RedisContainer redis-cli @Arguments 2>$null | ForEach-Object { $_.ToString() })
  } else {
    $shell = 'IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; exec redis-cli "$@"'
    $output = @($password | & $docker exec -i $script:RedisContainer sh -lc $shell browser-only @Arguments 2>$null | ForEach-Object { $_.ToString() })
  }
  if ($LASTEXITCODE -ne 0) { throw "$Operation failed" }
  return $output
}

function Get-SingleInteger([string]$SQL, [string]$Operation) {
  $lines = @(Invoke-MySQL $SQL $Operation)
  if ($lines.Count -ne 1 -or $lines[0] -notmatch '^[0-9]+$') { throw "$Operation returned invalid evidence" }
  return [uint64]$lines[0]
}

$script:RuntimeValues = Read-RuntimeEnvironment
foreach ($name in @('REDIS_DB', 'TOKEN_REDIS_DB', 'QUEUE_REDIS_DB')) {
  if (-not $script:RuntimeValues.ContainsKey($name) -or [string]$script:RuntimeValues[$name] -notmatch '^[0-9]{1,2}$') { throw "$name is invalid" }
}
$redisDB = [int]$script:RuntimeValues['REDIS_DB']
$tokenRedisDB = [int]$script:RuntimeValues['TOKEN_REDIS_DB']
$queueRedisDB = [int]$script:RuntimeValues['QUEUE_REDIS_DB']
if ($tokenRedisDB -eq $redisDB -or $tokenRedisDB -eq $queueRedisDB -or $redisDB -eq $queueRedisDB) { throw 'Redis DB isolation is invalid' }

$script:MySQLContainer = Get-StateContainer 'mysql'
$script:RedisContainer = Get-StateContainer 'redis'

$selector = "p.platform='admin' AND (p.path='/system/clientVersion' OR p.component='system/clientVersion' OR p.i18n_key='menu.system_clientVersion' OR p.code IN ('system_clientVersion_add','system_clientVersion_del','system_clientVersion_edit','system_clientVersion_forceUpdate','system_clientVersion_setLatest'))"
$activePermissionViolations = Get-SingleInteger "SELECT COUNT(*) FROM permissions p WHERE p.is_del=2 AND $selector" 'count active retired permissions'
$activeRolePermissionViolations = Get-SingleInteger "SELECT COUNT(*) FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id WHERE rp.is_del=2 AND $selector" 'count active retired role grants'
$activeAdminSessions = Get-SingleInteger "SELECT COUNT(*) FROM user_sessions WHERE platform='admin' AND revoked_at IS NULL AND is_del=2 AND refresh_expires_at>UTC_TIMESTAMP(6)" 'count active Admin sessions'
$reconciliationRuns = Get-SingleInteger "SELECT COUNT(*) FROM schema_reconciliation_runs WHERE script_name='046_retire_client_version_surface.sql' AND status='succeeded'" 'count retirement reconciliation runs'

$historySQL = "SET SESSION group_concat_max_len=67108864; SELECT COUNT(*),SHA2(COALESCE(GROUP_CONCAT(SHA2(CAST(JSON_ARRAY(id,version,notes,file_url,signature,platform,file_size,is_latest,force_update,is_del,created_at,updated_at) AS CHAR),256) ORDER BY id SEPARATOR ''),''),256) FROM client_versions"
$history = @(Invoke-MySQL $historySQL 'read client_versions history evidence')
if ($history.Count -ne 1) { throw 'client_versions history evidence was invalid' }
$historyParts = $history[0] -split "`t", 2
if ($historyParts.Count -ne 2 -or $historyParts[0] -notmatch '^[0-9]+$' -or $historyParts[1] -notmatch '^[0-9a-f]{64}$') {
  throw 'client_versions history evidence was malformed'
}

$policy = @(Invoke-MySQL "SELECT COUNT(*),COALESCE(MAX(single_session),0),COALESCE(MAX(max_sessions),0) FROM auth_platforms WHERE code='admin' AND is_del=2" 'read Admin session policy')
if ($policy.Count -ne 1) { throw 'Admin session policy evidence was invalid' }
$policyParts = $policy[0] -split "`t"
if ($policyParts.Count -ne 3 -or $policyParts[0] -cne '1' -or $policyParts[1] -cne '1' -or $policyParts[2] -cne '1') {
  throw 'Admin single_session/max_sessions policy changed'
}

$redisLines = @(Invoke-Redis @('-n', [string]$tokenRedisDB, '--raw', 'DBSIZE') 'read token Redis key count')
if ($redisLines.Count -ne 1 -or $redisLines[0] -notmatch '^[0-9]+$') { throw 'token Redis evidence was invalid' }
$tokenRedisKeys = [uint64]$redisLines[0]

if ($activePermissionViolations -ne 0 -or $activeRolePermissionViolations -ne 0 -or $activeAdminSessions -ne 0 -or $tokenRedisKeys -ne 0 -or $reconciliationRuns -lt 1) {
  throw 'Browser-only retirement verification failed'
}

Write-Output "active_permission_violations=$activePermissionViolations"
Write-Output "active_role_permission_violations=$activeRolePermissionViolations"
Write-Output "client_versions_count=$($historyParts[0])"
Write-Output "client_versions_sha256=$($historyParts[1])"
Write-Output "single_session=$($policyParts[1])"
Write-Output "max_sessions=$($policyParts[2])"
Write-Output "active_admin_sessions=$activeAdminSessions"
Write-Output "token_redis_keys=$tokenRedisKeys"
Write-Output "retirement_reconciliation_runs=$reconciliationRuns"
Write-Output 'result=passed'
