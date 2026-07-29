# AI 运行统计分析驾驶舱实施计划

> **给实施代理：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按 Task 逐项执行；使用复选框（`- [ ]`）跟踪步骤。

**目标：** 将“AI 助手 -> 运行监控 -> 统计分析”改造成口径统一、费用可信、异常可下钻且在 10 万 Run/90 天范围内可验证性能的综合运营驾驶舱。

**架构：** 后端以 `GET /api/admin/v1/ai-runs/dashboard` 作为唯一统计契约，在同一只读一致性事务中执行最多六条有界聚合查询；Service 统一日期、比例、金额和非空响应，列表接口承接精确下钻。前端只维护一个 Dashboard 资源，以纯 Presenter 生成展示值和运行列表筛选，ECharts 按需加载，归因表复用 `AppTable` 默认居中能力。

**技术栈：** Go 1.26.5、Gin、GORM、MySQL 8、Atlas、OpenAPI Admin Contract、Vue 3、TypeScript、Element Plus、ECharts、Vitest。

**设计规格：** `docs/superpowers/specs/2026-07-29-ai-run-analytics-dashboard-design.md`

---

## 执行约束

- 后端仓库：`E:/admin/admin_back_go`；前端仓库：`E:/admin/admin_front_ts`。
- 前端现有未跟踪目录 `E:/admin/admin_front_ts/.superpowers/` 属于用户文件，不提交、不删除、不回退。
- 不使用 Playwright；验收使用 Go 测试、Vitest、类型检查、契约检查、正式构建和数据库执行计划。
- 实际费用只来自满足结算不变量的 `ai_usage_charges.actual_units`；前端不估算、不补算历史价格。
- Dashboard 数组始终编码为 `[]`，聚合对象始终返回完整零值结构，禁止 `null`。
- 旧五个统计接口与前后端死代码在同一发布批次删除，不保留兼容层。
- `ai_run_list` 继续作为唯一管理权限，不新增菜单、按钮或 RBAC 编码。
- `AppTable` 已在组件内部设置列和表头居中；归因表禁止增加 `text-align: center`、`justify-content: center` 或列级 `align` 重复配置。
- 每个 Task 先写定向失败测试，再写最小实现；每次提交前执行对应测试与 `git diff --check`。

## 文件责任图

### 后端新增

| 文件 | 单一责任 |
| --- | --- |
| `internal/module/ai/run/dashboard_dto.go` | Dashboard HTTP DTO、Repository 原始聚合行和筛选类型 |
| `internal/module/ai/run/dashboard.go` | 日期规范化、派生比例、金额格式化、非空响应投影 |
| `internal/module/ai/run/dashboard_test.go` | Dashboard Service 口径与边界测试 |
| `internal/module/ai/run/dashboard_repository.go` | 一致性事务、六条聚合 SQL、共享异常分类表达式 |
| `internal/module/ai/run/dashboard_repository_test.go` | SQL 语义、固定查询数、事务失败和禁止大 JSON 测试 |
| `internal/module/ai/run/transport/admin/dashboard_handler_test.go` | Dashboard 参数绑定、响应和旧路由移除测试 |
| `internal/architecture/ai_run_dashboard_schema_test.go` | 已证明索引、迁移与 HCL 一致性测试 |
| `database/migrations/202607290101_ai_run_dashboard_indexes.sql` | 仅承载执行计划证明有效的新索引 |

### 后端修改

| 文件 | 改动 |
| --- | --- |
| `internal/module/ai/run/dto.go` | 删除旧 Stats 类型/接口；扩展 PageInit、Run List 与 Repository/HTTPService 接口 |
| `internal/module/ai/run/repository.go` | 删除旧统计查询；扩展模型选项和 Run List 精确筛选 |
| `internal/module/ai/run/repository_test.go` | 删除旧统计 SQL 测试；增加列表最终错误码和计费筛选测试 |
| `internal/module/ai/run/service.go` | 删除旧统计 Service；合并当前官方目录和历史模型选项 |
| `internal/module/ai/run/service_test.go` | 更新 fake repository；增加 PageInit/Run List 新事实测试 |
| `internal/module/ai/run/transport/admin/request.go` | 新 Dashboard 请求；扩展 Run List 参数；补 `outcome_unknown` |
| `internal/module/ai/run/transport/admin/handler.go` | 单一 Dashboard handler；删除五个旧 handler |
| `internal/module/ai/run/transport/admin/route.go` | 注册 `/dashboard` 并删除旧统计路由 |
| `internal/module/ai/run/transport/admin/handler_test.go` | 删除旧延迟 handler 测试 |
| `internal/module/ai/run/transport/admin/feedback_handler_test.go` | 更新 `HTTPService` 测试替身 |
| `internal/platform/admin/build.go` | 为 Run Service 注入平台 logger |
| `internal/admincontract/openapi_ai_schemas.go` | Dashboard 完整非空 Schema 与扩展列表 Schema |
| `internal/admincontract/openapi_workflows.go` | Dashboard 和列表查询参数；删除旧操作 |
| `internal/admincontract/openapi_test.go` | 新路径/参数/Schema 测试与旧路径缺席测试 |
| `internal/admincontract/permissions_test.go` | Dashboard 复用 `ai_run_list` |
| `internal/admincontract/bundle_test.go` | Bundle 中只发布新统计路径 |
| `internal/server/testdata/admin_routes_golden.txt` | Runtime 路由快照 |
| `internal/server/testdata/admin_route_policy_golden.json` | Runtime 权限策略快照 |
| `database/reconciliation/20260729_ai_run_dashboard_query_candidates.json` | 本次四组候选索引的独立 before/after 证据查询 |
| `database/schema/admin.hcl` | 仅追加被证据接受的索引 |
| `database/migrations/atlas.sum` | Atlas migration hash |
| `contracts/admin/v1/*` | 由后端契约生成命令重建，禁止手改 |

### 前端新增

| 文件 | 单一责任 |
| --- | --- |
| `src/views/Main/ai/runs/components/RunStats/RunDashboardFilters.vue` | 统一筛选栏与查询/重置事件 |
| `src/views/Main/ai/runs/components/RunStats/RunDashboardSummary.vue` | 核心指标带与状态分布 |
| `src/views/Main/ai/runs/components/RunStats/RunDashboardTrend.vue` | 三类趋势页签和图表生命周期 |
| `src/views/Main/ai/runs/components/RunStats/RunDashboardDiagnostics.vue` | 运行异常与计费异常并列诊断 |
| `src/views/Main/ai/runs/components/RunStats/RunDashboardBreakdowns.vue` | 六类归因页签和 `AppTable` |
| `src/views/Main/ai/runs/components/RunStats/dashboard-presenter.ts` | 格式化、默认日期和下钻参数纯函数 |
| `src/views/Main/ai/runs/components/RunStats/dashboard-chart.ts` | ECharts 按需加载和 option 纯构造 |
| `tests/helpers/ai-run-dashboard.ts` | 完整、可覆盖的 Dashboard 响应 fixture |
| `tests/shared/ai/ai-run-dashboard-presenter.test.ts` | Presenter 与下钻参数测试 |
| `tests/component/ai/RunDashboard.test.ts` | Dashboard 组件状态、AppTable 和图表生命周期测试 |

### 前端修改/删除

| 文件 | 改动 |
| --- | --- |
| `src/api/ai/runs.ts` | 单一 Dashboard API；扩展 Run List；删除五组旧统计 API |
| `src/features/ai-runs/workflow.ts` | 单一 Dashboard 资源、最后请求获胜、保留旧数据和实时防抖刷新 |
| `src/views/Main/ai/runs/index.vue` | 统计下钻切换到列表并同步 URL query |
| `src/views/Main/ai/runs/components/RunStats/index.vue` | 只负责页面组合和生命周期 |
| `src/views/Main/ai/runs/components/RunStats/styles.css` | 单层工作台布局、稳定图表尺寸和响应式规则 |
| `src/views/Main/ai/runs/components/RunList/index.vue` | 接收 URL 下钻参数并展示新增筛选 |
| `src/i18n/locales/zh-CN/ai.ts` | 中文驾驶舱文案 |
| `src/i18n/locales/en-US/ai.ts` | 同构英文键 |
| `tests/shared/ai/ai-run-api.test.ts` | 新 API 解码和查询序列化 |
| `tests/integration/features/ai-runs.test.ts` | Workflow 并发、失败保留和实时刷新 |
| `tests/component/ai/RunLatencyBreakdown.test.ts` | 只保留 Run 详情耗时分解测试 |
| `package.json`、`package-lock.json` | 增加 ECharts 依赖 |
| `contracts/backend/admin/v1/*` | 由契约同步命令生成，禁止手改 |
| `src/modules/http/generated/admin.ts` | 由 `contract:generate` 生成 |
| `src/modules/http/generated/operations.ts` | 由 `contract:generate` 生成 |
| `src/i18n/locales/generated.ts` | 由 `locale:generate` 生成 |
| Delete `src/views/Main/ai/runs/components/RunStats/RunLatencyStatsTable.vue` | 删除被 Dashboard 性能区替代的旧组件 |

## Task 1：建立 Dashboard 领域契约与统一口径

**Files:**
- Create: `internal/module/ai/run/dashboard_dto.go`
- Create: `internal/module/ai/run/dashboard.go`
- Create: `internal/module/ai/run/dashboard_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/service_test.go`

- [ ] **Step 1：先写日期、比例、金额和非空集合失败测试**

在 `dashboard_test.go` 增加以下测试：

```go
func TestDashboardDefaultsToSevenShanghaiCalendarDays(t *testing.T)
func TestDashboardRejectsPartialInvalidReversedAndOverNinetyDayRanges(t *testing.T)
func TestDashboardSuccessRateExcludesRunningAndCanceled(t *testing.T)
func TestDashboardFormatsOnlySettledActualUnits(t *testing.T)
func TestDashboardReturnsCompleteZeroObjectsAndEmptyArrays(t *testing.T)
func TestDashboardMarksNineteenSamplesInsufficientAndTwentySufficient(t *testing.T)
func TestDashboardFailsOnNegativeOrOverflowedMoneyFacts(t *testing.T)
```

固定时钟使用 `2026-07-29T15:42:18+08:00`。默认规范化结果必须是：

```text
start_at       = 2026-07-23T00:00:00+08:00
end_exclusive = 2026-07-30T00:00:00+08:00
stale_before  = generated_at - config.DefaultAIRunStaleTimeout
```

成功率样本固定为 `success=7, failed=2, timeout=1, outcome_unknown=1, canceled=3, running=4`，断言分母为 `11`、成功率为 `63.64`，而不是按总 Run 计算。

- [ ] **Step 2：运行测试并确认失败原因**

```powershell
go test ./internal/module/ai/run -run 'TestDashboard' -count=1
```

预期：FAIL，`DashboardFilter`、`DashboardResponse` 和 `Service.Dashboard` 尚不存在。

- [ ] **Step 3：定义唯一字段名和方法签名**

在 `dashboard_dto.go` 定义并在后续 Task 保持完全一致：

```go
type DashboardFilter struct {
    RequestID string
    DateStart string
    DateEnd string
    Platform string
    ModelID string
    AgentID *int64
    ProviderID *int64
    UserID *int64
}

type DashboardQuery struct {
    StartAt time.Time
    EndExclusive time.Time
    GeneratedAt time.Time
    StaleBefore time.Time
    Platform string
    ModelID string
    AgentID *int64
    ProviderID *int64
    UserID *int64
}

type DashboardResponse struct {
    GeneratedAt string `json:"generated_at"`
    Timezone string `json:"timezone"`
    DateRange DashboardDateRange `json:"date_range"`
    Summary DashboardSummary `json:"summary"`
    Performance DashboardPerformance `json:"performance"`
    Billing DashboardBilling `json:"billing"`
    Anomalies DashboardAnomalies `json:"anomalies"`
    Trend []DashboardTrendItem `json:"trend"`
    Breakdowns DashboardBreakdowns `json:"breakdowns"`
}

type DashboardPercentile struct {
    SampleCount int64 `json:"sample_count"`
    InsufficientSample bool `json:"insufficient_sample"`
    P50MS int64 `json:"p50_ms"`
    P95MS int64 `json:"p95_ms"`
}

type DashboardDateRange struct {
    StartAt string `json:"start_at"`
    EndExclusive string `json:"end_exclusive"`
}

type DashboardSummary struct {
    TotalRuns int64 `json:"total_runs"`
    TerminalRuns int64 `json:"terminal_runs"`
    InProgressRuns int64 `json:"in_progress_runs"`
    SuccessRuns int64 `json:"success_runs"`
    FailedRuns int64 `json:"failed_runs"`
    TimeoutRuns int64 `json:"timeout_runs"`
    OutcomeUnknownRuns int64 `json:"outcome_unknown_runs"`
    CanceledRuns int64 `json:"canceled_runs"`
    SuccessDenominator int64 `json:"success_denominator"`
    SuccessRate float64 `json:"success_rate"`
    PromptTokens int64 `json:"prompt_tokens"`
    CompletionTokens int64 `json:"completion_tokens"`
    TotalTokens int64 `json:"total_tokens"`
}

type DashboardPerformance struct {
    TTFT DashboardPercentile `json:"ttft"`
    EndToEnd DashboardPercentile `json:"end_to_end"`
}

type DashboardBilling struct {
    SettledRuns int64 `json:"settled_runs"`
    ActualAmount string `json:"actual_amount"`
    ReleasedRuns int64 `json:"released_runs"`
    ReleasedAmount string `json:"released_amount"`
    UnbilledRuns int64 `json:"unbilled_runs"`
}

type DashboardAnomalyItem struct {
    Code string `json:"code"`
    Count int64 `json:"count"`
}

type DashboardAnomalies struct {
    RunTotal int64 `json:"run_total"`
    BillingTotal int64 `json:"billing_total"`
    RunItems []DashboardAnomalyItem `json:"run_items"`
    BillingItems []DashboardAnomalyItem `json:"billing_items"`
}

type DashboardRepositoryResult struct {
    Summary DashboardSummaryRow
    Performance DashboardPerformanceRow
    Billing DashboardBillingRow
    RunAnomalies []DashboardCountRow
    BillingAnomalies []DashboardCountRow
    Trend []DashboardTrendRow
    Attributions []DashboardAttributionRow
    Errors []DashboardErrorRow
    Tools []DashboardToolRow
}

type DashboardSummaryRow struct {
    TotalRuns int64
    RunningRuns int64
    SuccessRuns int64
    FailedRuns int64
    CanceledRuns int64
    TimeoutRuns int64
    OutcomeUnknownRuns int64
    PromptTokens int64
    CompletionTokens int64
    TotalTokens int64
}

type DashboardDistributionRow struct {
    SampleCount int64
    P50MS int64
    P95MS int64
}

type DashboardPerformanceRow struct {
    TTFT DashboardDistributionRow
    EndToEnd DashboardDistributionRow
}

type DashboardBillingRow struct {
    SettledRuns int64
    ActualUnits int64
    ReleasedRuns int64
    ReleasedUnits int64
    UnbilledRuns int64
}

type DashboardCountRow struct { Code string; Count int64 }

type DashboardTrendRow struct {
    Date string
    TotalRuns int64
    RunningRuns int64
    SuccessRuns int64
    FailedRuns int64
    CanceledRuns int64
    TimeoutRuns int64
    OutcomeUnknownRuns int64
    ActualUnits int64
    TTFT DashboardDistributionRow
    EndToEnd DashboardDistributionRow
}

type DashboardAttributionRow struct {
    Dimension string
    Key string
    ID int64
    Name string
    TotalRuns int64
    SuccessRuns int64
    FailedRuns int64
    TimeoutRuns int64
    OutcomeUnknownRuns int64
    TotalTokens int64
    ActualUnits int64
    RunAnomalyCount int64
    BillingAnomalyCount int64
}

type DashboardErrorRow struct { ErrorCode string; Count int64 }

type DashboardToolRow struct {
    ToolCode string
    ToolName string
    TotalCalls int64
    SuccessCalls int64
    FailedCalls int64
    TimeoutCalls int64
    Duration DashboardDistributionRow
}
```

归因公共指标固定为：

```go
type DashboardAttributionMetrics struct {
    TotalRuns int64 `json:"total_runs"`
    SuccessRuns int64 `json:"success_runs"`
    SuccessDenominator int64 `json:"success_denominator"`
    SuccessRate float64 `json:"success_rate"`
    TotalTokens int64 `json:"total_tokens"`
    ActualAmount string `json:"actual_amount"`
    RunAnomalyCount int64 `json:"run_anomaly_count"`
    BillingAnomalyCount int64 `json:"billing_anomaly_count"`
}
```

模型、渠道、智能体、用户、错误、工具、趋势和六数组容器的响应类型固定为：

```go
type DashboardModelBreakdown struct {
    ModelID string `json:"model_id"`
    ModelDisplayName string `json:"model_display_name"`
    Historical bool `json:"historical"`
    DashboardAttributionMetrics
}

type DashboardProviderBreakdown struct {
    ProviderID int64 `json:"provider_id"`
    ProviderName string `json:"provider_name"`
    DashboardAttributionMetrics
}

type DashboardAgentBreakdown struct {
    AgentID int64 `json:"agent_id"`
    AgentName string `json:"agent_name"`
    DashboardAttributionMetrics
}

type DashboardUserBreakdown struct {
    UserID int64 `json:"user_id"`
    Username string `json:"username"`
    DashboardAttributionMetrics
}

type DashboardErrorBreakdown struct {
    ErrorCode string `json:"error_code"`
    Count int64 `json:"count"`
}

type DashboardToolBreakdown struct {
    ToolCode string `json:"tool_code"`
    ToolName string `json:"tool_name"`
    TotalCalls int64 `json:"total_calls"`
    SuccessCalls int64 `json:"success_calls"`
    FailedCalls int64 `json:"failed_calls"`
    TimeoutCalls int64 `json:"timeout_calls"`
    SuccessDenominator int64 `json:"success_denominator"`
    SuccessRate float64 `json:"success_rate"`
    Duration DashboardPercentile `json:"duration"`
}

type DashboardTrendItem struct {
    Date string `json:"date"`
    TotalRuns int64 `json:"total_runs"`
    InProgressRuns int64 `json:"in_progress_runs"`
    SuccessRuns int64 `json:"success_runs"`
    FailedRuns int64 `json:"failed_runs"`
    CanceledRuns int64 `json:"canceled_runs"`
    TimeoutRuns int64 `json:"timeout_runs"`
    OutcomeUnknownRuns int64 `json:"outcome_unknown_runs"`
    SuccessDenominator int64 `json:"success_denominator"`
    SuccessRate float64 `json:"success_rate"`
    ActualAmount string `json:"actual_amount"`
    TTFT DashboardPercentile `json:"ttft"`
    EndToEnd DashboardPercentile `json:"end_to_end"`
}

type DashboardBreakdowns struct {
    Models []DashboardModelBreakdown `json:"models"`
    Providers []DashboardProviderBreakdown `json:"providers"`
    Agents []DashboardAgentBreakdown `json:"agents"`
    Users []DashboardUserBreakdown `json:"users"`
    Errors []DashboardErrorBreakdown `json:"errors"`
    Tools []DashboardToolBreakdown `json:"tools"`
}
```

所有嵌入的 `DashboardAttributionMetrics` 字段按 JSON 扁平输出，不能产生嵌套 `metrics` 对象。趋势、异常和六个 breakdown slice 在无数据时归一化为非 `nil` 空数组。

- [ ] **Step 4：实现日期规范化和响应投影**

在 `dashboard.go` 实现：

```go
const dashboardTimezone = "Asia/Shanghai"
const dashboardDefaultDays = 7
const dashboardMaxDays = 90
const dashboardMinimumSamples = 20

func (s *Service) Dashboard(ctx context.Context, filter DashboardFilter) (*DashboardResponse, *apperror.Error)
func normalizeDashboardFilter(filter DashboardFilter, now time.Time) (DashboardQuery, *apperror.Error)
func buildDashboardResponse(query DashboardQuery, rows DashboardRepositoryResult) (*DashboardResponse, error)
func dashboardRate(numerator, denominator int64) float64
```

实现规则必须是：

1. 两个日期都空时使用上海时区近 7 个自然日；只传一个日期返回稳定 `400`。
2. 只接受严格 `YYYY-MM-DD`；结束日加一天形成半开区间；包含首尾日期不得超过 90 天。
3. 所有 ID 在 Service 再做正整数防御；平台复用 `validateOptionalPlatform`；`model_id` trim 后最大 191 字符。
4. `success_rate = success / (success + failed + timeout + outcome_unknown)`，四舍五入到两位。
5. 金额单位先检查非负和求和溢出，再调用 `sharedmoney.FormatRMBUnits`。
6. `nil` Repository slice 在返回前全部转为 `make(..., 0)`；零样本分位值返回零值但前端按样本数展示空态。

- [ ] **Step 5：运行定向测试并提交**

```powershell
go test ./internal/module/ai/run -run 'TestDashboard' -count=1
git diff --check
git add internal/module/ai/run/dashboard_dto.go internal/module/ai/run/dashboard.go internal/module/ai/run/dashboard_test.go internal/module/ai/run/dto.go internal/module/ai/run/service_test.go
git commit -m "feat(ai-run): 定义运行驾驶舱统一统计口径"
```

预期：所有 `TestDashboard*` PASS；提交中还没有 HTTP 路由和 SQL。

## Task 2：实现概览、性能和趋势三条聚合查询

**Files:**
- Create: `internal/module/ai/run/dashboard_repository.go`
- Create: `internal/module/ai/run/dashboard_repository_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/dashboard_test.go`

- [ ] **Step 1：写 SQL 语义失败测试**

增加：

```go
func TestDashboardOverviewUsesTerminalDeliveryAndSettledChargeFacts(t *testing.T)
func TestDashboardPerformanceUsesSuccessfulRunsAndNearestRank(t *testing.T)
func TestDashboardTrendUsesShanghaiDayBucketsAndNinetyRowLimit(t *testing.T)
func TestDashboardQueriesDoNotSelectLargeJSONColumns(t *testing.T)
```

测试必须检查规范化 SQL 包含：

```text
r.created_at >= ? AND r.created_at < ?
charge.status = 'settled' AND charge.finalized_at IS NOT NULL
r.billing_status = 'settled'
r.status = 'success'
attempt.state = 'succeeded'
ROW_NUMBER() OVER (... ORDER BY ...)
CEIL(0.50 * sample_count)
CEIL(0.95 * sample_count)
DATE(r.created_at)
LIMIT 90
```

并断言所有 Dashboard SQL 不包含：

```text
input_snapshot
pricing_snapshot_json
prepared_request_json
usage_json
arguments_json
result_json
SELECT *
LIMIT 10000
```

- [ ] **Step 2：运行测试确认失败**

```powershell
go test ./internal/module/ai/run -run 'TestDashboard(Overview|Performance|Trend|Queries)' -count=1
```

预期：FAIL，三条 SQL 和查询函数尚不存在。

- [ ] **Step 3：实现共享基础集合和互斥异常表达式**

在 `dashboard_repository.go` 固定以下 helper，后续列表精确下钻也复用相同分类文本：

```go
func applyDashboardFilters(db *gorm.DB, query DashboardQuery) *gorm.DB
func dashboardRunAnomalyCaseSQL() string
func dashboardBillingAnomalyCaseSQL() string
func dashboardOverviewSQL() string
func dashboardPerformanceSQL() string
func dashboardTrendSQL() string
```

计费异常 `CASE` 顺序必须严格为：

```text
state_inconsistent
open_overdue
pricing_snapshot_missing
legacy_unpriced
unbilled_usage_incomplete
unbilled_over_hold
```

`state_inconsistent` 覆盖 Charge 缺失、Run/Charge 终态冲突、settled Charge 缺 `finalized_at`；命中后不得再计入后续分类。概览 SQL 用单个语句返回 summary、billing、运行异常分组和计费异常分组；禁止为每个异常码单独查询。

Run/Charge 一致性矩阵固定为：

```text
pending  + pending                     <-> open     + finalized_at IS NULL
held     + held                        <-> open     + finalized_at IS NULL
settled  + settled_complete_usage      <-> settled  + finalized_at IS NOT NULL
released + released_before_dispatch |
           released_insufficient_balance |
           released_provider_failed |
           released_outcome_unknown    <-> released + finalized_at IS NOT NULL
unbilled + legacy_unpriced |
           unbilled_usage_incomplete |
           unbilled_over_hold           <-> unbilled + finalized_at IS NOT NULL
```

`charge.id IS NULL`、任一状态/原因/终态时间不符合矩阵，或 `r.status='running'` 却已有 `settled|released|unbilled`，均先归入 `state_inconsistent`。矩阵一致后，终态 Run 仍是 `pending|held/open`，或超过 `StaleBefore` 的 running Run 仍是 `pending|held/open`，才归入 `open_overdue`。不能把正常的 released 计为异常；测试必须逐行覆盖矩阵、缺 Charge、缺 finalized_at、running 已终态计费和终态仍 open。

金额和未计费计数在同一概览 SQL 中使用以下唯一事实：

```text
actual_units：仅 r.billing_status='settled' AND charge.status='settled' AND charge.finalized_at IS NOT NULL
released_units：仅一致的 released Run/Charge，汇总 charge.held_units
unbilled_runs：billing_status='unbilled'，不得并入 actual/released
pricing_snapshot_missing：非 legacy Run 已有 Charge 但 charge.pricing_version 为空
```

`pricing_snapshot_missing` 只检查 Charge 的标量版本证据，不读取或解析 `pricing_snapshot_json`；无法确定费用的状态冲突先归入 `state_inconsistent`，且不进入实际费用。

- [ ] **Step 4：实现数据库 nearest-rank 和日趋势**

本项目 MySQL DSN 使用 `loc=Local`，API/Worker 容器固定 `TZ=Asia/Shanghai`，`DATETIME` 持久化的是上海业务墙钟时间；因此日期桶直接使用 `DATE(r.created_at)`，不能再做一次 `+08:00` 转换。性能查询只使用：

```text
TTFT:       successful final attempt.first_delta_at - dispatched_at
end_to_end: successful ai_runs.duration_ms
```

先过滤空值和负值，再按指标分区排序，通过 `ROW_NUMBER/COUNT` 选择 `CEIL(percentile * count)` 对应行。趋势按上海自然日最多返回 90 桶，缺失日期由 Service 补零，SQL 不生成无限日历。

- [ ] **Step 5：验证三条查询并提交**

```powershell
go test ./internal/module/ai/run -run 'TestDashboard(Overview|Performance|Trend|Queries)' -count=1
git diff --check
git add internal/module/ai/run/dashboard_repository.go internal/module/ai/run/dashboard_repository_test.go internal/module/ai/run/dashboard_test.go internal/module/ai/run/dto.go
git commit -m "feat(ai-run): 聚合驾驶舱概览性能与趋势"
```

预期：PASS；当前 SQL 查询数为三条。

## Task 3：完成归因查询并锁定六查询一致性事务

**Files:**
- Modify: `internal/module/ai/run/dashboard_repository.go`
- Modify: `internal/module/ai/run/dashboard_repository_test.go`
- Modify: `internal/module/ai/run/dashboard_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/platform/admin/build.go`

- [ ] **Step 1：写归因、事务和失败原子性测试**

增加：

```go
func TestDashboardAttributionsUseFourUnionedDimensionsAndTopTwenty(t *testing.T)
func TestDashboardErrorsUseLastTerminalAttemptOnly(t *testing.T)
func TestDashboardToolsExcludeRunningAndUseSuccessfulDurations(t *testing.T)
func TestDashboardUsesExactlySixQueriesInOneReadOnlyRepeatableReadTransaction(t *testing.T)
func TestDashboardRollsBackAndReturnsNoPartialResultWhenAnyQueryFails(t *testing.T)
func TestDashboardFailureLogContainsRequestRangeFiltersStageAndDurationOnly(t *testing.T)
```

`sqlmock` 顺序固定为：`BEGIN -> overview -> performance -> trend -> attributions -> errors -> tools -> COMMIT`。在 error 查询注入错误时固定为 `BEGIN -> 前四条成功 -> error 失败 -> ROLLBACK`，Service 不返回半套响应。

日志测试注入 `slog.NewTextHandler` 和固定时钟，断言失败记录只允许白名单诊断字段 `request_id/start_at/end_exclusive/platform/model_id/agent_id/provider_id/user_id/stage/duration_ms/error`；不得包含消息内容、`input_snapshot`、任何 JSON payload、DSN、密码、原始 SQL 或渲染后的 SQL。`error` 使用 GORM/driver 返回的错误文本，但不能附带查询参数。

- [ ] **Step 2：运行测试确认失败**

```powershell
go test ./internal/module/ai/run -run 'TestDashboard(Attributions|Errors|Tools|UsesExactly|RollsBack|FailureLog)' -count=1
```

预期：FAIL，后三条查询和事务入口尚不存在。

- [ ] **Step 3：实现后三条有界归因查询**

归因查询使用四段 `UNION ALL`，每个维度独立 `ORDER BY actual_units DESC, total_runs DESC, stable_key ASC LIMIT 20`，最后扫描为 `DashboardAttributionRow{Dimension, Key, ID, Name, ...}`。模型只按 canonical `model_id` 聚合，展示名取范围内 `created_at DESC,id DESC` 第一条 Run 快照，不因历史改名拆成多行，也不用当前渠道配置改写历史；Service 通过 `officialmodel.Default.Models()` 判断 `historical`。

错误查询固定使用最终终态 Attempt：

```sql
ROW_NUMBER() OVER (
  PARTITION BY attempt.run_id
  ORDER BY attempt.attempt_no DESC, attempt.id DESC
) AS final_rank
```

只有 Run 为 `failed|timeout|outcome_unknown` 且 `final_rank=1` 的错误进入统计；空错误码统一输出 `unclassified`，禁止按 `error_message` 分组。

工具查询按 `tool_code` 聚合，`tool_name` 取范围内 `started_at DESC,id DESC` 第一条调用快照，不连接当前 `ai_tools` 改写历史：

```text
denominator = success + failed + timeout
running 不进入分母
P50/P95 只使用 status=success 且 duration_ms 非负的调用
每个工具按 total_calls DESC, tool_code ASC，最多 20 行
```

- [ ] **Step 4：实现单一 Repository 入口**

```go
func (r *GormRepository) Dashboard(ctx context.Context, query DashboardQuery) (DashboardRepositoryResult, error) {
    var result DashboardRepositoryResult
    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 严格按六个固定阶段调用；任一阶段立即返回错误。
        return scanDashboardQueries(tx, query, &result)
    }, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
    return result, err
}

func scanDashboardQueries(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error
```

`Repository` 接口只增加一个 `Dashboard` 方法，不暴露六个子查询给 Service。测试替身允许注入整包 `DashboardRepositoryResult` 或单一错误。

每个子查询错误用闭合阶段码 `overview|performance|trend|attributions|errors|tools` 包装。`Service` 增加 `WithLogger(*slog.Logger)`，默认使用 `slog.Default()`；`internal/platform/admin/build.go` 使用平台现有 `logger` 注入。Handler 通过 `middleware.GetRequestID(c)` 填入 `DashboardFilter.RequestID`。Service 只在整包查询失败时写一条结构化日志，随后返回安全 `apperror`，HTTP body 不包含 SQL。

```go
type DashboardQueryStage string

const (
    DashboardStageOverview DashboardQueryStage = "overview"
    DashboardStagePerformance DashboardQueryStage = "performance"
    DashboardStageTrend DashboardQueryStage = "trend"
    DashboardStageAttributions DashboardQueryStage = "attributions"
    DashboardStageErrors DashboardQueryStage = "errors"
    DashboardStageTools DashboardQueryStage = "tools"
)

type DashboardQueryError struct {
    Stage DashboardQueryStage
    Err error
}

func (e *DashboardQueryError) Error() string {
    return fmt.Sprintf("AI run dashboard %s query failed: %v", e.Stage, e.Err)
}

func (e *DashboardQueryError) Unwrap() error { return e.Err }
```

- [ ] **Step 5：运行整个 Run 模块并提交**

```powershell
go test ./internal/module/ai/run -count=1
git diff --check
git add internal/module/ai/run/dashboard_repository.go internal/module/ai/run/dashboard_repository_test.go internal/module/ai/run/dashboard_test.go internal/module/ai/run/service.go internal/platform/admin/build.go
git commit -m "feat(ai-run): 完成驾驶舱一致性归因查询"
```

预期：PASS；查询数量不随模型、用户、Run 或工具数量增长。

## Task 4：扩展模型选项与运行列表精确下钻

**Files:**
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/module/ai/run/transport/admin/request.go`
- Modify: `internal/module/ai/run/transport/admin/handler.go`

- [ ] **Step 1：写 PageInit 合并和列表筛选失败测试**

增加：

```go
func TestPageInitMergesOfficialCatalogAndHistoricalRunModels(t *testing.T)
func TestPageInitHistoricalModelsUseRequestedDateRangeAndLatestSnapshot(t *testing.T)
func TestRunListAcceptsOutcomeUnknownAndDashboardDrilldownFilters(t *testing.T)
func TestRunListReturnsBillingFactsAndFinalAttemptErrorCode(t *testing.T)
func TestRunListErrorFilterDoesNotDuplicateRunsWithRetries(t *testing.T)
func TestRunListUsesDashboardHalfOpenDateRangeForExactDrilldown(t *testing.T)
func TestRunListFiltersRunsContainingToolCodeWithoutDuplicateRows(t *testing.T)
```

测试数据包含当前官方模型、已停用但仍在官方目录的模型、范围内只存在于历史 Run 的模型、范围外历史模型、同一历史模型两条不同名称快照，以及同一 Run 两次 Attempt。断言只合并范围内历史模型，名称取范围内 `created_at/id` 最新快照并标记 `historical=true`；错误筛选只认最后 Attempt。

- [ ] **Step 2：运行测试确认失败**

```powershell
go test ./internal/module/ai/run -run 'Test(PageInit|RunList)' -count=1
```

预期：FAIL，新选项和列表字段不存在。

- [ ] **Step 3：扩展 PageInit 和 List 契约**

`InitDict` 增加：

```go
type ModelOption struct {
    Label string `json:"label"`
    Value string `json:"value"`
    Historical bool `json:"historical"`
}

ModelArr []ModelOption `json:"model_arr"`
BillingStatusArr []dict.Option[string] `json:"billing_status_arr"`
BillingReasonArr []dict.Option[string] `json:"billing_reason_arr"`
```

PageInit 日期契约固定为：

```go
type PageInitFilter struct {
    DateStart string
    DateEnd string
}

type HistoricalModelRow struct {
    ModelID string
    ModelDisplayName string
}

func (s *Service) PageInit(ctx context.Context, filter PageInitFilter) (*InitResponse, *apperror.Error)
func (r *GormRepository) HistoricalModelOptions(ctx context.Context, startAt, endExclusive time.Time) ([]HistoricalModelRow, error)
```

日期为空时复用 Dashboard 的上海近 7 个自然日默认值；只传一端、反向或超过 90 天时返回相同稳定 `400`。Handler 的 `pageInitRequest` 只绑定 `date_start/date_end`，并将其传给 Service。

`ListQuery/listRequest/AiRunListParams` 的最终筛选集合固定为：

```text
platform, status, user_id, request_id, agent_id, provider_id,
model_id, billing_status, billing_reason, error_code,
tool_code, run_anomaly, billing_anomaly, anomaly_as_of, date_start, date_end
```

其中 `run_anomaly` 只允许 `failed|timeout|outcome_unknown|stale_running`，`billing_anomaly` 只允许六个已确认互斥分类。这样 `stale_running`、`state_inconsistent` 和 `open_overdue` 也能精确下钻，而不是退化为不准确的近似筛选。

`anomaly_as_of` 接受 RFC3339，并只在存在 `run_anomaly` 或 `billing_anomaly` 时生效；Dashboard 下钻必须传响应中的 `generated_at`。Service 解析后计算 `StaleBefore = anomaly_as_of - config.DefaultAIRunStaleTimeout`；手工列表筛选未传时使用 Service 当前时钟。非法时间返回稳定 `400`，Repository 不使用数据库 `NOW()`。

Run List 的 `date_start/date_end` 必须复用 Dashboard 的严格日期解析和半开区间：`created_at >= start_at AND created_at < end_exclusive`。删除当前 `date_end <= 'YYYY-MM-DD'` 行为；测试用结束日 `00:00:00`、中午和 `23:59:59` 三条 Run 证明整日都能命中，并证明次日 `00:00:00` 不命中。列表日期只传一端、反向或超过 90 天同样返回稳定 `400`。

`ListItem/ListRow` 增加：

```text
billing_status
billing_reason
error_code
```

- [ ] **Step 4：实现模型合并和最终 Attempt 过滤**

Repository 的 `HistoricalModelOptions` 只查询规范化半开区间，并取每个模型在该范围内最新的 Run 名称：

```sql
SELECT model_id, model_display_name
FROM (
  SELECT model_id, model_display_name,
         ROW_NUMBER() OVER (PARTITION BY model_id ORDER BY created_at DESC, id DESC) AS row_no
  FROM ai_runs
  WHERE created_at >= ? AND created_at < ? AND model_id <> ''
) historical_models
WHERE row_no = 1
ORDER BY model_id ASC
```

Service 先按 `officialmodel.Default.Models()` 生成唯一 canonical 选项，再合并历史查询；同 ID 只保留官方选项，只有目录不存在的 ID 标记历史。

Run List 通过相关子查询或 anti-join 只选择最后终态 Attempt 的 `error_code`，不得对重试 Run 产生重复列表行。先限定 Attempt `state IN ('succeeded','failed','canceled','outcome_unknown')`，再按 `attempt_no DESC,id DESC` 取第一条；`error_code` 筛选作用于该最终行。`run_anomaly` 和 `billing_anomaly` 必须复用 `dashboardRunAnomalyCaseSQL/dashboardBillingAnomalyCaseSQL`，并使用 `anomaly_as_of` 规范化出的同一 `GeneratedAt/StaleBefore` 查询事实，保证下钻集合与 Dashboard 计数相同。

`tool_code` trim 后最大 128 字符，Repository 使用相关 `EXISTS (SELECT 1 FROM ai_tool_calls tc WHERE tc.run_id=r.id AND tc.tool_code=?)`，不能直接 join 后用 `DISTINCT` 掩盖同一 Run 多次调用造成的重复。列表展示的是包含该工具调用的 Run；工具归因表的 call 数仍以 `ai_tool_calls` 为事实，不把 Run 数伪装成 call 数。

- [ ] **Step 5：验证并提交**

```powershell
go test ./internal/module/ai/run -run 'Test(PageInit|RunList|List)' -count=1
git diff --check
git add internal/module/ai/run/dto.go internal/module/ai/run/repository.go internal/module/ai/run/repository_test.go internal/module/ai/run/service.go internal/module/ai/run/service_test.go internal/module/ai/run/transport/admin/request.go internal/module/ai/run/transport/admin/handler.go
git commit -m "feat(ai-run): 支持驾驶舱精确下钻筛选"
```

预期：PASS；`outcome_unknown` 不再被 Gin binding 拒绝。

## Task 5：切换 Runtime 路由并删除五个旧统计端点

**Files:**
- Create: `internal/module/ai/run/transport/admin/dashboard_handler_test.go`
- Modify: `internal/module/ai/run/transport/admin/request.go`
- Modify: `internal/module/ai/run/transport/admin/handler.go`
- Modify: `internal/module/ai/run/transport/admin/route.go`
- Modify: `internal/module/ai/run/transport/admin/handler_test.go`
- Modify: `internal/module/ai/run/transport/admin/feedback_handler_test.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/repository.go`
- Modify: `internal/module/ai/run/repository_test.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`

- [ ] **Step 1：写新 handler 与旧路由缺席测试**

```go
func TestDashboardHandlerBindsEveryFilterAndReturnsCompleteResponse(t *testing.T)
func TestPageInitHandlerBindsDashboardDateRange(t *testing.T)
func TestDashboardHandlerRejectsInvalidPositiveIDs(t *testing.T)
func TestDashboardHandlerDoesNotLeakRepositorySQL(t *testing.T)
func TestDashboardRouteUsesAIRunListPermissionAndNoAudit(t *testing.T)
func TestLegacyAIRunStatsRoutesAreNotRegistered(t *testing.T)
```

成功请求固定为：

```text
GET /api/admin/v1/ai-runs/dashboard?date_start=2026-07-23&date_end=2026-07-29&platform=admin&model_id=gpt-5.5&agent_id=2&provider_id=3&user_id=4
```

旧路径列表必须逐个断言不存在。

- [ ] **Step 2：运行 transport 测试确认失败**

```powershell
go test ./internal/module/ai/run/transport/admin -run 'Test(Dashboard|PageInitHandler|LegacyAIRunStats)' -count=1
```

预期：FAIL，新 handler 未注册且旧路由仍存在。

- [ ] **Step 3：注册唯一 Dashboard handler**

```go
type pageInitRequest struct {
    DateStart string `form:"date_start" binding:"omitempty,len=10,datetime=2006-01-02"`
    DateEnd string `form:"date_end" binding:"omitempty,len=10,datetime=2006-01-02"`
}

type dashboardRequest struct {
    DateStart string `form:"date_start" binding:"omitempty,len=10,datetime=2006-01-02"`
    DateEnd string `form:"date_end" binding:"omitempty,len=10,datetime=2006-01-02"`
    Platform string `form:"platform" binding:"omitempty,max=32"`
    ModelID string `form:"model_id" binding:"omitempty,max=191"`
    AgentID *int64 `form:"agent_id" binding:"omitempty,min=1"`
    ProviderID *int64 `form:"provider_id" binding:"omitempty,min=1"`
    UserID *int64 `form:"user_id" binding:"omitempty,min=1"`
}
```

Handler 构造 `DashboardFilter` 时额外设置 `RequestID: middleware.GetRequestID(c)`；该字段只用于结构化错误日志，不进入 SQL 筛选或响应。

路由定义固定为：

```go
routes.Handle(adminroute.Definition{
    Method: http.MethodGet,
    Path: "/api/admin/v1/ai-runs/dashboard",
    Access: adminroute.Permission("ai_run_list"),
    Audit: adminroute.NoAudit("read-only"),
}, handler.Dashboard)
```

- [ ] **Step 4：一次性删除旧统计代码**

删除以下方法及其专属 DTO/SQL/helper/测试替身字段：

```text
Stats
LatencyStats
StatsByDate
StatsByAgent
StatsByUser
StatsSummary
LatencySamples
scanGrouped
normalizeStatsFilter
normalizeStatsListQuery
metricItem
latencyDistribution（Run 详情的 LatencyBreakdown 不删除）
```

`HTTPService` 最终只保留 `PageInit(ctx, PageInitFilter)/List/Detail/Dashboard`；反馈继续通过 `FeedbackHTTPService` 独立接口。

- [ ] **Step 5：更新 Runtime 路由快照**

```powershell
$env:UPDATE_ADMIN_ROUTE_SNAPSHOT='1'
$env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN='1'
go test ./internal/server -run 'Test(AdminRouteSnapshot|RoutePolicyGoldenIsAdminOnlyAndCurrent)' -count=1
Remove-Item Env:UPDATE_ADMIN_ROUTE_SNAPSHOT
Remove-Item Env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN
```

检查 diff：只能新增 `/api/admin/v1/ai-runs/dashboard`，删除五个旧 stats 路径；权限仍为 `ai_run_list`。

- [ ] **Step 6：验证并提交**

```powershell
go test ./internal/module/ai/run ./internal/module/ai/run/transport/admin ./internal/server -count=1
git diff --check
git add internal/module/ai/run internal/server/testdata/admin_routes_golden.txt internal/server/testdata/admin_route_policy_golden.json
git commit -m "feat(ai-run): 切换统一驾驶舱运行时接口"
```

预期：PASS；运行时不再暴露任何 `/ai-runs/stats*` 路由。

## Task 6：用 10 万 Run 证据决定索引并同步 Atlas

**Files:**
- Create: `database/reconciliation/20260729_ai_run_dashboard_query_candidates.json`
- Create when accepted: `database/migrations/202607290101_ai_run_dashboard_indexes.sql`
- Modify when accepted: `database/schema/admin.hcl`
- Modify when accepted: `database/migrations/atlas.sum`
- Create: `internal/architecture/ai_run_dashboard_schema_test.go`
- Create: `internal/module/ai/run/dashboard_repository_performance_test.go`

- [ ] **Step 1：创建只含本次四个候选的独立 manifest**

不要修改历史 `040_query_candidates.json`。新 manifest 使用与 10 万 fixture 一致的 90 天边界、真实平台注册值 `admin` 和有选择性的模型/计费/错误值；内容固定为：

```json
[
  {
    "name": "ai_run_dashboard_model_created",
    "repository_file": "internal/module/ai/run/dashboard_repository.go",
    "sql": "SELECT r.id,r.model_id,r.created_at FROM ai_runs r WHERE r.model_id=:model_id AND r.created_at>=:date_start AND r.created_at<:date_end ORDER BY r.created_at DESC,r.id DESC LIMIT :limit",
    "bindings": {"model_id": "perf-model-1", "date_start": "2026-05-01 00:00:00", "date_end": "2026-07-30 00:00:00", "limit": 100},
    "expected_order": ["r.created_at DESC", "r.id DESC"],
    "row_distribution_sql": "SELECT model_id,COUNT(*) rows_count FROM ai_runs WHERE created_at>='2026-05-01 00:00:00' AND created_at<'2026-07-30 00:00:00' GROUP BY model_id ORDER BY rows_count DESC,model_id",
    "proposed_index": "CREATE INDEX idx_ai_runs_model_created ON ai_runs (model_id,created_at,id)",
    "max_rows_examined": 100,
    "max_p95_ms": 100
  },
  {
    "name": "ai_run_dashboard_platform_created",
    "repository_file": "internal/module/ai/run/dashboard_repository.go",
    "sql": "SELECT r.id,r.platform,r.created_at FROM ai_runs r WHERE r.platform=:platform AND r.created_at>=:date_start AND r.created_at<:date_end ORDER BY r.created_at DESC,r.id DESC LIMIT :limit",
    "bindings": {"platform": "admin", "date_start": "2026-05-01 00:00:00", "date_end": "2026-07-30 00:00:00", "limit": 100},
    "expected_order": ["r.created_at DESC", "r.id DESC"],
    "row_distribution_sql": "SELECT platform,COUNT(*) rows_count FROM ai_runs WHERE created_at>='2026-05-01 00:00:00' AND created_at<'2026-07-30 00:00:00' GROUP BY platform ORDER BY rows_count DESC,platform",
    "proposed_index": "CREATE INDEX idx_ai_runs_platform_created ON ai_runs (platform,created_at,id)",
    "max_rows_examined": 100,
    "max_p95_ms": 100
  },
  {
    "name": "ai_run_dashboard_billing_created",
    "repository_file": "internal/module/ai/run/dashboard_repository.go",
    "sql": "SELECT r.id,r.billing_status,r.billing_reason,r.created_at FROM ai_runs r WHERE r.billing_status=:billing_status AND r.billing_reason=:billing_reason AND r.created_at>=:date_start AND r.created_at<:date_end ORDER BY r.created_at DESC,r.id DESC LIMIT :limit",
    "bindings": {"billing_status": "settled", "billing_reason": "settled_complete_usage", "date_start": "2026-05-01 00:00:00", "date_end": "2026-07-30 00:00:00", "limit": 100},
    "expected_order": ["r.created_at DESC", "r.id DESC"],
    "row_distribution_sql": "SELECT billing_status,billing_reason,COUNT(*) rows_count FROM ai_runs WHERE created_at>='2026-05-01 00:00:00' AND created_at<'2026-07-30 00:00:00' GROUP BY billing_status,billing_reason ORDER BY rows_count DESC,billing_status,billing_reason",
    "proposed_index": "CREATE INDEX idx_ai_runs_billing_created ON ai_runs (billing_status,billing_reason,created_at,id)",
    "max_rows_examined": 100,
    "max_p95_ms": 100
  },
  {
    "name": "ai_run_dashboard_attempt_error",
    "repository_file": "internal/module/ai/run/dashboard_repository.go",
    "sql": "SELECT a.id,a.run_id,a.error_code FROM ai_provider_attempts a JOIN ai_runs r ON r.id=a.run_id WHERE a.error_code=:error_code AND r.created_at>=:date_start AND r.created_at<:date_end ORDER BY a.run_id DESC,a.id DESC LIMIT :limit",
    "bindings": {"error_code": "perf_error_1", "date_start": "2026-05-01 00:00:00", "date_end": "2026-07-30 00:00:00", "limit": 100},
    "expected_order": ["a.run_id DESC", "a.id DESC"],
    "row_distribution_sql": "SELECT error_code,COUNT(*) rows_count FROM ai_provider_attempts WHERE error_code<>'' GROUP BY error_code ORDER BY rows_count DESC,error_code",
    "proposed_index": "CREATE INDEX idx_ai_provider_attempts_error_run ON ai_provider_attempts (error_code,run_id,id)",
    "max_rows_examined": 100,
    "max_p95_ms": 100
  }
]
```

每条代表查询都显式选择标量列、包含对应筛选和时间边界，并以 ID 作为稳定排序 tie-breaker；不得改成聚合查询伪造索引收益。先运行：

```powershell
go run ./cmd/admin-db query-manifest files --manifest database/reconciliation/20260729_ai_run_dashboard_query_candidates.json
go test ./internal/databaseevolution -run 'Test.*QueryManifest' -count=1
```

预期：命令只输出 `internal/module/ai/run/dashboard_repository.go`，测试 PASS；专用 manifest 精确包含四个候选。

- [ ] **Step 2：准备 disposable 10 万 Run 数据集**

只在当前开发库的 disposable restore `admin_ai_dashboard_perf` 中执行，禁止对开发库 `admin` 造数。该 restore 必须已经包含至少一个用户、智能体和渠道；先取真实外键 ID，再用 MySQL 8 recursive CTE 生成 `100000` 条覆盖 90 天、六类状态、五类 billing status 和 20 个模型的 Run：

```sql
SET SESSION cte_max_recursion_depth = 100001;
SET SESSION time_zone = '+08:00';
SET @dashboard_anchor = TIMESTAMP('2026-07-29 12:00:00');
SET @dashboard_user_id = (SELECT MIN(id) FROM users);
SET @dashboard_agent_id = (SELECT MIN(id) FROM ai_agents);
SET @dashboard_provider_id = (SELECT MIN(id) FROM ai_providers);
DROP TEMPORARY TABLE IF EXISTS dashboard_fixture_guard;
CREATE TEMPORARY TABLE dashboard_fixture_guard (
  selected_database VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  agent_id BIGINT UNSIGNED NOT NULL,
  provider_id BIGINT UNSIGNED NOT NULL,
  CONSTRAINT chk_dashboard_fixture_database
    CHECK (selected_database = 'admin_ai_dashboard_perf')
);
INSERT INTO dashboard_fixture_guard (selected_database, user_id, agent_id, provider_id)
VALUES (DATABASE(), @dashboard_user_id, @dashboard_agent_id, @dashboard_provider_id);

INSERT INTO ai_runs (
  platform, request_id, request_fingerprint, request_identity_status, request_identity_marker,
  user_id, agent_id, provider_id, model_id, model_display_name, input_snapshot,
  pricing_snapshot_json, status, billing_status, billing_reason,
  prompt_tokens, completion_tokens, total_tokens, duration_ms, error_message,
  started_at, finished_at, settled_at, created_at, updated_at
)
WITH RECURSIVE seq AS (
  SELECT 1 AS n
  UNION ALL SELECT n + 1 FROM seq WHERE n < 100000
)
SELECT
  'admin', CONCAT('dashboard-perf-', n),
  UNHEX(SHA2(CONCAT('dashboard-perf-', n), 256)), 'replayable', '',
  @dashboard_user_id, @dashboard_agent_id, @dashboard_provider_id,
  CONCAT('perf-model-', MOD(n, 20)), CONCAT('Perf Model ', MOD(n, 20)), '', '{}',
  ELT(1 + MOD(n, 6), 'running','success','failed','canceled','timeout','outcome_unknown'),
  ELT(1 + MOD(n, 5), 'pending','held','settled','released','unbilled'),
  ELT(1 + MOD(n, 10), 'pending','held','settled_complete_usage','released_before_dispatch',
      'released_insufficient_balance','released_provider_failed','released_outcome_unknown',
      'unbilled_usage_incomplete','unbilled_over_hold','legacy_unpriced'),
  100 + MOD(n, 5000), 20 + MOD(n, 1000), 120 + MOD(n, 6000),
  IF(MOD(n, 6)=0, NULL, 100 + MOD(n, 60000)), '',
  @dashboard_anchor - INTERVAL MOD(n, 90) DAY,
  IF(MOD(n, 6)=0, NULL, TIMESTAMPADD(MICROSECOND, (100 + MOD(n, 60000)) * 1000, @dashboard_anchor - INTERVAL MOD(n, 90) DAY)),
  IF(MOD(n, 6)=0, NULL, TIMESTAMPADD(MICROSECOND, (150 + MOD(n, 60000)) * 1000, @dashboard_anchor - INTERVAL MOD(n, 90) DAY)),
  @dashboard_anchor - INTERVAL MOD(n, 90) DAY, @dashboard_anchor
FROM seq;
```

`dashboard_fixture_guard` 保证当前 schema 不等于 `admin_ai_dashboard_perf`，或三个 `@dashboard_*_id` 任一为 `NULL` 时，fixture 在插入 Run 前即由 MySQL 8 约束失败；不得删除该 guard，也不得关闭 `FOREIGN_KEY_CHECKS`。Run 插入完成后确认 `SELECT COUNT(*) FROM ai_runs WHERE request_id LIKE 'dashboard-perf-%'` 精确返回 `100000`，再为每个合成 Run 建立一个真实结构的 Attempt 和 Charge：

```sql
INSERT INTO ai_provider_attempts (
  run_id, attempt_no, idempotency_key, state,
  prepared_request_json, prepared_request_sha256, quote_json, usage_json,
  usage_status, dispatch_state, result_candidate_json,
  provider_request_id, response_sha256, error_code,
  prepare_started_at, dispatched_at, first_delta_at, finished_at, created_at, updated_at
)
SELECT
  r.id, 1, CONCAT('dashboard-perf-attempt-', r.id),
  CASE r.status
    WHEN 'running' THEN 'prepared' WHEN 'success' THEN 'succeeded'
    WHEN 'canceled' THEN 'canceled' WHEN 'outcome_unknown' THEN 'outcome_unknown'
    ELSE 'failed'
  END,
  '{}', UNHEX(SHA2(CONCAT('dashboard-perf-request-', r.id), 256)), '{}', '{}',
  IF(r.status='success', 'complete', 'unavailable'),
  CASE WHEN r.status='running' THEN 'not_dispatched'
       WHEN r.status='outcome_unknown' THEN 'unknown' ELSE 'dispatched' END,
  NULL, CONCAT('dashboard-provider-', r.id), '',
  CASE WHEN r.status IN ('failed','timeout') THEN CONCAT('perf_error_', MOD(r.id, 8))
       WHEN r.status='outcome_unknown' THEN 'provider_outcome_unknown' ELSE '' END,
  TIMESTAMPADD(MICROSECOND, 20000, r.created_at),
  IF(r.status='running', NULL, TIMESTAMPADD(MICROSECOND, 40000, r.created_at)),
  IF(r.status='success', TIMESTAMPADD(MICROSECOND, (100 + MOD(r.id, 4000)) * 1000, r.created_at), NULL),
  r.finished_at, r.created_at, r.updated_at
FROM ai_runs r
WHERE r.request_id LIKE 'dashboard-perf-%';

INSERT INTO ai_usage_charges (
  run_id, user_id, currency, pricing_version, multiplier_ppm,
  held_units, actual_units, status, finalized_at, created_at, updated_at
)
SELECT
  r.id, r.user_id, 'CNY', 'dashboard-perf-v1', 1000000,
  10000 + MOD(r.id, 50000),
  IF(r.billing_status='settled', 1000 + MOD(r.id, 9000), 0),
  CASE WHEN r.billing_status IN ('pending','held') THEN 'open' ELSE r.billing_status END,
  IF(r.billing_status IN ('pending','held'), NULL, COALESCE(r.finished_at, r.updated_at)),
  r.created_at, r.updated_at
FROM ai_runs r
WHERE r.request_id LIKE 'dashboard-perf-%';
```

工具归因 fixture 使用一个 disposable 专用工具，并只给每 10 个 Run 插入一次调用：

```sql
INSERT INTO ai_tools (
  name, code, description, parameters_json, result_schema_json,
  risk_level, timeout_ms, status, is_del
) VALUES (
  'Dashboard Perf Tool', 'dashboard_perf_tool', '', '{}', '{}',
  'low', 3000, 1, 2
) ON DUPLICATE KEY UPDATE name=VALUES(name);

SET @dashboard_tool_id = (SELECT id FROM ai_tools WHERE code='dashboard_perf_tool' LIMIT 1);

INSERT INTO ai_tool_calls (
  run_id, tool_id, tool_code, tool_name, call_id, status,
  arguments_json, result_json, error_message, duration_ms,
  started_at, finished_at, created_at, updated_at
)
SELECT
  r.id, @dashboard_tool_id, 'dashboard_perf_tool', 'Dashboard Perf Tool',
  CONCAT('dashboard-call-', r.id),
  ELT(1 + MOD(r.id, 3), 'success', 'failed', 'timeout'),
  JSON_OBJECT(), IF(MOD(r.id, 3)=0, JSON_OBJECT('ok', TRUE), NULL), '',
  10 + MOD(r.id, 5000), r.started_at,
  TIMESTAMPADD(MICROSECOND, (10 + MOD(r.id, 5000)) * 1000, r.started_at),
  r.created_at, r.updated_at
FROM ai_runs r
WHERE r.request_id LIKE 'dashboard-perf-%' AND MOD(r.id, 10)=0;
```

- [ ] **Step 3：捕获 before/after 执行计划**

```powershell
if ([string]::IsNullOrWhiteSpace($env:MYSQL_DSN) -or $env:MYSQL_DSN -notmatch '/admin_ai_dashboard_perf(?:\?|$)') {
  throw 'MYSQL_DSN must target the disposable admin_ai_dashboard_perf database'
}
pwsh -NoProfile -File scripts/database/capture-query-evidence.ps1 `
  -Database admin_ai_dashboard_perf `
  -Manifest database/reconciliation/20260729_ai_run_dashboard_query_candidates.json `
  -OutputRoot E:\admin-query-evidence\ai-run-dashboard-20260729
Get-Content -Raw E:\admin-query-evidence\ai-run-dashboard-20260729\summary.json
```

接受条件由工具固定判定：`after_rows < before_rows`、不超过候选 `max_rows_examined`、热查询 P95 不超过预算。证据输出必须位于仓库外，不提交包含数据分布的本地证据文件。

- [ ] **Step 4：只落地 accepted 索引**

读取 `accepted_indexes.json`。对于 `accepted=true` 的 DDL，同步写入：

```text
database/migrations/202607290101_ai_run_dashboard_indexes.sql
database/schema/admin.hcl
```

`database/reconciliation/041_apply_proven_indexes.sql` 是已执行并按字节记账的历史 reconciliation 输入，绝对不修改；新索引只通过新 Atlas migration 进入现有库和空库。对于 `accepted=false` 的候选，migration 与 HCL 均不得出现其索引名。如果四个候选全部未通过，不创建空 migration，只提交 manifest 和测试，并在验收记录写明“复用现有索引”。

架构测试读取 HCL、migration 和 accepted 名单对应的常量，断言两处名称/列顺序一致，并断言未接受索引不存在。

- [ ] **Step 5：验证 Atlas 和 90 天 Dashboard 热查询**

`dashboard_repository_performance_test.go` 增加 `TestDashboardPerformanceEvidence`。测试仅在 `AI_RUN_DASHBOARD_PERF=1` 时运行，解析 `TEST_MYSQL_DSN` 并拒绝 schema 不是 `admin_ai_dashboard_perf` 的连接；先断言 `dashboard-perf-%` Run 精确为 `100000`，再使用以下查询事实执行完整 Repository Dashboard 五次：

```go
DashboardQuery{
    StartAt: time.Date(2026, 5, 1, 0, 0, 0, 0, shanghai),
    EndExclusive: time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai),
    GeneratedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, shanghai),
    StaleBefore: time.Date(2026, 7, 29, 11, 45, 0, 0, shanghai),
}
```

第 1 次作为预热；测试用 `t.Logf` 输出第 2-5 次耗时及 nearest-rank P95，并要求 P95 `<500ms`。门禁关闭或 DSN 缺失时明确 `t.Skip`，该时间目标不进入普通 CI。

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
go test ./internal/architecture -run TestAIRunDashboardSchema -count=1
go test ./internal/module/ai/run -run 'TestDashboard' -count=1
$env:AI_RUN_DASHBOARD_PERF='1'
$env:TEST_MYSQL_DSN=$env:MYSQL_DSN
try {
  go test ./internal/module/ai/run -run TestDashboardPerformanceEvidence -count=1 -v
  if ($LASTEXITCODE -ne 0) { throw 'dashboard performance evidence failed' }
} finally {
  Remove-Item Env:AI_RUN_DASHBOARD_PERF -ErrorAction SilentlyContinue
  Remove-Item Env:TEST_MYSQL_DSN -ErrorAction SilentlyContinue
}
```

如果超标，先检查六条子查询的 `EXPLAIN ANALYZE`，不得用 Redis 掩盖扫描问题。

- [ ] **Step 6：提交索引证据相关改动**

```powershell
git diff --check
git add database/reconciliation/20260729_ai_run_dashboard_query_candidates.json internal/architecture/ai_run_dashboard_schema_test.go internal/module/ai/run/dashboard_repository_performance_test.go
git add database/schema/admin.hcl database/migrations/atlas.sum
if (Test-Path database/migrations/202607290101_ai_run_dashboard_indexes.sql) { git add database/migrations/202607290101_ai_run_dashboard_indexes.sql }
git commit -m "perf(ai-run): 以执行计划固化驾驶舱索引"
```

预期：提交只包含有证据的索引，不包含本地 10 万 fixture 或证据输出。

## Task 7：发布唯一 OpenAPI 契约和后端 Contract Bundle

**Files:**
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_workflows.go`
- Modify: `internal/admincontract/openapi_test.go`
- Modify: `internal/admincontract/permissions_test.go`
- Modify: `internal/admincontract/bundle_test.go`
- Create: `internal/architecture/ai_run_dashboard_contract_test.go`
- Regenerate: `contracts/admin/v1/*`

- [ ] **Step 1：先写新契约和旧契约缺席测试**

增加断言：

```go
func TestAIRunDashboardOpenAPIIsCompleteAndNonNullable(t *testing.T)
func TestAIRunDashboardUsesAIRunListPermission(t *testing.T)
func TestLegacyAIRunStatsContractsAreAbsent(t *testing.T)
func TestAIRunPageInitAndListPublishDashboardDrilldownContract(t *testing.T)
```

`GET /api/admin/v1/ai-runs/dashboard` 查询参数必须精确为：

```text
agent_id, date_end, date_start, model_id, platform, provider_id, user_id
```

`GET /api/admin/v1/ai-runs` 同步发布 Task 4 的下钻参数，包括 RFC3339 `anomaly_as_of`。测试递归检查 `trend` 和六个 breakdown 都是 required array，所有顶层对象 required 且非 nullable，金额字段是 string。
`GET /api/admin/v1/ai-runs/page-init` 同步发布 `date_start/date_end`，两端都使用与 Dashboard 相同的 inclusive `YYYY-MM-DD` 输入描述。

- [ ] **Step 2：运行契约测试确认失败**

```powershell
go test ./internal/admincontract ./internal/architecture -run 'TestAIRun(Dashboard|PageInit)|TestLegacyAIRunStats' -count=1
```

预期：FAIL，OpenAPI 仍发布旧 Schema/路径。

- [ ] **Step 3：实现 Dashboard Schema 和 workflow**

删除所有 `AIRunStats*` 与 `AIRunLatencyStats*` Schema，新增以下稳定 Schema：

```text
AIRunPageInitModelOption
AIRunDashboardDateRange
AIRunDashboardSummary
AIRunDashboardPercentile
AIRunDashboardPerformance
AIRunDashboardBilling
AIRunDashboardAnomalyItem
AIRunDashboardAnomalies
AIRunDashboardTrendItem
AIRunDashboardAttributionMetrics
AIRunDashboardModelBreakdown
AIRunDashboardProviderBreakdown
AIRunDashboardAgentBreakdown
AIRunDashboardUserBreakdown
AIRunDashboardErrorBreakdown
AIRunDashboardToolBreakdown
AIRunDashboardBreakdowns
AIRunDashboardResult
AIRunDashboardSuccessEnvelope
```

`AIRunPageInitModelOption` 必须 require `label:string/value:string/historical:boolean`；`AIRunPageInitDict` 增加 `model_arr` 及两个 `StringOption` 计费数组。`AIRunListItem` 增加 `billing_status/billing_reason/error_code`，列表参数发布 Task 4 的完整集合。

`aiRunDashboardQueryParameters()` 使用最大长度/正整数/平台注册 Schema；`aiRunPageInitQueryParameters()` 只复用其中的 `date_start/date_end`。日期描述明确 inclusive date input 和 exclusive normalized output，`anomaly_as_of` 使用 `format: date-time`。OpenAPI 示例中的空集合必须写 `[]`，不能用 `null`。

- [ ] **Step 4：验证后端契约并提交实现 SHA**

```powershell
go test ./internal/admincontract ./internal/architecture ./internal/server -count=1
go test ./internal/module/ai/run ./internal/module/ai/run/transport/admin -count=1
git diff --check
git add internal/admincontract internal/architecture/ai_run_dashboard_contract_test.go
git commit -m "feat(admin-contract): 发布 AI 运行驾驶舱契约"
$backendSha=(git rev-parse HEAD).Trim()
if ($backendSha -notmatch '^[0-9a-f]{40}$') { throw 'backend SHA is invalid' }
```

预期：后端实现和契约源代码已提交，`$backendSha` 是包含全部 Dashboard 后端行为的真实 40 位 SHA。

- [ ] **Step 5：生成并提交正式后端 Bundle**

```powershell
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendSha
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendSha
git add contracts/admin/v1
git commit -m "chore(admin-contract): 生成 AI 运行驾驶舱契约包"
git status --short
```

预期：生成文件 manifest 的 `backend_commit` 等于 `$backendSha`，后端工作区干净。

## Task 8：前端切换单一 Dashboard API 与 Workflow

**Files (`E:/admin/admin_front_ts`):**
- Regenerate: `contracts/backend/admin/v1/*`
- Regenerate: `src/modules/http/generated/admin.ts`
- Regenerate: `src/modules/http/generated/operations.ts`
- Modify: `src/api/ai/runs.ts`
- Modify: `src/features/ai-runs/workflow.ts`
- Modify: `tests/shared/ai/ai-run-api.test.ts`
- Modify: `tests/integration/features/ai-runs.test.ts`
- Create: `tests/helpers/ai-run-dashboard.ts`

- [ ] **Step 1：同步已提交后端契约并生成类型**

```powershell
Set-Location E:\admin\admin_back_go
$backendSha=((Get-Content -Raw contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit)
Set-Location E:\admin\admin_front_ts
npm run contract:sync -- --backend E:\admin\admin_back_go --commit $backendSha
npm run contract:generate
```

预期：generated operations 中存在 `get_api_admin_v1_ai_runs_dashboard`，不存在五个旧 stats operation。

- [ ] **Step 2：写 API 和 Workflow 失败测试**

```ts
it('serializes every dashboard filter into one GET request')
it('decodes complete zero objects and empty dashboard arrays')
it('removes every legacy stats request')
it('keeps the last successful dashboard visible while a new query is loading or fails')
it('aborts a superseded dashboard query and commits only the latest response')
it('debounces terminal realtime events for current ranges and ignores historical ranges')
```

运行：

```powershell
npm test -- tests/shared/ai/ai-run-api.test.ts tests/integration/features/ai-runs.test.ts
```

预期：FAIL，API/Workflow 仍引用旧统计资源。

- [ ] **Step 3：实现唯一 API 类型与调用**

`src/api/ai/runs.ts` 最终公开：

```ts
export type AiRunDashboardResponse = components['schemas']['AIRunDashboardResult']
export type AiRunDashboardPercentile = components['schemas']['AIRunDashboardPercentile']

export interface AiRunDashboardParams {
  date_start?: string
  date_end?: string
  platform?: AiRunPlatform | ''
  model_id?: string
  agent_id?: number | ''
  provider_id?: number | ''
  user_id?: number | ''
}

export type AiRunPageInitParams = Pick<AiRunDashboardParams, 'date_start' | 'date_end'>

dashboard: (params: AiRunDashboardParams, options: ExecuteOptions = {}) =>
  executeAdminOperation(
    adminOperations.get_api_admin_v1_ai_runs_dashboard,
    { query: normalizeDashboardParams(params) },
    options,
  )

pageInit: (params: AiRunPageInitParams, options: ExecuteOptions = {}) =>
  executeAdminOperation(
    adminOperations.get_api_admin_v1_ai_runs_page_init,
    { query: normalizePageInitParams(params) },
    options,
  )
```

删除 `stats/latencyStats/statsByDate/statsByAgent/statsByUser` 及其类型和 normalize helper。扩展 `AiRunListParams` 与生成契约一致。

- [ ] **Step 4：实现单一资源、快照保留和实时防抖**

Workflow 删除 `stats/statsByDate/statsByAgent/statsByUser` 四个旧统计 resource，并以一个 Dashboard resource 替代；现有 `pageInit/list/detail` resource 继续保留：

```ts
const dashboard = createResourceQuery<AiRunDashboardResponse, AiRunDashboardParams, AiRunDashboardResponse>({
  request: (params, context) => api.dashboard(params, context),
  selectItems: (result) => [result],
})
const lastDashboard = shallowRef<AiRunDashboardResponse | null>(null)
```

`AIRunsWorkflowApi.pageInit` 改为 `(params: AiRunPageInitParams, options: ExecuteOptions)`，并新增 `dashboard(params, options)`；list/detail 签名不变。`loadDashboard` 成功后原子更新 `lastDashboard`；新筛选执行时页面继续读取该快照。ResourceQuery 自带 AbortController，确保最后请求获胜。

三个终态实时事件仍立即刷新已加载的 list 和匹配 request ID 的 detail；对 Dashboard 则合并到一个 `250ms` timer，只有最后成功查询范围包含上海“今天”时才 `dashboard.refresh()`，历史区间不刷新。realtime recovery 同样刷新已加载的 list/detail/dashboard，不刷新 PageInit。`dispose()` 清 timer、unsubscribe，并 dispose `pageInit/list/detail/dashboard` 四个 resource。

`loadPageInit({date_start,date_end})` 使用日期对作为缓存键；只有初次进入或日期对变化时重新请求，渠道、智能体、用户和模型变化不刷新字典。日期变化触发的旧 PageInit 请求同样取消且只提交最后响应。

- [ ] **Step 5：运行测试并提交**

```powershell
npm test -- tests/shared/ai/ai-run-api.test.ts tests/integration/features/ai-runs.test.ts
npm run contract:check
git diff --check
git add contracts/backend/admin src/modules/http/generated src/api/ai/runs.ts src/features/ai-runs/workflow.ts tests/helpers/ai-run-dashboard.ts tests/shared/ai/ai-run-api.test.ts tests/integration/features/ai-runs.test.ts
git commit -m "feat(ai-runs): 切换统一驾驶舱数据流"
```

预期：PASS；一次查询只产生一个 `/ai-runs/dashboard` HTTP 请求。

## Task 9：实现 Presenter、URL 下钻和列表新筛选

**Files (`E:/admin/admin_front_ts`):**
- Create: `src/views/Main/ai/runs/components/RunStats/dashboard-presenter.ts`
- Create: `tests/shared/ai/ai-run-dashboard-presenter.test.ts`
- Modify: `src/views/Main/ai/runs/index.vue`
- Modify: `src/views/Main/ai/runs/components/RunList/index.vue`
- Modify: `tests/integration/features/ai-runs.test.ts`

- [ ] **Step 1：写格式化和每类下钻失败测试**

```ts
it('formats integer counts, decimal RMB strings, percentages and durations without estimates')
it('shows sample insufficiency instead of zero milliseconds')
it('builds a seven-day Asia/Shanghai default date range')
it('drills status and every run anomaly into exact list filters')
it('drills every billing anomaly into exact list filters')
it('preserves date platform model provider agent and user filters for every attribution row')
it('uses stable ids and error codes instead of display labels')
```

运行：

```powershell
npm test -- tests/shared/ai/ai-run-dashboard-presenter.test.ts tests/integration/features/ai-runs.test.ts
```

预期：FAIL，Presenter 和 URL 下钻尚不存在。

- [ ] **Step 2：实现无副作用 Presenter API**

```ts
export function formatDashboardCount(value: number): string
export function formatDashboardMoney(value: string): string
export function formatDashboardRate(value: number): string
export type DashboardDurationPresentation =
  | { kind: 'value'; text: string; sampleCount: number }
  | { kind: 'insufficient'; sampleCount: number }
export function formatDashboardDuration(value: AiRunDashboardPercentile): DashboardDurationPresentation
export function defaultDashboardDates(now: Date): [string, string]
export function buildRunListDrilldown(
  base: AiRunDashboardParams,
  generatedAt: string,
  target: DashboardDrilldownTarget,
): AiRunListParams
export function serializeRunListQuery(params: AiRunListParams): Record<string, string>
```

`DashboardDrilldownTarget` 是闭合联合类型，不接受任意显示文本：

```ts
export type DashboardDrilldownTarget =
  | { kind: 'status'; status: AiRunStatus }
  | { kind: 'run_anomaly'; code: 'failed' | 'timeout' | 'outcome_unknown' | 'stale_running' }
  | { kind: 'billing_anomaly'; code: 'state_inconsistent' | 'open_overdue' | 'pricing_snapshot_missing' | 'legacy_unpriced' | 'unbilled_usage_incomplete' | 'unbilled_over_hold' }
  | { kind: 'model'; model_id: string }
  | { kind: 'provider'; provider_id: number }
  | { kind: 'agent'; agent_id: number }
  | { kind: 'user'; user_id: number }
  | { kind: 'error'; error_code: string }
  | { kind: 'tool'; tool_code: string }
```

金额只校验后端十进制字符串并添加 `¥`；禁止乘费率。样本数为 0 或 `insufficient_sample=true` 时返回国际化所需结构 `{kind:'insufficient', sampleCount}`，不返回 `0 ms`。

- [ ] **Step 3：让父页面和 Run List 消费 URL query**

`runs/index.vue` 将 tab 写入 `?tab=list|stats`。收到 `RunStats` 的 `drilldown` 事件后：

1. 调用 `serializeRunListQuery`；
2. `router.push` 写入筛选与 `tab=list`；
3. Run List 从 query 恢复序列化的全部筛选，包括 `dateRange/platform/status/model_id/agent_id/provider_id/user_id/billing_status/billing_reason/error_code/tool_code/run_anomaly/billing_anomaly/anomaly_as_of`；
4. 只触发一次列表查询，浏览器后退可返回统计页。

新增列表搜索项使用 PageInit 的模型、计费状态和计费原因选项；错误码和工具编码使用文本输入。表格增加 billing status/reason/error code 列，但不添加居中 CSS。

- [ ] **Step 4：验证并提交**

```powershell
npm test -- tests/shared/ai/ai-run-dashboard-presenter.test.ts tests/integration/features/ai-runs.test.ts
npm run typecheck
git diff --check
git add src/views/Main/ai/runs/index.vue src/views/Main/ai/runs/components/RunList/index.vue src/views/Main/ai/runs/components/RunStats/dashboard-presenter.ts tests/shared/ai/ai-run-dashboard-presenter.test.ts tests/integration/features/ai-runs.test.ts
git commit -m "feat(ai-runs): 支持驾驶舱精确下钻"
```

预期：PASS；所有下钻 URL 可刷新恢复，不依赖表格显示文案反推 ID。

## Task 10：实现筛选、指标、异常和归因工作区

**Files (`E:/admin/admin_front_ts`):**
- Create: `src/views/Main/ai/runs/components/RunStats/RunDashboardFilters.vue`
- Create: `src/views/Main/ai/runs/components/RunStats/RunDashboardSummary.vue`
- Create: `src/views/Main/ai/runs/components/RunStats/RunDashboardDiagnostics.vue`
- Create: `src/views/Main/ai/runs/components/RunStats/RunDashboardBreakdowns.vue`
- Create: `tests/component/ai/RunDashboard.test.ts`
- Modify: `src/views/Main/ai/runs/components/RunStats/index.vue`
- Modify: `src/views/Main/ai/runs/components/RunStats/styles.css`

- [ ] **Step 1：写组件结构和状态失败测试**

```ts
it('renders six core metrics from one dashboard response')
it('shows success numerator denominator and all run statuses')
it('keeps run anomalies and billing anomalies separate')
it('uses AppTable for all six breakdown tabs without alignment overrides')
it('retains the last successful data and marks refresh failure as stale')
it('shows a retryable first-load error and a truthful empty state')
it('emits stable drilldown targets from metrics anomalies and rows')
```

测试中的 `AppTable` stub 记录 `columns`；断言所有列都没有 `elementProps.align/headerAlign`，并扫描 `styles.css` 不存在归因表专属居中规则。

- [ ] **Step 2：运行组件测试确认失败**

```powershell
npm test -- tests/component/ai/RunDashboard.test.ts
```

预期：FAIL，新组件尚不存在。

- [ ] **Step 3：实现紧凑单层组件**

组件契约固定为：

```ts
RunDashboardFilters:
  props { modelValue, dict, loading }
  emits { 'update:modelValue', query, reset }

RunDashboardSummary:
  props { summary, performance, billing, anomalies }
  emits { drilldown }

RunDashboardDiagnostics:
  props { anomalies }
  emits { drilldown }

RunDashboardBreakdowns:
  props { breakdowns, loading }
  emits { drilldown }
```

首屏指标固定为请求数、成功率、实际费用、TTFT P95、运行异常、计费异常。指标带使用同一平面和分隔线，不做六张浮动卡片；状态分布展示原始数量和占总 Run 比例。诊断区左右并列，窄屏转单列。归因区只使用一个 `AppTable`，通过 tab 切换 columns/data。

- [ ] **Step 4：重写页面组合和失败状态**

`RunStats/index.vue` 先用 Presenter 生成默认日期，再以同一 `date_start/date_end` 加载 PageInit 和 Dashboard；之后只在日期对变化时刷新 PageInit。它还负责维护筛选、组合四个子组件、映射 resource state、转发 drilldown和卸载 dispose。规则：

```text
首次 loading：显示骨架/加载状态，不渲染虚假数字
refreshing：保留 lastDashboard，局部显示刷新中
首次 error/missing：显示错误和重试按钮
已有快照后 error：保留数据，显示“数据未更新”和最后 generated_at
total_runs=0：显示完整空态，性能处显示“-”
```

- [ ] **Step 5：验证并提交**

```powershell
npm test -- tests/component/ai/RunDashboard.test.ts tests/shared/ai/ai-run-dashboard-presenter.test.ts
npm run typecheck
git diff --check
git add src/views/Main/ai/runs/components/RunStats tests/component/ai/RunDashboard.test.ts
git commit -m "feat(ai-runs): 构建运行与计费驾驶舱工作区"
```

预期：PASS；页面没有卡片套卡片，没有额外 AppTable 居中样式。

## Task 11：增加按需趋势图、国际化并删除旧统计组件

**Files (`E:/admin/admin_front_ts`):**
- Create: `src/views/Main/ai/runs/components/RunStats/RunDashboardTrend.vue`
- Create: `src/views/Main/ai/runs/components/RunStats/dashboard-chart.ts`
- Modify: `src/views/Main/ai/runs/components/RunStats/index.vue`
- Modify: `src/views/Main/ai/runs/components/RunStats/styles.css`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Regenerate: `src/i18n/locales/generated.ts`
- Modify: `tests/component/ai/RunDashboard.test.ts`
- Modify: `tests/component/ai/RunLatencyBreakdown.test.ts`
- Modify: `package.json`
- Modify: `package-lock.json`
- Delete: `src/views/Main/ai/runs/components/RunStats/RunLatencyStatsTable.vue`

- [ ] **Step 1：安装 ECharts 并写图表生命周期失败测试**

```powershell
npm install echarts@^6.0.0
```

增加：

```ts
it('switches run cost and performance trend options without refetching')
it('loads only line bar grid tooltip legend and canvas modules')
it('resizes from ResizeObserver and disposes the chart on unmount')
it('does not initialize a chart for an empty trend')
```

运行：

```powershell
npm test -- tests/component/ai/RunDashboard.test.ts
```

预期：FAIL，趋势组件尚不存在。

- [ ] **Step 2：实现按需图表 runtime 和三个 option**

`dashboard-chart.ts` 只导入/注册：

```text
echarts/core
LineChart
BarChart
GridComponent
TooltipComponent
LegendComponent
CanvasRenderer
```

不得 `import * as echarts from 'echarts'`。运行趋势显示总请求与成功/异常；费用趋势显示 `actual_amount`；性能趋势显示 TTFT 和完整耗时 P50/P95。金额字符串只在绘图边界用 `Number` 做坐标，tooltip 仍展示原始后端金额字符串。

`RunDashboardTrend.vue` 使用稳定 `min-height`，挂载后 init，数据/tab 变化只 `setOption(..., true)`，`ResizeObserver` 调 `resize()`，卸载调用 `dispose()`。

- [ ] **Step 3：补齐中英文同构文案并删除旧代码**

新增同构 key 组：

```text
aiRuns.dashboard.filters.*
aiRuns.dashboard.summary.*
aiRuns.dashboard.performance.*
aiRuns.dashboard.billing.*
aiRuns.dashboard.status.*
aiRuns.dashboard.runAnomalies.*
aiRuns.dashboard.billingAnomalies.*
aiRuns.dashboard.trend.*
aiRuns.dashboard.breakdowns.*
aiRuns.dashboard.states.*
```

删除旧 `avgLatency/providerLatency/latencyWindow/recentDates/topAgents` 页面文案和 `RunLatencyStatsTable.vue`；`RunLatencyBreakdown.test.ts` 只保留 Run 详情时间线测试。

```powershell
npm run locale:generate
npm run locale:check
```

- [ ] **Step 4：验证图表、类型和 bundle，不抬高基线掩盖体积**

```powershell
npm test -- tests/component/ai/RunDashboard.test.ts tests/component/ai/RunLatencyBreakdown.test.ts
npm run typecheck
npm run build
npm run bundle:check
```

若 `bundle:check` 超预算，将 ECharts runtime 改为组件挂载后的动态 import，使其成为统计页独立异步 chunk；不得直接执行 `bundle:baseline` 放宽预算。

- [ ] **Step 5：提交前端完成态**

```powershell
git diff --check
git add package.json package-lock.json src/views/Main/ai/runs/components/RunStats src/i18n/locales tests/component/ai/RunDashboard.test.ts tests/component/ai/RunLatencyBreakdown.test.ts
git commit -m "feat(ai-runs): 增加运营趋势与完整驾驶舱交互"
```

预期：PASS；旧统计组件和旧文案引用为零。

## Task 12：端到端静态验收与发布检查

**Files:**
- Verify only; only modify files when a verification failure identifies a defect in Tasks 1-11.

- [ ] **Step 1：后端全量验证**

在 `E:/admin/admin_back_go`：

```powershell
go test ./internal/module/ai/run -count=1
go test ./internal/module/ai/run/transport/admin -count=1
go test ./internal/admincontract -count=1
go test ./internal/architecture -count=1
go test ./internal/server -run 'Test(AdminRouteSnapshot|RoutePolicyGoldenIsAdminOnlyAndCurrent|RouterAIRunReadsRequireAIRunListPermission)' -count=1
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
pwsh -NoProfile -File scripts/check-admin-contract.ps1
go test ./... -count=1
git diff --check
git status --short
```

预期：全部 PASS；后端工作区干净；契约和 migration hash 当前。

- [ ] **Step 2：前端全量验证**

在 `E:/admin/admin_front_ts`：

```powershell
npm test -- tests/shared/ai/ai-run-api.test.ts
npm test -- tests/shared/ai/ai-run-dashboard-presenter.test.ts
npm test -- tests/integration/features/ai-runs.test.ts
npm test -- tests/component/ai/RunDashboard.test.ts
npm test -- tests/component/ai/RunLatencyBreakdown.test.ts
npm run locale:check
npm run contract:check
npm run check:browser-only
npm run routes:check
npm run lint -- --max-warnings 0
npm run typecheck
npm run build
npm run bundle:check
git diff --check
git status --short
```

预期：全部 PASS；`git status` 只允许用户原有未跟踪 `.superpowers/`，没有其他未提交文件。

- [ ] **Step 3：执行规格覆盖和死代码扫描**

```powershell
# 后端
rg -n '/ai-runs/stats|StatsByDate|StatsByAgent|StatsByUser|LatencyStats|LatencySamples|LIMIT 10000' internal contracts/admin/v1

# 前端
rg -n 'statsByDate|statsByAgent|statsByUser|latencyStats|RunLatencyStatsTable|/ai-runs/stats' src tests contracts/backend/admin/v1
```

预期：两条命令都没有匹配。随后核对：

```text
一个 Dashboard HTTP 请求
最多六条固定 SQL
同一时间范围和快照
真实 settled actual_units
成功样本 TTFT/完整耗时 P50/P95
运行/计费异常分离且闭合
六类归因可精确下钻
90 天/10 万 Run 热查询目标 <500ms
无 Redis、无 N+1、无大 JSON、无 Playwright
```

- [ ] **Step 4：若验收修复产生改动，按仓库分别提交**

```powershell
# 仅在确有 tracked-file 修复时执行，提交前重跑对应失败命令。
$verificationFixes = @(git diff --name-only)
if ($verificationFixes.Count -gt 0) {
  git add -- $verificationFixes
  git commit -m "fix(ai-runs): 修正驾驶舱验收问题"
}
```

最终报告必须列出：后端实现 SHA、Bundle manifest 的 backend SHA、前端最终 SHA、四个候选索引的 accepted/rejected 结果、90 天热查询第 2-5 次耗时，以及所有验证命令结果。
