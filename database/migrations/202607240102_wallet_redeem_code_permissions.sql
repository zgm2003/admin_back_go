-- Install the Admin wallet redeem-code permissions. Role assignment remains an
-- explicit RBAC administration action and is intentionally outside migration.
DROP TEMPORARY TABLE IF EXISTS `_wallet_redeem_code_permission_guard`;
CREATE TEMPORARY TABLE `_wallet_redeem_code_permission_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

DROP TEMPORARY TABLE IF EXISTS `_wallet_redeem_code_desired_permissions`;
CREATE TEMPORARY TABLE `_wallet_redeem_code_desired_permissions` (
  `id` INT UNSIGNED NOT NULL PRIMARY KEY,
  `name` VARCHAR(50) NOT NULL,
  `path` VARCHAR(255) NOT NULL,
  `icon` VARCHAR(100) NOT NULL,
  `parent_id` INT UNSIGNED NOT NULL,
  `component` VARCHAR(255) NULL,
  `platform` VARCHAR(10) NOT NULL,
  `type` TINYINT UNSIGNED NOT NULL,
  `sort` INT UNSIGNED NOT NULL,
  `code` VARCHAR(100) NOT NULL,
  `i18n_key` VARCHAR(128) NOT NULL,
  `show_menu` TINYINT UNSIGNED NOT NULL,
  `status` TINYINT UNSIGNED NOT NULL,
  `is_del` TINYINT UNSIGNED NOT NULL,
  UNIQUE KEY `uk_wallet_redeem_code_desired_permission_code` (`code`)
);

INSERT INTO `_wallet_redeem_code_desired_permissions`
  (`id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
VALUES
  (912, '兑换码管理', '/payment/redeem-codes', 'Ticket', 437, 'payment/redeem-codes', 'admin', 2, 35, 'payment_redeem_code_list', 'menu.payment_redeem_codes', 1, 1, 2),
  (913, '批量生成兑换码', '', '', 912, NULL, 'admin', 3, 1, 'payment_redeem_code_generate', '', 2, 1, 2),
  (914, '作废兑换码', '', '', 912, NULL, 'admin', 3, 2, 'payment_redeem_code_void', '', 2, 1, 2);

INSERT INTO `_wallet_redeem_code_permission_guard`
SELECT IF(COUNT(*) = 2, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN ('redeem_code_batches', 'redeem_codes')
  AND `table_type` = 'BASE TABLE';

INSERT INTO `_wallet_redeem_code_permission_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(
    permission.`id` = 437
    AND permission.`name` = '支付管理'
    AND permission.`path` = '/payment'
    AND permission.`icon` = 'CreditCard'
    AND permission.`parent_id` = 0
    AND permission.`component` = ''
    AND permission.`platform` = 'admin'
    AND permission.`type` = 1
    AND permission.`sort` = 40
    AND BINARY permission.`code` = BINARY 'payment'
    AND permission.`i18n_key` = 'menu.payment'
    AND permission.`show_menu` = 1
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  ) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 437 OR permission.`code` = 'payment';

INSERT INTO `_wallet_redeem_code_permission_guard`
SELECT IF(
  COUNT(*) = COALESCE(SUM(
    permission.`id` = desired.`id`
    AND permission.`name` = desired.`name`
    AND permission.`path` = desired.`path`
    AND permission.`icon` = desired.`icon`
    AND permission.`parent_id` = desired.`parent_id`
    AND permission.`component` <=> desired.`component`
    AND permission.`platform` = desired.`platform`
    AND permission.`type` = desired.`type`
    AND permission.`sort` = desired.`sort`
    AND BINARY permission.`code` = BINARY desired.`code`
    AND permission.`i18n_key` = desired.`i18n_key`
    AND permission.`show_menu` = desired.`show_menu`
    AND permission.`status` = desired.`status`
    AND desired.`is_del` = 2
    AND permission.`is_del` IN (1, 2)
  ), 0),
  0,
  1
)
FROM `permissions` AS permission
JOIN `_wallet_redeem_code_desired_permissions` AS desired
  ON permission.`id` = desired.`id` OR permission.`code` = desired.`code`
WHERE permission.`id` IN (912, 913, 914)
   OR permission.`code` IN ('payment_redeem_code_list', 'payment_redeem_code_generate', 'payment_redeem_code_void');

START TRANSACTION;

SELECT permission.`id`
FROM `permissions` AS permission
WHERE permission.`id` IN (437, 912, 913, 914)
   OR permission.`code` IN ('payment', 'payment_redeem_code_list', 'payment_redeem_code_generate', 'payment_redeem_code_void')
FOR UPDATE;

-- Repeat mutable-row checks after acquiring locks.
INSERT INTO `_wallet_redeem_code_permission_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(
    permission.`id` = 437
    AND permission.`name` = '支付管理'
    AND permission.`path` = '/payment'
    AND permission.`icon` = 'CreditCard'
    AND permission.`parent_id` = 0
    AND permission.`component` = ''
    AND permission.`platform` = 'admin'
    AND permission.`type` = 1
    AND permission.`sort` = 40
    AND BINARY permission.`code` = BINARY 'payment'
    AND permission.`i18n_key` = 'menu.payment'
    AND permission.`show_menu` = 1
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  ) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 437 OR permission.`code` = 'payment';

INSERT INTO `_wallet_redeem_code_permission_guard`
SELECT IF(
  COUNT(*) = COALESCE(SUM(
    permission.`id` = desired.`id`
    AND permission.`name` = desired.`name`
    AND permission.`path` = desired.`path`
    AND permission.`icon` = desired.`icon`
    AND permission.`parent_id` = desired.`parent_id`
    AND permission.`component` <=> desired.`component`
    AND permission.`platform` = desired.`platform`
    AND permission.`type` = desired.`type`
    AND permission.`sort` = desired.`sort`
    AND BINARY permission.`code` = BINARY desired.`code`
    AND permission.`i18n_key` = desired.`i18n_key`
    AND permission.`show_menu` = desired.`show_menu`
    AND permission.`status` = desired.`status`
    AND desired.`is_del` = 2
    AND permission.`is_del` IN (1, 2)
  ), 0),
  0,
  1
)
FROM `permissions` AS permission
JOIN `_wallet_redeem_code_desired_permissions` AS desired
  ON permission.`id` = desired.`id` OR permission.`code` = desired.`code`
WHERE permission.`id` IN (912, 913, 914)
   OR permission.`code` IN ('payment_redeem_code_list', 'payment_redeem_code_generate', 'payment_redeem_code_void');

INSERT INTO `permissions`
  (`id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT
  desired.`id`, desired.`name`, desired.`path`, desired.`icon`, desired.`parent_id`, desired.`component`, desired.`platform`,
  desired.`type`, desired.`sort`, desired.`code`, desired.`i18n_key`, desired.`show_menu`, desired.`status`, desired.`is_del`
FROM `_wallet_redeem_code_desired_permissions` AS desired
LEFT JOIN `permissions` AS permission
  ON permission.`id` = desired.`id` OR permission.`code` = desired.`code`
WHERE permission.`id` IS NULL;

UPDATE `permissions`
SET `is_del` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `id` IN (912, 913, 914)
  AND `is_del` = 1;

INSERT INTO `_wallet_redeem_code_permission_guard`
SELECT IF(
  COUNT(*) = 3
  AND SUM(
    permission.`id` = desired.`id`
    AND permission.`name` = desired.`name`
    AND permission.`path` = desired.`path`
    AND permission.`icon` = desired.`icon`
    AND permission.`parent_id` = desired.`parent_id`
    AND permission.`component` <=> desired.`component`
    AND permission.`platform` = desired.`platform`
    AND permission.`type` = desired.`type`
    AND permission.`sort` = desired.`sort`
    AND BINARY permission.`code` = BINARY desired.`code`
    AND permission.`i18n_key` = desired.`i18n_key`
    AND permission.`show_menu` = desired.`show_menu`
    AND permission.`status` = desired.`status`
    AND permission.`is_del` = desired.`is_del`
  ) = 3,
  0,
  1
)
FROM `_wallet_redeem_code_desired_permissions` AS desired
JOIN `permissions` AS permission ON permission.`id` = desired.`id`;

COMMIT;

DROP TEMPORARY TABLE `_wallet_redeem_code_desired_permissions`;
DROP TEMPORARY TABLE `_wallet_redeem_code_permission_guard`;
