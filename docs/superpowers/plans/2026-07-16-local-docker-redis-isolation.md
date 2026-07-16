# Local Docker Redis Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent, health-checked Docker Redis on host port `36379`, connect the backend containers to it, and start and verify the local backend and Vite frontend without disturbing FlyEnv Redis on `6379`.

**Architecture:** Redis joins the existing `admin-go-backend` Compose network and is reached by backend containers as `redis:6379`. Only host diagnostics use `127.0.0.1:36379`; P02 MySQL remains a separate container reached through `host.docker.internal:33306`, and Vite remains a host process on `5173`.

**Tech Stack:** Docker Compose, Redis 8.2.7 Alpine, Go 1.26.5 tests, PowerShell, Vite

---

## File map

- Create `internal/config/docker_compose_test.go`: executable contract for the local Redis service, port isolation, persistence, health check, and backend dependency ordering.
- Modify `deploy/docker-first/docker-compose.yml`: add Redis and wire API/worker startup to its health.
- Modify `deploy/docker-first/README.md`: document the new local Redis topology and verification commands.
- Replace ignored `deploy/docker-first/admin-go.env` through `init-local-env.ps1`: point the containers at P02 MySQL and Compose Redis without exposing credentials.

### Task 1: Add the failing Compose Redis contract

**Files:**
- Create: `internal/config/docker_compose_test.go`
- Test: `internal/config/docker_compose_test.go`

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDockerFirstComposeProvidesIsolatedRedis(t *testing.T) {
	type service struct {
		Image       string `yaml:"image"`
		Command     []string `yaml:"command"`
		Ports       []string `yaml:"ports"`
		Volumes     []string `yaml:"volumes"`
		Healthcheck struct {
			Test []string `yaml:"test"`
		} `yaml:"healthcheck"`
		DependsOn map[string]struct {
			Condition string `yaml:"condition"`
		} `yaml:"depends_on"`
	}
	var compose struct {
		Services map[string]service `yaml:"services"`
		Volumes  map[string]any `yaml:"volumes"`
	}

	content, err := os.ReadFile(filepath.Join("..", "..", "deploy", "docker-first", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(content, &compose); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}

	redis, ok := compose.Services["redis"]
	if !ok {
		t.Fatal("docker-compose.yml must define redis")
	}
	if redis.Image != "docker.m.daocloud.io/library/redis:8.2.7-alpine" {
		t.Fatalf("unexpected Redis image %q", redis.Image)
	}
	if !reflect.DeepEqual(redis.Ports, []string{"127.0.0.1:36379:6379"}) {
		t.Fatalf("Redis must bind only to isolated loopback port 36379, got %v", redis.Ports)
	}
	if !reflect.DeepEqual(redis.Command, []string{"redis-server", "--appendonly", "yes"}) {
		t.Fatalf("Redis must enable AOF persistence, got %v", redis.Command)
	}
	if !reflect.DeepEqual(redis.Volumes, []string{"redis-data:/data"}) {
		t.Fatalf("Redis must use the named data volume, got %v", redis.Volumes)
	}
	if !reflect.DeepEqual(redis.Healthcheck.Test, []string{"CMD", "redis-cli", "ping"}) {
		t.Fatalf("Redis must expose a PING health check, got %v", redis.Healthcheck.Test)
	}
	if _, ok := compose.Volumes["redis-data"]; !ok {
		t.Fatal("docker-compose.yml must declare redis-data")
	}
	for _, name := range []string{"admin-api", "admin-worker"} {
		dependency, ok := compose.Services[name].DependsOn["redis"]
		if !ok || dependency.Condition != "service_healthy" {
			t.Fatalf("%s must wait for healthy Redis", name)
		}
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```powershell
go test ./internal/config -run TestDockerFirstComposeProvidesIsolatedRedis -count=1
```

Expected: `FAIL` with `docker-compose.yml must define redis`.

### Task 2: Add Redis to the local Compose topology

**Files:**
- Modify: `deploy/docker-first/docker-compose.yml`
- Test: `internal/config/docker_compose_test.go`

- [ ] **Step 1: Add the Redis service before `admin-api`**

```yaml
  redis:
    image: docker.m.daocloud.io/library/redis:8.2.7-alpine
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes"]
    ports:
      - "127.0.0.1:36379:6379"
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s
```

- [ ] **Step 2: Make both backend services wait for Redis**

Add to `admin-api`:

```yaml
    depends_on:
      redis:
        condition: service_healthy
```

Keep the existing `admin-api` dependency under `admin-worker` and add:

```yaml
      redis:
        condition: service_healthy
```

- [ ] **Step 3: Declare the data volume**

```yaml
volumes:
  redis-data:
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```powershell
go test ./internal/config -run 'TestDockerFirstCompose(ProvidesIsolatedRedis|UsesDeveloperDefaultsWithoutProjectEnvFile)' -count=1
```

Expected: `ok admin-go/internal/config`.

- [ ] **Step 5: Validate the rendered Compose model**

Run from `deploy/docker-first`:

```powershell
docker compose config --quiet
```

Expected: exit code `0` with no validation errors.

- [ ] **Step 6: Commit the Compose change**

```powershell
git add -- internal/config/docker_compose_test.go deploy/docker-first/docker-compose.yml
git commit -m "feat(dev): add isolated Docker Redis"
```

### Task 3: Align local documentation and secret runtime env

**Files:**
- Modify: `deploy/docker-first/README.md`
- Runtime only: `deploy/docker-first/admin-go.env` (ignored; never stage or print)
- Test: `scripts/tests/init-local-env.tests.ps1`

- [ ] **Step 1: Update the README topology**

Document these exact local values:

```text
Redis host access: 127.0.0.1:36379
Redis container access: redis:6379
P02 MySQL host access: 127.0.0.1:33306
P02 MySQL container access: host.docker.internal:33306
```

Replace the statement that Compose does not create Redis. Keep the production-oriented `admin-go.env.example` unchanged.

- [ ] **Step 2: Run the initializer regression suite**

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/tests/init-local-env.tests.ps1
```

Expected: `init-local-env assertions passed`.

- [ ] **Step 3: Generate the ignored runtime env without printing secrets**

```powershell
$state = Get-Content -Raw "$env:TEMP\admin-p02-workspace\state.json" | ConvertFrom-Json
$dsn = "root:$($state.root_password)@tcp(host.docker.internal:33306)/admin?charset=utf8mb4&parseTime=True&loc=Local"
deploy\docker-first\init-local-env.ps1 `
  -MySQLDSN $dsn `
  -RedisAddress 'redis:6379' `
  -CorsOrigin 'http://localhost:5173'
$dsn = $null
$state = $null
```

Expected: `created ignored runtime env`; do not display the file.

- [ ] **Step 4: Commit documentation only**

```powershell
git add -- deploy/docker-first/README.md
git commit -m "docs(dev): document isolated Docker Redis"
```

### Task 4: Start and verify the complete local stack

**Files:**
- No tracked file changes
- Logs: `%TEMP%\admin-front-vite-5173.out.log`, `%TEMP%\admin-front-vite-5173.err.log`

- [ ] **Step 1: Start Redis and backend containers**

Run from `deploy/docker-first`:

```powershell
docker compose up -d --build
docker compose ps
```

Expected: `redis` is `healthy`, and `admin-api` plus `admin-worker` are running.

- [ ] **Step 2: Start Vite as a hidden host process**

Run from `E:\admin\admin_front_ts`:

```powershell
Start-Process -FilePath 'npm.cmd' `
  -ArgumentList @('run', 'dev', '--', '--host', 'localhost', '--port', '5173') `
  -WorkingDirectory 'E:\admin\admin_front_ts' `
  -RedirectStandardOutput "$env:TEMP\admin-front-vite-5173.out.log" `
  -RedirectStandardError "$env:TEMP\admin-front-vite-5173.err.log" `
  -WindowStyle Hidden
```

- [ ] **Step 3: Verify port isolation and Redis health**

```powershell
Get-NetTCPConnection -State Listen | Where-Object LocalPort -in 5173,6379,36379,8080,33306
docker compose exec -T redis redis-cli ping
```

Expected: both `6379` and `36379` listen independently, and Redis returns `PONG`.

- [ ] **Step 4: Verify backend and frontend HTTP paths**

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health
Invoke-RestMethod http://127.0.0.1:8080/ready
Invoke-WebRequest http://localhost:5173 -UseBasicParsing
Invoke-RestMethod http://127.0.0.1:8080/api/admin/v1/auth/login-config
```

Expected: health and readiness succeed, Vite returns HTTP `200`, and login configuration returns a successful API response.

- [ ] **Step 5: Run tracked-file verification**

```powershell
go test ./internal/config ./internal/architecture -count=1
git diff --check
git status --short --branch
```

Expected: tests pass, no whitespace errors, and only the planned commits are ahead of `origin/master`.
