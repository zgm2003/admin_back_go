# Local Docker Redis Isolation Design

**Date:** 2026-07-16  
**Scope:** Local Docker-first validation only

## Goal

Run Redis in Docker for the local backend without using or occupying the FlyEnv Redis instance on host port `6379`. Keep the existing P02 MySQL container and the local Vite frontend unchanged.

## Chosen design

- Add a `redis` service to `deploy/docker-first/docker-compose.yml`.
- Use `docker.m.daocloud.io/library/redis:8.2.7-alpine` and persist data in a named Compose volume.
- Bind Redis only to `127.0.0.1:36379`; the host's `6379` remains available to FlyEnv.
- Backend containers connect over the Compose network with `REDIS_ADDR=redis:6379`. The container port remains Redis's standard `6379`; only the host-facing port is changed.
- Add a Redis health check. `admin-api` and `admin-worker` wait for Redis to become healthy before starting.
- Keep P02 MySQL separate at `127.0.0.1:33306`; backend containers reach it through `host.docker.internal:33306`.
- Keep the frontend outside Compose and run Vite on `http://localhost:5173`.

## Alternatives considered

1. **Redis in the backend Compose project (chosen):** one lifecycle command, service-name networking, health-gated startup, and clean host-port isolation.
2. **A standalone `docker run` Redis container:** avoids editing Compose but creates a second lifecycle and manual network wiring.
3. **No host Redis port:** strongest isolation, but prevents convenient host-side diagnostics with Redis clients.

## Data and dependency flow

```text
Vite :5173 -> admin-api :8080
                  |-> P02 MySQL via host.docker.internal:33306
                  `-> Compose Redis via redis:6379

Host diagnostics -> Redis via 127.0.0.1:36379
FlyEnv Redis      -> remains on 127.0.0.1:6379
```

## Failure handling

- Compose must fail readiness when Redis cannot answer `PING`.
- Backend readiness must continue to prove MySQL, Redis, token Redis, and queue Redis connectivity.
- Redis binds to loopback only, so it is not exposed on LAN interfaces.
- The existing ignored runtime env remains secret-bearing and must never be printed or committed.

## Verification

1. Validate the rendered Compose model before startup.
2. Prove host ports `6379` and `36379` belong to different processes/containers.
3. Verify Redis `PING` through both the container health check and host port `36379`.
4. Verify `admin-api` health and readiness on `127.0.0.1:8080`.
5. Verify Vite responds on `localhost:5173` and calls the backend on `8080`.
6. Confirm MySQL remains reachable for Navicat at `127.0.0.1:33306`.

## Out of scope

- Replacing or stopping FlyEnv Redis.
- Moving the P02 MySQL container into this Compose project.
- Containerizing the frontend.
- Changing production/Baota deployment topology.
