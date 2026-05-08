-- Rebuild payment as a project-native bounded context.
-- Old pay/wallet data is intentionally preserved by renaming payment-owned
-- tables first. Do not drop orders/order_items here because their names are
-- not payment-specific and must be checked separately before deletion.

SET @pay_channel_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_channel'
);
SET @pay_channel_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_channel_legacy_20260508'
);
SET @rename_pay_channel_sql := IF(
    @pay_channel_exists = 1 AND @pay_channel_legacy_exists = 0,
    'RENAME TABLE `pay_channel` TO `pay_channel_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_pay_channel_stmt FROM @rename_pay_channel_sql;
EXECUTE rename_pay_channel_stmt;
DEALLOCATE PREPARE rename_pay_channel_stmt;

SET @pay_transactions_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_transactions'
);
SET @pay_transactions_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_transactions_legacy_20260508'
);
SET @rename_pay_transactions_sql := IF(
    @pay_transactions_exists = 1 AND @pay_transactions_legacy_exists = 0,
    'RENAME TABLE `pay_transactions` TO `pay_transactions_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_pay_transactions_stmt FROM @rename_pay_transactions_sql;
EXECUTE rename_pay_transactions_stmt;
DEALLOCATE PREPARE rename_pay_transactions_stmt;

SET @pay_notify_logs_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_notify_logs'
);
SET @pay_notify_logs_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_notify_logs_legacy_20260508'
);
SET @rename_pay_notify_logs_sql := IF(
    @pay_notify_logs_exists = 1 AND @pay_notify_logs_legacy_exists = 0,
    'RENAME TABLE `pay_notify_logs` TO `pay_notify_logs_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_pay_notify_logs_stmt FROM @rename_pay_notify_logs_sql;
EXECUTE rename_pay_notify_logs_stmt;
DEALLOCATE PREPARE rename_pay_notify_logs_stmt;

SET @user_wallets_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_wallets'
);
SET @user_wallets_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_wallets_legacy_20260508'
);
SET @rename_user_wallets_sql := IF(
    @user_wallets_exists = 1 AND @user_wallets_legacy_exists = 0,
    'RENAME TABLE `user_wallets` TO `user_wallets_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_user_wallets_stmt FROM @rename_user_wallets_sql;
EXECUTE rename_user_wallets_stmt;
DEALLOCATE PREPARE rename_user_wallets_stmt;

SET @wallet_transactions_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'wallet_transactions'
);
SET @wallet_transactions_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'wallet_transactions_legacy_20260508'
);
SET @rename_wallet_transactions_sql := IF(
    @wallet_transactions_exists = 1 AND @wallet_transactions_legacy_exists = 0,
    'RENAME TABLE `wallet_transactions` TO `wallet_transactions_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_wallet_transactions_stmt FROM @rename_wallet_transactions_sql;
EXECUTE rename_wallet_transactions_stmt;
DEALLOCATE PREPARE rename_wallet_transactions_stmt;

SET @order_fulfillments_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'order_fulfillments'
);
SET @order_fulfillments_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'order_fulfillments_legacy_20260508'
);
SET @rename_order_fulfillments_sql := IF(
    @order_fulfillments_exists = 1 AND @order_fulfillments_legacy_exists = 0,
    'RENAME TABLE `order_fulfillments` TO `order_fulfillments_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_order_fulfillments_stmt FROM @rename_order_fulfillments_sql;
EXECUTE rename_order_fulfillments_stmt;
DEALLOCATE PREPARE rename_order_fulfillments_stmt;

SET @pay_reconcile_tasks_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_reconcile_tasks'
);
SET @pay_reconcile_tasks_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_reconcile_tasks_legacy_20260508'
);
SET @rename_pay_reconcile_tasks_sql := IF(
    @pay_reconcile_tasks_exists = 1 AND @pay_reconcile_tasks_legacy_exists = 0,
    'RENAME TABLE `pay_reconcile_tasks` TO `pay_reconcile_tasks_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_pay_reconcile_tasks_stmt FROM @rename_pay_reconcile_tasks_sql;
EXECUTE rename_pay_reconcile_tasks_stmt;
DEALLOCATE PREPARE rename_pay_reconcile_tasks_stmt;

SET @pay_refunds_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_refunds'
);
SET @pay_refunds_legacy_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'pay_refunds_legacy_20260508'
);
SET @rename_pay_refunds_sql := IF(
    @pay_refunds_exists = 1 AND @pay_refunds_legacy_exists = 0,
    'RENAME TABLE `pay_refunds` TO `pay_refunds_legacy_20260508`',
    'SELECT 1'
);
PREPARE rename_pay_refunds_stmt FROM @rename_pay_refunds_sql;
EXECUTE rename_pay_refunds_stmt;
DEALLOCATE PREPARE rename_pay_refunds_stmt;

CREATE TABLE IF NOT EXISTS `payment_channels` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'channel code, e.g. alipay_sandbox',
  `name` VARCHAR(128) NOT NULL DEFAULT '',
  `provider` VARCHAR(32) NOT NULL DEFAULT 'alipay',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1 enabled, 2 disabled',
  `supported_methods` JSON NULL,
  `remark` VARCHAR(255) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2 COMMENT '1 deleted, 2 normal',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_channels_code` (`code`),
  KEY `idx_payment_channels_provider_status` (`provider`, `status`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='payment channels';

CREATE TABLE IF NOT EXISTS `payment_channel_configs` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `channel_id` BIGINT NOT NULL,
  `app_id` VARCHAR(64) NOT NULL DEFAULT '',
  `merchant_id` VARCHAR(64) NOT NULL DEFAULT '',
  `sign_type` VARCHAR(16) NOT NULL DEFAULT 'RSA2',
  `is_sandbox` TINYINT NOT NULL DEFAULT 1,
  `notify_url` VARCHAR(512) NOT NULL DEFAULT '',
  `return_url` VARCHAR(512) NOT NULL DEFAULT '',
  `private_key_enc` TEXT NULL,
  `private_key_hint` VARCHAR(64) NOT NULL DEFAULT '',
  `app_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `alipay_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `alipay_root_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `extra_config` JSON NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_channel_configs_channel_id` (`channel_id`),
  KEY `idx_payment_channel_configs_app_id` (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='payment channel configs';

CREATE TABLE IF NOT EXISTS `payment_orders` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `order_no` VARCHAR(64) NOT NULL DEFAULT '',
  `user_id` BIGINT NOT NULL DEFAULT 0,
  `channel_id` BIGINT NOT NULL DEFAULT 0,
  `provider` VARCHAR(32) NOT NULL DEFAULT 'alipay',
  `pay_method` VARCHAR(16) NOT NULL DEFAULT '',
  `subject` VARCHAR(128) NOT NULL DEFAULT '',
  `amount_cents` BIGINT NOT NULL DEFAULT 0,
  `currency` VARCHAR(8) NOT NULL DEFAULT 'CNY',
  `status` TINYINT NOT NULL DEFAULT 1,
  `out_trade_no` VARCHAR(64) NULL,
  `trade_no` VARCHAR(128) NOT NULL DEFAULT '',
  `pay_url` TEXT NULL,
  `paid_at` DATETIME NULL,
  `expired_at` DATETIME NOT NULL,
  `closed_at` DATETIME NULL,
  `client_ip` VARCHAR(64) NOT NULL DEFAULT '',
  `return_url` VARCHAR(512) NOT NULL DEFAULT '',
  `business_type` VARCHAR(64) NOT NULL DEFAULT 'manual_test',
  `business_ref` VARCHAR(128) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_orders_order_no` (`order_no`),
  UNIQUE KEY `uk_payment_orders_out_trade_no` (`out_trade_no`),
  KEY `idx_payment_orders_user_status` (`user_id`, `status`, `is_del`),
  KEY `idx_payment_orders_channel_status` (`channel_id`, `status`, `is_del`),
  KEY `idx_payment_orders_expired_at` (`expired_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='payment orders';

CREATE TABLE IF NOT EXISTS `payment_events` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `order_no` VARCHAR(64) NOT NULL DEFAULT '',
  `out_trade_no` VARCHAR(64) NOT NULL DEFAULT '',
  `event_type` VARCHAR(32) NOT NULL DEFAULT '',
  `provider` VARCHAR(32) NOT NULL DEFAULT 'alipay',
  `request_data` JSON NULL,
  `response_data` JSON NULL,
  `process_status` TINYINT NOT NULL DEFAULT 1,
  `error_message` VARCHAR(1024) NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_payment_events_order_no` (`order_no`),
  KEY `idx_payment_events_out_trade_no` (`out_trade_no`),
  KEY `idx_payment_events_type_status` (`event_type`, `process_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='payment events';

INSERT INTO `payment_channels` (`code`, `name`, `provider`, `status`, `supported_methods`, `remark`, `is_del`)
SELECT
  'alipay_sandbox',
  `name`,
  'alipay',
  `status`,
  JSON_ARRAY('web', 'h5'),
  `remark`,
  2
FROM `pay_channel_legacy_20260508`
WHERE `channel` = 2 AND `is_del` = 2
  AND NOT EXISTS (
    SELECT 1
    FROM `payment_channels` AS existing
    WHERE existing.`code` = 'alipay_sandbox'
  )
ORDER BY `id` ASC
LIMIT 1;

INSERT INTO `payment_channel_configs` (
  `channel_id`, `app_id`, `merchant_id`, `sign_type`, `is_sandbox`,
  `notify_url`, `return_url`, `private_key_enc`, `private_key_hint`,
  `app_cert_path`, `alipay_cert_path`, `alipay_root_cert_path`, `extra_config`
)
SELECT
  pc.`id`,
  legacy.`app_id`,
  legacy.`mch_id`,
  'RSA2',
  legacy.`is_sandbox`,
  legacy.`notify_url`,
  '',
  legacy.`app_private_key_enc`,
  legacy.`app_private_key_hint`,
  legacy.`public_cert_path`,
  legacy.`platform_cert_path`,
  legacy.`root_cert_path`,
  legacy.`extra_config`
FROM `payment_channels` AS pc
JOIN `pay_channel_legacy_20260508` AS legacy ON legacy.`channel` = 2 AND legacy.`is_del` = 2
WHERE pc.`code` = 'alipay_sandbox'
  AND NOT EXISTS (
    SELECT 1
    FROM `payment_channel_configs` AS existing
    WHERE existing.`channel_id` = pc.`id`
  )
ORDER BY legacy.`id` ASC
LIMIT 1;

UPDATE `role_permissions`
SET `is_del` = 1,
    `updated_at` = NOW()
WHERE `is_del` = 2
  AND `permission_id` IN (
    SELECT `id`
    FROM `permissions`
    WHERE `platform` = 'admin'
      AND (
        `path` = '/wallet'
        OR `path` LIKE '/wallet/%'
        OR `path` = '/pay'
        OR `path` LIKE '/pay/%'
        OR `code` LIKE 'pay\_%'
      )
  );

UPDATE `permissions`
SET `is_del` = 1, `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND (
    `path` = '/wallet'
    OR `path` LIKE '/wallet/%'
    OR `path` = '/pay'
    OR `path` LIKE '/pay/%'
    OR `code` LIKE 'pay\_%'
  );

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '支付管理', '/payment', 'Wallet', 0, '', 'admin', 1, 9600, 'payment', 'menu.payment', 1, 1, 2, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment' AND `is_del` = 2);

SET @payment_parent_id := (SELECT `id` FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment' AND `is_del` = 2 LIMIT 1);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '支付渠道', '/payment/channel', '', @payment_parent_id, 'payment/channel', 'admin', 2, 9610, 'payment_channel_list', 'menu.payment.channel', 1, 1, 2, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_channel_list' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '支付订单', '/payment/order', '', @payment_parent_id, 'payment/order', 'admin', 2, 9620, 'payment_order_list', 'menu.payment.order', 1, 1, 2, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_order_list' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '支付事件', '/payment/event', '', @payment_parent_id, 'payment/event', 'admin', 2, 9630, 'payment_event_list', 'menu.payment.event', 1, 1, 2, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_event_list' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '新增支付渠道', '', '', p.`id`, '', 'admin', 3, 9611, 'payment_channel_add', 'button.payment.channel.add', 2, 1, 2, NOW(), NOW()
FROM `permissions` AS p
WHERE p.`platform` = 'admin' AND p.`code` = 'payment_channel_list' AND p.`is_del` = 2
  AND NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_channel_add' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '编辑支付渠道', '', '', p.`id`, '', 'admin', 3, 9612, 'payment_channel_edit', 'button.payment.channel.edit', 2, 1, 2, NOW(), NOW()
FROM `permissions` AS p
WHERE p.`platform` = 'admin' AND p.`code` = 'payment_channel_list' AND p.`is_del` = 2
  AND NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_channel_edit' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '切换支付渠道状态', '', '', p.`id`, '', 'admin', 3, 9613, 'payment_channel_status', 'button.payment.channel.status', 2, 1, 2, NOW(), NOW()
FROM `permissions` AS p
WHERE p.`platform` = 'admin' AND p.`code` = 'payment_channel_list' AND p.`is_del` = 2
  AND NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_channel_status' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '删除支付渠道', '', '', p.`id`, '', 'admin', 3, 9614, 'payment_channel_del', 'button.payment.channel.del', 2, 1, 2, NOW(), NOW()
FROM `permissions` AS p
WHERE p.`platform` = 'admin' AND p.`code` = 'payment_channel_list' AND p.`is_del` = 2
  AND NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_channel_del' AND `is_del` = 2);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT '关闭支付订单', '', '', p.`id`, '', 'admin', 3, 9621, 'payment_order_close', 'button.payment.order.close', 2, 1, 2, NOW(), NOW()
FROM `permissions` AS p
WHERE p.`platform` = 'admin' AND p.`code` = 'payment_order_list' AND p.`is_del` = 2
  AND NOT EXISTS (SELECT 1 FROM `permissions` WHERE `platform` = 'admin' AND `code` = 'payment_order_close' AND `is_del` = 2);

UPDATE `permissions`
SET `component` = 'payment/channel', `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `code` = 'payment_channel_list'
  AND `path` = '/payment/channel'
  AND `component` = 'payment/channel/index'
  AND `is_del` = 2;

UPDATE `permissions`
SET `component` = 'payment/order', `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `code` = 'payment_order_list'
  AND `path` = '/payment/order'
  AND `component` = 'payment/order/index'
  AND `is_del` = 2;

UPDATE `permissions`
SET `component` = 'payment/event', `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `code` = 'payment_event_list'
  AND `path` = '/payment/event'
  AND `component` = 'payment/event/index'
  AND `is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`, `created_at`, `updated_at`)
SELECT DISTINCT legacy_grant.`role_id`, payment_perm.`id`, 2, NOW(), NOW()
FROM `role_permissions` AS legacy_grant
JOIN `permissions` AS legacy_perm
  ON legacy_perm.`id` = legacy_grant.`permission_id`
 AND legacy_perm.`platform` = 'admin'
 AND (
      legacy_perm.`path` = '/wallet'
      OR legacy_perm.`path` LIKE '/wallet/%'
      OR legacy_perm.`path` = '/pay'
      OR legacy_perm.`path` LIKE '/pay/%'
      OR legacy_perm.`code` LIKE 'pay\_%'
 )
JOIN `permissions` AS payment_perm
  ON payment_perm.`platform` = 'admin'
 AND payment_perm.`code` IN (
      'payment',
      'payment_channel_list',
      'payment_order_list',
      'payment_event_list',
      'payment_channel_add',
      'payment_channel_edit',
      'payment_channel_status',
      'payment_channel_del',
      'payment_order_close'
 )
 AND payment_perm.`is_del` = 2
WHERE legacy_grant.`is_del` = 1
ON DUPLICATE KEY UPDATE
    `is_del` = VALUES(`is_del`),
    `updated_at` = NOW();

UPDATE `cron_task`
SET `is_del` = 1, `updated_at` = NOW()
WHERE `name` IN ('pay_reconcile_daily', 'pay_reconcile_execute', 'pay_fulfillment_retry', 'pay_refund_sync', 'pay_close_expired_order', 'pay_sync_pending_transaction');

INSERT INTO `cron_task` (`name`, `title`, `description`, `cron`, `cron_readable`, `handler`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT 'payment_close_expired_order', '关闭过期支付订单', 'Go payment domain task', '0 * * * * *', '每分钟', 'payment:close-expired-order:v1', 1, 2, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `cron_task` WHERE `name` = 'payment_close_expired_order' AND `is_del` = 2);

INSERT INTO `cron_task` (`name`, `title`, `description`, `cron`, `cron_readable`, `handler`, `status`, `is_del`, `created_at`, `updated_at`)
SELECT 'payment_sync_pending_order', '同步待支付订单', 'Go payment domain task', '0 */5 * * * *', '每5分钟', 'payment:sync-pending-order:v1', 1, 2, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `cron_task` WHERE `name` = 'payment_sync_pending_order' AND `is_del` = 2);
