# 平台 RBAC Runtime 与 Admin 管理面 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把授权、角色管理、用户角色绑定和 Admin 管理页面从单一 `users.role_id` 切换到 `user_platform_roles`，并把 Admin 命名的 route policy 核心迁移为两个平台可共享的通用包。

**Architecture:** role/permission repository 的每个读写都携带可信 platform；principal 直接 join `user_platform_roles`，版本与缓存 key 继续使用 `user + platform`。Admin 用户编辑采用完整 `platform_roles` 集合替换并在同一事务 bump 受影响 principal；角色页面先选择平台，后端只接受该平台 permission。`infinite_canvas` 在本 Plan 是可管理平台代码，但尚不加入 compiled runtime registry，直到 Plan 03 trusted adapter 存在。

**Tech Stack:** Go 1.26.5、GORM、Gin route policy、Redis principal cache、Admin Contract Bundle、Vue 3、TypeScript、Vitest。

---

## 执行边界

- 依赖 Plan 01 migrations/HCL 已合并，测试 fixture 必须写 `roles.platform`、`role_permissions.platform` 和 `user_platform_roles`。
- 所有后端路径相对 `E:\admin\admin_back_go`；Admin 前端路径相对 `E:\admin\admin_front_ts`。
- 本 Plan 不新增 Canvas HTTP route、session 或 Cookie；`enum.RegisteredPlatforms()` 在完成态仍只返回 Admin。
- `users.role_id` 不得再出现在生产授权、用户列表、用户更新、角色统计、导出或 principal 查询中；只允许 migration、schema 和证明其无运行时读取的 negative test 命中。
- Admin `users/me` 和 profile 的单个 `role_id/role_name` 仍是 Admin 平台投影，不是全局角色。

## 文件结构

**Rename:**

- `internal/server/adminroute/**` -> `internal/server/routepolicy/**`：保持 API 行为，包名改为 `routepolicy`。

**Create:**

- `internal/module/user/platform_role.go`：用户平台角色 DTO/model 和集合归一化。
- `internal/module/user/platform_role_test.go`：完整替换、重复平台和跨平台 role 验证。
- `internal/architecture/platform_rbac_runtime_test.go`：禁止生产 Go 读取 `users.role_id`，验证 routepolicy 无 Admin 反向依赖。
- `E:\admin\admin_front_ts\tests\shared\permission\platform-role-api.test.ts`：角色 API platform 契约。
- `E:\admin\admin_front_ts\tests\shared\user\platform-role-bindings.test.ts`：用户 binding 契约和 UI 映射。

**Modify:**

- `internal/shared/enum/platform.go`、`platform_test.go`：定义可管理平台代码与 compiled registry 的区别。
- `internal/module/permission/{model,repository,service,principal_model,principal_repository,principal_service}.go` 及测试。
- `internal/module/role/{model,dto,repository,service}.go`、Admin transport 及测试。
- `internal/module/user/{model,dto,repository,service,export_provider}.go`、Admin transport 及测试。
- `internal/platform/admin/{build,build_test}.go`、`internal/server/**`、`internal/admincontract/**`、`cmd/admin-contract/**`：使用通用 routepolicy 和新 DTO。
- `contracts/admin/v1/**`：由正式生成命令更新。
- `E:\admin\admin_front_ts\contracts/backend/admin/v1/**`、lock、generated client：由正式同步命令更新。
- `E:\admin\admin_front_ts\src/api/permission/{permission,role}.ts`、`src/api/user/users.ts`、`src/types/user.ts`。
- `E:\admin\admin_front_ts\src/views/Main/permission/role/**`、`src/views/Main/user/userManager/components/UserList/**`、对应 i18n。

### Task 1: 把 adminroute 机械迁移为通用 routepolicy

**Files:**
- Rename: `internal/server/adminroute/*.go` -> `internal/server/routepolicy/*.go`
- Modify: all Go imports under `cmd`, `internal`
- Test: `internal/server/routepolicy/*_test.go`
- Test: `internal/architecture/platform_rbac_runtime_test.go`

- [ ] **Step 1: 写失败的架构测试**

```go
func TestRoutePolicyCoreIsPlatformNeutral(t *testing.T) {
    root := backendRoot(t)
    if _, err := os.Stat(filepath.Join(root, "internal", "server", "adminroute")); !errors.Is(err, os.ErrNotExist) {
        t.Fatalf("adminroute package must be retired, stat error=%v", err)
    }
    files := goFilesUnder(t, filepath.Join(root, "internal", "server", "routepolicy"))
    for _, path := range files {
        source := readFile(t, path)
        for _, forbidden := range []string{"internal/platform/admin", "module/auth/transport/admin", "PlatformAdmin"} {
            if strings.Contains(source, forbidden) {
                t.Errorf("%s depends on Admin fact %q", path, forbidden)
            }
        }
    }
}
```

- [ ] **Step 2: 运行测试确认旧目录导致失败**

Run: `go test ./internal/architecture -run TestRoutePolicyCoreIsPlatformNeutral -count=1`

Expected: FAIL，指出 `internal/server/adminroute` 仍存在。

- [ ] **Step 3: 执行文件移动和 import/package 机械替换**

逐文件使用 `git mv`，把 package 声明和 import 从：

```go
"admin_back_go/internal/server/adminroute"
```

改为：

```go
"admin_back_go/internal/server/routepolicy"
```

所有 `adminroute.Definition/Registry/NewRegistrar/Public/Authenticated/Permission/Audit/NoAudit/HTTPContract/EmptyData/IDData` 同名迁移到 `routepolicy`；不在这一步改变行为、JSON 字段或 operation ID。

- [ ] **Step 4: 格式化并运行 route policy/server 测试**

```powershell
gofmt -w internal/server/routepolicy internal/server internal/module/permission internal/module/role internal/module/user internal/platform/admin internal/admincontract cmd/admin-contract
go test ./internal/server/routepolicy ./internal/server ./internal/admincontract -count=1
go test ./internal/architecture -run TestRoutePolicyCoreIsPlatformNeutral -count=1
```

Expected: 全部 PASS；`rg -n 'server/adminroute|adminroute\.' cmd internal -g '*.go'` 无输出。

- [ ] **Step 5: 提交纯机械迁移**

```bash
git add internal/server/routepolicy internal/server internal/module internal/platform internal/admincontract cmd internal/architecture/platform_rbac_runtime_test.go
git commit -m "refactor(server): 通用化路由策略核心"
```

### Task 2: 让角色与权限 repository 以 platform 为强制参数

**Files:**
- Modify: `internal/shared/enum/platform.go`
- Modify: `internal/module/role/{model,dto,repository,service}.go`
- Modify: `internal/module/role/transport/admin/{request,handler,route}.go`
- Modify: `internal/module/permission/{model,repository,service}.go`
- Test: `internal/module/role/{repository,service}_test.go`
- Test: `internal/module/permission/{repository,service,management_service}_test.go`

- [ ] **Step 1: 先写同名角色、跨平台 permission 和默认角色测试**

测试必须证明：Admin 和 Canvas 可各有同名 `普通用户`；Canvas role 绑定 Admin permission 返回 `400`；`SetDefault(canvas,id)` 不清除 Admin 默认 role；删除 role 只统计同平台 binding。

```go
func TestServiceRejectsCrossPlatformPermission(t *testing.T) {
    repo := newRoleRepositoryFixture(
        rolePermissionFixture{ID: 11, Platform: enum.PlatformAdmin},
        rolePermissionFixture{ID: 22, Platform: enum.PlatformInfiniteCanvas},
    )
    service := role.NewService(repo, repo, nil, nil)
    _, appErr := service.Create(context.Background(), role.MutationInput{
        Platform: enum.PlatformInfiniteCanvas,
        Name: "无限画布用户",
        PermissionIDs: []int64{11},
    })
    if appErr == nil || appErr.MessageID != "role.permission.platform_mismatch" {
        t.Fatalf("expected platform mismatch, got %#v", appErr)
    }
}
```

- [ ] **Step 2: 添加可管理平台常量，但保持 runtime registry 未激活**

```go
const (
    PlatformAll            = "all"
    PlatformAdmin          = "admin"
    PlatformInfiniteCanvas = "infinite_canvas"
)

var manageablePlatforms = [...]string{PlatformAdmin, PlatformInfiniteCanvas}
var registeredPlatforms = [...]string{PlatformAdmin}

func ManageablePlatforms() []string { return append([]string(nil), manageablePlatforms[:]...) }
func IsManageablePlatform(value string) bool { return containsPlatform(manageablePlatforms[:], value) }
func RegisteredPlatforms() []string { return append([]string(nil), registeredPlatforms[:]...) }
```

测试精确要求 `ManageablePlatforms()==[admin,infinite_canvas]`、`RegisteredPlatforms()==[admin]`、退役 `app/canvas/all` 都不是可管理 product platform。

- [ ] **Step 3: 固定 role DTO 和 repository 签名**

```go
type ListQuery struct {
    CurrentPage int
    PageSize    int
    Platform    string
    Name        string
}

type ListItem struct {
    ID            int64   `json:"id"`
    Platform      string  `json:"platform"`
    Name          string  `json:"name"`
    PermissionIDs []int64 `json:"permission_id"`
    IsDefault     int     `json:"is_default"`
    CreatedAt     string  `json:"created_at"`
    UpdatedAt     string  `json:"updated_at"`
}

type MutationInput struct {
    Platform      string
    Name          string
    PermissionIDs []int64
}
```

repository 接口的 role ID 操作全部增加 platform：

```go
RoleByID(context.Context, string, int64) (*Role, error)
ExistsByName(context.Context, string, string, int64) (bool, error)
FindDeletedByName(context.Context, string, string) (*Role, error)
CountUsersByRoleIDs(context.Context, string, []int64) (int64, error)
PermissionIDsByRoleIDs(context.Context, string, []int64) (map[int64][]int64, error)
AllActivePermissions(context.Context, string) ([]permission.Permission, error)
SyncPermissions(context.Context, string, int64, []int64) error
UserIDsByRoleIDs(context.Context, string, []int64) ([]int64, error)
ClearDefault(context.Context, string) error
SetDefault(context.Context, string, int64) error
```

每条 SQL 同时过滤 `platform`；`SyncPermissions` 写入 `RolePermission.Platform`，并只接受同平台 permission。

- [ ] **Step 4: 更新 Admin role HTTP contract**

- `GET /roles` query 新增必填 `platform`。
- `POST /roles`、`PUT /roles/:id` body 新增必填 `platform`。
- DELETE/default 的 platform 通过 query `platform` 传入，handler 验证 `enum.IsManageablePlatform`。
- `PageInit` 仍返回完整 permission tree 和 platform option，但 role 表单只显示当前 platform subtree。

request 类型固定为：

```go
type mutationRequest struct {
    Platform     string  `json:"platform" binding:"required,max=32"`
    Name         string  `json:"name" binding:"required,max=50"`
    PermissionID []int64 `json:"permission_id" binding:"max=500,dive,min=1"`
}
type platformQuery struct { Platform string `form:"platform" binding:"required,max=32"` }
```

- [ ] **Step 5: 运行 role/permission 测试**

Run: `go test ./internal/module/role ./internal/module/permission -count=1`

Expected: PASS；SQL mock/fixture 断言每个 role/permission 查询均含 platform。

- [ ] **Step 6: 提交平台化角色管理**

```bash
git add internal/shared/enum internal/module/role internal/module/permission
git commit -m "feat(rbac): 按平台管理角色权限"
```

### Task 3: 把 principal snapshot 和版本完全切到 user_platform_roles

**Files:**
- Modify: `internal/module/permission/principal_repository.go`
- Modify: `internal/module/permission/principal_model.go`
- Modify: `internal/module/permission/principal_service.go`
- Modify: `internal/module/permission/principal_*_test.go`
- Modify: `internal/module/auth/session_multinode_integration_test.go`
- Test: `internal/architecture/platform_rbac_runtime_test.go`

- [ ] **Step 1: 写双平台 principal 失败测试**

fixture 给同一 user 绑定 Admin role A 和 Canvas role B，两个 role 分别授权不同 code。断言：

```go
adminSnapshot, _ := repository.LoadSnapshot(ctx, userID, enum.PlatformAdmin)
canvasSnapshot, _ := repository.LoadSnapshot(ctx, userID, enum.PlatformInfiniteCanvas)
require.Equal(t, adminRoleID, adminSnapshot.RoleID)
require.Equal(t, canvasRoleID, canvasSnapshot.RoleID)
require.ElementsMatch(t, []string{"admin_users_read"}, adminSnapshot.RouteCodes)
require.ElementsMatch(t, []string{"infinite_canvas_project_read"}, canvasSnapshot.RouteCodes)
```

再删除 Canvas binding，断言 Canvas 返回 `ErrPrincipalNotFound`，Admin snapshot 不变；bump Canvas version 不能改变 Admin version。

- [ ] **Step 2: 改写 identity 和 permission joins**

`LoadSnapshot` 的唯一 identity query：

```sql
SELECT u.id AS user_id, upr.role_id, upr.platform,
       u.status AS user_status, u.is_del AS user_is_del,
       r.is_del AS role_is_del, COALESCE(v.version, 1) AS version
FROM users AS u
JOIN user_platform_roles AS upr ON upr.user_id = u.id AND upr.platform = ?
JOIN roles AS r ON r.id = upr.role_id AND r.platform = upr.platform
LEFT JOIN authz_principal_versions AS v ON v.user_id = u.id AND v.platform = upr.platform
WHERE u.id = ?
```

permission ID 查询固定为：

```sql
SELECT permission_id FROM role_permissions
WHERE platform = ? AND role_id = ? AND is_del = 2 ORDER BY id ASC
```

active permissions 也过滤相同 platform。不存在 binding 返回 `ErrPrincipalNotFound`；role 软删除返回 inactive snapshot，不回退到 `users.role_id`。

- [ ] **Step 3: 改写 CurrentVersions/AllVersions/Bump**

- `AllVersions` 从 `user_platform_roles` 枚举实际 membership，不再对 registered platform 与所有 users 做笛卡尔积。
- `CurrentVersions` 对每个 subject join 对应 binding，并返回其 role_id。
- `BumpPrincipalVersions` 先按 user id 排序锁 users，再按 `(platform,user_id)` 排序锁 bindings；缺任一 binding 返回 `ErrPrincipalNotFound`。
- 版本表仍以 `(user_id,platform)` upsert，不改变 cache key schema。

- [ ] **Step 4: 更新 multi-node fixture 和 runtime negative scan**

`session_multinode_integration_test.go` 的 role/permission/user fixture 写 platform 列和 binding；角色变更更新 `user_platform_roles.role_id`。架构测试扫描 production `.go`，禁止下列模式：

```text
u.role_id
users.role_id
Update("role_id"
"role_id": normalized.RoleID
Joins("LEFT JOIN roles AS r ON r.id = u.role_id
```

扫描排除 `_test.go`、migration/schema/docs，但测试 fixture 本身也应优先使用新表。

- [ ] **Step 5: 运行 principal 定向测试**

```powershell
go test ./internal/module/permission -run 'Principal|Platform' -count=1
go test ./internal/module/auth -run TestMultiNodePrincipalChangesPropagateWithinTwoSeconds -count=1
go test ./internal/architecture -run TestPlatformRBACRuntime -count=1
```

Expected: PASS；negative scan 无生产命中。

- [ ] **Step 6: 提交 principal 切换**

```bash
git add internal/module/permission internal/module/auth/session_multinode_integration_test.go internal/architecture/platform_rbac_runtime_test.go
git commit -m "feat(rbac): 切换平台 principal 真相源"
```

### Task 4: 让 Admin 用户管理编辑完整平台角色集合

**Files:**
- Create: `internal/module/user/platform_role.go`
- Create: `internal/module/user/platform_role_test.go`
- Modify: `internal/module/user/{model,dto,repository,service,export_provider}.go`
- Modify: `internal/module/user/transport/admin/{request,handler,route}.go`
- Modify: `internal/platform/admin/build.go`
- Test: `internal/module/user/{repository,service}_test.go`

- [ ] **Step 1: 写 Canvas-only 用户和 full replacement 失败测试**

覆盖：列表可以显示只有 Canvas binding 的共享用户；按 `platform=infinite_canvas&role_id=X` 筛选准确；更新 `[admin:A, infinite_canvas:B]` 后两个 principal 都 bump；把 Canvas B 改为 C 只 bump Canvas；重复 platform、未知 platform、role/platform 不匹配返回 `400`；空集合允许把用户变成无平台成员但不删除共享身份。

- [ ] **Step 2: 定义唯一平台 binding DTO**

```go
type PlatformRoleInput struct {
    Platform string `json:"platform"`
    RoleID   int64  `json:"role_id"`
}

type PlatformRoleItem struct {
    Platform string `json:"platform"`
    RoleID   int64  `json:"role_id"`
    RoleName string `json:"role_name"`
}

type RoleOption struct {
    Label    string `json:"label"`
    Value    int64  `json:"value"`
    Platform string `json:"platform"`
}
```

`ListItem` 增加 `PlatformRoles []PlatformRoleItem json:"platform_roles"` 并删除列表层的单一 `role_id/role_name`；`UpdateInput` 用 `PlatformRoles []PlatformRoleInput` 替代 `RoleID`。`ProfileDetail` 和 `InitResponse` 保留 Admin 投影字段。

- [ ] **Step 3: 实现 repository 的批量读取和事务替换**

```go
PlatformRolesByUserIDs(context.Context, []int64) (map[int64][]PlatformRoleItem, error)
ActiveRole(context.Context, string, int64) (*Role, error)
ReplacePlatformRoles(context.Context, int64, []PlatformRoleInput) error
```

列表先分页查 users/profile 摘要，再一次查询所选 user ids 的 bindings，禁止 N+1。替换事务按 platform 排序锁当前 binding，验证目标 role 的 `roles.platform`，删除不再存在的 binding、更新变化项、插入新增项；不得写 `users.role_id`。

- [ ] **Step 4: 在 service 中计算精确 invalidation subjects**

```go
before := normalizePlatformRoles(current)
after := normalizePlatformRoles(input.PlatformRoles)
subjects := changedPlatformRoleSubjects(userID, before, after)
return s.principalMutations.Mutate(ctx, subjects, func() ([]permission.PrincipalVersion, error) {
    if err := tx.ReplacePlatformRoles(ctx, userID, after); err != nil { return nil, err }
    return tx.BumpPrincipalVersions(ctx, subjects)
})
```

删除 binding 后无法再从该表读取 version；因此 mutation 先 bump 旧 subject version，再删除，或在同一事务显式返回 bump 后的版本供 cache invalidator 删除旧 key。测试必须覆盖删除 binding 后旧 cached principal 被移除。

- [ ] **Step 5: 更新 Admin HTTP request/response**

`PUT /users/:id` body 使用：

```go
type updateRequest struct {
    Username      string                      `json:"username" binding:"required,max=50"`
    Avatar        string                      `json:"avatar" binding:"omitempty,max=1024"`
    PlatformRoles []platformRoleBindingRequest `json:"platform_roles" binding:"max=16,dive"`
    Sex           int                         `json:"sex"`
    AddressID     int64                       `json:"address_id"`
    DetailAddress string                      `json:"detail_address" binding:"max=255"`
    Bio           string                      `json:"bio" binding:"max=500"`
}
type platformRoleBindingRequest struct {
    Platform string `json:"platform" binding:"required,max=32"`
    RoleID   int64  `json:"role_id" binding:"required,min=1"`
}
```

`GET /users` query 同时接受 `platform` 和 `role_id`，两者必须一起为空或一起有效；page-init 返回全部带 platform 的 role options。

- [ ] **Step 6: 更新 Admin self projection 和用户导出**

`Init/Profile` 固定读取 `platform='admin'` binding；缺 Admin binding 返回 forbidden，不回退旧列。用户导出的角色列改为稳定字符串 `后台：A；无限画布：B`，顺序按 `enum.ManageablePlatforms()`，Canvas-only 用户导出不丢失。

- [ ] **Step 7: 运行 user 测试并提交**

```powershell
go test ./internal/module/user ./internal/platform/admin -count=1
go test ./internal/architecture -run TestPlatformRBACRuntime -count=1
```

Expected: PASS；production scan 不再读取/写入 `users.role_id`。

```bash
git add internal/module/user internal/platform/admin internal/architecture/platform_rbac_runtime_test.go
git commit -m "feat(user): 管理平台角色绑定"
```

### Task 5: 发布 Admin RBAC contract 并改造角色/用户页面

**Files:**
- Modify: `contracts/admin/v1/**`
- Create: `E:\admin\admin_front_ts\tests\shared\permission\platform-role-api.test.ts`
- Create: `E:\admin\admin_front_ts\tests\shared\user\platform-role-bindings.test.ts`
- Modify: `E:\admin\admin_front_ts\contracts/backend/admin/v1/**`
- Modify: `E:\admin\admin_front_ts\src/modules/http/generated/{admin,operations}.ts`
- Modify: Admin frontend API/types/views/i18n listed above

- [ ] **Step 1: 先生成开发期 Admin Bundle 并确认 schema 包含新字段**

```powershell
$commit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $commit
go test ./internal/admincontract -count=1
```

Expected: exit 0；OpenAPI 的 role mutation 必含 `platform`，user update 必含 `platform_roles`，旧列表 item 不再有全局 `role_id`。

- [ ] **Step 2: 同步前端并写失败契约测试**

在 `E:\admin\admin_front_ts`：

```powershell
npm run contract:sync -- --backend E:/admin/admin_back_go --commit $commit
npm run contract:generate
```

测试断言 role list/create/update/default/delete 全部发送明确 platform；用户筛选成对发送 platform/role_id；update body 逐字发送 `platform_roles`，不生成 `role_id` fallback。

Run: `npm test -- tests/shared/permission/platform-role-api.test.ts tests/shared/user/platform-role-bindings.test.ts`

Expected: FAIL，现有 adapter 仍发送单个 `role_id` 且 role API 没有 platform。

- [ ] **Step 3: 改造角色 API 和页面**

```ts
export interface RoleListParams {
  current_page: number
  page_size: number
  platform: 'admin' | 'infinite_canvas'
  name?: string
}

export interface RoleAddPayload {
  platform: RoleListParams['platform']
  name: string
  permission_id: number[]
}
```

页面 `activePlatform` 改变时重新请求 role list；新增/编辑 dialog 的 platform 固定为当前 tab，编辑期间不能切换；permission matrix 只显示当前 platform；默认角色 switch 只影响当前平台。列表增加“平台”列，所有按钮保持现有 RBAC directive。

- [ ] **Step 4: 改造用户 API 和编辑表单**

```ts
export interface UserPlatformRole {
  platform: 'admin' | 'infinite_canvas'
  role_id: number
  role_name: string
}

export interface UserEditParams {
  id: number
  username: string
  avatar: string
  platform_roles: Array<Pick<UserPlatformRole, 'platform' | 'role_id'>>
  sex: number
  address_id: number
  detail_address: string
  bio: string
}
```

筛选先选平台再显示该平台 roles；编辑 dialog 每个平台使用一个 select，允许清空 binding。列表用紧凑 tag 展示“后台 / 无限画布”角色，不嵌套卡片；Canvas-only 用户仍可编辑、禁用、删除。前端不得从旧 `role_id` 合成 bindings。

- [ ] **Step 5: 更新双语文案并运行前端门禁**

```powershell
npm run locale:generate
npm test -- tests/shared/permission/platform-role-api.test.ts tests/shared/user/platform-role-bindings.test.ts tests/shared/permission/role-matrix.test.ts tests/shared/user/users-api.test.ts
npm run locale:check
npm run typecheck
npm run build
```

Expected: 全部退出 0；两个新测试 PASS，Admin 角色和用户页面不使用 `any` 或手写 fallback DTO。

- [ ] **Step 6: 分仓提交**

后端：

```bash
git add contracts/admin/v1 internal/admincontract
git commit -m "feat(contract): 发布平台 RBAC 管理契约"
```

Admin 前端：

```bash
git add contracts/backend/admin src/modules/http/generated src/api/permission src/api/user src/types/user src/views/Main/permission/role src/views/Main/user/userManager src/i18n tests/shared/permission/platform-role-api.test.ts tests/shared/user/platform-role-bindings.test.ts
git commit -m "feat(rbac): 管理多平台用户角色"
```

### Task 6: 完成 runtime 负向证明和回归门禁

**Files:**
- Verify: all files changed in this Plan

- [ ] **Step 1: 扫描旧授权真相源和旧包名**

```powershell
rg -n 'u\.role_id|users\.role_id|Update\("role_id"|"role_id"\s*:' internal -g '*.go' -g '!**/*_test.go'
rg -n 'server/adminroute|adminroute\.' cmd internal -g '*.go'
```

Expected: 两条命令无输出；`users.role_id` 只存在 schema/migration/docs。

- [ ] **Step 2: 运行后端定向测试**

```powershell
go test ./internal/shared/enum ./internal/module/permission ./internal/module/role ./internal/module/user ./internal/platform/admin ./internal/server/routepolicy ./internal/server ./internal/admincontract -count=1
go test ./internal/architecture -run 'TestPlatformRBACRuntime|TestRoutePolicyCoreIsPlatformNeutral' -count=1
git diff --check
```

Expected: 全部退出 0。

- [ ] **Step 3: 确认两个仓库状态只包含已知事实**

```powershell
git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
git -C E:/admin/canvas_front_next status --short
```

Expected: 后端和 Admin 前端干净；Canvas 仅显示用户原有 `D a`。

## 完成标准

- role、role_permission、principal、用户列表/更新/导出均不再读写 `users.role_id`。
- 同一 user 的 Admin/Canvas snapshot 使用不同 role、permission、version 和 Redis key。
- Admin 用户页面可管理 Canvas-only 用户和完整 platform bindings；角色页面只展示同平台权限。
- Admin 新 prompt 权限仍未自动授权；Canvas HTTP adapter 尚未注册。
- 通用 routepolicy 包不依赖 Admin graph、transport 或平台常量。
