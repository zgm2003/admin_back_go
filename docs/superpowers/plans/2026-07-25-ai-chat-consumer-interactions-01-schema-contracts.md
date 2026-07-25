# AI 消费者交互 Schema 契约 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为阶段 B 建立已读游标、消息查询索引和 Run 点赞的最小数据库事实。

**Architecture:** 只扩展现有 `ai_conversations`、`ai_messages`、`ai_runs`，不创建版本树、点赞表或未读明细表。迁移在阶段 A 稳定后单独执行，应用启动不自动迁移，也不创建备份数据库。

**Tech Stack:** MySQL、Atlas HCL/SQL、GORM、architecture contract tests。

---

### Task 1: Lock the Stage B schema

**Files:**
- Modify: `database/schema/admin.hcl`
- Create: `database/migrations/202607250201_ai_consumer_interactions.sql`
- Modify: `database/migrations/atlas.sum`
- Create: `internal/architecture/ai_consumer_interactions_schema_test.go`

- [ ] **Step 1: Write the failing schema contract test**

Assert the HCL and migration contain `ai_conversations.last_read_message_id BIGINT UNSIGNED NOT NULL DEFAULT 0`, nullable `ai_runs.liked_at DATETIME(6)`, and one index on `ai_messages(conversation_id,is_del,role,id)`. Assert the migration does not create a table, permission, role grant, trigger, backup database or destructive data rewrite.

- [ ] **Step 2: Run the focused test and observe failure**

Run `go test ./internal/architecture -run TestAIConsumerInteractionsSchema -count=1`; expected failure names the missing columns/index.

- [ ] **Step 3: Add HCL and guarded migration**

Use deterministic, collision-checked DDL consistent with current Atlas migrations. Existing conversations backfill to cursor `0`; existing Runs keep `liked_at=NULL`. The message index must use the repository's real `is_del`/role values without changing rows. Do not add an FK from the cursor to messages.

- [ ] **Step 4: Hash without applying**

Run `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`. If the Atlas container cannot complete within two minutes, stop it and leave this command for the user; never apply the migration in this plan.

### Task 2: Publish shared model fields

**Files:**
- Modify: `internal/module/ai/conversation/model.go`
- Modify: `internal/module/ai/run/model.go`
- Test: `internal/module/ai/conversation/repository_test.go`
- Test: `internal/module/ai/run/repository_test.go`

- [ ] **Step 1: Map only the new facts**

Add `LastReadMessageID int64` with `column:last_read_message_id` and `LikedAt *time.Time` with `column:liked_at`, matching the repository's existing signed Go ID convention for MySQL unsigned IDs. Do not add derived `unread_count` to a persistence model and do not overload `meta_json`.

- [ ] **Step 2: Verify model/query parsing**

Add focused GORM schema assertions and run `go test ./internal/module/ai/conversation ./internal/module/ai/run -run 'Test.*Model|Test.*Repository' -count=1`.

### Task 3: Fast handoff

- [ ] **Step 1: Run light checks**

Run `gofmt -w internal/architecture/ai_consumer_interactions_schema_test.go internal/module/ai/conversation/model.go internal/module/ai/run/model.go` and `git diff --check`. Do not run MySQL, Docker or full backend tests.

- [ ] **Step 2: Commit**

```powershell
git add database/schema/admin.hcl database/migrations internal/architecture/ai_consumer_interactions_schema_test.go internal/module/ai/conversation/model.go internal/module/ai/run/model.go internal/module/ai/conversation/repository_test.go internal/module/ai/run/repository_test.go
git commit -m "feat(ai): add consumer interaction schema"
```
