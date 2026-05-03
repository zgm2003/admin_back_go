# Go captcha login plan

## Goal

Move the password login path to a fail-closed CAPTCHA boundary before credential lookup.

## Scope

```text
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/login
```

Out of scope:

```text
email / phone code login
auto register
async login log queue
legacy PHP captcha compatibility
```

## Backend steps

1. Add `internal/module/captcha` with `route -> handler -> service -> store/engine`.
2. Use `github.com/wenlng/go-captcha/v2` slide mode and `go-captcha-assets`.
3. Store only the answer in Redis with `CAPTCHA_REDIS_PREFIX` and `CAPTCHA_TTL`.
4. Verify with Redis `GETDEL`, so each challenge is consumed once.
5. Inject `captcha.Verifier` into `auth.Service`; login rejects missing/invalid captcha before credential lookup.
6. Keep handler free of Redis/DB access and service free of `gin.Context`.

## Frontend steps

1. Add typed captcha DTOs to `src/types/user.ts`.
2. Add `UsersApi.getCaptcha()` for `GET /api/admin/v1/auth/captcha`.
3. Add a small `LoginSlideCaptcha.vue` component.
4. Add password-login captcha state to `useLoginForm`.
5. Refresh captcha after failed password login because the server consumes every attempt.

## Verification

```text
go test ./...
go vet ./...
npx eslint targeted login files
```

