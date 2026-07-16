# Full Docker Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run frontend, API, worker, MySQL, and Redis as five containers split between `admin-app` and `admin-state`, while retaining the current P02 MySQL for rollback.

**Architecture:** `admin-state` owns MySQL, Redis, named volumes, and the shared `admin-platform` network. `admin-app` owns Nginx frontend, API, and worker. A verified logical restore is loaded into a new MySQL volume on `33307` before the final switch to `33306`.

**Tech Stack:** Docker Compose, MySQL 8.4.10, Redis 8.2.7, Node 22.23.1, Vite 8, nginx-unprivileged 1.31.3, Go 1.26.5, PowerShell 5.1

---

## File map

- Create `E:/admin/admin_front_ts/{Dockerfile,.dockerignore,deploy/nginx.conf}` for the frontend image and runtime.
- Create `E:/admin/admin_front_ts/tests/shared/deployment/docker-container.test.ts` for its deployment contract.
- Create `deploy/docker-state/{docker-compose.yml,admin-state.env.example}` for state services.
- Modify `deploy/docker-first/docker-compose.yml` into the `admin-app` project.
- Modify `internal/config/docker_compose_test.go` to lock both Compose contracts.
- Create `scripts/{docker-platform.ps1,tests/docker-platform.tests.ps1}` for safe lifecycle orchestration.
- Create only ignored runtime secret `deploy/docker-state/runtime/mysql-root-password.txt`.

### Task 1: Frontend container contract and runtime

**Files:**
- Create: `E:/admin/admin_front_ts/tests/shared/deployment/docker-container.test.ts`
- Create: `E:/admin/admin_front_ts/Dockerfile`
- Create: `E:/admin/admin_front_ts/.dockerignore`
- Create: `E:/admin/admin_front_ts/deploy/nginx.conf`
- Modify: `E:/admin/admin_front_ts/README.md`

- [ ] **Step 1: Write the failing Vitest contract**

```ts
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const read = (path: string) => readFileSync(resolve(process.cwd(), path), 'utf8')

describe('frontend Docker runtime', () => {
  it('uses pinned build and unprivileged runtime images', () => {
    const dockerfile = read('Dockerfile')
    expect(dockerfile).toContain('node:22.23.1-alpine')
    expect(dockerfile).toContain('nginxinc/nginx-unprivileged:1.31.3-alpine')
    expect(dockerfile).toContain('npm ci')
    expect(dockerfile).toContain('npm run build:check')
    expect(dockerfile).toContain('HEALTHCHECK')
  })

  it('proxies API and WebSocket traffic and keeps Vue fallback', () => {
    const nginx = read('deploy/nginx.conf')
    expect(nginx).toContain('listen 8080')
    expect(nginx).toContain('location = /healthz')
    expect(nginx).toContain('proxy_pass http://admin-api:8080')
    expect(nginx).toContain('proxy_set_header Upgrade $http_upgrade')
    expect(nginx).toContain('try_files $uri $uri/ /index.html')
  })
})
```

- [ ] **Step 2: Verify RED**

```powershell
npm.cmd exec vitest run tests/shared/deployment/docker-container.test.ts
```

Expected: fail because `Dockerfile` and `deploy/nginx.conf` are missing.

- [ ] **Step 3: Create the multi-stage Dockerfile**

```dockerfile
ARG NODE_IMAGE=node:22.23.1-alpine
ARG NGINX_IMAGE=nginxinc/nginx-unprivileged:1.31.3-alpine
FROM ${NODE_IMAGE} AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG VITE_GO_API_BASE_URL=http://localhost:5173
ARG VITE_WEB_SOCKET_URL=ws://localhost:5173/api/admin/v1/realtime/ws
ARG VITE_PLATFORM=admin
ENV VITE_GO_API_BASE_URL=${VITE_GO_API_BASE_URL}
ENV VITE_WEB_SOCKET_URL=${VITE_WEB_SOCKET_URL}
ENV VITE_PLATFORM=${VITE_PLATFORM}
RUN npm run build:check
FROM ${NGINX_IMAGE} AS runtime
COPY --chown=101:101 deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --chown=101:101 --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --retries=5 --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
```

- [ ] **Step 4: Create Nginx configuration**

```nginx
map $http_upgrade $connection_upgrade { default upgrade; '' close; }
server {
    listen 8080;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
    location = /healthz { access_log off; default_type text/plain; return 200 'ok'; }
    location /api/ {
        proxy_pass http://admin-api:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
    location / { try_files $uri $uri/ /index.html; }
}
```

- [ ] **Step 5: Create `.dockerignore`, document the image, and verify GREEN**

The ignore file contains `.git`, `.github`, `.vscode`, `.worktrees`, `node_modules`, `dist`, `src-tauri/target`, and `*.log`.

```powershell
npm.cmd exec vitest run tests/shared/deployment/docker-container.test.ts
npm.cmd run lint
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
```

Expected: all exit `0`.

- [ ] **Step 6: Commit frontend assets**

```powershell
git add -- Dockerfile .dockerignore deploy/nginx.conf tests/shared/deployment/docker-container.test.ts README.md
git commit -m "feat(deploy): containerize admin frontend"
```

### Task 2: App/state Compose contracts

**Files:**
- Modify: `internal/config/docker_compose_test.go`
- Test: `internal/config/docker_compose_test.go`

- [ ] **Step 1: Replace the current single-project Redis test**

```go
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

type composeService struct {
	Image string `yaml:"image"`
	Build struct {
		Context string `yaml:"context"`
	} `yaml:"build"`
	Ports    []string `yaml:"ports"`
	Volumes  []string `yaml:"volumes"`
	Networks []string `yaml:"networks"`
}

type composeContract struct {
	Name     string                    `yaml:"name"`
	Services map[string]composeService `yaml:"services"`
	Networks map[string]struct {
		Name     string `yaml:"name"`
		External bool   `yaml:"external"`
	} `yaml:"networks"`
}

func readComposeContract(t *testing.T, parts ...string) composeContract {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(parts...))
	if err != nil { t.Fatal(err) }
	var contract composeContract
	if err := yaml.Unmarshal(content, &contract); err != nil { t.Fatal(err) }
	return contract
}

func TestDockerStateComposeOwnsStateServices(t *testing.T) {
	c := readComposeContract(t, "..", "..", "deploy", "docker-state", "docker-compose.yml")
	if c.Name != "admin-state" { t.Fatalf("name=%q", c.Name) }
	if len(c.Services) != 2 { t.Fatalf("services=%v", c.Services) }
	mysql, redis := c.Services["mysql"], c.Services["redis"]
	if mysql.Image != "mysql:8.4.10" || !reflect.DeepEqual(mysql.Ports, []string{"127.0.0.1:${ADMIN_MYSQL_HOST_PORT:-33306}:3306"}) || !reflect.DeepEqual(mysql.Volumes, []string{"mysql-data:/var/lib/mysql"}) { t.Fatal("invalid MySQL contract") }
	if redis.Image != "redis:8.2.7-alpine" || !reflect.DeepEqual(redis.Ports, []string{"127.0.0.1:${ADMIN_REDIS_HOST_PORT:-36379}:6379"}) || !reflect.DeepEqual(redis.Volumes, []string{"redis-data:/data"}) { t.Fatal("invalid Redis contract") }
	if c.Networks["platform"].Name != "admin-platform" || c.Networks["platform"].External { t.Fatal("state must own admin-platform") }
}

func TestDockerAppComposeOwnsOnlyApplicationServices(t *testing.T) {
	c := readComposeContract(t, "..", "..", "deploy", "docker-first", "docker-compose.yml")
	if c.Name != "admin-app" { t.Fatalf("name=%q", c.Name) }
	for _, name := range []string{"frontend", "admin-api", "admin-worker"} { if _, ok := c.Services[name]; !ok { t.Fatalf("missing %s", name) } }
	for _, name := range []string{"mysql", "redis"} { if _, ok := c.Services[name]; ok { t.Fatalf("app owns %s", name) } }
	frontend := c.Services["frontend"]
	if frontend.Build.Context != "../../../admin_front_ts" || !reflect.DeepEqual(frontend.Ports, []string{"127.0.0.1:5173:8080"}) { t.Fatal("invalid frontend contract") }
	if !c.Networks["platform"].External || c.Networks["platform"].Name != "admin-platform" { t.Fatal("app must consume external admin-platform") }
}
```

- [ ] **Step 2: Verify RED**

```powershell
go test ./internal/config -run 'TestDocker(State|App)Compose' -count=1
```

Expected: fail because state Compose is absent and app Compose still owns Redis.

### Task 3: Implement state and application Compose projects

**Files:**
- Create: `deploy/docker-state/docker-compose.yml`
- Create: `deploy/docker-state/admin-state.env.example`
- Modify: `deploy/docker-first/docker-compose.yml`
- Modify: `deploy/docker-first/README.md`
- Modify: `.gitignore`

- [ ] **Step 1: Create `admin-state` Compose**

```yaml
name: admin-state
services:
  mysql:
    image: mysql:8.4.10
    restart: unless-stopped
    environment:
      MYSQL_DATABASE: admin
      MYSQL_ROOT_PASSWORD_FILE: /run/secrets/mysql_root_password
    secrets: [mysql_root_password]
    ports: ["127.0.0.1:${ADMIN_MYSQL_HOST_PORT:-33306}:3306"]
    volumes: ["mysql-data:/var/lib/mysql"]
    networks: [platform]
    healthcheck:
      test: ["CMD-SHELL", "MYSQL_PWD=$$(cat /run/secrets/mysql_root_password) mysqladmin ping --host=127.0.0.1 --user=root --silent"]
      interval: 5s
      timeout: 5s
      retries: 30
      start_period: 30s
  redis:
    image: redis:8.2.7-alpine
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes"]
    ports: ["127.0.0.1:${ADMIN_REDIS_HOST_PORT:-36379}:6379"]
    volumes: ["redis-data:/data"]
    networks: [platform]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s
networks:
  platform: {name: admin-platform}
volumes:
  mysql-data:
  redis-data:
secrets:
  mysql_root_password: {file: ./runtime/mysql-root-password.txt}
```

- [ ] **Step 2: Convert `docker-first` to `admin-app`**

Remove Redis and `redis-data`, set `name: admin-app`, attach API/worker to `platform`, and add:

```yaml
  frontend:
    image: admin-frontend:local
    build:
      context: ../../../admin_front_ts
      dockerfile: Dockerfile
      args:
        NODE_IMAGE: node:22.23.1-alpine
        NGINX_IMAGE: nginxinc/nginx-unprivileged:1.31.3-alpine
        VITE_GO_API_BASE_URL: http://localhost:5173
        VITE_WEB_SOCKET_URL: ws://localhost:5173/api/admin/v1/realtime/ws
        VITE_PLATFORM: admin
    restart: unless-stopped
    ports: ["127.0.0.1:5173:8080"]
    networks: [platform]
    depends_on:
      admin-api: {condition: service_healthy}
networks:
  platform: {external: true, name: admin-platform}
```

- [ ] **Step 3: Ignore state secrets and verify GREEN**

Add `deploy/docker-state/admin-state.env` and `deploy/docker-state/runtime/` to `.gitignore`.

```powershell
go test ./internal/config -run 'TestDocker(State|App)Compose' -count=1
docker compose -f deploy/docker-state/docker-compose.yml config --quiet
docker compose -f deploy/docker-first/docker-compose.yml config --quiet
```

- [ ] **Step 4: Commit backend Compose assets**

```powershell
git add -- internal/config/docker_compose_test.go deploy/docker-state deploy/docker-first/docker-compose.yml deploy/docker-first/README.md .gitignore
git commit -m "feat(deploy): split app and state Compose projects"
```

### Task 4: Safe lifecycle orchestration

**Files:**
- Create: `scripts/docker-platform.ps1`
- Create: `scripts/tests/docker-platform.tests.ps1`

- [ ] **Step 1: Write and run a failing PowerShell contract test**

```powershell
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$path=Join-Path $root 'scripts\docker-platform.ps1'
if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'docker-platform.ps1 is missing' }
$content=[IO.File]::ReadAllText($path)
foreach ($required in @("ValidateSet('init','up','stop','status')",'SetAccessRuleProtection($true,$false)','mysql:3306','redis:6379',"'--wait'")) {
  if (-not $content.Contains($required)) { throw "missing contract: $required" }
}
if ($content -match '(?i)down\s+-v|--volumes') { throw 'lifecycle script must not delete volumes' }
$stateUp=$content.IndexOf('$stateCompose,''up''',[StringComparison]::Ordinal)
$appUp=$content.IndexOf('$appCompose,''up''',[StringComparison]::Ordinal)
if ($stateUp -lt 0 -or $appUp -le $stateUp) { throw 'state must start before app' }
Write-Output 'docker-platform assertions passed'
```

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/tests/docker-platform.tests.ps1
```

Expected: fail because `scripts/docker-platform.ps1` is absent.

- [ ] **Step 2: Implement the lifecycle script**

```powershell
[CmdletBinding()]
param([Parameter(Mandatory=$true)][ValidateSet('init','up','stop','status')][string]$Action)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$stateCompose=Join-Path $root 'deploy\docker-state\docker-compose.yml'
$appCompose=Join-Path $root 'deploy\docker-first\docker-compose.yml'
$dockerBin='E:\Docker\Docker\resources\bin'
$docker=Join-Path $dockerBin 'docker.exe'
if (-not (Test-Path -LiteralPath $docker)) { $docker=(Get-Command docker.exe -ErrorAction Stop).Source }
$env:Path=(Split-Path $docker -Parent)+[IO.Path]::PathSeparator+$env:Path

function Invoke-Docker([string[]]$Arguments) {
  & $docker @Arguments
  if ($LASTEXITCODE -ne 0) { throw "docker exited $LASTEXITCODE" }
}

function Write-OwnerOnlySecret([string]$Path,[string]$Value) {
  $directory=Split-Path $Path -Parent
  [IO.Directory]::CreateDirectory($directory) | Out-Null
  $temporary=$Path+'.'+[guid]::NewGuid().ToString('N')+'.tmp'
  try {
    [IO.File]::WriteAllText($temporary,$Value+"`n",[Text.UTF8Encoding]::new($false))
    $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl=[Security.AccessControl.FileSecurity]::new()
    $acl.SetOwner($sid)
    $acl.SetAccessRuleProtection($true,$false)
    $acl.SetAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,[Security.AccessControl.FileSystemRights]::FullControl,[Security.AccessControl.AccessControlType]::Allow))
    Set-Acl -LiteralPath $temporary -AclObject $acl
    [IO.File]::Move($temporary,$Path,$true)
  } finally { Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue }
}

switch ($Action) {
  'init' {
    $statePath=Join-Path $env:TEMP 'admin-p02-workspace\state.json'
    $state=Get-Content -Raw -LiteralPath $statePath | ConvertFrom-Json
    $password=[string]$state.root_password
    if ($password -notmatch '^[A-Za-z0-9._~-]+$') { throw 'P02 root password is not Compose-safe' }
    Write-OwnerOnlySecret (Join-Path $root 'deploy\docker-state\runtime\mysql-root-password.txt') $password
    $dsn='root:'+$password+'@tcp(mysql:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local'
    & (Join-Path $root 'deploy\docker-first\init-local-env.ps1') -MySQLDSN $dsn -RedisAddress 'redis:6379' -CorsOrigin 'http://localhost:5173'
    $dsn=$null; $password=$null; $state=$null
  }
  'up' {
    Invoke-Docker @('compose','-f',$stateCompose,'up','-d','--wait','--wait-timeout','180')
    Invoke-Docker @('compose','-f',$appCompose,'up','-d','--build','--wait','--wait-timeout','300')
  }
  'stop' {
    Invoke-Docker @('compose','-f',$appCompose,'stop')
    Invoke-Docker @('compose','-f',$stateCompose,'stop')
  }
  'status' {
    Invoke-Docker @('compose','-f',$stateCompose,'ps')
    Invoke-Docker @('compose','-f',$appCompose,'ps')
  }
}
```

- [ ] **Step 3: Verify and commit**

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/tests/docker-platform.tests.ps1
git add -- scripts/docker-platform.ps1 scripts/tests/docker-platform.tests.ps1
git commit -m "feat(deploy): orchestrate full Docker platform"
```

### Task 5: Restore new MySQL on port 33307

**Files:** Runtime only; no tracked changes

- [ ] **Step 1: Initialize secrets**

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/docker-platform.ps1 init
```

- [ ] **Step 2: Start new MySQL on temporary port**

```powershell
$env:ADMIN_MYSQL_HOST_PORT='33307'
docker compose -f deploy/docker-state/docker-compose.yml up -d mysql --wait
Remove-Item Env:ADMIN_MYSQL_HOST_PORT
```

- [ ] **Step 3: Verify and restore the recovery dump**

Read `state.json`, require `artifact.verified=true`, compare the dump SHA-256, copy `admin.sql` to the new MySQL container, and run inside it:

```sh
MYSQL_PWD="$(cat /run/secrets/mysql_root_password)" mysql --user=root --database=admin < /tmp/admin.sql
```

Delete only `/tmp/admin.sql` after a successful import.

- [ ] **Step 4: Compare restored database acceptance evidence**

Set a process-only `MYSQL_DSN` for `127.0.0.1:33307`, run `scripts/database/capture-baseline.ps1`, and compare `schema_sha256` plus every `exact_counts` entry with `%TEMP%/admin-p02-workspace/baseline.json`.

Expected: all comparisons match before cutover.

### Task 6: Cut over and verify all five containers

**Files:** No tracked changes

- [ ] **Step 1: Stop host Vite safely**

Resolve the owner PID of `5173` and stop it only if `Win32_Process.CommandLine` contains both `admin_front_ts` and `vite`.

- [ ] **Step 2: Stop old containers without deleting volumes**

```powershell
docker compose -p admin-go-backend -f deploy/docker-first/docker-compose.yml down --remove-orphans
docker stop admin-p02-mysql
```

Never add `-v`; never remove `admin-p02-mysql`.

- [ ] **Step 3: Recreate state on final ports and start app**

```powershell
docker compose -f deploy/docker-state/docker-compose.yml up -d --force-recreate --wait
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/docker-platform.ps1 up
```

- [ ] **Step 4: Run full verification**

Backend:

```powershell
go test ./... -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/tests/init-local-env.tests.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/tests/docker-platform.tests.ps1
```

Frontend:

```powershell
npm.cmd run lint
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
```

Require five running containers; healthy MySQL, Redis, API, and frontend; worker restart count zero; Redis `PONG`; MySQL `admin` login on `33306`; and HTTP 200 for frontend root, Vue fallback, frontend-proxied login config, `/health`, and `/ready`.

- [ ] **Step 5: Confirm rollback assets and clean repositories**

Confirm `admin-p02-mysql` remains stopped but present, its anonymous volume/state/recovery artifact remain, and the old Redis volume remains. Run `git diff --check` and `git status --short --branch` in both repositories.
