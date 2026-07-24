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
$headerMapSource = (Get-RequiredFunctionAst -Name 'Get-HttpResponseHeaderMap').Extent.Text
$payloadFreeLogSource = (Get-RequiredFunctionAst -Name 'Wait-NewPayloadFreeMailOperationLog').Extent.Text
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

Invoke-Expression $headerMapSource
$webHeaders = [System.Net.WebHeaderCollection]::new()
$webHeaders['Cache-Control'] = 'no-store, private'
$webHeaders['Pragma'] = 'no-cache'
$headerMap = Get-HttpResponseHeaderMap ([pscustomobject]@{ Headers = $webHeaders })
if ([string]$headerMap['Cache-Control'] -cne 'no-store, private' -or
    [string]$headerMap['Pragma'] -cne 'no-cache') {
  throw 'Get-HttpResponseHeaderMap did not preserve WebHeaderCollection values'
}

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

# Exercise the real payload-free audit waiter with an in-memory operation-log response.
$script:PayloadFreeOperationLogResponse = $null
function Get-OperationLogList([string]$BaseURL, [hashtable]$Headers, [string]$Action) {
  return $script:PayloadFreeOperationLogResponse
}
Invoke-Expression $payloadFreeLogSource

$script:PayloadFreeOperationLogResponse = [pscustomobject]@{
  code = 0
  data = [pscustomobject]@{
    list = @([pscustomobject]@{
      id = 101
      request_data = '{"module":"mail","action":"list_logs"}'
      response_data = '{"status":"ok"}'
      is_success = 1
    })
  }
}
$payloadFreeLogID = Wait-NewPayloadFreeMailOperationLog `
  -BaseURL 'http://behavior.invalid' `
  -Headers @{} `
  -Action 'list_logs' `
  -AfterID 100
if ($payloadFreeLogID -ne 101) {
  throw 'payload-free mail operation-log waiter did not return the matching log id'
}

$script:PayloadFreeOperationLogResponse.data.list[0].request_data = '{"module":"mail","action":"list_logs","payload":{"fixture":"not-secret"}}'
Assert-Throws {
  Wait-NewPayloadFreeMailOperationLog `
    -BaseURL 'http://behavior.invalid' `
    -Headers @{} `
    -Action 'list_logs' `
    -AfterID 100 | Out-Null
} 'payload-free mail operation-log waiter accepted a request payload'

$script:PayloadFreeOperationLogResponse.data.list[0].request_data = '{"module":"mail","action":"list_logs"}'
$script:PayloadFreeOperationLogResponse.data.list[0].response_data = '{"status":"ok","payload":{"fixture":"not-secret"}}'
Assert-Throws {
  Wait-NewPayloadFreeMailOperationLog `
    -BaseURL 'http://behavior.invalid' `
    -Headers @{} `
    -Action 'list_logs' `
    -AfterID 100 | Out-Null
} 'payload-free mail operation-log waiter accepted a response payload'
$script:PayloadFreeOperationLogResponse = $null

# Execute the smoke function itself with fake HTTP and audit dependencies. Any
# response body that leaks into the pipeline makes the single-summary assertions fail.
Invoke-Expression $diagnosticSmokeSource
$script:MailBehaviorMode = ''
$script:MailRequestCalls = [Collections.Generic.List[object]]::new()
$script:MailOperationLogActions = [Collections.Generic.List[string]]::new()
$script:MailOperationLogBaseline = 40
$script:MailFixtureItem = [pscustomobject]@{
  id = 73
  scene = 'login'
  to_email = 'behavior@example.test'
  subject = 'behavior fixture'
  status = 2
  tencent_request_id = 'behavior-request'
  tencent_message_id = 'behavior-message'
  error_code = ''
  error_message = ''
  duration_ms = 9
  created_at = '2026-07-24 11:00:00'
  verification_code = 'fixture-code-135790'
  verification_code_status = 'not_expired'
  verification_code_expires_at = '2026-07-24 11:05:00'
}

function Invoke-JsonRequestAllowFailure(
  [string]$Method,
  [string]$URL,
  [hashtable]$Headers,
  $Body,
  [switch]$CaptureHttpResponse,
  [switch]$DiscardBody
) {
  $script:MailRequestCalls.Add([pscustomobject]@{
    Method = $Method
    URL = $URL
    CaptureHttpResponse = [bool]$CaptureHttpResponse
    DiscardBody = [bool]$DiscardBody
  })
  if (-not $CaptureHttpResponse) {
    throw 'mail diagnostic behavior request did not capture the HTTP response'
  }

  if ($script:MailBehaviorMode -ceq 'denied') {
    if (-not $DiscardBody) {
      throw 'mail diagnostic denied behavior request did not discard its body'
    }
    return [pscustomobject]@{ StatusCode = 403; Headers = @{}; Body = $null }
  }
  if ($DiscardBody) {
    throw 'mail diagnostic authorized behavior request discarded its body'
  }

  $responseHeaders = @{ 'Cache-Control' = 'no-store, private'; 'Pragma' = 'no-cache' }
  if ($script:MailBehaviorMode -ceq 'missing-header') {
    $responseHeaders.Remove('Pragma')
  }
  if ($URL -match '/mail/logs\?') {
    return [pscustomobject]@{
      StatusCode = 200
      Headers = $responseHeaders
      Body = [pscustomobject]@{
        code = 0
        data = [pscustomobject]@{
          page = [pscustomobject]@{ total = 1 }
          list = @($script:MailFixtureItem)
        }
      }
    }
  }

  $detail = $script:MailFixtureItem.PSObject.Copy()
  if ($script:MailBehaviorMode -ceq 'tuple-mismatch') {
    $detail.verification_code_status = 'expired'
  }
  return [pscustomobject]@{
    StatusCode = 200
    Headers = $responseHeaders
    Body = [pscustomobject]@{ code = 0; data = $detail }
  }
}

function Get-OperationLogList([string]$BaseURL, [hashtable]$Headers, [string]$Action) {
  $script:MailOperationLogBaseline++
  return [pscustomobject]@{
    code = 0
    data = [pscustomobject]@{ list = @([pscustomobject]@{ id = $script:MailOperationLogBaseline }) }
  }
}

function Get-MaxOperationLogID($Response) {
  return [int64]$Response.data.list[0].id
}

function Wait-NewPayloadFreeMailOperationLog(
  [string]$BaseURL,
  [hashtable]$Headers,
  [string]$Action,
  [int64]$AfterID
) {
  if ($Action -notin @('list_logs', 'view_log') -or $AfterID -le 0) {
    throw 'mail diagnostic behavior requested an invalid payload-free audit wait'
  }
  $script:MailOperationLogActions.Add($Action)
  return $AfterID + 100
}

function Reset-MailBehavior([string]$Mode) {
  $script:MailBehaviorMode = $Mode
  $script:MailRequestCalls.Clear()
  $script:MailOperationLogActions.Clear()
}

$behaviorHeaders = @{ Authorization = 'Bearer synthetic-non-secret-token' }
Reset-MailBehavior 'denied'
$deniedOutput = @(Invoke-MailDiagnosticSmoke `
  -BaseURL 'http://behavior.invalid' `
  -Headers $behaviorHeaders `
  -ButtonCodes @())
if ($deniedOutput.Count -ne 1 -or $deniedOutput[0].ListStatus -ne 403 -or $deniedOutput[0].DetailStatus -ne 403) {
  throw 'mail diagnostic denied behavior did not return one status-only summary'
}
if ($script:MailRequestCalls.Count -ne 2 -or
    @($script:MailRequestCalls | Where-Object { -not $_.DiscardBody }).Count -ne 0 -or
    $script:MailOperationLogActions.Count -ne 0) {
  throw 'mail diagnostic denied behavior did not discard both bodies without audit polling'
}

Reset-MailBehavior 'authorized'
$authorizedOutput = @(Invoke-MailDiagnosticSmoke `
  -BaseURL 'http://behavior.invalid' `
  -Headers $behaviorHeaders `
  -ButtonCodes @('system_mail_logView') `
  -ExpectMailDiagnosticAccess)
if ($authorizedOutput.Count -ne 1 -or
    $authorizedOutput[0].ListStatus -ne 200 -or
    $authorizedOutput[0].DetailStatus -ne 200 -or
    $authorizedOutput[0].ListCount -ne 1 -or
    $authorizedOutput[0].OperationLogCount -ne 2) {
  throw 'mail diagnostic authorized behavior did not return one validated summary'
}
if (($script:MailOperationLogActions -join ',') -cne 'list_logs,view_log') {
  throw 'mail diagnostic authorized behavior did not require both payload-free audit records'
}

Reset-MailBehavior 'tuple-mismatch'
Assert-Throws {
  Invoke-MailDiagnosticSmoke `
    -BaseURL 'http://behavior.invalid' `
    -Headers $behaviorHeaders `
    -ButtonCodes @('system_mail_logView') `
    -ExpectMailDiagnosticAccess | Out-Null
} 'mail diagnostic behavior accepted a list/detail tuple mismatch'

Reset-MailBehavior 'missing-header'
Assert-Throws {
  Invoke-MailDiagnosticSmoke `
    -BaseURL 'http://behavior.invalid' `
    -Headers $behaviorHeaders `
    -ButtonCodes @('system_mail_logView') `
    -ExpectMailDiagnosticAccess | Out-Null
} 'mail diagnostic behavior accepted a response without required cache headers'

Write-Output 'full-admin-smoke mail diagnostic source assertions passed'
