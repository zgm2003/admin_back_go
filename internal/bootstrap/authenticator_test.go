package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/platform/accesstoken"
	"admin_back_go/internal/platform/secretkey"
)

func TestNewTokenAuthenticatorFailsClosedWithoutResources(t *testing.T) {
	rootSecret := strings.Repeat("a", 64)
	keys, err := secretkey.NewKeyRing(rootSecret)
	if err != nil {
		t.Fatalf("NewKeyRing returned error: %v", err)
	}
	accessToken, err := accesstoken.NewJWTCodec(keys.JWTSigningKey(), accesstoken.Options{Issuer: "admin_go"}).Issue(accesstoken.Claims{
		SessionID: 1,
		UserID:    2,
		Platform:  "admin",
		IssuedAt:  time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}
	authenticator := NewTokenAuthenticator(nil, config.Config{
		App: config.AppConfig{Secret: rootSecret},
		Token: config.TokenConfig{
			RedisPrefix:     "token:",
			SessionCacheTTL: 30 * time.Minute,
		},
	})

	identity, appErr := authenticator(context.Background(), middleware.TokenInput{AccessToken: accessToken, Platform: "admin"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "Token认证未配置" {
		t.Fatalf("expected token auth not configured, got %#v", appErr)
	}
}
