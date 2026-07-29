-- Install the final official-model schema and Admin RBAC definitions.
-- Runtime startup never executes this migration.
DROP TEMPORARY TABLE IF EXISTS `_ai_official_model_guard`;
CREATE TEMPORARY TABLE `_ai_official_model_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

-- This release is intentionally a clean schema transition. The legacy price
-- tables are not copied or kept as compatibility aliases.
DROP TABLE IF EXISTS `ai_model_price_override_rates`;
DROP TABLE IF EXISTS `ai_model_price_overrides`;

INSERT INTO `_ai_official_model_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN (
    'ai_official_model_price_overrides',
    'ai_official_model_price_override_rates'
  );

INSERT INTO `_ai_official_model_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(
    permission.`id` = 5
    AND permission.`name` = 'AI助手'
    AND permission.`parent_id` = 0
    AND permission.`platform` = 'admin'
    AND permission.`type` = 1
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  ) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 5;

INSERT INTO `_ai_official_model_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `id` IN (921, 922)
   OR `code` IN ('ai_official_model_list', 'ai_official_model_price_sync');

CREATE TABLE `ai_official_model_price_overrides` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `catalog_vendor` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `model_id` VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `version` BIGINT UNSIGNED NOT NULL,
  `source_url` VARCHAR(2048) NOT NULL,
  `verified_at` DATE NOT NULL,
  `updated_by` INT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_official_model_price_overrides_identity` (`catalog_vendor`, `model_id`),
  CONSTRAINT `chk_ai_official_model_price_overrides_version` CHECK (`version` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `ai_official_model_price_override_rates` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `override_id` BIGINT UNSIGNED NOT NULL,
  `category` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `unit` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `tier_key` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  `price_units` BIGINT NOT NULL,
  `unit_scale` BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_official_model_price_override_rates_key` (`override_id`, `category`, `unit`, `tier_key`),
  CONSTRAINT `fk_ai_official_model_price_override_rates_override`
    FOREIGN KEY (`override_id`) REFERENCES `ai_official_model_price_overrides` (`id`)
    ON UPDATE RESTRICT ON DELETE CASCADE,
  CONSTRAINT `chk_ai_official_model_price_override_rates_category`
    CHECK (`category` IN ('input', 'output', 'cache_read', 'cache_write', 'media')),
  CONSTRAINT `chk_ai_official_model_price_override_rates_unit` CHECK (CHAR_LENGTH(TRIM(`unit`)) > 0),
  CONSTRAINT `chk_ai_official_model_price_override_rates_price` CHECK (`price_units` >= 0),
  CONSTRAINT `chk_ai_official_model_price_override_rates_scale` CHECK (`unit_scale` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE `ai_provider_models`
  ADD COLUMN `official_model_id` VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER `display_name`,
  ADD COLUMN `official_catalog_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER `official_model_id`,
  ADD COLUMN `mapping_status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'unmapped' AFTER `official_catalog_version`,
  ADD COLUMN `mapped_at` DATETIME(6) NULL AFTER `mapping_status`,
  ADD KEY `idx_ai_provider_models_official_mapping` (`mapping_status`, `official_model_id`, `status`),
  ADD CONSTRAINT `chk_ai_provider_models_mapping_status`
    CHECK (`mapping_status` IN ('mapped','unmapped')),
  ADD CONSTRAINT `chk_ai_provider_models_mapping`
    CHECK (
      (`mapping_status` = 'mapped'
        AND `official_model_id` IS NOT NULL
        AND `official_catalog_version` IS NOT NULL
        AND `mapped_at` IS NOT NULL)
      OR
      (`mapping_status` = 'unmapped'
        AND `official_model_id` IS NULL
        AND `official_catalog_version` IS NULL
        AND `mapped_at` IS NULL)
    );

ALTER TABLE `ai_agents`
  DROP COLUMN `max_output_tokens`;

START TRANSACTION;

SELECT permission.`id`
FROM `permissions` AS permission
WHERE permission.`id` IN (5, 921, 922)
   OR permission.`code` IN ('ai_official_model_list', 'ai_official_model_price_sync')
FOR UPDATE;

INSERT INTO `_ai_official_model_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(
    permission.`id` = 5
    AND permission.`name` = 'AI助手'
    AND permission.`parent_id` = 0
    AND permission.`platform` = 'admin'
    AND permission.`type` = 1
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  ) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 5;

INSERT INTO `_ai_official_model_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `id` IN (921, 922)
   OR `code` IN ('ai_official_model_list', 'ai_official_model_price_sync');

INSERT INTO `permissions`
  (`id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
VALUES
  (921, '官方模型', '/ai/official-models', '', 5, 'ai/official-models', 'admin', 2, 7, 'ai_official_model_list', 'menu.ai_official_models', 1, 1, 2),
  (922, '同步官方模型价格', '', '', 921, NULL, 'admin', 3, 1, 'ai_official_model_price_sync', '', 2, 1, 2);

COMMIT;

DROP TEMPORARY TABLE `_ai_official_model_guard`;
