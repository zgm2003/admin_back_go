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
	wallet           Wallet
	transaction      Transaction
	transactions     []TransactionWithUser
	wallets          []WalletWithUser
	consumeErr       error
	consumeInput     ConsumeInput
	transactionQuery TransactionListQuery
	walletUserQuery  WalletUserListQuery
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
