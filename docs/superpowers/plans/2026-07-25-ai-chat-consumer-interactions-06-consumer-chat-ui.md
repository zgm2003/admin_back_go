# AI 消费者聊天交互 UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有豆包式 ToC 会话界面补齐选择删除、编辑、重新生成、点赞、未读和免费朗读。

**Architecture:** 生成契约是唯一 API 类型来源；`ai-chat` workflow/composables 管理服务端状态，MessageList 只渲染并发出明确动作。视觉继续使用当前安静、无外层卡片的消息画布：把变化集中在消息旁的紧凑工具栏和底部选择操作条，不重做多智能体/会话布局。

**Tech Stack:** Vue 3、TypeScript、Element Plus icons、Web Speech API、Vitest/Vue Test Utils。

---

### Task 1: Consume the exact interaction contract

**Files:**
- Modify: `..\admin_front_ts\src\api\ai\conversations.ts`
- Modify: `..\admin_front_ts\src\api\ai\messages.ts`
- Modify: `..\admin_front_ts\src\api\ai\runs.ts`
- Modify: `..\admin_front_ts\src\features\ai-chat\workflow.ts`
- Modify: `..\admin_front_ts\tests\shared\ai\ai-conversation-api.test.ts`
- Create: `..\admin_front_ts\tests\shared\ai\ai-consumer-interactions-api.test.ts`

- [ ] **Step 1: Read frontend rules and write failing API tests**

Read `E:\admin\admin_front_ts\docs\rule.md`. Assert exact generated operations, path IDs and bodies for revision, regeneration, batch delete, read cursor and feedback. Use distinct conversation/message/Run IDs; assert no alias fields, runtime mock or fallback operation.

- [ ] **Step 2: Add typed API methods**

Map generated fields one-to-one. Require caller-supplied `request_id` for edit/regenerate, non-empty unique positive delete IDs, positive cursor message ID and explicit feedback boolean. API adapters must not invent request IDs or infer pair/Run ownership.

- [ ] **Step 3: Add workflow mutations and invalidation**

On history mutation, invalidate/recover the active message session and conversation list only after server success. Feedback updates the target assistant message optimistically but restores the exact prior state on failure. Read cursor refreshes the authoritative conversation list; it never increments/decrements a local counter.

### Task 2: Implement message actions and selection mode

**Files:**
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\components\MessageList\index.vue`
- Create: `..\admin_front_ts\src\views\Main\ai\chat\components\MessageList\MessageEditor.vue`
- Create: `..\admin_front_ts\src\views\Main\ai\chat\components\MessageSelectionBar.vue`
- Create: `..\admin_front_ts\src\views\Main\ai\chat\composables\useMessageSelection.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\composables\types.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\use-chat-page.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\index.vue`
- Create: `..\admin_front_ts\tests\component\ai\MessageInteractions.test.ts`

- [ ] **Step 1: Extend view models without guessing**

Carry `paired_message_id`, `run_id` and `liked` exactly from the published contract. Use the existing durable session/event state for local activity gating and treat the backend 409 as authoritative after reload or races. Preserve attachments/runtime params exactly; do not reconstruct them from rendered Markdown or local defaults.

- [ ] **Step 2: Build role-specific compact actions**

Assistant: copy, speak, like, regenerate, delete. User: copy, edit, delete. Use existing icon library, 28-32px stable icon buttons, tooltips, `aria-label`, keyboard focus and touch-visible access. Keep assistant content unframed and user bubbles as currently designed; do not add nested cards, feature-explanation copy or a permanent admin toolbar.

- [ ] **Step 3: Build selection and delete flow**

Opening delete selects the trigger and non-null `paired_message_id`; every checkbox remains independently editable. Render a stable message-gutter checkbox and one fixed bottom action strip with count/cancel/delete. Empty selection disables delete. Confirm once, submit exact selected IDs, then recover from server; never silently auto-add a pair at submission time.

- [ ] **Step 4: Build inline edit and regeneration**

Edit only user text in a compact inline editor; show inherited attachments read-only. Create a new request ID per user-confirmed operation and retain it when replaying the same operation after an unknown transport response. Regeneration uses its own new ID. Both enter the existing durable reply placeholder/session path after HTTP `202`.

- [ ] **Step 5: Enforce running-state ergonomics**

Disable edit/regenerate/delete while any command is `pending|claimed|running`, including stopping-but-not-terminal. Re-enable on all terminal events including `outcome_unknown`. Service errors remain authoritative and trigger a session/list recovery rather than a local compatibility state.

### Task 3: Add unread and browser speech state

**Files:**
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\components\ConversationList\index.vue`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\composables\useConversations.ts`
- Modify: `..\admin_front_ts\src\views\Main\ai\chat\composables\useConversationSocket.ts`
- Create: `..\admin_front_ts\src\views\Main\ai\chat\composables\useMessageSpeech.ts`
- Create: `..\admin_front_ts\tests\component\ai\ConversationUnread.test.ts`
- Create: `..\admin_front_ts\tests\shared\ai\message-speech.test.ts`

- [ ] **Step 1: Render authoritative unread badges**

Show `unread_count > 0` as a restrained red count badge in a fixed-width trailing slot so titles do not shift. On a non-current conversation completion, refresh the list; on current conversation recovery, send the latest visible assistant ID to read cursor and refresh. `realtime.resync_required` follows the same server recovery, never local accumulation.

- [ ] **Step 2: Implement one-owner speech controls**

Wrap `speechSynthesis` behind an injectable composable. Prefer Google Chinese, then `zh-CN`, then default voice; support start/pause/resume/stop. Starting another message, changing conversation or unmounting calls `cancel()`. Unsupported browsers expose a disabled button and short tooltip, with no network fallback.

- [ ] **Step 3: Test cleanup and race behavior**

Cover default paired selection with independent deselection, exact delete IDs, text-only edit, unique action IDs, activity gating, failed like rollback, non-current completion refresh, cursor timing, speech voice order and cleanup on switch/unmount.

### Task 4: Focused handoff

- [ ] **Step 1: Run targeted tests only**

From the frontend root run `npm test -- tests/shared/ai/ai-conversation-api.test.ts tests/shared/ai/ai-consumer-interactions-api.test.ts tests/shared/ai/message-speech.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/ai/ConversationUnread.test.ts tests/integration/features/ai-chat.test.ts`. Run typecheck only if it finishes within two minutes; do not run Playwright, full tests or build automatically.

- [ ] **Step 2: Commit**

```powershell
git add src/api/ai src/features/ai-chat src/views/Main/ai/chat tests/shared/ai tests/component/ai tests/integration/features/ai-chat.test.ts
git commit -m "feat(ai): add consumer chat interactions"
```
