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
  if ($null -eq $record -or
      [string]$record.lock_id -cne [string]$Handle.LockId -or
      [int]$record.pid -ne [int]$Handle.ProcessId -or
      [string]$record.process_started_at_utc -cne [string]$Handle.ProcessStartedAtUtc -or
      -not [StringComparer]::OrdinalIgnoreCase.Equals(
        (Get-AdminDevCanonicalPath -Path ([string]$record.repository_root)),
        (Get-AdminDevCanonicalPath -Path ([string]$Handle.RepositoryRoot))
      )) {
    return
  }
  Remove-Item -LiteralPath ([string]$Handle.Path) -Force -ErrorAction SilentlyContinue
}
