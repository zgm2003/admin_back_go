# Runtime 装配、Admin 契约与前端收尾 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把阶段 A 的 Gateway、钱包、pricing、Runner 和媒体服务装配进 API/Worker，发布只注册的新权限，同步 Admin Contract Bundle，并让 ToC 聊天 UI 正确显示 stopping/settled 状态和人民币金额。

**Architecture:** runtime 只负责依赖注入和 durable worker 注册；HTTP handler 不直接访问钱包或 provider。权限定义随契约发布但不自动写 `role_permissions`，管理员手工授权后再开放管理页。前端以生成契约为准，不手写后端 JSON。

**Tech Stack:** Go runtime wiring、Admin OpenAPI/permission bundle、Vue/TypeScript、现有 contract sync/check scripts。

**Scope note:** Plan 06 已完成 chat/text/tool/image 的付费 Worker、队列注册和恢复装配，Task 1 只复核这些已合并事实并补缺，不重复建第二套 Gateway。音频、视频不进入本计划，也不得在本计划中新增、改造或删除；它们由后续独立 Plan 08 完整退役。

---

### Task 1: Wire dependencies in API and Worker

**Files:**
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`
- Modify: `internal/jobs/noop.go`
- Modify: `internal/jobs/noop_test.go`

- [x] **Step 1: Construct shared repositories once**

Audit the Plan 06 composition and keep one process-owned wallet repository plus the existing Run/attempt/finalizer dependencies built from `Resources.DB`; pass those dependencies only through the existing chat/text/tool/image paths. Do not add a second DB, RPC client, secret or parallel Gateway implementation.

- [x] **Step 2: Register durable handlers**

Confirm chat drain/finalizer plus text and image task handlers remain registered in the existing task queue registry. HTTP request context must never be passed as provider execution context; the queue lease owns execution, and a lost dispatched lease closes locally as `outcome_unknown + released`.

- [x] **Step 3: Verify composition**

Run `go test ./internal/platform/admin ./internal/runtime -run 'Test.*Build|Test.*Worker|Test.*Registry'` and `git diff --check`.

### Task 2: Publish permissions without auto-authorizing roles

**Files:**
- Modify: `internal/module/ai/run/transport/admin/route.go`
- Modify: `internal/admincontract/views.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Test: `internal/server/router_test.go`
- Test: `internal/admincontract/permissions_test.go`

- [ ] **Step 1: Register stable codes**

Use the exact new code `ai_run_list` created by Plan 01. Apply it explicitly to the Phase A management reads `GET /api/admin/v1/ai-runs/page-init`, list, detail and stats routes plus the `/ai/runs` view; never enforce permission by a broad path-prefix rule. The Phase B current-user `PUT /api/admin/v1/ai-runs/:id/user-feedback` is an explicit future `Authenticated()` + ownership exception and must not inherit `ai_run_list`. Agent billing changes continue using existing `ai_agent_edit`; wallet administration continues using existing `payment_ledger_list`/`payment_wallet_list`. Do not create redundant pricing or wallet permissions and do not insert/update `role_permissions`.

- [ ] **Step 2: Protect management handlers**

Ensure agent multiplier/max-output mutations remain under `ai_agent_edit`, while Run/usage details require `ai_run_list`. Consumer wallet and chat endpoints remain authenticated user routes, not Admin permission checks.

- [ ] **Step 3: Test manual authorization boundary**

Assert a role without `ai_run_list` receives forbidden and a principal with it can read Run details. The migration only defines permission ID `920`; document that the user manually grants it before opening the Run page.

### Task 3: Regenerate and verify Admin Contract Bundle

**Files:**
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_test.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `internal/admincontract/realtime.go`
- Modify: `internal/admincontract/realtime_test.go`
- Modify: `docs/contracts/admin-v1-workflow-contracts.md`
- Modify through generator: `contracts/admin/v1/*`
- Modify through sync/generator: `..\admin_front_ts\contracts\backend\admin\*`
- Modify through generator: `..\admin_front_ts\src\modules\http\generated\*`
- Modify through generator: `..\admin_front_ts\src\modules\routing\generated\*`
- Test: `..\admin_front_ts\tests\unit\contracts\admin-contract.test.ts`

- [ ] **Step 1: Publish Run billing detail from its backend source**

Extend `RunDetailRow`/repository/service DTOs and curated `AIRunDetail` with `billing_status`, `billing_reason`, `held_amount`, `actual_amount`, closed `pricing`, `usage_items` and `provider_attempts`. The backend validates the immutable Run `pricing_snapshot_json` and maps only that Run-level configuration into the closed pricing object, which exposes distinct `catalog_vendor` and `transport_engine`; it must not retain the obsolete ambiguous `provider_engine` field or derive pricing from per-attempt `quote_json`/`prepared_request_json`. Publish the exact reason enum `pending|held|settled_complete_usage|released_before_dispatch|released_insufficient_balance|released_provider_failed|released_outcome_unknown|unbilled_usage_incomplete|unbilled_over_hold|legacy_unpriced`. Convert Charge `held_units` (the Run's maximum successfully reserved audit amount), `actual_units`, rate prices and item amounts through Plan 01's `sharedmoney.FormatRMBUnits`. Do not expose raw units as JSON numbers, do not add a second formatter, and do not derive terminal `held_amount` from the now-zero active Hold. Fetch charge, item and attempt facts with bounded batch queries; do not make the frontend parse Run or attempt JSON to reconstruct pricing or billed usage. Keep provider API keys, raw credential material, exact prepared request bodies and raw quote evidence absent from consumer DTOs. Add repository/service/schema tests that prove nullable legacy `unbilled` Runs map to `legacy_unpriced`, failed attempt items are non-billable, attempt quotes cannot override the Run pricing configuration, a terminal Run retains its maximum `held_amount`, and new settled Run item amounts sum exactly to `actual_amount`.

For historical `legacy_unpriced` Runs without a Charge or valid paid snapshot, publish `pricing: null`, zero canonical amount strings and empty billing arrays; never feed the legacy marker into the paid snapshot parser or fabricate pricing evidence.

- [ ] **Step 2: Generate from compiled routes/schema**

Commit the backend source first, then generate from that exact commit. Do not hand-edit generated JSON:

```powershell
git add internal/platform/admin internal/runtime/worker.go internal/runtime/worker_test.go internal/jobs internal/module/ai/run internal/admincontract internal/server docs/contracts/admin-v1-workflow-contracts.md
git commit -m "feat(ai): wire billing runtime and run detail"
$backendCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
```

Confirm new money fields are decimal strings, wallet transactions retain `source_type`, `source_id` and `remark`, `request_id` is required on paid inputs, `AIMessageCancelResult.status` is the exact constant `stopping`, stopping/settlement enums are present, and generated `ai.response.failed.v1` requires `error_code` plus nullable `wallet_path`/`recharge_path`. The realtime contract source must reject a blank `error_code`; only `ai.billing.insufficient_balance` requires both navigation paths, while every other failure requires both paths to be `null`.

- [ ] **Step 3: Sync frontend contract**

From `E:\admin\admin_front_ts`, read `docs/rule.md`, then sync the exact manifest commit and regenerate types:

```powershell
$backendCommit = (Get-Content E:\admin\admin_back_go\contracts\admin\v1\manifest.json -Raw | ConvertFrom-Json).backend_commit
npm run contract:sync -- --backend E:\admin\admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

Expected: all commands exit zero and no hand-written compatibility fields appear.

- [ ] **Step 4: Run the targeted frontend contract test**

Run `npm test -- tests/unit/contracts/admin-contract.test.ts` from the frontend root. Do not run the full frontend suite in this plan.

### Task 4: Update consumer chat and wallet UI state

**Files:**
- Create: `..\admin_front_ts\src\api\ai\request-id.ts`
- Modify: `..\admin_front_ts\src\api\ai\chat.ts`
- Modify: `..\admin_front_ts\src\api\ai\tools.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\use-chat-page.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\composables\types.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\composables\useConversationSessions.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\tools\components\ToolGenerateDialog\index.vue`
- Modify: `..\admin_front_ts\src\api\ai\agents.ts`
- Modify: `..\admin_front_ts\src\api\ai\runs.ts`
- Modify: `..\admin_front_ts\src\api\wallet\index.ts`
- Modify: `..\admin_front_ts\src\modules\http\error.ts`
- Modify: `..\admin_front_ts\src\modules\realtime\protocol.ts`
- Modify: `..\admin_front_ts\src\features\ai-chat\workflow.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\agents\use-agent-admin-page.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\agents\index.vue`
- Modify: `..\admin_front_ts\src\views\Main\ai\runs\components\RunList\RunDetailDialog.vue`
- Modify: `..\admin_front_ts\src\views\Main\ai\runs\components\RunList\detail-dialog.ts`
- Modify: `..\admin_front_ts\src\views\Main\payment\wallets\index.vue`
- Modify: `..\admin_front_ts\src\views\Main\payment\ledger\index.vue`
- Modify: `..\admin_front_ts\src\views\Main\personal\wallet\index.vue`
- Test: `..\admin_front_ts\tests\shared\ai\ai-agent-api.test.ts`
- Test: `..\admin_front_ts\tests\shared\ai\ai-run-api.test.ts`
- Create: `..\admin_front_ts\tests\shared\ai\ai-tool-generate-api.test.ts`
- Create: `..\admin_front_ts\tests\shared\ai\ai-billing-error.test.ts`
- Test: `..\admin_front_ts\tests\shared\ai\ai-chat-cancel-state.test.ts`
- Create: `..\admin_front_ts\tests\shared\ai\ai-request-id.test.ts`
- Create: `..\admin_front_ts\tests\component\ai\ToolGenerateDialog.test.ts`
- Create: `..\admin_front_ts\tests\shared\wallet\wallet-money-units.test.ts`
- Modify: `..\admin_front_ts\tests\unit\realtime\protocol.test.ts`
- Modify: `..\admin_front_ts\tests\shared\http\ai-conversation-websocket-contract.test.ts`
- Modify: `..\admin_front_ts\tests\integration\features\ai-chat.test.ts`

- [ ] **Step 1: Give tool generation a stable paid request identity**

Replace the current `Date.now()+Math.random` helper with one shared Web Crypto request-ID generator in `src/api/ai/request-id.ts`. Use `crypto.randomUUID()` when available and a UUID-v4 formatter backed only by `crypto.getRandomValues()` otherwise; if Web Crypto is unavailable, throw a visible client error instead of using `Math.random`. Make chat and tool generation use it and add required `request_id` to the generated-draft request contract. `ToolGenerateDialog` creates one ID when the user starts an operation, reuses it for transport retry/replay of the unchanged form, and creates a new ID only after the form changes or the user explicitly starts another generation. Tests assert UUID format, no `Math.random`, exact body, same-operation replay identity and new-operation rotation; no runtime fallback ID may be invented inside the API adapter.

- [ ] **Step 2: Handle insufficient balance by machine contract**

Handle both contract surfaces without aliases: synchronous task-wait HTTP failures use `error.code === 'ai.billing.insufficient_balance'` plus `data.wallet_path/recharge_path`; accepted chat commands use durable `ai.response.failed.v1.error_code` plus its two nullable path fields. Present clear wallet/recharge actions in the existing error surface, and require both paths for that code. Do not match `msg`, hard-code a third route, retry the provider call or preserve a fake streaming state. Other errors continue through the normal typed error path.

- [ ] **Step 3: Keep stop in `stopping` until durable event**

On click, call a dedicated `sessions.beginStopping(conversationID, requestID)` before the HTTP request: keep `pendingRequestId`, record the stopping request ID, stop applying deltas and render the existing stopping state without putting the ID in the terminal canceled set. The cancel endpoint must then return exact `status='stopping'`; that acknowledgment is a no-op when it arrives and must not call `sessions.cancel` or resurrect a request already finalized by an earlier event. Only durable `ai.response.canceled.v1` from the committed finalizer clears pending/stopping state and records the terminal canceled ID. A failed/ambiguous HTTP wait triggers authoritative workflow recovery with the same request ID rather than resuming a stream with a possible delta gap. Tests cover event-before-ack, ack-before-event, duplicate ack, late delta suppression and HTTP-error recovery.

- [ ] **Step 4: Render money as strings**

Display wallet balance, AI charge and transaction amounts from the canonical non-negative RMB strings (`"0"` or a plain decimal with at most 8 fractional digits). Never parse them through binary `number`, format a raw units JSON value or recompute cost in the browser; presentation may prefix `¥` but must preserve the exact decimal value. For `source_type='ai_generate'`, both the Admin ledger and personal funds list render `source_id` as `Run #<id>` and the backend-authored `remark` as the readable agent/model summary; do not perform a per-row Run request, derive a summary from mutable frontend state or expose the summary as a navigation permission bypass. Tests require the exact Run identity and summary to remain visible beside the AI debit.

- [ ] **Step 5: Show usage and settlement reason**

Render the generated `pricing`, `usage_items`, `provider_attempts`, billing status/reason and decimal amount fields directly, including the valid `Run failed + billing settled` case caused by a continuation top-up failure after prior succeeded usage. Show input/output/cache/media categories and provider request IDs only on the permission-protected Run detail; consumer chat must not receive provider identifiers or API keys.

- [ ] **Step 6: Run focused frontend checks**

Run `npm test -- tests/shared/ai/ai-agent-api.test.ts tests/shared/ai/ai-run-api.test.ts tests/shared/ai/ai-tool-generate-api.test.ts tests/shared/ai/ai-billing-error.test.ts tests/shared/ai/ai-chat-cancel-state.test.ts tests/shared/ai/ai-request-id.test.ts tests/component/ai/ToolGenerateDialog.test.ts tests/shared/wallet/wallet-money-units.test.ts tests/unit/realtime/protocol.test.ts tests/shared/http/ai-conversation-websocket-contract.test.ts tests/integration/features/ai-chat.test.ts`. Run `npm run typecheck` only if it completes within two minutes; otherwise leave the command for manual execution.

### Task 5: Final fast audit and handoff

- [ ] **Step 1: Search forbidden paths**

Run `rg -n 'APP_SECRET|APP_SECRET_PREVIOUS|SourceAIRefund|float64.*Cost|balance_cents|amount_cents' internal contracts ..\admin_front_ts\src` and inspect every remaining match. Secrets must be unchanged; generated/runtime wallet contracts must be units-derived decimal strings, with no cents alias.

- [ ] **Step 2: Check docs and generated diffs**

Run `git diff --check` and `git status --short`. Do not run Docker, Playwright, full backend tests or frontend build automatically.

- [ ] **Step 3: Commit generated backend contract**

```powershell
git add contracts/admin/v1
git commit -m "chore(contract): publish ai billing contract"
```

- [ ] **Step 4: Commit frontend integration in its own repository**

```powershell
Set-Location E:\admin\admin_front_ts
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated src/api/ai src/api/wallet src/views/Main/ai src/views/Main/payment src/views/Main/personal/wallet tests/shared/ai tests/shared/wallet tests/component/ai
git commit -m "feat(ai): consume billing and stopping contracts"
```
