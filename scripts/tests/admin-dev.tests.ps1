$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '..\admin_front_ts'))
$commonPath = Join-Path $repoRoot 'scripts\dev\admin-dev-common.ps1'
$apiAirPath = Join-Path $repoRoot '.air.api.toml'
$workerAirPath = Join-Path $repoRoot '.air.worker.toml'
$supervisorPath = Join-Path $repoRoot 'scripts\admin-dev.ps1'
$shortcutInstallerPath = Join-Path $repoRoot 'scripts\install-admin-shortcuts.ps1'

function Assert-ThrowsLike {
  param(
    [Parameter(Mandatory = $true)][scriptblock]$Operation,
    [Parameter(Mandatory = $true)][string]$Expected
  )

  $message = ''
  try {
    & $Operation
  }
  catch {
    $message = $_.Exception.Message
  }
  if (-not $message.Contains($Expected, [StringComparison]::Ordinal)) {
    throw "expected error containing '$Expected', received '$message'"
  }
}

if (-not (Test-Path -LiteralPath $commonPath -PathType Leaf)) {
  throw 'admin-dev-common.ps1 is missing'
}
. $commonPath

$nodePaths = Get-AdminDevNodePaths
if ([string]$nodePaths.NodeExecutable -cne 'E:\FlyEnv-Data\app\nodejs\v24.18.0\node.exe' -or
    [string]$nodePaths.NpmExecutable -cne 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd') {
  throw 'Node 24 must resolve only from the approved FlyEnv path'
}
Assert-AdminDevNodeVersions -NodeVersion 'v24.18.0' -NpmVersion '11.16.0'
Assert-ThrowsLike { Assert-AdminDevNodeVersions -NodeVersion 'v24.17.0' -NpmVersion '11.16.0' } 'ADMIN_DEV_NODE_VERSION_INVALID'
Assert-ThrowsLike { Assert-AdminDevNodeVersions -NodeVersion 'v24.18.0' -NpmVersion '11.15.0' } 'ADMIN_DEV_NPM_VERSION_INVALID'
Assert-AdminDevGoVersion -VersionOutput 'go version go1.26.5 windows/amd64'
Assert-ThrowsLike { Assert-AdminDevGoVersion -VersionOutput 'go version go1.26.4 windows/amd64' } 'ADMIN_DEV_GO_VERSION_INVALID'
Assert-ThrowsLike { Assert-AdminDevGoVersion -VersionOutput 'go version go1.26.5 linux/amd64' } 'ADMIN_DEV_GO_VERSION_INVALID'
$resolvedHostTools = Resolve-AdminDevHostTools
if (-not (Test-Path -LiteralPath $resolvedHostTools.ZoneInfoPath -PathType Leaf) -or
    [IO.Path]::GetFileName([string]$resolvedHostTools.ZoneInfoPath) -cne 'zoneinfo.zip') {
  throw 'Windows Go children require the exact toolchain zoneinfo.zip path'
}

$temporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-dev-test-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($temporaryRoot) | Out-Null
try {
  $envPath = Join-Path $temporaryRoot 'admin-go.env'
  $validEnvironment = @'
# fixture values are intentionally synthetic
MYSQL_DSN=fixture_user:fixture=pass@tcp(mysql:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local
REDIS_ADDR=redis:6379
REDIS_PASSWORD=fixture-redis-password
HTTP_ADDR=:8080
LOG_DIR=/app/runtime/logs
PAYMENT_CERT_BASE_DIR=/app
APP_SECRET=fixture-app-secret-value
OPAQUE_VALUE=left=middle=right
'@
  [IO.File]::WriteAllText($envPath, $validEnvironment, [Text.UTF8Encoding]::new($false))
  $requiredKeys = @(
    'MYSQL_DSN',
    'REDIS_ADDR',
    'REDIS_PASSWORD',
    'HTTP_ADDR',
    'LOG_DIR',
    'PAYMENT_CERT_BASE_DIR',
    'APP_SECRET'
  )
  $environment = Read-AdminDevEnvironmentFile -Path $envPath -RequiredKeys $requiredKeys
  if ([string]$environment.OPAQUE_VALUE -cne 'left=middle=right') {
    throw 'environment values must split only on the first equals sign'
  }

  $hostEnvironment = ConvertTo-AdminDevHostEnvironment -Environment $environment -RepositoryRoot $repoRoot
  $dockerRuntimeRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot 'deploy\docker-first'))
  if ([string]$hostEnvironment.MYSQL_DSN -cne 'fixture_user:fixture=pass@tcp(127.0.0.1:33306)/admin?charset=utf8mb4&parseTime=True&loc=Local' -or
      [string]$hostEnvironment.REDIS_ADDR -cne '127.0.0.1:36379' -or
      [string]$hostEnvironment.HTTP_ADDR -cne '[::]:8080' -or
      [string]$hostEnvironment.LOG_DIR -cne (Join-Path $dockerRuntimeRoot 'runtime\logs') -or
      [string]$hostEnvironment.PAYMENT_CERT_BASE_DIR -cne $dockerRuntimeRoot) {
    throw 'container-only addresses and paths were not converted exactly'
  }
  foreach ($unchanged in @('REDIS_PASSWORD', 'APP_SECRET', 'OPAQUE_VALUE')) {
    if ([string]$hostEnvironment[$unchanged] -cne [string]$environment[$unchanged]) {
      throw "non-container environment value changed: $unchanged"
    }
  }
  if ([string]$environment.MYSQL_DSN -notmatch 'mysql:3306') {
    throw 'host conversion must not mutate the parsed source environment'
  }

  $sensitiveValues = Get-AdminDevSensitiveValues -Environment $environment
  $unsafeText = "dsn=$($environment.MYSQL_DSN); secret=$($environment.APP_SECRET); redis=$($environment.REDIS_PASSWORD)"
  $safeText = Protect-AdminDevText -Text $unsafeText -SensitiveValues $sensitiveValues
  foreach ($secret in @($environment.MYSQL_DSN, $environment.APP_SECRET, $environment.REDIS_PASSWORD)) {
    if ($safeText.Contains([string]$secret, [StringComparison]::Ordinal)) {
      throw 'secret redaction left a configured secret in output'
    }
  }
  if (-not $safeText.Contains('[REDACTED]', [StringComparison]::Ordinal)) {
    throw 'secret redaction did not mark protected content'
  }

  $emptyRedisPasswordPath = Join-Path $temporaryRoot 'empty-redis-password.env'
  [IO.File]::WriteAllText(
    $emptyRedisPasswordPath,
    "APP_SECRET=fixture-app-secret-value`nREDIS_PASSWORD=",
    [Text.UTF8Encoding]::new($false)
  )
  $emptyRedisEnvironment = Read-AdminDevEnvironmentFile `
    -Path $emptyRedisPasswordPath `
    -RequiredKeys @('APP_SECRET', 'REDIS_PASSWORD') `
    -AllowEmptyKeys @('REDIS_PASSWORD')
  if (-not $emptyRedisEnvironment.ContainsKey('REDIS_PASSWORD') -or
      [string]$emptyRedisEnvironment['REDIS_PASSWORD'] -cne '') {
    throw 'an explicitly configured empty Redis password must remain valid and present'
  }

  foreach ($invalidFixture in @(
    @{ Name = 'duplicate'; Text = "APP_SECRET=one`nAPP_SECRET=two"; Error = 'ADMIN_DEV_ENV_DUPLICATE' },
    @{ Name = 'malformed'; Text = "NOT VALID=value"; Error = 'ADMIN_DEV_ENV_MALFORMED' },
    @{ Name = 'empty'; Text = "APP_SECRET="; Error = 'ADMIN_DEV_ENV_REQUIRED' },
    @{ Name = 'missing'; Text = "OTHER=value"; Error = 'ADMIN_DEV_ENV_REQUIRED' }
  )) {
    $invalidPath = Join-Path $temporaryRoot ($invalidFixture.Name + '.env')
    [IO.File]::WriteAllText($invalidPath, $invalidFixture.Text, [Text.UTF8Encoding]::new($false))
    Assert-ThrowsLike {
      Read-AdminDevEnvironmentFile -Path $invalidPath -RequiredKeys @('APP_SECRET') | Out-Null
    } $invalidFixture.Error
  }

  $lockfile = Join-Path $temporaryRoot 'package-lock.json'
  $stampPath = Join-Path $temporaryRoot 'frontend-dependencies.json'
  [IO.File]::WriteAllText($lockfile, '{"lockfileVersion":3}', [Text.UTF8Encoding]::new($false))
  if (Test-AdminDevDependencyStamp -LockfilePath $lockfile -StampPath $stampPath) {
    throw 'missing frontend dependency stamp must be a cache miss'
  }
  Write-AdminDevDependencyStamp -LockfilePath $lockfile -StampPath $stampPath
  if (-not (Test-AdminDevDependencyStamp -LockfilePath $lockfile -StampPath $stampPath)) {
    throw 'matching package-lock hash must be a cache hit'
  }
  [IO.File]::AppendAllText($lockfile, "`n", [Text.UTF8Encoding]::new($false))
  if (Test-AdminDevDependencyStamp -LockfilePath $lockfile -StampPath $stampPath) {
    throw 'changed package-lock hash must invalidate the dependency cache'
  }
  $stampText = [IO.File]::ReadAllText($stampPath, [Text.Encoding]::UTF8)
  if ($stampText.Contains('lockfileVersion', [StringComparison]::Ordinal)) {
    throw 'dependency stamp must contain a hash, not package-lock contents'
  }

  $airPaths = Get-AdminDevAirPaths -RepositoryRoot $temporaryRoot
  $expectedAirRoot = Join-Path $temporaryRoot '.tmp\tools\air\v1.66.0'
  if ([string]$airPaths.Root -cne $expectedAirRoot -or
      [string]$airPaths.Executable -cne (Join-Path $expectedAirRoot 'air.exe')) {
    throw 'Air must use the repository-private pinned tool path'
  }
  [IO.Directory]::CreateDirectory($airPaths.Root) | Out-Null
  [IO.File]::WriteAllBytes($airPaths.Executable, [byte[]]@(0))
  [IO.File]::WriteAllText($airPaths.VersionMarker, 'v1.66.0', [Text.UTF8Encoding]::new($false))
  if (-not (Test-AdminDevAirReady -RepositoryRoot $temporaryRoot)) {
    throw 'matching Air executable and marker must be reusable'
  }
  [IO.File]::WriteAllText($airPaths.VersionMarker, 'v1.65.0', [Text.UTF8Encoding]::new($false))
  if (Test-AdminDevAirReady -RepositoryRoot $temporaryRoot) {
    throw 'wrong Air marker must require reinstall'
  }
}
finally {
  Remove-Item -LiteralPath $temporaryRoot -Recurse -Force -ErrorAction SilentlyContinue
}

foreach ($airPath in @($apiAirPath, $workerAirPath)) {
  if (-not (Test-Path -LiteralPath $airPath -PathType Leaf)) {
    throw "Air configuration is missing: $airPath"
  }
}
$apiAir = [IO.File]::ReadAllText($apiAirPath)
$workerAir = [IO.File]::ReadAllText($workerAirPath)
foreach ($required in @(
  'send_interrupt = true',
  'kill_delay = "2s"',
  'include_ext = ["go"]',
  'exclude_dir = [".git", ".tmp", "deploy", "docs", "runtime"]'
)) {
  if (-not $apiAir.Contains($required) -or -not $workerAir.Contains($required)) {
    throw "Air watch contract is missing: $required"
  }
}
if (-not $apiAir.Contains('.tmp/dev/api/admin-api.exe') -or
    -not $apiAir.Contains('entrypoint = [".tmp/dev/api/admin-api.exe"]') -or
    -not $apiAir.Contains('./cmd/admin-api') -or
    -not $workerAir.Contains('.tmp/dev/worker/admin-worker.exe') -or
    -not $workerAir.Contains('entrypoint = [".tmp/dev/worker/admin-worker.exe"]') -or
    -not $workerAir.Contains('./cmd/admin-worker')) {
  throw 'API and worker Air outputs must be distinct and target the correct commands'
}
if ($apiAir -match '(?m)^\s*bin\s*=' -or $workerAir -match '(?m)^\s*bin\s*=') {
  throw 'Air configurations must use the non-deprecated entrypoint contract'
}
if ($apiAir.Contains('.tmp/dev/worker') -or $workerAir.Contains('.tmp/dev/api')) {
  throw 'Air configurations must not share build output directories'
}

if (-not (Test-Path -LiteralPath (Join-Path $frontendRoot 'package-lock.json') -PathType Leaf)) {
  throw 'frontend package-lock.json is missing'
}

if (-not (Test-Path -LiteralPath $supervisorPath -PathType Leaf)) {
  throw 'admin-dev.ps1 is missing'
}
$supervisorSource = [IO.File]::ReadAllText($supervisorPath)
$commonSource = [IO.File]::ReadAllText($commonPath)
foreach ($required in @(
  'Assert-AdminDevPrimaryRepositories',
  'Enter-AdminDevLock',
  'docker-platform.ps1',
  '-Action dev-state',
  'Wait-AdminDevRuntimeReady',
  "`$runtimeEnvironment['ZONEINFO']",
  'Stop-AdminDevManagedProcesses',
  'finally',
  "'--host', '::'",
  'http://127.0.0.1:5173',
  'http://127.0.0.1:8080/health',
  'http://127.0.0.1:8080/ready'
)) {
  if (-not $supervisorSource.Contains($required)) {
    throw "supervisor contract is missing: $required"
  }
}
foreach ($required in @(
  'RedirectStandardOutput = $true',
  'RedirectStandardError = $true',
  '$startInfo.Environment[',
  'Kill($true)',
  'ADMIN_DEV_PORT_IN_USE',
  'ADMIN_DEV_PROCESS_EXITED',
  'ADMIN_DEV_READINESS_TIMEOUT'
)) {
  if (-not $commonSource.Contains($required)) {
    throw "managed-process contract is missing: $required"
  }
}
foreach ($prefix in @('[WEB]', '[API]', '[WORKER]')) {
  if (-not $supervisorSource.Contains($prefix)) {
    throw "supervisor log prefix is missing: $prefix"
  }
}
foreach ($browserContract in @(
  '[switch]$NoBrowser',
  'if (-not $NoBrowser)',
  'http://localhost:5173',
  'UseShellExecute = $true',
  '[Diagnostics.Process]::Start($browserStartInfo)'
)) {
  if (-not $supervisorSource.Contains($browserContract)) {
    throw "browser launch contract is missing: $browserContract"
  }
}
$readyIndex = $supervisorSource.IndexOf('Wait-AdminDevRuntimeReady', [StringComparison]::Ordinal)
$browserIndex = $supervisorSource.IndexOf('[Diagnostics.Process]::Start($browserStartInfo)', [StringComparison]::Ordinal)
$watchIndex = $supervisorSource.IndexOf('Watch-AdminDevManagedProcesses', [StringComparison]::Ordinal)
if ($readyIndex -lt 0 -or $browserIndex -le $readyIndex -or $watchIndex -le $browserIndex) {
  throw 'browser must open once after readiness and before the process watch loop'
}
if ([regex]::Matches($supervisorSource, [regex]::Escape('[Diagnostics.Process]::Start($browserStartInfo)')).Count -ne 1) {
  throw 'browser launch must occur exactly once per admin-dev invocation'
}
if ($supervisorSource -match "(?i)-Action\s+(stop|down)" -or
    $supervisorSource -match "(?i)docker\s+compose.+\b(stop|down)\b") {
  throw 'admin-dev cleanup must preserve Docker state services'
}

$formatted = Format-AdminDevLogLine -Prefix '[API]' -Line 'secret=fixture-app-secret-value' -SensitiveValues @('fixture-app-secret-value')
if ($formatted -cne '[API] secret=[REDACTED]') {
  throw 'managed log lines must be prefixed and secret-redacted'
}

$processFixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-dev-process-test-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($processFixtureRoot) | Out-Null
$childScript = Join-Path $processFixtureRoot 'child.ps1'
$longScript = Join-Path $processFixtureRoot 'long.ps1'
$failScript = Join-Path $processFixtureRoot 'fail.ps1'
$childPidPath = Join-Path $processFixtureRoot 'child.pid'
[IO.File]::WriteAllText($childScript, @'
Start-Sleep -Seconds 60
'@, [Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText($longScript, @'
param([string]$ChildScript, [string]$ChildPidPath)
if ($env:ADMIN_DEV_FIXTURE -cne 'available') { exit 91 }
Write-Output 'fixture stdout'
[Console]::Error.WriteLine('fixture stderr')
if (-not [string]::IsNullOrWhiteSpace($ChildScript)) {
  $info = [Diagnostics.ProcessStartInfo]::new()
  $info.FileName = (Get-Command pwsh -ErrorAction Stop).Source
  $info.UseShellExecute = $false
  $info.CreateNoWindow = $true
  $info.ArgumentList.Add('-NoProfile')
  $info.ArgumentList.Add('-File')
  $info.ArgumentList.Add($ChildScript)
  $child = [Diagnostics.Process]::Start($info)
  [IO.File]::WriteAllText($ChildPidPath, [string]$child.Id, [Text.UTF8Encoding]::new($false))
}
Start-Sleep -Seconds 60
'@, [Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText($failScript, @'
Write-Output 'failing fixture'
exit 23
'@, [Text.UTF8Encoding]::new($false))

$pwsh = (Get-Command pwsh -ErrorAction Stop | Select-Object -First 1).Source
Assert-ThrowsLike {
  Start-AdminDevManagedProcess `
    -Name 'fixture-secret-argument' `
    -Prefix '[API]' `
    -FilePath $pwsh `
    -ArgumentList @('fixture-secret-command-value') `
    -WorkingDirectory $processFixtureRoot `
    -Environment @{} `
    -SensitiveValues @('fixture-secret-command-value') | Out-Null
} 'ADMIN_DEV_SECRET_ARGUMENT_REJECTED'
$managed = [Collections.Generic.List[object]]::new()
try {
  $managed.Add((Start-AdminDevManagedProcess `
    -Name 'fixture-web' `
    -Prefix '[WEB]' `
    -FilePath $pwsh `
    -ArgumentList @('-NoProfile', '-File', $longScript, '-ChildScript', $childScript, '-ChildPidPath', $childPidPath) `
    -WorkingDirectory $processFixtureRoot `
    -Environment @{ ADMIN_DEV_FIXTURE = 'available' } `
    -SensitiveValues @('fixture-secret-not-in-arguments')))
  $managed.Add((Start-AdminDevManagedProcess `
    -Name 'fixture-api' `
    -Prefix '[API]' `
    -FilePath $pwsh `
    -ArgumentList @('-NoProfile', '-File', $longScript) `
    -WorkingDirectory $processFixtureRoot `
    -Environment @{ ADMIN_DEV_FIXTURE = 'available' } `
    -SensitiveValues @()))
  Wait-AdminDevManagedProcesses `
    -States $managed.ToArray() `
    -TimeoutSeconds 5 `
    -ReadyCondition { param($states) return @($states | Where-Object { -not $_.Process.HasExited }).Count -eq 2 }
  $deadline = [DateTime]::UtcNow.AddSeconds(5)
  while (-not (Test-Path -LiteralPath $childPidPath -PathType Leaf) -and [DateTime]::UtcNow -lt $deadline) {
    Start-Sleep -Milliseconds 50
  }
  if (-not (Test-Path -LiteralPath $childPidPath -PathType Leaf)) {
    throw 'fixture descendant did not start'
  }
  $descendantPid = [int][IO.File]::ReadAllText($childPidPath, [Text.Encoding]::UTF8)
}
finally {
  Stop-AdminDevManagedProcesses -States $managed.ToArray()
}
foreach ($state in $managed) {
  if (-not $state.Process.HasExited) {
    throw "managed root process survived cleanup: $($state.Name)"
  }
}
if ($null -ne (Get-Process -Id $descendantPid -ErrorAction SilentlyContinue)) {
  throw 'managed descendant process survived cleanup'
}

$failureGroup = [Collections.Generic.List[object]]::new()
$failureDetected = $false
try {
  $failureGroup.Add((Start-AdminDevManagedProcess `
    -Name 'fixture-sibling' `
    -Prefix '[WEB]' `
    -FilePath $pwsh `
    -ArgumentList @('-NoProfile', '-File', $longScript) `
    -WorkingDirectory $processFixtureRoot `
    -Environment @{ ADMIN_DEV_FIXTURE = 'available' } `
    -SensitiveValues @()))
  $failureGroup.Add((Start-AdminDevManagedProcess `
    -Name 'fixture-failure' `
    -Prefix '[API]' `
    -FilePath $pwsh `
    -ArgumentList @('-NoProfile', '-File', $failScript) `
    -WorkingDirectory $processFixtureRoot `
    -Environment @{} `
    -SensitiveValues @()))
  try {
    Wait-AdminDevManagedProcesses -States $failureGroup.ToArray() -TimeoutSeconds 5 -ReadyCondition { return $false }
  }
  catch {
    $failureDetected = $_.Exception.Message.Contains('ADMIN_DEV_PROCESS_EXITED')
  }
}
finally {
  Stop-AdminDevManagedProcesses -States $failureGroup.ToArray()
}
if (-not $failureDetected) {
  throw 'one child failure must fail the supervisor and clean siblings'
}
foreach ($state in $failureGroup) {
  if (-not $state.Process.HasExited) {
    throw "sibling survived child failure cleanup: $($state.Name)"
  }
}

$timeoutGroup = [Collections.Generic.List[object]]::new()
$timeoutDetected = $false
try {
  $timeoutGroup.Add((Start-AdminDevManagedProcess `
    -Name 'fixture-timeout' `
    -Prefix '[WORKER]' `
    -FilePath $pwsh `
    -ArgumentList @('-NoProfile', '-File', $longScript) `
    -WorkingDirectory $processFixtureRoot `
    -Environment @{ ADMIN_DEV_FIXTURE = 'available' } `
    -SensitiveValues @()))
  try {
    Wait-AdminDevManagedProcesses -States $timeoutGroup.ToArray() -TimeoutSeconds 1 -ReadyCondition { return $false }
  }
  catch {
    $timeoutDetected = $_.Exception.Message.Contains('ADMIN_DEV_READINESS_TIMEOUT')
  }
}
finally {
  Stop-AdminDevManagedProcesses -States $timeoutGroup.ToArray()
  Remove-Item -LiteralPath $processFixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}
if (-not $timeoutDetected) {
  throw 'readiness timeout must fail closed and clean children'
}

if (-not (Test-Path -LiteralPath $shortcutInstallerPath -PathType Leaf)) {
  throw 'install-admin-shortcuts.ps1 is missing'
}
$profileFixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-profile-test-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($profileFixtureRoot) | Out-Null
$profilePaths = @(
  (Join-Path $profileFixtureRoot 'PowerShell\Microsoft.PowerShell_profile.ps1'),
  (Join-Path $profileFixtureRoot 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1')
)
try {
  foreach ($profilePath in $profilePaths) {
    [IO.Directory]::CreateDirectory((Split-Path $profilePath -Parent)) | Out-Null
    [IO.File]::WriteAllText($profilePath, "# unrelated profile content`n`$global:fixtureValue = 7`n", [Text.UTF8Encoding]::new($false))
  }
  & $shortcutInstallerPath -ProfilePaths $profilePaths
  & $shortcutInstallerPath -ProfilePaths $profilePaths

  foreach ($profilePath in $profilePaths) {
    $profileContent = [IO.File]::ReadAllText($profilePath, [Text.Encoding]::UTF8)
    if ([regex]::Matches($profileContent, '(?m)^# >>> admin platform shortcuts >>>$').Count -ne 1 -or
        [regex]::Matches($profileContent, '(?m)^# <<< admin platform shortcuts <<<$').Count -ne 1) {
      throw 'shortcut installation must leave exactly one managed block'
    }
    if (-not $profileContent.Contains('# unrelated profile content', [StringComparison]::Ordinal) -or
        -not $profileContent.Contains('$global:fixtureValue = 7', [StringComparison]::Ordinal)) {
      throw 'shortcut installation must preserve unrelated Profile content'
    }
    foreach ($shortcut in @('admin-dev', 'admin-up', 'admin-stop', 'admin-status')) {
      if (-not $profileContent.Contains("function global:$shortcut", [StringComparison]::Ordinal)) {
        throw "Profile shortcut is missing: $shortcut"
      }
    }
    if ([regex]::Matches($profileContent, "(?m)& 'pwsh' -NoProfile").Count -ne 4 -or
        -not $profileContent.Contains("-File 'E:\admin\admin_back_go\scripts\admin-dev.ps1'", [StringComparison]::Ordinal) -or
        -not $profileContent.Contains("-File 'E:\admin\admin_back_go\scripts\docker-platform.ps1' -Action up", [StringComparison]::Ordinal) -or
        -not $profileContent.Contains("-File 'E:\admin\admin_back_go\scripts\docker-platform.ps1' -Action stop", [StringComparison]::Ordinal) -or
        -not $profileContent.Contains("-File 'E:\admin\admin_back_go\scripts\docker-platform.ps1' -Action status", [StringComparison]::Ordinal)) {
      throw 'Profile shortcuts must use pwsh -NoProfile and repository-owned scripts'
    }
  }

  $escapedProfile = $profilePaths[0].Replace("'", "''")
  $discoveryOutput = @(& $pwsh -NoProfile -Command ". '$escapedProfile'; 'admin-dev','admin-up','admin-stop','admin-status' | ForEach-Object { (Get-Command `$_ -ErrorAction Stop).Name }")
  if ($LASTEXITCODE -ne 0 -or @($discoveryOutput).Count -ne 4) {
    throw 'fresh PowerShell 7 shell did not discover all Admin shortcuts'
  }
}
finally {
  Remove-Item -LiteralPath $profileFixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Output 'admin-dev preparation assertions passed'
