[CmdletBinding()]
param(
  [string]$LinuxGoImage = 'docker.m.daocloud.io/library/golang:1.26.5-bookworm'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-GoCommand {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$Arguments
  )

  $commandText = 'go ' + ($Arguments -join ' ')
  Write-Host "==> $commandText"
  & go @Arguments
  $exitCode = $LASTEXITCODE
  if ($exitCode -ne 0) {
    throw "$commandText failed with exit code $exitCode."
  }
}

function Resolve-DockerExecutable {
  $command = Get-Command docker.exe -ErrorAction SilentlyContinue
  if ($null -eq $command) {
    $command = Get-Command docker -ErrorAction SilentlyContinue
  }
  if ($null -ne $command) {
    return $command.Source
  }

  if ([IO.Path]::DirectorySeparatorChar -eq '\') {
    $dockerRoot = Join-Path $env:ProgramFiles 'Docker'
    if (Test-Path -LiteralPath $dockerRoot) {
      $candidate = Get-ChildItem -LiteralPath $dockerRoot -Filter docker.exe -File -Recurse -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
      if ($null -ne $candidate) {
        return $candidate.FullName
      }
    }
  }
  throw 'Docker is required to run Go race tests on Windows without a host C compiler.'
}

function Invoke-RaceTests {
  param(
    [Parameter(Mandatory = $true)]
    [string]$RepositoryRoot,
    [Parameter(Mandatory = $true)]
    [string[]]$Packages
  )

  if ([IO.Path]::DirectorySeparatorChar -ne '\') {
    Invoke-GoCommand -Arguments (@('test', '-race', '-count=1') + $Packages)
    return
  }

  # The Windows host intentionally has no C compiler. Compile and execute the
  # same package list under Linux so -race remains a blocking gate.
  Invoke-GoCommand -Arguments (@('test', '-count=1') + $Packages)
  $docker = Resolve-DockerExecutable
  $mount = 'type=bind,source=' + $RepositoryRoot + ',target=/src'
  $arguments = @(
    'run', '--rm',
    '--mount', $mount,
    '--mount', 'type=volume,source=admin-go-race-modcache,target=/go/pkg/mod',
    '--mount', 'type=volume,source=admin-go-race-buildcache,target=/root/.cache/go-build',
    '--workdir', '/src',
    '--env', 'GOTOOLCHAIN=local',
    '--env', 'GOWORK=off',
    '--env', 'GOFLAGS=-mod=readonly',
    $LinuxGoImage,
    'go', 'test', '-race', '-count=1'
  ) + $Packages
  Write-Host "==> docker run ... go test -race $($Packages -join ' ')"
  & $docker @arguments
  $exitCode = $LASTEXITCODE
  if ($exitCode -ne 0) {
    throw "Linux Go race test failed with exit code $exitCode."
  }
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$binarySuffix = ''
if ([IO.Path]::DirectorySeparatorChar -eq '\') {
  $binarySuffix = '.exe'
}
$apiBinary = Join-Path $temporaryRoot ("admin-api-contract-$PID$binarySuffix")
$workerBinary = Join-Path $temporaryRoot ("admin-worker-contract-$PID$binarySuffix")
$locationPushed = $false

try {
  Push-Location -LiteralPath $repoRoot
  $locationPushed = $true

  $packages = @(
    './internal/runtime',
    './internal/platform/admin',
    './internal/server/...',
    './internal/admincontract',
    './internal/telemetry',
    './internal/module/auth',
    './internal/module/payment/...',
    './internal/infra/taskqueue',
    './internal/infra/realtime/...'
  )
  Invoke-RaceTests -RepositoryRoot $repoRoot -Packages $packages

  & (Join-Path $repoRoot 'scripts/check-admin-contract.ps1')

  Invoke-GoCommand -Arguments @(
    'test',
    './internal/architecture',
    '-run',
    'TestRuntime|TestAdminContract|TestRoutePolicy',
    '-count=1'
  )
  Invoke-GoCommand -Arguments @('build', '-trimpath', '-o', $apiBinary, './cmd/admin-api')
  Invoke-GoCommand -Arguments @('build', '-trimpath', '-o', $workerBinary, './cmd/admin-worker')
}
finally {
  if ($locationPushed) {
    Pop-Location
  }
  foreach ($binary in @($apiBinary, $workerBinary)) {
    if (Test-Path -LiteralPath $binary -PathType Leaf) {
      Remove-Item -LiteralPath $binary -Force
    }
  }
}
