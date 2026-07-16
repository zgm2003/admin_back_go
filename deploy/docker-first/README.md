# Backend Docker-first Compose Assets

This directory contains the backend Docker-first Compose asset and shared env template.

Production/Baota Docker operation is documented in the root runbook: `E:/admin_go/docs/deployment/docker-first-backend.md`. Use that root runbook for server layout, lifecycle, and release steps; use this directory for the actual backend Compose files and local checks.

## Scope

- Runs one backend image as two containers: `admin-api` and `admin-worker`.
- Creates a persistent Redis container for local Docker-first validation.
- Does not deploy the Vue frontend.
- Does not create MySQL; the P02 MySQL container remains a separate state service.
- Uses `admin-go.env` as the only backend runtime env file.

## Current defaults

`docker-compose.yml` is the local validation Compose asset and does not require a Compose `.env` file. `admin-go.env.example` is the shared, production-oriented backend runtime env template. Generate the ignored `admin-go.env` locally with `init-local-env.ps1`; use the template and root Docker-first runbook for production.

```text
build context: ../..
env file:      ./admin-go.env
api port:      127.0.0.1:8080 -> container 8080
redis port:    127.0.0.1:36379 -> container 6379
redis address: redis:6379 from backend containers
runtime mount: ./runtime -> /app/runtime
exports mount: ./exports -> /app/exports
redis volume:  redis-data -> /data
```

If `8080` is occupied, edit the `ports` line in `docker-compose.yml` directly.

## Start locally from PowerShell

Set the credential-bearing MySQL DSN process environment variable before running this command. Inside containers, `127.0.0.1` is the container itself; the local backend reaches P02 MySQL at `host.docker.internal:33306` and reaches the Compose Redis service at `redis:6379`. Host-side Redis diagnostics use `127.0.0.1:36379`, leaving FlyEnv Redis isolated on `127.0.0.1:6379`.

`ADMIN_LOCAL_MYSQL_DSN` must use the Compose-safe canonical MySQL DSN form `SAFE_USER:SAFE_PASSWORD@tcp(HOST:PORT)/admin?charset=utf8mb4&parseTime=True&loc=Local`. `SAFE_USER` and `SAFE_PASSWORD` may contain only ASCII letters, digits, `.`, `_`, `~`, and `-`; the host must be a valid hostname, IPv4 address, or bracketed IPv6 address, and the port must be `1..65535`. The initializer rejects whitespace, `$`, `#`, quotes, backticks, backslashes, alternate query options, and other forms that a Compose `env_file` could reinterpret.

```powershell
New-Item -ItemType Directory -Force -Path runtime, exports | Out-Null
$env:ADMIN_LOCAL_REDIS_ADDR = 'redis:6379'
.\init-local-env.ps1 `
  -MySQLDSN $env:ADMIN_LOCAL_MYSQL_DSN `
  -RedisAddress $env:ADMIN_LOCAL_REDIS_ADDR `
  -CorsOrigin 'http://localhost:5173'
docker compose up -d --build
```

The initializer is for local development only. With its default path it replaces the repository-ignored `admin-go.env` and reports `created ignored runtime env`. Custom `-OutputPath` values are allowed only outside the repository and report `created runtime env`, because the initializer cannot claim those files are covered by this repository's ignore rule. Reusable `APP_SECRET` values must contain at least 64 ASCII characters and may use only letters, digits, `.`, `_`, `~`, `+`, `/`, `=`, and `-`; other existing values are replaced so Compose cannot trim, quote, or expand them. Production continues to use `admin-go.env.example` and the root production runbook; do not use the local initializer as a production provisioning workflow.

## Validate

```bash
docker compose ps
docker compose exec -T redis redis-cli ping
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

`/health` only proves the process is alive. `/ready` proves configured MySQL, Redis, token Redis, and queue Redis are reachable and that realtime configuration is accepted. It is not a WebSocket upgrade or Redis Pub/Sub fan-out smoke.
