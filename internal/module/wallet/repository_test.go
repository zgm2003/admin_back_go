package wallet

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/infra/database"

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
