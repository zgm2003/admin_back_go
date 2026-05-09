-- Product decision: AI provider config has no workspace concept.
-- Drop the discarded column from databases that already ran the old AI core rebuild.

SET @schema_name := DATABASE();
SET @drop_ai_provider_workspace_id := (
  SELECT IF(COUNT(*) > 0,
    'ALTER TABLE `ai_providers` DROP COLUMN `workspace_id`',
    'SELECT 1')
  FROM information_schema.columns
  WHERE table_schema = @schema_name
    AND table_name = 'ai_providers'
    AND column_name = 'workspace_id'
);

PREPARE stmt FROM @drop_ai_provider_workspace_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
