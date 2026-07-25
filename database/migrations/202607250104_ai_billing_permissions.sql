-- Register the Run read permission only. Role assignment is an explicit RBAC action.
DROP TEMPORARY TABLE IF EXISTS `_ai_run_permission_guard`;
CREATE TEMPORARY TABLE `_ai_run_permission_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_run_permission_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `ai_billing_migration_metadata`
WHERE `migration_key` = 'ai_billing_contract_v1' AND `phase` = 'complete';

SET @ai_billing_permissions_preexisting = (
  SELECT COUNT(*) FROM `ai_billing_migration_metadata`
  WHERE `migration_key` = 'ai_billing_permissions_v1'
);
INSERT INTO `_ai_run_permission_guard`
SELECT IF(COALESCE(@ai_billing_permissions_preexisting, 0) = 0, 0, 1);

INSERT INTO `_ai_run_permission_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(
    permission.`id` = 50
    AND permission.`name` = '运行监控'
    AND permission.`platform` = 'admin'
    AND permission.`type` = 2
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  ) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 50;

INSERT INTO `_ai_run_permission_guard`
SELECT IF(
  COUNT(*) = COALESCE(SUM(
    permission.`id` = 920
    AND BINARY permission.`code` = BINARY 'ai_run_list'
    AND permission.`name` = '查看运行记录'
    AND permission.`path` = ''
    AND permission.`icon` = ''
    AND permission.`parent_id` = 50
    AND permission.`component` IS NULL
    AND permission.`platform` = 'admin'
    AND permission.`type` = 3
    AND permission.`sort` = 1
    AND permission.`i18n_key` = ''
    AND permission.`show_menu` = 2
    AND permission.`status` = 1
    AND permission.`is_del` IN (1, 2)
  ), 0),
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 920 OR permission.`code` = 'ai_run_list';

-- All read-only permission guards passed; journal before the transaction that
-- mutates the permission row.
INSERT INTO `ai_billing_migration_metadata` (
  `migration_key`, `legacy_cutover_at`, `marker_version`, `marker_sha256`,
  `phase`, `phase_started_at`, `phase_completed_at`
)
VALUES (
  'ai_billing_permissions_v1', CURRENT_TIMESTAMP(6), 'ai_billing_permissions_v1',
  UNHEX(SHA2('ai_billing_permissions_v1', 256)), 'started', CURRENT_TIMESTAMP(6), NULL
);

START TRANSACTION;

SELECT permission.`id`
FROM `permissions` AS permission
WHERE permission.`id` IN (50, 920) OR permission.`code` = 'ai_run_list'
FOR UPDATE;

INSERT INTO `_ai_run_permission_guard`
SELECT IF(
  COUNT(*) = COALESCE(SUM(
    permission.`id` = 920
    AND BINARY permission.`code` = BINARY 'ai_run_list'
    AND permission.`name` = '查看运行记录'
    AND permission.`path` = ''
    AND permission.`icon` = ''
    AND permission.`parent_id` = 50
    AND permission.`component` IS NULL
    AND permission.`platform` = 'admin'
    AND permission.`type` = 3
    AND permission.`sort` = 1
    AND permission.`i18n_key` = ''
    AND permission.`show_menu` = 2
    AND permission.`status` = 1
    AND permission.`is_del` IN (1, 2)
  ), 0),
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 920 OR permission.`code` = 'ai_run_list';

INSERT INTO `permissions`
  (`id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`, `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`)
SELECT 920, '查看运行记录', '', '', 50, NULL, 'admin', 3, 1, 'ai_run_list', '', 2, 1, 2
WHERE NOT EXISTS (
  SELECT 1 FROM `permissions` WHERE `id` = 920 OR `code` = 'ai_run_list'
);

UPDATE `permissions`
SET `is_del` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `id` = 920 AND BINARY `code` = BINARY 'ai_run_list' AND `is_del` = 1;

INSERT INTO `_ai_run_permission_guard`
SELECT IF(
  COUNT(*) = 1
  AND SUM(
    permission.`id` = 920
    AND BINARY permission.`code` = BINARY 'ai_run_list'
    AND permission.`parent_id` = 50
    AND permission.`type` = 3
    AND permission.`status` = 1
    AND permission.`is_del` = 2
  ) = 1,
  0,
  1
)
FROM `permissions` AS permission
WHERE permission.`id` = 920 OR permission.`code` = 'ai_run_list';

UPDATE `ai_billing_migration_metadata`
SET `phase` = 'complete', `phase_completed_at` = CURRENT_TIMESTAMP(6)
WHERE `migration_key` = 'ai_billing_permissions_v1' AND `phase` = 'started';

COMMIT;
DROP TEMPORARY TABLE `_ai_run_permission_guard`;
