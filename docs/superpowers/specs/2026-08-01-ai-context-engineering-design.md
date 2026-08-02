# AI 上下文工程重构设计

**日期：** 2026-08-01

**状态：** 设计已确认，待书面规格复核

**涉及仓库：** `admin_back_go`、`admin_front_ts`

**取代范围：** 现有 AI 知识库页面、`internal/module/ai/knowledge`、`KnowledgeRuntime`、六张 `ai_knowledge_*` / `ai_agent_knowledge_bases` 表，以及用户可调的 `max_history` 运行参数。

**保留边界：** 现有消息持久化、历史附件、WebSocket、Provider Compiler、Prepared Request、Run、Attempt、计费与统一 finalizer 仍是权威主链；本文只重构它们进入 Provider 前的上下文装配方式。

## 1. 最终结论

本次不在旧“知识库”字符匹配实现上继续增加 Embedding、分支或兼容层，而是建立独立的“上下文工程”能力：

1. Go 内实现项目自己的 `ContextEngine`、`ContextPlan`、`ContextBlock`、`Evidence` 和 Token Budget。
2. MySQL 保存业务事实、不可变文档版本、Chunk 正文、上下文计划与审计。
3. Redis/Asynq 运行文档解析、分块、Embedding、索引和会话记忆任务。
4. Qdrant 保存可重建的 Dense/Sparse 派生索引，不拥有权限、文档状态或正文真相。
5. 聊天不再按固定条数加载历史，也不把知识文本拼进当前 `userContent`。
6. 当前消息、历史轮次、附件、会话记忆、文档证据和工具协议统一进入一份不可变 Context Plan。
7. Provider 请求报价和持久化前固定 Plan；恢复和重试复用原请求，不重新检索。
8. 旧页面、旧 API、旧权限、旧代码和旧六表在最终切换时一次性删除，不双写、不永久兼容。

该方案允许大规模内部重构，但不授权破坏已经稳定的用户行为：用户消息、助手消息、历史附件、Run 终态、Prepared Request 和钱包结算必须继续一致。

## 2. 当前问题与根因

当前 `internal/module/ai/knowledge` 不是完整 RAG：

- `retriever.go` 使用 `strings.Contains` 对标题和正文做词面加分；
- `chunker.go` 按字符窗口切分，不理解 Markdown 标题、段落、代码、PDF 页或表格；
- `service.go` 把绑定空间的 Chunk 分页全部加载到 Go 内存再评分；
- 每个绑定独立保存 `top_k`、`min_score` 和 `max_context_chars`，合并后没有统一模型窗口预算；
- 检索结果直接拼到当前用户正文前，污染消息语义；
- 检索错误被吞掉后继续调用模型，`failed` 与 `no_hit` 无法区分；
- 当前 `infraai.ChatInput.Inputs` 是 `map[string]any`，历史、附件和系统提示依赖隐式键名；
- `max_history` 把模型窗口管理推给最终用户，无法表达附件大小、工具协议、输出预留和不同模型窗口。

本地运行库在设计确认时已再次验证：

```text
ai_knowledge_bases             0
ai_knowledge_documents         0
ai_knowledge_chunks            0
ai_agent_knowledge_bases       0
ai_knowledge_retrievals        0
ai_knowledge_retrieval_hits    0
```

因此当前环境没有旧知识数据迁移价值。正式迁移仍必须验证每个环境的实际行数，不能把本地事实当成生产事实。

## 3. 目标与非目标

### 3.1 目标

1. 管理员可以创建上下文空间，上传 TXT、Markdown、可提取文本的 PDF、DOCX、CSV 和 XLSX，并绑定给一个或多个智能体。
2. 文档摄取采用不可变版本；新版本完整可用前，旧版本继续服务。
3. 使用 Dense + Sparse 检索、RRF 融合和显式可选的 Reranker。
4. 每个召回结果经过 MySQL 权限、绑定、文档状态和活动版本复核。
5. 用模型上下文窗口、输出预留和协议上界自动装配历史、附件、记忆和证据。
6. 当前附件和完整消息轮次不会被静默拆散或丢弃。
7. 长对话通过最近完整轮次、会话私有检索和滚动记忆维持连续性。
8. Assistant 引用与 Context Plan 证据一一映射，可从聊天和 Run 详情审计。
9. 同步 Context Build 的所有故障在 Provider Dispatch 前明确失败并由现有 Run/finalizer 收口；已声明为异步增强的 Memory/Backfill 另行记录，不伪造当前 Run 失败。
10. 最终删除旧模块、旧 API、旧页面、旧参数和旧表，只保留新架构所需事实。

### 3.2 非目标

- 首期不实现扫描 PDF、图片 OCR、PPT、音频、视频或压缩包解析。
- 首期不实现 GraphRAG、知识图谱、LightRAG 或多模态长期记忆。
- 首期不允许普通用户创建永久个人资料库；普通用户只使用管理员授权空间。
- 首期不接入 Dify、RAGFlow 或其他完整平台作为聊天运行时。
- 首期不让 Eino 接管现有 ChatModel、工具、流式输出、Run 或计费主链。
- 首期不使用 LLM Query Rewrite；检索查询使用可复现的当前问题和最近完整轮次变体。
- 首期不把外部 Embedding、Rerank 和后台记忆调用计入用户钱包；它们是平台运营成本，但必须记录可用量、耗时和失败事实。
- 首期不删除原始消息或原始附件来换取存储空间。

## 4. 开源组件采用决策

| 项目 | 首期定位 | 决策依据 |
| --- | --- | --- |
| Qdrant | 正式运行时依赖 | Apache-2.0；官方 Go Client；支持 Dense、Sparse、Payload Filter、IDF 与 RRF；单独 Docker 服务符合现有部署 |
| CloudWeGo Eino | 设计参考，首期不接管主链 | Apache-2.0、Go 原生，但其 ChatModel、Graph、Callback 与当前 gateway/run/finalizer 重叠 |
| RAGFlow | 摄取、版本和观测参考 | Apache-2.0，但完整部署引入 Python、MySQL、Redis、MinIO、Elasticsearch/Infinity 等第二套平台能力 |
| Dify | 管理流程和 LLMOps 参考 | 完整应用平台；使用带附加条件的 Dify Open Source License；不作为内嵌核心依赖 |
| LlamaIndex / Haystack / LangChain | Context Packing、Pipeline 和评估参考 | 主要是 Python 运行时；引入后会制造第二个编排边界 |
| GraphRAG / LightRAG / RAG-Anything | 后续独立能力 | 图检索和多模态价值明确，但不属于首期最小闭环 |
| Milvus | 不采用 | 面向更大规模分布式场景，当前部署和运维成本过高 |
| pgvector | 不采用 | 会为向量检索额外引入 PostgreSQL，与当前 MySQL 事实源冲突 |
| Weaviate | 不采用 | 当前需求不需要其集成式向量化和更大的服务边界 |

首期直接依赖固定为：

- `github.com/qdrant/go-client v1.18.3`，该版本要求 Go 1.25，项目 Go 1.26.5 满足要求；
- Qdrant Server 采用支持 Sparse IDF、QueryBatch 与 Query API RRF 的固定稳定版本，Compose 禁止使用 `latest`；
- PDF 文本层解析使用 BSD 许可的 `github.com/ledongthuc/pdf`；
- XLSX 继续使用项目已有的 `github.com/xuri/excelize/v2`；
- Markdown 使用 AST 解析器，DOCX 使用受限的 ZIP/XML 读取，CSV 使用标准库。

Eino 只有在后续某个单一 adapter 能明确减少本地代码、并通过现有接口测试时才允许引入；不能因为“框架完整”替换已稳定的架构。

Go Client 版本和 Go 工具链已用模块元数据验证。设计阶段不把未成功拉取并验证的 Qdrant Server tag 写成事实；Slice 2 必须实际拉取候选镜像、记录 RepoDigest、运行真实 Sparse IDF/QueryBatch/RRF/Filter 契约测试，再把 `tag@sha256:digest` 写入 Compose。Tag、网页说明或 `docker manifest inspect` 单独成功都不能替代运行时能力测试。

## 5. 核心不变量

### 5.1 MySQL 是唯一业务真相

Qdrant 只保存向量和最小过滤 Payload。以下事实只能来自 MySQL：

- 用户、平台、会话和智能体身份；
- 智能体与上下文空间绑定；
- 文档启停、删除和活动版本；
- Chunk 正文、定位信息和内容哈希；
- Context Plan、预算、选择/排除决定与内容快照。

删除 Qdrant Volume 后，系统必须能从 MySQL 和对象存储完整重建索引。

### 5.2 Plan 在 Provider 请求前不可变

每个聊天 Run 最多有一份 Plan 事实：成功时是不可变 `ready` Plan，真正的 Context 故障时是不可变 `failed` Plan，取消或超时发生在提交前则没有 Plan。Plan 必须在报价和 Prepared Request 写入前固定，并通过 `plan_sha256` 绑定 Provider Attempt。Provider 重试只能复用已经持久化的精确请求，不能再次查询 Qdrant。

重复 Reply Worker 调用 BuildPlan 时先按 `run_id` 读取已有 Plan；存在即返回该终局事实，不进入检索：`ready` 继续接受 Dispatch Guard，`failed` 直接返回原错误。两个 Worker 并发越过首次读取时，由 `run_id` 唯一键决定唯一赢家；输家丢弃自己的外部结果并重新读取，绝不拿 `NULL` 的 Failed Plan Hash 比较或覆盖赢家。只有仍持有当前 Reply Command Lease 的 Worker 才能继续，且必须对赢家 Plan 执行 Dispatch Guard；过期 Worker 直接退出。`ai.context.plan_conflict` 留给已持久化 Attempt/Plan/Prepared Hash 关系不一致，不能被并发输家拿来抢终态。取消信号在 Plan 提交事务中再次检查，避免取消后补写一份假失败 Plan。

### 5.3 权限必须在两处验证

Qdrant Filter 用于减少候选，但不是授权。候选返回后必须批量回 MySQL 验证绑定、文档状态和活动版本；Provider 派发前还必须验证空间授权和文档未被禁用/删除。

### 5.4 不吞错误

- 没有启用 Profile，或既没有启用 Space 来源、ready Conversation 文档版本，也没有可索引的已完成轮次：`skipped`；
- 有授权来源但没有候选：`no_hit`；
- Qdrant、Embedding、Reranker、权限复核或预算不变量失败：Plan 和 Run `failed`；
- 可选块因预算排除：Plan `ready`，但必须记录排除原因。

`failed` 不能伪装成 `no_hit`，`no_hit` 也不能伪装成系统故障。

### 5.5 当前消息和协议原子性

当前用户消息、当前附件、系统指令、工具定义和输出预留是必需项。历史按完整轮次分组，工具调用与工具结果成组；任何组不能只保留一半。

### 5.6 派生内容不覆盖原始事实

Chunk、Embedding、会话记忆和附件文本解析都是派生数据。它们可以失效、重建或删除，但不能修改原始文件、`ai_messages` 或已准备 Provider 请求。

## 6. 架构边界

```text
Admin API / Chat Command
          |
          +--> contextengine HTTP service
          |       +--> spaces / documents / profiles / evaluation
          |
          +--> ContextRuntime.BuildPlan(run snapshot)
                  |
                  +--> AuthorizationSnapshot (MySQL)
                  +--> ConversationSource (messages + attachments + memory)
                  +--> RetrievalPipeline (Embedding + Qdrant + RRF + Rerank)
                  +--> ContextPacker (typed blocks + token budget)
                  +--> PlanRepository (MySQL immutable audit)
                  |
                  +--> Typed ChatInput
                          |
                          +--> Provider Compiler
                                  |
                                  +--> existing Gateway / Attempt / Finalizer

Object Storage --> Ingestion Worker --> Parser --> Chunker --> MySQL --> Embedding --> Qdrant
```

### 6.1 `internal/module/ai/contextengine`

该模块拥有：

- Profile、Space、Document、DocumentVersion、Chunk、Binding；
- Context Plan 与 Plan Item；
- 文档摄取状态机；
- 检索、融合、重排和 Pack 策略；
- 会话记忆状态与 Run 证据投影；
- Admin HTTP DTO、Service 与 Repository 接口。

它不拥有：

- 供应商密钥；
- 通用上传签名；
- Provider 协议 JSON；
- 钱包冻结与结算；
- WebSocket 连接和消息投递。

### 6.2 `internal/infra/contextindex/qdrant`

该 adapter 只实现 collection、point、filter 和 query 操作，不读取业务 Repository，不判断权限，也不返回正文。

### 6.3 `internal/infra/documentparser`

该包按 MIME/扩展名选择 Parser，返回统一结构块与定位信息。Parser 不写数据库、不调用 Embedding、不拼 Prompt。

Embedding 与 Rerank 通过 `internal/infra/ai` 的供应商中立接口调用，并由 Profile 引用的 `ai_provider_models` 行解析实际 Provider。Context Engine 不判断供应商名称、不拼供应商 URL，也不在模型不可用时选择另一个模型。

### 6.4 `internal/infra/ai`

`ChatInput` 从 `Content + Inputs map[string]any` 收敛为有类型结构：

```go
type Message struct {
    Role  MessageRole
    Parts []ContentPart
}

type ContentPart struct {
    Kind       ContentPartKind
    Text       string
    Attachment *AttachmentRef
}
```

Provider Compiler 负责把统一结构转换为 Chat Completions、Responses 或其他供应商协议。业务模块不再通过隐式 map 键传递 `system_prompt`、`history` 和 `attachments`。

`MessageRole` 和 `ContentPartKind` 使用闭合枚举。构造器/Validator 保证 Text Part 只有非空 Text、Attachment Part 只有受信 `AttachmentRef`，禁止“两者都有”“两者都空”或未知 Kind 进入 Compiler；类型化不是把无效状态从 map 搬进 struct。

## 7. Context Plan 数据结构

`ContextPlan` 至少包含：

```text
plan_id
run snapshot resolved from run_id
context profile snapshot
policy_version
model capability snapshot
input fingerprint
plan hash
retrieval outcome
protocol/token safety budget components
known input budget and selected upper bound
effective output reserve
opaque attachment flag
selected blocks
excluded blocks
stage durations
```

`ContextBlock` 包含：

```text
kind
source identity and source hash
required
atomic group key
priority
text token upper bound
attachment reference without bytes/base64
retrieval scores
decision and exclusion reason
citation key
content snapshot when required for audit
```

允许的首期 Block Kind：

```text
system_instruction
current_user_message
current_attachment
recent_turn
recalled_turn
history_attachment
conversation_memory
document_evidence
tool_definition
tool_call
tool_result
```

Plan Item 的 `citation_key` 只分配给入选的 `document_evidence`，格式固定为 `C1`、`C2`，按最终 Pack 顺序生成。

### 7.1 哈希边界

`context_profile_sha256` 对 Profile 的不可变配置闭合清单计算，覆盖 Embedding/Rerank/Memory Provider Model 身份、Embedding 维度/输入上限/Counter ID、距离、阈值、Sparse Encoder 与版本；不覆盖名称、启停/退休状态、Active/Target Generation、索引状态或错误时间。

`input_fingerprint_sha256` 在检索前生成，覆盖消息 ID/内容哈希、附件对象事实、Agent/Provider/Model/Profile/Active Index Generation 快照、Binding 快照、工具定义、生成参数和 `policy_version`；不包含耗时、Plan/Plan Item 等本次审计行 ID 或外部检索结果。Message、Document、Version、Chunk 等来源 ID 属于来源身份，必须包含。

`plan_sha256` 只对 `context_plan_v1` 闭合清单计算 SHA-256。清单包含 Input Fingerprint、预算证明、按最终顺序排列的 Block/原子组、来源哈希、附件引用事实、选择/排除决定、规范化分数和 Citation Key；不包含 Plan ID、创建时间、阶段耗时或错误文案。所有分数在 adapter 边界转换为固定精度十进制，禁止 NaN/Infinity，并使用同一值排序、持久化和哈希。`ready` Plan 必须有 Plan Hash，`failed` Plan 的 Plan Hash 为 `NULL`。

## 8. 最小数据库模型

新模块只创建九张表。下面完整列出业务字段；统一时间戳、字符集和项目已有审计字段遵循仓库既有约定，实施时不得随意扩张业务字段。

### 8.1 `ai_context_profiles`

职责：保存不可变的检索/记忆模型配置。

核心字段：

```text
id
name
embedding_provider_model_id
embedding_dimensions
embedding_max_input_tokens
embedding_token_counter_id
dense_distance               cosine | dot | euclid
dense_min_score
sparse_encoder               unicode_lexical_v1
sparse_encoder_version
reranker_provider_model_id   nullable
reranker_min_score           nullable
memory_provider_model_id     nullable
status                       enabled | retired
active_index_generation      nullable
target_index_generation      nullable
index_state                  provisioning | ready | rebuilding | failed
index_error_code             nullable
index_verified_at            nullable
created_by
```

规则：

- Profile 一旦被 Space、Agent 或 Version 引用，Provider Model、Embedding 维度/上限/Counter、距离、Dense/Rerank 阈值、Sparse Encoder/Version 和 Memory Model 等全部策略字段不可修改；变更必须创建新 Profile。名称、退休状态以及 Generation/Index State/验证时间不属于配置身份，其中索引运行字段只能通过 CAS 更新。
- `enabled` 允许新建引用；`retired` 禁止新 Space、Binding 和 Version，但已绑定 Agent、活动 Version 和历史 Plan 继续可读，直到显式迁移。禁用业务读取应操作 Space/Document，不能靠退休 Profile 突然破坏现有聊天。
- Embedding Provider Model/Dimensions/Max Input Tokens/Token Counter ID/Distance/Dense Threshold 全部必填；Reranker Provider Model/Threshold 必须同时为空或同时有；Memory Provider Model 可空。每个 `*_provider_model_id` 都是 `ai_provider_models.id` 外键，Provider/Model 字符串只从该权威行解析，不在 Profile 重复保存。数据库 CHECK 与 Service 使用同一闭合规则。
- Reranker 未配置表示策略明确为 `disabled`；已配置时调用失败必须使本次检索失败，禁止回退到融合分数。
- Space 只有在没有 Document/Version 且没有 Binding 时才能切换 Profile；出现任一引用后，模型迁移通过创建新 Space 并重新摄取完成，不预建复杂的双 Profile 切换状态机。
- Agent 选择 Profile 和新 Binding 都只允许引用 `index_state=ready` 的 Profile。`rebuilding` 时现有聊天继续读取 Active Generation，但 Document/Conversation Index Worker 不领取该 Profile 的新 Point 写任务，Version 保持 `queued`。健康 Active Generation 的重建失败必须回到 `ready` 并保留安全错误事实；只有没有可验证 Active Generation 时才进入 `failed`。BuildPlan 先固定授权来源：既没有启用 Space 来源、ready Conversation 文档版本，也没有可索引已完成轮次时仍为 `skipped`，不因一个实际未使用的失败索引阻断纯聊天；存在应检索来源时 `failed` Profile 必须使 Run 失败。
- Generation 严格单调。合法 CAS 形状只有初建 `provisioning(NULL,target) -> ready(active,NULL)`、健康重建 `ready(active,NULL) -> rebuilding(active,target) -> ready(target,NULL)`，以及无健康 Active 时 `failed(active?,target?) -> rebuilding(active?,target) -> ready(target,NULL)`；健康重建失败回到原 `ready(active,NULL)`。数据库 CHECK 约束 Active/Target 的空值组合，Reconciler 不能凭错误文案猜状态。

### 8.2 `ai_context_spaces`

职责：管理员维护的共享上下文空间。

核心字段：

```text
id
platform
profile_id
name
description
status                       enabled | disabled
deleted_at                   nullable
created_by
```

Space 删除立即使其不可检索；历史 Plan 快照仍保留。

### 8.3 `ai_context_documents`

职责：文档逻辑身份和当前活动版本。

核心字段：

```text
id
space_id                     nullable
conversation_id              nullable
source_message_id            nullable
source_attachment_index      nullable
title
active_version_id            nullable
status                       enabled | disabled
deleted_at                   nullable
created_by
```

约束：

- `space_id` 与 `conversation_id` 必须且只能有一个非空；数据库 CHECK 和 Service 同时验证。
- Space 文档由管理员管理；Conversation 文档只来自该用户消息的受信附件。
- Space Document 的 `source_message_id/source_attachment_index` 全部为 `NULL`。
- Conversation Document 必须同时保存 `source_message_id` 和从 0 开始的 `source_attachment_index`，并验证该位置的附件属于该消息和会话；User/Agent 身份只从权威 `ai_conversations` 读取，不在 Document 复制。
- Conversation Document 唯一键为 `(conversation_id, source_message_id, source_attachment_index)`；一条消息的多个附件各自拥有 Document，不能用文件名或可变 URL 当身份。
- `active_version_id` 必须为空，或通过 Document `(id, active_version_id)` 到 Version `(document_id, id)` 的复合外键指向属于本 Document 的 Version。Service 在同一激活事务锁定并验证 Version 为 `ready`，且其 Profile 等于 Space Profile 或 Conversation 所属 Agent 的 Context Profile；复合外键只负责阻止跨 Document 挂错 Version，不假装数据库 CHECK 能跨表验证状态。

### 8.4 `ai_context_document_versions`

职责：一次不可变的文件事实、解析、分块和索引代次。

核心字段：

```text
id
document_id
profile_id
source_storage_provider
source_object_key
source_etag
source_size_bytes
source_mime_type
source_filename
source_facts_sha256
source_sha256                 nullable until conditional stream succeeds
parser_name
parser_version
chunker_version
state                        queued | processing | ready | failed
failure_stage                nullable
error_code                   nullable
error_message                nullable, sanitized
chunk_count
embedding_input_token_upper_bound
embedding_request_count
embedding_input_tokens       nullable
started_at                   nullable
finished_at                  nullable
attempt_count
lease_token                  nullable
lease_expires_at             nullable
```

原始对象事实属于 Version，不属于 Document。`source_facts_sha256` 对 Storage Provider、Object Key、ETag、Size、MIME 和 Filename 的闭合清单计算，创建 Version 时即可得到；`source_sha256` 由 Worker 条件读取对象时流式计算，只允许从 `NULL` 写入一次。请求已提供内容 SHA 时必须匹配，否则 Version 失败。替换文件必须创建新 Version。

状态字段使用数据库 CHECK：`ready` 必须有 Source SHA、正数 Chunk/Embedding Token 统计、完成时间且没有错误；`failed` 必须有失败阶段、稳定错误码和完成时间；`queued/processing` 不能伪造完成事实。终态清空租约，但失败前已经产生的 Chunk/Point 由清理任务按 Version 身份回收。

### 8.5 `ai_context_chunks`

职责：MySQL 中可重建索引的规范 Chunk 正文。

核心字段：

```text
id
document_version_id
ordinal                      unique within version
heading_path
content
content_sha256
chunk_facts_sha256
embedding_input_token_upper_bound
locator_json                 closed, versioned locator schema
created_at
```

`content_sha256` 只对正文计算，用于内容去重；`chunk_facts_sha256` 对 Heading Path、Content Hash 和版本化 Locator 的闭合清单计算，是 Qdrant Payload、Point ID 和 Plan Source Hash 使用的权威 Chunk Hash。Dense/Sparse 的规范 Index Text 是 Heading Path + 正文，算法由 Chunker Version 固定。Chunk 不保存 `status`、`is_del`、`top_k` 或分数。可见性完全由 Document 和 `active_version_id` 决定。

### 8.6 `ai_context_bindings`

职责：智能体与 Space 的多对多授权。

核心字段：

```text
id
agent_id
space_id
status                       enabled | disabled
created_at
updated_at
```

唯一键为 `(agent_id, space_id)`。不保存 `top_k`、`min_score`、`max_context_chars` 或 Chunk 参数。

一个 Agent 的所有启用 Binding 必须与 `ai_agents.context_profile_id` 指向同一 Profile。Service 在绑定事务中验证，Context Runtime 在构建 Plan 时再次验证；发现混用 Profile 直接返回配置错误，不跨不同 Embedding 空间比较不可比的分数。

### 8.7 `ai_context_plans`

职责：每个 Run 的不可变上下文计划头。

核心字段：

```text
id
run_id                       unique
context_profile_id_snapshot    nullable
context_profile_sha256         nullable
context_index_generation_snapshot nullable
policy_version
input_fingerprint_sha256
plan_sha256                  nullable for failed
model_capability_sha256
api_protocol_snapshot
token_counter_id_snapshot
context_window_tokens
effective_output_tokens
provider_protocol_upper_bound
tool_continuation_input_reserve
policy_safety_margin
known_input_budget
known_input_upper_bound
budget_proof                  exact | conservative | opaque_attachment
retrieval_outcome             skipped | no_hit | hit | failed
state                         ready | failed
error_stage                   nullable
error_code                    nullable
error_message                 nullable, sanitized
metrics_json                  closed, versioned stage metrics
created_at
```

Plan 不存在长期 `building` 状态。完整成功或完整失败事实一次事务写入。

Platform/User/Conversation/Agent/Provider/Model 身份不在 Plan 重复建列，统一从 `run_id` 指向的不可变 `ai_runs` 接受快照读取；这些 Run 身份字段一旦接受就禁止修改。领域 `ContextPlan` 可以携带解析后的 Run Snapshot，但 Repository 不能把它写成第二份真相。

数据库 CHECK 保证 `ready` 必须有 Plan Hash、非 `failed` Retrieval Outcome 且没有错误；`failed` 必须是 `retrieval_outcome=failed`、Plan Hash 为 `NULL` 并保存错误阶段和稳定错误码。`run_id` 唯一键消灭同一 Run 的第二份 Plan。

`known_input_upper_bound` 是所有已选 Block 的已知 Payload 上界之和，不含已经单列的协议包装。预算标量还必须满足 `known_input_budget = context_window_tokens - effective_output_tokens - provider_protocol_upper_bound - policy_safety_margin >= 0`、`tool_continuation_input_reserve <= provider_protocol_upper_bound` 和 `known_input_upper_bound <= known_input_budget`；`opaque_attachment` 只承认附件内部 Token 不可知，不允许已知文本越过该不等式。

`metrics_json` 使用 `context_plan_metrics_v1` 闭合结构，只保存各阶段毫秒数、候选数量、Query Embedding/Rerank 请求次数及供应商返回的可用 Token Usage。它不保存 Query、正文或原始 Provider 响应；首期这些调用是平台成本，不进入用户钱包账单。

### 8.8 `ai_context_plan_items`

职责：保存本次 Plan 考虑过的有界候选及最终决定。

核心字段：

```text
id
plan_id
ordinal
block_kind
source_type
source_ref
source_sha256
atomic_group_key
required
priority
decision                      selected | excluded
exclusion_reason              nullable
token_upper_bound
fusion_score                  nullable
rerank_score                  nullable
citation_key                  nullable
content_snapshot              nullable
metadata_json                 closed, versioned metadata schema
```

只有有界检索池、相关历史候选和最终块进入该表，不复制整段无限会话。Attachment 只保存受信引用和对象事实，不保存二进制或 Base64。

唯一键为 `(plan_id, ordinal)`，非空 Citation Key 在 Plan 内唯一。数据库 CHECK 保证 `selected` 没有 Exclusion Reason、`excluded` 必须有闭合原因；Citation Key 只能出现在已选 `document_evidence`。已选文本类 Block 必须保存有界 Content Snapshot，排除项和二进制 Attachment 不复制正文/字节，只保存 Source Hash 与闭合 Metadata。Service 负责固定 `C1..Cn` 连续顺序和各 Block Kind 的闭合 Metadata DTO，不能用空字符串绕过空值语义。

### 8.9 `ai_conversation_memories`

职责：保存可验证的滚动会话记忆。

核心字段：

```text
id
conversation_id
context_profile_id_snapshot
context_profile_sha256
previous_memory_id            nullable
from_message_id
through_message_id
source_sha256
summary_sha256               nullable for failed
policy_version
summary                       nullable for failed
prompt_tokens                 nullable
completion_tokens             nullable
provider_request_id           nullable
state                         ready | failed | invalidated
error_code                    nullable
created_at
```

Memory 是从会话首个有效完整轮次开始的累积派生事实，不替代消息。第一条 Memory 直接总结原始轮次；后续 Memory 输入同 Profile 的上一条 ready Summary 和新增加的完整轮次，保存 `previous_memory_id`。`source_sha256` 对 Profile ID/Hash、父 Memory ID/Summary Hash 和本次新增的统一 ConversationTurn Hash 清单计算，因而覆盖消息、附件、工具事实与 Assistant Delivery State；`summary_sha256` 校验输出。唯一键为 `(conversation_id, context_profile_id_snapshot, through_message_id, source_sha256)`。

MySQL 8.4 不允许 `CHECK` 引用本表 `AUTO_INCREMENT` 列，因此数据库只能直接约束 `from_message_id <= through_message_id`，不能用 `previous_memory_id <> id` 伪装成可执行约束。Memory Repository 插入时禁止调用方预设新行 ID，在锁定 Conversation 后验证父节点是同 Profile 的最新有效节点、覆盖边界连续且父节点不是候选自身；父身份和区间字段插入后不可修改。这样自环和分叉在唯一写入口被消灭，不引入触发器或第二套 ID 分配器。

数据库 CHECK 保证 `ready` 必须有 Summary、Summary Hash 且没有错误；`failed` 的两个摘要输出字段必须为 `NULL` 并保存稳定错误码；`invalidated` 只允许从 `ready` 转入并保留原摘要用于审计，但 Planner 永远不读取。禁止用空字符串伪装失败摘要。

Chunk/Version 的 `embedding_input_token_upper_bound` 只绑定不可变 Profile 的 Embedding Counter，用于摄取上限和批处理审计。Memory 不保存伪通用 Token 数。Planner 必须用当前 Agent Chat Model 的 Tokenizer/保守算法重新计算 Chunk、Memory、消息和工具 Block 的 `ai_context_plan_items.token_upper_bound`；Profile 可被不同 Chat Model 使用时也不会复用错误计数。

Planner 只读取父链连续、覆盖边界连续、Summary Hash 正确的最新 `ready` Memory。编辑、删除消息或改变附件/工具轮次事实时，历史操作事务把同一 Conversation 中 `through_message_id >= changed_message_id` 的 ready Memory 一次性标记为 `invalidated`，再异步从原始事实重建；累积区间保证这正好覆盖受影响节点及后代，不需要递归特殊处理。失败或失效 Memory 永远不能通过默认值继续使用。

Memory Worker 在调用模型前固定 Profile、父 Memory 和原始轮次 Hash，但不持有长数据库事务。写入结果时锁定 Conversation 并重新计算：Profile、最新有效父节点、覆盖边界或 Source Hash 任一变化，都把本次结果当作过期派生结果丢弃并补投当前任务，不写分叉链；只有仍匹配的结果才能插入 `ready`/终态 `failed` 行。外部临时错误在 Asynq 固定重试耗尽前不插入 `failed` 行。

### 8.10 对现有表的必要修改

`ai_provider_models` 增加闭合的 `model_kind`：

```text
chat | embedding | rerank
```

已有行迁移为 `chat`。Provider Model 的 `provider_id/model_id/model_kind` 在被 Agent 或 Context Profile 引用后不可修改，变更身份必须创建新行；现有 Agent/聊天模型查询必须显式过滤 `chat`，Context Profile 的 Embedding、Reranker、Memory 分别只允许引用 `embedding`、`rerank`、`chat`，避免新模型行混入现有 Agent 选择器。Context Profile 只能引用用途匹配、已启用且渠道可用的 Provider Model。

`ai_agents` 增加 nullable `context_profile_id`。已有 Agent 回填 `NULL`，保持纯聊天可用；管理员在 Agent 配置中显式选择一个已就绪 Profile，Binding 事务只验证 Space 使用同一 Profile，不能靠“第一次绑定”暗中修改 Agent。这样没有共享 Space 的 Agent 也能启用 Conversation 私有索引和 Rolling Memory。Profile 为空时仍使用自动 Token Budget 装配当前消息和最近完整轮次，但 Dense/Sparse、Conversation 私有索引和 Rolling Memory 明确禁用。首期 Profile 已产生启用 Binding、ready Conversation Version、可索引完整轮次或 Memory 后不可修改或清空；模型迁移使用新 Agent/Space，避免维护双索引过渡状态机。Profile 初次赋值触发该 Agent 历史终态轮次和支持附件的有界补建任务。

`ai_provider_attempts` 增加 nullable `context_plan_id` 和 `context_plan_sha256`。非聊天 Attempt 和历史行保持 `NULL`；新聊天 Attempt 的 Prepared Evidence 事务必须同时写入 Plan ID/Hash，并验证 Plan 属于同一 Run 且状态为 `ready`。这两个字段只建立证据关系，不改变现有 `prepared_request_json`、`prepared_request_sha256` 或请求恢复规则。

不向 `ai_messages` 添加 `max_history`、Chunk、Embedding 或摘要字段；旧 `meta_json.runtime_params.max_history` 保持历史可读并在新代码中忽略。

## 9. Qdrant 设计

### 9.1 Collection

每个不可变 Context Profile 使用稳定 Alias 和代次化 Physical Collection：

```text
alias:       admin_context_profile_<profile_id>
physical:    admin_context_profile_<profile_id>_g<generation>
```

Collection 包含两个命名向量：

```text
dense                         profile 固定维度和距离函数
sparse                        unicode_lexical_v1 + IDF modifier
```

MySQL 的 `active_index_generation` 是服务读取真相。Context Runtime 和 Point Writer 都用 Profile ID + 本次 MySQL Generation 快照拼出并访问具体 Physical Collection，不通过 Alias 猜当前代次；Alias 只提供稳定的运维指针、原子切换和 Readiness/Reconciler 一致性检查。这样 Alias 与 MySQL 两个系统之间无法原子提交的短窗口不会把零候选误判为 `no_hit`。

Point 同时承载文档 Chunk 和可召回的完整会话轮次。Point ID 使用 `sha256("admin-context-point-v1\\0" + profile_id + "\\0" + source_kind + "\\0" + source_id + "\\0" + source_sha256)` 的前 16 字节，并设置 RFC 9562 UUIDv8 Version/Variant 位；同一事实重试只会 Upsert 同一个 Point，不同来源不会因整数 ID 相同发生碰撞。Payload 使用闭合字段：

```text
platform
profile_id
index_generation
scope_kind                    space | conversation
source_kind                   document_chunk | conversation_turn
source_id                     chunk_id 或锚点 user_message_id
space_id                      nullable
conversation_id               nullable
user_id                       nullable
document_id                   nullable
document_version_id           nullable
chunk_id                      nullable
source_sha256
```

Payload 不保存 Chunk 正文、文件名、用户输入、Embedding 原文、签名 URL 或权限快照。查询先使用 Profile 和 Scope Filter 缩小候选，真正的正文和授权仍批量读取 MySQL。

Collection 只为实际 Filter 字段建立 Payload Index：`profile_id`、`index_generation`、`platform`、`scope_kind`、`source_kind`、`space_id`、`conversation_id`、`user_id`。`source_sha256` 仅用于复核和清理，不建立高基数索引。

Qdrant Server 必须支持 Sparse IDF、QueryBatch 和 Query API RRF。启动时验证能力；版本不满足时 `/ready` 失败，不在运行时偷偷退回另一套评分算法。

### 9.2 查询范围

每次检索先从 MySQL 固定 Profile、Active Physical Collection 和授权快照，再生成一个 Qdrant Filter：

```text
profile_id = current_profile
AND index_generation = mysql_active_generation
AND (
  (scope_kind = space
    AND platform = current_platform
    AND space_id IN enabled_agent_bindings)
  OR
  (scope_kind = conversation
    AND platform = current_platform
    AND conversation_id = current_conversation
    AND user_id = current_user)
)
```

Filter 只减少无关候选，不能替代权限校验。Qdrant 返回有界的 Chunk/Turn 来源身份后，Repository 必须按来源类型用固定数量的批量查询加载候选，并逐项验证，不能产生 N+1：

```text
profile.id == candidate.profile_id
profile.active_index_generation == candidate.index_generation
space/profile or conversation/profile relationship is valid
binding is still enabled for space candidates
document_chunk: document is enabled and not deleted
document_chunk: document.active_version_id == candidate.document_version_id
document_chunk: version.state == ready
document_chunk: chunk.document_version_id == candidate.document_version_id
document_chunk: chunk.chunk_facts_sha256 == candidate.source_sha256
conversation_turn: anchor user message, persisted tool call/result groups and paired assistant message form one completed turn
conversation_turn: turn belongs to current user and conversation
conversation_turn: recomputed canonical ConversationTurn hash == candidate.source_sha256
```

旧 Point、已撤权 Point 或哈希不一致 Point 都不能进入 Plan。已知旧 Point 被记录为排除项并异步清理；MySQL 查询失败、授权快照失败或索引事实冲突则明确失败，不能当作 `no_hit`。

### 9.3 重建与清理

Collection 可以从 MySQL Chunk、完整轮次、Profile 和对象存储重建，过程固定为：

1. Profile 级 fencing lock 先阻止新的 Point Writer，并等待已经持有租约的 Writer 完成；随后把 `target_index_generation` 设为下一代并进入 `rebuilding`。Context Runtime 继续按 MySQL 快照读取 Active Physical Collection，新 Document/Conversation Index 任务留在队列中。
2. 创建新的 Physical Collection，从 MySQL/对象存储重建全部活动文档 Chunk 与有效 Conversation Turn。
3. 对 Active Document Version 校验完整预期 Point ID 集合、数量、Chunk Facts Hash、向量维度和 Index Generation；对重建期间持续产生的 Conversation Turn 只校验已写入 Point，自切换后的确定性 Backfill 补齐其余可选缓存。
4. 使用 Qdrant 原子 Alias 操作把稳定 Alias 从旧代次切到目标代次。
5. MySQL 事务把 Target 提升为 Active、清空 Target、状态恢复 `ready`，随后释放 Version 激活。旧 Physical Collection 至少保留一个大于 BuildPlan 最大执行时间和有界 adapter 重试时间的退役宽限期；宽限期结束意味着旧快照读取已被超时边界收口，清理前再确认它不再是 MySQL Active/Target 或 Alias 目标，不能刚提交代次就删除仍被在途快照读取的旧 Collection。

Alias 已切换但 MySQL 尚未提交时，Context Runtime 仍读取旧 Active Physical Collection；MySQL 提交后才读取新代次。Reconciler 根据已记录 Target 和 Alias 指向完成或回滚切换，Readiness 在两者不一致时失败，不能同时信任两个代次。禁止在原 Physical Collection 上边删边补，也不为重建做运行时双写。

健康 Active Generation 上的主动重建失败时，删除 Target、保留旧 Alias 并恢复 `ready`，同时保存安全错误码；Active Generation 已损坏或缺失时先进入 `failed`，修复任务再用 CAS 进入无健康 Active 的 `rebuilding` 并写 Target，Context Run 在新代次校验并切换成功前持续失败。

一致性 Reconciler 分批比较 MySQL 事实与 Qdrant Scroll 结果，但按数据语义处理：`ready` Active Document Version 承诺已索引，缺 Point、维度错误、哈希错误或 Active Physical Collection Schema 错误会把 Profile 标为 `failed`、阻断实际依赖该来源的 Context Run 并排队完整重建；Conversation Turn 没有独立业务状态表，是可选的最终一致派生缓存，缺 Point 只补投确定性任务并记录指标，不能因为一个刚完成或等待回填的轮次拖垮整个 Profile。Alias/Generation 不一致由 Reconciler 修复并使 Readiness 失败，Runtime 仍只信任 MySQL 指定的 Physical Collection。候选 Point 即使存在，也必须经过逐项 MySQL 复核。

文档版本切换后，旧版本 Point 可异步删除。删除失败不影响正确性，因为 MySQL 的 `active_version_id` 复核会过滤旧 Point。Space、Document 或 Conversation 删除时先使 MySQL 授权失效，再排队清理派生 Point。

## 10. 文档摄取

### 10.1 状态机

Document Version 只有四个状态：

```text
queued -> processing -> ready
                     -> failed
```

`ready` 只表示该版本已完整解析、分块、Embedding 并写入 Qdrant；只有 `ai_context_documents.active_version_id` 指向它时才可检索。状态倒退、覆盖 `ready` Version 或修改不可变来源事实都属于错误。

`processing` 使用 `lease_token + lease_expires_at` fencing。Worker 只有持有当前 Token 才能提交阶段结果；崩溃后的过期租约由 Reconciler 重新排队。`attempt_count` 只记录处理代次，不产生第二套任务状态。

可重试的网络/依赖错误在尝试耗尽前保持 `processing`，释放或等待当前租约过期后由新 Token 继续幂等阶段；不能为了重试把状态倒退为 `queued`。只有永久错误或固定次数耗尽才 CAS 到 `failed`。因此 `failed` 是业务终态，不是 Asynq 某一次执行失败的别名。

### 10.2 创建与排队

1. API 验证管理员权限、Space/Profile 状态、上传对象事实和支持格式。
2. 一个 MySQL 事务创建或复用 Document，并插入不可变的 `queued` Version。
3. 事务提交后入队 `ai:context-document-index:v1`。
4. 入队失败时 API 只能返回真实的 `queued` 资源状态并记录 dispatch pending，不能声称已开始处理；周期 Reconciler 扫描无有效任务租约的 `queued` Version 并补投。

任务幂等键包含 Version ID、Profile ID、Source Facts SHA-256、Parser Version 和 Chunker Version。同一事实重复投递必须得到相同内容 SHA、Chunk Hash 和 Point ID。

同一 Version 的重试按 `(document_version_id, ordinal)` Upsert Chunk，但只允许所有不可变字段与已有行完全相同；相同 Ordinal 出现不同 Heading/Content/Locator/Hash 时返回确定性错误并终止，不能覆盖第一次结果掩盖 Parser/Chunker 非确定性。

“重新索引”不把 `failed` 或 `ready` Version 改回 `queued`，而是用相同对象事实和当前 Parser/Chunker Version 创建新的不可变 Version。这样所有状态转换保持单向，旧 Plan 仍能指向原版本。

### 10.3 解析边界

Parser 输出统一结构块：

```text
text
heading_path
locator
ordinal
```

首期格式与定位：

| 格式 | 解析规则 | Locator |
| --- | --- | --- |
| TXT | UTF-8 或 BOM 明确标识的 UTF-16LE/BE；拒绝猜测本地编码和二进制伪装 | 行区间 |
| Markdown | AST 保留标题层级、段落、列表、表格和代码块原子性 | 标题路径 + 块序号 |
| PDF | 只接受可提取文本层；无有效文本时明确判定扫描 PDF 不支持 | 页码 + 页内块序号 |
| DOCX | 受限读取 ZIP/XML；拒绝宏、外链对象和损坏包 | 标题路径 + 段落序号 |
| CSV | 标准 CSV 解析，保留表头和完整行 | 行区间 |
| XLSX | 复用 Excelize，只读取单元格值和合并区域语义 | Sheet + 单元格范围 |

Parser 必须限制压缩展开比、页数/Sheet 数、单元格数、文本字节数和处理时间。格式不支持、扫描 PDF、加密文件或资源上限超出属于永久错误，不无限重试。

### 10.4 分块与索引

Chunker 优先在标题、段落、列表、表格行和代码块边界切分；只有单一结构块超过上限时才使用版本化的 Token 窗口。上限取版本化服务策略与 Profile 已固化的 `embedding_max_input_tokens` 较小值；该字段在 Profile 创建时必须由 Adapter 真实能力验证，不能在摄取时读取一个会漂移的临时默认值。它不暴露旧 `chunk_size_chars` 和 `chunk_overlap_chars` 用户参数。

Worker 流程固定为：

1. 条件读取对象，验证 ETag/大小，流式计算并在已提供 SHA 时验证 SHA-256；
2. 解析并生成稳定结构块；
3. 分块、计算内容哈希和 Profile Embedding Counter 的保守输入 Token 上界；
4. 在 MySQL 写入该 Version 的规范 Chunk；
5. 分批请求 Dense Embedding，并生成一致的 Sparse Vector；
6. 以确定性 Point ID 幂等 Upsert Qdrant；
7. 校验 Chunk 数、Point 数、维度和 Chunk Facts Hash；
8. 最后一个 MySQL 事务锁定 Document 和候选 Version，把候选标为 `ready`，然后把 `active_version_id` 设置为该 Document 当前 ID 最大的 `ready` Version；较老 Version 晚完成不能覆盖已经就绪的较新 Version。Document 已删除、候选 Profile 不再匹配或 Worker Lease 失效时禁止激活。

永久错误或固定重试耗尽只把候选 Version 标为 `failed`，保留阶段、稳定错误码和脱敏消息；旧 `active_version_id` 不变。若较新 Version 失败，Reconciler 仍选择 ID 最大的 `ready` Version，不需要回退分支。失败清理只能删除候选 Version 的派生 Point，不能删除当前活动版本。

## 11. 检索管线

### 11.1 可复现查询

首期不调用 LLM 改写 Query。Planner 只构造两个确定性变体：

1. 当前用户文本；
2. 当前用户文本加最近一个完整 ConversationTurn 的有界规范检索文本。

空文本、规范化后重复的变体在请求 Embedding 前按 Query Hash 去重，不能让相同分支在 RRF 中获得双倍权重。空文本但带附件的当前消息不编造检索 Query；它仍可通过当前附件原生输入和已完成的 Conversation 私有索引参与上下文。最终启用的 Query 变体、授权快照和 Profile 一起进入输入指纹。

### 11.2 Dense、Sparse 与融合

`unicode_lexical_v1` 的文档端和查询端必须共用同一个实现：Unicode NFKC、大小写折叠；拉丁字母/数字按连续词元，汉字序列产生单字和双字词元；标点与空白只作为边界；权重固定为 `1 + ln(term_frequency)`；Sparse Index 取 `sha256("unicode-lexical-v1\\0" + token)` 前 4 字节的大端 `uint32`。哈希碰撞或重复词元产生相同 Index 时必须聚合权重，最终按 Index 升序输出唯一坐标，禁止把重复 Index 交给 Qdrant。Collection 开启 IDF Modifier。任何归一化、哈希、碰撞聚合或词频规则变化都必须创建新的 Sparse Encoder Version 和 Context Profile。

所有 Query 变体使用相同 Filter。官方嵌套 RRF 的 `ScoredPoint` 只返回最终 Score，不返回 Prefetch 分支分数；为满足审计，Adapter 使用一次 `QueryBatch` RPC，同时提交每个独立分支和一份带相同 Prefetch 的官方 RRF Query：

```text
current query Dense top-N
current query Sparse IDF top-N
optional contextual query Dense top-N
optional contextual query Sparse IDF top-N
Qdrant RRF fusion with the same active branches -> bounded authoritative candidate pool
```

最终候选集合和顺序只取官方 RRF Result。各独立分支 Result 只为最终候选补充审计，写入闭合 `retrieval_branches_v1` Metadata：`query_variant/modality/rank/normalized_score`；同一候选可以有多个 Dense/Sparse 分支记录，因此不在 Plan Item 放含义模糊的单一 Dense/Sparse Score 字段。RRF 候选若不出现在任何对应 top-N 分支中属于协议冲突并失败。该批量形状必须进入真实 Qdrant 能力和性能测试，不能假设 Prefetch 会回传中间分数。

`dense_min_score` 只作用于 Dense Prefetch，避免低相似向量污染 RRF；Sparse 分支不复用 Dense 阈值。配置 Reranker 时，`reranker_min_score` 作用于 Rerank 结果；未配置 Reranker 时不伪造跨查询可比的 RRF 分数阈值。候选上限由版本化服务端策略固定，不能由用户或单个 Agent 修改。Dense 与 Sparse 都无结果时为 `no_hit`；任何必需依赖调用失败为 `failed`。

MySQL 权威复核后，Document Chunk 按 `content_sha256` 去重，Conversation Turn 只按包含附件事实的 `source_sha256` 去重；不能因文字相同丢掉不同附件。只有同一文档版本中连续、定位相邻且合并后仍满足候选上限的 Chunk 才可合并；合并 Plan Item 的 Source Hash 对有序 `(chunk_id, chunk_facts_sha256)` 清单计算，Metadata 保存全部 Chunk ID/Locator，Content Snapshot 使用同一规范连接文本，不能制造不可追溯的新正文。

### 11.3 Reranker

Profile 未配置 Reranker 时，策略状态明确为 `disabled`，最终顺序使用 RRF 分数。Profile 配置了 Reranker 时：

- 只发送有界候选正文和当前 Query；
- 验证响应数量、Candidate ID、分数范围和模型快照；
- 使用 Rerank 分数作为最终相关性顺序；
- 超时、5xx、畸形响应或候选缺失都返回 `ai.context.rerank_failed`。

禁止在 Reranker 失败后静默改用 RRF，因为那会让同一策略在不同故障下生成不同 Plan。

候选数量和每段正文必须在 Rerank Adapter 声明的单请求文档数/Token 上限内；服务端策略在调用前确定性裁定有界池，不能临时截断正文。Adapter 不保证跨批分数可比时必须单批完成，否则该 Provider Model 不能用于此 Profile。

### 11.4 结果与引用

检索结果闭合为：

```text
skipped     没有启用 Context Profile，或没有启用 Space/ready Conversation 文档/可索引完整轮次
no_hit      有授权来源且管线成功，但没有通过复核和阈值的候选
hit         至少一个 Document Evidence 或 Recalled Turn 进入候选池
failed      依赖、权限快照、索引一致性或 Reranker 失败
```

`hit` 不保证所有候选最终进入 Provider 请求；Packer 可以因预算排除并记录原因。`C1`、`C2` 只在最终 Pack 完成后按选中 Evidence 顺序分配，未选中项没有 Citation Key。

Compiler 把每个已选 Document Evidence 放入带对应 `[C1]` Key 的不可信 Evidence Envelope，并要求模型只能引用给定 Key；Key 不是权限凭据。Assistant 是否真正引用由后端对最终持久化正文做闭合投影，不能因为模型输出了形似 Citation 的文本就信任任意来源 ID。

## 12. Token Budget 与 Context Packing

### 12.1 预算证明

预算以官方模型能力快照为输入：

```text
known_input_budget = context_window_tokens
                   - effective_output_tokens
                   - provider_protocol_upper_bound
                   - policy_safety_margin
```

`effective_output_tokens` 是 Provider 请求、报价和 Plan 共用的输出上界：取官方模型最大输出与服务端策略上限的较小值，并由 Compiler 写入协议支持的输出限制字段；协议无法表达该字段时按官方最大输出保留预算。不接受用户用 `max_history` 或其他参数扩大输入预算。每个 Block 的 Token 上界计算自身文本、Tool Schema、历史 Tool Call/Result 等实际 Payload；`provider_protocol_upper_bound` 只计算这些 Payload 之外的角色标记、JSON/Content Part 包装、Citation Envelope 包装和未来工具续调预留，禁止把同一内容计算两次。

Agent 启用工具时，`provider_protocol_upper_bound` 还必须包含版本化的 `tool_continuation_input_reserve`。服务端策略固定单轮最大 Tool Call 数、Call ID/Name/Arguments 总上界和所有 Tool Result 的规范序列化总上界，执行工具前与第二次 Provider Dispatch 前都验证完整 Call/Result 原子组；超过上界返回 `ai.context.tool_continuation_overflow` 并走现有 finalizer，禁止截断工具参数/结果或挤掉已经固定的 Plan Block。没有可证明上界的工具不能进入 Context Engine 的 Agent Tool Snapshot。

优先使用 Provider 官方 Tokenizer。没有官方 Tokenizer 时使用按 Provider/Model 注册的保守上界算法，并把 `budget_proof` 标记为 `conservative`；没有已注册算法则模型不可用于 Context Engine，不能用字符数拍脑袋兜底。

Plan 保存每个 Block 的上界和总上界。上游 Provider usage 只用于事后审计和计费，不能反向改写已完成 Plan。

原生文件和部分图片的上游解析 token 无法在本地证明。此时 Plan `budget_proof=opaque_attachment`：

- 当前附件仍是必需项；
- 所有可控文本继续遵守严格上界；
- 沿用已批准的 `native_file_context_window_v1` 资金上界；
- 上游因文件解析后超窗而拒绝时明确失败，不声称本地得到不存在的精确 token 数。

### 12.2 Pack 顺序

按稳定优先级和原子组执行确定性贪心，不做不可解释的动态百分比分配：

1. 系统指令、当前消息、当前附件、工具协议和输出预留；
2. 最近一个完整对话轮次及其附件；
3. Space 文档和 Conversation 附件的检索证据；
4. 最新有效滚动记忆；
5. 从 MySQL 直接分页得到的其他近期完整轮次，按新到旧；
6. Conversation Turn 私有索引召回的更早完整轮次；
7. 相邻 Chunk 和其他可选块。

同优先级使用检索分数、时间/序号和稳定 ID 排序。在权威输入快照与 Embedding/Rerank adapter 响应都相同的前提下，必须生成相同顺序和 `plan_sha256`；外部模型在相同文本上返回不同向量或分数时，Plan 会如实产生不同 Hash，不能谎称重新调用外部模型可以位级复现。Provider 重试不重新检索正是为了固定本 Run 的结果。

### 12.3 排除规则

排除原因使用闭合枚举：

```text
budget_exceeded
duplicate_content
below_relevance_threshold
superseded_memory
inactive_source
permission_changed
unsupported_attachment
```

必需块已超过可证明预算时返回 `ai.context.required_overflow`，不能截断当前消息或附件。

## 13. 会话记忆与历史附件

### 13.1 自动历史

新 Planner 不读取 `max_history`。它按完整轮次倒序分页读取消息，直到候选 Token 上界已覆盖本次 Budget 或遇到有效 Memory 覆盖边界。最近一轮和其余直接读取轮次使用上节不同优先级，但每轮始终保持原子。数据库查询不固定 N 条，也不一次加载无限会话。

旧客户端继续提交 `max_history` 时 JSON 绑定不报错，但字段不会进入有效 runtime params；新 OpenAPI 和前端不再发布该字段。

### 13.2 Conversation 私有索引

完成的历史对话按“用户消息 + 该轮附件事实 + 已持久化 Tool Call/Result 原子组 + 配对助手消息及 delivery state”形成原子轮次，以 User Message ID 为锚点、以完整轮次哈希写入 Qdrant 派生索引；正文、工具事实和配对关系仍来自 `ai_messages`、`ai_tool_calls`、`ai_reply_commands` 与 `ai_runs`。直接历史、Conversation Retrieval 和 Memory 必须复用同一个 `ConversationTurn` 构造器与 Source Hash。召回时返回整个轮次并生成同一 Atomic Group 下的 `recalled_turn/tool_call/tool_result` Block，不能只召回用户问题、工具结果或助手回答的一部分。

Agent 已有 `context_profile_id` 且助手消息进入 `completed` 或 `stopped` 可见终态后，投递幂等的 `ai:context-conversation-index:v1` 任务；Profile 为空或无助手消息的失败/取消 Run 不生成轮次 Point。任务键包含 Profile、Conversation、User Message ID 和完整轮次哈希。周期 Reconciler 以稳定 ID 游标分批扫描所有符合条件的终态 Run，计算预期 Point ID 并补投缺失任务；Agent 初次选择 Profile 时主动触发同一有界扫描，因此 API 事务提交后崩溃和旧会话回填都不会永久丢失索引。游标是可丢失的派生进度，周期全量重扫保证正确性。

Conversation Index Worker 在 Upsert 前重新验证 Agent Profile、完整轮次配对和 Source Hash。历史在任务排队后发生变化时，旧任务不写 Point，而是补投新 Hash；已知旧 Point 按确定性 ID 清理。进程在提交与入队之间崩溃仍由上述全量 Reconciler 收敛，Qdrant 中暂存的旧 Point 也会在查询时被 MySQL Hash 复核排除。

历史 TXT、Markdown、PDF、DOCX、CSV、XLSX 附件复用 Document/Version/Chunk 摄取链，Document 只保存 `conversation_id` 归属，User/Agent 从权威 Conversation 派生。写入 Qdrant 的 User 过滤值也来自该关系，后续检索仍必须同时过滤并复核 Conversation 和 User，不能被管理员共享 Space 或其他会话读取。

用户消息事务提交后，若 Agent 已有 Context Profile，调用幂等的 `EnsureConversationDocuments(message_id)`：按附件原始顺序为每个支持文件创建或复用 Conversation Document，并为新的对象事实创建 `queued` Version，再使用统一的 document-index 任务处理。这个后置摄取失败不回滚已经成功接收的聊天消息，因为当前附件仍走现有原生 Provider 路径；Version 已创建时失败写入其状态，创建事务本身失败时记录安全指标/日志并在 Run 诊断显示 `attachment_ingestion_pending`。周期 Reconciler 分批查找“有受支持附件但缺少对应 Document/Version”的消息并补建，覆盖事务提交后进程崩溃的窗口。

当前消息中的原生附件继续按既有 Provider 协议发送。最近完整轮次的附件保持原子；附件进入较老历史前，私有索引任务应已完成。若相关附件既无法原生重放又没有 ready 私有版本，Planner 返回明确的 `ai.context.attachment_unavailable`，不静默删除。

图片只保留在最近原生轮次中；OCR 和图片长期记忆不属于首期。

### 13.3 Rolling Memory

每次聊天产生 `completed` 或 `stopped` 可见助手消息并完成现有 finalizer 后，若“当前 ready Memory 未覆盖的完整历史”超过该 Agent Chat Model 能力快照中已知输入预算的 25%，异步压缩最老的完整轮次，使未覆盖部分回落到 12.5% 以内。Memory Model 自己的窗口只约束单次摘要任务，不能被误用成聊天上下文预算。

每个 Memory 任务只消费 Memory Model 已证明预算内的连续完整轮次前缀，并为摘要输出保留官方上限；积压过大时串行生成多级父链任务，不能一次加载无限历史或截断半个轮次。Memory Model 没有可信窗口、输出上界和 Tokenizer/保守算法时，Profile 不能启用 Memory。

Memory Model 来自 Context Profile：

- 未配置时，策略明确为 `conversation_retrieval_only`，不偷偷调用聊天模型；
- 已配置时，Profile ID/Hash 固定其 Provider Model 身份，后台任务另存 Source Hash、Usage 和 Provider Request ID；
- 首期 Memory 调用属于平台运营成本，不扣用户钱包；
- 任务失败不覆盖上一份 ready Memory；
- 当原始消息、附件、工具事实或 Assistant Delivery State 变化导致 Source Hash 不一致时，旧 Memory 自动失效。

Memory 是异步增强，不是同步 Provider Dispatch 的依赖。配置了 Memory Model 但后台任务失败时，失败事实留在 `ai_conversation_memories`；Planner 使用仍有效的旧 Memory，若不存在则按已定义的完整轮次与 Conversation Retrieval 规则装配，不把错误伪装成一条成功 Memory。`ai.context.memory_unavailable` 只用于后台任务错误，不把当前聊天 Run 标为失败。

摘要提示固定要求区分“用户声明”“助手回答”“已确认事实”“未解决事项”和“附件引用”，禁止把助手猜测提升为用户事实。

## 14. Chat、Gateway 与 Run 集成

### 14.1 调用位置

现有 `KnowledgeRuntime.RetrieveForRun` 替换为：

```go
BuildPlan(ctx context.Context, input BuildPlanInput) (ContextPlan, error)
```

调用发生在：

```text
用户消息已落库
Run 已有权威 ID
Provider/Agent/Model 快照已解析
Provider 请求尚未组装和报价
```

网络检索期间不持有数据库事务。写入 Plan 前的短事务锁定 Run/Reply Command，重新验证取消状态、当前消息与附件 Hash、Agent/Provider/Model/Profile 身份、Binding/Tool 授权以及入选来源仍存在且未禁用/删除，再原子写 Plan 和全部 Item。已选不可变 Version 在此期间被更新版本取代不构成冲突；它只要仍是同一授权文档的 `ready` 历史事实即可，后续 Plan Snapshot 负责审计。真正的身份、权限或内容 Hash 变化写 `ai.context.snapshot_conflict` 失败 Plan，不能拿旧快照继续组装请求。

### 14.2 不再污染 User Message

Document Evidence 作为单独的内部 Context Block 进入 Provider Compiler。对于只支持 system/user/assistant 的协议，Compiler 可把 Agent System Prompt 与 Evidence Envelope 规范合并为 system 内容，但原始用户正文保持不变。

Evidence Envelope 明确标记检索内容是不可信数据：文档中的命令、Prompt 或工具说明不得提升权限，也不能覆盖系统指令。

### 14.3 当前 Run 的工具续调

Context Plan 只冻结第一次 Provider 调用的基础上下文、工具定义以及 BuildPlan 时已经完成的历史 Tool Call/Result 原子组。当前 Run 在第一次 Provider Attempt 后产生的 Tool Call/Result 不存在于 BuildPlan 时，不能回写或重算 Plan。

当前 Run 的 Tool Call、Tool Result 和 Responses Continuation 作为 Attempt 自己的追加协议事实：完整原子组进入第二个 Prepared Request、Quote 和 `prepared_request_sha256`，该 Attempt 仍引用同一个 Plan ID/Hash。Dispatch 前验证它没有超过 Plan 预留的 continuation 上界；恢复只重放这份已准备请求。这样一份 Plan/Run 与多次 Provider Attempt 不冲突，也没有“为工具结果重新检索”的特殊分支。

### 14.4 三层身份不可混用

现有 `ai_reply_commands.request_fingerprint` 在用户动作接收事务中生成，继续只绑定原消息、当前附件、Agent/Model 和生成参数。它负责 Request ID 幂等，不能因为 Worker 稍后构建 Context Plan 而被改写。

Context Engine 单独使用：

```text
input_fingerprint_sha256       接收事实 + 历史/绑定/工具/Profile 快照
plan_sha256                    最终预算、Block、证据、选择与排除决定
prepared_request_sha256        Provider Compiler 生成的精确请求或清单
```

Provider Attempt 同时持久化 Plan ID/Hash 和 Prepared Request Hash，三者形成证据链。恢复时任一关系不一致都返回 `ai.context.plan_conflict`，不能覆盖旧值或重新检索；现有 Request ID 冲突仍使用原 `ai.billing.request_fingerprint_conflict` 语义。

### 14.5 Dispatch 前权限 Guard

Prepared Request 恢复仍发送同一字节，但 Dispatch 前验证：

- 每个 Space Evidence 对应的 Agent Binding 仍启用；
- 相关 Space/Document 未禁用或删除；
- Conversation 私有来源仍属于同一 User/Conversation；
- 当前消息、历史原子轮次、附件对象事实和 Conversation 私有来源仍存在且 Source Hash 与 Plan 一致；
- Prepared Request 中的 Tool Definition 仍有 Agent Binding/Authorizer 授权，Memory Item 仍为同 Profile 的有效 `ready` 父链。

文档后来产生新活动版本不改变已准备 Run 的历史事实；显式撤权或删除必须阻止尚未派发的旧请求。

### 14.6 终态

Context 构建失败发生在 Provider Dispatch 前：

- 未付费路径调用现有 Fail/Cancel/Timeout；
- 付费路径调用同一个 finalizer 释放冻结；
- 不创建伪 Assistant Message；
- 不留下 `running`；
- 不把 Context 错误转成空上下文继续生成。

用户取消或 Run 超时若发生在 Plan 提交前，不伪造 `failed` Plan；直接让现有 Run 状态机进入 `canceled` 或 `timeout`。真正的 Context 故障持久化 `failed` Plan 后，同一 Run 不再重新构建；用户显式重试会创建新的 Request ID 和 Run。

## 15. 错误码与重试

稳定错误码：

```text
ai.context.profile_unavailable
ai.context.document_parse_failed
ai.context.embedding_failed
ai.context.index_failed
ai.context.index_inconsistent
ai.context.snapshot_conflict
ai.context.permission_denied
ai.context.retrieval_failed
ai.context.rerank_failed
ai.context.required_overflow
ai.context.tool_continuation_overflow
ai.context.attachment_unavailable
ai.context.memory_unavailable
ai.context.plan_conflict
```

异步任务的永久错误（格式非法、扫描 PDF、维度错误、权限错误、资源超限）返回 `asynq.SkipRetry`。网络超时、临时 5xx 和 Qdrant 短暂不可用按注册的固定任务策略有限重试。同步 BuildPlan 只执行 adapter 的有界网络重试；仍失败就写 `failed` Plan 并收口 Run，同一 Run 不由 Reply Worker 重新检索。

任务类型必须版本化：

```text
ai:context-document-index:v1
ai:context-index-cleanup:v1
ai:context-conversation-index:v1
ai:context-memory-build:v1
ai:context-profile-rebuild:v1
```

`index-cleanup` 使用闭合 `cleanup_kind=document_version_points|conversation_points|retired_collection` DTO，分别携带唯一来源身份；Handler 在删除前统一复核 MySQL 可见性、Active/Target/Alias Generation 和退役宽限期，不为三个清理目标复制三套任务状态机。

幂等键闭合定义为：

```text
document index:      task version + version_id + profile_id + source_facts_sha256 + parser_version + chunker_version
index cleanup:       task version + cleanup_kind + profile_id + source identity + generation fence
conversation index:  task version + profile_id + conversation_id + user_message_id + source_sha256
memory build:        task version + profile_id + profile_sha256 + conversation_id + previous_memory_id + from_message_id + through_message_id + source_sha256 + policy_version
profile rebuild:     task version + profile_id + profile_sha256 + target_generation
```

## 16. Admin API 与权限

### 16.1 API

新路由族：

```text
/api/admin/v1/ai/context-profiles
/api/admin/v1/ai/context-spaces
/api/admin/v1/ai/context-spaces/{id}/documents
/api/admin/v1/ai/context-documents/{id}
/api/admin/v1/ai/context-documents/{id}/versions
/api/admin/v1/ai/context-documents/{id}/reindex
/api/admin/v1/ai/context-evaluations
/api/admin/v1/ai/agents/{id}/context-profile
/api/admin/v1/ai/agents/{id}/context-spaces
```

Run Detail 返回结构化 `context_plan`；Conversation/Message 响应为 Assistant Message 返回精简 Citation DTO。前端不能解析 Markdown 文本猜测文档身份。

Citation 不增加第十张表。后端使用 Assistant Message 的 `reply_command_id`/Run 关系定位唯一 Plan，以固定语法提取回答正文中的 `[C<number>]`，只把能映射到该 Plan 已选 `document_evidence` Item 的 Key 标记为已引用；Plan 已选但正文未出现的 Item 单独返回，未知 Key 只作为无效 Key 返回且没有来源 DTO。`completed` 和 `stopped` 消息使用同一投影，因此刷新后结果完全来自已持久化消息与 Plan，不依赖前端流式内存。

`context-evaluations` 复用同一检索与 Packer 管线并同步返回结果，但不创建 Run、Plan 或第十张评测表；管理操作日志只保存 Profile/Space ID、数量、耗时和安全结果码。需要长期比较的固定评测集进入仓库测试数据，由 CI/离线命令产出报告文件。

旧 `/api/admin/v1/ai/knowledge-*` 和 `/ai/agents/{id}/knowledge-bases` 在最终切换时删除，不保留 alias。

### 16.2 权限

```text
ai_context_view
ai_context_manage
ai_context_document_manage
ai_context_profile_manage
ai_context_evaluate
```

不为每个按钮创建重复权限。Run 中的 Context Plan 继续受现有 Run 查看权限约束；Conversation Citation 继续受会话所有权约束。

### 16.3 契约

所有请求/响应为闭合 DTO，进入 runtime route contract 和 Admin Contract Bundle。`locator_json`、`metrics_json` 和 `metadata_json` 在 Go 中使用版本化结构体，不暴露无约束 `map[string]any`。

## 17. 前端信息架构

菜单名称固定为“上下文工程”，路由 `/ai/context`，菜单码 `ai_context`。旧 `/ai/knowledge` View 和全部旧组件删除。

页面使用紧凑工作台，不使用旧卡片堆叠：

1. `上下文空间`：表格管理 Space、状态和 Agent 绑定；
2. `文档`：批量上传、处理阶段、活动版本、失败原因和版本历史；
3. `索引配置`：Embedding、维度、距离、Sparse Encoder 和可选 Reranker；
4. `检索评测`：查询、Dense/Sparse/Rerank 分数、最终证据和 Pack 结果。

Agent 页面显式选择一个 Context Profile，再选择同 Profile 的零个或多个 Context Space；Profile 为空表示纯聊天，选择 Profile 但不绑定 Space 仍可启用会话私有索引和滚动记忆。页面不暴露原始隐藏 ID，也不出现 `top_k`、分数阈值、字符预算或 Chunk Size。

Chat：

- 删除“携带历史消息”；
- `[C1]` 等合法引用可打开来源抽屉；
- 抽屉区分“正文实际引用”和“Plan 选择但未引用”；
- 无效 Citation Key 不映射到任何来源；
- 刷新后通过 Message -> Run -> Plan 关系恢复相同引用。

Run Detail：

- 显示预算、输出预留和 `budget_proof`；
- 显示 Query Embedding、Qdrant/RRF、Rerank 和 Pack 耗时；
- 显示选择与排除项、分数、原因、定位和安全快照；
- 显示 `skipped/no_hit/hit/failed`，会话索引待回填作为独立诊断，不混用文案。

## 18. 可观测性与安全

### 18.1 Readiness

Qdrant 是上下文工程被实际引用后的必需依赖，不是所有纯聊天请求的伪依赖：

- `/health` 仍只表示进程存活；
- `/ready` 检查 Qdrant 连接、Server 版本、Collection Schema、Dense/Sparse/IDF/QueryBatch/RRF 能力，并验证所有被运行时来源引用的 MySQL Active Physical Collection 以及 Alias 指向一致；只要存在启用的活动 Space Document、ready Conversation Document 或可索引完整轮次，失败就是阻塞。完全没有待检索来源时只报告 degraded 组件，不把纯聊天 API 判死；未被引用的失败 Profile 只在管理页告警。Readiness 请求本身不写业务 Collection；
- `admin-worker` Readiness 验证 Redis、MySQL、Qdrant 和任务注册。

### 18.2 指标

Prometheus 指标至少覆盖：

```text
context_plan_total{outcome}
context_plan_duration_seconds{stage}
context_document_versions_total{state,parser}
context_conversation_index_backlog{profile_state}
context_index_duration_seconds{stage}
context_retrieval_candidates{stage}
context_budget_upper_bound{block_kind,decision}
```

标签禁止出现 Query、文档名、文件名、User ID、Conversation ID、内容哈希或 Provider Request ID。

### 18.3 日志

日志只记录：

```text
request_id
run_id
plan_id
document_version_id
task type
safe error code
stage
duration
```

不得记录文档正文、消息正文、附件字节/Base64、COS 签名 URL、API Key 或完整 Provider 响应。

### 18.4 Prompt Injection

检索文本始终作为不可信 Evidence Envelope。工具权限仍由 Agent Tool Binding 和服务端 Authorizer 决定；文档内容不能创建工具、扩大权限、改变 Provider 或读取其他 Space。

### 18.5 Qdrant 网络

本地 Qdrant 只绑定 `127.0.0.1` 宿主端口和 `admin-platform` Docker 网络。生产不公开 6333/6334，启用 TLS/API Key，密钥来自 Secret，不写 Compose、日志或前端。

## 19. Docker 与配置

Qdrant 加入 `deploy/docker-state/docker-compose.yml`：

- 使用经真实能力测试的固定 `tag@sha256:digest`，禁止 `latest` 和仅固定 tag；
- 独立持久化 Volume；
- HTTP/gRPC Healthcheck；
- 容器内通过 `qdrant:6334` 访问；
- 可选宿主调试端口只绑定 `127.0.0.1`；
- 不改变 MySQL 和 Redis 现有端口。

`admin-go.env.example` 只增加非秘密占位配置：

```text
QDRANT_ADDR=qdrant:6334
QDRANT_COLLECTION_PREFIX=admin_context
QDRANT_TLS=false
QDRANT_API_KEY=
```

实际密钥只进入未跟踪的 `admin-go.env` 或生产 Secret。实施和验收不启动、停止或重启用户的 `admin-dev`。

## 20. 迁移与旧模块删除

### 20.1 Expand

1. 创建九张新表和必要索引；
2. 扩展 `ai_provider_models.model_kind`，已有行固定回填 `chat`，并把现有 Agent/Chat 查询收紧为 `model_kind=chat`；
3. 增加 Qdrant State Service 与配置；
4. 发布 Context Engine、摄取和检索代码，但旧 UI 尚不切换；
5. 不向旧表双写。

### 20.2 Cutover

前后端、契约、菜单和权限在同一实施批次切换：

- 先停止领取新的 Reply Command，由现有 Reconciler/finalizer 收口在途工作，并断言不存在可恢复的 `claimed/running/outcome_unknown` Reply Command 或 `prepared/dispatched/outcome_unknown` Chat Attempt；有残留就中止 Cutover，不能让新代码为 `context_plan_id=NULL` 的旧在途请求重新检索或猜上下文；
- 先枚举全部启用 Chat Agent，确认其模型可以解析可信的 `context_window_tokens`、最大输出、API Protocol 和 Tokenizer/保守上界策略；任何缺口使用 Agent/Provider/Model ID 报告并中止 Cutover，禁止写默认窗口继续上线；

- Chat 改用 `ContextRuntime`；
- Agent 契约改为显式可选 Context Profile，并只允许绑定同 Profile 的 Context Space；
- Run Detail 改读 Context Plan；
- Frontend 菜单改为“上下文工程”；
- OpenAPI 删除旧 Knowledge operation/schema；
- 删除 `max_history` 发布契约和 UI。

### 20.3 Contract

正式 Drop 前迁移必须分别断言旧六表为 0 行。任一表非空时使用 `SIGNAL SQLSTATE` 终止并报告表名/数量，禁止自动丢弃未知数据。

确认空后删除：

```text
ai_knowledge_retrieval_hits
ai_knowledge_retrievals
ai_agent_knowledge_bases
ai_knowledge_chunks
ai_knowledge_documents
ai_knowledge_bases
```

同步删除：

- `internal/module/ai/knowledge`；
- `KnowledgeRuntime` adapter 和 build wiring；
- 旧 admin routes/handlers/DTO/repositories/tests；
- `database/seeds/admin_permissions.sql` 中旧 Knowledge 菜单/按钮；
- 前端 `src/views/Main/ai/knowledge`、`src/api/ai/knowledge*`；
- Agent Knowledge Dialog、旧 Run Knowledge UI、i18n 和 CSS；
- 旧 OpenAPI operation/schema、生成 TypeScript 契约和专项测试；
- `max_history` capability、runtime DTO、前端控件和测试。

`database/legacy-migrations/20260510_ai_knowledge_rag.sql` 保持历史审计文件，不执行、不重写。新 DDL 进入 `database/migrations`，同步 `database/schema/admin.hcl`、`atlas.sum` 和 schema/关系验证。

破坏性 Contract 前沿用项目现有恢复制品与恢复验证门槛。本文授权删除空旧表，不授权绕过 Atlas、Schema Fingerprint 或恢复检查手工改库。

## 21. 实施切片

### Slice 1：核心契约与 typed request

- 新表、领域类型、Plan Repository；
- Provider Model Kind 与 Agent Context Profile 显式契约；
- `ChatInput` typed messages/content parts；
- Token Counter、Packer、Plan Hash；
- Attempt/Plan 证据绑定、工具续调预留与 Dispatch 授权 Guard；
- 纯聊天行为保持不变。

### Slice 2：文档摄取与 Qdrant

- Docker State、Go Client、Readiness；
- Profile/Space/Document Admin API；
- Parser、Chunker、Embedding 和版本激活 Worker；
- Qdrant 重建和一致性检查。

### Slice 3：检索、引用与聊天接入

- Dense/Sparse/RRF/Rerank；
- MySQL 权威复核；
- ContextRuntime 替换 KnowledgeRuntime；
- Citation、Run 详情、严格错误与 finalizer 回归。

### Slice 4：会话记忆与历史附件

- Conversation 私有 Document/Message 索引；
- Agent 初次选择 Profile 后的历史轮次与附件有界回填；
- 自动历史分页；
- Rolling Memory 与 Source Hash；
- 删除 `max_history` 的所有有效行为。

### Slice 5：管理 UI 与最终删除

- 上下文工程工作台、Agent 绑定和检索评测；
- Chat Citation 和 Run Plan UI；
- 契约重新生成；
- 旧代码、页面、权限、字段和六表 Contract。

每个 Slice 独立测试和提交。最终切换前允许新旧代码在分支中短暂共存，但没有运行时双写，也不允许半成品部署宣称完成。

## 22. 测试与评估

### 22.1 单元与属性测试

- Parser/Chunker Golden：TXT、Markdown、PDF、DOCX、CSV、XLSX；
- Chunk 结构、页/Sheet/行定位和稳定哈希；
- Sparse Encoder 文档/查询一致性、重复 Index 聚合和构造性哈希碰撞；
- Query 变体去重、RRF、阈值、内容去重、相邻合并和可选 Rerank；
- Packer 必需块、原子组、稳定顺序、工具续调预留和严格预算；
- 同一权威输入快照与同一 adapter 响应生成相同 Plan Hash；
- 不同 Chat Token Counter 会重新计算同一 Chunk/Memory 的 Plan Item 上界；
- 状态机禁止非法转换；
- Citation Key 只映射本 Plan 入选 Evidence。

属性测试必须证明：

```text
known selected upper bound <= known input budget
required blocks are never silently excluded
tool call and result are never split
atomic history turns are never split
selected order is deterministic
unauthorized source is never selected
```

### 22.2 集成测试

使用真实 MySQL、Redis/Asynq 和 Qdrant：

- 候选 Qdrant 镜像真实拉取后锁定 RepoDigest，并通过 Sparse IDF/QueryBatch/RRF/Filter 能力契约；
- 幂等版本任务和重复 Upsert；
- Worker 在 MySQL Chunk 写入后、Qdrant 写入后、激活前崩溃；
- 两个 Version 乱序完成时 ID 最大的 ready Version 保持活动，临时错误重试不提前写 failed；
- 新版本失败时旧版本继续召回；
- Alias 已切换/MySQL 未提交和 MySQL 已提交/旧 Collection 尚未清理两个窗口仍按 MySQL Physical Generation 正确读取；
- 健康 Active Generation 重建失败继续服务旧代次，退役宽限期内的在途读取不会遇到 Collection 被删；
- Qdrant stale Point 被 MySQL 活动版本过滤；
- ready Document Point 缺失使 Profile 失败，Conversation Turn Point 缺失只触发幂等回填；
- Agent 只有 Context Profile、没有 Space Binding 时，Conversation 私有索引和 Memory 仍工作；
- 并发 BuildPlan 只有一个 Run Plan，Memory 父节点在外部调用期间变化时不会写出分叉链；
- Agent 撤权、Document 禁用和 Conversation 越权；
- Prepared Request 重试复用相同 Plan；
- Cutover 前非终态 Reply/Chat Attempt 断言能阻止无 Plan 的旧请求进入新 Worker；
- 当前 Run 第二次工具 Attempt 复用 Plan、原子保存 Call/Result，超过续调预留时在 Dispatch 前失败；
- Qdrant/Embedding/Rerank 超时和 5xx；
- Cancel、Timeout、Paid/Unpaid finalizer 和余额释放；
- 刷新后 Message、Citation 和 Run 状态一致。

外部 Embedding/Rerank 使用协议级假服务测试错误和响应边界；Qdrant 本身不得只用 mock 代替。

### 22.3 离线评测集

仓库保存不少于 60 个固定中文案例：

```text
20 lexical / exact fact
20 semantic paraphrase
10 multi-turn reference
5 expected no-hit
5 cross-space or cross-user denial
```

首期门槛：

- expected source Recall@10 >= 0.90；
- MRR@10 >= 0.75；
- no-hit false-positive rate <= 0.05；
- cross-scope leakage = 0；
- Citation -> Plan Item 映射有效率 = 1.00。

非确定性的 LLM Faithfulness Judge 只作诊断，不作为唯一发布门槛。

### 22.4 性能与资源

在 100,000 个 Chunk 的本地基线中记录：

- Qdrant 热查询 P95 目标小于 300ms，不含外部 Embedding/Rerank 网络；
- Packer 对 1,000 个有界候选 P95 目标小于 50ms；
- 单次请求数据库查询数固定，不随候选数量产生 N+1；
- Parser/Embedding 按批次处理，不把整个大文件和全部向量同时留在内存；
- 性能目标作为可重复基线，不写成受 CI 硬件抖动影响的时间单元测试。

### 22.5 前端验证

- API 契约和闭合 DTO；
- Space/Document/Profile/Agent Profile/评测状态；
- Citation 有效/无效/未引用来源；
- Run Plan 的 hit/no-hit/failed/预算展示；
- 删除 `max_history` 后纯文本、图片和文件聊天回归；
- TypeScript、Vitest、组件测试和生产构建。

按用户要求不使用 Playwright，由用户执行浏览器人工验收。

## 23. 验收清单

- [ ] 菜单和页面只显示“上下文工程”，不存在旧知识库入口。
- [ ] 旧六表、旧 Knowledge API、旧 Go/Vue 模块和旧权限已删除。
- [ ] 新数据库只有本文九张 Context 表，没有重复 Retrieval/Hit/Citation 表。
- [ ] Agent Context Profile 是显式可选配置；没有 Space Binding 时仍能启用私有历史，Profile 为空时纯聊天行为不变。
- [ ] Qdrant 删除后可以从 MySQL/对象存储重建。
- [ ] Runtime 按 MySQL Active Generation 读取 Physical Collection，Alias 切换窗口不会产生假的 no-hit，旧代次在在途读取结束前不删除。
- [ ] 文档新版本失败不会影响旧活动版本。
- [ ] TXT、Markdown、文本 PDF、DOCX、CSV、XLSX 可以摄取并显示定位。
- [ ] Dense/Sparse/RRF 与显式可选 Reranker 进入检索审计。
- [ ] Qdrant 候选经过 MySQL 权威权限和活动版本复核。
- [ ] `max_history` 不再出现在 UI、OpenAPI 或有效运行参数中。
- [ ] 所有启用 Chat Agent 在 Cutover 前通过模型窗口、输出上界、协议和 Token 计数能力预检。
- [ ] 当前附件、最近完整轮次和工具调用不会被静默拆分。
- [ ] 历史支持文件有 Conversation 私有索引，不能跨用户/会话召回。
- [ ] Provider 请求固定后重试不重新检索。
- [ ] 检索失败不会继续生成，no-hit 不会被标为系统失败。
- [ ] Context 失败后 Run、Attempt、消息和钱包全部进入一致终态。
- [ ] Assistant Citation 可以恢复到同一 Plan Item 和文档定位。
- [ ] Run 详情能解释预算、选择、排除、耗时和失败阶段。
- [ ] 日志、指标、Qdrant Payload 不包含正文、附件字节、签名 URL 或密钥。
- [ ] 后端测试、集成测试、离线评测、前端测试、类型检查和正式构建通过。

## 24. 规范关系

本文显式扩展 `2026-07-30-ai-chat-native-file-attachments-design.md`：原生附件仍由上游读取，文件字节/Base64 仍不进入 MySQL；上下文工程新增的是受控的派生文本、版本和索引，不改变原文件请求清单与 usage 结算规则。

本文不改变 `2026-07-30-ai-chat-canceled-partial-delivery-design.md` 的停止展示、上游排空、权威 usage 和统一 finalizer 规则。

本文取代旧知识库相关文档、代码和契约中的以下假设：

- 固定字符窗口 Chunk；
- Go 内存 `strings.Contains` 检索；
- 每绑定独立 `top_k/min_score/max_context_chars`；
- 把知识文本拼进用户消息；
- 吞掉检索错误继续聊天；
- 用户手工设置 `max_history`。

实现与本文冲突时，以本文和运行时验证证据为准；旧文档必须同步修正或明确标为历史。
