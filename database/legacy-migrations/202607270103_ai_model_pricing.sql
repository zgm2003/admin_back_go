-- Install canonical AI model price overrides and their Admin RBAC definitions.
-- Runtime startup never executes this migration.
DROP TEMPORARY TABLE IF EXISTS `_ai_model_pricing_guard`;
CREATE TEMPORARY TABLE `_ai_model_pricing_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

-- Target table names must be unused. A partially applied DDL migration is a
-- recovery event and must not be guessed through a blind rerun.
INSERT INTO `_ai_model_pricing_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN ('ai_model_price_overrides', 'ai_model_price_override_rates');

-- The AI parent menu is an immutable input to this migration.
INSERT INTO `_ai_model_pricing_guard`
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

-- Refuse both ID collisions and code collisions before persistent DDL.
INSERT INTO `_ai_model_pricing_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `id` IN (921, 922)
   OR `code` IN ('ai_model_pricing_list', 'ai_model_pricing_edit');

CREATE TABLE `ai_model_price_overrides` (
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
  UNIQUE KEY `uk_ai_model_price_overrides_identity` (`catalog_vendor`, `model_id`),
  CONSTRAINT `chk_ai_model_price_overrides_version` CHECK (`version` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `ai_model_price_override_rates` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `override_id` BIGINT UNSIGNED NOT NULL,
  `category` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `unit` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `tier_key` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  `price_units` BIGINT NOT NULL,
  `unit_scale` BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_model_price_override_rates_key` (`override_id`, `category`, `unit`, `tier_key`),
  CONSTRAINT `fk_ai_model_price_override_rates_override`
    FOREIGN KEY (`override_id`) REFERENCES `ai_model_price_overrides` (`id`)
    ON UPDATE RESTRICT ON DELETE CASCADE,
  CONSTRAINT `chk_ai_model_price_override_rates_category`
    CHECK (`category` IN ('input', 'output', 'cache_read', 'cache_write')),
  CONSTRAINT `chk_ai_model_price_override_rates_unit` CHECK (`unit` = 'token'),
  CONSTRAINT `chk_ai_model_price_override_rates_price` CHECK (`price_units` >= 0),
  CONSTRAINT `chk_ai_model_price_override_rates_scale` CHECK (`unit_scale` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

START TRANSACTION;

SELECT permission.`id`
FROM `permissions` AS permission
WHERE permission.`id` IN (5, 921, 922)
   OR permission.`code` IN ('ai_model_pricing_list', 'ai_model_pricing_edit')
FOR UPDATE;

-- Repeat mutable permission checks after acquiring row/range locks.
INSERT INTO `_ai_model_pricing_guard`
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

INSERT INTO `_ai_model_pricing_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `id` IN (921, 922)
   OR `code` IN ('ai_model_pricing_list', 'ai_model_pricing_edit');

INSERT INTO `permissions`
  (`id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
VALUES
  (921, '模型定价', '/ai/model-pricing', '', 5, 'ai/model-pricing', 'admin', 2, 7, 'ai_model_pricing_list', 'menu.ai_model_pricing', 1, 1, 2),
  (922, '编辑模型定价', '', '', 921, NULL, 'admin', 3, 1, 'ai_model_pricing_edit', '', 2, 1, 2);

COMMIT;

DROP TEMPORARY TABLE `_ai_model_pricing_guard`;
