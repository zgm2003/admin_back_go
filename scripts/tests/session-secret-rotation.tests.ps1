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
      $directMutator = $commandLeaf -ceq 'invoke-maildiagnosticrekey'
      $mutationCapable = [string]::IsNullOrWhiteSpace($commandName) -or $commandLeaf -in @(
        'go',
        'docker',
        'admin-db',
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

function Write-RestrictedTextFile {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Content
  )

  if (Test-Path -LiteralPath $Path) { throw 'restricted file already exists' }
  if ($IsWindows) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $security = [Security.AccessControl.FileSecurity]::new()
    $security.SetOwner($identity.User)
    $security.SetAccessRuleProtection($true, $false)
    $rule = [Security.AccessControl.FileSystemAccessRule]::new(
      $identity.User,
      [Security.AccessControl.FileSystemRights]::FullControl,
      [Security.AccessControl.AccessControlType]::Allow
    )
    [void]$security.AddAccessRule($rule)
    $stream = [IO.FileSystemAclExtensions]::Create(
      [IO.FileInfo]::new($Path),
      [IO.FileMode]::CreateNew,
      [Security.AccessControl.FileSystemRights]::Write,
      [IO.FileShare]::None,
      4096,
      [IO.FileOptions]::None,
      $security
    )
  }
  else {
    $stream = [IO.FileStream]::new($Path, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    & chmod 600 -- $Path
    if ($LASTEXITCODE -ne 0) {
      $stream.Dispose()
      throw 'failed to restrict runtime file permissions'
    }
  }
  try {
    $bytes = [Text.UTF8Encoding]::new($false).GetBytes($Content)
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Flush($true)
  }
  finally {
    $stream.Dispose()
  }
}

function New-DisposableSchemaDSN {
  param(
    [Parameter(Mandatory = $true)][string]$SourceDSN,
    [Parameter(Mandatory = $true)][string]$Database
  )

  if ($Database -notmatch '^admin_rekey_[0-9a-f]{12}$') {
    throw 'refusing to build a DSN for an unexpected disposable schema'
  }
  $addressMarker = '@tcp('
  $addressIndex = $SourceDSN.LastIndexOf($addressMarker, [StringComparison]::Ordinal)
  $databaseStart = if ($addressIndex -gt 0) {
    $SourceDSN.IndexOf(')/', $addressIndex + $addressMarker.Length, [StringComparison]::Ordinal)
  }
  else {
    -1
  }
  if ($databaseStart -lt 0) { throw 'MYSQL_DSN is not a supported TCP DSN' }
  $databaseStart += 2
  $queryIndex = $SourceDSN.IndexOf('?', $databaseStart)
  $databaseEnd = if ($queryIndex -ge 0) { $queryIndex } else { $SourceDSN.Length }
  if ($SourceDSN.Substring($databaseStart, $databaseEnd - $databaseStart) -cne 'admin') {
    throw 'MYSQL_DSN must target the admin schema'
  }
  return $SourceDSN.Substring(0, $databaseStart) + $Database + $SourceDSN.Substring($databaseEnd)
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

function Invoke-DisposableSchemaBootstrap {
  param([Parameter(Mandatory = $true)][string]$Database)

  if ($Database -notmatch '^admin_rekey_[0-9a-f]{12}$') {
    throw 'refusing to bootstrap an unexpected disposable schema'
  }
  $schemaPath = Join-Path $script:RepoRoot 'database\schema.sql'
  if (-not (Test-Path -LiteralPath $schemaPath -PathType Leaf)) {
    throw 'canonical schema.sql is missing'
  }
  Invoke-FixtureHelper -Arguments @('apply-schema', $schemaPath)
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
	if command == "apply-schema" {
		if len(os.Args) != 3 {
			fail()
		}
		applySchema(os.Args[2])
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

func applySchema(path string) {
	content, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(content), "CREATE TABLE `schema_migrations`") ||
		strings.Contains(string(content), "CREATE DATABASE") || strings.Contains(string(content), "DROP DATABASE") {
		fail()
	}
	parsed, err := mysqldriver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || !schemaPattern.MatchString(parsed.DBName) {
		fail()
	}
	parsed.MultiStatements = true
	db, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		fail()
	}
	defer db.Close()
	if _, err = db.Exec(string(content)); err != nil {
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
$disposableDSN = $null
$containerEnvironment = $null
$hostEnvironment = $null
$summaryData = $null
$primaryFailureMessage = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
$previousAppSecret = [Environment]::GetEnvironmentVariable('APP_SECRET', 'Process')
$previousAppSecretPrevious = [Environment]::GetEnvironmentVariable('APP_SECRET_PREVIOUS', 'Process')
$previousPath = [Environment]::GetEnvironmentVariable('Path', 'Process')
$script:SensitiveValues = @($previousDSN, $previousAppSecret, $previousAppSecretPrevious)
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

  $schemaCreated = $true
  Invoke-FixtureHelper -Arguments @('create-schema', $disposableSchema)
  $disposableDSN = New-DisposableSchemaDSN -SourceDSN $persistentDSN -Database $disposableSchema
  $script:SensitiveValues += @($disposableDSN)
  $env:MYSQL_DSN = $disposableDSN
  Invoke-DisposableSchemaBootstrap -Database $disposableSchema

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
  $oldSecret = $null
  $newSecret = $null
  $persistentSecret = $null
  $persistentDSN = $null
  $disposableDSN = $null
  $previousDSN = $null
  $previousAppSecret = $null
  $previousAppSecretPrevious = $null
  $containerEnvironment = $null
  $hostEnvironment = $null
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
