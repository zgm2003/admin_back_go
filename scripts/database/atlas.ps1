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

$mounts = @('--volume', "${root}:/workspace:ro")
if ($Command -eq 'migrate' -and $Arguments.Count -gt 0 -and $Arguments[0] -eq 'hash') {
  $migrations = Join-Path $root 'database\migrations'
  $mounts += @('--volume', "${migrations}:/workspace/database/migrations:rw")
}

& docker run --rm --network none @mounts --workdir /workspace $image $Command @Arguments
if ($LASTEXITCODE -ne 0) {
  throw "Atlas exited with code $LASTEXITCODE"
}
