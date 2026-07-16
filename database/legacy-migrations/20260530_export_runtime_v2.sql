SET @export_tasks_has_kind := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'export_tasks'
    AND COLUMN_NAME = 'kind'
);

SET @export_tasks_add_kind_sql := IF(
  @export_tasks_has_kind = 0,
  'ALTER TABLE `export_tasks` ADD COLUMN `kind` varchar(64) NOT NULL DEFAULT ''user_list'' COMMENT ''导出类型'' AFTER `title`',
  'SELECT 1'
);
PREPARE export_tasks_add_kind_stmt FROM @export_tasks_add_kind_sql;
EXECUTE export_tasks_add_kind_stmt;
DEALLOCATE PREPARE export_tasks_add_kind_stmt;

SET @export_tasks_has_platform := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'export_tasks'
    AND COLUMN_NAME = 'platform'
);

SET @export_tasks_add_platform_sql := IF(
  @export_tasks_has_platform = 0,
  'ALTER TABLE `export_tasks` ADD COLUMN `platform` varchar(32) NOT NULL DEFAULT ''admin'' COMMENT ''平台入口'' AFTER `user_id`',
  'SELECT 1'
);
PREPARE export_tasks_add_platform_stmt FROM @export_tasks_add_platform_sql;
EXECUTE export_tasks_add_platform_stmt;
DEALLOCATE PREPARE export_tasks_add_platform_stmt;

SET @export_tasks_has_object_key := (
  SELECT COUNT(1)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'export_tasks'
    AND COLUMN_NAME = 'object_key'
);

SET @export_tasks_add_object_key_sql := IF(
  @export_tasks_has_object_key = 0,
  'ALTER TABLE `export_tasks` ADD COLUMN `object_key` varchar(500) NULL COMMENT ''COS object key'' AFTER `file_url`',
  'SELECT 1'
);
PREPARE export_tasks_add_object_key_stmt FROM @export_tasks_add_object_key_sql;
EXECUTE export_tasks_add_object_key_stmt;
DEALLOCATE PREPARE export_tasks_add_object_key_stmt;

UPDATE `export_tasks`
SET `kind` = 'user_list'
WHERE `kind` = '' OR `kind` IS NULL;

UPDATE `export_tasks`
SET `platform` = 'admin'
WHERE `platform` = '' OR `platform` IS NULL;

SET @export_tasks_has_user_platform_status_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'export_tasks'
    AND INDEX_NAME = 'idx_export_tasks_user_platform_status'
);

SET @export_tasks_add_user_platform_status_idx_sql := IF(
  @export_tasks_has_user_platform_status_idx = 0,
  'CREATE INDEX `idx_export_tasks_user_platform_status` ON `export_tasks` (`user_id`, `platform`, `status`, `is_del`)',
  'SELECT 1'
);
PREPARE export_tasks_add_user_platform_status_idx_stmt FROM @export_tasks_add_user_platform_status_idx_sql;
EXECUTE export_tasks_add_user_platform_status_idx_stmt;
DEALLOCATE PREPARE export_tasks_add_user_platform_status_idx_stmt;

SET @export_tasks_has_user_platform_kind_idx := (
  SELECT COUNT(1)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'export_tasks'
    AND INDEX_NAME = 'idx_export_tasks_user_platform_kind'
);

SET @export_tasks_add_user_platform_kind_idx_sql := IF(
  @export_tasks_has_user_platform_kind_idx = 0,
  'CREATE INDEX `idx_export_tasks_user_platform_kind` ON `export_tasks` (`user_id`, `platform`, `kind`, `is_del`)',
  'SELECT 1'
);
PREPARE export_tasks_add_user_platform_kind_idx_stmt FROM @export_tasks_add_user_platform_kind_idx_sql;
EXECUTE export_tasks_add_user_platform_kind_idx_stmt;
DEALLOCATE PREPARE export_tasks_add_user_platform_kind_idx_stmt;
