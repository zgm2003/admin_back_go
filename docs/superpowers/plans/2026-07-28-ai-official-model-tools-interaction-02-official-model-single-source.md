# AI 官方模型唯一信源与后端最终结构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用版本化 `OfficialModelCatalog` 统一模型身份、生命周期、能力、限制和官方价格，并一次性完成后端、数据库、API、RBAC 与审计的“官方模型”最终命名。

**Architecture:** `officialmodel` 拥有目录、严格匹配、当前价格覆盖和唯一 Resolver；`pricing` 只保留纯计价函数。Provider 模型保存自动映射事实，Agent 不保存官方能力或最大输出；消息受理和 Worker 在付费前通过同一个 Resolver 与有效能力计算器 fail closed。

**Tech Stack:** Go、Gin、GORM、MySQL 8、Atlas、Admin Contract、OpenAI-compatible Adapter。

**Spec:** `docs/superpowers/specs/2026-07-28-ai-chat-capability-tools-interaction-design.md` 第 3-5 节。

---

## 执行边界

- 本计划与计划 03 属于同一发布窗口；本计划单独完成后不得部署。
- 不保留 `modelpricing` 包、旧 API、旧权限、旧菜单路径、旧表、双写或请求兼容。
- 用户会重新初始化数据库；活动迁移只描述最终结构，不迁移当前本地业务数据。
- 历史 PricingSnapshot 只读解析可以保留；新请求不得再携带 `max_tokens`。
- 不修改知识库/RAG 实现，不开放文档解析入口。
- `max_history` 明确标记为过渡参数，后续由独立上下文工程的 `ContextBuilder` 接管并删除；本计划不实现该重构。
- 不自动提交 Git。

### Task 1：建立完整、可审查的 OfficialModelCatalog

**Files:**
- Create: `internal/module/ai/officialmodel/catalog.go`
- Create: `internal/module/ai/officialmodel/catalog_test.go`
- Create: `internal/module/ai/officialmodel/capability.go`
- Create: `internal/module/ai/officialmodel/capability_test.go`
- Create: `internal/module/ai/officialmodel/catalog/official_models_v1.json`
- Remove: `internal/module/ai/pricing/catalog.go`
- Remove: `internal/module/ai/pricing/catalog_test.go`
- Remove: `internal/module/ai/pricing/catalog/official_numeric_parity_v1.json`
- Remove: `internal/module/ai/pricing/catalog/official_numeric_parity_v2.json`
- Remove: `internal/module/ai/pricing/catalog/official_numeric_parity_v3.json`

- [ ] **Step 1：先写目录失败测试**

增加：

```go
func TestOfficialCatalogRejectsDuplicateCaseSensitiveIdentity(t *testing.T)
func TestOfficialCatalogRejectsUnprovedOrInconsistentCapabilities(t *testing.T)
func TestOfficialCatalogMatchesOnlyCanonicalIDOrReviewedAlias(t *testing.T)
func TestOfficialCatalogDefaultHasCompleteSourcesAndLimits(t *testing.T)
```

测试锁定：canonical ID 与 alias 全目录大小写敏感唯一；禁止 trim 后空值；生命周期只能是 `active|deprecated|retired`；`0 < max_output_tokens <= context_window_tokens`；图片模态与 `image_input` 同时存在；原生文件模态与 `native_file_input` 一致；参数只允许 `temperature`；来源 URL、`retrieved_at`、`review_after` 完整；禁止模糊、忽略大小写或名称前缀匹配。

运行：

```powershell
go test ./internal/module/ai/officialmodel -run 'TestOfficialCatalog'
```

预期：FAIL，包和统一目录尚不存在。

- [ ] **Step 2：定义闭合类型与目录入口**

固定公开类型：

```go
type LifecycleStatus string
const (LifecycleActive LifecycleStatus = "active"; LifecycleDeprecated LifecycleStatus = "deprecated"; LifecycleRetired LifecycleStatus = "retired")

type Model struct {
    CatalogVersion string
    CatalogVendor string
    ModelFamily string
    ModelID string
    Aliases []string
    LifecycleStatus LifecycleStatus
    ContextWindowTokens int64
    MaxOutputTokens int64
    Capabilities Capabilities
    PriceBook pricing.PriceBook
}

type Catalog struct {
    version string
    models []Model
    byCanonical map[string]int
    byAlias map[string]int
}

func (c *Catalog) ResolveIdentity(requestedID string) (Model, error)
func (c *Catalog) Models() []Model
```

`ResolveIdentity` 只 trim 首尾空白，先大小写敏感匹配 canonical ID，再匹配受审 alias；0 项返回 `ErrModelUnmapped`，多项返回 `ErrModelAmbiguous`。不从 model ID 猜厂商、模态或参数。

- [ ] **Step 3：录入 v1 目录并验证**

把当前 v3 的 24 个模型迁入统一文件；每个模型逐项补齐 spec 第 5.1 节字段。没有厂商第一方证据的能力写为关闭或空集合，不能凭模型名称补全。运行 Task 1 全包测试，预期 PASS。

### Task 2：将价格聚合收口为唯一 officialmodel.Resolver

**Files:**
- Create: `internal/module/ai/officialmodel/model.go`
- Create: `internal/module/ai/officialmodel/repository.go`
- Create: `internal/module/ai/officialmodel/repository_gorm.go`
- Create: `internal/module/ai/officialmodel/resolver.go`
- Create: `internal/module/ai/officialmodel/service.go`
- Create: `internal/module/ai/officialmodel/resolver_test.go`
- Create: `internal/module/ai/officialmodel/service_test.go`
- Create: `internal/module/ai/officialmodel/repository_gorm_test.go`
- Modify: `internal/module/ai/pricing/quote.go`
- Modify: `internal/module/ai/pricing/tier.go`
- Modify: `internal/module/ai/pricing/usage.go`
- Remove: `internal/module/ai/modelpricing/**`

- [ ] **Step 1：写 Resolver 失败测试**

```go
func TestResolverReturnsCatalogFactsWithEffectiveOverridePrice(t *testing.T)
func TestResolverNeverFallsBackFromCorruptOverride(t *testing.T)
func TestResolverPreservesOverrideUntilExplicitRestore(t *testing.T)
func TestResolverRejectsUnmappedAmbiguousAndMissingPrice(t *testing.T)
```

运行：

```powershell
go test ./internal/module/ai/officialmodel ./internal/module/ai/pricing -run 'Test(Resolver|PriceBook)'
```

预期：FAIL，Resolver 尚未实现且业务仍依赖 `modelpricing.Resolver`。

- [ ] **Step 2：实现最终接口**

`pricing` 只保留纯值与纯函数：

```go
type PriceBook struct {
    ModelID string
    ContextTierThresholdTokens int64
    Rates []Rate
}
```

唯一业务解析接口：

```go
type Resolver interface {
    Resolve(context.Context, string) (ResolvedModel, error)
}

type ResolvedModel struct {
    Model Model
    EffectivePrice pricing.PriceBook
    PriceSource string
    OverrideVersion uint64
    PriceSourceURL string
    PriceVerifiedAt time.Time
}
```

无覆盖返回目录官方基准；合法覆盖替换完整 rate 集；数据库错误、损坏覆盖、缺价直接失败。所有业务消费者只能注入 `officialmodel.Resolver`，不得直接加载目录 JSON 或读取价格表。

- [ ] **Step 3：验证通过并清除包引用**

```powershell
go test ./internal/module/ai/officialmodel ./internal/module/ai/pricing
rg -n 'modelpricing|pricing\.Default|official_numeric_parity' internal --glob '!**/*_testdata/**'
```

预期：测试 PASS；搜索无活动代码命中。

### Task 3：写最终数据库结构并删除 Agent 最大输出

**Files:**
- Move: `database/migrations/202607270103_ai_model_pricing.sql` -> `database/legacy-migrations/202607270103_ai_model_pricing.sql`
- Create: `database/migrations/202607280101_ai_official_models.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `database/schema/admin.hcl`
- Modify: `database/reconciliation/030_verify_schema.sql`
- Modify: `database/reconciliation/031_verify_relations.sql`
- Modify: `database/seeds/admin_permissions.sql`
- Move: `internal/architecture/ai_model_pricing_schema_test.go` -> `internal/architecture/ai_official_model_schema_test.go`
- Modify: `internal/module/ai/agent/model.go`
- Modify: `internal/module/ai/agent/dto.go`
- Modify: `internal/module/ai/agent/repository.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/transport/admin/request.go`
- Test: `internal/module/ai/agent/service_test.go`

- [ ] **Step 1：先写最终 schema 与 Agent 契约失败测试**

```go
func TestAIOfficialModelFinalSchemaUsesOnlyCanonicalNames(t *testing.T)
func TestAgentContractHasNoConfigurableMaxOutputTokens(t *testing.T)
```

测试断言最终表为 `ai_official_model_price_overrides`、`ai_official_model_price_override_rates`；`ai_provider_models` 有 `official_model_id`、`official_catalog_version`、`mapping_status`、`mapped_at`；`ai_agents` 无 `max_output_tokens`；权限为 `ai_official_model_list`、`ai_official_model_price_sync`，不写默认角色授权。

运行：

```powershell
go test ./internal/architecture ./internal/module/ai/agent -run 'Test(AIOfficialModel|AgentContract)'
```

预期：FAIL，当前仍是旧表名且 Agent 保存上限。

- [ ] **Step 2：落实最终 DDL 与 Agent DTO**

活动迁移先按外键顺序删除旧价格明细表和头表，再创建新表，并删除 `ai_agents.max_output_tokens`；不读、不复制旧覆盖数据。Provider mapping 约束固定为 `mapped|unmapped`；`mapped` 要求 official ID、catalog version、mapped_at 全部非空，`unmapped` 要求三者为空。

Agent create/update/list/detail/page-init 删除 `max_output_tokens` 与 `max_output_tokens_default`；只读模型摘要中的 `official_model.max_output_tokens` 保留。Repository 查询不再选择或更新 Agent 列。

- [ ] **Step 3：生成 Atlas hash 并验证**

```powershell
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations
go test ./internal/architecture ./internal/module/ai/agent
pwsh -NoProfile -File scripts/verify-database.ps1
```

预期：空库和导入 fixture 均收敛到同一最终 schema；当前本地业务数据不作为兼容目标。

### Task 4：严格映射 Provider 模型并执行生命周期

**Files:**
- Modify: `internal/module/ai/provider/model.go`
- Modify: `internal/module/ai/provider/dto.go`
- Modify: `internal/module/ai/provider/service.go`
- Modify: `internal/module/ai/provider/repository.go`
- Test: `internal/module/ai/provider/service_test.go`
- Modify: `internal/module/ai/agent/service.go`
- Test: `internal/module/ai/agent/service_test.go`
- Create: `internal/module/ai/officialmodel/reconcile.go`
- Test: `internal/module/ai/officialmodel/resolver_test.go`

- [ ] **Step 1：写映射和生命周期失败测试**

```go
func TestProviderSyncStoresExactOfficialModelMapping(t *testing.T)
func TestProviderSyncLeavesCaseMismatchAndUnknownModelUnmapped(t *testing.T)
func TestAgentSelectionAllowsOnlyMappedActiveRoutes(t *testing.T)
func TestLifecycleDeprecatedKeepsExistingCallButRejectsSelection(t *testing.T)
func TestLifecycleRetiredRejectsCallBeforeBilling(t *testing.T)
func TestDisabledProviderRouteDoesNotRetireOfficialModel(t *testing.T)
```

运行：

```powershell
go test ./internal/module/ai/provider ./internal/module/ai/agent ./internal/module/ai/officialmodel -run 'Test(ProviderSync|AgentSelection|Lifecycle|DisabledProvider)'
```

预期：FAIL，Provider 模型还没有映射事实。

- [ ] **Step 2：统一创建、更新、同步和目录升级 reconciliation**

Provider 创建、更新、`SyncModels` 全部调用同一个 matcher；管理员不能提交 official ID。目录升级 reconciliation 逐行重算映射并更新目录版本，不改变 Provider route 的启停状态。

新建/切换 Agent 只允许 `mapped + route enabled + official active`；已有 deprecated 允许调用但不能重新选择；retired、未映射或当前渠道停用在冻结前拒绝。禁止自动切换到其他 Provider。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，大小写不一致不会误映射。

### Task 5：计算服务端有效能力并权威校验消息附件与参数

**Files:**
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Test: `internal/infra/ai/openaicompat/client_test.go`
- Create: `internal/module/ai/officialmodel/effective_capability.go`
- Test: `internal/module/ai/officialmodel/capability_test.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/transport/admin/request.go`
- Test: `internal/module/ai/message/service_test.go`
- Test: `internal/module/ai/message/transport/admin/handler_test.go`
- Create: `internal/infra/storage/cos/object_inspector.go`
- Create: `internal/infra/storage/cos/object_inspector_test.go`
- Modify: `internal/runtime/providers.go`
- Modify: `internal/platform/admin/build.go`

- [ ] **Step 1：写能力与伪造请求失败测试**

```go
func TestEffectiveCapabilitiesCanOnlyNarrowOfficialFacts(t *testing.T)
func TestSendRejectsMaxTokensEvenWhenJSONIsForged(t *testing.T)
func TestSendRejectsUnsupportedTemperature(t *testing.T)
func TestSendRejectsImageWithoutEffectiveImageCapability(t *testing.T)
func TestSendUsesTrustedObjectMetadataForMimeAndSize(t *testing.T)
func TestSendNeverAcceptsNativeDocumentInThisRelease(t *testing.T)
```

运行：

```powershell
go test ./internal/module/ai/officialmodel ./internal/module/ai/message ./internal/infra/storage/cos -run 'Test(EffectiveCapabilities|Send|ObjectInspector)'
```

预期：FAIL，当前图片无模型能力校验，附件只信 URL/name/size，`max_tokens` 仍被接受。

- [ ] **Step 2：实现能力交集与受信附件检查**

有效能力固定为：

```text
OfficialModel ∩ Transport ∩ ProviderRoute ∩ AgentPolicy ∩ PlatformImplemented
```

`infraai.CapabilityMetadata` 增加传输层的 tools、streaming、structured output、input/output modalities 和 supported parameters；它只能收窄官方事实。

Attachment DTO 增加 `mime_type` 与受信 object key。`ObjectInspector.Head(ctx, key)` 从当前启用 COS 配置读取真实 `Content-Type`、`Content-Length`，验证 key 位于 `ai_chat_images/` 受信空间；最多 5 张图片并发检查，任一失败则整条消息不受理。服务只接受官方能力返回的 MIME、数量和大小。URL、客户端 size、扩展名不能覆盖 HEAD 事实。

消息请求改为严格 JSON decode，未知 `runtime_params` key 直接 400；允许字段只有 capability-gated `temperature` 与过渡期 `max_history`。`max_tokens` 无论数值是否合法都返回 `request.invalid`。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS；文档附件入口仍关闭，平台文本解析不被标记成模型原生文件。

### Task 6：用官方上限稳定组装请求、冻结与结算

**Files:**
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Test: `internal/infra/ai/openaicompat/client_test.go`
- Modify: `internal/module/ai/aigateway/contracts.go`
- Modify: `internal/module/ai/aigateway/quote_validator.go`
- Test: `internal/module/ai/aigateway/quote_validator_test.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Test: `internal/runtime/ai_billing_finalizer_test.go`
- Test: `internal/module/ai/chat/service_test.go`

- [ ] **Step 1：写输出上界一致性失败测试**

```go
func TestPaidChatAssemblerConvergesPreparedRequestAndSafeOutputBound(t *testing.T)
func TestPaidChatAssemblerFailsClosedWhenBoundDoesNotConverge(t *testing.T)
func TestInsufficientBalanceCreatesNoProviderAttempt(t *testing.T)
func TestSettlementUsesCompleteUsageAndReleasesHoldDifference(t *testing.T)
func TestOpenAIAdapterUsesSystemEffectiveMaxOutputTokens(t *testing.T)
```

运行：

```powershell
go test ./internal/infra/ai/openaicompat ./internal/module/ai/aigateway ./internal/module/ai/chat ./internal/runtime -run 'Test(PaidChatAssembler|InsufficientBalance|SettlementUses|OpenAIAdapter)'
```

预期：FAIL，系统 cap 仍借用 `Inputs["max_tokens"]`，prepared bytes 与 cap 没有稳定收敛契约。

- [ ] **Step 2：实现确定性收敛**

`infraai.ChatInput` 增加：

```go
EffectiveMaxOutputTokens int
```

Adapter 只从该字段生成上游 `max_tokens` 或等价字段；`Inputs` 只包含允许用户控制的参数。

Assembler 最多迭代 4 次：以官方最大输出构造 -> 计算实际 prepared request 安全输入上界 -> 计算 `min(official max, context window - safe input)` -> 用新上界重构。连续两轮 body hash 和上界同时不变才算收敛；非正、振荡或 4 次后未收敛，在冻结与插入 attempt 前失败。

稳定后的同一份 prepared bytes、hash、上界、目录版本和 canonical ID 同时写 Quote、Run 快照、Attempt 与请求指纹。余额不足不得创建 attempt；结算按所有成功 attempt 的完整真实 usage 捕获，释放冻结差额。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，用户参数与系统安全上界不再混用。

### Task 7：发布“官方模型”Admin API、RBAC、Graph 与开发契约

**Files:**
- Create: `internal/module/ai/officialmodel/dto.go`
- Create: `internal/module/ai/officialmodel/transport/admin/request.go`
- Create: `internal/module/ai/officialmodel/transport/admin/handler.go`
- Create: `internal/module/ai/officialmodel/transport/admin/handler_test.go`
- Create: `internal/module/ai/officialmodel/transport/admin/route.go`
- Create: `internal/module/ai/officialmodel/transport/admin/route_test.go`
- Modify: `internal/platform/admin/graph.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/server/routes_admin_ai.go`
- Modify: `internal/admincontract/views.go`
- Modify: `internal/admincontract/views_test.go`
- Modify: `internal/admincontract/permissions_test.go`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `docs/architecture.md`
- Modify: `internal/module/README.md`
- Modify: `internal/platform/README.md`

- [ ] **Step 1：写最终路由与只读事实测试**

固定接口：

```text
GET    /api/admin/v1/ai-official-models/page-init
GET    /api/admin/v1/ai-official-models
GET    /api/admin/v1/ai-official-models/:model_id
PUT    /api/admin/v1/ai-official-models/:model_id/price
DELETE /api/admin/v1/ai-official-models/:model_id/price-override
```

增加 `TestOfficialModelRoutesUseFinalPermissionsAndAudit`、`TestOfficialModelPriceMutationCannotChangeCatalogFacts`、`TestAdminViewsPublishOfficialModelRoute`。GET 使用 `ai_official_model_list`；PUT/DELETE 使用 `ai_official_model_price_sync`；写审计 module=`ai_official_model`。

运行：

```powershell
go test ./internal/module/ai/officialmodel/transport/admin ./internal/admincontract ./internal/server -run 'Test.*OfficialModel'
```

预期：FAIL，最终路由和 Graph 尚未发布。

- [ ] **Step 2：实现 API 与 Graph**

列表支持 vendor、family、lifecycle、input modality、model ID；详情返回身份、能力、限制、官方价、当前价和来源。PUT 只接受完整 rates、`expected_version`、`source_url`、`verified_at`；DELETE 要求 `expected_version`。retired 禁止新建价格覆盖，已有历史覆盖只读展示。

Graph 字段固定为 `OfficialModels`；菜单 `/ai/official-models`、view `ai/official-models`、i18n key `menu.ai_official_models`。

- [ ] **Step 3：生成临时开发契约并执行旧名清零**

```powershell
$sha=(git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $sha -Out "$env:TEMP/admin-contract-official-model"
go run ./cmd/admin-contract check --out "$env:TEMP/admin-contract-official-model" --commit $sha
rg -n 'modelpricing|model-pricing|ai-model-prices|ai_model_pricing|ai_model_price_overrides|ai_model_price_override_rates' internal database/schema database/reconciliation database/seeds docs/architecture.md internal/module/README.md internal/platform/README.md
rg -n 'modelpricing|model-pricing|ai-model-prices|ai_model_pricing|ai_model_price_overrides|ai_model_price_override_rates' "$env:TEMP/admin-contract-official-model"
```

预期：临时契约校验 PASS；旧名搜索无结果。`docs/superpowers/**`、`database/legacy-migrations/**` 和 Git 历史不属于清零范围。

- [ ] **Step 4：后端阶段验证**

```powershell
go test ./internal/module/ai/... ./internal/infra/ai/... ./internal/infra/storage/cos ./internal/platform/admin ./internal/admincontract ./internal/server ./internal/runtime ./internal/architecture
git diff --check
git status --short
```

预期：代码测试与临时契约检查通过；正式 bundle 留到计划 03 使用真实后端提交生成。不得部署，不创建 Git commit。
