[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[0-9a-f]{64}$')]
  [string]$ExpectedSourceFingerprint,

  [Parameter(Mandatory = $true)]
  [string]$InputLock,

  [string]$DestructiveApproval = '',
  [string]$ReleaseManifest = '',
  [string]$MySQLCommand = 'mysql',
  [string]$DockerCommand = 'docker',
  [switch]$Apply,
  [ValidateRange(30, 1800)][int]$CommandTimeoutSeconds = 600
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'atlas-runtime-common.ps1')

function Resolve-RequiredFile {
  param([Parameter(Mandatory = $true)][string]$Path, [Parameter(Mandatory = $true)][string]$Label)
  if ([string]::IsNullOrWhiteSpace($Path)) { throw "$Label path is required" }
  $resolved = Resolve-Path -LiteralPath $Path -ErrorAction Stop
  if (-not (Test-Path -LiteralPath $resolved.Path -PathType Leaf)) { throw "$Label file is missing" }
  return [System.IO.Path]::GetFullPath($resolved.Path)
}

function Assert-InputLock {
  param([Parameter(Mandatory = $true)][string]$Path)
  $lockPath = Resolve-RequiredFile -Path $Path -Label 'input lock'
  $schemaPath = Join-Path $script:BackendRoot 'release\admin-only\input-lock.schema.json'
  if (-not (Test-Path -LiteralPath $schemaPath -PathType Leaf)) { throw 'input lock schema is missing' }
  $json = [System.IO.File]::ReadAllText($lockPath, [System.Text.Encoding]::UTF8)
  if (-not ($json | Test-Json -SchemaFile $schemaPath -ErrorAction SilentlyContinue)) {
    throw 'input lock does not match its schema'
  }
  return $lockPath
}

function Invoke-GoAdminDB {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  Push-Location $script:BackendRoot
  try {
    $output = @(& go @Arguments 2>$null)
    if ($LASTEXITCODE -ne 0) { throw 'admin-db command failed' }
    return @($output | ForEach-Object { $_.ToString() })
  } finally {
    Pop-Location
  }
}

function Get-CurrentSchemaFingerprint {
  return Get-DatabaseFingerprintSHA -BackendRoot $script:BackendRoot -Settings $script:Settings -Database $Database
}

function Invoke-InvariantFile {
  param([Parameter(Mandatory = $true)][string]$RelativePath)
  $path = Join-Path $script:BackendRoot $RelativePath
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "invariant file is missing: $RelativePath" }
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  try {
    $env:MYSQL_DSN = New-SchemaDSN -Settings $script:Settings -Database $Database
    $lines = Invoke-GoAdminDB -Arguments @('run', './cmd/admin-db', 'invariants', '--schema', $Database, '--file', $RelativePath)
    [uint64]$total = 0
    foreach ($line in $lines) {
      $parts = [string]$line -split "`t", 2
      if ($parts.Count -ne 2 -or $parts[1] -notmatch '^[0-9]+$') { throw 'database invariant output was invalid' }
      $total += [uint64]$parts[1]
    }
    if ($total -ne 0) { throw "database invariant violations detected: $RelativePath" }
  } finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
  }
}

function Invoke-COSReachability {
  $outputPath = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-p09-cos-' + [guid]::NewGuid().ToString('N') + '.json')
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  try {
    $env:MYSQL_DSN = New-SchemaDSN -Settings $script:Settings -Database $Database
    [void](Invoke-GoAdminDB -Arguments @('run', './cmd/admin-db', 'cos-references', '--schema', $Database, '--out', $outputPath))
  } finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    Remove-Item -LiteralPath $outputPath -Force -ErrorAction SilentlyContinue
  }
}

function Invoke-LockedAtlasApply {
  param([Parameter(Mandatory = $true)][string]$Version)
  $currentFingerprint = Get-CurrentSchemaFingerprint
  $runtimeDirectory = New-AtlasRuntimeConfig -Settings $script:Settings -Database $Database
  try {
    # admin-db lock-run keeps one MySQL connection and one lock while Atlas applies the group.
    $dockerArguments = @(
      'run', '--rm', '--add-host', 'host.docker.internal:host-gateway',
      '--volume', "${script:BackendRoot}:/workspace:ro",
      '--volume', "${runtimeDirectory}:/runtime:ro",
      '--workdir', '/workspace',
      $script:AtlasImage,
      'migrate', 'apply',
      '--config', 'file:///runtime/atlas.hcl',
      '--env', 'runtime',
      '--dir', 'file:///workspace/database/migrations',
      '--to-version', $Version
    )
    $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
    try {
      $env:MYSQL_DSN = New-SchemaDSN -Settings $script:Settings -Database $Database
      [void](Invoke-GoAdminDB -Arguments (@(
        'run', './cmd/admin-db', 'lock-run',
        '--schema', $Database,
        '--name', 'admin:atlas:migrate',
        '--timeout', '30s',
        '--expected-fingerprint', $currentFingerprint,
        '--'
      ) + @($script:DockerExecutable) + $dockerArguments))
    } finally {
      [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    }
  } finally {
    Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
  }
}

if (-not $Apply) { throw 'contract admin-only migration requires explicit -Apply' }

$script:BackendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
[void](Assert-InputLock -Path $InputLock)

if ($Database -ceq 'admin' -and [string]::IsNullOrWhiteSpace($ReleaseManifest)) {
  throw 'live admin schema requires a validated release manifest'
}
if (-not [string]::IsNullOrWhiteSpace($ReleaseManifest)) {
  [void](Resolve-RequiredFile -Path $ReleaseManifest -Label 'release manifest')
}

$script:MySQLExecutable = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$script:DockerExecutable = (Get-Command -Name $DockerCommand -ErrorAction Stop | Select-Object -First 1).Source
$script:Settings = Get-MySQLDSNSettings -Database $Database
try {
  $sourceFingerprint = Get-CurrentSchemaFingerprint
  if ($sourceFingerprint -cne $ExpectedSourceFingerprint) { throw 'source fingerprint does not match expected value' }

  Invoke-InvariantFile -RelativePath 'database/reconciliation/050_contract_preconditions.sql'
  Invoke-LockedAtlasApply -Version '202607150201'
  Invoke-InvariantFile -RelativePath 'database/reconciliation/051_verify_admin_rows.sql'

  if ($DestructiveApproval -cne 'DROP_CLIENT_VERSIONS_FOR_P09') {
    throw 'P09_DESTRUCTIVE_APPROVAL_REQUIRED'
  }

  Invoke-LockedAtlasApply -Version '202607150202'
  Invoke-InvariantFile -RelativePath 'database/reconciliation/052_verify_ai_contract.sql'
  Invoke-COSReachability
  Invoke-LockedAtlasApply -Version '202607150203'
  Invoke-InvariantFile -RelativePath 'database/reconciliation/053_verify_admin_only.sql'

  [void](& pwsh -NoProfile -File (Join-Path $PSScriptRoot 'check-drift.ps1') -Database $Database -MySQLCommand $script:MySQLExecutable -DockerCommand $script:DockerExecutable -CommandTimeoutSeconds $CommandTimeoutSeconds)
  if ($LASTEXITCODE -ne 0) { throw 'post-contract drift check failed' }
} finally {
  $script:Settings.Password = $null
}
