package payruntime

import (
	"testing"
	"time"
)

func TestNewRechargeOrderInitializesJSONExtra(t *testing.T) {
	order := newRechargeOrder(RechargeOrderMutation{
		OrderNo:    "R202605080001",
		UserID:     7,
		Title:      "钱包充值 30 元",
		Amount:     3000,
		ChannelID:  1,
		PayMethod:  "web",
		ExpireTime: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		IP:         "127.0.0.1",
	})

	if order.Extra != "{}" {
		t.Fatalf("expected orders.extra to be valid JSON object, got %q", order.Extra)
	}
}
