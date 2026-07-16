# Full Docker Platform Design

**Date:** 2026-07-16  
**Status:** Approved for specification  
**Scope:** Admin frontend, Go backend, MySQL, and Redis local orchestration with a production-aligned container topology

## Goal

Run the complete Admin platform as five containers while keeping application releases separate from state-service lifecycle:

1. Nginx frontend
2. Go API
3. Go worker
4. MySQL 8.4
5. Redis 8.2

The local stack must remain easy to start, expose non-default loopback state ports, preserve the P02 database, and provide a direct path to production deployment.

## Current-state explanation

Docker Desktop grouping does not mean one container is inside another. Redis is currently a peer service in the `admin-go-backend` Compose project. MySQL is separate because P02 created `admin-p02-mysql` as a labeled database-evolution workspace with an anonymous data volume, a verified recovery artifact, and rollback state. That isolation protected the database while Docker and P02 verification were incomplete.

The frontend currently runs as a Windows Vite process. The approved change replaces that host process with a production-style Nginx container after container acceptance passes.

## Approaches considered

### 1. Separate application and state Compose projects — chosen

- `admin-app`: frontend, API, worker
- `admin-state`: MySQL, Redis

This keeps database/cache lifecycle independent from application builds and deploys. It mirrors the intended production boundary and still supports one-command local orchestration through a wrapper script.

### 2. One Compose project containing all five services

This is the simplest Docker Desktop view, but application-level `down`, recreation, or volume flags can accidentally affect MySQL and Redis. It couples state availability to application deployment.

### 3. Containerize only the frontend and keep the current state layout

This is the smallest immediate change, but leaves MySQL as a special P02 container and Redis owned by the application project. It does not establish the desired deployment boundary.

## Target topology

```text
Browser
  |
  v
127.0.0.1:5173
  frontend (Nginx, admin-app)
      |-- /api/* and WebSocket --> admin-api:8080
      `-- Vue history fallback --> /index.html

admin-api (admin-app) ------+
admin-worker (admin-app) ---+--> shared network: admin-platform
                                      |--> mysql:3306 (admin-state)
                                      `--> redis:6379 (admin-state)

Host diagnostics only:
  API:   127.0.0.1:8080
  MySQL: 127.0.0.1:33306
  Redis: 127.0.0.1:36379
```

The five services are peers on the shared `admin-platform` Docker network. No service uses `host.docker.internal` after cutover.

## Repository and file ownership

### Backend repository

- `deploy/docker-state/docker-compose.yml` owns MySQL, Redis, named volumes, health checks, and the shared network.
- `deploy/docker-state/admin-state.env.example` documents non-secret state settings.
- Ignored `deploy/docker-state/admin-state.env` contains local MySQL credentials and is never printed, staged, or committed.
- Existing `deploy/docker-first/docker-compose.yml` becomes the `admin-app` project: Redis is removed, frontend is added, and API/worker join the shared external network.
- Existing ignored `deploy/docker-first/admin-go.env` remains the backend runtime env and changes to `mysql:3306` plus `redis:6379`.
- A PowerShell lifecycle script starts state, waits for health, then starts the application project. Stop/restart commands do not remove volumes.

### Frontend repository

- `Dockerfile` uses a pinned Node build stage and pinned Nginx runtime stage.
- `.dockerignore` excludes `node_modules`, `dist`, Git metadata, Tauri build output, logs, and local caches.
- `deploy/nginx.conf` serves static assets, applies Vue history fallback, and proxies HTTP/WebSocket routes to `admin-api:8080`.
- Build arguments supply the public API/WebSocket origins. Local builds use `http://localhost:5173` and `ws://localhost:5173/api/admin/v1/realtime/ws`; production supplies its real HTTPS/WSS origin.

The application Compose build context points to the sibling `admin_front_ts` checkout. Production deployment uses tagged frontend/backend images rather than source bind mounts.

## MySQL migration and cutover

The existing P02 database must not be adopted by raw volume relabeling or deleted during this work.

1. Confirm the existing recovery artifact and current schema/data acceptance checks.
2. Start the new Compose MySQL on temporary loopback port `33307` with a new named volume.
3. Restore the verified logical recovery artifact into the new MySQL.
4. Compare required schema fingerprint/data invariants and confirm direct host login.
5. Stop the existing backend briefly, stop `admin-p02-mysql`, release port `33306`, and start the verified state MySQL on `33306`.
6. Regenerate the backend runtime env for Docker DNS (`mysql:3306`, `redis:6379`) and start the application project.
7. Keep `admin-p02-mysql`, its anonymous volume, state file, and recovery artifact intact for rollback. Do not run `docker rm -v` or `docker compose down -v`.

If any pre-cutover validation fails, the old database remains active. If post-cutover readiness fails, stop the new state MySQL, restart `admin-p02-mysql`, restore the previous runtime env, and restart the current backend stack.

## Redis migration

Redis receives a state-owned named volume with AOF persistence and a health check. Local Redis data is cache/session/queue state rather than the MySQL truth source. Before cutover, the current Redis is asked to persist; its data is copied or restored into the new state volume only if the persistence format and version match. Otherwise the controlled local cutover starts an empty Redis and explicitly records that users must log in again and transient queues are reset. The old Redis volume remains available until acceptance completes.

## Startup, failure handling, and health

- State starts first. MySQL must answer an authenticated query and Redis must answer `PING`.
- Application startup begins only after state health passes.
- API retains `/health` and `/ready`; `/ready` proves MySQL, Redis, token Redis, and queue Redis connectivity.
- Worker uses `restart: unless-stopped` and must remain running with zero restart loops.
- Frontend health checks the Nginx root page. Nginx returns `502` for unavailable backend routes instead of silently serving the Vue fallback.
- The local lifecycle script fails closed on missing env files, occupied ports, failed restore checks, or unhealthy services.

## Security and production direction

- MySQL and Redis local host ports bind only to `127.0.0.1`; production state ports are not published publicly.
- Secrets remain in ignored owner-only env files or production secret management. They are not Docker build arguments, image layers, Compose command strings, logs, or Git history.
- Frontend artifacts contain only public `VITE_*` values.
- Application deployment can rebuild/recreate `admin-app` without stopping `admin-state`.
- Database backup and restore are state-stack operations, not application release steps.
- TLS terminates at the production ingress/reverse proxy; local validation remains HTTP on loopback.

## Verification

1. Frontend lint, typecheck, unit tests, and production build pass before image creation.
2. Backend full Go tests and Docker build pass.
3. Both Compose models render successfully and all images use pinned versions.
4. All five containers are running; MySQL, Redis, API, and frontend health checks pass; worker has no restart loop.
5. `5173`, `8080`, `33306`, and `36379` listen only on the intended loopback addresses.
6. Frontend root, Vue history fallback, API proxy, and WebSocket upgrade path are tested through the frontend origin.
7. `/health`, `/ready`, and login configuration return successful responses.
8. Navicat-equivalent host MySQL login reaches the `admin` schema on `33306`.
9. Existing FlyEnv MySQL `3306` and Redis `6379` remain untouched.
10. Both repositories finish with reviewed, intentional commits and no secret-bearing files staged.

## Out of scope

- Containerizing `canvas_front_next`.
- Applying P02 reconciliation or destructive SQL.
- Deleting the existing P02 MySQL container, anonymous volume, recovery artifact, or old Redis volume.
- Configuring public DNS, certificates, firewall rules, or a production container registry in this local cutover.
