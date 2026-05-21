-- Wallet recharge + consume v1.
-- payment_orders is the Alipay/gateway collection ledger.
-- wallet_transactions is the funds ledger for recharge in and consume out.

SET @user_wallets_has_total_consume := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'user_wallets'
    AND COLUMN_NAME = 'total_consume_cents'
);

SET @user_wallets_add_total_consume_sql := IF(
  @user_wallets_has_total_consume = 0,
  'ALTER TABLE `user_wallets` ADD COLUMN `total_consume_cents` BIGINT NOT NULL DEFAULT 0 COMMENT ''累计消费金额，单位分'' AFTER `total_recharge_cents`',
  'SELECT 1'
);
PREPARE user_wallets_add_total_consume_stmt FROM @user_wallets_add_total_consume_sql;
EXECUTE user_wallets_add_total_consume_stmt;
DEALLOCATE PREPARE user_wallets_add_total_consume_stmt;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '钱包中心', '/wallet', 'Wallet', 0, '', 'admin', 1, 45, 'wallet_center', 'menu.wallet_center', 1, 1, 2
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

SET @wallet_center_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'admin' AND `code` = 'wallet_center' AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '资金明细', '/wallet/transactions', 'Tickets', @wallet_center_id, 'wallet/transactions', 'admin', 2, 20, 'wallet_transaction_list', 'menu.wallet_transaction', 1, 1, 2
WHERE @wallet_center_id IS NOT NULL
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

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '钱包管理', '/wallet-manage', 'WalletFilled', 0, '', 'admin', 1, 46, 'wallet_manage', 'menu.wallet_manage', 1, 1, 2
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

SET @wallet_manage_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'admin' AND `code` = 'wallet_manage' AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '用户钱包', '/wallet/users', 'Wallet', @wallet_manage_id, 'wallet/users', 'admin', 2, 10, 'wallet_user_list', 'menu.wallet_user', 1, 1, 2
WHERE @wallet_manage_id IS NOT NULL
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

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '资金流水', '/wallet/ledger', 'Tickets', @wallet_manage_id, 'wallet/ledger', 'admin', 2, 20, 'wallet_ledger_list', 'menu.wallet_ledger', 1, 1, 2
WHERE @wallet_manage_id IS NOT NULL
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

SET @wallet_transaction_perm_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'admin' AND `code` = 'wallet_transaction_list' AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '测试消费', '', '', @wallet_transaction_perm_id, '', 'admin', 3, 30, 'wallet_consume_add', '', 2, 1, 2
WHERE @wallet_transaction_perm_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = 2,
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_wallet_permission_grant_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_wallet_permission_grant_roles`;

INSERT IGNORE INTO `tmp_wallet_permission_grant_roles` (`role_id`)
SELECT DISTINCT rp.`role_id`
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
JOIN `roles` r ON r.`id` = rp.`role_id`
WHERE rp.`is_del` = 2
  AND p.`is_del` = 2
  AND r.`is_del` = 2
  AND p.`platform` = 'admin'
  AND p.`code` IN ('payment_config_list', 'payment_recharge_list', 'payment_order_list');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT gr.`role_id`, p.`id`, 2
FROM `tmp_wallet_permission_grant_roles` gr
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN ('wallet_center', 'wallet_transaction_list', 'wallet_manage', 'wallet_user_list', 'wallet_ledger_list')
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TEMPORARY TABLE IF EXISTS `tmp_wallet_permission_grant_roles`;
