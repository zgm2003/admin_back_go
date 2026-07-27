# GPT / Claude 模型定价管理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 GPT、Claude 全局 canonical model 基础价目录和受 RBAC 保护的覆盖管理，并让所有新 Run 在派发前冻结有效价格、阶梯规则和智能体倍率。

**Architecture:** `pricing` 保持无数据库的精确金额与阶梯计价领域；新增 `modelpricing` 聚合官方目录和数据库覆盖，并以 `Resolve(ctx, requestedModelID)` 注入所有 Run 接受入口。Admin API 只管理完整覆盖集；Gateway 只读取 Run 中不可变快照，改价不影响历史 Run。

**Tech Stack:** Go、GORM、MySQL 8、Atlas、Gin Admin Contract、Vue 3、TypeScript、Element Plus、Vitest。

**Spec:** `docs/superpowers/specs/2026-07-27-ai-model-pricing-management-design.md`

---

## 执行边界

- 不执行数据库迁移，不写 `role_permissions`，管理员权限由用户上线前手动授权。
- 不增加汇率、供应商价格、价格缓存、密钥、负余额或 AI 退款。
- 不恢复音频、视频；图片等仍在生产使用的只读官方价格不得被目录升级破坏。
- 不运行时抓取官网；官方数字只经代码审查写入版本化 JSON。
- 自动验证只运行本计划列出的定向命令和 `git diff --check`；全量测试、race、完整 typecheck/build、Docker、Playwright 留给用户手动执行。
- Task 1-4 在 `admin_back_go` 执行，Task 5 在 `admin_front_ts` 执行；各自提交但都不 push。

### Task 1: 升级官方目录、十进制金额与阶梯计价

**Files:**
- Modify: `internal/shared/money/units.go`
- Test: `internal/shared/money/units_test.go`
- Modify: `internal/module/ai/pricing/catalog.go`
- Modify: `internal/module/ai/pricing/quote.go`
- Create: `internal/module/ai/pricing/tier.go`
- Test: `internal/module/ai/pricing/catalog_test.go`
- Test: `internal/module/ai/pricing/quote_test.go`
- Create: `internal/module/ai/pricing/tier_test.go`
- Create: `internal/module/ai/pricing/catalog/official_numeric_parity_v3.json`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Test: `internal/infra/ai/openaicompat/client_test.go`

- [ ] **Step 1: 先锁定严格金额和目录失败用例**

测试 `ParseRMBUnits` 只接受非负普通十进制字符串、最多 8 位小数且结果可装入 `int64`；拒绝指数、符号、空白、小数超长和溢出。目录测试同时锁定：受审 canonical ID/别名唯一、`unit=token`、`unit_scale>0`、rate key 唯一、至少一项正价、HTTPS host 只允许 `openai.com`、`anthropic.com`、`claude.com` 及其子域，以及 `review_after` 必须是合法 UTC 日期。

- [ ] **Step 2: 实现精确十进制转换和 v3 目录类型**

在 `money` 中用字符串和整数实现：

```go
func ParseRMBUnits(value string) (int64, error)
```

目录 JSON 以 `price` 字符串作为唯一金额输入，加载时转换为运行时 `Rate.PriceUnits`，禁止经过 `float32/float64`；顶层必须校验 `official_currency=USD`、`billing_currency=CNY`、`conversion_policy=numeric_parity`。扩展 `ModelPrice` 保存 `model_family`、`pricing_profile`、`context_tier_threshold_tokens`、`review_after`、目录版本和价格来源元数据；旧 v1/v2 文件保留为历史证据，但 `Default` 只 embed v3。官方 host 白名单只在 v3 loader 和覆盖写入校验，不得收紧用于历史 PricingSnapshot 的通用 `NewCatalogChecked`，避免旧快照因新增规则失效。

- [ ] **Step 3: 录入受审 GPT/Claude 官方数字**

按 Spec 固定模型集合和官方 URL 录入 OpenAI Standard、Anthropic 标准全局价；美元价格数字原样写成人民币数字，不换汇。`claude-sonnet-5` 必须写入 `review_after=2026-09-01`；Batch、Flex、Priority、Regional/Data Residency、Fast Mode 不得复用普通价。保留当前仍被图片等生产代码消费的非页面 rate，页面可管理标志只能来自 `catalog_vendor + model_family`，不得按名称前缀猜测，也不得因供应商模型列表自动扩展目录。

- [ ] **Step 4: 增加显式的 Hold 与结算阶梯选择**

保持 `Quote` 只做 rate key 精确匹配和一次总额向上取整；在 `tier.go` 增加两个前置步骤：

```go
func UpperBoundLines(price ModelPrice, lines []QuoteLine) ([]QuoteLine, error)
func SettlementLines(price ModelPrice, lines []QuoteLine) ([]QuoteLine, error)
```

`UpperBoundLines` 为所有可用 input/cache/output 阶梯选择最贵合法 rate，确保 Hold 不低估。`SettlementLines` 按 `AttemptID` 分组，以该 attempt 去重后的 `input + cache_read + cache_write` 总量判断 GPT 阈值；只有严格大于 `272000` 才把该请求全部相关项切到 `long_context`。Claude `cache_write` 必须保留 `5m|1h` 明细；只有总写入量、无法唯一确定 tier 时返回 `ErrPriceUnavailable`，不能猜价。

- [ ] **Step 5: 解析 Claude 缓存写入时长**

OpenAI-compatible usage 同时识别 cache creation 总量和 `ephemeral_5m_input_tokens`、`ephemeral_1h_input_tokens` 明细。明细存在时分别产出带 `tier_key=5m|1h` 的 usage item，并验证明细之和等于总量；冲突、负数、超出 prompt 或只有无法判价的总量由后续计价 fail closed。

- [ ] **Step 6: 运行 Task 1 定向检查并提交**

```powershell
go test ./internal/shared/money ./internal/module/ai/pricing ./internal/infra/ai/openaicompat
git diff --check
git add internal/shared/money internal/module/ai/pricing internal/infra/ai/openaicompat
git commit -m "feat(ai): add canonical GPT and Claude pricing"
```

预期：目录、金额、阶梯和 usage parser 测试通过；不运行 `go test ./...`。

### Task 2: 建立价格覆盖 Schema、Repository 与 Service

**Files:**
- Create: `database/migrations/202607270103_ai_model_pricing.sql`
- Modify: `database/schema/admin.hcl`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/reconciliation/030_verify_schema.sql`
- Modify: `database/reconciliation/031_verify_relations.sql`
- Modify: `database/seeds/admin_permissions.sql`
- Create: `internal/architecture/ai_model_pricing_schema_test.go`
- Create: `internal/module/ai/modelpricing/model.go`
- Create: `internal/module/ai/modelpricing/repository.go`
- Create: `internal/module/ai/modelpricing/repository_gorm.go`
- Create: `internal/module/ai/modelpricing/service.go`
- Create: `internal/module/ai/modelpricing/service_test.go`
- Create: `internal/module/ai/modelpricing/repository_gorm_test.go`

- [ ] **Step 1: 先写覆盖完整性和并发测试**

覆盖以下事实：无覆盖返回官方价；合法覆盖优先；官方 `review_after` 已过期时无覆盖拒绝、合法覆盖仍可用；缺少、重复或新增 rate key、错误 `unit_scale`、负数、非法 URL、未知/非受审管理模型全部拒绝；损坏的已存在覆盖不回退官方价；首次写入只接受 `expected_version=0`；更新和删除必须匹配正版本，否则映射为稳定 version conflict。

- [ ] **Step 2: 创建规范化表和约束**

迁移创建 `ai_model_price_overrides` 与 `ai_model_price_override_rates`：头表以 `(catalog_vendor, model_id)` 唯一，保存 `version/source_url/verified_at/updated_by/timestamps`；明细以 `(override_id, category, unit, tier_key)` 唯一，保存非负 `price_units` 和正 `unit_scale`，外键在恢复官方价时级联删除。同步 HCL、schema/relations reconciliation 和定向 architecture test，不引入 JSON 价格列或软删除。

- [ ] **Step 3: 实现无缓存权威 Resolver**

对外运行时接口固定为：

```go
type Resolver interface {
	Resolve(context.Context, string) (pricing.ModelPrice, error)
}
```

每次调用先由官方目录解析 identity 和 rate shape（该步骤不因 `review_after` 过期丢失 canonical identity），再按 canonical ID 查询覆盖；只有没有覆盖、准备回退官方基线时才检查 `review_after`。`pricing.ModelPrice` 返回完整有效 rates 及 `official|override` 来源、目录版本、覆盖版本、来源 URL 和核验日期；`Version` 对官方价使用 catalog version，对覆盖价使用 `catalogVersion:override:version`，继续作为 Quote evidence 的稳定 revision。数据库不可用、覆盖损坏或映射歧义直接返回错误，不做 Redis/进程缓存。

- [ ] **Step 4: 实现完整替换和乐观锁事务**

Service 的管理方法只接受 `expected_version`、完整十进制 `prices`、`source_url`、`verified_at` 和管理员 ID。事务内锁定覆盖头、对照官方 rate key 全集、将价格精确解析为 units，再一次性替换明细；创建版本为 1，修改原子加 1，恢复官方价带版本删除。返回 before/after 闭合摘要，不允许请求修改 model/vendor/family/key/unit/unit_scale。

- [ ] **Step 5: 写入菜单/权限定义但不授权角色**

同一迁移和 `database/seeds/admin_permissions.sql` 使用尚未占用的 ID：页面 `921`（父级 AI 菜单 `5`，code=`ai_model_pricing_list`，path=`/ai/model-pricing`，view key=`ai/model-pricing`）和子权限 `922`（code=`ai_model_pricing_edit`）。不得插入或更新 `role_permissions`。

- [ ] **Step 6: 生成 Atlas hash、定向验证并提交**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
go test ./internal/module/ai/modelpricing ./internal/architecture -run 'TestAIModelPricing'
git diff --check
git add database internal/architecture/ai_model_pricing_schema_test.go internal/module/ai/modelpricing
git commit -m "feat(ai): persist model price overrides"
```

预期：只生成 migration/HCL/hash，不连接业务库、不执行 migration。

### Task 3: 注入 Resolver 并闭合每个新 Run 的价格快照

**Files:**
- Modify: `internal/module/ai/aigateway/quote_validator.go`
- Test: `internal/module/ai/aigateway/quote_validator_test.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/dto.go`
- Test: `internal/module/ai/agent/service_test.go`
- Modify: `internal/module/ai/chat/service.go`
- Test: `internal/module/ai/chat/service_test.go`
- Modify: `internal/module/ai/message/service.go`
- Test: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/tool/service.go`
- Test: `internal/module/ai/tool/service_test.go`
- Modify: `internal/module/ai/image/service.go`
- Test: `internal/module/ai/image/service_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Test: `internal/runtime/ai_billing_text_test.go`
- Test: `internal/runtime/ai_billing_finalizer_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/runtime/worker.go`

- [ ] **Step 1: 先写依赖注入和历史快照测试**

测试必须能用 fake Resolver 证明 agent/chat/message/tool/image 都不再读 `pricing.Default`。新增 snapshot schema 版本测试：历史无新字段 JSON 仍按旧语义解析；新快照缺价格来源、目录/覆盖版本或 tier 阈值元数据时拒绝；改价后旧 Run 仍按自身 rates 结算。

- [ ] **Step 2: 给五个消费者注入同一个 Resolver**

在各 Service dependencies/options 中加入非空 `modelpricing.Resolver`，替换生产路径中的全部 `pricing.Default.Resolve`：

```text
agent/service.go
chat/service.go
message/service.go
tool/service.go
image/service.go
```

Admin build 与 Worker 分别用各自 DB 创建同构 `modelpricing.Service` 并注入。任何 Run 接受入口解析失败时，必须在创建可派发 attempt、调用上游和冻结钱包之前返回现有 `ai.billing.price_unavailable`。

- [ ] **Step 3: 扩展不可变 PricingSnapshot**

新快照显式保存 snapshot schema version、canonical/requested model、有效 rates、`official|override`、catalog version、override version、source URL、verified/retrieved date、`context_tier_threshold_tokens`、catalog/effective output cap 和 agent `multiplier_ppm`。保留现有 JSON 字段和 legacy parse 分支，不重写已落库快照，也不在 dispatch/settlement 时重新查询当前价格。

- [ ] **Step 4: 把 tier 选择接入 Hold 和真实结算**

`paidChatAssembler`/图片等上界报价先调用 `UpperBoundLines` 再 `Quote`；prior usage 与 `persistedSettlementPricer` 按 attempt 调用 `SettlementLines` 再 `Quote`。Quote evidence 继续绑定快照版本、请求 hash 和 output cap；tier 无法判定时返回 usage incomplete/price unavailable，由既有 finalizer 处理，不估算、不退款。

- [ ] **Step 5: 更新智能体只读价格数据**

agent page-init/list/detail 返回当前有效价格、来源、canonical model 和完整 tier rates，但 mutation 仍只保存 `billing_multiplier` 与 `max_output_tokens`。输出上限校验也使用注入 Resolver；非页面图片价格仍可供运行时解析但不进入 GPT/Claude 管理页。

- [ ] **Step 6: 运行 Task 3 定向检查并提交**

```powershell
go test ./internal/module/ai/aigateway ./internal/module/ai/agent ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/tool ./internal/module/ai/image ./internal/runtime -run 'Test.*(Pricing|Price|Tier|Snapshot|Hold|Settlement|Resolver)'
git diff --check
git add internal/module/ai internal/runtime internal/platform/admin
git commit -m "feat(ai): resolve prices before run dispatch"
```

预期：新 Run 冻结有效价，历史快照兼容；不运行全量 runtime 测试。

### Task 4: 发布 Admin API、RBAC、审计与 Contract Bundle

**Files:**
- Create: `internal/module/ai/modelpricing/dto.go`
- Create: `internal/module/ai/modelpricing/transport/admin/request.go`
- Create: `internal/module/ai/modelpricing/transport/admin/handler.go`
- Create: `internal/module/ai/modelpricing/transport/admin/handler_test.go`
- Create: `internal/module/ai/modelpricing/transport/admin/route.go`
- Create: `internal/module/ai/modelpricing/transport/admin/route_test.go`
- Modify: `internal/platform/admin/graph.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/server/routes_admin_ai.go`
- Modify: `internal/admincontract/views.go`
- Modify: `internal/admincontract/views_test.go`
- Modify: `internal/admincontract/permissions_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Regenerate: `contracts/admin/v1/*`

- [ ] **Step 1: 先锁定 API、RBAC 和审计策略**

Handler/route 测试覆盖以下固定接口：三个 GET 只允许 `ai_model_pricing_list`；PUT/DELETE 只允许 `ai_model_pricing_edit`，并要求 OperationLog module=`ai_model_pricing`、action=`update|restore_official`。

```text
GET    /api/admin/v1/ai-model-prices/page-init
GET    /api/admin/v1/ai-model-prices
GET    /api/admin/v1/ai-model-prices/:model_id
PUT    /api/admin/v1/ai-model-prices/:model_id
DELETE /api/admin/v1/ai-model-prices/:model_id/override?expected_version=N
```

测试 400 invalid override、404 model not found、409 version conflict 和响应只暴露十进制人民币字符串、不暴露内部 units。

- [ ] **Step 2: 实现列表、详情、覆盖和恢复接口**

列表只返回受审 GPT/Claude，支持 family 和 canonical model ID 查询；PUT 要求完整 rates 和 `expected_version`；DELETE 从 query 读取唯一正整数版本。管理员 ID 从已认证 identity 获取，路径 model ID 必须先 canonical 精确匹配，额外 JSON 字段和未知 query key 均拒绝。错误码固定为 `ai.model_pricing.invalid_override`、`ai.model_pricing.version_conflict`、`ai.model_pricing.model_not_found`；运行时缺价继续使用 `ai.billing.price_unavailable`。

- [ ] **Step 3: 装配 Graph 和路由元数据**

在 `AIGraph` 增加 `ModelPrices` capability 并纳入 `Validate`，在 build 中复用 Task 2 的 service，在 `registerAdminAIRoutes` 注册。写操作响应包含 before/after 摘要供 OperationLog 保存；GET 明确 NoAudit，价格中没有密钥，审计无需新增加密能力。

- [ ] **Step 4: 发布菜单视图和权限契约**

加入 `/ai/model-pricing`、view key `ai/model-pricing`、i18n key `menu.ai_model_pricing`，绑定 list permission。权限总数断言从 `106` 精确调整为 `108`，更新 route policy/routes golden；不要给默认角色自动授权。

- [ ] **Step 5: 定向验证后先提交契约源，再生成 Bundle**

```powershell
go test ./internal/module/ai/modelpricing/transport/admin ./internal/admincontract ./internal/server -run 'Test.*(ModelPric|Permission|View|Route)'
git diff --check
git add internal/module/ai/modelpricing internal/platform/admin internal/server internal/admincontract
git commit -m "feat(ai): expose model pricing administration"
$backendCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
git add contracts/admin/v1
git commit -m "chore(contract): publish model pricing API"
```

预期：Bundle 锁定刚提交的后端 source commit；不 push。

### Task 5: 实现模型定价页和智能体价格联动

**Files:**
- Regenerate: `contracts/backend/admin/v1/*`
- Regenerate: `src/modules/http/generated/admin.ts`
- Regenerate: `src/modules/http/generated/operations.ts`
- Create: `src/api/ai/model-prices.ts`
- Create: `src/utils/fixed-decimal.ts`
- Create: `src/views/Main/ai/model-pricing/index.vue`
- Create: `src/views/Main/ai/model-pricing/use-model-pricing-page.ts`
- Create: `src/views/Main/ai/model-pricing/components/ModelPriceDrawer.vue`
- Modify: `src/views/Main/ai/agents/index.vue`
- Modify: `src/views/Main/ai/agents/use-agent-admin-page.ts`
- Modify: `src/api/ai/agents.ts`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Modify: `src/i18n/locales/zh-CN/layout.ts`
- Modify: `src/i18n/locales/en-US/layout.ts`
- Regenerate: `src/i18n/locales/generated.ts`
- Regenerate: `src/router/view-registry.ts`
- Create: `tests/shared/ai/ai-model-pricing-api.test.ts`
- Create: `tests/shared/ai/ai-model-pricing-decimal.test.ts`
- Create: `tests/component/ai/ModelPricingPage.test.ts`
- Modify: `tests/shared/ai/ai-agent-billing-config.test.ts`

- [ ] **Step 1: 同步契约并先写页面行为测试**

```powershell
npm run contract:sync
npm run contract:generate
```

API 类型只能从 generated Admin contract 提取。测试锁定：family/search 参数、PUT 完整 rate 集、DELETE expected version、409 后刷新；无 edit 权限不渲染编辑/恢复；多 tier 不折叠丢失；编辑抽屉不能增删 key；移动端抽屉全屏。

- [ ] **Step 2: 实现紧凑模型定价页面**

页面使用现有后台表格、筛选和 Drawer 习惯，显示 family、canonical ID、官方基线摘要、当前有效价、`官方|自定义`、核验日期和操作。编辑只开放每行 `price`、source URL、verified date；保存提交全量 rates 和当前 version。恢复官方价二次确认；409 提示数据已变更并重新取详情，不覆盖他人修改。

- [ ] **Step 3: 严格执行前端权限**

页面路由依赖 `ai_model_pricing_list`；所有写按钮和写请求同时检查 `userStore.can('ai_model_pricing_edit')`。只有 list 权限时完整只读；没有 list 权限时动态菜单和路由均不可达。权限控制只改善 UI，后端仍是最终授权边界。

- [ ] **Step 4: 调整智能体当前价格展示**

把“官方模型定价”改为“当前模型价格”，展示来源、基础 rates、智能体倍率和倍率后参考单价。`fixed-decimal.ts` 用十进制字符串转 `BigInt + scale` 相乘再格式化，不使用 `Number/parseFloat/toFixed`；参考价明确是展示值，最终账单仍以 Run 快照和后端一次取整为准。智能体表单不新增基础价字段。

- [ ] **Step 5: 生成路由/i18n，运行定向检查并提交**

```powershell
npm run locale:generate
npm run routes:generate
npx vitest run tests/shared/ai/ai-model-pricing-api.test.ts tests/shared/ai/ai-model-pricing-decimal.test.ts tests/component/ai/ModelPricingPage.test.ts tests/shared/ai/ai-agent-billing-config.test.ts
npm run contract:check
npm run locale:check
npm run routes:check
git diff --check
git add contracts/backend/admin/v1 src tests
git commit -m "feat(ai): manage GPT and Claude prices"
```

预期：四个定向测试文件及生成物检查通过；不运行完整 `npm test`、typecheck、build 或 Playwright。

## 用户手动验证（可选，不由执行 Agent 自动运行）

```powershell
# 后端全量与 race
go test ./...
go test -race ./...

# Atlas/Docker 数据库校验；仍不代表自动应用业务库迁移
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations

# 前端完整门禁
npm run typecheck
npm test
npm run build
```

上线前由用户手动执行 migration，并在角色管理中给目标管理员授予 `ai_model_pricing_list` / `ai_model_pricing_edit`；先授权 list 才会出现“模型定价”菜单。
