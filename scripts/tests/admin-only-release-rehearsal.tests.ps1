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
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $root '..\admin_front_ts'))
$verifierPath = Join-Path $root 'scripts\release\verify-admin-only-release.ps1'
$deploymentPath = Join-Path $root 'docs\runbooks\admin-only-deployment.md'
$rollbackPath = Join-Path $root 'docs\runbooks\admin-only-rollback.md'
$secretsPath = Join-Path $root 'docs\runbooks\admin-only-secrets.md'
$observabilityPath = Join-Path $root 'docs\runbooks\admin-only-observability.md'
$schemaStatusPath = Join-Path $root 'docs\runbooks\admin-only-schema-status.md'
$onboardingPath = Join-Path $root 'docs\runbooks\platform-onboarding.md'
$architecturePath = Join-Path $root 'docs\architecture.md'
$contextPath = Join-Path $root 'CONTEXT.md'

$verifier = Read-RequiredFile $verifierPath
$deployment = Read-RequiredFile $deploymentPath
$rollback = Read-RequiredFile $rollbackPath
$secrets = Read-RequiredFile $secretsPath
$observability = Read-RequiredFile $observabilityPath
$schemaStatus = Read-RequiredFile $schemaStatusPath
$onboarding = Read-RequiredFile $onboardingPath
$architecture = Read-RequiredFile $architecturePath
$context = Read-RequiredFile $contextPath

$expectedGates = @(
  'repository-boundary',
  'release-manifest',
  'backend-quality',
  'database-recovery-contract',
  'runtime-identity-durable-realtime',
  'admin-contract-bundle',
  'frontend-quality',
  'p07-runtime-acceptance',
  'p08r-browser-only-acceptance',
  'sensitive-material-scan',
  'admin-only-platform-kernel',
  'release-artifact-integrity'
)
$actualGates = @(& pwsh -NoProfile -File $verifierPath -ListGates)
Assert-True ($LASTEXITCODE -eq 0) 'release verifier gate listing failed'
Assert-True (($actualGates -join '|') -ceq ($expectedGates -join '|')) 'release verifier gate order changed'

foreach ($needle in @(
  '[string]$Manifest',
  '[switch]$ImportFunctions',
  '[switch]$ListGates',
  'release\admin-only\out\proof.json',
  'Assert-ReleaseManifest',
  'go clean -testcache',
  'go mod verify',
  'scripts\verify-backend.ps1',
  'scripts\verify-database.ps1',
  'scripts\tests\process-sigterm.tests.ps1',
  'scripts\tests\durable-work-restart.tests.ps1',
  'scripts\check-admin-contract.ps1',
  'node:24.18.0-alpine',
  'npm run verify:frontend',
  'docs\acceptance\p07-frontend-manual.md',
  'docs\acceptance\p08r-browser-only-manual.md',
  "'-Command', 'admin-status'",
  'scripts\tests\browser-only-cutover.tests.ps1',
  'client_versions',
  'auth-platforms',
  'git worktree list --porcelain',
  'git status --porcelain=v1 --untracked-files=all',
  'Move-Item -LiteralPath $temporaryPath -Destination $outputPath',
  'output_sha256',
  'duration_ms',
  'image_ids'
)) {
  Assert-Contains $verifier $needle "release verifier is missing $needle"
}
Assert-NotMatch $verifier '(Write-(Output|Host)|echo).*(MYSQL_DSN|APP_SECRET|REDIS_PASSWORD|refresh_token|access_token|Cookie)' 'release verifier may print a secret'
Assert-NotMatch $verifier '(ConvertTo-Json|Add-Member).*(prompt|dump_path|certificate|private_key|cookie|token)' 'release proof may serialize sensitive material'

foreach ($needle in @(
  'Release operator',
  'Database operator',
  'Maintenance window',
  'P09_DESTRUCTIVE_APPROVAL',
  'admin-status',
  'check-release-manifest.ps1',
  'deploy-admin-only.ps1',
  'health',
  'readiness',
  'STOP'
)) {
  Assert-Contains $deployment $needle "deployment runbook is missing $needle"
}

foreach ($needle in @(
  'Application rollback',
  'Full database rollback',
  'rollback-admin-only.ps1',
  'recovery artifact',
  'recovery rehearsal',
  'reverse DDL',
  'RTO',
  'RPO',
  'STOP'
)) {
  Assert-Contains $rollback $needle "rollback runbook is missing $needle"
}

foreach ($needle in @(
  'Release operator',
  'Database operator',
  'Security owner',
  'environment',
  'ignored',
  'redact',
  'MYSQL_DSN',
  'APP_SECRET',
  'REDIS_PASSWORD'
)) {
  Assert-Contains $secrets $needle "secrets runbook is missing $needle"
}
Assert-NotMatch $secrets '(?m)^(MYSQL_DSN|APP_SECRET|REDIS_PASSWORD|COS_SECRET_KEY)\s*=\s*[^<\s][^\r\n]+$' 'secrets runbook contains a credential value'

foreach ($needle in @(
  '/health',
  '/ready',
  'WebSocket',
  'queue',
  'scheduler',
  'provider',
  'redaction',
  'incident escalation'
)) {
  Assert-Contains $observability $needle "observability runbook is missing $needle"
}

foreach ($needle in @(
  '202607150201',
  '202607150202',
  '202607150203',
  'client_versions',
  'target fingerprint',
  'check-drift.ps1',
  '053_verify_admin_only.sql',
  'STOP'
)) {
  Assert-Contains $schemaStatus $needle "schema-status runbook is missing $needle"
}

$onboardingSteps = @(
  'Approve the platform contract',
  'Implement a dedicated trusted transport',
  'Add the compile-time registry entry',
  'Publish a matching Admin Contract Bundle',
  'Configure auth_platforms',
  'Assign independent permissions and roles',
  'Configure notification audiences',
  'Run cross-platform isolation tests',
  'Deploy immutable Docker images'
)
$lastPosition = -1
foreach ($step in $onboardingSteps) {
  $position = $onboarding.IndexOf($step, [StringComparison]::Ordinal)
  Assert-True ($position -gt $lastPosition) "platform onboarding order is missing or changed: $step"
  $lastPosition = $position
}
Assert-Contains $onboarding 'A database row or client header never activates a platform.' 'platform onboarding must reject data-driven activation'

foreach ($document in @($architecture, $context)) {
  Assert-Contains $document 'Admin-only immutable release' 'final architecture truth is missing'
  Assert-Contains $document 'database row or client header' 'platform activation boundary is missing'
  Assert-Contains $document 'release/admin-only/out/proof.json' 'release proof boundary is missing'
}

foreach ($scriptPath in @(
  $verifierPath,
  (Join-Path $root 'scripts\release\deploy-admin-only.ps1'),
  (Join-Path $root 'scripts\release\rollback-admin-only.ps1')
)) {
  $tokens = $null
  $errors = $null
  [Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$errors) | Out-Null
  Assert-True ($errors.Count -eq 0) "$scriptPath has PowerShell syntax errors"
}

& pwsh -NoProfile -File $verifierPath -ImportFunctions
Assert-True ($LASTEXITCODE -eq 0) 'release verifier function import failed'
Assert-True (Test-Path -LiteralPath (Join-Path $frontendRoot 'docs\acceptance\p07-frontend-manual.md') -PathType Leaf) 'P07 acceptance evidence is missing'
Assert-True (Test-Path -LiteralPath (Join-Path $frontendRoot 'docs\acceptance\p08r-browser-only-manual.md') -PathType Leaf) 'P08R acceptance evidence is missing'

Write-Output 'Admin-only release rehearsal assertions passed'
