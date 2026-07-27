# GPT / Claude 模型定价管理设计

**日期：** 2026-07-27
**状态：** 产品与技术设计已确认，等待 implementation plan
**规范关系：** 本文是 `2026-07-24-ai-chat-consumer-pricing-wallet-design.md` 的规范性补充。涉及模型基础价来源、后台改价、菜单和 RBAC 时以本文为准；钱包、Run 快照、冻结与结算仍以原 Spec 为准。

## 1. 目标

在现有 AI 计费闭环上增加一个独立的“模型定价”管理页面，让管理员可以查看 GPT、Claude 的官方数字基线价，并在官方价格变化时通过受 RBAC 保护的后台操作覆盖当前生效价。

本功能不改变已经确认的两层计价关系：

```text
模型基础价：全局 canonical model 维度
智能体倍率：ai_agents.billing_multiplier_ppm
最终计费：ceil_once(sum(usage × 生效模型基础价) × 智能体倍率)
```

供应商只负责 `base_url + api_key` 调用链路，不拥有价格，也不参与模型价格解析。通过官方 API、Sub2API 或其他专业上游调用同一个 canonical model 时，基础价完全相同。

## 2. 首期范围

首期模型定价页面只管理审核后的文本 Token 模型：

- `catalog_vendor=openai`、`model_family=gpt`；
- `catalog_vendor=anthropic`、`model_family=claude`。

页面只展示官方目录中明确标记为上述两个 family 的模型，不用名称前缀猜 family。现有图片等其他运行时价格若仍被业务引用，继续保持原有只读行为，但不进入本页面，也不能通过本页面覆盖。音频、视频不属于本期。

目录文件升级不得改变仍被生产代码使用的非页面价格语义；GPT/Claude 后台管理是增量能力，不得顺手删除图片等其他合法运行时 rate。已退役且无生产消费者的目录项只能按独立退役计划处理。

首期不允许管理员新增任意模型、修改 canonical model ID、修改厂商、增删价格分类或物理删除官方基线。新 GPT/Claude 模型必须先经过代码审查加入官方目录文件，随后才自动出现在页面。

截至 2026-07-27，首期受审 canonical model 集合固定为：

```text
OpenAI:
gpt-5.6-sol（受审别名 gpt-5.6）
gpt-5.6-terra
gpt-5.6-luna
gpt-5.5
gpt-5.5-pro
gpt-5.4
gpt-5.4-mini
gpt-5.4-nano
gpt-5.4-pro
gpt-4.1（受审别名 gpt-4.1-latest）
gpt-4.1-mini
gpt-4o
gpt-4o-mini

Anthropic:
claude-fable-5
claude-opus-5
claude-sonnet-5
claude-haiku-4-5-20251001（受审别名 claude-haiku-4-5）
claude-opus-4-8
claude-opus-4-7
claude-opus-4-6
claude-sonnet-4-6
claude-sonnet-4-5-20250929（受审别名 claude-sonnet-4-5）
claude-opus-4-5-20251101（受审别名 claude-opus-4-5）
```

Anthropic limited-availability、deprecated、retired 模型不进入首期目录。OpenAI 的四个 GPT-4.x 条目是当前运行时已受审基线，升级目录时必须保留，避免已有智能体失去价格。其他新增集合来自 2026-07-27 的官方直接 API 页面；实施时不得因为聚合供应商返回了相似名称而自动扩展集合。

## 3. 官方数字人民币规则

官方价格数字原样映射成人民币，不做汇率换算：

```text
官方 $5 / 1M input tokens
平台 ¥5 / 1M input tokens
```

数字 `5` 不变，仅将计费货币从 USD 解释为 CNY。系统不得增加 FX 配置、实时汇率、定时换汇或供应商倍率。

官方目录继续作为仓库内、版本化、可审查的 JSON 文件。官方事实来源固定为：

- OpenAI Standard 定价：`https://developers.openai.com/api/docs/pricing`；
- OpenAI 单模型能力和上下文规则：对应的 `https://developers.openai.com/api/docs/models/<model-id>`；
- Anthropic 第一方 API 定价：`https://platform.claude.com/docs/en/about-claude/pricing`；
- Anthropic 第一方 API model ID：`https://platform.claude.com/docs/en/about-claude/models/overview`。

下一版本目录应使用人类可核对的十进制字符串作为唯一价格输入。以下是 2026-07-27 已核对的 `gpt-5.4` 结构示例和真实数字，不是占位数据：

```json
{
  "version": "official_numeric_parity_v3",
  "official_currency": "USD",
  "billing_currency": "CNY",
  "conversion_policy": "numeric_parity",
  "models": [
    {
      "catalog_vendor": "openai",
      "model_family": "gpt",
      "model_id": "gpt-5.4",
      "pricing_profile": "standard_global",
      "max_output_tokens": 128000,
      "context_tier_threshold_tokens": 272000,
      "source_url": "https://developers.openai.com/api/docs/models/gpt-5.4",
      "retrieved_at": "2026-07-27",
      "rates": [
        {
          "category": "input",
          "unit": "token",
          "tier_key": "short_context",
          "price": "2.5",
          "unit_scale": 1000000
        },
        {
          "category": "cache_read",
          "unit": "token",
          "tier_key": "short_context",
          "price": "0.25",
          "unit_scale": 1000000
        },
        {
          "category": "output",
          "unit": "token",
          "tier_key": "short_context",
          "price": "15",
          "unit_scale": 1000000
        },
        {
          "category": "input",
          "unit": "token",
          "tier_key": "long_context",
          "price": "5",
          "unit_scale": 1000000
        },
        {
          "category": "cache_read",
          "unit": "token",
          "tier_key": "long_context",
          "price": "0.5",
          "unit_scale": 1000000
        },
        {
          "category": "output",
          "unit": "token",
          "tier_key": "long_context",
          "price": "22.5",
          "unit_scale": 1000000
        }
      ]
    }
  ]
}
```

正式目录中的每一行必须来自对应厂商的官方公开价格页，并记录真实 URL 和核验日期；无法从官方来源确认完整价格的模型不得录入。来源 URL 必须是 HTTPS，host 只允许 `openai.com`、`anthropic.com`、`claude.com` 及其子域。

加载目录时使用严格十进制解析，将 `price` 精确转换为 `money_units`；禁止先转 `float`。价格最多 8 位小数，必须非负，同一模型至少一个 rate 为正。`unit_scale` 必须为正，首期 rate 的 `unit` 固定为 `token`。目录可为已知限时价格设置 `review_after`；到达该 UTC 日期而目录仍未更新时，该基线 fail closed。`claude-sonnet-5` 的 2026-07-27 优惠价必须设置 `review_after=2026-09-01`，因为官方已公布从该日开始的新价格。

首期只实现厂商第一方直接 API 的普通全局处理价：OpenAI `Standard` 与 Anthropic 标准全局价。Batch、Flex、Priority、Regional/Data Residency、Fast Mode 等不同价格模式不复用普通价；当前产品不开放这些模式，未来一旦开放，必须先扩展目录、请求快照和 usage tier，否则 fail closed。

## 4. 生效价格与覆盖优先级

每次新 Run 接受时按以下顺序解析：

```text
合法数据库覆盖价 > 官方 JSON 基线价 > 无价格并拒绝调用
```

规则如下：

1. requested model ID 先由官方目录解析为唯一 canonical model；数据库覆盖只按 canonical model 查询，不能为未知 provider model 猜测或创建覆盖。数据库没有覆盖记录时，使用官方 JSON 基线。
2. 数据库存在覆盖记录时，必须完整、合法地加载覆盖记录；数据异常时 fail closed，不能悄悄回退官方价。
3. 覆盖必须包含官方基线定义的全部 rate key，且不能增加或删除 rate key。rate key 为 `(category, unit, tier_key)`。
4. 普通输入、输出、缓存读取和缓存写入分别计价；Claude 等模型的多个缓存写入时长使用不同 `tier_key`，不能合并成一个价格。GPT 官方长上下文规则使用 `short_context|long_context` tier：当一次请求去重后的总输入 Token（包含普通输入、缓存读取和缓存写入）超过官方阈值时，该请求的全部 input、cache 和 output 项都使用 long-context rates，不能只给超出部分加价。冻结报价按可能达到的最贵合法 tier 取上界，最终结算按完整终态 usage 确定实际 tier。
5. “恢复官方价”删除该模型的数据库覆盖记录及其明细，随后新 Run 立即使用官方基线。
6. 不增加 Redis 或进程内价格缓存。每个新 Run 从权威目录和数据库解析一次，避免多实例改价后的旧值窗口。
7. Run 接受事务复制 canonical model、价格来源、目录版本或覆盖版本、全部 rates、来源元数据、智能体倍率和输出上限到不可变定价快照。以后改价不能重算历史 Run、账单或流水。
8. 数据库不可用、模型映射不唯一或价格解析失败时，不创建可派发 attempt、不调用上游、不冻结余额。

## 5. 模块边界

新增 `internal/module/ai/modelpricing` 业务模块：

```text
modelpricing
  -> repository：数据库覆盖头与 rate 明细
  -> service：官方基线 + 数据库覆盖的权威解析、完整性校验和并发控制
  -> transport/admin：模型定价 Admin API 与 RBAC route metadata
  -> pricing：复用目录模型、严格金额解析、报价和分摊算法
```

`internal/module/ai/pricing` 继续是无数据库、无 HTTP 的领域组件，只拥有官方文件加载与校验、rate 类型、精确报价和分摊算法。`modelpricing` 拥有可变配置与有效价格解析。聊天、消息、工具、图片和智能体管理等消费者通过一个实际被生产代码和测试替换使用的 `Resolve(ctx, requestedModelID)` 能力依赖 `modelpricing`，不再直接读取全局 `pricing.Default`。

供应商模块、钱包模块和 provider adapter 不依赖 `modelpricing`。`aigateway` 继续只消费 Run 中已闭合的不可变定价快照，不在派发或结算阶段重新查询当前价格。

## 6. 数据模型

新增两张表，不把金融价格塞进无约束 JSON。

### 6.1 `ai_model_price_overrides`

保存某个 canonical model 当前唯一的覆盖头：

| 字段 | 语义 |
| --- | --- |
| `id` | 主键 |
| `catalog_vendor` | `openai` 或 `anthropic`，来自官方目录，不允许请求修改 |
| `model_id` | canonical model ID，来自官方目录，不允许请求修改 |
| `version` | 从 `1` 开始的乐观锁版本，每次成功修改加一 |
| `source_url` | 本次覆盖依据的官方来源 URL |
| `verified_at` | 管理员核对该官方来源的日期 |
| `updated_by` | 最后修改管理员 ID |
| `created_at` / `updated_at` | 审计时间 |

`(catalog_vendor, model_id)` 必须唯一。不存在软删除状态；恢复官方价是带版本条件的真实删除，历史财务事实由 Run 定价快照和操作日志保留。

### 6.2 `ai_model_price_override_rates`

保存覆盖头下的完整 rate 集合：

| 字段 | 语义 |
| --- | --- |
| `id` | 主键 |
| `override_id` | 覆盖头外键；恢复官方价时级联删除 |
| `category` | `input|output|cache_read|cache_write` |
| `unit` | 首期固定 `token` |
| `tier_key` | 无阶梯时为空；有阶梯时使用官方目录定义值 |
| `price_units` | 非负 `money_units` |
| `unit_scale` | 正整数，必须与官方基线对应 rate 一致 |

`(override_id, category, unit, tier_key)` 必须唯一。服务层在同一事务中校验完整集合并替换明细；任何一项失败都不允许留下半套价格。

## 7. 并发与写入语义

列表和详情返回 `override_version`：没有覆盖时为 `0`，存在覆盖时为数据库版本。

- 基线首次覆盖必须提交 `expected_version=0`；事务中只有仍不存在覆盖时才能创建。
- 修改覆盖必须提交当前正整数 `expected_version`；更新使用 `WHERE version = expected_version` 并原子加一。
- 恢复官方价必须提交当前正整数 `expected_version`；删除也带版本条件。
- 版本不匹配统一返回 HTTP `409` 和稳定 machine code，前端刷新当前价格，不覆盖另一位管理员刚保存的结果。

## 8. Admin API

```text
GET    /api/admin/v1/ai-model-prices/page-init
GET    /api/admin/v1/ai-model-prices
GET    /api/admin/v1/ai-model-prices/:model_id
PUT    /api/admin/v1/ai-model-prices/:model_id
DELETE /api/admin/v1/ai-model-prices/:model_id/override?expected_version=N
```

`PUT` 只能提交：

- `expected_version`；
- 完整 rates，其中 rate key 和 `unit_scale` 只读，只有十进制人民币 `price` 可改；
- 官方 `source_url` 与 `verified_at`。

服务端根据路径中的 canonical model ID 找官方基线，忽略并拒绝任何试图覆盖厂商、family、model ID、rate key 或单位的字段。`PUT` 和恢复接口返回 `before` 与 `after` 的闭合价格摘要，供页面立即刷新并让现有 OperationLog 捕获可审计的前后事实。API 不返回内部 `money_units` JSON number，只返回规范人民币十进制字符串。

稳定错误码至少包括：

```text
ai.model_pricing.invalid_override
ai.model_pricing.version_conflict
ai.model_pricing.model_not_found
```

消费者 Run 缺少价格继续使用现有 `ai.billing.price_unavailable`，不能为同一运行时事实另造错误码。

## 9. 菜单与 RBAC

在现有 AI 目录下新增：

```text
名称：模型定价
路径：/ai/model-pricing
view_key：ai/model-pricing
i18n_key：menu.ai_model_pricing
```

只新增两个按钮权限：

```text
ai_model_pricing_list  查看页面、列表和详情
ai_model_pricing_edit  修改覆盖价、恢复官方价
```

所有 `GET` 路由要求 `ai_model_pricing_list`；`PUT` 与 `DELETE` 要求 `ai_model_pricing_edit`。修改和恢复路由必须启用 OperationLog，模块为 `ai_model_pricing`，并捕获模型 ID、操作人、版本及前后价格摘要。

数据库迁移只创建菜单和权限定义，不自动写入 `role_permissions`。管理员在现有角色管理中手动授权。无查看权限时菜单和 API 都不可用；只有查看权限时页面只读且不渲染编辑、恢复按钮。

## 10. 页面与智能体配置联动

“模型定价”页面使用现有后台表格与抽屉组件：

- 支持 `GPT / Claude` family 筛选和 canonical model ID 搜索；
- 表格展示 family、模型 ID、价格来源、官方基线摘要、当前生效价摘要、核验日期和操作；
- `价格来源` 只有 `官方`、`自定义`；
- 多阶梯缓存价格在表格显示数量摘要，在详情或编辑抽屉逐行展示，不能丢失 tier；
- 编辑抽屉只允许修改完整价格和来源元数据，不允许增删行；
- 存在覆盖时提供“恢复官方价”，二次确认后带当前版本提交；
- 桌面端使用紧凑表格，移动端使用全屏抽屉。

智能体配置页继续只修改 `billing_multiplier` 和 `max_output_tokens`，不复制模型基础价字段。原“官方模型定价”区域调整为“当前模型价格”，显示价格来源、基础价、智能体倍率及“倍率后参考单价”。参考单价必须用十进制字符串/整数算法计算，不能使用 JavaScript 二进制浮点；它只用于说明，后端 Run 级一次取整仍是最终计费真相。

## 11. 错误处理

- 模型不在受审 GPT/Claude 页面目录中：管理 API 返回 404；运行时无价格则拒绝 Run。
- 覆盖缺 rate、增加未知 rate、负数、超过 8 位小数、错误单位、重复 rate 或无效官方 URL：返回 400，事务不落库。
- 乐观锁冲突：返回 409，前端刷新当前记录。
- 覆盖存在但数据库明细损坏：运行时 fail closed，不回退官方价。
- 官方目录自身不合法：进程启动失败，不能带错误价格继续服务。
- 任何价格错误都发生在上游派发和钱包冻结前。

## 12. 明确不做

- 不按供应商、Base URL 或 API Key 拆分价格；
- 不做美元兑人民币汇率换算；
- 不增加供应商倍率或供应商模型倍率；
- 不允许运行时抓取官方网页或定时自动更新价格；
- 不允许后台新增任意模型或改 canonical identity；
- 不增加价格缓存、价格审批流、套餐、折扣或会员价；
- 不重算历史 Run，不产生 AI 退款；
- 不把音频、视频纳入本期。

## 13. 短验证边界

实施 Agent 只自动运行预计两分钟内完成的定向检查：

1. 官方目录严格解析、GPT/Claude family 过滤和美元数字到人民币数字一致性；
2. 数据库覆盖优先、完整 rate 校验、恢复官方价和乐观锁冲突；
3. Admin API 的 `list/edit` RBAC 与审计策略；
4. 新 Run 复制生效价并乘智能体倍率，历史快照不受改价影响；
5. 前端 API、页面权限和编辑抽屉的定向测试；
6. Atlas migration 目录校验与 `git diff --check`。

不自动运行 `go test ./...`、race 全套、前端全量测试、完整 build、Docker E2E 或 Playwright。长验证命令只写入实施计划，由用户决定是否手动执行。
