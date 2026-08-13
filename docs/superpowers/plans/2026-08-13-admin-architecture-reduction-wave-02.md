# Admin 架构减法 Wave 02：系统设置 CRUD 样板 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变 API、数据库、权限、缓存、双语言和页面操作习惯的前提下，把系统设置收口成第一条社区可读的 CRUD 样板链，并让前端系统设置 API 脱离 generated operation 的日常依赖。

**Architecture:** 后端固定为 `route -> 全局 Auth/Permission/OperationLog middleware -> handler -> service -> repository -> model`。系统设置只有一个 Admin HTTP 表面，因此本波将 `transport/admin` 收回模块根目录；`Repository` 接口保留，因为它既是 Service 测试边界，也是 CAPTCHA 与上传 Token 的真实 typed-setting 读取边界。旧 `RouteRegistry/HTTPContract/OpenAPI` 只保留为 Wave 07 前的兼容桥，不再作为前端系统设置业务类型来源；前端 direct request 使用模块自有 Zod schema，但仍由唯一 `ApiClient` 解 envelope、分类错误并触发全局通知。

**Tech Stack:** Go 1.26、Gin、GORM、MySQL 8、go-redis v9、Vue 3、TypeScript、Vitest、现有 `request`/`ApiClient` 与 Element Plus。

---

## 0. 执行边界

本计划只处理：

1. Wave 01 已确认的两个收口门禁；
2. `systemsetting` 后端模块；
3. 前端 `src/api/system/setting.ts` 及其定向测试；
4. 因 Go 类型包路径变化而必须同步的 Admin contract 生成物。

本计划不处理：

- 不迁移用户、角色、权限、邮件、短信、支付、AI 或其他模块；
- 不删除全局 `adminroute.Registry`、OpenAPI、generated files 或 schema compiler；
- 不修改 `system_settings` 表、字段、索引和 4 条初始化 Seed；
- 不改变任何 `/api/admin/v1/system-settings` 路径、HTTP method、请求字段或响应字段；
- 不启动或重启 `admin-dev`；
- 不运行 `go test ./...`、Playwright、`npm run verify:frontend` 或发布长脚本；
- 不清理用户或其他窗口的未提交修改。

若任一仓库执行前不是干净状态，停止并汇报，不得 reset、checkout 或覆盖。

## 1. 当前事实与目标结构

当前链路：

```text
server
-> systemsetting/transport/admin/route.go
-> systemsetting/transport/admin/handler.go
-> systemsetting/service.go
-> systemsetting/repository.go
-> systemsetting/model.go

dto.go                     同时混放 Service input 和 HTTP response
transport/admin/request.go 再声明一份 HTTP input
前端 setting.ts            再从 generated contract 拼出业务类型
```

目标文件：

```text
internal/module/systemsetting/
├── model.go            # system_settings 表映射
├── request.go          # Handler binding + Service 输入
├── response.go         # data 内响应
├── repository.go       # MySQL + 可回源 Redis cache
├── repository_test.go
├── service.go          # 业务规则
├── service_test.go
├── handler.go          # HTTP 解析与统一响应
├── handler_test.go
└── route.go            # 路径、权限、审计、兼容合同
```

明确删除：

```text
internal/module/systemsetting/dto.go
internal/module/systemsetting/errors.go
internal/module/systemsetting/transport/admin/request.go
internal/module/systemsetting/transport/admin/handler.go
internal/module/systemsetting/transport/admin/handler_test.go
internal/module/systemsetting/transport/admin/route.go
```

保留但不扩大：

```text
internal/shared/setting
```

它继续拥有 `auth.captcha.ttl_minutes`、`upload.token.ttl_minutes` 的 typed 解释；`systemsetting.Repository.SettingByKey` 继续满足它以及 CAPTCHA、上传 Token 的最小 Reader 合同。不要把这些业务规则搬回系统设置 CRUD Service。

## Task 1：补齐 Wave 01 发布环境清理门禁

**Files:**

- Modify: `E:/admin/admin_back_go/scripts/release/verify-admin-only-release.ps1:37-67`
- Modify: `E:/admin/admin_back_go/scripts/tests/admin-only-release-rehearsal.tests.ps1:83-113`

- [ ] **Step 1: 写失败合同**

在 `admin-only-release-rehearsal.tests.ps1` 的 `$backendQualityEnvironmentNames` 中，把 Realtime 项固定为：

```powershell
  'REALTIME_ENABLED',
  'REALTIME_PUBLISHER',
  'REALTIME_REDIS_DB',
```

该列表随后会与 release verifier AST 中的实际环境清理列表做等值比较，因此当前实现应失败。

- [ ] **Step 2: 运行短测试确认失败**

```powershell
pwsh -NoProfile -File scripts/tests/admin-only-release-rehearsal.tests.ps1
```

Expected: FAIL，错误指出 backend-quality 环境名单不一致或没有清理 `REALTIME_REDIS_DB`。

- [ ] **Step 3: 修复唯一环境名单**

在 `verify-admin-only-release.ps1` 的 `$script:BackendQualityEnvironmentNames` 相同位置加入：

```powershell
  'REALTIME_ENABLED',
  'REALTIME_PUBLISHER',
  'REALTIME_REDIS_DB',
```

不要加入默认值，不读取本地 `admin-go.env`；这里的职责只是让质量门在测试前清除调用者环境污染，测试后恢复原环境。

- [ ] **Step 4: 复跑并提交独立修复**

```powershell
pwsh -NoProfile -File scripts/tests/admin-only-release-rehearsal.tests.ps1
git diff --check
git add scripts/release/verify-admin-only-release.ps1 scripts/tests/admin-only-release-rehearsal.tests.ps1
git commit -m "fix(release): clear realtime redis database override"
```

Expected: PowerShell 合同测试 PASS；提交中没有系统设置文件。

## Task 2：明确 Redis Pub/Sub 频道边界

Redis Pub/Sub 不受逻辑 DB 隔离。Wave 01 的独立 DB 1 仍有连接生命周期、健康检查和未来独立地址的价值，但生产逻辑不得把 DB 号当频道隔离。Realtime 主频道已经使用 `admin_go:realtime:publish`；当前唯一未命名空间化的是 AI cancel 的 `ai:reply:cancel:`。

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/ai/replycommand/cancel.go:17`
- Modify: `E:/admin/admin_back_go/internal/module/ai/replycommand/cancel_test.go:80-115`
- Modify: `E:/admin/admin_back_go/docs/architecture.md:1035-1045`

- [ ] **Step 1: 先把测试期望改成项目级频道**

把 cancel publisher/subscriber 测试中的期望频道统一改为：

```go
const wantCancelChannel = "admin_go:realtime:ai:reply:cancel:41"
```

断言保持精确：

```go
if err := publisher.PublishCancel(context.Background(), 41); err == nil || publishedChannel != wantCancelChannel {
	t.Fatalf("PublishCancel() channel=%q err=%v", publishedChannel, err)
}

subscription, err := subscriber.SubscribeCancel(context.Background(), 41)
if err != nil || subscription == nil || subscribedChannel != wantCancelChannel {
	t.Fatalf("SubscribeCancel() channel=%q subscription=%v err=%v", subscribedChannel, subscription, err)
}
```

- [ ] **Step 2: 运行失败测试**

```powershell
go test ./internal/module/ai/replycommand -run 'TestRedisCancel' -count=1
```

Expected: FAIL，实际频道仍为 `ai:reply:cancel:41`。

- [ ] **Step 3: 修改单一频道前缀**

在 `cancel.go` 只改常量：

```go
const cancelChannelPrefix = "admin_go:realtime:ai:reply:cancel:"
```

不要新增 Redis 客户端、配置字段、兼容双发或 DB 0 fallback。项目尚未上线，API 与 Worker 同批重启即可切换；双发只会重新制造两个事实源。

在 `docs/architecture.md` 的系统设置/Redis 边界附近加入准确说明：

```markdown
Redis 逻辑 DB 只隔离普通 key，不隔离 Pub/Sub。Realtime 事件使用
`admin_go:realtime:publish`，AI cancel 使用
`admin_go:realtime:ai:reply:cancel:<command_id>`；不得依赖 DB 1 阻止同名频道互通。
```

- [ ] **Step 4: 复跑并提交**

```powershell
go test ./internal/module/ai/replycommand -run 'TestRedisCancel' -count=1
git diff --check
git add internal/module/ai/replycommand/cancel.go internal/module/ai/replycommand/cancel_test.go docs/architecture.md
git commit -m "fix(realtime): namespace ai cancel channels"
```

Expected: 定向测试 PASS；不修改消息 payload 和取消语义。

## Task 3：拆开系统设置 request/response，消灭重复输入 DTO

**Files:**

- Create: `E:/admin/admin_back_go/internal/module/systemsetting/request.go`
- Create: `E:/admin/admin_back_go/internal/module/systemsetting/response.go`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/repository.go`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/service.go`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/service_test.go`
- Delete: `E:/admin/admin_back_go/internal/module/systemsetting/dto.go`
- Delete: `E:/admin/admin_back_go/internal/module/systemsetting/errors.go`

- [ ] **Step 1: 创建唯一请求结构**

新增 `request.go`，完整内容：

```go
package systemsetting

type ListRequest struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	PageSize    int    `form:"page_size" binding:"required,min=1,max=50"`
	Key         string `form:"key" binding:"max=100"`
	Status      *int   `form:"status" binding:"omitempty,common_status"`
}

type CreateRequest struct {
	Key    string `json:"key" binding:"required,max=100"`
	Value  string `json:"value"`
	Type   int    `json:"type" binding:"required,system_setting_value_type"`
	Remark string `json:"remark" binding:"max=255"`
}

type UpdateRequest struct {
	Value  string `json:"value"`
	Type   int    `json:"type" binding:"required,system_setting_value_type"`
	Remark string `json:"remark" binding:"max=255"`
}

type DeleteRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type StatusRequest struct {
	Status int `json:"status" binding:"required,common_status"`
}
```

这些结构既是 Handler 的 binding 输入，也是 Service 输入。不要再创建 `CreateInput/UpdateInput/ListQuery` 做字段逐个复制；Gin tag 是传输层元数据，不改变 Service 只依赖 `context.Context` 的边界。

- [ ] **Step 2: 创建唯一响应结构**

新增 `response.go`，完整内容：

```go
package systemsetting

import "admin_back_go/internal/shared/dict"

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	SystemSettingValueTypeArr []dict.Option[int] `json:"system_setting_value_type_arr"`
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID            int64  `json:"id"`
	SettingKey    string `json:"setting_key"`
	SettingValue  string `json:"setting_value"`
	ValueType     int    `json:"value_type"`
	ValueTypeName string `json:"value_type_name"`
	Remark        string `json:"remark"`
	Status        int    `json:"status"`
	StatusName    string `json:"status_name"`
	IsDel         int    `json:"is_del"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type CreateResponse struct {
	ID int64 `json:"id"`
}

type EmptyResponse struct{}
```

`ListResponse.List` 不加 `omitempty`，空列表必须序列化为 `[]`，不能变成 `null` 或 `{}`。

- [ ] **Step 3: 先机械改测试类型名**

在 `service_test.go` 做以下一一替换，不改变断言：

```text
ListQuery   -> ListRequest
CreateInput -> CreateRequest
UpdateInput -> UpdateRequest
InitResponse -> PageInitResponse
InitDict     -> PageInitDict
```

同时把 fake repository 的 `gotList` 改为 `ListRequest`。此时运行测试应因 Service 签名仍使用旧类型或重复定义失败。

- [ ] **Step 4: 运行失败测试**

```powershell
go test ./internal/module/systemsetting -run 'Test(Init|List|Create|Update|ChangeStatus|Delete)' -count=1
```

Expected: FAIL，原因是 Service 尚未切换到新 request/response 类型。

- [ ] **Step 5: 收口 Service 类型并删除一行错误包装层**

把 repository sentinel 移到它真正所属的 `repository.go`，放在 imports 后：

```go
var ErrRepositoryNotConfigured = errors.New("system setting repository is not configured")
```

从 `service.go` 删除同名变量和不再需要的 `errors` import。

在 `service.go` 做精确签名替换：

```go
func (s *Service) PageInit(context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		SystemSettingValueTypeArr: shareddict.SystemSettingValueTypeOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, request ListRequest) (*ListResponse, *apperror.Error)
func (s *Service) Create(ctx context.Context, request CreateRequest) (int64, *apperror.Error)
func (s *Service) Update(ctx context.Context, id int64, request UpdateRequest) *apperror.Error
```

`PageInit` 到 `Delete` 的完整实现固定为：

```go
func (s *Service) PageInit(context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		SystemSettingValueTypeArr: shareddict.SystemSettingValueTypeOptions(),
	}}, nil
}

func (s *Service) List(ctx context.Context, request ListRequest) (*ListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	if appErr := validateListRequest(request); appErr != nil {
		return nil, appErr
	}
	request.Key = strings.TrimSpace(request.Key)

	rows, total, err := repo.List(ctx, request)
	if err != nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}

	list := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, listItemFromSetting(row))
	}
	return &ListResponse{
		List: list,
		Page: Page{
			PageSize: request.PageSize, CurrentPage: request.CurrentPage,
			TotalPage: totalPage(total, request.PageSize), Total: total,
		},
	}, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (int64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	request, appErr = normalizeCreateRequest(request)
	if appErr != nil {
		return 0, appErr
	}
	exists, err := repo.ExistsByKey(ctx, request.Key, 0)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.key_check_failed", nil, "校验配置 key 失败", err)
	}
	if exists {
		return 0, apperror.BadRequestKey(
			"systemsetting.key.duplicate",
			map[string]any{"key": request.Key},
			"配置 key ["+request.Key+"] 已存在",
		)
	}

	id, err := repo.Create(ctx, Setting{
		SettingKey: request.Key, SettingValue: request.Value,
		ValueType: request.Type, Remark: request.Remark,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	})
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.create_failed", nil, "新增系统设置失败", err)
	}
	if err := repo.InvalidateCache(ctx, request.Key); err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.cache_clear_failed", nil, "清理系统设置缓存失败", err)
	}
	return id, nil
}

func (s *Service) Update(ctx context.Context, id int64, request UpdateRequest) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("systemsetting.id.invalid", nil, "无效的配置ID")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("systemsetting.not_found", nil, "配置项不存在")
	}

	request, appErr = normalizeUpdateRequest(request)
	if appErr != nil {
		return appErr
	}
	if err := repo.Update(ctx, id, map[string]any{
		"setting_value": request.Value,
		"value_type":    request.Type,
		"remark":        request.Remark,
	}); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.update_failed", nil, "更新系统设置失败", err)
	}
	if err := repo.InvalidateCache(ctx, row.SettingKey); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.cache_clear_failed", nil, "清理系统设置缓存失败", err)
	}
	return nil
}

func (s *Service) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	if id <= 0 {
		return apperror.BadRequestKey("systemsetting.id.invalid", nil, "无效的配置ID")
	}
	if !enum.IsCommonStatus(status) {
		return apperror.BadRequestKey("systemsetting.status.invalid", nil, "无效的状态")
	}
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	row, err := repo.Get(ctx, id)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}
	if row == nil {
		return apperror.NotFoundKey("systemsetting.not_found", nil, "配置项不存在")
	}
	if err := repo.Update(ctx, id, map[string]any{"status": status}); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.status_update_failed", nil, "更新系统设置状态失败", err)
	}
	if err := repo.InvalidateCache(ctx, row.SettingKey); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.cache_clear_failed", nil, "清理系统设置缓存失败", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, ids []int64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return apperror.BadRequestKey("systemsetting.delete.empty", nil, "请选择要删除的配置")
	}
	rows, err := repo.SettingsByIDs(ctx, ids)
	if err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.query_failed", nil, "查询系统设置失败", err)
	}
	if len(rows) != len(ids) {
		return apperror.BadRequestKey("systemsetting.delete.contains_missing", nil, "包含不存在的配置项")
	}
	if err := repo.Delete(ctx, ids); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.delete_failed", nil, "删除系统设置失败", err)
	}
	for _, id := range ids {
		if err := repo.InvalidateCache(ctx, rows[id].SettingKey); err != nil {
			return apperror.WrapKey(apperror.CodeInternal, 500, "systemsetting.cache_clear_failed", nil, "清理系统设置缓存失败", err)
		}
	}
	return nil
}
```

把内部 helper 替换为：

```go
func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.InternalKey(
			"systemsetting.repository_missing",
			nil,
			"系统设置仓储未配置",
		)
	}
	return s.repository, nil
}

func validateListRequest(request ListRequest) *apperror.Error {
	if request.CurrentPage <= 0 {
		return apperror.BadRequestKey("systemsetting.current_page.invalid", nil, "当前页无效")
	}
	if request.PageSize < enum.PageSizeMin || request.PageSize > enum.PageSizeMax {
		return apperror.BadRequestKey("systemsetting.page_size.invalid", nil, "每页数量无效")
	}
	if request.Status != nil && !enum.IsCommonStatus(*request.Status) {
		return apperror.BadRequestKey("systemsetting.status.invalid", nil, "无效的状态")
	}
	return nil
}

func normalizeCreateRequest(request CreateRequest) (CreateRequest, *apperror.Error) {
	request.Key = strings.TrimSpace(request.Key)
	if request.Key == "" || len([]rune(request.Key)) > maxKeyLen {
		return request, apperror.BadRequestKey("systemsetting.key.invalid", nil, "配置 key 不能为空且不能超过100个字符")
	}
	return normalizeCreateFields(request)
}

func normalizeCreateFields(request CreateRequest) (CreateRequest, *apperror.Error) {
	update, appErr := normalizeUpdateRequest(UpdateRequest{
		Value: request.Value, Type: request.Type, Remark: request.Remark,
	})
	if appErr != nil {
		return request, appErr
	}
	request.Value = update.Value
	request.Type = update.Type
	request.Remark = update.Remark
	return request, nil
}

func normalizeUpdateRequest(request UpdateRequest) (UpdateRequest, *apperror.Error) {
	request.Remark = strings.TrimSpace(request.Remark)
	if len([]rune(request.Remark)) > maxRemarkLen {
		return request, apperror.BadRequestKey("systemsetting.remark.too_long", nil, "备注不能超过255个字符")
	}
	if !enum.IsSystemSettingValueType(request.Type) {
		return request, apperror.BadRequestKey("systemsetting.value_type.invalid", nil, "无效的配置值类型")
	}
	if appErr := validateTypedValue(request.Type, request.Value); appErr != nil {
		return request, appErr
	}
	return request, nil
}
```

`validateTypedValue` 及其后的纯转换函数不改；它们继续锁定数字、布尔、JSON object/array、字符串空白、ID 归一化、分页和显示标签规则。

删除 `dto.go` 和 `errors.go`。随后在 `request.go` 文件尾加入 Task 5 前的临时别名：

```go
// Kept only until the Admin HTTP surface moves into this package in Wave 02.
type ListQuery = ListRequest
type CreateInput = CreateRequest
type UpdateInput = UpdateRequest
```

在 `response.go` 文件尾加入：

```go
// Kept only until the Admin HTTP surface moves into this package in Wave 02.
type InitResponse = PageInitResponse
type InitDict = PageInitDict
```

这五个 type alias 是 Task 3 到 Task 5 之间唯一允许的短期兼容层，用于让旧 `transport/admin` 继续编译；Task 5 切换全部消费者后必须删除。不要改错误 code、message id 或中文 fallback。

- [ ] **Step 6: 运行 Service 测试并提交**

```powershell
gofmt -w internal/module/systemsetting/request.go internal/module/systemsetting/response.go internal/module/systemsetting/service.go internal/module/systemsetting/service_test.go
go test ./internal/module/systemsetting -run 'Test(Init|List|Create|Update|ChangeStatus|Delete)' -count=1
git diff --check
git add internal/module/systemsetting/request.go internal/module/systemsetting/response.go internal/module/systemsetting/repository.go internal/module/systemsetting/service.go internal/module/systemsetting/service_test.go internal/module/systemsetting/dto.go internal/module/systemsetting/errors.go
git commit -m "refactor(systemsetting): separate requests and responses"
```

Expected: Service 定向测试 PASS；外部 JSON 字段不变。

## Task 4：把名义缓存修成可回源的 read-through cache

当前 `SettingByKey` 只读 MySQL，而写操作删除 `sys_setting_raw_*`；这个 key 从未由 Go 写入。它不是可用缓存。目标语义固定为：

```text
Redis hit                 -> 返回缓存 Setting
Redis miss/error/bad data -> 查询 MySQL
MySQL hit                 -> best-effort 写 Redis，TTL 5 分钟
MySQL error               -> 返回错误，绝不返回缓存默认值
CRUD                      -> best-effort 删除对应 key
```

Cache Redis 已有 telemetry hook 记录命令失败，因此缓存故障不再把已经成功提交的 MySQL mutation 改写成 HTTP 500。

**Files:**

- Create: `E:/admin/admin_back_go/internal/module/systemsetting/repository_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/repository.go`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/service.go`
- Modify: `E:/admin/admin_back_go/internal/module/systemsetting/service_test.go`
- Modify: `E:/admin/admin_back_go/docs/architecture.md:1032-1042`

- [ ] **Step 1: 写 cache fake 和三个失败测试**

在新 `repository_test.go` 定义：

```go
package systemsetting

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeSettingCache struct {
	payload    string
	getErr     error
	setErr     error
	deleteErr  error
	setKey     string
	setPayload string
	setTTL     time.Duration
	deletedKey string
}

func (f *fakeSettingCache) Get(context.Context, string) (string, error) {
	return f.payload, f.getErr
}

func (f *fakeSettingCache) Set(_ context.Context, key string, payload string, ttl time.Duration) error {
	f.setKey, f.setPayload, f.setTTL = key, payload, ttl
	return f.setErr
}

func (f *fakeSettingCache) Delete(_ context.Context, key string) error {
	f.deletedKey = key
	return f.deleteErr
}

func newRepositoryTestDatabase(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock
}
```

测试 1 锁定缓存命中不查 MySQL：

```go
func TestSettingByKeyReturnsMatchingCacheEntry(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	payload, err := json.Marshal(Setting{
		ID: 15, SettingKey: "auth.captcha.ttl_minutes", SettingValue: "2",
		ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo,
	})
	if err != nil {
		t.Fatalf("marshal cache fixture: %v", err)
	}
	repository := &GormRepository{db: db, cache: &fakeSettingCache{payload: string(payload)}}

	row, err := repository.SettingByKey(context.Background(), "auth.captcha.ttl_minutes")
	if err != nil || row == nil || row.ID != 15 || row.SettingValue != "2" {
		t.Fatalf("SettingByKey() row=%#v err=%v", row, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("cache hit queried MySQL: %v", err)
	}
}
```

测试 2 锁定 miss 后回源并写固定 key/TTL：

```go
func TestSettingByKeyCachesMySQLResult(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	cache := &fakeSettingCache{getErr: redis.Nil}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `system_settings` WHERE setting_key = ? AND is_del = ? ORDER BY `system_settings`.`id` LIMIT ?")).
		WithArgs("upload.token.ttl_minutes", enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "setting_key", "setting_value", "value_type", "remark", "status", "is_del", "created_at", "updated_at",
		}).AddRow(19, "upload.token.ttl_minutes", "15", enum.SystemSettingValueNumber, "上传临时凭证有效期分钟数", enum.CommonYes, enum.CommonNo, time.Now(), time.Now()))
	repository := &GormRepository{db: db, cache: cache}

	row, err := repository.SettingByKey(context.Background(), "upload.token.ttl_minutes")
	if err != nil || row == nil || row.ID != 19 {
		t.Fatalf("SettingByKey() row=%#v err=%v", row, err)
	}
	if cache.setKey != "sys_setting_raw_upload_token_ttl_minutes" || cache.setTTL != 5*time.Minute {
		t.Fatalf("cache write key=%q ttl=%s", cache.setKey, cache.setTTL)
	}
	var cached Setting
	if err := json.Unmarshal([]byte(cache.setPayload), &cached); err != nil || cached.ID != 19 {
		t.Fatalf("cached payload=%q row=%#v err=%v", cache.setPayload, cached, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
```

测试 3 锁定 Redis 故障可回源，失效故障不向 Service 传播：

```go
func TestSettingByKeyFallsBackWhenCacheFails(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	cache := &fakeSettingCache{getErr: errors.New("redis unavailable"), setErr: errors.New("redis unavailable")}
	mock.ExpectQuery("SELECT .* FROM .*system_settings.*setting_key.*is_del.*LIMIT").
		WithArgs("auth.captcha.ttl_minutes", enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "setting_key", "setting_value", "value_type", "remark", "status", "is_del", "created_at", "updated_at",
		}).AddRow(15, "auth.captcha.ttl_minutes", "2", enum.SystemSettingValueNumber, "", enum.CommonYes, enum.CommonNo, time.Now(), time.Now()))
	repository := &GormRepository{db: db, cache: cache}

	row, err := repository.SettingByKey(context.Background(), "auth.captcha.ttl_minutes")
	if err != nil || row == nil || row.ID != 15 {
		t.Fatalf("SettingByKey() row=%#v err=%v", row, err)
	}
	cache.deleteErr = errors.New("redis unavailable")
	repository.InvalidateCache(context.Background(), row.SettingKey)
	if cache.deletedKey != "sys_setting_raw_auth_captcha_ttl_minutes" {
		t.Fatalf("deleted key=%q", cache.deletedKey)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
```

- [ ] **Step 2: 运行失败测试**

```powershell
go test ./internal/module/systemsetting -run 'TestSettingByKey' -count=1
```

Expected: FAIL，因为 repository 尚无 read-through cache；本任务随后还会把 `InvalidateCache` 从可传播错误改成 best-effort 命令。

- [ ] **Step 3: 实现最小 cache adapter 和读路径**

在 `repository.go` imports 增加 `encoding/json`、`time` 和 go-redis，定义：

```go
const systemSettingCacheTTL = 5 * time.Minute

type settingCache interface {
	Get(context.Context, string) (string, error)
	Set(context.Context, string, string, time.Duration) error
	Delete(context.Context, string) error
}

type redisSettingCache struct {
	client *redis.Client
}

func (c *redisSettingCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *redisSettingCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *redisSettingCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}
```

把 repository 字段与 constructor 改为：

```go
type GormRepository struct {
	db    *gorm.DB
	cache settingCache
}

func NewGormRepository(client *database.Client, cacheClient *redisclient.Client) *GormRepository {
	if client == nil || client.Gorm == nil {
		return nil
	}
	var cache settingCache
	if cacheClient != nil && cacheClient.Redis != nil {
		cache = &redisSettingCache{client: cacheClient.Redis}
	}
	return &GormRepository{db: client.Gorm, cache: cache}
}
```

用以下完整逻辑替换 `SettingByKey`：

```go
func (r *GormRepository) SettingByKey(ctx context.Context, key string) (*Setting, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryNotConfigured
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}

	if row := r.cachedSetting(ctx, key); row != nil {
		return row, nil
	}

	var row Setting
	err := r.db.WithContext(ctx).
		Where("setting_key = ?", key).
		Where("is_del = ?", enum.CommonNo).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.cacheSetting(ctx, row)
	return &row, nil
}

func (r *GormRepository) cachedSetting(ctx context.Context, key string) *Setting {
	if r.cache == nil {
		return nil
	}
	payload, err := r.cache.Get(ctx, cacheKey(key))
	if err != nil {
		return nil
	}
	var row Setting
	if json.Unmarshal([]byte(payload), &row) != nil || row.SettingKey != key || row.IsDel != enum.CommonNo {
		return nil
	}
	return &row
}

func (r *GormRepository) cacheSetting(ctx context.Context, row Setting) {
	if r.cache == nil {
		return
	}
	payload, err := json.Marshal(row)
	if err != nil {
		return
	}
	_ = r.cache.Set(ctx, cacheKey(row.SettingKey), string(payload), systemSettingCacheTTL)
}
```

Cache payload 不对、key 不匹配或旧格式都视为派生状态失效并回源 MySQL；不能构造默认 `Setting`。

- [ ] **Step 4: 让失效变成 best-effort，删除假错误分支**

把 `Repository` 方法改为：

```go
InvalidateCache(ctx context.Context, key string)
```

实现：

```go
func (r *GormRepository) InvalidateCache(ctx context.Context, key string) {
	if r == nil || r.cache == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	_ = r.cache.Delete(ctx, cacheKey(key))
}
```

在 `service.go` 四处删除 `if err := ...` 的 HTTP 500 包装，只保留：

```go
repo.InvalidateCache(ctx, request.Key)
repo.InvalidateCache(ctx, row.SettingKey)
```

批量删除仍按归一化后的 ID 顺序逐 key 调用。`service_test.go` fake 同步改成无返回值：

```go
func (f *fakeRepository) InvalidateCache(_ context.Context, key string) {
	f.invalidated = append(f.invalidated, key)
}
```

不要删除 i18n catalog 的旧 `systemsetting.cache_clear_failed`，跨模块 catalog 清理留到统一清理波次。

- [ ] **Step 5: 更新架构事实并复跑**

把 `docs/architecture.md` 原第 1041 行替换为：

```text
SettingByKey 使用 Cache Redis read-through cache，TTL 固定 5 分钟；Redis miss、读取失败或非法派生数据必须回源 MySQL。写入、状态、删除 best-effort 清理 cache；key 规则为 sys_setting_raw_ + setting key 中的 "." 替换为 "_"。Cache Redis 故障不能把已提交的 MySQL mutation 改写成失败。
```

执行：

```powershell
gofmt -w internal/module/systemsetting/repository.go internal/module/systemsetting/repository_test.go internal/module/systemsetting/service.go internal/module/systemsetting/service_test.go
go test ./internal/module/systemsetting -run 'Test(SettingByKey|Init|List|Create|Update|ChangeStatus|Delete)' -count=1
go test ./internal/shared/setting ./internal/module/auth ./internal/module/uploadtoken -run 'Test.*SystemSetting|Test.*TTL|Test.*CaptchaPolicy' -count=1
git diff --check
git add internal/module/systemsetting/repository.go internal/module/systemsetting/repository_test.go internal/module/systemsetting/service.go internal/module/systemsetting/service_test.go docs/architecture.md
git commit -m "fix(systemsetting): restore read through cache"
```

Expected: 系统设置、typed setting、CAPTCHA policy 和上传 Token TTL 定向测试 PASS。

## Task 5：把 Admin Handler/Route 收回模块根目录

系统设置只有 Admin HTTP 表面，本波不为一个不存在的第二平台保留 `transport/admin`。权限和操作日志仍由 `adminroute.Definition` 编译到全局中间件；这是 Wave 07 前的兼容桥，不能在本波同时再挂一套 route-level middleware。

**Files:**

- Create: `E:/admin/admin_back_go/internal/module/systemsetting/handler.go`
- Create: `E:/admin/admin_back_go/internal/module/systemsetting/handler_test.go`
- Create: `E:/admin/admin_back_go/internal/module/systemsetting/route.go`
- Modify: `E:/admin/admin_back_go/internal/platform/admin/graph.go:32,58-65`
- Modify: `E:/admin/admin_back_go/internal/server/routes_admin_foundation.go:3-24`
- Modify: `E:/admin/admin_back_go/internal/server/dependencies_test.go:25-90`
- Modify: `E:/admin/admin_back_go/internal/server/router_test.go:981-1015,2494-2533`
- Modify: `E:/admin/admin_back_go/internal/module/README.md:7-42`
- Delete: `E:/admin/admin_back_go/internal/module/systemsetting/transport/admin/request.go`
- Delete: `E:/admin/admin_back_go/internal/module/systemsetting/transport/admin/handler.go`
- Delete: `E:/admin/admin_back_go/internal/module/systemsetting/transport/admin/handler_test.go`
- Delete: `E:/admin/admin_back_go/internal/module/systemsetting/transport/admin/route.go`

- [ ] **Step 1: 先创建根包 Handler 测试**

把现有 `transport/admin/handler_test.go` 搬到模块根目录并改为 `package systemsetting`。保留该文件原有 imports（去掉旧模块别名），并为新增列表协议测试补 `encoding/json`。fake 的完整接口改为：

```go
type fakeHTTPService struct {
	listRequest  ListRequest
	createRequest CreateRequest
	statusID     int64
	status       int
}

func (f *fakeHTTPService) PageInit(context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{}}, nil
}

func (f *fakeHTTPService) List(_ context.Context, request ListRequest) (*ListResponse, *apperror.Error) {
	f.listRequest = request
	return &ListResponse{
		List: []ListItem{{ID: 1, SettingKey: "user.default_avatar"}},
		Page: Page{CurrentPage: request.CurrentPage, PageSize: request.PageSize, Total: 1, TotalPage: 1},
	}, nil
}

func (f *fakeHTTPService) Create(_ context.Context, request CreateRequest) (int64, *apperror.Error) {
	f.createRequest = request
	return 1, nil
}

func (*fakeHTTPService) Update(context.Context, int64, UpdateRequest) *apperror.Error {
	return nil
}

func (*fakeHTTPService) Delete(context.Context, []int64) *apperror.Error {
	return nil
}

func (f *fakeHTTPService) ChangeStatus(_ context.Context, id int64, status int) *apperror.Error {
	f.statusID, f.status = id, status
	return nil
}
```

保留现有四个测试场景，并只替换字段名：

```text
service.listQuery   -> service.listRequest
service.createInput -> service.createRequest
```

再增加精确列表协议测试：

```go
func TestHandlerListReturnsListObjectInsteadOfBareArray(t *testing.T) {
	router, _ := newSystemSettingHandlerRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/admin/v1/system-settings?current_page=1&page_size=20",
		nil,
	)
	router.ServeHTTP(recorder, request)

	var payload struct {
		Code int `json:"code"`
		Data struct {
			List []ListItem `json:"list"`
			Page Page       `json:"page"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || payload.Code != 0 || len(payload.Data.List) != 1 {
		t.Fatalf("unexpected list response: status=%d payload=%#v", recorder.Code, payload)
	}
}
```

增加兼容测试，把每个 Handler 重复的 nil 分支收成一个明确边界：

```go
func TestHandlerReportsMissingService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/admin/v1/system-settings?current_page=1&page_size=20",
		nil,
	)
	router.ServeHTTP(recorder, request)

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError || payload["msg"] != "系统设置服务未配置" {
		t.Fatalf("unexpected response: status=%d payload=%#v", recorder.Code, payload)
	}
}
```

Admin contract 生成器当前使用空业务 Graph 构建 Router 来收集路由，因此 Wave 07 删除该生成链前，`RegisterRoutes(router, nil)` 必须仍能注册路由并返回明确错误，不能 panic。

- [ ] **Step 2: 创建根包 Handler**

新增 `handler.go`。接口和构造器固定为：

```go
package systemsetting

import (
	"context"
	"strconv"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type HTTPService interface {
	PageInit(context.Context) (*PageInitResponse, *apperror.Error)
	List(context.Context, ListRequest) (*ListResponse, *apperror.Error)
	Create(context.Context, CreateRequest) (int64, *apperror.Error)
	Update(context.Context, int64, UpdateRequest) *apperror.Error
	Delete(context.Context, []int64) *apperror.Error
	ChangeStatus(context.Context, int64, int) *apperror.Error
}

type Handler struct {
	service HTTPService
}

func NewHandler(service HTTPService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) requireService(c *gin.Context) (HTTPService, bool) {
	if h == nil || h.service == nil {
		response.Error(c, apperror.InternalKey(
			"systemsetting.service_missing",
			nil,
			"系统设置服务未配置",
		))
		return nil, false
	}
	return h.service, true
}
```

Handler 方法使用以下完整逻辑；每个入口只调用同一个 `requireService`，不复制错误构造：

```go
func (h *Handler) PageInit(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	result, appErr := service.PageInit(c.Request.Context())
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var request ListRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.list.request.invalid", nil, "列表参数错误"))
		return
	}
	result, appErr := service.List(c.Request.Context(), request)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var request CreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.create.request.invalid", nil, "参数错误"))
		return
	}
	id, appErr := service.Create(c.Request.Context(), request)
	if appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, CreateResponse{ID: id})
}

func (h *Handler) Update(c *gin.Context) {
	service, serviceOK := h.requireService(c)
	if !serviceOK {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request UpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.update.request.invalid", nil, "参数错误"))
		return
	}
	if appErr := service.Update(c.Request.Context(), id, request); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func (h *Handler) DeleteOne(c *gin.Context) {
	service, serviceOK := h.requireService(c)
	if !serviceOK {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	if appErr := service.Delete(c.Request.Context(), []int64{id}); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	service, ok := h.requireService(c)
	if !ok {
		return
	}
	var request DeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.delete.empty", nil, "请选择要删除的配置"))
		return
	}
	if appErr := service.Delete(c.Request.Context(), request.IDs); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func (h *Handler) ChangeStatus(c *gin.Context) {
	service, serviceOK := h.requireService(c)
	if !serviceOK {
		return
	}
	id, ok := routeID(c)
	if !ok {
		return
	}
	var request StatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, apperror.BadRequestKey("systemsetting.status.invalid", nil, "无效的状态"))
		return
	}
	if appErr := service.ChangeStatus(c.Request.Context(), id, request.Status); appErr != nil {
		response.Error(c, appErr)
		return
	}
	response.OK(c, EmptyResponse{})
}

func routeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, apperror.BadRequestKey("systemsetting.id.invalid", nil, "无效的配置ID"))
		return 0, false
	}
	return id, true
}
```

- [ ] **Step 3: 创建根包 Route，保持权限和审计事实**

新增 `route.go`，完整内容如下：

```go
package systemsetting

import (
	"net/http"

	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, service HTTPService, registries ...*adminroute.Registry) {
	validate.MustRegister()
	handler := NewHandler(service)
	routes := adminroute.NewRegistrar(router, registries...)

	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/system-settings/page-init",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Response: PageInitResponse{},
		},
	}, handler.PageInit)
	routes.Handle(adminroute.Definition{
		Method: http.MethodGet,
		Path:   "/api/admin/v1/system-settings",
		Access: adminroute.Authenticated(),
		Audit:  adminroute.NoAudit("read-only"),
		Contract: &adminroute.HTTPContract{
			Query:    ListRequest{},
			Response: ListResponse{},
		},
	}, handler.List)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPost,
		Path:   "/api/admin/v1/system-settings",
		Access: adminroute.Permission("system_setting_add"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "create",
			Title:   "新增系统设置",
		},
		Contract: &adminroute.HTTPContract{
			Request:  CreateRequest{},
			Response: CreateResponse{},
		},
	}, handler.Create)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPut,
		Path:   "/api/admin/v1/system-settings/:id",
		Access: adminroute.Permission("system_setting_edit"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "update",
			Title:   "编辑系统设置",
		},
		Contract: &adminroute.HTTPContract{
			Request:  UpdateRequest{},
			Response: EmptyResponse{},
		},
	}, handler.Update)
	routes.Handle(adminroute.Definition{
		Method: http.MethodPatch,
		Path:   "/api/admin/v1/system-settings/:id/status",
		Access: adminroute.Permission("system_setting_status"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "change_status",
			Title:   "修改系统设置状态",
		},
		Contract: &adminroute.HTTPContract{
			Request:  StatusRequest{},
			Response: EmptyResponse{},
		},
	}, handler.ChangeStatus)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/system-settings/:id",
		Access: adminroute.Permission("system_setting_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "delete",
			Title:   "删除系统设置",
		},
		Contract: &adminroute.HTTPContract{
			Response: EmptyResponse{},
		},
	}, handler.DeleteOne)
	routes.Handle(adminroute.Definition{
		Method: http.MethodDelete,
		Path:   "/api/admin/v1/system-settings",
		Access: adminroute.Permission("system_setting_del"),
		Audit: adminroute.AuditDecision{
			Enabled: true,
			Module:  "system_setting",
			Action:  "delete_batch",
			Title:   "批量删除系统设置",
		},
		Contract: &adminroute.HTTPContract{
			Request:  DeleteRequest{},
			Response: EmptyResponse{},
		},
	}, handler.DeleteBatch)
}
```

七个 Definition 的路径、Access 和 Audit 必须精确保持：

| Method + Path | Access | Audit |
|---|---|---|
| `GET /api/admin/v1/system-settings/page-init` | Authenticated | NoAudit(`read-only`) |
| `GET /api/admin/v1/system-settings` | Authenticated | NoAudit(`read-only`) |
| `POST /api/admin/v1/system-settings` | `system_setting_add` | `system_setting/create/新增系统设置` |
| `PUT /api/admin/v1/system-settings/:id` | `system_setting_edit` | `system_setting/update/编辑系统设置` |
| `PATCH /api/admin/v1/system-settings/:id/status` | `system_setting_status` | `system_setting/change_status/修改系统设置状态` |
| `DELETE /api/admin/v1/system-settings/:id` | `system_setting_del` | `system_setting/delete/删除系统设置` |
| `DELETE /api/admin/v1/system-settings` | `system_setting_del` | `system_setting/delete_batch/批量删除系统设置` |

不要在 Route 上再挂 `middleware.AuthToken/PermissionCheck/OperationLog`。当前 Router 在注册业务路由前已挂全局中间件，并从同一 Registry 读取规则；本波双挂会重复鉴权和重复写操作日志。

- [ ] **Step 4: 切换装配 import 和测试接口**

做以下精确 import/类型替换：

```go
// internal/platform/admin/graph.go
import "admin_back_go/internal/module/systemsetting"
Settings systemsetting.HTTPService

// internal/server/routes_admin_foundation.go
import "admin_back_go/internal/module/systemsetting"
systemsetting.RegisterRoutes(router, system.Settings, deps.Core.RouteRegistry)

// internal/server/dependencies_test.go
import "admin_back_go/internal/module/systemsetting"
SystemSettingService systemsetting.HTTPService
```

在 `router_test.go` fake 中替换：

```text
ListQuery        -> ListRequest
InitResponse     -> PageInitResponse
List(context.Context, ListQuery) -> List(context.Context, ListRequest)
CreateInput      -> CreateRequest
UpdateInput      -> UpdateRequest
```

保留 `TestRouterInstallsSystemSettingRESTRoutes` 的 URL、Authorization header 和断言；不得把它改成只测直接 Handler 而绕过全局 Auth/Permission。

完成 import 切换后，从 `request.go` 删除：

```go
type ListQuery = ListRequest
type CreateInput = CreateRequest
type UpdateInput = UpdateRequest
```

从 `response.go` 删除：

```go
type InitResponse = PageInitResponse
type InitDict = PageInitDict
```

这些别名只用于让 Task 3/4 的独立提交可编译，不能留在最终样板中。

把 `internal/module/README.md` 的长期结构改成已批准的迁移态规则：

````markdown
单一 Admin HTTP 表面的已迁移模块采用：

```text
internal/module/{capability}/
  model.go
  request.go
  response.go
  repository.go
  service.go
  handler.go
  route.go
```

当同一能力真实存在 Admin/App/Canvas 两个以上 HTTP 表面时，才把平台差异拆到：

```text
internal/module/{capability}/transport/{platform}/
  request.go
  handler.go
  route.go
```

Service、Repository 和 Model 始终共享。不得为了未来可能出现的第二平台预建 transport 目录；也不得在第二平台出现后复制业务 Service。
````

同时把职责表改成 `route.go / handler.go / request.go / response.go / service.go / repository.go / model.go`；保留现有 jobs、错误协议和多平台说明。注明 `systemsetting` 是 Wave 02 第一条新样板，其他模块在各自 Wave 前仍以现状为准，不能一次性机械搬目录。

- [ ] **Step 5: 删除 transport/admin 并运行定向测试**

删除四个旧文件及空目录，确认引用清零：

```powershell
rg -n 'systemsetting/transport/admin|systemsettingadmin|systemsetting\.(ListQuery|CreateInput|UpdateInput|InitResponse|InitDict)' internal --glob '*.go'
rg -n '\b(ListQuery|CreateInput|UpdateInput|InitResponse|InitDict|listRequest|createRequest|updateRequest|deleteBatchRequest|statusRequest)\b' internal/module/systemsetting --glob '*.go'
```

Expected: 两次搜索都没有输出；其他模块自己的同名 `ListQuery/CreateInput/UpdateInput` 不属于本波。

执行：

```powershell
gofmt -w internal/module/systemsetting internal/platform/admin/graph.go internal/server/routes_admin_foundation.go internal/server/dependencies_test.go internal/server/router_test.go
go test ./internal/module/systemsetting -count=1
go test ./internal/platform/admin -run 'TestBuild' -count=1
go test ./internal/server -run 'TestRouterInstallsSystemSettingREST|TestRouteRegistry' -count=1
git diff --check
git add internal/module/systemsetting internal/module/README.md internal/platform/admin/graph.go internal/server/routes_admin_foundation.go internal/server/dependencies_test.go internal/server/router_test.go
git commit -m "refactor(systemsetting): flatten admin crud flow"
```

Expected: 三组定向测试 PASS；旧 `transport/admin` 已删除，路由行为不变。

## Task 6：让前端系统设置 API 成为人可读事实源

本波不修改 Vue 页面、组件、样式或 `useCrudTable`。只把 `setting.ts` 从 generated types/operations 切换到现有 `request` facade，并让 facade 接受模块自有的响应 schema。校验仍发生在 `ApiClient` 内，因此业务错误、非法 envelope 和非法 `{list,page}` 都走同一全局错误通知边界；API wrapper 不手写 `throw` 吞掉通知。用 API 测试锁住 URL、query、body 与 `{list,page}` 协议。

**Files:**

- Modify: `E:/admin/admin_front_ts/src/api/system/setting.ts`
- Modify: `E:/admin/admin_front_ts/src/lib/http/index.ts`
- Create: `E:/admin/admin_front_ts/tests/shared/system/system-setting-api.test.ts`
- Modify: `E:/admin/admin_front_ts/tests/helpers/api-client.ts`
- Modify: `E:/admin/admin_front_ts/tests/unit/http/architecture.test.ts`

- [ ] **Step 1: 写前端 API 失败测试**

新增测试文件：

```ts
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SystemSettingApi } from '@/api/system/setting'
import type { ApiError } from '@/modules/http/error'
import { installApiClientHarness } from '../../helpers/api-client'

const cleanups: Array<() => void> = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

describe('system setting API behavior', () => {
  it('keeps the documented list object and mutation endpoints', async () => {
    const harness = installApiClientHarness({
      dict: { system_setting_value_type_arr: [
        { label: '字符串', value: 1 },
        { label: '数字', value: 2 },
        { label: '布尔', value: 3 },
        { label: 'JSON', value: 4 },
      ] },
    })
    cleanups.push(harness.uninstall)

    await SystemSettingApi.pageInit()
    harness.respondWith({
      list: [{
        id: 1,
        setting_key: 'user.default_avatar',
        setting_value: 'https://cos.zgm2003.cn/avatar.png',
        value_type: 1,
        value_type_name: '字符串',
        remark: '用户注册头像',
        status: 1,
        status_name: '启用',
        is_del: 2,
        created_at: '2026-08-13 10:00:00',
        updated_at: '2026-08-13 10:00:00',
      }],
      page: { current_page: 1, page_size: 20, total: 1, total_page: 1 },
    })
    const list = await SystemSettingApi.list({
      current_page: 1,
      page_size: 20,
      key: 'user.',
      status: 1,
    })
    expect(list.list[0]?.setting_key).toBe('user.default_avatar')

    harness.respondWith({ id: 20 })
    await SystemSettingApi.create({ key: 'feature.switch', value: 'true', type: 3, remark: '开关' })
    harness.respondWith({})
    await SystemSettingApi.update({ id: 20, value: 'false', type: 3, remark: '开关' })
    await SystemSettingApi.changeStatus({ id: 20, status: 2 })
    await SystemSettingApi.deleteOne({ id: 20 })
    await SystemSettingApi.deleteBatch({ ids: [20, 21] })

    expect(harness.requests.map(({ method, path, query, body }) => ({ method, path, query, body }))).toEqual([
      { method: 'GET', path: '/api/admin/v1/system-settings/page-init', query: undefined, body: undefined },
      { method: 'GET', path: '/api/admin/v1/system-settings', query: { current_page: 1, page_size: 20, key: 'user.', status: 1 }, body: undefined },
      { method: 'POST', path: '/api/admin/v1/system-settings', query: undefined, body: { key: 'feature.switch', value: 'true', type: 3, remark: '开关' } },
      { method: 'PUT', path: '/api/admin/v1/system-settings/20', query: undefined, body: { value: 'false', type: 3, remark: '开关' } },
      { method: 'PATCH', path: '/api/admin/v1/system-settings/20/status', query: undefined, body: { status: 2 } },
      { method: 'DELETE', path: '/api/admin/v1/system-settings/20', query: undefined, body: undefined },
      { method: 'DELETE', path: '/api/admin/v1/system-settings', query: undefined, body: { ids: [20, 21] } },
    ])
  })

  it('rejects malformed identities and response contracts instead of guessing', async () => {
    const onError = vi.fn((error: ApiError) => error)
    const harness = installApiClientHarness({ list: [] }, { onError })
    cleanups.push(harness.uninstall)

    await expect(SystemSettingApi.list({ current_page: 1, page_size: 20 })).rejects.toMatchObject({
      kind: 'contract',
      code: 'http.response_required_field_missing',
    })
    expect(onError).toHaveBeenCalledOnce()
    await expect(SystemSettingApi.deleteOne({ id: 0 })).rejects.toThrow(/positive integer/i)
    expect(harness.requests).toHaveLength(1)

    onError.mockClear()
    harness.respondWith({
      list: [{ id: 1, value_type: 9, status: 1 }],
      page: { current_page: 1, page_size: 20, total: 1, total_page: 1 },
    })
    await expect(SystemSettingApi.list({ current_page: 1, page_size: 20 })).rejects.toMatchObject({
      kind: 'contract',
      code: 'http.response_required_field_missing',
    })
    expect(onError).toHaveBeenCalledOnce()
    expect(harness.requests).toHaveLength(2)
  })
})
```

第一处 malformed response 用 `{list: []}` 故意缺 `page`；实现不能补默认 page。第二处列表项故意缺必需字段且带非法 `value_type`，不能被强转或补空值。两次服务端响应合同错误都必须经过 `ApiClient.onError`；本地非法 ID 在发请求前失败，不应触发全局请求通知。

- [ ] **Step 2: 运行当前测试确认架构门禁和行为不满足**

```powershell
npm test -- --run tests/shared/system/system-setting-api.test.ts tests/unit/http/architecture.test.ts
```

Expected: FAIL。现有兼容 `request` 不接受业务响应 schema，系统设置 API 仍依赖 generated operation；architecture test 也禁止 literal path/compat request，需要在同一任务为已迁移模块建立显式 allowlist。

- [ ] **Step 3: 让兼容 request 把响应 schema 交给唯一 ApiClient**

这是 direct request 唯一缺失的边界能力，不创建第二套 HTTP client，也不在 API wrapper 捕获后重新通知。

在 `src/lib/http/index.ts` 增加 type-only import：

```ts
import type { z } from 'zod'
```

把 `RequestConfig` 和 `RequestClient` 改成：

```ts
export interface RequestConfig<T = unknown, D = unknown> {
  readonly params?: object
  readonly data?: D
  readonly signal?: AbortSignal
  readonly idempotencyKey?: string
  readonly responseSchema?: z.ZodType<T>
}

export interface RequestClient {
  get<T = unknown>(url: string, config?: RequestConfig<T>): Promise<T>
  post<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
  put<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
  patch<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
  delete<T = unknown, D = unknown>(url: string, config?: RequestConfig<T, D>): Promise<T>
}
```

把 `execute` 签名改为 `config: RequestConfig<T, D> | undefined`，并在 `defineOperation` 中加入：

```ts
responseSchema: config?.responseSchema,
```

五个 facade 方法的 config 泛型同步为：

```ts
get<T = unknown>(url: string, config?: RequestConfig<T>): Promise<T>
post<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
put<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
patch<T = unknown, D = unknown>(url: string, data?: D, config?: RequestConfig<T, D>): Promise<T>
delete<T = unknown, D = unknown>(url: string, config?: RequestConfig<T, D>): Promise<T>
```

`responseSchema` 只描述成功 envelope 的 `data`；公共 `code/data/msg/error` 仍只由 `ApiClient.decodeResponse` 解释。不要在 request facade 新增 `try/catch`、通知或默认数据。

为让 API 测试观察同一全局错误边界，把 `tests/helpers/api-client.ts` 的 helper 签名改为：

```ts
import type { ApiClientOptions, TransportRequest } from '@/modules/http/client'

export function installApiClientHarness(
  initialData: unknown = {},
  options: Pick<ApiClientOptions, 'onError'> = {},
) {
```

创建 client 时增加：

```ts
onError: options.onError,
```

其余 harness 行为不变；现有所有单参数调用继续兼容。

- [ ] **Step 4: 用显式业务类型和 schema 重写 setting.ts**

完整替换 imports：

```ts
import { z } from 'zod'
import request from '@/lib/http'
import type { ExecuteOptions } from '@/modules/http/client'
import type { DictOption, Id, PageInfo } from '@/types/common'
```

业务类型固定为：

```ts
export type SystemSettingValueType = 1 | 2 | 3 | 4
export type SystemSettingStatus = 1 | 2

export interface SystemSettingInitResponse {
  dict: { system_setting_value_type_arr: DictOption<SystemSettingValueType>[] }
}

export interface SystemSettingListParams {
  current_page: number
  page_size: number
  key?: string
  status?: SystemSettingStatus | ''
}

export interface SystemSettingItem {
  id: number
  setting_key: string
  setting_value: string
  value_type: SystemSettingValueType
  value_type_name: string
  remark: string
  status: SystemSettingStatus
  status_name: string
  is_del: number
  created_at: string
  updated_at: string
}

export interface SystemSettingListResponse {
  list: SystemSettingItem[]
  page: Required<PageInfo>
}

export interface SystemSettingAddParams {
  key: string
  value: string
  type: SystemSettingValueType
  remark: string
}

export interface SystemSettingEditParams {
  id: number
  value: string
  type: SystemSettingValueType
  remark: string
}

export interface SystemSettingCreateResponse { id: number }
export interface SystemSettingBatchDeletePayload { ids: number[] }
export interface SystemSettingStatusPayload { id: Id; status: SystemSettingStatus }
```

保留现有 `normalizeSystemSettingIDs`；删除 `isValueType` 和 generated 专用类型，值域由下面唯一 schema 表达。用模块本地 schema 表达后端 `data`，不依赖 generated component：

```ts
const valueTypeSchema = z.union([z.literal(1), z.literal(2), z.literal(3), z.literal(4)])
const statusSchema = z.union([z.literal(1), z.literal(2)])
const pageSchema = z.object({
  page_size: z.number().int().positive(),
  current_page: z.number().int().positive(),
  total_page: z.number().int().nonnegative(),
  total: z.number().int().nonnegative(),
}).strict()
const itemSchema: z.ZodType<SystemSettingItem> = z.object({
  id: z.number().int().positive(),
  setting_key: z.string(),
  setting_value: z.string(),
  value_type: valueTypeSchema,
  value_type_name: z.string(),
  remark: z.string(),
  status: statusSchema,
  status_name: z.string(),
  is_del: z.number().int(),
  created_at: z.string(),
  updated_at: z.string(),
}).strict()
const pageInitSchema: z.ZodType<SystemSettingInitResponse> = z.object({
  dict: z.object({
    system_setting_value_type_arr: z.array(z.object({
      label: z.string(),
      value: valueTypeSchema,
    }).strict()),
  }).strict(),
}).strict()
const listSchema: z.ZodType<SystemSettingListResponse> = z.object({
  list: z.array(itemSchema),
  page: pageSchema,
}).strict()
const createSchema: z.ZodType<SystemSettingCreateResponse> = z.object({
  id: z.number().int().positive(),
}).strict()
const emptySchema = z.object({}).strict()

function optionsFrom(options: ExecuteOptions) {
  return { signal: options.signal, idempotencyKey: options.idempotencyKey }
}

function listQuery(params: SystemSettingListParams): Record<string, string | number> {
  const query: Record<string, string | number> = {
    current_page: params.current_page,
    page_size: params.page_size,
  }
  if (params.key) query.key = params.key
  if (typeof params.status === 'number') query.status = params.status
  return query
}
```

API 实现固定为：

```ts
const basePath = '/api/admin/v1/system-settings'

export const SystemSettingApi = {
  pageInit: (options: ExecuteOptions = {}): Promise<SystemSettingInitResponse> =>
    request.get(`${basePath}/page-init`, { ...optionsFrom(options), responseSchema: pageInitSchema }),

  list: (params: SystemSettingListParams, options: ExecuteOptions = {}): Promise<SystemSettingListResponse> =>
    request.get(basePath, {
      ...optionsFrom(options),
      params: listQuery(params),
      responseSchema: listSchema,
    }),

  create: (params: SystemSettingAddParams, options: ExecuteOptions = {}): Promise<SystemSettingCreateResponse> =>
    request.post(basePath, params, { ...optionsFrom(options), responseSchema: createSchema }),

  update: async (params: SystemSettingEditParams, options: ExecuteOptions = {}): Promise<void> => {
    const { id, ...body } = params
    await request.put(`${basePath}/${normalizeSystemSettingIDs(id)[0]}`, body, {
      ...optionsFrom(options), responseSchema: emptySchema,
    })
  },

  deleteOne: async (params: { id: Id }, options: ExecuteOptions = {}): Promise<void> => {
    await request.delete(`${basePath}/${normalizeSystemSettingIDs(params.id)[0]}`, {
      ...optionsFrom(options), responseSchema: emptySchema,
    })
  },

  deleteBatch: async (params: SystemSettingBatchDeletePayload, options: ExecuteOptions = {}): Promise<void> => {
    await request.delete(basePath, {
      ...optionsFrom(options),
      data: { ids: normalizeSystemSettingIDs(params.ids) },
      responseSchema: emptySchema,
    })
  },

  changeStatus: async (params: SystemSettingStatusPayload, options: ExecuteOptions = {}): Promise<void> => {
    await request.patch(`${basePath}/${normalizeSystemSettingIDs(params.id)[0]}/status`,
      { status: params.status }, { ...optionsFrom(options), responseSchema: emptySchema })
  },
}
```

不要加 `response?.list ?? []`、`page || defaultPage`、数字字符串转换，或在 wrapper 里 `catch` 后自行构造通知。响应不符必须由 `ApiClient` 产生 `ApiError(kind='contract')` 并调用已安装的 `onError`。

- [ ] **Step 5: 把 HTTP 架构测试改成显式迁移名单**

在 `tests/unit/http/architecture.test.ts` 的 “keeps feature API wrappers...” 测试中增加显式的当前迁移名单。仓库现在已有 `ai/agents.ts` 和 `ai/official-models.ts` 使用 direct `request`，它们也必须进入名单，不能让本波假装既有事实不存在：

```ts
const migratedRequestApis = new Set([
  'ai/agents.ts',
  'ai/official-models.ts',
  'system/setting.ts',
])
```

循环内只有 allowlist 成员允许 `import request` 和 `/api/` literal：

```ts
if (!migratedRequestApis.has(relativePath) && /import\s+request(?:\s|,)/.test(source)) {
  reasons.push('compat request import')
}
if (!migratedRequestApis.has(relativePath) && /(['"`])\/api\//.test(source)) {
  reasons.push('literal API path')
}
```

这不是全局放开；每个后续模块必须在自己迁移时显式加入，Wave 07 删除旧 generated 架构测试后再删除 allowlist。

- [ ] **Step 6: 运行前端定向测试并提交**

```powershell
npm test -- --run tests/shared/system/system-setting-api.test.ts tests/shared/http/client-error-contract.test.ts tests/unit/http/architecture.test.ts
npm run typecheck
git diff --check
git add src/api/system/setting.ts src/lib/http/index.ts tests/helpers/api-client.ts tests/shared/system/system-setting-api.test.ts tests/unit/http/architecture.test.ts
git commit -m "refactor(systemsetting): use direct frontend api"
```

Expected: 三个 Vitest 文件 PASS，`vue-tsc -b` PASS；非法系统设置响应经过根请求 `onError`，`src/views/Main/system/setting/index.vue` 无改动。

## Task 7：同步 Wave 07 前的 Admin contract 兼容产物

根包迁移会把 OpenAPI component 名从 `Go_internal_module_systemsetting_*` 改成相同字段的新包路径类型名。前端系统设置 API 已不再依赖这些 component，但仓库当前质量门仍要求 bundle、lock 和 generated outputs 自洽。本任务只机械同步，不新增业务类型。

**Files:**

- Modify generated backend bundle: `E:/admin/admin_back_go/contracts/admin/v1/**`
- Modify synchronized frontend bundle: `E:/admin/admin_front_ts/contracts/backend/admin/**`
- Modify generated frontend files: `E:/admin/admin_front_ts/src/modules/http/generated/admin.ts`
- Modify generated frontend files: `E:/admin/admin_front_ts/src/modules/http/generated/operations.ts`
- Modify generated frontend files if hashes change: `E:/admin/admin_front_ts/src/modules/routing/generated/permissions.ts`
- Modify generated frontend files if hashes change: `E:/admin/admin_front_ts/src/modules/routing/generated/views.ts`

- [ ] **Step 1: 取得后端业务提交的完整 SHA**

Task 5 的后端提交必须已经完成且后端工作区干净：

```powershell
git status --short
$backendCommit = git rev-parse HEAD
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'backend HEAD is not a full Git SHA' }
```

Expected: `git status --short` 无输出。

- [ ] **Step 2: 重新生成后端兼容 bundle**

```powershell
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendCommit
go test ./internal/admincontract -run 'TestSystemAndCommunicationsRoutesPublishRuntimeModelContracts|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1 -BackendCommit $backendCommit
git diff --check
git add contracts/admin/v1
git commit -m "chore(contract): refresh system setting schema"
```

Expected: 两个 Admin contract 定向测试和 check PASS。不要手工编辑 `openapi.json`、hash 或 manifest。

- [ ] **Step 3: 同步前端 bundle 和生成文件**

前端必须同步 manifest 中记录的业务提交 SHA，而不是后端生成物提交 SHA。不要依赖上一个 PowerShell 进程的局部变量；从已生成 manifest 精确读取：

```powershell
$backendCommit = [string](Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'contract manifest has no full backend SHA' }
node scripts/sync-admin-contract.mjs --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

Expected: sync、generate、check 均 PASS。虽然 generated `systemsetting` schema 名变化，`src/api/system/setting.ts` 不应重新出现 generated import。

- [ ] **Step 4: 运行前端定向回归并提交生成物**

```powershell
npm test -- --run tests/shared/system/system-setting-api.test.ts tests/shared/http/client-error-contract.test.ts tests/unit/http/architecture.test.ts tests/unit/http/generated-operations.test.ts
npm run typecheck
git diff --check
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated
git commit -m "chore(contract): sync system setting schema"
```

Expected: 四个 Vitest 文件和 typecheck PASS。不要运行完整 `contract:sync` 之外的发布验证脚本。

## Task 8：Wave 02 收口与人工验收交接

**Files:**

- Read-only: `E:/admin/admin_back_go/internal/module/systemsetting`
- Read-only: `E:/admin/admin_back_go/internal/shared/setting`
- Read-only: `E:/admin/admin_back_go/database/schema.sql`
- Read-only: `E:/admin/admin_back_go/database/seed.sql`
- Read-only: `E:/admin/admin_front_ts/src/api/system/setting.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/system/setting/index.vue`

- [ ] **Step 1: 运行最终短测试集合**

Backend, from `E:/admin/admin_back_go`:

```powershell
go test ./internal/module/systemsetting -count=1
go test ./internal/shared/setting ./internal/module/auth ./internal/module/uploadtoken -run 'Test.*SystemSetting|Test.*TTL|Test.*CaptchaPolicy' -count=1
go test ./internal/platform/admin -run 'TestBuild' -count=1
go test ./internal/server -run 'TestRouterInstallsSystemSettingREST|TestRouteRegistry' -count=1
go test ./internal/architecture -run 'TestDatabaseBaselineSeedContract|TestLocalSystemSettingSeedPreservesDefaultAvatar' -count=1
```

Frontend, from `E:/admin/admin_front_ts`:

```powershell
npm test -- --run tests/shared/system/system-setting-api.test.ts tests/shared/http/client-error-contract.test.ts tests/unit/http/architecture.test.ts tests/unit/http/generated-operations.test.ts
npm run typecheck
```

不要运行全量 Go/Vue 测试、Playwright、`verify:frontend` 或 release verifier。

- [ ] **Step 2: 检查结构和兼容边界**

```powershell
rg -n 'systemsetting/transport/admin|systemsettingadmin|systemsetting\.(ListQuery|CreateInput|UpdateInput|InitResponse|InitDict)' E:/admin/admin_back_go/internal --glob '*.go'
rg -n '\b(ListQuery|CreateInput|UpdateInput|InitResponse|InitDict|listRequest|createRequest|updateRequest|deleteBatchRequest|statusRequest)\b' E:/admin/admin_back_go/internal/module/systemsetting --glob '*.go'
rg -n "modules/http/generated|executeAdminOperation|adminOperations" E:/admin/admin_front_ts/src/api/system/setting.ts
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts diff --check
git -C E:/admin/admin_front_ts status --short
```

Expected: 三次 `rg` 都无输出；两个仓库 `diff --check` 和 `status --short` 均干净。

人工核对以下不变量：

```text
1. system_settings 表仍为原字段和索引，Seed 仍为 4 条，默认头像仍存在。
2. 七个 API path/method/request/response 不变，列表 data 仍是 {list,page}。
3. 权限码 system_setting_add/edit/del/status 不变，操作日志 action 不变。
4. 中英文 systemsetting catalog 未删除，英文请求错误仍能本地化。
5. CAPTCHA 与上传 Token 仍从 shared/setting 读取原 key。
6. Cache Redis 故障时 typed setting 回源 MySQL；MySQL 错误仍明确失败。
7. 前端页面、菜单、按钮和用户操作习惯无变化。
```

- [ ] **Step 3: 输出人工验收清单并停止**

交接只报告：

```text
- 后端/前端提交 SHA 与提交标题；
- 每条实际运行的短测试及 PASS/FAIL；
- 未运行的全量测试明确写“未运行”；
- 系统设置新目录树；
- Wave 01 两项门禁的实际处理结果；
- 人工验收：列表、创建、编辑、启停、单删、批删、默认头像、英文错误；
- 下一入口：用户验收通过后，单独只读盘点 Wave 03，不自动开始。
```

不要启动 `admin-dev`，不要进入 Wave 03。

## 2. 计划完成后的业务调用链

```text
GET/POST/PUT/PATCH/DELETE /api/admin/v1/system-settings
-> Router 全局 AuthToken / PermissionCheck / OperationLog
-> systemsetting.RegisterRoutes
-> systemsetting.Handler
-> systemsetting.Service
-> systemsetting.Repository
-> MySQL system_settings
-> Cache Redis DB 0（可回源派生缓存）
```

前端：

```text
views/Main/system/setting/index.vue
-> api/system/setting.ts
-> lib/http request
-> ApiClient envelope/error handling
-> backend
```

旧 generated contract 在 Wave 02 后只承担仓库兼容质量门，不再决定系统设置前端业务类型。它的最终删除仍属于 Wave 07，不能提前扩散到其他模块。
