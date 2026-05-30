package wallet

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositoryConsumeReturnsLockedWalletOnInsufficientBalance(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceConsume, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(50), int64(500), int64(450), enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Consume(context.Background(), ConsumeInput{UserID: 7, AmountCents: 100, SourceID: 88}, now)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
	if tx != nil {
		t.Fatalf("insufficient balance must not create transaction, got %#v", tx)
	}
	if wallet == nil || wallet.ID != 1 || wallet.BalanceCents != 50 || wallet.TotalConsumeCents != 450 {
		t.Fatalf("expected locked wallet to be returned, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryConsumeRejectsDuplicateSourceOwnedByAnotherUser(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceConsume, int64(88), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(9), "WLT20260521120000000001", int64(1), int64(7), DirectionOut, int64(100), int64(1000), int64(900), SourceConsume, int64(88), "owner-a", enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Consume(context.Background(), ConsumeInput{UserID: 8, AmountCents: 100, SourceID: 88}, now)
	if err == nil || err.Error() != "wallet consume source owner mismatch" {
		t.Fatalf("expected source owner mismatch, got wallet=%#v tx=%#v err=%v", wallet, tx, err)
	}
	if wallet != nil || tx != nil {
		t.Fatalf("source owner mismatch must not return another user's wallet/transaction, wallet=%#v tx=%#v", wallet, tx)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryListTransactionsUsesExclusiveNextDayDateEnd(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM wallet_transactions AS wt .*wt\\.created_at < \\?").
		WithArgs(enum.CommonNo, enum.CommonNo, "2026-05-31").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT wt\\.\\*, u\\.username AS username, u\\.phone AS phone, u\\.email AS email FROM wallet_transactions AS wt .*wt\\.created_at < \\?").
		WithArgs(enum.CommonNo, enum.CommonNo, "2026-05-31", defaultPageSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at", "username", "phone", "email"}))

	rows, total, err := repo.ListTransactions(context.Background(), TransactionListQuery{DateEnd: "2026-05-30"})
	if err != nil {
		t.Fatalf("ListTransactions error=%v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("expected empty rows, total=%d rows=%#v", total, rows)
	}
	assertMockExpectations(t, mock)
}

func newMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm mock db: %v", err)
	}
	client := &database.Client{Gorm: db, SQL: sqlDB}
	return NewGormRepository(client), mock, func() { _ = sqlDB.Close() }
}

func assertMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
