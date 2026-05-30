package alipay

import (
	"net/url"
	"testing"
)

func TestParseNotifyPayloadNormalizesAmountToCents(t *testing.T) {
	form := url.Values{}
	form.Set("notify_id", "notify-1")
	form.Set("out_trade_no", "PAY20260521100000000000")
	form.Set("trade_no", "202605212200")
	form.Set("trade_status", "TRADE_SUCCESS")
	form.Set("app_id", "2026000000000000")
	form.Set("total_amount", "10.05")
	form.Set("sign", "signature")
	form.Set("sign_type", "RSA2")

	payload, err := ParseNotifyPayload(form)
	if err != nil {
		t.Fatalf("ParseNotifyPayload error=%v", err)
	}
	if payload.NotifyID != "notify-1" || payload.OutTradeNo != "PAY20260521100000000000" || payload.TotalAmountCents != 1005 {
		t.Fatalf("unexpected payload=%#v", payload)
	}
	if payload.Raw["sign"] != "signature" {
		t.Fatalf("expected raw payload to preserve first form value, got %#v", payload.Raw)
	}
}

func TestParseNotifyPayloadRejectsInvalidAmount(t *testing.T) {
	tests := []string{
		"10.005",
		"10.-1",
		"10.+1",
		"10.a",
		"10.0a",
		"10..1",
		"-1",
		"abc",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			form := url.Values{"total_amount": []string{value}}
			_, err := ParseNotifyPayload(form)
			if err == nil {
				t.Fatal("expected invalid amount error")
			}
		})
	}
}
