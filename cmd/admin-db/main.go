package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/databaseevolution"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/module/mail"

	mysqldriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type commandDependencies struct {
	getenv                 func(string) string
	openDatabase           func(string) (*sql.DB, error)
	capture                func(context.Context, *sql.DB, string) (databaseevolution.Fingerprint, error)
	write                  func(string, databaseevolution.FingerprintDocument) error
	runInvariants          func(context.Context, *sql.DB, string) (databaseevolution.InvariantResult, error)
	verifyCOSReferences    func(context.Context, *sql.DB, string) ([]databaseevolution.COSReferenceResult, error)
	writeCOSManifest       func(string, []databaseevolution.COSReferenceResult) error
	queryManifestFiles     func(string) ([]string, error)
	withAdvisoryLock       func(context.Context, *sql.DB, string, time.Duration, func(*sql.Conn) error) error
	captureConnection      func(context.Context, *sql.Conn, string) (databaseevolution.Fingerprint, error)
	runExternal            func(context.Context, []string) error
	runMailDiagnosticRekey func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error)
	hashPassword           func(string) (string, error)
	stdout                 io.Writer
}

type fingerprintOptions struct {
	schema    string
	output    string
	gitCommit string
}

type invariantOptions struct {
	schema string
	file   string
}

type cosReferenceOptions struct {
	schema                  string
	output                  string
	allowClassifiedNotFound bool
}

type queryManifestOptions struct {
	manifest string
}

type lockRunOptions struct {
	schema              string
	name                string
	timeout             time.Duration
	expectedFingerprint string
	command             []string
}

type createAdminOptions struct {
	username string
	email    string
	roleID   int64
}

type commandError struct {
	operation string
	cause     error
}

func (err *commandError) Error() string {
	switch {
	case errors.Is(err.cause, context.Canceled):
		return err.operation + ": canceled"
	case errors.Is(err.cause, context.DeadlineExceeded):
		return err.operation + ": deadline exceeded"
	default:
		return err.operation + ": failed"
	}
}

func (err *commandError) Unwrap() error {
	return err.cause
}

type singleStringFlag struct {
	name      string
	value     *string
	set       bool
	duplicate bool
}

func (value *singleStringFlag) String() string {
	if value == nil || value.value == nil {
		return ""
	}
	return *value.value
}

func (value *singleStringFlag) Set(input string) error {
	if value.set {
		value.duplicate = true
		return fmt.Errorf("duplicate flag")
	}
	value.set = true
	*value.value = input
	return nil
}

func main() {
	dependencies := commandDependencies{
		getenv:                 os.Getenv,
		openDatabase:           func(dsn string) (*sql.DB, error) { return sql.Open("mysql", dsn) },
		capture:                databaseevolution.Capture,
		write:                  databaseevolution.WriteFingerprintDocument,
		runInvariants:          databaseevolution.RunInvariantFile,
		verifyCOSReferences:    databaseevolution.VerifyStoredCOSReferences,
		writeCOSManifest:       databaseevolution.WriteCOSReferenceManifest,
		queryManifestFiles:     loadQueryManifestFiles,
		withAdvisoryLock:       databaseevolution.WithAdvisoryLockConnection,
		captureConnection:      databaseevolution.CaptureConnection,
		runExternal:            runExternalCommand,
		runMailDiagnosticRekey: runMailDiagnosticRekeyProduction,
		hashPassword: func(password string) (string, error) {
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			return string(hash), err
		},
		stdout: os.Stdout,
	}
	if err := run(context.Background(), os.Args[1:], dependencies); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, dependencies commandDependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: admin-db <create-admin|fingerprint|invariants|cos-references|query-manifest|lock-run|mail-diagnostic-rekey> [arguments]")
	}
	switch args[0] {
	case "create-admin":
		options, err := parseCreateAdminOptions(args[1:])
		if err != nil {
			return err
		}
		return runCreateAdmin(ctx, options, dependencies)
	case "fingerprint":
		options, err := parseFingerprintOptions(args[1:])
		if err != nil {
			return err
		}
		return runFingerprint(ctx, options, dependencies)
	case "invariants":
		options, err := parseInvariantOptions(args[1:])
		if err != nil {
			return err
		}
		return runInvariants(ctx, options, dependencies)
	case "cos-references":
		options, err := parseCOSReferenceOptions(args[1:])
		if err != nil {
			return err
		}
		return runCOSReferences(ctx, options, dependencies)
	case "query-manifest":
		if len(args) < 2 || args[1] != "files" {
			return fmt.Errorf("usage: admin-db query-manifest files --manifest <path>")
		}
		options, err := parseQueryManifestOptions(args[2:])
		if err != nil {
			return err
		}
		return runQueryManifestFiles(options, dependencies)
	case "lock-run":
		options, err := parseLockRunOptions(args[1:])
		if err != nil {
			return err
		}
		return runLockRun(ctx, options, dependencies)
	case "mail-diagnostic-rekey":
		if len(args) != 1 {
			return fmt.Errorf("usage: admin-db mail-diagnostic-rekey")
		}
		return runMailDiagnosticRekeyCommand(ctx, dependencies)
	default:
		return fmt.Errorf("unsupported subcommand")
	}
}

var createAdminEmailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func parseCreateAdminOptions(args []string) (createAdminOptions, error) {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options createAdminOptions
	var roleID string
	usernameFlag := &singleStringFlag{name: "username", value: &options.username}
	emailFlag := &singleStringFlag{name: "email", value: &options.email}
	roleIDFlag := &singleStringFlag{name: "role-id", value: &roleID}
	flags.Var(usernameFlag, "username", "administrator display name")
	flags.Var(emailFlag, "email", "administrator login email")
	flags.Var(roleIDFlag, "role-id", "administrator role ID")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{usernameFlag, emailFlag, roleIDFlag} {
			if value.duplicate {
				return createAdminOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return createAdminOptions{}, fmt.Errorf("invalid create-admin arguments")
	}
	if flags.NArg() != 0 {
		return createAdminOptions{}, fmt.Errorf("invalid create-admin arguments")
	}
	options.username = strings.TrimSpace(options.username)
	options.email = strings.ToLower(strings.TrimSpace(options.email))
	if options.username == "" {
		return createAdminOptions{}, fmt.Errorf("--username is required")
	}
	if strings.ContainsAny(options.username, "\r\n\t") {
		return createAdminOptions{}, fmt.Errorf("--username contains control characters")
	}
	if len([]rune(options.username)) > 50 {
		return createAdminOptions{}, fmt.Errorf("--username is too long")
	}
	if options.email == "" {
		return createAdminOptions{}, fmt.Errorf("--email is required")
	}
	if len(options.email) > 255 || !createAdminEmailPattern.MatchString(options.email) {
		return createAdminOptions{}, fmt.Errorf("--email is invalid")
	}
	parsedRoleID, err := strconv.ParseInt(strings.TrimSpace(roleID), 10, 64)
	if err != nil || parsedRoleID != 2 {
		return createAdminOptions{}, fmt.Errorf("--role-id must be 2")
	}
	options.roleID = parsedRoleID
	return options, nil
}

func runCreateAdmin(ctx context.Context, options createAdminOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.hashPassword == nil || dependencies.stdout == nil {
		return fmt.Errorf("create-admin command dependencies are incomplete")
	}
	password := dependencies.getenv("ADMIN_INITIAL_PASSWORD")
	passwordLength := len([]rune(password))
	if passwordLength < 6 || passwordLength > 128 {
		return fmt.Errorf("ADMIN_INITIAL_PASSWORD is required and must contain 6 to 128 characters")
	}
	passwordHash, err := dependencies.hashPassword(password)
	password = ""
	if err != nil {
		return safeCommandError("hash initial administrator password", err)
	}

	dsn := strings.TrimSpace(dependencies.getenv("MYSQL_DSN"))
	if err := databaseevolution.ValidateSchemaDSN(dsn, "admin"); err != nil {
		return err
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return safeCommandError("begin create-admin transaction", err)
	}
	defer transaction.Rollback()

	var roleCount int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM roles WHERE id = ? AND is_del = ?", options.roleID, 2).Scan(&roleCount); err != nil {
		return safeCommandError("validate administrator role", err)
	}
	if roleCount != 1 {
		return fmt.Errorf("administrator role 2 is unavailable")
	}
	var userCount int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE email = ?", options.email).Scan(&userCount); err != nil {
		return safeCommandError("validate administrator email", err)
	}
	if userCount != 0 {
		return fmt.Errorf("administrator email already exists")
	}

	result, err := transaction.ExecContext(ctx,
		"INSERT INTO users (role_id, username, email, password, status, is_del) VALUES (?, ?, ?, ?, ?, ?)",
		options.roleID, options.username, options.email, passwordHash, 1, 2,
	)
	if err != nil {
		return safeCommandError("create administrator user", err)
	}
	userID, err := result.LastInsertId()
	if err != nil || userID <= 0 {
		return safeCommandError("read administrator user ID", err)
	}
	if _, err := transaction.ExecContext(ctx,
		"INSERT INTO user_profiles (user_id, sex, is_del) VALUES (?, ?, ?)", userID, 0, 2,
	); err != nil {
		return safeCommandError("create administrator profile", err)
	}
	if _, err := transaction.ExecContext(ctx,
		"INSERT INTO authz_principal_versions (user_id, platform, version) VALUES (?, ?, ?)", userID, "admin", 1,
	); err != nil {
		return safeCommandError("create administrator authorization version", err)
	}
	if err := transaction.Commit(); err != nil {
		return safeCommandError("commit create-admin transaction", err)
	}
	if _, err := fmt.Fprintf(dependencies.stdout, "created_admin\t%d\t%s\n", userID, options.username); err != nil {
		return fmt.Errorf("print created administrator: %w", err)
	}
	return nil
}

func runMailDiagnosticRekeyCommand(ctx context.Context, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.runMailDiagnosticRekey == nil || dependencies.stdout == nil {
		return safeMailDiagnosticRekeyCommandError(errors.New("command dependencies are incomplete"))
	}
	if ctx == nil {
		ctx = context.Background()
	}

	dsn := strings.TrimSpace(dependencies.getenv("MYSQL_DSN"))
	parsedDSN, err := mysqldriver.ParseDSN(dsn)
	if err != nil || strings.TrimSpace(parsedDSN.DBName) == "" {
		return safeMailDiagnosticRekeyCommandError(errors.New("MYSQL_DSN is invalid"))
	}
	currentRoot := dependencies.getenv("APP_SECRET")
	previousRoot := dependencies.getenv("APP_SECRET_PREVIOUS")
	previousRoots := []string(nil)
	if strings.TrimSpace(previousRoot) != "" {
		previousRoots = []string{previousRoot}
	}
	if err := config.ValidateRuntimeSecrets(config.Config{App: config.AppConfig{
		Secret: currentRoot, PreviousSecrets: previousRoots,
	}}); err != nil {
		return safeMailDiagnosticRekeyCommandError(err)
	}

	observer := func(id uint64) error {
		if _, err := fmt.Fprintf(dependencies.stdout, "rekeyed_row_id\t%d\n", id); err != nil {
			return mail.ErrDiagnosticRekeyOutputFailed
		}
		return nil
	}
	result, err := dependencies.runMailDiagnosticRekey(ctx, dsn, currentRoot, previousRoot, observer)
	if err != nil {
		return safeMailDiagnosticRekeyCommandError(err)
	}
	if !safeMailDiagnosticRekeyResult(result) {
		return safeMailDiagnosticRekeyCommandError(mail.ErrDiagnosticRekeyIncomplete)
	}

	lines := []struct {
		name  string
		value any
	}{
		{name: "current_key_id", value: result.CurrentKeyID},
		{name: "previous_key_id", value: result.PreviousKeyID},
		{name: "scanned", value: result.Scanned},
		{name: "rekeyed", value: result.Rekeyed},
		{name: "previous_references", value: result.PreviousReferences},
		{name: "unknown_references", value: result.UnknownReferences},
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(dependencies.stdout, "%s\t%v\n", line.name, line.value); err != nil {
			return safeMailDiagnosticRekeyCommandError(mail.ErrDiagnosticRekeyOutputFailed)
		}
	}
	return nil
}

func safeMailDiagnosticRekeyResult(result mail.DiagnosticRekeyResult) bool {
	if !mail.IsCanonicalDiagnosticKeyID(result.CurrentKeyID) {
		return false
	}
	if result.PreviousKeyID != "" {
		if !mail.IsCanonicalDiagnosticKeyID(result.PreviousKeyID) || result.PreviousKeyID == result.CurrentKeyID {
			return false
		}
	}
	return result.Rekeyed <= result.Scanned && result.PreviousReferences == 0 && result.UnknownReferences == 0
}

func safeMailDiagnosticRekeyCommandError(err error) error {
	return safeCommandError("mail diagnostic rekey command", safeMailDiagnosticRekeyCause(err))
}

func safeMailDiagnosticRekeyCause(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	for _, sentinel := range []error{
		mail.ErrDiagnosticRekeyRepositoryNotConfigured,
		mail.ErrDiagnosticRekeyRepositoryFailure,
		mail.ErrDiagnosticRekeyLockUnavailable,
		mail.ErrDiagnosticRekeyUnknownKey,
		mail.ErrDiagnosticRekeyCorruptCipher,
		mail.ErrDiagnosticRekeyOptimisticCompareFailed,
		mail.ErrDiagnosticRekeyOutputFailed,
		mail.ErrDiagnosticRekeyIncomplete,
	} {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return mail.ErrDiagnosticRekeyRepositoryFailure
}

func runMailDiagnosticRekeyProduction(ctx context.Context, dsn, currentRoot, previousRoot string, observer mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
	previousRoots := []string(nil)
	if strings.TrimSpace(previousRoot) != "" {
		previousRoots = []string{previousRoot}
	}
	ring, err := secretkey.NewKeyRingWithPrevious(currentRoot, previousRoots)
	if err != nil {
		return mail.DiagnosticRekeyResult{}, mail.ErrDiagnosticRekeyCorruptCipher
	}
	diagnosticBox, err := secretbox.NewVersioned(ring.MailDiagnosticKeyID(), ring.MailDiagnosticDecryptionKeys())
	if err != nil {
		return mail.DiagnosticRekeyResult{}, mail.ErrDiagnosticRekeyCorruptCipher
	}
	previousKeyID := ""
	if len(previousRoots) == 1 {
		previousRing, previousErr := secretkey.NewKeyRing(previousRoots[0])
		if previousErr != nil {
			return mail.DiagnosticRekeyResult{}, mail.ErrDiagnosticRekeyUnknownKey
		}
		previousKeyID = previousRing.MailDiagnosticKeyID()
	}

	client, err := database.Open(config.MySQLConfig{
		DSN: dsn, MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		return mail.DiagnosticRekeyResult{}, mail.ErrDiagnosticRekeyRepositoryFailure
	}
	service := mail.NewDiagnosticRekeyService(
		mail.NewGormDiagnosticRekeyRepository(client), diagnosticBox, previousKeyID, observer,
	)
	result, runErr := service.Run(ctx)
	closeErr := client.Close()
	if runErr != nil {
		return result, runErr
	}
	if closeErr != nil {
		return result, mail.ErrDiagnosticRekeyRepositoryFailure
	}
	return result, nil
}

var advisoryLockNamePattern = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,128}$`)

func parseLockRunOptions(args []string) (lockRunOptions, error) {
	flags := flag.NewFlagSet("lock-run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options lockRunOptions
	var timeoutValue string
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	nameFlag := &singleStringFlag{name: "name", value: &options.name}
	timeoutFlag := &singleStringFlag{name: "timeout", value: &timeoutValue}
	fingerprintFlag := &singleStringFlag{name: "expected-fingerprint", value: &options.expectedFingerprint}
	flags.Var(schemaFlag, "schema", "schema used for the lock connection")
	flags.Var(nameFlag, "name", "MySQL advisory lock name")
	flags.Var(timeoutFlag, "timeout", "lock acquisition timeout")
	flags.Var(fingerprintFlag, "expected-fingerprint", "expected source schema SHA-256")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, nameFlag, timeoutFlag, fingerprintFlag} {
			if value.duplicate {
				return lockRunOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return lockRunOptions{}, fmt.Errorf("invalid lock-run arguments")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.name = strings.TrimSpace(options.name)
	options.expectedFingerprint = strings.TrimSpace(options.expectedFingerprint)
	if options.schema == "" {
		return lockRunOptions{}, fmt.Errorf("--schema is required")
	}
	if !advisoryLockNamePattern.MatchString(options.name) {
		return lockRunOptions{}, fmt.Errorf("--name is invalid")
	}
	if strings.TrimSpace(timeoutValue) == "" {
		return lockRunOptions{}, fmt.Errorf("--timeout is required")
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil || timeout < time.Second || timeout > 5*time.Minute {
		return lockRunOptions{}, fmt.Errorf("--timeout must be between 1s and 5m")
	}
	options.timeout = timeout
	if len(options.expectedFingerprint) != 64 || strings.ToLower(options.expectedFingerprint) != options.expectedFingerprint {
		return lockRunOptions{}, fmt.Errorf("--expected-fingerprint must be a lowercase SHA-256")
	}
	if _, err := hex.DecodeString(options.expectedFingerprint); err != nil {
		return lockRunOptions{}, fmt.Errorf("--expected-fingerprint must be a lowercase SHA-256")
	}
	options.command = append([]string(nil), flags.Args()...)
	if len(options.command) == 0 || strings.TrimSpace(options.command[0]) == "" {
		return lockRunOptions{}, fmt.Errorf("external command is required after --")
	}
	return options, nil
}

func parseFingerprintOptions(args []string) (fingerprintOptions, error) {
	flags := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options fingerprintOptions
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	outputFlag := &singleStringFlag{name: "out", value: &options.output}
	commitFlag := &singleStringFlag{name: "commit", value: &options.gitCommit}
	flags.Var(schemaFlag, "schema", "schema to fingerprint")
	flags.Var(outputFlag, "out", "output JSON path")
	flags.Var(commitFlag, "commit", "Git commit")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, outputFlag, commitFlag} {
			if value.duplicate {
				return fingerprintOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return fingerprintOptions{}, fmt.Errorf("invalid fingerprint arguments")
	}
	if flags.NArg() != 0 {
		return fingerprintOptions{}, fmt.Errorf("unexpected argument")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.output = strings.TrimSpace(options.output)
	options.gitCommit = strings.TrimSpace(options.gitCommit)
	if options.schema == "" {
		return fingerprintOptions{}, fmt.Errorf("--schema is required")
	}
	if options.output == "" {
		return fingerprintOptions{}, fmt.Errorf("--out is required")
	}
	if options.gitCommit == "" {
		return fingerprintOptions{}, fmt.Errorf("--commit is required")
	}
	if !isFullGitObjectID(options.gitCommit) {
		return fingerprintOptions{}, fmt.Errorf("--commit must be a full Git object ID")
	}
	return options, nil
}

func parseInvariantOptions(args []string) (invariantOptions, error) {
	flags := flag.NewFlagSet("invariants", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options invariantOptions
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	fileFlag := &singleStringFlag{name: "file", value: &options.file}
	flags.Var(schemaFlag, "schema", "schema to verify")
	flags.Var(fileFlag, "file", "invariant SQL file")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, fileFlag} {
			if value.duplicate {
				return invariantOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return invariantOptions{}, fmt.Errorf("invalid invariants arguments")
	}
	if flags.NArg() != 0 {
		return invariantOptions{}, fmt.Errorf("unexpected argument")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.file = strings.TrimSpace(options.file)
	if options.schema == "" {
		return invariantOptions{}, fmt.Errorf("--schema is required")
	}
	if options.file == "" {
		return invariantOptions{}, fmt.Errorf("--file is required")
	}
	return options, nil
}

func parseCOSReferenceOptions(args []string) (cosReferenceOptions, error) {
	flags := flag.NewFlagSet("cos-references", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options cosReferenceOptions
	schemaFlag := &singleStringFlag{name: "schema", value: &options.schema}
	outputFlag := &singleStringFlag{name: "out", value: &options.output}
	flags.Var(schemaFlag, "schema", "schema containing COS references")
	flags.Var(outputFlag, "out", "evidence manifest path")
	flags.BoolVar(&options.allowClassifiedNotFound, "allow-classified-not-found", false, "allow previously classified not_found references while still failing dependency errors")
	if err := flags.Parse(args); err != nil {
		for _, value := range []*singleStringFlag{schemaFlag, outputFlag} {
			if value.duplicate {
				return cosReferenceOptions{}, fmt.Errorf("--%s may be provided only once", value.name)
			}
		}
		return cosReferenceOptions{}, fmt.Errorf("invalid COS reference arguments")
	}
	if flags.NArg() != 0 {
		return cosReferenceOptions{}, fmt.Errorf("unexpected argument")
	}
	options.schema = strings.TrimSpace(options.schema)
	options.output = strings.TrimSpace(options.output)
	if options.schema == "" {
		return cosReferenceOptions{}, fmt.Errorf("--schema is required")
	}
	if options.output == "" {
		return cosReferenceOptions{}, fmt.Errorf("--out is required")
	}
	return options, nil
}

func parseQueryManifestOptions(args []string) (queryManifestOptions, error) {
	flags := flag.NewFlagSet("query-manifest files", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options queryManifestOptions
	manifestFlag := &singleStringFlag{name: "manifest", value: &options.manifest}
	flags.Var(manifestFlag, "manifest", "query candidate manifest")
	if err := flags.Parse(args); err != nil {
		if manifestFlag.duplicate {
			return queryManifestOptions{}, fmt.Errorf("--manifest may be provided only once")
		}
		return queryManifestOptions{}, fmt.Errorf("invalid query-manifest arguments")
	}
	if flags.NArg() != 0 {
		return queryManifestOptions{}, fmt.Errorf("unexpected argument")
	}
	options.manifest = strings.TrimSpace(options.manifest)
	if options.manifest == "" {
		return queryManifestOptions{}, fmt.Errorf("--manifest is required")
	}
	return options, nil
}

func isFullGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func runFingerprint(ctx context.Context, options fingerprintOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.capture == nil || dependencies.write == nil || dependencies.stdout == nil {
		return fmt.Errorf("fingerprint command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	fingerprint, err := dependencies.capture(ctx, database, options.schema)
	if err != nil {
		return safeCommandError("capture schema fingerprint", err)
	}
	document, err := databaseevolution.NewFingerprintDocument(options.gitCommit, fingerprint)
	if err != nil {
		return err
	}
	if err := dependencies.write(options.output, document); err != nil {
		return safeCommandError("write fingerprint document", err)
	}
	if _, err := fmt.Fprintln(dependencies.stdout, options.output); err != nil {
		return fmt.Errorf("print fingerprint output path: %w", err)
	}
	if _, err := fmt.Fprintln(dependencies.stdout, document.SchemaSHA256); err != nil {
		return fmt.Errorf("print fingerprint schema SHA-256: %w", err)
	}
	return nil
}

func runInvariants(ctx context.Context, options invariantOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.runInvariants == nil || dependencies.stdout == nil {
		return fmt.Errorf("invariants command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	result, runErr := dependencies.runInvariants(ctx, database, options.file)
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(dependencies.stdout, "%s\t%d\n", check.Name, check.Violations); err != nil {
			return fmt.Errorf("print invariant result: %w", err)
		}
	}
	if runErr != nil {
		return safeCommandError("run database invariants", runErr)
	}
	return nil
}

func runCOSReferences(ctx context.Context, options cosReferenceOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.verifyCOSReferences == nil || dependencies.writeCOSManifest == nil || dependencies.stdout == nil {
		return fmt.Errorf("COS reference command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	rootSecret := dependencies.getenv("APP_SECRET")
	if strings.TrimSpace(rootSecret) == "" {
		return fmt.Errorf("APP_SECRET is required")
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()

	results, err := dependencies.verifyCOSReferences(ctx, database, rootSecret)
	if err != nil {
		return safeCommandError("verify COS references", err)
	}
	if err := dependencies.writeCOSManifest(options.output, results); err != nil {
		return safeCommandError("write COS reference manifest", err)
	}
	counts := map[string]int{
		databaseevolution.COSReferenceReachable:  0,
		databaseevolution.COSReferenceNotFound:   0,
		databaseevolution.COSReferenceDependency: 0,
	}
	for _, result := range results {
		counts[result.Status]++
	}
	if _, err := fmt.Fprintln(dependencies.stdout, options.output); err != nil {
		return fmt.Errorf("print COS reference manifest path: %w", err)
	}
	for _, status := range []string{databaseevolution.COSReferenceReachable, databaseevolution.COSReferenceNotFound, databaseevolution.COSReferenceDependency} {
		if _, err := fmt.Fprintf(dependencies.stdout, "%s\t%d\n", status, counts[status]); err != nil {
			return fmt.Errorf("print COS reference summary: %w", err)
		}
	}
	if counts[databaseevolution.COSReferenceDependency] != 0 ||
		(!options.allowClassifiedNotFound && counts[databaseevolution.COSReferenceNotFound] != 0) {
		return safeCommandError("verify COS references", errors.New("one or more COS references are not reachable"))
	}
	return nil
}

func runQueryManifestFiles(options queryManifestOptions, dependencies commandDependencies) error {
	if dependencies.queryManifestFiles == nil || dependencies.stdout == nil {
		return fmt.Errorf("query-manifest command dependencies are incomplete")
	}
	files, err := dependencies.queryManifestFiles(options.manifest)
	if err != nil {
		return safeCommandError("validate query manifest", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("query manifest produced no repository files")
	}
	for _, file := range files {
		if _, err := fmt.Fprintln(dependencies.stdout, file); err != nil {
			return fmt.Errorf("print query manifest file: %w", err)
		}
	}
	return nil
}

func runLockRun(ctx context.Context, options lockRunOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.withAdvisoryLock == nil || dependencies.captureConnection == nil || dependencies.runExternal == nil {
		return fmt.Errorf("lock-run command dependencies are incomplete")
	}
	dsn := dependencies.getenv("MYSQL_DSN")
	if err := databaseevolution.ValidateSchemaDSN(dsn, options.schema); err != nil {
		return err
	}
	database, err := dependencies.openDatabase(dsn)
	if err != nil {
		return safeCommandError("open MySQL connection", err)
	}
	defer database.Close()
	if err := dependencies.withAdvisoryLock(ctx, database, options.name, options.timeout, func(connection *sql.Conn) error {
		fingerprint, err := dependencies.captureConnection(ctx, connection, options.schema)
		if err != nil {
			return fmt.Errorf("capture locked schema fingerprint: %w", err)
		}
		actualFingerprint, err := databaseevolution.SchemaSHA256(fingerprint)
		if err != nil {
			return err
		}
		if actualFingerprint != options.expectedFingerprint {
			return errors.New("source schema fingerprint does not match expected value")
		}
		return dependencies.runExternal(ctx, options.command)
	}); err != nil {
		return safeCommandError("run command under database lock", err)
	}
	return nil
}

func runExternalCommand(ctx context.Context, command []string) error {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return errors.New("external command is required")
	}
	process := exec.CommandContext(ctx, command[0], command[1:]...)
	process.Stdin = nil
	process.Stdout = io.Discard
	process.Stderr = io.Discard
	return process.Run()
}

func loadQueryManifestFiles(path string) ([]string, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	candidates, err := databaseevolution.LoadQueryManifest(path, root)
	if err != nil {
		return nil, err
	}
	return databaseevolution.QueryManifestFiles(candidates), nil
}

func safeCommandError(operation string, err error) error {
	return &commandError{operation: operation, cause: err}
}
