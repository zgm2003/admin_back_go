# CORS Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a typed, tested CORS middleware for the Go admin API without hand-writing fragile CORS logic.

**Architecture:** Use `github.com/gin-contrib/cors` under `internal/middleware`. Keep CORS configuration in `internal/config`, install it in `server.NewRouter` after AccessLog, and pass typed config from `bootstrap.New`.

**Tech Stack:** Go, Gin, gin-contrib/cors, `httptest` tests.

---

## File Structure

- Modify `internal/config/config.go`: add `CORSConfig`, defaults, and env parsing.
- Modify `internal/config/config_test.go`: test defaults and env overrides.
- Create `internal/middleware/cors.go`: wrap gin-contrib/cors.
- Create `internal/middleware/cors_test.go`: test preflight and exposed request id.
- Modify `internal/server/router.go`: add CORS dependency and middleware order.
- Modify `internal/server/router_test.go`: test router-level CORS preflight and access log compatibility.
- Modify `internal/bootstrap/app.go`: pass `cfg.CORS` into router dependencies.
- Modify docs: record CORS defaults, env names, and scope.

## Task 1: Tests first

- [x] Write config tests for default local origins and frontend headers.
- [x] Write middleware tests for OPTIONS preflight and exposed `X-Request-Id`.
- [x] Write router test proving CORS is wired after AccessLog.
- [x] Run targeted tests and confirm RED.

## Task 2: Implementation

- [x] Add `CORSConfig` and env parsing.
- [x] Add `middleware.CORS` backed by `github.com/gin-contrib/cors`.
- [x] Add router/bootstrap wiring.
- [x] Run targeted tests and confirm GREEN.

## Task 3: Verification

- [ ] Update docs.
- [ ] Run `gofmt -w cmd internal`.
- [ ] Run `go mod tidy`.
- [ ] Run `go test ./...`.
- [ ] Run HTTP preflight smoke against `/health`.
- [ ] Confirm frontend repo remains clean.
