[CmdletBinding()]
param(
  [switch]$ImportFunctions,
  [switch]$Apply,
  [switch]$MaintenanceWindow,
  [string]$Manifest,
  [string]$ImageMetadata,
  [string]$RecoveryArtifact,
  [string]$BackendEnvFile,
  [string]$RuntimeVolume,
  [string]$ExportVolume,
  [string]$PlatformNetwork = 'admin-platform',
  [string]$StagingProject = 'admin-release-staging',
  [string]$ProductionProject = 'admin-app',
  [ValidateRange(1024, 65535)][int]$StagingFrontendPort = 15173,
  [ValidateRange(1024, 65535)][int]$StagingAPIPort = 18080,
  [ValidateRange(1024, 65535)][int]$ProductionFrontendPort = 5173,
  [ValidateRange(1024, 65535)][int]$ProductionAPIPort = 8080,
  [string]$DockerCommand = 'docker'
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$ProgressPreference = 'SilentlyContinue'
Set-StrictMode -Version Latest

$deployWasImported = [bool]$ImportFunctions
$requested = [pscustomobject]@{
  Apply = [bool]$Apply
  MaintenanceWindow = [bool]$MaintenanceWindow
  Manifest = $Manifest
  ImageMetadata = $ImageMetadata
  RecoveryArtifact = $RecoveryArtifact
  BackendEnvFile = $BackendEnvFile
  RuntimeVolume = $RuntimeVolume
  ExportVolume = $ExportVolume
  PlatformNetwork = $PlatformNetwork
  StagingProject = $StagingProject
  ProductionProject = $ProductionProject
  StagingFrontendPort = $StagingFrontendPort
  StagingAPIPort = $StagingAPIPort
  ProductionFrontendPort = $ProductionFrontendPort
  ProductionAPIPort = $ProductionAPIPort
  DockerCommand = $DockerCommand
}
. (Join-Path $PSScriptRoot 'check-release-manifest.ps1') -ImportFunctions
. (Join-Path $script:BackendRoot 'scripts\dev\admin-dev-common.ps1')

function Invoke-ReleaseDocker {
  param(
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label,
    [switch]$Capture
  )
  $lines = @(& $script:ReleaseDockerExecutable @Arguments 2>&1)
  if ($LASTEXITCODE -ne 0) { throw "$Label failed" }
  if ($Capture) { return ($lines | ForEach-Object { $_.ToString() }) -join "`n" }
}

function Invoke-AdminReleaseCompose {
  param(
    [Parameter(Mandatory = $true)][string]$Project,
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Label
  )
  Invoke-ReleaseDocker -Arguments (@('compose', '-p', $Project, '-f', $script:ReleaseComposePath) + $Arguments) -Label $Label
}

function Import-AdminReleaseImages {
  param(
    [Parameter(Mandatory = $true)]$Validation,
    [Parameter(Mandatory = $true)][string]$MetadataPath
  )
  foreach ($pair in @(
    [pscustomobject]@{ Label = 'backend'; Manifest = $Validation.Document.backend; Metadata = $Validation.ImageMetadata.backend },
    [pscustomobject]@{ Label = 'frontend'; Manifest = $Validation.Document.frontend; Metadata = $Validation.ImageMetadata.frontend }
  )) {
    $archivePath = Get-ReleaseArchivePath -MetadataPath $MetadataPath -ImageMetadataEntry $pair.Metadata -Label $pair.Label
    Assert-ExactString (Get-FileSha256 -Path $archivePath) ([string]$pair.Manifest.archive_sha256) "$($pair.Label) archive digest"
    # docker load restores only the archive already tied to the manifest digest.
    [void](Invoke-ReleaseDocker -Arguments @('load', '--input', $archivePath) -Label "docker load $($pair.Label)" -Capture)
    $inspection = Get-DockerImageInspection -DockerExecutable $script:ReleaseDockerExecutable -ImageID ([string]$pair.Metadata.image_id) -Label $pair.Label
    Assert-ExactString ([string]$inspection.Id) ([string]$pair.Metadata.image_id) "$($pair.Label) loaded image ID"
    Assert-ExactString ([string]$inspection.Config.Labels.'org.opencontainers.image.revision') ([string]$pair.Manifest.commit) "$($pair.Label) image revision"
  }
}

function Invoke-AdminReleaseJSONRequest {
  param(
    [Parameter(Mandatory = $true)][Net.Http.HttpClient]$Client,
    [Parameter(Mandatory = $true)][Net.Http.HttpMethod]$Method,
    [Parameter(Mandatory = $true)][string]$BaseURL,
    [Parameter(Mandatory = $true)][string]$Path,
    [AllowNull()][hashtable]$Body,
    [AllowEmptyString()][string]$AccessToken = ''
  )
  $request = [Net.Http.HttpRequestMessage]::new($Method, "$BaseURL$Path")
  try {
    [void]$request.Headers.TryAddWithoutValidation('Origin', $script:AdminReleaseSmokeOrigin)
    [void]$request.Headers.TryAddWithoutValidation('platform', 'admin')
    [void]$request.Headers.TryAddWithoutValidation('device-id', 'admin-release-smoke')
    if (-not [string]::IsNullOrWhiteSpace($AccessToken)) {
      $request.Headers.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $AccessToken)
    }
    if ($null -ne $Body) {
      $request.Content = [Net.Http.StringContent]::new(
        ($Body | ConvertTo-Json -Depth 8 -Compress),
        [Text.Encoding]::UTF8,
        'application/json'
      )
    }
    $response = $Client.SendAsync($request).GetAwaiter().GetResult()
    try {
      if (-not $response.IsSuccessStatusCode) { throw "release smoke request failed: $Path" }
      $content = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
      try { $document = $content | ConvertFrom-Json -Depth 30 } catch { throw "release smoke returned invalid JSON: $Path" }
      if ($null -eq $document -or [int]$document.code -ne 0) { throw "release smoke returned a non-success envelope: $Path" }
      return $document
    } finally {
      $response.Dispose()
    }
  } finally {
    $request.Dispose()
  }
}

function Receive-AdminReleaseWebSocketJSON {
  param(
    [Parameter(Mandatory = $true)][Net.WebSockets.ClientWebSocket]$Socket,
    [Parameter(Mandatory = $true)][Threading.CancellationToken]$Token
  )
  $buffer = [byte[]]::new(65536)
  $stream = [IO.MemoryStream]::new()
  try {
    do {
      $result = $Socket.ReceiveAsync([ArraySegment[byte]]::new($buffer), $Token).GetAwaiter().GetResult()
      if ($result.MessageType -eq [Net.WebSockets.WebSocketMessageType]::Close) { throw 'release realtime smoke closed early' }
      if ($result.MessageType -ne [Net.WebSockets.WebSocketMessageType]::Text) { throw 'release realtime smoke received a non-text frame' }
      $stream.Write($buffer, 0, $result.Count)
      if ($stream.Length -gt 1048576) { throw 'release realtime smoke frame exceeded its limit' }
    } until ($result.EndOfMessage)
    try { return [Text.Encoding]::UTF8.GetString($stream.ToArray()) | ConvertFrom-Json -Depth 30 } catch { throw 'release realtime smoke returned invalid JSON' }
  } finally {
    $stream.Dispose()
  }
}

function Invoke-AdminReleaseRealtimeSmoke {
  param(
    [Parameter(Mandatory = $true)][Net.Http.HttpClient]$Client,
    [Parameter(Mandatory = $true)][string]$FrontendURL,
    [Parameter(Mandatory = $true)][string]$AccessToken
  )
  $ticketResponse = Invoke-AdminReleaseJSONRequest -Client $Client -Method ([Net.Http.HttpMethod]::Post) -BaseURL $FrontendURL -Path '/api/admin/v1/auth/realtime-tickets' -Body @{} -AccessToken $AccessToken
  $ticket = [string]$ticketResponse.data.ticket
  if ([string]::IsNullOrWhiteSpace($ticket) -or [int]$ticketResponse.data.expires_in -le 0) { throw 'release realtime ticket is invalid' }
  $frontendURI = [uri]$FrontendURL
  $scheme = if ($frontendURI.Scheme -ceq 'https') { 'wss' } else { 'ws' }
  $webSocketURI = [uri]("${scheme}://$($frontendURI.Authority)/api/admin/v1/realtime/ws?ticket=" + [uri]::EscapeDataString($ticket))
  $socket = [Net.WebSockets.ClientWebSocket]::new()
  $timeout = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(15))
  try {
    $socket.Options.SetRequestHeader('Origin', $script:AdminReleaseSmokeOrigin)
    $null = $socket.ConnectAsync($webSocketURI, $timeout.Token).GetAwaiter().GetResult()
    $connected = Receive-AdminReleaseWebSocketJSON -Socket $socket -Token $timeout.Token
    if ([string]$connected.type -cne 'realtime.connected.v1' -or [string]$connected.data.platform -cne 'admin') {
      throw 'release realtime connected envelope is invalid'
    }
    $requestID = 'admin-release-ping'
    $payload = @{ type = 'realtime.ping.v1'; request_id = $requestID; data = @{} } | ConvertTo-Json -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($payload)
    $null = $socket.SendAsync([ArraySegment[byte]]::new($bytes), [Net.WebSockets.WebSocketMessageType]::Text, $true, $timeout.Token).GetAwaiter().GetResult()
    $matched = $false
    for ($attempt = 0; $attempt -lt 5 -and -not $matched; $attempt++) {
      $message = Receive-AdminReleaseWebSocketJSON -Socket $socket -Token $timeout.Token
      if ([string]$message.type -ceq 'realtime.error.v1') { throw 'release realtime smoke returned an error envelope' }
      $matched = [string]$message.type -ceq 'realtime.pong.v1' -and [string]$message.request_id -ceq $requestID
    }
    if (-not $matched) { throw 'release realtime smoke did not receive its pong' }
  } finally {
    $ticket = $null
    if ($socket.State -eq [Net.WebSockets.WebSocketState]::Open) {
      try { $null = $socket.CloseAsync([Net.WebSockets.WebSocketCloseStatus]::NormalClosure, 'complete', [Threading.CancellationToken]::None).GetAwaiter().GetResult() } catch { $socket.Abort() }
    }
    $timeout.Dispose()
    $socket.Dispose()
  }
}

function Invoke-AdminReleaseSmoke {
  param(
    [Parameter(Mandatory = $true)][string]$FrontendURL,
    [Parameter(Mandatory = $true)][string]$APIURL
  )
  $account = [string]$env:ADMIN_SMOKE_ACCOUNT
  $password = [string]$env:ADMIN_SMOKE_PASSWORD
  if ([string]::IsNullOrWhiteSpace($account) -or [string]::IsNullOrWhiteSpace($password)) {
    throw 'ADMIN_SMOKE_CREDENTIALS_REQUIRED'
  }
  $script:AdminReleaseSmokeOrigin = [string]$env:ADMIN_RELEASE_SMOKE_ORIGIN
  if ([string]::IsNullOrWhiteSpace($script:AdminReleaseSmokeOrigin)) {
    $script:AdminReleaseSmokeOrigin = 'http://localhost:5173'
  }
  if (-not [uri]::IsWellFormedUriString($script:AdminReleaseSmokeOrigin, [UriKind]::Absolute)) {
    throw 'ADMIN_RELEASE_SMOKE_ORIGIN is invalid'
  }
  $handler = [Net.Http.HttpClientHandler]::new()
  $handler.CookieContainer = [Net.CookieContainer]::new()
  $client = [Net.Http.HttpClient]::new($handler)
  $client.Timeout = [TimeSpan]::FromSeconds(20)
  $accessToken = ''
  try {
    foreach ($target in @("$FrontendURL/healthz", "$APIURL/health", "$APIURL/ready")) {
      $response = $client.GetAsync($target).GetAwaiter().GetResult()
      try { if (-not $response.IsSuccessStatusCode) { throw 'release health smoke failed' } } finally { $response.Dispose() }
    }
    $login = Invoke-AdminReleaseJSONRequest -Client $client -Method ([Net.Http.HttpMethod]::Post) -BaseURL $FrontendURL -Path '/api/admin/v1/auth/login' -Body @{
      login_account = $account.Trim()
      login_type = 'password'
      password = $password
    }
    $accessToken = [string]$login.data.access_token
    if ([string]::IsNullOrWhiteSpace($accessToken) -or [int64]$login.data.expires_in -le 0) { throw 'release login smoke returned an invalid credential' }
    [void](Invoke-AdminReleaseJSONRequest -Client $client -Method ([Net.Http.HttpMethod]::Get) -BaseURL $FrontendURL -Path '/api/admin/v1/users/me' -Body $null -AccessToken $accessToken)
    Invoke-AdminReleaseRealtimeSmoke -Client $client -FrontendURL $FrontendURL -AccessToken $accessToken
  } finally {
    if (-not [string]::IsNullOrWhiteSpace($accessToken)) {
      try { [void](Invoke-AdminReleaseJSONRequest -Client $client -Method ([Net.Http.HttpMethod]::Post) -BaseURL $FrontendURL -Path '/api/admin/v1/auth/logout' -Body @{} -AccessToken $accessToken) } catch { }
    }
    $accessToken = $null
    $password = $null
    $client.Dispose()
    $handler.Dispose()
  }
}

function Set-AdminReleaseComposeEnvironment {
  param(
    [Parameter(Mandatory = $true)]$Validation,
    [Parameter(Mandatory = $true)][string]$EnvFile,
    [Parameter(Mandatory = $true)][string]$RuntimeVolumeName,
    [Parameter(Mandatory = $true)][string]$ExportVolumeName,
    [Parameter(Mandatory = $true)][string]$NetworkName,
    [Parameter(Mandatory = $true)][int]$FrontendPort,
    [Parameter(Mandatory = $true)][int]$APIPort
  )
  $env:ADMIN_FRONTEND_IMAGE = [string]$Validation.ImageMetadata.frontend.image_id
  $env:ADMIN_BACKEND_IMAGE = [string]$Validation.ImageMetadata.backend.image_id
  $env:ADMIN_FRONTEND_REVISION = [string]$Validation.Document.frontend.commit
  $env:ADMIN_BACKEND_REVISION = [string]$Validation.Document.backend.commit
  $env:ADMIN_RELEASE_ID = [string]$Validation.Document.release_id
  $env:ADMIN_BACKEND_ENV_FILE = $EnvFile.Replace('\', '/')
  $env:ADMIN_RUNTIME_VOLUME = $RuntimeVolumeName
  $env:ADMIN_EXPORT_VOLUME = $ExportVolumeName
  $env:ADMIN_PLATFORM_NETWORK = $NetworkName
  $env:ADMIN_FRONTEND_BIND_ADDRESS = '127.0.0.1'
  $env:ADMIN_API_BIND_ADDRESS = '127.0.0.1'
  $env:ADMIN_FRONTEND_PORT = [string]$FrontendPort
  $env:ADMIN_API_PORT = [string]$APIPort
}

function Copy-AdminReleasePackageFile {
  param(
    [Parameter(Mandatory = $true)][string]$Source,
    [Parameter(Mandatory = $true)][string]$Destination
  )
  $sourcePath = Get-RequiredFilePath -Path $Source -Label 'release package source'
  if (Test-Path -LiteralPath $Destination -PathType Leaf) {
    Assert-ExactString (Get-FileSha256 -Path $Destination) (Get-FileSha256 -Path $sourcePath) 'existing release package file'
    return [IO.Path]::GetFullPath($Destination)
  }
  $temporaryPath = $Destination + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
  try {
    Copy-Item -LiteralPath $sourcePath -Destination $temporaryPath
    Move-Item -LiteralPath $temporaryPath -Destination $Destination
  } finally {
    if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
  }
  return [IO.Path]::GetFullPath($Destination)
}

if ($deployWasImported) { return }

$Apply = $requested.Apply
$MaintenanceWindow = $requested.MaintenanceWindow
$Manifest = $requested.Manifest
$ImageMetadata = $requested.ImageMetadata
$RecoveryArtifact = $requested.RecoveryArtifact
$BackendEnvFile = $requested.BackendEnvFile
$RuntimeVolume = $requested.RuntimeVolume
$ExportVolume = $requested.ExportVolume
$PlatformNetwork = $requested.PlatformNetwork
$StagingProject = $requested.StagingProject
$ProductionProject = $requested.ProductionProject
$StagingFrontendPort = $requested.StagingFrontendPort
$StagingAPIPort = $requested.StagingAPIPort
$ProductionFrontendPort = $requested.ProductionFrontendPort
$ProductionAPIPort = $requested.ProductionAPIPort
$DockerCommand = $requested.DockerCommand

if (-not $Apply) { throw 'Admin release deployment requires explicit -Apply' }
if (-not $MaintenanceWindow) { throw 'Admin release deployment requires an approved maintenance window' }
foreach ($required in @(
  [pscustomobject]@{ Value = $Manifest; Label = 'release manifest' },
  [pscustomobject]@{ Value = $RecoveryArtifact; Label = 'recovery artifact' },
  [pscustomobject]@{ Value = $BackendEnvFile; Label = 'backend environment file' },
  [pscustomobject]@{ Value = $RuntimeVolume; Label = 'runtime volume' },
  [pscustomobject]@{ Value = $ExportVolume; Label = 'export volume' }
)) {
  if ([string]::IsNullOrWhiteSpace([string]$required.Value)) { throw "$($required.Label) is required" }
}
foreach ($name in @($RuntimeVolume, $ExportVolume, $PlatformNetwork, $StagingProject, $ProductionProject)) {
  if ($name -cnotmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$') { throw 'Compose resource name is invalid' }
}

$script:ReleaseDockerExecutable = Resolve-ReleaseDocker -Command $DockerCommand
$script:ReleaseComposePath = Join-Path $script:BackendRoot 'deploy\admin-only\docker-compose.yml'
$adminDevLock = Join-Path $script:BackendRoot '.tmp\dev\admin-dev.lock.json'
Assert-NoLiveAdminDevLock -Path $adminDevLock -RepositoryRoot $script:BackendRoot
$manifestPath = Get-RequiredFilePath -Path $Manifest -Label 'release manifest'
if ([string]::IsNullOrWhiteSpace($ImageMetadata)) { $ImageMetadata = Join-Path $script:ReleaseOutputRoot 'images\metadata.json' }
$metadataPath = Get-RequiredFilePath -Path $ImageMetadata -Label 'image metadata'
$envFile = Get-RequiredFilePath -Path $BackendEnvFile -Label 'backend environment file'
$recoveryPath = Assert-ExternalEvidencePath -Path $RecoveryArtifact -Label 'recovery artifact'
$validation = Assert-ReleaseManifest -ManifestPath $manifestPath -InputLockPath $script:DefaultInputLock -PlatformKernelProofPath $script:DefaultPlatformKernelProof -ImageMetadataPath $metadataPath -SkipImageInspection
Assert-ExactString (Get-FileSha256 -Path $recoveryPath) ([string]$validation.Document.evidence.recovery_sha256) 'recovery artifact digest'
[void](Assert-RecoveryArtifact -Path $recoveryPath)

Import-AdminReleaseImages -Validation $validation -MetadataPath $metadataPath
$validation = Assert-ReleaseManifest -ManifestPath $manifestPath -InputLockPath $script:DefaultInputLock -PlatformKernelProofPath $script:DefaultPlatformKernelProof -ImageMetadataPath $metadataPath -DockerExecutable $script:ReleaseDockerExecutable
foreach ($resource in @(
  [pscustomobject]@{ Arguments = @('network', 'inspect', $PlatformNetwork); Label = 'platform network' },
  [pscustomobject]@{ Arguments = @('volume', 'inspect', $RuntimeVolume); Label = 'runtime volume' },
  [pscustomobject]@{ Arguments = @('volume', 'inspect', $ExportVolume); Label = 'export volume' }
)) {
  [void](Invoke-ReleaseDocker -Arguments $resource.Arguments -Label "$($resource.Label) inspection" -Capture)
}

$statePath = Join-Path $script:ReleaseOutputRoot 'deployment-state.json'
$previousState = if (Test-Path -LiteralPath $statePath -PathType Leaf) { Read-JsonEvidence -Path $statePath -Label 'deployment state' } else { $null }
$environmentNames = @(
  'ADMIN_FRONTEND_IMAGE', 'ADMIN_BACKEND_IMAGE', 'ADMIN_FRONTEND_REVISION', 'ADMIN_BACKEND_REVISION',
  'ADMIN_RELEASE_ID', 'ADMIN_BACKEND_ENV_FILE', 'ADMIN_RUNTIME_VOLUME', 'ADMIN_EXPORT_VOLUME',
  'ADMIN_PLATFORM_NETWORK', 'ADMIN_FRONTEND_BIND_ADDRESS', 'ADMIN_API_BIND_ADDRESS',
  'ADMIN_FRONTEND_PORT', 'ADMIN_API_PORT'
)
$previousEnvironment = @{}
foreach ($name in $environmentNames) { $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

try {
  Set-AdminReleaseComposeEnvironment -Validation $validation -EnvFile $envFile -RuntimeVolumeName $RuntimeVolume -ExportVolumeName $ExportVolume -NetworkName $PlatformNetwork -FrontendPort $StagingFrontendPort -APIPort $StagingAPIPort
  if ($null -ne $previousState -and -not [string]::IsNullOrWhiteSpace([string]$previousState.current_project)) {
    Invoke-AdminReleaseCompose -Project ([string]$previousState.current_project) -Arguments @('stop') -Label 'stop previous release project'
  }

  Invoke-AdminReleaseCompose -Project $StagingProject -Arguments @('up', '-d', '--no-build', '--force-recreate', '--wait', '--wait-timeout', '300') -Label 'start staging release'
  Invoke-AdminReleaseSmoke -FrontendURL "http://127.0.0.1:$StagingFrontendPort" -APIURL "http://127.0.0.1:$StagingAPIPort"
  Invoke-AdminReleaseCompose -Project $StagingProject -Arguments @('stop') -Label 'stop verified staging release'

  Set-AdminReleaseComposeEnvironment -Validation $validation -EnvFile $envFile -RuntimeVolumeName $RuntimeVolume -ExportVolumeName $ExportVolume -NetworkName $PlatformNetwork -FrontendPort $ProductionFrontendPort -APIPort $ProductionAPIPort
  Invoke-AdminReleaseCompose -Project $ProductionProject -Arguments @('up', '-d', '--no-build', '--force-recreate', '--wait', '--wait-timeout', '300') -Label 'promote release project'
  Invoke-AdminReleaseSmoke -FrontendURL "http://127.0.0.1:$ProductionFrontendPort" -APIURL "http://127.0.0.1:$ProductionAPIPort"

  $releasePackageDirectory = Join-Path $script:ReleaseOutputRoot ('releases\' + [string]$validation.Document.release_id)
  [IO.Directory]::CreateDirectory($releasePackageDirectory) | Out-Null
  $archivedManifest = Copy-AdminReleasePackageFile -Source $manifestPath -Destination (Join-Path $releasePackageDirectory 'release-manifest.json')
  $archivedProof = Copy-AdminReleasePackageFile -Source $script:DefaultPlatformKernelProof -Destination (Join-Path $releasePackageDirectory 'platform-kernel-proof.json')
  $archivedMetadata = Copy-AdminReleasePackageFile -Source $metadataPath -Destination (Join-Path $releasePackageDirectory 'image-metadata.json')
  $state = [ordered]@{
    schema_version = 1
    current_manifest = $archivedManifest
    current_manifest_sha256 = Get-FileSha256 -Path $archivedManifest
    current_platform_proof = $archivedProof
    current_image_metadata = $archivedMetadata
    current_project = $ProductionProject
    previous_manifest = if ($null -eq $previousState) { '' } else { [string]$previousState.current_manifest }
    previous_manifest_sha256 = if ($null -eq $previousState) { '' } else { [string]$previousState.current_manifest_sha256 }
    previous_platform_proof = if ($null -eq $previousState) { '' } else { [string]$previousState.current_platform_proof }
    previous_image_metadata = if ($null -eq $previousState) { '' } else { [string]$previousState.current_image_metadata }
    previous_project = if ($null -eq $previousState) { '' } else { [string]$previousState.current_project }
  }
  [IO.Directory]::CreateDirectory((Split-Path -Parent $statePath)) | Out-Null
  $temporaryPath = Join-Path (Split-Path -Parent $statePath) ('.deployment-state.' + [guid]::NewGuid().ToString('N') + '.tmp')
  try {
    [IO.File]::WriteAllText($temporaryPath, ($state | ConvertTo-Json -Depth 5) + "`n", [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporaryPath -Destination $statePath -Force
  } finally {
    if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force }
  }
} finally {
  foreach ($name in $environmentNames) { [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process') }
}

Write-Output 'Admin-only immutable release deployed'
