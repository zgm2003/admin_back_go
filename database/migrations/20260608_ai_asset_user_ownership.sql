-- Canvas assets are user-owned. Admin no longer manages a shared asset library.

SET @schema_name := DATABASE();

SET @ai_assets_has_user_id := (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_assets'
    AND column_name = 'user_id'
);

SET @ai_assets_add_user_id_sql := IF(
  @ai_assets_has_user_id = 0,
  'ALTER TABLE `ai_assets` ADD COLUMN `user_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `id`',
  'SELECT 1'
);
PREPARE ai_assets_add_user_id_stmt FROM @ai_assets_add_user_id_sql;
EXECUTE ai_assets_add_user_id_stmt;
DEALLOCATE PREPARE ai_assets_add_user_id_stmt;

-- Existing ownerless rows cannot be assigned to a real user without inventing ownership.
-- Keep them recoverable but inactive; new Canvas writes always carry the authenticated user_id.
UPDATE `ai_assets`
SET `is_del` = 1, `updated_at` = CURRENT_TIMESTAMP
WHERE `user_id` = 0
  AND `is_del` = 2;

SET @ai_assets_slug_unique_exists := (
  SELECT COUNT(*)
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_assets'
    AND index_name = 'uk_ai_assets_slug'
);

SET @ai_assets_drop_slug_unique_sql := IF(
  @ai_assets_slug_unique_exists > 0,
  'DROP INDEX `uk_ai_assets_slug` ON `ai_assets`',
  'SELECT 1'
);
PREPARE ai_assets_drop_slug_unique_stmt FROM @ai_assets_drop_slug_unique_sql;
EXECUTE ai_assets_drop_slug_unique_stmt;
DEALLOCATE PREPARE ai_assets_drop_slug_unique_stmt;

SET @ai_assets_user_slug_unique_exists := (
  SELECT COUNT(*)
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_assets'
    AND index_name = 'uk_ai_assets_user_slug'
);

SET @ai_assets_create_user_slug_unique_sql := IF(
  @ai_assets_user_slug_unique_exists = 0,
  'CREATE UNIQUE INDEX `uk_ai_assets_user_slug` ON `ai_assets` (`user_id`, `slug`)',
  'SELECT 1'
);
PREPARE ai_assets_create_user_slug_unique_stmt FROM @ai_assets_create_user_slug_unique_sql;
EXECUTE ai_assets_create_user_slug_unique_stmt;
DEALLOCATE PREPARE ai_assets_create_user_slug_unique_stmt;

SET @ai_assets_user_status_updated_exists := (
  SELECT COUNT(*)
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_assets'
    AND index_name = 'idx_ai_assets_user_status_updated'
);

SET @ai_assets_create_user_status_updated_sql := IF(
  @ai_assets_user_status_updated_exists = 0,
  'CREATE INDEX `idx_ai_assets_user_status_updated` ON `ai_assets` (`user_id`, `status`, `is_del`, `updated_at`, `id`)',
  'SELECT 1'
);
PREPARE ai_assets_create_user_status_updated_stmt FROM @ai_assets_create_user_status_updated_sql;
EXECUTE ai_assets_create_user_status_updated_stmt;
DEALLOCATE PREPARE ai_assets_create_user_status_updated_stmt;
