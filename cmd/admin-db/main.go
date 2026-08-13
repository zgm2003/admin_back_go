package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/config"
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
	runMailDiagnosticRekey func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error)
	hashPassword           func(string) (string, error)
	stdout                 io.Writer
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
		return errors.New("duplicate flag")
	}
	value.set = true
	*value.value = input
	return nil
}

func main() {
	dependencies := commandDependencies{
		getenv:                 os.Getenv,
		openDatabase:           func(dsn string) (*sql.DB, error) { return sql.Open("mysql", dsn) },
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
		return errors.New("usage: admin-db <create-admin|mail-diagnostic-rekey> [arguments]")
	}
	switch args[0] {
	case "create-admin":
		options, err := parseCreateAdminOptions(args[1:])
		if err != nil {
			return err
		}
		return runCreateAdmin(ctx, options, dependencies)
	case "mail-diagnostic-rekey":
		if len(args) != 1 {
			return errors.New("usage: admin-db mail-diagnostic-rekey")
		}
		return runMailDiagnosticRekeyCommand(ctx, dependencies)
	default:
		return errors.New("unsupported subcommand")
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
		return createAdminOptions{}, errors.New("invalid create-admin arguments")
	}
	if flags.NArg() != 0 {
		return createAdminOptions{}, errors.New("invalid create-admin arguments")
	}
	options.username = strings.TrimSpace(options.username)
	options.email = strings.ToLower(strings.TrimSpace(options.email))
	if options.username == "" {
		return createAdminOptions{}, errors.New("--username is required")
	}
	if strings.ContainsAny(options.username, "\r\n\t") {
		return createAdminOptions{}, errors.New("--username contains control characters")
	}
	if len([]rune(options.username)) > 50 {
		return createAdminOptions{}, errors.New("--username is too long")
	}
	if options.email == "" {
		return createAdminOptions{}, errors.New("--email is required")
	}
	if len(options.email) > 255 || !createAdminEmailPattern.MatchString(options.email) {
		return createAdminOptions{}, errors.New("--email is invalid")
	}
	parsedRoleID, err := strconv.ParseInt(strings.TrimSpace(roleID), 10, 64)
	if err != nil || parsedRoleID != 2 {
		return createAdminOptions{}, errors.New("--role-id must be 2")
	}
	options.roleID = parsedRoleID
	return options, nil
}

func validateAdminDSN(dsn string) error {
	parsed, err := mysqldriver.ParseDSN(strings.TrimSpace(dsn))
	if err != nil || parsed.DBName != "admin" {
		return errors.New("MYSQL_DSN must target admin schema")
	}
	return nil
}

func runCreateAdmin(ctx context.Context, options createAdminOptions, dependencies commandDependencies) error {
	if dependencies.getenv == nil || dependencies.openDatabase == nil || dependencies.hashPassword == nil || dependencies.stdout == nil {
		return errors.New("create-admin command dependencies are incomplete")
	}
	password := dependencies.getenv("ADMIN_INITIAL_PASSWORD")
	passwordLength := len([]rune(password))
	if passwordLength < 6 || passwordLength > 128 {
		return errors.New("ADMIN_INITIAL_PASSWORD is required and must contain 6 to 128 characters")
	}
	passwordHash, err := dependencies.hashPassword(password)
	password = ""
	if err != nil {
		return safeCommandError("hash initial administrator password", err)
	}

	dsn := strings.TrimSpace(dependencies.getenv("MYSQL_DSN"))
	if err := validateAdminDSN(dsn); err != nil {
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
		return errors.New("administrator role 2 is unavailable")
	}
	var userCount int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE email = ?", options.email).Scan(&userCount); err != nil {
		return safeCommandError("validate administrator email", err)
	}
	if userCount != 0 {
		return errors.New("administrator email already exists")
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

func safeCommandError(operation string, err error) error {
	return &commandError{operation: operation, cause: err}
}
