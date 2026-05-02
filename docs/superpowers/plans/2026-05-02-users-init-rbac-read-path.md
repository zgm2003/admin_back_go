# Users Init RBAC Read Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `POST /api/Users/init` as a read-only legacy-compatible endpoint backed by Go RBAC context calculation.

**Architecture:** Keep `/api/Users/init` as a legacy route adapter. Handler reads auth identity only, service computes user init data without `gin.Context`, repositories own GORM, Redis button cache is best-effort.

**Tech Stack:** Go, Gin, GORM, go-redis, existing `apperror`, `response`, `middleware.AuthToken`.

---

## File Structure

- Create `internal/module/permission/model.go`: RBAC constants and DTO-like output structs.
- Create `internal/module/permission/repository.go`: role, role_permissions, permissions read queries.
- Create `internal/module/permission/service.go`: permission context builder and button cache key.
- Create `internal/module/permission/service_test.go`: TDD coverage for missing role, ancestor/menu/route/button calculation, root-level app button.
- Create `internal/module/user/model.go`: users/user_profiles/roles/users_quick_entry models.
- Create `internal/module/user/dto.go`: `InitResponse` contract.
- Create `internal/module/user/repository.go`: current user/profile/role/quick entry queries.
- Create `internal/module/user/service.go`: assemble init response and best-effort button cache.
- Create `internal/module/user/handler.go`: read auth identity and call service.
- Create `internal/module/user/route.go`: register `POST /api/Users/init`.
- Modify `internal/server/router.go`: wire user handler dependency.
- Modify `internal/bootstrap/app.go`: build repositories/services and pass to router.

## Task 1: Permission service TDD

- [x] Write failing tests for permission context behavior.
- [x] Run `go test ./internal/module/permission -count=1` and confirm RED.
- [x] Implement minimal permission service and repository models.
- [x] Run targeted tests and confirm GREEN.

## Task 2: User init service/handler TDD

- [x] Write failing tests for user init assembly and handler auth behavior.
- [x] Run targeted tests and confirm RED.
- [x] Implement minimal user service, handler, route, repository.
- [x] Run targeted tests and confirm GREEN.

## Task 3: Bootstrap and verification

- [x] Wire bootstrap and router dependencies.
- [x] Run `gofmt -w cmd internal`.
- [x] Run `go mod tidy`.
- [x] Run `go test ./...`.
- [x] Smoke `POST /api/Users/init` without token and confirm `401 缺少Token`.
- [x] Confirm `admin_front_ts` remains untouched.


