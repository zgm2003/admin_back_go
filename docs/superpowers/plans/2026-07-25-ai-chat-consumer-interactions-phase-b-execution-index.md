# AI 消费者交互阶段 B 执行索引

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement these plans task-by-task. Check a step only after its stated focused verification succeeds.

**Goal:** 在阶段 A 计费与 durable Run 契约稳定后，分波完成消息历史操作、未读、Run 点赞、免费朗读、输入快照和充值页精简。

**Architecture:** 后端保持模块化单体与服务端权威状态；消息、会话/Run、payment 三条后端线分开拥有文件。契约发布是后端与前端之间的串行闸门；两个前端计划只消费生成类型，不猜字段、不直连 provider/wallet。

**Tech Stack:** Go/GORM/MySQL、Admin Contract Bundle、Vue/TypeScript、Web Speech API。

---

## Canonical Input

- 唯一业务依据：[2026-07-24-ai-chat-consumer-pricing-wallet-design.md](../specs/2026-07-24-ai-chat-consumer-pricing-wallet-design.md) 第 6.2-15 节。
- Git 历史中的 `2026-07-24-ai-chat-consumer-interactions-design.md` 及两份 pricing review 只供追溯，不能覆盖主 Spec。
- 开始 B01 前，阶段 A Wave 0-3 必须已合并，canonical `(user_id, request_id)`、Run/command 终态、Gateway 和生成契约已稳定。
- 后端 Agent 先读 `AGENTS.md`、`docs/architecture.md`、`internal/module/README.md`、`internal/platform/README.md`；B05-B07 前端步骤还必须读 `E:\admin\admin_front_ts\docs\rule.md`。

## Plan Files

| Wave | Plan | Ownership | Depends on |
| --- | --- | --- | --- |
| 0 | `2026-07-25-ai-chat-consumer-interactions-01-schema-contracts.md` | HCL、单独迁移、conversation/run 共享模型字段 | Phase A complete |
| 1 | `2026-07-25-ai-chat-consumer-interactions-02-message-history-actions.md` | message、history participant、reply-command history seam | B01 |
| 1 | `2026-07-25-ai-chat-consumer-interactions-03-unread-run-feedback.md` | conversation 已读/未读、run feedback | B01 |
| 1 | `2026-07-25-ai-chat-consumer-interactions-04-recharge-page-contract.md` | payment recharge PageInit/query 精简 | B01 |
| 2 | `2026-07-25-ai-chat-consumer-interactions-05-contract-publication.md` | curated OpenAPI、route golden、Bundle、前端 generated types | B02、B03、B04 |
| 3 | `2026-07-25-ai-chat-consumer-interactions-06-consumer-chat-ui.md` | ToC chat API/workflow/components、选择/编辑/点赞/未读/朗读 | B05 |
| 3 | `2026-07-25-ai-chat-consumer-interactions-07-run-snapshot-recharge-ui.md` | Run 快照解析/展示、充值 UI recent 删除 | B05 |

## Dependency Waves

### Wave 0: Schema serial gate

只允许 B01 修改 `database/schema/admin.hcl`、`database/migrations/*` 和 `atlas.sum`。迁移只生成与 hash，不自动连接 MySQL；用户在维护窗口决定是否执行。

### Wave 1: Three parallel backend owners

- B02 只修改 `ai/message`、新增 reply-command history participant 和其必要 runtime wiring；不修改 conversation/run/payment。
- B03 只修改 `ai/conversation` 与 `ai/run`；不修改 message/payment/runtime build。
- B04 只修改 payment recharge DTO/service/repository；不修改 wallet 入账、AI 或 contract generated files。

三条线均依赖 B01 已合并字段。若实际实现发现必须修改另一 owner 文件，先停下并调整波次，不能复制类型或跨分支同时修改。

### Wave 2: Contract serial gate

B05 等三个后端计划都合并后再运行。先关闭 schema/workflow/route policy 并提交，再从该精确 backend commit 生成 Bundle，最后前端按 manifest commit 同步。generated JSON/TS 禁止手改。

### Wave 3: Two parallel frontend owners

- B06 拥有 chat API/workflow/chat view 和 `src/api/ai/runs.ts` feedback 调用。
- B07 拥有 Run detail snapshot files、payment recharge view/API 和 payment locale。

两者不修改 generated contract。B06 使用 AI locale，B07 使用 payment locale；若共同测试文件发生冲突，由收尾 Agent 串行合并测试，不扩散运行时 ownership。

## Cross-Plan Invariants

1. 编辑/重新生成统一使用 canonical `(user_id, request_id)`；conversation/source message 是指纹内容，不是唯一键 scope。
2. 活动 command 仅 `pending|claimed|running`；cancel-requested 但未终态仍活动，`outcome_unknown` 是终态。
3. 历史操作不访问 provider/wallet；新付费动作只创建 durable command/Run，之后复用阶段 A Gateway。
4. 消息删除只改 `is_del`，不修改 Run、usage、Charge、Hold、流水或 `liked_at`，也不退款。
5. 未读数只从服务端游标查询；WebSocket 只触发刷新，前端不累计。
6. feedback 是 `Authenticated()` + Run 所有权；不挂 `ai_run_list`。后台 Run 管理读取仍受该权限保护。
7. 朗读只用浏览器 Web Speech API，不调用网络 TTS、不创建 Run。
8. 前端只使用 B05 生成字段；缺失即契约错误，不加 `a ?? b`、空数组或旧字段 fallback。

## Acceptance Gates

- 编辑只替换文字并继承服务端附件/参数；重新生成复制完整源输入；两者各创建一个新 command/Run。
- 同 ID 同指纹重放不重复切尾或派发，不同指纹返回 409。
- 默认选择准确配对，用户可取消任一条，服务端只删除提交 IDs。
- 活动生成竞态由事务拒绝；终态 unknown 不继续锁死会话。
- 非当前会话完成后 unread 增加，进入并恢复后清零，删除/重连不漂移。
- 点赞幂等、所有权隔离，消息删除不清除点赞。
- 朗读单 owner，切会话/卸载清理，完全不产生后端请求。
- 输入快照结构化且安全回退；充值收银台无 recent 查询/区域，记录 Tab 正常。

## Verification Budget

每个 Agent 只自动执行其计划列出的定向测试、格式检查、contract check 和 `git diff --check`，任何单条命令预计不超过两分钟。以下只在交付时列给用户，未经当前对话明确授权不得自动运行：

```powershell
go test ./...
go test -race ./...
Set-Location E:\admin\admin_front_ts
npm test
npm run typecheck
npm run build
# Docker、Playwright、完整 E2E 与真实上游验证由用户手动决定
```

## Execution Handoff

推荐按 Wave 为边界使用 `superpowers:subagent-driven-development`：Wave 0 一个 Agent；Wave 1 三个独立 Agent；Wave 2 一个契约 Agent；Wave 3 两个前端 Agent。并行 Agent 必须先按 `superpowers:using-git-worktrees` 建立各自 worktree，禁止在同一工作树并发提交或用宽泛 `git add` 带入其他计划文件。每个 Agent 只拿一份短计划，合并前做一次 spec compliance 和一次 code quality review，不运行额外长测试。
