package wallet

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	walletRows       []ListRow
	walletTotal      int64
	transactionRows  []TransactionRow
	transactionTotal int64
	lastListQuery    ListQuery
	lastTxnQuery     TransactionListQuery
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.lastListQuery = query
	return f.walletRows, f.walletTotal, nil
}

func (f *fakeRepository) Transactions(ctx context.Context, query TransactionListQuery) ([]TransactionRow, int64, error) {
	f.lastTxnQuery = query
	return f.transactionRows, f.transactionTotal, nil
}

func TestInitReturnsWalletDicts(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.WalletTypeArr) != len(enum.WalletTypes) || got.Dict.WalletTypeArr[0].Value != enum.WalletTypeRecharge {
		t.Fatalf("unexpected wallet type dict: %#v", got.Dict.WalletTypeArr)
	}
	if len(got.Dict.WalletSourceArr) != len(enum.WalletSources) || got.Dict.WalletSourceArr[2].Value != enum.WalletSourceManual {
		t.Fatalf("unexpected wallet source dict: %#v", got.Dict.WalletSourceArr)
	}
}

func TestListDefaultsPageAndFormatsWalletRows(t *testing.T) {
	createdAt := time.Date(2026, 5, 6, 9, 30, 0, 0, time.Local)
	repo := &fakeRepository{walletRows: []ListRow{{
		ID: 1, UserID: 7, UserName: "admin", UserEmail: "demo@example.test",
		Balance: 1000, Frozen: 200, TotalRecharge: 3000, TotalConsume: 1800, CreatedAt: createdAt,
	}}, walletTotal: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.lastListQuery.CurrentPage != 1 || repo.lastListQuery.PageSize != 20 {
		t.Fatalf("expected service defaults, got %#v", repo.lastListQuery)
	}
	if got.Page.CurrentPage != 1 || got.Page.PageSize != 20 || got.Page.TotalPage != 1 || got.Page.Total != 1 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
	if len(got.List) != 1 {
		t.Fatalf("unexpected list: %#v", got.List)
	}
	item := got.List[0]
	if item.UserName != "admin" || item.Balance != 1000 || item.Frozen != 200 || item.CreatedAt != "2026-05-06 09:30:00" {
		t.Fatalf("unexpected wallet item: %#v", item)
	}
}

func TestTransactionsDefaultsPageAndMapsTypeText(t *testing.T) {
	createdAt := time.Date(2026, 5, 6, 10, 20, 30, 0, time.Local)
	repo := &fakeRepository{transactionRows: []TransactionRow{{
		ID: 9, UserID: 7, UserName: "admin", UserEmail: "demo@example.test",
		BizActionNo: "WALLET:ADJUST:test", Type: enum.WalletTypeAdjust,
		AvailableDelta: -100, FrozenDelta: 0, BalanceBefore: 1000, BalanceAfter: 900,
		OrderNo: "", Title: "系统调账", Remark: "probe", CreatedAt: createdAt,
	}}, transactionTotal: 1}
	service := NewService(repo)

	got, appErr := service.Transactions(context.Background(), TransactionListQuery{})
	if appErr != nil {
		t.Fatalf("expected transactions to succeed, got %v", appErr)
	}
	if repo.lastTxnQuery.CurrentPage != 1 || repo.lastTxnQuery.PageSize != 20 {
		t.Fatalf("expected service defaults, got %#v", repo.lastTxnQuery)
	}
	if got.Page.Total != 1 || len(got.List) != 1 {
		t.Fatalf("unexpected transaction response: %#v", got)
	}
	item := got.List[0]
	if item.TypeText != "系统调账" || item.AvailableDelta != -100 || item.CreatedAt != "2026-05-06 10:20:30" {
		t.Fatalf("unexpected transaction item: %#v", item)
	}
}

func TestListRepositoryNotConfiguredFailsClearly(t *testing.T) {
	service := NewService(nil)

	_, appErr := service.List(context.Background(), ListQuery{})
	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected repository config error, got %#v", appErr)
	}
}
