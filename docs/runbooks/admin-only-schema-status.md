# Admin 本地数据库所有权状态

Status: active local-development ownership reference

MySQL 是当前业务数据、表结构和索引的唯一业务事实。仓库不包含
`database/`、seed、migration、baseline 或数据库生命周期命令；
`internal/infra/database` 只负责 Go 连接和事务适配。

## 只读确认

执行任何直接 SQL 前，确认目标是当前本机 Docker Compose 的 `admin` 数据库，拒绝
远程或不明确 DSN。不得在输出中打印密码或完整 DSN。

## 变更规则

work-ai 对确认过的本机 `admin` 数据库执行最小 SQL，并使用 `SHOW CREATE TABLE`、
`SHOW INDEX` 和精确 `SELECT` 读回验证。不得删除业务表、字段、索引或数据，也不得
为新模块创建 migration 文件。

进入多人协作、部署或需要新机器恢复时，另行批准正式数据库基线、备份和迁移方案。

**STOP** when the database target is not the local `admin` instance or the requested change
would alter an unapproved business fact.
