CREATE TABLE IF NOT EXISTS `payment_orders` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `order_no` VARCHAR(64) NOT NULL,
  `config_id` BIGINT NOT NULL,
  `config_code` VARCHAR(64) NOT NULL,
  `provider` VARCHAR(32) NOT NULL DEFAULT 'alipay',
  `pay_method` VARCHAR(16) NOT NULL,
  `subject` VARCHAR(128) NOT NULL,
  `amount_cents` BIGINT NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `pay_url` VARCHAR(2048) NOT NULL DEFAULT '',
  `return_url` VARCHAR(512) NOT NULL DEFAULT '',
  `alipay_trade_no` VARCHAR(64) NOT NULL DEFAULT '',
  `expired_at` DATETIME NOT NULL,
  `paid_at` DATETIME NULL,
  `closed_at` DATETIME NULL,
  `failure_reason` VARCHAR(255) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_order_no` (`order_no`),
  KEY `idx_payment_order_status_created` (`is_del`, `status`, `created_at`),
  KEY `idx_payment_order_config_created` (`config_id`, `created_at`, `is_del`),
  CONSTRAINT `fk_payment_order_config`
    FOREIGN KEY (`config_id`) REFERENCES `payment_configs` (`id`)
    ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
SELECT '支付订单', '/payment/orders', 'Tickets', @payment_parent_id, 'payment/orders', 'admin', 2, 20, 'payment_order_list', 'menu.payment_order', 1, 1, 2
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

SET @payment_order_perm_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'payment_order_list'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @payment_order_perm_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '新增支付订单' AS button_name, 'payment_order_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '拉起支付宝支付', 'payment_order_pay', 2
  UNION ALL SELECT '同步支付订单状态', 'payment_order_sync', 3
  UNION ALL SELECT '关闭支付订单', 'payment_order_close', 4
) AS payment_order_buttons
WHERE @payment_order_perm_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = 2,
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_payment_order_permission_grant_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_payment_order_permission_grant_roles`;

INSERT IGNORE INTO `tmp_payment_order_permission_grant_roles` (`role_id`)
SELECT DISTINCT rp.`role_id`
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
JOIN `roles` r ON r.`id` = rp.`role_id`
WHERE rp.`is_del` = 2
  AND p.`is_del` = 2
  AND r.`is_del` = 2
  AND p.`platform` = 'admin'
  AND p.`code` IN ('payment_config_list', 'payment_config_edit', 'payment_config_test');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT gr.`role_id`, p.`id`, 2
FROM `tmp_payment_order_permission_grant_roles` gr
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN (
    'payment_order_list',
    'payment_order_add',
    'payment_order_pay',
    'payment_order_sync',
    'payment_order_close'
  )
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TEMPORARY TABLE IF EXISTS `tmp_payment_order_permission_grant_roles`;
