-- Add Canvas recharge page permission and keep Canvas labels UTF-8/idempotent.
SET NAMES utf8mb4;

UPDATE `auth_platforms`
SET `name` = '无限画布',
    `updated_at` = CURRENT_TIMESTAMP
WHERE `code` = 'canvas';

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '充值', '/recharge', 'CreditCard', 0, 'recharge', 'canvas', 2, 80, 'canvas_recharge_page', 'menu.canvas_recharge', 2, 1, 2
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
  `is_del` = VALUES(`is_del`),
  `updated_at` = CURRENT_TIMESTAMP;

SET @canvas_wallet_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_wallet_page' AND `is_del` = 2
  LIMIT 1
);

SET @canvas_recharge_page_id := (
  SELECT `id` FROM `permissions`
  WHERE `platform` = 'canvas' AND `code` = 'canvas_recharge_page' AND `is_del` = 2
  LIMIT 1
);

UPDATE `permissions`
SET `name` = CASE `code`
    WHEN 'canvas_profile_page' THEN '个人资料'
    WHEN 'canvas_wallet_page' THEN '我的钱包'
    WHEN 'canvas_recharge_page' THEN '充值'
    WHEN 'canvas_wallet_read' THEN '读取钱包'
    WHEN 'canvas_recharge_add' THEN '创建充值'
    WHEN 'canvas_recharge_pay' THEN '支付充值'
    ELSE `name`
  END,
  `parent_id` = CASE
    WHEN `code` = 'canvas_wallet_read' AND @canvas_wallet_page_id IS NOT NULL THEN @canvas_wallet_page_id
    WHEN `code` IN ('canvas_recharge_add', 'canvas_recharge_pay') AND @canvas_recharge_page_id IS NOT NULL THEN @canvas_recharge_page_id
    ELSE `parent_id`
  END,
  `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'canvas'
  AND `is_del` = 2
  AND `code` IN (
    'canvas_profile_page',
    'canvas_wallet_page',
    'canvas_recharge_page',
    'canvas_wallet_read',
    'canvas_recharge_add',
    'canvas_recharge_pay'
  );

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT r.`id`, p.`id`, 2
FROM `roles` r
JOIN `permissions` p
  ON p.`platform` = 'canvas'
 AND p.`code` IN ('canvas_recharge_page', 'canvas_recharge_add', 'canvas_recharge_pay')
 AND p.`is_del` = 2
 AND p.`status` = 1
LEFT JOIN `role_permissions` rp
  ON rp.`role_id` = r.`id`
 AND rp.`permission_id` = p.`id`
 AND rp.`is_del` = 2
WHERE r.`is_del` = 2
  AND rp.`id` IS NULL;
