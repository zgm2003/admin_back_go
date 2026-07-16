-- AI image playground menu and button permissions.

SET @ai_parent_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `type` = 1
    AND `is_del` = 2
    AND (`code` = 'ai' OR `path` = '/ai' OR `i18n_key` = 'menu.ai')
  ORDER BY `id`
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT '图片工作台', '/ai/image-playground', 'Picture', @ai_parent_id, 'ai/image-playground', 'admin', 2, 7, 'ai_image_playground_page', 'menu.ai_image_playground', 1, 1, 2
WHERE @ai_parent_id IS NOT NULL
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

SET @ai_image_page_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `platform` = 'admin'
    AND `code` = 'ai_image_playground_page'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (`name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT button_name, '', '', @ai_image_page_id, '', 'admin', 3, button_sort, button_code, '', 2, 1, 2
FROM (
  SELECT '注册图片资产' AS button_name, 'ai_image_asset_add' AS button_code, 1 AS button_sort
  UNION ALL SELECT '创建图片任务', 'ai_image_task_add', 2
  UNION ALL SELECT '收藏图片任务', 'ai_image_task_favorite', 3
  UNION ALL SELECT '删除图片任务', 'ai_image_task_del', 4
) AS ai_image_buttons
WHERE @ai_image_page_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `parent_id` = VALUES(`parent_id`),
  `type` = VALUES(`type`),
  `sort` = VALUES(`sort`),
  `show_menu` = VALUES(`show_menu`),
  `status` = VALUES(`status`),
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

CREATE TEMPORARY TABLE IF NOT EXISTS `tmp_ai_image_permission_grant_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE `tmp_ai_image_permission_grant_roles`;

INSERT IGNORE INTO `tmp_ai_image_permission_grant_roles` (`role_id`)
SELECT DISTINCT rp.`role_id`
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
JOIN `roles` r ON r.`id` = rp.`role_id`
WHERE rp.`is_del` = 2
  AND p.`is_del` = 2
  AND r.`is_del` = 2
  AND p.`platform` = 'admin'
  AND p.`code` IN ('ai_agent_add', 'ai_agent_edit', 'ai_provider_add', 'ai_chat');

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT gr.`role_id`, p.`id`, 2
FROM `tmp_ai_image_permission_grant_roles` gr
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN (
    'ai_image_playground_page',
    'ai_image_asset_add',
    'ai_image_task_add',
    'ai_image_task_favorite',
    'ai_image_task_del'
  )
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_image_permission_grant_roles`;
