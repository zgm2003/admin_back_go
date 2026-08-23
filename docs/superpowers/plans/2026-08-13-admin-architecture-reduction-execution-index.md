# Admin 平台架构减法执行总索引

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development to execute each wave. Every wave has its own review and user acceptance gate. Do not execute this index as one giant batch.

**Goal:** 在不破坏现有业务、数据库、接口和用户习惯的前提下，把 Admin 前后端恢复为社区可读的模块化单体和线性调用链。

**Architecture:** 后端固定 `route -> middleware -> handler -> service -> repository -> model`，前端固定 `views -> api -> request -> backend`。MySQL 保存业务事实，Redis 按角色提供缓存/实时/Token/队列能力，COS 保存文件内容；AI 普通对话不再依赖向量数据库。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Redis 8、Asynq、Vue 3.5、TypeScript、Vite、Element Plus、Vitest。

---

## 使用规则

本轮唯一设计来源由一份中心方向书和两份已批准专项设计组成：

```text
E:/admin/admin_back_go/docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md
E:/admin/admin_back_go/docs/superpowers/specs/2026-08-14-ai-module-radical-simplification-design.md
E:/admin/admin_back_go/docs/superpowers/specs/2026-08-14-admin-generated-contract-retirement-design.md
E:/admin/admin_back_go/docs/superpowers/specs/2026-08-23-local-database-external-ownership-design.md
```

数据库外置所有权切换必须同时读取新的数据库 spec 和 plan；执行 Wave 06 必须同时读取 AI 专项设计；执行 Wave 07 必须同时读取合同退役专项设计。任何旧规格、旧计划或当前实现与这些文档冲突时，不得沿用旧方向。中心方向已经确认：

- 原仓逐模块迁移，不新建长期 v2 双轨；
- 保留 API、数据库、菜单、权限码和用户操作习惯；
- 删除前必须有引用盘点和用户批准；个人开发数据库切换不建立仓库备份流程；
- 不跑全量长脚本、Playwright 或 `admin-dev`，由用户人工启动和验收；
- 每波只修改自己的文件，不能顺手修计划外问题；
- 每波完成后先运行短测试，再等用户人工验收；
- 只有验收后才允许删除对应旧实现。
- Wave 06 彻底删除 Context/RAG/Qdrant/Embedding/Rerank/Memory/Context Plan 和钱包 Hold；
- Wave 07 彻底删除 OpenAPI 与生成合同体系，不保留可选产物或兼容 facade。

当前恢复点：

```text
Backend baseline tag: pre-database-baseline-20260813
Database baseline: historical only; superseded by local external ownership
Database cutover design: 2026-08-23-local-database-external-ownership-design.md
Database cutover plan: 2026-08-23-local-database-external-ownership-cutover.md
Backend documentation commits: bf44a11, 912a8db
Wave 02 accepted at: 2026-08-14
Wave 02 backend: 56b76c0
Wave 02 frontend: 3c27ec5
```

## 阶段总览

| 波次 | 内容 | 结果 | 删除边界 |
|---|---|---|---|
| Wave 01 | 权限矩阵 UI + Realtime Redis DB 1 | 页面权限自然可选，实时和 AI 取消信号脱离缓存 DB 0 | 不删除旧架构 |
| Wave 02 | 系统设置 CRUD 样板（已验收） | 第一条可读的后端/前端样板链 | 已删除系统设置旧重复层 |
| Wave 03 | 公共分页、配置、公共响应、RBAC、后台基础模块 | 基础段、User、Role、Permission + AuthPlatform 已完成人工验收；数据库切换已完成，继续 Mail、SMS、日志、UploadConfig | 只删除已迁移模块旧层 |
| 数据库切换 | 个人开发数据库外置所有权 | 已按 2026-08-23 计划执行 | 删除 database/、admin-db、migration/baseline 门禁；仅清理本地治理表，不改业务表 |
| Wave 04 | Worker、任务、Realtime、COS | 后台任务、WebSocket、上传边界收口 | 只删除已迁移 runtime 包装 |
| Wave 05 | 支付与钱包 | 订单、钱包、供应商、回调幂等边界清楚 | 只删除支付旧适配层 |
| Wave 06 | AI 激进减法 | 最近 N 个完整轮次与 COS 历史附件；真实 Usage 扣费允许负余额 | 删除 Context/RAG/Qdrant/Embedding/Rerank/Memory/Context Plan/Hold |
| Wave 07 | 生成合同与旧架构退役 | 日常 CRUD 只依赖 route/DTO/API/短测试 | OpenAPI、generated contract、Kernel、Registry 和旧脚本集中物理删除 |

每个 Wave 结束后，新窗口必须把实际变更、测试结果、未处理问题和下一波入口写入交接记录，等待用户人工验收。不要跨 Wave 自动继续。

## Wave 01

详细执行文件：

```text
docs/superpowers/plans/2026-08-13-admin-architecture-reduction-wave-01.md
```

该波只包含两个互不穿透的变更：

1. 角色权限矩阵中，页面名称本身承担页面访问权限；动作列只展示真实按钮。
2. Realtime 和 AI 取消信号使用独立 Redis DB 1；缓存仍为 DB 0，Token 为 DB 2，队列为 DB 3。

## Wave 02：系统设置样板

状态：已于 2026-08-14 完成人工验收。后端恢复点 `56b76c0`，前端恢复点 `3c27ec5`。本节只保留完成事实，不再回写已经执行的 Wave 02 计划。

开始条件：Wave 01 已被用户验收，当前代码和数据库状态已重新只读盘点。

目标调用链：

```text
后端 systemsetting route
-> middleware.Auth/Permission/OperationLog
-> handler
-> service
-> repository
-> model

前端 system/setting view
-> api/system-setting.ts
-> request
```

必须保留：系统设置缓存、双语言、默认头像、列表返回 `{list: [...]}` 的真实协议、权限码和操作日志。先为该模块写短 Handler/Service/Repository 测试，再迁移文件；不得先删除 generated contract。

## Wave 03：基础模块

基础段详细执行文件：

```text
docs/superpowers/plans/2026-08-14-admin-architecture-reduction-wave-03-foundation.md
```

开始业务模块前先完成一个独立基础任务：

```text
建立 internal/shared/pagination 的 Page / Result[T]
-> 建立 src/utils/pagination.ts 的严格 schema 与类型
-> 只迁移 systemsetting 使用公共分页
-> 在最终目标结构中正式保留 src/enums
-> 明确新 API 使用 src/utils/request.ts
-> 定向测试和人工验收
```

这一步只建立已经确认重复的分页协议，不创建万能响应包，不迁移所有业务模块，不删除 `src/lib` 或 `src/modules`。查询请求继续留在业务模块，因为筛选字段和分页限制并不统一。

### Wave 03 基础段当前恢复点

基础段已经完成并通过定向测试与用户人工验收。系统设置软删除 key 修复属于基础段收口后的必要修复，也已经完成 CRUD 人工验收。

```text
Accepted backend recovery point: 2a34e3eaf477eb177bf374e8319eada8132b0697
Accepted frontend recovery point: 7528becad61783ca37cc7de4c793c30e0e4ed701
Backend branch: master
Frontend branch: master
```

上面是已人工验收的基础段恢复点，不是正在执行中的仓库 HEAD 或工作区状态。后续 User/Role 等任务由各自完成记录更新，执行总索引不得把未验收提交写成新基线。

本基础段实际保留的事实：

- 后端公共分页唯一入口为 `internal/shared/pagination`；
- 前端公共分页唯一入口为 `src/utils/pagination.ts`；
- 新前端请求入口为 `src/utils/request.ts`，`src/lib/http` 仅是迁移期同实例兼容导出；
- `src/enums` 只保留稳定协议值域，页面颜色映射留在业务页面；
- 未迁移用户、角色、权限、邮件、短信、日志、上传、支付和 AI；
- 未删除 `src/lib`、`src/modules`、generated contract 或 runtime HTTP 模块。

### 多平台 transport 规则

用户已经确认后续会真实制作多个产品平台，因此 `transport` 是最终架构，不是临时目录。业务模块共享 `model/service/repository`，平台入口放在同一能力下：

```text
internal/module/{capability}/
├── model.go
├── service.go
├── repository.go
└── transport/
    ├── admin/
    ├── app/
    └── openapi/
```

只为已经存在的真实平台创建对应目录；不复制业务 Service，不建立 `adminuser`、`appauth` 等平台命名业务模块。Wave 03 用户模块计划必须保留 `internal/module/user/transport/admin`，不得把它收回模块根目录。

### Wave 03 当前入口

基础段、User 和 Admin Role 已经完成人工验收。当前进入 Permission + AuthPlatform 权限治理批次，批准的 Spec 与实施计划为：

```text
docs/superpowers/specs/2026-08-15-admin-permission-governance-batch-design.md
docs/superpowers/plans/2026-08-15-admin-architecture-reduction-wave-03-permission-auth-platform.md
```

本批次只合并强相关的 Permission 与 AuthPlatform：建立两个页面权限、修复 Page code 生命周期、删除 Permission 未装配旧失效支路，并迁移两个前端 API。不得进入邮件、短信、日志或上传。已执行的 User/Role 计划保留为历史恢复事实，不得回写。

### Wave 03 User 模块边界

User 迁移只处理 Admin 用户管理页面的核心能力，保持原有 REST 路径、外层响应 `code/data/msg/error`、数据库字段、菜单和按钮权限不变：

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

本轮明确不迁移 `/users/me`、`/users/export`、用户会话、登录日志、个人资料安全修改、地址字典缓存实现；这些入口继续由现有能力提供，直到各自 Wave。`internal/module/user/transport/admin` 是长期 Admin 平台入口，不能因为本轮减法删除或收回模块根目录。

前端 User 管理页面只保留一份 API 事实源：核心实现放在 `src/api/user/user-manager.ts`，`src/api/user/users.ts` 仅提供兼容导出，供登录日志、操作日志、通知任务和 AI 运行筛选继续使用。页面移除专用 `features/user-management/workflow.ts`，直接使用现有 `useCrudTable`/`useTable` 和模块 API；只有引用清零后才删除该 workflow 及其测试。

按一个模块一个提交迁移：用户、角色、权限、邮件、短信、日志、上传配置。每个模块都执行：

```text
记录 API/字段/权限
-> 新结构实现
-> 受影响 Go/Vue 测试
-> 用户验收
-> 删除该模块旧重复层
```

公共响应、错误通知和 `permission.ts` 只保留一份事实来源；后端低权限接口必须有 403 矩阵测试。

`src/enums` 只保留跨模块稳定值域；后端字典、本地化标签和页面展示映射不得搬入公共枚举。每个基础模块迁移时，同时把自己的重复分页结构切换到公共协议。

### Wave 03 User 恢复点（2026-08-14）

代码迁移已完成，并已通过用户人工验收。当前停在 User 模块，不得自动进入 Role；Role 必须另写计划、另设验收门。

后端提交：

```text
2780fe7 fix(permission): restore user manager page access fact
a2bf32b chore(contract): publish user manager page permission
a51ec19 refactor(user): use shared pagination
ed86f36 chore(contract): publish user management operations
05a5158 chore(contract): sync user management schemas
```

前端提交：

```text
eb3c01a chore(contract): sync user manager page permission
9a807b5 refactor(user): centralize user management API
494cdd6 refactor(user): use crud table in user manager
2102216 refactor(user): remove redundant management workflow
117f70d chore(contract): sync user management schemas
```

用户模块验收恢复点：

```text
Backend: 61bcf8926a915334a1b12de80709e84c3baa8c59
Frontend: 117f70d
```

验收范围：用户列表分页、用户资料读取、编辑、状态切换、单个/批量删除、权限矩阵和页面访问。用户确认人工测试无问题。403 全局通知调整后来已作为独立前端提交 `9b034cb474115fc5ae6712c5408011631c15657d` 收口：401/404 保持静默，403 授权失败进入全局通知。该提交不是 User 模块验收恢复点的一部分，但它是 Role 执行前必须验证并保留的前端基线。

合同恢复点：后端 manifest 绑定 `ed86f361e6514cc88502d49c548c142dc15abc59`，前端 lock 的 manifest SHA-256 为 `c15b60fd645179622280e8aba5cf182434420082afbfe770fcfa7bd193e090cd`。

计划内后端短测试、前端 ESLint、合同生成/检查和四个前端测试文件分别运行均通过。四个前端测试文件在同一 Vitest 进程合跑时存在既有模块隔离失败：`generated-operations.test.ts` 可能得到 `installApiClient is not a function` 或 `isApiError is not a function`；各文件独立运行全部通过，本 Wave 未修改 HTTP 产品代码或 Vitest 配置。

明确未运行：`admin-dev`、全量 Go/Vue 测试、全量 typecheck、Playwright、`verify:frontend` 和发布长脚本。

### Wave 03 Role 检查点（2026-08-14）

状态：代码迁移、定向短验证和数据库 forward migration 已完成，等待用户人工验收。不得自动进入 Permission。

Role 执行前验证并保留了既有 403 全局通知提交：

```text
Frontend: 9b034cb474115fc5ae6712c5408011631c15657d fix(http): 调整错误通知逻辑以包含授权失败
```

401 和 404 继续静默；403 授权失败触发全局通知。本轮没有重复修改 notifier。

后端提交：

```text
49c4876ac2d6cc2824f709664283b96191272d83 fix(permission): protect role manager page reads
abd8777402dd8a9df76f0027a9ec2dc94716f87d refactor(role): use shared pagination and principal invalidation
811dc03ea7eb5f720007334d5d420cf64a9e3480 chore(contract): publish role management schemas
872bf139de17fa4ece67f0fd8dc14c8a35476d8f test(role): align server pagination fixture
```

前端提交：

```text
84a9fde493112049a1d3110931893d65ee9d68e2 refactor(role): use direct frontend api
6523f78180972c367b1279f46beb260d9145a2ed refactor(role): remove retired cascader helpers
6218e97df48ca2243d9d6768bfd9f9f4ed1f086c chore(contract): sync role management schemas
```

数据库迁移：

```text
Migration: 202608140002_set_role_page_code.sql
SHA-256: de70a82f9c14cead49af210271a3ff1034e222f5fd8d8b1f63d63035cfbfbcf9
Applied version: 202608140002
permissions.id=13: admin | type=2 | /permission/role | permission/role | permission_role
role_permissions rows for permission_id=13: 2（未新增或改写）
```

迁移期合同：后端 manifest 绑定 `abd8777402dd8a9df76f0027a9ec2dc94716f87d`；前端 lock 的 manifest SHA-256 为 `a89d782c8d176aebf346ebb3f93868f675d4641c45d43cc6da7cdd297d945a2d`。

最终定向验证结果：

```text
PASS  go test ./internal/shared/pagination ./internal/module/permission ./internal/module/role ./internal/module/role/transport/admin -count=1
PASS  go test ./internal/architecture -run 'TestDatabaseBaselineRoleManagerPagePermissionContract|TestRoleManagerPagePermissionMigrationIsGuardedAndForwardOnly' -count=1
PASS  go test ./internal/admincontract -run 'TestViewsProtectRoleManagerWithPagePermission|TestRoleManagerReadsUsePagePermission|TestOpenAPIContainsEveryRuntimeAdminOperation' -count=1
PASS  go test ./internal/server -run 'Test.*Role' -count=1
PASS  npm test -- tests/shared/http/notifier.test.ts tests/shared/permission/role-api.test.ts tests/shared/permission/role-matrix.test.ts tests/component/permission/RolePermissionMatrix.test.ts tests/unit/http/generated-operations.test.ts tests/unit/routing/contracts.test.ts（6 files，31 tests）
PASS  npx eslint src/lib/http/notifier.ts src/api/permission/role.ts src/views/Main/permission/role/use-role-page.ts src/views/Main/permission/role/index.vue src/views/Main/permission/role/role-matrix.ts src/views/Main/permission/role/components/RolePermissionMatrix.vue tests/shared/http/notifier.test.ts tests/shared/permission/role-api.test.ts
PASS  backend/frontend git diff --check
```

减法检查确认 Role 后端不再定义本地 `Page` 或死 `CacheInvalidator`，Role 前端 API 不再依赖 generated operations，旧级联 helper 引用清零；通知任务仍使用同一 `RoleApi.list`。

历史数据库门禁记录（已废止）：旧 `scripts/database.ps1 check` 曾报 `DATABASE_SEED_FACTS_MISMATCH`。该结果只说明旧 baseline 与本地数据不同，不是当前数据库验收输入。

明确未运行：`admin-dev`、全量 Go/Vue 测试、全量 typecheck、Playwright、`verify:frontend` 和发布长脚本。

Role 人工验收清单：

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

### Wave 03 Permission + AuthPlatform 检查点（2026-08-23）

状态：代码、合同和本地 forward migration 已完成并通过用户人工验收；数据库外置所有权切换已完成并通过用户启动验证。当前进入剩余 Mail、SMS、日志、UploadConfig，尚未进入支付或 AI。

后端提交：

```text
250d1af fix(permission): preserve page access codes
431138f refactor(authplatform): use shared pagination and page access
b15eee8 fix(permission): protect governance page reads
8b38e78 chore(contract): publish permission governance schemas
c32eb64 test(server): align auth platform pagination fixture
```

前端提交：

```text
c38db28 refactor(permission): use direct frontend api
16f3794 chore(contract): sync permission governance schemas
```

数据库事实：

```text
Migration: 202608150001_set_permission_governance_page_codes.sql
SHA-256: 69f9bdc45d6ccb1f683fbfe0446c7818674e20b62cffcc3aa37f3a04448e48c6
Applied version: 202608150001
permissions.id=12: admin | type=2 | /permission/permission | permission/permission | permission_permission
permissions.id=85: admin | type=2 | /permission/authPlatform | permission/authPlatform | permission_authPlatform
permissions count before/after: 132/132
```

admin 超管核验：唯一命中 `admin/admin@qq.com`，user_id=1，role_id=2，用户启用且未删除，角色未删除；role_permissions 对 ID 12、85 均已为 `is_del=2`，未新增或改写关系，未补按钮权限。

Redis 精确清理：仅使用 DB 0、prefix `token:`，删除当前 admin user_id=1 的 1 个 Principal state key，删除后目标零命中；未访问 DB 1、2、3。由于 admin-dev 未由本批次启动，Principal 重建证据等待用户下一次受保护请求。

定向验证：

```text
PASS backend Permission/AuthPlatform/architecture/admincontract/server 定向测试
PASS frontend --no-file-parallelism 定向测试（9 files，35 tests）
PASS frontend ESLint（Permission/AuthPlatform API、相关页面和测试）
PASS backend/frontend contract generate/check
PASS git diff --check（两个仓库）
```

历史计划外问题（已废止）：旧 database baseline 与本地 `system_settings` 行数不同；数据库外置所有权切换后不再运行该门禁，也不修改业务数据来迎合旧 seed。

明确未运行：`admin-dev` 启停、`go test ./...`、全量 Vue 测试、全量 typecheck、Playwright、`verify:frontend` 和发布长脚本。

## Wave 04：运行与存储

保留 `admin-worker` 和 Asynq，任务显式注册。WebSocket 收口为 `realtime` 包，MySQL 是消息/运行终态事实，Redis Pub/Sub 只做广播。COS 使用 `init -> 直传 -> complete -> HEAD 校验 -> MySQL 元数据`，API 不中转大文件。

## Wave 05：支付

支付域按 `order`、`wallet`、`provider`、`handler` 组织。支付宝回调先验签，再按订单号幂等更新；AI 只能调用钱包 Service，不能直接写支付表；金额、事务、退款和流水测试先于 UI 迁移。

## Wave 06：AI

专项设计：

```text
docs/superpowers/specs/2026-08-14-ai-module-radical-simplification-design.md
```

本波不是把上下文工程改成可选，而是彻底删除：

```text
Profile / Space / Document / Memory / Context Plan / Citation
Embedding / Rerank / Qdrant
wallet hold / reserve / capture / release
```

替代运行语义：

- 普通聊天只使用系统提示词、最近 N 个完整轮次、当前消息、原生附件和本 Run 工具结果；
- `max_history` 表示完整轮次数，默认 20、范围 0 到 50；
- 历史轮次里的图片和文件继续从持久化附件事实授权，并通过 COS 重新物化；
- 模型类型只保留 `chat` 和 `image`；
- 新 Run 只要求接受时余额大于 0，不做理论最大费用冻结；
- 真实 Provider Usage 结算允许余额为负，余额非正时拒绝下一次 Run；
- Usage 不完整时不猜测、不扣款，明确记录为 `unbilled`；
- 保留供应商、智能体、会话、消息、Run、附件、工具、生图、WebSocket、Usage、钱包流水、充值和支付宝支付。

本波必须拆成专项设计规定的短波次逐段验收。不得恢复旧 Knowledge，不得建立 Context 兼容层，也不得保留 Qdrant Docker 作为“以后可能使用”的闲置依赖。

## Wave 07：生成合同与旧架构退役

专项设计：

```text
docs/superpowers/specs/2026-08-14-admin-generated-contract-retirement-design.md
```

只有所有消费者迁移且用户验收后，才按引用盘点删除：

- 后端 `contracts/admin/v1`、`internal/admincontract`、`cmd/admin-contract`；
- route `HTTPContract`、`Definition.Contract` 和各模块重复请求/响应合同声明；
- 前端 `contracts/backend/admin`、`src/modules/http/generated`、`src/modules/routing/generated`；
- OpenAPI、permissions/views/manifest、realtime schema bundle、合同镜像、lock、revision 和 SHA-256 检查；
- `contract:sync`、`contract:generate`、`contract:check` 及生成、同步、校验和发布门禁；
- 只验证生成 bundle、固定哈希、固定 revision、生成文件路径或操作清单的测试；
- AppKernel、RuntimeRouteRegistry、万能 Workflow、纯转发 Adapter；
- 已完成迁移且引用清零的 `src/lib`、`src/modules` 和其他过渡目录；
- 一次性 context preflight、旧 `admin-db` 入口和数据库生命周期脚本；
- 多层 PowerShell smoke、合同生成、发布 rehearsal、browser-only 和历史 cutover 脚本；
- 失效架构文档、空目录和只保护旧文件路径的测试。

替代事实源固定为：

```text
后端 route.go + request.go + response.go + handler.go + Handler 短测试
前端 src/api/<business>.ts + TypeScript/Zod + src/utils/request.ts
权限 数据库事实 + 后端路由中间件 + 403 矩阵测试
菜单 users/me + 普通页面 registry/import.meta.glob
实时 后端/前端明确事件结构 + 协议短测试
```

Wave 07 完成后不保留 OpenAPI、Swagger、SDK、生成合同可选产物或 deprecated facade。删除合同生成体系不等于删除运行时 RBAC、动态菜单、`users/me`、WebSocket 协议或统一错误响应。

最终公开入口必须是：

```text
cmd/admin-api
cmd/admin-worker

scripts/admin-dev.ps1
scripts/docker.ps1
scripts/install-shortcuts.ps1
scripts/internal/common.ps1
```

## 禁止事项

- 不在 Wave 01 顺手迁移系统设置、AI、支付或数据库表；
- 不在 Wave 03 的公共分页任务中一次性改写所有业务模块；
- 不把后端字典、本地化标签或页面展示映射复制进 `src/enums`；
- 不改变 API 返回外层 `code/data/msg/error`；
- 不将页面访问改成新的权限码或新表；
- 不把 Redis 多 DB 当成 Cluster；Cluster 适配另设 Wave；
- 不恢复 Context/RAG/Qdrant/Embedding/Rerank、跨会话长期记忆或钱包 Hold；
- 不为新接口增加 route `HTTPContract`、generated operation 或合同生成物；
- 不用另一套 IDL、SDK 生成器或兼容 facade 替代已退役合同体系；
- 不用 `|| ''`、`?? []` 或空对象吞掉合同错误；
- 不运行全仓长测试代替目标测试；
- 不主动启动、停止或重启用户的 `admin-dev`；
- 不在个人开发阶段新增 `database/`、migration、seed、baseline 或 `admin-db`/`admin-cli` 数据库命令；
- 不覆盖其他窗口或用户的未提交修改。

## 每波完成标准

```text
目标行为有短测试
-> 受影响模块测试通过
-> git diff --check 通过
-> 变更只在本波范围
-> 用户人工验收
-> 记录提交和下一波入口
```
