# AI Run 延迟持久化观测 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 持久化聊天从 HTTP 受理到结算的完整时间线，让 Run 详情和渠道统计能用证据判断慢在本地、队列还是上游。

**Architecture:** Command 保存受理和 claim，Provider Attempt 保存准备、派发、首个可交付增量和上游完成，Run 保存原子结算时间。现有 Asynq wake 继续作为主路径，1 秒 poller 继续作为 fallback；先记录 `wake|poll|recovery`，不重复建设唤醒机制。

**Tech Stack:** Go、GORM、MySQL 8、Gin、OpenAI-compatible streaming、Vue 3、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-07-28-ai-chat-capability-tools-interaction-design.md` 第 7 节。

---

## 已知基线

Run `455` 的已确认单次证据：排队约 `1.215s`、本地准备约 `1.489s`、Provider 总耗时约 `17.220s`、结算约 `0.119s`、端到端约 `19.876s`。本次主耗时在上游，但单条样本不能代表渠道 P50/P95/P99；当前数据库也不能还原 TTFT。

## 执行边界

- 不用假输出掩盖 TTFT，不绕过 durable 接受、冻结或结算。
- 不把 prepared request、API Key、完整 Provider 响应或完整敏感提示词返回前端。
- 不重算或改写上游 usage。
- 不自动提交 Git。

### Task 1：增加最终时间线字段和 claim 来源

**Files:**
- Create: `database/migrations/202607280102_ai_run_latency_timeline.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/schema/admin.hcl`
- Modify: `database/reconciliation/030_verify_schema.sql`
- Modify: `database/reconciliation/031_verify_relations.sql`
- Create: `internal/architecture/ai_run_latency_schema_test.go`
- Modify: `internal/module/ai/replycommand/model.go`
- Modify: `internal/module/ai/replycommand/attempt.go`
- Modify: `internal/module/ai/run/model.go`

- [ ] **Step 1：先写 schema 失败测试**

```go
func TestAIRunLatencyTimelineSchemaHasCanonicalColumns(t *testing.T)
```

断言：

```text
ai_reply_commands.request_received_at
ai_reply_commands.accepted_at
ai_reply_commands.claimed_at
ai_reply_commands.claim_source = wake|poll|recovery
ai_provider_attempts.prepare_started_at
ai_provider_attempts.first_delta_at
ai_runs.settled_at
```

`ai_provider_attempts.dispatched_at/finished_at` 继续分别作为 `provider_dispatched_at/provider_finished_at` 的存储事实，不创建重复列。

运行：

```powershell
go test ./internal/architecture -run TestAIRunLatencyTimelineSchema
```

预期：FAIL，缺少新字段。

- [ ] **Step 2：实现 DDL 与约束**

所有时间使用 UTC `datetime(6)` nullable；`claim_source` 使用非空字符串并用 CHECK 限制为 `''|wake|poll|recovery`，未 claim 行保存空字符串。增加 `(provider_id, model_id, created_at)` 渠道统计索引时必须以 `EXPLAIN` 证明现有索引不能覆盖，否则不新增重复索引。

- [ ] **Step 3：验证 schema**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
go test ./internal/architecture -run TestAIRunLatencyTimelineSchema
pwsh -NoProfile -File scripts/verify-database.ps1
```

预期：PASS，空库与导入 fixture 收敛。

### Task 2：记录受理、claim 与准备时间且不破坏冻结边界

**Files:**
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/transport/admin/handler.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/replycommand/model.go`
- Modify: `internal/module/ai/replycommand/repository.go`
- Modify: `internal/module/ai/replycommand/runner.go`
- Modify: `internal/module/ai/replycommand/reconciler.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Test: `internal/module/ai/message/service_test.go`
- Test: `internal/module/ai/replycommand/runner_test.go`
- Test: `internal/module/ai/replycommand/reconciler_test.go`
- Test: `internal/runtime/ai_billing_finalizer_test.go`

- [ ] **Step 1：写时间来源失败测试**

```go
func TestSendPersistsReceivedAndAcceptedTimes(t *testing.T)
func TestRunCommandMarksWakeClaimSource(t *testing.T)
func TestRunOnceMarksPollClaimSource(t *testing.T)
func TestOutcomeReconcilerMarksRecoverySource(t *testing.T)
func TestPrepareStartedAtIsPersistedOnlyWithHeldAttempt(t *testing.T)
func TestInsufficientBalancePersistsNoProviderAttemptTiming(t *testing.T)
```

运行：

```powershell
go test ./internal/module/ai/message ./internal/module/ai/replycommand ./internal/runtime -run 'Test(SendPersists|RunCommandMarks|RunOnceMarks|OutcomeReconcilerMarks|PrepareStarted|InsufficientBalancePersists)'
```

预期：FAIL，当前没有这些时间和来源字段。

- [ ] **Step 2：实现时间传递**

Handler 进入 `Send` 时捕获 `RequestReceivedAt`；`CreateReply` 事务写入 command 的 `accepted_at`。Repository claim 方法接收闭合类型：

```go
type ClaimSource string
const (ClaimSourceWake ClaimSource = "wake"; ClaimSourcePoll ClaimSource = "poll"; ClaimSourceRecovery ClaimSource = "recovery")
```

`Runner.RunCommand` 传 wake，`Runner.RunOnce` 传 poll，Outcome Reconciler 在锁定恢复工作时写 recovery。claim 时间和来源与 fencing claim 同一 SQL 更新，不能在 claim 后补写。

ChatService 在开始加载历史、工具和组装上下文前捕获 `PrepareStartedAt`，通过 `PaidChatAttemptInput` 传到 Gateway。该时间只在余额冻结成功并插入 Provider attempt 的同一事务中落库；余额不足不能为了记录时间提前创建 attempt。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，现有 wake 与 poll 行为保持不变。

### Task 3：以 CAS 记录首个可交付增量和结算时间

**Files:**
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Test: `internal/infra/ai/openaicompat/client_test.go`
- Create: `internal/runtime/first_deliverable_sink.go`
- Create: `internal/runtime/first_deliverable_sink_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/module/ai/replycommand/attempt.go`
- Modify: `internal/runtime/ai_billing_finalizer.go`
- Test: `internal/runtime/ai_billing_finalizer_test.go`

- [ ] **Step 1：写首增量和结算失败测试**

```go
func TestFirstDeliverableSinkIgnoresEmptyAndUsageEvents(t *testing.T)
func TestFirstDeliverableSinkRecordsTextOrToolDeltaOnce(t *testing.T)
func TestAttemptFirstDeltaUsesCompareAndSwap(t *testing.T)
func TestFinalizationWritesSettledAtInTerminalTransaction(t *testing.T)
```

运行：

```powershell
go test ./internal/infra/ai/openaicompat ./internal/runtime -run 'Test(FirstDeliverable|AttemptFirstDelta|FinalizationWritesSettled)'
```

预期：FAIL，当前首增量只有进程内 telemetry，没有 durable 字段。

- [ ] **Step 2：实现可交付事件语义**

`infraai.Event` 的可交付事件固定为：非空 `Type=delta` 文本，或非空 `Type=tool_delta` 且 payload 含 tool call ID/name/arguments delta。SSE 注释、空 chunk、role-only、finish reason、usage 元数据不算首增量。

Gateway 在 attempt 已持久化后用 `firstDeliverableSink` 包装原 sink；首个可交付事件执行：

```sql
UPDATE ai_provider_attempts
SET first_delta_at = ?
WHERE id = ? AND first_delta_at IS NULL
```

写入失败要返回错误并走既有 Provider 结果未知/失败收尾，不能悄悄丢遥测。`settled_at` 与钱包 capture/release、Run terminal、Command terminal 在现有 finalization transaction 内使用同一个 `now` 写入。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，同一 attempt 最多一个 `first_delta_at`。

### Task 4：在 Run API 返回安全耗时分解和渠道分位数

**Files:**
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/service.go`
- Test: `internal/module/ai/run/repository_test.go`
- Test: `internal/module/ai/run/service_test.go`
- Modify: `internal/module/ai/run/transport/admin/request.go`
- Modify: `internal/module/ai/run/transport/admin/handler.go`
- Test: `internal/module/ai/run/transport/admin/handler_test.go`

- [ ] **Step 1：写 presenter 和统计失败测试**

```go
func TestRunDetailBuildsLatencyBreakdownFromDurableTimeline(t *testing.T)
func TestRunDetailReturnsSafePreparedRequestSummaryOnly(t *testing.T)
func TestLatencyStatsUsesNearestRankP50P95P99PerProviderModel(t *testing.T)
func TestLatencyStatsExcludesIncompleteAndNegativeDurations(t *testing.T)
```

运行：

```powershell
go test ./internal/module/ai/run -run 'Test(RunDetail|LatencyStats)'
```

预期：FAIL，DTO 只有总 `duration_ms`。

- [ ] **Step 2：定义并计算安全响应**

详情增加：

```go
type LatencyBreakdown struct {
    AcceptMS *int64 `json:"accept_ms"`
    QueueMS *int64 `json:"queue_ms"`
    PrepareMS *int64 `json:"prepare_ms"`
    TTFTMS *int64 `json:"ttft_ms"`
    ProviderTotalMS *int64 `json:"provider_total_ms"`
    SettlementMS *int64 `json:"settlement_ms"`
    EndToEndMS *int64 `json:"end_to_end_ms"`
    ClaimSource string `json:"claim_source"`
}
```

计算边界固定为相邻持久化时间点；时间缺失返回 null，负值视为损坏证据并返回 null，不制造 0ms。安全摘要只返回 provider request ID、usage item、attempt/tool round 数、`len(prepared_request_json)` 和解析后的 message count；不返回 prepared body、headers、API key、完整 usage JSON 或 result candidate。

Stats API 按 provider + model + 最近 30 天、最多 10000 个完成 attempt 采样，在 Go 中排序并用 nearest-rank 计算 P50/P95/P99，同时返回 sample count。TTFT 与 Provider total 分开统计；小于 20 个样本时返回数值但标记 `insufficient_sample=true`。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，单次 Run 可被可靠归因为排队、本地准备、TTFT、上游完成或结算。

### Task 5：在前端展示耗时并验证 wake 主路径

**Backend Files:**
- Modify: `internal/module/ai/replycommand/jobs_test.go`
- Modify: `internal/jobs/noop_test.go`
- Modify: `internal/module/ai/replycommand/runner_integration_test.go`
- Regenerate after approved backend commit: `contracts/admin/v1/*`

**Frontend Files (`E:/admin/admin_front_ts`):**
- Regenerate: `contracts/backend/admin/v1/*`
- Regenerate: `src/modules/http/generated/admin.ts`
- Regenerate: `src/modules/http/generated/operations.ts`
- Modify: `src/api/ai/runs.ts`
- Modify: `src/views/Main/ai/runs/components/RunList/presenters.ts`
- Modify: `src/views/Main/ai/runs/components/RunList/RunDetailDialog.vue`
- Modify: `src/views/Main/ai/runs/components/RunList/run-detail-dialog.css`
- Modify: `src/views/Main/ai/runs/components/RunStats/index.vue`
- Modify: `tests/shared/ai/ai-run-api.test.ts`
- Modify: `tests/shared/ai/ai-run-billing-presenters.test.ts`
- Create: `tests/component/ai/RunLatencyBreakdown.test.ts`

- [ ] **Step 1：锁定 wake 与 fallback 行为**

后端测试断言 `WakeReply -> ai:reply-command:v1 -> RunCommand -> claim_source=wake`；没有或重复 wake 时 `RunOnce -> claim_source=poll` 仍能处理已提交命令；outcome reconciliation 标记 recovery。运行：

```powershell
go test ./internal/module/ai/replycommand ./internal/jobs -run 'Test.*(Wake|Poll|Recovery|Restart)'
```

预期：新增来源断言先 FAIL，实现后 PASS。不要删除 1 秒 poller，也不要新增第二套 wake。

- [ ] **Step 2：写并实现前端耗时测试**

前端实现前，待用户批准后端提交边界后用真实 40 位 SHA 发布契约，再同步生成类型：

```powershell
# E:/admin/admin_back_go
$sha=(git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $sha
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $sha

# E:/admin/admin_front_ts
npm run contract:sync
npm run contract:generate
```

该步骤不自动创建提交；没有包含延迟 DTO 改动的真实后端提交时暂停契约发布，不能伪造 manifest SHA。

```ts
it('renders queue prepare ttft provider and settlement durations')
it('shows unavailable instead of zero for missing timestamps')
it('shows provider request and safe request summary without raw payload')
it('renders P50 P95 P99 with sample quality')
```

运行：

```powershell
npm test -- tests/shared/ai/ai-run-api.test.ts tests/shared/ai/ai-run-billing-presenters.test.ts tests/component/ai/RunLatencyBreakdown.test.ts
```

预期：测试先 FAIL；实现后 PASS。详情使用紧凑时间轴/表格，不新增嵌套卡片。

- [ ] **Step 3：全链路验证**

后端：

```powershell
go test ./internal/module/ai/replycommand ./internal/module/ai/run ./internal/infra/ai/openaicompat ./internal/runtime ./internal/jobs ./internal/architecture
git diff --check
```

前端：

```powershell
npm run contract:check
npm run typecheck
npm test -- tests/shared/ai/ai-run-api.test.ts tests/shared/ai/ai-run-billing-presenters.test.ts tests/component/ai/RunLatencyBreakdown.test.ts
npm run build
git diff --check
```

预期：全部 PASS。用至少 20 条同 Provider + model 的完成样本比较 P50/P95/P99；Run `455` 这类请求将显示主要耗时在 Provider，而不是凭 491 字节 prepared request 改写上游 reported usage。

- [ ] **Step 4：阶段收尾**

分别执行 `git status --short`。预期：仅出现本计划文件与生成契约产物；没有知识库改动，没有 Git commit。对本地优化的后续修改必须引用阶段耗时分位证据，不能依据单次 `hi` 继续猜测。
