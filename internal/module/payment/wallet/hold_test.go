package wallet

import (
	"context"
	"database/sql"
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

func TestRequireHoldTransactionRejectsTypedNilCommitter(t *testing.T) {
	var committer *sql.Tx
	tx := &gorm.DB{Statement: &gorm.Statement{ConnPool: committer}}
	if err := requireHoldTransaction(tx); !errors.Is(err, ErrHoldTransactionRequired) {
		t.Fatalf("typed-nil transaction error = %v", err)
	}
}

func TestReserveAndTopUpHoldRejectTerminalHolds(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		captured  int64
		operation func(*GormRepository, *gorm.DB) (*Hold, error)
	}{
		{
			name:     "reserve captured",
			status:   HoldCaptured,
			captured: 30,
			operation: func(repo *GormRepository, tx *gorm.DB) (*Hold, error) {
				return repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
			},
		},
		{
			name:     "top-up captured",
			status:   HoldCaptured,
			captured: 30,
			operation: func(repo *GormRepository, tx *gorm.DB) (*Hold, error) {
				return repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 40})
			},
		},
		{
			name:   "reserve released",
			status: HoldReleased,
			operation: func(repo *GormRepository, tx *gorm.DB) (*Hold, error) {
				return repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
			},
		},
		{
			name:   "top-up released",
			status: HoldReleased,
			operation: func(repo *GormRepository, tx *gorm.DB) (*Hold, error) {
				return repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 40})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				WillReturnRows(holdRows(now, 9, 1, 7, 88, 0, tt.captured, tt.status))
			if tt.status == HoldCaptured {
				expectTerminalAIGenerateLedger(mock, now, tt.captured)
			} else {
				expectNoTerminalAIGenerateLedger(mock)
			}
			mock.ExpectRollback()

			tx := repo.db.Begin()
			hold, err := tt.operation(repo, tx)
			if !errors.Is(err, ErrHoldIntegrity) || hold != nil {
				t.Fatalf("terminal hold must reject reserve/top-up, hold=%#v err=%v", hold, err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestReserveHoldRejectsInvalidActiveHoldFacts(t *testing.T) {
	tests := []struct {
		name     string
		held     int64
		captured int64
	}{
		{name: "zero held", held: 0, captured: 0},
		{name: "captured units", held: 30, captured: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectActiveHoldFactLock(mock, now, tt.held, tt.captured)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			hold, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 40})
			if hold != nil || !errors.Is(err, ErrHoldIntegrity) {
				t.Fatalf("invalid active hold must reject reserve, hold=%#v err=%v", hold, err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestCaptureAndReleaseRejectInvalidActiveHoldFacts(t *testing.T) {
	tests := []struct {
		name     string
		held     int64
		captured int64
		call     func(*GormRepository, *gorm.DB) error
	}{
		{
			name: "capture zero held", held: 0, captured: 0,
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, _, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 1, SourceSummary: "summary"})
				return err
			},
		},
		{
			name: "release zero held", held: 0, captured: 0,
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
				return err
			},
		},
		{
			name: "capture negative held", held: -5, captured: 0,
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, _, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 1, SourceSummary: "summary"})
				return err
			},
		},
		{
			name: "release captured active", held: 30, captured: 1,
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectActiveHoldFactLock(mock, now, tt.held, tt.captured)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			if err := tt.call(repo, tx); !errors.Is(err, ErrHoldIntegrity) {
				t.Fatalf("invalid active hold must reject finalization, err=%v", err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func expectActiveHoldFactLock(mock sqlmock.Sqlmock, now time.Time, held, captured int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(walletRowsWithHeld(now, 1, 7, 100, held))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).
		WithArgs(int64(1), HoldActive).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(held))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(88), 1).
		WillReturnRows(holdRows(now, 9, 1, 7, 88, held, captured, HoldActive))
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

func TestReserveAndTopUpHoldReplayEqualOrLowerTargetWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		target int64
		topUp  bool
	}{
		{name: "duplicate reserve", target: 30},
		{name: "lower reserve", target: 20},
		{name: "equal top-up", target: 30, topUp: true},
		{name: "lower top-up", target: 20, topUp: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
			expectHoldRunLock(mock, now, 30, 0, HoldActive)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			var (
				hold *Hold
				err  error
			)
			if tt.topUp {
				hold, err = repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: tt.target})
			} else {
				hold, err = repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: tt.target})
			}
			if err != nil || hold == nil || hold.HeldUnits != 30 || hold.Status != HoldActive {
				t.Fatalf("active replay=(%#v,%v)", hold, err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestReserveHoldUsesAllAvailableBalanceAndPreservesActiveAggregate(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 50, 20, 20)
	expectMissingHoldRunLock(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_holds`")).WillReturnResult(sqlmock.NewResult(9, 1))
	expectWalletHeldUpdate(mock, 50, 1)
	expectActiveHoldAggregate(mock, 50)
	mock.ExpectRollback()

	tx := repo.db.Begin()
	hold, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
	if err != nil || hold == nil || hold.HeldUnits != 30 {
		t.Fatalf("reserve at available boundary=(%#v,%v)", hold, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestTopUpHoldSuccessAndInsufficientAvailableBalance(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo, mock, closeDB := newMockRepository(t)
		defer closeDB()
		now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		expectHoldWalletAndAggregate(mock, now, 100, 40, 40)
		expectHoldRunLock(mock, now, 30, 0, HoldActive)
		expectActiveHoldUnitsUpdate(mock, 50, 1)
		expectWalletHeldUpdate(mock, 60, 1)
		expectActiveHoldAggregate(mock, 60)
		mock.ExpectRollback()

		tx := repo.db.Begin()
		hold, err := repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 50})
		if err != nil || hold == nil || hold.HeldUnits != 50 {
			t.Fatalf("top-up=(%#v,%v)", hold, err)
		}
		if err := tx.Rollback().Error; err != nil {
			t.Fatalf("rollback: %v", err)
		}
		assertMockExpectations(t, mock)
	})

	t.Run("insufficient", func(t *testing.T) {
		repo, mock, closeDB := newMockRepository(t)
		defer closeDB()
		now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		expectHoldWalletAndAggregate(mock, now, 100, 80, 80)
		expectHoldRunLock(mock, now, 30, 0, HoldActive)
		mock.ExpectRollback()

		tx := repo.db.Begin()
		hold, err := repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 60})
		if hold != nil || !errors.Is(err, ErrHoldInsufficient) {
			t.Fatalf("insufficient top-up=(%#v,%v)", hold, err)
		}
		if err := tx.Rollback().Error; err != nil {
			t.Fatalf("rollback: %v", err)
		}
		assertMockExpectations(t, mock)
	})
}

func TestCaptureHoldSuccess(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 100, 50, 50)
	expectHoldRunLock(mock, now, 30, 0, HoldActive)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(10, 1))
	expectCaptureWalletUpdate(mock, 80, 20, 20, 1)
	expectTerminalHoldUpdate(mock, 20, HoldCaptured, 1)
	expectActiveHoldAggregate(mock, 20)
	mock.ExpectRollback()

	tx := repo.db.Begin()
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "persisted run summary"})
	if err != nil || wallet == nil || wallet.BalanceUnits != 80 || wallet.HeldUnits != 20 || wallet.TotalConsumeUnits != 20 {
		t.Fatalf("capture wallet=(%#v,%v)", wallet, err)
	}
	if ledger == nil || ledger.ID != 10 || ledger.SourceType != SourceAIGenerate || ledger.SourceID != 88 || ledger.AmountUnits != 20 || ledger.Remark != "persisted run summary" {
		t.Fatalf("capture ledger=%#v", ledger)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCaptureHoldRejectsZeroActualUnitsWithoutMutation(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	mock.ExpectRollback()

	tx := repo.db.Begin()
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 0, SourceSummary: "summary"})
	if !errors.Is(err, ErrHoldInvalidInput) || wallet != nil || ledger != nil {
		t.Fatalf("zero capture must fail before mutation, wallet=%#v ledger=%#v err=%v", wallet, ledger, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCaptureHoldReplayReturnsOriginalLedgerWithoutSummaryRewrite(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectCaptureTerminalWalletAndHold(mock, now, HoldCaptured, 20)
	expectTerminalAIGenerateLedger(mock, now, 20)
	mock.ExpectRollback()

	tx := repo.db.Begin()
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "new summary must be ignored"})
	if err != nil || wallet == nil || ledger == nil || ledger.Remark != "saved" || ledger.AmountUnits != 20 {
		t.Fatalf("captured replay=(%#v,%#v,%v)", wallet, ledger, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCaptureHoldReplayRejectsZeroCapturedUnits(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	expectNoTerminalAIGenerateLedger(mock)
	mock.ExpectRollback()

	tx := repo.db.Begin()
	hold := &Hold{ID: 9, WalletID: 1, UserID: 7, RunID: 88, CapturedUnits: 0, Status: HoldCaptured}
	wallet := &Wallet{ID: 1, UserID: 7}
	ledger, err := validateCapturedHoldFact(tx, hold, wallet, 7)
	if !errors.Is(err, ErrHoldIntegrity) || ledger != nil {
		t.Fatalf("zero-unit captured fact must fail closed, ledger=%#v err=%v", ledger, err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestCaptureHoldReplayRejectsInvalidPersistedSummary(t *testing.T) {
	tests := []struct {
		name   string
		remark string
	}{
		{name: "blank", remark: ""},
		{name: "ASCII whitespace only", remark: "   "},
		{name: "Unicode whitespace only", remark: " \u2003 "},
		{name: "over 255 runes", remark: strings.Repeat("中", 256)},
		{name: "control character", remark: "ok\nno"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectCaptureTerminalWalletAndHold(mock, now, HoldCaptured, 20)
			expectTerminalAIGenerateLedgerWithRemark(mock, now, 20, tt.remark)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "valid incoming summary must be ignored"})
			if !errors.Is(err, ErrHoldIntegrity) || wallet != nil || ledger != nil {
				t.Fatalf("invalid persisted summary must fail closed, wallet=%#v ledger=%#v err=%v", wallet, ledger, err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestReleaseHoldSuccessAndReplay(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo, mock, closeDB := newMockRepository(t)
		defer closeDB()
		now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		expectHoldWalletAndAggregate(mock, now, 100, 50, 50)
		expectHoldRunLock(mock, now, 30, 0, HoldActive)
		expectWalletHeldUpdate(mock, 20, 1)
		expectTerminalHoldUpdate(mock, 0, HoldReleased, 1)
		expectActiveHoldAggregate(mock, 20)
		mock.ExpectRollback()

		tx := repo.db.Begin()
		hold, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
		if err != nil || hold == nil || hold.Status != HoldReleased || hold.HeldUnits != 0 || hold.CapturedUnits != 0 {
			t.Fatalf("release=(%#v,%v)", hold, err)
		}
		if err := tx.Rollback().Error; err != nil {
			t.Fatalf("rollback: %v", err)
		}
		assertMockExpectations(t, mock)
	})

	t.Run("replay", func(t *testing.T) {
		repo, mock, closeDB := newMockRepository(t)
		defer closeDB()
		now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		mock.ExpectBegin()
		expectHoldWalletAndAggregate(mock, now, 100, 0, 0)
		expectHoldRunLock(mock, now, 0, 0, HoldReleased)
		expectNoTerminalAIGenerateLedger(mock)
		mock.ExpectRollback()

		tx := repo.db.Begin()
		hold, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
		if err != nil || hold == nil || hold.Status != HoldReleased {
			t.Fatalf("release replay=(%#v,%v)", hold, err)
		}
		if err := tx.Rollback().Error; err != nil {
			t.Fatalf("rollback: %v", err)
		}
		assertMockExpectations(t, mock)
	})
}

func expectHoldWalletAndAggregate(mock sqlmock.Sqlmock, now time.Time, balance, walletHeld, aggregate int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(walletRowsWithHeld(now, 1, 7, balance, walletHeld))
	expectActiveHoldAggregate(mock, aggregate)
}

func expectActiveHoldAggregate(mock sqlmock.Sqlmock, aggregate int64) {
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(held_units\), 0\) AS total FROM wallet_holds WHERE wallet_id = \? AND status = \? FOR UPDATE`).
		WithArgs(int64(1), HoldActive).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(aggregate))
}

func expectHoldRunLock(mock sqlmock.Sqlmock, now time.Time, held, captured int64, status string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(88), 1).
		WillReturnRows(holdRows(now, 9, 1, 7, 88, held, captured, status))
}

func expectMissingHoldRunLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_holds` WHERE run_id = ? ORDER BY `wallet_holds`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(88), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func expectActiveHoldUnitsUpdate(mock sqlmock.Sqlmock, held int64, rows int64) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `wallet_holds` SET `held_units`=?,`updated_at`=? WHERE id = ? AND status = ?")).
		WithArgs(held, sqlmock.AnyArg(), int64(9), HoldActive).
		WillReturnResult(sqlmock.NewResult(0, rows))
}

func expectWalletHeldUpdate(mock sqlmock.Sqlmock, held int64, rows int64) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET `held_units`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WithArgs(held, sqlmock.AnyArg(), int64(1), enum.CommonNo).
		WillReturnResult(sqlmock.NewResult(0, rows))
}

func expectCaptureWalletUpdate(mock sqlmock.Sqlmock, balance, held, consume, rows int64) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET `balance_units`=?,`held_units`=?,`total_consume_units`=?,`updated_at`=? WHERE id = ? AND is_del = ?")).
		WithArgs(balance, held, consume, sqlmock.AnyArg(), int64(1), enum.CommonNo).
		WillReturnResult(sqlmock.NewResult(0, rows))
}

func expectTerminalHoldUpdate(mock sqlmock.Sqlmock, captured int64, status string, rows int64) {
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `wallet_holds` SET `captured_units`=?,`held_units`=?,`status`=?,`updated_at`=? WHERE id = ? AND status = ?")).
		WithArgs(captured, int64(0), status, sqlmock.AnyArg(), int64(9), HoldActive).
		WillReturnResult(sqlmock.NewResult(0, rows))
}

func TestHoldOperationsRejectActiveAggregateMismatch(t *testing.T) {
	tests := []struct {
		name string
		call func(*GormRepository, *gorm.DB) error
	}{
		{
			name: "reserve",
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 40})
				return err
			},
		},
		{
			name: "capture",
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, _, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "summary"})
				return err
			},
		},
		{
			name: "release",
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectHoldWalletAndAggregate(mock, now, 100, 30, 29)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			if err := tt.call(repo, tx); !errors.Is(err, ErrHoldIntegrity) {
				t.Fatalf("aggregate mismatch error=%v", err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestCaptureAndReleaseRejectHoldUnderflow(t *testing.T) {
	tests := []struct {
		name       string
		walletHeld int64
		actual     int64
		release    bool
	}{
		{name: "capture over hold", walletHeld: 30, actual: 31},
		{name: "capture aggregate underflow", walletHeld: 20, actual: 20},
		{name: "release aggregate underflow", walletHeld: 20, release: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectHoldWalletAndAggregate(mock, now, 100, tt.walletHeld, tt.walletHeld)
			expectHoldRunLock(mock, now, 30, 0, HoldActive)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			var err error
			if tt.release {
				_, err = repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
			} else {
				_, _, err = repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: tt.actual, SourceSummary: "summary"})
			}
			if !errors.Is(err, ErrHoldUnderflow) {
				t.Fatalf("underflow error=%v", err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestHoldMutationsRejectZeroRowsAffected(t *testing.T) {
	tests := []struct {
		name   string
		expect func(sqlmock.Sqlmock, time.Time)
		call   func(*GormRepository, *gorm.DB) error
	}{
		{
			name: "reserve hold insert",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 0, 0)
				expectMissingHoldRunLock(mock)
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_holds`")).WillReturnResult(sqlmock.NewResult(9, 0))
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
				return err
			},
		},
		{
			name: "reserve wallet update",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 0, 0)
				expectMissingHoldRunLock(mock)
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_holds`")).WillReturnResult(sqlmock.NewResult(9, 1))
				expectWalletHeldUpdate(mock, 30, 0)
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
				return err
			},
		},
		{
			name: "top-up hold update",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
				expectHoldRunLock(mock, now, 30, 0, HoldActive)
				expectActiveHoldUnitsUpdate(mock, 40, 0)
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 40})
				return err
			},
		},
		{
			name: "capture wallet update",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
				expectHoldRunLock(mock, now, 30, 0, HoldActive)
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(10, 1))
				expectCaptureWalletUpdate(mock, 80, 0, 20, 0)
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, _, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "summary"})
				return err
			},
		},
		{
			name: "capture hold update",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
				expectHoldRunLock(mock, now, 30, 0, HoldActive)
				mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(10, 1))
				expectCaptureWalletUpdate(mock, 80, 0, 20, 1)
				expectTerminalHoldUpdate(mock, 20, HoldCaptured, 0)
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, _, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 20, SourceSummary: "summary"})
				return err
			},
		},
		{
			name: "release wallet update",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
				expectHoldRunLock(mock, now, 30, 0, HoldActive)
				expectWalletHeldUpdate(mock, 0, 0)
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
				return err
			},
		},
		{
			name: "release hold update",
			expect: func(mock sqlmock.Sqlmock, now time.Time) {
				expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
				expectHoldRunLock(mock, now, 30, 0, HoldActive)
				expectWalletHeldUpdate(mock, 0, 1)
				expectTerminalHoldUpdate(mock, 0, HoldReleased, 0)
			},
			call: func(repo *GormRepository, tx *gorm.DB) error {
				_, err := repo.ReleaseHoldInTx(context.Background(), tx, ReleaseHoldInput{UserID: 7, RunID: 88})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			tt.expect(mock, now)
			mock.ExpectRollback()

			tx := repo.db.Begin()
			if err := tt.call(repo, tx); !errors.Is(err, ErrHoldIntegrity) {
				t.Fatalf("zero RowsAffected error=%v", err)
			}
			if err := tx.Rollback().Error; err != nil {
				t.Fatalf("rollback: %v", err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestSerializedHoldReserveTopUpCaptureAndReplayPreserveFacts(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 100, 0, 0)
	expectMissingHoldRunLock(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_holds`")).WillReturnResult(sqlmock.NewResult(9, 1))
	expectWalletHeldUpdate(mock, 30, 1)
	expectActiveHoldAggregate(mock, 30)
	mock.ExpectCommit()

	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
	expectHoldRunLock(mock, now, 30, 0, HoldActive)
	mock.ExpectCommit()

	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 100, 30, 30)
	expectHoldRunLock(mock, now, 30, 0, HoldActive)
	expectActiveHoldUnitsUpdate(mock, 50, 1)
	expectWalletHeldUpdate(mock, 50, 1)
	expectActiveHoldAggregate(mock, 50)
	mock.ExpectCommit()

	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 100, 50, 50)
	expectHoldRunLock(mock, now, 50, 0, HoldActive)
	mock.ExpectCommit()

	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 100, 50, 50)
	expectHoldRunLock(mock, now, 50, 0, HoldActive)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(10, 1))
	expectCaptureWalletUpdate(mock, 60, 0, 40, 1)
	expectTerminalHoldUpdate(mock, 40, HoldCaptured, 1)
	expectActiveHoldAggregate(mock, 0)
	mock.ExpectCommit()

	mock.ExpectBegin()
	expectHoldWalletAndAggregate(mock, now, 60, 0, 0)
	expectHoldRunLock(mock, now, 0, 40, HoldCaptured)
	expectTerminalAIGenerateLedger(mock, now, 40)
	mock.ExpectCommit()

	tx := repo.db.Begin()
	hold, err := repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
	if err != nil || hold == nil || hold.HeldUnits != 30 {
		t.Fatalf("initial reserve=(%#v,%v)", hold, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit initial reserve: %v", err)
	}

	tx = repo.db.Begin()
	hold, err = repo.ReserveHoldInTx(context.Background(), tx, ReserveHoldInput{UserID: 7, RunID: 88, AmountUnits: 30})
	if err != nil || hold == nil || hold.HeldUnits != 30 {
		t.Fatalf("duplicate reserve=(%#v,%v)", hold, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit duplicate reserve: %v", err)
	}

	tx = repo.db.Begin()
	hold, err = repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 50})
	if err != nil || hold == nil || hold.HeldUnits != 50 {
		t.Fatalf("top-up=(%#v,%v)", hold, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit top-up: %v", err)
	}

	tx = repo.db.Begin()
	hold, err = repo.TopUpHoldInTx(context.Background(), tx, TopUpHoldInput{UserID: 7, RunID: 88, AmountUnits: 40})
	if err != nil || hold == nil || hold.HeldUnits != 50 {
		t.Fatalf("stale lower top-up=(%#v,%v)", hold, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit stale top-up: %v", err)
	}

	tx = repo.db.Begin()
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 40, SourceSummary: "saved"})
	if err != nil || wallet == nil || wallet.BalanceUnits != 60 || wallet.HeldUnits != 0 || ledger == nil || ledger.AmountUnits != 40 {
		t.Fatalf("capture=(%#v,%#v,%v)", wallet, ledger, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit capture: %v", err)
	}

	tx = repo.db.Begin()
	wallet, ledger, err = repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 40, SourceSummary: "stale retry"})
	if err != nil || wallet == nil || ledger == nil || ledger.ID != 10 || ledger.Remark != "saved" {
		t.Fatalf("capture replay=(%#v,%#v,%v)", wallet, ledger, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit capture replay: %v", err)
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
	wallet, ledger, err := repo.CaptureHoldInTx(context.Background(), tx, CaptureHoldInput{UserID: 7, RunID: 88, ActualUnits: 1, SourceSummary: "new summary"})
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
	expectTerminalAIGenerateLedgerWithRemark(mock, now, amount, "saved")
}

func expectTerminalAIGenerateLedgerWithRemark(mock sqlmock.Sqlmock, now time.Time, amount int64, remark string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).WithArgs(SourceAIGenerate, int64(88), 2).WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_units", "balance_before_units", "balance_after_units", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).AddRow(int64(10), "WLT", int64(1), int64(7), DirectionOut, amount, int64(100), int64(100-amount), SourceAIGenerate, int64(88), remark, enum.CommonNo, now, now))
}

func expectNoTerminalAIGenerateLedger(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY id ASC LIMIT ? FOR UPDATE")).WithArgs(SourceAIGenerate, int64(88), 2).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func TestCaptureHoldRejectsInvalidSummary(t *testing.T) {
	for _, summary := range []string{"", "   ", " \u2003 ", strings.Repeat("a", 256), "ok\nno"} {
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
