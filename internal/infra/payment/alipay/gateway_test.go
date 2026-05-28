package alipay

import "testing"

func TestValidateChannelConfigRejectsMissingRequiredFields(t *testing.T) {
	valid := ChannelConfig{
		AppID:          "app-id",
		PrivateKey:     "private-key",
		AppCertPath:    "app.crt",
		AlipayCertPath: "alipay.crt",
		RootCertPath:   "root.crt",
		NotifyURL:      "https://example.test/api/admin/v1/payment/configs/1/test",
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
