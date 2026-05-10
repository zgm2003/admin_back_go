-- Remove duplicate AI tool executor column.
-- `ai_tools.code` is the single tool identity and the server dispatch key.

SET @schema_name := DATABASE();

SET @drop_ai_tools_executor := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tools' AND column_name = 'executor'),
    'ALTER TABLE `ai_tools` DROP COLUMN `executor`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_tools_executor;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
