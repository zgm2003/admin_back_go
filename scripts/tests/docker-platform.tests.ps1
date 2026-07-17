$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$scriptPath = Join-Path $repoRoot 'scripts\docker-platform.ps1'
$stabilityScriptPath = Join-Path $repoRoot 'scripts\tests\docker-stability.tests.ps1'

if (-not (Test-Path -LiteralPath $scriptPath -PathType Leaf)) {
  throw 'docker-platform.ps1 is missing'
}
if (-not (Test-Path -LiteralPath $stabilityScriptPath -PathType Leaf)) {
  throw 'docker-stability.tests.ps1 is missing'
}

$content = [IO.File]::ReadAllText($scriptPath)
foreach ($required in @(
  "ValidateSet('init','up','stop','status')",
  'SetAccessRuleProtection($true, $false)',
  'mysql:3306',
  'redis:6379',
  "'--wait'",
  'mysql-root-password.txt',
  'function Resolve-GitRevision',
  'ADMIN_BACKEND_BUILD_REVISION',
  'ADMIN_FRONTEND_BUILD_REVISION',
  "Invoke-Docker @('compose', '-f', `$appCompose, 'build', 'admin-api', 'frontend')",
  "Invoke-Docker @('compose', '-f', `$appCompose, 'up', '-d', '--no-build', '--wait', '--wait-timeout', '300')"
)) {
  if (-not $content.Contains($required)) {
    throw "missing lifecycle contract: $required"
  }
}

if ($content -match '(?i)down\s+-v|--volumes') {
  throw 'lifecycle script must not delete volumes'
}

$stabilityContent = [IO.File]::ReadAllText($stabilityScriptPath)
foreach ($required in @(
  "'--ip', `$oldAPIAddress",
  "'--no-deps', '--no-build', 'admin-api'",
  "'--force-recreate', 'admin-api', 'admin-worker'",
  "'stop', '--signal', 'SIGTERM'",
  'RestartCount',
  'org.opencontainers.image.revision',
  'finally'
)) {
  if (-not $stabilityContent.Contains($required)) {
    throw "missing Docker stability contract: $required"
  }
}
if ($stabilityContent -match '(?i)down\s+-v|--volumes|volume\s+rm') {
  throw 'Docker stability regression must not delete volumes'
}

$buildNeedle = "Invoke-Docker @('compose', '-f', `$appCompose, 'build', 'admin-api', 'frontend')"
$stateNeedle = "Invoke-Docker @('compose', '-f', `$stateCompose, 'up'"
$appNeedle = "Invoke-Docker @('compose', '-f', `$appCompose, 'up'"
$buildUp = $content.IndexOf($buildNeedle, [StringComparison]::Ordinal)
$stateUp = $content.IndexOf($stateNeedle, [StringComparison]::Ordinal)
$appUp = $content.IndexOf($appNeedle, [StringComparison]::Ordinal)
if ($buildUp -lt 0 -or $stateUp -le $buildUp -or $appUp -le $stateUp) {
  throw 'lifecycle must build once, wait for state, then start app without building'
}

if ([regex]::Matches($content, [regex]::Escape($buildNeedle)).Count -ne 1) {
  throw 'lifecycle must contain exactly one application image build phase'
}
$forbiddenAppBuild = "Invoke-Docker @('compose', '-f', `$appCompose, 'up', '-d', '--build'"
if ($content.Contains($forbiddenAppBuild)) {
  throw 'application up must not build images'
}

Write-Output 'docker-platform assertions passed'
