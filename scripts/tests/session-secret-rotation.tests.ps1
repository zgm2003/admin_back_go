[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$runtimeEnv = Join-Path $repoRoot 'deploy\docker-first\admin-go.env'
$defaultDocker = 'E:\Docker\Docker\resources\bin\docker.exe'
$docker = if (Test-Path -LiteralPath $defaultDocker -PathType Leaf) {
  $defaultDocker
}
else {
  (Get-Command docker.exe -ErrorAction Stop | Select-Object -First 1).Source
}

if (-not (Test-Path -LiteralPath $runtimeEnv -PathType Leaf)) {
  throw 'Docker runtime env is missing; run scripts/docker-platform.ps1 init first.'
}

function New-RotationSecret {
  $bytes = New-Object byte[] 48
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $rng.GetBytes($bytes)
  }
  finally {
    $rng.Dispose()
  }
  return [Convert]::ToBase64String($bytes)
}

function Test-PathInsideRoot {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Root
  )

  $comparison = if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    [StringComparison]::OrdinalIgnoreCase
  }
  else {
    [StringComparison]::Ordinal
  }
  $separators = [char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
  $prefix = $Root.TrimEnd($separators) + [IO.Path]::DirectorySeparatorChar
  return $Path.StartsWith($prefix, $comparison)
}

$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$tempDirectory = [IO.Path]::GetFullPath((Join-Path $tempRoot ('admin-p04-secret-rotation-' + [guid]::NewGuid().ToString('N'))))
if (-not (Test-PathInsideRoot -Path $tempDirectory -Root $tempRoot) -or
    -not ([IO.Path]::GetFileName($tempDirectory)).StartsWith('admin-p04-secret-rotation-', [StringComparison]::Ordinal)) {
  throw 'Refusing to create an unverified rotation rehearsal directory.'
}

$oldSecret = New-RotationSecret
$newSecret = New-RotationSecret
$secretEnv = Join-Path $tempDirectory 'rotation.env'
$testLog = Join-Path $tempDirectory 'rotation.log'

try {
  [IO.Directory]::CreateDirectory($tempDirectory) | Out-Null
  $envText = @(
    'ADMIN_IDENTITY_INTEGRATION=1'
    'P04_ROTATION_OLD_SECRET=' + $oldSecret
    'P04_ROTATION_NEW_SECRET=' + $newSecret
  ) -join "`n"
  [IO.File]::WriteAllText($secretEnv, $envText + "`n", (New-Object Text.UTF8Encoding($false)))

  $arguments = @(
    'run', '--rm', '--network', 'admin-platform',
    '--env-file', $runtimeEnv,
    '--env-file', $secretEnv,
    '-v', ($repoRoot + ':/src'),
    '-w', '/src',
    '-v', 'admin-go-mod-cache:/go/pkg/mod',
    '-v', 'admin-go-build-cache:/root/.cache/go-build',
    'docker.m.daocloud.io/library/golang:1.26.5-bookworm',
    'go', 'test', './internal/module/auth', '-run', '^TestMultiNode', '-race', '-count=1', '-v'
  )
  $output = @(& $docker @arguments 2>&1 | ForEach-Object { $_.ToString() })
  $exitCode = $LASTEXITCODE
  [IO.File]::WriteAllLines($testLog, $output, (New-Object Text.UTF8Encoding($false)))
  $logText = [IO.File]::ReadAllText($testLog)
  if ($logText.Contains($oldSecret, [StringComparison]::Ordinal) -or
      $logText.Contains($newSecret, [StringComparison]::Ordinal)) {
    throw 'Rotation rehearsal leaked an ephemeral secret into its logs.'
  }
  if ($exitCode -ne 0) {
    $output | Write-Output
    throw "Docker rotation rehearsal failed with exit code $exitCode."
  }
  Write-Output 'Docker session-secret rotation rehearsal passed without secret leakage.'
}
finally {
  $oldSecret = $null
  $newSecret = $null
  if (Test-Path -LiteralPath $tempDirectory -PathType Container) {
    $resolvedDirectory = [IO.Path]::GetFullPath($tempDirectory)
    if (-not (Test-PathInsideRoot -Path $resolvedDirectory -Root $tempRoot) -or
        -not ([IO.Path]::GetFileName($resolvedDirectory)).StartsWith('admin-p04-secret-rotation-', [StringComparison]::Ordinal)) {
      throw 'Refusing to delete an unverified rotation rehearsal directory.'
    }
    [IO.Directory]::Delete($resolvedDirectory, $true)
  }
}
