package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validConfigForTest() Config {
	return Config{
		App: AppConfig{Env: "local", Secret: strings.Repeat("s", 64)},
		HTTP: HTTPConfig{
			Addr:              ":8080",
			ReadHeaderTimeout: 5 * time.Second,
		},
		MySQL: MySQLConfig{
			DSN:             "user:pass@tcp(db.example.com:3306)/admin",
			MaxOpenConns:    20,
			MaxIdleConns:    10,
			ConnMaxLifetime: time.Hour,
		},
		Redis:     RedisConfig{Addr: "redis.example.com:6379", DB: 0},
		Token:     TokenConfig{RedisDB: DefaultTokenRedisDB},
		Queue:     QueueConfig{Enabled: true, RedisDB: 3, Concurrency: 10},
		Realtime:  RealtimeConfig{Enabled: true, Publisher: RealtimePublisherLocal},
		Scheduler: SchedulerConfig{Enabled: true, Timezone: DefaultSchedulerTimezone},
		CORS:      DefaultCORSConfig(),
	}
}

func productionConfigForTest() Config {
	cfg := validConfigForTest()
	cfg.App.Env = "production"
	cfg.Realtime.Publisher = RealtimePublisherRedis
	cfg.CORS.AllowOrigins = []string{"https://admin.example.com"}
	return cfg
}

func TestValidateRejectsInvalidRuntimeConfig(t *testing.T) {
	certRoot := t.TempDir()
	missingCertRoot := filepath.Join(t.TempDir(), "missing")
	certFile := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(certFile, []byte("test"), 0o600); err != nil {
		t.Fatalf("write certificate fixture: %v", err)
	}
	uncleanCertRoot := certRoot + string(os.PathSeparator) + "."

	tests := []struct {
		name       string
		process    Process
		production bool
		mutate     func(*Config)
		want       string
	}{
		{name: "unsupported process", process: Process("other"), want: "process"},
		{name: "app env", process: ProcessAPI, mutate: func(c *Config) { c.App.Env = "preview" }, want: "APP_ENV"},
		{name: "short secret", process: ProcessAPI, mutate: func(c *Config) { c.App.Secret = strings.Repeat("s", 63) }, want: "APP_SECRET"},
		{name: "sentinel secret", process: ProcessAPI, mutate: func(c *Config) { c.App.Secret = "  change_me_to_long_random  " }, want: "APP_SECRET"},
		{name: "api http required", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "" }, want: "HTTP_ADDR"},
		{name: "api http host port", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "localhost" }, want: "HTTP_ADDR"},
		{name: "api http empty port", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "localhost:" }, want: "HTTP_ADDR"},
		{name: "api http nonnumeric port", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "localhost:http" }, want: "HTTP_ADDR"},
		{name: "api http zero port", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "localhost:0" }, want: "HTTP_ADDR"},
		{name: "api http high port", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "localhost:65536" }, want: "HTTP_ADDR"},
		{name: "worker nonempty http must be valid", process: ProcessWorker, mutate: func(c *Config) { c.HTTP.Addr = "localhost" }, want: "HTTP_ADDR"},
		{name: "mysql required", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.DSN = "" }, want: "MYSQL_DSN"},
		{name: "mysql syntax", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.DSN = "not-a-dsn" }, want: "MYSQL_DSN"},
		{name: "mysql database", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp(db.example.com:3306)/" }, want: "MYSQL_DSN"},
		{name: "mysql open", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.MaxOpenConns = 0 }, want: "MYSQL_MAX_OPEN_CONNS"},
		{name: "mysql idle negative", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.MaxIdleConns = -1 }, want: "MYSQL_MAX_IDLE_CONNS"},
		{name: "mysql idle above open", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.MaxIdleConns = c.MySQL.MaxOpenConns + 1 }, want: "MYSQL_MAX_IDLE_CONNS"},
		{name: "mysql lifetime", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.ConnMaxLifetime = 0 }, want: "MYSQL_CONN_MAX_LIFETIME"},
		{name: "redis required", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "" }, want: "REDIS_ADDR"},
		{name: "redis host port", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "redis.example.com" }, want: "REDIS_ADDR"},
		{name: "redis empty host", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = ":6379" }, want: "REDIS_ADDR"},
		{name: "redis empty port", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "redis.example.com:" }, want: "REDIS_ADDR"},
		{name: "redis nonnumeric port", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "redis.example.com:redis" }, want: "REDIS_ADDR"},
		{name: "redis zero port", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "redis.example.com:0" }, want: "REDIS_ADDR"},
		{name: "redis high port", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "redis.example.com:65536" }, want: "REDIS_ADDR"},
		{name: "redis db", process: ProcessAPI, mutate: func(c *Config) { c.Redis.DB = -1 }, want: "REDIS_DB"},
		{name: "token redis db", process: ProcessAPI, mutate: func(c *Config) { c.Token.RedisDB = -1 }, want: "TOKEN_REDIS_DB"},
		{name: "queue redis db", process: ProcessAPI, mutate: func(c *Config) { c.Queue.RedisDB = -1 }, want: "QUEUE_REDIS_DB"},
		{name: "production mysql localhost", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp(localhost:3306)/admin" }, want: "MYSQL_DSN"},
		{name: "production mysql IPv6 loopback", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp([::1]:3306)/admin" }, want: "MYSQL_DSN"},
		{name: "production mysql mapped loopback", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp([::ffff:127.0.0.1]:3306)/admin" }, want: "MYSQL_DSN"},
		{name: "production mysql unspecified IPv4", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp(0.0.0.0:3306)/admin" }, want: "MYSQL_DSN"},
		{name: "production mysql unspecified IPv6", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp([::]:3306)/admin" }, want: "MYSQL_DSN"},
		{name: "production mysql multicast", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp(239.1.1.1:3306)/admin" }, want: "MYSQL_DSN"},
		{name: "production mysql unix socket", process: ProcessAPI, production: true, mutate: func(c *Config) { c.MySQL.DSN = "user:pass@unix(/var/run/mysqld/mysqld.sock)/admin" }, want: "MYSQL_DSN"},
		{name: "production redis localhost", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "localhost:6379" }, want: "REDIS_ADDR"},
		{name: "production redis loopback", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "127.0.0.1:6379" }, want: "REDIS_ADDR"},
		{name: "production redis IPv6 loopback", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "[::1]:6379" }, want: "REDIS_ADDR"},
		{name: "production redis mapped link local", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "[::ffff:169.254.1.8]:6379" }, want: "REDIS_ADDR"},
		{name: "production redis unspecified IPv4", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "0.0.0.0:6379" }, want: "REDIS_ADDR"},
		{name: "production redis unspecified IPv6", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "[::]:6379" }, want: "REDIS_ADDR"},
		{name: "production redis multicast", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Redis.Addr = "[ff0e::1]:6379" }, want: "REDIS_ADDR"},
		{name: "queue concurrency zero", process: ProcessAPI, mutate: func(c *Config) { c.Queue.Concurrency = 0 }, want: "QUEUE_CONCURRENCY"},
		{name: "queue concurrency negative", process: ProcessAPI, mutate: func(c *Config) { c.Queue.Concurrency = -1 }, want: "QUEUE_CONCURRENCY"},
		{name: "scheduler queue", process: ProcessAPI, mutate: func(c *Config) { c.Queue.Enabled = false }, want: "SCHEDULER_ENABLED"},
		{name: "scheduler timezone empty", process: ProcessAPI, mutate: func(c *Config) { c.Scheduler.Timezone = "" }, want: "SCHEDULER_TIMEZONE"},
		{name: "scheduler timezone invalid", process: ProcessAPI, mutate: func(c *Config) { c.Scheduler.Timezone = "Mars/Olympus" }, want: "SCHEDULER_TIMEZONE"},
		{name: "realtime publisher empty", process: ProcessAPI, mutate: func(c *Config) { c.Realtime.Publisher = "" }, want: "REALTIME_PUBLISHER"},
		{name: "realtime publisher unsupported", process: ProcessAPI, mutate: func(c *Config) { c.Realtime.Publisher = "kafka" }, want: "REALTIME_PUBLISHER"},
		{name: "production realtime", process: ProcessAPI, production: true, mutate: func(c *Config) { c.Realtime.Publisher = RealtimePublisherLocal }, want: "REALTIME_PUBLISHER"},
		{name: "cors relative", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"admin.example.com"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors scheme", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"ftp://admin.example.com"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors user info", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://user@admin.example.com"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors query", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com?x=1"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors empty query", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com?"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors fragment", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com#x"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors path", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com/app"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "cors invalid port", process: ProcessAPI, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com:bad"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin http", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"http://admin.example.com"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin localhost", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://localhost"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin loopback", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://127.0.0.1"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin private", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://172.16.0.8"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin private IPv6", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://[fc00::1]"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin mapped private", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://[::ffff:172.16.0.8]"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "production origin multicast", process: ProcessAPI, production: true, mutate: func(c *Config) { c.CORS.AllowOrigins = []string{"https://[ff0e::1]"} }, want: "CORS_ALLOW_ORIGINS"},
		{name: "payment relative", process: ProcessAPI, mutate: func(c *Config) { c.Payment.CertBaseDir = "certs" }, want: "PAYMENT_CERT_BASE_DIR"},
		{name: "payment unclean", process: ProcessAPI, mutate: func(c *Config) { c.Payment.CertBaseDir = uncleanCertRoot }, want: "PAYMENT_CERT_BASE_DIR"},
		{name: "payment missing", process: ProcessAPI, mutate: func(c *Config) { c.Payment.CertBaseDir = missingCertRoot }, want: "PAYMENT_CERT_BASE_DIR"},
		{name: "payment file", process: ProcessAPI, mutate: func(c *Config) { c.Payment.CertBaseDir = certFile }, want: "PAYMENT_CERT_BASE_DIR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfigForTest()
			if tt.production {
				cfg = productionConfigForTest()
			}
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}
			err := Validate(tt.process, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error=%v, want key %q", err, tt.want)
			}
		})
	}
}

func TestValidateProductionAcceptsPrivateStateNodes(t *testing.T) {
	tests := []struct {
		name      string
		mysqlDSN  string
		redisAddr string
	}{
		{
			name:      "private IPv4",
			mysqlDSN:  "user:pass@tcp(10.0.0.8:3306)/admin",
			redisAddr: "192.168.1.8:6379",
		},
		{
			name:      "IPv6 ULA",
			mysqlDSN:  "user:pass@tcp([fd00::8]:3306)/admin",
			redisAddr: "[fd00::9]:6379",
		},
		{
			name:      "IPv4-mapped private",
			mysqlDSN:  "user:pass@tcp([::ffff:10.0.0.8]:3306)/admin",
			redisAddr: "[::ffff:192.168.1.8]:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productionConfigForTest()
			cfg.MySQL.DSN = tt.mysqlDSN
			cfg.Redis.Addr = tt.redisAddr

			if err := Validate(ProcessAPI, cfg); err != nil {
				t.Fatalf("Validate(production private state nodes) error=%v", err)
			}
		})
	}
}

func TestValidateProductionDependencyErrorsDoNotClaimPrivateHosts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
		raw    string
	}{
		{
			name:   "mysql localhost",
			mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp(localhost:3306)/admin" },
			want:   "MYSQL_DSN",
			raw:    "user:pass@tcp(localhost:3306)/admin",
		},
		{
			name:   "redis localhost",
			mutate: func(c *Config) { c.Redis.Addr = "localhost:6379" },
			want:   "REDIS_ADDR",
			raw:    "localhost:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productionConfigForTest()
			tt.mutate(&cfg)

			err := Validate(ProcessAPI, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error=%v, want key %q", err, tt.want)
			}
			if strings.Contains(strings.ToLower(err.Error()), "private") || strings.Contains(err.Error(), tt.raw) {
				t.Fatalf("validation error exposed or misclassified dependency: %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidCORSOriginPorts(t *testing.T) {
	origins := []string{
		"https://admin.example.com:",
		"https://admin.example.com:0",
		"https://admin.example.com:65536",
		"https://admin.example.com:bad",
	}

	for _, origin := range origins {
		t.Run(origin, func(t *testing.T) {
			cfg := validConfigForTest()
			cfg.CORS.AllowOrigins = []string{origin}

			err := Validate(ProcessAPI, cfg)
			if err == nil || !strings.Contains(err.Error(), "CORS_ALLOW_ORIGINS") {
				t.Fatalf("Validate() error=%v, want CORS_ALLOW_ORIGINS error", err)
			}
			if strings.Contains(err.Error(), origin) {
				t.Fatalf("validation error exposed origin: %v", err)
			}
		})
	}
}

func TestValidateAPIRequiresAtLeastOneCORSOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origins []string
	}{
		{name: "nil origins", origins: nil},
		{name: "empty origins", origins: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfigForTest()
			cfg.CORS.AllowOrigins = tt.origins

			err := Validate(ProcessAPI, cfg)
			if err == nil || !strings.Contains(err.Error(), "CORS_ALLOW_ORIGINS") {
				t.Fatalf("Validate() error=%v, want CORS_ALLOW_ORIGINS error", err)
			}
		})
	}
}

func TestValidateProductionWorkerIgnoresAPIOnlyCORS(t *testing.T) {
	cfg := productionConfigForTest()
	cfg.HTTP.Addr = ""
	cfg.CORS = DefaultCORSConfig()

	if err := Validate(ProcessWorker, cfg); err != nil {
		t.Fatalf("Validate(production worker) error=%v", err)
	}
}

func TestValidateProductionWorkerStillChecksRuntimeRequirements(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name:   "mysql topology",
			mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp(127.0.0.1:3306)/admin" },
			want:   "MYSQL_DSN",
		},
		{
			name:   "redis topology",
			mutate: func(c *Config) { c.Redis.Addr = "127.0.0.1:6379" },
			want:   "REDIS_ADDR",
		},
		{
			name:   "realtime publisher",
			mutate: func(c *Config) { c.Realtime.Publisher = RealtimePublisherLocal },
			want:   "REALTIME_PUBLISHER",
		},
		{
			name:   "scheduler timezone",
			mutate: func(c *Config) { c.Scheduler.Timezone = "Mars/Olympus" },
			want:   "SCHEDULER_TIMEZONE",
		},
		{
			name:   "payment directory",
			mutate: func(c *Config) { c.Payment.CertBaseDir = "relative-certs" },
			want:   "PAYMENT_CERT_BASE_DIR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productionConfigForTest()
			cfg.HTTP.Addr = ""
			cfg.CORS.AllowOrigins = nil
			tt.mutate(&cfg)

			err := Validate(ProcessWorker, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate(production worker) error=%v, want key %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsEmptyCORSFragmentDelimiter(t *testing.T) {
	const origin = "https://admin.example.com#"
	cfg := validConfigForTest()
	cfg.CORS.AllowOrigins = []string{origin}

	err := Validate(ProcessAPI, cfg)
	if err == nil || !strings.Contains(err.Error(), "CORS_ALLOW_ORIGINS") {
		t.Fatalf("Validate() error=%v, want CORS_ALLOW_ORIGINS error", err)
	}
	if strings.Contains(err.Error(), origin) {
		t.Fatalf("validation error exposed origin: %v", err)
	}
}

func TestValidateRejectsProcessLocalSchedulerTimezone(t *testing.T) {
	timezones := []string{"Local", "  lOcAl  "}

	for _, timezone := range timezones {
		t.Run(timezone, func(t *testing.T) {
			cfg := validConfigForTest()
			cfg.Scheduler.Timezone = timezone

			err := Validate(ProcessAPI, cfg)
			if err == nil || !strings.Contains(err.Error(), "SCHEDULER_TIMEZONE") {
				t.Fatalf("Validate() error=%v, want SCHEDULER_TIMEZONE error", err)
			}
			if strings.Contains(err.Error(), timezone) {
				t.Fatalf("validation error exposed timezone: %v", err)
			}
		})
	}
}

func TestValidateProductionRejectsLocalhostSubdomainsAndZonedLinkLocalHosts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
		raw    string
	}{
		{
			name: "cors localhost subdomain",
			mutate: func(c *Config) {
				c.CORS.AllowOrigins = []string{"https://api.localhost"}
			},
			want: "CORS_ALLOW_ORIGINS",
			raw:  "https://api.localhost",
		},
		{
			name: "mysql localhost subdomain",
			mutate: func(c *Config) {
				c.MySQL.DSN = "user:pass@tcp(db.localhost:3306)/admin"
			},
			want: "MYSQL_DSN",
			raw:  "user:pass@tcp(db.localhost:3306)/admin",
		},
		{
			name: "redis localhost subdomain",
			mutate: func(c *Config) {
				c.Redis.Addr = "cache.localhost:6379"
			},
			want: "REDIS_ADDR",
			raw:  "cache.localhost:6379",
		},
		{
			name: "cors zoned link local IPv6",
			mutate: func(c *Config) {
				c.CORS.AllowOrigins = []string{"https://[fe80::1%25eth0]"}
			},
			want: "CORS_ALLOW_ORIGINS",
			raw:  "https://[fe80::1%25eth0]",
		},
		{
			name: "mysql zoned link local IPv6",
			mutate: func(c *Config) {
				c.MySQL.DSN = "user:pass@tcp([fe80::1%eth0]:3306)/admin"
			},
			want: "MYSQL_DSN",
			raw:  "user:pass@tcp([fe80::1%eth0]:3306)/admin",
		},
		{
			name: "redis zoned link local IPv6",
			mutate: func(c *Config) {
				c.Redis.Addr = "[fe80::1%eth0]:6379"
			},
			want: "REDIS_ADDR",
			raw:  "[fe80::1%eth0]:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productionConfigForTest()
			tt.mutate(&cfg)

			err := Validate(ProcessAPI, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error=%v, want key %q", err, tt.want)
			}
			if strings.Contains(err.Error(), tt.raw) {
				t.Fatalf("validation error exposed configuration value: %v", err)
			}
		})
	}
}

func TestValidateProductionRejectsAllZonedIPHosts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
		raw    string
	}{
		{
			name:   "mysql zoned ULA",
			mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp([fd00::8%eth0]:3306)/admin" },
			want:   "MYSQL_DSN",
			raw:    "user:pass@tcp([fd00::8%eth0]:3306)/admin",
		},
		{
			name:   "mysql zoned global",
			mutate: func(c *Config) { c.MySQL.DSN = "user:pass@tcp([2001:4860:4860::8888%eth0]:3306)/admin" },
			want:   "MYSQL_DSN",
			raw:    "user:pass@tcp([2001:4860:4860::8888%eth0]:3306)/admin",
		},
		{
			name:   "redis zoned ULA",
			mutate: func(c *Config) { c.Redis.Addr = "[fd00::9%eth0]:6379" },
			want:   "REDIS_ADDR",
			raw:    "[fd00::9%eth0]:6379",
		},
		{
			name:   "redis zoned global",
			mutate: func(c *Config) { c.Redis.Addr = "[2001:4860:4860::8888%eth0]:6379" },
			want:   "REDIS_ADDR",
			raw:    "[2001:4860:4860::8888%eth0]:6379",
		},
		{
			name: "cors zoned ULA",
			mutate: func(c *Config) {
				c.CORS.AllowOrigins = []string{"https://[fd00::8%25eth0]"}
			},
			want: "CORS_ALLOW_ORIGINS",
			raw:  "https://[fd00::8%25eth0]",
		},
		{
			name: "cors zoned global",
			mutate: func(c *Config) {
				c.CORS.AllowOrigins = []string{"https://[2001:4860:4860::8888%25eth0]"}
			},
			want: "CORS_ALLOW_ORIGINS",
			raw:  "https://[2001:4860:4860::8888%25eth0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := productionConfigForTest()
			tt.mutate(&cfg)

			err := Validate(ProcessAPI, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error=%v, want key %q", err, tt.want)
			}
			if strings.Contains(err.Error(), tt.raw) {
				t.Fatalf("validation error exposed configuration value: %v", err)
			}
		})
	}
}

func TestIsLocalOrPrivateHostRecognizesLocalBoundaries(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "LOCALHOST.", want: true},
		{host: "api.localhost", want: true},
		{host: "API.LOCALHOST.", want: true},
		{host: "fe80::1%eth0", want: true},
		{host: "fd00::1%eth0", want: true},
		{host: "2001:4860:4860::8888%eth0", want: true},
		{host: "::ffff:10.0.0.8", want: true},
		{host: "::ffff:127.0.0.1", want: true},
		{host: "::ffff:0.0.0.0", want: true},
		{host: "::ffff:169.254.1.8", want: true},
		{host: "::ffff:239.1.1.1", want: true},
		{host: "localhost.example.com", want: false},
		{host: "example-localhost", want: false},
		{host: "2001:4860:4860::8888", want: false},
		{host: "::ffff:8.8.8.8", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLocalOrPrivateHost(tt.host); got != tt.want {
				t.Fatalf("isLocalOrPrivateHost()=%t, want %t", got, tt.want)
			}
		})
	}
}

func TestIsLocalOrUnusableDependencyHostClassification(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "db.localhost.", want: true},
		{host: "127.0.0.1", want: true},
		{host: "::1", want: true},
		{host: "0.0.0.0", want: true},
		{host: "::", want: true},
		{host: "169.254.1.8", want: true},
		{host: "fe80::1%eth0", want: true},
		{host: "fd00::8%eth0", want: true},
		{host: "2001:4860:4860::8888%eth0", want: true},
		{host: "239.1.1.1", want: true},
		{host: "ff0e::1", want: true},
		{host: "::ffff:127.0.0.1", want: true},
		{host: "::ffff:0.0.0.0", want: true},
		{host: "::ffff:169.254.1.8", want: true},
		{host: "::ffff:239.1.1.1", want: true},
		{host: "10.0.0.8", want: false},
		{host: "192.168.1.8", want: false},
		{host: "fd00::8", want: false},
		{host: "::ffff:10.0.0.8", want: false},
		{host: "::ffff:192.168.1.8", want: false},
		{host: "db.example.com", want: false},
		{host: "2001:4860:4860::8888", want: false},
		{host: "::ffff:8.8.8.8", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLocalOrUnusableDependencyHost(tt.host); got != tt.want {
				t.Fatalf("isLocalOrUnusableDependencyHost()=%t, want %t", got, tt.want)
			}
		})
	}
}

func TestValidateAcceptsAppEnvironments(t *testing.T) {
	values := []string{
		"local",
		"development",
		"test",
		"staging",
		"production",
		"  StAgInG  ",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			cfg := validConfigForTest()
			if strings.EqualFold(strings.TrimSpace(value), "production") {
				cfg = productionConfigForTest()
			}
			cfg.App.Env = value

			if err := Validate(ProcessAPI, cfg); err != nil {
				t.Fatalf("Validate(APP_ENV) error=%v", err)
			}
			if cfg.App.Env != value {
				t.Fatalf("Validate() mutated APP_ENV: got %q, want %q", cfg.App.Env, value)
			}
		})
	}
}

func TestValidateAcceptsProcessSpecificConfig(t *testing.T) {
	api := validConfigForTest()
	api.Payment.CertBaseDir = filepath.ToSlash(t.TempDir())
	api.CORS.AllowOrigins = []string{"https://admin.example.com/"}
	if err := Validate(ProcessAPI, api); err != nil {
		t.Fatalf("Validate(api): %v", err)
	}

	worker := validConfigForTest()
	worker.HTTP.Addr = ""
	if err := Validate(ProcessWorker, worker); err != nil {
		t.Fatalf("Validate(worker): %v", err)
	}

	production := productionConfigForTest()
	if err := Validate(ProcessAPI, production); err != nil {
		t.Fatalf("Validate(production api): %v", err)
	}

	disabled := validConfigForTest()
	disabled.Queue.Enabled = false
	disabled.Scheduler.Enabled = false
	disabled.Realtime.Enabled = false
	disabled.Realtime.Publisher = RealtimePublisherLocal
	if err := Validate(ProcessAPI, disabled); err != nil {
		t.Fatalf("Validate(api with disabled optional runtimes): %v", err)
	}

	rawByteSecret := validConfigForTest()
	rawByteSecret.App.Secret = strings.Repeat("s", 62) + "  "
	if err := Validate(ProcessAPI, rawByteSecret); err != nil {
		t.Fatalf("Validate(64-byte raw APP_SECRET): %v", err)
	}
}

func TestValidateErrorsDoNotExposeConfigValues(t *testing.T) {
	missingCertRoot := filepath.Join(t.TempDir(), "private-missing-cert-root")
	tests := []struct {
		name      string
		process   Process
		mutate    func(*Config)
		forbidden []string
	}{
		{name: "process", process: Process("private-process"), forbidden: []string{"private-process"}},
		{name: "app env", process: ProcessAPI, mutate: func(c *Config) { c.App.Env = "private-preview" }, forbidden: []string{"private-preview"}},
		{name: "secret", process: ProcessAPI, mutate: func(c *Config) { c.App.Secret = strings.Repeat("private", 9) }, forbidden: []string{strings.Repeat("private", 9)}},
		{name: "http", process: ProcessAPI, mutate: func(c *Config) { c.HTTP.Addr = "private-http-address" }, forbidden: []string{"private-http-address"}},
		{name: "mysql", process: ProcessAPI, mutate: func(c *Config) { c.MySQL.DSN = "user:private-dsn-password@tcp(db.example.com:3306)" }, forbidden: []string{"private-dsn-password", "user:private-dsn-password@tcp(db.example.com:3306)"}},
		{name: "redis", process: ProcessAPI, mutate: func(c *Config) { c.Redis.Addr = "private-redis-address" }, forbidden: []string{"private-redis-address"}},
		{name: "scheduler", process: ProcessAPI, mutate: func(c *Config) { c.Scheduler.Timezone = "Private/Timezone" }, forbidden: []string{"Private/Timezone"}},
		{name: "cors", process: ProcessAPI, mutate: func(c *Config) {
			c.CORS.AllowOrigins = []string{"https://private-user:private-password@admin.example.com"}
		}, forbidden: []string{"private-user", "private-password", "https://private-user:private-password@admin.example.com"}},
		{name: "payment", process: ProcessAPI, mutate: func(c *Config) { c.Payment.CertBaseDir = missingCertRoot }, forbidden: []string{missingCertRoot}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfigForTest()
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}
			err := Validate(tt.process, cfg)
			if err == nil {
				t.Fatal("Validate() error=nil, want validation error")
			}
			for _, value := range tt.forbidden {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("validation error exposed configuration value: %v", err)
				}
			}
		})
	}
}

func validEnvironmentForTest() map[string]string {
	return map[string]string{
		"APP_ENV":                  "local",
		"APP_SECRET":               strings.Repeat("s", 64),
		"HTTP_ADDR":                ":8080",
		"HTTP_READ_HEADER_TIMEOUT": "5s",
		"MYSQL_DSN":                "user:pass@tcp(db.example.com:3306)/admin",
		"MYSQL_MAX_OPEN_CONNS":     "20",
		"MYSQL_MAX_IDLE_CONNS":     "10",
		"MYSQL_CONN_MAX_LIFETIME":  "1h",
		"REDIS_ADDR":               "redis.example.com:6379",
		"REDIS_PASSWORD":           "opaque-password",
		"REDIS_DB":                 "0",
		"TOKEN_REDIS_DB":           "2",
		"QUEUE_ENABLED":            "true",
		"QUEUE_REDIS_DB":           "3",
		"QUEUE_CONCURRENCY":        "10",
		"REALTIME_ENABLED":         "true",
		"REALTIME_PUBLISHER":       RealtimePublisherLocal,
		"SCHEDULER_ENABLED":        "true",
		"CORS_ALLOW_ORIGINS":       "http://localhost:5173",
		"PAYMENT_CERT_BASE_DIR":    "",
		"LOG_DIR":                  "runtime/logs",
		"DB_HOST":                  "legacy-db.example.com",
		"DB_PORT":                  "3306",
		"DB_DATABASE":              "legacy_admin",
		"DB_USERNAME":              "legacy_user",
		"DB_PASSWORD":              "legacy-password",
		"REDIS_HOST":               "legacy-redis.example.com",
		"REDIS_PORT":               "6379",
	}
}

func setEnvironmentForTest(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func TestLoadActivatesProcessValidation(t *testing.T) {
	values := validEnvironmentForTest()
	values["QUEUE_ENABLED"] = "false"
	values["SCHEDULER_ENABLED"] = "false"
	setEnvironmentForTest(t, values)

	_, err := Load(ProcessWorker)
	if err == nil || !strings.Contains(err.Error(), "QUEUE_ENABLED must be true for admin-worker") {
		t.Fatalf("Load(worker) error=%v", err)
	}
}

func TestLoadAPIRejectsCommaOnlyCORSOrigins(t *testing.T) {
	values := validEnvironmentForTest()
	values["CORS_ALLOW_ORIGINS"] = ",,,"
	setEnvironmentForTest(t, values)

	_, err := Load(ProcessAPI)
	if err == nil || !strings.Contains(err.Error(), "CORS_ALLOW_ORIGINS") {
		t.Fatalf("Load(api) error=%v, want CORS_ALLOW_ORIGINS error", err)
	}
	if strings.Contains(err.Error(), values["CORS_ALLOW_ORIGINS"]) {
		t.Fatalf("validation error exposed CORS_ALLOW_ORIGINS value: %v", err)
	}
}

func TestLoadAcceptsValidAPIEnvironment(t *testing.T) {
	values := validEnvironmentForTest()
	setEnvironmentForTest(t, values)

	cfg, err := Load(ProcessAPI)
	if err != nil {
		t.Fatalf("Load(api) error=%v", err)
	}
	if len(cfg.CORS.AllowOrigins) != 1 || cfg.CORS.AllowOrigins[0] != values["CORS_ALLOW_ORIGINS"] {
		t.Fatalf("unexpected API CORS origins: %#v", cfg.CORS.AllowOrigins)
	}
}

func TestLoadProductionAcceptsPrivateStateNodes(t *testing.T) {
	values := validEnvironmentForTest()
	values["APP_ENV"] = "production"
	values["MYSQL_DSN"] = "user:pass@tcp(10.0.0.8:3306)/admin"
	values["REDIS_ADDR"] = "192.168.1.8:6379"
	values["REALTIME_PUBLISHER"] = RealtimePublisherRedis
	values["CORS_ALLOW_ORIGINS"] = "https://admin.example.com"
	setEnvironmentForTest(t, values)

	if _, err := Load(ProcessAPI); err != nil {
		t.Fatalf("Load(production private state nodes) error=%v", err)
	}
}

func TestLoadProductionWorkerIgnoresAPIOnlyCORS(t *testing.T) {
	values := validEnvironmentForTest()
	values["APP_ENV"] = "production"
	values["REALTIME_PUBLISHER"] = RealtimePublisherRedis
	values["CORS_ALLOW_ORIGINS"] = ",,,"
	setEnvironmentForTest(t, values)

	cfg, err := Load(ProcessWorker)
	if err != nil {
		t.Fatalf("Load(production worker) error=%v", err)
	}
	if len(cfg.CORS.AllowOrigins) != 0 {
		t.Fatalf("expected comma-only CORS input to parse empty, got %#v", cfg.CORS.AllowOrigins)
	}
}

func TestValidateProductionRejectsUnsafeTopology(t *testing.T) {
	cfg := validConfigForTest()
	cfg.App.Env = "production"
	cfg.MySQL.DSN = "user:pass@tcp(127.0.0.1:3306)/admin"
	cfg.Redis.Addr = "redis.example.com:6379"
	cfg.Realtime.Publisher = RealtimePublisherRedis
	cfg.CORS.AllowOrigins = []string{"https://admin.example.com"}

	err := Validate(ProcessAPI, cfg)
	if err == nil || !strings.Contains(err.Error(), "MYSQL_DSN") {
		t.Fatalf("expected production MYSQL_DSN validation error, got %v", err)
	}
}

func TestValidateWorkerRequiresQueue(t *testing.T) {
	cfg := validConfigForTest()
	cfg.Queue.Enabled = false
	cfg.Scheduler.Enabled = false

	err := Validate(ProcessWorker, cfg)
	if err == nil || !strings.Contains(err.Error(), "QUEUE_ENABLED must be true for admin-worker") {
		t.Fatalf("expected worker queue validation error, got %v", err)
	}
}
