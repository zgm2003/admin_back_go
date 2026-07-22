[CmdletBinding()]
param(
  [switch]$NoBrowser,
  [ValidateRange(10, 600)][int]$ReadinessTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest

if ($PSVersionTable.PSVersion.Major -ne 7) {
  throw 'ADMIN_DEV_POWERSHELL_7_REQUIRED'
}

$backendRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$frontendRoot = [IO.Path]::GetFullPath((Join-Path $backendRoot '..\admin_front_ts'))
$platformScript = Join-Path $PSScriptRoot 'docker-platform.ps1'
$commonScript = Join-Path $PSScriptRoot 'dev\admin-dev-common.ps1'
$environmentPath = Join-Path $backendRoot 'deploy\docker-first\admin-go.env'
$lockPath = Join-Path $backendRoot '.tmp\dev\admin-dev.lock.json'
$dependencyStampPath = Join-Path $backendRoot '.tmp\dev\frontend-dependencies.json'

. $commonScript

$lockHandle = $null
$states = [Collections.Generic.List[object]]::new()
$sensitiveValues = @()
try {
  Assert-AdminDevPrimaryRepositories -BackendRoot $backendRoot -FrontendRoot $frontendRoot
  $lockHandle = Enter-AdminDevLock -Path $lockPath -RepositoryRoot $backendRoot

  & $platformScript -Action dev-state

  $tools = Resolve-AdminDevHostTools
  $requiredEnvironmentKeys = @(
    'APP_ENV',
    'HTTP_ADDR',
    'HTTP_READ_HEADER_TIMEOUT',
    'LOG_DIR',
    'MYSQL_DSN',
    'MYSQL_MAX_OPEN_CONNS',
    'MYSQL_MAX_IDLE_CONNS',
    'MYSQL_CONN_MAX_LIFETIME',
    'REDIS_ADDR',
    'REDIS_PASSWORD',
    'REDIS_DB',
    'APP_SECRET',
    'TOKEN_REDIS_DB',
    'PAYMENT_CERT_BASE_DIR',
    'QUEUE_ENABLED',
    'QUEUE_REDIS_DB',
    'QUEUE_CONCURRENCY',
    'REALTIME_ENABLED',
    'REALTIME_PUBLISHER',
    'SCHEDULER_ENABLED',
    'CORS_ALLOW_ORIGINS'
  )
  $containerEnvironment = Read-AdminDevEnvironmentFile `
    -Path $environmentPath `
    -RequiredKeys $requiredEnvironmentKeys `
    -AllowEmptyKeys @('REDIS_PASSWORD')
  $sensitiveValues = Get-AdminDevSensitiveValues -Environment $containerEnvironment
  $runtimeEnvironment = ConvertTo-AdminDevHostEnvironment `
    -Environment $containerEnvironment `
    -RepositoryRoot $backendRoot

  [IO.Directory]::CreateDirectory($runtimeEnvironment['LOG_DIR']) | Out-Null
  [IO.Directory]::CreateDirectory((Join-Path $runtimeEnvironment['PAYMENT_CERT_BASE_DIR'] 'exports')) | Out-Null

  $airExecutable = Install-AdminDevAir -RepositoryRoot $backendRoot -GoExecutable $tools.GoExecutable
  $dependenciesChanged = Initialize-AdminDevFrontendDependencies `
    -FrontendRoot $frontendRoot `
    -NpmExecutable $tools.NpmExecutable `
    -StampPath $dependencyStampPath
  if ($dependenciesChanged) {
    Write-Host '[WEB] frontend dependencies synchronized from package-lock.json'
  }

  Assert-AdminDevPortsAvailable -Ports @(5173, 8080)

  $runtimeEnvironment['CGO_ENABLED'] = '0'
  $runtimeEnvironment['GOTOOLCHAIN'] = 'local'
  $runtimeEnvironment['GOWORK'] = 'off'
  $runtimeEnvironment['GOFLAGS'] = '-mod=readonly'
  $runtimeEnvironment['ZONEINFO'] = $tools.ZoneInfoPath
  $runtimeEnvironment['Path'] = @(
    (Split-Path $tools.GoExecutable -Parent),
    (Split-Path $tools.NodeExecutable -Parent),
    $env:Path
  ) -join [IO.Path]::PathSeparator

  $webEnvironment = @{
    NODE_ENV = 'development'
    Path = @((Split-Path $tools.NodeExecutable -Parent), $env:Path) -join [IO.Path]::PathSeparator
  }
  $viteEntrypoint = Join-Path $frontendRoot 'node_modules\vite\bin\vite.js'
  if (-not (Test-Path -LiteralPath $viteEntrypoint -PathType Leaf)) {
    throw 'ADMIN_DEV_VITE_ENTRYPOINT_MISSING'
  }

  $states.Add((Start-AdminDevManagedProcess `
    -Name 'api' `
    -Prefix '[API]' `
    -FilePath $airExecutable `
    -ArgumentList @('-c', '.air.api.toml') `
    -WorkingDirectory $backendRoot `
    -Environment $runtimeEnvironment `
    -SensitiveValues $sensitiveValues))
  $states.Add((Start-AdminDevManagedProcess `
    -Name 'worker' `
    -Prefix '[WORKER]' `
    -FilePath $airExecutable `
    -ArgumentList @('-c', '.air.worker.toml') `
    -WorkingDirectory $backendRoot `
    -Environment $runtimeEnvironment `
    -SensitiveValues $sensitiveValues))
  $states.Add((Start-AdminDevManagedProcess `
    -Name 'web' `
    -Prefix '[WEB]' `
    -FilePath $tools.NodeExecutable `
    -ArgumentList @($viteEntrypoint, '--host', '::', '--port', '5173', '--strictPort') `
    -WorkingDirectory $frontendRoot `
    -Environment $webEnvironment `
    -SensitiveValues @()))

  Wait-AdminDevRuntimeReady -States $states.ToArray() -TimeoutSeconds $ReadinessTimeoutSeconds
  Write-Host '[WEB] Vite is ready at http://127.0.0.1:5173'
  Write-Host '[API] API health and readiness checks passed at http://127.0.0.1:8080/health and http://127.0.0.1:8080/ready'
  Write-Host '[WORKER] worker child process is stable'
  if (-not $NoBrowser) {
    $browserStartInfo = [Diagnostics.ProcessStartInfo]::new()
    $browserStartInfo.FileName = 'http://localhost:5173'
    $browserStartInfo.UseShellExecute = $true
    [Diagnostics.Process]::Start($browserStartInfo) | Out-Null
  }
  Write-Host 'admin-dev is running; press Ctrl+C to stop host processes and keep MySQL/Redis running'
  Watch-AdminDevManagedProcesses -States $states.ToArray()
}
finally {
  Stop-AdminDevManagedProcesses -States $states.ToArray()
  Exit-AdminDevLock -Handle $lockHandle
}
