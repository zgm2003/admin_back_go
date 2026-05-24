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
	App       AppConfig
	HTTP      HTTPConfig
	Logging   LoggingConfig
	MySQL     MySQLConfig
	Redis     RedisConfig
	Token     TokenConfig
	Queue     QueueConfig
	Realtime  RealtimeConfig
	Scheduler SchedulerConfig
	Payment   PaymentConfig
	AI        AIConfig
	CORS      CORSConfig
}

type AppConfig struct {
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

const (
	defaultLogDir            = "runtime/logs"
	defaultAPIFileName       = "admin-api.log"
	defaultWorkerFileName    = "admin-worker.log"
	defaultMaxTailLines      = 2000
	defaultFileMaxSizeMB     = 64
	defaultFileMaxBackups    = 7
	defaultFileMaxAgeDays    = 14
	defaultLogFileCompress   = true
	defaultLogFileEnableFile = true
)

func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		EnableFile:        defaultLogFileEnableFile,
		Dir:               filepath.FromSlash(defaultLogDir),
		FileName:          defaultAPIFileName,
		APIFileName:       defaultAPIFileName,
		WorkerFileName:    defaultWorkerFileName,
		MaxTailLines:      defaultMaxTailLines,
		AllowedExtensions: []string{".log"},
		FileMaxSizeMB:     defaultFileMaxSizeMB,
		FileMaxBackups:    defaultFileMaxBackups,
		FileMaxAgeDays:    defaultFileMaxAgeDays,
		FileCompress:      defaultLogFileCompress,
	}
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

const (
	DefaultTokenRedisPrefix             = "token:"
	DefaultTokenSessionCacheTTL         = 30 * time.Minute
	DefaultTokenSingleSessionPointerTTL = 30 * 24 * time.Hour
	DefaultTokenRedisDB                 = 2
)

func NormalizeTokenConfig(cfg TokenConfig) TokenConfig {
	cfg.RedisPrefix = strings.TrimSpace(cfg.RedisPrefix)
	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = DefaultTokenRedisPrefix
	}
	if cfg.SessionCacheTTL <= 0 {
		cfg.SessionCacheTTL = DefaultTokenSessionCacheTTL
	}
	if cfg.SingleSessionPointerTTL <= 0 {
		cfg.SingleSessionPointerTTL = DefaultTokenSingleSessionPointerTTL
	}
	return cfg
}

type QueueConfig struct {
	Enabled     bool
	RedisDB     int
	Concurrency int
}

const (
	RealtimePublisherLocal = "local"
	RealtimePublisherNoop  = "noop"
	RealtimePublisherRedis = "redis"

	DefaultRealtimeRedisChannel      = "admin_go:realtime:publish"
	DefaultRealtimeHeartbeatInterval = 25 * time.Second
	DefaultRealtimeSendBuffer        = 16
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

const (
	DefaultSchedulerTimezone   = "Asia/Shanghai"
	DefaultSchedulerLockPrefix = "admin_go:scheduler:"
	DefaultSchedulerLockTTL    = 30 * time.Second
)

func NormalizeSchedulerConfig(cfg SchedulerConfig) SchedulerConfig {
	cfg.Timezone = strings.TrimSpace(cfg.Timezone)
	if cfg.Timezone == "" {
		cfg.Timezone = DefaultSchedulerTimezone
	}
	cfg.LockPrefix = strings.TrimSpace(cfg.LockPrefix)
	if cfg.LockPrefix == "" {
		cfg.LockPrefix = DefaultSchedulerLockPrefix
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = DefaultSchedulerLockTTL
	}
	return cfg
}

type PaymentConfig struct {
	CertBaseDir string
}

type AIConfig struct {
	ChatStreamMaxDuration time.Duration
	ChatStreamIdleTimeout time.Duration
	RunStaleTimeout       time.Duration
}

const (
	DefaultAIChatStreamMaxDuration = 5 * time.Minute
	DefaultAIChatStreamIdleTimeout = 60 * time.Second
	DefaultAIRunStaleTimeout       = 15 * time.Minute
)

func NormalizeAIConfig(cfg AIConfig) AIConfig {
	if cfg.ChatStreamMaxDuration <= 0 {
		cfg.ChatStreamMaxDuration = DefaultAIChatStreamMaxDuration
	}
	if cfg.ChatStreamIdleTimeout <= 0 {
		cfg.ChatStreamIdleTimeout = DefaultAIChatStreamIdleTimeout
	}
	if cfg.RunStaleTimeout <= 0 {
		cfg.RunStaleTimeout = DefaultAIRunStaleTimeout
	}
	return cfg
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

	loggingConfig := DefaultLoggingConfig()
	loggingConfig.Dir = envString("LOG_DIR", loggingConfig.Dir)

	return Config{
		App: AppConfig{
			Env:    envString("APP_ENV", "local"),
			Secret: envString("APP_SECRET", ""),
		},
		HTTP: HTTPConfig{
			Addr:              envString("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		},
		Logging: loggingConfig,
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
		Token: NormalizeTokenConfig(TokenConfig{
			RedisDB: envInt("TOKEN_REDIS_DB", DefaultTokenRedisDB),
		}),
		Queue: QueueConfig{
			Enabled:     envBool("QUEUE_ENABLED", true),
			RedisDB:     envInt("QUEUE_REDIS_DB", 3),
			Concurrency: envInt("QUEUE_CONCURRENCY", 10),
		},
		Realtime: RealtimeConfig{
			Enabled:           envBool("REALTIME_ENABLED", true),
			Publisher:         envString("REALTIME_PUBLISHER", RealtimePublisherLocal),
			HeartbeatInterval: DefaultRealtimeHeartbeatInterval,
			SendBuffer:        DefaultRealtimeSendBuffer,
			RedisChannel:      DefaultRealtimeRedisChannel,
		},
		Scheduler: NormalizeSchedulerConfig(SchedulerConfig{
			Enabled: envBool("SCHEDULER_ENABLED", true),
		}),
		Payment: PaymentConfig{
			CertBaseDir: envString("PAYMENT_CERT_BASE_DIR", ""),
		},
		AI:   NormalizeAIConfig(AIConfig{}),
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
