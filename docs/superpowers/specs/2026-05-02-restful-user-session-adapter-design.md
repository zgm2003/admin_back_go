# RESTful User Session Adapter Design

## Scope

Cut only the current-user session read path from the legacy `Users/init` contract to the Go REST API contract, then point the frontend session bootstrap at that contract.

This slice exists to make the frontend start consuming Go through a real RESTful boundary without dragging login, captcha, registration, login logs, user management, or personal-center mutations into the same change.

## Linus three questions

1. Real problem: yes. The frontend dynamic route, menu, button permission, and quick-entry bootstrap still depends on `POST /api/Users/init`.
2. Simpler path: add one Go-owned read endpoint and switch the frontend bootstrap call. Do not rewrite all user APIs.
3. Breakage risk: keep the response payload shape stable for the store and router. Do not change MySQL schema. Do not add frontend fallback routes.

## Decision

Use:

```text
GET /api/v1/users/me
```

as the new current authenticated user aggregate endpoint.

Why not `POST /api/Users/init`:

```text
It is legacy-compatible, but it is not the new contract.
```

Why not migrate every `/api/Users/*` call now:

```text
Most of those endpoints still belong to login, captcha, personal center, or user management slices. Pretending they are RESTful before Go owns them creates compatibility garbage.
```

Why not rename Redis keys in this slice:

```text
The user allowed Redis key renames, but this slice does not need one. Keep cache-key churn out unless the endpoint implementation actually needs it.
```

## Endpoint list

### New Go REST endpoint

```text
GET /api/v1/users/me
```

Auth:

```text
Requires AuthToken middleware.
Uses middleware.AuthIdentity.UserID and middleware.AuthIdentity.Platform.
No request body.
```

### Existing Go endpoint kept

```text
POST /api/Users/init
```

Purpose:

```text
Legacy-compatible adapter only. Keep it working during migration.
```

### Existing Go auth endpoints used by frontend refresh

```text
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
```

Refresh is public and receives `refresh_token`.
Logout is authenticated and revokes the current access-token session.

### Legacy auth endpoints kept for old callers

```text
POST /api/Users/refresh
POST /api/Users/logout
```

Purpose:

```text
Legacy-compatible adapter only.
```

## Request schema

### `GET /api/v1/users/me`

Headers:

```text
Authorization: Bearer <access_token>
platform: admin
device-id: <device id>
```

Body:

```text
none
```

Query:

```text
none
```

## Response schema

The response envelope remains the current Go envelope:

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "user_id": 1,
    "username": "admin",
    "avatar": "",
    "role_name": "管理员",
    "permissions": [],
    "router": [],
    "buttonCodes": [],
    "quick_entry": []
  }
}
```

The frontend store may continue reading:

```text
data.user_id
data.avatar
data.username
data.role_name
data.permissions
data.router
data.buttonCodes
data.quick_entry
```

No fallback fields are added.

## Error cases

```text
401 missing token
401 invalid or expired token
404 current user not found
100 invalid platform
500 repository or unexpected server failure
```

Redis button-cache writes remain best-effort. Cache write failure must not fail `GET /api/v1/users/me`.

## Legacy mapping

```text
POST /api/Users/init  -> GET /api/v1/users/me
POST /api/Users/refresh -> POST /api/v1/auth/refresh
POST /api/Users/logout  -> POST /api/v1/auth/logout
```

This mapping is a migration bridge. It is not permission to keep adding new legacy-style endpoints.

## Frontend impact

Change only the session/bootstrap HTTP boundary first:

```text
src/api/user/users.ts
  UsersApi.me()   -> request.get<UserInitResponse>('/api/v1/users/me')
  UsersApi.init() -> same implementation or store switches to UsersApi.me()

src/store/user.ts
  fetchUserInfo() keeps the current assignment logic.

src/lib/http/auth-session.ts
  refreshToken() posts to /api/v1/auth/refresh.
  401 self-check must recognize /api/v1/auth/refresh.
```

Do not modify:

```text
login
captcha
send code
forgot password
personal center mutations
user list management
quick-entry save
database schema
```

Those are separate slices.

## Verification

Backend:

```text
go test ./...
```

Frontend targeted checks:

```text
type check or build:check if the local Node process is healthy
static grep proving the bootstrap path no longer calls /api/Users/init
static grep proving refresh calls /api/v1/auth/refresh
```

Runtime smoke when services are available:

```text
1. Use the existing PHP login flow to obtain token.
2. Call GET /api/v1/users/me against Go with Bearer token.
3. Confirm code=0 and non-empty router/buttonCodes for the known admin account.
4. Confirm frontend dynamic route bootstrap still uses the returned router array.
```

## Non-goals

```text
No MySQL table changes.
No full OpenAPI generation yet.
No Go login implementation.
No go-captcha implementation.
No frontend visual rewrite.
No broad `/api/admin/*` migration.
No compatibility fallback in the frontend.
```

## Self-review result

No placeholders. Scope is one current-user read endpoint plus frontend bootstrap adaptation. The design keeps legacy adapters alive without letting legacy naming define the new API.
