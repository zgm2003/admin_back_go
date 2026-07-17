[CmdletBinding()]
param(
  [string]$BackendCommit = '',
  [string]$Out = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if ([string]::IsNullOrWhiteSpace($Out)) {
  $Out = Join-Path $repoRoot 'contracts\admin\v1'
}

Push-Location $repoRoot
try {
  if ([string]::IsNullOrWhiteSpace($BackendCommit)) {
    $manifestPath = Join-Path $Out 'manifest.json'
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
      throw "Admin contract manifest does not exist: $manifestPath"
    }
    $manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
    $BackendCommit = [string]$manifest.backend_commit
  }

  $BackendCommit = $BackendCommit.Trim()
  if ($BackendCommit -notmatch '^[0-9a-f]{40}$') {
    throw 'BackendCommit must be a full 40-character lowercase Git SHA.'
  }

  & go run ./cmd/admin-contract check --out $Out --commit $BackendCommit
  if ($LASTEXITCODE -ne 0) {
    throw "Admin contract check failed with exit code $LASTEXITCODE."
  }
}
finally {
  Pop-Location
}
