[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
  $PSNativeCommandUseErrorActionPreference = $false
}
if ($PSVersionTable.PSVersion.Major -ne 7) {
  throw 'session secret rotation verification requires PowerShell 7'
}

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$runtimeEnv = Join-Path $repoRoot 'deploy\docker-first\admin-go.env'
$platformScript = Join-Path $repoRoot 'scripts\docker-platform.ps1'
$adminDevScript = Join-Path $repoRoot 'scripts\admin-dev.ps1'
$adminDevCommonScript = Join-Path $repoRoot 'scripts\dev\admin-dev-common.ps1'
$defaultDocker = 'E:\Docker\Docker\resources\bin\docker.exe'
$docker = if (Test-Path -LiteralPath $defaultDocker -PathType Leaf) {
  $defaultDocker
}
else {
  (Get-Command docker.exe -ErrorAction Stop | Select-Object -First 1).Source
}

. $adminDevCommonScript
. (Join-Path $repoRoot 'scripts\database\atlas-runtime-common.ps1')

if (-not (Test-Path -LiteralPath $runtimeEnv -PathType Leaf)) {
  throw 'Docker runtime env is missing; run scripts/docker-platform.ps1 init first.'
}

function Assert-NoAdminDevDatabaseMutation {
  param([Parameter(Mandatory = $true)][string[]]$Paths)

  foreach ($path in $Paths) {
    $tokens = $null
    $parseErrors = $null
    $sourceAst = [Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -ne 0) {
      throw 'admin-dev startup source has PowerShell parse errors'
    }
    $commands = $sourceAst.FindAll({
      param($node)
      $node -is [Management.Automation.Language.CommandAst]
    }, $true)
    foreach ($command in $commands) {
      $commandName = [string]$command.GetCommandName()
      $commandLeaf = if ([string]::IsNullOrWhiteSpace($commandName)) {
        ''
      }
      else {
        [IO.Path]::GetFileNameWithoutExtension($commandName).ToLowerInvariant()
      }
      $literalValues = @($command.FindAll({
        param($node)
        $node -is [Management.Automation.Language.StringConstantExpressionAst] -or
          $node -is [Management.Automation.Language.ExpandableStringExpressionAst]
      }, $true) | ForEach-Object { ([string]$_.Value).Trim().ToLowerInvariant() })
      $directMutator = $commandLeaf -in @('invoke-lockedatlasmigration', 'invoke-maildiagnosticrekey')
      $mutationCapable = [string]::IsNullOrWhiteSpace($commandName) -or $commandLeaf -in @(
        'go',
        'docker',
        'atlas',
        'admin-db',
        'invoke-atlascontainer',
        'pwsh',
        'powershell'
      )
      $hasRekey = @($literalValues | Where-Object { $_ -ceq 'mail-diagnostic-rekey' }).Count -gt 0
      $hasMigrate = @($literalValues | Where-Object { $_ -ceq 'migrate' }).Count -gt 0
      $hasMutationVerb = @($literalValues | Where-Object { $_ -in @('apply', 'set') }).Count -gt 0
      if ($directMutator -or ($mutationCapable -and ($hasRekey -or ($hasMigrate -and $hasMutationVerb)))) {
        throw 'admin-dev must not perform startup migration or rekey'
      }
    }
  }
}

Assert-NoAdminDevDatabaseMutation -Paths @($adminDevScript, $adminDevCommonScript)

function New-RotationSecret {
  $bytes = New-Object byte[] 48
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $rng.GetBytes($bytes)
  }
  finally {
    $rng.Dispose()
  }
  return [Convert]::ToBase64String($bytes)
}

function Test-PathInsideRoot {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Root
  )

  $comparison = if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    [StringComparison]::OrdinalIgnoreCase
  }
  else {
    [StringComparison]::Ordinal
  }
  $separators = [char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
  $prefix = $Root.TrimEnd($separators) + [IO.Path]::DirectorySeparatorChar
  return $Path.StartsWith($prefix, $comparison)
}

function Invoke-BoundedProcessCapture {
  param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [Parameter(Mandatory = $true)][string]$Operation,
    [Parameter(Mandatory = $true)][ValidateRange(1, 3600)][int]$TimeoutSeconds,
    [Parameter(Mandatory = $true)][string]$WorkingDirectory
  )

  foreach ($argument in $Arguments) {
    foreach ($sensitiveValue in @($script:SensitiveValues)) {
      if (-not [string]::IsNullOrEmpty([string]$sensitiveValue) -and
          ([string]$argument).Contains([string]$sensitiveValue, [StringComparison]::Ordinal)) {
        throw 'rotation command arguments contain a sensitive runtime value'
      }
    }
  }

  $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
  $startInfo.FileName = $Executable
  $startInfo.WorkingDirectory = $WorkingDirectory
  $startInfo.UseShellExecute = $false
  $startInfo.CreateNoWindow = $true
  $startInfo.RedirectStandardOutput = $true
  $startInfo.RedirectStandardError = $true
  $startInfo.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
  $startInfo.StandardErrorEncoding = [Text.UTF8Encoding]::new($false)
  foreach ($argument in $Arguments) {
    [void]$startInfo.ArgumentList.Add([string]$argument)
  }

  $process = [System.Diagnostics.Process]::new()
  $process.StartInfo = $startInfo
  try {
    try {
      if (-not $process.Start()) { throw 'start returned false' }
    }
    catch {
      throw "$Operation failed to start"
    }
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
      try { $process.Kill($true) } catch { }
      try { [void]$process.WaitForExit(5000) } catch { }
      throw "$Operation timed out"
    }
    try {
      $stdout = $stdoutTask.GetAwaiter().GetResult()
      $stderr = $stderrTask.GetAwaiter().GetResult()
    }
    catch {
      throw "$Operation output capture failed"
    }
    $text = if ([string]::IsNullOrEmpty($stdout)) {
      $stderr
    }
    elseif ([string]::IsNullOrEmpty($stderr)) {
      $stdout
    }
    else {
      $stdout.TrimEnd([char[]]"`r`n") + "`n" + $stderr
    }
    Assert-NoSensitiveOutput -Text $text
    $stdoutLines = if ([string]::IsNullOrEmpty($stdout)) {
      @()
    }
    else {
      @($stdout.TrimEnd([char[]]"`r`n") -split "`r?`n")
    }
    $stderrLines = if ([string]::IsNullOrEmpty($stderr)) {
      @()
    }
    else {
      @($stderr.TrimEnd([char[]]"`r`n") -split "`r?`n")
    }
    return [pscustomobject]@{
      ExitCode = [int]$process.ExitCode
      StdOut = $stdout
      StdErr = $stderr
      StdOutLines = $stdoutLines
      StdErrLines = $stderrLines
      Lines = $stdoutLines
      Text = $text
    }
  }
  finally {
    $process.Dispose()
  }
}

function Invoke-GoCapture {
  param(
    [Parameter(Mandatory = $true)][string[]]$Arguments,
    [ValidateRange(1, 3600)][int]$TimeoutSeconds = 300,
    [string]$Operation = 'Go rotation command'
  )

  return Invoke-BoundedProcessCapture `
    -Executable $script:GoExecutable `
    -Arguments $Arguments `
    -Operation $Operation `
    -TimeoutSeconds $TimeoutSeconds `
    -WorkingDirectory $script:RepoRoot
}

function Assert-NoSensitiveOutput {
  param([AllowEmptyString()][string]$Text)

  foreach ($value in @($script:SensitiveValues)) {
    if (-not [string]::IsNullOrEmpty([string]$value) -and $Text.Contains([string]$value, [StringComparison]::Ordinal)) {
      throw 'rotation verification command output exposed a sensitive runtime value'
    }
  }
}

function Assert-SchemaFingerprintCapture {
  param(
    [Parameter(Mandatory = $true)]$Result,
    [Parameter(Mandatory = $true)][string]$ExpectedPath,
    [Parameter(Mandatory = $true)][string]$ExpectedSHA256
  )

  if ($Result.ExitCode -ne 0 -or
      -not [string]::IsNullOrWhiteSpace([string]$Result.StdErr) -or
      $Result.StdOutLines.Count -ne 2 -or
      [string]$Result.StdOutLines[0] -cne $ExpectedPath -or
      [string]$Result.StdOutLines[1] -cne $ExpectedSHA256) {
    throw 'schema fingerprint capture output was malformed'
  }
}

function Get-SchemaFingerprint {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )

  $fingerprintPath = Join-Path ([IO.Path]::GetTempPath()) ('admin-rotation-fingerprint-' + [guid]::NewGuid().ToString('N') + '.json')
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  $primaryFailure = $null
  $cleanupFailure = $null
  $fingerprint = $null
  try {
    $env:MYSQL_DSN = New-SchemaDSN -Settings $Settings -Database $Database
    $gitCommand = Get-Command 'git.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $gitCommand) {
      $gitCommand = Get-Command 'git' -ErrorAction Stop | Select-Object -First 1
    }
    $gitResult = Invoke-BoundedProcessCapture `
      -Executable $gitCommand.Source `
      -Arguments @('-C', $script:RepoRoot, 'rev-parse', 'HEAD') `
      -Operation 'Git commit resolution' `
      -TimeoutSeconds 30 `
      -WorkingDirectory $script:RepoRoot
    $commit = $gitResult.StdOut.Trim()
    if ($gitResult.ExitCode -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') {
      throw 'Git commit could not be resolved'
    }
    $gitResult = $null
    $fingerprintResult = Invoke-GoCapture `
      -Arguments @('run', './cmd/admin-db', 'fingerprint', '--schema', $Database, '--out', $fingerprintPath, '--commit', $commit) `
      -TimeoutSeconds 180 `
      -Operation 'schema fingerprint capture'
    if ($fingerprintResult.ExitCode -ne 0) {
      throw 'schema fingerprint capture failed'
    }
    $document = [IO.File]::ReadAllText($fingerprintPath, [Text.Encoding]::UTF8) | ConvertFrom-Json
    if ([string]$document.schema_sha256 -notmatch '^[0-9a-f]{64}$') {
      throw 'schema fingerprint output was invalid'
    }
    Assert-SchemaFingerprintCapture `
      -Result $fingerprintResult `
      -ExpectedPath $fingerprintPath `
      -ExpectedSHA256 ([string]$document.schema_sha256)
    $fingerprintResult = $null
    $fingerprint = [string]$document.schema_sha256
  }
  catch {
    $primaryFailure = $_
  }
  finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    try {
      if (Test-Path -LiteralPath $fingerprintPath -PathType Leaf) {
        Remove-Item -LiteralPath $fingerprintPath -Force -ErrorAction Stop
      }
    }
    catch {
      $cleanupFailure = 'schema fingerprint temporary file cleanup failed'
    }
  }
  if ($null -ne $primaryFailure) {
    if ($null -ne $cleanupFailure) {
      throw "$($primaryFailure.Exception.Message); cleanup also failed: $cleanupFailure"
    }
    throw $primaryFailure
  }
  if ($null -ne $cleanupFailure) {
    throw $cleanupFailure
  }
  return $fingerprint
}

function Invoke-DisposableSchemaBootstrap {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )

  if ($Database -notmatch '^admin_rekey_[0-9a-f]{12}$') {
    throw 'refusing to bootstrap an unexpected disposable schema'
  }

  $runtimeDirectory = ''
  $primaryFailure = $null
  $cleanupFailure = $null
  try {
    $runtimeDirectory = New-AtlasRuntimeConfig -Settings $Settings -Database $Database
    $schemaPath = Join-Path $script:RepoRoot 'database\schema\admin.hcl'
    if (-not (Test-Path -LiteralPath $schemaPath -PathType Leaf)) {
      throw 'canonical admin.hcl is missing'
    }
    $canonicalSchema = [IO.File]::ReadAllText($schemaPath, [Text.Encoding]::UTF8)
    if ([regex]::Matches($canonicalSchema, '(?m)^schema "admin" \{$').Count -ne 1) {
      throw 'canonical schema must contain exactly one admin schema declaration'
    }
    $runtimeSchema = $canonicalSchema.Replace('schema "admin" {', 'schema "' + $Database + '" {')
    $runtimeSchema = $runtimeSchema.Replace('schema.admin', "schema.$Database")
    if ([regex]::IsMatch($runtimeSchema, '\bschema\.admin\b')) {
      throw 'canonical schema reference rebinding was incomplete'
    }
    $runtimeSchemaPath = Join-Path $runtimeDirectory 'admin.hcl'
    [IO.File]::WriteAllText($runtimeSchemaPath, $runtimeSchema, [Text.UTF8Encoding]::new($false))
    $schemaApplyOutput = Invoke-AtlasContainer `
      -DockerExecutable $script:DockerExecutable `
      -BackendRoot $script:RepoRoot `
      -RuntimeDirectory $runtimeDirectory `
      -AtlasArguments @(
        'schema', 'apply',
        '--config', 'file:///runtime/atlas.hcl',
        '--env', 'runtime',
        '--to', 'file:///runtime/admin.hcl',
        '--auto-approve'
      ) `
      -TimeoutSeconds 600
    Assert-NoSensitiveOutput -Text $schemaApplyOutput
    $schemaApplyOutput = $null

  }
  catch {
    $primaryFailure = $_
  }
  finally {
    try {
      Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
    }
    catch {
      $cleanupFailure = 'disposable Atlas runtime config cleanup failed'
    }
  }
  if ($null -ne $primaryFailure) {
    if ($null -ne $cleanupFailure) {
      throw "$($primaryFailure.Exception.Message); cleanup also failed: $cleanupFailure"
    }
    throw $primaryFailure
  }
  if ($null -ne $cleanupFailure) {
    throw $cleanupFailure
  }
}

function Invoke-LockedAtlasMigration {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )

  $beforeFingerprint = Get-SchemaFingerprint -Settings $Settings -Database $Database
  $runtimeDirectory = ''
  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  $primaryFailure = $null
  $cleanupFailure = $null
  $migrationResult = $null
  try {
    $runtimeDirectory = New-AtlasRuntimeConfig -Settings $Settings -Database $Database
    $statusBefore = Invoke-AtlasContainer `
      -DockerExecutable $script:DockerExecutable `
      -BackendRoot $script:RepoRoot `
      -RuntimeDirectory $runtimeDirectory `
      -AtlasArguments @(
        'migrate', 'status',
        '--config', 'file:///runtime/atlas.hcl',
        '--env', 'runtime',
        '--dir', 'file:///workspace/database/migrations'
      ) `
      -TimeoutSeconds 600
    Assert-NoSensitiveOutput -Text $statusBefore
    $dockerArguments = @(
      'run', '--rm', '--add-host', 'host.docker.internal:host-gateway',
      '--volume', "${script:RepoRoot}:/workspace:ro",
      '--volume', "${runtimeDirectory}:/runtime:ro",
      '--workdir', '/workspace',
      $script:AtlasImage,
      'migrate', 'apply',
      '--config', 'file:///runtime/atlas.hcl',
      '--env', 'runtime',
      '--dir', 'file:///workspace/database/migrations',
      '--to-version', '202607230101'
    )
    $env:MYSQL_DSN = New-SchemaDSN -Settings $Settings -Database $Database
    $locked = Invoke-GoCapture `
      -Arguments (@(
        'run', './cmd/admin-db', 'lock-run',
        '--schema', $Database,
        '--name', 'admin:atlas:migrate',
        '--timeout', '30s',
        '--expected-fingerprint', $beforeFingerprint,
        '--'
      ) + @($script:DockerExecutable) + $dockerArguments) `
      -TimeoutSeconds 900 `
      -Operation 'locked Atlas migration'
    if ($locked.ExitCode -ne 0) { throw 'locked Atlas migration failed' }
    $locked = $null

    $statusText = Invoke-AtlasContainer `
      -DockerExecutable $script:DockerExecutable `
      -BackendRoot $script:RepoRoot `
      -RuntimeDirectory $runtimeDirectory `
      -AtlasArguments @(
        'migrate', 'status',
        '--config', 'file:///runtime/atlas.hcl',
        '--env', 'runtime',
        '--dir', 'file:///workspace/database/migrations'
      ) `
      -TimeoutSeconds 600
    Assert-NoSensitiveOutput -Text $statusText
    if ($statusText -notmatch '(?i)migration status:\s*ok') {
      throw 'Atlas migration status is not clean'
    }
    $statusText = $null
    $afterFingerprint = Get-SchemaFingerprint -Settings $Settings -Database $Database
    $migrationResult = [pscustomobject]@{ Status = 'ok'; Fingerprint = $afterFingerprint }
  }
  catch {
    $primaryFailure = $_
  }
  finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    try {
      Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
    }
    catch {
      $cleanupFailure = 'locked Atlas runtime config cleanup failed'
    }
  }
  if ($null -ne $primaryFailure) {
    if ($null -ne $cleanupFailure) {
      throw "$($primaryFailure.Exception.Message); cleanup also failed: $cleanupFailure"
    }
    throw $primaryFailure
  }
  if ($null -ne $cleanupFailure) {
    throw $cleanupFailure
  }
  return $migrationResult
}

function Invoke-InvariantGate {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$RelativePath,
    [Parameter(Mandatory = $true)][string[]]$ExpectedNames
  )

  $result = Invoke-GoCapture `
    -Arguments @('run', './cmd/admin-db', 'invariants', '--schema', $Database, '--file', $RelativePath) `
    -TimeoutSeconds 180 `
    -Operation 'database reconciliation invariant command'
  if ($result.ExitCode -ne 0 -or $result.Lines.Count -ne $ExpectedNames.Count -or
      -not [string]::IsNullOrWhiteSpace([string]$result.StdErr)) {
    throw 'database reconciliation invariant command failed'
  }
  $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  for ($index = 0; $index -lt $ExpectedNames.Count; $index++) {
    $line = $result.Lines[$index]
    $parts = [string]$line -split "`t", 2
    if ($parts.Count -ne 2 -or $parts[0] -cne $ExpectedNames[$index] -or
        -not $seen.Add($parts[0]) -or $parts[1] -notmatch '^[0-9]+$' -or [uint64]$parts[1] -ne 0) {
      throw 'database reconciliation invariant was non-zero or malformed'
    }
  }
  return $result.Lines.Count
}

function Invoke-MailDiagnosticRekey {
  param(
    [Parameter(Mandatory = $true)][bool]$ExpectSuccess,
    [uint64]$ExpectedScanned = 0,
    [uint64]$ExpectedRekeyed = 0
  )

  $result = Invoke-GoCapture `
    -Arguments @('run', './cmd/admin-db', 'mail-diagnostic-rekey') `
    -TimeoutSeconds 300 `
    -Operation 'mail diagnostic rekey command'
  if (-not $ExpectSuccess) {
    if ($result.ExitCode -eq 0) { throw 'mail diagnostic rekey unexpectedly succeeded' }
    $failureLines = @($result.StdErrLines | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) })
    if (-not [string]::IsNullOrWhiteSpace([string]$result.StdOut) -or
        $failureLines.Count -ne 2 -or
        [string]$failureLines[0] -cne 'mail diagnostic rekey command: failed' -or
        [string]$failureLines[1] -cne 'exit status 1') {
      throw 'mail diagnostic rekey failure output violated the safe sentinel contract'
    }
    return [pscustomobject]@{ ExitCode = $result.ExitCode; Scanned = 0; Rekeyed = 0; PreviousReferences = 0; UnknownReferences = 0 }
  }
  if ($result.ExitCode -ne 0 -or -not [string]::IsNullOrWhiteSpace([string]$result.StdErr)) {
    throw 'mail diagnostic rekey failed'
  }

  $values = @{}
  $rekeyedRowIDs = [Collections.Generic.List[uint64]]::new()
  $uniqueRekeyedRowIDs = [Collections.Generic.HashSet[uint64]]::new()
  foreach ($line in $result.Lines) {
    $parts = [string]$line -split "`t", 2
    if ($parts.Count -ne 2) { throw 'mail diagnostic rekey output was malformed' }
    if ($parts[0] -ceq 'rekeyed_row_id') {
      if ($parts[1] -notmatch '^[1-9][0-9]*$') { throw 'mail diagnostic rekey row id was malformed' }
      $rowID = [uint64]$parts[1]
      if (-not $uniqueRekeyedRowIDs.Add($rowID)) { throw 'mail diagnostic rekey repeated a row id' }
      $rekeyedRowIDs.Add($rowID)
      continue
    }
    if ($parts[0] -cnotin @('current_key_id', 'previous_key_id', 'scanned', 'rekeyed', 'previous_references', 'unknown_references')) {
      throw 'mail diagnostic rekey output contained an unknown field'
    }
    if ($values.ContainsKey($parts[0])) { throw 'mail diagnostic rekey output repeated a field' }
    $values[$parts[0]] = $parts[1]
  }
  foreach ($field in @('current_key_id', 'previous_key_id', 'scanned', 'rekeyed', 'previous_references', 'unknown_references')) {
    if (-not $values.ContainsKey($field)) { throw 'mail diagnostic rekey output omitted a field' }
  }
  foreach ($field in @('scanned', 'rekeyed', 'previous_references', 'unknown_references')) {
    if ([string]$values[$field] -notmatch '^[0-9]+$') { throw 'mail diagnostic rekey count was malformed' }
  }
  $keyIDPattern = '^mail-diagnostic-v1-[A-Za-z0-9_-]{22}$'
  if ([string]$values.current_key_id -notmatch $keyIDPattern) {
    throw 'mail diagnostic rekey current key id was malformed'
  }
  if ([string]::IsNullOrWhiteSpace($env:APP_SECRET_PREVIOUS)) {
    if ([string]$values.previous_key_id -cne '') {
      throw 'mail diagnostic rekey unexpectedly reported a previous key id'
    }
  }
  elseif ([string]$values.previous_key_id -notmatch $keyIDPattern -or
          [string]$values.previous_key_id -ceq [string]$values.current_key_id) {
    throw 'mail diagnostic rekey previous key id was malformed or not distinct'
  }
  if ([uint64]$values.scanned -ne $ExpectedScanned -or [uint64]$values.rekeyed -ne $ExpectedRekeyed -or
      [uint64]$values.previous_references -ne 0 -or [uint64]$values.unknown_references -ne 0) {
    throw 'mail diagnostic rekey counts did not satisfy the zero-reference contract'
  }
  if ($rekeyedRowIDs.Count -ne $ExpectedRekeyed) {
    throw 'mail diagnostic rekey row id count did not match the rekeyed count'
  }
  return [pscustomobject]@{
    ExitCode = 0
    CurrentKeyID = [string]$values.current_key_id
    PreviousKeyID = [string]$values.previous_key_id
    Scanned = [uint64]$values.scanned
    Rekeyed = [uint64]$values.rekeyed
    PreviousReferences = [uint64]$values.previous_references
    UnknownReferences = [uint64]$values.unknown_references
    RekeyedRowIDs = $rekeyedRowIDs.ToArray()
  }
}

function Write-RekeyFixtureHelper {
  param([Parameter(Mandatory = $true)][string]$Path)

  $source = @'
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"

	mysqldriver "github.com/go-sql-driver/mysql"
)

var schemaPattern = regexp.MustCompile(`^admin_rekey_[0-9a-f]{12}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func main() {
	if len(os.Args) < 2 {
		fail()
	}
	command := os.Args[1]
	if command == "create-schema" || command == "drop-schema" || command == "schema-absent" {
		if len(os.Args) != 3 || !schemaPattern.MatchString(os.Args[2]) {
			fail()
		}
		runSchemaCommand(command, os.Args[2])
		return
	}
	if command == "verify-current-pair" {
		if len(os.Args) != 5 {
			fail()
		}
		rekeyedRowID, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil || rekeyedRowID <= 0 {
			fail()
		}
		db, _ := openDisposable()
		defer db.Close()
		verifyCurrentPair(db, rekeyedRowID, os.Args[3], os.Args[4])
		return
	}
	snapshotCommand := command == "stage-unknown" || command == "verify-unknown-unchanged" ||
		command == "stage-corrupt" || command == "verify-corrupt-unchanged"
	if (snapshotCommand && (len(os.Args) != 3 || strings.TrimSpace(os.Args[2]) == "")) ||
		(!snapshotCommand && len(os.Args) != 2) {
		fail()
	}
	db, parsed := openDisposable()
	defer db.Close()
	switch command {
	case "clear":
		clearFixtures(db)
	case "stage-pair":
		clearFixtures(db)
		current := mustRing(os.Getenv("APP_SECRET"))
		previous := mustRing(os.Getenv("APP_SECRET_PREVIOUS"))
		insertFixture(db, current.MailDiagnosticKeyID(), encrypt(current, fixtureCode("current")))
		insertFixture(db, previous.MailDiagnosticKeyID(), encrypt(previous, fixtureCode("previous")))
	case "stage-unknown":
		clearFixtures(db)
		current := mustRing(os.Getenv("APP_SECRET"))
		keyID := fixtureUnknownKeyID()
		if keyID == current.MailDiagnosticKeyID() {
			fail()
		}
		insertFixture(db, keyID, fixtureCiphertext("unknown"))
		writeFixtureSnapshot(db, os.Args[2])
	case "verify-unknown-unchanged":
		verifySingleRow(db, fixtureUnknownKeyID(), fixtureCiphertext("unknown"), os.Args[2])
	case "stage-corrupt":
		clearFixtures(db)
		previous := mustRing(os.Getenv("APP_SECRET_PREVIOUS"))
		insertFixture(db, previous.MailDiagnosticKeyID(), fixtureCiphertext("corrupt"))
		writeFixtureSnapshot(db, os.Args[2])
	case "verify-corrupt-unchanged":
		previous := mustRing(os.Getenv("APP_SECRET_PREVIOUS"))
		verifySingleRow(db, previous.MailDiagnosticKeyID(), fixtureCiphertext("corrupt"), os.Args[2])
	default:
		fail()
	}
	_ = parsed
}

func runSchemaCommand(command, schema string) {
	dsn := os.Getenv("MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.DBName != "admin" {
		fail()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fail()
	}
	defer db.Close()
	switch command {
	case "create-schema":
		_, err = db.Exec("CREATE DATABASE `" + schema + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
	case "drop-schema":
		_, err = db.Exec("DROP DATABASE IF EXISTS `" + schema + "`")
	case "schema-absent":
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?", schema).Scan(&count)
		if err == nil && count != 0 {
			fail()
		}
	}
	if err != nil {
		fail()
	}
}

func openDisposable() (*sql.DB, *mysqldriver.Config) {
	dsn := os.Getenv("MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || !schemaPattern.MatchString(parsed.DBName) {
		fail()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil || db.Ping() != nil {
		fail()
	}
	return db, parsed
}

func clearFixtures(db *sql.DB) {
	if _, err := db.Exec("DELETE FROM mail_log_verification_codes"); err != nil {
		fail()
	}
	if _, err := db.Exec("DELETE FROM mail_logs"); err != nil {
		fail()
	}
}

func insertFixture(db *sql.DB, keyID, ciphertext string) {
	result, err := db.Exec("INSERT INTO mail_logs(scene,to_email,subject,status,is_del) VALUES('login','fixture@example.invalid','fixture',2,2)")
	if err != nil {
		fail()
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		fail()
	}
	_, err = db.Exec("INSERT INTO mail_log_verification_codes(mail_log_id,key_id,code_enc,expires_at) VALUES(?,?,?,?)", id, keyID, ciphertext, time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		fail()
	}
}

func mustRing(root string) *secretkey.KeyRing {
	if strings.TrimSpace(root) == "" {
		fail()
	}
	ring, err := secretkey.NewKeyRing(root)
	if err != nil {
		fail()
	}
	return ring
}

func encrypt(ring *secretkey.KeyRing, plain string) string {
	ciphertext, err := secretbox.New(ring.MailDiagnosticKey()).Encrypt(plain)
	if err != nil || ciphertext == "" {
		fail()
	}
	return ciphertext
}

func fixtureCode(label string) string {
	digest := sha256.Sum256([]byte("mail-diagnostic-fixture-code:" + label))
	value := (uint32(digest[0])<<24 | uint32(digest[1])<<16 | uint32(digest[2])<<8 | uint32(digest[3])) % 1000000
	return fmt.Sprintf("%06d", value)
}

func fixtureUnknownKeyID() string {
	digest := sha256.Sum256([]byte("mail-diagnostic-fixture-unknown-key"))
	return "mail-diagnostic-v1-" + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func fixtureCiphertext(label string) string {
	digest := sha256.Sum256([]byte("mail-diagnostic-fixture-ciphertext:" + label))
	return base64.StdEncoding.EncodeToString(digest[:])
}

func verifyCurrentPair(db *sql.DB, rekeyedRowID int64, reportedCurrentKeyID, reportedPreviousKeyID string) {
	current := mustRing(os.Getenv("APP_SECRET"))
	previous := mustRing(os.Getenv("APP_SECRET_PREVIOUS"))
	if reportedCurrentKeyID != current.MailDiagnosticKeyID() || reportedPreviousKeyID != previous.MailDiagnosticKeyID() {
		fail()
	}
	rows, err := db.Query("SELECT id,key_id,code_enc FROM mail_log_verification_codes ORDER BY id")
	if err != nil {
		fail()
	}
	defer rows.Close()
	expected := []string{fixtureCode("current"), fixtureCode("previous")}
	box := secretbox.New(current.MailDiagnosticKey())
	count := 0
	for rows.Next() {
		if count >= len(expected) {
			fail()
		}
		var id int64
		var keyID, ciphertext string
		if rows.Scan(&id, &keyID, &ciphertext) != nil || keyID != current.MailDiagnosticKeyID() {
			fail()
		}
		if (count == 0 && id == rekeyedRowID) || (count == 1 && id != rekeyedRowID) {
			fail()
		}
		plain, decryptErr := box.Decrypt(ciphertext)
		if decryptErr != nil || plain != expected[count] {
			fail()
		}
		count++
	}
	if rows.Err() != nil || count != len(expected) {
		fail()
	}
}

func verifySingleRow(db *sql.DB, expectedKeyID, expectedCiphertext, snapshotPath string) {
	rows, err := db.Query("SELECT key_id,code_enc FROM mail_log_verification_codes ORDER BY id")
	if err != nil {
		fail()
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var keyID, ciphertext string
		if rows.Scan(&keyID, &ciphertext) != nil || keyID != expectedKeyID || ciphertext != expectedCiphertext {
			fail()
		}
		count++
	}
	if rows.Err() != nil || count != 1 {
		fail()
	}
	verifyFixtureSnapshot(db, snapshotPath)
}

func fixtureStateDigest(db *sql.DB) string {
	rows, err := db.Query(`
SELECT SHA2(CONCAT_WS(CHAR(31),
  CAST(vc.id AS CHAR), CAST(vc.mail_log_id AS CHAR), vc.key_id, vc.code_enc,
  DATE_FORMAT(vc.expires_at,'%Y-%m-%d %H:%i:%s.%f'), DATE_FORMAT(vc.created_at,'%Y-%m-%d %H:%i:%s.%f'),
  CAST(ml.id AS CHAR), ml.scene, COALESCE(CAST(ml.template_id AS CHAR),'<null>'), ml.to_email, ml.subject,
  ml.tencent_request_id, ml.tencent_message_id, CAST(ml.status AS CHAR), CAST(ml.is_del AS CHAR),
  ml.error_code, ml.error_message, CAST(ml.duration_ms AS CHAR),
  COALESCE(DATE_FORMAT(ml.sent_at,'%Y-%m-%d %H:%i:%s.%f'),'<null>'),
  DATE_FORMAT(ml.created_at,'%Y-%m-%d %H:%i:%s.%f'), DATE_FORMAT(ml.updated_at,'%Y-%m-%d %H:%i:%s.%f')
),256)
FROM mail_log_verification_codes vc
JOIN mail_logs ml ON ml.id=vc.mail_log_id
ORDER BY vc.id`)
	if err != nil {
		fail()
	}
	defer rows.Close()
	digest := ""
	count := 0
	for rows.Next() {
		if count != 0 || rows.Scan(&digest) != nil {
			fail()
		}
		count++
	}
	if rows.Err() != nil || count != 1 || !digestPattern.MatchString(digest) {
		fail()
	}
	return digest
}

func writeFixtureSnapshot(db *sql.DB, path string) {
	if err := os.WriteFile(path, []byte(fixtureStateDigest(db)+"\n"), 0600); err != nil {
		fail()
	}
}

func verifyFixtureSnapshot(db *sql.DB, path string) {
	expected, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(expected)) != fixtureStateDigest(db) {
		fail()
	}
}

func fail() {
	fmt.Fprintln(os.Stderr, "fixture helper failed")
	os.Exit(1)
}
'@
  [IO.File]::WriteAllText($Path, $source, [Text.UTF8Encoding]::new($false))
}

function Invoke-FixtureHelper {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  $result = Invoke-GoCapture `
    -Arguments (@('run', $script:FixtureHelper) + $Arguments) `
    -TimeoutSeconds 180 `
    -Operation 'mail diagnostic fixture helper'
  if ($result.ExitCode -ne 0 -or -not [string]::IsNullOrWhiteSpace($result.Text)) {
    throw 'mail diagnostic fixture helper failed or wrote output'
  }
}

function Invoke-SessionRotationRace {
  param([Parameter(Mandatory = $true)][string]$SecretEnv)

  $arguments = @(
    'run', '--rm', '--network', 'admin-platform',
    '--env-file', $script:RuntimeEnv,
    '--env-file', $SecretEnv,
    '-v', ($script:RepoRoot + ':/src'),
    '-w', '/src',
    '-v', 'admin-go-mod-cache:/go/pkg/mod',
    '-v', 'admin-go-build-cache:/root/.cache/go-build',
    'docker.m.daocloud.io/library/golang:1.26.5-bookworm',
    'go', 'test', './internal/module/auth', '-run', '^TestMultiNode', '-race', '-count=1', '-v'
  )
  $result = Invoke-BoundedProcessCapture `
    -Executable $script:DockerExecutable `
    -Arguments $arguments `
    -Operation 'Docker session-secret rotation rehearsal' `
    -TimeoutSeconds 900 `
    -WorkingDirectory $script:RepoRoot
  if ($result.ExitCode -ne 0) { throw 'Docker session-secret rotation rehearsal failed' }
  $exitCode = [int]$result.ExitCode
  $result = $null
  return $exitCode
}

$script:RepoRoot = $repoRoot
$script:RuntimeEnv = $runtimeEnv
$script:DockerExecutable = $docker
$tempRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot ('.tmp\admin-p04-secret-rotation-' + [guid]::NewGuid().ToString('N'))))
$verifiedTempRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.tmp'))
if (-not (Test-PathInsideRoot -Path $tempRoot -Root $verifiedTempRoot) -or
    -not ([IO.Path]::GetFileName($tempRoot)).StartsWith('admin-p04-secret-rotation-', [StringComparison]::Ordinal)) {
  throw 'Refusing to create an unverified rotation rehearsal directory.'
}

$secretEnv = Join-Path $tempRoot 'rotation.env'
$fixtureHelper = Join-Path $tempRoot 'main.go'
$fixtureSnapshot = Join-Path $tempRoot 'fixture-state.sha256'
$script:FixtureHelper = $fixtureHelper
$disposableSchema = 'admin_rekey_' + [guid]::NewGuid().ToString('N').Substring(0, 12)
$schemaCreated = $false
$oldSecret = $null
$newSecret = $null
$persistentDSN = $null
$persistentSecret = $null
$persistentAtlasDSN = $null
$disposableDSN = $null
$disposableAtlasDSN = $null
$containerEnvironment = $null
$hostEnvironment = $null
$persistentSettings = $null
Set-Variable -Name disposableSettings -Value $null
$summaryData = $null
$primaryFailureMessage = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
$previousAppSecret = [Environment]::GetEnvironmentVariable('APP_SECRET', 'Process')
$previousAppSecretPrevious = [Environment]::GetEnvironmentVariable('APP_SECRET_PREVIOUS', 'Process')
$previousPath = [Environment]::GetEnvironmentVariable('Path', 'Process')
$script:SensitiveValues = @($previousDSN, $previousAppSecret, $previousAppSecretPrevious)
$schemaInvariantNames = @(
  'required_tables',
  'required_columns',
  'required_column_shapes',
  'required_indexes',
  'required_constraints',
  'mail_verification_diagnostic_table',
  'mail_verification_diagnostic_columns',
  'mail_verification_diagnostic_column_shapes',
  'mail_verification_diagnostic_indexes',
  'mail_verification_diagnostic_foreign_key'
)
$relationInvariantNames = @(
  'rbac_relationship_orphans',
  'payment_relationship_orphans',
  'wallet_relationship_orphans',
  'ai_relationship_orphans',
  'notification_relationship_orphans',
  'export_relationship_orphans',
  'mail_verification_diagnostic_orphans'
)

try {
  $oldSecret = New-RotationSecret
  $newSecret = New-RotationSecret
  $script:SensitiveValues += @($oldSecret, $newSecret)
  [IO.Directory]::CreateDirectory($tempRoot) | Out-Null
  Write-RekeyFixtureHelper -Path $fixtureHelper
  Write-RestrictedTextFile -Path $secretEnv -Content ((@(
    'ADMIN_IDENTITY_INTEGRATION=1'
    'P04_ROTATION_OLD_SECRET=' + $oldSecret
    'P04_ROTATION_NEW_SECRET=' + $newSecret
  ) -join "`n") + "`n")

  $platformResult = Invoke-BoundedProcessCapture `
    -Executable (Join-Path $PSHOME 'pwsh.exe') `
    -Arguments @('-NoProfile', '-File', $platformScript, '-Action', 'dev-state') `
    -Operation 'Docker state preparation' `
    -TimeoutSeconds 600 `
    -WorkingDirectory $repoRoot
  if ($platformResult.ExitCode -ne 0) { throw 'Docker state preparation failed' }
  $platformResult = $null

  $requiredEnvironmentKeys = @('MYSQL_DSN', 'APP_SECRET', 'REDIS_ADDR', 'HTTP_ADDR', 'LOG_DIR', 'PAYMENT_CERT_BASE_DIR')
  $containerEnvironment = Read-AdminDevEnvironmentFile `
    -Path $runtimeEnv `
    -RequiredKeys $requiredEnvironmentKeys `
    -AllowEmptyKeys @()
  $hostEnvironment = ConvertTo-AdminDevHostEnvironment -Environment $containerEnvironment -RepositoryRoot $repoRoot
  $tools = Resolve-AdminDevHostTools
  $script:GoExecutable = $tools.GoExecutable
  $env:Path = (Split-Path $tools.GoExecutable -Parent) + [IO.Path]::PathSeparator + $env:Path

  $persistentDSN = [string]$hostEnvironment['MYSQL_DSN']
  $persistentSecret = [string]$hostEnvironment['APP_SECRET']
  $script:SensitiveValues += @($persistentDSN, $persistentSecret)
  $env:MYSQL_DSN = $persistentDSN
  $env:APP_SECRET = $persistentSecret
  $env:APP_SECRET_PREVIOUS = $null
  $persistentSettings = Get-MySQLDSNSettings -Database 'admin'
  $persistentAtlasDSN = Get-AtlasDatabaseURL -Settings $persistentSettings -Database 'admin'
  $script:SensitiveValues += @($persistentSettings.Password, $persistentAtlasDSN)

  $persistentMigration = Invoke-LockedAtlasMigration -Settings $persistentSettings -Database 'admin'
  $schemaInvariantCount = Invoke-InvariantGate `
    -Database 'admin' `
    -RelativePath 'database/reconciliation/030_verify_schema.sql' `
    -ExpectedNames $schemaInvariantNames
  $relationInvariantCount = Invoke-InvariantGate `
    -Database 'admin' `
    -RelativePath 'database/reconciliation/031_verify_relations.sql' `
    -ExpectedNames $relationInvariantNames
  $persistentNoOp = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 0 -ExpectedRekeyed 0

  $schemaCreated = $true
  Invoke-FixtureHelper -Arguments @('create-schema', $disposableSchema)
  $disposableSettings = [pscustomobject]@{
    User = $persistentSettings.User
    Password = $persistentSettings.Password
    Host = $persistentSettings.Host
    Port = $persistentSettings.Port
    Query = $persistentSettings.Query
  }
  $disposableDSN = New-SchemaDSN -Settings $disposableSettings -Database $disposableSchema
  $disposableAtlasDSN = Get-AtlasDatabaseURL -Settings $disposableSettings -Database $disposableSchema
  $script:SensitiveValues += @($disposableSettings.Password, $disposableDSN, $disposableAtlasDSN)
  $env:MYSQL_DSN = $disposableDSN
  Invoke-DisposableSchemaBootstrap -Settings $disposableSettings -Database $disposableSchema
  $disposableSchemaInvariantCount = Invoke-InvariantGate `
    -Database $disposableSchema `
    -RelativePath 'database/reconciliation/030_verify_schema.sql' `
    -ExpectedNames $schemaInvariantNames
  $disposableRelationInvariantCount = Invoke-InvariantGate `
    -Database $disposableSchema `
    -RelativePath 'database/reconciliation/031_verify_relations.sql' `
    -ExpectedNames $relationInvariantNames

  $env:APP_SECRET = $newSecret
  $env:APP_SECRET_PREVIOUS = $oldSecret
  Invoke-FixtureHelper -Arguments @('stage-pair')
  $conversion = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 1 -ExpectedRekeyed 1
  Invoke-FixtureHelper -Arguments @(
    'verify-current-pair',
    [string]$conversion.RekeyedRowIDs[0],
    [string]$conversion.CurrentKeyID,
    [string]$conversion.PreviousKeyID
  )

  $env:APP_SECRET_PREVIOUS = $null
  $idempotent = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 0 -ExpectedRekeyed 0

  $env:APP_SECRET_PREVIOUS = $oldSecret
  Invoke-FixtureHelper -Arguments @('stage-unknown', $fixtureSnapshot)
  $unknown = Invoke-MailDiagnosticRekey -ExpectSuccess $false
  Invoke-FixtureHelper -Arguments @('verify-unknown-unchanged', $fixtureSnapshot)
  Invoke-FixtureHelper -Arguments @('stage-corrupt', $fixtureSnapshot)
  $corrupt = Invoke-MailDiagnosticRekey -ExpectSuccess $false
  Invoke-FixtureHelper -Arguments @('verify-corrupt-unchanged', $fixtureSnapshot)
  Invoke-FixtureHelper -Arguments @('clear')
  $null = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 0 -ExpectedRekeyed 0

  $env:MYSQL_DSN = $persistentDSN
  $env:APP_SECRET = $persistentSecret
  $env:APP_SECRET_PREVIOUS = $null
  Invoke-FixtureHelper -Arguments @('drop-schema', $disposableSchema)
  Invoke-FixtureHelper -Arguments @('schema-absent', $disposableSchema)
  $schemaCreated = $false

  $sessionRaceExit = Invoke-SessionRotationRace -SecretEnv $secretEnv
  $summaryData = [ordered]@{
    atlas_status = [string]$persistentMigration.Status
    schema_sha256 = [string]$persistentMigration.Fingerprint
    reconciliation_030_checks = [int]$schemaInvariantCount
    reconciliation_031_checks = [int]$relationInvariantCount
    persistent_rekey_scanned = [uint64]$persistentNoOp.Scanned
    persistent_rekeyed = [uint64]$persistentNoOp.Rekeyed
    disposable_reconciliation_030_checks = [int]$disposableSchemaInvariantCount
    disposable_reconciliation_031_checks = [int]$disposableRelationInvariantCount
    conversion_scanned = [uint64]$conversion.Scanned
    conversion_rekeyed = [uint64]$conversion.Rekeyed
    previous_references = [uint64]$conversion.PreviousReferences
    unknown_references = [uint64]$conversion.UnknownReferences
    idempotent_scanned = [uint64]$idempotent.Scanned
    idempotent_rekeyed = [uint64]$idempotent.Rekeyed
    unknown_exit_status = [int]$unknown.ExitCode
    corrupt_exit_status = [int]$corrupt.ExitCode
    session_rotation_exit_status = [int]$sessionRaceExit
  }
}
catch {
  $candidateMessage = [string]$_.Exception.Message
  try {
    Assert-NoSensitiveOutput -Text $candidateMessage
    $primaryFailureMessage = $candidateMessage
  }
  catch {
    $primaryFailureMessage = 'session secret rotation verification failed'
  }
}
finally {
  if ($schemaCreated -and -not [string]::IsNullOrWhiteSpace($persistentDSN) -and (Test-Path -LiteralPath $fixtureHelper -PathType Leaf)) {
    try {
      $env:MYSQL_DSN = $persistentDSN
      $env:APP_SECRET = $persistentSecret
      $env:APP_SECRET_PREVIOUS = $null
      Invoke-FixtureHelper -Arguments @('drop-schema', $disposableSchema)
      Invoke-FixtureHelper -Arguments @('schema-absent', $disposableSchema)
      $schemaCreated = $false
    }
    catch {
      $cleanupFailures.Add('failed to verify disposable rotation schema cleanup')
    }
  }
  foreach ($restoration in @(
    @('MYSQL_DSN', $previousDSN),
    @('APP_SECRET', $previousAppSecret),
    @('APP_SECRET_PREVIOUS', $previousAppSecretPrevious),
    @('Path', $previousPath)
  )) {
    try {
      [Environment]::SetEnvironmentVariable([string]$restoration[0], $restoration[1], 'Process')
    }
    catch {
      if (-not $cleanupFailures.Contains('failed to restore the rotation process environment')) {
        $cleanupFailures.Add('failed to restore the rotation process environment')
      }
    }
  }
  if (Test-Path -LiteralPath $tempRoot -PathType Container) {
    try {
      $resolvedDirectory = [IO.Path]::GetFullPath($tempRoot)
      if (-not (Test-PathInsideRoot -Path $resolvedDirectory -Root $verifiedTempRoot) -or
          -not ([IO.Path]::GetFileName($resolvedDirectory)).StartsWith('admin-p04-secret-rotation-', [StringComparison]::Ordinal)) {
        throw 'unverified rotation rehearsal directory'
      }
      [IO.Directory]::Delete($resolvedDirectory, $true)
    }
    catch {
      $cleanupFailures.Add('failed to remove the verified rotation rehearsal directory')
    }
  }
  if ($null -ne $persistentSettings) {
    $persistentSettings.Password = $null
  }
  if ($null -ne $disposableSettings) {
    $disposableSettings.Password = $null
  }
  $oldSecret = $null
  $newSecret = $null
  $persistentSecret = $null
  $persistentDSN = $null
  $persistentAtlasDSN = $null
  $disposableDSN = $null
  $disposableAtlasDSN = $null
  $previousDSN = $null
  $previousAppSecret = $null
  $previousAppSecretPrevious = $null
  $containerEnvironment = $null
  $hostEnvironment = $null
  $persistentSettings = $null
  $disposableSettings = $null
  $script:FixtureHelper = $null
  $script:GoExecutable = $null
  $script:SensitiveValues = @()
}

if ($null -ne $primaryFailureMessage) {
  if ($cleanupFailures.Count -gt 0) {
    throw ($primaryFailureMessage + '; cleanup also failed: ' + ($cleanupFailures -join '; '))
  }
  throw $primaryFailureMessage
}
if ($cleanupFailures.Count -gt 0) {
  throw ('session secret rotation cleanup failed: ' + ($cleanupFailures -join '; '))
}
if ($null -eq $summaryData) {
  throw 'session secret rotation completed without a summary'
}
$summaryJson = $summaryData | ConvertTo-Json -Compress
Write-Output $summaryJson
$summaryJson = $null
