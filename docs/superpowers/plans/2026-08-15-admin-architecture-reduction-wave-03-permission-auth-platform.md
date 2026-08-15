# Admin 架构减法 Wave 03 Permission + AuthPlatform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Permission 与 AuthPlatform 的管理读取建立真实页面权限，修复 Page code 被普通编辑清空的问题，删除 Permission 内部未装配的旧失效支路，并把两个前端 API 迁移到直接、严格的 request 边界。

**Architecture:** 后端保持 `transport/admin route -> middleware -> handler -> service -> repository -> model`；Permission 列表保持树形数组，AuthPlatform 使用 `internal/shared/pagination`。授权写入只走 Principal Mutation，User 仍在使用的路由授权缓存保持不动；前端保持 `view -> api -> utils/request -> backend`，不改变现有 UI、REST 路径或 JSON。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Redis 8、Vue 3.5、TypeScript、Zod、Vitest、PowerShell。

---

## 0. 执行边界

本计划只处理：

```text
Permission
AuthPlatform
两个页面权限 code
迁移期 Admin Contract 同步
本地 admin 超管权限核验与 Redis DB 0 精确缓存失效
```

明确禁止：

```text
进入 Mail / SMS / 日志 / 上传 / 支付 / AI
删除 internal/module/permission/cache.go
删除 RouteAccessCacheKey 或 RouteAccessCacheKeySchema
修改 User 路由授权缓存读写语义
创建假分页包装 Permission 树
改变 AuthPlatform 登录、Token、验证码、会话或 TTL 语义
修改现有 REST 路径、JSON 字段、按钮权限或 UI 布局
启动、停止或重启 admin-dev
go test ./...
全量 Vue 测试或全量 typecheck
Playwright、verify:frontend 或发布长脚本
FLUSHDB、FLUSHALL、KEYS
修改 Redis DB 1、2、3
```

执行者是 work-ai。当前窗口只维护 Spec、Plan 和执行提示词，不实施产品代码。

后端起始基线至少包含：

```text
5486a4a docs(spec): preserve shared route access cache
```

前端起始基线：

```text
6218e97 chore(contract): sync role management schemas
```

若任一目标文件存在计划外未提交修改，立即停止并报告，不覆盖其他窗口工作。所有代码 Task 先 RED、再最小 GREEN、再定向验证和提交。

## 1. 文件职责锁定

```text
E:/admin/admin_back_go/database/migrations/202608150001_set_permission_governance_page_codes.sql
  只迁移 permissions.id=12/85 的页面 code，不写角色关系

E:/admin/admin_back_go/internal/module/permission/service.go
  Permission 业务规则、Page code 生命周期、Principal Mutation 编排

E:/admin/admin_back_go/internal/module/permission/cache.go
  User 仍在使用的路由授权缓存，必须保留

E:/admin/admin_back_go/internal/module/permission/transport/admin/route.go
  Permission Admin HTTP 与访问权限事实

E:/admin/admin_back_go/internal/module/auth_platform/dto.go
  AuthPlatform 管理 DTO，列表使用 shared pagination

E:/admin/admin_back_go/internal/module/auth_platform/service.go
  AuthPlatform 策略与管理 CRUD，不改认证运行时语义

E:/admin/admin_front_ts/src/api/permission/permission.ts
  Permission 唯一前端 API、严格 schema 和树解析

E:/admin/admin_front_ts/src/api/permission/authPlatform.ts
  AuthPlatform 唯一前端 API、严格 schema 和公共分页

E:/admin/admin_back_go/contracts/admin/v1/**
E:/admin/admin_front_ts/contracts/backend/admin/**
E:/admin/admin_front_ts/src/modules/http/generated/**
E:/admin/admin_front_ts/src/modules/routing/generated/**
  Wave 07 前的迁移期生成物，不是两个产品 API 的依赖
```

## 2. Task 0：锁定执行基线

**Files:**

- Read-only: `E:/admin/admin_back_go/AGENTS.md`
- Read-only: `E:/admin/admin_back_go/docs/architecture.md`
- Read-only: `E:/admin/admin_back_go/internal/module/README.md`
- Read-only: `E:/admin/admin_back_go/internal/platform/README.md`
- Read-only: `E:/admin/admin_back_go/docs/superpowers/specs/2026-08-15-admin-permission-governance-batch-design.md`
- Read-only: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`
- Read-only: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-15-admin-architecture-reduction-wave-03-permission-auth-platform.md`

- [ ] **Step 1: 完整读取边界文档**

不得从历史对话猜当前结构。`E:/admin/LONG_TASK_PARALLEL_EXECUTION.md` 若仍不存在，只记录既有文档漂移，不创建替代文件。

- [ ] **Step 2: 验证两个仓库恢复点**

```powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_back_go rev-parse HEAD
git -C E:/admin/admin_front_ts status --short
git -C E:/admin/admin_front_ts rev-parse HEAD
```

Expected：两个工作区均为空；后端 HEAD 包含 `5486a4a`，前端 HEAD 为或后继于 `6218e97`。若 HEAD 已前进，只接受能解释且工作区干净的已提交变更。

- [ ] **Step 3: 运行既有短基线**

后端：

```powershell
go test ./internal/module/permission ./internal/module/permission/transport/admin ./internal/module/auth_platform ./internal/module/auth_platform/transport/admin -count=1
```

前端：

```powershell
npm test -- tests/shared/permission/permission-api.test.ts tests/shared/permission/role-api.test.ts tests/shared/permission/permission-definition-helpers.test.ts tests/component/permission/PermissionTreeTable.test.ts tests/unit/auth-platform/session-policy.test.ts tests/shared/http/notifier.test.ts
```

Expected：当前短基线 PASS。若失败，只报告既有失败，不先修改计划外文件。Task 0 不提交。

## 3. Task 1：修复 Page code 普通编辑清空问题

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/permission/service.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/management_service_test.go`

- [ ] **Step 1: 写 Page code 生命周期失败测试**

在 `management_service_test.go` 增加三个测试。第一个证明同类型 Page 更新不写 `code`：

```go
func TestServiceUpdatePreservesExistingPageCode(t *testing.T) {
	repo := &fakeManagementRepository{perms: []Permission{
		{ID: 2, Name: "权限管理", ParentID: RootParentID, Platform: "admin", Type: TypeDir},
		{ID: 12, Name: "后台菜单管理", ParentID: 2, Platform: "admin", Type: TypePage, Code: "permission_permission"},
	}}
	svc := NewService(repo, []string{"admin"})

	appErr := svc.Update(context.Background(), 12, PermissionMutationInput{
		Platform: "admin", Type: TypePage, Name: "菜单管理", ParentID: 2,
		Path: "/permission/permission", Component: "permission/permission",
		I18nKey: "menu.permission_permission", Sort: 1, ShowMenu: CommonYes,
		Code: "",
	})

	if appErr != nil {
		t.Fatalf("Update() error = %v", appErr)
	}
	if _, exists := repo.updateMap["code"]; exists {
		t.Fatalf("same-type page update must not write code: %#v", repo.updateMap)
	}
}
```

第二个证明已有非空 code 的 Page 不能改类型：

```go
func TestServiceUpdateRejectsChangingCodedPageType(t *testing.T) {
	repo := &fakeManagementRepository{perms: []Permission{
		{ID: 12, Name: "后台菜单管理", ParentID: RootParentID, Platform: "admin", Type: TypePage, Code: "permission_permission"},
	}}
	svc := NewService(repo, []string{"admin"})

	appErr := svc.Update(context.Background(), 12, PermissionMutationInput{
		Platform: "admin", Type: TypeDir, Name: "后台菜单管理", ParentID: RootParentID,
		I18nKey: "menu.permission_permission", Sort: 1, ShowMenu: CommonYes,
	})

	if appErr == nil || appErr.LegacyCode != 100 || appErr.Message != "已绑定页面权限码的页面不能修改类型" {
		t.Fatalf("expected coded page type rejection, got %#v", appErr)
	}
	if repo.updateID != 0 {
		t.Fatalf("rejected type change reached repository: %#v", repo.updateMap)
	}
}
```

第三个锁住 Button 转 Page 时必须清理旧 code：

```go
func TestServiceUpdateClearsButtonCodeWhenConvertingToPage(t *testing.T) {
	repo := &fakeManagementRepository{perms: []Permission{
		{ID: 2, Name: "权限管理", ParentID: RootParentID, Platform: "admin", Type: TypeDir},
		{ID: 14, Name: "新增", ParentID: 2, Platform: "admin", Type: TypeButton, Code: "permission_permission_add"},
	}}
	svc := NewService(repo, []string{"admin"})

	appErr := svc.Update(context.Background(), 14, PermissionMutationInput{
		Platform: "admin", Type: TypePage, Name: "临时页面", ParentID: 2,
		Path: "/permission/temporary", Component: "permission/temporary",
		I18nKey: "menu.permission_temporary", Sort: 1, ShowMenu: CommonYes,
	})

	if appErr != nil {
		t.Fatalf("Update() error = %v", appErr)
	}
	if code, exists := repo.updateMap["code"]; !exists || code != "" {
		t.Fatalf("button-to-page update must clear old code: %#v", repo.updateMap)
	}
}
```

- [ ] **Step 2: 运行测试验证 RED**

```powershell
go test ./internal/module/permission -run 'TestServiceUpdatePreservesExistingPageCode|TestServiceUpdateRejectsChangingCodedPageType|TestServiceUpdateClearsButtonCodeWhenConvertingToPage' -count=1
```

Expected：至少前两个失败；当前实现会给 Page 更新写 `code=nil`，并允许 coded Page 改类型。

- [ ] **Step 3: 最小修复 Update 数据映射**

`Update` 在 `normalizeMutationInput` 后、父子和唯一性查询前增加：

```go
if existing.Type == TypePage && strings.TrimSpace(existing.Code) != "" && input.Type != TypePage {
	return apperror.BadRequest("已绑定页面权限码的页面不能修改类型")
}
```

把调用改为：

```go
return repository.UpdatePermission(ctx, id, permissionUpdateMap(*existing, input))
```

把 `permissionUpdateMap` 改为接收现有行。公共字段不预写 `code`；各类型明确处理：

```go
func permissionUpdateMap(existing Permission, input PermissionMutationInput) map[string]any {
	row := permissionFromMutation(input)
	fields := map[string]any{
		"name": row.Name, "parent_id": row.ParentID, "platform": row.Platform,
		"type": row.Type, "sort": row.Sort,
		"path": "", "icon": "", "component": "", "i18n_key": "", "show_menu": CommonNo,
	}

	switch input.Type {
	case TypeDir:
		fields["icon"] = row.Icon
		fields["i18n_key"] = row.I18nKey
		fields["show_menu"] = row.ShowMenu
		fields["code"] = ""
	case TypePage:
		fields["path"] = row.Path
		fields["icon"] = row.Icon
		fields["component"] = row.Component
		fields["i18n_key"] = row.I18nKey
		fields["show_menu"] = row.ShowMenu
		if existing.Type != TypePage {
			fields["code"] = ""
		}
	case TypeButton:
		fields["code"] = row.Code
	}
	return fields
}
```

不得使用请求中的空 code 回填，也不得硬编码 ID 7、12、13、85。

- [ ] **Step 4: 运行 Permission 短测试并提交**

```powershell
gofmt -w internal/module/permission/service.go internal/module/permission/management_service_test.go
go test ./internal/module/permission -count=1
git diff --check
git add internal/module/permission/service.go internal/module/permission/management_service_test.go
git commit -m "fix(permission): preserve page access codes"
```

Expected：Permission 包 PASS，提交只包含 Page code 生命周期修复。

## 4. Task 2：建立 Permission 与 AuthPlatform 页面权限事实

**Files:**

- Create: `E:/admin/admin_back_go/database/migrations/202608150001_set_permission_governance_page_codes.sql`
- Modify: `E:/admin/admin_back_go/database/seed.sql`
- Modify: `E:/admin/admin_back_go/database/baseline.json`
- Modify: `E:/admin/admin_back_go/internal/architecture/database_baseline_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/transport/admin/route.go`
- Create: `E:/admin/admin_back_go/internal/module/permission/transport/admin/route_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/auth_platform/transport/admin/route.go`
- Create: `E:/admin/admin_back_go/internal/module/auth_platform/transport/admin/route_test.go`
- Modify: `E:/admin/admin_back_go/internal/admincontract/views.go`
- Modify: `E:/admin/admin_back_go/internal/admincontract/views_test.go`
- Modify: `E:/admin/admin_back_go/internal/admincontract/permissions_test.go`
- Modify: `E:/admin/admin_back_go/internal/server/testdata/admin_route_policy_golden.json`

- [ ] **Step 1: 写失败的数据库事实测试**

在 `database_baseline_test.go` 增加表驱动测试，要求 seed 中两行身份完全匹配：

```go
func TestDatabaseBaselinePermissionGovernancePageContracts(t *testing.T) {
	seed, err := os.ReadFile(filepath.Join(backendRoot(t), "database", "seed.sql"))
	if err != nil { t.Fatalf("read database/seed.sql: %v", err) }
	rows, err := parsePermissionSeedRows(string(seed))
	if err != nil { t.Fatalf("parse permissions: %v", err) }

	want := map[int64]struct{ path, component, code string }{
		12: {path: "/permission/permission", component: "permission/permission", code: "permission_permission"},
		85: {path: "/permission/authPlatform", component: "permission/authPlatform", code: "permission_authPlatform"},
	}
	for _, row := range rows {
		expected, ok := want[row.id]
		if !ok { continue }
		if row.platform != "admin" || row.typeID != 2 || row.path != expected.path ||
			row.component != expected.component || row.code != expected.code || row.status != 1 || row.isDel != 2 {
			t.Fatalf("permission governance page id=%d row=%+v", row.id, row)
		}
		delete(want, row.id)
	}
	if len(want) != 0 { t.Fatalf("missing permission governance pages: %#v", want) }
}
```

再增加迁移源码守卫测试，必须检查版本文件包含两个 ID、platform/type/path/component/code 联合保护，并禁止 `INSERT permissions`、任何 `role_permissions` 写入、`users/roles/auth_platforms` 写入。

- [ ] **Step 2: 写失败的路由与发布合同测试**

两个新 `route_test.go` 使用 `adminroute.NewRegistry()`，分别锁住：

```text
GET /api/admin/v1/permissions/page-init    -> permission_permission
GET /api/admin/v1/permissions              -> permission_permission
GET /api/admin/v1/auth-platforms/page-init -> permission_authPlatform
GET /api/admin/v1/auth-platforms           -> permission_authPlatform
```

同一个测试必须继续断言所有现有 POST/PUT/PATCH/DELETE 使用原按钮权限码，不能只测两个 GET。

在 `service_test.go` 增加 `TestServiceBuildContextPublishesPermissionGovernancePageAccessCodes`：角色只授权 Page 12、85 时，`RouteAccessCodes` 精确包含 `permission_authPlatform`、`permission_permission`，`ButtonCodes` 为空，两个 Router meta code 分别保持对应页面 code。

两个新路由测试的函数名固定为 `TestAdminPermissionRoutePermissions` 和 `TestAdminAuthPlatformRoutePermissions`，这样后续短测试命令只会命中本批次新增的路由访问断言。

在 `views_test.go` 增加 `TestViewsProtectPermissionGovernancePagesWithPagePermissions`；在 `permissions_test.go` 增加 `TestPermissionGovernanceReadsUsePagePermissions`，检查上述四个 GET 的 access kind/code。权限 catalog 预期数量从 109 增至 111。

- [ ] **Step 3: 运行测试验证 RED**

```powershell
go test ./internal/architecture -run 'TestDatabaseBaselinePermissionGovernancePageContracts|TestPermissionGovernancePageMigrationIsGuardedAndForwardOnly' -count=1
go test ./internal/module/permission/transport/admin ./internal/module/auth_platform/transport/admin -run 'TestAdmin.*RoutePermissions' -count=1
go test ./internal/module/permission -run 'TestServiceBuildContextPublishesPermissionGovernancePageAccessCodes' -count=1
go test ./internal/admincontract -run 'TestViewsProtectPermissionGovernancePagesWithPagePermissions|TestPermissionGovernanceReadsUsePagePermissions' -count=1
```

Expected：缺少迁移、seed code 和 GET 页面权限导致 RED。

- [ ] **Step 4: 创建严格 forward migration**

新迁移内容固定为：

```sql
CREATE TEMPORARY TABLE `_permission_governance_page_code_guard` (
  `value` TINYINT NOT NULL,
  CONSTRAINT `chk_permission_governance_page_code_guard` CHECK (`value` = 1)
);

INSERT INTO `_permission_governance_page_code_guard` (`value`)
SELECT CASE WHEN
  (SELECT COUNT(*) FROM `permissions`
   WHERE id = 12 AND platform = 'admin' AND type = 2
     AND path = '/permission/permission' AND component = 'permission/permission'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'permission_permission')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'permission_permission' AND id <> 12) = 0
  AND (SELECT COUNT(*) FROM `permissions`
   WHERE id = 85 AND platform = 'admin' AND type = 2
     AND path = '/permission/authPlatform' AND component = 'permission/authPlatform'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'permission_authPlatform')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'permission_authPlatform' AND id <> 85) = 0
THEN 1 ELSE 0 END;

UPDATE `permissions`
SET code = 'permission_permission'
WHERE id = 12 AND platform = 'admin' AND type = 2
  AND path = '/permission/permission' AND component = 'permission/permission'
  AND (code IS NULL OR TRIM(code) = '');

UPDATE `permissions`
SET code = 'permission_authPlatform'
WHERE id = 85 AND platform = 'admin' AND type = 2
  AND path = '/permission/authPlatform' AND component = 'permission/authPlatform'
  AND (code IS NULL OR TRIM(code) = '');

DROP TEMPORARY TABLE `_permission_governance_page_code_guard`;
```

同步 `database/seed.sql` 两行 code。用以下命令计算新 seed SHA-256，并只替换 `database/baseline.json -> target.seed_sha256`：

```powershell
(Get-FileHash database/seed.sql -Algorithm SHA256).Hash.ToLowerInvariant()
```

不得改 schema hash、表计数、seed 行数或恢复元数据。

- [ ] **Step 5: 修改路由和 view 页面权限**

四个 GET 改为：

```go
Access: adminroute.Permission("permission_permission"),
```

或：

```go
Access: adminroute.Permission("permission_authPlatform"),
```

`views.go` 两个页面增加：

```go
PermissionCodes: []string{"permission_permission"}
PermissionCodes: []string{"permission_authPlatform"}
```

更新 `admin_route_policy_golden.json` 的四个 GET；不改变操作数量、方法、路径和 audit。

- [ ] **Step 6: 运行后端短验证并提交**

```powershell
gofmt -w internal/architecture/database_baseline_test.go internal/module/permission/service_test.go internal/module/permission/transport/admin/route.go internal/module/permission/transport/admin/route_test.go internal/module/auth_platform/transport/admin/route.go internal/module/auth_platform/transport/admin/route_test.go internal/admincontract/views.go internal/admincontract/views_test.go internal/admincontract/permissions_test.go
go test ./internal/architecture -run 'TestDatabaseBaselinePermissionGovernancePageContracts|TestPermissionGovernancePageMigrationIsGuardedAndForwardOnly' -count=1
go test ./internal/module/permission/transport/admin ./internal/module/auth_platform/transport/admin -run 'TestAdmin.*RoutePermissions' -count=1
go test ./internal/module/permission -run 'TestServiceBuildContextPublishesPermissionGovernancePageAccessCodes' -count=1
go test ./internal/admincontract -run 'TestViewsProtectPermissionGovernancePagesWithPagePermissions|TestPermissionGovernanceReadsUsePagePermissions' -count=1
git diff --check
git add database/migrations/202608150001_set_permission_governance_page_codes.sql database/seed.sql database/baseline.json internal/architecture/database_baseline_test.go internal/module/permission/service_test.go internal/module/permission/transport/admin internal/module/auth_platform/transport/admin/route.go internal/module/auth_platform/transport/admin/route_test.go internal/admincontract/views.go internal/admincontract/views_test.go internal/admincontract/permissions_test.go internal/server/testdata/admin_route_policy_golden.json
git commit -m "fix(permission): protect governance page reads"
```

Expected：两页页面权限事实和 Page code 运行时投影通过；迁移没有角色关系写入。

## 5. Task 3：删除 Permission 未装配的旧失效支路

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/permission/service.go`
- Modify: `E:/admin/admin_back_go/internal/module/permission/management_service_test.go`
- Read-only: `E:/admin/admin_back_go/internal/module/permission/cache.go`
- Read-only: `E:/admin/admin_back_go/internal/module/user/service.go`

- [ ] **Step 1: 写边界检查并验证当前失败**

```powershell
rg -n 'CacheInvalidator|WithCacheInvalidator|cacheInvalidator|invalidateRoleUsers' internal/module/permission
rg -n 'RedisRouteAccessGrantCache|RouteAccessCacheKey|RouteAccessCacheKeySchema' internal/module/permission internal/module/user
```

Expected：第一组命中旧 Service 支路和测试 fake；第二组证明共享路由缓存仍被 User 使用。

- [ ] **Step 2: 删除且只删除旧 Service 支路**

从 `service.go` 删除：

```text
Service.cacheInvalidator
CacheInvalidator interface
WithCacheInvalidator
invalidateRoleUsers
Create/Update/Delete/ChangeStatus/restore 后的 invalidateRoleUsers 调用
```

`roleIDsByPermissionIDs` 的无工作条件改为：

```go
if len(permissionIDs) == 0 || s.principalMutations == nil {
	return []int64{}, nil
}
```

从 `management_service_test.go` 删除 `fakePermissionCacheInvalidator` 和只断言 `auth_perm_uid_*` 删除的三个旧测试。必须保留并通过 Principal Mutation 成功、Redis gate 失败、事务回滚以及软删除按钮恢复测试。

不得删除或改写：

```text
internal/module/permission/cache.go
RouteAccessCacheKey
RouteAccessCacheKeySchema
TestRouteAccessCacheKey
User Service 的 routeAccessCache Get/Set/Delete
```

- [ ] **Step 3: 运行短测试和引用检查**

```powershell
gofmt -w internal/module/permission/service.go internal/module/permission/management_service_test.go
go test ./internal/module/permission -count=1
go test ./internal/module/user -run 'Test.*RouteAccess|TestPrincipal.*' -count=1
rg -n 'CacheInvalidator|WithCacheInvalidator|cacheInvalidator|invalidateRoleUsers' internal/module/permission
rg -n 'RedisRouteAccessGrantCache|RouteAccessCacheKey|RouteAccessCacheKeySchema' internal/module/permission internal/module/user
```

Expected：第一组无命中；第二组仍有 User 与共享缓存命中；定向测试 PASS。

- [ ] **Step 4: 提交 Permission 减法**

```powershell
git diff --check
git add internal/module/permission/service.go internal/module/permission/management_service_test.go
git commit -m "refactor(permission): remove retired cache invalidator"
```

## 6. Task 4：建立直接、严格的前端 Permission API

**Files:**

- Modify: `E:/admin/admin_front_ts/src/api/permission/permission.ts`
- Modify: `E:/admin/admin_front_ts/tests/shared/permission/permission-api.test.ts`
- Read-only: `E:/admin/admin_front_ts/src/api/permission/role.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/permission/**`

- [ ] **Step 1: 扩充 API 失败测试**

保留现有七个 REST 方法断言，补充完整 Permission Page、Tree 和 List fixture。测试必须覆盖：

```text
page-init 未知字段 -> reject
非法 platform/type/show_menu/status -> reject
list 缺少 code/type_name/children 合同字段 -> reject
create id 非正整数 -> reject
update/delete/status 非正整数 ID -> 在发请求前 reject
DELETE batch 去重且空数组拒绝
Permission 列表仍直接返回树形数组，不接受 {list,page}
```

测试还要调用 `parsePermissionTree`，证明 Role API 继续复用同一份树解析事实。

- [ ] **Step 2: 运行测试验证 RED**

```powershell
npm test -- tests/shared/permission/permission-api.test.ts tests/shared/permission/role-api.test.ts
```

Expected：当前 generated API 不满足新的直接 request、严格 shape 和本地 ID 校验断言。

- [ ] **Step 3: 改为直接 request 与本地类型**

文件顶部只保留这些基础依赖：

```ts
import { z } from 'zod'
import request from '@/utils/request'
import type { ExecuteOptions } from '@/modules/http/client'
import type { DictOption } from '@/types/common'
```

定义稳定本地类型：

```ts
export type PermissionPlatform = 'admin' | 'app' | 'canvas'
export type PermissionType = 1 | 2 | 3
export type PermissionStatus = 1 | 2

export interface PermissionListParams {
  platform: PermissionPlatform
  name?: string
  path?: string
  type?: PermissionType
}

export interface PermissionMutationPayload {
  platform: PermissionPlatform
  type: PermissionType
  name: string
  parent_id: number
  icon: string
  path: string
  component: string
  code: string
  i18n_key: string
  sort: number
  show_menu: 1 | 2
}
```

递归 wire schema 必须逐字段 strict：

```ts
export interface PermissionTreeWireNode {
  id: number
  label: string
  value: number
  parent_id: number
  platform: string
  type: number
  code?: string
  children?: PermissionTreeWireNode[]
}

const permissionTreeWireSchema: z.ZodType<PermissionTreeWireNode> = z.lazy(() => z.object({
  id: z.number().int().positive(),
  label: z.string(),
  value: z.number().int().positive(),
  parent_id: z.number().int().nonnegative(),
  platform: z.union([z.literal('admin'), z.literal('app'), z.literal('canvas')]),
  type: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  code: z.string().optional(),
  children: z.array(permissionTreeWireSchema).optional(),
}).strict())
```

`PermissionListItem` schema 必须包含：

```text
id name path parent_id icon component status type type_name code i18n_key sort show_menu children?
```

数字 ID 为正整数，`parent_id` 非负，`status/show_menu` 仅 1/2，`type` 仅 1/2/3，所有对象 `.strict()`。`pageInitSchema` 严格校验三个字典数组；create 使用 `{id: positive int}`；空响应使用 `z.object({}).strict()`。

保留 `isPermissionPlatform`、`parsePermissionTree` 和递归列表解析，但输入改为本地 wire 类型。不得写 `as never`、`Record<string, any>`、`?? []` 或双 shape 解包。

所有操作直接调用：

```ts
const basePath = '/api/admin/v1/permissions'
request.get(`${basePath}/page-init`, { responseSchema: pageInitSchema })
request.get(basePath, { params, responseSchema: permissionListSchema })
request.post(basePath, body, { responseSchema: createSchema })
request.put(`${basePath}/${id}`, body, { responseSchema: emptySchema })
request.patch(`${basePath}/${id}/status`, { status }, { responseSchema: emptySchema })
request.delete(`${basePath}/${id}`, { responseSchema: emptySchema })
request.delete(basePath, { data: { ids }, responseSchema: emptySchema })
```

`ExecuteOptions` 只投影 `signal` 和 `idempotencyKey`，沿用 Role API 的 `optionsFrom` 形式，不新建公共 helper。

- [ ] **Step 4: 验证无 generated 依赖且页面不变**

```powershell
npm test -- tests/shared/permission/permission-api.test.ts tests/shared/permission/role-api.test.ts tests/shared/permission/permission-definition-helpers.test.ts tests/component/permission/PermissionTreeTable.test.ts
npx eslint src/api/permission/permission.ts tests/shared/permission/permission-api.test.ts src/api/permission/role.ts
rg -n '@/lib/http|@/modules/http/generated|adminOperations|executeAdminOperation' src/api/permission/permission.ts
```

Expected：测试与 ESLint PASS；`rg` 无命中；Permission 和 Role 页面仍消费同一 `PermissionApi`/树解析。

- [ ] **Step 5: 提交前端 Permission API**

```powershell
git diff --check
git add src/api/permission/permission.ts tests/shared/permission/permission-api.test.ts
git commit -m "refactor(permission): use direct frontend api"
```

## 7. Task 5：AuthPlatform 后端使用公共分页

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/auth_platform/dto.go`
- Modify: `E:/admin/admin_back_go/internal/module/auth_platform/service.go`
- Modify: `E:/admin/admin_back_go/internal/module/auth_platform/management_service_test.go`
- Modify: `E:/admin/admin_back_go/internal/module/auth_platform/transport/admin/handler_test.go`

- [ ] **Step 1: 写失败的公共分页类型测试**

在 `management_service_test.go` 增加：

```go
func TestListResponseUsesSharedPagination(t *testing.T) {
	var response ListResponse
	if reflect.TypeOf(response.Page) != reflect.TypeOf(pagination.Page{}) {
		t.Fatalf("auth platform page type=%T want pagination.Page", response.Page)
	}
}
```

导入 `reflect` 和 `admin_back_go/internal/shared/pagination`。同步 handler fake 的期望类型，先让当前本地 `Page` 产生 RED。

- [ ] **Step 2: 运行测试验证 RED**

```powershell
go test ./internal/module/auth_platform -run 'TestListResponseUsesSharedPagination' -count=1
```

Expected：本地 `authplatform.Page` 与 `pagination.Page` 类型不一致。

- [ ] **Step 3: 删除本地 Page 并使用 shared pagination**

`dto.go`：

```go
import (
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/pagination"
)

type ListResponse = pagination.Result[ListItem]
```

删除本地 `type Page struct`。`service.go` 的列表返回改为：

```go
return &ListResponse{
	List: list,
	Page: pagination.Page{
		PageSize: query.PageSize, CurrentPage: query.CurrentPage,
		TotalPage: totalPage(total, query.PageSize), Total: total,
	},
}, nil
```

handler fake 同样构造 `pagination.Page`。保留 `ListQuery` 和 `totalPage()` 在 AuthPlatform，不建立公共查询 DTO 或默认分页。

- [ ] **Step 4: 运行后端短测试并提交**

```powershell
gofmt -w internal/module/auth_platform/dto.go internal/module/auth_platform/service.go internal/module/auth_platform/management_service_test.go internal/module/auth_platform/transport/admin/handler_test.go
go test ./internal/module/auth_platform ./internal/module/auth_platform/transport/admin -count=1
go test ./internal/shared/pagination -run 'TestResultJSONKeepsEmptyListAndCompletePage' -count=1
rg -n 'type Page struct' internal/module/auth_platform
git diff --check
git add internal/module/auth_platform/dto.go internal/module/auth_platform/service.go internal/module/auth_platform/management_service_test.go internal/module/auth_platform/transport/admin/handler_test.go
git commit -m "refactor(authplatform): use shared pagination"
```

Expected：测试 PASS；AuthPlatform 不再定义本地 Page；JSON 仍为 `{list,page}`。

## 8. Task 6：建立直接、严格的前端 AuthPlatform API

**Files:**

- Modify: `E:/admin/admin_front_ts/src/api/permission/authPlatform.ts`
- Create: `E:/admin/admin_front_ts/tests/shared/permission/auth-platform-api.test.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/authPlatform/index.vue`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/authPlatform/helpers.ts`
- Read-only: `E:/admin/admin_front_ts/src/views/Main/permission/authPlatform/components/FormDialog.vue`

- [ ] **Step 1: 写严格 API 失败测试**

测试使用 `installApiClientHarness`，覆盖：

```text
page-init 三组字典和未知字段
list 完整 item + page
缺少 total_page、未知字段、非法 captcha_type/login_type/二值字段/status -> reject
GET 空 status 不发送，数值 status 正常发送
POST/PUT/PATCH/DELETE 方法、路径和 body
单个/批量 ID 的正整数校验、去重和空数组拒绝
create id 非正整数 -> reject
```

完整 list fixture 必须包含现有 `AuthPlatformItem` 的全部字段，不能用 `as never` 绕过。

- [ ] **Step 2: 运行测试验证 RED**

```powershell
npm test -- tests/shared/permission/auth-platform-api.test.ts tests/unit/auth-platform/session-policy.test.ts
```

Expected：新测试文件或当前 generated API 的严格合同断言失败。

- [ ] **Step 3: 改为直接 request 和公共分页**

导入：

```ts
import { z } from 'zod'
import request from '@/utils/request'
import { paginatedSchema, type PaginatedResponse } from '@/utils/pagination'
import type { ExecuteOptions } from '@/modules/http/client'
import type { DictOption, Id } from '@/types/common'
```

本地协议类型固定为：

```ts
export type AuthPlatformLoginType = 'email' | 'phone' | 'password'
export type AuthPlatformCaptchaType = 'slide'
export type AuthPlatformStatus = 1 | 2
export type AuthPlatformYesNo = 1 | 2
```

`AuthPlatformItem` 严格 schema 必须包含：

```text
id code name login_types captcha_type access_ttl refresh_ttl
bind_platform bind_device bind_ip max_sessions allow_register
status status_name created_at updated_at
```

`id` 正整数；TTL 和 `max_sessions` 非负整数；四个 yes/no 和 status 只允许 1/2；login type、captcha type 使用上面的闭集；对象 `.strict()`。列表 schema 必须是：

```ts
const listSchema: z.ZodType<PaginatedResponse<AuthPlatformItem>> = paginatedSchema(authPlatformItemSchema)
```

page-init 三组 `DictOption` 分别严格校验 1/2、三种 login type 和 `slide`。create schema、empty schema 和 ID 归一化与 Permission 同样 fail-fast，不做默认值。

操作固定为：

```ts
const basePath = '/api/admin/v1/auth-platforms'
request.get(`${basePath}/page-init`, { responseSchema: pageInitSchema })
request.get(basePath, { params: normalizeListParams(params), responseSchema: listSchema })
request.post(basePath, body, { responseSchema: createSchema })
request.put(`${basePath}/${id}`, body, { responseSchema: emptySchema })
request.patch(`${basePath}/${id}/status`, { status }, { responseSchema: emptySchema })
request.delete(`${basePath}/${id}`, { responseSchema: emptySchema })
request.delete(basePath, { data: { ids }, responseSchema: emptySchema })
```

不得改 AuthPlatform 页面、表单、TTL 格式化、session mode helper 或字典标签映射。

- [ ] **Step 4: 运行前端短测试和引用检查**

```powershell
npm test -- tests/shared/permission/auth-platform-api.test.ts tests/unit/auth-platform/session-policy.test.ts
npx eslint src/api/permission/authPlatform.ts tests/shared/permission/auth-platform-api.test.ts src/views/Main/permission/authPlatform/index.vue src/views/Main/permission/authPlatform/helpers.ts
rg -n '@/lib/http|@/modules/http/generated|adminOperations|executeAdminOperation' src/api/permission/authPlatform.ts
```

Expected：测试和 ESLint PASS；`rg` 无命中；页面无需改动。

- [ ] **Step 5: 提交前端 AuthPlatform API**

```powershell
git diff --check
git add src/api/permission/authPlatform.ts tests/shared/permission/auth-platform-api.test.ts
git commit -m "refactor(authplatform): use direct frontend api"
```

## 9. Task 7：同步 Wave 07 前迁移期合同

**Files:**

- Generated backend: `E:/admin/admin_back_go/contracts/admin/v1/**`
- Generated frontend: `E:/admin/admin_front_ts/contracts/backend/admin/**`
- Generated frontend: `E:/admin/admin_front_ts/src/modules/http/generated/**`
- Generated frontend: `E:/admin/admin_front_ts/src/modules/routing/generated/**`

不得新增 HTTP Client、生成器抽象或产品 API generated 依赖。

- [ ] **Step 1: 生成临时后端 bundle**

```powershell
$out = Join-Path $env:TEMP 'admin-permission-authplatform-contract'
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -Out $out
go test ./internal/admincontract -run 'TestViewsProtectPermissionGovernancePagesWithPagePermissions|TestPermissionGovernanceReadsUsePagePermissions|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
```

Expected：四个 GET 发布页面权限；AuthPlatform list 的 JSON page 字段不变；catalog 包含两个新页面 code。若 `admin-dev` 锁住原位目录，不停止它，只用 `-Out` 临时目录。

- [ ] **Step 2: 同步并提交后端 bundle**

只同步生成器拥有的文件到 `contracts/admin/v1`。不要手工编辑 JSON，也不要删除 `realtime/` 目录；先比较临时 bundle 与原位目录的相对文件集合，再复制并逐文件比较 SHA-256：

```powershell
$target = (Resolve-Path E:/admin/admin_back_go/contracts/admin/v1).Path
$sourceFiles = @(Get-ChildItem -LiteralPath $out -Recurse -File | Sort-Object FullName)
$targetFiles = @(Get-ChildItem -LiteralPath $target -Recurse -File | Sort-Object FullName)
$sourceRelative = @($sourceFiles | ForEach-Object { [IO.Path]::GetRelativePath($out, $_.FullName) })
$targetRelative = @($targetFiles | ForEach-Object { [IO.Path]::GetRelativePath($target, $_.FullName) })
if (-not [System.Linq.Enumerable]::SequenceEqual([string[]]$sourceRelative, [string[]]$targetRelative)) {
  throw 'temporary contract bundle file set differs from checked-in bundle'
}
foreach ($file in $sourceFiles) {
  $relative = [IO.Path]::GetRelativePath($out, $file.FullName)
  $destination = Join-Path $target $relative
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
  Copy-Item -LiteralPath $file.FullName -Destination $destination -Force
}
foreach ($file in $sourceFiles) {
  $relative = [IO.Path]::GetRelativePath($out, $file.FullName)
  $destination = Join-Path $target $relative
  $sourceHash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
  $targetHash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash
  if ($sourceHash -cne $targetHash) { throw "contract artifact hash mismatch: $relative" }
}
pwsh -NoProfile -File scripts/check-admin-contract.ps1
git diff --check
git add contracts/admin/v1
git commit -m "chore(contract): publish permission governance schemas"
```

- [ ] **Step 3: 同步前端 generated 文件**

在前端仓库执行：

```powershell
$backendCommit = [string](Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'contract manifest has no full backend SHA' }
node scripts/sync-admin-contract.mjs --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

- [ ] **Step 4: 运行定向生成物测试并提交**

```powershell
npm test -- tests/unit/http/generated-operations.test.ts tests/unit/routing/contracts.test.ts tests/shared/permission/permission-api.test.ts tests/shared/permission/auth-platform-api.test.ts tests/shared/permission/role-api.test.ts
git diff --check
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated
git commit -m "chore(contract): sync permission governance schemas"
```

Expected：生成物同步；Permission/AuthPlatform/Role 产品 API 都不重新依赖 generated operations。

## 10. Task 8：执行本地迁移、核验超管授权并精确清理 Redis

**Files:**

- Runtime-only: MySQL `admin.permissions`, `admin.role_permissions`
- Runtime-only: Redis DB 0 的当前 admin 用户授权 key
- Read-only config: `E:/admin/admin_back_go/deploy/docker-first/admin-go.env`

本 Task 必须由 work-ai 实际执行并报告，不提交本地数据，不启动或停止 `admin-dev`。

- [ ] **Step 1: 应用并检查 forward migration**

```powershell
pwsh -NoProfile -File scripts/database.ps1 migrate
pwsh -NoProfile -File scripts/database.ps1 check
```

Expected：`202608150001` 被记录；ID 12/85 code 正确；没有新增 permission 行。

- [ ] **Step 2: 唯一核验 admin 超管身份和页面事实**

把以下 SQL 通过 `admin-state-mysql-1` 的 `/run/secrets/mysql_root_password` 输入 MySQL；不得把密码写进命令行或输出。PowerShell 只把标准输出保存到变量或人工阅读，不打印 `MYSQL_PWD`：

```sql
SELECT COUNT(*) AS identity_count,
       SUM(CASE WHEN u.status = 1 AND u.is_del = 2 THEN 1 ELSE 0 END) AS active_count
FROM users AS u
WHERE u.username = 'admin' AND u.email = 'admin@qq.com';

SELECT u.id, u.username, u.email, u.role_id, u.status, u.is_del,
       r.name AS role_name, r.is_del AS role_is_del
FROM users AS u
JOIN roles AS r ON r.id = u.role_id
WHERE u.username = 'admin' AND u.email = 'admin@qq.com'
  AND u.status = 1 AND u.is_del = 2;

SELECT id, platform, type, path, component, code, status, is_del
FROM permissions
WHERE id IN (12, 85)
ORDER BY id;

SELECT rp.role_id, rp.permission_id, rp.is_del
FROM role_permissions AS rp
JOIN users AS u ON u.role_id = rp.role_id
WHERE u.username = 'admin' AND u.email = 'admin@qq.com'
  AND rp.permission_id IN (12, 85)
ORDER BY rp.permission_id;
```

Expected：唯一用户、启用且未删除；其真实 role 唯一且未删除，报告实际 `role_name`；两页身份/code 正确。不得以中文角色名作为 SQL 判断条件。

当前代码的 Principal cache 使用主 Redis 客户端的 DB 0，prefix 来自 `config.DefaultTokenRedisPrefix`，固定默认值为 `token:`；`admin-go.env.example` 没有 `TOKEN_REDIS_PREFIX` 覆盖项。执行前必须检查当前代码和 `admin-go.env`，若没有明确的已实现覆盖就使用 `token:`，不得自行发明环境变量。该账号的 `role_id` 和用户 ID 只能从上面的查询结果取得。

- [ ] **Step 3: 仅在缺失时最小恢复页面授权关系**

如果 Step 2 两条关系都为 `is_del=2`，不执行写入。如果缺失或软删除，执行以下事务：

```sql
START TRANSACTION;

CREATE TEMPORARY TABLE `_admin_governance_grant_guard` (
  `value` TINYINT NOT NULL,
  CONSTRAINT `chk_admin_governance_grant_guard` CHECK (`value` = 1)
);

SET @admin_user_id := (
  SELECT id FROM users
  WHERE username = 'admin' AND email = 'admin@qq.com' AND status = 1 AND is_del = 2
);
SET @admin_role_id := (SELECT role_id FROM users WHERE id = @admin_user_id);

INSERT INTO `_admin_governance_grant_guard` (`value`)
SELECT CASE WHEN
  (SELECT COUNT(*) FROM users
   WHERE username = 'admin' AND email = 'admin@qq.com' AND status = 1 AND is_del = 2) = 1
  AND (SELECT COUNT(*) FROM roles WHERE id = @admin_role_id AND is_del = 2) = 1
  AND (SELECT COUNT(*) FROM permissions
       WHERE id = 12 AND platform = 'admin' AND type = 2
         AND status = 1 AND is_del = 2 AND code = 'permission_permission') = 1
  AND (SELECT COUNT(*) FROM permissions
       WHERE id = 85 AND platform = 'admin' AND type = 2
         AND status = 1 AND is_del = 2 AND code = 'permission_authPlatform') = 1
THEN 1 ELSE 0 END;

INSERT INTO role_permissions (role_id, permission_id, is_del)
SELECT @admin_role_id, id, 2
FROM permissions
WHERE (id = 12 AND code = 'permission_permission')
   OR (id = 85 AND code = 'permission_authPlatform')
ON DUPLICATE KEY UPDATE is_del = 2;

DROP TEMPORARY TABLE `_admin_governance_grant_guard`;
COMMIT;
```

再次执行 Step 2 的关系查询，必须恰好两行且均为 `is_del=2`。不给其他角色授权，不补按钮权限。

- [ ] **Step 4: 计算并打印精确 Redis DB 0 目标**

按 Step 2 和当前代码得到 prefix（当前默认是 `token:`），从 Step 2 使用真实 `user_id`，构造：

```text
<prefix>authz:principal-state:v1:admin:<user_id>
<prefix>authz:principal:v1:admin:<user_id>:*
auth_perm_uid_<user_id>_admin_rbac_route_access_grants_v3
```

只允许使用 DB 0 的 `SCAN`。下面的 PowerShell 片段会拒绝 prefix 中的 glob 字符，逐个打印并校验目标，不会触碰 DB 1/2/3：

```powershell
$redisPrefix = 'token:'
if ($redisPrefix -match '[*?\[\]]') { throw 'TOKEN_REDIS_PREFIX_CONTAINS_GLOB' }
$userID = [int64]$adminUserID
$stateKey = "${redisPrefix}authz:principal-state:v1:admin:${userID}"
$principalPattern = "${redisPrefix}authz:principal:v1:admin:${userID}:*"
$legacyKey = "auth_perm_uid_${userID}_admin_rbac_route_access_grants_v3"

function Scan-Redis([string]$pattern) {
  $rows = @(docker exec admin-state-redis-1 redis-cli -n 0 --raw --scan --pattern $pattern)
  if ($LASTEXITCODE -ne 0) { throw "REDIS_SCAN_FAILED: $pattern" }
  return @($rows | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) })
}

$stateMatches = @(Scan-Redis $stateKey | Where-Object { $_ -ceq $stateKey })
$principalMatches = @(Scan-Redis $principalPattern | Where-Object {
  [string]$_ -match ('^' + [regex]::Escape("${redisPrefix}authz:principal:v1:admin:${userID}:") + '\d+:\d+$')
})
$legacyMatches = @(Scan-Redis $legacyKey | Where-Object { $_ -ceq $legacyKey })
$redisKeys = @($stateMatches + $principalMatches + $legacyMatches | Sort-Object -Unique)
$redisKeys | ForEach-Object { "TARGET $_" }
```

不得用 `KEYS`，不得扫描或连接 DB 1/2/3。

- [ ] **Step 5: 删除当前用户精确授权 key 并复核**

对 Step 4 得到的 `$redisKeys` 逐个执行单 key `DEL`，不把 pattern 直接传给 `DEL`。删除前打印 key，删除后对三个目标重新 `SCAN`；必须零命中。禁止 `FLUSHDB`、`FLUSHALL` 或通配批量删除其他用户：

```powershell
foreach ($key in $redisKeys) {
  $deleted = @(docker exec admin-state-redis-1 redis-cli -n 0 --raw DEL $key)
  if ($LASTEXITCODE -ne 0 -or $deleted.Count -ne 1 -or [int]$deleted[0] -ne 1) {
    throw "REDIS_DELETE_FAILED: $key"
  }
  "DELETED $key"
}
if (@(Scan-Redis $stateKey | Where-Object { $_ -ceq $stateKey }).Count -ne 0 -or
    @(Scan-Redis $principalPattern | Where-Object { [string]$_ -match ('^' + [regex]::Escape("${redisPrefix}authz:principal:v1:admin:${userID}:") + '\d+:\d+$') }).Count -ne 0 -or
    @(Scan-Redis $legacyKey | Where-Object { $_ -ceq $legacyKey }).Count -ne 0) {
  throw 'REDIS_TARGET_KEYS_REMAIN'
}
```

- [ ] **Step 6: 验证首次受保护请求重建 Principal**

如果现有 `admin-dev` 正在运行，等待用户用当前 admin 会话访问 Permission 或 AuthPlatform 页面；work-ai 不启动服务。随后在 DB 0 只 SCAN：

```text
<prefix>authz:principal:v1:admin:<user_id>:*
```

读取唯一新 snapshot 的 JSON（只在内存中解析，不打印完整 payload），确认其 `route_codes` 包含：

```text
permission_permission
permission_authPlatform
```

不得输出 Token、密码、完整 session 或无关授权内容。若服务未运行或用户尚未发起请求，明确报告“缓存已精确删除，重建证据等待用户请求”，不得伪造成功。

## 11. Task 9：最终短验证、索引更新和交接

**Files:**

- Modify: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`

- [ ] **Step 1: 后端最终短验证**

```powershell
go test ./internal/architecture -run 'TestDatabaseBaselinePermissionGovernancePageContracts|TestPermissionGovernancePageMigrationIsGuardedAndForwardOnly' -count=1
go test ./internal/shared/pagination -run 'TestResultJSONKeepsEmptyListAndCompletePage' -count=1
go test ./internal/module/permission ./internal/module/permission/transport/admin -count=1
go test ./internal/module/auth_platform ./internal/module/auth_platform/transport/admin -count=1
go test ./internal/admincontract -run 'TestViewsProtectPermissionGovernancePagesWithPagePermissions|TestPermissionGovernanceReadsUsePagePermissions|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
go test ./internal/server -run 'Test.*Permission|Test.*AuthPlatform' -count=1
```

- [ ] **Step 2: 前端最终短验证**

```powershell
npm test -- tests/shared/http/notifier.test.ts tests/shared/permission/permission-api.test.ts tests/shared/permission/role-api.test.ts tests/shared/permission/auth-platform-api.test.ts tests/shared/permission/permission-definition-helpers.test.ts tests/component/permission/PermissionTreeTable.test.ts tests/unit/auth-platform/session-policy.test.ts tests/unit/http/generated-operations.test.ts tests/unit/routing/contracts.test.ts
npx eslint src/api/permission/permission.ts src/api/permission/authPlatform.ts src/api/permission/role.ts src/views/Main/permission/permission src/views/Main/permission/authPlatform tests/shared/permission/permission-api.test.ts tests/shared/permission/auth-platform-api.test.ts
```

- [ ] **Step 3: 做减法和兼容性检查**

```powershell
rg -n 'CacheInvalidator|WithCacheInvalidator|cacheInvalidator|invalidateRoleUsers' E:/admin/admin_back_go/internal/module/permission
rg -n 'RedisRouteAccessGrantCache|RouteAccessCacheKey|RouteAccessCacheKeySchema' E:/admin/admin_back_go/internal/module/permission E:/admin/admin_back_go/internal/module/user
rg -n 'type Page struct' E:/admin/admin_back_go/internal/module/auth_platform
rg -n '@/lib/http|@/modules/http/generated|adminOperations|executeAdminOperation' E:/admin/admin_front_ts/src/api/permission/permission.ts E:/admin/admin_front_ts/src/api/permission/authPlatform.ts
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_front_ts diff --check
```

Expected：第一、第三、第四组无命中；第二组仍证明 User 共享路由缓存存在；两个仓库无空白错误。

- [ ] **Step 4: 更新执行总索引并提交**

记录：

```text
后端/前端最终 HEAD
202608150001 migration checksum 和 database check 结果
Admin contract manifest SHA
超管 user_id、实际 role_id/role_name（不含敏感值）
ID 12/85 授权关系执行前后状态
Redis DB 0 精确删除 key 数量和重建状态
全部定向短测试结果
未运行项
状态：等待用户人工验收
```

```powershell
git add docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md
git commit -m "docs(plan): record permission governance checkpoint"
```

写完交接后停止，不进入 Mail + SMS。

## 12. 人工验收清单

### Permission

- [ ] page-init、权限树和平台切换正常；
- [ ] 目录、页面、按钮新增和编辑正常；
- [ ] 编辑 User、Role、Permission、AuthPlatform 页面名称或排序后，页面 code 不丢失；
- [ ] coded Page 改成 Dir/Button 时收到明确错误；
- [ ] 父子约束、状态切换、部分子树删除拒绝和完整子树删除行为不变；
- [ ] 软删除按钮 code 恢复正常；
- [ ] Permission/Role 页面刷新和重新登录后仍可进入。

### AuthPlatform

- [ ] page-init、列表、搜索和分页正常；
- [ ] 新增、编辑、启停、单删和批删正常；
- [ ] TTL、登录方式、验证码和 session policy 显示不变；
- [ ] admin 平台不能停用或删除；
- [ ] admin 登录、Token 刷新和已有会话不受影响。

### 权限与通知

- [ ] `admin/admin@qq.com` 的真实超管角色拥有页面 ID 12、85；
- [ ] Permission 与 AuthPlatform 四个管理 GET 正常；
- [ ] 无页面权限的已登录用户访问四个 GET 得到 403，并出现全局 notifier；
- [ ] 401 和 404 保持现有静默策略；
- [ ] Redis DB 0 只失效当前 admin 用户的授权快照，DB 1/2/3 未改。

明确未运行：`admin-dev` 启停、`go test ./...`、全量 Vue 测试、全量 typecheck、Playwright、`verify:frontend` 和发布长脚本。

## 13. 完成后的调用链

Permission：

```text
transport/admin route
-> Permission middleware
-> handler
-> permission.Service
-> Principal Mutation Coordinator
-> MySQL permission/role_permissions/principal_versions transaction
-> Redis principal snapshot invalidation
```

AuthPlatform：

```text
transport/admin route
-> Permission middleware
-> handler
-> authplatform.Service
-> authplatform.Repository
-> auth_platforms
```

前端：

```text
Permission/AuthPlatform view
-> src/api/permission/*.ts
-> src/utils/request.ts
-> /api/admin/v1/permissions* or /auth-platforms*
```

`src/modules/http`、生成合同和 shared route access cache 仍有真实消费者。本批次只删除已确认的 Permission Service 旧失效支路；只有 Wave 07 在所有消费者清零并人工验收后才能物理删除 generated 体系。
