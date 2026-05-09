-- Product decision: provider config only stores available model catalog.
-- The concrete model is selected by future agent/app config, not by provider.

SET @schema_name := DATABASE();

SET @drop_ai_provider_models_default_idx := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP INDEX `idx_ai_provider_models_provider_default`',
    'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND index_name = 'idx_ai_provider_models_provider_default'
);

PREPARE stmt FROM @drop_ai_provider_models_default_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_provider_models_is_default := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_provider_models` DROP COLUMN `is_default`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_provider_models'
    AND column_name = 'is_default'
);

PREPARE stmt FROM @drop_ai_provider_models_is_default;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
