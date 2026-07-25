package wallet

import (
	"context"
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
	wallet           Wallet
	transactions     []TransactionWithUser
	wallets          []WalletWithUser
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
