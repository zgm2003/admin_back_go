[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)][string]$RecoveryArtifact,
  [Parameter(Mandatory = $true)][string]$CosDispositionEvidence,
  [Parameter(Mandatory = $true)][string]$QueryEvidence,
  [string]$ClientVersionsFreezeEvidence,
  [string]$OutputPath,
  [switch]$CheckOnly
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$backendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $backendRoot '..\admin_front_ts'))
$checkScript = Join-Path $PSScriptRoot 'check-inputs.ps1'
. $checkScript `
  -ImportFunctions `
  -RecoveryArtifact $RecoveryArtifact `
  -CosDispositionEvidence $CosDispositionEvidence `
  -QueryEvidence $QueryEvidence `
  -ClientVersionsFreezeEvidence $ClientVersionsFreezeEvidence

if ([string]::IsNullOrWhiteSpace($ClientVersionsFreezeEvidence)) {
  $ClientVersionsFreezeEvidence = Join-Path $backendRoot 'docs\runbooks\admin-browser-only-cutover.md'
}
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
  $OutputPath = Join-Path $backendRoot 'release\admin-only\input-lock.json'
}
$outputPath = [IO.Path]::GetFullPath($OutputPath)
$expectedOutput = [IO.Path]::GetFullPath((Join-Path $backendRoot 'release\admin-only\input-lock.json'))
if (-not $outputPath.Equals($expectedOutput, [StringComparison]::OrdinalIgnoreCase)) {
  throw 'input lock output path is fixed by the release contract'
}

if ($CheckOnly) {
  & pwsh -NoProfile -File $checkScript `
    -LockPath $outputPath `
    -RecoveryArtifact $RecoveryArtifact `
    -CosDispositionEvidence $CosDispositionEvidence `
    -QueryEvidence $QueryEvidence `
    -ClientVersionsFreezeEvidence $ClientVersionsFreezeEvidence
  if ($LASTEXITCODE -ne 0) { throw 'release input lock check failed' }
  return
}

$allowedBackendPaths = @(
  'release/admin-only/input-lock.json',
  'release/admin-only/input-lock.schema.json',
  'release/admin-only/.gitignore',
  'scripts/release/lock-inputs.ps1',
  'scripts/release/check-inputs.ps1',
  'scripts/tests/release-input-lock.tests.ps1',
  'docs/runbooks/admin-only-data-disposition.md'
)

Assert-SingleWorktree -Repository $backendRoot
Assert-SingleWorktree -Repository $frontendRoot
Assert-RepositoryStatus -Repository $backendRoot -AllowedPaths $allowedBackendPaths
Assert-RepositoryStatus -Repository $frontendRoot

$recoveryFile = Assert-ExternalEvidencePath -Path $RecoveryArtifact -Label 'recovery artifact'
$cosFile = Assert-ExternalEvidencePath -Path $CosDispositionEvidence -Label 'COS disposition evidence'
$queryFile = Assert-ExternalEvidencePath -Path $QueryEvidence -Label 'query evidence'
$freezeFile = Get-RequiredFilePath -Path $ClientVersionsFreezeEvidence -Label 'client-version freeze evidence'
$expectedFreeze = [IO.Path]::GetFullPath((Join-Path $backendRoot 'docs\runbooks\admin-browser-only-cutover.md'))
if (-not $freezeFile.Equals($expectedFreeze, [StringComparison]::OrdinalIgnoreCase)) {
  throw 'client-version freeze evidence must be the committed P08R runbook'
}

Assert-ExternalEvidenceBundle `
  -RecoveryPath $recoveryFile `
  -COSPath $cosFile `
  -QueryPath $queryFile `
  -FreezePath $freezeFile

$backendCommit = Assert-GitSha -Value (Get-RepositoryCommit -Repository $backendRoot) -Label 'backend commit'
$frontendCommit = Assert-GitSha -Value (Get-RepositoryCommit -Repository $frontendRoot) -Label 'frontend commit'
$manifestPath = Get-RequiredFilePath -Path (Join-Path $backendRoot 'contracts\admin\v1\manifest.json') -Label 'Admin contract manifest'

$lock = [ordered]@{
  schema_version = 1
  backend_commit = $backendCommit
  frontend_commit = $frontendCommit
  contract_manifest_sha256 = Get-FileSha256 -Path $manifestPath
  recovery_artifact_sha256 = Get-FileSha256 -Path $recoveryFile
  cos_disposition_evidence_sha256 = Get-FileSha256 -Path $cosFile
  query_evidence_sha256 = Get-FileSha256 -Path $queryFile
  client_versions_freeze_evidence_sha256 = Get-FileSha256 -Path $freezeFile
}

$json = $lock | ConvertTo-Json -Depth 3
if (-not ($json | Test-Json -SchemaFile (Join-Path $backendRoot 'release\admin-only\input-lock.schema.json'))) {
  throw 'generated input lock does not match its schema'
}
if (Test-Path -LiteralPath $outputPath) {
  throw 'input lock already exists; use -CheckOnly instead of rewriting frozen inputs'
}

$outputDirectory = Split-Path -Parent $outputPath
if (-not (Test-Path -LiteralPath $outputDirectory -PathType Container)) {
  throw 'input lock output directory is missing'
}
$temporaryPath = Join-Path $outputDirectory ('.input-lock.' + [guid]::NewGuid().ToString('N') + '.tmp')
try {
  [IO.File]::WriteAllText($temporaryPath, $json + "`n", [Text.UTF8Encoding]::new($false))
  Move-Item -LiteralPath $temporaryPath -Destination $outputPath -Force
} finally {
  if (Test-Path -LiteralPath $temporaryPath) {
    Remove-Item -LiteralPath $temporaryPath -Force
  }
}

Write-Output 'release input lock created'
