[CmdletBinding()]
param(
  [switch]$SchemaOnly,
  [switch]$ImportFunctions,
  [string]$LockPath,
  [string]$DatabaseFingerprint,
  [string]$RecoveryArtifact,
  [string]$CosDispositionEvidence,
  [string]$QueryEvidence,
  [string]$ClientVersionsFreezeEvidence
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$script:BackendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$script:FrontendRoot = [IO.Path]::GetFullPath((Join-Path $script:BackendRoot '..\admin_front_ts'))
$script:InputLockSchema = Join-Path $script:BackendRoot 'release\admin-only\input-lock.schema.json'
$script:DefaultInputLock = Join-Path $script:BackendRoot 'release\admin-only\input-lock.json'
$script:DefaultFreezeEvidence = Join-Path $script:BackendRoot 'docs\runbooks\admin-browser-only-cutover.md'

function Assert-Condition {
  param([bool]$Condition, [string]$Message)
  if (-not $Condition) { throw $Message }
}

function Get-RequiredFilePath {
  param([Parameter(Mandatory = $true)][string]$Path, [string]$Label = 'evidence')
  if ([string]::IsNullOrWhiteSpace($Path)) { throw "$Label path is required" }
  $resolved = Resolve-Path -LiteralPath $Path -ErrorAction Stop
  if (-not (Test-Path -LiteralPath $resolved.Path -PathType Leaf)) { throw "$Label file is missing" }
  return [IO.Path]::GetFullPath($resolved.Path)
}

function Test-PathWithin {
  param([Parameter(Mandatory = $true)][string]$Candidate, [Parameter(Mandatory = $true)][string]$Parent)
  $candidatePath = [IO.Path]::GetFullPath($Candidate).TrimEnd('\', '/')
  $parentPath = [IO.Path]::GetFullPath($Parent).TrimEnd('\', '/')
  return $candidatePath.Equals($parentPath, [StringComparison]::OrdinalIgnoreCase) -or
    $candidatePath.StartsWith($parentPath + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)
}

function Assert-ExternalEvidencePath {
  param([Parameter(Mandatory = $true)][string]$Path, [Parameter(Mandatory = $true)][string]$Label)
  $resolved = Get-RequiredFilePath -Path $Path -Label $Label
  if ((Test-PathWithin -Candidate $resolved -Parent $script:BackendRoot) -or
      (Test-PathWithin -Candidate $resolved -Parent $script:FrontendRoot)) {
    throw "$Label must remain outside both repositories"
  }
  return $resolved
}

function Get-FileSha256 {
  param([Parameter(Mandatory = $true)][string]$Path)
  return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Assert-GitSha {
  param([Parameter(Mandatory = $true)][string]$Value, [string]$Label = 'Git SHA')
  $normalized = $Value.Trim()
  if ($normalized -cnotmatch '^[0-9a-f]{40}$') { throw "$Label must be a full lowercase Git SHA" }
  return $normalized
}

function Invoke-GitLines {
  param([Parameter(Mandatory = $true)][string]$Repository, [Parameter(Mandatory = $true)][string[]]$Arguments)
  $output = @(& git -C $Repository @Arguments 2>&1)
  if ($LASTEXITCODE -ne 0) { throw "Git command failed for repository $(Split-Path -Leaf $Repository)" }
  return @($output | ForEach-Object { $_.ToString() })
}

function Get-RepositoryCommit {
  param([Parameter(Mandatory = $true)][string]$Repository)
  $lines = @(Invoke-GitLines -Repository $Repository -Arguments @('rev-parse', 'HEAD'))
  if ($lines.Count -ne 1) { throw 'Git commit lookup returned an unexpected result' }
  return Assert-GitSha -Value $lines[0] -Label 'repository commit'
}

function Assert-SingleWorktree {
  param([Parameter(Mandatory = $true)][string]$Repository)
  # Contract: git worktree list --porcelain must describe one primary checkout only.
  $lines = @(Invoke-GitLines -Repository $Repository -Arguments @('worktree', 'list', '--porcelain'))
  $worktreeLines = @($lines | Where-Object { $_.StartsWith('worktree ', [StringComparison]::Ordinal) })
  if ($worktreeLines.Count -ne 1) { throw 'secondary Git worktree registration is forbidden' }
  $actual = [IO.Path]::GetFullPath($worktreeLines[0].Substring('worktree '.Length).Replace('/', '\'))
  $expected = [IO.Path]::GetFullPath($Repository)
  if (-not $actual.Equals($expected, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Git primary worktree path does not match the declared repository'
  }
  if (Test-Path -LiteralPath (Join-Path $Repository '.worktrees')) { throw '.worktrees directories are forbidden' }
}

function Assert-RepositoryStatus {
  param(
    [Parameter(Mandatory = $true)][string]$Repository,
    [string[]]$AllowedPaths = @()
  )
  # Contract: git status --porcelain=v1 --untracked-files=all is the only status source.
  $lines = @(Invoke-GitLines -Repository $Repository -Arguments @('status', '--porcelain=v1', '--untracked-files=all'))
  $allowed = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach ($path in $AllowedPaths) { [void]$allowed.Add($path.Replace('\', '/')) }
  $unexpected = [Collections.Generic.List[string]]::new()
  foreach ($line in $lines) {
    if ($line.Length -lt 4) { [void]$unexpected.Add('<malformed>'); continue }
    $path = $line.Substring(3).Replace('\', '/')
    $renameIndex = $path.LastIndexOf(' -> ', [StringComparison]::Ordinal)
    if ($renameIndex -ge 0) {
      $path = $path.Substring($renameIndex + 4)
    }
    if (-not $allowed.Contains($path)) { [void]$unexpected.Add($path) }
  }
  if ($unexpected.Count -gt 0) {
    throw "repository contains paths outside the declared release task: $($unexpected -join ', ')"
  }
}

function Read-JsonEvidence {
  param([Parameter(Mandatory = $true)][string]$Path, [Parameter(Mandatory = $true)][string]$Label)
  try {
    return [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8) | ConvertFrom-Json
  } catch {
    throw "$Label is not valid JSON"
  }
}

function Assert-DatabaseFingerprint {
  param([Parameter(Mandatory = $true)][string]$Path)
  $document = Read-JsonEvidence -Path $Path -Label 'database fingerprint evidence'
  $schemaHash = [string]$document.schema_sha256
  if ($schemaHash -cnotmatch '^[0-9a-f]{64}$') { throw 'database fingerprint evidence has no valid schema hash' }
  if ([string]$document.database -cne 'admin') { throw 'database fingerprint evidence must identify admin' }
  return [pscustomobject]@{ SchemaSha256 = $schemaHash }
}

function Assert-RecoveryArtifact {
  param([Parameter(Mandatory = $true)][string]$Path)
  $artifact = Read-JsonEvidence -Path $Path -Label 'recovery artifact'
  if ($artifact.verified -ne $true) { throw 'recovery artifact was not restore-verified' }
  if ([string]$artifact.database -cne 'admin') { throw 'recovery artifact must identify admin' }
  $dumpPath = Get-RequiredFilePath -Path ([string]$artifact.dump_path) -Label 'recovery dump'
  $expectedHash = [string]$artifact.dump_sha256
  if ($expectedHash -cnotmatch '^[0-9a-f]{64}$' -or (Get-FileSha256 -Path $dumpPath) -cne $expectedHash) {
    throw 'recovery dump checksum does not match its artifact'
  }
  if ([long]$artifact.dump_bytes -ne (Get-Item -LiteralPath $dumpPath).Length) {
    throw 'recovery dump byte count does not match its artifact'
  }
  $source = @($artifact.source_counts.PSObject.Properties | Sort-Object Name)
  $restore = @($artifact.restore_counts.PSObject.Properties | Sort-Object Name)
  if ($source.Count -eq 0 -or $source.Count -ne $restore.Count) { throw 'recovery count evidence is incomplete' }
  for ($index = 0; $index -lt $source.Count; $index++) {
    if ($source[$index].Name -cne $restore[$index].Name -or [long]$source[$index].Value -ne [long]$restore[$index].Value) {
      throw 'recovery source and restore counts differ'
    }
  }
  return [pscustomobject]@{ DumpSha256 = $expectedHash }
}

function Assert-QueryEvidence {
  param([Parameter(Mandatory = $true)][string]$Path)
  $entries = @(Read-JsonEvidence -Path $Path -Label 'query evidence')
  if ($entries.Count -eq 0) { throw 'query evidence is empty' }
  $names = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach ($entry in $entries) {
    if ([string]::IsNullOrWhiteSpace([string]$entry.name)) { throw 'query evidence contains an unnamed candidate' }
    if (-not $names.Add([string]$entry.name)) { throw 'query evidence contains duplicate candidates' }
    if ($entry.PSObject.Properties.Name -contains 'accepted') {
      if ($entry.accepted -ne $true -or [long]$entry.before_rows -lt 0 -or
          [long]$entry.after_rows -lt 0 -or [double]$entry.p95_ms -lt 0) {
        throw 'query evidence contains an unaccepted candidate or invalid measurements'
      }
    } elseif ([string]::IsNullOrWhiteSpace([string]$entry.index_name) -or
              [string]::IsNullOrWhiteSpace([string]$entry.table_name) -or
              [string]::IsNullOrWhiteSpace([string]$entry.ddl) -or
              $entry.preexisting -ne $true) {
      throw 'accepted-index evidence is incomplete'
    }
  }
}

function Assert-ClientVersionsFreezeEvidence {
  param([Parameter(Mandatory = $true)][string]$Path)
  $text = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
  $countMatch = [regex]::Match($text, '(?m)^client_versions_count=([0-9]+)$')
  $hashMatch = [regex]::Match($text, '(?m)^client_versions_sha256=([0-9a-f]{64})$')
  if (-not $countMatch.Success -or -not $hashMatch.Success -or
      -not $text.Contains('automated_gate_result=passed', [StringComparison]::Ordinal) -or
      -not $text.Contains('user_acceptance=passed_', [StringComparison]::Ordinal)) {
    throw 'P08R client-version freeze evidence is incomplete'
  }
  return [pscustomobject]@{
    Count = [long]$countMatch.Groups[1].Value
    RowSha256 = $hashMatch.Groups[1].Value
    Text = $text
  }
}

function Assert-COSDispositionEvidence {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)]$FreezeEvidence
  )
  $raw = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
  if ($raw -match '"(key|url|credential|secret|query)"\s*:') {
    throw 'COS disposition evidence must contain hashes and classifications only'
  }
  $document = Read-JsonEvidence -Path $Path -Label 'COS disposition evidence'
  if ([int]$document.schema_version -ne 1 -or [string]$document.database -cne 'admin') {
    throw 'COS disposition evidence identity is invalid'
  }
  if ([string]$document.object_delete_policy -cne 'none') {
    throw 'P09 COS evidence must not authorize object deletion'
  }
  if ([long]$document.client_versions_count -ne [long]$FreezeEvidence.Count -or
      [string]$document.client_versions_sha256 -cne [string]$FreezeEvidence.RowSha256) {
    throw 'COS disposition evidence does not match the frozen client-version history'
  }
  $references = @($document.references)
  if ($references.Count -eq 0 -or [long]$document.reference_count -ne $references.Count) {
    throw 'COS disposition evidence reference count is invalid'
  }
  $identities = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  $retained = 0L
  $retired = 0L
  $missing = 0L
  $clientVersions = 0L
  foreach ($reference in $references) {
    $sha = [string]$reference.reference_sha256
    $source = [string]$reference.source
    $platform = [string]$reference.platform
    $status = [string]$reference.status
    $disposition = [string]$reference.disposition
    if ($sha -cnotmatch '^[0-9a-f]{64}$' -or
        $source -cnotmatch '^(ai_image_file|client_version|export)$' -or
        $platform -cnotmatch '^(admin|canvas|orphan|history)$' -or
        $status -cnotmatch '^(reachable|not_found)$' -or
        $disposition -cnotmatch '^(retain|delete_database_row_preserve_object|record_missing_no_object_delete)$') {
      throw 'COS disposition evidence contains an invalid reference classification'
    }
    if (-not $identities.Add("$source|$sha")) { throw 'COS disposition evidence contains duplicate references' }
    if ($disposition -ceq 'retain') {
      $retained++
      if ($status -cne 'reachable' -or $platform -cne 'admin') {
        throw 'every retained COS reference must be reachable Admin data'
      }
    } else {
      $retired++
    }
    if ($status -ceq 'not_found') { $missing++ }
    if ($source -ceq 'client_version') {
      $clientVersions++
      if ($platform -cne 'history' -or $status -cne 'not_found' -or $disposition -cne 'record_missing_no_object_delete') {
        throw 'client-version COS history must record its observed missing state without a delete operation'
      }
    }
  }
  if ($clientVersions -ne [long]$FreezeEvidence.Count -or
      $retained -ne [long]$document.retained_reference_count -or
      $retired -ne [long]$document.retired_reference_count -or
      $missing -ne [long]$document.missing_reference_count) {
    throw 'COS disposition evidence aggregate counts do not match its references'
  }
  return $document
}

function Assert-ExternalEvidenceBundle {
  param(
    [Parameter(Mandatory = $true)][string]$FingerprintPath,
    [Parameter(Mandatory = $true)][string]$RecoveryPath,
    [Parameter(Mandatory = $true)][string]$COSPath,
    [Parameter(Mandatory = $true)][string]$QueryPath,
    [Parameter(Mandatory = $true)][string]$FreezePath
  )
  $freeze = Assert-ClientVersionsFreezeEvidence -Path $FreezePath
  $fingerprint = Assert-DatabaseFingerprint -Path $FingerprintPath
  $recovery = Assert-RecoveryArtifact -Path $RecoveryPath
  Assert-QueryEvidence -Path $QueryPath
  [void](Assert-COSDispositionEvidence -Path $COSPath -FreezeEvidence $freeze)
  $fingerprintMatch = [regex]::Match($freeze.Text, '(?m)^source_schema_fingerprint=([0-9a-f]{64})$')
  $recoveryMatch = [regex]::Match($freeze.Text, '(?m)^recovery_dump_sha256=([0-9a-f]{64})$')
  if (-not $fingerprintMatch.Success -or $fingerprint.SchemaSha256 -cne $fingerprintMatch.Groups[1].Value) {
    throw 'database fingerprint does not match P08R evidence'
  }
  if (-not $recoveryMatch.Success -or $recovery.DumpSha256 -cne $recoveryMatch.Groups[1].Value) {
    throw 'recovery artifact does not match P08R evidence'
  }
}

function Get-GitBlobSha256 {
  param(
    [Parameter(Mandatory = $true)][string]$Repository,
    [Parameter(Mandatory = $true)][string]$Commit,
    [Parameter(Mandatory = $true)][string]$Path
  )
  $startInfo = [Diagnostics.ProcessStartInfo]::new()
  $startInfo.FileName = (Get-Command git -ErrorAction Stop).Source
  $startInfo.UseShellExecute = $false
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  [void]$startInfo.ArgumentList.Add('-C')
  [void]$startInfo.ArgumentList.Add($Repository)
  [void]$startInfo.ArgumentList.Add('cat-file')
  [void]$startInfo.ArgumentList.Add('blob')
  [void]$startInfo.ArgumentList.Add("$Commit`:$Path")
  $process = [Diagnostics.Process]::new()
  $process.StartInfo = $startInfo
  if (-not $process.Start()) { throw 'failed to start Git blob reader' }
  $memory = [IO.MemoryStream]::new()
  try {
    $process.StandardOutput.BaseStream.CopyTo($memory)
    $errorText = $process.StandardError.ReadToEnd()
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw 'locked contract manifest is unavailable from Git' }
    return [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($memory.ToArray())).ToLowerInvariant()
  } finally {
    $memory.Dispose()
    $process.Dispose()
  }
}

function Assert-InputLockSchema {
  param([Parameter(Mandatory = $true)][string]$Path)
  $json = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
  if (-not ($json | Test-Json -SchemaFile $script:InputLockSchema -ErrorAction SilentlyContinue)) {
    throw 'input lock does not match its schema'
  }
  return $json | ConvertFrom-Json
}

function Test-LockedCommitAncestor {
  param([Parameter(Mandatory = $true)][string]$Repository, [Parameter(Mandatory = $true)][string]$LockedCommit)
  # Contract: git merge-base --is-ancestor validates a frozen pre-contract input.
  & git -C $Repository merge-base --is-ancestor $LockedCommit HEAD 2>$null
  if ($LASTEXITCODE -ne 0) { throw 'current repository does not descend from the frozen input commit' }
}

function Invoke-InputLockSchemaSelfTest {
  $valid = [ordered]@{
    schema_version = 1
    backend_commit = 'a' * 40
    frontend_commit = 'b' * 40
    contract_manifest_sha256 = 'c' * 64
    database_fingerprint_sha256 = 'd' * 64
    recovery_artifact_sha256 = 'e' * 64
    cos_disposition_evidence_sha256 = 'f' * 64
    query_evidence_sha256 = '0' * 64
    client_versions_freeze_evidence_sha256 = '1' * 64
  }
  $validJSON = $valid | ConvertTo-Json -Compress
  if (-not ($validJSON | Test-Json -SchemaFile $script:InputLockSchema)) { throw 'input lock schema rejected its valid self-test' }
  $valid.backend_commit = 'invalid'
  if (($valid | ConvertTo-Json -Compress) | Test-Json -SchemaFile $script:InputLockSchema -ErrorAction SilentlyContinue) {
    throw 'input lock schema accepted its invalid self-test'
  }
}

if ($ImportFunctions) { return }

if ($SchemaOnly) {
  Invoke-InputLockSchemaSelfTest
  Write-Output 'input lock schema check passed'
  return
}

if ([string]::IsNullOrWhiteSpace($LockPath)) { $LockPath = $script:DefaultInputLock }
if ([string]::IsNullOrWhiteSpace($ClientVersionsFreezeEvidence)) { $ClientVersionsFreezeEvidence = $script:DefaultFreezeEvidence }
$lockFile = Get-RequiredFilePath -Path $LockPath -Label 'input lock'
$fingerprintFile = Assert-ExternalEvidencePath -Path $DatabaseFingerprint -Label 'database fingerprint evidence'
$recoveryFile = Assert-ExternalEvidencePath -Path $RecoveryArtifact -Label 'recovery artifact'
$cosFile = Assert-ExternalEvidencePath -Path $CosDispositionEvidence -Label 'COS disposition evidence'
$queryFile = Assert-ExternalEvidencePath -Path $QueryEvidence -Label 'query evidence'
$freezeFile = Get-RequiredFilePath -Path $ClientVersionsFreezeEvidence -Label 'client-version freeze evidence'
if (-not $freezeFile.Equals($script:DefaultFreezeEvidence, [StringComparison]::OrdinalIgnoreCase)) {
  throw 'client-version freeze evidence must be the committed P08R runbook'
}

Assert-SingleWorktree -Repository $script:BackendRoot
Assert-SingleWorktree -Repository $script:FrontendRoot
$lock = Assert-InputLockSchema -Path $lockFile
$backendCommit = Assert-GitSha -Value ([string]$lock.backend_commit) -Label 'locked backend commit'
$frontendCommit = Assert-GitSha -Value ([string]$lock.frontend_commit) -Label 'locked frontend commit'
$allowedBackendPaths = @()
if ((Get-RepositoryCommit -Repository $script:BackendRoot) -ceq $backendCommit) {
  $allowedBackendPaths = @(
    'release/admin-only/input-lock.json',
    'release/admin-only/input-lock.schema.json',
    'release/admin-only/.gitignore',
    'scripts/release/lock-inputs.ps1',
    'scripts/release/check-inputs.ps1',
    'scripts/tests/release-input-lock.tests.ps1',
    'database/reconciliation/050_contract_preconditions.sql',
    'docs/runbooks/admin-only-data-disposition.md'
  )
}
Assert-RepositoryStatus -Repository $script:BackendRoot -AllowedPaths $allowedBackendPaths
Assert-RepositoryStatus -Repository $script:FrontendRoot
Assert-ExternalEvidenceBundle -FingerprintPath $fingerprintFile -RecoveryPath $recoveryFile -COSPath $cosFile -QueryPath $queryFile -FreezePath $freezeFile

Test-LockedCommitAncestor -Repository $script:BackendRoot -LockedCommit $backendCommit
Test-LockedCommitAncestor -Repository $script:FrontendRoot -LockedCommit $frontendCommit

$expected = [ordered]@{
  contract_manifest_sha256 = Get-GitBlobSha256 -Repository $script:BackendRoot -Commit $backendCommit -Path 'contracts/admin/v1/manifest.json'
  database_fingerprint_sha256 = Get-FileSha256 -Path $fingerprintFile
  recovery_artifact_sha256 = Get-FileSha256 -Path $recoveryFile
  cos_disposition_evidence_sha256 = Get-FileSha256 -Path $cosFile
  query_evidence_sha256 = Get-FileSha256 -Path $queryFile
  client_versions_freeze_evidence_sha256 = Get-FileSha256 -Path $freezeFile
}
foreach ($entry in $expected.GetEnumerator()) {
  if ([string]$lock.($entry.Key) -cne [string]$entry.Value) { throw "input lock digest mismatch: $($entry.Key)" }
}

Write-Output 'release input lock check passed'
