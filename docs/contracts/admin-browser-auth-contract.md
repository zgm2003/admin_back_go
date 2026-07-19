# Admin Browser authentication transport contract

Status: active formal contract

Activation marker:

```text
bundle_version=admin-2026-07-15.1
backend_source_commit=5e6915c6d415daf0cc1fb53fe08767aff1ec77b6
manifest_sha256=d0a7649f4fe22ac5a095a108e7c8969fa1a626dea50fdf82f1fa19dfc0b8b1fa
```

The checked-in generated bundle is the machine-readable current truth for this
Browser-only transport. Frontend authentication consumes that exact manifest;
historical browser/desktop variants are not compatibility inputs.

## Shared transport rules

- The only Admin client is a browser served from an origin explicitly listed in
  `CORS_ALLOW_ORIGINS`.
- `Origin` is mandatory and must match one configured origin exactly after
  canonical scheme/host normalization. Missing, malformed, unlisted, opaque,
  and historical Tauri origins are rejected.
- `platform` is exactly `admin`; `device-id` is a non-empty browser device ID.
- The access credential is returned to the caller and remains browser memory
  state. The refresh credential exists only in the scoped HttpOnly Cookie.
- The Cookie name and attributes are exactly:

  ```text
  __Secure-admin_refresh; Path=/api/admin/v1/auth; HttpOnly; Secure; SameSite=Strict
  ```

- `X-Admin-Client-Variant`, `ClientVariant`, `browser|desktop`, desktop
  origins, JSON `refresh_token`, public `refresh_expires_in`, and User-Agent
  inference do not exist in this contract.
- Success data is a closed object. Undocumented properties are contract
  violations; clients must report them instead of accepting aliases or fallback
  fields.

## Login

```text
POST /api/admin/v1/auth/login
Headers: Origin, platform=admin, device-id, Content-Type=application/json

Password body:
{"login_type":"password","login_account":"string","password":"string"}

Code body:
{"login_type":"email|phone","login_account":"string","code":"6 digits"}

Success data:
{"access_token":"string","expires_in":positive integer}

Side effect:
Set-Cookie __Secure-admin_refresh; Path=/api/admin/v1/auth; HttpOnly; Secure; SameSite=Strict
```

Password login does not generate or consume captcha proof. Captcha remains
mandatory for every send-code flow under its separately published contract.

## Refresh

```text
POST /api/admin/v1/auth/refresh
Headers: Origin, platform=admin, device-id
Body: forbidden
Credential input: __Secure-admin_refresh Cookie only

Success data:
{"access_token":"string","expires_in":positive integer}

Side effect:
Rotate __Secure-admin_refresh with the same Path/HttpOnly/Secure/SameSite attributes
```

No request body is valid. Content-Length greater than zero or any transfer
encoding is rejected before Cookie lookup or credential rotation. This includes
`{}`, `{"refresh_token":"..."}`, whitespace, and every other payload.

## Logout

```text
POST /api/admin/v1/auth/logout
Headers: Origin, Authorization=Bearer <access>, platform=admin, device-id
Body: forbidden

Success data:
{}

Side effects:
Revoke the authenticated session.
Expire __Secure-admin_refresh with the same Path/HttpOnly/Secure/SameSite attributes.
```

Logout rejects a body before session revocation. Cookie expiry is emitted only
through this Browser transport; the service/session lifecycle remains transport
neutral.

## Exact failure contract

All failures use the shared classified error envelope. They are not converted
to success or empty data.

| Condition | HTTP | `error.code` | Category | Retryable |
| --- | ---: | --- | --- | --- |
| Missing, malformed, or unapproved `Origin` | 403 | `auth.origin_forbidden` | `authorization` | `false` |
| Any refresh request body, including `{}` | 400 | `request.invalid` with message ID `auth.refresh_body_forbidden` | `validation` | `false` |
| Any logout request body, including `{}` | 400 | `request.invalid` with message ID `auth.logout_body_forbidden` | `validation` | `false` |
| Missing or empty refresh Cookie | 401 | `auth.unauthenticated` | `authentication` | `false` |
| Unknown, expired, revoked, or already rotated refresh credential | 401 | `auth.refresh_reused` | `authentication` | `false` |
| Deleted or inactive user behind an otherwise valid refresh session | 401 | `auth.unauthenticated` | `authentication` | `false` |
| Missing or invalid logout Bearer credential | 401 | `auth.unauthenticated` | `authentication` | `false` |

If a nominally successful response is not the exact closed success shape above,
the frontend enters its explicit contract-error state. It must not guess a
field, preserve stale authentication data, or relabel the violation as an empty
or offline state.

## Unchanged session policy

This transport change does not change `auth_platforms.single_session=1` or
`max_sessions=1`. Login and refresh continue to use the existing MySQL session
truth, isolated token Redis cache, refresh rotation, device/platform binding,
and session revocation rules.

## Retirement boundary

P08R removes the client-version route/menu/permission/UI/updater runtime but
does not mutate or delete `client_versions` rows or historical COS objects. The
table stays frozen until P09 restore verification and a new explicit approval
for destructive DDL.
