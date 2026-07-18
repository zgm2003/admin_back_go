[CmdletBinding()]
param(
  [switch]$SkipRestartScenario,
  [string]$GoImage = 'docker.m.daocloud.io/library/golang:1.26.5-bookworm'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Resolve-DockerExecutable {
  $preferred = 'E:\Docker\Docker\resources\bin\docker.exe'
  if (Test-Path -LiteralPath $preferred -PathType Leaf) { return $preferred }
  $command = Get-Command docker.exe -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $command) { $command = Get-Command docker -ErrorAction Stop | Select-Object -First 1 }
  return $command.Source
}

function Invoke-DockerCommand {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  & $script:Docker @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "Docker verification command failed: $($Arguments[0])"
  }
}

function Invoke-DockerGo {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  $dockerArguments = @(
    'run','--rm','--network','none',
    '--mount',('type=bind,source=' + $script:RepoRoot + ',target=/src'),
    '--mount','type=volume,source=admin-go-mod-cache,target=/go/pkg/mod',
    '--mount','type=volume,source=admin-go-race-buildcache,target=/root/.cache/go-build',
    '--workdir','/src','--env','GOTOOLCHAIN=local','--env','GOWORK=off','--env','GOFLAGS=-mod=readonly',
    $GoImage,'go'
  ) + $Arguments
  Invoke-DockerCommand -Arguments $dockerArguments
}

$script:Docker = Resolve-DockerExecutable
$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

Invoke-DockerGo -Arguments @(
  'test','-race','-count=10',
  './internal/infra/realtime',
  './internal/module/realtime',
  './internal/module/notification/task',
  './internal/module/ai/replycommand',
  './internal/runtime'
)
Invoke-DockerGo -Arguments @('test','./internal/architecture','-run','TestDurableWork','-count=1')
Invoke-DockerGo -Arguments @('test','./internal/admincontract','-run','TestRealtime','-count=1')

Invoke-DockerCommand -Arguments @(
  'run','--rm','--network','none',
  '--mount',('type=bind,source=' + $script:RepoRoot + ',target=/src,readonly'),
  '--workdir','/src','arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a',
  'migrate','validate','--dir','file://database/migrations'
)

$manifest = Get-Content -Raw -LiteralPath (Join-Path $script:RepoRoot 'contracts\admin\v1\manifest.json') | ConvertFrom-Json
$contractCommit = [string]$manifest.backend_commit
Invoke-DockerGo -Arguments @('run','./cmd/admin-contract','check','--out','contracts/admin/v1','--commit',$contractCommit)
Invoke-DockerGo -Arguments @('build','-trimpath','-o','/tmp/admin-api','./cmd/admin-api')
Invoke-DockerGo -Arguments @('build','-trimpath','-o','/tmp/admin-worker','./cmd/admin-worker')

if (-not $SkipRestartScenario) {
  & (Join-Path $PSScriptRoot 'tests\durable-work-restart.tests.ps1') -GoImage $GoImage
  if ($LASTEXITCODE -ne 0) {
    throw "durable-work-restart.tests.ps1 failed with exit code $LASTEXITCODE"
  }
}

Write-Output 'durable work verification passed'
