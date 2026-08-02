# AI 上下文工程管理端与最终切换 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 发布闭合的 Context Admin 契约和紧凑管理工作台，在 Agent、Chat 与 Run 页面接入 Profile、Space、Citation 和 Context Plan，并通过一次受保护的 Contract 切换删除旧 Knowledge 表面与 `max_history` 发布能力。

**Architecture:** Wave 01-04 已经完成九表 Expand、摄取、检索、聊天和会话记忆，本计划只做最终管理面与原子切换。后端运行时路由是 Admin Contract Bundle 的唯一来源；后端必须先形成 clean commit 并生成 Bundle，前端才能同步和生成类型。权限迁移只自动映射语义等价的完整旧授权，任何细粒度部分授权都在写入前中止并要求管理员明确处理。

**Tech Stack:** Go 1.26.5、Gin route registry、GORM、MySQL 8.4、Atlas、Admin Contract Bundle、Vue 3.5 Composition API、TypeScript 5.9、Element Plus、Vitest。

---

## Fixed Cutover Contract

### Permission identities

| ID | Code | Purpose |
| --- | --- | --- |
| 923 | `ai_context_view` | Read Profiles, Spaces, Documents and Agent Context assignments |
| 924 | `ai_context_manage` | Create, edit, enable, disable and delete Spaces |
| 925 | `ai_context_document_manage` | Create versions, change Document status, delete and reindex |
| 926 | `ai_context_profile_manage` | Create/retire Profiles and change Agent Profile/Space assignments |
| 927 | `ai_context_evaluate` | Execute synchronous Context evaluation |

Menu ID `122` is reused and becomes `name=上下文工程`, `path=/ai/context`, `component=ai/context`, `i18n_key=menu.ai_context`. Old button IDs `123-131`, `415`, and the old Agent Knowledge binding button `413` are retired. ID `413` has no surviving non-Knowledge use.

Run Context Plan remains protected by `ai_run_list`. Message Citation remains protected by the existing conversation ownership check. Neither surface receives a second permission code.

### Exact route policy matrix

| Method and path | Access | Audit action |
| --- | --- | --- |
| `GET /api/admin/v1/ai/context-profiles` | `ai_context_view` | read-only |
| `GET /api/admin/v1/ai/context-profiles/:id` | `ai_context_view` | read-only |
| `POST /api/admin/v1/ai/context-profiles` | `ai_context_profile_manage` | `ai_context_profile.create` |
| `PUT /api/admin/v1/ai/context-profiles/:id` | `ai_context_profile_manage` | `ai_context_profile.update_metadata` |
| `PATCH /api/admin/v1/ai/context-profiles/:id/status` | `ai_context_profile_manage` | `ai_context_profile.change_status` |
| `GET /api/admin/v1/ai/context-spaces` | `ai_context_view` | read-only |
| `GET /api/admin/v1/ai/context-spaces/:id` | `ai_context_view` | read-only |
| `POST /api/admin/v1/ai/context-spaces` | `ai_context_manage` | `ai_context_space.create` |
| `PUT /api/admin/v1/ai/context-spaces/:id` | `ai_context_manage` | `ai_context_space.update` |
| `PATCH /api/admin/v1/ai/context-spaces/:id/status` | `ai_context_manage` | `ai_context_space.change_status` |
| `DELETE /api/admin/v1/ai/context-spaces/:id` | `ai_context_manage` | `ai_context_space.delete` |
| `GET /api/admin/v1/ai/context-spaces/:id/documents` | `ai_context_view` | read-only |
| `POST /api/admin/v1/ai/context-spaces/:id/documents` | `ai_context_document_manage` | `ai_context_document.create` |
| `GET /api/admin/v1/ai/context-documents/:id` | `ai_context_view` | read-only |
| `GET /api/admin/v1/ai/context-documents/:id/versions` | `ai_context_view` | read-only |
| `POST /api/admin/v1/ai/context-documents/:id/versions` | `ai_context_document_manage` | `ai_context_document.create_version` |
| `PATCH /api/admin/v1/ai/context-documents/:id/status` | `ai_context_document_manage` | `ai_context_document.change_status` |
| `DELETE /api/admin/v1/ai/context-documents/:id` | `ai_context_document_manage` | `ai_context_document.delete` |
| `POST /api/admin/v1/ai/context-documents/:id/reindex` | `ai_context_document_manage` | `ai_context_document.reindex` |
| `POST /api/admin/v1/ai/context-evaluations` | `ai_context_evaluate` | `ai_context.evaluate` |
| `GET /api/admin/v1/ai/agents/:id/context-profile` | `ai_context_view` | read-only |
| `PUT /api/admin/v1/ai/agents/:id/context-profile` | `ai_context_profile_manage` | `ai_agent_context.update_profile` |
| `GET /api/admin/v1/ai/agents/:id/context-spaces` | `ai_context_view` | read-only |
| `PUT /api/admin/v1/ai/agents/:id/context-spaces` | `ai_context_profile_manage` | `ai_agent_context.update_spaces` |

No route accepts platform, user, Profile identity, Provider identity or generation from an untrusted header/body field when the server can derive it.

### Frontend component map

| Component/composable | Single responsibility | Contract |
| --- | --- | --- |
| `ai/context/index.vue` | Compose the four workspace tabs | no business mutation |
| `use-context-workspace.ts` | Own selected tab/Profile/Space and API request state | returns readonly state plus explicit actions |
| `ContextSpacePanel.vue` | List and mutate Spaces | props: selected Profile/Space; emits: select, changed |
| `ContextDocumentPanel.vue` | Show Documents, immutable versions and ingestion state | props: Space; emits: version-created, changed |
| `ContextProfilePanel.vue` | Show immutable index configuration and lifecycle | emits: selected, changed |
| `ContextEvaluationPanel.vue` | Run one Agent/query evaluation and display returned Pack | props: available Agents; no persisted local result fallback |
| `AgentContextDialog.vue` | Assign one Profile and compatible Spaces to one Agent | props: Agent; emits: saved |
| `MessageCitationDrawer.vue` | Display cited, selected-unreferenced and invalid Citation keys | props: persisted `MessageContext`; emits: close only |
| `RunContextPlan.vue` | Render one persisted Plan budget, stages and ordered decisions | props: generated `context_plan` DTO |

All new Vue files use `<script setup lang="ts">`, generated operation types, props-down/events-up and minimal source state. The route view remains a composition surface; no runtime mock, hand-written response alias, guessed `||`/`??` field chain or `Record<string, any>` is allowed.

### Atomic operational order

```text
1. stop the old admin-api/ingress so no new chat message can create a Reply Command
2. keep the old admin-worker running until Reply/Reconciler/finalizer work drains
3. stop the old admin-worker and prove no old API/Worker process can write
4. run the read-only preflight: zero active Reply Commands, zero active Chat Attempts,
   six zero old-table counts, and every enabled Chat Agent capability valid
5. create and verify the locked database backup/restore artifact
6. apply Expand, permissions, then guarded Contract migrations consecutively
7. start the synchronized backend/frontend cutover images
8. verify readiness, all five Context task registrations, contract SHA,
   menu authorization and browser acceptance
```

This plan creates and validates migration files only. It does not execute migrations, stop claims, deploy images, restart `admin-dev` or run Playwright.

### Task 1: Create the conservative permission and role migration

**Files:**
- Create: `database/migrations/202608020102_ai_context_permissions.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/seeds/admin_permissions.sql`
- Create: `internal/architecture/ai_context_permission_contract_test.go`
- Modify: `internal/admincontract/permissions_test.go`
- Modify: `internal/admincontract/views_test.go`

- [ ] **Step 1: Write failing identity, mapping and retirement tests**

Parse the seed and migration as structured SQL fixtures. Assert IDs/codes `923-927` occur exactly once, menu `122` has the final `code=ai_context`, path/component/i18n identity, and old IDs `123-131`, `413`, `415` are absent from the final seed. Add SQL integration fixtures for full grants, menu-only grants, partial base grants, partial document grants, binding-only grants, an affected role whose principal-version row is missing, and an unaffected active Admin principal whose version must remain byte-for-byte unchanged.

The mapping test must lock these exact rules:

```text
view:             any active old grant in 122-131 or 415
manage:           all four active grants 128,129,130,131
document_manage:  all five active grants 124,125,126,127,415
evaluate:         active grant 123
profile_manage:   all four base-manage grants plus active grant 413
```

A role holding one to three base-manage grants, one to four document grants, or `413` without the complete base-manage set makes the migration fail before any permission/menu/role row is changed. This is an intentional authorization review gate, not a default role expansion.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/architecture ./internal/admincontract -run 'AIContextPermission|ContextView|KnowledgePermissionRetirement' -count=1`

Expected: FAIL because migration `202608020102_ai_context_permissions.sql` and permission IDs `923-927` do not exist.

- [ ] **Step 3: Implement the guarded mapping transaction**

The migration uses temporary role sets populated before any write. Use `GROUP BY role_id HAVING COUNT(DISTINCT permission_id)` for complete sets, and a temporary guard table with `CHECK (violations = 0)` for partial-set rejection. Then, in one transaction:

```sql
UPDATE `permissions`
SET `name` = '上下文工程',
    `path` = '/ai/context',
    `icon` = 'Collection',
    `component` = 'ai/context',
    `code` = 'ai_context',
    `i18n_key` = 'menu.ai_context',
    `sort` = 3,
    `show_menu` = 1,
    `status` = 1,
    `is_del` = 2
WHERE `id` = 122
  AND `platform` = 'admin'
  AND `type` = 2;

INSERT INTO `permissions` (
  `id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`,
  `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`
) VALUES
  (923, '查看上下文工程', '', '', 122, NULL, 'admin', 3, 1, 'ai_context_view', '', 2, 1, 2),
  (924, '管理上下文空间', '', '', 122, NULL, 'admin', 3, 2, 'ai_context_manage', '', 2, 1, 2),
  (925, '管理上下文文档', '', '', 122, NULL, 'admin', 3, 3, 'ai_context_document_manage', '', 2, 1, 2),
  (926, '管理上下文配置', '', '', 122, NULL, 'admin', 3, 4, 'ai_context_profile_manage', '', 2, 1, 2),
  (927, '执行上下文评测', '', '', 122, NULL, 'admin', 3, 5, 'ai_context_evaluate', '', 2, 1, 2);
```

Before changing grants, populate `_ai_context_affected_roles(role_id)` from
the distinct roles with an active grant in `122-131`, `413` or `415`. Insert
active `role_permissions` for each precomputed set with `INSERT ... SELECT ...
ON DUPLICATE KEY UPDATE is_del=2`. Delete role grants for retired IDs and delete
retired Permission rows. Then create missing principal-version rows and bump
only users in `_ai_context_affected_roles`; changing unrelated Admin cache
versions is forbidden:

```sql
INSERT INTO `authz_principal_versions` (`user_id`, `platform`, `version`, `updated_at`)
SELECT DISTINCT user_row.`id`, 'admin', 1, UTC_TIMESTAMP(6)
FROM `users` AS user_row
JOIN `_ai_context_affected_roles` AS affected_role
  ON affected_role.`role_id` = user_row.`role_id`
WHERE user_row.`status` = 1
  AND user_row.`is_del` = 2
ON DUPLICATE KEY UPDATE `user_id` = `authz_principal_versions`.`user_id`;

UPDATE `authz_principal_versions` AS principal_version
JOIN `users` AS user_row ON user_row.`id` = principal_version.`user_id`
JOIN `_ai_context_affected_roles` AS affected_role
  ON affected_role.`role_id` = user_row.`role_id`
SET principal_version.`version` = principal_version.`version` + 1,
    principal_version.`updated_at` = UTC_TIMESTAMP(6)
WHERE principal_version.`platform` = 'admin'
  AND user_row.`status` = 1
  AND user_row.`is_del` = 2;
```

The migration ends with assertions for the exact five new Permission rows, zero old button rows, no dangling retired role grants and one version increment per snapshotted active Admin principal. The seed contains only the final menu and five buttons; it does not preserve role-specific grants.

- [ ] **Step 4: Hash, validate and commit**

Run: `go test ./internal/architecture ./internal/admincontract -run 'AIContextPermission|ContextView|KnowledgePermissionRetirement' -count=1`

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations`

Expected: all commands exit 0; `atlas.sum` adds the permissions migration checksum and no live database is opened.

```bash
git add -- database/migrations/202608020102_ai_context_permissions.sql database/migrations/atlas.sum database/seeds/admin_permissions.sql internal/architecture/ai_context_permission_contract_test.go internal/admincontract/permissions_test.go internal/admincontract/views_test.go
git commit -m "feat(ai): cut over context permissions"
```

### Task 2: Lock Context Admin DTOs, evaluation and cutover preflight

**Files:**
- Modify: `internal/module/ai/contextengine/admin_dto.go`
- Modify: `internal/module/ai/contextengine/admin_service.go`
- Modify: `internal/module/ai/contextengine/admin_service_test.go`
- Modify: `internal/module/ai/contextengine/evaluation.go`
- Modify: `internal/module/ai/contextengine/evaluation_test.go`
- Create: `internal/module/ai/contextengine/cutover_preflight.go`
- Create: `internal/module/ai/contextengine/cutover_preflight_test.go`
- Modify: `internal/module/ai/contextengine/transport/admin/request.go`
- Modify: `internal/module/ai/contextengine/transport/admin/handler.go`
- Modify: `internal/module/ai/contextengine/transport/admin/handler_test.go`
- Modify: `internal/module/ai/contextengine/transport/admin/route.go`
- Modify: `internal/server/adminroute/compile_test.go`
- Create: `cmd/ai-context-preflight/main.go`

- [ ] **Step 1: Write failing closed-contract and authorization tests**

For every route in the fixed matrix, assert method, path, operation ID, permission code, audit/no-audit decision and concrete request/response model. Assert Profile configuration fields are immutable after reference, Space and Document state enums are closed, JSON locator/metrics/metadata use versioned structs, and errors remain distinct from empty lists.

Evaluation accepts only:

```go
type EvaluationRequest struct {
	AgentID uint64 `json:"agent_id" binding:"required,gt=0"`
	Query   string `json:"query" binding:"required,min=1,max=20000"`
}
```

It resolves the Agent's current Profile, Chat model and enabled bindings, then invokes the same query, retrieval, authority recheck and Packer code as BuildPlan. It returns `retrieval_outcome`, Budget, safe stage metrics and ordered selected/excluded Items, but creates no Run, Plan, Message, wallet row or evaluation table. There are no request-side `top_k`, score, chunk, character-budget, model-window or Space override fields.

- [ ] **Step 2: Write preflight failure matrix**

Use repository fakes and temporary SQL fixtures to cover:

```text
claimed/running/outcome_unknown Reply Command -> fail with count
prepared/dispatched/outcome_unknown Chat Attempt -> fail with count and IDs
each non-empty old Knowledge table -> fail with table name and count
enabled Chat Agent with missing Provider/Model -> fail with IDs
enabled Chat Agent with non-chat model kind -> fail with IDs
missing/unknown API protocol -> fail with IDs
missing/non-positive context window or max output -> fail with IDs
unknown token counter -> fail with IDs
all checks valid -> exit success with counts only
```

The command logs no API key, message content, object key, signed URL, query, Prompt or Context snapshot. It opens the configured database read-only and performs no update, task enqueue or Qdrant mutation.

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/server/adminroute ./cmd/ai-context-preflight -run 'AdminContract|Evaluation|CutoverPreflight' -count=1`

Expected: FAIL because evaluation, exact route policies and preflight are incomplete.

- [ ] **Step 4: Implement one route table and one read-only preflight**

Register the fixed matrix directly in `transport/admin/route.go`; do not create permission middleware inside handlers. Evaluation reuses injected `RetrievalPipeline` and `ContextPacker` interfaces and sets `persist=false`; it must not copy their algorithms.

The preflight returns a typed report and the CLI maps any non-empty violation list to exit code 1:

```go
type CutoverViolation struct {
	Code       string
	ResourceID uint64
	Detail     string
}

type CutoverReport struct {
	ReplyCommandCount uint64
	ChatAttemptCount  uint64
	LegacyTableCounts map[string]uint64
	CheckedAgentCount uint64
	Violations        []CutoverViolation
}
```

`LegacyTableCounts` is internal CLI output, not an HTTP DTO and not persisted. Its keys are created from the six fixed table constants, never database input.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/server/adminroute ./cmd/ai-context-preflight -run 'AdminContract|Evaluation|CutoverPreflight' -count=1`

Expected: PASS with exact permission and failure-code assertions.

```bash
git add -- internal/module/ai/contextengine cmd/ai-context-preflight internal/server/adminroute/compile_test.go
git commit -m "feat(ai): finalize context administration contract"
```

### Task 3: Switch the backend HTTP contract and disconnect Knowledge

**Files:**
- Modify: `internal/server/routes_admin_ai.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/server/route_registry_integration_test.go`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/dependencies_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/platform/admin/graph.go`
- Modify: `internal/platform/admin/graph_test.go`
- Modify: `internal/module/ai/agent/dto.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/service_test.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_test.go`
- Modify: `internal/admincontract/views.go`
- Modify: `internal/admincontract/views_test.go`
- Modify: `docs/contracts/admin-v1-workflow-contracts.md`

- [ ] **Step 1: Write failing final-surface tests**

Assert compiled routes and OpenAPI contain every Context route and contain none of:

```text
/api/admin/v1/ai-knowledge-bases
/api/admin/v1/ai-knowledge-documents
/api/admin/v1/ai-agents/:id/knowledge-bases
AIRunKnowledgeRetrieval
AIRunKnowledgeHit
knowledge_retrievals
max_history
```

Assert `GET /api/admin/v1/ai-runs/:id` returns one nullable `context_plan` and no old retrieval array. Assert Assistant `AIMessageItem.context` is nullable for historical/pending/User rows and is the exact persisted projection from Plan 03 for completed/stopped Assistant rows. Unknown Citation keys contain only the key, never a guessed source.

Assert both concrete route fixtures are regenerated from the compiled registry:
`internal/server/testdata/admin_routes_golden.txt` contains every final Context
route and no Knowledge route, while
`internal/server/testdata/admin_route_policy_golden.json` additionally locks
operation ID, permission, audit and policy metadata. No hand-edited fixture is
accepted.

Old JSON clients may still send integer `runtime_params.max_history`. Handler binding and historical `meta_json` reading continue to succeed, but the value is ignored and omitted from output/OpenAPI. Invalid JSON types still fail normal JSON decoding; no string/float coercion is added.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/server ./internal/platform/admin ./internal/module/ai/agent ./internal/module/ai/message ./internal/module/ai/run ./internal/admincontract -run 'Context|KnowledgeRetired|MaxHistoryCompatibility|Citation' -count=1`

Expected: FAIL because old routes, Run Knowledge fields and published `max_history` still exist.

- [ ] **Step 3: Perform the runtime switch without deleting files yet**

Register Context transport and remove Knowledge transport from `routes_admin_ai.go`. Delete `knowledgeRuntimeAdapter` construction and all old Run repository/service reads of `ai_knowledge_retrievals` and `ai_knowledge_retrieval_hits`. Keep the old module directory untouched until Task 4 so this checkpoint isolates routing/contract regressions from physical deletion.

Agent output publishes nullable `context_profile_id` and display-only Profile name/state. Agent runtime capabilities publish only supported active runtime parameters; remove `MaxHistoryParameterCapability` entirely. The message request Go type retains an internal `MaxHistory *int` compatibility member, while curated `AIRuntimeParams` exposes only `temperature`.

Replace the Run detail section with Plan 03's closed `ContextPlanDetail`. It includes Budget, `retrieval_outcome`, state/error, safe metrics, ordered Items and locator, but no query text, object key, signed URL, unrestricted JSON or raw Provider response.

- [ ] **Step 4: Update human-readable contract and run removal checks**

Document each final Context route, request, response, permission and nullable field. Explicitly state that `max_history` is accepted only as an unpublished ignored compatibility input and has no runtime effect.

Run: `rg -n 'KnowledgeRuntime|knowledgeRuntimeAdapter|KnowledgeRetrievals|KnowledgeRetrievalHits' internal/server internal/platform/admin internal/module/ai/chat internal/module/ai/run`

Expected: no active runtime match.

Regenerate both route fixtures from the actual compiled router, then immediately
rerun without update flags:

```powershell
$env:UPDATE_ADMIN_ROUTE_SNAPSHOT = '1'
go test ./internal/server -run '^TestAdminRouteSnapshot$' -count=1
Remove-Item Env:UPDATE_ADMIN_ROUTE_SNAPSHOT
$env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN = '1'
go test ./internal/server -run '^TestRoutePolicyGoldenIsAdminOnlyAndCurrent$' -count=1
Remove-Item Env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN
go test ./internal/server -run 'TestAdminRouteSnapshot|TestRoutePolicyGoldenIsAdminOnlyAndCurrent' -count=1
```

Expected: all three commands exit 0; update flags are absent for the final run.

Run: `go test ./internal/server ./internal/platform/admin ./internal/module/ai/agent ./internal/module/ai/message ./internal/module/ai/run ./internal/admincontract -run 'Context|KnowledgeRetired|MaxHistoryCompatibility|Citation' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the backend contract source**

```bash
git add -- internal/server/routes_admin_ai.go internal/server/router_test.go internal/server/route_registry_integration_test.go internal/server/testdata/admin_routes_golden.txt internal/server/testdata/admin_route_policy_golden.json internal/server/dependencies_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go internal/platform/admin/graph.go internal/platform/admin/graph_test.go internal/module/ai/agent internal/module/ai/message internal/module/ai/run internal/admincontract docs/contracts/admin-v1-workflow-contracts.md
git commit -m "feat(ai): cut over context admin runtime"
```

Run: `git status --short`

Expected: clean before generating the Bundle.

### Task 4: Retire the backend Knowledge module and create Contract DDL

**Files:**
- Delete: `internal/module/ai/knowledge/chunker.go`
- Delete: `internal/module/ai/knowledge/chunker_test.go`
- Delete: `internal/module/ai/knowledge/dto.go`
- Delete: `internal/module/ai/knowledge/model.go`
- Delete: `internal/module/ai/knowledge/repository.go`
- Delete: `internal/module/ai/knowledge/repository_test.go`
- Delete: `internal/module/ai/knowledge/retriever.go`
- Delete: `internal/module/ai/knowledge/retriever_test.go`
- Delete: `internal/module/ai/knowledge/service.go`
- Delete: `internal/module/ai/knowledge/service_test.go`
- Delete: `internal/module/ai/knowledge/transport/admin/handler.go`
- Delete: `internal/module/ai/knowledge/transport/admin/request.go`
- Delete: `internal/module/ai/knowledge/transport/admin/route.go`
- Delete: `internal/architecture/ai_knowledge_aggregation_test.go`
- Modify: `internal/architecture/ai_capability_boundary_test.go`
- Modify: `internal/architecture/ai_context_schema_contract_test.go`
- Create: `database/migrations/202608020103_ai_context_contract.sql`
- Modify: `database/schema/admin.hcl`
- Modify: `database/migrations/atlas.sum`
- Create: `scripts/tests/ai-context-cutover-migrations.tests.ps1`
- Modify: `docs/architecture.md`
- Modify: `docs/contracts/admin-v1-workflow-contracts.md`

- [ ] **Step 1: Write failing physical-retirement and final-schema tests**

Assert all thirteen Go files and `ai_knowledge_aggregation_test.go` are absent,
no production Go import contains `internal/module/ai/knowledge`, and the final
HCL contains exactly the nine Context tables but none of the six Knowledge
tables. Lock all of these Contract facts:

```text
Expand 202608020101: ai_provider_models.model_kind has DEFAULT 'chat'
Contract 202608020103: six explicit non-empty guards precede every DROP
Contract 202608020103: model_kind DROP DEFAULT follows all six table guards
final HCL:             model_kind has its closed CHECK and no default
legacy migration:      database/legacy-migrations/20260510_ai_knowledge_rag.sql unchanged
```

Extend `TestRetainedAICapabilitiesAreTransportNeutral` to include
`contextengine`, and add exact `mustNotExist` assertions rather than a wildcard
directory assertion. The test may name the guarded Contract migration, but no
runtime code may retain a Knowledge symbol.

- [ ] **Step 2: Run the retirement tests and verify RED**

Run: `go test ./internal/architecture ./internal/admincontract ./internal/server ./internal/platform/admin -run 'KnowledgeRetired|ContextOnly|AIContextContract' -count=1`

Expected: FAIL while the old Go module, six HCL tables and Contract migration
still exist/do not exist in their pre-cutover state.

- [ ] **Step 3: Write the guarded Contract migration before deleting schema facts**

Create a temporary stored procedure using Atlas's MySQL delimiter directive.
It performs six separate `SELECT COUNT(*) INTO legacy_rows` checks in this
exact child-to-parent order and uses `SIGNAL SQLSTATE '45000'` with
`table=<name>, rows=<count>` for each non-zero result:

```text
ai_knowledge_retrieval_hits
ai_knowledge_retrievals
ai_agent_knowledge_bases
ai_knowledge_chunks
ai_knowledge_documents
ai_knowledge_bases
```

Call the procedure before the first `DROP TABLE`, then drop the procedure. Only
after all six assertions pass, drop the six tables in the same order. MySQL DDL
autocommits, so the migration and runbook must never promise rollback after a
DROP. Finally remove the Expand compatibility default without weakening the
closed CHECK:

```sql
ALTER TABLE `ai_provider_models`
  ALTER COLUMN `model_kind` DROP DEFAULT;
```

Update `database/schema/admin.hcl` to the exact post-Contract shape. Never edit
or execute `database/legacy-migrations/20260510_ai_knowledge_rag.sql`.

- [ ] **Step 4: Delete the exact backend files and finish runtime documentation**

Delete only the files listed above. Remove the last Knowledge graph/service
claims from architecture and workflow documentation; keep historical migration
references clearly marked as audit-only. Document MySQL as authority, Qdrant as
rebuildable derivation, the one-terminal-Plan rule, persisted request replay,
and the no-live-migration rule. Do not leave a compatibility adapter or empty
Knowledge package.

- [ ] **Step 5: Prove DDL on owned disposable schemas**

Run:
`pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`

Run:
`pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations`

Run:
`pwsh -NoProfile -File scripts/tests/ai-context-cutover-migrations.tests.ps1`

Expected: PASS. The script owns a uniquely labeled `mysql:8.4.10` container and
regex-validated disposable schemas only. It must prove:

```text
pre-Contract HCL + 202608020103 fingerprint == final HCL fingerprint
each one of the six non-empty legacy-table fixtures aborts before any DROP
all-six-empty fixture drops exactly six tables and removes model_kind default
full/menu-only grants map exactly as specified
partial base/document/binding grants abort before permission/menu/role writes
affected principals increment once; unaffected Admin principals do not change
```

The script snapshots permission, role-grant and principal-version rows before
failure cases and compares them after. It binds any published port to loopback,
does not read `admin-go.env`, never targets database `admin`, and cleans only
its own labeled container/schemas in `finally`.

- [ ] **Step 6: Run backend removal checks and commit a clean runtime**

Run: `go test ./internal/architecture ./internal/admincontract ./internal/server ./internal/platform/admin ./internal/module/ai/... -run 'KnowledgeRetired|ContextOnly|AIContextContract|MaxHistory' -count=1`

Run:

```powershell
rg -n 'internal/module/ai/knowledge|KnowledgeRuntime|knowledgeRuntimeAdapter|ai_knowledge_(bases|documents|chunks|retrievals|retrieval_hits)|ai_agent_knowledge_bases' internal cmd database/schema docs/architecture.md docs/contracts
```

Expected: no active runtime/schema match; only explicitly asserted historical
or guarded Contract references are allowed.

```bash
git add -- database/migrations/202608020103_ai_context_contract.sql database/migrations/atlas.sum database/schema/admin.hcl scripts/tests/ai-context-cutover-migrations.tests.ps1 internal/module/ai/knowledge internal/architecture/ai_knowledge_aggregation_test.go internal/architecture/ai_capability_boundary_test.go internal/architecture/ai_context_schema_contract_test.go docs/architecture.md docs/contracts/admin-v1-workflow-contracts.md
git commit -m "refactor(ai): retire legacy knowledge backend"
```

Run: `git status --short`

Expected: clean. This committed SHA is the only valid source for the final
Admin Contract Bundle. Do not start Bundle generation from Task 3 or from a
working tree where the old module/Contract schema still exists.

### Task 5: Generate the final backend Bundle

**Files:**
- Modify generated: `contracts/admin/v1/openapi.json`
- Modify generated: `contracts/admin/v1/permissions.json`
- Modify generated: `contracts/admin/v1/views.json`
- Modify generated: `contracts/admin/v1/manifest.json`

- [ ] **Step 1: Generate from a committed clean backend**

From `E:/admin/admin_back_go`:

```powershell
$backendCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
```

Expected: Bundle contains Context operations/permissions/view, no Knowledge operation/schema/permission and no `max_history` schema property.

- [ ] **Step 2: Commit generated backend artifacts**

```bash
git add -- contracts/admin/v1
git commit -m "chore(contract): publish context engineering contract"
```

Run the generator again with the manifest's bound commit and require zero drift:

```powershell
$manifest = Get-Content -Raw contracts/admin/v1/manifest.json | ConvertFrom-Json
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $manifest.backend_commit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $manifest.backend_commit
git diff --exit-code -- contracts/admin/v1
```

- [ ] **Step 3: Record the immutable backend handoff**

Run: `git status --short`

Expected: clean. Record the Bundle commit, `manifest.backend_commit`, and the
SHA-256 of all four files. Frontend synchronization deliberately starts in Task
6 and is not committed until Task 9 has removed every old generated consumer;
there is no intermediate frontend commit that references deleted operations.

### Task 6: Synchronize the Contract and build the Context workspace atomically

**Files:**
- Modify generated: `E:/admin/admin_front_ts/contracts/backend/admin/v1/openapi.json`
- Modify generated: `E:/admin/admin_front_ts/contracts/backend/admin/v1/permissions.json`
- Modify generated: `E:/admin/admin_front_ts/contracts/backend/admin/v1/views.json`
- Modify generated: `E:/admin/admin_front_ts/contracts/backend/admin/v1/manifest.json`
- Modify generated: `E:/admin/admin_front_ts/contracts/backend/admin/lock.json`
- Modify generated: `E:/admin/admin_front_ts/src/modules/http/generated/admin.ts`
- Modify generated: `E:/admin/admin_front_ts/src/modules/http/generated/operations.ts`
- Modify generated: `E:/admin/admin_front_ts/src/modules/routing/generated/permissions.ts`
- Modify generated: `E:/admin/admin_front_ts/src/modules/routing/generated/views.ts`
- Modify: `E:/admin/admin_front_ts/tests/unit/contracts/admin-contract.test.ts`
- Create: `E:/admin/admin_front_ts/src/api/ai/context.ts`
- Modify: `E:/admin/admin_front_ts/src/lib/upload/uploadClient.ts`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/index.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/use-context-workspace.ts`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextSpacePanel.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextDocumentPanel.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextProfilePanel.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextEvaluationPanel.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextSpaceDialog.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextProfileDialog.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextVersionDialog.vue`
- Create: `E:/admin/admin_front_ts/tests/shared/ai/ai-context-api.test.ts`
- Create: `E:/admin/admin_front_ts/tests/shared/ai/ai-context-upload.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/system/upload-client-url.test.ts`
- Create: `E:/admin/admin_front_ts/tests/component/ai/ContextWorkspace.test.ts`
- Create: `E:/admin/admin_front_ts/tests/component/ai/ContextDocuments.test.ts`
- Create: `E:/admin/admin_front_ts/tests/component/ai/ContextEvaluation.test.ts`

Tasks 6-9 are one atomic frontend working-tree sequence. Synchronizing the
final Contract immediately removes old Knowledge operations, so there must be
no frontend commit and no full `typecheck` between this step and Task 9's final
green gate. Do not add a generated Knowledge alias or hand-written compatibility
DTO to manufacture an intermediate green state.

- [ ] **Step 1: Synchronize the final committed backend Contract**

From `E:/admin/admin_front_ts`, require a clean starting tree, then run:

```powershell
$backendCommit = (Get-Content -Raw ..\admin_back_go\contracts\admin\v1\manifest.json | ConvertFrom-Json).backend_commit
npm run contract:sync -- --backend ..\admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

Expected: generated operations expose the final Context routes, five permission
codes, `ai/context`, Citation/Plan DTOs, and contain no Knowledge operation or
`max_history`. Do not commit yet; old consumers are removed across Tasks 6-9.

- [ ] **Step 2: Write failing API, upload-fact and state tests**

Assert exact generated operation selection, path/query/body mapping, positive ID checks and no fallback fields. Component tests cover loading, success, documented empty, missing required field and request error as distinct states. Add status transitions for `queued/processing/ready/failed`, immutable version history, active version, sanitized failure code/message, Profile ready/rebuilding/failed, and evaluation `skipped/no_hit/hit/failed`.

Upload tests prove the token request accepts exactly
`folder='ai_context_documents'`; the COS callback's exact non-empty `ETag` is
retained; Context version creation submits
`storage_provider/object_key/etag/size_bytes/mime_type/filename`; and a missing
ETag is an explicit Context upload error before the business request. Existing
Chat image/file uploads must still succeed and keep their current URL/key
behavior when callers do not request Context object facts.

Use contract-valid fixture IDs that differ across Profile, Space, Document, Version and Agent so accidental ID reuse fails.

- [ ] **Step 3: Run tests and verify RED**

Run: `npm test -- --run tests/shared/ai/ai-context-api.test.ts tests/shared/ai/ai-context-upload.test.ts tests/shared/system/upload-client-url.test.ts tests/component/ai/ContextWorkspace.test.ts tests/component/ai/ContextDocuments.test.ts tests/component/ai/ContextEvaluation.test.ts`

Expected: FAIL because Context frontend files do not exist.

- [ ] **Step 4: Implement a generated-operation-only API**

Derive every type from `AdminOperationInput`/`AdminOperationOutput` and call `executeAdminOperation`. The adapter exports no duplicate backend DTO and no guessed field transformer. Its public surface is grouped by responsibility:

```ts
export const AiContextApi = {
  profiles: { list, detail, create, update, changeStatus },
  spaces: { list, detail, create, update, changeStatus, remove },
  documents: { list, detail, versions, create, createVersion, changeStatus, remove, reindex },
  evaluations: { run },
  agents: { profile, updateProfile, spaces, updateSpaces },
} as const
```

Optional request properties are included only when the generated contract declares them and the caller supplied them. A backend error remains an error; it never becomes an empty list.

Extend the existing upload client without changing Chat semantics. Its low-level
COS result carries optional provider `etag` in addition to URL/key; the Context
adapter requires that field and constructs the generated closed object-facts
request using `file.size`, `file.type`, `file.name`, provider `cos`, the exact
issued key and exact returned ETag. Do not parse a public URL back into an
object key and do not invent an ETag from key, size or hash.

- [ ] **Step 5: Implement the workspace component map**

Use an `el-tabs` workspace with four stable panes: Spaces, Documents, Index Profiles and Evaluation. Use tables, status tags, compact forms, upload/version dialogs and an unframed split layout; do not recreate the old card gallery. Profile policy fields are read-only after create. Do not display or accept `top_k`, thresholds, character budgets or chunk size.

`use-context-workspace.ts` owns replaceable payloads in refs, primitive tab/ID state in shallow refs, pure computed selections and explicit async actions. Watch selected Profile/Space only for fetch side effects and cancel stale requests through the existing HTTP execution options.

Document upload follows the existing trusted upload/object-fact flow and submits only the generated request fields. It never sends file bytes/Base64 to the Context JSON API and never creates a client-only fake `ready` state.

- [ ] **Step 6: Run the focused slice without creating a broken commit**

Run: `npm test -- --run tests/shared/ai/ai-context-api.test.ts tests/shared/ai/ai-context-upload.test.ts tests/shared/system/upload-client-url.test.ts tests/component/ai/ContextWorkspace.test.ts tests/component/ai/ContextDocuments.test.ts tests/component/ai/ContextEvaluation.test.ts`

Expected: PASS.

Do not run full typecheck and do not commit. The generated Contract has already
removed operations still consumed by untouched Agent/Run/Knowledge files; Tasks
7-9 finish that atomic replacement.

### Task 7: Replace Agent Knowledge binding with Profile and Space controls

**Files:**
- Modify: `E:/admin/admin_front_ts/src/api/ai/agents.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/agents/use-agent-admin-page.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/agents/index.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/agents/styles.css`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/agents/components/AgentContextDialog.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/agents/components/AgentKnowledgeDialog/index.vue`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/ai-agent-api.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/component/ai/AgentSystemPromptEditor.test.ts`
- Create: `E:/admin/admin_front_ts/tests/component/ai/AgentContextDialog.test.ts`

- [ ] **Step 1: Write failing Profile/Space tests**

Cover Profile `null` as deliberate pure chat, selected ready Profile with zero Spaces, compatible Spaces, mixed-Profile response as a contract error, retired/failed Profile display, forbidden save, Profile conflict and successful refresh. Assert the UI contains none of `top_k`, minimum score, Context chars or chunk settings. Update `AgentSystemPromptEditor.test.ts` to mock `AgentContextDialog` and the exact `contextDialogVisible`/`contextAgent`/`openContext` page state; no old Knowledge component or state name may survive in this existing Agent-page regression.

- [ ] **Step 2: Run tests and verify RED**

Run: `npm test -- --run tests/shared/ai/ai-agent-api.test.ts tests/component/ai/AgentContextDialog.test.ts tests/component/ai/AgentSystemPromptEditor.test.ts`

Expected: FAIL because Agent still opens `AgentKnowledgeDialog` and old binding operations.

- [ ] **Step 3: Implement explicit two-stage assignment**

Replace the Knowledge action with Context configuration. The dialog loads the persisted Agent Profile and Space bindings from the dedicated generated operations. Saving Profile calls `PUT .../context-profile`; after success it reloads compatible Spaces. Saving bindings calls `PUT .../context-spaces` with exact Space IDs. A changed Profile is never inferred from selected Spaces, and a second-call failure displays the persisted Profile plus the binding error instead of rolling back UI state locally.

`src/api/ai/agents.ts` only deletes its old Knowledge operation wrappers. The
dialog consumes `AiContextApi.agents` from Task 6; it must not define a second
Context request mapper or duplicate generated DTO aliases.

Profile options show names and lifecycle/index state; raw IDs remain internal select values. Profile `null` disables Space selection and sends the contract's explicit nullable value. A selected Profile with an empty Space array remains valid and is described as private conversation Context, not as missing configuration.

- [ ] **Step 4: Run focused tests and keep the atomic tree uncommitted**

Run: `npm test -- --run tests/shared/ai/ai-agent-api.test.ts tests/component/ai/AgentContextDialog.test.ts tests/component/ai/AgentSystemPromptEditor.test.ts tests/component/ai/AgentOfficialModelForm.test.ts`

Expected: PASS.

Do not commit or run full typecheck yet; Task 9 owns the first complete frontend
green checkpoint.

### Task 8: Add persisted Citation and Run Context Plan views

**Files:**
- Modify: `E:/admin/admin_front_ts/src/api/ai/messages.ts`
- Modify: `E:/admin/admin_front_ts/src/api/ai/runs.ts`
- Modify: `E:/admin/admin_front_ts/src/components/MarkdownRenderer/src/index.vue`
- Modify: `E:/admin/admin_front_ts/tests/component/markdown/MarkdownRenderer.test.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageList/index.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageList/MessageCitationDrawer.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/composables/types.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/use-chat-page.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/detail-dialog.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/input-snapshot.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/presenters.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/RunDetailDialog.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/RunInputSnapshot.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/run-detail-dialog.css`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/RunContextPlan.vue`
- Create: `E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/context-plan.ts`
- Create: `E:/admin/admin_front_ts/tests/component/ai/ChatCitations.test.ts`
- Create: `E:/admin/admin_front_ts/tests/component/ai/RunContextPlan.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/component/ai/RunInputSnapshot.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/ai-run-api.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/ai-run-input-snapshot.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/page-presenters.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/integration/features/ai-runs.test.ts`

- [ ] **Step 1: Write failing persisted-source tests**

Chat tests cover repeated valid `[C1]`, valid `[C2]`, selected-unreferenced source, unknown `[C99]`, malformed key, stopped Assistant Message and full page reload. Only keys listed by `message.context.sources` become interactive. Unknown keys remain plain answer text and appear in the drawer's invalid-key section without a source.

Run tests cover nullable Plan, `skipped/no_hit/hit/failed`, exact/conservative/opaque budget proof, stage metrics, selected/excluded rows, nullable scores, closed exclusion reason, locator and safe bounded snapshot. Missing required Plan fields are a contract error, not a Knowledge fallback.

- [ ] **Step 2: Run tests and verify RED**

Run: `npm test -- --run tests/component/markdown/MarkdownRenderer.test.ts tests/component/ai/ChatCitations.test.ts tests/component/ai/RunContextPlan.test.ts tests/shared/ai/ai-run-api.test.ts`

Expected: FAIL because current Markdown/Chat/Run views do not consume Context DTOs.

- [ ] **Step 3: Make only server-validated citations interactive**

Extend `MarkdownRenderer` with `citationKeys?: readonly string[]` and a typed `citation` emit. A Markdown-it inline rule replaces an exact server-validated `[C<number>]` token with a sanitized button carrying `data-citation-key`; it never recognizes a key absent from `citationKeys`. The delegated click handler emits that key. HTML remains disabled and output still passes through `vSafeHtml`.

`MessageList` opens `MessageCitationDrawer` from persisted `message.context`. Streaming state does not manufacture Context; after terminal completion the existing authoritative message refresh supplies it. The drawer separates `cited=true`, `cited=false` and `invalid_keys`, and renders title/locator/snapshot as escaped text.

- [ ] **Step 4: Replace Run Knowledge cards with one Plan component**

Delete Knowledge presenter helpers and markup. `context-plan.ts` only maps closed enum values to locale keys/tag types and formats fixed decimal scores; it does not read raw JSON. `RunContextPlan.vue` renders budget scalars and proof, stage metrics, ordered decisions, citations, source locator and exclusion reason in tables/collapse rows with stable dimensions.

- [ ] **Step 5: Run focused tests and keep the atomic tree uncommitted**

Run: `npm test -- --run tests/component/markdown/MarkdownRenderer.test.ts tests/component/ai/ChatCitations.test.ts tests/component/ai/RunContextPlan.test.ts tests/shared/ai/ai-run-api.test.ts tests/component/ai/ChatStopDelivery.test.ts tests/component/ai/RunInputSnapshot.test.ts`

Expected: PASS.

Do not commit or run full typecheck yet. Task 9 removes the remaining stale
Knowledge and `max_history` consumers, generates routes/locales, then commits
the whole frontend cutover once.

### Task 9: Remove the remaining frontend Knowledge surface and publish one green cutover

**Files:**
- Delete: `E:/admin/admin_front_ts/src/api/ai/knowledge.ts`
- Delete: `E:/admin/admin_front_ts/src/api/ai/knowledge.types.ts`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeBaseCard/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeBaseFormDialog/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeBaseList/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeChunkDialog/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeDocumentFormDialog/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeDocumentHero/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/KnowledgeDocumentPanel/index.vue`
- Delete: `E:/admin/admin_front_ts/src/views/Main/ai/knowledge/components/RetrievalTestDialog/index.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageInput/index.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageInput/RuntimeParamsPanel.vue`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageInput/runtime-params.ts`
- Modify: `E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageInput/runtime-params-panel.css`
- Modify: `E:/admin/admin_front_ts/tests/component/ai/RuntimeParamsPanel.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/component/ai/ChatAttachments.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/component/ai/MessageInteractions.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/component/ai/AgentOfficialModelForm.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/ai-chat-capabilities.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/ai-agent-billing-config.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/ai/admin-ai-interaction-retirement.test.ts`
- Modify: `E:/admin/admin_front_ts/scripts/test-migration-manifest.json`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/zh-CN/layout.ts`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/en-US/layout.ts`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/zh-CN/ai.ts`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/en-US/ai.ts`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/zh-CN/ai-extended.ts`
- Modify: `E:/admin/admin_front_ts/src/i18n/locales/en-US/ai-extended.ts`
- Modify generated: `E:/admin/admin_front_ts/src/i18n/locales/generated.ts`
- Modify generated: `E:/admin/admin_front_ts/src/modules/routing/generated/local-views.ts`

- [ ] **Step 1: Write failing physical-retirement checks**

Extend `admin-ai-interaction-retirement.test.ts` with every exact deleted path
above and scan source, tests, generated Contract and registries for camelCase,
snake_case and PascalCase Knowledge identities. Update
`scripts/test-migration-manifest.json` by removing the stale
`tests/shared/ai/ai-knowledge-api.test.ts` entry; do not replace it with a
nonexistent path.

Change every remaining Agent and Chat capability fixture, plus the runtime
parameter regression, to the final generated capability shape: Temperature is
the only runtime parameter and serialized requests cannot contain
`max_history`. This explicitly includes `AgentOfficialModelForm.test.ts` and
`ai-agent-billing-config.test.ts`; the zero-symbol scan must not be made green by
excluding fixture files.

- [ ] **Step 2: Run tests and verify RED**

Run from frontend: `npm test -- --run tests/component/ai/RuntimeParamsPanel.test.ts tests/component/ai/AgentOfficialModelForm.test.ts tests/shared/ai/ai-agent-billing-config.test.ts tests/unit/contracts/admin-contract.test.ts tests/unit/routing/contracts.test.ts tests/unit/i18n/locale-generator.test.ts`

Expected: FAIL while the old frontend files, locale keys and runtime parameter
consumers remain. The final generated backend Contract is already synchronized;
do not restore removed operations to make this test pass.

- [ ] **Step 3: Delete old UI and replace locale domains exactly**

Delete the exact frontend Knowledge files listed above. Tasks 7 and 8 have
already removed Agent and Run consumers; finish the Chat
`maxHistory`/`transitionalParam` state, serialization and capability fixtures,
then remove the complete `aiKnowledge` locale tree in both languages. Any old
consumer still found is fixed here, never hidden behind a generated alias.

Replace menu key `menu.ai_knowledge` with `menu.ai_context` (`上下文工程` / `Context Engineering`). Add matching `aiContext`, `aiAgents.context`, `aiChat.citations` and `aiRuns.contextPlan` key trees in both locales for the labels exercised by Tasks 6-8: Space, Document, Version, Profile, Evaluation, status/outcome, budget proof, selected/excluded, cited/unreferenced/invalid, stages, scores, locator, errors and actions. English and Chinese key paths must be identical; no visible key is built dynamically from an unchecked server string.

Remove the complete `maxHistory` state/model/serialization path from Message Input. Keep backend compatibility only; the frontend never sends it.

- [ ] **Step 4: Regenerate local view and locale registries**

From `E:/admin/admin_front_ts`:

```powershell
npm run routes:generate
npm run locale:generate
npm run routes:check
npm run locale:check
npm test -- --run tests/component/ai/RuntimeParamsPanel.test.ts tests/component/ai/AgentOfficialModelForm.test.ts tests/shared/ai/ai-agent-billing-config.test.ts tests/unit/contracts/admin-contract.test.ts tests/unit/routing/contracts.test.ts tests/unit/i18n/locale-generator.test.ts
```

Expected: all commands exit 0; generated local view contains `ai/context` and not `ai/knowledge`.

- [ ] **Step 5: Run the first full green frontend checkpoint**

```powershell
npm run contract:check
npm run routes:check
npm run locale:check
npm test -- --run tests/unit/contracts/admin-contract.test.ts tests/unit/http/contract-schema.test.ts tests/unit/routing/contracts.test.ts tests/unit/i18n/locale-generator.test.ts tests/shared/system/upload-client-url.test.ts tests/shared/ai/ai-context-api.test.ts tests/shared/ai/ai-context-upload.test.ts tests/shared/ai/ai-agent-api.test.ts tests/shared/ai/ai-agent-billing-config.test.ts tests/shared/ai/ai-run-api.test.ts tests/shared/ai/ai-run-input-snapshot.test.ts tests/shared/ai/page-presenters.test.ts tests/shared/ai/ai-chat-capabilities.test.ts tests/shared/ai/admin-ai-interaction-retirement.test.ts tests/component/ai/ContextWorkspace.test.ts tests/component/ai/ContextDocuments.test.ts tests/component/ai/ContextEvaluation.test.ts tests/component/ai/AgentContextDialog.test.ts tests/component/ai/AgentOfficialModelForm.test.ts tests/component/ai/AgentSystemPromptEditor.test.ts tests/component/ai/ChatCitations.test.ts tests/component/ai/RunContextPlan.test.ts tests/component/ai/RunInputSnapshot.test.ts tests/component/ai/RuntimeParamsPanel.test.ts tests/component/ai/ChatAttachments.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/markdown/MarkdownRenderer.test.ts tests/integration/features/ai-runs.test.ts
npm run typecheck
```

Expected: all commands exit 0. This is the first point after Contract sync at
which a full frontend compile is allowed to be claimed green.

- [ ] **Step 6: Run zero-symbol scans and commit the atomic frontend cutover**

Run from `E:/admin/admin_front_ts`:

```powershell
rg -n 'aiKnowledge|ai_knowledge|ai/knowledge|AgentKnowledge|maxHistory|max_history|knowledgeRetrieval|knowledge_retrieval|knowledge_base|KnowledgeRetrieval|KnowledgeBase' src contracts/backend/admin scripts/test-migration-manifest.json tests -g '!admin-ai-interaction-retirement.test.ts'
```

Expected: zero matches. The retirement test is excluded only because it owns
the explicit forbidden-token/path assertions.

```powershell
$frontendCutoverPaths = @(
  'contracts/backend/admin/lock.json'
  'contracts/backend/admin/v1/manifest.json'
  'contracts/backend/admin/v1/openapi.json'
  'contracts/backend/admin/v1/permissions.json'
  'contracts/backend/admin/v1/views.json'
  'scripts/test-migration-manifest.json'
  'src/api/ai/agents.ts'
  'src/api/ai/context.ts'
  'src/api/ai/knowledge.ts'
  'src/api/ai/knowledge.types.ts'
  'src/api/ai/messages.ts'
  'src/api/ai/runs.ts'
  'src/components/MarkdownRenderer/src/index.vue'
  'src/i18n/locales/en-US/ai-extended.ts'
  'src/i18n/locales/en-US/ai.ts'
  'src/i18n/locales/en-US/layout.ts'
  'src/i18n/locales/generated.ts'
  'src/i18n/locales/zh-CN/ai-extended.ts'
  'src/i18n/locales/zh-CN/ai.ts'
  'src/i18n/locales/zh-CN/layout.ts'
  'src/lib/upload/uploadClient.ts'
  'src/modules/http/generated/admin.ts'
  'src/modules/http/generated/operations.ts'
  'src/modules/routing/generated/local-views.ts'
  'src/modules/routing/generated/permissions.ts'
  'src/modules/routing/generated/views.ts'
  'src/views/Main/ai/agents/components/AgentContextDialog.vue'
  'src/views/Main/ai/agents/components/AgentKnowledgeDialog/index.vue'
  'src/views/Main/ai/agents/index.vue'
  'src/views/Main/ai/agents/styles.css'
  'src/views/Main/ai/agents/use-agent-admin-page.ts'
  'src/views/Main/ai/chat/components/MessageInput/index.vue'
  'src/views/Main/ai/chat/components/MessageInput/runtime-params-panel.css'
  'src/views/Main/ai/chat/components/MessageInput/runtime-params.ts'
  'src/views/Main/ai/chat/components/MessageInput/RuntimeParamsPanel.vue'
  'src/views/Main/ai/chat/components/MessageList/index.vue'
  'src/views/Main/ai/chat/components/MessageList/MessageCitationDrawer.vue'
  'src/views/Main/ai/chat/composables/types.ts'
  'src/views/Main/ai/chat/use-chat-page.ts'
  'src/views/Main/ai/context/components/ContextDocumentPanel.vue'
  'src/views/Main/ai/context/components/ContextEvaluationPanel.vue'
  'src/views/Main/ai/context/components/ContextProfileDialog.vue'
  'src/views/Main/ai/context/components/ContextProfilePanel.vue'
  'src/views/Main/ai/context/components/ContextSpaceDialog.vue'
  'src/views/Main/ai/context/components/ContextSpacePanel.vue'
  'src/views/Main/ai/context/components/ContextVersionDialog.vue'
  'src/views/Main/ai/context/index.vue'
  'src/views/Main/ai/context/use-context-workspace.ts'
  'src/views/Main/ai/knowledge/components/KnowledgeBaseCard/index.vue'
  'src/views/Main/ai/knowledge/components/KnowledgeBaseFormDialog/index.vue'
  'src/views/Main/ai/knowledge/components/KnowledgeBaseList/index.vue'
  'src/views/Main/ai/knowledge/components/KnowledgeChunkDialog/index.vue'
  'src/views/Main/ai/knowledge/components/KnowledgeDocumentFormDialog/index.vue'
  'src/views/Main/ai/knowledge/components/KnowledgeDocumentHero/index.vue'
  'src/views/Main/ai/knowledge/components/KnowledgeDocumentPanel/index.vue'
  'src/views/Main/ai/knowledge/components/RetrievalTestDialog/index.vue'
  'src/views/Main/ai/knowledge/index.vue'
  'src/views/Main/ai/runs/components/RunList/context-plan.ts'
  'src/views/Main/ai/runs/components/RunList/detail-dialog.ts'
  'src/views/Main/ai/runs/components/RunList/input-snapshot.ts'
  'src/views/Main/ai/runs/components/RunList/presenters.ts'
  'src/views/Main/ai/runs/components/RunList/run-detail-dialog.css'
  'src/views/Main/ai/runs/components/RunList/RunContextPlan.vue'
  'src/views/Main/ai/runs/components/RunList/RunDetailDialog.vue'
  'src/views/Main/ai/runs/components/RunList/RunInputSnapshot.vue'
  'tests/component/ai/AgentContextDialog.test.ts'
  'tests/component/ai/AgentOfficialModelForm.test.ts'
  'tests/component/ai/AgentSystemPromptEditor.test.ts'
  'tests/component/ai/ChatAttachments.test.ts'
  'tests/component/ai/ChatCitations.test.ts'
  'tests/component/ai/ContextDocuments.test.ts'
  'tests/component/ai/ContextEvaluation.test.ts'
  'tests/component/ai/ContextWorkspace.test.ts'
  'tests/component/ai/MessageInteractions.test.ts'
  'tests/component/ai/RunContextPlan.test.ts'
  'tests/component/ai/RunInputSnapshot.test.ts'
  'tests/component/ai/RuntimeParamsPanel.test.ts'
  'tests/component/markdown/MarkdownRenderer.test.ts'
  'tests/integration/features/ai-runs.test.ts'
  'tests/shared/ai/admin-ai-interaction-retirement.test.ts'
  'tests/shared/ai/ai-agent-api.test.ts'
  'tests/shared/ai/ai-agent-billing-config.test.ts'
  'tests/shared/ai/ai-chat-capabilities.test.ts'
  'tests/shared/ai/ai-context-api.test.ts'
  'tests/shared/ai/ai-context-upload.test.ts'
  'tests/shared/ai/ai-run-api.test.ts'
  'tests/shared/ai/ai-run-input-snapshot.test.ts'
  'tests/shared/ai/page-presenters.test.ts'
  'tests/shared/system/upload-client-url.test.ts'
  'tests/unit/contracts/admin-contract.test.ts'
) | Sort-Object -Unique

$frontendCutoverPaths | git add --pathspec-from-file=-
$actualStagedPaths = @(git diff --cached --name-only) | Sort-Object -Unique
$unexpected = Compare-Object $frontendCutoverPaths $actualStagedPaths
if ($unexpected) {
  $unexpected | Format-Table
  throw 'staged frontend paths differ from the exact Tasks 6-9 union'
}
git diff --cached --check
git commit -m "feat(ai): cut over context engineering admin"
```

Every pathspec is one file from the declared Tasks 6-9 union, including exact
deleted files. Never replace this list with a directory, `git add .` or
`git add -A`. A mismatch stops the commit without reverting or staging unrelated
user work.

### Task 10: Verify the atomic cutover package without touching live services

**Files:**
- Modify only if evidence exposes drift: `docs/runbooks/ai-context-index-rebuild.md`
- Create: `docs/runbooks/ai-context-cutover.md`

- [ ] **Step 1: Rebuild both generated contracts from bound commits**

Backend:

```powershell
$manifest = Get-Content -Raw contracts/admin/v1/manifest.json | ConvertFrom-Json
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $manifest.backend_commit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $manifest.backend_commit
git diff --exit-code -- contracts/admin/v1
```

Frontend:

```powershell
npm run contract:check
npm run routes:check
npm run locale:check
git diff --exit-code -- contracts/backend/admin src/modules/http/generated src/modules/routing/generated src/i18n/locales/generated.ts
```

Expected: all commands exit 0 and generated artifacts have zero drift.

- [ ] **Step 2: Run focused backend and frontend suites**

Backend:

```powershell
go test ./internal/module/ai/contextengine/... ./internal/infra/contextindex/... ./internal/infra/documentparser ./internal/infra/ai/... ./internal/module/ai/agent ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/run ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/jobs ./internal/server ./internal/platform/admin ./internal/runtime ./internal/admincontract ./internal/architecture -count=1
pwsh -NoProfile -File scripts/verify-backend.ps1
```

Frontend:

```powershell
npm test -- --run tests/shared/ai/ai-context-api.test.ts tests/shared/ai/ai-context-upload.test.ts tests/shared/system/upload-client-url.test.ts tests/shared/ai/ai-agent-api.test.ts tests/shared/ai/ai-run-api.test.ts tests/component/ai/ContextWorkspace.test.ts tests/component/ai/ContextDocuments.test.ts tests/component/ai/ContextEvaluation.test.ts tests/component/ai/AgentContextDialog.test.ts tests/component/ai/ChatCitations.test.ts tests/component/ai/RunContextPlan.test.ts tests/component/ai/RuntimeParamsPanel.test.ts tests/component/ai/ChatAttachments.test.ts tests/component/ai/ChatStopDelivery.test.ts tests/component/ai/RunInputSnapshot.test.ts
npm run verify:frontend
```

Expected: every command exits 0. Do not substitute a partial test for either repository verification gate.

- [ ] **Step 3: Verify migration and removal evidence**

Run:

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
pwsh -NoProfile -File scripts/tests/ai-context-expand.tests.ps1 -BaselineCommit 6fcc5248a22d3cb4dc4f09e99665ff697be7e6c5
pwsh -NoProfile -File scripts/tests/ai-context-cutover-migrations.tests.ps1
git diff --check
git status --short
```

Expected: Atlas validation and both disposable-MySQL scripts exit 0. The Expand
script proves baseline HCL plus `202608020101` equals the Expand HCL fingerprint;
the cutover script proves sequential `202608020101`/`0102`/`0103`, guarded
non-empty rejection and final HCL equivalence without opening the live database.

Record SHA-256/checksum entries for all three migrations and confirm the Contract file contains six named zero-row assertions before its first DROP. Confirm the workspace contains exactly nine new Context table definitions and no Citation/Retrieval/Hit/Job/Cursor table.

- [ ] **Step 4: Write the deployment runbook with hard stop conditions**

`docs/runbooks/ai-context-cutover.md` records the atomic operational order above, exact commands for stopping the old API first, observing the old Worker drain, stopping the Worker, proving both old containers are stopped, running preflight, verifying the backup/restore artifact, applying migrations in order, and starting the synchronized images. It records expected zero counts, Atlas invocation, image/Contract SHA checks and rollback boundary. A failed preflight stops before any migration/deployment. A failed Contract assertion stops and reports the old table/count. It never suggests manually deleting rows, starting the new binary against a partially migrated schema, or bypassing schema fingerprint checks.

- [ ] **Step 5: Record user-run browser acceptance**

The runbook gives the user this manual checklist after deployment:

```text
/ai/context shows Spaces, Documents, Index Profiles and Evaluation
Agent can save Profile NULL, ready Profile with zero Spaces, and compatible Spaces
supported TXT/Markdown/PDF/DOCX/CSV/XLSX versions show truthful ingestion state
valid Chat Citation opens the persisted source drawer after refresh
invalid Citation has no source mapping
Run detail shows budget, stages, selected/excluded Items and failure outcome
menu/search contains Context Engineering and no Knowledge entry
Chat settings contain no history-count control
```

No Playwright command is added. Browser acceptance is explicitly user-run.

- [ ] **Step 6: Commit documentation and report checkpoints**

```bash
git add -- docs/runbooks/ai-context-cutover.md docs/runbooks/ai-context-index-rebuild.md
git commit -m "docs(ai): add context cutover runbook"
```

Return both repository SHAs, clean statuses, migration checksums, Bundle manifest SHA, full verification results and the pending user browser/deployment steps. Do not claim live cutover until migrations, images and the manual browser checklist have actually run in the target environment.
