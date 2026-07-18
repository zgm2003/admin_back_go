SET NAMES utf8mb4;

UPDATE `cron_task`
SET
  `title` = '清理过期导出任务',
  `description` = '由 Worker 软删除已过期导出任务，列表与统计接口保持只读',
  `cron_readable` = '每小时',
  `updated_at` = UTC_TIMESTAMP()
WHERE `name` = 'export_cleanup_expired'
  AND (
    NOT (BINARY `title` <=> BINARY '清理过期导出任务')
    OR NOT (BINARY `description` <=> BINARY '由 Worker 软删除已过期导出任务，列表与统计接口保持只读')
    OR NOT (BINARY `cron_readable` <=> BINARY '每小时')
  );

UPDATE `cron_task`
SET
  `title` = '清理过期实时事件',
  `description` = '每日清理超过七天的 durable realtime events，并在同一事务推进用户 retention watermark',
  `cron_readable` = '每天 03:15',
  `updated_at` = UTC_TIMESTAMP()
WHERE `name` = 'realtime_event_retention_cleanup'
  AND (
    NOT (BINARY `title` <=> BINARY '清理过期实时事件')
    OR NOT (BINARY `description` <=> BINARY '每日清理超过七天的 durable realtime events，并在同一事务推进用户 retention watermark')
    OR NOT (BINARY `cron_readable` <=> BINARY '每天 03:15')
  );
