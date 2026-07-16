package databaseevolution

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunInvariantFileFailsOnViolation(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	query := "SELECT 'wallet_balance_matches_ledger' AS invariant, 1 AS violations"
	mock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnRows(
		sqlmock.NewRows([]string{"invariant", "violations"}).AddRow("wallet_balance_matches_ledger", 1),
	)

	path := filepath.Join("testdata", "violations.sql")
	result, err := RunInvariantFile(context.Background(), database, path)
	if err == nil || result.Name != "wallet_balance_matches_ledger" || result.Violations != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunInvariantFileReportsEveryZeroCheck(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	first := "SELECT 'rbac_orphans' AS invariant, 0 AS violations"
	second := "SELECT 'payment_orphans' AS invariant, 0 AS violations"
	mock.ExpectQuery(regexp.QuoteMeta(first)).WillReturnRows(
		sqlmock.NewRows([]string{"invariant", "violations"}).AddRow("rbac_orphans", 0),
	)
	mock.ExpectQuery(regexp.QuoteMeta(second)).WillReturnRows(
		sqlmock.NewRows([]string{"invariant", "violations"}).AddRow("payment_orphans", 0),
	)

	path := filepath.Join(t.TempDir(), "relations.sql")
	if err := os.WriteFile(path, []byte(first+";\n"+second+";\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RunInvariantFile(context.Background(), database, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checks) != 2 || result.Checks[0].Name != "rbac_orphans" || result.Checks[1].Name != "payment_orphans" {
		t.Fatalf("result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunInvariantFileReportsAllChecksBeforeFailing(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	first := "SELECT 'rbac_orphans' AS invariant, 1 AS violations"
	second := "SELECT 'payment_orphans' AS invariant, 0 AS violations"
	mock.ExpectQuery(regexp.QuoteMeta(first)).WillReturnRows(
		sqlmock.NewRows([]string{"invariant", "violations"}).AddRow("rbac_orphans", 1),
	)
	mock.ExpectQuery(regexp.QuoteMeta(second)).WillReturnRows(
		sqlmock.NewRows([]string{"invariant", "violations"}).AddRow("payment_orphans", 0),
	)

	path := filepath.Join(t.TempDir(), "all-checks.sql")
	if err := os.WriteFile(path, []byte(first+";\n"+second+";\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RunInvariantFile(context.Background(), database, path)
	if err == nil || result.Name != "rbac_orphans" || result.Violations != 1 || len(result.Checks) != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
