$script:AtlasImage = 'arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a'

function Get-MySQLDSNSettings {
  param([Parameter(Mandatory = $true)][string]$Database)

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
  $query = if ($queryIndex -ge 0) { $dsn.Substring($queryIndex) } else { '' }
  return [pscustomobject]@{
    User = $credentials.Substring(0, $credentialSeparator)
    Password = $credentials.Substring($credentialSeparator + 1)
    Host = $address.Substring(0, $portSeparator).Trim('[', ']')
    Port = $address.Substring($portSeparator + 1)
    Query = $query
  }
}

function New-SchemaDSN {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )
  return "$($Settings.User):$($Settings.Password)@tcp($($Settings.Host):$($Settings.Port))/$Database$($Settings.Query)"
}

function Get-AtlasDatabaseURL {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )
  $hostName = $Settings.Host
  if ($hostName -in @('127.0.0.1', 'localhost', '::1')) { $hostName = 'host.docker.internal' }
  if ($hostName.Contains(':') -and -not $hostName.StartsWith('[')) { $hostName = "[$hostName]" }
  $user = [uri]::EscapeDataString([string]$Settings.User)
  $password = [uri]::EscapeDataString([string]$Settings.Password)
  $schema = [uri]::EscapeDataString($Database)
  return "mysql://${user}:${password}@${hostName}:$($Settings.Port)/${schema}"
}

function Write-RestrictedTextFile {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Content
  )
  if (Test-Path -LiteralPath $Path) { throw 'restricted file already exists' }
  if ($IsWindows) {
    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    $security = [System.Security.AccessControl.FileSecurity]::new()
    $security.SetOwner($identity.User)
    $security.SetAccessRuleProtection($true, $false)
    $rule = [System.Security.AccessControl.FileSystemAccessRule]::new(
      $identity.User,
      [System.Security.AccessControl.FileSystemRights]::FullControl,
      [System.Security.AccessControl.AccessControlType]::Allow
    )
    [void]$security.AddAccessRule($rule)
    $stream = [System.IO.FileSystemAclExtensions]::Create(
      [System.IO.FileInfo]::new($Path),
      [System.IO.FileMode]::CreateNew,
      [System.Security.AccessControl.FileSystemRights]::Write,
      [System.IO.FileShare]::None,
      4096,
      [System.IO.FileOptions]::None,
      $security
    )
  } else {
    $stream = [System.IO.FileStream]::new($Path, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
    & chmod 600 -- $Path
    if ($LASTEXITCODE -ne 0) {
      $stream.Dispose()
      throw 'failed to restrict runtime file permissions'
    }
  }
  try {
    $bytes = [System.Text.UTF8Encoding]::new($false).GetBytes($Content)
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Flush($true)
  } finally {
    $stream.Dispose()
  }
}

function Invoke-BoundedCommand {
  param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Operation,
    [ValidateRange(1, 3600)][int]$TimeoutSeconds = 300
  )
  $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
  $startInfo.FileName = $Executable
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  foreach ($argument in $Arguments) { [void]$startInfo.ArgumentList.Add($argument) }
  $process = [System.Diagnostics.Process]::new()
  $process.StartInfo = $startInfo
  try {
    if (-not $process.Start()) { throw "$Operation failed to start" }
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
      try { $process.Kill($true) } catch { }
      [void]$process.WaitForExit(5000)
      throw "$Operation timed out"
    }
    $stdout = $stdoutTask.GetAwaiter().GetResult()
    [void]$stderrTask.GetAwaiter().GetResult()
    if ($process.ExitCode -ne 0) { throw "$Operation failed" }
    return $stdout
  } finally {
    $process.Dispose()
  }
}

function New-AtlasRuntimeConfig {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )
  $directory = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-atlas-runtime-' + [guid]::NewGuid().ToString('N'))
  [void](New-Item -ItemType Directory -Path $directory)
  $configPath = Join-Path $directory 'atlas.hcl'
  $url = Get-AtlasDatabaseURL -Settings $Settings -Database $Database
  $content = 'env "runtime" {' + "`n  " + 'url = "' + $url + '"' + "`n}`n"
  Write-RestrictedTextFile -Path $configPath -Content $content
  return $directory
}

function Remove-AtlasRuntimeConfig {
  param([string]$Directory)
  if ([string]::IsNullOrWhiteSpace($Directory) -or -not (Test-Path -LiteralPath $Directory)) { return }
  $resolved = [System.IO.Path]::GetFullPath($Directory)
  $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd([System.IO.Path]::DirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
  if (-not $resolved.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolved) -notmatch '^admin-atlas-runtime-[0-9a-f]{32}$') {
    throw 'refusing to remove unexpected Atlas runtime directory'
  }
  Remove-Item -LiteralPath $resolved -Recurse -Force
}

function Invoke-AtlasContainer {
  param(
    [Parameter(Mandatory = $true)][string]$DockerExecutable,
    [Parameter(Mandatory = $true)][string]$BackendRoot,
    [Parameter(Mandatory = $true)][string]$RuntimeDirectory,
    [Parameter(Mandatory = $true)][string[]]$AtlasArguments,
    [switch]$NetworkNone,
    [ValidateRange(1, 3600)][int]$TimeoutSeconds = 300
  )
  $arguments = @('run', '--rm')
  if ($NetworkNone) {
    $arguments += @('--network', 'none')
  } else {
    $arguments += @('--add-host', 'host.docker.internal:host-gateway')
  }
  $arguments += @(
    '--volume', "${BackendRoot}:/workspace:ro",
    '--volume', "${RuntimeDirectory}:/runtime:ro",
    '--workdir', '/workspace',
    $script:AtlasImage
  )
  $arguments += $AtlasArguments
  return Invoke-BoundedCommand -Executable $DockerExecutable -Arguments $arguments -Operation 'Atlas command' -TimeoutSeconds $TimeoutSeconds
}

function Get-DatabaseFingerprintSHA {
  param(
    [Parameter(Mandatory = $true)][string]$BackendRoot,
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )
  $path = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-fingerprint-' + [guid]::NewGuid().ToString('N') + '.json')
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  try {
    $env:MYSQL_DSN = New-SchemaDSN -Settings $Settings -Database $Database
    $commit = (& git -C $BackendRoot rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') { throw 'Git commit could not be resolved' }
    Push-Location $BackendRoot
    try {
      & go run ./cmd/admin-db fingerprint --schema $Database --out $path --commit $commit 2>$null | Out-Null
      if ($LASTEXITCODE -ne 0) { throw 'schema fingerprint capture failed' }
    } finally {
      Pop-Location
    }
    $document = Get-Content -Raw -LiteralPath $path -Encoding utf8 | ConvertFrom-Json
    if ([string]$document.schema_sha256 -notmatch '^[0-9a-f]{64}$') { throw 'schema fingerprint output was invalid' }
    return [string]$document.schema_sha256
  } finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
  }
}

function Invoke-MySQLStatement {
  param(
    [Parameter(Mandatory = $true)][string]$MySQLExecutable,
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$SQL
  )
  $previousPassword = [Environment]::GetEnvironmentVariable('MYSQL_PWD', 'Process')
  try {
    $env:MYSQL_PWD = $Settings.Password
    $arguments = @('--protocol=tcp', "--host=$($Settings.Host)", "--port=$($Settings.Port)", "--user=$($Settings.User)", '--batch', '--skip-column-names', '--raw', "--execute=$SQL")
    return Invoke-BoundedCommand -Executable $MySQLExecutable -Arguments $arguments -Operation 'MySQL statement' -TimeoutSeconds 60
  } finally {
    [Environment]::SetEnvironmentVariable('MYSQL_PWD', $previousPassword, 'Process')
  }
}
