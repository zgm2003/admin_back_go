# 上下文工程本地验收

状态：本地开发流程。上下文工程的完整表结构已经属于
`database/schema.sql`，不再存在独立的 Atlas cutover 或历史迁移窗口。

## 数据库准备

已有新基线数据库只需检查当前事实：

```powershell
pwsh -NoProfile -File scripts/database.ps1 migrate
pwsh -NoProfile -File scripts/database.ps1 check
```

需要从零验收时，先由开发者停止 `admin-dev`，再执行破坏性的本地重建：

```powershell
pwsh -NoProfile -File scripts/database.ps1 reset -ConfirmReset admin -CreateAdmin
```

`reset` 会重建本地 `admin` schema，并清理本项目专用的 Redis 数据库和
`admin_context_*` Qdrant 派生索引。它不会自动停止或启动 API/Worker，也不会保留
本地会话、对话、文档、供应商配置或支付数据。执行前必须确认这些数据已经不需要，
或可从基线前的 Git tag 和外部完整备份恢复。

未来的 schema 变更只允许新增版本大于 `202608130001` 的
`database/migrations/*.sql`，再运行 `migrate` 和只读 `check`。应用启动不得自动迁移。

## 启动检查

数据库准备完成后由开发者自行启动 `admin-dev`，然后检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health
Invoke-RestMethod http://127.0.0.1:8080/ready
```

`/health` 与 `/ready` 必须同时成功。不要用安静日志代替 readiness，也不要在本次
基线验收中调用付费 AI 或支付供应商。

## 人工验收

- 上下文工程页面能打开空间、文档、索引配置和评估。
- 智能体可以关闭上下文配置；没有 Embedding 配置时普通对话仍可用。
- TXT、Markdown、PDF、DOCX、CSV、XLSX 文档展示真实处理状态。
- 有效引用可在刷新后打开持久化来源；无效引用只显示为普通文本。
- 对话完成后刷新仍保留消息，运行记录保持终态。
- 运行详情能显示预算、阶段、选中项、排除项和失败原因。
- 菜单和搜索只出现“上下文工程”，不恢复已退役 RAG 入口。

记录失败时使用 conversation/run/document ID，不记录提示词、对象密钥、签名 URL、
API key 或用户文件内容。
