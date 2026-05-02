# Health Readiness Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a minimal `/ready` endpoint that reports process readiness without turning `/health` into an external dependency check.

**Architecture:** Keep `/health` as a cheap liveness endpoint. Add a small `internal/readiness` contract for operational check results, let `bootstrap.Resources` own DB/Redis probing, and let `system` expose the result through Gin without importing concrete platform clients.

**Tech Stack:** Go, Gin, GORM MySQL client, go-redis, standard `net/http/httptest` tests.

---

## File Structure

- Create `internal/readiness/readiness.go`: neutral readiness DTO and status constants.
- Create `internal/readiness/readiness_test.go`: report aggregation behavior.
- Modify `internal/bootstrap/resources.go`: track configured resources, init errors, and ping DB/Redis on readiness checks only.
- Modify `internal/bootstrap/resources_test.go`: verify disabled checks and configured-but-unreachable resources.
- Modify `internal/module/system/dto.go`: expose ready response shape if needed by system tests.
- Modify `internal/module/system/service.go`: add `Ready(ctx)` and keep service free of `gin.Context`.
- Modify `internal/module/system/service_test.go`: verify service behavior with a fake checker.
- Modify `internal/module/system/handler.go`: add `Ready` handler with `response.OK` or error response with data.
- Modify `internal/module/system/route.go`: register `GET /ready`.
- Modify `internal/server/router.go`: inject readiness checker from bootstrap resources.
- Modify `internal/server/router_test.go`: verify `/ready` disabled resources returns code 0.
- Modify `internal/response/response.go` and tests only if readiness failure needs error details in `data`.
- Modify `docs/architecture.md`: record health/readiness split.

## Task 1: Readiness contract

- [ ] Write tests in `internal/readiness/readiness_test.go` for all-up and any-down aggregation.
- [ ] Run `go test ./internal/readiness`; expected RED because package/function does not exist.
- [ ] Implement `internal/readiness/readiness.go` with constants, `Check`, `Report`, and `NewReport`.
- [ ] Run `go test ./internal/readiness`; expected PASS.

## Task 2: Resource readiness checks

- [ ] Add tests in `internal/bootstrap/resources_test.go` for disabled DB/Redis checks and configured unreachable DB/Redis checks.
- [ ] Run `go test ./internal/bootstrap`; expected RED because `Readiness` does not exist.
- [ ] Implement `Resources.Readiness(ctx)` and internal DB/Redis check helpers.
- [ ] Run `go test ./internal/bootstrap`; expected PASS.

## Task 3: System module endpoint

- [ ] Add system service and router tests for `/ready`.
- [ ] Run `go test ./internal/module/system ./internal/server`; expected RED because `Ready` route is missing.
- [ ] Implement `Service.Ready(ctx)`, `Handler.Ready`, `RegisterRoutes(router, checker)`, and router dependency injection.
- [ ] If failure responses need details, add `response.ErrorWithData` with tests.
- [ ] Run `go test ./internal/module/system ./internal/server ./internal/response`; expected PASS.

## Task 4: Docs and verification

- [ ] Update `docs/architecture.md` with `/health` vs `/ready` rules.
- [ ] Run `gofmt -w cmd internal`.
- [ ] Run `go mod tidy`.
- [ ] Run `go test ./...`.
- [ ] Start the API with empty `MYSQL_DSN` and `REDIS_ADDR`, request `/health` and `/ready`, then stop it.
- [ ] Confirm `E:\admin_go\admin_front_ts` remains clean with `git -C ..\admin_front_ts status --short`.
