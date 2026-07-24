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
$defaultDocker = 'E:\Docker\Docker\resources\bin\docker.exe'
$docker = if (Test-Path -LiteralPath $defaultDocker -PathType Leaf) {
  $defaultDocker
}
else {
  (Get-Command docker.exe -ErrorAction Stop | Select-Object -First 1).Source
}

. (Join-Path $repoRoot 'scripts\dev\admin-dev-common.ps1')
. (Join-Path $repoRoot 'scripts\database\atlas-runtime-common.ps1')

if (-not (Test-Path -LiteralPath $runtimeEnv -PathType Leaf)) {
  throw 'Docker runtime env is missing; run scripts/docker-platform.ps1 init first.'
}

$adminDevSource = [IO.File]::ReadAllText($adminDevScript, [Text.Encoding]::UTF8)
foreach ($forbidden in @('mail-diagnostic-rekey', 'migrate apply', 'migrate set')) {
  if ($adminDevSource.Contains($forbidden, [StringComparison]::OrdinalIgnoreCase)) {
    throw "admin-dev must not perform startup migration or rekey: $forbidden"
  }
}

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

function Invoke-GoCapture {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  Push-Location $script:RepoRoot
  try {
    $output = @(& $script:GoExecutable @Arguments 2>&1 | ForEach-Object { $_.ToString() })
    $exitCode = $LASTEXITCODE
    $text = $output -join "`n"
    Assert-NoSensitiveOutput -Text $text
    return [pscustomobject]@{ ExitCode = $exitCode; Lines = $output; Text = $text }
  }
  finally {
    Pop-Location
  }
}

function Assert-NoSensitiveOutput {
  param([AllowEmptyString()][string]$Text)

  foreach ($value in @($script:SensitiveValues)) {
    if (-not [string]::IsNullOrEmpty([string]$value) -and $Text.Contains([string]$value, [StringComparison]::Ordinal)) {
      throw 'rotation verification command output exposed a sensitive runtime value'
    }
  }
}

function Get-SchemaFingerprint {
  param(
    [Parameter(Mandatory = $true)]$Settings,
    [Parameter(Mandatory = $true)][string]$Database
  )

  $previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
  try {
    $env:MYSQL_DSN = New-SchemaDSN -Settings $Settings -Database $Database
    return Get-DatabaseFingerprintSHA -BackendRoot $script:RepoRoot -Settings $Settings -Database $Database
  }
  finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
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
    $locked = Invoke-GoCapture -Arguments (@(
      'run', './cmd/admin-db', 'lock-run',
      '--schema', $Database,
      '--name', 'admin:atlas:migrate',
      '--timeout', '30s',
      '--expected-fingerprint', $beforeFingerprint,
      '--'
    ) + @($script:DockerExecutable) + $dockerArguments)
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
    return [pscustomobject]@{ Status = 'ok'; Fingerprint = $afterFingerprint }
  }
  finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    Remove-AtlasRuntimeConfig -Directory $runtimeDirectory
  }
}

function Invoke-InvariantGate {
  param(
    [Parameter(Mandatory = $true)][string]$Database,
    [Parameter(Mandatory = $true)][string]$RelativePath
  )

  $result = Invoke-GoCapture -Arguments @('run', './cmd/admin-db', 'invariants', '--schema', $Database, '--file', $RelativePath)
  if ($result.ExitCode -ne 0 -or $result.Lines.Count -eq 0) {
    throw 'database reconciliation invariant command failed'
  }
  foreach ($line in $result.Lines) {
    $parts = [string]$line -split "`t", 2
    if ($parts.Count -ne 2 -or $parts[1] -notmatch '^[0-9]+$' -or [uint64]$parts[1] -ne 0) {
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

  $result = Invoke-GoCapture -Arguments @('run', './cmd/admin-db', 'mail-diagnostic-rekey')
  if (-not $ExpectSuccess) {
    if ($result.ExitCode -eq 0) { throw 'mail diagnostic rekey unexpectedly succeeded' }
    return [pscustomobject]@{ ExitCode = $result.ExitCode; Scanned = 0; Rekeyed = 0; PreviousReferences = 0; UnknownReferences = 0 }
  }
  if ($result.ExitCode -ne 0) { throw 'mail diagnostic rekey failed' }

  $values = @{}
  foreach ($line in $result.Lines) {
    $parts = [string]$line -split "`t", 2
    if ($parts.Count -ne 2) { throw 'mail diagnostic rekey output was malformed' }
    if ($parts[0] -eq 'rekeyed_row_id') { continue }
    if ($values.ContainsKey($parts[0])) { throw 'mail diagnostic rekey output repeated a field' }
    $values[$parts[0]] = $parts[1]
  }
  foreach ($field in @('current_key_id', 'previous_key_id', 'scanned', 'rekeyed', 'previous_references', 'unknown_references')) {
    if (-not $values.ContainsKey($field)) { throw 'mail diagnostic rekey output omitted a field' }
  }
  foreach ($field in @('scanned', 'rekeyed', 'previous_references', 'unknown_references')) {
    if ([string]$values[$field] -notmatch '^[0-9]+$') { throw 'mail diagnostic rekey count was malformed' }
  }
  if ([uint64]$values.scanned -ne $ExpectedScanned -or [uint64]$values.rekeyed -ne $ExpectedRekeyed -or
      [uint64]$values.previous_references -ne 0 -or [uint64]$values.unknown_references -ne 0) {
    throw 'mail diagnostic rekey counts did not satisfy the zero-reference contract'
  }
  return [pscustomobject]@{
    ExitCode = 0
    Scanned = [uint64]$values.scanned
    Rekeyed = [uint64]$values.rekeyed
    PreviousReferences = [uint64]$values.previous_references
    UnknownReferences = [uint64]$values.unknown_references
  }
}

function Write-RekeyFixtureHelper {
  param([Parameter(Mandatory = $true)][string]$Path)

  $source = @'
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"

	mysqldriver "github.com/go-sql-driver/mysql"
)

var schemaPattern = regexp.MustCompile(`^admin_rekey_[0-9a-f]{12}$`)
var codePattern = regexp.MustCompile(`^[0-9]{6}$`)

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
	if len(os.Args) != 2 {
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
		insertFixture(db, current.MailDiagnosticKeyID(), encrypt(current, newCode()))
		insertFixture(db, previous.MailDiagnosticKeyID(), encrypt(previous, newCode()))
	case "verify-current-pair":
		current := mustRing(os.Getenv("APP_SECRET"))
		rows, err := db.Query("SELECT key_id,code_enc FROM mail_log_verification_codes ORDER BY id")
		if err != nil {
			fail()
		}
		defer rows.Close()
		count := 0
		box := secretbox.New(current.MailDiagnosticKey())
		for rows.Next() {
			var keyID, ciphertext string
			if rows.Scan(&keyID, &ciphertext) != nil || keyID != current.MailDiagnosticKeyID() {
				fail()
			}
			plain, err := box.Decrypt(ciphertext)
			if err != nil || !codePattern.MatchString(plain) {
				fail()
			}
			count++
		}
		if rows.Err() != nil || count != 2 {
			fail()
		}
	case "stage-unknown":
		clearFixtures(db)
		current := mustRing(os.Getenv("APP_SECRET"))
		unknown := randomBytes(16)
		keyID := "mail-diagnostic-v1-" + base64.RawURLEncoding.EncodeToString(unknown)
		for keyID == current.MailDiagnosticKeyID() {
			unknown = randomBytes(16)
			keyID = "mail-diagnostic-v1-" + base64.RawURLEncoding.EncodeToString(unknown)
		}
		insertFixture(db, keyID, encrypt(current, newCode()))
	case "stage-corrupt":
		clearFixtures(db)
		previous := mustRing(os.Getenv("APP_SECRET_PREVIOUS"))
		insertFixture(db, previous.MailDiagnosticKeyID(), base64.StdEncoding.EncodeToString(randomBytes(32)))
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

func newCode() string {
	raw := randomBytes(6)
	for index := range raw {
		raw[index] = '0' + raw[index]%10
	}
	return string(raw)
}

func randomBytes(length int) []byte {
	value := make([]byte, length)
	if _, err := rand.Read(value); err != nil {
		fail()
	}
	return value
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

  $result = Invoke-GoCapture -Arguments (@('run', $script:FixtureHelper) + $Arguments)
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
  $output = @(& $script:DockerExecutable @arguments 2>&1 | ForEach-Object { $_.ToString() })
  $exitCode = $LASTEXITCODE
  Assert-NoSensitiveOutput -Text ($output -join "`n")
  $output = $null
  if ($exitCode -ne 0) { throw 'Docker session-secret rotation rehearsal failed' }
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

$oldSecret = New-RotationSecret
$newSecret = New-RotationSecret
$secretEnv = Join-Path $tempRoot 'rotation.env'
$fixtureHelper = Join-Path $tempRoot 'main.go'
$script:FixtureHelper = $fixtureHelper
$disposableSchema = 'admin_rekey_' + [guid]::NewGuid().ToString('N').Substring(0, 12)
$schemaCreated = $false
$persistentDSN = $null
$persistentSecret = $null
$containerEnvironment = $null
$hostEnvironment = $null
$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
$previousAppSecret = [Environment]::GetEnvironmentVariable('APP_SECRET', 'Process')
$previousAppSecretPrevious = [Environment]::GetEnvironmentVariable('APP_SECRET_PREVIOUS', 'Process')
$script:SensitiveValues = @($oldSecret, $newSecret, $previousDSN, $previousAppSecret, $previousAppSecretPrevious)

try {
  [IO.Directory]::CreateDirectory($tempRoot) | Out-Null
  Write-RekeyFixtureHelper -Path $fixtureHelper
  Write-RestrictedTextFile -Path $secretEnv -Content ((@(
    'ADMIN_IDENTITY_INTEGRATION=1'
    'P04_ROTATION_OLD_SECRET=' + $oldSecret
    'P04_ROTATION_NEW_SECRET=' + $newSecret
  ) -join "`n") + "`n")

  & $platformScript -Action dev-state *> $null
  if ($LASTEXITCODE -ne 0) { throw 'Docker state preparation failed' }

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
  try {
    $persistentMigration = Invoke-LockedAtlasMigration -Settings $persistentSettings -Database 'admin'
    $schemaInvariantCount = Invoke-InvariantGate -Database 'admin' -RelativePath 'database/reconciliation/030_verify_schema.sql'
    $relationInvariantCount = Invoke-InvariantGate -Database 'admin' -RelativePath 'database/reconciliation/031_verify_relations.sql'
    $persistentNoOp = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 0 -ExpectedRekeyed 0

    Invoke-FixtureHelper -Arguments @('create-schema', $disposableSchema)
    $schemaCreated = $true
    $disposableSettings = [pscustomobject]@{
      User = $persistentSettings.User
      Password = $persistentSettings.Password
      Host = $persistentSettings.Host
      Port = $persistentSettings.Port
      Query = $persistentSettings.Query
    }
    try {
      $disposableDSN = New-SchemaDSN -Settings $disposableSettings -Database $disposableSchema
      $script:SensitiveValues += $disposableDSN
      $env:MYSQL_DSN = $disposableDSN
      $disposableMigration = Invoke-LockedAtlasMigration -Settings $disposableSettings -Database $disposableSchema
      $disposableSchemaInvariantCount = Invoke-InvariantGate -Database $disposableSchema -RelativePath 'database/reconciliation/030_verify_schema.sql'
      $disposableRelationInvariantCount = Invoke-InvariantGate -Database $disposableSchema -RelativePath 'database/reconciliation/031_verify_relations.sql'

      $env:APP_SECRET = $newSecret
      $env:APP_SECRET_PREVIOUS = $oldSecret
      Invoke-FixtureHelper -Arguments @('stage-pair')
      $conversion = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 1 -ExpectedRekeyed 1
      Invoke-FixtureHelper -Arguments @('verify-current-pair')

      $env:APP_SECRET_PREVIOUS = $null
      $idempotent = Invoke-MailDiagnosticRekey -ExpectSuccess $true -ExpectedScanned 0 -ExpectedRekeyed 0

      $env:APP_SECRET_PREVIOUS = $oldSecret
      Invoke-FixtureHelper -Arguments @('stage-unknown')
      $unknown = Invoke-MailDiagnosticRekey -ExpectSuccess $false
      Invoke-FixtureHelper -Arguments @('stage-corrupt')
      $corrupt = Invoke-MailDiagnosticRekey -ExpectSuccess $false
      Invoke-FixtureHelper -Arguments @('clear')
    }
    finally {
      $disposableSettings.Password = $null
      $env:MYSQL_DSN = $persistentDSN
      $env:APP_SECRET = $persistentSecret
      $env:APP_SECRET_PREVIOUS = $null
    }

    Invoke-FixtureHelper -Arguments @('drop-schema', $disposableSchema)
    $schemaCreated = $false
    Invoke-FixtureHelper -Arguments @('schema-absent', $disposableSchema)

    $sessionRaceExit = Invoke-SessionRotationRace -SecretEnv $secretEnv
    [ordered]@{
      atlas_status = [string]$persistentMigration.Status
      schema_sha256 = [string]$persistentMigration.Fingerprint
      reconciliation_030_checks = [int]$schemaInvariantCount
      reconciliation_031_checks = [int]$relationInvariantCount
      persistent_rekey_scanned = [uint64]$persistentNoOp.Scanned
      persistent_rekeyed = [uint64]$persistentNoOp.Rekeyed
      disposable_atlas_status = [string]$disposableMigration.Status
      disposable_schema_sha256 = [string]$disposableMigration.Fingerprint
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
    } | ConvertTo-Json -Compress
  }
  finally {
    $persistentSettings.Password = $null
  }
}
finally {
  $cleanupFailure = $null
  if ($schemaCreated -and -not [string]::IsNullOrWhiteSpace($persistentDSN) -and (Test-Path -LiteralPath $fixtureHelper -PathType Leaf)) {
    try {
      $env:MYSQL_DSN = $persistentDSN
      $env:APP_SECRET = $persistentSecret
      $env:APP_SECRET_PREVIOUS = $null
      Invoke-FixtureHelper -Arguments @('drop-schema', $disposableSchema)
      Invoke-FixtureHelper -Arguments @('schema-absent', $disposableSchema)
    }
    catch {
      $cleanupFailure = 'failed to verify disposable rotation schema cleanup'
    }
  }
  [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
  [Environment]::SetEnvironmentVariable('APP_SECRET', $previousAppSecret, 'Process')
  [Environment]::SetEnvironmentVariable('APP_SECRET_PREVIOUS', $previousAppSecretPrevious, 'Process')
  $oldSecret = $null
  $newSecret = $null
  $persistentSecret = $null
  $persistentDSN = $null
  $containerEnvironment = $null
  $hostEnvironment = $null
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
      if ($null -eq $cleanupFailure) {
        $cleanupFailure = 'failed to remove the verified rotation rehearsal directory'
      }
    }
  }
  if ($null -ne $cleanupFailure) {
    throw $cleanupFailure
  }
}
