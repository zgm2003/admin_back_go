# AI 计费数据库维护

旧的 AI Billing expand/backfill/contract 阶段链和
`ai_billing_migration_metadata` 已随本地初始化基线退役。当前数据库事实只来自：

```text
database/schema.sql
database/seed.sql
database/migrations/*.sql
schema_migrations
```

修改计费表前必须先确认这是当前真实需求，并保留金额精度、Run 幂等、钱包冻结、
结算终态和账本唯一性。不要为已经完成的历史阶段恢复 marker 表或兼容分支。

新变更使用版本大于 `202608130001` 的单个前向 migration。migration 字节一旦记入
`schema_migrations` 就不可修改；修正错误必须新增版本。应用前停止会写相关表的本地
进程，保留可恢复备份，然后执行：

```powershell
pwsh -NoProfile -File scripts/database.ps1 migrate
pwsh -NoProfile -File scripts/database.ps1 check
```

禁止手工删除 migration ledger、伪造完成状态、用旧 cents 字段猜测当前 units，或在
状态不一致时继续派发付费 AI 请求。
