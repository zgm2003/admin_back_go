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

func TestRepositoryDebitLocksOrCreatesWalletInsideTransaction(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_wallets`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE id = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(0), int64(0), int64(0), enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88}, now)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected insufficient balance after locked wallet create, wallet=%#v tx=%#v err=%v", wallet, tx, err)
	}
	if tx != nil {
		t.Fatalf("insufficient debit must not create transaction, got %#v", tx)
	}
	if wallet == nil || wallet.ID != 1 || wallet.UserID != 7 {
		t.Fatalf("expected created and locked wallet, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryDebitTransactionAndBalanceUpdateAreAtomic(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1000), int64(1000), int64(0), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	wallet, tx, err := repo.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88, Remark: "billing"}, now)
	if err != nil {
		t.Fatalf("Debit error=%v", err)
	}
	if tx == nil || tx.Direction != DirectionOut || tx.SourceType != SourceAIGenerate || tx.SourceID != 88 || tx.BalanceAfterCents != 900 {
		t.Fatalf("expected ai_generate out transaction, got %#v", tx)
	}
	if wallet == nil || wallet.BalanceCents != 900 || wallet.TotalConsumeCents != 100 {
		t.Fatalf("expected debit wallet balance and total_consume update, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryMutationDuplicateSourceReturnsExistingSameUserTransaction(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIRefund, int64(88), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(10), "WLT20260530120000000002", int64(1), int64(7), DirectionIn, int64(100), int64(1000), int64(1100), SourceAIRefund, int64(88), "already refunded", enum.CommonNo, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ?")).
		WithArgs(int64(1), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1100), int64(1000), int64(100), enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Credit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIRefund, SourceID: 88}, now)
	if err != nil {
		t.Fatalf("expected duplicate source to return existing transaction, got err=%v", err)
	}
	if tx == nil || tx.ID != 10 || tx.UserID != 7 || tx.SourceType != SourceAIRefund || tx.SourceID != 88 {
		t.Fatalf("expected existing transaction, got %#v", tx)
	}
	if wallet == nil || wallet.ID != 1 || wallet.BalanceCents != 1100 || wallet.TotalConsumeCents != 100 {
		t.Fatalf("expected existing transaction wallet, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryMutationDuplicateSourceOwnedByAnotherUserReturnsOwnerMismatch(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(9), "WLT20260530120000000001", int64(1), int64(7), DirectionOut, int64(100), int64(1000), int64(900), SourceAIGenerate, int64(88), "owner-a", enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Debit(context.Background(), MutationInput{UserID: 8, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88}, now)
	if !errors.Is(err, ErrMutationSourceOwnerMismatch) {
		t.Fatalf("expected source owner mismatch, got wallet=%#v tx=%#v err=%v", wallet, tx, err)
	}
	if wallet != nil || tx != nil {
		t.Fatalf("owner mismatch must not return another user's wallet/transaction, wallet=%#v tx=%#v", wallet, tx)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryDebitDuplicateSourceRaceReturnsExistingSameUserTransaction(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1000), int64(1000), int64(0), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry 'ai_generate-88' for key 'uk_wallet_transaction_source'"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(9), "WLT20260530120000000001", int64(1), int64(7), DirectionOut, int64(100), int64(1000), int64(900), SourceAIGenerate, int64(88), "race winner", enum.CommonNo, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(1), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(900), int64(1000), int64(100), enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88}, now)
	if err != nil {
		t.Fatalf("expected duplicate source race to return existing transaction, got err=%v", err)
	}
	if tx == nil || tx.ID != 9 || tx.UserID != 7 || tx.SourceType != SourceAIGenerate || tx.SourceID != 88 {
		t.Fatalf("expected existing same-user transaction after duplicate source race, got %#v", tx)
	}
	if wallet == nil || wallet.ID != 1 || wallet.BalanceCents != 900 || wallet.TotalConsumeCents != 100 {
		t.Fatalf("expected existing transaction wallet after duplicate source race, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryDebitDuplicateSourceRaceOwnedByAnotherUserReturnsOwnerMismatch(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(8), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(2), int64(8), int64(1000), int64(1000), int64(0), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry 'ai_generate-88' for key 'uk_wallet_transaction_source'"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"}).
			AddRow(int64(9), "WLT20260530120000000001", int64(1), int64(7), DirectionOut, int64(100), int64(1000), int64(900), SourceAIGenerate, int64(88), "race winner", enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, tx, err := repo.Debit(context.Background(), MutationInput{UserID: 8, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88}, now)
	if !errors.Is(err, ErrMutationSourceOwnerMismatch) {
		t.Fatalf("expected source owner mismatch after duplicate source race, got wallet=%#v tx=%#v err=%v", wallet, tx, err)
	}
	if wallet != nil || tx != nil {
		t.Fatalf("owner mismatch race must not return another user's wallet/transaction, wallet=%#v tx=%#v", wallet, tx)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryCreditDoesNotDecrementTotalConsumeCents(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIRefund, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1000), int64(1000), int64(100), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	wallet, tx, err := repo.Credit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIRefund, SourceID: 88, Remark: "refund"}, now)
	if err != nil {
		t.Fatalf("Credit error=%v", err)
	}
	if tx == nil || tx.Direction != DirectionIn || tx.SourceType != SourceAIRefund || tx.SourceID != 88 || tx.BalanceAfterCents != 1100 {
		t.Fatalf("expected ai_refund in transaction, got %#v", tx)
	}
	if wallet == nil || wallet.BalanceCents != 1100 || wallet.TotalConsumeCents != 100 {
		t.Fatalf("credit must keep total_consume_cents unchanged, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryDebitRetriesDuplicateTransactionNo(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? AND is_del = ? ORDER BY `wallet_transactions`.`id` LIMIT ?")).
		WithArgs(SourceAIGenerate, int64(88), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(1000), int64(1000), int64(0), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry 'WLT20260530120000000000123000001' for key 'uk_wallet_transaction_no'"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	wallet, tx, err := repo.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88}, now)
	if err != nil {
		t.Fatalf("expected duplicate transaction_no to retry, got err=%v", err)
	}
	if tx == nil || tx.ID != 10 || tx.UserID != 7 || tx.SourceID != 88 {
		t.Fatalf("expected retried transaction, got %#v", tx)
	}
	if wallet == nil || wallet.BalanceCents != 900 || wallet.TotalConsumeCents != 100 {
		t.Fatalf("expected updated wallet after retry, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryGetOrCreateWalletReturnsExistingOnDuplicateUserWallet(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ?")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_wallets`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry '7' for key 'uk_user_wallet_user'"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? AND is_del = ? ORDER BY `user_wallets`.`id` LIMIT ?")).
		WithArgs(int64(7), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"}).
			AddRow(int64(1), int64(7), int64(0), int64(0), int64(0), enum.CommonNo, now, now))

	wallet, err := repo.GetOrCreateWallet(context.Background(), 7)
	if err != nil {
		t.Fatalf("expected duplicate wallet race to return existing wallet, got err=%v", err)
	}
	if wallet == nil || wallet.ID != 1 || wallet.UserID != 7 {
		t.Fatalf("expected existing wallet after duplicate race, got %#v", wallet)
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

func TestTransactionNoKeepsMillisecondDistinct(t *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 123, time.UTC)

	if newTransactionNo(base) == newTransactionNo(base.Add(time.Millisecond)) {
		t.Fatalf("transaction numbers must differ across millisecond-separated timestamps: %s", newTransactionNo(base))
	}
	if newTransactionNo(base) == newTransactionNo(base) {
		t.Fatalf("transaction numbers must differ across repeated calls at the same timestamp")
	}
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
