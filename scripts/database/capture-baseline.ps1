[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [string]$RecoveryArtifact,

  [string]$OutputPath = (Join-Path ([System.IO.Path]::GetTempPath()) 'admin-p02-baseline.json'),

  [string]$MySQLCommand = 'mysql'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Get-DSNClientSettings {
  $dsn = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  if ([string]::IsNullOrWhiteSpace($dsn)) { throw 'MYSQL_DSN is required' }
  $markerIndex = $dsn.LastIndexOf('@tcp(', [System.StringComparison]::Ordinal)
  $addressStart = $markerIndex + 5
  $addressEnd = if ($markerIndex -gt 0) { $dsn.IndexOf(')/', $addressStart, [System.StringComparison]::Ordinal) } else { -1 }
  $credentials = if ($markerIndex -gt 0) { $dsn.Substring(0, $markerIndex) } else { '' }
  $credentialSeparator = $credentials.IndexOf(':')
  if ($markerIndex -le 0 -or $credentialSeparator -le 0 -or $addressEnd -le $addressStart) { throw 'MYSQL_DSN is not a supported TCP DSN' }
  $address = $dsn.Substring($addressStart, $addressEnd - $addressStart)
  $portSeparator = $address.LastIndexOf(':')
  $databaseStart = $addressEnd + 2
  $queryIndex = $dsn.IndexOf('?', $databaseStart)
  $dsnDatabase = if ($queryIndex -ge 0) { $dsn.Substring($databaseStart, $queryIndex - $databaseStart) } else { $dsn.Substring($databaseStart) }
  if ($portSeparator -le 0 -or $dsnDatabase -cne $Database) { throw 'MYSQL_DSN does not match requested schema' }
  return [pscustomobject]@{
    User=$credentials.Substring(0,$credentialSeparator)
    Password=$credentials.Substring($credentialSeparator+1)
    Host=$address.Substring(0,$portSeparator).Trim('[',']')
    Port=$address.Substring($portSeparator+1)
  }
}

function Invoke-MySQLQuery {
  param([Parameter(Mandatory = $true)][string]$SQL)
  $arguments = @('--protocol=tcp',"--host=$($script:Client.Host)","--port=$($script:Client.Port)","--user=$($script:Client.User)",'--batch','--skip-column-names','--raw',"--database=$Database","--execute=$SQL")
  $output = @(& $script:MySQLExecutable @arguments 2>$null)
  if ($LASTEXITCODE -ne 0) { throw 'baseline query failed' }
  return @($output | ForEach-Object { $_.ToString() })
}

function Move-FileWithOverwrite {
  param(
    [Parameter(Mandatory = $true)][string]$SourcePath,
    [Parameter(Mandatory = $true)][string]$DestinationPath
  )

  $overwriteMove = [IO.File].GetMethod('Move', [type[]]@([string], [string], [bool]))
  if ($null -ne $overwriteMove) {
    [void]$overwriteMove.Invoke($null, [object[]]@($SourcePath, $DestinationPath, $true))
    return
  }
  if ([IO.File]::Exists($DestinationPath)) {
    [IO.File]::Replace($SourcePath, $DestinationPath, $null)
    return
  }
  [IO.File]::Move($SourcePath, $DestinationPath)
}

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$artifactPath = (Resolve-Path -LiteralPath $RecoveryArtifact).Path
$artifact = Get-Content -Raw -LiteralPath $artifactPath -Encoding utf8 | ConvertFrom-Json
if ($artifact.verified -ne $true -or [string]$artifact.dump_sha256 -notmatch '^[0-9a-f]{64}$' -or -not (Test-Path -LiteralPath $artifact.dump_path -PathType Leaf)) {
  throw 'recovery artifact is not verified'
}
$actualRecoverySHA = (Get-FileHash -LiteralPath $artifact.dump_path -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualRecoverySHA -cne [string]$artifact.dump_sha256) { throw 'recovery dump checksum does not match artifact' }

$gitCommit = (& git -C $backendRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $gitCommit -notmatch '^[0-9a-f]{40}$') { throw 'Git commit could not be resolved' }
$fingerprintPath = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-p02-baseline-fingerprint-' + [guid]::NewGuid().ToString('N') + '.json')
$MySQLExecutable = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$Client = Get-DSNClientSettings
$previousMySQLPassword = [Environment]::GetEnvironmentVariable('MYSQL_PWD', 'Process')
$env:MYSQL_PWD = $Client.Password

try {
  Push-Location $backendRoot
  try {
    & go run ./cmd/admin-db fingerprint --schema $Database --out $fingerprintPath --commit $gitCommit 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'baseline fingerprint capture failed' }
  } finally { Pop-Location }
  $fingerprint = Get-Content -Raw -LiteralPath $fingerprintPath -Encoding utf8 | ConvertFrom-Json
  if ([string]$fingerprint.schema_sha256 -notmatch '^[0-9a-f]{64}$') { throw 'baseline fingerprint was invalid' }
  if ([string]$fingerprint.server_version -notmatch '^8\.4\.' -or [string]$fingerprint.sql_mode -notmatch 'NO_ENGINE_SUBSTITUTION') {
    throw 'baseline MySQL version or SQL mode is unsupported'
  }

  $countTables = @('cron_task_log','notifications','export_tasks','ai_runs','ai_image_tasks','user_sessions','users','wallet_transactions')
  $counts = [ordered]@{}
  foreach ($table in $countTables) {
    $exists = @(Invoke-MySQLQuery -SQL "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='$table'")
    if ($exists.Count -ne 1 -or $exists[0] -notmatch '^[01]$') { throw 'table inventory query was invalid' }
    if ($exists[0] -eq '1') {
      $value = @(Invoke-MySQLQuery -SQL "SELECT COUNT(*) FROM ``$table``")
      if ($value.Count -ne 1 -or $value[0] -notmatch '^\d+$') { throw "row count output was invalid for $table" }
      $counts[$table] = [long]$value[0]
    }
  }

  $platformValues = [ordered]@{}
  $platformTables = @(Invoke-MySQLQuery -SQL "SELECT table_name FROM information_schema.columns WHERE table_schema=DATABASE() AND column_name='platform' ORDER BY table_name")
  foreach ($table in $platformTables) {
    if ($table -notmatch '^[A-Za-z][A-Za-z0-9_]{0,63}$') { throw 'platform table name was invalid' }
    $rows = @(Invoke-MySQLQuery -SQL "SELECT COALESCE(CAST(platform AS CHAR),'<NULL>'),COUNT(*) FROM ``$table`` GROUP BY platform ORDER BY 1")
    $values = @()
    foreach ($row in $rows) {
      $parts = $row -split "`t",2
      if ($parts.Count -ne 2 -or $parts[1] -notmatch '^\d+$') { throw 'platform inventory output was invalid' }
      $values += [ordered]@{value=$parts[0];count=[long]$parts[1]}
    }
    $platformValues[$table] = $values
  }

  $objectReferenceCounts = @()
  $objectColumns = @(Invoke-MySQLQuery -SQL "SELECT table_name,column_name FROM information_schema.columns WHERE table_schema=DATABASE() AND column_name IN ('object_key','storage_key','file_url') ORDER BY table_name,column_name")
  foreach ($row in $objectColumns) {
    $parts = $row -split "`t",2
    if ($parts.Count -ne 2 -or $parts[0] -notmatch '^[A-Za-z][A-Za-z0-9_]{0,63}$' -or $parts[1] -notmatch '^[A-Za-z][A-Za-z0-9_]{0,63}$') { throw 'object reference inventory output was invalid' }
    $value = @(Invoke-MySQLQuery -SQL "SELECT COUNT(DISTINCT ``$($parts[1])``) FROM ``$($parts[0])`` WHERE ``$($parts[1])`` IS NOT NULL AND TRIM(CAST(``$($parts[1])`` AS CHAR))<>''")
    if ($value.Count -ne 1 -or $value[0] -notmatch '^\d+$') { throw 'object reference count was invalid' }
    $objectReferenceCounts += [ordered]@{table=$parts[0];column=$parts[1];distinct_count=[long]$value[0]}
  }

  $baseline = [ordered]@{
    database=$Database
    created_at=[DateTimeOffset]::UtcNow.ToString('o')
    git_commit=$gitCommit
    recovery_artifact=$artifactPath
    recovery_dump_sha256=$actualRecoverySHA
    schema_sha256=[string]$fingerprint.schema_sha256
    fingerprint=$fingerprint
    exact_counts=$counts
    platform_values=$platformValues
    object_reference_counts=$objectReferenceCounts
  }
  $temporaryPath = $OutputPath + '.tmp-' + [guid]::NewGuid().ToString('N')
  try {
    [System.IO.File]::WriteAllText($temporaryPath,(($baseline|ConvertTo-Json -Depth 100)+"`n"),[System.Text.UTF8Encoding]::new($false))
    Move-FileWithOverwrite -SourcePath $temporaryPath -DestinationPath ([System.IO.Path]::GetFullPath($OutputPath))
  } finally { Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue }
  Write-Output ([System.IO.Path]::GetFullPath($OutputPath))
  Write-Output ([string]$fingerprint.schema_sha256)
} finally {
  Remove-Item -LiteralPath $fingerprintPath -Force -ErrorAction SilentlyContinue
  [Environment]::SetEnvironmentVariable('MYSQL_PWD',$previousMySQLPassword,'Process')
  $Client.Password=$null
}
