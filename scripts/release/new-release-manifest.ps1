[CmdletBinding()]
param(
  [string]$ReleaseID,
  [string]$InputLock,
  [string]$PlatformKernelProof,
  [string]$ImageMetadata,
  [string]$Output,
  [string]$DockerCommand = 'docker'
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest

$requested = [pscustomobject]@{
  ReleaseID = $ReleaseID
  InputLock = $InputLock
  PlatformKernelProof = $PlatformKernelProof
  ImageMetadata = $ImageMetadata
  Output = $Output
  DockerCommand = $DockerCommand
}
. (Join-Path $PSScriptRoot 'check-release-manifest.ps1') -ImportFunctions
$ReleaseID = $requested.ReleaseID
$InputLock = $requested.InputLock
$PlatformKernelProof = $requested.PlatformKernelProof
$ImageMetadata = $requested.ImageMetadata
$Output = $requested.Output
$DockerCommand = $requested.DockerCommand

if ([string]::IsNullOrWhiteSpace($InputLock)) { $InputLock = Join-Path $script:BackendRoot 'release\admin-only\input-lock.json' }
if ([string]::IsNullOrWhiteSpace($PlatformKernelProof)) { $PlatformKernelProof = Join-Path $script:ReleaseOutputRoot 'platform-kernel-proof.json' }
if ([string]::IsNullOrWhiteSpace($ImageMetadata)) { $ImageMetadata = Join-Path $script:ReleaseOutputRoot 'images\metadata.json' }
if ([string]::IsNullOrWhiteSpace($Output)) { $Output = Join-Path $script:ReleaseOutputRoot 'release-manifest.json' }

$outputPath = [IO.Path]::GetFullPath($Output)
$expectedOutput = [IO.Path]::GetFullPath((Join-Path $script:ReleaseOutputRoot 'release-manifest.json'))
if (-not $outputPath.Equals($expectedOutput, [StringComparison]::OrdinalIgnoreCase)) {
  throw 'release manifest output path is fixed by the release contract'
}

if ([string]::IsNullOrWhiteSpace($ReleaseID)) {
  $sequence = [string]$env:ADMIN_RELEASE_SEQUENCE
  if ([string]::IsNullOrWhiteSpace($sequence)) { $sequence = '1' }
  if ($sequence -notmatch '^[1-9][0-9]*$') { throw 'ADMIN_RELEASE_SEQUENCE must be a positive integer' }
  $ReleaseID = 'admin-v' + [DateTime]::UtcNow.ToString('yyyy.MM.dd', [Globalization.CultureInfo]::InvariantCulture) + '.' + $sequence
}
if ($ReleaseID -cnotmatch '^admin-v[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[1-9][0-9]*$') {
  throw 'release ID is invalid'
}

$lockPath = Get-RequiredFilePath -Path $InputLock -Label 'input lock'
$proofPath = Get-RequiredFilePath -Path $PlatformKernelProof -Label 'platform-kernel proof'
$metadataPath = Get-RequiredFilePath -Path $ImageMetadata -Label 'image metadata'
$lock = Assert-InputLockSchema -Path $lockPath
$proof = Read-JsonEvidence -Path $proofPath -Label 'platform-kernel proof'
$metadata = Read-JsonEvidence -Path $metadataPath -Label 'image metadata'
if ([int]$proof.schema_version -ne 1 -or $proof.passed -ne $true) { throw 'platform-kernel proof did not pass' }
if ([int]$metadata.schema_version -ne 1) { throw 'image metadata identity is invalid' }

$backendCommit = Get-RepositoryCommit -Repository $script:BackendRoot
$frontendCommit = Get-RepositoryCommit -Repository $script:FrontendRoot
Assert-SingleWorktree -Repository $script:BackendRoot
Assert-SingleWorktree -Repository $script:FrontendRoot
Assert-RepositoryStatus -Repository $script:BackendRoot
Assert-RepositoryStatus -Repository $script:FrontendRoot
Test-LockedCommitAncestor -Repository $script:BackendRoot -LockedCommit (Assert-GitSha -Value ([string]$lock.backend_commit) -Label 'locked backend commit')
Test-LockedCommitAncestor -Repository $script:FrontendRoot -LockedCommit (Assert-GitSha -Value ([string]$lock.frontend_commit) -Label 'locked frontend commit')
Assert-ExactString ([string]$proof.backend_commit) $backendCommit 'platform proof backend commit'
Assert-ExactString ([string]$proof.frontend_commit) $frontendCommit 'platform proof frontend commit'
Assert-ExactString ([string]$metadata.backend.commit) $backendCommit 'backend image commit'
Assert-ExactString ([string]$metadata.frontend.commit) $frontendCommit 'frontend image commit'

$contractPath = Join-Path $script:BackendRoot 'contracts\admin\v1\manifest.json'
$contract = Read-JsonEvidence -Path $contractPath -Label 'backend contract manifest'
$contractDigest = Get-FileSha256 -Path $contractPath
Assert-ExactString ([string]$proof.bundle_version) ([string]$contract.bundle_version) 'platform proof bundle version'
Assert-ExactString ([string]$proof.contract_manifest_sha256) $contractDigest 'platform proof contract digest'

$releaseDocument = [ordered]@{
  schema_version = 1
  release_id = $ReleaseID
  backend = [ordered]@{
    commit = $backendCommit
    image = [string]$metadata.backend.image
    archive_sha256 = [string]$metadata.backend.archive_sha256
  }
  frontend = [ordered]@{
    commit = $frontendCommit
    image = [string]$metadata.frontend.image
    archive_sha256 = [string]$metadata.frontend.archive_sha256
  }
  contract = [ordered]@{
    bundle_version = [string]$contract.bundle_version
    manifest_sha256 = $contractDigest
  }
  evidence = [ordered]@{
    input_lock_sha256 = Get-FileSha256 -Path $lockPath
    query_sha256 = [string]$lock.query_evidence_sha256
    cos_disposition_sha256 = [string]$lock.cos_disposition_evidence_sha256
    recovery_sha256 = [string]$lock.recovery_artifact_sha256
    browser_only_retirement_sha256 = [string]$lock.client_versions_freeze_evidence_sha256
    platform_kernel_sha256 = Get-FileSha256 -Path $proofPath
  }
}

$json = $releaseDocument | ConvertTo-Json -Depth 8
if (-not ($json | Test-Json -SchemaFile $script:ReleaseManifestSchema -ErrorAction SilentlyContinue)) {
  throw 'generated release manifest does not match its schema'
}
[IO.Directory]::CreateDirectory((Split-Path -Parent $outputPath)) | Out-Null
$temporaryPath = Join-Path (Split-Path -Parent $outputPath) ('.release-manifest.' + [guid]::NewGuid().ToString('N') + '.tmp')
try {
  [IO.File]::WriteAllText($temporaryPath, $json + "`n", [Text.UTF8Encoding]::new($false))
  $dockerExecutable = Resolve-ReleaseDocker -Command $DockerCommand
  [void](Assert-ReleaseManifest -ManifestPath $temporaryPath -InputLockPath $lockPath -PlatformKernelProofPath $proofPath -ImageMetadataPath $metadataPath -DockerExecutable $dockerExecutable)
  Move-Item -LiteralPath $temporaryPath -Destination $outputPath -Force
} finally {
  if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
}
Write-Output "release manifest created: $ReleaseID"
