package payment

import (
	"bytes"
	"context"
	"net/url"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"
)

func TestCallbackDedupeKeyUsesStableFactsWhenNotifyIDIsEmpty(t *testing.T) {
	first := url.Values{
		"out_trade_no": {"PAY20260521100000000000"},
		"trade_no":     {"202605212200"},
		"trade_status": {"TRADE_SUCCESS"},
		"app_id":       {"2026000000000000"},
		"total_amount": {"10.00"},
	}
	replay := url.Values{
		"total_amount": {"10.00"},
		"app_id":       {"2026000000000000"},
		"trade_status": {"TRADE_SUCCESS"},
		"trade_no":     {"202605212200"},
		"out_trade_no": {"PAY20260521100000000000"},
	}
	changed := url.Values{}
	for key, values := range replay {
		changed[key] = append([]string(nil), values...)
	}
	changed.Set("trade_status", "WAIT_BUYER_PAY")

	firstKey := callbackDedupeKey(providerAlipay, first)
	if len(firstKey) != 32 || !bytes.Equal(firstKey, callbackDedupeKey(providerAlipay, replay)) {
		t.Fatalf("equivalent callback facts must have one 32-byte key")
	}
	if bytes.Equal(firstKey, callbackDedupeKey(providerAlipay, changed)) {
		t.Fatalf("different callback states must not share a fallback key")
	}
}

func TestCallbackAuditEventRecordsPendingThenProcessed(t *testing.T) {
	repo := newFakeCallbackRepo()
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	eventID, err := repo.CreateCallbackEvent(context.Background(), CallbackEvent{
		Provider:         providerAlipay,
		NotifyID:         "notify-1",
		OutTradeNo:       "PAY20260521100000000000",
		TradeNo:          "202605212200",
		TradeStatus:      "TRADE_SUCCESS",
		AppID:            "2026000000000000",
		TotalAmountCents: 1000,
		SignatureValid:   enum.CommonNo,
		ProcessStatus:    callbackProcessPending,
		RawPayloadJSON:   `{"out_trade_no":"PAY20260521100000000000"}`,
		ReceivedAt:       now,
		IsDel:            enum.CommonNo,
	})
	if err != nil {
		t.Fatalf("CreateCallbackEvent error=%v", err)
	}
	if eventID != 1 {
		t.Fatalf("expected event id 1, got %d", eventID)
	}

	processedAt := now.Add(time.Second)
	if err := repo.UpdateCallbackEventProcessed(context.Background(), eventID, enum.CommonYes, callbackProcessSuccess, "credited", processedAt); err != nil {
		t.Fatalf("UpdateCallbackEventProcessed error=%v", err)
	}
	if repo.callbackEvent.ProcessStatus != callbackProcessSuccess || repo.callbackEvent.SignatureValid != enum.CommonYes {
		t.Fatalf("unexpected event=%#v", repo.callbackEvent)
	}
}

type fakeCallbackRepo struct {
	callbackEvent CallbackEvent
}

func newFakeCallbackRepo() *fakeCallbackRepo { return &fakeCallbackRepo{} }

func (r *fakeCallbackRepo) CreateCallbackEvent(ctx context.Context, event CallbackEvent) (int64, error) {
	event.ID = 1
	r.callbackEvent = event
	return event.ID, nil
}

func (r *fakeCallbackRepo) UpdateCallbackEventProcessed(ctx context.Context, id int64, signatureValid int, status string, message string, processedAt time.Time) error {
	r.callbackEvent.SignatureValid = signatureValid
	r.callbackEvent.ProcessStatus = status
	r.callbackEvent.ProcessMessage = message
	r.callbackEvent.ProcessedAt = &processedAt
	return nil
}
