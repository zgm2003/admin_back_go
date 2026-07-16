$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$scriptPath = Join-Path $repoRoot 'scripts\docker-platform.ps1'

if (-not (Test-Path -LiteralPath $scriptPath -PathType Leaf)) {
  throw 'docker-platform.ps1 is missing'
}

$content = [IO.File]::ReadAllText($scriptPath)
foreach ($required in @(
  "ValidateSet('init','up','stop','status')",
  'SetAccessRuleProtection($true, $false)',
  'mysql:3306',
  'redis:6379',
  "'--wait'",
  'mysql-root-password.txt'
)) {
  if (-not $content.Contains($required)) {
    throw "missing lifecycle contract: $required"
  }
}

if ($content -match '(?i)down\s+-v|--volumes') {
  throw 'lifecycle script must not delete volumes'
}

$stateNeedle = "Invoke-Docker @('compose', '-f', `$stateCompose, 'up'"
$appNeedle = "Invoke-Docker @('compose', '-f', `$appCompose, 'up'"
$stateUp = $content.IndexOf($stateNeedle, [StringComparison]::Ordinal)
$appUp = $content.IndexOf($appNeedle, [StringComparison]::Ordinal)
if ($stateUp -lt 0 -or $appUp -le $stateUp) {
  throw 'state must start before app'
}

Write-Output 'docker-platform assertions passed'
