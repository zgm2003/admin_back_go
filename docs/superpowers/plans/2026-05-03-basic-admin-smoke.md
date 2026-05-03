# Basic admin smoke gate

## Goal

Make "basic admin is OK" reproducible instead of relying on manual claims.

This smoke covers only the current core admin bootstrap:

```text
readiness -> login-config -> go-captcha -> password login -> users/me -> users/init -> permission create/delete -> logout
```

## Non-goals

```text
email / phone code login
registration
frontend visual regression
legacy PHP feature migration
full CRUD migration
```

## Script

```text
scripts/basic-admin-smoke.ps1
```

Usage:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\basic-admin-smoke.ps1 `
  -Account <test-account> `
  -Password <test-password>
```

The script deliberately does not hardcode credentials. CI or local users can also provide:

```text
SMOKE_LOGIN_ACCOUNT
SMOKE_LOGIN_PASSWORD
```

## Contract checked

```text
/ready
GET  /api/admin/v1/auth/login-config
GET  /api/admin/v1/auth/captcha
POST /api/admin/v1/auth/login
GET  /api/admin/v1/users/me
GET  /api/admin/v1/users/init
POST /api/admin/v1/permissions
DELETE /api/admin/v1/permissions/:id
POST /api/admin/v1/auth/logout
```

## Design rules

```text
Do not bypass captcha in the login service.
Do not add a fake testing endpoint.
Do not hardcode test account/password into the repository.
Do not mutate DB schema for smoke.
Do not leave the smoke session active after success.
Do not leave the temporary smoke permission active after success.
```

The only test-only behavior is that the script reads the generated captcha answer from Redis so the browser-style slide interaction can be automated. Production HTTP code still sees a normal captcha_id + captcha_answer login request.

## Verification command

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\basic-admin-smoke.ps1 -Account 15671628271 -Password 123456
```

Latest verified result:

```json
{
  "ready_code": 0,
  "login_config_code": 0,
  "captcha_code": 0,
  "captcha_type": "slide",
  "login_code": 0,
  "access_token_present": true,
  "me_code": 0,
  "init_code": 0,
  "router_count": 38,
  "button_code_count": 65,
  "permission_create_code": 0,
  "permission_delete_code": 0,
  "logout_code": 0
}
```

