package bootstrap

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
)

func TestNewTokenAuthenticatorFailsClosedWithoutResources(t *testing.T) {
	authenticator := NewTokenAuthenticator(nil, config.Config{
		Token: config.TokenConfig{
			Pepper:          "pepper-value",
			RedisPrefix:     "token:",
			SessionCacheTTL: 30 * time.Minute,
		},
	})

	identity, appErr := authenticator(context.Background(), middleware.TokenInput{AccessToken: "valid-token"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "Token认证未配置" {
		t.Fatalf("expected token auth not configured, got %#v", appErr)
	}
}
