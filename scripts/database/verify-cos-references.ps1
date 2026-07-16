[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dsn = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
if ([string]::IsNullOrWhiteSpace($dsn)) {
  throw 'MYSQL_DSN is required'
}
$appSecret = [Environment]::GetEnvironmentVariable('APP_SECRET', 'Process')
if ([string]::IsNullOrWhiteSpace($appSecret)) {
  throw 'APP_SECRET is required'
}

$fullOutputPath = [System.IO.Path]::GetFullPath($OutputPath)
$rootPrefix = $backendRoot.TrimEnd('\') + '\'
if ($fullOutputPath.StartsWith($rootPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
  throw 'COS evidence output must be outside the repository'
}
$outputDirectory = Split-Path -Parent $fullOutputPath
if ([string]::IsNullOrWhiteSpace($outputDirectory) -or -not (Test-Path -LiteralPath $outputDirectory -PathType Container)) {
  throw 'COS evidence output directory does not exist'
}

Push-Location $backendRoot
try {
  & go run ./cmd/admin-db cos-references --schema $Database --out $fullOutputPath
  if ($LASTEXITCODE -ne 0) {
    throw 'COS reference verification failed'
  }
} finally {
  Pop-Location
  $dsn = $null
  $appSecret = $null
}
