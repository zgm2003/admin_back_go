SELECT 'export_cleanup_worker_schedule' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 1
  WHERE (
    SELECT COUNT(*)
    FROM cron_task
    WHERE name = 'export_cleanup_expired'
      AND handler = 'export:cleanup-expired:v1'
      AND status = 1
      AND is_del = 2
      AND cron <> ''
  ) <> 1
) invalid;
