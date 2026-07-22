# Auth Session Policy Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make refresh TTL the sole login-persistence policy and `max_sessions` the sole concurrent-session policy while simplifying both Admin interactions.

**Architecture:** Remove `single_session` from the Admin management contract and canonical schema, then derive strict pointer enforcement from `max_sessions == 1`. Keep access credentials memory-only and always persist only the last successful non-secret login account/type. Expose session capacity as a three-mode frontend control mapped to `0`, `1`, or `2..100`.

**Tech Stack:** Go 1.26, Gin, GORM, MySQL 8.4, Atlas, Vue 3, TypeScript, Element Plus, Zod, Vitest, Playwright

---

## File Structure

- Backend policy and API: `internal/module/auth/session_contract.go`, `internal/module/auth/session_lifecycle.go`, `internal/module/auth_platform/*`, `internal/admincontract/*`.
- Database: `database/schema/admin.hcl`, a new forward migration under `database/migrations/`, `database/migrations/atlas.sum`, and current local MySQL.
- Frontend login: `src/views/Login/composables/useLoginForm.ts`, `src/views/Login/components/LoginFormCard.vue`, and focused tests.
- Frontend platform policy: `src/views/Main/permission/authPlatform/*`, `src/api/permission/authPlatform.ts`, locale files, and focused tests.
- Generated contract: backend `contracts/admin/v1/` and frontend `contracts/backend/admin/`, HTTP types, operations, permissions, and views.

### Task 1: Lock Backend Session Semantics

**Files:**
- Modify: `internal/module/auth_platform/service_test.go`
- Modify: `internal/module/auth/session_issue_integration_test.go`
- Modify: `internal/module/auth_platform/transport/admin/handler_test.go`
- Modify: `internal/admincontract/openapi_test.go`

- [ ] **Step 1: Write failing tests**

Assert that platform policy maps `max_sessions=1` to strict single-session,
`max_sessions=0` to unlimited, and `max_sessions=3` to capacity three. Remove
`single_session` from request fixtures and assert generated request/list schemas
do not publish that property.

- [ ] **Step 2: Verify RED**

```powershell
go test ./internal/module/auth_platform ./internal/module/auth ./internal/admincontract -count=1
```

Expected: failures show policy still reads `single_session` and the Admin contract
still requires/publishes it.

- [ ] **Step 3: Implement the backend policy and management contract**

Remove `SingleSession` from platform DTO/model/request/write maps. Build policy as:

```go
MaxSessions:              row.MaxSessions,
SingleSessionPerPlatform: row.MaxSessions == 1,
```

Keep lifecycle eviction rules unchanged: `1` revokes all old sessions and uses
the Redis pointer; `0` revokes none; values above one revoke oldest overflow.

- [ ] **Step 4: Verify GREEN**

Run the Task 1 command and require all packages to pass.

### Task 2: Remove Redundant Storage

**Files:**
- Modify: `database/schema/admin.hcl`
- Create: `database/migrations/202607220101_auth_platform_session_policy.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `internal/architecture/platform_kernel_test.go`
- Modify: `scripts/browser-only/verify-retirement.ps1`

- [ ] **Step 1: Add failing schema assertions**

Assert canonical schema and new migration contain no active
`auth_platforms.single_session` column while retaining `max_sessions` with range
`0..100`.

- [ ] **Step 2: Verify RED**

```powershell
go test ./internal/architecture -run PlatformKernel -count=1
```

- [ ] **Step 3: Add and hash the forward migration**

Use one destructive statement:

```sql
ALTER TABLE `auth_platforms` DROP COLUMN `single_session`;
```

Remove the column from `admin.hcl`, recalculate `atlas.sum` using the repository
database verifier workflow, and update the current browser-only policy probe to
assert only `max_sessions=1`.

- [ ] **Step 4: Apply and verify local MySQL**

Apply the migration to `admin`, then prove the column is absent and the active
Admin row still has `max_sessions=1`.

### Task 3: Simplify Login Account Persistence

**Files:**
- Modify: `tests/shared/user/login-captcha-state.test.ts`
- Modify: `tests/component/login/LoginForm.test.ts`
- Modify: `tests/component/accessibility/login.test.ts`
- Modify: `src/views/Login/composables/useLoginForm.ts`
- Modify: `src/views/Login/components/LoginFormCard.vue`
- Modify: `src/views/Login/index.vue`
- Modify: `src/i18n/locales/zh-CN/auth.ts`
- Modify: `src/i18n/locales/en-US/auth.ts`

- [ ] **Step 1: Write failing interaction tests**

Assert successful login always writes:

```ts
{ rememberedLogin: { account: 'normalized-account', type: 'password' } }
```

Assert the login card has no remember checkbox or `update:remember` event.

- [ ] **Step 2: Verify RED**

```powershell
npm test -- tests/shared/user/login-captcha-state.test.ts tests/component/login/LoginForm.test.ts tests/component/accessibility/login.test.ts
```

- [ ] **Step 3: Implement automatic persistence**

Delete `loginForm.remember`, rename `rememberPwd` to `rememberLoginAccount`, call
it only after successful kernel login, and always overwrite `rememberedLogin`.
Delete the checkbox, event, and obsolete locale key. Never persist secret fields.

- [ ] **Step 4: Verify GREEN**

Run the Task 3 command and require all tests to pass.

### Task 4: Replace Auth Platform Controls

**Files:**
- Create: `tests/unit/auth-platform/session-policy.test.ts`
- Modify: `src/views/Main/permission/authPlatform/helpers.ts`
- Modify: `src/views/Main/permission/authPlatform/components/FormDialog.vue`
- Modify: `src/views/Main/permission/authPlatform/index.vue`
- Modify: `src/api/permission/authPlatform.ts`
- Modify: `src/i18n/locales/zh-CN/auth.ts`
- Modify: `src/i18n/locales/en-US/auth.ts`

- [ ] **Step 1: Write failing mapping tests**

Define expected pure mappings:

```ts
expect(sessionModeFromMaxSessions(0)).toBe('unlimited')
expect(sessionModeFromMaxSessions(1)).toBe('single')
expect(sessionModeFromMaxSessions(5)).toBe('limited')
expect(maxSessionsForMode('limited', 1)).toBe(2)
```

- [ ] **Step 2: Verify RED**

```powershell
npm test -- tests/unit/auth-platform/session-policy.test.ts
```

- [ ] **Step 3: Implement the interaction**

Add the pure mapping helpers. Replace the two form controls with `el-segmented`
for `single`, `limited`, and `unlimited`; show the numeric input only for
`limited`. Replace two list columns with a single localized policy column and
remove `single_session` from all payload normalization.

- [ ] **Step 4: Verify GREEN**

Run Task 4 tests, frontend typecheck, and locale generation/check.

### Task 5: Regenerate The Admin Contract

**Files:**
- Modify: `contracts/admin/v1/*`
- Modify: `contracts/backend/admin/*`
- Modify: `src/modules/http/generated/admin.ts`
- Modify: `src/modules/http/generated/operations.ts`
- Modify: `src/modules/routing/generated/permissions.ts`
- Modify: `src/modules/routing/generated/views.ts`

- [ ] **Step 1: Generate and check backend bundle**

Commit backend source first, run `scripts/generate-admin-contract.ps1`, then run
`scripts/check-admin-contract.ps1`.

- [ ] **Step 2: Sync and generate frontend types**

Sync using the manifest backend commit, run `npm run contract:generate`, and run
`npm run contract:check`.

### Task 6: Runtime And Delivery Verification

**Files:**
- Modify: this plan with observed evidence.

- [ ] **Step 1: Run full backend checks**

```powershell
go test ./... -count=1
pwsh -NoProfile -File scripts/verify-database.ps1
git diff --check
```

- [ ] **Step 2: Run full frontend checks**

```powershell
npm test
npm run typecheck
npm run lint
npm run build
npm run contract:check
npm run routes:check
npm run locale:check
git diff --check
```

- [ ] **Step 3: Verify admin-dev in a browser**

Confirm login has no remember checkbox, successful login still restores, the
platform dialog renders the segmented policy control, limited mode reveals a
numeric input, and save/reload preserves values without console errors.

- [ ] **Step 4: Commit both master repositories**

Use scoped backend and frontend commits. Do not create a worktree or push unless
the user requests it.
