-- Payment recharge cashier v1.
--
-- Active runtime after this migration:
-- - payment_configs.sort selects the preferred enabled Alipay config
-- - payment_recharge_packages
-- - payment_recharges
-- - user_wallets
-- - wallet_transactions
-- - /payment/recharge
-- - payment_recharge_* permission codes

SET @payment_configs_has_sort := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'payment_configs'
    AND COLUMN_NAME = 'sort'
);

SET @payment_configs_add_sort_sql := IF(
  @payment_configs_has_sort = 0,
  'ALTER TABLE `payment_configs` ADD COLUMN `sort` INT NOT NULL DEFAULT 100 AFTER `enabled_methods_json`',
  'SELECT 1'
);

PREPARE payment_configs_add_sort_stmt FROM @payment_configs_add_sort_sql;
EXECUTE payment_configs_add_sort_stmt;
DEALLOCATE PREPARE payment_configs_add_sort_stmt;

SET @payment_configs_has_sort_idx := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'payment_configs'
    AND INDEX_NAME = 'idx_payment_configs_provider_status_sort'
);

SET @payment_configs_add_sort_idx_sql := IF(
  @payment_configs_has_sort_idx = 0,
  'CREATE INDEX `idx_payment_configs_provider_status_sort` ON `payment_configs` (`provider`, `status`, `is_del`, `sort`, `id`)',
  'SELECT 1'
);

PREPARE payment_configs_add_sort_idx_stmt FROM @payment_configs_add_sort_idx_sql;
EXECUTE payment_configs_add_sort_idx_stmt;
DEALLOCATE PREPARE payment_configs_add_sort_idx_stmt;

CREATE TABLE IF NOT EXISTS `payment_recharge_packages` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(64) NOT NULL,
  `name` VARCHAR(128) NOT NULL,
  `amount_cents` BIGINT NOT NULL,
  `badge` VARCHAR(32) NOT NULL DEFAULT '',
  `sort` INT NOT NULL DEFAULT 100,
  `status` TINYINT NOT NULL DEFAULT 1,
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_recharge_package_code` (`code`),
  KEY `idx_payment_recharge_package_status_sort` (`status`, `is_del`, `sort`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_wallets` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `balance_cents` BIGINT NOT NULL DEFAULT 0,
  `total_recharge_cents` BIGINT NOT NULL DEFAULT 0,
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_wallet_user` (`user_id`),
  KEY `idx_user_wallet_isdel` (`is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `wallet_transactions` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `transaction_no` VARCHAR(64) NOT NULL,
  `wallet_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `direction` VARCHAR(16) NOT NULL,
  `amount_cents` BIGINT NOT NULL,
  `balance_before_cents` BIGINT NOT NULL,
  `balance_after_cents` BIGINT NOT NULL,
  `source_type` VARCHAR(32) NOT NULL,
  `source_id` BIGINT NOT NULL,
  `remark` VARCHAR(255) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_wallet_transaction_no` (`transaction_no`),
  UNIQUE KEY `uk_wallet_transaction_source` (`source_type`, `source_id`),
  KEY `idx_wallet_transaction_user_created` (`user_id`, `is_del`, `created_at`),
  KEY `idx_wallet_transaction_wallet_created` (`wallet_id`, `is_del`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `payment_recharges` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `recharge_no` VARCHAR(64) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `package_code` VARCHAR(64) NOT NULL,
  `package_name` VARCHAR(128) NOT NULL,
  `amount_cents` BIGINT NOT NULL,
  `payment_order_id` BIGINT NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `paid_at` DATETIME NULL,
  `credited_at` DATETIME NULL,
  `failure_reason` VARCHAR(255) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_recharge_no` (`recharge_no`),
  UNIQUE KEY `uk_payment_recharge_order` (`payment_order_id`),
  KEY `idx_payment_recharge_user_status_created` (`user_id`, `is_del`, `status`, `created_at`),
  KEY `idx_payment_recharge_created` (`is_del`, `created_at`),
  CONSTRAINT `fk_payment_recharge_order`
    FOREIGN KEY (`payment_order_id`) REFERENCES `payment_orders` (`id`)
    ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `payment_recharge_packages` (`code`, `name`, `amount_cents`, `badge`, `sort`, `status`, `is_del`)
VALUES
  ('recharge_10', '¥10', 1000, '', 10, 1, 2),
  ('recharge_20', '¥20', 2000, '推荐', 20, 1, 2),
  ('recharge_30', '¥30', 3000, '推荐', 30, 1, 2),
  ('recharge_50', '¥50', 5000, '推荐', 40, 1, 2),
  ('recharge_100', '¥100', 10000, '推荐', 50, 1, 2),
  ('recharge_300', '¥300', 30000, '推荐', 60, 1, 2),
  ('recharge_500', '¥500', 50000, '推荐', 70, 1, 2),
  ('recharge_888', '¥888', 88800, '', 80, 1, 2)
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `amount_cents` = VALUES(`amount_cents`),
  `badge` = VALUES(`badge`),
  `sort` = VALUES(`sort`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

SET @payment_parent_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND (`code` = 'payment' OR `path` = '/payment' OR `i18n_key` = 'menu.payment')
  ORDER BY `id`
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '充值/记录', '/payment/recharge', 'Wallet', @payment_parent_id, 'payment/recharge', 'admin', 2, 20, 'payment_recharge_list', 'menu.payment_recharge', 1, 1, 2
WHERE @payment_parent_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `path` = VALUES(`path`),
  `icon` = VALUES(`icon`),
  `parent_id` = VALUES(`parent_id`),
  `component` = VALUES(`component`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `i18n_key` = VALUES(`i18n_key`),
  `show_menu` = 1,
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

SET @payment_recharge_perm_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'payment_recharge_list'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @payment_recharge_perm_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '创建充值' AS button_name, 'payment_recharge_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '继续支付', 'payment_recharge_pay', 2
  UNION ALL SELECT '同步充值状态', 'payment_recharge_sync', 3
  UNION ALL SELECT '关闭充值', 'payment_recharge_close', 4
) AS payment_recharge_buttons
WHERE @payment_recharge_perm_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = 2,
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_payment_recharge_permission_grant_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_payment_recharge_permission_grant_roles`;

INSERT IGNORE INTO `tmp_payment_recharge_permission_grant_roles` (`role_id`)
SELECT DISTINCT rp.`role_id`
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
JOIN `roles` r ON r.`id` = rp.`role_id`
WHERE rp.`is_del` = 2
  AND p.`is_del` = 2
  AND r.`is_del` = 2
  AND p.`platform` = 'admin'
  AND p.`code` IN ('payment_config_list', 'payment_order_list');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT gr.`role_id`, p.`id`, 2
FROM `tmp_payment_recharge_permission_grant_roles` gr
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN (
    'payment_recharge_list',
    'payment_recharge_add',
    'payment_recharge_pay',
    'payment_recharge_sync',
    'payment_recharge_close'
  )
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TEMPORARY TABLE IF EXISTS `tmp_payment_recharge_permission_grant_roles`;

UPDATE `permissions`
SET `show_menu` = 2,
    `sort` = 30,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `code` = 'payment_order_list'
  AND `is_del` = 2;
