# AuthToken Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a thin AuthToken middleware boundary without migrating login/session/RBAC business yet.

**Architecture:** `internal/middleware.AuthToken` extracts `Authorization: Bearer <token>`, passes token plus request platform/device/IP to an injected authenticator, and stores the returned session identity in Gin context. Session lookup, token hash, Redis/DB fallback, platform policy, single-session policy, and RBAC remain future service responsibilities.

**Tech Stack:** Go, Gin, existing `apperror` and `response` packages.

---

## File Structure

- Create `internal/middleware/auth_token.go`: token extraction, public-path skipping, authenticator hook, auth identity context helpers.
- Create `internal/middleware/auth_token_test.go`: missing/malformed token, nil authenticator, public path skip, identity mount.
- Modify `internal/server/router.go`: install AuthToken after CORS with default public path skip.
- Modify `internal/server/router_test.go`: prove non-public paths require token while public endpoints still work.
- Modify docs: record AuthToken scope and what it deliberately does not own.

## Task 1: Tests first

- [x] Write middleware behavior tests.
- [x] Run `go test ./internal/middleware` and confirm RED.
- [x] Write router integration test.
- [x] Run `go test ./internal/server` and confirm RED.

## Task 2: Implementation

- [x] Implement AuthToken middleware and helpers.
- [x] Wire AuthToken into router after CORS.
- [x] Run targeted tests and confirm GREEN.

## Task 3: Verification

- [x] Update docs.
- [x] Run `gofmt -w cmd internal`.
- [x] Run `go mod tidy`.
- [x] Run `go test ./...`.
- [x] Smoke `/health`, `/ready`, and one non-public path without token.
- [x] Confirm frontend repo remains clean.

