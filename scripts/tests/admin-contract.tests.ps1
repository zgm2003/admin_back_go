[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$generate = Join-Path $repoRoot 'scripts\generate-admin-contract.ps1'
$check = Join-Path $repoRoot 'scripts\check-admin-contract.ps1'
$output = Join-Path ([System.IO.Path]::GetTempPath()) ('admin-contract-test-' + [Guid]::NewGuid().ToString('N'))

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
