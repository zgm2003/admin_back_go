# Admin Foundation Verification Implementation Plan

> **Superseded delivery note (2026-07-18):** This completed plan's `.github` and GitHub Actions steps are historical evidence and must not be replayed. Web/backend verification and delivery now use repository scripts plus Docker Compose. The only allowed future Workflow is the P08.5 Windows Tauri candidate release defined by the execution index.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make backend configuration, dependency verification, local environment initialization, builds, and CI deterministic before database or architecture changes begin.

**Architecture:** Keep configuration as explicit data parsed from the process environment and validated for either `admin-api` or `admin-worker`. Put developer/CI verification behind repository-owned PowerShell entrypoints, then make GitHub Actions call the same entrypoints from a clean module cache.

**Tech Stack:** Go 1.26.5, PowerShell 7, GitHub Actions, Docker BuildKit, MySQL/Redis runtime configuration.

---

## Target file map

- Create `internal/config/env.go` — strict environment parsing with key-aware errors.
- Create `internal/config/runtime.go` — process enum and process-specific validation.
- Modify `internal/config/config.go` — return `(Config, error)` and remove silent invalid-value fallback.
- Modify `cmd/admin-api/main.go` and `cmd/admin-worker/main.go` — fail before resource construction on config errors.
- Create `deploy/docker-first/init-local-env.ps1` — secret-safe creation of ignored `admin-go.env`.
- Create `scripts/tests/init-local-env.tests.ps1` — executable script behavior test.
- Create `scripts/verify-go-clean.ps1` — clean-cache test/static-analysis/build entrypoint.
- Create `scripts/verify-backend.ps1` — normal local verification wrapper.
- Create `.github/workflows/verify-backend.yml` — blocking backend CI.
- Create `.github/dependabot.yml` — Go modules, Docker, and GitHub Actions update policy.
- Modify `go.sum` — replace only the invalid `asynqmon v0.7.2` content checksum.
- Modify `deploy/docker-first/README.md` and `README.md` — exact supported commands.

### Task 1: Repair and guard the asynqmon checksum

**Files:**
- Create: `internal/architecture/dependency_integrity_test.go`
- Modify: `go.sum:148`

- [x] **Step 1: Write the failing checksum guard**

```go
package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const asynqmonV072Sum = "github.com/hibiken/asynqmon v0.7.2 h1:YohWgTIPwtMyZ6khBDcVUz9BdSdQW2Dxn8SoxtbmjSg="

func TestAsynqmonChecksumMatchesTransparencyLog(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), asynqmonV072Sum+"\n") {
		t.Fatalf("go.sum must contain verified sum %q", asynqmonV072Sum)
	}
}
```

- [x] **Step 2: Run the guard and confirm the repository is wrong**

Run:

```powershell
go test ./internal/architecture -run TestAsynqmonChecksumMatchesTransparencyLog -count=1
```

Expected: FAIL with `go.sum must contain verified sum` or Go's checksum mismatch showing the repository value `EfLR...`.

- [x] **Step 3: Verify the public sum and change one line**

Run:

```powershell
$lookup = Invoke-WebRequest -UseBasicParsing "https://sum.golang.org/lookup/github.com/hibiken/asynqmon@v0.7.2"
if ($lookup.Content -notmatch [regex]::Escape("h1:YohWgTIPwtMyZ6khBDcVUz9BdSdQW2Dxn8SoxtbmjSg=")) {
  throw "sum.golang.org did not return the approved checksum"
}
```

Replace only:

```text
github.com/hibiken/asynqmon v0.7.2 h1:EfLRppj5GlklMPzdCjdonpXz/D23meW0Pk6NAtkOPhw=
```

with:

```text
github.com/hibiken/asynqmon v0.7.2 h1:YohWgTIPwtMyZ6khBDcVUz9BdSdQW2Dxn8SoxtbmjSg=
```

- [x] **Step 4: Verify from a unique empty module cache**

Run:

```powershell
$verifyRoot = Join-Path $env:TEMP ("admin-go-sum-" + [guid]::NewGuid())
$env:GOMODCACHE = Join-Path $verifyRoot "modcache"
New-Item -ItemType Directory -Path $env:GOMODCACHE | Out-Null
go mod download
go mod verify
go test ./internal/architecture -run TestAsynqmonChecksumMatchesTransparencyLog -count=1
```

Expected: all three commands exit 0; `go mod verify` prints `all modules verified`.

- [x] **Step 5: Commit**

```powershell
git add -- go.sum internal/architecture/dependency_integrity_test.go
git commit -m "fix(build): restore verified asynqmon checksum"
```

### Task 2: Add strict environment parsing

**Files:**
- Create: `internal/config/env.go`
- Create: `internal/config/env_test.go`
- Modify: `cmd/admin-api/main.go`
- Modify: `cmd/admin-worker/main.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/logging_process_test.go`
- Modify: `internal/config/logging_test.go`
- Modify: `internal/config/secretbox_config_test.go`

- [x] **Step 1: Write table-driven failures for malformed values**

```go
func TestLoadRejectsMalformedEnvironment(t *testing.T) {
	tests := []struct {
		key, value, want string
	}{
		{"MYSQL_MAX_OPEN_CONNS", "many", "MYSQL_MAX_OPEN_CONNS: parse integer"},
		{"MYSQL_MAX_IDLE_CONNS", "-1", "MYSQL_MAX_IDLE_CONNS: must not be negative"},
		{"REDIS_DB", "-1", "REDIS_DB: must not be negative"},
		{"TOKEN_REDIS_DB", "-1", "TOKEN_REDIS_DB: must not be negative"},
		{"QUEUE_REDIS_DB", "-1", "QUEUE_REDIS_DB: must not be negative"},
		{"MYSQL_CONN_MAX_LIFETIME", "tomorrow", "MYSQL_CONN_MAX_LIFETIME: parse duration"},
		{"QUEUE_ENABLED", "sometimes", "QUEUE_ENABLED: parse boolean"},
		{"QUEUE_CONCURRENCY", "0", "QUEUE_CONCURRENCY: must be greater than zero"},
		{"HTTP_READ_HEADER_TIMEOUT", "-1s", "HTTP_READ_HEADER_TIMEOUT: must be greater than zero"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			_, err := Load(ProcessAPI)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load(%s) error=%v, want substring %q", ProcessAPI, err, tt.want)
			}
		})
	}
}
```

- [x] **Step 2: Run the test and verify the old loader silently falls back**

Run:

```powershell
go test ./internal/config -run TestLoadRejectsMalformedEnvironment -count=1
```

Expected: FAIL to compile because the old `Load` accepts no process and returns only `Config`. This is the intended RED for the new error-returning API; the old implementation also silently falls back for these malformed values.

- [x] **Step 3: Implement strict parsers**

Create `internal/config/env.go`:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type lookupEnv func(string) (string, bool)

func osLookup(key string) (string, bool) { return os.LookupEnv(key) }

func envText(lookup lookupEnv, key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func envInteger(lookup lookupEnv, key string, fallback int, positive bool) (int, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s: parse integer: %w", key, err)
	}
	if positive && value <= 0 {
		return 0, fmt.Errorf("%s: must be greater than zero", key)
	}
	if !positive && value < 0 {
		return 0, fmt.Errorf("%s: must not be negative", key)
	}
	return value, nil
}

func envBoolean(lookup lookupEnv, key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s: parse boolean: %w", key, err)
	}
	return value, nil
}

func envPeriod(lookup lookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s: parse duration: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s: must be greater than zero", key)
	}
	return value, nil
}

func envList(lookup lookupEnv, key string, fallback []string) []string {
	raw, ok := lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return append([]string(nil), fallback...)
	}
	out := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
```

- [x] **Step 4: Change the loader signature and propagate parser errors**

Add a `Process` enum and change the signature to return parser errors while reserving the process argument for Task 3:

```go
type Process string

const (
	ProcessAPI    Process = "admin-api"
	ProcessWorker Process = "admin-worker"
)

func Load(_ Process) (Config, error) {
	return loadFrom(osLookup)
}
```

Task 2 must not call `Validate`: process-specific validation is introduced in Task 3. The typed argument is added now so every repository caller is migrated once and Task 3 can activate validation without another public signature change.

`loadFrom` calls `envInteger` for `MYSQL_MAX_OPEN_CONNS`, `MYSQL_MAX_IDLE_CONNS`, `REDIS_DB`, `TOKEN_REDIS_DB`, `QUEUE_REDIS_DB`, and `QUEUE_CONCURRENCY`; `envBoolean` for `QUEUE_ENABLED`, `REALTIME_ENABLED`, and `SCHEDULER_ENABLED`; and `envPeriod` for `HTTP_READ_HEADER_TIMEOUT` and `MYSQL_CONN_MAX_LIFETIME`. Open connections, queue concurrency, and both durations are positive; idle connections and Redis DB numbers allow zero but reject negatives. Keep only defaults already represented by constants. Do not retain `envInt`, `envBool`, or `envDuration` fallback-on-error behavior.

- [x] **Step 5: Update existing tests to handle `(Config, error)`**

Use this helper in config tests:

```go
func loadForTest(t *testing.T, process Process) Config {
	t.Helper()
	cfg, err := Load(process)
	if err != nil {
		t.Fatalf("Load(%s): %v", process, err)
	}
	return cfg
}
```

Replace every direct `cfg := Load()` call with `cfg := loadForTest(t, ProcessAPI)` in:

- `internal/config/config_test.go`
- `internal/config/logging_process_test.go`
- `internal/config/logging_test.go`
- `internal/config/secretbox_config_test.go`

- [x] **Step 6: Update both binaries to handle parser errors before logger and resource construction**

Use `ProcessAPI` in `cmd/admin-api/main.go` and `ProcessWorker` in `cmd/admin-worker/main.go`:

```go
cfg, err := config.Load(config.ProcessAPI)
if err != nil {
	slog.Error("invalid environment configuration", "error", err)
	os.Exit(1)
}
```

Do not log `cfg` or any environment value. Keep logger construction after this guard; the worker uses the same code with `config.ProcessWorker`.

- [x] **Step 7: Run the focused and full repository tests**

Run:

```powershell
go test ./internal/config -count=1
go test ./... -count=1
go build -o $env:TEMP\admin-api-strict-env.exe ./cmd/admin-api
go build -o $env:TEMP\admin-worker-strict-env.exe ./cmd/admin-worker
```

Expected: all commands exit 0, including malformed integer/duration/boolean and negative-integer cases. This commit must leave every package compiling; do not defer caller migration to Task 3.

- [x] **Step 8: Commit**

```powershell
git add -- cmd/admin-api/main.go cmd/admin-worker/main.go internal/config/config.go internal/config/config_test.go internal/config/env.go internal/config/env_test.go internal/config/logging_process_test.go internal/config/logging_test.go internal/config/secretbox_config_test.go
git commit -m "refactor(config): reject malformed runtime settings"
```

### Task 3: Enforce process-specific runtime configuration

**Files:**
- Create: `internal/config/runtime.go`
- Create: `internal/config/runtime_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [x] **Step 1: Write production and process validation tests**

```go
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
		Redis: RedisConfig{Addr: "redis.example.com:6379", DB: 0},
		Token: TokenConfig{RedisDB: DefaultTokenRedisDB},
		Queue: QueueConfig{Enabled: true, RedisDB: 3, Concurrency: 10},
		Realtime: RealtimeConfig{Enabled: true, Publisher: RealtimePublisherLocal},
		Scheduler: SchedulerConfig{Enabled: true, Timezone: DefaultSchedulerTimezone},
		CORS: DefaultCORSConfig(),
	}
}

func TestValidateProductionRejectsUnsafeTopology(t *testing.T) {
	cfg := validConfigForTest()
	cfg.App.Env = "production"
	cfg.MySQL.DSN = "user:pass@tcp(127.0.0.1:3306)/admin"
	err := Validate(ProcessAPI, cfg)
	if err == nil || !strings.Contains(err.Error(), "MYSQL_DSN") {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateWorkerRequiresQueue(t *testing.T) {
	cfg := validConfigForTest()
	cfg.Queue.Enabled = false
	err := Validate(ProcessWorker, cfg)
	if err == nil || !strings.Contains(err.Error(), "QUEUE_ENABLED must be true for admin-worker") {
		t.Fatalf("error=%v", err)
	}
}
```

Use a production-safe fixture and table-driven coverage for every rule listed in Step 3:

```go
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
		t.Fatal(err)
	}
	uncleanCertRoot := certRoot + string(os.PathSeparator) + "."

	tests := []struct {
		name       string
		process    Process
		production bool
		mutate     func(*Config)
		want       string
	}{
		{"unsupported process", Process("other"), false, nil, "process"},
		{"app env", ProcessAPI, false, func(c *Config) { c.App.Env = "preview" }, "APP_ENV"},
		{"short secret", ProcessAPI, false, func(c *Config) { c.App.Secret = strings.Repeat("s", 63) }, "APP_SECRET"},
		{"api http required", ProcessAPI, false, func(c *Config) { c.HTTP.Addr = "" }, "HTTP_ADDR"},
		{"http host port", ProcessAPI, false, func(c *Config) { c.HTTP.Addr = "localhost" }, "HTTP_ADDR"},
		{"mysql required", ProcessAPI, false, func(c *Config) { c.MySQL.DSN = "" }, "MYSQL_DSN"},
		{"mysql syntax", ProcessAPI, false, func(c *Config) { c.MySQL.DSN = "not-a-dsn" }, "MYSQL_DSN"},
		{"mysql database", ProcessAPI, false, func(c *Config) { c.MySQL.DSN = "user:pass@tcp(db.example.com:3306)/" }, "MYSQL_DSN"},
		{"mysql open", ProcessAPI, false, func(c *Config) { c.MySQL.MaxOpenConns = 0 }, "MYSQL_MAX_OPEN_CONNS"},
		{"mysql idle negative", ProcessAPI, false, func(c *Config) { c.MySQL.MaxIdleConns = -1 }, "MYSQL_MAX_IDLE_CONNS"},
		{"mysql idle above open", ProcessAPI, false, func(c *Config) { c.MySQL.MaxIdleConns = c.MySQL.MaxOpenConns + 1 }, "MYSQL_MAX_IDLE_CONNS"},
		{"mysql lifetime", ProcessAPI, false, func(c *Config) { c.MySQL.ConnMaxLifetime = 0 }, "MYSQL_CONN_MAX_LIFETIME"},
		{"redis required", ProcessAPI, false, func(c *Config) { c.Redis.Addr = "" }, "REDIS_ADDR"},
		{"redis host port", ProcessAPI, false, func(c *Config) { c.Redis.Addr = "redis.example.com" }, "REDIS_ADDR"},
		{"redis db", ProcessAPI, false, func(c *Config) { c.Redis.DB = -1 }, "REDIS_DB"},
		{"token redis db", ProcessAPI, false, func(c *Config) { c.Token.RedisDB = -1 }, "TOKEN_REDIS_DB"},
		{"queue redis db", ProcessAPI, false, func(c *Config) { c.Queue.RedisDB = -1 }, "QUEUE_REDIS_DB"},
		{"worker queue", ProcessWorker, false, func(c *Config) { c.Queue.Enabled = false }, "QUEUE_ENABLED"},
		{"queue concurrency", ProcessAPI, false, func(c *Config) { c.Queue.Concurrency = 0 }, "QUEUE_CONCURRENCY"},
		{"scheduler queue", ProcessAPI, false, func(c *Config) { c.Queue.Enabled = false }, "SCHEDULER_ENABLED"},
		{"scheduler timezone", ProcessAPI, false, func(c *Config) { c.Scheduler.Timezone = "Mars/Olympus" }, "SCHEDULER_TIMEZONE"},
		{"realtime publisher", ProcessAPI, false, func(c *Config) { c.Realtime.Publisher = "kafka" }, "REALTIME_PUBLISHER"},
		{"production realtime", ProcessAPI, true, func(c *Config) { c.Realtime.Publisher = RealtimePublisherLocal }, "REALTIME_PUBLISHER"},
		{"cors scheme", ProcessAPI, false, func(c *Config) { c.CORS.AllowOrigins = []string{"ftp://admin.example.com"} }, "CORS_ALLOW_ORIGINS"},
		{"cors user info", ProcessAPI, false, func(c *Config) { c.CORS.AllowOrigins = []string{"https://user@admin.example.com"} }, "CORS_ALLOW_ORIGINS"},
		{"cors query", ProcessAPI, false, func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com?x=1"} }, "CORS_ALLOW_ORIGINS"},
		{"cors fragment", ProcessAPI, false, func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com#x"} }, "CORS_ALLOW_ORIGINS"},
		{"cors path", ProcessAPI, false, func(c *Config) { c.CORS.AllowOrigins = []string{"https://admin.example.com/app"} }, "CORS_ALLOW_ORIGINS"},
		{"production mysql loopback", ProcessAPI, true, func(c *Config) { c.MySQL.DSN = "user:pass@tcp(127.0.0.1:3306)/admin" }, "MYSQL_DSN"},
		{"production origin http", ProcessAPI, true, func(c *Config) { c.CORS.AllowOrigins = []string{"http://admin.example.com"} }, "CORS_ALLOW_ORIGINS"},
		{"production origin loopback", ProcessAPI, true, func(c *Config) { c.CORS.AllowOrigins = []string{"https://127.0.0.1"} }, "CORS_ALLOW_ORIGINS"},
		{"production origin private", ProcessAPI, true, func(c *Config) { c.CORS.AllowOrigins = []string{"https://172.16.0.8"} }, "CORS_ALLOW_ORIGINS"},
		{"payment relative", ProcessAPI, false, func(c *Config) { c.Payment.CertBaseDir = "certs" }, "PAYMENT_CERT_BASE_DIR"},
		{"payment unclean", ProcessAPI, false, func(c *Config) { c.Payment.CertBaseDir = uncleanCertRoot }, "PAYMENT_CERT_BASE_DIR"},
		{"payment missing", ProcessAPI, false, func(c *Config) { c.Payment.CertBaseDir = missingCertRoot }, "PAYMENT_CERT_BASE_DIR"},
		{"payment file", ProcessAPI, false, func(c *Config) { c.Payment.CertBaseDir = certFile }, "PAYMENT_CERT_BASE_DIR"},
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

func TestValidateAcceptsProcessSpecificConfig(t *testing.T) {
	api := validConfigForTest()
	api.Payment.CertBaseDir = t.TempDir()
	if err := Validate(ProcessAPI, api); err != nil {
		t.Fatalf("Validate(api): %v", err)
	}

	worker := validConfigForTest()
	worker.HTTP.Addr = ""
	if err := Validate(ProcessWorker, worker); err != nil {
		t.Fatalf("Validate(worker): %v", err)
	}

	privateState := productionConfigForTest()
	privateState.MySQL.DSN = "user:pass@tcp(10.0.0.8:3306)/admin"
	privateState.Redis.Addr = "192.168.1.8:6379"
	if err := Validate(ProcessAPI, privateState); err != nil {
		t.Fatalf("Validate(production private state nodes): %v", err)
	}
}
```

Error assertions check configuration keys and reasons, never a secret value. Prove the public loader activates `Validate` before resource construction with an isolated complete environment:

```go
func TestLoadActivatesProcessValidation(t *testing.T) {
	values := map[string]string{
		"APP_ENV":                   "local",
		"APP_SECRET":                strings.Repeat("s", 64),
		"HTTP_ADDR":                 ":8080",
		"HTTP_READ_HEADER_TIMEOUT":  "5s",
		"MYSQL_DSN":                 "user:pass@tcp(db.example.com:3306)/admin",
		"MYSQL_MAX_OPEN_CONNS":      "20",
		"MYSQL_MAX_IDLE_CONNS":      "10",
		"MYSQL_CONN_MAX_LIFETIME":   "1h",
		"REDIS_ADDR":                "redis.example.com:6379",
		"REDIS_DB":                  "0",
		"TOKEN_REDIS_DB":            "2",
		"QUEUE_ENABLED":             "false",
		"QUEUE_REDIS_DB":            "3",
		"QUEUE_CONCURRENCY":         "10",
		"REALTIME_ENABLED":          "true",
		"REALTIME_PUBLISHER":        RealtimePublisherLocal,
		"SCHEDULER_ENABLED":         "false",
		"CORS_ALLOW_ORIGINS":        "http://localhost:5173",
		"PAYMENT_CERT_BASE_DIR":     "",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}

	_, err := Load(ProcessWorker)
	if err == nil || !strings.Contains(err.Error(), "QUEUE_ENABLED must be true for admin-worker") {
		t.Fatalf("Load(worker) error=%v", err)
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test ./internal/config -run 'TestValidate' -count=1`

Expected: FAIL because `Validate` does not exist.

- [x] **Step 3: Implement process validation**

```go
func Validate(process Process, cfg Config) error {
	if process != ProcessAPI && process != ProcessWorker {
		return fmt.Errorf("process %q is unsupported", process)
	}
	if err := ValidateRuntimeSecrets(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.MySQL.DSN) == "" {
		return fmt.Errorf("MYSQL_DSN is required")
	}
	if strings.TrimSpace(cfg.Redis.Addr) == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if cfg.MySQL.MaxIdleConns < 0 || cfg.MySQL.MaxIdleConns > cfg.MySQL.MaxOpenConns {
		return fmt.Errorf("MYSQL_MAX_IDLE_CONNS must be between 0 and MYSQL_MAX_OPEN_CONNS")
	}
	if process == ProcessWorker && !cfg.Queue.Enabled {
		return fmt.Errorf("QUEUE_ENABLED must be true for admin-worker")
	}
	if cfg.Scheduler.Enabled && !cfg.Queue.Enabled {
		return fmt.Errorf("SCHEDULER_ENABLED requires QUEUE_ENABLED")
	}
	if strings.EqualFold(cfg.App.Env, "production") {
		if strings.Contains(cfg.MySQL.DSN, "127.0.0.1") || strings.Contains(cfg.MySQL.DSN, "localhost") {
			return fmt.Errorf("MYSQL_DSN must not use loopback in production")
		}
		if cfg.Realtime.Enabled && cfg.Realtime.Publisher != RealtimePublisherRedis {
			return fmt.Errorf("REALTIME_PUBLISHER must be redis in production")
		}
		for _, origin := range cfg.CORS.AllowOrigins {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return fmt.Errorf("CORS_ALLOW_ORIGINS contains invalid production origin %q", origin)
			}
		}
	}
	return nil
}
```

Complete `Validate` with these explicit rules:

- `APP_ENV` is one of `local`, `development`, `test`, `staging`, or `production`;
- `HTTP_ADDR` and `REDIS_ADDR` parse with `net.SplitHostPort`; Redis DB values are nonnegative;
- `MYSQL_DSN` parses with `mysql.ParseDSN` and contains a database name;
- `MYSQL_MAX_OPEN_CONNS > 0`, `0 <= MYSQL_MAX_IDLE_CONNS <= MYSQL_MAX_OPEN_CONNS`, and connection lifetime is positive;
- API requires HTTP/MySQL/Redis/App secret; Worker requires MySQL/Redis/App secret and `QUEUE_ENABLED=true`;
- queue concurrency is positive; Scheduler requires Queue and a valid IANA timezone;
- Realtime publisher is one of `local`, `noop`, or `redis`; enabled production realtime requires `redis`;
- every CORS origin is an absolute origin with no user-info, query, fragment, or non-root path; production requires HTTPS and rejects loopback/private hosts;
- a non-empty `PAYMENT_CERT_BASE_DIR` is absolute, clean, and exists as a directory; an empty value remains allowed until an enabled payment configuration is assembled in P03;
- `ValidateRuntimeSecrets` rejects the sentinel values and requires at least 64 bytes.

Production MySQL and Redis dependencies reject localhost, loopback, unspecified, and link-local hosts, but allow private-network state nodes as required by the Docker-first production template. Production CORS origins reject both local and private hosts. Use parsed hosts (`mysql.ParseDSN`, `net.SplitHostPort`, and `url.URL.Hostname`) rather than substring checks. Do not perform DNS lookups and do not include a DSN, password, secret, or raw environment value in validation errors.

- [x] **Step 4: Activate validation in the loader before resource construction**

Replace the staged Task 2 loader body with:

```go
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
```

Task 2 already migrated both mains to `Load(ProcessAPI)` / `Load(ProcessWorker)` and exits through `slog.Error` before logger or resource construction. Verify those guards remain intact. Do not log `cfg` or an environment value.

Keep parser/default tests independent from runtime requirements by changing the existing test helper in `internal/config/config_test.go` to:

```go
func loadForTest(t *testing.T, _ Process) Config {
	t.Helper()
	cfg, err := loadFrom(osLookup)
	if err != nil {
		t.Fatalf("loadFrom(): %v", err)
	}
	return cfg
}
```

`runtime_test.go` owns all `Load`/`Validate` behavior tests. Do not make every parser test manufacture database, Redis, and secret requirements merely to exercise parsing.

- [x] **Step 5: Verify**

Run:

```powershell
go test ./internal/config ./cmd/admin-api ./cmd/admin-worker -count=1
go test ./... -count=1
go build -o $env:TEMP\admin-api-foundation.exe ./cmd/admin-api
go build -o $env:TEMP\admin-worker-foundation.exe ./cmd/admin-worker
```

Expected: focused and full repository tests pass and both binaries build.

- [x] **Step 6: Commit**

```powershell
git add -- internal/config/config.go internal/config/config_test.go internal/config/runtime.go internal/config/runtime_test.go
git commit -m "feat(config): validate api and worker runtime requirements"
```

### Task 4: Create the ignored local environment safely

**Files:**
- Create: `deploy/docker-first/init-local-env.ps1`
- Create: `scripts/tests/init-local-env.tests.ps1`
- Modify: `deploy/docker-first/admin-go.env.example`
- Modify: `deploy/docker-first/README.md`

- [x] **Step 1: Write the script behavior test**

```powershell
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$script = Join-Path $root "deploy\docker-first\init-local-env.ps1"
$output = Join-Path $env:TEMP ("admin-go-env-test-" + [guid]::NewGuid() + ".env")
$dsn = "test_user:test_password@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local"
try {
  $log = & $script -OutputPath $output -MySQLDSN $dsn -RedisAddress "127.0.0.1:6379" -CorsOrigin "http://127.0.0.1:5173" 6>&1 | Out-String
  $text = Get-Content -Raw -LiteralPath $output
  if ($text -match "CHANGE_ME|DB_PRIVATE_IP|REDIS_PRIVATE_IP|FRONTEND_DOMAIN_REQUIRED") { throw "placeholder remains" }
  if ($text -notmatch "(?m)^APP_ENV=local$") { throw "local initializer must set APP_ENV=local" }
  $secretMatch = [regex]::Match($text, "(?m)^APP_SECRET=([^\r\n]{64,})$")
  if (!$secretMatch.Success) { throw "APP_SECRET is too short" }
  $firstSecret = $secretMatch.Groups[1].Value
  if ($log.Contains($dsn)) { throw "initializer leaked MYSQL_DSN" }

  $secondLog = & $script -OutputPath $output -MySQLDSN $dsn -RedisAddress "127.0.0.1:6379" -CorsOrigin "http://127.0.0.1:5173" 6>&1 | Out-String
  $secondText = Get-Content -Raw -LiteralPath $output
  $secondSecret = [regex]::Match($secondText, "(?m)^APP_SECRET=([^\r\n]{64,})$").Groups[1].Value
  if ($secondSecret -ne $firstSecret) { throw "initializer rotated APP_SECRET" }
  if ($secondLog.Contains($dsn)) { throw "initializer leaked MYSQL_DSN on rerun" }
} finally {
  Remove-Item -LiteralPath $output -Force -ErrorAction SilentlyContinue
}
```

- [x] **Step 2: Run and verify failure**

Run: `pwsh -NoProfile -File scripts/tests/init-local-env.tests.ps1`

Expected: FAIL because the initializer does not exist.

- [x] **Step 3: Implement the initializer**

Change the template CORS line to `CORS_ALLOW_ORIGINS=https://FRONTEND_DOMAIN_REQUIRED`, then implement:

```powershell
[CmdletBinding()]
param(
  [string]$OutputPath = (Join-Path $PSScriptRoot "admin-go.env"),
  [Parameter(Mandatory)][string]$MySQLDSN,
  [Parameter(Mandatory)][string]$RedisAddress,
  [Parameter(Mandatory)][string]$CorsOrigin
)
$ErrorActionPreference = "Stop"
$template = Join-Path $PSScriptRoot "admin-go.env.example"
if (!(Test-Path -LiteralPath $template)) { throw "env template not found: $template" }
if ($MySQLDSN -notmatch "/admin(?:\?|$)") { throw "MYSQL_DSN must select the admin database" }
if ($RedisAddress -notmatch "^[^:]+:\d+$") { throw "RedisAddress must be host:port" }
$origin = [uri]$CorsOrigin
if ($origin.Scheme -notin @("http", "https") -or !$origin.Host) { throw "CorsOrigin must be an HTTP(S) origin" }
$secret = $null
if (Test-Path -LiteralPath $OutputPath -PathType Leaf) {
  $existing = Get-Content -Raw -LiteralPath $OutputPath
  $existingSecret = [regex]::Match($existing, "(?m)^APP_SECRET=([^\r\n]{64,})$")
  if ($existingSecret.Success -and !$existingSecret.Groups[1].Value.Contains("CHANGE_ME")) {
    $secret = $existingSecret.Groups[1].Value
  }
}
if ([string]::IsNullOrEmpty($secret)) {
  $secretBytes = New-Object byte[] 48
  [Security.Cryptography.RandomNumberGenerator]::Fill($secretBytes)
  $secret = [Convert]::ToBase64String($secretBytes)
}
$text = Get-Content -Raw -LiteralPath $template
$text = $text.Replace("APP_ENV=production", "APP_ENV=local")
$text = $text.Replace("admin_user:CHANGE_ME@tcp(DB_PRIVATE_IP:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local", $MySQLDSN)
$text = $text.Replace("REDIS_PRIVATE_IP:6379", $RedisAddress)
$text = $text.Replace("CHANGE_ME_TO_64_PLUS_RANDOM_CHARS", $secret)
$text = $text.Replace("https://FRONTEND_DOMAIN_REQUIRED", $CorsOrigin)
[IO.File]::WriteAllText($OutputPath, $text, [Text.UTF8Encoding]::new($false))
Write-Output "created ignored runtime env at $OutputPath"
```

Keep `admin-go.env.example` production-oriented. The initializer changes only the generated local output to `APP_ENV=local`. On rerun, reuse an existing non-placeholder 64+ character `APP_SECRET`; generate a new 48-byte random Base64 secret only when no valid existing value is present. Never print either the existing or generated secret.

- [x] **Step 4: Test and create the real ignored env**

Run:

```powershell
pwsh -NoProfile -File scripts/tests/init-local-env.tests.ps1
pwsh -NoProfile -File deploy/docker-first/init-local-env.ps1 `
  -MySQLDSN $env:ADMIN_LOCAL_MYSQL_DSN `
  -RedisAddress $env:ADMIN_LOCAL_REDIS_ADDR `
  -CorsOrigin "http://127.0.0.1:5173"
git check-ignore deploy/docker-first/admin-go.env
```

Expected: test exits 0; initializer prints only the output path; `git check-ignore` prints `deploy/docker-first/admin-go.env`. The root agent supplies the already-authorized local credentials through process environment without logging them.

- [x] **Step 5: Commit only scripts/docs**

```powershell
git add -- deploy/docker-first/init-local-env.ps1 deploy/docker-first/admin-go.env.example deploy/docker-first/README.md scripts/tests/init-local-env.tests.ps1
git diff --cached --check
git commit -m "feat(dev): add secret-safe local env initializer"
```

Do not stage `deploy/docker-first/admin-go.env`.

### Task 5: Add repository-owned verification entrypoints

**Files:**
- Create: `scripts/verify-go-clean.ps1`
- Create: `scripts/verify-backend.ps1`
- Modify: `README.md`
- Modify: `internal/architecture/dependency_integrity_test.go`

- [x] **Step 1: Add a failing existence guard**

```go
func TestBackendVerificationEntrypointsExist(t *testing.T) {
	root := backendRoot(t)
	for _, name := range []string{"scripts/verify-go-clean.ps1", "scripts/verify-backend.ps1"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test ./internal/architecture -run TestBackendVerificationEntrypointsExist -count=1`

Expected: FAIL naming `scripts/verify-go-clean.ps1`.

- [x] **Step 3: Implement clean-cache verification**

```powershell
[CmdletBinding()]
param([switch]$KeepScratch)
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$scratch = Join-Path $env:TEMP ("admin-go-verify-" + [guid]::NewGuid())
$modCache = Join-Path $scratch "modcache"
$bin = Join-Path $scratch "bin"
New-Item -ItemType Directory -Path $modCache, $bin | Out-Null
$old = $env:GOMODCACHE
try {
  $env:GOMODCACHE = $modCache
  Push-Location $root
  go mod download
  go mod verify
  go test ./...
  go test -race ./internal/module/auth ./internal/module/payment/... ./internal/infra/taskqueue ./internal/infra/realtime/...
  go vet ./...
  go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
  go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
  go build -trimpath -o (Join-Path $bin "admin-api.exe") ./cmd/admin-api
  go build -trimpath -o (Join-Path $bin "admin-worker.exe") ./cmd/admin-worker
} finally {
  Pop-Location
  $env:GOMODCACHE = $old
  if (!$KeepScratch) {
    $resolvedScratch = [IO.Path]::GetFullPath($scratch)
    $resolvedTemp = [IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\'
    if (!$resolvedScratch.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase)) {
      throw "refusing to remove scratch path outside TEMP: $resolvedScratch"
    }
    Remove-Item -LiteralPath $resolvedScratch -Recurse -Force
  }
}
```

`scripts/verify-backend.ps1` runs the same test/vet/staticcheck/vulnerability/build commands with the normal module cache and outputs binaries under ignored `.tmp/verify-bin`.

- [x] **Step 4: Run both entrypoints**

Run:

```powershell
pwsh -NoProfile -File scripts/verify-backend.ps1
pwsh -NoProfile -File scripts/verify-go-clean.ps1
```

Expected: all tests/static checks/builds exit 0 and no binary is written into the repository root.

- [x] **Step 5: Commit**

```powershell
git add -- README.md internal/architecture/dependency_integrity_test.go scripts/verify-go-clean.ps1 scripts/verify-backend.ps1
git commit -m "build: add reproducible backend verification"
```

### Task 6: Make backend CI blocking and immutable

**Files:**
- Create: `.github/workflows/verify-backend.yml`
- Create: `.github/dependabot.yml`
- Modify: `Dockerfile`
- Modify: `internal/architecture/dependency_integrity_test.go`

- [x] **Step 1: Write the workflow guard**

```go
func TestBackendCIUsesRepositoryVerification(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/verify-backend.yml"))
	if err != nil { t.Fatal(err) }
	text := string(data)
	for _, required := range []string{
		"scripts/verify-go-clean.ps1",
		"docker/build-push-action",
		"push: false",
		"actions/upload-artifact",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("workflow missing %q", required)
		}
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test ./internal/architecture -run TestBackendCIUsesRepositoryVerification -count=1`

Expected: FAIL because the workflow does not exist.

- [x] **Step 3: Create the SHA-pinned workflow**

```yaml
name: Verify backend
on:
  pull_request:
  push:
    branches: [master]
permissions:
  contents: read
concurrency:
  group: backend-${{ github.ref }}
  cancel-in-progress: true
jobs:
  verify:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5
      - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff
        with:
          go-version: 1.26.5
          cache: false
      - name: Verify from clean module cache
        shell: pwsh
        run: ./scripts/verify-go-clean.ps1
      - uses: docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f
      - name: Build immutable image
        uses: docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8
        with:
          context: .
          push: false
          tags: admin-go-backend:${{ github.sha }}
          outputs: type=docker,dest=/tmp/admin-go-backend.tar
      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02
        with:
          name: admin-go-backend-${{ github.sha }}
          path: /tmp/admin-go-backend.tar
          if-no-files-found: error
          retention-days: 7
```

The action SHAs are the resolved `v4`/`v5`/`v6`/`v3` tag commits captured during planning; Dependabot proposes reviewed updates.

- [x] **Step 4: Harden the Docker build**

Add a test stage that runs `go test ./...` before binary compilation, preserve `GOSUMDB=sum.golang.org`, and add OCI labels for `org.opencontainers.image.revision` supplied through `BUILD_REVISION`. Do not add `GONOSUMDB`, `GOINSECURE`, or `GOSUMDB=off`.

- [x] **Step 5: Add dependency update policy**

Configure weekly updates for `gomod`, `docker`, and `github-actions`, each with a limit of five open PRs and no automatic merge.

- [x] **Step 6: Verify**

Run:

```powershell
go test ./internal/architecture -run 'TestBackendCI|TestAsynqmon' -count=1
docker build --build-arg BUILD_REVISION=$(git rev-parse HEAD) -t admin-go-backend:verify .
```

Expected: architecture tests pass. If Docker is unavailable locally, the protected GitHub workflow must pass before this task is complete.

- [x] **Step 7: Commit**

```powershell
git add -- .github/workflows/verify-backend.yml .github/dependabot.yml Dockerfile internal/architecture/dependency_integrity_test.go
git commit -m "ci: make backend verification blocking"
```

## Completion evidence (2026-07-17)

- Completion was re-audited at backend revision `d1843368f1f4921064c74e42a9cd5210239fd7ed`.
- `pwsh -NoProfile -File scripts/verify-go-clean.ps1` exited `0` from a unique empty `GOMODCACHE`: `go mod verify` reported `all modules verified`, the full repository test suite passed, and the shared runtime gate passed both ordinary and Linux `-race` execution.
- The same clean-cache run completed `go vet ./...`, `staticcheck@v0.8.0-rc.1 ./...`, and `govulncheck@v1.6.0 ./...`; govulncheck reported `0` called vulnerabilities.
- Clean-cache builds of both `./cmd/admin-api` and `./cmd/admin-worker` exited `0`. The Dockerfile test stage also ran `go test ./...` successfully while building `admin-go-backend:local`.
- Strict config, ignored local-env creation, Docker secret exclusion, pinned workflow/action inputs, immutable build metadata, and checksum replacement remain covered by the passing `internal/config` and `internal/architecture` suites. No runtime env or database credential is tracked.

## Plan completion gate

Run:

```powershell
pwsh -NoProfile -File scripts/tests/init-local-env.tests.ps1
pwsh -NoProfile -File scripts/verify-go-clean.ps1
git status --short
```

Expected: both scripts exit 0; status is clean; the ignored real `admin-go.env` exists and is not tracked; the protected GitHub workflow is green.
