[CmdletBinding()]
param(
  [string]$OutputDirectory,
  [string]$BackendImageName = 'admin-go-backend',
  [string]$FrontendImageName = 'admin-frontend',
  [string]$DockerCommand = 'docker'
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot 'check-inputs.ps1') -ImportFunctions

$outputRoot = Join-Path $script:BackendRoot 'release\admin-only\out'
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) { $OutputDirectory = Join-Path $outputRoot 'images' }
$imageDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$expectedDirectory = [IO.Path]::GetFullPath((Join-Path $outputRoot 'images'))
if (-not $imageDirectory.Equals($expectedDirectory, [StringComparison]::OrdinalIgnoreCase)) {
  throw 'image output directory is fixed by the release contract'
}
foreach ($name in @($BackendImageName, $FrontendImageName)) {
  if ($name -cnotmatch '^[a-z0-9][a-z0-9._/-]*$') { throw 'release image name is invalid' }
}

function Resolve-DockerExecutable {
  param([Parameter(Mandatory = $true)][string]$Command)
  if ([IO.Path]::IsPathRooted($Command)) {
    if (-not (Test-Path -LiteralPath $Command -PathType Leaf)) { throw 'Docker executable is missing' }
    return [IO.Path]::GetFullPath($Command)
  }
  return (Get-Command $Command -ErrorAction Stop | Select-Object -First 1).Source
}

function Invoke-DockerCommand {
  param(
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label,
    [switch]$Capture
  )
  $lines = @(& $script:DockerExecutable @Arguments 2>&1)
  if ($LASTEXITCODE -ne 0) { throw "$Label failed" }
  if ($Capture) { return ($lines | ForEach-Object { $_.ToString() }) -join "`n" }
}

function Get-ImageInspection {
  param([Parameter(Mandatory = $true)][string]$Reference)
  $json = Invoke-DockerCommand -Arguments @('image', 'inspect', $Reference) -Label 'docker image inspect' -Capture
  try {
    $items = @($json | ConvertFrom-Json -Depth 40)
  } catch {
    throw 'Docker image inspection returned invalid JSON'
  }
  if ($items.Count -ne 1) { throw 'Docker image inspection returned an unexpected count' }
  return $items[0]
}

function Export-VerifiedImage {
  param(
    [Parameter(Mandatory = $true)][string]$ImageName,
    [Parameter(Mandatory = $true)][string]$Tag,
    [Parameter(Mandatory = $true)][string]$Commit,
    [Parameter(Mandatory = $true)][string]$ArchivePrefix
  )
  $inspection = Get-ImageInspection -Reference $Tag
  $imageID = [string]$inspection.Id
  if ($imageID -cnotmatch '^sha256:[0-9a-f]{64}$') { throw "$ArchivePrefix image ID is invalid" }
  if ([string]$inspection.Config.Labels.'org.opencontainers.image.revision' -cne $Commit) {
    throw "$ArchivePrefix image revision label does not match its current owning commit"
  }
  $repoDigests = @($inspection.RepoDigests | ForEach-Object { $_.ToString() } | Sort-Object -Unique)

  $archiveName = "$ArchivePrefix-$Commit.tar"
  $archivePath = Join-Path $imageDirectory $archiveName
  if (Test-Path -LiteralPath $archivePath) { throw "$ArchivePrefix image archive already exists" }
  $temporaryArchive = Join-Path $imageDirectory ('.' + $archiveName + '.' + [guid]::NewGuid().ToString('N') + '.tmp')
  try {
    # docker save and docker load form the immutable archive round trip.
    Invoke-DockerCommand -Arguments @('save', '--output', $temporaryArchive, $Tag) -Label "docker save $ArchivePrefix"
    if (-not (Test-Path -LiteralPath $temporaryArchive -PathType Leaf) -or (Get-Item -LiteralPath $temporaryArchive).Length -le 0) {
      throw "$ArchivePrefix image archive is empty"
    }
    Move-Item -LiteralPath $temporaryArchive -Destination $archivePath

    Invoke-DockerCommand -Arguments @('image', 'rm', $Tag) -Label "remove $ArchivePrefix verification tag"
    [void](Invoke-DockerCommand -Arguments @('load', '--input', $archivePath) -Label "docker load $ArchivePrefix" -Capture)
    $loaded = Get-ImageInspection -Reference $Tag
    if ([string]$loaded.Id -cne $imageID -or
        [string]$loaded.Config.Labels.'org.opencontainers.image.revision' -cne $Commit) {
      throw "$ArchivePrefix image changed during archive load verification"
    }
  } finally {
    if (Test-Path -LiteralPath $temporaryArchive) { Remove-Item -LiteralPath $temporaryArchive -Force }
  }

  return [ordered]@{
    commit = $Commit
    tag = $Tag
    image = "$ImageName@$imageID"
    image_id = $imageID
    repo_digests = $repoDigests
    archive_file = $archiveName
    archive_sha256 = Get-FileSha256 -Path $archivePath
  }
}

# Assert-RepositoryStatus uses git status --porcelain=v1 --untracked-files=all.
# Test-LockedCommitAncestor uses git merge-base --is-ancestor.
Assert-SingleWorktree -Repository $script:BackendRoot
Assert-SingleWorktree -Repository $script:FrontendRoot
Assert-RepositoryStatus -Repository $script:BackendRoot
Assert-RepositoryStatus -Repository $script:FrontendRoot
$inputLock = Assert-InputLockSchema -Path (Join-Path $script:BackendRoot 'release\admin-only\input-lock.json')
Test-LockedCommitAncestor -Repository $script:BackendRoot -LockedCommit (Assert-GitSha -Value ([string]$inputLock.backend_commit) -Label 'locked backend commit')
Test-LockedCommitAncestor -Repository $script:FrontendRoot -LockedCommit (Assert-GitSha -Value ([string]$inputLock.frontend_commit) -Label 'locked frontend commit')

$backendCommit = Get-RepositoryCommit -Repository $script:BackendRoot
$frontendCommit = Get-RepositoryCommit -Repository $script:FrontendRoot
$backendTag = "${BackendImageName}:$backendCommit"
$frontendTag = "${FrontendImageName}:$frontendCommit"
$script:DockerExecutable = Resolve-DockerExecutable -Command $DockerCommand
[IO.Directory]::CreateDirectory($imageDirectory) | Out-Null

$applicationCompose = Join-Path $script:BackendRoot 'deploy\docker-first\docker-compose.yml'
$previousBackendRevision = [Environment]::GetEnvironmentVariable('ADMIN_BACKEND_BUILD_REVISION', 'Process')
$previousFrontendRevision = [Environment]::GetEnvironmentVariable('ADMIN_FRONTEND_BUILD_REVISION', 'Process')
try {
  $env:ADMIN_BACKEND_BUILD_REVISION = $backendCommit
  $env:ADMIN_FRONTEND_BUILD_REVISION = $frontendCommit
  Invoke-DockerCommand -Arguments @('compose', '-f', $applicationCompose, 'build', 'admin-api') -Label 'backend Docker build'
  Invoke-DockerCommand -Arguments @('tag', 'admin-go-backend:local', $backendTag) -Label 'backend release image tag'
} finally {
  [Environment]::SetEnvironmentVariable('ADMIN_BACKEND_BUILD_REVISION', $previousBackendRevision, 'Process')
  [Environment]::SetEnvironmentVariable('ADMIN_FRONTEND_BUILD_REVISION', $previousFrontendRevision, 'Process')
}

$frontendVerifier = Join-Path $script:FrontendRoot 'scripts\verify-frontend.ps1'
& pwsh -NoProfile -File $frontendVerifier -GitSha $frontendCommit -ImageName $FrontendImageName
if ($LASTEXITCODE -ne 0) { throw 'frontend Docker verification failed' }

$metadata = [ordered]@{
  schema_version = 1
  backend = Export-VerifiedImage -ImageName $BackendImageName -Tag $backendTag -Commit $backendCommit -ArchivePrefix 'backend'
  frontend = Export-VerifiedImage -ImageName $FrontendImageName -Tag $frontendTag -Commit $frontendCommit -ArchivePrefix 'frontend'
}
$metadataPath = Join-Path $imageDirectory 'metadata.json'
$temporaryPath = Join-Path $imageDirectory ('.metadata.' + [guid]::NewGuid().ToString('N') + '.tmp')
try {
  [IO.File]::WriteAllText($temporaryPath, ($metadata | ConvertTo-Json -Depth 8) + "`n", [Text.UTF8Encoding]::new($false))
  Move-Item -LiteralPath $temporaryPath -Destination $metadataPath -Force
} finally {
  if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
}

Write-Output 'immutable Docker image archives exported'
