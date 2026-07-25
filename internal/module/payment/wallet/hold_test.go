package wallet

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestReserveHoldRejectsNilOrRootTransaction(t *testing.T) {
	repo := &GormRepository{}
	if _, err := repo.ReserveHoldInTx(context.Background(), nil, ReserveHoldInput{UserID: 1, RunID: 2, AmountUnits: 1}); err != ErrRepositoryNotConfigured {
		t.Fatalf("nil repository handle error = %v", err)
	}
	repo = &GormRepository{db: &gorm.DB{}}
	if _, err := repo.ReserveHoldInTx(context.Background(), repo.db, ReserveHoldInput{UserID: 1, RunID: 2, AmountUnits: 1}); err != ErrHoldTransactionRequired {
		t.Fatalf("root transaction error = %v", err)
	}
}

func TestReserveHoldLocksWalletBeforeCurrentHoldAggregate(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(walletRowsWithHeld(now, 1, 7, 100, 0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).
		WithArgs(int64(1), HoldActive).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(88), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_holds`")).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET `held_units`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WithArgs(int64(30), sqlmock.AnyArg(), int64(1), enum.CommonNo).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).
		WithArgs(int64(1), HoldActive).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(30)))
	mock.ExpectRollback()

	tx := repo.db.Begin()
	hold, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
	if err != nil || hold == nil || hold.Status != HoldActive || hold.HeldUnits != 30 {
		t.Fatalf("ReserveHoldInTx=(%#v,%v)", hold, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestReleaseHoldRejectsCapturedTerminalStateWithoutLedgerFact(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(walletRowsWithHeld(now, 1, 7, 100, 0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).
		WithArgs(int64(1), HoldActive).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(88), 1).
		WillReturnRows(holdRows(now, 9, 1, 7, 88, 0, 30, HoldCaptured))
	mock.ExpectCommit()

	tx := repo.db.Begin()
	hold, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
	if !errors.Is(err, ErrHoldIntegrity) || hold != nil {
		t.Fatalf("captured terminal state without matching ledger must fail closed, hold=%#v err=%v", hold, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestReleaseHoldRejectsReleasedTerminalStateWithAIGenerateLedger(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(walletRowsWithHeld(now, 1, 7, 100, 0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).
		WithArgs(int64(1), HoldActive).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(88), 1).
		WillReturnRows(holdRows(now, 9, 1, 7, 88, 0, 0, HoldReleased))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).
		WithArgs(SourceAIGenerate, int64(88), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_units", "balance_before_units", "balance_after_units", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(10), "WLT", int64(1), int64(7), DirectionOut, int64(30), int64(100), int64(70), SourceAIGenerate, int64(88), "AI", enum.CommonNo, now, now))
	mock.ExpectCommit()

	tx := repo.db.Begin()
	hold, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
	if !errors.Is(err, ErrHoldIntegrity) || hold != nil {
		t.Fatalf("released terminal state with conflicting AI ledger must fail closed, hold=%#v err=%v", hold, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCaptureHoldRejectsCapturedReplayWithDifferentAmount(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectCaptureTerminalWalletAndHold(mock, now, HoldCaptured, 30)
	mock.ExpectCommit()
	tx := repo.db.Begin()
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "new summary"})
	if !errors.Is(err, ErrHoldIntegrity) || wallet != nil || ledger != nil {
		t.Fatalf("captured replay must match captured amount, wallet=%#v ledger=%#v err=%v", wallet, ledger, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCaptureHoldRejectsReleasedTerminalState(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectCaptureTerminalWalletAndHold(mock, now, HoldReleased, 0)
	mock.ExpectCommit()
	tx := repo.db.Begin()
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 0, SourceSummary: "new summary"})
	if !errors.Is(err, ErrHoldIntegrity) || wallet != nil || ledger != nil {
		t.Fatalf("capture must reject released terminal state, wallet=%#v ledger=%#v err=%v", wallet, ledger, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit: %v", err)
	}
	assertMockExpectations(t, mock)
}

func expectCaptureTerminalWalletAndHold(mock sqlmock.Sqlmock, now time.Time, status string, captured int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).WithArgs(int64(7), enum.CommonNo, 1).WillReturnRows(walletRowsWithHeld(now, 1, 7, 100, 0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).WithArgs(int64(1), HoldActive).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).WithArgs(int64(88), 1).WillReturnRows(holdRows(now, 9, 1, 7, 88, 0, captured, status))
}

func expectTerminalAIGenerateLedger(mock sqlmock.Sqlmock, now time.Time, amount int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).WithArgs(SourceAIGenerate, int64(88), 2).WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_units", "balance_before_units", "balance_after_units", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).AddRow(int64(10), "WLT", int64(1), int64(7), DirectionOut, amount, int64(100), int64(100-amount), SourceAIGenerate, int64(88), "saved", enum.CommonNo, now, now))
}

func expectNoTerminalAIGenerateLedger(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).WithArgs(SourceAIGenerate, int64(88), 2).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func TestCaptureHoldRejectsInvalidSummary(t *testing.T) {
	for _, summary := range []string{"", strings.Repeat("a", 256), "ok\nno"} {
		if err := validateHoldSummary(summary); err != ErrHoldSummaryInvalid {
			t.Fatalf("summary %q error = %v", summary, err)
		}
	}
	if utf8.RuneCountInString(strings.Repeat("中", 255)) != 255 {
		t.Fatal("test setup invalid")
	}
	if err := validateHoldSummary(strings.Repeat("中", 255)); err != nil {
		t.Fatalf("valid summary error = %v", err)
	}
}

func walletRowsWithHeld(now time.Time, id, userID, balance, held int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "balance_units", "total_recharge_units", "total_consume_units", "held_units", "is_del", "created_at", "updated_at"}).
		AddRow(id, userID, balance, balance, int64(0), held, enum.CommonNo, now, now)
}

func holdRows(now time.Time, id, walletID, userID, runID, held, captured int64, status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "wallet_id", "user_id", "run_id", "held_units", "captured_units", "status", "created_at", "updated_at"}).
		AddRow(id, walletID, userID, runID, held, captured, status, now, now)
}
