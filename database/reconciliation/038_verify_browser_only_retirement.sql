SELECT 'browser_only_active_permission' AS invariant, COUNT(*) AS violations
FROM `permissions`
WHERE `platform` = 'admin'
  AND `is_del` = 2
  AND (
    `path` = '/system/clientVersion'
    OR `component` = 'system/clientVersion'
    OR `i18n_key` = 'menu.system_clientVersion'
    OR `code` IN (
      'system_clientVersion_add',
      'system_clientVersion_del',
      'system_clientVersion_edit',
      'system_clientVersion_forceUpdate',
      'system_clientVersion_setLatest'
    )
  );

SELECT 'browser_only_active_role_permission' AS invariant, COUNT(*) AS violations
FROM `role_permissions` AS rp
JOIN `permissions` AS p ON p.`id` = rp.`permission_id`
WHERE rp.`is_del` = 2
  AND p.`platform` = 'admin'
  AND (
    p.`path` = '/system/clientVersion'
    OR p.`component` = 'system/clientVersion'
    OR p.`i18n_key` = 'menu.system_clientVersion'
    OR p.`code` IN (
      'system_clientVersion_add',
      'system_clientVersion_del',
      'system_clientVersion_edit',
      'system_clientVersion_forceUpdate',
      'system_clientVersion_setLatest'
    )
  );

SELECT 'browser_only_client_versions_history_table_missing' AS invariant,
  IF(COUNT(*) = 1, 0, 1) AS violations
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name = 'client_versions'
  AND table_type = 'BASE TABLE';
