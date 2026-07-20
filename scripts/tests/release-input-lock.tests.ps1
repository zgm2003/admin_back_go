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
$schemaPath = Join-Path $root 'release\admin-only\input-lock.schema.json'
$lockPath = Join-Path $root 'release\admin-only\input-lock.json'
$lockScriptPath = Join-Path $root 'scripts\release\lock-inputs.ps1'
$checkScriptPath = Join-Path $root 'scripts\release\check-inputs.ps1'
$preconditionsPath = Join-Path $root 'database\reconciliation\050_contract_preconditions.sql'
$runbookPath = Join-Path $root 'docs\runbooks\admin-only-data-disposition.md'
$gitignorePath = Join-Path $root 'release\admin-only\.gitignore'

$schemaText = Read-RequiredFile $schemaPath
$lockScript = Read-RequiredFile $lockScriptPath
$checkScript = Read-RequiredFile $checkScriptPath
$preconditions = Read-RequiredFile $preconditionsPath
$runbook = Read-RequiredFile $runbookPath
$gitignore = Read-RequiredFile $gitignorePath

$schema = $schemaText | ConvertFrom-Json
Assert-True ([int]$schema.properties.schema_version.const -eq 1) 'input lock schema_version must be one'
Assert-True ($schema.additionalProperties -eq $false) 'input lock schema must reject additional properties'
$required = @($schema.required)
$expectedFields = @(
  'schema_version',
  'backend_commit',
  'frontend_commit',
  'contract_manifest_sha256',
  'database_fingerprint_sha256',
  'recovery_artifact_sha256',
  'cos_disposition_evidence_sha256',
  'query_evidence_sha256',
  'client_versions_freeze_evidence_sha256'
)
Assert-True (($required -join '|') -ceq ($expectedFields -join '|')) 'input lock required fields/order changed'

$valid = [ordered]@{schema_version=1}
foreach ($field in $expectedFields | Select-Object -Skip 1) {
  $valid[$field] = if ($field.EndsWith('_commit', [StringComparison]::Ordinal)) { 'a' * 40 } else { 'b' * 64 }
}
$validJSON = $valid | ConvertTo-Json -Compress
Assert-True ($validJSON | Test-Json -SchemaFile $schemaPath) 'schema rejected a valid input lock'
$invalid = [ordered]@{} + $valid
$invalid.backend_commit = 'ABC'
Assert-True (-not (($invalid | ConvertTo-Json -Compress) | Test-Json -SchemaFile $schemaPath -ErrorAction SilentlyContinue)) 'schema accepted an invalid Git SHA'
$invalid = [ordered]@{} + $valid
$invalid.extra = 'forbidden'
Assert-True (-not (($invalid | ConvertTo-Json -Compress) | Test-Json -SchemaFile $schemaPath -ErrorAction SilentlyContinue)) 'schema accepted an extra property'

$inputScripts = $lockScript + "`n" + $checkScript
foreach ($needle in @(
  '[string]$DatabaseFingerprint',
  '[string]$RecoveryArtifact',
  '[string]$CosDispositionEvidence',
  '[string]$QueryEvidence',
  '[string]$ClientVersionsFreezeEvidence',
  'git worktree list --porcelain',
  'git status --porcelain=v1 --untracked-files=all',
  'Assert-RecoveryArtifact',
  'Assert-COSDispositionEvidence',
  'Assert-QueryEvidence',
  'Assert-ClientVersionsFreezeEvidence',
  'Move-Item -LiteralPath $temporaryPath -Destination $outputPath',
  '[ordered]@{'
)) {
  Assert-Contains $inputScripts $needle "input lock scripts are missing $needle"
}
Assert-NotMatch $lockScript '(Write-(Output|Host)|echo).*(MYSQL_DSN|APP_SECRET|refresh_token|access_token|Cookie)' 'lock-inputs.ps1 may print a secret'

foreach ($needle in @(
  '[switch]$SchemaOnly',
  'Test-Json -SchemaFile',
  'git merge-base --is-ancestor',
  'contract_manifest_sha256',
  'database_fingerprint_sha256',
  'recovery_artifact_sha256',
  'cos_disposition_evidence_sha256',
  'query_evidence_sha256',
  'client_versions_freeze_evidence_sha256'
)) {
  Assert-Contains $checkScript $needle "check-inputs.ps1 is missing $needle"
}
Assert-NotMatch $checkScript '(Get-Content|Write-(Output|Host)).*(dump_path|MYSQL_DSN|APP_SECRET|refresh_token|access_token|Cookie)' 'check-inputs.ps1 may expose evidence or a secret'

foreach ($needle in @(
  'active_retired_session_violations',
  'unknown_platform_violations',
  'unmapped_scene_violations',
  'nonterminal_durable_work_violations',
  'client_version_surface_violations',
  'client_versions_count_mismatch',
  'client_versions_hash_mismatch',
  'wallet_balance_violations',
  'orphan_relationship_violations'
)) {
  Assert-Contains $preconditions $needle "precondition SQL is missing $needle"
}
Assert-NotMatch $preconditions '(DROP|TRUNCATE|DELETE|UPDATE|INSERT|ALTER)\s+' 'precondition SQL must be read-only'

foreach ($needle in @(
  'App/Canvas sessions and login attempts',
  'users_quick_entry',
  'notification_task',
  'export_tasks',
  'client_versions',
  'canvas_text_generate',
  'text_generate',
  'No COS delete operation',
  'already missing',
  'fresh destructive approval'
)) {
  Assert-Contains $runbook $needle "data-disposition runbook is missing $needle"
}

Assert-True ($gitignore.Trim() -ceq "out/") 'release output ignore must contain only out/'

foreach ($scriptPath in @($lockScriptPath, $checkScriptPath)) {
  $tokens = $null
  $errors = $null
  [Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$errors) | Out-Null
  Assert-True ($errors.Count -eq 0) "$scriptPath has PowerShell syntax errors"
}

& pwsh -NoProfile -File $checkScriptPath -SchemaOnly
Assert-True ($LASTEXITCODE -eq 0) 'schema-only input lock check failed'

Write-Output 'release input lock assertions passed'
