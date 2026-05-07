package alipay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFormatCents(t *testing.T) {
	tests := []struct {
		name  string
		cents int
		want  string
	}{
		{name: "yuan", cents: 100, want: "1.00"},
		{name: "cents", cents: 123, want: "1.23"},
		{name: "zero", cents: 0, want: "0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCents(tt.cents); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestParseYuanToCents(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "yuan", in: "1", want: 100},
		{name: "one decimal", in: "1.2", want: 120},
		{name: "two decimals", in: "1.23", want: 123},
		{name: "trim zeros", in: "001.20", want: 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseYuanToCents(tt.in)
			if err != nil {
				t.Fatalf("parseYuanToCents returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestParseYuanToCentsRejectsInvalidAmounts(t *testing.T) {
	for _, in := range []string{"", "abc", "1.234", "-1.00"} {
		t.Run(in, func(t *testing.T) {
			if _, err := parseYuanToCents(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		})
	}
}

func TestValidateChannelConfigRejectsMissingRequiredFields(t *testing.T) {
	valid := ChannelConfig{
		ChannelID:      1,
		AppID:          "app-id",
		PrivateKey:     "private-key",
		AppCertPath:    "app.crt",
		AlipayCertPath: "alipay.crt",
		RootCertPath:   "root.crt",
		NotifyURL:      "https://example.test/api/pay/notify/alipay",
	}

	tests := []struct {
		name string
		cfg  ChannelConfig
	}{
		{name: "app id", cfg: func() ChannelConfig { cfg := valid; cfg.AppID = ""; return cfg }()},
		{name: "private key", cfg: func() ChannelConfig { cfg := valid; cfg.PrivateKey = ""; return cfg }()},
		{name: "app cert", cfg: func() ChannelConfig { cfg := valid; cfg.AppCertPath = ""; return cfg }()},
		{name: "alipay cert", cfg: func() ChannelConfig { cfg := valid; cfg.AlipayCertPath = ""; return cfg }()},
		{name: "root cert", cfg: func() ChannelConfig { cfg := valid; cfg.RootCertPath = ""; return cfg }()},
		{name: "notify url", cfg: func() ChannelConfig { cfg := valid; cfg.NotifyURL = ""; return cfg }()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateChannelConfig(tt.cfg); err == nil {
				t.Fatalf("expected missing config error")
			}
		})
	}

	if err := validateChannelConfig(valid); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestSuccessAndFailureBodies(t *testing.T) {
	gateway := NewGopayGateway(0)
	if gateway.SuccessBody() != "success" || gateway.FailureBody() != "fail" {
		t.Fatalf("unexpected bodies: %q %q", gateway.SuccessBody(), gateway.FailureBody())
	}
}

func TestWithGatewayTimeoutAddsDeadlineWhenParentHasNone(t *testing.T) {
	ctx, cancel := withGatewayTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected timeout deadline")
	}
	if time.Until(deadline) <= 0 || time.Until(deadline) > 100*time.Millisecond {
		t.Fatalf("unexpected deadline: %s", deadline)
	}
}

func TestWithGatewayTimeoutDoesNotExtendShorterParentDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer parentCancel()
	parentDeadline, _ := parent.Deadline()

	ctx, cancel := withGatewayTimeout(parent, time.Second)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected parent deadline")
	}
	if !deadline.Equal(parentDeadline) {
		t.Fatalf("expected parent deadline %s, got %s", parentDeadline, deadline)
	}
}

func TestWithGatewayTimeoutShortensLongerParentDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), time.Second)
	defer parentCancel()
	parentDeadline, _ := parent.Deadline()

	ctx, cancel := withGatewayTimeout(parent, 100*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected timeout deadline")
	}
	if !deadline.Before(parentDeadline) {
		t.Fatalf("expected gateway timeout to be before parent deadline %s, got %s", parentDeadline, deadline)
	}
	if time.Until(deadline) <= 0 || time.Until(deadline) > 100*time.Millisecond {
		t.Fatalf("unexpected deadline: %s", deadline)
	}
}

func TestWithGatewayTimeoutDisabledWhenNonPositive(t *testing.T) {
	ctx, cancel := withGatewayTimeout(context.Background(), 0)
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatalf("did not expect deadline when timeout is disabled")
	}
}

func TestStructToMapPreservesAlipayJSONFieldNames(t *testing.T) {
	raw := structToMap(struct {
		OutTradeNo  string `json:"out_trade_no,omitempty"`
		TradeStatus string `json:"trade_status,omitempty"`
	}{
		OutTradeNo:  "T1",
		TradeStatus: "TRADE_SUCCESS",
	})

	if raw["out_trade_no"] != "T1" || raw["trade_status"] != "TRADE_SUCCESS" {
		t.Fatalf("unexpected raw map: %#v", raw)
	}
}

func TestDownloadBillContentRejectsNon2xxAndOversizedBody(t *testing.T) {
	non2xx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer non2xx.Close()
	if _, err := downloadBillContentWithClient(context.Background(), non2xx.Client(), non2xx.URL); err == nil {
		t.Fatalf("expected non-2xx error")
	}

	oversized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, maxAlipayBillBytes+1))
	}))
	defer oversized.Close()
	if _, err := downloadBillContentWithClient(context.Background(), oversized.Client(), oversized.URL); err == nil {
		t.Fatalf("expected oversized body error")
	}
}

func TestDownloadBillContentReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bill-content"))
	}))
	defer server.Close()

	got, err := downloadBillContentWithClient(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("downloadBillContentWithClient returned error: %v", err)
	}
	if string(got) != "bill-content" {
		t.Fatalf("unexpected body: %q", string(got))
	}
}
