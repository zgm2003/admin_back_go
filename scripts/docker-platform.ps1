[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidateSet('init','dev-state','up','stop','status')]
  [string]$Action
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
. (Join-Path $PSScriptRoot 'dev\admin-dev-common.ps1')
$stateCompose = Join-Path $repoRoot 'deploy\docker-state\docker-compose.yml'
$stateImageEnv = Join-Path $repoRoot 'deploy\docker-state\qdrant-image.env'
$appCompose = Join-Path $repoRoot 'deploy\docker-first\docker-compose.yml'
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '..\admin_front_ts'))
$stateRuntime = Join-Path $repoRoot 'deploy\docker-state\runtime'
$mysqlSecret = Join-Path $stateRuntime 'mysql-root-password.txt'
$backendEnv = Join-Path $repoRoot 'deploy\docker-first\admin-go.env'
$adminDevLock = Join-Path $repoRoot '.tmp\dev\admin-dev.lock.json'
$defaultDockerBin = 'E:\Docker\Docker\resources\bin'
$defaultDocker = Join-Path $defaultDockerBin 'docker.exe'

if (Test-Path -LiteralPath $defaultDocker -PathType Leaf) {
  $script:DockerExecutable = $defaultDocker
}
else {
  $script:DockerExecutable = (Get-Command docker.exe -ErrorAction Stop | Select-Object -First 1).Source
}
$dockerBin = Split-Path $script:DockerExecutable -Parent
$env:Path = $dockerBin + [IO.Path]::PathSeparator + $env:Path

function Invoke-Docker {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  & $script:DockerExecutable @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "docker exited with code $LASTEXITCODE"
  }
}

function Resolve-GitRevision {
  param([Parameter(Mandatory = $true)][string]$Repository)

  if (-not (Test-Path -LiteralPath $Repository -PathType Container)) {
    throw "repository is missing: $Repository"
  }
  $git = Get-Command git.exe -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $git) {
    $git = Get-Command git -ErrorAction Stop | Select-Object -First 1
  }
  $output = & $git.Source -C $Repository rev-parse --verify HEAD 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "could not resolve Git revision for $Repository"
  }
  $revision = (($output | Select-Object -Last 1).ToString()).Trim().ToLowerInvariant()
  if ($revision -notmatch '^[0-9a-f]{40}$') {
    throw "invalid Git revision for $Repository"
  }
  return $revision
}

function Write-OwnerOnlySecret {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Value
  )

  $directory = Split-Path $Path -Parent
  [IO.Directory]::CreateDirectory($directory) | Out-Null
  $temporaryPath = $Path + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
  try {
    [IO.File]::WriteAllText($temporaryPath, $Value + "`n", (New-Object Text.UTF8Encoding($false)))

    $currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = New-Object Security.AccessControl.FileSecurity
    $acl.SetOwner($currentSid)
    $acl.SetAccessRuleProtection($true, $false)
    $accessRule = New-Object Security.AccessControl.FileSystemAccessRule(
      $currentSid,
      [Security.AccessControl.FileSystemRights]::FullControl,
      [Security.AccessControl.AccessControlType]::Allow
    )
    $acl.SetAccessRule($accessRule)
    Set-Acl -LiteralPath $temporaryPath -AclObject $acl

    Move-Item -LiteralPath $temporaryPath -Destination $Path -Force
    $temporaryPath = $null

    $finalACL = Get-Acl -LiteralPath $Path
    if (-not $finalACL.AreAccessRulesProtected -or
        $finalACL.GetOwner([Security.Principal.SecurityIdentifier]).Value -cne $currentSid.Value) {
      throw 'MySQL secret permissions could not be verified'
    }
    foreach ($rule in $finalACL.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])) {
      if ($rule.IdentityReference.Value -cne $currentSid.Value) {
        throw 'MySQL secret grants access to another identity'
      }
    }
  }
  finally {
    if (-not [string]::IsNullOrEmpty($temporaryPath) -and (Test-Path -LiteralPath $temporaryPath)) {
      Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue
    }
  }
}

switch ($Action) {
  'init' {
    $statePath = Join-Path $env:TEMP 'admin-p02-workspace\state.json'
    if (-not (Test-Path -LiteralPath $statePath -PathType Leaf)) {
      throw 'P02 workspace state is missing'
    }

    $state = Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json
    $password = [string]$state.root_password
    try {
      if ($password -notmatch '^[A-Za-z0-9._~-]+$') {
        throw 'P02 root password is not Compose-safe'
      }

      Write-OwnerOnlySecret -Path $mysqlSecret -Value $password
      $dsn = 'root:' + $password + '@tcp(mysql:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local'
      try {
        & (Join-Path $repoRoot 'deploy\docker-first\init-local-env.ps1') `
          -MySQLDSN $dsn `
          -RedisAddress 'redis:6379' `
          -CorsOrigin 'http://localhost:5173,http://127.0.0.1:5173'
      }
      finally {
        $dsn = $null
      }
    }
    finally {
      $password = $null
      $state = $null
    }
    Write-Output 'initialized ignored Docker platform runtime files'
  }

  'dev-state' {
    if (-not (Test-Path -LiteralPath $mysqlSecret -PathType Leaf)) {
      throw 'MySQL secret is missing; run init first'
    }
    Invoke-Docker @('compose', '-f', $appCompose, 'stop', 'frontend', 'admin-api', 'admin-worker')
    Invoke-Docker @('compose', '--env-file', $stateImageEnv, '-f', $stateCompose, 'up', '-d', '--wait', '--wait-timeout', '180')
  }

  'up' {
    Assert-NoLiveAdminDevLock -Path $adminDevLock -RepositoryRoot $repoRoot
    if (-not (Test-Path -LiteralPath $mysqlSecret -PathType Leaf)) {
      throw 'MySQL secret is missing; run init first'
    }
    if (-not (Test-Path -LiteralPath $backendEnv -PathType Leaf)) {
      throw 'backend runtime env is missing; run init first'
    }
    $previousBackendRevision = [Environment]::GetEnvironmentVariable('ADMIN_BACKEND_BUILD_REVISION', 'Process')
    $previousFrontendRevision = [Environment]::GetEnvironmentVariable('ADMIN_FRONTEND_BUILD_REVISION', 'Process')
    try {
      $env:ADMIN_BACKEND_BUILD_REVISION = Resolve-GitRevision -Repository $repoRoot
      $env:ADMIN_FRONTEND_BUILD_REVISION = Resolve-GitRevision -Repository $frontendRoot
      Invoke-Docker @('compose', '-f', $appCompose, 'build', 'admin-api', 'frontend')
      Invoke-Docker @('compose', '--env-file', $stateImageEnv, '-f', $stateCompose, 'up', '-d', '--wait', '--wait-timeout', '180')
      Invoke-Docker @('compose', '-f', $appCompose, 'up', '-d', '--no-build', '--wait', '--wait-timeout', '300')
    }
    finally {
      [Environment]::SetEnvironmentVariable('ADMIN_BACKEND_BUILD_REVISION', $previousBackendRevision, 'Process')
      [Environment]::SetEnvironmentVariable('ADMIN_FRONTEND_BUILD_REVISION', $previousFrontendRevision, 'Process')
    }
  }

  'stop' {
    Assert-NoLiveAdminDevLock -Path $adminDevLock -RepositoryRoot $repoRoot
    Invoke-Docker @('compose', '-f', $appCompose, 'stop')
      Invoke-Docker @('compose', '--env-file', $stateImageEnv, '-f', $stateCompose, 'stop')
  }

  'status' {
      Invoke-Docker @('compose', '--env-file', $stateImageEnv, '-f', $stateCompose, 'ps')
    Invoke-Docker @('compose', '-f', $appCompose, 'ps')
  }
}
