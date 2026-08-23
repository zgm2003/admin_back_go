# 上下文工程本地验收

状态：历史上下文工程说明，数据库准备部分已被本地数据库外置所有权方案取代。

## 数据库准备

数据库准备和变更遵循 `docs/database-ownership.md`：确认本机 Docker 的 `admin`
数据库后执行最小 SQL，并读回验证。此历史 runbook 不再提供仓库 schema、迁移或
reset 命令。

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
