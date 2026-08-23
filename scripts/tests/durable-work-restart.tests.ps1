[CmdletBinding()]
param(
  [string]$GoImage = 'docker.m.daocloud.io/library/golang:1.26.5-bookworm',
  [string]$MySQLImage = 'mysql:8.4.10',
  [string]$RedisImage = 'redis:8.2.7-alpine',
  [string]$PythonImage = 'python:3.13-slim'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Resolve-DockerExecutable {
  $preferred = 'E:\Docker\Docker\resources\bin\docker.exe'
  if (Test-Path -LiteralPath $preferred -PathType Leaf) { return $preferred }
  $command = Get-Command docker.exe -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $command) { $command = Get-Command docker -ErrorAction Stop | Select-Object -First 1 }
  return $command.Source
}

function Invoke-Docker {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  $output = @(& $script:Docker @Arguments 2>&1 | ForEach-Object { $_.ToString() })
  if ($LASTEXITCODE -ne 0) {
    $output | Write-Output
    throw "Docker command failed: $($Arguments[0])"
  }
  return $output
}

function Wait-ForCondition {
  param(
    [Parameter(Mandatory = $true)][scriptblock]$Probe,
    [int]$TimeoutSeconds = 60,
    [Parameter(Mandatory = $true)][string]$FailureMessage
  )
  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
  while ([DateTime]::UtcNow -lt $deadline) {
    try {
      if (& $Probe) { return }
    }
    catch { }
    Start-Sleep -Milliseconds 250
  }
  throw $FailureMessage
}

function New-HexSecret {
  param([int]$Bytes = 48)
  $buffer = New-Object byte[] $Bytes
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try { $rng.GetBytes($buffer) } finally { $rng.Dispose() }
  return ([Convert]::ToHexString($buffer)).ToLowerInvariant()
}

function Invoke-MySQLScalar {
  param([Parameter(Mandatory = $true)][string]$SQL)
  $arguments = @(
    'exec', '-e', "MYSQL_PWD=$script:MySQLPassword", $script:MySQLContainer,
    'mysql', '--batch', '--skip-column-names', '--raw', '--user=root', '--database=admin', "--execute=$SQL"
  )
  $output = Invoke-Docker -Arguments $arguments
  $last = $output | Select-Object -Last 1
  if ($null -eq $last) { return '' }
  return ([string]$last).Trim()
}

function Invoke-Fixture {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  $dockerArguments = @(
    'run', '--rm', '--network', $script:Network,
    '--env-file', $script:RuntimeEnv,
    '--mount', ('type=bind,source=' + $script:RepoRoot + ',target=/src'),
    '--mount', 'type=volume,source=admin-go-mod-cache,target=/go/pkg/mod',
    '--mount', 'type=volume,source=admin-go-build-cache,target=/root/.cache/go-build',
    '--workdir', '/src', $script:GoImage,
    'go', 'run', './scripts/tests/durableworkfixture'
  ) + $Arguments
  $output = Invoke-Docker -Arguments $dockerArguments
  $line = $output | Where-Object { $_ -match '^P05_RESULT ' } | Select-Object -Last 1
  if ([string]::IsNullOrWhiteSpace($line)) { throw 'durable work fixture did not return a result' }
  return ($line.Substring('P05_RESULT '.Length) | ConvertFrom-Json)
}

function Get-CommandState {
  param([Parameter(Mandatory = $true)][UInt64]$CommandID)
  return Invoke-MySQLScalar -SQL "SELECT state FROM ai_reply_commands WHERE id=$CommandID"
}

function Wait-ForCommandState {
  param(
    [Parameter(Mandatory = $true)][UInt64]$CommandID,
    [Parameter(Mandatory = $true)][string]$State,
    [int]$TimeoutSeconds = 60
  )
  Wait-ForCondition -TimeoutSeconds $TimeoutSeconds -FailureMessage "command $CommandID did not reach $State" -Probe {
    (Get-CommandState -CommandID $CommandID) -ceq $State
  }
}

function Start-RuntimeContainer {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][ValidateSet('admin-api','admin-worker')][string]$Process
  )
  Invoke-Docker -Arguments @(
    'run', '--detach', '--name', $Name, '--network', $script:Network,
    '--env-file', $script:RuntimeEnv,
    '--entrypoint', ('/app/' + $Process), $script:RuntimeImage
  ) | Out-Null
}

function Assert-CommandCardinality {
  param(
    [Parameter(Mandatory = $true)][UInt64]$CommandID,
    [Parameter(Mandatory = $true)][string]$State,
    [Parameter(Mandatory = $true)][int]$AssistantCount,
    [Parameter(Mandatory = $true)][int]$EventCount,
    [Parameter(Mandatory = $true)][int]$AttemptCount,
    [UInt64]$MinimumLeaseToken = 1
  )
  $status = Invoke-Fixture -Arguments @('-mode','status','-command-id',[string]$CommandID)
  if ([string]$status.state -cne $State -or
      [int]$status.assistant_count -ne $AssistantCount -or
      [int]$status.event_count -ne $EventCount -or
      [int]$status.attempt_count -ne $AttemptCount -or
      [UInt64]$status.lease_token -lt $MinimumLeaseToken) {
    throw "command cardinality mismatch: $($status | ConvertTo-Json -Compress)"
  }
}

$script:Docker = Resolve-DockerExecutable
$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$script:GoImage = $GoImage
$suffix = "$PID-$([Guid]::NewGuid().ToString('N').Substring(0,8))"
$script:Network = "admin-p05-$suffix"
$script:MySQLContainer = "admin-p05-mysql-$suffix"
$redisContainer = "admin-p05-redis-$suffix"
$providerContainer = "admin-p05-provider-$suffix"
$apiContainer = "admin-p05-api-$suffix"
$workerA = "admin-p05-worker-a-$suffix"
$workerKilled = "admin-p05-worker-killed-$suffix"
$workerB = "admin-p05-worker-b-$suffix"
$lockContainer = "admin-p05-attempt-lock-$suffix"
$script:RuntimeImage = "admin-go-p05:$suffix"
$containers = @($apiContainer, $workerA, $workerKilled, $workerB, $lockContainer, $providerContainer, $redisContainer, $script:MySQLContainer)

$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$tempDirectory = [IO.Path]::GetFullPath((Join-Path $tempRoot ("admin-p05-durable-$suffix")))
if (-not $tempDirectory.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or
    -not ([IO.Path]::GetFileName($tempDirectory)).StartsWith('admin-p05-durable-', [StringComparison]::Ordinal)) {
  throw 'refusing an unverified durable-work temporary directory'
}
[IO.Directory]::CreateDirectory($tempDirectory) | Out-Null
$script:RuntimeEnv = Join-Path $tempDirectory 'runtime.env'
$providerScript = Join-Path $tempDirectory 'provider.py'
$script:MySQLPassword = New-HexSecret -Bytes 24
$appSecret = New-HexSecret -Bytes 64

$providerSource = @'
import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

state = {"requests": 0, "paused": 0}
lock = threading.Lock()
release = threading.Event()

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):
        return

    def do_GET(self):
        if self.path != "/state":
            self.send_error(404)
            return
        with lock:
            body = json.dumps(state).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path == "/release":
            release.set()
            self.send_response(204)
            self.end_headers()
            return
        if self.path != "/v1/chat/completions":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        document = json.loads(raw or b"{}")
        pause = "P05_PAUSE" in json.dumps(document)
        with lock:
            state["requests"] += 1
            if pause:
                state["paused"] += 1
        if pause:
            release.wait(240)
        chunks = [
            b'data: {"choices":[{"delta":{"content":"durable answer"}}]}\n\n',
            b'data: {"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}\n\n',
            b'data: [DONE]\n\n',
        ]
        try:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("X-Request-Id", "p05-provider-request")
            self.send_header("Connection", "close")
            self.end_headers()
            for chunk in chunks:
                self.wfile.write(chunk)
                self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass
        self.close_connection = True

ThreadingHTTPServer(("0.0.0.0", 18080), Handler).serve_forever()
'@
[IO.File]::WriteAllText($providerScript, $providerSource, [Text.UTF8Encoding]::new($false))

try {
  Invoke-Docker -Arguments @('network','create',$script:Network) | Out-Null
  Invoke-Docker -Arguments @(
    'run','--detach','--name',$script:MySQLContainer,'--network',$script:Network,'--network-alias','mysql',
    '--env',("MYSQL_ROOT_PASSWORD=" + $script:MySQLPassword),'--env','MYSQL_DATABASE=admin',$MySQLImage
  ) | Out-Null
  Invoke-Docker -Arguments @(
    'run','--detach','--name',$redisContainer,'--network',$script:Network,'--network-alias','redis',$RedisImage
  ) | Out-Null

  Wait-ForCondition -TimeoutSeconds 180 -FailureMessage 'P05 MySQL did not become ready' -Probe {
    $logs = @(& $script:Docker logs $script:MySQLContainer 2>&1 | ForEach-Object { $_.ToString() })
    $LASTEXITCODE -eq 0 -and (($logs -join "`n") -match 'MySQL init process done')
  }
  Wait-ForCondition -TimeoutSeconds 120 -FailureMessage 'P05 MySQL schema was not ready after initialization' -Probe {
    (Invoke-MySQLScalar -SQL 'SELECT 1') -eq '1'
  }
  Wait-ForCondition -TimeoutSeconds 30 -FailureMessage 'P05 Redis did not become ready' -Probe {
    & $script:Docker exec $redisContainer redis-cli ping 2>$null | Out-Null
    $LASTEXITCODE -eq 0
  }

  $runtimeLines = @(
    'APP_ENV=production',
    'HTTP_ADDR=:8080',
    'LOG_DIR=/tmp/admin-p05-logs',
    "MYSQL_DSN=root:$script:MySQLPassword@tcp(mysql:3306)/admin?charset=utf8mb4&parseTime=True&loc=UTC",
    'MYSQL_MAX_OPEN_CONNS=20',
    'MYSQL_MAX_IDLE_CONNS=10',
    'MYSQL_CONN_MAX_LIFETIME=1h',
    'REDIS_ADDR=redis:6379',
    'REDIS_PASSWORD=',
    'REDIS_DB=11',
    'TOKEN_REDIS_DB=12',
    'TOKEN_REDIS_PREFIX=p05:',
    'QUEUE_ENABLED=true',
    'QUEUE_REDIS_DB=13',
    'QUEUE_CONCURRENCY=4',
    'REALTIME_ENABLED=true',
    'REALTIME_PUBLISHER=redis',
    'SCHEDULER_ENABLED=false',
    'CORS_ALLOW_ORIGINS=https://admin.example.com',
    "APP_SECRET=$appSecret",
    "TEST_MYSQL_DSN=root:$script:MySQLPassword@tcp(mysql:3306)/admin?charset=utf8mb4&parseTime=True&loc=UTC",
    'TEST_REDIS_ADDR=redis:6379',
    'ADMIN_DURABLE_WORK_INTEGRATION=1'
  )
  [IO.File]::WriteAllText($script:RuntimeEnv, (($runtimeLines -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))

  Invoke-Docker -Arguments @(
    'run','--detach','--name',$providerContainer,'--network',$script:Network,'--network-alias','provider',
    '--mount',('type=bind,source=' + $providerScript + ',target=/app/provider.py,readonly'),
    $PythonImage,'python','/app/provider.py'
  ) | Out-Null
  Wait-ForCondition -TimeoutSeconds 30 -FailureMessage 'fake provider did not become ready' -Probe {
    & $script:Docker exec $providerContainer python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:18080/state',timeout=1).read()" *> $null
    $LASTEXITCODE -eq 0
  }

  $integrationArguments = @(
    'run','--rm','--network',$script:Network,'--env-file',$script:RuntimeEnv,
    '--mount',('type=bind,source=' + $script:RepoRoot + ',target=/src'),
    '--mount','type=volume,source=admin-go-mod-cache,target=/go/pkg/mod',
    '--mount','type=volume,source=admin-go-race-buildcache,target=/root/.cache/go-build',
    '--workdir','/src',$GoImage,
    'go','test','-race','-count=1',
    './internal/module/ai/replycommand','./internal/module/realtime',
    './internal/module/notification/task','./internal/module/export','./internal/infra/realtime'
  )
  Invoke-Docker -Arguments $integrationArguments | Write-Output

  $revision = (git -C $script:RepoRoot rev-parse HEAD).Trim()
  Invoke-Docker -Arguments @(
    'build','--target','runtime','--tag',$script:RuntimeImage,
    '--build-arg',("BUILD_REVISION=" + $revision),$script:RepoRoot
  ) | Out-Null

  Start-RuntimeContainer -Name $apiContainer -Process 'admin-api'
  try {
    Wait-ForCondition -TimeoutSeconds 45 -FailureMessage 'isolated admin-api did not become ready' -Probe {
      & $script:Docker exec $apiContainer curl -fsS http://127.0.0.1:8080/ready *> $null
      $LASTEXITCODE -eq 0
    }
  }
  catch {
    $state = @(& $script:Docker inspect --format '{{.State.Status}}/{{.State.ExitCode}}' $apiContainer 2>&1) -join "`n"
    $logs = @(& $script:Docker logs $apiContainer 2>&1) -join "`n"
    foreach ($secret in @($script:MySQLPassword, $appSecret)) {
      if (-not [string]::IsNullOrEmpty($secret)) {
        $state = $state.Replace($secret, '<redacted>')
        $logs = $logs.Replace($secret, '<redacted>')
      }
    }
    Write-Warning "isolated admin-api state=$state`n$logs"
    throw
  }

  $fixture = Invoke-Fixture -Arguments @('-mode','setup','-provider-url','http://provider:18080/v1')
  $userID = [Int64]$fixture.user_id
  $conversationID = [Int64]$fixture.conversation_id

  $request1 = "p05-api-restart-$suffix"
  $created1 = Invoke-Fixture -Arguments @('-mode','create','-user-id',[string]$userID,'-conversation-id',[string]$conversationID,'-request-id',$request1,'-content','API committed before termination')
  $command1 = [UInt64]$created1.command_id
  Invoke-Docker -Arguments @('kill','--signal','KILL',$apiContainer) | Out-Null

  Start-RuntimeContainer -Name $workerA -Process 'admin-worker'
  Wait-ForCommandState -CommandID $command1 -State 'succeeded' -TimeoutSeconds 90
  Assert-CommandCardinality -CommandID $command1 -State 'succeeded' -AssistantCount 1 -EventCount 1 -AttemptCount 1
  Invoke-Docker -Arguments @('stop','--time','15',$workerA) | Out-Null

  $request2 = "p05-lease-recovery-$suffix"
  $created2 = Invoke-Fixture -Arguments @('-mode','create','-user-id',[string]$userID,'-conversation-id',[string]$conversationID,'-request-id',$request2,'-content','recover after lease expiry')
  $command2 = [UInt64]$created2.command_id
  Invoke-Docker -Arguments @(
    'run','--detach','--name',$lockContainer,'--network',$script:Network,
    '--env',("MYSQL_PWD=" + $script:MySQLPassword),$MySQLImage,
    'mysql','--host=mysql','--user=root','--database=admin',
    '--execute=LOCK TABLES ai_provider_attempts WRITE; SELECT SLEEP(90); UNLOCK TABLES;'
  ) | Out-Null
  Start-Sleep -Seconds 1
  Start-RuntimeContainer -Name $workerKilled -Process 'admin-worker'
  Wait-ForCommandState -CommandID $command2 -State 'running' -TimeoutSeconds 45
  Invoke-Docker -Arguments @('kill','--signal','KILL',$workerKilled) | Out-Null
  Invoke-Docker -Arguments @('rm','--force',$lockContainer) | Out-Null

  Wait-ForCondition -TimeoutSeconds 45 -FailureMessage 'killed Worker lease did not expire' -Probe {
    $expired = Invoke-MySQLScalar -SQL "SELECT lease_expires_at < UTC_TIMESTAMP(6) FROM ai_reply_commands WHERE id=$command2"
    $expired -eq '1'
  }
  Start-RuntimeContainer -Name $workerB -Process 'admin-worker'
  Wait-ForCommandState -CommandID $command2 -State 'succeeded' -TimeoutSeconds 90
  Assert-CommandCardinality -CommandID $command2 -State 'succeeded' -AssistantCount 1 -EventCount 1 -AttemptCount 1 -MinimumLeaseToken 2

  $request3 = "p05-cross-node-cancel-$suffix"
  $created3 = Invoke-Fixture -Arguments @('-mode','create','-user-id',[string]$userID,'-conversation-id',[string]$conversationID,'-request-id',$request3,'-content','P05_PAUSE cross node cancellation')
  $command3 = [UInt64]$created3.command_id
  Wait-ForCommandState -CommandID $command3 -State 'running' -TimeoutSeconds 45
  Wait-ForCondition -TimeoutSeconds 45 -FailureMessage 'fake provider did not reach the cancellation pause point' -Probe {
    $paused = Invoke-Docker -Arguments @('exec',$providerContainer,'python','-c',"import json,urllib.request; print(json.load(urllib.request.urlopen('http://127.0.0.1:18080/state'))['paused'])")
    [int](($paused | Select-Object -Last 1).Trim()) -ge 1
  }
  Invoke-Fixture -Arguments @(
    '-mode','cancel','-command-id',[string]$command3,'-user-id',[string]$userID,
    '-conversation-id',[string]$conversationID,'-request-id',$request3
  ) | Out-Null
  Wait-ForCommandState -CommandID $command3 -State 'canceled' -TimeoutSeconds 45
  Assert-CommandCardinality -CommandID $command3 -State 'canceled' -AssistantCount 1 -EventCount 1 -AttemptCount 1

  $replay = Invoke-Fixture -Arguments @('-mode','resume','-user-id',[string]$userID,'-after-sequence','0')
  if ([int]$replay.count -ne 3 -or [bool]$replay.resync_required) {
    throw "durable cursor replay mismatch: $($replay | ConvertTo-Json -Compress)"
  }
  $afterReplay = Invoke-Fixture -Arguments @('-mode','resume','-user-id',[string]$userID,'-after-sequence',[string]$replay.latest_sequence)
  if ([int]$afterReplay.count -ne 0 -or [bool]$afterReplay.resync_required) {
    throw "durable cursor duplicate replay: $($afterReplay | ConvertTo-Json -Compress)"
  }

  $duplicateAssistant = Invoke-MySQLScalar -SQL 'SELECT COUNT(*) FROM (SELECT reply_command_id FROM ai_messages WHERE reply_command_id IS NOT NULL GROUP BY reply_command_id HAVING COUNT(*)>1) d'
  $duplicateEvents = Invoke-MySQLScalar -SQL 'SELECT COUNT(*) FROM (SELECT event_id FROM realtime_events GROUP BY event_id HAVING COUNT(*)>1) d'
  $duplicateAttempts = Invoke-MySQLScalar -SQL 'SELECT COUNT(*) FROM (SELECT command_id,attempt_no FROM ai_provider_attempts GROUP BY command_id,attempt_no HAVING COUNT(*)>1) d'
  if ($duplicateAssistant -ne '0' -or $duplicateEvents -ne '0' -or $duplicateAttempts -ne '0') {
    throw "duplicate durable result detected: assistant=$duplicateAssistant events=$duplicateEvents attempts=$duplicateAttempts"
  }

  Write-Output 'durable work kill/restart, cross-node cancel, and cursor resume tests passed'
}
finally {
  $appSecret = $null
  $script:MySQLPassword = $null
  foreach ($container in $containers) {
    & $script:Docker rm --force $container *> $null
  }
  & $script:Docker image rm --force $script:RuntimeImage *> $null
  & $script:Docker network rm $script:Network *> $null
  if (Test-Path -LiteralPath $tempDirectory -PathType Container) {
    $resolved = [IO.Path]::GetFullPath($tempDirectory)
    if (-not $resolved.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or
        -not ([IO.Path]::GetFileName($resolved)).StartsWith('admin-p05-durable-', [StringComparison]::Ordinal)) {
      throw 'refusing to delete an unverified durable-work temporary directory'
    }
    [IO.Directory]::Delete($resolved, $true)
  }
}
