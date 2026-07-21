[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[0-9a-f]{64}$')]
  [string]$ExpectedFingerprint,

  [string]$MySQLCommand = 'mysql',
  [string]$DockerCommand = 'docker',
  [string]$OutputPath = '',
  [ValidateRange(30, 1800)][int]$CommandTimeoutSeconds = 300
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'atlas-runtime-common.ps1')

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
  $OutputPath = Join-Path $backendRoot 'database\schema\admin.hcl'
}
$outputFullPath = [System.IO.Path]::GetFullPath($OutputPath)
$schemaRoot = [System.IO.Path]::GetFullPath((Join-Path $backendRoot 'database\schema'))
if (-not $outputFullPath.StartsWith($schemaRoot.TrimEnd('\') + '\', [System.StringComparison]::OrdinalIgnoreCase)) {
  throw 'canonical schema output must stay under database/schema'
}
$mysql = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$docker = (Get-Command -Name $DockerCommand -ErrorAction Stop | Select-Object -First 1).Source
$settings = Get-MySQLDSNSettings -Database $Database
$runtimeDirectory = ''

try {
  $beforeFingerprint = Get-DatabaseFingerprintSHA -BackendRoot $backendRoot -Settings $settings -Database $Database
  if ($beforeFingerprint -cne $ExpectedFingerprint) { throw 'source fingerprint does not match expected value' }

  $runtimeDirectory = New-AtlasRuntimeConfig -Settings $settings -Database $Database
  [void](Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
    'migrate', 'status', '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime', '--dir', 'file:///workspace/database/migrations'
  ) -TimeoutSeconds $CommandTimeoutSeconds)

  $dockerArguments = @(
    'run', '--rm', '--add-host', 'host.docker.internal:host-gateway',
    '--volume', "${backendRoot}:/workspace:ro",
    '--volume', "${runtimeDirectory}:/runtime:ro",
    '--workdir', '/workspace',
    $script:AtlasImage,
    'migrate', 'set', '202607150001',
    '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime',
    '--dir', 'file:///workspace/database/migrations'
  )
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  try {
    $env:MYSQL_DSN = New-SchemaDSN -Settings $settings -Database $Database
    Push-Location $backendRoot
    try {
      $lockArguments = @('run', './cmd/admin-db', 'lock-run', '--schema', $Database, '--name', 'admin:atlas:migrate', '--timeout', '30s', '--expected-fingerprint', $ExpectedFingerprint, '--') + @($docker) + $dockerArguments
      [void](Invoke-BoundedCommand -Executable 'go' -Arguments $lockArguments -Operation 'initialize Atlas baseline under lock' -TimeoutSeconds $CommandTimeoutSeconds)
    } finally {
      Pop-Location
    }
  } finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
  }

  $statusOutput = Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
    'migrate', 'status', '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime', '--dir', 'file:///workspace/database/migrations'
  ) -TimeoutSeconds $CommandTimeoutSeconds
  if ($statusOutput -notmatch '(?i)migration status:\s*ok') { throw 'Atlas revision history is not clean' }
  [void](Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
    'migrate', 'validate', '--dir', 'file:///workspace/database/migrations'
  ) -NetworkNone -TimeoutSeconds $CommandTimeoutSeconds)

  $schemaText = Invoke-AtlasContainer -DockerExecutable $docker -BackendRoot $backendRoot -RuntimeDirectory $runtimeDirectory -AtlasArguments @(
    'schema', 'inspect', '--config', 'file:///runtime/atlas.hcl', '--env', 'runtime'
  ) -TimeoutSeconds $CommandTimeoutSeconds
  if ([string]::IsNullOrWhiteSpace($schemaText)) { throw 'Atlas schema inspection returned no schema' }
  $schemaNamePattern = [regex]::Escape($Database)
  $schemaText = [regex]::Replace($schemaText, "schema\.$schemaNamePattern\b", 'schema.admin')
  $schemaText = [regex]::Replace($schemaText, "schema\s+`"$schemaNamePattern`"", 'schema "admin"')
  $schemaText = $schemaText.TrimEnd() + "`n"
  if ($schemaText -match '(?mi)^\s*auto_increment\s*=\s*[0-9]+\s*$') { throw 'Atlas schema contains a volatile auto-increment counter' }
  if ($schemaText -match '(?i)\bdefiner\b') { throw 'Atlas schema contains a definer' }
  if ($schemaText -notmatch 'table\s+"atlas_schema_revisions"' -or $schemaText -notmatch 'table\s+"users"') {
    throw 'Atlas schema is missing required baseline tables'
  }

  $validationPath = Join-Path $runtimeDirectory 'admin.hcl'
  [System.IO.File]::WriteAllText($validationPath, $schemaText, [System.Text.UTF8Encoding]::new($false))
  [void](Invoke-BoundedCommand -Executable $docker -Arguments @(
    'run', '--rm', '--network', 'none',
    '--volume', "${runtimeDirectory}:/runtime:rw",
    $script:AtlasImage,
    'schema', 'fmt', '/runtime/admin.hcl'
  ) -Operation 'validate Atlas HCL' -TimeoutSeconds $CommandTimeoutSeconds)
  $schemaText = [System.IO.File]::ReadAllText($validationPath, [System.Text.Encoding]::UTF8).TrimEnd() + "`n"

  [void](New-Item -ItemType Directory -Path $schemaRoot -Force)
  $temporaryOutput = $outputFullPath + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
  try {
    [System.IO.File]::WriteAllText($temporaryOutput, $schemaText, [System.Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporaryOutput -Destination $outputFullPath -Force
  } finally {
    Remove-Item -LiteralPath $temporaryOutput -Force -ErrorAction SilentlyContinue
  }

  $afterFingerprint = Get-DatabaseFingerprintSHA -BackendRoot $backendRoot -Settings $settings -Database $Database
  Write-Output $outputFullPath
  Write-Output $afterFingerprint
} finally {
  Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
  $settings.Password = $null
}
