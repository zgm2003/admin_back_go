# Admin Go Identity and Route Policy Implementation Plan

> **Superseded delivery note (2026-07-18):** This completed plan's backend Workflow edits are historical evidence and must not be replayed. Web/backend verification and delivery now use repository scripts plus Docker Compose. The only allowed future Workflow is the P08.5 Windows Tauri candidate release defined by the execution index.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make session issue/rotation/revocation atomic, cut browser and desktop credential transport to secure variants, version RBAC principals without SQL on cache hits, and place complete access/audit policy next to every route.

**Architecture:** MySQL remains session and authorization truth. Redis accelerates session revocation, owns short-lived browser grants, and implements a fail-closed versioned principal cache. The Session Lifecycle is split by behavior while one public interface serves login, middleware, and Admin session management.

**Tech Stack:** Go 1.26.5, Gin, GORM/MySQL, Redis Lua, JWT access credentials, opaque rotating refresh credentials.

---

## Execution prerequisite and Docker-only runtime policy

P04 starts only after P03.5 and Gate C.5 are complete. The standard state/app stack must be started with `pwsh -NoProfile -File scripts/docker-platform.ps1 up`; API, worker, Vite, MySQL, and Redis must never be started as host processes.

Pure source/unit checks may run on the host. Any test that opens MySQL/Redis, exercises a running process, or performs integration/smoke/E2E behavior runs in a container attached to `admin-platform`. Use the pinned Go image with the repository bind-mounted at `/src`, `--env-file deploy/docker-first/admin-go.env`, and named Go caches; browser flows target the Docker frontend origin. PowerShell may orchestrate Docker but may not substitute a host runtime.

## Target file map

- Replace `internal/module/auth/session.go` with `session_contract.go`, `session_lifecycle.go`, `session_repository.go`, `session_cache.go`, `session_token.go`, and `session_admin.go`.
- Modify `internal/module/auth/service.go` and Admin auth transport request/handler/presenter/route files.
- Create `internal/module/auth/client_variant.go`, `browser_grant.go`, and integration tests.
- Create `internal/module/permission/principal_{model,repository,cache,service}.go`.
- Modify permission, role, and user mutation transactions to bump affected versions.
- Modify every active Admin and temporary App/Canvas route file listed in Task 7 to register through `adminroute.Registry`; P09 deletes the temporary product routes.
- Delete `internal/bootstrap/route_meta.go` and duplicate permission/audit middleware maps.
- Create `docs/runbooks/session-secret-rotation.md` and identity integration scripts.

### Task 1: Split Session Lifecycle by behavior without changing results

**Files:**
- Create: `internal/module/auth/session_contract.go`
- Create: `internal/module/auth/session_cache.go`
- Create: `internal/module/auth/session_token.go`
- Create: `internal/module/auth/session_repository.go`
- Create: `internal/module/auth/session_lifecycle.go`
- Create: `internal/module/auth/session_admin.go`
- Move tests from: `internal/module/auth/session_test.go`
- Delete: `internal/module/auth/session.go`

- [x] **Step 1: Add a compile-time public contract test**

```go
type Lifecycle interface {
	Issue(context.Context, IssueCommand) (*CredentialSet, *apperror.Error)
	Authenticate(context.Context, AccessCredential) (*Identity, *apperror.Error)
	Rotate(context.Context, RotateCommand) (*CredentialSet, *apperror.Error)
	Revoke(context.Context, RevokeCommand) *apperror.Error
}
var _ Lifecycle = (*SessionLifecycle)(nil)
```

Add golden behavior tests for cache hit/miss, policy binding, JWT claims, refresh expiry, logout, single session, max sessions, and Admin revoke before moving code.

- [x] **Step 2: Prove the characterization suite**

Run: `go test ./internal/module/auth -run 'TestSession|TestAuthenticator|TestHashToken' -count=1`

Expected: PASS against the old file; this records behavior before the mechanical split.

- [x] **Step 3: Move responsibilities**

`session_contract.go` owns commands/results/identity/policy; `session_token.go` owns random tokens, hashes, and JWT verification; `session_cache.go` owns versioned JSON cache payloads and revocation; `session_repository.go` owns GORM and transactions; `session_lifecycle.go` owns Issue/Authenticate/Rotate/Revoke; `session_admin.go` owns list/stats/Admin revoke.

The cache payload is:

```go
type CachedSession struct {
	SessionID int64 `json:"session_id"`
	UserID int64 `json:"user_id"`
	Platform string `json:"platform"`
	DeviceID string `json:"device_id"`
	IP string `json:"ip"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	SchemaVersion int `json:"schema_version"`
}
```

Reject unknown cache schema versions and fall back to MySQL.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/auth -count=1
git add -- internal/module/auth/session_contract.go internal/module/auth/session_cache.go internal/module/auth/session_token.go internal/module/auth/session_repository.go internal/module/auth/session_lifecycle.go internal/module/auth/session_admin.go internal/module/auth/session_test.go
git rm -- internal/module/auth/session.go
git commit -m "refactor(auth): split session lifecycle by behavior"
```

### Task 2: Make session issue and limits one transaction

**Files:**
- Modify: `internal/module/auth/session_repository.go`
- Modify: `internal/module/auth/session_lifecycle.go`
- Create: `internal/module/auth/session_issue_integration_test.go`

- [x] **Step 1: Write concurrent issue tests**

Start 20 goroutines for one user/platform with `single_session=true` and assert exactly one active row. Repeat with `max_sessions=3` and assert three newest rows active, all older rows revoked, and no stale single-session pointer survives.

- [x] **Step 2: Add a transaction-scoped repository contract**

```go
type SessionRepository interface {
	WithUserLock(context.Context, int64, string, func(SessionRepository) error) error
	ListActiveForUpdate(context.Context, int64, string, time.Time) ([]Session, error)
	Insert(context.Context, SessionCreate) (int64, error)
	RevokeIDs(context.Context, []int64, time.Time) error
	FindValidByID(context.Context, int64, time.Time) (*Session, error)
	RotateIfRefreshHash(context.Context, int64, string, SessionRotation) (bool, error)
	Revoke(context.Context, int64, time.Time) error
}
```

`WithUserLock` locks the stable `users` row with `FOR UPDATE` before reading sessions, so an empty session set is still serialized. Issue selects active sessions for update, applies policy, inserts the new row, and commits before publishing cache state.

- [x] **Step 3: Run MySQL integration and race tests**

```powershell
go test ./internal/module/auth -run TestConcurrentIssue -race -count=20
```

Expected: one winner for single-session; max-session invariant holds every run.

- [x] **Step 4: Commit**

```powershell
git add -- internal/module/auth/session_repository.go internal/module/auth/session_lifecycle.go internal/module/auth/session_issue_integration_test.go
git commit -m "fix(auth): enforce session limits transactionally"
```

### Task 3: Make refresh rotation compare-and-swap

**Files:**
- Modify: `internal/module/auth/session_repository.go`
- Modify: `internal/module/auth/session_lifecycle.go`
- Create: `internal/module/auth/session_rotate_integration_test.go`

- [x] **Step 1: Write the 20-way refresh race**

Issue one refresh credential, release 20 goroutines simultaneously, and assert one succeeds, 19 return stable code `auth.refresh_reused`, one new refresh hash is stored, and the original hash cannot rotate again.

- [x] **Step 2: Implement the CAS**

```go
result := db.WithContext(ctx).Model(&Session{}).
	Where("id=? AND refresh_token_hash=? AND revoked_at IS NULL AND refresh_expires_at>?", id, previousHash, now).
	Updates(map[string]any{
		"refresh_token_hash": next.RefreshHash,
		"expires_at": next.AccessExpiresAt,
		"last_seen_at": now,
		"ip": next.IP,
		"user_agent": next.UserAgent,
		"updated_at": now,
	})
return result.RowsAffected == 1, result.Error
```

Do not extend `refresh_expires_at`. Delete the old cache after commit; a reused or lost credential is permanent, not retryable.

- [x] **Step 3: Verify and commit**

```powershell
go test ./internal/module/auth -run TestConcurrentRotate -race -count=20
git add -- internal/module/auth/session_repository.go internal/module/auth/session_lifecycle.go internal/module/auth/session_rotate_integration_test.go
git commit -m "fix(auth): rotate refresh credentials with one winner"
```

### Task 4: Enforce persisted token properties and retire access hashes

**Files:**
- Modify: `internal/module/auth/session_contract.go`
- Modify: `internal/module/auth/session_token.go`
- Modify: `internal/module/auth/session_lifecycle.go`
- Modify: `internal/module/auth/session_repository.go`
- Modify: `internal/module/auth/session_test.go`

- [x] **Step 1: Add mismatch cases**

Table-test session ID, subject/user ID, issuer, platform, device ID, issued-at/not-before, access expiry, revoked state, deleted state, user status, and refresh expiry. Every mismatch must reject with authentication category.

- [x] **Step 2: Remove unused hash behavior**

Stop generating, storing, selecting, or updating `access_token_hash`. Authenticate verifies the signed access credential claims against the MySQL/cache session properties above. The column remains physically present until P09 contract DDL.

- [x] **Step 3: Verify and commit**

```powershell
go test ./internal/module/auth -run 'TestAuthenticate|TestAccessHash' -count=1
rg -n "access_token_hash" internal/module/auth
git add -- internal/module/auth/session_contract.go internal/module/auth/session_token.go internal/module/auth/session_lifecycle.go internal/module/auth/session_repository.go internal/module/auth/session_test.go
git commit -m "refactor(auth): verify session claims without access hashes"
```

Expected: tests pass and the search returns no active session code reference.

### Task 5: Cut browser and desktop credential transport atomically

**Files:**
- Create: `internal/module/auth/client_variant.go`
- Create: `internal/module/auth/browser_grant.go`
- Create: `internal/module/auth/browser_grant_test.go`
- Modify: `internal/module/auth/transport/admin/request.go`
- Modify: `internal/module/auth/transport/admin/presenter.go`
- Modify: `internal/module/auth/transport/admin/handler.go`
- Modify: `internal/module/auth/transport/admin/handler_test.go`
- Modify: `internal/module/auth/transport/admin/route.go`
- Modify: `internal/middleware/auth_token.go`
- Modify: `internal/middleware/auth_token_test.go`

- [x] **Step 1: Write browser/desktop contract tests**

Browser login must set `__Secure-admin_refresh` with `HttpOnly`, `Secure`, `SameSite=Strict`, path `/api/admin/v1/auth`, and return no refresh credential. Desktop login must return the rotating refresh credential in its one-time response and set no cookie. Browser refresh reads only the cookie and requires an allowed exact Origin; desktop refresh reads only the JSON credential and requires native variant.

- [x] **Step 2: Define the explicit variant contract**

```go
type ClientVariant string
const (
	ClientBrowser ClientVariant = "browser"
	ClientDesktop ClientVariant = "desktop"
)
const ClientVariantHeader = "X-Admin-Client-Variant"
```

Login, refresh, and logout reject a missing/unknown variant. Login response:

```go
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn int64 `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	RefreshExpiresIn int64 `json:"refresh_expires_in,omitempty"`
}
```

Only the desktop presenter populates refresh fields. Browser state-changing cookie operations compare normalized `Origin` against configured exact origins before service invocation.

- [x] **Step 3: Replace browser-only access-cookie workarounds**

Add `POST /api/admin/v1/auth/realtime-tickets`. It requires bearer authentication and returns an opaque, single-use, 30-second Redis ticket bound to session/user/platform. WebSocket upgrade consumes `?ticket=` and redacts it from access logs.

Add `POST /api/admin/v1/auth/queue-monitor-grants`. It sets a random, HttpOnly/Secure/SameSite-Strict cookie scoped to `/api/admin/v1/queue-monitor-ui` with 60-second expiry. The Redis value binds the current session; the UI middleware accepts only this grant. Remove `access_token` cookie fallback and query-token support.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/auth/transport/admin ./internal/module/auth ./internal/middleware -run 'TestBrowser|TestDesktop|TestOrigin|TestGrant|TestTicket' -count=1
git add -- internal/module/auth/client_variant.go internal/module/auth/browser_grant.go internal/module/auth/browser_grant_test.go internal/module/auth/transport/admin/request.go internal/module/auth/transport/admin/presenter.go internal/module/auth/transport/admin/handler.go internal/module/auth/transport/admin/handler_test.go internal/module/auth/transport/admin/route.go internal/middleware/auth_token.go internal/middleware/auth_token_test.go
git diff --cached --check
git commit -m "feat(auth): secure browser and desktop credential transport"
```

### Task 6: Add fail-closed versioned principal snapshots

**Files:**
- Create: `internal/module/permission/principal_model.go`
- Create: `internal/module/permission/principal_repository.go`
- Create: `internal/module/permission/principal_cache.go`
- Create: `internal/module/permission/principal_service.go`
- Create: `internal/module/permission/principal_integration_test.go`
- Modify: `internal/bootstrap/permission_checker.go`
- Modify: `internal/module/user/service.go`
- Modify: `internal/module/user/repository.go`
- Modify: `internal/module/role/service.go`
- Modify: `internal/module/role/repository.go`
- Modify: `internal/module/permission/service.go`
- Modify: `internal/module/permission/repository.go`

- [x] **Step 1: Test cache hits and mutation races**

Assert a populated snapshot authorizes with zero SQL, cache miss builds from MySQL, Redis failure denies Permission routes, user disable/role change/role permission change makes the old snapshot unusable, and a request concurrent with mutation linearizes either before the invalidation gate or after the new version.

- [x] **Step 2: Define the snapshot key and result**

```go
type PrincipalSnapshot struct {
	UserID int64 `json:"user_id"`
	RoleID int64 `json:"role_id"`
	Platform string `json:"platform"`
	Version uint64 `json:"version"`
	UserActive bool `json:"user_active"`
	RoleActive bool `json:"role_active"`
	RouteCodes []string `json:"route_codes"`
	ButtonCodes []string `json:"button_codes"`
}
func PrincipalKey(userID, roleID int64, platform string, version uint64) string {
	return fmt.Sprintf("authz:principal:v1:%s:%d:%d:%d", platform, userID, roleID, version)
}
```

`VersionRepository` locks/bump rows in `authz_principal_versions` inside the same business mutation transaction.

- [x] **Step 3: Implement an atomic fail-closed cache protocol**

Before the DB mutation, a Redis Lua script places an `invalidating` gate for every affected Admin user and returns the previous version. Permission checks atomically read gate/version/snapshot; `invalidating` or Redis errors deny. After DB commit, a second Lua script publishes the new versions, removes old snapshot keys, and clears gates. If the process dies after DB commit, gates remain fail-closed and the next mutation/startup reconciler repairs versions from MySQL.

User status/role updates, role grant changes, permission status/tree changes, and user deletion call the same protocol. A Redis failure before DB mutation aborts the mutation; it never commits a permission change with an uninvalidated cache.

- [x] **Step 4: Verify with MySQL and Redis**

```powershell
go test ./internal/module/permission ./internal/module/role ./internal/module/user -run 'TestPrincipal|TestInvalidation' -race -count=10
```

Expected: cache hits issue zero SQL and all cache/invalidation failures deny.

- [x] **Step 5: Commit**

```powershell
git add -- internal/module/permission/principal_model.go internal/module/permission/principal_repository.go internal/module/permission/principal_cache.go internal/module/permission/principal_service.go internal/module/permission/principal_integration_test.go internal/module/permission/service.go internal/module/permission/repository.go internal/module/role/service.go internal/module/role/repository.go internal/module/user/service.go internal/module/user/repository.go internal/bootstrap/permission_checker.go
git diff --cached --check
git commit -m "feat(rbac): version principal snapshots and fail closed"
```

### Task 7: Move route policy next to every registration

**Files:**
- Modify: the 31 exact Admin route files listed below
- Modify: the 16 exact temporary App/Canvas route files listed below
- Modify: `internal/module/notification/transport/admin/task_route.go`
- Modify: `internal/module/payment/transport/callback/route.go`
- Modify: `internal/server/routes_auth.go`
- Modify: `internal/server/routes_admin_ai.go`
- Modify: `internal/server/routes_admin_commerce_rbac.go`
- Modify: `internal/server/routes_admin_comms.go`
- Modify: `internal/server/routes_admin_foundation.go`
- Modify: `internal/server/routes_admin_user.go`
- Modify: `internal/server/routes_canvas.go`
- Delete: `internal/bootstrap/route_meta.go`
- Delete: `internal/bootstrap/route_meta_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `contracts/admin/v1/openapi.json`
- Modify: `contracts/admin/v1/permissions.json`
- Modify: `contracts/admin/v1/views.json`
- Modify: `contracts/admin/v1/realtime/envelope.schema.json`
- Modify: `contracts/admin/v1/realtime/events.schema.json`
- Modify: `contracts/admin/v1/manifest.json`

The 31 exact Admin route files are:

```text
internal/module/ai/agent/transport/admin/route.go
internal/module/ai/chat/transport/admin/route.go
internal/module/ai/conversation/transport/admin/route.go
internal/module/ai/knowledge/transport/admin/route.go
internal/module/ai/message/transport/admin/route.go
internal/module/ai/prompt/transport/admin/route.go
internal/module/ai/provider/transport/admin/route.go
internal/module/ai/run/transport/admin/route.go
internal/module/ai/tool/transport/admin/route.go
internal/module/auth/transport/admin/route.go
internal/module/auth_platform/transport/admin/route.go
internal/module/clientversion/transport/admin/route.go
internal/module/crontask/transport/admin/route.go
internal/module/export/transport/admin/route.go
internal/module/mail/transport/admin/route.go
internal/module/notification/transport/admin/route.go
internal/module/operationlog/transport/admin/route.go
internal/module/payment/transport/admin/route.go
internal/module/payment/wallet/transport/admin/route.go
internal/module/permission/transport/admin/route.go
internal/module/profile/transport/admin/route.go
internal/module/queuemonitor/transport/admin/route.go
internal/module/realtime/transport/admin/route.go
internal/module/role/transport/admin/route.go
internal/module/sms/transport/admin/route.go
internal/module/system/transport/admin/route.go
internal/module/systemlog/transport/admin/route.go
internal/module/systemsetting/transport/admin/route.go
internal/module/uploadconfig/transport/admin/route.go
internal/module/uploadtoken/transport/admin/route.go
internal/module/user/transport/admin/route.go
```

The 16 exact temporary product route files are:

```text
internal/module/ai/asset/transport/canvas/route.go
internal/module/ai/audio/transport/canvas/route.go
internal/module/ai/chat/transport/canvas/route.go
internal/module/ai/image/transport/canvas/route.go
internal/module/ai/prompt/transport/canvas/route.go
internal/module/ai/video/transport/canvas/route.go
internal/module/auth/transport/app/route.go
internal/module/auth/transport/canvas/route.go
internal/module/canvas/transport/canvas/route.go
internal/module/payment/transport/canvas/route.go
internal/module/payment/wallet/transport/canvas/route.go
internal/module/profile/transport/app/route.go
internal/module/profile/transport/canvas/route.go
internal/module/uploadtoken/transport/app/route.go
internal/module/user/transport/app/route.go
internal/module/user/transport/canvas/route.go
```

- [x] **Step 1: Add a failing no-legacy-map guard**

Reject `permissionRouteRules`, `operationRouteRules`, middleware rule maps, or any active route registered directly with `GET/POST/PUT/PATCH/DELETE` instead of the registry. Temporary App/Canvas routes receive explicit policies and audit decisions but remain excluded from the Admin Contract Bundle until P09 deletes them.

- [x] **Step 2: Register definition and handler together**

Representative form:

```go
routes.Handle(adminroute.Definition{
	Method: http.MethodPost,
	Path: "/api/admin/v1/ai-conversations/:id/messages",
	OperationID: "sendAIConversationMessage",
	Access: adminroute.Authenticated(),
	Audit: adminroute.Audit("ai_message", "send", "发送AI消息"),
	RequestSchema: "SendAIMessageRequest",
	ResponseSchema: "AIReplyCommandResponse",
}, handler.Send)
```

Reads use `NoAudit("read-only")`. Current-user notification mutations use `NoAudit("self-service notification state")`. Public health/auth bootstrap and payment callback routes are explicitly Public. Every permission code must exist in the generated permission catalog.

- [x] **Step 3: Delete duplicates and regenerate contracts**

Compile the router, compare it with the approved Admin route golden, delete both old maps, regenerate the bundle, and verify every mutation has one audit decision.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/server/... ./internal/module/... -run 'TestRoute|TestPolicy|TestPermission|TestOperation' -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
$routeFiles = @(
  'internal/module/ai/agent/transport/admin/route.go','internal/module/ai/chat/transport/admin/route.go','internal/module/ai/conversation/transport/admin/route.go','internal/module/ai/knowledge/transport/admin/route.go','internal/module/ai/message/transport/admin/route.go','internal/module/ai/prompt/transport/admin/route.go','internal/module/ai/provider/transport/admin/route.go','internal/module/ai/run/transport/admin/route.go','internal/module/ai/tool/transport/admin/route.go','internal/module/auth/transport/admin/route.go','internal/module/auth_platform/transport/admin/route.go','internal/module/clientversion/transport/admin/route.go','internal/module/crontask/transport/admin/route.go','internal/module/export/transport/admin/route.go','internal/module/mail/transport/admin/route.go','internal/module/notification/transport/admin/route.go','internal/module/operationlog/transport/admin/route.go','internal/module/payment/transport/admin/route.go','internal/module/payment/wallet/transport/admin/route.go','internal/module/permission/transport/admin/route.go','internal/module/profile/transport/admin/route.go','internal/module/queuemonitor/transport/admin/route.go','internal/module/realtime/transport/admin/route.go','internal/module/role/transport/admin/route.go','internal/module/sms/transport/admin/route.go','internal/module/system/transport/admin/route.go','internal/module/systemlog/transport/admin/route.go','internal/module/systemsetting/transport/admin/route.go','internal/module/uploadconfig/transport/admin/route.go','internal/module/uploadtoken/transport/admin/route.go','internal/module/user/transport/admin/route.go',
  'internal/module/ai/asset/transport/canvas/route.go','internal/module/ai/audio/transport/canvas/route.go','internal/module/ai/chat/transport/canvas/route.go','internal/module/ai/image/transport/canvas/route.go','internal/module/ai/prompt/transport/canvas/route.go','internal/module/ai/video/transport/canvas/route.go','internal/module/auth/transport/app/route.go','internal/module/auth/transport/canvas/route.go','internal/module/canvas/transport/canvas/route.go','internal/module/payment/transport/canvas/route.go','internal/module/payment/wallet/transport/canvas/route.go','internal/module/profile/transport/app/route.go','internal/module/profile/transport/canvas/route.go','internal/module/uploadtoken/transport/app/route.go','internal/module/user/transport/app/route.go','internal/module/user/transport/canvas/route.go'
)
git add -- $routeFiles internal/module/notification/transport/admin/task_route.go internal/module/payment/transport/callback/route.go internal/server/routes_auth.go internal/server/routes_admin_ai.go internal/server/routes_admin_commerce_rbac.go internal/server/routes_admin_comms.go internal/server/routes_admin_foundation.go internal/server/routes_admin_user.go internal/server/routes_canvas.go internal/server/testdata/admin_route_policy_golden.json contracts/admin/v1/openapi.json contracts/admin/v1/permissions.json contracts/admin/v1/views.json contracts/admin/v1/realtime/envelope.schema.json contracts/admin/v1/realtime/events.schema.json contracts/admin/v1/manifest.json
git rm -- internal/bootstrap/route_meta.go internal/bootstrap/route_meta_test.go
git diff --cached --check
git commit -m "refactor(routing): colocate admin access and audit policy"
```

### Task 8: Prove cross-node revocation and secret rotation

**Files:**
- Create: `internal/module/auth/session_multinode_integration_test.go`
- Create: `scripts/tests/session-secret-rotation.tests.ps1`
- Create: `docs/runbooks/session-secret-rotation.md`
- Modify: `internal/runtime/api.go`

- [x] **Step 1: Test two API nodes**

Start two runtime instances against one MySQL/Redis pair. Authenticate on node A, warm node B cache, revoke on A, then poll B. Require denial within two seconds. Repeat for user disable, role change, and refresh reuse.

- [x] **Step 2: Define explicit rotation procedure**

The runbook uses a dual-key deployment window:

1. deploy `APP_SECRET_PREVIOUS` plus new `APP_SECRET`;
2. verify old access credentials authenticate while all newly issued credentials use the new key ID;
3. revoke remaining old refresh sessions or wait for the declared window;
4. remove `APP_SECRET_PREVIOUS` and prove old credentials fail.

The key ring carries an explicit `kid`. Config rejects identical current/previous values and more than one previous key. Refresh credentials remain database hashes and are revoked as part of the cutover; no silent config-only rotation is permitted.

- [x] **Step 3: Automate the rehearsal**

`session-secret-rotation.tests.ps1` starts old, dual, and new-key-only API instances, executes issue/authenticate/rotate/revoke, and scans logs for both secret values. It uses ephemeral generated secrets and deletes logs/binaries from a verified TEMP path.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/auth -run TestMultiNode -race -count=5
pwsh -NoProfile -File scripts/tests/session-secret-rotation.tests.ps1
git add -- internal/module/auth/session_multinode_integration_test.go scripts/tests/session-secret-rotation.tests.ps1 docs/runbooks/session-secret-rotation.md internal/runtime/api.go
git commit -m "test(auth): prove revocation and secret rotation across nodes"
```

### Task 9: Make identity and policy verification blocking

**Files:**
- Create: `scripts/verify-identity-routing.ps1`
- Modify: `scripts/verify-backend.ps1`
- Modify: `.github/workflows/verify-backend.yml`
- Create: `internal/architecture/identity_routing_test.go`

- [x] **Step 1: Add architecture guards**

Reject JS-readable refresh cookie fields in OpenAPI, `access_token` cookie fallback, refresh rotation without expected-hash condition, permission cache allow-on-error, route metadata maps, unknown permission codes, and unclassified mutations.

- [x] **Step 2: Implement the shared gate**

```powershell
go test ./internal/module/auth ./internal/module/permission ./internal/module/role ./internal/module/user ./internal/middleware ./internal/server/... -race -count=1
go test ./internal/architecture -run 'TestIdentity|TestRoutePolicy|TestCredential' -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
```

- [x] **Step 3: Run and commit**

```powershell
pwsh -NoProfile -File scripts/verify-identity-routing.ps1
pwsh -NoProfile -File scripts/verify-backend.ps1
git add -- scripts/verify-identity-routing.ps1 scripts/verify-backend.ps1 .github/workflows/verify-backend.yml internal/architecture/identity_routing_test.go
git commit -m "ci: enforce atomic identity and complete route policy"
```

## Completion evidence (2026-07-17)

- Execution stayed directly on `E:\admin\admin_back_go` branch `master`; no P04 worktree was created or used.
- Task commits: `c77dd8c`, `deec013`, `6841302`, `88f59bc`, `8c322b7`, `9594d86`, `a3af4a8`, `0438c54`, `ce636cf`, and `352badc`.
- Docker MySQL/Redis race suites proved transactional session limits, one-winner refresh CAS, fail-closed principal invalidation, and cross-node revoke/user-disable/role-change/refresh-reuse propagation. `TestMultiNode` passed with `-race -count=5`; the measured denial path remained inside the two-second SLA.
- `scripts/tests/session-secret-rotation.tests.ps1` passed the Docker old/dual/new-only rehearsal and its generated-secret log scan.
- `scripts/verify-identity-routing.ps1` passed host source checks plus the pinned Linux Docker race gate, architecture guards, and Admin Contract Bundle drift check.
- `scripts/verify-backend.ps1` passed all Go tests, runtime and identity Linux race gates, `go vet`, pinned `staticcheck`, `govulncheck` with zero called vulnerabilities, and both process builds.
- Browser responses expose no refresh credential; desktop refresh remains one-time response data; all active routes have access policy and every mutation has an explicit audit/no-audit decision.

## Plan completion gate

```powershell
pwsh -NoProfile -File scripts/verify-identity-routing.ps1
pwsh -NoProfile -File scripts/tests/session-secret-rotation.tests.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git status --short
```

Expected: one winner in 20-way refresh and session races; cross-node revocation meets two seconds; cache failures fail closed; browser/desktop credential contracts expose no browser refresh token; every active route has one access policy and each mutation has one audit decision; status is clean.
