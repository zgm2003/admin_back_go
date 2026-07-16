-- Move prompt and asset ownership from Canvas tables to AI capability tables.
-- This migration is intentionally non-destructive: old Canvas tables stay available during the route ownership transition.

CREATE TABLE IF NOT EXISTS `ai_prompts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `slug` VARCHAR(191) NOT NULL,
  `category` VARCHAR(191) NOT NULL DEFAULT '',
  `title` VARCHAR(191) NOT NULL,
  `cover_url` VARCHAR(1024) NOT NULL DEFAULT '',
  `prompt` TEXT NOT NULL,
  `preview` VARCHAR(512) NOT NULL DEFAULT '',
  `tags_json` JSON NULL,
  `source_url` VARCHAR(1024) NOT NULL DEFAULT '',
  `status` TINYINT NOT NULL DEFAULT 1,
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_prompts_slug` (`slug`),
  KEY `idx_ai_prompts_category_status` (`category`, `status`, `is_del`, `updated_at`, `id`),
  KEY `idx_ai_prompts_status_updated` (`status`, `is_del`, `updated_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='AI提示词库';

CREATE TABLE IF NOT EXISTS `ai_assets` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `slug` VARCHAR(191) NOT NULL,
  `type` VARCHAR(16) NOT NULL,
  `category` VARCHAR(191) NOT NULL DEFAULT '',
  `title` VARCHAR(191) NOT NULL,
  `cover_url` VARCHAR(1024) NOT NULL DEFAULT '',
  `description` VARCHAR(512) NOT NULL DEFAULT '',
  `content` TEXT NULL,
  `url` VARCHAR(1024) NOT NULL DEFAULT '',
  `tags_json` JSON NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_assets_slug` (`slug`),
  KEY `idx_ai_assets_type_status` (`type`, `status`, `is_del`, `updated_at`, `id`),
  KEY `idx_ai_assets_status_updated` (`status`, `is_del`, `updated_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='AI素材库';

INSERT IGNORE INTO `ai_prompts` (
  `id`, `slug`, `category`, `title`, `cover_url`, `prompt`, `preview`, `tags_json`, `source_url`, `status`, `is_del`, `created_at`, `updated_at`
)
SELECT
  `id`, `slug`, `category`, `title`, `cover_url`, `prompt`, `preview`, `tags_json`, `source_url`, `status`, `is_del`, `created_at`, `updated_at`
FROM `canvas_prompts`;

INSERT IGNORE INTO `ai_assets` (
  `id`, `slug`, `type`, `category`, `title`, `cover_url`, `description`, `content`, `url`, `tags_json`, `status`, `is_del`, `created_at`, `updated_at`
)
SELECT
  `id`, `slug`, `type`, `category`, `title`, `cover_url`, `description`, `content`, `url`, `tags_json`, `status`, `is_del`, `created_at`, `updated_at`
FROM `canvas_assets`;

-- Admin AI prompt/asset menu and button permissions.
SET @ai_parent_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND (`code` = 'ai' OR `path` = '/ai' OR `i18n_key` = 'menu.ai')
  ORDER BY `id`
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '提示词库', '/ai/prompts', 'FileText', @ai_parent_id, 'ai/prompts', 'admin', 2, 8, 'ai_prompt_page', 'menu.ai_prompts', 1, 1, 2
WHERE @ai_parent_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `path` = VALUES(`path`),
  `icon` = VALUES(`icon`),
  `parent_id` = VALUES(`parent_id`),
  `component` = VALUES(`component`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `i18n_key` = VALUES(`i18n_key`),
  `show_menu` = VALUES(`show_menu`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '素材库', '/ai/assets', 'Files', @ai_parent_id, 'ai/assets', 'admin', 2, 9, 'ai_asset_page', 'menu.ai_assets', 1, 1, 2
WHERE @ai_parent_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `path` = VALUES(`path`),
  `icon` = VALUES(`icon`),
  `parent_id` = VALUES(`parent_id`),
  `component` = VALUES(`component`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `i18n_key` = VALUES(`i18n_key`),
  `show_menu` = VALUES(`show_menu`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

SET @ai_prompt_page_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'ai_prompt_page'
    AND `is_del` = 2
  LIMIT 1
);

SET @ai_asset_page_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'ai_asset_page'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @ai_prompt_page_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '新增提示词' AS button_name, 'ai_prompt_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '编辑提示词', 'ai_prompt_edit', 2
  UNION ALL SELECT '修改提示词状态', 'ai_prompt_status', 3
  UNION ALL SELECT '删除提示词', 'ai_prompt_del', 4
) AS ai_prompt_buttons
WHERE @ai_prompt_page_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = VALUES(`show_menu`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @ai_asset_page_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '新增素材' AS button_name, 'ai_asset_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '编辑素材', 'ai_asset_edit', 2
  UNION ALL SELECT '删除素材', 'ai_asset_del', 3
) AS ai_asset_buttons
WHERE @ai_asset_page_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = VALUES(`show_menu`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_ai_prompt_asset_permission_grant_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_ai_prompt_asset_permission_grant_roles`;

INSERT IGNORE INTO `tmp_ai_prompt_asset_permission_grant_roles` (`role_id`)
SELECT DISTINCT rp.`role_id`
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
JOIN `roles` r ON r.`id` = rp.`role_id`
WHERE rp.`is_del` = 2
  AND p.`is_del` = 2
  AND r.`is_del` = 2
  AND p.`platform` = 'admin'
  AND p.`code` IN ('ai_agent_add', 'ai_provider_add', 'ai_tool_add', 'ai_knowledge_add', 'ai_image_task_add', 'ai_chat');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT gr.`role_id`, p.`id`, 2
FROM `tmp_ai_prompt_asset_permission_grant_roles` gr
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN (
    'ai_prompt_page',
    'ai_prompt_add',
    'ai_prompt_edit',
    'ai_prompt_status',
    'ai_prompt_del',
    'ai_asset_page',
    'ai_asset_add',
    'ai_asset_edit',
    'ai_asset_del'
  )
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prompt_asset_permission_grant_roles`;
