$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$script:initializer = Join-Path $repoRoot 'deploy\docker-first\init-local-env.ps1'
$templatePath = Join-Path $repoRoot 'deploy\docker-first\admin-go.env.example'
$readmePath = Join-Path $repoRoot 'deploy\docker-first\README.md'

if (-not (Test-Path -LiteralPath $script:initializer -PathType Leaf)) {
  throw 'init-local-env.ps1 is missing'
}

function Assert-True {
  param(
    [Parameter(Mandatory = $true)][bool]$Condition,
    [Parameter(Mandatory = $true)][string]$Message
  )

  if (-not $Condition) {
    throw $Message
  }
}

function Assert-False {
  param(
    [Parameter(Mandatory = $true)][bool]$Condition,
    [Parameter(Mandatory = $true)][string]$Message
  )

  if ($Condition) {
    throw $Message
  }
}

function Assert-Equal {
  param(
    [Parameter(Mandatory = $true)]$Expected,
    [Parameter(Mandatory = $true)]$Actual,
    [Parameter(Mandatory = $true)][string]$Message
  )

  if ($Expected -cne $Actual) {
    throw $Message
  }
}

function Get-AppSecret {
  param([Parameter(Mandatory = $true)][string]$Content)

  $matches = [regex]::Matches($Content, '(?m)^APP_SECRET=([^\r\n]*)(?=\r?$)')
  Assert-Equal 1 $matches.Count 'generated env must contain exactly one APP_SECRET line'
  return $matches[0].Groups[1].Value
}

function Write-Utf8NoBom {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Content
  )

  [IO.File]::WriteAllText($Path, $Content, (New-Object Text.UTF8Encoding($false)))
}

function Get-FileSystemAclExtensionsType {
  $extensionsType = 'System.IO.FileSystemAclExtensions' -as [type]
  if ($null -eq $extensionsType) {
    foreach ($assembly in [AppDomain]::CurrentDomain.GetAssemblies()) {
      $extensionsType = $assembly.GetType('System.IO.FileSystemAclExtensions')
      if ($null -ne $extensionsType) {
        break
      }
    }
  }
  if ($null -eq $extensionsType) {
    try {
      $aclAssembly = [Reflection.Assembly]::Load('System.IO.FileSystem.AccessControl')
      $extensionsType = $aclAssembly.GetType('System.IO.FileSystemAclExtensions')
    }
    catch {
    }
  }
  if ($null -eq $extensionsType) {
    throw 'test runtime must expose access-only file ACL APIs'
  }
  return $extensionsType
}

function Get-AccessOnlyFileAcl {
  param([Parameter(Mandatory = $true)][string]$Path)

  $fileInfo = New-Object IO.FileInfo($Path)
  $getAccessControl = $fileInfo.GetType().GetMethod(
    'GetAccessControl',
    [type[]]@([Security.AccessControl.AccessControlSections])
  )
  if ($null -ne $getAccessControl) {
    $arguments = New-Object 'object[]' 1
    $arguments[0] = [Security.AccessControl.AccessControlSections]::Access
    return [Security.AccessControl.FileSecurity]$getAccessControl.Invoke($fileInfo, $arguments)
  }

  $extensionsType = Get-FileSystemAclExtensionsType
  $getAccessControl = $extensionsType.GetMethod(
    'GetAccessControl',
    [type[]]@([IO.FileInfo], [Security.AccessControl.AccessControlSections])
  )
  if ($null -eq $getAccessControl) {
    throw 'test runtime must expose access-only file ACL APIs'
  }
  $arguments = New-Object 'object[]' 2
  $arguments[0] = $fileInfo
  $arguments[1] = [Security.AccessControl.AccessControlSections]::Access
  return [Security.AccessControl.FileSecurity]$getAccessControl.Invoke($null, $arguments)
}

function Set-AccessOnlyFileAcl {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][Security.AccessControl.FileSecurity]$Acl
  )

  $fileInfo = New-Object IO.FileInfo($Path)
  $setAccessControl = $fileInfo.GetType().GetMethod(
    'SetAccessControl',
    [type[]]@([Security.AccessControl.FileSecurity])
  )
  if ($null -ne $setAccessControl) {
    $arguments = New-Object 'object[]' 1
    $arguments[0] = $Acl
    [void]$setAccessControl.Invoke($fileInfo, $arguments)
    return
  }

  $extensionsType = Get-FileSystemAclExtensionsType
  $setAccessControl = $extensionsType.GetMethod(
    'SetAccessControl',
    [type[]]@([IO.FileInfo], [Security.AccessControl.FileSecurity])
  )
  if ($null -eq $setAccessControl) {
    throw 'test runtime must expose access-only file ACL APIs'
  }
  $arguments = New-Object 'object[]' 2
  $arguments[0] = $fileInfo
  $arguments[1] = $Acl
  [void]$setAccessControl.Invoke($null, $arguments)
}

function Assert-OwnerOnlyPermissions {
  param([Parameter(Mandatory = $true)][string]$Path)

  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = Get-Acl -LiteralPath $Path
    Assert-True $acl.AreAccessRulesProtected 'runtime env ACL must not inherit access rules'
    Assert-Equal $currentSid.Value $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value 'runtime env owner must be the current user'

    $hasCurrentUserFullControl = $false
    $rules = $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])
    foreach ($rule in $rules) {
      if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) {
        continue
      }
      if ($rule.IdentityReference.Value -cne $currentSid.Value) {
        throw 'runtime env ACL must not grant Allow access to another principal'
      }
      if ($rule.IdentityReference.Value -ceq $currentSid.Value -and
          ($rule.FileSystemRights -band [Security.AccessControl.FileSystemRights]::FullControl) -eq [Security.AccessControl.FileSystemRights]::FullControl) {
        $hasCurrentUserFullControl = $true
      }
    }
    Assert-True $hasCurrentUserFullControl 'runtime env ACL must grant only the current user full control'
    return
  }

  $unixFileModeType = [IO.File].Assembly.GetType('System.IO.UnixFileMode')
  Assert-True ($null -ne $unixFileModeType) 'runtime must expose UnixFileMode for owner-only permission verification'
  $getUnixFileMode = [IO.File].GetMethod('GetUnixFileMode', [type[]]@([string]))
  Assert-True ($null -ne $getUnixFileMode) 'runtime must expose File.GetUnixFileMode for owner-only permission verification'
  $mode = [int]$getUnixFileMode.Invoke($null, [object[]]@($Path))
  Assert-Equal 384 ($mode -band 511) 'runtime env Unix permissions must be 0600'
}

function Assert-NoInitializerTemporaryFiles {
  $temporaryFiles = @(
    Get-ChildItem -LiteralPath $script:tempRoot -Recurse -Force -File -ErrorAction Stop |
      Where-Object { $_.Name -match '^\..+\.[0-9a-f]{32}\.tmp$' }
  )
  Assert-Equal 0 $temporaryFiles.Count 'initializer must not leave temporary runtime env files behind'
}

function Invoke-SuccessInitializer {
  param(
    [Parameter(Mandatory = $true)][string]$OutputPath,
    [Parameter(Mandatory = $true)][string]$MySQLDSN,
    [Parameter(Mandatory = $true)][string]$RedisAddress,
    [Parameter(Mandatory = $true)][string]$CorsOrigin,
    [string]$InitializerPath = $script:initializer
  )

  $messages = @(
    & $InitializerPath `
      -OutputPath $OutputPath `
      -MySQLDSN $MySQLDSN `
      -RedisAddress $RedisAddress `
      -CorsOrigin $CorsOrigin 2>&1 |
      ForEach-Object { [string]$_ }
  )
  Assert-OwnerOnlyPermissions $OutputPath
  Assert-NoInitializerTemporaryFiles
  return $messages
}

function Invoke-RejectedInitializer {
  param(
    [Parameter(Mandatory = $true)][string]$CaseName,
    [Parameter(Mandatory = $true)][string]$OutputPath,
    [Parameter(Mandatory = $true)][string]$MySQLDSN,
    [Parameter(Mandatory = $true)][string]$RedisAddress,
    [Parameter(Mandatory = $true)][string]$CorsOrigin,
    [Parameter(Mandatory = $true)][string]$ExpectedReason,
    [Parameter(Mandatory = $true)][string[]]$RawValues,
    [string]$InitializerPath = $script:initializer,
    [switch]$AllowTemporaryFiles
  )

  $captured = New-Object 'System.Collections.Generic.List[string]'
  $failed = $false
  $failureMessage = ''

  try {
    & $InitializerPath `
      -OutputPath $OutputPath `
      -MySQLDSN $MySQLDSN `
      -RedisAddress $RedisAddress `
      -CorsOrigin $CorsOrigin 2>&1 |
      ForEach-Object { [void]$captured.Add([string]$_) }
  }
  catch {
    $failed = $true
    $failureMessage = $_.Exception.Message
  }

  Assert-True $failed "initializer must reject case: $CaseName"
  Assert-Equal $ExpectedReason $failureMessage "initializer must use a static reason for case: $CaseName"
  Assert-Equal 0 $captured.Count "initializer must not emit output before rejecting case: $CaseName"

  $allFailureText = (($captured.ToArray() + @($failureMessage)) -join "`n")
  foreach ($rawValue in $RawValues) {
    if (-not [string]::IsNullOrEmpty($rawValue)) {
      Assert-False $allFailureText.Contains($rawValue) "initializer leaked a raw parameter for case: $CaseName"
    }
  }
  if (-not $AllowTemporaryFiles) {
    Assert-NoInitializerTemporaryFiles
  }
}

function Invoke-RaceFixture {
  param(
    [Parameter(Mandatory = $true)][string]$InitializerPath,
    [Parameter(Mandatory = $true)][string]$OutputPath,
    [Parameter(Mandatory = $true)][string]$MySQLDSN,
    [Parameter(Mandatory = $true)][string]$RedisAddress,
    [Parameter(Mandatory = $true)][string]$CorsOrigin
  )

  $captured = New-Object 'System.Collections.Generic.List[string]'
  $failed = $false
  $failureMessage = ''
  try {
    & $InitializerPath `
      -OutputPath $OutputPath `
      -MySQLDSN $MySQLDSN `
      -RedisAddress $RedisAddress `
      -CorsOrigin $CorsOrigin 2>&1 |
      ForEach-Object { [void]$captured.Add([string]$_) }
  }
  catch {
    $failed = $true
    $failureMessage = $_.Exception.Message
  }

  return [pscustomobject]@{
    Failed = $failed
    FailureMessage = $failureMessage
    Captured = [string[]]$captured.ToArray()
  }
}

$validMySQLDSN = 'test_user:fake-password@tcp(127.0.0.1:3307)/admin?charset=utf8mb4&parseTime=True&loc=Local'
$updatedMySQLDSN = 'test_user:updated-fake-password@tcp([::1]:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local'
$canonicalDsnError = 'MySQLDSN must use the Compose-safe canonical local format.'
$validRedisAddress = '127.0.0.1:6380'
$validCorsOrigin = 'http://localhost:5173'
$validCorsOrigins = 'http://localhost:5173,http://127.0.0.1:5173'

$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ('admin-init-local-env-tests-' + [guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($tempRoot)

try {
  $happyPath = Join-Path $tempRoot 'happy.env'
  $happyLog = @(Invoke-SuccessInitializer `
      -OutputPath $happyPath `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin)

  Assert-Equal 1 $happyLog.Count 'successful initializer must emit exactly one message'
  Assert-Equal "created runtime env at $happyPath" $happyLog[0] 'custom output success message must not claim the path is ignored'
  Assert-True (Test-Path -LiteralPath $happyPath -PathType Leaf) 'happy path must create the requested env file'

  $happyContent = [IO.File]::ReadAllText($happyPath)
  foreach ($marker in @('CHANGE_ME', 'DB_PRIVATE_IP', 'REDIS_PRIVATE_IP', 'FRONTEND_DOMAIN_REQUIRED')) {
    Assert-False $happyContent.Contains($marker) 'generated env must not retain template placeholders'
  }
  Assert-True $happyContent.Contains("APP_ENV=local") 'generated env must use APP_ENV=local'

  $initializerSource = [IO.File]::ReadAllText($script:initializer)
  $testSource = [IO.File]::ReadAllText($PSCommandPath)
  $legacyFileAclPrefix = '[IO.File]::'
  $fileSystemAclExtensionsName = 'System.IO.' + 'FileSystemAclExtensions'
  Assert-False $testSource.Contains($legacyFileAclPrefix + 'GetAccessControl(') 'ACL fixtures must not use the PowerShell 5.1-only File.GetAccessControl API'
  Assert-False $testSource.Contains($legacyFileAclPrefix + 'SetAccessControl(') 'ACL fixtures must not use the PowerShell 5.1-only File.SetAccessControl API'
  Assert-True $testSource.Contains($fileSystemAclExtensionsName) 'ACL fixtures must provide a FileSystemAclExtensions fallback for PowerShell 7'
  $rightsConstructorBranch = 'if ($null -ne $rightsConstructor) {'
  Assert-Equal 1 ([regex]::Matches($initializerSource, [regex]::Escape($rightsConstructorBranch))).Count 'initializer must expose exactly one FileSystemRights constructor branch'
  $rightsConstructorPattern = '(?s)\$rightsConstructor\s*=\s*\[IO\.FileStream\]\.GetConstructor\(\s*\[type\[\]\]@\(\s*\[string\],\s*\[IO\.FileMode\],\s*\[Security\.AccessControl\.FileSystemRights\],\s*\[IO\.FileShare\],\s*\[int\],\s*\[IO\.FileOptions\]\s*\)\s*\)'
  Assert-Equal 1 ([regex]::Matches($initializerSource, $rightsConstructorPattern)).Count 'initializer must reflect the exact FileSystemRights FileStream constructor signature'
  Assert-True $initializerSource.Contains('CreateFileW') 'initializer must provide a native CreateFileW fallback'

  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $createFileFixtureRoot = Join-Path $tempRoot 'createfile-fallback-fixture'
    $createFileFixtureDocker = Join-Path $createFileFixtureRoot 'deploy\docker-first'
    [void][IO.Directory]::CreateDirectory($createFileFixtureDocker)
    $createFileFixtureInitializer = Join-Path $createFileFixtureDocker 'init-local-env.ps1'
    $createFileFixtureTemplate = Join-Path $createFileFixtureDocker 'admin-go.env.example'
    [IO.File]::Copy($script:initializer, $createFileFixtureInitializer)
    [IO.File]::Copy($templatePath, $createFileFixtureTemplate)

    $createFileFixtureSource = [IO.File]::ReadAllText($createFileFixtureInitializer)
    Assert-Equal 1 ([regex]::Matches($createFileFixtureSource, [regex]::Escape($rightsConstructorBranch))).Count 'CreateFileW fixture must replace exactly one constructor branch'
    $createFileFixtureInjection = '$rightsConstructor = $null' + "`r`n  " + $rightsConstructorBranch
    $createFileFixtureSource = $createFileFixtureSource.Replace($rightsConstructorBranch, $createFileFixtureInjection)
    Write-Utf8NoBom -Path $createFileFixtureInitializer -Content $createFileFixtureSource

    $createFileFallbackOutput = Join-Path $tempRoot 'createfile-fallback.env'
    $createFileFallbackLog = @(Invoke-SuccessInitializer `
        -InitializerPath $createFileFixtureInitializer `
        -OutputPath $createFileFallbackOutput `
        -MySQLDSN $validMySQLDSN `
        -RedisAddress $validRedisAddress `
        -CorsOrigin $validCorsOrigin)
    Assert-Equal 1 $createFileFallbackLog.Count 'CreateFileW fallback must emit exactly one success message'
    Assert-Equal "created runtime env at $createFileFallbackOutput" $createFileFallbackLog[0] 'CreateFileW fallback must report the external custom output'
    $createFileFallbackContent = [IO.File]::ReadAllText($createFileFallbackOutput)
    Assert-True $createFileFallbackContent.Contains("MYSQL_DSN=$validMySQLDSN") 'CreateFileW fallback must render the requested MySQL DSN'
    Assert-True $createFileFallbackContent.Contains("REDIS_ADDR=$validRedisAddress") 'CreateFileW fallback must render the requested Redis address'
    Assert-True $createFileFallbackContent.Contains("CORS_ALLOW_ORIGINS=$validCorsOrigin") 'CreateFileW fallback must render the requested CORS origin'
  }
  Assert-False $happyContent.Contains("APP_ENV=production") 'generated env must not retain APP_ENV=production'
  Assert-True $happyContent.Contains("MYSQL_DSN=$validMySQLDSN") 'generated env must contain the requested MySQL DSN'
  Assert-True $happyContent.Contains("REDIS_ADDR=$validRedisAddress") 'generated env must contain the requested Redis address'
  Assert-True $happyContent.Contains("CORS_ALLOW_ORIGINS=$validCorsOrigin") 'generated env must contain the requested CORS origin'

  $multipleOriginsPath = Join-Path $tempRoot 'multiple-origins.env'
  $null = Invoke-SuccessInitializer `
    -OutputPath $multipleOriginsPath `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigins
  $multipleOriginsContent = [IO.File]::ReadAllText($multipleOriginsPath)
  Assert-True $multipleOriginsContent.Contains("CORS_ALLOW_ORIGINS=$validCorsOrigins") 'initializer must render multiple comma-separated CORS origins'

  $firstSecret = Get-AppSecret $happyContent
  Assert-True ($firstSecret.Length -ge 64) 'generated APP_SECRET must contain at least 64 characters'
  $happyLogText = $happyLog -join "`n"
  Assert-False $happyLogText.Contains($validMySQLDSN) 'success output must not contain the MySQL DSN'
  Assert-False $happyLogText.Contains($validRedisAddress) 'success output must not contain the Redis address'
  Assert-False $happyLogText.Contains($firstSecret) 'success output must not contain APP_SECRET'

  $happyBytes = [IO.File]::ReadAllBytes($happyPath)
  Assert-True ($happyBytes.Length -ge 3) 'generated env must not be empty'
  $hasUtf8Bom = $happyBytes[0] -eq 0xEF -and $happyBytes[1] -eq 0xBB -and $happyBytes[2] -eq 0xBF
  Assert-False $hasUtf8Bom 'generated env must be UTF-8 without BOM'

  $repeatLog = @(Invoke-SuccessInitializer `
      -OutputPath $happyPath `
      -MySQLDSN $updatedMySQLDSN `
      -RedisAddress '[::1]:6381' `
      -CorsOrigin 'https://admin.localhost')
  $repeatContent = [IO.File]::ReadAllText($happyPath)
  $secondSecret = Get-AppSecret $repeatContent

  Assert-Equal $firstSecret $secondSecret 'rerunning initializer must preserve a valid generated APP_SECRET'
  Assert-True $repeatContent.Contains("MYSQL_DSN=$updatedMySQLDSN") 'rerunning initializer must update the MySQL DSN'
  Assert-True $repeatContent.Contains('REDIS_ADDR=[::1]:6381') 'initializer must accept a bracketed IPv6 Redis address'
  Assert-True $repeatContent.Contains('CORS_ALLOW_ORIGINS=https://admin.localhost') 'rerunning initializer must update the CORS origin'
  Assert-Equal "created runtime env at $happyPath" ($repeatLog -join "`n") 'custom output rerun message must remain secret-safe without claiming ignore coverage'
  Assert-False (($repeatLog -join "`n").Contains($secondSecret)) 'rerun output must not contain the preserved APP_SECRET'

  $customPath = Join-Path $tempRoot 'custom.env'
  $customSecret = 'CUSTOM-' + ('x' * 60) + '=='
  Write-Utf8NoBom -Path $customPath -Content "APP_SECRET=$customSecret`r`n"
  $customLog = @(Invoke-SuccessInitializer `
      -OutputPath $customPath `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress 'redis.local:6379' `
      -CorsOrigin 'https://[::1]:443')
  $customContent = [IO.File]::ReadAllText($customPath)
  $preservedCustomSecret = Get-AppSecret $customContent
  Assert-Equal $customSecret $preservedCustomSecret 'initializer must preserve a valid custom APP_SECRET byte-for-byte'
  Assert-True $customContent.Contains('CORS_ALLOW_ORIGINS=https://[::1]:443') 'initializer must accept a bracketed IPv6 CORS origin with a valid port'
  Assert-False (($customLog -join "`n").Contains($customSecret)) 'success output must not contain a custom APP_SECRET'

  $unsafeExistingSecrets = @(
    (' ' * 64),
    ((' ' * 12) + 'change_me_to_at_least_64_random_chars' + (' ' * 15)),
    ((' ' * 20) + 'change_me_to_long_random' + (' ' * 20)),
    (('s' * 62) + '  '),
    (('d' * 60) + '$ADMIN_MISSING_FOR_TEST'),
    (('q' * 63) + '"'),
    (('a' * 63) + "'"),
    (('b' * 63) + '`'),
    (('c' * 63) + '\'),
    (('h' * 63) + '#'),
    (('t' * 63) + "`t"),
    (('x' * 63) + [char]1)
  )
  $unsafeSecretIndex = 0
  foreach ($unsafeExistingSecret in $unsafeExistingSecrets) {
    $unsafeSecretIndex++
    Assert-True ($unsafeExistingSecret.Length -ge 64) 'unsafe existing secret fixture must exercise raw length of at least 64 characters'
    $unsafeSecretPath = Join-Path $tempRoot ("unsafe-secret-{0:D2}.env" -f $unsafeSecretIndex)
    Write-Utf8NoBom -Path $unsafeSecretPath -Content "APP_SECRET=$unsafeExistingSecret`n"
    [void](Invoke-SuccessInitializer `
        -OutputPath $unsafeSecretPath `
        -MySQLDSN $validMySQLDSN `
        -RedisAddress $validRedisAddress `
        -CorsOrigin $validCorsOrigin)
    $replacementForUnsafeSecret = Get-AppSecret ([IO.File]::ReadAllText($unsafeSecretPath))
    Assert-Equal 64 $replacementForUnsafeSecret.Length 'runtime-unsafe APP_SECRET must be replaced with 48 random bytes encoded as Base64'
    Assert-False ($replacementForUnsafeSecret -ceq $unsafeExistingSecret) "runtime-unsafe APP_SECRET fixture $unsafeSecretIndex must not be preserved"
  }

  $markerValuePath = Join-Path $tempRoot 'marker-values.env'
  $markerSecret = 'legitimate_CHANGE_ME_DB_PRIVATE_IP_secret_' + ('x' * 40)
  $markerMySQLDSN = 'test_user:validDB_PRIVATE_IPCHANGE_MEpassword@tcp(127.0.0.1:3307)/admin?charset=utf8mb4&parseTime=True&loc=Local'
  Write-Utf8NoBom -Path $markerValuePath -Content "APP_SECRET=$markerSecret`n"
  $markerLog = @(Invoke-SuccessInitializer `
      -OutputPath $markerValuePath `
      -MySQLDSN $markerMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin)
  $markerContent = [IO.File]::ReadAllText($markerValuePath)
  Assert-Equal $markerSecret (Get-AppSecret $markerContent) 'valid APP_SECRET containing ordinary marker text must be preserved'
  Assert-True $markerContent.Contains("MYSQL_DSN=$markerMySQLDSN") 'valid MySQL DSN containing ordinary marker text must be rendered exactly'
  Assert-False (($markerLog -join "`n").Contains($markerSecret)) 'success output must not contain marker-bearing APP_SECRET'
  Assert-False (($markerLog -join "`n").Contains($markerMySQLDSN)) 'success output must not contain marker-bearing MySQL DSN'

  $templateFixtureRoot = Join-Path $tempRoot 'template-fixture'
  $templateFixtureDocker = Join-Path $templateFixtureRoot 'deploy\docker-first'
  [void][IO.Directory]::CreateDirectory($templateFixtureDocker)
  $fixtureInitializer = Join-Path $templateFixtureDocker 'init-local-env.ps1'
  $fixtureTemplate = Join-Path $templateFixtureDocker 'admin-go.env.example'
  [IO.File]::Copy($script:initializer, $fixtureInitializer)
  [IO.File]::Copy($templatePath, $fixtureTemplate)
  $fixtureTemplateContent = [IO.File]::ReadAllText($fixtureTemplate)
  $fixtureTemplateContent = $fixtureTemplateContent.Replace(
    'MYSQL_DSN=admin_user:CHANGE_ME@tcp(DB_PRIVATE_IP:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local',
    'MYSQL_DSN=admin_user:CHANGE_ME_EXTRA@tcp(DB_PRIVATE_IP:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local'
  )
  Write-Utf8NoBom -Path $fixtureTemplate -Content $fixtureTemplateContent
  $fixtureOutput = Join-Path $tempRoot 'template-fixture-output.env'
  Invoke-RejectedInitializer `
    -CaseName 'template exact placeholder drift' `
    -OutputPath $fixtureOutput `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigin `
    -ExpectedReason 'runtime env template does not match required placeholders.' `
    -RawValues @($fixtureOutput, $validMySQLDSN, $validRedisAddress, $validCorsOrigin) `
    -InitializerPath $fixtureInitializer
  Assert-False (Test-Path -LiteralPath $fixtureOutput) 'template placeholder drift must not create a runtime env'

  $boundaryFixtureRoot = Join-Path $tempRoot 'boundary-fixture'
  $boundaryFixtureDocker = Join-Path $boundaryFixtureRoot 'deploy\docker-first'
  [void][IO.Directory]::CreateDirectory($boundaryFixtureDocker)
  $boundaryInitializer = Join-Path $boundaryFixtureDocker 'init-local-env.ps1'
  $boundaryTemplate = Join-Path $boundaryFixtureDocker 'admin-go.env.example'
  $boundaryReadme = Join-Path $boundaryFixtureDocker 'README.md'
  [IO.File]::Copy($script:initializer, $boundaryInitializer)
  [IO.File]::Copy($templatePath, $boundaryTemplate)
  Write-Utf8NoBom -Path $boundaryReadme -Content "protected fixture README`n"

  foreach ($protectedRepoPath in @($boundaryTemplate, $boundaryReadme, $boundaryInitializer)) {
    $protectedBytes = [Convert]::ToBase64String([IO.File]::ReadAllBytes($protectedRepoPath))
    Invoke-RejectedInitializer `
      -CaseName 'repository output path protection' `
      -OutputPath $protectedRepoPath `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin `
      -ExpectedReason 'OutputPath must be the default ignored env or outside the repository.' `
      -RawValues @($protectedRepoPath, $validMySQLDSN, $validRedisAddress, $validCorsOrigin) `
      -InitializerPath $boundaryInitializer
    Assert-Equal $protectedBytes ([Convert]::ToBase64String([IO.File]::ReadAllBytes($protectedRepoPath))) 'rejected repository output path must preserve the protected file byte-for-byte'
  }

  $newRepoPath = Join-Path $boundaryFixtureRoot 'must-not-create.env'
  Invoke-RejectedInitializer `
    -CaseName 'new repository output path protection' `
    -OutputPath $newRepoPath `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigin `
    -ExpectedReason 'OutputPath must be the default ignored env or outside the repository.' `
    -RawValues @($newRepoPath, $validMySQLDSN, $validRedisAddress, $validCorsOrigin) `
    -InitializerPath $boundaryInitializer
  Assert-False (Test-Path -LiteralPath $newRepoPath) 'rejected new repository output path must not be created'

  $fixtureDefaultOutput = Join-Path $boundaryFixtureDocker 'admin-go.env'
  $fixtureDefaultLog = @(Invoke-SuccessInitializer `
      -OutputPath $fixtureDefaultOutput `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin `
      -InitializerPath $boundaryInitializer)
  Assert-Equal "created ignored runtime env at $fixtureDefaultOutput" ($fixtureDefaultLog -join "`n") 'fixture default output must be the sole repository path reported as ignored'
  [IO.File]::Delete($fixtureDefaultOutput)
  Assert-NoInitializerTemporaryFiles

  $realAliasParent = Join-Path $tempRoot 'real-alias-parent'
  $aliasParent = Join-Path $tempRoot 'alias-parent'
  [void][IO.Directory]::CreateDirectory($realAliasParent)
  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    [void](New-Item -ItemType Junction -Path $aliasParent -Target $realAliasParent)
  }
  else {
    [void](New-Item -ItemType SymbolicLink -Path $aliasParent -Target $realAliasParent)
  }
  try {
    $aliasedOutput = Join-Path $aliasParent 'aliased.env'
    Invoke-RejectedInitializer `
      -CaseName 'reparse parent output path' `
      -OutputPath $aliasedOutput `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin `
      -ExpectedReason 'OutputPath must not use a symbolic link, junction, or reparse point.' `
      -RawValues @($aliasedOutput, $validMySQLDSN, $validRedisAddress, $validCorsOrigin)
    Assert-False (Test-Path -LiteralPath (Join-Path $realAliasParent 'aliased.env')) 'reparse parent must not receive a runtime env through its alias'
  }
  finally {
    if (Test-Path -LiteralPath $aliasParent) {
      Remove-Item -LiteralPath $aliasParent -Force
    }
  }

  $realAliasTarget = Join-Path $tempRoot 'real-alias-target'
  $aliasTarget = Join-Path $tempRoot 'alias-target.env'
  [void][IO.Directory]::CreateDirectory($realAliasTarget)
  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    [void](New-Item -ItemType Junction -Path $aliasTarget -Target $realAliasTarget)
  }
  else {
    [void](New-Item -ItemType SymbolicLink -Path $aliasTarget -Target $realAliasTarget)
  }
  try {
    Invoke-RejectedInitializer `
      -CaseName 'reparse target output path' `
      -OutputPath $aliasTarget `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin `
      -ExpectedReason 'OutputPath must not use a symbolic link, junction, or reparse point.' `
      -RawValues @($aliasTarget, $validMySQLDSN, $validRedisAddress, $validCorsOrigin)
  }
  finally {
    if (Test-Path -LiteralPath $aliasTarget) {
      Remove-Item -LiteralPath $aliasTarget -Force
    }
  }

  $directoryOutput = Join-Path $tempRoot 'existing-directory.env'
  [void][IO.Directory]::CreateDirectory($directoryOutput)
  $directorySentinel = Join-Path $directoryOutput 'sentinel.txt'
  Write-Utf8NoBom -Path $directorySentinel -Content "directory must remain unchanged`n"
  $directorySentinelBytes = [Convert]::ToBase64String([IO.File]::ReadAllBytes($directorySentinel))
  Invoke-RejectedInitializer `
    -CaseName 'existing directory move failure' `
    -OutputPath $directoryOutput `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigin `
    -ExpectedReason 'runtime env could not be written.' `
    -RawValues @($directoryOutput, $validMySQLDSN, $validRedisAddress, $validCorsOrigin)
  Assert-True (Test-Path -LiteralPath $directoryOutput -PathType Container) 'move failure must preserve the existing directory target'
  Assert-Equal $directorySentinelBytes ([Convert]::ToBase64String([IO.File]::ReadAllBytes($directorySentinel))) 'move failure must preserve existing target contents byte-for-byte'

  $cleanupFixtureRoot = Join-Path $tempRoot 'cleanup-fixture'
  $cleanupFixtureDocker = Join-Path $cleanupFixtureRoot 'deploy\docker-first'
  [void][IO.Directory]::CreateDirectory($cleanupFixtureDocker)
  $cleanupFixtureInitializer = Join-Path $cleanupFixtureDocker 'init-local-env.ps1'
  $cleanupFixtureTemplate = Join-Path $cleanupFixtureDocker 'admin-go.env.example'
  [IO.File]::Copy($script:initializer, $cleanupFixtureInitializer)
  [IO.File]::Copy($templatePath, $cleanupFixtureTemplate)
  $cleanupFixtureSource = [IO.File]::ReadAllText($cleanupFixtureInitializer)
  $cleanupDeleteStatement = '[IO.File]::Delete($temporaryPath)'
  Assert-Equal 1 ([regex]::Matches($cleanupFixtureSource, [regex]::Escape($cleanupDeleteStatement))).Count 'cleanup fixture must replace exactly one production temp-delete statement'
  $cleanupFixtureSource = $cleanupFixtureSource.Replace($cleanupDeleteStatement, "throw 'fixture cleanup failure'")
  Write-Utf8NoBom -Path $cleanupFixtureInitializer -Content $cleanupFixtureSource

  $cleanupFailureTarget = Join-Path $tempRoot 'cleanup-failure-target.env'
  [void][IO.Directory]::CreateDirectory($cleanupFailureTarget)
  $cleanupFailureSentinel = Join-Path $cleanupFailureTarget 'sentinel.txt'
  Write-Utf8NoBom -Path $cleanupFailureSentinel -Content "cleanup failure target must remain unchanged`n"
  $cleanupFailureSentinelBytes = [Convert]::ToBase64String([IO.File]::ReadAllBytes($cleanupFailureSentinel))
  Invoke-RejectedInitializer `
    -CaseName 'cleanup failure after move failure' `
    -OutputPath $cleanupFailureTarget `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigin `
    -ExpectedReason 'runtime env cleanup failed after runtime env write failure.' `
    -RawValues @($cleanupFailureTarget, $validMySQLDSN, $validRedisAddress, $validCorsOrigin) `
    -InitializerPath $cleanupFixtureInitializer `
    -AllowTemporaryFiles
  Assert-Equal $cleanupFailureSentinelBytes ([Convert]::ToBase64String([IO.File]::ReadAllBytes($cleanupFailureSentinel))) 'cleanup failure must preserve existing target contents byte-for-byte'
  $cleanupFailureTemporaryFiles = @(
    Get-ChildItem -LiteralPath $tempRoot -Recurse -Force -File |
      Where-Object { $_.Name -match '^\.cleanup-failure-target\.env\.[0-9a-f]{32}\.tmp$' }
  )
  Assert-Equal 1 $cleanupFailureTemporaryFiles.Count 'cleanup failure fixture must leave exactly the injected temp file for test cleanup'
  Assert-OwnerOnlyPermissions $cleanupFailureTemporaryFiles[0].FullName
  [IO.File]::Delete($cleanupFailureTemporaryFiles[0].FullName)
  Assert-NoInitializerTemporaryFiles

  $hardlinkFixtureRoot = Join-Path $tempRoot 'hardlink-race-fixture'
  $hardlinkFixtureDocker = Join-Path $hardlinkFixtureRoot 'deploy\docker-first'
  [void][IO.Directory]::CreateDirectory($hardlinkFixtureDocker)
  $hardlinkFixtureInitializer = Join-Path $hardlinkFixtureDocker 'init-local-env.ps1'
  $hardlinkFixtureTemplate = Join-Path $hardlinkFixtureDocker 'admin-go.env.example'
  [IO.File]::Copy($script:initializer, $hardlinkFixtureInitializer)
  [IO.File]::Copy($templatePath, $hardlinkFixtureTemplate)
  $hardlinkVictim = Join-Path $tempRoot 'hardlink-victim.txt'
  Write-Utf8NoBom -Path $hardlinkVictim -Content "hardlink victim must remain unchanged`n"
  $hardlinkVictimBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($hardlinkVictim))
  $hardlinkFixtureSource = [IO.File]::ReadAllText($hardlinkFixtureInitializer)
  $hardlinkAnchor = @(
    'Set-OwnerOnlyPermissions $temporaryPath',
    'Set-OwnerOnlyPermissions -Stream $temporaryStream'
  ) | Where-Object { $hardlinkFixtureSource.Contains($_) } | Select-Object -First 1
  Assert-True (-not [string]::IsNullOrEmpty($hardlinkAnchor)) 'hardlink race fixture must find the owner-only permission anchor'
  Assert-Equal 1 ([regex]::Matches($hardlinkFixtureSource, [regex]::Escape($hardlinkAnchor))).Count 'hardlink race fixture must replace exactly one permission anchor'
  $hardlinkVictimLiteral = $hardlinkVictim.Replace("'", "''")
  $hardlinkInjection = $hardlinkAnchor + "`r`n" +
    '[IO.File]::Delete($temporaryPath)' + "`r`n" +
    "[void](New-Item -ItemType HardLink -Path `$temporaryPath -Target '$hardlinkVictimLiteral')"
  $hardlinkFixtureSource = $hardlinkFixtureSource.Replace($hardlinkAnchor, $hardlinkInjection)
  Write-Utf8NoBom -Path $hardlinkFixtureInitializer -Content $hardlinkFixtureSource
  $hardlinkRaceOutput = Join-Path $tempRoot 'hardlink-race-output.env'
  $hardlinkRaceResult = Invoke-RaceFixture `
    -InitializerPath $hardlinkFixtureInitializer `
    -OutputPath $hardlinkRaceOutput `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigin
  $hardlinkVictimAfter = [Convert]::ToBase64String([IO.File]::ReadAllBytes($hardlinkVictim))
  $hardlinkOutputCreated = Test-Path -LiteralPath $hardlinkRaceOutput
  if ($hardlinkOutputCreated) {
    [IO.File]::Delete($hardlinkRaceOutput)
  }

  $junctionFixtureRoot = Join-Path $tempRoot 'junction-race-fixture'
  $junctionFixtureDocker = Join-Path $junctionFixtureRoot 'deploy\docker-first'
  [void][IO.Directory]::CreateDirectory($junctionFixtureDocker)
  $junctionFixtureInitializer = Join-Path $junctionFixtureDocker 'init-local-env.ps1'
  $junctionFixtureTemplate = Join-Path $junctionFixtureDocker 'admin-go.env.example'
  [IO.File]::Copy($script:initializer, $junctionFixtureInitializer)
  [IO.File]::Copy($templatePath, $junctionFixtureTemplate)
  $junctionRaceParent = Join-Path $tempRoot 'junction-race-parent'
  $junctionRaceOriginalParent = Join-Path $tempRoot 'junction-race-original-parent'
  $junctionRaceTarget = Join-Path $tempRoot 'junction-race-target'
  [void][IO.Directory]::CreateDirectory($junctionRaceParent)
  [void][IO.Directory]::CreateDirectory($junctionRaceTarget)
  $junctionFixtureSource = [IO.File]::ReadAllText($junctionFixtureInitializer)
  $junctionAnchor = '$templatePath = Join-Path $PSScriptRoot ''admin-go.env.example'''
  Assert-Equal 1 ([regex]::Matches($junctionFixtureSource, [regex]::Escape($junctionAnchor))).Count 'junction race fixture must replace exactly one post-check anchor'
  $junctionRaceParentLiteral = $junctionRaceParent.Replace("'", "''")
  $junctionRaceOriginalLiteral = $junctionRaceOriginalParent.Replace("'", "''")
  $junctionRaceTargetLiteral = $junctionRaceTarget.Replace("'", "''")
  $junctionItemType = if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) { 'Junction' } else { 'SymbolicLink' }
  $junctionInjection = "[IO.Directory]::Move('$junctionRaceParentLiteral', '$junctionRaceOriginalLiteral')`r`n" +
    "[void](New-Item -ItemType $junctionItemType -Path '$junctionRaceParentLiteral' -Target '$junctionRaceTargetLiteral')`r`n" +
    $junctionAnchor
  $junctionFixtureSource = $junctionFixtureSource.Replace($junctionAnchor, $junctionInjection)
  Write-Utf8NoBom -Path $junctionFixtureInitializer -Content $junctionFixtureSource
  $junctionRaceOutput = Join-Path $junctionRaceParent 'junction-race-output.env'
  try {
    $junctionRaceResult = Invoke-RaceFixture `
      -InitializerPath $junctionFixtureInitializer `
      -OutputPath $junctionRaceOutput `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin
    $junctionPayload = Join-Path $junctionRaceTarget 'junction-race-output.env'
    $junctionPayloadCreated = Test-Path -LiteralPath $junctionPayload
  }
  finally {
    if (Test-Path -LiteralPath $junctionRaceParent) {
      if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        [IO.Directory]::Delete($junctionRaceParent)
      }
      else {
        Remove-Item -LiteralPath $junctionRaceParent -Force
      }
    }
  }
  if ($junctionPayloadCreated) {
    [IO.File]::Delete($junctionPayload)
  }

  $hardlinkProtected = $hardlinkRaceResult.Failed -and -not $hardlinkOutputCreated -and $hardlinkVictimAfter -ceq $hardlinkVictimBefore
  $junctionProtected = $junctionRaceResult.Failed -and -not $junctionPayloadCreated
  Assert-True ($hardlinkProtected -and $junctionProtected) "TOCTOU fixtures must reject both races (hardlinkProtected=$hardlinkProtected, junctionProtected=$junctionProtected)"
  foreach ($raceResult in @($hardlinkRaceResult, $junctionRaceResult)) {
    Assert-Equal 'runtime env could not be written.' $raceResult.FailureMessage 'TOCTOU fixture must return a static write failure'
    Assert-Equal 0 $raceResult.Captured.Count 'TOCTOU fixture must not emit output before rejecting the race'
    $raceFailureText = (($raceResult.Captured + @($raceResult.FailureMessage)) -join "`n")
    foreach ($rawValue in @($hardlinkRaceOutput, $junctionRaceOutput, $validMySQLDSN, $validRedisAddress, $validCorsOrigin)) {
      Assert-False $raceFailureText.Contains($rawValue) 'TOCTOU fixture failure must not leak raw parameters'
    }
  }
  Assert-NoInitializerTemporaryFiles

  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $currentProcessSid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $systemSid = New-Object Security.Principal.SecurityIdentifier('S-1-5-18')
    Assert-False ($currentProcessSid.Value -ceq $systemSid.Value) 'foreign-rule fixture requires a non-SYSTEM current process'

    $foreignRuleFixtureRoot = Join-Path $tempRoot 'foreign-rule-fixture'
    $foreignRuleFixtureDocker = Join-Path $foreignRuleFixtureRoot 'deploy\docker-first'
    [void][IO.Directory]::CreateDirectory($foreignRuleFixtureDocker)
    $foreignRuleFixtureInitializer = Join-Path $foreignRuleFixtureDocker 'init-local-env.ps1'
    $foreignRuleFixtureTemplate = Join-Path $foreignRuleFixtureDocker 'admin-go.env.example'
    [IO.File]::Copy($script:initializer, $foreignRuleFixtureInitializer)
    [IO.File]::Copy($templatePath, $foreignRuleFixtureTemplate)
    $foreignRuleFixtureSource = [IO.File]::ReadAllText($foreignRuleFixtureInitializer)
    $foreignRuleAnchor = '$temporaryPath = $null'
    Assert-Equal 1 ([regex]::Matches($foreignRuleFixtureSource, [regex]::Escape($foreignRuleAnchor))).Count 'foreign-rule fixture must replace exactly one post-move anchor'
    $foreignRuleInjection = $foreignRuleAnchor + "`r`n" +
      '$fixtureAcl = Get-AccessOnlyFileAcl -Path $fullOutputPath' + "`r`n" +
      '$fixtureSid = New-Object Security.Principal.SecurityIdentifier(''S-1-5-18'')' + "`r`n" +
      '$fixtureRule = New-Object Security.AccessControl.FileSystemAccessRule($fixtureSid, [Security.AccessControl.FileSystemRights]::WriteData, [Security.AccessControl.AccessControlType]::Allow)' + "`r`n" +
      '[void]$fixtureAcl.AddAccessRule($fixtureRule)' + "`r`n" +
      'Set-AccessOnlyFileAcl -Path $fullOutputPath -Acl $fixtureAcl'
    $foreignRuleFixtureSource = $foreignRuleFixtureSource.Replace($foreignRuleAnchor, $foreignRuleInjection)
    Write-Utf8NoBom -Path $foreignRuleFixtureInitializer -Content $foreignRuleFixtureSource
    $foreignRuleOutput = Join-Path $tempRoot 'foreign-rule-output.env'
    $foreignRuleResult = Invoke-RaceFixture `
      -InitializerPath $foreignRuleFixtureInitializer `
      -OutputPath $foreignRuleOutput `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin
    $foreignRuleOutputCreated = Test-Path -LiteralPath $foreignRuleOutput -PathType Leaf
    Assert-True $foreignRuleOutputCreated 'foreign-rule fixture must leave the post-move output available for ACL verification'
    try {
      $foreignRuleAcl = Get-AccessOnlyFileAcl -Path $foreignRuleOutput
      $foreignWriteAllowRules = @(
        $foreignRuleAcl.GetAccessRules($true, $false, [Security.Principal.SecurityIdentifier]) |
          Where-Object {
            $_.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and
            $_.IdentityReference.Value -ceq $systemSid.Value -and
            -not $_.IsInherited -and
            ($_.FileSystemRights -band [Security.AccessControl.FileSystemRights]::WriteData) -ne 0
          }
      )
      Assert-True ($foreignWriteAllowRules.Count -gt 0) 'foreign-rule fixture must persist an explicit SYSTEM WriteData Allow rule'
    }
    finally {
      if ([IO.File]::Exists($foreignRuleOutput)) {
        [IO.File]::Delete($foreignRuleOutput)
      }
    }
    Assert-True $foreignRuleResult.Failed 'final permission verification must reject a foreign write-only Allow rule'
    Assert-Equal 'runtime env could not be written.' $foreignRuleResult.FailureMessage 'foreign-rule fixture must return a static write failure'
    Assert-Equal 0 $foreignRuleResult.Captured.Count 'foreign-rule fixture must not emit output before rejection'
    $foreignRuleFailureText = (($foreignRuleResult.Captured + @($foreignRuleResult.FailureMessage)) -join "`n")
    foreach ($rawValue in @($foreignRuleOutput, $validMySQLDSN, $validRedisAddress, $validCorsOrigin)) {
      Assert-False $foreignRuleFailureText.Contains($rawValue) 'foreign-rule fixture failure must not leak raw parameters'
    }
    Assert-NoInitializerTemporaryFiles
  }

  $placeholderPath = Join-Path $tempRoot 'placeholder.env'
  Write-Utf8NoBom -Path $placeholderPath -Content "APP_SECRET=CHANGE_ME_TO_64_PLUS_RANDOM_CHARS`n"
  [void](Invoke-SuccessInitializer `
      -OutputPath $placeholderPath `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin)
  $replacementForPlaceholder = Get-AppSecret ([IO.File]::ReadAllText($placeholderPath))
  Assert-Equal 64 $replacementForPlaceholder.Length 'placeholder APP_SECRET must be replaced with 48 random bytes encoded as Base64'
  Assert-False ($replacementForPlaceholder -ceq 'CHANGE_ME_TO_64_PLUS_RANDOM_CHARS') 'placeholder APP_SECRET must not be preserved'

  $shortPath = Join-Path $tempRoot 'short.env'
  Write-Utf8NoBom -Path $shortPath -Content "APP_SECRET=too-short`n"
  [void](Invoke-SuccessInitializer `
      -OutputPath $shortPath `
      -MySQLDSN $validMySQLDSN `
      -RedisAddress $validRedisAddress `
      -CorsOrigin $validCorsOrigin)
  $replacementForShortSecret = Get-AppSecret ([IO.File]::ReadAllText($shortPath))
  Assert-Equal 64 $replacementForShortSecret.Length 'short APP_SECRET must be replaced with 48 random bytes encoded as Base64'
  Assert-False ($replacementForShortSecret -ceq 'too-short') 'short APP_SECRET must not be preserved'

  $invalidCases = @(
    @{
      Name = 'MySQL noncanonical parseTime'
      MySQLDSN = 'test_user:fake-password@tcp(127.0.0.1:3307)/admin?charset=utf8mb4&parseTime=not-a-bool&loc=Local'
      RedisAddress = $validRedisAddress
      CorsOrigin = $validCorsOrigin
      Reason = $canonicalDsnError
    },
    @{
      Name = 'MySQL database'
      MySQLDSN = 'test_user:fake-password@tcp(127.0.0.1:3307)/other?charset=utf8mb4&parseTime=True&loc=Local'
      RedisAddress = $validRedisAddress
      CorsOrigin = $validCorsOrigin
      Reason = $canonicalDsnError
    },
    @{
      Name = 'Redis missing port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = 'redis.local'
      CorsOrigin = $validCorsOrigin
      Reason = 'RedisAddress must be a valid host:port with port 1..65535.'
    },
    @{
      Name = 'Redis zero port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = 'redis.local:0'
      CorsOrigin = $validCorsOrigin
      Reason = 'RedisAddress must be a valid host:port with port 1..65535.'
    },
    @{
      Name = 'Redis oversized port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = '127.0.0.1:65536'
      CorsOrigin = $validCorsOrigin
      Reason = 'RedisAddress must be a valid host:port with port 1..65535.'
    },
    @{
      Name = 'CORS userinfo'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://user@example.test'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS empty userinfo delimiter'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://@example.test'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS empty port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test:'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS zero port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test:0'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS oversized port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test:65536'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS bracketed IPv6 empty port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://[::1]:'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS bracketed IPv6 zero port'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://[::1]:0'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS query'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test?token=fake'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS fragment'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test#fragment'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS path'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test/admin'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'CORS normalized path'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = 'https://example.test/admin/..'
      Reason = 'CorsOrigin must be a plain HTTP(S) origin.'
    },
    @{
      Name = 'MySQL CRLF injection'
      MySQLDSN = $validMySQLDSN + "`rINJECTED=true"
      RedisAddress = $validRedisAddress
      CorsOrigin = $validCorsOrigin
      Reason = 'parameters must not contain CR or LF.'
    },
    @{
      Name = 'Redis CRLF injection'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress + "`nINJECTED=true"
      CorsOrigin = $validCorsOrigin
      Reason = 'parameters must not contain CR or LF.'
    },
    @{
      Name = 'CORS CRLF injection'
      MySQLDSN = $validMySQLDSN
      RedisAddress = $validRedisAddress
      CorsOrigin = $validCorsOrigin + "`r`nINJECTED=true"
      Reason = 'parameters must not contain CR or LF.'
    }
  )

  foreach ($unsafePasswordCase in @(
      @{ Name = 'dollar'; Password = 'bad$password' },
      @{ Name = 'whitespace'; Password = 'bad password' },
      @{ Name = 'hash'; Password = 'bad#password' },
      @{ Name = 'single quote'; Password = 'bad''password' },
      @{ Name = 'double quote'; Password = 'bad"password' },
      @{ Name = 'backtick'; Password = 'bad`password' },
      @{ Name = 'backslash'; Password = 'bad\password' }
    )) {
    $invalidCases += @{
      Name = 'MySQL Compose-unsafe password ' + $unsafePasswordCase.Name
      MySQLDSN = 'test_user:' + $unsafePasswordCase.Password + '@tcp(127.0.0.1:3307)/admin?charset=utf8mb4&parseTime=True&loc=Local'
      RedisAddress = $validRedisAddress
      CorsOrigin = $validCorsOrigin
      Reason = $canonicalDsnError
    }
  }

  foreach ($invalidMySQLAddress in @('db.local', 'db.local:0', 'db.local:65536', 'bad host:3306')) {
    $invalidCases += @{
      Name = 'MySQL invalid hostport'
      MySQLDSN = 'test_user:fake-password@tcp(' + $invalidMySQLAddress + ')/admin?charset=utf8mb4&parseTime=True&loc=Local'
      RedisAddress = $validRedisAddress
      CorsOrigin = $validCorsOrigin
      Reason = $canonicalDsnError
    }
  }

  $caseIndex = 0
  foreach ($invalidCase in $invalidCases) {
    $caseIndex++
    $newTarget = Join-Path $tempRoot ("reject-new-{0:D2}.env" -f $caseIndex)
    $rawValues = @($newTarget, $invalidCase.MySQLDSN, $invalidCase.RedisAddress, $invalidCase.CorsOrigin)
    Invoke-RejectedInitializer `
      -CaseName $invalidCase.Name `
      -OutputPath $newTarget `
      -MySQLDSN $invalidCase.MySQLDSN `
      -RedisAddress $invalidCase.RedisAddress `
      -CorsOrigin $invalidCase.CorsOrigin `
      -ExpectedReason $invalidCase.Reason `
      -RawValues $rawValues
    Assert-False (Test-Path -LiteralPath $newTarget) "rejected input must not create a target for case: $($invalidCase.Name)"

    $existingTarget = Join-Path $tempRoot ("reject-existing-{0:D2}.env" -f $caseIndex)
    $sentinelBytes = [byte[]](0x65, 0x78, 0x69, 0x73, 0x74, 0x69, 0x6E, 0x67)
    [IO.File]::WriteAllBytes($existingTarget, $sentinelBytes)
    $beforeFailure = [Convert]::ToBase64String([IO.File]::ReadAllBytes($existingTarget))
    $rawValues = @($existingTarget, $invalidCase.MySQLDSN, $invalidCase.RedisAddress, $invalidCase.CorsOrigin)
    Invoke-RejectedInitializer `
      -CaseName ($invalidCase.Name + ' existing target') `
      -OutputPath $existingTarget `
      -MySQLDSN $invalidCase.MySQLDSN `
      -RedisAddress $invalidCase.RedisAddress `
      -CorsOrigin $invalidCase.CorsOrigin `
      -ExpectedReason $invalidCase.Reason `
      -RawValues $rawValues
    $afterFailure = [Convert]::ToBase64String([IO.File]::ReadAllBytes($existingTarget))
    Assert-Equal $beforeFailure $afterFailure "rejected input must not modify a target for case: $($invalidCase.Name)"
  }

  $injectedOutputPath = (Join-Path $tempRoot 'output') + "`nINJECTED=true"
  Invoke-RejectedInitializer `
    -CaseName 'OutputPath CRLF injection' `
    -OutputPath $injectedOutputPath `
    -MySQLDSN $validMySQLDSN `
    -RedisAddress $validRedisAddress `
    -CorsOrigin $validCorsOrigin `
    -ExpectedReason 'parameters must not contain CR or LF.' `
    -RawValues @($injectedOutputPath, $validMySQLDSN, $validRedisAddress, $validCorsOrigin)
  Assert-False (Test-Path -LiteralPath $injectedOutputPath -ErrorAction SilentlyContinue) 'CRLF OutputPath must not create a target'

  $templateContent = [IO.File]::ReadAllText($templatePath)
  Assert-True $templateContent.Contains('APP_ENV=production') 'shared env template must remain production-oriented'
  Assert-False $templateContent.Contains('APP_ENV=local') 'shared env template must not be changed to local'
  Assert-True $templateContent.Contains('CORS_ALLOW_ORIGINS=https://FRONTEND_DOMAIN_REQUIRED') 'shared env template must use the required CORS placeholder'

  $readmeContent = [IO.File]::ReadAllText($readmePath)
  Assert-True $readmeContent.Contains('init-local-env.ps1') 'Docker-first README must document the local initializer'
  Assert-True $readmeContent.Contains('scripts/docker-platform.ps1 init') 'Docker-first README must document the platform initializer'
  Assert-True $readmeContent.Contains('mysql:3306') 'Docker-first README must use Docker DNS for MySQL'
  Assert-True $readmeContent.Contains('redis:6379') 'Docker-first README must use Docker DNS for Redis'
  Assert-True $readmeContent.Contains('CORS_ALLOW_ORIGINS=http://localhost:5173,http://127.0.0.1:5173') 'Docker-first README must document both supported loopback origins'
  Assert-False $readmeContent.Contains('$env:ADMIN_LOCAL_MYSQL_DSN') 'Docker-first README must not retain the retired host MySQL variable flow'
  Assert-False $readmeContent.Contains('$env:ADMIN_LOCAL_REDIS_ADDR') 'Docker-first README must not retain the retired host Redis variable flow'
  Assert-True $readmeContent.Contains('Compose-safe canonical MySQL DSN') 'Docker-first README must document the narrow local MySQL DSN contract'
  Assert-True $readmeContent.Contains('charset=utf8mb4&parseTime=True&loc=Local') 'Docker-first README must document the exact canonical MySQL DSN query'
  Assert-True $readmeContent.Contains('Reusable `APP_SECRET` values must contain at least 64 ASCII characters') 'Docker-first README must document the Compose-safe reusable APP_SECRET contract'
  Assert-True $readmeContent.Contains('Custom `-OutputPath` values are allowed only outside the repository') 'Docker-first README must distinguish external custom outputs from the default ignored path'
  Assert-False $readmeContent.Contains('cp -n admin-go.env.example admin-go.env') 'Docker-first README must not retain the unsafe copy-only local flow'
  Assert-False $readmeContent.Contains('test_user:fake-password') 'Docker-first README must not embed test credentials'

  Write-Output 'init-local-env assertions passed'
}
finally {
  if (Test-Path -LiteralPath $tempRoot) {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force
  }
}
