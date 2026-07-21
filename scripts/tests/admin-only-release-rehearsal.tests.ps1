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

$invalidDatabaseOutput = @(& pwsh -NoProfile -File $verifierPath -Database 'invalid-name' 2>&1)
Assert-True ($LASTEXITCODE -ne 0) 'release verifier returned before validating its database argument'
Assert-True (($invalidDatabaseOutput -join "`n").Contains('ADMIN_RESTORE_DB or -Database', [StringComparison]::Ordinal)) 'release verifier did not report its database validation failure'

$tokens = $null
$parseErrors = $null
$verifierAst = [Management.Automation.Language.Parser]::ParseInput($verifier, [ref]$tokens, [ref]$parseErrors)
Assert-True ($parseErrors.Count -eq 0) 'release verifier contains PowerShell parse errors'
$manifestCalls = @($verifierAst.FindAll({
  param($node)
  return $node -is [Management.Automation.Language.CommandAst] -and $node.GetCommandName() -ceq 'Assert-ReleaseManifest'
}, $true))
Assert-True ($manifestCalls.Count -eq 2) 'release verifier must validate the manifest at both artifact gates'
foreach ($call in $manifestCalls) {
  $parameterNames = @($call.CommandElements |
    Where-Object { $_ -is [Management.Automation.Language.CommandParameterAst] } |
    ForEach-Object { $_.ParameterName })
  foreach ($requiredParameter in @('ManifestPath', 'InputLockPath', 'PlatformKernelProofPath', 'ImageMetadataPath', 'DockerExecutable')) {
    Assert-True ($parameterNames -ccontains $requiredParameter) "release verifier manifest validation is missing -$requiredParameter"
  }
}

$backendQualityEnvironmentNames = @(
  'APP_ENV',
  'APP_SECRET',
  'APP_SECRET_PREVIOUS',
  'HTTP_ADDR',
  'HTTP_READ_HEADER_TIMEOUT',
  'LOG_DIR',
  'MYSQL_DSN',
  'MYSQL_MAX_OPEN_CONNS',
  'MYSQL_MAX_IDLE_CONNS',
  'MYSQL_CONN_MAX_LIFETIME',
  'REDIS_ADDR',
  'REDIS_PASSWORD',
  'REDIS_DB',
  'TOKEN_REDIS_DB',
  'PAYMENT_CERT_BASE_DIR',
  'QUEUE_ENABLED',
  'QUEUE_REDIS_DB',
  'QUEUE_CONCURRENCY',
  'REALTIME_ENABLED',
  'REALTIME_PUBLISHER',
  'SCHEDULER_ENABLED',
  'CORS_ALLOW_ORIGINS',
  'DB_HOST',
  'DB_PORT',
  'DB_DATABASE',
  'DB_USERNAME',
  'DB_PASSWORD',
  'REDIS_HOST',
  'REDIS_PORT'
)
$backendQualityGates = @($verifierAst.FindAll({
  param($node)
  if ($node -isnot [Management.Automation.Language.CommandAst] -or
      $node.GetCommandName() -cne 'Invoke-AdminReleaseGate') {
    return $false
  }
  return @($node.CommandElements | Where-Object {
    $_ -is [Management.Automation.Language.StringConstantExpressionAst] -and
    $_.Value -ceq 'backend-quality'
  }).Count -eq 1
}, $true))
Assert-True ($backendQualityGates.Count -eq 1) 'release verifier must define one backend-quality gate'
$backendEnvironmentScopes = @($backendQualityGates[0].FindAll({
  param($node)
  return $node -is [Management.Automation.Language.CommandAst] -and
    $node.GetCommandName() -ceq 'Invoke-ReleaseWithCleanBackendEnvironment'
}, $true))
Assert-True ($backendEnvironmentScopes.Count -eq 1) 'backend-quality must run in one clean runtime environment scope'

$backendEnvironmentRestored = & {
  param([string]$VerifierPath, [string[]]$EnvironmentNames)
  . $VerifierPath -ImportFunctions

  $target = [EnvironmentVariableTarget]::Process
  $before = [Environment]::GetEnvironmentVariables($target)
  $original = [ordered]@{}
  foreach ($name in $EnvironmentNames) {
    $original[$name] = [pscustomobject]@{
      Exists = $before.Contains($name)
      Value = [Environment]::GetEnvironmentVariable($name, $target)
    }
  }

  try {
    for ($index = 0; $index -lt $EnvironmentNames.Count; $index++) {
      if ($index -eq ($EnvironmentNames.Count - 1)) {
        Remove-Item -LiteralPath "Env:$($EnvironmentNames[$index])" -ErrorAction SilentlyContinue
      } else {
        [Environment]::SetEnvironmentVariable($EnvironmentNames[$index], "release-fixture-$index", $target)
      }
    }

    $caughtFixtureFailure = $false
    try {
      Invoke-ReleaseWithCleanBackendEnvironment -Action {
        $isolated = [Environment]::GetEnvironmentVariables([EnvironmentVariableTarget]::Process)
        foreach ($name in $EnvironmentNames) {
          if ($isolated.Contains($name)) { throw "backend quality inherited $name" }
        }
        [Environment]::SetEnvironmentVariable('HTTP_ADDR', 'changed-inside-action', [EnvironmentVariableTarget]::Process)
        throw 'backend environment fixture failure'
      }
    } catch {
      if ($_.Exception.Message -cne 'backend environment fixture failure') { throw }
      $caughtFixtureFailure = $true
    }
    Assert-True $caughtFixtureFailure 'backend environment fixture did not execute'

    $restored = [Environment]::GetEnvironmentVariables($target)
    for ($index = 0; $index -lt ($EnvironmentNames.Count - 1); $index++) {
      $name = $EnvironmentNames[$index]
      Assert-True ($restored.Contains($name)) "backend quality did not restore $name"
      Assert-True ([string]$restored[$name] -ceq "release-fixture-$index") "backend quality changed restored $name"
    }
    Assert-True (-not $restored.Contains($EnvironmentNames[-1])) 'backend quality restored an originally absent variable'
    return $true
  } finally {
    foreach ($name in $EnvironmentNames) {
      if ($original[$name].Exists) {
        [Environment]::SetEnvironmentVariable($name, $original[$name].Value, $target)
      } else {
        Remove-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
      }
    }
  }
} $verifierPath $backendQualityEnvironmentNames
Assert-True $backendEnvironmentRestored 'backend-quality environment restoration fixture failed'

$frontendCommit = (& git -C $frontendRoot rev-parse --verify HEAD).Trim().ToLowerInvariant()
$cleanBoundary = & {
  param([string]$VerifierPath, [string]$Repository, [string]$Commit)
  . $VerifierPath -ImportFunctions
  Assert-ReleaseRepositoryBoundary -Repository $Repository -ExpectedCommit $Commit -Label 'clean fixture'
} $verifierPath $frontendRoot $frontendCommit
Assert-True ([int]$cleanBoundary.dirty_path_count -eq 0) 'release verifier did not accept a clean repository boundary'

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
