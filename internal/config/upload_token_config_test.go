package config

import (
	"testing"
	"time"
)

func TestLoadReadsUploadTokenConfig(t *testing.T) {
	t.Setenv("UPLOAD_TOKEN_TTL", "10m")
	t.Setenv("UPLOAD_KEY_RANDOM_BYTES", "6")
	t.Setenv("COS_STS_ENABLED", "true")
	t.Setenv("COS_STS_ENDPOINT", "sts.tencentcloudapi.com")
	t.Setenv("COS_STS_REGION", "ap-shanghai")

	cfg := Load()

	if cfg.UploadToken.TTL != 10*time.Minute {
		t.Fatalf("expected 10m, got %s", cfg.UploadToken.TTL)
	}
	if cfg.UploadToken.KeyRandomBytes != 6 {
		t.Fatalf("expected random bytes 6, got %d", cfg.UploadToken.KeyRandomBytes)
	}
	if !cfg.UploadToken.COS.Enabled {
		t.Fatalf("expected COS STS enabled")
	}
	if cfg.UploadToken.COS.Endpoint != "sts.tencentcloudapi.com" {
		t.Fatalf("unexpected endpoint %q", cfg.UploadToken.COS.Endpoint)
	}
	if cfg.UploadToken.COS.Region != "ap-shanghai" {
		t.Fatalf("unexpected region %q", cfg.UploadToken.COS.Region)
	}
}
