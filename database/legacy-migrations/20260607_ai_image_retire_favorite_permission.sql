-- Retire Admin AI image favorite button permission.
--
-- The public Admin AI image favorite mutation route was removed.
-- Keep ai_image_tasks.is_favorite for schema compatibility, but remove the
-- dead RBAC button and grants so fresh or already-migrated DBs do not expose a
-- permission with no route behind it.

UPDATE `role_permissions` AS rp
JOIN `permissions` AS p ON p.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = CURRENT_TIMESTAMP
WHERE p.`platform` = 'admin'
  AND p.`code` = 'ai_image_task_favorite'
  AND rp.`is_del` = 2;

UPDATE `permissions`
SET `is_del` = 1,
    `status` = 2,
    `show_menu` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `code` = 'ai_image_task_favorite'
  AND `is_del` = 2;
