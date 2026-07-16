-- Production launch cleanup for admin permission/menu data.
--
-- Scope:
-- - permissions is the runtime menu/permission table.
-- - role_permissions is the runtime role grant table.
-- - Remove template/demo/test menu rows and stale soft-deleted history rows.
-- - Keep active business menus and button grants.
--
-- Idempotent for local/prod migration replay.

CREATE TEMPORARY TABLE IF NOT EXISTS tmp_permission_launch_cleanup_ids (
  id INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

TRUNCATE TABLE tmp_permission_launch_cleanup_ids;

INSERT IGNORE INTO tmp_permission_launch_cleanup_ids (id)
SELECT id
FROM permissions
WHERE is_del = 2
  AND (
    (
      platform = 'admin'
      AND (
        -- Open-source template/demo menus, not production admin business.
        id = 4
        OR parent_id = 4
        OR path LIKE '/component/%'
        OR component LIKE 'component/%'
        OR i18n_key LIKE 'menu.component%'
      )
    )

    -- Local test page/root button rows across all platforms.
    OR path = '/test'
    OR component = 'test'
    OR code = 'test_test'
    OR i18n_key = 'menu.test'
  );

-- Remove launch-excluded grants first so menu/user init cannot expose them.
UPDATE role_permissions
SET is_del = 1,
    updated_at = CURRENT_TIMESTAMP
WHERE is_del = 2
  AND permission_id IN (SELECT id FROM tmp_permission_launch_cleanup_ids);

UPDATE permissions
SET is_del = 1,
    status = 2,
    show_menu = 2,
    updated_at = CURRENT_TIMESTAMP
WHERE is_del = 2
  AND id IN (SELECT id FROM tmp_permission_launch_cleanup_ids);

-- DIR nodes are containers; page routing lives on PAGE rows.
UPDATE permissions
SET path = '',
    component = '',
    updated_at = CURRENT_TIMESTAMP
WHERE platform = 'admin'
  AND is_del = 2
  AND type = 1
  AND (COALESCE(path, '') <> '' OR COALESCE(component, '') <> '');

-- Buttons are permission codes, not sidebar menu entries.
UPDATE permissions
SET show_menu = 2,
    updated_at = CURRENT_TIMESTAMP
WHERE is_del = 2
  AND type = 3
  AND show_menu <> 2;

-- Purge dead grant rows after the soft-delete step.
DELETE rp
FROM role_permissions rp
LEFT JOIN roles r ON r.id = rp.role_id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE rp.is_del <> 2
   OR r.id IS NULL
   OR p.id IS NULL
   OR r.is_del <> 2
   OR p.is_del <> 2;

-- Purge soft-deleted menu/permission and role records so launch data is not
-- polluted by old smoke/template/history rows.
DELETE FROM permissions
WHERE is_del <> 2;

DELETE FROM roles
WHERE is_del <> 2;

-- Old one-off backup table from early cleanup. Runtime never reads it and the
-- real backup for this migration must be an external dump, not a production table.
DROP TABLE IF EXISTS permission_backup_20260306_cleanup;

DROP TEMPORARY TABLE IF EXISTS tmp_permission_launch_cleanup_ids;
