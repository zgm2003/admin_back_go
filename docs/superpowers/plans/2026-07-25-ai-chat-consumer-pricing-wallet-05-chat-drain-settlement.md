# 聊天停止 Drain 与结算 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复聊天取消导致 usage 丢失的 P0 缺陷，并让普通重试、unknown、finalizer 重试和用户停止都回到同一 Run 级结算路径。

**Architecture:** HTTP/WS 只创建或取消 durable command；Worker lease 持有上游读取。delivery context 负责前端 delta，drain context 负责读取同一 provider stream。停止后 sink 变成 no-op，Runner 继续读 usage；流结束没有完整 usage 时释放，租约丢失则执行一次 `unknown + release`，都不自动重发。

**Tech Stack:** Go context/cause、Redis cancel pub/sub、task queue、GORM transactions、durable realtime events。

---

### Task 1: Make cancellation a durable request, not provider cancellation

**Files:**
- Modify: `internal/module/ai/replycommand/cancel.go`
- Modify: `internal/module/ai/replycommand/repository.go`
- Modify: `internal/module/ai/replycommand/model.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/realtime/event.go`
- Modify: `internal/module/realtime/event_test.go`
- Test: `internal/module/ai/replycommand/cancel_test.go`
- Test: `internal/module/ai/replycommand/cancel_integration_test.go`
- Test: `internal/module/ai/message/service_test.go`
- Test: `internal/module/ai/message/transport/admin/handler_test.go`

- [ ] **Step 1: Persist cancel intent**

`RequestCancel` records `cancel_requested_at` idempotently. A running command remains non-terminal while the same Worker drains the current stream. Pending/prepared commands may be canceled before dispatch and release their Hold.

- [ ] **Step 2: Enforce request fingerprint replay**

During `CreateReply`, calculate the shared typed fingerprint before inserting the user message/command. The same canonical `(user_id, request_id)` with an equal fingerprint returns the original command without a second message, Run or attempt; a different fingerprint returns the shared conflict error that the handler maps to HTTP `409`. `conversation_id` remains part of the fingerprint payload, but is not the database idempotency scope.

- [ ] **Step 3: Define state outcomes**

Use exactly: stop before dispatch -> Run `canceled`, Charge `released`, reason `released_before_dispatch`; dispatched + complete usage after a user stop -> Run `canceled`, Charge `settled`, reason `settled_complete_usage`; the same stream ends without complete usage -> Run `canceled`, Charge `unbilled`, reason `unbilled_usage_incomplete`, and release; connection/process/lease outcome cannot be known -> Run `outcome_unknown`, Charge `released`, reason `released_outcome_unknown`. The canonical Phase A state machine has no `uncertain` state.

The message Admin handler still maps synchronous request-fingerprint conflicts to HTTP `409`, but it must not pretend to return a Gateway error after its accepted `202` response. Extend durable `ai.response.failed.v1` with required `error_code` and nullable `wallet_path`/`recharge_path`. Every failed command copies its non-empty persisted `last_error_code` into the event in the same transaction; when the Worker cannot reserve before dispatch, atomically fail the command/Run, release with `released_insufficient_balance`, append `error_code=ai.billing.insufficient_balance` and the exact two paths, and make zero provider calls. Non-billing failures set both paths to explicit `null`; event creation rejects a blank code rather than inventing one from localized `msg`. Do not make chat service, Gateway or realtime payload validation depend on Gin or localized message text.

The cancel HTTP endpoint only acknowledges the durable intent and returns exact `status="stopping"`, including when the pre-dispatch path can finish quickly. It must never return the old false terminal `status="canceled"`; terminal truth comes from the durable canceled event after finalization.

- [ ] **Step 4: Test races and replay**

Test cancel-vs-complete row-lock order, repeated cancel, cancel before dispatch, cancel after dispatch, incomplete usage release, equal request replay, mismatched request `409`, insufficient funding as a durable typed failure with zero provider calls, and no assistant message after cancel wins. Run `go test ./internal/module/ai/replycommand -run 'Test.*Cancel|Test.*Release|Test.*Replay|Test.*Fingerprint|Test.*Balance'`.

### Task 2: Separate delivery and drain contexts

**Files:**
- Modify: `internal/module/ai/replycommand/runner.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/events.go`
- Create: `internal/module/ai/chat/drain_sink.go`
- Test: `internal/module/ai/replycommand/runner_test.go`
- Test: `internal/module/ai/chat/service_test.go`

- [ ] **Step 1: Build two contexts**

Create a bounded Worker-owned drain context from the task lease, and a delivery context that can be canceled by the user. Pass only the delivery context to the realtime publisher; pass only the drain context to `StreamChat`.

- [ ] **Step 2: Make canceled delivery a no-op**

After cancel wins, `Emit` drops delta events and never returns `infraai.ErrCanceled` to the provider. The provider stream continues until a terminal usage frame, explicit upstream failure or lease loss.

- [ ] **Step 3: Preserve durable completion**

Persist the normalized answer/tool-call candidate in the current attempt before finalization. The finalizer clears/discards that candidate after cancel wins but still records complete usage. On successful non-cancel, publish the assistant message and durable completion event only after finalizer commit; finalizer retry reads the stored candidate and never calls the provider again.

- [ ] **Step 4: Test usage after stop**

Use a fake engine that emits deltas, observes sink cancellation, then emits final usage. Assert no post-stop delta reaches publisher and the result contains usage for settlement. Run `go test ./internal/module/ai/chat -run 'Test.*Cancel|Test.*Usage|Test.*Stream'`.

### Task 3: Close ordinary retry and top-up lifecycle

**Files:**
- Modify: `internal/module/ai/replycommand/runner.go`
- Modify: `internal/module/ai/replycommand/reconciler.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/jobs.go`
- Test: `internal/module/ai/replycommand/runner_test.go`
- Test: `internal/module/ai/replycommand/reconciler_test.go`

- [ ] **Step 1: Reuse Run/Charge/Hold**

A retryable provider failure returns the command to `pending`, appends one idempotent `retry_scheduled` event and keeps the non-zero Hold. A new attempt gets a new `attempt_no`; no new Run, Charge or price snapshot is created. Complete usage from the failed attempt remains audit-only and is excluded from the eventual customer charge.

- [ ] **Step 2: Top-up before every dispatch**

Every initial call, ordinary retry and tool continuation uses one Gateway sequence. For a new attempt, load the immutable Run pricing configuration, assemble the complete outbound request, quote `prior complete billable amount + current conservative upper bound`, then call `ReserveAndPrepare`; that one transaction reserves/top-ups the existing Run Hold and inserts the new `prepared` attempt with the exact request body, request hash and quote evidence. Only after it commits may the Runner mark the attempt `dispatched` and invoke the provider. A quote or reserve/top-up error creates no attempt, dispatch or provider call. Initial funding failure releases with no charge. If a tool continuation top-up fails after complete succeeded attempts, the finalizer captures only those prior actual items and closes `Run failed + Charge settled`; failed retry attempts remain audit-only and therefore produce no charge. A retry never reuses an earlier attempt number for a new network call and never creates a second Run, Charge or pricing snapshot.

The Runner may read or conditionally claim a command/task before this sequence, but it must release every command/task row lock before entering Gateway quote/reserve/finalize transactions. The billing lock order is always `Run -> Charge -> wallet -> Hold`; no retry path may acquire a Hold first and then a wallet.

- [ ] **Step 3: Bound recovery**

Use the existing command lease as the execution boundary. When a lease expires with a dispatched attempt and no terminal evidence, one reconciler pass records `outcome_unknown + released`, clears the lease, and never accesses the upstream or creates another attempt. A `prepared`/not-dispatched attempt is recovered as the same attempt: `ReserveAndPrepare` idempotently revalidates its stored quote against the existing Hold, and dispatch uses its persisted exact request bytes and attempt key. Recovery must not reassemble, renumber or overwrite `prepared_request_json`, its hash or `quote_json`.

- [ ] **Step 4: Test retry invariants**

Cover retry while Hold remains active, first-attempt reserve insufficiency, continuation top-up insufficiency with prior succeeded usage captured, retry top-up insufficiency with only failed-attempt usage released, atomic reserve-and-prepare rollback, byte-identical prepared recovery without reassembly, max attempts final failure, dispatched lease expiry and no duplicate dispatch. Run `go test ./internal/module/ai/replycommand -run 'Test.*Retry|Test.*Reconcile|Test.*Lease|Test.*TopUp|Test.*Prepare'`.

### Task 4: Atomic finalizer and Run events

**Files:**
- Modify: `internal/module/ai/aigateway/finalizer.go`
- Modify: `internal/module/ai/run/recorder.go`
- Modify: `internal/module/ai/run/recorder_repository.go`
- Modify: `internal/module/ai/chat/repository.go`
- Test: `internal/module/ai/run/recorder_test.go`
- Test: `internal/module/ai/chat/repository_test.go`

- [ ] **Step 1: Add one transaction participant**

Expose a transaction callback entered without any command/task row lock. It acquires `Run -> Charge -> wallet -> Hold`, revalidates the expected attempt/business state, and only then invokes the chat participant to conditionally bind or discard the stored candidate. The same commit inserts immutable usage items after the once-rounded amount is known and writes Run state plus sequenced `ai_run_events`. For a user stop, that commit also appends `ai.response.canceled.v1`; no request handler or pre-finalizer path may publish the terminal event. A stale participant state aborts and retries the finalizer; it never reverses lock order or calls the provider.

- [ ] **Step 2: Keep result and finance states separate**

Allow `canceled + settled`, `canceled + released`, `outcome_unknown + released`, `canceled + unbilled`, `failed + unbilled`, and the specific funding-stop combination `failed + settled`; never infer billing status from command/Run status alone. A provider-failed Run follows `failed + released`; incomplete/over-Hold apparent success becomes `failed + unbilled` and never publishes its candidate. `failed + settled` is valid only when no new attempt was dispatched and earlier succeeded usage was complete.

- [ ] **Step 3: Test finalizer idempotency**

Run `go test ./internal/module/ai/run ./internal/module/ai/chat -run 'Test.*Finish|Test.*Event|Test.*Idempot'` and `git diff --check`.

- [ ] **Step 4: Commit**

```powershell
git add internal/module/ai/replycommand internal/module/ai/chat internal/module/ai/aigateway internal/module/ai/run
git commit -m "fix(ai): drain canceled chats before settlement"
```
