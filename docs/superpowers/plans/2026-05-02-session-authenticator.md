# Session Authenticator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect `AuthToken` to a minimal session authenticator that can resolve existing PHP tokens through Redis and MySQL.

**Architecture:** Keep HTTP token parsing in `internal/middleware.AuthToken`. Put token hash, Redis token-session cache parsing, and `user_sessions` lookup in `internal/module/session`. Wire the authenticator in `internal/bootstrap` using existing `Resources`.

**Tech Stack:** Go, Gin, GORM, go-redis, existing app error/response packages.

---

## File Structure

- Create `internal/module/session/token.go`: legacy `sha256(token + "|" + TOKEN_PEPPER)` hash.
- Create `internal/module/session/model.go`: `user_sessions` model and Redis payload parser/serializer.
- Create `internal/module/session/cache.go`: Redis cache adapter.
- Create `internal/module/session/repository.go`: GORM `user_sessions` repository.
- Create `internal/module/session/service.go`: session authenticator service.
- Create `internal/module/session/service_test.go`: cache hit, DB fallback, expired cache, fail-closed cases.
- Create `internal/bootstrap/authenticator.go`: adapter from middleware token input to session service.
- Create `internal/bootstrap/authenticator_test.go`: no resources fail closed.
- Modify `internal/config/config.go`: add token Redis prefix and session cache TTL.
- Update docs.

## Task 1: Tests first

- [x] Write session authenticator tests.
- [x] Write token config tests.
- [x] Run targeted tests and confirm RED.

## Task 2: Implementation

- [x] Implement legacy token hash.
- [x] Implement session model/cache/repository/service.
- [x] Wire bootstrap authenticator into router dependencies.
- [x] Fix typed-nil interface bug for nil Redis/DB adapters.

## Task 3: Verification

- [x] Update docs.
- [x] Run `gofmt -w cmd internal`.
- [x] Run `go mod tidy`.
- [x] Run `go test ./...`.
- [x] Smoke public endpoints and protected endpoint behavior.
- [x] Confirm frontend repo remains clean.

