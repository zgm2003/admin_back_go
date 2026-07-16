[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [string]$Manifest,

  [Parameter(Mandatory = $true)]
  [string]$OutputRoot,

  [string]$MySQLCommand = 'mysql'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$warm_run_count = 5

function Get-DSNClientSettings {
  $dsn = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  if ([string]::IsNullOrWhiteSpace($dsn)) { throw 'MYSQL_DSN is required' }
  $marker = '@tcp('
  $markerIndex = $dsn.LastIndexOf($marker, [System.StringComparison]::Ordinal)
  $credentialSeparator = if ($markerIndex -gt 0) { $dsn.Substring(0, $markerIndex).IndexOf(':') } else { -1 }
  $addressStart = $markerIndex + $marker.Length
  $addressEnd = if ($markerIndex -gt 0) { $dsn.IndexOf(')/', $addressStart, [System.StringComparison]::Ordinal) } else { -1 }
  if ($markerIndex -le 0 -or $credentialSeparator -le 0 -or $addressEnd -le $addressStart) { throw 'MYSQL_DSN is not a supported TCP DSN' }
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

function ConvertTo-SQLLiteral {
  param([Parameter(Mandatory = $true)]$Value)
  if ($Value -is [string]) { return "'" + $Value.Replace("'", "''") + "'" }
  if ($Value -is [bool]) { return $(if ($Value) { '1' } else { '0' }) }
  if ($Value -is [byte] -or $Value -is [int16] -or $Value -is [int32] -or $Value -is [int64] -or $Value -is [uint16] -or $Value -is [uint32] -or $Value -is [uint64] -or $Value -is [single] -or $Value -is [double] -or $Value -is [decimal]) {
    return [System.Convert]::ToString($Value, [System.Globalization.CultureInfo]::InvariantCulture)
  }
  throw 'query manifest binding type is unsupported'
}

function Expand-QueryBindings {
  param([Parameter(Mandatory = $true)][string]$SQL, [Parameter(Mandatory = $true)]$Bindings)
  $expanded = $SQL
  $properties = @($Bindings.PSObject.Properties | Sort-Object { $_.Name.Length } -Descending)
  foreach ($property in $properties) {
    $pattern = '(?<![A-Za-z0-9_]):' + [regex]::Escape($property.Name) + '\b'
    $literal = ConvertTo-SQLLiteral -Value $property.Value
    $expanded = [regex]::Replace($expanded, $pattern, [System.Text.RegularExpressions.MatchEvaluator]{ param($match) $literal })
  }
  if ($expanded -match '(?<![A-Za-z0-9_]):[a-z][a-z0-9_]*\b') { throw 'query manifest contains an unbound placeholder' }
  return $expanded
}

function Invoke-MySQL {
  param([Parameter(Mandatory = $true)][string]$SQL, [Parameter(Mandatory = $true)][string]$Operation)
  $output = @(& $script:MySQLExecutable @script:BaseArguments "--execute=$SQL" 2>$null)
  if ($LASTEXITCODE -ne 0) { throw "$Operation failed" }
  return @($output | ForEach-Object { $_.ToString() })
}

function Get-IndexExists {
  param([string]$Table, [string]$Index)
  $sql = "SELECT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='$Table' AND index_name='$Index')"
  $rows = @(Invoke-MySQL -SQL $sql -Operation 'inspect candidate index')
  return $rows.Count -eq 1 -and $rows[0] -eq '1'
}

function Get-TableFootprint {
  param([string]$Table)
  $rows = @(Invoke-MySQL -SQL "SELECT COALESCE(data_length,0)+COALESCE(index_length,0) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='$Table'" -Operation 'inspect table footprint')
  if ($rows.Count -ne 1 -or $rows[0] -notmatch '^[0-9]+$') { throw 'table footprint output was invalid' }
  return [uint64]$rows[0]
}

function Get-DigestTotals {
  $sql = "SELECT COALESCE(SUM(count_star),0),COALESCE(SUM(sum_timer_wait),0),COALESCE(SUM(sum_rows_examined),0) FROM performance_schema.events_statements_summary_by_digest WHERE schema_name=DATABASE()"
  $rows = @(Invoke-MySQL -SQL $sql -Operation 'capture performance_schema digest totals')
  if ($rows.Count -ne 1) { throw 'performance_schema output was invalid' }
  $parts = $rows[0] -split "`t"
  if ($parts.Count -ne 3 -or @($parts | Where-Object { $_ -notmatch '^[0-9]+$' }).Count -ne 0) { throw 'performance_schema totals were invalid' }
  return [pscustomobject]@{ Count = [decimal]$parts[0]; Timer = [decimal]$parts[1]; RowsExamined = [decimal]$parts[2] }
}

function Get-PlanRows {
  param([string[]]$Plan)
  $maximum = [double]0
  $pattern = [regex]'actual time=[^)]*? rows=([0-9.]+) loops=([0-9]+)'
  foreach ($line in $Plan) {
    foreach ($match in $pattern.Matches($line)) {
      $examined = [double]::Parse($match.Groups[1].Value, [System.Globalization.CultureInfo]::InvariantCulture) * [double]$match.Groups[2].Value
      if ($examined -gt $maximum) { $maximum = $examined }
    }
  }
  return [uint64][Math]::Ceiling($maximum)
}

function Get-Percentile {
  param([double[]]$Values, [double]$Percentile)
  $sorted = @($Values | Sort-Object)
  if ($sorted.Count -eq 0) { return [double]0 }
  $index = [Math]::Ceiling($Percentile * $sorted.Count) - 1
  if ($index -lt 0) { $index = 0 }
  if ($index -ge $sorted.Count) { $index = $sorted.Count - 1 }
  return [double]$sorted[$index]
}

$BackendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$ManifestPath = [System.IO.Path]::GetFullPath((Join-Path $BackendRoot $Manifest))
$OutputPath = [System.IO.Path]::GetFullPath($OutputRoot)
$rootPrefix = $BackendRoot.TrimEnd('\') + '\'
if ($OutputPath.StartsWith($rootPrefix, [System.StringComparison]::OrdinalIgnoreCase)) { throw 'query evidence output must be outside the repository' }
New-Item -ItemType Directory -Force -Path $OutputPath | Out-Null

Push-Location $BackendRoot
try {
  & go run ./cmd/admin-db query-manifest files --manifest $ManifestPath 2>$null | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'query manifest validation failed' }
  $Candidates = @(Get-Content -Raw -LiteralPath $ManifestPath | ConvertFrom-Json)
} finally {
  Pop-Location
}

$MySQLExecutable = (Get-Command -Name $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
$client = Get-DSNClientSettings
$BaseArguments = @('--protocol=tcp', "--host=$($client.Host)", "--port=$($client.Port)", "--user=$($client.User)", '--batch', '--skip-column-names', '--raw', "--database=$Database")
$previousPassword = [Environment]::GetEnvironmentVariable('MYSQL_PWD', 'Process')
$env:MYSQL_PWD = $client.Password
$accepted = [System.Collections.Generic.List[object]]::new()
$summaries = [System.Collections.Generic.List[object]]::new()

try {
  foreach ($candidate in $Candidates) {
    $match = [regex]::Match([string]$candidate.proposed_index, '(?i)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+([A-Za-z0-9_]+)\s+ON\s+([A-Za-z0-9_]+)\s*\(')
    if (-not $match.Success) { throw 'validated index DDL could not be parsed' }
    $indexName = $match.Groups[1].Value
    $tableName = $match.Groups[2].Value
    $query = Expand-QueryBindings -SQL ([string]$candidate.sql) -Bindings $candidate.bindings
    $indexExisted = Get-IndexExists -Table $tableName -Index $indexName
    $indexHidden = $false
    $indexCreated = $false
    $footprintBefore = Get-TableFootprint -Table $tableName
    try {
      $distribution = @(Invoke-MySQL -SQL ([string]$candidate.row_distribution_sql) -Operation 'capture row distribution')
      if ($indexExisted) {
        [void](Invoke-MySQL -SQL "ALTER TABLE ``$tableName`` ALTER INDEX ``$indexName`` INVISIBLE" -Operation 'hide candidate index')
        $indexHidden = $true
      }
      $beforePlan = @(Invoke-MySQL -SQL "EXPLAIN ANALYZE FORMAT=TREE $query" -Operation 'capture before plan')
      $beforeRows = Get-PlanRows -Plan $beforePlan
      if ($indexExisted) {
        [void](Invoke-MySQL -SQL "ALTER TABLE ``$tableName`` ALTER INDEX ``$indexName`` VISIBLE" -Operation 'restore candidate index')
        $indexHidden = $false
      } else {
        [void](Invoke-MySQL -SQL ([string]$candidate.proposed_index) -Operation 'create candidate index')
        $indexCreated = $true
      }
      $afterPlan = @(Invoke-MySQL -SQL "EXPLAIN ANALYZE FORMAT=TREE $query" -Operation 'capture after plan')
      $afterRows = Get-PlanRows -Plan $afterPlan
      $digestBefore = Get-DigestTotals
      $durations = [System.Collections.Generic.List[double]]::new()
      for ($run = 0; $run -lt $warm_run_count; $run++) {
        $timer = [System.Diagnostics.Stopwatch]::StartNew()
        [void](Invoke-MySQL -SQL $query -Operation 'execute warm candidate query')
        $timer.Stop()
        $durations.Add($timer.Elapsed.TotalMilliseconds)
      }
      $digestAfter = Get-DigestTotals
      $p50 = Get-Percentile -Values $durations.ToArray() -Percentile 0.50
      $p95 = Get-Percentile -Values $durations.ToArray() -Percentile 0.95
      $footprintAfter = Get-TableFootprint -Table $tableName
      $acceptedCandidate = $afterRows -lt $beforeRows -and $afterRows -le [uint64]$candidate.max_rows_examined -and $p95 -le [double]$candidate.max_p95_ms
      $evidence = [ordered]@{
        name = [string]$candidate.name
        repository_file = [string]$candidate.repository_file
        row_distribution = $distribution
        index_name = $indexName
        index_preexisting = $indexExisted
        proposed_index = [string]$candidate.proposed_index
        before_plan = $beforePlan
        before_rows_examined = $beforeRows
        after_plan = $afterPlan
        after_rows_examined = $afterRows
        warm_runs_ms = $durations.ToArray()
        p50_ms = [Math]::Round($p50, 3)
        p95_ms = [Math]::Round($p95, 3)
        max_rows_examined = [uint64]$candidate.max_rows_examined
        max_p95_ms = [uint64]$candidate.max_p95_ms
        table_footprint_before = $footprintBefore
        table_footprint_after = $footprintAfter
        estimated_index_bytes = $(if ($footprintAfter -gt $footprintBefore) { $footprintAfter - $footprintBefore } else { 0 })
        performance_schema = [ordered]@{
          count_delta = $digestAfter.Count - $digestBefore.Count
          timer_wait_delta = $digestAfter.Timer - $digestBefore.Timer
          rows_examined_delta = $digestAfter.RowsExamined - $digestBefore.RowsExamined
        }
        accepted = $acceptedCandidate
      }
      $evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $OutputPath ($candidate.name + '.json')) -Encoding utf8NoBOM
      $summaries.Add([pscustomobject]@{ name = [string]$candidate.name; accepted = $acceptedCandidate; preexisting = $indexExisted; before_rows = $beforeRows; after_rows = $afterRows; p95_ms = [Math]::Round($p95, 3) })
      if ($acceptedCandidate) {
        $accepted.Add([pscustomobject]@{ name = [string]$candidate.name; index_name = $indexName; table_name = $tableName; preexisting = $indexExisted; ddl = [string]$candidate.proposed_index })
      }
    } finally {
      if ($indexHidden) { try { [void](Invoke-MySQL -SQL "ALTER TABLE ``$tableName`` ALTER INDEX ``$indexName`` VISIBLE" -Operation 'restore candidate index') } catch { } }
      if ($indexCreated) { try { [void](Invoke-MySQL -SQL "DROP INDEX ``$indexName`` ON ``$tableName``" -Operation 'drop temporary candidate index') } catch { } }
    }
  }
  $accepted | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $OutputPath 'accepted_indexes.json') -Encoding utf8NoBOM
  $summaries | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $OutputPath 'summary.json') -Encoding utf8NoBOM
  [ordered]@{ candidates = $summaries.Count; accepted = @($summaries | Where-Object accepted).Count; evidence_root = $OutputPath } | ConvertTo-Json -Compress
} finally {
  [Environment]::SetEnvironmentVariable('MYSQL_PWD', $previousPassword, 'Process')
  $client.Password = $null
}
