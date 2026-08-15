# Admin Permission + AuthPlatform 权限治理批次设计

**状态：** 已完成方案讨论，等待用户书面复核
**日期：** 2026-08-15
**后端基线：** `ac1fabd9ff2de3bc99bae8a065b8a694d041e2a6`
**前端基线：** `6218e97df48ca2243d9d6768bfd9f9f4ed1f086c`

## 1. 目标

在不改变现有权限树、认证平台策略、API、JSON、数据库字段和用户操作习惯的前提下，一次迁移两个强相关模块：

```text
Permission
AuthPlatform
```

本批次完成后：

- Permission 与 AuthPlatform 的管理 GET 都有真实页面权限；
- 已有页面权限码成为稳定授权事实，普通页面编辑不会清空或改写它；
- Permission 保留唯一的 Principal Mutation 授权失效机制，删除运行时未装配的旧缓存失效支路；
- AuthPlatform 使用后端与前端公共分页；
- 两个前端 API 都直接使用 `src/utils/request.ts` 和严格 Zod schema；
- 两个模块共用一份计划、一次数据库迁移门和一次人工验收，但保持独立代码提交；
- 用户的本地超管账号确认挂载页面权限，并只清理该账号相关的 Redis 授权缓存。

## 2. 已确认决策

### 2.1 批次边界

采用“领域内合并迁移”：Permission 与 AuthPlatform 放在一个计划中执行，不把 Mail、SMS、日志、上传、支付或 AI 混入本批次。

### 2.2 页面权限

现有页面事实不新增、不改 ID：

| 页面 | Permission ID | 当前 code | 目标 code |
|---|---:|---|---|
| 后台菜单管理 | 12 | `NULL` | `permission_permission` |
| 认证平台 | 85 | `NULL` | `permission_authPlatform` |

对应管理 GET 必须使用页面权限：

```text
GET /api/admin/v1/permissions/page-init       -> permission_permission
GET /api/admin/v1/permissions                 -> permission_permission
GET /api/admin/v1/auth-platforms/page-init    -> permission_authPlatform
GET /api/admin/v1/auth-platforms              -> permission_authPlatform
```

现有按钮权限保持不变：

```text
permission_permission_add
permission_permission_edit
permission_permission_del
permission_permission_status

permission_authPlatform_add
permission_authPlatform_edit
permission_authPlatform_del
permission_authPlatform_status
```

页面 `code` 从本批次开始不再被当作“按钮专属展示字段”。已有非空 Page `code` 是路由授权事实，其生命周期规则固定为：

1. 普通页面编辑必须由后端保留数据库中的原 `code`，不能信任隐藏表单提交的空值或旧值；
2. 已有非空 `code` 的 Page 不允许通过通用编辑改成 Dir 或 Button；
3. 普通编辑不能修改、清空或转移已有 Page `code`；
4. 新建普通 Page 仍允许 `code` 为空，本批次不在通用表单中开放页面权限码创建；
5. 规则按“TypePage 且已有非空 code”判断，不在业务代码中硬编码 ID 12、85；
6. 前端可以继续隐藏 Page code 编辑框，但提交值不拥有清空数据库事实的权力。

这不是为 ID 12、85 增加特判，而是修正 Page 数据结构的授权语义。否则管理员在权限页面中编辑名称、排序或图标时，会清空路由仍在使用的权限码，刷新授权快照后把自己锁在页面外。

### 2.3 超管账号与 Redis

用户明确要求 work-ai 在本地执行时确认超管权限挂载并清理授权缓存。

当前本地数据库只读核验事实：

```text
用户：id=1, username=admin, email=admin@qq.com, role_id=2
角色：id=2, name=超管, is_del=2
role_permissions(role_id=2, permission_id=12, is_del=2) 已存在
role_permissions(role_id=2, permission_id=85, is_del=2) 已存在
```

执行时必须重新核验，不能依赖上述快照猜测：

1. `admin/admin@qq.com` 必须唯一命中一个未删除、启用的用户；
2. 读取其真实 `user_id` 和 `role_id`，并确认该 `role_id` 唯一命中一个未删除角色；
3. 报告实际角色名用于证明这是用户所指的超管角色，但业务判断不得依赖角色中文名称；
4. ID 12、85 的 `role_permissions` 已启用时不重复写；
5. 关系缺失或软删除时，只恢复或补齐该角色与这两个页面的关系；
6. 不给其他角色批量授权，不写任何按钮权限；
7. 该本地账号挂载动作是运维验收步骤，不写入通用 forward migration。

页面 code 更新后，旧 Principal 快照可能仍缺少新页面 code。work-ai 必须在数据库事实完成后清理该用户在 Redis DB 0 的精确授权缓存：

```text
<redis-prefix>authz:principal-state:v1:admin:<user_id>
<redis-prefix>authz:principal:v1:admin:<user_id>:*
auth_perm_uid_<user_id>_admin_rbac_route_access_grants_v3（存在时删除）
```

当前本地观测到的 `redis-prefix` 是 `token:`，但执行者必须从当前配置和实际 `SCAN` 结果确认。禁止：

```text
FLUSHDB
FLUSHALL
清空 Redis DB 0
修改 Token Redis DB 2
修改 Realtime Redis DB 1
修改 Queue Redis DB 3
用 KEYS 扫描生产式 keyspace
```

清理前打印精确目标，确认只属于该用户和 `admin` 平台；清理后再次 `SCAN` 证明目标不存在。下一次受保护请求应从 MySQL 重新构建 Principal 快照。

## 3. 当前问题

### 3.1 Permission

- 页面 ID 12 没有页面 code；
- 两个管理 GET 只要求登录；
- `permissionUpdateMap()` 当前会先把所有更新的 `code` 写为空，TypePage 分支不会恢复原值；
- 前端 Page 表单不展示 code，因此普通页面编辑会无声清空新增的路由授权事实；
- 前端 `src/api/permission/permission.ts` 直接依赖 generated operations/types；
- `permission.Service` 同时存在 Principal Mutation 和旧 `CacheInvalidator`；
- 运行时只装配 Principal Mutation，旧 `CacheInvalidator` 只剩测试自证；
- Permission 管理列表是完整树形列表，不是分页数据。

### 3.2 AuthPlatform

- 页面 ID 85 没有页面 code；
- 两个管理 GET 只要求登录；
- 后端重复定义本地 `Page`；
- 前端 API 直接依赖 generated operations/types；
- 认证平台同时被登录、Token、会话和验证码策略读取，不能只按普通展示 CRUD 对待。

## 4. 数据结构设计

### 4.1 数据库迁移

新增一个短小、forward-only、可重复核验的迁移：

```text
202608150001_set_permission_governance_page_codes.sql
```

迁移只允许修改：

```text
permissions.id=12 code
permissions.id=85 code
```

迁移必须通过 ID、platform、type、path、component 和当前 code 联合保护；任一页面不存在、重复、身份不符或目标 code 被其他行占用时，整体失败。允许当前 code 为 `NULL`、空字符串或已经等于目标值，以支持安全重试。

迁移禁止：

```text
INSERT permissions
DELETE permissions
修改 permission ID
写 role_permissions
写 users/roles
修改 auth_platforms
```

同步更新 `database/seed.sql` 和 `database/baseline.json` 的 Seed 哈希，不重建 schema baseline。

### 4.2 Permission 数据结构

保留现有：

```text
Permission
PermissionTreeNode
PermissionListItem
PermissionMutationInput
PrincipalSubject / PrincipalVersion / PrincipalSnapshot
```

Permission 列表继续返回树形数组：

```json
[
  {
    "id": 1,
    "children": []
  }
]
```

不得为了统一而强行套 `{list,page}`。这里没有分页，制造假分页只会增加错误状态。

### 4.3 AuthPlatform 数据结构

删除 `internal/module/auth_platform` 的本地 `Page`，`ListResponse` 使用：

```text
internal/shared/pagination.Result[ListItem]
```

JSON 仍保持：

```json
{
  "list": [],
  "page": {
    "page_size": 20,
    "current_page": 1,
    "total_page": 0,
    "total": 0
  }
}
```

## 5. 后端设计

### 5.1 Permission

最终调用链保持：

```text
transport/admin route
-> middleware permission check
-> handler
-> permission.Service
-> permission.Repository
-> permissions / role_permissions / principal_versions
```

本批次只删除确定的死支路：

```text
CacheInvalidator interface
WithCacheInvalidator
Service.cacheInvalidator
invalidateRoleUsers
只证明旧 key 删除的测试 fake
RouteAccessCacheKey（引用归零后删除）
```

必须保留并加强：

```text
PrincipalMutationCoordinator
角色/用户查询
事务内权限写入与 principal version bump
Redis mutation gate begin/publish/abort
父子节点约束
级联删除约束
软删除按钮 code 恢复
平台隔离
```

Permission Update 必须先读取现有行，并按现有行身份处理 Page code：

```text
existing.Type == TypePage && trim(existing.Code) != ""
-> input.Type 必须仍为 TypePage
existing.Type == TypePage && input.Type == TypePage
-> update fields 不包含 code，数据库原值保持不变
existing.Type != TypePage && input.Type == TypePage
-> update fields 明确清空旧类型遗留的 code
```

不允许从前端请求体覆盖 Page code，也不允许用 `code ?? existing.Code` 一类兜底掩盖来源。最简单的正确实现是：同类型 Page 更新根本不写 `code` 字段；只有 Button 写入请求中的 code，非 Page 转成 Page 时才清掉旧类型遗留值。现有 Dir、Button 和无 code Page 的可见编辑语义保持不变。

Permission 写操作不能在 Redis gate 失败后提交 MySQL，也不能增加“Redis 不可用则直接写库”的降级。

### 5.2 AuthPlatform

保留：

```text
认证平台 code 唯一性
登录方式和验证码类型校验
access/refresh TTL
bind_platform / bind_device / bind_ip
max_sessions
allow_register
默认或在用平台的现有保护规则
登录运行时 Policy 读取
```

只迁移公共分页和路由页面权限，不拆分认证服务，不改登录协议，不改变 Redis Session 行为。

### 5.3 错误语义

- 数据库迁移事实不匹配：立即失败，不猜测新 ID；
- 已有非空 code 的 Page 尝试改类型：返回明确的 400 校验错误，不进入 Principal Mutation；
- Permission Principal Mutation 失败：返回明确依赖错误，不提交一半数据；
- Service 理论必有的响应为空：返回内部错误，不构造空 DTO；
- 无页面权限：返回 403，由现有全局 notifier 展示；
- AuthPlatform 非法策略：保持现有双语言错误 code/msg 语义；
- 不使用空数组、空对象或默认字符串吞掉合同错误。

## 6. 前端设计

### 6.1 API 唯一入口

以下文件改为直接使用 `src/utils/request.ts`：

```text
src/api/permission/permission.ts
src/api/permission/authPlatform.ts
```

两个文件不再导入：

```text
@/lib/http
@/modules/http/generated/admin
@/modules/http/generated/operations
adminOperations
executeAdminOperation
```

### 6.2 严格 schema

Permission API 自己声明并校验：

```text
page-init 字典
递归 permission tree
树形列表
create id
空响应
状态、类型、show_menu、platform 值域
```

AuthPlatform API 自己声明并校验：

```text
page-init 字典
认证平台列表项
公共分页
create id
空响应
login_types、captcha_type、二值字段和 status
```

未知字段、缺少必填字段和非法值域必须抛出合同错误，不做兼容兜底。

### 6.3 页面边界

保留现有页面和真实 helper：

```text
permission/composables/usePermissionDefinitionPage.ts
permission/helpers.ts
PermissionTreeTable.vue
PermissionDefinitionDialog.vue
authPlatform/index.vue
authPlatform/helpers.ts
authPlatform/components/FormDialog.vue
```

这些文件包含真实权限树规则、会话模式与 TTL 表单行为，不因为迁移 API 就删除或改写。只在引用清零且确属纯转发时删除文件。

本批次不做 UI 重设计，不改变按钮位置、表格列、表单字段或用户习惯。

Page code 的稳定性由后端保证。前端不得为了“兼容”在提交前把空 code 猜成当前列表值，也不得新增一个可编辑的 Page code 输入框。这样既不改变现有操作习惯，也避免把授权事实交给隐藏表单状态。

## 7. 迁移期合同

Wave 07 前仍需同步现有生成合同：

```text
permissions.json
views.json
openapi.json
manifest.json
前端镜像、lock 和 generated 文件
```

本批次不新增新的合同抽象，不修改生成器，不物理删除 generated 体系。Permission 与 AuthPlatform 产品 API 迁移后不再消费 generated operations；生成物仅服务尚未迁移的消费者和现有门禁。

## 8. 提交与执行边界

一份实施计划内保持可恢复提交：

```text
1. 接受 Role 人工验收并建立恢复点
2. 页面权限迁移与路由权限
3. Permission 删除死缓存支路
4. Permission 前端 API 迁移
5. AuthPlatform 公共分页与后端迁移
6. AuthPlatform 前端 API 迁移
7. 迁移期合同同步
8. 本地超管权限核验/补齐与精确 Redis 清理
9. 联合短验证与交接
```

代码任务独立提交；本地数据库授权补齐和 Redis 清理由 work-ai 执行并报告，不把本地数据写成 Git 提交。

第 8 步必须由 work-ai 实际执行，不能只给用户 SQL 或 Redis 命令。执行报告至少包含：唯一命中的超管用户与角色、两条页面授权关系的执行前后状态、Redis DB 编号、精确删除的 key 数量，以及清理后重新访问产生的新 Principal 快照所包含的两个页面 code。不得输出 Token、密码或其他敏感值。

## 9. 验证策略

只运行定向短测试：

- Permission Service/Repository/Handler/Route；
- 编辑已有非空 code 的 Page 后 code 保持不变，并拒绝把该 Page 改成 Dir/Button；
- Principal Mutation 的成功、Redis gate 失败和事务回滚；
- AuthPlatform Service/Repository/Handler/Route；
- 数据库 Seed、迁移保护和页面权限；
- Admin route policy、views、permissions 和 OpenAPI 定向合同；
- 前端 Permission/AuthPlatform API 严格 schema；
- Permission tree/helper/component；
- AuthPlatform session policy/helper/page；
- 403 notifier；
- `git diff --check` 和旧引用清零检查。

明确不运行：

```text
admin-dev 启停或重启
go test ./...
全量 Vue 测试
全量 typecheck
Playwright
verify:frontend
发布长脚本
```

## 10. 人工验收

### Permission

- 页面初始化和权限树正常；
- 按平台筛选正常；
- 目录、页面、按钮新增和编辑正常；
- 非法父子关系明确报错；
- 状态切换正常；
- 部分子树删除被拒绝，完整子树删除正常；
- 软删除按钮 code 可以按现有规则恢复；
- Role 权限矩阵能看到两个新增页面 code。

### AuthPlatform

- 列表、搜索、分页正常；
- 新增、编辑、状态切换、单删和批删正常；
- TTL、登录方式、验证码和会话策略显示不变；
- `admin` 认证平台策略仍能正常登录；
- 删除或停用受保护平台仍按现有规则拒绝。

### 权限与缓存

- 唯一启用且未删除的 `admin/admin@qq.com` 超管账号，其真实角色拥有页面 ID 12、85；
- 定向回归测试已证明 User、Role、Permission、AuthPlatform 这类已有非空 code 的 Page 在普通编辑后不会丢失 code；
- Redis DB 0 只清理该账号的 admin Principal 快照；
- 清理后第一次访问重新生成包含 `permission_permission` 和 `permission_authPlatform` 的快照；
- 无页面权限的已登录用户访问四个 GET 得到 403，并出现全局 notifier；
- 401 和 404 继续保持现有静默策略。

## 11. 完成标准

```text
两个页面权限事实已迁移
-> 超管角色关系已核验或最小补齐
-> 精确 Redis 授权缓存已清理
-> Permission 只剩 Principal Mutation 失效机制
-> AuthPlatform 使用公共分页
-> 两个前端 API 脱离 generated operations
-> 定向短测试通过
-> 用户人工验收
-> 才允许进入 Mail + SMS 通信批次
```
