CREATE TABLE IF NOT EXISTS `sms_configs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `config_key` VARCHAR(32) NOT NULL DEFAULT 'default',
  `secret_id_enc` TEXT NOT NULL,
  `secret_id_hint` VARCHAR(64) NOT NULL DEFAULT '',
  `secret_key_enc` TEXT NOT NULL,
  `secret_key_hint` VARCHAR(64) NOT NULL DEFAULT '',
  `sms_sdk_app_id` VARCHAR(32) NOT NULL DEFAULT '',
  `sign_name` VARCHAR(128) NOT NULL DEFAULT '',
  `region` VARCHAR(64) NOT NULL DEFAULT 'ap-guangzhou',
  `endpoint` VARCHAR(128) NOT NULL DEFAULT 'sms.tencentcloudapi.com',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 2,
  `last_test_at` DATETIME NULL,
  `last_test_error` VARCHAR(500) NOT NULL DEFAULT '',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sms_configs_config_key` (`config_key`),
  KEY `idx_sms_configs_status_del` (`status`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `sms_templates` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `scene` VARCHAR(32) NOT NULL,
  `name` VARCHAR(100) NOT NULL,
  `tencent_template_id` VARCHAR(32) NOT NULL,
  `variables_json` JSON NOT NULL,
  `sample_variables_json` JSON NOT NULL,
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1,
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sms_templates_scene` (`scene`),
  KEY `idx_sms_templates_status_del` (`status`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `sms_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `scene` VARCHAR(32) NOT NULL,
  `template_id` BIGINT UNSIGNED NULL,
  `to_phone` VARCHAR(32) NOT NULL,
  `status` TINYINT UNSIGNED NOT NULL,
  `tencent_request_id` VARCHAR(128) NOT NULL DEFAULT '',
  `tencent_serial_no` VARCHAR(128) NOT NULL DEFAULT '',
  `tencent_fee` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `error_code` VARCHAR(128) NOT NULL DEFAULT '',
  `error_message` VARCHAR(500) NOT NULL DEFAULT '',
  `duration_ms` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `sent_at` DATETIME NULL,
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_sms_logs_scene_created` (`is_del`, `scene`, `created_at`),
  KEY `idx_sms_logs_status_created` (`is_del`, `status`, `created_at`),
  KEY `idx_sms_logs_to_phone_created` (`is_del`, `to_phone`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @system_parent_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND (`i18n_key` = 'menu.system' OR `path` = '/system' OR `code` = 'system')
  ORDER BY `id`
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '短信管理', '/system/sms', 'ChatDotRound', @system_parent_id, 'system/sms', 'admin', 2, 91, 'system_sms', 'menu.system_sms', 1, 1, 2
WHERE @system_parent_id IS NOT NULL
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
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

SET @sms_page_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'system_sms'
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @sms_page_id, NULL, 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '编辑短信配置' AS button_name, 'system_sms_configEdit' AS button_code, 1 AS button_sort
  UNION ALL SELECT '删除短信配置', 'system_sms_configDel', 2
  UNION ALL SELECT '发送测试短信', 'system_sms_test', 3
  UNION ALL SELECT '新增短信模板', 'system_sms_templateAdd', 4
  UNION ALL SELECT '编辑短信模板', 'system_sms_templateEdit', 5
  UNION ALL SELECT '修改短信模板状态', 'system_sms_templateStatus', 6
  UNION ALL SELECT '删除短信模板', 'system_sms_templateDel', 7
  UNION ALL SELECT '删除短信日志', 'system_sms_logDel', 8
) AS sms_buttons
WHERE @sms_page_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = VALUES(`show_menu`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_sms_permission_grant_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_sms_permission_grant_roles`;

INSERT IGNORE INTO `tmp_sms_permission_grant_roles` (`role_id`)
SELECT DISTINCT rp.`role_id`
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
JOIN `roles` r ON r.`id` = rp.`role_id`
WHERE rp.`is_del` = 2
  AND p.`is_del` = 2
  AND r.`is_del` = 2
  AND p.`platform` = 'admin'
  AND p.`code` IN ('system_mail', 'system_setting_edit', 'system_uploadConfig_settingEdit');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT gr.`role_id`, p.`id`, 2
FROM `tmp_sms_permission_grant_roles` gr
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN (
    'system_sms',
    'system_sms_configEdit',
    'system_sms_configDel',
    'system_sms_test',
    'system_sms_templateAdd',
    'system_sms_templateEdit',
    'system_sms_templateStatus',
    'system_sms_templateDel',
    'system_sms_logDel'
  )
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TEMPORARY TABLE IF EXISTS `tmp_sms_permission_grant_roles`;
