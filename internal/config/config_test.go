package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func loadForTest(t *testing.T, _ Process) Config {
	t.Helper()
	cfg, err := loadFrom(osLookup)
	if err != nil {
		t.Fatalf("loadFrom(): %v", err)
	}
	return cfg
}

func TestLoadUsesSafeDefaults(t *testing.T) {
	cfg := loadForTest(t, ProcessAPI)

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
	if cfg.Token.RedisPrefix != DefaultTokenRedisPrefix {
		t.Fatalf("expected token redis prefix %q, got %q", DefaultTokenRedisPrefix, cfg.Token.RedisPrefix)
	}
	if cfg.Token.SessionCacheTTL != DefaultTokenSessionCacheTTL {
		t.Fatalf("expected token session cache ttl %s, got %s", DefaultTokenSessionCacheTTL, cfg.Token.SessionCacheTTL)
	}
	if cfg.Token.SingleSessionPointerTTL != DefaultTokenSingleSessionPointerTTL {
		t.Fatalf("expected single session pointer ttl %s, got %s", DefaultTokenSingleSessionPointerTTL, cfg.Token.SingleSessionPointerTTL)
	}
	if cfg.Token.RedisDB != DefaultTokenRedisDB {
		t.Fatalf("expected token redis db %d, got %d", DefaultTokenRedisDB, cfg.Token.RedisDB)
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
	if !cfg.Realtime.Enabled {
		t.Fatalf("expected realtime to be enabled by default")
	}
	if cfg.Realtime.Publisher != RealtimePublisherLocal {
		t.Fatalf("expected realtime publisher local, got %q", cfg.Realtime.Publisher)
	}
	if cfg.Realtime.HeartbeatInterval != DefaultRealtimeHeartbeatInterval {
		t.Fatalf("expected realtime heartbeat interval %s, got %s", DefaultRealtimeHeartbeatInterval, cfg.Realtime.HeartbeatInterval)
	}
	if cfg.Realtime.SendBuffer != DefaultRealtimeSendBuffer {
		t.Fatalf("expected realtime send buffer %d, got %d", DefaultRealtimeSendBuffer, cfg.Realtime.SendBuffer)
	}
	if cfg.Realtime.RedisChannel != DefaultRealtimeRedisChannel {
		t.Fatalf("expected realtime redis channel default %q, got %q", DefaultRealtimeRedisChannel, cfg.Realtime.RedisChannel)
	}
	if !cfg.Scheduler.Enabled {
		t.Fatalf("expected scheduler to be enabled by default")
	}
	if cfg.Scheduler.Timezone != DefaultSchedulerTimezone {
		t.Fatalf("expected scheduler timezone %s, got %q", DefaultSchedulerTimezone, cfg.Scheduler.Timezone)
	}
	if cfg.Scheduler.LockPrefix != DefaultSchedulerLockPrefix {
		t.Fatalf("expected scheduler lock prefix %s, got %q", DefaultSchedulerLockPrefix, cfg.Scheduler.LockPrefix)
	}
	if cfg.Scheduler.LockTTL != DefaultSchedulerLockTTL {
		t.Fatalf("expected scheduler lock ttl %s, got %s", DefaultSchedulerLockTTL, cfg.Scheduler.LockTTL)
	}
	if cfg.AI.ChatStreamMaxDuration != DefaultAIChatStreamMaxDuration {
		t.Fatalf("expected AI chat stream max duration %s, got %s", DefaultAIChatStreamMaxDuration, cfg.AI.ChatStreamMaxDuration)
	}
	if cfg.AI.ChatStreamIdleTimeout != DefaultAIChatStreamIdleTimeout {
		t.Fatalf("expected AI chat stream idle timeout %s, got %s", DefaultAIChatStreamIdleTimeout, cfg.AI.ChatStreamIdleTimeout)
	}
	if cfg.AI.RunStaleTimeout != DefaultAIRunStaleTimeout {
		t.Fatalf("expected AI run stale timeout %s, got %s", DefaultAIRunStaleTimeout, cfg.AI.RunStaleTimeout)
	}
	wantOrigins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	}
	if !reflect.DeepEqual(cfg.CORS.AllowOrigins, wantOrigins) {
		t.Fatalf("unexpected default cors origins: %#v", cfg.CORS.AllowOrigins)
	}
	for _, origin := range []string{"http://localhost:5174", "http://127.0.0.1:5174"} {
		if containsString(cfg.CORS.AllowOrigins, origin) {
			t.Fatalf("default cors origins must not include %s: %#v", origin, cfg.CORS.AllowOrigins)
		}
	}
	for _, header := range []string{
		"Origin",
		"Content-Type",
		"Accept",
		"Accept-Language",
		"Authorization",
		"platform",
		"device-id",
		"X-Trace-Id",
		"X-Request-Id",
	} {
		if !containsString(cfg.CORS.AllowHeaders, header) {
			t.Fatalf("default cors headers must contain %s: %#v", header, cfg.CORS.AllowHeaders)
		}
	}
	if containsString(cfg.CORS.AllowHeaders, "X-Admin-Client-Variant") {
		t.Fatalf("default CORS must not allow retired client variant header: %#v", cfg.CORS.AllowHeaders)
	}
	if !containsString(cfg.CORS.ExposeHeaders, "X-Request-Id") {
		t.Fatalf("default cors expose headers must contain X-Request-Id: %#v", cfg.CORS.ExposeHeaders)
	}
	if !cfg.CORS.AllowCredentials {
		t.Fatalf("expected cors credentials to be allowed by default")
	}
	if cfg.CORS.MaxAge != 12*time.Hour {
		t.Fatalf("expected cors max age 12h, got %s", cfg.CORS.MaxAge)
	}
}

func TestLoadPreservesLanDevCORSOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173,http://192.168.5.20:5173")

	cfg := loadForTest(t, ProcessAPI)
	want := []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://192.168.5.20:5173"}
	if !reflect.DeepEqual(cfg.CORS.AllowOrigins, want) {
		t.Fatalf("unexpected LAN dev CORS origins: %#v", cfg.CORS.AllowOrigins)
	}
}

func TestLoadReadsEnvironmentOverrides(t *testing.T) {
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
	t.Setenv("TOKEN_SINGLE_SESSION_POINTER_TTL", "111h")
	t.Setenv("TOKEN_REDIS_DB", "5")
	t.Setenv("QUEUE_ENABLED", "false")
	t.Setenv("QUEUE_REDIS_DB", "4")
	t.Setenv("QUEUE_CONCURRENCY", "22")
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
	t.Setenv("CORS_ALLOW_HEADERS", "X-Legacy-CORS")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "false")
	t.Setenv("CORS_MAX_AGE", "30m")

	cfg := loadForTest(t, ProcessAPI)

	if cfg.App.Env != "test" || cfg.App.Secret != strings.Repeat("s", 64) {
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
	if cfg.Token.RedisPrefix != DefaultTokenRedisPrefix ||
		cfg.Token.SessionCacheTTL != DefaultTokenSessionCacheTTL ||
		cfg.Token.SingleSessionPointerTTL != DefaultTokenSingleSessionPointerTTL {
		t.Fatalf("token Redis/cache policy env must be ignored, got %#v", cfg.Token)
	}
	if cfg.Token.RedisDB != 5 {
		t.Fatalf("expected token redis db 5, got %d", cfg.Token.RedisDB)
	}
	if cfg.Queue.Enabled {
		t.Fatalf("expected queue enabled override to false")
	}
	if cfg.Queue.RedisDB != 4 || cfg.Queue.Concurrency != 22 {
		t.Fatalf("unexpected queue config: %#v", cfg.Queue)
	}
	if cfg.Realtime.Enabled {
		t.Fatalf("expected realtime enabled override to false")
	}
	if cfg.Realtime.Publisher != RealtimePublisherNoop {
		t.Fatalf("expected realtime publisher noop, got %q", cfg.Realtime.Publisher)
	}
	if cfg.Realtime.HeartbeatInterval != DefaultRealtimeHeartbeatInterval ||
		cfg.Realtime.SendBuffer != DefaultRealtimeSendBuffer ||
		cfg.Realtime.RedisChannel != DefaultRealtimeRedisChannel {
		t.Fatalf("realtime policy env must be ignored, got %#v", cfg.Realtime)
	}
	if cfg.Scheduler.Enabled {
		t.Fatalf("expected scheduler enabled override to false")
	}
	if cfg.Scheduler.Timezone != DefaultSchedulerTimezone ||
		cfg.Scheduler.LockPrefix != DefaultSchedulerLockPrefix ||
		cfg.Scheduler.LockTTL != DefaultSchedulerLockTTL {
		t.Fatalf("scheduler policy env must be ignored, got %#v", cfg.Scheduler)
	}
	if cfg.AI.ChatStreamMaxDuration != DefaultAIChatStreamMaxDuration ||
		cfg.AI.ChatStreamIdleTimeout != DefaultAIChatStreamIdleTimeout ||
		cfg.AI.RunStaleTimeout != DefaultAIRunStaleTimeout {
		t.Fatalf("AI runtime timeout env must be ignored, got %#v", cfg.AI)
	}
	if !reflect.DeepEqual(cfg.CORS.AllowOrigins, []string{"https://admin.example.com", "http://localhost:5173"}) {
		t.Fatalf("unexpected cors origins: %#v", cfg.CORS.AllowOrigins)
	}
	wantCORSDefaults := DefaultCORSConfig()
	if !reflect.DeepEqual(cfg.CORS.AllowHeaders, wantCORSDefaults.AllowHeaders) {
		t.Fatalf("cors allow headers env must be ignored, got %#v", cfg.CORS.AllowHeaders)
	}
	if !reflect.DeepEqual(cfg.CORS.ExposeHeaders, wantCORSDefaults.ExposeHeaders) {
		t.Fatalf("cors expose headers must stay default, got %#v", cfg.CORS.ExposeHeaders)
	}
	if cfg.CORS.AllowCredentials != wantCORSDefaults.AllowCredentials {
		t.Fatalf("cors credentials env must be ignored, got %v", cfg.CORS.AllowCredentials)
	}
	if cfg.CORS.MaxAge != wantCORSDefaults.MaxAge {
		t.Fatalf("cors max age env must be ignored, got %s", cfg.CORS.MaxAge)
	}
}

func TestConfigDoesNotExposeAppName(t *testing.T) {
	unsetEnvForTest(t, "APP_ENV")
	t.Setenv(deprecatedAppNameEnvKey(), "admin-api-test")

	cfg := loadForTest(t, ProcessAPI)
	appType := reflect.TypeOf(cfg.App)
	if _, ok := appType.FieldByName("Name"); ok {
		t.Fatalf("AppConfig must not expose Name; process identity is owned by cmd entrypoints and Compose services")
	}
	if cfg.App.Env != "local" {
		t.Fatalf("expected app env local, got %q", cfg.App.Env)
	}
}

func TestDockerFirstEnvDoesNotDocumentAppName(t *testing.T) {
	deprecatedKey := deprecatedAppNameEnvKey()
	paths := []string{
		filepath.Join("..", "..", "deploy", "docker-first", "admin-go.env.example"),
		filepath.Join("..", "..", "deploy", "docker-first", "admin-go.env"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(content), deprecatedKey) {
			t.Fatalf("%s must not document deprecated shared app-name env key", path)
		}
	}
}

func deprecatedAppNameEnvKey() string {
	return strings.Join([]string{"APP", "NAME"}, "_")
}

func TestNormalizeSchedulerConfigAppliesCodeOwnedDefaults(t *testing.T) {
	cfg := NormalizeSchedulerConfig(SchedulerConfig{Enabled: true})

	if !cfg.Enabled {
		t.Fatalf("expected enabled flag to be preserved")
	}
	if cfg.Timezone != DefaultSchedulerTimezone {
		t.Fatalf("expected default timezone %q, got %q", DefaultSchedulerTimezone, cfg.Timezone)
	}
	if cfg.LockPrefix != DefaultSchedulerLockPrefix {
		t.Fatalf("expected default lock prefix %q, got %q", DefaultSchedulerLockPrefix, cfg.LockPrefix)
	}
	if cfg.LockTTL != DefaultSchedulerLockTTL {
		t.Fatalf("expected default lock ttl %s, got %s", DefaultSchedulerLockTTL, cfg.LockTTL)
	}
}

func TestNormalizeTokenConfigAppliesCodeOwnedDefaults(t *testing.T) {
	cfg := NormalizeTokenConfig(TokenConfig{RedisPrefix: "   "})

	if cfg.RedisPrefix != DefaultTokenRedisPrefix {
		t.Fatalf("expected default token redis prefix %q, got %q", DefaultTokenRedisPrefix, cfg.RedisPrefix)
	}
	if cfg.SessionCacheTTL != DefaultTokenSessionCacheTTL {
		t.Fatalf("expected default token session cache ttl %s, got %s", DefaultTokenSessionCacheTTL, cfg.SessionCacheTTL)
	}
	if cfg.SingleSessionPointerTTL != DefaultTokenSingleSessionPointerTTL {
		t.Fatalf("expected default single session pointer ttl %s, got %s", DefaultTokenSingleSessionPointerTTL, cfg.SingleSessionPointerTTL)
	}
}

func TestNormalizeTokenConfigPreservesExplicitValues(t *testing.T) {
	cfg := NormalizeTokenConfig(TokenConfig{
		RedisPrefix:             " custom-token: ",
		SessionCacheTTL:         45 * time.Minute,
		SingleSessionPointerTTL: 48 * time.Hour,
		RedisDB:                 5,
	})

	if cfg.RedisPrefix != "custom-token:" {
		t.Fatalf("expected trimmed token redis prefix custom-token:, got %q", cfg.RedisPrefix)
	}
	if cfg.SessionCacheTTL != 45*time.Minute {
		t.Fatalf("expected explicit token session cache ttl 45m, got %s", cfg.SessionCacheTTL)
	}
	if cfg.SingleSessionPointerTTL != 48*time.Hour {
		t.Fatalf("expected explicit single session pointer ttl 48h, got %s", cfg.SingleSessionPointerTTL)
	}
	if cfg.RedisDB != 5 {
		t.Fatalf("expected explicit token redis db 5, got %d", cfg.RedisDB)
	}
}

func TestNormalizeTokenConfigDefaultsNonPositiveDurations(t *testing.T) {
	cfg := NormalizeTokenConfig(TokenConfig{
		RedisPrefix:             "token:",
		SessionCacheTTL:         -time.Second,
		SingleSessionPointerTTL: -time.Hour,
	})

	if cfg.SessionCacheTTL != DefaultTokenSessionCacheTTL {
		t.Fatalf("expected negative session cache ttl to default to %s, got %s", DefaultTokenSessionCacheTTL, cfg.SessionCacheTTL)
	}
	if cfg.SingleSessionPointerTTL != DefaultTokenSingleSessionPointerTTL {
		t.Fatalf("expected negative pointer ttl to default to %s, got %s", DefaultTokenSingleSessionPointerTTL, cfg.SingleSessionPointerTTL)
	}
}

func TestNormalizeSchedulerConfigTrimsExplicitValues(t *testing.T) {
	cfg := NormalizeSchedulerConfig(SchedulerConfig{
		Timezone:   " UTC ",
		LockPrefix: " custom:scheduler: ",
		LockTTL:    45 * time.Second,
	})

	if cfg.Timezone != "UTC" {
		t.Fatalf("expected trimmed timezone UTC, got %q", cfg.Timezone)
	}
	if cfg.LockPrefix != "custom:scheduler:" {
		t.Fatalf("expected trimmed lock prefix, got %q", cfg.LockPrefix)
	}
	if cfg.LockTTL != 45*time.Second {
		t.Fatalf("expected explicit lock ttl 45s, got %s", cfg.LockTTL)
	}
}

func TestNormalizeAIConfigAppliesCodeOwnedDefaults(t *testing.T) {
	cfg := NormalizeAIConfig(AIConfig{})

	if cfg.ChatStreamMaxDuration != DefaultAIChatStreamMaxDuration {
		t.Fatalf("expected default AI chat stream max duration %s, got %s", DefaultAIChatStreamMaxDuration, cfg.ChatStreamMaxDuration)
	}
	if cfg.ChatStreamIdleTimeout != DefaultAIChatStreamIdleTimeout {
		t.Fatalf("expected default AI chat stream idle timeout %s, got %s", DefaultAIChatStreamIdleTimeout, cfg.ChatStreamIdleTimeout)
	}
	if cfg.RunStaleTimeout != DefaultAIRunStaleTimeout {
		t.Fatalf("expected default AI run stale timeout %s, got %s", DefaultAIRunStaleTimeout, cfg.RunStaleTimeout)
	}
}

func TestNormalizeAIConfigPreservesExplicitValues(t *testing.T) {
	cfg := NormalizeAIConfig(AIConfig{
		ChatStreamMaxDuration: 7 * time.Minute,
		ChatStreamIdleTimeout: 90 * time.Second,
		RunStaleTimeout:       22 * time.Minute,
	})

	if cfg.ChatStreamMaxDuration != 7*time.Minute {
		t.Fatalf("expected explicit AI chat stream max duration 7m, got %s", cfg.ChatStreamMaxDuration)
	}
	if cfg.ChatStreamIdleTimeout != 90*time.Second {
		t.Fatalf("expected explicit AI chat stream idle timeout 90s, got %s", cfg.ChatStreamIdleTimeout)
	}
	if cfg.RunStaleTimeout != 22*time.Minute {
		t.Fatalf("expected explicit AI run stale timeout 22m, got %s", cfg.RunStaleTimeout)
	}
}

func TestLoadReadsAppSecret(t *testing.T) {
	t.Setenv("APP_SECRET", strings.Repeat("a", 64))

	cfg := loadForTest(t, ProcessAPI)

	if cfg.App.Secret != strings.Repeat("a", 64) {
		t.Fatalf("expected APP_SECRET to be loaded")
	}
}

func TestLoadReadsOnePreviousAppSecret(t *testing.T) {
	current := strings.Repeat("c", 64)
	previous := strings.Repeat("p", 64)
	t.Setenv("APP_SECRET", current)
	t.Setenv("APP_SECRET_PREVIOUS", previous)

	cfg := loadForTest(t, ProcessAPI)

	if len(cfg.App.PreviousSecrets) != 1 || cfg.App.PreviousSecrets[0] != previous {
		t.Fatalf("APP_SECRET_PREVIOUS = %#v, want one byte-preserved value", cfg.App.PreviousSecrets)
	}
}

func TestValidateRuntimeSecretsRejectsInvalidPreviousSet(t *testing.T) {
	current := strings.Repeat("c", 64)
	tests := []struct {
		name     string
		previous []string
	}{
		{name: "same as current", previous: []string{current}},
		{name: "unsafe", previous: []string{"short"}},
		{name: "more than one", previous: []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{App: AppConfig{Secret: current, PreviousSecrets: test.previous}}
			if err := ValidateRuntimeSecrets(cfg); err == nil {
				t.Fatal("expected previous secret configuration to be rejected")
			}
		})
	}
}

func TestValidateRuntimeSecretsRejectsMissingAppSecret(t *testing.T) {
	cfg := Config{App: AppConfig{Env: "local"}}

	err := ValidateRuntimeSecrets(cfg)

	if err == nil || !strings.Contains(err.Error(), "APP_SECRET") {
		t.Fatalf("expected APP_SECRET validation error, got %v", err)
	}
}

func TestValidateRuntimeSecretsRejectsDefaultAppSecret(t *testing.T) {
	cfg := Config{App: AppConfig{Env: "local", Secret: "change_me_to_at_least_64_random_chars"}}

	err := ValidateRuntimeSecrets(cfg)

	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe APP_SECRET validation error, got %v", err)
	}
}

func TestValidateRuntimeSecretsAcceptsLongAppSecret(t *testing.T) {
	cfg := Config{App: AppConfig{Env: "local", Secret: strings.Repeat("k", 64)}}

	if err := ValidateRuntimeSecrets(cfg); err != nil {
		t.Fatalf("expected APP_SECRET to pass validation: %v", err)
	}
}

func TestLoadReadsPaymentConfig(t *testing.T) {
	t.Setenv("PAYMENT_CERT_BASE_DIR", "E:/admin_go/admin_back_go")

	cfg := loadForTest(t, ProcessAPI)

	if cfg.Payment.CertBaseDir != "E:/admin_go/admin_back_go" {
		t.Fatalf("expected payment cert base dir to point at Go backend, got %q", cfg.Payment.CertBaseDir)
	}
}

func TestDockerEnvExampleUsesContainerPaymentCertBase(t *testing.T) {
	values := readEnvExample(t)

	if values["PAYMENT_CERT_BASE_DIR"] != "/app" {
		t.Fatalf("expected Docker PAYMENT_CERT_BASE_DIR to point at container app root, got %q", values["PAYMENT_CERT_BASE_DIR"])
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

func TestDockerFirstEnvDoesNotDocumentCaptchaRuntimePolicy(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}

		for _, key := range []string{"CAPTCHA_TTL", "CAPTCHA_REDIS_PREFIX", "CAPTCHA_SLIDE_PADDING"} {
			if _, ok := values[key]; ok {
				t.Fatalf("%s should move to system_settings or code constant, not Docker env file %s", key, fileName)
			}
		}
	}
}

func TestDockerFirstEnvDoesNotDocumentVerifyCodeRuntimePolicy(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}

		for _, key := range []string{"VERIFY_CODE_TTL", "VERIFY_CODE_REDIS_PREFIX"} {
			if _, ok := values[key]; ok {
				t.Fatalf("%s should move to system_settings or code constant, not Docker env file %s", key, fileName)
			}
		}
	}
}

func TestDockerFirstEnvDocumentsOnlyTokenRuntimeKnobs(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		if strings.TrimSpace(values["APP_SECRET"]) == "" {
			t.Fatalf("deploy/docker-first/%s must keep APP_SECRET", fileName)
		}
		if got := values["TOKEN_REDIS_DB"]; got != "2" {
			t.Fatalf("deploy/docker-first/%s must keep TOKEN_REDIS_DB=2, got %q", fileName, got)
		}
		for _, key := range deprecatedTokenSessionEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document token/session policy key %s", fileName, key)
			}
		}
	}
}

func deprecatedTokenSessionEnvKeys() []string {
	return []string{
		"TOKEN_REDIS_PREFIX",
		"TOKEN_SESSION_CACHE_TTL",
		"TOKEN_SINGLE_SESSION_POINTER_TTL",
	}
}

func TestDockerFirstEnvDocumentsOnlyLogDir(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		if got := values["LOG_DIR"]; got != "/app/runtime/logs" {
			t.Fatalf("deploy/docker-first/%s must keep LOG_DIR=/app/runtime/logs, got %q", fileName, got)
		}
		for _, key := range deprecatedLoggingEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document logging policy key %s", fileName, key)
			}
		}
	}
}

func deprecatedLoggingEnvKeys() []string {
	return []string{
		"LOG_ENABLE_FILE",
		"LOG_FILE_NAME",
		"LOG_API_FILE_NAME",
		"LOG_WORKER_FILE_NAME",
		"LOG_MAX_TAIL_LINES",
		"LOG_ALLOWED_EXTENSIONS",
		"LOG_FILE_MAX_SIZE_MB",
		"LOG_FILE_MAX_BACKUPS",
		"LOG_FILE_MAX_AGE_DAYS",
		"LOG_FILE_COMPRESS",
	}
}

func TestDockerFirstEnvDocumentsOnlyQueueRuntimeKnobs(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		if got := values["QUEUE_ENABLED"]; got != "true" {
			t.Fatalf("deploy/docker-first/%s must keep QUEUE_ENABLED=true, got %q", fileName, got)
		}
		if got := values["QUEUE_REDIS_DB"]; got != "3" {
			t.Fatalf("deploy/docker-first/%s must keep QUEUE_REDIS_DB=3, got %q", fileName, got)
		}
		if got := values["QUEUE_CONCURRENCY"]; got != "10" {
			t.Fatalf("deploy/docker-first/%s must keep QUEUE_CONCURRENCY=10, got %q", fileName, got)
		}
		for _, key := range deprecatedQueuePolicyEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document queue policy key %s", fileName, key)
			}
		}
	}
}

func deprecatedQueuePolicyEnvKeys() []string {
	return []string{
		"QUEUE_DEFAULT_QUEUE",
		"QUEUE_CRITICAL_WEIGHT",
		"QUEUE_DEFAULT_WEIGHT",
		"QUEUE_LOW_WEIGHT",
		"QUEUE_SHUTDOWN_TIMEOUT",
		"QUEUE_DEFAULT_MAX_RETRY",
		"QUEUE_DEFAULT_TIMEOUT",
	}
}

func TestDockerFirstEnvDocumentsOnlyRealtimeRuntimeKnobs(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		if got := values["REALTIME_ENABLED"]; got != "true" {
			t.Fatalf("deploy/docker-first/%s must keep REALTIME_ENABLED=true, got %q", fileName, got)
		}
		if got := values["REALTIME_PUBLISHER"]; got != RealtimePublisherRedis {
			t.Fatalf("deploy/docker-first/%s must keep REALTIME_PUBLISHER=redis, got %q", fileName, got)
		}
		for _, key := range deprecatedRealtimePolicyEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document realtime policy key %s", fileName, key)
			}
		}
	}
}

func TestDockerFirstEnvDocumentsOnlyCORSOrigin(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		if strings.TrimSpace(values["CORS_ALLOW_ORIGINS"]) == "" {
			t.Fatalf("deploy/docker-first/%s must keep CORS_ALLOW_ORIGINS", fileName)
		}
		for _, key := range deprecatedCORSPolicyEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document CORS policy key %s", fileName, key)
			}
		}
	}
}

func TestDockerFirstComposeUsesDeveloperDefaultsWithoutProjectEnvFile(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "deploy", "docker-first", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	text := string(content)

	for _, key := range []string{
		"ADMIN_BACK_GO_DIR",
		"ADMIN_GO_ENV_FILE",
		"ADMIN_GO_RUNTIME_DIR",
		"ADMIN_GO_EXPORTS_DIR",
		"ADMIN_API_HOST_BIND",
		"ADMIN_API_HOST_PORT",
	} {
		if strings.Contains(text, key) {
			t.Fatalf("docker-compose.yml should not require project env key %s", key)
		}
	}
	for _, want := range []string{
		"context: ../..",
		"- ./admin-go.env",
		"127.0.0.1:8080:8080",
		"- ./runtime:/app/runtime",
		"- ./exports:/app/exports",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("docker-compose.yml missing developer default %q", want)
		}
	}
}

func deprecatedCORSPolicyEnvKeys() []string {
	return []string{
		"CORS_ALLOW_HEADERS",
		"CORS_ALLOW_CREDENTIALS",
		"CORS_MAX_AGE",
	}
}

func deprecatedRealtimePolicyEnvKeys() []string {
	return []string{
		"REALTIME_REDIS_CHANNEL",
		"REALTIME_HEARTBEAT_INTERVAL",
		"REALTIME_SEND_BUFFER",
	}
}

func TestConfigDoesNotExposeVerifyCodeRuntimePolicy(t *testing.T) {
	if _, ok := reflect.TypeOf(Config{}).FieldByName("VerifyCode"); ok {
		t.Fatalf("verify-code runtime policy should not be loaded from env config")
	}
}

func TestDockerFirstDeployAssetsDoNotUseLocalBinaryDockerfile(t *testing.T) {
	deployDir := filepath.Join("..", "..", "deploy", "docker-first")

	if _, err := os.Stat(filepath.Join(deployDir, "Dockerfile.local")); !os.IsNotExist(err) {
		t.Fatalf("deploy/docker-first must not keep Dockerfile.local; use the root Dockerfile multi-stage build only")
	}

	composeBytes, err := os.ReadFile(filepath.Join(deployDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read deploy/docker-first/docker-compose.yml: %v", err)
	}
	compose := string(composeBytes)
	for _, disallowed := range []string{"ADMIN_BACK_GO_DOCKERFILE", "Dockerfile.local", ".docker-bin"} {
		if strings.Contains(compose, disallowed) {
			t.Fatalf("docker-compose.yml must not reference local-binary Docker build path %q", disallowed)
		}
	}
}

func TestDockerFirstEnvDoesNotDocumentAITimeoutPolicy(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		for _, key := range deprecatedAITimeoutEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document AI timeout policy key %s", fileName, key)
			}
		}
	}
}

func deprecatedAITimeoutEnvKeys() []string {
	return []string{
		"AI_CHAT_STREAM_MAX_DURATION",
		"AI_CHAT_STREAM_IDLE_TIMEOUT",
		"AI_RUN_STALE_TIMEOUT",
	}
}

func TestDockerFirstEnvDocumentsOnlySchedulerRuntimeKnob(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		if len(values) == 0 {
			continue
		}
		if got := values["SCHEDULER_ENABLED"]; got != "true" {
			t.Fatalf("deploy/docker-first/%s must keep SCHEDULER_ENABLED=true, got %q", fileName, got)
		}
		for _, key := range deprecatedSchedulerPolicyEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document scheduler policy key %s", fileName, key)
			}
		}
	}
}

func deprecatedSchedulerPolicyEnvKeys() []string {
	return []string{
		"SCHEDULER_TIMEZONE",
		"SCHEDULER_LOCK_PREFIX",
		"SCHEDULER_LOCK_TTL",
	}
}

func TestDefaultCORSConfigUsesCodeOwnedPolicy(t *testing.T) {
	cfg := DefaultCORSConfig()

	if !reflect.DeepEqual(cfg.AllowOrigins, []string{"http://localhost:5173", "http://127.0.0.1:5173"}) {
		t.Fatalf("unexpected default cors origins: %#v", cfg.AllowOrigins)
	}
	for _, origin := range []string{"http://localhost:5174", "http://127.0.0.1:5174"} {
		if containsString(cfg.AllowOrigins, origin) {
			t.Fatalf("default cors origins must not include %s: %#v", origin, cfg.AllowOrigins)
		}
	}
	for _, header := range []string{
		"Origin",
		"Content-Type",
		"Accept",
		"Accept-Language",
		"Authorization",
		"platform",
		"device-id",
		"X-Trace-Id",
		"X-Request-Id",
	} {
		if !containsString(cfg.AllowHeaders, header) {
			t.Fatalf("DefaultCORSConfig must allow %s, got %#v", header, cfg.AllowHeaders)
		}
	}
	if containsString(cfg.AllowHeaders, "X-Admin-Client-Variant") {
		t.Fatalf("DefaultCORSConfig must not allow retired client variant header: %#v", cfg.AllowHeaders)
	}
	if !reflect.DeepEqual(cfg.ExposeHeaders, []string{"X-Request-Id"}) {
		t.Fatalf("unexpected default cors expose headers: %#v", cfg.ExposeHeaders)
	}
	if !cfg.AllowCredentials {
		t.Fatalf("expected default cors credentials to be true")
	}
	if cfg.MaxAge != 12*time.Hour {
		t.Fatalf("expected default cors max age 12h, got %s", cfg.MaxAge)
	}
}

func TestLoadBuildsMySQLDSNFromLegacyDBEnvironment(t *testing.T) {
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_DATABASE", "admin")
	t.Setenv("DB_USERNAME", "admin_user")
	t.Setenv("DB_PASSWORD", "secret")

	cfg := loadForTest(t, ProcessAPI)

	want := "admin_user:secret@tcp(127.0.0.1:3307)/admin?charset=utf8mb4&parseTime=True&loc=Local"
	if cfg.MySQL.DSN != want {
		t.Fatalf("expected legacy mysql dsn %q, got %q", want, cfg.MySQL.DSN)
	}
}

func TestLoadBuildsRedisAddrFromLegacyRedisEnvironment(t *testing.T) {
	t.Setenv("REDIS_HOST", "127.0.0.1")
	t.Setenv("REDIS_PORT", "6380")

	cfg := loadForTest(t, ProcessAPI)

	if cfg.Redis.Addr != "127.0.0.1:6380" {
		t.Fatalf("expected redis addr 127.0.0.1:6380, got %q", cfg.Redis.Addr)
	}
}

func TestDockerFirstEnvDoesNotDocumentUploadRuntimePolicy(t *testing.T) {
	for _, fileName := range []string{"admin-go.env", "admin-go.env.example"} {
		values := readDockerFirstEnvIfExists(t, fileName)
		for _, key := range uploadRuntimePolicyEnvKeys() {
			if _, ok := values[key]; ok {
				t.Fatalf("deploy/docker-first/%s must not document upload runtime policy key %s", fileName, key)
			}
		}
	}
}

func TestConfigDoesNotExposeUploadRuntimePolicy(t *testing.T) {
	if _, ok := reflect.TypeOf(Config{}).FieldByName("UploadToken"); ok {
		t.Fatalf("Config must not expose UploadToken runtime policy")
	}
}

func uploadRuntimePolicyEnvKeys() []string {
	return []string{
		joinEnvKey("UPLOAD", "TOKEN", "TTL"),
		joinEnvKey("UPLOAD", "KEY", "RANDOM", "BYTES"),
		joinEnvKey("COS", "STS", "ENABLED"),
		joinEnvKey("COS", "STS", "ENDPOINT"),
		joinEnvKey("COS", "STS", "REGION"),
	}
}

func joinEnvKey(parts ...string) string {
	return strings.Join(parts, "_")
}

func TestLoadDotEnvReadsLocalEnvFile(t *testing.T) {
	unsetEnvForTest(t, "APP_ENV")
	unsetEnvForTest(t, "HTTP_ADDR")
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("APP_ENV=dotenv\nHTTP_ADDR=:19090\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := LoadDotEnv(envPath); err != nil {
		t.Fatalf("LoadDotEnv returned error: %v", err)
	}

	cfg := loadForTest(t, ProcessAPI)
	if cfg.App.Env != "dotenv" {
		t.Fatalf("expected app env from .env, got %q", cfg.App.Env)
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
	return readDockerFirstEnv(t, "admin-go.env.example")
}

func readDockerFirstEnv(t *testing.T, fileName string) map[string]string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "deploy", "docker-first", fileName))
	if err != nil {
		t.Fatalf("read deploy/docker-first/%s: %v", fileName, err)
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

func readDockerFirstEnvIfExists(t *testing.T, fileName string) map[string]string {
	t.Helper()
	path := filepath.Join("..", "..", "deploy", "docker-first", fileName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}
		}
		t.Fatalf("stat deploy/docker-first/%s: %v", fileName, err)
	}
	return readDockerFirstEnv(t, fileName)
}
