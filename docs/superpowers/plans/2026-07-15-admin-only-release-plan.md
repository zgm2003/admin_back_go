# Current-Admin Release and Extensible Platform Kernel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task directly on `master`. Do not use subagents or worktrees unless the user explicitly changes that rule. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the retired App/Canvas product implementations and classified data, preserve the extensible platform kernel for the next independently authenticated platform, physically retire the P08R-frozen `client_versions` history after explicit approval, retain independently tested generic capabilities, and produce one synchronized, immutable, rollback-ready Browser-only Admin release.

**Architecture:** Product transports and legacy workflow terminology are deleted before database contract DDL, while `auth_platforms`, platform-aware RBAC, sessions, login logs, notification audiences, provenance columns, and their indexes remain generic. The runtime adapter registry contains only Admin after P09; database configuration alone never activates a platform, and every trusted transport injects its own registered provenance. Generic AI/provider/storage capabilities survive behind transport-neutral interfaces and direct tests. A final two-repository proof locks backend/frontend Docker image digests, the Browser-only Admin Contract Bundle, database fingerprint, recovery/query/COS-disposition evidence, and runbooks to one generated release manifest. No desktop artifact or GitHub Workflow belongs to the release.

**Tech Stack:** Go, MySQL 8.4, Atlas 0.38.0, Vue/TypeScript, Docker Compose, PowerShell 7.

---

## Current execution order and contract boundary

This plan implements
`docs/superpowers/specs/2026-07-20-extensible-platform-kernel-design.md`.
“Current Admin-only” describes the adapters and active product data shipped by
this release; it never means that the platform kernel or database schema is a
permanent singleton.

Task numbers group related artifacts; they are not permission to make the
frontend outrun the formal backend contract. Execute the groups in this order:

```text
Task 1 (complete: 87b86c4)
→ Task 3 (complete: db8c458)
→ Tasks 4–5
→ Task 6 implementation and non-mutating validation
→ fresh destructive approval
→ Task 6 approved disposable-restore rehearsals and contract DDL
→ Task 7 backend bundle publication
→ Task 2 together with Task 7 frontend bundle consumption
→ Tasks 8–9
```

The frontend Task 2 changes API fields, operations, and scene values, so it may
start only after Task 7 has published the matching formal backend bundle. This
preserves `admin_front_ts/docs/rule.md`: no handwritten frontend API type may
anticipate an unpublished backend contract. Tasks 3–6 are not a deployable
mixed-version release; user-visible Docker promotion happens only after both
repositories consume the same Task 7 bundle.

Windows development uses `admin-dev` (Docker MySQL/Redis plus host
Vite/API/Worker) for focused feedback. Every authoritative frontend gate and
the final release still use exact `node:24.18.0-alpine` and full Docker.

The 2026-07-20 preflight fingerprint and a fresh read-only live-schema query
also corrected an older database assumption: `users_quick_entry`,
`canvas_prompts`, `canvas_assets`, `ai_billing_rules`, and
`ai_billing_records` are already absent because reviewed legacy migrations ran
before P09. P09 must prove that absence and must not recreate or pretend to
drop those tables. The remaining physical product tables are the empty
`canvas_video_tasks` compatibility table and the eight-row frozen
`client_versions` history; `user_sessions.access_token_hash` is also still
present. Current `ai_video_tasks` is retained.

The P09 platform invariant is two-part:

1. the compiled runtime and active product data contain only the Admin adapter;
2. the generic platform configuration, RBAC, session, login-log, notification,
   provenance, and indexing seams remain available for a separately contracted
   future adapter.

An `auth_platforms` row is configuration, not a route. A future platform becomes
runnable only after its adapter, registry entry, permissions, and formal
Contract Bundle are committed. Admin routes never trust a client-supplied
platform value.

## Destructive-stage prerequisites

P09 stops before DDL unless P01-P08R and all P07 tasks are committed and accepted, both primary checkout working directories are clean, no Git worktree registration/directory exists, every active program gate is green, the latest recovery artifact restores, retained COS keys are reachable, every legacy row has an explicit disposition, P08R proves `client_versions` has had no runtime reader/writer since cutover, and the frontend contract lock matches the backend bundle manifest.

P09 reads and writes only `E:/admin/admin_back_go` and `E:/admin/admin_front_ts`. It does not inspect, lock, or modify any third repository. Immediately before destructive DDL—including `DROP TABLE client_versions`—it must stop for fresh explicit user approval.

## File map

- `release/admin-only/` owns tracked schemas and the immutable pre-contract input lock. Generated release output lives below ignored `release/admin-only/out/`.
- `scripts/release/` owns input locking, release-manifest validation, deployment, rollback, and full proof orchestration.
- `database/reconciliation/050`–`053` own pre/post destructive invariants; `database/migrations/202607150201`–`203` own the serialized Atlas contract groups, including the approved physical removal of frozen `client_versions`.
- `internal/module/ai/capability/` owns transport-neutral generation scene names. Retained text/image/video/audio services use these names and direct behavior tests.
- `internal/architecture/admin_only_test.go`, `internal/architecture/platform_kernel_test.go`, and `tests/shared/architecture/admin-only.test.ts` jointly guard retired product removal and platform-kernel preservation.
- Backend `contracts/admin/v1/` remains contract truth; frontend `contracts/backend/admin/v1/` and `contracts/backend/admin/lock.json` remain the exact consumer lock.

### Task 1: Freeze the pre-contract release inputs (complete: `87b86c4`)

**Files:**
- Create: `release/admin-only/input-lock.json`
- Create: `release/admin-only/input-lock.schema.json`
- Create: `release/admin-only/.gitignore`
- Create: `scripts/release/lock-inputs.ps1`
- Create: `scripts/release/check-inputs.ps1`
- Create: `scripts/tests/release-input-lock.tests.ps1`
- Create: `database/reconciliation/050_contract_preconditions.sql`
- Create: `docs/runbooks/admin-only-data-disposition.md`

- [x] **Step 1: Define and test the release lock**

`input-lock.schema.json` requires `schema_version: 1`, 40-character lowercase Git SHAs, 64-character lowercase SHA-256 values, and these fields:

```text
backend_commit
frontend_commit
contract_manifest_sha256
database_fingerprint_sha256
recovery_artifact_sha256
cos_disposition_evidence_sha256
query_evidence_sha256
client_versions_freeze_evidence_sha256
```

`lock-inputs.ps1` uses an ordered object populated only from verified commands and artifacts:

```powershell
$lock = [ordered]@{
  schema_version = 1
  backend_commit = Assert-GitSha (git -C $BackendRoot rev-parse HEAD)
  frontend_commit = Assert-GitSha (git -C $FrontendRoot rev-parse HEAD)
  contract_manifest_sha256 = Get-FileSha256 "$BackendRoot/contracts/admin/v1/manifest.json"
  database_fingerprint_sha256 = Get-EvidenceDigest $DatabaseFingerprint
  recovery_artifact_sha256 = Get-EvidenceDigest $RecoveryArtifact
  cos_disposition_evidence_sha256 = Get-EvidenceDigest $CosDispositionEvidence
  query_evidence_sha256 = Get-EvidenceDigest $QueryEvidence
  client_versions_freeze_evidence_sha256 = Get-EvidenceDigest $ClientVersionsFreezeEvidence
}
```

It requires a clean frontend primary checkout. In the backend it permits only the exact Task 1 untracked/modified paths declared above, so the lock can be generated before their single commit; any runtime, contract, migration, or unrelated change fails. It rejects any registered secondary Git worktree, any path outside the two declared repositories, missing P08R freeze/COS-disposition evidence, a recovery artifact that has not passed restore verification, a `client_versions` count/hash that differs from the P08R cutover proof, or any command that would print a DSN/token. It writes JSON through a temporary file and atomic rename. `check-inputs.ps1` validates the schema and recomputes every evidence digest without printing artifact content. Its final `-CheckOnly` mode treats backend/frontend commits as the intentionally frozen pre-contract inputs and checks ancestry plus evidence, not equality with later P09 HEAD commits.

- [x] **Step 2: Encode every legacy disposition**

The runbook and its executable count queries classify exactly:

- App/Canvas sessions and login attempts: preserve counts in recovery evidence, revoke, then delete;
- App/Canvas permissions and role grants: delete grants before permissions;
- pre-existing role grants whose permission row is already absent: lock the
  exact grant/role/permission IDs as orphan evidence and delete only that
  classified set; never infer a replacement permission;
- App/Canvas `auth_platforms` rows: delete; the Admin policy row remains enabled;
- `users_quick_entry`: already absent before P09; preserve the historical P02
  107-row / 3-active-row evidence, prove current absence, and do not recreate
  or claim a new P09 drop;
- `all` notification tasks/notifications: convert to `admin`; App/Canvas-only rows: delete;
- non-Admin exports: verify artifact/object evidence, then delete task rows;
- Canvas AI run/task/file/billing rows: preserve exact object keys and counts, delete dependents before owners, do not delete COS objects during this migration, and never delete provider/agent/storage configuration solely because a retired transport used it;
- agent scenes: rename `canvas_text_generate`, `canvas_image_generate`,
  `canvas_video_generate`, and `canvas_audio_generate` to `text_generate`,
  `image_generate`, `video_generate`, and `audio_generate`; the already-absent
  legacy billing tables are verified absent rather than referenced;
- `canvas_video_tasks`: remove product rows, then drop the retired table; P02 already created the canonical `ai_video_tasks` capability table;
- `canvas_prompts` and `canvas_assets`: already absent before P09 after their
  source-to-`ai_prompts`/`ai_assets` migration; verify absence and retain the
  P02 evidence without recreating or dropping them again.
- `client_versions`: preserve the P08R row count/hash in the input lock, prove no active route/task/menu/grant/runtime package references the table, record the exact historical COS object retention/deletion decision, then drop the table only in Task 6 after the fresh destructive approval.

No row receives an owner, platform, scene, kind, or object key without a documented source rule.

- [x] **Step 3: Make preconditions zero-row invariants**

`050_contract_preconditions.sql` returns named violation result sets for unresolved retained object URLs, wallet mismatches, unclassified RBAC/payment/AI/export/notification orphans, duplicate idempotency keys, running/claimed durable work, active App/Canvas sessions, unknown platform values, missing scene mappings, non-terminal provider attempts, active client-version menu/grants/routes, any runtime reference to `client_versions`, and any unexpected reappearance of the five already-absent legacy tables. The PowerShell input checker owns external evidence-file digest validation. SQL verifies the exact pre-existing orphan-grant set, the live `client_versions` count/hash, and the P08R freeze evidence. `admin-db invariants` must fail on any returned row.

- [x] **Step 4: Run and commit**

```powershell
pwsh -NoProfile -File scripts/tests/release-input-lock.tests.ps1
pwsh -NoProfile -File scripts/release/lock-inputs.ps1
pwsh -NoProfile -File scripts/release/check-inputs.ps1
go run ./cmd/admin-db invariants --schema admin --file database/reconciliation/050_contract_preconditions.sql
git add -- release/admin-only/input-lock.json release/admin-only/input-lock.schema.json release/admin-only/.gitignore scripts/release/lock-inputs.ps1 scripts/release/check-inputs.ps1 scripts/tests/release-input-lock.tests.ps1 database/reconciliation/050_contract_preconditions.sql docs/runbooks/admin-only-data-disposition.md
git commit -m "chore(release): lock admin-only contract inputs"
```

Expected: all lock fields are literal current values, precondition violations are zero, and no secret or dump is tracked.

### Task 2: Remove retired frontend product branches while preserving platform administration

**Repository:** `E:/admin/admin_front_ts`

**Files:**
- Modify: `src/lib/http/headers.ts`
- Modify: `.env.development`
- Modify: `.env.production`
- Modify: `src/vite-env.d.ts`
- Modify: `src/app/environment.ts`
- Modify: `src/enums/index.ts`
- Modify: `src/api/ai/agents.ts`
- Modify: `src/api/ai/runs.ts`
- Modify: `src/api/permission/authPlatform.ts`
- Modify: `src/api/permission/permission.ts`
- Modify: `src/api/permission/role.ts`
- Modify: `src/api/system/notificationTask.ts`
- Modify: `src/api/user/users.ts`
- Modify: `src/features/ai-runs/workflow.ts`
- Modify: `src/features/notifications/workflow.ts`
- Modify: `src/features/user-management/workflow.ts`
- Modify: `src/views/Main/ai/agents/index.vue`
- Modify: `src/views/Main/ai/agents/use-agent-admin-page.ts`
- Modify: `src/views/Main/ai/runs/components/RunList/index.vue`
- Modify: `src/views/Main/ai/runs/components/RunStats/index.vue`
- Modify: `src/views/Main/permission/authPlatform/index.vue`
- Modify: `src/views/Main/permission/authPlatform/helpers.ts`
- Modify: `src/views/Main/permission/authPlatform/components/FormDialog.vue`
- Modify: `src/views/Main/permission/permission/index.vue`
- Modify: `src/views/Main/permission/permission/helpers.ts`
- Modify: `src/views/Main/permission/permission/composables/usePermissionDefinitionPage.ts`
- Modify: `src/views/Main/permission/permission/components/PermissionDefinitionDialog.vue`
- Modify: `src/views/Main/permission/permission/components/PlatformTabs.vue`
- Modify: `src/views/Main/permission/role/index.vue`
- Modify: `src/views/Main/permission/role/role-matrix.ts`
- Modify: `src/views/Main/permission/role/use-role-page.ts`
- Modify: `src/views/Main/permission/role/components/RolePermissionMatrix.vue`
- Modify: `src/views/Main/system/notificationTask/index.vue`
- Modify: `src/views/Main/system/notificationTask/use-notification-task-page.ts`
- Modify: `src/views/Main/user/userManager/components/SessionList/index.vue`
- Modify: `src/api/user/usersLoginLog.ts`
- Modify: `src/views/Main/user/usersLoginLog/index.vue`
- Modify: `src/views/Main/user/usersLoginLog/composables/useUsersLoginLogTable.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Modify: `src/i18n/locales/en-US/permission.ts`
- Modify: `src/i18n/locales/en-US/system.ts`
- Modify: `src/i18n/locales/en-US/user.ts`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/zh-CN/permission.ts`
- Modify: `src/i18n/locales/zh-CN/system.ts`
- Modify: `src/i18n/locales/zh-CN/user.ts`
- Create: `scripts/check-admin-only.mjs`
- Create: `tests/shared/architecture/admin-only.test.ts`
- Create: `tests/integration/features/admin-auth-policy.test.ts`
- Create: `tests/integration/features/platform-kernel.test.ts`
- Modify: `tests/unit/app/environment.test.ts`
- Modify: `tests/integration/app/kernel.test.ts`
- Modify: `tests/integration/features/user-management.test.ts`
- Modify: `tests/integration/features/notifications.test.ts`
- Modify: `tests/integration/features/ai-runs.test.ts`

- [ ] **Step 1: Add failing retired-product and platform-kernel guards**

The source guard parses production TypeScript/Vue and locale values. It excludes DOM `HTMLCanvasElement`, Canvas rendering APIs, and CSS class names. It rejects retired product values/imports rather than the substring alone:

```text
/api/app/
/api/canvas/
PlatformEnum.APP
PlatformEnum.CANVAS
VITE_PLATFORM
canvas_text_generate
canvas_image_generate
canvas_video_generate
canvas_audio_generate
user-agent based product inference
App/Canvas product labels in active menu/locale values
```

The same guard must fail if the generic platform kernel disappears. It requires
the auth-platform view and API, add/edit/delete/status permissions, permission
platform selection, role platform tabs, session/login-log platform filters,
and notification audience selection. It must not assert that every platform
type is the literal `admin`.

`platform-kernel.test.ts` uses the current formal Admin option to prove UI
adapters pass the exact selected platform to permission, role, session/log,
and notification APIs without fallback chains. It asserts that platform types
come from generated operations and dictionaries rather than a handwritten
`'admin' | 'app' | 'canvas'` union or literal runtime whitelist. It must not
invent a `partner_portal` mock before that code exists in a future formal
Bundle.

Run:

```powershell
npm test -- tests/shared/architecture/admin-only.test.ts tests/integration/features/platform-kernel.test.ts
```

Expected: FAIL on the current product helper, env key, old scene values, and
any platform-kernel control removed by the earlier singleton draft.

- [ ] **Step 2: Keep Admin transport truth without collapsing management**

P08R already removed `getPlatform` and `src/lib/http/platform.ts`; keep
`src/lib/http/headers.ts` on the literal Admin provenance required by the Admin
transport contract. This header cannot select another runtime adapter.

Retain the complete auth-platform CRUD/status page, permission platform field,
role platform tabs, session/login-log platform filters, and notification
audience selector. Their options come only from the formal backend response;
the frontend does not invent a second platform or infer one from the browser.
Remove handwritten platform whitelists; generated schemas and exact page-init
dictionaries validate the current options. Remove only App/Canvas literals and
branches. Scene values become:

```text
text_generate
image_generate
video_generate
audio_generate
```

P08R has already removed native client platforms, updater targets, and
client-version UI/contracts; this task must not recreate them. Do not edit
generated backend contracts in this task—Task 7 replaces them from backend
truth. Do not handwrite a compatibility type while waiting for Task 7.

- [ ] **Step 3: Verify and commit exact source files**

```powershell
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:24.18.0-alpine sh -lc "npm ci && npm test -- tests/shared/architecture/admin-only.test.ts tests/unit/app/environment.test.ts && npm test -- --project integration tests/integration/app/kernel.test.ts tests/integration/features/admin-auth-policy.test.ts tests/integration/features/platform-kernel.test.ts tests/integration/features/user-management.test.ts tests/integration/features/notifications.test.ts tests/integration/features/ai-runs.test.ts && npm run typecheck && npm run lint"
git add -- .env.development .env.production src/vite-env.d.ts src/app/environment.ts src/enums/index.ts src/lib/http/headers.ts src/api/ai/agents.ts src/api/ai/runs.ts src/api/permission/authPlatform.ts src/api/permission/permission.ts src/api/permission/role.ts src/api/system/notificationTask.ts src/api/user/users.ts src/api/user/usersLoginLog.ts src/features/ai-runs/workflow.ts src/features/notifications/workflow.ts src/features/user-management/workflow.ts src/views/Main/ai/agents/index.vue src/views/Main/ai/agents/use-agent-admin-page.ts src/views/Main/ai/runs/components/RunList/index.vue src/views/Main/ai/runs/components/RunStats/index.vue src/views/Main/permission/authPlatform/index.vue src/views/Main/permission/authPlatform/helpers.ts src/views/Main/permission/authPlatform/components/FormDialog.vue src/views/Main/permission/permission/index.vue src/views/Main/permission/permission/helpers.ts src/views/Main/permission/permission/composables/usePermissionDefinitionPage.ts src/views/Main/permission/permission/components/PermissionDefinitionDialog.vue src/views/Main/permission/permission/components/PlatformTabs.vue src/views/Main/permission/role/index.vue src/views/Main/permission/role/role-matrix.ts src/views/Main/permission/role/use-role-page.ts src/views/Main/permission/role/components/RolePermissionMatrix.vue src/views/Main/system/notificationTask/index.vue src/views/Main/system/notificationTask/use-notification-task-page.ts src/views/Main/user/userManager/components/SessionList/index.vue src/views/Main/user/usersLoginLog/index.vue src/views/Main/user/usersLoginLog/composables/useUsersLoginLogTable.ts src/i18n/locales/en-US/ai.ts src/i18n/locales/en-US/permission.ts src/i18n/locales/en-US/system.ts src/i18n/locales/en-US/user.ts src/i18n/locales/zh-CN/ai.ts src/i18n/locales/zh-CN/permission.ts src/i18n/locales/zh-CN/system.ts src/i18n/locales/zh-CN/user.ts scripts/check-admin-only.mjs tests/shared/architecture/admin-only.test.ts tests/integration/features/admin-auth-policy.test.ts tests/integration/features/platform-kernel.test.ts tests/unit/app/environment.test.ts tests/integration/app/kernel.test.ts tests/integration/features/user-management.test.ts tests/integration/features/notifications.test.ts tests/integration/features/ai-runs.test.ts
git diff --cached --check
git commit -m "refactor(frontend): retire legacy products without collapsing platform admin"
```

Expected: source-only guard, platform-kernel integration tests, typecheck, and
zero-warning lint pass; generated-contract synchronization remains intentionally
pending Task 7.

### Task 3: Remove backend App/Canvas route registration and transports (complete: `db8c458`)

**Files:**
- Delete: `internal/server/routes_canvas.go`
- Modify: `internal/server/router.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/server/routes_auth.go`
- Modify: `internal/server/routes_admin_user.go`
- Modify: `internal/server/routes_admin_commerce_rbac.go`
- Modify: `internal/server/routes_admin_comms.go`
- Delete: `internal/module/auth/transport/app/`
- Delete: `internal/module/auth/transport/canvas/`
- Delete: `internal/module/profile/transport/app/`
- Delete: `internal/module/profile/transport/canvas/`
- Delete: `internal/module/uploadtoken/transport/app/`
- Delete: `internal/module/user/transport/app/`
- Delete: `internal/module/user/transport/canvas/`
- Delete: `internal/module/payment/transport/canvas/`
- Delete: `internal/module/payment/wallet/transport/canvas/`
- Delete: `internal/module/ai/asset/transport/canvas/`
- Delete: `internal/module/ai/audio/transport/canvas/`
- Delete: `internal/module/ai/chat/transport/canvas/`
- Delete: `internal/module/ai/image/transport/canvas/`
- Delete: `internal/module/ai/prompt/transport/canvas/`
- Delete: `internal/module/ai/video/transport/canvas/`
- Delete: `internal/module/ai/internal/canvasrequest/`
- Delete: `internal/module/canvas/`
- Delete: `internal/platform/retired/`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/middleware/auth_token.go`
- Modify: `internal/middleware/auth_token_test.go`
- Delete: `scripts/check-canvas-rbac.ps1`
- Modify: `scripts/full-admin-smoke.ps1`
- Delete: `internal/architecture/canvas_free_agent_scenes_test.go`
- Delete: `internal/architecture/canvas_front_next_integration_test.go`
- Delete: `internal/architecture/multiplatform_boundary_test.go`
- Delete: `internal/architecture/multiplatform_phase2_closure_test.go`
- Delete: `internal/architecture/platform_route_line_test.go`
- Create: `internal/architecture/admin_only_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/testdata/admin_routes_golden.txt`

- [x] **Step 1: Add route, dependency, and public-path guards**

`admin_only_test.go` compiles the router and asserts no `/api/app` or `/api/canvas` route, no import of a retired transport/module, no retired public auth path, and no App/Canvas permission seed outside `database/legacy-migrations`. It also asserts the compiled Admin route/policy registry equals both golden files.

- [x] **Step 2: Remove registration before deleting packages**

Delete App/Canvas calls, dependency fields, imports, auth public paths, and platform inference from the runtime graph. Run server/Admin tests while the old packages still exist:

```powershell
go test ./internal/server ./internal/middleware ./internal/architecture -run 'TestAdminOnly|TestRoute|TestAuth' -count=1
```

Expected: the binary exposes only Admin, health/readiness, metrics if configured, and required payment callback routes.

- [x] **Step 3: Delete the now-unreachable product code**

Delete only the directories/files declared above. Retain provider, engine, run-recorder, prompt, asset, storage, image, video, audio, and chat capability packages for Task 5 classification.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/server ./internal/middleware ./internal/architecture -run 'TestAdminOnly|TestRoute|TestAuth' -count=1
go test ./internal/module/... -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git add -- internal/server/router.go internal/server/router_test.go internal/server/routes_auth.go internal/server/routes_admin_user.go internal/server/routes_admin_commerce_rbac.go internal/server/routes_admin_comms.go internal/platform/admin/build.go internal/middleware/auth_token.go internal/middleware/auth_token_test.go scripts/full-admin-smoke.ps1 internal/architecture/admin_only_test.go internal/server/testdata/admin_route_policy_golden.json internal/server/testdata/admin_routes_golden.txt
git add -u -- internal/server/routes_canvas.go internal/module/auth/transport/app internal/module/auth/transport/canvas internal/module/profile/transport/app internal/module/profile/transport/canvas internal/module/uploadtoken/transport/app internal/module/user/transport/app internal/module/user/transport/canvas internal/module/payment/transport/canvas internal/module/payment/wallet/transport/canvas internal/module/ai/asset/transport/canvas internal/module/ai/audio/transport/canvas internal/module/ai/chat/transport/canvas internal/module/ai/image/transport/canvas internal/module/ai/prompt/transport/canvas internal/module/ai/video/transport/canvas internal/module/ai/internal/canvasrequest internal/module/canvas internal/platform/retired scripts/check-canvas-rbac.ps1 internal/architecture/canvas_free_agent_scenes_test.go internal/architecture/canvas_front_next_integration_test.go internal/architecture/multiplatform_boundary_test.go internal/architecture/multiplatform_phase2_closure_test.go internal/architecture/platform_route_line_test.go
git diff --cached --check
git commit -m "refactor(backend): remove app and canvas transports"
```

Expected: no retired route/import remains and retained capability tests still pass.

### Task 4: Retire legacy product values while preserving the platform kernel

**Files:**
- Modify: `internal/shared/enum/platform.go`
- Modify: `internal/shared/dict/dict.go`
- Modify: `internal/shared/dict/options_test.go`
- Modify: `internal/shared/validate/platform.go`
- Modify: `internal/shared/validate/validate_test.go`
- Modify: `internal/module/auth_platform/dto.go`
- Modify: `internal/module/auth_platform/errors.go`
- Modify: `internal/module/auth_platform/repository.go`
- Modify: `internal/module/auth_platform/service.go`
- Modify: `internal/module/auth_platform/service_test.go`
- Modify: `internal/module/auth_platform/management_service_test.go`
- Modify: `internal/module/auth_platform/management_i18n_test.go`
- Modify: `internal/module/auth_platform/transport/admin/request.go`
- Modify: `internal/module/auth_platform/transport/admin/handler.go`
- Modify: `internal/module/auth_platform/transport/admin/handler_test.go`
- Modify: `internal/module/auth_platform/transport/admin/handler_i18n_test.go`
- Modify: `internal/module/auth_platform/transport/admin/route.go`
- Modify: `internal/module/permission/dto.go`
- Modify: `internal/module/permission/service.go`
- Modify: `internal/module/permission/service_test.go`
- Modify: `internal/module/permission/management_service_test.go`
- Modify: `internal/module/permission/transport/admin/request.go`
- Modify: `internal/module/permission/transport/admin/handler.go`
- Modify: `internal/module/permission/transport/admin/handler_test.go`
- Modify: `internal/module/role/dto.go`
- Modify: `internal/module/role/service.go`
- Modify: `internal/module/role/service_test.go`
- Modify: `internal/module/notification/task/dto.go`
- Modify: `internal/module/notification/task/service.go`
- Modify: `internal/module/notification/task/service_test.go`
- Modify: `internal/module/notification/task/i18n_test.go`
- Modify: `internal/module/notification/task/jobs_test.go`
- Modify: `internal/module/notification/task/repository.go`
- Modify: `internal/module/notification/transport/admin/task_request.go`
- Modify: `internal/module/notification/transport/admin/task_handler.go`
- Modify: `internal/module/auth/loginlog.go`
- Modify: `internal/module/auth/loginlog_test.go`
- Modify: `internal/module/auth/session_admin.go`
- Modify: `internal/module/auth/session_test.go`
- Modify: `internal/module/auth/transport/admin/request.go`
- Modify: `internal/module/auth/transport/admin/handler.go`
- Modify: `internal/module/auth/transport/admin/handler_test.go`
- Modify: `internal/module/user/service.go`
- Modify: `internal/module/user/service_test.go`
- Modify: `internal/module/ai/run/recorder.go`
- Modify: `internal/module/ai/run/recorder_test.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/module/export/notifier.go`
- Modify: `internal/middleware/auth_token.go`
- Modify: `internal/middleware/auth_token_test.go`
- Modify: `internal/server/routes_admin_commerce_rbac.go`
- Modify: `internal/server/routes_admin_comms.go`
- Modify: `internal/server/routes_admin_user.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/module/role/transport/admin/handler_test.go`
- Create: `internal/architecture/platform_kernel_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `scripts/full-admin-smoke.ps1`

- [ ] **Step 1: Write failing registry and kernel-preservation tests**

Treat the enum registry as the compile-time adapter registry, not as the
database configuration catalog. Replace exported mutable slices with copied
accessors:

```go
const PlatformAdmin = "admin"

func RegisteredPlatforms() []string
func IsRegisteredPlatform(string) bool
func NotificationAudiencePlatforms() []string
func IsNotificationAudiencePlatform(string) bool
```

`RegisteredPlatforms()` returns a defensive copy containing only `admin`.
`NotificationAudiencePlatforms()` returns `all` followed by registered adapter
codes. `all` is a notification audience only. Keep `PlatformApp` and
`PlatformCanvas` temporarily only for Task 5 AI compilation, but neither is
registered or accepted by runtime validators; Task 5 removes both constants.
Replace every production caller of the ambiguous `IsPlatform` and exported
`Platforms` slice with `IsRegisteredPlatform`/`RegisteredPlatforms` in this
task. AI-run tests use Admin while retaining the platform field; they do not
use retired Canvas merely to make compilation pass.

Add tests with the non-production code `partner_portal` proving the two-layer
boundary:

```go
id, appErr := service.Create(ctx, validCreateInput("partner_portal"))
if appErr != nil || id == 0 { t.Fatalf("configuration must be creatable: %v", appErr) }
if enum.IsRegisteredPlatform("partner_portal") { t.Fatal("configuration must not activate an adapter") }
```

Route tests require all seven auth-platform operations and their exact
permissions/audits. Architecture tests fail if the auth-platform CRUD/status
methods, permission/role platform fields, session/login-log filters, or
notification audience fields disappear. They also fail if App/Canvas is
registered or if an Admin handler trusts a spoofed platform header.

- [ ] **Step 2: Restore the configuration catalog and keep runtime fail-closed**

Preserve these exact management operations:

```text
GET    /api/admin/v1/auth-platforms/page-init
GET    /api/admin/v1/auth-platforms
POST   /api/admin/v1/auth-platforms
PUT    /api/admin/v1/auth-platforms/:id
PATCH  /api/admin/v1/auth-platforms/:id/status
DELETE /api/admin/v1/auth-platforms/:id
DELETE /api/admin/v1/auth-platforms
```

Restore `CreateInput`, repository create/code-exists/batch lookup/delete
methods, service create/delete/status methods, request DTOs, handlers, error
keys, and route policies. Keep the existing rules that `admin` cannot be
deleted or disabled. Creating a syntactically valid configuration does not add
it to `RegisteredPlatforms()` and therefore cannot add a route.

Admin authentication and authorization transports inject
`enum.PlatformAdmin` internally. A `platform` header, query, or body field on
an Admin credential endpoint cannot change that value. No generic fallback
turns an unknown platform into Admin.

- [ ] **Step 3: Preserve platform isolation across core modules**

Permission list/create/update retains its explicit management `platform`
field and validates it against `RegisteredPlatforms()`. Role initialization
retains `permission_platform_arr` and one permission tree per registered
adapter. Cache invalidation and principal versions use the selected platform.

Session and login-log management retain platform filters, platform labels, and
distribution statistics. Repositories apply an explicit requested platform
when supplied and otherwise query all registered platform provenance; they are
not hard-coded to `WHERE platform = 'admin'`. Service validation tests prove an
unregistered `partner_portal` filter fails closed. Repository/cache isolation
tests use `admin` and `partner_portal` with different counts and IDs directly,
without adding the latter to the production registry, to expose accidental
mixing before the future adapter exists.

Notification task create/list/count/cancel/delete/claim retains its audience
field. It accepts `all` and registered platform codes, rejects App/Canvas and
unknown codes, and persists the exact documented audience. Realtime delivery
must not infer an unregistered target. Existing historical `all` row
disposition remains Task 6 data work, not a reason to delete the runtime
audience contract.

User/export/AI provenance fields remain explicit. Current Admin transports
write Admin; future adapters can supply only their compiled registered code.
Capability modules contain no App/Canvas switch.

- [ ] **Step 4: Verify and commit**

```powershell
go test -p 1 ./internal/shared/dict ./internal/shared/validate ./internal/module/auth_platform ./internal/module/auth_platform/transport/admin ./internal/module/permission ./internal/module/permission/transport/admin ./internal/module/role ./internal/module/role/transport/admin ./internal/module/notification/task ./internal/module/notification/transport/admin ./internal/module/auth ./internal/module/auth/transport/admin ./internal/module/user ./internal/server -count=1
go test -p 1 ./internal/platform/admin ./internal/runtime ./internal/architecture -run 'TestAdminOnly|TestPlatformKernel' -count=1
go test -p 1 ./internal/module/... -count=1
git add -- internal/shared/enum/platform.go internal/shared/dict/dict.go internal/shared/dict/options_test.go internal/shared/validate/platform.go internal/shared/validate/validate_test.go internal/module/auth_platform/dto.go internal/module/auth_platform/errors.go internal/module/auth_platform/repository.go internal/module/auth_platform/service.go internal/module/auth_platform/service_test.go internal/module/auth_platform/management_service_test.go internal/module/auth_platform/management_i18n_test.go internal/module/auth_platform/transport/admin/request.go internal/module/auth_platform/transport/admin/handler.go internal/module/auth_platform/transport/admin/handler_test.go internal/module/auth_platform/transport/admin/handler_i18n_test.go internal/module/auth_platform/transport/admin/route.go internal/module/permission/dto.go internal/module/permission/service.go internal/module/permission/service_test.go internal/module/permission/management_service_test.go internal/module/permission/transport/admin/request.go internal/module/permission/transport/admin/handler.go internal/module/permission/transport/admin/handler_test.go internal/module/role/dto.go internal/module/role/service.go internal/module/role/service_test.go internal/module/role/transport/admin/handler_test.go internal/module/notification/task/dto.go internal/module/notification/task/repository.go internal/module/notification/task/service.go internal/module/notification/task/service_test.go internal/module/notification/task/i18n_test.go internal/module/notification/task/jobs_test.go internal/module/notification/transport/admin/task_request.go internal/module/notification/transport/admin/task_handler.go internal/module/auth/loginlog.go internal/module/auth/loginlog_test.go internal/module/auth/session_admin.go internal/module/auth/session_test.go internal/module/auth/transport/admin/request.go internal/module/auth/transport/admin/handler.go internal/module/auth/transport/admin/handler_test.go internal/module/user/service.go internal/module/user/service_test.go internal/module/ai/run/recorder.go internal/module/ai/run/recorder_test.go internal/module/ai/run/service_test.go internal/module/export/notifier.go internal/middleware/auth_token.go internal/middleware/auth_token_test.go internal/server/router_test.go internal/server/routes_admin_commerce_rbac.go internal/server/routes_admin_comms.go internal/server/routes_admin_user.go internal/architecture/platform_kernel_test.go internal/server/testdata/admin_route_policy_golden.json internal/server/testdata/admin_routes_golden.txt scripts/full-admin-smoke.ps1
git diff --cached --check
git commit -m "refactor(platform): preserve the extensible platform kernel"
```

Expected: only Admin is registered, all auth-platform management operations
remain, App/Canvas is rejected, an unregistered configuration is not routable,
and platform-aware modules pass isolation tests.

### Task 5: Retain generic AI capabilities without Canvas language

**Files:**
- Create: `internal/module/ai/capability/scenes.go`
- Create: `internal/module/ai/capability/scenes_test.go`
- Modify: `internal/shared/enum/platform.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/service_test.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/module/ai/image/dto.go`
- Modify: `internal/module/ai/image/service.go`
- Modify: `internal/module/ai/image/service_test.go`
- Modify: `internal/module/ai/image/model_split_test.go`
- Modify: `internal/module/ai/audio/dto.go`
- Modify: `internal/module/ai/audio/service.go`
- Modify: `internal/module/ai/audio/service_test.go`
- Modify: `internal/module/ai/video/dto.go`
- Modify: `internal/module/ai/video/model.go`
- Modify: `internal/module/ai/video/service.go`
- Modify: `internal/module/ai/video/service_test.go`
- Modify: `internal/module/ai/run/recorder_test.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/module/ai/run/transport/admin/request.go`
- Create: `internal/shared/i18n/locales/en-US/aitext.yaml`
- Create: `internal/shared/i18n/locales/en-US/aiaudio.yaml`
- Create: `internal/shared/i18n/locales/en-US/aivideo.yaml`
- Create: `internal/shared/i18n/locales/zh-CN/aitext.yaml`
- Create: `internal/shared/i18n/locales/zh-CN/aiaudio.yaml`
- Create: `internal/shared/i18n/locales/zh-CN/aivideo.yaml`
- Modify: `internal/shared/i18n/locales/en-US/aiimage.yaml`
- Modify: `internal/shared/i18n/locales/zh-CN/aiimage.yaml`
- Modify: `internal/shared/i18n/locales/en-US/user.yaml`
- Modify: `internal/shared/i18n/locales/zh-CN/user.yaml`
- Delete: `internal/shared/i18n/locales/en-US/canvas.yaml`
- Delete: `internal/shared/i18n/locales/zh-CN/canvas.yaml`
- Create: `internal/architecture/ai_capability_boundary_test.go`
- Modify: `scripts/full-admin-smoke.ps1`

- [ ] **Step 1: Centralize canonical scenes and prove classification**

```go
package capability

const (
	SceneTextGenerate  = "text_generate"
	SceneImageGenerate = "image_generate"
	SceneVideoGenerate = "video_generate"
	SceneAudioGenerate = "audio_generate"
)
```

`ai_capability_boundary_test.go` requires direct behavior tests for provider
dispatch, run recording, storage keys, cancellation/failure, provenance, and
repository state in every retained modality. It rejects Gin imports, retired
transport imports, `canvas_` identifiers, `canvas.` i18n keys,
`canvas_video_tasks`, and App/Canvas platform switches inside retained
capability packages. It must not reject the generic `platform` provenance
field. After the capability references are replaced, delete `PlatformApp` and
`PlatformCanvas` from `internal/shared/enum/platform.go` and prove all
production packages compile.

- [ ] **Step 2: Rename interfaces and records, not only labels**

Rename `CanvasCompletionInput/Response` and `CanvasCompletion` to
`TextCompletionInput/Response` and `CompleteText`. Rename helper/request IDs to
`text-completion-*`, `ai_audio_*`, and `ai_video_task_*`.
`VideoTask.TableName()` becomes `ai_video_tasks`. Image/text/audio/video run
records retain a required registered-platform provenance. The Admin transport
injects Admin internally; an Admin create request cannot choose another
platform. AI-run management filters retain the generic platform field and use
the registered adapter options, which currently contain only Admin.

Replace every capability error key with `aitext.*`, `aiimage.*`, `aiaudio.*`, or `aivideo.*`; remove Canvas wording from safe messages. Do not rename DOM Canvas APIs or the untouched repository.

- [ ] **Step 3: Prove retained capability behavior directly**

Each modality test invokes its service with fake engine, secret, run recorder, repository/storage, and clock. Tests cover success, invalid scene, provider failure, empty response, run completion/failure, object-key generation, and idempotent terminal update without any HTTP transport.

- [ ] **Step 4: Run the terminology guard and commit**

```powershell
go test ./internal/module/ai/capability ./internal/module/ai/agent ./internal/module/ai/chat ./internal/module/ai/image ./internal/module/ai/audio ./internal/module/ai/video ./internal/module/ai/run ./internal/shared/i18n ./internal/architecture -count=1
rg -n -i "canvas_|canvas\.|PlatformCanvas|CanvasCompletion|canvas_video_tasks" internal/module/ai internal/shared scripts/full-admin-smoke.ps1 --glob "!**/legacy/**"
git add -- internal/module/ai/capability/scenes.go internal/module/ai/capability/scenes_test.go internal/shared/enum/platform.go internal/module/ai/agent/service.go internal/module/ai/agent/service_test.go internal/module/ai/chat/dto.go internal/module/ai/chat/service.go internal/module/ai/chat/service_test.go internal/module/ai/image/dto.go internal/module/ai/image/service.go internal/module/ai/image/service_test.go internal/module/ai/image/model_split_test.go internal/module/ai/audio/dto.go internal/module/ai/audio/service.go internal/module/ai/audio/service_test.go internal/module/ai/video/dto.go internal/module/ai/video/model.go internal/module/ai/video/service.go internal/module/ai/video/service_test.go internal/module/ai/run/recorder_test.go internal/module/ai/run/service_test.go internal/module/ai/run/transport/admin/request.go internal/shared/i18n/locales/en-US/aitext.yaml internal/shared/i18n/locales/en-US/aiaudio.yaml internal/shared/i18n/locales/en-US/aivideo.yaml internal/shared/i18n/locales/zh-CN/aitext.yaml internal/shared/i18n/locales/zh-CN/aiaudio.yaml internal/shared/i18n/locales/zh-CN/aivideo.yaml internal/shared/i18n/locales/en-US/aiimage.yaml internal/shared/i18n/locales/zh-CN/aiimage.yaml internal/shared/i18n/locales/en-US/user.yaml internal/shared/i18n/locales/zh-CN/user.yaml internal/architecture/ai_capability_boundary_test.go scripts/full-admin-smoke.ps1
git add -u -- internal/shared/i18n/locales/en-US/canvas.yaml internal/shared/i18n/locales/zh-CN/canvas.yaml
git diff --cached --check
git commit -m "refactor(ai): retain transport-neutral generation capabilities"
```

Expected: `rg` has no matches and exits 1; all retained capability tests pass without a retired transport.

### Task 6: Execute guarded legacy-product Atlas contract groups

**Files:**
- Create: `database/migrations/202607150201_admin_only_rows.sql`
- Create: `database/migrations/202607150202_admin_only_schema.sql`
- Create: `database/migrations/202607150203_admin_only_constraints.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/schema/admin.hcl`
- Create: `database/reconciliation/051_verify_admin_rows.sql`
- Create: `database/reconciliation/052_verify_ai_contract.sql`
- Create: `database/reconciliation/053_verify_admin_only.sql`
- Create: `internal/databaseevolution/migration_lock.go`
- Create: `internal/databaseevolution/migration_lock_test.go`
- Modify: `cmd/admin-db/main.go`
- Create: `scripts/database/contract-admin-only.ps1`
- Create: `scripts/tests/admin-only-contract.tests.ps1`
- Modify: `database/README.md`

- [ ] **Step 1: Add a lock-held child execution primitive**

`admin-db lock-run` opens one MySQL connection, acquires `GET_LOCK('admin:atlas:migrate', 30)`, verifies the schema and input fingerprint, executes one child command while the same connection holds the lock, and releases it in `defer`. It forwards the child exit code but never prints `MYSQL_DSN`.

Tests prove lock contention fails closed, child failure releases the lock, a DSN for another schema is rejected, and arguments are passed without shell interpolation.

- [ ] **Step 2: Write three independently verifiable migration groups**

`202607150201_admin_only_rows.sql` performs only the dispositions locked in
Task 1, in dependency order. These statements remove retired product rows;
they do not redefine the platform kernel as a singleton:

```sql
DELETE rp FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
WHERE p.platform IN ('app', 'canvas');
DELETE rp FROM role_permissions rp LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE p.id IS NULL AND rp.id = 723 AND rp.role_id = 1 AND rp.permission_id = 539;
DELETE FROM permissions WHERE platform IN ('app', 'canvas');
DELETE FROM user_sessions WHERE platform IN ('app', 'canvas');
DELETE FROM users_login_log WHERE platform IN ('app', 'canvas');
DELETE FROM auth_platforms WHERE code IN ('app', 'canvas');
UPDATE notification_task SET platform = 'admin' WHERE platform = 'all';
UPDATE notifications SET platform = 'admin' WHERE platform = 'all';
DELETE FROM notification_task WHERE platform IN ('app', 'canvas');
DELETE FROM notifications WHERE platform IN ('app', 'canvas');
DELETE FROM export_tasks WHERE platform IN ('app', 'canvas');
```

The `all` updates above are the locked disposition of historical rows only.
The runtime notification contract and final schema continue to permit the
formally defined `all` virtual audience for new tasks.

The same file deletes Canvas AI dependents before `ai_runs`, `ai_text_tasks`,
`ai_image_tasks`, and video-task owners; it updates all four `ai_agents` scene
values using `JSON_REPLACE`. It does not reference the already-absent billing
tables. The migration contains no guessed reassignment and aborts if an
expected evidence count differs.

Use a temporary ID set so dependent deletion is explicit and repeatable:

```sql
CREATE TEMPORARY TABLE contract_retired_ai_runs (`id` BIGINT PRIMARY KEY)
SELECT `id` FROM `ai_runs` WHERE `platform` IN ('app', 'canvas');
DELETE h FROM ai_knowledge_retrieval_hits h
JOIN ai_knowledge_retrievals r ON r.id = h.retrieval_id
JOIN contract_retired_ai_runs x ON x.id = r.run_id;
DELETE r FROM ai_knowledge_retrievals r JOIN contract_retired_ai_runs x ON x.id = r.run_id;
DELETE c FROM ai_tool_calls c JOIN contract_retired_ai_runs x ON x.id = c.run_id;
DELETE e FROM ai_run_events e JOIN contract_retired_ai_runs x ON x.id = e.run_id;
DELETE r FROM ai_runs r JOIN contract_retired_ai_runs x ON x.id = r.id;
DELETE f FROM ai_image_files f JOIN ai_image_tasks t ON t.id = f.task_id
WHERE t.platform IN ('app', 'canvas');
DELETE FROM ai_image_tasks WHERE platform IN ('app', 'canvas');
DELETE FROM ai_text_tasks WHERE platform IN ('app', 'canvas');
DELETE FROM canvas_video_tasks;
UPDATE ai_agents
SET scenes_json = REPLACE(REPLACE(REPLACE(REPLACE(
  scenes_json,
  '"canvas_text_generate"', '"text_generate"'),
  '"canvas_image_generate"', '"image_generate"'),
  '"canvas_video_generate"', '"video_generate"'),
  '"canvas_audio_generate"', '"audio_generate"')
WHERE JSON_VALID(scenes_json);
```

`051_verify_admin_rows.sql` proves the temporary-set source counts match locked evidence, no dependent survives, every remaining scene belongs to the canonical set, and no duplicate scene was introduced.

`202607150202_admin_only_schema.sql` first proves
`users_quick_entry`, `canvas_prompts`, `canvas_assets`, `ai_billing_rules`, and
`ai_billing_records` are already absent. It drops only the empty
`canvas_video_tasks` compatibility table and the P08R-frozen
`client_versions` table; drops the unused `user_sessions.access_token_hash`
column proven unreferenced by P04; verifies the canonical `ai_video_tasks`
table from P02 remains; and removes only other compatibility columns/indexes
accepted by P02 query evidence. Before dropping `client_versions`, it rechecks
the locked count/hash, proves no foreign key/view/event references it, and
requires the operator's fresh destructive approval token through the wrapper.
It does not delete COS objects and does not drop a performance index without
matching before/after evidence.

`202607150203_admin_only_constraints.sql` proves `client_versions` and all
client-version constraints are absent, then records the target fingerprint.
It must not add `platform = 'admin'` or `auth_platforms.code = 'admin'` checks.
Instead it adds named syntax/retirement checks equivalent to:

```sql
CHECK (
  platform REGEXP '^[a-z][a-z0-9_]{1,48}$'
  AND platform NOT IN ('app', 'canvas')
)
```

For notification tables, `all` remains an allowed virtual audience. For every
other active provenance row, `052`/`053` use zero-row joins to prove the code
exists in a non-deleted `auth_platforms` row. The checks and invariants cover
`permissions`, `authz_principal_versions`, `user_sessions`,
`users_login_log`, `notification_task`, `notifications`, `export_tasks`,
`ai_runs`, `ai_text_tasks`, and `ai_image_tasks`; they do not reference the
already-absent `ai_billing_records` table.

`053_verify_admin_only.sql` additionally proves that `auth_platforms`, its
unique code index, its policy columns, every retained provenance column, and
the platform query indexes still exist. It proves current registered/active
product rows are Admin-only without forbidding a future valid code in schema.

- [ ] **Step 3: Make the wrapper stop between groups**

`contract-admin-only.ps1` requires `-ExpectedSourceFingerprint`, `-InputLock`, and explicit `-Apply`. It rejects the live `admin` schema unless a validated release manifest is also supplied. For a restore/staging schema it runs:

```text
050 preconditions
Atlas apply through 202607150201
051 row invariants
Atlas apply through 202607150202
052 AI/schema invariants and COS reachability
Atlas apply through 202607150203
053 final invariants, drift check, and fingerprint
```

Every Atlas call runs beneath `admin-db lock-run`. A failed invariant prevents the next version.

- [ ] **Step 4: Rehearse on two disposable restores**

```powershell
pwsh -NoProfile -File scripts/tests/admin-only-contract.tests.ps1
pwsh -NoProfile -File scripts/database/contract-admin-only.ps1 -Database $env:ADMIN_RESTORE_DB -ExpectedSourceFingerprint $env:ADMIN_VERIFIED_FINGERPRINT -InputLock release/admin-only/input-lock.json -Apply
pwsh -NoProfile -File scripts/database/contract-admin-only.ps1 -Database $env:ADMIN_SECOND_RESTORE_DB -ExpectedSourceFingerprint $env:ADMIN_VERIFIED_FINGERPRINT -InputLock release/admin-only/input-lock.json -Apply
pwsh -NoProfile -File scripts/database/check-drift.ps1 -Database $env:ADMIN_RESTORE_DB
```

Expected: both restores reach the same fingerprint, `053` returns zero rows,
repeat execution is clean, no live schema is touched, App/Canvas is absent,
and the platform-kernel schema remains intact.

- [ ] **Step 5: Commit the reviewed migration and canonical schema**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
git add -- database/migrations/202607150201_admin_only_rows.sql database/migrations/202607150202_admin_only_schema.sql database/migrations/202607150203_admin_only_constraints.sql database/migrations/atlas.sum database/schema/admin.hcl database/reconciliation/051_verify_admin_rows.sql database/reconciliation/052_verify_ai_contract.sql database/reconciliation/053_verify_admin_only.sql internal/databaseevolution/migration_lock.go internal/databaseevolution/migration_lock_test.go cmd/admin-db/main.go scripts/database/contract-admin-only.ps1 scripts/tests/admin-only-contract.tests.ps1 database/README.md
git diff --cached --check
git commit -m "feat(database): retire legacy product schema safely"
```

### Task 7: Publish and consume the current-Admin platform-kernel Contract Bundle

**Backend files:**
- Modify: `internal/admincontract/bundle.go`
- Modify: `internal/admincontract/openapi.go`
- Modify: `internal/admincontract/permissions.go`
- Modify: `internal/admincontract/views.go`
- Modify: `internal/admincontract/realtime.go`
- Modify: `internal/admincontract/manifest.go`
- Modify: `internal/admincontract/bundle_test.go`
- Modify: `contracts/admin/v1/openapi.json`
- Modify: `contracts/admin/v1/permissions.json`
- Modify: `contracts/admin/v1/views.json`
- Modify: `contracts/admin/v1/realtime/envelope.schema.json`
- Modify: `contracts/admin/v1/realtime/events.schema.json`
- Modify: `contracts/admin/v1/manifest.json`
- Modify: `internal/architecture/admin_only_test.go`

**Frontend files:**
- Replace: `contracts/backend/admin/v1/`
- Modify: `contracts/backend/admin/lock.json`
- Modify: `src/modules/http/generated/admin.ts`
- Modify: `src/modules/http/generated/operations.ts`
- Modify: `src/modules/routing/generated/permissions.ts`
- Modify: `src/modules/routing/generated/views.ts`
- Modify: `tests/shared/architecture/admin-only.test.ts`

- [ ] **Step 1: Add final generated-contract guards**

Backend and frontend guards reject App/Canvas paths, permission codes, view
keys, retired product enum values, old scenes, client-version
operations/views/permissions, client-variant headers, desktop refresh fields,
and every Tauri/native generated identifier. They require every operation to
have one route policy/audit decision and every artifact hash to match the
manifest.

The guards also require all seven auth-platform management operations,
permission/role platform fields, session/login-log platform filters and
statistics, notification audience fields including `all`, and generic
provenance fields. Generated platform option enums may contain only the current
registered Admin adapter (plus notification `all`); configuration code fields
remain validated strings so a future adapter can be staged without a schema
rewrite.

- [ ] **Step 2: Regenerate backend truth from the committed runtime**

```powershell
cd E:/admin/admin_back_go
pwsh -NoProfile -File scripts/generate-admin-contract.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
go test ./internal/admincontract ./internal/architecture -run 'TestBundle|TestManifest|TestAdminOnly' -count=1
git add -- internal/admincontract/bundle.go internal/admincontract/openapi.go internal/admincontract/permissions.go internal/admincontract/views.go internal/admincontract/realtime.go internal/admincontract/manifest.go internal/admincontract/bundle_test.go contracts/admin/v1/openapi.json contracts/admin/v1/permissions.json contracts/admin/v1/views.json contracts/admin/v1/realtime/envelope.schema.json contracts/admin/v1/realtime/events.schema.json contracts/admin/v1/manifest.json internal/architecture/admin_only_test.go
git diff --cached --check
git commit -m "feat(contract): publish current-admin platform-kernel bundle"
```

The generator records the current clean backend source SHA containing Tasks 1–6 and bumps the bundle version to `admin-2026-07-15.2`. The following generated-contract commit is deliberately newer, avoiding a self-referential commit hash.

- [ ] **Step 3: Lock frontend consumption to that exact bundle**

```powershell
cd E:/admin/admin_front_ts
$backendManifest = Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json
$backendSourceCommit = $backendManifest.backend_commit
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --mount "type=bind,src=E:/admin/admin_back_go,dst=/backend,readonly" --workdir /workspace node:24.18.0-alpine sh -lc "npm ci && npm run contract:sync -- --backend /backend --commit $backendSourceCommit && npm run contract:generate && npm run routes:generate && npm run contract:check && npm test -- tests/shared/architecture/admin-only.test.ts && npm run check:browser-only && npm run typecheck && npm run lint"
git add -- contracts/backend/admin/v1 contracts/backend/admin/lock.json src/modules/http/generated/admin.ts src/modules/http/generated/operations.ts src/modules/routing/generated/permissions.ts src/modules/routing/generated/views.ts tests/shared/architecture/admin-only.test.ts
git diff --cached --check
git commit -m "chore(contract): lock current-admin platform-kernel bundle"
```

Expected: frontend lock commit/digest equals backend manifest, all generated
files are clean, no retired App/Canvas operation is representable, and the
platform administration core remains fully representable.

### Task 8: Build a synchronized immutable deployment and rollback path

**Backend files:**
- Create: `release/admin-only/release-manifest.schema.json`
- Create: `scripts/release/new-release-manifest.ps1`
- Create: `scripts/release/check-release-manifest.ps1`
- Create: `scripts/release/check-platform-kernel.ps1`
- Create: `scripts/release/export-docker-images.ps1`
- Create: `scripts/release/deploy-admin-only.ps1`
- Create: `scripts/release/rollback-admin-only.ps1`
- Create: `deploy/admin-only/docker-compose.yml`
- Create: `internal/architecture/admin_release_test.go`

**Frontend files:**
- Create: `tests/shared/deployment/admin-release.test.ts`

- [ ] **Step 1: Write failing Docker-release boundary tests**

Backend tests require a strict release schema, revision-labelled
frontend/backend image digests, image-archive SHA-256 values, the exact
Browser-only Admin Contract Bundle digest, database/recovery evidence, P08R
retirement evidence, the approved COS historical-object disposition digest,
and a platform-kernel proof digest. The platform-kernel proof must demonstrate
both current Admin-only adapter registration and retained generic management/
schema seams. Frontend tests reject every `.github` directory, deployment
Workflow, desktop artifact, and versioned-web shell switch.

`check-platform-kernel.ps1` runs the committed backend
`TestPlatformKernel` guard, checks the final backend Bundle for all seven
auth-platform operations and platform-aware schemas, checks the frontend lock
against that Bundle, and executes the post-contract `053` platform-kernel
invariants against the supplied disposable database. It writes only commits,
artifact digests, invariant counts, and pass/fail state to ignored
`release/admin-only/out/platform-kernel-proof.json` through atomic rename.

```powershell
cd E:/admin/admin_back_go
go test ./internal/architecture -run TestAdminRelease -count=1

cd E:/admin/admin_front_ts
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:24.18.0-alpine sh -lc "npm ci && npm test -- tests/shared/deployment/admin-release.test.ts"
```

Expected: FAIL because release schema/scripts/tests do not exist.

- [ ] **Step 2: Define one non-circular release manifest**

The generated manifest is never committed. Its tracked schema contains concrete validation rules rather than a value template:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "release_id", "backend", "frontend", "contract", "database", "evidence"],
  "$defs": {
    "gitSha": {"type": "string", "pattern": "^[0-9a-f]{40}$"},
    "sha256": {"type": "string", "pattern": "^[0-9a-f]{64}$"}
  },
  "properties": {
    "schema_version": {"const": 1},
    "release_id": {"type": "string", "pattern": "^admin-v[0-9]{4}\\.[0-9]{2}\\.[0-9]{2}\\.[1-9][0-9]*$"},
    "backend": {
      "type": "object", "additionalProperties": false, "required": ["commit", "image", "archive_sha256"],
      "properties": {"commit": {"$ref": "#/$defs/gitSha"}, "image": {"type": "string", "pattern": "^[^@\\s]+@sha256:[0-9a-f]{64}$"}, "archive_sha256": {"$ref": "#/$defs/sha256"}}
    },
    "frontend": {
      "type": "object", "additionalProperties": false, "required": ["commit", "image", "archive_sha256"],
      "properties": {"commit": {"$ref": "#/$defs/gitSha"}, "image": {"type": "string", "pattern": "^[^@\\s]+@sha256:[0-9a-f]{64}$"}, "archive_sha256": {"$ref": "#/$defs/sha256"}}
    },
    "contract": {
      "type": "object", "additionalProperties": false, "required": ["bundle_version", "manifest_sha256"],
      "properties": {"bundle_version": {"type": "string", "pattern": "^admin-[0-9]{4}-[0-9]{2}-[0-9]{2}\\.[1-9][0-9]*$"}, "manifest_sha256": {"$ref": "#/$defs/sha256"}}
    },
    "database": {
      "type": "object", "additionalProperties": false, "required": ["atlas_version", "target_fingerprint_sha256", "atlas_sum_sha256"],
      "properties": {"atlas_version": {"const": "202607150203"}, "target_fingerprint_sha256": {"$ref": "#/$defs/sha256"}, "atlas_sum_sha256": {"$ref": "#/$defs/sha256"}}
    },
    "evidence": {
      "type": "object", "additionalProperties": false, "required": ["input_lock_sha256", "query_sha256", "cos_disposition_sha256", "recovery_sha256", "browser_only_retirement_sha256", "platform_kernel_sha256"],
      "properties": {"input_lock_sha256": {"$ref": "#/$defs/sha256"}, "query_sha256": {"$ref": "#/$defs/sha256"}, "cos_disposition_sha256": {"$ref": "#/$defs/sha256"}, "recovery_sha256": {"$ref": "#/$defs/sha256"}, "browser_only_retirement_sha256": {"$ref": "#/$defs/sha256"}, "platform_kernel_sha256": {"$ref": "#/$defs/sha256"}}
    }
  }
}
```

`new-release-manifest.ps1` reads artifact metadata, the committed
platform-kernel verification output, and the pre-contract input
lock, verifies both current clean P09 release commits, proves that the two
pre-contract commits are ancestors of them, and writes
`release/admin-only/out/release-manifest.json` atomically. The manifest records
the current release commits, never the older pre-contract commits.

- [ ] **Step 3: Export verified Docker images without a deployment Workflow**

`export-docker-images.ps1` first verifies both primary checkout directories are
clean at their current P09 release commits and proves the input-lock commits are
ancestors. It must not check out or build either pre-contract commit. It
delegates builds only to the repository Docker tooling, verifies each image
revision label equals its current owning commit, records immutable image
digests, exports the two images to ignored `release/admin-only/out/images/`,
computes archive SHA-256 values, and reruns `docker image inspect` after a clean
load test. It never builds or deploys through GitHub Actions.

There is no desktop release unit. Task 8 reads only the reviewed COS historical-object disposition evidence; it cannot upload, promote, overwrite, or delete those objects.

- [ ] **Step 4: Implement Compose deployment and rollback**

`deploy-admin-only.ps1` validates the manifest, image archives/digests/revision labels, current database fingerprint, recovery proof, and explicit maintenance inputs. It loads the verified images, runs Task 6 migration groups under the database lock, starts a staging Compose project by immutable digest, waits for health/readiness, runs Admin HTTP/realtime smoke, then promotes the Compose project. It records the previous manifest/project and never deletes the previous image archives or state volumes.

`rollback-admin-only.ps1` verifies the previous manifest/archive digests, loads the previous frontend/backend images, restores the previous Compose project, and reruns health/readiness/Admin smoke. If the operator selects full database rollback, it requires the locked recovery artifact, a maintenance-window flag, and successful restore rehearsal evidence; it never invents reverse DDL for deleted rows or reconstructs `client_versions` from guessed metadata.

- [ ] **Step 5: Test and commit backend release machinery**

```powershell
cd E:/admin/admin_back_go
go test ./internal/architecture -run TestAdminRelease -count=1
pwsh -NoProfile -File scripts/release/check-release-manifest.ps1 -SchemaOnly
git add -- release/admin-only/release-manifest.schema.json scripts/release/new-release-manifest.ps1 scripts/release/check-release-manifest.ps1 scripts/release/check-platform-kernel.ps1 scripts/release/export-docker-images.ps1 scripts/release/deploy-admin-only.ps1 scripts/release/rollback-admin-only.ps1 deploy/admin-only/docker-compose.yml internal/architecture/admin_release_test.go
git diff --cached --check
git commit -m "build(release): deploy immutable Docker artifacts"
```

- [ ] **Step 6: Test and commit the frontend deployment boundary**

```powershell
cd E:/admin/admin_front_ts
$root = (Get-Location).Path
docker run --rm --mount "type=bind,src=$root,dst=/workspace" --workdir /workspace node:24.18.0-alpine sh -lc "npm ci && npm test -- tests/shared/deployment/admin-release.test.ts"
git add -- tests/shared/deployment/admin-release.test.ts
git diff --cached --check
git commit -m "test(release): enforce Docker-only frontend delivery"
```

### Task 9: Rehearse rollback and produce the cross-repository release proof

**Backend files:**
- Create: `scripts/release/verify-admin-only-release.ps1`
- Create: `scripts/tests/admin-only-release-rehearsal.tests.ps1`
- Create: `docs/runbooks/admin-only-deployment.md`
- Create: `docs/runbooks/admin-only-rollback.md`
- Create: `docs/runbooks/admin-only-secrets.md`
- Create: `docs/runbooks/admin-only-observability.md`
- Create: `docs/runbooks/admin-only-schema-status.md`
- Create: `docs/runbooks/platform-onboarding.md`
- Modify: `docs/architecture.md`
- Modify: `CONTEXT.md`

- [ ] **Step 1: Write operational runbooks with exact stop conditions**

Document required roles/secrets without values, artifact acquisition, digest
verification, database lock/status, preflight fingerprints, migration group
stop points, health/readiness, Admin smoke, WebSocket/queue/scheduler/provider
metrics, log redaction checks, application rollback, full recovery rollback,
RTO/RPO recording, and incident escalation. Every command reads credentials
from the environment or ignored files and redacts them from output.

`platform-onboarding.md` records the mandatory future order: approve the new
platform contract, implement a dedicated trusted transport, add its compile-
time registry entry, publish a matching Bundle, configure `auth_platforms`,
assign independent permission/role data, configure notification audiences,
run cross-platform isolation tests, and deploy by Docker. It explicitly states
that a database row or client header never activates a platform.

- [ ] **Step 2: Automate the complete proof**

`verify-admin-only-release.ps1` runs, in order:

```text
backend clean-cache/dependency/test/vet/static/vulnerability/build/container gates
database restore/reconciliation/contract/repeat/drift/invariants/query-plan gates
runtime/identity/durable-work/realtime multi-node and termination gates
Admin Contract Bundle generation/check and frontend lock check
frontend lint/typecheck/unit/component/integration/build/budget gates
P07 Docker health/revision/authenticated HTTP/realtime smoke and user acceptance evidence
P08R Browser-only source/contract/menu/session retirement and user acceptance evidence
secret/dump/sensitive-log scan
current-Admin adapter and Browser-only source/generated scan, including absent client_versions
platform-kernel contract/schema scan requiring auth-platform CRUD, RBAC/session/log/notification dimensions, provenance columns, and indexes
primary-checkout/no-worktree/no-.github/no-deployment-Workflow checks
```

It writes only hashes, counts, timings, Docker image identifiers, reconciliation/restore identifiers, and pass/fail status to `release/admin-only/out/proof.json`; it never copies live rows, prompts, credentials, dumps, certificates, or object content.

- [ ] **Step 3: Rehearse deploy and both rollback modes**

On an isolated environment restored from the locked recovery artifact:

1. deploy the release manifest through all three migration groups;
2. kill API and Worker during durable AI work and prove recovery/cancel;
3. run full Admin Docker HTTP/realtime smoke and present the manual UI checklist for user confirmation;
4. switch to the previous frontend/backend Docker images and prove application rollback;
5. enter maintenance mode, restore the recovery artifact, verify the original fingerprint, and prove full database rollback;
6. redeploy the staging release and prove repeatability.

Any mismatch invalidates the release manifest; do not patch evidence by hand.

- [ ] **Step 4: Commit runbooks and proof tooling**

```powershell
cd E:/admin/admin_back_go
pwsh -NoProfile -File scripts/tests/admin-only-release-rehearsal.tests.ps1
git add -- scripts/release/verify-admin-only-release.ps1 scripts/tests/admin-only-release-rehearsal.tests.ps1 docs/runbooks/admin-only-deployment.md docs/runbooks/admin-only-rollback.md docs/runbooks/admin-only-secrets.md docs/runbooks/admin-only-observability.md docs/runbooks/admin-only-schema-status.md docs/runbooks/platform-onboarding.md docs/architecture.md CONTEXT.md
git diff --cached --check
git commit -m "docs(release): add admin-only operations and rollback proof"
```

- [ ] **Step 5: Generate the final proof from clean locked commits**

```powershell
pwsh -NoProfile -File scripts/release/lock-inputs.ps1 -CheckOnly
pwsh -NoProfile -File scripts/release/check-platform-kernel.ps1 -Database $env:ADMIN_RESTORE_DB -Output release/admin-only/out/platform-kernel-proof.json
pwsh -NoProfile -File scripts/release/new-release-manifest.ps1
pwsh -NoProfile -File scripts/release/check-release-manifest.ps1 -Manifest release/admin-only/out/release-manifest.json
pwsh -NoProfile -File scripts/release/verify-admin-only-release.ps1 -Manifest release/admin-only/out/release-manifest.json
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
```

Expected: both status commands produce no output; all nine plan gates pass;
imported, empty, and post-contract databases share the committed fingerprint;
immutable Docker artifacts match the manifest; `client_versions` is absent
only after the approved contract group; App/Canvas adapters and data are absent;
the extensible platform kernel proof is present; rollback rehearsal passes; no
secondary worktree, `.github` directory, desktop artifact, or deployment
Workflow exists. Only after a fresh explicit user approval may the operator run
the live contract migration and Compose promotion commands.
