[CmdletBinding()]
param(
  [ValidateSet('all', 'empty', 'imported')]
  [string]$Mode = 'all',

  [string]$DockerCommand = 'docker',

  [string]$OutputPath = '',

  [ValidateRange(60, 1800)]
  [int]$CommandTimeoutSeconds = 600,

  [ValidateRange(30, 600)]
  [int]$ReadinessTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
. (Join-Path $backendRoot 'scripts/database/atlas-runtime-common.ps1')

$mysqlImage = 'mysql:8.4.10'
$atlasImage = 'arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a'
if ($script:AtlasImage -cne $atlasImage) { throw 'Atlas image pin differs from the database verifier' }
if (-not [string]::IsNullOrWhiteSpace($env:ADMIN_DATABASE_VERIFY_MYSQL_IMAGE) -and $env:ADMIN_DATABASE_VERIFY_MYSQL_IMAGE -cne $mysqlImage) {
  throw 'database verifier MySQL image override is not immutable'
}
if (-not [string]::IsNullOrWhiteSpace($env:ADMIN_DATABASE_VERIFY_ATLAS_IMAGE) -and $env:ADMIN_DATABASE_VERIFY_ATLAS_IMAGE -cne $atlasImage) {
  throw 'database verifier Atlas image override is not immutable'
}

$docker = (Get-Command -Name $DockerCommand -ErrorAction Stop | Select-Object -First 1).Source
$runID = [guid]::NewGuid().ToString('N')
$shortID = $runID.Substring(0, 12)
$containerName = "admin-p02-verify-mysql-$shortID"
$containerLabel = "admin.p02.verify=$runID"
$emptyDatabase = "admin_empty_$shortID"
$importedDatabase = "admin_imported_$shortID"
$disposableSchemaPattern = '^admin_(empty|imported)_[0-9a-f]{12}$'
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) "admin-database-verify-$runID"
$mysqlWrapper = Join-Path $temporaryRoot 'mysql-client.ps1'
$containerCleanupRequired = $false
$containerID = ''
$containerReady = $false
$createdDatabases = [System.Collections.Generic.List[string]]::new()

$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
$previousContainer = [Environment]::GetEnvironmentVariable('ADMIN_VERIFY_MYSQL_CONTAINER', 'Process')
$previousDocker = [Environment]::GetEnvironmentVariable('ADMIN_VERIFY_DOCKER_COMMAND', 'Process')
$previousRoot = [Environment]::GetEnvironmentVariable('ADMIN_VERIFY_BACKEND_ROOT', 'Process')
$cleanupErrors = [System.Collections.Generic.List[string]]::new()
$primaryError = $null

function Test-TrackedSensitivePaths {
  Push-Location $backendRoot
  try {
    $tracked = @(& git ls-files)
    if ($LASTEXITCODE -ne 0) { throw 'git tracked-file scan failed' }
  } finally {
    Pop-Location
  }
  $violations = @()
  foreach ($path in $tracked) {
    $normalized = ([string]$path).Replace('\', '/')
    $leaf = [System.IO.Path]::GetFileName($normalized)
    if ($leaf -ieq 'admin-go.env' -or $leaf.EndsWith('.cnf', [System.StringComparison]::OrdinalIgnoreCase)) {
      $violations += $normalized
      continue
    }
    if ($normalized -match '(?i)(^|/)[^/]*(dump|backup)[^/]*\.(sql|sql\.gz|dump)$') {
      $violations += $normalized
      continue
    }
    if ($normalized -match '^database/evidence/' -and $leaf -ne '.gitignore') {
      $violations += $normalized
    }
  }
  if ($violations.Count -ne 0) { throw 'tracked dump, .cnf, admin-go.env, or recovery evidence file detected' }
  return $tracked.Count
}

function Test-QueryManifest {
  Push-Location $backendRoot
  try {
    $files = @(& go run ./cmd/admin-db query-manifest files --manifest database/reconciliation/040_query_candidates.json 2>$null)
    if ($LASTEXITCODE -ne 0 -or $files.Count -eq 0) { throw 'query-manifest validation failed' }
    return $files.Count
  } finally {
    Pop-Location
  }
}

function Test-MigrationDirectory {
  $migrationRoot = Join-Path $backendRoot 'database/migrations'
  [void](Invoke-BoundedCommand -Executable $docker -Arguments @(
    'run', '--rm', '--network', 'none',
    '--volume', "${backendRoot}:/workspace:ro",
    '--workdir', '/workspace',
    $atlasImage,
    'migrate', 'validate', '--dir', 'file:///workspace/database/migrations'
  ) -Operation 'validate Atlas migration directory' -TimeoutSeconds $CommandTimeoutSeconds)

  $hashRoot = Join-Path $temporaryRoot 'migration-hash'
  [void](New-Item -ItemType Directory -Path $hashRoot)
  Get-ChildItem -LiteralPath $migrationRoot -Filter '*.sql' -File | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $hashRoot $_.Name)
  }
  [void](Invoke-BoundedCommand -Executable $docker -Arguments @(
    'run', '--rm', '--network', 'none',
    '--volume', "${hashRoot}:/migrations:rw",
    $atlasImage,
    'migrate', 'hash', '--dir', 'file:///migrations'
  ) -Operation 'calculate Atlas migration checksum' -TimeoutSeconds $CommandTimeoutSeconds)
  $calculatedSum = Join-Path $hashRoot 'atlas.sum'
  $trackedSum = Join-Path $migrationRoot 'atlas.sum'
  if (-not (Test-Path -LiteralPath $calculatedSum -PathType Leaf)) { throw 'Atlas did not calculate atlas.sum' }
  if ((Get-FileHash -LiteralPath $calculatedSum -Algorithm SHA256).Hash -cne (Get-FileHash -LiteralPath $trackedSum -Algorithm SHA256).Hash) {
    throw 'tracked Atlas checksum differs from a clean calculation'
  }
}

function Write-MySQLClientWrapper {
  $content = @'
[CmdletBinding()]
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$docker = [Environment]::GetEnvironmentVariable('ADMIN_VERIFY_DOCKER_COMMAND', 'Process')
$container = [Environment]::GetEnvironmentVariable('ADMIN_VERIFY_MYSQL_CONTAINER', 'Process')
$backendRoot = [Environment]::GetEnvironmentVariable('ADMIN_VERIFY_BACKEND_ROOT', 'Process')
if ([string]::IsNullOrWhiteSpace($docker) -or [string]::IsNullOrWhiteSpace($container) -or [string]::IsNullOrWhiteSpace($backendRoot)) {
  throw 'database verifier MySQL wrapper environment is incomplete'
}
$mapped = @()
foreach ($argument in $Arguments) {
  if ($argument -eq '--protocol=tcp' -or $argument.StartsWith('--host=') -or $argument.StartsWith('--port=')) { continue }
  if ($argument.StartsWith('--execute=SOURCE ', [System.StringComparison]::Ordinal)) {
    $source = $argument.Substring('--execute=SOURCE '.Length)
    $fullSource = [System.IO.Path]::GetFullPath($source)
    $fullRoot = [System.IO.Path]::GetFullPath($backendRoot)
    $relative = [System.IO.Path]::GetRelativePath($fullRoot, $fullSource).Replace('\', '/')
    if ($relative.StartsWith('../', [System.StringComparison]::Ordinal) -or $relative -notmatch '^database/reconciliation/[0-9]{3}_[a-z0-9_-]+\.sql$') {
      throw 'database verifier refused an unexpected SOURCE path'
    }
    $mapped += '--execute=SOURCE /workspace/' + $relative
    continue
  }
  $mapped += $argument
}
& $docker exec $container mysql @mapped
if ($LASTEXITCODE -ne 0) { throw 'containerized MySQL client failed' }
'@
  [System.IO.File]::WriteAllText($mysqlWrapper, $content, [System.Text.UTF8Encoding]::new($false))
}

function Start-DisposableMySQL {
  $script:containerCleanupRequired = $true
  $output = Invoke-BoundedCommand -Executable $docker -Arguments @(
    'run', '--detach',
    '--name', $containerName,
    '--label', $containerLabel,
    '--publish', '127.0.0.1::3306',
    '--volume', "${backendRoot}:/workspace:ro",
    '--env', 'MYSQL_ALLOW_EMPTY_PASSWORD=yes',
    '--env', 'MYSQL_ROOT_HOST=%',
    $mysqlImage,
    '--skip-log-bin',
    '--sql-mode=NO_ENGINE_SUBSTITUTION'
  ) -Operation 'start disposable MySQL' -TimeoutSeconds 120
  $script:containerID = $output.Trim()
  if ($script:containerID -notmatch '^[0-9a-f]{64}$') { throw 'Docker returned an invalid MySQL container ID' }

  $deadline = [DateTimeOffset]::UtcNow.AddSeconds($ReadinessTimeoutSeconds)
  while ([DateTimeOffset]::UtcNow -lt $deadline) {
    $running = (Invoke-BoundedCommand -Executable $docker -Arguments @('inspect', '--format', '{{.State.Running}}', $script:containerID) -Operation 'inspect disposable MySQL' -TimeoutSeconds 15).Trim()
    if ($running -cne 'true') { throw 'disposable MySQL exited before becoming ready' }
    & $docker exec $script:containerID sh -c 'test "$(cat /proc/1/comm)" = mysqld' *> $null
    $pidReady = $LASTEXITCODE -eq 0
    & $docker exec $script:containerID mysqladmin ping --user=root --silent *> $null
    $pingReady = $LASTEXITCODE -eq 0
    if ($pidReady -and $pingReady) {
      $script:containerReady = $true
      break
    }
    Start-Sleep -Milliseconds 500
  }
  if (-not $script:containerReady) { throw 'disposable MySQL readiness timed out' }

  $portOutput = Invoke-BoundedCommand -Executable $docker -Arguments @('port', $script:containerID, '3306/tcp') -Operation 'resolve disposable MySQL port' -TimeoutSeconds 15
  $match = [regex]::Match($portOutput, '(?m)^127\.0\.0\.1:([0-9]+)\s*$')
  if (-not $match.Success) { throw 'disposable MySQL port output was invalid' }
  return [int]$match.Groups[1].Value
}

function Set-DatabaseDSN {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port
  )
  if ($Database -notmatch $disposableSchemaPattern) { throw 'refusing to create a DSN for an unexpected schema' }
  $env:MYSQL_DSN = "root:@tcp(127.0.0.1:${Port})/${Database}?parseTime=true&multiStatements=true&loc=Local"
}

function Invoke-MySQLClient {
  param(
    [string]$Database = '',
    [Parameter(Mandatory = $true)][string]$SQL,
    [Parameter(Mandatory = $true)][string]$Operation
  )
  $arguments = @('--user=root', '--batch', '--skip-column-names', '--raw')
  if (-not [string]::IsNullOrWhiteSpace($Database)) {
    if ($Database -notmatch $disposableSchemaPattern) { throw 'refusing to target an unexpected schema' }
    $arguments += "--database=$Database"
  }
  $arguments += "--execute=$SQL"
  $output = @(& $mysqlWrapper @arguments 2>$null)
  if ($LASTEXITCODE -ne 0) { throw "$Operation failed" }
  return @($output | ForEach-Object { $_.ToString() })
}

function New-DisposableDatabase {
  param([Parameter(Mandatory = $true)][string]$Database)
  if ($Database -notmatch $disposableSchemaPattern) { throw 'generated disposable schema name is invalid' }
  [void](Invoke-MySQLClient -SQL "CREATE DATABASE ``$Database`` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci" -Operation 'create disposable schema')
  $createdDatabases.Add($Database)
}

function Apply-CanonicalSchema {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port
  )
  Set-DatabaseDSN -Database $Database -Port $Port
  $settings = Get-MySQLDSNSettings -Database $Database
  $runtimeDirectory = ''
  try {
    $runtimeDirectory = New-AtlasRuntimeConfig -Settings $settings -Database $Database
    $schemaPath = Join-Path $backendRoot 'database/schema/admin.hcl'
    $canonical = [System.IO.File]::ReadAllText($schemaPath, [System.Text.Encoding]::UTF8)
    if ([regex]::Matches($canonical, '(?m)^schema "admin" \{$').Count -ne 1) { throw 'canonical schema declaration is invalid' }
    $rebound = $canonical.Replace('schema "admin" {', 'schema "' + $Database + '" {').Replace('schema.admin', "schema.$Database")
    if ([regex]::IsMatch($rebound, '\bschema\.admin\b')) { throw 'canonical schema rebinding was incomplete' }
    [System.IO.File]::WriteAllText((Join-Path $runtimeDirectory 'admin.hcl'), $rebound, [System.Text.UTF8Encoding]::new($false))
    [void](Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
      'schema', 'apply', '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime',
      '--to', 'file:///runtime/admin.hcl', '--auto-approve'
    ) -TimeoutSeconds $CommandTimeoutSeconds)
  } finally {
    Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
    $settings.Password = $null
  }
}

function Get-Fingerprint {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port
  )
  Set-DatabaseDSN -Database $Database -Port $Port
  $settings = Get-MySQLDSNSettings -Database $Database
  try {
    return Get-DatabaseFingerprintSHA -BackendRoot $backendRoot -Settings $settings -Database $Database
  } finally {
    $settings.Password = $null
  }
}

function Get-FingerprintDocument {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port
  )
  Set-DatabaseDSN -Database $Database -Port $Port
  $path = Join-Path $temporaryRoot ("fingerprint-$Database-" + [guid]::NewGuid().ToString('N') + '.json')
  Push-Location $backendRoot
  try {
    $commit = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') { throw 'Git commit could not be resolved' }
    & go run ./cmd/admin-db fingerprint --schema $Database --out $path --commit $commit 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'schema convergence diagnostic fingerprint failed' }
    return Get-Content -Raw -LiteralPath $path -Encoding utf8 | ConvertFrom-Json
  } finally {
    Pop-Location
    Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
  }
}

function Get-SchemaConvergenceDiagnostic {
  param(
    [Parameter(Mandatory = $true)]$Expected,
    [Parameter(Mandatory = $true)]$Actual
  )
  $differences = [System.Collections.Generic.List[string]]::new()
  if ([string]$Expected.server_version -cne [string]$Actual.server_version) { $differences.Add('server_version') }
  if ([string]$Expected.sql_mode -cne [string]$Actual.sql_mode) { $differences.Add('sql_mode') }

  $expectedTables = @{}
  foreach ($table in @($Expected.tables)) { $expectedTables[[string]$table.name] = $table }
  $actualTables = @{}
  foreach ($table in @($Actual.tables)) { $actualTables[[string]$table.name] = $table }
  $tableNames = @(($expectedTables.Keys + $actualTables.Keys) | Sort-Object -Unique)
  foreach ($tableName in $tableNames) {
    if (-not $expectedTables.ContainsKey($tableName)) { $differences.Add("extra-table:$tableName"); continue }
    if (-not $actualTables.ContainsKey($tableName)) { $differences.Add("missing-table:$tableName"); continue }
    $expectedTable = $expectedTables[$tableName]
    $actualTable = $actualTables[$tableName]
    $expectedMetadata = [ordered]@{ engine = $expectedTable.engine; collation = $expectedTable.collation; comment = $expectedTable.comment }
    $actualMetadata = [ordered]@{ engine = $actualTable.engine; collation = $actualTable.collation; comment = $actualTable.comment }
    if (($expectedMetadata | ConvertTo-Json -Compress) -cne ($actualMetadata | ConvertTo-Json -Compress)) {
      $differences.Add("table-metadata:$tableName")
    }

    $expectedColumns = @{}
    foreach ($column in @($expectedTable.columns)) { $expectedColumns[[string]$column.name] = $column }
    $actualColumns = @{}
    foreach ($column in @($actualTable.columns)) { $actualColumns[[string]$column.name] = $column }
    foreach ($columnName in @(($expectedColumns.Keys + $actualColumns.Keys) | Sort-Object -Unique)) {
      if (-not $expectedColumns.ContainsKey($columnName)) { $differences.Add("extra-column:$tableName.$columnName"); continue }
      if (-not $actualColumns.ContainsKey($columnName)) { $differences.Add("missing-column:$tableName.$columnName"); continue }
      if (($expectedColumns[$columnName] | ConvertTo-Json -Depth 8 -Compress) -cne ($actualColumns[$columnName] | ConvertTo-Json -Depth 8 -Compress)) {
        $differences.Add("changed-column:$tableName.$columnName")
      }
    }

    $expectedIndexes = @{}
    foreach ($index in @($expectedTable.indexes)) { $expectedIndexes[[string]$index.name] = $index }
    $actualIndexes = @{}
    foreach ($index in @($actualTable.indexes)) { $actualIndexes[[string]$index.name] = $index }
    foreach ($indexName in @(($expectedIndexes.Keys + $actualIndexes.Keys) | Sort-Object -Unique)) {
      if (-not $expectedIndexes.ContainsKey($indexName)) { $differences.Add("extra-index:$tableName.$indexName"); continue }
      if (-not $actualIndexes.ContainsKey($indexName)) { $differences.Add("missing-index:$tableName.$indexName"); continue }
      if (($expectedIndexes[$indexName] | ConvertTo-Json -Depth 8 -Compress) -cne ($actualIndexes[$indexName] | ConvertTo-Json -Depth 8 -Compress)) {
        $differences.Add("changed-index:$tableName.$indexName")
      }
    }
  }

  $expectedForeignKeys = @($Expected.foreign_keys | ForEach-Object {
    [ordered]@{
      table = $_.table; name = $_.name; ordinal = $_.ordinal; column = $_.column
      referenced_schema = if ([string]$_.referenced_schema -ceq [string]$Expected.schema) { '' } else { [string]$_.referenced_schema }
      referenced_table = $_.referenced_table; referenced_column = $_.referenced_column
      update_rule = $_.update_rule; delete_rule = $_.delete_rule
    }
  })
  $actualForeignKeys = @($Actual.foreign_keys | ForEach-Object {
    [ordered]@{
      table = $_.table; name = $_.name; ordinal = $_.ordinal; column = $_.column
      referenced_schema = if ([string]$_.referenced_schema -ceq [string]$Actual.schema) { '' } else { [string]$_.referenced_schema }
      referenced_table = $_.referenced_table; referenced_column = $_.referenced_column
      update_rule = $_.update_rule; delete_rule = $_.delete_rule
    }
  })
  if (($expectedForeignKeys | ConvertTo-Json -Depth 8 -Compress) -cne ($actualForeignKeys | ConvertTo-Json -Depth 8 -Compress)) {
    $differences.Add('changed-collection:foreign_keys')
  }

  $expectedChecks = @{}
  foreach ($check in @($Expected.checks)) { $expectedChecks[([string]$check.table + '|' + [string]$check.name)] = $check }
  $actualChecks = @{}
  foreach ($check in @($Actual.checks)) { $actualChecks[([string]$check.table + '|' + [string]$check.name)] = $check }
  foreach ($checkKey in @(($expectedChecks.Keys + $actualChecks.Keys) | Sort-Object -Unique)) {
    if (-not $expectedChecks.ContainsKey($checkKey)) { $differences.Add("extra-check:$checkKey"); continue }
    if (-not $actualChecks.ContainsKey($checkKey)) { $differences.Add("missing-check:$checkKey"); continue }
    if (($expectedChecks[$checkKey] | ConvertTo-Json -Depth 8 -Compress) -cne ($actualChecks[$checkKey] | ConvertTo-Json -Depth 8 -Compress)) {
      $differences.Add("changed-check:$checkKey")
    }
  }

  foreach ($collection in @('triggers', 'routines', 'events')) {
    if (($Expected.$collection | ConvertTo-Json -Depth 12 -Compress) -cne ($Actual.$collection | ConvertTo-Json -Depth 12 -Compress)) {
      $differences.Add("changed-collection:$collection")
    }
  }
  if ($differences.Count -eq 0) { return 'hashes differ without a projected structural difference' }
  return (@($differences | Select-Object -First 40) -join ',')
}

function Restore-SyntheticImportedFixture {
  param([Parameter(Mandatory = $true)][string]$Database)
  $fixtureSQL = @'
DROP INDEX `idx_user_sessions_user_platform_active_refresh` ON `user_sessions`;
DROP INDEX `idx_ai_runs_status_started` ON `ai_runs`;
DROP INDEX `uk_ai_runs_idempotency` ON `ai_runs`;
ALTER TABLE `ai_runs` DROP COLUMN `idempotency_key`;
ALTER TABLE `ai_runs` MODIFY COLUMN `request_id` VARCHAR(64) NOT NULL;
ALTER TABLE `ai_reply_commands` MODIFY COLUMN `request_id` VARCHAR(64) NOT NULL;
DROP INDEX `idx_notifications_user_active_unread_platform` ON `notifications`;
DROP INDEX `uk_notifications_source_user` ON `notifications`;
ALTER TABLE `notifications` DROP COLUMN `source_task_id`;
DROP INDEX `idx_notification_task_claim` ON `notification_task`;
ALTER TABLE `notification_task`
  DROP COLUMN `claim_expires_at`,
  DROP COLUMN `claim_token`,
  DROP COLUMN `claim_owner`;
DROP INDEX `idx_export_tasks_user_platform_active_id` ON `export_tasks`;
DROP INDEX `idx_export_task_claim` ON `export_tasks`;
ALTER TABLE `export_tasks`
  DROP COLUMN `claim_expires_at`,
  DROP COLUMN `claim_token`,
  DROP COLUMN `claim_owner`;
DROP INDEX `idx_cron_task_log_task_active_created` ON `cron_task_log`;
ALTER TABLE `ai_image_files` DROP COLUMN `is_del`;
ALTER TABLE `ai_image_tasks` DROP COLUMN `is_del`;
SET FOREIGN_KEY_CHECKS=0;
DROP TABLE `ai_provider_attempts`;
DROP TABLE `realtime_events`;
DROP TABLE `realtime_event_retention_watermarks`;
SET FOREIGN_KEY_CHECKS=1;
INSERT INTO `users` (`id`,`role_id`,`username`,`status`,`is_del`,`created_at`,`updated_at`)
VALUES (900001,0,'synthetic_admin',1,2,UTC_TIMESTAMP(),UTC_TIMESTAMP());
INSERT INTO `export_tasks` (`id`,`user_id`,`platform`,`title`,`kind`,`status`,`is_del`,`created_at`,`updated_at`)
VALUES (900001,900001,'','Synthetic export fixture','',1,2,UTC_TIMESTAMP(),UTC_TIMESTAMP());
'@
  [void](Invoke-MySQLClient -Database $Database -SQL $fixtureSQL -Operation 'restore sanitized synthetic imported fixture')
}

function Invoke-Reconciliation {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port,
    [Parameter(Mandatory = $true)][string]$ExpectedFingerprint
  )
  Set-DatabaseDSN -Database $Database -Port $Port
  return @(& (Join-Path $backendRoot 'scripts/database/reconcile.ps1') -Database $Database -Stage 'all-nondestructive' -ExpectedSourceFingerprint $ExpectedFingerprint -MySQLCommand $mysqlWrapper -Executor 'database-verifier')
}

function Invoke-ExpandedVerification {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port
  )
  Set-DatabaseDSN -Database $Database -Port $Port
  Push-Location $backendRoot
  try {
    $output = @(& (Join-Path $backendRoot 'scripts/database/verify-expanded-schema.ps1') -Database $Database -MySQLCommand $mysqlWrapper)
  } finally {
    Pop-Location
  }
  if ($output.Count -eq 0) { throw 'expanded database verification returned no summary' }
  $summary = $output[-1] | ConvertFrom-Json
  foreach ($field in @(
    'schema_violations', 'relationship_violations', 'money_violations', 'ai_violations',
    'platform_violations', 'ai_image_delete_violations', 'export_cleanup_violations',
    'cron_task_metadata_violations'
  )) {
    if ([uint64]$summary.$field -ne 0) { throw "expanded database verification failed: $field" }
  }
  if ([string]$summary.admin_smoke -cne 'passed') { throw 'expanded Admin smoke did not pass' }
  return $summary
}

function Write-VerificationSummary {
  param([Parameter(Mandatory = $true)]$Summary)
  if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $script:OutputPath = Join-Path ([System.IO.Path]::GetTempPath()) "admin-database-verification-$runID.json"
  }
  $fullPath = [System.IO.Path]::GetFullPath($OutputPath)
  if ([System.IO.Path]::GetExtension($fullPath) -cne '.json') { throw 'database verification output must be a JSON file' }
  $parent = Split-Path -Parent $fullPath
  [void](New-Item -ItemType Directory -Path $parent -Force)
  $temporary = $fullPath + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
  try {
    $json = $Summary | ConvertTo-Json -Depth 8
    [System.IO.File]::WriteAllText($temporary, $json + "`n", [System.Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporary -Destination $fullPath -Force
  } finally {
    Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
  }
  Write-Output $fullPath
  Write-Output ($Summary | ConvertTo-Json -Depth 8 -Compress)
}

function Remove-DisposableContainer {
  $target = if ($containerID -match '^[0-9a-f]{64}$') { $containerID } else { $containerName }
  $label = @(& $docker inspect --format '{{index .Config.Labels "admin.p02.verify"}}' $target 2>$null)
  if ($LASTEXITCODE -ne 0) { return }
  if ($label.Count -ne 1 -or [string]$label[0] -cne $runID) { throw 'refusing to remove an unowned MySQL container' }
  [void](Invoke-BoundedCommand -Executable $docker -Arguments @('rm', '--force', '--volumes', $target) -Operation 'remove disposable MySQL' -TimeoutSeconds 60)
}

function Remove-VerificationTemporaryRoot {
  if (-not (Test-Path -LiteralPath $temporaryRoot)) { return }
  $resolved = [System.IO.Path]::GetFullPath($temporaryRoot)
  $tempPrefix = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  if (-not $resolved.StartsWith($tempPrefix, [System.StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolved) -notmatch '^admin-database-verify-[0-9a-f]{32}$') {
    throw 'refusing to remove an unexpected verification directory'
  }
  Remove-Item -LiteralPath $resolved -Recurse -Force
}

try {
  [void](New-Item -ItemType Directory -Path $temporaryRoot)
  $trackedFileCount = Test-TrackedSensitivePaths
  $queryManifestFileCount = Test-QueryManifest
  Test-MigrationDirectory

  $port = Start-DisposableMySQL
  $env:ADMIN_VERIFY_MYSQL_CONTAINER = $containerID
  $env:ADMIN_VERIFY_DOCKER_COMMAND = $docker
  $env:ADMIN_VERIFY_BACKEND_ROOT = $backendRoot
  Write-MySQLClientWrapper

  New-DisposableDatabase -Database $emptyDatabase
  Apply-CanonicalSchema -Database $emptyDatabase -Port $port
  $emptyFingerprint = Get-Fingerprint -Database $emptyDatabase -Port $port

  $importedFingerprint = $null
  $reconciliationApplied = 0
  $reconciliationSkipped = 0
  $invariantSummary = $null
  if ($Mode -in @('all', 'imported')) {
    New-DisposableDatabase -Database $importedDatabase
    Apply-CanonicalSchema -Database $importedDatabase -Port $port
    Restore-SyntheticImportedFixture -Database $importedDatabase
    $fixtureFingerprint = Get-Fingerprint -Database $importedDatabase -Port $port
    if ($fixtureFingerprint -ceq $emptyFingerprint) { throw 'synthetic imported fixture did not exercise schema reconciliation' }

    $firstRun = Invoke-Reconciliation -Database $importedDatabase -Port $port -ExpectedFingerprint $fixtureFingerprint
    $reconciliationApplied = @($firstRun | Where-Object { $_ -match '^APPLY ' }).Count
    if ($reconciliationApplied -ne 9) { throw 'initial reconciliation did not apply every non-destructive script' }
    $importedFingerprint = Get-Fingerprint -Database $importedDatabase -Port $port
    if ($importedFingerprint -cne $emptyFingerprint) {
      $emptyDocument = Get-FingerprintDocument -Database $emptyDatabase -Port $port
      $importedDocument = Get-FingerprintDocument -Database $importedDatabase -Port $port
      $diagnostic = Get-SchemaConvergenceDiagnostic -Expected $emptyDocument -Actual $importedDocument
      throw "reconciled imported schema differs from canonical empty schema: $diagnostic"
    }

    $secondRun = Invoke-Reconciliation -Database $importedDatabase -Port $port -ExpectedFingerprint $importedFingerprint
    $reconciliationSkipped = @($secondRun | Where-Object { $_ -match '^SKIP ' }).Count
    if ($reconciliationSkipped -ne 9 -or @($secondRun | Where-Object { $_ -match '^APPLY ' }).Count -ne 0) {
      throw 'repeated reconciliation was not a complete no-op'
    }
    $repeatedFingerprint = Get-Fingerprint -Database $importedDatabase -Port $port
    if ($repeatedFingerprint -cne $importedFingerprint) { throw 'repeated reconciliation changed the schema fingerprint' }
    $invariantSummary = Invoke-ExpandedVerification -Database $importedDatabase -Port $port
  }

  $summary = [ordered]@{
    mode = $Mode
    mysql_image = $mysqlImage
    atlas_image = $atlasImage
    migration_checksum = 'validated'
    query_manifest_files = $queryManifestFileCount
    tracked_files_scanned = $trackedFileCount
    tracked_sensitive_paths = 0
    empty_schema_sha256 = $emptyFingerprint
    imported_schema_sha256 = $importedFingerprint
    reconciliation_applied = $reconciliationApplied
    reconciliation_skipped_on_repeat = $reconciliationSkipped
    invariants = if ($null -eq $invariantSummary) { 'not-requested' } else { 'passed' }
    admin_smoke = if ($null -eq $invariantSummary) { 'not-requested' } else { [string]$invariantSummary.admin_smoke }
  }
  Write-VerificationSummary -Summary $summary
} catch {
  $primaryError = $_
} finally {
  if ($containerReady -and (Test-Path -LiteralPath $mysqlWrapper)) {
    for ($index = $createdDatabases.Count - 1; $index -ge 0; $index--) {
      $database = $createdDatabases[$index]
      if ($database -notmatch $disposableSchemaPattern) {
        $cleanupErrors.Add('refused unexpected disposable schema name')
        continue
      }
      try { [void](Invoke-MySQLClient -SQL "DROP DATABASE IF EXISTS ``$database``" -Operation 'drop disposable schema') } catch { $cleanupErrors.Add('drop disposable schema failed') }
    }
  }
  if ($containerCleanupRequired) {
    try { Remove-DisposableContainer } catch { $cleanupErrors.Add('remove disposable MySQL failed') }
  }
  try { Remove-VerificationTemporaryRoot } catch { $cleanupErrors.Add('remove verification temporary directory failed') }
  [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
  [Environment]::SetEnvironmentVariable('ADMIN_VERIFY_MYSQL_CONTAINER', $previousContainer, 'Process')
  [Environment]::SetEnvironmentVariable('ADMIN_VERIFY_DOCKER_COMMAND', $previousDocker, 'Process')
  [Environment]::SetEnvironmentVariable('ADMIN_VERIFY_BACKEND_ROOT', $previousRoot, 'Process')
}

if ($null -ne $primaryError) {
  if ($cleanupErrors.Count -ne 0) {
    throw [System.Exception]::new(
      ($primaryError.Exception.Message + '; cleanup: ' + ($cleanupErrors -join '; ')),
      $primaryError.Exception
    )
  }
  $PSCmdlet.ThrowTerminatingError($primaryError)
}
if ($cleanupErrors.Count -ne 0) { throw ($cleanupErrors -join '; ') }
