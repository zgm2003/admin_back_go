-- AI agent MVP config fields.
-- Provider owns selectable model snapshots; agent owns the concrete model,
-- scene, prompt and avatar used by runtime.

SET @schema_name := DATABASE();

SET @add_agent_model_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_id'),
    'ALTER TABLE `ai_agents` ADD COLUMN `model_id` VARCHAR(191) NOT NULL DEFAULT '''' AFTER `agent_type`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_model_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_agent_model_display_name := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_display_name'),
    'ALTER TABLE `ai_agents` ADD COLUMN `model_display_name` VARCHAR(191) NOT NULL DEFAULT '''' AFTER `model_id`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_model_display_name;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_agent_scenes := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'scenes_json'),
    'ALTER TABLE `ai_agents` ADD COLUMN `scenes_json` JSON NULL AFTER `model_display_name`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_scenes;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_agent_system_prompt := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'system_prompt'),
    'ALTER TABLE `ai_agents` ADD COLUMN `system_prompt` TEXT NULL AFTER `scenes_json`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_system_prompt;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_agent_avatar := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'avatar'),
    'ALTER TABLE `ai_agents` ADD COLUMN `avatar` VARCHAR(512) NOT NULL DEFAULT '''' AFTER `system_prompt`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_avatar;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `ai_agents`
SET `scenes_json` = JSON_ARRAY('chat'),
    `updated_at` = NOW()
WHERE `scenes_json` IS NULL;

SET @add_agent_model_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'idx_ai_agents_model'),
    'ALTER TABLE `ai_agents` ADD KEY `idx_ai_agents_model` (`provider_id`, `model_id`, `status`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_model_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
