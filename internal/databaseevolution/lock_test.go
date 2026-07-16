package databaseevolution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWithAdvisoryLockHoldsDedicatedConnectionAroundCallback(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT GET_LOCK\(\?, \?\)`).WithArgs("admin:atlas:migrate", int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	mock.ExpectQuery(`SELECT RELEASE_LOCK\(\?\)`).WithArgs("admin:atlas:migrate").
		WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))

	called := false
	if err := WithAdvisoryLock(context.Background(), db, "admin:atlas:migrate", 30*time.Second, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("WithAdvisoryLock returned error: %v", err)
	}
	if !called {
		t.Fatal("callback was not executed while lock was held")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithAdvisoryLockReleasesAfterCallbackFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT GET_LOCK\(\?, \?\)`).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
	mock.ExpectQuery(`SELECT RELEASE_LOCK\(\?\)`).WillReturnRows(sqlmock.NewRows([]string{"released"}).AddRow(1))
	sentinel := errors.New("atlas failed")
	err = WithAdvisoryLock(context.Background(), db, "admin:atlas:migrate", time.Second, func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("callback error was not preserved: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
