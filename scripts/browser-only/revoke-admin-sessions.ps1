[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[0-9a-f]{40}$')]
  [string]$BackendCommit,

  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[0-9a-f]{40}$')]
  [string]$FrontendCommit,

  [switch]$Apply,

  [string]$DockerCommand = 'docker'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$backendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $backendRoot '..\admin_front_ts'))
$stateCompose = Join-Path $backendRoot 'deploy\docker-state\docker-compose.yml'
$runtimeEnv = Join-Path $backendRoot 'deploy\docker-first\admin-go.env'
$docker = (Get-Command -Name $DockerCommand -ErrorAction Stop | Select-Object -First 1).Source

function Invoke-NativeLines([string]$Executable, [string[]]$Arguments, [string]$Operation) {
  $output = @(& $Executable @Arguments 2>$null | ForEach-Object { $_.ToString() })
  if ($LASTEXITCODE -ne 0) { throw "$Operation failed" }
  return $output
}

function Assert-PrimaryCheckout([string]$Repository, [string]$ExpectedCommit, [string]$Label) {
  $actual = @(Invoke-NativeLines 'git' @('-C', $Repository, 'rev-parse', 'HEAD') "resolve $Label revision")
  if ($actual.Count -ne 1 -or $actual[0] -cne $ExpectedCommit) {
    throw "$Label revision does not match the approved cutover revision"
  }
  # Contract: git worktree list --porcelain must describe exactly one primary checkout.
  $worktrees = @(Invoke-NativeLines 'git' @('-C', $Repository, 'worktree', 'list', '--porcelain') "inspect $Label worktrees")
  $paths = @($worktrees | Where-Object { $_ -like 'worktree *' } | ForEach-Object { $_.Substring('worktree '.Length) })
  if ($paths.Count -ne 1) { throw "$Label must have exactly one checkout" }
  $expectedPath = [IO.Path]::GetFullPath($Repository).TrimEnd('\', '/')
  $actualPath = [IO.Path]::GetFullPath($paths[0]).TrimEnd('\', '/')
  if ($actualPath -cne $expectedPath -or $actualPath -match '(?i)[\\/]\.worktrees(?:[\\/]|$)') {
    throw "$Label checkout is not the primary repository"
  }
}

function Read-RuntimeEnvironment {
  if (-not (Test-Path -LiteralPath $runtimeEnv -PathType Leaf)) {
    throw 'Docker runtime env is missing; run scripts/docker-platform.ps1 init first'
  }
  $values = @{}
  foreach ($raw in [IO.File]::ReadAllLines($runtimeEnv, [Text.Encoding]::UTF8)) {
    $line = $raw.Trim()
    if ($line.Length -eq 0 -or $line.StartsWith('#')) { continue }
    $separator = $line.IndexOf('=')
    if ($separator -le 0) { continue }
    $values[$line.Substring(0, $separator).Trim()] = $line.Substring($separator + 1)
  }
  return $values
}

function Get-RequiredRedisDB([hashtable]$Values, [string]$Name) {
  if (-not $Values.ContainsKey($Name) -or [string]$Values[$Name] -notmatch '^[0-9]{1,2}$') {
    throw "$Name is missing or invalid"
  }
  $value = [int]$Values[$Name]
  if ($value -lt 0 -or $value -gt 15) { throw "$Name is outside Redis DB range" }
  return $value
}

function Get-StateContainer([string]$Service) {
  $ids = @(Invoke-NativeLines $docker @('compose', '-f', $stateCompose, 'ps', '-q', $Service) "resolve $Service container")
  if ($ids.Count -ne 1 -or $ids[0] -notmatch '^[0-9a-f]{64}$') { throw "$Service state container is not uniquely running" }
  $id = $ids[0]
  $project = @(Invoke-NativeLines $docker @('inspect', '--format', '{{index .Config.Labels "com.docker.compose.project"}}', $id) "inspect $Service project")
  $labelService = @(Invoke-NativeLines $docker @('inspect', '--format', '{{index .Config.Labels "com.docker.compose.service"}}', $id) "inspect $Service label")
  $running = @(Invoke-NativeLines $docker @('inspect', '--format', '{{.State.Running}}', $id) "inspect $Service state")
  $health = @(Invoke-NativeLines $docker @('inspect', '--format', '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}', $id) "inspect $Service health")
  if ($project.Count -ne 1 -or $project[0] -cne 'admin-state' -or $labelService.Count -ne 1 -or $labelService[0] -cne $Service) {
    throw "$Service container identity is invalid"
  }
  if ($running.Count -ne 1 -or $running[0] -cne 'true' -or $health.Count -ne 1 -or $health[0] -cne 'healthy') {
    throw "$Service state container is not healthy"
  }
  return $id
}

function Invoke-MySQL([string]$SQL, [string]$Operation) {
  $shell = 'MYSQL_PWD="$(cat /run/secrets/mysql_root_password)" exec mysql --protocol=socket --user=root --default-character-set=utf8mb4 --batch --skip-column-names --raw --database=admin --execute="$1"'
  return @(Invoke-NativeLines $docker @('exec', $script:MySQLContainer, 'sh', '-lc', $shell, 'browser-only', $SQL) $Operation)
}

function Invoke-Redis([string[]]$Arguments, [string]$Operation) {
  $shell = 'IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; exec redis-cli "$@"'
  $password = if ($script:RuntimeValues.ContainsKey('REDIS_PASSWORD')) { [string]$script:RuntimeValues['REDIS_PASSWORD'] } else { '' }
  $output = @($password | & $docker exec -i $script:RedisContainer sh -lc $shell browser-only @Arguments 2>$null | ForEach-Object { $_.ToString() })
  if ($LASTEXITCODE -ne 0) { throw "$Operation failed" }
  return $output
}

function Get-ActiveAdminSessionSnapshot {
  $sql = "SET SESSION group_concat_max_len=67108864; SELECT COUNT(*),SHA2(COALESCE(GROUP_CONCAT(SHA2(CAST(JSON_ARRAY(id,user_id,platform,device_id,refresh_expires_at) AS CHAR),256) ORDER BY id SEPARATOR ''),''),256) FROM user_sessions WHERE platform='admin' AND revoked_at IS NULL AND is_del=2 AND refresh_expires_at>UTC_TIMESTAMP(6)"
  $lines = @(Invoke-MySQL $sql 'read active Admin session snapshot')
  if ($lines.Count -ne 1) { throw 'active Admin session snapshot was invalid' }
  $parts = $lines[0] -split "`t", 2
  if ($parts.Count -ne 2 -or $parts[0] -notmatch '^[0-9]+$' -or $parts[1] -notmatch '^[0-9a-f]{64}$') {
    throw 'active Admin session snapshot was malformed'
  }
  return [pscustomobject]@{ Count = [uint64]$parts[0]; Hash = $parts[1] }
}

function Get-TokenRedisKeyCount {
  $lines = @(Invoke-Redis @('-n', [string]$script:TokenRedisDB, '--raw', 'DBSIZE') 'read token Redis key count')
  if ($lines.Count -ne 1 -or $lines[0] -notmatch '^[0-9]+$') { throw 'token Redis key count was invalid' }
  return [uint64]$lines[0]
}

Assert-PrimaryCheckout $backendRoot $BackendCommit 'backend'
Assert-PrimaryCheckout $frontendRoot $FrontendCommit 'frontend'
$script:RuntimeValues = Read-RuntimeEnvironment
$redisDB = Get-RequiredRedisDB $script:RuntimeValues 'REDIS_DB'
$script:TokenRedisDB = Get-RequiredRedisDB $script:RuntimeValues 'TOKEN_REDIS_DB'
$queueRedisDB = Get-RequiredRedisDB $script:RuntimeValues 'QUEUE_REDIS_DB'
if ($script:TokenRedisDB -eq $redisDB -or $script:TokenRedisDB -eq $queueRedisDB -or $redisDB -eq $queueRedisDB) {
  throw 'REDIS_DB, TOKEN_REDIS_DB, and QUEUE_REDIS_DB must be isolated'
}

$script:MySQLContainer = Get-StateContainer 'mysql'
$script:RedisContainer = Get-StateContainer 'redis'
$before = Get-ActiveAdminSessionSnapshot
$tokenKeysBefore = Get-TokenRedisKeyCount
$revoked = [uint64]0

if ($Apply) {
  $updateSQL = @'
START TRANSACTION;
UPDATE `user_sessions`
SET revoked_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6)
WHERE platform='admin' AND revoked_at IS NULL AND is_del=2 AND refresh_expires_at>UTC_TIMESTAMP(6);
SELECT ROW_COUNT();
COMMIT;
'@
  $updated = @(Invoke-MySQL $updateSQL 'revoke active Admin sessions')
  if ($updated.Count -ne 1 -or $updated[0] -notmatch '^[0-9]+$') { throw 'Admin session revocation count was invalid' }
  $revoked = [uint64]$updated[0]
  $flushed = @(Invoke-Redis @('-n', [string]$script:TokenRedisDB, '--raw', 'FLUSHDB') 'clear isolated token Redis DB')
  if ($flushed.Count -ne 1 -or $flushed[0] -cne 'OK') { throw 'token Redis flush did not return OK' }
}

$after = Get-ActiveAdminSessionSnapshot
$tokenKeysAfter = Get-TokenRedisKeyCount
if ($Apply -and ($after.Count -ne 0 -or $tokenKeysAfter -ne 0)) {
  throw 'Browser-only session cutover did not reach a zero-active state'
}

Write-Output "mode=$(if ($Apply) { 'apply' } else { 'dry-run' })"
Write-Output "backend_commit=$BackendCommit"
Write-Output "frontend_commit=$FrontendCommit"
Write-Output "active_admin_sessions_before=$($before.Count)"
Write-Output "active_admin_sessions_before_sha256=$($before.Hash)"
Write-Output "token_redis_keys_before=$tokenKeysBefore"
Write-Output "revoked_admin_sessions=$revoked"
Write-Output "active_admin_sessions_after=$($after.Count)"
Write-Output "active_admin_sessions_after_sha256=$($after.Hash)"
Write-Output "token_redis_keys_after=$tokenKeysAfter"
Write-Output 'result=passed'
