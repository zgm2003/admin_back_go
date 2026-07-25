# 文本、工具与媒体结算 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把文本、工具草稿、图片、视频和音频入口统一到同一 Run/Gateway/钱包事实，同时遵守各上游可取消能力和 usage 完整性要求。

**Architecture:** 所有付费入口先持久化 `request_id`、Run 和业务任务，再由 durable Worker 调用 Gateway；供应商返回后先把结果保存为候选，finalizer 同事务决定发布或丢弃。图片/视频仅调用上游文档化取消 API；本地停止不伪造上游取消。音频当前没有权威 usage 时 fail closed，不猜价格、不免费放行。结果保存不可变、用户删除不退款。

**Tech Stack:** Go task queue、GORM、OpenAI-compatible image/video/audio adapters、object storage references、Gateway public interfaces。

---

### Task 1: Make non-chat requests idempotent and durable

**Files:**
- Modify: `internal/module/ai/text/store.go`
- Modify: `internal/module/ai/tool/dto.go`
- Modify: `internal/module/ai/tool/service.go`
- Modify: `internal/module/ai/image/dto.go`
- Modify: `internal/module/ai/image/model.go`
- Modify: `internal/module/ai/image/service.go`
- Modify: `internal/module/ai/video/dto.go`
- Modify: `internal/module/ai/video/model.go`
- Modify: `internal/module/ai/video/service.go`
- Modify: `internal/module/ai/audio/service.go`
- Create: `internal/module/ai/audio/model.go`
- Modify: `internal/module/ai/audio/repository.go`
- Create: `internal/module/ai/audio/jobs.go`
- Test: `internal/module/ai/tool/service_test.go`
- Modify: `internal/module/ai/tool/transport/admin/request.go`
- Modify: `internal/module/ai/tool/transport/admin/handler.go`
- Modify: `internal/module/ai/tool/transport/admin/route.go`
- Create: `internal/module/ai/tool/transport/admin/handler_test.go`
- Test: `internal/module/ai/image/service_test.go`
- Test: `internal/module/ai/video/service_test.go`
- Test: `internal/module/ai/audio/service_test.go`

- [ ] **Step 1: Require client request IDs**

Reject blank `request_id`; store a binary fingerprint of the normalized request, model, agent, attachments and options. Same ID/same fingerprint returns the original task or terminal result; same ID/different fingerprint returns `409`.

- [ ] **Step 2: Persist result candidates**

Never keep paid audio bytes only in memory. The submission transaction creates the task without a result. After provider success, store an immutable `(storage_provider, storage_key)` candidate in `ai_audio_tasks` before finalizer commit; finalizer may discard an unbilled candidate but cannot rewrite a settled result. Text and tool drafts reuse durable `ai_text_tasks` with distinct `kind` values and likewise store their answer/draft only as a post-provider candidate until settlement.

- [ ] **Step 3: Test replay**

Run package tests that issue the same request twice and assert one Run, one attempt and one provider call. Assert a fingerprint mismatch is rejected before provider dispatch.

- [ ] **Step 4: Map paid tool HTTP errors**

Require `request_id` on tool-draft generation at the Admin request boundary. The handler submits one durable `ai_text_tasks(kind=tool_draft)` task and waits for that same task's terminal result; its HTTP context owns only the wait and never the Worker/provider context. A stored insufficient-balance terminal maps to HTTP `409` with exact `wallet_path`/`recharge_path` via `response.ErrorWithData`; a disconnected retry with the same ID waits for or returns the same task/result. Preserve the existing successful draft response shape and distinct configuration/price machine codes. Handlers never call wallet or provider directly.

### Task 2: Integrate text and tool generation with Gateway

**Files:**
- Create: `internal/module/ai/text/service.go`
- Create: `internal/module/ai/text/jobs.go`
- Modify: `internal/module/ai/tool/executor.go`
- Modify: `internal/module/ai/tool/service.go`
- Modify: `internal/module/ai/text/store.go`
- Test: `internal/module/ai/tool/service_test.go`
- Create: `internal/module/ai/text/service_test.go`

- [ ] **Step 1: Remove direct provider calls**

Build the exact final text/tool request first and ask Gateway to quote it from the immutable Run pricing configuration. Call `ReserveAndPrepare` so reserve/top-up and insertion of the new attempt's exact request, hash and quote evidence commit atomically before `MarkDispatched`; business modules never call the wallet Hold participant directly. Every tool round uses the same Run and targets `prior complete billable amount + current conservative upper bound`. If a continuation top-up fails, the transaction creates no attempt/provider call and the finalizer captures only complete prior succeeded usage; a first-call failure releases without charge. Recovering an existing `prepared` attempt must dispatch its stored request and quote with the same attempt key, never rebuild or overwrite them.

- [ ] **Step 2: Keep audio/text failure closed**

When the provider does not return all required usage categories, finalize as released/unbilled and do not publish a billable result. Do not estimate from character count unless the catalog explicitly defines that media unit.

- [ ] **Step 3: Test no-provider paths**

Cover missing price, unsafe prompt upper bound, insufficient balance and missing usage. Run `go test ./internal/module/ai/tool ./internal/module/ai/text`.

### Task 3: Integrate image and video task cancellation

**Files:**
- Modify: `internal/module/ai/image/service.go`
- Modify: `internal/module/ai/image/repository.go`
- Modify: `internal/module/ai/video/service.go`
- Modify: `internal/module/ai/video/repository.go`
- Modify: `internal/infra/ai/imagecompat/client.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Test: `internal/module/ai/image/jobs_test.go`
- Test: `internal/module/ai/video/service_test.go`

- [ ] **Step 1: Freeze determined media upper bounds**

Serialize the exact final media request and use its final `n`, duration, resolution, character or other catalog-supported fields to calculate the conservative upper bound. Call Gateway `ReserveAndPrepare`; only its successful atomic reserve/top-up plus prepared-attempt commit permits `MarkDispatched` and the provider call. If the unit cannot be determined or reserve/top-up fails, create no attempt and do not call the provider. A recovered prepared media attempt reuses its stored request bytes, quote and attempt key.

- [ ] **Step 2: Use only documented upstream cancel**

For image/video adapters that expose a documented task cancel method, invoke it and poll the same task ID to a terminal authoritative usage state. Otherwise stop delivery/local waiting but let the current durable Worker continue the same provider call/task polling while it still owns the lease. If that lease is lost without terminal usage, finalize once as `outcome_unknown + released`; never issue a replacement provider request.

- [ ] **Step 3: Preserve immutable storage identity**

Save the immutable `(storage_provider, storage_key)` tuple and provider task/request IDs. A successful settled result has no TTL. This phase does not change existing object-retention cleanup policy; any user deletion or permanent storage damage only changes business availability and never creates an AI refund or regeneration call.

- [ ] **Step 4: Test cancel and usage matrix**

Test documented cancel + complete usage, cancel + missing usage, local-only stop, reserve-and-prepare rollback with zero attempts/provider calls, byte-identical prepared recovery, duplicate task polling and immutable result replay. Run `go test ./internal/module/ai/image ./internal/module/ai/video -run 'Test.*Cancel|Test.*Usage|Test.*Replay|Test.*Prepare'`.

### Task 4: Fail closed for audio usage

**Files:**
- Modify: `internal/module/ai/audio/service.go`
- Test: `internal/module/ai/audio/service_test.go`

- [ ] **Step 1: Require authoritative audio units**

At adapter capability/preflight time, if the selected audio adapter cannot report the catalog-required seconds/characters/requests or token categories, return a stable “暂不可计费” error before quote/reserve/dispatch. Do not set usage to zero and do not call the provider.

- [ ] **Step 2: Test capability rejection before dispatch**

Use an adapter whose declared capability lacks the required audio unit. Assert the Gateway rejects before reserve/dispatched state and the provider fake has exactly zero calls.

- [ ] **Step 3: Test missing terminal usage after dispatch**

Use a capability-valid fake that is actually dispatched and returns bytes/task success without the required terminal usage. Assert the result candidate is not published as billable, the Hold is released, the Charge is `unbilled`, and no wallet debit/refund transaction exists. Run `go test ./internal/module/ai/audio -run 'Test.*Usage|Test.*Capability|Test.*Fail'`.

### Task 5: Verify and commit

- [ ] **Step 1: Run focused checks**

Run `gofmt -w internal/module/ai/text internal/module/ai/tool internal/module/ai/image internal/module/ai/video internal/module/ai/audio`, package-level `go test` for the changed modules, and `git diff --check`.

- [ ] **Step 2: Commit**

```powershell
git add internal/module/ai/text internal/module/ai/tool internal/module/ai/image internal/module/ai/video internal/module/ai/audio internal/infra/ai
git commit -m "feat(ai): settle media and tool runs through gateway"
```
