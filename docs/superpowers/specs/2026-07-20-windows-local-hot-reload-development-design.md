# Windows Local Hot-Reload Development Design

**Status:** Implemented and automatically verified on 2026-07-20; final user acceptance pending.

**Execution order:** Implement and accept this development loop before opening P09. P09 remains the next product-reduction phase after this work.

## 1. Purpose

The current `admin-up` path correctly produces revision-labelled frontend and backend Docker images, but it rebuilds the Vue application and both Go binaries before recreating the application containers. That is the correct production path and an unnecessarily slow edit-feedback loop.

This design introduces a Windows-only hybrid development mode:

- MySQL and Redis continue to run in Docker with their existing named volumes.
- Vite, the Go API, and the Go worker run on the Windows host with hot reload.
- Production builds, release evidence, full acceptance, rollback, and deployment remain full-Docker operations.

The change is a development optimization, not a deployment-architecture change.

## 2. Goals

1. `admin-dev` starts a complete local Admin development environment with one command.
2. Vue edits update through Vite HMR without rebuilding an image.
3. Shared/backend Go edits rebuild and restart both API and worker through Air.
4. `[WEB]`, `[API]`, and `[WORKER]` logs appear in one PowerShell terminal.
5. `Ctrl+C` stops all host development processes while leaving MySQL and Redis running.
6. `admin-up` remains the single full-Docker build/start command for production-like verification.
7. The frontend's formal Node baseline moves from Node 22 to exact Node `24.18.0` LTS.
8. No secret is copied to a committed file, printed, or added to a command line.

## 3. Non-goals

- macOS or Linux host development.
- Docker bind-mounted hot reload for frontend or backend application code.
- Replacing Docker-based production gates with host commands.
- Starting MySQL or Redis on the host.
- Adding GitHub Workflow deployment, a worktree, Playwright, or a browser automation gate.
- Automatically applying schema migrations when `admin-dev` starts.
- Making uncommitted development state eligible for a production release.

## 4. Runtime boundary

| Concern | `admin-dev` | `admin-up` / formal release |
| --- | --- | --- |
| MySQL | Docker `admin-state-mysql-1` | Docker `admin-state-mysql-1` |
| Redis | Docker `admin-state-redis-1` | Docker `admin-state-redis-1` |
| Frontend | Host Vite with HMR | Revision-labelled Nginx image |
| Admin API | Host Air + Go binary | Revision-labelled backend image |
| Admin worker | Host Air + Go binary | Same backend image as API |
| Static quality gates | Optional focused host development checks; authoritative gate remains Docker | Pinned Node Docker gate |
| Runtime acceptance | Developer checks at `localhost` | Five healthy containers plus Docker smoke |

`admin-dev` may run with a dirty working tree because its purpose is active editing. Formal release and completion gates continue to require committed, clean revisions.

## 5. Fixed Windows toolchain

The supported host is Windows with PowerShell 7.

| Tool | Required value | Resolution rule |
| --- | --- | --- |
| PowerShell | major version 7 | Resolve `pwsh`; reject Windows PowerShell as the actual supervisor runtime |
| Node | `v24.18.0` | Use `E:\FlyEnv-Data\app\nodejs\v24.18.0\node.exe` directly |
| npm | `11.16.0` | Use the sibling `npm.cmd` directly |
| Go | `go1.26.5 windows/amd64` | Resolve host `go.exe`, then require the exact version and platform |
| Air | `v1.66.0` | Install privately with `go install github.com/air-verse/air@v1.66.0` |
| Docker | existing Docker Desktop CLI | Resolve the existing fixed CLI first, then `docker.exe` |

Node `24.18.0` is the exact LTS baseline for local development, the frontend Dockerfile, the Docker quality wrapper, Compose build arguments, tests, and performance metadata. `package.json` declares Node 24 only (`>=24.18.0 <25`) and npm `11.16.0`; Node 26 is no longer accepted by `admin-dev`, even if Vite could start under it.

Air is installed below ignored backend scratch storage, for example `.tmp/tools/air/v1.66.0/air.exe`. It is never installed globally and is reinstalled only when missing or when its version marker is invalid.

## 6. Command model

### 6.1 `admin-dev`

`admin-dev` is a PowerShell profile function that launches a repository-owned PowerShell 7 composition root. It performs this sequence:

1. Resolve the two exact primary repositories and require both to be on `master` with one registered checkout each.
2. Acquire the local-development lock.
3. Stop only the Docker application services: frontend, API, and worker.
4. Start or retain the Docker state services and wait for MySQL and Redis health.
5. Validate the fixed toolchain and prepare private dependencies.
6. Convert the existing ignored Docker runtime configuration into child-process-only host configuration.
7. Ensure ports `5173` and `8080` are free after the Docker application containers stop.
8. Start API Air, worker Air, and Vite under one supervisor.
9. Wait for Vite, API `/health`, API `/ready`, and a stable worker child process.
10. Stream prefixed logs until a process fails or the user presses `Ctrl+C`.

### 6.2 `admin-up`

`admin-up` keeps its existing meaning: rebuild the frontend and backend images, start the five-container platform, and wait for health. It does not become a development command.

If a live `admin-dev` lock exists, `admin-up` refuses before building and tells the user to exit `admin-dev`. `admin-stop` uses the same protection so it cannot remove MySQL or Redis from underneath a running host API/worker. `admin-status` remains read-only.

### 6.3 PowerShell shortcuts

An idempotent repository script owns the marked shortcut block in both user Profile locations. Each shortcut invokes `pwsh -NoProfile -File ...`, so a command entered from an older Windows PowerShell terminal still runs the actual orchestration under PowerShell 7. The managed commands are:

```text
admin-dev
admin-up
admin-stop
admin-status
```

The installer preserves all Profile content outside its own markers.

## 7. Docker state ownership

The existing `deploy/docker-state/docker-compose.yml` remains the only MySQL/Redis definition. Development mode uses its published loopback ports:

```text
MySQL  127.0.0.1:33306
Redis  127.0.0.1:36379
```

It never runs `down`, removes volumes, creates alternate databases, or starts host state services. `Ctrl+C` leaves both state containers healthy and running. A later `admin-up` reuses the same data volumes.

The development script may stop the three `admin-app` services through their existing Compose project, but it does not delete their images. This makes switching back to full Docker deterministic.

## 8. Secret-safe host configuration

The ignored `deploy/docker-first/admin-go.env` remains the runtime source. The supervisor parses it in memory with these rules:

- require exactly one value for every required key;
- split only on the first `=` so opaque/base64 values remain intact;
- reject malformed names, duplicate keys, line breaks, and missing required values;
- never serialize the expanded environment, DSN, `APP_SECRET`, Redis password, or child command line to logs.

Only environment values that differ between container and host are transformed:

| Container value | Host child value |
| --- | --- |
| MySQL host `mysql:3306` inside `MYSQL_DSN` | `127.0.0.1:33306` with the rest of the DSN byte-preserved |
| `REDIS_ADDR=redis:6379` | `REDIS_ADDR=127.0.0.1:36379` |
| `HTTP_ADDR=:8080` | `HTTP_ADDR=127.0.0.1:8080` |
| `LOG_DIR=/app/runtime/logs` | absolute `deploy/docker-first/runtime/logs` path |
| `PAYMENT_CERT_BASE_DIR=/app` | absolute `deploy/docker-first` path, preserving `runtime/...` certificate references |

All other approved values, including `APP_SECRET`, Redis database numbers, queue policy, scheduler policy, realtime publisher, and CORS origins, pass to API and worker unchanged through `ProcessStartInfo.Environment`. They are not exported into the parent terminal and are not written to `.env`.

This preserves Cookie/session validity across Go reloads because the same application secret and Redis state are reused.

## 9. Frontend dependency cache

Host `node_modules` is separate from the named Docker quality-gate volume. Before Vite starts, the supervisor computes SHA-256 for `package-lock.json` and compares an ignored atomic marker containing:

```text
lockfile_sha256
node_version = v24.18.0
npm_version = 11.16.0
```

It runs the fixed `npm.cmd ci --no-audit --no-fund` only when:

- host `node_modules` is missing;
- the marker is missing or malformed;
- the lockfile hash differs;
- the Node or npm version differs.

The marker is written only after `npm ci` exits zero. Ordinary `admin-dev` starts therefore avoid reinstalling dependencies.

Vite uses the committed development origins:

```text
http://localhost:8080
ws://localhost:8080/api/admin/v1/realtime/ws
```

The existing Cookie/Origin contract remains unchanged.

## 10. Go hot reload

Two tracked Air configurations provide separate build outputs:

```text
.tmp/dev/api/admin-api.exe
.tmp/dev/worker/admin-worker.exe
```

Both watch shared Go sources because a shared service/repository change can affect both processes. Generated scratch, runtime data, Git metadata, frontend files, and documentation are excluded. Air uses interrupt-before-rebuild behavior with a bounded kill delay so an old process releases resources before its replacement starts.

The exact host Go directory is prepended only to the Air child environment after version validation. API and worker receive the same secret-safe runtime map, use the backend repository as their working directory, and do not load an additional generated `.env` file.

## 11. Supervision and logs

The PowerShell supervisor starts children with redirected stdout/stderr and prefixes complete lines:

```text
[WEB] ...
[API] ...
[WORKER] ...
```

It does not merge or rewrite JSON application log bodies beyond the prefix. Readiness is reached only when:

- Vite accepts HTTP connections on `127.0.0.1:5173`;
- API `/health` and `/ready` return success;
- the worker Air child binary exists and remains alive for the configured stability window.

If one core process exits unexpectedly, the supervisor names the failing component, stops the remaining host process trees, removes the lock, and returns nonzero. MySQL and Redis stay running.

On `Ctrl+C`, the supervisor first requests bounded graceful termination and then terminates any remaining descendant process tree. Cleanup is performed in `finally`, including partial-start failures. It never uses a broad process-name kill that could affect unrelated Node or Go programs.

## 12. Lock and conflict handling

The ignored lock contains the supervisor PID, creation time, backend path, and frontend path. Creation is exclusive and atomic.

- A live matching PID means another `admin-dev` owns the environment; a second start fails.
- A missing PID means the lock is stale; the next invocation removes it before proceeding.
- A reused PID whose executable/command identity does not match the supervisor is stale, not active.
- Ports are checked after Docker application services stop. Any remaining owner causes a fail-closed message with the port and PID; the script does not kill an unknown process.

The lock is coordination metadata only and contains no secret.

## 13. Failure behavior

| Failure | Required behavior |
| --- | --- |
| Wrong Node/npm/Go/PowerShell version | Stop before launching children and print the exact expected path/version |
| Docker unavailable or state unhealthy | Stop; do not fall back to host MySQL/Redis |
| Air installation fails | Stop with the pinned module/version; do not use a global or different Air |
| `npm ci` fails | Do not write the dependency marker; stop |
| Runtime configuration is malformed | Stop without printing the offending secret value |
| Port owned by an unknown process | Report port/PID and stop; never kill it automatically |
| Vite/API/worker readiness timeout | Identify the component, clean host children, leave state running |
| Child exits after readiness | Prefix the final output, clean siblings, exit nonzero |
| `Ctrl+C` | Clean host children and lock, leave state running, exit normally |

## 14. Production invariants

This development mode must not weaken any production invariant:

1. Dockerfile and Docker quality gates use exact Node `24.18.0` images.
2. API and worker still ship from one revision-labelled Go image.
3. Frontend still ships as one revision-labelled Nginx image.
4. `admin-up` still builds and waits for all five containers.
5. Formal verification still runs lint, types, tests, coverage, contract, locale, route, bundle, dependency audit, backend tests, runtime smoke, and revision-label checks through Docker-defined gates.
6. No host-development cache or tool is copied into a production image or Git commit.
7. P09 destructive DDL still requires recovery proof and fresh explicit user approval.

## 15. Testing strategy

### 15.1 Contract tests

PowerShell fixture tests cover:

- exact version acceptance and rejection;
- Air private installation path/version;
- env parsing and host transformation without secret output;
- dependency-marker hit, miss, malformed marker, and failed-install behavior;
- live-lock rejection, stale-lock cleanup, and PID-reuse handling;
- port conflict refusal;
- prefix preservation for stdout/stderr;
- partial-start and child-failure cleanup;
- `admin-up`/`admin-stop` refusal while development is live;
- state containers remaining up after cleanup.

Frontend source/deployment tests require Node `24.18.0` consistently across `package.json`, Dockerfile, Compose, wrappers, and evidence metadata. Existing full frontend and backend gates remain blocking.

### 15.2 Manual acceptance

Without Playwright, the user verifies:

1. Run `admin-dev` and observe all three prefixed streams.
2. Edit a Vue component and observe immediate HMR without restarting Vite.
3. Edit shared/API Go code and observe API rebuild plus successful `/ready` recovery.
4. Edit worker/shared Go code and observe worker rebuild without leaving an old worker process.
5. Exercise login, Cookie refresh, realtime, queue monitor, and a worker-backed task.
6. Press `Ctrl+C`; confirm `5173`/`8080` are released and MySQL/Redis remain healthy.
7. Run `admin-up`; confirm five healthy containers and exact frontend/backend revision labels.

## 16. Documentation and plan impact

Implementation updates the execution index before P09:

```text
P07 Tasks 6-10: complete and user-reviewed
Windows local development loop: active before P09
P09: pending until this development loop is accepted
```

Earlier statements that prohibit host Vite/Go execution are retained as historical execution evidence for their original phases but are superseded for future local development by this approved design. They continue to apply to formal verification and deployment.

## 17. Acceptance criteria

The design is complete only when all of the following are true:

- `admin-dev` produces a complete hot-reload environment from one command.
- Repeat startup with an unchanged lockfile does not run `npm ci`.
- Local runtime uses exact Node `24.18.0`, npm `11.16.0`, Go `1.26.5`, and Air `1.66.0`.
- No secret appears in terminal output, process arguments, tracked files, or dependency markers.
- API, worker, and Vite are cleaned on `Ctrl+C` and failure; Docker state persists.
- Full Docker rebuild and every formal gate pass after local-development acceptance.
- Both repositories are clean on `master`, with no `.worktrees` or deployment Workflow.
- The user completes the manual hot-reload review before P09 begins.
