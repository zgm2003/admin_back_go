# AI 对话原生文件附件 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不引入本地解析、RAG 或 Responses API 的前提下，让官方支持文件输入且渠道显式开启协议的 AI 对话可靠接收图片与原生文件，并以可恢复、内存有界、按权威 usage 结算的方式派发到 Chat Completions 上游。

**Architecture:** 官方模型目录、OpenAI-compatible transport、供应商 `file_input_mode` 和 Admin Chat 平台实现能力逐层求交集；消息层只持久化经 COS HEAD 校验的附件引用。含文件请求保存 `openai_chat_file_manifest_v1` 规范清单，Worker 通过 ETag 条件读取和流式 Base64 物化上游 body；纯文本与历史图片继续使用现有 inline prepared request。文件请求按官方 context window 与 max output 两个独立财务上界冻结，最终仍只按完整上游 usage 结算。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Atlas、腾讯云 COS SDK、OpenAI-compatible Chat Completions、Vue 3、TypeScript 5.9、Zod 4、Vitest、Element Plus、Admin Contract 生成链路。

---

## 执行边界

- 后端路径均相对 `E:\admin\admin_back_go`；前端路径均相对 `E:\admin\admin_front_ts`。
- 实施前使用 `superpowers:using-git-worktrees` 创建隔离工作树；每个 Task 按测试先行、最小实现、定向验证、独立提交执行。
- 官方模型目录继续是模型能力、context window、max output 和价格的唯一信源；前端、Agent、Provider 都不能复制可编辑的模型能力。
- 通用上传配置继续允许管理员设置 100 MB；AI 层固定叠加单条消息 5 个附件、总量 50 MiB、单个原生文件严格小于 50 MiB、一次请求历史原生文件合计不超过 50 MiB。
- 不把文件内容、Base64、完整物化请求、对象临时凭证或完整 object key 写入 MySQL、日志、Run 详情或审计。
- 不执行 Playwright、全仓长脚本、真实收费自动化、长时间压力测试；真实渠道和付费链路在 Task 9 由用户手工验收。
- 不新增菜单、路由族、RBAC 权限或文件内容表；供应商字段复用现有供应商管理权限，附件继续保存在 `ai_messages.meta_json`。
- 不切换 Responses API，不做 PDF/Office 本地解析，不做知识库、切片、向量化或 fallback。

## 文件结构

### 后端新增

- `database/migrations/202607300102_ai_chat_native_file_attachments.sql`：为 `ai_providers` 增加闭合的 `file_input_mode` 字段。
- `internal/module/ai/capability/attachment_policy.go`：集中拥有 AI 图片/文件扩展名、MIME、数量与大小常量，以及准确的关闭原因。
- `internal/module/ai/capability/attachment_policy_test.go`：证明系统上传规则与 AI 官方子集的交集及四类关闭原因。
- `internal/shared/uploadpolicy/policy.go`：定义当前启用系统上传规则的只读跨模块契约。
- `internal/module/uploadtoken/rule_policy.go`：从现有启用 upload setting/rule 解析规范扩展名与单文件上限。
- `internal/module/uploadtoken/rule_policy_test.go`：验证配置缺失、畸形 JSON、扩展名和大小的 fail-closed 行为。
- `internal/infra/storage/cos/object_stream.go`：受信前缀、ETag HEAD、`If-Match` 条件 GET 和 context 控制的流式 reader。
- `internal/infra/storage/cos/object_stream_test.go`：验证条件请求、版本漂移、取消关闭和不执行 `io.ReadAll`。
- `internal/infra/ai/file_input.go`：定义 provider-neutral 的文件引用、manifest schema 和文件请求物化接口。
- `internal/infra/ai/openaicompat/file_manifest.go`：规范化 `file_ref` 清单、精确长度计算和 Chat Completions `file` part 流式物化。
- `internal/infra/ai/openaicompat/file_manifest_test.go`：验证 manifest 稳定 hash、Base64 body、Content-Length、顺序和取消。
- `internal/module/ai/aigateway/native_file_quote_test.go`：验证 `native_file_context_window_v1` 冻结和最终结算边界。

### 后端修改

- `internal/shared/enum/upload.go`、`upload_test.go`：修正并扩充系统扩展名，新增 `ai_chat_attachments` 目录。
- `internal/platform/admin/build.go`、`build_test.go`：复用 uploadtoken 配置源装配规则 resolver、COS inspector 和条件 streamer。
- `internal/module/uploadconfig/dto.go`、`service.go`、`service_test.go`：让请求、列表响应和 page-init 字典共享正式 enum。
- `internal/admincontract/openapi_models_test.go`：证明上传扩展名响应使用现有生成器输出闭合 OpenAPI enum。
- `database/schema/admin.hcl`、`database/migrations/atlas.sum`：同步 Provider 目标 schema 和迁移校验和。
- `internal/module/ai/provider/model.go`、`dto.go`、`service.go`、`service_test.go`、`repository.go`、`transport/admin/request.go`、`transport/admin/handler.go`、`transport/admin/handler_test.go`：贯通 `file_input_mode`。
- `internal/module/ai/agent/model.go`、`repository.go`、`service.go`、`service_test.go`、`dto.go`：携带渠道模式并投影有效附件能力。
- `internal/module/ai/capability/chat.go`、`chat_test.go`：将 Admin Chat 和 transport 文件实现纳入交集，不扩大官方能力。
- `internal/infra/ai/types.go`、`types_json_test.go`：transport 声明 `file` 与多种安全上界策略。
- `internal/module/ai/message/dto.go`、`service.go`、`service_test.go`、`repository.go`、`history_repository.go`、`history_actions_test.go`、`transport/admin/request.go`、`transport/admin/handler_test.go`：统一附件校验、可信对象事实、编辑重发和重新生成。
- `internal/module/ai/chat/dto.go`、`repository.go`、`service.go`、`service_test.go`：历史附件进入最终请求上下文并携带 Provider 文件模式。
- `internal/infra/storage/cos/object_inspector.go`、`object_inspector_test.go`：兼容历史图片目录并返回 ETag。
- `internal/module/ai/aigateway/contracts.go`、`chat_provider.go`、`chat_provider_test.go`、`quote_validator.go`、`quote_validator_test.go`、`gateway.go`、`gateway_test.go`：支持 inline/file 两种 prepared request 与 proof strategy。
- `internal/runtime/ai_billing_gateway.go`、`ai_billing_gateway_test.go`、`worker.go`、`worker_test.go`：组装文件准备、条件流和恢复依赖。
- `internal/infra/ai/openaicompat/client.go`、`client_test.go`：历史附件、manifest 准备和流式派发。
- `internal/module/ai/run/dto.go`、`service.go`、`service_test.go`：仅返回文件请求安全摘要和 COS 时延。
- `internal/shared/apperror/error.go` 及现有 AI 错误映射文件：收敛模型、渠道、类型、大小、上下文、对象版本和上游拒绝错误。
- `contracts/admin/v1/openapi.json`、`contracts/admin/v1/manifest.json`：由生成脚本发布最终后端契约。
- `docs/architecture.md`：记录文件能力交集、manifest、条件流和财务上界。

### 前端修改

- `contracts/backend/admin/v1/**`、`contracts/backend/admin/lock.json`、`src/modules/http/generated/admin.ts`、`operations.ts`：只通过契约同步和生成命令更新。
- `src/api/system/uploadConfig.ts`、`uploadConfig.types.ts`：删除手写扩展名白名单，直接消费闭合生成类型。
- `src/api/ai/providers.ts`、`agents.ts`、`messages.ts`：增加 Provider 模式、原生文件能力和 `image | file` 附件契约。
- `src/views/Main/ai/providers/components/ProviderFormDialog.vue`、`composables/useProviderForm.ts`：显式选择渠道文件协议。
- `src/views/Main/ai/chat/components/MessageInput/use-image-attachments.ts`：重命名为 `use-attachments.ts`，统一队列、校验、上传、重试、删除和去重。
- `src/views/Main/ai/chat/components/MessageInput/index.vue`、`MessageInputToolbar.vue`、`PendingAttachments.vue`：回形针入口、选择/拖拽/粘贴、图片缩略图与文件卡片。
- `src/views/Main/ai/chat/components/MessageList/MessageEditor.vue`、`index.vue`、`src/views/Main/ai/chat/use-chat-page.ts`、`components/MessageInput/capability-transition.ts`：编辑附件、重新生成和 Agent 切换阻断。
- `src/views/Main/ai/runs/components/RunList/RunLatencyBreakdown.vue`：展示安全附件摘要和 COS 耗时，不泄露对象身份。
- `src/i18n/locales/zh-CN/ai.ts`、`en-US/ai.ts`、`generated.ts`：附件状态、能力关闭原因和稳定错误文案。
- `tests/shared/system/upload-config-contract.test.ts`、`tests/shared/ai/ai-chat-capabilities.test.ts`、`tests/component/ai/ChatAttachments.test.ts`、`MessageInteractions.test.ts`、`RunLatencyBreakdown.test.ts`：定向契约与交互测试。

---

### Task 1: 修复上传扩展名并建立 OpenAPI 唯一契约

**Files:**
- Create: `internal/shared/uploadpolicy/policy.go`
- Create: `internal/module/uploadtoken/rule_policy.go`
- Create: `internal/module/uploadtoken/rule_policy_test.go`
- Modify: `internal/shared/enum/upload.go`
- Modify: `internal/shared/enum/upload_test.go`
- Modify: `internal/module/uploadconfig/dto.go`
- Modify: `internal/module/uploadconfig/service.go`
- Modify: `internal/module/uploadconfig/service_test.go`
- Modify: `internal/admincontract/openapi_models_test.go`

- [ ] **Step 1: 先写系统扩展名和响应 enum 的失败测试**

在 `internal/shared/enum/upload_test.go` 固定 canonical 集合，明确 `doc` 只属于文件、`jfif`/`psd` 属于图片、代码文件属于通用文件：

```go
func TestUploadExtensionCatalogIsCanonical(t *testing.T) {
	wantImages := []string{"jpeg", "jpg", "jfif", "pjpeg", "png", "gif", "webp", "bmp", "tif", "tiff", "svg", "ico", "psd", "avif"}
	if !reflect.DeepEqual(UploadImageExts, wantImages) {
		t.Fatalf("image extensions=%v", UploadImageExts)
	}
	for _, ext := range []string{"doc", "docx", "pptx", "xlsx", "md", "json", "ts", "tsx", "go", "py", "sql", "yaml", "zip", "tar"} {
		if !IsUploadFileExt(ext) { t.Errorf("file extension %q is missing", ext) }
	}
	if IsUploadImageExt("doc") { t.Fatal("doc must not be an image extension") }
	if !IsUploadFolder("ai_chat_attachments") { t.Fatal("AI attachment folder is missing") }
}
```

在 `internal/module/uploadtoken/rule_policy_test.go` 证明所有业务层读取的是当前启用规则：

```go
type fakeRuleRepository struct {
	config *EnabledConfig
	err    error
}

func (repository *fakeRuleRepository) GetEnabledConfig(context.Context) (*EnabledConfig, error) {
	return repository.config, repository.err
}

func TestActiveRuleResolverReturnsNormalizedEnabledRule(t *testing.T) {
	repository := &fakeRuleRepository{config: &EnabledConfig{
		SettingID: 1, MaxSizeMB: 100,
		ImageExts: `["png","jpeg"]`, FileExts: `["pdf","md","go","zip"]`,
	}}
	rule, err := NewActiveRuleResolver(repository).ResolveActive(context.Background())
	if err != nil { t.Fatal(err) }
	want := uploadpolicy.Rule{
		MaxFileBytes: 100 << 20,
		ImageExtensions: []string{"jpeg", "png"},
		FileExtensions: []string{"pdf", "md", "go", "zip"},
	}
	if diff := cmp.Diff(want, rule); diff != "" { t.Fatal(diff) }
}

func TestActiveRuleResolverFailsClosedForMissingOrMalformedRule(t *testing.T) {
	for _, repository := range []*fakeRuleRepository{
		{},
		{config: &EnabledConfig{SettingID: 1, MaxSizeMB: 100, ImageExts: `{`, FileExts: `[]`}},
	} {
		if _, err := NewActiveRuleResolver(repository).ResolveActive(context.Background()); err == nil {
			t.Fatal("invalid active upload rule must fail")
		}
	}
}
```

在 `internal/admincontract/openapi_models_test.go` 通过 `mustBuildBundle(t)` 解码 `openapi.json`，断言 Rule 列表响应和 page-init option 的 `value` 都是同一闭合 enum：

```go
func TestUploadRuleResponsePublishesClosedExtensionEnums(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct { Components struct { Schemas map[string]map[string]any `json:"schemas"` } `json:"components"` }
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil { t.Fatal(err) }
	assertStringArrayEnum(t, document.Components.Schemas, "Go_internal_module_uploadconfig_RuleItem_Output", "image_exts", enum.UploadImageExts)
	assertStringArrayEnum(t, document.Components.Schemas, "Go_internal_module_uploadconfig_RuleItem_Output", "file_exts", enum.UploadFileExts)
	assertOptionValueEnum(t, document.Components.Schemas, "Go_internal_module_uploadconfig_UploadImageExtOption_Output", enum.UploadImageExts)
	assertOptionValueEnum(t, document.Components.Schemas, "Go_internal_module_uploadconfig_UploadFileExtOption_Output", enum.UploadFileExts)
}

func assertStringArrayEnum(t *testing.T, schemas map[string]map[string]any, schemaName, property string, want []string) {
	t.Helper()
	properties := schemas[schemaName]["properties"].(map[string]any)
	node := properties[property].(map[string]any)["items"].(map[string]any)
	assertEnumNode(t, schemas, node, want)
}

func assertStringPropertyEnum(t *testing.T, schemas map[string]map[string]any, schemaName, property string, want []string) {
	t.Helper()
	properties := schemas[schemaName]["properties"].(map[string]any)
	assertEnumNode(t, schemas, properties[property].(map[string]any), want)
}

func assertOptionValueEnum(t *testing.T, schemas map[string]map[string]any, schemaName string, want []string) {
	t.Helper()
	properties := schemas[schemaName]["properties"].(map[string]any)
	assertEnumNode(t, schemas, properties["value"].(map[string]any), want)
}

func assertEnumNode(t *testing.T, schemas map[string]map[string]any, node map[string]any, want []string) {
	t.Helper()
	if ref, ok := node["$ref"].(string); ok {
		node = schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
	}
	raw, ok := node["enum"].([]any)
	if !ok { t.Fatalf("enum node=%#v", node) }
	got := make([]string, len(raw))
	for index, value := range raw {
		got[index], ok = value.(string)
		if !ok { t.Fatalf("enum value=%#v", value) }
	}
	if diff := cmp.Diff(want, got); diff != "" { t.Fatal(diff) }
}
```

- [ ] **Step 2: 运行后端定向测试并确认先失败**

Run: `go test ./internal/shared/enum ./internal/shared/uploadpolicy ./internal/module/uploadtoken ./internal/module/uploadconfig ./internal/admincontract -run 'UploadExtensionCatalog|ActiveRuleResolver|UploadRuleResponsePublishesClosedExtensionEnums|Rule' -count=1`

Expected: FAIL，至少指出图片列表仍含 `doc`、缺少 `jfif/avif`、文件列表不完整、active rule resolver 不存在或响应 schema 没有 enum。

- [ ] **Step 3: 用后端 enum 修正目录并为响应 DTO 添加正式校验标签**

`internal/shared/enum/upload.go` 只保留以下有序事实源：

```go
var UploadImageExts = []string{
	"jpeg", "jpg", "jfif", "pjpeg", "png", "gif", "webp", "bmp",
	"tif", "tiff", "svg", "ico", "psd", "avif",
}

var UploadFileExts = []string{
	"pdf", "doc", "docx", "dot", "odt", "rtf", "ppt", "pptx", "pot", "ppa", "pps", "pwz", "wiz",
	"xla", "xlb", "xlc", "xlm", "xls", "xlsx", "xlt", "xlw", "csv", "tsv", "iif",
	"txt", "text", "md", "markdown", "json", "html", "htm", "xml", "css",
	"asm", "bat", "c", "cc", "cpp", "cxx", "h", "hh", "def", "in",
	"js", "mjs", "jsx", "ts", "tsx", "py", "go", "java", "cs", "php", "rb", "rs",
	"sh", "bash", "zsh", "ksh", "ps1", "sql", "pl", "lua", "r", "scala", "swift", "kt", "kts",
	"yaml", "yml", "toml", "ini", "conf", "properties", "proto",
	"eml", "log", "rst", "srt", "vtt", "ics", "ifb", "vcf", "diff", "patch", "zip", "tar",
}
```

`internal/module/uploadconfig/dto.go` 使用专用 option，避免通用 `dict.Option[string]` 丢失响应 enum：

```go
type UploadImageExtOption struct {
	Label string `json:"label"`
	Value string `json:"value" validate:"upload_image_ext"`
}

type UploadFileExtOption struct {
	Label string `json:"label"`
	Value string `json:"value" validate:"upload_file_ext"`
}

type RulePageInitDict struct {
	UploadImageExtArr []UploadImageExtOption `json:"upload_image_ext_arr"`
	UploadFileExtArr  []UploadFileExtOption  `json:"upload_file_ext_arr"`
}

type RuleItem struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	MaxSizeMB int      `json:"max_size_mb"`
	ImageExts []string `json:"image_exts" validate:"dive,upload_image_ext"`
	FileExts  []string `json:"file_exts" validate:"dive,upload_file_ext"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}
```

`service.go` 构造 page-init 时直接将 `enum.UploadImageExts`/`enum.UploadFileExts` 映射到两个专用 option，不能复制另一份字符串集合。

- [ ] **Step 4: 建立当前启用上传规则的只读 resolver**

`internal/shared/uploadpolicy/policy.go` 定义稳定读取契约和测试便利适配器：

```go
type Rule struct {
	MaxFileBytes    int64
	ImageExtensions []string
	FileExtensions  []string
}

type Resolver interface {
	ResolveActive(context.Context) (Rule, error)
}

type ResolverFunc func(context.Context) (Rule, error)

func (resolve ResolverFunc) ResolveActive(ctx context.Context) (Rule, error) {
	return resolve(ctx)
}
```

`uploadtoken.NewActiveRuleResolver(repository)` 复用现有 `Repository.GetEnabledConfig`，解析 JSON 后分别调用 `enum.NormalizeUploadExts`；没有启用配置、`MaxSizeMB <= 0`、MiB 乘法溢出、畸形 JSON 或未知扩展名全部返回错误。它不缓存规则，确保管理员修改后下一次能力查询、上传 token 和消息受理立即使用新事实。

- [ ] **Step 5: 运行后端测试并确认通过**

Run: `gofmt -w internal/shared/enum/upload.go internal/shared/enum/upload_test.go internal/shared/uploadpolicy internal/module/uploadtoken/rule_policy.go internal/module/uploadtoken/rule_policy_test.go internal/module/uploadconfig/dto.go internal/module/uploadconfig/service.go internal/module/uploadconfig/service_test.go internal/admincontract/openapi_models_test.go`

Run: `go test ./internal/shared/enum ./internal/shared/uploadpolicy ./internal/module/uploadtoken ./internal/module/uploadconfig ./internal/admincontract -count=1`

Expected: PASS，OpenAPI 中请求、Rule 响应与 page-init option value 使用相同 enum。

- [ ] **Step 6: 提交后端上传事实**

```bash
git add internal/shared/enum/upload.go internal/shared/enum/upload_test.go internal/shared/uploadpolicy internal/module/uploadtoken/rule_policy.go internal/module/uploadtoken/rule_policy_test.go internal/module/uploadconfig/dto.go internal/module/uploadconfig/service.go internal/module/uploadconfig/service_test.go internal/admincontract/openapi_models_test.go
git commit -m "fix(upload): 统一上传扩展名契约"
```

---

### Task 2: 增加供应商文件协议字段并贯通数据库与 Admin API

**Files:**
- Create: `database/migrations/202607300102_ai_chat_native_file_attachments.sql`
- Modify: `database/schema/admin.hcl`
- Modify: `database/migrations/atlas.sum`
- Modify: `internal/module/ai/provider/model.go`
- Modify: `internal/module/ai/provider/dto.go`
- Modify: `internal/module/ai/provider/repository.go`
- Modify: `internal/module/ai/provider/service.go`
- Modify: `internal/module/ai/provider/service_test.go`
- Modify: `internal/module/ai/provider/transport/admin/request.go`
- Modify: `internal/module/ai/provider/transport/admin/handler.go`
- Create: `internal/module/ai/provider/transport/admin/handler_test.go`
- Modify: `internal/admincontract/openapi_models_test.go`

- [ ] **Step 1: 写 Provider 默认关闭、合法枚举和 DTO 投影失败测试**

在 `internal/module/ai/provider/service_test.go` 增加：

```go
func TestCreatePersistsExplicitFileInputMode(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)
	_, appErr := service.Create(context.Background(), CreateInput{
		Name: "OpenAI", EngineType: "openai", APIKey: "sk-test",
		ModelIDs: []string{"gpt-5.6"}, FileInputMode: FileInputModeChatCompletions, Status: 1,
	})
	if appErr != nil { t.Fatal(appErr) }
	if repo.created == nil || repo.created.FileInputMode != FileInputModeChatCompletions {
		t.Fatalf("created provider=%#v", repo.created)
	}
}

func TestCreateRejectsUnknownFileInputMode(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil)
	_, appErr := service.Create(context.Background(), CreateInput{
		Name: "bad", EngineType: "openai", APIKey: "sk-test",
		ModelIDs: []string{"gpt-5.6"}, FileInputMode: "auto", Status: 1,
	})
	if appErr == nil { t.Fatal("unknown file input mode must be rejected") }
}
```

在 handler 测试中断言请求省略字段会被 binding 拒绝，响应 list/detail 均投影闭合值，page-init 的 `file_input_mode_arr` 只包含 `disabled` 与 `chat_completions`。

在 `internal/admincontract/openapi_models_test.go` 增加，复用 Task 1 的 helper 断言 `ProviderDTO_Output.file_input_mode` 与专用 option value 都等于 `FileInputModes`，不能退化为任意 string：

```go
func TestAIProviderFileInputModeResponseUsesClosedEnum(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct { Components struct { Schemas map[string]map[string]any `json:"schemas"` } `json:"components"` }
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil { t.Fatal(err) }
	assertStringPropertyEnum(t, document.Components.Schemas, "Go_internal_module_ai_provider_ProviderDTO_Output", "file_input_mode", aiprovider.FileInputModes)
	assertOptionValueEnum(t, document.Components.Schemas, "Go_internal_module_ai_provider_FileInputModeOption_Output", aiprovider.FileInputModes)
}
```

- [ ] **Step 2: 运行 Provider 定向测试并确认先失败**

Run: `go test ./internal/module/ai/provider ./internal/admincontract -run 'FileInputMode|Provider' -count=1`

Expected: FAIL，原因是字段和常量尚不存在。

- [ ] **Step 3: 写 fail-closed 迁移、HCL 和 Provider 领域字段**

迁移内容固定为：

```sql
ALTER TABLE `ai_providers`
  ADD COLUMN `file_input_mode` VARCHAR(32) NOT NULL DEFAULT 'disabled' AFTER `base_url`,
  ADD CONSTRAINT `chk_ai_providers_file_input_mode`
    CHECK (`file_input_mode` IN ('disabled', 'chat_completions'));
```

在 `provider/dto.go` 定义唯一业务常量并贯通 DTO：

```go
const (
	FileInputModeDisabled        = "disabled"
	FileInputModeChatCompletions = "chat_completions"
)

var FileInputModes = []string{FileInputModeDisabled, FileInputModeChatCompletions}

type FileInputModeOption struct {
	Label string `json:"label"`
	Value string `json:"value" validate:"oneof=disabled chat_completions"`
}

type InitDict struct {
	EngineTypeArr     []dict.Option[string] `json:"engine_type_arr"`
	FileInputModeArr  []FileInputModeOption `json:"file_input_mode_arr"`
	CommonStatusArr   []dict.Option[int]    `json:"common_status_arr"`
	HealthStatusArr   []dict.Option[string] `json:"health_status_arr"`
	ModelSyncArr      []dict.Option[string] `json:"model_sync_arr"`
}

type CreateInput struct {
	Name              string
	EngineType        string
	BaseURL           string
	APIKey            string
	FileInputMode     string
	ModelIDs          []string
	ModelDisplayNames map[string]string
	Status            int
}
```

`Provider`、`ProviderDTO`、repository scan/update map 和 handler mapping 都使用 `FileInputMode`；`ProviderDTO.FileInputMode` 增加 `validate:"oneof=disabled chat_completions"`，`mutationRequest` 使用闭合校验：

```go
FileInputMode string `json:"file_input_mode" binding:"required,oneof=disabled chat_completions"`
```

已有数据库行由迁移默认 `disabled`；API mutation 必须显式提交闭合值，不能根据 `openai` 名称或 Base URL 自动开启。

本迁移不为 `file_input_mode` 增加索引：该列只随 Provider 主键/既有关联 JOIN 读取，不作为筛选、排序或连接键，低基数字段的独立索引只增加写放大。活动上传规则继续使用现有 `upload_setting.idx_status`；规则表是低频、小规模配置数据，且每个能力请求最多读取一次，本期不增加组合索引。

- [ ] **Step 4: 更新 Atlas hash 并跑数据库/Provider 短测试**

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations`

Run: `go test ./internal/module/ai/provider ./internal/admincontract -count=1`

Expected: 三条命令退出 0，Provider 测试 PASS，HCL 与迁移均包含检查约束。

- [ ] **Step 5: 提交数据库与 Admin API**

```bash
git add database/migrations/202607300102_ai_chat_native_file_attachments.sql database/schema/admin.hcl database/migrations/atlas.sum internal/module/ai/provider internal/admincontract/openapi_models_test.go
git commit -m "feat(ai): 增加供应商文件输入协议"
```

---

### Task 3: 建立统一 AI 附件策略与有效能力交集

**Files:**
- Create: `internal/module/ai/capability/attachment_policy.go`
- Create: `internal/module/ai/capability/attachment_policy_test.go`
- Modify: `internal/module/ai/capability/chat.go`
- Modify: `internal/module/ai/capability/chat_test.go`
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/types_json_test.go`
- Modify: `internal/module/ai/agent/model.go`
- Modify: `internal/module/ai/agent/dto.go`
- Modify: `internal/module/ai/agent/repository.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/service_test.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/repository.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`

- [ ] **Step 1: 写四层交集与关闭原因失败测试**

`attachment_policy_test.go` 使用表驱动证明官方、transport、provider 和平台任一层关闭都会禁用文件，且原因准确：

```go
func TestResolveNativeFileCapabilityNeverWidensOfficialTruth(t *testing.T) {
	tests := []struct {
		name, mode, wantReason                 string
		official, transport, route, platform  bool
		wantEnabled                            bool
	}{
		{"enabled", aiprovider.FileInputModeChatCompletions, "", true, true, true, true, true},
		{"official", aiprovider.FileInputModeChatCompletions, NativeFileDisabledOfficialModel, false, true, true, true, false},
		{"transport", aiprovider.FileInputModeChatCompletions, NativeFileDisabledTransport, true, false, true, true, false},
		{"provider mode", aiprovider.FileInputModeDisabled, NativeFileDisabledProviderMode, true, true, true, true, false},
		{"provider route", aiprovider.FileInputModeChatCompletions, NativeFileDisabledProviderMode, true, true, false, true, false},
		{"platform", aiprovider.FileInputModeChatCompletions, NativeFileDisabledPlatform, true, true, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveNativeFileCapability(NativeFileCapabilityInput{
				OfficialEnabled: tt.official, TransportEnabled: tt.transport,
				ProviderMode: tt.mode, ProviderRouteEnabled: tt.route,
				PlatformReady: tt.platform, AcceptedExtensions: []string{"pdf", "md"},
			})
			if got.Enabled != tt.wantEnabled || got.DisabledReason != tt.wantReason {
				t.Fatalf("capability=%#v", got)
			}
		})
	}
}

func TestNativeFilePolicyIntersectsSystemUploadRule(t *testing.T) {
	got := AllowedNativeFileExtensions([]string{"pdf", "md", "zip", "psd", "go"})
	if diff := cmp.Diff([]string{"pdf", "md", "go"}, got); diff != "" { t.Fatal(diff) }
}
```

在 `agent/service_test.go` 扩展现有 `TestOptionsExposeOfficialModelAndEffectiveChatCapabilities`：放入两个可见 Agent，为 `AgentWithProvider.FileInputMode` 设置 `chat_completions`，并注入以下计数 resolver。一次 `Options` 请求必须只读取一次当前规则，两个 Agent 都只能看到系统规则与 AI 子集的交集：

```go
type countingUploadRuleResolver struct {
	calls int
	rule  uploadpolicy.Rule
	err   error
}

func (resolver *countingUploadRuleResolver) ResolveActive(context.Context) (uploadpolicy.Rule, error) {
	resolver.calls++
	return resolver.rule, resolver.err
}

rules := &countingUploadRuleResolver{rule: uploadpolicy.Rule{
	MaxFileBytes: 100 << 20,
	FileExtensions: []string{"pdf", "md", "zip", "go"},
}}
service := NewService(
	repo, box, nil,
	WithPricingResolver(modelResolver),
	WithTransportCapabilityResolver(infraai.TransportCapabilityResolverFunc(infraai.DefaultTransportCapabilities)),
	WithUploadRuleResolver(rules),
)
result, appErr := service.Options(context.Background(), OptionQuery{UserID: 9})
if appErr != nil { t.Fatal(appErr) }
if rules.calls != 1 { t.Fatalf("active upload rule calls=%d", rules.calls) }
for _, option := range result.List {
	files := option.Capabilities.Attachments.NativeFile
	if !files.Enabled || files.DisabledReason != "" {
		t.Fatalf("native file capability=%#v", files)
	}
	if diff := cmp.Diff([]string{"pdf", "md", "go"}, files.AcceptedExtensions); diff != "" { t.Fatal(diff) }
}
```

另加表驱动用例，分别不注入 resolver、让 resolver 返回错误、返回没有 AI 文件交集的有效规则：

```go
tests := []struct {
	name  string
	rules uploadpolicy.Resolver
}{
	{name: "resolver missing"},
	{name: "resolver error", rules: uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
		return uploadpolicy.Rule{}, errors.New("upload config unavailable")
	})},
	{name: "empty AI intersection", rules: uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
		return uploadpolicy.Rule{MaxFileBytes: 100 << 20, FileExtensions: []string{"zip", "tar"}}, nil
	})},
}
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		service := NewService(
			repo, box, nil,
			WithPricingResolver(modelResolver),
			WithTransportCapabilityResolver(infraai.TransportCapabilityResolverFunc(infraai.DefaultTransportCapabilities)),
			WithUploadRuleResolver(tt.rules),
		)
		result, appErr := service.Options(context.Background(), OptionQuery{UserID: 9})
		if appErr != nil { t.Fatal(appErr) }
		for _, option := range result.List {
			attachments := option.Capabilities.Attachments
			if !attachments.Image.Enabled { t.Fatal("image capability must remain available") }
			if attachments.NativeFile.Enabled || attachments.NativeFile.DisabledReason != capability.NativeFileDisabledPlatform ||
				len(attachments.NativeFile.AcceptedExtensions) != 0 {
				t.Fatalf("native file capability=%#v", attachments.NativeFile)
			}
		}
	})
}
```

把这段表驱动代码放在现有 `TestOptionsExposeOfficialModelAndEffectiveChatCapabilities` 的末尾，使其直接复用该函数内的 `repo`；同时把原官方 resolver 变量重命名为 `modelResolver`，并把固定 secretbox 提取为 `box`。它固定“配置读取故障只关闭收费文件入口，不伪装成模型不支持，也不把整页变成 500”。

在 `chat_test.go` 将当前平台期望从 `text,image` 改为：只有官方 `NativeFileInput=true` 且 transport 包含 `file` 时，基础结构交集才包含 `file`；Provider mode、route 和上传规则的继续收窄由本 Task 的 `attachment_policy_test.go` 与 Agent service 测试固定。

- [ ] **Step 2: 运行 capability/agent 测试并确认先失败**

Run: `go test ./internal/module/ai/capability ./internal/module/ai/agent ./internal/platform/admin -run 'NativeFile|EffectiveChatCapabilities|ProviderModels|UploadRule' -count=1`

Expected: FAIL，当前 Admin Chat 固定移除 `file` 且 Agent 固定返回 `Enabled:false`。

- [ ] **Step 3: 实现不可配置的 AI 附件策略常量**

`attachment_policy.go` 集中定义，其他模块只能调用，不得再写数值或扩展名副本：

```go
const (
	MaxAttachmentsPerMessage    = 5
	MaxMessageAttachmentBytes   = int64(50 << 20)
	MaxNativeFileBytesExclusive = int64(50 << 20)
	MaxRequestNativeFileBytes   = int64(50 << 20)

	NativeFileDisabledOfficialModel = "official_model_unsupported"
	NativeFileDisabledProviderMode  = "provider_file_input_disabled"
	NativeFileDisabledTransport     = "transport_unsupported"
	NativeFileDisabledPlatform      = "platform_unsupported"
)

var NativeFileExtensions = []string{
	"pdf", "doc", "docx", "dot", "odt", "rtf", "ppt", "pptx", "pot", "ppa", "pps", "pwz", "wiz",
	"xla", "xlb", "xlc", "xlm", "xls", "xlsx", "xlt", "xlw", "csv", "tsv", "iif",
	"txt", "text", "md", "markdown", "json", "html", "htm", "xml", "css",
	"asm", "bat", "c", "cc", "cpp", "cxx", "h", "hh", "def", "in", "js", "mjs", "jsx", "ts", "tsx",
	"py", "go", "java", "cs", "php", "rb", "rs", "sh", "bash", "zsh", "ksh", "ps1", "sql", "pl", "lua",
	"r", "scala", "swift", "kt", "kts", "yaml", "yml", "toml", "ini", "conf", "properties", "proto",
	"eml", "log", "rst", "srt", "vtt", "ics", "ifb", "vcf", "diff", "patch",
}

var ImageMIMETypes = []string{"image/jpeg", "image/png", "image/webp", "image/gif"}

type NativeFileCapabilityInput struct {
	OfficialEnabled       bool
	TransportEnabled      bool
	ProviderMode          string
	ProviderRouteEnabled  bool
	PlatformReady         bool
	AcceptedExtensions    []string
}

type NativeFileCapability struct {
	Enabled            bool
	DisabledReason     string
	AcceptedExtensions []string
}
```

`AllowedNativeFileExtensions` 保持 `enum.UploadFileExts` 的顺序并求交集，显式排除 `zip`/`tar`。`ResolveNativeFileCapability` 严格按 official、transport、provider route/mode、platform 的顺序给出关闭原因；只有全部通过才启用，并始终复制输入扩展名，防止调用方修改共享 slice。图片只允许四个协议 MIME，GIF 必须由现有图像检查证明非动画。

- [ ] **Step 4: 扩展 transport 与 Admin Chat，但仍逐层收窄**

`infra/ai/types.go` 的默认 OpenAI-compatible 能力改为：

```go
InputModalities: []string{"text", "image", "file"},
SafeInputUpperBoundStrategies: []string{
	SafeInputUpperBoundStrategyUTF8RequestBytesV1,
	SafeInputUpperBoundStrategyNativeFileContextWindowV1,
},
```

保留旧 `SafeInputUpperBoundStrategy` 字段作为 inline 兼容值；新增策略数组只用于声明 transport 能处理的 proof，不改变现有图片生成 transport。

`adminChatPlatformCapabilities()` 包含 `file` 和 `NativeFileInput:true`。Agent service 在现有 `EffectiveChatCapabilities` 得到 official/transport/platform 的结构化交集后，再用唯一的 `ResolveNativeFileCapability` 应用 provider mode、provider route 与本次请求的上传规则投影；关闭时同时移除 `InputModalities` 中的 `file` 并把 `NativeFileInput` 置为 false。官方详情继续直接读取 catalog，不走该有效能力函数。Task 4 的消息受理复用同一个决策函数，不能再写一套条件判断。

- [ ] **Step 5: 把 Provider 模式带入 Agent/Message/Chat 运行投影**

在三个运行 DTO 中增加同名字段：

```go
FileInputMode string
```

具体位置为 `aimessage.AgentRuntime`、`aichat.AgentEngineConfig` 与 Agent repository 的 Provider 投影。`internal/module/ai/agent/model.go` 的 `Provider` 和 `AgentWithProvider`、对应 SELECT 都投影 `ai_providers.file_input_mode`，不能根据 engine type 或模型 ID 推导。附件根节点同时返回统一数量/总量，避免前端写死 `5` 与 `50 MiB`：

```go
type AttachmentCapabilities struct {
	MaxAttachmentsPerMessage  int                            `json:"max_attachments_per_message"`
	MaxMessageAttachmentBytes int64                          `json:"max_message_attachment_bytes"`
	Image                     ImageAttachmentCapability      `json:"image"`
	NativeFile                NativeFileAttachmentCapability `json:"native_file"`
}

type NativeFileAttachmentCapability struct {
	Enabled               bool     `json:"enabled"`
	DisabledReason        string   `json:"disabled_reason"`
	MaxFilesPerMessage    int      `json:"max_files_per_message"`
	MaxFileBytesExclusive int64    `json:"max_file_bytes_exclusive"`
	MaxRequestFileBytes   int64    `json:"max_request_file_bytes"`
	AcceptedExtensions    []string `json:"accepted_extensions"`
}
```

Agent service 增加：

```go
type requestAttachmentPolicy struct {
	PlatformReady     bool
	AcceptedExtensions []string
}

func WithUploadRuleResolver(resolver uploadpolicy.Resolver) Option {
	return func(service *Service) { service.uploadRules = resolver }
}

func (s *Service) resolveRequestAttachmentPolicy(ctx context.Context) requestAttachmentPolicy {
	if s == nil || s.uploadRules == nil { return requestAttachmentPolicy{} }
	rule, err := s.uploadRules.ResolveActive(ctx)
	if err != nil { return requestAttachmentPolicy{} }
	accepted := capability.AllowedNativeFileExtensions(rule.FileExtensions)
	return requestAttachmentPolicy{
		PlatformReady: len(accepted) > 0,
		AcceptedExtensions: accepted,
	}
}
```

`PageInit`、`List`、`Detail` 和 `Options` 各自在进入 repository/DTO 循环前只调用一次 `resolveRequestAttachmentPolicy(ctx)`，再把不可变结果传给 `agentDTO`/`effectiveCapabilities`；禁止在每个 Provider、模型或 Agent 行内调用 resolver，避免 N+1。resolver 缺失、读取失败或交集为空时，文件能力 fail closed 为 `platform_unsupported`，但文本/图片能力和整个响应继续可用。

`effectiveCapabilityDTO` 只使用 catalog、transport resolver、Provider mode、route 状态和该请求快照组装 DTO；删除固定 `Enabled:false`。`AcceptedExtensions` 必须严格等于当前启用系统上传规则与 `NativeFileExtensions` 的有序交集。

在 `internal/platform/admin/build.go` 把 `uploadTokenRepository` 提前到 Agent service 之前，只创建一个 resolver，并注入 Agent：

```go
uploadTokenRepository := uploadtoken.NewGormRepository(resources.DB)
uploadRuleResolver := uploadtoken.NewActiveRuleResolver(uploadTokenRepository)

aiAgentService := aiagent.NewService(
	aiagent.NewGormRepository(resources.DB),
	providers.Secretbox,
	providers.AIConnectionTester,
	aiagent.WithPricingResolver(aiOfficialModelResolver),
	aiagent.WithTransportCapabilityResolver(providers.AITransportCapabilities),
	aiagent.WithUploadRuleResolver(uploadRuleResolver),
)
```

后面的 upload token service、COS config provider 和 Task 4 的 message service 全部复用这两个变量。`build_test.go` 断言 `uploadtoken.NewActiveRuleResolver(uploadTokenRepository)` 恰好出现一次，且 Agent service 包含 `aiagent.WithUploadRuleResolver(uploadRuleResolver)`。

- [ ] **Step 6: 运行有效能力回归并提交**

Run: `gofmt -w internal/module/ai/capability internal/infra/ai/types.go internal/infra/ai/types_json_test.go internal/module/ai/agent internal/module/ai/message/dto.go internal/module/ai/message/repository.go internal/module/ai/chat/dto.go internal/module/ai/chat/repository.go internal/platform/admin/build.go internal/platform/admin/build_test.go`

Run: `go test ./internal/module/ai/capability ./internal/module/ai/agent ./internal/module/ai/message ./internal/module/ai/chat ./internal/infra/ai ./internal/platform/admin -count=1`

Expected: PASS；GPT-5.5/5.6 官方详情仍为 `text,image,file`，渠道关闭时有效能力原因是 `provider_file_input_disabled`，而不是“模型不支持文件”。

```bash
git add internal/module/ai/capability internal/infra/ai/types.go internal/infra/ai/types_json_test.go internal/module/ai/agent internal/module/ai/message/dto.go internal/module/ai/message/repository.go internal/module/ai/chat/dto.go internal/module/ai/chat/repository.go internal/platform/admin/build.go internal/platform/admin/build_test.go
git commit -m "feat(ai): 建立原生文件有效能力"
```

---

### Task 4: 统一可信附件受理、历史存储、编辑重发与重新生成

**Files:**
- Modify: `internal/infra/storage/cos/object_inspector.go`
- Modify: `internal/infra/storage/cos/object_inspector_test.go`
- Modify: `internal/module/ai/message/dto.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/module/ai/message/service_test.go`
- Modify: `internal/module/ai/message/history_actions.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/history_repository.go`
- Modify: `internal/module/ai/message/history_actions_test.go`
- Modify: `internal/module/ai/message/transport/admin/request.go`
- Modify: `internal/module/ai/message/transport/admin/handler.go`
- Modify: `internal/module/ai/message/transport/admin/handler_test.go`
- Modify: `internal/module/ai/chat/repository.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`

- [ ] **Step 1: 写可信对象、混合附件和编辑语义失败测试**

在 `object_inspector_test.go` 增加以下边界：

```go
func TestTrustedAIChatObjectKeySeparatesLegacyImagesFromNewFiles(t *testing.T) {
	tests := []struct { key, typ string; wantOK bool }{
		{"ai_chat_images/2026/07/old.jpg", "image", true},
		{"ai_chat_images/2026/07/old.pdf", "file", false},
		{"ai_chat_attachments/2026/07/new.jpg", "image", true},
		{"ai_chat_attachments/2026/07/report.pdf", "file", true},
		{"ai_chat_attachments/../secret.pdf", "file", false},
		{"exports/report.pdf", "file", false},
	}
	for _, tt := range tests {
		_, err := TrustedAIChatObjectKey(tt.key, tt.typ)
		if (err == nil) != tt.wantOK { t.Errorf("key=%q type=%q err=%v", tt.key, tt.typ, err) }
	}
}
```

在 `message/service_test.go` 使用 fake inspector 返回与浏览器不同的 size/URL，证明服务只保存 HEAD 事实：

```go
func validFileMessageAgent() *AgentRuntime {
	agent := validMessageAgent()
	agent.ModelID = "gpt-5.6"
	agent.OfficialModelID = "gpt-5.6"
	agent.FileInputMode = aiprovider.FileInputModeChatCompletions
	return agent
}

func TestSendNormalizesMixedAttachmentsFromTrustedHEAD(t *testing.T) {
	repo := &fakeRepository{
		conversation: &Conversation{ID: 9, UserID: 7, AgentID: 5},
		agent: validFileMessageAgent(),
	}
	inspector := &fakeMessageObjectInspector{metadata: map[string]storagecos.ObjectMetadata{
		"ai_chat_attachments/report.pdf": {
			Key: "ai_chat_attachments/report.pdf", MIMEType: "application/pdf",
			Size: 4096, ETag: `"v1"`, TrustedURL: "https://trusted.example/ai_chat_attachments/report.pdf",
		},
	}}
	service := NewService(
		repo,
		WithPricingResolver(testMessagePricingResolver()),
		WithObjectInspector(inspector),
		WithUploadRuleResolver(uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
			return uploadpolicy.Rule{MaxFileBytes: 100 << 20, FileExtensions: []string{"pdf"}}, nil
		})),
		WithTransportCapabilityResolver(staticTransportCapabilityResolver{
			ok: true,
			metadata: infraai.CapabilityMetadata{InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"}},
		}),
	)
	_, appErr := service.Send(context.Background(), 7, SendInput{
		ConversationID: 9, RequestID: "req-file", Content: "总结文件",
		Attachments: []Attachment{{
			Type: "file", ObjectKey: "ai_chat_attachments/report.pdf",
			MIMEType: "text/plain", URL: "https://evil.example/report.pdf", Name: "report.pdf", Size: 1,
		}},
	})
	if appErr != nil { t.Fatal(appErr) }
	want := Attachment{Type: "file", ObjectKey: "ai_chat_attachments/report.pdf", MIMEType: "application/pdf", URL: "https://trusted.example/ai_chat_attachments/report.pdf", Name: "report.pdf", Size: 4096, ETag: `"v1"`}
	var meta struct { Attachments []Attachment `json:"attachments"` }
	if repo.replyInput.MetaJSON == nil || json.Unmarshal([]byte(*repo.replyInput.MetaJSON), &meta) != nil { t.Fatal("missing attachment meta") }
	if diff := cmp.Diff([]Attachment{want}, meta.Attachments); diff != "" { t.Fatal(diff) }
}
```

`history_actions_test.go` 固定三种编辑语义：附件字段缺省时保留源附件；显式空数组时全部删除；显式数组时重新 HEAD 并替换。重新生成始终克隆源用户消息的规范附件。

- [ ] **Step 2: 运行消息/COS 定向测试并确认先失败**

Run: `go test ./internal/infra/storage/cos ./internal/module/ai/message ./internal/module/ai/chat -run 'TrustedAIChatObject|MixedAttachments|Edit.*Attachments|Regenerate.*Attachments' -count=1`

Expected: FAIL，消息服务尚未注入 active rule resolver，inspector 只信任 `ai_chat_images/`、元数据没有 ETag、消息只接受图片且编辑请求没有附件字段。

- [ ] **Step 3: 扩展可信 HEAD 元数据和附件 DTO**

`ObjectMetadata` 增加不可为空的 ETag：

```go
type ObjectMetadata struct {
	Key        string
	MIMEType   string
	Size       int64
	ETag       string
	TrustedURL string
}
```

HEAD 从响应 `ETag` header 读取并保留规范引号；缺失 ETag、size 非正数或 MIME 无法解析均返回 `ErrInvalidObjectMetadata`。受信 key 函数接受附件类型并执行：

```go
func TrustedAIChatObjectKey(key, attachmentType string) (string, error) {
	clean, err := normalizeTrustedKey(key)
	if err != nil { return "", err }
	if strings.HasPrefix(clean, "ai_chat_attachments/") { return clean, nil }
	if attachmentType == "image" && strings.HasPrefix(clean, "ai_chat_images/") { return clean, nil }
	return "", ErrUntrustedObjectKey
}
```

`Attachment` 增加 `ETag string`，但浏览器请求不提交 ETag；transport request 使用独立 DTO，handler 只把以下字段传给 service，ETag 必须由 HEAD 产生：

```go
type attachmentRequest struct {
	Type      string `json:"type" binding:"required,oneof=image file"`
	ObjectKey string `json:"object_key" binding:"required,max=1024"`
	MIMEType  string `json:"mime_type" binding:"required,max=255"`
	URL       string `json:"url" binding:"required,max=2048"`
	Name      string `json:"name" binding:"required,max=255"`
	Size      int64  `json:"size" binding:"required,gt=0"`
}
```

- [ ] **Step 4: 用一个后端管线校验图片和文件**

将现有 `inspectImageAttachments`/`normalizeAttachments` 收敛为一个函数，顺序固定为：

```go
func (s *Service) inspectAttachments(
	ctx context.Context,
	runtime AgentRuntime,
	attachments []Attachment,
) ([]Attachment, *apperror.Error) {
	if len(attachments) == 0 { return []Attachment{}, nil }
	if len(attachments) > capability.MaxAttachmentsPerMessage { return nil, errTooManyAttachments() }
	if s.uploadRules == nil { return nil, errUploadRuleUnavailable() }
	uploadRule, err := s.uploadRules.ResolveActive(ctx)
	if err != nil { return nil, errUploadRuleUnavailable() }
	locals := make([]localAttachment, len(attachments))
	for index, raw := range attachments {
		item, appErr := s.normalizeLocalAttachment(runtime, uploadRule, raw)
		if appErr != nil { return nil, appErr }
		locals[index] = item
	}
	result := make([]Attachment, len(locals))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(capability.MaxAttachmentsPerMessage)
	for index := range locals {
		index := index
		group.Go(func() error {
			metadata, err := s.objectInspector.Head(groupCtx, locals[index].ObjectKey)
			if err != nil { return err }
			item, appErr := normalizeTrustedAttachment(locals[index], metadata)
			if appErr != nil { return appErr }
			result[index] = item
			return nil
		})
	}
	if err := group.Wait(); err != nil { return nil, mapObjectInspectionError(err) }
	var total int64
	for _, item := range result {
		if item.Size > uploadRule.MaxFileBytes { return nil, errSystemUploadFileTooLarge() }
		if item.Type == "file" && item.Size >= capability.MaxNativeFileBytesExclusive { return nil, errNativeFileTooLarge() }
		if item.Size > capability.MaxMessageAttachmentBytes-total { return nil, errAttachmentTotalTooLarge() }
		total += item.Size
	}
	return result, nil
}
```

实现必须同时满足：

- `type` 只允许 `image|file`，名字只取 basename，拒绝空名和控制字符。
- 图片扩展名、HEAD MIME 和非动画 GIF 使用严格图片策略；图片数量与 HEAD size 继续服从当前模型有效 `ImageInput.MaxFiles/MaxBytes`，不能因统一管线放宽旧限制。
- 文件名扩展名与受信 object key 扩展名必须一致，并同时存在于当前启用上传规则与 `NativeFileExtensions`；`application/octet-stream` 对已允许文件可接受，明确冲突的 MIME 拒绝。
- `URL`、`MIMEType`、`Size`、`ETag` 由 HEAD 覆盖，不信任浏览器值。
- 图片与文件合计最多 5 个，单条消息合计不超过 50 MiB，原生单文件严格 `< 50 MiB`。
- `meta_json` 只序列化规范引用，不保存内容或 Base64。
- 空附件直接返回空 slice，不读取上传规则、不访问 COS，保证纯文本请求的延迟和可用性不依赖附件配置。

`aimessage.NewService` 增加 `WithUploadRuleResolver(uploadpolicy.Resolver)`；未装配 resolver 时，只要请求含附件就 fail closed。`internal/platform/admin/build.go` 复用 Task 3 已创建的 `uploadRuleResolver` 并注入 `aimessage.WithUploadRuleResolver(uploadRuleResolver)`；不得再次调用 `uploadtoken.NewActiveRuleResolver`。`build_test.go` 的架构断言必须证明 resolver 构造仍恰好一次，Agent、消息服务和上传 token 复用同一个 `uploadTokenRepository`，没有第二套扩展名事实源。

- [ ] **Step 5: 为编辑附件定义缺省与空数组的不同语义**

transport request 使用指针区分字段是否出现：

```go
type revisionRequest struct {
	Content     string               `json:"content" binding:"required,max=20000"`
	RequestID   string               `json:"request_id" binding:"required,max=128"`
	Attachments *[]attachmentRequest `json:"attachments" binding:"omitempty,max=5,dive"`
}
```

领域输入同样使用指针：

```go
type EditInput struct {
	UserID         int64
	ConversationID int64
	MessageID      int64
	Content        string
	RequestID      string
	Attachments    *[]Attachment
	ValidatedAttachments    []Attachment
	SourceAttachmentsSHA256 [32]byte
}
```

`HistoryRepository` 增加只读准备方法，返回源用户消息附件、当前 Agent runtime 和源附件摘要，不创建消息、command、Run 或 hold：

```go
type HistoryActionPreparation struct {
	Runtime                 AgentRuntime
	SourceAttachments       []Attachment
	SourceAttachmentsSHA256 [32]byte
}

type HistoryPrepareInput struct {
	Operation         string
	UserID            int64
	ConversationID    int64
	SourceMessageID   int64
}

type HistoryRepository interface {
	PrepareAction(context.Context, HistoryPrepareInput) (HistoryActionPreparation, error)
	Revise(context.Context, EditInput) (HistoryAccepted, error)
	Regenerate(context.Context, RegenerateInput) (HistoryAccepted, error)
	DeleteMessages(context.Context, DeleteInput) ([]int64, error)
}
```

`history_actions.go` 的 `Revise` 先调用 `PrepareAction`；附件字段缺省时选择 `preparation.SourceAttachments`，显式空数组时选择空 slice，显式数组时选择请求数组。随后使用 `preparation.Runtime` 调用同一个 `inspectAttachments`，把结果写入 `ValidatedAttachments`，最后才调用 repository mutation。`Regenerate` 固定选择源用户消息附件并走同一管线，不接收浏览器附件。对象不存在、ETag 改变、当前模型/渠道/规则不再允许时在创建新付费事实前返回稳定错误。

`history_repository.go` 的 mutation 事务锁定源消息后，重新计算源附件 SHA-256 并与 `SourceAttachmentsSHA256` 常量时间比较；不一致说明准备与提交之间发生变化，返回 `ErrHistorySourceChanged`，不得使用过期 HEAD 结果。匹配后用 `ValidatedAttachments` 重建规范 `MetaJSON`，同时保留源消息中已校验的 runtime params；不能原样复制旧附件 JSON。`RegenerateInput` 增加与 `EditInput` 相同的两个内部字段。编辑生成的新请求指纹和输入快照必须包含有序附件的 `type/object_key/etag/size/mime_type/name`。

`history_actions_test.go` 对缺省保留、显式清空、显式替换和重新生成四条路径都断言 inspector 调用次数及 repository 收到的 `ValidatedAttachments`；再用源摘要漂移测试证明事务不创建 command/Run/hold。消息列表读取不因历史对象已删除而失败，仍返回原文件卡片元数据；但编辑重发和重新生成在创建新的付费 command 前必须重新 HEAD。对象缺失或 ETag 改变时返回稳定错误，不接受旧 URL，也不创建 Run/hold。

- [ ] **Step 6: 验证历史兼容和付费受理边界**

Run: `gofmt -w internal/infra/storage/cos/object_inspector.go internal/infra/storage/cos/object_inspector_test.go internal/module/ai/message internal/module/ai/chat/repository.go internal/module/ai/chat/service_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go`

Run: `go test ./internal/infra/storage/cos ./internal/module/ai/message ./internal/module/ai/chat ./internal/platform/admin -count=1`

Expected: PASS；历史 `ai_chat_images/` 图片仍能展示和重发，历史目录下伪造的文件被拒绝，编辑附件的保留/清空/替换语义明确；失效历史对象可以展示但不能产生新付费请求。

- [ ] **Step 7: 提交可信附件事实**

```bash
git add internal/infra/storage/cos/object_inspector.go internal/infra/storage/cos/object_inspector_test.go internal/module/ai/message internal/module/ai/chat/repository.go internal/module/ai/chat/service_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go
git commit -m "feat(ai): 统一可信附件消息链路"
```

---

### Task 5: 建立文件 prepared manifest 与独立财务上界

**Files:**
- Create: `internal/infra/ai/file_input.go`
- Create: `internal/module/ai/aigateway/native_file_quote_test.go`
- Modify: `internal/module/ai/aigateway/contracts.go`
- Modify: `internal/module/ai/aigateway/chat_provider.go`
- Modify: `internal/module/ai/aigateway/chat_provider_test.go`
- Modify: `internal/module/ai/aigateway/quote_validator.go`
- Modify: `internal/module/ai/aigateway/quote_validator_test.go`
- Modify: `internal/module/ai/aigateway/gateway.go`
- Modify: `internal/module/ai/aigateway/gateway_test.go`
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/types_json_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/runtime/ai_billing_gateway_test.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/service_test.go`

- [ ] **Step 1: 写 manifest 稳定性、历史总量和财务上界失败测试**

`native_file_quote_test.go` 固定文件请求的两个独立上界。测试直接覆盖本任务新增的纯函数 `validateNativeFileTokenBounds(snapshot PricingSnapshot, quote QuoteEvidence, inputBound int64, outputBound int64) error`，`PersistedQuoteValidator.ValidateQuote` 在归一化 usage item 后调用同一函数：

```go
func TestNativeFileQuoteUsesOfficialContextAndMaxOutputBounds(t *testing.T) {
	snapshot := validPricingSnapshot()
	snapshot.ContextWindowTokens = 400000
	snapshot.CatalogMaxOutputTokens = 32768
	snapshot.EffectiveMaxOutputTokens = 32768
	quote := QuoteEvidence{
		InputUpperBoundStrategy: infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1,
		EffectiveMaxOutputTokens: 32768,
		UpperBoundItems: []billing.UsageItem{
			{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: 400000},
			{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 32768},
		},
	}
	if err := validateNativeFileTokenBounds(snapshot, quote, 400000, 32768); err != nil {
		t.Fatal(err)
	}
}

func TestNativeFileQuoteRejectsNonOfficialBounds(t *testing.T) {
	snapshot := validPricingSnapshot()
	snapshot.ContextWindowTokens = 400000
	snapshot.CatalogMaxOutputTokens = 32768
	snapshot.EffectiveMaxOutputTokens = 32768
	for _, input := range []int64{399999, 400001} {
		quote := QuoteEvidence{
			InputUpperBoundStrategy: infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1,
			EffectiveMaxOutputTokens: 32768,
		}
		if err := validateNativeFileTokenBounds(snapshot, quote, input, 32768); err == nil {
			t.Fatalf("input bound %d must fail", input)
		}
	}
}

func TestGatewayPreflightFailureNeverMarksOrDispatchesAttempt(t *testing.T) {
	store := &testAttemptStore{attempt: validAttempt(72, 1, 5), state: "prepared"}
	provider := &testProvider{preflightErr: errors.New("etag changed")}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider
	_, err := New(deps).Dispatch(context.Background(), store.attempt)
	if err == nil { t.Fatal("preflight failure was accepted") }
	if store.markCalls != 0 || provider.calls != 0 {
		t.Fatalf("mark calls=%d provider calls=%d", store.markCalls, provider.calls)
	}
}
```

为此在现有 `testProvider` 增加 `preflightErr error`、`preflightCalls int`，其 `PreflightPrepared` 只记录调用并返回该错误；所有其他 provider fake 显式实现 no-op preflight，避免通过可选 type assertion 绕过安全边界。

`chat/service_test.go` 组装当前消息与入选历史消息共两个文件，断言总量 `50 MiB + 1` 在 Provider 派发前失败；恰好 `50 MiB` 可以准备。测试还要断言纯文本/图片仍选择 `utf8_request_bytes_plus_framing_v1`。

- [ ] **Step 2: 运行 Gateway/runtime 定向测试并确认先失败**

Run: `go test ./internal/module/ai/aigateway ./internal/runtime ./internal/module/ai/chat -run 'NativeFile|PreparedUpperBound|RequestFileBytes' -count=1`

Expected: FAIL，当前 capability 只允许一个 strategy，quote validator 强制 `output <= context - input`，且历史文件未进入大小计算。

- [ ] **Step 3: 定义紧凑、版本化、可严格校验的 manifest**

`internal/infra/ai/file_input.go` 定义 provider-neutral 契约：

```go
const (
	PreparedChatSchemaInlineV1       = "openai_chat_inline_v1"
	PreparedChatSchemaFileManifestV1 = "openai_chat_file_manifest_v1"
	SafeInputUpperBoundStrategyNativeFileContextWindowV1 = "native_file_context_window_v1"
)

type PreparedFileRef struct {
	Ref       string `json:"ref"`
	ObjectKey string `json:"object_key"`
	ETag      string `json:"etag"`
	Size      int64  `json:"size"`
	MIMEType  string `json:"mime_type"`
	Filename  string `json:"filename"`
}

type PreparedChatFileManifest struct {
	Schema        string            `json:"schema"`
	FileInputMode string            `json:"file_input_mode"`
	Request       json.RawMessage   `json:"request"`
	Files         []PreparedFileRef `json:"files"`
}

type PreparedFileOpenInput struct {
	ObjectKey string
	ETag      string
	Size      int64
}

type PreparedFileObjectMetadata struct {
	ETag     string
	Size     int64
	MIMEType string
}

type PreparedFileOpener interface {
	Head(context.Context, PreparedFileOpenInput) (PreparedFileObjectMetadata, error)
	Open(context.Context, PreparedFileOpenInput) (io.ReadCloser, PreparedFileObjectMetadata, error)
}

type FileInputMetrics struct {
	COSHeadMS                int64 `json:"cos_head_ms"`
	COSStreamMS              int64 `json:"cos_stream_ms"`
	MaterializedRequestBytes int64 `json:"materialized_request_bytes"`
}
```

为清单实现 `Validate()`：schema 必须精确匹配；`file_input_mode` 必须精确为 `chat_completions`；ref 从 `file-1` 连续递增且唯一；request 中每个内部 `file_ref` 恰好引用一个 file；每个 file 恰好被引用一次；key/ETag/size/MIME/filename 必填；文件顺序与消息 content part 顺序一致；未知字段通过 `json.Decoder.DisallowUnknownFields()` 拒绝。使用字段固定的 struct `json.Marshal` 产生规范 JSON，`prepared_request_sha256` 对该 JSON 计算。该模式值是受理时的渠道协议快照，恢复不得重新读取当前 Provider 行覆盖它。

同文件增加 `DetectPreparedChatSchema(body []byte) (string, error)`：顶层没有 `schema` 时只要是合法 JSON 就返回 `openai_chat_inline_v1`；顶层 schema 精确为 `openai_chat_file_manifest_v1` 时严格解析并验证 manifest；任何其他非空 schema 都拒绝。prepare、quote、proof、dispatch 和 recovery 必须调用这一个 detector，不能各自猜 schema。

- [ ] **Step 4: 让 prepared call 与 proof 明确记录 schema 和 strategy**

保持数据库字段不变，`PreparedCall.RequestBody []byte` 与 attempt 的 `PreparedRequest []byte` 继续承载 inline JSON 或紧凑 manifest。领域证据增加：

```go
type QuoteEvidence struct {
	PricingVersion           string              `json:"pricing_version"`
	RequestFingerprint       [32]byte            `json:"request_fingerprint"`
	PreparedRequestSHA256    [32]byte            `json:"prepared_request_sha256"`
	PreparedRequestSchema    string              `json:"prepared_request_schema"`
	InputUpperBoundStrategy  string              `json:"input_upper_bound_strategy"`
	EffectiveMaxOutputTokens int                 `json:"effective_max_output_tokens"`
	UpperBoundItems          []billing.UsageItem `json:"upper_bound_items"`
	CurrentCallMaxUnits      int64               `json:"current_call_max_units"`
	PriorBillableUnits       int64               `json:"prior_billable_units"`
	TargetHoldUnits          int64               `json:"target_hold_units"`
}

var ErrUnsupportedPreparedRequestSchema = errors.New("unsupported prepared request schema")
```

`CapabilityMetadata` 新增 `SafeInputUpperBoundStrategies []string`，同时保留单值字段作为旧 transport inline 默认策略。`validatePreparedUpperBoundProof()` 通过包含关系校验 proof strategy：

```go
func supportsUpperBoundStrategy(capabilities infraai.CapabilityMetadata, strategy string) bool {
	if strings.TrimSpace(capabilities.SafeInputUpperBoundStrategy) == strategy { return true }
	return slices.Contains(capabilities.SafeInputUpperBoundStrategies, strategy)
}
```

`aigateway.Provider` 增加 `PreflightPrepared(context.Context, ProviderAttempt) error`，Gateway 的顺序固定为：校验 hash/schema -> 校验 proof -> preflight -> `MarkDispatched` -> Provider HTTP dispatch。inline preflight 是 no-op；file manifest preflight 严格解析清单并对每个文件调用 `PreparedFileOpener.Head` 验证 key/ETag/size/MIME。preflight 失败时 `testAttemptStore.markCalls` 与 fake provider dispatch calls 必须都为 0。

Task 5 尚未装配 COS opener，因此真实 OpenAI-compatible adapter 对 file manifest 必须返回“文件 preflight 未配置”，不能临时 no-op；inline 请求正常通过。Task 6 装配 `PreparedChatPreflighter` 后才把 file manifest preflight 打开，保证任何中间提交都不会在未校验对象版本时派发收费请求。

- [ ] **Step 5: 为文件请求实现财务冻结专用分支**

quote 构造从官方 catalog 快照取得 context window 与 effective max output：

```go
schema, err := infraai.DetectPreparedChatSchema(call.RequestBody)
if err != nil { return PreparedCall{}, err }
switch schema {
case infraai.PreparedChatSchemaInlineV1:
	inputBound, err = infraai.SafeInputUpperBoundFromRequest(call.RequestBody)
	strategy = infraai.SafeInputUpperBoundStrategyUTF8RequestBytesV1
case infraai.PreparedChatSchemaFileManifestV1:
	inputBound = snapshot.ContextWindowTokens
	strategy = infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1
default:
	return PreparedCall{}, ErrUnsupportedPreparedRequestSchema
}
if err != nil { return PreparedCall{}, err }
call.Quote.PreparedRequestSchema = schema
call.Quote.InputUpperBoundStrategy = strategy
```

`validateNativeFileTokenBounds` 对 `native_file_context_window_v1` 必须精确验证：input quantity 等于 `snapshot.ContextWindowTokens`，output quantity 等于 `snapshot.EffectiveMaxOutputTokens`，且 effective max output 不超过 `snapshot.CatalogMaxOutputTokens`；`PersistedQuoteValidator` 的这一分支不执行 `output <= context_window - input`。inline 分支保持原有 remaining-context 校验。最终 settlement 代码不分文件类型，只接受完整上游 usage，沿用 `unbilled_over_hold` fail-closed。

- [ ] **Step 6: 把最终入选历史附件写入请求清单并校验 50 MiB**

Chat 组装阶段必须从最终 `MessageHistory.MetaJSON` 解析附件，不能只看当前消息。使用防溢出累加：

```go
func requireNativeFileContextWithinLimit(messages []MessageHistory) *apperror.Error {
	var total int64
	for _, message := range messages {
		for _, attachment := range nativeFileAttachments(message.MetaJSON) {
			if attachment.Size > capability.MaxRequestNativeFileBytes-total {
				return errNativeFileContextTooLarge()
			}
			total += attachment.Size
		}
	}
	return nil
}
```

错误固定为“当前对话文件上下文超过 50 MB，请新建对话或减少历史范围”，发生在 reserve/Provider 派发前；不得静默删除旧附件。request fingerprint、manifest 和 prepared hash 必须包含相同有序附件事实。

- [ ] **Step 7: 运行计费回归并提交**

Run: `gofmt -w internal/infra/ai/file_input.go internal/infra/ai/types.go internal/infra/ai/types_json_test.go internal/module/ai/aigateway internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/module/ai/chat`

Run: `go test ./internal/module/ai/aigateway ./internal/runtime ./internal/module/ai/chat ./internal/infra/ai -count=1`

Expected: PASS；文件冻结 input 为官方 context window、output 为官方 max output，成功后仍按完整 usage 结算并释放差额，inline 路径结果不变。

```bash
git add internal/infra/ai/file_input.go internal/infra/ai/types.go internal/infra/ai/types_json_test.go internal/module/ai/aigateway internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/module/ai/chat
git commit -m "feat(ai): 建立文件请求清单与冻结上界"
```

---

### Task 6: 实现 COS 条件流与 Chat Completions 文件 body 物化

**Files:**
- Create: `internal/infra/storage/cos/object_stream.go`
- Create: `internal/infra/storage/cos/object_stream_test.go`
- Create: `internal/infra/ai/openaicompat/file_manifest.go`
- Create: `internal/infra/ai/openaicompat/file_manifest_test.go`
- Modify: `internal/infra/ai/types.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Modify: `internal/infra/ai/openaicompat/client_test.go`
- Modify: `internal/infra/storage/cos/object_reader.go`
- Modify: `internal/infra/storage/cos/object_reader_test.go`

- [ ] **Step 1: 写条件 GET、精确长度和流式取消失败测试**

`object_stream_test.go` 使用 `httptest.Server` 断言 HEAD/GET 都绑定同一 ETag，GET 必须携带 `If-Match`：

```go
func TestConditionalStreamRequiresMatchingETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.Header.Get("If-Match") != `"etag-v1"` {
			http.Error(w, "precondition", http.StatusPreconditionFailed)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "4")
		_, _ = io.WriteString(w, "data")
	}))
	defer server.Close()
	streamer := newTestObjectStreamer(t, server.URL)
	body, metadata, err := streamer.Open(context.Background(), infraai.PreparedFileOpenInput{
		ObjectKey: "ai_chat_attachments/report.pdf", ETag: `"etag-v1"`, Size: 4,
	})
	if err != nil { t.Fatal(err) }
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "data" || metadata.ETag != `"etag-v1"` { t.Fatalf("body=%q metadata=%#v err=%v", got, metadata, err) }
}

func newTestObjectStreamer(t *testing.T, endpoint string) *COSObjectStreamer {
	t.Helper()
	provider := &staticObjectConfigProvider{config: ObjectConfig{
		SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1",
		Region: "ap-test", Endpoint: endpoint,
	}}
	return NewObjectStreamer(provider, ObjectStreamerConfig{
		Enabled: true, Timeout: time.Second, HTTPClient: http.DefaultClient,
	})
}
```

`file_manifest_test.go` 使用两个小文件，断言：preflight 逐个 HEAD 且不发 POST；内部 `file_ref` 不出现在出站 body；`file_data` 是正确 data URL/Base64；顺序保持；`Content-Length` 等于实际字节数；取消 context 后 producer 与 reader 都结束。另加模式快照测试：首次文件 `PrepareChat` 在 mode disabled/opener nil 时失败且纯文本仍成功；已经持久化为 `file_input_mode:"chat_completions"` 的 manifest 即使 client 当前 mode 变为 disabled，仍按 manifest 通过 preflight/dispatch。

- [ ] **Step 2: 运行 COS/OpenAI-compatible 定向测试并确认先失败**

Run: `go test ./internal/infra/storage/cos ./internal/infra/ai/openaicompat -run 'ConditionalStream|Preflight|FileManifest|FileInputModeSnapshot|Materialized|ContentLength|Cancellation' -count=1`

Expected: FAIL，当前 reader 使用 `io.ReadAll` 且 prepared body 必须是可直接发送 JSON。

- [ ] **Step 3: 新增 context 控制的条件对象流接口**

`object_stream.go` 实现 `infraai.PreparedFileOpener`，OpenAI adapter 因此只依赖 provider-neutral 接口，不依赖 COS SDK：

```go
type ObjectStreamerConfig struct {
	Enabled    bool
	Timeout    time.Duration
	HTTPClient *http.Client
}

type COSObjectStreamer struct {
	enabled bool
	timeout time.Duration
	client  *http.Client
	config  ObjectConfigProvider
}

var _ infraai.PreparedFileOpener = (*COSObjectStreamer)(nil)

func (streamer *COSObjectStreamer) Head(
	ctx context.Context,
	input infraai.PreparedFileOpenInput,
) (infraai.PreparedFileObjectMetadata, error)

func (streamer *COSObjectStreamer) Open(
	ctx context.Context,
	input infraai.PreparedFileOpenInput,
) (io.ReadCloser, infraai.PreparedFileObjectMetadata, error)
```

`Head` 再次验证受信 key、取得活动 COS 配置并验证 size/ETag/MIME。`Open` 在真正派发时再次执行同一 HEAD，然后 GET 使用自定义 header 携带 `If-Match`：

```go
headers := make(http.Header)
headers.Set("If-Match", input.ETag)
response, err := client.Object.Get(ctx, input.ObjectKey, &tencentcos.ObjectGetOptions{
	XOptionHeader: &headers,
})
```

返回受 context 控制的 `io.ReadCloser`。`Close()`、context 取消和 transport 错误都必须关闭响应 body。`412 Precondition Failed` 映射为稳定的对象版本变化错误。现有 `ObjectReader.Get()` 保留给小对象旧用途，原生文件链路禁止调用它。

- [ ] **Step 4: 规范化内部 `file_ref` 并计算物化长度**

OpenAI-compatible `PrepareChat()` 对没有原生文件的请求继续返回 inline JSON；有文件时将每个 content part 写为唯一内部 token，例如 `{"type":"file_ref","ref":"file-1"}`，并返回 `openai_chat_file_manifest_v1`。物化后该 token 必须精确变为 Chat Completions content part：

```json
{
  "type": "file",
  "file": {
    "filename": "report.pdf",
    "file_data": "data:application/pdf;base64,JVBERi0..."
  }
}
```

内部 `file_ref`、object key 与 ETag 都不能出现在出站 JSON。

长度计算只使用整数并检查溢出：

```go
func base64EncodedLength(size int64) (int64, error) {
	if size < 0 || size > (math.MaxInt64-2)/4*3 { return 0, ErrMaterializedBodyTooLarge }
	return 4 * ((size + 2) / 3), nil
}

func dataURLLength(file infraai.PreparedFileRef) (int64, error) {
	encoded, err := base64EncodedLength(file.Size)
	if err != nil { return 0, err }
	return int64(len("data:"+file.MIMEType+";base64,")) + encoded, nil
}
```

实现先将规范 request 编码成 literal/file segments，再累计所有 literal 和 Base64 长度得到精确 `Content-Length`；文件名、MIME 和 JSON 字符串必须通过 `encoding/json` 编码，禁止手工转义。

- [ ] **Step 5: 使用 `io.Pipe` 顺序写出 Chat Completions body**

`openaicompat.Config` 与 `Client` 增加 `FileInputMode string` 和 `FileOpener infraai.PreparedFileOpener`。首次 `PrepareChat` 发现原生文件时要求 mode 为 `chat_completions` 且 opener 非 nil，并把 mode 写入 manifest；这发生在 quote/reserve 之前。纯文本/图片允许 mode disabled 或 opener nil；恢复已持久化 file manifest 时只校验 manifest 内模式，不用当前 Config mode 覆盖受理快照，但仍要求 opener 可用。

物化器签名：

```go
type MaterializedRequest struct {
	Body          io.ReadCloser
	ContentLength int64
	Result        <-chan MaterializationResult
}

type MaterializationResult struct {
	Metrics infraai.FileInputMetrics
	Err     error
}

func MaterializeFileManifest(
	ctx context.Context,
	manifest infraai.PreparedChatFileManifest,
	objects infraai.PreparedFileOpener,
) (MaterializedRequest, error)
```

producer goroutine按顺序执行：写 literal；`Open` 当前文件；创建 `base64.NewEncoder(base64.StdEncoding, pipeWriter)`；`io.CopyN` 精确复制 manifest size；关闭 encoder 和 object reader；写下一个 literal。任何 HEAD/GET/读取/写入/取消错误都调用 `pipeWriter.CloseWithError(err)`，并向容量 1 的 `Result` channel 写入一次结果后关闭 channel。`StreamPreparedChat` 在 HTTP transport 已消费 request body 后读取该结果，将安全整数指标复制到 `ChatResult`；不得在 producer 与 HTTP goroutine间无锁共享可变 struct。同一时刻只打开一个文件，不能创建完整文件 `[]byte` 或 Base64 `string`。

- [ ] **Step 6: 在 prepared dispatch 中选择 inline 或 manifest body**

`infraai` 增加：

```go
type PreparedChatPreflighter interface {
	PreflightPreparedChat(context.Context, []byte) error
}
```

`openaicompat.Client.PreflightPreparedChat` 使用统一 schema detector；inline 直接成功，file manifest 对有序 files 调用 `FileOpener.Head`，任一事实不一致立即失败且不创建 HTTP request。`aigateway` 的 chat provider adapter 将它实现为 Task 5 的 `Provider.PreflightPrepared`。

`StreamPreparedChat()` 调用 `infraai.DetectPreparedChatSchema(input.Body)`：inline 继续 `bytes.NewReader(input.Body)`；file manifest 严格解码后调用物化器，并设置：

```go
request.ContentLength = materialized.ContentLength
request.Header.Set("Content-Type", "application/json")
request.Header.Set("Idempotency-Key", input.IdempotencyKey)
```

`infraai.ChatResult` 增加 `FileInputMetrics *FileInputMetrics`；文件 body 被 transport 完整消费后从 `MaterializedRequest.Result` 读取一次并填入，inline 请求保持 `nil`。保持现有 SSE、工具调用、usage、provider request ID、idle timeout 和停止后排空逻辑。COS/编码在明确未 dispatch 前失败可由现有恢复规则重试；HTTP body 已可能到达上游后失败必须进入现有 `outcome_unknown`，不能创建一个盲目新 attempt。

- [ ] **Step 7: 运行定向回归、内存形态断言并提交**

Run: `gofmt -w internal/infra/storage/cos/object_stream.go internal/infra/storage/cos/object_stream_test.go internal/infra/ai/types.go internal/infra/ai/openaicompat/file_manifest.go internal/infra/ai/openaicompat/file_manifest_test.go internal/infra/ai/openaicompat/client.go internal/infra/ai/openaicompat/client_test.go`

Run: `go test ./internal/infra/storage/cos ./internal/infra/ai/openaicompat -count=1`

Expected: PASS；preflight ETag 漂移时测试服务未收到 POST；正常请求收到精确 Content-Length 和合法 Chat Completions `file` content part，源码测试确认新文件链路不调用 `io.ReadAll`。

```bash
git add internal/infra/storage/cos internal/infra/ai/types.go internal/infra/ai/openaicompat
git commit -m "feat(ai): 流式物化原生文件请求"
```

---

### Task 7: 装配 Worker 恢复链路、稳定错误和安全运行摘要

**Files:**
- Modify: `internal/runtime/providers.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/runtime/ai_billing_gateway_test.go`
- Modify: `internal/module/ai/aigateway/gateway.go`
- Modify: `internal/module/ai/aigateway/gateway_test.go`
- Modify: `internal/module/ai/aigateway/contracts.go`
- Modify: `internal/module/ai/run/dto.go`
- Modify: `internal/module/ai/run/service.go`
- Modify: `internal/module/ai/run/service_test.go`
- Modify: `internal/shared/enum/ai.go`
- Modify: `internal/shared/apperror/error.go`
- Modify: `internal/module/ai/message/service.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Modify: `docs/architecture.md`

- [ ] **Step 1: 写恢复不可重新组装、对象漂移不扣费和摘要脱敏失败测试**

先在 `aigateway/gateway_test.go` 使用现有 `validAttempt`、`testAttemptStore` 和 `testGatewayDependencies` 证明 Gateway 把持久化 manifest 原样交给 Provider：

```go
type capturingProvider struct {
	testProvider
	attempt ProviderAttempt
}

func (provider *capturingProvider) Dispatch(ctx context.Context, attempt ProviderAttempt) (DispatchResult, error) {
	provider.attempt = cloneAttempt(attempt)
	return provider.testProvider.Dispatch(ctx, attempt)
}

func TestGatewayDispatchPassesPersistedFileManifestUnchanged(t *testing.T) {
	manifest := []byte(`{"schema":"openai_chat_file_manifest_v1","file_input_mode":"chat_completions","request":{"model":"gpt-5.6"},"files":[{"ref":"file-1","object_key":"ai_chat_attachments/report.pdf","etag":"\\"v1\\"","size":4,"mime_type":"application/pdf","filename":"report.pdf"}]}`)
	attempt := validAttempt(71, 1, 5)
	attempt.PreparedRequest = append([]byte(nil), manifest...)
	attempt.RequestSHA256 = sha256.Sum256(manifest)
	attempt.Quote.PreparedRequestSHA256 = attempt.RequestSHA256
	attempt.Quote.PreparedRequestSchema = infraai.PreparedChatSchemaFileManifestV1
	attempt.Quote.InputUpperBoundStrategy = infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1
	store := &testAttemptStore{attempt: cloneAttempt(attempt), state: "prepared"}
	provider := &capturingProvider{testProvider: testProvider{capabilities: infraai.CapabilityMetadata{
		SupportsIdempotencyHeader: true,
		SupportedUsageIdentities: []infraai.UsageIdentity{{Category: infraai.UsageCategoryInput, Unit: "token"}},
		SafeInputUpperBoundStrategy: infraai.SafeInputUpperBoundStrategyNativeFileContextWindowV1,
	}}}
	deps := testGatewayDependencies(&testReserve{}, store)
	deps.Provider = provider
	_, err := New(deps).Dispatch(context.Background(), attempt)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(provider.attempt.PreparedRequest, manifest) { t.Fatal("manifest bytes changed") }
	if provider.attempt.IdempotencyKey != attempt.IdempotencyKey { t.Fatal("idempotency key changed") }
}
```

`ai_billing_gateway_test.go` 再增加 `TestRecoveredNativeFileAttemptUsesPersistedManifestOnly`：持久化含 `file_input_mode:"chat_completions"` 的 manifest 后修改当前会话、Agent 和 Provider 行（包括把当前 Provider mode 改为 `disabled`），恢复同一 attempt，fake transport 仍必须收到上述 manifest bytes 和原 idempotency key。其余用例按下表使用该文件现有 executor/store fake 完整实现：

| 测试名 | Arrange | 必须断言 |
| --- | --- | --- |
| `TestETagMismatchBeforeDispatchReleasesHoldWithoutCharge` | opener 在 Provider mark-dispatched 前返回 `ErrObjectVersionChanged` | attempt=`failed`、hold=0、usage unavailable、钱包流水无扣费 |
| `TestBodyFailureAfterPossibleDispatchBecomesOutcomeUnknown` | HTTP transport 已读取一个 body segment 后返回错误 | dispatch=`unknown`、terminal=`outcome_unknown`、不创建第二 attempt |
| `TestSafeRequestSummaryNeverContainsObjectIdentityOrManifest` | Run detail 读取一个 file manifest attempt | JSON 含计数/字节/模式，不含 object key、manifest、`file_ref`、`file_data`、Base64 |

最后一个测试将 `DetailResponse` JSON marshal 后断言不存在 `object_key`、`ai_chat_attachments/`、`file_ref`、`file_data`、`base64`，但计数、字节量和模式存在。

- [ ] **Step 2: 运行 runtime/run 定向测试并确认先失败**

Run: `go test ./internal/runtime ./internal/module/ai/aigateway ./internal/module/ai/run -run 'RecoveredNativeFile|ETagMismatch|PossibleDispatch|SafeRequestSummary' -count=1`

Expected: FAIL，Worker 尚未装配条件流，Run 摘要只有 prepared request 字节数。

- [ ] **Step 3: 在平台装配层复用一个活动 COS 配置源并传入 Chat Worker**

`runtime.BuildProviders` 运行时没有数据库，因此不能在 `runtime/providers.go` 查询活动上传配置。实际装配放在已有 `internal/platform/admin/build.go`：

```go
aiObjectConfig := uploadtoken.NewObjectConfigProvider(uploadTokenRepository, providers.Secretbox)
aiChatObjectInspector := storagecos.NewObjectInspector(
	aiObjectConfig,
	storagecos.ObjectInspectorConfig{Enabled: true},
)
aiChatObjectStreamer := storagecos.NewObjectStreamer(
	aiObjectConfig,
	storagecos.ObjectStreamerConfig{Enabled: true},
)
```

`aichat.Dependencies` 增加 `FileOpener infraai.PreparedFileOpener`；`EngineConfig` 增加 `FileOpener` 与 `FileInputMode`。`build.go` 将 `aiChatObjectStreamer` 传给 `aichat.NewRuntimeService`；service 创建每个渠道 engine 时把 opener 和 Task 3 已投影的 Provider mode 原样放入 `EngineConfig`。`runtime.aiChatEngineFactory.NewEngine` 不因 mode/opener 组合阻断整个 engine，而是把二者传给：

```go
openaicompat.New(openaicompat.Config{
	BaseURL: input.BaseURL,
	APIKey: input.APIKey,
	FileInputMode: input.FileInputMode,
	FileOpener: input.FileOpener,
	Timeout: 30 * time.Second,
	StreamIdleTimeout: factory.streamIdleTimeout,
})
```

这样 API 消息受理用 inspector，后台付费 Worker 用 streamer，两者共享同一个 `uploadtoken.ObjectConfigProvider`；adapter 不重新查询数据库，也不根据附件 URL 创建匿名 reader。`build_test.go` 和 `worker_test.go` 必须断言 file opener 从平台装配贯通到 engine；新文件请求 mode 为 `chat_completions` 但 opener 缺失时由 `PrepareChat` 在 reserve/Provider 派发前 fail closed，mode 为 `disabled` 或 opener 缺失的纯文本/图片请求不被附件依赖阻断。恢复已准备文件请求使用 manifest 内的 mode 快照，不能因当前 Provider 行变化而重组或拒绝。

- [ ] **Step 4: 固定恢复、停止与失败分类**

恢复规则必须按 attempt durable state 执行：

```go
switch attempt.DispatchState {
case infraai.DispatchStateNotDispatched:
	return dispatchPersistedPreparedRequest(ctx, attempt)
case infraai.DispatchStateDispatched, infraai.DispatchStateUnknown:
	return finalizeWithoutBlindRedispatch(ctx, attempt)
default:
	return ErrInvalidDispatchState
}
```

稳定错误至少映射为以下 code/category，HTTP 文案由 Admin API 统一返回：

```text
ai.attachment.model_unsupported
ai.attachment.provider_file_input_disabled
ai.attachment.transport_unsupported
ai.attachment.type_unsupported
ai.attachment.too_many
ai.attachment.file_too_large
ai.attachment.message_total_too_large
ai.attachment.context_total_too_large
ai.attachment.object_unavailable
ai.attachment.object_version_changed
ai.provider.file_part_rejected
```

对象错误发生在明确未 dispatch 时标记 attempt failed、释放 hold、无 usage 不扣费；上游明确拒绝 file part 归类 provider rejected；可能已发送的 body 失败沿用 outcome unknown/fail-closed。用户停止发生在 COS 物化或 Provider 派发前时释放 hold；已派发时继续沿用即时停止展示、后台排空和完整 usage 结算。

- [ ] **Step 5: 从 manifest 构造安全摘要，不解析或返回文件内容**

`SafeRequestSummary` 扩展为：

```go
type SafeRequestSummary struct {
	ProviderAttemptCount     int    `json:"provider_attempt_count"`
	ToolCallCount            int    `json:"tool_call_count"`
	PreparedRequestBytes     int    `json:"prepared_request_bytes"`
	MessageCount             *int   `json:"message_count"`
	AttachmentCount          int    `json:"attachment_count"`
	NativeFileCount          int    `json:"native_file_count"`
	NativeFileBytes          int64  `json:"native_file_bytes"`
	PreparedManifestBytes    int    `json:"prepared_manifest_bytes"`
	MaterializedRequestBytes int64  `json:"materialized_request_bytes"`
	FileInputMode            string `json:"file_input_mode"`
}
```

`buildSafeRequestSummary` 严格解析已持久化 schema：inline 沿用现有 message count；manifest 只累计 request content part、file count、manifest 长度、根据公式得到的 materialized 长度，并从 manifest 投影 `FileInputMode`。不得读取当前 Provider 行填这个字段。解析失败返回零值摘要并由既有结构异常字段暴露，不把原始 prepared JSON 回传。

Task 5 已在 `infraai.FileInputMetrics` 定义只含三个整数的 transport 诊断。`MaterializeFileManifest` 累计所有文件 HEAD 与实际读取耗时并随 `ChatResult`/`DispatchResult` 返回。`internal/shared/enum/ai.go` 增加内部事件 `AIRunEventFileMaterialized = "file_materialized_v1"`；`gormGatewayAttemptStore.RecordTerminalOutcome` 在记录 attempt 终态的同一事务中使用现有 run event seq 追加一条事件，`message` 只保存该三整数 JSON；不得含 filename、key、MIME、ETag 或 manifest。这样无需增加表字段或索引，也能在恢复后的 Run 详情读取 durable 指标。

`LatencyBreakdown` 增加可空的 `COSHeadMS`、`COSStreamMS`；只从 `file_materialized_v1` durable event 聚合，未执行文件链路时为 `nil`，不能用前端时间估算。该内部事件必须从公开 `DetailResponse.Events` 过滤，不能让用户看到 JSON message。event JSON 缺字段、负数或超出当前 end-to-end 时长时按结构异常忽略并记录服务端错误，不能把未经验证的值返回前端。

- [ ] **Step 6: 更新架构文档并运行恢复/观测回归**

`docs/architecture.md` 增加以下不变量：

```text
Native file request: immutable manifest -> conditional COS stream -> provider body
Recovery source: persisted manifest only
Financial proof: native_file_context_window_v1
Settlement source: complete upstream usage only
Forbidden persistence: file bytes, Base64, materialized request, temporary credentials
```

Run: `gofmt -w internal/runtime/providers.go internal/runtime/worker.go internal/runtime/worker_test.go internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/module/ai/chat/dto.go internal/module/ai/chat/service.go internal/module/ai/chat/service_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go internal/module/ai/aigateway internal/module/ai/run internal/shared/enum/ai.go internal/shared/apperror/error.go internal/module/ai/message/service.go internal/infra/ai/openaicompat/client.go`

Run: `go test ./internal/runtime ./internal/module/ai/aigateway ./internal/module/ai/run ./internal/module/ai/message ./internal/infra/ai/openaicompat -count=1`

Expected: PASS；恢复测试证明不读取可变会话重组请求，Run JSON 只有安全统计，没有对象身份或 manifest。

- [ ] **Step 7: 提交 Worker 与观测链路**

```bash
git add internal/runtime internal/module/ai/chat/dto.go internal/module/ai/chat/service.go internal/module/ai/chat/service_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go internal/module/ai/aigateway internal/module/ai/run internal/shared/enum/ai.go internal/shared/apperror/error.go internal/module/ai/message/service.go internal/infra/ai/openaicompat/client.go docs/architecture.md
git commit -m "feat(ai): 装配文件请求恢复与安全观测"
```

---

### Task 8: 实现前端统一附件队列、回形针入口与编辑交互

**Files:**
- Modify: `src/api/system/uploadConfig.ts`
- Modify: `src/api/system/uploadConfig.types.ts`
- Modify: `src/api/ai/providers.ts`
- Modify: `src/views/Main/ai/providers/composables/useProviderForm.ts`
- Modify: `src/views/Main/ai/providers/components/ProviderFormDialog.vue`
- Move: `src/views/Main/ai/chat/components/MessageInput/use-image-attachments.ts` -> `src/views/Main/ai/chat/components/MessageInput/use-attachments.ts`
- Modify: `src/api/ai/agents.ts`
- Modify: `src/api/ai/messages.ts`
- Modify: `src/views/Main/ai/chat/components/MessageInput/use-attachments.ts`
- Modify: `src/views/Main/ai/chat/components/MessageInput/index.vue`
- Modify: `src/views/Main/ai/chat/components/MessageInput/MessageInputToolbar.vue`
- Modify: `src/views/Main/ai/chat/components/MessageInput/PendingAttachments.vue`
- Modify: `src/views/Main/ai/chat/components/MessageInput/pending-attachments.css`
- Modify: `src/views/Main/ai/chat/components/MessageList/MessageEditor.vue`
- Modify: `src/views/Main/ai/chat/components/MessageList/index.vue`
- Modify: `src/views/Main/ai/chat/use-chat-page.ts`
- Modify: `src/views/Main/ai/chat/components/MessageInput/capability-transition.ts`
- Modify: `src/views/Main/ai/runs/components/RunList/RunLatencyBreakdown.vue`
- Modify: `src/i18n/locales/zh-CN/ai.ts`
- Modify: `src/i18n/locales/en-US/ai.ts`
- Modify: `src/i18n/locales/generated.ts`
- Modify: `tests/shared/system/upload-config-contract.test.ts`
- Create: `tests/shared/ai/ai-provider-file-input-mode.test.ts`
- Create: `tests/component/ai/ChatAttachments.test.ts`
- Modify: `tests/shared/ai/ai-chat-capabilities.test.ts`
- Modify: `tests/component/ai/MessageInteractions.test.ts`
- Modify: `tests/component/ai/RunLatencyBreakdown.test.ts`

- [ ] **Step 1: 同步当前后端契约，让 TypeScript 先看到闭合能力与附件类型**

在 `E:\admin\admin_back_go`：

```powershell
$backendCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
git add contracts/admin/v1/openapi.json contracts/admin/v1/manifest.json
git commit -m "chore(contract): 发布原生文件附件契约"
```

在 `E:\admin\admin_front_ts`：

```powershell
$manifest = Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json
$backendCommit = $manifest.backend_commit
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
```

Expected: 命令退出 0；后端 contract 提交的 manifest 绑定 Tasks 1-7 的实现提交；生成类型中的附件 `type` 为 `'image' | 'file'`，Provider 包含 `file_input_mode`，原生文件能力包含服务端限制与关闭原因，上传响应扩展名不再退化为任意 string。

- [ ] **Step 2: 先写上传规则和 Provider 表单的失败测试**

在 `tests/shared/system/upload-config-contract.test.ts` 增加真实响应，复现“全选 + `psd`”崩溃：

```ts
it('accepts every extension already validated by the generated response contract', async () => {
  const harness = installApiClientHarness({
    list: [{
      id: 1,
      title: '全部类型',
      max_size_mb: 100,
      image_exts: ['psd', 'jfif', 'avif'],
      file_exts: ['pdf', 'pptx', 'md', 'ts', 'go'],
      created_at: '2026-07-31 10:00:00',
      updated_at: '2026-07-31 10:00:00',
    }],
    page: emptyPage,
  })
  cleanups.push(harness.uninstall)

  await expect(UploadRuleApi.list({ current_page: 1, page_size: 20 }))
    .resolves.toMatchObject({ list: [{ image_exts: ['psd', 'jfif', 'avif'] }] })
})
```

新建 `tests/shared/ai/ai-provider-file-input-mode.test.ts`，直接测试表单事实到 API payload 的纯映射：

```ts
import { describe, expect, it } from 'vitest'
import {
  buildProviderMutationParams,
  createDefaultProviderForm,
} from '@/views/Main/ai/providers/composables/useProviderForm'

describe('provider file input mode', () => {
  it('defaults to disabled and preserves an explicit Chat Completions selection', () => {
    const form = createDefaultProviderForm()
    expect(form.file_input_mode).toBe('disabled')
    form.file_input_mode = 'chat_completions'
    form.name = 'OpenAI'
    form.api_key = 'sk-test'
    form.model_ids = ['gpt-5.6']
    expect(buildProviderMutationParams(form)).toMatchObject({
      file_input_mode: 'chat_completions',
    })
  })
})
```

Run: `npm test -- tests/shared/system/upload-config-contract.test.ts tests/shared/ai/ai-provider-file-input-mode.test.ts`

Expected: FAIL；旧上传 adapter 抛出 `upload rule item extensions violate the contract`，Provider 表单尚无 `file_input_mode`。

- [ ] **Step 3: 删除前端上传白名单并实现 Provider 显式协议选择**

`src/api/system/uploadConfig.ts` 删除 `allowedImageExts`、`allowedFileExts`、`isUploadImageExt`、`isUploadFileExt`；生成契约已完成响应校验，adapter 只保留结构映射：

```ts
function toUploadRule(item: UploadRuleContractItem): UploadRuleItem {
  return { ...item, image_exts: item.image_exts, file_exts: item.file_exts }
}

const rulePageInit = async (options: ExecuteOptions = {}): Promise<UploadRuleInitResponse> => {
  const response = await executeAdminOperation(
    adminOperations.get_api_admin_v1_upload_rules_page_init,
    {},
    options,
  )
  return { dict: response.dict }
}
```

`src/api/ai/providers.ts` 从生成 mutation body 派生模式类型，并在 mutation/list/page-init DTO 中贯通：

```ts
type AiProviderMutationContractBody = NonNullable<
  AdminOperationInput<'post_api_admin_v1_ai_providers'>['body']
>
export type AiProviderFileInputMode = AiProviderMutationContractBody['file_input_mode']
```

`ProviderFormState` 增加 `file_input_mode`；`createDefaultProviderForm()` 固定为 `disabled`；导出纯函数供组件和测试共用：

```ts
export function buildProviderMutationParams(form: ProviderFormState): AiProviderMutationParams {
  return {
    id: form.id,
    name: form.name,
    engine_type: form.driver,
    base_url: form.base_url,
    model_ids: [...form.model_ids],
    model_display_names: { ...form.model_display_names },
    status: form.status,
    file_input_mode: form.file_input_mode,
    ...(form.api_key ? { api_key: form.api_key } : {}),
  }
}
```

`ProviderFormDialog.vue` 删除本地 `buildPayload()`，调用该纯函数；使用二选一单选/分段控件展示“关闭”和“Chat Completions”。这是渠道协议事实，不展示为模型能力，也不触发收费文件探测。

Run: `npm test -- tests/shared/system/upload-config-contract.test.ts tests/shared/ai/ai-provider-file-input-mode.test.ts`

Expected: PASS；上传规则不再二次拒绝后端合法值，新增 Provider 默认关闭且显式选择会进入请求 body。

- [ ] **Step 4: 先写选择、拖拽、粘贴、失败重试和切 Agent 的失败测试**

`ChatAttachments.test.ts` 至少覆盖：

```ts
it.each(['picker', 'drop', 'paste'] as const)('adds a PDF through %s into the same queue', async (source) => {
  const file = new File(['pdf'], 'report.pdf', { type: 'application/pdf', lastModified: 1 })
  const wrapper = mountChatInput({ nativeFileEnabled: true })
  await addAttachmentThrough(wrapper, source, file)
  expect(wrapper.find('[data-attachment-kind="file"]').text()).toContain('report.pdf')
  expect(uploadToken).toHaveBeenCalledWith(expect.objectContaining({
    folderName: 'ai_chat_attachments', fileKind: 'file',
  }))
})

it('does not consume a plain text paste', async () => {
  const wrapper = mountChatInput({ nativeFileEnabled: true })
  const event = pasteEvent({ text: 'hello', files: [] })
  await wrapper.find('textarea').trigger('paste', event)
  expect(event.preventDefault).not.toHaveBeenCalled()
  expect(uploadToken).not.toHaveBeenCalled()
})

it('retries only the failed attachment', async () => {
  const wrapper = mountChatInputWithOneUploadedAndOneFailed()
  await wrapper.find('[data-testid="attachment-retry"]').trigger('click')
  expect(uploadFileToCloud).toHaveBeenCalledTimes(1)
  expect(uploadFileToCloud).toHaveBeenCalledWith(expect.objectContaining({ name: 'failed.pdf' }), expect.anything())
})

it('keeps incompatible files visible and blocks send after changing agent', async () => {
  const wrapper = mountChatInput({ nativeFileEnabled: true, seededFile: 'report.pdf' })
  await wrapper.setProps({ capabilities: imageOnlyCapabilities })
  expect(wrapper.text()).toContain('当前渠道未开通文件传输')
  expect(wrapper.find('[data-testid="send-message"]').attributes('disabled')).toBeDefined()
  expect(wrapper.text()).toContain('report.pdf')
})
```

Run: `npm test -- tests/shared/ai/ai-chat-capabilities.test.ts tests/component/ai/ChatAttachments.test.ts tests/component/ai/MessageInteractions.test.ts`

Expected: FAIL，当前 composable 只接受图片、paste 只读取 `image/*`、没有 retry，切 Agent 不能表达文件不兼容。

- [ ] **Step 5: 将图片 composable 重构为统一附件状态机**

先执行文件移动：

Run: `git mv src/views/Main/ai/chat/components/MessageInput/use-image-attachments.ts src/views/Main/ai/chat/components/MessageInput/use-attachments.ts`

核心类型固定为：

```ts
export type AttachmentKind = 'image' | 'file'
export type AttachmentStatus = 'queued' | 'uploading' | 'uploaded' | 'failed' | 'retrying'

export interface PendingAttachment {
  id: string
  identity: string
  kind: AttachmentKind
  file: File
  preview?: string
  status: AttachmentStatus
  url?: string
  objectKey?: string
  error?: string
}

const attachmentIdentity = (file: File) =>
  [file.name, file.size, file.lastModified, file.type].join('\u0000')
```

`useAttachments(capabilities)` 的同一个 `addFiles()` 被 picker、drop 和 paste 调用。选择规则按顺序执行：当前 Agent 已选择；能力支持 kind；扩展名/MIME 在服务端返回范围；同一批次和现有队列去重；合计数量不超过 `attachments.max_attachments_per_message`；合计 size 不超过 `attachments.max_message_attachment_bytes`；文件严格 `< native_file.max_file_bytes_exclusive`。任何上传失败进入 `failed`，`retryAttachment(id)` 只重传该项；前端不得另写数值常量。

上传统一使用：

```ts
await getUploadToken({
  folderName: 'ai_chat_attachments',
  fileName: pending.file.name,
  fileSize: pending.file.size,
  fileKind: pending.kind,
})
```

上传成功后请求 DTO 必须包含后端要求的 `type/object_key/url/mime_type/name/size`；不能把 File 内容或 data URL 放入 API payload。

- [ ] **Step 6: 用一个回形针入口完成选择、拖拽和剪贴板行为**

`MessageInputToolbar.vue` 使用项目现有 Element Plus `Paperclip` 图标，将事件改为 `addAttachment`，tooltip/aria-label 为“添加附件”。按钮在未选 Agent、发送中、录音中、达到合计上限时禁用；只支持图片时仍显示同一个入口，但 input accept 只包含图片 MIME。

paste 只在浏览器实际暴露 file item 时阻止默认行为：

```ts
function handlePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.items ?? [])
    .filter(item => item.kind === 'file')
    .map(item => item.getAsFile())
    .filter((file): file is File => file !== null)
  if (files.length === 0) return
  event.preventDefault()
  void addFiles(files)
}
```

图片项显示缩略图；文件项显示文件图标、名称、格式化大小和状态；失败项提供重试图标按钮，所有项提供删除按钮。图标按钮必须有 tooltip 和 aria-label，卡片最大 8px 圆角，名称使用省略与 title，不能因长文件名撑开输入区。

- [ ] **Step 7: 阻止不完整上传发送，并在 Agent 切换时保留事实**

发送条件增加：

```ts
const hasPendingUpload = computed(() => attachments.value.some(item =>
  item.status === 'queued' || item.status === 'uploading' || item.status === 'retrying',
))
const hasFailedUpload = computed(() => attachments.value.some(item => item.status === 'failed'))
const hasIncompatibleAttachment = computed(() => attachments.value.some(item =>
  !isAttachmentSupported(item, capabilities.value),
))
const canSend = computed(() => !hasPendingUpload.value && !hasFailedUpload.value && !hasIncompatibleAttachment.value)
```

`components/MessageInput/capability-transition.ts` 不再删除不兼容附件，而是返回阻断原因；优先显示服务端 `disabled_reason`：`provider_file_input_disabled` 映射“当前渠道未开通文件传输”，`official_model_unsupported` 才映射“当前模型不支持文件”。切回兼容 Agent 后无需重新上传即可发送。

- [ ] **Step 8: 让编辑重发可保留、删除和新增附件**

`AiMessageRevisionParams` 增加可选 `attachments?: AiMessageAttachmentRequest[]`，API 仅在编辑器确实修改附件集合时发送该字段：缺省表示保留，空数组表示全部删除。

`MessageEditor.vue` 使用统一附件展示/选择逻辑初始化历史附件；原附件以 `uploaded` 状态加入，允许删除和新增。提交事件改为：

```ts
submit: [payload: {
  content: string
  attachments?: AiMessageAttachmentRequest[]
}]
```

只改文字时省略 attachments；附件集合变化时发送完整规范数组。`canSubmit` 在内容或附件任一变化且不存在 pending/failed/incompatible 状态时为 true。重新生成仍不从前端重复上传，后端按源用户消息附件清单处理。

- [ ] **Step 9: 展示安全运行摘要并补齐中文/英文文案**

`RunLatencyBreakdown.vue` 只在 `native_file_count > 0` 时展示“原生文件数”“原生文件总大小”“COS 校验”“COS 读取”“物化请求大小”“渠道文件协议”；不显示 object key、URL、manifest 或 Base64。数值使用现有格式化函数，缺失耗时显示 `--`。

补齐 `zh-CN/ai.ts` 与 `en-US/ai.ts` 的附件状态、重试、数量/大小、四类能力关闭原因、历史上下文超限、对象失效和渠道拒绝文案，然后运行 `npm run locale:generate` 更新 `generated.ts`，再运行 `npm run locale:check`，Expected: 两条命令退出 0 且中英文 key 完全一致。

- [ ] **Step 10: 运行前端定向测试、类型检查与构建并提交**

Run: `npm test -- tests/shared/system/upload-config-contract.test.ts tests/shared/ai/ai-provider-file-input-mode.test.ts tests/shared/ai/ai-chat-capabilities.test.ts tests/component/ai/ChatAttachments.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/ai/RunLatencyBreakdown.test.ts`

Run: `npm run typecheck`

Run: `npm run locale:check`

Run: `npm run build`

Expected: 全部退出 0；选择、拖拽、文件粘贴、纯文本粘贴、失败重试、Agent 切换、编辑附件和安全运行摘要均通过。

```bash
git add contracts/backend/admin src/modules/http/generated src/api/system/uploadConfig.ts src/api/system/uploadConfig.types.ts src/api/ai src/views/Main/ai/providers src/views/Main/ai/chat src/views/Main/ai/runs/components/RunList/RunLatencyBreakdown.vue src/i18n tests/shared/system/upload-config-contract.test.ts tests/shared/ai/ai-provider-file-input-mode.test.ts tests/shared/ai/ai-chat-capabilities.test.ts tests/component/ai/ChatAttachments.test.ts tests/component/ai/MessageInteractions.test.ts tests/component/ai/RunLatencyBreakdown.test.ts
git commit -m "feat(ai): 完成统一文件附件交互"
```

---

### Task 9: 发布契约、执行定向门禁并完成真实渠道手工验收

**Files:**
- Verify: `contracts/admin/v1/openapi.json`
- Verify: `contracts/admin/v1/manifest.json`
- Verify: `contracts/backend/admin/v1/**`
- Verify: `contracts/backend/admin/lock.json`
- Verify: `src/modules/http/generated/admin.ts`
- Verify: `src/modules/http/generated/operations.ts`

- [ ] **Step 1: 在后端运行全部短定向测试**

在 `E:\admin\admin_back_go` 逐条运行，任何一条失败都先修复对应 Task，不能跳过：

```powershell
go test ./internal/shared/enum ./internal/shared/uploadpolicy ./internal/module/uploadconfig ./internal/module/uploadtoken ./internal/admincontract -count=1
go test ./internal/module/ai/provider ./internal/module/ai/capability ./internal/module/ai/agent -count=1
go test ./internal/module/ai/message ./internal/module/ai/chat -count=1
go test ./internal/infra/storage/cos ./internal/infra/ai/openaicompat -count=1
go test ./internal/module/ai/aigateway ./internal/module/ai/run ./internal/runtime -count=1
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
```

Expected: 每条命令退出 0；不运行 `go test ./...`、覆盖率脚本或长 smoke。

- [ ] **Step 2: 验证已发布 Admin Contract 可由绑定提交确定性重建**

读取 manifest 中已经发布的实现提交并重新生成，随后要求工作区零漂移：

```powershell
$manifest = Get-Content -Raw contracts/admin/v1/manifest.json | ConvertFrom-Json
$backendCommit = $manifest.backend_commit
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
git diff --exit-code -- contracts/admin/v1
```

Expected: contract check 和 diff 均退出 0；manifest 记录的 backend commit 与 `$backendCommit` 完全一致。

- [ ] **Step 3: 前端重新同步同一后端提交并运行契约门禁**

在 `E:\admin\admin_front_ts`：

```powershell
$manifest = Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json
$backendCommit = $manifest.backend_commit
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
git diff --exit-code -- contracts/backend/admin src/modules/http/generated/admin.ts src/modules/http/generated/operations.ts
```

Expected: 同步、生成、检查和 diff 均退出 0，前端 lock 与后端 manifest 指向同一实现提交。

- [ ] **Step 4: 运行最终前端短门禁**

```powershell
npm test -- tests/shared/system/upload-config-contract.test.ts
npm test -- tests/shared/ai/ai-provider-file-input-mode.test.ts
npm test -- tests/shared/ai/ai-chat-capabilities.test.ts
npm test -- tests/component/ai/ChatAttachments.test.ts
npm test -- tests/component/ai/MessageInteractions.test.ts
npm test -- tests/component/ai/RunLatencyBreakdown.test.ts
npm run locale:check
npm run typecheck
npm run build
```

Expected: 全部退出 0；不执行 Playwright 或真实收费自动化。

- [ ] **Step 5: 执行静态泄露和重复事实源检查**

在后端：

```powershell
rg -n 'file_data|;base64,|io\.ReadAll' internal/module/ai internal/runtime internal/infra/ai/openaicompat
rg -n 'ai_chat_images/' internal/module/ai internal/infra/storage/cos
```

Expected: 第一条只命中流式协议实现/测试，不命中数据库模型、日志或 Run DTO；`io.ReadAll` 不出现在原生文件 materializer。第二条只命中历史图片兼容分支和测试，新增上传统一使用 `ai_chat_attachments/`。

在前端：

```powershell
rg -n 'allowedImageExts|allowedFileExts|isUploadImageExt|isUploadFileExt|useImageAttachments|ai_chat_images' src tests
```

Expected: 无命中；扩展名、附件能力和上传目录均来自生成契约或统一附件实现。

- [ ] **Step 6: 确认两个仓库没有生成物漂移或无关改动**

```powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
```

Expected: 两个命令均无输出；如果用户原本有无关改动，只允许显示那些预先存在且未被本计划触碰的文件，实施文件和生成物必须干净。

- [ ] **Step 7: 由用户执行本地数据库和真实付费手工验收**

用户在业务数据可接受迁移的本地环境运行 `admin-dev`，确认 API 与 Worker 均启动且不存在缺表/缺列。随后按以下顺序手工验收，每一次真实请求都在运行记录核对冻结、usage、扣费和释放差额：

1. 上传配置保持“全选 + 100 MB”，打开规则页和列表，确认 `psd`、`jfif`、`pptx`、`md`、`ts` 不再触发前端契约异常。
2. 将测试 Provider 的文件协议设为 `chat_completions`；确认 GPT-5.5、GPT-5.5 Pro、GPT-5.6 显示官方 `text + image + file`，有效能力允许文件。
3. 分别通过回形针、拖拽、`Ctrl+V` 上传图片、PDF、Word、Excel、Markdown 和代码文件；纯文本粘贴仍进入输入框。
4. 用 GPT-5.5 和 GPT-5.6 分别读取上述文件；PDF 同时含文字与页面图片，Word/PPT 内嵌图表不要求视觉识别。
5. 制造一次上传失败并点击重试；确认只重传失败项，上传中/失败时不能发送。
6. 上传文件后切换到文件协议关闭的 Agent；确认文件保留、发送被阻止且文案为“当前渠道未开通文件传输”，切回后无需重传。
7. 编辑已有用户消息：只改文字、删除一个附件、全部删除、增加一个附件各执行一次；重新生成复用源附件。
8. 构造当前消息和历史文件合计超过 50 MiB；确认 Provider 未收到请求，界面提示新建对话或减少历史范围。
9. 文件请求生成中点击停止；界面立即停止展示，聊天保存可见前缀，后台最终按完整权威 usage 结算。
10. 在运行详情核对文件数、总字节、COS 耗时、TTFT、manifest 大小、物化大小和文件协议；确认页面、接口响应和日志均不出现完整 object key、Base64 或文件内容。
11. 将 Provider 模式改回 `disabled`；确认文本和图片仍可发送，文件被准确阻止，官方模型详情仍保留文件能力。

Expected: 全路径符合设计；真实收费请求只由用户手工触发，没有自动测试产生费用。

---

## 完成标准覆盖矩阵

| 设计完成标准 | 对应任务与证据 |
| --- | --- |
| 1. 上传规则全选不崩溃，前端无手写白名单 | Task 1 后端闭合 enum、前端契约测试；Task 9 静态搜索 |
| 2. 扩展名范围完整且 `doc` 不属于图片 | Task 1 canonical 集合测试 |
| 3. GPT-5.5/5.6 官方详情始终显示文件能力 | Task 3 catalog/effective capability 回归；Task 9 手工步骤 2 |
| 4. 渠道关闭原因不冒充模型不支持 | Task 3 四层交集测试；Task 8 capability transition 测试 |
| 5. 前后端都按有效能力拒绝伪造文件请求 | Task 3 能力 DTO；Task 4 service 校验；Task 8 UI 阻断 |
| 6. 选择、拖拽和浏览器可见剪贴板文件可上传 | Task 8 参数化组件测试 |
| 7. 图片/文件共用上传、重试、删除和编辑状态机 | Task 8 unified queue 与 MessageEditor 测试 |
| 8. 新目录生效且历史图片路径兼容 | Task 1 folder enum；Task 4 trusted key 测试 |
| 9. MySQL 不保存文件 Base64 或物化请求 | Task 5 紧凑 manifest；Task 7 摘要脱敏；Task 9 静态检查 |
| 10. 50 MiB 文件不产生完整原始字节/Base64 分配 | Task 6 `io.Pipe`、顺序 reader 与源码测试 |
| 11. manifest、ETag 和 `If-Match` 防止对象替换 | Task 5 manifest 校验；Task 6 条件流测试；Task 7 恢复测试 |
| 12. 文件请求不使用 UTF-8 字节 token 上界 | Task 5 `native_file_context_window_v1` 测试 |
| 13. 冻结覆盖官方最坏上界且只按完整 usage 扣费 | Task 5 quote/settlement 测试；Task 9 手工核账 |
| 14. 停止后仍即时停止、后台排空并最终结算 | Task 7 停止分类回归；Task 9 手工步骤 9 |
| 15. 文本和历史图片 inline 路径不回归 | Task 5/6 inline 回归；Task 9 Provider disabled 手工步骤 11 |
| 16. 后端、前端、类型、构建和 Contract gate 通过 | Task 9 Steps 1-4 |

## 实施完成条件

- 九个 Task 的 checkbox 全部完成，且每个任务的定向测试先观察到预期 FAIL，再在实现后得到 PASS。
- 两个仓库的提交边界与计划一致，生成物只由正式脚本更新，工作区没有无关改动。
- 数据库迁移、HCL、Atlas hash、后端 OpenAPI、前端 lock 和生成 TypeScript 指向同一实现事实。
- 用户完成 Task 9 的真实渠道验收并确认运行记录中的冻结、完整 usage、最终扣费和差额释放正确。
