-- P02 non-destructive expand. Every ALTER is selected from observed metadata.

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='ai_runs' AND column_name='idempotency_key'),
  'DO 0',
  'ALTER TABLE `ai_runs` ADD COLUMN `idempotency_key` VARCHAR(128) NULL AFTER `input_snapshot`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='notifications' AND column_name='source_task_id'),
  'DO 0',
  'ALTER TABLE `notifications` ADD COLUMN `source_task_id` BIGINT NULL AFTER `user_id`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='notification_task' AND column_name='claim_owner'),
  'DO 0',
  'ALTER TABLE `notification_task` ADD COLUMN `claim_owner` VARCHAR(128) NULL AFTER `status`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;
SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='notification_task' AND column_name='claim_token'),
  'DO 0',
  'ALTER TABLE `notification_task` ADD COLUMN `claim_token` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `claim_owner`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;
SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='notification_task' AND column_name='claim_expires_at'),
  'DO 0',
  'ALTER TABLE `notification_task` ADD COLUMN `claim_expires_at` DATETIME(6) NULL AFTER `claim_token`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='export_tasks' AND column_name='claim_owner'),
  'DO 0',
  'ALTER TABLE `export_tasks` ADD COLUMN `claim_owner` VARCHAR(128) NULL AFTER `status`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;
SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='export_tasks' AND column_name='claim_token'),
  'DO 0',
  'ALTER TABLE `export_tasks` ADD COLUMN `claim_token` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `claim_owner`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;
SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='export_tasks' AND column_name='claim_expires_at'),
  'DO 0',
  'ALTER TABLE `export_tasks` ADD COLUMN `claim_expires_at` DATETIME(6) NULL AFTER `claim_token`'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

CREATE TABLE IF NOT EXISTS `authz_principal_versions` (
  `user_id` BIGINT NOT NULL,
  `platform` VARCHAR(32) NOT NULL,
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`user_id`, `platform`),
  CONSTRAINT `chk_authz_principal_platform` CHECK (`platform`='admin')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `ai_reply_commands` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `request_id` VARCHAR(64) NOT NULL,
  `idempotency_key` VARCHAR(128) NOT NULL,
  `platform` VARCHAR(32) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `conversation_id` BIGINT NOT NULL,
  `user_message_id` BIGINT NOT NULL,
  `assistant_message_id` BIGINT NULL,
  `state` VARCHAR(32) NOT NULL DEFAULT 'pending',
  `attempt_count` INT UNSIGNED NOT NULL DEFAULT 0,
  `max_attempts` INT UNSIGNED NOT NULL DEFAULT 3,
  `lease_owner` VARCHAR(128) NULL,
  `lease_token` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `lease_expires_at` DATETIME(6) NULL,
  `next_attempt_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `cancel_requested_at` DATETIME(6) NULL,
  `outcome_unknown_at` DATETIME(6) NULL,
  `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `last_error_message` VARCHAR(512) NOT NULL DEFAULT '',
  `started_at` DATETIME(6) NULL,
  `finished_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_reply_request` (`conversation_id`, `request_id`),
  UNIQUE KEY `uk_ai_reply_message` (`user_message_id`),
  UNIQUE KEY `uk_ai_reply_idempotency` (`idempotency_key`),
  KEY `idx_ai_reply_claim` (`state`, `next_attempt_at`, `lease_expires_at`, `id`),
  CONSTRAINT `chk_ai_reply_platform` CHECK (`platform`='admin'),
  CONSTRAINT `chk_ai_reply_state` CHECK (`state` IN ('pending','claimed','running','succeeded','failed','canceled','outcome_unknown','timed_out'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `ai_provider_attempts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `command_id` BIGINT UNSIGNED NOT NULL,
  `attempt_no` INT UNSIGNED NOT NULL,
  `idempotency_key` VARCHAR(128) NOT NULL,
  `state` VARCHAR(24) NOT NULL,
  `provider_request_id` VARCHAR(191) NOT NULL DEFAULT '',
  `response_sha256` CHAR(64) NOT NULL DEFAULT '',
  `error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `dispatched_at` DATETIME(6) NULL,
  `finished_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_attempt_command_no` (`command_id`, `attempt_no`),
  UNIQUE KEY `uk_ai_attempt_key` (`idempotency_key`),
  KEY `idx_ai_attempt_state` (`state`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `realtime_events` (
  `sequence` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `event_id` CHAR(26) NOT NULL,
  `event_type` VARCHAR(96) NOT NULL,
  `request_id` VARCHAR(64) NULL,
  `target_type` VARCHAR(16) NOT NULL,
  `target_id` VARCHAR(64) NOT NULL,
  `durability` VARCHAR(16) NOT NULL,
  `payload_json` JSON NOT NULL,
  `occurred_at` DATETIME(6) NOT NULL,
  `expires_at` DATETIME(6) NULL,
  PRIMARY KEY (`sequence`),
  UNIQUE KEY `uk_realtime_event_id` (`event_id`),
  KEY `idx_realtime_resume` (`target_type`, `target_id`, `sequence`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `ai_video_tasks` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `platform` VARCHAR(32) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `agent_id` BIGINT NOT NULL,
  `provider_id` BIGINT NOT NULL,
  `model_id` VARCHAR(191) NOT NULL DEFAULT '',
  `prompt` TEXT NOT NULL,
  `duration_seconds` INT NOT NULL DEFAULT 0,
  `size` VARCHAR(32) NOT NULL DEFAULT '',
  `resolution_name` VARCHAR(64) NOT NULL DEFAULT '',
  `provider_task_id` VARCHAR(191) NOT NULL DEFAULT '',
  `run_id` BIGINT NOT NULL DEFAULT 0,
  `status` VARCHAR(32) NOT NULL,
  `error_message` VARCHAR(1024) NOT NULL DEFAULT '',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `finished_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ai_video_user_created` (`user_id`, `is_del`, `created_at`, `id`),
  KEY `idx_ai_video_status_created` (`status`, `is_del`, `created_at`, `id`),
  KEY `idx_ai_video_provider_task` (`provider_id`, `provider_task_id`),
  CONSTRAINT `chk_ai_video_platform` CHECK (`platform` IN ('admin','canvas'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='ai_runs' AND index_name='uk_ai_runs_idempotency'),
  'DO 0',
  'CREATE UNIQUE INDEX `uk_ai_runs_idempotency` ON `ai_runs` (`idempotency_key`)'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='notifications' AND index_name='uk_notifications_source_user'),
  'DO 0',
  'CREATE UNIQUE INDEX `uk_notifications_source_user` ON `notifications` (`source_task_id`, `user_id`)'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='notification_task' AND index_name='idx_notification_task_claim'),
  'DO 0',
  'CREATE INDEX `idx_notification_task_claim` ON `notification_task` (`status`, `is_del`, `send_at`, `claim_expires_at`, `id`)'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;

SET @p02_sql := IF(
  EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='export_tasks' AND index_name='idx_export_task_claim'),
  'DO 0',
  'CREATE INDEX `idx_export_task_claim` ON `export_tasks` (`status`, `is_del`, `claim_expires_at`, `id`)'
);
PREPARE p02_stmt FROM @p02_sql; EXECUTE p02_stmt; DEALLOCATE PREPARE p02_stmt;
