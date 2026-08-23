# AGENTS.md for admin_back_go

## 当前定位

这是 `E:\admin\admin_back_go` 的 Go 主后端。

主架构：

```text
Gin modular monolith
route -> handler -> service -> repository -> model
```

## 必读

开始任何 Go 后端任务前，先读：

```text
E:\admin\LONG_TASK_PARALLEL_EXECUTION.md
E:\admin\admin_back_go\docs\architecture.md
E:\admin\admin_back_go\internal\module\README.md
E:\admin\admin_back_go\internal\platform\README.md
```

## 禁止

```text
禁止写成 Java 风格
禁止无意义 interface
禁止 ServiceImpl
禁止 Manager/Factory 滥用
禁止 handler 直接查 DB/Redis
禁止 service 依赖 gin.Context
禁止在 RBAC 契约未固定前实现权限业务
禁止写兜底字段、猜字段、吞未知 DTO
禁止新接口继续全 POST 动作 URL
```

个人本地数据库阶段的额外边界：

- 禁止新增 `database/`、migration、seed、baseline 或数据库生命周期脚本；
- 禁止恢复 `cmd/admin-db` 或创建替代的 `admin-cli`；
- MySQL 是唯一业务事实，运行时只通过 `internal/infra/database` 连接；
- 数据库变更只能按 `docs/database-ownership.md` 对确认过的本机 `admin` 实例执行，
  不得把密码或完整 DSN 写入日志、SQL 或提交。

## 当前阶段

`admin_back_go` 已是 active Go runtime，现有 Auth/RBAC、用户、日志、通知、上传、支付、AI、realtime、queue 和 worker 能力以运行时代码、测试及 `docs/architecture.md` 为事实来源。

Infinite Canvas 平台接入只能按已批准规格和 `docs/superpowers/plans/2026-07-31-infinite-canvas-platform-foundation-execution-index.md` 分波执行：先冻结当前 AI/COS 基线，再推进 platform RBAC、trusted Auth、项目/素材/提示词和独立前端。不得绕过 Contract 与数据库基线检查，也不得恢复退役 app/canvas adapter。
