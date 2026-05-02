# Users Init RBAC Read Path Design

## Scope

Implement only the legacy-compatible `POST /api/Users/init` read path in `admin_back_go`.

This endpoint is the first Go-owned RBAC read boundary for the existing frontend. It must return the current user's profile, role name, menu tree, dynamic routes, button codes, and quick entries. It must not implement login, captcha, registration, login logs, permission-check middleware, or frontend changes.

## Linus three questions

1. Real problem: yes. The frontend already calls `/api/Users/init` to build menus, routes, and buttons after auth.
2. Simpler path: keep the legacy endpoint as an adapter, compute RBAC in a small permission service, and keep HTTP parsing in the handler.
3. Breakage risk: preserve the existing response shape and use the trusted `AuthToken` identity platform, not headers.

## Contract

Endpoint:

```text
POST /api/Users/init
```

Auth:

```text
Requires AuthToken middleware. This path is not public.
```

Response data:

```json
{
  "user_id": 1,
  "username": "admin",
  "avatar": "",
  "role_name": "管理员",
  "permissions": [],
  "router": [],
  "buttonCodes": [],
  "quick_entry": []
}
```

## Architecture

Use the existing Gin modular monolith boundary:

```text
route -> handler -> service -> repository -> model
```

`handler` reads `middleware.AuthIdentity` and returns `response.OK/Error`.
`user.Service` loads user/profile/role/quick entries and calls `permission.Service`.
`permission.Service` computes menu/router/button context from role permissions and active permissions.
Repositories own GORM queries only.

## RBAC rules kept from legacy

- Single role: `users.role_id`.
- Permission types: DIR=1, PAGE=2, BUTTON=3.
- Active permissions query: `is_del = 2 AND status = 1`, ordered by `parent_id, sort, id`.
- Role grants query: `role_permissions.is_del = 2`.
- Granted PAGE/BUTTON nodes imply ancestors by walking parent_id to root.
- PAGE with `path` and `component` produces dynamic route.
- DIR/PAGE produces menu tree.
- BUTTON with `code` produces `buttonCodes`.
- `show_menu` is output metadata only; it does not remove page permission.
- Button cache key stays `auth_perm_uid_{userId}_{platform}_rbac_page_grants`.

## Error behavior

- Missing/invalid auth is handled by existing `AuthToken`.
- Missing current user returns 404.
- Invalid platform returns 100 business error.
- Repository failures return 500.
- Redis button-code cache write is best-effort and must not fail the endpoint.

## go-captcha placement

`go-captcha` belongs to the future login slice. It is deliberately not added here because login also touches captcha state, user registration/default role, login logs, token issuing, and platform policy.
