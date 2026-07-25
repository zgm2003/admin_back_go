# AI Gateway、Provider Adapter 与 Usage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立进程内 `aigateway`，把最终请求报价、Hold 预占、provider attempt 幂等、分类 usage 归一化和 finalizer 输入统一到一个边界。

**Architecture:** Gateway 不拥有 HTTP handler、不重复上游路由。付费动作接受事务先创建 Run/Charge 和不可变价格配置快照；Worker 用它组装并报价。Gateway 在同一 reserve/top-up 事务成功后才保存 exact prepared request/quote attempt，再执行 `prepared -> dispatched`；provider adapter 只使用现有 `base_url + api_key`。上游上下文不等于浏览器连接上下文。

**Tech Stack:** Go interfaces、OpenAI-compatible HTTP、JSON usage parsing、GORM repositories、Redis-independent provider calls。

---

### Task 1: Replace floating cost with classified usage

**Files:**
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/image.go`
- Modify: `internal/infra/ai/types_json_test.go`
- Modify: `internal/infra/ai/fake.go`
- Test: `internal/infra/ai/types_json_test.go`

- [ ] **Step 1: Define usage fields**

Replace `ChatResult.Cost float64` with `Usage UsageSnapshot`. `UsageSnapshot` contains `Status`, raw provider JSON and normalized items shaped as `{Category, Unit, TierKey, Quantity}` for input/output/cache-read/cache-write/media. Normalized categories are mutually exclusive: aggregate input that includes cached tokens must be decomposed into non-cached input plus documented cache subsets before it reaches pricing. A missing required item or an unprovable aggregate/subset relationship is unavailable, not zero; an explicitly reported zero quantity remains a real item.

- [ ] **Step 2: Preserve provider IDs**

Keep provider request/task/message IDs and add `DispatchState` (`not_dispatched|dispatched|unknown`). The adapter must return the original normalized usage and a stable response hash.

Define explicit capability metadata for each adapter: supported usage item keys, safe input upper-bound strategy, idempotency-header support, and optional task cancellation. Image/video cancellation is exposed only when the adapter implements a documented `CancelTask(ctx, taskID)` method; absence is a capability result, not an assumed cancellation.

- [ ] **Step 3: Test JSON semantics**

Reject negative counts, fractional token counts and total mismatch; allow zero actual usage only when the provider explicitly reports it. Run `go test ./internal/infra/ai -run 'Test.*Usage|Test.*JSON'`.

### Task 2: Implement Gateway interfaces and lifecycle

**Files:**
- Create: `internal/module/ai/aigateway/gateway.go`
- Create: `internal/module/ai/aigateway/contracts.go`
- Create: `internal/module/ai/aigateway/finalizer.go`
- Modify: `internal/shared/i18n/locales/zh-CN/wallet.yaml`
- Modify: `internal/shared/i18n/locales/en-US/wallet.yaml`
- Create: `internal/module/ai/aigateway/gateway_test.go`
- Create: `internal/module/ai/aigateway/finalizer_test.go`

- [ ] **Step 1: Define the public call contract**

Use explicit inputs:

```go
type RunRequest struct { UserID, RunID int64; RequestID string; RequestFingerprint [32]byte; Modality string; Input []byte }
type PreparedCall struct { RequestBody []byte; RequestSHA256 [32]byte; Quote QuoteEvidence }
type QuoteEvidence struct { PricingVersion string; EffectiveMaxOutputTokens int; UpperBoundItems []billing.UsageItem; TargetHoldUnits int64 }
type ReserveAndPrepareInput struct { RunID int64; AttemptNo uint32; NewCall *PreparedCall }
type ProviderAttempt struct { RunID int64; AttemptNo uint32; IdempotencyKey string; PreparedRequest []byte; Quote QuoteEvidence }
type DispatchResult struct { ProviderRequestID string; DispatchState string; TerminalState string; Usage infraai.UsageSnapshot; UsageComplete bool }
```

The interface exposes `AssembleAndQuote`, `ReserveAndPrepare`, `MarkDispatched`, `Dispatch`, `RecordOutcome`, and `Finalize`; handlers never call wallet or provider directly. `ReserveAndPrepareInput.NewCall != nil` means create the exact next attempt; `NewCall == nil` means restore the already persisted `(run_id,attempt_no)` and is invalid when that prepared row does not exist. Run acceptance has already selected and snapshotted effective `max_output_tokens`: omitted request value uses the agent ceiling, an explicit larger value is rejected, and Worker must use the exact snapshotted value for both `QuoteEvidence` and the serialized provider request.

- [ ] **Step 2: Enforce operation ordering**

For a new attempt, enforce `load immutable Run pricing config -> assemble exact final request -> quote -> reserve/top-up + create prepared attempt in one transaction -> mark dispatched -> provider`. For byte-level BPE models, the adapter may prove a conservative input ceiling from the complete serialized UTF-8 request payload plus documented framing overhead using the bound `tokens <= payload bytes + overhead`; other model families require a model-specific proven estimator. The serialized payload must already include system prompt, history, knowledge context, tool schemas and attachment metadata. `ReserveAndPrepare(NewCall != nil)` inserts the next attempt only after the wallet participant has accepted the target; a failed transaction creates neither Hold delta nor attempt. If the caller retries an uncertain transaction result with the same in-memory `NewCall`, an existing row is returned only after canonical request bytes, hash and typed quote evidence match exactly.

For durable recovery, first query the expected attempt state. When it is already `prepared`, skip `AssembleAndQuote` and call `ReserveAndPrepare(NewCall == nil)` to load its stored exact request, quote and attempt key while idempotently revalidating the active Hold. It must never renumber or overwrite prepared evidence. Provider dispatch always uses the returned stored body. Reject `MarkDispatched` unless pricing succeeded, the Hold is active/sufficient, the owner command/task is still runnable with no pre-existing cancel intent, and the attempt is `prepared`; reject provider invocation when no proven estimator exists, the fingerprint conflicts, balance is insufficient or top-up fails.

At paid-action acceptance, persist exact catalog version, `catalog_vendor`, OpenAI-compatible `transport_engine`, canonical model/alias resolution, rate rows, agent multiplier PPM and effective `max_output_tokens` in immutable `pricing_snapshot_json`; never infer catalog vendor from transport engine, provider display name or `base_url`. New Runs and their one Charge start with `billing_status=pending,billing_reason=pending`, and Worker never rereads mutable agent/catalog values for that Run. Per-attempt requested upper-bound items, target Hold units, effective output value and prepared-request hash belong in `quote_json` beside `prepared_request_json`, not in the Run snapshot.

`ReserveAndPrepare` owns one outer transaction, locks `Run -> Charge`, then invokes Plan 02's `ReserveHoldInTx`/`TopUpHoldInTx` on the same GORM transaction so wallet/Hold locks, the Run transition to `held,held`, `ai_usage_charges.held_units = wallet_holds.held_units = max(existing_hold_units,target_units)`, and insertion of the prepared attempt evidence commit together in that order. An equal/lower replay preserves the prior maximum. Charge `held_units` remains that audit maximum after the active Hold is captured/released; Run detail never derives historical `held_amount` from a terminal Hold whose current held units are zero. The wallet participant may not begin a nested transaction.

Insufficient reserve/top-up returns a typed, non-retryable error with stable code `ai.billing.insufficient_balance`. A synchronous HTTP transport that is waiting for the same durable task maps the stored terminal error to HTTP `409` with `response.ErrorWithData` and `data = {"wallet_path":"/profile/wallet","recharge_path":"/payment/recharge"}`; the Gateway itself must not import Gin or write HTTP. Chat/edit/regenerate have already returned `202`, so their Runner writes the same code and paths to the durable failure event instead of pretending an HTTP error can still be returned. The frontend branches on the machine code, never localized `msg`. Failure before the first call creates no attempt and finalizes Run `failed`, Charge `released`, reason `released_insufficient_balance`; a later tool/retry top-up failure creates no additional attempt at all.

- [ ] **Step 3: Implement Run-level finalizer**

Every cross-module billing transaction acquires rows in the exact order `Run -> Charge -> wallet -> Hold`; wallet-only operations use `wallet -> Hold`. The caller must not hold a reply-command, task, attempt or result-candidate row lock when entering this transaction. After the canonical locks are held, business-result participation uses conditional state checks/updates in the same transaction.

Determine usage completeness across the entire Run, not only the current attempt. A successful Run must have complete, legal, priceable usage for every succeeded attempt that contributes to the result. A user-stopped Run must additionally have complete usage for the dispatched in-flight attempt being stopped. Failed attempts always retain raw usage for audit, are excluded from charge items, and neither satisfy nor block the completeness of succeeded/stopped billable attempts. A top-up failure creates no attempt: if prior succeeded usage is complete, finalize the Run as `failed + settled` and capture only that prior actual amount; if there is no prior billable usage, use Run `failed`, Charge `released`, reason `released_insufficient_balance`; if prior chargeable usage is incomplete, use `failed + unbilled + unbilled_usage_incomplete`. Only when all chargeable attempts are complete may the finalizer quote the whole Run once, allocate item amounts deterministically, insert immutable items, capture actual units and release the difference. Every transition writes the exact Spec `billing_reason` in the same transaction: overall provider failure uses `failed + released + released_provider_failed`, `outcome_unknown` uses `outcome_unknown + released + released_outcome_unknown`, incomplete chargeable usage uses `unbilled_usage_incomplete`, and every valid capture including a top-up-stop capture uses `settled_complete_usage`. For that capture, build the bounded ledger summary only from persisted Run facts as `Agent #<agent_id> · <model_display_name-or-model_id>` and pass it to wallet; never include prompts, provider URLs, headers or credentials and never make wallet query AI tables. Incomplete/illegal usage produces `canceled + unbilled` for a user stop or `failed + unbilled` otherwise, releases the Hold and discards the result candidate. If actual exceeds Hold, use `unbilled_over_hold` with the same canceled-or-failed business-state rule, release, discard the candidate and record a platform billing anomaly. None of these paths creates an AI refund transaction.

- [ ] **Step 4: Test idempotency and money rules**

Test same request replay, same ID with different fingerprint (`409`), duplicate attempt key, immutable Run config despite later agent/catalog changes, effective output cap equality, reserve-before-dispatch, atomic reserve-and-prepare rollback, uncertain-transaction replay with byte-equal evidence, prepared recovery that skips assembly and returns the stored request/quote/key, recovery rejection for a missing prepared row, same-transaction `Run -> Charge -> wallet -> Hold -> new attempt` acquisition, Charge/Hold target equality with terminal Charge audit preservation, AI ledger source ID/summary with no private request material, initial insufficient balance with zero attempts/provider calls, later top-up failure with no new attempt and prior succeeded usage captured once, multi-attempt completeness, failed-attempt usage exclusion, stopped-current-attempt completeness, complete usage settlement once, incomplete usage release plus candidate discard, over-hold anomaly plus candidate discard, and no zero-value transaction. Run `go test ./internal/module/ai/aigateway`.

### Task 3: Adapt OpenAI-compatible chat/image providers

**Files:**
- Modify: `internal/infra/ai/openaicompat/client.go`
- Modify: `internal/infra/ai/imagecompat/client.go`
- Test: `internal/infra/ai/openaicompat/client_test.go`
- Test: `internal/infra/ai/imagecompat/client_test.go`

- [ ] **Step 1: Keep usage enabled for streams**

Continue sending `stream_options.include_usage=true`; parse the final usage frame even when the delivery sink becomes a no-op. Never use connection close as proof of zero usage.

Send the persisted provider-attempt key as `Idempotency-Key` on compatible upstream calls. A retry that represents a new provider call receives a new attempt number and key; replaying the same dispatched call reuses its stored key and never creates another request.

- [ ] **Step 2: Normalize cache fields**

Map provider-specific cached-token fields to `CacheRead`/`CacheWrite`. When a documented aggregate input includes those subsets, emit ordinary `InputTokens` as `aggregate - cache_read - cache_write` (or the exact provider-documented relation), reject negative/overlapping totals, and never emit the unchanged aggregate beside its subsets. If the provider cannot prove the relationship or distinguish required tiers, return `UsageStatusUnavailable` and let Gateway release rather than double-charge.

- [ ] **Step 3: Test provider fixtures**

Add fixtures for complete stream usage, missing final usage, aggregate input containing cached input, already-exclusive cache fields, overlapping/negative cache decomposition, image usage and malformed totals. Run `go test ./internal/infra/ai/openaicompat ./internal/infra/ai/imagecompat`.

### Task 4: Persist attempt evidence and provider keys

**Files:**
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/replycommand/attempt.go`
- Modify: `internal/module/ai/replycommand/repository.go`
- Test: `internal/module/ai/replycommand/attempt_integration_test.go`

- [ ] **Step 1: Move ownership to Run**

All attempt writes include `run_id`; `attempt_no` increments per Run. Preserve a nullable command correlation for chat display, but never use command ID as the uniqueness owner for media or tool attempts.

- [ ] **Step 2: Persist raw evidence once**

On prepare, store the exact credential-free provider request JSON/body, its SHA-256 and closed quote evidence together with the idempotency key; never persist Authorization/API key material. On outcome, store provider request ID, dispatch state, normalized usage JSON, response hash and terminal state before finalizer retries. A prepared replay validates and returns the stored request/quote; a terminal replay reads the stored result. Neither path calls the provider again merely to reconstruct evidence.

- [ ] **Step 3: Focused verification and commit**

Run `gofmt -w internal/infra/ai internal/module/ai/aigateway internal/module/ai/chat internal/module/ai/replycommand`, `go test ./internal/module/ai/aigateway ./internal/infra/ai/...`, and `git diff --check`.

```powershell
git add internal/infra/ai internal/module/ai/aigateway internal/module/ai/chat internal/module/ai/replycommand
git commit -m "feat(ai): add gateway usage and attempt evidence"
```
