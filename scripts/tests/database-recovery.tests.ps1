[CmdletBinding()]
param([switch]$ArgumentTransportPocOnly)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-True {
  param(
    [Parameter(Mandatory = $true)][bool]$Condition,
    [Parameter(Mandatory = $true)][string]$Message
  )
  if (-not $Condition) {
    throw $Message
  }
}

function Invoke-TestPowerShellScript {
  param(
    [Parameter(Mandatory = $true)][string]$ScriptPath,
    [Parameter(Mandatory = $true)][string[]]$Arguments
  )

  $payload = [ordered]@{
    script    = $ScriptPath
    arguments = @($Arguments)
  } | ConvertTo-Json -Compress -Depth 3
  $launcher = @'
$ErrorActionPreference = 'Stop'
try {
  $payloadBytes = [Convert]::FromBase64String($env:ADMIN_RECOVERY_PROCESS_PAYLOAD)
  $payload = [System.Text.Encoding]::UTF8.GetString($payloadBytes) | ConvertFrom-Json
  Remove-Item Env:ADMIN_RECOVERY_PROCESS_PAYLOAD -ErrorAction SilentlyContinue
  & ([string]$payload.script) @($payload.arguments | ForEach-Object { [string]$_ })
  exit 0
} catch {
  exit 1
}
'@
  $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
  $startInfo.FileName = [Environment]::ProcessPath
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  [void]$startInfo.ArgumentList.Add('-NoProfile')
  [void]$startInfo.ArgumentList.Add('-NonInteractive')
  [void]$startInfo.ArgumentList.Add('-EncodedCommand')
  [void]$startInfo.ArgumentList.Add([Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($launcher)))
  $startInfo.Environment['ADMIN_RECOVERY_PROCESS_PAYLOAD'] = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($payload))
  $process = [System.Diagnostics.Process]::Start($startInfo)
  $standardOutputTask = $process.StandardOutput.ReadToEndAsync()
  $standardErrorTask = $process.StandardError.ReadToEndAsync()
  if (-not $process.WaitForExit(10000)) {
    $process.Kill($true)
    throw 'test script process timed out'
  }
  [void]$standardOutputTask.GetAwaiter().GetResult()
  [void]$standardErrorTask.GetAwaiter().GetResult()
  return $process.ExitCode
}

$managedEnvironmentNames = @(
  'ADMIN_DB_HOST',
  'ADMIN_DB_PORT',
  'ADMIN_DB_USER',
  'ADMIN_DB_PASSWORD',
  'FAKE_MYSQL_LOG',
  'FAKE_DOCKER_FAIL_RESTORE',
  'FAKE_DOCKER_FAIL_DROP',
  'FAKE_DOCKER_FAIL_RM',
  'FAKE_CNF_CLEANUP_FAILURE',
  'FAKE_DOCKER_HANG_RESTORE',
  'FAKE_DOCKER_HANG_RUN_AFTER_CREATE',
  'FAKE_HANG_CHILD_SCRIPT',
  'FAKE_HANG_CHILD_MARKER',
  'FAKE_DOCKER_STATE_DIR',
  'FAKE_DOCKER_NAME_CONFLICT',
  'FAKE_DOCKER_INSPECT_MISMATCH',
  'FAKE_DOCKER_TAMPER_DUMP',
  'FAKE_DOCKER_ARTIFACT_COLLISION'
)
$originalEnvironment = [ordered]@{}
foreach ($environmentName in $managedEnvironmentNames) {
  $originalEnvironment[$environmentName] = [Environment]::GetEnvironmentVariable($environmentName, 'Process')
  if ($environmentName.StartsWith('FAKE_', [System.StringComparison]::Ordinal)) {
    [Environment]::SetEnvironmentVariable($environmentName, $null, 'Process')
  }
}

$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$recoveryScript = Join-Path $root 'scripts\database\new-recovery-artifact.ps1'
$databaseReadme = Join-Path $root 'database\README.md'
if (-not (Test-Path -LiteralPath $recoveryScript)) {
  throw 'new-recovery-artifact.ps1 is missing'
}

$testRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-recovery-test-' + [guid]::NewGuid().ToString('N'))
$fakeBin = Join-Path $testRoot 'fake-bin'
$backupRoot = Join-Path $testRoot 'backups'
$clientLog = Join-Path $testRoot 'clients.log'
$failureBackupRoot = Join-Path $testRoot 'failure-backups'
$failureClientLog = Join-Path $testRoot 'failure-clients.log'
$cleanupFailureBackupRoot = Join-Path $testRoot 'cleanup-failure-backups'
$cleanupFailureClientLog = Join-Path $testRoot 'cleanup-failure-clients.log'
$timeoutBackupRoot = Join-Path $testRoot 'timeout-backups'
$timeoutClientLog = Join-Path $testRoot 'timeout-clients.log'
$timeoutChildScript = Join-Path $testRoot 'timeout-child.ps1'
$timeoutChildMarker = Join-Path $testRoot 'timeout-child-survived.txt'
$successDockerState = Join-Path $testRoot 'docker-state-success'
$failureDockerState = Join-Path $testRoot 'docker-state-failure'
$cleanupFailureDockerState = Join-Path $testRoot 'docker-state-cleanup-failure'
$timeoutDockerState = Join-Path $testRoot 'docker-state-timeout'
$runTimeoutBackupRoot = Join-Path $testRoot 'run-timeout-backups'
$runTimeoutClientLog = Join-Path $testRoot 'run-timeout-clients.log'
$runTimeoutDockerState = Join-Path $testRoot 'docker-state-run-timeout'
$nameConflictBackupRoot = Join-Path $testRoot 'name-conflict-backups'
$nameConflictClientLog = Join-Path $testRoot 'name-conflict-clients.log'
$nameConflictDockerState = Join-Path $testRoot 'docker-state-name-conflict'
$identityMismatchBackupRoot = Join-Path $testRoot 'identity-mismatch-backups'
$identityMismatchClientLog = Join-Path $testRoot 'identity-mismatch-clients.log'
$identityMismatchDockerState = Join-Path $testRoot 'docker-state-identity-mismatch'
$tamperBackupRoot = Join-Path $testRoot 'tamper-backups'
$tamperClientLog = Join-Path $testRoot 'tamper-clients.log'
$tamperDockerState = Join-Path $testRoot 'docker-state-tamper'
$artifactFailureBackupRoot = Join-Path $testRoot 'artifact-failure-backups'
$artifactFailureClientLog = Join-Path $testRoot 'artifact-failure-clients.log'
$artifactFailureDockerState = Join-Path $testRoot 'docker-state-artifact-failure'
$evidenceRoot = Join-Path $root 'database\evidence'
$junctionTarget = Join-Path $evidenceRoot ('recovery-path-test-' + [guid]::NewGuid().ToString('N'))
$junctionPath = Join-Path $testRoot 'backend-junction'
$junctionBackupRoot = Join-Path $junctionPath 'backups'
$junctionClientLog = Join-Path $testRoot 'junction-clients.log'
$uncTarget = Join-Path $evidenceRoot ('recovery-unc-test-' + [guid]::NewGuid().ToString('N'))
$rootRelativeToDrive = $root.Substring([System.IO.Path]::GetPathRoot($root).Length)
$uncTargetSuffix = ($rootRelativeToDrive + '\database\evidence\' + (Split-Path -Leaf $uncTarget)).Replace('/', '\')
$uncBackupRoots = @(
  ('\\localhost\' + $root.Substring(0, 1) + '$\' + $uncTargetSuffix),
  ('\\?\UNC\localhost\' + $root.Substring(0, 1) + '$\' + $uncTargetSuffix)
)
$password = 'fake-super-secret'

try {
  New-Item -ItemType Directory -Path $fakeBin, $backupRoot | Out-Null

  $argumentProbeScript = Join-Path $fakeBin 'argument-probe.ps1'
  $argumentProbeOutput = Join-Path $testRoot 'argument-probe.txt'
  [System.IO.File]::WriteAllText($argumentProbeScript, @'
$ErrorActionPreference = 'Stop'
[System.IO.File]::WriteAllLines($env:FAKE_ARGUMENT_PROBE_OUTPUT, [string[]]$args)
exit 0
'@, [System.Text.UTF8Encoding]::new($false))
  $probeArguments = @(
    '--defaults-extra-file=C:\Users\Administrator\AppData\Local\Temp\admin-mysql-probe.cnf',
    '--execute=SOURCE /tmp/recovery.sql',
    'argument with spaces'
  )
  $probePayload = [ordered]@{
    script    = $argumentProbeScript
    arguments = $probeArguments
  } | ConvertTo-Json -Compress -Depth 3
  $probePayloadBase64 = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($probePayload))
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
  $probeStartInfo = [System.Diagnostics.ProcessStartInfo]::new()
  $probeStartInfo.FileName = [Environment]::ProcessPath
  $probeStartInfo.UseShellExecute = $false
  $probeStartInfo.CreateNoWindow = $true
  [void]$probeStartInfo.ArgumentList.Add('-NoProfile')
  [void]$probeStartInfo.ArgumentList.Add('-NonInteractive')
  [void]$probeStartInfo.ArgumentList.Add('-EncodedCommand')
  [void]$probeStartInfo.ArgumentList.Add($encodedLauncher)
  $probeStartInfo.Environment['ADMIN_RECOVERY_PROCESS_PAYLOAD'] = $probePayloadBase64
  $probeStartInfo.Environment['FAKE_ARGUMENT_PROBE_OUTPUT'] = $argumentProbeOutput
  $probeProcess = [System.Diagnostics.Process]::Start($probeStartInfo)
  Assert-True ($probeProcess.WaitForExit(10000)) 'argument transport POC timed out'
  Assert-True ($probeProcess.ExitCode -eq 0) "argument transport POC exit=$($probeProcess.ExitCode)"
  $receivedProbeArguments = @(Get-Content -LiteralPath $argumentProbeOutput)
  Assert-True ($receivedProbeArguments.Count -eq $probeArguments.Count) 'argument transport POC changed argument count'
  for ($probeIndex = 0; $probeIndex -lt $probeArguments.Count; $probeIndex++) {
    Assert-True ($receivedProbeArguments[$probeIndex] -ceq $probeArguments[$probeIndex]) "argument transport POC changed argument $probeIndex"
  }
  if ($ArgumentTransportPocOnly) {
    Write-Output 'argument transport POC: PASS'
    return
  }

  $fakeDump = Join-Path $fakeBin 'mysqldump.ps1'
  $fakeMySQL = Join-Path $fakeBin 'mysql.ps1'
  $fakeDocker = Join-Path $fakeBin 'docker.ps1'
  [System.IO.File]::WriteAllText($fakeDump, @'
$ErrorActionPreference = 'Stop'
[System.IO.File]::AppendAllText($env:FAKE_MYSQL_LOG, "mysqldump`t" + [string]::Join("`t", $args) + "`n")
$optionArgument = @($args | Where-Object { $_ -like '--defaults-extra-file=*' } | Select-Object -First 1)
if ($optionArgument.Count -ne 1) {
  [Environment]::Exit(6)
}
$optionPath = $optionArgument[0].Substring('--defaults-extra-file='.Length)
if (-not (Test-Path -LiteralPath $optionPath -PathType Leaf)) {
  [Environment]::Exit(6)
}
$acl = Get-Acl -LiteralPath $optionPath
$currentSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
if (-not $acl.AreAccessRulesProtected) {
  [Environment]::Exit(6)
}
try {
  $ownerSid = ([System.Security.Principal.NTAccount]::new($acl.Owner)).Translate([System.Security.Principal.SecurityIdentifier])
} catch {
  try {
    $ownerSid = [System.Security.Principal.SecurityIdentifier]::new($acl.Owner)
  } catch {
    [Environment]::Exit(6)
  }
}
$allowRules = @($acl.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier]) |
  Where-Object { $_.AccessControlType -eq [System.Security.AccessControl.AccessControlType]::Allow })
if ($ownerSid.Value -ne $currentSid.Value -or $allowRules.Count -eq 0) {
  [Environment]::Exit(6)
}
foreach ($rule in $allowRules) {
  if ($rule.IsInherited -or $rule.IdentityReference.Value -ne $currentSid.Value) {
    [Environment]::Exit(6)
  }
}
$resultArgument = @($args | Where-Object { $_ -like '--result-file=*' } | Select-Object -First 1)
if ($resultArgument.Count -ne 1) {
  [Environment]::Exit(2)
}
$resultPath = $resultArgument[0].Substring('--result-file='.Length)
$dump = @"
-- MySQL dump
-- Host: fake    Database: admin
CREATE TABLE ``users`` (``id`` bigint NOT NULL);
CREATE TABLE ``wallet_transactions`` (``id`` bigint NOT NULL);
"@
[System.IO.File]::WriteAllText($resultPath, $dump, [System.Text.UTF8Encoding]::new($false))
if ($env:FAKE_CNF_CLEANUP_FAILURE -eq '1') {
  Remove-Item -LiteralPath $optionPath -Force
  New-Item -ItemType Directory -Path $optionPath | Out-Null
  [System.IO.File]::WriteAllText((Join-Path $optionPath 'cleanup-blocker'), 'test-only')
}
exit 0
'@, [System.Text.UTF8Encoding]::new($false))

  [System.IO.File]::WriteAllText($fakeMySQL, @'
$ErrorActionPreference = 'Stop'
[System.IO.File]::AppendAllText($env:FAKE_MYSQL_LOG, "mysql`t" + [string]::Join("`t", $args) + "`n")
$optionArgument = @($args | Where-Object { $_ -like '--defaults-extra-file=*' } | Select-Object -First 1)
if ($optionArgument.Count -ne 1) {
  [Environment]::Exit(3)
}
$optionPath = $optionArgument[0].Substring('--defaults-extra-file='.Length)
$optionFile = [System.IO.File]::ReadAllText($optionPath)
$acl = Get-Acl -LiteralPath $optionPath
$currentSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
if (-not $acl.AreAccessRulesProtected) {
  [Environment]::Exit(5)
}
try {
  $ownerSid = ([System.Security.Principal.NTAccount]::new($acl.Owner)).Translate([System.Security.Principal.SecurityIdentifier])
} catch {
  try {
    $ownerSid = [System.Security.Principal.SecurityIdentifier]::new($acl.Owner)
  } catch {
    [Environment]::Exit(5)
  }
}
if ($ownerSid.Value -ne $currentSid.Value) {
  [Environment]::Exit(5)
}
$accessRules = @($acl.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier]))
$allowRules = @($accessRules | Where-Object { $_.AccessControlType -eq [System.Security.AccessControl.AccessControlType]::Allow })
if ($allowRules.Count -eq 0) {
  [Environment]::Exit(5)
}
foreach ($rule in $allowRules) {
  if ($rule.IsInherited -or $rule.IdentityReference.Value -ne $currentSid.Value) {
    [Environment]::Exit(5)
  }
}
$requiredLines = @(
  'host="127.0.0.1"',
  'port=3306',
  'user="fake_admin"',
  'password="fake-super-secret"'
)
foreach ($requiredLine in $requiredLines) {
  if (($optionFile -split "`r?`n") -notcontains $requiredLine) {
    [Environment]::Exit(3)
  }
}
$executeArgument = @($args | Where-Object { $_ -like '--execute=*' } | Select-Object -First 1)
if ($executeArgument.Count -eq 1 -and $executeArgument[0] -match 'COUNT\(\*\)') {
  @(
    "users`t3",
    "wallet_transactions`t5",
    "user_sessions`t7",
    "export_tasks`t11",
    "ai_runs`t13",
    "notifications`t17"
  ) | Write-Output
}
exit 0
'@, [System.Text.UTF8Encoding]::new($false))

  [System.IO.File]::WriteAllText($fakeDocker, @'
$ErrorActionPreference = 'Stop'
[System.IO.File]::AppendAllText($env:FAKE_MYSQL_LOG, "docker`t" + [string]::Join("`t", $args) + "`n")
$stateRoot = $env:FAKE_DOCKER_STATE_DIR
if ($args.Count -eq 0 -or [string]::IsNullOrWhiteSpace($stateRoot)) {
  [Environment]::Exit(4)
}
$containerPath = Join-Path $stateRoot 'container.txt'
$databasePath = Join-Path $stateRoot 'database.txt'
$containerIDPath = Join-Path $stateRoot 'container-id.txt'
$labelTokenPath = Join-Path $stateRoot 'label-token.txt'
$readyPath = Join-Path $stateRoot 'ready'
$copiedPath = Join-Path $stateRoot 'copied'
$sourcedPath = Join-Path $stateRoot 'sourced'

function Read-StateValue {
  param([string]$Path)
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    [Environment]::Exit(70)
  }
  return [System.IO.File]::ReadAllText($Path)
}

function Assert-Container {
  param([string]$Name)
  $storedName = Read-StateValue -Path $containerPath
  $storedID = if (Test-Path -LiteralPath $containerIDPath -PathType Leaf) { Read-StateValue -Path $containerIDPath } else { '' }
  if ($Name -ne $storedName -and $Name -ne $storedID) {
    [Environment]::Exit(71)
  }
}

switch ($args[0]) {
  'run' {
    if ($env:FAKE_DOCKER_NAME_CONFLICT -eq '1') {
      [Environment]::Exit(81)
    }
    $nameIndex = [array]::IndexOf([object[]]$args, '--name')
    if ($nameIndex -lt 0 -or $nameIndex + 1 -ge $args.Count) {
      [Environment]::Exit(72)
    }
    $containerName = $args[$nameIndex + 1]
    if ($containerName -notmatch '^admin-recovery-[0-9a-f]{12}$' -or (Test-Path -LiteralPath $stateRoot)) {
      [Environment]::Exit(72)
    }
    New-Item -ItemType Directory -Path $stateRoot | Out-Null
    [System.IO.File]::WriteAllText($containerPath, $containerName)
    $labelIndex = [array]::IndexOf([object[]]$args, '--label')
    $labelToken = ''
    if ($labelIndex -ge 0 -and $labelIndex + 1 -lt $args.Count -and
      $args[$labelIndex + 1] -match '^admin\.recovery\.token=([0-9a-f]{12})$') {
      $labelToken = $Matches[1]
    }
    $containerID = 'a' * 64
    [System.IO.File]::WriteAllText($containerIDPath, $containerID)
    [System.IO.File]::WriteAllText($labelTokenPath, $labelToken)
    Write-Output $containerID
    if ($env:FAKE_DOCKER_HANG_RUN_AFTER_CREATE -eq '1') {
      Start-Sleep -Seconds 30
    }
    exit 0
  }
  'ps' {
    if ($args.Count -ne 5 -or $args[1] -ne '--all' -or $args[2] -ne '--quiet' -or
      $args[3] -ne '--filter' -or $args[4] -notmatch '^label=admin\.recovery\.token=([0-9a-f]{12})$') {
      [Environment]::Exit(83)
    }
    if (Test-Path -LiteralPath $labelTokenPath -PathType Leaf) {
      $requestedToken = $Matches[1]
      if ((Read-StateValue -Path $labelTokenPath) -eq $requestedToken) {
        Write-Output (Read-StateValue -Path $containerIDPath)
      }
    }
    exit 0
  }
  'inspect' {
    if ($args.Count -ne 4 -or $args[1] -ne '--format') {
      [Environment]::Exit(82)
    }
    Assert-Container -Name $args[3]
    if ($env:FAKE_DOCKER_TAMPER_DUMP -eq '1') {
      $hostDumpPath = Read-StateValue -Path $copiedPath
      [System.IO.File]::AppendAllText($hostDumpPath, "`n-- tampered during cleanup`n")
    }
    if ($env:FAKE_DOCKER_ARTIFACT_COLLISION -eq '1') {
      $hostDumpPath = Read-StateValue -Path $copiedPath
      $artifactDirectory = Split-Path -Parent $hostDumpPath
      $artifactCollisionPath = Join-Path $artifactDirectory 'artifact.json'
      $temporaryCollisionPath = Join-Path $artifactDirectory 'artifact.json.tmp'
      New-Item -ItemType Directory -Path $artifactCollisionPath | Out-Null
      New-Item -ItemType Directory -Path $temporaryCollisionPath | Out-Null
      [System.IO.File]::WriteAllText((Join-Path $temporaryCollisionPath 'cleanup-blocker'), 'test-only')
    }
    if ($env:FAKE_DOCKER_INSPECT_MISMATCH -eq '1') {
      Write-Output (('b' * 64) + '|ffffffffffff|/admin-recovery-ffffffffffff')
      exit 0
    }
    Write-Output ((Read-StateValue -Path $containerIDPath) + '|' + (Read-StateValue -Path $labelTokenPath) + '|/' + (Read-StateValue -Path $containerPath))
    exit 0
  }
  'exec' {
    if ($args.Count -lt 3) {
      [Environment]::Exit(73)
    }
    Assert-Container -Name $args[1]
    if ($args[2] -eq 'sh' -and $args.Count -eq 5 -and $args[3] -eq '-c' -and $args[4] -match 'mysqladmin') {
      [System.IO.File]::WriteAllText($readyPath, 'ready')
      exit 0
    }
    $executeArgument = @($args | Where-Object { $_ -like '--execute=*' } | Select-Object -First 1)
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -match '^--execute=CREATE DATABASE `admin_restore_([0-9a-f]{12})`') {
      if (-not (Test-Path -LiteralPath $readyPath -PathType Leaf) -or $args[1] -ne ('admin-recovery-' + $Matches[1])) {
        [Environment]::Exit(74)
      }
      [System.IO.File]::WriteAllText($databasePath, ('admin_restore_' + $Matches[1]))
      exit 0
    }
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -eq '--execute=SOURCE /tmp/recovery.sql' -and $env:FAKE_DOCKER_HANG_RESTORE -eq '1') {
      if (-not (Test-Path -LiteralPath $copiedPath -PathType Leaf)) {
        [Environment]::Exit(75)
      }
      Start-Process -FilePath ([Environment]::ProcessPath) `
        -ArgumentList @('-NoProfile', '-NonInteractive', '-File', $env:FAKE_HANG_CHILD_SCRIPT) `
        -WindowStyle Hidden | Out-Null
      Start-Sleep -Seconds 30
      [Environment]::Exit(44)
    }
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -eq '--execute=SOURCE /tmp/recovery.sql' -and $env:FAKE_DOCKER_FAIL_RESTORE -eq '1') {
      if (-not (Test-Path -LiteralPath $copiedPath -PathType Leaf)) {
        [Environment]::Exit(75)
      }
      [Environment]::Exit(41)
    }
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -match '^--execute=DROP DATABASE IF EXISTS ' -and $env:FAKE_DOCKER_FAIL_DROP -eq '1') {
      [void](Read-StateValue -Path $databasePath)
      [Environment]::Exit(42)
    }
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -eq '--execute=SOURCE /tmp/recovery.sql') {
      $databaseName = Read-StateValue -Path $databasePath
      if (-not (Test-Path -LiteralPath $copiedPath -PathType Leaf) -or $args -notcontains "--database=$databaseName") {
        [Environment]::Exit(75)
      }
      [System.IO.File]::WriteAllText($sourcedPath, 'sourced')
      exit 0
    }
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -match 'COUNT\(\*\)') {
      if (-not (Test-Path -LiteralPath $sourcedPath -PathType Leaf)) {
        [Environment]::Exit(76)
      }
      @(
        "users`t3",
        "wallet_transactions`t5",
        "user_sessions`t7",
        "export_tasks`t11",
        "ai_runs`t13",
        "notifications`t17"
      ) | Write-Output
      exit 0
    }
    if ($executeArgument.Count -eq 1 -and $executeArgument[0] -match '^--execute=DROP DATABASE IF EXISTS `([^`]+)`$') {
      if ((Read-StateValue -Path $databasePath) -ne $Matches[1]) {
        [Environment]::Exit(77)
      }
      [System.IO.File]::WriteAllText((Join-Path $stateRoot 'dropped'), 'dropped')
      exit 0
    }
    [Environment]::Exit(78)
  }
  'cp' {
    if ($args.Count -ne 3) {
      [Environment]::Exit(79)
    }
    Assert-Container -Name (($args[2] -split ':', 2)[0])
    [void](Read-StateValue -Path $databasePath)
    if (-not (Test-Path -LiteralPath $args[1] -PathType Leaf) -or
      (Get-Item -LiteralPath $args[1]).Length -le 0 -or
      $args[2] -ne ((Read-StateValue -Path $containerPath) + ':/tmp/recovery.sql')) {
      [Environment]::Exit(79)
    }
    [System.IO.File]::WriteAllText($copiedPath, $args[1])
    exit 0
  }
  'rm' {
    if ($args.Count -ne 4 -or $args[1] -ne '--force' -or $args[2] -ne '--volumes') {
      [Environment]::Exit(80)
    }
    Assert-Container -Name $args[3]
    if ($env:FAKE_DOCKER_FAIL_RM -eq '1') {
      [Environment]::Exit(43)
    }
    [System.IO.File]::WriteAllText((Join-Path $stateRoot 'removed'), 'removed')
    exit 0
  }
  default { exit 4 }
}
'@, [System.Text.UTF8Encoding]::new($false))

  [System.IO.File]::WriteAllText($timeoutChildScript, @'
$ErrorActionPreference = 'Stop'
Start-Sleep -Milliseconds 1500
[System.IO.File]::WriteAllText($env:FAKE_HANG_CHILD_MARKER, 'orphaned child survived')
'@, [System.Text.UTF8Encoding]::new($false))

  $fakeGuardLog = Join-Path $testRoot 'fake-guard.log'
  $fakeGuardState = Join-Path $testRoot 'fake-guard-state'
  $env:FAKE_MYSQL_LOG = $fakeGuardLog
  $env:FAKE_DOCKER_STATE_DIR = $fakeGuardState
  $outOfOrderExit = Invoke-TestPowerShellScript -ScriptPath $fakeDocker -Arguments @(
    'cp',
    (Join-Path $testRoot 'missing.sql'),
    'admin-recovery-000000000000:/tmp/recovery.sql'
  )
  Assert-True ($outOfOrderExit -ne 0) 'fake docker accepted cp before run/create'
  $unguardedDumpPath = Join-Path $testRoot 'unguarded.sql'
  $unguardedDumpExit = Invoke-TestPowerShellScript -ScriptPath $fakeDump -Arguments @(
    ('--defaults-extra-file=' + (Join-Path $testRoot 'missing.cnf')),
    ('--result-file=' + $unguardedDumpPath),
    'admin'
  )
  Assert-True ($unguardedDumpExit -ne 0) 'fake mysqldump accepted a missing option file'
  Remove-Item Env:FAKE_DOCKER_STATE_DIR -ErrorAction SilentlyContinue

  $env:ADMIN_DB_HOST = '127.0.0.1'
  $env:ADMIN_DB_PORT = '3306'
  $env:ADMIN_DB_USER = 'fake_admin'
  $env:ADMIN_DB_PASSWORD = $password
  $env:FAKE_MYSQL_LOG = $clientLog
  $env:FAKE_DOCKER_STATE_DIR = $successDockerState

  $outputLines = @(& $recoveryScript `
    -Database admin `
    -BackupRoot $backupRoot `
    -MySQLDumpCommand $fakeDump `
    -MySQLCommand $fakeMySQL `
    -DockerCommand $fakeDocker 2>&1)

  $artifacts = @(Get-ChildItem -LiteralPath $backupRoot -Recurse -Filter artifact.json -File)
  Assert-True ($artifacts.Count -eq 1) "artifact count=$($artifacts.Count)"
  $artifact = Get-Content -Raw -Encoding utf8 $artifacts[0].FullName | ConvertFrom-Json

  Assert-True ($outputLines.Count -eq 2) "success output line count=$($outputLines.Count)"
  Assert-True ([string]$outputLines[0] -eq $artifacts[0].FullName) 'first success output line was not the artifact path'
  Assert-True ([string]$outputLines[1] -match '^[0-9a-f]{64}$') 'second success output line was not a lowercase SHA-256'

  Assert-True ($artifact.verified -eq $true) 'artifact was not verified'
  Assert-True ($artifact.dump_sha256 -match '^[0-9a-f]{64}$') "invalid SHA-256: $($artifact.dump_sha256)"
  Assert-True ($artifact.restore_database -match '^admin_restore_[0-9a-f]{12}$') "invalid restore database: $($artifact.restore_database)"
  Assert-True (Test-Path -LiteralPath $artifact.dump_path -PathType Leaf) 'dump file is missing'
  Assert-True ((Get-Item -LiteralPath $artifact.dump_path).Length -gt 0) 'dump file is empty'
  Assert-True ([string]$outputLines[1] -ceq [string]$artifact.dump_sha256) 'output SHA did not match artifact SHA'
  Assert-True ((Get-FileHash -LiteralPath $artifact.dump_path -Algorithm SHA256).Hash.ToLowerInvariant() -ceq [string]$artifact.dump_sha256) 'artifact SHA did not match the current dump'
  Assert-True ([long]$artifact.dump_bytes -eq (Get-Item -LiteralPath $artifact.dump_path).Length) 'artifact dump_bytes did not match the current dump'

  foreach ($table in @('users', 'wallet_transactions', 'user_sessions', 'export_tasks', 'ai_runs', 'notifications')) {
    Assert-True ($artifact.source_counts.$table -eq $artifact.restore_counts.$table) "count mismatch for $table"
  }

  $log = Get-Content -Raw -Encoding utf8 $clientLog
  $dumpLines = @($log -split "`r?`n" | Where-Object { $_ -like "mysqldump`t*" })
  Assert-True ($dumpLines.Count -eq 1) "mysqldump invocation count=$($dumpLines.Count)"
  $dumpArguments = @(($dumpLines[0] -split "`t") | Select-Object -Skip 1)
  foreach ($requiredArgument in @(
      '--single-transaction',
      '--quick',
      '--routines',
      '--triggers',
      '--events',
      '--default-character-set=utf8mb4',
      '--no-tablespaces'
    )) {
    Assert-True ($dumpArguments -contains $requiredArgument) "mysqldump omitted $requiredArgument"
  }
  Assert-True (@($dumpArguments | Where-Object { $_ -like '--result-file=*' }).Count -eq 1) 'mysqldump did not receive exactly one --result-file'

  $secretMatches = [regex]::Matches($log, '--defaults-extra-file=([^\t\r\n]+)')
  Assert-True ($secretMatches.Count -gt 0) 'fake clients did not receive an option file'
  foreach ($match in $secretMatches) {
    Assert-True (-not (Test-Path -LiteralPath $match.Groups[1].Value)) 'temporary MySQL option file remains'
  }
  Assert-True ($log -match 'DROP DATABASE IF EXISTS `admin_restore_[0-9a-f]{12}`') 'restore database was not dropped'
  Assert-True ($log -match 'docker\trun\t.*--network\tnone') 'restore container was not network-isolated'
  Assert-True ($log -match 'mysql:8\.4\.10') 'restore image was not version pinned'
  Assert-True ($log -match 'docker\trm\t--force\t--volumes\t[0-9a-f]{64}') 'restore container and anonymous volumes were not removed'
  Assert-True ($log -match 'docker\tcp\t[^\t\r\n]+\tadmin-recovery-[0-9a-f]{12}:/tmp/recovery\.sql') 'dump was not copied to the fixed container path'
  Assert-True ($log -match 'docker\texec\tadmin-recovery-[0-9a-f]{12}\tmysql\t.*--execute=SOURCE /tmp/recovery\.sql') 'container did not SOURCE the copied dump'
  $readinessLines = @($log -split "`r?`n" | Where-Object { $_ -match '^docker\texec\tadmin-recovery-[0-9a-f]{12}\tsh\t-c\t.*mysqladmin' })
  Assert-True ($readinessLines.Count -eq 1) "container readiness used $($readinessLines.Count) in-container loops"
  Assert-True ($readinessLines[0] -match '/proc/1/comm') 'container readiness accepted the entrypoint temporary server'

  $runLines = @($log -split "`r?`n" | Where-Object { $_ -like "docker`trun`t*" })
  Assert-True ($runLines.Count -eq 1) "docker run invocation count=$($runLines.Count)"
  $runArguments = @(($runLines[0] -split "`t") | Select-Object -Skip 2)
  $forbiddenRunArguments = @($runArguments | Where-Object {
      $_ -in @('-p', '--publish', '-v', '--volume', '--mount') -or
      $_ -match '^(--publish|--volume|--mount)=' -or
      $_ -match '^-[pv].+'
    })
  Assert-True ($forbiddenRunArguments.Count -eq 0) 'docker run published a port or mounted a host path'
  Assert-True ($runLines[0] -match "--name`tadmin-recovery-([0-9a-f]{12})(?:`t|$)") 'docker run omitted the random container name'
  $restoreToken = $Matches[1]
  Assert-True ($runLines[0] -match "--label`tadmin\.recovery\.token=$restoreToken(?:`t|$)") 'docker run omitted the ownership label'
  Assert-True ($log -match "CREATE DATABASE ``admin_restore_$restoreToken``") 'container and restore database tokens differed'
  Assert-True ($log -match 'docker\tinspect\t--format\t[^\r\n]+\t[0-9a-f]{64}') 'container identity was not inspected before cleanup'

  $sourceMySQLLog = (@($log -split "`r?`n" | Where-Object { $_ -like "mysql`t*" }) -join "`n")
  Assert-True ($sourceMySQLLog -notmatch '(?i)\b(?:CREATE|DROP)\s+DATABASE\b') 'source mysql client received database DDL'

  $combined = ($outputLines -join "`n") + "`n" + $log + "`n" + (Get-Content -Raw -Encoding utf8 $artifacts[0].FullName)
  Assert-True (-not $combined.Contains($password)) 'password leaked into output or evidence'

  $env:FAKE_MYSQL_LOG = $failureClientLog
  $env:FAKE_DOCKER_STATE_DIR = $failureDockerState
  $env:FAKE_DOCKER_FAIL_RESTORE = '1'
  $env:FAKE_DOCKER_FAIL_DROP = '1'
  $env:FAKE_DOCKER_FAIL_RM = '1'
  $env:FAKE_CNF_CLEANUP_FAILURE = '1'
  $failureMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $failureBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 | Out-Null
  } catch {
    $failureMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_FAIL_RESTORE -ErrorAction SilentlyContinue
    Remove-Item Env:FAKE_DOCKER_FAIL_DROP -ErrorAction SilentlyContinue
    Remove-Item Env:FAKE_DOCKER_FAIL_RM -ErrorAction SilentlyContinue
    Remove-Item Env:FAKE_CNF_CLEANUP_FAILURE -ErrorAction SilentlyContinue
  }
  $expectedFailureMessage = 'restore recovery dump failed with exit code 41; cleanup also failed: restore database cleanup failed; restore container cleanup failed; temporary MySQL option file cleanup failed'
  Assert-True ($failureMessage -eq $expectedFailureMessage) "restore and cleanup failures were not aggregated: $failureMessage"
  $failureLog = Get-Content -Raw -Encoding utf8 $failureClientLog
  Assert-True ($failureLog -match 'DROP DATABASE IF EXISTS `admin_restore_[0-9a-f]{12}`') 'restore failure did not attempt to drop the restore database'
  Assert-True ($failureLog -match 'docker\trm\t--force\t--volumes\t[0-9a-f]{64}') 'restore failure did not remove the container and anonymous volumes'
  $failureSecretMatches = [regex]::Matches($failureLog, '--defaults-extra-file=([^\t\r\n]+)')
  Assert-True ($failureSecretMatches.Count -gt 0) 'restore failure did not use an option file'
  foreach ($match in $failureSecretMatches) {
    $injectedPath = $match.Groups[1].Value
    if (Test-Path -LiteralPath $injectedPath) {
      $resolvedInjectedPath = [System.IO.Path]::GetFullPath($injectedPath)
      if ((Split-Path -Leaf $resolvedInjectedPath) -notmatch '^admin-mysql-[0-9a-f]{32}\.cnf$' -or
        -not $resolvedInjectedPath.StartsWith([System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()), [System.StringComparison]::OrdinalIgnoreCase)) {
        throw 'refusing to remove an unexpected injected option path'
      }
      Remove-Item -LiteralPath $resolvedInjectedPath -Recurse -Force
    }
    Assert-True (-not (Test-Path -LiteralPath $injectedPath)) 'test cleanup left an injected option path'
  }
  Assert-True (@(Get-ChildItem -LiteralPath $failureBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'restore failure left artifact.json'
  Assert-True (-not ($failureMessage + "`n" + $failureLog).Contains($password)) 'restore failure leaked the password'

  $env:FAKE_MYSQL_LOG = $cleanupFailureClientLog
  $env:FAKE_DOCKER_STATE_DIR = $cleanupFailureDockerState
  $env:FAKE_DOCKER_FAIL_RM = '1'
  $cleanupFailureOutput = [System.Collections.Generic.List[string]]::new()
  $cleanupFailureMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $cleanupFailureBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 |
      ForEach-Object { [void]$cleanupFailureOutput.Add($_.ToString()) }
  } catch {
    $cleanupFailureMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_FAIL_RM -ErrorAction SilentlyContinue
  }
  Assert-True ($cleanupFailureMessage -eq 'cleanup failed: restore container cleanup failed') "unexpected cleanup failure: $cleanupFailureMessage"
  Assert-True ($cleanupFailureOutput.Count -eq 0) "cleanup failure emitted $($cleanupFailureOutput.Count) success lines"
  Assert-True (@(Get-ChildItem -LiteralPath $cleanupFailureBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'cleanup failure left artifact.json'
  $cleanupFailureLog = Get-Content -Raw -Encoding utf8 $cleanupFailureClientLog
  Assert-True ($cleanupFailureLog -match 'DROP DATABASE IF EXISTS `admin_restore_[0-9a-f]{12}`') 'cleanup failure did not drop the restore database first'
  Assert-True ($cleanupFailureLog -match 'docker\trm\t--force\t--volumes\t[0-9a-f]{64}') 'cleanup failure did not request container and volume removal'
  $cleanupSecretMatches = [regex]::Matches($cleanupFailureLog, '--defaults-extra-file=([^\t\r\n]+)')
  foreach ($match in $cleanupSecretMatches) {
    Assert-True (-not (Test-Path -LiteralPath $match.Groups[1].Value)) 'cleanup failure left a temporary MySQL option file'
  }
  Assert-True (-not (($cleanupFailureOutput -join "`n") + "`n" + $cleanupFailureMessage + "`n" + $cleanupFailureLog).Contains($password)) 'cleanup failure leaked the password'

  $env:FAKE_MYSQL_LOG = $timeoutClientLog
  $env:FAKE_DOCKER_STATE_DIR = $timeoutDockerState
  $env:FAKE_DOCKER_HANG_RESTORE = '1'
  $env:FAKE_HANG_CHILD_SCRIPT = $timeoutChildScript
  $env:FAKE_HANG_CHILD_MARKER = $timeoutChildMarker
  $timeoutMessage = $null
  $timeoutStopwatch = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $timeoutBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker `
      -CommandTimeoutSeconds 1 `
      -ReadinessTimeoutSeconds 1 2>&1 | Out-Null
  } catch {
    $timeoutMessage = $_.Exception.Message
  } finally {
    $timeoutStopwatch.Stop()
    Remove-Item Env:FAKE_DOCKER_HANG_RESTORE -ErrorAction SilentlyContinue
    Remove-Item Env:FAKE_HANG_CHILD_SCRIPT -ErrorAction SilentlyContinue
    Remove-Item Env:FAKE_HANG_CHILD_MARKER -ErrorAction SilentlyContinue
  }
  Assert-True ($timeoutMessage -eq 'restore recovery dump timed out after 1 seconds') "unexpected timeout failure: $timeoutMessage"
  Assert-True ($timeoutStopwatch.Elapsed.TotalSeconds -lt 8) "timeout took $($timeoutStopwatch.Elapsed.TotalSeconds) seconds"
  Start-Sleep -Milliseconds 800
  Assert-True (-not (Test-Path -LiteralPath $timeoutChildMarker)) 'timed-out process tree left a live child process'
  $timeoutLog = Get-Content -Raw -Encoding utf8 $timeoutClientLog
  Assert-True ($timeoutLog -match 'DROP DATABASE IF EXISTS `admin_restore_[0-9a-f]{12}`') 'timeout did not attempt database cleanup'
  Assert-True ($timeoutLog -match 'docker\trm\t--force\t--volumes\t[0-9a-f]{64}') 'timeout did not remove the container and volumes'
  $timeoutSecretMatches = [regex]::Matches($timeoutLog, '--defaults-extra-file=([^\t\r\n]+)')
  foreach ($match in $timeoutSecretMatches) {
    Assert-True (-not (Test-Path -LiteralPath $match.Groups[1].Value)) 'timeout left a temporary MySQL option file'
  }
  Assert-True (@(Get-ChildItem -LiteralPath $timeoutBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'timeout left artifact.json'

  $env:FAKE_MYSQL_LOG = $runTimeoutClientLog
  $env:FAKE_DOCKER_STATE_DIR = $runTimeoutDockerState
  $env:FAKE_DOCKER_HANG_RUN_AFTER_CREATE = '1'
  $runTimeoutMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $runTimeoutBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker `
      -CommandTimeoutSeconds 1 `
      -ReadinessTimeoutSeconds 1 2>&1 | Out-Null
  } catch {
    $runTimeoutMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_HANG_RUN_AFTER_CREATE -ErrorAction SilentlyContinue
  }
  Assert-True ($runTimeoutMessage -eq 'start restore container timed out after 1 seconds') "unexpected run-timeout failure: $runTimeoutMessage"
  $runTimeoutLog = Get-Content -Raw -Encoding utf8 $runTimeoutClientLog
  Assert-True ($runTimeoutLog -match '(?m)^docker\tps\t') 'run timeout did not discover an owned partial container'
  Assert-True ($runTimeoutLog -match '(?m)^docker\tinspect\t') 'run timeout did not inspect the discovered container'
  Assert-True ($runTimeoutLog -match '(?m)^docker\trm\t--force\t--volumes\t') 'run timeout did not remove the discovered container and volumes'
  Assert-True (@(Get-ChildItem -LiteralPath $runTimeoutBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'run timeout left artifact.json'

  $env:FAKE_MYSQL_LOG = $nameConflictClientLog
  $env:FAKE_DOCKER_STATE_DIR = $nameConflictDockerState
  $env:FAKE_DOCKER_NAME_CONFLICT = '1'
  $nameConflictMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $nameConflictBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 | Out-Null
  } catch {
    $nameConflictMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_NAME_CONFLICT -ErrorAction SilentlyContinue
  }
  Assert-True ($nameConflictMessage -eq 'start restore container failed with exit code 81') "unexpected name-conflict error: $nameConflictMessage"
  $nameConflictLog = Get-Content -Raw -Encoding utf8 $nameConflictClientLog
  Assert-True ($nameConflictLog -match '(?m)^docker\trun\t') 'name-conflict scenario did not call docker run'
  Assert-True ($nameConflictLog -notmatch '(?m)^docker\t(?:inspect|rm)\t') 'name-conflict scenario touched the pre-existing container'
  Assert-True (@(Get-ChildItem -LiteralPath $nameConflictBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'name-conflict scenario left artifact.json'

  $env:FAKE_MYSQL_LOG = $identityMismatchClientLog
  $env:FAKE_DOCKER_STATE_DIR = $identityMismatchDockerState
  $env:FAKE_DOCKER_INSPECT_MISMATCH = '1'
  $identityMismatchMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $identityMismatchBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 | Out-Null
  } catch {
    $identityMismatchMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_INSPECT_MISMATCH -ErrorAction SilentlyContinue
  }
  Assert-True ($identityMismatchMessage -eq 'cleanup failed: restore container identity verification failed') "unexpected identity mismatch error: $identityMismatchMessage"
  $identityMismatchLog = Get-Content -Raw -Encoding utf8 $identityMismatchClientLog
  Assert-True ($identityMismatchLog -match '(?m)^docker\tinspect\t') 'identity mismatch did not inspect the container'
  Assert-True ($identityMismatchLog -notmatch 'DROP DATABASE IF EXISTS') 'identity mismatch attempted to drop through an unverified container'
  Assert-True ($identityMismatchLog -notmatch '(?m)^docker\trm\t') 'identity mismatch removed an unverified container'
  Assert-True (@(Get-ChildItem -LiteralPath $identityMismatchBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'identity mismatch left artifact.json'

  $env:FAKE_MYSQL_LOG = $tamperClientLog
  $env:FAKE_DOCKER_STATE_DIR = $tamperDockerState
  $env:FAKE_DOCKER_TAMPER_DUMP = '1'
  $tamperOutput = [System.Collections.Generic.List[string]]::new()
  $tamperMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $tamperBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 |
      ForEach-Object { [void]$tamperOutput.Add($_.ToString()) }
  } catch {
    $tamperMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_TAMPER_DUMP -ErrorAction SilentlyContinue
  }
  Assert-True ($tamperMessage -eq 'recovery dump changed during verification') "unexpected tamper result: $tamperMessage"
  Assert-True ($tamperOutput.Count -eq 0) "tamper scenario emitted $($tamperOutput.Count) success lines"
  Assert-True (@(Get-ChildItem -LiteralPath $tamperBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'tamper scenario left artifact.json'
  $tamperLog = Get-Content -Raw -Encoding utf8 $tamperClientLog
  Assert-True ($tamperLog -match '(?m)^docker\tinspect\t') 'tamper scenario did not reach cleanup inspection'
  Assert-True ($tamperLog -match 'DROP DATABASE IF EXISTS') 'tamper scenario did not drop the verified restore database'
  Assert-True ($tamperLog -match '(?m)^docker\trm\t--force\t--volumes\t') 'tamper scenario did not remove the container and volumes'

  $env:FAKE_MYSQL_LOG = $artifactFailureClientLog
  $env:FAKE_DOCKER_STATE_DIR = $artifactFailureDockerState
  $env:FAKE_DOCKER_ARTIFACT_COLLISION = '1'
  $artifactFailureOutput = [System.Collections.Generic.List[string]]::new()
  $artifactFailureMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $artifactFailureBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 |
      ForEach-Object { [void]$artifactFailureOutput.Add($_.ToString()) }
  } catch {
    $artifactFailureMessage = $_.Exception.Message
  } finally {
    Remove-Item Env:FAKE_DOCKER_ARTIFACT_COLLISION -ErrorAction SilentlyContinue
  }
  $expectedArtifactFailure = 'artifact publication failed; cleanup also failed: artifact temporary file cleanup failed'
  Assert-True ($artifactFailureMessage -eq $expectedArtifactFailure) "unexpected artifact publication failure: $artifactFailureMessage"
  Assert-True ($artifactFailureOutput.Count -eq 0) "artifact publication failure emitted $($artifactFailureOutput.Count) success lines"
  Assert-True (@(Get-ChildItem -LiteralPath $artifactFailureBackupRoot -Recurse -Filter artifact.json -File).Count -eq 0) 'artifact publication failure left artifact.json'

  New-Item -ItemType Directory -Path $junctionTarget | Out-Null
  New-Item -ItemType Junction -Path $junctionPath -Target $junctionTarget | Out-Null
  $env:FAKE_MYSQL_LOG = $junctionClientLog
  $junctionFailureMessage = $null
  try {
    & $recoveryScript `
      -Database admin `
      -BackupRoot $junctionBackupRoot `
      -MySQLDumpCommand $fakeDump `
      -MySQLCommand $fakeMySQL `
      -DockerCommand $fakeDocker 2>&1 | Out-Null
  } catch {
    $junctionFailureMessage = $_.Exception.Message
  }
  Assert-True ($junctionFailureMessage -eq 'BackupRoot must be outside the backend and frontend repositories') "junction BackupRoot was not rejected: $junctionFailureMessage"
  Assert-True (-not (Test-Path -LiteralPath $junctionClientLog)) 'junction BackupRoot invoked a database client'
  Assert-True (@(Get-ChildItem -LiteralPath $junctionTarget -Force).Count -eq 0) 'junction BackupRoot created repository content before rejection'

  New-Item -ItemType Directory -Path $uncTarget | Out-Null
  for ($uncIndex = 0; $uncIndex -lt $uncBackupRoots.Count; $uncIndex++) {
    $uncClientLog = Join-Path $testRoot "unc-clients-$uncIndex.log"
    $env:FAKE_MYSQL_LOG = $uncClientLog
    $uncFailureMessage = $null
    try {
      & $recoveryScript `
        -Database admin `
        -BackupRoot $uncBackupRoots[$uncIndex] `
        -MySQLDumpCommand $fakeDump `
        -MySQLCommand $fakeMySQL `
        -DockerCommand $fakeDocker 2>&1 | Out-Null
    } catch {
      $uncFailureMessage = $_.Exception.Message
    }
    Assert-True ($uncFailureMessage -eq 'BackupRoot must use a local filesystem path') "UNC BackupRoot was not clearly rejected: $uncFailureMessage"
    Assert-True (-not (Test-Path -LiteralPath $uncClientLog)) 'UNC BackupRoot invoked a database client'
    Assert-True (@(Get-ChildItem -LiteralPath $uncTarget -Force).Count -eq 0) 'UNC BackupRoot created repository content before rejection'
  }

  $readme = Get-Content -Raw -Encoding utf8 -LiteralPath $databaseReadme
  foreach ($requiredDocumentation in @(
      'The source database is used only for critical row counts and `mysqldump`',
      'fixed `mysql:8.4.10` temporary container',
      '`--network none`',
      'publishes no ports',
      'mounts no host credentials',
      'random `admin_restore_<12hex>` database',
      'anonymous MySQL data volume',
      '`docker rm --force --volumes`'
      'local filesystem path',
      'UNC paths are rejected',
      '`CommandTimeoutSeconds`',
      'atomically created with a current-user-only ACL'
    )) {
    Assert-True ($readme.Contains($requiredDocumentation)) "database README omitted: $requiredDocumentation"
  }

  $recoveryScriptSource = Get-Content -Raw -Encoding utf8 -LiteralPath $recoveryScript
  Assert-True ($recoveryScriptSource.Contains('[System.IO.FileSystemAclExtensions]::Create(')) 'option file was not created with the atomic ACL API'
  Assert-True ($recoveryScriptSource.Contains('[System.IO.FileMode]::CreateNew')) 'option file did not use CreateNew'
  Assert-True ($recoveryScriptSource -notmatch 'WriteAllText\(\$secretFile') 'option file content was written before ACL application'
  Assert-True ($recoveryScriptSource -notmatch '\$LASTEXITCODE') 'external process wrapper still depended on LASTEXITCODE'
  Assert-True ($recoveryScriptSource.Contains('process tree termination failed for PID')) 'timeout did not surface process-tree termination failure'
  Assert-True ($recoveryScriptSource.Contains('Assert-SafeBackupPath -Path $artifactDirectory')) 'artifact directory was not revalidated after creation'
  Assert-True ($recoveryScriptSource.Contains('artifact temporary file cleanup failed')) 'artifact temporary cleanup errors were not detected'
  Assert-True ($recoveryScriptSource -notmatch 'Remove-Item -LiteralPath \$artifactTemporaryPath -Force -ErrorAction SilentlyContinue') 'artifact temporary cleanup still used SilentlyContinue'

  Write-Output 'database recovery fake-client tests: PASS'
} finally {
  foreach ($environmentName in $managedEnvironmentNames) {
    [Environment]::SetEnvironmentVariable($environmentName, $originalEnvironment[$environmentName], 'Process')
  }
  if (Test-Path -LiteralPath $junctionPath) {
    Remove-Item -LiteralPath $junctionPath -Force
  }
  if (Test-Path -LiteralPath $junctionPath) {
    throw 'failed to remove the test junction safely'
  }
  if (Test-Path -LiteralPath $junctionTarget) {
    $resolvedTarget = (Resolve-Path -LiteralPath $junctionTarget).Path
    $resolvedEvidenceRoot = (Resolve-Path -LiteralPath $evidenceRoot).Path
    if ((Split-Path -Parent $resolvedTarget) -ne $resolvedEvidenceRoot -or
      (Split-Path -Leaf $resolvedTarget) -notmatch '^recovery-path-test-[0-9a-f]{32}$') {
      throw 'refusing to remove an unexpected junction target'
    }
    Remove-Item -LiteralPath $resolvedTarget -Recurse -Force
  }
  if (Test-Path -LiteralPath $uncTarget) {
    $resolvedUncTarget = (Resolve-Path -LiteralPath $uncTarget).Path
    $resolvedEvidenceRoot = (Resolve-Path -LiteralPath $evidenceRoot).Path
    if ((Split-Path -Parent $resolvedUncTarget) -ne $resolvedEvidenceRoot -or
      (Split-Path -Leaf $resolvedUncTarget) -notmatch '^recovery-unc-test-[0-9a-f]{32}$') {
      throw 'refusing to remove an unexpected UNC test target'
    }
    Remove-Item -LiteralPath $resolvedUncTarget -Recurse -Force
  }
  if (Test-Path -LiteralPath $testRoot) {
    $resolved = (Resolve-Path -LiteralPath $testRoot).Path
    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
    if (-not $resolved.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
      throw 'refusing to remove unexpected test path'
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
  }
}
