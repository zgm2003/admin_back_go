# Admin Go Durable Work and Realtime Implementation Plan

> **Superseded delivery note (2026-07-18):** This completed plan's backend Workflow edits are historical evidence and must not be replayed. Web/backend verification and delivery now use repository scripts plus Docker Compose. The only allowed future Workflow is the P08.5 Windows Tauri candidate release defined by the execution index.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make AI replies, notification/export work, schedules, cancellation, and terminal realtime delivery durable and safe across API/Worker termination and multiple nodes.

**Architecture:** MySQL state machines are durability truth; Asynq is a wake-up/delivery adapter. Claims use leases, owners, and fencing tokens. Provider ambiguity is recorded before retry decisions. Redis handles cross-node signals and ephemeral fan-out, while durable terminal events resume from MySQL cursors.

**Tech Stack:** Go 1.26.5, MySQL 8.4, Redis, Asynq, gocron, WebSocket.

---

## Execution prerequisite and Docker-only runtime policy

P05 starts only after P02, P03.5, and Gate C.5 are complete. The standard state/app stack must be started with `pwsh -NoProfile -File scripts/docker-platform.ps1 up`; API, worker, Vite, MySQL, and Redis must never be started as host processes.

Pure source/unit checks may run on the host. Any test that opens MySQL/Redis, terminates/restarts API or worker, or performs realtime/integration/smoke/E2E behavior runs in Docker on `admin-platform`. Use the pinned Go image with the repository bind-mounted at `/src`, `--env-file deploy/docker-first/admin-go.env`, and named Go caches. PowerShell may orchestrate Docker and signals but may not substitute a host runtime.

## Target file map

- Deepen `internal/infra/taskqueue` with a typed TaskRegistry and Error Module retry mapping.
- Create `internal/module/ai/replycommand/*` and migration `202607150101_ai_reply_durability.sql`.
- Modify message/chat/run/OpenAI-compatible adapters to use durable commands and attempts.
- Replace `internal/bootstrap/ai_reply_dispatcher.go` with Worker handlers and reconciliation.
- Deepen `internal/infra/redislock` and `internal/infra/scheduler` with renewable fenced leases.
- Create `internal/module/crontask/reconciler.go`.
- Add claim leases/idempotency to notification and export repositories/jobs.
- Deepen `internal/module/realtime` and `internal/infra/realtime` for typed envelopes, subscriptions, cursors, recovery, and metrics.
- Regenerate `contracts/admin/v1/realtime/*` and bundle manifest.

### Task 1: Centralize executable tasks in a typed TaskRegistry

**Files:**
- Create: `internal/infra/taskqueue/registry.go`
- Create: `internal/infra/taskqueue/registry_test.go`
- Modify: `internal/infra/taskqueue/client.go`
- Modify: `internal/infra/taskqueue/server.go`
- Modify: `internal/jobs/noop.go`
- Modify: `internal/module/auth/jobs.go`
- Modify: `internal/module/notification/task/jobs.go`
- Modify: `internal/module/payment/jobs.go`
- Modify: `internal/module/export/jobs.go`
- Modify: `internal/module/ai/chat/jobs.go`
- Modify: `internal/module/ai/image/jobs.go`

- [x] **Step 1: Test duplicate types, payload failure, and retry mapping**

```go
func TestRegistryMapsPermanentFailureToSkipRetry(t *testing.T) {
	r := NewRegistry()
	err := r.Register(Definition{
		Type: "widget:run:v1", Queue: QueueDefault, Timeout: time.Minute,
		Decode: func([]byte) (any, *apperror.Error) { return nil, apperror.BadRequest("bad payload").WithCode("task.payload_invalid") },
		Handle: func(context.Context, any) *apperror.Error { return nil },
	})
	if err != nil { t.Fatal(err) }
	got := r.Handle(context.Background(), Task{Type: "widget:run:v1", Payload: []byte("{")})
	if !errors.Is(got, asynq.SkipRetry) { t.Fatalf("err=%v", got) }
}
```

- [x] **Step 2: Define complete task ownership**

```go
type Definition struct {
	Type string
	Queue string
	Timeout time.Duration
	MaxRetry int
	UniqueTTL time.Duration
	Decode func([]byte) (any, *apperror.Error)
	Handle func(context.Context, any) *apperror.Error
}
type Registry struct { definitions map[string]Definition }
```

Registration rejects empty/duplicate/unversioned types, unknown lanes, non-positive timeouts, and nil decode/handle. Enqueue accepts a registered type and applies its definition; producers no longer repeat retry/queue/timeout policy.

- [x] **Step 3: Migrate every current task definition**

Register `system:no-op:v1`, `auth:login-log:v1`, notification dispatch/send, payment sync/close, export run, AI run timeout, and AI image generation through one Worker registry. Invalid payload/invariant errors are Permanent; dependency/timeout errors are Retryable; unknown errors become bounded Retryable internal errors.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/infra/taskqueue ./internal/jobs ./internal/module/auth ./internal/module/notification/task ./internal/module/payment ./internal/module/export ./internal/module/ai/chat ./internal/module/ai/image -run 'TestRegistry|TestTask|TestPayload' -count=1
git add -- internal/infra/taskqueue internal/jobs/noop.go internal/module/auth/jobs.go internal/module/notification/task/jobs.go internal/module/payment/jobs.go internal/module/export/jobs.go internal/module/ai/chat/jobs.go internal/module/ai/image/jobs.go
git commit -m "refactor(queue): centralize versioned task policy"
```

### Task 2: Commit user messages and reply commands atomically

**Files:**
- Create: `database/migrations/202607150101_ai_reply_durability.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/schema/admin.hcl`
- Create: `internal/module/ai/replycommand/model.go`
- Create: `internal/module/ai/replycommand/repository.go`
- Create: `internal/module/ai/replycommand/repository_test.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/message/dto.go`

- [x] **Step 1: Write rollback/idempotency integration tests**

Test message insert failure, command insert failure, duplicate `(conversation_id,request_id)`, and duplicate delivery. A failure leaves neither row; a duplicate returns the original message/command IDs.

- [x] **Step 2: Add assistant publication identity**

The Atlas migration adds nullable `ai_messages.reply_command_id BIGINT UNSIGNED` and unique `uk_ai_messages_reply_command`. It does not alter historical messages. Validate/hash Atlas before applying to a disposable schema.

- [x] **Step 3: Implement one transaction**

```go
type CreateReplyInput struct {
	ConversationID int64
	UserID int64
	RequestID string
	Content string
	MetaJSON *string
}
type CreateReplyResult struct {
	UserMessageID int64
	CommandID uint64
	RequestID string
	State string
}
func (r *GormRepository) CreateReply(ctx context.Context, in CreateReplyInput) (CreateReplyResult, error)
```

Lock the owned active conversation, return the existing command on duplicate request, insert the user message, insert `ai_reply_commands` with idempotency key `sha256("admin-reply:"+conversationID+":"+requestID)`, update conversation last-message time, and commit. Enqueue `ai:reply-command:v1` after commit as best effort; the Worker poller is the recovery path.

- [x] **Step 4: Remove process-local dispatch**

Replace `WithReplyEnqueuer` with the durable repository. HTTP returns `202` data containing command ID, request ID, and `pending`. Delete goroutine dispatch construction from API runtime only after tests pass.

- [x] **Step 5: Verify and commit**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
go test ./internal/module/ai/message ./internal/module/ai/replycommand -race -count=1
git add -- database/migrations/202607150101_ai_reply_durability.sql database/migrations/atlas.sum database/schema/admin.hcl internal/module/ai/replycommand/model.go internal/module/ai/replycommand/repository.go internal/module/ai/replycommand/repository_test.go internal/module/ai/message/repository.go internal/module/ai/message/service.go internal/module/ai/message/service_test.go internal/module/ai/message/dto.go
git diff --cached --check
git commit -m "feat(ai): commit messages with durable reply commands"
```

### Task 3: Claim, renew, and fence reply work

**Files:**
- Create: `internal/module/ai/replycommand/service.go`
- Create: `internal/module/ai/replycommand/runner.go`
- Create: `internal/module/ai/replycommand/jobs.go`
- Create: `internal/module/ai/replycommand/runner_integration_test.go`
- Modify: `internal/runtime/worker.go`
- Delete: `internal/bootstrap/ai_reply_dispatcher.go`
- Delete: `internal/bootstrap/ai_reply_dispatcher_test.go`

- [x] **Step 1: Write stale-owner and restart tests**

Worker A claims token 1 and stops. After expiry Worker B claims token 2. Assert A cannot mark running, publish an assistant message, terminalize, or renew; B can. Stop API immediately after HTTP commit, start Worker, and assert completion.

- [x] **Step 2: Implement the state machine**

```go
type Claim struct {
	Command Command
	Owner string
	FencingToken uint64
	LeaseExpiresAt time.Time
}
type Repository interface {
	ClaimNext(context.Context, string, time.Time, time.Duration) (*Claim, error)
	Renew(context.Context, uint64, string, uint64, time.Time) (bool, error)
	Transition(context.Context, uint64, string, uint64, State, State, map[string]any) (bool, error)
	RequestCancel(context.Context, int64, int64, string, time.Time) (*Command, error)
}
```

Claim uses `SELECT ... FOR UPDATE SKIP LOCKED` over pending or expired claimed/running rows ordered by `next_attempt_at,id`, increments `lease_token`, and commits. Renew every one-third lease duration. Every state write includes owner and token. Lease loss cancels provider context and prevents publication.

- [x] **Step 3: Add wake-up plus polling**

`ai:reply-command:v1` payload contains only `command_id`. The Worker also polls every second so DB-committed work survives failed Redis enqueue. Duplicate wake-ups call the same claim/transition code.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/ai/replycommand ./internal/runtime -run 'TestClaim|TestLease|TestRestart|TestDuplicate' -race -count=10
git add -- internal/module/ai/replycommand internal/runtime/worker.go
git rm -- internal/bootstrap/ai_reply_dispatcher.go internal/bootstrap/ai_reply_dispatcher_test.go
git commit -m "feat(ai): execute reply commands with fenced leases"
```

### Task 4: Persist provider attempts and reconcile ambiguity

**Files:**
- Create: `internal/module/ai/replycommand/attempt.go`
- Create: `internal/module/ai/replycommand/reconciler.go`
- Create: `internal/module/ai/replycommand/reconciler_test.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/run/recorder.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Modify: `internal/infra/ai/openaicompat/client_test.go`

- [x] **Step 1: Test pre-dispatch, ambiguous, and known failures**

Assert the attempt row exists before the HTTP request; connection failure before headers returns to retryable pending; timeout/disconnect after headers becomes `outcome_unknown`; a persisted assistant result reconciles to succeeded; an OpenAI-compatible ambiguous attempt with no lookup/result reconciles to terminal failed and is never blindly sent again.

- [x] **Step 2: Persist attempt identity**

```go
type Attempt struct {
	ID uint64
	CommandID uint64
	AttemptNo uint
	IdempotencyKey string
	State AttemptState
	ProviderRequestID string
	ResponseSHA256 string
	DispatchedAt *time.Time
	FinishedAt *time.Time
}
```

Before dispatch, insert `prepared` with key `sha256("ai-provider:"+commandID+":"+attemptNo)`. Send `Idempotency-Key` when the compatible provider supports it. Mark `dispatched` immediately before network I/O. Preserve run recorder cause/category and link run context to command/attempt.

- [x] **Step 3: Implement explicit reconciliation**

Rules:

- no headers/bytes observed: failed attempt may schedule bounded retry;
- provider lookup proves a result: persist it idempotently and succeed;
- local assistant row exists for command: succeed without provider call;
- provider lookup proves rejection: fail;
- provider cannot query an acknowledged/stream-started attempt: fail with `ai.provider_outcome_unknown`, retain evidence, and require explicit operator retry as a new request.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/ai/replycommand ./internal/module/ai/chat ./internal/module/ai/run ./internal/infra/ai/openaicompat -run 'TestAttempt|TestOutcome|TestIdempotency' -count=1
git add -- internal/module/ai/replycommand/attempt.go internal/module/ai/replycommand/reconciler.go internal/module/ai/replycommand/reconciler_test.go internal/module/ai/chat/service.go internal/module/ai/run/recorder.go internal/infra/ai/openaicompat/client.go internal/infra/ai/openaicompat/client_test.go
git commit -m "feat(ai): reconcile ambiguous provider attempts safely"
```

### Task 5: Make cancellation and assistant publication cross-node idempotent

**Files:**
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/replycommand/service.go`
- Modify: `internal/module/ai/replycommand/runner.go`
- Create: `internal/module/ai/replycommand/cancel.go`
- Create: `internal/module/ai/replycommand/cancel_integration_test.go`

- [x] **Step 1: Write cross-node cancel and duplicate-result tests**

Worker A runs a command; API node B requests cancel. Assert durable `cancel_requested_at` is set, Redis signal cancels A promptly, terminal state is canceled, and a second cancel is idempotent. Deliver the same provider result twice and assert one assistant message linked by `reply_command_id`.

- [x] **Step 2: Implement durable-first cancellation**

Verify conversation ownership, update the matching non-terminal command in MySQL, commit, then publish `ai:reply:cancel:{commandID}`. Worker subscribes for latency but also checks the DB flag on each lease renewal. Redis loss cannot lose cancellation intent.

- [x] **Step 3: Publish assistant result under fencing**

In one transaction, verify current owner/token and running state, insert assistant message with unique `reply_command_id`, update conversation, link command `assistant_message_id`, and transition to succeeded. Unique-key conflict reloads and returns the existing result. A stale owner affects zero rows and publishes no terminal event.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/ai/message ./internal/module/ai/replycommand -run 'TestCancel|TestDuplicateResult|TestStaleOwner' -race -count=10
git add -- internal/module/ai/message/service.go internal/module/ai/message/repository.go internal/module/ai/replycommand
git commit -m "fix(ai): make cancellation and reply publication idempotent"
```

### Task 6: Add renewable scheduler leases and live reconciliation

**Files:**
- Modify: `internal/infra/redislock/redislock.go`
- Modify: `internal/infra/redislock/redislock_test.go`
- Modify: `internal/infra/scheduler/scheduler.go`
- Modify: `internal/infra/scheduler/scheduler_test.go`
- Create: `internal/module/crontask/reconciler.go`
- Create: `internal/module/crontask/reconciler_test.go`
- Modify: `internal/module/crontask/scheduler_service.go`
- Modify: `internal/runtime/worker.go`

- [x] **Step 1: Test lease loss and five-second convergence**

Run two schedulers against one Redis. A long callback renews beyond three TTLs and executes once. Force renewal failure and assert its fencing check prevents enqueue. Create/update/disable/delete a DB schedule and require running jobs to match within five seconds. An unknown enabled task makes health unhealthy.

- [x] **Step 2: Deepen the lease contract**

```go
type Lease struct { Key string; Owner string; Token uint64; ExpiresAt time.Time }
type LeaseStore interface {
	Acquire(context.Context, string, string, time.Duration) (Lease, error)
	Renew(context.Context, Lease, time.Duration) (Lease, error)
	Release(context.Context, Lease) error
}
```

Redis Lua uses an incrementing fencing counter and a hash containing owner/token. Renew/release compare both. Scheduler wraps callbacks with a renewal goroutine and cancels callback context on lease loss.

- [x] **Step 3: Reconcile database schedules**

`Reconciler` polls at two seconds, validates every enabled row against TaskRegistry and cron syntax, diffs by name+expression+status+updated time, and add/update/remove jobs through an explicit scheduler API. It records last success/error and exposes unhealthy state for unknown tasks, invalid cron, or repository failure.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/infra/redislock ./internal/infra/scheduler ./internal/module/crontask ./internal/runtime -run 'TestLease|TestReconcile|TestMultiWorker' -race -count=10
git add -- internal/infra/redislock internal/infra/scheduler internal/module/crontask/reconciler.go internal/module/crontask/reconciler_test.go internal/module/crontask/scheduler_service.go internal/runtime/worker.go
git commit -m "feat(scheduler): renew fenced leases and reconcile live schedules"
```

### Task 7: Lease notification/export work and remove list-side cleanup

**Files:**
- Modify: `internal/module/notification/task/repository.go`
- Modify: `internal/module/notification/task/jobs.go`
- Modify: `internal/module/notification/task/jobs_test.go`
- Modify: `internal/module/export/repository.go`
- Modify: `internal/module/export/jobs.go`
- Modify: `internal/module/export/run_test.go`
- Modify: `internal/module/export/service.go`

- [x] **Step 1: Write duplicate and lease-expiry tests**

Two Workers claim the same due notification/export task; only one wins. Expired work is reclaimed with a higher token. A stale owner cannot mark success. Duplicate notification delivery creates one `(source_task_id,user_id)` row. Duplicate export publication uses one deterministic object key and one terminal update.

- [x] **Step 2: Implement claims**

Notification and export repositories expose `ClaimNext(owner,now,ttl)`, `Renew(id,owner,token)`, and fenced terminal transitions. Notification bulk insert includes `source_task_id` and uses the unique key. Export object key is `exports/{yyyyMMdd}/{taskID}-{artifactVersion}.xlsx`; upload retry overwrites the same key and success update is fenced.

- [x] **Step 3: Move expiration cleanup to Worker**

Register `export:cleanup-expired:v1` in TaskRegistry and the cron registry. Remove `CleanExpired` from list/count service methods; reading an export page performs no cleanup write.

- [x] **Step 4: Verify and commit**

```powershell
go test ./internal/module/notification/task ./internal/module/export -run 'TestClaim|TestLease|TestDuplicate|TestCleanup' -race -count=10
git add -- internal/module/notification/task/repository.go internal/module/notification/task/jobs.go internal/module/notification/task/jobs_test.go internal/module/export/repository.go internal/module/export/jobs.go internal/module/export/run_test.go internal/module/export/service.go
git commit -m "fix(work): lease notification and export execution"
```

### Task 8: Deepen typed realtime delivery and recovery

#### Confirmed supplemental contract (2026-07-18)

The following decisions are part of the formal P05 contract and are not implementation fallbacks:

- Durable realtime events are retained for exactly **7 days**. Every durable row stores `expires_at = occurred_at + 7 days`.
- `realtime.resync_required.v1` is a server-only Ephemeral event (`sequence = 0`) with the exact payload `{"latest_sequence": 123}`.
- `ai.response.canceled.v1` is a server-only Durable event with the exact payload `{"conversation_id": 1, "request_id": "..."}`.
- `request_id` is non-empty where the event/domain contract requires it and is at most **128 Unicode characters** end to end: HTTP validation, WebSocket envelope/payload, repositories, MySQL columns, reconciliation schema, and generated contract.
- Retention progress is stored per target in `realtime_event_retention_watermarks(target_type,target_id,deleted_through_sequence,updated_at)` with a unique `(target_type,target_id)` key.
- Resume requires resync exactly when `after_sequence < deleted_through_sequence`. The returned `latest_sequence` is `max(current target event maximum sequence, deleted_through_sequence)`.
- Expired-event deletion and advancement of `deleted_through_sequence` happen in the same MySQL transaction. A cleanup rollback changes neither events nor watermark.
- Redis remains best-effort live fan-out only. MySQL events plus the watermark are the durable recovery truth.

**Files:**
- Modify: `internal/infra/realtime/envelope.go`
- Modify: `internal/infra/realtime/envelope_test.go`
- Modify: `internal/infra/realtime/session.go`
- Modify: `internal/infra/realtime/manager.go`
- Create: `internal/module/realtime/event.go`
- Create: `internal/module/realtime/repository.go`
- Modify: `internal/module/realtime/service.go`
- Modify: `internal/module/realtime/service_test.go`
- Modify: `internal/module/realtime/transport/admin/handler.go`
- Modify: `internal/module/notification/task/jobs.go`
- Modify: `internal/module/ai/replycommand/runner.go`
- Modify: `contracts/admin/v1/realtime/envelope.schema.json`
- Modify: `contracts/admin/v1/realtime/events.schema.json`
- Modify: `contracts/admin/v1/manifest.json`

- [x] **Step 1: Write protocol and recovery tests**

Cover handshake, real subscription filtering, monotonic cursors, duplicate IDs, ordered durable replay, expired cursor resync, malformed/unknown event rejection, handler isolation, slow-client disconnect, Redis outage, and two API instances.

- [x] **Step 2: Close the envelope**

```go
type Durability string
const (Ephemeral Durability = "ephemeral"; Durable Durability = "durable")
type Envelope struct {
	EventID string `json:"event_id"`
	Type string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Sequence uint64 `json:"sequence"`
	OccurredAt time.Time `json:"occurred_at"`
	Durability Durability `json:"durability"`
	Data json.RawMessage `json:"data"`
}
```

Generate ULID-compatible 26-character IDs without external state. All event types are registered with a payload encoder/decoder; unknown names fail.

- [x] **Step 3: Make subscription state effective**

`Session` owns a synchronized topic set. `realtime.subscribe.v1` replaces or increments allowed topics according to the request schema, and `Manager.Send` filters by actual subscription. A handler panic/error is recovered, counted, and does not stop other handlers or the read pump.

- [x] **Step 4: Persist and resume terminal events**

Notification creation and AI completed/failed/canceled transitions insert a `realtime_events` durable row in the same domain transaction. After commit, publish to Redis best effort. On `realtime.resume.v1 {after_sequence}`, query authorized user events in sequence order with limit 500. If the cursor predates retained truth, return `realtime.resync_required.v1`; the client then calls the authoritative domain query.

Typing/progress/delta are Ephemeral with sequence 0 and are never persisted. A daily Worker task deletes expired durable events only after the documented retention window.

- [x] **Step 5: Verify and regenerate contracts**

```powershell
go test ./internal/infra/realtime ./internal/module/realtime ./internal/module/notification/task ./internal/module/ai/replycommand -run 'TestEnvelope|TestSubscription|TestResume|TestSlow|TestMultiNode' -race -count=10
pwsh -NoProfile -File scripts/generate-admin-contract.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git add -- internal/infra/realtime internal/module/realtime internal/module/notification/task/jobs.go internal/module/ai/replycommand/runner.go contracts/admin/v1/realtime contracts/admin/v1/manifest.json
git commit -m "feat(realtime): resume durable terminal events by cursor"
```

### Task 9: Prove termination, recovery, and multi-node behavior

**Files:**
- Create: `scripts/tests/durable-work-restart.tests.ps1`
- Create: `scripts/verify-durable-work.ps1`
- Create: `internal/architecture/durable_work_test.go`
- Modify: `scripts/verify-backend.ps1`
- Modify: `.github/workflows/verify-backend.yml`
- Modify: `docs/architecture.md`

- [x] **Step 1: Add architecture guards**

Reject process-local AI reply goroutines/maps, fixed non-renewing scheduler locks, task handlers outside TaskRegistry, unfenced terminal updates, Redis as durable terminal truth, and free-form realtime event types.

- [x] **Step 2: Automate kill/restart scenarios**

The PowerShell test:

1. sends an AI message and kills API after commit;
2. starts Worker and observes completion;
3. kills Worker after claim and before terminal write;
4. waits lease expiry and observes a second Worker recover;
5. cancels from another API node;
6. disconnects realtime, completes work, reconnects with cursor, and observes one terminal result;
7. verifies no duplicate assistant, notification, export artifact, or provider attempt.

Use fake provider endpoints with deterministic pause points; never call a paid provider.

- [x] **Step 3: Run the complete gate**

```powershell
pwsh -NoProfile -File scripts/tests/durable-work-restart.tests.ps1
pwsh -NoProfile -File scripts/verify-durable-work.ps1
pwsh -NoProfile -File scripts/verify-backend.ps1
```

- [x] **Step 4: Commit**

```powershell
git add -- scripts/tests/durable-work-restart.tests.ps1 scripts/verify-durable-work.ps1 internal/architecture/durable_work_test.go scripts/verify-backend.ps1 .github/workflows/verify-backend.yml docs/architecture.md
git commit -m "ci: prove durable work and realtime recovery"
```

## Plan completion gate

```powershell
pwsh -NoProfile -File scripts/verify-durable-work.ps1
pwsh -NoProfile -File scripts/tests/durable-work-restart.tests.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git status --short
```

Expected: API/Worker termination loses no committed reply; stale owners cannot publish; ambiguous provider attempts never retry blindly; schedule changes converge within five seconds; duplicate notification/export/assistant results are prevented; durable terminal events resume once by cursor; status is clean.

## Completion evidence (2026-07-18)

- P05 was executed directly on backend `master` by explicit operator instruction; no P05 worktree was created or used.
- Tasks 1–7 landed as `ca84c81`, `58f34be`, `41dbc45`, `a7f350b`, `e690aed`, and `8a73dc8`; Task 8 landed as `8458ecf`.
- `scripts/tests/durable-work-restart.tests.ps1` exited `0` and proved API termination after commit, Worker lease-expiry recovery, cross-node cancellation, cursor resume, and absence of duplicate assistant/notification/export/provider-attempt results against disposable Docker MySQL/Redis nodes.
- `scripts/verify-durable-work.ps1 -SkipRestartScenario` exited `0`; the race suites, durable-work architecture guard, realtime contract tests, Atlas validation, contract drift check, and API/Worker builds passed. The restart scenario was run separately immediately before it.
- `scripts/verify-backend.ps1` exited `0`; full repository tests, Linux race gates, identity/runtime contracts, the complete durable restart scenario, vet, pinned staticcheck, govulncheck, and both process builds passed. Govulncheck found `0` called vulnerabilities.
- `scripts/verify-database.ps1 -Mode all` exited `0`; empty and reconciled imported schemas converged to SHA-256 `50e7642abe6f615167ab0fc64e3bd4aa765c0dc8695d2d4a2fc515365bc713cb`, all 8 reconciliation scripts applied once and skipped on repeat, and invariants/Admin smoke passed.
- The generated Admin contract manifest is locked to Task 8 commit `8458ecfc671f558af65a6f89c590891253179cdc`; Docker contract generation/check reported no drift.
- Live Docker MySQL received `044_realtime_retention.sql`; `realtime_events.expires_at` is non-null `datetime(6)`, request IDs are bounded to `varchar(128)`, the retention watermark table exists, and the enabled `realtime:cleanup-expired:v1` daily cron row is present.
