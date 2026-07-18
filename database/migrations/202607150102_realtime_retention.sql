-- Close the P05 request identity and durable realtime retention contract.
ALTER TABLE `ai_reply_commands`
  MODIFY COLUMN `request_id` VARCHAR(128) NOT NULL;

ALTER TABLE `ai_runs`
  MODIFY COLUMN `request_id` VARCHAR(128) NOT NULL COMMENT 'client request identifier';

UPDATE `realtime_events`
SET `expires_at` = DATE_ADD(`occurred_at`, INTERVAL 7 DAY)
WHERE `expires_at` IS NULL;

ALTER TABLE `realtime_events`
  MODIFY COLUMN `request_id` VARCHAR(128) NULL,
  MODIFY COLUMN `expires_at` DATETIME(6) NOT NULL;

CREATE INDEX `idx_realtime_expiry`
  ON `realtime_events` (`expires_at`, `sequence`);

CREATE TABLE `realtime_event_retention_watermarks` (
  `target_type` VARCHAR(16) NOT NULL,
  `target_id` VARCHAR(64) NOT NULL,
  `deleted_through_sequence` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`target_type`, `target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
