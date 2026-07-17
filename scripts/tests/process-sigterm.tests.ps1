[CmdletBinding()]
param(
  [string]$RuntimeEnv = '',
  [string]$Network = 'admin-platform'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Resolve-DockerExecutable {
  $command = Get-Command docker.exe -ErrorAction SilentlyContinue
  if ($null -ne $command) {
    return $command.Source
  }

  $dockerRoot = Join-Path $env:ProgramFiles 'Docker'
  if (Test-Path -LiteralPath $dockerRoot) {
    $candidate = Get-ChildItem -LiteralPath $dockerRoot -Filter docker.exe -File -Recurse -ErrorAction SilentlyContinue |
      Sort-Object LastWriteTimeUtc -Descending |
      Select-Object -First 1
    if ($null -ne $candidate) {
      return $candidate.FullName
    }
  }
  throw 'docker.exe is required for the real SIGTERM process test'
}

function Invoke-Docker {
  param([string[]]$Arguments)

  $output = & $script:DockerExecutable @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "Docker command failed: $($Arguments[0])"
  }
  return $output
}

function Test-ContainerRunning {
  param([string]$Name)

  $state = & $script:DockerExecutable inspect --format '{{.State.Running}}' $Name 2>$null
  return $LASTEXITCODE -eq 0 -and (($state -join '').Trim() -eq 'true')
}

function Read-ContainerLogs {
  param([string]$Name)

  $previousPreference = $ErrorActionPreference
  try {
    # Docker forwards application stderr as native stderr even when `docker
    # logs` succeeds. PowerShell 5 turns that stream into ErrorRecord values.
    $ErrorActionPreference = 'Continue'
    $captured = & $script:DockerExecutable logs $Name 2>&1
    $exitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $previousPreference
  }
  if ($exitCode -ne 0) {
    throw "$Name logs could not be inspected"
  }
  return @($captured | ForEach-Object { $_.ToString() })
}

function Wait-ForCondition {
  param(
    [scriptblock]$Probe,
    [int]$TimeoutSeconds,
    [string]$FailureMessage
  )

  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
  while ([DateTime]::UtcNow -lt $deadline) {
    if (& $Probe) {
      return
    }
    Start-Sleep -Milliseconds 250
  }
  throw $FailureMessage
}

function Wait-ForAPIReady {
  param([string]$Name)

  Wait-ForCondition -TimeoutSeconds 45 -FailureMessage 'admin-api did not become ready' -Probe {
    if (-not (Test-ContainerRunning -Name $Name)) {
      return $false
    }
    & $script:DockerExecutable exec $Name curl -fsS http://127.0.0.1:8080/ready *> $null
    return $LASTEXITCODE -eq 0
  }
}

function Wait-ForWorkerReady {
  param([string]$Name)

  Wait-ForCondition -TimeoutSeconds 45 -FailureMessage 'admin-worker did not become ready' -Probe {
    if (-not (Test-ContainerRunning -Name $Name)) {
      return $false
    }
	$logs = Read-ContainerLogs -Name $Name
    return (($logs -join "`n") -match 'admin worker (queue|scheduler) started')
  }
}

function Stop-And-AssertGraceful {
  param([string]$Name)

  & $script:DockerExecutable stop --signal SIGTERM --timeout 15 $Name *> $null
  if ($LASTEXITCODE -ne 0) {
    throw "$Name did not stop within 15 seconds"
  }
  $exitCode = (Invoke-Docker -Arguments @('inspect', '--format', '{{.State.ExitCode}}', $Name) | Select-Object -Last 1).Trim()
  if ($exitCode -ne '0') {
    throw "$Name exited nonzero"
  }
	$logs = (Read-ContainerLogs -Name $Name) -join "`n"
  $shutdownCount = [regex]::Matches($logs, 'process stopped').Count
  if ($shutdownCount -ne 1) {
    throw "$Name emitted an invalid shutdown sequence count"
  }
}

$workspace = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
if ([string]::IsNullOrWhiteSpace($RuntimeEnv)) {
  $RuntimeEnv = Join-Path $workspace 'deploy\docker-first\admin-go.env'
}
$RuntimeEnv = [IO.Path]::GetFullPath($RuntimeEnv)
if (-not (Test-Path -LiteralPath $RuntimeEnv -PathType Leaf)) {
  throw 'ignored runtime env is required for the process test'
}

$script:DockerExecutable = Resolve-DockerExecutable
$dockerDirectory = Split-Path -Parent $script:DockerExecutable
if (($env:PATH -split ';') -notcontains $dockerDirectory) {
  $env:PATH = "$dockerDirectory;$env:PATH"
}
$suffix = '{0}-{1}' -f $PID, ([Guid]::NewGuid().ToString('N').Substring(0, 8))
$image = "admin-go-runtime-sigterm:$suffix"
$apiName = "admin-api-sigterm-$suffix"
$workerName = "admin-worker-sigterm-$suffix"
$containers = @($apiName, $workerName)

try {
  Invoke-Docker -Arguments @('network', 'inspect', $Network) | Out-Null
  Invoke-Docker -Arguments @('build', '--quiet', '--target', 'runtime', '--tag', $image, $workspace) | Out-Null

  Invoke-Docker -Arguments @(
    'run', '--detach', '--name', $apiName, '--network', $Network,
    '--env-file', $RuntimeEnv, '--entrypoint', '/app/admin-api', $image
  ) | Out-Null
  Wait-ForAPIReady -Name $apiName
  Stop-And-AssertGraceful -Name $apiName

  Invoke-Docker -Arguments @(
    'run', '--detach', '--name', $workerName, '--network', $Network,
    '--env-file', $RuntimeEnv, '--entrypoint', '/app/admin-worker', $image
  ) | Out-Null
  Wait-ForWorkerReady -Name $workerName
  Stop-And-AssertGraceful -Name $workerName

  Write-Output 'process SIGTERM tests passed'
}
finally {
  foreach ($container in $containers) {
    & $script:DockerExecutable rm --force $container *> $null
  }
  & $script:DockerExecutable image rm --force $image *> $null
}
