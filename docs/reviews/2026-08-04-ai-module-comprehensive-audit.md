# AI 核心模块综合审计报告

> 审计日期：2026-08-04  
> 审计仓库：`E:/admin/admin_back_go`、`E:/admin/admin_front_ts`  
> 审计方式：源码静态审查、现行 HCL 审查、运行中 MySQL 只读查询、容器状态核对  
> 本次动作：只新增本报告；未修改业务代码，未删除字段，未执行迁移，未运行 Playwright、全量测试或长测试脚本  
> 明确排除：暂不把“未配置可用向量/Embedding 模型”列为缺陷，也不评价向量召回效果

## 0. 结论先行

【结论】

AI 模块不是“整体架构已经坏掉”，相反，它已经形成了一套相当完整的业务闭环：会话、持久化命令、Run、Provider Attempt、上下文计划、附件事实、工具调用、钱包预占、用量结算、支付充值、Realtime 恢复和 Dashboard 投影都已存在。最有价值的设计是“先落事实，再执行外部副作用”，以及 MySQL 作为业务事实源、Qdrant 只作为可重建派生索引。

但当前仍有一处确定性 P0、六类 P1 正确性风险和若干容量/数据约束缺口。最严重的不是“字段太多”，而是少数核心关系只在 Go 代码中成立、数据库并不知道这些关系；一旦某条异常路径绕过 Service，错误数据可以合法写入。前端 Realtime 还存在“处理失败却确认消费”的语义错误，这会直接制造刷新后状态丢失。

本次审计的核心判断如下：

| 判断 | 结论 |
| --- | --- |
| 是否需要推倒重做 | 不需要。应保留现有模块边界和持久化状态机，修复关系与确认语义 |
| 是否存在可直接删除的大量字段 | 没有。Run、Attempt、Plan、支付订单中的多数重复字段是历史快照或证据，不能按“重复”删除 |
| 是否存在明确冗余 | 有。`ai_agents` 的 Provider/Model 三字段是高置信迁移候选；两组索引是高置信冗余候选 |
| 是否应立即加一堆索引 | 不应。当前数据量太小，先修正确性，再用生产级查询和 `EXPLAIN ANALYZE` 决定 |
| 是否应直接删表/删字段 | 不应。统一采用 `expand -> 数据扫描与 backfill -> dual-read / verify -> contract` |
| 没有向量模型是否阻塞本报告结论 | 不阻塞。本报告评价关系事实、状态机、附件、支付、Realtime 和容量，不评价向量效果 |

优先级汇总：

| 级别 | 数量 | 含义 |
| --- | ---: | --- |
| P0 | 1 | 已在真实 MySQL 复现，正常管理操作必然 500 |
| P1 | 6 | 会造成持久事件丢失、后台误执行、附件状态污染、支付幂等不足或关系事实失真 |
| P2 | 9 | 数据量增长、多标签页、缓存上限、状态建模和查询放大风险 |
| P3 | 3 | 可维护性、文档漂移和可验证的索引清理候选 |

## 1. 审计范围与证据等级

### 1.1 覆盖范围

本次覆盖以下链路：

| 领域 | 主要事实 |
| --- | --- |
| AI 对话 | `ai_conversations`、`ai_messages`、消息恢复、停止与已读游标 |
| 持久执行 | `ai_reply_commands`、`ai_runs`、`ai_provider_attempts`、`ai_run_events` |
| 上下文工程 | Profile、Space、Document、Version、Chunk、Binding、Plan、Plan Item、Memory |
| Provider | Provider、Provider Model、官方模型映射、API Protocol |
| Agent / Tool | Agent 模型绑定、Agent Tool 绑定、Tool Call |
| 附件 / COS | 浏览器选择、上传 Token、COS 上传、服务端 HEAD、历史附件 |
| AI 计费 | Wallet Hold、Usage Charge、Charge Item、价格快照、结算终态 |
| 支付 | 支付配置、订单、充值、支付宝异步回调、钱包入账 |
| Realtime | WebSocket Session、durable event、resume、cursor、resync |
| 前端状态 | Vue 会话缓存、刷新恢复、附件 Promise、Realtime handler |
| 数据库 | 字段、FK、CHECK、唯一键、普通索引、软删除唯一键 |

### 1.2 证据等级

| 等级 | 定义 | 本报告示例 |
| --- | --- | --- |
| E1 | 真实运行环境可重复复现 | `ai_conversation_memories.agent_id` 查询返回 MySQL 1054 |
| E2 | 真实数据库结构或数据扫描 | FK/索引清单、精确行数、孤儿和跨归属扫描 |
| E3 | 当前 master 源码确定路径 | Realtime 吞错后推进 cursor、删除会话不取消 Command |
| E4 | 基于当前查询形态的容量推断 | N+1、复杂 OR claim、索引候选；需要生产数据验证 |

### 1.3 运行快照

快照时间为 `2026-08-04 12:32:52 +08:00`：

| 组件 | 版本/状态 |
| --- | --- |
| MySQL | `8.4.10`，容器 `admin-state-mysql-1`，healthy |
| Redis | `8.2.7-alpine`，容器 `admin-state-redis-1`，healthy |
| Qdrant | `1.18.3`，容器 `admin-state-qdrant-1`，healthy |
| 后端分支 | `master...origin/master`，审计开始时干净 |
| 前端分支 | `master...origin/master`，审计开始时干净 |

现行 Schema 中共有 34 张 `ai_%` 表。以下是精确 `COUNT(*)`，不是 `information_schema.TABLES.table_rows` 的估算值：

| 表 | 行数 |
| --- | ---: |
| `ai_runs` | 28 |
| `ai_reply_commands` | 28 |
| `ai_provider_attempts` | 28 |
| `ai_messages` | 43 |
| `ai_context_chunks` | 307 |
| `ai_run_events` | 109 |
| `realtime_events` | 26 |
| `wallet_holds` | 26 |
| `wallet_transactions` | 17 |

这些数据足以验证关系和功能性错误，但远远不足以证明生产大数据量下的执行计划稳定。任何“索引已经足够”或“某索引一定没用”的绝对判断都不成立。

## 2. 需求分析

### 【需求判断】

这是真问题，不是为了架构图而制造的问题。

用户已经经历过“AI 已回复但刷新后消失”“Run 仍显示运行中”“带附件上下文异常”等状态闭环问题。AI 又直接关联支付和钱包，任何持久状态不一致都可能从 UI 问题扩大为计费争议。审计字段、约束和索引有明确业务价值。

### 【核心问题】

真正需要解决的不是“表太多”，而是下面四件事：

1. 一个用户请求是否能从 Message、Command、Run、Attempt、Plan、Charge 一路闭合到同一事实。
2. 外部副作用发生前，权限、Lease、取消状态和来源事实是否再次校验。
3. 刷新、断线、重试和多标签页时，前端是否只确认真正处理成功的 durable event。
4. 数据库是否能拒绝跨用户、跨会话、跨 Run 的非法组合，而不是完全相信应用代码。

### 【复杂度检查】

现有复杂度大部分来自真实问题：模型调用不可回滚、Provider 结果可能不确定、钱包需要预占和结算、附件需要可信 HEAD、上下文需要可复现证据。Run、Attempt、Plan、Charge 不是多余抽象。

不合理复杂度主要集中在：

- Command claim 用一个巨大 OR 表达多类领取任务。
- Agent 通过 `provider_id + model_id` 字符串关系寻找 Provider Model。
- Binding 表同时像当前关系表，又像没有操作者的伪历史表。
- 前端 durable event 的 handler 与 cursor 确认没有统一成功语义。

### 【破坏性分析】

直接删字段、改 API 返回、强行重建表都会破坏现有会话、Run、计费证据和管理端 DTO。尤其不能删除 Run/Order 中看似重复的快照字段，因为历史记录必须在 Agent、Provider、价格和支付配置修改后仍可解释。

所有数据库收敛必须按以下顺序：

```text
expand
-> 数据扫描与 backfill
-> dual-read / verify
-> contract
```

## 3. 当前架构判断

### 【数据结构】

核心数据结构总体方向合理：

```text
Conversation
  -> User Message
  -> Reply Command
  -> Run
  -> Context Plan
  -> Provider Attempt(s)
  -> Assistant Message
  -> Usage Charge / Wallet Hold
  -> Durable Realtime Event
```

上下文工程的数据结构也有清晰分层：

```text
Profile
  -> Space
  -> Document
  -> Document Version
  -> Chunk

Agent -> Binding -> Space
Run -> Context Plan -> Plan Item
Conversation -> Memory chain
```

问题在于图中的若干边只存在于代码，没有数据库约束。

### 【特殊情况】

最需要消灭的特殊情况是：

- Command、Run、Message 通过多个可变字段“碰巧属于同一个请求”。
- Provider Attempt 的 Plan “碰巧属于同一个 Run”。
- Document 的 source message “碰巧属于声明的 Conversation”。
- Realtime handler 失败被当作成功。
- 删除会话后后台任务仍把它当作正常会话。

这些不该继续靠更多 `if` 维护，应通过直接 FK、组合 FK、唯一键和统一确认语义消灭。

### 【复杂度】

状态维度多，但大多正交：执行状态、派发状态、用量状态、计费状态不能合并成一个万能状态。应简化的是关系和查询，不是压扁必要状态。

### 【兼容性】

必须保留：

- 现有 API JSON 字段和含义。
- 旧 Message/Run/Order 可读取。
- 已持久化 request id、fingerprint、price snapshot、plan hash。
- 老客户端在 Realtime 协议升级期间仍能 resync。
- 已上传对象和历史附件仍可作为上下文事实读取。

### 【结论】

值得做，而且应分阶段修。不要推倒重写，不要先删表，不要先追求索引数量最少。

## 4. P0/P1 问题清单

| ID | 级别 | 证据 | 问题 | 直接影响 |
| --- | --- | --- | --- | --- |
| AI-001 | P0 | E1 | 修改 Agent 上下文配置查询不存在的 `agent_id` | 接口稳定返回 500 |
| RT-001 | P1 | E3 | durable handler 报错被吞，cursor 仍推进 | 事件永久漏消费 |
| RT-002 | P1 | E3 | resume 最多 500 条且无继续/截断信号 | 积压超过 500 后无法补齐 |
| AI-002 | P1 | E3 | 删除会话不取消后台 Command | 删除后仍调用模型、计费、写终态 |
| UP-001 | P1 | E3 | 附件上传迟到 Promise 可回写已删除/已切换输入 | 当前输入被旧任务污染，COS 留孤儿 |
| PAY-001 | P1 | E2/E3 | 支付回调去重和交易号唯一性不足 | 审计重复、交易串单防线不足 |
| DB-001 | P1 | E2 | Command/Run/Message/Agent 等核心边缺少数据库约束 | 跨归属坏数据可合法落库 |

### 4.1 AI-001：上下文配置必然 500

问题代码在 [E:/admin/admin_back_go/internal/module/ai/contextengine/repository.go:413](/E:/admin/admin_back_go/internal/module/ai/contextengine/repository.go#L413)。SQL 的第三个 `EXISTS` 查询：

```sql
SELECT 1
FROM ai_conversation_memories
WHERE agent_id = ?
```

但现行 `ai_conversation_memories` 只有 `conversation_id`，并没有 `agent_id`。表结构见 [E:/admin/admin_back_go/database/schema/admin.hcl:1355](/E:/admin/admin_back_go/database/schema/admin.hcl#L1355)。真实 MySQL 已复现：

```text
ERROR 1054 (42S22): Unknown column 'agent_id' in 'where clause'
```

入口会把该错误转成“检查 AI 智能体上下文引用失败”，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/admin_service.go:490](/E:/admin/admin_back_go/internal/module/ai/contextengine/admin_service.go#L490)。

正确修复不是给 Memory 补一个冗余 `agent_id`。Memory 的所有者是 Conversation；Agent 应通过 `memory.conversation_id -> conversation.agent_id` 推导。应基于 Conversation、有效 Profile、Binding、Conversation Document 和 Memory 链做一次关系查询，并增加真实 Schema 回归测试。

### 4.2 RT-001：处理失败仍确认消费

前端订阅发布器捕获每个 handler 错误，只调用 `onHandlerError`，最终 Promise 仍成功，见 [E:/admin/admin_front_ts/src/modules/realtime/subscriptions.ts:43](/E:/admin/admin_front_ts/src/modules/realtime/subscriptions.ts#L43)。

客户端收到 durable event 后先 `publish`，随后无条件写 cursor，见 [E:/admin/admin_front_ts/src/modules/realtime/client.ts:346](/E:/admin/admin_front_ts/src/modules/realtime/client.ts#L346)。由于 `publish` 已吞错，业务恢复失败也会执行：

```ts
this.cursorStore.set(envelope.sequence)
```

这违反 durable event 的最基本规则：只有业务处理成功才能确认。修复时应让 durable publish 传播聚合错误，cursor 只在全部权威 handler 成功后推进。纯日志型 observer 可以单独使用 best-effort 通道，不能与权威恢复 handler 共用确认语义。

### 4.3 RT-002：Resume 静默截断

后端硬上限为 500，见 [E:/admin/admin_back_go/internal/module/realtime/repository.go:23](/E:/admin/admin_back_go/internal/module/realtime/repository.go#L23)。查询只执行一次 `LIMIT`，见 [E:/admin/admin_back_go/internal/module/realtime/repository.go:193](/E:/admin/admin_back_go/internal/module/realtime/repository.go#L193)，Service 也只返回这一页，见 [E:/admin/admin_back_go/internal/module/realtime/service.go:123](/E:/admin/admin_back_go/internal/module/realtime/service.go#L123)。

最低风险修复不是立刻设计复杂分页协议。查询 `limit + 1`，一旦发现还有更多积压，就直接返回现有 `realtime.resync_required.v1`，让客户端走权威 HTTP 恢复。这样旧协议已有处理路径，不会静默丢第 501 条之后的事件。以后只有确认 500 条 resync 成本过高，才增加 continuation cursor。

### 4.4 AI-002：删除会话与后台执行脱节

删除会话只软删 Conversation 和 Message，见 [E:/admin/admin_back_go/internal/module/ai/conversation/repository.go:216](/E:/admin/admin_back_go/internal/module/ai/conversation/repository.go#L216)。它没有锁定或取消活跃 Reply Command。

Command claim 的查询只判断 Command 自身状态、重试条件和 Lease，没有检查 Conversation 是否仍有效，见 [E:/admin/admin_back_go/internal/module/ai/replycommand/repository.go:445](/E:/admin/admin_back_go/internal/module/ai/replycommand/repository.go#L445)。

结果是会话从用户界面消失后，Worker 仍可能：

- 领取 Command；
- 调用 Provider；
- 消耗或结算余额；
- 尝试写 Assistant Message；
- 最后才在其他关系处失败。

修复必须同时覆盖三层：删除事务写取消意图；claim 排除已删除 Conversation；dispatch guard 在 Provider 派发前再次拒绝已删除 Conversation。已经派发到上游的请求不能假装“从未发生”，仍要按真实 usage 做结算，只是不允许复活已删除聊天 UI。

### 4.5 UP-001：附件上传迟到回写

上传函数捕获了最初的 `item` 引用，经过 Token 请求和 COS 上传两个 `await` 后直接回写，见 [E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageInput/use-attachments.ts:268](/E:/admin/admin_front_ts/src/views/Main/ai/chat/components/MessageInput/use-attachments.ts#L268)。

期间如果用户删除附件、清空输入、切换 Agent 或切换会话，旧 Promise 仍可把 URL、object key 和 `uploaded` 状态写回旧对象。当前没有：

- `AbortController`；
- 输入 generation/epoch；
- 每个 `await` 后重新按 ID 查当前项；
- Agent/会话身份复核；
- 已上传但未提交对象的回收策略。

推荐以 `attachment.id + inputGeneration` 作为提交令牌。每个异步阶段完成后重新查 Map，令牌不一致就禁止回写。若 COS 已成功而令牌失效，将 object key 登记为可回收对象；由短 TTL 或后台清理处理，不能在浏览器里假装删除一定成功。

### 4.6 PAY-001：支付回调幂等边界不完整

回调先单独创建审计事件，失败后把 `eventID` 设为 0 并继续支付处理，见 [E:/admin/admin_back_go/internal/module/payment/callback_service.go:14](/E:/admin/admin_back_go/internal/module/payment/callback_service.go#L14)。审计更新也是另一个独立写操作，见 [E:/admin/admin_back_go/internal/module/payment/callback_repository.go:27](/E:/admin/admin_back_go/internal/module/payment/callback_repository.go#L27)。

当前结构问题：

- `payment_callback_events(provider, notify_id)` 只是普通索引，不是唯一约束，见 [E:/admin/admin_back_go/database/schema/admin.hcl:5138](/E:/admin/admin_back_go/database/schema/admin.hcl#L5138)。
- `payment_orders.alipay_trade_no` 没有唯一约束，见 [E:/admin/admin_back_go/database/schema/admin.hcl:5310](/E:/admin/admin_back_go/database/schema/admin.hcl#L5310)。
- 审计创建、订单/充值/钱包结算、审计终态不在同一事务事实中。

正面事实是 `FinalizePaidOrder` 会锁订单，并在一个事务内完成订单、充值和钱包动作，见 [E:/admin/admin_back_go/internal/module/payment/recharge_repository.go:204](/E:/admin/admin_back_go/internal/module/payment/recharge_repository.go#L204)。这一部分应保留。

正确收敛方式：外部签名验证在事务外；验证后进入一个本地事务，锁回调事件和订单，幂等结算并更新审计终态。`notify_id` 为空时不能用空字符串直接做唯一键，可归一化为 NULL 或生成稳定 dedupe key。`alipay_trade_no` 也应从空字符串迁移为 nullable 非空事实，再加唯一约束。

### 4.7 DB-001：核心关系只靠应用代码

真实数据库扫描确认当前样本没有孤儿，但约束仍缺失。缺失的主要关系包括：

- `ai_provider_models.provider_id -> ai_providers.id`；
- `ai_agents` 到 Provider Model 的直接关系；
- `ai_conversations.agent_id -> ai_agents.id`；
- `ai_reply_commands` 到 Run、Conversation、User Message、Assistant Message；
- `ai_provider_attempts.command_id -> ai_reply_commands.id`；
- Attempt 的 `context_plan_id` 与 `run_id` 属于同一 Plan/Run；
- Document 的 `source_message_id` 与 `conversation_id` 属于同一会话；
- Memory 的 from/through/previous 都属于同一 Conversation/Profile；
- `ai_image_files.task_id` 和 `related_file_id`；
- Text Task、Image Task 与 Run 的一对一关系。

当前类型也阻碍直接加 FK：Conversation ID/Agent ID 是 `INT UNSIGNED`，Agent 主键是 `BIGINT UNSIGNED`；Reply Command 的关系字段是有符号 `BIGINT`，Message/Run 的主键是 `BIGINT UNSIGNED`。现行定义可见 [E:/admin/admin_back_go/database/schema/admin.hcl:1503](/E:/admin/admin_back_go/database/schema/admin.hcl#L1503)、[E:/admin/admin_back_go/database/schema/admin.hcl:2523](/E:/admin/admin_back_go/database/schema/admin.hcl#L2523) 和 [E:/admin/admin_back_go/database/schema/admin.hcl:2811](/E:/admin/admin_back_go/database/schema/admin.hcl#L2811)。

不能直接 `ALTER` 完事。先统一类型，扫描坏数据，再补关系。

## 5. AI 对话、Command 与 Run 状态机

### 5.1 应保留的设计

以下设计正确，不应因字段多而删除：

| 设计 | 理由 |
| --- | --- |
| `request_id + request_fingerprint + idempotency_key` | 分别解决客户端身份、内容一致性和外部/内部幂等 |
| Command Lease | Worker 崩溃后可恢复，避免多个 Worker 同时执行 |
| Run 执行状态与 billing 状态分离 | Provider 已结束不代表钱包结算已完成 |
| Attempt 独立表 | 一次 Run 可有多次准备/派发，且外部结果可能未知 |
| `prepared_request_json + SHA256` | 证明重试发送的是同一请求 |
| Run 的模型/价格/输入快照 | 配置变化后仍可解释历史消费 |
| `outcome_unknown` | 外部副作用无法可靠确认时不能伪装成功或失败 |

Provider 派发前的 Guard 会重新核对 Plan、Attempt、prepared request hash、Command Lease、Run 和取消状态，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/dispatch_guard.go:54](/E:/admin/admin_back_go/internal/module/ai/contextengine/dispatch_guard.go#L54)。这是整个 AI 架构中质量最高的部分之一，应作为其他链路的标准。

### 5.2 Command 与 Run 应建立直接关系

`ai_reply_commands` 当前没有 `run_id`。Command 和 Run 依靠 `user_message_id`、request identity 以及 Attempt 间接闭合。数据结构过于绕。

推荐 expand 一个 nullable `run_id BIGINT UNSIGNED`：

1. 通过唯一的 `ai_runs.user_message_id = ai_reply_commands.user_message_id` backfill。
2. 双写并验证 `request_id/user/conversation/message` 一致。
3. 加唯一 FK，明确一个 Reply Command 对应一个 Run。
4. 是否最终删除 Command 中某些重复字段，要等查询和恢复路径切换后再决定。

短期不要删除 `user_message_id`，因为它仍是业务幂等和消息归属的重要锚点。

### 5.3 状态 CHECK 还不够完整

现有状态枚举 CHECK 很好，例如 Run 状态、billing 状态和 reason 已封闭，见 [E:/admin/admin_back_go/database/schema/admin.hcl:3055](/E:/admin/admin_back_go/database/schema/admin.hcl#L3055)。但还可补以下不变量：

| 表 | 建议不变量 |
| --- | --- |
| `ai_messages` | `role IN (1,2)`；当前 delivery CHECK 不能阻止 role=3 |
| `ai_reply_commands` | `max_attempts > 0`；Lease owner/expiry 成对；终态与 `finished_at` 成对 |
| `ai_runs` | `total_tokens = prompt_tokens + completion_tokens`；终态与 `finished_at/duration_ms` 一致 |
| `ai_provider_attempts` | prepared/dispatched/terminal timestamp 形状与 state 一致 |
| `payment_orders` | amount 正数；paid/closed 时间和 status 一致 |
| `payment_callback_events` | signature flag、process status 为封闭集合 |
| `wallet_transactions` | direction 封闭；credit/debit 的 before/after 算术一致 |

这些 CHECK 必须先跑全量扫描，不能拿新约束直接撞历史数据。

### 5.4 Claim 查询复杂度

领取查询包含多组错误码、Attempt `EXISTS`、取消和过期 Lease 的大 OR，见 [E:/admin/admin_back_go/internal/module/ai/replycommand/repository.go:459](/E:/admin/admin_back_go/internal/module/ai/replycommand/repository.go#L459)。当前 `idx_ai_reply_claim(state,next_attempt_at,lease_expires_at,id)` 只能帮助其中一部分分支。

不建议现在盲加“覆盖所有字段”的巨型索引。正确顺序：

1. 收集生产条件下各 claim 分支命中比例。
2. 对真实 SQL 做 `EXPLAIN ANALYZE`。
3. 如果 OR 导致扫描放大，拆成正常 pending、过期 lease、finalization/recovery 三类有限查询。
4. 保持每类查询 `FOR UPDATE SKIP LOCKED` 和统一排序。

只有一种真实高频路径时，不要为所有理论分支设计万能索引。

## 6. 上下文工程九张表

### 6.1 总体结论

九张表不是过度设计。它们分别承担配置、归属、不可变版本、可重建 chunk、运行证据和会话压缩记忆，生命周期不同，合并反而会制造更多 nullable 特殊情况。

| 表 | 所有者与生命周期 | 结论 |
| --- | --- | --- |
| `ai_context_profiles` | Embedding/检索/Memory 配置和索引 generation | 保留 |
| `ai_context_spaces` | 平台内正式知识空间 | 保留 |
| `ai_context_documents` | 文档逻辑身份，指向 active version | 保留 |
| `ai_context_document_versions` | 不可变来源与处理状态 | 保留 |
| `ai_context_chunks` | MySQL 中可审计、可重建的 chunk 事实 | 保留 |
| `ai_context_bindings` | Agent 与 Space 当前关系 | 需要收敛语义 |
| `ai_context_plans` | 每个 Run 的上下文预算与哈希证据 | 必须保留 |
| `ai_context_plan_items` | 每个上下文块的选择/排除证据 | 必须保留 |
| `ai_conversation_memories` | Conversation 的摘要链 | 保留，修复错误查询和组合归属 |

### 6.2 Profile

Profile 同时有 `status` 和 `index_state`，不是重复：前者是配置生命周期，后者是派生索引生命周期。generation 的 CHECK 能表达 provisioning、ready、rebuilding、failed 的合法形状，见 [E:/admin/admin_back_go/database/schema/admin.hcl:477](/E:/admin/admin_back_go/database/schema/admin.hcl#L477)。

本报告不把未配置向量模型当成缺陷。但如果产品允许“仅直接历史、附件和 Memory，不启用向量检索”的正式模式，未来可单独设计无向量 Profile；不要通过空 ID 或默认模型伪装。

### 6.3 Space 与 Document

Document 用 owner CHECK 明确“Space 文档”与“Conversation 附件文档”二选一，见 [E:/admin/admin_back_go/database/schema/admin.hcl:705](/E:/admin/admin_back_go/database/schema/admin.hcl#L705)。这是消灭 nullable 组合歧义的好设计。

`status` 与 `deleted_at` 也有不同语义：disabled 可恢复，deleted 是墓碑。不能简单合并。

风险是 source message 的 FK 只证明 Message 存在，不证明它属于 `document.conversation_id`。应加组合归属约束。现有 `active_version` 已使用 `(document.id, active_version_id) -> (version.document_id, version.id)` 的组合 FK，见 [E:/admin/admin_back_go/database/schema/admin.hcl:680](/E:/admin/admin_back_go/database/schema/admin.hcl#L680)，同样方法可以复用到 source message。

### 6.4 Version 与 Chunk

Version 的来源 key、ETag、size、MIME、parser/chunker version、hash、Lease 和终态字段都解决真实问题。管理员创建文档和新版本时会重新 HEAD 对象，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/admin_service.go:320](/E:/admin/admin_back_go/internal/module/ai/contextengine/admin_service.go#L320)。这些字段不是冗余。

Chunk 是 MySQL 事实，Qdrant point 只是派生检索数据。候选返回后还会回到 MySQL 批量读取并校验权限/版本，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/candidate_repository.go:192](/E:/admin/admin_back_go/internal/module/ai/contextengine/candidate_repository.go#L192)。Profile rebuild 也从 MySQL Version/Chunk 重建，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/ingestion.go:593](/E:/admin/admin_back_go/internal/module/ai/contextengine/ingestion.go#L593)。这一原则必须保留。

### 6.5 Binding

当前更新实现直接删除 Agent 的所有 Binding 再插入新关系，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/repository.go:365](/E:/admin/admin_back_go/internal/module/ai/contextengine/repository.go#L365)。因此 `status=disabled`、`created_at`、`updated_at` 并没有形成真实历史。

必须做一个明确产品决定：

| 需求 | 数据结构 |
| --- | --- |
| 只需要当前绑定 | 主键/唯一键直接使用 `(agent_id,space_id)`，删除 `id/status/created_at/updated_at` 候选；反向索引 `(space_id,agent_id)` |
| 需要审计谁在何时改了绑定 | 当前表仍只保存有效关系；另建有 actor、operation、occurred_at 的事件表 |

不要保留一个没有操作者、没有操作类型、还会被 DELETE 的“伪历史”。

### 6.6 Plan 与 Plan Item

每个 Run 最多一个 Plan 的唯一约束已经存在，见 [E:/admin/admin_back_go/database/schema/admin.hcl:1191](/E:/admin/admin_back_go/database/schema/admin.hcl#L1191)。预算等式、终态形状、citation、selected/excluded 和附件不内联 snapshot 的 CHECK 都很完整。

这些表字段多，但它们是可解释性和重放证据，不是 CRUD 配置：

- 为什么选中某段；
- 为什么排除某段；
- 预算如何计算；
- 当时模型能力是什么；
- 当时索引 generation 是什么；
- 最终 prepared request 对应哪个 Plan hash。

删除这些字段会让支付争议、上下文错答和重试问题无法取证。

### 6.7 Memory

Memory 以 Conversation 为所有者是正确的。`from_message_id <= through_message_id` 只保证数字顺序，不能保证同一会话。应补组合 FK；previous memory 也应属于同一 Conversation 和 Profile。

Memory reconciler 先取一批 Conversation，再逐个加载 Profile、模型、上一条 Memory 和分页 Turn，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/memory_reconciler_repository.go:13](/E:/admin/admin_back_go/internal/module/ai/contextengine/memory_reconciler_repository.go#L13)。这是容量风险，不是当前正确性错误。

## 7. Provider、Agent、Model 与 Tool

### 7.1 Agent Model 关系应收敛

`ai_agents` 当前保存：

- `provider_id`；
- 字符串 `model_id`；
- `model_display_name`；
- `billing_multiplier_ppm`。

`ai_provider_models` 已经有稳定数字主键，并以 `(provider_id,model_id,model_kind)` 唯一，见 [E:/admin/admin_back_go/database/schema/admin.hcl:2330](/E:/admin/admin_back_go/database/schema/admin.hcl#L2330)。

Agent 创建/更新时，服务端会读取 Provider Model 并复制 `display_name`，见 [E:/admin/admin_back_go/internal/module/ai/agent/service.go:203](/E:/admin/admin_back_go/internal/module/ai/agent/service.go#L203) 和 [E:/admin/admin_back_go/internal/module/ai/agent/service.go:247](/E:/admin/admin_back_go/internal/module/ai/agent/service.go#L247)。这证明 `model_display_name` 是派生配置，不是 Agent 独立业务输入。

长期推荐：

```text
ai_agents.provider_model_id -> ai_provider_models.id
```

完成 dual-read 后，以下为删除候选：

- `ai_agents.provider_id`；
- `ai_agents.model_id`；
- `ai_agents.model_display_name`。

`billing_multiplier_ppm` 必须保留，它是 Agent 定价策略，不属于 Provider Model。

Run、Image Task 中同名字段是历史 snapshot，不能跟着删除。

### 7.2 Provider Model 缺少 Provider FK

`ai_provider_models.provider_id` 没有 FK。当前扫描孤儿为 0，但数据库不会阻止未来孤儿。应在坏数据扫描后补 `RESTRICT` FK。

### 7.3 Agent Tool 绑定

`ai_agent_tools.status` 当前支持禁用再启用，更新逻辑是“全部 disabled，再 FirstOrCreate 启用指定项”，见 [E:/admin/admin_back_go/internal/module/ai/tool/repository.go:213](/E:/admin/admin_back_go/internal/module/ai/tool/repository.go#L213)。

与 Context Binding 一样，需要明确：

- 若产品只关心当前绑定，纯关系表更简单；
- 若需要历史，必须记录 actor 和变更事件，单个 status 不是审计历史。

Tool Call 本身是 Run 历史事实，必须保留。Agent Tool Binding 是否保留状态，不影响 Tool Call 历史。

### 7.4 软删除唯一键

`ai_providers` 的唯一键是 `(engine_type,name,is_del)`，见 [E:/admin/admin_back_go/database/schema/admin.hcl:2515](/E:/admin/admin_back_go/database/schema/admin.hcl#L2515)。这会导致同名 Provider 第二次删除时与第一条已删除记录冲突。

这不是 AI 特例，所有包含 `is_del` 的唯一键都要扫描。MySQL 可用 nullable active key 解决：只有正常行生成 identity，删除行生成 NULL；唯一索引允许多个 NULL。不要继续把 `is_del` 直接拼进唯一键。

## 8. 附件与 COS

### 8.1 后端可信边界正确

服务端不信任浏览器声明。发送消息时会：

- 规范化 object key；
- 按模型能力检查图片/原生文件；
- 并发执行 COS HEAD；
- 用 HEAD 的 key、size、ETag、MIME、URL 覆盖客户端事实；
- 校验单个原生文件严格小于 50 MiB；
- 校验消息附件总量不超过 50 MiB。

实现见 [E:/admin/admin_back_go/internal/module/ai/message/service.go:403](/E:/admin/admin_back_go/internal/module/ai/message/service.go#L403)。这是正确安全边界，不能为了“减少代码”移回前端。

### 8.2 历史附件进入后续上下文

Context Plan Item 明确区分 `current_attachment` 和 `history_attachment`，见 [E:/admin/admin_back_go/database/schema/admin.hcl:1339](/E:/admin/admin_back_go/database/schema/admin.hcl#L1339)。因此“上一轮带附件、下一轮不重新上传”不等于抛弃附件；是否入选取决于上下文预算和计划证据。

### 8.3 前端生命周期需要取消令牌

前端风险不是校验不足，而是异步生命周期归属不明确。建议状态结构从可变数组对象收敛为：

```text
Map<attachmentId, { generation, status, file, objectKey, error }>
```

删除/清空/切换上下文只增加 generation 并取消 controller。旧 Promise 没有匹配 token 就不能提交结果。

### 8.4 COS 孤儿对象治理

“上传成功但消息未提交”是合法业务场景，不能假设前端能零失败清理。应有明确对象生命周期：

| 阶段 | 建议 |
| --- | --- |
| 已签发未上传 | Token 自然过期 |
| 已上传未被 Message 引用 | 标记临时前缀或上传 intent，短 TTL 清理 |
| 已被 Message 引用 | 由 Message/Document 事实持有，不自动清理 |
| Message 软删 | 按数据保留政策异步回收，不能同步物理删导致 Run 证据失效 |

## 9. AI 计费、钱包与 Dashboard

### 9.1 应保留的资金闭环

以下关系设计合理：

```text
Run
  -> Wallet Hold (one per Run)
  -> Usage Charge (one per Run)
  -> Usage Charge Item(s) (per Attempt/category/unit)
  -> Wallet Transaction (immutable ledger source)
```

`wallet_holds.run_id` 和 `ai_usage_charges.run_id` 已有唯一约束，见 [E:/admin/admin_back_go/database/schema/admin.hcl:3487](/E:/admin/admin_back_go/database/schema/admin.hcl#L3487) 与 [E:/admin/admin_back_go/database/schema/admin.hcl:3572](/E:/admin/admin_back_go/database/schema/admin.hcl#L3572)。

`wallet_transactions(source_type,source_id)` 的唯一键为资金动作提供业务幂等，见 [E:/admin/admin_back_go/database/schema/admin.hcl:7135](/E:/admin/admin_back_go/database/schema/admin.hcl#L7135)。这类“重复字段”是账本证据，不应删除。

### 9.2 Run/Charge 的重复字段不是冗余

| 字段 | 为什么保留 |
| --- | --- |
| Run `pricing_snapshot_json` | 记录受理时价格，不受未来改价影响 |
| Run provider/model/display name | 记录实际调用事实，不受 Agent 改绑影响 |
| Charge `held_units/actual_units` | 记录预占和最终金额，不从当前钱包反推 |
| Hold `held_units/captured_units/status` | 表达资金生命周期和重放状态 |
| Charge Item usage/quote | 支撑 Provider 账单对账和争议定位 |

### 9.3 Dashboard Fact 表必须保留

`ai_run_dashboard_facts` 是每个终态 Run 的不可变投影，`ai_run_dashboard_daily_facts` 是有界日聚合，结构见 [E:/admin/admin_back_go/database/schema/admin.hcl:7184](/E:/admin/admin_back_go/database/schema/admin.hcl#L7184)。

它们与 Run 字段重复是刻意的读模型，不是配置冗余。Dashboard 查询已明确从这些投影读取，避免扫描所有 Attempt、Charge Item 和 Tool Call。删除会把管理看板重新变成大范围在线聚合。

### 9.4 钱包可增加的约束

当前账本只 CHECK 非负。建议在历史扫描通过后增加：

- `direction` 封闭集合；
- credit：`after = before + amount`；
- debit：`after = before - amount`；
- Wallet 的 `held_units <= balance_units` 是否成立要按当前余额定义确认，不能直接猜。

## 10. 支付与支付宝回调

### 10.1 支付订单快照字段应保留

`payment_orders.config_code/provider/pay_method/subject/return_url` 看似能从配置或请求推导，但它们是下单时事实：

- 配置可改名或禁用；
- 支付方式影响网关行为；
- subject 是第三方交易描述；
- return URL 属于单次请求，不属于全局配置。

因此这些字段不能按重复删除。

### 10.2 回调事件应成为幂等入口

推荐状态流：

```text
接收原始表单
-> 规范化并计算 callback dedupe key
-> INSERT callback event（唯一键抢占）
-> 事务外验签/读取必要配置
-> 事务内锁 callback event + order
-> 校验 app_id / amount / trade_no
-> 订单 + 充值 + 钱包幂等结算
-> callback event 写终态
-> 返回 success/fail
```

同一 callback 的重放应读取既有终态，不创建无限重复审计行。

### 10.3 支付索引候选

`payment_configs.idx_payment_configs_provider_status(provider,status,is_del)` 是 `idx_payment_configs_provider_status_sort(provider,status,is_del,sort,id)` 的左前缀，见 [E:/admin/admin_back_go/database/schema/admin.hcl:5248](/E:/admin/admin_back_go/database/schema/admin.hcl#L5248)。它是高置信冗余候选，但仍需用生产 `performance_schema` 确认没有特殊覆盖查询后再删除。

`payment_callback_events` 修复后建议：

- 唯一 dedupe key；
- `(provider,out_trade_no,received_at,id)` 用于订单审计；
- `(process_status,received_at,id)` 用于补偿任务。

是否扩展现有索引要按实际分页 SQL 决定。

## 11. WebSocket 与 Realtime

### 11.1 正面设计

`realtime_events` 的 `(target_type,target_id,sequence)` 与 `(expires_at,sequence)` 索引分别服务 resume 和清理，结构合理，见 [E:/admin/admin_back_go/database/schema/admin.hcl:5718](/E:/admin/admin_back_go/database/schema/admin.hcl#L5718)。Retention Watermark 能区分“从未存在”和“已被清理”，也正确。

### 11.2 Cursor 的业务含义必须固定

Cursor 不是“浏览器收到的最后事件”，而是“所有权威业务 handler 已成功应用的最后 durable event”。修复 RT-001 后，应增加以下契约测试：

| 场景 | 预期 |
| --- | --- |
| handler 成功 | cursor 前进 |
| 任一权威 handler 失败 | cursor 不前进，连接进入恢复 |
| 重复 sequence | 幂等忽略 |
| sequence gap | 权威恢复，不拼接局部状态 |
| ephemeral observer 失败 | 可记录错误，但不影响 durable cursor |

### 11.3 多标签页连接互相抢占

Session key 是 `platform:user_id:session_id`，见 [E:/admin/admin_back_go/internal/module/realtime/service.go:63](/E:/admin/admin_back_go/internal/module/realtime/service.go#L63)。同一登录 Session 的两个标签页通常得到相同 key。

Manager 注册新连接时会关闭旧连接，见 [E:/admin/admin_back_go/internal/infra/realtime/manager.go:41](/E:/admin/admin_back_go/internal/infra/realtime/manager.go#L41)。这可能导致两个标签页互相重连和抢占。

这里需要先固定产品语义：

| 语义 | 实现 |
| --- | --- |
| 每个标签页独立接收 | key 增加前端生成的 connection instance id；SendToUser 广播全部实例 |
| 每个认证 Session 只允许一条连接 | 前端显式协调 leader tab，其他 tab 不重连，并向用户明确状态 |

当前是“隐式单连接”，行为不够明确，列为 P2。

### 11.4 多节点边界

本地 Manager 只管理当前进程连接，代码也明确把多节点 fan-out 放到 Publisher。部署多 API 实例时必须验证 Redis/消息总线 fan-out 和 durable MySQL event 的顺序关系。不能因为单机 WebSocket 正常就认为多节点正确。

## 12. Vue 前端状态与刷新恢复

### 12.1 单一事实源方向正确

会话消息缓存在内存，刷新后从后端权威消息恢复。`use-chat-page` 在切换会话时强制加载后端消息，再推进已读游标，见 [E:/admin/admin_front_ts/src/views/Main/ai/chat/use-chat-page.ts:299](/E:/admin/admin_front_ts/src/views/Main/ai/chat/use-chat-page.ts#L299)。这个方向正确。

前端不应把整个聊天历史长期复制到 localStorage。只保存选中会话、Realtime cursor 等轻量状态；Message、Run、结算状态由后端恢复。

### 12.2 Session LRU 可超过上限

LRU 清理遇到最老的活跃会话会直接 `break`，见 [E:/admin/admin_front_ts/src/views/Main/ai/chat/composables/useConversationSessions.ts:86](/E:/admin/admin_front_ts/src/views/Main/ai/chat/composables/useConversationSessions.ts#L86)。后面即使有可淘汰会话也不会继续扫描，缓存可以长期超过 `MAX_SESSIONS`。

修复应遍历整个尾部寻找第一个可淘汰项；所有会话都活跃时允许暂时超限，并记录这是明确策略，而不是循环意外退出。

### 12.3 不要用默认值掩盖恢复错误

聊天核心字段如 `request_id`、Conversation ID、delivery state、settlement state 理论上存在时，前端不能用空字符串或 0 把后端契约错误吞掉。合法降级必须是业务规则，例如“没有 assistant message 表示请求仍在执行”，而不是任意 fallback。

## 13. 外键与组合一致性建议

### 13.1 当前扫描结果

在本次快照中，以下异常计数均为 0：

- Provider Model 的 Provider 孤儿；
- Agent 的 Provider 孤儿和 Model mapping 缺失；
- Conversation 的 User/Agent 孤儿；
- Command 与 Conversation User 不一致；
- Command 的 User Message 会话/角色不一致；
- Attempt 与 Context Plan 的 Run 不一致；
- Document source message 跨 Conversation；
- Memory from/through message 跨 Conversation；
- Image File 的 Task/related file 孤儿；
- 同一 Run 对应多个 Text Task 或 Image Task；
- 重复非空 callback notify id；
- 重复非空 Alipay trade no。

这说明当前数据可迁移，不代表约束不需要。样本干净正是补约束的最佳窗口。

### 13.2 建议关系矩阵

| 子表字段 | 父表字段 | 方式 | 前置条件 |
| --- | --- | --- | --- |
| `ai_provider_models.provider_id` | `ai_providers.id` | FK RESTRICT | 孤儿扫描 |
| `ai_agents.provider_model_id` | `ai_provider_models.id` | 新 FK | expand/backfill/dual-read |
| `ai_conversations.agent_id` | `ai_agents.id` | FK RESTRICT | 类型统一为 BIGINT UNSIGNED |
| `ai_reply_commands.run_id` | `ai_runs.id` | 新唯一 FK | 按 user_message backfill |
| Command conversation/message | Conversation/Message 组合键 | 组合 FK | signed/unsigned 统一 |
| `ai_provider_attempts.command_id` | `ai_reply_commands.id` | FK RESTRICT | nullable 历史扫描 |
| Attempt `(context_plan_id,run_id)` | Plan `(id,run_id)` | 组合 FK | 父表唯一组合键 |
| Document `(source_message_id,conversation_id)` | Message `(id,conversation_id)` | 组合 FK | 仅 Conversation 文档 |
| Memory from/through + conversation | Message id + conversation | 两个组合 FK | 历史扫描 |
| Memory previous + conversation/profile | Memory id + conversation/profile | 组合自 FK | 链扫描 |
| `ai_image_files.task_id` | `ai_image_tasks.id` | FK RESTRICT/CASCADE 按保留策略 | 孤儿扫描 |
| `ai_image_files.related_file_id` | `ai_image_files.id` | 自 FK SET NULL/RESTRICT | 明确修订语义 |
| Text/Image Task `run_id` | Run id | UNIQUE FK | 当前重复组为 0 |

组合 FK 的目标是让非法状态无法写入，而不是在每个 Service 方法再堆五个 `if`。

## 14. 字段与表的保留/删除候选

### 14.1 高置信保留

| 表/字段 | 判定 | 原因 |
| --- | --- | --- |
| Run 模型、Provider、价格、输入快照 | 保留 | 历史执行事实 |
| Attempt prepared/quote/usage/result/hash | 保留 | 外部副作用和重试证据 |
| Context Plan/Item 全部预算、hash、decision 字段 | 保留 | 上下文可解释性和重放 |
| Document Version 来源、parser/chunker/hash/lease | 保留 | 不可变版本与恢复 |
| Message `meta_json` | 保留 | 附件、runtime、block、feedback 的封闭扩展事实 |
| Payment Order 配置和请求快照 | 保留 | 支付历史不依赖当前配置 |
| Callback raw payload | 保留但加保留期/访问控制 | 支付争议和验签证据 |
| Wallet Hold/Charge/Transaction | 保留 | 资金账本 |
| Dashboard Fact/Daily Fact | 保留 | 有界读模型 |
| `ai_reply_delivery_chunks` | 保留 | stopped partial delivery 的持久证据，即使当前 0 行 |
| `ai_official_model_price_overrides*` | 保留 | 官方模型价格覆盖能力，0 行不等于无设计用途 |
| `ai_assets` / `ai_prompts` | 保留 | 架构文档明确为通用能力和后续平台基础，不能只看当前 0 行删除 |

### 14.2 高置信迁移候选

| 当前字段 | 目标 | 最终候选 |
| --- | --- | --- |
| Agent `provider_id + model_id + model_display_name` | `provider_model_id` | dual-read 后删除三个旧字段 |
| Context Binding 的伪历史字段 | 纯关系表或单独事件表 | 视产品决定删除 `id/status/timestamps` |
| Agent Tool Binding 的伪历史字段 | 纯关系表或单独事件表 | 视产品决定删除 `status/timestamps` |
| Callback 空字符串交易身份 | nullable/稳定 dedupe key | contract 后删除旧空字符串语义 |

### 14.3 暂不删除

| 表/字段 | 原因 |
| --- | --- |
| `ai_billing_migration_metadata` | 是数据库演进账本，必须等迁移恢复协议明确结束 |
| `ai_text_tasks` / `ai_image_tasks` | 当前 0 行不代表无路由；仍有运行时代码和统一 Run 关系 |
| `ai_image_files.related_file_id` | 表达 mask/revision 来源，先补 FK，不要先删 |
| Provider health/sync 字段 | 运行可观测状态，不是 Provider 配置重复 |
| Profile embedding/rerank/memory 字段 | 属于上下文能力配置；本报告排除“当前无向量模型”评价 |

### 14.4 明确禁止的删除方式

- 看到当前 0 行就删表；
- 看到字段能 JOIN 出来就删历史 snapshot；
- 在同一 migration 中 add、backfill、drop；
- 用 `COALESCE(old,new,0)` 长期掩盖 backfill 失败；
- 为了加 FK 直接删除孤儿；
- 让新代码同时接受多个互相冲突的字段并静默择一。

## 15. 索引审计

### 15.1 当前合理索引

| 索引 | 评价 |
| --- | --- |
| Message `(conversation_id,is_del,id)` | 符合游标分页 |
| Message `(conversation_id,is_del,role,id)` | 符合按角色和未读统计 |
| Run `(conversation_id,created_at,id)` | 符合会话 Run 时间线 |
| Attempt unique `(run_id,attempt_no)` | 保证 Attempt 序号 |
| Plan unique `(run_id)` | 保证一个 Run 一个 Plan |
| Chunk unique `(version_id,ordinal)` | 保证不可变 Chunk 身份 |
| Realtime `(target_type,target_id,sequence)` | 符合 resume |
| Realtime `(expires_at,sequence)` | 符合 retention 清理 |
| Wallet Transaction unique source | 保证资金幂等 |

### 15.2 高置信冗余候选

| 候选 | 被覆盖索引 | 处理 |
| --- | --- | --- |
| `payment_configs.idx_payment_configs_provider_status` | `idx_payment_configs_provider_status_sort` | 生产 usage 确认后删除 |
| `user_wallets.idx_user_wallet_isdel` | `idx_user_wallet_updated` | 生产 usage 确认后删除 |

Wallet 两个索引定义见 [E:/admin/admin_back_go/database/schema/admin.hcl:6841](/E:/admin/admin_back_go/database/schema/admin.hcl#L6841)。

### 15.3 应升级为唯一的候选

| 当前普通索引 | 建议 |
| --- | --- |
| `ai_text_tasks(run_id)` | 若一 Run 一 Task 契约成立，改 UNIQUE |
| `ai_image_tasks(run_id)` | 若一 Run 一 Task 契约成立，改 UNIQUE |
| Callback `(provider,notify_id)` | 使用 nullable/稳定 key 后改 UNIQUE |
| `payment_orders.alipay_trade_no` | 空串迁为 NULL 后对非空事实唯一 |

### 15.4 不应立即增加的索引

- 给 `ai_reply_commands` 的巨大 OR 查询增加十几个字段的巨型索引；
- 给低基数 `status/is_del` 单独建索引；
- 因为当前 307 个 Chunk 查询很快，就认定未来无需调整；
- 给 JSON 每个潜在字段做 generated column，除非存在真实 WHERE/ORDER BY。

## 16. 容量与查询放大

### 16.1 Candidate 鉴权 N+1

候选先批量加载，但 Conversation Document Version 又逐条调用 authoritative 查询，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/candidate_repository.go:214](/E:/admin/admin_back_go/internal/module/ai/contextengine/candidate_repository.go#L214)。

建议把所有 conversation-owned version id 一次性批量鉴权，返回 Set，再过滤。

### 16.2 Profile Rebuild N+1

每个 Version 分别加载 authority、work 和 chunks，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/ingestion.go:604](/E:/admin/admin_back_go/internal/module/ai/contextengine/ingestion.go#L604)。文档数增长后查询数约为 `1 + 3N`。

建议批量加载 work/chunks，并按 version id 分组；authoritative 规则也批量化。保持每个 Version 的不可变校验，不要为了批量而降低正确性。

### 16.3 Memory Reconciler N+1

`ListMemoryBuildPayloads` 每个 Conversation 调一次 `memoryBuildPayload`，内部继续查 authority、模型限制、上一条 Memory 和消息页。应先度量每批查询数，再考虑批量 authority 和 parent memory。消息分页可以保留按 Conversation 有界读取，避免一次 JOIN 产生笛卡尔放大。

### 16.4 Chunk Upsert 逐条 SELECT/INSERT

每个 Chunk 都先 SELECT，再决定 INSERT，见 [E:/admin/admin_back_go/internal/module/ai/contextengine/ingestion.go:788](/E:/admin/admin_back_go/internal/module/ai/contextengine/ingestion.go#L788)。

建议一次读取该 Version 已有 ordinal，内存比较 immutable facts，然后批量 INSERT 缺失行。发生同 ordinal 不同 hash 时仍必须硬失败，不能 `ON DUPLICATE KEY UPDATE` 覆盖不可变事实。

### 16.5 当前数据规模限制

307 个 Chunk、28 个 Run 无法暴露：

- Buffer Pool 压力；
- OR claim 的扫描放大；
- Context rebuild 的网络往返；
- Dashboard 长时间范围筛选；
- Realtime 单用户 500+ 积压。

后续性能验收应使用可回滚测试数据或预生产副本，不能在当前正式本地数据上跑破坏性造数脚本。

## 17. 分阶段修复方案

### Wave 0：代码正确性，不改 Schema

1. 修复 AI-001，用 Conversation/Agent/Profile 真实关系判断冲突。
2. 修复 RT-001，durable handler 失败不推进 cursor。
3. 修复 RT-002，超过 resume 上限立即 `resync_required`。
4. 删除会话时写取消意图；claim 和 dispatch guard 检查 Conversation tombstone。
5. 附件上传增加 generation、AbortController 和迟到提交校验。
6. 增加窄范围单元/集成测试，不跑全仓长脚本。

### Wave 1：Schema Expand

1. 新增 `ai_agents.provider_model_id` nullable。
2. 统一需要建 FK 的 signed/unsigned 和 INT/BIGINT 类型，优先使用兼容 shadow column 或评估在线 ALTER。
3. 新增 `ai_reply_commands.run_id` nullable。
4. 新增 callback dedupe key、nullable trade no 迁移列。
5. 新增必要父表组合唯一键，为组合 FK 做准备。
6. 只新增，不删除旧字段，不改变 API。

### Wave 2：数据扫描与 Backfill

1. Provider/Model 关系 backfill，遇到多映射或无映射立即失败。
2. Command `run_id` 按唯一 User Message backfill。
3. Callback/Trade identity 规范化，检查空值和重复。
4. 扫描所有跨用户、跨 Conversation、跨 Run 关系。
5. 扫描 CHECK 候选的历史非法状态。
6. 输出可重复 SQL 和异常明细，禁止静默填 0。

### Wave 3：Dual-read / Verify

1. 写路径同时写新旧 Agent Model 关系。
2. 读路径以新关系为主，并对比旧关系；不一致报警并拒绝派发。
3. Command/Run 双关系逐请求核对。
4. Callback 新旧 dedupe 身份并行记录。
5. 运行一段真实业务窗口，异常计数必须持续为 0。

### Wave 4：Contract

1. 新列改 NOT NULL。
2. 增加 FK、组合 FK、CHECK、UNIQUE。
3. 切掉旧读写。
4. 最后删除 Agent 旧 Provider/Model 字段和确认无用的 Binding 字段。
5. 删除冗余索引前读取生产 index usage，并保留回滚 DDL。

## 18. 修复验收清单

### 18.1 正确性

| 场景 | 验收结果 |
| --- | --- |
| 修改无上下文数据的 Agent Profile | 200，不再 1054 |
| 修改已有 Binding/Document/Memory 的 Agent Profile | 明确 409，不是 500 |
| durable handler 人工抛错 | cursor 不前进，重连后重放/恢复 |
| 单用户积压 501 条 | 不静默停在 500；进入 authoritative resync |
| 删除正在生成的会话 | 不再新派发；资金按真实派发状态闭合；消息不复活 |
| 删除附件后上传完成 | 旧 Promise 不回写当前输入 |
| 切换 Agent 后旧附件完成 | 不污染新 Agent 能力状态 |
| 同一支付宝 notify 重放 | 不重复创建有效处理事实，不重复入账 |
| 同一 trade no 指向不同订单 | 数据库/事务拒绝 |

### 18.2 数据闭合

每个 Chat Run 应满足：

```text
1 Conversation
1 User Message
1 Reply Command
1 Run
0..1 Context Plan
1..N Provider Attempts
0..1 Assistant Message（按终态规则）
1 Usage Charge
1 Wallet Hold
0..N Usage Charge Items
```

所有 user、conversation、message、command、run、plan、attempt 必须属于同一请求身份。

### 18.3 性能

- Candidate 验证查询数不随 Conversation Document 数线性增长。
- Profile rebuild 查询数接近常数加批次数，而不是 `3N`。
- Chunk insert 使用批量读取/写入，仍保持 immutable conflict。
- Claim 使用真实数据 `EXPLAIN ANALYZE`，记录 rows examined/claimed。
- Realtime resume 使用目标用户索引，不扫全表。

## 19. 文档与维护性问题

仓库 `AGENTS.md` 要求先读 `E:/admin/LONG_TASK_PARALLEL_EXECUTION.md`，但该文件当前不存在。这属于文档漂移，不应在业务代码里加兜底，也不应凭空创建内容。应由项目维护者决定删除引用还是恢复权威文档。

旧 `database/legacy-migrations/20260510_ai_knowledge_rag.sql` 仍包含退役的 `ai_knowledge_*` 设计文本。当前 canonical Schema 已收敛为 Context Engineering 九张表。后续开发和故障排查必须以 `database/schema/admin.hcl`、forward migrations 和当前 runtime 为事实来源，不得从 legacy migration 恢复旧 RAG 表。

## 20. 最终中文总结

【数据结构】

总体合格，甚至部分设计很好。Plan、Attempt、Charge、Dashboard Fact 的字段多是因为它们保存不可变事实，不是简单 CRUD 冗余。真正错误的是 Agent Model 还在用字符串复合关系，以及 Command/Run/Message 等核心边没有被数据库表达。

【特殊情况】

当前最糟糕的特殊情况有三个：不存在的 Memory `agent_id`、Realtime handler 失败却推进 cursor、删除会话后后台仍能执行。这三类都不能靠加默认值或 catch 后忽略解决，必须修正数据归属和状态确认语义。

【复杂度】

业务复杂度是真实的，不应把状态机压扁。应消灭的是复杂 OR claim、N+1 和伪历史 Binding。Context Plan、Provider Attempt、Wallet Hold 不是过度设计。

【兼容性】

禁止直接删字段、改 API 或重建历史。Agent Model、Callback identity、组合 FK 的收敛全部走 `expand -> 数据扫描与 backfill -> dual-read / verify -> contract`。旧会话、旧 Run、旧支付订单必须继续可读。

【结论】

值得修，而且完全可以在现有架构上修好。第一批只处理一处 P0 和四条立即运行正确性链路（两条 Realtime、一条删除链路、一条附件链路）；第二批再补支付与数据关系；第三批才处理 N+1 和索引。不要因为 AI 表有 34 张就大规模删表，也不要因为当前数据少就宣布索引没问题。先把事实闭环做成数据库和协议都无法破坏的状态，再谈精简。
