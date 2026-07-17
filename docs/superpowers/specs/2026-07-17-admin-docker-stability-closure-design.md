# Admin Docker Stability Closure Design

**Date:** 2026-07-17
**Status:** Approved
**Phase:** P03.5, between P03 Runtime/Contracts and P04 Identity/RBAC
**Scope:** Docker lifecycle, backend dependency startup, Nginx service discovery, image provenance, and Docker-only regression evidence

## Goal

Close the operational gaps found after P01-P03 so the five-container Admin stack remains stable when Docker Engine or individual application containers restart. P04 must start from a platform whose application processes tolerate state-service recovery, whose frontend follows the current API container address, and whose local images identify the exact source revisions that produced them.

## Observed failures and root causes

### Nginx retained an obsolete API address

`admin_front_ts/deploy/nginx.conf` used a literal `proxy_pass http://admin-api:8080`. Nginx resolved that name while loading configuration and retained the resulting address. After Docker Engine restored the two Compose projects, the old API address was reassigned to the worker. The unchanged frontend then proxied API traffic to the wrong container and returned `502` until the frontend was restarted.

The root cause is startup-time DNS resolution in a long-lived Nginx process, not an API readiness or Vue routing failure.

### Engine auto-restore bypassed lifecycle ordering

`admin-state` and `admin-app` are intentionally separate Compose projects and all services use `restart: unless-stopped`. Docker Engine restores those containers independently; it does not call `scripts/docker-platform.ps1`, so the lifecycle script's state-before-app ordering is not guaranteed. API and worker currently exit immediately when their first MySQL or Redis ping fails. During one Engine recovery this produced seven or eight restarts per process before state became available.

The root cause is treating a temporarily unavailable Docker dependency as a terminal bootstrap error even though restart-policy recovery can start application and state projects concurrently.

### Deployment ownership was incomplete

The worker had no health check, API and worker each declared the same backend build, and local Compose builds left `org.opencontainers.image.revision=unknown`. These do not directly cause the two failures above, but they prevent deterministic `compose --wait`, waste build work, and make a running image impossible to trace to its repository state.

## Chosen architecture

### Dynamic Docker DNS in Nginx

The frontend runtime configures Docker's embedded DNS resolver at `127.0.0.11` and sends `proxy_pass` through a variable:

```nginx
resolver 127.0.0.11 valid=5s ipv6=off;
set $admin_api_upstream admin-api:8080;
proxy_pass http://$admin_api_upstream;
```

Variable-based proxying causes Nginx to refresh the service name instead of retaining the address resolved when the frontend started. HTTP and WebSocket forwarding keep the existing path and headers. A temporary API outage may still return `502`, but replacing API at a different address must recover without recreating frontend.

### Bounded, whole-graph dependency retry

`internal/runtime` keeps `OpenResources` as the single-attempt, all-or-nothing primitive. A new production startup wrapper retries only classified retryable dependency failures for a code-owned bounded window:

- at most 181 attempts;
- one initial attempt followed by at most 180 one-second waits;
- context cancellation stops waiting immediately;
- each failed attempt closes every resource opened by that attempt;
- no partial `Resources` value is published;
- configuration and other permanent errors are returned without retry;
- after the bound is exhausted, the last dependency error is returned and the existing restart policy may take over.

Both API and worker production hooks use the wrapper. Tests inject a zero-time waiter and small attempt count; retry duration is not an environment setting.

This keeps process-owned resource initialization atomic while covering the same 180-second recovery horizon used by the state lifecycle wait.

### One backend build per lifecycle invocation

`docker-platform.ps1 up` performs these phases explicitly:

1. resolve clean source revisions for backend and frontend;
2. build `admin-api` and `frontend` once, with service-specific revision arguments;
3. start/wait for `admin-state`;
4. start/wait for `admin-app` with `--no-build`.

`admin-worker` consumes the same `admin-go-backend:local` image as API and has no independent build declaration. Application startup therefore cannot rebuild the backend a second time.

The lifecycle script does not require a clean worktree because verification may build intentional uncommitted changes, but the label always records `HEAD`, not `unknown`. Release gates still require a clean repository before acceptance.

### Worker health and image provenance

Worker receives a PID 1 liveness health check (`kill -0 1`). It deliberately proves container-process liveness rather than inventing an HTTP endpoint or claiming dependency readiness. Runtime dependency health remains owned by API `/ready` and runtime reports.

Both Dockerfiles accept `BUILD_REVISION` and place it in `org.opencontainers.image.revision`. Compose supplies different backend/frontend values resolved by the lifecycle script. Docker-only acceptance inspects both labels and compares them to `git rev-parse HEAD` in the owning repository.

## Docker-only regression

`scripts/tests/docker-stability.tests.ps1` operates only on containers and Docker networking. It never starts API, worker, Vite, MySQL, or Redis as host processes.

The regression performs four scenarios:

1. **API address replacement:** preserve the frontend container, remove API, temporarily reserve its old network address, recreate API at a different address, and require the frontend proxy to return HTTP 200 without a frontend restart.
2. **State-late recovery:** stop state, recreate API and worker, require both to remain alive with restart count zero while state is absent, restore state, and require API readiness without a restart loop.
3. **Graceful termination:** send Docker `SIGTERM` to worker and API, require exit code zero and exactly one `process stopped` record, then restore each service from the already-built image.
4. **Finally restoration:** remove only the temporary address-reservation container, start/wait state, then start/wait app using `--no-build`. No command uses `down -v`, `--volumes`, or removes a state volume.

The test validates the actual five-container topology. It is intentionally serialized because it temporarily owns the shared `admin-platform` network and local state/app containers.

## Plan and gate integration

- P01-P03 implementation checkboxes are updated only after their recorded commands and acceptance evidence are reviewed.
- The execution index gains P03.5 and Gate C.5.
- P04 and P05 depend on P03.5, not directly on an operationally incomplete P03.
- P04, P05, and P07 execution instructions state that application/runtime/E2E commands run through Docker; host commands are limited to static source checks and Docker orchestration.
- P06 retains ownership of frontend Linux test portability (`E:/admin/...` fixtures and `.dockerignore` coverage). P03.5 does not broaden into that work.

## Acceptance criteria

1. Frontend proxy follows a recreated API address without recreating frontend.
2. API and worker survive state-late startup for the bounded recovery window with zero restarts.
3. Partial resource attempts are closed before retry and are never published.
4. Worker participates in `docker compose --wait` through a PID 1 health check.
5. One `docker-platform.ps1 up` builds the backend image exactly once and app startup uses `--no-build`.
6. Backend and frontend image labels equal their repository `HEAD` revisions and are not `unknown`.
7. API and worker exit zero on Docker `SIGTERM`.
8. Regression cleanup restores five healthy/running services and never deletes MySQL or Redis volumes.
9. P01-P03 and Gates A-C contain concrete completion evidence; P03.5 has Gate C.5 evidence.

## Out of scope

- Combining state and application into one Compose project.
- Removing `restart: unless-stopped`.
- Adding a worker HTTP server.
- Moving dependency retry policy into environment variables.
- Reworking frontend path fixtures or running the complete frontend suite inside Linux; P06/P07 own that portability work.
- P04 identity, session, refresh, transport, or RBAC behavior.
