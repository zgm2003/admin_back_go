# Auth Session Policy Simplification Design

## Goal

Remove two misleading choices from Admin authentication UX. Login persistence is
controlled by the authentication platform refresh TTL, while concurrent login
capacity is controlled by one authoritative `max_sessions` value.

## Login Interaction

- Remove the `记住我` checkbox from every login method.
- After a successful login, always persist only the normalized account and login
  type in device preferences.
- Never persist passwords, verification codes, access tokens, or refresh tokens.
- Access tokens remain memory-only. The HttpOnly refresh cookie and the platform
  `refresh_ttl` continue to control restoration after reload or browser restart.

## Session Policy Interaction

Replace `单端登录（互踢）` and the always-visible maximum input with one segmented
`会话策略` control:

- `单端登录`: `max_sessions = 1`; a new login revokes older platform sessions and
  strict Redis single-session pointer checks remain active.
- `限制数量`: show a numeric input from 2 through 100; a new login revokes the
  oldest sessions above the limit.
- `不限制`: `max_sessions = 0`; login does not revoke sessions for capacity.

The list page shows one `会话策略` column instead of separate single-session and
maximum columns.

## Backend And Data Contract

- Remove `single_session` from management requests, responses, service DTOs, and
  the generated Admin contract.
- Derive strict single-session behavior from `max_sessions == 1` inside the auth
  policy provider. Session lifecycle remains transport-neutral.
- Remove the redundant `auth_platforms.single_session` column from the canonical
  schema through a forward-only Atlas migration and apply the same DDL locally.
- Preserve existing behavior by keeping each row's current `max_sessions`; the
  local Admin row therefore remains strict single-session with value `1`.

## Validation

- Backend tests cover policy derivation for `0`, `1`, and values greater than 1,
  request/response closure, migration integrity, and session eviction behavior.
- Frontend tests cover unconditional non-secret account persistence, absence of
  the checkbox, mode/value mapping, conditional numeric input, and request shape.
- Regenerate and synchronize the Admin contract, run full backend/frontend gates,
  and verify the running `admin-dev` form and login page in a browser.
