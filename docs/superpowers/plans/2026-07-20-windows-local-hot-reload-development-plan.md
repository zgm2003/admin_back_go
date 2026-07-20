# Windows Local Hot-Reload Development Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task directly on the existing `master` checkouts. Do not create a worktree or use browser automation. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add a one-command Windows development loop where MySQL/Redis remain in Docker and Vite/API/Worker hot-reload on the host, while preserving full-Docker production delivery.

**Architecture:** `scripts/admin-dev.ps1` supervises three host processes and delegates state/app container lifecycle to the existing Docker platform script. Pure, security-sensitive parsing/locking/version/cache functions live in a dot-sourced module with fixture tests. Node `24.18.0` becomes the exact frontend baseline everywhere, while the existing full Docker gates remain authoritative.

**Tech Stack:** PowerShell 7, Node 24.18.0 LTS, npm 11.16.0, Vite 8, Go 1.26.5, Air 1.66.0, Docker Compose, Vitest.

---

### Task 1: Upgrade the exact frontend Node baseline

**Repository:** `E:/admin/admin_front_ts`

**Files:**
- Modify: `package.json`
- Modify: `package-lock.json`
- Modify: `Dockerfile`
- Modify: `scripts/docker-frontend-gate.ps1`
- Modify: `scripts/verify-docker-runtime.ps1`
- Modify: `tests/shared/deployment/docker-container.test.ts`
- Modify: `tests/shared/deployment/docker-only-delivery.test.ts`
- Modify: `tests/shared/deployment/docker-runtime-smoke.test.ts`
- Modify: `tests/shared/deployment/quality-gates.test.ts`
- Modify: `performance/baseline.json`
- Modify: `E:/admin/admin_back_go/deploy/docker-first/docker-compose.yml`

- [x] **Step 1: Change deployment tests to require Node 24.18.0**

Replace active exact-image assertions with `node:24.18.0-alpine`, and add assertions that `package.json` has:

```json
{
  "engines": { "node": ">=24.18.0 <25", "npm": "11.16.0" },
  "packageManager": "npm@11.16.0"
}
```

- [x] **Step 2: Run the focused Docker test and observe RED**

Run:

```powershell
pwsh -NoProfile -File scripts/docker-frontend-gate.ps1 -Command 'npm test -- tests/shared/deployment/docker-container.test.ts tests/shared/deployment/docker-only-delivery.test.ts tests/shared/deployment/docker-runtime-smoke.test.ts tests/shared/deployment/quality-gates.test.ts'
```

Expected: failure because runtime/build scripts still contain Node 22 and package metadata is not exact Node 24.

- [x] **Step 3: Update runtime truth and lock metadata**

Use exact `node:24.18.0-alpine` in Dockerfile, Compose, quality wrapper, and runtime verifier. Update `package.json`, then run fixed npm `11.16.0` with `npm install --package-lock-only --ignore-scripts` so the root lock metadata is byte-consistent.

- [x] **Step 4: Rebuild the Node 24 performance baseline and verify GREEN**

Run the production build and baseline writer inside `node:24.18.0-alpine`, then rerun the focused deployment tests. Expected: all focused tests pass and baseline metadata reports `v24.18.0`.

- [x] **Step 5: Commit the version baseline**

```powershell
git add -- package.json package-lock.json Dockerfile scripts/docker-frontend-gate.ps1 scripts/verify-docker-runtime.ps1 tests/shared/deployment/docker-container.test.ts tests/shared/deployment/docker-only-delivery.test.ts tests/shared/deployment/docker-runtime-smoke.test.ts tests/shared/deployment/quality-gates.test.ts performance/baseline.json
git commit -m "build(frontend): move runtime baseline to node 24 lts"

git -C E:/admin/admin_back_go add -- deploy/docker-first/docker-compose.yml
git -C E:/admin/admin_back_go commit -m "build(docker): use node 24 lts for frontend"
```

### Task 2: Add state-only development lifecycle and mutual exclusion

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Modify: `scripts/docker-platform.ps1`
- Modify: `scripts/tests/docker-platform.tests.ps1`
- Create: `scripts/dev/admin-dev-common.ps1`
- Modify: `.gitignore`

- [x] **Step 1: Write failing lifecycle and lock contract tests**

Require a `dev-state` action that stops only frontend/API/worker, starts only MySQL/Redis with health waiting, and never removes volumes. Require `up` and `stop` to reject a live `.tmp/dev/admin-dev.lock.json` while `status` remains read-only.

- [x] **Step 2: Run the PowerShell test and observe RED**

```powershell
pwsh -NoProfile -File scripts/tests/docker-platform.tests.ps1
```

Expected: failure because `dev-state` and the live-lock guard do not exist.

- [x] **Step 3: Implement the shared lock primitives**

`admin-dev-common.ps1` implements exclusive JSON lock creation, live PID plus process-start-time validation, stale-lock removal, and owner-only cleanup. The lock contains only PID/time/repository identity and no secret.

- [x] **Step 4: Implement `dev-state` and guards**

Add `dev-state` to `docker-platform.ps1`: stop the app Compose services, then `up -d --wait` the state Compose project. Call the live-lock guard before `up` and `stop`, but not from `dev-state` or `status`.

- [x] **Step 5: Verify and commit**

```powershell
pwsh -NoProfile -File scripts/tests/docker-platform.tests.ps1
git add -- .gitignore scripts/docker-platform.ps1 scripts/dev/admin-dev-common.ps1 scripts/tests/docker-platform.tests.ps1
git commit -m "feat(dev): separate state containers from host runtimes"
```

### Task 3: Implement secret-safe local configuration and dependency preparation

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Modify: `scripts/dev/admin-dev-common.ps1`
- Create: `scripts/tests/admin-dev.tests.ps1`
- Create: `.air.api.toml`
- Create: `.air.worker.toml`

- [x] **Step 1: Write failing pure-function tests**

Fixture tests cover exact Node/npm/Go validation, strict env parsing, MySQL/Redis/HTTP/log/cert host conversion, secret redaction, dependency-stamp hit/miss, Air private path/version, stale/live lock behavior, and distinct Air build outputs.

- [x] **Step 2: Run and observe RED**

```powershell
pwsh -NoProfile -File scripts/tests/admin-dev.tests.ps1
```

Expected: failure on missing preparation functions and Air configuration.

- [x] **Step 3: Implement the minimal helpers**

Add functions that:

- resolve exact `E:/FlyEnv-Data/app/nodejs/v24.18.0/node.exe` and `npm.cmd`;
- require Go `1.26.5 windows/amd64`;
- install `github.com/air-verse/air@v1.66.0` under `.tmp/tools/air/v1.66.0` only when absent;
- parse `admin-go.env` without logging values;
- transform only container-specific addresses and paths in child memory;
- hash `package-lock.json`, run npm ci only on cache miss, and atomically update the ignored marker.

- [x] **Step 4: Add separate Air configurations**

Both configurations watch backend Go files, send interrupt before rebuild, use a bounded kill delay, and exclude `.git`, `.tmp`, runtime, deployment state, frontend, docs, and generated scratch. Their outputs are respectively `.tmp/dev/api/admin-api.exe` and `.tmp/dev/worker/admin-worker.exe`.

- [x] **Step 5: Verify and commit**

```powershell
pwsh -NoProfile -File scripts/tests/admin-dev.tests.ps1
git add -- .air.api.toml .air.worker.toml scripts/dev/admin-dev-common.ps1 scripts/tests/admin-dev.tests.ps1
git commit -m "feat(dev): prepare pinned host hot reload tools"
```

### Task 4: Implement the one-terminal supervisor

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Create: `scripts/admin-dev.ps1`
- Modify: `scripts/dev/admin-dev-common.ps1`
- Modify: `scripts/tests/admin-dev.tests.ps1`

- [x] **Step 1: Add failing supervisor source/fixture tests**

Require `[WEB]`, `[API]`, and `[WORKER]` line prefixes; redirected stdout/stderr; no secrets in arguments; port conflict refusal; API/Vite readiness; worker stability; sibling cleanup on failure; descendant cleanup in `finally`; and state preservation.

- [x] **Step 2: Run and observe RED**

```powershell
pwsh -NoProfile -File scripts/tests/admin-dev.tests.ps1
```

- [x] **Step 3: Implement `admin-dev.ps1`**

The script validates roots/branches/single checkouts, acquires the lock, invokes `docker-platform.ps1 dev-state`, prepares tools/config/dependencies, checks ports, starts API Air, worker Air, and Vite with `ProcessStartInfo.Environment`, waits for readiness, then pumps prefixed logs. Any failure sets a nonzero result and cleans all owned process trees. `Ctrl+C` cleans host children and lock while leaving state services running.

- [x] **Step 4: Verify failure and cleanup fixtures**

Run the fixture suite with fake child executables for successful startup, one-child failure, timeout, stale lock, and Ctrl+C-equivalent cleanup. Expected: owned descendants are gone and state-stop/down commands are absent.

- [x] **Step 5: Commit**

```powershell
git add -- scripts/admin-dev.ps1 scripts/dev/admin-dev-common.ps1 scripts/tests/admin-dev.tests.ps1
git commit -m "feat(dev): supervise local admin hot reload"
```

### Task 5: Install shortcuts and synchronize the execution plan

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Create: `scripts/install-admin-shortcuts.ps1`
- Modify: `scripts/tests/admin-dev.tests.ps1`
- Modify: `docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md`
- Modify: `README.md`

- [x] **Step 1: Write failing idempotent Profile tests**

Against temporary Profile files, require one managed block after two installs, preserve unrelated content, and ensure every shortcut invokes `pwsh -NoProfile` plus the repository script. Require `admin-dev`, `admin-up`, `admin-stop`, and `admin-status`.

- [x] **Step 2: Implement and verify the installer**

The installer targets both PowerShell 7 and Windows PowerShell Profile paths but always executes orchestration under `pwsh`. Run the test twice and verify command discovery from fresh shells.

- [x] **Step 3: Correct the execution index**

Mark P07 Tasks 6-10 and Gate F complete with their existing evidence; insert the approved/active Windows local-development phase before P09 without weakening P09's full-Docker/destructive prerequisites.

- [x] **Step 4: Commit**

```powershell
git add -- README.md scripts/install-admin-shortcuts.ps1 scripts/tests/admin-dev.tests.ps1 docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md
git commit -m "docs(dev): publish windows admin development workflow"
```

### Task 6: Execute local and full-Docker acceptance

**Repositories:** both primary `master` checkouts

**Files:**
- Modify evidence/checklists only if command results require recording.

- [x] **Step 1: Run static gates**

```powershell
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/tests/admin-dev.tests.ps1
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/tests/docker-platform.tests.ps1
pwsh -NoProfile -File E:/admin/admin_front_ts/scripts/docker-frontend-gate.ps1 -Command 'npm run verify:frontend'
```

- [x] **Step 2: Install shortcuts and start local development**

```powershell
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/install-admin-shortcuts.ps1
admin-dev
```

Confirm one terminal has the three prefixes, Vite and API are ready, the worker is stable, a Vue edit triggers HMR, and a harmless Go edit triggers Air rebuild. Do not use Playwright.

- [x] **Step 3: Exit and prove state preservation**

Press `Ctrl+C`; verify ports `5173`/`8080` are released while `admin-state-mysql-1` and `admin-state-redis-1` remain healthy.

- [x] **Step 4: Rebuild the formal Docker platform**

```powershell
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/docker-platform.ps1 -Action up
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/docker-platform.ps1 -Action status
```

Expected: all five containers are healthy and frontend/API/worker image labels match the two repository HEAD revisions.

- [x] **Step 5: Final clean-state audit**

Both repositories must be clean on `master`, have one checkout, contain no `.worktrees` or deployment Workflow, and retain no host dev process. Stop for the user's local-development acceptance; P09 starts only after that acceptance.

## Implementation evidence (2026-07-20)

- Node/npm baseline: frontend `f83fee8`; Compose baseline: backend `fc6a00a`.
- Development lifecycle/tooling/supervisor/publication: backend `653ecef`, `8fbb381`, `823d809`, and `9b59ead`.
- Focused acceptance fixes covered empty Redis passwords, secret-free child arguments, Windows Go timezone data, README verification contracts, and owner-lock cleanup through backend `9a4aaf8`.
- `scripts/tests/admin-dev.tests.ps1` and `scripts/tests/docker-platform.tests.ps1` passed; the frontend Node 24 Docker gate passed 111 files / 439 tests, lint 0/0, typecheck, build, bundle, architecture, and production dependency audit.
- Real host acceptance returned `LOCAL_DEV_READY=1`, `VITE_HMR=1`, `AIR_API_REBUILD=1`, `AIR_WORKER_REBUILD=1`, `HOST_PORTS_RELEASED=1`, and `DEV_LOCK_RELEASED=1` without Playwright.
- MySQL/Redis remained healthy across the host-development exit, and the full five-container Docker platform was rebuilt healthy afterward.
- User-owned local-development acceptance remains the only open prerequisite before P09.
