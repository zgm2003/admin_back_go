# Admin 架构减法 Wave 03 User 模块 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to execute this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变 Admin 用户管理 API、数据库、权限码和用户操作习惯的前提下，把 User 核心迁移到公共分页、线性 Service/Repository 链和可读的前端 API/表格调用，并删除已经引用清零的专用 workflow。

**Architecture:** 后端继续以 `internal/module/user` 作为能力边界，`transport/admin` 永久保留为 Admin HTTP 表面；本轮只收紧核心管理入口的合同、权限和 DTO，不移动未迁移的个人资料、会话、登录日志和导出入口。前端由 `src/api/user/user-manager.ts` 持有用户管理 API 的唯一实现，`users.ts` 只做兼容导出；页面直接使用 `useCrudTable`，不再经过 `features/user-management/workflow.ts`。

**Tech Stack:** Go 1.26.5、Gin、GORM、Vue 3.5、TypeScript 5.9、Zod 4、Vitest 4、现有 Admin route registry/OpenAPI 生成链。

---

## 0. 执行边界

本计划只处理以下 Admin 用户管理核心入口：

```text
GET    /api/admin/v1/users/page-init
GET    /api/admin/v1/users
GET    /api/admin/v1/users/:id/profile
PUT    /api/admin/v1/users/:id
PATCH  /api/admin/v1/users/:id/status
PATCH  /api/admin/v1/users
DELETE /api/admin/v1/users/:id
DELETE /api/admin/v1/users
```

必须保持：

- API 路径、HTTP method、查询字段、JSON 字段、`code/data/msg/error` 外层响应和错误语义；
- `internal/module/user/transport/admin` 路径及其与 `profile/transport/admin` 的现有复用；
- 数据库表、字段、索引、seed、菜单和现有按钮权限码；
- `user_userManager_edit`、`user_userManager_batchEdit`、`user_userManager_del`、`user_userManager_export`、`user_userManager_kick` 的既有语义；
- `UsersListApi.list` 仍可被登录日志、操作日志、通知任务和 AI 运行筛选调用。

本计划明确不处理：

- `/users/me`、`/users/export`、用户会话、登录日志；
- `/profile` 个人资料、密码、邮箱、手机修改；
- 地址字典缓存实现；
- 角色、权限、邮件、短信、日志、上传、支付或 AI 模块；
- 数据库迁移、seed、权限新增、权限码重命名或 API 路径重命名；
- `src/lib`、`src/modules` 全局删除；
- `admin-dev`、全量 Go/Vue 测试、Playwright、`verify:frontend`、全量 typecheck 或发布脚本。

如任一目标文件存在计划外未提交修改，先停止并报告，不覆盖其他窗口的工作。

## 1. 文件职责锁定

```text
E:/admin/admin_back_go/internal/module/user/dto.go
  保留 User 业务 DTO 和 ListQuery；删除重复 Page，ListResponse 改用 shared pagination。

E:/admin/admin_back_go/internal/module/user/service.go
  保留分页校验、地址路径组装和业务规则；只替换响应分页类型，不搬查询条件到 shared。

E:/admin/admin_back_go/internal/module/user/transport/admin/route.go
  保留 Admin transport；为核心入口补齐请求/响应合同，并把页面读入口绑定到已有用户管理页面访问权限。

E:/admin/admin_back_go/internal/module/user/transport/admin/handler.go
  只解析 HTTP request、调用 User Service、返回 response；不直接访问 DB/Redis。

E:/admin/admin_back_go/internal/module/user/transport/admin/*_test.go
  锁住请求绑定、分页 JSON、路由权限和未迁移入口仍存在。

E:/admin/admin_front_ts/src/api/user/user-manager.ts
  用户管理核心 API 唯一实现；所有响应使用严格 Zod schema 和 src/utils/pagination。

E:/admin/admin_front_ts/src/api/user/users.ts
  保留认证、个人资料、会话、日志 API；UsersListApi 作为 user-manager 的兼容导出，不再复制实现。

E:/admin/admin_front_ts/src/views/Main/user/userManager/components/UserList/use-user-list.ts
  页面状态、page-init、useCrudTable 和少量用户管理 mutation 的编排，不再创建 workflow。

E:/admin/admin_front_ts/src/features/user-management/workflow.ts
  仅在引用清零并完成人工验收后删除；不建立新的同类 workflow。
```

## 2. Task 1：先锁住后端公共分页和核心权限合同

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/user/dto.go:100-127`
- Modify: `E:/admin/admin_back_go/internal/module/user/service.go:262-295`
- Modify: `E:/admin/admin_back_go/internal/module/user/transport/admin/route.go:24-44`
- Modify: `E:/admin/admin_back_go/internal/module/user/transport/admin/handler_test.go:19-174`
- Create: `E:/admin/admin_back_go/internal/module/user/transport/admin/route_test.go`
- Test: `E:/admin/admin_back_go/internal/module/user/service_test.go`

- [ ] **Step 1: 写失败的公共分页类型测试**

在 `internal/module/user/service_test.go` 增加反射合同测试，要求 `ListResponse.Page` 的类型必须是 `pagination.Page`：

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
```

同时在 `route_test.go` 写路由定义合同：注册到 `adminroute.NewRegistry()` 后，以下三个 GET 必须是页面访问权限，不能继续是纯 `Authenticated()`；写操作必须仍使用现有按钮权限码：

```go
wantReads := map[string]string{
	"GET /api/admin/v1/users/page-init": "user_userManager",
	"GET /api/admin/v1/users": "user_userManager",
	"GET /api/admin/v1/users/:id/profile": "user_userManager",
}
wantWrites := map[string]string{
	"PUT /api/admin/v1/users/:id": "user_userManager_edit",
	"PATCH /api/admin/v1/users/:id/status": "user_userManager_edit",
	"PATCH /api/admin/v1/users": "user_userManager_batchEdit",
	"DELETE /api/admin/v1/users/:id": "user_userManager_del",
	"DELETE /api/admin/v1/users": "user_userManager_del",
}
```

测试必须同时确认 `/api/admin/v1/users/me` 仍注册且保持认证访问，避免把未迁移入口误收进管理页面权限。

- [ ] **Step 2: 运行定向测试确认当前实现失败**

```powershell
go test ./internal/module/user -run 'TestListResponseUsesSharedPagination' -count=1
go test ./internal/module/user/transport/admin -run 'TestAdminUserRoutePermissions' -count=1
```

Expected：分页测试因当前 `user.Page` 重复而失败；权限测试因三个 GET 当前仍为 `Authenticated()` 而失败。不要运行 `go test ./...`。

- [ ] **Step 3: 替换 User 本地 Page，保留 ListQuery**

在 `dto.go` 引入：

```go
"admin_back_go/internal/shared/pagination"
```

删除本地 `type Page struct`，把响应改为：

```go
type ListResponse pagination.Result[ListItem]
```

不要把 `ListQuery` 搬入 shared；筛选字段、日期范围和地址层级是 User 业务事实。

- [ ] **Step 4: 让 Service 构造 shared pagination.Page**

在 `service.go` 引入 `internal/shared/pagination`，把 `List` 的返回字段改为：

```go
return &ListResponse{
	List: list,
	Page: pagination.Page{
		PageSize: normalized.PageSize,
		CurrentPage: normalized.CurrentPage,
		TotalPage: totalPage(total, normalized.PageSize),
		Total: total,
	},
}, nil
```

保留 `normalizeListQuery` 和 User 自己的 `totalPage`；不添加默认页码、空列表兜底或第二个分页结构。

- [ ] **Step 5: 收紧核心读入口权限并保持未迁移入口**

将 `route.go` 中 `page-init`、列表、目标用户 profile 三个 GET 的 `Access` 改为：

```go
adminroute.Permission("user_userManager")
```

保留 `/users/me` 的 `adminroute.Authenticated()`。不修改按钮权限，不新增 seed 或数据库记录。若现有编译权限目录无法解析 `user_userManager`，必须停止并汇报实际目录事实，不能用 `Authenticated()` 偷渡，也不能私自补权限码。

- [ ] **Step 6: 同步 handler fake 并运行后端短测试**

在 `handler_test.go` 使用 `pagination.Page{}`，保留对 `{list, page}` JSON 的断言，并增加一个低权限矩阵测试：权限检查器拒绝 `user_userManager` 时，三个读入口均返回 403，`/users/me` 仍只受认证控制。

```powershell
gofmt -w internal/module/user/dto.go internal/module/user/service.go internal/module/user/transport/admin/route.go internal/module/user/transport/admin/handler_test.go internal/module/user/transport/admin/route_test.go
go test ./internal/shared/pagination ./internal/module/user ./internal/module/user/transport/admin -count=1
go test ./internal/middleware -run 'TestPermissionCheckRejectsWhenCheckerDenies|TestPermissionCheckFailsClosedWithoutAuthIdentity' -count=1
git diff --check
```

Expected：所有定向测试 PASS；列表 data 仍为 `{list: [...], page: {...}}`，错误外层仍为 `code/data/msg/error`。

- [ ] **Step 7: 提交后端核心迁移**

```powershell
git add internal/module/user/dto.go internal/module/user/service.go internal/module/user/transport/admin/route.go internal/module/user/transport/admin/handler_test.go internal/module/user/transport/admin/route_test.go internal/module/user/service_test.go
git commit -m "refactor(user): use shared pagination and page access"
```

## 3. Task 2：补齐 User Admin HTTP 合同并同步后端生成物

**Files:**

- Modify: `E:/admin/admin_back_go/internal/module/user/transport/admin/route.go`
- Modify: `E:/admin/admin_back_go/internal/module/user/transport/admin/request.go`
- Modify: `E:/admin/admin_back_go/internal/module/user/dto.go`
- Test: `E:/admin/admin_back_go/internal/module/user/transport/admin/handler_test.go`
- Generated: `E:/admin/admin_back_go/contracts/admin/v1/**`

- [ ] **Step 1: 为八个核心操作补齐真实 Request/Response contract**

按现有 `systemsetting/route.go` 写法绑定已有 request/response 类型：列表 Query 使用 `listRequest{}`，列表 Response 使用 `ListResponse{}`；page-init、profile、update、status、batch update、delete 使用当前实际 DTO。只声明现有字段，不借合同生成机会增加字段或改名。

- [ ] **Step 2: 写 handler 合同测试**

覆盖：列表成功 data 同时包含 `list` 与完整 `page`；旧 `address` 字段仍被拒绝；缺少必需分页参数返回 400；目标 profile 仍把当前用户 ID 传入 Service。测试必须检查 response `code`、`data`、`msg` 和错误对象的现有形状。

- [ ] **Step 3: 运行后端合同短测试和生成器**

```powershell
go test ./internal/module/user/transport/admin -run 'TestAdminUser|TestHandler' -count=1
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -Out "$env:TEMP\admin-user-contract"
go test ./internal/admincontract -run 'Test.*User.*Route|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
```

Expected：临时 bundle 中八个核心操作的路径、method、权限和字段与 route registry 一致。只有确认 bundle 内容正确后，才把对应 `contracts/admin/v1` 文件原位同步；不停止或重启 `admin-dev`。

- [ ] **Step 4: 提交后端合同**

```powershell
git add internal/module/user/transport/admin/route.go internal/module/user/transport/admin/request.go contracts/admin/v1
git commit -m "chore(contract): publish user management operations"
```

## 4. Task 3：建立唯一 User 管理前端 API

**Files:**

- Create: `E:/admin/admin_front_ts/src/api/user/user-manager.ts`
- Modify: `E:/admin/admin_front_ts/src/api/user/users.ts:1-36,263-312`
- Modify: `E:/admin/admin_front_ts/src/types/user.ts`（只在类型无法从现有合同复用时）
- Create/Modify test: `E:/admin/admin_front_ts/tests/shared/user/user-manager-api.test.ts`

- [ ] **Step 1: 先写严格响应合同测试**

测试使用现有 `installApiClientHarness`，验证：

```ts
await UserManagerApi.list({ current_page: 1, page_size: 20 })
expect(harness.requests.at(-1)).toMatchObject({ method: 'GET', path: '/api/admin/v1/users' })

await expect(UserManagerApi.list({
  current_page: 1,
  page_size: 20,
})).rejects.toThrow()
```

第二次响应 fixture 缺少 `page.total_page` 或带未知分页字段，必须因严格 `paginatedSchema(itemSchema)` 失败；不能回退成 `[]` 或补默认页。

- [ ] **Step 2: 创建 `user-manager.ts` 的唯一实现**

沿 `src/api/system/setting.ts` 的直接 request 风格实现 `pageInit`、`list`、`update`、`batchEdit`、`changeStatus`、`deleteOne`、`deleteBatch`、`export`。列表响应必须使用：

```ts
const listSchema = paginatedSchema(userListItemSchema)
```

请求参数继续保留 User 自己的 `normalizeUsersListParams`、正整数 ID 校验、地址数组拼接和日期范围转换；不要把 User 查询 DTO 放进 `src/utils`。

- [ ] **Step 3: 把旧 `UsersListApi` 改成兼容导出**

在 `users.ts` 删除重复的用户管理实现，改为：

```ts
export { UserManagerApi as UsersListApi } from './user-manager'
```

认证、个人资料、会话和其他用户支持 API 原样保留。`rg` 必须证明所有旧消费者仍只引用兼容名称，没有第二份 list/update/delete 实现。

- [ ] **Step 4: 运行前端 API 短测试并提交**

```powershell
npm test -- tests/shared/user/user-manager-api.test.ts tests/shared/user/users-api.test.ts
npx eslint src/api/user/user-manager.ts src/api/user/users.ts tests/shared/user/user-manager-api.test.ts
git diff --check
git add src/api/user/user-manager.ts src/api/user/users.ts src/types/user.ts tests/shared/user/user-manager-api.test.ts
git commit -m "refactor(user): centralize user management API"
```

## 5. Task 4：把 User 管理页面改为 `useCrudTable` 直连

**Files:**

- Modify: `E:/admin/admin_front_ts/src/views/Main/user/userManager/components/UserList/use-user-list.ts`
- Test: `E:/admin/admin_front_ts/tests/shared/table/useTable.test.ts`

- [ ] **Step 1: 删除页面对 `createUserManagementWorkflow` 的依赖**

在 `use-user-list.ts` 使用：

```ts
import { UserManagerApi } from '@/api/user/user-manager'
import { useCrudTable } from '@/hooks/useCrudTable'
import { createMutation } from '@/modules/resource-query/mutation'
```

以 `useCrudTable<UserListItem, UsersListParams, 1 | 2>` 管理 list、分页、选中项、删除、状态切换；`update`、`batchEdit`、`export` 各保留一个 mutation，invalidate 只指向 `table.resource`。`pageInit` 直接调用 `UserManagerApi.pageInit`，不再为 page-init 建 ResourceQuery。

页面必须继续提供：筛选、翻页、刷新、单删、批删、状态切换、编辑、批量编辑、导出、跳转个人资料和卸载时 dispose。异常继续交给现有全局 API 错误通知，不增加 `catch` 后空数据兜底。

- [ ] **Step 2: 锁住分页和 mutation 行为**

直接运行现有 `useTable` 测试覆盖最新请求覆盖旧请求和最后一页回退；User API 测试覆盖 mutation 请求路径和严格响应合同。不要复制 workflow 测试或再建一层表格抽象。

- [ ] **Step 3: 运行页面相关短测试**

```powershell
npm test -- tests/shared/table/useTable.test.ts tests/shared/user/user-manager-api.test.ts
npx eslint src/views/Main/user/userManager/components/UserList/use-user-list.ts src/views/Main/user/userManager/components/UserList/index.vue
```

Expected：列表仍按 `{list,page}` 渲染，分页回退和错误通知行为不变；不运行 Playwright 或全量 Vue 测试。

- [ ] **Step 4: 提交页面迁移**

```powershell
git add src/views/Main/user/userManager/components/UserList/use-user-list.ts src/views/Main/user/userManager/components/UserList/index.vue tests/shared/table/useTable.test.ts
git commit -m "refactor(user): use crud table in user manager"
```

## 6. Task 5：删除 User 专用 workflow，确认兼容消费者

**Files:**

- Delete after reference check: `E:/admin/admin_front_ts/src/features/user-management/workflow.ts`
- Delete after reference check: `E:/admin/admin_front_ts/tests/integration/features/user-management.test.ts`
- Read-only consumers: `src/views/Main/user/usersLoginLog`, `src/views/Main/system/operationLog`, `src/views/Main/system/notificationTask`, `src/views/Main/ai/runs`

- [ ] **Step 1: 证明引用清零**

```powershell
rg -n 'createUserManagementWorkflow|features/user-management/workflow' src tests
```

Expected：仅命中待删除的 workflow 文件和旧测试；若仍命中任何页面，先迁移该引用，不得删除。

- [ ] **Step 2: 删除重复层和重复测试**

删除两个只服务 User 管理页面的文件。不要修改或删除 `src/features/shared/use-workflow-table.ts`，它仍可能服务其他未迁移页面；不要删除 `src/modules/resource-query`，它仍是迁移期 mutation/query 基础设施。

- [ ] **Step 3: 验证旧消费者仍使用兼容 facade**

```powershell
rg -n 'UsersListApi\.list' src/views
rg -n 'from .*@/api/user/users' src tests
npm test -- tests/shared/user/users-api.test.ts tests/shared/user/user-manager-api.test.ts
npx eslint src/api/user/users.ts src/api/user/user-manager.ts
git diff --check
```

Expected：登录日志、操作日志、通知任务和 AI 运行筛选仍能调用同一 User list 实现；没有第二份 API 实现。

- [ ] **Step 4: 提交删除过渡层**

```powershell
git add src/features/user-management/workflow.ts tests/integration/features/user-management.test.ts
git commit -m "refactor(user): remove redundant management workflow"
```

## 7. Task 6：同步前端合同生成物

**Files:**

- Modify: `E:/admin/admin_front_ts/contracts/backend/admin/**`
- Modify: `E:/admin/admin_front_ts/src/modules/http/generated/**`
- Test: `E:/admin/admin_front_ts/tests/unit/http/generated-operations.test.ts`

- [ ] **Step 1: 使用后端已提交的合同 bundle**

从前端仓库执行：

```powershell
if (git status --short) { throw 'frontend worktree must be clean before contract sync' }
$backendCommit = [string](Get-Content -Raw E:/admin/admin_back_go/contracts/admin/v1/manifest.json | ConvertFrom-Json).backend_commit
if ($backendCommit -notmatch '^[0-9a-f]{40}$') { throw 'contract manifest has no full backend SHA' }
node scripts/sync-admin-contract.mjs --backend E:/admin/admin_back_go --commit $backendCommit
npm run contract:generate
npm run contract:check
```

Expected：User 八个核心操作的 query/body/response schema 与后端 bundle 一致，`Page` 仍为公共分页 JSON，不改变用户 API 字段。

- [ ] **Step 2: 运行生成物定向测试并提交**

```powershell
npm test -- tests/unit/http/generated-operations.test.ts tests/shared/user/user-manager-api.test.ts tests/shared/user/users-api.test.ts
git diff --check
git add contracts/backend/admin src/modules/http/generated src/modules/routing/generated
git commit -m "chore(contract): sync user management schemas"
```

## 8. Task 7：最终短验证、索引更新和人工验收交接

**Files:**

- Read-only: `E:/admin/admin_back_go/internal/module/user/**`
- Read-only: `E:/admin/admin_front_ts/src/api/user/**`
- Modify: `E:/admin/admin_back_go/docs/superpowers/plans/2026-08-13-admin-architecture-reduction-execution-index.md`（只记录本 Wave 恢复点）

- [ ] **Step 1: 运行计划内后端短测试**

```powershell
go test ./internal/shared/pagination ./internal/module/user ./internal/module/user/transport/admin -count=1
go test ./internal/admincontract -run 'Test.*User.*Route|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
go test ./internal/middleware -run 'TestPermissionCheckRejectsWhenCheckerDenies|TestPermissionCheckFailsClosedWithoutAuthIdentity' -count=1
```

- [ ] **Step 2: 运行计划内前端短测试**

```powershell
npm test -- tests/shared/user/user-manager-api.test.ts tests/shared/user/users-api.test.ts tests/shared/table/useTable.test.ts tests/unit/http/generated-operations.test.ts
npx eslint src/api/user/user-manager.ts src/api/user/users.ts src/views/Main/user/userManager/components/UserList/use-user-list.ts src/views/Main/user/userManager/components/UserList/index.vue
```

- [ ] **Step 3: 做范围和兼容性检查**

```powershell
rg -n 'type Page struct' E:/admin/admin_back_go/internal/module/user
rg -n 'createUserManagementWorkflow|features/user-management/workflow' E:/admin/admin_front_ts/src E:/admin/admin_front_ts/tests
rg -n 'UsersListApi\.list' E:/admin/admin_front_ts/src/views
git -C E:/admin/admin_back_go diff --check
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts diff --check
git -C E:/admin/admin_front_ts status --short
```

Expected：User 不再定义本地 Page；User workflow 无引用；旧筛选页面仍引用兼容 `UsersListApi`；两个仓库只保留计划内提交。

- [ ] **Step 4: 更新总索引并停止**

在总索引的 Wave 03 User 段记录：后端/前端最终提交 SHA、合同 manifest SHA、短测试结果和未运行项。交接后停止，不自动进入角色模块。

人工验收清单：

- 用户管理页面可加载 page-init、列表和地址筛选；
- 翻页、最后一页删除回退、刷新和空列表正常；
- 编辑、批量编辑、启停、单删、批删、导出正常；
- 没有 `user_userManager` 页面访问权限的已登录用户访问三个读入口得到 403；`/users/me` 仍可按认证规则访问；
- 登录日志、操作日志、通知任务和 AI 运行筛选中的用户下拉仍有数据；
- 页面错误仍由全局 `ElNotification`/API 错误协议处理，没有被空数组兜底吞掉。

明确未运行：`admin-dev`、全量 Go/Vue 测试、全量 typecheck、Playwright、`verify:frontend` 和发布长脚本。

## 9. 完成后的调用链

后端：

```text
transport/admin route
-> Authenticated + Permission middleware
-> handler
-> user.Service
-> user.Repository
-> user.Model
```

前端：

```text
userManager view
-> useUserList
-> useCrudTable/useTable
-> api/user/user-manager.ts
-> utils/request.ts
-> Admin ApiClient
-> /api/admin/v1/users*
```

`src/lib/http`、`src/modules/http`、`src/features/shared` 在本 Wave 都是迁移期兼容基础设施，不得提前删除；只有 Wave 07 引用清零并人工验收后才统一归档。
