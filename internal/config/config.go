package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App         AppConfig
	HTTP        HTTPConfig
	Logging     LoggingConfig
	MySQL       MySQLConfig
	Redis       RedisConfig
	Token       TokenConfig
	Captcha     CaptchaConfig
	VerifyCode  VerifyCodeConfig
	Queue       QueueConfig
	Realtime    RealtimeConfig
	Scheduler   SchedulerConfig
	Payment     PaymentConfig
	UploadToken UploadTokenConfig
	AI          AIConfig
	CORS        CORSConfig
}

type AppConfig struct {
	Name   string
	Env    string
	Secret string
}

type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
}

type LoggingConfig struct {
	EnableFile        bool
	Dir               string
	FileName          string
	APIFileName       string
	WorkerFileName    string
	MaxTailLines      int
	AllowedExtensions []string
	FileMaxSizeMB     int
	FileMaxBackups    int
	FileMaxAgeDays    int
	FileCompress      bool
}

func (c LoggingConfig) ForProcess(process string) LoggingConfig {
	next := c
	switch strings.TrimSpace(process) {
	case "admin-api":
		if strings.TrimSpace(c.APIFileName) != "" {
			next.FileName = strings.TrimSpace(c.APIFileName)
		}
	case "admin-worker":
		if strings.TrimSpace(c.WorkerFileName) != "" {
			next.FileName = strings.TrimSpace(c.WorkerFileName)
		}
	}
	if strings.TrimSpace(next.FileName) == "" {
		next.FileName = strings.TrimSpace(process) + ".log"
	}
	return next
}

type MySQLConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type TokenConfig struct {
	RedisPrefix             string
	SessionCacheTTL         time.Duration
	SingleSessionPointerTTL time.Duration
	RedisDB                 int
}

type CaptchaConfig struct {
	TTL          time.Duration
	RedisPrefix  string
	SlidePadding int
}

type VerifyCodeConfig struct {
	RedisPrefix string
}

type QueueConfig struct {
	Enabled         bool
	RedisDB         int
	Concurrency     int
	DefaultQueue    string
	CriticalWeight  int
	DefaultWeight   int
	LowWeight       int
	ShutdownTimeout time.Duration
	DefaultMaxRetry int
	DefaultTimeout  time.Duration
}

const (
	RealtimePublisherLocal = "local"
	RealtimePublisherNoop  = "noop"
	RealtimePublisherRedis = "redis"
)

type RealtimeConfig struct {
	Enabled           bool
	Publisher         string
	HeartbeatInterval time.Duration
	SendBuffer        int
	RedisChannel      string
}

type SchedulerConfig struct {
	Enabled    bool
	Timezone   string
	LockPrefix string
	LockTTL    time.Duration
}

type PaymentConfig struct {
	CertBaseDir   string
	AlipayTimeout time.Duration
}

type UploadTokenConfig struct {
	TTL            time.Duration
	KeyRandomBytes int
	COS            COSSTSConfig
}

type AIConfig struct {
	ChatStreamMaxDuration time.Duration
	ChatStreamIdleTimeout time.Duration
	RunStaleTimeout       time.Duration
}

type COSSTSConfig struct {
	Enabled  bool
	Endpoint string
	Region   string
}

type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func Load() Config {
	corsConfig := DefaultCORSConfig()
	corsConfig.AllowOrigins = envCSV("CORS_ALLOW_ORIGINS", corsConfig.AllowOrigins)
	corsConfig.AllowHeaders = envCSV("CORS_ALLOW_HEADERS", corsConfig.AllowHeaders)
	corsConfig.AllowCredentials = envBool("CORS_ALLOW_CREDENTIALS", corsConfig.AllowCredentials)
	corsConfig.MaxAge = envDuration("CORS_MAX_AGE", corsConfig.MaxAge)

	logFileName := envString("LOG_FILE_NAME", "admin-api.log")

	return Config{
		App: AppConfig{
			Name:   envString("APP_NAME", "admin-api"),
			Env:    envString("APP_ENV", "local"),
			Secret: envString("APP_SECRET", ""),
		},
		HTTP: HTTPConfig{
			Addr:              envString("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		},
		Logging: LoggingConfig{
			EnableFile:        envBool("LOG_ENABLE_FILE", true),
			Dir:               envString("LOG_DIR", filepath.Join("runtime", "logs")),
			FileName:          logFileName,
			APIFileName:       envString("LOG_API_FILE_NAME", logFileName),
			WorkerFileName:    envString("LOG_WORKER_FILE_NAME", "admin-worker.log"),
			MaxTailLines:      envInt("LOG_MAX_TAIL_LINES", 2000),
			AllowedExtensions: envCSV("LOG_ALLOWED_EXTENSIONS", []string{".log"}),
			FileMaxSizeMB:     envInt("LOG_FILE_MAX_SIZE_MB", 64),
			FileMaxBackups:    envInt("LOG_FILE_MAX_BACKUPS", 7),
			FileMaxAgeDays:    envInt("LOG_FILE_MAX_AGE_DAYS", 14),
			FileCompress:      envBool("LOG_FILE_COMPRESS", true),
		},
		MySQL: MySQLConfig{
			DSN:             envString("MYSQL_DSN", legacyMySQLDSN()),
			MaxOpenConns:    envInt("MYSQL_MAX_OPEN_CONNS", 20),
			MaxIdleConns:    envInt("MYSQL_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: envDuration("MYSQL_CONN_MAX_LIFETIME", time.Hour),
		},
		Redis: RedisConfig{
			Addr:     envString("REDIS_ADDR", legacyRedisAddr()),
			Password: envString("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
		},
		Token: TokenConfig{
			RedisPrefix:             envString("TOKEN_REDIS_PREFIX", "token:"),
			SessionCacheTTL:         envDuration("TOKEN_SESSION_CACHE_TTL", 30*time.Minute),
			SingleSessionPointerTTL: envDuration("TOKEN_SINGLE_SESSION_POINTER_TTL", 30*24*time.Hour),
			RedisDB:                 envInt("TOKEN_REDIS_DB", 2),
		},
		Captcha: CaptchaConfig{
			TTL:          envDuration("CAPTCHA_TTL", 2*time.Minute),
			RedisPrefix:  envString("CAPTCHA_REDIS_PREFIX", "captcha:slide:"),
			SlidePadding: envInt("CAPTCHA_SLIDE_PADDING", 10),
		},
		VerifyCode: VerifyCodeConfig{
			RedisPrefix: envString("VERIFY_CODE_REDIS_PREFIX", "auth:verify_code:"),
		},
		Queue: QueueConfig{
			Enabled:         envBool("QUEUE_ENABLED", true),
			RedisDB:         envInt("QUEUE_REDIS_DB", 3),
			Concurrency:     envInt("QUEUE_CONCURRENCY", 10),
			DefaultQueue:    envString("QUEUE_DEFAULT_QUEUE", "default"),
			CriticalWeight:  envInt("QUEUE_CRITICAL_WEIGHT", 6),
			DefaultWeight:   envInt("QUEUE_DEFAULT_WEIGHT", 3),
			LowWeight:       envInt("QUEUE_LOW_WEIGHT", 1),
			ShutdownTimeout: envDuration("QUEUE_SHUTDOWN_TIMEOUT", 10*time.Second),
			DefaultMaxRetry: envInt("QUEUE_DEFAULT_MAX_RETRY", 3),
			DefaultTimeout:  envDuration("QUEUE_DEFAULT_TIMEOUT", 30*time.Second),
		},
		Realtime: RealtimeConfig{
			Enabled:           envBool("REALTIME_ENABLED", true),
			Publisher:         envString("REALTIME_PUBLISHER", RealtimePublisherLocal),
			HeartbeatInterval: envDuration("REALTIME_HEARTBEAT_INTERVAL", 25*time.Second),
			SendBuffer:        envInt("REALTIME_SEND_BUFFER", 16),
			RedisChannel:      envString("REALTIME_REDIS_CHANNEL", "admin_go:realtime:publish"),
		},
		Scheduler: SchedulerConfig{
			Enabled:    envBool("SCHEDULER_ENABLED", true),
			Timezone:   envString("SCHEDULER_TIMEZONE", "Asia/Shanghai"),
			LockPrefix: envString("SCHEDULER_LOCK_PREFIX", "admin_go:scheduler:"),
			LockTTL:    envDuration("SCHEDULER_LOCK_TTL", 30*time.Second),
		},
		Payment: PaymentConfig{
			CertBaseDir:   envString("PAYMENT_CERT_BASE_DIR", ""),
			AlipayTimeout: envDuration("PAYMENT_ALIPAY_TIMEOUT", 10*time.Second),
		},
		UploadToken: UploadTokenConfig{
			TTL:            envDuration("UPLOAD_TOKEN_TTL", 15*time.Minute),
			KeyRandomBytes: envInt("UPLOAD_KEY_RANDOM_BYTES", 4),
			COS: COSSTSConfig{
				Enabled:  envBool("COS_STS_ENABLED", false),
				Endpoint: envString("COS_STS_ENDPOINT", "sts.tencentcloudapi.com"),
				Region:   envString("COS_STS_REGION", "ap-guangzhou"),
			},
		},
		AI: AIConfig{
			ChatStreamMaxDuration: envDuration("AI_CHAT_STREAM_MAX_DURATION", 5*time.Minute),
			ChatStreamIdleTimeout: envDuration("AI_CHAT_STREAM_IDLE_TIMEOUT", 60*time.Second),
			RunStaleTimeout:       envDuration("AI_RUN_STALE_TIMEOUT", 15*time.Minute),
		},
		CORS: corsConfig,
	}
}

var unsafeAppSecrets = map[string]struct{}{
	"":                                      {},
	"change_me_to_at_least_64_random_chars": {},
	"change_me_to_long_random":              {},
}

func ValidateRuntimeSecrets(cfg Config) error {
	secret := strings.TrimSpace(cfg.App.Secret)
	if _, unsafe := unsafeAppSecrets[secret]; unsafe {
		return fmt.Errorf("APP_SECRET is missing or unsafe")
	}
	if len(secret) < 32 {
		return fmt.Errorf("APP_SECRET is too short: got %d chars, need at least 32", len(secret))
	}
	return nil
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:5174",
			"http://127.0.0.1:5174",
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Accept-Language",
			"Authorization",
			"platform",
			"device-id",
			"X-Trace-Id",
			"X-Request-Id",
		},
		ExposeHeaders:    []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
}

func envString(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func legacyMySQLDSN() string {
	host := os.Getenv("DB_HOST")
	database := os.Getenv("DB_DATABASE")
	username := os.Getenv("DB_USERNAME")
	if host == "" || database == "" || username == "" {
		return ""
	}
	port := envString("DB_PORT", "3306")
	password := os.Getenv("DB_PASSWORD")
	return username + ":" + password + "@tcp(" + host + ":" + port + ")/" + database + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func legacyRedisAddr() string {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		return ""
	}
	return host + ":" + envString("REDIS_PORT", "6379")
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envCSV(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return values
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
