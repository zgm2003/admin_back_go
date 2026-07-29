# AI 运行监控统计分析整改设计

**日期：** 2026-07-29

**状态：** 设计已确认，待书面规格复核

**涉及仓库：** `admin_back_go`、`admin_front_ts`

**规范关系：** 本文补充 `2026-07-28-ai-chat-capability-tools-interaction-design.md` 中的 Run / Attempt 延迟审计设计，并沿用 `2026-07-24-ai-chat-consumer-pricing-wallet-design.md` 的冻结、结算与人民币最小单位规则。若旧的 AI 运行统计接口、成功率、平均耗时或费用展示与本文冲突，以本文为准。

## 1. 背景与结论

当前“AI 助手 -> 运行监控 -> 统计分析”是早期临时统计页，已经不能支撑正式收费后的运行判断和成本核算。现状包括：

1. 页面同时调用概览、延迟、日期和智能体四组接口，不能保证筛选条件和数据时间点一致。
2. `/stats/latency` 不接受页面筛选条件，日期、智能体、渠道和用户筛选对延迟表无效。
3. 成功率把取消、超时和结果未知混在模糊分母中，成功数、失败数和请求数无法解释闭合。
4. 平均耗时会被历史卡死 Run 严重拉高，不能代表用户真实体验。
5. 页面没有基于 `ai_usage_charges` 最终事实的正式费用、释放金额和计费异常指标。
6. 页面没有结构化错误归因、上游渠道可靠性和工具调用可靠性。
7. “最近日期 Top10”只是静态表格，不是可比较的趋势分析。
8. 页面以通用卡片堆叠为主，筛选、健康、成本、异常和归因之间没有清晰层级。

2026-07-29 检查到的 23 条 Run 分布为：

| Run 状态 | 计费事实 | 数量 |
| --- | --- | ---: |
| success | settled | 10 |
| success | legacy_unpriced | 5 |
| failed | released_before_dispatch | 4 |
| failed | legacy_unpriced | 1 |
| timeout | legacy_unpriced | 1 |
| canceled | settled | 1 |
| outcome_unknown | released | 1 |

其中四条旧失败 Run 的平均耗时约为 `585834ms`，最大约为 `1181024ms`，直接污染当前 `114634ms` 的平均耗时。

本文最终选择：

- 将统计页定位为“综合运营驾驶舱”，运行健康与真实费用并列；
- 新增一个统一 Dashboard 查询契约，替换五个旧统计接口；
- 所有指标共享同一筛选、同一 Run 集合和同一数据库快照；
- 实际费用只认最终结算 Charge 的 `actual_units`，前端不估算；
- 成功率排除用户取消和未完成请求；
- 响应速度采用成功请求的 TTFT、完整耗时 P50/P95；
- 运行异常和计费异常拆开呈现；
- 趋势、异常和归因都能下钻回运行列表；
- 通过有界查询、正确索引、固定查询数和 `EXPLAIN ANALYZE` 证明性能。

## 2. 产品目标与范围

### 2.1 首屏必须回答的问题

管理员进入统计页后，应在一个视口内回答：

1. 选定时间范围内有多少 AI 请求，成功交付比例是多少？
2. 用户通常多久看到首个输出，完整请求多久结束？
3. 已最终结算的真实费用是多少？
4. 当前有多少运行异常和计费异常？
5. 异常和费用主要来自哪个官方模型、渠道、智能体、用户或错误类型？
6. 点击任意异常或归因项后，能否直接看到对应 Run 事实？

### 2.2 本期范围

- 重构 AI Run 统计 DTO、Repository、Service、Handler、路由和 OpenAPI 契约。
- 重构统计页的 API、Workflow、组件、图表、状态管理和国际化文本。
- 扩展运行列表的官方模型、计费状态、计费原因和错误码筛选，用于统计下钻。
- 补充统计查询需要且经执行计划验证的数据库索引。
- 删除被统一 Dashboard 替代的旧统计接口和前后端死代码。
- 增加工具调用的可靠性归因页签，但不改变工具执行协议。

### 2.3 非目标

- 不建立独立数据仓库、OLAP 服务或异步统计事实表。
- 不修改官方模型价格、计费公式、冻结或结算流程。
- 不重新估算历史未定价请求的费用。
- 不重构知识库、RAG 或上下文工程；知识检索统计留待对应重构。
- 不新增统计页专属 RBAC 权限或菜单。
- 不修改运行列表和运行详情已有事实的业务含义。

## 3. 统一时间与筛选契约

### 3.1 时间范围

- 默认范围：近 7 个自然日，包含今天。
- 快捷范围：今天、近 7 天、近 30 天、自定义。
- 单次查询最大跨度：90 个自然日。
- 业务时区：`Asia/Shanghai`。
- 前端提交 `YYYY-MM-DD` 的 `date_start` 和 `date_end`。
- 后端把结束日期规范化为下一自然日零点，SQL 始终采用半开区间：

```text
created_at >= start_at AND created_at < end_exclusive
```

- 响应返回规范化后的 `start_at`、`end_exclusive`、业务时区和 `generated_at`。

日期范围按 `ai_runs.created_at` 归属。费用表达的是“该时间范围内创建的 Run 最终产生的实际费用”，不是钱包流水按入账时间形成的财务账期。钱包日账核对仍以钱包流水为准。

### 3.2 统一筛选

Dashboard 支持以下可选筛选：

| 参数 | 含义 | 事实来源 |
| --- | --- | --- |
| `platform` | 请求平台 | `ai_runs.platform` |
| `agent_id` | 智能体 | `ai_runs.agent_id` |
| `model_id` | 官方模型 canonical ID | Run 的 `model_id` 快照 |
| `provider_id` | 实际调用渠道 | `ai_runs.provider_id` |
| `user_id` | 请求用户 | `ai_runs.user_id` |

官方模型筛选和归因不连接当前启用状态来改写历史。已停用或已改名模型仍按 Run 中持久化的 `model_id`、`model_display_name` 展示，并标记为历史模型。筛选选项由当前官方目录与时间范围内出现过的历史模型合并产生。

### 3.3 一致性快照

Repository 暴露一个 Dashboard 聚合入口，在同一只读一致性事务中完成固定数量查询。所有子结果使用同一个：

- 规范化时间范围；
- 筛选条件；
- `generated_at`；
- Run 基础集合；
- 事务快照。

禁止 Service 分别调用多个无事务统计方法后拼接响应。一个子查询失败时整个接口失败，不发布半套统计。

## 4. 指标口径

### 4.1 请求与状态

对于筛选后的 Run 集合：

```text
total_runs = 所有匹配 Run
in_progress_runs = status = running
terminal_runs = status IN (success, failed, canceled, timeout, outcome_unknown)
delivery_denominator = success + failed + timeout + outcome_unknown
success_rate = success / delivery_denominator * 100
```

规则：

- `canceled` 表示用户主动取消，不进入成功率分母。
- `running` 尚未形成交付结论，不进入成功率分母。
- 分母为 0 时返回 `0`，同时返回分子和分母，前端不自行猜测。
- 成功率由整数计数在 Service 中计算并保留两位小数，避免数据库浮点差异。
- 状态分布百分比使用 `total_runs` 作为分母，因此它与成功率分母不同；响应必须同时返回原始数量。

### 4.2 Token

Token 指标只汇总 Run 已持久化的上游用量：

- `prompt_tokens`
- `completion_tokens`
- `total_tokens`

不根据消息字数或附件大小推算。没有完整用量的 Run 保持已持久化值，并通过计费异常体现，不能在统计层补造数据。

### 4.3 性能

核心性能指标：

| 指标 | 样本 | 计算事实 |
| --- | --- | --- |
| TTFT | 成功 Run 的成功 Attempt | `first_delta_at - dispatched_at` |
| 完整耗时 | 成功 Run | 持久化的 `ai_runs.duration_ms` |

每项返回：

- `sample_count`
- `insufficient_sample`
- `p50_ms`
- `p95_ms`

采用 nearest-rank 百分位算法。样本数少于 20 时 `insufficient_sample = true`；后端仍可返回计算值用于审计，但前端显示“样本不足（N）”，不显示伪精确 P95。

失败、超时、取消和结果未知不混入核心性能分布。它们的最长耗时和数量只出现在运行异常诊断中。

### 4.4 实际费用与释放金额

金额统计使用整数单位完成聚合，最后统一调用 `shared/money.FormatRMBUnits` 转换为十进制字符串。前端只添加货币展示，不参与计算。

实际费用必须同时满足：

```text
ai_runs.billing_status = settled
ai_usage_charges.status = settled
ai_usage_charges.finalized_at IS NOT NULL
```

```text
actual_amount_units = SUM(ai_usage_charges.actual_units)
```

以下事实不计入实际费用：

- 预冻结金额；
- 已释放金额；
- 派发前释放；
- 余额不足释放；
- 上游失败或结果未知释放；
- `unbilled_usage_incomplete`；
- `unbilled_over_hold`；
- `legacy_unpriced`。

释放金额使用 Charge 持久化的 `held_units` 作为审计信息单独展示，不与实际费用相减，也不称为退款。

### 4.5 运行异常

运行异常分类互斥：

| 分类 | 条件 |
| --- | --- |
| `failed` | `status = failed` |
| `timeout` | `status = timeout` |
| `outcome_unknown` | `status = outcome_unknown` |
| `stale_running` | `status = running` 且 `started_at < generated_at - 15m` |

`run_anomaly_count` 是以上分类数量之和。取消不算运行异常。15 分钟与当前 `DefaultAIRunStaleTimeout` 保持同一常量来源，不能在统计模块另写魔法值。

### 4.6 计费异常

正确释放是正常结算结果，不自动视为计费异常。计费异常采用互斥优先级，一个 Run 只进入最高优先级分类，确保总数等于分类之和：

1. `state_inconsistent`：Run 与 Charge 缺失、终态或状态不一致，或已结算 Charge 缺少 `finalized_at`。
2. `open_overdue`：终态 Run 仍处于 `pending/held/open`，或运行超过统一 15 分钟时限仍有开放 Charge。
3. `pricing_snapshot_missing`：应按正式价格结算的非历史 Run 缺少有效价格快照。
4. `legacy_unpriced`：历史数据没有可追溯价格。
5. `unbilled_usage_incomplete`：上游未返回完整用量，未扣费。
6. `unbilled_over_hold`：实际用量超过权威冻结上限，未扣费。

`billing_anomaly_count` 是以上互斥分类数量之和。详细的 Charge Item 求和不变量继续由运行详情和结算域验证，Dashboard 不读取大 JSON 或重算价格。

### 4.7 错误与工具归因

错误归因优先采用最终失败 Attempt 的 `ai_provider_attempts.error_code`。一个 Run 有重试时：

- 最终成功的 Run 不进入运行错误分类；
- 最终失败、超时或结果未知的 Run 使用最后一个终态 Attempt 的结构化错误码；
- 没有结构化错误码时进入 `unclassified`；
- 禁止按 `error_message` 文案做模糊分组。

工具归因使用 `ai_tool_calls` 的持久化状态：

```text
tool_denominator = success + failed + timeout
tool_success_rate = success / tool_denominator * 100
```

`running` 不进入工具成功率。工具耗时 P50/P95 只统计成功调用；工具名称和编码使用调用时快照。

## 5. 统一 API 契约

### 5.1 路由与权限

新增：

```http
GET /api/admin/v1/ai-runs/dashboard
```

继续使用：

```text
permission = ai_run_list
audit = read-only
```

同一改动删除：

```text
GET /api/admin/v1/ai-runs/stats
GET /api/admin/v1/ai-runs/stats/latency
GET /api/admin/v1/ai-runs/stats/by-date
GET /api/admin/v1/ai-runs/stats/by-agent
GET /api/admin/v1/ai-runs/stats/by-user
```

这是一次性内部契约替换，不保留双写、兼容适配器或废弃接口。菜单和 RBAC 权限编码不变。

### 5.2 响应结构

响应至少包含：

```json
{
  "generated_at": "2026-07-29T15:42:18+08:00",
  "timezone": "Asia/Shanghai",
  "date_range": {
    "start_at": "2026-07-23T00:00:00+08:00",
    "end_exclusive": "2026-07-30T00:00:00+08:00"
  },
  "summary": {
    "total_runs": 23,
    "terminal_runs": 23,
    "in_progress_runs": 0,
    "success_runs": 15,
    "failed_runs": 5,
    "timeout_runs": 1,
    "outcome_unknown_runs": 1,
    "canceled_runs": 1,
    "success_denominator": 22,
    "success_rate": 68.18,
    "prompt_tokens": 76000,
    "completion_tokens": 11800,
    "total_tokens": 87800
  },
  "performance": {
    "ttft": {
      "sample_count": 15,
      "insufficient_sample": true,
      "p50_ms": 1210,
      "p95_ms": 2840
    },
    "end_to_end": {
      "sample_count": 15,
      "insufficient_sample": true,
      "p50_ms": 18400,
      "p95_ms": 41700
    }
  },
  "billing": {
    "settled_runs": 11,
    "actual_amount": "18.42",
    "released_runs": 5,
    "released_amount": "3.8",
    "unbilled_runs": 7
  },
  "anomalies": {
    "run_total": 7,
    "billing_total": 7,
    "run_items": [
      { "code": "failed", "count": 5 },
      { "code": "timeout", "count": 1 },
      { "code": "outcome_unknown", "count": 1 },
      { "code": "stale_running", "count": 0 }
    ],
    "billing_items": [
      { "code": "legacy_unpriced", "count": 7 }
    ]
  },
  "trend": [],
  "breakdowns": {
    "models": [],
    "providers": [],
    "agents": [],
    "users": [],
    "errors": [],
    "tools": []
  }
}
```

示例数字只用于说明字段关系，不是数据库最终结果。所有数组在无数据时返回 `[]`，对象返回完整零值结构，禁止返回 `null`。

金额字段使用十进制字符串。趋势和归因中的成功率、金额、性能分布复用本章口径，不定义第二套算法。

## 6. 后端架构与查询

### 6.1 责任边界

```text
admin transport
  -> 绑定并校验日期与筛选参数
run Service
  -> 规范化时间、生成 generated_at、计算派生比例和格式化金额
Dashboard Repository
  -> 在一致性事务内执行固定聚合查询
MySQL persisted facts
  -> ai_runs / ai_usage_charges / ai_provider_attempts / ai_tool_calls
```

Repository 返回整数计数、整数金额单位和毫秒值。Service 负责：

- 成功率和工具成功率；
- 金额格式化；
- 样本不足标记；
- 日期和时区输出；
- 非空集合归一化。

### 6.2 查询预算

一次 Dashboard 请求最多执行六条固定查询：

1. Run 状态、Token、计费和异常概览；
2. TTFT 与完整耗时分位值；
3. 时间趋势；
4. 官方模型、渠道、智能体和用户归因；
5. 结构化错误归因；
6. 工具调用归因。

查询数量不随结果行数增长。禁止逐个模型、用户或 Run 发起补充查询。

P50/P95 使用 MySQL 窗口排序和 nearest-rank 位置计算，不再读取任意“最近 10000 条”到 Go 内存中近似统计。查询只读取所需标量列，不读取 `input_snapshot`、`pricing_snapshot_json`、`prepared_request_json`、`usage_json`、工具参数或结果 JSON。

趋势最多返回 90 个日桶。每个归因维度最多返回前 20 项，默认按实际费用降序，金额相同再按请求数和稳定键排序。查看完整集合统一进入运行列表。

### 6.3 索引设计

复用已有索引：

```text
ai_runs(created_at, id)
ai_runs(agent_id, created_at, id)
ai_runs(provider_id, created_at, id)
ai_runs(user_id, created_at, id)
ai_runs(status, created_at, id)
ai_usage_charges UNIQUE(run_id)
ai_provider_attempts UNIQUE(run_id, attempt_no)
ai_tool_calls(run_id, id)
```

为新增一等筛选准备以下候选索引：

```text
ai_runs(model_id, created_at, id)
ai_runs(platform, created_at, id)
ai_runs(billing_status, billing_reason, created_at, id)
ai_provider_attempts(error_code, run_id, id)
```

候选索引必须在代表性数据上执行 `EXPLAIN ANALYZE`。只有执行计划证明减少扫描行数且命中对应查询时才写入 `database/schema/admin.hcl` 和 Atlas migration。不得为了“可能有用”堆叠重复或低收益索引。

### 6.4 首期不缓存

首期不使用 Redis 缓存统计响应，原因是：

- 页面需要反映刚完成的 Run 和最终结算；
- 统一接口已经把五个 HTTP 请求收敛为一个；
- 当前首先需要证明 SQL 和索引正确；
- 缓存会引入按筛选组合失效和数据陈旧问题。

当单次 90 天查询在正确索引和有界返回下仍不能满足预算时，再以真实执行计划决定引入日聚合事实表，而不是在本期提前建设数据仓库。

## 7. 前端信息架构与交互

### 7.1 页面层级

页面采用单层、紧凑、适合重复扫描的后台工作台：

1. 统一筛选栏：快捷日期、日期范围、官方模型、渠道、智能体、用户、查询和重置。
2. 核心指标带：请求数、成功率、实际费用、TTFT P95、运行异常、计费异常。
3. 趋势区：运行、费用、性能三个页签。
4. 运行状态分布：展示所有状态数量和占总请求比例。
5. 双异常诊断：运行异常与计费异常并列。
6. 归因表：官方模型、渠道、智能体、用户、错误类型和工具调用页签。

页面不使用营销式大卡片，不嵌套卡片。指标带通过分隔线形成稳定网格，趋势、诊断和归因是并列页面区域。

### 7.2 组件边界

`RunStats/index.vue` 只负责组合和页面生命周期，拆分为职责明确的组件：

- `RunDashboardFilters`：筛选模型与查询事件；
- `RunDashboardSummary`：核心指标和状态分布；
- `RunDashboardTrend`：运行、费用和性能趋势；
- `RunDashboardDiagnostics`：双异常诊断；
- `RunDashboardBreakdowns`：归因页签与 AppTable；
- `dashboard-presenter.ts`：金额、百分比、耗时和下钻参数纯函数。

趋势图使用按需引入的 ECharts 柱线图模块和 Canvas renderer，不手写图表算法。组件在卸载时销毁实例，并使用 `ResizeObserver` 响应容器尺寸。

归因表使用现有 `AppTable` 默认对齐行为，不添加重复的居中 CSS。只编写页面布局、稳定尺寸和响应式所必需的样式。

### 7.3 加载与实时刷新

- 首次进入和点击查询时请求 Dashboard。
- 查询期间保留上一份成功数据，局部显示刷新状态，不清空整页。
- 用户快速切换筛选时取消旧请求，只允许最后一次请求提交。
- 页面处于当前实时范围时，收到完成、失败或取消事件后合并防抖刷新。
- 自定义历史区间不包含今天时，不因实时事件刷新。
- 不增加固定轮询。
- 页面展示 `generated_at`；刷新失败时标记“数据未更新”，保留最后成功时间。

### 7.4 下钻

点击以下元素跳转运行列表，并保留日期、平台、智能体、官方模型、渠道和用户筛选：

- 状态数量 -> `status`
- 运行异常 -> `status` 或 stale 条件
- 计费异常 -> `billing_status + billing_reason`
- 官方模型、渠道、智能体、用户行 -> 对应维度 ID
- 错误类型 -> `error_code`

运行列表新增 `model_id`、`billing_status`、`billing_reason`、`error_code` 查询参数和搜索项。参数由纯函数生成并测试，禁止通过显示文本反向推导 ID。

## 8. 异常处理

### 8.1 请求校验

以下情况返回稳定 `400`：

- 日期格式非法；
- 开始日期晚于结束日期；
- 范围超过 90 天；
- 筛选 ID 非正整数；
- 平台或枚举值不受支持。

### 8.2 查询与不变量错误

- 任一 Repository 查询失败，事务终止并返回统一 Dashboard 查询错误。
- 金额为负、溢出或无法格式化时返回内部不变量错误，禁止截断或返回估算值。
- 结算状态冲突作为计费异常统计；如果冲突导致无法确定实际费用，则该 Charge 不进入实际费用。
- 日志记录 request ID、规范化时间范围、筛选摘要、失败阶段、耗时和数据库错误，不记录消息内容或大 JSON。
- API 响应使用稳定安全错误包络，不泄露 SQL。

### 8.3 前端失败状态

- 首次加载失败：展示明确错误态和重试命令。
- 已有数据刷新失败：保留数据，标记更新时间失效并允许重试。
- 无数据：展示完整空态，不渲染虚假 `0ms` 或 `NaN%`。
- 样本不足：展示“样本不足（N）”，不使用 `0ms` 伪装真实性能。
- 单个图表渲染失败不能破坏筛选和其他数据区，但接口响应不允许部分成功。

## 9. 契约与删除策略

统一接口落地时同步完成：

- 删除后端旧 Stats DTO、Repository 方法、Service 方法、Handler、路由和测试替身；
- 删除 OpenAPI 中五个旧路径及旧响应 Schema；
- 添加 Dashboard 路径、查询参数、完整非空响应 Schema；
- 重新生成 `contracts/admin/v1/openapi.json` 和前端 generated contract；
- 删除前端 `stats`、`latencyStats`、`statsByDate`、`statsByAgent`、`statsByUser` API 与 Workflow 资源；
- 删除 `RunLatencyStatsTable.vue` 和旧统计页面死代码；
- 更新权限、runtime route、bundle 和 browser-only 契约测试；
- RBAC 权限仍为 `ai_run_list`，不要求重新挂载菜单权限。

不保留旧接口兼容期。前后端与契约必须在同一实施批次切换。

## 10. 测试与性能验收

### 10.1 后端正确性

必须覆盖：

- success、failed、timeout、canceled、outcome_unknown、running 状态矩阵；
- 取消和处理中不进入成功率分母；
- 分母为零；
- 只有一致的 settled Charge 进入实际费用；
- released、unbilled、legacy_unpriced 不进入实际费用；
- 运行异常和计费异常互斥分类与总数闭合；
- TTFT 和完整耗时只采用成功样本；
- nearest-rank P50/P95、零样本、19 条和 20 条样本边界；
- `Asia/Shanghai` 日期边界和结束日半开区间；
- 所有筛选及其组合；
- 多 Attempt 重试后的最终错误归因；
- 工具成功率和耗时样本；
- 空数据返回非空对象和空数组；
- 任一子查询失败时整体失败；
- 金额格式化和不变量错误。

### 10.2 契约与数据库

- Handler 参数绑定和错误响应测试；
- Runtime route、权限和 OpenAPI 覆盖测试；
- 前后端生成契约同步测试；
- Schema 与 Atlas migration 测试；
- SQL 关联条件、固定查询数量和禁止大 JSON 列测试；
- 候选索引的 before/after `EXPLAIN ANALYZE` 证据。

### 10.3 前端

- API 查询序列化和完整响应解码；
- 零值、空数组和金额字符串；
- 指标 Presenter、成功率说明、耗时和样本不足；
- 查询取消与只提交最后响应；
- 实时事件合并防抖和历史范围不刷新；
- 初次失败、刷新失败、空态和恢复；
- 所有下钻元素生成正确运行列表筛选；
- ECharts 生命周期和容器 resize；
- AppTable 使用默认对齐，不依赖额外居中样式。

按项目约定执行 Vitest、`npm run typecheck` 和正式构建，不使用 Playwright。

### 10.4 性能预算

在构造的 10 万条 Run、90 天匹配范围上验证：

- 单次请求最多六条固定数据库查询；
- 无 N+1；
- 无范围外全表扫描；
- 无大 JSON 列读取；
- 归因列表每类最多 20 行；
- 趋势最多 90 个时间桶；
- 开发机热查询目标小于 `500ms`；
- 超过现有 `200ms` 慢 SQL 阈值时能够定位到具体子查询。

时间目标作为性能工程验收记录，不作为容易受 CI 机器抖动影响的硬编码单元测试。结构性约束、查询数和执行计划必须自动或可重复验证。

## 11. 验收清单

- [ ] 一个 Dashboard 请求替代五个旧统计请求。
- [ ] 所有筛选同时作用于概览、性能、费用、异常、趋势和归因。
- [ ] 成功率符合已确认公式并显示分子、分母。
- [ ] 实际费用只来自最终结算 `actual_units`。
- [ ] TTFT 和完整耗时采用成功请求 P50/P95，不再展示混合平均值。
- [ ] 运行异常与计费异常分开且分类数量闭合。
- [ ] 官方模型、渠道、智能体、用户、错误和工具均可归因。
- [ ] 指标、状态、异常和归因均能下钻运行列表。
- [ ] 空数据、刷新失败和样本不足不会产生误导数字。
- [ ] 旧接口、旧契约、旧组件和死代码一次性删除。
- [ ] 候选索引有 `EXPLAIN ANALYZE` 证据。
- [ ] 后端测试、前端测试、类型检查和正式构建通过。
