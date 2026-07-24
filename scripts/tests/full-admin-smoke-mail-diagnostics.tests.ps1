[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$scriptPath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\full-admin-smoke.ps1'))
$tokens = $null
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -ne 0) {
  throw 'full-admin-smoke.ps1 has PowerShell parse errors'
}

function Get-RequiredFunctionAst {
  param([Parameter(Mandatory = $true)][string]$Name)

  $functionAst = $ast.Find({
    param($node)
    $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -ceq $Name
  }, $true)
  if ($null -eq $functionAst) {
    throw "full-admin-smoke.ps1 is missing $Name"
  }
  return $functionAst
}

function Assert-Contains {
  param(
    [Parameter(Mandatory = $true)][string]$Content,
    [Parameter(Mandatory = $true)][string]$Needle,
    [Parameter(Mandatory = $true)][string]$Message
  )

  if (-not $Content.Contains($Needle, [StringComparison]::Ordinal)) {
    throw $Message
  }
}

function Assert-NotContains {
  param(
    [Parameter(Mandatory = $true)][string]$Content,
    [Parameter(Mandatory = $true)][string]$Needle,
    [Parameter(Mandatory = $true)][string]$Message
  )

  if ($Content.Contains($Needle, [StringComparison]::OrdinalIgnoreCase)) {
    throw $Message
  }
}

function Assert-Throws {
  param(
    [Parameter(Mandatory = $true)][scriptblock]$Action,
    [Parameter(Mandatory = $true)][string]$Message
  )

  try {
    & $Action
  }
  catch {
    return
  }
  throw $Message
}

$mailLogsAst = Get-RequiredFunctionAst -Name 'Assert-MailLogs'
$mailLogsSource = $mailLogsAst.Extent.Text
$smsLogsSource = (Get-RequiredFunctionAst -Name 'Assert-SmsLogs').Extent.Text
$diagnosticSmokeSource = (Get-RequiredFunctionAst -Name 'Invoke-MailDiagnosticSmoke').Extent.Text
$scriptSource = [IO.File]::ReadAllText($scriptPath, [Text.Encoding]::UTF8)

Assert-Contains $scriptSource '[switch]$ExpectMailDiagnosticAccess' 'full-admin-smoke must expose the explicit mail diagnostic access switch'
foreach ($marker in @(
  'Invoke-JsonRequestAllowFailure',
  '/api/admin/v1/mail/logs?current_page=1&page_size=20',
  '/api/admin/v1/mail/logs/1',
  'ExpectMailDiagnosticAccess',
  '403',
  'no-store, private',
  'no-cache',
  'verification_code',
  'verification_code_status',
  'verification_code_expires_at',
  'ListCount',
  'DetailStatus'
)) {
  Assert-Contains $diagnosticSmokeSource $marker "mail diagnostic smoke is missing required marker: $marker"
}
foreach ($marker in @('role_permissions', 'permission_ids', 'grant', 'ConvertTo-Json', 'Write-Host', 'Write-Output', 'Out-File', 'Set-Content')) {
  Assert-NotContains $diagnosticSmokeSource $marker "mail diagnostic smoke must not mutate authorization or serialize/print/store diagnostic bodies: $marker"
}
$deniedBranchStart = $diagnosticSmokeSource.IndexOf('if (-not $ExpectMailDiagnosticAccess) {', [StringComparison]::Ordinal)
$deniedBranchEnd = $diagnosticSmokeSource.IndexOf('return [pscustomobject]@{', $deniedBranchStart, [StringComparison]::Ordinal)
if ($deniedBranchStart -lt 0 -or $deniedBranchEnd -le $deniedBranchStart) {
  throw 'mail diagnostic smoke is missing the default denied branch'
}
$deniedBranchSource = $diagnosticSmokeSource.Substring($deniedBranchStart, $deniedBranchEnd - $deniedBranchStart)
Assert-NotContains $deniedBranchSource '& $assertNoStore' 'mail diagnostic denied requests must not require handler-owned no-store headers'
if ($diagnosticSmokeSource -notmatch '(?m)\$\w*(List|Detail)Response\s*=\s*\$null') {
  throw 'mail diagnostic smoke must clear in-memory response objects after validation'
}

foreach ($field in @('verification_code', 'verification_code_status', 'verification_code_expires_at')) {
  Assert-Contains $mailLogsSource "'$field'" "Assert-MailLogs must require approved diagnostic field $field"
}
Assert-NotContains $mailLogsSource 'ConvertTo-Json' 'Assert-MailLogs must not serialize diagnostic response bodies'
Assert-NotContains $mailLogsSource 'Assert-ApiOK' 'Assert-MailLogs must not call a helper that can serialize diagnostic response bodies'
if ($mailLogsSource -match "(?s)forbidden.*'verify_code'") {
  throw 'Assert-MailLogs must not reject the approved mail diagnostic verification code'
}
Assert-Contains $smsLogsSource "'verify_code'" 'Assert-SmsLogs must continue rejecting verify_code'

# Load only the function under test. The smoke script itself must never execute in this source test.
function Assert-ApiOK($Response, [string]$Label) {
  if ($Response.code -ne 0) { throw "$Label failed" }
}
function Get-ObjectArray($Value) {
  if ($null -eq $Value) { return @() }
  return @($Value)
}
function Test-HasProperty($Value, [string]$Name) {
  if ($null -eq $Value) { return $false }
  return @($Value.PSObject.Properties.Name) -contains $Name
}
function ConvertTo-Json {
  throw 'Assert-MailLogs attempted to serialize a diagnostic response body'
}
Invoke-Expression $mailLogsSource

$validItem = [pscustomobject]@{
  id = 17
  scene = 'login'
  to_email = 'operator@example.test'
  subject = 'verification'
  status = 2
  tencent_request_id = 'request-id'
  tencent_message_id = 'message-id'
  error_code = ''
  error_message = ''
  duration_ms = 10
  created_at = '2026-07-24 10:00:00'
  verification_code = 'test-value-never-print'
  verification_code_status = 'not_expired'
  verification_code_expires_at = '2026-07-24 10:05:00'
  verify_code = 'legacy-name-must-not-be-rejected'
}
$validResponse = [pscustomobject]@{
  code = 0
  data = [pscustomobject]@{
    page = [pscustomobject]@{ total = 1 }
    list = @($validItem)
  }
}
$summary = Assert-MailLogs $validResponse
if ($summary.ListCount -ne 1 -or $summary.Total -ne 1) {
  throw 'Assert-MailLogs returned an invalid summary'
}

foreach ($missing in @('verification_code', 'verification_code_status', 'verification_code_expires_at')) {
  $item = $validItem.PSObject.Copy()
  $item.PSObject.Properties.Remove($missing)
  $response = [pscustomobject]@{
    code = 0
    data = [pscustomobject]@{ page = [pscustomobject]@{ total = 1 }; list = @($item) }
  }
  Assert-Throws { Assert-MailLogs $response | Out-Null } "Assert-MailLogs accepted an item without $missing"
}

$leakingItem = $validItem.PSObject.Copy()
$leakingItem | Add-Member -NotePropertyName 'template_data' -NotePropertyValue 'must-not-be-visible'
$leakingResponse = [pscustomobject]@{
  code = 0
  data = [pscustomobject]@{ page = [pscustomobject]@{ total = 1 }; list = @($leakingItem) }
}
Assert-Throws { Assert-MailLogs $leakingResponse | Out-Null } 'Assert-MailLogs accepted a forbidden mail diagnostic field'

Write-Output 'full-admin-smoke mail diagnostic source assertions passed'
