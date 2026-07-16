[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [string]$MySQLCommand = 'mysql',
  [string]$DockerCommand = 'docker',
  [ValidateRange(30, 1800)][int]$CommandTimeoutSeconds = 300
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'atlas-runtime-common.ps1')

$DisposableSchemaPattern = '^admin_(empty|imported)_[0-9a-f]{12}$'
$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$schemaPath = Join-Path $backendRoot 'database\schema\admin.hcl'
if (-not (Test-Path -LiteralPath $schemaPath -PathType Leaf)) { throw 'canonical admin.hcl is missing' }
$mysql = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$docker = (Get-Command -Name $DockerCommand -ErrorAction Stop | Select-Object -First 1).Source
$settings = Get-MySQLDSNSettings -Database $Database
$emptyDatabase = 'admin_empty_' + ([guid]::NewGuid().ToString('N').Substring(0, 12))
if ($emptyDatabase -notmatch $DisposableSchemaPattern) { throw 'generated disposable schema name is invalid' }
$runtimeDirectory = ''
$created = $false

try {
  $targetFingerprint = Get-DatabaseFingerprintSHA -BackendRoot $backendRoot -Settings $settings -Database $Database
  [void](Invoke-MySQLStatement -MySQLExecutable $mysql -Settings $settings -SQL "CREATE DATABASE ``$emptyDatabase`` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
  $created = $true
  $runtimeDirectory = New-AtlasRuntimeConfig -Settings $settings -Database $emptyDatabase
  $canonicalSchema = [System.IO.File]::ReadAllText($schemaPath, [System.Text.Encoding]::UTF8)
  if ([regex]::Matches($canonicalSchema, '(?m)^schema "admin" \{$').Count -ne 1) {
    throw 'canonical schema must contain exactly one admin schema declaration'
  }
  $runtimeSchema = $canonicalSchema.Replace('schema "admin" {', 'schema "' + $emptyDatabase + '" {')
  $runtimeSchema = $runtimeSchema.Replace('schema.admin', "schema.$emptyDatabase")
  if ([regex]::IsMatch($runtimeSchema, '\bschema\.admin\b')) { throw 'canonical schema reference rebinding was incomplete' }
  $runtimeSchemaPath = Join-Path $runtimeDirectory 'admin.hcl'
  [System.IO.File]::WriteAllText($runtimeSchemaPath, $runtimeSchema, [System.Text.UTF8Encoding]::new($false))
  [void](Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
    'schema', 'apply', '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime',
    '--to', 'file:///runtime/admin.hcl', '--auto-approve'
  ) -TimeoutSeconds $CommandTimeoutSeconds)
  $emptyFingerprint = Get-DatabaseFingerprintSHA -BackendRoot $backendRoot -Settings $settings -Database $emptyDatabase
  if ($emptyFingerprint -cne $targetFingerprint) { throw 'canonical empty schema fingerprint does not match imported schema fingerprint' }
  [ordered]@{
    database = $Database
    schema_sha256 = $targetFingerprint
    empty_schema_sha256 = $emptyFingerprint
    drift = 0
  } | ConvertTo-Json -Compress
} finally {
  Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
  if ($created) {
    if ($emptyDatabase -notmatch $DisposableSchemaPattern) { throw 'refusing to drop unexpected schema name' }
    [void](Invoke-MySQLStatement -MySQLExecutable $mysql -Settings $settings -SQL "DROP DATABASE ``$emptyDatabase``")
  }
  $settings.Password = $null
}
