package databaseevolution

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWithAdvisoryLockConnectionPassesLockOwningConnection(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT GET_LOCK\(\?, \?\)`).WithArgs("admin:atlas:migrate", int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	mock.ExpectQuery(`SELECT RELEASE_LOCK\(\?\)`).WithArgs("admin:atlas:migrate").
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))

	var lockedConnection *sql.Conn
	err = WithAdvisoryLockConnection(context.Background(), db, "admin:atlas:migrate", 30*time.Second, func(connection *sql.Conn) error {
		if connection == nil {
			t.Fatal("lock-held callback did not receive the dedicated connection")
		}
		lockedConnection = connection
		return nil
	})
	if err != nil {
		t.Fatalf("WithAdvisoryLockConnection returned error: %v", err)
	}
	if lockedConnection == nil {
		t.Fatal("callback was not executed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithAdvisoryLockConnectionReleasesAfterChildFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT GET_LOCK\(\?, \?\)`).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	mock.ExpectQuery(`SELECT RELEASE_LOCK\(\?\)`).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))

	sentinel := errors.New("atlas apply failed")
	err = WithAdvisoryLockConnection(context.Background(), db, "admin:atlas:migrate", time.Second, func(*sql.Conn) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("callback error was not preserved: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
