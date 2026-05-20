$ErrorActionPreference = 'Stop'
$script = Get-Content -Raw (Join-Path $PSScriptRoot '..\full-admin-smoke.ps1')

$forbiddenEnvGate = ('COS','STS','ENABLED') -join '_'
$forbiddenSkip = ('skipped','cos','sts','disabled') -join '_'
if ($script -match [regex]::Escape($forbiddenEnvGate)) {
  throw 'full-admin-smoke must not gate upload token probe on removed COS STS env key'
}
if ($script -match [regex]::Escape($forbiddenSkip)) {
  throw 'full-admin-smoke must not return the old COS STS disabled skip status'
}
foreach ($expected in @('skipped_upload_setting_missing', 'skipped_upload_setting_not_usable', 'Invoke-JsonRequestAllowFailure')) {
  if ($script -notmatch [regex]::Escape($expected)) {
    throw "full-admin-smoke missing expected upload token behavior marker: $expected"
  }
}

Write-Output 'full-admin-smoke upload token env cleanup assertions passed'
