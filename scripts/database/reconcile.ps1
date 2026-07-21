[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [ValidateSet('ledger', 'expand', 'backfill-core', 'backfill-ai', 'proven-indexes', 'ai-image-soft-delete', 'export-cleanup-schedule', 'realtime-retention', 'cron-task-utf8-metadata', 'browser-only-retirement', 'post-contract', 'all-nondestructive')]
  [string]$Stage,

  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[0-9a-f]{64}$')]
  [string]$ExpectedSourceFingerprint,

  [string]$MySQLCommand = 'mysql',

  [string]$Executor = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function ConvertTo-SQLString {
  param([Parameter(Mandatory = $true)][string]$Value)
  return "'" + $Value.Replace("'", "''") + "'"
}

function Get-DSNClientSettings {
  $dsn = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  if ([string]::IsNullOrWhiteSpace($dsn)) {
    throw 'MYSQL_DSN is required'
  }
  $marker = '@tcp('
  $markerIndex = $dsn.LastIndexOf($marker, [System.StringComparison]::Ordinal)
  $credentialSeparator = if ($markerIndex -gt 0) { $dsn.Substring(0, $markerIndex).IndexOf(':') } else { -1 }
  $addressStart = $markerIndex + $marker.Length
  $addressEnd = if ($markerIndex -gt 0) { $dsn.IndexOf(')/', $addressStart, [System.StringComparison]::Ordinal) } else { -1 }
  if ($markerIndex -le 0 -or $credentialSeparator -le 0 -or $addressEnd -le $addressStart) {
    throw 'MYSQL_DSN is not a supported TCP DSN'
  }
  $credentials = $dsn.Substring(0, $markerIndex)
  $address = $dsn.Substring($addressStart, $addressEnd - $addressStart)
  $portSeparator = $address.LastIndexOf(':')
  if ($portSeparator -le 0) {
    throw 'MYSQL_DSN address is malformed'
  }
  $databaseStart = $addressEnd + 2
  $queryIndex = $dsn.IndexOf('?', $databaseStart)
  $dsnDatabase = if ($queryIndex -ge 0) { $dsn.Substring($databaseStart, $queryIndex - $databaseStart) } else { $dsn.Substring($databaseStart) }
  if ($dsnDatabase -cne $Database) {
    throw 'MYSQL_DSN database does not match requested schema'
  }
  return [pscustomobject]@{
    User     = $credentials.Substring(0, $credentialSeparator)
    Password = $credentials.Substring($credentialSeparator + 1)
    Host     = $address.Substring(0, $portSeparator).Trim('[', ']')
    Port     = $address.Substring($portSeparator + 1)
  }
}

function Invoke-MySQL {
  param(
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Operation
  )
  $output = @(& $script:MySQLExecutable @Arguments 2>$null)
  if ($LASTEXITCODE -ne 0) {
    throw "$Operation failed"
  }
  return @($output | ForEach-Object { $_.ToString() })
}

function Get-SchemaFingerprint {
  $outputPath = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-p02-fingerprint-' + [guid]::NewGuid().ToString('N') + '.json')
  try {
    Push-Location $script:BackendRoot
    try {
      & go run ./cmd/admin-db fingerprint --schema $Database --out $outputPath --commit $script:GitCommit 2>$null | Out-Null
      if ($LASTEXITCODE -ne 0) {
        throw 'schema fingerprint capture failed'
      }
    } finally {
      Pop-Location
    }
    $document = Get-Content -Raw -LiteralPath $outputPath -Encoding utf8 | ConvertFrom-Json
    if ([string]$document.schema_sha256 -notmatch '^[0-9a-f]{64}$') {
      throw 'schema fingerprint output was invalid'
    }
    return [string]$document.schema_sha256
  } finally {
    Remove-Item -LiteralPath $outputPath -Force -ErrorAction SilentlyContinue
  }
}

$BackendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$ReconciliationRoot = Join-Path $BackendRoot 'database\reconciliation'
$GitCommit = (& git -C $BackendRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $GitCommit -notmatch '^[0-9a-f]{40}$') {
  throw 'Git commit could not be resolved'
}
$MySQLExecutable = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$client = Get-DSNClientSettings
if ([string]::IsNullOrWhiteSpace($Executor)) {
  $Executor = [Environment]::UserName
}
if ($Executor -notmatch '^[A-Za-z0-9_.@-]{1,191}$') {
  throw 'Executor contains unsupported characters'
}
$previousMySQLPassword = [Environment]::GetEnvironmentVariable('MYSQL_PWD', 'Process')
$env:MYSQL_PWD = $client.Password

$stageFiles = [ordered]@{
  'ledger' = @('001_ledger.sql')
  'expand' = @('010_expand_core.sql')
  'backfill-core' = @('020_backfill_core.sql')
  'backfill-ai' = @('021_backfill_ai.sql')
  'proven-indexes' = @('041_apply_proven_indexes.sql')
  'ai-image-soft-delete' = @('042_add_ai_image_soft_delete.sql')
  'export-cleanup-schedule' = @('043_register_export_cleanup.sql')
  'realtime-retention' = @('044_realtime_retention.sql')
  'cron-task-utf8-metadata' = @('045_repair_cron_task_utf8_metadata.sql')
  'browser-only-retirement' = @('046_retire_client_version_surface.sql')
  'post-contract' = @('001_ledger.sql', '010_expand_core.sql', '020_backfill_core.sql', '041_apply_proven_indexes.sql', '042_add_ai_image_soft_delete.sql', '043_register_export_cleanup.sql', '044_realtime_retention.sql', '045_repair_cron_task_utf8_metadata.sql')
  'all-nondestructive' = @('001_ledger.sql', '010_expand_core.sql', '020_backfill_core.sql', '021_backfill_ai.sql', '041_apply_proven_indexes.sql', '042_add_ai_image_soft_delete.sql', '043_register_export_cleanup.sql', '044_realtime_retention.sql', '045_repair_cron_task_utf8_metadata.sql', '046_retire_client_version_surface.sql')
}
$files = @($stageFiles[$Stage] | Where-Object { Test-Path -LiteralPath (Join-Path $ReconciliationRoot $_) -PathType Leaf })
if ($files.Count -eq 0) {
  throw "no reconciliation SQL files exist for stage $Stage"
}

try {
  $sourceFingerprint = Get-SchemaFingerprint
  if ($sourceFingerprint -cne $ExpectedSourceFingerprint) {
    throw 'source fingerprint does not match expected value'
  }
  $baseArguments = @(
    '--protocol=tcp',
    "--host=$($client.Host)",
    "--port=$($client.Port)",
    "--user=$($client.User)",
    '--default-character-set=utf8mb4',
    '--batch',
    '--skip-column-names',
    '--raw',
    "--database=$Database"
  )

  foreach ($fileName in $files) {
    $filePath = Join-Path $ReconciliationRoot $fileName
    $scriptSHA = (Get-FileHash -LiteralPath $filePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($fileName -eq '001_ledger.sql') {
      $sourcePath = $filePath.Replace('\', '/')
      [void](Invoke-MySQL -Arguments ($baseArguments + "--execute=SOURCE $sourcePath") -Operation 'bootstrap reconciliation ledger')
    }

    $nameSQL = ConvertTo-SQLString -Value $fileName
    $history = @(Invoke-MySQL -Arguments ($baseArguments + "--execute=SELECT script_sha256, status FROM schema_reconciliation_runs WHERE script_name=$nameSQL ORDER BY id") -Operation 'read reconciliation history')
    foreach ($row in $history) {
      $parts = $row -split "`t", 2
      if ($parts.Count -ne 2 -or $parts[0] -notmatch '^[0-9a-f]{64}$') {
        throw 'reconciliation history output was invalid'
      }
      if ($parts[0] -cne $scriptSHA) {
        throw "reconciliation history checksum drift for $fileName"
      }
      if ($parts[1] -eq 'succeeded') {
        Write-Output "SKIP $fileName $scriptSHA"
        continue
      }
    }
    if (@($history | Where-Object { ($_ -split "`t", 2)[1] -eq 'succeeded' }).Count -gt 0) {
      continue
    }

    $shaSQL = ConvertTo-SQLString -Value $scriptSHA
    $stageSQL = ConvertTo-SQLString -Value $Stage
    $sourceSQL = ConvertTo-SQLString -Value $sourceFingerprint
    $executorSQL = ConvertTo-SQLString -Value $Executor
    $runningSQL = "INSERT INTO schema_reconciliation_runs(stage,script_name,script_sha256,source_fingerprint_sha256,executor,status,started_at) VALUES($stageSQL,$nameSQL,$shaSQL,$sourceSQL,$executorSQL,'running',UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE stage=VALUES(stage),source_fingerprint_sha256=VALUES(source_fingerprint_sha256),executor=VALUES(executor),status='running',details_json=NULL,started_at=UTC_TIMESTAMP(6),finished_at=NULL"
    [void](Invoke-MySQL -Arguments ($baseArguments + "--execute=$runningSQL") -Operation 'record reconciliation start')

    try {
      $sourcePath = $filePath.Replace('\', '/')
      [void](Invoke-MySQL -Arguments ($baseArguments + "--execute=SOURCE $sourcePath") -Operation "execute $fileName")
      $targetFingerprint = Get-SchemaFingerprint
      $targetSQL = ConvertTo-SQLString -Value $targetFingerprint
      $successSQL = "UPDATE schema_reconciliation_runs SET target_fingerprint_sha256=$targetSQL,status='succeeded',details_json=NULL,finished_at=UTC_TIMESTAMP(6) WHERE script_name=$nameSQL AND script_sha256=$shaSQL"
      [void](Invoke-MySQL -Arguments ($baseArguments + "--execute=$successSQL") -Operation 'record reconciliation success')
      Write-Output "APPLY $fileName $scriptSHA $targetFingerprint"
    } catch {
      $failedSQL = "UPDATE schema_reconciliation_runs SET status='failed',details_json=JSON_OBJECT('error','sql execution failed'),finished_at=UTC_TIMESTAMP(6) WHERE script_name=$nameSQL AND script_sha256=$shaSQL"
      try { [void](Invoke-MySQL -Arguments ($baseArguments + "--execute=$failedSQL") -Operation 'record reconciliation failure') } catch { }
      throw
    }
  }
} finally {
  [Environment]::SetEnvironmentVariable('MYSQL_PWD', $previousMySQLPassword, 'Process')
  $client.Password = $null
}
