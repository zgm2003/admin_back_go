# Admin 平台架构减法执行总索引

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development to execute each wave. Every wave has its own review and user acceptance gate. Do not execute this index as one giant batch.

**Goal:** 在不破坏现有业务、数据库、接口和用户习惯的前提下，把 Admin 前后端恢复为社区可读的模块化单体和线性调用链。

**Architecture:** 后端固定 `route -> middleware -> handler -> service -> repository -> model`，前端固定 `views -> api -> request -> backend`。MySQL 保存业务事实，Redis 按角色提供缓存/实时/Token/队列能力，COS 保存文件内容，Qdrant 保存可重建派生索引。

**Tech Stack:** Go 1.26.5、Gin、GORM、MySQL 8.4、Redis 8、Asynq、Qdrant、Vue 3.5、TypeScript、Vite、Element Plus、Vitest。

---

## 使用规则

唯一设计来源是：

```text
E:/admin/admin_back_go/docs/superpowers/specs/2026-08-13-admin-architecture-reduction-direction.md
```

执行前必须读取中心方向书和当前代码，不能只按旧计划中的文件名操作。中心方向书已经确认：

- 原仓逐模块迁移，不新建长期 v2 双轨；
- 保留 API、数据库、菜单、权限码和用户操作习惯；
- 删除前必须有引用盘点、恢复点和用户批准；
- 不跑全量长脚本、Playwright 或 `admin-dev`，由用户人工启动和验收；
- 每波只修改自己的文件，不能顺手修计划外问题；
- 每波完成后先运行短测试，再等用户人工验收；
- 只有验收后才允许删除对应旧实现。

当前恢复点：

```text
Backend baseline tag: pre-database-baseline-20260813
Database baseline: 202608130001
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
| Wave 03 | 公共分页、配置、公共响应、RBAC、后台基础模块 | 基础段已完成并人工验收；进入用户模块迁移 | 只删除已迁移模块旧层 |
| Wave 04 | Worker、任务、Realtime、COS | 后台任务、WebSocket、上传边界收口 | 只删除已迁移 runtime 包装 |
| Wave 05 | 支付与钱包 | 订单、钱包、供应商、回调幂等边界清楚 | 只删除支付旧适配层 |
| Wave 06 | AI 七个子包 | AI、附件、上下文、扣费均沿真实业务边界 | 核心域最后清理 |
| Wave 07 | 旧合同、Kernel、Registry、脚本归档 | 日常开发不依赖生成器和脚本框架 | 最后一波集中删除 |

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
Backend HEAD: 2a34e3eaf477eb177bf374e8319eada8132b0697
Frontend HEAD: 7528becad61783ca37cc7de4c793c30e0e4ed701
Backend branch: master
Frontend branch: master
Backend worktree: clean
Frontend worktree: clean
```

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

### Wave 03 下一入口

基础段完成后，下一项只迁移 Admin 用户管理核心，计划文件为：

```text
docs/superpowers/plans/2026-08-14-admin-architecture-reduction-wave-03-user.md
```

用户模块计划完成并人工验收后，才进入角色模块；不得在同一计划中继续迁移权限、邮件、短信、日志或上传。

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

## Wave 04：运行与存储

保留 `admin-worker` 和 Asynq，任务显式注册。WebSocket 收口为 `realtime` 包，MySQL 是消息/运行终态事实，Redis Pub/Sub 只做广播。COS 使用 `init -> 直传 -> complete -> HEAD 校验 -> MySQL 元数据`，API 不中转大文件。

## Wave 05：支付

支付域按 `order`、`wallet`、`provider`、`handler` 组织。支付宝回调先验签，再按订单号幂等更新；AI 只能调用钱包 Service，不能直接写支付表；金额、事务、退款和流水测试先于 UI 迁移。

## Wave 06：AI

AI 最后迁移，内部只保留真实子包：

```text
provider / agent / conversation / run / asset / context / billing
```

Embedding/Qdrant 不可用时，上下文增强降级，普通聊天继续工作。MySQL 保存文档和版本事实，Qdrant 只保存可重建索引。附件只引用 `asset_id`，文件内容继续走 COS。

## Wave 07：删除与归档

只有所有消费者迁移且用户验收后，才按引用盘点删除：

- generated operations/views/permissions 和 runtime schema 日常依赖；
- AppKernel、RuntimeRouteRegistry、万能 Workflow、纯转发 Adapter；
- 已完成迁移且引用清零的 `src/lib`、`src/modules` 和其他过渡目录；
- `cmd/admin-contract`、一次性 context preflight 和旧 `admin-db` 入口；
- 多层 PowerShell smoke、合同生成、发布 rehearsal、browser-only 和历史 cutover 脚本；
- 失效架构文档、空目录和只保护旧文件路径的测试。

最终公开入口必须是：

```text
cmd/admin-api
cmd/admin-worker
cmd/admin-cli

scripts/admin-dev.ps1
scripts/database.ps1
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
- 不把 Qdrant 必需性改成阻断普通聊天；
- 不用 `|| ''`、`?? []` 或空对象吞掉合同错误；
- 不运行全仓长测试代替目标测试；
- 不主动启动、停止或重启用户的 `admin-dev`；
- 不覆盖其他窗口或用户的未提交修改。

## 每波完成标准

```text
目标行为有短测试
-> 受影响模块测试通过
-> git diff --check 通过
-> 变更只在本波范围
-> 用户人工验收
-> 记录提交和下一波恢复点
```
