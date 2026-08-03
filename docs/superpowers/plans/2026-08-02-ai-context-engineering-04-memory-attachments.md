# AI 上下文工程会话记忆与历史附件 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用统一完整轮次、Conversation 私有检索、历史附件摄取、自动历史分页和可验证 Rolling Memory 维持长对话连续性，并彻底停止 `max_history` 对运行时的影响。

**Architecture:** 原始 Message、Attachment、Tool Call/Result、Reply Command 和 Run 仍是唯一会话事实；`ConversationTurn`、私有 Point、Document Version 和 Memory 都是可失效/重建的派生物。直接历史、私有召回和 Memory 使用同一个 Turn 构造器和 Source Hash，因此修改历史只需按消息边界失效后继派生物，无需特殊补丁。

**Tech Stack:** Go 1.26.5、MySQL/GORM、Asynq、Qdrant、现有 Chat/Message/Reply/Tool 持久链。

---

## Fixed Conversation Contract

```text
turn anchor:             user_message_id
turn members:            user message + attachment facts + complete tool groups
                         + paired visible assistant message + delivery state
visible assistant state: completed | stopped
turn hash schema:        conversation_turn_v1
private scope:           platform + user_id + conversation_id + profile_id
history paging:          descending stable ID pages, complete turns only
memory high watermark:   uncovered complete history > 25% chat known input budget
memory low watermark:    summarize oldest prefix until uncovered <= 12.5%
```

### Task 1: Extend the canonical ConversationTurn repository for long history

**Files:**
- Modify: `internal/module/ai/contextengine/conversation_turn.go`
- Modify: `internal/module/ai/contextengine/conversation_turn_test.go`
- Modify: `internal/module/ai/contextengine/conversation_repository.go`
- Modify: `internal/module/ai/contextengine/conversation_repository_test.go`

- [ ] **Step 1: Write failing stable-pagination and long-history tests**

Plan 03 Task 1 already owns the only `ConversationTurn` type, canonical hash and bounded retrieval/index text. Build more than 50 complete fixture Turns interleaved with failed/canceled/incomplete rows. Page backward from a stable User Message anchor and assert every complete Turn appears exactly once in descending anchor order across page boundaries; no incomplete member appears, and rows inserted above the starting anchor cannot shift or duplicate an existing traversal.

```go
type ConversationTurnPage struct {
	Turns                   []ConversationTurn
	NextBeforeUserMessageID *uint64
}

type ConversationTurnPager interface {
	PageCompleteBefore(
		context.Context, uint64, uint64, *uint64, int,
	) (ConversationTurnPage, error)
}
```

Test fixed query count for page sizes 1, 16 and 64. Reload each returned anchor through the Plan 03 batch method and assert byte-identical `SourceSHA256` and `ConversationTurnText`; pagination is an access pattern, never a second builder.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/module/ai/contextengine -run 'ConversationTurnPage|Descending|MoreThanFifty|NPlusOne' -count=1`

Expected: FAIL because the repository cannot page complete Turns by stable descending anchors.

- [ ] **Step 3: Add one bounded descending page method**

The arguments after `context.Context` are Conversation ID, User ID, optional exclusive User Message anchor and page size. The caller supplies only a service-owned page size within a fixed maximum; HTTP DTOs never expose it. One anchor query uses `EXISTS`/`NOT EXISTS` against the same terminal Reply/Run, paired visible Assistant and complete Tool-group rules to select at most `pageSize + 1` eligible User Message IDs by `(conversation_id, user_id, id < before_id)` in descending order. Fixed batch loaders then build the first `pageSize` anchors through the Plan 03 canonical builder. The extra anchor only proves another page exists; when it does, `NextBeforeUserMessageID` is the oldest returned anchor so the next exclusive query includes, rather than skips, the extra row. Otherwise it is `nil`. No retry loop or malformed-row special case can make query count depend on conversation length.

Do not redefine `ConversationTurn`, recompute its hash in the pager, or introduce a page-only text representation. Direct history, private indexing, candidate verification and Memory continue to consume the one Plan 03 contract.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine -run 'ConversationTurn|ConversationTurnPage|Descending|MoreThanFifty|NPlusOne' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/conversation_turn.go internal/module/ai/contextengine/conversation_turn_test.go internal/module/ai/contextengine/conversation_repository.go internal/module/ai/contextengine/conversation_repository_test.go
git commit -m "feat(ai): page canonical conversation turns"
```

### Task 2: Replace fixed message counts with budget-driven history paging

**Files:**
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/repository.go`
- Modify: `internal/module/ai/chat/repository_test.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/module/ai/contextengine/planner.go`
- Modify: `internal/module/ai/contextengine/planner_test.go`

- [ ] **Step 1: Write the regression tests that expose fixed-history loss**

Create a conversation with more than 50 Message rows where an old complete turn fits the token budget. Assert Planner pages until the budget is covered rather than truncating at 50. Send identical messages with `max_history=1` and `max_history=50`; assert identical Plan input fingerprint, selected history and Prepared Request.

Add tests proving the newest Turn and each older Turn are whole atomic groups: when one does not fit it is excluded as a group with `budget_exceeded`, never split into a user-only or assistant-only message.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/module/ai/chat ./internal/module/ai/contextengine -run 'AutomaticHistory|MaxHistoryIgnored|HistoryAtomic|MoreThanFifty' -count=1`

Expected: FAIL because chat still calls `LatestMessages(..., maxHistoryLimit+1)` and `maxHistoryFromMeta`.

- [ ] **Step 3: Remove effective `max_history` behavior**

Delete `maxHistoryLimit`, `maxHistoryFromMeta`, `chatHistoryWithLimit`, `selectedChatContext` and `chatHistoryInputsWithLimit`. Chat service loads only the current message needed for acceptance identity; Planner owns all historical paging through the canonical Conversation repository.

Keep `MessageRuntimeParams.MaxHistory` temporarily as a typed compatibility input so old clients receive no JSON binding error, but never copy it into `BuildPlanInput`, input fingerprint, Budget, Plan, ChatInput or Provider request. Plan 05 removes it from the published DTO/OpenAPI.

- [ ] **Step 4: Implement budget-driven paging**

Planner requests descending pages of complete Turns, calculates each full group's bound using the current Chat counter, and stops when all known input budget has candidate coverage or it reaches the latest valid Memory boundary. It records considered selected/excluded groups in stable order, does not load the entire conversation, and uses a fixed page size as an I/O batching detail only, never as a user-visible history limit.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/chat ./internal/module/ai/contextengine -run 'AutomaticHistory|MaxHistoryIgnored|HistoryAtomic|MoreThanFifty' -count=1`

Expected: PASS.

Run: `rg -n 'maxHistoryLimit|maxHistoryFromMeta|chatHistoryWithLimit|selectedChatContext|LatestMessages\(' internal/module/ai/chat internal/module/ai/contextengine`

Expected: no active runtime matches.

```bash
git add -- internal/module/ai/chat/dto.go internal/module/ai/chat/repository.go internal/module/ai/chat/repository_test.go internal/module/ai/chat/service.go internal/module/ai/chat/service_test.go internal/module/ai/contextengine/planner.go internal/module/ai/contextengine/planner_test.go
git commit -m "refactor(ai): page history by context budget"
```

### Task 3: Index complete Turns in Conversation-private Qdrant scope

**Files:**
- Modify: `internal/jobs/ai_context.go`
- Modify: `internal/jobs/ai_context_test.go`
- Create: `internal/module/ai/contextengine/conversation_index.go`
- Create: `internal/module/ai/contextengine/conversation_index_test.go`
- Modify: `internal/module/ai/contextengine/cleanup.go`
- Modify: `internal/module/ai/contextengine/cleanup_test.go`
- Modify: `internal/module/ai/contextengine/jobs.go`
- Modify: `internal/module/ai/contextengine/jobs_test.go`
- Modify: `internal/module/ai/contextengine/reconciler.go`
- Modify: `internal/module/ai/contextengine/reconciler_test.go`
- Modify: `internal/module/ai/replycommand/finalization.go`
- Modify: `internal/module/ai/replycommand/finalization_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`
- Modify: `internal/runtime/context_readiness.go`
- Modify: `internal/runtime/context_readiness_test.go`

- [ ] **Step 1: Write failing scope, idempotency and crash-window tests**

Test that completed/stopped visible Turns enqueue, failed/canceled/no-assistant Runs do not, Profile NULL disables indexing, same Turn retries to the same Point, changed/deleted Turn facts suppress stale work and schedule deterministic old-Point cleanup, another User/Conversation cannot query it, and a crash after finalization but before enqueue is repaired by bounded full rescan. Missing expected Conversation Points re-enqueue without failing the Profile; a stale Point whose source Message is no longer visible is cleanup work, never a valid candidate.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/jobs ./internal/module/ai/contextengine ./internal/module/ai/replycommand ./internal/runtime -run 'ConversationIndex|PrivateScope|TurnBackfill' -count=1`

Expected: FAIL because the task and post-finalizer hook do not exist.

- [ ] **Step 3: Define the task and deterministic identity**

```go
const TaskContextConversationIndexV1 = "ai:context-conversation-index:v1"

type ContextConversationIndexV1 struct {
	ProfileID       uint64   `json:"profile_id"`
	ConversationID  uint64   `json:"conversation_id"`
	UserMessageID   uint64   `json:"user_message_id"`
	SourceSHA256    [32]byte `json:"source_sha256"`
}
```

Unique key includes task type and all fields. Point uses `source_kind=conversation_turn`, anchor User Message ID and the UUIDv8 algorithm from Plan 02. Payload derives platform/user/conversation from authoritative Conversation rows, not task input.

Extend Plan 02's closed cleanup union only now that Conversation Points exist:

```go
const CleanupConversationPoints CleanupKind = "conversation_points"
```

Cleanup input carries Profile/generation, Conversation/User Message anchor and exact old Source Hash. Its handler first proves that exact Turn identity is no longer authoritative in MySQL, then removes only the deterministic old Point. A still-visible matching Turn cancels cleanup.

- [ ] **Step 4: Implement eventual repair without a cursor table**

After the existing finalizer has committed a visible Assistant Message, best-effort enqueue the task; enqueue failure records a low-cardinality metric and does not roll back the finalizer. Reconciler scans terminal Runs in stable ID batches on every cycle, computes expected Point IDs and re-enqueues missing work. It also validates bounded batches of existing Conversation Points against the same canonical MySQL visibility/hash rule and schedules cleanup for orphaned identities. It may keep an in-memory/per-process scan position for efficiency, but periodic full restart from ID zero guarantees correctness and no cursor/status table is added.

Worker reloads Agent Profile and canonical Turn immediately before Upsert. Changed facts discard stale work, enqueue current hash and schedule deterministic cleanup for the old Point. Missing Turn Points never mark an otherwise healthy Profile failed.

Extend source-aware API readiness from Plan 02: an indexable complete Turn now makes Qdrant mandatory. Register `conversation-index` and require exactly four delivered Context handlers at this checkpoint: document index, index cleanup, profile rebuild and conversation index. Memory is not required until Task 5 creates it.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/jobs ./internal/module/ai/contextengine ./internal/module/ai/replycommand ./internal/runtime -run 'ConversationIndex|PrivateScope|TurnBackfill' -count=1`

Expected: PASS.

```bash
git add -- internal/jobs/ai_context.go internal/jobs/ai_context_test.go internal/module/ai/contextengine/conversation_index.go internal/module/ai/contextengine/conversation_index_test.go internal/module/ai/contextengine/cleanup.go internal/module/ai/contextengine/cleanup_test.go internal/module/ai/contextengine/jobs.go internal/module/ai/contextengine/jobs_test.go internal/module/ai/contextengine/reconciler.go internal/module/ai/contextengine/reconciler_test.go internal/module/ai/replycommand/finalization.go internal/module/ai/replycommand/finalization_test.go internal/runtime/worker.go internal/runtime/worker_test.go internal/runtime/context_readiness.go internal/runtime/context_readiness_test.go
git commit -m "feat(ai): index private conversation turns"
```

### Task 4: Reuse Document ingestion for historical supported attachments

**Files:**
- Create: `internal/module/ai/contextengine/conversation_document.go`
- Create: `internal/module/ai/contextengine/conversation_document_test.go`
- Create: `internal/module/ai/contextengine/history_invalidation.go`
- Create: `internal/module/ai/contextengine/history_invalidation_test.go`
- Modify: `internal/module/ai/contextengine/repository.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`
- Modify: `internal/module/ai/contextengine/candidate_repository.go`
- Modify: `internal/module/ai/contextengine/candidate_repository_test.go`
- Modify: `internal/module/ai/contextengine/ingestion.go`
- Modify: `internal/module/ai/contextengine/ingestion_test.go`
- Modify: `internal/module/ai/contextengine/rebuild.go`
- Modify: `internal/module/ai/contextengine/rebuild_test.go`
- Modify: `internal/module/ai/contextengine/cleanup.go`
- Modify: `internal/module/ai/contextengine/cleanup_test.go`
- Modify: `internal/module/ai/contextengine/reconciler.go`
- Modify: `internal/module/ai/contextengine/reconciler_test.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/history_repository.go`
- Modify: `internal/module/ai/message/history_actions_test.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/runtime/context_readiness.go`
- Modify: `internal/runtime/context_readiness_test.go`

- [ ] **Step 1: Write failing identity and post-commit tests**

Use one message with two same-name files and assert two Documents keyed by `(conversation_id, source_message_id, zero-based attachment_index)`. Repeating the hook reuses the Document/Version for identical object facts; changed ETag/size creates a new immutable Version. Other conversation/user/space cannot read the Documents.

Test that message acceptance stays successful when post-commit Document creation/enqueue fails, while Run detail projects diagnostic code `attachment_ingestion_pending`; Reconciler later creates missing rows and the projection disappears. Assert the diagnostic is derived without inserting a diagnostic/job/status row and that the current attachment still follows the existing native Provider path.

Add transaction tests for revision, regeneration and exact message deletion. Before old rows become invisible, the injected invalidator must capture affected canonical Turn identities, disable every Conversation Document whose source User Message becomes invisible, and invalidate every ready Memory at or after the earliest affected Turn anchor in the same transaction. Any invalidation error rolls back the history mutation. Deleting only an Assistant Message invalidates its Turn/Memory but does not disable a still-visible User Message's Document. A crash or enqueue failure after commit leaves MySQL unauthorized and is repaired by Reconciler cleanup.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/message ./internal/module/ai/run ./internal/runtime ./internal/platform/admin -run 'ConversationDocument|AttachmentIngestion|AttachmentIndex|HistoryDerivedInvalidation|ConversationDocumentAuthority' -count=1`

Expected: FAIL because `EnsureConversationDocuments`, the transaction invalidator and the shared authority query do not exist.

- [ ] **Step 3: Implement the post-commit attachment adapter**

Expose:

```go
type ConversationDocumentEnsurer interface {
	EnsureConversationDocuments(context.Context, uint64) error
}
```

Message service calls it only after the user-message acceptance transaction commits and only when the Agent has a Context Profile. For TXT, Markdown, text PDF, DOCX, CSV and XLSX, validate the stored trusted attachment location against Message/Conversation ownership, create/reuse Document, create a `queued` Version for new source facts and enqueue the existing document-index task. Images and unsupported formats do not create Documents.

Creation failure emits a low-cardinality metric and safe IDs only; it does not delete/alter the accepted Message or convert the current Run to failed. Do not persist the string `attachment_ingestion_pending`. Add one repository query shared by Reconciler and Run projection: for the Run's accepted user message, enumerate supported attachment indexes from the closed `meta_json` DTO and LEFT JOIN the exact `(conversation_id, source_message_id, source_attachment_index)` Document plus matching source-facts Version. A missing Document/Version or a `queued|processing` Version projects `attachment_ingestion_pending`; `ready` removes it; `failed` projects that Version's stable error code instead. This query is the diagnostic truth and adds no tenth table.

Reconciler uses the same query in stable Message ID batches to create missing rows or re-enqueue existing queued work. Because both readers use the same attachment identity, there is no separate boolean to drift and no default value hiding a broken post-commit hook.

- [ ] **Step 4: Make history mutation invalidate derived authority in its transaction**

Define the narrow contract in package `message`, which already owns the GORM history transaction and therefore must not import `contextengine`:

```go
type HistoryAfterCommit func(context.Context)

type HistoryDerivedInvalidator interface {
	InvalidateSuffixInTransaction(
		context.Context,
		*gorm.DB,
		userID int64,
		conversationID int64,
		fromMessageID int64,
		throughMessageID int64,
	) (HistoryAfterCommit, error)
	InvalidateMessagesInTransaction(
		context.Context,
		*gorm.DB,
		userID int64,
		conversationID int64,
		messageIDs []int64,
	) (HistoryAfterCommit, error)
}
```

Revision/regeneration calls the suffix method with the already locked `[cutFrom, upperBound]`; exact deletion calls the message-set method with the already sorted, locked IDs. There is no optional-field union and therefore no representable “both selectors” or “no selector” state. Inject the interface with `WithRepositoryHistoryDerivedInvalidator`; production Admin wiring must always provide the Context implementation, and history mutations fail closed when it is absent. Call it before hiding rows. The Context implementation resolves affected User Message anchors, snapshots exact old Turn Point identities, sets matching Conversation Documents to `disabled`, and invalidates ready Memory rows with `through_message_id >= earliest_affected_anchor`, all through the supplied `tx`. It returns a callback containing primitive cleanup identities only, never the transaction handle.

Invoke the callback only after commit. It best-effort enqueues exact Conversation/Document Point cleanup and records a low-cardinality failure metric itself; it cannot turn a committed history mutation into an API failure. Reconciler applies the same MySQL visibility rule to bounded Qdrant batches, so a process crash before the callback or Redis failure is compensatable. MySQL authorization is always removed before any Point deletion.

- [ ] **Step 5: Centralize Conversation Document authority and availability**

Add one typed authority query used by ingestion activation, candidate verification, rebuild, readiness, diagnostic projection and Reconciler. A Conversation Document is authoritative only when its Conversation belongs to the current User, Document is enabled/not deleted, source Message is visible, the zero-based attachment index still exists in the closed Message metadata, and storage provider/object key/ETag/size/MIME/name exactly match the Version source facts. Candidate lookup additionally requires the exact active ready Version and Chunk hash. A mismatch is `inactive_source` plus deterministic cleanup, never a default attachment or a second metadata parser.

Rebuild excludes unauthorized Conversation Documents. Source-aware API readiness counts only authoritative ready Conversation Documents; the presence of one makes Qdrant mandatory. Reconciler and ingestion use the same query to prevent a queued stale Version from activating after history mutation. They may delete stale Points only after the MySQL rule has denied them.

Recent complete Turn attachments may be replayed through the existing native file protocol as an atomic group. Before older history is selected, supported files require an authoritative ready private Document Version; if neither native replay nor ready version is possible, return `ai.context.attachment_unavailable`. Never silently omit the attachment. Images remain only in recent native Turns; no OCR or long-term image memory is added.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/message ./internal/module/ai/run ./internal/module/ai/chat ./internal/runtime ./internal/platform/admin -run 'ConversationDocument|AttachmentIngestion|AttachmentIndex|AttachmentUnavailable|HistoryDerivedInvalidation|ConversationDocumentAuthority|Rebuild|ContextReadiness' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/conversation_document.go internal/module/ai/contextengine/conversation_document_test.go internal/module/ai/contextengine/history_invalidation.go internal/module/ai/contextengine/history_invalidation_test.go internal/module/ai/contextengine/repository.go internal/module/ai/contextengine/repository_test.go internal/module/ai/contextengine/candidate_repository.go internal/module/ai/contextengine/candidate_repository_test.go internal/module/ai/contextengine/ingestion.go internal/module/ai/contextengine/ingestion_test.go internal/module/ai/contextengine/rebuild.go internal/module/ai/contextengine/rebuild_test.go internal/module/ai/contextengine/cleanup.go internal/module/ai/contextengine/cleanup_test.go internal/module/ai/contextengine/reconciler.go internal/module/ai/contextengine/reconciler_test.go internal/module/ai/message/dto.go internal/module/ai/message/repository.go internal/module/ai/message/history_repository.go internal/module/ai/message/history_actions_test.go internal/module/ai/message/service.go internal/module/ai/message/service_test.go internal/module/ai/run/dto.go internal/module/ai/run/repository.go internal/module/ai/run/repository_test.go internal/module/ai/run/service.go internal/module/ai/run/service_test.go internal/runtime/context_readiness.go internal/runtime/context_readiness_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go
git commit -m "feat(ai): ingest private conversation attachments"
```

### Task 5: Build a linear, verifiable Rolling Memory chain

**Files:**
- Modify: `internal/jobs/ai_context.go`
- Modify: `internal/jobs/ai_context_test.go`
- Create: `internal/module/ai/contextengine/memory.go`
- Create: `internal/module/ai/contextengine/memory_test.go`
- Create: `internal/module/ai/contextengine/memory_repository.go`
- Create: `internal/module/ai/contextengine/memory_repository_test.go`
- Modify: `internal/module/ai/contextengine/jobs.go`
- Modify: `internal/module/ai/contextengine/jobs_test.go`
- Modify: `internal/module/ai/contextengine/reconciler.go`
- Modify: `internal/module/ai/contextengine/reconciler_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`

- [x] **Step 1: Write failing chain, watermark and invalidation tests**

Cover no Memory model, below 25%, above 25% summarized to at most 12.5%, multiple bounded prefix tasks, parent Summary plus new Turns, parent changed during external call, Profile changed, edit/delete at message boundary, transient error, exhausted permanent error and an older ready Memory surviving a newer failure.

Assert editing User Message anchor `N` invalidates every ready Memory with `through_message_id >= N` in one update and leaves earlier nodes intact; deleting its paired Assistant uses that same User anchor rather than the Assistant ID. Planner never reads `failed` or `invalidated` rows and never treats empty Summary as success.

Add Repository tests that reject a preassigned ID for a new Memory, reject a
self-parent candidate, reject a parent from another Conversation/Profile or a
non-contiguous parent interval, and prove parent identity/range fields are
never updated after insert. MySQL 8.4 cannot put the self-parent comparison in
a CHECK because `id` is `AUTO_INCREMENT`; the locked Repository write path is
the executable owner of this invariant.

Add two exhausted-window tests. If `FinalizeExhausted` commits the terminal failed row and the process dies afterward, replay observes that exact unique identity and makes no Provider call. If the process dies before that row commits, no durable fact says the retry was exhausted: Reconciler re-enqueues the same Source Hash and the Provider may be called again. In both cases the unique key permits only one terminal row and no forked parent chain.

- [x] **Step 2: Run tests and verify RED**

Run: `go test ./internal/jobs ./internal/module/ai/contextengine ./internal/module/ai/message ./internal/runtime -run 'Memory|Watermark|ParentChain|Invalidat|ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: FAIL because Memory services/tasks do not exist.

- [x] **Step 3: Define the task and source identity**

```go
const TaskContextMemoryBuildV1 = "ai:context-memory-build:v1"

type ContextMemoryBuildV1 struct {
	ProfileID       uint64   `json:"profile_id"`
	ProfileSHA256   [32]byte `json:"profile_sha256"`
	ConversationID  uint64   `json:"conversation_id"`
	PreviousMemoryID *uint64 `json:"previous_memory_id"`
	FromMessageID   uint64   `json:"from_message_id"`
	ThroughMessageID uint64  `json:"through_message_id"`
	SourceSHA256    [32]byte `json:"source_sha256"`
	PolicyVersion   string   `json:"policy_version"`
}
```

Unique key includes every field. Source Hash covers Profile ID/Hash, parent ID/Summary Hash and ordered canonical Turn hashes. Summary prompt is fixed and distinguishes user claims, assistant answers, confirmed facts, unresolved matters and attachment references.

- [x] **Step 4: Implement no-long-transaction model calls and CAS commit**

Use the Profile's explicit Chat-kind Memory Provider Model and its own trusted window/output/counter for each bounded prefix. Do not use the current Agent Chat model implicitly and do not charge the user wallet. Before calling, snapshot parent/profile/turn hashes without a long transaction. After response, lock Conversation and revalidate latest parent, bounds and hashes; stale result is discarded and current task re-enqueued, never inserted as a fork.

The insert contract accepts only a zero candidate ID, derives the parent from
the locked latest valid chain, and writes parent identity and interval fields
once. It rejects self-parenting, cross-Conversation/Profile parents and gaps;
no update method may mutate those fields later.

Temporary network errors remain Asynq retries and do not insert failed rows. Permanent errors lock/revalidate the chain and insert a terminal failed Memory with NULL summary/hash and a safe code before returning `asynq.SkipRetry`. Register the Plan 02 `FinalizeExhausted` hook for `memory-build`: on the last retryable attempt it locks Conversation, revalidates Profile/parent/range/source hash, and inserts the same terminal failed shape. If that hook crashes before commit, Reconciler cannot infer exhaustion from the terminal-only Memory table; it re-enqueues the identical current Source Hash and a later handler may call the Provider again. If the failed row committed, the unique identity short-circuits replay. The unique key and parent/source revalidation prevent divergent terminal rows in either window. Ready insert stores usage and provider request ID for operations audit.

Register `memory-build` and extend Worker readiness from four to exactly all five delivered Context handlers. Do not add a Memory processing table, attempt column or Redis-derived business status merely to hide the honest at-least-once Provider-call window.

- [x] **Step 5: Run tests and commit**

Run: `go test ./internal/jobs ./internal/module/ai/contextengine ./internal/module/ai/message ./internal/runtime -run 'Memory|Watermark|ParentChain|Invalidat|ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: PASS.

```bash
git add -- internal/jobs/ai_context.go internal/jobs/ai_context_test.go internal/module/ai/contextengine/memory.go internal/module/ai/contextengine/memory_test.go internal/module/ai/contextengine/memory_repository.go internal/module/ai/contextengine/memory_repository_test.go internal/module/ai/contextengine/jobs.go internal/module/ai/contextengine/jobs_test.go internal/module/ai/contextengine/reconciler.go internal/module/ai/contextengine/reconciler_test.go internal/runtime/worker.go internal/runtime/worker_test.go
git commit -m "feat(ai): maintain rolling conversation memory"
```

### Task 6: Compose recent Turns, private recall, attachments and Memory in Planner

**Files:**
- Modify: `internal/module/ai/contextengine/planner.go`
- Modify: `internal/module/ai/contextengine/planner_test.go`
- Modify: `internal/module/ai/contextengine/retrieval.go`
- Modify: `internal/module/ai/contextengine/retrieval_test.go`
- Modify: `internal/module/ai/contextengine/dispatch_guard.go`
- Modify: `internal/module/ai/contextengine/dispatch_guard_test.go`
- Modify: `internal/module/ai/replycommand/finalization.go`
- Modify: `internal/module/ai/replycommand/finalization_test.go`

- [ ] **Step 1: Write failing precedence and private-recall tests**

Construct one Plan containing current file, newest Turn with image, Space Evidence, private attachment Evidence, valid Memory, direct older Turn and recalled old Turn. Assert exact design section 12.2 order, whole atomic groups, no duplicate Turn, and source hash/citation behavior. Profile selected with zero Space bindings must still use private Turns/Memory; Profile NULL uses automatic direct history only.

Test failed Memory is an async diagnostic and does not fail the current Run; failed Qdrant when actual private/Space sources are needed does fail. Missing optional private Point is backfill pending, while a returned stale/unauthorized Point is excluded and cleaned.

- [ ] **Step 2: Run Planner tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/replycommand -run 'ContextPrecedence|PrivateRecall|MemoryDiagnostic|HistoricalAttachment' -count=1`

Expected: FAIL because Planner does not yet compose these sources.

- [ ] **Step 3: Implement the one-source-of-turn composition**

Direct history, Conversation retrieval and Memory all consume the canonical `ConversationTurn` created in Plan 03 Task 1 and paged by this Plan's Task 1. Planner deduplicates direct/recalled Turns by Source Hash, chooses latest valid continuous Memory chain, and records every selected/excluded Block with closed reason. Memory covered Turns are not also selected directly unless a higher-priority recent Turn is deliberately outside the summarized prefix.

After visible completed/stopped finalization, enqueue Conversation Index and evaluate Memory watermark independently. Failure to enqueue either async enhancement records diagnostics but cannot roll back the already consistent finalizer.

- [ ] **Step 4: Recheck dispatch authority for private facts**

Dispatch Guard recomputes selected direct/recalled Turn hashes, attachment object facts and Memory parent validity. A historical edit/delete/replacement before dispatch returns snapshot/permission error; it does not compile a different history. A completed prepared request still retries from persisted bytes only after the same guard passes.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/replycommand ./internal/runtime -run 'ContextPrecedence|PrivateRecall|MemoryDiagnostic|HistoricalAttachment|DispatchGuard' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/planner.go internal/module/ai/contextengine/planner_test.go internal/module/ai/contextengine/retrieval.go internal/module/ai/contextengine/retrieval_test.go internal/module/ai/contextengine/dispatch_guard.go internal/module/ai/contextengine/dispatch_guard_test.go internal/module/ai/replycommand/finalization.go internal/module/ai/replycommand/finalization_test.go
git commit -m "feat(ai): compose long conversation context"
```

### Task 7: Verify continuity, refresh and terminal behavior

**Files:**
- Create: `internal/module/ai/replycommand/context_history_integration_test.go`
- Modify: `docs/architecture.md`

- [ ] **Step 1: Add the user-visible regression scenario**

Integration test sends an attachment turn, a later text-only turn that refers to it, then another normal message. For every turn, fake Provider completes, persisted Assistant Message is visible, Reply/Run/Attempt are terminal, and reloading messages/Run returns the same answer/Citation/Plan. Simulate process restart between completion and read; no in-memory stream state is required.

Add a case where finalizer completes but async Conversation Index/Memory enqueue initially fails. Refresh must still show completed chat and terminal Run; Reconciler later restores only the derived index/memory work.

- [ ] **Step 2: Run integration regressions**

Run: `go test ./internal/module/ai/replycommand ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/run ./internal/module/ai/contextengine -run 'ContextHistory|Refresh|Restart|AsyncBackfill' -count=1`

Expected: PASS; no Run remains `running` after a visible terminal Assistant response.

Run: `go test ./internal/runtime ./internal/jobs -run 'ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: PASS and Worker readiness now requires exactly all five delivered Context task types: document index, index cleanup, profile rebuild, conversation index, and memory build.

- [ ] **Step 3: Run static removal checks**

Run: `rg -n 'maxHistoryLimit|maxHistoryFromMeta|chatHistoryWithLimit|selectedChatContext|max_history' internal/module/ai/chat internal/module/ai/contextengine internal/infra/ai`

Expected: only compatibility DTO/parsing tests may mention `max_history`; no Planner, ChatInput, Packer, hash or compiler path does.

Run: `go list -deps ./internal/module/ai/message | rg 'internal/module/ai/contextengine'`

Expected: no matches; `message` owns the narrow transaction interface and production composition injects the Context implementation, so the dependency direction never reverses.

- [ ] **Step 4: Format, run focused suite and commit**

Run: `gofmt -w internal/module/ai/contextengine internal/module/ai/chat internal/module/ai/message internal/module/ai/replycommand internal/jobs internal/runtime internal/platform/admin`

Run: `go test ./internal/module/ai/contextengine/... ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/replycommand ./internal/module/ai/run ./internal/jobs ./internal/runtime ./internal/platform/admin -count=1`

Expected: PASS.

Run: `git diff --check`

```bash
git add -- internal/module/ai/replycommand/context_history_integration_test.go docs/architecture.md
git commit -m "test(ai): lock context conversation continuity"
```

- [ ] **Step 5: Record checkpoint**

Run: `git status --short`

Expected: clean. Do not restart `admin-dev`; do not use Playwright; the final frontend and browser acceptance remain Plan 05.
