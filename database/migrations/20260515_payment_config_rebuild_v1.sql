-- Payment config rebuild v1.
--
-- Active runtime after this migration:
-- - payment_alipay_configs
-- - /payment/config
-- - payment_config_* permission codes
--
-- Retired from active Go/Vue runtime by this slice:
-- - payment_channels / payment_channel_configs as old migration sources only
-- - /payment/channel, /payment/order, /payment/event menus
-- - payment_channel_*, payment_order_*, payment_event_* permission codes
-- - payment order compensation cron handlers until the order slice is rebuilt

CREATE TABLE IF NOT EXISTS `payment_alipay_configs` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(64) NOT NULL,
  `name` VARCHAR(128) NOT NULL,
  `app_id` VARCHAR(64) NOT NULL,
  `app_private_key_enc` TEXT NOT NULL,
  `app_private_key_hint` VARCHAR(64) NOT NULL DEFAULT '',
  `app_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `alipay_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
  `alipay_root_cert_path` VARCHAR(512) NOT NULL DEFAULT '',
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
  UNIQUE KEY `uk_payment_alipay_configs_code` (`code`),
  KEY `idx_payment_alipay_configs_status` (`status`, `is_del`),
  KEY `idx_payment_alipay_configs_environment` (`environment`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `payment_alipay_configs` (
  `code`, `name`, `app_id`, `app_private_key_enc`, `app_private_key_hint`,
  `app_cert_path`, `alipay_cert_path`, `alipay_root_cert_path`,
  `notify_url`, `return_url`, `environment`, `enabled_methods_json`,
  `status`, `remark`, `is_del`, `created_at`, `updated_at`
)
SELECT
  ch.`code`,
  ch.`name`,
  cfg.`app_id`,
  COALESCE(cfg.`private_key_enc`, ''),
  COALESCE(cfg.`private_key_hint`, ''),
  cfg.`app_cert_path`,
  cfg.`alipay_cert_path`,
  cfg.`alipay_root_cert_path`,
  cfg.`notify_url`,
  cfg.`return_url`,
  CASE WHEN cfg.`is_sandbox` = 1 THEN 'sandbox' ELSE 'production' END,
  CASE
    WHEN JSON_VALID(ch.`supported_methods`) AND JSON_LENGTH(ch.`supported_methods`) > 0 THEN ch.`supported_methods`
    ELSE JSON_ARRAY('web', 'h5')
  END,
  ch.`status`,
  ch.`remark`,
  ch.`is_del`,
  ch.`created_at`,
  ch.`updated_at`
FROM `payment_channels` ch
JOIN `payment_channel_configs` cfg ON cfg.`channel_id` = ch.`id`
WHERE ch.`provider` = 'alipay'
  AND ch.`is_del` = 2
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `app_id` = VALUES(`app_id`),
  `app_private_key_enc` = VALUES(`app_private_key_enc`),
  `app_private_key_hint` = VALUES(`app_private_key_hint`),
  `app_cert_path` = VALUES(`app_cert_path`),
  `alipay_cert_path` = VALUES(`alipay_cert_path`),
  `alipay_root_cert_path` = VALUES(`alipay_root_cert_path`),
  `notify_url` = VALUES(`notify_url`),
  `return_url` = VALUES(`return_url`),
  `environment` = VALUES(`environment`),
  `enabled_methods_json` = VALUES(`enabled_methods_json`),
  `status` = VALUES(`status`),
  `remark` = VALUES(`remark`),
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

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @payment_config_page_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '新增支付配置' AS button_name, 'payment_config_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '编辑支付配置', 'payment_config_edit', 2
  UNION ALL SELECT '切换支付配置状态', 'payment_config_status', 3
  UNION ALL SELECT '删除支付配置', 'payment_config_delete', 4
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

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_payment_config_permission_map` (
  `old_code` VARCHAR(100) NOT NULL,
  `new_code` VARCHAR(100) NOT NULL,
  PRIMARY KEY (`old_code`, `new_code`)
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_payment_config_permission_map`;

INSERT INTO `tmp_payment_config_permission_map` (`old_code`, `new_code`) VALUES
  ('payment_channel_list', 'payment_config_list'),
  ('payment_channel_add', 'payment_config_add'),
  ('payment_channel_add', 'payment_config_upload_cert'),
  ('payment_channel_add', 'payment_config_test'),
  ('payment_channel_edit', 'payment_config_edit'),
  ('payment_channel_edit', 'payment_config_upload_cert'),
  ('payment_channel_edit', 'payment_config_test'),
  ('payment_channel_status', 'payment_config_status'),
  ('payment_channel_del', 'payment_config_delete');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT DISTINCT rp.`role_id`, new_p.`id`, 2
FROM `role_permissions` rp
JOIN `permissions` old_p ON old_p.`id` = rp.`permission_id`
JOIN `tmp_payment_config_permission_map` m ON m.`old_code` = old_p.`code`
JOIN `permissions` new_p ON new_p.`platform` = 'admin'
  AND new_p.`is_del` = 2
  AND new_p.`code` = m.`new_code`
WHERE rp.`is_del` = 2
  AND old_p.`is_del` = 2
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

UPDATE `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = CURRENT_TIMESTAMP
WHERE p.`platform` = 'admin'
  AND p.`code` IN (
    'payment_channel_list',
    'payment_channel_add',
    'payment_channel_edit',
    'payment_channel_status',
    'payment_channel_del',
    'payment_order_list',
    'payment_order_close',
    'payment_event_list'
  );

UPDATE `permissions`
SET `is_del` = 1,
    `status` = 2,
    `show_menu` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `code` IN (
    'payment_channel_list',
    'payment_channel_add',
    'payment_channel_edit',
    'payment_channel_status',
    'payment_channel_del',
    'payment_order_list',
    'payment_order_close',
    'payment_event_list'
  );

UPDATE `cron_task`
SET `status` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `name` IN ('payment_close_expired_order', 'payment_sync_pending_order');

DROP TEMPORARY TABLE IF EXISTS `tmp_payment_config_permission_map`;

