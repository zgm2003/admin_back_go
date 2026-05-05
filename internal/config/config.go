package config

import (
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
	Secretbox   SecretboxConfig
	UploadToken UploadTokenConfig
	CORS        CORSConfig
}

type AppConfig struct {
	Name string
	Env  string
}

type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
}

type LoggingConfig struct {
	EnableFile        bool
	Dir               string
	FileName          string
	MaxTailLines      int
	AllowedExtensions []string
	FileMaxSizeMB     int
	FileMaxBackups    int
	FileMaxAgeDays    int
	FileCompress      bool
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
	Pepper                  string
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
	TTL         time.Duration
	RedisPrefix string
	DevMode     bool
	DevCode     string
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
)

type RealtimeConfig struct {
	Enabled           bool
	Publisher         string
	HeartbeatInterval time.Duration
	SendBuffer        int
}

type SchedulerConfig struct {
	Enabled    bool
	Timezone   string
	LockPrefix string
}

type SecretboxConfig struct {
	Key string
}

type UploadTokenConfig struct {
	TTL            time.Duration
	KeyRandomBytes int
	COS            COSSTSConfig
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

	return Config{
		App: AppConfig{
			Name: envString("APP_NAME", "admin-api"),
			Env:  envString("APP_ENV", "local"),
		},
		HTTP: HTTPConfig{
			Addr:              envString("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		},
		Logging: LoggingConfig{
			EnableFile:        envBool("LOG_ENABLE_FILE", true),
			Dir:               envString("LOG_DIR", filepath.Join("runtime", "logs")),
			FileName:          envString("LOG_FILE_NAME", "admin-api.log"),
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
			Pepper:                  envString("TOKEN_PEPPER", ""),
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
			TTL:         envDuration("VERIFY_CODE_TTL", 5*time.Minute),
			RedisPrefix: envString("VERIFY_CODE_REDIS_PREFIX", "auth:verify_code:"),
			DevMode:     envBool("VERIFY_CODE_DEV_MODE", strings.EqualFold(envString("APP_ENV", "local"), "local")),
			DevCode:     envString("VERIFY_CODE_DEV_CODE", "123456"),
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
		},
		Scheduler: SchedulerConfig{
			Enabled:    envBool("SCHEDULER_ENABLED", true),
			Timezone:   envString("SCHEDULER_TIMEZONE", "Asia/Shanghai"),
			LockPrefix: envString("SCHEDULER_LOCK_PREFIX", "admin_go:scheduler:"),
		},
		Secretbox: SecretboxConfig{
			Key: envString("VAULT_KEY", ""),
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
		CORS: corsConfig,
	}
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
