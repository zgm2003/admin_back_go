[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($PSVersionTable.PSVersion.Major -ne 7) {
  throw 'session secret rotation verification requires PowerShell 7'
}

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$go = (Get-Command go -ErrorAction Stop | Select-Object -First 1).Source

# The session module owns rotation and concurrent-node behavior. Database
# schema ownership is external to this repository; this test exercises only
# the runtime contract.
$output = @(& $go -C $repoRoot test ./internal/module/auth -run '^TestMultiNode' -race -count=1 -v 2>&1)
if ($LASTEXITCODE -ne 0) {
  throw 'session secret rotation rehearsal failed'
}

if (($output -join "`n") -match '(?i)(MYSQL_DSN|APP_SECRET|REDIS_PASSWORD|refresh_token|access_token)\s*[:=]') {
  throw 'session secret rotation rehearsal exposed sensitive output'
}

Write-Output 'session secret rotation rehearsal passed'
