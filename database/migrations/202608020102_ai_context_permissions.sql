-- Replace legacy Knowledge permissions with the final Context Engineering RBAC surface.
-- All role mappings are computed and validated before persistent rows change.
DROP TEMPORARY TABLE IF EXISTS `_ai_context_permission_guard`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_principal_versions_before`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_profile_roles`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_evaluate_roles`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_document_roles`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_manage_roles`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_view_roles`;
DROP TEMPORARY TABLE IF EXISTS `_ai_context_affected_roles`;

CREATE TEMPORARY TABLE `_ai_context_affected_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
);
CREATE TEMPORARY TABLE `_ai_context_view_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
);
CREATE TEMPORARY TABLE `_ai_context_manage_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
);
CREATE TEMPORARY TABLE `_ai_context_document_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
);
CREATE TEMPORARY TABLE `_ai_context_evaluate_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
);
CREATE TEMPORARY TABLE `_ai_context_profile_roles` (
  `role_id` INT UNSIGNED NOT NULL PRIMARY KEY
);
CREATE TEMPORARY TABLE `_ai_context_principal_versions_before` (
  `user_id` INT UNSIGNED NOT NULL PRIMARY KEY,
  `affected` TINYINT UNSIGNED NOT NULL,
  `version_was_present` TINYINT UNSIGNED NOT NULL,
  `version_before` BIGINT UNSIGNED NULL,
  `updated_at_before` DATETIME(6) NULL
);
CREATE TEMPORARY TABLE `_ai_context_permission_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

START TRANSACTION;

SELECT permission.`id`
FROM `permissions` AS permission
WHERE permission.`id` IN (122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415, 923, 924, 925, 926, 927)
   OR (
     permission.`platform` = 'admin'
     AND permission.`code` IN (
       'ai_context', 'ai_context_view', 'ai_context_manage',
       'ai_context_document_manage', 'ai_context_profile_manage', 'ai_context_evaluate'
     )
   )
FOR UPDATE;

SELECT role_permission.`id`
FROM `role_permissions` AS role_permission
WHERE role_permission.`permission_id` IN (122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415, 923, 924, 925, 926, 927)
FOR UPDATE;

INSERT INTO `_ai_context_affected_roles` (`role_id`)
SELECT DISTINCT role_permission.`role_id`
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE role_permission.`permission_id` IN (122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415)
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2;

INSERT INTO `_ai_context_view_roles` (`role_id`)
SELECT DISTINCT role_permission.`role_id`
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`id` IN (122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 415)
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2;

INSERT INTO `_ai_context_manage_roles` (`role_id`)
SELECT role_permission.`role_id`
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`id` IN (128, 129, 130, 131)
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2
GROUP BY role_permission.`role_id`
HAVING COUNT(DISTINCT permission.`id`) = 4;

INSERT INTO `_ai_context_document_roles` (`role_id`)
SELECT role_permission.`role_id`
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`id` IN (124, 125, 126, 127, 415)
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2
GROUP BY role_permission.`role_id`
HAVING COUNT(DISTINCT permission.`id`) = 5;

INSERT INTO `_ai_context_evaluate_roles` (`role_id`)
SELECT DISTINCT role_permission.`role_id`
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`id` = 123
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2;

INSERT INTO `_ai_context_profile_roles` (`role_id`)
SELECT manage_role.`role_id`
FROM `_ai_context_manage_roles` AS manage_role
JOIN `role_permissions` AS role_permission ON role_permission.`role_id` = manage_role.`role_id`
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`id` = 413
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2;

-- Reject partial base-management grants before any persistent write.
INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT role_permission.`role_id`
  FROM `role_permissions` AS role_permission
  JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
  WHERE permission.`id` IN (128, 129, 130, 131)
    AND role_permission.`is_del` = 2
    AND permission.`platform` = 'admin'
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  GROUP BY role_permission.`role_id`
  HAVING COUNT(DISTINCT permission.`id`) BETWEEN 1 AND 3
) AS partial_manage_role;

-- Reject partial document-management grants before any persistent write.
INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT role_permission.`role_id`
  FROM `role_permissions` AS role_permission
  JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
  WHERE permission.`id` IN (124, 125, 126, 127, 415)
    AND role_permission.`is_del` = 2
    AND permission.`platform` = 'admin'
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  GROUP BY role_permission.`role_id`
  HAVING COUNT(DISTINCT permission.`id`) BETWEEN 1 AND 4
) AS partial_document_role;

-- A binding grant has no Context meaning without the complete base-manage set.
INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
LEFT JOIN `_ai_context_manage_roles` AS manage_role ON manage_role.`role_id` = role_permission.`role_id`
WHERE permission.`id` = 413
  AND role_permission.`is_del` = 2
  AND permission.`platform` = 'admin'
  AND permission.`status` = 1
  AND permission.`is_del` = 2
  AND manage_role.`role_id` IS NULL;

INSERT INTO `_ai_context_permission_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(permission.`platform` = 'admin' AND permission.`type` = 2) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 122;

INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions` AS permission
WHERE permission.`id` IN (923, 924, 925, 926, 927)
   OR (
     permission.`platform` = 'admin'
     AND permission.`code` IN (
       'ai_context', 'ai_context_view', 'ai_context_manage',
       'ai_context_document_manage', 'ai_context_profile_manage', 'ai_context_evaluate'
     )
   );

INSERT INTO `_ai_context_principal_versions_before` (
  `user_id`, `affected`, `version_was_present`, `version_before`, `updated_at_before`
)
SELECT
  user_row.`id`,
  IF(affected_role.`role_id` IS NULL, 0, 1),
  IF(principal_version.`user_id` IS NULL, 0, 1),
  principal_version.`version`,
  principal_version.`updated_at`
FROM `users` AS user_row
LEFT JOIN `_ai_context_affected_roles` AS affected_role ON affected_role.`role_id` = user_row.`role_id`
LEFT JOIN `authz_principal_versions` AS principal_version
  ON principal_version.`user_id` = user_row.`id`
 AND principal_version.`platform` = 'admin'
WHERE user_row.`status` = 1
  AND user_row.`is_del` = 2;

UPDATE `permissions`
SET `name` = '上下文工程',
    `path` = '/ai/context',
    `icon` = 'Collection',
    `component` = 'ai/context',
    `code` = 'ai_context',
    `i18n_key` = 'menu.ai_context',
    `sort` = 3,
    `show_menu` = 1,
    `status` = 1,
    `is_del` = 2
WHERE `id` = 122
  AND `platform` = 'admin'
  AND `type` = 2;

INSERT INTO `permissions` (
  `id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`,
  `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`
) VALUES
  (923, '查看上下文工程', '', '', 122, NULL, 'admin', 3, 1, 'ai_context_view', '', 2, 1, 2),
  (924, '管理上下文空间', '', '', 122, NULL, 'admin', 3, 2, 'ai_context_manage', '', 2, 1, 2),
  (925, '管理上下文文档', '', '', 122, NULL, 'admin', 3, 3, 'ai_context_document_manage', '', 2, 1, 2),
  (926, '管理上下文配置', '', '', 122, NULL, 'admin', 3, 4, 'ai_context_profile_manage', '', 2, 1, 2),
  (927, '执行上下文评测', '', '', 122, NULL, 'admin', 3, 5, 'ai_context_evaluate', '', 2, 1, 2);

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT `role_id`, 923, 2 FROM `_ai_context_view_roles`
ON DUPLICATE KEY UPDATE `is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT `role_id`, 924, 2 FROM `_ai_context_manage_roles`
ON DUPLICATE KEY UPDATE `is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT `role_id`, 925, 2 FROM `_ai_context_document_roles`
ON DUPLICATE KEY UPDATE `is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT `role_id`, 926, 2 FROM `_ai_context_profile_roles`
ON DUPLICATE KEY UPDATE `is_del` = 2;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT `role_id`, 927, 2 FROM `_ai_context_evaluate_roles`
ON DUPLICATE KEY UPDATE `is_del` = 2;

DELETE FROM `role_permissions`
WHERE `permission_id` IN (123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415);

DELETE FROM `permissions`
WHERE `id` IN (123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415)
  AND `platform` = 'admin';

INSERT INTO `authz_principal_versions` (`user_id`, `platform`, `version`, `updated_at`)
SELECT DISTINCT user_row.`id`, 'admin', 1, UTC_TIMESTAMP(6)
FROM `users` AS user_row
JOIN `_ai_context_affected_roles` AS affected_role
  ON affected_role.`role_id` = user_row.`role_id`
WHERE user_row.`status` = 1
  AND user_row.`is_del` = 2
ON DUPLICATE KEY UPDATE `user_id` = `authz_principal_versions`.`user_id`;

UPDATE `authz_principal_versions` AS principal_version
JOIN `users` AS user_row ON user_row.`id` = principal_version.`user_id`
JOIN `_ai_context_affected_roles` AS affected_role
  ON affected_role.`role_id` = user_row.`role_id`
SET principal_version.`version` = principal_version.`version` + 1,
    principal_version.`updated_at` = UTC_TIMESTAMP(6)
WHERE principal_version.`platform` = 'admin'
  AND user_row.`status` = 1
  AND user_row.`is_del` = 2;

-- Final permission and retirement assertions.
INSERT INTO `_ai_context_permission_guard`
SELECT IF(
  COUNT(*) = 5
  AND SUM(
    (permission.`id` = 923 AND permission.`code` = 'ai_context_view')
    OR (permission.`id` = 924 AND permission.`code` = 'ai_context_manage')
    OR (permission.`id` = 925 AND permission.`code` = 'ai_context_document_manage')
    OR (permission.`id` = 926 AND permission.`code` = 'ai_context_profile_manage')
    OR (permission.`id` = 927 AND permission.`code` = 'ai_context_evaluate')
  ) = 5,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` IN (923, 924, 925, 926, 927)
  AND permission.`parent_id` = 122
  AND permission.`platform` = 'admin'
  AND permission.`type` = 3
  AND permission.`status` = 1
  AND permission.`is_del` = 2;

INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `id` IN (123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415);

INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `role_permissions`
WHERE `permission_id` IN (123, 124, 125, 126, 127, 128, 129, 130, 131, 413, 415);

-- Every affected principal increments exactly once, including a missing row
-- which is created at version 1 and then receives the single cutover bump.
INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `_ai_context_principal_versions_before` AS snapshot
LEFT JOIN `authz_principal_versions` AS current_version
  ON current_version.`user_id` = snapshot.`user_id`
 AND current_version.`platform` = 'admin'
WHERE snapshot.`affected` = 1
  AND (
    current_version.`user_id` IS NULL
    OR current_version.`version` <> COALESCE(snapshot.`version_before`, 1) + 1
  );

-- Existing and missing version rows for unaffected active Admin users remain
-- byte-for-byte unchanged.
INSERT INTO `_ai_context_permission_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `_ai_context_principal_versions_before` AS snapshot
LEFT JOIN `authz_principal_versions` AS current_version
  ON current_version.`user_id` = snapshot.`user_id`
 AND current_version.`platform` = 'admin'
WHERE snapshot.`affected` = 0
  AND (
    (snapshot.`version_was_present` = 0 AND current_version.`user_id` IS NOT NULL)
    OR (
      snapshot.`version_was_present` = 1
      AND (
        current_version.`user_id` IS NULL
        OR NOT (current_version.`version` <=> snapshot.`version_before`)
        OR NOT (current_version.`updated_at` <=> snapshot.`updated_at_before`)
      )
    )
  );

COMMIT;

DROP TEMPORARY TABLE `_ai_context_permission_guard`;
DROP TEMPORARY TABLE `_ai_context_principal_versions_before`;
DROP TEMPORARY TABLE `_ai_context_profile_roles`;
DROP TEMPORARY TABLE `_ai_context_evaluate_roles`;
DROP TEMPORARY TABLE `_ai_context_document_roles`;
DROP TEMPORARY TABLE `_ai_context_manage_roles`;
DROP TEMPORARY TABLE `_ai_context_view_roles`;
DROP TEMPORARY TABLE `_ai_context_affected_roles`;
