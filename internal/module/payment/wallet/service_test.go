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
	if result.Balance != "0" || result.TotalRecharge != "0" || result.TotalConsume != "0" {
		t.Fatalf("unexpected summary=%#v", result)
	}
	if repo.wallet.UserID != 7 {
		t.Fatalf("expected wallet user id 7, got %#v", repo.wallet)
	}
}

func TestServiceDebitRejectsInvalidAmount(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountUnits: 0, SourceType: SourceAIGenerate, SourceID: 88})
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

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountUnits: 100, SourceType: "manual", SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.debit.source_type.invalid" {
		t.Fatalf("expected debit source_type invalid keyed error, got %v", appErr)
	}
	if repo.debitCalled {
		t.Fatalf("invalid source_type must not call repository")
	}
}

func TestServiceDebitRejectsAIGenerateSource(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Debit(context.Background(), MutationInput{UserID: 7, AmountUnits: 100, SourceType: SourceAIGenerate, SourceID: 88, Remark: " billing "})
	if appErr == nil || appErr.MessageID != "wallet.debit.source_type.invalid" {
		t.Fatalf("generic AI debit must be rejected, got %v", appErr)
	}
	if repo.debitCalled {
		t.Fatal("AI generation must be captured through a hold, not generic Debit")
	}
}

func TestServiceCreditRejectsUnsupportedSource(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Credit(context.Background(), MutationInput{UserID: 7, AmountUnits: 100, SourceType: "unsupported", SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.credit.source_type.invalid" || repo.creditCalled {
		t.Fatalf("unsupported credit must be rejected, appErr=%v called=%v", appErr, repo.creditCalled)
	}
}

func TestServiceCreditRejectsRechargeSourceType(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Credit(context.Background(), MutationInput{UserID: 7, AmountUnits: 100, SourceType: SourceRecharge, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.credit.source_type.invalid" {
		t.Fatalf("expected credit source_type invalid keyed error, got %v", appErr)
	}
	if repo.creditCalled {
		t.Fatalf("generic wallet credit must not handle recharge; recharge uses payment atomic finalization")
	}
}

func TestServiceCreditRejectsRedeemCodeSourceType(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo)

	_, appErr := service.Credit(context.Background(), MutationInput{UserID: 7, AmountUnits: 100, SourceType: SourceRedeemCode, SourceID: 88})
	if appErr == nil || appErr.MessageID != "wallet.credit.source_type.invalid" {
		t.Fatalf("expected credit source_type invalid keyed error, got %v", appErr)
	}
	if repo.creditCalled {
		t.Fatalf("generic wallet credit must not handle redeem codes")
	}
}

func TestServiceRedeemCodeSourceTypeText(t *testing.T) {
	if got := sourceTypeText(SourceRedeemCode); got != "兑换码充值" {
		t.Fatalf("unexpected redeem code source text=%q", got)
	}
}

func TestWalletDictExposesOnlyCurrentContractSourceTypes(t *testing.T) {
	dict := walletDict()
	values := make([]string, 0, len(dict.SourceTypeArr))
	for _, option := range dict.SourceTypeArr {
		values = append(values, option.Value)
		if option.Value == "consume" {
			t.Fatalf("legacy consume source must not be exposed in selectable dict: %#v", dict.SourceTypeArr)
		}
	}

	want := []string{SourceRecharge, SourceAIGenerate, SourceRedeemCode}
	if len(values) != len(want) {
		t.Fatalf("unexpected source type count, got=%#v want=%#v", values, want)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("unexpected source type order, got=%#v want=%#v", values, want)
		}
	}
}

func TestServiceListsNormalizeFilters(t *testing.T) {
	repo := &fakeRepo{
		transactions: []TransactionWithUser{{Transaction: Transaction{ID: 1, Direction: DirectionIn, SourceType: SourceRecharge, AmountUnits: 1234, CreatedAt: time.Date(2026, 5, 21, 1, 2, 3, 0, time.UTC)}, Username: "u1", Phone: "15671628271"}},
		wallets:      []WalletWithUser{{Wallet: Wallet{ID: 2, UserID: 7, BalanceUnits: 1234, UpdatedAt: time.Date(2026, 5, 21, 1, 2, 3, 0, time.UTC)}, Username: "u1", Phone: "15671628271"}},
	}
	service := NewService(repo)

	txs, appErr := service.Transactions(context.Background(), TransactionListQuery{CurrentPage: -1, PageSize: 999, UserID: 7, Direction: DirectionIn, SourceType: SourceRecharge, Keyword: " u "})
	if appErr != nil {
		t.Fatalf("Transactions error=%v", appErr)
	}
	if txs.Page.CurrentPage != 1 || txs.Page.PageSize != maxPageSize || txs.List[0].Amount != "0.00001234" || repo.transactionQuery.Keyword != "u" {
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
	debitTransaction         Transaction
	creditTransaction        Transaction
	transactions             []TransactionWithUser
	wallets                  []WalletWithUser
	debitErr                 error
	creditErr                error
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
