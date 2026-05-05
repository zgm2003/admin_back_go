-- Add RBAC button permissions for Go notification-task mutations.
--
-- This is a data migration, not a table-structure migration.
-- Route metadata now protects:
--   POST   /api/admin/v1/notification-tasks             -> system_notificationTask_add
--   PATCH  /api/admin/v1/notification-tasks/:id/cancel  -> system_notificationTask_cancel
--   DELETE /api/admin/v1/notification-tasks/:id         -> system_notificationTask_del
--
-- To avoid breaking roles that already owned the notification-task page, grant
-- the new buttons to roles that currently own the page permission.

SET @notification_task_page_id := (
    SELECT `id`
    FROM `permissions`
    WHERE `platform` = 'admin'
      AND `is_del` = 2
      AND (`path` = '/system/notificationTask' OR `i18n_key` = 'menu.system_notificationTask')
    ORDER BY `id` ASC
    LIMIT 1
);

INSERT INTO `permissions` (
    `name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`, `created_at`, `updated_at`
)
SELECT
    item.`name`, '', '', @notification_task_page_id, NULL, 'admin', 3, item.`sort`, item.`code`, '', 2, 1, 2, NOW(), NOW()
FROM (
    SELECT '发布通知' AS `name`, 'system_notificationTask_add' AS `code`, 1 AS `sort`
    UNION ALL SELECT '取消任务', 'system_notificationTask_cancel', 2
    UNION ALL SELECT '删除任务', 'system_notificationTask_del', 3
) item
WHERE @notification_task_page_id IS NOT NULL
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
    `updated_at` = NOW();

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`, `created_at`, `updated_at`)
SELECT DISTINCT
    page_grant.`role_id`, button.`id`, 2, NOW(), NOW()
FROM `role_permissions` page_grant
JOIN `permissions` button
  ON button.`platform` = 'admin'
 AND button.`code` IN (
      'system_notificationTask_add',
      'system_notificationTask_cancel',
      'system_notificationTask_del'
 )
 AND button.`is_del` = 2
WHERE page_grant.`permission_id` = @notification_task_page_id
  AND page_grant.`is_del` = 2
ON DUPLICATE KEY UPDATE
    `is_del` = VALUES(`is_del`),
    `updated_at` = NOW();
