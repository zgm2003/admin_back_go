-- Product decision: provider models are the current selectable model snapshot.
-- They are not audit history and do not keep remote raw JSON blobs,
-- provider-specific dumping-ground JSON, soft-delete history, or fake row auditors.

SET @schema_name := DATABASE();

SET @delete_ai_provider_models_soft_deleted := (
  SELECT IF(COUNT(*) > 0,
    'DELETE FROM `ai_provider_models` WHERE `is_del` = 1',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'is_del'
);

PREPARE stmt FROM @delete_ai_provider_models_soft_deleted;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

DELETE stale
FROM `ai_provider_models` stale
JOIN `ai_provider_models` keep_row
  ON keep_row.`provider_id` = stale.`provider_id`
 AND keep_row.`model_id` = stale.`model_id`
 AND keep_row.`id` > stale.`id`;

SET @drop_ai_provider_models_unique := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP INDEX `uk_ai_provider_models_provider_model`',
    'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND index_name = 'uk_ai_provider_models_provider_model'
);

PREPARE stmt FROM @drop_ai_provider_models_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_status_idx := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP INDEX `idx_ai_provider_models_provider_status`',
    'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND index_name = 'idx_ai_provider_models_provider_status'
);

PREPARE stmt FROM @drop_ai_provider_models_status_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_raw_json := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP COLUMN `raw_json`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'raw_json'
);

PREPARE stmt FROM @drop_ai_provider_models_raw_json;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_source := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP COLUMN `source`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'source'
);

PREPARE stmt FROM @drop_ai_provider_models_source;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_is_del := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP COLUMN `is_del`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'is_del'
);

PREPARE stmt FROM @drop_ai_provider_models_is_del;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_created_by := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP COLUMN `created_by`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'created_by'
);

PREPARE stmt FROM @drop_ai_provider_models_created_by;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_updated_by := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP COLUMN `updated_by`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'updated_by'
);

PREPARE stmt FROM @drop_ai_provider_models_updated_by;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

ALTER TABLE `ai_provider_models`
  ADD UNIQUE KEY `uk_ai_provider_models_provider_model` (`provider_id`, `model_id`),
  ADD KEY `idx_ai_provider_models_provider_status` (`provider_id`, `status`);

SET @drop_ai_providers_config_json := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_providers` DROP COLUMN `config_json`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_providers'
    AND column_name = 'config_json'
);

PREPARE stmt FROM @drop_ai_providers_config_json;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_providers_created_by := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_providers` DROP COLUMN `created_by`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_providers'
    AND column_name = 'created_by'
);

PREPARE stmt FROM @drop_ai_providers_created_by;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_providers_updated_by := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_providers` DROP COLUMN `updated_by`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_providers'
    AND column_name = 'updated_by'
);

PREPARE stmt FROM @drop_ai_providers_updated_by;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
