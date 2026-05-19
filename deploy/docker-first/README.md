# Backend Docker-first Deploy Assets

This directory is the canonical Docker-first deployment entry for `admin_back_go`.

## Scope

- Runs one backend image as two containers: `admin-api` and `admin-worker`.
- Does not deploy the Vue frontend.
- Does not create MySQL or Redis by default.
- Designed for Baota Docker / Docker Compose.


## Local Docker start

This workspace has ignored local files for direct Docker startup:

```text
.env          # Docker Compose project env, local paths/ports
admin-go.env  # backend runtime env, backend-only smoke by default
runtime/      # local mounted runtime files
exports/      # local mounted export files
```

Start locally from this directory:

```powershell
cd E:/admin_go/admin_back_go/deploy/docker-first
docker compose up -d --build
docker compose ps
```

Validate:

```powershell
curl.exe http://127.0.0.1:8080/health
curl.exe http://127.0.0.1:8080/ready
```

If Docker Hub token fetch times out during build, edit local `.env` and point `GO_BUILD_IMAGE` / `GO_RUNTIME_IMAGE` to your reachable registry mirror, or pre-pull and tag the two base images locally.

Local `admin-go.env` starts in backend-only smoke mode: `MYSQL_DSN` and `REDIS_ADDR` are empty, `QUEUE_ENABLED=false`, and `SCHEDULER_ENABLED=false`, so the containers can start even when local MySQL/Redis are not running. For full business smoke, set `MYSQL_DSN=root:@tcp(host.docker.internal:3306)/admin?...`, `REDIS_ADDR=host.docker.internal:6379`, then turn queue/scheduler back on. If MySQL/Redis are Dockerized, they still belong to a separate state project; do not add them to this backend Compose file.

## Server paths

```text
/www/project/admin_back_go                 # backend source checkout
/www/docker/admin-go-backend               # compose working directory
/www/docker/admin-go-backend/.env          # docker compose project env
/www/docker/admin-go-backend/admin-go.env  # backend runtime env file
/www/docker/admin-go-backend/runtime       # mounted to /app/runtime
/www/docker/admin-go-backend/exports       # mounted to /app/exports
```

## Start

```bash
mkdir -p /www/docker/admin-go-backend/runtime /www/docker/admin-go-backend/exports
cp /www/project/admin_back_go/deploy/docker-first/docker-compose.yml /www/docker/admin-go-backend/docker-compose.yml
cp /www/project/admin_back_go/deploy/docker-first/compose.env.example /www/docker/admin-go-backend/.env
cp /www/project/admin_back_go/deploy/docker-first/admin-go.env.example /www/docker/admin-go-backend/admin-go.env
chmod 600 /www/docker/admin-go-backend/.env /www/docker/admin-go-backend/admin-go.env
chown -R 10001:10001 /www/docker/admin-go-backend/runtime /www/docker/admin-go-backend/exports
cd /www/docker/admin-go-backend
docker compose up -d --build
```

## Validate

```bash
docker compose ps
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

`.env` is for Docker Compose path/port variables. `admin-go.env` is for backend runtime variables.

`/health` only proves the process is alive. `/ready` proves MySQL, Redis, token Redis, queue Redis, and realtime configuration are usable.
