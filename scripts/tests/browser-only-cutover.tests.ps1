$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-True([bool]$Condition, [string]$Message) {
  if (-not $Condition) { throw $Message }
}

function Read-RequiredFile([string]$Path) {
  Assert-True (Test-Path -LiteralPath $Path -PathType Leaf) "required file is missing: $Path"
  return [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
}

function Assert-Contains([string]$Content, [string]$Needle, [string]$Message) {
  Assert-True ($Content.Contains($Needle, [StringComparison]::Ordinal)) $Message
}

function Assert-NotMatch([string]$Content, [string]$Pattern, [string]$Message) {
  Assert-True (-not [regex]::IsMatch($Content, $Pattern, [Text.RegularExpressions.RegexOptions]::IgnoreCase)) $Message
}

$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$retirementSQLPath = Join-Path $root 'database\reconciliation\046_retire_client_version_surface.sql'
$verificationSQLPath = Join-Path $root 'database\reconciliation\038_verify_browser_only_retirement.sql'
$revokePath = Join-Path $root 'scripts\browser-only\revoke-admin-sessions.ps1'
$verifyPath = Join-Path $root 'scripts\browser-only\verify-retirement.ps1'
$runbookPath = Join-Path $root 'docs\runbooks\admin-browser-only-cutover.md'

$retirementSQL = Read-RequiredFile $retirementSQLPath
$verificationSQL = Read-RequiredFile $verificationSQLPath
$revoke = Read-RequiredFile $revokePath
$verify = Read-RequiredFile $verifyPath
$runbook = Read-RequiredFile $runbookPath
$reconcile = Read-RequiredFile (Join-Path $root 'scripts\database\reconcile.ps1')
$expanded = Read-RequiredFile (Join-Path $root 'scripts\database\verify-expanded-schema.ps1')
$databaseGate = Read-RequiredFile (Join-Path $root 'scripts\verify-database.ps1')
$basicSmoke = Read-RequiredFile (Join-Path $root 'scripts\basic-admin-smoke.ps1')

foreach ($needle in @(
  '`platform` = ''admin''',
  '`path` = ''/system/clientVersion''',
  '`component` = ''system/clientVersion''',
  '`i18n_key` = ''menu.system_clientVersion''',
  "'system_clientVersion_add'",
  "'system_clientVersion_del'",
  "'system_clientVersion_edit'",
  "'system_clientVersion_forceUpdate'",
  "'system_clientVersion_setLatest'",
  'SET SESSION group_concat_max_len',
  'JSON_ARRAY(id,version,notes,file_url,signature,platform,file_size,is_latest,force_update,is_del,created_at,updated_at)',
  "SIGNAL SQLSTATE '45000'"
)) {
  Assert-Contains $retirementSQL $needle "retirement SQL is missing $needle"
}
$roleUpdate = $retirementSQL.IndexOf('UPDATE `role_permissions`', [StringComparison]::Ordinal)
$permissionUpdate = $retirementSQL.IndexOf('UPDATE `permissions`', [StringComparison]::Ordinal)
Assert-True ($roleUpdate -ge 0 -and $permissionUpdate -gt $roleUpdate) 'role_permissions must be soft-deleted before permissions'
Assert-NotMatch $retirementSQL '(DROP|TRUNCATE)\s+(TABLE\s+)?`?client_versions`?' 'retirement SQL must not drop/truncate client_versions'
Assert-NotMatch $retirementSQL 'DELETE\s+FROM\s+`?client_versions`?' 'retirement SQL must not delete client_versions rows'
Assert-NotMatch $retirementSQL '(UPDATE|INSERT\s+INTO)\s+`?client_versions`?' 'retirement SQL must not mutate client_versions rows'

foreach ($needle in @('browser-only-retirement', '046_retire_client_version_surface.sql')) {
  Assert-Contains $reconcile $needle "reconcile.ps1 is missing $needle"
}
Assert-Contains $expanded '038_verify_browser_only_retirement.sql' 'expanded verification does not run Browser-only invariant SQL'
Assert-Contains $reconcile "'post-contract'" 'reconcile.ps1 is missing the post-contract replay stage'
Assert-Contains $databaseGate "-Stage 'post-contract'" 'database gate does not use the post-contract replay stage'
Assert-Contains $databaseGate 'reconciliationApplied -ne 8' 'database gate does not require eight post-contract first-run reconciliations'
Assert-Contains $databaseGate 'reconciliationSkipped -ne 8' 'database gate does not require eight post-contract repeat-run skips'
Assert-Contains $basicSmoke "if (Test-RoutePath `$init.data.router '/system/clientVersion')" 'basic Admin smoke does not reject the retired client-version route path'
Assert-Contains $basicSmoke "if (Test-RouteViewKey `$init.data.router 'system/clientVersion')" 'basic Admin smoke does not reject the retired client-version view key'
Assert-NotMatch $basicSmoke '(?i)missing canonical client version' 'basic Admin smoke still requires the retired client-version surface'

foreach ($needle in @(
  '[switch]$Apply',
  '[string]$BackendCommit',
  '[string]$FrontendCommit',
  "'^[0-9a-f]{40}$'",
  'git worktree list --porcelain',
  'com.docker.compose.project',
  'com.docker.compose.service',
  'healthy',
  "platform='admin'",
  'UPDATE `user_sessions`',
  'revoked_at=UTC_TIMESTAMP(6)',
  'TOKEN_REDIS_DB',
  'QUEUE_REDIS_DB',
  'FLUSHDB',
  'active_admin_sessions_after',
  'token_redis_keys_after'
)) {
  Assert-Contains $revoke $needle "session revocation script is missing $needle"
}
Assert-NotMatch $revoke '(Write-(Output|Host)|echo).*(APP_SECRET|MYSQL_DSN|Cookie|refresh_token|access_token)' 'revocation script may expose a secret'
Assert-NotMatch $revoke '(single_session|max_sessions)\s*=' 'revocation script must not modify session policy'
Assert-NotMatch $revoke '(UPDATE|DELETE)\s+`?(users|users_login_log|auth_platforms)`?' 'revocation script touches forbidden business tables'

foreach ($needle in @(
  'client_versions_count',
  'client_versions_sha256',
  'active_permission_violations',
  'active_role_permission_violations',
  'max_sessions',
  'token_redis_keys'
)) {
  Assert-Contains $verify $needle "retirement verifier is missing evidence field $needle"
}

foreach ($needle in @(
  'service: cn.zgm2003.admin.refresh',
  'account: current-session',
  'Windows Credential Manager',
  'cannot remotely',
  'client_versions',
  'P09'
)) {
  Assert-Contains $runbook $needle "cutover runbook is missing $needle"
}

foreach ($scriptPath in @($revokePath, $verifyPath)) {
  $redisScript = Read-RequiredFile $scriptPath
  Assert-Contains $redisScript 'if ([string]::IsNullOrEmpty($password))' "$scriptPath must not export an empty REDISCLI_AUTH"
  Assert-Contains $redisScript '& $docker exec $script:RedisContainer redis-cli @Arguments' "$scriptPath must invoke redis-cli directly when Redis has no password"

  $tokens = $null
  $errors = $null
  [Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$errors) | Out-Null
  Assert-True ($errors.Count -eq 0) "$scriptPath has PowerShell syntax errors"
}

Write-Output 'browser-only cutover assertions passed'
