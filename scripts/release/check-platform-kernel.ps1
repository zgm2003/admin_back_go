[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[A-Za-z][A-Za-z0-9_]{0,63}$')]
  [string]$Database,

  [string]$Output
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest

$backendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $backendRoot '..\admin_front_ts'))
$outputRoot = Join-Path $backendRoot 'release\admin-only\out'
if ([string]::IsNullOrWhiteSpace($Output)) { $Output = Join-Path $outputRoot 'platform-kernel-proof.json' }
$outputPath = [IO.Path]::GetFullPath($Output)
$expectedOutput = [IO.Path]::GetFullPath((Join-Path $outputRoot 'platform-kernel-proof.json'))
if (-not $outputPath.Equals($expectedOutput, [StringComparison]::OrdinalIgnoreCase)) {
  throw 'platform-kernel proof output path is fixed by the release contract'
}

. (Join-Path $PSScriptRoot 'check-inputs.ps1') -ImportFunctions
. (Join-Path $backendRoot 'scripts\database\atlas-runtime-common.ps1')

function Invoke-GoReleaseCheck {
  param(
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label
  )
  Push-Location $backendRoot
  try {
    $output = @(& go @Arguments 2>$null)
    if ($LASTEXITCODE -ne 0) { throw "$Label failed" }
    return @($output | ForEach-Object { $_.ToString() })
  } finally {
    Pop-Location
  }
}

Assert-SingleWorktree -Repository $backendRoot
Assert-SingleWorktree -Repository $frontendRoot
Assert-RepositoryStatus -Repository $backendRoot
Assert-RepositoryStatus -Repository $frontendRoot

[void](Invoke-GoReleaseCheck -Arguments @('test', './internal/architecture', '-run', '^TestPlatformKernel', '-count=1') -Label 'TestPlatformKernel')
& pwsh -NoProfile -File (Join-Path $backendRoot 'scripts\check-admin-contract.ps1') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'backend contract check failed' }

$contractPath = Join-Path $backendRoot 'contracts\admin\v1\manifest.json'
$openAPIPath = Join-Path $backendRoot 'contracts\admin\v1\openapi.json'
$contract = Read-JsonEvidence -Path $contractPath -Label 'backend contract manifest'
$openAPI = [IO.File]::ReadAllText($openAPIPath, [Text.Encoding]::UTF8) | ConvertFrom-Json -AsHashtable
$methods = @('get', 'post', 'put', 'patch', 'delete')
$operationIDs = [Collections.Generic.List[string]]::new()
foreach ($path in @($openAPI.paths.Keys | Sort-Object)) {
  foreach ($method in $methods) {
    if ($openAPI.paths[$path].ContainsKey($method)) {
      $operationID = [string]$openAPI.paths[$path][$method].operationId
      if (-not [string]::IsNullOrWhiteSpace($operationID)) { $operationIDs.Add($operationID) }
    }
  }
}
$authPlatformOperations = @($operationIDs | Where-Object { $_ -like '*_auth_platforms*' } | Sort-Object)
$expectedAuthPlatformOperations = @(
  'delete_api_admin_v1_auth_platforms',
  'delete_api_admin_v1_auth_platforms_id',
  'get_api_admin_v1_auth_platforms',
  'get_api_admin_v1_auth_platforms_page_init',
  'patch_api_admin_v1_auth_platforms_id_status',
  'post_api_admin_v1_auth_platforms',
  'put_api_admin_v1_auth_platforms_id'
)
if (($authPlatformOperations -join '|') -cne ($expectedAuthPlatformOperations -join '|')) {
  throw 'Admin Contract Bundle does not contain all seven auth-platforms operations'
}
if (@($openAPI.paths.Keys | Where-Object { $_ -like '/api/admin/v1/ai-prompts*' }).Count -ne 0) {
  throw 'Admin Contract Bundle still contains Prompt management operations'
}

$requiredPlatformSchemas = [ordered]@{
  'AIRunDetail' = @('platform')
  'Go_internal_module_auth_LoginLogListItem_Output' = @('platform', 'platform_name')
  'Go_internal_module_auth_SessionListItem_Output' = @('platform', 'platform_name')
  'Go_internal_module_auth_SessionStatsResponse_Output' = @('platform_distribution')
  'Go_internal_module_notification_task_ListItem_Output' = @('platform', 'platform_text')
  'Go_internal_module_permission_PermissionDict_Output' = @('permission_platform_arr')
  'Go_internal_module_permission_PermissionTreeNode_Output' = @('platform')
  'Go_internal_module_role_InitDict_Output' = @('permission_platform_arr')
}
$platformSchemaFieldCount = 0
foreach ($entry in $requiredPlatformSchemas.GetEnumerator()) {
  if (-not $openAPI.components.schemas.ContainsKey($entry.Key)) { throw "platform schema is missing: $($entry.Key)" }
  $properties = $openAPI.components.schemas[$entry.Key].properties
  foreach ($field in $entry.Value) {
    if (-not $properties.ContainsKey($field)) { throw "platform schema field is missing: $($entry.Key).$field" }
    $platformSchemaFieldCount++
  }
}

$frontendManifestPath = Join-Path $frontendRoot 'contracts\backend\admin\v1\manifest.json'
$frontendLockPath = Join-Path $frontendRoot 'contracts\backend\admin\lock.json'
$frontendLock = Read-JsonEvidence -Path $frontendLockPath -Label 'frontend contract lock'
$contractDigest = Get-FileSha256 -Path $contractPath
if ((Get-FileSha256 -Path $frontendManifestPath) -cne $contractDigest -or
    [string]$frontendLock.manifest_sha256 -cne $contractDigest -or
    [string]$frontendLock.bundle_version -cne [string]$contract.bundle_version -or
    [string]$frontendLock.backend_commit -cne [string]$contract.backend_commit) {
  throw 'frontend contract lock does not match the backend Bundle'
}

$settings = Get-MySQLDSNSettings -Database $Database
$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
try {
  $targetFingerprint = Get-DatabaseFingerprintSHA -BackendRoot $backendRoot -Settings $settings -Database $Database
  $env:MYSQL_DSN = New-SchemaDSN -Settings $settings -Database $Database
  $invariantLines = @(Invoke-GoReleaseCheck -Arguments @(
    'run', './cmd/admin-db', 'invariants',
    '--schema', $Database,
    '--file', 'database/reconciliation/053_verify_admin_only.sql'
  ) -Label '053_verify_admin_only.sql')
} finally {
  [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
  $settings.Password = $null
}

$invariantCounts = [ordered]@{}
foreach ($line in $invariantLines) {
  $parts = [string]$line -split "`t", 2
  if ($parts.Count -ne 2 -or [string]::IsNullOrWhiteSpace($parts[0]) -or $parts[1] -notmatch '^[0-9]+$') {
    throw 'platform-kernel invariant output is invalid'
  }
  if ($invariantCounts.Contains($parts[0])) { throw 'platform-kernel invariant output contains duplicate names' }
  $count = [uint64]$parts[1]
  if ($count -ne 0) { throw "platform-kernel invariant failed: $($parts[0])" }
  $invariantCounts[$parts[0]] = $count
}
if ($invariantCounts.Count -eq 0) { throw 'platform-kernel invariants produced no evidence' }

$proof = [ordered]@{
  schema_version = 1
  passed = $true
  backend_commit = Get-RepositoryCommit -Repository $backendRoot
  frontend_commit = Get-RepositoryCommit -Repository $frontendRoot
  bundle_version = [string]$contract.bundle_version
  contract_manifest_sha256 = $contractDigest
  atlas_version = '202607150203'
  target_fingerprint_sha256 = $targetFingerprint
  registered_platform_count = 1
  retired_platform_count = 0
  auth_platform_operation_count = $authPlatformOperations.Count
  platform_schema_field_count = $platformSchemaFieldCount
  invariant_counts = $invariantCounts
}

[IO.Directory]::CreateDirectory((Split-Path -Parent $outputPath)) | Out-Null
$temporaryPath = Join-Path (Split-Path -Parent $outputPath) ('.platform-kernel-proof.' + [guid]::NewGuid().ToString('N') + '.tmp')
try {
  [IO.File]::WriteAllText($temporaryPath, ($proof | ConvertTo-Json -Depth 8) + "`n", [Text.UTF8Encoding]::new($false))
  Move-Item -LiteralPath $temporaryPath -Destination $outputPath -Force
} finally {
  if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
}

Write-Output 'platform-kernel proof passed'
