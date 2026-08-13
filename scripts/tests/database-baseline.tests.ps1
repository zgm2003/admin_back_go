[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-True {
  param(
    [Parameter(Mandatory = $true)][bool]$Condition,
    [Parameter(Mandatory = $true)][string]$Message
  )
  if (-not $Condition) { throw $Message }
}

function Assert-Match {
  param(
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Pattern,
    [Parameter(Mandatory = $true)][string]$Message
  )
  if ($Text -notmatch $Pattern) { throw $Message }
}

function Assert-NotMatch {
  param(
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Pattern,
    [Parameter(Mandatory = $true)][string]$Message
  )
  if ($Text -match $Pattern) { throw $Message }
}

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$scriptPath = Join-Path $repoRoot 'scripts\database.ps1'
Assert-True (Test-Path -LiteralPath $scriptPath -PathType Leaf) 'scripts/database.ps1 is missing'

$source = [IO.File]::ReadAllText($scriptPath, [Text.Encoding]::UTF8)
Assert-Match $source "ValidateSet\('init','reset','migrate','check'\)" 'database command must expose exactly init, reset, migrate, and check'
Assert-Match $source 'Join-Path \$PSScriptRoot ''\.\.''' 'database command must resolve the repository from its own location'
Assert-Match $source 'ConfirmReset -cne ''admin''' 'reset must require the exact admin confirmation token'
Assert-Match $source 'Assert-ApplicationStopped' 'reset must refuse live API, Worker, and admin-dev processes'
Assert-Match $source 'Get-Process.*admin-api.*admin-worker' 'reset must refuse independently started host API and Worker processes'
Assert-Match $source "admin-state-mysql-1" 'MySQL operations must target the canonical state container'
Assert-Match $source "admin-state-redis-1" 'Redis operations must target the canonical state container'
Assert-Match $source "admin-state-qdrant-1" 'Qdrant operations must target the canonical state container'
Assert-Match $source "@\(0, 2, 3\)" 'reset must clear only the three project Redis databases'
Assert-NotMatch $source '(?i)FLUSHALL' 'database reset must never flush every Redis database'
Assert-Match $source "admin_context" 'Qdrant deletion must be restricted to the Admin context prefix'
Assert-Match $source 'delete_alias' 'Qdrant reset must explicitly delete project aliases'
Assert-Match $source 'schema_migrations' 'migration checksums must use the immutable migration ledger'
Assert-Match $source '202608130001' 'migration discovery must be anchored after the canonical baseline'
Assert-Match $source 'Get-FileHash.*SHA256' 'schema and migration bytes must be checked with SHA-256'
Assert-Match $source 'database.reference.address.sql' 'database command must own the canonical address reference path'
Assert-Match $source 'address_reference_sha256' 'database command must verify the address reference hash'
Assert-Match $source 'reference_row_counts.address' 'database command must read address counts from reference ownership metadata'
Assert-Match $source 'Invoke-SQLFile -Path \$addressReferencePath' 'database initialization must load the address reference'
Assert-Match $source 'ADMIN_INITIAL_PASSWORD' 'administrator password must use process environment input'
Assert-Match $source '& go -C \$repoRoot run ./cmd/admin-db' 'administrator creation must run from the resolved repository root'
Assert-NotMatch $source '(?i)(Write-(Host|Output)|echo).*?(MYSQL_DSN|ADMIN_INITIAL_PASSWORD|MYSQL_ROOT_PASSWORD)' 'database command may not print credentials'

$checkFunction = [regex]::Match(
  $source,
  '(?s)function Test-Database \{(?<body>.*?)\r?\n\}',
  [Text.RegularExpressions.RegexOptions]::CultureInvariant
)
Assert-True $checkFunction.Success 'database check implementation is missing'
Assert-NotMatch $checkFunction.Groups['body'].Value 'Invoke-Migrations' 'database check must never apply pending migrations'
Assert-Match $checkFunction.Groups['body'].Value 'Assert-Migrations' 'database check must validate migration state without changing it'
Assert-Match $checkFunction.Groups['body'].Value 'referential_constraints' 'database check must validate live foreign keys'
Assert-Match $checkFunction.Groups['body'].Value 'check_constraints' 'database check must validate live CHECK constraints'
Assert-Match $checkFunction.Groups['body'].Value 'statistics' 'database check must validate live unique indexes'
Assert-Match $checkFunction.Groups['body'].Value "index_name <> ''PRIMARY''" 'database check must count business unique indexes without table primary keys'
Assert-Match $checkFunction.Groups['body'].Value 'FROM address' 'database check must validate the address reference row count'
Assert-Match $checkFunction.Groups['body'].Value 'ADDRESS_REFERENCE_ORPHANED' 'database check must reject orphaned address nodes'

$failureOutput = & pwsh -NoProfile -File $scriptPath reset -ConfirmReset wrong 2>&1 | Out-String
Assert-True ($LASTEXITCODE -ne 0) 'reset accepted an invalid confirmation token'
Assert-Match $failureOutput 'DATABASE_RESET_CONFIRMATION_REQUIRED' 'reset returned an unstable confirmation failure'

$releaseSchema = [IO.File]::ReadAllText((Join-Path $repoRoot 'release\admin-only\release-manifest.schema.json'), [Text.Encoding]::UTF8)
foreach ($field in @('baseline_version', 'baseline_schema_sha256', 'baseline_seed_sha256', 'migration_checksums')) {
  Assert-Match $releaseSchema $field "release manifest schema is missing $field"
}
Assert-NotMatch $releaseSchema 'atlas_version|atlas_sum_sha256|target_fingerprint_sha256' 'release manifest still owns retired database evidence'

$databaseVerifier = [IO.File]::ReadAllText((Join-Path $repoRoot 'scripts\verify-database.ps1'), [Text.Encoding]::UTF8)
Assert-Match $databaseVerifier 'scripts[\\/]database\.ps1.*check' 'database verifier must delegate to the single database check entry'
Assert-NotMatch $databaseVerifier '(?i)atlas|reconcil' 'database verifier still runs the retired governance chain'

$durableVerifier = [IO.File]::ReadAllText((Join-Path $repoRoot 'scripts\verify-durable-work.ps1'), [Text.Encoding]::UTF8)
Assert-NotMatch $durableVerifier '(?i)arigaio/atlas|migrate.validate' 'durable-work verification still starts Atlas'

$databaseReadme = [IO.File]::ReadAllText((Join-Path $repoRoot 'database\README.md'), [Text.Encoding]::UTF8)
foreach ($command in @('database.ps1 init', 'database.ps1 reset', 'database.ps1 migrate', 'database.ps1 check')) {
  Assert-Match $databaseReadme ([regex]::Escape($command)) "database README is missing $command"
}
Assert-NotMatch $databaseReadme '(?i)atlas|reconcil|legacy-migrations|admin\.hcl' 'database README still documents the retired governance chain'

Write-Output 'database baseline command contracts passed'
