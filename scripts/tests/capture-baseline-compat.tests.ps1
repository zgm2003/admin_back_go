$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$scriptPath = Join-Path $repoRoot 'scripts\database\capture-baseline.ps1'
$content = [IO.File]::ReadAllText($scriptPath)

if ($content.Contains('[System.IO.File]::Move($temporaryPath,[System.IO.Path]::GetFullPath($OutputPath),$true)')) {
  throw 'capture-baseline still requires the unavailable three-argument File.Move overload'
}
if (-not $content.Contains('function Move-FileWithOverwrite')) {
  throw 'capture-baseline is missing its compatible overwrite helper'
}
if (-not $content.Contains("GetMethod('Move'")) {
  throw 'capture-baseline must detect overwrite support at runtime'
}

Write-Output 'capture-baseline compatibility assertions passed'
