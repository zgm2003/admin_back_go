# Auth Platform Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the existing session authenticator to legacy `auth_platforms` policy checks without moving RBAC into middleware.

**Architecture:** Keep `AuthToken` as a thin HTTP boundary. `internal/module/session` owns token/session authentication and defines the policy it needs. `internal/module/authplatform` reads `auth_platforms` and maps legacy YES/NO flags into session policy.

**Tech Stack:** Go, Gin, GORM, existing Redis/MySQL platform resources.

---

## File Structure

- Modify `internal/module/session/service.go`: enforce current platform, bind_platform, bind_device, bind_ip, single_session policy.
- Modify `internal/module/session/repository.go`: add latest active session lookup for single-session pointer rebuild.
- Modify `internal/module/session/model.go`: add `refresh_expires_at` mapping.
- Create `internal/module/authplatform/*`: read active platform policy from `auth_platforms`.
- Modify `internal/bootstrap/authenticator.go`: wire auth platform policy provider into session authenticator.
- Modify `internal/config/config.go`: add single-session pointer TTL.
- Update docs.

## Task 1: Tests first

- [x] Add session policy tests for invalid platform, platform mismatch, device mismatch, IP mismatch, stale single-session pointer, pointer rebuild.
- [x] Add config tests for `TOKEN_SINGLE_SESSION_POINTER_TTL`.
- [x] Add authplatform service tests for missing platform, legacy YES/NO mapping, repository errors.
- [x] Run targeted tests and confirm RED.

## Task 2: Implementation

- [x] Add `AuthPolicy` / `PolicyProvider` to session service.
- [x] Enforce platform/device/IP/single-session after session resolution.
- [x] Add latest active session repository lookup.
- [x] Add authplatform module and GORM repository.
- [x] Wire policy provider in bootstrap.
- [x] Keep AuthToken middleware thin; no RBAC logic added.

## Task 3: Verification

- [x] Run `gofmt -w cmd internal`.
- [x] Run `go test ./...`.
- [x] Run smoke endpoints with empty env.
- [x] Run readiness smoke with masked legacy `.env`.
- [x] Confirm frontend repo remains clean.

