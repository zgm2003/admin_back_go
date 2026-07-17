# Admin Docker Stability Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task in the current session. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make the five-container Admin platform recover from Docker Engine and service restarts without stale proxy addresses or application restart loops, while producing traceable images and P01-P03 gate evidence.

**Architecture:** Nginx resolves `admin-api` through Docker DNS at request time. API and worker retry their entire atomic resource graph for a bounded 180-second startup window. The lifecycle script builds backend/frontend images with source revisions before state startup, starts state with health waiting, and starts the already-built app with `--no-build`; a Docker-only destructive regression restores the standard stack in `finally` without deleting volumes.

**Tech Stack:** Go 1.26.5, Docker Compose, nginx-unprivileged 1.31.3, PowerShell 7.6, Vitest 4, MySQL 8.4.10, Redis 8.2.7

---

## File map

### Backend repository

- Modify `internal/runtime/resources.go` to add bounded startup retry around atomic resource opening.
- Modify `internal/runtime/resources_test.go` to prove retry, cleanup, exhaustion, and cancellation.
- Modify `internal/runtime/api.go` and `internal/runtime/worker.go` so production hooks use startup retry.
- Modify `internal/config/docker_compose_test.go` to lock one backend build, worker health, and revision args.
- Modify `deploy/docker-first/docker-compose.yml` to remove the worker build, add revision args, and add worker health.
- Modify `scripts/docker-platform.ps1` to resolve revisions and execute build-once/state/app-no-build phases.
- Modify `scripts/tests/docker-platform.tests.ps1` to guard lifecycle order, revision flow, and volume safety.
- Create `scripts/tests/docker-stability.tests.ps1` for Docker-only runtime recovery and SIGTERM regression.
- Modify P01-P05 plans and `2026-07-15-admin-platform-super-refactor-execution-index.md` for completion evidence, P03.5, and Docker-only dependencies.
- Modify `docs/architecture.md` with the startup recovery and deployment boundary.

### Frontend repository

- Modify `tests/shared/deployment/docker-container.test.ts` to require dynamic Docker DNS and revision metadata.
- Modify `deploy/nginx.conf` to resolve `admin-api` dynamically.
- Modify `Dockerfile` to label the runtime image with `BUILD_REVISION`.
- Modify P07 plan to require Docker-only runtime/E2E execution.

## Task 1: Lock deployment behavior with failing contract tests

**Files:**
- Modify: `E:/admin/admin_front_ts/tests/shared/deployment/docker-container.test.ts`
- Modify: `internal/config/docker_compose_test.go`
- Modify: `scripts/tests/docker-platform.tests.ps1`

- [x] **Step 1: Make the frontend contract require runtime DNS and provenance**

Replace the literal proxy assertion with:

```ts
expect(nginx).toContain('resolver 127.0.0.11 valid=5s ipv6=off')
expect(nginx).toContain('set $admin_api_upstream admin-api:8080')
expect(nginx).toContain('proxy_pass http://$admin_api_upstream')
expect(dockerfile).toContain('ARG BUILD_REVISION=unknown')
expect(dockerfile).toContain('LABEL org.opencontainers.image.revision="${BUILD_REVISION}"')
```

- [x] **Step 2: Make the backend Compose contract require one build and worker health**

Extend the YAML test model with build args and healthcheck, then assert:

```go
api := contract.Services["admin-api"]
worker := contract.Services["admin-worker"]
if api.Build.Context != "../.." || worker.Build.Context != "" {
	t.Fatal("backend image must be built once by admin-api")
}
if api.Build.Args["BUILD_REVISION"] != "${ADMIN_BACKEND_BUILD_REVISION:-unknown}" {
	t.Fatal("backend revision build arg is not wired")
}
if worker.Healthcheck.Test[1] != "kill -0 1" {
	t.Fatal("worker must expose PID 1 health")
}
if contract.Services["frontend"].Build.Args["BUILD_REVISION"] != "${ADMIN_FRONTEND_BUILD_REVISION:-unknown}" {
	t.Fatal("frontend revision build arg is not wired")
}
```

- [x] **Step 3: Strengthen lifecycle source assertions**

Require the script to contain `Resolve-GitRevision`, both revision environment names, a single Compose `build` invocation naming `admin-api` and `frontend`, state `up --wait`, and app `up --no-build --wait`. Reject app `up` with `--build`, `down -v`, and `--volumes`.

- [x] **Step 4: Run Docker-contained RED checks**

```powershell
docker run --rm -v "${PWD}:/src" -w /src docker.m.daocloud.io/library/golang:1.26.5-bookworm `
  go test ./internal/config -run TestDockerAppComposeOwnsOnlyApplicationServices -count=1
docker run --rm -v "E:/admin/admin_front_ts:/app" -w /app node:22.23.1-alpine `
  npm exec vitest run tests/shared/deployment/docker-container.test.ts
pwsh -NoProfile -File scripts/tests/docker-platform.tests.ps1
```

Expected: all three fail for the newly required behavior, not for syntax or missing dependencies.

## Task 2: Add bounded atomic resource startup retry

**Files:**
- Modify: `internal/runtime/resources_test.go`
- Modify: `internal/runtime/resources.go`
- Modify: `internal/runtime/api.go`
- Modify: `internal/runtime/worker.go`

- [x] **Step 1: Write retry tests before implementation**

Add tests using an injected policy:

```go
func TestOpenResourcesWithStartupRetryClosesPartialAttemptsBeforePublishing(t *testing.T) {
	attempts, waits, closes := 0, 0, 0
	openers := successfulOpeners(&[]string{})
	openers.Redis = func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
		attempts++
		if attempts < 3 {
			return OpenedResource[*redisclient.Client]{}, errors.New("redis unavailable")
		}
		return openedRedis(&[]string{}, "redis"), nil
	}
	openers.Database = func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
		opened := openedDatabase(&[]string{}, "database")
		close := opened.Close
		opened.Close = func(ctx context.Context) error { closes++; return close(ctx) }
		return opened, nil
	}
	resources, err := openResourcesWithStartupRetry(t.Context(), config.ProcessAPI, configuredResources(), openers, resourceRetryPolicy{
		Attempts: 3,
		Wait: func(context.Context, time.Duration) error { waits++; return nil },
	})
	if err != nil || resources == nil || attempts != 3 || waits != 2 || closes != 2 {
		t.Fatalf("resources=%+v err=%v attempts=%d waits=%d closes=%d", resources, err, attempts, waits, closes)
	}
	defer resources.Close(t.Context())
}
```

Add separate tests that the last retryable error is returned after the exact attempt bound, a permanent error is not retried, and cancellation during the wait returns `context.Canceled`.

- [x] **Step 2: Verify runtime RED inside Docker**

```powershell
docker run --rm -v "${PWD}:/src" -w /src docker.m.daocloud.io/library/golang:1.26.5-bookworm `
  go test ./internal/runtime -run 'TestOpenResourcesWithStartupRetry' -count=1
```

Expected: compile failure because the retry policy/helper does not exist.

- [x] **Step 3: Implement the minimal retry wrapper**

Keep `OpenResources` unchanged as the single-attempt primitive and add:

```go
const (
	resourceStartupAttempts = 181
	resourceStartupDelay    = time.Second
)

type resourceRetryPolicy struct {
	Attempts int
	Delay    time.Duration
	Wait     func(context.Context, time.Duration) error
}

func OpenResourcesWithStartupRetry(ctx context.Context, process config.Process, cfg config.Config, open Openers) (*Resources, error) {
	return openResourcesWithStartupRetry(ctx, process, cfg, open, resourceRetryPolicy{
		Attempts: resourceStartupAttempts,
		Delay:    resourceStartupDelay,
		Wait:     waitForResourceRetry,
	})
}
```

The helper normalizes nil context, attempts at least once, returns immediately for a non-retryable error, waits with the context between retryable failures, and returns the last failure after exhaustion. Retryability is determined with `errors.As(err, *apperror.Error)` plus `Retryable()`; `OpenResources` already closes the failed partial graph.

- [x] **Step 4: Activate retry in both production hooks**

Replace only the production resource calls in `api.go` and `worker.go`:

```go
opened, err := OpenResourcesWithStartupRetry(ctx, config.ProcessAPI, cfg, Openers{Telemetry: recorder})
opened, err := OpenResourcesWithStartupRetry(ctx, config.ProcessWorker, cfg, Openers{Telemetry: recorder})
```

- [x] **Step 5: Verify GREEN and the surrounding runtime**

```powershell
docker run --rm -v "${PWD}:/src" -w /src docker.m.daocloud.io/library/golang:1.26.5-bookworm `
  go test ./internal/runtime -count=1
```

Expected: runtime tests pass with no real-time retry delay.

## Task 3: Implement dynamic proxy, provenance, health, and build-once lifecycle

**Files:**
- Modify: `E:/admin/admin_front_ts/deploy/nginx.conf`
- Modify: `E:/admin/admin_front_ts/Dockerfile`
- Modify: `deploy/docker-first/docker-compose.yml`
- Modify: `scripts/docker-platform.ps1`
- Modify: `docs/architecture.md`

- [x] **Step 1: Implement frontend DNS and label changes**

Add the resolver at `server` scope and use the variable in the API location. Add global `ARG BUILD_REVISION=unknown`, redeclare it in the runtime stage, and add the OCI revision label.

- [x] **Step 2: Make Compose build the backend once**

Keep `admin-api.build`, add its backend revision arg, remove `admin-worker.build`, add frontend revision arg, and add:

```yaml
    healthcheck:
      test: ["CMD-SHELL", "kill -0 1"]
      interval: 10s
      timeout: 3s
      retries: 3
      start_period: 5s
```

- [x] **Step 3: Make lifecycle phases explicit**

Add a strict `Resolve-GitRevision` helper. In `up`, temporarily set `ADMIN_BACKEND_BUILD_REVISION` and `ADMIN_FRONTEND_BUILD_REVISION`, then run exactly:

```powershell
Invoke-Docker @('compose', '-f', $appCompose, 'build', 'admin-api', 'frontend')
Invoke-Docker @('compose', '-f', $stateCompose, 'up', '-d', '--wait', '--wait-timeout', '180')
Invoke-Docker @('compose', '-f', $appCompose, 'up', '-d', '--no-build', '--wait', '--wait-timeout', '300')
```

Restore both process environment variables in `finally`, including the distinction between absent and empty values.

- [x] **Step 4: Document the boundary**

Record dynamic Docker DNS, the 180-second whole-resource-graph retry, one-image API/worker ownership, PID health semantics, and Docker-only runtime verification in `docs/architecture.md`.

- [x] **Step 5: Verify contract GREEN**

Run the three Task 1 commands again. Expected: all exit `0`.

- [x] **Step 6: Commit focused repository changes**

Frontend:

```powershell
git add -- Dockerfile deploy/nginx.conf tests/shared/deployment/docker-container.test.ts
git diff --cached --check
git commit -m "fix(deploy): refresh Docker API discovery"
```

Backend:

```powershell
git add -- internal/runtime/resources.go internal/runtime/resources_test.go internal/runtime/api.go internal/runtime/worker.go internal/config/docker_compose_test.go deploy/docker-first/docker-compose.yml scripts/docker-platform.ps1 scripts/tests/docker-platform.tests.ps1 docs/architecture.md
git diff --cached --check
git commit -m "fix(deploy): stabilize Docker process recovery"
```

## Task 4: Add and execute the Docker-only stability regression

**Files:**
- Create: `scripts/tests/docker-stability.tests.ps1`

- [x] **Step 1: Implement safe Docker helpers**

The script resolves Docker, captures Compose service container IDs, waits on inspect conditions, reads logs without treating application stderr as a PowerShell failure, asserts restart counts/exit codes, and wraps all disruption in `try/finally`.

- [x] **Step 2: Implement the API-address replacement scenario**

Capture frontend ID and API address, stop/remove API only, reserve that exact address using `admin-go-backend:local`, recreate API with `--no-deps --no-build`, require the new address to differ, wait for API health, and execute the proxied ping from inside frontend. Assert the frontend ID is unchanged.

- [x] **Step 3: Implement state-late recovery**

Stop state without removing it, force-recreate API/worker from existing images, wait five seconds, require both containers running with restart count zero, start/wait state, then require API health, worker startup log, and unchanged zero restart counts.

- [x] **Step 4: Implement SIGTERM and final restoration**

Stop worker then API using Docker `SIGTERM --time 20`; each must exit zero and log one `process stopped`. Restore each via Compose `--no-build`. In `finally`, remove only the temporary reservation, run state `up --wait`, then app `up --no-build --wait`.

- [x] **Step 5: Build the standard stack and run the regression**

```powershell
pwsh -NoProfile -File scripts/docker-platform.ps1 up
pwsh -NoProfile -File scripts/tests/docker-stability.tests.ps1
```

Expected: proxy recovery, state-late recovery, image labels, restart counts, and SIGTERM assertions pass; final Compose status shows five services running and four health checks healthy.

- [x] **Step 6: Commit the regression**

```powershell
git add -- scripts/tests/docker-stability.tests.ps1
git diff --cached --check
git commit -m "test(deploy): prove Docker restart recovery"
```

## Task 5: Close P01-P03 documentation and gates

**Files:**
- Modify: `docs/superpowers/plans/2026-07-15-admin-foundation-verification-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-go-runtime-contracts-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-go-identity-routing-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-go-durable-work-realtime-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md`
- Modify: `E:/admin/admin_front_ts/docs/superpowers/plans/2026-07-15-admin-frontend-realtime-resource-plan.md`

- [x] **Step 1: Record fresh gate commands**

Run P01 clean-cache/build, P02 recovery/convergence/invariant, P03 contract/runtime/SIGTERM, and P03.5 Docker stability commands. Capture exact revisions, fingerprint, reconciliation applied/skipped counts, artifact hash/counts, contract drift result, test counts, exit codes, restart counts, image labels, and final container health.

- [x] **Step 2: Mark only implemented P01-P03 steps complete**

Change their task step checkboxes to `[x]`, add a dated `Completion evidence (2026-07-17)` section to each plan, and leave no claim unsupported by the captured commands.

- [x] **Step 3: Add P03.5 and Gate C.5 to the execution index**

Insert P03.5 between P03 and P04/P05, make P04/P05 depend on it, update the dependency graph, mark index setup/protocol steps that have evidence, mark Gates A-C complete, and add:

```markdown
- [x] **Gate C.5:** P03.5 proves dynamic API discovery, bounded state-late startup with zero restart loops, correct image revisions, and zero-exit Docker SIGTERM; final restoration preserves all state volumes.
```

- [x] **Step 4: Make later runtime execution Docker-only**

Add an execution note to P04, P05, and P07: runtime services and runtime/E2E tests are launched only as containers; host PowerShell may orchestrate Docker and perform static source checks but may not start API, worker, Vite, MySQL, or Redis directly.

- [x] **Step 5: Verify documentation consistency and commit**

```powershell
rg -n 'P03\.5|Gate C\.5|Docker-only|Completion evidence \(2026-07-17\)' docs/superpowers/plans E:/admin/admin_front_ts/docs/superpowers/plans
git diff --check
git add -- docs/superpowers/plans/2026-07-15-admin-foundation-verification-plan.md docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md docs/superpowers/plans/2026-07-15-admin-go-runtime-contracts-plan.md docs/superpowers/plans/2026-07-15-admin-go-identity-routing-plan.md docs/superpowers/plans/2026-07-15-admin-go-durable-work-realtime-plan.md docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md docs/superpowers/plans/2026-07-17-admin-docker-stability-closure-plan.md
git commit -m "docs(plan): close P01-P03 and gate P03.5"
```

Commit the P07 note in the frontend repository with its deployment changes or a focused documentation commit.

## Final verification gate

- [x] Run backend static/unit/race/contract verification through the repository Docker-backed entrypoints.
- [x] Run the frontend deployment contract and production build inside the frontend image build.
- [x] Run `scripts/tests/docker-stability.tests.ps1` from a freshly built standard stack.
- [x] Inspect backend/frontend image revision labels against repository `HEAD`.
- [x] Require frontend proxy, API `/health`, API `/ready`, MySQL health, Redis health, and worker PID health.
- [x] Require three repositories to have no uncommitted changes and the five standard containers to be restored.
- [x] Leave a clean, verified P04-ready handoff; the user will create and start the P04 goal separately.

## Completion evidence (2026-07-17)

- TDD RED was observed for frontend Docker DNS/revision assertions, backend Compose build/health assertions, lifecycle source assertions, and the new runtime retry API. Focused GREEN runs passed inside pinned Node/Go containers.
- Runtime retry tests prove partial-attempt cleanup, exact attempt exhaustion, permanent-error short circuiting, and context cancellation. The full runtime/config suites passed after implementation.
- `docker-platform.ps1 up` built `admin-api` and `frontend` once, waited for state, and started app with `--no-build`; its Dockerfile test stage passed `go test ./...`.
- `docker-stability.tests.ps1` passed API address replacement without frontend recreation, state-late API/worker startup, restart counts `0/0`, Docker SIGTERM exit `0`, correct image revision labels, and final five-container restoration.
- Gate A clean-cache verification and Gate B full database verification exited `0`; Gate C contract/runtime/race verification passed with no bundle drift. The execution index now records Gates A, B, C, and C.5.
- P04, P05, and P07 explicitly require Docker-only runtime/integration/E2E execution. The user retained ownership of creating and starting the P04 goal.
