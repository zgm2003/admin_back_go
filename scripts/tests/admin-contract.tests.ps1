[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$generate = Join-Path $repoRoot 'scripts\generate-admin-contract.ps1'
$check = Join-Path $repoRoot 'scripts\check-admin-contract.ps1'
$output = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-contract-test-' + [Guid]::NewGuid().ToString('N'))
$rotationPath = Join-Path $repoRoot 'scripts\tests\session-secret-rotation.tests.ps1'

function Assert-True([bool]$Condition, [string]$Message) {
  if (-not $Condition) { throw $Message }
}

function Assert-ThrowsMessage {
  param(
    [Parameter(Mandatory = $true)][scriptblock]$Action,
    [Parameter(Mandatory = $true)][string]$Expected
  )

  try {
    & $Action
  }
  catch {
    Assert-True ([string]$_.Exception.Message -ceq $Expected) "unexpected failure message: $($_.Exception.Message)"
    return
  }
  throw "expected failure: $Expected"
}

$rotationTokens = $null
$rotationErrors = $null
$rotationAst = [Management.Automation.Language.Parser]::ParseFile(
  $rotationPath,
  [ref]$rotationTokens,
  [ref]$rotationErrors
)
Assert-True ($rotationErrors.Count -eq 0) 'session rotation script has PowerShell parse errors'
$rotationSource = [IO.File]::ReadAllText($rotationPath, [Text.Encoding]::UTF8)
$boundedCaptureAst = $rotationAst.Find({
  param($node)
  $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -ceq 'Invoke-BoundedProcessCapture'
}, $true)
Assert-True ($null -ne $boundedCaptureAst) 'session rotation must define reusable bounded process capture'
$sensitiveAssertionAst = $rotationAst.Find({
  param($node)
  $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -ceq 'Assert-NoSensitiveOutput'
}, $true)
Assert-True ($null -ne $sensitiveAssertionAst) 'session rotation must retain its sensitive-output assertion'
$startupGuardAst = $rotationAst.Find({
  param($node)
  $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -ceq 'Assert-NoAdminDevDatabaseMutation'
}, $true)
Assert-True ($null -ne $startupGuardAst) 'session rotation must define the admin-dev startup mutation guard'
$rekeyParserAst = $rotationAst.Find({
  param($node)
  $node -is [Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -ceq 'Invoke-MailDiagnosticRekey'
}, $true)
Assert-True ($null -ne $rekeyParserAst) 'session rotation must define the mail diagnostic rekey parser'

Invoke-Expression $sensitiveAssertionAst.Extent.Text
Invoke-Expression $boundedCaptureAst.Extent.Text
$testPwsh = Join-Path $PSHOME 'pwsh.exe'
$script:SensitiveValues = @()
$capture = Invoke-BoundedProcessCapture `
  -Executable $testPwsh `
  -Arguments @('-NoProfile', '-Command', '[Console]::Out.Write("bounded-out"); [Console]::Error.Write("bounded-err"); exit 7') `
  -Operation 'bounded command test' `
  -TimeoutSeconds 10 `
  -WorkingDirectory $repoRoot
Assert-True ($capture.ExitCode -eq 7) 'bounded capture must return expected nonzero exit status'
Assert-True ([string]$capture.StdOut -ceq 'bounded-out') 'bounded capture lost stdout'
Assert-True ([string]$capture.StdErr -ceq 'bounded-err') 'bounded capture lost stderr'

$timeoutWatch = [Diagnostics.Stopwatch]::StartNew()
Assert-ThrowsMessage {
  Invoke-BoundedProcessCapture `
    -Executable $testPwsh `
    -Arguments @('-NoProfile', '-Command', 'Start-Sleep -Seconds 10') `
    -Operation 'bounded command test' `
    -TimeoutSeconds 1 `
    -WorkingDirectory $repoRoot | Out-Null
} 'bounded command test timed out'
$timeoutWatch.Stop()
Assert-True ($timeoutWatch.Elapsed.TotalSeconds -lt 6) 'bounded capture did not terminate its process tree promptly'

$script:SensitiveValues = @('synthetic-sensitive-marker')
Assert-ThrowsMessage {
  Invoke-BoundedProcessCapture `
    -Executable $testPwsh `
    -Arguments @('-NoProfile', '-Command', 'synthetic-sensitive-marker') `
    -Operation 'bounded command test' `
    -TimeoutSeconds 10 `
    -WorkingDirectory $repoRoot | Out-Null
} 'rotation command arguments contain a sensitive runtime value'
$script:SensitiveValues = @()

Invoke-Expression $startupGuardAst.Extent.Text
$startupGuardRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-startup-guard-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($startupGuardRoot) | Out-Null
try {
  $safeStartupPath = Join-Path $startupGuardRoot 'safe.ps1'
  $unsafeMigrationPath = Join-Path $startupGuardRoot 'unsafe-migration.ps1'
  $unsafeRekeyPath = Join-Path $startupGuardRoot 'unsafe-rekey.ps1'
  [IO.File]::WriteAllText(
    $safeStartupPath,
    "`$migrationMessage = 'rekey is disabled'`nfunction Test-RekeyDisabled {}`nWrite-Host 'migrate apply is disabled'`nTest-RekeyDisabled`n& `$tool @('run', './cmd/admin-api')`n",
    [Text.UTF8Encoding]::new($false)
  )
  [IO.File]::WriteAllText(
    $unsafeMigrationPath,
    "& `$tool 'migrate' 'apply'`n",
    [Text.UTF8Encoding]::new($false)
  )
  [IO.File]::WriteAllText(
    $unsafeRekeyPath,
    "& `$tool 'run' './cmd/admin-db' 'mail-diagnostic-rekey'`n",
    [Text.UTF8Encoding]::new($false)
  )
  Assert-NoAdminDevDatabaseMutation -Paths @($safeStartupPath)
  Assert-ThrowsMessage {
    Assert-NoAdminDevDatabaseMutation -Paths @($unsafeMigrationPath)
  } 'admin-dev must not perform startup migration or rekey'
  Assert-ThrowsMessage {
    Assert-NoAdminDevDatabaseMutation -Paths @($unsafeRekeyPath)
  } 'admin-dev must not perform startup migration or rekey'
}
finally {
  Remove-Item -LiteralPath $startupGuardRoot -Recurse -Force
}

Invoke-Expression $rekeyParserAst.Extent.Text
$script:RekeyCaptureResult = $null
function Invoke-GoCapture {
  param(
    [string[]]$Arguments,
    [int]$TimeoutSeconds,
    [string]$Operation
  )
  return $script:RekeyCaptureResult
}
$previousAppSecretPreviousForParser = [Environment]::GetEnvironmentVariable('APP_SECRET_PREVIOUS', 'Process')
try {
  $env:APP_SECRET_PREVIOUS = 'synthetic-previous-root-present'
  $script:RekeyCaptureResult = [pscustomobject]@{
    ExitCode = 0
    StdOut = "rekeyed_row_id`t17`ncurrent_key_id`tmail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA`nprevious_key_id`tmail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBB`nscanned`t1`nrekeyed`t1`nprevious_references`t0`nunknown_references`t0`n"
    StdErr = ''
    StdOutLines = @(
      "rekeyed_row_id`t17",
      "current_key_id`tmail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA",
      "previous_key_id`tmail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBB",
      "scanned`t1",
      "rekeyed`t1",
      "previous_references`t0",
      "unknown_references`t0"
    )
    StdErrLines = @()
    Lines = @(
      "rekeyed_row_id`t17",
      "current_key_id`tmail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA",
      "previous_key_id`tmail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBB",
      "scanned`t1",
      "rekeyed`t1",
      "previous_references`t0",
      "unknown_references`t0"
    )
  }
  $parsedRekey = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 1 -ExpectedRekeyed 1
  Assert-True ($parsedRekey.RekeyedRowIDs.Count -eq 1 -and $parsedRekey.RekeyedRowIDs[0] -eq 17) 'rekey parser lost the exact rekeyed row id'

  $script:RekeyCaptureResult = [pscustomobject]@{
    ExitCode = 1
    StdOut = ''
    StdErr = "unrelated command failure`n"
    StdOutLines = @()
    StdErrLines = @('unrelated command failure')
    Lines = @()
  }
  Assert-ThrowsMessage {
    Invoke-MailDiagnosticRekey -ExpectSuccess $false | Out-Null
  } 'mail diagnostic rekey failure output violated the safe sentinel contract'

  $script:RekeyCaptureResult = [pscustomobject]@{
    ExitCode = 1
    StdOut = ''
    StdErr = "mail diagnostic rekey command: failed`nexit status 1`n"
    StdOutLines = @()
    StdErrLines = @('mail diagnostic rekey command: failed', 'exit status 1')
    Lines = @()
  }
  $safeFailure = Invoke-MailDiagnosticRekey -ExpectSuccess $false
  Assert-True ($safeFailure.ExitCode -eq 1) 'rekey parser rejected the fixed safe failure sentinel'

  $script:RekeyCaptureResult = [pscustomobject]@{
    ExitCode = 0
    StdOut = ''
    StdErr = ''
    StdOutLines = @()
    StdErrLines = @()
    Lines = @(
      "rekeyed_row_id`t17",
      "rekeyed_row_id`t17",
      "current_key_id`tmail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA",
      "previous_key_id`tmail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBB",
      "scanned`t2",
      "rekeyed`t2",
      "previous_references`t0",
      "unknown_references`t0"
    )
  }
  Assert-ThrowsMessage {
    Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 2 -ExpectedRekeyed 2 | Out-Null
  } 'mail diagnostic rekey repeated a row id'
}
finally {
  [Environment]::SetEnvironmentVariable('APP_SECRET_PREVIOUS', $previousAppSecretPreviousForParser, 'Process')
  $script:RekeyCaptureResult = $null
}

foreach ($requiredRotationMarker in @(
  '[System.Diagnostics.ProcessStartInfo]::new()',
  '.ArgumentList.Add(',
  '.ReadToEndAsync()',
  '.WaitForExit(',
  '.Kill($true)',
  'mail diagnostic rekey command: failed',
  'verify-unknown-unchanged',
  'verify-corrupt-unchanged',
  'crypto/sha256',
  'rekeyed_row_id',
  'function Assert-NoAdminDevDatabaseMutation',
  '$script:SensitiveValues = @()'
)) {
  Assert-True ($rotationSource.Contains($requiredRotationMarker, [StringComparison]::Ordinal)) "session rotation is missing quality marker: $requiredRotationMarker"
}
foreach ($unboundedInvocation in @('@(& $script:GoExecutable', '@(& $script:DockerExecutable')) {
  Assert-True (-not $rotationSource.Contains($unboundedInvocation, [StringComparison]::Ordinal)) "session rotation retains unbounded invocation: $unboundedInvocation"
}
$sensitiveClearIndex = $rotationSource.LastIndexOf('$script:SensitiveValues = @()', [StringComparison]::Ordinal)
$summarySerializationIndex = $rotationSource.LastIndexOf('$summaryData | ConvertTo-Json -Compress', [StringComparison]::Ordinal)
Assert-True ($sensitiveClearIndex -ge 0 -and $summarySerializationIndex -gt $sensitiveClearIndex) 'session rotation must serialize its success summary only after cleanup clears sensitive references'

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

  $openapi = Get-Content -Raw -LiteralPath (Join-Path $output 'openapi.json') | ConvertFrom-Json
  $permissions = Get-Content -Raw -LiteralPath (Join-Path $output 'permissions.json') | ConvertFrom-Json
  $views = Get-Content -Raw -LiteralPath (Join-Path $output 'views.json') | ConvertFrom-Json

  $expectedRoutes = @(
    [pscustomobject]@{ OpenAPIPath = '/api/admin/v1/mail/logs'; RuntimePath = '/api/admin/v1/mail/logs'; Action = 'list_logs' },
    [pscustomobject]@{ OpenAPIPath = '/api/admin/v1/mail/logs/{id}'; RuntimePath = '/api/admin/v1/mail/logs/:id'; Action = 'view_log' }
  )
  foreach ($expected in $expectedRoutes) {
    $pathProperty = $openapi.paths.PSObject.Properties[$expected.OpenAPIPath]
    if ($null -eq $pathProperty -or $null -eq $pathProperty.Value.get) {
      throw "Generated OpenAPI is missing GET $($expected.OpenAPIPath)."
    }
    $operation = $pathProperty.Value.get
    $access = $operation.PSObject.Properties['x-admin-access'].Value
    $audit = $operation.PSObject.Properties['x-admin-audit'].Value
    if ([string]$access.kind -cne 'permission' -or [string]$access.permission_code -cne 'system_mail_logView') {
      throw "Generated OpenAPI mail diagnostic access policy is invalid for $($expected.OpenAPIPath)."
    }
    if (-not [bool]$audit.enabled -or -not [bool]$audit.required -or
        [string]$audit.module -cne 'mail' -or [string]$audit.action -cne $expected.Action -or
        -not [bool]$audit.skip_request_payload -or -not [bool]$audit.skip_response_payload) {
      throw "Generated OpenAPI mail diagnostic audit policy is invalid for $($expected.OpenAPIPath)."
    }

    $published = @($permissions.operations | Where-Object {
      [string]$_.method -ceq 'GET' -and [string]$_.path -ceq $expected.RuntimePath
    })
    if ($published.Count -ne 1 -or
        [string]$published[0].access.kind -cne 'permission' -or
        [string]$published[0].access.permission_code -cne 'system_mail_logView' -or
        -not [bool]$published[0].audit.required -or
        -not [bool]$published[0].audit.skip_request_payload -or
        -not [bool]$published[0].audit.skip_response_payload) {
      throw "Generated permission route policy is invalid for GET $($expected.RuntimePath)."
    }
  }

  $logSchemaProperty = $openapi.components.schemas.PSObject.Properties['Go_internal_module_mail_LogDTO_Output']
  if ($null -eq $logSchemaProperty) { throw 'Generated OpenAPI is missing the mail log output schema.' }
  $logSchema = $logSchemaProperty.Value
  foreach ($field in @('verification_code', 'verification_code_status', 'verification_code_expires_at')) {
    if (@($logSchema.required) -cnotcontains $field) {
      throw "Generated mail log schema does not require $field."
    }
    $property = $logSchema.properties.PSObject.Properties[$field]
    if ($null -eq $property -or @($property.Value.anyOf).Count -ne 2 -or
        @($property.Value.anyOf | Where-Object { [string]$_.type -ceq 'null' }).Count -ne 1) {
      throw "Generated mail log schema does not publish nullable $field."
    }
  }
  foreach ($forbidden in @('key_id', 'code_enc', 'ciphertext', 'template_data', 'provider', 'body')) {
    if ($null -ne $logSchema.properties.PSObject.Properties[$forbidden]) {
      throw "Generated mail log schema publishes forbidden field $forbidden."
    }
  }

  $buttonCodes = @($views.users_me.response_schema.properties.buttonCodes.items.enum)
  if ($buttonCodes -cnotcontains 'system_mail_logView') {
    throw 'Generated users/me view contract does not publish system_mail_logView.'
  }

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
