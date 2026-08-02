# AI 上下文工程检索与聊天接入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现可审计的 Dense/Sparse/QueryBatch/RRF/可选 Rerank 检索，以 MySQL 复核每个候选，并在 Provider 报价前把唯一 Context Plan 接入聊天、Citation、Run 和现有 finalizer。

**Architecture:** Planner 在无长事务的情况下固定授权快照、调用外部依赖并 Pack；提交前短事务重验 Run、Reply lease、身份、工具和来源后一次写入终局 Plan。Provider Attempt 引用 Plan ID/Hash 和精确 Prepared Request；恢复和重试只读这些持久事实，不再进入检索管线。

**Tech Stack:** Go 1.26.5、MySQL/GORM、Qdrant QueryBatch、OpenAI-compatible Embedding/Rerank HTTP、existing Reply Command/Gateway/Run/Finalizer。

---

## Fixed Retrieval Contract

```text
query variants:       current text; current text + newest complete turn
variant dedupe:       canonical query SHA-256 before Embedding
sparse encoder:       unicode_lexical_v1
candidate authority:  official Qdrant RRF result only
branch evidence:      independent Dense/Sparse results from same QueryBatch
authorization:        Qdrant filter, then mandatory MySQL batch verification
rerank disabled:      use RRF order
rerank configured:    all failures fail the Plan; never fall back to RRF
citation allocation:  selected document evidence only, after Pack, C1..Cn
```

### Task 1: Build canonical Turns, deterministic queries and QueryBatch RRF

**Files:**
- Create: `internal/module/ai/contextengine/conversation_turn.go`
- Create: `internal/module/ai/contextengine/conversation_turn_test.go`
- Create: `internal/module/ai/contextengine/conversation_repository.go`
- Create: `internal/module/ai/contextengine/conversation_repository_test.go`
- Create: `internal/module/ai/contextengine/query.go`
- Create: `internal/module/ai/contextengine/query_test.go`
- Create: `internal/infra/contextindex/query.go`
- Create: `internal/infra/contextindex/qdrant/query.go`
- Create: `internal/infra/contextindex/qdrant/query_test.go`
- Modify: `internal/infra/contextindex/qdrant/contract_integration_test.go`

- [ ] **Step 1: Write failing canonical-turn, query and QueryBatch tests**

Build fixtures for text-only, file/image attachments, completed/stopped answers, multiple complete Tool Call/Result groups, failed/canceled Runs, a missing Tool Result, edited/deleted Messages and changed Assistant delivery state. Assert only complete visible Turns are returned and no Tool Call, Tool Result or paired Message can appear independently. Golden tests prove any message text, attachment object fact, tool field, pairing or delivery-state change changes `SourceSHA256`; timestamps, signed URLs and database update time do not.

Test the canonical Turn text builder with the Plan 01 `TokenCounter`: it emits stable labeled User/Attachment/Tool/Assistant fields, never splits a Tool Call/Result pair, returns the exact upper bound, and deterministically stops before `max_tokens`. The fixed envelope not fitting is an error; an overlong individual text field uses the longest valid UTF-8 prefix proven by the same counter. Query tests prove empty/current-only/contextual variants, canonical SHA-256 duplicate removal and no fabricated text for attachment-only input. Both Dense and Sparse vectors for a contextual query must use that same bounded Turn text and the single `EncodeSparse` implementation created in Plan 02 Task 4.

Qdrant adapter test asserts a single QueryBatch contains every active Dense/Sparse branch plus one official RRF Query with the same Prefetch branches; candidate order comes only from RRF, while branch results only annotate those candidates.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/infra/contextindex ./internal/infra/contextindex/qdrant -run 'ConversationTurn|TurnHash|TurnText|ToolGroup|QueryVariant|SparseReuse|QueryBatch|RRF' -count=1`

Expected: FAIL because the canonical Turn builder/repository, query variants and runtime QueryBatch adapter do not exist.

- [ ] **Step 3: Implement one canonical Turn contract before retrieval**

Define the complete Profile-independent facts:

```go
type ConversationTurn struct {
	ConversationID    uint64
	UserID            uint64
	AgentID           uint64
	UserMessage       TurnMessage
	ToolGroups        []ToolGroup
	AssistantMessage  TurnMessage
	AssistantDelivery string
	SourceSHA256      [32]byte
}

type TurnMessage struct {
	ID            uint64
	Role          string
	Content       string
	ContentSHA256 [32]byte
	Attachments   []TurnAttachment
}

type TurnAttachment struct {
	Index           uint32
	Type            string
	StorageProvider string
	ObjectKey       string
	ETag            string
	Size            int64
	MIMEType        string
	Name            string
}

type ToolGroup struct {
	CallID    string
	Name      string
	Arguments string
	Result    string
}

type ConversationTurnText struct {
	Text            string
	TokenUpperBound int64
}

type ConversationTurnReader interface {
	NewestComplete(
		context.Context, uint64, uint64, *uint64,
	) (*ConversationTurn, error)
	CompleteByAnchors(
		context.Context, uint64, uint64, []uint64,
	) ([]ConversationTurn, error)
}

func BuildConversationTurnText(
	turn ConversationTurn,
	counter TokenCounter,
	maxTokens int64,
) (ConversationTurnText, error)
```

The `NewestComplete` arguments after `context.Context` are Conversation ID, User ID and an optional exclusive User Message anchor; `CompleteByAnchors` preserves the caller's anchor order and rejects cross-Conversation/User rows. Both use a fixed query count. They read paired Assistant, attachment, Tool Call/Result, Reply and Run facts and reject user-only failed/canceled Turns or incomplete Tool groups. Hash canonical `conversation_turn_v1` with stable IDs, roles, content hashes, ordered attachment type/object key/ETag/size/MIME/name, ordered Tool groups, Assistant delivery state and pairing. `BuildConversationTurnText` is the only bounded retrieval/index text builder; timestamps, signed URLs, Qdrant IDs, Memory IDs and Profile IDs never enter either text or hash.

- [ ] **Step 4: Implement deterministic query variants with the shared Sparse encoder**

Normalize current text once, build the optional contextual variant from `BuildConversationTurnText`, and deduplicate variants by canonical Query SHA-256 before Embedding. Empty current text produces no retrieval Query even when the current message has attachments. Call Plan 02's exported `EncodeSparse` for every query variant; deleting or copying that algorithm into this Task is forbidden.

- [ ] **Step 5: Implement one authoritative QueryBatch**

Define raw adapter input/result types in provider-neutral package `contextindex`:

```go
type QueryModality string

const (
	QueryModalityDense  QueryModality = "dense"
	QueryModalitySparse QueryModality = "sparse"
)

type QueryVariantVector struct {
	VariantID   string
	QuerySHA256 [32]byte
	Dense       []float32
	Sparse      SparseVector
}

type QueryBatchInput struct {
	Collection string
	Filter     ScopeFilter
	Variants   []QueryVariantVector
	TopN       uint64
}

type QueryFusionHit struct {
	Point PointRef
	Rank  uint64
	Score float64
}

type QueryBranchHit struct {
	Point     PointRef
	VariantID string
	Modality  QueryModality
	Rank      uint64
	Score     float64
}

type QueryBatchResult struct {
	Fusion  []QueryFusionHit
	Branches []QueryBranchHit
}

type Querier interface {
	QueryBatch(context.Context, QueryBatchInput) (QueryBatchResult, error)
}
```

Define the normalized domain result separately in `contextengine/query.go`:

```go
type Candidate struct {
	Point       contextindex.PointRef
	FusionScore FixedScore
	Branches    RetrievalBranchesV1
}
```

The Qdrant package imports only its parent `contextindex`, validates dimensions/finite scores/closed modalities and implements `Querier`; it never imports `contextengine` or constructs a domain `Candidate`. For each variant submit Dense and Sparse IDF Prefetch plus one official RRF Query. Context Engine converts finite adapter scores through Plan 01's six-decimal `FixedScore`, uses only Fusion output as the bounded candidate set/order, and attaches normalized branch evidence. Every Fusion candidate must occur in at least one independent branch result; otherwise return `ai.context.index_inconsistent`. Dense threshold applies only to Dense branches; Sparse has no Dense threshold.

- [ ] **Step 6: Run unit and real-Qdrant tests, then commit**

Run: `go test ./internal/module/ai/contextengine ./internal/infra/contextindex ./internal/infra/contextindex/qdrant -run 'ConversationTurn|TurnHash|TurnText|ToolGroup|QueryVariant|SparseReuse|QueryBatch|RRF' -count=1`

Run: `pwsh -NoProfile -File scripts/context/verify-qdrant-candidate.ps1 -CandidateImage qdrant/qdrant:v1.18.3 -PinEnv deploy/docker-state/qdrant-image.env`

Expected: both PASS and the integration gate exercises the runtime QueryBatch shape. The verifier requires the exact `qdrant/qdrant:v1.18.3` candidate and byte-identical lock already proven in Plan 02; a changed RepoDigest aborts this Wave instead of silently rewriting its dependency.

```bash
git add -- internal/module/ai/contextengine/conversation_turn.go internal/module/ai/contextengine/conversation_turn_test.go internal/module/ai/contextengine/conversation_repository.go internal/module/ai/contextengine/conversation_repository_test.go internal/module/ai/contextengine/query.go internal/module/ai/contextengine/query_test.go internal/infra/contextindex/query.go internal/infra/contextindex/qdrant/query.go internal/infra/contextindex/qdrant/query_test.go internal/infra/contextindex/qdrant/contract_integration_test.go
git commit -m "feat(ai): add hybrid context retrieval"
```

### Task 2: Revalidate candidates in MySQL, deduplicate and optionally Rerank

**Files:**
- Create: `internal/module/ai/contextengine/candidate_repository.go`
- Create: `internal/module/ai/contextengine/candidate_repository_test.go`
- Create: `internal/module/ai/contextengine/retrieval.go`
- Create: `internal/module/ai/contextengine/retrieval_test.go`
- Create: `internal/infra/ai/rerank.go`
- Create: `internal/infra/ai/openaicompat/rerank.go`
- Create: `internal/infra/ai/openaicompat/rerank_test.go`
- Modify: `internal/runtime/providers.go`
- Modify: `internal/runtime/providers_test.go`

- [ ] **Step 1: Write failing authorization and N+1 tests**

Use sqlmock query counts to prove any candidate count uses a fixed batch shape: one Profile/generation snapshot, one Space/Binding/Document/Version/Chunk batch, and one Conversation Turn batch. Test stale generation, revoked Binding, disabled/deleted Document, inactive Version, mismatched Chunk hash, other User/Conversation and changed Turn hash. None may enter Plan.

Add retrieval tests for Document `content_sha256` dedupe, Conversation `source_sha256` dedupe, adjacent same-version Chunk merge, non-adjacent/no-cross-version/no-over-limit merge, and deterministic tie order.

- [ ] **Step 2: Write failing Reranker strictness tests**

Test `disabled` performs no network call. Configured mode rejects timeout, 5xx, malformed JSON, duplicate/missing Candidate ID, response count mismatch, non-finite/out-of-range score and model mismatch. Every failure must be `ai.context.rerank_failed`; no test may observe RRF results returned as success afterward.

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/infra/ai ./internal/infra/ai/openaicompat ./internal/runtime -run 'Authority|Candidate|Deduplicate|Adjacent|Rerank|NPlusOne' -count=1`

Expected: FAIL because authoritative loading and Rerank interfaces are absent.

- [ ] **Step 4: Implement fixed batch verification and normalization**

Repository returns source-specific typed rows and recomputes every rule from design section 9.2. Known stale/revoked Point becomes an excluded item plus cleanup request; database error, snapshot failure or internally contradictory source becomes Plan failure, never `no_hit`.

Merge only consecutive Chunks from the same active Version with adjacent locators and a combined service limit. Merged Source Hash covers ordered `(chunk_id, chunk_facts_sha256)`; Metadata preserves every ID/locator and Content Snapshot uses the same canonical join.

Define `RerankClient` with declared single-request document/token limits. OpenAI-compatible adapter posts a closed `/rerank` request to the explicitly selected `model_kind=rerank` Provider Model. If a Provider does not implement that endpoint or cannot fit the bounded pool in one comparable request, Profile creation/use fails rather than batching incomparable scores.

- [ ] **Step 5: Add safe telemetry and injection boundaries**

Metrics record stage, duration, candidate counts and optional provider token usage only. Logs may contain request/run/plan IDs, stage and stable error code; logs and metric labels must never contain query, content, filename, signed URL, User ID or provider response. Qdrant Payload remains the closed schema from design section 9.1: Conversation scope may carry the authoritative numeric `user_id` required for pre-filtering, but never user text, filenames, URLs or unrestricted metadata. MySQL batch verification remains the final authorization decision. Evidence is marked untrusted in metadata for the Compiler; it cannot add tools or change authorization.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/infra/ai ./internal/infra/ai/openaicompat ./internal/runtime -run 'Authority|Candidate|Deduplicate|Adjacent|Rerank|NPlusOne|Telemetry' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/candidate_repository.go internal/module/ai/contextengine/candidate_repository_test.go internal/module/ai/contextengine/retrieval.go internal/module/ai/contextengine/retrieval_test.go internal/infra/ai/rerank.go internal/infra/ai/openaicompat/rerank.go internal/infra/ai/openaicompat/rerank_test.go internal/runtime/providers.go internal/runtime/providers_test.go
git commit -m "feat(ai): authorize and rerank context evidence"
```

### Task 3: Build one immutable terminal Plan per Run

**Files:**
- Create: `internal/module/ai/contextengine/planner.go`
- Create: `internal/module/ai/contextengine/planner_test.go`
- Create: `internal/module/ai/contextengine/authorization.go`
- Create: `internal/module/ai/contextengine/authorization_test.go`
- Modify: `internal/module/ai/contextengine/repository.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`

- [ ] **Step 1: Write the full BuildPlan state matrix first**

Cover `skipped/no_hit/hit/failed`, existing ready Plan, existing failed Plan, concurrent winner, cancellation before commit, lease loss, snapshot conflict, required overflow, Profile failed with/without actual sources, and external result discarded after another Worker wins.

```go
type BuildPlanInput struct {
	RunID           uint64
	ReplyCommandID  uint64
	LeaseOwner      string
	LeaseToken      uint64
	CurrentMessageID uint64
	AgentID         uint64
	UserID          uint64
	ConversationID  uint64
	ProviderID      uint64
	ModelID         string
	APIProtocol     string
	Tools           []infraai.ToolDefinition
}
```

Test that two concurrent calls may perform redundant external work, but both return the single committed terminal row and only the current lease holder proceeds. A failed Plan is returned as its original stable error and is never rebuilt for the same Run.

- [ ] **Step 2: Run Planner tests and verify RED**

Run: `go test ./internal/module/ai/contextengine -run 'BuildPlan|Snapshot|Concurrent|Lease|RetrievalOutcome' -count=1`

Expected: FAIL because Planner/authorization snapshot do not exist.

- [ ] **Step 3: Implement the no-long-transaction planning flow**

The exact order is:

```text
1. FindTerminalByRunID; return existing ready/failed fact.
2. Load immutable Run and authoritative message/attachment/agent/model/profile/binding/tool snapshot.
3. Compute input_fingerprint_sha256 before retrieval.
4. Resolve registered Chat counter and strict Budget components.
5. If no retrieval source, Pack core blocks with outcome=skipped.
6. Otherwise run bounded query Embedding, Qdrant QueryBatch, MySQL verification and optional Rerank.
7. Pack required and optional atomic groups; assign Citation keys; compute plan_sha256.
8. Open a short transaction, lock Run/Reply Command, recheck cancel/timeout/lease and every identity/hash/authorization.
9. Insert terminal Plan and Items once; on duplicate run_id reload winner.
10. Return ready Plan or the persisted failed Plan error.
```

Implement `authorization.GuardPlanCommitInTransaction(ctx, tx, token)` as the concrete `PlanCommitTransactionGuard` introduced in Plan 01. It must use the exact non-nil `*gorm.DB` supplied by `PersistTerminal`; lock Run and Reply Command, verify lease/cancel/timeout, then reload current message/attachment hashes, Agent/Provider/Model/Profile identity, Binding/Tool grants and every selected source hash. No guard query may escape to a repository rooted at the outer DB connection.

Cancellation, timeout or lease loss returns `ErrPlanCommitAborted` and rolls
back without a Plan. A snapshot/identity/content/authorization conflict returns
`PlanCommitGuardResult.SnapshotConflict`; `PersistTerminal` then writes exactly
one failed Plan header with NULL hash and no Items in that same transaction, as
fixed in Plan 01. Database failures roll back without a business Plan. Do not
collapse these three outcomes into one generic guard error.

Cancel/timeout before Step 9 produces no Plan and delegates to existing command state. Lease loss also produces no Plan and lets the current owner proceed. A real Context error persists a failed Plan with NULL hash and no Items through the same transaction API. External calls have bounded adapter retries only; Reply Worker never reruns retrieval for that Run after terminal persistence.

- [ ] **Step 4: Compile Plan Blocks to typed ChatInput**

Add a pure function `CompileChatInput(ContextPlan) (infraai.ChatInput, error)` that preserves selected Plan order and atomic groups. Document Evidence is a separate system-side envelope:

```text
[UNTRUSTED_CONTEXT C1]
source: <safe title and locator snapshot>
content:
<selected content snapshot>
[/UNTRUSTED_CONTEXT C1]
```

The original current User Message Part remains byte-for-byte unchanged. Compiler rejects an Evidence Item without a valid Citation key or safe snapshot.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine -run 'BuildPlan|CompileChatInput|Snapshot|Concurrent|Lease|RetrievalOutcome' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/planner.go internal/module/ai/contextengine/planner_test.go internal/module/ai/contextengine/authorization.go internal/module/ai/contextengine/authorization_test.go internal/module/ai/contextengine/repository.go internal/module/ai/contextengine/repository_test.go
git commit -m "feat(ai): build immutable context plans"
```

### Task 4: Replace KnowledgeRuntime in the chat command path

**Files:**
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/module/ai/aigateway/gateway.go`
- Modify: `internal/module/ai/aigateway/gateway_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/runtime/ai_billing_gateway_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`

- [ ] **Step 1: Write failing order and finalizer tests**

Instrument fakes to assert this sequence:

```text
message/run accepted -> tools resolved -> BuildPlan terminal ready
-> Prepared Request/Quote -> Attempt prepared -> Dispatch Guard -> Provider dispatch
```

Add negative tests for Qdrant/Embedding/Rerank/permission/budget failure on paid and unpaid paths. Expected: Run terminal `failed`, Reply Command terminal, frozen wallet released when present, no Provider call, no Assistant Message, no lingering `running`. Cancellation/timeout before Plan commit yields canceled/timeout with no failed Plan.

For every new paid and unpaid Chat Attempt, assert the ready Plan's exact
ID/SHA-256 is copied into `PaidChatAttemptInput`, `aigateway.RunRequest`,
`PreparedCall` and the persisted Attempt before dispatch. Assert a ready Plan
cannot produce a NULL/NULL Attempt relation after activation. Historical,
non-chat and already persisted pre-activation Attempts remain readable with
NULL/NULL. Recovery must return the persisted pair and bytes without invoking
BuildPlan or retrieval.

- [ ] **Step 2: Run chat/finalizer tests and verify RED**

Run: `go test ./internal/module/ai/chat ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/platform/admin ./internal/runtime -run 'ContextPlan|BeforeQuote|ContextFailure|NoAssistant|Finalizer' -count=1`

Expected: FAIL because chat still calls `KnowledgeRuntime.RetrieveForRun` and mutates `userContent`.

- [ ] **Step 3: Replace the dependency and reorder preparation**

Replace `KnowledgeRuntime` in `aichat.Dependencies` with `ContextRuntime`. Resolve current Tools before BuildPlan because tool schema belongs to the input fingerprint, budget and Plan. Call BuildPlan only after authoritative Run ID/model snapshot exist and before `paidChatAssembler.AssembleAndQuote` or unpaid Provider call.

For a ready Plan, require non-zero `plan.ID` and non-nil `plan.PlanSHA256`, build
one `aigateway.ContextPlanEvidence{ID: plan.ID, SHA256: *plan.PlanSHA256}`, and
pass that same value through the paid/unpaid attempt path. Never reload or
re-hash Plan Items while assembling the request. A failed Plan has no evidence
and enters the existing finalizer before any quote, prepare or Provider call.

Delete from chat service:

```go
knowledge, _ := s.knowledgeRuntime.RetrieveForRun(...)
userContent = knowledge.Context + "\n\n用户问题：\n" + userContent
```

Do not add an empty-context fallback. `skipped` and `no_hit` are valid ready Plans; an error goes through the existing paid/unpaid finalization paths.

- [ ] **Step 4: Wire Context Engine once**

In `internal/platform/admin/build.go`, construct Context repositories, index/Embedding/Rerank adapters and Planner, then inject the same `ContextRuntime` into chat and dispatch guard. Remove `knowledgeRuntimeAdapter` and its wiring; retain the old module files/routes until Plan 05 removes them.

Worker lease ownership is passed unchanged into BuildPlan. If BuildPlan reloads a concurrent winner, chat rechecks lease and Dispatch Guard before continuing; stale Worker exits without finalizing another owner's work.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/chat ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/platform/admin ./internal/runtime -count=1`

Expected: PASS, including previous attachment, stopped delivery, outcome recovery and billing finalizer regressions.

```bash
git add -- internal/module/ai/chat/dto.go internal/module/ai/chat/service.go internal/module/ai/chat/service_test.go internal/module/ai/aigateway/gateway.go internal/module/ai/aigateway/gateway_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/runtime/worker.go internal/runtime/worker_test.go
git commit -m "feat(ai): run chat through context plans"
```

### Task 5: Guard every dispatch and preserve Plan across tool Attempts

**Files:**
- Create: `internal/module/ai/contextengine/dispatch_guard.go`
- Create: `internal/module/ai/contextengine/dispatch_guard_test.go`
- Modify: `internal/module/ai/aigateway/gateway.go`
- Modify: `internal/module/ai/aigateway/gateway_test.go`
- Modify: `internal/module/ai/replycommand/attempt.go`
- Modify: `internal/module/ai/replycommand/attempt_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/runtime/ai_billing_gateway_test.go`
- Modify: `internal/runtime/ai_billing_finalizer.go`
- Modify: `internal/runtime/ai_billing_finalizer_test.go`

- [ ] **Step 1: Write revoke/recovery/tool-continuation tests**

Test binding revoked, Space/Document disabled/deleted, Conversation owner changed, message/attachment/Turn hash changed, Tool authorization removed, Memory invalidated, Plan/Attempt/Prepared hash mismatch, exact recovery success and second tool Attempt. On any revocation before dispatch: no network request and existing finalizer closes the Run. A newer active Document Version does not revoke an already prepared historical Version.

Test the second Attempt keeps the same Plan ID/Hash, adds the complete current Tool Call/Result group only to its own Prepared Request, and fails before dispatch with `ai.context.tool_continuation_overflow` if the reserved upper bound is exceeded. It never edits Plan Items or reruns retrieval.

- [ ] **Step 2: Run guard tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/aigateway ./internal/module/ai/replycommand ./internal/runtime -run 'DispatchGuard|PlanConflict|ToolContinuation|PreparedRecovery' -count=1`

Expected: FAIL because dispatch does not yet query Context authority.

- [ ] **Step 3: Implement one strict GuardDispatch**

`GuardDispatch` verifies the Plan is ready, belongs to Run, hash matches Attempt, Prepared hash matches persisted Attempt, Reply lease is current, and every selected source/tool/memory remains authorized with the Plan Source Hash. It accepts the same historical ready Version when a newer version became active, but rejects explicit disable/delete/revoke.

Gateway calls it immediately before every first or continuation network dispatch, including recovered prepared Attempts. It never asks Planner to rebuild and never rewrites persisted evidence.

- [ ] **Step 4: Enforce the continuation reserve**

Before executing Tools and before preparing Attempt 2, canonicalize Call ID/Name/Arguments and Result payload as one atomic group, calculate the registered upper bound, and compare with the Plan's fixed `tool_continuation_input_reserve`. Unknown/unbounded Tool schema was already rejected at Plan build; runtime overflow is a terminal Context error, never truncation.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/aigateway ./internal/module/ai/replycommand ./internal/runtime -run 'DispatchGuard|PlanConflict|ToolContinuation|PreparedRecovery' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/dispatch_guard.go internal/module/ai/contextengine/dispatch_guard_test.go internal/module/ai/aigateway/gateway.go internal/module/ai/aigateway/gateway_test.go internal/module/ai/replycommand/attempt.go internal/module/ai/replycommand/attempt_test.go internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/runtime/ai_billing_finalizer.go internal/runtime/ai_billing_finalizer_test.go
git commit -m "feat(ai): guard context provider dispatch"
```

### Task 6: Project Citations from persisted Message and Plan facts

**Files:**
- Create: `internal/module/ai/contextengine/citation.go`
- Create: `internal/module/ai/contextengine/citation_test.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/repository.go`
- Create: `internal/module/ai/message/repository_test.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`

- [ ] **Step 1: Write failing valid/invalid/unreferenced projection tests**

Use completed and stopped Assistant Messages with `reply_command_id -> run_id -> plan_id`. Test repeated `[C1]`, valid `[C2]`, unknown `[C99]`, malformed/zero/negative keys, selected-but-unmentioned Evidence and non-document Plan Items. Refreshing from repositories must return the same projection without WebSocket memory.

- [ ] **Step 2: Run projection tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/message ./internal/module/ai/run -run 'Citation|ContextPlanProjection|Refresh' -count=1`

Expected: FAIL because Message and Run DTOs do not expose Context facts.

- [ ] **Step 3: Define one concise Message context DTO**

```go
type MessageContext struct {
	PlanID      uint64           `json:"plan_id"`
	Outcome     RetrievalOutcome `json:"outcome"`
	Sources     []CitationSource `json:"sources"`
	InvalidKeys []string         `json:"invalid_keys"`
}

type CitationSource struct {
	Key               string           `json:"key"`
	Cited             bool             `json:"cited"`
	Title             string           `json:"title"`
	Locator           ContextLocatorV1 `json:"locator"`
	DocumentID        uint64           `json:"document_id"`
	DocumentVersionID uint64           `json:"document_version_id"`
}
```

Only selected `document_evidence` Items supply Sources. Parse final persisted Assistant content with fixed `\[C([1-9][0-9]*)\]`, preserve Plan order, mark actual keys, and return unknown keys without source DTO. Do not create a Citation table and do not infer source from Markdown links or titles.

- [ ] **Step 4: Replace Run knowledge projection with structured context_plan**

Run detail returns Plan header, exact Budget proof, safe stage metrics and ordered Items with decisions, exclusion reason, fixed scores, Citation key, locator and bounded safe snapshot. It never exposes Query text, object key, signed URL, credentials, raw Provider response or unrestricted metadata map. Keep old Run fields only until Plan 05 performs the same-batch Contract removal.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/message ./internal/module/ai/run -run 'Citation|ContextPlanProjection|Refresh' -count=1`

Expected: PASS for completed/stopped and refresh projections.

```bash
git add -- internal/module/ai/contextengine/citation.go internal/module/ai/contextengine/citation_test.go internal/module/ai/message internal/module/ai/run
git commit -m "feat(ai): project context citations and runs"
```

### Task 7: Add deterministic evaluation data and end-to-end terminal tests

**Files:**
- Create: `internal/module/ai/contextengine/testdata/evaluation_corpus_v1.jsonl`
- Create: `internal/module/ai/contextengine/evaluation.go`
- Create: `internal/module/ai/contextengine/evaluation_test.go`
- Create: `internal/module/ai/replycommand/context_integration_test.go`
- Modify: `internal/module/ai/replycommand/runner_integration_test.go`
- Modify: `internal/module/ai/aigateway/finalizer_test.go`

- [ ] **Step 1: Commit a closed 60-case Chinese evaluation set**

Each JSONL row uses this schema and contains no credentials or production text:

```json
{"id":"lexical-01","category":"lexical","query":"退款到账需要几天","expected_source_ids":["policy-refund-01"],"denied_source_ids":[]}
```

Exact category counts are 20 lexical/exact, 20 semantic paraphrase, 10 multi-turn, 5 expected no-hit and 5 cross-scope denial. The test rejects duplicate IDs, missing expected sources, wrong counts and unknown fields. Fixtures include a paired synthetic corpus with explicit Space/User/Conversation ownership.

- [ ] **Step 2: Write metric threshold and terminal matrix tests**

Evaluation computes Recall@10, MRR@10, no-hit false-positive rate, cross-scope leakage and Citation mapping validity using the real retrieval/Packer pipeline. Thresholds are `0.90`, `0.75`, `0.05`, `0`, `1.00` respectively.

Reply integration table covers paid/unpaid plus `skipped/no_hit/hit`, Qdrant/Embedding/Rerank/permission/overflow failure, cancel, timeout, provider failure, completed and stopped. Every row asserts Run/Reply/Attempt/wallet/message terminal facts and that no Context error leaves `running`.

- [ ] **Step 3: Run tests and verify RED, then implement the evaluator**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/replycommand ./internal/module/ai/aigateway -run 'Evaluation|ContextTerminalMatrix' -count=1`

Expected before implementation: FAIL because evaluator and terminal matrix are absent.

Implement the evaluator as a pure report over typed expected/actual results. It creates no Run, Plan, evaluation table or wallet charge. Non-deterministic LLM judging is excluded.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/replycommand ./internal/module/ai/aigateway -run 'Evaluation|ContextTerminalMatrix' -count=1`

Expected: PASS and print the five metrics without query/source text.

```bash
git add -- internal/module/ai/contextengine/testdata/evaluation_corpus_v1.jsonl internal/module/ai/contextengine/evaluation.go internal/module/ai/contextengine/evaluation_test.go internal/module/ai/replycommand/context_integration_test.go internal/module/ai/replycommand/runner_integration_test.go internal/module/ai/aigateway/finalizer_test.go
git commit -m "test(ai): verify context retrieval outcomes"
```

### Task 8: Verify chat integration before memory work

**Files:**
- Modify: `docs/architecture.md`

- [ ] **Step 1: Document Plan/Attempt/Prepared evidence and failure order**

Add the three-hash relationship, BuildPlan placement, dispatch revocation semantics, valid/no-hit/failed distinction, tool continuation reserve and persisted Citation projection. Mark old KnowledgeRuntime as disconnected but not physically deleted until Plan 05.

- [ ] **Step 2: Run formatting and focused regression**

Run: `gofmt -w internal/module/ai/contextengine internal/infra/contextindex internal/infra/ai internal/module/ai/chat internal/module/ai/message internal/module/ai/run internal/module/ai/replycommand internal/module/ai/aigateway internal/runtime internal/platform/admin`

Run: `go test ./internal/module/ai/contextengine/... ./internal/infra/contextindex/... ./internal/infra/ai/... ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/run ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/runtime ./internal/platform/admin -count=1`

Expected: PASS.

- [ ] **Step 3: Check root-cause removals**

Run: `rg -n 'RetrieveForRun|knowledge\.Context|用户问题：|strings\.Contains' internal/module/ai/chat internal/platform/admin`

Expected: no active chat/runtime match.

Run: `rg -n 'context.*(fallback|continue)|rerank.*rrf' internal/module/ai/contextengine internal/runtime`

Expected: no silent fallback implementation; explicit test names may mention rejection.

Run: `rg -n 'internal/module/ai/contextengine' internal/infra/contextindex`

Expected: no matches; the provider-neutral contract and Qdrant adapter never import the domain module.

- [ ] **Step 4: Commit and record checkpoint**

Run: `git diff --check`

```bash
git add -- docs/architecture.md
git commit -m "docs(ai): define context chat evidence flow"
```

Run: `git status --short`

Expected: clean. Do not restart `admin-dev`; browser validation remains deferred to the final UI Slice.
