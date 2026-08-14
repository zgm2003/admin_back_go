# Admin 架构减法 Wave 03 Role 模块 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变角色 CRUD、权限矩阵、默认角色、数据库关系和通知任务角色选择行为的前提下，为角色管理建立页面级读权限，把后端分页和前端 API 收口到已确认的公共入口，并删除引用清零的旧级联选择器 helper。

**Architecture:** 后端保留 `internal/module/role` 能力根和长期 `transport/admin` HTTP 表面，只删除重复分页与未接入运行时的旧缓存失效支路；角色权限变更继续通过现有 Principal Mutation Coordinator 在事务边界内失效授权事实。前端保留 `use-role-page.ts`、`useCrudTable` 和现有权限矩阵，`src/api/permission/role.ts` 改为直接使用 `src/utils/request.ts` 与严格 Zod schema，不再直接依赖 generated operations/types。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Redis 8、Vue 3.5、TypeScript 5.9、Zod 4、Vitest 4、现有迁移期 Admin contract 生成链。

---

## 0. 执行边界

本计划只处理以下 Admin Role 入口：

```text
GET    /api/admin/v1/roles/page-init
GET    /api/admin/v1/roles
POST   /api/admin/v1/roles
PUT    /api/admin/v1/roles/:id
PATCH  /api/admin/v1/roles/:id/default
DELETE /api/admin/v1/roles/:id
DELETE /api/admin/v1/roles
```

必须保持：

- API 路径、HTTP method、查询字段、JSON 字段和 `code/data/msg/error` 外层协议；
- `internal/module/role/transport/admin` 作为长期 Admin HTTP 表面；
- `roles`、`role_permissions`、`users.role_id` 的表字段和现有索引；
- `permission_role_add`、`permission_role_edit`、`permission_role_del`、`permission_role_setDefault` 的按钮权限语义；
- 页面 ID 13 作为“角色管理”页面权限事实，不新增权限行或伪造“页面访问”按钮；
- 默认角色不可删除、已绑定用户角色不可删除、软删除同名角色恢复复用原 ID；
- 按钮权限自动包含所属页面权限，目录 ID 不写入 `role_permissions`；
- 角色授权变更通过 Principal Mutation Coordinator 使所有绑定用户的授权版本失效；Redis 失败时不得先提交 MySQL；
- `RoleApi.list` 继续供通知任务的角色 RemoteSelect 使用；
- 现有 Role 权限矩阵 UI、页面/按钮联动、差异确认弹窗和多平台页签。

本计划明确不处理：

- Permission 管理页面 CRUD；
- User、Mail、SMS、日志、上传、支付或 AI 模块迁移；
- 角色状态字段或“启停角色”功能，因为当前 `roles` 表和 API 没有该业务事实；
- API 路径、数据库表结构或现有按钮权限码重命名；
- `src/lib`、`src/modules` 或生成合同体系的全局物理删除；
- Role 权限矩阵视觉重设计；
- `admin-dev`、全量 Go/Vue 测试、全量 typecheck、Playwright、`verify:frontend` 或发布长脚本。

用户确认正确的 403 全局通知修改已经独立提交：

```text
Frontend commit: 9b034cb474115fc5ae6712c5408011631c15657d
src/lib/http/notifier.ts
tests/shared/http/notifier.test.ts
```

它不是 Role 业务代码。执行者只验证该既有提交仍在当前分支并通过定向测试，不得重做、回滚或混入 Role 提交。

## 1. 文件职责锁定

```text
E:/admin/admin_back_go/database/migrations/202608140002_set_role_page_code.sql
  为现有页面 ID 13 设置稳定 code=permission_role；只更新既有行，不新增权限或角色授权。

E:/admin/admin_back_go/internal/module/role/dto.go
  保留 Role 业务 DTO 和 ListQuery；删除本地 Page，ListResponse 使用 shared pagination。

E:/admin/admin_back_go/internal/module/role/service.go
  保留角色规则、权限归一化和事务；删除运行时从未装配的 CacheInvalidator 支路。

E:/admin/admin_back_go/internal/module/role/repository.go
  保留角色、角色权限和绑定用户的 MySQL 访问；不把业务规则下沉到 Repository。

E:/admin/admin_back_go/internal/module/role/transport/admin/route.go
  两个管理 GET 使用 permission_role；写操作继续使用四个现有按钮权限码。

E:/admin/admin_front_ts/src/api/permission/role.ts
  Role API 唯一实现；直接 request、严格 Zod、公共分页，不导入 generated admin/operations。

E:/admin/admin_front_ts/src/views/Main/permission/role/use-role-page.ts
  保留页面状态、useCrudTable、表单、默认角色确认和权限差异确认；不再增加 workflow。

E:/admin/admin_front_ts/src/views/Main/permission/role/role-matrix.ts
  保留页面/按钮权限矩阵的纯数据规则；本计划不改其交互语义。

E:/admin/admin_front_ts/src/views/Main/permission/role/helpers.ts
  旧级联选择器 helper；引用清零后删除，不迁移到 utils。
```

## 2. Task 0：验证已提交的 403 通知基线

**Files:**

- Verify: `E:/admin/admin_front_ts/src/lib/http/notifier.ts`
- Verify: `E:/admin/admin_front_ts/tests/shared/http/notifier.test.ts`

- [ ] **Step 1: 确认既有提交位于当前前端历史**

```powershell
git merge-base --is-ancestor 9b034cb474115fc5ae6712c5408011631c15657d HEAD
git show --stat --oneline 9b034cb474115fc5ae6712c5408011631c15657d
git show --format= 9b034cb474115fc5ae6712c5408011631c15657d -- src/lib/http/notifier.ts tests/shared/http/notifier.test.ts
```

Expected：第一条命令退出码为 0；提交只修改 notifier 及其测试；401 和 404 仍静默，`authorization`/403 返回 `true` 并触发全局通知。

- [ ] **Step 2: 运行通知定向测试和 ESLint**

```powershell
npm test -- tests/shared/http/notifier.test.ts
npx eslint src/lib/http/notifier.ts tests/shared/http/notifier.test.ts
```

Expected：定向测试 PASS，ESLint 无错误。

- [ ] **Step 3: 锁定 Role 执行前的前端恢复点**

```powershell
git status --short
git rev-parse HEAD
```

Expected：前端工作区干净；Role 执行前 HEAD 至少包含 `9b034cb474115fc5ae6712c5408011631c15657d`。Task 0 不创建新提交。

## 3. Task 1：建立角色管理页面访问权限事实

**Files:**

- Create: `E:/admin/admin_back_go/database/migrations/202608140002_set_role_page_code.sql`
- Modify: `E:/admin/admin_back_go/database/seed.sql`
- Modify: `E:/admin/admin_back_go/database/baseline.json`
- Modify: `E:/admin/admin_back_go/internal/architecture/database_baseline_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/model.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/management_service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/user/service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/role/service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/role/transport/admin/route.go`
- Create: `E:/admin/admin_back_go/internal/module/role/transport/admin/route_test.go`
- Modify: `E:/admin/admin_back_go/internal/admincontract/views.go`
- Modify: `E:/admin/admin_back_go/internal/admincontract/views_test.go`
- Modify: `E:/admin/admin_back_go/internal/admincontract/permissions_test.go`
- Modify: `E:/admin/admin_back_go/internal/server/testdata/admin_route_policy_golden.json`

- [ ] **Step 1: 写失败的数据库和路由权限测试**

在 `database_baseline_test.go` 增加：

```go
func TestDatabaseBaselineRoleManagerPagePermissionContract(t *testing.T) {
	seed, err := os.ReadFile(filepath.Join(backendRoot(t), "database", "seed.sql"))
	if err != nil {
		t.Fatalf("read database/seed.sql: %v", err)
	}
	rows, err := parsePermissionSeedRows(string(seed))
	if err != nil {
		t.Fatalf("parse permissions: %v", err)
	}
	for _, row := range rows {
		if row.id == 13 {
			if row.platform != "admin" || row.typeID != 2 || row.path != "/permission/role" ||
				row.component != "permission/role" || row.code != "permission_role" ||
				row.status != 1 || row.isDel != 2 {
				t.Fatalf("role manager page permission=%+v", row)
			}
			return
		}
	}
	t.Fatal("role manager page permission id=13 is missing")
}
```

在新 `route_test.go` 注册 `adminroute.NewRegistry()`，要求：

```go
	wantPermissions := map[string]string{
	"GET /api/admin/v1/roles/page-init":     "permission_role",
	"GET /api/admin/v1/roles":               "permission_role",
	"POST /api/admin/v1/roles":              "permission_role_add",
	"PUT /api/admin/v1/roles/:id":           "permission_role_edit",
	"PATCH /api/admin/v1/roles/:id/default": "permission_role_setDefault",
	"DELETE /api/admin/v1/roles/:id":        "permission_role_del",
	"DELETE /api/admin/v1/roles":            "permission_role_del",
}
```

测试循环必须显式验证每条定义存在且访问类型为权限：

```go
for route, wantCode := range wantPermissions {
	definition, ok := definitions[route]
	if !ok {
		t.Fatalf("route %s is missing", route)
	}
	if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != wantCode {
		t.Fatalf("route %s access=%#v want permission %q", route, definition.Access, wantCode)
	}
}
```

同时增加迁移保护测试：

```go
func TestRoleManagerPagePermissionMigrationIsGuardedAndForwardOnly(t *testing.T) {
	path := filepath.Join(backendRoot(t), "database", "migrations", "202608140002_set_role_page_code.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read role page permission migration: %v", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(string(body)), " "))
	for _, required := range []string{
		"create temporary table",
		"id = 13",
		"platform = 'admin'",
		"type = 2",
		"path = '/permission/role'",
		"component = 'permission/role'",
		"code = 'permission_role'",
		"update `permissions`",
		"code is null or trim(code) = ''",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("permission migration missing guard %q", required)
		}
	}
	for _, forbidden := range []string{
		"insert into `permissions`",
		"insert into `role_permissions`",
		"update `role_permissions`",
		"delete from `role_permissions`",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("permission migration contains forbidden write %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: 写失败的权限投影和合同测试**

在 `permission/service_test.go` 增加 `TestServiceBuildContextPublishesRoleManagerPageAccessCode`：角色只授权页面 ID 13 时，`RouteAccessCodes` 必须包含 `permission_role`，`ButtonCodes` 必须为空，Router meta code 必须是 `permission_role`。

在 `admincontract/views_test.go` 要求：

```go
View{
	Path:            "/permission/role",
	ViewKey:         "permission/role",
	I18nKey:         "menu.permission_role",
	ShowMenu:        1,
	PermissionCodes: []string{"permission_role"},
}
```

在 `admincontract/permissions_test.go` 要求两个 GET 的 Access 都是 `adminroute.Permission("permission_role")`。

```go
for _, path := range []string{
	"/api/admin/v1/roles/page-init",
	"/api/admin/v1/roles",
} {
	operation, exists := findOperationPolicy(document.Operations, http.MethodGet, path)
	if !exists {
		t.Fatalf("missing GET %s", path)
	}
	if operation.Access.Kind != adminroute.AccessPermission || operation.Access.PermissionCode != "permission_role" {
		t.Fatalf("GET %s access=%#v", path, operation.Access)
	}
}
```

- [ ] **Step 3: 运行 RED 测试**

```powershell
go test ./internal/architecture -run 'TestDatabaseBaselineRoleManagerPagePermissionContract' -count=1
go test ./internal/module/role/transport/admin -run 'TestAdminRoleRoutePermissions' -count=1
go test ./internal/module/permission -run 'TestServiceBuildContextPublishesRoleManagerPageAccessCode' -count=1
go test ./internal/admincontract -run 'TestViewsProtectRoleManagerWithPagePermission|TestRoleManagerReadsUsePagePermission' -count=1
```

Expected：分别因 seed code 为空、GET 仍为 Authenticated、权限投影缺 code、view/operation 未发布页面权限而失败。

- [ ] **Step 4: 创建受保护的 forward migration**

创建 `202608140002_set_role_page_code.sql`：

```sql
CREATE TEMPORARY TABLE `_role_page_code_guard` (
  `value` TINYINT NOT NULL,
  CONSTRAINT `chk_role_page_code_guard` CHECK (`value` = 1)
);

INSERT INTO `_role_page_code_guard` (`value`)
SELECT CASE WHEN
  (SELECT COUNT(*)
   FROM `permissions`
   WHERE id = 13
     AND platform = 'admin'
     AND type = 2
     AND path = '/permission/role'
     AND component = 'permission/role'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'permission_role')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'permission_role' AND id <> 13) = 0
THEN 1 ELSE 0 END;

UPDATE `permissions`
SET code = 'permission_role'
WHERE id = 13
  AND platform = 'admin'
  AND type = 2
  AND path = '/permission/role'
  AND component = 'permission/role'
  AND (code IS NULL OR TRIM(code) = '');

DROP TEMPORARY TABLE `_role_page_code_guard`;
```

迁移不得写 `role_permissions`。既有角色已经通过页面 ID 13 获得页面权限，不需要额外授权写入。

- [ ] **Step 5: 同步初始化 Seed 和 baseline hash**

把 `database/seed.sql` 的页面 ID 13 code 从 `NULL` 改为 `'permission_role'`。只做这一处 Seed 变化；完成后：

```powershell
(Get-FileHash database/seed.sql -Algorithm SHA256).Hash.ToLowerInvariant()
```

Expected：

```text
ce6622f1a754aabd696608526806aea322499dc705c4d0e7e14f938897ddd2f2
```

将 `database/baseline.json` 的 `target.seed_sha256` 精确更新为该值；不得重新生成 schema 或 baseline。

- [ ] **Step 6: 隔离旧 Redis 授权缓存**

将：

```go
RouteAccessCacheKeySchema = "rbac_route_access_grants_v2"
```

改为：

```go
RouteAccessCacheKeySchema = "rbac_route_access_grants_v3"
```

同步以下测试中的精确 key：

```text
internal/module/permission/service_test.go
internal/module/permission/management_service_test.go
internal/module/user/service_test.go
internal/module/role/service_test.go
```

不扫描删除 Redis key，不引入兼容读；新 schema 自然绕开旧缓存。

- [ ] **Step 7: 收紧路由并发布现有 view 事实**

把 Role 两个 GET 改为：

```go
Access: adminroute.Permission("permission_role"),
```

`internal/admincontract/views.go` 的 `permission/role` view 增加：

```go
PermissionCodes: []string{"permission_role"},
```

保持现有四个按钮权限和角色权限矩阵数据结构不变。同步 route policy golden；不得增加新的 `HTTPContract`，只保留这七个既有定义直到 Wave 07。

使用测试自身的显式更新入口生成 golden，然后立即移除环境变量：

```powershell
$env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN = '1'
go test ./internal/server -run 'TestRoutePolicyGoldenIsAdminOnlyAndCurrent' -count=1
Remove-Item Env:UPDATE_ADMIN_ROUTE_POLICY_GOLDEN
go test ./internal/server -run 'TestRoutePolicyGoldenIsAdminOnlyAndCurrent' -count=1
```

第二次不带更新开关仍必须 PASS，证明提交的 golden 是当前路由事实而不是测试时临时写入。

- [ ] **Step 8: 运行 GREEN 短测试并提交**

```powershell
gofmt -w internal/architecture/database_baseline_test.go internal/module/permission/model.go internal/module/permission/service_test.go internal/module/permission/management_service_test.go internal/module/user/service_test.go internal/module/role/service_test.go internal/module/role/transport/admin/route.go internal/module/role/transport/admin/route_test.go internal/admincontract/views.go internal/admincontract/views_test.go internal/admincontract/permissions_test.go
go test ./internal/architecture -run 'TestDatabaseBaselineRoleManagerPagePermissionContract|TestRoleManagerPagePermissionMigrationIsGuardedAndForwardOnly' -count=1
go test ./internal/module/permission ./internal/module/role ./internal/module/role/transport/admin -count=1
go test ./internal/admincontract -run 'TestViewsProtectRoleManagerWithPagePermission|TestRoleManagerReadsUsePagePermission' -count=1
git diff --check
```

Expected：所有定向测试 PASS；现有页面 ID 和角色授权关系不变。

```powershell
git add database/migrations/202608140002_set_role_page_code.sql database/seed.sql database/baseline.json internal/architecture/database_baseline_test.go internal/module/permission/model.go internal/module/permission/service_test.go internal/module/permission/management_service_test.go internal/module/user/service_test.go internal/module/role/service_test.go internal/module/role/transport/admin/route.go internal/module/role/transport/admin/route_test.go internal/admincontract/views.go internal/admincontract/views_test.go internal/admincontract/permissions_test.go internal/server/testdata/admin_route_policy_golden.json
git commit -m "fix(permission): protect role manager page reads"
```

## 4. Task 2：Role 后端使用公共分页并删除死缓存支路

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/role/dto.go`
- Modify: `E:/admin/admin_back_go/internal/module/role/service.go`
- Modify: `E:/admin/admin_back_go/internal/module/role/service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/role/transport/admin/handler_test.go`
- Modify: `E:/admin/admin_back_go/internal/platform/admin/build.go`

- [ ] **Step 1: 写失败的公共分页和 nil 字典测试**

在 `role/service_test.go` 增加：

```go
func TestListResponseUsesSharedPagination(t *testing.T) {
	field, ok := reflect.TypeOf(ListResponse{}).FieldByName("Page")
	if !ok {
		t.Fatal("ListResponse.Page is missing")
	}
	if field.Type != reflect.TypeOf(pagination.Page{}) {
		t.Fatalf("ListResponse.Page type = %v, want pagination.Page", field.Type)
	}
}

func TestPageInitRejectsNilPermissionDictionaryResult(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakePermissionDict{}, nil, nil)
	result, appErr := service.PageInit(context.Background())
	if appErr == nil || result != nil {
		t.Fatalf("PageInit() result=%#v error=%#v", result, appErr)
	}
}
```

调整 `fakePermissionDict` 允许明确返回 `nil, nil`。该状态不是合法空列表，不能继续用 `&InitResponse{}` 掩盖。

- [ ] **Step 2: 运行 RED 测试**

```powershell
go test ./internal/module/role -run 'TestListResponseUsesSharedPagination|TestPageInitRejectsNilPermissionDictionaryResult' -count=1
```

Expected：分页测试因本地 `role.Page` 失败，nil 字典测试因当前返回空对象失败。

- [ ] **Step 3: 替换本地 Page**

在 `dto.go` 引入：

```go
"admin_back_go/internal/shared/pagination"
```

删除 `type Page struct`，把响应改为：

```go
type ListResponse pagination.Result[ListItem]
```

在 `service.go` 使用：

```go
Page: pagination.Page{
	PageSize:    query.PageSize,
	CurrentPage: query.CurrentPage,
	TotalPage:   totalPage(total, query.PageSize),
	Total:       total,
},
```

`ListQuery` 留在 Role；不建立公共查询 DTO，不添加默认分页。

- [ ] **Step 4: 删除没有运行时消费者的 CacheInvalidator**

删除：

```go
type CacheInvalidator interface { ... }
Service.cacheInvalidator
invalidateRoleUsers
Update 末尾的第二次缓存删除
```

把构造器收口为：

```go
func NewService(
	repository Repository,
	permissionDictionary PermissionDictionary,
	platforms []string,
	options ...Option,
) *Service
```

`internal/platform/admin/build.go` 改为：

```go
role.NewService(
	role.NewGormRepository(resources.DB),
	permissionService,
	nil,
	role.WithPrincipalMutations(principalService),
)
```

完成构造器修改后，把 Step 1 的测试调用同步为 `NewService(&fakeRepository{}, &fakePermissionDict{}, nil)`；其他 Role 测试也只删除原第三个 cache 参数，不改变测试业务输入。

更新 Role 测试构造调用，删除 `fakeCacheInvalidator` 和只验证旧 key 删除的测试。必须保留并通过：

```text
TestPrincipalRoleUpdateBumpsEveryBoundUserAcrossServicePlatformsInTransaction
TestPrincipalRoleUpdateDoesNotTouchSQLWhenGateFails
```

这两个测试才是当前真实运行时授权失效边界。

- [ ] **Step 5: nil 字典明确失败并同步 Handler fake**

把 `PageInit` 的 nil 结果处理改为明确内部错误：

```go
if result == nil {
	return nil, apperror.Internal("权限字典返回为空")
}
```

`handler_test.go` 使用 `pagination.Page{}`，继续锁定 data 为 `{list, page}`，不改字段名。

- [ ] **Step 6: 运行后端短测试并提交**

```powershell
gofmt -w internal/module/role/dto.go internal/module/role/service.go internal/module/role/service_test.go internal/module/role/transport/admin/handler_test.go internal/platform/admin/build.go
go test ./internal/shared/pagination ./internal/module/role ./internal/module/role/transport/admin -count=1
go test ./internal/platform/admin -run 'TestBuildProducesCompleteAdminGraph' -count=1
git diff --check
```

Expected：Role 不再定义本地 Page；Role runtime 只有 Principal Mutation 一条授权失效路径。

```powershell
git add internal/module/role/dto.go internal/module/role/service.go internal/module/role/service_test.go internal/module/role/transport/admin/handler_test.go internal/platform/admin/build.go
git commit -m "refactor(role): use shared pagination and principal invalidation"
```

## 5. Task 3：建立直接、严格的前端 Role API

**Files:**

- Modify: `E:/admin/admin_front_ts/src/api/permission/role.ts`
- Create: `E:/admin/admin_front_ts/tests/shared/permission/role-api.test.ts`
- Read-only: `E:/admin/admin_front_ts/src/api/permission/permission.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/system/notificationTask/index.vue`

- [ ] **Step 1: 写失败的直接 API 测试**

测试使用 `installApiClientHarness`，先锁住列表：

```ts
const role = {
  id: 9,
  name: '运营',
  permission_id: [13, 18],
  is_default: 2,
  created_at: '2026-08-14 10:00:00',
  updated_at: '2026-08-14 10:00:00',
}
const page = { current_page: 1, page_size: 50, total_page: 1, total: 1 }

const harness = installApiClientHarness({ list: [role], page })
const response = await RoleApi.list({ current_page: 1, page_size: 50, name: '运营' })
expect(response.list[0]?.name).toBe('运营')
expect(harness.requests.at(-1)).toMatchObject({
  method: 'GET',
  path: '/api/admin/v1/roles',
  query: { current_page: 1, page_size: 50, name: '运营' },
})
```

随后令 fixture 缺少 `page.total_page`、增加未知分页字段、让 `is_default=3`，三种情况都必须 reject。

再覆盖：page-init 非法 platform 明确失败；create/update/default/deleteOne/deleteBatch 的 method、path、body 正确；ID 0 或空批量 ID 在发请求前失败。

- [ ] **Step 2: 运行 RED 测试**

```powershell
npm test -- tests/shared/permission/role-api.test.ts
```

Expected：测试文件尚不存在或当前 Role API 不能满足直接 request/严格 schema 断言。

- [ ] **Step 3: 手写稳定业务类型和严格 schema**

删除：

```ts
import { executeAdminOperation } from '@/lib/http'
import type { components } from '@/modules/http/generated/admin'
import { adminOperations } from '@/modules/http/generated/operations'
```

改为：

```ts
import { z } from 'zod'
import request from '@/utils/request'
import { paginatedSchema, type PaginatedResponse } from '@/utils/pagination'
```

手写稳定类型：

```ts
export interface RoleListItem {
  id: number
  name: string
  permission_id: number[]
  is_default: 1 | 2
  created_at: string
  updated_at: string
}

export type RoleListResponse = PaginatedResponse<RoleListItem>
export interface RoleCreateResponse { id: number }
```

列表 schema 使用：

```ts
const roleListItemSchema: z.ZodType<RoleListItem> = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  permission_id: z.array(z.number().int().positive()),
  is_default: z.union([z.literal(1), z.literal(2)]),
  created_at: z.string(),
  updated_at: z.string(),
}).strict()

const listSchema: z.ZodType<RoleListResponse> = paginatedSchema(roleListItemSchema)
const createSchema: z.ZodType<RoleCreateResponse> = z.object({ id: z.number().int().positive() }).strict()
const emptySchema = z.object({}).strict()
```

page-init 定义递归 raw schema，字段严格对应后端 `permission.PermissionTreeNode`：

```ts
interface RawPermissionTreeNode {
  id: number
  label: string
  value: number
  parent_id: number
  platform: string
  type: number
  code?: string
  children?: RawPermissionTreeNode[]
}

const rawPermissionTreeNodeSchema: z.ZodType<RawPermissionTreeNode> = z.lazy(() => z.object({
  id: z.number().int().positive(),
  label: z.string(),
  value: z.number().int().positive(),
  parent_id: z.number().int().nonnegative(),
  platform: z.string(),
  type: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  code: z.string().optional(),
  children: z.array(rawPermissionTreeNodeSchema).optional(),
}).strict())

const pageInitSchema = z.object({
  dict: z.object({
    permission_tree: z.array(rawPermissionTreeNodeSchema),
    permission_platform_arr: z.array(z.object({
      label: z.string(),
      value: z.string(),
    }).strict()),
  }).strict(),
}).strict()
```

解析后继续调用现有 `parsePermissionTree` 和 `isPermissionPlatform`：

```ts
const pageInit = async (options: ExecuteOptions = {}): Promise<RoleInitResponse> => {
  const response = await request.get(`${basePath}/page-init`, {
    ...optionsFrom(options),
    responseSchema: pageInitSchema,
  })
  const permissionPlatformArr = response.dict.permission_platform_arr.map((option) => {
    if (!isPermissionPlatform(option.value)) {
      throw new Error('role platform dictionary violates the editable contract')
    }
    return { label: option.label, value: option.value }
  })
  return {
    dict: {
      permission_tree: parsePermissionTree(response.dict.permission_tree),
      permission_platform_arr: permissionPlatformArr,
    },
  }
}
```

不修改 `permission.ts`，该模块将在下一项 Permission 迁移中独立去除 generated 依赖。

- [ ] **Step 4: 使用 `src/utils/request.ts` 实现七个操作**

固定：

```ts
const basePath = '/api/admin/v1/roles'
```

实现映射：

```text
pageInit    GET    /roles/page-init
list        GET    /roles
create      POST   /roles
update      PUT    /roles/:id
default     PATCH  /roles/:id/default
deleteOne   DELETE /roles/:id
deleteBatch DELETE /roles body={ids}
```

每个响应传入对应 `responseSchema`。保留 `ExecuteOptions` 的 `signal` 和 `idempotencyKey`，统一用：

```ts
function optionsFrom(options: ExecuteOptions) {
  return { signal: options.signal, idempotencyKey: options.idempotencyKey }
}
```

所有 path/body ID 继续经过 `normalizeRoleIDs`；不加 `?? []`、空对象兜底或双 shape 解包。

核心实现必须保持一份：

```ts
const create = (
  params: RoleAddPayload,
  options: ExecuteOptions = {},
): Promise<RoleCreateResponse> => request.post(basePath, params, {
  ...optionsFrom(options),
  responseSchema: createSchema,
})

const update = async (params: RoleEditPayload, options: ExecuteOptions = {}): Promise<void> => {
  const { id: rawID, ...body } = params
  const [id] = normalizeRoleIDs(rawID)
  await request.put(`${basePath}/${id}`, body, {
    ...optionsFrom(options),
    responseSchema: emptySchema,
  })
}

export const RoleApi = {
  pageInit,
  list: (params: RoleListParams, options: ExecuteOptions = {}): Promise<RoleListResponse> =>
    request.get(basePath, { ...optionsFrom(options), params, responseSchema: listSchema }),
  create,
  update,
  async deleteOne(params: RoleDeleteOnePayload, options: ExecuteOptions = {}): Promise<void> {
    const [id] = normalizeRoleIDs(params.id)
    await request.delete(`${basePath}/${id}`, { ...optionsFrom(options), responseSchema: emptySchema })
  },
  async deleteBatch(params: RoleBatchDeletePayload, options: ExecuteOptions = {}): Promise<void> {
    const ids = normalizeRoleIDs(params.ids)
    await request.delete(basePath, {
      ...optionsFrom(options),
      data: { ids },
      responseSchema: emptySchema,
    })
  },
  async default(params: { id: number }, options: ExecuteOptions = {}): Promise<void> {
    const [id] = normalizeRoleIDs(params.id)
    await request.patch(`${basePath}/${id}/default`, undefined, {
      ...optionsFrom(options),
      responseSchema: emptySchema,
    })
  },
}
```

- [ ] **Step 5: 验证页面和通知任务消费者不需要适配**

```powershell
rg -n 'RoleApi' src/views tests
```

Expected：角色页面和通知任务仍使用同一个 `RoleApi`，返回 `{list,page}`；不建立 `RoleListApi` facade 或第二份 list 实现。

- [ ] **Step 6: 运行前端 API 短测试并提交**

```powershell
npm test -- tests/shared/permission/role-api.test.ts tests/shared/permission/role-matrix.test.ts
npx eslint src/api/permission/role.ts tests/shared/permission/role-api.test.ts
git diff --check
```

```powershell
git add src/api/permission/role.ts tests/shared/permission/role-api.test.ts
git commit -m "refactor(role): use direct frontend api"
```

## 6. Task 4：删除旧级联 helper，保留权限矩阵

**Files:**

- Delete: `E:/admin/admin_front_ts/src/views/Main/permission/role/helpers.ts`
- Delete: `E:/admin/admin_front_ts/tests/shared/permission/role-permission-tree.test.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/role/role-matrix.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/role/components/RolePermissionMatrix.vue`
- Read-only: `E:/admin/admin_front_ts/tests/shared/permission/role-matrix.test.ts`
- Read-only: `E:/admin/admin_front_ts/tests/component/permission/RolePermissionMatrix.test.ts`

- [ ] **Step 1: 证明旧 helper 没有产品消费者**

```powershell
rg -n 'buildLeafSelectablePermissionTree|collectLeafPermissionIds|permission/role/helpers' src tests
```

Expected：只命中 `helpers.ts` 和它自己的 `role-permission-tree.test.ts`。若命中产品代码，停止删除并报告。

- [ ] **Step 2: 删除 helper 和自证测试**

删除这两个文件。不要把函数移动到 `utils`，不要修改当前 matrix 数据结构。

- [ ] **Step 3: 验证现有矩阵语义**

```powershell
npm test -- tests/shared/permission/role-matrix.test.ts tests/component/permission/RolePermissionMatrix.test.ts
npx eslint src/views/Main/permission/role/role-matrix.ts src/views/Main/permission/role/components/RolePermissionMatrix.vue src/views/Main/permission/role/use-role-page.ts src/views/Main/permission/role/index.vue
git diff --check
```

Expected：页面本身可选；勾选按钮自动包含页面；取消页面清除按钮；取消最后一个按钮保留页面；无按钮页面仍可选；目录 ID 不保存。

- [ ] **Step 4: 提交死代码删除**

```powershell
git add src/views/Main/permission/role/helpers.ts tests/shared/permission/role-permission-tree.test.ts
git commit -m "refactor(role): remove retired cascader helpers"
```

## 7. Task 5：同步迁移期合同生成物

**Files:**

- Generated backend: `E:/admin/admin_back_go/contracts/admin/v1/**`
- Generated frontend: `E:/admin/admin_front_ts/contracts/backend/admin/**`
- Generated frontend: `E:/admin/admin_front_ts/src/modules/http/generated/**`
- Generated frontend: `E:/admin/admin_front_ts/src/modules/routing/generated/**`

本 Task 只是保持 Wave 07 前仍存在的消费者可运行。不得新增 route `HTTPContract`，不得扩大生成体系。

- [ ] **Step 1: 生成并检查后端临时 bundle**

```powershell
$out = Join-Path $env:TEMP 'admin-role-contract'
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -Out $out
go test ./internal/admincontract -run 'TestViewsProtectRoleManagerWithPagePermission|TestRoleManagerReadsUsePagePermission|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
```

Expected：Role 两个 GET 权限是 `permission_role`；列表 page 字段不变；view 发布 `permission_role`。若 `admin-dev` 持有原位目录，不停止它，继续使用生成器的 `-Out` 临时目录完成集合比对。

- [ ] **Step 2: 同步后端 bundle 并提交**

只把临时 bundle 中生成器拥有的文件同步到 `contracts/admin/v1`，随后：

```powershell
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git diff --check
git add contracts/admin/v1
git commit -m "chore(contract): publish role management schemas"
```

- [ ] **Step 3: 同步前端合同和 generated 文件**

在前端仓库：

```powershell
$backendCommit = [string](Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'contract manifest has no full backend SHA' }
node scripts/sync-admin-contract.mjs --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

Expected：`permission_role` 进入权限/view 生成物；Role API 产品代码仍不导入 generated operations/types。

- [ ] **Step 4: 运行定向生成物测试并提交**

```powershell
npm test -- tests/unit/http/generated-operations.test.ts tests/unit/routing/contracts.test.ts tests/shared/permission/role-api.test.ts
git diff --check
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated
git commit -m "chore(contract): sync role management schemas"
```

## 8. Task 6：最终短验证、迁移交接和索引更新

**Files:**

- Read-only: `E:/admin/admin_back_go/internal/module/role/**`
- Read-only: `E:/admin/admin_front_ts/src/api/permission/role.ts`
- Modify: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`

- [ ] **Step 1: 运行计划内后端短测试**

```powershell
go test ./internal/shared/pagination ./internal/module/permission ./internal/module/role ./internal/module/role/transport/admin -count=1
go test ./internal/architecture -run 'TestDatabaseBaselineRoleManagerPagePermissionContract|TestRoleManagerPagePermissionMigrationIsGuardedAndForwardOnly' -count=1
go test ./internal/admincontract -run 'TestViewsProtectRoleManagerWithPagePermission|TestRoleManagerReadsUsePagePermission|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
go test ./internal/server -run 'Test.*Role' -count=1
```

- [ ] **Step 2: 运行计划内前端短测试**

```powershell
npm test -- tests/shared/http/notifier.test.ts tests/shared/permission/role-api.test.ts tests/shared/permission/role-matrix.test.ts tests/component/permission/RolePermissionMatrix.test.ts tests/unit/http/generated-operations.test.ts tests/unit/routing/contracts.test.ts
npx eslint src/lib/http/notifier.ts src/api/permission/role.ts src/views/Main/permission/role/use-role-page.ts src/views/Main/permission/role/index.vue src/views/Main/permission/role/role-matrix.ts src/views/Main/permission/role/components/RolePermissionMatrix.vue tests/shared/http/notifier.test.ts tests/shared/permission/role-api.test.ts
```

- [ ] **Step 3: 做减法和兼容性检查**

```powershell
rg -n 'type Page struct' E:/admin/admin_back_go/internal/module/role
rg -n 'CacheInvalidator|invalidateRoleUsers' E:/admin/admin_back_go/internal/module/role
rg -n '@/modules/http/generated|adminOperations|executeAdminOperation' E:/admin/admin_front_ts/src/api/permission/role.ts
rg -n 'buildLeafSelectablePermissionTree|collectLeafPermissionIds|permission/role/helpers' E:/admin/admin_front_ts/src E:/admin/admin_front_ts/tests
rg -n 'RoleApi\.list' E:/admin/admin_front_ts/src/views
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_front_ts diff --check
```

Expected：前三组旧实现无命中；`RoleApi.list` 仍命中角色页面和通知任务；两个仓库无空白错误。

- [ ] **Step 4: 数据库迁移人工门**

不要在 API/Worker 正运行时修改本地数据库。交接时要求用户停止 `admin-dev`，然后执行：

```powershell
pwsh -NoProfile -File scripts/database.ps1 migrate
```

Expected：只应用 `202608140002_set_role_page_code.sql`；页面 ID 13 code 为 `permission_role`，不新增 `permissions` 或 `role_permissions` 行。执行者不得自行启动或重启 `admin-dev`。

- [ ] **Step 5: 更新总索引并停止**

在总索引 Wave 03 Role 段记录：

- notifier 既有独立提交 `9b034cb474115fc5ae6712c5408011631c15657d` 的验证结果；
- 后端/前端最终提交 SHA；
- migration 和合同 manifest SHA；
- 定向测试结果；
- 未运行项；
- 当前状态为“等待用户人工验收”。

提交交接记录：

```powershell
git add docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md
git commit -m "docs(plan): record wave 03 role checkpoint"
```

写完交接后停止，不自动进入 Permission。

人工验收清单：

- [ ] 角色 page-init 和列表正常，搜索、翻页、刷新正常；
- [ ] 新增、编辑、单删、批删正常；
- [ ] 删除后重新创建同名角色恢复原记录且权限正确；
- [ ] 默认角色设置正常，默认角色和已绑定用户角色不能删除；
- [ ] 页面权限可以单独勾选，无按钮页面仍可勾选；
- [ ] 勾选按钮自动勾选所属页面，取消页面清除其按钮；
- [ ] 多平台权限页签、全选本平台、清空本平台和差异确认正常；
- [ ] 没有 `permission_role` 的已登录用户访问两个 Role GET 得到 403，并出现全局错误通知；
- [ ] 通知任务发布弹窗中的角色 RemoteSelect 仍可搜索和选择角色；
- [ ] 刷新和重新登录后角色权限变更立即生效，没有旧 Redis 授权缓存。

明确未运行：`admin-dev`、全量 Go/Vue 测试、全量 typecheck、Playwright、`verify:frontend` 和发布长脚本。

## 9. 完成后的调用链

后端：

```text
transport/admin route
-> Permission middleware
-> handler
-> role.Service
-> role.Repository
-> role.Model / RolePermission
```

授权变更：

```text
role.Service
-> Principal Mutation Coordinator
-> MySQL role/role_permissions + principal version transaction
-> Redis authorization snapshot invalidation
```

前端：

```text
permission/role view
-> useRolePage
-> useCrudTable
-> api/permission/role.ts
-> utils/request.ts
-> /api/admin/v1/roles*
```

`src/modules/http` 和合同生成物仍是未迁移模块的临时依赖；Role 产品 API 已经脱离它们，只有 Wave 07 在全部消费者清零并人工验收后才能物理删除。
