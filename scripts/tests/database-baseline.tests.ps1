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

$failureOutput = & pwsh -NoProfile -File $scriptPath reset -ConfirmReset wrong 2>&1 | Out-String
Assert-True ($LASTEXITCODE -ne 0) 'reset accepted an invalid confirmation token'
Assert-Match $failureOutput 'DATABASE_RESET_CONFIRMATION_REQUIRED' 'reset returned an unstable confirmation failure'

Write-Output 'database baseline command contracts passed'
