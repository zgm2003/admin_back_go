[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
& pwsh -NoProfile -File (Join-Path $repoRoot 'scripts\database.ps1') check
if ($LASTEXITCODE -ne 0) { throw 'database baseline verification failed' }
