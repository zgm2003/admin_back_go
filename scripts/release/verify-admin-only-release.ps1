[CmdletBinding()]
param(
  [string]$Manifest,
  [string]$Database = $env:ADMIN_RESTORE_DB,
  [string]$Output,
  [string]$FrontendURL = 'http://127.0.0.1:5173',
  [string]$APIURL = 'http://127.0.0.1:8080',
  [string]$DockerCommand = 'docker',
  [switch]$ListGates,
  [switch]$ImportFunctions
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$ProgressPreference = 'SilentlyContinue'
Set-StrictMode -Version Latest

$script:BackendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$script:FrontendRoot = [IO.Path]::GetFullPath((Join-Path $script:BackendRoot '..\admin_front_ts'))
$script:ReleaseOutputRoot = Join-Path $script:BackendRoot 'release\admin-only\out'
# Fixed contract path: release\admin-only\out\proof.json
$script:DefaultProofPath = Join-Path $script:ReleaseOutputRoot 'proof.json'
$script:GateNames = @(
  'repository-boundary',
  'release-manifest',
  'backend-quality',
  'database-recovery-contract',
  'runtime-identity-durable-realtime',
  'admin-contract-bundle',
  'frontend-quality',
  'p07-runtime-acceptance',
  'p08r-browser-only-acceptance',
  'sensitive-material-scan',
  'admin-only-platform-kernel',
  'release-artifact-integrity'
)

if ($ListGates) {
  $script:GateNames | Write-Output
  return
}

. (Join-Path $PSScriptRoot 'check-release-manifest.ps1') -ImportFunctions

function Get-ReleaseTextSHA256 {
  param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text)
  $bytes = [Text.Encoding]::UTF8.GetBytes($Text)
  try {
    return [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
  } finally {
    [Array]::Clear($bytes, 0, $bytes.Length)
  }
}

function Get-ReleaseFileSHA256 {
  param([Parameter(Mandatory = $true)][string]$Path)
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw 'required release evidence is missing' }
  return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Resolve-ReleaseExecutable {
  param([Parameter(Mandatory = $true)][string]$Command)
  if ([IO.Path]::IsPathRooted($Command)) {
    if (-not (Test-Path -LiteralPath $Command -PathType Leaf)) { throw 'required executable is missing' }
    return [IO.Path]::GetFullPath($Command)
  }
  return (Get-Command $Command -ErrorAction Stop | Select-Object -First 1).Source
}

function Invoke-ReleaseVerificationCommand {
  param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][AllowEmptyCollection()][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label,
    [string]$WorkingDirectory = $script:BackendRoot
  )
  Push-Location $WorkingDirectory
  try {
    $lines = @(& $Executable @Arguments 2>&1)
    if ($LASTEXITCODE -ne 0) { throw "$Label failed" }
    $text = (@($lines | ForEach-Object { $_.ToString() }) -join "`n")
    return [ordered]@{
      output_sha256 = Get-ReleaseTextSHA256 -Text $text
      output_line_count = $lines.Count
    }
  } finally {
    Pop-Location
  }
}

function Invoke-ReleasePowerShell {
  param(
    [Parameter(Mandatory = $true)][string]$RelativePath,
    [AllowEmptyCollection()][string[]]$Arguments = @(),
    [Parameter(Mandatory = $true)][string]$Label
  )
  $path = Join-Path $script:BackendRoot $RelativePath
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "$Label script is missing" }
  return Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'pwsh') -Arguments (@('-NoProfile', '-File', $path) + $Arguments) -Label $Label
}

function Invoke-ReleaseGit {
  param(
    [Parameter(Mandatory = $true)][string]$Repository,
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $lines = @(& git -C $Repository @Arguments 2>&1)
  if ($LASTEXITCODE -ne 0) { throw "$Label failed" }
  return @($lines | ForEach-Object { $_.ToString() })
}

function Assert-ReleaseRepositoryBoundary {
  param(
    [Parameter(Mandatory = $true)][string]$Repository,
    [Parameter(Mandatory = $true)][string]$ExpectedCommit,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $commit = ((Invoke-ReleaseGit -Repository $Repository -Arguments @('rev-parse', '--verify', 'HEAD') -Label "$Label revision") -join '').Trim().ToLowerInvariant()
  if ($commit -cne $ExpectedCommit) { throw "$Label release commit changed" }

  # The literal commands are part of the audited release contract:
  # git worktree list --porcelain
  # git status --porcelain=v1 --untracked-files=all
  $worktreeLines = Invoke-ReleaseGit -Repository $Repository -Arguments @('worktree', 'list', '--porcelain') -Label "$Label worktree check"
  $worktreeCount = @($worktreeLines | Where-Object { $_ -like 'worktree *' }).Count
  if ($worktreeCount -ne 1) { throw "$Label has a secondary worktree" }
  $worktreePath = [IO.Path]::GetFullPath(([string]($worktreeLines | Where-Object { $_ -like 'worktree *' } | Select-Object -First 1)).Substring(9))
  if (-not $worktreePath.Equals($Repository, [StringComparison]::OrdinalIgnoreCase)) { throw "$Label primary checkout changed" }

  $status = Invoke-ReleaseGit -Repository $Repository -Arguments @('status', '--porcelain=v1', '--untracked-files=all') -Label "$Label status check"
  if ($status.Count -ne 0) { throw "$Label repository is not clean" }
  if (Test-Path -LiteralPath (Join-Path $Repository '.github')) { throw "$Label contains a forbidden .github directory" }
  return [ordered]@{ commit = $commit; worktree_count = $worktreeCount; dirty_path_count = 0; github_path_count = 0 }
}

function Get-AcceptanceEvidence {
  param([Parameter(Mandatory = $true)][string]$Path)
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw 'manual acceptance evidence is missing' }
  $text = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
  $checked = [regex]::Matches($text, '(?im)^\s*-\s*\[[x]\]').Count
  $pending = [regex]::Matches($text, '(?im)^\s*-\s*\[ \]').Count
  if (($checked + $pending) -eq 0) { throw 'manual acceptance evidence has no checklist' }
  return [ordered]@{
    sha256 = Get-ReleaseFileSHA256 -Path $Path
    checked_count = $checked
    pending_count = $pending
  }
}

function Assert-ReleaseHTTP {
  param([Parameter(Mandatory = $true)][string]$URL, [Parameter(Mandatory = $true)][string]$Label)
  try {
    $response = Invoke-WebRequest -Uri $URL -Method Get -TimeoutSec 15 -UseBasicParsing
  } catch {
    throw "$Label probe failed"
  }
  if ([int]$response.StatusCode -ne 200) { throw "$Label probe did not return HTTP 200" }
  return [ordered]@{ status_code = [int]$response.StatusCode; body_sha256 = Get-ReleaseTextSHA256 -Text ([string]$response.Content) }
}

function Assert-TrackedSensitiveMaterialAbsent {
  $backendFiles = Invoke-ReleaseGit -Repository $script:BackendRoot -Arguments @('ls-files') -Label 'backend tracked-file scan'
  $frontendFiles = Invoke-ReleaseGit -Repository $script:FrontendRoot -Arguments @('ls-files') -Label 'frontend tracked-file scan'
  $violations = [Collections.Generic.List[string]]::new()
  foreach ($item in @(
    [pscustomobject]@{ Root = $script:BackendRoot; Files = $backendFiles },
    [pscustomobject]@{ Root = $script:FrontendRoot; Files = $frontendFiles }
  )) {
    foreach ($relative in $item.Files) {
      $normalized = ([string]$relative).Replace('\', '/')
      $leaf = [IO.Path]::GetFileName($normalized)
      if ($normalized -match '(?i)(^|/)[^/]*(dump|backup)[^/]*\.(sql|sql\.gz|rdb|dump)$' -or
          $leaf -match '(?i)\.(pem|p12|pfx|jks|key)$' -or
          ($leaf -match '(?i)^\.env($|\.)' -and $leaf -notmatch '(?i)\.example$' -and
           -not ($item.Root -eq $script:FrontendRoot -and $leaf -in @('.env.development', '.env.production')))) {
        $violations.Add($normalized)
      }
    }
  }
  if ($violations.Count -ne 0) { throw 'tracked secret, certificate, or dump material detected' }

  foreach ($relative in @('.env.development', '.env.production')) {
    $path = Join-Path $script:FrontendRoot $relative
    $content = [IO.File]::ReadAllText($path, [Text.Encoding]::UTF8)
    if ($content -match '(?im)^\s*[^#\r\n]*(SECRET|PASSWORD|PRIVATE_KEY|ACCESS_TOKEN)\s*=\s*\S+') {
      throw 'frontend environment file contains sensitive material'
    }
  }
  return [ordered]@{
    tracked_file_count = $backendFiles.Count + $frontendFiles.Count
    sensitive_path_count = 0
  }
}

function Write-AdminReleaseProof {
  param(
    [Parameter(Mandatory = $true)]$Proof,
    [Parameter(Mandatory = $true)][string]$Path
  )
  $outputPath = [IO.Path]::GetFullPath($Path)
  $expectedPath = [IO.Path]::GetFullPath($script:DefaultProofPath)
  if (-not $outputPath.Equals($expectedPath, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'release proof output path is fixed by the release contract'
  }
  [IO.Directory]::CreateDirectory((Split-Path -Parent $outputPath)) | Out-Null
  $temporaryPath = Join-Path (Split-Path -Parent $outputPath) ('.proof.' + [guid]::NewGuid().ToString('N') + '.tmp')
  try {
    $json = $Proof | ConvertTo-Json -Depth 12
    [IO.File]::WriteAllText($temporaryPath, $json + "`n", [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporaryPath -Destination $outputPath -Force
  } finally {
    if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
  }
}

function Invoke-AdminReleaseGate {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][scriptblock]$Action
  )
  $watch = [Diagnostics.Stopwatch]::StartNew()
  try {
    $evidence = & $Action
    $watch.Stop()
    return [ordered]@{
      name = $Name
      passed = $true
      duration_ms = [long]$watch.ElapsedMilliseconds
      evidence = $evidence
    }
  } catch {
    $watch.Stop()
    $script:FailedGate = $Name
    $script:FailureSHA256 = Get-ReleaseTextSHA256 -Text $_.Exception.Message
    throw
  }
}

if ($ImportFunctions) { return }

if ([string]::IsNullOrWhiteSpace($Manifest)) { $Manifest = Join-Path $script:ReleaseOutputRoot 'release-manifest.json' }
if ([string]::IsNullOrWhiteSpace($Output)) { $Output = $script:DefaultProofPath }
if ($Database -cnotmatch '^[A-Za-z][A-Za-z0-9_]{0,63}$') { throw 'ADMIN_RESTORE_DB or -Database must name a post-contract disposable database' }
if ($FrontendURL -cnotmatch '^http://127\.0\.0\.1:[0-9]{2,5}$' -or $APIURL -cnotmatch '^http://127\.0\.0\.1:[0-9]{2,5}$') {
  throw 'release verification URLs must be loopback HTTP origins'
}

$manifestPath = Get-RequiredFilePath -Path $Manifest -Label 'release manifest'
$release = Get-ReleaseManifestDocument -Path $manifestPath
$docker = Resolve-ReleaseExecutable -Command $DockerCommand
$frontendP07Acceptance = Join-Path $script:FrontendRoot 'docs\acceptance\p07-frontend-manual.md'
$frontendP08RAcceptance = Join-Path $script:FrontendRoot 'docs\acceptance\p08r-browser-only-manual.md'
$script:FailedGate = ''
$script:FailureSHA256 = ''
$gateResults = [Collections.Generic.List[object]]::new()
$totalWatch = [Diagnostics.Stopwatch]::StartNew()

$proof = [ordered]@{
  schema_version = 1
  release_id = [string]$release.release_id
  passed = $false
  generated_at_utc = [DateTime]::UtcNow.ToString('o')
  manifest_sha256 = Get-ReleaseFileSHA256 -Path $manifestPath
  backend_commit = [string]$release.backend.commit
  frontend_commit = [string]$release.frontend.commit
  contract_manifest_sha256 = [string]$release.contract.manifest_sha256
  database_target_fingerprint_sha256 = [string]$release.database.target_fingerprint_sha256
  image_ids = [ordered]@{
    backend = Get-ReleaseImageID -Image ([string]$release.backend.image)
    frontend = Get-ReleaseImageID -Image ([string]$release.frontend.image)
  }
  gates = $gateResults
  total_duration_ms = 0
  failed_gate = ''
  failure_sha256 = ''
}

try {
  $gateResults.Add((Invoke-AdminReleaseGate -Name 'repository-boundary' -Action {
    return [ordered]@{
      backend = Assert-ReleaseRepositoryBoundary -Repository $script:BackendRoot -ExpectedCommit ([string]$release.backend.commit) -Label 'backend'
      frontend = Assert-ReleaseRepositoryBoundary -Repository $script:FrontendRoot -ExpectedCommit ([string]$release.frontend.commit) -Label 'frontend'
    }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'release-manifest' -Action {
    $validated = Assert-ReleaseManifest -ManifestPath $manifestPath -DockerExecutable $docker
    return [ordered]@{
      manifest_sha256 = Get-ReleaseFileSHA256 -Path $validated.Path
      input_lock_sha256 = Get-ReleaseFileSHA256 -Path (Join-Path $script:BackendRoot 'release\admin-only\input-lock.json')
      platform_kernel_sha256 = Get-ReleaseFileSHA256 -Path (Join-Path $script:ReleaseOutputRoot 'platform-kernel-proof.json')
    }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'backend-quality' -Action {
    # go clean -testcache
    $clean = Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'go') -Arguments @('clean', '-testcache') -Label 'backend clean test cache'
    # go mod verify
    $modules = Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'go') -Arguments @('mod', 'verify') -Label 'backend dependency verification'
    $quality = Invoke-ReleasePowerShell -RelativePath 'scripts\verify-backend.ps1' -Label 'backend quality gate'
    $releaseArchitecture = Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'go') -Arguments @('test', './internal/architecture', '-run', 'TestAdminRelease', '-count=1') -Label 'Admin release architecture gate'
    return [ordered]@{ clean = $clean; dependencies = $modules; quality = $quality; release_architecture = $releaseArchitecture }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'database-recovery-contract' -Action {
    $databaseOutput = Join-Path $script:ReleaseOutputRoot 'database-verification.json'
    $databaseGate = Invoke-ReleasePowerShell -RelativePath 'scripts\verify-database.ps1' -Arguments @('-Mode', 'all', '-OutputPath', $databaseOutput) -Label 'database restore and reconciliation gate'
    $recovery = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\database-recovery.tests.ps1' -Label 'database recovery assertions'
    $contract = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\admin-only-contract.tests.ps1' -Label 'Admin-only contract assertions'
    $inputLock = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\release-input-lock.tests.ps1' -Label 'release input lock assertions'
    return [ordered]@{
      database = $databaseGate
      database_summary_sha256 = Get-ReleaseFileSHA256 -Path $databaseOutput
      recovery = $recovery
      contract = $contract
      input_lock = $inputLock
    }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'runtime-identity-durable-realtime' -Action {
    $runtime = Invoke-ReleasePowerShell -RelativePath 'scripts\verify-runtime-contracts.ps1' -Label 'runtime contract gate'
    $identity = Invoke-ReleasePowerShell -RelativePath 'scripts\verify-identity-routing.ps1' -Label 'identity routing gate'
    $durable = Invoke-ReleasePowerShell -RelativePath 'scripts\verify-durable-work.ps1' -Label 'durable work gate'
    $termination = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\process-sigterm.tests.ps1' -Label 'process termination assertions'
    $restart = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\durable-work-restart.tests.ps1' -Label 'durable work restart assertions'
    $dockerPlatform = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\docker-platform.tests.ps1' -Label 'Docker platform assertions'
    $dockerStability = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\docker-stability.tests.ps1' -Label 'Docker stability assertions'
    return [ordered]@{
      runtime = $runtime; identity = $identity; durable = $durable; termination = $termination
      restart = $restart; docker_platform = $dockerPlatform; docker_stability = $dockerStability
    }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'admin-contract-bundle' -Action {
    $contract = Invoke-ReleasePowerShell -RelativePath 'scripts\check-admin-contract.ps1' -Label 'Admin Contract Bundle check'
    $bundle = Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'go') -Arguments @('test', './internal/admincontract', '-count=1') -Label 'Admin Contract Bundle tests'
    return [ordered]@{ contract = $contract; bundle = $bundle; manifest_sha256 = [string]$release.contract.manifest_sha256 }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'frontend-quality' -Action {
    $containerCommand = 'apk add --no-cache git >/dev/null && git config --global core.autocrlf true && npm ci --no-audit --no-fund && npm run verify:frontend'
    $frontend = Invoke-ReleaseVerificationCommand -Executable $docker -Arguments @(
      'run', '--rm',
      '--mount', "type=bind,src=$script:FrontendRoot,dst=/workspace",
      '--mount', 'type=volume,dst=/workspace/node_modules',
      '--workdir', '/workspace',
      'node:24.18.0-alpine',
      'sh', '-lc', $containerCommand
    ) -Label 'frontend Docker quality gate' -WorkingDirectory $script:FrontendRoot
    return $frontend
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'p07-runtime-acceptance' -Action {
    $acceptance = Get-AcceptanceEvidence -Path $frontendP07Acceptance
    $status = Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'pwsh') -Arguments @('-NoLogo', '-Command', 'admin-status') -Label 'admin-status'
    $web = Assert-ReleaseHTTP -URL $FrontendURL -Label 'Admin Web'
    $health = Assert-ReleaseHTTP -URL ($APIURL + '/health') -Label 'Admin API health'
    $readiness = Assert-ReleaseHTTP -URL ($APIURL + '/ready') -Label 'Admin API readiness'
    return [ordered]@{ acceptance = $acceptance; status = $status; web = $web; health = $health; readiness = $readiness }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'p08r-browser-only-acceptance' -Action {
    $acceptance = Get-AcceptanceEvidence -Path $frontendP08RAcceptance
    if ([int]$acceptance.pending_count -ne 0 -or [int]$acceptance.checked_count -eq 0) { throw 'P08R user acceptance is incomplete' }
    $retirement = Invoke-ReleasePowerShell -RelativePath 'scripts\tests\browser-only-cutover.tests.ps1' -Label 'Browser-only retirement assertions'
    return [ordered]@{ acceptance = $acceptance; retirement = $retirement }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'sensitive-material-scan' -Action {
    return Assert-TrackedSensitiveMaterialAbsent
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'admin-only-platform-kernel' -Action {
    $platform = Invoke-ReleasePowerShell -RelativePath 'scripts\release\check-platform-kernel.ps1' -Arguments @('-Database', $Database, '-Output', (Join-Path $script:ReleaseOutputRoot 'platform-kernel-proof.json')) -Label 'Admin-only platform-kernel gate'
    $architecture = Invoke-ReleaseVerificationCommand -Executable (Resolve-ReleaseExecutable -Command 'go') -Arguments @('test', './internal/architecture', '-run', 'Test(AdminOnly|PlatformKernel)', '-count=1') -Label 'Admin-only source and generated scan'
    # client_versions absence and all seven auth-platforms operations are enforced by 053 and the platform proof.
    return [ordered]@{
      platform = $platform
      architecture = $architecture
      proof_sha256 = Get-ReleaseFileSHA256 -Path (Join-Path $script:ReleaseOutputRoot 'platform-kernel-proof.json')
    }
  }))

  $gateResults.Add((Invoke-AdminReleaseGate -Name 'release-artifact-integrity' -Action {
    $validated = Assert-ReleaseManifest -ManifestPath $manifestPath -DockerExecutable $docker
    $metadataPath = Join-Path $script:ReleaseOutputRoot 'images\metadata.json'
    return [ordered]@{
      metadata_sha256 = Get-ReleaseFileSHA256 -Path $metadataPath
      backend_archive_sha256 = [string]$validated.Document.backend.archive_sha256
      frontend_archive_sha256 = [string]$validated.Document.frontend.archive_sha256
      backend_image_id = Get-ReleaseImageID -Image ([string]$validated.Document.backend.image)
      frontend_image_id = Get-ReleaseImageID -Image ([string]$validated.Document.frontend.image)
    }
  }))

  $proof.passed = $true
} catch {
  $proof.failed_gate = $script:FailedGate
  $proof.failure_sha256 = $script:FailureSHA256
  throw
} finally {
  $totalWatch.Stop()
  $proof.total_duration_ms = [long]$totalWatch.ElapsedMilliseconds
  Write-AdminReleaseProof -Proof $proof -Path $Output
}

Write-Output 'Admin-only release proof passed'
