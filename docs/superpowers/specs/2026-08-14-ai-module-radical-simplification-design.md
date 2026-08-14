# AI 模块激进减法设计

> 日期：2026-08-14
>
> 状态：核心产品决策已确认，等待用户审阅完整设计；尚未编写实施计划，尚未修改运行时代码或数据库
>
> 适用仓库：`E:\admin\admin_back_go`、`E:\admin\admin_front_ts`
>
> 文档地位：Wave 06 AI 重构的专项设计。用户确认本文后，本文替代中心方向书中关于上下文工程、Qdrant、Embedding、Rerank、Memory、Context Plan 和预冻结的旧方向；实施前必须同步中心方向书与执行总索引。

## 1. 需求判断

【需求判断】

这是真问题。当前普通 AI 对话被两套与核心体验无关的复杂机制绑住：

1. 上下文工程把简单的多轮聊天扩展成 Profile、Space、Document、Version、Chunk、Embedding、Rerank、Memory、Qdrant、Context Plan 和 Citation 的完整 RAG 平台。
2. AI 扣费在调用前按模型最大上下文、最大输出和附件上限冻结资金，而不是按本次真实用量收费。

它们已经产生实际伤害：配置难理解、依赖和表数量增加、普通聊天受 Qdrant 与向量模型影响、极高理论冻结额阻止有余额用户调用模型。

【核心问题】

系统真正需要的是：稳定的多轮对话、原生附件、可靠终态、真实用量结算和可追溯钱包流水。它不需要在当前阶段同时成为知识库平台和授信风控平台。

【复杂度检查】

当前复杂度远高于真实产品需求。继续给上下文工程增加“可选开关”“降级路径”和“热插拔”只会保留两套运行时。正确方向是完整删除，而不是再包一层兼容接口。

【破坏性分析】

本次会主动删除上下文工程数据、配置和派生索引，也会改变 AI 扣费状态机。项目尚未上线、处于纯本地开发，用户已明确接受这项破坏性减法。聊天、附件、Run、Usage、钱包流水、充值支付等核心事实必须保留。

【结论】

值得做。采用一次明确的离线数据库迁移和单运行时切换，不保留 RAG 双轨，不建设兼容适配层。

## 2. 已确认的产品决策

以下决策已经确认，不在实施阶段重新发明：

- 完整退役上下文工程，不保留隐藏开关或备用运行时。
- 删除 Qdrant、Embedding、Rerank、Memory、Context Profile、Context Space、Context Plan 和结构化 Citation。
- 普通聊天恢复为“最近 N 个完整对话轮次”。
- 历史轮次中的原生图片和文件附件必须继续随上下文发送，不能因为当前消息没有重新上传附件就丢失历史附件。
- 保留 COS、附件授权、WebSocket、工具调用、运行监控、真实 Usage、钱包流水、充值和支付宝支付。
- 取消调用前高额预冻结，只在接受一个新 Run 时检查当前余额是否大于 0。
- 已接受的 Run 必须允许完成；不得因中途余额变化打断 Provider 重试或工具续轮。
- 按供应商返回的完整真实 Usage 结算。
- 本次结算允许把余额扣成负数。
- 余额小于或等于 0 时拒绝下一次新 Run；充值后只有余额重新大于 0 才恢复调用。
- 并发 Run 在余额大于 0 时可以同时被接受，最终可能使余额进一步为负；不再用预冻结伪装并发授信控制。
- Provider 不返回完整可验证 Usage 时不猜测、不按字符估算扣费，Run 标记为未计费并保留诊断事实。

## 3. 目标架构

AI 保持一个业务域，不再建设“平台里的平台”。目标结构表达真实职责：

```text
internal/module/ai/
├── provider/       供应商、模型和价格配置
├── agent/          智能体、系统提示词、模型和工具绑定
├── conversation/   会话事实
├── message/        消息、附件引用、编辑和反馈
├── chat/           对话编排和最近轮次上下文
├── run/            Run、Attempt、Usage、终态和监控
├── tool/           工具定义和工具调用记录
├── image/          生图任务
└── billing/        真实 Usage 定价和钱包扣款编排
```

基础设施只保留实际使用的边界：

```text
MySQL       会话、消息、Run、Attempt、Usage、价格快照、钱包和流水真相
COS         图片与文件内容
Redis       缓存、Token、队列、WebSocket 广播和 AI 取消信号
Provider    对话或生图上游
WebSocket   流式增量通知；刷新后的恢复真相仍来自 MySQL
```

明确不存在：

```text
Qdrant
Embedding provider call
Rerank provider call
Context Profile / Space / Plan
Conversation Memory summary
Document ingestion / vector indexing
structured Citation projection
wallet hold / reserve / capture / release
upper-bound quote proof
```

## 4. 普通聊天上下文

### 4.1 上下文组成

每次新 Run 的 Provider 请求只由以下事实组成：

```text
智能体 system_prompt
-> 最近 N 个可见完整轮次
-> 当前用户消息及当前附件
-> 本 Run 的工具定义、工具调用和工具结果
```

不再读取 Profile、Space、Document、Memory、Qdrant 或历史 Context Plan。

### 4.2 完整轮次定义

一个历史轮次是同一会话中配对的：

```text
user message
+ terminal visible assistant message
```

规则：

- Assistant `delivery_state=completed` 的轮次可进入历史。
- 用户停止后已持久化且可见的 `delivery_state=stopped` 回复也可进入历史。
- pending、running、failed、没有可见 Assistant 内容的 canceled 记录不进入历史。
- 按消息 ID/创建顺序确定先后，不从 WebSocket 内存恢复历史。
- 取最近 N 个完整轮次后，按时间正序发送给 Provider。
- `max_history` 是最大轮次数，不是消息条数。

### 4.3 参数规则

`max_history`：

- 默认值：20。
- 有效范围：0 到 50，包含 0 和 50。
- 0 表示不发送历史轮次，但仍发送系统提示词、当前消息、当前附件和本 Run 工具结果。
- 保存到当前用户消息的运行参数快照，保证刷新、重试和运行详情能解释本次请求。

`max_tokens`：

- 恢复为用户可设置参数。
- 默认值：4096。
- 最小值：1。
- 最大值：当前启用官方模型声明的最大输出 Token。
- 默认值高于模型上限时使用模型上限；显式输入高于模型上限时返回明确 400，不静默改值。
- 不再强制使用官方模型最大输出作为每次请求的输出量。

### 4.4 历史附件

附件属于它所在的用户消息，不属于 Context Plan。只要该用户消息所在轮次进入最近 N 轮，它的原生附件就必须进入 Provider 请求。

```text
第一轮：用户上传 PDF + 提问
第二轮：用户问“我刚刚上传的文件叫什么？”

max_history >= 1
-> 第二轮请求必须重新从持久化附件清单取得第一轮 PDF
-> 重新校验用户、会话、对象 key、ETag、大小和 MIME
-> 通过 COS 受控读取后发送给 Provider
```

规则：

- 当前附件与历史附件共用现有不可变 manifest、COS 授权和条件读取链。
- 不保存文件二进制、Base64、临时凭证或临时签名 URL。
- 同一个附件在同一次请求中只物化一次。
- 保留当前原生附件聚合大小限制；超过限制明确失败，不静默删除旧附件。
- Provider 不支持某种文件或图片能力时返回明确能力错误，不伪装成普通文本成功。
- 删除 Context Document 后，聊天附件仍然存在；两者不是同一个业务事实。

### 4.5 超长上下文

`max_history` 表示最多携带 N 轮，不承诺 Provider 一定接受所有字节。第一版不重新建设 Token Planner：

- 当前消息和当前附件永不静默丢弃。
- 历史只按完整轮次选择，不拆散 user/assistant，不单独删除轮次中的附件。
- 本地可证明超过已有硬限制时返回明确错误。
- 上游返回 context length 错误时保留真实上游错误分类和 Run 终态，不用空回复或截断掩盖。

如未来真实使用证明需要自动裁剪，只允许增加“从最旧完整轮次开始删除”的单一规则，并单独写规格；不得恢复 Context Plan。

## 5. Provider 与智能体减法

### 5.1 模型类型

`model_kind` 只保留：

```text
chat
image
```

- `chat` 服务普通对话、文本生成和工具调用。
- `image` 服务生图任务，例如 `gpt-image-2`。
- 删除 `embedding` 和 `rerank` 类型、校验、字典、接口字段、前端选项和官方模型目录分支。
- 供应商仍可配置多个 `chat` 和多个 `image` 模型。
- 不根据模型名字进行运行时猜测；官方目录映射提供推荐类型，用户对未映射模型做显式确认。

### 5.2 智能体

智能体只保留真实运行配置：

```text
provider_id
provider_model_id
model_id / model_display_name snapshot
name
scenes
system_prompt
avatar
billing_multiplier_ppm
status / soft delete / timestamps
```

删除 `context_profile_id` 以及 Profile/Space 配置入口。工具绑定 `ai_agent_tools` 保留。

### 5.3 前端变化

- 删除“上下文工程”菜单、页面和全部管理组件。
- 删除智能体页面的“上下文配置”按钮与弹窗。
- Provider 模型编辑器只显示“对话”和“生图”两种类型。
- 删除向量维度、Embedding 最大输入、Token Counter、Rerank 等表单项。
- AI 对话参数面板显示“上下文轮数”和“最大输出 Token”。
- 删除消息 Citation 抽屉和 Run Detail 的 Context Plan 展示。
- 保留附件列表、附件预览、工具调用、Run Detail、Usage 和实际扣费展示。

## 6. 钱包与 AI 结算

### 6.1 为什么删除预冻结

当前实现根据理论最大输入、官方最大输出、文件上下文窗口和智能体倍率计算 Hold。对于超大上下文模型，它冻结的是“最坏情况报价”，不是用户本次实际消费，导致有真实余额的用户也无法开始一次普通对话。

预冻结没有解决本项目当前的真实问题，反而把钱包、Attempt、Run 和恢复流程绑成了复杂状态机。项目采用用户已确认的信用语义：允许一次已接受的真实消费把余额扣成负数，负余额阻止下一次调用。

### 6.2 新调用准入

只在接受一个新的用户 Run 时检查一次钱包：

```text
balance_units > 0  -> 接受 Run
balance_units <= 0 -> 拒绝 Run，不创建 Provider Attempt，不调用上游
```

规则：

- 沿用现有钱包惰性创建规则；用户没有钱包行时，在事务中创建余额为 0 的钱包，然后按余额不足拒绝本次 Run。
- 不计算最低预估费用。
- 不冻结任何余额。
- 不检查余额能否覆盖最大输出或附件理论成本。
- 已接受 Run 的 Provider 重试、工具续轮和终态收口不重复做余额准入，避免半途中断。
- 并发请求可能在同一正余额下同时通过；这是已接受的产品语义，不增加隐藏串行锁或并发额度。

### 6.3 真实用量结算

Provider 返回完整 Usage 后：

```text
不可变 Provider Usage
-> 使用 Run 接受时保存的 pricing snapshot
-> 生成 ai_usage_charge_items
-> 计算 actual_units
-> MySQL 事务锁定 user_wallets
-> 插入唯一 wallet_transaction(source_type=ai_run, source_id=run_id)
-> balance_units = balance_units - actual_units
-> total_consume_units += actual_units
-> 完成 charge 与 Run billing 终态
```

余额更新允许：

```text
100 - 30 = 70
10 - 30 = -20
-20 + 10 充值 = -10，仍不可调用
-10 + 20 充值 = 10，可以再次调用
```

结算必须满足：

- 钱包扣款、钱包流水、Charge、Charge Items 和 Run billing 终态在同一事务中完成。
- 现有唯一索引 `uk_wallet_transaction_source(source_type, source_id)` 保证一个 Run 只扣一次。
- 重试结算读取已存在流水并返回同一结果，不能重复扣款。
- `total_consume_units` 保持非负累计值；只有 `balance_units` 允许为负。
- 价格快照和智能体倍率继续保留，避免价格变化改写历史账单。
- 零价格或零用量是合法实际结算，不制造 Hold。

### 6.4 Usage 不完整

如果 Provider 没有返回完整、可验证的 Usage：

- 不根据文本长度、文件大小、图片数量或模型名称猜测。
- 不扣钱包。
- Charge 和 Run 进入 `unbilled`，原因使用 `unbilled_usage_incomplete` 或明确的 `legacy_unpriced`。
- 保留 Attempt、原始 Usage 证据、错误码和运行监控诊断。
- Reconciler 可以使用后来获得的权威 Usage 重试结算，但不能用估算值补账。

### 6.5 简化后的计费状态

Run `billing_status` 只保留：

```text
pending
settled
unbilled
```

Run `billing_reason` 只保留与真实结算有关的值：

```text
pending
settled_complete_usage
unbilled_usage_incomplete
legacy_unpriced
```

删除：

```text
held
released
released_before_dispatch
released_insufficient_balance
released_provider_failed
released_outcome_unknown
unbilled_over_hold
```

Provider 失败、取消、超时和结果未知继续由 Run/Attempt 的执行状态表达，不再通过“释放冻结”重复表达一次。

## 7. 数据库减法

### 7.1 删除上下文工程表

删除九张表：

```text
ai_context_bindings
ai_context_chunks
ai_context_document_versions
ai_context_documents
ai_context_plan_items
ai_context_plans
ai_context_profiles
ai_context_spaces
ai_conversation_memories
```

这些表只服务已退役的 Context/RAG 生命周期，不做兼容读写。

### 7.2 删除上下文字段和约束

`ai_agents`：

```text
DROP context_profile_id
DROP idx_ai_agents_context_profile
DROP fk_ai_agents_context_profile
```

`ai_provider_attempts`：

```text
DROP context_plan_id
DROP context_plan_sha256
DROP Context Plan 相关索引、外键和成对 CHECK
DROP quote_json
```

保留 `prepared_request_json`、请求哈希、响应哈希、原始 Usage 和 Provider Request ID，因为它们服务幂等、恢复和真实结算，不是 RAG 事实。

`ai_provider_models`：

```text
DELETE model_kind IN ('embedding', 'rerank') 的本地模型行
DROP embedding_dimensions
DROP embedding_max_input_tokens
DROP embedding_token_counter_id
重建 model_kind CHECK，仅允许 chat/image
删除 embedding 专用 CHECK 和索引分支
```

### 7.3 删除 Hold 数据结构

删除：

```text
wallet_holds 表
user_wallets.held_units
ai_usage_charges.held_units
Hold 相关外键、索引和 CHECK
Attempt quote_json 中的上限报价证明
Run/Charge 的 held/released 状态和值域
Run Dashboard 的 released_runs / released_units 派生字段
```

`user_wallets` 新约束：

```text
balance_units 允许负数
total_recharge_units >= 0
total_consume_units >= 0
```

`wallet_transactions` 的金额约束同步调整为：

```text
amount_units >= 0
balance_before_units 允许负数
balance_after_units 允许负数
```

流水必须原样记录扣款前后的负余额，不能用 0 截断或展示兜底掩盖真实账面。

保留：

```text
ai_usage_charges.actual_units
ai_usage_charge_items
ai_runs.pricing_snapshot_json
wallet_transactions
payment_recharges
payment_orders
payment_callback_events
```

### 7.4 迁移已有本地数据

迁移必须在 API/Worker 停止后执行，禁止一边有运行中 Run 一边拆 Hold：

1. 记录并终止或收口现有 running Run/Reply Command。
2. 已 `settled` 的历史 Charge、实际 Usage、Charge Items 和钱包流水原样保留。
3. 旧 `held`、`released` 或 `unbilled_over_hold` 行转换为新的 `unbilled` 语义，并保留 Run/Attempt 的执行结果。
4. 所有钱包 `held_units` 清零后再删除列和 `wallet_holds`。
5. 删除 Context 外键后删除九张表。
6. 删除 Embedding/Rerank 模型行，再收紧模型类型约束。
7. Run Dashboard 派生表按新的基础事实重建，不手工伪造历史聚合。

项目未上线，不保留长期双读、双写或旧字段兼容层。迁移文件必须是新数据库基线之后的一条或一组短 forward migration；已执行迁移不得回写。

## 8. 删除的后端运行时

当前已确认删除范围：

- `internal/module/ai/contextengine` 及其 Admin transport、jobs、reconciler 和 tests。
- `internal/infra/contextindex`、`internal/infra/contextindex/qdrant`。
- Context Memory、Conversation Index、Document Ingestion、Evaluation 和 Citation projection。
- Context Plan build/load/re-authorize/dispatch guard 链。
- Qdrant resource opener、readiness、lifecycle 和 telemetry。
- Embedding/Rerank Provider 协议与模型校验。
- Context 专用 Asynq task、cron 注册和 preflight CLI。
- `github.com/qdrant/go-client` 依赖。
- AI Gateway 的 Reserve/TopUp/EnsureActiveHold/Capture/Release 和上限报价验证。
- `payment/wallet` 中仅服务 AI Hold 的 model、repository 和状态转换。

保留并简化：

- Run 接受、请求幂等、Attempt 持久化、Provider dispatch 和 outcome evidence。
- Reply Command lease、取消、timeout、reconciler 和终态收口。
- Provider Usage 证据恢复。
- 对话附件 COS materialization。
- WebSocket start/delta/completed/failed/canceled 事件。
- 工具调用及续轮。

## 9. 删除的部署与脚本

删除 Qdrant 作为项目依赖：

- `deploy/docker-state/docker-compose.yml` 中的 qdrant service 和 qdrant-data volume。
- `deploy/docker-state/qdrant-image.env`。
- `admin-go.env.example` 和本地开发解析中的全部 `QDRANT_*`。
- `ADMIN_QDRANT_HTTP_HOST_PORT`、`ADMIN_QDRANT_GRPC_HOST_PORT`。
- Qdrant candidate 验证脚本、合同测试和数据库清理分支。
- API/Worker `/ready` 中的 qdrant component。

完成后本地状态服务只需要 MySQL 和 Redis；关闭向量能力不再是一个运行时配置，因为向量能力已经不存在。

## 10. API、合同和 UI 兼容边界

### 10.1 必须保持

- AI 供应商、智能体、会话、消息、Run、工具和生图的现有核心 REST 路径。
- 消息发送、停止、编辑、反馈和历史读取行为。
- `code/data/msg/error` 公共响应。
- WebSocket 事件类型和刷新后从 MySQL 恢复终态的行为。
- 附件上传、COS 对象引用、预览和权限校验。
- Run 详情的 Provider、模型、耗时、错误、Usage 和实际费用。
- 钱包余额、充值、支付宝支付和钱包流水。

### 10.2 明确删除

- `/api/admin/v1/ai-context-*` 和 Agent Context Profile/Space 接口。
- Context 管理菜单、页面、权限、字典和 i18n 文案。
- Context Plan/Citation 响应字段和前端生成物。
- Provider 的 Embedding/Rerank 模型字段与接口值域。
- Run Detail 的 Hold、Released、Context Plan、Retrieval、Rerank、Memory 和 Citation 诊断。

这些是明确的产品删除，不用空对象、`null` 或 deprecated 字段伪装兼容。

## 11. 错误处理

新增或保留稳定程序错误：

```text
ai.balance.insufficient
  当前余额 <= 0，拒绝新的 Run；HTTP 400，面向用户提示充值

ai.chat.history_attachment_invalid
  选中历史轮次的附件事实与 COS 对象不一致；明确失败，不丢附件继续回答

ai.chat.context_too_large
  当前输入或选中完整轮次超过本地可证明的硬限制

ai.provider.capability_unsupported
  当前模型不支持请求中的图片或文件能力

ai.billing.usage_incomplete
  Provider Usage 不完整；Run 可有执行终态，但本次不扣款
```

业务判断依赖 `error.code`，展示使用中英文 `msg`。不得通过 `|| ''`、`?? []`、空 Usage 或零费用兜底吞掉真实错误。

## 12. 状态与恢复

删除 Context Plan 后，恢复链更短：

```text
ai_messages + immutable attachment manifest
-> ai_reply_commands
-> ai_runs
-> ai_provider_attempts
-> provider outcome + usage evidence
-> ai_usage_charges / items
-> wallet_transactions
```

- Provider 请求重试从持久化消息、附件 manifest、运行参数和 Attempt 证据重建。
- 刷新页面从 MySQL 读取消息和 Run，不依赖 WebSocket 内存。
- Assistant 已经可见时，后续结算失败不得删除消息或把 Run 重新变成 running。
- `settlement_pending` 可以继续表达“回复已可见、账单尚在收口”，它不是 Hold。
- Reconciler 只修复 Reply/Run/Usage/Settlement，不再修复向量、Memory 或 Context Plan。

## 13. 实施分解原则

本文通过后再写计划。计划必须拆成可人工验收的短波次，不能一次改完所有 AI 文件：

```text
Wave A  数据库与模型值域前置迁移，建立可恢复点
Wave B  普通聊天最近 N 轮与历史附件，旁路并删除 Context Plan
Wave C  删除 Context 管理后端、Worker、前端和合同
Wave D  删除 Qdrant 配置、Docker、依赖和脚本
Wave E  钱包准入、负余额真实结算和 Hold 删除
Wave F  Run Dashboard、合同、文档和最终引用清理
```

每波都必须先证明新路径可用，再删除旧路径。不得为了“暂时能跑”保留 Context/Legacy 双运行时。

## 14. 定向验证与人工终验

实施阶段不默认运行全量长测试、Playwright 或 `admin-dev`。每波运行受影响模块短测试，最后由用户人工终验。

必须覆盖的后端事实：

- `max_history=0/1/20/50` 的完整轮次选择。
- stopped 可见回复进入历史，failed/pending 不进入历史。
- 上一轮图片、PDF 和普通文件在下一轮仍被物化并发送。
- 当前消息附件不因历史裁剪丢失。
- `max_tokens` 默认 4096，显式超官方模型上限返回 400。
- 余额 1 单位可以开始高成本 Run，真实结算后可变负数。
- 负余额和 0 余额拒绝下一次新 Run，且不创建 Attempt、不调用 Provider。
- 充值不足以转正时仍拒绝；充值后大于 0 恢复。
- 同一 Run 重复结算只产生一条钱包扣款流水。
- Usage 不完整不猜测、不扣款，Run 为 `unbilled`。
- Provider 失败、取消、timeout、outcome_unknown 均有稳定终态，不再依赖 Hold release。
- 刷新和 API/Worker 重启后消息、Run、Usage 和实际费用仍可恢复。

人工测试清单：

- [ ] 普通文本连续对话，刷新后消息仍在，Run 不停留 running。
- [ ] 第一轮上传图片，第二轮询问图片内容，历史图片仍生效。
- [ ] 第一轮上传 PDF，第二轮询问文件名和正文，历史文件仍生效。
- [ ] 同时上传图片和文件，消息展示和 Provider 请求均正确。
- [ ] 工具调用、工具结果和续轮正常。
- [ ] 停止生成后可见前缀保留，运行和结算最终收口。
- [ ] 小额正余额完成一次昂贵调用后显示负余额、真实费用和钱包流水。
- [ ] 负余额下一次调用收到明确余额不足通知。
- [ ] 充值后余额未转正仍拒绝；转正后恢复。
- [ ] 生图智能体和 `gpt-image-2` 仍走 image runtime。
- [ ] 上下文工程菜单、智能体上下文配置、Embedding/Rerank 表单和 Qdrant readiness 均已消失。

## 15. 不做的事情

- 不引入新的向量数据库、RAG 框架或本地 Embedding 模型。
- 不实现 ChatGPT 级跨会话长期记忆。
- 不用摘要模型替代已删除的 Memory。
- 不做自动授信额度、最大负债、按用户风控或冻结余额。
- 不猜测 Provider Usage。
- 不修改充值支付、支付宝验签或充值入账幂等语义。
- 不把聊天附件改造成知识库文档。
- 不为已删除接口建立长期 deprecated facade。

## 16. 完成标准

完成后必须同时满足：

- 用户不理解 Profile、Space、Embedding 或 Qdrant 也能完整使用 AI。
- 普通聊天只依赖 MySQL、Redis、COS 和已配置 Provider。
- 最近 N 轮语义可从代码直接读懂，历史附件不会在下一轮神秘消失。
- 调用前不再计算理论最大冻结额。
- 正余额可开始 Run，真实结算允许负余额，非正余额阻止下一次 Run。
- 每一笔实际扣款都能从 Provider Usage、价格快照、Charge Items 和钱包流水追溯。
- 回复持久化、Run 终态和结算故障互不互相删除事实。
- 数据库中不存在 Context/Memory/Hold 死表和死字段。
- Docker、配置、依赖、菜单、合同和前端中不存在 Qdrant/Embedding/Rerank 残留。
- AI 主流程仍支持附件、WebSocket、工具、运行监控、生图、余额和充值。

## 17. 最终原则

```text
聊天就是聊天，不是知识库平台。
附件就是消息事实，不是向量文档。
Usage 就是真实账单，不是理论最大报价。
余额可以为负，但下一次调用必须先充值。
删除一套错误的状态机，胜过给它增加一个“可选”开关。
```
