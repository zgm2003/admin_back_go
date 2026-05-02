# Refresh Logout Session Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add refresh-token rotation and logout revocation on top of the existing session authenticator, without implementing full login or RBAC.

**Architecture:** Keep `AuthToken` middleware thin. Put token generation, refresh hash lookup, session rotation, token cache deletion, and logout revocation in `internal/module/session`. Put HTTP request parsing in `internal/module/auth` and register both new `/api/v1/auth/*` and legacy-compatible `/api/Users/*` refresh/logout routes.

**Tech Stack:** Go, Gin, GORM, go-redis, existing config/resources/response/app-error packages.

---

## File Structure

- Modify `internal/module/session/service.go`: add refresh/logout behavior and token generation.
- Modify `internal/module/session/repository.go`: add refresh lookup, rotate, revoke methods.
- Modify `internal/module/session/model.go`: map refresh token hash, UA, refresh expiration.
- Create `internal/module/auth/*`: refresh/logout HTTP handler and routes.
- Modify `internal/server/router.go`: mount auth routes.
- Modify `internal/middleware/auth_token.go`: expose Bearer parser and public refresh skip paths.
- Modify `internal/bootstrap/app.go`: reuse one session authenticator for middleware and auth routes.
- Update docs.

## Task 1: Tests first

- [x] Add session refresh rotation, expired refresh, stale single-session, and logout revocation tests.
- [x] Add auth handler tests for missing refresh token, refresh response, and logout Bearer parsing.
- [x] Add router test proving refresh route is public and not intercepted by `AuthToken`.
- [x] Run targeted tests and confirm RED.

## Task 2: Implementation

- [x] Implement refresh token hash lookup and session rotation.
- [x] Implement access token cache deletion and single-session pointer handling.
- [x] Implement logout revoke + token/pointer cache cleanup.
- [x] Implement `auth` module routes and handlers.
- [x] Register `/api/v1/auth/refresh`, `/api/v1/auth/logout`, `/api/Users/refresh`, `/api/Users/logout`.
- [x] Keep refresh public and logout protected.

## Task 3: Verification

- [x] Run `gofmt -w cmd internal`.
- [x] Run `go test ./...`.
- [x] Run empty-env smoke for health/ready/refresh/logout.
- [x] Run masked legacy `.env` readiness smoke.
- [x] Confirm frontend repo remains clean.

