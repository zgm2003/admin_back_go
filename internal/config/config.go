package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Process string

const (
	ProcessAPI    Process = "admin-api"
	ProcessWorker Process = "admin-worker"
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

// Snapshot returns a process-owned configuration value. Config is otherwise a
// value type, but its slice fields would still alias the caller without this
// boundary copy.
func Snapshot(cfg Config) Config {
	cfg.App.PreviousSecrets = cloneConfigStrings(cfg.App.PreviousSecrets)
	cfg.Logging.AllowedExtensions = cloneConfigStrings(cfg.Logging.AllowedExtensions)
	cfg.CORS.AllowOrigins = cloneConfigStrings(cfg.CORS.AllowOrigins)
	cfg.CORS.AllowMethods = cloneConfigStrings(cfg.CORS.AllowMethods)
	cfg.CORS.AllowHeaders = cloneConfigStrings(cfg.CORS.AllowHeaders)
	cfg.CORS.ExposeHeaders = cloneConfigStrings(cfg.CORS.ExposeHeaders)
	return cfg
}

func cloneConfigStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append(make([]string, 0, len(values)), values...)
}

type AppConfig struct {
	Env             string
	Secret          string
	PreviousSecrets []string
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

func Load(process Process) (Config, error) {
	cfg, err := loadFrom(osLookup)
	if err != nil {
		return Config{}, err
	}
	if err := Validate(process, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadFrom(lookup lookupEnv) (Config, error) {
	readHeaderTimeout, err := envPeriod(lookup, "HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxOpenConns, err := envInteger(lookup, "MYSQL_MAX_OPEN_CONNS", 20, true)
	if err != nil {
		return Config{}, err
	}
	maxIdleConns, err := envInteger(lookup, "MYSQL_MAX_IDLE_CONNS", 10, false)
	if err != nil {
		return Config{}, err
	}
	connMaxLifetime, err := envPeriod(lookup, "MYSQL_CONN_MAX_LIFETIME", time.Hour)
	if err != nil {
		return Config{}, err
	}
	redisDB, err := envInteger(lookup, "REDIS_DB", 0, false)
	if err != nil {
		return Config{}, err
	}
	tokenRedisDB, err := envInteger(lookup, "TOKEN_REDIS_DB", DefaultTokenRedisDB, false)
	if err != nil {
		return Config{}, err
	}
	queueEnabled, err := envBoolean(lookup, "QUEUE_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	queueRedisDB, err := envInteger(lookup, "QUEUE_REDIS_DB", 3, false)
	if err != nil {
		return Config{}, err
	}
	queueConcurrency, err := envInteger(lookup, "QUEUE_CONCURRENCY", 10, true)
	if err != nil {
		return Config{}, err
	}
	realtimeEnabled, err := envBoolean(lookup, "REALTIME_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	schedulerEnabled, err := envBoolean(lookup, "SCHEDULER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}

	corsConfig := DefaultCORSConfig()
	corsConfig.AllowOrigins = envList(lookup, "CORS_ALLOW_ORIGINS", corsConfig.AllowOrigins)

	loggingConfig := DefaultLoggingConfig()
	loggingConfig.Dir = envText(lookup, "LOG_DIR", loggingConfig.Dir)

	return Config{
		App: AppConfig{
			Env:             envText(lookup, "APP_ENV", "local"),
			Secret:          envOpaque(lookup, "APP_SECRET", ""),
			PreviousSecrets: optionalSecret(envOpaque(lookup, "APP_SECRET_PREVIOUS", "")),
		},
		HTTP: HTTPConfig{
			Addr:              envText(lookup, "HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: readHeaderTimeout,
		},
		Logging: loggingConfig,
		MySQL: MySQLConfig{
			DSN:             envText(lookup, "MYSQL_DSN", legacyMySQLDSN(lookup)),
			MaxOpenConns:    maxOpenConns,
			MaxIdleConns:    maxIdleConns,
			ConnMaxLifetime: connMaxLifetime,
		},
		Redis: RedisConfig{
			Addr:     envText(lookup, "REDIS_ADDR", legacyRedisAddr(lookup)),
			Password: envOpaque(lookup, "REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		Token: NormalizeTokenConfig(TokenConfig{
			RedisDB: tokenRedisDB,
		}),
		Queue: QueueConfig{
			Enabled:     queueEnabled,
			RedisDB:     queueRedisDB,
			Concurrency: queueConcurrency,
		},
		Realtime: RealtimeConfig{
			Enabled:           realtimeEnabled,
			Publisher:         envText(lookup, "REALTIME_PUBLISHER", RealtimePublisherLocal),
			HeartbeatInterval: DefaultRealtimeHeartbeatInterval,
			SendBuffer:        DefaultRealtimeSendBuffer,
			RedisChannel:      DefaultRealtimeRedisChannel,
		},
		Scheduler: NormalizeSchedulerConfig(SchedulerConfig{
			Enabled: schedulerEnabled,
		}),
		Payment: PaymentConfig{
			CertBaseDir: envText(lookup, "PAYMENT_CERT_BASE_DIR", ""),
		},
		AI:   NormalizeAIConfig(AIConfig{}),
		CORS: corsConfig,
	}, nil
}

var unsafeAppSecrets = map[string]struct{}{
	"":                                      {},
	"change_me_to_at_least_64_random_chars": {},
	"change_me_to_long_random":              {},
}

func ValidateRuntimeSecrets(cfg Config) error {
	if err := validateAppSecret("APP_SECRET", cfg.App.Secret); err != nil {
		return err
	}
	if len(cfg.App.PreviousSecrets) > 1 {
		return fmt.Errorf("APP_SECRET_PREVIOUS supports at most one key")
	}
	for _, previous := range cfg.App.PreviousSecrets {
		if err := validateAppSecret("APP_SECRET_PREVIOUS", previous); err != nil {
			return err
		}
		if strings.TrimSpace(previous) == strings.TrimSpace(cfg.App.Secret) {
			return fmt.Errorf("APP_SECRET_PREVIOUS must differ from APP_SECRET")
		}
	}
	return nil
}

func optionalSecret(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func validateAppSecret(name string, value string) error {
	if _, unsafe := unsafeAppSecrets[strings.TrimSpace(value)]; unsafe {
		return fmt.Errorf("%s is missing or unsafe", name)
	}
	if len(value) < 64 {
		return fmt.Errorf("%s is too short: got %d bytes, need at least 64", name, len(value))
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

func legacyMySQLDSN(lookup lookupEnv) string {
	host := envText(lookup, "DB_HOST", "")
	database := envText(lookup, "DB_DATABASE", "")
	username := envText(lookup, "DB_USERNAME", "")
	if host == "" || database == "" || username == "" {
		return ""
	}
	port := envText(lookup, "DB_PORT", "3306")
	password := envOpaque(lookup, "DB_PASSWORD", "")
	return username + ":" + password + "@tcp(" + host + ":" + port + ")/" + database + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func legacyRedisAddr(lookup lookupEnv) string {
	host := envText(lookup, "REDIS_HOST", "")
	if host == "" {
		return ""
	}
	return host + ":" + envText(lookup, "REDIS_PORT", "6379")
}
