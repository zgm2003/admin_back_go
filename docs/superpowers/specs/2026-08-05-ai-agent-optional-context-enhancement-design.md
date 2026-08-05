# AI Agent 可选上下文增强设计

**日期：** 2026-08-05

**状态：** 设计已确认，待用户复核书面规格

**涉及仓库：** `admin_back_go`、`admin_front_ts`

**上游规格：** `docs/superpowers/specs/2026-08-01-ai-context-engineering-design.md`

**覆盖范围：** 本文只覆盖上游规格第 5.4 节和第 8.7 节中“Embedding、Qdrant、Reranker 或 Memory 故障必须使 Context Plan 和 Run 失败”以及“`ready` Plan 不能保存错误诊断”的规则，并补齐供应商模型类型、Context Profile 选择和降级体验。上游规格的数据所有权、权限复核、不可变 Plan、Prepared Request、计费、消息持久化和历史附件真相仍然有效。

## 1. 最终结论

Embedding 是 Agent 的可选上下文增强能力，不是聊天主链的生命维持依赖。

系统必须同时满足：

1. 未配置 Embedding 时，现有聊天、当前附件、WebSocket、消息持久化、Run 终态和计费保持可用。
2. 已配置 Embedding，但供应商欠费、鉴权失败、限流、超时或服务异常时，本轮聊天仍可完成。
3. 降级不能伪装成 `no_hit`、`skipped` 或正常命中；Context Plan 必须保存 `degraded` 事实和稳定错误码。
4. 权限、当前附件、必需上下文、请求快照和 Plan 一致性错误仍然失败，不能借“可用性”绕过正确性与安全性。
5. Embedding 模型由 Context Profile 直接引用供应商模型，不增加 `embedding` 智能体场景。
6. 不增加本地 Embedding Docker；本期使用 OpenAI-compatible API，首选硅基流动 `Qwen/Qwen3-Embedding-0.6B`、1024 维。

这次是现有 Context Engine 的收口升级，不新建第二套 RAG、Memory、Provider 或 Run 状态机。

## 2. 为什么调整原设计

上游规格选择了严格失败语义：存在可检索来源时，Embedding、Qdrant、Reranker 或 Memory 故障会产生 `failed` Context Plan，并阻止主模型调用。这能防止错误被吞掉，但把可选增强层变成了聊天硬依赖。

真实故障会因此扩大：

- Embedding API Key 到期会让普通问候失败；
- Qdrant 短暂不可用会让当前图片或当前文件对话失败；
- 后台索引异常会影响消息回复、Run 终态和用户刷新后的体验；
- 用户无法区分“主模型失败”和“历史资料暂时没被检索”。

正确的数据结构不是继续增加重试或兜底分支，而是把上下文分为两层：

```text
Core Context（必须成功）
  系统提示词、当前消息、当前附件、最近完整轮次、工具续调、输出预算

Enhancement Context（允许显式降级）
  Conversation Memory、历史附件索引、空间文档、Dense/Sparse 检索、Reranker
```

主链只依赖 Core Context。Enhancement Context 失败时，系统构建一份不含未验证证据的不可变降级 Plan，然后继续主模型调用。

## 3. 参考系统结论

OpenClaw 与 Hermes Agent 都说明 Agent 身份不依赖 Embedding：

- [OpenClaw Memory](https://github.com/openclaw/openclaw/blob/main/docs/concepts/memory-search.md) 以 `USER.md`、`MEMORY.md` 和每日 Markdown 文件保存记忆；Embedding 配置后使用向量与关键词混合检索，未配置时仍可使用关键词路径。
- [Hermes Agent](https://github.com/NousResearch/hermes-agent) 的主循环、工具、技能、终端与消息网关独立运行；跨会话 Memory 通过可选 Provider 插件扩展。

本项目不复制它们的文件结构或插件系统，只采用相同边界：主 Agent 可独立运行，Embedding 负责提高相关内容召回率。

## 4. 方案选择

### 4.1 不采用：上下文硬失败

Embedding 或 Qdrant 故障直接失败 Run，错误最明显，但违反“没有 Embedding 不影响整体使用”的产品约束。

### 4.2 不采用：静默回退

把故障改写成 `skipped` 或 `no_hit`，实现最少，但会掩盖密钥错误、欠费、限流和索引故障，运行监控也无法解释回答为何没有引用资料。

### 4.3 采用：显式降级

新增 `degraded` Retrieval Outcome。Context Plan 仍为 `ready`，保存 Plan Hash、降级阶段和错误码；主模型继续执行，但不得携带失败管线产生的部分候选或伪造引用。

## 5. 核心不变量

### 5.1 主聊天不依赖 Embedding

- Agent 未绑定 Context Profile 时，不创建 Embedding 请求。
- Agent 已绑定 Profile，但没有启用来源时，不创建 Embedding 请求。
- 当前附件继续由既有原生附件协议交给主模型，不等待解析、分块或索引。
- 异步摄取失败只影响该文档版本或索引代次，不删除消息，不取消已经完成的助手回复。

### 5.2 降级必须可见

- `degraded` 不能映射成 `skipped`、`no_hit` 或 `hit`。
- Context Plan 保存一个确定性的主要降级阶段和稳定错误码。
- Run Detail 投影该错误码到现有 `diagnostic_codes`，不新增第二套 Run 错误字段。
- 聊天界面显示非阻塞状态，明确本轮未引用空间文档或历史附件的检索资料；不能笼统声称“完全未使用历史”，因为近期完整轮次或已独立验证的 Ready Memory 仍可能在 Plan 中。

### 5.3 不使用半套检索结果

Embedding、Qdrant 或 Reranker 任一可选增强阶段失败后，本轮全部 Retrieval Evidence 都丢弃。不能把 Dense 失败后的 Sparse 结果、Reranker 失败前的 RRF 结果或权限复核前的 Qdrant 候选继续交给模型。

Memory 在检索前解析。本轮按 Profile 和完整轮次边界应当存在 Memory，但没有可验证的 Ready Memory 时，保存 `memory_unavailable` 并停止后续检索；不得一边声明增强降级，一边继续生成一组用户无法判断完整性的 Citation。尚未达到 Memory 生成阈值属于正常无 Memory，不是故障。

MySQL 权威复核不是可降级依赖。已知旧 Point、已撤权 Point 或已失效来源按既有闭合原因排除并异步清理；没有剩余候选时是正常 `no_hit`。无法读取权威事实、无法证明候选授权，或 Plan 提交前已选来源发生权限/快照冲突时严格失败。数据库、事务或 Repository 本身出错时，不得粗暴映射为 `retrieval_failed`，不得持久化一份虚假的业务失败 Plan，也不得调用主模型。错误返回现有 Reply Command/Finalizer 路径重试或收口，避免出现“模型已经回复，但消息无法落库且 Run 一直运行中”。

未来可以单独设计明确的 `lexical_only` Profile；本期不把运行时故障临时解释成另一种检索策略。

### 5.4 权限与必需输入仍然严格失败

以下错误不得降级：

- `ai.context.permission_denied`
- `ai.context.snapshot_conflict`
- `ai.context.required_overflow`
- `ai.context.tool_continuation_overflow`
- `ai.context.attachment_unavailable`（当前请求的必需附件）
- `ai.context.plan_conflict`
- MySQL 权威复核、Plan 提交事务或消息持久化的基础设施错误

这些错误涉及授权、用户当前输入、真相存储或不可变请求证明。继续派发可能泄漏数据、丢失用户输入或破坏重试一致性。

### 5.5 Provider 重试不重新检索

`ready + degraded` Plan 与普通 `ready` Plan 一样不可变。Prepared Request 固定后，主模型重试只复用精确请求字节，不因为 Embedding 恢复而在同一个 Run 中重新构建上下文。

## 6. 状态模型

`retrieval_outcome` 的闭合集合扩展为：

```text
skipped | no_hit | hit | degraded | failed
```

状态语义如下：

| 条件 | Plan State | Retrieval Outcome | Provider Dispatch |
| --- | --- | --- | --- |
| 未绑定 Profile | `ready` | `skipped` | 继续 |
| 没有可检索来源 | `ready` | `skipped` | 继续 |
| 检索完成但没有候选 | `ready` | `no_hit` | 继续 |
| 检索成功并选入证据 | `ready` | `hit` | 继续 |
| 可选增强依赖失败 | `ready` | `degraded` | 继续 |
| 权限、必需输入或一致性失败 | `failed` | `failed` | 阻止 |

### 6.1 `ready + degraded` 形状

`ai_context_plans` 不新增列，复用现有字段：

```text
state              = ready
retrieval_outcome  = degraded
plan_sha256         = NOT NULL
error_stage         = NOT NULL
error_code          = NOT NULL
error_message       = NULL 或脱敏非空文本
```

Plan Hash 覆盖 `retrieval_outcome`、`error_stage` 和 `error_code`，不覆盖可能漂移的人类错误文案。Context Plan Items 只保存最终进入 Provider 请求的 Core Context 和已经完整验证的增强块。

### 6.2 可降级错误

以下稳定错误码在同步 BuildPlan 中转为 `degraded`：

- `ai.context.profile_unavailable`
- `ai.context.embedding_failed`
- `ai.context.index_failed`
- `ai.context.index_inconsistent`
- `ai.context.retrieval_failed`
- `ai.context.rerank_failed`
- `ai.context.memory_unavailable`

只有已经分类为上述闭合错误码的增强依赖故障才能降级：`profile_unavailable` 要求 Profile 已成功读取但状态或配置不可用；`retrieval_failed` 只表示 Qdrant 查询、响应或检索变换失败；`memory_unavailable` 只表示本轮按策略应当存在、但派生 Memory 状态或 Memory Provider 不可用。权限冲突、快照冲突和未分类的数据库/Repository 错误不允许落入这些错误码兜底。

后台 Document Version、Memory 和 Rebuild Job 仍保留自己的 `failed` 状态和有限重试；只有聊天 Run 的派发决策发生变化。

## 7. 数据所有权与组件边界

### 7.1 供应商与模型

`ai_providers` 继续拥有：

- `engine_type`
- `base_url`
- 加密 API Key
- 供应商健康与同步状态

`ai_provider_models` 继续拥有模型身份和用途：

```text
chat | embedding | rerank
```

同一个硅基流动供应商可以同时拥有三种模型。模型用途不是 Agent Scene，不创建 Embedding Agent。

### 7.2 智能体

`ai_agents` 仍只绑定 `chat` Provider Model。现有场景保持：

```text
chat | agent_generate | text_generate | image_generate
```

Agent 通过可空 `context_profile_id` 获得上下文增强；该字段为空时纯聊天行为不变。

### 7.3 Context Profile

Context Profile 直接引用：

- 必填的 `embedding_provider_model_id`
- 显式 `embedding_dimensions`
- 显式 `embedding_max_input_tokens`
- 显式 Token Counter 和 Dense Distance
- 可选 Reranker Provider Model
- 可选 Conversation Memory Chat Model

Profile 的索引身份保持不可变。更换模型或维度必须创建新 Profile 并重建索引，不能把不同向量混进同一 Collection。

### 7.4 真相源

- MySQL：消息、文档、Chunk 正文、Profile、Plan、Memory 和授权真相。
- COS：原始附件真相。
- Qdrant：可重建 Dense/Sparse 派生索引。
- Provider：只接收本轮最终 Context Plan。

Embedding API、Qdrant 或派生索引故障不能修改原始消息和附件事实。

## 8. 供应商配置升级

### 8.1 后端契约

现有 Provider API 已支持结构化模型：

```json
{
  "models": [
    { "model_id": "gpt-5.6", "model_kind": "chat" },
    { "model_id": "Qwen/Qwen3-Embedding-0.6B", "model_kind": "embedding" }
  ]
}
```

保留旧 `model_ids` 输入作为兼容入口；它只协调 `chat` 模型。`model_ids` 与 `models` 不能同时提交。新管理端统一提交 `models`，避免把 Embedding 模型误写为 Chat。

Provider Model DTO、OpenAPI 和生成 TypeScript 必须公开 `model_kind`。现有 Agent 模型选项继续只返回启用的 `chat` 模型。

远端 `/models` 同步继续只协调 Chat 模型。同步不得修改、停用或删除管理员显式配置的 Embedding/Rerank 行；否则一次普通模型同步就会破坏 Context Profile。

### 8.2 前端表单

供应商表单对每个已选模型显示：

- Model ID
- 显示名称
- 模型类型：Chat / Embedding / Rerank
- 启停状态（编辑模型清单时）

已有模型使用服务端 `model_kind`，不得在编辑时重置为 Chat。远端 `/models` 只提供候选 ID，不能可靠推断用途；新选择项默认 Chat，但类型控件必须始终可见并可修改。

供应商列表中的模型摘要显示类型，避免同名模型用途不清。

### 8.3 硅基流动首个配置

```text
Name: 硅基流动
Engine Type: openai
Base URL: https://api.siliconflow.cn/v1
API Protocol: chat_completions（仅影响 Chat；Embedding Adapter 使用 /embeddings）
Model ID: Qwen/Qwen3-Embedding-0.6B
Model Kind: embedding
Dimensions: 1024
Distance: cosine
```

API Key 只保存在后端加密字段，不返回前端、不写日志、不进入 Plan。

## 9. Context Profile 与 Agent 配置体验

上下文工程 Page Init 返回按用途分组的启用模型选项：

- `embedding_model_options`
- `reranker_model_options`
- `memory_model_options`

Profile 表单把当前裸数字 `embedding_provider_model_id` 输入改为 Embedding 模型下拉框，显示“供应商 / 模型”。Reranker 和 Memory 使用对应类型选项。

Agent 表单继续选择 Chat 模型和业务场景，并可选绑定 Context Profile。未绑定时不显示错误；绑定失败或 Profile 不可用时在管理页告警，但不阻止普通 Chat Agent 被选择。

## 10. 聊天运行数据流

### 10.1 无 Profile

```text
Current Input
  -> Existing Core Context Builder
  -> Prepared Request
  -> Main Provider
```

不访问 Embedding、Qdrant 或 Context Profile。已有用户行为保持不变。

### 10.2 Profile 正常

```text
Core Context
  + Ready Conversation Memory
  + Query Embedding
  + Qdrant Dense/Sparse/RRF
  + MySQL Authority Check
  + Optional Rerank
  -> Immutable ready/hit or ready/no_hit Plan
  -> Prepared Request
```

### 10.3 Profile 降级

```text
Core Context
  + 已经成功读取且仍有效的 Ready Memory
  + Enhancement Pipeline Failure
  -> Discard incomplete retrieval evidence
  -> Immutable ready/degraded Plan
  -> Prepared Request
  -> Main Provider
```

主模型请求必须带有结构化内部指令：本轮未获得历史/空间检索证据，不得声称已经读取这些来源，不得生成 Citation。该指令不是用户消息，也不修改原始 `ai_messages`。

如果在 Memory 阶段已经确定 `memory_unavailable`，本轮不再调用 Embedding、Qdrant 或 Reranker，直接用 Core Context 生成同样不可变的 `ready + degraded` Plan。

## 11. 附件与记忆行为

### 11.1 当前附件

当前用户上传的图片和文件继续走 COS 原始对象和 Provider 原生附件输入。Embedding 不参与当前附件能否发送给主模型。

### 11.2 历史附件

- 无 Profile 时保持现有兼容行为，避免本次升级破坏旧会话。
- Profile 正常且文档版本 Ready 后，后续请求优先使用检索 Chunk，不重复发送全部历史文件。
- Profile 降级时不重新引入“每轮重传全部历史附件”作为兜底；该行为会恢复已确认的延迟和失败问题。
- 用户需要历史文件事实时，聊天界面明确提示本轮未引用空间文档或历史附件的检索资料；用户可以重试或重新附上当前需要的文件。

### 11.3 Conversation Memory

已有 Ready Memory 是持久化派生事实，仍属于 Enhancement Context。只有当它已经独立读取、校验且未受本轮失败阶段影响时，才可以进入最终 Plan。新的 Memory 生成失败不覆盖仍满足策略的旧 Ready Memory，也不阻止聊天；此时失败只保留在后台 Job/Admin 诊断，不污染当前 Plan。尚未达到生成阈值也是正常状态。只有本轮按策略应当存在 Memory、但派生状态不可用时，才不使用 Memory 并产生 `degraded` 诊断；MySQL 读取错误仍按基础设施错误严格失败。

本期不新增跨会话全局 Saved Memory，不把 Conversation Memory 偷换成用户画像。跨会话用户记忆需要独立的作用域、删除、纠正、来源和隐私设计，后续单独立项。

## 12. 前端状态与运行监控

### 12.1 聊天界面

当助手回复对应 `retrieval_outcome=degraded` 时，消息附近显示非阻塞状态：

```text
本轮知识检索暂不可用，回答未引用空间或历史附件资料
```

该文案不否认仍可能进入 Plan 的近期完整轮次或 Ready Memory。

状态不是 Assistant 文本，不写入对话历史，不参与下一轮 Prompt。刷新后从 Message Context 投影恢复，不能只存在前端内存。

### 12.2 运行监控

Run Detail 显示：

- Plan State：`ready`
- Retrieval Outcome：`degraded`
- 主要降级阶段
- 稳定错误码
- Query Embedding、Retrieval 和 Rerank 已发生的有界耗时/请求统计
- 空 Citation 列表

Run 主状态仍按主模型执行结果进入成功或失败；上下文降级不能让成功 Run 永久显示“运行中”。

### 12.3 管理告警

Provider、Profile 和 Document Version 的真实失败状态继续保留。`degraded` 只描述一次聊天 Plan，不把供应商或索引故障改写为健康。

## 13. Readiness 与可观测性

### 13.1 API Readiness

`admin-api` 的总体 Ready 由聊天核心依赖决定。Embedding、Qdrant 或 Reranker 不可用时，上下文组件报告 `degraded`，但不能让纯聊天 API 总体 Down。

### 13.2 Worker Readiness

`admin-worker` 继续报告索引任务依赖的真实状态。Context Worker 失败不能阻止其他队列消费者工作；Readiness 输出必须区分核心队列与上下文增强组件。

### 13.3 指标

至少保留或增加：

```text
context_plan_total{outcome="degraded",code}
context_plan_duration_seconds{stage}
context_embedding_requests_total{outcome}
context_degraded_total{stage,code}
```

指标标签只能使用闭合阶段和错误码，不能放 User ID、Conversation ID、文件名、Query 或供应商原始错误文本。

## 14. 计费与终态

- Embedding、Rerank 和后台 Memory 仍是平台成本，不进入用户钱包。
- 主模型 Hold、Attempt、Usage 和结算使用最终 Prepared Request，不受被舍弃的检索候选影响。
- 主模型成功时 Run 正常结算，即使 Context Outcome 为 `degraded`。
- 主模型失败时按现有 Finalizer 收口，不能把 Embedding 的降级码覆盖为主模型最终错误码。
- 取消、超时和未知结果仍使用现有状态机，不增加 Context 专用终态。

## 15. 兼容性

### 15.1 API

- 旧 Provider `model_ids` 请求继续工作，并只更新 Chat 模型范围。
- 新前端使用 `models`，显式携带 `model_kind`。
- 原有 Agent Scene 和 Agent API 不增加 `embedding`。
- Context Plan DTO 增加闭合枚举值 `degraded`，属于兼容扩展。

### 15.2 数据

- 现有 Provider Model 行不改身份。
- 现有 Context Plan 是终局审计事实，不回写历史 `failed` Plan。
- Schema Migration 只替换 Context Plan 的 Retrieval Outcome 和终态 CHECK，不新增业务表。
- 旧会话、消息、附件、Run、Attempt 和账单不迁移语义。

### 15.3 用户行为

- 没有 Context Profile 的 Agent 行为不变。
- 当前附件行为不变。
- 已经完成的回复刷新后仍存在。
- Run 不能因上下文增强故障停留在非终态。

## 16. 安全与错误处理

- Provider 原始错误先分类并脱敏，再映射到稳定 Context Error Code。
- API Key、签名 URL、文件正文和用户 Query 不进入 `error_message`、日志标签或指标。
- 文档摄取只把当前 Chunk 文本发送给选定 Embedding Provider；查询只发送有界 Query Variant；Reranker 只接收有界候选正文。不得把完整会话、COS 签名 URL 或无关附件一并外发。
- 管理员选择外部 Embedding/Rerank Provider 即确定该 Profile 的数据处理边界；Run Detail 只展示供应商、模型、请求计数和脱敏诊断，不回显发送正文。
- 降级时不使用未经 MySQL 权威复核的 Qdrant 候选。
- 降级时不生成 Citation，不保留失败阶段的半成品 Context Plan Items。
- 权限或快照错误保持 Fatal，不能以可用性为理由继续。

## 17. 测试策略

### 17.1 后端聚焦测试

- Provider `models` 能保存 `chat/embedding/rerank`，旧 `model_ids` 仍只协调 Chat。
- 远端模型同步只协调 Chat，保留已有 Embedding/Rerank 的身份、类型和状态。
- Agent 模型选项不包含 Embedding/Rerank。
- Context Profile 只接受启用且类型匹配的模型。
- `ready + degraded` Plan 通过类型、哈希和数据库 CHECK。
- Embedding 401、429、超时、5xx 和维度错误生成确定性降级 Plan。
- Qdrant、Reranker、Memory 故障不阻止主 Provider Dispatch。
- `memory_unavailable` 停止本轮后续检索；未达到 Memory 生成阈值和仍有效的旧 Ready Memory 不产生假降级。
- Permission、Snapshot、Required Overflow 和当前附件错误仍然失败。
- 候选权威复核的数据库错误不降级、不调用 Provider，并由现有 Reply Command/Finalizer 路径重试或收口。
- Profile/Memory 的数据库读取错误不伪装成 `profile_unavailable` 或 `memory_unavailable`。
- 同一 Run 重试复用已保存的降级 Plan，不二次调用 Embedding。
- 降级 Run 正常进入结算终态，错误码只进入 Context/Diagnostic 投影。

只运行相关 Go Package、Schema Contract 和前端单元测试，不运行长脚本、全量 Docker 集成或付费主模型探针。

### 17.2 前端聚焦测试

- Provider 表单编辑后保留每个模型的 `model_kind`。
- 新模型能显式选择 Chat、Embedding 或 Rerank。
- Context Profile 下拉框只显示匹配类型的模型。
- `degraded` 状态和错误码正确展示。
- 聊天降级提示刷新后仍存在，不写入 Assistant Content。

### 17.3 用户人工验收

1. 不配置 Embedding，发送普通消息；必须正常回复，刷新后双方消息都存在。
2. 不配置 Embedding，上传当前 PDF 或图片；支持该附件类型的主模型必须正常回答。
3. 新增硅基流动供应商，把 `Qwen/Qwen3-Embedding-0.6B` 标记为 Embedding，创建 1024 维 Profile 并绑定 Agent。
4. 上传中文架构文档，等待版本 Ready；使用不同措辞追问，Run 应显示 `hit` 和真实 Citation。
5. 故意写错 Embedding API Key；普通聊天仍成功，Run 为成功终态，Context Outcome 为 `degraded`。
6. 降级时刷新页面，消息不能消失，Run 不能停留在“运行中”。
7. 降级时询问历史文档，界面必须提示未引用空间或历史附件检索资料，不能伪造 Citation。
8. 降级时上传一个当前新文件，主模型仍能直接处理；后台索引失败不能删除消息。
9. 恢复 API Key 并重建失败版本，再次提问应恢复 `hit/no_hit` 正常状态。
10. 回归原有 Chat Provider、Agent Scene、工具调用、图片对话、文件对话、WebSocket 和计费。

用户不使用 Playwright；浏览器流程由用户人工验收。

## 18. 发布与回退

### 18.1 发布顺序

1. 扩展 Schema CHECK 和后端 Context Outcome。
2. 完成 Runtime 降级、Plan 持久化、Run Diagnostic 和聚焦测试。
3. 发布 OpenAPI 并同步前端生成契约。
4. 完成 Provider 类型配置、Context Profile 选项和降级展示。
5. 用户配置硅基流动并人工验收。

### 18.2 回退

代码回退前不能直接恢复不接受 `degraded` 的旧 CHECK，否则已写入的新 Plan 会阻止 DDL。回退顺序必须先停止新流量、确认不存在或迁移 `degraded` Plan，再回退 Schema 和代码。

Provider Model Kind、Profile 和文档索引均保留；回退不能删除用户消息、附件、Run 或账单。

## 19. 验收清单

- [ ] 未配置 Embedding 时，聊天主链完全可用。
- [ ] 已配置但 Embedding/Qdrant/Reranker/Memory 故障时，主模型仍可回复。
- [ ] 降级使用 `ready + degraded`，不伪装成 `skipped/no_hit`。
- [ ] 权限、当前附件和必需上下文错误仍严格失败。
- [ ] Provider UI 能显式保存模型类型，旧 Provider 配置不被重置。
- [ ] Embedding 模型不进入 Agent 选择器，不新增 Agent Scene。
- [ ] Context Profile 使用类型过滤后的模型选项，不要求手填数据库 ID。
- [ ] 当前附件不依赖 Embedding；历史文档正常时使用检索，降级时不伪造引用。
- [ ] 降级回复刷新后存在，Run 正常终态，钱包正常结算。
- [ ] Run Monitor 能解释降级阶段和稳定错误码。
- [ ] 不新增 Embedding Docker、不新增 Provider 表、不新增重复 Context 状态机。
- [ ] 聚焦自动化测试通过，用户完成人工浏览器验收。
