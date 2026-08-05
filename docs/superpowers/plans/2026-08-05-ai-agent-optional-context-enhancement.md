# AI Agent 可选上下文增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本计划按用户要求在当前会话串行执行，不分派子代理。

**Goal:** 把现有 Context Engine 的 Embedding、Qdrant、Reranker 和 Conversation Memory 改为可观测、可重试复用的可选增强，使增强依赖故障时主聊天、当前附件、消息持久化、Run 终态和计费仍保持正确。

**Architecture:** 保留现有 MySQL 权威事实、不可变 Context Plan、Prepared Request、Reply Command、Finalizer 和 Provider 状态机，只把上下文明确拆成严格 Core Context 与可显式降级的 Enhancement Context。只有闭合分类的增强错误可以形成 `state=ready + retrieval_outcome=degraded`；权限、快照、当前必需附件、预算、Repository/MySQL 和 Plan 冲突继续沿现有严格失败路径返回。

**Tech Stack:** Go 1.26.5、Gin、GORM/MySQL 8、Atlas 0.38、Qdrant、OpenAI-compatible Embedding/Rerank、Vue 3 `<script setup>`、TypeScript 5.9、Element Plus、AppTable、Vitest、generated Admin OpenAPI Contract。

---

## Delivery Boundaries

```text
Core Context
  system prompt + current message + current attachments + recent complete turns
  + tool continuation + output budget
  -> any failure remains fatal

Enhancement Context
  ready conversation memory + historical attachment/document retrieval
  + embedding + Qdrant + optional rerank
  -> only a closed EnhancementFailure may become ready/degraded
```

- Do not add an Embedding Docker service, Provider table, Agent Scene, second Context state machine, lexical emergency mode, or cross-conversation Saved Memory.
- Do not change current attachment delivery: the current request still sends the COS-backed native attachment directly to the main model.
- Do not rewrite historical terminal Context Plans. The migration only permits new `degraded` rows.
- Do not catch unknown errors as `retrieval_failed`. MySQL, GORM, transaction, authority, permission, snapshot and persistence errors must remain errors.
- Do not put the degradation instruction into `ai_messages`; it is a selected internal `system_instruction` Plan Item only.
- Do not run `admin-dev`, Compose, Playwright, a real migration, or paid Provider probes while executing this plan.
- Run only the focused commands listed below. Full backend/frontend verification and browser acceptance remain user-owned.
- `E:\admin\LONG_TASK_PARALLEL_EXECUTION.md` was absent while this plan was written. Recheck it before execution if it appears; this plan's explicit serial/no-subagent rule still applies unless the user changes it.

## File Responsibility Map

### Backend domain and persistence

- `internal/module/ai/contextengine/types.go`: closed Plan state/outcome invariants.
- `internal/module/ai/contextengine/errors.go`: closed enhancement stage/code classification; no catch-all mapping.
- `internal/module/ai/contextengine/hash.go`: immutable Plan hash, including degraded stage and code but excluding human text.
- `internal/module/ai/contextengine/retrieval.go`: stage-local Embedding/Qdrant/authority/Rerank boundaries and bounded metrics.
- `internal/module/ai/contextengine/runtime_evidence.go`: convert only typed optional failures into degradation candidates.
- `internal/module/ai/contextengine/runtime_memory.go`: `Record + Expected` memory semantics.
- `internal/module/ai/contextengine/runtime_materializer.go`: construct Core groups, valid Memory, retrieval groups, or the degraded internal instruction.
- `internal/module/ai/contextengine/planner.go`: terminal Plan reuse, packing, immutable hash and persistence.
- `internal/module/ai/contextengine/runtime.go`: reuse an existing terminal Plan before any enhancement call and emit bounded telemetry.
- `internal/module/ai/contextengine/repository.go`: Plan round-trip plus enabled typed model options for Context Page Init.
- `database/schema/admin.hcl`: canonical CHECK definitions.
- `database/migrations/202608050101_ai_context_optional_enhancement.sql`: forward-only CHECK replacement.
- `database/migrations/atlas.sum`: Atlas directory checksum only; no live apply.

### Backend API, projection and operations

- `internal/module/ai/message/context_projection.go`: refresh-stable `degraded` message context with no fabricated source.
- `internal/module/ai/run/repository.go` and `internal/module/ai/run/service.go`: Run Detail Context Plan and `diagnostic_codes` projection.
- `internal/module/ai/contextengine/citation.go`: closed degraded Run projection and empty Citation behavior.
- `internal/runtime/context_readiness.go`: API Qdrant failure is degraded; Worker Qdrant failure remains down.
- `internal/telemetry/redact.go` and `internal/telemetry/prometheus.go`: allow one closed low-cardinality `context.stage` label.
- `internal/module/ai/contextengine/admin_dto.go`, `admin_service.go`, `repository.go`, `transport/admin/{route,handler}.go`: `/api/admin/v1/ai/context/page-init`.
- `internal/module/ai/provider/{dto,service}.go` and `transport/admin/request.go`: structured Provider models plus statuses, with legacy Chat-only compatibility.
- `internal/admincontract/openapi_models.go`: stop field-level `required` inference at `dive`.
- `internal/admincontract/openapi_ai_schemas.go`: publish `degraded` as a closed Context outcome.

### Frontend

- `src/api/ai/providers.ts`: submit structured `models[]`, display names and statuses.
- `src/views/Main/ai/providers/composables/useProviderForm.ts`: one typed model-row source of truth.
- `src/views/Main/ai/providers/components/ProviderModelEditor.vue`: AppTable-backed Model ID/kind/name/status editor.
- `src/views/Main/ai/providers/components/ProviderFormDialog.vue`: fetch candidates without guessing their kind.
- `src/views/Main/ai/providers/components/ProviderModelList.vue`: compact typed summary without local `:deep` overrides.
- `src/api/ai/context.ts` and `src/views/Main/ai/context/use-context-workspace.ts`: load typed Context model options once.
- `src/views/Main/ai/context/components/ContextProfileDialog.vue`: typed model selectors instead of database ID inputs.
- `src/views/Main/ai/chat/components/MessageList/index.vue`: refresh-stable non-blocking degradation status.
- `src/views/Main/ai/runs/components/RunList/RunContextPlan.vue`: AppTable Plan items plus stable degradation stage/code.
- `src/i18n/locales/{zh-CN,en-US}/ai*.ts`: visible labels and status text.

## Commit and Contract Order

```text
backend Tasks 1-7 source commits
  -> record full backend source SHA
  -> Task 8 generates and commits backend Admin Contract Bundle
  -> frontend Task 9 syncs exactly the manifest backend SHA and regenerates types
  -> frontend Tasks 9-10 commits
```

No backend source file may change between recording the source SHA and generating the Bundle. Backend and frontend commits remain independent.

### Task 1: Persist the closed `ready + degraded` Plan shape

**Files:**
- Modify: `internal/module/ai/contextengine/types.go`
- Modify: `internal/module/ai/contextengine/types_test.go`
- Modify: `internal/module/ai/contextengine/hash.go`
- Modify: `internal/module/ai/contextengine/hash_test.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`
- Modify: `internal/module/ai/contextengine/repository_mysql_test.go`
- Modify: `internal/architecture/ai_context_schema_contract_test.go`
- Modify: `database/schema/admin.hcl`
- Create: `database/migrations/202608050101_ai_context_optional_enhancement.sql`
- Modify: `database/migrations/atlas.sum`

- [ ] **Step 1: Write failing domain, hash and repository tests**

Extend the existing `validReadyPlan()` fixture rather than creating a second complete Plan fixture. The tests must assert this exact terminal matrix:

```go
func degradedReadyPlan(t *testing.T) ContextPlan {
	plan := validReadyPlan()
	diagnostic, err := NewPlanError("embedding", ErrCodeEmbeddingFailed)
	if err != nil {
		t.Fatal(err)
	}
	plan.RetrievalOutcome = RetrievalDegraded
	plan.Error = &diagnostic
	plan.PlanSHA256 = nil
	hash, err := HashPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanSHA256 = &hash
	return plan
}
```

Required cases:

```text
ready/degraded + non-zero plan_sha256 + stage/code + items -> valid
ready/degraded without error                              -> invalid
ready/degraded without plan_sha256                        -> invalid
ready/degraded with history_attachment/document_evidence/recalled_turn or citation -> invalid
ready/skipped|no_hit|hit with error                        -> invalid
failed/failed with items or plan_sha256                    -> invalid
changing error stage or code changes HashPlan
changing only error message or metrics does not change HashPlan
repository round-trip preserves ready/degraded and Error
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'RetrievalDegraded|DegradedPlan|HashPlan|Persist.*Degraded' -count=1
```

Expected: FAIL because `RetrievalDegraded` is undefined and current ready Plan validation rejects every error.

- [ ] **Step 3: Implement one Plan invariant instead of scattered exceptions**

Add the closed value and keep state validation centralized:

```go
const (
	RetrievalSkipped  RetrievalOutcome = "skipped"
	RetrievalNoHit    RetrievalOutcome = "no_hit"
	RetrievalHit      RetrievalOutcome = "hit"
	RetrievalDegraded RetrievalOutcome = "degraded"
	RetrievalFailed   RetrievalOutcome = "failed"
)
```

For `PlanReady`, require a hash and at least one Item. Normal outcomes require `Error == nil`; `RetrievalDegraded` requires a valid `PlanError` and rejects selected `history_attachment`, `document_evidence`, `recalled_turn`, any Citation key, and any block carrying retrieval metadata. `PlanFailed` remains `failed/failed`, hashless and itemless. `current_attachment` remains legal and is not covered by this rejection.

Extend `contextPlanCanonicalV1` with only stable diagnostics:

```go
ErrorStage *string    `json:"error_stage,omitempty"`
ErrorCode  *ErrorCode `json:"error_code,omitempty"`
```

Populate them only when `plan.Error != nil`. Never hash `Error.Message` or timings.

- [ ] **Step 4: Replace the two MySQL CHECK constraints forward-only**

The new migration contains no data update and no table/column addition:

```sql
ALTER TABLE `ai_context_plans`
  DROP CHECK `chk_ai_context_plans_retrieval_outcome`,
  DROP CHECK `chk_ai_context_plans_terminal_shape`,
  ADD CONSTRAINT `chk_ai_context_plans_retrieval_outcome`
    CHECK (`retrieval_outcome` IN ('skipped', 'no_hit', 'hit', 'degraded', 'failed')),
  ADD CONSTRAINT `chk_ai_context_plans_terminal_shape`
    CHECK (
      (`state` = 'ready' AND `plan_sha256` IS NOT NULL AND (
        (`retrieval_outcome` IN ('skipped', 'no_hit', 'hit')
          AND `error_stage` IS NULL AND `error_code` IS NULL AND `error_message` IS NULL)
        OR
        (`retrieval_outcome` = 'degraded'
          AND `error_stage` IS NOT NULL AND CHAR_LENGTH(`error_stage`) > 0
          AND `error_code` IS NOT NULL AND CHAR_LENGTH(`error_code`) > 0
          AND (`error_message` IS NULL OR CHAR_LENGTH(`error_message`) > 0))
      ))
      OR
      (`state` = 'failed' AND `plan_sha256` IS NULL AND `retrieval_outcome` = 'failed'
        AND `error_stage` IS NOT NULL AND CHAR_LENGTH(`error_stage`) > 0
        AND `error_code` IS NOT NULL AND CHAR_LENGTH(`error_code`) > 0
        AND (`error_message` IS NULL OR CHAR_LENGTH(`error_message`) > 0))
    );
```

Mirror the exact expressions in `admin.hcl`. Update the architecture schema test to require `degraded` and the three terminal branches.

- [ ] **Step 5: Recalculate the migration checksum without touching MySQL**

First prefer an already-installed exact Atlas binary:

```powershell
atlas version
atlas migrate hash --dir file://database/migrations
```

Expected: Atlas reports `v0.38.0`; `atlas.sum` changes for the new migration and old migration files remain byte-identical. If that binary is unavailable, do not install tools or start Docker; leave this repository-standard user-owned command for the handoff:

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
```

Do not run `migrate apply`, `schema apply`, `admin-dev`, or a MySQL command.

- [ ] **Step 6: Verify GREEN and commit**

Run:

```powershell
go test ./internal/module/ai/contextengine ./internal/architecture -run 'RetrievalDegraded|DegradedPlan|HashPlan|Persist.*Degraded|ContextSchema' -count=1
git diff --check
```

Expected: PASS; the migration changes only CHECK acceptance and does not rewrite historical rows.

```powershell
git add -- database/schema/admin.hcl database/migrations/202608050101_ai_context_optional_enhancement.sql database/migrations/atlas.sum internal/architecture/ai_context_schema_contract_test.go internal/module/ai/contextengine/types.go internal/module/ai/contextengine/types_test.go internal/module/ai/contextengine/hash.go internal/module/ai/contextengine/hash_test.go internal/module/ai/contextengine/repository_test.go internal/module/ai/contextengine/repository_mysql_test.go
git commit -m "feat(ai): persist degraded context plans"
```

### Task 2: Close enhancement failure classification

**Files:**
- Modify: `internal/module/ai/contextengine/errors.go`
- Modify: `internal/module/ai/contextengine/types_test.go`
- Modify: `internal/module/ai/contextengine/retrieval.go`
- Modify: `internal/module/ai/contextengine/retrieval_test.go`
- Modify: `internal/module/ai/contextengine/runtime_evidence.go`
- Modify: `internal/module/ai/contextengine/runtime_evidence_test.go`
- Modify: `internal/module/ai/contextengine/runtime_authority_test.go`

- [ ] **Step 1: Write failing tests for typed and untyped failures**

Use distinct sentinel errors for every boundary and assert:

```text
Embedding client error                   -> enhancement/embedding_failed
Qdrant QueryBatch transport/shape error  -> enhancement/retrieval_failed or index_inconsistent
Reranker error                           -> enhancement/rerank_failed
disabled/invalid Profile already loaded  -> enhancement/profile_unavailable
failed/inconsistent index state          -> enhancement/index_failed or index_inconsistent
NewestComplete repository error          -> original error
CandidateAuthorityReader MySQL error      -> original error
Plan repository/transaction error         -> original error
permission/snapshot/attachment error      -> original strict error
unknown error                             -> original error
```

Tests must compare `errors.Is` against the repository sentinel so an accidental wrapper into `ai.context.retrieval_failed` cannot pass.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'EnhancementFailure|RetrievalClassification|Authority.*NotDegraded|Unknown.*NotDegraded' -count=1
```

Expected: FAIL because `retrievalErrorCode` currently maps every unmatched error to `retrieval_failed`.

- [ ] **Step 3: Define the closed enhancement failure data structure**

Add one typed wrapper in `errors.go`:

```go
type EnhancementStage string

const (
	EnhancementStageProfile   EnhancementStage = "profile"
	EnhancementStageMemory    EnhancementStage = "memory"
	EnhancementStageEmbedding EnhancementStage = "embedding"
	EnhancementStageIndex     EnhancementStage = "index"
	EnhancementStageRetrieval EnhancementStage = "retrieval"
	EnhancementStageRerank    EnhancementStage = "rerank"
)

type EnhancementFailure struct {
	Stage EnhancementStage
	Code  ErrorCode
	Cause error
}
```

Implement `Error()`, `Unwrap()`, a validating constructor and `AsEnhancementFailure(error)`. The constructor accepts only these pairs:

```go
func NewEnhancementFailure(stage EnhancementStage, code ErrorCode, cause error) error
func AsEnhancementFailure(err error) (EnhancementFailure, bool)
```

The constructor returns the validated wrapper as `error`; an invalid stage/code pair returns `ErrInvalidContextValue` instead of constructing a degradable error. This keeps boundary call sites single-return and makes the examples below compile without a second unchecked result.

```text
profile   -> profile_unavailable
memory    -> memory_unavailable
embedding -> embedding_failed
index     -> index_failed | index_inconsistent
retrieval -> retrieval_failed
rerank    -> rerank_failed
```

It rejects permission, snapshot, attachment, overflow and Plan conflict codes. The persisted `PlanError` is built from `Stage` and `Code`; `Cause` is never persisted, exposed, used as a metric label, or hashed.

- [ ] **Step 4: Classify at the owning boundary and delete the default mapper**

`Retrieve` classifies only operations it owns:

```go
embedding, err := dependencies.Embedding.Embed(ctx, texts)
if err != nil {
	return result, NewEnhancementFailure(EnhancementStageEmbedding, ErrCodeEmbeddingFailed, err)
}

batch, err := dependencies.Querier.QueryBatch(ctx, query)
if err != nil {
	return result, NewEnhancementFailure(EnhancementStageRetrieval, ErrCodeRetrievalFailed, err)
}

verification, err := dependencies.Authority.VerifyCandidates(ctx, input.Authority, candidates)
if err != nil {
	return result, err
}
```

Malformed Qdrant candidate identity/branch evidence becomes `index_inconsistent`. A configured Reranker failure becomes `rerank_failed`. Profile state/config failures are classified only after the Profile row was successfully loaded. Delete `retrievalErrorCode`; no default branch may manufacture a degradable code.

- [ ] **Step 5: Verify GREEN and commit**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'EnhancementFailure|Retrieval|Rerank|Authority|RuntimeEvidence' -count=1
git diff --check
```

Expected: PASS; authority and unknown failures remain distinguishable from optional dependency failures.

```powershell
git add -- internal/module/ai/contextengine/errors.go internal/module/ai/contextengine/types_test.go internal/module/ai/contextengine/retrieval.go internal/module/ai/contextengine/retrieval_test.go internal/module/ai/contextengine/runtime_evidence.go internal/module/ai/contextengine/runtime_evidence_test.go internal/module/ai/contextengine/runtime_authority_test.go
git commit -m "refactor(ai): classify optional context failures"
```

### Task 3: Make Conversation Memory absence explicit

**Files:**
- Modify: `internal/module/ai/contextengine/runtime_memory.go`
- Create: `internal/module/ai/contextengine/runtime_memory_test.go`
- Modify: `internal/module/ai/contextengine/runtime_materializer.go`
- Modify: `internal/module/ai/contextengine/planner_test.go`
- Modify: `internal/module/ai/contextengine/memory_reconciler_repository.go`
- Modify: `internal/module/ai/contextengine/memory_test.go`

- [ ] **Step 1: Write failing Memory availability tests**

Cover four different facts instead of treating all `nil` values alike:

```text
Profile has no Memory model                                      -> Record=nil, Expected=false
uncovered complete turns do not cross MemoryWindow               -> Record=nil, Expected=false
window says Memory should exist but no valid Ready record exists -> Record=nil, Expected=true
valid independently verified old Ready Memory exists             -> Record=memory, Expected=true
Memory Repository returns an error                               -> exact error, no Plan
```

Also assert that a newer failed background Memory job does not hide a still-valid older Ready Memory and that an invalid hash/profile/range on a returned record is `Expected=true`, not a silent `nil`.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'RuntimeMemory|MemoryExpected|MemoryRepositoryError|OldReadyMemory' -count=1
```

Expected: FAIL because `PlanMaterializer.Materialize` currently discards `memoryErr` and cannot distinguish normal absence from unavailable derived state.

- [ ] **Step 3: Return `Record + Expected` as one fact**

Define:

```go
type RuntimeMemoryContext struct {
	Record   *MemoryRecord
	Expected bool
}
```

Extract the complete-turn token threshold calculation already used by `selectMemoryPrefix` so both the Memory reconciler and runtime use `MemoryWindow`; do not create a second percentage policy. The runtime input excludes the current incomplete turn and applies the last valid Ready Memory boundary before calculating uncovered complete turns.

`latestMemory` returns Repository errors unchanged. A record is usable only when conversation, Profile ID/hash, Ready state, summary hash, source hash and message range all validate. A missing/invalid record above the threshold returns `Expected=true` with no record.

- [ ] **Step 4: Materialize the three legal outcomes**

Use this branch structure:

```go
memoryContext, err := materializer.runtimeMemory(ctx, input, facts)
if err != nil {
	return BuildPlanInput{}, err
}
if memoryContext.Record != nil {
	// append the independently valid Ready Memory and use its boundary
}
if memoryContext.Expected && memoryContext.Record == nil {
	return materializer.degradedInput(output, EnhancementStageMemory, ErrCodeMemoryUnavailable)
}
```

`memory_unavailable` stops the enhancement pipeline before Embedding, Qdrant and Rerank. Below-threshold absence remains normal and retrieval may continue. Repository errors never enter `degradedInput`.

- [ ] **Step 5: Verify GREEN and commit**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'RuntimeMemory|MemoryWindow|MemoryExpected|PlanMaterializer' -count=1
git diff --check
```

Expected: PASS with a counting Embedding resolver proving zero calls after `memory_unavailable`.

```powershell
git add -- internal/module/ai/contextengine/runtime_memory.go internal/module/ai/contextengine/runtime_memory_test.go internal/module/ai/contextengine/runtime_materializer.go internal/module/ai/contextengine/planner_test.go internal/module/ai/contextengine/memory_reconciler_repository.go internal/module/ai/contextengine/memory_test.go
git commit -m "fix(ai): preserve memory availability semantics"
```

### Task 4: Reuse terminal Plans and build immutable degraded requests

**Files:**
- Modify: `internal/module/ai/contextengine/runtime.go`
- Create: `internal/module/ai/contextengine/runtime_test.go`
- Modify: `internal/module/ai/contextengine/runtime_materializer.go`
- Modify: `internal/module/ai/contextengine/runtime_evidence.go`
- Modify: `internal/module/ai/contextengine/planner.go`
- Modify: `internal/module/ai/contextengine/planner_test.go`
- Modify: `internal/module/ai/contextengine/citation_test.go`

- [ ] **Step 1: Write failing idempotency and degraded packing tests**

Tests must prove:

```text
existing ready/hit Plan      -> no materializer, Embedding or Qdrant call
existing ready/degraded Plan -> no materializer, Embedding or Qdrant call
new typed enhancement error  -> persisted ready/degraded Plan and RuntimeResult
new unknown error            -> no Plan and no Provider dispatch evidence
degraded Plan                -> Core + valid Ready Memory + internal instruction only
degraded Plan                -> zero history attachment/document/recalled evidence and zero Citation
current attachment block     -> remains selected and compiled into main ChatInput
Provider retry               -> reuses identical Plan ID/hash and Prepared Request input
```

Use counting fakes; do not use sleeps or network calls.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'TerminalPlanReuse|DegradedRuntime|DegradedPacking|CurrentAttachment.*Degraded' -count=1
```

Expected: FAIL because `RuntimeService.BuildPlan` materializes before `Planner.BuildPlan` checks for an existing terminal Plan, and any `Failure` currently creates `PlanFailed`.

- [ ] **Step 3: Move terminal lookup before materialization**

Expose the existing repository lookup through `Planner`:

```go
func (planner *Planner) FindTerminalByRunID(ctx context.Context, runID uint64) (*ContextPlan, error) {
	if planner == nil || planner.repository == nil || runID == 0 {
		return nil, ErrPlanRepositoryNotConfigured
	}
	return planner.repository.FindTerminalByRunID(ctx, runID)
}
```

`RuntimeService.BuildPlan` first calls it and immediately returns `RuntimeResultFromPlan` for a terminal row. `Planner.BuildPlan` retains its own lookup to close the concurrent creation race.

- [ ] **Step 4: Represent degraded diagnostics separately from fatal Plan failure**

Replace the ambiguous materializer `Failure` field with `Diagnostic *PlanError`. Enforce:

```text
RetrievalDegraded <=> Diagnostic != nil
RetrievalSkipped|NoHit|Hit => Diagnostic == nil
RetrievalFailed is created only by a strict Planner failure path
```

When `AsEnhancementFailure(err)` succeeds, create a stable diagnostic and discard the resolver's retrieval groups. Unknown errors return immediately.

- [ ] **Step 5: Add one required internal degradation instruction**

Create a deterministic `BlockSystemInstruction` with a stable source identity and this exact semantic content:

```text
Context enhancement is unavailable for this request. Use only the supplied core context and any explicitly supplied ready conversation memory. Do not claim that space documents or historical attachment retrieval was consulted. Do not emit [C<number>] citations.
```

The block is `Required=true`, is hashed and persisted as a Plan Item, and is compiled as a system message. It is never appended to `RuntimeInput.Messages`, `ai_messages`, Assistant content or the next turn's raw history.

The Planner still runs normal Pack and authority guard logic. If Core plus the required instruction cannot fit, existing `required_overflow` remains fatal.

- [ ] **Step 6: Verify GREEN and commit**

Run:

```powershell
go test ./internal/module/ai/contextengine -run 'Terminal|Degraded|CompileChatInput|Citation|Attachment|PlanMaterializer' -count=1
git diff --check
```

Expected: PASS; a degraded plan has a non-zero hash and Provider dispatch evidence, while an unknown/MySQL error has neither.

```powershell
git add -- internal/module/ai/contextengine/runtime.go internal/module/ai/contextengine/runtime_test.go internal/module/ai/contextengine/runtime_materializer.go internal/module/ai/contextengine/runtime_evidence.go internal/module/ai/contextengine/planner.go internal/module/ai/contextengine/planner_test.go internal/module/ai/contextengine/citation_test.go
git commit -m "feat(ai): continue chat with degraded context"
```

### Task 5: Project and observe degradation without changing Run truth

**Files:**
- Modify: `internal/module/ai/message/context_projection.go`
- Modify: `internal/module/ai/message/repository_test.go`
- Modify: `internal/module/ai/contextengine/citation.go`
- Modify: `internal/module/ai/contextengine/retrieval.go`
- Modify: `internal/module/ai/contextengine/retrieval_test.go`
- Modify: `internal/module/ai/contextengine/runtime_evidence.go`
- Modify: `internal/module/ai/contextengine/runtime_evidence_test.go`
- Modify: `internal/module/ai/contextengine/runtime_materializer.go`
- Modify: `internal/module/ai/contextengine/planner.go`
- Modify: `internal/module/ai/contextengine/planner_test.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/module/ai/run/context_projection_test.go`
- Modify: `internal/runtime/context_readiness.go`
- Modify: `internal/runtime/context_readiness_test.go`
- Create: `internal/module/ai/contextengine/runtime_telemetry.go`
- Create: `internal/module/ai/contextengine/runtime_telemetry_test.go`
- Modify: `internal/module/ai/contextengine/runtime.go`
- Modify: `internal/module/ai/contextengine/runtime_build.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/telemetry/redact.go`
- Modify: `internal/telemetry/redact_test.go`
- Modify: `internal/telemetry/prometheus.go`
- Modify: `internal/telemetry/prometheus_test.go`
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_test.go`
- Modify: `docs/architecture.md`
- Modify: `docs/contracts/admin-v1-workflow-contracts.md`

- [ ] **Step 1: Write failing projection, readiness and telemetry tests**

Required assertions:

```text
message refresh projects context.outcome=degraded with sources=[]
assistant text containing [C1] during degradation does not create a source
Run Detail projects ready/degraded, stage and code
Run Detail diagnostic_codes contains the context error code once
successful Run status and billing fields remain unchanged
API ContextReadiness Qdrant failure -> degraded even with active collections
Worker ContextReadiness Qdrant failure -> down
telemetry accepts only closed context.stage values
telemetry drops query, filename, user/run/conversation IDs and raw Provider error
typed retrieval failure preserves bounded stage metrics while discarding every partial evidence group
existing terminal Plan reuse emits no new Embedding request metric
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/message ./internal/module/ai/run ./internal/runtime ./internal/telemetry ./internal/admincontract -run 'Context.*Degraded|Degraded.*Context|ContextReadiness|ContextStage' -count=1
```

Expected: FAIL because the OpenAPI enum and readiness policy do not accept the new semantics and Run diagnostics omit the Context code.

- [ ] **Step 3: Preserve one persisted projection source**

Keep `MessageContext.Outcome` as the source for the chat status. `projectMessageContext` accepts the closed `degraded` outcome only for a ready Plan and returns empty sources/invalid keys without inventing a document.

`ProjectContextPlan` already exposes `Error`; retain that shape. When Run Detail loads a degraded Plan, append `plan.Error.Code` to the existing `diagnostic_codes` with stable deduplication. Do not overwrite Provider `error_code`, Run status, billing status, billing reason or finalizer output.

Update `AIContextPlan.retrieval_outcome` to:

```go
stringEnumSchema("skipped", "no_hit", "hit", "degraded", "failed")
```

- [ ] **Step 4: Separate API and Worker readiness policy**

Keep source-MySQL read failure as `down`. For a Qdrant readiness failure:

```go
if checker.required {
	return readiness.Check{Status: readiness.StatusDown, Message: "context index is unavailable"}
}
return readiness.Check{Status: readiness.StatusDegraded, Message: "context index is unavailable"}
```

The number of active collections must not change API availability. `NewWorkerContextReadiness` remains the only constructor setting `required=true`.

- [ ] **Step 5: Emit bounded Context telemetry through the existing recorder**

Add `context.stage` to the sanitizer allowlist and Prometheus label list. Validate stage values against the same closed `EnhancementStage` set before recording.

Use `ContextPlanMetricsV1` as the only metrics carrier; do not create a parallel metrics DTO:

```go
type RetrievalResult struct {
	Outcome    RetrievalOutcome
	Candidates []VerifiedCandidate
	Excluded   []CandidateExclusion
	Cleanup    []contextindex.PointRef
	Metrics    ContextPlanMetricsV1
}

type RuntimeEvidence struct {
	Outcome    RetrievalOutcome
	Groups     []PackGroup
	Diagnostic *PlanError
	Metrics    ContextPlanMetricsV1
}

type BuildPlanInput struct {
	// existing immutable input fields
	Metrics ContextPlanMetricsV1
}
```

`Retrieve` initializes the schema and returns its partial metrics on both success and error. `RetrievalEvidenceResolver` forwards those metrics even when it returns a typed `EnhancementFailure`; `PlanMaterializer` discards partial Groups but preserves Metrics; `Planner` copies Metrics into the Plan and adds `PackingMS`. Add an injectable `Now func() time.Time` to `RetrievalDependencies` and `PlannerDependencies`, defaulting to `time.Now`, so tests use deterministic elapsed values without sleeps.

Measure only owned boundaries: Embedding client time/request count, Qdrant query time, MySQL authority time, Rerank time/request count, candidate count and Pack time. Populate token counters only from validated Provider usage. Metrics never participate in `HashPlan`.

Pass `resources.Telemetry` through `RuntimeDependencies`. After the current call has materialized input and obtained its terminal Plan, emit existing generic telemetry events using these metric names:

```text
context_plan_total             outcome, error.code, context.stage
context_degraded_total         error.code, context.stage
context_plan_duration_seconds  context.stage
context_embedding_requests_total outcome
```

Only a call that actually ran the materializer emits execution counters and histograms. The early terminal-Plan reuse path returns before telemetry so Provider retry cannot replay a persisted Embedding request count. Label values are only closed outcome/stage/code strings; Query text, IDs, filenames, URLs and raw causes never reach the recorder.

- [ ] **Step 6: Update architecture truth and verify GREEN**

Replace the old statement that ready Plans can only be `skipped/no_hit/hit`. Document Core versus Enhancement behavior, strict error boundaries, terminal reuse, API/Worker readiness and refresh-stable degraded projection.

Run:

```powershell
go test ./internal/module/ai/message ./internal/module/ai/run ./internal/runtime ./internal/telemetry ./internal/admincontract -run 'Context|Degraded|Prometheus|Redact' -count=1
git diff --check
```

Expected: PASS; no test changes the existing Run or billing terminal enum.

- [ ] **Step 7: Commit**

```powershell
git add -- internal/module/ai/message/context_projection.go internal/module/ai/message/repository_test.go internal/module/ai/contextengine/citation.go internal/module/ai/contextengine/retrieval.go internal/module/ai/contextengine/retrieval_test.go internal/module/ai/contextengine/runtime_evidence.go internal/module/ai/contextengine/runtime_evidence_test.go internal/module/ai/contextengine/runtime_materializer.go internal/module/ai/contextengine/planner.go internal/module/ai/contextengine/planner_test.go internal/module/ai/run/repository.go internal/module/ai/run/repository_test.go internal/module/ai/run/service.go internal/module/ai/run/service_test.go internal/module/ai/run/context_projection_test.go internal/runtime/context_readiness.go internal/runtime/context_readiness_test.go internal/module/ai/contextengine/runtime_telemetry.go internal/module/ai/contextengine/runtime_telemetry_test.go internal/module/ai/contextengine/runtime.go internal/module/ai/contextengine/runtime_build.go internal/platform/admin/build.go internal/telemetry/redact.go internal/telemetry/redact_test.go internal/telemetry/prometheus.go internal/telemetry/prometheus_test.go internal/admincontract/openapi_ai_schemas.go internal/admincontract/openapi_test.go docs/architecture.md docs/contracts/admin-v1-workflow-contracts.md
git commit -m "feat(ai): expose context degradation diagnostics"
```

### Task 6: Publish typed Context model options

**Files:**
- Modify: `internal/module/ai/contextengine/admin_dto.go`
- Modify: `internal/module/ai/contextengine/admin_service.go`
- Modify: `internal/module/ai/contextengine/admin_service_test.go`
- Modify: `internal/module/ai/contextengine/repository.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`
- Modify: `internal/module/ai/contextengine/transport/admin/route.go`
- Modify: `internal/module/ai/contextengine/transport/admin/handler.go`
- Modify: `internal/module/ai/contextengine/transport/admin/handler_test.go`
- Modify: `internal/admincontract/openapi_test.go`

- [ ] **Step 1: Write failing repository, service and route tests**

The repository test supplies enabled/disabled Providers and Chat/Embedding/Rerank models. Assert one ordered query returns only enabled models on enabled, non-deleted Providers. The service partitions exactly:

```text
embedding_model_options -> model_kind=embedding
reranker_model_options  -> model_kind=rerank
memory_model_options    -> model_kind=chat
```

The route test requires `GET /api/admin/v1/ai/context/page-init`, operation ID `ai_context_page_init`, `ai_context_view` access and no audit payload.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/contextengine ./internal/module/ai/contextengine/transport/admin ./internal/admincontract -run 'ContextPageInit|ProviderModelOptions' -count=1
```

Expected: FAIL because the repository method, DTO and route do not exist.

- [ ] **Step 3: Add one option DTO and one repository query**

Define:

```go
type ProviderModelOptionDTO struct {
	Value        uint64 `json:"value"`
	Label        string `json:"label"`
	ProviderName string `json:"provider_name"`
	ModelID      string `json:"model_id"`
}

type ContextPageInitResponse struct {
	EmbeddingModelOptions []ProviderModelOptionDTO `json:"embedding_model_options"`
	RerankerModelOptions  []ProviderModelOptionDTO `json:"reranker_model_options"`
	MemoryModelOptions    []ProviderModelOptionDTO `json:"memory_model_options"`
}
```

The label is deterministically `provider_name + " / " + display_name-or-model_id`. The repository row additionally carries `ModelKind` for partitioning but that field is not exposed in each already-typed array. No fallback option is fabricated for a missing model.

- [ ] **Step 4: Add the GET handler through the normal route chain**

Extend the existing `transport/admin.adminReadService` with `PageInit(context.Context) (*ContextPageInitResponse, *apperror.Error)`. The handler follows the same read-service type assertion as the other admin reads, calls only the service, and returns `response.Success`; it does not query GORM. Register the route before parameterized Context routes to keep the contract unambiguous.

- [ ] **Step 5: Verify GREEN and commit**

Run:

```powershell
go test ./internal/module/ai/contextengine ./internal/module/ai/contextengine/transport/admin ./internal/admincontract -run 'ContextPageInit|ProviderModelOptions|Context.*Route' -count=1
git diff --check
```

Expected: PASS; Agent option endpoints remain Chat-only and unchanged.

```powershell
git add -- internal/module/ai/contextengine/admin_dto.go internal/module/ai/contextengine/admin_service.go internal/module/ai/contextengine/admin_service_test.go internal/module/ai/contextengine/repository.go internal/module/ai/contextengine/repository_test.go internal/module/ai/contextengine/transport/admin/route.go internal/module/ai/contextengine/transport/admin/handler.go internal/module/ai/contextengine/transport/admin/handler_test.go internal/admincontract/openapi_test.go
git commit -m "feat(ai): publish context model options"
```

### Task 7: Preserve typed Provider models through every mutation

**Files:**
- Modify: `internal/module/ai/provider/dto.go`
- Modify: `internal/module/ai/provider/service.go`
- Modify: `internal/module/ai/provider/service_test.go`
- Modify: `internal/module/ai/provider/repository_gorm_test.go`
- Modify: `internal/module/ai/provider/transport/admin/request.go`
- Modify: `internal/module/ai/provider/transport/admin/handler.go`
- Modify: `internal/module/ai/provider/transport/admin/handler_test.go`
- Modify: `internal/admincontract/openapi_models.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `internal/admincontract/openapi_test.go`

- [ ] **Step 1: Write failing compatibility and OpenAPI tests**

Required cases:

```text
create/update with models[] + statuses saves chat/embedding/rerank and exact status
model_ids and models together is rejected
legacy model_ids reconciles Chat rows only
remote /models sync reconciles Chat rows only
remote /models sync preserves existing Embedding/Rerank kind, status and identity
ProviderModelDTO publishes model_kind
Agent model options exclude Embedding/Rerank
OpenAPI marks neither model_ids nor models required at the parent request level
OpenAPI requires model_id/model_kind inside each models[] element
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
go test ./internal/module/ai/provider ./internal/module/ai/provider/transport/admin ./internal/admincontract -run 'TypedProviderModels|LegacyModelIDs|SyncModels|ModelFieldRequired|ProviderModelKind' -count=1
```

Expected: FAIL because primary mutation requests do not carry `statuses`, and OpenAPI field inference scans element validators after `dive` as if they applied to the slice field.

- [ ] **Step 3: Carry statuses through the existing catalog normalizer**

Add `Statuses map[string]int` to `CreateInput`, `mutationRequest` and handler conversion. Feed it into the existing `buildModelCatalog` used by Create/Update. Do not add status to `ProviderModelInput`; the existing `statuses[model_id]` map remains the one transport representation.

The primary contract remains:

```json
{
  "models": [
    { "model_id": "gpt-5.6", "model_kind": "chat" },
    { "model_id": "Qwen/Qwen3-Embedding-0.6B", "model_kind": "embedding" }
  ],
  "model_display_names": {
    "gpt-5.6": "GPT-5.6",
    "Qwen/Qwen3-Embedding-0.6B": "Qwen3 Embedding 0.6B"
  },
  "statuses": {
    "gpt-5.6": 1,
    "Qwen/Qwen3-Embedding-0.6B": 1
  }
}
```

Legacy `model_ids` still produces Chat models and `ModelReconcileChatOnly`. Structured `models` uses `ModelReconcileAll`. Keep the existing remote sync explicitly Chat-only.

- [ ] **Step 4: Stop OpenAPI parent validation at `dive`**

Change `modelFieldRequired` to stop scanning when it reaches an element boundary:

```go
for _, token := range validation {
	if token == "dive" {
		break
	}
	if token == "required" {
		return true
	}
}
```

Nested `ProviderModelInput` fields remain required because their own struct tags are compiled separately.

- [ ] **Step 5: Verify GREEN and commit the final backend source revision**

Run:

```powershell
go test ./internal/module/ai/provider ./internal/module/ai/provider/transport/admin ./internal/admincontract -run 'Provider|Model|OpenAPI' -count=1
git diff --check
```

Expected: PASS; `/models` synchronization cannot disable or retype explicit Embedding/Rerank rows.

```powershell
git add -- internal/module/ai/provider/dto.go internal/module/ai/provider/service.go internal/module/ai/provider/service_test.go internal/module/ai/provider/repository_gorm_test.go internal/module/ai/provider/transport/admin/request.go internal/module/ai/provider/transport/admin/handler.go internal/module/ai/provider/transport/admin/handler_test.go internal/admincontract/openapi_models.go internal/admincontract/openapi_models_test.go internal/admincontract/openapi_test.go
git commit -m "fix(ai): preserve typed provider models"
```

### Task 8: Freeze and publish the backend Admin Contract

**Files:**
- Modify generated: `contracts/admin/v1/openapi.json`
- Modify generated: `contracts/admin/v1/manifest.json`
- Modify only if generator changes them: `contracts/admin/v1/permissions.json`
- Modify only if generator changes them: `contracts/admin/v1/views.json`
- Modify only if generator changes them: `contracts/admin/v1/realtime/envelope.schema.json`
- Modify only if generator changes them: `contracts/admin/v1/realtime/events.schema.json`

- [ ] **Step 1: Prove the backend source checkout is committed and clean**

Run:

```powershell
git status --short --branch
$backendSourceCommit = (git rev-parse HEAD).Trim().ToLowerInvariant()
if ($backendSourceCommit -notmatch '^[0-9a-f]{40}$') { throw 'backend source SHA is invalid' }
```

Expected: backend is only ahead by the intended source commits and has no worktree changes. Record `$backendSourceCommit`; do not edit backend source after this point.

- [ ] **Step 2: Generate and check the Bundle against that exact SHA**

Run:

```powershell
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendSourceCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendSourceCommit
```

Expected: both exit 0. The OpenAPI contains `/api/admin/v1/ai/context/page-init`, structured Provider models/statuses, `model_kind`, and `degraded`; it contains no App/Canvas path.

- [ ] **Step 3: Inspect generated drift and commit only the Bundle**

Run:

```powershell
git diff -- contracts/admin/v1
git diff --check
```

Expected: every changed artifact is listed in `manifest.json` with a matching SHA-256; no source file changed during generation.

```powershell
git add -- contracts/admin/v1
git commit -m "chore(contract): publish optional context contract"
```

- [ ] **Step 4: Recheck the committed Bundle**

Run:

```powershell
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendSourceCommit
git status --short --branch
```

Expected: check exits 0 and backend worktree is clean. The manifest `backend_commit` equals `$backendSourceCommit`, which is an ancestor of the new Contract commit.

### Task 9: Sync the Contract and build the typed Provider editor

**Files:**
- Modify generated snapshot: `contracts/backend/admin/v1/*`
- Modify generated lock: `contracts/backend/admin/lock.json`
- Modify generated: `src/modules/http/generated/admin.ts`
- Modify generated: `src/modules/http/generated/operations.ts`
- Modify only if generated content changes: `src/modules/routing/generated/permissions.ts`
- Modify only if generated content changes: `src/modules/routing/generated/views.ts`
- Modify: `src/api/ai/providers.ts`
- Modify: `src/views/Main/ai/providers/composables/useProviderForm.ts`
- Create: `src/views/Main/ai/providers/components/ProviderModelEditor.vue`
- Create: `tests/component/ai/ProviderModelEditor.test.ts`
- Modify: `src/views/Main/ai/providers/components/ProviderFormDialog.vue`
- Modify: `src/views/Main/ai/providers/components/ProviderFormDialog.styles.css`
- Modify: `src/views/Main/ai/providers/components/ProviderModelList.vue`
- Modify: `tests/component/ai/ProviderModelList.test.ts`
- Modify: `tests/shared/ai/ai-provider-api-protocol.test.ts`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`

- [ ] **Step 1: Sync and generate from the manifest source SHA**

From `E:\admin\admin_front_ts` run:

```powershell
$manifest = Get-Content -Raw ..\admin_back_go\contracts\admin\v1\manifest.json | ConvertFrom-Json
$backendSourceCommit = [string]$manifest.backend_commit
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $backendSourceCommit
npm run contract:generate
npm run contract:check
```

Expected: all commands exit 0; frontend lock and generated types identify the same backend source SHA. Do not hand-edit generated files.

- [ ] **Step 2: Write failing form serialization and component tests**

Define the frontend source-of-truth row:

```ts
export interface ProviderModelDraft {
  model_id: string
  model_kind: 'chat' | 'embedding' | 'rerank'
  display_name: string
  status: 1 | 2
}
```

Tests assert:

```text
edit round-trip preserves model_kind, display_name and status
new remote candidate defaults explicitly to chat
kind can be changed to embedding/rerank before submit
mutation emits models + model_display_names + statuses and no model_ids
duplicate/blank Model ID is rejected
editor uses AppTable and contains no el-table or :deep
summary displays the kind and bounds overflow without wrapping the row
```

- [ ] **Step 3: Run focused tests and verify RED**

Run:

```powershell
npx vitest run tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelList.test.ts tests/component/ai/ProviderModelEditor.test.ts
```

Expected: FAIL because the form still stores only `model_ids` and edit loses `model_kind`.

- [ ] **Step 4: Replace parallel maps with one model-row array**

`ProviderFormState` owns `models: ProviderModelDraft[]`. `buildProviderMutationParams` derives the transport fields in one pass:

```ts
const models = form.models.map(({ model_id, model_kind }) => ({ model_id, model_kind }))
return {
  id: form.id,
  name: form.name,
  engine_type: form.driver,
  base_url: form.base_url,
  models,
  model_display_names: Object.fromEntries(form.models.map(row => [row.model_id, row.display_name])),
  statuses: Object.fromEntries(form.models.map(row => [row.model_id, row.status])),
  status: form.status,
  api_protocol: form.api_protocol,
  ...(form.api_key ? { api_key: form.api_key } : {}),
}
```

Candidate fetch merges by exact Model ID and never changes an existing row's kind/status. New candidates get `model_kind: 'chat'` visibly, not through an invisible backend default.

- [ ] **Step 5: Implement the editor with AppTable and stock Element controls**

Use AppTable columns for Model ID, display name, kind, status and remove action. Cell slots use `el-input`, `el-select`, `el-switch` and an icon-only Delete button with tooltip. Set `fixed-footer=false`, `show-refresh=false`, `show-column-setting=false`; do not add `:deep` rules.

`ProviderModelList` renders a small type tag beside each visible model and uses a plain flex wrapper instead of styling Element Plus internals. Keep the existing bounded overflow popover behavior.

- [ ] **Step 6: Verify GREEN and commit frontend changes**

Run:

```powershell
npx vitest run tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelList.test.ts tests/component/ai/ProviderModelEditor.test.ts tests/unit/contracts/admin-contract.test.ts
npm run contract:check
npm run locale:generate
npm run locale:check
git diff --check
```

Expected: PASS; generated locale types are committed if changed. No full frontend suite or build is run.

```powershell
git add -- contracts/backend/admin src/modules/http/generated src/modules/routing/generated src/api/ai/providers.ts src/views/Main/ai/providers/composables/useProviderForm.ts src/views/Main/ai/providers/components/ProviderModelEditor.vue src/views/Main/ai/providers/components/ProviderFormDialog.vue src/views/Main/ai/providers/components/ProviderFormDialog.styles.css src/views/Main/ai/providers/components/ProviderModelList.vue tests/component/ai/ProviderModelEditor.test.ts tests/component/ai/ProviderModelList.test.ts tests/shared/ai/ai-provider-api-protocol.test.ts src/i18n/locales/zh-CN/ai.ts src/i18n/locales/en-US/ai.ts src/i18n/locales/generated.ts
git commit -m "feat(ai): edit typed provider models"
```

### Task 10: Add typed Profile selectors and degradation UI

**Files:**
- Modify: `src/api/ai/context.ts`
- Modify: `src/views/Main/ai/context/use-context-workspace.ts`
- Modify: `src/views/Main/ai/context/index.vue`
- Modify: `src/views/Main/ai/context/components/ContextProfilePanel.vue`
- Modify: `src/views/Main/ai/context/components/ContextProfileDialog.vue`
- Modify: `tests/component/ai/ContextProfileDialog.test.ts`
- Modify: `tests/component/ai/ContextAdminTables.test.ts`
- Modify: `src/views/Main/ai/chat/components/MessageList/index.vue`
- Modify: `tests/component/ai/MessageInteractions.test.ts`
- Modify: `tests/component/accessibility/ai-chat.test.ts`
- Modify: `src/views/Main/ai/runs/components/RunList/context-plan.ts`
- Modify: `src/views/Main/ai/runs/components/RunList/RunContextPlan.vue`
- Create: `tests/component/ai/RunContextPlan.test.ts`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Modify: `src/i18n/locales/zh-CN/ai-extended.ts`
- Modify: `src/i18n/locales/en-US/ai-extended.ts`

- [ ] **Step 1: Write failing Profile, chat and Run tests**

Required assertions:

```text
Context Page Init is loaded once with the workspace
Embedding selector shows only embedding_model_options and is required
Reranker selector shows only reranker_model_options and is clearable
Memory selector shows only memory_model_options and is clearable
submitted IDs are option values, not parsed labels or manually typed strings
degraded assistant context displays the non-blocking status
the status is derived from message.context after a remount/refresh fixture
the status text is not appended to Assistant content
Run outcome helper maps degraded to warning
Run detail shows ready, degraded, stage and stable code
Run items use AppTable and contain no el-table or :deep
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
npx vitest run tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextAdminTables.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/accessibility/ai-chat.test.ts tests/component/ai/RunContextPlan.test.ts
```

Expected: FAIL because Profile still accepts naked numeric IDs, `degraded` has no presentation branch, and Run Context uses `el-table` directly.

- [ ] **Step 3: Consume Context Page Init without option guessing**

Add:

```ts
export type AiContextPageInit = Output<'ai_context_page_init'>

const pageInit = (options: ExecuteOptions = {}): Promise<AiContextPageInit> =>
  executeAdminOperation(adminOperations.ai_context_page_init, {}, options)
```

The workspace loads Page Init and profiles together on mount and stores the three arrays separately. Pass them through `ContextProfilePanel` to `ContextProfileDialog`. Do not call Provider list endpoints from the dialog.

Replace the three model ID controls with stock `el-select` controls using `option.value` and `option.label`. Use a normal component class with `width: 100%`; remove the existing `:deep(.el-input-number), :deep(.el-select)` rule.

- [ ] **Step 4: Render a persisted non-blocking chat status**

For a settled Assistant message with `message.context?.outcome === 'degraded'`, render a small status row outside `.message-card` and outside `message.content`:

```vue
<div
  v-if="isAssistant(message) && !message.isStreaming && message.context?.outcome === 'degraded'"
  class="message-context-status"
  data-test="context-degraded-status"
  role="status"
>
  <el-icon><Warning /></el-icon>
  <span>{{ t('aiChat.contextDegraded') }}</span>
</div>
```

Chinese text is exactly `本轮知识检索暂不可用，回答未引用空间或历史附件资料`. It does not claim recent complete turns or a valid Ready Memory were absent.

- [ ] **Step 5: Convert Run Context items to AppTable**

Define typed columns with stable widths and one-line overflow. Add `degraded` to `contextOutcomeTagType`. The summary shows Plan state/outcome/budget/proof; when `plan.error` exists it shows `stage` and `code`. Use warning presentation for a ready/degraded diagnostic and error presentation only for a failed Plan.

Use AppTable with `fixed-footer=false`, no toolbar controls and existing cell slots for decision/score. Do not wrap it in another card and do not add local deep selectors.

- [ ] **Step 6: Verify GREEN and commit**

Run:

```powershell
npx vitest run tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextAdminTables.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/accessibility/ai-chat.test.ts tests/component/ai/RunContextPlan.test.ts
npm run locale:generate
npm run locale:check
git diff --check
```

Expected: PASS; no Playwright, full Vitest suite, Vite build or browser automation is run.

```powershell
git add -- src/api/ai/context.ts src/views/Main/ai/context/use-context-workspace.ts src/views/Main/ai/context/index.vue src/views/Main/ai/context/components/ContextProfilePanel.vue src/views/Main/ai/context/components/ContextProfileDialog.vue tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextAdminTables.test.ts src/views/Main/ai/chat/components/MessageList/index.vue tests/component/ai/MessageInteractions.test.ts tests/component/accessibility/ai-chat.test.ts src/views/Main/ai/runs/components/RunList/context-plan.ts src/views/Main/ai/runs/components/RunList/RunContextPlan.vue tests/component/ai/RunContextPlan.test.ts src/i18n/locales/zh-CN/ai.ts src/i18n/locales/en-US/ai.ts src/i18n/locales/zh-CN/ai-extended.ts src/i18n/locales/en-US/ai-extended.ts src/i18n/locales/generated.ts
git commit -m "feat(ai): surface optional context degradation"
```

### Task 11: Focused verification, release boundary and manual acceptance handoff

**Files:**
- Verify only; no planned source edit.

- [ ] **Step 1: Run the bounded backend verification set**

From `E:\admin\admin_back_go` run:

```powershell
go test ./internal/module/ai/contextengine -run 'Degraded|Enhancement|Memory|Terminal|Retrieval|Attachment' -count=1
go test ./internal/module/ai/message ./internal/module/ai/run ./internal/runtime ./internal/telemetry -run 'Context|Degraded|Readiness|Redact|Prometheus' -count=1
go test ./internal/module/ai/provider ./internal/module/ai/provider/transport/admin ./internal/admincontract -run 'Provider|Model|ContextPageInit|OpenAPI' -count=1
$manifest = Get-Content -Raw contracts/admin/v1/manifest.json | ConvertFrom-Json
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $manifest.backend_commit
```

Expected: every command exits 0. Do not replace these with `verify-backend.ps1`, Docker integration or an unfiltered `go test ./...`.

- [ ] **Step 2: Run the bounded frontend verification set**

From `E:\admin\admin_front_ts` run:

```powershell
npx vitest run tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelEditor.test.ts tests/component/ai/ProviderModelList.test.ts tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextAdminTables.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/accessibility/ai-chat.test.ts tests/component/ai/RunContextPlan.test.ts tests/unit/contracts/admin-contract.test.ts
npm run contract:check
npm run locale:check
```

Expected: every command exits 0. Do not run `verify:frontend`, the full Vitest suite, build, Playwright or browser automation.

- [ ] **Step 3: Scan the implementation and Plan for forbidden shortcuts**

Run:

```powershell
rg -n 'RetrievalFailed.*default|default:.*ErrCodeRetrievalFailed|model_ids:' internal/module/ai/contextengine E:/admin/admin_front_ts/src/views/Main/ai/providers
rg -n '<el-table|:deep' E:/admin/admin_front_ts/src/views/Main/ai/runs/components/RunList/RunContextPlan.vue E:/admin/admin_front_ts/src/views/Main/ai/providers/components/ProviderModelEditor.vue E:/admin/admin_front_ts/src/views/Main/ai/context/components/ContextProfileDialog.vue
rg -n 'T[O]DO|T[B]D|implement l[a]ter|fill in d[e]tails|Similar t[o]|同[ ]Task|Add appropriate error h[a]ndling|Write tests for the a[b]ove' docs/superpowers/plans/2026-08-05-ai-agent-optional-context-enhancement.md
```

Expected: no matches. A compatibility `model_ids` type may remain in `src/api/ai/providers.ts` only if generated legacy input requires it; the new UI mutation path must not populate it.

- [ ] **Step 4: Verify repository and Contract identities**

Run:

```powershell
git -C E:/admin/admin_back_go status --short --branch
git -C E:/admin/admin_front_ts status --short --branch
$manifest = Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json
$lock = Get-Content -Raw E:/admin/admin_front_ts/contracts/backend/admin/lock.json | ConvertFrom-Json
if ($manifest.backend_commit -cne $lock.backend_commit) { throw 'frontend/backend contract source SHA mismatch' }
```

Expected: both worktrees are clean and the Contract source SHA is identical. Record both repository HEADs and the manifest SHA for the user.

- [ ] **Step 5: Preserve release and rollback ordering**

Release order:

```text
1. User reviews and applies 202608050101 during a maintenance window.
2. Deploy backend API/Worker that understands degraded.
3. Deploy the frontend locked to the matching Admin Contract.
4. Configure an Embedding model/Profile only after pure-chat regression passes.
```

Rollback order:

```text
1. Stop new traffic that can create degraded Plans.
2. Count existing ai_context_plans rows with retrieval_outcome='degraded'.
3. Keep the widened CHECK while any such row exists.
4. Roll back application code without deleting messages, attachments, Runs, Attempts or billing rows.
5. Narrow the CHECK only in a separately reviewed data migration after degraded rows are absent or explicitly migrated.
```

- [ ] **Step 6: Hand the user this manual browser checklist**

```text
1. Agent without Context Profile: normal message replies; refresh keeps both messages; Run is terminal.
2. Agent without Context Profile: current PDF and image still reach a capable main model.
3. Provider editor: add SiliconFlow Qwen/Qwen3-Embedding-0.6B as embedding and retain kind/status after reopening.
4. Context Profile: Embedding dropdown shows only Embedding models; choose 1024 dimensions and cosine.
5. Ready indexed architecture document: paraphrased question produces hit and real Citation.
6. Wrong Embedding API key: reply still succeeds; message survives refresh; Run is success with ready/degraded.
7. Degraded Run: stage/code are visible, Citation is empty, chat shows the non-blocking Chinese status.
8. Degraded request with a newly attached current file: capable main model still processes that file.
9. Restore the key and rebuild failed document versions: later requests return hit/no_hit normally.
10. Regression: Chat Provider, Agent scenes, tools, WebSocket streaming, image/file conversations and wallet settlement still behave as before.
```

Do not claim browser acceptance until the user reports these checks.

## Spec Coverage Matrix

| Approved requirement | Implementation task |
| --- | --- |
| no Profile/no sources avoids Embedding | Tasks 2, 4 |
| closed ready/degraded Plan and hash | Task 1 |
| typed optional failures only | Task 2 |
| Memory expected versus normal absence | Task 3 |
| discard partial evidence and Citation | Tasks 1, 4, 5 |
| current attachments independent of Embedding | Task 4 |
| existing terminal Plan reused before retrieval | Task 4 |
| permissions/snapshots/MySQL remain strict | Tasks 2, 4 |
| refresh-stable message and terminal Run | Task 5 |
| API degraded, Worker down | Task 5 |
| bounded stage telemetry | Task 5 |
| typed Context model options | Task 6 |
| Provider model kinds/statuses and legacy Chat compatibility | Task 7 |
| exact backend-to-frontend Contract SHA | Tasks 8, 9 |
| Provider typed model editor | Task 9 |
| Profile dropdowns, chat status and AppTable Run detail | Task 10 |
| no Embedding Docker/new table/new scene/second state machine | Delivery Boundaries and Task 11 scan |
| focused automated checks plus user browser acceptance | Task 11 |
