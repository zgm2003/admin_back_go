package alipay

import (
	"testing"
	"time"
)

func TestFormatAmountCents(t *testing.T) {
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{name: "one cent", cents: 1, want: "0.01"},
		{name: "one yuan", cents: 100, want: "1.00"},
		{name: "yuan and cents", cents: 1234, want: "12.34"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatAmountCents(tt.cents)
			if err != nil {
				t.Fatalf("formatAmountCents error=%v", err)
			}
			if got != tt.want {
				t.Fatalf("amount=%s want=%s", got, tt.want)
			}
		})
	}
}

func TestFormatAmountCentsRejectsInvalidAmount(t *testing.T) {
	for _, cents := range []int64{0, -1} {
		if _, err := formatAmountCents(cents); err == nil {
			t.Fatalf("expected error for cents=%d", cents)
		}
	}
}

func TestBuildPayBodyUsesReturnURLAndExpireTime(t *testing.T) {
	expiredAt := time.Date(2026, 5, 15, 13, 30, 0, 0, time.UTC)
	body, err := buildPayBody(PayInput{
		OutTradeNo:  "PAY202605150001",
		Method:      "web",
		Subject:     "测试订单",
		AmountCents: 1234,
		ReturnURL:   "https://example.test/pay/result",
		ExpiredAt:   expiredAt,
	})
	if err != nil {
		t.Fatalf("buildPayBody error=%v", err)
	}
	if body.GetString("out_trade_no") != "PAY202605150001" {
		t.Fatalf("unexpected out_trade_no: %s", body.GetString("out_trade_no"))
	}
	if body.GetString("total_amount") != "12.34" {
		t.Fatalf("unexpected total_amount: %s", body.GetString("total_amount"))
	}
	if body.GetString("return_url") != "https://example.test/pay/result" {
		t.Fatalf("unexpected return_url: %s", body.GetString("return_url"))
	}
	if body.GetString("time_expire") != "2026-05-15 13:30:00" {
		t.Fatalf("unexpected time_expire: %s", body.GetString("time_expire"))
	}
}
