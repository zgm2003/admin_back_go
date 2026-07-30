# AI 对话原生文件附件与上传规则整改设计

**日期：** 2026-07-30

**状态：** 设计已确认，待书面规格复核

**涉及仓库：** `admin_back_go`、`admin_front_ts`

**官方依据：** [OpenAI File inputs](https://developers.openai.com/api/docs/guides/file-inputs)

**规范关系：** 本文补充 `2026-07-28-ai-chat-capability-tools-interaction-design.md` 中官方模型能力、有效能力和原生文件输入边界。若旧文档仍把 Admin Chat 或 OpenAI-compatible transport 的输入能力固定为 `text + image`，以本文为准。本文不改变 `2026-07-30-ai-chat-canceled-partial-delivery-design.md` 的停止生成、上游排空和权威 usage 结算规则。

## 1. 背景与最终结论

当前 AI 对话已经支持图片选择、拖拽和剪贴板粘贴，但附件契约、上传目录、预览组件和 OpenAI-compatible 请求组装都写死为图片。官方模型目录中的 GPT-5.5、GPT-5.5 Pro 和 GPT-5.6 系列已经声明：

```json
{
  "input_modalities": ["text", "image", "file"],
  "native_file_input": true
}
```

界面当前仍可能显示“不支持文件”，不是官方模型目录错误，而是有效能力取交集时，transport 与 Admin Chat 平台实现仍只有 `text + image`。

本次采用以下最终方案：

1. 官方模型目录继续作为模型身份、能力和官方限制的唯一信源。
2. 供应商显式声明当前渠道是否支持 Chat Completions 原生文件协议，不通过供应商名称或 Base URL 猜测。
3. 前端将图片入口升级为统一附件入口，支持选择、拖拽和 `Ctrl+V` 粘贴图片或文件。
4. 文件只交给上游原生解析，不在本系统解析正文、切片、摘要或注入 prompt。
5. 数据库不保存 Base64 文件或膨胀后的完整上游请求；付费网关保存紧凑、可校验、可恢复的请求清单。
6. 派发时按清单从 COS 条件读取对象，边读边 Base64 编码到上游 HTTP body，保持内存有界。
7. 上游 usage 仍是唯一扣费事实；原生文件无法预知解析 token 时采用官方上下文窗口的保守冻结上界。
8. 同期修复上传规则前后端枚举漂移，并补全文件类型。

## 2. 目标与非目标

### 2.1 目标

1. 同一消息可以混合附带图片和文件，单条用户消息最多 5 个附件。
2. 文件选择、拖拽、剪贴板粘贴、编辑重发和重新生成使用同一附件状态机。
3. GPT-5.5、GPT-5.6 等官方支持文件的模型，在渠道协议已开启时可以真实读取文件。
4. 官方模型能力、渠道传输能力和平台实现能力各自拥有明确边界，错误文案不再混淆。
5. 上传配置仍是系统级扩展名和单文件大小事实源，AI 层只叠加不可修改的官方协议限制。
6. COS 对象身份、大小、类型和版本在付费派发前由后端验证，不能信任浏览器 DTO。
7. 文件请求可在 Worker 崩溃或租约恢复后重放同一请求事实，不从可变会话重新组装。
8. 文件内容不进入 MySQL，不要求一次性读入内存，不破坏纯文本和图片请求性能。
9. 真实上游拒绝、usage、停止生成和最终结算继续进入现有 Run、Attempt、Charge 和钱包事实链。

### 2.2 非目标

- 不实现后端 PDF、Word、Excel 或代码解析。
- 不实现文件切片、向量化、File Search、知识库或 RAG fallback。
- 不把不支持原生文件协议的渠道伪装为支持文件。
- 不为文件输入切换到 Responses API。
- 不自动调用付费模型探测供应商文件能力。
- 不支持 ZIP、RAR、TAR 等压缩包作为 AI 原生文件输入；它们可以继续是通用上传文件。
- 不支持音频、视频或可执行文件作为本期 AI 对话附件。
- 不重构通用上下文工程；仅对当前实际发送的历史附件做协议上限校验。

## 3. 核心不变量

### 3.1 能力只能逐层收窄

```text
EffectiveChatCapability =
  OfficialModelCapability
  ∩ TransportImplementedCapability
  ∩ ProviderFileInputMode
  ∩ AdminChatPlatformCapability
```

必须满足：

1. 官方目录声明 `file=false` 时，任何渠道开关都不能把模型扩大为支持文件。
2. 官方目录声明 `file=true` 但渠道为 `disabled` 时，界面显示“当前渠道未开通文件传输”，不能显示“模型不支持文件”。
3. 官方模型详情始终展示官方能力，不被当前供应商状态覆盖。
4. Agent 和前端不能保存一份可编辑的模型文件能力副本。

### 3.2 文件内容不进入数据库

以下内容禁止写入 `ai_messages.meta_json`、`ai_provider_attempts.prepared_request_json`、Run 快照、日志或操作审计：

- 文件原始字节；
- 文件 Base64；
- 包含完整 Base64 的物化上游请求；
- 从文件本地提取的正文或摘要。

数据库只保存附件引用、对象事实和版本化请求清单。

### 3.3 计费只信任上游 usage

- 文件字节数、页数、图片数量和扩展名不得用于估算最终扣费 token。
- 派发前冻结只做资金安全上界，不作为最终账单。
- 上游返回完整 usage 后按现有价格快照和倍率结算，并释放冻结差额。
- usage 缺失、超过冻结上界或结果未知时继续使用现有 fail-closed 规则，不猜测账单。

### 3.4 已准备请求必须可恢复

文件请求不再要求 MySQL 保存膨胀后的物化 HTTP body，但必须保存足以确定同一出站字节的规范清单：

```text
模型与参数
+ 有序消息结构
+ 有序附件引用
+ COS object key
+ ETag
+ 真实 size
+ MIME
+ filename
+ 清单 schema version
```

恢复时只允许从该清单物化。对象缺失、ETag 改变或元数据冲突时停止派发，禁止从当前会话重新猜一份请求。

## 4. 能力与供应商配置

### 4.1 官方模型能力

`official_models_v1.json` 继续保存每个模型的：

```text
input_modalities
native_file_input
context_window_tokens
max_output_tokens
官方价格与来源
```

GPT-5.5、GPT-5.5 Pro 和 GPT-5.6 系列保持 `text + image + file` 与 `native_file_input=true`。本期不通过经验或模型名称补能力；只修复有效能力链路对官方事实的消费。

### 4.2 供应商文件协议

`ai_providers` 增加：

```text
file_input_mode VARCHAR(32) NOT NULL DEFAULT 'disabled'
```

当前允许值：

| 值 | 语义 |
| --- | --- |
| `disabled` | 当前渠道不承诺原生文件输入 |
| `chat_completions` | 当前 Base URL/API Key 组合支持 Chat Completions `file` content part |

规则：

- 所有已有供应商迁移后默认 `disabled`，不根据 Base URL 猜测。
- 新增和编辑供应商时由管理员显式选择。
- 该字段是传输协议事实，不是模型能力字段。
- 不新增菜单、路由族或 RBAC 权限；复用供应商现有编辑权限。
- 连接测试仍只测试基础连接，不发送收费文件探测请求。

### 4.3 有效能力响应

Agent/Chat 有效能力中的附件段至少返回：

```json
{
  "attachments": {
    "image": {
      "enabled": true
    },
    "native_file": {
      "enabled": true,
      "max_files_per_message": 5,
      "max_file_bytes_exclusive": 52428800,
      "max_request_file_bytes": 52428800,
      "accepted_extensions": ["pdf", "docx", "md"]
    }
  }
}
```

约束由服务端返回，前端只消费，不自行根据模型 ID 或供应商名称推导。

## 5. 上传配置整改

### 5.1 当前确定的缺陷

当前后端图片扩展名包含 `psd`，前端 `allowedImageExts` 漏掉 `psd`，却额外包含后端不支持的 `jfif`。用户选择“全选”后，列表接口返回合法 `psd`，前端在 `toUploadRule` 抛出：

```text
upload rule item extensions violate the contract
```

当前枚举还存在：

- `doc` 被错误放入图片扩展名；
- 文件列表缺少 `pptx`、`md`、`json`、`tsv`、`rtf`、`odt` 和常见代码文件；
- 前端手写枚举与后端/OpenAPI 重复维护。

### 5.2 唯一契约

上传扩展名的运行时事实源仍是后端 `internal/shared/enum/upload.go`。后端请求和响应模型必须把扩展名输出为正式 OpenAPI enum，前端通过生成契约取得类型与运行时校验，删除：

```text
allowedImageExts
allowedFileExts
isUploadImageExt
isUploadFileExt
```

前端不得再维护第三份白名单。

### 5.3 系统级扩展名范围

系统图片类型移除错误的 `doc`，本期 canonical 集合固定为：

```text
jpeg jpg jfif pjpeg png gif webp bmp tif tiff svg ico psd avif
```

系统文件类型保留现有通用压缩文件，并覆盖 OpenAI 官方文件输入与常见代码类型；本期 canonical 集合固定为：

```text
pdf
doc docx dot odt rtf
ppt pptx pot ppa pps pwz wiz
xla xlb xlc xlm xls xlsx xlt xlw csv tsv iif
txt text md markdown json html htm xml css
asm bat c cc cpp cxx h hh def in
js mjs jsx ts tsx py go java cs php rb rs
sh bash zsh ksh ps1 sql pl lua r scala swift kt kts
yaml yml toml ini conf properties proto
eml log rst srt vtt ics ifb vcf diff patch
zip tar
```

文件名无扩展名的 `Dockerfile`、`Makefile` 等仍不在本期通用上传契约内；不通过 MIME 猜扩展名。

### 5.4 AI 子集

AI 对话使用系统上传规则与官方 AI 允许范围的交集：

```text
AIAllowed = EnabledUploadRule ∩ OfficialNativeFilePolicy
```

因此：

- `zip`、`tar` 可以在其他业务上传，但不能作为 AI 文件输入；
- `psd`、`svg`、`ico` 等系统图片不自动成为模型图片输入；
- AI 图片只接受官方图像协议支持的 `png/jpeg/jpg/webp` 与非动画 `gif`；
- PDF、Office、表格、文本和代码文件按官方文件输入清单开放。

## 6. 附件契约与对象存储

### 6.1 前后端附件结构

统一附件结构：

```json
{
  "type": "file",
  "object_key": "ai_chat_attachments/2026/07/30/...",
  "url": "https://cos.example/...",
  "mime_type": "application/pdf",
  "name": "report.pdf",
  "size": 1024
}
```

新写入必须满足：

- `type` 只能为 `image|file`；
- `object_key`、`mime_type`、`name`、`size` 必填；
- `url` 由后端根据可信 bucket domain 和 object key 重建，不信任浏览器域名；
- 文件名只用于展示和上游 filename，不参与对象寻址；
- `size` 在消息受理时用 COS HEAD 真实值覆盖浏览器声明。

历史图片继续使用现有兼容读取规则；新增文件不允许缺字段或靠 URL 猜 object key。

### 6.2 存储目录

新增统一目录：

```text
ai_chat_attachments/
```

新图片和文件都写入该目录。历史：

```text
ai_chat_images/
```

保持可读、可展示和可编辑重发，不迁移旧对象。

COS inspector/reader 同时信任新目录和历史图片目录，但文件类型只允许来自新目录。对象 key 必须经过规范化前缀检查，禁止路径穿越和任意 bucket 对象引用。

### 6.3 对象事实

消息受理与请求准备阶段至少验证：

```text
object key 前缀
Content-Length
ETag
Content-Type
文件扩展名
浏览器声明与 HEAD 事实的一致性
```

对 Office、代码等对象，COS 可能返回 `application/octet-stream`。扩展名已在官方白名单内时允许通用二进制 MIME；若返回明确且冲突的 MIME，则拒绝。图片继续使用严格的图片 MIME 校验。

## 7. 数量与大小限制

限制分为系统上传规则和 AI 协议规则：

| 约束 | 规则 |
| --- | --- |
| 系统单文件上限 | 当前启用上传规则，例如 100 MB |
| 单条用户消息附件数 | 图片和文件合计最多 5 个 |
| 单条用户消息附件总量 | 图片和文件合计不超过 50 MiB |
| AI 单个原生文件 | 严格小于 50 MiB |
| 一次上游请求全部原生文件 | 当前消息与入选历史消息中的文件合计不超过 50 MiB |

实际单文件上限：

```text
min(系统上传规则上限, AI 官方单文件上限)
```

`50 MiB` 固定为 `50 * 1024 * 1024` 字节。单个原生文件必须 `< 52428800`，一次上游请求的原生文件合计必须 `<= 52428800`。

全局上传规则可以继续保持 100 MB。AI 的 50 MiB 是代码拥有的官方协议限制，不增加管理员可编辑的第二个大小配置。

历史附件也会进入 Chat Completions 上下文，因此 Worker 在准备请求时必须按最终入选消息重新计算文件总量。超过上游请求上限时，在 Provider 派发前失败并给出“当前对话文件上下文超过 50 MB，请新建对话或减少历史范围”的稳定错误，不静默丢弃旧文件。

## 8. 付费请求清单与流式物化

### 8.1 为什么不能内联持久化 Base64

50 MiB 文件 Base64 后约为 66.7 MiB。当前 `prepared_request_json` 为 `MEDIUMTEXT`，且付费网关会复制、哈希和恢复该字段。直接内联会：

- 超过字段容量；
- 把文件完整复制进 MySQL；
- 增加数据库备份、查询和 Run 详情统计成本；
- 在并发 Worker 中制造大块内存分配；
- 暴露不必要的文件内容审计面。

本设计明确禁止将 Base64 请求写入数据库或把字段改成 `LONGTEXT` 规避问题。

### 8.2 两种 prepared request 版本

纯文本和现有图片请求继续使用当前精确上游 JSON：

```text
openai_chat_inline_v1
```

包含原生文件时使用紧凑清单：

```json
{
  "schema": "openai_chat_file_manifest_v1",
  "request": {
    "model": "gpt-5.6",
    "stream": true,
    "messages": []
  },
  "files": [
    {
      "ref": "file-1",
      "object_key": "ai_chat_attachments/.../report.pdf",
      "etag": "...",
      "size": 1024,
      "mime_type": "application/pdf",
      "filename": "report.pdf"
    }
  ]
}
```

`request.messages` 中使用内部、版本化的 `file_ref` content part 指向 `files.ref`。清单必须具有确定字段顺序和附件顺序；`prepared_request_sha256` 改为绑定该规范清单。恢复只读取已持久化清单，不重新查询 Agent 配置、价格、模型或当前消息内容。

### 8.3 COS 条件读取

派发前：

1. 解析并严格校验清单 schema。
2. 对每个对象执行 HEAD，验证 key、size、ETag 和 MIME。
3. GET 对象时携带 `If-Match: <etag>`。
4. 对象不存在、被覆盖或 ETag 不一致时拒绝物化。
5. 不把 COS URL 或临时凭证发送给模型渠道。

### 8.4 有界流式编码

OpenAI-compatible adapter 将 `file_ref` 物化为 Chat Completions 官方 `file` content part，并把对象字节编码到 `file_data`。

实现必须：

- 通过 `io.Reader`/`io.Pipe` 产生 HTTP body；
- 使用流式 Base64 encoder；
- 按清单文件 size 计算 Base64 长度和最终 `Content-Length`；
- 按顺序逐个读取文件，不同时加载所有文件；
- 不构造完整 `[]byte` 或完整 Base64 `string`；
- 请求 context 取消时关闭 COS reader 和 HTTP pipe；
- 保持现有 `Idempotency-Key`、SSE、工具调用和 usage 解析。

纯文本和图片请求继续走当前 `[]byte` prepared body，不承担文件物化开销。

### 8.5 部分发送失败

如果 COS 或本地编码在 HTTP body 已开始发送后失败，结果按现有 provider outcome 规则归类，不盲目重试为新 attempt。相同 attempt 只能在现有“已准备但明确未派发”边界内恢复；已可能到达上游时继续使用 outcome unknown 和 fail-closed 结算规则。

## 9. 冻结、输出上限与最终结算

### 9.1 新的安全上界策略

当前纯文本策略：

```text
utf8_request_bytes_plus_framing_v1
```

不能用于原生文件，因为压缩文档、PDF 页面图像和表格增强后的 token 与文件字节数没有可靠一一关系。

包含原生文件的请求采用：

```text
native_file_context_window_v1
```

冻结规则：

```text
input financial upper bound  = official_model.context_window_tokens
output financial upper bound = official_model.max_output_tokens
```

这是资金冻结的独立最坏上界，允许有意过度冻结；它不声称输入和最大输出会同时占满上下文。最终实际费用只能来自上游完整 usage。

### 9.2 Provider 请求输出上限

文件请求仍使用官方模型的有效最大输出，不增加用户可调 `max_tokens`。由于本系统不解析文件，无法在派发前知道上游实际文件 token；若上游因文件内容与最大输出合计超过上下文而拒绝，请求按明确 provider rejected 失败，没有完整 usage 时不扣费。

### 9.3 价格与余额

原生文件请求可能比纯文本请求冻结更多余额。界面和运行记录沿用现有“冻结不是最终扣费”语义：

- 余额不足时不派发上游；
- 成功后按真实 input/cache/output usage 结算；
- 释放全部冻结差额；
- 实际费用超过保守冻结属于结构异常，继续使用 `unbilled_over_hold`，不得透支钱包。

### 9.4 停止生成

文件请求完全复用既有停止设计：

- 前端点击后立即停止展示；
- 上游已派发时后台继续排空；
- 聊天只保存用户已见前缀；
- 最终按上游完整 usage 结算；
- 停止发生在 COS 物化或 Provider 派发前时，不产生上游 usage，释放冻结。

## 10. 后端模块边界

### 10.1 `uploadconfig` / `uploadtoken`

- `uploadconfig` 继续拥有系统扩展名和大小配置。
- `uploadtoken` 继续只签发 COS token，不拥有 AI 文件策略。
- 新增 `ai_chat_attachments` folder。
- 后端签发 token 时继续校验声明的扩展名和系统单文件上限。

### 10.2 `ai/message`

- Attachment 接受 `image|file`。
- 统一数量、字段、官方能力和当前消息大小校验。
- 使用 COS inspector 覆盖真实对象事实。
- `meta_json` 只保存引用和展示元数据。
- 编辑重发、重新生成、请求指纹和输入快照必须包含文件附件身份。

### 10.3 `ai/capability`

- Admin Chat 平台实现能力扩展到 `text + image + file`。
- Provider `file_input_mode` 参与有效能力求交集。
- 官方模型能力和渠道关闭原因分别投影。

### 10.4 `ai/aigateway`

- prepared request 支持 inline body 和 file manifest 两种版本。
- quote proof 识别 `native_file_context_window_v1`。
- 请求 hash 绑定规范清单。
- 恢复验证同一 manifest，不重新组装可变输入。

### 10.5 `infra/ai/openaicompat`

- 继续拥有 Chat Completions 协议编码、SSE、usage 和 provider error 解析。
- 通过小接口接收附件流，不直接依赖 COS SDK 或业务 repository。
- 只有 file manifest 触发流式物化。

### 10.6 `infra/storage/cos`

- 增加受 context 控制的条件流式读取接口。
- 支持 HEAD 事实和 `If-Match` GET。
- reader 只允许受信 AI attachment prefix 和清单中的精确 key。

## 11. 前端交互

### 11.1 统一入口

原“上传图片”按钮改为回形针图标“添加附件”：

- 文件选择器根据当前有效能力动态接受图片和文件；
- 拖拽和文件选择走同一处理函数；
- `Ctrl+V` 中存在浏览器可见的文件项时加入附件队列；
- 纯文本剪贴板继续正常写入输入框；
- 网页复制出的图片继续按图片处理。

浏览器或操作系统未向 `ClipboardEvent.clipboardData.files/items` 暴露的本地文件无法被网页读取；该情况不伪造成功，用户仍可使用回形针或拖拽。

### 11.2 附件状态

统一状态机：

```text
queued -> uploading -> uploaded
                  -> failed -> retrying
```

- 上传中或存在失败附件时禁止发送。
- 重试只重传失败项，已成功项不重复上传。
- 同一选择/粘贴批次按文件身份去重。
- 图片显示缩略图；文件显示类型图标、名称、大小、状态和删除按钮。
- 历史文件卡片可以打开可信 URL。

### 11.3 模型与 Agent 切换

- 未选择 Agent 时附件按钮禁用。
- 当前 Agent 仅支持图片时选择器只接受图片。
- 切换 Agent 后已有文件不兼容时，不静默删除；保留附件、阻止发送并提示当前渠道未开通文件传输。
- 官方模型支持文件但渠道关闭时，不显示“模型不支持文件”。

### 11.4 编辑与重新生成

- 编辑用户消息默认保留原附件，可删除或增加附件。
- 编辑重发必须重新执行当前模型、渠道、上传规则和 COS 对象验证。
- 重新生成复用源用户消息的附件清单。
- 历史对象不可用时消息仍可展示文件卡片和不可用状态，但不能发起新的付费请求。

## 12. 错误、观测与安全展示

新增或收敛稳定错误分类：

| 场景 | 行为 |
| --- | --- |
| 官方模型不支持文件 | 受理前拒绝，指出模型能力 |
| 渠道 `file_input_mode=disabled` | 受理前拒绝，指出渠道未开通 |
| 文件类型不在系统规则 | 上传 token 阶段拒绝 |
| 文件类型不在 AI 官方范围 | AI 消息受理前拒绝 |
| 当前消息超过数量/大小 | 前端提示，后端再次拒绝 |
| 历史文件合计超过 50 MiB | 请求准备阶段拒绝，不派发 |
| COS 对象缺失或 ETag 变化 | 派发前/物化时拒绝 |
| 上游不接受 `file` part | provider rejected；无 usage 不扣费 |
| 上游 usage 不完整 | 现有 unbilled 规则 |

运行详情增加或复用以下安全摘要：

```text
attachment_count
native_file_count
native_file_bytes
cos_head_latency_ms
cos_stream_latency_ms
provider_ttft_ms
prepared_manifest_bytes
materialized_request_bytes
file_input_mode
```

不展示 object 临时凭证、完整 object key、文件内容、Base64、完整 prepared manifest 或上游原始请求。

## 13. Schema、API 与契约变更

### 13.1 数据库

本期唯一必要业务字段：

```text
ai_providers.file_input_mode
```

同步更新最终 HCL 和迁移。附件继续保存在 `ai_messages.meta_json`；不新增文件内容表，不把 `prepared_request_json` 改成 `LONGTEXT`。

### 13.2 Admin API

供应商 create/update/list/detail 增加 `file_input_mode`，page-init 增加闭合字典。

AI 消息附件契约将：

```text
type: image
```

扩展为：

```text
type: image | file
```

有效能力响应返回原生文件限制和准确关闭原因。

### 13.3 Admin Contract Bundle

后端重新生成：

```text
contracts/admin/v1/openapi.json
route/access/audit bundle manifest
```

前端重新生成：

```text
src/modules/http/generated/admin.ts
src/modules/http/generated/operations.ts
```

上传扩展名 enum 必须同时约束请求和响应，确保前端不再手写白名单。

## 14. 测试与验收

### 14.1 后端定向测试

1. 官方 `file=true`、transport 实现、provider mode 和平台能力的交集。
2. provider mode 默认 disabled、合法枚举和 DTO 投影。
3. 上传扩展名：`psd` 全选可返回、`jfif` 一致、图片列表不含 `doc`、文件列表包含新增类型。
4. Attachment `image|file` 归一化、最多 5 个、单文件严格小于 50 MiB、当前消息总量。
5. COS 前缀、HEAD size/MIME/ETag、历史图片兼容和文件仅允许新目录。
6. 最终历史文件合计不超过 50 MiB。
7. manifest 规范序列化、hash 稳定、字段/引用/顺序/ETag 篡改被拒绝。
8. 条件 GET、流式 Base64、Content-Length 和 context cancellation。
9. 物化 body 符合 Chat Completions file content part，不包含内部 `file_ref`。
10. 文本/图片 inline prepared request 行为不变。
11. 文件上界策略、冻结、完整 usage 结算、余额不足和 over-hold fail closed。
12. 编辑重发、重新生成、停止生成和恢复复用同一附件事实。

### 14.2 前端定向测试

1. 选择、拖拽、截图粘贴和文件粘贴进入同一队列。
2. 纯文本粘贴不触发附件上传。
3. 上传失败可重试，成功项不重复上传。
4. 图片缩略图与文件卡片状态正确。
5. 未选 Agent、模型不支持、渠道关闭和切换不兼容 Agent 的状态正确。
6. 编辑重发和重新生成保留文件附件。
7. 上传规则列表包含 `psd` 时不再抛契约错误。
8. OpenAPI 类型检查与 Vitest 定向用例通过。

### 14.3 手工付费验收

不在自动化测试中调用真实收费渠道。手工验收至少覆盖：

1. GPT-5.5 和 GPT-5.6 分别读取图片、PDF、Word、Excel、Markdown 和代码文件。
2. PDF 同时包含文字和页面图像；确认模型可以读取二者。
3. Word/PPT 内嵌图表不承诺视觉识别，符合官方“非 PDF 仅提取文本”限制。
4. 选择、拖拽、`Ctrl+V`、编辑重发和重新生成。
5. provider mode 关闭时文件不可发送，文本和图片不受影响。
6. 文件生成中停止，界面立即停止，Run 最终按权威 usage 结算。
7. 核对 COS 读取耗时、上游 TTFT、冻结金额、实际扣费和释放差额。

本期不要求 Playwright 或长脚本；只运行定向 Go/Vitest/类型与 contract gate，真实渠道由用户手工验收。

## 15. 实施顺序

1. 修复上传扩展名唯一契约和上传规则页面崩溃。
2. 增加 provider `file_input_mode` schema、API、表单和有效能力交集。
3. 扩展附件 DTO、上传目录、COS inspector 与消息存储。
4. 完成前端统一附件队列、文件卡片、选择/拖拽/粘贴和编辑链路。
5. 增加 prepared file manifest、COS 条件流 reader 和 Chat Completions 流式物化。
6. 增加 native file 安全上界策略并接入现有 Gateway、Hold 和 settlement。
7. 增加错误分类、运行观测、安全摘要和历史上下文总量校验。
8. 同步 HCL、迁移、OpenAPI、生成客户端、架构文档和定向测试。
9. 用户手工执行真实渠道和付费验收。

## 16. 完成标准

1. 上传规则页面选择全扩展名后可正常加载，前端不存在手写扩展名白名单。
2. 系统上传规则包含确认的图片、文档、表格和代码类型，`doc` 不再属于图片。
3. GPT-5.5/5.6 官方详情始终显示文件能力。
4. 渠道关闭时准确显示渠道原因，不能把渠道问题说成模型不支持。
5. 文件按钮只在有效能力允许时开放，伪造请求同样被后端拒绝。
6. 选择、拖拽和浏览器可见的剪贴板文件均可上传。
7. 图片和文件共用可靠的上传、失败重试、删除和编辑状态机。
8. 新附件使用 `ai_chat_attachments/`，历史 `ai_chat_images/` 仍可用。
9. MySQL 中不存在文件 Base64 或膨胀后的完整文件请求。
10. 50 MiB 文件物化不会一次性分配同等大小的原始字节和 Base64 字符串。
11. prepared manifest、ETag 与条件读取保证恢复时不会发送被替换的对象。
12. 文件请求不会使用 UTF-8 请求字节假装 token 上界。
13. 余额冻结覆盖官方最坏上界，最终只按完整上游 usage 扣费。
14. 文件请求停止后继续符合现有即时停止、后台排空和最终结算语义。
15. 纯文本与现有图片请求的延迟、prepared body 和结算路径没有回归。
16. 后端定向测试、前端定向测试、类型检查和 Admin Contract gate 通过。

## 17. 已确认的产品决策

1. 本期只做上游原生文件输入，不做本地解析 fallback。
2. 官方模型能力仍是唯一模型能力信源。
3. GPT-5.5、GPT-5.5 Pro 和 GPT-5.6 系列支持文件输入。
4. 当前渠道必须显式声明 Chat Completions 文件协议支持。
5. 采用现有 Chat Completions 通道，不切 Responses API。
6. 通用上传配置继续允许 100 MB；AI 固定叠加官方 50 MiB 限制。
7. 单条消息图片和文件合计最多 5 个。
8. 文件范围包含 PDF、Office、表格、文本、Markdown、JSON/XML/HTML 和常见代码；AI 拒绝压缩包、音视频和可执行文件。
9. UI 使用一个回形针附件入口，同时支持选择、拖拽和 `Ctrl+V`。
10. 新附件统一进入 `ai_chat_attachments/`，保留旧图片路径兼容。
11. 文件请求保存紧凑 manifest，并在派发时从 COS 条件流式物化。
12. 文件内容和 Base64 不进入数据库。
13. 原生文件请求按官方上下文最坏费用冻结，按上游真实 usage 结算。
14. 不运行 Playwright 和长脚本；真实付费渠道由用户手工验收。

当前范围没有剩余产品决策。书面规格复核通过后进入实施计划，不在计划中加入真实收费自动化或长脚本测试。
