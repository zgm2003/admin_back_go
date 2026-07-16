-- Prune ai_agents back to the actual MVP contract.
-- Agent config owns only local selection/display metadata here. Runtime/tool/RAG
-- extras must be introduced by explicit later contracts, not by placeholder
-- columns.

SET @schema_name := DATABASE();

SET @drop_ai_agents_external_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'external_agent_id'),
    'ALTER TABLE `ai_agents` DROP COLUMN `external_agent_id`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_external_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_external_key := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'external_agent_api_key_enc'),
    'ALTER TABLE `ai_agents` DROP COLUMN `external_agent_api_key_enc`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_external_key;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_external_hint := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'external_agent_api_key_hint'),
    'ALTER TABLE `ai_agents` DROP COLUMN `external_agent_api_key_hint`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_external_hint;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_response_mode := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'default_response_mode'),
    'ALTER TABLE `ai_agents` DROP COLUMN `default_response_mode`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_response_mode;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_runtime_config := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'runtime_config_json'),
    'ALTER TABLE `ai_agents` DROP COLUMN `runtime_config_json`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_runtime_config;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_model_snapshot := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_snapshot_json'),
    'ALTER TABLE `ai_agents` DROP COLUMN `model_snapshot_json`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_model_snapshot;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_created_by := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'created_by'),
    'ALTER TABLE `ai_agents` DROP COLUMN `created_by`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_created_by;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_updated_by := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'updated_by'),
    'ALTER TABLE `ai_agents` DROP COLUMN `updated_by`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @drop_ai_agents_updated_by;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
