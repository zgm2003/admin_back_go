[CmdletBinding()]
param(
  [switch]$SchemaOnly,
  [switch]$ImportFunctions,
  [string]$Manifest,
  [string]$InputLock,
  [string]$PlatformKernelProof,
  [string]$ImageMetadata,
  [string]$DockerCommand = 'docker'
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest

$requestedSchemaOnly = [bool]$SchemaOnly
$requestedImportFunctions = [bool]$ImportFunctions
. (Join-Path $PSScriptRoot 'check-inputs.ps1') -ImportFunctions

$script:ReleaseManifestSchema = Join-Path $script:BackendRoot 'release\admin-only\release-manifest.schema.json'
$script:ReleaseOutputRoot = Join-Path $script:BackendRoot 'release\admin-only\out'
$script:DefaultReleaseManifest = Join-Path $script:ReleaseOutputRoot 'release-manifest.json'
$script:DefaultPlatformKernelProof = Join-Path $script:ReleaseOutputRoot 'platform-kernel-proof.json'
$script:DefaultImageMetadata = Join-Path $script:ReleaseOutputRoot 'images\metadata.json'

function Assert-ExactString {
  param(
    [AllowEmptyString()][string]$Actual,
    [AllowEmptyString()][string]$Expected,
    [Parameter(Mandatory = $true)][string]$Label
  )
  if ($Actual -cne $Expected) { throw "$Label mismatch" }
}

function Get-ReleaseManifestDocument {
  param([Parameter(Mandatory = $true)][string]$Path)
  $resolved = Get-RequiredFilePath -Path $Path -Label 'release manifest'
  $json = [IO.File]::ReadAllText($resolved, [Text.Encoding]::UTF8)
  if (-not ($json | Test-Json -SchemaFile $script:ReleaseManifestSchema -ErrorAction SilentlyContinue)) {
    throw 'release manifest does not match its schema'
  }
  return $json | ConvertFrom-Json -Depth 20
}

function Get-ReleaseImageID {
  param([Parameter(Mandatory = $true)][string]$Image)
  if ($Image -cnotmatch '^[^@\s]+@(sha256:[0-9a-f]{64})$') {
    throw 'release image is not an immutable named digest'
  }
  return $Matches[1]
}

function Assert-PlatformKernelProof {
  param([Parameter(Mandatory = $true)]$Proof)
  if ([int]$Proof.schema_version -ne 1 -or $Proof.passed -ne $true) {
    throw 'platform-kernel proof did not pass'
  }
  if ([string]$Proof.target_fingerprint_sha256 -cnotmatch '^[0-9a-f]{64}$' -or
      [string]$Proof.contract_manifest_sha256 -cnotmatch '^[0-9a-f]{64}$' -or
      [string]$Proof.backend_commit -cnotmatch '^[0-9a-f]{40}$' -or
      [string]$Proof.frontend_commit -cnotmatch '^[0-9a-f]{40}$') {
    throw 'platform-kernel proof digest or commit is invalid'
  }
  if ([int]$Proof.registered_platform_count -ne 1 -or
      [int]$Proof.retired_platform_count -ne 0 -or
      [int]$Proof.auth_platform_operation_count -ne 7 -or
      [int]$Proof.platform_schema_field_count -lt 11) {
    throw 'platform-kernel proof does not demonstrate the retained Admin kernel'
  }
  $counts = @($Proof.invariant_counts.PSObject.Properties)
  if ($counts.Count -eq 0) { throw 'platform-kernel proof has no invariant counts' }
  foreach ($entry in $counts) {
    if ([string]$entry.Value -notmatch '^[0-9]+$' -or [uint64]$entry.Value -ne 0) {
      throw 'platform-kernel proof contains a non-zero invariant count'
    }
  }
}

function Get-ReleaseArchivePath {
  param(
    [Parameter(Mandatory = $true)][string]$MetadataPath,
    [Parameter(Mandatory = $true)]$ImageMetadataEntry,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $name = [string]$ImageMetadataEntry.archive_file
  if ([string]::IsNullOrWhiteSpace($name) -or $name -cne [IO.Path]::GetFileName($name) -or $name -notmatch '\.tar$') {
    throw "$Label archive file name is invalid"
  }
  $root = [IO.Path]::GetFullPath((Split-Path -Parent $MetadataPath))
  $path = [IO.Path]::GetFullPath((Join-Path $root $name))
  if (-not (Test-PathWithin -Candidate $path -Parent $root)) { throw "$Label archive escaped its image directory" }
  return Get-RequiredFilePath -Path $path -Label "$Label image archive"
}

function Resolve-ReleaseDocker {
  param([Parameter(Mandatory = $true)][string]$Command)
  if ([IO.Path]::IsPathRooted($Command)) {
    if (-not (Test-Path -LiteralPath $Command -PathType Leaf)) { throw 'Docker executable is missing' }
    return [IO.Path]::GetFullPath($Command)
  }
  return (Get-Command $Command -ErrorAction Stop | Select-Object -First 1).Source
}

function Get-DockerImageInspection {
  param(
    [Parameter(Mandatory = $true)][AllowEmptyString()][string]$DockerExecutable,
    [Parameter(Mandatory = $true)][string]$ImageID,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $output = @(& $DockerExecutable image inspect $ImageID 2>$null)
  if ($LASTEXITCODE -ne 0) { throw "$Label image is not loaded" }
  try {
    $items = @(($output -join "`n") | ConvertFrom-Json -Depth 40)
  } catch {
    throw "$Label image inspection is invalid"
  }
  if ($items.Count -ne 1) { throw "$Label image inspection returned an unexpected count" }
  return $items[0]
}

function Assert-ReleaseImage {
  param(
    [Parameter(Mandatory = $true)]$ManifestEntry,
    [Parameter(Mandatory = $true)]$MetadataEntry,
    [Parameter(Mandatory = $true)][string]$MetadataPath,
    [Parameter(Mandatory = $true)][AllowEmptyString()][string]$DockerExecutable,
    [Parameter(Mandatory = $true)][string]$Label,
    [switch]$SkipImageInspection
  )
  $imageID = Get-ReleaseImageID -Image ([string]$ManifestEntry.image)
  Assert-ExactString ([string]$MetadataEntry.commit) ([string]$ManifestEntry.commit) "$Label image commit"
  Assert-ExactString ([string]$MetadataEntry.image) ([string]$ManifestEntry.image) "$Label immutable image"
  Assert-ExactString ([string]$MetadataEntry.image_id) $imageID "$Label image ID"
  Assert-ExactString ([string]$MetadataEntry.archive_sha256) ([string]$ManifestEntry.archive_sha256) "$Label archive digest"
  $archivePath = Get-ReleaseArchivePath -MetadataPath $MetadataPath -ImageMetadataEntry $MetadataEntry -Label $Label
  Assert-ExactString (Get-FileSha256 -Path $archivePath) ([string]$ManifestEntry.archive_sha256) "$Label archive SHA-256"
  if (-not $SkipImageInspection) {
    $inspection = Get-DockerImageInspection -DockerExecutable $DockerExecutable -ImageID $imageID -Label $Label
    Assert-ExactString ([string]$inspection.Id) $imageID "$Label loaded image ID"
    Assert-ExactString ([string]$inspection.Config.Labels.'org.opencontainers.image.revision') ([string]$ManifestEntry.commit) "$Label image revision label"
  }
}

function Assert-ReleaseRepositories {
  param(
    [Parameter(Mandatory = $true)]$Release,
    [Parameter(Mandatory = $true)]$Lock
  )
  Assert-SingleWorktree -Repository $script:BackendRoot
  Assert-SingleWorktree -Repository $script:FrontendRoot
  Assert-RepositoryStatus -Repository $script:BackendRoot
  Assert-RepositoryStatus -Repository $script:FrontendRoot
  Assert-ExactString (Get-RepositoryCommit -Repository $script:BackendRoot) ([string]$Release.backend.commit) 'backend release commit'
  Assert-ExactString (Get-RepositoryCommit -Repository $script:FrontendRoot) ([string]$Release.frontend.commit) 'frontend release commit'
  Test-LockedCommitAncestor -Repository $script:BackendRoot -LockedCommit (Assert-GitSha -Value ([string]$Lock.backend_commit) -Label 'locked backend commit')
  Test-LockedCommitAncestor -Repository $script:FrontendRoot -LockedCommit (Assert-GitSha -Value ([string]$Lock.frontend_commit) -Label 'locked frontend commit')
}

function Assert-ReleaseManifest {
  param(
    [Parameter(Mandatory = $true)][string]$ManifestPath,
    [Parameter(Mandatory = $true)][string]$InputLockPath,
    [Parameter(Mandatory = $true)][string]$PlatformKernelProofPath,
    [Parameter(Mandatory = $true)][string]$ImageMetadataPath,
    [string]$DockerExecutable = '',
    [switch]$SkipRepositoryCheck,
    [switch]$SkipImageInspection
  )
  $releasePath = Get-RequiredFilePath -Path $ManifestPath -Label 'release manifest'
  $lockPath = Get-RequiredFilePath -Path $InputLockPath -Label 'input lock'
  $proofPath = Get-RequiredFilePath -Path $PlatformKernelProofPath -Label 'platform-kernel proof'
  $metadataPath = Get-RequiredFilePath -Path $ImageMetadataPath -Label 'image metadata'
  $release = Get-ReleaseManifestDocument -Path $releasePath
  $lock = Assert-InputLockSchema -Path $lockPath
  $proof = Read-JsonEvidence -Path $proofPath -Label 'platform-kernel proof'
  $metadata = Read-JsonEvidence -Path $metadataPath -Label 'image metadata'
  Assert-PlatformKernelProof -Proof $proof
  if ([int]$metadata.schema_version -ne 1) { throw 'image metadata identity is invalid' }

  if (-not $SkipRepositoryCheck) { Assert-ReleaseRepositories -Release $release -Lock $lock }

  $backendContractPath = Join-Path $script:BackendRoot 'contracts\admin\v1\manifest.json'
  $frontendContractPath = Join-Path $script:FrontendRoot 'contracts\backend\admin\v1\manifest.json'
  $frontendLockPath = Join-Path $script:FrontendRoot 'contracts\backend\admin\lock.json'
  $backendContract = Read-JsonEvidence -Path $backendContractPath -Label 'backend contract manifest'
  $frontendLock = Read-JsonEvidence -Path $frontendLockPath -Label 'frontend contract lock'
  $contractDigest = Get-FileSha256 -Path $backendContractPath
  Assert-ExactString (Get-FileSha256 -Path $frontendContractPath) $contractDigest 'frontend contract manifest digest'
  Assert-ExactString ([string]$release.contract.bundle_version) ([string]$backendContract.bundle_version) 'contract bundle version'
  Assert-ExactString ([string]$release.contract.manifest_sha256) $contractDigest 'contract manifest digest'
  Assert-ExactString ([string]$frontendLock.bundle_version) ([string]$backendContract.bundle_version) 'frontend bundle version lock'
  Assert-ExactString ([string]$frontendLock.manifest_sha256) $contractDigest 'frontend manifest lock'
  Assert-ExactString ([string]$frontendLock.backend_commit) ([string]$backendContract.backend_commit) 'frontend backend-source lock'

  Assert-ExactString ([string]$proof.backend_commit) ([string]$release.backend.commit) 'platform proof backend commit'
  Assert-ExactString ([string]$proof.frontend_commit) ([string]$release.frontend.commit) 'platform proof frontend commit'
  Assert-ExactString ([string]$proof.contract_manifest_sha256) $contractDigest 'platform proof contract digest'
  Assert-ExactString ([string]$proof.bundle_version) ([string]$backendContract.bundle_version) 'platform proof bundle version'
  Assert-ExactString ([string]$proof.atlas_version) ([string]$release.database.atlas_version) 'platform proof Atlas version'
  Assert-ExactString ([string]$proof.target_fingerprint_sha256) ([string]$release.database.target_fingerprint_sha256) 'platform proof target fingerprint'
  Assert-ExactString (Get-FileSha256 -Path (Join-Path $script:BackendRoot 'database\migrations\atlas.sum')) ([string]$release.database.atlas_sum_sha256) 'Atlas sum digest'

  $evidence = [ordered]@{
    input_lock_sha256 = Get-FileSha256 -Path $lockPath
    query_sha256 = [string]$lock.query_evidence_sha256
    cos_disposition_sha256 = [string]$lock.cos_disposition_evidence_sha256
    recovery_sha256 = [string]$lock.recovery_artifact_sha256
    browser_only_retirement_sha256 = [string]$lock.client_versions_freeze_evidence_sha256
    platform_kernel_sha256 = Get-FileSha256 -Path $proofPath
  }
  foreach ($entry in $evidence.GetEnumerator()) {
    Assert-ExactString ([string]$release.evidence.($entry.Key)) ([string]$entry.Value) "release evidence $($entry.Key)"
  }

  Assert-ExactString ([string]$metadata.backend.commit) ([string]$release.backend.commit) 'backend image metadata commit'
  Assert-ExactString ([string]$metadata.frontend.commit) ([string]$release.frontend.commit) 'frontend image metadata commit'
  if ([string]::IsNullOrWhiteSpace($DockerExecutable) -and -not $SkipImageInspection) {
    $DockerExecutable = Resolve-ReleaseDocker -Command 'docker'
  }
  Assert-ReleaseImage -ManifestEntry $release.backend -MetadataEntry $metadata.backend -MetadataPath $metadataPath -DockerExecutable $DockerExecutable -Label 'backend' -SkipImageInspection:$SkipImageInspection
  Assert-ReleaseImage -ManifestEntry $release.frontend -MetadataEntry $metadata.frontend -MetadataPath $metadataPath -DockerExecutable $DockerExecutable -Label 'frontend' -SkipImageInspection:$SkipImageInspection

  return [pscustomobject]@{
    Path = $releasePath
    Document = $release
    InputLock = $lock
    PlatformProof = $proof
    ImageMetadata = $metadata
  }
}

function Invoke-ReleaseManifestSchemaSelfTest {
  $valid = [ordered]@{
    schema_version = 1
    release_id = 'admin-v2026.07.21.1'
    backend = [ordered]@{ commit = 'a' * 40; image = 'admin-go-backend@sha256:' + ('b' * 64); archive_sha256 = 'c' * 64 }
    frontend = [ordered]@{ commit = 'd' * 40; image = 'admin-frontend@sha256:' + ('e' * 64); archive_sha256 = 'f' * 64 }
    contract = [ordered]@{ bundle_version = 'admin-2026-07-15.2'; manifest_sha256 = '0' * 64 }
    database = [ordered]@{ atlas_version = '202607150203'; target_fingerprint_sha256 = '1' * 64; atlas_sum_sha256 = '2' * 64 }
    evidence = [ordered]@{
      input_lock_sha256 = '3' * 64
      query_sha256 = '4' * 64
      cos_disposition_sha256 = '5' * 64
      recovery_sha256 = '6' * 64
      browser_only_retirement_sha256 = '7' * 64
      platform_kernel_sha256 = '8' * 64
    }
  }
  $validJSON = $valid | ConvertTo-Json -Depth 8 -Compress
  if (-not ($validJSON | Test-Json -SchemaFile $script:ReleaseManifestSchema)) { throw 'release manifest schema rejected its valid self-test' }
  $invalid = [ordered]@{} + $valid
  $invalid.extra = $true
  if (($invalid | ConvertTo-Json -Depth 8 -Compress) | Test-Json -SchemaFile $script:ReleaseManifestSchema -ErrorAction SilentlyContinue) {
    throw 'release manifest schema accepted an additional property'
  }
  $invalid = [ordered]@{} + $valid
  $invalid.backend = [ordered]@{} + $valid.backend
  $invalid.backend.image = 'admin-go-backend:latest'
  if (($invalid | ConvertTo-Json -Depth 8 -Compress) | Test-Json -SchemaFile $script:ReleaseManifestSchema -ErrorAction SilentlyContinue) {
    throw 'release manifest schema accepted a mutable image tag'
  }
}

if ($requestedImportFunctions) { return }

if ($requestedSchemaOnly) {
  Invoke-ReleaseManifestSchemaSelfTest
  Write-Output 'release manifest schema check passed'
  return
}

if ([string]::IsNullOrWhiteSpace($Manifest)) { $Manifest = $script:DefaultReleaseManifest }
if ([string]::IsNullOrWhiteSpace($InputLock)) { $InputLock = $script:DefaultInputLock }
if ([string]::IsNullOrWhiteSpace($PlatformKernelProof)) { $PlatformKernelProof = $script:DefaultPlatformKernelProof }
if ([string]::IsNullOrWhiteSpace($ImageMetadata)) { $ImageMetadata = $script:DefaultImageMetadata }
$dockerExecutable = Resolve-ReleaseDocker -Command $DockerCommand
[void](Assert-ReleaseManifest -ManifestPath $Manifest -InputLockPath $InputLock -PlatformKernelProofPath $PlatformKernelProof -ImageMetadataPath $ImageMetadata -DockerExecutable $dockerExecutable)
Write-Output 'release manifest check passed'
