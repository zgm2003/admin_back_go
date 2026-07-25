# AI Billing Migration Recovery

AI billing 的 expand/backfill/contract/permissions/dispatch-state 脚本运行在维护窗口，且
MySQL 的 `ALTER TABLE` 会隐式提交。每个阶段都会在
`ai_billing_migration_metadata` 写入 `phase=started`，只有该阶段全部校验和
DDL 完成后才写入 `phase=complete`。

## 运行前

1. 停止所有旧的 AI paid writers，并确认 Plan 05/06/07 的 durable Run 写入路径
   已上线；不能用旧 `RunRecorder.Start` 创建新的计费 Run。
2. 依次执行 expand、backfill、contract、permissions、dispatch-state，不能跳阶段。
3. contract 只在已部署二进制通过 units-only 校验后设置会话变量：

```sql
SET @ai_billing_units_only_verified = 1;
```

## 发现半完成阶段时

先停止应用并只读检查：

```sql
SELECT migration_key, phase, phase_started_at, phase_completed_at,
       marker_version, HEX(marker_sha256)
FROM ai_billing_migration_metadata
ORDER BY migration_key;
```

如果目标阶段为 `started`，**不要重新执行同一 migration，也不要直接把 phase
改成 complete**。根据对应脚本已经提交的 DDL，逐项核对列、索引、约束、外键、
回填值和旧/新单位换算；必要时编写一个新的、可审查的纠正 migration。只有纠正
migration 完成并通过全部 guard 后，才能由维护负责人记录新的完成边界。

如果阶段为 `complete`，脚本已经执行过，重复运行必须被视为操作错误；使用
Atlas 的 migration 版本和本表 phase 一起确认发布状态，不得用删除 metadata 行
的方式“解锁”。

## 禁止事项

- 不执行 `DROP DATABASE`、不创建备份数据库、不修改 `role_permissions`。
- 不用旧 cents 列推断已经完成 contract；contract 前必须逐行验证
  `units = cents * 1000000`。
- 不把 legacy marker 复制到新 Run；新 Run 必须由 Gateway 写入真实 fingerprint、
  pricing snapshot 和 billing 状态。
- 不在未知或不一致的阶段继续派发 AI 请求。
