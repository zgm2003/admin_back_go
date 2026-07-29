# AI 官方模型单一信源、对话交互与工具调用修复设计

**日期：** 2026-07-28

**状态：** 设计已确认，可进入 implementation plan

**涉及仓库：** `admin_back_go`、`admin_front_ts`
**规范关系：** 本文补充现有 AI Gateway 和聊天消费规范，并取代 `2026-07-27-ai-model-pricing-management-design.md` 中“模型定价”的领域命名、菜单、接口、RBAC、表名和 Agent 最大输出配置。计费、冻结、结算公式仍以 `2026-07-24-ai-chat-consumer-pricing-wallet-design.md` 为准；与本文“官方模型唯一信源”和输出上界规则冲突的旧描述以本文为准。

## 1. 背景与结论

当前发现的四个问题并非互不相关：

1. Agent 编辑页和聊天输入框都允许设置最大输出 Token，形成了两份可变配置，并与官方模型上限冲突。
2. 用户只发送 `hi` 也可能等待约 20 秒，界面和运行审计无法准确说明时间消耗在哪个阶段。
3. 工具已在后台启用并绑定 Agent，真实聊天请求却没有携带工具定义。
4. 当前“模型定价”只承载官方价格，没有成为模型身份、生命周期、上下文、输出上限和能力的统一基础事实。

根本缺口是一条统一契约没有贯穿以下链路：

```text
官方模型事实
  -> 当前传输引擎与渠道能力
  -> Agent 管理策略
  -> Worker 最终请求组装
  -> 聊天界面功能门控
  -> Run / Attempt 审计
```

本文最终选择：

- 将“模型定价”完整重命名并扩展为“官方模型”；
- 仓库内版本化 `OfficialModelCatalog` 是模型身份、别名、生命周期、能力、上下文窗口、最大输出和官方基准价格的唯一基础信源；
- Agent 和聊天都删除最大输出配置，运行时只采用官方模型上限；
- 官方模型的基础字段全部只读，数据库只允许保存人工同步的当前生效价格；
- 供应商模型只能按 canonical ID 或受审别名匹配官方模型，未匹配模型禁止创建 Agent、调用和计费；
- 功能是否展示由服务端返回的“有效能力”决定，前端不再猜模型名称；
- 聊天暂时保留 `temperature` 和“携带历史消息”，后者在上下文工程落地后删除；
- 工具调用先修复 Worker 依赖装配，并增加装配级测试；
- 知识库本期不修补现有拼接逻辑，只划定后续上下文工程边界；
- 延迟按受理、排队、本地准备、首个增量、上游完成、结算分段记录。

## 2. 已确认的现状证据

### 2.1 参数交互冲突

Agent 当前保存 `max_output_tokens`，后端把它作为该 Agent 的有效输出上限。聊天页同时允许提交：

```text
temperature
max_tokens
max_history
```

其中聊天页 `max_tokens` 滑块被硬编码为 `256～32768`，默认视觉位置为 `4096`，没有读取：

- 当前 Agent 的 `max_output_tokens`；
- 官方模型目录的 `max_output_tokens`；
- 当前模型是否支持 `temperature`。

后端实际规则是：聊天显式提交的 `max_tokens` 覆盖本次调用，但不能超过 Agent 上限和官方模型上限。当前界面因此存在三份输出上限事实。

最终设计不再修补滑块范围，而是删除这两份可变配置：

- Agent 创建、编辑和详情接口删除 `max_output_tokens`；
- `ai_agents.max_output_tokens` 从最终数据库结构删除；
- 聊天请求的 `runtime_params` 删除 `max_tokens`；
- 前端 Agent 表单和聊天设置面板不再展示最大输出；
- 后端收到新请求携带 `max_tokens` 时返回参数不受支持，不静默忽略；
- 每次调用的安全输出上界只由官方模型目录和本次输入占用计算。

图片入口同样被硬编码：

```ts
const supportsImage = computed(() => true)
```

这不代表模型真的支持图片，只代表当前前端无条件开放入口。

### 2.2 `hi` 请求耗时

2026-07-28 的 Run `455` 对应会话 `161`，模型为 `gpt-5.4`。分段耗时如下：

| 阶段 | 耗时 | 判断 |
| --- | ---: | --- |
| HTTP 受理 | 约 `843ms` | 本地受理偏慢，但不是主因 |
| Command 创建到 Worker 开始 | `1215ms` | 受 1 秒轮询周期影响 |
| Worker 开始到 Provider 派发 | `1489ms` | 本地加载、组装、报价和冻结 |
| Provider 派发到完整终态 | `17220ms` | 本次主要耗时 |
| Provider 完成到 Command 终态 | `119ms` | 本地结算正常 |
| Run 总时长 | `19876ms` | 与用户感知约 20 秒一致 |

该请求最终 prepared request 只有 `491` 字节、5 条消息，消息正文约 222 个字符。上游仍报告：

```text
prompt_tokens      = 4576
cache_read_tokens  = 3840
completion_tokens  = 8
```

这组 Token 是上游响应中的原始 usage，不是本系统根据 491 字节请求自行估算。需要把它作为渠道审计证据保留，但仅凭当前事实不能断言渠道做了何种隐藏包装。

现有遥测在进程内观测 `provider.first_byte_seconds`，但没有把首个增量时间持久化到 Run / Attempt。因此服务重启后只能确认 Provider 总耗时，无法从数据库严谨还原 TTFT。

### 2.3 工具调用根因

当前唯一已注册的服务端执行器是：

```text
admin_user_count
```

Agent `157` 已绑定并启用该工具，但数据库中历史 `ai_tool_calls` 总数为 `0`，Run `455` 的 prepared request 也没有 `tools` 字段。

根因不是模型拒绝调用，而是进程装配不一致：

- Admin API 创建 `ChatService` 时注入了 `ToolRuntime`；
- Durable Reply 实际由 Worker 执行；
- Worker 创建 `ChatService` 时没有注入 `ToolRuntime`；
- `ChatService` 对空 `ToolRuntime` 静默返回空工具列表；
- Provider 从未收到工具定义，自然不可能产生 `tool_calls`。

业务服务单测使用手工注入的 fake runtime，因此没有覆盖生产 Worker 的依赖图。

### 2.4 当前官方模型事实不完整

当前官方价格目录已保存 canonical model ID、别名、厂商、family、价格、最大输出 Token 和来源信息，但没有形成统一的官方模型领域，也没有保存：

- 输入和输出模态；
- 工具调用能力；
- 支持的生成参数；
- 图片、音频或原生文件输入方式；
- 上下文窗口；
- 附件 MIME、数量和大小限制。

Provider 同步得到的模型记录也只有模型 ID、展示名和启停状态。系统目前无法可靠回答“这个模型是否真的能识别图片或文件”，也不能证明一个渠道模型应使用哪一组官方能力和价格。

## 3. 产品原则

### 3.1 权威来源和策略边界

```text
官方模型：模型身份、生命周期、能力、限制和官方基准价格
价格覆盖：管理员根据官方变价人工同步的当前生效价格
供应商映射：渠道模型指向哪个已受审官方模型，只能精确自动匹配
Agent 策略：工具、知识来源和允许使用的产品能力
单次设置：temperature 与过渡期 max_history
```

最大输出不属于 Agent 策略或单次设置。禁止再出现来源不明的前端默认上限，所有界面和运行时必须能追溯到同一官方模型目录版本。

### 3.2 未知能力默认关闭

对于官方目录没有收录、别名无法唯一解析或出现歧义的供应商模型：

- 标记为“未映射”；
- 不允许管理员手工选择一个相似模型强行映射；
- 不允许启用为 Agent 模型；
- 不允许发起调用、冻结余额或计费；
- 必须先通过代码审查把 canonical ID 或渠道 ID 作为受审别名加入官方目录，随新版本发布后才能使用。

Provider 的模型列表只能用于发现渠道模型，不能自动扩展官方模型事实。

### 3.3 前端隐藏不是安全边界

前端根据有效能力优化体验，后端仍必须对最终请求进行权威校验。伪造请求不能绕过：

- 模态限制；
- 附件 MIME、数量和大小限制；
- 参数支持和数值范围；
- 官方模型限制及 Agent 能力策略；
- 工具绑定、工具状态、风险级别和服务端执行器注册状态。

### 3.4 文件能力必须拆开表达

“支持文件”不能作为一个布尔值。至少区分：

1. **模型原生文件输入：** 供应商协议把文件 ID 或原始文件作为模型输入。
2. **平台文本提取：** 平台解析 PDF、DOCX 等文档，再把受 Token 预算约束的文本加入上下文。
3. **图片视觉输入：** 图片通过 URL、data URI 或供应商文件引用进入视觉模型。

只有第一种可以标记为“模型原生文件”。第二种属于后续上下文工程，本期不以附件按钮假装已经支持。

## 4. 最终交互设计

### 4.1 Agent 编辑页

Agent 编辑弹窗不再承担模型基础配置，只选择供应商路由和已映射的官方模型，并配置 Agent 自身业务策略：

```text
基本信息
  名称 / 头像 / 使用场景 / 状态

模型
  供应商 / 供应商模型
  映射的官方模型
  只读摘要：生命周期、上下文、最大输出、输入/输出模态、工具、流式

可用能力
  工具绑定
  知识来源绑定
  Agent 级能力开关（只能收窄官方模型能力）

计费
  当前生效价格摘要 / Agent 倍率 / 预估说明
  查看官方模型详情
```

Agent 表单中彻底删除“最大输出 Token”输入框。模型摘要可以只读显示：

```text
官方模型：gpt-5.4
上下文窗口：1,050,000 Token
单次最大输出：128,000 Token
目录版本：official_models_v1
```

这里的最大输出只是官方事实展示，不是可编辑控件。Agent 创建、更新和详情 DTO 不再包含 `max_output_tokens`，Agent 表也不再保存该列。

模型选择规则：

- 新建或切换 Agent 时只列出 `active` 且已映射的供应商模型；
- 已绑定 `deprecated` 模型的 Agent 可以继续使用，但编辑页显示警告且不能重新选择该模型；
- 已绑定 `retired` 模型的 Agent 不允许调用，必须切换到其他 `active` 模型后才能恢复；
- 供应商模型未映射官方模型时不能进入 Agent 下拉选项。

模型选择后紧邻模型字段显示紧凑摘要，例如：

```text
输入：文本、图片    输出：文本    工具：支持    流式：支持
官方模型 · active · 2026-07-27 核验
```

官方目录未声明的能力不开放，不提供“仍然尝试”的入口。

工具绑定弹窗只列出真正可执行的工具：

- 工具记录启用；
- 工具未删除；
- 服务端已注册对应 executor；
- 当前 Agent 模型有效能力包含 `tools`；
- 当前自动执行阶段只允许 `risk_level=low`。

暂未实现 executor 的工具草稿可以保存为禁用状态，但不能绑定到生产 Agent，也不能显示为“可用”。

### 4.2 “官方模型”页面与详情抽屉

现有“模型定价”菜单一次性更名为“官方模型”。页面不再以价格作为唯一主体，而是先回答“这是什么模型、能做什么、限制是什么、当前如何计费”。

#### 列表信息

| 列 | 内容 |
| --- | --- |
| 官方模型 | canonical ID、family、vendor、受审别名 |
| 生命周期 | `active`、`deprecated`、`retired` |
| 输入 / 输出 | 文本、图片、音频、原生文件等模态摘要 |
| 核心能力 | 工具、流式、结构化输出等紧凑状态 |
| 模型限制 | 上下文窗口、单次最大输出，只读 |
| 当前价格 | 生效价格摘要及“官方 / 人工同步”来源 |
| 核验 | 目录版本、最后核验日期 |
| 操作 | 查看详情；有权限时同步价格 |

默认只列出正式受审模型，可按厂商、family、生命周期、输入模态和 model ID 筛选。价格仍可搜索和查看，但不再把页面命名为价格管理工具。

#### 详情抽屉

点击模型行或“查看详情”打开官方模型详情抽屉。桌面端使用稳定宽度，移动端全屏；抽屉按以下顺序展示：

```text
模型身份
  canonical model ID / vendor / family / aliases / 生命周期

容量与限制
  上下文窗口 / 单次最大输出 / 长上下文阈值

输入与输出
  input modalities / output modalities
  原生文件输入与图片输入方式

模型能力
  tools / streaming / structured output / 支持的生成参数

价格
  官方基准价 / 当前生效价 / 来源 / 覆盖版本
  [同步价格] [恢复官方价格]

事实来源
  模型文档 URL / 价格文档 URL / 目录版本 / 核验日期 / review_after
```

交互规则：

- 身份、生命周期、能力、限制和官方基准价全部只读；
- “同步价格”只修改数据库中的当前生效 rate、来源 URL 和核验日期；
- 人工同步价格不能修改 canonical ID、别名、能力、上下文或最大输出；
- 官方目录版本更新后，人工覆盖仍继续生效，直到管理员明确执行“恢复官方价格”；
- 新官方基准价与人工覆盖完全一致时显示“可恢复官方”，但不自动删除覆盖事实；
- `deprecated` 可以同步价格，`retired` 只读保留历史事实，不再允许创建新的价格覆盖；
- 抽屉顶部明确显示当前来源是“官方基准”还是“人工同步”。

#### 生命周期

生命周期由我们审核后随官方目录发布，不跟随任一供应商自动变化：

| 状态 | 新建 / 切换 Agent | 已有 Agent | 调用 |
| --- | --- | --- | --- |
| `active` | 允许 | 允许 | 允许 |
| `deprecated` | 禁止 | 暂时保留并警告 | 允许 |
| `retired` | 禁止 | 必须切换模型 | 禁止 |

供应商停用某个渠道模型只影响该供应商路由；官方模型本身仍可通过其他已映射供应商使用。只有官方目录标记 `retired` 才会全平台禁止该 canonical model。当前 Agent 若绑定的正是已停用渠道，则该 Agent 的调用在 Provider 派发和费用冻结前被拒绝，管理员需要恢复原渠道或切换到另一个已映射且可用的渠道；本期不隐式跨供应商自动切换，避免路由、价格和账单事实悄悄变化。

### 4.3 聊天输入区

聊天输入区默认保持简洁，只展示当前有效能力允许的动作：

```text
[附件入口（按能力出现）] [输入框................] [本次回答设置] [发送]
```

规则：

- 仅支持文本时不显示回形针/图片按钮；
- 支持图片时显示图片入口，并按能力返回的 MIME、大小和数量限制选择文件；
- 只有原生文件或平台解析管线真正可用时才显示文档入口；
- 工具由 Agent 自动提供给模型，不在普通聊天输入区增加“调用工具”按钮；
- 当前 Agent 切换后，立即重新计算有效能力并清理不再合法的待发送附件和 `temperature` 覆盖，清理前需要明确提示。

### 4.4 “本次回答设置”面板

齿轮按钮打开的面板继续叫“本次回答设置”。最大输出从面板完全删除，默认状态不提交任何覆盖参数。

```text
本次回答设置                         [恢复默认]

随机性
使用模型默认                          [启用覆盖]
启用后：[0.7]   范围 0～2
仅模型明确支持 temperature 时显示

携带历史消息
系统默认（20 条）                     [启用覆盖]
启用后：[20]    范围 1～50
```

面板只显示当前仍允许用户控制的两个参数。最大输出即使以只读形式也不放在这里，避免再次形成“可以调整”的认知。

控件规则：

- 二元行为使用开关，不通过“拖动滑块就突然变成自定义值”表达；
- 数值使用输入框或步进器，必要时辅以滑块；
- 不展示、不构造、不提交 `max_tokens`；
- `temperature` 仅在有效能力声明支持时显示；
- `max_history` 在 UI 中叫“携带历史消息”，不再表现为模型采样参数；
- 面板关闭不会丢失本次已选覆盖，切换会话或 Agent 时恢复默认；
- 发送请求只携带已经显式启用的覆盖字段；
- `max_history` 是上下文工程上线前的过渡参数，后续由统一 `ContextBuilder` 策略替代并删除。

### 4.5 普通用户与高级用户

采用“默认简洁、按需展开高级参数”，而不是维护两套独立页面：

- 普通用户默认只看到输入、附件和发送；
- 设置按钮上有自定义状态点，表示当前存在单次覆盖；
- 高级参数面板只包含当前模型支持的 `temperature` 和过渡期历史条数；
- 不在页面常驻说明模型原理或操作教程，必要解释通过字段辅助文本和 tooltip 提供。

## 5. 官方模型唯一信源契约

### 5.1 `OfficialModelCatalog` 是唯一基础信源

不再创建相互独立的“价格目录”和“能力目录”。仓库内只维护一份版本化、可代码审查的 `OfficialModelCatalog`，每个 canonical model 条目同时拥有：

- 身份：vendor、family、canonical model ID、受审别名；
- 生命周期：`active`、`deprecated`、`retired`；
- 容量：上下文窗口、最大输出、长上下文阈值；
- 输入和输出模态；
- 工具、流式、结构化输出和生成参数能力；
- 图片、音频和原生文件等模型固有输入能力；
- 官方基准价格及计费分类；
- 模型文档与价格文档来源、核验时间和复核期限。

可以在 Go 内部把身份、能力、限制和价格拆成小结构进行校验，但对所有业务消费者只暴露一个 `officialmodel.Resolver`。Agent、消息受理、Worker、工具、图片、Run 和计费不得绕过它直接读取另一份模型配置。

目录示例：

```json
{
  "version": "official_models_v1",
  "official_currency": "USD",
  "billing_currency": "CNY",
  "conversion_policy": "numeric_parity",
  "models": [
    {
      "catalog_vendor": "openai",
      "model_family": "gpt",
      "model_id": "gpt-5.4",
      "aliases": [],
      "lifecycle_status": "active",
      "context_window_tokens": 1050000,
      "max_output_tokens": 128000,
      "context_tier_threshold_tokens": 272000,
      "input_modalities": ["text", "image"],
      "output_modalities": ["text"],
      "supports_streaming": true,
      "supports_tools": true,
      "supports_structured_output": true,
      "supported_parameters": ["temperature"],
      "native_file_input": false,
      "image_input": {
        "mime_types": ["image/jpeg", "image/png", "image/webp", "image/gif"]
      },
      "pricing_profile": "standard_global",
      "rates": [],
      "model_source_url": "https://developers.openai.com/api/docs/models/gpt-5.4",
      "pricing_source_url": "https://developers.openai.com/api/docs/pricing",
      "retrieved_at": "2026-07-27"
    }
  ]
}
```

示例只描述聚合结构。实施时每个值都必须逐项用厂商第一方文档核验，尤其不能把示例中的模态、参数或能力布尔值未经复核直接作为生产事实。

目录的基础字段只通过代码审查和新版本发布修改，后台页面只读。运行时不抓网页、不信任第三方聚合模型表，也不从模型名称前缀猜能力。

### 5.2 人工价格同步是唯一可变层

官方模型唯一信源不等于价格永远不能及时调整。官方基准价仍在版本化目录中，数据库只保存管理员根据厂商官方变价人工同步的当前生效价格：

```text
当前生效价格 = 合法人工价格覆盖 > 官方目录基准价 > 拒绝调用
```

人工价格覆盖只拥有：

- 与官方模型 rate key 完全一致的价格；
- 官方来源 URL；
- 核验日期；
- 乐观锁版本和操作者审计。

它不能覆盖模型身份、别名、生命周期、能力、上下文窗口或最大输出。所有消费者仍只调用 `officialmodel.Resolver`，由该服务返回官方基础事实和当前生效价格，不直接读取覆盖表。

### 5.3 供应商模型严格映射

供应商同步模型时执行确定性匹配：

1. 对渠道 model ID 只做首尾空白清理，不转小写、不模糊匹配；
2. 先匹配唯一 canonical model ID，再匹配唯一受审别名；
3. 匹配结果必须在整个官方目录中唯一；
4. 成功后保存解析出的 canonical model ID 和目录版本；
5. 零匹配或多匹配均标记为 `unmapped`。

管理员不能在 Provider 页面把未映射模型手工指向“看起来相似”的官方模型。需要支持新的渠道别名时，必须先修改官方目录、通过代码审查并发布新版本。

`unmapped` 模型可以在供应商模型列表中用于诊断，但必须满足：

- 不能启用为 Agent 模型；
- 不能创建或切换 Agent；
- 不能接受聊天或生成任务；
- 不能创建付费 Run、冻结余额或派发 Provider attempt。

### 5.4 生命周期

生命周期只由官方目录版本决定，不由 Provider 模型状态反向修改：

- `active`：允许新建 Agent 和调用；
- `deprecated`：不允许新建或切换，已有 Agent 暂时允许调用并持续提示迁移；
- `retired`：所有新调用在付费边界前拒绝，已有 Agent 必须换模型。

供应商模型的启停和健康状态仍是渠道路由事实。同一官方模型可以在供应商 A 不可用、供应商 B 可用；这不会把官方模型自动改成 `retired`。绑定供应商 A 路由的 Agent 不会自动漂移到供应商 B；只有管理员显式切换后，后续 Run 才引用供应商 B 的路由、价格和审计事实。

### 5.5 有效能力只能收窄

聊天页消费的是当前 Agent 的有效能力：

```text
EffectiveCapability =
  OfficialModelCapability
  ∩ TransportCapability
  ∩ ProviderVerifiedCapability
  ∩ AgentPolicy
  ∩ PlatformImplementedCapability
```

| 层 | 负责回答 |
| --- | --- |
| 官方模型 | 模型身份、理论能力和不可修改的官方限制 |
| Transport | 当前 adapter 能否正确表达该能力 |
| Provider 验证 | 当前渠道是否实际接受该能力 |
| Agent 策略 | 当前 Agent 是否允许工具、知识来源等产品能力 |
| 平台实现 | 本系统是否真正实现上传、校验、解析和消息组装 |

后四层只能关闭或收窄官方能力，不能新增官方目录未声明的能力。Provider 模型列表和连接测试不能成为第二能力信源，管理员也不能手工扩大能力。

### 5.6 Agent 策略不拥有模型上限

Agent 只保存 Agent 自身策略，例如：

```text
scenes                     Agent 可用于哪些业务场景
tool bindings               可交给模型的已实现低风险工具
knowledge bindings          允许使用的知识来源
attachment policy           在官方能力范围内进一步收窄附件能力
billing multiplier          Agent 的商业倍率，不是模型基础价
```

Agent 不保存 `max_output_tokens`，也不保存官方模型能力副本。`temperature` 未覆盖时由模型/渠道默认处理；本期不增加 Agent 级温度默认。

### 5.7 有效能力 API

聊天 Agent 列表或会话初始化响应直接返回服务端已求交集的能力，前端不自行拼装：

```json
{
  "agent_id": 157,
  "provider_model_id": 31,
  "official_model_id": "gpt-5.4",
  "official_model": {
    "lifecycle_status": "active",
    "catalog_version": "official_models_v1",
    "context_window_tokens": 1050000,
    "max_output_tokens": 128000
  },
  "capabilities": {
    "input_modalities": ["text", "image"],
    "output_modalities": ["text"],
    "supports_tools": true,
    "supports_streaming": true,
    "runtime_parameters": {
      "temperature": { "supported": false },
      "max_history": { "supported": true, "default": 20, "min": 1, "max": 50, "transitional": true }
    },
    "attachments": {
      "image": {
        "enabled": true,
        "mime_types": ["image/jpeg", "image/png", "image/webp"],
        "max_files": 5,
        "max_file_bytes": 10485760
      },
      "native_file": { "enabled": false }
    }
  }
}
```

`official_model.max_output_tokens` 只用于信息展示和服务端运行，不出现在可编辑 `runtime_parameters` 中。

### 5.8 请求、冻结与快照

消息受理时执行：

1. 根据会话锁定 Agent、Provider model 和自动解析的 official model；
2. 要求 official model 已映射、可计价且生命周期允许调用；
3. 解析有效能力，校验附件、`temperature` 和过渡期 `max_history`；
4. 新请求携带 `max_tokens` 时直接拒绝；
5. 计算最终请求的安全输入上界；
6. 计算安全输出上界；
7. 使用同一输出上界组装 Provider 请求、计算冻结金额并保存 Attempt 报价；
8. 将官方目录版本、canonical model、生命周期、能力、上下文、输出上限和当前生效价格写入不可变 Run 快照。

安全输出上界固定为：

```text
safe_output_upper_bound = min(
  official_model.max_output_tokens,
  official_model.context_window_tokens - safe_input_upper_bound
)
```

若安全输入上界已经达到上下文窗口，或安全输出上界不为正，则在 Provider 派发前拒绝。Adapter 将该系统计算值映射为目标协议的 `max_tokens`、`max_completion_tokens` 或等价字段；它不是用户配置。

付费规则已经确认：

- 每次调用按安全输入上界和安全输出上界冻结最大可能费用；
- 高输出上限模型可能产生较高临时冻结；
- 可用余额不足时不调用上游；
- Provider 返回完整真实 usage 后只捕获实际费用，并释放全部冻结差额；
- 报价、prepared request、Run 快照和最终结算必须引用同一个官方目录版本与 canonical model。

### 5.9 全链路统一命名

“模型定价”一次性改名为“官方模型”，不保留旧路由、旧权限或代码兼容层：

| 层 | 最终命名 |
| --- | --- |
| 菜单 | `官方模型` |
| 前端路径 | `/ai/official-models` |
| view key | `ai/official-models` |
| 菜单 i18n | `menu.ai_official_models` |
| 前端 API | `src/api/ai/official-models.ts` |
| 前端页面 | `src/views/Main/ai/official-models` |
| 页面 i18n | `aiOfficialModel` |
| 后端模块 | `internal/module/ai/officialmodel` |
| Admin API | `/api/admin/v1/ai-official-models` |
| Graph capability | `OfficialModels` |
| 列表权限 | `ai_official_model_list` |
| 价格同步权限 | `ai_official_model_price_sync` |
| 审计模块 | `ai_official_model` |
| 价格覆盖头表 | `ai_official_model_price_overrides` |
| 价格覆盖明细表 | `ai_official_model_price_override_rates` |

API 采用：

```text
GET    /api/admin/v1/ai-official-models/page-init
GET    /api/admin/v1/ai-official-models
GET    /api/admin/v1/ai-official-models/:model_id
PUT    /api/admin/v1/ai-official-models/:model_id/price
DELETE /api/admin/v1/ai-official-models/:model_id/price-override
```

仓库目录文件是基础数据权威，因此不创建 `ai_official_models` 基础表。数据库只保存价格覆盖和自动解析的供应商映射事实。

最终 schema 同时：

- 删除 `ai_agents.max_output_tokens`；
- 从新消息契约删除 `runtime_params.max_tokens`；
- 为供应商模型保存自动解析的 canonical official model ID、目录版本和映射状态；
- 同步更新 HCL、reconciliation、OpenAPI、RBAC seeds、route metadata、菜单、i18n、测试和文档。

本次按全新数据库初始化后的最终结构实施，不提供当前本地数据兼容、旧表双写或旧 API 过渡期。历史迁移与已失效规范可以作为审计记录保留，但所有当前运行代码、当前契约和最终 schema 不得继续使用旧领域名。

## 6. 工具调用修复设计

### 6.1 本期修复范围

本期只保证已实现工具 `admin_user_count` 在真实 Durable Worker 对话中可用，修复范围包括：

1. Worker 创建 `aiToolRepository`；
2. Worker 使用 `DefaultExecutors` 创建 `aiToolService`；
3. Worker 创建 `ChatService` 时注入 `ToolRuntime`；
4. 启动时校验 Worker 的工具运行时完整性；
5. 增加 Worker 装配级测试；
6. 用真实 Agent 绑定执行一次端到端工具调用并核对数据库事实。

本期不实现“后台生成任意代码后自动成为可执行工具”。AI 生成只生成受校验的工具定义草稿；没有服务端 executor 的工具只能保持禁用。

### 6.2 依赖装配规则

API 和 Worker 可以拥有不同的 HTTP 能力，但凡参与聊天运行时的依赖必须使用同一构造边界：

```text
ToolRepository
  -> DefaultExecutors
  -> ToolService
  -> ChatService.ToolRuntime
```

禁止 `ChatService` 在生产 Worker 中把缺失 `ToolRuntime` 当作“没有绑定工具”静默降级。建议规则：

- 无工具功能的明确测试实例可以不注入；
- 生产 Worker 开启 AI Chat 时，`ToolRuntime` 是必需依赖；
- 依赖缺失时 Worker 启动失败，并返回明确错误；
- Agent 没有绑定工具时才合法返回空工具集。

### 6.3 运行流程

```text
加载 Agent 有效工具
  -> 将工具 definitions 写入首次 Provider 请求
  -> 模型返回 tool_calls + 第一轮完整 usage
  -> 校验工具仍绑定、启用、低风险且 executor 已注册
  -> 创建 ai_tool_calls running 事实
  -> 在 timeout 内执行 executor
  -> 保存 success / failed、耗时和结果摘要
  -> 第二次 Provider 调用携带 assistant tool_calls + role=tool 输出
  -> 返回最终答案并按全部成功 attempt 的完整 usage 结算
```

当前 MVP 保留最多一轮工具调用。模型在第二轮继续请求工具时返回稳定错误并正确收尾，不循环调用。

### 6.4 工具失败语义

| 失败点 | 行为 |
| --- | --- |
| 工具未绑定或已禁用 | 拒绝执行，Run 失败 |
| executor 未注册 | 启动期或绑定期阻止；运行时仍 fail closed |
| 参数不是合法 JSON | 写入 failed tool call，不执行 executor |
| 参数不符合 JSON Schema | 写入 failed tool call，不执行 executor |
| 超时 | 标记 tool call failed，Run 失败 |
| executor 返回错误 | 保存稳定错误摘要，Run 失败 |
| 第二轮 Provider 失败 | 保留已发生的工具与 attempt 事实，按既有计费规范收尾 |

工具结果不能包含未声明的个人敏感字段。`admin_user_count` 只返回汇总数量。

### 6.5 测试与验收

必须先增加会失败的装配测试，再修改 Worker：

- Worker AI Chat 依赖图包含非空 `ToolRuntime`；
- 已绑定工具出现在 prepared request 的 `tools`；
- 模型返回工具调用后创建一条 `ai_tool_calls`；
- `admin_user_count` 执行成功并把 `role=tool` 输出发送到第二轮；
- 第二轮回答成功，Run 结算完成；
- 未绑定工具、未注册 executor、非法参数和超时均有确定终态；
- API 和 Worker 对同一 Agent 得到相同的 runtime tool definitions。

生产验收不能只看界面答案，至少核对：

```text
ai_provider_attempts 至少 2 条
第一条 prepared_request_json 包含 tools
ai_tool_calls 至少 1 条且状态 success
第二条 prepared_request_json 包含 assistant tool_calls 与 role=tool
ai_runs 最终 success + settled
```

## 7. 延迟诊断与优化设计

### 7.1 持久化时间点

为每个聊天 Run / Command / Attempt 形成以下时间线：

```text
accepted_at             HTTP 事务成功受理
claimed_at              Worker 获得 Command
prepare_started_at      开始加载上下文和组装请求
provider_dispatched_at  请求真正发往 Provider
first_delta_at          收到第一个可交付文本增量或工具增量
provider_finished_at    收到终态和完整 usage
settled_at              Run / 钱包 / Command 原子收尾
```

由此计算：

```text
accept_latency_ms
queue_latency_ms
prepare_latency_ms
ttft_ms
provider_total_ms
settlement_latency_ms
end_to_end_ms
```

首个 SSE 字节可能只是空 chunk 或 usage 元数据，因此用户体验指标优先记录 `first_delta_at`；底层遥测仍可保留 `first_byte`。

### 7.2 管理端展示

Run 详情增加“耗时分解”，使用时间轴或紧凑表格，不只展示一个 `duration_ms`：

```text
排队 1.22s | 准备 1.49s | 首次输出 16.80s | 上游完成 17.22s | 结算 0.12s
```

同时展示：

- Provider request ID；
- prompt / cache read / completion Token；
- 是否包含工具轮次；
- prepared request 字节数和消息数量；
- 不展示 API Key、完整敏感提示词或完整 Provider 原始响应。

### 7.3 当前可实施的优化顺序

1. 先补观测，区分 TTFT 和完整生成时间；
2. 将固定 1 秒轮询改为可靠唤醒优先、轮询兜底，降低约 0～1 秒排队等待；
3. 测量定价解析、余额冻结、历史加载各自耗时，再针对超过预算的阶段优化；
4. 对同模型、同渠道采样 P50 / P95 / P99，不用单条请求判断渠道质量；
5. 对 prepared request 大小与上游 usage 建立比值异常审计，但不据此重算或篡改上游账单证据。

本期不通过“前端先显示假字”掩盖 TTFT，也不为了更快而绕过 durable 接受、冻结和结算。

## 8. 知识库后续边界

知识库不进入本期工具修复。现有行为只是 best-effort 检索后把文本直接拼到用户问题前，缺少正式的上下文预算和来源契约。

后续应独立设计 `ContextBuilder`：

```text
会话与 Agent 策略
  -> 用户意图 / 查询改写
  -> 多来源检索
  -> 权限过滤
  -> 去重与重排
  -> Token 预算装箱
  -> 引用与来源编号
  -> Prompt / Message 组装
  -> 能力与计费上界校验
  -> Run 审计
```

知识库、历史消息、平台解析文件、系统提示词和工具定义都属于上下文消费者，必须共享一个总 Token 预算，不能各自无限追加。

当前 `max_history` 只作为上下文工程前的过渡控制。`ContextBuilder` 正式接管历史裁剪和 Token 预算时，前后端同时删除该参数，不保留两套上下文策略。

本期明确不做：

- 不把 Worker 缺少的 `KnowledgeRuntime` 简单注入后宣称知识库修复完成；
- 不继续扩展字符串拼接协议；
- 不在工具调用修复中修改检索、切片、向量库或引用展示；
- 不让知识库失败阻断与其无关的工具修复验收。

## 9. 分阶段实施顺序

### 阶段 A：工具恢复

- 修复 Worker `ToolRuntime` 装配；
- 增加生产依赖图测试；
- 验证 `admin_user_count` 完整两轮调用；
- 补充工具调用关键日志和数据库断言。

### 阶段 B：官方模型唯一信源

- 把现有官方价格目录升级为统一 `OfficialModelCatalog`；
- 将 `modelpricing` 全链路一次性重命名为 `officialmodel`；
- 更新菜单、前端路由、Admin API、RBAC、表名、HCL、reconciliation、OpenAPI 和测试；
- 删除 Agent 与消息契约中的可变最大输出；
- 实现供应商模型严格自动映射和生命周期规则；
- 建立唯一 `officialmodel.Resolver` 与有效能力解析；
- 增加 Agent / 会话有效能力 API；
- 后端对附件、参数和工具做权威能力校验；
- 将官方目录版本、能力、限制和生效价格写入 Run 快照；
- 按全新数据库初始化后的最终结构验证，不提供旧命名兼容层。

### 阶段 C：聊天与 Agent 交互

- 重组 Agent 编辑页信息层级；
- 将“模型定价”页面重做为“官方模型”列表和详情抽屉；
- 从 Agent 和聊天界面删除最大输出控件；
- 聊天面板只保留 capability-gated `temperature` 和过渡期 `max_history`；
- 按能力显示参数和附件入口；
- 增加切换 Agent 时的非法附件/覆盖清理流程；
- 用桌面和移动端真实浏览器验证。

### 阶段 D：延迟观测与优化

- 持久化 `first_delta_at` 等关键时间点；
- Run 详情展示耗时分解；
- 建立渠道延迟分位指标；
- 优化 Worker 唤醒和可证明的本地慢阶段。

### 阶段 E：上下文工程

- 单独评审 `ContextBuilder`；
- 再决定知识库、文件解析、历史裁剪、引用和工具上下文如何统一。

阶段 A 可以先于官方模型重构落地，但只能恢复当前已注册工具；新的工具绑定入口和多模态入口必须等待阶段 B 的官方模型能力契约。

## 10. 方案取舍

### 10.1 已选择：官方模型上限唯一且不可调

Agent 和聊天都不保存、不展示、不提交最大输出。每次请求直接按官方模型上限和剩余上下文计算安全输出上界，付费调用按该上界冻结，完成后按真实 usage 结算并释放差额。

### 10.2 已选择：统一官方模型目录

身份、别名、生命周期、能力、上下文、最大输出和官方基准价归属于同一个版本化官方模型条目。内部可以分结构校验，但不能形成多个业务信源。

### 10.3 已选择：价格允许人工同步

官方基础配置全部只读，只有当前生效价格允许管理员根据官方变价人工覆盖。价格覆盖不能改动任何模型能力或限制。

### 10.4 已选择：供应商模型严格自动映射

只接受 canonical ID 或受审别名的唯一匹配，不支持模糊匹配或管理员手工指向。未映射模型不能创建 Agent、调用、冻结或计费。

### 10.5 已选择：全链路一次性改名

菜单、前端、后端模块、API、Graph、表名、RBAC、审计、i18n、契约和测试统一使用“官方模型”，不维护“模型定价”兼容层。最终数据库按重新初始化后的结构验收，不承诺兼容当前本地数据。

### 10.6 已选择：聊天参数面板暂时保留

删除最大输出后，暂时保留 capability-gated `temperature` 和 `max_history`。后者随知识库/RAG 的 `ContextBuilder` 重构一起删除。

### 10.7 已选择：知识库延后重构

工具故障已有独立明确根因；把知识库字符串拼接一并修补会扩大范围，并使未来上下文工程更难收敛。

## 11. 完成标准

本设计全部完成时，应满足：

1. 当前运行代码只有一个官方模型基础解析入口 `officialmodel.Resolver`。
2. Agent 表、DTO 和表单均不存在可变 `max_output_tokens`。
3. 新聊天请求和聊天面板均不存在可变 `max_tokens`；伪造该字段会被拒绝。
4. 请求、冻结和 Attempt 报价使用同一个由官方模型计算的安全输出上界。
5. 余额不足时不会创建 Provider attempt，完成后只按真实完整 usage 扣费并释放差额。
6. 供应商模型未映射、映射歧义、无生效价格或官方模型 `retired` 时均不能付费调用。
7. “官方模型”列表和抽屉完整展示身份、生命周期、能力、限制、价格和事实来源。
8. 除价格同步外，官方模型基础字段在后台全部只读。
9. 当前前后端、Admin API、RBAC、表名、Graph、审计和 i18n 不再使用“模型定价”领域命名。
10. 不支持 `temperature` 的模型不显示也不提交该参数。
11. 不支持图片或文件的模型不显示相应入口，伪造请求也会被后端拒绝。
12. 模型原生文件与平台文本解析在能力和 UI 上被明确区分。
13. Agent 绑定的已实现工具确实出现在 Worker 的最终 Provider 请求中。
14. `admin_user_count` 可以完成调用、执行、二次回传和结算，并留下完整审计事实。
15. Worker 缺少工具运行时会启动失败，不再静默降级。
16. Run 详情可以区分排队、本地准备、TTFT、Provider 总耗时和结算耗时。
17. `hi` 慢时能够用持久化证据判断是本地、队列还是上游问题。
18. `max_history` 明确是过渡能力，并在 `ContextBuilder` 上线时删除。
19. 知识库没有被本期补丁进一步固化，后续可在独立上下文工程下重构。

## 12. 已确认的产品决策

1. 菜单和领域统一叫“官方模型”。
2. 官方模型目录是所有模型基础配置的唯一信源。
3. 基础目录采用仓库内版本化文件，代码审查后发布。
4. 身份、能力、生命周期、上下文和最大输出全部只读。
5. 当前生效价格允许管理员根据官方变价人工同步。
6. Agent 和聊天都删除最大输出配置，调用直接使用官方安全上界。
7. 付费调用按官方安全上界冻结，按真实 usage 结算。
8. 未映射供应商模型不能创建 Agent、调用或计费。
9. 供应商模型只允许 canonical ID / 受审别名自动匹配，不允许手工映射。
10. 生命周期采用 `active / deprecated / retired`，由我们审核后发布。
11. `temperature` 和 `max_history` 暂时保留，后者随上下文工程删除。
12. “模型定价”到“官方模型”一次性完整改名，不保留兼容层。
13. 最终数据库按重新初始化后的结构验收，不兼容当前本地数据。
14. 工具本期只恢复已注册 executor，不实现任意生成代码自动执行。
15. 知识库延后到独立 `ContextBuilder / RAG` 重构。

当前范围没有剩余产品决策。进入实施前按阶段分别编写短小 implementation plan；所有行为修改从失败测试开始。
