-- Canonical rename: AI configurable units are agents, not apps.
-- This migration is intentionally idempotent because some developer DBs already
-- ran an earlier 20260508 AI rebuild that used ai_apps/app_id naming.

SET @schema_name := DATABASE();

SET @rename_ai_apps_table := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_apps')
    AND NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents'),
    'RENAME TABLE `ai_apps` TO `ai_agents`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_apps_table;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_app_bindings_table := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_app_bindings')
    AND NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings'),
    'RENAME TABLE `ai_app_bindings` TO `ai_agent_bindings`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_app_bindings_table;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_old_unique := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_agents` DROP INDEX `uk_ai_apps_code`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'uk_ai_apps_code'
);
PREPARE stmt FROM @drop_ai_agents_old_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agents_old_provider_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_agents` DROP INDEX `idx_ai_apps_provider`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'idx_ai_apps_provider'
);
PREPARE stmt FROM @drop_ai_agents_old_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_agents_agent_type := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'app_type')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'agent_type'),
    'ALTER TABLE `ai_agents` CHANGE COLUMN `app_type` `agent_type` VARCHAR(32) NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_agents_agent_type;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_agents_external_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'engine_app_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'external_agent_id'),
    'ALTER TABLE `ai_agents` CHANGE COLUMN `engine_app_id` `external_agent_id` VARCHAR(128) NOT NULL DEFAULT ''''',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_agents_external_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_agents_external_key := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'engine_app_api_key_enc')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'external_agent_api_key_enc'),
    'ALTER TABLE `ai_agents` CHANGE COLUMN `engine_app_api_key_enc` `external_agent_api_key_enc` TEXT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_agents_external_key;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_agents_external_hint := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'engine_app_api_key_hint')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'external_agent_api_key_hint'),
    'ALTER TABLE `ai_agents` CHANGE COLUMN `engine_app_api_key_hint` `external_agent_api_key_hint` VARCHAR(32) NOT NULL DEFAULT ''''',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_agents_external_hint;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @comment_ai_agents := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents'),
    'ALTER TABLE `ai_agents` COMMENT = ''AI agent mappings''',
    'SELECT 1'
  )
);
PREPARE stmt FROM @comment_ai_agents;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_agents_unique := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agents')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'uk_ai_agents_code'),
    'ALTER TABLE `ai_agents` ADD UNIQUE KEY `uk_ai_agents_code` (`code`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_agents_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_agents_provider_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'idx_ai_agents_provider'),
    'ALTER TABLE `ai_agents` ADD KEY `idx_ai_agents_provider` (`provider_id`, `status`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_agents_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- MVP columns may have been skipped if 20260509_ai_agent_mvp_config.sql ran
-- before this rename on a DB that still had ai_apps.
SET @add_agent_model_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'agent_type')
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
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_id')
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
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_display_name')
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
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'scenes_json')
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
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'system_prompt')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'avatar'),
    'ALTER TABLE `ai_agents` ADD COLUMN `avatar` VARCHAR(512) NOT NULL DEFAULT '''' AFTER `system_prompt`',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_avatar;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @set_agent_scenes := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'scenes_json'),
    'UPDATE `ai_agents` SET `scenes_json` = JSON_ARRAY(''chat''), `updated_at` = NOW() WHERE `scenes_json` IS NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @set_agent_scenes;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_agent_model_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND column_name = 'model_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agents' AND index_name = 'idx_ai_agents_model'),
    'ALTER TABLE `ai_agents` ADD KEY `idx_ai_agents_model` (`provider_id`, `model_id`, `status`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_agent_model_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agent_bindings_old_unique := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_agent_bindings` DROP INDEX `uk_ai_app_bindings_key`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND index_name = 'uk_ai_app_bindings_key'
);
PREPARE stmt FROM @drop_ai_agent_bindings_old_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_agent_bindings_old_scope := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_agent_bindings` DROP INDEX `idx_ai_app_bindings_scope`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND index_name = 'idx_ai_app_bindings_scope'
);
PREPARE stmt FROM @drop_ai_agent_bindings_old_scope;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_agent_bindings_agent_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND column_name = 'app_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND column_name = 'agent_id'),
    'ALTER TABLE `ai_agent_bindings` CHANGE COLUMN `app_id` `agent_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_agent_bindings_agent_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @comment_ai_agent_bindings := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings'),
    'ALTER TABLE `ai_agent_bindings` COMMENT = ''AI agent visibility bindings''',
    'SELECT 1'
  )
);
PREPARE stmt FROM @comment_ai_agent_bindings;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_agent_bindings_unique := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND column_name = 'agent_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND index_name = 'uk_ai_agent_bindings_key'),
    'ALTER TABLE `ai_agent_bindings` ADD UNIQUE KEY `uk_ai_agent_bindings_key` (`agent_id`, `bind_type`, `bind_key`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_agent_bindings_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_agent_bindings_scope := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_agent_bindings' AND index_name = 'idx_ai_agent_bindings_scope'),
    'ALTER TABLE `ai_agent_bindings` ADD KEY `idx_ai_agent_bindings_scope` (`bind_type`, `bind_key`, `status`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_agent_bindings_scope;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_conversations_old_user_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_conversations` DROP INDEX `idx_ai_conversations_user`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_conversations'
    AND index_name = 'idx_ai_conversations_user'
    AND column_name = 'app_id'
);
PREPARE stmt FROM @drop_ai_conversations_old_user_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_conversations_agent_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_conversations' AND column_name = 'app_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_conversations' AND column_name = 'agent_id'),
    'ALTER TABLE `ai_conversations` CHANGE COLUMN `app_id` `agent_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_conversations_agent_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_conversations_user_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_conversations' AND column_name = 'agent_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_conversations' AND index_name = 'idx_ai_conversations_user'),
    'ALTER TABLE `ai_conversations` ADD KEY `idx_ai_conversations_user` (`user_id`, `agent_id`, `status`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_conversations_user_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_runs_old_app_provider_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_runs` DROP INDEX `idx_ai_runs_app_provider`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND index_name = 'idx_ai_runs_app_provider'
);
PREPARE stmt FROM @drop_ai_runs_old_app_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_runs_agent_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'app_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'agent_id'),
    'ALTER TABLE `ai_runs` CHANGE COLUMN `app_id` `agent_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_runs_agent_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_runs_agent_provider_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'agent_id')
    AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_runs' AND index_name = 'idx_ai_runs_agent_provider'),
    'ALTER TABLE `ai_runs` ADD KEY `idx_ai_runs_agent_provider` (`agent_id`, `provider_id`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_runs_agent_provider_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_tool_maps_old_app_idx := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_tool_maps` DROP INDEX `idx_ai_tool_maps_app`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND index_name = 'idx_ai_tool_maps_app'
);
PREPARE stmt FROM @drop_ai_tool_maps_old_app_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_tool_maps_agent_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND column_name = 'app_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND column_name = 'agent_id'),
    'ALTER TABLE `ai_tool_maps` CHANGE COLUMN `app_id` `agent_id` BIGINT UNSIGNED NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_tool_maps_agent_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_tool_maps_agent_idx := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND column_name = 'agent_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_tool_maps' AND index_name = 'idx_ai_tool_maps_agent'),
    'ALTER TABLE `ai_tool_maps` ADD KEY `idx_ai_tool_maps_agent` (`agent_id`, `status`, `is_del`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_tool_maps_agent_idx;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @drop_ai_usage_daily_old_unique := (
  SELECT IF(COUNT(*) > 0, 'ALTER TABLE `ai_usage_daily` DROP INDEX `uk_ai_usage_daily_scope`', 'SELECT 1')
  FROM information_schema.statistics
  WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily'
    AND index_name = 'uk_ai_usage_daily_scope'
    AND column_name = 'app_id'
);
PREPARE stmt FROM @drop_ai_usage_daily_old_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @rename_ai_usage_daily_agent_id := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'app_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'agent_id'),
    'ALTER TABLE `ai_usage_daily` CHANGE COLUMN `app_id` `agent_id` BIGINT UNSIGNED NOT NULL',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_ai_usage_daily_agent_id;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @add_ai_usage_daily_unique := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'agent_id')
    AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND column_name = 'provider_id')
    AND NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema = @schema_name AND table_name = 'ai_usage_daily' AND index_name = 'uk_ai_usage_daily_scope'),
    'ALTER TABLE `ai_usage_daily` ADD UNIQUE KEY `uk_ai_usage_daily_scope` (`usage_date`, `agent_id`, `provider_id`, `user_id`)',
    'SELECT 1'
  )
);
PREPARE stmt FROM @add_ai_usage_daily_unique;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `permissions`
SET `path` = '/ai/agents',
    `component` = 'ai/agents',
    `i18n_key` = 'menu.ai_agents',
    `name` = '智能体配置',
    `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `is_del` = 2
  AND (
    `path` = '/ai/apps'
    OR `component` = 'ai/apps'
    OR `i18n_key` = 'menu.ai_apps'
  );

UPDATE permissions oldp
JOIN permissions target ON target.platform = oldp.platform
  AND target.is_del = 2
  AND target.code = CASE oldp.code
    WHEN 'ai_app_add' THEN 'ai_agent_add'
    WHEN 'ai_app_edit' THEN 'ai_agent_edit'
    WHEN 'ai_app_test' THEN 'ai_agent_test'
    WHEN 'ai_app_status' THEN 'ai_agent_status'
    WHEN 'ai_app_del' THEN 'ai_agent_del'
    WHEN 'ai_app_binding' THEN 'ai_agent_binding_add'
    ELSE REPLACE(oldp.code, 'ai_app_', 'ai_agent_')
  END
  AND target.id <> oldp.id
SET oldp.code = CONCAT('retired_', oldp.code, '_', oldp.id),
    oldp.status = 2,
    oldp.is_del = 1,
    oldp.updated_at = NOW()
WHERE oldp.platform = 'admin'
  AND oldp.is_del = 2
  AND oldp.code LIKE 'ai\_app\_%';

UPDATE `permissions`
SET `code` = CASE `code`
    WHEN 'ai_app_add' THEN 'ai_agent_add'
    WHEN 'ai_app_edit' THEN 'ai_agent_edit'
    WHEN 'ai_app_test' THEN 'ai_agent_test'
    WHEN 'ai_app_status' THEN 'ai_agent_status'
    WHEN 'ai_app_del' THEN 'ai_agent_del'
    WHEN 'ai_app_binding' THEN 'ai_agent_binding_add'
    ELSE REPLACE(`code`, 'ai_app_', 'ai_agent_')
  END,
  `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `is_del` = 2
  AND `code` LIKE 'ai\_app\_%';

SET @rename_generic_agent_binding := (
  SELECT IF(
    EXISTS(SELECT 1 FROM permissions WHERE platform = 'admin' AND is_del = 2 AND code = 'ai_agent_binding')
    AND NOT EXISTS(SELECT 1 FROM permissions WHERE platform = 'admin' AND is_del = 2 AND code = 'ai_agent_binding_add'),
    'UPDATE `permissions` SET `name` = ''新增智能体绑定'', `code` = ''ai_agent_binding_add'', `sort` = 6, `updated_at` = NOW() WHERE `platform` = ''admin'' AND `is_del` = 2 AND `code` = ''ai_agent_binding''',
    'SELECT 1'
  )
);
PREPARE stmt FROM @rename_generic_agent_binding;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @retire_generic_agent_binding := (
  SELECT IF(
    EXISTS(SELECT 1 FROM permissions WHERE platform = 'admin' AND is_del = 2 AND code = 'ai_agent_binding')
    AND EXISTS(SELECT 1 FROM permissions WHERE platform = 'admin' AND is_del = 2 AND code = 'ai_agent_binding_add'),
    'UPDATE `permissions` SET `code` = CONCAT(''retired_ai_agent_binding_'', `id`), `status` = 2, `is_del` = 1, `updated_at` = NOW() WHERE `platform` = ''admin'' AND `is_del` = 2 AND `code` = ''ai_agent_binding''',
    'SELECT 1'
  )
);
PREPARE stmt FROM @retire_generic_agent_binding;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @ai_parent_id := (
  SELECT id FROM permissions
  WHERE platform = 'admin' AND is_del = 2 AND i18n_key = 'menu.ai'
  ORDER BY id ASC LIMIT 1
);

INSERT INTO permissions (name, path, icon, parent_id, component, platform, type, sort, code, i18n_key, show_menu, status, is_del, created_at, updated_at)
SELECT 'AI助手', '', 'Cpu', 0, NULL, 'admin', 1, 5, NULL, 'menu.ai', 1, 1, 2, NOW(), NOW()
WHERE @ai_parent_id IS NULL;

SET @ai_parent_id := (
  SELECT id FROM permissions
  WHERE platform = 'admin' AND is_del = 2 AND i18n_key = 'menu.ai'
  ORDER BY id ASC LIMIT 1
);

SET @ai_agents_page_id := (
  SELECT id FROM permissions
  WHERE platform = 'admin' AND is_del = 2 AND i18n_key = 'menu.ai_agents'
  ORDER BY id ASC LIMIT 1
);

INSERT INTO permissions (name, path, icon, parent_id, component, platform, type, sort, code, i18n_key, show_menu, status, is_del, created_at, updated_at)
SELECT '智能体配置', '/ai/agents', '', @ai_parent_id, 'ai/agents', 'admin', 2, 2, NULL, 'menu.ai_agents', 1, 1, 2, NOW(), NOW()
WHERE @ai_agents_page_id IS NULL;

SET @ai_agents_page_id := (
  SELECT id FROM permissions
  WHERE platform = 'admin' AND is_del = 2 AND i18n_key = 'menu.ai_agents'
  ORDER BY id ASC LIMIT 1
);

UPDATE permissions
SET name = '智能体配置',
    path = '/ai/agents',
    component = 'ai/agents',
    parent_id = @ai_parent_id,
    sort = 2,
    show_menu = 1,
    status = 1,
    is_del = 2,
    updated_at = NOW()
WHERE id = @ai_agents_page_id;

DROP TEMPORARY TABLE IF EXISTS tmp_ai_agent_buttons;
CREATE TEMPORARY TABLE tmp_ai_agent_buttons (
  name VARCHAR(64) NOT NULL,
  code VARCHAR(128) NOT NULL,
  sort INT NOT NULL,
  PRIMARY KEY (code)
) ENGINE=MEMORY;

INSERT INTO tmp_ai_agent_buttons (name, code, sort) VALUES
('新增智能体', 'ai_agent_add', 1),
('编辑智能体', 'ai_agent_edit', 2),
('测试智能体', 'ai_agent_test', 3),
('智能体状态', 'ai_agent_status', 4),
('删除智能体', 'ai_agent_del', 5),
('新增智能体绑定', 'ai_agent_binding_add', 6),
('删除智能体绑定', 'ai_agent_binding_del', 7);

INSERT INTO permissions (name, path, icon, parent_id, component, platform, type, sort, code, i18n_key, show_menu, status, is_del, created_at, updated_at)
SELECT b.name, '', '', @ai_agents_page_id, NULL, 'admin', 3, b.sort, b.code, '', 2, 1, 2, NOW(), NOW()
FROM tmp_ai_agent_buttons b
WHERE @ai_agents_page_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  name = VALUES(name),
  parent_id = VALUES(parent_id),
  sort = VALUES(sort),
  show_menu = VALUES(show_menu),
  status = VALUES(status),
  is_del = VALUES(is_del),
  updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, is_del, created_at, updated_at)
SELECT DISTINCT page_grant.role_id, button.id, 2, NOW(), NOW()
FROM role_permissions page_grant
JOIN permissions button ON button.platform = 'admin'
  AND button.is_del = 2
  AND button.code IN (
    'ai_agent_add',
    'ai_agent_edit',
    'ai_agent_test',
    'ai_agent_status',
    'ai_agent_del',
    'ai_agent_binding_add',
    'ai_agent_binding_del'
  )
WHERE page_grant.permission_id = @ai_agents_page_id
  AND page_grant.is_del = 2
ON DUPLICATE KEY UPDATE is_del = VALUES(is_del), updated_at = NOW();

DROP TEMPORARY TABLE IF EXISTS tmp_ai_agent_buttons;
