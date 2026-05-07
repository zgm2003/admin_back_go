-- Canonicalize client version storage and RBAC codes.
--
-- The project is not online yet, so we do not keep the historical Tauri table
-- name or devTools button codes. Go code now maps the model to
-- `client_versions`, and mutating route metadata now uses
-- `system_clientVersion_*` permission codes.

SET @old_table_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'tauri_version'
);

SET @new_table_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'client_versions'
);

SET @rename_client_versions_sql := IF(
    @old_table_exists = 1 AND @new_table_exists = 0,
    'RENAME TABLE `tauri_version` TO `client_versions`',
    'SELECT 1'
);

PREPARE rename_client_versions_stmt FROM @rename_client_versions_sql;
EXECUTE rename_client_versions_stmt;
DEALLOCATE PREPARE rename_client_versions_stmt;

SET @client_versions_exists := (
    SELECT COUNT(*)
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'client_versions'
);

SET @comment_client_versions_sql := IF(
    @client_versions_exists = 1,
    "ALTER TABLE `client_versions` COMMENT = '客户端版本管理'",
    'SELECT 1'
);

PREPARE comment_client_versions_stmt FROM @comment_client_versions_sql;
EXECUTE comment_client_versions_stmt;
DEALLOCATE PREPARE comment_client_versions_stmt;

UPDATE `permissions`
SET `code` = CASE `code`
    WHEN 'devTools_tauriVersion_add' THEN 'system_clientVersion_add'
    WHEN 'devTools_tauriVersion_edit' THEN 'system_clientVersion_edit'
    WHEN 'devTools_tauriVersion_setLatest' THEN 'system_clientVersion_setLatest'
    WHEN 'devTools_tauriVersion_forceUpdate' THEN 'system_clientVersion_forceUpdate'
    WHEN 'devTools_tauriVersion_del' THEN 'system_clientVersion_del'
    ELSE `code`
  END,
  `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `type` = 3
  AND `is_del` = 2
  AND `code` IN (
      'devTools_tauriVersion_add',
      'devTools_tauriVersion_edit',
      'devTools_tauriVersion_setLatest',
      'devTools_tauriVersion_forceUpdate',
      'devTools_tauriVersion_del'
  );
