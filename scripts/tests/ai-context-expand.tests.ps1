[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[0-9a-f]{40}$')]
  [string]$BaselineCommit,

  [ValidateRange(60, 600)]
  [int]$ReadinessTimeoutSeconds = 180,

  [ValidateRange(60, 1800)]
  [int]$CommandTimeoutSeconds = 600
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
. (Join-Path $backendRoot 'scripts/database/atlas-runtime-common.ps1')

$mysqlImage = 'mysql:8.4.10'
$atlasImage = 'arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a'
if ($script:AtlasImage -cne $atlasImage) { throw 'Atlas image pin differs from the Context Expand verifier' }

$docker = (Get-Command -Name docker -ErrorAction Stop | Select-Object -First 1).Source
$go = (Get-Command -Name go -ErrorAction Stop | Select-Object -First 1).Source
$runID = [guid]::NewGuid().ToString('N')
$shortID = $runID.Substring(0, 12)
$containerName = "admin-ai-context-expand-$shortID"
$containerLabelName = 'admin.ai-context.expand'
$containerLabel = "$containerLabelName=$runID"
$beforeDatabase = "admin_context_before_$shortID"
$afterDatabase = "admin_context_after_$shortID"
$disposableSchemaPattern = '^admin_context_(before|after)_[0-9a-f]{12}$'
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) "admin-ai-context-expand-$runID"
$migrationPath = '/workspace/database/migrations/202608020101_ai_context_expand.sql'
$containerID = ''
$containerReady = $false
$containerCleanupRequired = $false
$createdDatabases = [System.Collections.Generic.List[string]]::new()
$cleanupErrors = [System.Collections.Generic.List[string]]::new()
$primaryError = $null
$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')

function Assert-DisposableSchemaName {
  param([Parameter(Mandatory = $true)][string]$Database)
  if ($Database -notmatch $disposableSchemaPattern) {
    throw 'refusing an unexpected disposable schema name'
  }
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
  ) -Operation 'start disposable Context Expand MySQL' -TimeoutSeconds 120
  $script:containerID = $output.Trim()
  if ($script:containerID -notmatch '^[0-9a-f]{64}$') {
    throw 'Docker returned an invalid disposable MySQL container ID'
  }

  $deadline = [DateTimeOffset]::UtcNow.AddSeconds($ReadinessTimeoutSeconds)
  while ([DateTimeOffset]::UtcNow -lt $deadline) {
    $running = (Invoke-BoundedCommand -Executable $docker -Arguments @(
      'inspect', '--format', '{{.State.Running}}', $script:containerID
    ) -Operation 'inspect disposable Context Expand MySQL' -TimeoutSeconds 15).Trim()
    if ($running -cne 'true') { throw 'disposable Context Expand MySQL exited before readiness' }
    $pidState = (Invoke-BoundedCommand -Executable $docker -Arguments @(
      'exec', $script:containerID, 'sh', '-c',
      'if test "$(cat /proc/1/comm)" = mysqld; then printf true; else printf false; fi'
    ) -Operation 'probe disposable Context Expand MySQL PID 1' -TimeoutSeconds 15).Trim()
    $pidReady = $pidState -ceq 'true'
    $pingState = (Invoke-BoundedCommand -Executable $docker -Arguments @(
      'exec', $script:containerID, 'sh', '-c',
      'if mysqladmin ping --user=root --silent >/dev/null 2>&1; then printf true; else printf false; fi'
    ) -Operation 'probe disposable Context Expand MySQL ping' -TimeoutSeconds 15).Trim()
    $pingReady = $pingState -ceq 'true'
    if ($pidReady -and $pingReady) {
      $script:containerReady = $true
      break
    }
    Start-Sleep -Milliseconds 500
  }
  if (-not $script:containerReady) { throw 'disposable Context Expand MySQL readiness timed out' }

  $portOutput = Invoke-BoundedCommand -Executable $docker -Arguments @(
    'port', $script:containerID, '3306/tcp'
  ) -Operation 'resolve disposable Context Expand MySQL port' -TimeoutSeconds 15
  $match = [regex]::Match($portOutput, '(?m)^127\.0\.0\.1:([0-9]+)\s*$')
  if (-not $match.Success) { throw 'disposable Context Expand MySQL port was not loopback-only' }
  return [int]$match.Groups[1].Value
}

function Invoke-ContainerMySQL {
  param(
    [string]$Database = '',
    [Parameter(Mandatory = $true)][string]$SQL,
    [Parameter(Mandatory = $true)][string]$Operation
  )
  $arguments = @('exec', $containerID, 'mysql', '--user=root', '--batch', '--skip-column-names', '--raw')
  if (-not [string]::IsNullOrWhiteSpace($Database)) {
    Assert-DisposableSchemaName -Database $Database
    $arguments += "--database=$Database"
  }
  $arguments += "--execute=$SQL"
  return Invoke-BoundedCommand -Executable $docker -Arguments $arguments -Operation $Operation -TimeoutSeconds $CommandTimeoutSeconds
}

function Invoke-CapturedContainerMySQL {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$SQL,
    [Parameter(Mandatory = $true)][string]$Operation
  )
  Assert-DisposableSchemaName -Database $Database
  $wrapper = @'
output=$(mysql --user=root --database="$1" --batch --skip-column-names --raw --execute="$2" 2>&1)
status=$?
printf '%s\n' "$output"
printf '__ADMIN_CONTEXT_MYSQL_EXIT__=%s\n' "$status"
'@
  $raw = Invoke-BoundedCommand -Executable $docker -Arguments @(
    'exec', $containerID, 'sh', '-c', $wrapper,
    'admin-context-mysql', $Database, $SQL
  ) -Operation $Operation -TimeoutSeconds $CommandTimeoutSeconds
  $lines = @($raw -split "`r?`n")
  $marker = @($lines | Where-Object { $_ -match '^__ADMIN_CONTEXT_MYSQL_EXIT__=([0-9]+)$' })
  if ($marker.Count -ne 1) { throw "$Operation returned malformed status output" }
  $match = [regex]::Match($marker[0], '^__ADMIN_CONTEXT_MYSQL_EXIT__=([0-9]+)$')
  return [pscustomobject]@{
    ExitCode = [int]$match.Groups[1].Value
    Output = (@($lines | Where-Object { $_ -notmatch '^__ADMIN_CONTEXT_MYSQL_EXIT__=' }) -join "`n").Trim()
  }
}

function New-DisposableDatabase {
  param([Parameter(Mandatory = $true)][string]$Database)
  Assert-DisposableSchemaName -Database $Database
  [void](Invoke-ContainerMySQL -SQL "CREATE DATABASE ``$Database`` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci" -Operation 'create disposable Context Expand schema')
  $createdDatabases.Add($Database)
}

function Get-CanonicalAtCommit {
  Push-Location $backendRoot
  try {
    $lines = @(& git show "${BaselineCommit}:database/schema/admin.hcl" 2>$null)
    if ($LASTEXITCODE -ne 0 -or $lines.Count -eq 0) { throw 'baseline canonical HCL could not be read' }
    return ($lines -join "`n") + "`n"
  } finally {
    Pop-Location
  }
}

function Get-ExpandCanonical {
  Push-Location $backendRoot
  try {
    $expandCommits = @(& git log --diff-filter=A --format=%H -- database/migrations/202608020101_ai_context_expand.sql)
    if ($LASTEXITCODE -ne 0) { throw 'Expand migration history could not be read' }
    if ($expandCommits.Count -eq 0) {
      return [pscustomobject]@{
        Canonical = [System.IO.File]::ReadAllText((Join-Path $backendRoot 'database/schema/admin.hcl'), [System.Text.Encoding]::UTF8)
        Target = 'worktree'
      }
    }
    if ($expandCommits.Count -ne 1 -or [string]$expandCommits[0] -notmatch '^[0-9a-f]{40}$') {
      throw 'Expand migration must have exactly one introducing commit'
    }
    $expandCommit = [string]$expandCommits[0]
    & git merge-base --is-ancestor $BaselineCommit $expandCommit
    if ($LASTEXITCODE -ne 0) { throw 'Expand commit is not a descendant of the requested baseline' }
    & git merge-base --is-ancestor $expandCommit HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Expand commit is not an ancestor of HEAD' }
    $lines = @(& git show "${expandCommit}:database/schema/admin.hcl" 2>$null)
    if ($LASTEXITCODE -ne 0 -or $lines.Count -eq 0) { throw 'immutable Expand canonical HCL could not be read' }
    return [pscustomobject]@{
      Canonical = ($lines -join "`n") + "`n"
      Target = $expandCommit
    }
  } finally {
    Pop-Location
  }
}

function Apply-CanonicalSchema {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$Canonical,
    [Parameter(Mandatory = $true)][int]$Port
  )
  Assert-DisposableSchemaName -Database $Database
  if ([regex]::Matches($Canonical, '(?m)^schema "admin" \{$').Count -ne 1) {
    throw 'canonical HCL has an invalid schema declaration'
  }
  $rebound = $Canonical.Replace('schema "admin" {', 'schema "' + $Database + '" {').Replace('schema.admin', "schema.$Database")
  if ([regex]::IsMatch($rebound, '\bschema\.admin\b')) { throw 'canonical HCL schema rebinding was incomplete' }

  $settings = [pscustomobject]@{
    User = 'root'
    Password = ''
    Host = '127.0.0.1'
    Port = [string]$Port
    Query = '?parseTime=true&multiStatements=true&loc=Local'
  }
  $runtimeDirectory = New-AtlasRuntimeConfig -Settings $settings -Database $Database
  try {
    Write-RestrictedTextFile -Path (Join-Path $runtimeDirectory 'admin.hcl') -Content $rebound
    [void](Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
      'schema', 'apply', '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime',
      '--to', 'file:///runtime/admin.hcl', '--auto-approve'
    ) -TimeoutSeconds $CommandTimeoutSeconds)
  } finally {
    Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
    $settings.Password = $null
  }
}

function Apply-ExpandMigration {
  param([Parameter(Mandatory = $true)][string]$Database)
  Assert-DisposableSchemaName -Database $Database
  $result = Invoke-CapturedContainerMySQL -Database $Database -SQL "SOURCE $migrationPath" -Operation 'apply only the 202608020101 Context Expand migration'
  if ($result.ExitCode -ne 0) {
    throw ('apply only the 202608020101 Context Expand migration failed: ' + $result.Output)
  }
}

function Get-FingerprintDocument {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][int]$Port
  )
  Assert-DisposableSchemaName -Database $Database
  $path = Join-Path $temporaryRoot ("fingerprint-$Database.json")
  $env:MYSQL_DSN = "root:@tcp(127.0.0.1:${Port})/${Database}?parseTime=true&multiStatements=true&loc=Local"
  Push-Location $backendRoot
  try {
    $commit = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') { throw 'current Git commit could not be resolved' }
    [void](Invoke-BoundedCommand -Executable $go -Arguments @(
      'run', './cmd/admin-db', 'fingerprint', '--schema', $Database,
      '--out', $path, '--commit', $commit
    ) -Operation 'capture Context Expand schema fingerprint' -TimeoutSeconds $CommandTimeoutSeconds)
  } finally {
    Pop-Location
  }
  $document = Get-Content -Raw -LiteralPath $path -Encoding utf8 | ConvertFrom-Json
  if ([string]$document.schema_sha256 -notmatch '^[0-9a-f]{64}$') { throw 'admin-db fingerprint output was invalid' }
  foreach ($table in @($document.tables)) {
    if ($null -eq $table.columns -or $null -eq $table.indexes) {
      throw 'admin-db fingerprint omitted columns or indexes'
    }
  }
  if ($null -eq $document.foreign_keys -or $null -eq $document.checks) {
    throw 'admin-db fingerprint omitted foreign keys or checks'
  }
  return $document
}

function Get-ComparableFingerprintJSON {
  param([Parameter(Mandatory = $true)]$Document)
  $copy = ($Document | ConvertTo-Json -Depth 100 | ConvertFrom-Json)
  $schema = [string]$copy.schema
  [void]$copy.PSObject.Properties.Remove('git_commit')
  [void]$copy.PSObject.Properties.Remove('schema_sha256')
  [void]$copy.PSObject.Properties.Remove('schema')
  foreach ($foreignKey in @($copy.foreign_keys)) {
    if ([string]$foreignKey.referenced_schema -ceq $schema) {
      $foreignKey.referenced_schema = ''
    }
  }
  return ($copy | ConvertTo-Json -Depth 100 -Compress)
}

function Get-FingerprintDifferences {
  param(
    [Parameter(Mandatory = $true)]$Expected,
    [Parameter(Mandatory = $true)]$Actual
  )
  $differences = [System.Collections.Generic.List[string]]::new()
  $firstCheckDifference = ''
  $expectedTables = @{}
  foreach ($table in @($Expected.tables)) { $expectedTables[[string]$table.name] = $table }
  $actualTables = @{}
  foreach ($table in @($Actual.tables)) { $actualTables[[string]$table.name] = $table }
  foreach ($tableName in @(($expectedTables.Keys + $actualTables.Keys) | Sort-Object -Unique)) {
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
      if ([string]::IsNullOrWhiteSpace($firstCheckDifference)) {
        $firstCheckDifference = "expected=$([string]$expectedChecks[$checkKey].clause); actual=$([string]$actualChecks[$checkKey].clause)"
      }
    }
  }
  if ($differences.Count -eq 0) { return 'unprojected fingerprint difference' }
  $summary = @($differences | Select-Object -First 40) -join ','
  if (-not [string]::IsNullOrWhiteSpace($firstCheckDifference)) { $summary += "; first-check: $firstCheckDifference" }
  return $summary
}

function Assert-ExcludedPlanItemIsRejected {
  param([Parameter(Mandatory = $true)][string]$Database)
  Assert-DisposableSchemaName -Database $Database
  $sql = @'
SET FOREIGN_KEY_CHECKS=0;
INSERT INTO `ai_context_plans` (
  `id`, `run_id`, `policy_version`, `input_fingerprint_sha256`, `plan_sha256`,
  `model_capability_sha256`, `api_protocol_snapshot`, `token_counter_id_snapshot`,
  `context_window_tokens`, `effective_output_tokens`, `provider_protocol_upper_bound`,
  `tool_continuation_input_reserve`, `policy_safety_margin`, `known_input_budget`,
  `known_input_upper_bound`, `budget_proof`, `retrieval_outcome`, `state`, `metrics_json`
) VALUES (
  9000000001, 9000000001, 'context-v1', UNHEX(REPEAT('1', 64)), UNHEX(REPEAT('2', 64)),
  UNHEX(REPEAT('3', 64)), 'responses', 'counter-v1',
  1000, 100, 50, 25, 50, 800, 700, 'exact', 'skipped', 'ready', JSON_OBJECT()
);
INSERT INTO `ai_context_plan_items` (
  `plan_id`, `ordinal`, `block_kind`, `source_type`, `source_ref`, `source_sha256`,
  `atomic_group_key`, `required`, `priority`, `decision`, `exclusion_reason`,
  `token_upper_bound`, `metadata_json`
) VALUES (
  9000000001, 1, 'recent_turn', 'message', 'message:1', UNHEX(REPEAT('4', 64)),
  'turn:1', 0, 0, 'excluded', NULL, 1, JSON_OBJECT()
);
'@
  $result = Invoke-CapturedContainerMySQL -Database $Database -SQL $sql -Operation 'probe excluded Context Plan Item constraint'
  if ($result.ExitCode -eq 0) { throw 'excluded Plan Item with a NULL exclusion reason was accepted' }
  if ($result.Output -notmatch 'chk_ai_context_plan_items_decision') {
    throw 'excluded Plan Item failed for an unexpected reason'
  }
}

function Assert-UppercasePlatformIsRejected {
  param([Parameter(Mandatory = $true)][string]$Database)
  Assert-DisposableSchemaName -Database $Database
  $sql = @'
SET FOREIGN_KEY_CHECKS=0;
INSERT INTO `ai_context_spaces` (
  `id`, `platform`, `profile_id`, `name`, `description`, `status`, `created_by`
) VALUES (
  9000000001, 'Admin', 9000000001, 'invalid uppercase platform', '', 'enabled', 9000000001
);
'@
  $result = Invoke-CapturedContainerMySQL -Database $Database -SQL $sql -Operation 'probe Context Space platform constraint'
  if ($result.ExitCode -eq 0) { throw 'uppercase Context Space platform was accepted' }
  if ($result.Output -notmatch 'chk_ai_context_spaces_platform') {
    throw 'uppercase Context Space platform failed for an unexpected reason'
  }
}

function Remove-DisposableSchemas {
  if (-not $containerReady) { return }
  for ($index = $createdDatabases.Count - 1; $index -ge 0; $index--) {
    $database = $createdDatabases[$index]
    if ($database -notmatch $disposableSchemaPattern) {
      $cleanupErrors.Add('refused unexpected disposable schema during cleanup')
      continue
    }
    try {
      [void](Invoke-ContainerMySQL -SQL "DROP DATABASE IF EXISTS ``$database``" -Operation 'drop disposable Context Expand schema')
    } catch {
      $cleanupErrors.Add("drop disposable Context Expand schema failed: $database")
    }
  }
}

function Remove-OwnedContainer {
  if (-not $containerCleanupRequired) { return }
  $target = if ($containerID -match '^[0-9a-f]{64}$') { $containerID } else { $containerName }
  try {
    $labelOutput = Invoke-BoundedCommand -Executable $docker -Arguments @(
      'inspect', '--format', "{{index .Config.Labels `"$containerLabelName`"}}", $target
    ) -Operation 'inspect disposable Context Expand MySQL ownership' -TimeoutSeconds 15
  } catch {
    return
  }
  $label = @($labelOutput.Trim())
  if ($label.Count -ne 1 -or [string]$label[0] -cne $runID) {
    throw 'refusing to remove an unowned Context Expand MySQL container'
  }
  [void](Invoke-BoundedCommand -Executable $docker -Arguments @(
    'rm', '--force', '--volumes', $target
  ) -Operation 'remove disposable Context Expand MySQL' -TimeoutSeconds 60)
}

function Remove-TemporaryRoot {
  if (-not (Test-Path -LiteralPath $temporaryRoot)) { return }
  $resolved = [System.IO.Path]::GetFullPath($temporaryRoot)
  $tempPrefix = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  if (-not $resolved.StartsWith($tempPrefix, [System.StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolved) -notmatch '^admin-ai-context-expand-[0-9a-f]{32}$') {
    throw 'refusing to remove an unexpected Context Expand temporary directory'
  }
  Remove-Item -LiteralPath $resolved -Recurse -Force
}

try {
  Push-Location $backendRoot
  try {
    & git cat-file -e "${BaselineCommit}^{commit}" 2>$null
    if ($LASTEXITCODE -ne 0) { throw 'baseline commit does not exist' }
    & git merge-base --is-ancestor $BaselineCommit HEAD
    if ($LASTEXITCODE -ne 0) { throw 'baseline commit is not an ancestor of HEAD' }
  } finally {
    Pop-Location
  }

  [void](New-Item -ItemType Directory -Path $temporaryRoot)
  $port = Start-DisposableMySQL
  New-DisposableDatabase -Database $beforeDatabase
  New-DisposableDatabase -Database $afterDatabase

  $baselineCanonical = Get-CanonicalAtCommit
  Apply-CanonicalSchema -Database $beforeDatabase -Canonical $baselineCanonical -Port $port
  Apply-ExpandMigration -Database $beforeDatabase

  $expandTarget = Get-ExpandCanonical
  Apply-CanonicalSchema -Database $afterDatabase -Canonical $expandTarget.Canonical -Port $port

  $beforeFingerprint = Get-FingerprintDocument -Database $beforeDatabase -Port $port
  $afterFingerprint = Get-FingerprintDocument -Database $afterDatabase -Port $port
  $beforeProjection = Get-ComparableFingerprintJSON -Document $beforeFingerprint
  $afterProjection = Get-ComparableFingerprintJSON -Document $afterFingerprint
  if ($beforeProjection -cne $afterProjection) {
    $diagnostic = Get-FingerprintDifferences -Expected $beforeFingerprint -Actual $afterFingerprint
    throw "migration result does not converge with canonical HCL fingerprint: $diagnostic"
  }
  if ([string]$beforeFingerprint.schema_sha256 -cne [string]$afterFingerprint.schema_sha256) {
    throw 'migration and canonical HCL schema hashes differ'
  }

  Assert-ExcludedPlanItemIsRejected -Database $afterDatabase
  Assert-UppercasePlatformIsRejected -Database $afterDatabase
  Write-Output ([ordered]@{
    status = 'passed'
    mysql_image = $mysqlImage
    baseline_commit = $BaselineCommit
    expand_target = [string]$expandTarget.Target
    migration = '202608020101_ai_context_expand.sql'
    schema_sha256 = [string]$afterFingerprint.schema_sha256
    excluded_plan_item_check = 'rejected'
    uppercase_platform_check = 'rejected'
  } | ConvertTo-Json -Compress)
} catch {
  $primaryError = $_
} finally {
  Remove-DisposableSchemas
  try { Remove-OwnedContainer } catch { $cleanupErrors.Add($_.Exception.Message) }
  try { Remove-TemporaryRoot } catch { $cleanupErrors.Add($_.Exception.Message) }
  [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
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
