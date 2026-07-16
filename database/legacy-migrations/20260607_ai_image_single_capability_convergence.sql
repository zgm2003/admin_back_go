-- Converge Admin and Canvas image generation into one AI image capability.
-- External routes stay /api/admin/v1/ai-images and /api/canvas/v1/ai/images/*.
-- Retired split/global image tables are copied into ai_image_tasks / ai_image_files before being dropped.

CREATE TABLE IF NOT EXISTS `ai_image_tasks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `platform` varchar(32) NOT NULL COMMENT 'admin/canvas platform surface',
  `user_id` bigint unsigned NOT NULL,
  `agent_id` bigint unsigned NOT NULL,
  `agent_name_snapshot` varchar(128) NOT NULL DEFAULT '',
  `provider_id_snapshot` bigint unsigned NOT NULL DEFAULT 0,
  `provider_name_snapshot` varchar(128) NOT NULL DEFAULT '',
  `model_id_snapshot` varchar(128) NOT NULL DEFAULT '',
  `model_display_name_snapshot` varchar(128) NOT NULL DEFAULT '',
  `prompt` text NOT NULL,
  `size` varchar(32) NOT NULL DEFAULT '1024x1024',
  `quality` varchar(16) NOT NULL DEFAULT 'auto',
  `output_format` varchar(16) NOT NULL DEFAULT 'png',
  `output_compression` int DEFAULT NULL,
  `moderation` varchar(16) NOT NULL DEFAULT 'auto',
  `n` int NOT NULL DEFAULT 1,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `error_message` varchar(1000) NOT NULL DEFAULT '',
  `actual_params_json` json DEFAULT NULL,
  `raw_response_json` json DEFAULT NULL,
  `is_favorite` tinyint NOT NULL DEFAULT 2,
  `finished_at` datetime DEFAULT NULL,
  `elapsed_ms` int NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_ai_image_tasks_platform_user_created` (`platform`,`user_id`,`created_at`),
  KEY `idx_ai_image_tasks_platform_status_created` (`platform`,`status`,`created_at`),
  KEY `idx_ai_image_tasks_agent_created` (`agent_id`,`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_image_files` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_id` bigint unsigned NOT NULL,
  `role` varchar(16) NOT NULL COMMENT 'input/mask/output',
  `sort_order` int NOT NULL DEFAULT 0,
  `storage_provider` varchar(32) NOT NULL DEFAULT '',
  `storage_key` varchar(512) NOT NULL DEFAULT '',
  `storage_url` varchar(1000) NOT NULL DEFAULT '',
  `mime_type` varchar(64) NOT NULL DEFAULT '',
  `width` int NOT NULL DEFAULT 0,
  `height` int NOT NULL DEFAULT 0,
  `size_bytes` bigint NOT NULL DEFAULT 0,
  `related_file_id` bigint unsigned DEFAULT NULL,
  `revised_prompt` text DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_ai_image_files_task_role_sort` (`task_id`,`role`,`sort_order`),
  KEY `idx_ai_image_files_related` (`related_file_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @ai_image_tasks_has_platform := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_tasks'
    AND COLUMN_NAME = 'platform'
);

SET @ai_image_tasks_add_platform_sql := IF(
  @ai_image_tasks_has_platform = 0,
  'ALTER TABLE `ai_image_tasks` ADD COLUMN `platform` VARCHAR(32) NULL AFTER `id`',
  'SELECT 1'
);
PREPARE ai_image_tasks_add_platform_stmt FROM @ai_image_tasks_add_platform_sql;
EXECUTE ai_image_tasks_add_platform_stmt;
DEALLOCATE PREPARE ai_image_tasks_add_platform_stmt;

UPDATE `ai_image_tasks`
SET `platform` = 'admin'
WHERE `platform` IS NULL OR `platform` = '';

ALTER TABLE `ai_image_tasks`
  MODIFY COLUMN `platform` VARCHAR(32) NOT NULL;

SET @ai_image_tasks_has_is_del := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_tasks'
    AND COLUMN_NAME = 'is_del'
);

SET @ai_image_tasks_delete_soft_deleted_sql := IF(
  @ai_image_tasks_has_is_del > 0,
  'DELETE FROM `ai_image_tasks` WHERE `is_del` = 1',
  'SELECT 1'
);
PREPARE ai_image_tasks_delete_soft_deleted_stmt FROM @ai_image_tasks_delete_soft_deleted_sql;
EXECUTE ai_image_tasks_delete_soft_deleted_stmt;
DEALLOCATE PREPARE ai_image_tasks_delete_soft_deleted_stmt;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_image_task_map`;
CREATE TEMPORARY TABLE `tmp_ai_image_task_map` (
  `new_task_id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `platform` varchar(32) NOT NULL,
  `old_task_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `agent_id` bigint unsigned NOT NULL,
  `agent_name_snapshot` varchar(128) NOT NULL DEFAULT '',
  `provider_id_snapshot` bigint unsigned NOT NULL DEFAULT 0,
  `provider_name_snapshot` varchar(128) NOT NULL DEFAULT '',
  `model_id_snapshot` varchar(128) NOT NULL DEFAULT '',
  `model_display_name_snapshot` varchar(128) NOT NULL DEFAULT '',
  `prompt` text NOT NULL,
  `size` varchar(32) NOT NULL DEFAULT '1024x1024',
  `quality` varchar(16) NOT NULL DEFAULT 'auto',
  `output_format` varchar(16) NOT NULL DEFAULT 'png',
  `output_compression` int DEFAULT NULL,
  `moderation` varchar(16) NOT NULL DEFAULT 'auto',
  `n` int NOT NULL DEFAULT 1,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `error_message` varchar(1000) NOT NULL DEFAULT '',
  `actual_params_json` json DEFAULT NULL,
  `raw_response_json` json DEFAULT NULL,
  `is_favorite` tinyint NOT NULL DEFAULT 2,
  `finished_at` datetime DEFAULT NULL,
  `elapsed_ms` int NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`new_task_id`),
  UNIQUE KEY `uk_tmp_ai_image_task_old` (`platform`,`old_task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @next_ai_image_task_id := (SELECT COALESCE(MAX(`id`), 0) + 1 FROM `ai_image_tasks`);
SET @tmp_ai_image_task_auto_sql := CONCAT('ALTER TABLE `tmp_ai_image_task_map` AUTO_INCREMENT = ', @next_ai_image_task_id);
PREPARE tmp_ai_image_task_auto_stmt FROM @tmp_ai_image_task_auto_sql;
EXECUTE tmp_ai_image_task_auto_stmt;
DEALLOCATE PREPARE tmp_ai_image_task_auto_stmt;

SET @admin_ai_image_tasks_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'admin_ai_image_tasks'
);

SET @insert_admin_ai_image_tasks_sql := IF(
  @admin_ai_image_tasks_exists > 0,
  'INSERT IGNORE INTO `tmp_ai_image_task_map` (`platform`,`old_task_id`,`user_id`,`agent_id`,`agent_name_snapshot`,`provider_id_snapshot`,`provider_name_snapshot`,`model_id_snapshot`,`model_display_name_snapshot`,`prompt`,`size`,`quality`,`output_format`,`output_compression`,`moderation`,`n`,`status`,`error_message`,`actual_params_json`,`raw_response_json`,`is_favorite`,`finished_at`,`elapsed_ms`,`created_at`,`updated_at`) SELECT ''admin'', `id`, `user_id`, `agent_id`, `agent_name_snapshot`, `provider_id_snapshot`, `provider_name_snapshot`, `model_id_snapshot`, `model_display_name_snapshot`, `prompt`, `size`, `quality`, `output_format`, `output_compression`, `moderation`, `n`, `status`, `error_message`, `actual_params_json`, `raw_response_json`, `is_favorite`, `finished_at`, `elapsed_ms`, `created_at`, `updated_at` FROM `admin_ai_image_tasks`',
  'SELECT 1'
);
PREPARE insert_admin_ai_image_tasks_stmt FROM @insert_admin_ai_image_tasks_sql;
EXECUTE insert_admin_ai_image_tasks_stmt;
DEALLOCATE PREPARE insert_admin_ai_image_tasks_stmt;

SET @canvas_image_tasks_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'canvas_image_tasks'
);

SET @insert_canvas_image_tasks_sql := IF(
  @canvas_image_tasks_exists > 0,
  'INSERT IGNORE INTO `tmp_ai_image_task_map` (`platform`,`old_task_id`,`user_id`,`agent_id`,`agent_name_snapshot`,`provider_id_snapshot`,`provider_name_snapshot`,`model_id_snapshot`,`model_display_name_snapshot`,`prompt`,`size`,`quality`,`output_format`,`output_compression`,`moderation`,`n`,`status`,`error_message`,`actual_params_json`,`raw_response_json`,`is_favorite`,`finished_at`,`elapsed_ms`,`created_at`,`updated_at`) SELECT ''canvas'', `id`, `user_id`, `agent_id`, `agent_name_snapshot`, `provider_id_snapshot`, `provider_name_snapshot`, `model_id_snapshot`, `model_display_name_snapshot`, `prompt`, `size`, `quality`, `output_format`, `output_compression`, `moderation`, `n`, `status`, `error_message`, `actual_params_json`, `raw_response_json`, 2, `finished_at`, `elapsed_ms`, `created_at`, `updated_at` FROM `canvas_image_tasks`',
  'SELECT 1'
);
PREPARE insert_canvas_image_tasks_stmt FROM @insert_canvas_image_tasks_sql;
EXECUTE insert_canvas_image_tasks_stmt;
DEALLOCATE PREPARE insert_canvas_image_tasks_stmt;

INSERT IGNORE INTO `ai_image_tasks` (
  `id`, `platform`, `user_id`, `agent_id`, `agent_name_snapshot`, `provider_id_snapshot`,
  `provider_name_snapshot`, `model_id_snapshot`, `model_display_name_snapshot`, `prompt`,
  `size`, `quality`, `output_format`, `output_compression`, `moderation`, `n`, `status`,
  `error_message`, `actual_params_json`, `raw_response_json`, `is_favorite`, `finished_at`,
  `elapsed_ms`, `created_at`, `updated_at`
)
SELECT
  `new_task_id`, `platform`, `user_id`, `agent_id`, `agent_name_snapshot`, `provider_id_snapshot`,
  `provider_name_snapshot`, `model_id_snapshot`, `model_display_name_snapshot`, `prompt`,
  `size`, `quality`, `output_format`, `output_compression`, `moderation`, `n`, `status`,
  `error_message`, `actual_params_json`, `raw_response_json`, `is_favorite`, `finished_at`,
  `elapsed_ms`, `created_at`, `updated_at`
FROM `tmp_ai_image_task_map`;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_image_file_map`;
CREATE TEMPORARY TABLE `tmp_ai_image_file_map` (
  `new_file_id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `platform` varchar(32) NOT NULL,
  `old_file_id` bigint unsigned NOT NULL,
  `old_task_id` bigint unsigned NOT NULL,
  `old_related_file_id` bigint unsigned DEFAULT NULL,
  `old_asset_id` bigint unsigned DEFAULT NULL,
  `old_related_asset_id` bigint unsigned DEFAULT NULL,
  `new_task_id` bigint unsigned NOT NULL,
  `role` varchar(16) NOT NULL,
  `sort_order` int NOT NULL DEFAULT 0,
  `storage_provider` varchar(32) NOT NULL DEFAULT '',
  `storage_key` varchar(512) NOT NULL DEFAULT '',
  `storage_url` varchar(1000) NOT NULL DEFAULT '',
  `mime_type` varchar(64) NOT NULL DEFAULT '',
  `width` int NOT NULL DEFAULT 0,
  `height` int NOT NULL DEFAULT 0,
  `size_bytes` bigint NOT NULL DEFAULT 0,
  `revised_prompt` text DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`new_file_id`),
  UNIQUE KEY `uk_tmp_ai_image_file_old` (`platform`,`old_file_id`),
  KEY `idx_tmp_ai_image_file_related_file` (`platform`,`old_related_file_id`),
  KEY `idx_tmp_ai_image_file_related_asset` (`platform`,`old_task_id`,`old_asset_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @next_ai_image_file_id := (SELECT COALESCE(MAX(`id`), 0) + 1 FROM `ai_image_files`);
SET @tmp_ai_image_file_auto_sql := CONCAT('ALTER TABLE `tmp_ai_image_file_map` AUTO_INCREMENT = ', @next_ai_image_file_id);
PREPARE tmp_ai_image_file_auto_stmt FROM @tmp_ai_image_file_auto_sql;
EXECUTE tmp_ai_image_file_auto_stmt;
DEALLOCATE PREPARE tmp_ai_image_file_auto_stmt;

SET @admin_ai_image_files_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'admin_ai_image_files'
);

SET @insert_admin_ai_image_files_sql := IF(
  @admin_ai_image_files_exists > 0,
  'INSERT IGNORE INTO `tmp_ai_image_file_map` (`platform`,`old_file_id`,`old_task_id`,`old_related_file_id`,`new_task_id`,`role`,`sort_order`,`storage_provider`,`storage_key`,`storage_url`,`mime_type`,`width`,`height`,`size_bytes`,`revised_prompt`,`created_at`) SELECT ''admin'', f.`id`, f.`task_id`, f.`related_file_id`, m.`new_task_id`, f.`role`, f.`sort_order`, f.`storage_provider`, f.`storage_key`, f.`storage_url`, f.`mime_type`, f.`width`, f.`height`, f.`size_bytes`, f.`revised_prompt`, f.`created_at` FROM `admin_ai_image_files` f JOIN `tmp_ai_image_task_map` m ON m.`platform` = ''admin'' AND m.`old_task_id` = f.`task_id`',
  'SELECT 1'
);
PREPARE insert_admin_ai_image_files_stmt FROM @insert_admin_ai_image_files_sql;
EXECUTE insert_admin_ai_image_files_stmt;
DEALLOCATE PREPARE insert_admin_ai_image_files_stmt;

SET @canvas_image_files_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'canvas_image_files'
);

SET @insert_canvas_image_files_sql := IF(
  @canvas_image_files_exists > 0,
  'INSERT IGNORE INTO `tmp_ai_image_file_map` (`platform`,`old_file_id`,`old_task_id`,`old_related_file_id`,`new_task_id`,`role`,`sort_order`,`storage_provider`,`storage_key`,`storage_url`,`mime_type`,`width`,`height`,`size_bytes`,`revised_prompt`,`created_at`) SELECT ''canvas'', f.`id`, f.`task_id`, NULL, m.`new_task_id`, f.`role`, f.`sort_order`, f.`storage_provider`, f.`storage_key`, f.`storage_url`, f.`mime_type`, f.`width`, f.`height`, f.`size_bytes`, f.`revised_prompt`, f.`created_at` FROM `canvas_image_files` f JOIN `tmp_ai_image_task_map` m ON m.`platform` = ''canvas'' AND m.`old_task_id` = f.`task_id`',
  'SELECT 1'
);
PREPARE insert_canvas_image_files_stmt FROM @insert_canvas_image_files_sql;
EXECUTE insert_canvas_image_files_stmt;
DEALLOCATE PREPARE insert_canvas_image_files_stmt;

SET @ai_image_task_assets_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_task_assets'
);

SET @ai_image_assets_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_assets'
);

SET @insert_legacy_ai_image_files_sql := IF(
  @ai_image_task_assets_exists > 0 AND @ai_image_assets_exists > 0,
  'INSERT IGNORE INTO `tmp_ai_image_file_map` (`platform`,`old_file_id`,`old_task_id`,`old_asset_id`,`old_related_asset_id`,`new_task_id`,`role`,`sort_order`,`storage_provider`,`storage_key`,`storage_url`,`mime_type`,`width`,`height`,`size_bytes`,`revised_prompt`,`created_at`) SELECT ''legacy'', rel.`id`, rel.`task_id`, rel.`asset_id`, rel.`related_asset_id`, rel.`task_id`, rel.`role`, rel.`sort_order`, asset.`storage_provider`, asset.`storage_key`, asset.`storage_url`, asset.`mime_type`, asset.`width`, asset.`height`, asset.`size_bytes`, rel.`revised_prompt`, rel.`created_at` FROM `ai_image_task_assets` rel JOIN `ai_image_assets` asset ON asset.`id` = rel.`asset_id` WHERE rel.`is_del` = 2 AND asset.`is_del` = 2',
  'SELECT 1'
);
PREPARE insert_legacy_ai_image_files_stmt FROM @insert_legacy_ai_image_files_sql;
EXECUTE insert_legacy_ai_image_files_stmt;
DEALLOCATE PREPARE insert_legacy_ai_image_files_stmt;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_image_file_related_map`;
CREATE TEMPORARY TABLE `tmp_ai_image_file_related_map` AS
SELECT
  `new_file_id`,
  `platform`,
  `old_file_id`,
  `old_task_id`,
  `old_asset_id`
FROM `tmp_ai_image_file_map`;

INSERT IGNORE INTO `ai_image_files` (
  `id`, `task_id`, `role`, `sort_order`, `storage_provider`, `storage_key`, `storage_url`,
  `mime_type`, `width`, `height`, `size_bytes`, `related_file_id`, `revised_prompt`, `created_at`
)
SELECT
  current_file.`new_file_id`, current_file.`new_task_id`, current_file.`role`, current_file.`sort_order`,
  current_file.`storage_provider`, current_file.`storage_key`, current_file.`storage_url`, current_file.`mime_type`,
  current_file.`width`, current_file.`height`, current_file.`size_bytes`, related_file.`new_file_id`,
  current_file.`revised_prompt`, current_file.`created_at`
FROM `tmp_ai_image_file_map` current_file
LEFT JOIN `tmp_ai_image_file_related_map` related_file
  ON related_file.`platform` = current_file.`platform`
 AND (
   (current_file.`old_related_file_id` IS NOT NULL AND related_file.`old_file_id` = current_file.`old_related_file_id`)
   OR (current_file.`old_related_asset_id` IS NOT NULL AND related_file.`old_task_id` = current_file.`old_task_id` AND related_file.`old_asset_id` = current_file.`old_related_asset_id`)
 );

UPDATE `ai_runs` r
JOIN `tmp_ai_image_task_map` m
  ON r.`source_id` = m.`old_task_id`
 AND (
   (r.`source_type` = 'admin_ai_image_task' AND m.`platform` = 'admin')
   OR (r.`source_type` = 'canvas_image_task' AND m.`platform` = 'canvas')
 )
SET r.`source_type` = 'ai_image_task',
    r.`source_id` = m.`new_task_id`;

UPDATE `ai_runs`
SET `source_type` = 'ai_image_task'
WHERE `source_type` IN ('admin_ai_image_task', 'canvas_image_task');

DROP TABLE IF EXISTS `admin_ai_image_files`;
DROP TABLE IF EXISTS `admin_ai_image_tasks`;
DROP TABLE IF EXISTS `canvas_image_files`;
DROP TABLE IF EXISTS `canvas_image_tasks`;
DROP TABLE IF EXISTS `ai_image_task_assets`;
DROP TABLE IF EXISTS `ai_image_assets`;
