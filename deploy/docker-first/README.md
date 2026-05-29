# Backend Docker-first Compose Assets

This directory contains the backend Docker-first Compose asset and shared env template.

Production/Baota Docker operation is documented in the root runbook: `E:/admin_go/docs/deployment/docker-first-backend.md`. Use that root runbook for server layout, lifecycle, and release steps; use this directory for the actual backend Compose files and local checks.

## Scope

- Runs one backend image as two containers: `admin-api` and `admin-worker`.
- Does not deploy the Vue frontend.
- Does not create MySQL or Redis by default.
- Uses `admin-go.env` as the only backend runtime env file.

## Current defaults

`docker-compose.yml` is the local validation Compose asset and does not require a Compose `.env` file. `admin-go.env.example` is the shared backend runtime env template; copy it to `admin-go.env` locally, or to the production path described by the root Docker-first runbook.

```text
build context: ../..
env file:      ./admin-go.env
api port:      127.0.0.1:8080 -> container 8080
runtime mount: ./runtime -> /app/runtime
exports mount: ./exports -> /app/exports
```

If `8080` is occupied, edit the `ports` line in `docker-compose.yml` directly.

## Start

```bash
mkdir -p runtime exports
cp -n admin-go.env.example admin-go.env
# Edit admin-go.env before starting. Inside containers, 127.0.0.1 is the container itself;
# use private IP/DNS for state services, or host.docker.internal on Docker Desktop.
docker compose up -d --build
```

## Validate

```bash
docker compose ps
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

`/health` only proves the process is alive. `/ready` proves configured MySQL, Redis, token Redis, and queue Redis are reachable and that realtime configuration is accepted. It is not a WebSocket upgrade or Redis Pub/Sub fan-out smoke.
