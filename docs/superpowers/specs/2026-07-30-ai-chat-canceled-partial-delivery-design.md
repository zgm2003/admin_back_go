# AI 对话停止后的部分回复交付设计

**日期：** 2026-07-30

**状态：** 设计已确认，待书面规格复核

**涉及仓库：** `admin_back_go`、`admin_front_ts`

**规范关系：** 本文定向覆盖 `2026-07-24-ai-chat-consumer-pricing-wallet-design.md` 第 4.1 节、第 6.1 节停止交互和第 14.1 节第 8 条中“取消后正文不发布、等待结算后才显示已停止”的规则。完整候选仍然丢弃，完整上游 usage 仍是唯一计费事实；本文新增发布的是用户已经看到的服务端投递前缀。若 `docs/architecture.md` 中 `ai.response.delta.v1`、`ai.response.canceled.v1` 或“canceled 不产生助手消息”的描述与本文冲突，以本文为准，实施时同步更新架构文档。

## 1. 背景与最终结论

当前实现已经把前端投递 context 与上游 drain context 分开。用户点击停止后，Worker 不再向浏览器发送 delta，但会继续读取同一上游流，取得权威 usage 后再完成 Run、账单和钱包结算。这解决了“停止即丢 usage”的计费问题。

当前仍有两个交付缺陷：

1. 前端在停止请求后的约 8 秒排空期间仍保持 `isStreaming=true`，继续显示生成动画和停止按钮，视觉状态与用户动作不一致。
2. canceled command 明确禁止保存助手消息，因此用户停止前已经看到的 `1234` 刷新后消失；数据库只有完整 usage，没有用户实际看到的回复前缀。

本文确认以下语义：

```text
上游实际生成：123456789
用户点击停止时已看到：1234

聊天正文：1234，并标记“已停止生成”
上游完整候选：不发布到聊天，结算后清除
计费：只按上游最终权威 usage；若该 usage 覆盖 123456789，则按完整 usage 计费
usage 不完整：不估算、不按正文长度猜测，继续使用 unbilled 规则
```

用户交付状态与上游计费状态是两个独立状态机。点击停止后，用户界面立即进入已停止状态；后台排空、usage 获取和费用结算可以继续运行。

## 2. 目标与非目标

### 2.1 目标

1. 点击停止后立即冻结当前可见内容，关闭动画、隐藏停止按钮并拒绝迟到 delta。
2. 服务端可靠保存浏览器已经看到的准确前缀，不接受客户端提交助手正文。
3. 部分回复可通过消息列表刷新、WebSocket 重连和页面重新进入恢复。
4. 上游继续排空，完整 usage、价格快照和现有结算规则不变。
5. canceled command、Run、助手消息和 durable terminal event 保持可追溯的一一关联。
6. 流式持久化必须有明确写入上界、正确索引和终态清理，不按上游 token 逐条写 MySQL。

### 2.2 非目标

- 不直接从浏览器续连上游，也不把 provider request ID 或密钥暴露给前端。
- 不保存或发布取消后的完整候选。
- 不按可见正文长度计算费用，不在 usage 缺失时估算 token。
- 不在本期改变上游失败、timeout 或 outcome unknown 的部分正文策略；这些分支仍不发布助手正文。
- 不重构知识库、RAG、工具调用或通用实时基础设施。
- 不建立可回放全部 token 的长期审计日志。

## 3. 不变量与事实边界

系统必须同时保留四类事实，禁止相互代替：

| 事实 | 权威来源 | 用途 |
| --- | --- | --- |
| 已承诺投递分片 | 服务端在发布 WebSocket delta 前写入的临时分片 | 证明浏览器可见内容只能来自服务端输出 |
| 用户可见前缀 | 停止请求携带的最后连续投递序号，从服务端分片重建 | canceled 助手消息正文 |
| 上游完整候选 | provider attempt 的 `result_candidate_json` | 仅成功时发布；取消时丢弃 |
| 上游权威 usage | provider attempt 的完整 usage | 唯一计费事实 |

必须满足：

1. `canceled_message.content` 是服务端已持久化分片从 `1..stop_delivery_seq` 的严格拼接。
   拼接必须逐字节保留已投递 UTF-8 文本，不做 `TrimSpace`、Markdown 修复或自动补全。
2. 客户端不能提交、替换或补写助手正文。
3. 任何发送给浏览器的 v2 delta，必须先有对应的 MySQL 分片；数据库写入失败时不得发送该 delta。
4. 停止事务提交后，同一 command 不得再提交新的投递分片。
5. canceled 的聊天正文永远不从 `result_candidate_json` 反推。
6. `completion_tokens`、`total_tokens` 和实际扣费不从 canceled 消息正文推算。
7. 同一 command 最多有一条助手消息，继续由 `ai_messages.reply_command_id` 唯一约束保证。
8. canceled 助手消息可以为空；派发前或首个 delta 前停止时仍保存一条“已停止生成”的空助手消息，以保证配对、恢复和事件契约闭合。

## 4. 方案选择

### 4.1 方案 A：前端回传当前正文

实现最小，但客户端可以伪造、注入或篡改助手内容，服务端无法证明正文来自模型。拒绝。

### 4.2 方案 B：Worker 内存累积，取消时一次落库

正常路径写入少，但 API 与 Worker 是不同进程；Worker 崩溃、重启或租约切换会丢失用户已见内容，也无法由取消接口原子冻结。拒绝。

### 4.3 方案 C：服务端投递分片先持久化，再发布（采用）

Worker 对上游小 delta 做短时间合并，以一个 command 内单调序号写入临时分片表，提交成功后才发布 `delta.v2`。浏览器只回传最后连续序号，取消事务从服务端分片重建前缀并立即创建 stopped 助手消息。

该方案增加有界 MySQL 写入，但换取了准确性、跨进程一致性、崩溃恢复和不信任客户端正文。通过合并窗口、append-only 主键和终态删除控制成本。

## 5. 双状态机

### 5.1 用户交付状态

```text
streaming
  -> completed       正常完成，发布完整候选
  -> stopped         用户停止，发布已见前缀
  -> not_delivered   失败、超时或结果未知，不发布助手正文
```

`stopped` 在取消 HTTP 事务提交时成立，不等待上游 drain 或钱包结算。

### 5.2 后台运行与计费状态

```text
Run:     running -> success | canceled | failed | timeout | outcome_unknown
Billing: pending/held -> settled | released | unbilled
```

合法组合包括：

- `delivery=stopped + run=running + billing=held`：用户已经停止，后台仍在排空。
- `delivery=stopped + run=canceled + billing=settled`：usage 完整，按实际用量收费。
- `delivery=stopped + run=canceled + billing=unbilled`：usage 不完整或超过冻结上限，本次未扣费。
- `delivery=stopped + run=canceled + billing=released`：派发前停止，释放冻结。

前端不得用 `delivery=stopped` 推导“免费”，也不得用 `billing=settled` 把完整候选补回聊天。

## 6. 服务端数据流

### 6.1 正常流式投递

```text
provider delta
  -> delivery coalescer（短窗口合并）
  -> 锁定 command，验证 running + lease fencing + cancel_requested_at IS NULL
  -> command.delivery_seq 原子加一
  -> INSERT ai_reply_delivery_chunks(command_id, delivery_seq, delta)
  -> COMMIT
  -> 发布 ai.response.delta.v2(delivery_seq, delta)
  -> 浏览器按连续序号追加正文
```

投递合并器采用以下固定边界：

- 最长合并等待 `50ms`；
- 单个持久化分片最多 `16KiB` UTF-8 字节，超出时按合法 UTF-8 边界拆分；
- 停止信号、正常终态或 sink 错误到达时立即处理缓冲区；停止信号到达时，尚未持久化且从未发布的缓冲内容直接丢弃；
- 不按 token、字符或 provider 原始 event 一条一写。

`50ms` 是额外流式展示延迟上界，不是上游超时。实施性能测试若证明本地数据库无法满足该预算，只能基于执行计划调整批量阈值，不能退回客户端正文或先发后写。

### 6.2 用户点击停止

前端在发 HTTP 前同步完成：

1. 读取并冻结当前 `streamingContent` 与 `lastContinuousDeliverySeq`。
2. 将消息设为 `isStreaming=false` 和 `delivery_state=stopped`。
3. 清空 active streaming request，记录 canceled request ID，后续 delta 一律忽略。
4. 隐藏停止按钮和生成动画，并设置独立的 `stopCommitPending`；后台结算不占用流式 UI。

然后请求：

```http
POST /api/admin/v1/ai-conversations/:id/messages/cancel
```

```json
{
  "request_id": "...",
  "delivered_seq": 4
}
```

`delivered_seq=0` 表示停止时没有看到任何助手分片。

服务端在一个短事务内按固定顺序执行：

1. 锁定目标 `ai_reply_commands`，校验当前用户、会话和 request identity。
2. 锁定会话；command 锁同时阻止 delivery sink 提交新分片。
3. 读取该 command 的 `1..delivered_seq` 分片并验证序号连续；`delivered_seq` 不得大于 command 已提交的 `delivery_seq`。
4. 拼接服务端分片，创建 `delivery_state=stopped` 的助手消息；正文允许为空。
5. 设置 `cancel_requested_at`、`stop_delivery_seq` 和 `assistant_message_id`，更新会话最后消息时间。
6. 提交后 best-effort 发布现有 Redis cancel signal，让 Worker 尽快关闭 delivery context；Redis 发布失败不撤销已持久化停止意图。
7. 提交后按 command ID 触发有界分片清理；清理失败交给 reconciler，不拉长停止事务。

取消意图优先于正文恢复。如果序号越界、缺口或数据库事实冲突，事务仍须持久化取消，但不得信任客户端猜测正文：服务端把权威 `stop_delivery_seq` 降为 `0`，创建空 stopped 消息并记录结构化一致性异常。这样最多损失无法证明的可见前缀，不会继续发布完整答案，也不会注入伪造正文。只有所有权、request identity 或数据库事务本身失败时取消请求才失败。

### 6.3 后台 drain 与结算

取消事务不触碰 Run、Charge、wallet、Hold 或 attempt，避免反转既有计费锁序。Worker 继续：

1. 维持租约并读取同一上游流到终态。
2. 保存 provider request ID、完整 usage 和 attempt 终态。
3. 对 `TriggerUserStop` 继续使用 `SettlementCandidateDiscard`。
4. 外层结算事务按 `Run -> Charge -> wallet -> Hold -> command` 锁序执行。
5. canceled command participant 验证已经存在的 stopped 助手消息，保留正文并把同一 `assistant_message_id` 绑定到 Run。
6. 清除完整候选，提交 canceled Run 与账单终态，并追加 durable canceled v2 event。
7. 结算事务提交后按 command ID 触发分片清理；不得在持有 Run、Charge、wallet 和 Hold 锁时批量删除分片。

finalizer 不修改 stopped 消息的正文、创建时间或会话 `last_message_at`，避免后台结算晚到后把旧回复移动到新消息之后。若 finalizer 暂时失败，stopped 消息仍可通过消息列表恢复；command 保持可重试的结算状态，不能再次调用上游。

后台 drain 仍受现有代码级上游总时长、静默超时、租约和 stale run guardrail 约束，不允许无限运行。guardrail 到达且拿不到完整终态 usage 时继续 fail-closed：不猜 usage、不盲目重发，并按现有 unknown/unbilled 或 release 规则收尾。

## 7. 数据模型与迁移

### 7.1 `ai_reply_delivery_chunks`

新增仅用于活动流式回复的临时表：

| 字段 | 类型 | 约束与语义 |
| --- | --- | --- |
| `command_id` | `BIGINT UNSIGNED` | reply command ID |
| `delivery_seq` | `INT UNSIGNED` | 从 1 开始、command 内连续递增 |
| `delta` | `TEXT` | 最大 16KiB UTF-8 字节 |
| `created_at` | `DATETIME(6)` | 服务端提交时间 |

索引和约束：

```text
PRIMARY KEY (command_id, delivery_seq)
FOREIGN KEY (command_id) REFERENCES ai_reply_commands(id) ON DELETE RESTRICT
CHECK (delivery_seq > 0 AND OCTET_LENGTH(delta) > 0 AND OCTET_LENGTH(delta) <= 16384)
```

不增加单列 `delivery_seq`、时间或正文索引。所有热路径都以 command ID 点查或主键范围扫描，额外索引只会增加流式写放大。

### 7.2 `ai_reply_commands`

新增：

```text
delivery_seq INT UNSIGNED NOT NULL DEFAULT 0
stop_delivery_seq INT UNSIGNED NULL
```

规则：

- `delivery_seq` 是服务端已提交分片的最后序号；分片事务在 command 行锁内递增它，避免每次执行 `MAX()`。
- `stop_delivery_seq` 在从未请求停止时为 `NULL`。
- 首次停止写入非负值，重复取消不得改大或改小。
- `cancel_requested_at IS NOT NULL` 时必须非空。
- `stop_delivery_seq <= delivery_seq`。
- 新行为下 canceled command 必须有 `assistant_message_id`；历史 canceled 数据不反向伪造，迁移验证允许其保持 `NULL`。

停止事务提交后、后台结算完成前允许以下暂态，这是双状态机分离后的合法事实：

```text
command.state = running
command.cancel_requested_at IS NOT NULL
command.assistant_message_id = stopped message ID
run.status = running
run.assistant_message_id IS NULL
```

外层 finalizer 最终把 command/Run 都绑定到同一 message ID。现有“活动 command 绝不允许 assistant_message_id”的代码级假设必须同步删除。

### 7.3 `ai_messages`

新增：

```text
delivery_state VARCHAR(16) NULL
```

取值：

- 助手正常完成：`completed`；
- 用户停止的部分或空回复：`stopped`；
- 用户和 system 消息：`NULL`。

历史可见助手消息回填 `completed`。新增检查约束保证非助手消息不能携带 delivery state，助手消息只能是 `completed|stopped`。不把该状态塞进 `meta_json`，因为它是需要约束、查询和稳定展示的一等业务事实。

### 7.4 临时分片生命周期

- 正常成功：完整助手消息与终态提交后，对应分片进入可清理状态。
- 用户停止：停止事务创建 stopped 消息后，对应分片进入可清理状态。
- failed、timeout、outcome unknown：终态提交后进入可清理状态，不发布部分正文。
- 进程崩溃：分片保留，command 恢复或终态 reconciler 按 command 主键有界清理。
- 正常路径在业务事务提交后按已知 command ID 循环小批量删除；reconciler 从分片表复合主键顺序取得少量 distinct command ID，再检查 command 终态并清理。
- 清理必须按主键小批量进行，不在资金事务中删除，不做无索引正文扫描，不建立独立通用日志系统。

## 8. API 与实时契约

### 8.1 消息接口

`AIMessageItem` 增加两个必返字段：

```json
{
  "delivery_state": "completed",
  "settlement_pending": false
}
```

允许值为 `completed`、`stopped`、`null`。前端只在 `stopped` 时显示紧邻助手正文的“已停止生成”弱状态文本，不使用错误红色，不继续显示打字动画。

`settlement_pending` 由关联 reply command 是否仍为 `pending|claimed|running` 计算，不由前端根据消息 ID 猜测。它用于刷新后继续禁用 stopped 消息的编辑、删除、重新生成和反馈，不显示成流式动画，也不代表最终是否扣费。

取消请求增加必填非负整数 `delivered_seq`。客户端不提交正文、hash、Run ID、message ID 或 provider 信息。

取消响应替换旧的模糊 `status=stopping`：

```json
{
  "conversation_id": 3,
  "request_id": "...",
  "status": "stopped",
  "assistant_message_id": 97,
  "settlement_pending": true
}
```

规则：

- 首次或重复停止返回 `status=stopped` 和同一助手消息 ID。
- `settlement_pending=true` 只表示 Run/账单尚未终态，不改变 stopped UI。
- 如果取消与已提交成功/失败终态竞态，返回 `status=already_terminal`、权威可空助手消息 ID 和 `settlement_pending=false`；前端立即恢复消息列表，不把成功消息改成 stopped。

### 8.2 `ai.response.delta.v2`

用 v2 一次性替换 v1：

```json
{
  "conversation_id": 3,
  "request_id": "...",
  "delivery_seq": 4,
  "delta": "4"
}
```

前端只接受 `last_seq + 1`：

- 小于或等于 last seq 的重复事件忽略；
- 大于 `last_seq + 1` 表示流式缺口，停止追加并进入服务端恢复，不能跳号拼接；
- stopped request 的任何 delta 永久忽略。

### 8.3 `ai.response.canceled.v2`

用 v2 一次性替换 v1：

```json
{
  "conversation_id": 3,
  "request_id": "...",
  "assistant_message_id": 97
}
```

该 durable event 仍与 command、Run 和账单终态在同一 MySQL 事务提交。它表示后台结算完成，不负责第一次切换 stopped UI。前端收到后按 request ID 清除 settlement pending 状态，刷新消息和运行事实；不得用事件重写本地冻结正文。

`start.v1`、`completed.v1` 和 `failed.v1` 保持不变。本项目采用前后端、OpenAPI 和实时契约一次性切换，不长期双发 v1/v2。

## 9. 前端状态与交互

### 9.1 点击停止的同步状态变更

`beginStopping` 改为用户交付终态，而不是保留旧 pending stream：

```text
isStreaming = false
sending = false
pendingRequestId = ''
streamingContent = ''（正文已冻结到消息对象）
deliveryState = stopped
canceledRequestIds += requestId
stopCommitPendingRequestId = requestId
```

停止按钮和动画立即消失，但 composer 在很短的停止落库请求期间保持不可发送，防止下一条用户消息先于 stopped 助手消息入库。HTTP `stopped` 或权威消息恢复确认后，清除 `stopCommitPendingRequestId`，再按响应设置 `settlementPendingRequestIds`。后台 settlement pending 不显示打字动画或红色停止按钮。

停止落库确认后，用户无需等待上游 drain 和费用结算即可发送下一条普通消息；新 command 使用新 request ID，并把已落库 stopped 消息作为历史上下文。对仍在后台结算的 stopped 消息，编辑、删除、重新生成和反馈继续禁用，直到 durable canceled v2 或权威恢复确认 command 终态，避免命中现有活动 command 冲突。复制和浏览器朗读仅在正文非空时可用。

### 9.2 HTTP 与重连恢复

- HTTP `stopped` 响应用权威 assistant message ID 替换本地负 ID，并保留冻结正文。
- HTTP `already_terminal` 立即从消息接口恢复，显示服务端成功/失败终态。
- HTTP 结果不明确时，用相同 request ID 和相同 delivered seq 幂等重试；随后恢复消息列表。在权威结果明确前保持 `stopCommitPending`，不能先发送下一条造成消息顺序倒置。
- WebSocket 重连只连接本系统实时服务。durable canceled v2 可以重放；流式 delta 不重放。
- 消息列表是 stopped 正文恢复真相源，运行列表/详情是 usage 和费用恢复真相源。
- 页面刷新后，`delivery_state=stopped` 的消息仍显示停止标记，不恢复为 streaming。
- A 已停止后允许立即发送 B。A 的 canceled v2 到达时必须按 request ID 合并 A 的权威消息和 settlement 状态；若 B 正在 streaming，不得调用会清空整个 session 流状态的全量 `recoverMessages`。只有当前活动 request 自身终态或全局 resync 才能结束该 request 的本地流。
- stopped request 永久忽略后续 start/delta，但不能无条件忽略 completed/failed。若取消未提交或成功终态先赢得竞态，completed/failed 是服务端权威结果；前端按 request ID 恢复该消息，同时仍不得影响另一个正在 streaming 的 request。

### 9.3 会话、未读和反馈

- stopped 助手消息参与问答配对、历史上下文、软删除和重新生成源定位。
- stopped 消息不计入未读数；用户主动停止不能制造新的未读提醒。
- stopped 消息不允许点赞，避免把不完整输出混入正式回复质量统计。
- Run 详情必须能通过 `assistant_message_id` 查看该 stopped 消息，同时继续展示完整权威 usage 和实际费用。

## 10. 竞态、错误与幂等

### 10.1 停止与新 delta

delivery sink 和取消事务都先锁 command。谁先取得锁决定边界：

- 分片先提交并发布，浏览器若已处理则可在 delivered seq 中包含它。
- 取消先提交，后续分片因 `cancel_requested_at IS NOT NULL` 被拒绝且不得发布。
- 分片已提交但浏览器尚未处理时，浏览器回传较小 seq；停止正文只取该较小前缀。

### 10.2 停止与成功终态

- 成功事务先提交：取消返回 `already_terminal`，不得把 completed 消息改成 stopped。
- 停止事务先提交：成功候选不得发布，finalizer 必须走 canceled 并保留 stopped 消息。
- 两条路径都依赖 command 行锁和 `cancel_requested_at` 条件，不依赖内存时序判断。

### 10.3 重复请求

- 相同 request ID 的重复停止返回首次 `stop_delivery_seq` 和同一 message ID。
- 重复请求携带不同 seq 时不改变已冻结正文。
- canceled finalizer 重试只补齐 Run/Charge/Hold/事件终态，不重复创建消息或资金流水。
- delta 分片主键冲突时，只有相同 command、seq 和相同正文才可视为幂等重放；不同正文是内部一致性错误。

### 10.4 错误处理

- 分片落库失败：不发布 delta，当前 attempt 按现有本地失败/unknown 规则收尾。
- cancel signal 发布失败：停止意图和消息已经持久化，Worker 通过续租检查最终发现取消。
- canceled 消息创建失败：整个停止事务回滚，不留下半条 command/message 关系。
- finalizer 暂时失败：消息保持 stopped，账单保持开放并由 finalization retry 恢复；不得二次派发上游。
- 日志只记录 command ID、request ID、seq、字节数和错误码，不记录 delta 或完整正文。

## 11. 性能与索引预算

1. 热路径每个合并分片执行一次短事务：command 主键点查/锁定 + 单行主键 INSERT。
2. 不更新不断增长的 LONGTEXT，避免每次 delta 重写全部前缀。
3. 不给临时表增加正文、时间或低选择性二级索引。
4. cancel 只扫描单个 command 的主键连续范围，并在 Go 中有界拼接，不使用受 `group_concat_max_len` 影响的 SQL 拼接。
5. 业务事务提交后按 command 主键小批量删除；禁止在资金锁内清理，也禁止一条 delta 发一条 DELETE。
6. 自动测试固定验证“先提交后发布”和“没有按 provider 原始 event 逐条写”的结构边界。
7. 使用代表性长回复执行 `EXPLAIN ANALYZE`，确认 cancel 范围读取命中 PRIMARY key；以 10 分钟持续流验证分片数量、InnoDB 写入和 purge 行为。
8. 目标是本地数据库健康时，投递持久化带来的额外展示延迟 P95 小于 `75ms`；该目标记录为性能证据，不写成易受 CI 抖动影响的硬超时单测。

## 12. 安全与隐私

- 取消接口继续校验当前登录用户对会话、command 和 request ID 的所有权。
- `delivered_seq` 只选择服务端已存前缀，不能注入客户端文字。
- 临时分片包含回复正文，沿用消息正文的数据访问边界，不进入操作日志、错误响应或监控标签。
- 分片只服务活动执行，终态后立即触发有界清理，失败时由 reconciler 补偿；不作为长期分析或全文检索来源。
- provider 完整候选在取消结算后继续清除，不因部分消息功能延长保留。

## 13. 迁移与发布

这是一次性内部契约升级：

1. 更新 `database/schema/admin.hcl`、Atlas migration、reconciliation 和 schema invariant tests。
2. 迁移前置守卫要求不存在 `pending|claimed|running` reply command 或与其关联的未终态 provider attempt，避免旧 Worker 与 v2 Worker 混写同一 stream。
3. 新建投递分片表，新增 command/message 字段及检查约束。
4. 历史可见助手消息回填 `delivery_state=completed`；已有 `cancel_requested_at` 的 command 把 `stop_delivery_seq` 回填为 `0`。历史 canceled command 无法恢复正文，保持无助手消息。
5. 同步更新后端实时 registry、OpenAPI、`docs/architecture.md` 和生成契约。
6. 前端同步切换 delta/canceled v2、取消请求、消息 delivery state 和新本地状态机。
7. 迁移先删除或过期 `realtime_events` 中残留的 `ai.response.canceled.v1` 传输记录；Run、command 和账单事实不删除。delta v1 本来不持久化。
8. 删除 delta/canceled v1 的前后端定义、订阅、测试和死代码，不保留双发适配器。
9. 发布期间前后端必须作为一个版本切换；数据库迁移先于新 Worker/API 启动。

用户已计划从空数据重新测试全链路，但迁移仍必须对已有历史数据 fail-closed，不能假设生产库为空。

## 14. 测试与验收

### 14.1 后端

- 分片必须先提交再发布，数据库失败时零 delta。
- 合并窗口、16KiB 拆分、UTF-8 边界和连续 seq。
- cancel seq 为 0、首分片、中间分片和最后分片。
- 停止正文严格等于 `1..delivered_seq`，不包含已提交但客户端未确认的尾部。
- 前端无法通过请求体注入助手正文。
- 停止/分片、停止/成功和重复停止竞态。
- 派发前停止生成空 stopped 助手消息。
- canceled finalizer 接受 stopped 消息并将同一 ID 绑定 Run。
- 完整 usage 下 `run=canceled + billing=settled`；usage 缺失时 `unbilled` 且不估算。
- 完整候选不发布并在终态清除。
- stopped 消息不计未读、不允许反馈，终态后可重新生成和软删除。
- 所有终态都会触发分片清理，崩溃残留可由 reconciler 按主键有界补偿。
- v2 事件严格 payload、durability 和 OpenAPI 契约。

### 14.2 前端

- 点击停止的同一 tick 内关闭 streaming/动画/停止按钮并冻结正文。
- 停止后迟到、重复和跳号 delta 的处理。
- cancel 请求携带最后连续 seq，不携带正文。
- stopped ack 绑定真实 message ID；already terminal 恢复权威列表。
- canceled v2 只结束 settlement pending，不覆盖冻结正文。
- 刷新和 WebSocket resync 后 stopped 正文与标记保持一致。
- 空 stopped 消息、复制、朗读、反馈、重新生成和删除的可用性。
- OpenAPI 生成类型、Vitest、typecheck 和正式构建通过。

不使用 Playwright。

### 14.3 关键集成验收

Fake provider 输出 `123456789`，浏览器处理到序号对应的 `1234` 后停止，同时 provider 最终上报覆盖完整输出的 usage。最终必须同时满足：

```text
ai_messages.content = "1234"
ai_messages.delivery_state = "stopped"
ai_reply_commands.state = "canceled"
ai_reply_commands.assistant_message_id = ai_messages.id
ai_runs.status = "canceled"
ai_runs.assistant_message_id = ai_messages.id
ai_runs.billing_status = "settled"
completion_tokens / actual_units = 上游完整权威 usage 的结算结果
result_candidate_json = NULL
durable event = ai.response.canceled.v2 + 同一 assistant_message_id
```

## 15. 验收清单

- [ ] 点击停止后前端立即显示已停止，不等待后台 drain。
- [ ] 用户已见前缀刷新后不丢失，且只能来自服务端分片。
- [ ] 取消后的完整候选不会进入聊天正文。
- [ ] 费用只按完整权威 usage；不完整 usage 不估算扣费。
- [ ] canceled message、command、Run 和 durable event 使用同一 assistant message ID。
- [ ] delta/canceled v2 契约前后端一次性同步，v1 死代码删除。
- [ ] 临时分片只使用主键热路径，并在所有终态清理。
- [ ] 停止、成功、重试、重连和进程恢复竞态有自动测试。
- [ ] 后端测试、前端 Vitest、typecheck 和正式构建通过。
