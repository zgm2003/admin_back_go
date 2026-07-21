$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-True([bool]$Condition, [string]$Message) {
  if (-not $Condition) { throw $Message }
}

function Read-Required([string]$Path) {
  Assert-True (Test-Path -LiteralPath $Path -PathType Leaf) "required contract file is missing: $Path"
  return [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
}

function Assert-Contains([string]$Content, [string]$Needle, [string]$Message) {
  Assert-True ($Content.Contains($Needle, [StringComparison]::Ordinal)) $Message
}

$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$rowPath = Join-Path $root 'database\migrations\202607150201_admin_only_rows.sql'
$schemaPath = Join-Path $root 'database\migrations\202607150202_admin_only_schema.sql'
$constraintPath = Join-Path $root 'database\migrations\202607150203_admin_only_constraints.sql'
$verifyRowsPath = Join-Path $root 'database\reconciliation\051_verify_admin_rows.sql'
$verifySchemaPath = Join-Path $root 'database\reconciliation\052_verify_ai_contract.sql'
$verifyFinalPath = Join-Path $root 'database\reconciliation\053_verify_admin_only.sql'
$preconditionsPath = Join-Path $root 'database\reconciliation\050_contract_preconditions.sql'
$wrapperPath = Join-Path $root 'scripts\database\contract-admin-only.ps1'

$rows = Read-Required $rowPath
$schema = Read-Required $schemaPath
$constraints = Read-Required $constraintPath
$preconditions = Read-Required $preconditionsPath
$verifyRows = Read-Required $verifyRowsPath
$verifySchema = Read-Required $verifySchemaPath
$verifyFinal = Read-Required $verifyFinalPath
$wrapper = Read-Required $wrapperPath

foreach ($needle in @(
  'DELETE FROM `ai_prompts`',
  "'ai_prompt_page'",
  'DELETE FROM `auth_platforms`',
  'DELETE FROM `user_sessions`',
  'DELETE FROM `users_login_log`',
  'DELETE FROM `ai_video_tasks`',
  'DELETE FROM `ai_reply_commands`',
  'system_clientVersion_add',
  'system_clientVersion_del',
  'system_clientVersion_edit',
  'system_clientVersion_forceUpdate',
  'system_clientVersion_setLatest',
  'contract_retired_ai_runs',
  'canvas_text_generate',
  'text_generate'
)) {
  Assert-Contains $rows $needle "row migration is missing $needle"
}
Assert-True (-not [regex]::IsMatch($rows, '(?i)\b(ALTER|TRUNCATE)\b|CREATE\s+TABLE|DROP\s+TABLE')) 'row migration may not contain permanent DDL'
Assert-True (-not $rows.Contains('ROW_COUNT()', [StringComparison]::OrdinalIgnoreCase)) 'row migration must not depend on ROW_COUNT across Atlas statements'

foreach ($needle in @(
  'DROP TABLE `canvas_video_tasks`',
  'DROP TABLE `client_versions`',
  'DROP INDEX `uniq_access_hash`',
  'DROP COLUMN `access_token_hash`'
)) {
  Assert-Contains $schema $needle "schema migration is missing $needle"
}
$dropTables = [regex]::Matches($schema, '(?i)DROP\s+TABLE\s+`([^`]+)`') | ForEach-Object { $_.Groups[1].Value }
Assert-True (($dropTables -join '|') -ceq 'canvas_video_tasks|client_versions') 'schema migration may drop only the two approved tables'
$dropColumns = [regex]::Matches($schema, '(?i)DROP\s+COLUMN\s+`([^`]+)`') | ForEach-Object { $_.Groups[1].Value }
Assert-True (($dropColumns -join '|') -ceq 'access_token_hash') 'schema migration may drop only access_token_hash'
Assert-True (-not $schema.Contains('DROP TABLE `ai_prompts`', [StringComparison]::OrdinalIgnoreCase)) 'ai_prompts table must survive'

foreach ($table in @('auth_platforms','permissions','authz_principal_versions','user_sessions','users_login_log','notification_task','notifications','export_tasks','ai_runs','ai_text_tasks','ai_image_tasks','ai_video_tasks','ai_reply_commands')) {
  Assert-Contains $constraints "ALTER TABLE ``$table``" "constraints do not cover $table"
}
Assert-True (-not [regex]::IsMatch($constraints, "(?i)platform`?\s*=\s*'admin'")) 'constraints must not freeze provenance to Admin'
Assert-Contains $constraints "NOT IN ('app', 'canvas')" 'constraints must permanently reject retired product codes'

foreach ($pair in @(
  @($preconditions, 'client_version_surface_count_mismatch'),
  @($preconditions, "SUM(``kind`` = 'permission') = 6"),
  @($preconditions, "SUM(``kind`` = 'role_permission') = 12"),
  @($verifyRows, 'prompt_rows_remaining'),
  @($verifyRows, 'retired_platform_rows_remaining'),
  @($verifyRows, 'client_version_surface_remaining'),
  @($verifySchema, 'retired_schema_surface_remaining'),
  @($verifySchema, 'access_token_hash_remaining'),
  @($verifyFinal, 'platform_kernel_schema_missing'),
  @($verifyFinal, 'unconfigured_platform_provenance_remaining'),
  @($verifyFinal, 'admin_only_check_constraint_violations')
)) {
  Assert-Contains $pair[0] $pair[1] "verification SQL is missing $($pair[1])"
}

$platformJoinPattern = 'platform_row\.`code`\s+COLLATE\s+utf8mb4_0900_ai_ci\s*=\s*row_data\.`platform`\s+COLLATE\s+utf8mb4_0900_ai_ci'
Assert-True ([regex]::Matches($verifyFinal, $platformJoinPattern).Count -eq 12) 'verification SQL must normalize all platform provenance joins to one collation'

foreach ($needle in @(
  '[string]$ExpectedSourceFingerprint',
  '[string]$InputLock',
  '[string]$DestructiveApproval',
  '[switch]$Apply',
  'admin-db lock-run',
  'Invoke-LockedAtlasSet',
  '--allow-classified-not-found',
  '202607150101',
  '202607150102',
  '050_contract_preconditions.sql',
  '051_verify_admin_rows.sql',
  '052_verify_ai_contract.sql',
  '053_verify_admin_only.sql',
  '202607150201',
  '202607150202',
  '202607150203'
)) {
  Assert-Contains $wrapper $needle "contract wrapper is missing $needle"
}
Assert-True (-not [regex]::IsMatch($wrapper, '(?i)(Write-(Host|Output)|echo).*MYSQL_DSN')) 'wrapper may not print MYSQL_DSN'

$tokens = $null
$errors = $null
[Management.Automation.Language.Parser]::ParseFile($wrapperPath, [ref]$tokens, [ref]$errors) | Out-Null
Assert-True ($errors.Count -eq 0) 'contract wrapper has PowerShell syntax errors'

Write-Output 'admin-only contract assertions passed'
