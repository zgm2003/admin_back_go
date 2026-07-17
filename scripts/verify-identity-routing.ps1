[CmdletBinding()]
param(
  [string]$LinuxGoImage = 'docker.m.daocloud.io/library/golang:1.26.5-bookworm'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-GoCommand {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  $commandText = 'go ' + ($Arguments -join ' ')
  Write-Host "==> $commandText"
  & go @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "$commandText failed with exit code $LASTEXITCODE."
  }
}

function Resolve-DockerExecutable {
  $defaultDocker = 'E:\Docker\Docker\resources\bin\docker.exe'
  if (Test-Path -LiteralPath $defaultDocker -PathType Leaf) {
    return $defaultDocker
  }
  $command = Get-Command docker.exe -ErrorAction SilentlyContinue
  if ($null -eq $command) {
    $command = Get-Command docker -ErrorAction SilentlyContinue
  }
  if ($null -eq $command) {
    throw 'Docker is required for the Windows race gate.'
  }
  return $command.Source
}

function Invoke-IdentityRaceTests {
  param(
    [Parameter(Mandatory = $true)][string]$RepositoryRoot,
    [Parameter(Mandatory = $true)][string[]]$Packages
  )

  if ([IO.Path]::DirectorySeparatorChar -ne '\') {
    Invoke-GoCommand -Arguments (@('test', '-race', '-count=1') + $Packages)
    return
  }

  # Keep a fast host compile/unit signal, then run the blocking race execution
  # in the pinned Linux Go container. No API, Worker, MySQL, or Redis host
  # process is started by this gate.
  Invoke-GoCommand -Arguments (@('test', '-count=1') + $Packages)
  $docker = Resolve-DockerExecutable
  $mount = 'type=bind,source=' + $RepositoryRoot + ',target=/src'
  $arguments = @(
    'run', '--rm',
    '--mount', $mount,
    '--mount', 'type=volume,source=admin-go-identity-race-modcache,target=/go/pkg/mod',
    '--mount', 'type=volume,source=admin-go-identity-race-buildcache,target=/root/.cache/go-build',
    '--workdir', '/src',
    '--env', 'GOTOOLCHAIN=local',
    '--env', 'GOWORK=off',
    '--env', 'GOFLAGS=-mod=readonly',
    $LinuxGoImage,
    'go', 'test', '-race', '-count=1'
  ) + $Packages
  Write-Host "==> docker run ... go test -race $($Packages -join ' ')"
  & $docker @arguments
  if ($LASTEXITCODE -ne 0) {
    throw "Linux identity race gate failed with exit code $LASTEXITCODE."
  }
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$locationPushed = $false
try {
  Push-Location -LiteralPath $repoRoot
  $locationPushed = $true

  $packages = @(
    './internal/module/auth',
    './internal/module/permission',
    './internal/module/role',
    './internal/module/user',
    './internal/middleware',
    './internal/server/...'
  )
  Invoke-IdentityRaceTests -RepositoryRoot $repoRoot -Packages $packages
  Invoke-GoCommand -Arguments @(
    'test', './internal/architecture',
    '-run', 'TestIdentity|TestRoutePolicy|TestCredential',
    '-count=1'
  )
  & (Join-Path $repoRoot 'scripts/check-admin-contract.ps1')
  if ($LASTEXITCODE -ne 0) {
    throw "Admin contract gate failed with exit code $LASTEXITCODE."
  }
}
finally {
  if ($locationPushed) {
    Pop-Location
  }
}
