# Admin Foundation Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make backend configuration, dependency verification, local environment initialization, builds, and CI deterministic before database or architecture changes begin.

**Architecture:** Keep configuration as explicit data parsed from the process environment and validated for either `admin-api` or `admin-worker`. Put developer/CI verification behind repository-owned PowerShell entrypoints, then make GitHub Actions call the same entrypoints from a clean module cache.

**Tech Stack:** Go 1.26.1, PowerShell 7, GitHub Actions, Docker BuildKit, MySQL/Redis runtime configuration.

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

- [ ] **Step 1: Write the failing checksum guard**

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

- [ ] **Step 2: Run the guard and confirm the repository is wrong**

Run:

```powershell
go test ./internal/architecture -run TestAsynqmonChecksumMatchesTransparencyLog -count=1
```

Expected: FAIL with `go.sum must contain verified sum` or Go's checksum mismatch showing the repository value `EfLR...`.

- [ ] **Step 3: Verify the public sum and change one line**

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

- [ ] **Step 4: Verify from a unique empty module cache**

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

- [ ] **Step 5: Commit**

```powershell
git add -- go.sum internal/architecture/dependency_integrity_test.go
git commit -m "fix(build): restore verified asynqmon checksum"
```

### Task 2: Add strict environment parsing

**Files:**
- Create: `internal/config/env.go`
- Create: `internal/config/env_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write table-driven failures for malformed values**

```go
func TestLoadRejectsMalformedEnvironment(t *testing.T) {
	tests := []struct {
		key, value, want string
	}{
		{"MYSQL_MAX_OPEN_CONNS", "many", "MYSQL_MAX_OPEN_CONNS: parse integer"},
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
				t.Fatalf("Load() error=%v, want substring %q", err, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and verify the old loader silently falls back**

Run:

```powershell
go test ./internal/config -run TestLoadRejectsMalformedEnvironment -count=1
```

Expected: FAIL because `Load` returns only `Config` and invalid values currently fall back.

- [ ] **Step 3: Implement strict parsers**

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

- [ ] **Step 4: Change the loader signature and propagate parser errors**

Add a `Process` enum and change the signature to:

```go
type Process string

const (
	ProcessAPI    Process = "admin-api"
	ProcessWorker Process = "admin-worker"
)

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

`loadFrom` calls `envInteger` for `MYSQL_MAX_OPEN_CONNS`, `MYSQL_MAX_IDLE_CONNS`, `REDIS_DB`, `TOKEN_REDIS_DB`, `QUEUE_REDIS_DB`, and `QUEUE_CONCURRENCY`; `envBoolean` for `QUEUE_ENABLED`, `REALTIME_ENABLED`, and `SCHEDULER_ENABLED`; and `envPeriod` for `HTTP_READ_HEADER_TIMEOUT` and `MYSQL_CONN_MAX_LIFETIME`. Open connections, queue concurrency, and both durations are positive; idle connections and Redis DB numbers allow zero but reject negatives. Keep only defaults already represented by constants. Do not retain `envInt`, `envBool`, or `envDuration` fallback-on-error behavior.

- [ ] **Step 5: Update existing tests to handle `(Config, error)`**

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

Replace direct `cfg := Load()` calls with `cfg := loadForTest(t, ProcessAPI)`.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/config -count=1`

Expected: PASS, including malformed integer/duration/boolean cases.

- [ ] **Step 7: Commit**

```powershell
git add -- internal/config/config.go internal/config/config_test.go internal/config/env.go internal/config/env_test.go
git commit -m "refactor(config): reject malformed runtime settings"
```

### Task 3: Enforce process-specific runtime configuration

**Files:**
- Create: `internal/config/runtime.go`
- Create: `internal/config/runtime_test.go`
- Modify: `cmd/admin-api/main.go`
- Modify: `cmd/admin-worker/main.go`
- Modify: `internal/config/config.go`

- [ ] **Step 1: Write production and process validation tests**

```go
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

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/config -run 'TestValidate' -count=1`

Expected: FAIL because `Validate` does not exist.

- [ ] **Step 3: Implement process validation**

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

- [ ] **Step 4: Fail both processes before resource construction**

Use in both mains:

```go
cfg, err := config.Load(config.ProcessAPI) // ProcessWorker in admin-worker
if err != nil {
	logger.Error("invalid runtime configuration", "error", err)
	os.Exit(1)
}
```

Do not log `cfg` or an environment value.

- [ ] **Step 5: Verify**

Run:

```powershell
go test ./internal/config ./cmd/admin-api ./cmd/admin-worker -count=1
go build -o $env:TEMP\admin-api-foundation.exe ./cmd/admin-api
go build -o $env:TEMP\admin-worker-foundation.exe ./cmd/admin-worker
```

Expected: tests pass and both binaries build.

- [ ] **Step 6: Commit**

```powershell
git add -- cmd/admin-api/main.go cmd/admin-worker/main.go internal/config/config.go internal/config/runtime.go internal/config/runtime_test.go
git commit -m "feat(config): validate api and worker runtime requirements"
```

### Task 4: Create the ignored local environment safely

**Files:**
- Create: `deploy/docker-first/init-local-env.ps1`
- Create: `scripts/tests/init-local-env.tests.ps1`
- Modify: `deploy/docker-first/admin-go.env.example`
- Modify: `deploy/docker-first/README.md`

- [ ] **Step 1: Write the script behavior test**

```powershell
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$script = Join-Path $root "deploy\docker-first\init-local-env.ps1"
$output = Join-Path $env:TEMP ("admin-go-env-test-" + [guid]::NewGuid() + ".env")
$dsn = "test_user:test_password@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local"
try {
  $log = & $script -OutputPath $output -MySQLDSN $dsn -RedisAddress "127.0.0.1:6379" -CorsOrigin "http://127.0.0.1:5173" 6>&1 | Out-String
  $text = Get-Content -Raw -LiteralPath $output
  if ($text -match "CHANGE_ME|DB_PRIVATE_IP|REDIS_PRIVATE_IP|FRONTEND_DOMAIN_REQUIRED") { throw "placeholder remains" }
  if ($text -notmatch "(?m)^APP_SECRET=.{64,}$") { throw "APP_SECRET is too short" }
  if ($log.Contains($dsn)) { throw "initializer leaked MYSQL_DSN" }
} finally {
  Remove-Item -LiteralPath $output -Force -ErrorAction SilentlyContinue
}
```

- [ ] **Step 2: Run and verify failure**

Run: `pwsh -NoProfile -File scripts/tests/init-local-env.tests.ps1`

Expected: FAIL because the initializer does not exist.

- [ ] **Step 3: Implement the initializer**

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
$secretBytes = New-Object byte[] 48
[Security.Cryptography.RandomNumberGenerator]::Fill($secretBytes)
$secret = [Convert]::ToBase64String($secretBytes)
$text = Get-Content -Raw -LiteralPath $template
$text = $text.Replace("admin_user:CHANGE_ME@tcp(DB_PRIVATE_IP:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local", $MySQLDSN)
$text = $text.Replace("REDIS_PRIVATE_IP:6379", $RedisAddress)
$text = $text.Replace("CHANGE_ME_TO_64_PLUS_RANDOM_CHARS", $secret)
$text = $text.Replace("https://FRONTEND_DOMAIN_REQUIRED", $CorsOrigin)
[IO.File]::WriteAllText($OutputPath, $text, [Text.UTF8Encoding]::new($false))
Write-Output "created ignored runtime env at $OutputPath"
```

- [ ] **Step 4: Test and create the real ignored env**

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

- [ ] **Step 5: Commit only scripts/docs**

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

- [ ] **Step 1: Add a failing existence guard**

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

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/architecture -run TestBackendVerificationEntrypointsExist -count=1`

Expected: FAIL naming `scripts/verify-go-clean.ps1`.

- [ ] **Step 3: Implement clean-cache verification**

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

- [ ] **Step 4: Run both entrypoints**

Run:

```powershell
pwsh -NoProfile -File scripts/verify-backend.ps1
pwsh -NoProfile -File scripts/verify-go-clean.ps1
```

Expected: all tests/static checks/builds exit 0 and no binary is written into the repository root.

- [ ] **Step 5: Commit**

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

- [ ] **Step 1: Write the workflow guard**

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

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/architecture -run TestBackendCIUsesRepositoryVerification -count=1`

Expected: FAIL because the workflow does not exist.

- [ ] **Step 3: Create the SHA-pinned workflow**

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
          go-version: 1.26.1
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

- [ ] **Step 4: Harden the Docker build**

Add a test stage that runs `go test ./...` before binary compilation, preserve `GOSUMDB=sum.golang.org`, and add OCI labels for `org.opencontainers.image.revision` supplied through `BUILD_REVISION`. Do not add `GONOSUMDB`, `GOINSECURE`, or `GOSUMDB=off`.

- [ ] **Step 5: Add dependency update policy**

Configure weekly updates for `gomod`, `docker`, and `github-actions`, each with a limit of five open PRs and no automatic merge.

- [ ] **Step 6: Verify**

Run:

```powershell
go test ./internal/architecture -run 'TestBackendCI|TestAsynqmon' -count=1
docker build --build-arg BUILD_REVISION=$(git rev-parse HEAD) -t admin-go-backend:verify .
```

Expected: architecture tests pass. If Docker is unavailable locally, the protected GitHub workflow must pass before this task is complete.

- [ ] **Step 7: Commit**

```powershell
git add -- .github/workflows/verify-backend.yml .github/dependabot.yml Dockerfile internal/architecture/dependency_integrity_test.go
git commit -m "ci: make backend verification blocking"
```

## Plan completion gate

Run:

```powershell
pwsh -NoProfile -File scripts/tests/init-local-env.tests.ps1
pwsh -NoProfile -File scripts/verify-go-clean.ps1
git status --short
```

Expected: both scripts exit 0; status is clean; the ignored real `admin-go.env` exists and is not tracked; the protected GitHub workflow is green.
