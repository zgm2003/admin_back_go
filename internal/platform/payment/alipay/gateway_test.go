package alipay

import (
	"testing"
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
	gateway := NewGopayGateway()
	if gateway.SuccessBody() != "success" || gateway.FailureBody() != "fail" {
		t.Fatalf("unexpected bodies: %q %q", gateway.SuccessBody(), gateway.FailureBody())
	}
}
