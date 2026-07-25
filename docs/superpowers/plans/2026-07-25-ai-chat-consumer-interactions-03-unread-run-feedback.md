# AI 会话未读与 Run 点赞 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以会话读游标提供服务端权威未读数，并为当前用户提供 Run 点赞/取消点赞。

**Architecture:** 未读数是 `last_read_message_id` 之后仍可见 AI 消息的查询派生值，不持久化计数。点赞直接属于单用户 Run；消费者写接口只做登录态和所有权校验，后台 Run 管理读取仍使用 `ai_run_list`。

**Tech Stack:** Go、GORM、MySQL monotonic update、Gin authenticated routes。

---

### Task 1: Add monotonic read state

**Files:**
- Modify: `internal/module/ai/conversation/dto.go`
- Modify: `internal/module/ai/conversation/repository.go`
- Modify: `internal/module/ai/conversation/service.go`
- Modify: `internal/module/ai/conversation/repository_test.go`
- Modify: `internal/module/ai/conversation/service_test.go`

- [ ] **Step 1: Write failing unread tests**

Cover two conversations, multiple visible assistant messages, deleted unread messages, visible user messages, cursor `0`, repeated cursor updates and an attempted backward update. Assert list pagination computes counts in one grouped query, not one query per conversation.

- [ ] **Step 2: Add `unread_count` projection**

Extend conversation list DTOs with non-negative `unread_count`. Count only `role=assistant`, `is_del=2`, `id > last_read_message_id`; streaming deltas and terminal Runs without an assistant message never enter the count. Join/group the current page of conversation IDs in one repository call.

- [ ] **Step 3: Implement monotonic cursor update**

Validate current-user conversation ownership and a current visible assistant message in that conversation, then update with `GREATEST(last_read_message_id, message_id)`. Repeated and lower requests succeed without moving backward; cross-user/cross-conversation/hidden/user-role messages return ownership-safe errors.

### Task 2: Add current-user Run feedback

**Files:**
- Create: `internal/module/ai/run/feedback.go`
- Create: `internal/module/ai/run/feedback_repository.go`
- Create: `internal/module/ai/run/feedback_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`

- [ ] **Step 1: Write ownership and idempotency tests**

Use different user IDs. Accept only an owned chat Run with `status=success` and a bound assistant message. Test `liked=true` twice, `liked=false` twice, non-chat/media Run, failed/canceled/outcome-unknown Run, missing message and another user's Run.

- [ ] **Step 2: Persist `liked_at` exactly**

For `liked=true`, set `liked_at` to service time only when it is currently null; an identical replay preserves the original timestamp. For `liked=false`, clear it; repeated false remains null. Expose `liked`/`liked_at` in Run detail. Plan B02's read projection may read the same Run column but B03 must not modify message files. Do not create a count, event, wallet write, refund or message `meta_json` mutation.

### Task 3: Register self-service routes

**Files:**
- Modify: `internal/module/ai/conversation/transport/admin/request.go`
- Modify: `internal/module/ai/conversation/transport/admin/handler.go`
- Modify: `internal/module/ai/conversation/transport/admin/route.go`
- Create: `internal/module/ai/conversation/transport/admin/handler_test.go`
- Create: `internal/module/ai/run/transport/admin/feedback_request.go`
- Modify: `internal/module/ai/run/transport/admin/handler.go`
- Modify: `internal/module/ai/run/transport/admin/route.go`
- Create: `internal/module/ai/run/transport/admin/feedback_handler_test.go`
- Modify: `internal/shared/i18n/locales/zh-CN/aiconversation.yaml`
- Modify: `internal/shared/i18n/locales/en-US/aiconversation.yaml`
- Modify: `internal/shared/i18n/locales/zh-CN/airun.yaml`
- Modify: `internal/shared/i18n/locales/en-US/airun.yaml`

- [ ] **Step 1: Add read-cursor REST contract**

Register `PUT /api/admin/v1/ai-conversations/:id/read-cursor` with `{message_id}`. Require `Authenticated()` and return exact `conversation_id`, persisted `last_read_message_id` and freshly queried `unread_count`.

- [ ] **Step 2: Add feedback REST contract with explicit permission exception**

Register `PUT /api/admin/v1/ai-runs/:id/user-feedback` with `{liked:boolean}` and `Authenticated()`, returning exact `id`, `liked` and nullable `liked_at`. Do not attach `ai_run_list`; add route-policy tests proving a logged-in owner can call feedback while management list/detail remains permission-protected after Phase A.

- [ ] **Step 3: Run focused checks and commit**

Run `gofmt -w internal/module/ai/conversation internal/module/ai/run`, then `go test ./internal/module/ai/conversation ./internal/module/ai/run -run 'Test.*Unread|Test.*Cursor|Test.*Feedback|Test.*Liked' -count=1` and `git diff --check`.

```powershell
git add internal/module/ai/conversation internal/module/ai/run internal/shared/i18n/locales/zh-CN/aiconversation.yaml internal/shared/i18n/locales/en-US/aiconversation.yaml internal/shared/i18n/locales/zh-CN/airun.yaml internal/shared/i18n/locales/en-US/airun.yaml
git commit -m "feat(ai): add unread state and run feedback"
```
