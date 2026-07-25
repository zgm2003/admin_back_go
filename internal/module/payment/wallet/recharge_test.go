package wallet

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreditRechargeInTxReturnsCreatedFact(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectRechargeWalletLock(mock, now, 100, 100)
	expectRechargeLedgerLookup(mock, sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET `balance_units`=?,`total_recharge_units`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WithArgs(int64(130), int64(130), sqlmock.AnyArg(), int64(1), enum.CommonNo).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	tx := repo.db.Begin()
	fact, err := repo.CreditRechargeInTx(context.Background(), tx, CreditRechargeInput{UserID: 7, RechargeID: 88, AmountUnits: 30, Remark: "支付宝充值"})
	if err != nil || fact == nil || fact.Disposition != RechargeCreditCreated || fact.Wallet == nil || fact.Transaction == nil {
		t.Fatalf("created recharge fact=%#v err=%v", fact, err)
	}
	if fact.Wallet.BalanceUnits != 130 || fact.Transaction.SourceType != SourceRecharge || fact.Transaction.SourceID != 88 {
		t.Fatalf("unexpected created recharge fact=%#v", fact)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCreditRechargeInTxReturnsReplayedFactWithoutMutation(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectRechargeWalletLock(mock, now, 130, 130)
	expectRechargeLedgerLookup(mock, rechargeLedgerRows(now, enum.CommonNo, DirectionIn, SourceRecharge, 30))
	mock.ExpectRollback()

	tx := repo.db.Begin()
	fact, err := repo.CreditRechargeInTx(context.Background(), tx, CreditRechargeInput{UserID: 7, RechargeID: 88, AmountUnits: 30, Remark: "ignored replay remark"})
	if err != nil || fact == nil || fact.Disposition != RechargeCreditReplayed || fact.Wallet == nil || fact.Transaction == nil || fact.Transaction.ID != 9 {
		t.Fatalf("replayed recharge fact=%#v err=%v", fact, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestFindRechargeCreditInTxMissingLedgerIsLookupOnly(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectRechargeWalletLock(mock, now, 130, 130)
	expectRechargeLedgerLookup(mock, sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	tx := repo.db.Begin()
	fact, err := repo.FindRechargeCreditInTx(context.Background(), tx, CreditRechargeInput{UserID: 7, RechargeID: 88, AmountUnits: 30})
	if fact != nil || !errors.Is(err, ErrRechargeCreditIntegrity) {
		t.Fatalf("missing credited replay ledger must fail, fact=%#v err=%v", fact, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestFindRechargeCreditInTxRejectsBadLedgerWithoutMutation(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectRechargeWalletLock(mock, now, 130, 130)
	expectRechargeLedgerLookup(mock, rechargeLedgerRows(now, enum.CommonYes, DirectionIn, SourceRecharge, 30))
	mock.ExpectRollback()

	tx := repo.db.Begin()
	fact, err := repo.FindRechargeCreditInTx(context.Background(), tx, CreditRechargeInput{UserID: 7, RechargeID: 88, AmountUnits: 30})
	if fact != nil || !errors.Is(err, ErrRechargeCreditIntegrity) {
		t.Fatalf("bad credited replay ledger must fail, fact=%#v err=%v", fact, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func expectRechargeWalletLock(mock sqlmock.Sqlmock, now time.Time, balance, totalRecharge int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_units", "total_recharge_units", "total_consume_units", "held_units", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), balance, totalRecharge, int64(0), int64(0), enum.CommonNo, now, now))
}

func expectRechargeLedgerLookup(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).
		WithArgs(SourceRecharge, int64(88), 2).
		WillReturnRows(rows)
}

func rechargeLedgerRows(now time.Time, isDel int, direction, sourceType string, amount int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_units", "balance_before_units", "balance_after_units", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
		AddRow(int64(9), "WLT20260726120000000001", int64(1), int64(7), direction, amount, int64(100), int64(130), sourceType, int64(88), "支付宝充值", isDel, now, now)
}
