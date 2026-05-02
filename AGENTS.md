# AGENTS.md for admin_back_go

## 当前定位

这是 `E:\admin_go` 的 Go 主后端。

主架构：

```text
Gin modular monolith
route -> handler -> service -> repository -> model
```

## 必读

开始任何 Go 后端任务前，先读：

```text
E:\admin_go\AGENTS.md
E:\admin_go\docs\architecture\04-go-backend-framework.md
docs\architecture.md
internal\module\README.md
internal\platform\README.md
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
```

## 当前阶段

当前只做架构骨架。业务迁移还没开始。
