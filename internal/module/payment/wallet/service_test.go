package wallet

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceSummaryCreatesZeroWallet(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	result, appErr := service.Summary(context.Background(), 7)
	if appErr != nil {
		t.Fatalf("Summary error=%v", appErr)
	}
	if result.BalanceCents != 0 || result.TotalRechargeCents != 0 || result.TotalConsumeCents != 0 {
		t.Fatalf("unexpected summary=%#v", result)
	}
	if repo.wallet.UserID != 7 {
		t.Fatalf("expected wallet user id 7, got %#v", repo.wallet)
	}
}

func TestServiceConsumeRejectsInsufficientBalance(t *testing.T) {
	repo := &fakeRepo{consumeErr: ErrInsufficientBalance}
	service := NewService(repo)

	_, appErr := service.Consume(context.Background(), ConsumeInput{UserID: 7, AmountCents: 100, SourceID: 1})
	if appErr == nil || appErr.Message != "余额不足" {
		t.Fatalf("expected insufficient balance, got %v", appErr)
	}
}

func TestServiceConsumeRejectsSourceOwnedByAnotherUser(t *testing.T) {
	repo := &fakeRepo{consumeErr: ErrConsumeSourceOwnerMismatch}
	service := NewService(repo)

	_, appErr := service.Consume(context.Background(), ConsumeInput{UserID: 8, AmountCents: 100, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.consume.source_id.owner_mismatch" {
		t.Fatalf("expected source owner mismatch keyed error, got %v", appErr)
	}
}

func TestServiceConsumeReturnsTransactionAndWallet(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		wallet:      Wallet{ID: 1, UserID: 7, BalanceCents: 900, TotalRechargeCents: 1000, TotalConsumeCents: 100},
		transaction: Transaction{ID: 9, TransactionNo: "WLT20260521120000000001", WalletID: 1, UserID: 7, Direction: DirectionOut, AmountCents: 100, BalanceBeforeCents: 1000, BalanceAfterCents: 900, SourceType: SourceConsume, SourceID: 3, Remark: "test", CreatedAt: now},
	}
	service := NewService(repo)

	result, appErr := service.Consume(context.Background(), ConsumeInput{UserID: 7, AmountCents: 100, SourceID: 3, Remark: " test "})
	if appErr != nil {
		t.Fatalf("Consume error=%v", appErr)
	}
	if result.Transaction.Direction != DirectionOut || result.Transaction.SourceType != SourceConsume || result.Transaction.AmountText != "1.00" {
		t.Fatalf("unexpected transaction=%#v", result.Transaction)
	}
	if result.Wallet.BalanceCents != 900 || result.Wallet.TotalConsumeCents != 100 {
		t.Fatalf("unexpected wallet=%#v", result.Wallet)
	}
	if repo.consumeInput.Remark != "test" {
		t.Fatalf("expected trimmed remark, got %q", repo.consumeInput.Remark)
	}
}

func TestServiceDebitRejectsInvalidAmount(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 0, SourceType: SourceAIGenerate, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.debit.amount.invalid" {
		t.Fatalf("expected debit amount invalid keyed error, got %v", appErr)
	}
	if repo.debitCalled {
		t.Fatalf("invalid amount must not call repository")
	}
}

func TestServiceDebitRejectsInvalidSourceType(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: "manual", SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.debit.source_type.invalid" {
		t.Fatalf("expected debit source_type invalid keyed error, got %v", appErr)
	}
	if repo.debitCalled {
		t.Fatalf("invalid source_type must not call repository")
	}
}

func TestServiceDebitRejectsInsufficientBalanceWithoutTransaction(t *testing.T) {
	repo := &fakeRepo{debitErr: ErrInsufficientBalance}
	service := NewService(repo)

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.debit.insufficient_balance" {
		t.Fatalf("expected debit insufficient balance keyed error, got %v", appErr)
	}
	if !repo.debitCalled {
		t.Fatalf("valid debit must call repository")
	}
	if repo.debitReturnedTransaction {
		t.Fatalf("insufficient balance must not write or return a transaction")
	}
}

func TestServiceDebitWritesAIGenerateOutTransaction(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		wallet:           Wallet{ID: 1, UserID: 7, BalanceCents: 900, TotalRechargeCents: 1000, TotalConsumeCents: 100},
		debitTransaction: Transaction{ID: 9, TransactionNo: "WLT20260530120000000001", WalletID: 1, UserID: 7, Direction: DirectionOut, AmountCents: 100, BalanceBeforeCents: 1000, BalanceAfterCents: 900, SourceType: SourceAIGenerate, SourceID: 88, Remark: "billing", CreatedAt: now},
	}
	service := NewService(repo)

	result, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88, Remark: " billing "})
	if appErr != nil {
		t.Fatalf("Debit error=%v", appErr)
	}
	if result.Transaction.Direction != DirectionOut || result.Transaction.SourceType != SourceAIGenerate || result.Transaction.SourceID != 88 {
		t.Fatalf("expected ai_generate debit out transaction, got %#v", result.Transaction)
	}
	if result.Wallet.BalanceCents != 900 || result.Wallet.TotalConsumeCents != 100 {
		t.Fatalf("unexpected wallet=%#v", result.Wallet)
	}
	if repo.debitInput.Remark != "billing" {
		t.Fatalf("expected trimmed remark, got %q", repo.debitInput.Remark)
	}
}

func TestServiceCreditWritesAIRefundInTransaction(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		wallet:            Wallet{ID: 1, UserID: 7, BalanceCents: 1100, TotalRechargeCents: 1000, TotalConsumeCents: 100},
		creditTransaction: Transaction{ID: 10, TransactionNo: "WLT20260530120000000002", WalletID: 1, UserID: 7, Direction: DirectionIn, AmountCents: 100, BalanceBeforeCents: 1000, BalanceAfterCents: 1100, SourceType: SourceAIRefund, SourceID: 88, Remark: "refund", CreatedAt: now},
	}
	service := NewService(repo)

	result, appErr := service.Credit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIRefund, SourceID: 88, Remark: " refund "})
	if appErr != nil {
		t.Fatalf("Credit error=%v", appErr)
	}
	if result.Transaction.Direction != DirectionIn || result.Transaction.SourceType != SourceAIRefund || result.Transaction.SourceID != 88 {
		t.Fatalf("expected ai_refund credit in transaction, got %#v", result.Transaction)
	}
	if result.Wallet.BalanceCents != 1100 || result.Wallet.TotalConsumeCents != 100 {
		t.Fatalf("credit must not decrement total_consume_cents, wallet=%#v", result.Wallet)
	}
	if repo.creditInput.Remark != "refund" {
		t.Fatalf("expected trimmed remark, got %q", repo.creditInput.Remark)
	}
}

func TestServiceCreditRejectsRechargeSourceType(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Credit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceRecharge, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.credit.source_type.invalid" {
		t.Fatalf("expected credit source_type invalid keyed error, got %v", appErr)
	}
	if repo.creditCalled {
		t.Fatalf("generic wallet credit must not handle recharge; recharge uses payment CreditRecharge")
	}
}

func TestServiceCreditReturnsExistingTransactionForSameSource(t *testing.T) {
	repo := &fakeRepo{
		wallet:            Wallet{ID: 1, UserID: 7, BalanceCents: 1100, TotalRechargeCents: 1000, TotalConsumeCents: 100},
		creditTransaction: Transaction{ID: 10, TransactionNo: "WLT20260530120000000002", WalletID: 1, UserID: 7, Direction: DirectionIn, AmountCents: 100, BalanceBeforeCents: 1000, BalanceAfterCents: 1100, SourceType: SourceAIRefund, SourceID: 88, Remark: "already refunded"},
	}
	service := NewService(repo)

	result, appErr := service.Credit(context.Background(), MutationInput{UserID: 7, AmountCents: 100, SourceType: SourceAIRefund, SourceID: 88})
	if appErr != nil {
		t.Fatalf("Credit error=%v", appErr)
	}
	if result.Transaction.ID != 10 || result.Transaction.SourceType != SourceAIRefund || result.Transaction.SourceID != 88 {
		t.Fatalf("expected existing same-source credit transaction, got %#v", result.Transaction)
	}
}

func TestWalletDictExposesOnlyCurrentContractSourceTypes(t *testing.T) {
	dict := walletDict()
	values := make([]string, 0, len(dict.SourceTypeArr))
	for _, option := range dict.SourceTypeArr {
		values = append(values, option.Value)
		if option.Value == SourceConsume {
			t.Fatalf("legacy consume source must not be exposed in selectable dict: %#v", dict.SourceTypeArr)
		}
	}

	want := []string{SourceRecharge, SourceAIGenerate, SourceAIRefund}
	if len(values) != len(want) {
		t.Fatalf("unexpected source type count, got=%#v want=%#v", values, want)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("unexpected source type order, got=%#v want=%#v", values, want)
		}
	}
}

func TestServiceMutationRejectsSameSourceOwnedByAnotherUser(t *testing.T) {
	repo := &fakeRepo{debitErr: ErrMutationSourceOwnerMismatch}
	service := NewService(repo)

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 8, AmountCents: 100, SourceType: SourceAIGenerate, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.mutation.source_id.owner_mismatch" {
		t.Fatalf("expected mutation source owner mismatch keyed error, got %v", appErr)
	}
}

func TestServiceListsNormalizeFilters(t *testing.T) {
	repo := &fakeRepo{
		transactions: []TransactionWithUser{{Transaction: Transaction{ID: 1, Direction: DirectionIn, SourceType: SourceRecharge, AmountCents: 1234, CreatedAt: time.Date(2026, 5, 21, 1, 2, 3, 0, time.UTC)}, Username: "u1", Phone: "15671628271"}},
		wallets:      []WalletWithUser{{Wallet: Wallet{ID: 2, UserID: 7, BalanceCents: 1234, UpdatedAt: time.Date(2026, 5, 21, 1, 2, 3, 0, time.UTC)}, Username: "u1", Phone: "15671628271"}},
	}
	service := NewService(repo)

	txs, appErr := service.Transactions(context.Background(), TransactionListQuery{CurrentPage: -1, PageSize: 999, UserID: 7, Direction: DirectionIn, SourceType: SourceRecharge, Keyword: " u "})
	if appErr != nil {
		t.Fatalf("Transactions error=%v", appErr)
	}
	if txs.Page.CurrentPage != 1 || txs.Page.PageSize != maxPageSize || txs.List[0].AmountText != "12.34" || repo.transactionQuery.Keyword != "u" {
		t.Fatalf("unexpected transaction list=%#v query=%#v", txs, repo.transactionQuery)
	}

	users, appErr := service.WalletUsers(context.Background(), WalletUserListQuery{CurrentPage: 1, PageSize: 20, Keyword: " u "})
	if appErr != nil {
		t.Fatalf("WalletUsers error=%v", appErr)
	}
	if users.List[0].ID != 2 || users.List[0].Account != "15671628271" || repo.walletUserQuery.Keyword != "u" {
		t.Fatalf("unexpected wallet users=%#v query=%#v", users, repo.walletUserQuery)
	}
}

type fakeRepo struct {
	wallet                   Wallet
	transaction              Transaction
	debitTransaction         Transaction
	creditTransaction        Transaction
	transactions             []TransactionWithUser
	wallets                  []WalletWithUser
	consumeErr               error
	debitErr                 error
	creditErr                error
	consumeInput             ConsumeInput
	debitInput               MutationInput
	creditInput              MutationInput
	debitCalled              bool
	creditCalled             bool
	debitReturnedTransaction bool
	transactionQuery         TransactionListQuery
	walletUserQuery          WalletUserListQuery
}

func (f *fakeRepo) GetOrCreateWallet(ctx context.Context, userID int64) (*Wallet, error) {
	if f.wallet.UserID == 0 {
		f.wallet = Wallet{ID: 1, UserID: userID}
	}
	return &f.wallet, nil
}

func (f *fakeRepo) ListTransactions(ctx context.Context, query TransactionListQuery) ([]TransactionWithUser, int64, error) {
	f.transactionQuery = query
	return f.transactions, int64(len(f.transactions)), nil
}

func (f *fakeRepo) ListWalletUsers(ctx context.Context, query WalletUserListQuery) ([]WalletWithUser, int64, error) {
	f.walletUserQuery = query
	return f.wallets, int64(len(f.wallets)), nil
}

func (f *fakeRepo) Consume(ctx context.Context, input ConsumeInput, now time.Time) (*Wallet, *Transaction, error) {
	f.consumeInput = input
	if f.consumeErr != nil {
		return nil, nil, f.consumeErr
	}
	if f.transaction.ID == 0 {
		return nil, nil, errors.New("missing fake transaction")
	}
	return &f.wallet, &f.transaction, nil
}

func (f *fakeRepo) Debit(ctx context.Context, input MutationInput, now time.Time) (*Wallet, *Transaction, error) {
	f.debitCalled = true
	f.debitInput = input
	if f.debitErr != nil {
		return nil, nil, f.debitErr
	}
	if f.debitTransaction.ID == 0 {
		return nil, nil, errors.New("missing fake debit transaction")
	}
	f.debitReturnedTransaction = true
	return &f.wallet, &f.debitTransaction, nil
}

func (f *fakeRepo) Credit(ctx context.Context, input MutationInput, now time.Time) (*Wallet, *Transaction, error) {
	f.creditCalled = true
	f.creditInput = input
	if f.creditErr != nil {
		return nil, nil, f.creditErr
	}
	if f.creditTransaction.ID == 0 {
		return nil, nil, errors.New("missing fake credit transaction")
	}
	return &f.wallet, &f.creditTransaction, nil
}
