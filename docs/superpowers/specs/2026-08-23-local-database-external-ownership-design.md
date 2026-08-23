# 本地开发数据库外置所有权设计

> 状态：已批准方向，等待 Wave 03 Permission + AuthPlatform 完成后执行。

## 1. 需求判断

项目处于个人本地开发阶段，没有上线、多人协作或空库交付要求。当前仓库中的
`database/schema.sql`、`seed.sql`、迁移链、baseline、PowerShell 生命周期脚本和
数据库门禁，维护成本已经超过它们对当前阶段的实际价值。

这不是删除 MySQL，也不是删除业务表。要删除的是“仓库同时维护一份数据库事实”的
治理链，避免模型每次写业务都被迁移文件、哈希、seed 和恢复脚本拖住。

## 2. 决策

当前阶段采用“本地数据库外置所有权”：

```text
仓库代码             只依赖运行中的 MySQL，不读取仓库 SQL 文件
本地 MySQL           当前唯一数据库事实源
仓库外私有 SQL       用户可用 Navicat 自行导出，供个人恢复使用
work-ai              直接读取并修改本地 MySQL，再验证代码和数据
```

推荐的私有导出位置是 `E:/admin-local/admin.sql`。它不属于任一 Git 仓库，不能
提交、生成到 `database/`，也不能被前端、后端或 Docker 启动流程自动读取。

本设计明确接受：个人开发阶段不提供仓库内一键空库初始化、迁移回滚、自动备份和
发布数据库门禁。未来进入多人协作、部署或需要新机器一键恢复时，再从外部 SQL
基线重新建立正式数据库管理方案。

## 3. 保留与删除边界

### 3.1 必须保留

- `internal/infra/database`：连接池、事务和数据库错误处理；
- MySQL Docker Compose 服务及 `admin-go.env` 的连接配置；
- 所有业务表、字段、索引、约束和运行时 Repository；
- `/health`、`/ready` 对数据库连通性的检查；
- 业务 Service/Repository 测试和真实数据库语义测试；
- `docs/` 中的一份数据库所有权说明，防止后续模型重新引入迁移链。

### 3.2 执行后删除

- 顶层 `database/` 目录及其 schema、seed、reference、migrations、baseline；
- `scripts/database.ps1`、`scripts/verify-database.ps1` 和数据库 baseline 测试脚本；
- 只为仓库 SQL、migration 文本、baseline 哈希服务的架构测试；
- 发布 manifest 中的数据库文件哈希、migration checksum 和数据库发布门禁；
- `cmd/admin-db` 及其测试，不创建替代的 `admin-cli`；
- 仅被上述入口使用的历史数据库演进辅助代码和文档引用。

`schema_migrations` 不是业务数据。执行切换时，work-ai 必须先通过只读代码搜索
确认运行时代码没有读取它，再直接从当前本地 MySQL 删除；若仍有运行时调用，先
删除调用再删除表，不增加兼容表或空实现。

## 4. 直接数据库变更协议

所有后续数据库变更由 work-ai 在当前本地 MySQL 执行，不创建 migration 文件、
seed 修改或 schema 哈希：

1. 读取当前表、字段、索引和目标数据，确认变更只作用于本地 `admin` 数据库；
2. 执行最小 SQL，优先使用事务；DDL 无法事务化时逐条执行并立即检查结果；
3. 用 `SHOW CREATE TABLE`、`SHOW INDEX` 和精确 `SELECT` 验证变更；
4. 运行受影响 Go 包的短测试和 `/ready` 检查；
5. 后端、前端代码分别提交并合并到 `master`，报告 SQL、验证结果和提交 SHA。

计划不提供备份步骤，这是个人开发阶段的明确取舍。任何 SQL 执行前仍必须做
目标身份检查：拒绝非本机、非 `admin` 数据库和非当前 Docker 状态实例；目标不
匹配时停止，不猜测、不切换到其他 DSN。

## 5. 当前 Wave 03 的并发边界

work-ai 正在执行 Permission + AuthPlatform，本设计不打断、不回写她当前任务，
也不修改她正在使用的代码文件。该批次可以按已批准计划完成；其数据库迁移文件
属于待退役历史，不成为新的长期维护入口。

Wave 03 完成并人工验收后，单独执行本设计对应的数据库所有权切换计划：先清理
仓库治理链，再把仍需要的数据库事实直接落到当前 MySQL，最后合并 master。以后
新模块不再追加 `database/migrations/*.sql`。

## 6. 兼容性与接受的风险

### 保持不变

- API 路径、`code/data/msg/error` 外层协议和前端行为；
- 业务表结构和已有数据，除明确批准的数据库减法；
- MySQL、Redis、COS、Qdrant 的运行时连接边界；
- `admin-dev` 的启动方式，不自动增加初始化或迁移动作。

### 明确接受

- 新机器不能仅靠 Git 仓库建立空库；
- 数据库历史不再由 Git 提供可审计的逐步迁移链；
- 个人误操作没有仓库内自动恢复流程；
- 进入平台化前，数据库变更依赖当前 MySQL 和用户自己的 SQL 导出。

这些风险只在本地个人开发阶段接受；一旦项目需要交付或多人协作，必须重新评估
并建立新的数据库基线，不得继续沿用本设计的无备份模式。

## 7. 完成标准

- `database/`、`admin-db` 和数据库生命周期脚本不再存在于主动仓库路径；
- `rg` 不再发现运行时代码读取 schema、seed、migration 或 baseline；
- API/Worker 只通过 `internal/infra/database` 连接 MySQL；
- `schema_migrations` 没有运行时读者并从本地数据库清理；
- Permission + AuthPlatform 的代码和前端已合并到两个仓库 `master`；
- 后续新计划明确禁止新增 migration 文件；
- 人工启动 `admin-dev` 后，`/ready`、登录、RBAC、系统设置和 AI 基础页面仍正常。
