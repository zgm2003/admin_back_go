package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/databaseevolution"
	"admin_back_go/internal/module/mail"

	"github.com/DATA-DOG/go-sqlmock"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestRunRejectsIncompleteAndUnexpectedFingerprintArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing subcommand", args: nil, want: "usage"},
		{name: "unknown subcommand", args: []string{"unknown"}, want: "unsupported subcommand"},
		{name: "missing schema", args: []string{"fingerprint", "--out", "admin.json", "--commit", testCommit}, want: "--schema is required"},
		{name: "missing output", args: []string{"fingerprint", "--schema", "admin", "--commit", testCommit}, want: "--out is required"},
		{name: "missing commit", args: []string{"fingerprint", "--schema", "admin", "--out", "admin.json"}, want: "--commit is required"},
		{name: "invalid commit", args: []string{"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", "not-a-git-commit"}, want: "--commit must be a full Git object ID"},
		{name: "positional argument", args: []string{"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", testCommit, "extra"}, want: "unexpected argument"},
		{name: "duplicate schema", args: []string{"fingerprint", "--schema", "admin", "--schema", "other", "--out", "admin.json", "--commit", testCommit}, want: "--schema may be provided only once"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), test.args, commandDependencies{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run() error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestRunArgumentErrorsDoNotReflectInputValues(t *testing.T) {
	const secret = "admin_user:super-secret@tcp(127.0.0.1:3306)/admin"
	tests := [][]string{
		{secret},
		{"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", testCommit, secret},
		{"fingerprint", "--schema", "admin", "--schema", secret, "--out", "admin.json", "--commit", testCommit},
		{"fingerprint", "--super-secret"},
	}
	for _, args := range tests {
		err := run(context.Background(), args, commandDependencies{})
		if err == nil {
			t.Fatalf("run(%v) unexpectedly succeeded", args)
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "super-secret") {
			t.Fatalf("argument error reflected secret input: %v", err)
		}
	}
}

func TestRunFingerprintCapturesWritesAndPrintsOnlyPathAndHash(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()

	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	var output bytes.Buffer
	var writtenPath string
	var writtenDocument databaseevolution.FingerprintDocument
	dependencies := commandDependencies{
		getenv: func(key string) string {
			if key != "MYSQL_DSN" {
				t.Fatalf("unexpected environment key %q", key)
			}
			return dsn
		},
		openDatabase: func(gotDSN string) (*sql.DB, error) {
			if gotDSN != dsn {
				t.Fatalf("DSN changed before open")
			}
			return database, nil
		},
		capture: func(_ context.Context, gotDatabase *sql.DB, schema string) (databaseevolution.Fingerprint, error) {
			if gotDatabase != database || schema != "admin" {
				t.Fatalf("capture database=%p schema=%q", gotDatabase, schema)
			}
			return databaseevolution.Fingerprint{ServerVersion: "8.4.10", Schema: schema}, nil
		},
		write: func(path string, document databaseevolution.FingerprintDocument) error {
			writtenPath = path
			writtenDocument = document
			return nil
		},
		stdout: &output,
	}

	err = run(context.Background(), []string{
		"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", testCommit,
	}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if writtenPath != "admin.json" || writtenDocument.GitCommit != testCommit || writtenDocument.Schema != "admin" {
		t.Fatalf("write path=%q document=%+v", writtenPath, writtenDocument)
	}
	wantOutput := "admin.json\n" + writtenDocument.SchemaSHA256 + "\n"
	if output.String() != wantOutput {
		t.Fatalf("stdout=%q want %q", output.String(), wantOutput)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunFingerprintRedactsDSNAndPasswordFromErrors(t *testing.T) {
	const dsn = "admin_user:super-secret@tcp(127.0.0.1:3306)/admin?parseTime=true"
	dependencies := commandDependencies{
		getenv: func(string) string { return dsn },
		openDatabase: func(string) (*sql.DB, error) {
			return nil, errors.New("connection failed for " + dsn + " password=super-secret")
		},
		capture: func(context.Context, *sql.DB, string) (databaseevolution.Fingerprint, error) {
			return databaseevolution.Fingerprint{}, nil
		},
		write:  func(string, databaseevolution.FingerprintDocument) error { return nil },
		stdout: io.Discard,
	}

	err := run(context.Background(), []string{
		"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", testCommit,
	}, dependencies)
	if err == nil {
		t.Fatal("expected database open failure")
	}
	if strings.Contains(err.Error(), dsn) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked MYSQL_DSN credentials: %v", err)
	}
	if !strings.Contains(err.Error(), "open MySQL connection") {
		t.Fatalf("error lost safe context: %v", err)
	}
}

func TestRunFingerprintDoesNotExposeEncodedPasswordAndPreservesCause(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:p%40ss@tcp(127.0.0.1:3306)/admin?parseTime=true"
	sentinel := errors.New("provider echoed p%40ss and decoded p@ss from " + dsn)
	dependencies := commandDependencies{
		getenv:       func(string) string { return dsn },
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		capture: func(context.Context, *sql.DB, string) (databaseevolution.Fingerprint, error) {
			return databaseevolution.Fingerprint{}, sentinel
		},
		write:  func(string, databaseevolution.FingerprintDocument) error { return nil },
		stdout: io.Discard,
	}

	err = run(context.Background(), []string{
		"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", testCommit,
	}, dependencies)
	if err == nil {
		t.Fatal("expected capture failure")
	}
	for _, secret := range []string{dsn, "p%40ss", "p@ss"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %v", secret, err)
		}
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error chain lost capture cause: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunFingerprintDoesNotExposeWriteFailureAndPreservesCause(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:super-secret@tcp(127.0.0.1:3306)/admin?parseTime=true"
	sentinel := errors.New("writer echoed " + dsn)
	dependencies := commandDependencies{
		getenv:       func(string) string { return dsn },
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		capture: func(context.Context, *sql.DB, string) (databaseevolution.Fingerprint, error) {
			return databaseevolution.Fingerprint{Schema: "admin"}, nil
		},
		write:  func(string, databaseevolution.FingerprintDocument) error { return sentinel },
		stdout: io.Discard,
	}

	err = run(context.Background(), []string{
		"fingerprint", "--schema", "admin", "--out", "admin.json", "--commit", testCommit,
	}, dependencies)
	if err == nil {
		t.Fatal("expected write failure")
	}
	if strings.Contains(err.Error(), dsn) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked writer details: %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error chain lost write cause: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunInvariantsPrintsOnlyNamesAndCounts(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	var output bytes.Buffer
	dependencies := commandDependencies{
		getenv:       func(string) string { return dsn },
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		runInvariants: func(_ context.Context, gotDatabase *sql.DB, path string) (databaseevolution.InvariantResult, error) {
			if gotDatabase != database || path != "database/reconciliation/031_verify_relations.sql" {
				t.Fatalf("database=%p path=%q", gotDatabase, path)
			}
			return databaseevolution.InvariantResult{Checks: []databaseevolution.InvariantCheck{
				{Name: "rbac_orphans", Violations: 0},
				{Name: "payment_orphans", Violations: 0},
			}}, nil
		},
		stdout: &output,
	}

	err = run(context.Background(), []string{
		"invariants", "--schema", "admin", "--file", "database/reconciliation/031_verify_relations.sql",
	}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "rbac_orphans\t0\npayment_orphans\t0\n" {
		t.Fatalf("stdout=%q", output.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCOSReferencesPrintsOnlyManifestPathAndCounts(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	var output bytes.Buffer
	var written []databaseevolution.COSReferenceResult
	dependencies := commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return strings.Repeat("a", 64)
			default:
				t.Fatalf("unexpected environment key %q", key)
				return ""
			}
		},
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		verifyCOSReferences: func(context.Context, *sql.DB, string) ([]databaseevolution.COSReferenceResult, error) {
			return []databaseevolution.COSReferenceResult{
				{Key: "private/one.png", Status: databaseevolution.COSReferenceReachable},
				{Key: "private/two.png", Status: databaseevolution.COSReferenceNotFound},
			}, nil
		},
		writeCOSManifest: func(path string, results []databaseevolution.COSReferenceResult) error {
			if path != "cos-evidence.json" {
				t.Fatalf("path=%q", path)
			}
			written = append([]databaseevolution.COSReferenceResult(nil), results...)
			return nil
		},
		stdout: &output,
	}

	err = run(context.Background(), []string{
		"cos-references", "--schema", "admin", "--out", "cos-evidence.json",
	}, dependencies)
	if err == nil {
		t.Fatal("expected non-reachable reference failure")
	}
	if len(written) != 2 {
		t.Fatalf("written=%+v", written)
	}
	if output.String() != "cos-evidence.json\nreachable\t1\nnot_found\t1\ndependency\t0\n" {
		t.Fatalf("stdout=%q", output.String())
	}
	if strings.Contains(output.String(), "private/") {
		t.Fatalf("stdout leaked object keys: %q", output.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCOSReferencesAllowsClassifiedNotFoundWhenRequested(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	var output bytes.Buffer
	dependencies := commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return strings.Repeat("a", 64)
			default:
				t.Fatalf("unexpected environment key %q", key)
				return ""
			}
		},
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		verifyCOSReferences: func(context.Context, *sql.DB, string) ([]databaseevolution.COSReferenceResult, error) {
			return []databaseevolution.COSReferenceResult{
				{Key: "private/missing.png", Status: databaseevolution.COSReferenceNotFound},
			}, nil
		},
		writeCOSManifest: func(string, []databaseevolution.COSReferenceResult) error { return nil },
		stdout:           &output,
	}

	err = run(context.Background(), []string{
		"cos-references", "--schema", "admin", "--out", "cos-evidence.json", "--allow-classified-not-found",
	}, dependencies)
	if err != nil {
		t.Fatalf("expected classified not_found reference to pass: %v", err)
	}
	if output.String() != "cos-evidence.json\nreachable\t0\nnot_found\t1\ndependency\t0\n" {
		t.Fatalf("stdout=%q", output.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCOSReferencesAllowClassifiedNotFoundStillFailsDependency(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	var output bytes.Buffer
	dependencies := commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return strings.Repeat("a", 64)
			default:
				t.Fatalf("unexpected environment key %q", key)
				return ""
			}
		},
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		verifyCOSReferences: func(context.Context, *sql.DB, string) ([]databaseevolution.COSReferenceResult, error) {
			return []databaseevolution.COSReferenceResult{
				{Key: "private/dependency.png", Status: databaseevolution.COSReferenceDependency, DependencyClass: "provider"},
			}, nil
		},
		writeCOSManifest: func(string, []databaseevolution.COSReferenceResult) error { return nil },
		stdout:           &output,
	}

	err = run(context.Background(), []string{
		"cos-references", "--schema", "admin", "--out", "cos-evidence.json", "--allow-classified-not-found",
	}, dependencies)
	if err == nil {
		t.Fatal("expected dependency reference failure")
	}
	if output.String() != "cos-evidence.json\nreachable\t0\nnot_found\t0\ndependency\t1\n" {
		t.Fatalf("stdout=%q", output.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunQueryManifestFilesPrintsNormalizedUniquePaths(t *testing.T) {
	var output bytes.Buffer
	dependencies := commandDependencies{
		queryManifestFiles: func(path string) ([]string, error) {
			if path != "database/reconciliation/040_query_candidates.json" {
				t.Fatalf("path=%q", path)
			}
			return []string{"internal/module/ai/run/repository.go", "internal/module/auth/session.go"}, nil
		},
		stdout: &output,
	}
	err := run(context.Background(), []string{
		"query-manifest", "files", "--manifest", "database/reconciliation/040_query_candidates.json",
	}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "internal/module/ai/run/repository.go\ninternal/module/auth/session.go\n" {
		t.Fatalf("stdout=%q", output.String())
	}
}

func TestRunLockRunHoldsNamedLockWhileExecutingCommand(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	locked := false
	executed := false
	lockedFingerprint := databaseevolution.Fingerprint{}
	expectedFingerprint, err := databaseevolution.SchemaSHA256(lockedFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	var lockedConnection *sql.Conn
	dependencies := commandDependencies{
		getenv:       func(string) string { return dsn },
		openDatabase: func(string) (*sql.DB, error) { return database, nil },
		withAdvisoryLock: func(ctx context.Context, got *sql.DB, name string, timeout time.Duration, callback func(*sql.Conn) error) error {
			if got != database || name != "admin:atlas:migrate" || timeout != 30*time.Second {
				t.Fatalf("unexpected lock request database=%p name=%q timeout=%s", got, name, timeout)
			}
			connection, connectionErr := got.Conn(ctx)
			if connectionErr != nil {
				return connectionErr
			}
			defer connection.Close()
			lockedConnection = connection
			locked = true
			defer func() { locked = false }()
			return callback(connection)
		},
		captureConnection: func(_ context.Context, connection *sql.Conn, schema string) (databaseevolution.Fingerprint, error) {
			if !locked || connection == nil || connection != lockedConnection || schema != "admin" {
				t.Fatalf("fingerprint was not captured on the lock-owning connection")
			}
			return lockedFingerprint, nil
		},
		runExternal: func(_ context.Context, command []string) error {
			if !locked || !reflect.DeepEqual(command, []string{"docker", "run", "atlas"}) {
				t.Fatalf("external command executed without lock or changed: locked=%v command=%v", locked, command)
			}
			executed = true
			return nil
		},
		stdout: io.Discard,
	}
	if err := run(context.Background(), []string{"lock-run", "--schema", "admin", "--name", "admin:atlas:migrate", "--timeout", "30s", "--expected-fingerprint", expectedFingerprint, "--", "docker", "run", "atlas"}, dependencies); err != nil {
		t.Fatalf("lock-run returned error: %v", err)
	}
	if !executed || locked {
		t.Fatalf("lock lifecycle was incomplete: executed=%v locked=%v", executed, locked)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestParseLockRunRequiresExpectedFingerprint(t *testing.T) {
	validFingerprint := strings.Repeat("a", 64)
	if _, err := parseLockRunOptions([]string{
		"--schema", "admin",
		"--name", "admin:atlas:migrate",
		"--timeout", "30s",
		"--expected-fingerprint", validFingerprint,
		"--", "docker", "run", "atlas",
	}); err != nil {
		t.Fatalf("valid expected fingerprint was rejected: %v", err)
	}
	if _, err := parseLockRunOptions([]string{
		"--schema", "admin",
		"--name", "admin:atlas:migrate",
		"--timeout", "30s",
		"--", "docker", "run", "atlas",
	}); err == nil {
		t.Fatal("lock-run accepted a command without an expected source fingerprint")
	}
}

func TestMailDiagnosticRekeyCommandRejectsArgumentsWithoutRunning(t *testing.T) {
	const marker = "marker-mail-rekey-argument-secret"
	for _, args := range [][]string{
		{"mail-diagnostic-rekey", marker},
		{"mail-diagnostic-rekey", "--" + marker},
	} {
		called := false
		var output bytes.Buffer
		err := run(context.Background(), args, commandDependencies{
			getenv: func(string) string { return "" },
			runMailDiagnosticRekey: func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
				called = true
				return mail.DiagnosticRekeyResult{}, nil
			},
			stdout: &output,
		})
		if err == nil || err.Error() != "usage: admin-db mail-diagnostic-rekey" {
			t.Fatalf("mail diagnostic rekey arguments did not return fixed usage")
		}
		if called || output.Len() != 0 || strings.Contains(err.Error(), marker) {
			t.Fatalf("invalid arguments ran or exposed the command input")
		}
	}
}

func TestMailDiagnosticRekeyCommandValidatesEnvironmentRunsAndPrintsSafeFields(t *testing.T) {
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	currentRoot := strings.Repeat("a", 64)
	previousRoot := strings.Repeat("b", 64)
	const currentID = "mail-diagnostic-v1-AAAAAAAAAAAAAAAAAAAAAA"
	const previousID = "mail-diagnostic-v1-BBBBBBBBBBBBBBBBBBBBBB"
	var output bytes.Buffer
	runnerCalls := 0
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
				t.Fatalf("unexpected environment key")
				return ""
			}
		},
		runMailDiagnosticRekey: func(_ context.Context, gotDSN, gotCurrent, gotPrevious string, observer mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			runnerCalls++
			if gotDSN != dsn || gotCurrent != currentRoot || gotPrevious != previousRoot || observer == nil {
				t.Fatalf("runner inputs were changed or incomplete")
			}
			if err := observer(7); err != nil {
				return mail.DiagnosticRekeyResult{}, err
			}
			if err := observer(9); err != nil {
				return mail.DiagnosticRekeyResult{}, err
			}
			return mail.DiagnosticRekeyResult{
				CurrentKeyID: currentID, PreviousKeyID: previousID,
				Scanned: 2, Rekeyed: 2, PreviousReferences: 0, UnknownReferences: 0,
			}, nil
		},
		stdout: &output,
	})
	if err != nil {
		t.Fatalf("mail diagnostic rekey command returned fixed-error candidate: %v", err)
	}
	if runnerCalls != 1 {
		t.Fatalf("mail diagnostic rekey runner call count was not one")
	}
	want := "rekeyed_row_id\t7\n" +
		"rekeyed_row_id\t9\n" +
		"current_key_id\t" + currentID + "\n" +
		"previous_key_id\t" + previousID + "\n" +
		"scanned\t2\n" +
		"rekeyed\t2\n" +
		"previous_references\t0\n" +
		"unknown_references\t0\n"
	if output.String() != want {
		t.Fatalf("mail diagnostic rekey output did not match the stable field contract")
	}
	for _, forbidden := range []string{dsn, currentRoot, previousRoot, "safe-password"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("mail diagnostic rekey output exposed a forbidden value")
		}
	}
}

func TestMailDiagnosticRekeyCommandRejectsUnsafeEnvironmentBeforeRunner(t *testing.T) {
	validDSN := "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	validRoot := strings.Repeat("a", 64)
	tests := []struct {
		name     string
		dsn      string
		current  string
		previous string
		markers  []string
	}{
		{name: "malformed dsn", dsn: "marker-malformed-dsn", current: validRoot, markers: []string{"marker-malformed-dsn"}},
		{name: "dsn without database", dsn: "admin_user:safe-password@tcp(127.0.0.1:3306)/", current: validRoot, markers: []string{"safe-password"}},
		{name: "short current root", dsn: validDSN, current: "marker-short-current-root", markers: []string{"marker-short-current-root", "safe-password"}},
		{name: "placeholder current root", dsn: validDSN, current: "change_me_to_long_random", markers: []string{"change_me_to_long_random", "safe-password"}},
		{name: "same previous root", dsn: validDSN, current: validRoot, previous: validRoot, markers: []string{validRoot, "safe-password"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			var output bytes.Buffer
			err := run(context.Background(), []string{"mail-diagnostic-rekey"}, commandDependencies{
				getenv: func(key string) string {
					switch key {
					case "MYSQL_DSN":
						return test.dsn
					case "APP_SECRET":
						return test.current
					case "APP_SECRET_PREVIOUS":
						return test.previous
					default:
						return ""
					}
				},
				runMailDiagnosticRekey: func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
					called = true
					return mail.DiagnosticRekeyResult{}, nil
				},
				stdout: &output,
			})
			if err == nil || err.Error() != "mail diagnostic rekey command: failed" {
				t.Fatalf("unsafe environment did not return the fixed command failure")
			}
			if called || output.Len() != 0 {
				t.Fatalf("unsafe environment ran the rekey or wrote output")
			}
			for _, marker := range test.markers {
				if marker != "" && strings.Contains(err.Error(), marker) {
					t.Fatalf("unsafe environment error exposed a forbidden value")
				}
			}
		})
	}
}

func TestMailDiagnosticRekeyCommandRedactsRunnerFailure(t *testing.T) {
	const dsn = "admin_user:marker-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	currentRoot := strings.Repeat("c", 64)
	previousRoot := strings.Repeat("d", 64)
	providerErr := errors.New("marker-provider-error-with-secret-material")
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
		runMailDiagnosticRekey: func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			return mail.DiagnosticRekeyResult{}, providerErr
		},
		stdout: &output,
	})
	if err == nil || err.Error() != "mail diagnostic rekey command: failed" || !errors.Is(err, providerErr) {
		t.Fatalf("runner failure did not preserve its cause behind the fixed command error")
	}
	if output.Len() != 0 {
		t.Fatalf("failed runner wrote summary output")
	}
	for _, marker := range []string{dsn, "marker-password", currentRoot, previousRoot, providerErr.Error()} {
		if strings.Contains(err.Error(), marker) {
			t.Fatalf("runner failure exposed a forbidden value")
		}
	}
}

func TestMailDiagnosticRekeyCommandMapsObserverWriterFailureToFixedError(t *testing.T) {
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	root := strings.Repeat("e", 64)
	writer := diagnosticRekeyFailWriter{}
	err := run(context.Background(), []string{"mail-diagnostic-rekey"}, commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return root
			case "APP_SECRET_PREVIOUS":
				return ""
			default:
				return ""
			}
		},
		runMailDiagnosticRekey: func(_ context.Context, _ string, _ string, _ string, observer mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			err := observer(41)
			if !errors.Is(err, mail.ErrDiagnosticRekeyOutputFailed) || err.Error() != mail.ErrDiagnosticRekeyOutputFailed.Error() {
				t.Fatalf("observer did not map writer failure to the fixed output sentinel")
			}
			return mail.DiagnosticRekeyResult{}, err
		},
		stdout: writer,
	})
	if err == nil || err.Error() != "mail diagnostic rekey command: failed" || !errors.Is(err, mail.ErrDiagnosticRekeyOutputFailed) {
		t.Fatalf("writer failure did not return the fixed command failure")
	}
	if strings.Contains(err.Error(), diagnosticRekeyWriterMarker) || strings.Contains(err.Error(), dsn) || strings.Contains(err.Error(), root) {
		t.Fatalf("writer failure exposed a forbidden value")
	}
}

func TestMailDiagnosticRekeyCommandRejectsUnsafeResultBeforeSummaryOutput(t *testing.T) {
	const dsn = "admin_user:safe-password@tcp(127.0.0.1:3306)/admin?parseTime=true"
	root := strings.Repeat("f", 64)
	const unsafeID = "marker-unsafe-key-id\nmarker-injected-field"
	var output bytes.Buffer
	err := run(context.Background(), []string{"mail-diagnostic-rekey"}, commandDependencies{
		getenv: func(key string) string {
			switch key {
			case "MYSQL_DSN":
				return dsn
			case "APP_SECRET":
				return root
			case "APP_SECRET_PREVIOUS":
				return ""
			default:
				return ""
			}
		},
		runMailDiagnosticRekey: func(context.Context, string, string, string, mail.DiagnosticRekeyObserverFunc) (mail.DiagnosticRekeyResult, error) {
			return mail.DiagnosticRekeyResult{CurrentKeyID: unsafeID}, nil
		},
		stdout: &output,
	})
	if err == nil || err.Error() != "mail diagnostic rekey command: failed" || output.Len() != 0 {
		t.Fatalf("unsafe result ID was emitted or did not return fixed failure")
	}
	if strings.Contains(err.Error(), "marker-unsafe") || strings.Contains(err.Error(), root) {
		t.Fatalf("unsafe result failure exposed a forbidden value")
	}
}

const diagnosticRekeyWriterMarker = "marker-diagnostic-rekey-writer-provider-error"

type diagnosticRekeyFailWriter struct{}

func (diagnosticRekeyFailWriter) Write([]byte) (int, error) {
	return 0, errors.New(diagnosticRekeyWriterMarker)
}
