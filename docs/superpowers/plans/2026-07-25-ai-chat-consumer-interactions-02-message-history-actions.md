# AI 消息历史操作 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供权威问答配对、批量软删除、编辑用户文字和重新生成，同时保留旧消息、Run 与财务审计。

**Architecture:** 消息模块拥有可见链事务，阶段 A reply-command/Run participant 拥有新付费执行身份。编辑与重新生成只在本地事务内改写可见链并创建新的 durable command/Run；提交后由现有 Runner 执行 Gateway，不在 HTTP 请求内调用 provider 或 wallet。

**Tech Stack:** Go、GORM transactions、Gin Admin transport、canonical request fingerprint。

---

### Task 1: Extend the message read projection

**Files:**
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/service.go`
- Test: `internal/module/ai/message/service_test.go`

- [ ] **Step 1: Write failing projection tests**

Cover user/assistant pairing from reply-command IDs, orphan messages with `paired_message_id=null`, assistant `run_id/liked`, user `run_id=null/liked=false`, and a bounded batch query for a page of messages. Use non-adjacent message IDs so adjacency guessing fails.

- [ ] **Step 2: Add the exact response fields**

Add nullable `paired_message_id`, nullable `run_id` and boolean `liked`. Resolve them with one page-level projection query/join; do not issue one query per message and do not infer pairs from sort order.

- [ ] **Step 3: Run the projection slice**

Run `go test ./internal/module/ai/message -run 'Test.*Pair|Test.*Projection|Test.*List' -count=1`.

### Task 2: Implement one linear-history transaction

**Files:**
- Create: `internal/module/ai/message/history_actions.go`
- Create: `internal/module/ai/message/history_repository.go`
- Create: `internal/module/ai/message/history_actions_test.go`
- Create: `internal/module/ai/replycommand/history_participant.go`
- Create: `internal/module/ai/replycommand/history_participant_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`

- [ ] **Step 1: Define focused inputs and participant**

Define edit `{user_id, conversation_id, message_id, content, request_id}`; regenerate `{user_id, conversation_id, assistant_message_id, request_id}`; delete `{user_id, conversation_id, ids}`. The reply participant accepts the caller transaction plus the typed Phase A request identity and creates exactly one new user message/command/Run identity. It must not expose GORM to handlers or call provider/wallet.

- [ ] **Step 2: Test canonical replay before history mutation**

Same `(user_id, request_id)` and equal fingerprint returns the original accepted result without re-cutting history. Same ID with a changed operation, conversation, source message, content, attachment/parameter snapshot or agent returns `409`. `conversation_id` is fingerprint data, not the unique-key scope.

- [ ] **Step 3: Implement edit and regenerate**

Lock and verify the owned conversation, reject active commands in `pending|claimed|running` (including cancel-requested commands not yet terminal), capture the visible tail upper bound, and validate source visibility/role. Edit copies server-side `meta_json` and replaces only text; regenerate copies the paired user's complete content/meta. Soft-delete the old visible tail, create a new user message and new command/Run through the participant, update `last_message_at`, commit, then wake the existing Runner. `outcome_unknown` is terminal and must not count as active.

- [ ] **Step 4: Implement exact-ID batch soft delete**

Reject empty, duplicate, non-positive, cross-conversation, missing or already-hidden IDs and active commands. Soft-delete only submitted IDs, recompute `last_message_at` from remaining visible messages, and never touch Run, usage, Charge, Hold, wallet transaction or `liked_at` rows.

- [ ] **Step 5: Test rollback and audit preservation**

Cover edit-only-text inheritance, regeneration after missing pair, arbitrary single-message delete, default-pair-independent delete semantics, transaction rollback, concurrent active-command race, old Run/charge preservation and post-commit Runner wakeup.

### Task 3: Publish authenticated REST handlers

**Files:**
- Modify: `internal/module/ai/message/transport/admin/request.go`
- Modify: `internal/module/ai/message/transport/admin/handler.go`
- Modify: `internal/module/ai/message/transport/admin/route.go`
- Modify: `internal/module/ai/message/transport/admin/handler_test.go`
- Create: `internal/shared/i18n/locales/zh-CN/aimessage.yaml`
- Create: `internal/shared/i18n/locales/en-US/aimessage.yaml`

- [ ] **Step 1: Register the three contracts**

Add `POST .../:message_id/revisions`, `POST .../:message_id/regenerations` and collection `DELETE .../messages`, all with `Authenticated()` and explicit operation metadata. Return HTTP `202` plus the existing `AIMessageSendResult` shape for edit/regenerate, sorted unique `deleted_ids` for delete, 409 machine errors for active generation/fingerprint conflict, and ownership-safe 404s.

- [ ] **Step 2: Run only this slice**

Run `gofmt -w internal/module/ai/message internal/module/ai/replycommand/history_participant*.go internal/platform/admin/build*.go`, then `go test ./internal/module/ai/message ./internal/module/ai/replycommand ./internal/platform/admin -run 'Test.*History|Test.*Revision|Test.*Regenerat|Test.*Delete|Test.*Build' -count=1` and `git diff --check`.

- [ ] **Step 3: Commit**

```powershell
git add internal/module/ai/message internal/module/ai/replycommand/history_participant* internal/platform/admin/build.go internal/platform/admin/build_test.go internal/shared/i18n/locales/zh-CN/aimessage.yaml internal/shared/i18n/locales/en-US/aimessage.yaml
git commit -m "feat(ai): add consumer message history actions"
```
