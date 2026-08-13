package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"

	"admin_back_go/internal/module/mail"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunCreateAdminRequiresExplicitSafeInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing username", args: []string{"create-admin", "--email", "admin@example.com", "--role-id", "2"}, want: "--username is required"},
		{name: "control character username", args: []string{"create-admin", "--username", "admin\nforged", "--email", "admin@example.com", "--role-id", "2"}, want: "--username contains control characters"},
		{name: "missing email", args: []string{"create-admin", "--username", "admin", "--role-id", "2"}, want: "--email is required"},
		{name: "invalid email", args: []string{"create-admin", "--username", "admin", "--email", "not-an-email", "--role-id", "2"}, want: "--email is invalid"},
		{name: "wrong role", args: []string{"create-admin", "--username", "admin", "--email", "admin@example.com", "--role-id", "1"}, want: "--role-id must be 2"},
		{name: "password argument", args: []string{"create-admin", "--username", "admin", "--email", "admin@example.com", "--role-id", "2", "--password", "secret"}, want: "invalid create-admin arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), test.args, commandDependencies{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run() error=%v want text %q", err, test.want)
			}
		})
	}
}

func TestRunCreateAdminWritesOwnedRowsInOneTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM roles").WithArgs(int64(2), 2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").WithArgs("admin@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO users").
		WithArgs(int64(2), "Local Admin", "admin@example.com", "$2a$10$fixture", 1, 2).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec("INSERT INTO user_profiles").WithArgs(int64(9), 0, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO authz_principal_versions").WithArgs(int64(9), "admin", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectClose()

	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	var output bytes.Buffer
	dependencies := commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "ADMIN_INITIAL_PASSWORD":
				return "secret12"
			default:
				t.Fatalf("unexpected environment key %q", key)
				return ""
			}
		},
		openDatabase: func(got string) (*sql.DB, error) {
			if got != dsn {
				t.Fatal("MYSQL_DSN changed")
			}
			return database, nil
		},
		hashPassword: func(password string) (string, error) {
			if password != "secret12" {
				t.Fatal("initial password changed")
			}
			return "$2a$10$fixture", nil
		},
		stdout: &output,
	}

	err = run(context.Background(), []string{
		"create-admin", "--username", "Local Admin", "--email", "admin@example.com", "--role-id", "2",
	}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "created_admin\t9\tLocal Admin\n" {
		t.Fatalf("stdout=%q", output.String())
	}
	if strings.Contains(output.String(), "secret12") || strings.Contains(output.String(), "$2a$") {
		t.Fatal("create-admin output exposed password material")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCreateAdminRejectsMissingPasswordBeforeOpeningDatabase(t *testing.T) {
	opened := false
	err := run(context.Background(), []string{
		"create-admin", "--username", "admin", "--email", "admin@example.com", "--role-id", "2",
	}, commandDependencies{
		getenv: func(string) string { return "" },
		openDatabase: func(string) (*sql.DB, error) {
			opened = true
			return nil, errors.New("must not open")
		},
		hashPassword: func(string) (string, error) { return "", errors.New("must not hash") },
		stdout:       io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "ADMIN_INITIAL_PASSWORD is required") {
		t.Fatalf("run() error=%v", err)
	}
	if opened {
		t.Fatal("database opened without an initial password")
	}
}

func TestRunCreateAdminRequiresAdminSchema(t *testing.T) {
	opened := false
	err := run(context.Background(), []string{
		"create-admin", "--username", "admin", "--email", "admin@example.com", "--role-id", "2",
	}, commandDependencies{
		getenv: func(key string) string {
			if key == "ADMIN_INITIAL_PASSWORD" {
				return "secret12"
			}
			return "admin_user:password@tcp(127.0.0.1:3306)/other"
		},
		openDatabase: func(string) (*sql.DB, error) {
			opened = true
			return nil, errors.New("must not open")
		},
		hashPassword: func(string) (string, error) { return "$2a$10$fixture", nil },
		stdout:       io.Discard,
	})
	if err == nil || err.Error() != "MYSQL_DSN must target admin schema" || opened {
		t.Fatalf("run() error=%v opened=%v", err, opened)
	}
}

func TestRunRejectsRetiredDatabaseEvolutionCommands(t *testing.T) {
	for _, command := range []string{
		"fingerprint",
		"invariants",
		"cos-references",
		"query-manifest",
		"lock-run",
	} {
		t.Run(command, func(t *testing.T) {
			err := run(context.Background(), []string{command}, commandDependencies{})
			if err == nil || err.Error() != "unsupported subcommand" {
				t.Fatalf("run(%q) error = %v, want unsupported subcommand", command, err)
			}
		})
	}
}

func TestMailDiagnosticRekeyCommandRejectsArgumentsWithoutRunning(t *testing.T) {
	called := false
	err := run(context.Background(), []string{"mail-diagnostic-rekey", "secret"}, commandDependencies{
		getenv: func(string) string { return "" },
		runMailDiagnosticRekey: func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			called = true
			return mail.DiagnosticRekeyResult{}, nil
		},
		stdout: io.Discard,
	})
	if err == nil || err.Error() != "usage: admin-db mail-diagnostic-rekey" || called {
		t.Fatalf("invalid arguments error=%v called=%v", err, called)
	}
}

func TestMailDiagnosticRekeyCommandRunsAndPrintsSafeFields(t *testing.T) {
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	currentRoot := strings.Repeat("a", 64)
	previousRoot := strings.Repeat("b", 64)
	const currentID = "mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA"
	const previousID = "mail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBQ"
	var output bytes.Buffer
	err := run(context.Background(), []string{"mail-diagnostic-rekey"}, commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return currentRoot
			case "APP_SECRET_PREVIOUS":
				return previousRoot
			default:
				return ""
			}
		},
		runMailDiagnosticRekey: func(_ context.Context, gotDSN, gotCurrent, gotPrevious string, observer mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			if gotDSN != dsn || gotCurrent != currentRoot || gotPrevious != previousRoot {
				t.Fatal("runner inputs changed")
			}
			if err := observer(7); err != nil {
				return mail.DiagnosticRekeyResult{}, err
			}
			return mail.DiagnosticRekeyResult{
				CurrentKeyID: currentID, PreviousKeyID: previousID,
				Scanned: 1, Rekeyed: 1,
			}, nil
		},
		stdout: &output,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "rekeyed_row_id\t7\n" +
		"current_key_id\t" + currentID + "\n" +
		"previous_key_id\t" + previousID + "\n" +
		"scanned\t1\nrekeyed\t1\nprevious_references\t0\nunknown_references\t0\n"
	if output.String() != want {
		t.Fatalf("stdout=%q", output.String())
	}
	for _, secret := range []string{dsn, currentRoot, previousRoot, "safe-password"} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("mail diagnostic output exposed secret material")
		}
	}
}

func TestMailDiagnosticRekeyCommandRedactsRunnerFailure(t *testing.T) {
	const dsn = "admin_user:marker-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	root := strings.Repeat("c", 64)
	providerErr := errors.New("marker-provider-error-with-secret-material")
	err := run(context.Background(), []string{"mail-diagnostic-rekey"}, commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return root
			default:
				return ""
			}
		},
		runMailDiagnosticRekey: func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			return mail.DiagnosticRekeyResult{}, providerErr
		},
		stdout: io.Discard,
	})
	if err == nil || err.Error() != "mail diagnostic rekey command: failed" || errors.Is(err, providerErr) || !errors.Is(err, mail.ErrDiagnosticRekeyRepositoryFailure) {
		t.Fatalf("runner failure was not reduced to a safe sentinel: %v", err)
	}
	for _, secret := range []string{dsn, root, providerErr.Error()} {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("mail diagnostic error exposed secret material")
		}
	}
}
