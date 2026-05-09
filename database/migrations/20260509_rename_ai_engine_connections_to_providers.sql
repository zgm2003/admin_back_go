-- Canonical rename: AI supplier configs are providers, not engine connections.
-- Safe to run on a database that is already renamed; it becomes a no-op except permission-code cleanup.

SET @schema_name := DATABASE();

SET @rename_provider_table := (
  SELECT IF(
    SUM(table_name = 'ai_engine_connections') > 0 AND SUM(table_name = 'ai_providers') = 0,
    'RENAME TABLE `ai_engine_connections` TO `ai_providers`',
    'SELECT 1'
  )
  FROM information_schema.tables
  WHERE table_schema = @schema_name
    AND table_name IN ('ai_engine_connections', 'ai_providers')
);
PREPARE stmt FROM @rename_provider_table;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_provider_old_unique := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_providers` DROP INDEX `uk_ai_engine_connections_type_name`',
    'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_providers'
    AND index_name = 'uk_ai_engine_connections_type_name'
);
PREPARE stmt FROM @drop_provider_old_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_provider_old_status := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_providers` DROP INDEX `idx_ai_engine_connections_status`',
    'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_providers'
    AND index_name = 'idx_ai_engine_connections_status'
);
PREPARE stmt FROM @drop_provider_old_status;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_provider_unique := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_providers')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_providers' AND index_name = 'uk_ai_providers_type_name'),
    'ALTER TABLE `ai_providers` ADD UNIQUE KEY `uk_ai_providers_type_name` (`engine_type`, `name`, `is_del`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_provider_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_provider_status := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_providers')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_providers' AND index_name = 'idx_ai_providers_status'),
    'ALTER TABLE `ai_providers` ADD KEY `idx_ai_providers_status` (`status`, `is_del`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_provider_status;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_apps_old_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_apps` DROP INDEX `idx_ai_apps_connection`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_apps' AND index_name = 'idx_ai_apps_connection'
);
PREPARE stmt FROM @drop_ai_apps_old_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_apps_provider_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_apps' AND column_name = 'engine_connection_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_apps' AND column_name = 'provider_id'),
    'ALTER TABLE `ai_apps` CHANGE COLUMN `engine_connection_id` `provider_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1')
);
PREPARE stmt FROM @rename_ai_apps_provider_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_apps_provider_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_apps' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_apps' AND index_name = 'idx_ai_apps_provider'),
    'ALTER TABLE `ai_apps` ADD KEY `idx_ai_apps_provider` (`provider_id`, `status`, `is_del`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_ai_apps_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_runs_old_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_runs` DROP INDEX `idx_ai_runs_app`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND index_name = 'idx_ai_runs_app'
);
PREPARE stmt FROM @drop_ai_runs_old_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_runs_provider_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'engine_connection_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'provider_id'),
    'ALTER TABLE `ai_runs` CHANGE COLUMN `engine_connection_id` `provider_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1')
);
PREPARE stmt FROM @rename_ai_runs_provider_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_runs_provider_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND index_name = 'idx_ai_runs_app_provider'),
    'ALTER TABLE `ai_runs` ADD KEY `idx_ai_runs_app_provider` (`app_id`, `provider_id`, `is_del`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_ai_runs_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_knowledge_maps_old_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_knowledge_maps` DROP INDEX `idx_ai_knowledge_maps_engine`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_knowledge_maps' AND index_name = 'idx_ai_knowledge_maps_engine'
);
PREPARE stmt FROM @drop_ai_knowledge_maps_old_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_knowledge_maps_provider_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_knowledge_maps' AND column_name = 'engine_connection_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_knowledge_maps' AND column_name = 'provider_id'),
    'ALTER TABLE `ai_knowledge_maps` CHANGE COLUMN `engine_connection_id` `provider_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1')
);
PREPARE stmt FROM @rename_ai_knowledge_maps_provider_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_knowledge_maps_provider_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_knowledge_maps' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_knowledge_maps' AND index_name = 'idx_ai_knowledge_maps_provider'),
    'ALTER TABLE `ai_knowledge_maps` ADD KEY `idx_ai_knowledge_maps_provider` (`provider_id`, `status`, `is_del`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_ai_knowledge_maps_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_tool_maps_provider_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND column_name = 'engine_connection_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND column_name = 'provider_id'),
    'ALTER TABLE `ai_tool_maps` CHANGE COLUMN `engine_connection_id` `provider_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1')
);
PREPARE stmt FROM @rename_ai_tool_maps_provider_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_tool_maps_provider_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND index_name = 'idx_ai_tool_maps_provider'),
    'ALTER TABLE `ai_tool_maps` ADD KEY `idx_ai_tool_maps_provider` (`provider_id`, `status`, `is_del`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_ai_tool_maps_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_usage_daily_old_unique := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_usage_daily` DROP INDEX `uk_ai_usage_daily_scope`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND index_name = 'uk_ai_usage_daily_scope'
);
PREPARE stmt FROM @drop_ai_usage_daily_old_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_usage_daily_provider_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'engine_connection_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'provider_id'),
    'ALTER TABLE `ai_usage_daily` CHANGE COLUMN `engine_connection_id` `provider_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1')
);
PREPARE stmt FROM @rename_ai_usage_daily_provider_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_usage_daily_unique := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND index_name = 'uk_ai_usage_daily_scope'),
    'ALTER TABLE `ai_usage_daily` ADD UNIQUE KEY `uk_ai_usage_daily_scope` (`usage_date`, `app_id`, `provider_id`, `user_id`)',
    'SELECT 1')
);
PREPARE stmt FROM @add_ai_usage_daily_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `permissions`
SET `code` = REPLACE(`code`, 'ai_engine_', 'ai_provider_'),
    `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `code` LIKE 'ai\_engine\_%';
