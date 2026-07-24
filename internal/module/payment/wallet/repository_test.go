package wallet

import (
	"context"
	"errors"
	"math"
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

var _ TransactionParticipant = (*GormRepository)(nil)

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
	first := newTransactionNo(base)
	second := newTransactionNo(base)
	if first == second {
		t.Fatalf("transaction numbers must differ across repeated calls at the same timestamp")
	}
}

func TestRepositoryRedeemCodeCreditRequiresOuterTransactionAndValidInput(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	valid := RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}
	for name, tx := range map[string]*gorm.DB{"nil": nil, "root": repo.db} {
		t.Run(name, func(t *testing.T) {
			wallet, transaction, err := repo.CreditRedeemCodeInTx(context.Background(), tx, valid, now)
			if !errors.Is(err, ErrRedeemCodeTransactionRequired) {
				t.Fatalf("expected outer transaction requirement, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
			}
		})
	}

	invalidInputs := map[string]RedeemCodeCreditInput{
		"user":   {UserID: 0, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"},
		"code":   {UserID: 7, CodeID: 0, AmountCents: 100, BatchNo: "RCB202607240001"},
		"amount": {UserID: 7, CodeID: 88, AmountCents: 0, BatchNo: "RCB202607240001"},
		"batch":  {UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "  "},
	}
	for name, input := range invalidInputs {
		t.Run(name, func(t *testing.T) {
			mock.ExpectBegin()
			mock.ExpectRollback()
			wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, input, now)
			if !errors.Is(err, ErrRedeemCodeInvalidInput) {
				t.Fatalf("expected invalid input sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
			}
		})
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryRedeemCodeCreditLocksActiveWalletAndUpdatesRechargeTotals(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	expectWalletByUserQuery(mock, 7).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(1000), int64(1500), int64(200), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{
		UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "  RCB202607240001  ",
	}, now)
	if err != nil {
		t.Fatalf("CreditRedeemCodeInTx error=%v", err)
	}
	if wallet == nil || wallet.BalanceCents != 1100 || wallet.TotalRechargeCents != 1600 || wallet.TotalConsumeCents != 200 {
		t.Fatalf("unexpected credited wallet=%#v", wallet)
	}
	if transaction == nil || transaction.Direction != DirectionIn || transaction.SourceType != SourceRedeemCode || transaction.SourceID != 88 || transaction.Remark != "RCB202607240001" {
		t.Fatalf("unexpected redeem code transaction=%#v", transaction)
	}
	if transaction.BalanceBeforeCents != 1000 || transaction.BalanceAfterCents != 1100 || transaction.AmountCents != 100 {
		t.Fatalf("unexpected redeem code balance history=%#v", transaction)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryRedeemCodeCreditCreatesAndLocksWalletOnlyWhenNoWalletFactExists(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	expectWalletByUserQuery(mock, 7).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_wallets`")).WillReturnResult(sqlmock.NewResult(1, 1))
	expectWalletByIDQuery(mock, 1, true).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(0), int64(0), int64(0), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}, now)
	if err != nil {
		t.Fatalf("CreditRedeemCodeInTx error=%v", err)
	}
	if wallet == nil || wallet.ID != 1 || wallet.UserID != 7 || wallet.BalanceCents != 100 || wallet.TotalRechargeCents != 100 {
		t.Fatalf("unexpected created wallet=%#v", wallet)
	}
	if transaction == nil || transaction.WalletID != 1 || transaction.UserID != 7 {
		t.Fatalf("unexpected transaction for created wallet=%#v", transaction)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryRedeemCodeCreditRejectsSoftDeletedWalletFact(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	expectWalletByUserQuery(mock, 7).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(0), int64(0), int64(0), enum.CommonYes, now, now))
	mock.ExpectRollback()

	wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}, now)
	if !errors.Is(err, ErrRedeemCodeWalletIntegrity) {
		t.Fatalf("expected soft-deleted wallet integrity sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryRedeemCodeCreditClassifiesDuplicateSoftDeletedWalletRace(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	expectWalletByUserQuery(mock, 7).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_wallets`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry '7' for key 'uk_user_wallet_user'"))
	expectWalletByUserQuery(mock, 7).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(0), int64(0), int64(0), enum.CommonYes, now, now))
	mock.ExpectRollback()

	wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}, now)
	if !errors.Is(err, ErrRedeemCodeWalletIntegrity) {
		t.Fatalf("expected duplicate soft-deleted wallet integrity sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryRedeemCodeCreditRejectsAnyExistingSourceFact(t *testing.T) {
	for name, isDel := range map[string]int{"active": enum.CommonNo, "soft_deleted": enum.CommonYes} {
		t.Run(name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectRedeemCodeSourceQuery(mock, 88, false).WillReturnRows(redeemCodeTransactionRows().
				AddRow(int64(9), "WLT20260724120000000001", int64(1), int64(7), DirectionIn, int64(100), int64(1000), int64(1100), SourceRedeemCode, int64(88), "RCB202607240001", isDel, now, now))
			mock.ExpectRollback()

			wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}, now)
			if !errors.Is(err, ErrRedeemCodeSourceExists) {
				t.Fatalf("expected existing source sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestRepositoryRedeemCodeCreditClassifiesDuplicateSourceRace(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	expectWalletByUserQuery(mock, 7).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(1000), int64(1500), int64(200), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).
		WillReturnError(errors.New("Error 1062 (23000): Duplicate entry 'redeem_code-88' for key 'uk_wallet_transaction_source'"))
	mock.ExpectRollback()

	wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}, now)
	if !errors.Is(err, ErrRedeemCodeSourceExists) {
		t.Fatalf("expected duplicate source sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryRedeemCodeCreditRejectsBalanceAndRechargeOverflowBeforeWriting(t *testing.T) {
	tests := []struct {
		name          string
		balance       int64
		totalRecharge int64
		wantErr       error
	}{
		{name: "balance", balance: math.MaxInt64, totalRecharge: 100, wantErr: ErrRedeemCodeBalanceOverflow},
		{name: "total_recharge", balance: 100, totalRecharge: math.MaxInt64, wantErr: ErrRedeemCodeTotalRechargeOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
			expectWalletByUserQuery(mock, 7).WillReturnRows(redeemCodeWalletRows().
				AddRow(int64(1), int64(7), tt.balance, tt.totalRecharge, int64(0), enum.CommonNo, now, now))
			mock.ExpectRollback()

			wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 1, BatchNo: "RCB202607240001"}, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected controlled overflow sentinel %v, wallet=%#v transaction=%#v err=%v", tt.wantErr, wallet, transaction, err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestRepositoryRedeemCodeCreditLeavesRollbackToOuterTransactionAfterWalletUpdateFailure(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	updateErr := errors.New("wallet update failed")
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	expectWalletByUserQuery(mock, 7).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(1000), int64(1500), int64(200), enum.CommonNo, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `wallet_transactions`")).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_wallets` SET")).WillReturnError(updateErr)
	mock.ExpectRollback()

	wallet, transaction, err := runRedeemCodeCreditInOuterTransaction(repo, RedeemCodeCreditInput{UserID: 7, CodeID: 88, AmountCents: 100, BatchNo: "RCB202607240001"}, now)
	if !errors.Is(err, updateErr) {
		t.Fatalf("expected wallet update error to reach outer transaction, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryFindRedeemCodeCreditInTxReturnsOriginalFactsWithoutCurrentBalanceEquality(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, true).WillReturnRows(redeemCodeTransactionRows().
		AddRow(int64(9), "WLT20260724120000000001", int64(1), int64(7), DirectionIn, int64(100), int64(1000), int64(1100), SourceRedeemCode, int64(88), "RCB202607240001", enum.CommonNo, now, now))
	expectWalletByIDQuery(mock, 1, true).WillReturnRows(redeemCodeWalletRows().
		AddRow(int64(1), int64(7), int64(1750), int64(2000), int64(250), enum.CommonNo, now, now))
	mock.ExpectCommit()

	wallet, transaction, err := runFindRedeemCodeCreditInOuterTransaction(repo, 88, true)
	if err != nil {
		t.Fatalf("FindRedeemCodeCreditInTx error=%v", err)
	}
	if transaction == nil || transaction.ID != 9 || transaction.BalanceAfterCents != 1100 {
		t.Fatalf("unexpected original transaction=%#v", transaction)
	}
	if wallet == nil || wallet.ID != 1 || wallet.BalanceCents != 1750 {
		t.Fatalf("expected current wallet despite later balance changes, got %#v", wallet)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryFindRedeemCodeCreditInTxReturnsNilWhenNoFactExists(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectCommit()

	wallet, transaction, err := runFindRedeemCodeCreditInOuterTransaction(repo, 88, false)
	if err != nil || wallet != nil || transaction != nil {
		t.Fatalf("expected no redeem source fact, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryFindRedeemCodeCreditInTxRejectsSoftDeletedTransaction(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	expectRedeemCodeSourceQuery(mock, 88, false).WillReturnRows(redeemCodeTransactionRows().
		AddRow(int64(9), "WLT20260724120000000001", int64(1), int64(7), DirectionIn, int64(100), int64(1000), int64(1100), SourceRedeemCode, int64(88), "RCB202607240001", enum.CommonYes, now, now))
	mock.ExpectRollback()

	wallet, transaction, err := runFindRedeemCodeCreditInOuterTransaction(repo, 88, false)
	if !errors.Is(err, ErrRedeemCodeWalletIntegrity) || wallet != nil || transaction != nil {
		t.Fatalf("expected soft-deleted transaction integrity sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryFindRedeemCodeCreditInTxRejectsMissingOrSoftDeletedWallet(t *testing.T) {
	for name, walletResult := range map[string]func(*sqlmock.ExpectedQuery){
		"missing": func(query *sqlmock.ExpectedQuery) { query.WillReturnError(gorm.ErrRecordNotFound) },
		"soft_deleted": func(query *sqlmock.ExpectedQuery) {
			now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
			query.WillReturnRows(redeemCodeWalletRows().AddRow(int64(1), int64(7), int64(1100), int64(1600), int64(0), enum.CommonYes, now, now))
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo, mock, closeDB := newMockRepository(t)
			defer closeDB()
			now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			expectRedeemCodeSourceQuery(mock, 88, false).WillReturnRows(redeemCodeTransactionRows().
				AddRow(int64(9), "WLT20260724120000000001", int64(1), int64(7), DirectionIn, int64(100), int64(1000), int64(1100), SourceRedeemCode, int64(88), "RCB202607240001", enum.CommonNo, now, now))
			walletResult(expectWalletByIDQuery(mock, 1, false))
			mock.ExpectRollback()

			wallet, transaction, err := runFindRedeemCodeCreditInOuterTransaction(repo, 88, false)
			if !errors.Is(err, ErrRedeemCodeWalletIntegrity) || wallet != nil || transaction != nil {
				t.Fatalf("expected associated wallet integrity sentinel, wallet=%#v transaction=%#v err=%v", wallet, transaction, err)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func runRedeemCodeCreditInOuterTransaction(repo *GormRepository, input RedeemCodeCreditInput, now time.Time) (resultWallet *Wallet, resultTransaction *Transaction, resultErr error) {
	resultErr = repo.db.Transaction(func(tx *gorm.DB) error {
		resultWallet, resultTransaction, resultErr = repo.CreditRedeemCodeInTx(context.Background(), tx, input, now)
		return resultErr
	})
	return resultWallet, resultTransaction, resultErr
}

func runFindRedeemCodeCreditInOuterTransaction(repo *GormRepository, codeID int64, lock bool) (resultWallet *Wallet, resultTransaction *Transaction, resultErr error) {
	resultErr = repo.db.Transaction(func(tx *gorm.DB) error {
		resultWallet, resultTransaction, resultErr = repo.FindRedeemCodeCreditInTx(context.Background(), tx, codeID, lock)
		return resultErr
	})
	return resultWallet, resultTransaction, resultErr
}

func expectRedeemCodeSourceQuery(mock sqlmock.Sqlmock, codeID int64, lock bool) *sqlmock.ExpectedQuery {
	query := "SELECT * FROM `wallet_transactions` WHERE source_type = ? AND source_id = ? ORDER BY `wallet_transactions`.`id` LIMIT ?"
	if lock {
		query += " FOR UPDATE"
	}
	return mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(SourceRedeemCode, int64(codeID), 1)
}

func expectWalletByUserQuery(mock sqlmock.Sqlmock, userID int64) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_wallets` WHERE user_id = ? ORDER BY `user_wallets`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(int64(userID), 1)
}

func expectWalletByIDQuery(mock sqlmock.Sqlmock, walletID int64, lock bool) *sqlmock.ExpectedQuery {
	query := "SELECT * FROM `user_wallets` WHERE id = ? ORDER BY `user_wallets`.`id` LIMIT ?"
	if lock {
		query += " FOR UPDATE"
	}
	return mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(int64(walletID), 1)
}

func redeemCodeWalletRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "balance_cents", "total_recharge_cents", "total_consume_cents", "is_del", "created_at", "updated_at"})
}

func redeemCodeTransactionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "transaction_no", "wallet_id", "user_id", "direction", "amount_cents", "balance_before_cents", "balance_after_cents", "source_type", "source_id", "remark", "is_del", "created_at", "updated_at"})
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
