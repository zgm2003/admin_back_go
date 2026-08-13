[CmdletBinding()]
param(
  [Parameter(Mandatory = $true, Position = 0)]
  [ValidateSet('init','reset','migrate','check')]
  [string]$Action,

  [string]$ConfirmReset,
  [switch]$CreateAdmin,
  [string]$AdminUsername,
  [string]$AdminEmail
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$schemaPath = Join-Path $repoRoot 'database\schema.sql'
$seedPath = Join-Path $repoRoot 'database\seed.sql'
$addressReferencePath = Join-Path $repoRoot 'database\reference\address.sql'
$baselinePath = Join-Path $repoRoot 'database\baseline.json'
$migrationRoot = Join-Path $repoRoot 'database\migrations'
$runtimeEnvPath = Join-Path $repoRoot 'deploy\docker-first\admin-go.env'
$adminDevCommon = Join-Path $repoRoot 'scripts\dev\admin-dev-common.ps1'
$adminDevLock = Join-Path $repoRoot '.tmp\dev\admin-dev.lock.json'

$script:MySQLContainer = 'admin-state-mysql-1'
$script:RedisContainer = 'admin-state-redis-1'
$script:QdrantContainer = 'admin-state-qdrant-1'
$script:ApplicationContainers = @('admin-app-admin-api-1', 'admin-app-admin-worker-1')
$script:DatabaseName = 'admin'
$script:RedisDatabases = @(0, 2, 3)
$script:QdrantCollectionPrefix = 'admin_context'

function Read-RuntimeEnvironment {
  if (-not (Test-Path -LiteralPath $runtimeEnvPath -PathType Leaf)) {
    throw 'DATABASE_RUNTIME_ENV_MISSING'
  }
  $values = @{}
  foreach ($line in [IO.File]::ReadAllLines($runtimeEnvPath, [Text.Encoding]::UTF8)) {
    if ([string]::IsNullOrWhiteSpace($line) -or $line.TrimStart().StartsWith('#')) { continue }
    $separator = $line.IndexOf('=')
    if ($separator -le 0) { throw 'DATABASE_RUNTIME_ENV_INVALID' }
    $name = $line.Substring(0, $separator).Trim()
    if ($values.ContainsKey($name)) { throw 'DATABASE_RUNTIME_ENV_DUPLICATE_KEY' }
    $values[$name] = $line.Substring($separator + 1)
  }
  return $values
}

function Invoke-Docker {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  $output = & docker @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) { throw 'DATABASE_DOCKER_COMMAND_FAILED' }
  return @($output)
}

function Assert-StateContainer {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$Service
  )
  $identity = @(Invoke-Docker @(
    'inspect', $Name,
    '--format', '{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}|{{.State.Running}}'
  ))
  if ($identity.Count -ne 1 -or [string]$identity[0] -cne "admin-state|$Service|true") {
    throw 'DATABASE_STATE_CONTAINER_IDENTITY_MISMATCH'
  }
}

function Assert-ApplicationStopped {
  if (Test-Path -LiteralPath $adminDevCommon -PathType Leaf) {
    . $adminDevCommon
    if (Test-Path -LiteralPath $adminDevLock -PathType Leaf) {
      $record = Read-AdminDevLock -Path $adminDevLock
      if (Test-AdminDevLockLive -Record $record -RepositoryRoot $repoRoot) {
        throw 'DATABASE_RESET_APPLICATION_RUNNING'
      }
    }
  }
  foreach ($container in $script:ApplicationContainers) {
    $running = @(& docker inspect $container --format '{{.State.Running}}' 2>$null)
    if ($LASTEXITCODE -eq 0 -and $running.Count -eq 1 -and [string]$running[0] -ceq 'true') {
      throw 'DATABASE_RESET_APPLICATION_RUNNING'
    }
  }
  $hostProcesses = @(Get-Process -Name 'admin-api','admin-worker' -ErrorAction SilentlyContinue)
  if ($hostProcesses.Count -ne 0) { throw 'DATABASE_RESET_APPLICATION_RUNNING' }
}

function Invoke-MySQL {
  param(
    [Parameter(Mandatory = $true)][string]$SQL,
    [switch]$WithoutDatabase
  )
  $arguments = @(
    'exec', '-i', $script:MySQLContainer, 'sh', '-ec',
    'export MYSQL_PWD="$(cat /run/secrets/mysql_root_password)"; exec mysql --default-character-set=utf8mb4 --batch --skip-column-names "$@"',
    'admin-database', '--user=root'
  )
  if (-not $WithoutDatabase) { $arguments += $script:DatabaseName }
  $bytes = [Text.UTF8Encoding]::new($false).GetBytes($SQL)
  $process = [Diagnostics.Process]::new()
  $process.StartInfo = [Diagnostics.ProcessStartInfo]::new()
  $process.StartInfo.FileName = 'docker'
  $process.StartInfo.UseShellExecute = $false
  $process.StartInfo.RedirectStandardInput = $true
  $process.StartInfo.RedirectStandardOutput = $true
  $process.StartInfo.RedirectStandardError = $true
  foreach ($argument in $arguments) { $null = $process.StartInfo.ArgumentList.Add($argument) }
  if (-not $process.Start()) { throw 'DATABASE_MYSQL_START_FAILED' }
  $process.StandardInput.BaseStream.Write($bytes, 0, $bytes.Length)
  $process.StandardInput.Close()
  $stdout = $process.StandardOutput.ReadToEnd()
  $null = $process.StandardError.ReadToEnd()
  $process.WaitForExit()
  if ($process.ExitCode -ne 0) { throw 'DATABASE_MYSQL_COMMAND_FAILED' }
  return $stdout.Trim()
}

function Invoke-SQLFile {
  param([Parameter(Mandatory = $true)][string]$Path)
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw 'DATABASE_SQL_FILE_MISSING' }
  $sql = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
  $null = Invoke-MySQL -SQL $sql
}

function Assert-BaselineFiles {
  foreach ($path in @($schemaPath, $seedPath, $addressReferencePath, $baselinePath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'DATABASE_BASELINE_FILE_MISSING' }
  }
  $baseline = [IO.File]::ReadAllText($baselinePath, [Text.Encoding]::UTF8) | ConvertFrom-Json
  $schemaHash = (Get-FileHash -LiteralPath $schemaPath -Algorithm SHA256).Hash.ToLowerInvariant()
  $seedHash = (Get-FileHash -LiteralPath $seedPath -Algorithm SHA256).Hash.ToLowerInvariant()
  $addressReferenceHash = (Get-FileHash -LiteralPath $addressReferencePath -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($schemaHash -cne [string]$baseline.target.schema_sha256 -or
      $seedHash -cne [string]$baseline.target.seed_sha256 -or
      $addressReferenceHash -cne [string]$baseline.target.address_reference_sha256) {
    throw 'DATABASE_BASELINE_HASH_MISMATCH'
  }
  return $baseline
}

function Get-MigrationFiles {
  return @(
    Get-ChildItem -LiteralPath $migrationRoot -Filter '*.sql' -File -ErrorAction Stop |
      Where-Object { $_.BaseName -match '^(?<version>[0-9]{12})_[a-z0-9_]+$' -and $Matches.version -gt '202608130001' } |
      Sort-Object Name
  )
}

function Invoke-Migrations {
  foreach ($file in Get-MigrationFiles) {
    if ($file.BaseName -notmatch '^(?<version>[0-9]{12})_[a-z0-9_]+$') {
      throw 'DATABASE_MIGRATION_NAME_INVALID'
    }
    $version = $Matches.version
    if ($version -le '202608130001') { throw 'DATABASE_MIGRATION_VERSION_INVALID' }
    $checksum = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    $existing = Invoke-MySQL -SQL "SELECT checksum_sha256 FROM schema_migrations WHERE version = '$version';"
    if (-not [string]::IsNullOrEmpty($existing)) {
      if ($existing -cne $checksum) { throw 'DATABASE_MIGRATION_CHECKSUM_MISMATCH' }
      continue
    }
    $sql = [IO.File]::ReadAllText($file.FullName, [Text.Encoding]::UTF8)
    $ledgerSQL = "INSERT INTO schema_migrations (version, checksum_sha256) VALUES ('$version', '$checksum');"
    $null = Invoke-MySQL -SQL ("START TRANSACTION;`n" + $sql + "`n" + $ledgerSQL + "`nCOMMIT;")
  }
}

function Assert-Migrations {
  $appliedRows = Invoke-MySQL -SQL 'SELECT CONCAT(version, ''|'', checksum_sha256) FROM schema_migrations ORDER BY version;'
  $applied = @{}
  foreach ($row in @($appliedRows -split "`r?`n")) {
    if ([string]::IsNullOrWhiteSpace($row)) { continue }
    $parts = $row.Split('|')
    if ($parts.Count -ne 2 -or $parts[0] -notmatch '^[0-9]{12}$' -or $parts[1] -notmatch '^[0-9a-f]{64}$') {
      throw 'DATABASE_MIGRATION_LEDGER_INVALID'
    }
    $applied[$parts[0]] = $parts[1]
  }
  foreach ($file in Get-MigrationFiles) {
    if ($file.BaseName -notmatch '^(?<version>[0-9]{12})_[a-z0-9_]+$') {
      throw 'DATABASE_MIGRATION_NAME_INVALID'
    }
    $version = $Matches.version
    if ($version -le '202608130001') { throw 'DATABASE_MIGRATION_VERSION_INVALID' }
    $checksum = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    if (-not $applied.ContainsKey($version)) { throw 'DATABASE_PENDING_MIGRATION' }
    if ([string]$applied[$version] -cne $checksum) { throw 'DATABASE_MIGRATION_CHECKSUM_MISMATCH' }
    $applied.Remove($version)
  }
  $applied.Remove('202608130001')
  if ($applied.Count -ne 0) { throw 'DATABASE_MIGRATION_LEDGER_ORPHANED_VERSION' }
}

function Invoke-CreateAdmin {
  if (-not $CreateAdmin) { return }
  if ([string]::IsNullOrWhiteSpace($AdminUsername) -or [string]::IsNullOrWhiteSpace($AdminEmail)) {
    throw 'DATABASE_CREATE_ADMIN_IDENTITY_REQUIRED'
  }
  $previousPassword = [Environment]::GetEnvironmentVariable('ADMIN_INITIAL_PASSWORD', 'Process')
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  if ([string]::IsNullOrEmpty($env:ADMIN_INITIAL_PASSWORD)) {
    $credential = Get-Credential -UserName $AdminEmail -Message 'Enter the initial local administrator password'
    if ($null -eq $credential) { throw 'DATABASE_CREATE_ADMIN_PASSWORD_REQUIRED' }
    $plainPassword = [Net.NetworkCredential]::new('', $credential.Password).Password
    $env:ADMIN_INITIAL_PASSWORD = $plainPassword
    $plainPassword = $null
  }
  $runtime = Read-RuntimeEnvironment
  $containerDSN = [string]$runtime['MYSQL_DSN']
  if ($containerDSN -notmatch '@tcp\(mysql:3306\)/admin\?') { throw 'DATABASE_MYSQL_DSN_INVALID' }
  $env:MYSQL_DSN = $containerDSN -replace '@tcp\(mysql:3306\)', '@tcp(127.0.0.1:33306)'
  try {
    & go -C $repoRoot run ./cmd/admin-db create-admin --username $AdminUsername --email $AdminEmail --role-id 2
    if ($LASTEXITCODE -ne 0) { throw 'DATABASE_CREATE_ADMIN_FAILED' }
  }
  finally {
    [Environment]::SetEnvironmentVariable('ADMIN_INITIAL_PASSWORD', $previousPassword, 'Process')
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
  }
}

function Initialize-Database {
  Assert-StateContainer -Name $script:MySQLContainer -Service 'mysql'
  $tableCount = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = ''BASE TABLE'';'
  if ([int]$tableCount -ne 0) { throw 'DATABASE_INIT_REQUIRES_EMPTY_SCHEMA' }
  $null = Assert-BaselineFiles
  Invoke-SQLFile -Path $schemaPath
  Invoke-SQLFile -Path $addressReferencePath
  Invoke-SQLFile -Path $seedPath
  Invoke-Migrations
  Invoke-CreateAdmin
}

function Test-Database {
  $baseline = Assert-BaselineFiles
  Assert-StateContainer -Name $script:MySQLContainer -Service 'mysql'
  $tableCount = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = ''BASE TABLE'';'
  if ([int]$tableCount -ne [int]$baseline.target.base_table_count) { throw 'DATABASE_TABLE_COUNT_MISMATCH' }
  $foreignKeyCount = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE();'
  if ([int]$foreignKeyCount -ne [int]$baseline.target.foreign_key_count) { throw 'DATABASE_FOREIGN_KEY_COUNT_MISMATCH' }
  $checkConstraintCount = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM information_schema.check_constraints WHERE constraint_schema = DATABASE();'
  if ([int]$checkConstraintCount -ne [int]$baseline.target.check_constraint_count) { throw 'DATABASE_CHECK_CONSTRAINT_COUNT_MISMATCH' }
  $uniqueIndexCount = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM (SELECT table_name, index_name FROM information_schema.statistics WHERE table_schema = DATABASE() AND non_unique = 0 AND index_name <> ''PRIMARY'' GROUP BY table_name, index_name) AS unique_indexes;'
  if ([int]$uniqueIndexCount -ne [int]$baseline.target.unique_index_count) { throw 'DATABASE_UNIQUE_INDEX_COUNT_MISMATCH' }
  $baselineChecksum = Invoke-MySQL -SQL "SELECT checksum_sha256 FROM schema_migrations WHERE version = '$($baseline.baseline_version)';"
  if ($baselineChecksum -cne [string]$baseline.target.schema_sha256) { throw 'DATABASE_LEDGER_BASELINE_MISMATCH' }
  $seedFacts = Invoke-MySQL -SQL "SELECT CONCAT((SELECT COUNT(*) FROM permissions),'|',(SELECT COUNT(*) FROM roles),'|',(SELECT COUNT(*) FROM auth_platforms),'|',(SELECT COUNT(*) FROM system_settings),'|',(SELECT COUNT(*) FROM mail_templates),'|',(SELECT COUNT(*) FROM ai_tools));"
  $expected = "$($baseline.target.seed_row_counts.permissions)|$($baseline.target.seed_row_counts.roles)|$($baseline.target.seed_row_counts.auth_platforms)|$($baseline.target.seed_row_counts.system_settings)|$($baseline.target.seed_row_counts.mail_templates)|$($baseline.target.seed_row_counts.ai_tools)"
  if ($seedFacts -cne $expected) { throw 'DATABASE_SEED_FACTS_MISMATCH' }
  $addressCount = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM address WHERE is_del = 2;'
  if ([int]$addressCount -ne [int]$baseline.target.reference_row_counts.address) { throw 'DATABASE_ADDRESS_REFERENCE_COUNT_MISMATCH' }
  $addressOrphans = Invoke-MySQL -SQL 'SELECT COUNT(*) FROM address AS child LEFT JOIN address AS parent ON parent.id = child.parent_id AND parent.is_del = 2 WHERE child.is_del = 2 AND child.parent_id <> 0 AND parent.id IS NULL;'
  if ([int]$addressOrphans -ne 0) { throw 'DATABASE_ADDRESS_REFERENCE_ORPHANED' }
  Assert-Migrations
  Write-Output 'database baseline check passed'
}

function Clear-RedisState {
  Assert-StateContainer -Name $script:RedisContainer -Service 'redis'
  foreach ($database in $script:RedisDatabases) {
    $result = @(Invoke-Docker @('exec', $script:RedisContainer, 'redis-cli', '-n', [string]$database, '--raw', 'FLUSHDB'))
    if ($result.Count -ne 1 -or [string]$result[0] -cne 'OK') { throw 'DATABASE_REDIS_RESET_FAILED' }
  }
}

function Clear-QdrantState {
  Assert-StateContainer -Name $script:QdrantContainer -Service 'qdrant'
  $runtime = Read-RuntimeEnvironment
  if ([string]$runtime['QDRANT_COLLECTION_PREFIX'] -cne $script:QdrantCollectionPrefix) {
    throw 'DATABASE_QDRANT_PREFIX_MISMATCH'
  }
  $aliasResponse = Invoke-RestMethod -Method Get -Uri 'http://127.0.0.1:36333/aliases'
  foreach ($alias in @($aliasResponse.result.aliases)) {
    $aliasName = [string]$alias.alias_name
    if ($aliasName.StartsWith($script:QdrantCollectionPrefix + '_', [StringComparison]::Ordinal)) {
      $body = @{ actions = @(@{ delete_alias = @{ alias_name = $aliasName } }) } | ConvertTo-Json -Depth 4 -Compress
      $null = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:36333/collections/aliases' -ContentType 'application/json' -Body $body
    }
  }
  $response = Invoke-RestMethod -Method Get -Uri 'http://127.0.0.1:36333/collections'
  foreach ($collection in @($response.result.collections)) {
    $name = [string]$collection.name
    if ($name.StartsWith($script:QdrantCollectionPrefix + '_', [StringComparison]::Ordinal)) {
      $encodedName = [Uri]::EscapeDataString($name)
      $null = Invoke-RestMethod -Method Delete -Uri "http://127.0.0.1:36333/collections/$encodedName"
    }
  }
}

function Reset-Database {
  if ($ConfirmReset -cne 'admin') { throw 'DATABASE_RESET_CONFIRMATION_REQUIRED' }
  Assert-ApplicationStopped
  Assert-StateContainer -Name $script:MySQLContainer -Service 'mysql'
  $null = Invoke-MySQL -WithoutDatabase -SQL 'DROP DATABASE IF EXISTS `admin`; CREATE DATABASE `admin` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;'
  Clear-RedisState
  Clear-QdrantState
  Initialize-Database
}

switch ($Action) {
  'init' { Initialize-Database }
  'reset' { Reset-Database }
  'migrate' {
    $null = Assert-BaselineFiles
    Assert-StateContainer -Name $script:MySQLContainer -Service 'mysql'
    Invoke-Migrations
  }
  'check' { Test-Database }
}
