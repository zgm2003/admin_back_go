[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$generate = Join-Path $repoRoot 'scripts\generate-admin-contract.ps1'
$check = Join-Path $repoRoot 'scripts\check-admin-contract.ps1'
$output = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-contract-test-' + [Guid]::NewGuid().ToString('N'))

Push-Location $repoRoot
try {
  $commit = (& git rev-parse HEAD).Trim()
  if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') {
    throw 'Could not resolve a full backend commit for the contract script test.'
  }

  & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $generate -BackendCommit $commit -Out $output
  if ($LASTEXITCODE -ne 0) { throw 'Contract generation script failed.' }

  & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $check -Out $output
  if ($LASTEXITCODE -ne 0) { throw 'Fresh generated contract did not pass check.' }

  $viewsPath = Join-Path $output 'views.json'
  $bytes = [System.IO.File]::ReadAllBytes($viewsPath)
  $bytes[0] = $bytes[0] -bxor 1
  [System.IO.File]::WriteAllBytes($viewsPath, $bytes)

  $previousPreference = $ErrorActionPreference
  try {
    # Windows PowerShell promotes redirected native stderr to ErrorRecord.
    # This invocation is expected to fail because the artifact was tampered.
    $ErrorActionPreference = 'Continue'
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $check -Out $output *> $null
    $tamperedExitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $previousPreference
  }
  if ($tamperedExitCode -eq 0) { throw 'Tampered generated contract unexpectedly passed check.' }
}
finally {
  Pop-Location
  if (Test-Path -LiteralPath $output) {
    Remove-Item -LiteralPath $output -Recurse -Force
  }
}

Write-Host 'Admin contract PowerShell scripts passed.'
