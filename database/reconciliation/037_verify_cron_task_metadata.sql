SELECT 'cron_task_utf8_metadata' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 1
  WHERE (
    SELECT COUNT(*)
    FROM `cron_task`
    WHERE `name` = 'export_cleanup_expired'
      AND BINARY `title` = BINARY '清理过期导出任务'
      AND BINARY `description` = BINARY '由 Worker 软删除已过期导出任务，列表与统计接口保持只读'
      AND BINARY `cron_readable` = BINARY '每小时'
  ) <> 1
  UNION ALL
  SELECT 1
  WHERE (
    SELECT COUNT(*)
    FROM `cron_task`
    WHERE `name` = 'realtime_event_retention_cleanup'
      AND BINARY `title` = BINARY '清理过期实时事件'
      AND BINARY `description` = BINARY '每日清理超过七天的 durable realtime events，并在同一事务推进用户 retention watermark'
      AND BINARY `cron_readable` = BINARY '每天 03:15'
  ) <> 1
) invalid;
