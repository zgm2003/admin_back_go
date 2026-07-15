[CmdletBinding()]
param(
  [string]$OutputPath = (Join-Path $PSScriptRoot 'admin-go.env'),
  [Parameter(Mandatory = $true)][string]$MySQLDSN,
  [Parameter(Mandatory = $true)][string]$RedisAddress,
  [Parameter(Mandatory = $true)][string]$CorsOrigin
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Test-ContainsLineBreak {
  param([AllowEmptyString()][string]$Value)

  return $Value.Contains("`r") -or $Value.Contains("`n")
}

function Test-AdminMySQLDSN {
  param([AllowEmptyString()][string]$Value)

  $matchResult = [regex]::Match(
    $Value,
    '^(?<username>[A-Za-z0-9._~-]+):(?<password>[A-Za-z0-9._~-]+)@tcp\((?<hostPort>[^)]+)\)/admin\?charset=utf8mb4&parseTime=True&loc=Local$'
  )
  return $matchResult.Success -and (Test-HostPort $matchResult.Groups['hostPort'].Value)
}

function Test-Hostname {
  param([AllowEmptyString()][string]$Value)

  if ([string]::IsNullOrWhiteSpace($Value) -or $Value.Length -gt 253) {
    return $false
  }

  $hostname = $Value
  if ($hostname.EndsWith('.')) {
    $hostname = $hostname.Substring(0, $hostname.Length - 1)
  }
  if ([string]::IsNullOrEmpty($hostname)) {
    return $false
  }

  foreach ($label in $hostname.Split('.')) {
    if ($label.Length -lt 1 -or $label.Length -gt 63) {
      return $false
    }
    if ($label -notmatch '^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$') {
      return $false
    }
  }

  return $true
}

function Test-HostPort {
  param([AllowEmptyString()][string]$Value)

  $redisHost = ''
  $portText = ''
  $bracketedMatch = [regex]::Match($Value, '^\[(?<host>[^\]]+)\]:(?<port>[0-9]+)$')
  if ($bracketedMatch.Success) {
    $redisHost = $bracketedMatch.Groups['host'].Value
    $portText = $bracketedMatch.Groups['port'].Value
    $ipAddress = $null
    if (-not [Net.IPAddress]::TryParse($redisHost, [ref]$ipAddress)) {
      return $false
    }
    if ($ipAddress.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetworkV6) {
      return $false
    }
  }
  else {
    $hostPortMatch = [regex]::Match($Value, '^(?<host>[^:\s]+):(?<port>[0-9]+)$')
    if (-not $hostPortMatch.Success) {
      return $false
    }

    $redisHost = $hostPortMatch.Groups['host'].Value
    $portText = $hostPortMatch.Groups['port'].Value
    $ipAddress = $null
    if ([Net.IPAddress]::TryParse($redisHost, [ref]$ipAddress)) {
      if ($ipAddress.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork) {
        return $false
      }
    }
    elseif (-not (Test-Hostname $redisHost)) {
      return $false
    }
  }

  $port = 0
  if (-not [int]::TryParse($portText, [ref]$port)) {
    return $false
  }
  return $port -ge 1 -and $port -le 65535
}

function Test-CorsOrigin {
  param([AllowEmptyString()][string]$Value)

  $originMatch = [regex]::Match($Value, '(?i)^https?://(?<authority>[^/\\?#\s]+)/?$')
  if (-not $originMatch.Success) {
    return $false
  }

  $authority = $originMatch.Groups['authority'].Value
  if ($authority.Contains('@')) {
    return $false
  }

  if ($authority.StartsWith('[')) {
    $authorityMatch = [regex]::Match($authority, '^\[[^\]]+\](?<portDelimiter>:(?<port>[0-9]*))?$')
  }
  else {
    $authorityMatch = [regex]::Match($authority, '^[^:]+(?<portDelimiter>:(?<port>[0-9]*))?$')
  }
  if (-not $authorityMatch.Success) {
    return $false
  }

  if ($authorityMatch.Groups['portDelimiter'].Success) {
    $port = 0
    $portText = $authorityMatch.Groups['port'].Value
    if (-not [int]::TryParse($portText, [ref]$port) -or $port -lt 1 -or $port -gt 65535) {
      return $false
    }
  }

  $parsedUri = $null
  if (-not [Uri]::TryCreate($Value, [UriKind]::Absolute, [ref]$parsedUri)) {
    return $false
  }
  if ($parsedUri.Scheme -ine 'http' -and $parsedUri.Scheme -ine 'https') {
    return $false
  }
  if ([string]::IsNullOrEmpty($parsedUri.Host) -or -not [string]::IsNullOrEmpty($parsedUri.UserInfo)) {
    return $false
  }
  if (-not [string]::IsNullOrEmpty($parsedUri.Query) -or -not [string]::IsNullOrEmpty($parsedUri.Fragment)) {
    return $false
  }
  return $parsedUri.AbsolutePath -eq '/'
}

function Get-ReusableSecret {
  param([Parameter(Mandatory = $true)][string]$Path)

  if (-not [IO.File]::Exists($Path)) {
    return $null
  }

  try {
    $existingContent = [IO.File]::ReadAllText($Path)
  }
  catch {
    throw 'existing runtime env could not be read.'
  }

  $secretMatches = [regex]::Matches($existingContent, '(?m)^APP_SECRET=([^\r\n]*)(?=\r?$)')
  if ($secretMatches.Count -ne 1) {
    return $null
  }

  $candidate = $secretMatches[0].Groups[1].Value
  if ([Text.Encoding]::UTF8.GetByteCount($candidate) -lt 64 -or $candidate -notmatch '^[A-Za-z0-9._~+/=-]+$') {
    return $null
  }

  $semanticValue = $candidate.Trim()
  $unsafeSecrets = @(
    '',
    'CHANGE_ME_TO_64_PLUS_RANDOM_CHARS',
    'change_me_to_at_least_64_random_chars',
    'change_me_to_long_random'
  )
  if ($unsafeSecrets -ccontains $semanticValue) {
    return $null
  }
  return $candidate
}

function New-AppSecret {
  $randomBytes = New-Object byte[] 48
  $randomNumberGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $randomNumberGenerator.GetBytes($randomBytes)
  }
  finally {
    $randomNumberGenerator.Dispose()
  }
  return [Convert]::ToBase64String($randomBytes)
}

function Set-EnvValue {
  param(
    [Parameter(Mandatory = $true)][string]$Content,
    [Parameter(Mandatory = $true)][string]$Name,
    [AllowEmptyString()][Parameter(Mandatory = $true)][string]$Value
  )

  $linePattern = '(?m)^' + [regex]::Escape($Name) + '=[^\r\n]*(?=\r?$)'
  $lineMatches = [regex]::Matches($Content, $linePattern)
  if ($lineMatches.Count -ne 1) {
    throw 'runtime env template does not match required placeholders.'
  }

  $lineMatch = $lineMatches[0]
  return $Content.Substring(0, $lineMatch.Index) + $Name + '=' + $Value + $Content.Substring($lineMatch.Index + $lineMatch.Length)
}

function Assert-SingleEnvValue {
  param(
    [Parameter(Mandatory = $true)][string]$Content,
    [Parameter(Mandatory = $true)][string]$Name,
    [AllowEmptyString()][Parameter(Mandatory = $true)][string]$ExpectedValue,
    [Parameter(Mandatory = $true)][string]$FailureReason
  )

  $linePattern = '(?m)^' + [regex]::Escape($Name) + '=([^\r\n]*)(?=\r?$)'
  $lineMatches = [regex]::Matches($Content, $linePattern)
  if ($lineMatches.Count -ne 1 -or $lineMatches[0].Groups[1].Value -cne $ExpectedValue) {
    throw $FailureReason
  }
}

function Get-PathStringComparison {
  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    return [StringComparison]::OrdinalIgnoreCase
  }
  return [StringComparison]::Ordinal
}

function Test-PathInsideRoot {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Root
  )

  $comparison = Get-PathStringComparison
  $trimCharacters = [char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
  $rootPrefix = $Root.TrimEnd($trimCharacters) + [IO.Path]::DirectorySeparatorChar
  return $Path.Equals($Root, $comparison) -or $Path.StartsWith($rootPrefix, $comparison)
}

function Test-PathContainsReparsePoint {
  param([Parameter(Mandatory = $true)][string]$Path)

  $pathRoot = [IO.Path]::GetPathRoot($Path)
  $currentPath = $pathRoot
  $separatorCharacters = [char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
  $segments = $Path.Substring($pathRoot.Length).Split($separatorCharacters, [StringSplitOptions]::RemoveEmptyEntries)

  foreach ($segment in $segments) {
    $currentPath = Join-Path $currentPath $segment
    try {
      $attributes = [IO.File]::GetAttributes($currentPath)
      if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        return $true
      }
    }
    catch [IO.FileNotFoundException] {
    }
    catch [IO.DirectoryNotFoundException] {
    }
    catch {
      throw 'OutputPath could not be inspected safely.'
    }
  }

  return $false
}

function New-SecureTemporaryStream {
  param([Parameter(Mandatory = $true)][string]$Path)

  $safeHandle = $null
  try {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
      return [IO.File]::Open($Path, [IO.FileMode]::CreateNew, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    }

    $rightsConstructor = [IO.FileStream].GetConstructor(
      [type[]]@(
        [string],
        [IO.FileMode],
        [Security.AccessControl.FileSystemRights],
        [IO.FileShare],
        [int],
        [IO.FileOptions]
      )
    )
    if ($null -ne $rightsConstructor) {
      $rightsConstructorArguments = New-Object 'object[]' 6
      $rightsConstructorArguments[0] = [string]$Path
      $rightsConstructorArguments[1] = [IO.FileMode]::CreateNew
      $rightsConstructorArguments[2] = [Security.AccessControl.FileSystemRights]::FullControl
      $rightsConstructorArguments[3] = [IO.FileShare]::None
      $rightsConstructorArguments[4] = [int]4096
      $rightsConstructorArguments[5] = [IO.FileOptions]::WriteThrough
      return [IO.FileStream]$rightsConstructor.Invoke($rightsConstructorArguments)
    }

    $nativeWindowsType = 'AdminLocalEnvWindowsFile' -as [type]
    if ($null -eq $nativeWindowsType) {
      $nativeWindowsType = Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

public static class AdminLocalEnvWindowsFile
{
    private const uint GENERIC_READ = 0x80000000u;
    private const uint GENERIC_WRITE = 0x40000000u;
    private const uint WRITE_DAC = 0x00040000u;
    private const uint WRITE_OWNER = 0x00080000u;
    private const uint CREATE_NEW = 1u;
    private const uint FILE_ATTRIBUTE_NORMAL = 0x00000080u;
    private const uint FILE_FLAG_WRITE_THROUGH = 0x80000000u;

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true, EntryPoint = "CreateFileW")]
    private static extern SafeFileHandle CreateFileW(
        string fileName,
        uint desiredAccess,
        uint shareMode,
        IntPtr securityAttributes,
        uint creationDisposition,
        uint flagsAndAttributes,
        IntPtr templateFile);

    public static SafeFileHandle CreateNewReadWrite(string path)
    {
        return CreateFileW(
            path,
            GENERIC_READ | GENERIC_WRITE | WRITE_DAC | WRITE_OWNER,
            0u,
            IntPtr.Zero,
            CREATE_NEW,
            FILE_ATTRIBUTE_NORMAL | FILE_FLAG_WRITE_THROUGH,
            IntPtr.Zero);
    }
}
'@ -PassThru
    }

    $createNewMethod = $nativeWindowsType.GetMethod('CreateNewReadWrite', [type[]]@([string]))
    if ($null -eq $createNewMethod) {
      throw 'secure temporary runtime env creation failed.'
    }
    $safeHandle = [Microsoft.Win32.SafeHandles.SafeFileHandle]$createNewMethod.Invoke(
      $null,
      [object[]]@($Path)
    )
    if ($null -eq $safeHandle -or $safeHandle.IsClosed -or $safeHandle.IsInvalid) {
      throw 'secure temporary runtime env creation failed.'
    }

    $safeHandleConstructor = [IO.FileStream].GetConstructor(
      [type[]]@([Microsoft.Win32.SafeHandles.SafeFileHandle], [IO.FileAccess])
    )
    if ($null -eq $safeHandleConstructor) {
      throw 'secure temporary runtime env creation failed.'
    }
    $safeHandleConstructorArguments = New-Object 'object[]' 2
    $safeHandleConstructorArguments[0] = $safeHandle
    $safeHandleConstructorArguments[1] = [IO.FileAccess]::ReadWrite
    $stream = [IO.FileStream]$safeHandleConstructor.Invoke($safeHandleConstructorArguments)
    if ($null -eq $stream) {
      throw 'secure temporary runtime env creation failed.'
    }

    $safeHandle = $null
    return $stream
  }
  catch {
    throw 'secure temporary runtime env creation failed.'
  }
  finally {
    if ($null -ne $safeHandle) {
      $safeHandle.Dispose()
    }
  }
}

function Set-OwnerOnlyPermissions {
  param([Parameter(Mandatory = $true)][IO.FileStream]$Stream)

  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = New-Object Security.AccessControl.FileSecurity
    $acl.SetOwner($currentSid)
    $acl.SetAccessRuleProtection($true, $false)
    $accessRule = New-Object Security.AccessControl.FileSystemAccessRule(
      $currentSid,
      [Security.AccessControl.FileSystemRights]::FullControl,
      [Security.AccessControl.AccessControlType]::Allow
    )
    $acl.SetAccessRule($accessRule)
    $setAccessControl = $Stream.GetType().GetMethod(
      'SetAccessControl',
      [type[]]@([Security.AccessControl.FileSecurity])
    )
    if ($null -ne $setAccessControl) {
      $Stream.SetAccessControl($acl)
      return
    }

    $aclExtensionsType = $null
    foreach ($assembly in [AppDomain]::CurrentDomain.GetAssemblies()) {
      $aclExtensionsType = $assembly.GetType('System.IO.FileSystemAclExtensions')
      if ($null -ne $aclExtensionsType) {
        break
      }
    }
    if ($null -eq $aclExtensionsType) {
      try {
        $aclAssembly = [Reflection.Assembly]::Load('System.IO.FileSystem.AccessControl')
        $aclExtensionsType = $aclAssembly.GetType('System.IO.FileSystemAclExtensions')
      }
      catch {
      }
    }
    if ($null -eq $aclExtensionsType) {
      throw 'owner-only runtime env permissions are unavailable.'
    }
    $extensionMethod = $aclExtensionsType.GetMethod(
      'SetAccessControl',
      [type[]]@([IO.FileStream], [Security.AccessControl.FileSecurity])
    )
    if ($null -eq $extensionMethod) {
      throw 'owner-only runtime env permissions are unavailable.'
    }
    [void]$extensionMethod.Invoke($null, [object[]]@($Stream, $acl))
    return
  }

  $nativeUnixType = 'AdminLocalEnvUnixFile' -as [type]
  if ($null -eq $nativeUnixType) {
    $nativeUnixType = Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class AdminLocalEnvUnixFile
{
    [DllImport("libc", SetLastError = true)]
    public static extern int fchmod(IntPtr fileDescriptor, uint mode);
}
'@ -PassThru
  }

  if ($Stream.SafeFileHandle.IsClosed -or $Stream.SafeFileHandle.IsInvalid) {
    throw 'owner-only runtime env permissions are unavailable.'
  }
  $fileDescriptor = $Stream.SafeFileHandle.DangerousGetHandle()
  $fchmodResult = [int]$nativeUnixType.GetMethod('fchmod').Invoke(
    $null,
    [object[]]@($fileDescriptor, [uint32]384)
  )
  if ($fchmodResult -ne 0) {
    throw 'owner-only runtime env permissions are unavailable.'
  }
}

function Assert-OwnerOnlyFilePermissions {
  param([Parameter(Mandatory = $true)][string]$Path)

  if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = Get-Acl -LiteralPath $Path
    if (-not $acl.AreAccessRulesProtected -or
        $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -cne $currentSid.Value) {
      throw 'runtime env permissions could not be verified.'
    }

    $hasCurrentUserFullControl = $false
    $rules = $acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier])
    foreach ($rule in $rules) {
      if ($rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) {
        continue
      }
      if ($rule.IdentityReference.Value -cne $currentSid.Value) {
        throw 'runtime env permissions could not be verified.'
      }
      if ($rule.IdentityReference.Value -ceq $currentSid.Value -and
          ($rule.FileSystemRights -band [Security.AccessControl.FileSystemRights]::FullControl) -eq [Security.AccessControl.FileSystemRights]::FullControl) {
        $hasCurrentUserFullControl = $true
      }
    }
    if (-not $hasCurrentUserFullControl) {
      throw 'runtime env permissions could not be verified.'
    }
    return
  }

  $unixFileModeType = [IO.File].Assembly.GetType('System.IO.UnixFileMode')
  $getUnixFileMode = $null
  if ($null -ne $unixFileModeType) {
    $getUnixFileMode = [IO.File].GetMethod('GetUnixFileMode', [type[]]@([string]))
  }
  if ($null -eq $getUnixFileMode) {
    throw 'runtime env permissions could not be verified.'
  }
  $mode = [int]$getUnixFileMode.Invoke($null, [object[]]@($Path))
  if (($mode -band 511) -ne 384) {
    throw 'runtime env permissions could not be verified.'
  }
}

function Assert-NoReparseDuringWrite {
  param(
    [Parameter(Mandatory = $true)][string]$OutputPath,
    [AllowEmptyString()][string]$TemporaryPath = ''
  )

  if ((Test-PathContainsReparsePoint $OutputPath) -or
      (-not [string]::IsNullOrEmpty($TemporaryPath) -and (Test-PathContainsReparsePoint $TemporaryPath))) {
    throw 'runtime env path changed during initialization.'
  }
}

function Move-FileWithOverwrite {
  param(
    [Parameter(Mandatory = $true)][string]$SourcePath,
    [Parameter(Mandatory = $true)][string]$DestinationPath
  )

  $overwriteMove = [IO.File].GetMethod('Move', [type[]]@([string], [string], [bool]))
  if ($null -ne $overwriteMove) {
    [void]$overwriteMove.Invoke($null, [object[]]@($SourcePath, $DestinationPath, $true))
    return
  }

  $nativeFileType = 'AdminLocalEnvNativeFile' -as [type]
  if ($null -eq $nativeFileType) {
    $nativeFileType = Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;

public static class AdminLocalEnvNativeFile
{
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    public static extern bool MoveFileEx(string sourcePath, string destinationPath, int flags);
}
'@ -PassThru
  }

  $replaceExistingAndWriteThrough = 1 -bor 8
  $moved = [bool]$nativeFileType.GetMethod('MoveFileEx').Invoke(
    $null,
    [object[]]@($SourcePath, $DestinationPath, $replaceExistingAndWriteThrough)
  )
  if (-not $moved) {
    throw 'atomic runtime env replacement failed.'
  }
}

foreach ($parameterValue in @($OutputPath, $MySQLDSN, $RedisAddress, $CorsOrigin)) {
  if (Test-ContainsLineBreak $parameterValue) {
    throw 'parameters must not contain CR or LF.'
  }
}

if (-not (Test-AdminMySQLDSN $MySQLDSN)) {
  throw 'MySQLDSN must use the Compose-safe canonical local format.'
}
if (-not (Test-HostPort $RedisAddress)) {
  throw 'RedisAddress must be a valid host:port with port 1..65535.'
}
if (-not (Test-CorsOrigin $CorsOrigin)) {
  throw 'CorsOrigin must be a plain HTTP(S) origin.'
}
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
  throw 'OutputPath must identify a file in an existing directory.'
}

try {
  $fullOutputPath = [IO.Path]::GetFullPath($OutputPath)
  $outputDirectory = [IO.Path]::GetDirectoryName($fullOutputPath)
  $outputFileName = [IO.Path]::GetFileName($fullOutputPath)
  $repositoryRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
  $defaultOutputPath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'admin-go.env'))
}
catch {
  throw 'OutputPath must identify a file in an existing directory.'
}
if ([string]::IsNullOrWhiteSpace($outputFileName) -or -not [IO.Directory]::Exists($outputDirectory)) {
  throw 'OutputPath must identify a file in an existing directory.'
}

$pathComparison = Get-PathStringComparison
$isDefaultOutput = $fullOutputPath.Equals($defaultOutputPath, $pathComparison)
if ((Test-PathInsideRoot -Path $fullOutputPath -Root $repositoryRoot) -and -not $isDefaultOutput) {
  throw 'OutputPath must be the default ignored env or outside the repository.'
}
if (Test-PathContainsReparsePoint $fullOutputPath) {
  throw 'OutputPath must not use a symbolic link, junction, or reparse point.'
}

$templatePath = Join-Path $PSScriptRoot 'admin-go.env.example'
try {
  $renderedContent = [IO.File]::ReadAllText($templatePath)
}
catch {
  throw 'runtime env template could not be read.'
}

$templateValues = [ordered]@{
  APP_ENV = 'production'
  MYSQL_DSN = 'admin_user:CHANGE_ME@tcp(DB_PRIVATE_IP:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local'
  REDIS_ADDR = 'REDIS_PRIVATE_IP:6379'
  APP_SECRET = 'CHANGE_ME_TO_64_PLUS_RANDOM_CHARS'
  CORS_ALLOW_ORIGINS = 'https://FRONTEND_DOMAIN_REQUIRED'
}
foreach ($templateValue in $templateValues.GetEnumerator()) {
  Assert-SingleEnvValue `
    -Content $renderedContent `
    -Name $templateValue.Key `
    -ExpectedValue $templateValue.Value `
    -FailureReason 'runtime env template does not match required placeholders.'
}

$appSecret = Get-ReusableSecret $fullOutputPath
if ([string]::IsNullOrEmpty($appSecret)) {
  $appSecret = New-AppSecret
}

$renderedContent = Set-EnvValue -Content $renderedContent -Name 'APP_ENV' -Value 'local'
$renderedContent = Set-EnvValue -Content $renderedContent -Name 'MYSQL_DSN' -Value $MySQLDSN
$renderedContent = Set-EnvValue -Content $renderedContent -Name 'REDIS_ADDR' -Value $RedisAddress
$renderedContent = Set-EnvValue -Content $renderedContent -Name 'APP_SECRET' -Value $appSecret
$renderedContent = Set-EnvValue -Content $renderedContent -Name 'CORS_ALLOW_ORIGINS' -Value $CorsOrigin

$renderedValues = [ordered]@{
  APP_ENV = 'local'
  MYSQL_DSN = $MySQLDSN
  REDIS_ADDR = $RedisAddress
  APP_SECRET = $appSecret
  CORS_ALLOW_ORIGINS = $CorsOrigin
}
foreach ($renderedValue in $renderedValues.GetEnumerator()) {
  Assert-SingleEnvValue `
    -Content $renderedContent `
    -Name $renderedValue.Key `
    -ExpectedValue $renderedValue.Value `
    -FailureReason 'generated runtime env values could not be verified.'
}

$temporaryPath = Join-Path $outputDirectory ('.' + $outputFileName + '.' + [guid]::NewGuid().ToString('N') + '.tmp')
$runtimeEnvWriteFailed = $false
$temporaryStream = $null
try {
  $temporaryStream = New-SecureTemporaryStream -Path $temporaryPath
  Assert-NoReparseDuringWrite -OutputPath $fullOutputPath -TemporaryPath $temporaryPath
  Set-OwnerOnlyPermissions -Stream $temporaryStream
  Assert-NoReparseDuringWrite -OutputPath $fullOutputPath -TemporaryPath $temporaryPath

  $utf8WithoutBom = New-Object Text.UTF8Encoding($false)
  $renderedBytes = $utf8WithoutBom.GetBytes($renderedContent)
  $temporaryStream.Write($renderedBytes, 0, $renderedBytes.Length)
  $temporaryStream.Flush($true)
  $temporaryStream.Dispose()
  $temporaryStream = $null

  Assert-NoReparseDuringWrite -OutputPath $fullOutputPath -TemporaryPath $temporaryPath
  if ([IO.File]::ReadAllText($temporaryPath) -cne $renderedContent) {
    throw 'temporary runtime env content could not be verified.'
  }
  Assert-OwnerOnlyFilePermissions $temporaryPath
  Assert-NoReparseDuringWrite -OutputPath $fullOutputPath -TemporaryPath $temporaryPath

  if ([IO.File]::Exists($fullOutputPath)) {
    Move-FileWithOverwrite -SourcePath $temporaryPath -DestinationPath $fullOutputPath
  }
  else {
    [IO.File]::Move($temporaryPath, $fullOutputPath)
  }
  $temporaryPath = $null

  Assert-NoReparseDuringWrite -OutputPath $fullOutputPath
  if ([IO.File]::ReadAllText($fullOutputPath) -cne $renderedContent) {
    throw 'final runtime env content could not be verified.'
  }
  Assert-OwnerOnlyFilePermissions $fullOutputPath
  Assert-NoReparseDuringWrite -OutputPath $fullOutputPath
}
catch {
  $runtimeEnvWriteFailed = $true
}

$runtimeEnvCleanupFailed = $false
if ($null -ne $temporaryStream) {
  try {
    $temporaryStream.Dispose()
    $temporaryStream = $null
  }
  catch {
    $runtimeEnvCleanupFailed = $true
  }
}
if (-not [string]::IsNullOrEmpty($temporaryPath) -and [IO.File]::Exists($temporaryPath)) {
  try {
    [IO.File]::Delete($temporaryPath)
  }
  catch {
    $runtimeEnvCleanupFailed = $true
  }
}

if ($runtimeEnvCleanupFailed) {
  if ($runtimeEnvWriteFailed) {
    throw 'runtime env cleanup failed after runtime env write failure.'
  }
  throw 'runtime env cleanup failed.'
}
if ($runtimeEnvWriteFailed) {
  throw 'runtime env could not be written.'
}

if ($isDefaultOutput) {
  Write-Output "created ignored runtime env at $OutputPath"
}
else {
  Write-Output "created runtime env at $OutputPath"
}
