[CmdletBinding()]
param(
  [string[]]$ProfilePaths
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repositoryRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$expectedRoot = [IO.Path]::GetFullPath('E:\admin\admin_back_go')
if (-not [StringComparer]::OrdinalIgnoreCase.Equals($repositoryRoot, $expectedRoot)) {
  throw "ADMIN_SHORTCUT_PRIMARY_REPOSITORY_REQUIRED: $expectedRoot"
}

if ($null -eq $ProfilePaths -or $ProfilePaths.Count -eq 0) {
  $documents = [Environment]::GetFolderPath([Environment+SpecialFolder]::MyDocuments)
  $ProfilePaths = @(
    (Join-Path $documents 'PowerShell\Microsoft.PowerShell_profile.ps1'),
    (Join-Path $documents 'WindowsPowerShell\Microsoft.PowerShell_profile.ps1')
  )
}

$beginMarker = '# >>> admin platform shortcuts >>>'
$endMarker = '# <<< admin platform shortcuts <<<'
$adminDevScript = Join-Path $repositoryRoot 'scripts\admin-dev.ps1'
$dockerPlatformScript = Join-Path $repositoryRoot 'scripts\docker-platform.ps1'
$block = @"
$beginMarker
function global:admin-dev { & 'pwsh' -NoProfile -File '$adminDevScript' @args }
function global:admin-up { & 'pwsh' -NoProfile -File '$dockerPlatformScript' -Action up }
function global:admin-stop { & 'pwsh' -NoProfile -File '$dockerPlatformScript' -Action stop }
function global:admin-status { & 'pwsh' -NoProfile -File '$dockerPlatformScript' -Action status }
$endMarker
"@

$seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach ($profilePathValue in $ProfilePaths) {
  if ([string]::IsNullOrWhiteSpace($profilePathValue)) {
    throw 'ADMIN_SHORTCUT_PROFILE_PATH_REQUIRED'
  }
  $profilePath = [IO.Path]::GetFullPath($profilePathValue)
  if (-not $seen.Add($profilePath)) {
    continue
  }
  $directory = Split-Path $profilePath -Parent
  [IO.Directory]::CreateDirectory($directory) | Out-Null
  $content = if (Test-Path -LiteralPath $profilePath -PathType Leaf) {
    [IO.File]::ReadAllText($profilePath, [Text.Encoding]::UTF8)
  }
  else {
    ''
  }

  $beginIndex = $content.IndexOf($beginMarker, [StringComparison]::Ordinal)
  $endIndex = $content.IndexOf($endMarker, [StringComparison]::Ordinal)
  $extraBegin = if ($beginIndex -ge 0) {
    $content.IndexOf($beginMarker, $beginIndex + $beginMarker.Length, [StringComparison]::Ordinal)
  }
  else {
    -1
  }
  $extraEnd = if ($endIndex -ge 0) {
    $content.IndexOf($endMarker, $endIndex + $endMarker.Length, [StringComparison]::Ordinal)
  }
  else {
    -1
  }
  if (($beginIndex -ge 0) -xor ($endIndex -ge 0) -or
      $extraBegin -ge 0 -or
      $extraEnd -ge 0 -or
      ($beginIndex -ge 0 -and $endIndex -lt $beginIndex)) {
    throw "ADMIN_SHORTCUT_BLOCK_MALFORMED: $profilePath"
  }

  if ($beginIndex -ge 0) {
    $afterIndex = $endIndex + $endMarker.Length
    $content = $content.Substring(0, $beginIndex) + $content.Substring($afterIndex)
  }
  $content = $content.TrimEnd("`r", "`n")
  $newContent = if ([string]::IsNullOrEmpty($content)) {
    $block + "`n"
  }
  else {
    $content + "`n`n" + $block + "`n"
  }

  $temporaryPath = Join-Path $directory ([IO.Path]::GetRandomFileName())
  try {
    [IO.File]::WriteAllText($temporaryPath, $newContent, [Text.UTF8Encoding]::new($false))
    [IO.File]::Move($temporaryPath, $profilePath, $true)
    $temporaryPath = $null
  }
  finally {
    if (-not [string]::IsNullOrEmpty($temporaryPath) -and (Test-Path -LiteralPath $temporaryPath)) {
      Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue
    }
  }
  Write-Output "installed Admin shortcuts in $profilePath"
}
