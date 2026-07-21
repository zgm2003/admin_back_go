SELECT 'client_versions_table_remaining' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'client_versions';

SELECT 'platform_kernel_schema_missing' AS invariant, COALESCE(SUM(`missing`), 0) AS violations
FROM (
  SELECT IF(COUNT(*) = 1, 0, 1) AS `missing`
  FROM `information_schema`.`tables`
  WHERE `table_schema` = DATABASE() AND `table_name` = 'auth_platforms'
  UNION ALL
  SELECT IF(COUNT(*) >= 1, 0, 1)
  FROM `information_schema`.`statistics`
  WHERE `table_schema` = DATABASE() AND `table_name` = 'auth_platforms' AND `column_name` = 'code'
  UNION ALL
  SELECT IF(COUNT(*) = 5, 0, 1)
  FROM `information_schema`.`columns`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'auth_platforms'
    AND `column_name` IN ('code', 'login_types', 'captcha_type', 'access_ttl', 'refresh_ttl')
  UNION ALL
  SELECT IF(COUNT(*) = 12, 0, 1)
  FROM `information_schema`.`columns`
  WHERE `table_schema` = DATABASE()
    AND `table_name` IN ('permissions', 'authz_principal_versions', 'user_sessions', 'users_login_log', 'notification_task', 'notifications', 'export_tasks', 'ai_runs', 'ai_text_tasks', 'ai_image_tasks', 'ai_video_tasks', 'ai_reply_commands')
    AND `column_name` = 'platform'
  UNION ALL
  SELECT IF(COUNT(*) >= 10, 0, 1)
  FROM `information_schema`.`statistics`
  WHERE `table_schema` = DATABASE()
    AND `table_name` IN ('permissions', 'authz_principal_versions', 'user_sessions', 'users_login_log', 'notification_task', 'notifications', 'export_tasks', 'ai_runs', 'ai_text_tasks', 'ai_image_tasks')
    AND `column_name` = 'platform'
) AS kernel_requirement;

SELECT 'unconfigured_platform_provenance_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations
  FROM `permissions` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `authz_principal_versions` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `user_sessions` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `users_login_log` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `notification_task` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE row_data.`platform` <> 'all' AND platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `notifications` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE row_data.`platform` <> 'all' AND platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `export_tasks` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `ai_runs` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `ai_text_tasks` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `ai_image_tasks` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `ai_video_tasks` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
  UNION ALL
  SELECT COUNT(*)
  FROM `ai_reply_commands` AS row_data
  LEFT JOIN `auth_platforms` AS platform_row ON platform_row.`code` COLLATE utf8mb4_0900_ai_ci = row_data.`platform` COLLATE utf8mb4_0900_ai_ci AND platform_row.`is_del` = 2
  WHERE platform_row.`id` IS NULL
) AS unconfigured_platform;

SELECT 'admin_only_check_constraint_violations' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`check_constraints`
WHERE `constraint_schema` = DATABASE()
  AND (
    LOWER(`check_clause`) REGEXP 'platform`? *= *''admin'''
    OR LOWER(`check_clause`) REGEXP 'code`? *= *''admin'''
  );

SELECT 'retired_platform_constraint_missing' AS invariant,
  GREATEST(13 - COUNT(DISTINCT `constraint_name`), 0) AS violations
FROM `information_schema`.`check_constraints`
WHERE `constraint_schema` = DATABASE()
  AND `constraint_name` IN (
    'chk_auth_platforms_code',
    'chk_permissions_platform',
    'chk_authz_principal_platform',
    'chk_user_sessions_platform',
    'chk_users_login_log_platform',
    'chk_notification_task_platform',
    'chk_notifications_platform',
    'chk_export_tasks_platform',
    'chk_ai_runs_platform',
    'chk_ai_text_tasks_platform',
    'chk_ai_image_tasks_platform',
    'chk_ai_video_platform',
    'chk_ai_reply_platform'
  )
  AND LOWER(`check_clause`) LIKE '%app%'
  AND LOWER(`check_clause`) LIKE '%canvas%';

SELECT 'active_retired_product_data_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations FROM `auth_platforms` WHERE `code` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `permissions` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `authz_principal_versions` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `user_sessions` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `users_login_log` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `notification_task` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `notifications` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `export_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_runs` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_text_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_image_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_video_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_reply_commands` WHERE `platform` IN ('app', 'canvas')
) AS retired_product;
