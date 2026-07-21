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
