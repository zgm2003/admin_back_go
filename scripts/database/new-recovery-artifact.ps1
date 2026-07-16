[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [Parameter(Mandatory = $true)]
  [string]$BackupRoot,

  [string]$MySQLDumpCommand = 'mysqldump',

  [string]$MySQLCommand = 'mysql',

  [string]$DockerCommand = 'docker',

  [ValidateRange(1, 7200)]
  [int]$CommandTimeoutSeconds = 1800,

  [ValidateRange(1, 600)]
  [int]$ReadinessTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$criticalTables = @(
  'users',
  'wallet_transactions',
  'user_sessions',
  'export_tasks',
  'ai_runs',
  'notifications'
)

if (-not ('AdminRecovery.PhysicalPath' -as [type])) {
  Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using Microsoft.Win32.SafeHandles;

namespace AdminRecovery
{
    public static class PhysicalPath
    {
        private const uint FileFlagBackupSemantics = 0x02000000;

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern SafeFileHandle CreateFileW(
            string fileName,
            uint desiredAccess,
            FileShare shareMode,
            IntPtr securityAttributes,
            FileMode creationDisposition,
            uint flagsAndAttributes,
            IntPtr templateFile);

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern uint GetFinalPathNameByHandleW(
            SafeFileHandle file,
            StringBuilder filePath,
            uint filePathLength,
            uint flags);

        public static string GetFinalPath(string path)
        {
            using (SafeFileHandle handle = CreateFileW(
                path,
                0,
                FileShare.ReadWrite | FileShare.Delete,
                IntPtr.Zero,
                FileMode.Open,
                FileFlagBackupSemantics,
                IntPtr.Zero))
            {
                if (handle.IsInvalid)
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error());
                }

                var buffer = new StringBuilder(512);
                uint length = GetFinalPathNameByHandleW(handle, buffer, (uint)buffer.Capacity, 0);
                if (length == 0)
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error());
                }
                if (length >= buffer.Capacity)
                {
                    buffer.Capacity = checked((int)length + 1);
                    length = GetFinalPathNameByHandleW(handle, buffer, (uint)buffer.Capacity, 0);
                    if (length == 0 || length >= buffer.Capacity)
                    {
                        throw new Win32Exception(Marshal.GetLastWin32Error());
                    }
                }
                return buffer.ToString();
            }
        }
    }
}
'@ | Out-Null
}

function Get-RequiredEnvironmentValue {
  param([Parameter(Mandatory = $true)][string]$Name)

  $value = [Environment]::GetEnvironmentVariable($Name, 'Process')
  if ([string]::IsNullOrWhiteSpace($value)) {
    throw "$Name is required"
  }
  if ($value.IndexOfAny([char[]]@("`0", "`r", "`n")) -ge 0) {
    throw "$Name contains an unsupported control character"
  }
  return $value
}

function ConvertTo-MySQLOptionValue {
  param([Parameter(Mandatory = $true)][string]$Value)

  return '"' + $Value.Replace('\', '\\').Replace('"', '\"') + '"'
}

function Test-PathWithin {
  param(
    [Parameter(Mandatory = $true)][string]$Candidate,
    [Parameter(Mandatory = $true)][string]$Parent
  )

  $candidatePath = [System.IO.Path]::GetFullPath($Candidate).TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
  $parentPath = [System.IO.Path]::GetFullPath($Parent).TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
  return $candidatePath.Equals($parentPath, [System.StringComparison]::OrdinalIgnoreCase) -or
    $candidatePath.StartsWith($parentPath + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)
}

function ConvertFrom-ExtendedPathPrefix {
  param([Parameter(Mandatory = $true)][string]$Path)

  if ($Path.StartsWith('\\?\UNC\', [System.StringComparison]::OrdinalIgnoreCase)) {
    return '\\' + $Path.Substring(8)
  }
  if ($Path.StartsWith('\\?\', [System.StringComparison]::OrdinalIgnoreCase)) {
    return $Path.Substring(4)
  }
  return $Path
}

function Test-IsUnsupportedNetworkPath {
  param([Parameter(Mandatory = $true)][string]$Path)

  $fullPath = [System.IO.Path]::GetFullPath($Path)
  if ($fullPath.StartsWith('\\?\', [System.StringComparison]::OrdinalIgnoreCase)) {
    $extendedPath = $fullPath.Substring(4)
    return $extendedPath -notmatch '^[A-Za-z]:[\\/]'
  }
  return $fullPath.StartsWith('\\', [System.StringComparison]::OrdinalIgnoreCase)
}

function Get-PhysicalPath {
  param([Parameter(Mandatory = $true)][string]$Path)

  $fullPath = ConvertFrom-ExtendedPathPrefix -Path ([System.IO.Path]::GetFullPath($Path))
  $probe = [System.IO.Path]::TrimEndingDirectorySeparator($fullPath)
  $missingSegments = [System.Collections.Generic.List[string]]::new()

  while (-not [System.IO.Directory]::Exists($probe) -and -not [System.IO.File]::Exists($probe)) {
    $segment = [System.IO.Path]::GetFileName($probe)
    $parent = [System.IO.Directory]::GetParent($probe)
    if ([string]::IsNullOrWhiteSpace($segment) -or $null -eq $parent) {
      throw 'BackupRoot physical path could not be resolved'
    }
    $missingSegments.Insert(0, $segment)
    $probe = $parent.FullName
  }

  $physicalPath = ConvertFrom-ExtendedPathPrefix -Path ([AdminRecovery.PhysicalPath]::GetFinalPath($probe))
  foreach ($segment in $missingSegments) {
    $physicalPath = [System.IO.Path]::Combine($physicalPath, $segment)
  }
  return [System.IO.Path]::GetFullPath($physicalPath)
}

function Assert-SafeBackupPath {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$BackendRoot,
    [Parameter(Mandatory = $true)][string]$BackendPhysicalRoot,
    [Parameter(Mandatory = $true)][string]$FrontendRoot
  )

  $fullPath = [System.IO.Path]::GetFullPath($Path)
  if (Test-IsUnsupportedNetworkPath -Path $fullPath) {
    throw 'BackupRoot must use a local filesystem path'
  }
  $pathRoot = [System.IO.Path]::GetPathRoot($fullPath)
  if (-not [string]::IsNullOrWhiteSpace($pathRoot)) {
    try {
      if ([System.IO.DriveInfo]::new($pathRoot).DriveType -eq [System.IO.DriveType]::Network) {
        throw 'BackupRoot must use a local filesystem path'
      }
    } catch [System.ArgumentException] {
      throw 'BackupRoot physical path could not be resolved'
    }
  }

  $physicalPath = Get-PhysicalPath -Path $fullPath
  if (Test-IsUnsupportedNetworkPath -Path $physicalPath) {
    throw 'BackupRoot must use a local filesystem path'
  }
  if ((Test-PathWithin -Candidate $fullPath -Parent $BackendRoot) -or
    (Test-PathWithin -Candidate $fullPath -Parent $FrontendRoot) -or
    (Test-PathWithin -Candidate $physicalPath -Parent $BackendPhysicalRoot) -or
    ((Test-Path -LiteralPath $FrontendRoot) -and
      (Test-PathWithin -Candidate $physicalPath -Parent (Get-PhysicalPath -Path $FrontendRoot)))) {
    throw 'BackupRoot must be outside the backend and frontend repositories'
  }
  return $physicalPath
}

function Resolve-ClientCommand {
  param([Parameter(Mandatory = $true)][string]$Command)

  $resolved = Get-Command -Name $Command -ErrorAction Stop | Select-Object -First 1
  if ([string]::IsNullOrWhiteSpace($resolved.Source)) {
    throw 'database client command could not be resolved'
  }
  return $resolved.Source
}

function Invoke-DatabaseClient {
  param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Operation,
    [Parameter(Mandatory = $true)][int]$TimeoutSeconds
  )

  $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true

  if ([System.IO.Path]::GetExtension($Executable).Equals('.ps1', [System.StringComparison]::OrdinalIgnoreCase)) {
    $startInfo.FileName = [Environment]::ProcessPath
    $payloadJSON = [ordered]@{
      script    = $Executable
      arguments = @($Arguments)
    } | ConvertTo-Json -Compress -Depth 3
    $startInfo.Environment['ADMIN_RECOVERY_PROCESS_PAYLOAD'] = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($payloadJSON))
    $launcherSource = @'
$ErrorActionPreference = 'Stop'
try {
  $payloadBytes = [Convert]::FromBase64String($env:ADMIN_RECOVERY_PROCESS_PAYLOAD)
  $payload = [System.Text.Encoding]::UTF8.GetString($payloadBytes) | ConvertFrom-Json
  Remove-Item Env:ADMIN_RECOVERY_PROCESS_PAYLOAD -ErrorAction SilentlyContinue
  $scriptPath = [string]$payload.script
  $scriptArguments = @($payload.arguments | ForEach-Object { [string]$_ })
  & $scriptPath @scriptArguments
  exit 0
} catch {
  exit 1
}
'@
    $encodedLauncher = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($launcherSource))
    foreach ($prefixArgument in @('-NoProfile', '-NonInteractive', '-EncodedCommand', $encodedLauncher)) {
      [void]$startInfo.ArgumentList.Add($prefixArgument)
    }
  } else {
    $startInfo.FileName = $Executable
    foreach ($argument in $Arguments) {
      [void]$startInfo.ArgumentList.Add($argument)
    }
  }

  $process = [System.Diagnostics.Process]::new()
  $process.StartInfo = $startInfo
  try {
    try {
      if (-not $process.Start()) {
        throw 'process did not start'
      }
    } catch {
      throw "$Operation failed to start"
    }

    $standardOutputTask = $process.StandardOutput.ReadToEndAsync()
    $standardErrorTask = $process.StandardError.ReadToEndAsync()
    if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
      $processID = $process.Id
      $terminationErrors = [System.Collections.Generic.List[string]]::new()
      try {
        $process.Kill($true)
      } catch {
        $terminationErrors.Add('kill request failed')
      }
      $terminated = $false
      try {
        $terminated = $process.WaitForExit(5000)
      } catch {
        $terminationErrors.Add('termination wait failed')
      }
      if (-not $terminated) {
        $terminationErrors.Add('process remained alive after kill request')
      }
      if ($process.HasExited) {
        [void]$standardOutputTask.GetAwaiter().GetResult()
        [void]$standardErrorTask.GetAwaiter().GetResult()
      }
      if ($terminationErrors.Count -gt 0) {
        throw ("$Operation timed out after $TimeoutSeconds seconds; process tree termination failed for PID $processID")
      }
      throw "$Operation timed out after $TimeoutSeconds seconds"
    }

    $standardOutput = $standardOutputTask.GetAwaiter().GetResult()
    [void]$standardErrorTask.GetAwaiter().GetResult()
    $exitCode = $process.ExitCode
  } finally {
    $process.Dispose()
  }

  if ($exitCode -ne 0) {
    throw "$Operation failed with exit code $exitCode"
  }
  if ([string]::IsNullOrEmpty($standardOutput)) {
    return @()
  }
  return @($standardOutput -split "`r?`n" | Where-Object { $_.Length -gt 0 })
}

function Get-DatabaseCounts {
  param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string[]]$ClientArguments,
    [Parameter(Mandatory = $true)][string]$Schema,
    [Parameter(Mandatory = $true)][string]$Operation,
    [Parameter(Mandatory = $true)][int]$TimeoutSeconds
  )

  $queries = foreach ($table in $criticalTables) {
    "SELECT '$table', COUNT(*) FROM ``$table``"
  }
  $sql = $queries -join ' UNION ALL '
  $arguments = @($ClientArguments) + @(
    '--batch',
    '--skip-column-names',
    '--raw',
    "--database=$Schema",
    "--execute=$sql"
  )
  $lines = Invoke-DatabaseClient -Executable $Executable -Arguments $arguments -Operation $Operation -TimeoutSeconds $TimeoutSeconds
  $counts = [ordered]@{}
  foreach ($line in $lines) {
    if ([string]::IsNullOrWhiteSpace($line)) {
      continue
    }
    $parts = $line -split "`t", 2
    if ($parts.Count -ne 2 -or $criticalTables -notcontains $parts[0]) {
      throw 'database count output was invalid'
    }
    if ($counts.Contains($parts[0])) {
      throw 'database count output was invalid'
    }
    [long]$count = 0
    if (-not [long]::TryParse($parts[1], [Globalization.NumberStyles]::None, [Globalization.CultureInfo]::InvariantCulture, [ref]$count) -or $count -lt 0) {
      throw 'database count output was invalid'
    }
    $counts[$parts[0]] = $count
  }
  foreach ($table in $criticalTables) {
    if (-not $counts.Contains($table)) {
      throw "database count output omitted $table"
    }
  }
  return $counts
}

function Wait-RestoreContainer {
  param(
    [Parameter(Mandatory = $true)][string]$DockerExecutable,
    [Parameter(Mandatory = $true)][string]$ContainerName,
    [Parameter(Mandatory = $true)][int]$TimeoutSeconds
  )

  [void](Invoke-DatabaseClient -Executable $DockerExecutable -Arguments @(
      'exec',
      $ContainerName,
      'sh',
      '-c',
      'until [ "$(cat /proc/1/comm)" = "mysqld" ] && mysqladmin --protocol=socket --user=root ping --silent; do sleep 1; done'
    ) -Operation 'restore container readiness' -TimeoutSeconds $TimeoutSeconds)
}

function Write-RestrictedOptionFile {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Content
  )

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

  $stream = $null
  try {
    $stream = [System.IO.FileSystemAclExtensions]::Create(
      [System.IO.FileInfo]::new($Path),
      [System.IO.FileMode]::CreateNew,
      [System.Security.AccessControl.FileSystemRights]::Write,
      [System.IO.FileShare]::None,
      4096,
      [System.IO.FileOptions]::None,
      $security
    )
    $bytes = [System.Text.UTF8Encoding]::new($false).GetBytes($Content)
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Flush($true)
  } finally {
    if ($null -ne $stream) {
      $stream.Dispose()
    }
  }
}

$backendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$frontendRoot = Join-Path (Split-Path -Parent $backendRoot) 'admin_front_ts'
$backupRootPath = [System.IO.Path]::GetFullPath($BackupRoot)
$backendPhysicalRoot = Get-PhysicalPath -Path $backendRoot
[void](Assert-SafeBackupPath -Path $backupRootPath -BackendRoot $backendRoot -BackendPhysicalRoot $backendPhysicalRoot -FrontendRoot $frontendRoot)

$hostName = Get-RequiredEnvironmentValue -Name 'ADMIN_DB_HOST'
$portText = Get-RequiredEnvironmentValue -Name 'ADMIN_DB_PORT'
$userName = Get-RequiredEnvironmentValue -Name 'ADMIN_DB_USER'
$password = Get-RequiredEnvironmentValue -Name 'ADMIN_DB_PASSWORD'
[int]$port = 0
if (-not [int]::TryParse($portText, [ref]$port) -or $port -lt 1 -or $port -gt 65535) {
  throw 'ADMIN_DB_PORT must be an integer between 1 and 65535'
}

$dumpExecutable = Resolve-ClientCommand -Command $MySQLDumpCommand
$mysqlExecutable = Resolve-ClientCommand -Command $MySQLCommand
$dockerExecutable = Resolve-ClientCommand -Command $DockerCommand

New-Item -ItemType Directory -Force -Path $backupRootPath | Out-Null
[void](Assert-SafeBackupPath -Path $backupRootPath -BackendRoot $backendRoot -BackendPhysicalRoot $backendPhysicalRoot -FrontendRoot $frontendRoot)
$artifactDirectory = Join-Path $backupRootPath ((Get-Date -Format 'yyyyMMddTHHmmssfff') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 12))
New-Item -ItemType Directory -Path $artifactDirectory | Out-Null
[void](Assert-SafeBackupPath -Path $artifactDirectory -BackendRoot $backendRoot -BackendPhysicalRoot $backendPhysicalRoot -FrontendRoot $frontendRoot)
$dumpPath = Join-Path $artifactDirectory ($Database + '.sql')
$artifactPath = Join-Path $artifactDirectory 'artifact.json'
$secretFile = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-mysql-' + [guid]::NewGuid().ToString('N') + '.cnf')
$restoreToken = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(6)).ToLowerInvariant()
$restoreDatabase = 'admin_restore_' + $restoreToken
$restoreContainer = 'admin-recovery-' + $restoreToken
$restoreImage = 'mysql:8.4.10'
$restoreCreated = $false
$restoreMayExist = $false
$containerMayExist = $false
$containerCleanupRequired = $false
$containerID = $null
$primaryErrorMessage = $null
$cleanupErrors = [System.Collections.Generic.List[string]]::new()

try {
  $optionFile = @(
    '[client]',
    'protocol="tcp"',
    ('host=' + (ConvertTo-MySQLOptionValue -Value $hostName)),
    ('port=' + $port),
    ('user=' + (ConvertTo-MySQLOptionValue -Value $userName)),
    ('password=' + (ConvertTo-MySQLOptionValue -Value $password))
  ) -join "`n"
  Write-RestrictedOptionFile -Path $secretFile -Content ($optionFile + "`n")

  $sourceCounts = Get-DatabaseCounts `
    -Executable $mysqlExecutable `
    -ClientArguments @("--defaults-extra-file=$secretFile") `
    -Schema $Database `
    -Operation 'read source database counts' `
    -TimeoutSeconds $CommandTimeoutSeconds
  $dumpArguments = @(
    "--defaults-extra-file=$secretFile",
    '--single-transaction',
    '--quick',
    '--routines',
    '--triggers',
    '--events',
    '--default-character-set=utf8mb4',
    '--no-tablespaces',
    "--result-file=$dumpPath",
    $Database
  )
  [void](Invoke-DatabaseClient -Executable $dumpExecutable -Arguments $dumpArguments -Operation 'create recovery dump' -TimeoutSeconds $CommandTimeoutSeconds)

  $dumpInfo = Get-Item -LiteralPath $dumpPath
  if ($dumpInfo.Length -le 0) {
    throw 'recovery dump is empty'
  }
  foreach ($definition in @('CREATE TABLE `users`', 'CREATE TABLE `wallet_transactions`')) {
    if (-not (Select-String -LiteralPath $dumpPath -SimpleMatch -Pattern $definition -Quiet)) {
      throw 'recovery dump is missing a critical table definition'
    }
  }
  $dumpSHA256 = (Get-FileHash -LiteralPath $dumpPath -Algorithm SHA256).Hash.ToLowerInvariant()

  $containerMayExist = $true
  $runOutput = @(Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
      'run',
      '--detach',
      '--name',
      $restoreContainer,
      '--label',
      "admin.recovery.token=$restoreToken",
      '--network',
      'none',
      '--env',
      'MYSQL_ALLOW_EMPTY_PASSWORD=yes',
      $restoreImage
    ) -Operation 'start restore container' -TimeoutSeconds $CommandTimeoutSeconds)
  $containerCleanupRequired = $true
  if ($runOutput.Count -ne 1 -or $runOutput[0] -notmatch '^[0-9a-f]{64}$') {
    throw 'restore container returned an invalid container ID'
  }
  $containerID = $runOutput[0]
  Wait-RestoreContainer -DockerExecutable $dockerExecutable -ContainerName $restoreContainer -TimeoutSeconds $ReadinessTimeoutSeconds

  $createSQL = "CREATE DATABASE ``$restoreDatabase`` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"
  $restoreMayExist = $true
  [void](Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
      'exec',
      $restoreContainer,
      'mysql',
      '--protocol=socket',
      '--user=root',
      "--execute=$createSQL"
    ) -Operation 'create restore database' -TimeoutSeconds $CommandTimeoutSeconds)
  $restoreCreated = $true

  [void](Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
      'cp',
      $dumpPath,
      "${restoreContainer}:/tmp/recovery.sql"
    ) -Operation 'copy recovery dump into restore container' -TimeoutSeconds $CommandTimeoutSeconds)
  [void](Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
      'exec',
      $restoreContainer,
      'mysql',
      '--protocol=socket',
      '--user=root',
      "--database=$restoreDatabase",
      '--execute=SOURCE /tmp/recovery.sql'
    ) -Operation 'restore recovery dump' -TimeoutSeconds $CommandTimeoutSeconds)
  $restoreCounts = Get-DatabaseCounts `
    -Executable $dockerExecutable `
    -ClientArguments @('exec', $restoreContainer, 'mysql', '--protocol=socket', '--user=root') `
    -Schema $restoreDatabase `
    -Operation 'read restored database counts' `
    -TimeoutSeconds $CommandTimeoutSeconds

  foreach ($table in $criticalTables) {
    if ($sourceCounts[$table] -ne $restoreCounts[$table]) {
      throw "restored row count mismatch for $table"
    }
  }

  $artifact = [ordered]@{
    database         = $Database
    created_at       = [DateTimeOffset]::UtcNow.ToString('o')
    dump_path        = $dumpPath
    dump_bytes       = $dumpInfo.Length
    dump_sha256      = $dumpSHA256
    restore_database = $restoreDatabase
    source_counts    = $sourceCounts
    restore_counts   = $restoreCounts
    verified         = $true
  }
} catch {
  $primaryErrorMessage = ($_.Exception.Message -replace '[\r\n]+', ' ').Replace($password, '[REDACTED]')
} finally {
  if ($containerMayExist -and
    (-not $containerCleanupRequired -or $containerID -notmatch '^[0-9a-f]{64}$')) {
    try {
      $discoveredContainers = @(Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
          'ps',
          '--all',
          '--quiet',
          '--filter',
          "label=admin.recovery.token=$restoreToken"
        ) -Operation 'discover restore container ownership' -TimeoutSeconds $CommandTimeoutSeconds)
      if ($discoveredContainers.Count -eq 1 -and $discoveredContainers[0] -match '^[0-9a-f]{64}$') {
        $containerID = $discoveredContainers[0]
        $containerCleanupRequired = $true
      } elseif ($discoveredContainers.Count -gt 0 -or $containerCleanupRequired) {
        $cleanupErrors.Add('restore container ownership discovery failed')
      }
    } catch {
      $cleanupErrors.Add('restore container ownership discovery failed')
    }
  }

  $containerIdentityVerified = $false
  if ($containerCleanupRequired) {
    if ($restoreContainer -notmatch '^admin-recovery-[0-9a-f]{12}$' -or
      $containerID -notmatch '^[0-9a-f]{64}$') {
      $cleanupErrors.Add('restore container identity verification failed')
    } else {
      try {
        $inspectOutput = @(Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
            'inspect',
            '--format',
            '{{.Id}}|{{index .Config.Labels "admin.recovery.token"}}|{{.Name}}',
            $containerID
          ) -Operation 'verify restore container identity' -TimeoutSeconds $CommandTimeoutSeconds)
        if ($inspectOutput.Count -ne 1 -or
          $inspectOutput[0] -cne ($containerID + '|' + $restoreToken + '|/' + $restoreContainer)) {
          throw 'container identity did not match'
        }
        $containerIdentityVerified = $true
      } catch {
        $cleanupErrors.Add('restore container identity verification failed')
      }
    }
  }

  if ($restoreMayExist -and $containerIdentityVerified) {
    if ($restoreDatabase -notmatch '^admin_restore_[0-9a-f]{12}$') {
      $cleanupErrors.Add('refusing to drop an unexpected restore database')
    } else {
      try {
        $dropSQL = "DROP DATABASE IF EXISTS ``$restoreDatabase``"
        [void](Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
            'exec',
            $containerID,
            'mysql',
            '--protocol=socket',
            '--user=root',
            "--execute=$dropSQL"
          ) -Operation 'drop restore database' -TimeoutSeconds $CommandTimeoutSeconds)
      } catch {
        $cleanupErrors.Add('restore database cleanup failed')
      }
    }
  }

  if ($containerCleanupRequired -and $containerIdentityVerified) {
    try {
      [void](Invoke-DatabaseClient -Executable $dockerExecutable -Arguments @(
          'rm',
          '--force',
          '--volumes',
          $containerID
        ) -Operation 'remove restore container' -TimeoutSeconds $CommandTimeoutSeconds)
    } catch {
      $cleanupErrors.Add('restore container cleanup failed')
    }
  }
  try {
    Remove-Item -LiteralPath $secretFile -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $secretFile) {
      throw 'temporary MySQL option file remains'
    }
  } catch {
    $cleanupErrors.Add('temporary MySQL option file cleanup failed')
  }
}

if ($null -ne $primaryErrorMessage) {
  if ($cleanupErrors.Count -gt 0) {
    throw ($primaryErrorMessage + '; cleanup also failed: ' + ($cleanupErrors -join '; '))
  }
  throw $primaryErrorMessage
}
if ($cleanupErrors.Count -gt 0) {
  throw ('cleanup failed: ' + ($cleanupErrors -join '; '))
}

$postCleanupDumpInfo = Get-Item -LiteralPath $dumpPath
$postCleanupDumpSHA256 = (Get-FileHash -LiteralPath $dumpPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($postCleanupDumpInfo.Length -ne $dumpInfo.Length -or $postCleanupDumpSHA256 -cne $dumpSHA256) {
  throw 'recovery dump changed during verification'
}

$artifactJSON = $artifact | ConvertTo-Json -Depth 5
$artifactTemporaryPath = $artifactPath + '.tmp'
$artifactPublicationError = $null
$artifactCleanupErrors = [System.Collections.Generic.List[string]]::new()
$artifactStream = $null
try {
  try {
    $artifactStream = [System.IO.FileStream]::new(
      $artifactTemporaryPath,
      [System.IO.FileMode]::CreateNew,
      [System.IO.FileAccess]::Write,
      [System.IO.FileShare]::None
    )
    $artifactBytes = [System.Text.UTF8Encoding]::new($false).GetBytes($artifactJSON + "`n")
    $artifactStream.Write($artifactBytes, 0, $artifactBytes.Length)
    $artifactStream.Flush($true)
  } finally {
    if ($null -ne $artifactStream) {
      $artifactStream.Dispose()
      $artifactStream = $null
    }
  }
  [System.IO.File]::Move($artifactTemporaryPath, $artifactPath)
} catch {
  $artifactPublicationError = 'artifact publication failed'
} finally {
  try {
    if (Test-Path -LiteralPath $artifactTemporaryPath) {
      Remove-Item -LiteralPath $artifactTemporaryPath -Force -ErrorAction Stop
    }
    if (Test-Path -LiteralPath $artifactTemporaryPath) {
      throw 'artifact temporary path remains'
    }
  } catch {
    $artifactCleanupErrors.Add('artifact temporary file cleanup failed')
  }
}

if ($null -ne $artifactPublicationError) {
  if ($artifactCleanupErrors.Count -gt 0) {
    throw ($artifactPublicationError + '; cleanup also failed: ' + ($artifactCleanupErrors -join '; '))
  }
  throw $artifactPublicationError
}
if ($artifactCleanupErrors.Count -gt 0) {
  throw ('artifact publication cleanup failed: ' + ($artifactCleanupErrors -join '; '))
}

Write-Output $artifactPath
Write-Output $dumpSHA256
