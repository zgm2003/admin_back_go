package payment

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

func TestGormRepositoryCreditRechargeRetriesDuplicateTransactionNo(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)
	paidAt := now.Add(-time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `payment_recharges` WHERE id = ? AND is_del = ? ORDER BY `payment_recharges`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(10), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "recharge_no", "user_id", "package_code", "package_name", "amount_cents", "payment_order_id", "status", "paid_at", "credited_at", "failure_reason", "is_del", "created_at", "updated_at"}).
			AddRow(int64(10), "RCG20260530115900000001", int64(7), "recharge_5", "¥5", int64(500), int64(20), rechargeStatusPaid, nil, nil, "", enum.CommonNo, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1000), int64(1000), int64(0), enum.CommonNo, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ?")).
		WithArgs(walletSourceRecharge, int64(10), enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry 'WLT20260530120000000000123000001' for key 'uk_wallet_transaction_no'"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnResult(sqlmock.NewResult(99, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `payment_recharges` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	wallet, recharge, err := repo.CreditRecharge(context.Background(), 10, paidAt, now)
	if err != nil {
		t.Fatalf("expected duplicate transaction_no to retry, got err=%v", err)
	}
	if wallet == nil || wallet.BalanceCents != 1500 || wallet.TotalRechargeCents != 1500 {
		t.Fatalf("expected credited wallet after retry, got %#v", wallet)
	}
	if recharge == nil || recharge.Status != rechargeStatusCredited || recharge.CreditedAt == nil {
		t.Fatalf("expected credited recharge after retry, got %#v", recharge)
	}
	assertPaymentMockExpectations(t, mock)
}

func TestGormRepositoryListUncreditedPaidRechargesFindsPaidOrdersWithoutWalletCredit(t *testing.T) {
	repo, mock, closeDB := newPaymentMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Minute)
	mock.ExpectQuery(`SELECT r\.\*, po\.order_no AS payment_order_no.*FROM payment_recharges AS r.*JOIN payment_orders AS po.*po\.status = \?.*r\.status IN.*r\.credited_at IS NULL.*r\.is_del = \?.*ORDER BY r\.id asc.*LIMIT \?`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "recharge_no", "user_id", "package_code", "package_name", "amount_cents", "payment_order_id",
			"status", "paid_at", "credited_at", "failure_reason", "is_del", "created_at", "updated_at",
			"payment_order_no", "pay_url", "order_status", "alipay_trade_no", "order_paid_at",
		}).AddRow(
			int64(10), "RCG20260530115900000001", int64(7), "recharge_5", "¥5", int64(500), int64(20),
			rechargeStatusPaid, &paidAt, nil, "", enum.CommonNo, now, now,
			"PAY20260530115900000001", "", orderStatusPaid, "202605302200", &paidAt,
		))

	rows, err := repo.ListUncreditedPaidRecharges(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListUncreditedPaidRecharges error=%v", err)
	}
	if len(rows) != 1 || rows[0].ID != 10 || rows[0].PaymentOrderNo == "" || rows[0].OrderPaidAt == nil {
		t.Fatalf("unexpected rows=%#v", rows)
	}
	assertPaymentMockExpectations(t, mock)
}

func newPaymentMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
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

func assertPaymentMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
