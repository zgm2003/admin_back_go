-- Unified AI run records.
-- Scope: record real provider attempts for chat/text/image/video without reviving AI billing.

ALTER TABLE `ai_runs`
  ADD COLUMN `platform` VARCHAR(32) NULL AFTER `id`,
  ADD COLUMN `modality` VARCHAR(32) NULL AFTER `platform`,
  ADD COLUMN `source_type` VARCHAR(64) NULL AFTER `modality`,
  ADD COLUMN `source_id` BIGINT UNSIGNED NULL AFTER `source_type`,
  ADD COLUMN `input_snapshot` MEDIUMTEXT NULL AFTER `model_display_name`,
  ADD COLUMN `usage_status` VARCHAR(16) NULL AFTER `total_tokens`;

UPDATE `ai_runs` r
JOIN `ai_messages` m ON m.id = r.user_message_id
SET
  r.`platform` = 'admin',
  r.`modality` = 'chat',
  r.`source_type` = 'ai_chat_message',
  r.`source_id` = r.`user_message_id`,
  r.`input_snapshot` = m.`content`,
  -- Stored token counts are the only old-row evidence of provider usage; do not infer usage from prompt text or model name.
  r.`usage_status` = IF((r.`prompt_tokens` + r.`completion_tokens` + r.`total_tokens`) > 0, 'reported', 'unavailable');

ALTER TABLE `ai_runs`
  MODIFY COLUMN `platform` VARCHAR(32) NOT NULL,
  MODIFY COLUMN `modality` VARCHAR(32) NOT NULL,
  MODIFY COLUMN `source_type` VARCHAR(64) NOT NULL,
  MODIFY COLUMN `source_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `input_snapshot` MEDIUMTEXT NOT NULL,
  MODIFY COLUMN `usage_status` VARCHAR(16) NOT NULL,
  MODIFY COLUMN `conversation_id` INT UNSIGNED NULL COMMENT 'ai_conversations.id; chat rows only',
  MODIFY COLUMN `user_message_id` BIGINT UNSIGNED NULL COMMENT '本轮用户消息ID; chat rows only',
  MODIFY COLUMN `assistant_message_id` BIGINT UNSIGNED NULL COMMENT '完成后写入的助手消息ID; chat rows only';

CREATE INDEX `idx_ai_runs_platform_modality_created` ON `ai_runs` (`platform`, `modality`, `created_at`, `id`);
CREATE INDEX `idx_ai_runs_source` ON `ai_runs` (`source_type`, `source_id`, `created_at`, `id`);
CREATE UNIQUE INDEX `uk_ai_runs_source_request` ON `ai_runs` (`source_type`, `source_id`, `request_id`);

CREATE TABLE `ai_text_tasks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `platform` VARCHAR(32) NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `model_id` VARCHAR(191) NOT NULL,
  `prompt` MEDIUMTEXT NOT NULL,
  `answer` MEDIUMTEXT NULL,
  `status` VARCHAR(16) NOT NULL,
  `error_message` VARCHAR(1024) NULL,
  `started_at` DATETIME NULL,
  `finished_at` DATETIME NULL,
  `elapsed_ms` INT UNSIGNED NOT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ai_text_tasks_user_created` (`user_id`, `created_at`, `id`),
  KEY `idx_ai_text_tasks_status_created` (`status`, `created_at`, `id`),
  CONSTRAINT `chk_ai_text_tasks_status` CHECK (`status` IN ('running', 'success', 'failed'))
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI文本生成任务';

ALTER TABLE `ai_image_tasks` ADD COLUMN `platform` VARCHAR(32) NULL AFTER `id`;
UPDATE `ai_image_tasks` SET `platform` = 'admin' WHERE `platform` IS NULL;
ALTER TABLE `ai_image_tasks` MODIFY COLUMN `platform` VARCHAR(32) NOT NULL;
CREATE INDEX `idx_ai_image_tasks_platform_created` ON `ai_image_tasks` (`platform`, `created_at`, `id`);

ALTER TABLE `ai_runs`
  ADD CONSTRAINT `chk_ai_runs_usage_status` CHECK (`usage_status` IN ('pending', 'reported', 'unavailable'));
