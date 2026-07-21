[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [string]$MySQLCommand = 'mysql',

  [string]$COSManifest = '',

  [switch]$PostContract
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Get-DSNClientSettings {
  $dsn = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  if ([string]::IsNullOrWhiteSpace($dsn)) { throw 'MYSQL_DSN is required' }
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
  if ($portSeparator -le 0) { throw 'MYSQL_DSN address is malformed' }
  $databaseStart = $addressEnd + 2
  $queryIndex = $dsn.IndexOf('?', $databaseStart)
  $dsnDatabase = if ($queryIndex -ge 0) { $dsn.Substring($databaseStart, $queryIndex - $databaseStart) } else { $dsn.Substring($databaseStart) }
  if ($dsnDatabase -cne $Database) { throw 'MYSQL_DSN database does not match requested schema' }
  return [pscustomobject]@{
    User = $credentials.Substring(0, $credentialSeparator)
    Password = $credentials.Substring($credentialSeparator + 1)
    Host = $address.Substring(0, $portSeparator).Trim('[', ']')
    Port = $address.Substring($portSeparator + 1)
  }
}

function Get-SchemaSHA256 {
  param([Parameter(Mandatory = $true)][string]$Commit)
  $path = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-expanded-fingerprint-' + [guid]::NewGuid().ToString('N') + '.json')
  try {
    & go run ./cmd/admin-db fingerprint --schema $Database --out $path --commit $Commit 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'schema fingerprint capture failed' }
    $document = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json
    if ([string]$document.schema_sha256 -notmatch '^[0-9a-f]{64}$') { throw 'schema fingerprint was invalid' }
    return [string]$document.schema_sha256
  } finally {
    Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
  }
}

function Invoke-InvariantFile {
  param([Parameter(Mandatory = $true)][string]$Path)
  $lines = @(& go run ./cmd/admin-db invariants --schema $Database --file $Path 2>$null)
  if ($LASTEXITCODE -ne 0) { throw "database invariant file failed: $([System.IO.Path]::GetFileName($Path))" }
  $total = [uint64]0
  foreach ($line in $lines) {
    $parts = [string]$line -split "`t", 2
    if ($parts.Count -ne 2 -or $parts[1] -notmatch '^[0-9]+$') { throw 'database invariant output was invalid' }
    $total += [uint64]$parts[1]
  }
  return $total
}

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$mysql = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$client = Get-DSNClientSettings
$previousPassword = [Environment]::GetEnvironmentVariable('MYSQL_PWD', 'Process')
$env:MYSQL_PWD = $client.Password

Push-Location $backendRoot
try {
  $commit = (& git rev-parse HEAD).Trim()
  if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') { throw 'Git commit could not be resolved' }
  $beforeSHA = Get-SchemaSHA256 -Commit $commit

  if ($PostContract) {
    $schemaViolations = Invoke-InvariantFile -Path 'database/reconciliation/053_verify_admin_only.sql'
    $relationshipViolations = Invoke-InvariantFile -Path 'database/reconciliation/031_verify_relations.sql'
    $moneyViolations = Invoke-InvariantFile -Path 'database/reconciliation/032_verify_money.sql'
    $aiViolations = Invoke-InvariantFile -Path 'database/reconciliation/052_verify_ai_contract.sql'
    $platformViolations = Invoke-InvariantFile -Path 'database/reconciliation/051_verify_admin_rows.sql'
    $aiImageDeleteViolations = Invoke-InvariantFile -Path 'database/reconciliation/035_verify_ai_image_soft_delete.sql'
    $exportCleanupViolations = Invoke-InvariantFile -Path 'database/reconciliation/036_verify_export_cleanup.sql'
    $cronTaskMetadataViolations = Invoke-InvariantFile -Path 'database/reconciliation/037_verify_cron_task_metadata.sql'
    $browserOnlyRetirementViolations = [uint64]0
  } else {
    $schemaViolations = Invoke-InvariantFile -Path 'database/reconciliation/030_verify_schema.sql'
    $relationshipViolations = Invoke-InvariantFile -Path 'database/reconciliation/031_verify_relations.sql'
    $moneyViolations = Invoke-InvariantFile -Path 'database/reconciliation/032_verify_money.sql'
    $aiViolations = Invoke-InvariantFile -Path 'database/reconciliation/033_verify_ai.sql'
    $platformViolations = Invoke-InvariantFile -Path 'database/reconciliation/034_verify_platform.sql'
    $aiImageDeleteViolations = Invoke-InvariantFile -Path 'database/reconciliation/035_verify_ai_image_soft_delete.sql'
    $exportCleanupViolations = Invoke-InvariantFile -Path 'database/reconciliation/036_verify_export_cleanup.sql'
    $cronTaskMetadataViolations = Invoke-InvariantFile -Path 'database/reconciliation/037_verify_cron_task_metadata.sql'
    $browserOnlyRetirementViolations = Invoke-InvariantFile -Path 'database/reconciliation/038_verify_browser_only_retirement.sql'
  }

  & go test ./internal/module/auth ./internal/module/user ./internal/module/notification/... ./internal/module/export ./internal/module/payment/... ./internal/module/ai/run 2>$null | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'focused Admin smoke tests failed' }

  $baseArguments = @('--protocol=tcp', "--host=$($client.Host)", "--port=$($client.Port)", "--user=$($client.User)", '--batch', '--skip-column-names', '--raw', "--database=$Database")
  $legacySQL = @"
SELECT
  (SELECT COUNT(*) FROM role_permissions rp LEFT JOIN permissions p ON p.id=rp.permission_id WHERE rp.is_del=2 AND p.id IS NULL),
  (SELECT COUNT(*) FROM notifications WHERE is_del=2 AND platform='all'),
  (SELECT COUNT(*) FROM notification_task WHERE is_del=2 AND platform='all'),
  (SELECT COUNT(*) FROM ai_runs WHERE platform='canvas'),
  (SELECT COUNT(*) FROM ai_image_tasks WHERE platform='canvas'),
  (SELECT COUNT(*) FROM ai_assets WHERE is_del=2 AND user_id=0),
  (SELECT COUNT(*) FROM export_tasks WHERE COALESCE(file_url,'')<>'' AND COALESCE(object_key,'')='')
"@
  $legacyOutput = @(& $mysql @baseArguments "--execute=$legacySQL" 2>$null)
  if ($LASTEXITCODE -ne 0 -or $legacyOutput.Count -ne 1) { throw 'legacy evidence summary failed' }
  $legacy = [string]$legacyOutput[0] -split "`t"
  if ($legacy.Count -ne 7 -or @($legacy | Where-Object { $_ -notmatch '^[0-9]+$' }).Count -ne 0) { throw 'legacy evidence output was invalid' }

  $cosSummary = [ordered]@{ reachable = 0; not_found = 0; dependency = 0 }
  if (-not [string]::IsNullOrWhiteSpace($COSManifest)) {
    $manifestPath = [System.IO.Path]::GetFullPath($COSManifest)
    $rootPrefix = $backendRoot.TrimEnd('\') + '\'
    if ($manifestPath.StartsWith($rootPrefix, [System.StringComparison]::OrdinalIgnoreCase)) { throw 'COS manifest must be outside the repository' }
    $manifest = @(Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json)
    foreach ($entry in $manifest) {
      $status = [string]$entry.status
      if (-not $cosSummary.Contains($status)) { throw 'COS manifest status was invalid' }
      $cosSummary[$status]++
    }
  }

  $afterSHA = Get-SchemaSHA256 -Commit $commit
  if ($afterSHA -cne $beforeSHA) { throw 'expanded schema verification changed the schema fingerprint' }

  [ordered]@{
    schema_sha256 = $afterSHA
    schema_violations = $schemaViolations
    relationship_violations = $relationshipViolations
    money_violations = $moneyViolations
    ai_violations = $aiViolations
    platform_violations = $platformViolations
    ai_image_delete_violations = $aiImageDeleteViolations
    export_cleanup_violations = $exportCleanupViolations
    cron_task_metadata_violations = $cronTaskMetadataViolations
    browser_only_retirement_violations = $browserOnlyRetirementViolations
    admin_smoke = 'passed'
    legacy_evidence = [ordered]@{
      legacy_missing_permission_grants = [uint64]$legacy[0]
      notification_all_active = [uint64]$legacy[1]
      notification_task_all_active = [uint64]$legacy[2]
      canvas_ai_runs = [uint64]$legacy[3]
      canvas_ai_image_tasks = [uint64]$legacy[4]
      global_ai_assets = [uint64]$legacy[5]
      unresolved_export_object_keys = [uint64]$legacy[6]
      cos_references = $cosSummary
    }
  } | ConvertTo-Json -Depth 5 -Compress
} finally {
  Pop-Location
  [Environment]::SetEnvironmentVariable('MYSQL_PWD', $previousPassword, 'Process')
  $client.Password = $null
}
