[CmdletBinding()]
param(
  [Parameter(Mandatory = $true, Position = 0)]
  [ValidateSet('migrate', 'schema')]
  [string]$Command,

  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$Arguments
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$image = 'arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a'

if ($null -eq (Get-Command docker -ErrorAction SilentlyContinue)) {
  throw 'Docker is required to run the pinned Atlas container.'
}

& docker run --rm --network none --volume "${root}:/workspace:ro" --workdir /workspace $image $Command @Arguments
if ($LASTEXITCODE -ne 0) {
  throw "Atlas exited with code $LASTEXITCODE"
}
