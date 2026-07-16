SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS `canvas_video_tasks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `model_id` VARCHAR(191) NOT NULL DEFAULT '',
  `prompt` TEXT NOT NULL,
  `duration_seconds` INT NOT NULL DEFAULT 0,
  `size` VARCHAR(64) NOT NULL DEFAULT '',
  `resolution_name` VARCHAR(64) NOT NULL DEFAULT '',
  `provider_task_id` VARCHAR(191) NOT NULL DEFAULT '',
  `status` VARCHAR(32) NOT NULL DEFAULT 'pending',
  `error_message` VARCHAR(1024) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `finished_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  KEY `idx_canvas_video_tasks_user_status` (`user_id`, `status`, `is_del`, `created_at`, `id`),
  KEY `idx_canvas_video_tasks_provider_task` (`provider_id`, `provider_task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='无限画布视频生成任务';

UPDATE `ai_agents`
SET `scenes_json` = JSON_ARRAY_APPEND(CAST(`scenes_json` AS JSON), '$', 'canvas_text_generate'),
    `updated_at` = NOW()
WHERE `is_del` = 2
  AND JSON_VALID(`scenes_json`)
  AND JSON_CONTAINS(CAST(`scenes_json` AS JSON), JSON_QUOTE('chat'))
  AND NOT JSON_CONTAINS(CAST(`scenes_json` AS JSON), JSON_QUOTE('canvas_text_generate'));

UPDATE `ai_agents`
SET `scenes_json` = JSON_ARRAY_APPEND(CAST(`scenes_json` AS JSON), '$', 'canvas_image_generate'),
    `updated_at` = NOW()
WHERE `is_del` = 2
  AND JSON_VALID(`scenes_json`)
  AND JSON_CONTAINS(CAST(`scenes_json` AS JSON), JSON_QUOTE('image_generate'))
  AND NOT JSON_CONTAINS(CAST(`scenes_json` AS JSON), JSON_QUOTE('canvas_image_generate'));

UPDATE `permissions`
SET `is_del` = 1,
    `updated_at` = NOW()
WHERE `code` = 'ai_billing_rule_edit';

SET @ai_image_tasks_has_billing_record_idx := (
  SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_tasks'
    AND INDEX_NAME = 'idx_ai_image_tasks_billing_record_id'
);
SET @ai_image_tasks_drop_billing_record_idx_sql := IF(
  @ai_image_tasks_has_billing_record_idx > 0,
  'ALTER TABLE `ai_image_tasks` DROP INDEX `idx_ai_image_tasks_billing_record_id`',
  'SELECT 1'
);
PREPARE ai_image_tasks_drop_billing_record_idx_stmt FROM @ai_image_tasks_drop_billing_record_idx_sql;
EXECUTE ai_image_tasks_drop_billing_record_idx_stmt;
DEALLOCATE PREPARE ai_image_tasks_drop_billing_record_idx_stmt;

SET @ai_image_tasks_has_billing_record_id := (
  SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_tasks'
    AND COLUMN_NAME = 'billing_record_id'
);
SET @ai_image_tasks_drop_billing_record_id_sql := IF(
  @ai_image_tasks_has_billing_record_id > 0,
  'ALTER TABLE `ai_image_tasks` DROP COLUMN `billing_record_id`',
  'SELECT 1'
);
PREPARE ai_image_tasks_drop_billing_record_id_stmt FROM @ai_image_tasks_drop_billing_record_id_sql;
EXECUTE ai_image_tasks_drop_billing_record_id_stmt;
DEALLOCATE PREPARE ai_image_tasks_drop_billing_record_id_stmt;

DROP TABLE IF EXISTS `ai_billing_records`;
DROP TABLE IF EXISTS `ai_billing_rules`;


