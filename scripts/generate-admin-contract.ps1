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
    $status = @(& git status --porcelain --untracked-files=all)
    if ($LASTEXITCODE -ne 0) {
      throw 'Could not inspect the backend Git checkout.'
    }
    if ($status.Count -gt 0) {
      throw 'Default contract generation requires a clean committed checkout; pass -BackendCommit explicitly during development.'
    }
    $BackendCommit = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) {
      throw 'Could not resolve the backend Git commit.'
    }
  }

  $BackendCommit = $BackendCommit.Trim()
  if ($BackendCommit -notmatch '^[0-9a-f]{40}$') {
    throw 'BackendCommit must be a full 40-character lowercase Git SHA.'
  }

  & go run ./cmd/admin-contract generate --out $Out --commit $BackendCommit
  if ($LASTEXITCODE -ne 0) {
    throw "Admin contract generation failed with exit code $LASTEXITCODE."
  }
}
finally {
  Pop-Location
}
