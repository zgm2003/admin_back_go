# AI 对话停止后部分回复交付 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户在流式回复显示到 `1234` 时点击停止，界面同一 tick 固定为“`1234` + 已停止生成”，聊天记录只保存服务端已投递且浏览器连续确认的 `1234`，后台继续排空上游并按完整权威 usage 结算。

**Architecture:** 用户交付状态与 Run/计费状态分离。Worker 将上游小 delta 在 50ms/16KiB 边界内合并，先以 command 内连续序号提交 MySQL 临时分片，再发布 WebSocket v2；取消事务只接受 `delivered_seq`，从服务端分片重建前缀并立即创建 stopped 助手消息。后台 finalizer 继续绑定同一消息 ID、清除完整候选并完成资金结算，前端使用 request-scoped 状态合并，旧请求终态不得清除新请求的 stream。

**Tech Stack:** Go 1.25、Gin、GORM、MySQL 8.4、Redis Pub/Sub、Atlas、Vue 3、TypeScript 5.9、Zod 4、Vitest、Element Plus、WebSocket Admin Contract。

---

## 执行边界

- 后端路径均相对 `E:\admin\admin_back_go`；前端路径均相对 `E:\admin\admin_front_ts`。
- 数据库迁移必须先于新 API/Worker，前后端和实时 v2 契约作为同一版本发布，不保留 v1/v2 双发。
- 自动验证只执行单包/单文件定向测试、短契约检查、`npm run typecheck` 和正式构建。
- 不执行 Playwright、`full-admin-smoke.ps1`、全仓覆盖率、长 PowerShell smoke 或 10 分钟自动压力脚本。
- `admin-dev` 全链路、真实停止竞态、10 分钟持续流和 `EXPLAIN ANALYZE` 由用户在 Task 9 手工验收。

## 文件结构

### 后端新增

- `database/migrations/202607300101_ai_chat_stopped_delivery.sql`：迁移守卫、分片表、command/message 字段、历史回填、v1 durable event 清理。
- `database/reconciliation/20260730_ai_reply_delivery_query_candidates.json`：取消范围读和分片清理的主键查询证据。
- `internal/architecture/ai_stopped_delivery_schema_test.go`：数据库字段、约束、索引、迁移守卫和 reconciliation 不变量。
- `internal/module/ai/replycommand/delivery.go`：分片提交、严格前缀读取和有界清理仓储。
- `internal/module/ai/replycommand/delivery_test.go`：command fencing、连续 seq、原文保真和清理边界测试。
- `internal/module/ai/chat/delivery_sink.go`：50ms/16KiB 合并、UTF-8 拆分、先提交后发布。
- `internal/module/ai/chat/delivery_sink_test.go`：合并、拆分、停止丢弃缓冲和发布顺序测试。
- `internal/runtime/reply_delivery.go`：`aichat.DeliveryCommitter` 到 `replycommand` 仓储的运行时适配。

### 后端修改

- `database/schema/admin.hcl`、`database/migrations/atlas.sum`：目标 schema 与迁移校验和。
- `database/reconciliation/030_verify_schema.sql`、`database/reconciliation/031_verify_relations.sql`：导入库字段/约束/外键校验。
- `internal/module/ai/replycommand/model.go`、`repository.go`、`cancel_test.go`、`finalization.go`、`finalization_test.go`、`reconciler.go`、`reconciler_test.go`：双状态、停止事务、finalizer 和残留清理。
- `internal/module/ai/chat/dto.go`、`events.go`、`events_test.go`、`service.go`、`service_test.go`：持久化投递依赖和 delta v2。
- `internal/module/ai/message/model.go`、`dto.go`、`repository.go`、`service.go`、`service_test.go`、`history_actions_test.go`：取消 HTTP、消息投影和历史动作约束。
- `internal/module/ai/message/transport/admin/request.go`、`handler.go`、`handler_test.go`：必填 `delivered_seq` 与 stopped/already_terminal 响应。
- `internal/module/ai/conversation/repository.go`、`repository_test.go`：stopped 消息不计未读。
- `internal/module/realtime/event.go`、`event_test.go`、`internal/admincontract/realtime.go`、`realtime_test.go`：delta/canceled v2 严格事件。
- `internal/admincontract/openapi_ai_schemas.go`、`openapi_models_test.go`：消息与取消 OpenAPI 契约。
- `internal/runtime/worker.go`、`worker_test.go`、`ai_billing_finalizer.go`、`ai_billing_finalizer_test.go`：依赖装配、同一消息 ID 结算和事务外清理。
- `contracts/admin/v1/openapi.json`、`contracts/admin/v1/realtime/events.schema.json`、`contracts/admin/v1/manifest.json`：生成后的后端契约。
- `docs/architecture.md`：v2 事件、部分交付和双状态机说明。

### 前端修改

- `contracts/backend/admin/v1/**`、`contracts/backend/admin/lock.json`、`src/modules/http/generated/admin.ts`、`operations.ts`：只通过契约同步/生成命令更新。
- `src/modules/realtime/protocol.ts`、`src/api/ai/chat-events.ts`、`chat.ts`、`messages.ts`：v2、`delivered_seq` 和新消息字段。
- `src/views/Main/ai/chat/composables/types.ts`、`useConversationSessions.ts`、`src/views/Main/ai/chat/use-chat-page.ts`：连续序号、立即停止、停止提交 pending、结算 pending 和 request-scoped 合并。
- `src/features/ai-chat/workflow.ts`：canceled v2 不再无条件全量恢复并清空另一个请求。
- `src/views/Main/ai/chat/components/MessageList/index.vue`：停止弱状态和逐消息交互可用性。
- `src/i18n/locales/zh-CN/ai.ts`、`src/i18n/locales/en-US/ai.ts`、`src/i18n/locales/generated.ts`：停止状态文案与类型生成。
- `tests/unit/realtime/protocol.test.ts`、`tests/shared/http/ai-stream-contract.test.ts`、`tests/shared/http/ai-conversation-websocket-contract.test.ts`、`tests/component/ai/MessageInteractions.test.ts`：定向契约和交互回归。
- `tests/component/ai/ChatStopDelivery.test.ts`：新增停止状态机单文件测试。

---

### Task 1: 建立数据库事实、迁移守卫与唯一主键热路径

**Files:**
- Create: `database/migrations/202607300101_ai_chat_stopped_delivery.sql`
- Create: `database/reconciliation/20260730_ai_reply_delivery_query_candidates.json`
- Create: `internal/architecture/ai_stopped_delivery_schema_test.go`
- Modify: `database/schema/admin.hcl`
- Modify: `database/reconciliation/030_verify_schema.sql`
- Modify: `database/reconciliation/031_verify_relations.sql`
- Modify: `database/migrations/atlas.sum`

- [ ] **Step 1: 先写 schema 不变量测试**

测试必须读取 HCL、迁移和 reconciliation 文件，并逐项断言：新表只有复合主键、没有二级索引；三个新增字段存在；迁移含活动 command/attempt 守卫、v1 durable event 清理、历史回填；关系校验含 command 外键。

```go
func TestAIStoppedDeliverySchemaContract(t *testing.T) {
	root := backendRoot(t)
	read := func(parts ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil { t.Fatal(err) }
		return strings.ToLower(string(data))
	}
	schema := read("database", "schema", "admin.hcl")
	migration := read("database", "migrations", "202607300101_ai_chat_stopped_delivery.sql")
	verifySchema := read("database", "reconciliation", "030_verify_schema.sql")
	verifyRelations := read("database", "reconciliation", "031_verify_relations.sql")
	for _, required := range []string{
		`table "ai_reply_delivery_chunks"`, `column "delivery_seq"`,
		`column "stop_delivery_seq"`, `column "delivery_state"`,
		`primary_key`, `chk_ai_reply_delivery_chunk_size`,
	} {
		if !strings.Contains(schema, required) { t.Errorf("schema missing %q", required) }
	}
	if strings.Count(hclTableBlock(t, schema, "ai_reply_delivery_chunks"), `index "`) != 0 {
		t.Fatal("delivery chunks must not add secondary indexes")
	}
	for _, required := range []string{"pending", "claimed", "running", "prepared", "dispatched", "ai.response.canceled.v1"} {
		if !strings.Contains(migration, required) { t.Errorf("migration guard missing %q", required) }
	}
	if !strings.Contains(verifySchema, "ai_reply_delivery_chunks") || !strings.Contains(verifyRelations, "fk_ai_reply_delivery_chunks_command") {
		t.Fatal("reconciliation does not prove stopped-delivery schema")
	}
}
```

- [ ] **Step 2: 运行定向测试并确认先失败**

Run: `go test ./internal/architecture -run TestAIStoppedDeliverySchemaContract -count=1`

Expected: FAIL，原因是迁移、新表或新增字段尚不存在。

- [ ] **Step 3: 写入 fail-closed 迁移与 HCL**

迁移按以下顺序执行，守卫必须位于第一条永久 DDL 之前：

```sql
CREATE TEMPORARY TABLE `_ai_stopped_delivery_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_stopped_delivery_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `id` FROM `ai_reply_commands` WHERE `state` IN ('pending','claimed','running')
  UNION ALL
  SELECT a.`id` FROM `ai_provider_attempts` a
  JOIN `ai_reply_commands` c ON c.`id` = a.`command_id`
  WHERE a.`state` IN ('prepared','dispatched')
) active_work;

DELETE FROM `realtime_events` WHERE `event_type` = 'ai.response.canceled.v1';

ALTER TABLE `ai_reply_commands`
  ADD COLUMN `delivery_seq` INT UNSIGNED NOT NULL DEFAULT 0 AFTER `cancel_requested_at`,
  ADD COLUMN `stop_delivery_seq` INT UNSIGNED NULL AFTER `delivery_seq`;

UPDATE `ai_reply_commands`
SET `stop_delivery_seq` = 0
WHERE `cancel_requested_at` IS NOT NULL;

ALTER TABLE `ai_reply_commands`
  ADD CONSTRAINT `chk_ai_reply_delivery_seq`
  CHECK (
    (`cancel_requested_at` IS NULL AND `stop_delivery_seq` IS NULL)
    OR (`cancel_requested_at` IS NOT NULL AND `stop_delivery_seq` IS NOT NULL AND `stop_delivery_seq` <= `delivery_seq`)
  );

ALTER TABLE `ai_messages`
  ADD COLUMN `delivery_state` VARCHAR(16) NULL AFTER `reply_command_id`;

UPDATE `ai_messages` SET `delivery_state` = 'completed' WHERE `role` = 2;

ALTER TABLE `ai_messages`
  ADD CONSTRAINT `chk_ai_messages_delivery_state`
  CHECK (
    (`role` = 2 AND `delivery_state` IN ('completed','stopped'))
    OR (`role` <> 2 AND `delivery_state` IS NULL)
  );

CREATE TABLE `ai_reply_delivery_chunks` (
  `command_id` BIGINT UNSIGNED NOT NULL,
  `delivery_seq` INT UNSIGNED NOT NULL,
  `delta` TEXT NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`command_id`, `delivery_seq`),
  CONSTRAINT `fk_ai_reply_delivery_chunks_command`
    FOREIGN KEY (`command_id`) REFERENCES `ai_reply_commands` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_reply_delivery_chunk_seq` CHECK (`delivery_seq` > 0),
  CONSTRAINT `chk_ai_reply_delivery_chunk_size`
    CHECK (OCTET_LENGTH(`delta`) > 0 AND OCTET_LENGTH(`delta`) <= 16384)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

DROP TEMPORARY TABLE `_ai_stopped_delivery_guard`;
```

`20260730_ai_reply_delivery_query_candidates.json` 固定记录两类 SQL：

```json
[
  {
    "name": "ai_reply_delivery_cancel_prefix",
    "repository_file": "internal/module/ai/replycommand/delivery.go",
    "sql": "SELECT delivery_seq,delta FROM ai_reply_delivery_chunks WHERE command_id=:command_id AND delivery_seq<=:delivered_seq ORDER BY delivery_seq ASC",
    "bindings": {"command_id": 41, "delivered_seq": 400},
    "expected_key": "PRIMARY",
    "max_rows_examined": 400
  },
  {
    "name": "ai_reply_delivery_cleanup_batch",
    "repository_file": "internal/module/ai/replycommand/delivery.go",
    "sql": "DELETE FROM ai_reply_delivery_chunks WHERE command_id=:command_id ORDER BY delivery_seq ASC LIMIT :limit",
    "bindings": {"command_id": 41, "limit": 256},
    "expected_key": "PRIMARY",
    "max_rows_examined": 256
  }
]
```

- [ ] **Step 4: 更新 Atlas hash 并运行短验证**

Run: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`

Expected: `database/migrations/atlas.sum` 更新且命令退出 0。

Run: `go test ./internal/architecture -run 'TestAIStoppedDeliverySchemaContract|TestDatabaseLayoutSeparatesLegacyAndAtlasMigrations' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交数据库事实**

```bash
git add database/schema/admin.hcl database/migrations/202607300101_ai_chat_stopped_delivery.sql database/migrations/atlas.sum database/reconciliation/030_verify_schema.sql database/reconciliation/031_verify_relations.sql database/reconciliation/20260730_ai_reply_delivery_query_candidates.json internal/architecture/ai_stopped_delivery_schema_test.go
git commit -m "feat(ai): 建立停止回复投递事实"
```

---

### Task 2: 实现连续分片提交与有界清理仓储

**Files:**
- Create: `internal/module/ai/replycommand/delivery.go`
- Create: `internal/module/ai/replycommand/delivery_test.go`
- Modify: `internal/module/ai/replycommand/model.go`
- Modify: `internal/module/ai/replycommand/repository.go`

- [ ] **Step 1: 写分片仓储失败测试**

测试覆盖：只允许持有有效 lease 的 running command 写入；command 行锁后 `delivery_seq+1`；同一事务更新 command 并 INSERT；取消后返回 `Committed=false`；空串、非法 UTF-8、超过 16KiB 直接拒绝；前缀读取严格逐字节保留空白。

```go
func TestAppendDeliveryChunkRequiresRunningFencedCommandAndPreservesBytes(t *testing.T) {
	input := AppendDeliveryChunkInput{
		CommandID: 41, Owner: "worker-a", Token: 7,
		Delta: "  你\n", Now: time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
	}
	result, err := repository.AppendDeliveryChunk(context.Background(), input)
	if err != nil { t.Fatal(err) }
	if !result.Committed || result.DeliverySeq != 4 { t.Fatalf("result=%+v", result) }
	if persisted.Delta != input.Delta { t.Fatalf("delta=%q want=%q", persisted.Delta, input.Delta) }
}
```

- [ ] **Step 2: 运行仓储单包定向测试并确认失败**

Run: `go test ./internal/module/ai/replycommand -run 'TestAppendDeliveryChunk|TestReadDeliveryPrefix|TestDeleteDeliveryChunks' -count=1`

Expected: FAIL，原因是分片类型和仓储方法尚不存在。

- [ ] **Step 3: 增加模型、输入类型和仓储方法**

使用明确类型，禁止正文进入日志字段：

```go
const MaxDeliveryChunkBytes = 16 * 1024

type DeliveryChunk struct {
	CommandID   uint64    `gorm:"column:command_id;primaryKey"`
	DeliverySeq uint32    `gorm:"column:delivery_seq;primaryKey"`
	Delta       string    `gorm:"column:delta"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (DeliveryChunk) TableName() string { return "ai_reply_delivery_chunks" }

type AppendDeliveryChunkInput struct {
	CommandID uint64
	Owner     string
	Token     uint64
	Delta     string
	Now       time.Time
}

type AppendDeliveryChunkResult struct {
	DeliverySeq uint32
	Committed   bool
}

type DeliveryPrefix struct {
	StopDeliverySeq uint32
	Content         string
	Consistent      bool
}
```

`AppendDeliveryChunk` 必须使用一个短事务：`SELECT ... FOR UPDATE` command，校验 `state=running`、owner/token、lease 未过期、`cancel_requested_at IS NULL`；计算下一 seq；更新 command；插入分片；提交后才返回。`ReadDeliveryPrefixTx` 只按 `(command_id, delivery_seq)` 主键范围升序读取，在 Go 中验证 `1..delivered_seq` 无缺口并用 `strings.Builder` 原样拼接。`DeleteDeliveryChunks` 每次最多删除 256 行，调用方循环但每批独立提交。

```go
func validDeliveryDelta(delta string) bool {
	return delta != "" && utf8.ValidString(delta) && len(delta) <= MaxDeliveryChunkBytes
}

func (r *GormRepository) DeleteDeliveryChunks(ctx context.Context, commandID uint64, limit int) (int64, error) {
	if limit <= 0 || limit > 256 { limit = 256 }
	result := r.db.WithContext(ctx).Exec(
		"DELETE FROM ai_reply_delivery_chunks WHERE command_id = ? ORDER BY delivery_seq ASC LIMIT ?",
		commandID, limit,
	)
	return result.RowsAffected, result.Error
}
```

- [ ] **Step 4: 运行仓储定向测试**

Run: `go test ./internal/module/ai/replycommand -run 'TestAppendDeliveryChunk|TestReadDeliveryPrefix|TestDeleteDeliveryChunks' -count=1`

Expected: PASS；sqlmock 证明写入事务和 cleanup LIMIT，不连接真实数据库。

- [ ] **Step 5: 提交分片仓储**

```bash
git add internal/module/ai/replycommand/model.go internal/module/ai/replycommand/repository.go internal/module/ai/replycommand/delivery.go internal/module/ai/replycommand/delivery_test.go
git commit -m "feat(ai): 持久化连续回复分片"
```

---

### Task 3: 建立 50ms/16KiB 先提交后发布的投递 sink

**Files:**
- Create: `internal/module/ai/chat/delivery_sink.go`
- Create: `internal/module/ai/chat/delivery_sink_test.go`
- Create: `internal/runtime/reply_delivery.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/events.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/realtime/event.go`
- Modify: `internal/module/realtime/event_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`

- [ ] **Step 1: 先写投递顺序、合并和 UTF-8 测试**

单文件测试使用 fake committer/publisher，不依赖 sleep 断言 P95。通过显式 `Flush` 验证多个原始 delta 合并为一个持久分片；通过记录调用顺序验证 commit 严格早于 publish；数据库错误时 publish 次数为 0；停止信号到达时未提交缓冲被丢弃。

```go
func TestPersistentDeliverySinkCommitsBeforePublishing(t *testing.T) {
	order := []string{}
	committer := deliveryCommitterFunc(func(context.Context, DeliveryCommit) (uint32, bool, error) {
		order = append(order, "commit")
		return 1, true, nil
	})
	publisher := publisherFunc(func(context.Context, infrarealtime.Publication) error {
		order = append(order, "publish")
		return nil
	})
	sink := newDeliverySink(deliverySinkOptions{Committer: committer, Publisher: publisher, MaxWait: 50 * time.Millisecond})
	if err := sink.Accept("1"); err != nil { t.Fatal(err) }
	if err := sink.Accept("2"); err != nil { t.Fatal(err) }
	if err := sink.Flush(context.Background()); err != nil { t.Fatal(err) }
	if len(order) != 2 || order[0] != "commit" || order[1] != "publish" {
		t.Fatalf("unexpected delivery order: %v", order)
	}
}
```

- [ ] **Step 2: 运行 chat 单包定向测试并确认失败**

Run: `go test ./internal/module/ai/chat -run 'TestPersistentDeliverySink|TestSplitDeliveryUTF8' -count=1`

Expected: FAIL，原因是持久投递 sink 尚不存在。

- [ ] **Step 3: 实现异步合并器和运行时适配**

`aichat` 只依赖接口，不能导入 `replycommand`：

```go
type DeliveryCommit struct {
	CommandID uint64
	Owner     string
	Token     uint64
	Delta     string
	Now       time.Time
}

type DeliveryCommitter interface {
	CommitDelivery(context.Context, DeliveryCommit) (deliverySeq uint32, committed bool, err error)
}
```

UTF-8 拆分必须按字节上界且逐字节保真：

```go
func splitDeliveryUTF8(value string, maxBytes int) []string {
	parts := make([]string, 0, (len(value)/maxBytes)+1)
	for len(value) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(value[cut]) { cut-- }
		parts = append(parts, value[:cut])
		value = value[cut:]
	}
	if value != "" { parts = append(parts, value) }
	return parts
}
```

合并器使用一个有界 channel 和单 goroutine 保序，首个 delta 启动 50ms timer，达到 16KiB、timer 到期、正常终态或显式 Flush 时提交；停止时丢弃尚未提交的 buffer。这个任务同时把后端运行时 delta 从 v1 替换成 `ai.response.delta.v2`，删除 delta v1 registry 定义；canceled 事件等 stopped message ID 可用后在 Task 6 切 v2。每个提交完成后才构造 delta v2 并发布，事件必须携带仓储返回的 `delivery_seq`。任何提交错误包装成 fatal sink error；发布失败不能回滚已提交分片，且不能伪造客户端已经看到该 seq。

```go
const TypeAIResponseDeltaV2 = "ai.response.delta.v2"

type AIResponseDeltaPayload struct {
	ConversationID int64  `json:"conversation_id"`
	RequestID      string `json:"request_id"`
	DeliverySeq    uint32 `json:"delivery_seq"`
	Delta          string `json:"delta"`
}
```

`internal/runtime/reply_delivery.go` 只映射字段：

```go
func (p replyDeliveryCommitter) CommitDelivery(ctx context.Context, input aichat.DeliveryCommit) (uint32, bool, error) {
	result, err := p.repository.AppendDeliveryChunk(ctx, replycommand.AppendDeliveryChunkInput{
		CommandID: input.CommandID, Owner: input.Owner, Token: input.Token,
		Delta: input.Delta, Now: input.Now,
	})
	return result.DeliverySeq, result.Committed, err
}
```

在 `worker.go` 将同一个 `replyRepository` 注入 `aichat.Dependencies.DeliveryCommitter`。`service.go` 创建一个覆盖整轮（包括工具调用续轮）的 coalescer；原始 `conversationEventSink` 不再直接发布 delta；所有返回路径必须 Flush/Close。空内容 fallback 仅在 `delta == ""` 时跳过，禁止 `TrimSpace` 破坏空白。

- [ ] **Step 4: 运行 chat 与 runtime 定向测试**

Run: `go test ./internal/module/ai/chat -run 'TestPersistentDeliverySink|TestSplitDeliveryUTF8|TestDeliveryStopDiscardsBuffer' -count=1`

Expected: PASS。

Run: `go test ./internal/runtime -run 'TestReplyDeliveryCommitter|TestWorkerReplyRepositoryUsesDurableRealtimeSink' -count=1`

Expected: PASS。

Run: `go test ./internal/module/realtime -run 'TestAIResponseDeltaV2|TestRegistry' -count=1`

Expected: PASS，delta v1 已从运行时 registry 删除。

- [ ] **Step 5: 提交持久投递链路**

```bash
git add internal/module/ai/chat/dto.go internal/module/ai/chat/events.go internal/module/ai/chat/service.go internal/module/ai/chat/delivery_sink.go internal/module/ai/chat/delivery_sink_test.go internal/module/realtime/event.go internal/module/realtime/event_test.go internal/runtime/reply_delivery.go internal/runtime/worker.go internal/runtime/worker_test.go
git commit -m "feat(ai): 提交后发布流式回复"
```

---

### Task 4: 将取消改成权威 stopped 消息事务

**Files:**
- Modify: `internal/module/ai/replycommand/model.go`
- Modify: `internal/module/ai/replycommand/repository.go`
- Modify: `internal/module/ai/replycommand/cancel_test.go`
- Modify: `internal/module/ai/message/model.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/message/transport/admin/request.go`
- Modify: `internal/module/ai/message/transport/admin/handler.go`
- Modify: `internal/module/ai/message/transport/admin/handler_test.go`
- Modify: `internal/module/ai/conversation/repository.go`
- Modify: `internal/module/ai/conversation/repository_test.go`

- [ ] **Step 1: 写取消、消息投影和未读失败测试**

覆盖 `delivered_seq=0`、中间前缀、越界/缺口降为 0、重复停止不同 seq 不改正文、成功终态先赢得竞态、停止事务先赢得竞态、stopped 不计未读。请求体额外携带 `content` 必须被 strict JSON 拒绝。

```go
func TestRequestCancelBuildsStoppedMessageFromServerPrefix(t *testing.T) {
	result, err := repository.RequestCancel(context.Background(), RequestCancelInput{
		ConversationID: 3, UserID: 7, RequestID: "request-1",
		DeliveredSeq: 4, Now: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
	})
	if err != nil { t.Fatal(err) }
	if result.Status != CancelStatusStopped || result.AssistantMessageID <= 0 || !result.SettlementPending {
		t.Fatalf("result=%+v", result)
	}
	if persisted.Content != "1234" || persisted.DeliveryState != DeliveryStateStopped {
		t.Fatalf("message=%+v", persisted)
	}
}
```

- [ ] **Step 2: 运行 replycommand/aimessage 定向测试并确认失败**

Run: `go test ./internal/module/ai/replycommand -run 'TestRequestCancel' -count=1`

Expected: FAIL，旧实现只写 `cancel_requested_at`。

Run: `go test ./internal/module/ai/message -run 'TestCancel|TestListProjectsStopped|TestStoppedMessageUnread' -count=1`

Expected: FAIL，新字段与响应尚不存在。

- [ ] **Step 3: 定义停止输入、结果和事务规则**

```go
type CancelStatus string

const (
	CancelStatusStopped         CancelStatus = "stopped"
	CancelStatusAlreadyTerminal CancelStatus = "already_terminal"
	DeliveryStateCompleted                  = "completed"
	DeliveryStateStopped                    = "stopped"
)

type RequestCancelInput struct {
	ConversationID int64
	UserID         int64
	RequestID      string
	DeliveredSeq   uint32
	Now            time.Time
}

type RequestCancelResult struct {
	CommandID          uint64
	Status             CancelStatus
	AssistantMessageID int64
	SettlementPending  bool
}
```

`RequestCancel` 的单事务锁序固定为 command 后 conversation：

1. `SELECT command FOR UPDATE` 并校验 `(user_id, request_id, conversation_id)`。
2. terminal command 返回 `already_terminal`、现有可空助手 ID、`settlement_pending=false`，不改事实。
3. 已停止 command 返回第一次的 stopped 消息 ID 和 seq，忽略重复请求的新 seq。
4. 首次停止读取 `1..delivered_seq`；连续且不越界时逐字节拼接；不一致时权威 seq 降为 0、内容为空，并返回可观测的一致性标记供 service 记录 command/request/seq/字节数，日志不含正文。
5. 创建允许空正文的 `role=assistant, delivery_state=stopped, reply_command_id=command.id` 消息。
6. 更新 command 的 `cancel_requested_at`、`stop_delivery_seq`、`assistant_message_id`，并更新 conversation 的 `last_message_at`。
7. 提交后才 best-effort 发布 Redis cancel signal，并以 `context.WithoutCancel` 只执行一批最多 256 行的分片清理；剩余行由 Worker/reconciler 继续处理。这样停止 HTTP 不等待大量 DELETE，这两个副作用失败也不撤销 stopped 事实。

- [ ] **Step 4: 更新 HTTP 与消息列表契约实现**

请求用指针保证 JSON 缺字段与合法 0 可区分：

```go
type cancelRequest struct {
	RequestID    string  `json:"request_id" binding:"required,max=128"`
	DeliveredSeq *uint32 `json:"delivered_seq" binding:"required"`
}

type CancelResponse struct {
	ConversationID     int64   `json:"conversation_id"`
	RequestID          string  `json:"request_id"`
	Status             string  `json:"status"`
	AssistantMessageID *int64  `json:"assistant_message_id"`
	SettlementPending  bool    `json:"settlement_pending"`
}
```

`AIMessageItem` 和 SQL projection 增加必返字段：

```go
DeliveryState     *string `json:"delivery_state" gorm:"column:delivery_state"`
SettlementPending bool    `json:"settlement_pending" gorm:"column:settlement_pending"`
```

`settlement_pending` 对用户消息取 `user_commands.state`，对助手消息取 `assistant_commands.state`，只在 `pending|claimed|running` 为 true。`delivery_state` 直接来自消息行。`UnreadCounts` 和 read-cursor unread LEFT JOIN 增加 `m.delivery_state <> 'stopped'`；stopped 仍参与配对、历史上下文、软删除和终态后的重新生成。

- [ ] **Step 5: 运行取消与投影定向测试**

Run: `go test ./internal/module/ai/replycommand -run 'TestRequestCancel' -count=1`

Expected: PASS。

Run: `go test ./internal/module/ai/message -run 'TestCancel|TestListProjectsStopped' -count=1`

Expected: PASS。

Run: `go test ./internal/module/ai/conversation -run 'TestUnreadCounts|TestAdvanceReadCursor' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交权威停止事务**

```bash
git add internal/module/ai/replycommand/model.go internal/module/ai/replycommand/repository.go internal/module/ai/replycommand/cancel_test.go internal/module/ai/message internal/module/ai/conversation/repository.go internal/module/ai/conversation/repository_test.go
git commit -m "feat(ai): 保存用户已见停止回复"
```

---

### Task 5: 让 canceled finalizer 绑定同一消息并在事务外清理分片

**Files:**
- Modify: `internal/module/ai/replycommand/finalization.go`
- Modify: `internal/module/ai/replycommand/finalization_test.go`
- Modify: `internal/module/ai/replycommand/reconciler.go`
- Modify: `internal/module/ai/replycommand/reconciler_test.go`
- Modify: `internal/runtime/ai_billing_finalizer.go`
- Modify: `internal/runtime/ai_billing_finalizer_test.go`
- Modify: `internal/module/ai/message/history_actions_test.go`

- [ ] **Step 1: 写同一消息 ID、usage 与清理失败测试**

测试必须同时证明：canceled finalizer 接受已存在 stopped 消息；command 和 Run 都绑定该 ID；消息正文/时间不变；完整 usage 可以 settled；usage 不完整保持 unbilled；candidate 清空；清理发生在外层资金事务提交后；清理失败不回滚资金事实且 reconciler 可补偿。

```go
func TestCanceledFinalizationReusesStoppedAssistantMessage(t *testing.T) {
	result, err := repository.FinalizePaidCommandInTransaction(ctx, tx, PaidCommandFinalizationInput{
		CommandID: 41, UserID: 7, RequestID: "request-1",
		State: StateCanceled, Now: now,
	})
	if err != nil { t.Fatal(err) }
	if result.AssistantMessageID != 97 { t.Fatalf("assistant id=%d", result.AssistantMessageID) }
	if stopped.Content != "1234" || stopped.CreatedAt != originalCreatedAt {
		t.Fatalf("stopped message mutated: %+v", stopped)
	}
}
```

- [ ] **Step 2: 运行 finalizer 定向测试并确认失败**

Run: `go test ./internal/module/ai/replycommand -run 'TestCanceledFinalization' -count=1`

Expected: FAIL，旧 finalizer 要求 canceled command 不存在助手消息。

Run: `go test ./internal/runtime -run 'TestCanceledChatFinalization' -count=1`

Expected: FAIL，旧 Run canceled 分支会把 `assistant_message_id` 写 NULL。

- [ ] **Step 3: 修改 command participant 与 Run finalizer**

`FinalizePaidCommandInTransaction` 对 canceled 分支必须校验：command 已有 `cancel_requested_at` 和 `stop_delivery_seq`；`ai_messages.reply_command_id` 唯一命中同一 `assistant_message_id`；消息属于同会话、role=assistant、`delivery_state=stopped`、未软删。它不更新消息正文、时间或 conversation 排序，只返回已有 ID 并终态化 command。

```go
case StateCanceled:
	if command.CancelRequestedAt == nil || command.StopDeliverySeq == nil ||
		command.AssistantMessageID == nil || errors.Is(messageErr, gorm.ErrRecordNotFound) ||
		existing.ID != *command.AssistantMessageID || existing.DeliveryState != DeliveryStateStopped {
		return nil, ErrPaidCommandFinalizationConflict
	}
	result.AssistantMessageID = existing.ID
```

`finalizeChatRunAndCharge` 对 success 和 canceled 都写 participant 返回的助手 ID；`chatCommandMatchesRunTerminal` 对 canceled 也要求 command/run ID 同值；`appendChatRealtimeFinalization` 在 Task 6 切换为含该 ID 的 canceled v2。资金决策和 token 统计继续只读 provider attempts 的完整 usage，禁止读取 stopped 正文长度。

- [ ] **Step 4: 增加事务外小批量清理与 reconciler 补偿**

外层 finalizer 事务成功后调用：

```go
func cleanupDeliveryChunks(ctx context.Context, repository DeliveryCleaner, commandID uint64, maxBatches int) error {
	if maxBatches <= 0 { return nil }
	for batch := 0; batch < maxBatches; batch++ {
		deleted, err := repository.DeleteDeliveryChunks(ctx, commandID, 256)
		if err != nil { return err }
		if deleted < 256 { return nil }
	}
	return nil
}
```

停止 HTTP 提交后使用 `maxBatches=1`；Worker 的正常成功/失败/canceled/outcome_unknown 终态提交后使用固定小上界 `maxBatches=4`。不得在持有 Run、Charge、wallet、Hold 锁的事务中 DELETE。`Reconciler.RunOnce` 每轮从复合主键顺序读取少量 `DISTINCT command_id`，逐个确认 command 已有可清理终态或 stopped 消息，再按主键批量清理；活动 command 不能被清理。

- [ ] **Step 5: 验证 finalizer、清理和历史动作**

Run: `go test ./internal/module/ai/replycommand -run 'TestCanceledFinalization|TestReconcilerCleansDeliveryChunks' -count=1`

Expected: PASS。

Run: `go test ./internal/runtime -run 'TestCanceledChatFinalization|TestChatFinalizationCleanupRunsAfterCommit' -count=1`

Expected: PASS。

Run: `go test ./internal/module/ai/message -run 'TestRegenerateStoppedMessage|TestHistoryActiveCommand' -count=1`

Expected: PASS；settlement pending 时拒绝历史变更，终态后允许重新生成和软删除。

- [ ] **Step 6: 提交结算与清理闭环**

```bash
git add internal/module/ai/replycommand/finalization.go internal/module/ai/replycommand/finalization_test.go internal/module/ai/replycommand/reconciler.go internal/module/ai/replycommand/reconciler_test.go internal/runtime/ai_billing_finalizer.go internal/runtime/ai_billing_finalizer_test.go internal/module/ai/message/history_actions_test.go
git commit -m "feat(ai): 结算停止回复并清理分片"
```

---

### Task 6: 完成 canceled v2、OpenAPI 与正式架构契约

**Files:**
- Modify: `internal/module/realtime/event.go`
- Modify: `internal/module/realtime/event_test.go`
- Modify: `internal/module/ai/chat/events.go`
- Modify: `internal/module/ai/chat/events_test.go`
- Modify: `internal/module/ai/replycommand/realtime_integration_test.go`
- Modify: `internal/runtime/ai_billing_finalizer.go`
- Modify: `internal/runtime/ai_billing_finalizer_test.go`
- Modify: `internal/admincontract/realtime.go`
- Modify: `internal/admincontract/realtime_test.go`
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `docs/architecture.md`
- Generated: `contracts/admin/v1/openapi.json`
- Generated: `contracts/admin/v1/realtime/events.schema.json`
- Generated: `contracts/admin/v1/manifest.json`

- [ ] **Step 1: 将 canceled 契约测试切到 v2，并锁定已有 delta v2**

```go
const (
	TypeAIResponseCanceledV2 = "ai.response.canceled.v2"
)

type AIResponseCanceledPayload struct {
	ConversationID     int64  `json:"conversation_id"`
	RequestID          string `json:"request_id"`
	AssistantMessageID int64  `json:"assistant_message_id"`
}
```

事件 registry、JSON Schema 和测试中删除 canceled v1，并验证 Task 3 已删除 delta v1；delta v2 必须 ephemeral、seq 正整数、delta 非空且 UTF-8 字节不超过 16KiB；canceled v2 必须 durable 且 message ID 为正整数。

- [ ] **Step 2: 运行 realtime/admincontract 定向测试并确认失败**

Run: `go test ./internal/module/realtime -run 'TestAIResponseDeltaV2|TestAIResponseCanceledV2' -count=1`

Expected: FAIL，canceled registry 仍是 v1；delta v2 断言已经通过。

Run: `go test ./internal/admincontract -run 'TestRealtime|TestAIMessage' -count=1`

Expected: FAIL，OpenAPI 仍缺新字段。

- [ ] **Step 3: 更新 registry、final event 与 OpenAPI schema**

OpenAPI 使用严格必返字段：

```go
"AIMessageCancelRequest": closedObjectSchema([]string{"request_id", "delivered_seq"}, map[string]any{
	"request_id": schemaWith(maxStringSchema(128), "minLength", 1),
	"delivered_seq": nonNegativeIntegerSchema(),
}),
"AIMessageCancelResult": closedObjectAllProperties(map[string]any{
	"conversation_id": positiveIntegerSchema(),
	"request_id": stringSchema(),
	"status": stringEnumSchema("stopped", "already_terminal"),
	"assistant_message_id": nullableSchema(positiveIntegerSchema()),
	"settlement_pending": booleanSchema(),
}),
```

`AIMessageItem` required 列表增加 `delivery_state`、`settlement_pending`，其中 delivery state 为 `nullable(enum(completed, stopped))`。canceled final event 从 finalizer result 读取同一 `assistant_message_id`；`transitionWithTerminalEvent` 的 generic canceled 分支也必须读取 command 上的 stopped message ID，缺失时拒绝提交 durable v2，不能生成 message ID 为 0 的事件。`docs/architecture.md` 明确“delta 先提交分片再发布”“stopped 是交付终态，Run 可仍 running”“canceled v2 只表示后台结算完成”。

- [ ] **Step 4: 运行后端短契约测试**

Run: `go test ./internal/module/realtime -run 'TestAIResponseDeltaV2|TestAIResponseCanceledV2|TestRegistry' -count=1`

Expected: PASS。

Run: `go test ./internal/admincontract -run 'TestRealtime|TestAIMessage' -count=1`

Expected: PASS。

Run: `rg -n "ai\.response\.(delta|canceled)\.v1" internal contracts docs/architecture.md`

Expected: 无输出；迁移文件中主动删除历史 v1 的 SQL 是唯一允许命中项，执行检查时排除 `database/migrations/202607300101_ai_chat_stopped_delivery.sql`。

- [ ] **Step 5: 先提交后端代码，再生成并提交契约包**

```bash
git add internal/module/realtime internal/module/ai/chat/events.go internal/module/ai/chat/events_test.go internal/module/ai/replycommand/realtime_integration_test.go internal/runtime/ai_billing_finalizer.go internal/runtime/ai_billing_finalizer_test.go internal/admincontract docs/architecture.md
git commit -m "feat(contract): 发布停止回复实时v2"
```

Run: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/generate-admin-contract.ps1`

Expected: 后端工作区在生成前干净；契约 bundle 生成成功。

Run: `go test ./internal/admincontract -run 'TestBundle|TestOpenAPI|TestRealtime' -count=1`

Expected: PASS。

```bash
git add contracts/admin/v1
git commit -m "chore(contract): 生成停止回复契约"
```

---

### Task 7: 同步前端契约并只订阅 delta/canceled v2

**Files:**
- Generated: `contracts/backend/admin/v1/**`
- Generated: `contracts/backend/admin/lock.json`
- Generated: `src/modules/http/generated/admin.ts`
- Generated: `src/modules/http/generated/operations.ts`
- Modify: `src/modules/realtime/protocol.ts`
- Modify: `src/api/ai/chat-events.ts`
- Modify: `src/api/ai/chat.ts`
- Modify: `src/api/ai/messages.ts`
- Modify: `tests/unit/realtime/protocol.test.ts`
- Modify: `tests/shared/http/ai-stream-contract.test.ts`
- Modify: `tests/shared/http/ai-conversation-websocket-contract.test.ts`

- [ ] **Step 1: 从后端已生成 manifest 同步并生成前端类型**

在 PowerShell 中执行：

```powershell
$manifest = Get-Content -Raw 'E:\admin\admin_back_go\contracts\admin\v1\manifest.json' | ConvertFrom-Json
npm run contract:sync -- --backend E:\admin\admin_back_go --commit $manifest.backend_commit
npm run contract:generate
```

Expected: lock、后端快照、`admin.ts`、`operations.ts` 更新，生成文件不手改。

- [ ] **Step 2: 先把协议测试改为 v2 并确认失败**

测试数据固定为：

```ts
['ai.response.delta.v2', {
  conversation_id: 10,
  request_id: 'request-1',
  delivery_seq: 1,
  delta: 'hello',
}],
['ai.response.canceled.v2', {
  conversation_id: 10,
  request_id: 'request-1',
  assistant_message_id: 13,
}],
```

Run: `npm test -- tests/unit/realtime/protocol.test.ts`

Expected: FAIL，前端协议仍只认识 v1。

- [ ] **Step 3: 更新 Zod、事件常量和取消 API**

```ts
const aiDeltaPayloadSchema = z.strictObject({
  conversation_id: safePositiveInteger,
  request_id: requiredRequestIdSchema,
  delivery_seq: safePositiveInteger,
  delta: z.string().min(1).refine((value) => new TextEncoder().encode(value).byteLength <= 16_384),
})

const aiCanceledPayloadSchema = z.strictObject({
  conversation_id: safePositiveInteger,
  request_id: requiredRequestIdSchema,
  assistant_message_id: safePositiveInteger,
})
```

`RealtimeEventMap`、durable union、schema map、`AI_RESPONSE_EVENTS` 一次性替换为 v2。`AiMessageCancelParams` 增加 `delivered_seq: number`，API body 只发送 `request_id` 和 `delivered_seq`。`assertAiStoppingAcknowledgment` 改成返回判别联合：`stopped` 必须有正 message ID；`already_terminal` 必须 `settlement_pending=false`，助手 ID 可空。

- [ ] **Step 4: 运行三个单文件契约测试**

Run: `npm test -- tests/unit/realtime/protocol.test.ts`

Expected: PASS。

Run: `npm test -- tests/shared/http/ai-stream-contract.test.ts`

Expected: PASS，源码只包含 v2 且取消无正文。

Run: `npm test -- tests/shared/http/ai-conversation-websocket-contract.test.ts`

Expected: PASS。

Run: `npm run contract:check`

Expected: PASS。

- [ ] **Step 5: 提交前端契约切换**

```bash
git add contracts/backend/admin src/modules/http/generated src/modules/realtime/protocol.ts src/api/ai/chat-events.ts src/api/ai/chat.ts src/api/ai/messages.ts tests/unit/realtime/protocol.test.ts tests/shared/http/ai-stream-contract.test.ts tests/shared/http/ai-conversation-websocket-contract.test.ts
git commit -m "feat(contract): 同步停止回复实时v2"
```

---

### Task 8: 实现前端立即停止与 request-scoped 终态合并

**Files:**
- Create: `tests/component/ai/ChatStopDelivery.test.ts`
- Modify: `src/views/Main/ai/chat/composables/types.ts`
- Modify: `src/views/Main/ai/chat/composables/useConversationSessions.ts`
- Modify: `src/views/Main/ai/chat/use-chat-page.ts`
- Modify: `src/features/ai-chat/workflow.ts`

- [ ] **Step 1: 写一个单文件状态机测试**

同一文件覆盖：连续 delta 1..4；重复 seq 忽略；跳号进入 gap；`beginStopping` 同步冻结 `1234` 并返回 4；停止提交前 composer 禁用；网络结果不明确时使用同一 request ID/seq 重试；stopped ack 换真实 ID；ack 后允许发送 B；A 的迟到 delta 永久忽略；A 的 canceled v2 只清 A 的 settlement pending，不清 B stream；already_terminal 触发权威恢复；取消事务未提交而 completed/failed 先赢时恢复 A 的权威终态但不影响 B。

```ts
it('freezes the delivered prefix immediately and does not let A settle B', () => {
  const sessions = useConversationSessions()
  sessions.beginSend(3, 'request-a', 'count')
  for (const [seq, delta] of [[1, '1'], [2, '2'], [3, '3'], [4, '4']] as const) {
    expect(sessions.appendDelta(3, 'request-a', seq, delta)).toBe('applied')
  }
  expect(sessions.beginStopping(3, 'request-a')).toBe(4)
  expect(sessions.get(3)).toMatchObject({
    isStreaming: false,
    pendingRequestId: '',
    stopCommitPendingRequestId: 'request-a',
  })
  expect(sessions.get(3)?.messages.at(-1)).toMatchObject({
    content: '1234', delivery_state: 'stopped', isStreaming: false,
  })
  sessions.confirmStopped(3, 'request-a', 97, true)
  sessions.beginSend(3, 'request-b', 'next')
  sessions.appendDelta(3, 'request-b', 1, 'B')
  sessions.settleStopped(3, 'request-a', 97)
  expect(sessions.get(3)).toMatchObject({ pendingRequestId: 'request-b', isStreaming: true })
  expect(sessions.get(3)?.messages.at(-1)?.content).toBe('B')
})
```

- [ ] **Step 2: 运行单文件并确认失败**

Run: `npm test -- tests/component/ai/ChatStopDelivery.test.ts`

Expected: FAIL，新状态字段和序号 API 尚不存在。

- [ ] **Step 3: 重写 session 的请求级交付状态**

```ts
export interface ConversationSession {
  conversationId: number
  messages: Message[]
  nextMessageId: number
  hasMoreMessages: boolean
  loadingMessages: boolean
  loadingMoreMessages: boolean
  sending: boolean
  isStreaming: boolean
  pendingRequestId: string
  stopCommitPendingRequestId: string
  streamingContent: string
  lastContinuousDeliverySeq: number
  canceledRequestIds: string[]
  settlementPendingRequestIds: string[]
  updatedAt: number
}
```

`appendDelta` 返回 `applied | duplicate | gap | ignored`：仅 `seq===last+1` 追加；`seq<=last` 忽略；`seq>last+1` 停止追加并由 page 发起恢复。`beginStopping` 在发 HTTP 前同步把助手 placeholder 冻结为 stopped，清 `pendingRequestId/streamingContent`，关闭 `sending/isStreaming`，加入 canceled 集合，设置 stop commit pending 并返回最后连续 seq。只有 stop HTTP 明确 `stopped` 或权威恢复后才清 stop commit pending。

- [ ] **Step 4: 修改 page 与 workflow 的竞态处理**

```ts
const deliveredSeq = sessions.beginStopping(conversationId, requestId)
if (deliveredSeq === null) return
const stopInput = {
  conversation_id: conversationId,
  request_id: requestId,
  delivered_seq: deliveredSeq,
}
const result = await chatWorkflow.cancelMessage.mutate(stopInput).catch(async (error: unknown) => {
  if (!isApiError(error) || !error.retryable) throw error
  return chatWorkflow.cancelMessage.mutate(stopInput)
})
if (result.kind === 'canceled') {
  await chatWorkflow.recoverRequest(conversationId, requestId)
  return
}
const acknowledgment = assertAiStoppingAcknowledgment(result.data, conversationId, requestId)
if (acknowledgment.status === 'stopped') {
  sessions.confirmStopped(conversationId, requestId, acknowledgment.assistant_message_id, acknowledgment.settlement_pending)
} else {
  await chatWorkflow.recoverRequest(conversationId, requestId)
}
```

两次取消请求必须复用同一个 `stopInput`，绝不重新读取已变化的正文或 seq；第二次仍不明确时再走权威消息恢复。composer disable 条件只包含当前 streaming/sending、`stopCommitPendingRequestId` 和全局历史 mutation，不包含 settlement pending。canceled v2 订阅先调用 `settleStopped(conversation, request, assistant_message_id)`，再刷新会话摘要；禁止无条件调用会清全 session 的 `recoverConversation`。stopped request 永久忽略 start/delta，但 completed/failed 不能按 canceled 集合无条件丢弃：若 stop commit 尚未得到 stopped 确认，按 request ID 恢复 A 的权威终态；该 request-scoped 恢复不得清另一个 pending request。

- [ ] **Step 5: 运行状态机单文件测试**

Run: `npm test -- tests/component/ai/ChatStopDelivery.test.ts`

Expected: PASS。

- [ ] **Step 6: 提交前端停止状态机**

```bash
git add src/views/Main/ai/chat/composables/types.ts src/views/Main/ai/chat/composables/useConversationSessions.ts src/views/Main/ai/chat/use-chat-page.ts src/features/ai-chat/workflow.ts tests/component/ai/ChatStopDelivery.test.ts
git commit -m "feat(ai-chat): 立即冻结停止回复"
```

---

### Task 9: 展示停止状态、收紧逐消息交互并完成短验证

**Files:**
- Modify: `src/views/Main/ai/chat/components/MessageList/index.vue`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Generated: `src/i18n/locales/generated.ts`
- Modify: `tests/component/ai/MessageInteractions.test.ts`

- [ ] **Step 1: 先写 MessageList 交互测试**

测试 stopped 空/非空两种消息：紧邻正文显示弱状态；不显示 typing；settlement pending 时复制/非空朗读可用但反馈、重新生成、删除和配对用户编辑禁用；终态后重新生成/删除可用；stopped 永远不允许点赞。

```ts
it('renders stopped delivery without treating settlement as streaming', () => {
  const wrapper = mountList({
    messages: [{
      ...messages[1]!, content: '1234', delivery_state: 'stopped',
      settlement_pending: true, isStreaming: false,
    }],
  })
  expect(wrapper.text()).toContain('aiChat.generationStopped')
  expect(wrapper.find('.typing-dots').exists()).toBe(false)
  expect(action(wrapper, 'aiChat.copyMessage').attributes('disabled')).toBeUndefined()
  expect(action(wrapper, 'aiChat.like').attributes('disabled')).toBeDefined()
  expect(action(wrapper, 'aiChat.regenerate').attributes('disabled')).toBeDefined()
})
```

- [ ] **Step 2: 运行 MessageInteractions 并确认失败**

Run: `npm test -- tests/component/ai/MessageInteractions.test.ts`

Expected: FAIL，页面尚未展示 delivery state。

- [ ] **Step 3: 使用现有 Element Plus 文本组件展示弱状态**

不增加装饰卡片或多余布局 CSS，直接在助手正文下使用现有组件：

```vue
<el-text
  v-if="isAssistant(message) && message.delivery_state === 'stopped'"
  tag="div"
  size="small"
  type="info"
>
  {{ t('aiChat.generationStopped') }}
</el-text>
```

将历史 mutation 不可用条件改为 `isStreaming || settlement_pending || id<=0 || 全局 mutation pending`。复制仅在正文非空时可用；朗读同样只看正文；feedback 在 `delivery_state==='stopped'` 时永久禁用；selection checkbox 对 settlement pending 禁用。中文文案为“已停止生成”，英文为“Generation stopped”。

- [ ] **Step 4: 生成 locale 类型并运行单文件测试**

Run: `npm run locale:generate`

Expected: `src/i18n/locales/generated.ts` 更新。

Run: `npm test -- tests/component/ai/MessageInteractions.test.ts`

Expected: PASS。

- [ ] **Step 5: 执行允许范围内的最终自动验证**

后端逐个运行，避免全仓长测：

Run: `go test ./internal/module/ai/replycommand -run 'TestAppendDeliveryChunk|TestReadDeliveryPrefix|TestRequestCancel|TestCanceledFinalization|TestReconcilerCleansDeliveryChunks' -count=1`

Expected: PASS。

Run: `go test ./internal/module/ai/chat -run 'TestPersistentDeliverySink|TestSplitDeliveryUTF8|TestDeliveryStopDiscardsBuffer' -count=1`

Expected: PASS。

Run: `go test ./internal/module/ai/message -run 'TestCancel|TestListProjectsStopped|TestRegenerateStoppedMessage' -count=1`

Expected: PASS。

Run: `go test ./internal/runtime -run 'TestCanceledChatFinalization|TestChatFinalizationCleanupRunsAfterCommit|TestReplyDeliveryCommitter' -count=1`

Expected: PASS。

Run: `go test ./internal/admincontract -run 'TestBundle|TestOpenAPI|TestRealtime' -count=1`

Expected: PASS。

Run: `go build ./cmd/admin-api ./cmd/admin-worker`

Expected: 两个正式入口构建成功。

前端只运行相关单文件和正式检查：

Run: `npm test -- tests/component/ai/ChatStopDelivery.test.ts`

Expected: PASS。

Run: `npm test -- tests/component/ai/MessageInteractions.test.ts`

Expected: PASS。

Run: `npm test -- tests/unit/realtime/protocol.test.ts`

Expected: PASS。

Run: `npm run typecheck`

Expected: PASS。

Run: `npm run build`

Expected: Vite 正式构建成功。

- [ ] **Step 6: 提交 UI 与文案**

```bash
git add src/views/Main/ai/chat/components/MessageList/index.vue src/i18n/locales/zh-CN/ai.ts src/i18n/locales/en-US/ai.ts src/i18n/locales/generated.ts tests/component/ai/MessageInteractions.test.ts
git commit -m "feat(ai-chat): 展示已停止生成状态"
```

- [ ] **Step 7: 用户手工执行全链路与性能验收**

用户重启 `admin-dev` 后手工执行，不纳入自动脚本：

1. Fake provider/可控上游输出 `123456789`，浏览器显示到 `1234` 时停止；确认停止按钮和动画立即消失，正文固定为 `1234`。
2. 停止 HTTP 返回后、后台仍结算时立即发送 B；确认 A 的 canceled v2 到达后 B 仍继续 streaming。
3. 刷新页面；确认 A 恢复为 `1234 + 已停止生成`，不出现 `123456789`。
4. 分别验证派发前停止、正常成功先赢竞态、usage 完整、usage 缺失、停止后重新生成和软删除。
5. 在运行详情确认 stopped 助手消息、完整权威 token 和实际费用共存；stopped 消息不能点赞。
6. 对取消范围读和 cleanup SQL 执行 `EXPLAIN ANALYZE`，确认 key 为 `PRIMARY` 且 examined rows 不超过请求 seq/256 批次。
7. 手工持续流 10 分钟，记录分片数量、InnoDB 写入、purge 和展示延迟；健康本地数据库额外展示延迟目标 P95 `<75ms`，不作为抖动单测硬阈值。

最终数据库事实核对：

```sql
SELECT m.id, m.content, m.delivery_state,
       c.state, c.delivery_seq, c.stop_delivery_seq, c.assistant_message_id,
       r.status, r.assistant_message_id AS run_assistant_message_id,
       r.billing_status, r.completion_tokens,
       charge.status AS charge_status, charge.actual_units
FROM ai_reply_commands c
JOIN ai_messages m ON m.id = c.assistant_message_id
JOIN ai_runs r ON r.user_id = c.user_id AND BINARY r.request_id = BINARY c.request_id
LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id
WHERE BINARY c.request_id = BINARY '手工验收时浏览器生成的request_id';

SELECT COUNT(*) AS remaining_chunks
FROM ai_reply_delivery_chunks
WHERE command_id = (
  SELECT id FROM ai_reply_commands
  WHERE BINARY request_id = BINARY '手工验收时浏览器生成的request_id'
  LIMIT 1
);
```

Expected: `content='1234'`、`delivery_state='stopped'`、command/Run 均 canceled 且共享同一助手 ID；usage 完整时 settled 并按上游完整 usage 计费，usage 不完整时 unbilled；`remaining_chunks=0`；provider attempt 的 `result_candidate_json IS NULL`。

---

## 完成定义

- [ ] UI 停止与后台结算是两个独立状态，停止不再等待 drain。
- [ ] stopped 正文只能由服务端已提交分片 `1..delivered_seq` 构成，客户端不能提交正文。
- [ ] command、消息、Run、canceled v2 使用同一助手消息 ID。
- [ ] 完整候选不发布且终态清除；计费只使用上游权威 usage。
- [ ] delta/canceled v1 已从运行时代码、前端订阅和生成契约删除。
- [ ] 所有终态均在资金事务外触发有界清理，残留可由 reconciler 补偿。
- [ ] 相关定向测试、后端正式构建、前端 typecheck/正式构建通过。
- [ ] 长链路和性能项目已由用户按 Task 9 Step 7 手工验收。
