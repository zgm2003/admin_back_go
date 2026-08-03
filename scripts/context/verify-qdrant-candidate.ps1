[CmdletBinding()]
param(
  [Parameter(Mandatory)]
  [ValidateSet('qdrant/qdrant:v1.18.3')]
  [string] $CandidateImage,

  [Parameter(Mandatory)]
  [string] $PinEnv
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$containerName = 'admin-context-qdrant-contract'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$approvedPin = [IO.Path]::GetFullPath((Join-Path $repoRoot 'deploy\docker-state\qdrant-image.env'))
$resolvedPin = if ([IO.Path]::IsPathRooted($PinEnv)) {
  [IO.Path]::GetFullPath($PinEnv)
}
else {
  [IO.Path]::GetFullPath((Join-Path $repoRoot $PinEnv))
}
if (-not [string]::Equals($resolvedPin, $approvedPin, [StringComparison]::OrdinalIgnoreCase)) {
  throw "PinEnv must resolve to $approvedPin"
}

function Resolve-DockerExecutable {
  $preferred = 'E:\Docker\Docker\resources\bin\docker.exe'
  if (Test-Path -LiteralPath $preferred -PathType Leaf) {
    return $preferred
  }
  $command = Get-Command docker.exe -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $command) {
    $command = Get-Command docker -ErrorAction Stop | Select-Object -First 1
  }
  return $command.Source
}

function Invoke-Docker {
  param([Parameter(Mandatory)][string[]] $Arguments)

  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = 'Continue'
    $output = @(& $script:DockerExecutable @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $previousPreference
  }
  if ($exitCode -ne 0) {
    $tail = (@($output | ForEach-Object { $_.ToString() }) | Select-Object -Last 20) -join "`n"
    throw "docker $($Arguments -join ' ') failed with exit code $exitCode`n$tail"
  }
  return @($output | ForEach-Object { $_.ToString() })
}

function Test-ContainerExists {
  param([Parameter(Mandatory)][string] $Name)

  & $script:DockerExecutable container inspect $Name *> $null
  return $LASTEXITCODE -eq 0
}

function Invoke-ContractTest {
  $goExecutable = (Get-Command go -ErrorAction Stop | Select-Object -First 1).Source
  $testArguments = @(
    'test', '-tags=integration', './internal/infra/contextindex/qdrant',
    '-run', 'ServerSupportsContextQueryContract', '-count=1'
  )
  $previousAddress = [Environment]::GetEnvironmentVariable('QDRANT_INTEGRATION_ADDR', 'Process')
  Push-Location $repoRoot
  try {
    $env:QDRANT_INTEGRATION_ADDR = '127.0.0.1:36336'
    $output = @(& $goExecutable @testArguments 2>&1)
    if ($LASTEXITCODE -ne 0) {
      $tail = (@($output | ForEach-Object { $_.ToString() }) | Select-Object -Last 40) -join "`n"
      throw "Qdrant capability contract failed`n$tail"
    }
  }
  finally {
    Pop-Location
    [Environment]::SetEnvironmentVariable('QDRANT_INTEGRATION_ADDR', $previousAddress, 'Process')
  }
}

function Test-ByteEqual {
  param(
    [Parameter(Mandatory)][byte[]] $Left,
    [Parameter(Mandatory)][byte[]] $Right
  )

  if ($Left.Length -ne $Right.Length) {
    return $false
  }
  for ($index = 0; $index -lt $Left.Length; $index++) {
    if ($Left[$index] -ne $Right[$index]) {
      return $false
    }
  }
  return $true
}

function Assert-PinContent {
  param(
    [Parameter(Mandatory)][string] $Path,
    [Parameter(Mandatory)][byte[]] $Expected
  )

  $actual = [IO.File]::ReadAllBytes($Path)
  if (-not (Test-ByteEqual -Left $actual -Right $Expected)) {
    throw "existing Qdrant image lock does not match the tested RepoDigest: $Path"
  }
}

function Write-PinAtomically {
  param(
    [Parameter(Mandatory)][string] $Path,
    [Parameter(Mandatory)][string] $TestedImage
  )

  $directory = Split-Path -Parent $Path
  if (-not (Test-Path -LiteralPath $directory -PathType Container)) {
    throw "Qdrant image lock directory is missing: $directory"
  }
  $canonical = "QDRANT_SERVER_IMAGE=$TestedImage`n"
  $bytes = [Text.UTF8Encoding]::new($false).GetBytes($canonical)
  if (Test-Path -LiteralPath $Path -PathType Leaf) {
    Assert-PinContent -Path $Path -Expected $bytes
    return
  }

  $temporaryPath = $Path + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
  try {
    [IO.File]::WriteAllBytes($temporaryPath, $bytes)
    try {
      [IO.File]::Move($temporaryPath, $Path)
      $temporaryPath = $null
    }
    catch [IO.IOException] {
      if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw
      }
      Assert-PinContent -Path $Path -Expected $bytes
    }
  }
  finally {
    if ($null -ne $temporaryPath -and (Test-Path -LiteralPath $temporaryPath -PathType Leaf)) {
      Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue
    }
  }
}

$script:DockerExecutable = Resolve-DockerExecutable
if (Test-ContainerExists -Name $containerName) {
  throw "reserved verifier container already exists: $containerName"
}

$healthCommand = 'bash -c ''exec 3<>/dev/tcp/127.0.0.1/6333 && printf "GET /readyz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" >&3 && grep -q "200 OK" <&3'''
try {
  Invoke-Docker -Arguments @('pull', $CandidateImage) | Out-Null
  Invoke-Docker -Arguments @('run', '--rm', '--entrypoint', 'bash', $CandidateImage, '--version') | Out-Null
  Invoke-Docker -Arguments @(
    'run', '--detach', '--name', $containerName,
    '--publish', '127.0.0.1:36335:6333',
    '--publish', '127.0.0.1:36336:6334',
    '--health-cmd', $healthCommand,
    '--health-interval', '2s',
    '--health-timeout', '2s',
    '--health-retries', '30',
    '--health-start-period', '5s',
    $CandidateImage
  ) | Out-Null

  $deadline = [DateTimeOffset]::UtcNow.AddSeconds(90)
  $health = ''
  do {
    $health = ((Invoke-Docker -Arguments @('inspect', $containerName, '--format', '{{.State.Health.Status}}')) | Select-Object -Last 1).Trim()
    if ($health -eq 'unhealthy') {
      throw 'candidate failed the exact Compose healthcheck'
    }
    if ($health -eq 'healthy') {
      break
    }
    Start-Sleep -Seconds 1
  } while ([DateTimeOffset]::UtcNow -lt $deadline)
  if ($health -ne 'healthy') {
    throw "candidate health timeout: $health"
  }

  Invoke-ContractTest

  $repoDigestsJSON = ((Invoke-Docker -Arguments @('image', 'inspect', $CandidateImage, '--format', '{{json .RepoDigests}}')) | Select-Object -Last 1).Trim()
  $repoDigests = @($repoDigestsJSON | ConvertFrom-Json)
  $matchingDigests = @($repoDigests | Where-Object { $_ -match '^qdrant/qdrant@sha256:[0-9a-f]{64}$' } | Select-Object -Unique)
  if ($matchingDigests.Count -ne 1) {
    throw "candidate must resolve to exactly one qdrant/qdrant@sha256 RepoDigest: $repoDigestsJSON"
  }
  $digest = $matchingDigests[0].Substring('qdrant/qdrant@sha256:'.Length)
  $testedImage = "$CandidateImage@sha256:$digest"
  Write-PinAtomically -Path $resolvedPin -TestedImage $testedImage

  [pscustomobject]@{
    candidate_image = $CandidateImage
    tested_image    = $testedImage
  } | ConvertTo-Json -Compress
}
finally {
  if (Test-ContainerExists -Name $containerName) {
    Invoke-Docker -Arguments @('rm', '--force', $containerName) | Out-Null
  }
}
