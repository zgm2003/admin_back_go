-- OpenAI-only provider config slice.
-- ai_providers is the canonical provider table; ai_provider_models is the current selectable model snapshot.

ALTER TABLE `ai_providers`
  MODIFY `engine_type` VARCHAR(32) NOT NULL,
  MODIFY `base_url` VARCHAR(512) NOT NULL DEFAULT '';

ALTER TABLE `ai_providers`
  ADD COLUMN `last_check_error` VARCHAR(1024) NOT NULL DEFAULT '' AFTER `last_checked_at`,
  ADD COLUMN `last_model_sync_at` DATETIME NULL AFTER `last_check_error`,
  ADD COLUMN `last_model_sync_status` VARCHAR(32) NOT NULL DEFAULT 'unknown' AFTER `last_model_sync_at`,
  ADD COLUMN `last_model_sync_error` VARCHAR(1024) NOT NULL DEFAULT '' AFTER `last_model_sync_status`;

CREATE TABLE IF NOT EXISTS `ai_provider_models` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `model_id` VARCHAR(191) NOT NULL,
  `display_name` VARCHAR(191) NOT NULL DEFAULT '',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_provider_models_provider_model` (`provider_id`, `model_id`),
  KEY `idx_ai_provider_models_provider_status` (`provider_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI provider selectable model snapshot';

UPDATE `permissions`
SET `sort` = CASE `i18n_key`
  WHEN 'menu.ai_providers' THEN 1
  WHEN 'menu.ai_agents' THEN 2
  WHEN 'menu.ai_knowledge' THEN 3
  WHEN 'menu.ai_tools' THEN 4
  WHEN 'menu.ai_runs' THEN 5
  WHEN 'menu.ai_chat' THEN 6
  ELSE `sort`
END,
`updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `is_del` = 2
  AND `i18n_key` IN (
    'menu.ai_providers',
    'menu.ai_agents',
    'menu.ai_knowledge',
    'menu.ai_tools',
    'menu.ai_runs',
    'menu.ai_chat'
  );
