# AI 消费者交互契约发布 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把阶段 B 已实现的会话、消息、反馈和充值 PageInit 契约发布为闭合 OpenAPI，并同步前端生成类型。

**Architecture:** 后端编译路由与 curated workflow schema 是唯一字段来源；先提交契约源，再用该精确 commit 生成 Bundle，前端只同步 manifest 指向的 commit。不得手改 generated JSON/TypeScript，也不得为消费者接口新增 RBAC 权限。

**Tech Stack:** Admin Contract Bundle、OpenAPI 3.1、PowerShell generators、TypeScript contract sync。

---

### Task 1: Close the backend workflow schemas

**Files:**
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_workflows.go`
- Modify: `internal/admincontract/openapi_test.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `docs/contracts/admin-v1-workflow-contracts.md`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/router_test.go`

- [ ] **Step 1: Add failing contract assertions**

Require conversation `unread_count`; message nullable `paired_message_id/run_id` and boolean `liked`; edit/regenerate responses referencing `AIMessageSendResult`; sorted `deleted_ids`; exact cursor result; exact feedback `id/liked/liked_at`; Run detail `liked/liked_at`; recharge PageInit without `recent`; and the inherited Phase A cancel acknowledgment constant `status="stopping"`. Assert all new paths, methods, path IDs, body requiredness and edit/regenerate HTTP `202` exactly.

- [ ] **Step 2: Publish the workflow definitions**

Add workflow entries for message revisions, regenerations, collection delete, conversation read cursor and Run user feedback. Document canonical `(user_id, request_id)` replay semantics and nullable fields. Do not add GenericObject, aliases, compatibility fields or frontend-inferred defaults.

- [ ] **Step 3: Lock access metadata**

Update route-policy golden data from compiled routes. Message actions, read cursor and feedback are `Authenticated()` consumer endpoints; feedback must have no `ai_run_list`. Existing Run page/list/detail/stats remain under the Phase A management permission. Add no permission seed and no role grant.

- [ ] **Step 4: Run focused backend contract checks**

Run `gofmt -w internal/admincontract`, then `go test ./internal/admincontract ./internal/server -run 'Test.*OpenAPI|Test.*Workflow|Test.*RoutePolicy' -count=1` and `git diff --check`.

### Task 2: Generate from one backend commit

- [ ] **Step 1: Commit contract sources**

```powershell
git add internal/admincontract internal/server/testdata/admin_route_policy_golden.json internal/server/router_test.go docs/contracts/admin-v1-workflow-contracts.md
git commit -m "feat(contract): define ai consumer interactions"
$backendCommit = (git rev-parse HEAD).Trim()
```

- [ ] **Step 2: Generate and check the backend bundle**

```powershell
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
```

Expected: commands exit zero; OpenAPI has no `recent` PageInit field and every new schema is closed. Stop rather than run broader gates if either command exceeds two minutes.

- [ ] **Step 3: Commit generated backend artifacts**

```powershell
git add contracts/admin/v1
git commit -m "chore(contract): publish ai consumer interactions"
```

### Task 3: Sync generated frontend types

**Files:**
- Modify through sync: `..\admin_front_ts\contracts\backend\admin\*`
- Modify through generator: `..\admin_front_ts\src\modules\http\generated\*`
- Modify through generator: `..\admin_front_ts\src\modules\routing\generated\*`
- Modify: `..\admin_front_ts\tests\unit\contracts\admin-contract.test.ts`

- [ ] **Step 1: Read the frontend rule and sync exact manifest**

From `E:\admin\admin_front_ts`, read `docs/rule.md`, then run:

```powershell
$backendCommit = (Get-Content E:\admin\admin_back_go\contracts\admin\v1\manifest.json -Raw | ConvertFrom-Json).backend_commit
npm run contract:sync -- --backend E:\admin\admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

- [ ] **Step 2: Run only the contract unit test**

Run `npm test -- tests/unit/contracts/admin-contract.test.ts`. Do not run the full frontend suite or build in this plan.

- [ ] **Step 3: Commit frontend generated artifacts**

```powershell
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated tests/unit/contracts/admin-contract.test.ts
git commit -m "chore(contract): sync ai consumer interactions"
```
