# AI 上下文文档预览与模型用途治理设计

**日期：** 2026-08-08

**状态：** 方案 A 已确认，等待用户复核书面规格

**涉及仓库：** `admin_back_go`、`admin_front_ts`

**上游规格：**

- `docs/superpowers/specs/2026-08-01-ai-context-engineering-design.md`
- `docs/superpowers/specs/2026-08-05-ai-agent-optional-context-enhancement-design.md`

**覆盖关系：** 本文覆盖上游规格中“`ai_provider_models` 只有 `chat | embedding | rerank`”“`ai_agents` 只能绑定 Chat 模型”“远端模型同步固定按 Chat 协调”以及“Context Profile 由管理员手填 Embedding 规格”的规则。上下文降级、MySQL/COS/Qdrant 所有权、不可变 Context Plan、消息持久化、Run、Attempt、WebSocket 和计费主链不变。

## 1. 需求分析

### 【需求判断】

这是真问题，不是为了未来扩展而制造的抽象。

当前系统已经拥有官方模型目录、供应商模型、智能体场景和 Context Profile，但四者的语义没有闭合：

- 官方目录保存了模型 family、输入输出模态和价格，却没有明确的执行用途；
- 供应商远端同步把所有发现的模型强制写成 `chat`；
- `gpt-image-2` 实际调用图片生成接口，却被伪装成 Chat 模型；
- Embedding 的维度、输入上限和 Token Counter 被放在 Profile 中手填，导致同一个供应商模型可以被描述成多套互相冲突的规格；
- 上下文文档已经可以上传、解析和索引，但管理员无法查看原始文档内容。

### 【核心问题】

真正需要解决的是两类数据所有权错误：

1. 模型“是什么、通过哪类执行接口调用”属于模型事实，不能由 Agent Scene、文件输入能力或模型名称推测。
2. 原始文档属于 COS 和 MySQL 管理的业务事实，查看文件不应该依赖 Qdrant、Embedding 或解析任务。

### 【复杂度检查】

本设计不增加通用模型能力图、不增加模型多对多用途表、不探测模型接口、不引入 Office 转换服务，也不增加第二套文档存储。

闭合枚举和现有数据结构已经足够：

```text
官方模型目录      定义受审 canonical 模型事实和 model_kind
ai_provider_models 表示某个供应商可调用的具体模型路由
ai_agents          选择 Chat 或 Image 路由并声明业务场景
ai_context_profiles 选择 Embedding/Rerank/Memory 路由并保存索引快照
COS                保存原始文档
Qdrant             只保存可重建索引，不参与文档查看
```

### 【破坏性分析】

主要兼容风险是现有 `gpt-image-2` 行和图片 Agent 都按 `chat` 保存。迁移必须原地修改 Provider Model 的 `model_kind`，保留其主键和 Agent 的 `provider_model_id`；不得删除重建、重新编号或让历史任务失去引用。

现有 Context Profile 的 Embedding 三项字段必须保留为不可变索引快照。新 Profile 不再手填这些字段，但旧 Profile、旧空间、旧文档和旧 Qdrant Collection 不重写、不混用、不自动重建。

## 2. 最终结论

### 2.1 模型用途采用四类闭合枚举

```text
chat       对话模型
embedding  向量模型
rerank     重排模型
image      图片生成模型
```

`gpt-image-2` 的 `model_kind` 必须是 `image`。它可以接收文本和图片，但输出是图片，并通过图片生成接口执行；输入模态不等于执行用途。

`gpt-5.6-sol` 等模型即使支持图片和原生文件输入，输出仍是文本并通过对话协议执行，因此仍属于 `chat`。

### 2.2 官方目录是自动分类的唯一权威来源

自动分类只发生在供应商模型 ID 能被官方目录精确解析为 canonical ID 或受审别名时。系统不得根据以下信息猜测：

- 名称是否包含 `embed`、`image`、`rerank`；
- `owned_by` 字符串；
- 模型 family；
- 是否支持图片输入；
- 对未知接口发起试探请求。

未映射的供应商模型处于“用途待确认”的候选态，由管理员手动选择用途。候选态不是新的运行时模型类型，不能用 `unknown`、空字符串或默认 `chat` 塞进 `ai_provider_models`。

### 2.3 模型用途与业务场景分层

```text
Provider Model Kind  说明模型通过哪类执行契约调用
Agent Scene          说明这个 Agent 对外承担什么业务入口
```

二者有关联，但不能混为一个字段。`image_generate` 继续是 Agent Scene；`image` 是 Provider Model Kind。

### 2.4 文档查看使用 COS 短期签名 URL

文档版本点击后在当前上下文空间页面打开右侧详情抽屉。后端按版本 ID 授权并校验 COS 对象后签发最长 5 分钟的 GET URL。TXT、Markdown 和 PDF 在抽屉内查看；Office 等格式显示文件信息并提供打开或下载入口。

不引入 LibreOffice、OnlyOffice、在线 Office 预览服务或后台格式转换任务。

## 3. 核心不变量

1. `model_family` 表示模型产品家族，`model_kind` 表示执行用途，两者不能互相替代。
2. 输入模态只说明模型能接收什么，不决定执行用途。
3. 一个已持久化 Provider Model 必须有一个合法 `model_kind`；待确认候选不能进入运行表。
4. 官方映射成功时，Provider Model 的用途必须与官方目录完全一致。
5. 人工分类不能伪造官方映射、官方能力或官方价格。
6. Agent 只能绑定 `chat` 或 `image`；Embedding 和 Rerank 不创建 Agent。
7. Context Profile 只引用 `embedding`、`rerank` 和用于 Memory 的 `chat` 模型。
8. Profile 中的 Embedding 规格是索引快照；Provider Model 中的 Embedding 规格是当前模型事实。
9. 已被 Profile 引用的 Provider Model 规格不得原地修改。
10. 更换 Embedding 模型必须新建 Profile 和索引代次；旧索引继续按旧 Profile 工作。
11. 未配置或关闭 Context Profile 时，普通聊天、当前附件、WebSocket、消息持久化、Run 终态和计费不受影响。
12. 文档查看只依赖 MySQL 授权事实和 COS 原始对象，不依赖 Qdrant、Embedding、Rerank 或文档解析状态。
13. 临时 COS URL 不写数据库、不写日志、不进入审计负载，也不能作为长期文件身份。

## 4. 术语与数据层级

### 4.1 官方模型

官方模型是仓库内受版本控制、经过人工核对的 canonical 模型事实，不是供应商返回 `/models` 后自动信任的任意字符串。

官方目录继续拥有：

- canonical model ID 和受审别名；
- 厂商与 family；
- 生命周期；
- 输入输出模态；
- Chat/Image 能力和限制；
- 受审价格事实；
- 本次新增的 `model_kind`。

不新增 `is_official`。供应商模型是否已映射，继续由现有 `mapping_status`、`official_model_id`、`official_catalog_version` 和 `mapped_at` 共同表达。

### 4.2 供应商模型

`ai_provider_models` 表示一条实际调用路由：

```text
供应商 + 上游 model_id + model_kind
```

同一个硅基流动供应商可以拥有多个不同的 Embedding 模型，例如：

```text
Qwen/Qwen3-Embedding-0.6B   embedding
BAAI/bge-m3                 embedding
另一个对话模型              chat
```

Context Profile 选择具体 Provider Model ID，不只选择供应商。

### 4.3 智能体

`ai_agents` 保存面向业务的 Agent 配置：名称、提示词、头像、倍率、场景、主模型和可选 Context Profile。

智能体不是供应商模型，也不负责描述 Embedding 维度。Embedding/Rerank 是 Context Profile 的依赖，不增加 `embedding` Agent Scene。

## 5. 官方目录模型用途

### 5.1 目录结构

官方目录每个模型增加必填字段：

```json
{
  "model_id": "gpt-image-2",
  "model_family": "image",
  "model_kind": "image",
  "input_modalities": ["text", "image"],
  "output_modalities": ["image"]
}
```

`officialmodel.Model`、目录 JSON loader、Admin DTO 和生成 OpenAPI 均公开同一闭合枚举。`officialmodel` 是枚举定义的唯一所有者；Provider 包只通过 Go type alias 保留现有调用名称，不维护第二份会漂移的枚举定义。

### 5.2 目录校验

目录加载时执行 kind 与能力的一致性检查：

| `model_kind` | 必需事实 | 禁止事实 |
| --- | --- | --- |
| `chat` | 文本输入、文本输出 | Embedding 规格 |
| `image` | 文本输入、图片输出 | Embedding 规格、伪装成 Chat 输出 |
| `embedding` | 文本输入、完整 Embedding 规格 | Chat/Image 执行能力 |
| `rerank` | 文本输入、重排执行契约 | Embedding 规格 |

目录中任一模型缺少用途、用途非法或与能力矛盾时，目录加载失败。不能在运行时回退为 Chat。

首期给当前所有受审官方目录项补齐用途：GPT/Claude 文本输出模型为 `chat`，`gpt-image-2` 为 `image`。本次不凭名称把未受审的第三方 Embedding/Rerank 模型写入官方目录；它们继续走人工用途确认。以后新增官方 Embedding 条目时，必须同时提供经过核对的 Embedding 规格和来源。

### 5.3 官方身份与供应商路由映射

匹配顺序保持严格：

```text
上游 model_id
  -> canonical ID 精确匹配
  -> 受审 alias 精确匹配
  -> 未映射
```

匹配大小写敏感，不做模糊搜索。映射成功后同时得到：

- `official_model_id`；
- `official_catalog_version`；
- `model_kind`；
- 官方能力和价格事实。

如果一个已存 Provider Model 的人工用途与新识别到的官方用途冲突，普通同步返回稳定冲突错误，不静默改类型。`gpt-image-2` 的既有错误数据只由第 11 节的一次性兼容迁移修正。

## 6. Provider Model 数据设计

### 6.1 保留字段

保留现有模型身份和官方映射字段：

```text
id
provider_id
model_id
model_kind
display_name
official_model_id
official_catalog_version
mapping_status
mapped_at
status
created_at
updated_at
```

不增加重复的 `is_official`、`model_category`、`scene` 或能力 JSON。

### 6.2 新增 Embedding 规格

`ai_provider_models` 增加三个可空字段：

```text
embedding_dimensions          INT UNSIGNED NULL
embedding_max_input_tokens    BIGINT UNSIGNED NULL
embedding_token_counter_id    VARCHAR(64) NULL
```

闭合规则：

- 启用的 `embedding` 模型三项必须全部存在且为合法正数/已注册 Counter；
- 禁用的历史 Embedding 行可以暂时缺规格，但不能被 Profile 选择或被运行时调用；
- 非 `embedding` 模型三项必须全部为 `NULL`；
- 官方映射的 Embedding 模型从官方目录复制规格，后台只读；
- 未映射的 Embedding 模型由管理员填写，启用前严格校验。

这三项描述模型输出和输入契约，不属于某个空间或某次索引策略。

### 6.3 Provider Model 身份

继续保留现有唯一身份：

```text
(provider_id, model_id, model_kind)
```

不把同名不同执行端点强行合并。Agent 和 Profile 都以 `provider_model_id` 引用具体路由。

新的结构化模型写入项应完整携带：

```json
{
  "model_id": "vendor-embedding-v1",
  "model_kind": "embedding",
  "display_name": "供应商向量模型 V1",
  "status": 1,
  "embedding_dimensions": 1024,
  "embedding_max_input_tokens": 8192,
  "embedding_token_counter_id": "utf8_bytes_v1"
}
```

这里的数值只是请求形状示例，不是给任意模型套用的默认值。服务端不得在字段缺失时猜测 1024、8192 或任意 Counter。

新结构化 `models` 以 `(model_id, model_kind)` 去重；相同 model ID 的不同用途可以独立存在。显示名称、状态和 Embedding 规格都放在各自模型项内，不能继续用仅以 model ID 为键的 Map 表达新数据。

旧 `model_ids`、`model_display_names` 和 `statuses` 输入保留为兼容入口，只能表达 Chat 模型；新管理端统一提交完整 `models`。旧形状与新形状不能同时提交。

### 6.4 引用保护

Provider Model 已被以下事实引用时，不允许修改 `model_kind` 或 Embedding 三项规格：

- `ai_agents.provider_model_id`；
- `ai_context_profiles.embedding_provider_model_id`；
- `ai_context_profiles.reranker_provider_model_id`；
- `ai_context_profiles.memory_provider_model_id`。

请求返回 HTTP 409 和稳定错误码。管理员应新增或启用另一条模型路由，再创建新 Profile 或重新绑定 Agent；系统不原地改变历史索引的向量语义。

第 11 节把既有 `gpt-image-2` 从错误的 `chat` 修正为 `image` 是唯一的一次性迁移例外；该操作由迁移守卫证明引用关系安全，不能开放成日常管理 API。

## 7. 模型发现、自动分类与同步

### 7.1 候选态与运行态分离

远端 `/models` 返回的是候选模型，不是可信运行配置。候选 DTO 使用：

```text
model_id
display_name
owned_by
mapping_status              mapped | unmapped
official_model_id           映射成功时存在
official_catalog_version    映射成功时存在
model_kind                  映射成功时存在，未映射时为空
```

候选 DTO 中的空 `model_kind` 是“尚未持久化、等待管理员选择”，不是数据库合法值。

### 7.2 拉取模型

供应商新增/编辑页面拉取候选后：

- 官方已映射：自动显示用途，类型控件只读；
- 未映射：显示“用途待确认”，管理员必须选择类型；
- 选择 `embedding`：展开三项必填规格；
- 已有同一 `(model_id, model_kind)`：合并展示，不重复新增；
- 不得把所有新候选默认成 `chat`。

### 7.3 同步模型

同步采用非破坏性事务：

1. 拉取远端候选。
2. 逐项进行官方精确匹配。
3. 对官方已映射候选，按官方用途新增或更新映射元数据。
4. 对已存在的人工模型，保留人工用途、规格和启停状态。
5. 对未映射且从未配置的候选，只返回给前端确认，不写运行表。
6. 远端暂时没有返回的既有模型不自动删除、不改类型、不停用。
7. 任一官方用途冲突使本次写事务失败，不留下半套同步结果。

这取代当前“同步返回什么就全部按 Chat 协调”的规则，也避免一次普通 Chat 同步破坏 Embedding Profile。

### 7.4 人工分类边界

人工选择用途只证明管理员希望通过某类 adapter 调用该路由，不会产生：

- 官方身份；
- 官方能力；
- 官方价格；
- 原生文件支持证明。

因此未映射 Chat/Image 模型仍不能进入要求官方能力与用户计费闭环的 Agent 选择器。未映射 Embedding/Rerank 模型可以在规格完整、adapter 支持且状态启用后供 Context Profile 使用，因为其成本仍按既有规则作为平台运营成本，不进入用户钱包。

## 8. Agent 与场景约束

### 8.1 场景映射

| Agent Scene | 允许的 Provider Model Kind |
| --- | --- |
| `chat` | `chat` |
| `agent_generate` | `chat` |
| `text_generate` | `chat` |
| `image_generate` | `image` |

一个 Agent 只绑定一个 Provider Model，因此 `image_generate` 与三个 Chat 场景互斥。服务端校验是权威；前端过滤只改善体验，不能代替后端约束。

### 8.2 Agent 选择器

Agent Page Init/供应商模型接口返回 Provider Model ID、kind、官方映射和有效能力。前端根据场景过滤：

- 普通对话、智能体生成、文本生成只显示 `chat`；
- 图片生成只显示 `image`；
- `embedding` 和 `rerank` 永不出现在 Agent 主模型列表。

切换场景后如果当前模型不再合法，前端清空选择并要求重新选择，不能自动挑第一个模型。打开已有非法数据时应显示明确错误，不静默修复。

### 8.3 `gpt-image-2`

`gpt-image-2` 继续由图片模块通过现有图片生成引擎和 `/images/generations` 执行，继续沿用现有任务、文件、Run 和计费闭环。

本次只纠正模型用途和选择链路，不借 `image` 分类宣称所有图片模型都已受支持。首期图片 Agent 仍只允许平台已经实现并受审的 `gpt-image-2`；以后接入其他图片模型时必须先完成参数、返回值、计费和文件落库适配。

## 9. Context Profile 与 Embedding 规格

### 9.1 字段所有权

新建 Context Profile 时，管理员只选择具体 Embedding Provider Model。服务端从该模型复制以下三项到 Profile：

```text
embedding_dimensions
embedding_max_input_tokens
embedding_token_counter_id
```

Profile 表单不再让管理员手填这三项。选中模型后只读显示，便于核对。

`dense_distance`、召回阈值、Sparse、Reranker、Memory 和索引策略继续属于 Profile。Embedding 模型事实不能替代检索策略。

### 9.2 为什么 Profile 仍保留快照

现有 Profile 字段不删除，因为它们共同决定：

- Qdrant Collection 的向量维度；
- 文档版本使用的 Embedding 输入上限；
- Chunk 和 Plan 的 Token 计算；
- 索引代次是否可以安全复用。

Provider Model 后续停用、供应商改名或目录升级，不能改变历史 Profile 已经建立的索引身份。

### 9.3 更换向量模型

更换 Embedding 模型的正确流程是：

```text
新增/启用另一个 embedding Provider Model
  -> 新建 Context Profile
  -> 建立新的 Qdrant Collection/索引代次
  -> 文档按新 Profile 重新摄取或重建
  -> Agent 显式切换 Profile
```

旧 Profile、旧空间、旧文档版本和旧索引不受影响。系统不把不同维度或不同模型生成的向量混入同一 Collection。

### 9.4 无 Embedding 的热插拔

Agent 的 `context_profile_id = NULL` 即关闭上下文增强。关闭后：

- 不调用 Embedding；
- 不查询 Qdrant；
- 不使用绑定空间检索；
- 普通聊天、当前图片/文件、近期完整对话、WebSocket、消息落库、Run 和计费继续工作。

已经存在的 Profile、空间和索引不删除；以后重新绑定即可恢复使用。

## 10. 上下文文档查看

### 10.1 交互

版本列表中的每一行都是可查看操作。点击后在页面右侧打开详情抽屉，不离开当前空间和文档选择状态。

抽屉头部显示：

- 文档标题与版本；
- 原始文件名；
- MIME；
- 文件大小；
- 摄取状态；
- 临时链接剩余有效时间；
- 打开/下载与刷新链接操作。

正文区域按 `preview_kind` 展示：

| 类型 | 展示方式 |
| --- | --- |
| `text` | 等宽文本阅读，保留换行，不执行 HTML |
| `markdown` | 复用现有 `MarkdownRenderer`，HTML 关闭并经过现有安全指令清洗 |
| `pdf` | 浏览器原生 PDF 内嵌预览 |
| `external` | 文件信息、打开和下载按钮，不伪造 Office 在线预览 |

TXT/Markdown 超过 2 MiB 时不在前端整文件渲染，返回 `external`，避免主线程和内存被大文件拖垮。PDF 由浏览器按需加载，不受该内联文本上限约束。

TXT/Markdown 的正文由浏览器使用签名 URL 发起无凭据 GET。COS Bucket 必须允许后台管理域名和本地开发域名执行 `GET/HEAD`，并暴露 `Content-Type`、`Content-Length` 和 `ETag`；未满足 CORS 时按“预览暂不可用”处理，不能改走无鉴权代理兜底。PDF iframe 设置 `referrerpolicy=no-referrer`，外部打开链接使用 `noopener noreferrer`，避免签名查询参数经 Referer 泄漏。

### 10.2 Admin API

新增：

```text
GET /api/admin/v1/ai/context-document-versions/:id/preview
```

请求只携带版本 ID。响应：

```json
{
  "url": "https://cos.example/...signed...",
  "expires_in": 300,
  "filename": "系统架构说明.md",
  "mime_type": "text/markdown",
  "size_bytes": 128000,
  "preview_kind": "markdown"
}
```

响应使用 `Cache-Control: no-store`。前端不把 URL 写入 localStorage、路由、全局 Store 或业务日志；抽屉关闭后丢弃。

### 10.3 授权与对象完整性

后端按版本 ID 一次加载并验证：

1. 版本、文档和空间存在；
2. 平台固定为 `admin`；
3. 文档和空间未删除；
4. 当前管理员拥有 `ai_context_view`；
5. 存储类型为 COS，object key 属于受信上下文文档前缀；
6. COS HEAD 返回的 ETag、大小和 MIME 与 MySQL 版本事实一致；
7. 签名 TTL 不超过 300 秒。

文件查看不要求文档版本处于 `ready`。只要源对象仍然完整，`queued`、`processing` 或 `failed` 版本也可查看原始文件，从而帮助管理员诊断摄取失败。

Qdrant 不参与此接口，也不能作为权限判断来源。

### 10.4 错误语义

| 条件 | HTTP | 稳定错误码 |
| --- | --- | --- |
| 非法版本 ID | 400 | `ai.context.document_version.invalid_id` |
| 版本/文档/空间不存在 | 404 | `ai.context.document_version.not_found` |
| COS 对象不存在或 ETag/大小/MIME 改变 | 409 | `ai.context.document_version.source_changed` |
| COS 配置、网络或签名暂时失败 | 503 | `ai.context.document_version.preview_unavailable` |

403 继续由现有 RBAC 中间件处理。前端使用现有全局错误展示；抽屉保留当前文件信息和“重试”，不能吞错或显示空白正文。

签名过期后，前端最多自动重新请求一次新的 preview；再次失败交给用户点击重试，禁止无限刷新。

## 11. 数据库迁移与兼容

### 11.1 扩展阶段

1. 扩展 `model_kind` CHECK，加入 `image`。
2. 给 `ai_provider_models` 增加三个 Embedding 规格列。
3. 增加 Embedding 规格形状 CHECK：非 Embedding 必须全空；启用的 Embedding 必须三项完整。
4. 发布能同时读取既有 `chat + gpt-image-2` 和新 `image + gpt-image-2` 的短期兼容代码。

### 11.2 数据迁移

迁移前守卫检查：

- 同一供应商不存在同时启用的 `chat/gpt-image-2` 与 `image/gpt-image-2` 冲突行；
- 引用 `gpt-image-2` 的 Agent 场景只能是 `image_generate`；
- 现有 Embedding Provider Model 关联的 Profile 快照不存在互相冲突的三项规格。

守卫失败时迁移明确失败并输出冲突 ID，不删除数据、不任选赢家。

守卫通过后：

1. 原地把 `gpt-image-2` Provider Model 的 `model_kind` 从 `chat` 改为 `image`；
2. 保留 Provider Model 主键，因此 `ai_agents.provider_model_id` 不变；
3. 从引用该模型的既有 Profile 中回填一致的 Embedding 规格；
4. 未被 Profile 引用且规格未知的 Embedding 行改为禁用，保留数据，等待管理员补齐后重新启用；
5. 不修改任何 Context Profile 快照、空间、文档版本、Chunk 或 Qdrant Collection。

### 11.3 收缩阶段

数据守卫确认不存在 `chat + gpt-image-2` 后，删除临时兼容分支。最终运行时只接受正确的 `image` 类型，不永久保留模型 ID 特判。

### 11.4 API 兼容

- 现有响应字段只增不删；新增 `model_kind`/Embedding 规格后重新生成 Admin OpenAPI 和前端类型。
- 旧 `model_ids` 输入继续只表示 Chat，保持旧调用者可用。
- 旧 `model_display_names` / `statuses` Map 只服务旧 Chat 形状；新结构化 `models` 内联显示名称、状态和规格，避免相同 model ID 不同用途互相覆盖。
- 旧 Profile 创建请求中的 Embedding 三项在一个兼容周期内允许继续提交，但必须与选中 Provider Model 规格完全一致；不一致返回 409，绝不静默忽略。
- 新前端不再提交这三项，由后端复制。
- 历史 Agent、Run、Attempt、Task 和账单快照结构不改变。

## 12. 后端组件边界

### 12.1 `officialmodel`

负责：

- canonical identity；
- 受审 alias；
- `model_kind`；
- 官方能力、限制和价格；
- 目录严格校验。

不负责供应商密钥、Agent Scene 或 Context Profile。

### 12.2 `provider`

负责：

- 供应商实际模型路由；
- 官方映射；
- 人工用途配置；
- Embedding 路由规格；
- 非破坏性模型发现和同步。

不根据名称推测用途，不决定 Agent 业务场景。

### 12.3 `agent` 与 `image`

`agent` 负责校验 Scene 与 Provider Model Kind 的组合。`image` 只消费已经验证的 `image` Agent，并继续使用现有 ImageEngine、队列、任务状态机、COS 输出和 finalizer。

普通 Chat/Tool/Text 代码只能消费 `chat` 路由，不能接受 `image` 作为兼容兜底。

### 12.4 `contextengine`

负责按 Provider Model Kind 生成三个模型选项集合：

```text
embedding_model_options  -> embedding
reranker_model_options   -> rerank
memory_model_options     -> chat
```

创建 Profile 时从 Embedding Provider Model 复制规格；运行时继续使用 Profile 快照构造或验证索引。

### 12.5 文档预览

Context Admin Service 负责业务授权和版本事实；COS adapter 负责条件 HEAD 与签名。Transport 只做参数绑定和稳定错误映射。前端只消费生成的 Admin Operation，不手写 URL。

## 13. 前端体验

### 13.1 供应商页面

模型表格增加清晰的用途标签：对话、向量、重排、图片生成。

- 官方识别模型显示“官方已识别”，类型只读；
- 未识别候选显示“用途待确认”，类型必须手选；
- Embedding 规格只在选择“向量”后展示；
- 编辑已被引用的模型时，受保护字段只读并说明引用来源；
- 不给新模型默认 Chat；
- 继续使用现有 `AppTable` 和 Element Plus 控件，不增加大量 `:deep` 样式。

### 13.2 智能体页面

场景与模型联动，但不自动替用户选择：

- Chat 场景显示对话模型；
- 图片生成显示图片模型；
- 不合法组合在保存前就有中文提示；
- 后端仍执行同样校验。

### 13.3 Context Profile 页面

管理员新建 Profile 时主要操作是“选择向量模型”。维度、输入上限和 Counter 只读展示。已有 Profile 继续显示自己的快照值，并明确标识“索引快照”，避免管理员误以为改供应商模型会重写旧文档。

### 13.4 文档抽屉

采用已经确认的右侧详情抽屉。抽屉不是嵌套卡片，不给正文增加无意义灰色背景板；表格、标签、抽屉、滚动和加载状态尽量使用组件默认样式，只补必要布局。

## 14. 稳定错误码

本次至少新增：

```text
ai.provider.model_kind_invalid
ai.provider.model_kind_confirmation_required
ai.provider.model_kind_conflict
ai.provider.embedding_spec_invalid
ai.provider.model_in_use
ai.agent.model_scene_mismatch
ai.context.document_version.invalid_id
ai.context.document_version.not_found
ai.context.document_version.source_changed
ai.context.document_version.preview_unavailable
```

错误必须发生在写事务或上游调用前。不能把用途冲突改写成 Chat，不能把 Embedding 规格缺失改写成默认维度，也不能把 COS 源对象变化显示成空白预览。

## 15. 验证范围

### 15.1 自动短验证

实施阶段只运行预计两分钟内完成的定向检查：

1. 官方目录用途解析、非法用途和能力冲突测试；
2. Provider 候选分类、未知模型待确认、同步不覆盖人工模型测试；
3. `gpt-image-2` 迁移与主键/Agent 引用保持测试；
4. Agent Scene 与模型用途矩阵测试；
5. Profile 自动复制 Embedding 规格和引用保护测试；
6. 文档 Preview 授权、条件 HEAD、签名 TTL 和错误映射测试；
7. 前端 Provider、Agent、Profile、文档抽屉定向测试；
8. OpenAPI 生成契约一致性与 `git diff --check`。

不自动运行 `go test ./...`、前端全量测试、完整构建、Docker E2E 或 Playwright。长测试由用户在最终人工验收阶段决定。

### 15.2 用户人工验收

1. 拉取官方 GPT/Claude 模型，模型自动显示“对话”，无需手选。
2. 拉取 `gpt-image-2`，自动显示“图片生成”，不再出现在普通对话模型列表。
3. 新建图片 Agent，只能选择 `gpt-image-2`；图片生成、余额扣减、运行监控和结果图片保持正常。
4. 新建普通 Chat Agent，不能选择 `gpt-image-2`；普通消息、图片附件、文件附件、刷新历史和 Run 终态保持正常。
5. 拉取未进入官方目录的模型，界面显示“用途待确认”，保存前必须人工选择。
6. 把未知 Qwen 模型设为向量并填写完整规格；Context Profile 只需选模型，三项规格自动显示且不可改。
7. 现有 Profile、空间和文档仍能检索；关闭 Agent Context Profile 后普通聊天仍正常。
8. 新增另一向量模型并创建新 Profile；旧 Profile 和旧文档索引不改变。
9. 点击 TXT/Markdown/PDF 文档版本，在右侧抽屉正确查看。
10. 点击 DOCX/XLSX，看到正式文件信息和打开/下载入口，不出现伪造预览。
11. 修改或删除 COS 测试对象后再次预览，页面明确显示源文件变化或暂不可用，不显示空白内容。

## 16. 明确不做

- 不按模型名称、前缀或 `owned_by` 猜用途；
- 不增加 `unknown` 运行时模型类型；
- 不增加 `is_official` 重复字段；
- 不让一个 Agent 同时承担 Chat 和 Image 场景；
- 不把 Embedding/Rerank 建成 Agent；
- 不把模型用途和 Agent Scene 合并；
- 不自动切换 Agent 模型或 Context Profile；
- 不原地重写旧 Profile 或旧 Qdrant 向量；
- 不把 Embedding/Rerank 平台成本并入本次用户钱包计费；
- 不因增加 `image` 类型就宣称支持所有图片模型；
- 不引入本地 Embedding Docker；
- 不引入 Office 转换器、OCR 或第三方在线预览；
- 不让 Qdrant参与文档权限和原文件查看；
- 不运行长测试脚本或 Playwright。

## 17. 实施顺序

1. 扩展官方目录 `model_kind` 与严格校验。
2. 扩展 Provider Model schema、DTO 和结构化写入契约。
3. 完成 Provider 候选自动分类和非破坏性同步。
4. 执行 `gpt-image-2` 与 Embedding 规格兼容迁移。
5. 收口 Agent Scene/Kind、图片模块和 Context Profile 选择。
6. 发布 Admin OpenAPI 并同步前端生成契约。
7. 完成供应商、Agent 和 Profile 前端联动。
8. 完成文档版本 Preview API 和右侧抽屉。
9. 运行短定向检查，交付人工验收清单。

## 18. 结论

【数据结构】官方目录定义受审模型用途，Provider Model 表示实际路由，Agent 表示业务场景，Profile 表示索引策略和不可变快照，层次清晰。

【特殊情况】`gpt-image-2` 不再通过永久模型 ID 特判伪装成 Chat；一次性迁移完成后，它就是普通的 `image` Provider Model。

【复杂度】四类闭合用途足够覆盖当前真实调用链，不引入多对多能力图、启发式分类或 Office 转换平台。

【兼容性】旧 Agent 主键引用、旧 Profile、旧空间、旧文档、旧索引、Run 和计费事实保持不变；迁移遇到冲突时明确失败，不猜测、不删数据。

【结论】值得做。它修复的是现有数据所有权和执行路由错误，同时补齐上下文文档最基本的可查看能力。
