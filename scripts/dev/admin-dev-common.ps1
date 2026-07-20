function Get-AdminDevCanonicalPath {
  param([Parameter(Mandatory = $true)][string]$Path)

  return [IO.Path]::GetFullPath($Path).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
}

function Get-AdminDevProcessStartTime {
  param([Parameter(Mandatory = $true)][int]$ProcessId)

  try {
    $process = Get-Process -Id $ProcessId -ErrorAction Stop
    return $process.StartTime.ToUniversalTime().ToString('O', [Globalization.CultureInfo]::InvariantCulture)
  }
  catch {
    return $null
  }
}

function Read-AdminDevLock {
  param([Parameter(Mandatory = $true)][string]$Path)

  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    return $null
  }

  try {
    $record = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8) | ConvertFrom-Json
  }
  catch {
    throw 'ADMIN_DEV_LOCK_UNREADABLE'
  }

  $propertyNames = @($record.PSObject.Properties.Name)
  foreach ($required in @('schema_version', 'lock_id', 'pid', 'process_started_at_utc', 'repository_root')) {
    if ($propertyNames -notcontains $required) {
      return $null
    }
  }
  if ([int]$record.schema_version -ne 1 -or
      [string]$record.lock_id -notmatch '^[0-9a-f]{32}$' -or
      [int]$record.pid -le 0 -or
      [string]::IsNullOrWhiteSpace([string]$record.process_started_at_utc) -or
      [string]::IsNullOrWhiteSpace([string]$record.repository_root)) {
    return $null
  }
  try {
    $rawStartTime = $record.process_started_at_utc
    if ($rawStartTime -is [DateTime]) {
      $startTime = [DateTimeOffset]::new($rawStartTime.ToUniversalTime())
    }
    elseif ($rawStartTime -is [DateTimeOffset]) {
      $startTime = $rawStartTime.ToUniversalTime()
    }
    else {
      $startTime = [DateTimeOffset]::ParseExact(
        [string]$rawStartTime,
        'O',
        [Globalization.CultureInfo]::InvariantCulture,
        [Globalization.DateTimeStyles]::RoundtripKind
      )
    }
    $record.process_started_at_utc = $startTime.ToUniversalTime().ToString(
      'O',
      [Globalization.CultureInfo]::InvariantCulture
    )
    $null = Get-AdminDevCanonicalPath -Path ([string]$record.repository_root)
  }
  catch {
    return $null
  }
  return $record
}

function Test-AdminDevLockLive {
  param(
    [AllowNull()][psobject]$Record,
    [Parameter(Mandatory = $true)][string]$RepositoryRoot
  )

  if ($null -eq $Record) {
    return $false
  }
  $expectedRoot = Get-AdminDevCanonicalPath -Path $RepositoryRoot
  $recordRoot = Get-AdminDevCanonicalPath -Path ([string]$Record.repository_root)
  if (-not [StringComparer]::OrdinalIgnoreCase.Equals($recordRoot, $expectedRoot)) {
    return $false
  }
  $actualStart = Get-AdminDevProcessStartTime -ProcessId ([int]$Record.pid)
  if ([string]::IsNullOrWhiteSpace($actualStart)) {
    return $false
  }
  $expectedTime = [DateTimeOffset]::ParseExact(
    [string]$Record.process_started_at_utc,
    'O',
    [Globalization.CultureInfo]::InvariantCulture,
    [Globalization.DateTimeStyles]::RoundtripKind
  )
  $actualTime = [DateTimeOffset]::ParseExact(
    $actualStart,
    'O',
    [Globalization.CultureInfo]::InvariantCulture,
    [Globalization.DateTimeStyles]::RoundtripKind
  )
  return $expectedTime.UtcTicks -eq $actualTime.UtcTicks
}

function Remove-StaleAdminDevLock {
  param([Parameter(Mandatory = $true)][string]$Path)

  if (Test-Path -LiteralPath $Path) {
    Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
  }
}

function Assert-NoLiveAdminDevLock {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$RepositoryRoot
  )

  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    return
  }
  $record = Read-AdminDevLock -Path $Path
  if (Test-AdminDevLockLive -Record $record -RepositoryRoot $RepositoryRoot) {
    throw 'ADMIN_DEV_ACTIVE: exit admin-dev before changing the full Docker platform'
  }
  Remove-StaleAdminDevLock -Path $Path
}

function Enter-AdminDevLock {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$RepositoryRoot
  )

  $canonicalPath = [IO.Path]::GetFullPath($Path)
  $canonicalRoot = Get-AdminDevCanonicalPath -Path $RepositoryRoot
  [IO.Directory]::CreateDirectory((Split-Path $canonicalPath -Parent)) | Out-Null

  for ($attempt = 0; $attempt -lt 3; $attempt++) {
    if (Test-Path -LiteralPath $canonicalPath -PathType Leaf) {
      $existing = Read-AdminDevLock -Path $canonicalPath
      if (Test-AdminDevLockLive -Record $existing -RepositoryRoot $canonicalRoot) {
        throw 'ADMIN_DEV_ALREADY_RUNNING'
      }
      Remove-StaleAdminDevLock -Path $canonicalPath
    }

    $record = [ordered]@{
      schema_version = 1
      lock_id = [guid]::NewGuid().ToString('N')
      pid = $PID
      process_started_at_utc = Get-AdminDevProcessStartTime -ProcessId $PID
      repository_root = $canonicalRoot
    }
    if ([string]::IsNullOrWhiteSpace([string]$record.process_started_at_utc)) {
      throw 'ADMIN_DEV_PROCESS_IDENTITY_UNAVAILABLE'
    }
    $json = $record | ConvertTo-Json -Compress
    $stream = $null
    try {
      $stream = [IO.FileStream]::new(
        $canonicalPath,
        [IO.FileMode]::CreateNew,
        [IO.FileAccess]::Write,
        [IO.FileShare]::None
      )
      $bytes = [Text.UTF8Encoding]::new($false).GetBytes($json)
      $stream.Write($bytes, 0, $bytes.Length)
      $stream.Flush($true)
      return [pscustomobject]@{
        Path = $canonicalPath
        LockId = [string]$record.lock_id
        ProcessId = [int]$record.pid
        ProcessStartedAtUtc = [string]$record.process_started_at_utc
        RepositoryRoot = $canonicalRoot
      }
    }
    catch [IO.IOException] {
      if ($attempt -eq 2) {
        throw 'ADMIN_DEV_LOCK_CREATE_RACE'
      }
    }
    finally {
      if ($null -ne $stream) {
        $stream.Dispose()
      }
    }
  }
  throw 'ADMIN_DEV_LOCK_CREATE_RACE'
}

function Exit-AdminDevLock {
  param([AllowNull()][psobject]$Handle)

  if ($null -eq $Handle -or -not (Test-Path -LiteralPath ([string]$Handle.Path) -PathType Leaf)) {
    return
  }
  try {
    $record = Read-AdminDevLock -Path ([string]$Handle.Path)
  }
  catch {
    return
  }
  try {
    $recordStartTicks = [DateTimeOffset]::Parse(
      [string]$record.process_started_at_utc,
      [Globalization.CultureInfo]::InvariantCulture,
      [Globalization.DateTimeStyles]::RoundtripKind
    ).UtcTicks
    $handleStartTicks = [DateTimeOffset]::Parse(
      [string]$Handle.ProcessStartedAtUtc,
      [Globalization.CultureInfo]::InvariantCulture,
      [Globalization.DateTimeStyles]::RoundtripKind
    ).UtcTicks
  }
  catch {
    return
  }
  if ($null -eq $record -or
      [string]$record.lock_id -cne [string]$Handle.LockId -or
      [int]$record.pid -ne [int]$Handle.ProcessId -or
      $recordStartTicks -ne $handleStartTicks -or
      -not [StringComparer]::OrdinalIgnoreCase.Equals(
        (Get-AdminDevCanonicalPath -Path ([string]$record.repository_root)),
        (Get-AdminDevCanonicalPath -Path ([string]$Handle.RepositoryRoot))
      )) {
    return
  }
  Remove-Item -LiteralPath ([string]$Handle.Path) -Force -ErrorAction SilentlyContinue
}

function Get-AdminDevNodePaths {
  $nodeRoot = 'E:\FlyEnv-Data\app\nodejs\v24.18.0'
  return [pscustomobject]@{
    NodeExecutable = Join-Path $nodeRoot 'node.exe'
    NpmExecutable = Join-Path $nodeRoot 'npm.cmd'
  }
}

function Assert-AdminDevNodeVersions {
  param(
    [Parameter(Mandatory = $true)][string]$NodeVersion,
    [Parameter(Mandatory = $true)][string]$NpmVersion
  )

  if ($NodeVersion.Trim() -cne 'v24.18.0') {
    throw 'ADMIN_DEV_NODE_VERSION_INVALID: expected v24.18.0'
  }
  if ($NpmVersion.Trim() -cne '11.16.0') {
    throw 'ADMIN_DEV_NPM_VERSION_INVALID: expected 11.16.0'
  }
}

function Assert-AdminDevGoVersion {
  param([Parameter(Mandatory = $true)][string]$VersionOutput)

  if ($VersionOutput.Trim() -cne 'go version go1.26.5 windows/amd64') {
    throw 'ADMIN_DEV_GO_VERSION_INVALID: expected go1.26.5 windows/amd64'
  }
}

function Invoke-AdminDevVersionCommand {
  param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label
  )

  $output = @(& $Executable @Arguments 2>&1)
  $exitCode = $LASTEXITCODE
  if ($exitCode -ne 0) {
    throw "ADMIN_DEV_TOOL_VERSION_FAILED: $Label"
  }
  return (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
}

function Resolve-AdminDevHostTools {
  $nodePaths = Get-AdminDevNodePaths
  foreach ($path in @($nodePaths.NodeExecutable, $nodePaths.NpmExecutable)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
      throw "ADMIN_DEV_TOOL_MISSING: $path"
    }
  }
  $goCommand = Get-Command 'go.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $goCommand) {
    $goCommand = Get-Command 'go' -ErrorAction Stop | Select-Object -First 1
  }

  $nodeVersion = Invoke-AdminDevVersionCommand -Executable $nodePaths.NodeExecutable -Arguments @('--version') -Label 'node'
  $npmVersion = Invoke-AdminDevVersionCommand -Executable $nodePaths.NpmExecutable -Arguments @('--version') -Label 'npm'
  $goVersion = Invoke-AdminDevVersionCommand -Executable $goCommand.Source -Arguments @('version') -Label 'go'
  $goRoot = Invoke-AdminDevVersionCommand -Executable $goCommand.Source -Arguments @('env', 'GOROOT') -Label 'go root'
  Assert-AdminDevNodeVersions -NodeVersion $nodeVersion -NpmVersion $npmVersion
  Assert-AdminDevGoVersion -VersionOutput $goVersion
  $zoneInfoPath = Join-Path $goRoot 'lib\time\zoneinfo.zip'
  if (-not (Test-Path -LiteralPath $zoneInfoPath -PathType Leaf)) {
    throw 'ADMIN_DEV_GO_ZONEINFO_MISSING'
  }

  return [pscustomobject]@{
    NodeExecutable = [string]$nodePaths.NodeExecutable
    NpmExecutable = [string]$nodePaths.NpmExecutable
    GoExecutable = [string]$goCommand.Source
    ZoneInfoPath = [IO.Path]::GetFullPath($zoneInfoPath)
  }
}

function Read-AdminDevEnvironmentFile {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string[]]$RequiredKeys,
    [string[]]$AllowEmptyKeys = @()
  )

  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw 'ADMIN_DEV_ENV_MISSING'
  }
  $text = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
  if ($text.Contains([char]0)) {
    throw 'ADMIN_DEV_ENV_MALFORMED'
  }
  $environment = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
  foreach ($line in [regex]::Split($text, "\r\n|\n|\r")) {
    if ([string]::IsNullOrWhiteSpace($line) -or $line -match '^\s*#') {
      continue
    }
    $match = [regex]::Match($line, '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$')
    if (-not $match.Success) {
      throw 'ADMIN_DEV_ENV_MALFORMED'
    }
    $name = $match.Groups[1].Value
    if ($environment.ContainsKey($name)) {
      throw "ADMIN_DEV_ENV_DUPLICATE: $name"
    }
    $environment.Add($name, $match.Groups[2].Value)
  }
  $allowedEmpty = [Collections.Generic.HashSet[string]]::new(
    $AllowEmptyKeys,
    [StringComparer]::OrdinalIgnoreCase
  )
  foreach ($required in $RequiredKeys) {
    if (-not $environment.ContainsKey($required) -or
        (-not $allowedEmpty.Contains($required) -and
          [string]::IsNullOrWhiteSpace($environment[$required]))) {
      throw "ADMIN_DEV_ENV_REQUIRED: $required"
    }
  }
  return $environment
}

function ConvertTo-AdminDevHostEnvironment {
  param(
    [Parameter(Mandatory = $true)][Collections.Generic.IDictionary[string,string]]$Environment,
    [Parameter(Mandatory = $true)][string]$RepositoryRoot
  )

  $result = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
  foreach ($entry in $Environment.GetEnumerator()) {
    $result.Add([string]$entry.Key, [string]$entry.Value)
  }
  foreach ($required in @('MYSQL_DSN', 'REDIS_ADDR', 'HTTP_ADDR', 'LOG_DIR', 'PAYMENT_CERT_BASE_DIR')) {
    if (-not $result.ContainsKey($required)) {
      throw "ADMIN_DEV_ENV_REQUIRED: $required"
    }
  }

  $dsnNeedle = '@tcp(mysql:3306)'
  $dsn = $result['MYSQL_DSN']
  $dsnIndex = $dsn.IndexOf($dsnNeedle, [StringComparison]::Ordinal)
  if ($dsnIndex -lt 0 -or
      $dsn.IndexOf($dsnNeedle, $dsnIndex + $dsnNeedle.Length, [StringComparison]::Ordinal) -ge 0) {
    throw 'ADMIN_DEV_MYSQL_DSN_CONTAINER_ADDRESS_INVALID'
  }
  $result['MYSQL_DSN'] = $dsn.Substring(0, $dsnIndex) + '@tcp(127.0.0.1:33306)' + $dsn.Substring($dsnIndex + $dsnNeedle.Length)

  if ($result['REDIS_ADDR'] -cne 'redis:6379') {
    throw 'ADMIN_DEV_REDIS_CONTAINER_ADDRESS_INVALID'
  }
  if ($result['HTTP_ADDR'] -cne ':8080') {
    throw 'ADMIN_DEV_HTTP_CONTAINER_ADDRESS_INVALID'
  }
  if ($result['LOG_DIR'] -cne '/app/runtime/logs') {
    throw 'ADMIN_DEV_LOG_DIR_CONTAINER_PATH_INVALID'
  }
  if ($result['PAYMENT_CERT_BASE_DIR'] -cne '/app') {
    throw 'ADMIN_DEV_CERT_DIR_CONTAINER_PATH_INVALID'
  }

  $runtimeRoot = [IO.Path]::GetFullPath((Join-Path $RepositoryRoot 'deploy\docker-first'))
  $result['REDIS_ADDR'] = '127.0.0.1:36379'
  $result['HTTP_ADDR'] = '127.0.0.1:8080'
  $result['LOG_DIR'] = Join-Path $runtimeRoot 'runtime\logs'
  $result['PAYMENT_CERT_BASE_DIR'] = $runtimeRoot
  return $result
}

function Get-AdminDevSensitiveValues {
  param([Parameter(Mandatory = $true)][Collections.Generic.IDictionary[string,string]]$Environment)

  $values = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach ($entry in $Environment.GetEnumerator()) {
    if ([string]$entry.Key -match '(?i)(PASSWORD|SECRET|DSN|PRIVATE_KEY|API_KEY|ACCESS_TOKEN|REFRESH_TOKEN)$' -and
        -not [string]::IsNullOrEmpty([string]$entry.Value)) {
      $null = $values.Add([string]$entry.Value)
    }
  }
  return @($values | Sort-Object Length -Descending)
}

function Protect-AdminDevText {
  param(
    [AllowEmptyString()][string]$Text,
    [AllowNull()][string[]]$SensitiveValues
  )

  $safe = [string]$Text
  foreach ($value in @($SensitiveValues)) {
    if (-not [string]::IsNullOrEmpty($value)) {
      $safe = $safe.Replace($value, '[REDACTED]', [StringComparison]::Ordinal)
    }
  }
  return $safe
}

function Get-AdminDevFileSHA256 {
  param([Parameter(Mandatory = $true)][string]$Path)

  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "ADMIN_DEV_FILE_MISSING: $Path"
  }
  return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Test-AdminDevDependencyStamp {
  param(
    [Parameter(Mandatory = $true)][string]$LockfilePath,
    [Parameter(Mandatory = $true)][string]$StampPath
  )

  if (-not (Test-Path -LiteralPath $LockfilePath -PathType Leaf) -or
      -not (Test-Path -LiteralPath $StampPath -PathType Leaf)) {
    return $false
  }
  try {
    $stamp = [IO.File]::ReadAllText($StampPath, [Text.Encoding]::UTF8) | ConvertFrom-Json
  }
  catch {
    return $false
  }
  return [int]$stamp.schema_version -eq 1 -and
    [string]$stamp.package_lock_sha256 -ceq (Get-AdminDevFileSHA256 -Path $LockfilePath) -and
    [string]$stamp.node_version -ceq 'v24.18.0' -and
    [string]$stamp.npm_version -ceq '11.16.0'
}

function Write-AdminDevDependencyStamp {
  param(
    [Parameter(Mandatory = $true)][string]$LockfilePath,
    [Parameter(Mandatory = $true)][string]$StampPath
  )

  $stamp = [ordered]@{
    schema_version = 1
    package_lock_sha256 = Get-AdminDevFileSHA256 -Path $LockfilePath
    node_version = 'v24.18.0'
    npm_version = '11.16.0'
  }
  $directory = Split-Path ([IO.Path]::GetFullPath($StampPath)) -Parent
  [IO.Directory]::CreateDirectory($directory) | Out-Null
  $temporaryPath = Join-Path $directory ([IO.Path]::GetRandomFileName())
  try {
    [IO.File]::WriteAllText(
      $temporaryPath,
      (($stamp | ConvertTo-Json -Compress) + "`n"),
      [Text.UTF8Encoding]::new($false)
    )
    [IO.File]::Move($temporaryPath, [IO.Path]::GetFullPath($StampPath), $true)
    $temporaryPath = $null
  }
  finally {
    if (-not [string]::IsNullOrEmpty($temporaryPath) -and (Test-Path -LiteralPath $temporaryPath)) {
      Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue
    }
  }
}

function Initialize-AdminDevFrontendDependencies {
  param(
    [Parameter(Mandatory = $true)][string]$FrontendRoot,
    [Parameter(Mandatory = $true)][string]$NpmExecutable,
    [Parameter(Mandatory = $true)][string]$StampPath
  )

  $lockfile = Join-Path $FrontendRoot 'package-lock.json'
  $nodeModules = Join-Path $FrontendRoot 'node_modules'
  if ((Test-Path -LiteralPath $nodeModules -PathType Container) -and
      (Test-AdminDevDependencyStamp -LockfilePath $lockfile -StampPath $StampPath)) {
    return $false
  }
  Push-Location $FrontendRoot
  try {
    & $NpmExecutable 'ci' '--no-audit' '--no-fund'
    if ($LASTEXITCODE -ne 0) {
      throw 'ADMIN_DEV_NPM_CI_FAILED'
    }
  }
  finally {
    Pop-Location
  }
  Write-AdminDevDependencyStamp -LockfilePath $lockfile -StampPath $StampPath
  return $true
}

function Get-AdminDevAirPaths {
  param([Parameter(Mandatory = $true)][string]$RepositoryRoot)

  $root = Join-Path (Get-AdminDevCanonicalPath -Path $RepositoryRoot) '.tmp\tools\air\v1.66.0'
  return [pscustomobject]@{
    Root = $root
    Executable = Join-Path $root 'air.exe'
    VersionMarker = Join-Path $root 'version.txt'
  }
}

function Test-AdminDevAirReady {
  param([Parameter(Mandatory = $true)][string]$RepositoryRoot)

  $paths = Get-AdminDevAirPaths -RepositoryRoot $RepositoryRoot
  if (-not (Test-Path -LiteralPath $paths.Executable -PathType Leaf) -or
      -not (Test-Path -LiteralPath $paths.VersionMarker -PathType Leaf)) {
    return $false
  }
  try {
    return [IO.File]::ReadAllText($paths.VersionMarker, [Text.Encoding]::UTF8).Trim() -ceq 'v1.66.0'
  }
  catch {
    return $false
  }
}

function Install-AdminDevAir {
  param(
    [Parameter(Mandatory = $true)][string]$RepositoryRoot,
    [Parameter(Mandatory = $true)][string]$GoExecutable
  )

  $paths = Get-AdminDevAirPaths -RepositoryRoot $RepositoryRoot
  if (Test-AdminDevAirReady -RepositoryRoot $RepositoryRoot) {
    return [string]$paths.Executable
  }
  [IO.Directory]::CreateDirectory($paths.Root) | Out-Null
  Remove-Item -LiteralPath $paths.Executable -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $paths.VersionMarker -Force -ErrorAction SilentlyContinue

  $previousGoBin = [Environment]::GetEnvironmentVariable('GOBIN', 'Process')
  $previousGoToolchain = [Environment]::GetEnvironmentVariable('GOTOOLCHAIN', 'Process')
  try {
    $env:GOBIN = $paths.Root
    $env:GOTOOLCHAIN = 'local'
    & $GoExecutable 'install' 'github.com/air-verse/air@v1.66.0'
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $paths.Executable -PathType Leaf)) {
      throw 'ADMIN_DEV_AIR_INSTALL_FAILED'
    }
  }
  finally {
    [Environment]::SetEnvironmentVariable('GOBIN', $previousGoBin, 'Process')
    [Environment]::SetEnvironmentVariable('GOTOOLCHAIN', $previousGoToolchain, 'Process')
  }

  $versionOutput = Invoke-AdminDevVersionCommand -Executable $paths.Executable -Arguments @('-v') -Label 'air'
  if ($versionOutput -notmatch '(?m)\bv?1\.66\.0\b') {
    throw 'ADMIN_DEV_AIR_VERSION_INVALID'
  }
  [IO.File]::WriteAllText($paths.VersionMarker, "v1.66.0`n", [Text.UTF8Encoding]::new($false))
  return [string]$paths.Executable
}

function Invoke-AdminDevGitCapture {
  param(
    [Parameter(Mandatory = $true)][string]$RepositoryRoot,
    [Parameter(Mandatory = $true)][string[]]$Arguments
  )

  $gitCommand = Get-Command 'git.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $gitCommand) {
    $gitCommand = Get-Command 'git' -ErrorAction Stop | Select-Object -First 1
  }
  $output = @(& $gitCommand.Source -C $RepositoryRoot @Arguments 2>&1)
  if ($LASTEXITCODE -ne 0) {
    throw 'ADMIN_DEV_GIT_VALIDATION_FAILED'
  }
  return (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
}

function Assert-AdminDevPrimaryRepositories {
  param(
    [Parameter(Mandatory = $true)][string]$BackendRoot,
    [Parameter(Mandatory = $true)][string]$FrontendRoot
  )

  foreach ($repository in @($BackendRoot, $FrontendRoot)) {
    $canonicalRoot = Get-AdminDevCanonicalPath -Path $repository
    if (-not (Test-Path -LiteralPath $canonicalRoot -PathType Container)) {
      throw "ADMIN_DEV_REPOSITORY_MISSING: $canonicalRoot"
    }
    $gitRoot = Invoke-AdminDevGitCapture -RepositoryRoot $canonicalRoot -Arguments @('rev-parse', '--show-toplevel')
    if (-not [StringComparer]::OrdinalIgnoreCase.Equals(
      (Get-AdminDevCanonicalPath -Path $gitRoot),
      $canonicalRoot
    )) {
      throw "ADMIN_DEV_PRIMARY_CHECKOUT_REQUIRED: $canonicalRoot"
    }
    $branch = Invoke-AdminDevGitCapture -RepositoryRoot $canonicalRoot -Arguments @('branch', '--show-current')
    if ($branch -cne 'master') {
      throw "ADMIN_DEV_MASTER_REQUIRED: $canonicalRoot"
    }
    $worktreeOutput = Invoke-AdminDevGitCapture -RepositoryRoot $canonicalRoot -Arguments @('worktree', 'list', '--porcelain')
    $worktreePaths = @(
      [regex]::Matches($worktreeOutput, '(?m)^worktree (.+)$') |
        ForEach-Object { Get-AdminDevCanonicalPath -Path $_.Groups[1].Value.Trim() }
    )
    if ($worktreePaths.Count -ne 1 -or
        -not [StringComparer]::OrdinalIgnoreCase.Equals($worktreePaths[0], $canonicalRoot) -or
        (Test-Path -LiteralPath (Join-Path $canonicalRoot '.worktrees'))) {
      throw "ADMIN_DEV_SINGLE_CHECKOUT_REQUIRED: $canonicalRoot"
    }
  }
}

function Assert-AdminDevPortsAvailable {
  param([Parameter(Mandatory = $true)][int[]]$Ports)

  $listeningPorts = @(
    [Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().GetActiveTcpListeners() |
      ForEach-Object { $_.Port }
  )
  foreach ($port in $Ports) {
    if ($listeningPorts -contains $port) {
      throw "ADMIN_DEV_PORT_IN_USE: $port"
    }
  }
}

function Format-AdminDevLogLine {
  param(
    [Parameter(Mandatory = $true)][string]$Prefix,
    [AllowEmptyString()][string]$Line,
    [AllowNull()][string[]]$SensitiveValues
  )

  return "$Prefix $(Protect-AdminDevText -Text $Line -SensitiveValues $SensitiveValues)"
}

function Start-AdminDevManagedProcess {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$Prefix,
    [Parameter(Mandatory = $true)][string]$FilePath,
    [Parameter(Mandatory = $true)][string[]]$ArgumentList,
    [Parameter(Mandatory = $true)][string]$WorkingDirectory,
    [Parameter(Mandatory = $true)][Collections.IDictionary]$Environment,
    [AllowNull()][string[]]$SensitiveValues
  )

  foreach ($argument in $ArgumentList) {
    foreach ($secret in @($SensitiveValues)) {
      if (-not [string]::IsNullOrEmpty($secret) -and
          [string]$argument -ne '' -and
          ([string]$argument).Contains($secret, [StringComparison]::Ordinal)) {
        throw 'ADMIN_DEV_SECRET_ARGUMENT_REJECTED'
      }
    }
  }

  $startInfo = [Diagnostics.ProcessStartInfo]::new()
  $startInfo.FileName = $FilePath
  $startInfo.WorkingDirectory = $WorkingDirectory
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  $startInfo.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
  $startInfo.StandardErrorEncoding = [Text.UTF8Encoding]::new($false)
  foreach ($argument in $ArgumentList) {
    $startInfo.ArgumentList.Add([string]$argument)
  }
  foreach ($entry in $Environment.GetEnumerator()) {
    $startInfo.Environment[[string]$entry.Key] = [string]$entry.Value
  }

  $process = [Diagnostics.Process]::new()
  $process.StartInfo = $startInfo
  if (-not $process.Start()) {
    throw "ADMIN_DEV_PROCESS_START_FAILED: $Name"
  }
  return [pscustomobject]@{
    Name = $Name
    Prefix = $Prefix
    Process = $process
    OutputTask = $process.StandardOutput.ReadLineAsync()
    ErrorTask = $process.StandardError.ReadLineAsync()
    SensitiveValues = @($SensitiveValues)
  }
}

function Receive-AdminDevManagedProcessLines {
  param([Parameter(Mandatory = $true)][object[]]$States)

  foreach ($state in $States) {
    foreach ($stream in @(
      @{ Property = 'OutputTask'; Reader = $state.Process.StandardOutput },
      @{ Property = 'ErrorTask'; Reader = $state.Process.StandardError }
    )) {
      $task = $state.($stream.Property)
      while ($null -ne $task -and $task.IsCompleted) {
        $line = $task.GetAwaiter().GetResult()
        if ($null -eq $line) {
          $state.($stream.Property) = $null
          $task = $null
          continue
        }
        Write-Host (Format-AdminDevLogLine -Prefix $state.Prefix -Line $line -SensitiveValues $state.SensitiveValues)
        $task = $stream.Reader.ReadLineAsync()
        $state.($stream.Property) = $task
      }
    }
  }
}

function Assert-AdminDevManagedProcessesRunning {
  param([Parameter(Mandatory = $true)][object[]]$States)

  foreach ($state in $States) {
    if ($state.Process.HasExited) {
      Receive-AdminDevManagedProcessLines -States @($state)
      throw "ADMIN_DEV_PROCESS_EXITED: $($state.Name) ($($state.Process.ExitCode))"
    }
  }
}

function Wait-AdminDevManagedProcesses {
  param(
    [Parameter(Mandatory = $true)][object[]]$States,
    [Parameter(Mandatory = $true)][ValidateRange(1, 600)][int]$TimeoutSeconds,
    [Parameter(Mandatory = $true)][scriptblock]$ReadyCondition
  )

  $stopwatch = [Diagnostics.Stopwatch]::StartNew()
  while ($stopwatch.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
    Receive-AdminDevManagedProcessLines -States $States
    Assert-AdminDevManagedProcessesRunning -States $States
    $ready = & $ReadyCondition $States
    if ($ready -eq $true) {
      return
    }
    Start-Sleep -Milliseconds 50
  }
  throw "ADMIN_DEV_READINESS_TIMEOUT: $TimeoutSeconds seconds"
}

function Stop-AdminDevManagedProcesses {
  param([AllowNull()][object[]]$States)

  $items = @($States)
  [array]::Reverse($items)
  foreach ($state in $items) {
    if ($null -eq $state -or $null -eq $state.Process) {
      continue
    }
    try {
      if (-not $state.Process.HasExited) {
        $state.Process.Kill($true)
      }
    }
    catch {
      Write-Warning "could not stop managed process $($state.Name)"
    }
  }
  foreach ($state in $items) {
    if ($null -eq $state -or $null -eq $state.Process) {
      continue
    }
    try {
      if (-not $state.Process.HasExited) {
        $null = $state.Process.WaitForExit(10000)
      }
    }
    catch {
      Write-Warning "could not wait for managed process $($state.Name)"
    }
  }
}

function Test-AdminDevHttpReady {
  param(
    [Parameter(Mandatory = $true)][string]$Uri,
    [ValidateRange(1, 10)][int]$TimeoutSeconds = 1
  )

  $client = [Net.Http.HttpClient]::new()
  $client.Timeout = [TimeSpan]::FromSeconds($TimeoutSeconds)
  try {
    $response = $client.GetAsync($Uri).GetAwaiter().GetResult()
    try {
      return $response.IsSuccessStatusCode
    }
    finally {
      $response.Dispose()
    }
  }
  catch {
    return $false
  }
  finally {
    $client.Dispose()
  }
}

function Get-AdminDevWorkerDescendantId {
  param([Parameter(Mandatory = $true)][int]$RootProcessId)

  try {
    $processes = @(Get-CimInstance -ClassName Win32_Process -Property ProcessId, ParentProcessId, Name -ErrorAction Stop)
  }
  catch {
    return $null
  }
  $descendants = [Collections.Generic.HashSet[int]]::new()
  $null = $descendants.Add($RootProcessId)
  $changed = $true
  while ($changed) {
    $changed = $false
    foreach ($process in $processes) {
      if ($descendants.Contains([int]$process.ParentProcessId) -and
          -not $descendants.Contains([int]$process.ProcessId)) {
        $null = $descendants.Add([int]$process.ProcessId)
        $changed = $true
      }
    }
  }
  $worker = $processes |
    Where-Object { $descendants.Contains([int]$_.ProcessId) -and [string]$_.Name -ieq 'admin-worker.exe' } |
    Select-Object -First 1
  if ($null -eq $worker) {
    return $null
  }
  return [int]$worker.ProcessId
}

function Wait-AdminDevRuntimeReady {
  param(
    [Parameter(Mandatory = $true)][object[]]$States,
    [ValidateRange(10, 600)][int]$TimeoutSeconds = 180
  )

  $workerState = $States | Where-Object { $_.Name -ceq 'worker' } | Select-Object -First 1
  if ($null -eq $workerState) {
    throw 'ADMIN_DEV_WORKER_STATE_MISSING'
  }
  $stableWorkerId = $null
  $stableSince = [DateTime]::MinValue
  $lastHttpCheck = [DateTime]::MinValue
  $viteReady = $false
  $healthReady = $false
  $apiReady = $false
  $stopwatch = [Diagnostics.Stopwatch]::StartNew()
  while ($stopwatch.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
    Receive-AdminDevManagedProcessLines -States $States
    Assert-AdminDevManagedProcessesRunning -States $States

    if (([DateTime]::UtcNow - $lastHttpCheck).TotalMilliseconds -ge 500) {
      $viteReady = Test-AdminDevHttpReady -Uri 'http://127.0.0.1:5173'
      $healthReady = Test-AdminDevHttpReady -Uri 'http://127.0.0.1:8080/health'
      $apiReady = Test-AdminDevHttpReady -Uri 'http://127.0.0.1:8080/ready'
      $lastHttpCheck = [DateTime]::UtcNow
    }

    $workerId = Get-AdminDevWorkerDescendantId -RootProcessId $workerState.Process.Id
    if ($null -eq $workerId) {
      $stableWorkerId = $null
      $stableSince = [DateTime]::MinValue
    }
    elseif ($stableWorkerId -ne $workerId) {
      $stableWorkerId = $workerId
      $stableSince = [DateTime]::UtcNow
    }
    $workerReady = $null -ne $stableWorkerId -and
      ([DateTime]::UtcNow - $stableSince).TotalSeconds -ge 3

    if ($viteReady -and $healthReady -and $apiReady -and $workerReady) {
      return
    }
    Start-Sleep -Milliseconds 100
  }
  throw "ADMIN_DEV_READINESS_TIMEOUT: $TimeoutSeconds seconds"
}

function Watch-AdminDevManagedProcesses {
  param([Parameter(Mandatory = $true)][object[]]$States)

  while ($true) {
    Receive-AdminDevManagedProcessLines -States $States
    Assert-AdminDevManagedProcessesRunning -States $States
    Start-Sleep -Milliseconds 100
  }
}
