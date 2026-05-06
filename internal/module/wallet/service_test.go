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
	adjustmentResult *AdjustmentResult
	adjustmentErr    error
	lastAdjustment   AdjustmentMutation
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.lastListQuery = query
	return f.walletRows, f.walletTotal, nil
}

func (f *fakeRepository) Transactions(ctx context.Context, query TransactionListQuery) ([]TransactionRow, int64, error) {
	f.lastTxnQuery = query
	return f.transactionRows, f.transactionTotal, nil
}

func (f *fakeRepository) CreateAdjustment(ctx context.Context, input AdjustmentMutation) (*AdjustmentResult, error) {
	f.lastAdjustment = input
	if f.adjustmentErr != nil {
		return nil, f.adjustmentErr
	}
	if f.adjustmentResult != nil {
		return f.adjustmentResult, nil
	}
	return &AdjustmentResult{
		TransactionID: 1,
		BizActionNo:   input.BizActionNo,
		BalanceBefore: 1000,
		BalanceAfter:  1000 + input.Delta,
	}, nil
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

func TestCreateAdjustmentValidatesInput(t *testing.T) {
	service := NewService(&fakeRepository{})
	tests := []struct {
		name  string
		input CreateAdjustmentInput
	}{
		{name: "missing user", input: CreateAdjustmentInput{Delta: 1, Reason: "修正", IdempotencyKey: "idem-0001", OperatorID: 1}},
		{name: "zero delta", input: CreateAdjustmentInput{UserID: 7, Delta: 0, Reason: "修正", IdempotencyKey: "idem-0001", OperatorID: 1}},
		{name: "blank reason", input: CreateAdjustmentInput{UserID: 7, Delta: 1, Reason: "  ", IdempotencyKey: "idem-0001", OperatorID: 1}},
		{name: "bad idempotency", input: CreateAdjustmentInput{UserID: 7, Delta: 1, Reason: "修正", IdempotencyKey: "bad key with spaces", OperatorID: 1}},
		{name: "missing operator", input: CreateAdjustmentInput{UserID: 7, Delta: 1, Reason: "修正", IdempotencyKey: "idem-0001", OperatorID: 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, appErr := service.CreateAdjustment(context.Background(), tc.input)
			if appErr == nil || appErr.Code != apperror.CodeBadRequest {
				t.Fatalf("expected bad request, got %#v", appErr)
			}
		})
	}
}

func TestCreateAdjustmentNormalizesInputAndReturnsResponse(t *testing.T) {
	repo := &fakeRepository{adjustmentResult: &AdjustmentResult{
		TransactionID: 9,
		BizActionNo:   "WALLET:ADJUST:idem-0001",
		BalanceBefore: 1000,
		BalanceAfter:  1100,
	}}
	service := NewService(repo)

	got, appErr := service.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		UserID:         7,
		Delta:          100,
		Reason:         "  人工修正  ",
		IdempotencyKey: " idem-0001 ",
		OperatorID:     3,
	})
	if appErr != nil {
		t.Fatalf("expected success, got %v", appErr)
	}
	if repo.lastAdjustment.Reason != "人工修正" || repo.lastAdjustment.BizActionNo != "WALLET:ADJUST:idem-0001" {
		t.Fatalf("unexpected mutation input: %#v", repo.lastAdjustment)
	}
	if got.TransactionID != 9 || got.BalanceBefore != 1000 || got.BalanceAfter != 1100 {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestCreateAdjustmentMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{name: "user not found", err: ErrUserNotFound, code: apperror.CodeNotFound, msg: "用户不存在"},
		{name: "insufficient", err: ErrInsufficientBalance, code: apperror.CodeBadRequest, msg: "可用余额不足，无法调减"},
		{name: "conflict", err: ErrAdjustmentConflict, code: apperror.CodeBadRequest, msg: "幂等键已被不同请求使用"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(&fakeRepository{adjustmentErr: tc.err})
			_, appErr := service.CreateAdjustment(context.Background(), CreateAdjustmentInput{
				UserID:         7,
				Delta:          100,
				Reason:         "修正",
				IdempotencyKey: "idem-0001",
				OperatorID:     3,
			})
			if appErr == nil || appErr.Code != tc.code || appErr.Message != tc.msg {
				t.Fatalf("unexpected appErr: %#v", appErr)
			}
		})
	}
}
