# Access Log Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a minimal Gin access log middleware after RequestID and before module handlers.

**Architecture:** Keep logging as a cross-cutting middleware in `internal/middleware`. `server.NewRouter` owns middleware order and receives the logger from `bootstrap`; modules and services do not log HTTP access details.

**Tech Stack:** Go, Gin, `log/slog`, standard `httptest` tests.

---

## File Structure

- Create `internal/middleware/access_log.go`: Gin middleware that logs one structured line after each request.
- Create `internal/middleware/access_log_test.go`: verify method/path/status/request_id are logged.
- Modify `internal/server/router.go`: add `Dependencies.Logger` and install `AccessLog` after `RequestID`.
- Modify `internal/server/router_test.go`: verify router wiring emits access log and keep existing endpoint tests quiet with discard logger.
- Modify `internal/bootstrap/app.go`: pass bootstrap logger into router dependencies.
- Modify `docs/architecture.md` and `internal/middleware/README.md`: record middleware order and logging fields.

## Task 1: Middleware behavior

- [ ] Write failing test for `middleware.AccessLog(logger)`.
- [ ] Run `go test ./internal/middleware`; expected RED because `AccessLog` does not exist.
- [ ] Implement `AccessLog` with `slog.Logger`, `c.Next()`, status, method, path, latency, client_ip, and request_id.
- [ ] Run `go test ./internal/middleware`; expected PASS.

## Task 2: Router integration

- [ ] Write failing router test that injects a buffer logger and confirms `/health` emits access log.
- [ ] Run `go test ./internal/server`; expected RED because router dependency does not include logger and middleware is not wired.
- [ ] Add `Dependencies.Logger` and install middleware in order: Recovery, RequestID, AccessLog.
- [ ] Pass `logger` from `bootstrap.New` into `server.NewRouter`.
- [ ] Run `go test ./internal/server ./internal/bootstrap`; expected PASS.

## Task 3: Docs and verification

- [ ] Update middleware docs with current order and fields.
- [ ] Run `gofmt -w cmd internal`.
- [ ] Run `go mod tidy`.
- [ ] Run `go test ./...`.
- [ ] Start API on `:18080`, call `/health`, confirm endpoint still works.
- [ ] Confirm `E:\admin_go\admin_front_ts` remains clean.
