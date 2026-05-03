package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadUsesSafeDefaults(t *testing.T) {
	cfg := Load()

	if cfg.App.Name != "admin-api" {
		t.Fatalf("expected app name admin-api, got %q", cfg.App.Name)
	}
	if cfg.App.Env != "local" {
		t.Fatalf("expected app env local, got %q", cfg.App.Env)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("expected http addr :8080, got %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("expected read header timeout 5s, got %s", cfg.HTTP.ReadHeaderTimeout)
	}
	if cfg.MySQL.DSN != "" {
		t.Fatalf("expected empty mysql dsn, got %q", cfg.MySQL.DSN)
	}
	if cfg.MySQL.MaxOpenConns != 20 {
		t.Fatalf("expected mysql max open conns 20, got %d", cfg.MySQL.MaxOpenConns)
	}
	if cfg.Redis.Addr != "" {
		t.Fatalf("expected empty redis addr by default, got %q", cfg.Redis.Addr)
	}
	if cfg.Redis.DB != 0 {
		t.Fatalf("expected redis db 0, got %d", cfg.Redis.DB)
	}
	if cfg.Token.AccessTTL != 2*time.Hour {
		t.Fatalf("expected token access ttl 2h, got %s", cfg.Token.AccessTTL)
	}
	if cfg.Token.Pepper != "" {
		t.Fatalf("expected empty token pepper by default, got %q", cfg.Token.Pepper)
	}
	if cfg.Token.RedisPrefix != "token:" {
		t.Fatalf("expected token redis prefix token:, got %q", cfg.Token.RedisPrefix)
	}
	if cfg.Token.SessionCacheTTL != 30*time.Minute {
		t.Fatalf("expected token session cache ttl 30m, got %s", cfg.Token.SessionCacheTTL)
	}
	if cfg.Token.SingleSessionPointerTTL != 30*24*time.Hour {
		t.Fatalf("expected single session pointer ttl 30d, got %s", cfg.Token.SingleSessionPointerTTL)
	}
	if cfg.Token.RedisDB != 2 {
		t.Fatalf("expected token redis db 2, got %d", cfg.Token.RedisDB)
	}
	if cfg.Captcha.TTL != 2*time.Minute {
		t.Fatalf("expected captcha ttl 2m, got %s", cfg.Captcha.TTL)
	}
	if cfg.Captcha.RedisPrefix != "captcha:slide:" {
		t.Fatalf("expected captcha redis prefix captcha:slide:, got %q", cfg.Captcha.RedisPrefix)
	}
	if cfg.Captcha.SlidePadding != 10 {
		t.Fatalf("expected captcha slide padding 10, got %d", cfg.Captcha.SlidePadding)
	}
	wantOrigins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:5174",
		"http://127.0.0.1:5174",
	}
	if !reflect.DeepEqual(cfg.CORS.AllowOrigins, wantOrigins) {
		t.Fatalf("unexpected default cors origins: %#v", cfg.CORS.AllowOrigins)
	}
	if !containsString(cfg.CORS.AllowHeaders, "Authorization") ||
		!containsString(cfg.CORS.AllowHeaders, "platform") ||
		!containsString(cfg.CORS.AllowHeaders, "device-id") ||
		!containsString(cfg.CORS.AllowHeaders, "X-Trace-Id") {
		t.Fatalf("default cors headers do not cover frontend common headers: %#v", cfg.CORS.AllowHeaders)
	}
	if !cfg.CORS.AllowCredentials {
		t.Fatalf("expected cors credentials to be allowed by default")
	}
	if cfg.CORS.MaxAge != 12*time.Hour {
		t.Fatalf("expected cors max age 12h, got %s", cfg.CORS.MaxAge)
	}
}

func TestLoadReadsEnvironmentOverrides(t *testing.T) {
	t.Setenv("APP_NAME", "admin-api-test")
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDR", ":18080")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "7s")
	t.Setenv("MYSQL_DSN", "user:pass@tcp(127.0.0.1:3306)/admin")
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "50")
	t.Setenv("MYSQL_MAX_IDLE_CONNS", "12")
	t.Setenv("MYSQL_CONN_MAX_LIFETIME", "30m")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6380")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("TOKEN_PEPPER", "pepper")
	t.Setenv("TOKEN_ACCESS_TTL", "15m")
	t.Setenv("TOKEN_REFRESH_TTL", "24h")
	t.Setenv("TOKEN_REDIS_PREFIX", "token-test:")
	t.Setenv("TOKEN_SESSION_CACHE_TTL", "45m")
	t.Setenv("TOKEN_SINGLE_SESSION_POINTER_TTL", "720h")
	t.Setenv("TOKEN_REDIS_DB", "5")
	t.Setenv("CAPTCHA_TTL", "3m")
	t.Setenv("CAPTCHA_REDIS_PREFIX", "captcha-test:")
	t.Setenv("CAPTCHA_SLIDE_PADDING", "8")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://admin.example.com, http://localhost:5173")
	t.Setenv("CORS_ALLOW_HEADERS", "Content-Type,Authorization,X-Custom")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "false")
	t.Setenv("CORS_MAX_AGE", "30m")

	cfg := Load()

	if cfg.App.Name != "admin-api-test" || cfg.App.Env != "test" {
		t.Fatalf("unexpected app config: %#v", cfg.App)
	}
	if cfg.HTTP.Addr != ":18080" || cfg.HTTP.ReadHeaderTimeout != 7*time.Second {
		t.Fatalf("unexpected http config: %#v", cfg.HTTP)
	}
	if cfg.MySQL.DSN != "user:pass@tcp(127.0.0.1:3306)/admin" {
		t.Fatalf("unexpected mysql dsn: %q", cfg.MySQL.DSN)
	}
	if cfg.MySQL.MaxOpenConns != 50 || cfg.MySQL.MaxIdleConns != 12 || cfg.MySQL.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("unexpected mysql pool config: %#v", cfg.MySQL)
	}
	if cfg.Redis.Addr != "127.0.0.1:6380" || cfg.Redis.Password != "secret" || cfg.Redis.DB != 2 {
		t.Fatalf("unexpected redis config: %#v", cfg.Redis)
	}
	if cfg.Token.Pepper != "pepper" || cfg.Token.AccessTTL != 15*time.Minute || cfg.Token.RefreshTTL != 24*time.Hour {
		t.Fatalf("unexpected token config: %#v", cfg.Token)
	}
	if cfg.Token.RedisPrefix != "token-test:" || cfg.Token.SessionCacheTTL != 45*time.Minute {
		t.Fatalf("unexpected token cache config: %#v", cfg.Token)
	}
	if cfg.Token.SingleSessionPointerTTL != 30*24*time.Hour {
		t.Fatalf("expected single session pointer ttl 720h, got %s", cfg.Token.SingleSessionPointerTTL)
	}
	if cfg.Token.RedisDB != 5 {
		t.Fatalf("expected token redis db 5, got %d", cfg.Token.RedisDB)
	}
	if cfg.Captcha.TTL != 3*time.Minute || cfg.Captcha.RedisPrefix != "captcha-test:" || cfg.Captcha.SlidePadding != 8 {
		t.Fatalf("unexpected captcha config: %#v", cfg.Captcha)
	}
	if !reflect.DeepEqual(cfg.CORS.AllowOrigins, []string{"https://admin.example.com", "http://localhost:5173"}) {
		t.Fatalf("unexpected cors origins: %#v", cfg.CORS.AllowOrigins)
	}
	if !reflect.DeepEqual(cfg.CORS.AllowHeaders, []string{"Content-Type", "Authorization", "X-Custom"}) {
		t.Fatalf("unexpected cors allow headers: %#v", cfg.CORS.AllowHeaders)
	}
	if cfg.CORS.AllowCredentials {
		t.Fatalf("expected cors credentials override to false")
	}
	if cfg.CORS.MaxAge != 30*time.Minute {
		t.Fatalf("expected cors max age 30m, got %s", cfg.CORS.MaxAge)
	}
}

func TestLoadFallsBackOnInvalidNumericAndDurationValues(t *testing.T) {
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "garbage")
	t.Setenv("REDIS_DB", "garbage")
	t.Setenv("TOKEN_ACCESS_TTL", "garbage")

	cfg := Load()

	if cfg.MySQL.MaxOpenConns != 20 {
		t.Fatalf("expected fallback mysql max open conns 20, got %d", cfg.MySQL.MaxOpenConns)
	}
	if cfg.Redis.DB != 0 {
		t.Fatalf("expected fallback redis db 0, got %d", cfg.Redis.DB)
	}
	if cfg.Token.AccessTTL != 2*time.Hour {
		t.Fatalf("expected fallback token access ttl 2h, got %s", cfg.Token.AccessTTL)
	}
}

func TestLoadBuildsMySQLDSNFromLegacyDBEnvironment(t *testing.T) {
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_DATABASE", "admin")
	t.Setenv("DB_USERNAME", "admin_user")
	t.Setenv("DB_PASSWORD", "secret")

	cfg := Load()

	want := "admin_user:secret@tcp(127.0.0.1:3307)/admin?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.MySQL.DSN != want {
		t.Fatalf("expected legacy mysql dsn %q, got %q", want, cfg.MySQL.DSN)
	}
}

func TestLoadBuildsRedisAddrFromLegacyRedisEnvironment(t *testing.T) {
	t.Setenv("REDIS_HOST", "127.0.0.1")
	t.Setenv("REDIS_PORT", "6380")

	cfg := Load()

	if cfg.Redis.Addr != "127.0.0.1:6380" {
		t.Fatalf("expected redis addr 127.0.0.1:6380, got %q", cfg.Redis.Addr)
	}
}

func TestLoadDotEnvReadsLocalEnvFile(t *testing.T) {
	unsetEnvForTest(t, "APP_NAME")
	unsetEnvForTest(t, "HTTP_ADDR")
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("APP_NAME=admin-api-dotenv\nHTTP_ADDR=:19090\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := LoadDotEnv(envPath); err != nil {
		t.Fatalf("LoadDotEnv returned error: %v", err)
	}

	cfg := Load()
	if cfg.App.Name != "admin-api-dotenv" {
		t.Fatalf("expected app name from .env, got %q", cfg.App.Name)
	}
	if cfg.HTTP.Addr != ":19090" {
		t.Fatalf("expected http addr from .env, got %q", cfg.HTTP.Addr)
	}
}

func TestLoadDotEnvAllowsMissingLocalEnvFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), ".env")

	if err := LoadDotEnv(missingPath); err != nil {
		t.Fatalf("expected missing .env to be ignored, got %v", err)
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	oldValue, hadValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
