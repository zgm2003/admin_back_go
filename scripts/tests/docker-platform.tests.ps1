$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$scriptPath = Join-Path $repoRoot 'scripts\docker-platform.ps1'
$commonPath = Join-Path $repoRoot 'scripts\dev\admin-dev-common.ps1'
$stabilityScriptPath = Join-Path $repoRoot 'scripts\tests\docker-stability.tests.ps1'

if (-not (Test-Path -LiteralPath $scriptPath -PathType Leaf)) {
  throw 'docker-platform.ps1 is missing'
}
if (-not (Test-Path -LiteralPath $stabilityScriptPath -PathType Leaf)) {
  throw 'docker-stability.tests.ps1 is missing'
}
if (-not (Test-Path -LiteralPath $commonPath -PathType Leaf)) {
  throw 'admin-dev-common.ps1 is missing'
}

$content = [IO.File]::ReadAllText($scriptPath)
foreach ($required in @(
  "ValidateSet('init','dev-state','up','stop','status')",
  ". (Join-Path `$PSScriptRoot 'dev\admin-dev-common.ps1')",
  "'dev-state' {",
  "Invoke-Docker @('compose', '-f', `$appCompose, 'stop', 'frontend', 'admin-api', 'admin-worker')",
  "Invoke-Docker @('compose', '-f', `$stateCompose, 'up', '-d', '--wait', '--wait-timeout', '180')",
  'Assert-NoLiveAdminDevLock',
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

$guardNeedle = 'Assert-NoLiveAdminDevLock -Path $adminDevLock -RepositoryRoot $repoRoot'
if ([regex]::Matches($content, [regex]::Escape($guardNeedle)).Count -ne 2) {
  throw 'up and stop must each reject a live admin-dev lock'
}

$devStateStart = $content.IndexOf("'dev-state' {", [StringComparison]::Ordinal)
$upStart = $content.IndexOf("'up' {", [StringComparison]::Ordinal)
if ($devStateStart -lt 0 -or $upStart -le $devStateStart) {
  throw 'dev-state lifecycle block is missing or misplaced'
}
$devStateBlock = $content.Substring($devStateStart, $upStart - $devStateStart)
if ($devStateBlock.Contains($guardNeedle)) {
  throw 'dev-state must remain callable by the lock-owning supervisor'
}
if ($devStateBlock -match "(?i)'(down|rm)'|--volumes|\bbuild\b") {
  throw 'dev-state must not build, remove services, or delete volumes'
}

if ($content -match '(?i)down\s+-v|--volumes') {
  throw 'lifecycle script must not delete volumes'
}

. $commonPath
$temporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-dev-lock-test-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($temporaryRoot) | Out-Null
$lockPath = Join-Path $temporaryRoot 'admin-dev.lock.json'
$handle = $null
try {
  $handle = Enter-AdminDevLock -Path $lockPath -RepositoryRoot $repoRoot
  if (-not (Test-Path -LiteralPath $lockPath -PathType Leaf)) {
    throw 'exclusive admin-dev lock was not created'
  }
  $lockJSON = Get-Content -Raw -LiteralPath $lockPath | ConvertFrom-Json
  if ([int]$lockJSON.pid -ne $PID -or
      [string]::IsNullOrWhiteSpace([string]$lockJSON.process_started_at_utc) -or
      -not [StringComparer]::OrdinalIgnoreCase.Equals(
        [IO.Path]::GetFullPath([string]$lockJSON.repository_root),
        [IO.Path]::GetFullPath($repoRoot)
      )) {
    throw 'admin-dev lock identity is incomplete'
  }
  if (@($lockJSON.PSObject.Properties.Name | Where-Object { $_ -match '(?i)secret|password|dsn|token' }).Count -ne 0) {
    throw 'admin-dev lock must not contain secret-like fields'
  }
  $lockRecord = Read-AdminDevLock -Path $lockPath
  if (-not (Test-AdminDevLockLive -Record $lockRecord -RepositoryRoot $repoRoot)) {
    $actualStart = Get-AdminDevProcessStartTime -ProcessId $PID
    throw "new lock is not live: expected=$($lockJSON.process_started_at_utc), actual=$actualStart"
  }

  $secondEntryRejected = $false
  $secondEntryError = ''
  try {
    $null = Enter-AdminDevLock -Path $lockPath -RepositoryRoot $repoRoot
  }
  catch {
    $secondEntryError = $_.Exception.Message
    $secondEntryRejected = $secondEntryError.Contains('ADMIN_DEV_ALREADY_RUNNING')
  }
  if (-not $secondEntryRejected) {
    throw "a live admin-dev lock must reject a second owner: $secondEntryError"
  }

  $platformGuardRejected = $false
  try {
    Assert-NoLiveAdminDevLock -Path $lockPath -RepositoryRoot $repoRoot
  }
  catch {
    $platformGuardRejected = $_.Exception.Message.Contains('ADMIN_DEV_ACTIVE')
  }
  if (-not $platformGuardRejected) {
    throw 'full Docker lifecycle must reject a live admin-dev lock'
  }

  $foreignJSON = Get-Content -Raw -LiteralPath $lockPath | ConvertFrom-Json
  $foreignJSON.lock_id = [guid]::NewGuid().ToString('N')
  [IO.File]::WriteAllText($lockPath, ($foreignJSON | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
  Exit-AdminDevLock -Handle $handle
  if (-not (Test-Path -LiteralPath $lockPath -PathType Leaf)) {
    throw 'lock cleanup must not remove a lock owned by another identity'
  }
  Remove-Item -LiteralPath $lockPath -Force

  $staleLock = [ordered]@{
    schema_version = 1
    lock_id = [guid]::NewGuid().ToString('N')
    pid = 2147483647
    process_started_at_utc = '2000-01-01T00:00:00.0000000Z'
    repository_root = [IO.Path]::GetFullPath($repoRoot)
  }
  [IO.File]::WriteAllText($lockPath, ($staleLock | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
  Assert-NoLiveAdminDevLock -Path $lockPath -RepositoryRoot $repoRoot
  if (Test-Path -LiteralPath $lockPath) {
    throw 'a stale lock must be removed before Docker lifecycle continues'
  }

  $pidReuseLock = [ordered]@{
    schema_version = 1
    lock_id = [guid]::NewGuid().ToString('N')
    pid = $PID
    process_started_at_utc = '2000-01-01T00:00:00.0000000Z'
    repository_root = [IO.Path]::GetFullPath($repoRoot)
  }
  [IO.File]::WriteAllText($lockPath, ($pidReuseLock | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
  Assert-NoLiveAdminDevLock -Path $lockPath -RepositoryRoot $repoRoot
  if (Test-Path -LiteralPath $lockPath) {
    throw 'PID reuse with a different process start time must be treated as stale'
  }
}
finally {
  if ($null -ne $handle) {
    Exit-AdminDevLock -Handle $handle
  }
  Remove-Item -LiteralPath $temporaryRoot -Recurse -Force -ErrorAction SilentlyContinue
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
$stopStart = $content.IndexOf("'stop' {", $upStart, [StringComparison]::Ordinal)
if ($stopStart -le $upStart) {
  throw 'stop lifecycle block is missing'
}
$upBlock = $content.Substring($upStart, $stopStart - $upStart)
$buildUp = $upBlock.IndexOf($buildNeedle, [StringComparison]::Ordinal)
$stateUp = $upBlock.IndexOf($stateNeedle, [StringComparison]::Ordinal)
$appUp = $upBlock.IndexOf($appNeedle, [StringComparison]::Ordinal)
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
