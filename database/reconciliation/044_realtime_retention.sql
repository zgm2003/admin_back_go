-- P05 non-destructive request identity and realtime retention expansion.

SET @p05_sql := IF(
  EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ai_reply_commands' AND column_name='request_id'
      AND (column_type<>'varchar(128)' OR is_nullable<>'NO')
  ),
  'ALTER TABLE `ai_reply_commands` MODIFY COLUMN `request_id` VARCHAR(128) NOT NULL',
  'DO 0'
);
PREPARE p05_stmt FROM @p05_sql; EXECUTE p05_stmt; DEALLOCATE PREPARE p05_stmt;

SET @p05_sql := IF(
  EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ai_runs' AND column_name='request_id'
      AND (column_type<>'varchar(128)' OR is_nullable<>'NO')
  ),
  'ALTER TABLE `ai_runs` MODIFY COLUMN `request_id` VARCHAR(128) NOT NULL COMMENT ''client request identifier''',
  'DO 0'
);
PREPARE p05_stmt FROM @p05_sql; EXECUTE p05_stmt; DEALLOCATE PREPARE p05_stmt;

UPDATE `realtime_events`
SET `expires_at`=DATE_ADD(`occurred_at`, INTERVAL 7 DAY)
WHERE `expires_at` IS NULL;

SET @p05_sql := IF(
  EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='realtime_events' AND column_name='request_id'
      AND column_type<>'varchar(128)'
  ),
  'ALTER TABLE `realtime_events` MODIFY COLUMN `request_id` VARCHAR(128) NULL',
  'DO 0'
);
PREPARE p05_stmt FROM @p05_sql; EXECUTE p05_stmt; DEALLOCATE PREPARE p05_stmt;

SET @p05_sql := IF(
  EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='realtime_events' AND column_name='expires_at'
      AND (column_type<>'datetime(6)' OR is_nullable<>'NO')
  ),
  'ALTER TABLE `realtime_events` MODIFY COLUMN `expires_at` DATETIME(6) NOT NULL',
  'DO 0'
);
PREPARE p05_stmt FROM @p05_sql; EXECUTE p05_stmt; DEALLOCATE PREPARE p05_stmt;

CREATE TABLE IF NOT EXISTS `realtime_event_retention_watermarks` (
  `target_type` VARCHAR(16) NOT NULL,
  `target_id` VARCHAR(64) NOT NULL,
  `deleted_through_sequence` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`target_type`, `target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

SET @p05_sql := IF(
  EXISTS(
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema=DATABASE() AND table_name='realtime_events' AND index_name='idx_realtime_expiry'
  ),
  'DO 0',
  'CREATE INDEX `idx_realtime_expiry` ON `realtime_events` (`expires_at`,`sequence`)'
);
PREPARE p05_stmt FROM @p05_sql; EXECUTE p05_stmt; DEALLOCATE PREPARE p05_stmt;

INSERT INTO `cron_task` (
  `name`, `title`, `description`, `cron`, `cron_readable`, `handler`,
  `status`, `is_del`, `created_at`, `updated_at`
) VALUES (
  'realtime_event_retention_cleanup',
  '清理过期实时事件',
  '每日清理超过七天的 durable realtime events，并在同一事务推进用户 retention watermark',
  '0 15 3 * * *',
  '每天 03:15',
  'realtime:cleanup-expired:v1',
  1,
  2,
  UTC_TIMESTAMP(),
  UTC_TIMESTAMP()
)
ON DUPLICATE KEY UPDATE
  `title`=VALUES(`title`),
  `description`=VALUES(`description`),
  `cron`=VALUES(`cron`),
  `cron_readable`=VALUES(`cron_readable`),
  `handler`=VALUES(`handler`),
  `status`=VALUES(`status`),
  `is_del`=VALUES(`is_del`),
  `updated_at`=UTC_TIMESTAMP();
