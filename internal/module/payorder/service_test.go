package payorder

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	listRows           []ListRow
	total              int64
	counts             map[int]int64
	detail             *DetailRow
	items              []OrderItem
	row                *Order
	lastListQuery      ListQuery
	lastStatusQuery    StatusCountQuery
	lastDetailID       int64
	remarkID           int64
	remark             string
	remarkAffected     int64
	closeOrderID       int64
	closeStatus        int
	closeReason        string
	closeAffected      int64
	closedTxnOrderID   int64
	closedTxnAt        time.Time
	withTxCalled       bool
	err                error
}

func (f *fakeRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	f.withTxCalled = true
	return fn(f)
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.lastListQuery = query
	return f.listRows, f.total, f.err
}

func (f *fakeRepository) CountByStatus(ctx context.Context, query StatusCountQuery) (map[int]int64, error) {
	f.lastStatusQuery = query
	return f.counts, f.err
}

func (f *fakeRepository) Detail(ctx context.Context, id int64) (*DetailRow, error) {
	f.lastDetailID = id
	return f.detail, f.err
}

func (f *fakeRepository) Items(ctx context.Context, orderID int64) ([]OrderItem, error) {
	return f.items, f.err
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Order, error) {
	return f.row, f.err
}

func (f *fakeRepository) UpdateRemark(ctx context.Context, id int64, remark string) (int64, error) {
	f.remarkID = id
	f.remark = remark
	return f.remarkAffected, f.err
}

func (f *fakeRepository) CloseOrder(ctx context.Context, id int64, currentStatus int, reason string, now time.Time) (int64, error) {
	f.closeOrderID = id
	f.closeStatus = currentStatus
	f.closeReason = reason
	return f.closeAffected, f.err
}

func (f *fakeRepository) CloseLastActiveTransaction(ctx context.Context, orderID int64, now time.Time) (int64, error) {
	f.closedTxnOrderID = orderID
	f.closedTxnAt = now
	return 1, f.err
}

func TestInitReturnsPayOrderDicts(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.ChannelArr) != 2 || len(got.Dict.PayMethodArr) != 6 || len(got.Dict.OrderTypeArr) != 3 || len(got.Dict.PayStatusArr) != 5 || len(got.Dict.BizStatusArr) != 6 || len(got.Dict.RechargePresetArr) != 6 {
		t.Fatalf("unexpected dict counts: %#v", got.Dict)
	}
}

func TestStatusCountReturnsAllPayStatuses(t *testing.T) {
	repo := &fakeRepository{counts: map[int]int64{enum.PayStatusPaid: 2}}
	service := NewService(repo)

	got, appErr := service.StatusCount(context.Background(), StatusCountQuery{OrderNo: " R1 "})
	if appErr != nil {
		t.Fatalf("expected status count to succeed, got %v", appErr)
	}
	if repo.lastStatusQuery.OrderNo != "R1" {
		t.Fatalf("expected trimmed status query, got %#v", repo.lastStatusQuery)
	}
	if len(got) != len(enum.PayStatuses) || got[2].Value != enum.PayStatusPaid || got[2].Count != 2 {
		t.Fatalf("unexpected status counts: %#v", got)
	}
}

func TestListDefaultsPageAndMapsLabels(t *testing.T) {
	payTime := time.Date(2026, 4, 13, 18, 45, 1, 0, time.Local)
	createdAt := time.Date(2026, 4, 13, 18, 38, 26, 0, time.Local)
	repo := &fakeRepository{listRows: []ListRow{{
		ID: 3, OrderNo: "R260413183826000005", UserID: 1, UserName: "admin", UserEmail: "demo@example.test",
		OrderType: enum.PayOrderRecharge, Title: "钱包充值 10 元", TotalAmount: 1000, PayAmount: 1000,
		PayStatus: enum.PayStatusPaid, BizStatus: enum.PayBizSuccess, PayTime: &payTime, CreatedAt: createdAt,
	}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.lastListQuery.CurrentPage != 1 || repo.lastListQuery.PageSize != 20 {
		t.Fatalf("unexpected normalized query: %#v", repo.lastListQuery)
	}
	item := got.List[0]
	if item.OrderTypeText != "充值" || item.PayStatusText != "已支付" || item.BizStatusText != "履约成功" {
		t.Fatalf("unexpected labels: %#v", item)
	}
	if item.PayTime == nil || *item.PayTime != "2026-04-13 18:45:01" || item.CreatedAt != "2026-04-13 18:38:26" {
		t.Fatalf("unexpected times: %#v", item)
	}
}

func TestDetailMapsOrderItemsAndJSON(t *testing.T) {
	createdAt := time.Date(2026, 4, 13, 18, 38, 26, 0, time.Local)
	expireAt := time.Date(2026, 4, 13, 19, 8, 26, 0, time.Local)
	repo := &fakeRepository{detail: &DetailRow{
		ID: 3, OrderNo: "R1", UserID: 1, UserName: "admin", UserEmail: "demo@example.test", OrderType: enum.PayOrderRecharge,
		BizType: "wallet", Title: "钱包充值", TotalAmount: 1000, PayAmount: 1000, PayStatus: enum.PayStatusPaid,
		BizStatus: enum.PayBizSuccess, ChannelID: 1, ChannelName: "支付宝", Channel: enum.PayChannelAlipay,
		PayMethod: enum.PayMethodWeb, ExpireTime: expireAt, Extra: `{"source":"test"}`, CreatedAt: createdAt,
	}, items: []OrderItem{{ID: 9, Title: "充值10元", Price: 1000, Quantity: 1, Amount: 1000}}}
	service := NewService(repo)

	got, appErr := service.Detail(context.Background(), 3)
	if appErr != nil {
		t.Fatalf("expected detail to succeed, got %v", appErr)
	}
	if got.Order.Channel == nil || got.Order.Channel.Name != "支付宝" || got.Order.Extra["source"] != "test" || len(got.Items) != 1 {
		t.Fatalf("unexpected detail: %#v", got)
	}
	if got.Order.ExpireTime == nil || *got.Order.ExpireTime != "2026-04-13 19:08:26" {
		t.Fatalf("unexpected expire time: %#v", got.Order.ExpireTime)
	}
}

func TestDetailMissingReturnsNotFound(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.Detail(context.Background(), 404)
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestRemarkUpdatesExistingOrder(t *testing.T) {
	repo := &fakeRepository{row: &Order{ID: 3}, remarkAffected: 1}
	service := NewService(repo)

	appErr := service.Remark(context.Background(), 3, RemarkInput{Remark: "  ok  "})
	if appErr != nil {
		t.Fatalf("expected remark to succeed, got %v", appErr)
	}
	if repo.remarkID != 3 || repo.remark != "ok" {
		t.Fatalf("unexpected remark update: %#v", repo)
	}
}

func TestRemarkRejectsMissingOrder(t *testing.T) {
	service := NewService(&fakeRepository{})

	appErr := service.Remark(context.Background(), 3, RemarkInput{Remark: "ok"})
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestCloseUsesTransactionAndClosesActiveTransaction(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.Local)
	repo := &fakeRepository{row: &Order{ID: 3, PayStatus: enum.PayStatusPending}, closeAffected: 1}
	service := NewService(repo)

	appErr := service.Close(context.Background(), 3, CloseInput{Reason: "  admin  ", Now: now})
	if appErr != nil {
		t.Fatalf("expected close to succeed, got %v", appErr)
	}
	if !repo.withTxCalled || repo.closeOrderID != 3 || repo.closeStatus != enum.PayStatusPending || repo.closeReason != "admin" || repo.closedTxnOrderID != 3 || !repo.closedTxnAt.Equal(now) {
		t.Fatalf("unexpected close flow: %#v", repo)
	}
}

func TestCloseRejectsPaidOrder(t *testing.T) {
	repo := &fakeRepository{row: &Order{ID: 3, PayStatus: enum.PayStatusPaid}}
	service := NewService(repo)

	appErr := service.Close(context.Background(), 3, CloseInput{})
	if appErr == nil || appErr.Message != "该订单状态不允许关闭" {
		t.Fatalf("expected status rejection, got %#v", appErr)
	}
	if repo.closeOrderID != 0 {
		t.Fatalf("paid order must not be closed")
	}
}

func TestRepositoryErrorIsWrapped(t *testing.T) {
	service := NewService(&fakeRepository{err: errors.New("db down")})

	_, appErr := service.List(context.Background(), ListQuery{})
	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
}
