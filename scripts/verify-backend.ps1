[CmdletBinding()]
param()

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

function Assert-SafeDirectory {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path
  )

  $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
  if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw "refusing to use reparse point as verification directory: $($item.FullName)"
  }
  if (-not $item.PSIsContainer) {
    throw "verification directory path is not a directory: $($item.FullName)"
  }
}

function Assert-NoReparsePointsInTree {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path
  )

  $pending = New-Object System.Collections.Stack
  $pending.Push($Path)
  while ($pending.Count -gt 0) {
    $current = [string]$pending.Pop()
    $item = Get-Item -LiteralPath $current -Force -ErrorAction Stop
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
      throw "refusing to remove reparse point from verification directory: $($item.FullName)"
    }
    if ($item.PSIsContainer) {
      foreach ($child in @(Get-ChildItem -LiteralPath $current -Force -ErrorAction Stop)) {
        $pending.Push($child.FullName)
      }
    }
  }
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$temporaryDirectory = Join-Path $repoRoot ".tmp"
$binaryDirectory = Join-Path $repoRoot ".tmp/verify-bin"
$locationPushed = $false
$verificationFailure = $null
$cleanupErrors = @()

try {
  if (-not (Test-Path -LiteralPath $temporaryDirectory)) {
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
  }
  Assert-SafeDirectory -Path $temporaryDirectory

  if (Test-Path -LiteralPath $binaryDirectory) {
    Assert-NoReparsePointsInTree -Path $binaryDirectory
    Remove-Item -LiteralPath $binaryDirectory -Recurse -Force -ErrorAction Stop
  }
  New-Item -ItemType Directory -Path $binaryDirectory | Out-Null
  Assert-SafeDirectory -Path $binaryDirectory

  Push-Location -LiteralPath $repoRoot
  $locationPushed = $true

  Invoke-GoCommand -Arguments @("test", "./...")
  & (Join-Path $PSScriptRoot "verify-runtime-contracts.ps1")
  & (Join-Path $PSScriptRoot "verify-identity-routing.ps1")
  & (Join-Path $PSScriptRoot "verify-durable-work.ps1")
  Invoke-GoCommand -Arguments @("vet", "./...")
  Invoke-GoCommand -Arguments @("run", "honnef.co/go/tools/cmd/staticcheck@v0.8.0-rc.1", "./...")
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
  if ($locationPushed) {
    try {
      Pop-Location -ErrorAction Stop
    } catch {
      $cleanupErrors += "restore location: $($_.Exception.Message)"
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
