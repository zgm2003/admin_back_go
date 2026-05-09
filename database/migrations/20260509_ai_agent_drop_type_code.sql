-- Drop fake agent code/type concepts from the AI agent MVP.
-- Current MVP identifies agents by id/name and filters by scenes_json.

SET @schema_name := DATABASE();

SET @drop_ai_agents_code_index := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'uk_ai_agents_code'),
    'ALTER TABLE `ai_agents` DROP INDEX `uk_ai_agents_code`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_code_index;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_code := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'code'),
    'ALTER TABLE `ai_agents` DROP COLUMN `code`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_code;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_agent_type := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'agent_type'),
    'ALTER TABLE `ai_agents` DROP COLUMN `agent_type`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_agent_type;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
