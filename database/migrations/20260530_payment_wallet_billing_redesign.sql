-- Payment wallet and AI billing redesign.
-- Keep existing payment runtime tables. This migration adds billing rules/records,
-- links AI image tasks to billing records, and converges payment menu permissions.

CREATE TABLE IF NOT EXISTS `ai_billing_rules` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `scene` VARCHAR(64) NOT NULL,
  `unit` VARCHAR(16) NOT NULL,
  `unit_price_cents` BIGINT NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_billing_rules_scene` (`scene`),
  KEY `idx_ai_billing_rules_status` (`status`, `is_del`, `scene`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ai_billing_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `request_no` VARCHAR(64) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `platform` VARCHAR(32) NOT NULL,
  `scene` VARCHAR(64) NOT NULL,
  `agent_id` BIGINT NOT NULL,
  `provider_id` BIGINT NOT NULL,
  `model_id` VARCHAR(191) NOT NULL,
  `unit` VARCHAR(16) NOT NULL,
  `unit_count` INT NOT NULL,
  `unit_price_cents` BIGINT NOT NULL,
  `amount_cents` BIGINT NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `debit_transaction_id` BIGINT NULL,
  `refund_transaction_id` BIGINT NULL,
  `provider_task_id` VARCHAR(128) NOT NULL DEFAULT '',
  `error_message` VARCHAR(512) NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `finished_at` DATETIME NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_billing_records_request_no` (`request_no`),
  KEY `idx_ai_billing_records_user_created` (`user_id`, `created_at`, `id`),
  KEY `idx_ai_billing_records_scene_created` (`scene`, `created_at`, `id`),
  KEY `idx_ai_billing_records_status_created` (`status`, `created_at`, `id`),
  KEY `idx_ai_billing_records_provider_task` (`provider_id`, `provider_task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @ai_billing_rules_has_scene_uk := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_rules'
    AND INDEX_NAME = 'uk_ai_billing_rules_scene'
);

SET @ai_billing_rules_add_scene_uk_sql := IF(
  @ai_billing_rules_has_scene_uk = 0,
  'CREATE UNIQUE INDEX `uk_ai_billing_rules_scene` ON `ai_billing_rules` (`scene`)',
  'SELECT 1'
);
PREPARE ai_billing_rules_add_scene_uk_stmt FROM @ai_billing_rules_add_scene_uk_sql;
EXECUTE ai_billing_rules_add_scene_uk_stmt;
DEALLOCATE PREPARE ai_billing_rules_add_scene_uk_stmt;

SET @ai_billing_rules_has_status_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_rules'
    AND INDEX_NAME = 'idx_ai_billing_rules_status'
);

SET @ai_billing_rules_add_status_idx_sql := IF(
  @ai_billing_rules_has_status_idx = 0,
  'CREATE INDEX `idx_ai_billing_rules_status` ON `ai_billing_rules` (`status`, `is_del`, `scene`)',
  'SELECT 1'
);
PREPARE ai_billing_rules_add_status_idx_stmt FROM @ai_billing_rules_add_status_idx_sql;
EXECUTE ai_billing_rules_add_status_idx_stmt;
DEALLOCATE PREPARE ai_billing_rules_add_status_idx_stmt;

SET @ai_billing_records_has_request_uk := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_records'
    AND INDEX_NAME = 'uk_ai_billing_records_request_no'
);

SET @ai_billing_records_add_request_uk_sql := IF(
  @ai_billing_records_has_request_uk = 0,
  'CREATE UNIQUE INDEX `uk_ai_billing_records_request_no` ON `ai_billing_records` (`request_no`)',
  'SELECT 1'
);
PREPARE ai_billing_records_add_request_uk_stmt FROM @ai_billing_records_add_request_uk_sql;
EXECUTE ai_billing_records_add_request_uk_stmt;
DEALLOCATE PREPARE ai_billing_records_add_request_uk_stmt;

SET @ai_billing_records_has_user_created_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_records'
    AND INDEX_NAME = 'idx_ai_billing_records_user_created'
);

SET @ai_billing_records_add_user_created_idx_sql := IF(
  @ai_billing_records_has_user_created_idx = 0,
  'CREATE INDEX `idx_ai_billing_records_user_created` ON `ai_billing_records` (`user_id`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE ai_billing_records_add_user_created_idx_stmt FROM @ai_billing_records_add_user_created_idx_sql;
EXECUTE ai_billing_records_add_user_created_idx_stmt;
DEALLOCATE PREPARE ai_billing_records_add_user_created_idx_stmt;

SET @ai_billing_records_has_scene_created_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_records'
    AND INDEX_NAME = 'idx_ai_billing_records_scene_created'
);

SET @ai_billing_records_add_scene_created_idx_sql := IF(
  @ai_billing_records_has_scene_created_idx = 0,
  'CREATE INDEX `idx_ai_billing_records_scene_created` ON `ai_billing_records` (`scene`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE ai_billing_records_add_scene_created_idx_stmt FROM @ai_billing_records_add_scene_created_idx_sql;
EXECUTE ai_billing_records_add_scene_created_idx_stmt;
DEALLOCATE PREPARE ai_billing_records_add_scene_created_idx_stmt;

SET @ai_billing_records_has_status_created_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_records'
    AND INDEX_NAME = 'idx_ai_billing_records_status_created'
);

SET @ai_billing_records_add_status_created_idx_sql := IF(
  @ai_billing_records_has_status_created_idx = 0,
  'CREATE INDEX `idx_ai_billing_records_status_created` ON `ai_billing_records` (`status`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE ai_billing_records_add_status_created_idx_stmt FROM @ai_billing_records_add_status_created_idx_sql;
EXECUTE ai_billing_records_add_status_created_idx_stmt;
DEALLOCATE PREPARE ai_billing_records_add_status_created_idx_stmt;

SET @ai_billing_records_has_provider_task_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_billing_records'
    AND INDEX_NAME = 'idx_ai_billing_records_provider_task'
);

SET @ai_billing_records_add_provider_task_idx_sql := IF(
  @ai_billing_records_has_provider_task_idx = 0,
  'CREATE INDEX `idx_ai_billing_records_provider_task` ON `ai_billing_records` (`provider_id`, `provider_task_id`)',
  'SELECT 1'
);
PREPARE ai_billing_records_add_provider_task_idx_stmt FROM @ai_billing_records_add_provider_task_idx_sql;
EXECUTE ai_billing_records_add_provider_task_idx_stmt;
DEALLOCATE PREPARE ai_billing_records_add_provider_task_idx_stmt;

SET @ai_image_tasks_has_billing_record_id := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_tasks'
    AND COLUMN_NAME = 'billing_record_id'
);

SET @ai_image_tasks_add_billing_record_id_sql := IF(
  @ai_image_tasks_has_billing_record_id = 0,
  'ALTER TABLE `ai_image_tasks` ADD COLUMN `billing_record_id` BIGINT UNSIGNED NULL AFTER `id`',
  'SELECT 1'
);
PREPARE ai_image_tasks_add_billing_record_id_stmt FROM @ai_image_tasks_add_billing_record_id_sql;
EXECUTE ai_image_tasks_add_billing_record_id_stmt;
DEALLOCATE PREPARE ai_image_tasks_add_billing_record_id_stmt;

SET @ai_image_tasks_has_billing_record_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_image_tasks'
    AND INDEX_NAME = 'idx_ai_image_tasks_billing_record_id'
);

SET @ai_image_tasks_add_billing_record_idx_sql := IF(
  @ai_image_tasks_has_billing_record_idx = 0,
  'CREATE INDEX `idx_ai_image_tasks_billing_record_id` ON `ai_image_tasks` (`billing_record_id`)',
  'SELECT 1'
);
PREPARE ai_image_tasks_add_billing_record_idx_stmt FROM @ai_image_tasks_add_billing_record_idx_sql;
EXECUTE ai_image_tasks_add_billing_record_idx_stmt;
DEALLOCATE PREPARE ai_image_tasks_add_billing_record_idx_stmt;

SET @wallet_transactions_has_admin_created_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'wallet_transactions'
    AND INDEX_NAME = 'idx_wallet_tx_admin_created'
);

SET @wallet_transactions_add_admin_created_idx_sql := IF(
  @wallet_transactions_has_admin_created_idx = 0,
  'CREATE INDEX `idx_wallet_tx_admin_created` ON `wallet_transactions` (`is_del`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE wallet_transactions_add_admin_created_idx_stmt FROM @wallet_transactions_add_admin_created_idx_sql;
EXECUTE wallet_transactions_add_admin_created_idx_stmt;
DEALLOCATE PREPARE wallet_transactions_add_admin_created_idx_stmt;

SET @wallet_transactions_has_admin_direction_created_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'wallet_transactions'
    AND INDEX_NAME = 'idx_wallet_tx_admin_direction_created'
);

SET @wallet_transactions_add_admin_direction_created_idx_sql := IF(
  @wallet_transactions_has_admin_direction_created_idx = 0,
  'CREATE INDEX `idx_wallet_tx_admin_direction_created` ON `wallet_transactions` (`direction`, `is_del`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE wallet_transactions_add_admin_direction_created_idx_stmt FROM @wallet_transactions_add_admin_direction_created_idx_sql;
EXECUTE wallet_transactions_add_admin_direction_created_idx_stmt;
DEALLOCATE PREPARE wallet_transactions_add_admin_direction_created_idx_stmt;

SET @wallet_transactions_has_admin_source_created_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'wallet_transactions'
    AND INDEX_NAME = 'idx_wallet_tx_admin_source_created'
);

SET @wallet_transactions_add_admin_source_created_idx_sql := IF(
  @wallet_transactions_has_admin_source_created_idx = 0,
  'CREATE INDEX `idx_wallet_tx_admin_source_created` ON `wallet_transactions` (`source_type`, `is_del`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE wallet_transactions_add_admin_source_created_idx_stmt FROM @wallet_transactions_add_admin_source_created_idx_sql;
EXECUTE wallet_transactions_add_admin_source_created_idx_stmt;
DEALLOCATE PREPARE wallet_transactions_add_admin_source_created_idx_stmt;

SET @user_wallets_has_updated_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'user_wallets'
    AND INDEX_NAME = 'idx_user_wallet_updated'
);

SET @user_wallets_add_updated_idx_sql := IF(
  @user_wallets_has_updated_idx = 0,
  'CREATE INDEX `idx_user_wallet_updated` ON `user_wallets` (`is_del`, `updated_at`, `id`)',
  'SELECT 1'
);
PREPARE user_wallets_add_updated_idx_stmt FROM @user_wallets_add_updated_idx_sql;
EXECUTE user_wallets_add_updated_idx_stmt;
DEALLOCATE PREPARE user_wallets_add_updated_idx_stmt;

UPDATE `permissions`
SET `show_menu` = 2,
    `status` = 2,
    `is_del` = 1,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `is_del` = 2
  AND `path` IN ('/wallet', '/wallet-manage', '/wallet/transactions', '/wallet/ledger', '/wallet/users', '/payment/orders');

UPDATE `permissions`
SET `show_menu` = 2,
    `status` = 2,
    `is_del` = 1,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `is_del` = 2
  AND `code` IN (
    'wallet_consume_add',
    'payment_order_add',
    'payment_order_pay',
    'payment_order_sync',
    'payment_order_close',
    'payment_recharge_sync',
    'payment_recharge_close'
  );

SET @payment_parent_id := (
  SELECT MIN(`id`)
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND (`code` = 'payment' OR `path` = '/payment' OR `i18n_key` = 'menu.payment')
);

UPDATE `permissions`
SET `code` = NULL,
    `show_menu` = 2,
    `status` = 2,
    `is_del` = 1,
    `updated_at` = CURRENT_TIMESTAMP
WHERE @payment_parent_id IS NOT NULL
  AND `platform` = 'admin'
  AND `type` = 1
  AND `is_del` = 2
  AND `id` <> @payment_parent_id
  AND (`code` = 'payment' OR `path` = '/payment' OR `i18n_key` = 'menu.payment');

UPDATE `permissions`
SET `code` = NULL,
    `updated_at` = CURRENT_TIMESTAMP
WHERE @payment_parent_id IS NOT NULL
  AND `platform` = 'admin'
  AND `type` = 1
  AND `id` <> @payment_parent_id
  AND `code` = 'payment';

UPDATE `permissions`
SET `name` = '支付管理',
    `path` = '/payment',
    `icon` = 'CreditCard',
    `parent_id` = 0,
    `component` = '',
    `platform` = 'admin',
    `type` = 1,
    `sort` = 40,
    `code` = 'payment',
    `i18n_key` = 'menu.payment',
    `show_menu` = 1,
    `status` = 1,
    `is_del` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE @payment_parent_id IS NOT NULL
  AND `id` = @payment_parent_id;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '支付管理', '/payment', 'CreditCard', 0, '', 'admin', 1, 40, 'payment', 'menu.payment', 1, 1, 2
WHERE @payment_parent_id IS NULL
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

SET @payment_parent_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND `code` = 'payment'
  ORDER BY `id`
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT page_name, page_path, page_icon, @payment_parent_id, page_component, 'admin', 2, page_sort, page_code, page_i18n_key, page_show_menu, 1, 2
FROM (
  SELECT '支付配置' AS page_name, '/payment/config' AS page_path, 'CreditCard' AS page_icon, 'payment/config' AS page_component, 10 AS page_sort, 'payment_config_list' AS page_code, 'menu.payment_config' AS page_i18n_key, 1 AS page_show_menu
  UNION ALL SELECT '收支明细', '/payment/ledger', 'Tickets', 'payment/ledger', 20, 'payment_ledger_list', 'menu.payment_ledger', 1
  UNION ALL SELECT '用户钱包', '/payment/wallets', 'Wallet', 'payment/wallets', 30, 'payment_wallet_list', 'menu.payment_wallets', 1
  UNION ALL SELECT '充值收银台', '/payment/recharge', 'WalletFilled', 'payment/recharge', 40, 'payment_recharge_list', 'menu.payment_recharge', 2
) AS payment_pages
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
  `show_menu` = VALUES(`show_menu`),
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '我的钱包', '/profile/wallet', 'Wallet', 0, 'profile/wallet', 'admin', 2, 90, 'profile_wallet', 'menu.profile_wallet', 2, 1, 2
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `path` = VALUES(`path`),
  `icon` = VALUES(`icon`),
  `parent_id` = VALUES(`parent_id`),
  `component` = VALUES(`component`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `i18n_key` = VALUES(`i18n_key`),
  `show_menu` = 2,
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

SET @payment_config_perm_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'payment_config_list'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT 'AI计费规则编辑', '', '', @payment_config_perm_id, '', 'admin', 3, 20, 'ai_billing_rule_edit', '', 2, 1, 2
WHERE @payment_config_perm_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = 2,
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

UPDATE `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = CURRENT_TIMESTAMP
WHERE p.`platform` = 'admin'
  AND p.`is_del` = 1
  AND (
    p.`path` IN ('/wallet', '/wallet-manage', '/wallet/transactions', '/wallet/ledger', '/wallet/users', '/payment/orders')
    OR p.`code` IN (
      'wallet_consume_add',
      'payment_order_add',
      'payment_order_pay',
      'payment_order_sync',
      'payment_order_close',
      'payment_recharge_sync',
      'payment_recharge_close'
    )
  )
  AND rp.`is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT r.`id`, p.`id`, 2
FROM `roles` r
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`status` = 1
  AND p.`code` IN (
    'payment',
    'payment_config_list',
    'payment_ledger_list',
    'payment_wallet_list',
    'payment_recharge_list',
    'payment_recharge_add',
    'payment_recharge_pay',
    'profile_wallet',
    'ai_billing_rule_edit'
  )
WHERE r.`is_del` = 2
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;
