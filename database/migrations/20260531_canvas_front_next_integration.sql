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
SELECT permission_name, '', '', 0, '', 'canvas', 3, permission_sort, permission_code, '', 2, 1, 2
FROM (
  SELECT '访问无限画布' AS permission_name, 10 AS permission_sort, 'canvas_access' AS permission_code
  UNION ALL SELECT '读取提示词库', 20, 'canvas_prompt_read'
  UNION ALL SELECT '读取素材库', 30, 'canvas_asset_read'
  UNION ALL SELECT '文本生成', 40, 'canvas_ai_text_generate'
  UNION ALL SELECT '图片生成', 50, 'canvas_ai_image_generate'
  UNION ALL SELECT '视频生成', 60, 'canvas_ai_video_generate'
  UNION ALL SELECT '读取钱包', 70, 'canvas_wallet_read'
  UNION ALL SELECT '创建充值', 80, 'canvas_recharge_add'
  UNION ALL SELECT '支付充值', 90, 'canvas_recharge_pay'
) AS canvas_permissions
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
