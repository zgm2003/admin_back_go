package paynotifylog

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
	listRows  []ListRow
	total     int64
	detail    *DetailRow
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

func TestInitReturnsPayNotifyLogDicts(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.ChannelArr) != len(enum.PayChannels) || got.Dict.ChannelArr[1].Value != enum.PayChannelAlipay {
		t.Fatalf("unexpected channel dict: %#v", got.Dict.ChannelArr)
	}
	if len(got.Dict.NotifyTypeArr) != len(enum.NotifyTypes) || got.Dict.NotifyTypeArr[0].Label != "支付回调" {
		t.Fatalf("unexpected notify type dict: %#v", got.Dict.NotifyTypeArr)
	}
	if len(got.Dict.NotifyProcessStatusArr) != len(enum.NotifyProcessStatuses) || got.Dict.NotifyProcessStatusArr[1].Value != enum.NotifyProcessSuccess {
		t.Fatalf("unexpected process status dict: %#v", got.Dict.NotifyProcessStatusArr)
	}
}

func TestListDefaultsPageAndMapsLabels(t *testing.T) {
	createdAt := time.Date(2026, 5, 7, 10, 11, 12, 0, time.Local)
	repo := &fakeRepository{listRows: []ListRow{{
		ID: 1, Channel: enum.PayChannelAlipay, NotifyType: enum.NotifyPay, TransactionNo: "T1", TradeNo: "TRADE1",
		ProcessStatus: enum.NotifyProcessSuccess, ProcessMsg: "ok", IP: "127.0.0.1", CreatedAt: createdAt,
	}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if got.Page.CurrentPage != 1 || got.Page.PageSize != 20 || got.Page.TotalPage != 1 || got.Page.Total != 1 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
	item := got.List[0]
	if item.ChannelText != "支付宝" || item.NotifyTypeText != "支付回调" || item.ProcessStatusText != "处理成功" {
		t.Fatalf("unexpected labels: %#v", item)
	}
	if item.CreatedAt != "2026-05-07 10:11:12" {
		t.Fatalf("unexpected created_at: %#v", item)
	}
}

func TestDetailNormalizesJSONPayloads(t *testing.T) {
	createdAt := time.Date(2026, 5, 7, 10, 11, 12, 0, time.Local)
	updatedAt := time.Date(2026, 5, 7, 10, 11, 13, 0, time.Local)
	repo := &fakeRepository{detail: &DetailRow{
		ID: 1, Channel: enum.PayChannelAlipay, NotifyType: enum.NotifyPay, TransactionNo: "T1", TradeNo: "TRADE1",
		ProcessStatus: enum.NotifyProcessFailed, ProcessMsg: "bad sign", Headers: `{"x-request-id":"rid-1"}`,
		RawData: ``, IP: "127.0.0.1", CreatedAt: createdAt, UpdatedAt: updatedAt,
	}}
	service := NewService(repo)

	got, appErr := service.Detail(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("expected detail to succeed, got %v", appErr)
	}
	if got.Log.Headers["x-request-id"] != "rid-1" {
		t.Fatalf("unexpected headers: %#v", got.Log.Headers)
	}
	if len(got.Log.RawData) != 0 {
		t.Fatalf("blank raw_data should become empty object: %#v", got.Log.RawData)
	}
	if got.Log.ProcessStatusText != "处理失败" || got.Log.UpdatedAt != "2026-05-07 10:11:13" {
		t.Fatalf("unexpected detail labels/times: %#v", got.Log)
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
