-- Canvas Front Next integration baseline.
-- Adds the canvas auth platform, canvas capability gates, and the only two
-- canvas-owned public-library tables used by the first Next.js frontend slice.

INSERT INTO `auth_platforms` (
  `code`, `name`, `login_types`, `captcha_type`,
  `access_ttl`, `refresh_ttl`,
  `bind_platform`, `bind_device`, `bind_ip`, `single_session`, `max_sessions`,
  `allow_register`, `status`, `is_del`
)
VALUES (
  'canvas', '无限画布', JSON_ARRAY('email', 'phone', 'password'), 'slide',
  14400, 1209600,
  1, 2, 2, 2, 5,
  1, 1, 2
)
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `login_types` = VALUES(`login_types`),
  `captcha_type` = VALUES(`captcha_type`),
  `allow_register` = VALUES(`allow_register`),
  `status` = VALUES(`status`),
  `is_del` = VALUES(`is_del`),
  `updated_at` = CURRENT_TIMESTAMP;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT page_name, page_path, page_icon, 0, page_component, 'canvas', 2, page_sort, page_code, page_i18n_key, page_show_menu, 1, 2
FROM (
  SELECT '我的画布' AS page_name, '/canvas' AS page_path, 'Maximize2' AS page_icon, 'canvas' AS page_component, 10 AS page_sort, 'canvas_page' AS page_code, 'menu.canvas' AS page_i18n_key, 1 AS page_show_menu
  UNION ALL SELECT '生图工作台', '/image', 'ImagePlus', 'image', 20, 'canvas_image_page', 'menu.canvas_image', 1
  UNION ALL SELECT '视频创作台', '/video', 'Video', 'video', 30, 'canvas_video_page', 'menu.canvas_video', 1
  UNION ALL SELECT '提示词库', '/prompts', 'FileText', 'prompts', 40, 'canvas_prompts_page', 'menu.canvas_prompts', 1
  UNION ALL SELECT '我的素材', '/assets', 'Images', 'assets', 50, 'canvas_assets_page', 'menu.canvas_assets', 1
  UNION ALL SELECT '个人资料', '/profile', 'UserRound', 'profile', 60, 'canvas_profile_page', 'menu.canvas_profile', 2
  UNION ALL SELECT '我的钱包', '/wallet', 'WalletCards', 'wallet', 70, 'canvas_wallet_page', 'menu.canvas_wallet', 2
) AS canvas_pages
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
  `is_del` = VALUES(`is_del`),
  `updated_at` = CURRENT_TIMESTAMP;

SET @canvas_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_page' AND `is_del` = 2
  LIMIT 1
);
SET @canvas_image_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_image_page' AND `is_del` = 2
  LIMIT 1
);
SET @canvas_video_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_video_page' AND `is_del` = 2
  LIMIT 1
);
SET @canvas_prompts_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_prompts_page' AND `is_del` = 2
  LIMIT 1
);
SET @canvas_assets_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_assets_page' AND `is_del` = 2
  LIMIT 1
);
SET @canvas_wallet_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_wallet_page' AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', parent_id, '', 'canvas', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '访问画布' AS button_name, @canvas_page_id AS parent_id, 10 AS button_sort, 'canvas_access' AS button_code
  UNION ALL SELECT '图片生成', @canvas_image_page_id, 10, 'canvas_ai_image_generate'
  UNION ALL SELECT '视频生成', @canvas_video_page_id, 10, 'canvas_ai_video_generate'
  UNION ALL SELECT '读取提示词库', @canvas_prompts_page_id, 10, 'canvas_prompt_read'
  UNION ALL SELECT '读取素材库', @canvas_assets_page_id, 10, 'canvas_asset_read'
  UNION ALL SELECT '读取钱包', @canvas_wallet_page_id, 10, 'canvas_wallet_read'
  UNION ALL SELECT '创建充值', @canvas_wallet_page_id, 20, 'canvas_recharge_add'
  UNION ALL SELECT '支付充值', @canvas_wallet_page_id, 30, 'canvas_recharge_pay'
) AS canvas_buttons
WHERE parent_id IS NOT NULL
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
  `is_del` = VALUES(`is_del`),
  `updated_at` = CURRENT_TIMESTAMP;

UPDATE `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = CURRENT_TIMESTAMP
WHERE p.`platform` = 'canvas'
  AND p.`type` = 3
  AND p.`code` NOT IN (
    'canvas_access',
    'canvas_prompt_read',
    'canvas_asset_read',
    'canvas_ai_image_generate',
    'canvas_ai_video_generate',
    'canvas_wallet_read',
    'canvas_recharge_add',
    'canvas_recharge_pay'
  )
  AND rp.`is_del` = 2;

UPDATE `permissions`
SET `status` = 2,
    `is_del` = 1,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'canvas'
  AND `type` = 3
  AND `code` NOT IN (
    'canvas_access',
    'canvas_prompt_read',
    'canvas_asset_read',
    'canvas_ai_image_generate',
    'canvas_ai_video_generate',
    'canvas_wallet_read',
    'canvas_recharge_add',
    'canvas_recharge_pay'
  )
  AND `is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT r.`id`, p.`id`, 2
FROM `roles` r
JOIN `permissions` p
  ON p.`platform` = 'canvas'
 AND p.`type` IN (2, 3)
 AND p.`is_del` = 2
 AND p.`status` = 1
LEFT JOIN `role_permissions` rp
  ON rp.`role_id` = r.`id`
 AND rp.`permission_id` = p.`id`
 AND rp.`is_del` = 2
WHERE r.`is_del` = 2
  AND rp.`id` IS NULL;

CREATE TABLE IF NOT EXISTS `canvas_prompts` (
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
  UNIQUE KEY `uk_canvas_prompts_slug` (`slug`),
  KEY `idx_canvas_prompts_category_status` (`category`, `status`, `is_del`, `updated_at`, `id`),
  KEY `idx_canvas_prompts_status_updated` (`status`, `is_del`, `updated_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='无限画布提示词公共库';

CREATE TABLE IF NOT EXISTS `canvas_assets` (
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
  UNIQUE KEY `uk_canvas_assets_slug` (`slug`),
  KEY `idx_canvas_assets_type_status` (`type`, `status`, `is_del`, `updated_at`, `id`),
  KEY `idx_canvas_assets_status_updated` (`status`, `is_del`, `updated_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='无限画布素材公共库';
