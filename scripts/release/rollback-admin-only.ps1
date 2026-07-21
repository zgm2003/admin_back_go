[CmdletBinding()]
param(
  [switch]$ImportFunctions,
  [switch]$Apply,
  [switch]$FullDatabaseRollback,
  [switch]$MaintenanceWindow,
  [string]$PreviousManifest,
  [string]$PreviousPlatformKernelProof,
  [string]$PreviousImageMetadata,
  [string]$ImageArchiveDirectory,
  [string]$RecoveryArtifact,
  [string]$RecoveryRehearsalEvidence,
  [string]$Database,
  [string]$ExpectedRecoveryFingerprint,
  [string]$BackendEnvFile,
  [string]$RuntimeVolume,
  [string]$ExportVolume,
  [string]$PlatformNetwork = 'admin-platform',
  [string]$ProductionProject = 'admin-app',
  [ValidateRange(1024, 65535)][int]$FrontendPort = 5173,
  [ValidateRange(1024, 65535)][int]$APIPort = 8080,
  [ValidateRange(60, 7200)][int]$RestoreTimeoutSeconds = 1800,
  [string]$MySQLCommand = 'mysql',
  [string]$DockerCommand = 'docker'
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$ProgressPreference = 'SilentlyContinue'
Set-StrictMode -Version Latest

$rollbackWasImported = [bool]$ImportFunctions
$rollbackRequest = [pscustomobject]@{
  Apply = [bool]$Apply
  FullDatabaseRollback = [bool]$FullDatabaseRollback
  MaintenanceWindow = [bool]$MaintenanceWindow
  PreviousManifest = $PreviousManifest
  PreviousPlatformKernelProof = $PreviousPlatformKernelProof
  PreviousImageMetadata = $PreviousImageMetadata
  ImageArchiveDirectory = $ImageArchiveDirectory
  RecoveryArtifact = $RecoveryArtifact
  RecoveryRehearsalEvidence = $RecoveryRehearsalEvidence
  Database = $Database
  ExpectedRecoveryFingerprint = $ExpectedRecoveryFingerprint
  BackendEnvFile = $BackendEnvFile
  RuntimeVolume = $RuntimeVolume
  ExportVolume = $ExportVolume
  PlatformNetwork = $PlatformNetwork
  ProductionProject = $ProductionProject
  FrontendPort = $FrontendPort
  APIPort = $APIPort
  RestoreTimeoutSeconds = $RestoreTimeoutSeconds
  MySQLCommand = $MySQLCommand
  DockerCommand = $DockerCommand
}
. (Join-Path $PSScriptRoot 'deploy-admin-only.ps1') -ImportFunctions

function Get-RollbackArchivePath {
  param(
    [Parameter(Mandatory = $true)][string]$ArchiveRoot,
    [Parameter(Mandatory = $true)]$MetadataEntry,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $name = [string]$MetadataEntry.archive_file
  if ([string]::IsNullOrWhiteSpace($name) -or $name -cne [IO.Path]::GetFileName($name) -or $name -notmatch '\.tar$') {
    throw "$Label rollback archive name is invalid"
  }
  $path = [IO.Path]::GetFullPath((Join-Path $ArchiveRoot $name))
  if (-not (Test-PathWithin -Candidate $path -Parent $ArchiveRoot)) { throw "$Label rollback archive escaped its directory" }
  return Get-RequiredFilePath -Path $path -Label "$Label rollback archive"
}

function Import-RollbackImages {
  param(
    [Parameter(Mandatory = $true)]$Release,
    [Parameter(Mandatory = $true)]$Metadata,
    [Parameter(Mandatory = $true)][string]$ArchiveRoot
  )
  foreach ($pair in @(
    [pscustomobject]@{ Label = 'backend'; Manifest = $Release.backend; Metadata = $Metadata.backend },
    [pscustomobject]@{ Label = 'frontend'; Manifest = $Release.frontend; Metadata = $Metadata.frontend }
  )) {
    Assert-ExactString ([string]$pair.Metadata.commit) ([string]$pair.Manifest.commit) "$($pair.Label) rollback commit"
    Assert-ExactString ([string]$pair.Metadata.image) ([string]$pair.Manifest.image) "$($pair.Label) rollback image"
    Assert-ExactString ([string]$pair.Metadata.archive_sha256) ([string]$pair.Manifest.archive_sha256) "$($pair.Label) rollback metadata archive digest"
    $imageID = Get-ReleaseImageID -Image ([string]$pair.Manifest.image)
    Assert-ExactString ([string]$pair.Metadata.image_id) $imageID "$($pair.Label) rollback image ID"
    $archivePath = Get-RollbackArchivePath -ArchiveRoot $ArchiveRoot -MetadataEntry $pair.Metadata -Label $pair.Label
    Assert-ExactString (Get-FileSha256 -Path $archivePath) ([string]$pair.Manifest.archive_sha256) "$($pair.Label) rollback archive digest"
    # docker load restores the previously exported image without rebuilding it.
    [void](Invoke-ReleaseDocker -Arguments @('load', '--input', $archivePath) -Label "docker load previous $($pair.Label)" -Capture)
    $inspection = Get-DockerImageInspection -DockerExecutable $script:ReleaseDockerExecutable -ImageID $imageID -Label "previous $($pair.Label)"
    Assert-ExactString ([string]$inspection.Id) $imageID "$($pair.Label) rollback loaded image"
    Assert-ExactString ([string]$inspection.Config.Labels.'org.opencontainers.image.revision') ([string]$pair.Manifest.commit) "$($pair.Label) rollback revision"
  }
}

function Invoke-RecoveryDumpRestore {
  param(
    [Parameter(Mandatory = $true)][string]$ArtifactPath,
    [Parameter(Mandatory = $true)][string]$RehearsalPath,
    [Parameter(Mandatory = $true)][string]$TargetDatabase,
    [Parameter(Mandatory = $true)][string]$ExpectedFingerprint,
    [Parameter(Mandatory = $true)][string]$MySQLExecutable,
    [Parameter(Mandatory = $true)][int]$TimeoutSeconds,
    [Parameter(Mandatory = $true)][string]$ExpectedArtifactSHA
  )
  if ($ExpectedFingerprint -cnotmatch '^[0-9a-f]{64}$') { throw 'expected recovery fingerprint is invalid' }
  Assert-ExactString (Get-FileSha256 -Path $ArtifactPath) $ExpectedArtifactSHA 'rollback recovery artifact digest'
  [void](Assert-RecoveryArtifact -Path $ArtifactPath)
  $artifact = Read-JsonEvidence -Path $ArtifactPath -Label 'rollback recovery artifact'
  $rehearsal = Read-JsonEvidence -Path $RehearsalPath -Label 'recovery rehearsal evidence'
  if ($RehearsalPath.Equals($ArtifactPath, [StringComparison]::OrdinalIgnoreCase)) {
    if ($rehearsal.verified -ne $true) { throw 'recovery rehearsal evidence did not pass' }
  } elseif ($rehearsal.passed -ne $true -or [string]$rehearsal.recovery_artifact_sha256 -cne $ExpectedArtifactSHA) {
    throw 'recovery rehearsal evidence does not match the locked artifact'
  }

  $dumpPath = Get-RequiredFilePath -Path ([string]$artifact.dump_path) -Label 'rollback recovery dump'
  Assert-ExactString (Get-FileSha256 -Path $dumpPath) ([string]$artifact.dump_sha256) 'rollback recovery dump digest'
  $settings = Get-MySQLDSNSettings -Database $TargetDatabase
  try {
    # Full database rollback restores the verified artifact; it never synthesizes reverse DDL.
    $dropAndCreate = "DROP DATABASE IF EXISTS ``$TargetDatabase``; CREATE DATABASE ``$TargetDatabase`` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
    [void](Invoke-MySQLStatement -MySQLExecutable $MySQLExecutable -Settings $settings -SQL $dropAndCreate)

    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $MySQLExecutable
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.Environment['MYSQL_PWD'] = [string]$settings.Password
    foreach ($argument in @(
      '--protocol=tcp', "--host=$($settings.Host)", "--port=$($settings.Port)",
      "--user=$($settings.User)", "--database=$TargetDatabase", '--binary-mode', '--default-character-set=utf8mb4'
    )) { [void]$startInfo.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    $dumpStream = $null
    try {
      if (-not $process.Start()) { throw 'recovery restore process did not start' }
      $stdoutTask = $process.StandardOutput.ReadToEndAsync()
      $stderrTask = $process.StandardError.ReadToEndAsync()
      $dumpStream = [IO.File]::OpenRead($dumpPath)
      $dumpStream.CopyTo($process.StandardInput.BaseStream)
      $process.StandardInput.Close()
      if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
        try { $process.Kill($true) } catch { }
        [void]$process.WaitForExit(5000)
        throw 'recovery restore timed out'
      }
      [void]$stdoutTask.GetAwaiter().GetResult()
      [void]$stderrTask.GetAwaiter().GetResult()
      if ($process.ExitCode -ne 0) { throw 'recovery restore failed' }
    } finally {
      if ($null -ne $dumpStream) { $dumpStream.Dispose() }
      $process.Dispose()
    }

    foreach ($property in @($artifact.source_counts.PSObject.Properties)) {
      if ($property.Name -cnotmatch '^[a-z][a-z0-9_]*$') { throw 'recovery artifact contains an invalid count table' }
      $actual = (Invoke-MySQLStatement -MySQLExecutable $MySQLExecutable -Settings $settings -SQL "SELECT COUNT(*) FROM ``$TargetDatabase``.``$($property.Name)``;").Trim()
      if ($actual -notmatch '^[0-9]+$' -or [long]$actual -ne [long]$property.Value) {
        throw "recovery row count mismatch: $($property.Name)"
      }
    }
    $fingerprint = Get-DatabaseFingerprintSHA -BackendRoot $script:BackendRoot -Settings $settings -Database $TargetDatabase
    Assert-ExactString $fingerprint $ExpectedFingerprint 'restored database fingerprint'
  } finally {
    $settings.Password = $null
  }
}

if ($rollbackWasImported) { return }

$Apply = $rollbackRequest.Apply
$FullDatabaseRollback = $rollbackRequest.FullDatabaseRollback
$MaintenanceWindow = $rollbackRequest.MaintenanceWindow
$PreviousManifest = $rollbackRequest.PreviousManifest
$PreviousPlatformKernelProof = $rollbackRequest.PreviousPlatformKernelProof
$PreviousImageMetadata = $rollbackRequest.PreviousImageMetadata
$ImageArchiveDirectory = $rollbackRequest.ImageArchiveDirectory
$RecoveryArtifact = $rollbackRequest.RecoveryArtifact
$RecoveryRehearsalEvidence = $rollbackRequest.RecoveryRehearsalEvidence
$Database = $rollbackRequest.Database
$ExpectedRecoveryFingerprint = $rollbackRequest.ExpectedRecoveryFingerprint
$BackendEnvFile = $rollbackRequest.BackendEnvFile
$RuntimeVolume = $rollbackRequest.RuntimeVolume
$ExportVolume = $rollbackRequest.ExportVolume
$PlatformNetwork = $rollbackRequest.PlatformNetwork
$ProductionProject = $rollbackRequest.ProductionProject
$FrontendPort = $rollbackRequest.FrontendPort
$APIPort = $rollbackRequest.APIPort
$RestoreTimeoutSeconds = $rollbackRequest.RestoreTimeoutSeconds
$MySQLCommand = $rollbackRequest.MySQLCommand
$DockerCommand = $rollbackRequest.DockerCommand

if (-not $Apply) { throw 'Admin release rollback requires explicit -Apply' }
foreach ($required in @(
  [pscustomobject]@{ Value = $BackendEnvFile; Label = 'backend environment file' },
  [pscustomobject]@{ Value = $RuntimeVolume; Label = 'runtime volume' },
  [pscustomobject]@{ Value = $ExportVolume; Label = 'export volume' }
)) {
  if ([string]::IsNullOrWhiteSpace([string]$required.Value)) { throw "$($required.Label) is required" }
}
foreach ($name in @($RuntimeVolume, $ExportVolume, $PlatformNetwork, $ProductionProject)) {
  if ($name -cnotmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$') { throw 'rollback Compose resource name is invalid' }
}

$statePath = Join-Path $script:ReleaseOutputRoot 'deployment-state.json'
$state = Read-JsonEvidence -Path (Get-RequiredFilePath -Path $statePath -Label 'deployment state') -Label 'deployment state'
if ([string]::IsNullOrWhiteSpace($PreviousManifest)) { $PreviousManifest = [string]$state.previous_manifest }
if ([string]::IsNullOrWhiteSpace($PreviousPlatformKernelProof)) { $PreviousPlatformKernelProof = [string]$state.previous_platform_proof }
if ([string]::IsNullOrWhiteSpace($PreviousImageMetadata)) { $PreviousImageMetadata = [string]$state.previous_image_metadata }
if ([string]::IsNullOrWhiteSpace($PreviousManifest) -or [string]::IsNullOrWhiteSpace($PreviousPlatformKernelProof) -or [string]::IsNullOrWhiteSpace($PreviousImageMetadata)) {
  throw 'deployment state has no previous release package'
}
$manifestPath = Get-RequiredFilePath -Path $PreviousManifest -Label 'previous release manifest'
$proofPath = Get-RequiredFilePath -Path $PreviousPlatformKernelProof -Label 'previous platform-kernel proof'
$metadataPath = Get-RequiredFilePath -Path $PreviousImageMetadata -Label 'previous image metadata'
foreach ($path in @($manifestPath, $proofPath, $metadataPath)) {
  if (-not (Test-PathWithin -Candidate $path -Parent $script:ReleaseOutputRoot)) { throw 'previous release package must remain inside release output' }
}
if (-not [string]::IsNullOrWhiteSpace([string]$state.previous_manifest_sha256)) {
  Assert-ExactString (Get-FileSha256 -Path $manifestPath) ([string]$state.previous_manifest_sha256) 'previous manifest state digest'
}
$release = Get-ReleaseManifestDocument -Path $manifestPath
$proof = Read-JsonEvidence -Path $proofPath -Label 'previous platform-kernel proof'
$metadata = Read-JsonEvidence -Path $metadataPath -Label 'previous image metadata'
Assert-PlatformKernelProof -Proof $proof
if ([int]$metadata.schema_version -ne 1) { throw 'previous image metadata identity is invalid' }
Assert-ExactString (Get-FileSha256 -Path $proofPath) ([string]$release.evidence.platform_kernel_sha256) 'previous platform proof digest'
Assert-ExactString ([string]$proof.backend_commit) ([string]$release.backend.commit) 'previous proof backend commit'
Assert-ExactString ([string]$proof.frontend_commit) ([string]$release.frontend.commit) 'previous proof frontend commit'

if ([string]::IsNullOrWhiteSpace($ImageArchiveDirectory)) { $ImageArchiveDirectory = Join-Path $script:ReleaseOutputRoot 'images' }
$archiveRoot = [IO.Path]::GetFullPath($ImageArchiveDirectory)
if (-not (Test-Path -LiteralPath $archiveRoot -PathType Container)) { throw 'rollback image archive directory is missing' }
$envFile = Get-RequiredFilePath -Path $BackendEnvFile -Label 'backend environment file'
$script:ReleaseDockerExecutable = Resolve-ReleaseDocker -Command $DockerCommand
$script:ReleaseComposePath = Join-Path $script:BackendRoot 'deploy\admin-only\docker-compose.yml'
Assert-NoLiveAdminDevLock -Path (Join-Path $script:BackendRoot '.tmp\dev\admin-dev.lock.json') -RepositoryRoot $script:BackendRoot
Assert-SingleWorktree -Repository $script:BackendRoot
Assert-SingleWorktree -Repository $script:FrontendRoot
Assert-RepositoryStatus -Repository $script:BackendRoot
Assert-RepositoryStatus -Repository $script:FrontendRoot
Import-RollbackImages -Release $release -Metadata $metadata -ArchiveRoot $archiveRoot

$validation = [pscustomobject]@{ Document = $release; ImageMetadata = $metadata }
$currentProject = [string]$state.current_project
if ([string]::IsNullOrWhiteSpace($currentProject)) { $currentProject = $ProductionProject }
$previousProject = [string]$state.previous_project
if ([string]::IsNullOrWhiteSpace($previousProject)) { $previousProject = $ProductionProject }
$environmentNames = @(
  'ADMIN_FRONTEND_IMAGE', 'ADMIN_BACKEND_IMAGE', 'ADMIN_FRONTEND_REVISION', 'ADMIN_BACKEND_REVISION',
  'ADMIN_RELEASE_ID', 'ADMIN_BACKEND_ENV_FILE', 'ADMIN_RUNTIME_VOLUME', 'ADMIN_EXPORT_VOLUME',
  'ADMIN_PLATFORM_NETWORK', 'ADMIN_FRONTEND_BIND_ADDRESS', 'ADMIN_API_BIND_ADDRESS',
  'ADMIN_FRONTEND_PORT', 'ADMIN_API_PORT'
)
$previousEnvironment = @{}
foreach ($name in $environmentNames) { $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

try {
  Set-AdminReleaseComposeEnvironment -Validation $validation -EnvFile $envFile -RuntimeVolumeName $RuntimeVolume -ExportVolumeName $ExportVolume -NetworkName $PlatformNetwork -FrontendPort $FrontendPort -APIPort $APIPort
  Invoke-AdminReleaseCompose -Project $currentProject -Arguments @('stop') -Label 'stop current release for rollback'

  if ($FullDatabaseRollback) {
    if (-not $MaintenanceWindow) { throw 'full database rollback requires an approved maintenance window' }
    foreach ($required in @(
      [pscustomobject]@{ Value = $RecoveryArtifact; Label = 'locked recovery artifact' },
      [pscustomobject]@{ Value = $RecoveryRehearsalEvidence; Label = 'recovery rehearsal evidence' },
      [pscustomobject]@{ Value = $Database; Label = 'rollback database' },
      [pscustomobject]@{ Value = $ExpectedRecoveryFingerprint; Label = 'expected recovery fingerprint' }
    )) {
      if ([string]::IsNullOrWhiteSpace([string]$required.Value)) { throw "$($required.Label) is required for full database rollback" }
    }
    if ($Database -cnotmatch '^[A-Za-z][A-Za-z0-9_]{0,63}$') { throw 'rollback database name is invalid' }
    $artifactPath = Assert-ExternalEvidencePath -Path $RecoveryArtifact -Label 'locked recovery artifact'
    $rehearsalPath = Assert-ExternalEvidencePath -Path $RecoveryRehearsalEvidence -Label 'recovery rehearsal evidence'
    $mysqlExecutable = (Get-Command $MySQLCommand -ErrorAction Stop | Select-Object -First 1).Source
    Invoke-RecoveryDumpRestore -ArtifactPath $artifactPath -RehearsalPath $rehearsalPath -TargetDatabase $Database -ExpectedFingerprint $ExpectedRecoveryFingerprint -MySQLExecutable $mysqlExecutable -TimeoutSeconds $RestoreTimeoutSeconds -ExpectedArtifactSHA ([string]$release.evidence.recovery_sha256)
  }

  Invoke-AdminReleaseCompose -Project $previousProject -Arguments @('up', '-d', '--no-build', '--force-recreate', '--wait', '--wait-timeout', '300') -Label 'restore previous release project'
  Invoke-AdminReleaseSmoke -FrontendURL "http://127.0.0.1:$FrontendPort" -APIURL "http://127.0.0.1:$APIPort"

  $nextState = [ordered]@{
    schema_version = 1
    current_manifest = $manifestPath
    current_manifest_sha256 = Get-FileSha256 -Path $manifestPath
    current_platform_proof = $proofPath
    current_image_metadata = $metadataPath
    current_project = $previousProject
    previous_manifest = [string]$state.current_manifest
    previous_manifest_sha256 = [string]$state.current_manifest_sha256
    previous_platform_proof = [string]$state.current_platform_proof
    previous_image_metadata = [string]$state.current_image_metadata
    previous_project = $currentProject
  }
  $temporaryPath = Join-Path (Split-Path -Parent $statePath) ('.deployment-state.' + [guid]::NewGuid().ToString('N') + '.tmp')
  try {
    [IO.File]::WriteAllText($temporaryPath, ($nextState | ConvertTo-Json -Depth 5) + "`n", [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporaryPath -Destination $statePath -Force
  } finally {
    if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
  }
} finally {
  foreach ($name in $environmentNames) { [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process') }
}

Write-Output 'Admin-only release rollback completed'
