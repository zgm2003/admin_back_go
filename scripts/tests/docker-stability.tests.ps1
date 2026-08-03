[CmdletBinding()]
param(
  [string]$Network = '',
  [int]$LateStateProbeSeconds = 5
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '..\admin_front_ts'))
$stateCompose = Join-Path $repoRoot 'deploy\docker-state\docker-compose.yml'
$appCompose = Join-Path $repoRoot 'deploy\docker-first\docker-compose.yml'

function Resolve-DockerExecutable {
  $preferred = 'E:\Docker\Docker\resources\bin\docker.exe'
  if (Test-Path -LiteralPath $preferred -PathType Leaf) {
    return $preferred
  }
  return (Get-Command docker.exe -ErrorAction Stop | Select-Object -First 1).Source
}

function Invoke-Docker {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'Continue'
    $output = @(& $script:DockerExecutable @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $previousPreference
  }
  $lines = @($output | ForEach-Object { $_.ToString() })
  if ($exitCode -ne 0) {
    $tail = ($lines | Select-Object -Last 20) -join "`n"
    throw "docker command failed ($($Arguments -join ' '))`n$tail"
  }
  return $lines
}

function Invoke-AppCompose {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  return Invoke-Docker -Arguments (@(
    'compose', '--project-name', $script:AppProject,
    '-f', $appCompose,
    '-f', $script:AppOverride
  ) + $Arguments)
}

function Invoke-StateCompose {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  return Invoke-Docker -Arguments (@('compose', '-f', $stateCompose) + $Arguments)
}

function Invoke-DockerCleanup {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'SilentlyContinue'
    & $script:DockerExecutable @Arguments *> $null
  }
  catch {
  }
  finally {
    $ErrorActionPreference = $previousPreference
  }
}

function Resolve-GitRevision {
  param([Parameter(Mandatory = $true)][string]$Repository)

  $git = Get-Command git.exe -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $git) {
    $git = Get-Command git -ErrorAction Stop | Select-Object -First 1
  }
  $output = @(& $git.Source -C $Repository rev-parse --verify HEAD 2>&1)
  if ($LASTEXITCODE -ne 0) {
    throw "could not resolve Git revision for $Repository"
  }
  $revision = (($output | Select-Object -Last 1).ToString()).Trim().ToLowerInvariant()
  if ($revision -notmatch '^[0-9a-f]{40}$') {
    throw "invalid Git revision for $Repository"
  }
  return $revision
}

function Get-ComposeContainerID {
  param(
    [Parameter(Mandatory = $true)][ValidateSet('app', 'state')][string]$Project,
    [Parameter(Mandatory = $true)][string]$Service
  )

  if ($Project -eq 'app') {
    $output = Invoke-AppCompose -Arguments @('ps', '--all', '--quiet', $Service)
  }
  else {
    $output = Invoke-StateCompose -Arguments @('ps', '--all', '--quiet', $Service)
  }
  $ids = @($output | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
  if ($ids.Count -ne 1) {
    throw "$Project service $Service has $($ids.Count) containers"
  }
  return $ids[0].Trim()
}

function Get-ContainerInspect {
  param([Parameter(Mandatory = $true)][string]$Container)

  $json = (Invoke-Docker -Arguments @('inspect', $Container)) -join "`n"
  $items = @($json | ConvertFrom-Json)
  if ($items.Count -ne 1) {
    throw "container inspect returned $($items.Count) records for $Container"
  }
  return $items[0]
}

function Get-ContainerNetworkAddress {
  param([Parameter(Mandatory = $true)][string]$Container)

  $inspect = Get-ContainerInspect -Container $Container
  $networkProperty = $inspect.NetworkSettings.Networks.PSObject.Properties[$Network]
  if ($null -eq $networkProperty) {
    throw "$Container is not attached to $Network"
  }
  $address = [string]$networkProperty.Value.IPAddress
  if ([string]::IsNullOrWhiteSpace($address)) {
    throw "$Container has no IPv4 address on $Network"
  }
  return $address
}

function Get-RestartCount {
  param([Parameter(Mandatory = $true)][string]$Container)
  return [int](Get-ContainerInspect -Container $Container).RestartCount
}

function Test-ContainerRunning {
  param([Parameter(Mandatory = $true)][string]$Container)

  $value = & $script:DockerExecutable inspect --format '{{.State.Running}}' $Container 2>$null
  return $LASTEXITCODE -eq 0 -and (($value -join '').Trim() -eq 'true')
}

function Test-ContainerHealthy {
  param([Parameter(Mandatory = $true)][string]$Container)

  $value = & $script:DockerExecutable inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' $Container 2>$null
  return $LASTEXITCODE -eq 0 -and (($value -join '').Trim() -eq 'healthy')
}

function Read-ContainerLogs {
  param([Parameter(Mandatory = $true)][string]$Container)

  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'Continue'
    $captured = @(& $script:DockerExecutable logs $Container 2>&1)
    $exitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $previousPreference
  }
  if ($exitCode -ne 0) {
    throw "$Container logs could not be inspected"
  }
  return @($captured | ForEach-Object { $_.ToString() })
}

function Wait-ForCondition {
  param(
    [Parameter(Mandatory = $true)][scriptblock]$Probe,
    [Parameter(Mandatory = $true)][int]$TimeoutSeconds,
    [Parameter(Mandatory = $true)][string]$FailureMessage
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

function Wait-ForHealthy {
  param(
    [Parameter(Mandatory = $true)][string]$Container,
    [int]$TimeoutSeconds = 180
  )

  Wait-ForCondition -TimeoutSeconds $TimeoutSeconds -FailureMessage "$Container did not become healthy" -Probe {
    Test-ContainerHealthy -Container $Container
  }
}

function Get-WorkerStartCount {
  param([Parameter(Mandatory = $true)][string]$Container)
  $logs = (Read-ContainerLogs -Container $Container) -join "`n"
  return [regex]::Matches($logs, 'admin worker (queue|scheduler) started').Count
}

function Wait-ForWorkerStartCount {
  param(
    [Parameter(Mandatory = $true)][string]$Container,
    [Parameter(Mandatory = $true)][int]$Minimum,
    [int]$TimeoutSeconds = 180
  )

  Wait-ForCondition -TimeoutSeconds $TimeoutSeconds -FailureMessage "$Container did not finish worker startup" -Probe {
    if (-not (Test-ContainerRunning -Container $Container)) {
      return $false
    }
    return (Get-WorkerStartCount -Container $Container) -ge $Minimum
  }
}

function Test-FrontendProxy {
  param([Parameter(Mandatory = $true)][string]$FrontendContainer)

  & $script:DockerExecutable exec $FrontendContainer wget -q -O /dev/null http://127.0.0.1:8080/api/admin/v1/ping 2>$null
  return $LASTEXITCODE -eq 0
}

function Assert-ImageRevision {
  param(
    [Parameter(Mandatory = $true)][string]$Image,
    [Parameter(Mandatory = $true)][string]$Expected
  )

  $json = (Invoke-Docker -Arguments @('image', 'inspect', $Image)) -join "`n"
  $items = @($json | ConvertFrom-Json)
  $actual = [string]$items[0].Config.Labels.'org.opencontainers.image.revision'
  if ($actual -cne $Expected -or $actual -eq 'unknown') {
    throw "$Image revision label=$actual expected=$Expected"
  }
}

function Assert-NoRestarts {
  param([Parameter(Mandatory = $true)][string]$Container)

  $inspect = Get-ContainerInspect -Container $Container
  if (-not [bool]$inspect.State.Running) {
    throw "$Container is not running"
  }
  if ([int]$inspect.RestartCount -ne 0) {
    throw "$Container RestartCount=$($inspect.RestartCount), expected 0"
  }
}

function Stop-And-AssertGraceful {
  param([Parameter(Mandatory = $true)][string]$Container)

  $before = [regex]::Matches(((Read-ContainerLogs -Container $Container) -join "`n"), 'process stopped').Count
  if ($before -ne 0) {
    throw "$Container already contains $before shutdown records"
  }
  Invoke-Docker -Arguments @('stop', '--signal', 'SIGTERM', '--time', '20', $Container) | Out-Null
  $inspect = Get-ContainerInspect -Container $Container
  if ([int]$inspect.State.ExitCode -ne 0) {
    throw "$Container exited with code $($inspect.State.ExitCode)"
  }
  $after = [regex]::Matches(((Read-ContainerLogs -Container $Container) -join "`n"), 'process stopped').Count
  if ($after -ne 1) {
    throw "$Container emitted $after shutdown records, expected 1"
  }
}

$script:DockerExecutable = Resolve-DockerExecutable
$dockerDirectory = Split-Path -Parent $script:DockerExecutable
if (($env:Path -split [IO.Path]::PathSeparator) -notcontains $dockerDirectory) {
  $env:Path = $dockerDirectory + [IO.Path]::PathSeparator + $env:Path
}

$backendRevision = Resolve-GitRevision -Repository $repoRoot
$frontendRevision = Resolve-GitRevision -Repository $frontendRoot
$script:AppProject = 'admin-stability-' + $PID
if ([string]::IsNullOrWhiteSpace($Network)) {
  $Network = $script:AppProject + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
}
if ($Network -cnotmatch '^[a-z0-9][a-z0-9_.-]{0,62}$') {
  throw 'stability network name is invalid'
}
$temporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ($script:AppProject + '-' + [guid]::NewGuid().ToString('N'))
$script:AppOverride = Join-Path $temporaryRoot 'docker-compose.override.yml'
$reservationName = 'admin-api-address-reservation-' + $PID
$reservationExists = $false
$networkExists = $false
$stateMySQL = ''
$stateRedis = ''
$lateAPI = ''
$lateWorker = ''

try {
  [IO.Directory]::CreateDirectory($temporaryRoot) | Out-Null
  $overrideLines = @(
    'services:',
    '  frontend:',
    '    ports: !override []',
    '  admin-api:',
    '    ports: !override []',
    'networks:',
    '  platform:',
    "    name: $Network"
  )
  [IO.File]::WriteAllLines($script:AppOverride, $overrideLines, [Text.UTF8Encoding]::new($false))

  Assert-ImageRevision -Image 'admin-go-backend:local' -Expected $backendRevision
  Assert-ImageRevision -Image 'admin-frontend:local' -Expected $frontendRevision

  Invoke-StateCompose -Arguments @('up', '-d', '--wait', '--wait-timeout', '180') | Out-Null
  $stateMySQL = Get-ComposeContainerID -Project state -Service mysql
  $stateRedis = Get-ComposeContainerID -Project state -Service redis
  Wait-ForHealthy -Container $stateMySQL
  Wait-ForHealthy -Container $stateRedis

  Invoke-Docker -Arguments @('network', 'create', $Network) | Out-Null
  $networkExists = $true
  Invoke-Docker -Arguments @('network', 'connect', '--alias', 'mysql', $Network, $stateMySQL) | Out-Null
  Invoke-Docker -Arguments @('network', 'connect', '--alias', 'redis', $Network, $stateRedis) | Out-Null
  Invoke-AppCompose -Arguments @('up', '-d', '--no-build', '--wait', '--wait-timeout', '300') | Out-Null

  $frontend = Get-ComposeContainerID -Project app -Service frontend
  $originalAPI = Get-ComposeContainerID -Project app -Service admin-api
  Wait-ForHealthy -Container $frontend
  Wait-ForHealthy -Container $originalAPI
  $oldAPIAddress = Get-ContainerNetworkAddress -Container $originalAPI

  Invoke-AppCompose -Arguments @('stop', 'admin-api') | Out-Null
  Invoke-AppCompose -Arguments @('rm', '-f', 'admin-api') | Out-Null
  Invoke-Docker -Arguments @(
    'run', '--detach', '--name', $reservationName, '--network', $Network,
    '--ip', $oldAPIAddress,
    '--entrypoint', '/bin/sh', 'admin-go-backend:local',
    '-c', 'while :; do sleep 3600; done'
  ) | Out-Null
  $reservationExists = $true

  Invoke-AppCompose -Arguments @('up', '-d', '--no-deps', '--no-build', 'admin-api') | Out-Null
  $replacementAPI = Get-ComposeContainerID -Project app -Service admin-api
  Wait-ForHealthy -Container $replacementAPI
  $newAPIAddress = Get-ContainerNetworkAddress -Container $replacementAPI
  if ($newAPIAddress -eq $oldAPIAddress) {
    throw "API address did not change from $oldAPIAddress"
  }
  if ((Get-ComposeContainerID -Project app -Service frontend) -cne $frontend) {
    throw 'frontend was recreated during API address replacement'
  }
  Wait-ForCondition -TimeoutSeconds 20 -FailureMessage 'frontend proxy did not follow the replacement API address' -Probe {
    Test-FrontendProxy -FrontendContainer $frontend
  }

  Invoke-Docker -Arguments @('rm', '--force', $reservationName) | Out-Null
  $reservationExists = $false

  Invoke-Docker -Arguments @('network', 'disconnect', $Network, $stateMySQL) | Out-Null
  Invoke-Docker -Arguments @('network', 'disconnect', $Network, $stateRedis) | Out-Null
  Invoke-AppCompose -Arguments @('up', '-d', '--no-deps', '--no-build', '--force-recreate', 'admin-api', 'admin-worker') | Out-Null
  $lateAPI = Get-ComposeContainerID -Project app -Service admin-api
  $lateWorker = Get-ComposeContainerID -Project app -Service admin-worker
  Wait-ForCondition -TimeoutSeconds 15 -FailureMessage 'API did not remain running while state was stopped' -Probe {
    Test-ContainerRunning -Container $lateAPI
  }
  Wait-ForCondition -TimeoutSeconds 15 -FailureMessage 'worker did not remain running while state was stopped' -Probe {
    Test-ContainerRunning -Container $lateWorker
  }
  Start-Sleep -Seconds $LateStateProbeSeconds
  Assert-NoRestarts -Container $lateAPI
  Assert-NoRestarts -Container $lateWorker

  Invoke-Docker -Arguments @('network', 'connect', '--alias', 'mysql', $Network, $stateMySQL) | Out-Null
  Invoke-Docker -Arguments @('network', 'connect', '--alias', 'redis', $Network, $stateRedis) | Out-Null
  Wait-ForHealthy -Container $lateAPI
  Wait-ForHealthy -Container $lateWorker
  Wait-ForWorkerStartCount -Container $lateWorker -Minimum 1
  Assert-NoRestarts -Container $lateAPI
  Assert-NoRestarts -Container $lateWorker
  if ((Get-ComposeContainerID -Project app -Service frontend) -cne $frontend) {
    throw 'frontend was recreated during state-late recovery'
  }
  Wait-ForCondition -TimeoutSeconds 20 -FailureMessage 'frontend proxy did not recover after state-late startup' -Probe {
    Test-FrontendProxy -FrontendContainer $frontend
  }

  Stop-And-AssertGraceful -Container $lateWorker
  $workerStartCount = Get-WorkerStartCount -Container $lateWorker
  Invoke-AppCompose -Arguments @('up', '-d', '--no-deps', '--no-build', 'admin-worker') | Out-Null
  Wait-ForHealthy -Container $lateWorker
  Wait-ForWorkerStartCount -Container $lateWorker -Minimum ($workerStartCount + 1)

  Stop-And-AssertGraceful -Container $lateAPI
  Invoke-AppCompose -Arguments @('up', '-d', '--no-deps', '--no-build', 'admin-api') | Out-Null
  Wait-ForHealthy -Container $lateAPI

  $finalFrontend = Get-ComposeContainerID -Project app -Service frontend
  $finalAPI = Get-ComposeContainerID -Project app -Service admin-api
  $finalWorker = Get-ComposeContainerID -Project app -Service admin-worker
  foreach ($container in @($finalFrontend, $finalAPI, $finalWorker, $stateMySQL, $stateRedis)) {
    if (-not (Test-ContainerRunning -Container $container)) {
      throw "$container is not running after final restoration"
    }
  }
  foreach ($container in @($finalFrontend, $finalAPI, $finalWorker, $stateMySQL, $stateRedis)) {
    Wait-ForHealthy -Container $container
  }
  Assert-NoRestarts -Container $finalAPI
  Assert-NoRestarts -Container $finalWorker
  Wait-ForCondition -TimeoutSeconds 20 -FailureMessage 'frontend proxy is unavailable after final restoration' -Probe {
    Test-FrontendProxy -FrontendContainer $finalFrontend
  }

  Write-Output "backend_revision=$backendRevision"
  Write-Output "frontend_revision=$frontendRevision"
  Write-Output "api_restart_count=$(Get-RestartCount -Container $finalAPI)"
  Write-Output "worker_restart_count=$(Get-RestartCount -Container $finalWorker)"
  Write-Output 'docker stability assertions passed'
}
finally {
  if ($reservationExists) {
    Invoke-DockerCleanup -Arguments @('rm', '--force', $reservationName)
  }
  if (Test-Path -LiteralPath $script:AppOverride -PathType Leaf) {
    Invoke-DockerCleanup -Arguments @(
      'compose', '--project-name', $script:AppProject,
      '-f', $appCompose,
      '-f', $script:AppOverride,
      'down', '--remove-orphans'
    )
  }
  if (-not [string]::IsNullOrWhiteSpace($stateMySQL)) {
    Invoke-DockerCleanup -Arguments @('network', 'disconnect', $Network, $stateMySQL)
  }
  if (-not [string]::IsNullOrWhiteSpace($stateRedis)) {
    Invoke-DockerCleanup -Arguments @('network', 'disconnect', $Network, $stateRedis)
  }
  if ($networkExists) {
    Invoke-DockerCleanup -Arguments @('network', 'rm', $Network)
  }
  Remove-Item -LiteralPath $temporaryRoot -Recurse -Force -ErrorAction SilentlyContinue
}
