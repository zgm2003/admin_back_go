package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	if cfg.App.Secret != "" {
		t.Fatalf("expected empty app secret by default, got %q", cfg.App.Secret)
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
	if !cfg.Queue.Enabled {
		t.Fatalf("expected queue to be enabled by default")
	}
	if cfg.Queue.RedisDB != 3 {
		t.Fatalf("expected queue redis db 3, got %d", cfg.Queue.RedisDB)
	}
	if cfg.Queue.Concurrency != 10 {
		t.Fatalf("expected queue concurrency 10, got %d", cfg.Queue.Concurrency)
	}
	if cfg.Queue.DefaultQueue != "default" {
		t.Fatalf("expected default queue name default, got %q", cfg.Queue.DefaultQueue)
	}
	if cfg.Queue.CriticalWeight != 6 || cfg.Queue.DefaultWeight != 3 || cfg.Queue.LowWeight != 1 {
		t.Fatalf("unexpected queue weights: %#v", cfg.Queue)
	}
	if cfg.Queue.ShutdownTimeout != 10*time.Second {
		t.Fatalf("expected queue shutdown timeout 10s, got %s", cfg.Queue.ShutdownTimeout)
	}
	if cfg.Queue.DefaultMaxRetry != 3 {
		t.Fatalf("expected queue default max retry 3, got %d", cfg.Queue.DefaultMaxRetry)
	}
	if cfg.Queue.DefaultTimeout != 30*time.Second {
		t.Fatalf("expected queue default timeout 30s, got %s", cfg.Queue.DefaultTimeout)
	}
	if !cfg.Realtime.Enabled {
		t.Fatalf("expected realtime to be enabled by default")
	}
	if cfg.Realtime.Publisher != RealtimePublisherLocal {
		t.Fatalf("expected realtime publisher local, got %q", cfg.Realtime.Publisher)
	}
	if cfg.Realtime.HeartbeatInterval != 25*time.Second {
		t.Fatalf("expected realtime heartbeat interval 25s, got %s", cfg.Realtime.HeartbeatInterval)
	}
	if cfg.Realtime.SendBuffer != 16 {
		t.Fatalf("expected realtime send buffer 16, got %d", cfg.Realtime.SendBuffer)
	}
	if cfg.Realtime.RedisChannel != "admin_go:realtime:publish" {
		t.Fatalf("expected realtime redis channel default, got %q", cfg.Realtime.RedisChannel)
	}
	if !cfg.Scheduler.Enabled {
		t.Fatalf("expected scheduler to be enabled by default")
	}
	if cfg.Scheduler.Timezone != "Asia/Shanghai" {
		t.Fatalf("expected scheduler timezone Asia/Shanghai, got %q", cfg.Scheduler.Timezone)
	}
	if cfg.Scheduler.LockPrefix != "admin_go:scheduler:" {
		t.Fatalf("expected scheduler lock prefix admin_go:scheduler:, got %q", cfg.Scheduler.LockPrefix)
	}
	if cfg.Scheduler.LockTTL != 30*time.Second {
		t.Fatalf("expected scheduler lock ttl 30s, got %s", cfg.Scheduler.LockTTL)
	}
	if cfg.AI.ChatStreamMaxDuration != 5*time.Minute {
		t.Fatalf("expected AI chat stream max duration 5m, got %s", cfg.AI.ChatStreamMaxDuration)
	}
	if cfg.AI.ChatStreamIdleTimeout != 60*time.Second {
		t.Fatalf("expected AI chat stream idle timeout 60s, got %s", cfg.AI.ChatStreamIdleTimeout)
	}
	if cfg.AI.RunStaleTimeout != 15*time.Minute {
		t.Fatalf("expected AI run stale timeout 15m, got %s", cfg.AI.RunStaleTimeout)
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
	t.Setenv("APP_SECRET", strings.Repeat("s", 64))
	t.Setenv("HTTP_ADDR", ":18080")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "7s")
	t.Setenv("MYSQL_DSN", "user:pass@tcp(127.0.0.1:3306)/admin")
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "50")
	t.Setenv("MYSQL_MAX_IDLE_CONNS", "12")
	t.Setenv("MYSQL_CONN_MAX_LIFETIME", "30m")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6380")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("TOKEN_REDIS_PREFIX", "token-test:")
	t.Setenv("TOKEN_SESSION_CACHE_TTL", "45m")
	t.Setenv("TOKEN_SINGLE_SESSION_POINTER_TTL", "720h")
	t.Setenv("TOKEN_REDIS_DB", "5")
	t.Setenv("CAPTCHA_TTL", "3m")
	t.Setenv("CAPTCHA_REDIS_PREFIX", "captcha-test:")
	t.Setenv("CAPTCHA_SLIDE_PADDING", "8")
	t.Setenv("QUEUE_ENABLED", "false")
	t.Setenv("QUEUE_REDIS_DB", "4")
	t.Setenv("QUEUE_CONCURRENCY", "22")
	t.Setenv("QUEUE_DEFAULT_QUEUE", "admin")
	t.Setenv("QUEUE_CRITICAL_WEIGHT", "8")
	t.Setenv("QUEUE_DEFAULT_WEIGHT", "4")
	t.Setenv("QUEUE_LOW_WEIGHT", "2")
	t.Setenv("QUEUE_SHUTDOWN_TIMEOUT", "12s")
	t.Setenv("QUEUE_DEFAULT_MAX_RETRY", "5")
	t.Setenv("QUEUE_DEFAULT_TIMEOUT", "45s")
	t.Setenv("REALTIME_ENABLED", "false")
	t.Setenv("REALTIME_PUBLISHER", "noop")
	t.Setenv("REALTIME_HEARTBEAT_INTERVAL", "10s")
	t.Setenv("REALTIME_SEND_BUFFER", "32")
	t.Setenv("REALTIME_REDIS_CHANNEL", "test:realtime")
	t.Setenv("SCHEDULER_ENABLED", "false")
	t.Setenv("SCHEDULER_TIMEZONE", "UTC")
	t.Setenv("SCHEDULER_LOCK_PREFIX", "test:scheduler:")
	t.Setenv("SCHEDULER_LOCK_TTL", "45s")
	t.Setenv("AI_CHAT_STREAM_MAX_DURATION", "3m")
	t.Setenv("AI_CHAT_STREAM_IDLE_TIMEOUT", "45s")
	t.Setenv("AI_RUN_STALE_TIMEOUT", "20m")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://admin.example.com, http://localhost:5173")
	t.Setenv("CORS_ALLOW_HEADERS", "Content-Type,Authorization,X-Custom")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "false")
	t.Setenv("CORS_MAX_AGE", "30m")

	cfg := Load()

	if cfg.App.Name != "admin-api-test" || cfg.App.Env != "test" || cfg.App.Secret != strings.Repeat("s", 64) {
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
	if cfg.Queue.Enabled {
		t.Fatalf("expected queue enabled override to false")
	}
	if cfg.Queue.RedisDB != 4 || cfg.Queue.Concurrency != 22 || cfg.Queue.DefaultQueue != "admin" {
		t.Fatalf("unexpected queue config: %#v", cfg.Queue)
	}
	if cfg.Queue.CriticalWeight != 8 || cfg.Queue.DefaultWeight != 4 || cfg.Queue.LowWeight != 2 {
		t.Fatalf("unexpected queue weights: %#v", cfg.Queue)
	}
	if cfg.Queue.ShutdownTimeout != 12*time.Second || cfg.Queue.DefaultMaxRetry != 5 || cfg.Queue.DefaultTimeout != 45*time.Second {
		t.Fatalf("unexpected queue retry/timeout config: %#v", cfg.Queue)
	}
	if cfg.Realtime.Enabled {
		t.Fatalf("expected realtime enabled override to false")
	}
	if cfg.Realtime.Publisher != RealtimePublisherNoop {
		t.Fatalf("expected realtime publisher noop, got %q", cfg.Realtime.Publisher)
	}
	if cfg.Realtime.HeartbeatInterval != 10*time.Second || cfg.Realtime.SendBuffer != 32 || cfg.Realtime.RedisChannel != "test:realtime" {
		t.Fatalf("unexpected realtime config: %#v", cfg.Realtime)
	}
	if cfg.Scheduler.Enabled {
		t.Fatalf("expected scheduler enabled override to false")
	}
	if cfg.Scheduler.Timezone != "UTC" || cfg.Scheduler.LockPrefix != "test:scheduler:" || cfg.Scheduler.LockTTL != 45*time.Second {
		t.Fatalf("unexpected scheduler config: %#v", cfg.Scheduler)
	}
	if cfg.AI.ChatStreamMaxDuration != 3*time.Minute ||
		cfg.AI.ChatStreamIdleTimeout != 45*time.Second ||
		cfg.AI.RunStaleTimeout != 20*time.Minute {
		t.Fatalf("unexpected AI config: %#v", cfg.AI)
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

func TestLoadReadsAppSecret(t *testing.T) {
	t.Setenv("APP_SECRET", strings.Repeat("a", 64))

	cfg := Load()

	if cfg.App.Secret != strings.Repeat("a", 64) {
		t.Fatalf("expected APP_SECRET to be loaded")
	}
}

func TestValidateRuntimeSecretsRejectsMissingAppSecret(t *testing.T) {
	cfg := Config{App: AppConfig{Name: "admin-api", Env: "local"}}

	err := ValidateRuntimeSecrets(cfg)

	if err == nil || !strings.Contains(err.Error(), "APP_SECRET") {
		t.Fatalf("expected APP_SECRET validation error, got %v", err)
	}
}

func TestValidateRuntimeSecretsRejectsDefaultAppSecret(t *testing.T) {
	cfg := Config{App: AppConfig{Name: "admin-api", Env: "local", Secret: "change_me_to_at_least_64_random_chars"}}

	err := ValidateRuntimeSecrets(cfg)

	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe APP_SECRET validation error, got %v", err)
	}
}

func TestValidateRuntimeSecretsAcceptsLongAppSecret(t *testing.T) {
	cfg := Config{App: AppConfig{Name: "admin-api", Env: "local", Secret: strings.Repeat("k", 64)}}

	if err := ValidateRuntimeSecrets(cfg); err != nil {
		t.Fatalf("expected APP_SECRET to pass validation: %v", err)
	}
}

func TestLoadReadsPaymentConfig(t *testing.T) {
	t.Setenv("PAYMENT_CERT_BASE_DIR", "E:/admin_go/admin_back_go")

	cfg := Load()

	if cfg.Payment.CertBaseDir != "E:/admin_go/admin_back_go" {
		t.Fatalf("expected payment cert base dir to point at Go backend, got %q", cfg.Payment.CertBaseDir)
	}
}

func TestEnvExampleUsesGoOwnedPaymentCerts(t *testing.T) {
	values := readEnvExample(t)

	if values["PAYMENT_CERT_BASE_DIR"] != "E:/admin_go/admin_back_go" {
		t.Fatalf("expected PAYMENT_CERT_BASE_DIR to point at Go backend, got %q", values["PAYMENT_CERT_BASE_DIR"])
	}
	if _, ok := values["LEGACY_ADMIN_BACK_ROOT"]; ok {
		t.Fatalf("LEGACY_ADMIN_BACK_ROOT should not be documented in Go-owned env example")
	}
	if _, ok := values["PAYMENT_NOTIFY_LOCK_TTL"]; ok {
		t.Fatalf("PAYMENT_NOTIFY_LOCK_TTL should not be documented without runtime usage")
	}
	if _, ok := values["PAYMENT_ATTEMPT_LOCK_TTL"]; ok {
		t.Fatalf("PAYMENT_ATTEMPT_LOCK_TTL should not be documented without runtime usage")
	}
}

func TestEnvExampleDocumentsAITimeouts(t *testing.T) {
	values := readEnvExample(t)

	if values["AI_CHAT_STREAM_MAX_DURATION"] != "5m" {
		t.Fatalf("expected AI_CHAT_STREAM_MAX_DURATION=5m, got %q", values["AI_CHAT_STREAM_MAX_DURATION"])
	}
	if values["AI_CHAT_STREAM_IDLE_TIMEOUT"] != "60s" {
		t.Fatalf("expected AI_CHAT_STREAM_IDLE_TIMEOUT=60s, got %q", values["AI_CHAT_STREAM_IDLE_TIMEOUT"])
	}
	if values["AI_RUN_STALE_TIMEOUT"] != "15m" {
		t.Fatalf("expected AI_RUN_STALE_TIMEOUT=15m, got %q", values["AI_RUN_STALE_TIMEOUT"])
	}
}

func TestEnvExampleDocumentsSchedulerDistributedLock(t *testing.T) {
	values := readEnvExample(t)

	if values["SCHEDULER_LOCK_PREFIX"] != "admin_go:scheduler:" {
		t.Fatalf("expected SCHEDULER_LOCK_PREFIX default, got %q", values["SCHEDULER_LOCK_PREFIX"])
	}
	if values["SCHEDULER_LOCK_TTL"] != "30s" {
		t.Fatalf("expected SCHEDULER_LOCK_TTL=30s, got %q", values["SCHEDULER_LOCK_TTL"])
	}
}

func TestDefaultCORSAllowsAcceptLanguage(t *testing.T) {
	cfg := DefaultCORSConfig()
	if !containsString(cfg.AllowHeaders, "Accept-Language") {
		t.Fatalf("DefaultCORSConfig must allow Accept-Language, got %#v", cfg.AllowHeaders)
	}
}

func TestLoadFallsBackOnInvalidNumericAndDurationValues(t *testing.T) {
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "garbage")
	t.Setenv("REDIS_DB", "garbage")

	cfg := Load()

	if cfg.MySQL.MaxOpenConns != 20 {
		t.Fatalf("expected fallback mysql max open conns 20, got %d", cfg.MySQL.MaxOpenConns)
	}
	if cfg.Redis.DB != 0 {
		t.Fatalf("expected fallback redis db 0, got %d", cfg.Redis.DB)
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

func readEnvExample(t *testing.T) map[string]string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}

	values := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}
