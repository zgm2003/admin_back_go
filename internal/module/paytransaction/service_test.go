package paytransaction

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	listRows []ListRow
	total    int64
	detail   *DetailRow
	lastQuery ListQuery
	lastID    int64
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.lastQuery = query
	return f.listRows, f.total, nil
}

func (f *fakeRepository) Detail(ctx context.Context, id int64) (*DetailRow, error) {
	f.lastID = id
	return f.detail, nil
}

func TestInitReturnsPayTransactionDicts(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.ChannelArr) != len(enum.PayChannels) || got.Dict.ChannelArr[0].Value != enum.PayChannelWechat {
		t.Fatalf("unexpected channel dict: %#v", got.Dict.ChannelArr)
	}
	if len(got.Dict.TxnStatusArr) != len(enum.PayTxnStatuses) || got.Dict.TxnStatusArr[2].Value != enum.PayTxnSuccess {
		t.Fatalf("unexpected txn status dict: %#v", got.Dict.TxnStatusArr)
	}
}

func TestListDefaultsPageAndMapsLabels(t *testing.T) {
	paidAt := time.Date(2026, 4, 13, 18, 45, 1, 0, time.Local)
	createdAt := time.Date(2026, 4, 13, 18, 38, 26, 0, time.Local)
	repo := &fakeRepository{listRows: []ListRow{{
		ID: 3, TransactionNo: "T260413183826000006", OrderNo: "R260413183826000005", UserID: 1,
		UserName: "admin", UserEmail: "demo@example.test", AttemptNo: 1, ChannelID: 1, Channel: enum.PayChannelAlipay,
		PayMethod: enum.PayMethodWeb, Amount: 1000, Status: enum.PayTxnSuccess, PaidAt: &paidAt, CreatedAt: createdAt,
	}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if got.Page.CurrentPage != 1 || got.Page.PageSize != 20 || got.Page.TotalPage != 1 || got.Page.Total != 1 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
	if len(got.List) != 1 {
		t.Fatalf("unexpected list len: %#v", got.List)
	}
	item := got.List[0]
	if item.ChannelText != "支付宝" || item.PayMethodText != "PC网页支付" || item.StatusText != "支付成功" {
		t.Fatalf("unexpected labels: %#v", item)
	}
	if item.PaidAt == nil || *item.PaidAt != "2026-04-13 18:45:01" || item.CreatedAt != "2026-04-13 18:38:26" {
		t.Fatalf("unexpected times: %#v", item)
	}
}

func TestDetailNormalizesJSONAndSummaries(t *testing.T) {
	createdAt := time.Date(2026, 4, 13, 18, 38, 26, 0, time.Local)
	closedAt := time.Date(2026, 4, 13, 19, 0, 0, 0, time.Local)
	repo := &fakeRepository{detail: &DetailRow{
		ID: 3, TransactionNo: "T260413183826000006", OrderID: 9, OrderNo: "R260413183826000005", AttemptNo: 1,
		ChannelID: 1, Channel: enum.PayChannelAlipay, PayMethod: enum.PayMethodWeb, Amount: 1000, TradeNo: "trade-1",
		TradeStatus: "TRADE_SUCCESS", Status: enum.PayTxnSuccess, ClosedAt: &closedAt, ChannelResp: `{"qr_code":"https://pay.example.test"}`,
		RawNotify: ``, CreatedAt: createdAt, PayChannelName: "支付宝默认", OrderUserID: 1, OrderUserName: "admin",
		OrderUserEmail: "demo@example.test", OrderTitle: "充值10元", OrderPayAmount: 1000, OrderPayStatus: enum.PayTxnSuccess,
	}}
	service := NewService(repo)

	got, appErr := service.Detail(context.Background(), 3)
	if appErr != nil {
		t.Fatalf("expected detail to succeed, got %v", appErr)
	}
	if got.Transaction.ChannelResp["qr_code"] != "https://pay.example.test" {
		t.Fatalf("unexpected channel_resp: %#v", got.Transaction.ChannelResp)
	}
	if len(got.Transaction.RawNotify) != 0 {
		t.Fatalf("blank raw_notify should become empty object: %#v", got.Transaction.RawNotify)
	}
	if got.Channel == nil || got.Channel.Name != "支付宝默认" || got.Order == nil || got.Order.UserName != "admin" {
		t.Fatalf("unexpected summaries: %#v", got)
	}
	if got.Transaction.ClosedAt == nil || *got.Transaction.ClosedAt != "2026-04-13 19:00:00" {
		t.Fatalf("unexpected closed_at: %#v", got.Transaction.ClosedAt)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	if strings.Contains(string(encoded), "app_private_key") {
		t.Fatalf("detail leaked private key fields: %s", encoded)
	}
}

func TestDetailMissingRowReturnsNotFound(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.Detail(context.Background(), 404)
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}
