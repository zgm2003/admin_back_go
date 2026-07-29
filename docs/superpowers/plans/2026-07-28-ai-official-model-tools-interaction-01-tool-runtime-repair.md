# AI 工具调用运行时修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Do not dispatch subagents. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让已注册的 `admin_user_count` 在 Durable Worker 对话中完成工具发现、执行、二次模型请求、结算和审计，生产依赖缺失时明确失败。

**Architecture:** Admin API 与 Worker 都按 `ToolRepository -> DefaultExecutors -> ToolService -> ChatService.ToolRuntime` 装配。聊天生产构造器强制工具运行时非空；工具服务只暴露已注册低风险工具，并用 JSON Schema 验证参数和结果。

**Tech Stack:** Go、GORM、MySQL 8、`github.com/santhosh-tekuri/jsonschema/v6@v6.0.2`、OpenAI-compatible Adapter。

**Spec:** `docs/superpowers/specs/2026-07-28-ai-chat-capability-tools-interaction-design.md` 第 6 节。

---

## 执行边界

- 不修改 `internal/module/ai/knowledge/**`，不把注入 `KnowledgeRuntime` 算作知识库修复。
- 不执行模型生成的 Go、JavaScript、SQL 或 Shell；只调用 `DefaultExecutors` 注册的 Go executor。
- 不自动提交 Git。

### Task 1：锁死生产聊天服务的工具依赖

**Files:**
- Modify: `internal/module/ai/chat/service.go`
- Test: `internal/module/ai/chat/service_test.go`
- Modify: `internal/platform/admin/build.go`
- Test: `internal/platform/admin/build_test.go`
- Modify: `internal/runtime/worker.go`
- Test: `internal/runtime/worker_test.go`

- [ ] **Step 1：先写失败测试**

增加：

```go
func TestNewRuntimeServiceRejectsMissingToolRuntime(t *testing.T)
func TestBuildUsesRuntimeChatConstructorWithDefaultToolRuntime(t *testing.T)
func TestWorkerUsesRuntimeChatConstructorWithDefaultToolRuntime(t *testing.T)
```

前者断言空 `ToolRuntime` 返回 `ErrToolRuntimeNotConfigured`；后两项沿用现有 production composition source test 风格，断言 API 与 Worker 都创建 `aitool.NewGormRepository`、`aitool.DefaultExecutors` 并调用 `aichat.NewRuntimeService`。

运行：

```powershell
go test ./internal/module/ai/chat ./internal/platform/admin ./internal/runtime -run 'Test(NewRuntimeService|BuildUsesRuntimeChat|WorkerUsesRuntimeChat)'
```

预期：FAIL，生产构造器不存在，Worker 也没有工具装配。

- [ ] **Step 2：实现生产构造边界**

在 `chat/service.go` 增加：

```go
var ErrToolRuntimeNotConfigured = errors.New("AI tool runtime is not configured")

func NewRuntimeService(deps Dependencies) (*Service, error) {
    if deps.ToolRuntime == nil {
        return nil, ErrToolRuntimeNotConfigured
    }
    return NewService(deps), nil
}
```

保留 `NewService` 供不涉及工具的窄单测。Admin Build 与 Worker 生产路径改用 `NewRuntimeService` 并传播错误；Worker 在创建聊天服务前明确构造：

```go
toolRepository := aitool.NewGormRepository(resources.DB)
toolRuntime := aitool.NewService(toolRepository, aitool.DefaultExecutors(toolRepository))
```

API 原有草稿生成和价格依赖继续通过 options 注入，Worker 不装配后台草稿生成能力。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，并证明生产路径不再允许静默缺少工具运行时。

### Task 2：只向模型暴露真正可执行的低风险工具

**Files:**
- Modify: `internal/module/ai/tool/service.go`
- Test: `internal/module/ai/tool/service_test.go`
- Modify: `internal/module/ai/tool/repository.go`
- Test: `internal/module/ai/tool/repository_gorm_test.go`

- [ ] **Step 1：写工具发现失败测试**

增加：

```go
func TestListRuntimeToolsReturnsOnlyLowRiskRegisteredValidTools(t *testing.T)
func TestGormRepositoryListRuntimeToolsUsesActiveBindingAndToolPredicates(t *testing.T)
```

输入同时包含禁用绑定、禁用工具、`medium/high`、未注册 code、损坏参数 Schema、损坏结果 Schema 和合法 `admin_user_count`。只允许最后一项返回；Schema 损坏必须返回稳定内部错误，不能跳过后假装配置正常。

运行：

```powershell
go test ./internal/module/ai/tool -run 'Test(ListRuntimeToolsReturnsOnly|GormRepositoryListRuntimeTools)'
```

预期：FAIL，当前服务仍暴露高风险和未注册工具，且缺 repository SQL 测试。

- [ ] **Step 2：实现发现规则**

`ListRuntimeTools` 固定按以下顺序校验：工具启用、绑定启用、`risk_level == RiskLow`、`executorRegistered(code)`、参数与结果 Schema 均可解析。Repository 保留 SQL 层启用过滤，Service 作为最终安全边界再次检查。

`UpdateAgentTools` 同样拒绝未注册或非低风险工具，避免管理端显示“已绑定”但运行时永远不可执行。

- [ ] **Step 3：验证通过**

运行 Step 1 命令。预期：PASS，API 与 Worker 对同一绑定使用同一个 `ListRuntimeTools` 结果。

### Task 3：验证工具参数、结果并保存确定终态

**Files:**
- Create: `internal/module/ai/tool/schema.go`
- Create: `internal/module/ai/tool/schema_test.go`
- Modify: `internal/module/ai/tool/service.go`
- Modify: `internal/module/ai/tool/repository.go`
- Test: `internal/module/ai/tool/service_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1：锁定失败语义**

增加：

```go
func TestExecuteRejectsInvalidJSONBeforeExecutorAndAuditsFailure(t *testing.T)
func TestExecuteRejectsArgumentsOutsideSchema(t *testing.T)
func TestExecuteRejectsResultOutsideSchema(t *testing.T)
func TestExecuteMarksTimeoutAndNeverReportsSuccess(t *testing.T)
func TestExecuteRejectsNonLowRiskToolAtRuntime(t *testing.T)
```

每项断言 `StartToolCall` 先发生、executor 是否被调用符合预期、`FinishToolCall` 恰好一次，并分别落 `failed` 或 `timeout`。非法 JSON 的 `arguments_json` 保存合法摘要信封：

```json
{"invalid_json":true,"sha256":"<64 lowercase hex>","byte_length":17}
```

运行：

```powershell
go test ./internal/module/ai/tool -run 'TestExecute(Rejects|Marks)'
```

预期：FAIL，当前非法 JSON 会被 `compactJSON` 转成 `{}`，且未校验 Schema。

- [ ] **Step 2：实现闭合 JSON Schema 校验器**

引入并锁定 `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2`。`schema.go` 提供：

```go
func validateJSONAgainstSchema(schema map[string]any, raw json.RawMessage) error
func invalidJSONAuditEnvelope(raw json.RawMessage) json.RawMessage
```

Compiler 固定 `jsonschema.Draft2020`，资源只从内存 `urn:admin:ai-tool-schema` 加载；自定义 `URLLoader` 对任何外部 `$ref` 返回错误，禁止运行时网络取 Schema。实例用 `json.Decoder.UseNumber()` 解码，拒绝尾随 JSON。

`Execute` 顺序固定为：创建审计 -> 低风险检查 -> executor 检查 -> JSON 解码 -> 参数 Schema -> timeout executor -> 结果序列化 -> 结果 Schema -> 写唯一终态。`FinishToolCall` 失败必须向上返回，不能报告调用成功。

- [ ] **Step 3：验证通过**

```powershell
go test ./internal/module/ai/tool
go mod verify
```

预期：PASS；非法 JSON 原始字节不被当成 `{}`，也不把原始敏感内容写入审计。

### Task 4：验证 Durable 两轮工具调用和结算证据

**Files:**
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/runtime/ai_billing_finalizer_test.go`
- Create: `internal/module/ai/replycommand/tool_runtime_integration_test.go`

- [ ] **Step 1：增加失败测试**

增加：

```go
func TestPaidConversationReplyCarriesToolsAcrossTwoPreparedAttempts(t *testing.T)
func TestAdminUserCountDurableRoundPersistsToolAndSettlesRun(t *testing.T)
```

第一项使用 fake paid executor 锁定：首次 `ChatInput.Tools` 含 `admin_user_count`，第二次同时含原 assistant `ToolCalls` 和 `ToolOutputs`，两次完整 usage 都进入结算。

集成测试使用仓库既有 `ADMIN_DURABLE_WORK_INTEGRATION=1` 与 `MYSQL_DSN` 门禁，在 disposable 数据库创建用户、钱包、Provider、映射、Agent、工具绑定和会话；Provider fake 第一轮返回工具调用及完整 usage，第二轮返回最终文本及完整 usage。

运行：

```powershell
go test ./internal/module/ai/chat ./internal/runtime -run 'TestPaidConversationReplyCarriesToolsAcrossTwoPreparedAttempts'
$env:ADMIN_DURABLE_WORK_INTEGRATION='1'; go test ./internal/module/ai/replycommand -run 'TestAdminUserCountDurableRoundPersistsToolAndSettlesRun' -count=1
```

首次预期：FAIL，Worker 生产链路与完整数据库证据尚未闭合。第二条仅在已提供 disposable `MYSQL_DSN` 时执行；缺环境时测试明确 SKIP。

- [ ] **Step 2：闭合断言**

集成测试最终直接查询并断言：

```text
ai_provider_attempts = 2
attempt 1 prepared_request_json 含 tools
ai_tool_calls = 1 且 status = success
attempt 2 prepared_request_json 含 assistant tool_calls 与 role=tool
ai_runs.status = success
ai_runs.billing_status = settled
wallet_holds.status = captured
```

用 `t.Cleanup` 只删除本测试按唯一后缀创建的行。

- [ ] **Step 3：阶段验证**

```powershell
go test ./internal/module/ai/tool ./internal/module/ai/chat ./internal/platform/admin ./internal/runtime
git diff --check
git status --short
```

预期：全部单元测试 PASS；集成测试在未开启门禁时 SKIP、开启时 PASS；没有知识库文件改动，没有 Git commit。
