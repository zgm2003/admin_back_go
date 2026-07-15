[CmdletBinding()]
param(
  [switch]$KeepScratch
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-GoCommand {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$Arguments
  )

  $commandText = "go " + ($Arguments -join " ")
  Write-Host "==> $commandText"
  & go @Arguments
  $exitCode = $LASTEXITCODE
  if ($exitCode -ne 0) {
    throw "$commandText failed with exit code $exitCode."
  }
}

function Get-ProcessEnvironmentVariableState {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name
  )

  $environment = [Environment]::GetEnvironmentVariables(
    [EnvironmentVariableTarget]::Process
  )
  $isSet = $environment.Contains($Name)
  $value = $null
  if ($isSet) {
    $value = [string]$environment[$Name]
  }

  return [pscustomobject]@{
    IsSet = $isSet
    Value = $value
  }
}

function Restore-ProcessEnvironmentVariableState {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,

    [Parameter(Mandatory = $true)]
    [psobject]$State
  )

  if (-not $State.IsSet) {
    [Environment]::SetEnvironmentVariable(
      $Name,
      $null,
      [EnvironmentVariableTarget]::Process
    )
    return
  }

  $isWindows = [IO.Path]::DirectorySeparatorChar -eq "\"
  if ($isWindows -and $State.Value.Length -eq 0) {
    if ($null -eq ("AdminGoVerificationNativeEnvironment" -as [type])) {
      Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;

public static class AdminGoVerificationNativeEnvironment
{
    [DllImport("kernel32.dll", EntryPoint = "SetEnvironmentVariableW", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    public static extern bool SetEnvironmentVariable(string name, string value);
}
'@
    }

    if (-not [AdminGoVerificationNativeEnvironment]::SetEnvironmentVariable($Name, [string]::Empty)) {
      $errorCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
      throw "SetEnvironmentVariableW failed with Win32 error $errorCode."
    }
    return
  }

  [Environment]::SetEnvironmentVariable(
    $Name,
    [string]$State.Value,
    [EnvironmentVariableTarget]::Process
  )
}

function Test-PathIsUnderDirectory {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,

    [Parameter(Mandatory = $true)]
    [string]$ParentDirectory
  )

  $resolvedPath = [IO.Path]::GetFullPath(
    (Resolve-Path -LiteralPath $Path -ErrorAction Stop).Path
  )
  $resolvedParent = [IO.Path]::GetFullPath(
    (Resolve-Path -LiteralPath $ParentDirectory -ErrorAction Stop).Path
  )
  $resolvedParent = $resolvedParent.TrimEnd(
    [char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
  )
  $parentPrefix = $resolvedParent + [IO.Path]::DirectorySeparatorChar
  $comparison = [StringComparison]::Ordinal
  if ([IO.Path]::DirectorySeparatorChar -eq "\") {
    $comparison = [StringComparison]::OrdinalIgnoreCase
  }

  return $resolvedPath.StartsWith($parentPrefix, $comparison)
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$systemTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$scratchRoot = Join-Path $systemTemp ("admin-go-verify-" + [guid]::NewGuid().ToString("N"))
$moduleCache = Join-Path $scratchRoot "modcache"
$binaryDirectory = Join-Path $scratchRoot "bin"

$originalGoModCache = Get-ProcessEnvironmentVariableState -Name "GOMODCACHE"

$scratchCreated = $false
$locationPushed = $false
$verificationFailure = $null
$cleanupErrors = @()

try {
  New-Item -ItemType Directory -Path $scratchRoot | Out-Null
  $scratchCreated = $true
  New-Item -ItemType Directory -Path $moduleCache, $binaryDirectory | Out-Null

  [Environment]::SetEnvironmentVariable(
    "GOMODCACHE",
    $moduleCache,
    [EnvironmentVariableTarget]::Process
  )

  Push-Location -LiteralPath $repoRoot
  $locationPushed = $true

  Invoke-GoCommand -Arguments @("mod", "download")
  Invoke-GoCommand -Arguments @("mod", "verify")
  Invoke-GoCommand -Arguments @("test", "./...")
  Invoke-GoCommand -Arguments @(
    "test",
    "-race",
    "./internal/module/auth",
    "./internal/module/payment/...",
    "./internal/infra/taskqueue",
    "./internal/infra/realtime/..."
  )
  Invoke-GoCommand -Arguments @("vet", "./...")
  Invoke-GoCommand -Arguments @("run", "honnef.co/go/tools/cmd/staticcheck@v0.7.0", "./...")
  Invoke-GoCommand -Arguments @("run", "golang.org/x/vuln/cmd/govulncheck@v1.6.0", "./...")
  Invoke-GoCommand -Arguments @(
    "build",
    "-trimpath",
    "-o",
    (Join-Path $binaryDirectory "admin-api.exe"),
    "./cmd/admin-api"
  )
  Invoke-GoCommand -Arguments @(
    "build",
    "-trimpath",
    "-o",
    (Join-Path $binaryDirectory "admin-worker.exe"),
    "./cmd/admin-worker"
  )
} catch {
  $verificationFailure = $_
} finally {
  try {
    Restore-ProcessEnvironmentVariableState -Name "GOMODCACHE" -State $originalGoModCache
  } catch {
    $cleanupErrors += "restore GOMODCACHE: $($_.Exception.Message)"
  }

  if ($locationPushed) {
    try {
      Pop-Location -ErrorAction Stop
    } catch {
      $cleanupErrors += "restore location: $($_.Exception.Message)"
    }
  }

  if ($scratchCreated) {
    if ($KeepScratch) {
      Write-Host "Scratch retained at: $scratchRoot"
    } else {
      try {
        if (-not (Test-PathIsUnderDirectory -Path $scratchRoot -ParentDirectory $systemTemp)) {
          throw "refusing to remove scratch path outside system temp: $scratchRoot"
        }
        Remove-Item -LiteralPath $scratchRoot -Recurse -Force -ErrorAction Stop
      } catch {
        $cleanupErrors += "remove scratch directory: $($_.Exception.Message)"
      }
    }
  }
}

if ($cleanupErrors.Count -gt 0) {
  $cleanupMessage = $cleanupErrors -join [Environment]::NewLine
  if ($null -ne $verificationFailure) {
    throw "$($verificationFailure.Exception.Message)$([Environment]::NewLine)Cleanup failed:$([Environment]::NewLine)$cleanupMessage"
  }
  throw "Cleanup failed:$([Environment]::NewLine)$cleanupMessage"
}

if ($null -ne $verificationFailure) {
  throw $verificationFailure
}
