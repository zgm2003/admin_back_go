-- AI tool management: AI-generate button permission.
-- Scope: permission/menu data only. The generate endpoint returns a draft and never writes ai_tools.

SET @ai_tools_page_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `path` = '/ai/tools'
    AND `type` = 2
    AND `platform` = 'admin'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `permissions` (
  `name`,
  `path`,
  `icon`,
  `parent_id`,
  `component`,
  `platform`,
  `type`,
  `sort`,
  `code`,
  `i18n_key`,
  `show_menu`,
  `status`,
  `is_del`
)
SELECT
  'AI生成',
  '',
  '',
  @ai_tools_page_id,
  NULL,
  'admin',
  3,
  5,
  'ai_tool_generate',
  '',
  2,
  1,
  2
WHERE @ai_tools_page_id IS NOT NULL
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
  `is_del` = VALUES(`is_del`);

SET @ai_tool_generate_id := (
  SELECT `id`
  FROM `permissions`
  WHERE `code` = 'ai_tool_generate'
    AND `platform` = 'admin'
    AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT DISTINCT rp.`role_id`, @ai_tool_generate_id, 2
FROM `role_permissions` AS rp
JOIN `permissions` AS p ON p.`id` = rp.`permission_id`
WHERE @ai_tool_generate_id IS NOT NULL
  AND p.`code` = 'ai_tool_add'
  AND p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND rp.`is_del` = 2
ON DUPLICATE KEY UPDATE
  `is_del` = VALUES(`is_del`),
  `updated_at` = CURRENT_TIMESTAMP;
