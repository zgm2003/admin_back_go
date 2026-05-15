-- Payment config naming canonicalization.
--
-- Run this after the earlier payment-config-only slice if live DB already has
-- payment_alipay_configs. Cold-start rebuild SQL now creates payment_configs
-- directly. This file migrates already-upgraded local DBs to the canonical name.

CREATE TABLE IF NOT EXISTS `payment_configs` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `provider` VARCHAR(32) NOT NULL DEFAULT 'alipay',
  `code` VARCHAR(64) NOT NULL,
  `name` VARCHAR(128) NOT NULL,
  `app_id` VARCHAR(64) NOT NULL,
  `private_key_enc` TEXT NOT NULL,
  `private_key_hint` VARCHAR(64) NOT NULL DEFAULT '',
  `app_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `platform_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `root_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `notify_url` VARCHAR(512) NOT NULL DEFAULT '',
  `return_url` VARCHAR(512) NOT NULL DEFAULT '',
  `environment` VARCHAR(16) NOT NULL DEFAULT 'sandbox',
  `enabled_methods_json` JSON NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 2,
  `remark` VARCHAR(255) NOT NULL DEFAULT '',
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_configs_code` (`code`),
  KEY `idx_payment_configs_provider_status` (`provider`, `status`, `is_del`),
  KEY `idx_payment_configs_environment` (`environment`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `payment_configs` (
  `provider`, `code`, `name`, `app_id`, `private_key_enc`, `private_key_hint`,
  `app_cert_path`, `platform_cert_path`, `root_cert_path`,
  `notify_url`, `return_url`, `environment`, `enabled_methods_json`,
  `status`, `remark`, `is_del`, `created_at`, `updated_at`
)
SELECT
  'alipay',
  `code`,
  `name`,
  `app_id`,
  `app_private_key_enc`,
  `app_private_key_hint`,
  `app_cert_path`,
  `alipay_cert_path`,
  `alipay_root_cert_path`,
  `notify_url`,
  `return_url`,
  `environment`,
  `enabled_methods_json`,
  `status`,
  `remark`,
  `is_del`,
  `created_at`,
  `updated_at`
FROM `payment_alipay_configs`
ON DUPLICATE KEY UPDATE
  `provider` = VALUES(`provider`),
  `name` = VALUES(`name`),
  `app_id` = VALUES(`app_id`),
  `private_key_enc` = VALUES(`private_key_enc`),
  `private_key_hint` = VALUES(`private_key_hint`),
  `app_cert_path` = VALUES(`app_cert_path`),
  `platform_cert_path` = VALUES(`platform_cert_path`),
  `root_cert_path` = VALUES(`root_cert_path`),
  `notify_url` = VALUES(`notify_url`),
  `return_url` = VALUES(`return_url`),
  `environment` = VALUES(`environment`),
  `enabled_methods_json` = VALUES(`enabled_methods_json`),
  `status` = VALUES(`status`),
  `remark` = VALUES(`remark`),
  `is_del` = VALUES(`is_del`),
  `updated_at` = CURRENT_TIMESTAMP;

UPDATE `permissions`
SET `code` = NULL,
    `path` = '',
    `component` = '',
    `type` = 1,
    `show_menu` = 1,
    `status` = 1,
    `is_del` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `type` = 1
  AND (`i18n_key` = 'menu.payment' OR `code` = 'payment' OR `path` = '/payment');

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '支付管理', '', 'CreditCard', 0, '', 'admin', 1, 90, NULL, 'menu.payment', 1, 1, 2
WHERE NOT EXISTS (
  SELECT 1 FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND `i18n_key` = 'menu.payment'
);

SET @payment_parent_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND `i18n_key` = 'menu.payment'
  ORDER BY `id`
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '支付配置', '/payment/config', 'CreditCard', @payment_parent_id, 'payment/config', 'admin', 2, 10, 'payment_config_list', 'menu.payment_config', 1, 1, 2
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

SET @payment_config_page_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'payment_config_list'
  LIMIT 1
);

SET @old_payment_config_delete_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'payment_config_delete'
  LIMIT 1
);

SET @new_payment_config_del_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'payment_config_del'
  LIMIT 1
);

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT rp.`role_id`, @new_payment_config_del_id, 2
FROM `role_permissions` rp
WHERE @old_payment_config_delete_id IS NOT NULL
  AND @new_payment_config_del_id IS NOT NULL
  AND rp.`permission_id` = @old_payment_config_delete_id
  AND rp.`is_del` = 2
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

UPDATE `permissions`
SET `code` = IF(@new_payment_config_del_id IS NULL, 'payment_config_del', `code`),
    `is_del` = IF(@new_payment_config_del_id IS NULL, `is_del`, 1),
    `status` = IF(@new_payment_config_del_id IS NULL, `status`, 2),
    `show_menu` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `id` = @old_payment_config_delete_id;

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @payment_config_page_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '新增支付配置' AS button_name, 'payment_config_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '编辑支付配置', 'payment_config_edit', 2
  UNION ALL SELECT '切换支付配置状态', 'payment_config_status', 3
  UNION ALL SELECT '删除支付配置', 'payment_config_del', 4
  UNION ALL SELECT '上传支付宝证书', 'payment_config_upload_cert', 5
  UNION ALL SELECT '测试支付配置', 'payment_config_test', 6
) AS payment_config_buttons
WHERE @payment_config_page_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = 2,
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TABLE IF EXISTS `payment_alipay_configs`;
