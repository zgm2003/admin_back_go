# Admin Browser-only Tauri Retirement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Execute inline on the two existing `master` checkouts; do not create subagents or worktrees unless the user explicitly changes that rule.

**Goal:** Retire Tauri and the Admin browser/desktop split completely, leave one browser Cookie/Origin authentication transport, remove client-version runtime surfaces, and switch the Docker platform without changing Admin single-session policy.

**Architecture:** The backend formal contract changes first and is published as a new deterministic Admin Contract Bundle. The frontend then consumes that exact bundle, removes every Tauri/native/client-variant branch, and keeps only explicit browser capabilities. `client_versions` is frozen in P08R and is physically dropped only by P09 after restore proof and fresh user approval.

**Tech Stack:** Go 1.26.5, Gin, GORM, MySQL 8.4, Redis, Vue 3.5, TypeScript 5.9, Vite 8, Vitest 4, Docker Compose, PowerShell 7.

---

## Fixed execution boundary

- Work only in `E:/admin/admin_back_go` and `E:/admin/admin_front_ts`, both directly on `master`.
- Do not create Git worktrees or `.worktrees`; do not create `.github` or any GitHub Workflow.
- Web, API, Worker, MySQL, and Redis runtime verification uses only `admin_back_go/scripts/docker-platform.ps1` and Docker containers. Never start host Vite, Go API/Worker, MySQL, or Redis.
- Frontend package installation, generation, lint, typecheck, tests, and build run inside `node:22.23.1-alpine`; do not run host `npm`.
- Do not install, configure, or invoke Playwright.
- Read `admin_front_ts/docs/rule.md` before every frontend contract edit. The backend formal document and generated bundle are the only field contract; no alias, fallback, compatibility request, or runtime mock is allowed.
- P08 implementation commits remain historical evidence. P08R removes their runtime result instead of rewriting Git history.
- P08.5 is cancelled. No tag grammar, Windows runner, NSIS artifact, signing secret, COS candidate, candidate import, updater manifest, or promotion path may be created.
- Keep `auth_platforms.single_session=1` and `max_sessions=1` unchanged. P08R is not a multi-browser-session product change.

## Target invariants

1. `POST /api/admin/v1/auth/login`, `refresh`, and `logout` require an exact configured Web `Origin`.
2. Login and refresh success data contain exactly `access_token` and `expires_in`; refresh credentials exist only in the scoped `HttpOnly + Secure + SameSite=Strict` Cookie.
3. Refresh accepts no JSON request body. A body containing `refresh_token`, an empty object, or any other field is a contract error rather than an ignored compatibility input.
4. `X-Admin-Client-Variant`, `ClientVariant`, `browser|desktop`, desktop credential DTOs, and Tauri local origins are absent from the active contract and production code.
5. The client-version route family, permissions, view, menu, frontend API/page, and updater publisher are unreachable.
6. `client_versions` rows and historical COS objects are not modified or deleted in P08R.
7. Browser download, safe external navigation, same-origin queue-monitor opening, in-app notification, global online/offline UI, and ordinary preferences continue to work without a one-implementation `NativeBridge` abstraction.
8. The final frontend dependency graph contains no Rust/Tauri/NSIS package or script.

## File ownership map

- `docs/contracts/admin-browser-auth-contract.md` owns the human-readable browser authentication contract.
- `internal/module/auth/transport/admin/` owns Cookie/Origin HTTP presentation; the service/session lifecycle remains transport-neutral.
- `internal/architecture/browser_only_admin_test.go` owns the permanent backend Browser-only source/contract guard.
- `database/reconciliation/046_retire_client_version_surface.sql` retires active menu/grant data while proving `client_versions` is unchanged.
- `scripts/browser-only/` owns the one-time Docker cutover and session-revocation proof.
- Backend `contracts/admin/v1/` remains the generated source of truth; frontend `contracts/backend/admin/` remains its exact lock.
- `src/adapters/browser/cookie-credentials.ts` is the single frontend credential adapter.
- `src/lib/browser/navigation.ts` and `src/lib/browser/download.ts` own browser-only capability policy; they are not generic native bridges.
- `tests/shared/architecture/browser-only.test.ts` owns the frontend permanent retirement guard.

## Approved-spec coverage map

| Approved design requirement | Implemented by |
| --- | --- |
| one Browser-only runtime and naming model | Tasks 2, 6, 7, 8, 9 |
| Cookie/Origin login, refresh, logout; no JSON refresh credential | Tasks 1, 2, 5, 6, 10 |
| preserve `single_session=1` / `max_sessions=1` | fixed boundary, Tasks 4 and 10 |
| revoke pre-cutover sessions and invalidate old desktop clients | Tasks 4 and 10 |
| remove Rust/Tauri/NSIS/dependencies/native UI | Tasks 7 and 9 |
| retain browser navigation/download/notification/network behavior | Tasks 8, 9, 10 |
| retire client-version routes/menu/permissions/API/UI/contracts | Tasks 3, 4, 5, 6, 8 |
| freeze `client_versions` and COS history for P09 | Tasks 4 and 10; revised P09 plan |
| cancel P08.5 and use no GitHub deployment Workflow | execution index, archived P08.5 banner, Tasks 7 and 9 |
| Docker-only build/runtime/smoke and manual user acceptance | Tasks 6–10 |
| no Playwright unless separately requested | fixed boundary and permanent gate |
| rollback without reverse DDL in P08R | Tasks 4, 10, and revised P09 restore path |

### Task 1: Publish the target Browser-only backend contract before runtime edits

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Create: `docs/contracts/admin-browser-auth-contract.md`
- Modify: `docs/contracts/admin-v1-runtime-model-contracts.md`
- Modify: `docs/architecture.md`
- Modify: `docs/runbooks/session-secret-rotation.md`
- Modify: `README.md`
- Modify: `CONTEXT.md`

- [ ] **Step 1: Record clean master baselines**

Run:

```powershell
git -C E:/admin/admin_back_go branch --show-current
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts branch --show-current
git -C E:/admin/admin_front_ts status --short
git -C E:/admin/admin_back_go worktree list --porcelain
git -C E:/admin/admin_front_ts worktree list --porcelain
```

Expected: both branches are `master`, both status commands are empty, and each worktree list has one `worktree` entry with no `.worktrees` path.

- [ ] **Step 2: Write the exact formal transport contract**

`docs/contracts/admin-browser-auth-contract.md` must state that it becomes active only with the P08R bundle hash and must contain these exact shapes:

```text
POST /api/admin/v1/auth/login
Headers: Origin, platform=admin, device-id, Content-Type=application/json
Password body: {"login_type":"password","login_account":"string","password":"string"}
Code body: {"login_type":"email|phone","login_account":"string","code":"6 digits"}
Success data: {"access_token":"string","expires_in":positive integer}
Side effect: Set-Cookie __Secure-admin_refresh; Path=/api/admin/v1/auth; HttpOnly; Secure; SameSite=Strict

POST /api/admin/v1/auth/refresh
Headers: Origin, platform=admin, device-id
Body: forbidden
Credential input: __Secure-admin_refresh Cookie only
Success data: {"access_token":"string","expires_in":positive integer}
Side effect: rotate the same scoped Cookie

POST /api/admin/v1/auth/logout
Headers: Origin, Authorization=Bearer <access>, platform=admin, device-id
Body: forbidden
Success data: {}
Side effect: revoke session and expire the same scoped Cookie
```

Also define exact failures for missing/unapproved Origin, missing Cookie, non-empty refresh/logout body, expired/reused refresh credential, and structurally invalid response. State explicitly that `X-Admin-Client-Variant`, JSON `refresh_token`, `refresh_expires_in`, desktop Origin, and User-Agent inference do not exist in the contract.

- [ ] **Step 3: Mark current versus target truth correctly**

The existing architecture/runtime documents must say:

```text
Current bundle before P08R: historical browser/desktop variant contract.
Approved target: docs/contracts/admin-browser-auth-contract.md.
Frontend implementation is blocked until Task 5 publishes the matching generated bundle.
```

Do not describe the target as deployed before Task 5. Remove client-version/Tauri language from forward-looking architecture sections while retaining a short historical migration note and the P09 table-drop boundary.

- [ ] **Step 4: Review and commit the formal contract**

```powershell
rg -n "T[B]D|T[O]DO|implement l[a]ter|fill i[n]" docs/contracts/admin-browser-auth-contract.md
git add -- docs/contracts/admin-browser-auth-contract.md docs/contracts/admin-v1-runtime-model-contracts.md docs/architecture.md docs/runbooks/session-secret-rotation.md README.md CONTEXT.md
git diff --cached --check
git diff --cached
git commit -m "docs(auth): define browser-only admin credential contract"
```

Expected: `rg` exits `1`; the commit changes documentation only and does not claim that the target bundle is already active.

### Task 2: Collapse backend Admin authentication to Cookie and Origin

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Delete: `internal/module/auth/client_variant.go`
- Modify: `internal/module/auth/dto.go`
- Modify: `internal/module/auth/transport/admin/request.go`
- Modify: `internal/module/auth/transport/admin/presenter.go`
- Modify: `internal/module/auth/transport/admin/route.go`
- Modify: `internal/module/auth/transport/admin/handler.go`
- Modify: `internal/module/auth/transport/admin/handler_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/middleware/cors_test.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `internal/architecture/identity_routing_test.go`
- Create: `internal/architecture/browser_only_admin_test.go`
- Modify: `internal/shared/i18n/locales/en-US/auth.yaml`
- Modify: `internal/shared/i18n/locales/zh-CN/auth.yaml`
- Modify: `scripts/basic-admin-smoke.ps1`
- Modify: `scripts/export-task-smoke.ps1`
- Modify: `scripts/full-admin-smoke.ps1`

- [ ] **Step 1: Write failing browser-only handler and contract tests**

Tests must prove:

```go
// login: exact allowed Origin -> access fields + scoped Cookie
// login: missing/unlisted/Tauri Origin -> auth.origin_forbidden
// refresh: Cookie + empty body -> rotated Cookie + access fields
// refresh: {"refresh_token":"legacy"} -> auth.refresh_body_forbidden
// refresh: {} -> auth.refresh_body_forbidden
// refresh: missing Cookie -> unauthorized
// logout: exact Origin + Bearer + empty body -> expired Cookie
// queue-monitor grant: exact Origin succeeds without a variant header
// every success JSON schema lacks refresh_token and refresh_expires_in
// OpenAPI has no refresh requestBody and no refresh credential property
```

`browser_only_admin_test.go` must scan production Go, CORS defaults, and generated contract artifacts while excluding historical `docs/superpowers` and test fixtures. It rejects `ClientVariant`, `ClientDesktop`, `ClientBrowser`, `X-Admin-Client-Variant`, and a public JSON `refresh_token` property.

Run:

```powershell
go test ./internal/module/auth/transport/admin ./internal/config ./internal/middleware ./internal/admincontract ./internal/architecture -run 'BrowserOnly|Login|Refresh|Logout|QueueMonitor|CredentialContract|CORS' -count=1
```

Expected: FAIL on the current variant parser, desktop presenter fields, refresh body, and allowed CORS header.

- [ ] **Step 2: Replace variant presentation with one response type**

Use exactly one public response:

```go
type CredentialResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func presentRefresh(result *authmodule.TokenResult) *CredentialResponse {
	if result == nil {
		return nil
	}
	return &CredentialResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   int64(result.ExpiresIn),
	}
}

func presentLogin(result *authmodule.LoginResponse) *CredentialResponse {
	if result == nil {
		return nil
	}
	return &CredentialResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   int64(result.ExpiresIn),
	}
}
```

`LoginResponse` may retain an unexported/internal refresh credential for Cookie issuance, but it must not carry JSON tags that make it a public response schema. Delete `RefreshRequest`; do not replace it with an optional/compatibility body.

- [ ] **Step 3: Require Origin and reject bodies before credential work**

Login, refresh, logout, and queue-monitor grant all call `requireAllowedOrigin`. Refresh and logout use an exact body guard:

```go
func requireEmptyBody(c *gin.Context, messageID string) bool {
	if c.Request.ContentLength > 0 || len(c.Request.TransferEncoding) > 0 {
		response.Error(c, apperror.BadRequestKey(messageID, nil, "请求体必须为空"))
		return false
	}
	return true
}
```

Refresh reads only `BrowserRefreshCookieName`; login/refresh always rotate the Cookie; logout always expires it. Queue-monitor grant removes the browser-variant check because the endpoint itself is now browser-only. Unknown headers are not interpreted as a variant and do not change behavior.

- [ ] **Step 4: Remove the obsolete CORS and i18n surface**

Remove `X-Admin-Client-Variant` from `DefaultCORSConfig().AllowHeaders`. Remove only `auth.client_variant_invalid` and `auth.browser_variant_required` catalog keys; add exact `auth.refresh_body_forbidden` and `auth.logout_body_forbidden` translations used by the handlers.

- [ ] **Step 5: Convert smoke clients to the browser contract**

The three smoke scripts use a `Microsoft.PowerShell.Commands.WebRequestSession`, send the configured `Origin`, receive the refresh Cookie, refresh with no JSON body, and assert neither authentication response contains `refresh_token` nor `refresh_expires_in`. They must never print the Cookie or access token.

- [ ] **Step 6: Verify and commit**

```powershell
go test ./internal/module/auth/... ./internal/config ./internal/middleware ./internal/server ./internal/admincontract ./internal/architecture -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git add -- internal/module/auth/dto.go internal/module/auth/transport/admin/request.go internal/module/auth/transport/admin/presenter.go internal/module/auth/transport/admin/route.go internal/module/auth/transport/admin/handler.go internal/module/auth/transport/admin/handler_test.go internal/config/config.go internal/config/config_test.go internal/middleware/cors_test.go internal/server/router_test.go internal/admincontract/openapi_models_test.go internal/architecture/identity_routing_test.go internal/architecture/browser_only_admin_test.go internal/shared/i18n/locales/en-US/auth.yaml internal/shared/i18n/locales/zh-CN/auth.yaml scripts/basic-admin-smoke.ps1 scripts/export-task-smoke.ps1 scripts/full-admin-smoke.ps1
git add -u -- internal/module/auth/client_variant.go
git diff --cached --check
git commit -m "refactor(auth): collapse admin credentials to browser transport"
```

Expected: focused tests pass; contract drift is expected until Task 5 regeneration but the checker must report only the declared authentication changes.

### Task 3: Remove client-version and updater runtime ownership

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Delete: `internal/module/clientversion/`
- Delete: `internal/shared/enum/client_version.go`
- Delete: `internal/shared/enum/client_version_test.go`
- Delete: `internal/shared/dict/client_version_test.go`
- Delete: `internal/shared/validate/client_version.go`
- Delete: `internal/shared/i18n/locales/en-US/clientversion.yaml`
- Delete: `internal/shared/i18n/locales/zh-CN/clientversion.yaml`
- Modify: `internal/shared/dict/dict.go`
- Modify: `internal/shared/enum/upload.go`
- Modify: `internal/infra/storage/cos/object_writer_test.go`
- Modify: `internal/platform/admin/graph.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/server/routes_admin_foundation.go`
- Modify: `internal/server/dependencies_test.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `internal/admincontract/views.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `internal/architecture/multiplatform_boundary_test.go`
- Modify: `internal/architecture/browser_only_admin_test.go`
- Modify: `internal/module/README.md`
- Modify: `scripts/full-admin-smoke.ps1`

- [ ] **Step 1: Write failing absence tests**

The backend guard must require all of these to be absent from active runtime and the compiled route registry:

```text
internal/module/clientversion
/api/admin/v1/client-versions
system_clientVersion_*
system/clientVersion
menu.system_clientVersion
tauri_updater upload folder
releases upload folder (currently has no non-Tauri business owner)
```

It must simultaneously require `database/schema/admin.hcl` to still define `table "client_versions"` so P08R cannot accidentally perform P09 DDL.

Run:

```powershell
go test ./internal/architecture ./internal/server ./internal/admincontract -run 'BrowserOnly|FoundationAdminTransportShells|Route|Contract' -count=1
```

Expected: FAIL on the module, routes, permissions, view, and smoke assertions.

- [ ] **Step 2: Remove composition and routes before packages**

Delete the `ClientVersions` graph field, provider construction, route registration, fake router service, route tests, route-policy golden entries, and OpenAPI prefix expectation. Remove the client-version view from `buildViewsDocument`. Keep generic `storage/cos.ObjectWriter` because AI image/video and export still use it; rewrite its tests with generic keys such as `exports/report.json`.

- [ ] **Step 3: Delete capability-only helpers and upload folders**

Delete the module, client platform enum/dict/validator, and catalogs. Remove `releases` and `tauri_updater` from the closed upload-folder enum because neither has a surviving business owner. Do not add aliases or deprecated constants.

- [ ] **Step 4: Remove client-version smoke behavior**

Delete client-version init/list/update-json/current-check functions and invocations from `full-admin-smoke.ps1`. Add an unauthenticated/authenticated negative probe asserting `/api/admin/v1/client-versions` is not registered; do not accept a `403` as proof of retirement when the expected result is route absence.

- [ ] **Step 5: Verify and commit runtime removal**

```powershell
go test ./internal/platform/admin ./internal/server ./internal/admincontract ./internal/architecture ./internal/shared/... ./internal/infra/storage/cos -count=1
go test ./internal/module/... -count=1
git add -- internal/shared/dict/dict.go internal/shared/enum/upload.go internal/infra/storage/cos/object_writer_test.go internal/platform/admin/graph.go internal/platform/admin/build.go internal/server/routes_admin_foundation.go internal/server/dependencies_test.go internal/server/router_test.go internal/server/testdata/admin_route_policy_golden.json internal/server/testdata/admin_routes_golden.txt internal/admincontract/views.go internal/admincontract/openapi_models_test.go internal/architecture/multiplatform_boundary_test.go internal/architecture/browser_only_admin_test.go internal/module/README.md scripts/full-admin-smoke.ps1
git add -u -- internal/module/clientversion internal/shared/enum/client_version.go internal/shared/enum/client_version_test.go internal/shared/dict/client_version_test.go internal/shared/validate/client_version.go internal/shared/i18n/locales/en-US/clientversion.yaml internal/shared/i18n/locales/zh-CN/clientversion.yaml
git diff --cached --check
git commit -m "refactor(admin): retire client version runtime surface"
```

Expected: no Go runtime imports the removed module; `client_versions` remains in the canonical schema.

### Task 4: Retire menu grants, freeze history, and implement one-time session cutover

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Create: `database/reconciliation/038_verify_browser_only_retirement.sql`
- Create: `database/reconciliation/046_retire_client_version_surface.sql`
- Modify: `database/reconciliation/README.md`
- Modify: `scripts/database/reconcile.ps1`
- Modify: `scripts/database/verify-expanded-schema.ps1`
- Modify: `scripts/verify-database.ps1`
- Create: `scripts/browser-only/revoke-admin-sessions.ps1`
- Create: `scripts/browser-only/verify-retirement.ps1`
- Create: `scripts/tests/browser-only-cutover.tests.ps1`
- Create: `docs/runbooks/admin-browser-only-cutover.md`

- [ ] **Step 1: Write failing reconciliation and cutover contract tests**

The PowerShell test must require:

```text
reconciliation selector = exact Admin page path/component/i18n key and the five generated permission codes
role_permissions soft-deleted before permissions
permissions soft-deleted, never reassigned
client_versions row count and deterministic row hash unchanged in the same transaction
no DROP/TRUNCATE/DELETE against client_versions
session cutover requires -Apply, expected backend/frontend full SHA, and healthy Docker state services
active Admin sessions are revoked in MySQL
the isolated TOKEN_REDIS_DB is cleared without printing APP_SECRET, DSN, Cookie, or token
repeat execution is idempotent
```

Run:

```powershell
pwsh -NoProfile -File scripts/tests/browser-only-cutover.tests.ps1
```

Expected: FAIL because the scripts and SQL do not exist.

- [ ] **Step 2: Implement the idempotent menu reconciliation**

`046_retire_client_version_surface.sql` creates a temporary exact ID set using:

```sql
SELECT `id`
FROM `permissions`
WHERE `platform` = 'admin'
  AND (
    `path` = '/system/clientVersion'
    OR `component` = 'system/clientVersion'
    OR `i18n_key` = 'menu.system_clientVersion'
    OR `code` IN (
      'system_clientVersion_add',
      'system_clientVersion_del',
      'system_clientVersion_edit',
      'system_clientVersion_forceUpdate',
      'system_clientVersion_setLatest'
    )
  );
```

Before updates, set a bounded `group_concat_max_len`, capture `COUNT(*)`, and compute the deterministic history hash as `SHA2(COALESCE(GROUP_CONCAT(SHA2(CAST(JSON_ARRAY(id,version,notes,file_url,signature,platform,file_size,is_latest,force_update,is_del,created_at,updated_at) AS CHAR),256) ORDER BY id SEPARATOR ''),''),256)`. Soft-delete matching active `role_permissions`, then matching `permissions`. Recompute count/hash and `SIGNAL SQLSTATE '45000'` if either differs. Never mutate `client_versions` or COS objects.

Add stage `browser-only-retirement` and include it in `all-nondestructive`; update the expected reconciliation count from `9` to `10`. `038_verify_browser_only_retirement.sql` returns named violations if an active permission/grant/view selector survives or the historical table is missing.

- [ ] **Step 3: Implement fail-closed session revocation**

`revoke-admin-sessions.ps1` operates only through the existing Docker MySQL and Redis state containers. Without `-Apply` it prints counts only. With `-Apply` it:

1. verifies both full Git SHAs and one primary checkout per repository;
2. verifies the token Redis DB is distinct from ordinary/queue Redis DBs;
3. updates only active `platform='admin'` rows to `revoked_at=UTC_TIMESTAMP(6)` in one transaction;
4. clears the isolated token Redis DB;
5. proves zero active Admin sessions and zero token Redis keys;
6. emits only counts, hashes, revisions, and pass/fail.

It must not change `single_session`, `max_sessions`, `auth_platforms`, users, or login logs.

- [ ] **Step 4: Write exact operator and Credential Manager cleanup instructions**

The runbook records the maintenance order and tells former Windows users to remove these historical entries manually after retirement:

```text
service: cn.zgm2003.admin.refresh
account: current-session
```

The application cannot remotely erase already installed clients, so the runbook must not claim that server deployment clears Windows Credential Manager.

- [ ] **Step 5: Rehearse on Docker-backed disposable data and commit**

```powershell
pwsh -NoProfile -File scripts/tests/browser-only-cutover.tests.ps1
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Stage browser-only-retirement -Database $env:ADMIN_RESTORE_DB -ExpectedSourceFingerprint $env:ADMIN_VERIFIED_FINGERPRINT
pwsh -NoProfile -File scripts/database/verify-expanded-schema.ps1 -Database $env:ADMIN_RESTORE_DB
git add -- database/reconciliation/038_verify_browser_only_retirement.sql database/reconciliation/046_retire_client_version_surface.sql database/reconciliation/README.md scripts/database/reconcile.ps1 scripts/database/verify-expanded-schema.ps1 scripts/verify-database.ps1 scripts/browser-only/revoke-admin-sessions.ps1 scripts/browser-only/verify-retirement.ps1 scripts/tests/browser-only-cutover.tests.ps1 docs/runbooks/admin-browser-only-cutover.md
git diff --cached --check
git commit -m "feat(cutover): retire desktop menu and sessions safely"
```

Expected: the disposable restore has no active client-version menu/grant, its historical rows hash identically, and live data is not touched.

### Task 5: Generate and publish the Browser-only Admin Contract Bundle

**Repository:** `E:/admin/admin_back_go`

**Files:**
- Modify: `contracts/admin/v1/openapi.json`
- Modify: `contracts/admin/v1/permissions.json`
- Modify: `contracts/admin/v1/views.json`
- Modify: `contracts/admin/v1/manifest.json`
- Modify: `internal/admincontract/bundle_test.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `internal/architecture/browser_only_admin_test.go`
- Modify: `docs/contracts/admin-browser-auth-contract.md`
- Modify: `docs/contracts/admin-v1-runtime-model-contracts.md`
- Modify: `docs/architecture.md`

- [ ] **Step 1: Strengthen generated-bundle assertions**

Tests must assert all of the following by parsing JSON, not by matching a sample response:

```text
login/refresh responses: access_token + expires_in only
refresh/logout: no requestBody
no ClientVariant header parameter or schema
no refresh_token / refresh_expires_in public property
no /api/admin/v1/client-versions path
no system_clientVersion_* permission
no system/clientVersion view
manifest hashes exactly match every artifact
```

- [ ] **Step 2: Regenerate backend truth from a clean committed source**

```powershell
git diff --name-only
pwsh -NoProfile -File scripts/generate-admin-contract.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
go test ./internal/admincontract ./internal/architecture -run 'Bundle|Manifest|BrowserOnly|CredentialContract' -count=1
```

Expected: before generation the diff lists only the declared Task 5 test/docs paths; generation/check/tests pass and only the declared generated/docs files change.

- [ ] **Step 3: Activate the formal contract with the generated hash**

Replace the target/pending marker in `admin-browser-auth-contract.md` with the generated bundle version, backend source commit, and manifest SHA-256. Update architecture wording from “approved target” to “active Browser-only contract”. Do not hand-edit generated JSON.

- [ ] **Step 4: Commit the bundle**

```powershell
git add -- contracts/admin/v1/openapi.json contracts/admin/v1/permissions.json contracts/admin/v1/views.json contracts/admin/v1/manifest.json internal/admincontract/bundle_test.go internal/admincontract/openapi_models_test.go internal/architecture/browser_only_admin_test.go docs/contracts/admin-browser-auth-contract.md docs/contracts/admin-v1-runtime-model-contracts.md docs/architecture.md
git diff --cached --check
git commit -m "feat(contract): publish browser-only admin bundle"
```

Expected: the bundle contains no retired operation/field/view/permission and records the committed source revision according to the generator's non-self-referential rule.

### Task 6: Consume the exact bundle and collapse frontend authentication

**Repository:** `E:/admin/admin_front_ts`

**Files:**
- Replace: `contracts/backend/admin/v1/`
- Modify: `contracts/backend/admin/lock.json`
- Modify: `src/modules/http/generated/admin.ts`
- Modify: `src/modules/http/generated/operations.ts`
- Modify: `src/modules/routing/generated/permissions.ts`
- Modify: `src/modules/routing/generated/views.ts`
- Modify: `src/modules/routing/generated/local-views.ts`
- Delete: `src/adapters/web/browser-credentials.ts`
- Create: `src/adapters/browser/cookie-credentials.ts`
- Modify: `src/modules/auth/types.ts`
- Modify: `src/modules/http/schema.ts`
- Modify: `src/app/environment.ts`
- Modify: `src/vite-env.d.ts`
- Delete: `src/lib/http/platform.ts`
- Create: `src/lib/http/headers.ts`
- Modify: `src/main.ts`
- Modify: `tests/integration/auth/refresh-race.test.ts`
- Delete: `tests/integration/auth/desktop-credentials.test.ts`
- Modify: `tests/unit/app/environment.test.ts`
- Modify: `tests/shared/http-language-header.test.ts`
- Modify: `tests/integration/app/kernel.test.ts`
- Modify: `tests/integration/app/logout.test.ts`
- Modify: `tests/integration/app/session-events.test.ts`

- [ ] **Step 1: Sync and generate only from the Task 5 bundle**

Run in the pinned Node container:

```powershell
$root = (Get-Location).Path
$backendCommit = ((Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit)
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --mount "type=bind,src=E:/admin/admin_back_go,dst=/backend,readonly" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm run contract:sync -- --backend /backend --commit $backendCommit && npm run contract:generate && npm run routes:generate && npm run contract:check"
```

Expected: generated transport removes the variant header, refresh body/fields, client-version operations, permission codes, and view key. If the backend manifest and requested commit differ, stop instead of editing generated files.

- [ ] **Step 2: Write failing single-adapter tests**

The credential tests require:

```ts
export interface CredentialAdapter {
  restore(signal: AbortSignal): Promise<AccessCredential | null>
  login(input: LoginCommand, signal: AbortSignal): Promise<AccessCredential>
  refresh(signal: AbortSignal): Promise<AccessCredential>
  revoke(accessToken: string | null, signal: AbortSignal): Promise<void>
  clear(): Promise<void>
}
```

They assert `CookieCredentialAdapter` always uses `credentials: 'include'`, sends no `X-Admin-Client-Variant`, sends no refresh body, includes the common Origin-compatible request headers, and treats any undocumented response field as a Zod contract error. Delete rather than skip desktop credential tests.

- [ ] **Step 3: Remove environment and header variants**

`AppEnvironment` contains only `mode`, `platform`, `apiOrigin`, and `realtimeOrigin`. Delete `VITE_ADMIN_CLIENT_VARIANT`. Rename the common header helper to `headers.ts`; its output is exactly:

```ts
return {
  'Accept-Language': context.language,
  platform: 'admin',
  'device-id': context.deviceId,
  'X-Trace-Id': context.traceId,
}
```

- [ ] **Step 4: Install one credential adapter in `main.ts`**

Create one `CookieCredentialAdapter`; remove lazy variant selection, desktop adapter imports, and the `variant` getter. `AuthSession` behavior, access-token memory ownership, refresh coordinator, and logout cleanup remain unchanged.

- [ ] **Step 5: Run Dockerized tests and commit**

```powershell
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm run contract:check && npm run routes:generate && npm test -- tests/integration/auth/refresh-race.test.ts tests/unit/app/environment.test.ts tests/shared/http-language-header.test.ts tests/integration/app && npm run typecheck"
git add -- contracts/backend/admin/v1 contracts/backend/admin/lock.json src/modules/http/generated/admin.ts src/modules/http/generated/operations.ts src/modules/routing/generated/permissions.ts src/modules/routing/generated/views.ts src/modules/routing/generated/local-views.ts src/adapters/browser/cookie-credentials.ts src/modules/auth/types.ts src/modules/http/schema.ts src/app/environment.ts src/vite-env.d.ts src/lib/http/headers.ts src/main.ts tests/integration/auth/refresh-race.test.ts tests/unit/app/environment.test.ts tests/shared/http-language-header.test.ts tests/integration/app/kernel.test.ts tests/integration/app/logout.test.ts tests/integration/app/session-events.test.ts
git add -u -- src/adapters/web/browser-credentials.ts src/lib/http/platform.ts tests/integration/auth/desktop-credentials.test.ts
git diff --cached --check
git commit -m "refactor(auth): consume browser-only admin contract"
```

### Task 7: Delete Tauri, Rust, and the desktop shell while keeping a compiling Web transition

**Repository:** `E:/admin/admin_front_ts`

**Files:**
- Delete: `src-tauri/`
- Delete: `rust-toolchain.toml`
- Delete: `scripts/verify-tauri.ps1`
- Delete: `scripts/tests/tauri-security-source.tests.ps1`
- Delete: `src/adapters/tauri/`
- Modify: `src/adapters/native.ts`
- Modify: `src/adapters/web/native-bridge.ts`
- Modify: `src/modules/native/types.ts`
- Delete: `src/store/tauri.ts`
- Delete: `src/components/TauriManager/`
- Delete: `src/views/Layout/components/Header/components/WindowControls.vue`
- Delete: `tests/shared/tauri/`
- Delete: `tests/integration/native/`
- Delete: `tests/unit/native/managed-download.test.ts`
- Delete: `tests/unit/native/tauri-approved-operations.test.ts`
- Delete: `tests/unit/native/tauri-close-confirmation.test.ts`
- Delete: `tests/unit/persistence/tauri.test.ts`
- Delete: `docs/acceptance/p08-tauri-windows-manual.md`
- Modify: `package.json`
- Modify: `package-lock.json`
- Modify: `vite.config.ts`
- Modify: `eslint.config.js`
- Modify: `src/main.ts`
- Modify: `src/App.vue`
- Modify: `src/views/Login/index.vue`
- Modify: `src/views/Layout/components/Header/index.vue`
- Modify: `src/views/Layout/components/Header/components/SettingDrawer.vue`
- Modify: `src/modules/persistence/preferences.ts`
- Create: `tests/unit/persistence/device-preferences.test.ts`

- [ ] **Step 1: Write failing removal and preference-migration tests**

Tests require all Tauri/Rust paths and dependencies to be absent. A temporary Web-only bridge may remain for one commit so existing browser consumers continue to compile; it must have no Tauri import, desktop credential, updater implementation, process/window control, or runtime-kind branch and is deleted in Task 8. Device preferences move to codec version `2` and migrate version `1` by retaining only `theme`, `language`, and `rememberedLogin`; `desktopWindow` is discarded rather than preserved under an alias.

Use this migration shape:

```ts
const devicePreferencesCodec = defineCodec({
  version: 2,
  maxBytes: 8 * 1024,
  schema: devicePreferencesSchema,
  migrate(version, value) {
    if (version !== 1 || typeof value !== 'object' || value === null || Array.isArray(value)) return null
    const source = value as Record<string, unknown>
    return {
      ...(source.theme === undefined ? {} : { theme: source.theme }),
      ...(source.language === undefined ? {} : { language: source.language }),
      ...(source.rememberedLogin === undefined ? {} : { rememberedLogin: source.rememberedLogin }),
    }
  },
})
```

- [ ] **Step 2: Remove Tauri packages using the pinned container**

```powershell
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm uninstall @tauri-apps/api @tauri-apps/plugin-updater @tauri-apps/cli"
```

Remove the `tauri`, `tauri:dev`, and `tauri:build` scripts. Do not hand-edit the lockfile to simulate uninstall.

- [ ] **Step 3: Remove desktop composition and UI**

`main.ts` no longer detects or installs a Tauri bridge and no longer installs a Tauri store. `src/adapters/native.ts` is reduced temporarily to the existing Web implementation used by browser consumers; it cannot dynamically import a platform adapter. `App.vue` no longer mounts `TauriManager`. Login/Header/Settings contain no draggable region, window controls, updater status, close behavior, desktop labels, or Tauri condition. Delete the obsolete preference/store tests rather than weakening them.

- [ ] **Step 4: Remove build/config exemptions**

Delete `TAURI_DEV_HOST`, `TAURI_ENV_DEBUG`, fixed Tauri port comments, and `src-tauri` watch/lint exclusions. The Vite dev command remains available for project tooling but is never used as a runtime acceptance path.

- [ ] **Step 5: Verify deletion and commit**

```powershell
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm test -- tests/unit/persistence/device-preferences.test.ts tests/integration/app && npm run typecheck"
git add -- package.json package-lock.json vite.config.ts eslint.config.js src/main.ts src/App.vue src/adapters/native.ts src/adapters/web/native-bridge.ts src/modules/native/types.ts src/views/Login/index.vue src/views/Layout/components/Header/index.vue src/views/Layout/components/Header/components/SettingDrawer.vue src/modules/persistence/preferences.ts tests/unit/persistence/device-preferences.test.ts
git add -u -- src-tauri rust-toolchain.toml scripts/verify-tauri.ps1 scripts/tests/tauri-security-source.tests.ps1 src/adapters/tauri src/store/tauri.ts src/components/TauriManager src/views/Layout/components/Header/components/WindowControls.vue tests/shared/tauri tests/integration/native tests/unit/native/managed-download.test.ts tests/unit/native/tauri-approved-operations.test.ts tests/unit/native/tauri-close-confirmation.test.ts tests/unit/persistence/tauri.test.ts docs/acceptance/p08-tauri-windows-manual.md
git diff --cached --check
git commit -m "refactor(frontend): remove tauri and desktop shell"
```

### Task 8: Replace native abstractions with focused browser capabilities and remove the client-version UI

**Repository:** `E:/admin/admin_front_ts`

**Files:**
- Delete: `src/adapters/native.ts`
- Delete: `src/adapters/web/native-bridge.ts`
- Delete: `src/modules/native/`
- Delete: `tests/unit/native/web-bridge.test.ts`
- Create: `src/lib/browser/navigation.ts`
- Create: `src/lib/browser/download.ts`
- Create: `tests/unit/browser/navigation.test.ts`
- Create: `tests/unit/browser/download.test.ts`
- Modify: `src/components/NotificationRuntime/src/index.vue`
- Delete: `src/components/DownloadManager/`
- Modify: `src/views/Main/home/composables/useHomeDashboard.ts`
- Modify: `src/views/Main/payment/recharge/composables/usePaymentRechargePage.ts`
- Modify: `src/views/Main/system/queueMonitor/index.vue`
- Modify: `src/views/Main/system/exportTask/index.vue`
- Modify: `src/views/Main/component/download/index.vue`
- Modify: `src/views/Main/test/index.vue`
- Delete: `src/api/system/clientVersion.ts`
- Delete: `src/views/Main/system/clientVersion/`
- Modify: `src/i18n/locales/en-US.ts`
- Modify: `src/i18n/locales/zh-CN.ts`
- Modify: `src/lib/upload/uploadClient.ts`
- Modify: `tests/component/notification/NotificationRuntime.test.ts`
- Modify: `tests/shared/deployment/frontend-deploy-env.test.ts`
- Test: `tests/component/network/NetworkStatusNotice.test.ts`
- Test: `tests/unit/auth/session.test.ts`

- [ ] **Step 1: Test focused browser policies first**

Navigation tests cover exact allowlisted HTTPS external hosts, credentials/HTTP/`javascript:` rejection, `noopener,noreferrer`, opener nulling, same-origin queue-monitor URLs, and payment navigation. Download tests cover same-origin or allowlisted HTTPS only, filename derivation, HTTP failure, Blob URL revocation, anchor cleanup, and no mock fallback.

- [ ] **Step 2: Implement direct browser navigation helpers**

Export only named browser functions:

```ts
export function openExternalUrl(input: string): void
export function openSameOriginPath(input: string): void
export function navigateToExternalHttps(input: string): void
```

Do not create `Bridge`, `Adapter`, `kind`, `isDesktop`, `isTauri`, or an unavailable native-operation object.

- [ ] **Step 3: Reduce downloads to real browser behavior**

`src/lib/browser/download.ts` exports `downloadFile(url, filename?)` and pure filename/size formatting. It fetches the validated URL, requires `response.ok`, creates a Blob URL, clicks a temporary `<a download>`, and always removes/revokes resources. Delete native task IDs, cancel/reveal/progress manager UI, header download count, and Tauri demo text. Keep the export-task download action and a browser-only download demo.

- [ ] **Step 4: Keep browser notification and online behavior explicit**

`NotificationRuntime` always renders the in-app Element Plus notification; it does not attempt a native notification. Run the existing `NetworkStatusNotice` and AuthSession connectivity suites to prove the global `online`/`offline` UI remains visible on login/Layout and that API 401/500/contract errors are not relabelled as offline.

- [ ] **Step 5: Delete client-version frontend surface**

Delete the API wrapper, route page, signature component, local view loader, permission/view literal, and i18n domains. Remove `releases` and `tauri_updater` from the frontend upload-folder guard so it remains exactly equal to the generated backend union. Do not leave a hidden route or “temporarily unavailable” page.

- [ ] **Step 6: Verify and commit**

```powershell
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm run contract:check && npm run routes:generate && npm test -- tests/unit/browser tests/component/notification tests/component/network/NetworkStatusNotice.test.ts tests/unit/auth/session.test.ts tests/shared/deployment/frontend-deploy-env.test.ts && npm run typecheck && npm run lint:baseline && npm run build:check"
git add -- src/lib/browser/navigation.ts src/lib/browser/download.ts tests/unit/browser/navigation.test.ts tests/unit/browser/download.test.ts src/components/NotificationRuntime/src/index.vue src/views/Main/home/composables/useHomeDashboard.ts src/views/Main/payment/recharge/composables/usePaymentRechargePage.ts src/views/Main/system/queueMonitor/index.vue src/views/Main/system/exportTask/index.vue src/views/Main/component/download/index.vue src/views/Main/test/index.vue src/i18n/locales/en-US.ts src/i18n/locales/zh-CN.ts src/lib/upload/uploadClient.ts tests/component/notification/NotificationRuntime.test.ts tests/shared/deployment/frontend-deploy-env.test.ts src/modules/routing/generated/local-views.ts
git add -u -- src/adapters/native.ts src/adapters/web/native-bridge.ts src/modules/native tests/unit/native/web-bridge.test.ts src/components/DownloadManager src/api/system/clientVersion.ts src/views/Main/system/clientVersion
git diff --cached --check
git commit -m "refactor(browser): replace native abstractions with web capabilities"
```

### Task 9: Make Browser-only retirement a permanent frontend gate

**Repository:** `E:/admin/admin_front_ts`

**Files:**
- Create: `scripts/check-browser-only.mjs`
- Create: `tests/shared/architecture/browser-only.test.ts`
- Create: `docs/acceptance/p08r-browser-only-manual.md`
- Modify: `package.json`
- Modify: `scripts/verify-frontend.ps1`

- [ ] **Step 1: Write the failing architecture gate contract**

The test invokes/parses `check-browser-only.mjs` and requires it to reject:

```text
src-tauri, Cargo.toml, Cargo.lock, rust-toolchain.toml
@tauri-apps dependencies or scripts
Tauri/native/desktop/client-variant production identifiers
X-Admin-Client-Variant and VITE_ADMIN_CLIENT_VARIANT
client-version API/page/view/permission identifiers
refresh_token or refresh_expires_in in frontend generated public schemas
.github, Workflow deployment files, and .worktrees
runtime mock/fallback authentication data
```

The guard excludes historical `docs/superpowers/plans/2026-07-15-admin-tauri-security-plan.md` and the guard's own literal list. It must parse `package.json`, contract JSON, and tracked paths instead of relying only on one broad substring search.

- [ ] **Step 2: Add the gate without taking over P07 quality work**

Add npm script `check:browser-only`. Rewrite every package/generation/lint/typecheck/test/build invocation in `verify-frontend.ps1` to execute inside `node:22.23.1-alpine`; do not implement P07's zero-warning, bundle-budget, or accessibility tasks here. The script must not run host npm.

- [ ] **Step 3: Write the user-owned manual checklist**

The checklist records backend/frontend full revisions and bundle hash, and leaves unchecked user-owned boxes for:

```text
password login without captcha
email/phone code send with captcha
Cookie refresh and logout
old desktop session invalidation
first protected route/menu persistence/direct URL
queue monitor embedded and standalone open
browser download and external payment/navigation
in-app notification and realtime reconnect
online/offline banner on login and Layout
no client-version menu/route
theme/language/remembered account persistence after v1 preference migration
```

The Agent may fill evidence fields but may not mark acceptance boxes.

- [ ] **Step 4: Run the complete Dockerized frontend gate and commit**

```powershell
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm run check:browser-only && npm run contract:check && npm run routes:generate && npm run lint:baseline && npm run typecheck && npm test -- --coverage && npm run build:check"
git add -- scripts/check-browser-only.mjs tests/shared/architecture/browser-only.test.ts docs/acceptance/p08r-browser-only-manual.md package.json package-lock.json scripts/verify-frontend.ps1
git diff --cached --check
git commit -m "test(frontend): block desktop runtime regressions"
```

Expected: the static gate, contract lock, generated routes, current P07 warning ceiling, typecheck, all tests, coverage, and production build pass entirely in Docker.

### Task 10: Switch the joint Docker platform and produce P08R evidence

**Repositories:** `E:/admin/admin_back_go`, `E:/admin/admin_front_ts`

**Files:**
- Modify after execution evidence only: `docs/runbooks/admin-browser-only-cutover.md`
- Modify after execution evidence only: `E:/admin/admin_front_ts/docs/acceptance/p08r-browser-only-manual.md`
- Modify after execution evidence only: this plan's task checkboxes/evidence section

- [ ] **Step 1: Re-run both clean static gates before touching live state**

```powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/verify-backend.ps1

cd E:/admin/admin_front_ts
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm run check:browser-only && npm run contract:check && npm run typecheck && npm test -- --coverage && npm run build:check"
```

Expected: both status commands are empty and every static gate passes.

- [ ] **Step 2: Build and start only through the existing Docker platform**

```powershell
cd E:/admin/admin_back_go
pwsh -NoProfile -File scripts/docker-platform.ps1 up
pwsh -NoProfile -File scripts/docker-platform.ps1 status
```

Expected: frontend/API/Worker images carry their owning full Git revision labels; frontend, API, Worker, MySQL, and Redis are healthy. No host runtime is started.

- [ ] **Step 3: Apply the retirement reconciliation and revoke pre-cutover sessions**

```powershell
pwsh -NoProfile -File scripts/database/reconcile.ps1 -Stage browser-only-retirement -Database admin -ExpectedSourceFingerprint $env:ADMIN_VERIFIED_FINGERPRINT
pwsh -NoProfile -File scripts/browser-only/revoke-admin-sessions.ps1 -BackendCommit (git rev-parse HEAD) -FrontendCommit (git -C E:/admin/admin_front_ts rev-parse HEAD) -Apply
pwsh -NoProfile -File scripts/browser-only/verify-retirement.ps1
```

Expected: menu/grants are retired, `client_versions` count/hash is unchanged, all pre-cutover Admin sessions are revoked, token Redis is empty, and the operator must log in again.

- [ ] **Step 4: Run browser-only Docker smoke**

Use smoke credentials only through `ADMIN_SMOKE_ACCOUNT` and `ADMIN_SMOKE_PASSWORD`:

```powershell
pwsh -NoProfile -File scripts/basic-admin-smoke.ps1
pwsh -NoProfile -File scripts/full-admin-smoke.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
pwsh -NoProfile -File scripts/docker-platform.ps1 status
```

Smoke must prove password/code login, Cookie rotation, logout, Origin denial, JSON refresh-body denial, queue-monitor grant, realtime ticket/WebSocket, retired route absence, and no secret output.

- [ ] **Step 5: Stop for user functional acceptance**

Populate revisions, image IDs, manifest hash, reconciliation run ID/hash, session revocation count, and automated command results in the runbook/checklist. Do not mark user boxes. P07 Task 6 cannot start until the user confirms the checklist.

- [ ] **Step 6: Commit evidence after user acceptance**

```powershell
git -C E:/admin/admin_back_go add -- docs/runbooks/admin-browser-only-cutover.md docs/superpowers/plans/2026-07-19-admin-browser-only-tauri-retirement-plan.md
git -C E:/admin/admin_back_go diff --cached --check
git -C E:/admin/admin_back_go commit -m "docs(cutover): record browser-only retirement proof"

git -C E:/admin/admin_front_ts add -- docs/acceptance/p08r-browser-only-manual.md
git -C E:/admin/admin_front_ts diff --cached --check
git -C E:/admin/admin_front_ts commit -m "docs(acceptance): record browser-only functional review"
```

## Plan completion gate

```powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
git -C E:/admin/admin_back_go worktree list --porcelain
git -C E:/admin/admin_front_ts worktree list --porcelain

pwsh -NoProfile -File E:/admin/admin_back_go/scripts/verify-backend.ps1
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/verify-database.ps1 -Mode all
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/browser-only/verify-retirement.ps1
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/docker-platform.ps1 status

cd E:/admin/admin_front_ts
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:22.23.1-alpine sh -lc "npm ci && npm run check:browser-only && npm run contract:check && npm run routes:generate && npm run lint:baseline && npm run typecheck && npm test -- --coverage && npm run build:check"
```

Expected: both repositories are clean and have one primary checkout; backend contract/database/runtime gates pass; the five Docker containers are healthy with exact revision labels; frontend static/contract/type/test/build gates pass in Docker; no Tauri/native/client-variant/client-version runtime surface exists; `client_versions` remains frozen; single-session behavior is unchanged; and the user has separately accepted `docs/acceptance/p08r-browser-only-manual.md`.

P09 is the only phase allowed to drop `client_versions`, and it must stop immediately before that DDL for a fresh explicit user approval.
