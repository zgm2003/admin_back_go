INSERT INTO `cron_task` (
  `name`, `title`, `description`, `cron`, `cron_readable`, `handler`,
  `status`, `is_del`, `created_at`, `updated_at`
) VALUES (
  'export_cleanup_expired',
  '清理过期导出任务',
  '由 Worker 软删除已过期导出任务，列表与统计接口保持只读',
  '0 0 * * * *',
  '每小时',
  'export:cleanup-expired:v1',
  1,
  2,
  UTC_TIMESTAMP(),
  UTC_TIMESTAMP()
)
ON DUPLICATE KEY UPDATE
  `title` = VALUES(`title`),
  `description` = VALUES(`description`),
  `cron` = VALUES(`cron`),
  `cron_readable` = VALUES(`cron_readable`),
  `handler` = VALUES(`handler`),
  `status` = VALUES(`status`),
  `is_del` = VALUES(`is_del`),
  `updated_at` = UTC_TIMESTAMP();
