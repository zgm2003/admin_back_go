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
