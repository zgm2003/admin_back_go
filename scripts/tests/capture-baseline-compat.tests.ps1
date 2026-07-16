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

$tokens = $null
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$errors)
if ($errors.Count -ne 0) { throw 'capture-baseline could not be parsed' }
$functionAst = $ast.Find({
  param($node)
  $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -ceq 'Move-FileWithOverwrite'
}, $true)
if ($null -eq $functionAst) { throw 'overwrite helper AST was not found' }
Invoke-Expression $functionAst.Extent.Text

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-capture-baseline-compat-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($testRoot) | Out-Null
try {
  $source = Join-Path $testRoot 'source.tmp'
  $destination = Join-Path $testRoot 'baseline.json'
  [IO.File]::WriteAllText($source, 'new')
  [IO.File]::WriteAllText($destination, 'old')
  Move-FileWithOverwrite -SourcePath $source -DestinationPath $destination
  if ([IO.File]::ReadAllText($destination) -cne 'new') { throw 'overwrite helper did not replace destination content' }
  if ([IO.File]::Exists($source)) { throw 'overwrite helper left the source file behind' }
} finally {
  Remove-Item -LiteralPath $testRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Output 'capture-baseline compatibility assertions passed'
