-- Rebuild the AI core around admin_go-owned Dify sidecar mirror/control tables.
-- Required order:
--   1. Run 20260508_ai_core_backup.sql.
--   2. Run this file.
-- Cache invalidation is intentionally left to deployment operation.

DROP TEMPORARY TABLE IF EXISTS tmp_ai_core_page_grants;
CREATE TEMPORARY TABLE tmp_ai_core_page_grants (
  role_id INT UNSIGNED NOT NULL,
  page_key VARCHAR(32) NOT NULL,
  PRIMARY KEY (role_id, page_key)
) ENGINE=MEMORY;

INSERT IGNORE INTO tmp_ai_core_page_grants (role_id, page_key)
SELECT DISTINCT rp.role_id,
  CASE
    WHEN p.path IN ('/ai/models', '/ai/providers') OR p.i18n_key IN ('menu.ai_models', 'menu.ai_providers') THEN 'providers'
    WHEN p.path IN ('/ai/agents', '/ai/apps') OR p.i18n_key IN ('menu.ai_agents', 'menu.ai_apps') THEN 'apps'
    WHEN p.path = '/ai/knowledge' OR p.i18n_key = 'menu.ai_knowledge' THEN 'knowledge'
    WHEN p.path = '/ai/tools' OR p.i18n_key = 'menu.ai_tools' THEN 'tools'
    WHEN p.path = '/ai/chat' OR p.i18n_key = 'menu.ai_chat' THEN 'chat'
    WHEN p.path = '/ai/runs' OR p.i18n_key = 'menu.ai_runs' THEN 'runs'
    ELSE ''
  END AS page_key
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE rp.is_del = 2
  AND p.platform = 'admin'
  AND p.is_del = 2
  AND (
    p.path IN ('/ai/models', '/ai/providers', '/ai/agents', '/ai/apps', '/ai/knowledge', '/ai/tools', '/ai/chat', '/ai/runs')
    OR p.i18n_key IN ('menu.ai_models', 'menu.ai_providers', 'menu.ai_agents', 'menu.ai_apps', 'menu.ai_knowledge', 'menu.ai_tools', 'menu.ai_chat', 'menu.ai_runs')
  );

DELETE FROM tmp_ai_core_page_grants WHERE page_key = '';

DROP TABLE IF EXISTS ai_models;
DROP TABLE IF EXISTS ai_tools;
DROP TABLE IF EXISTS ai_prompts;
DROP TABLE IF EXISTS ai_prompt;
DROP TABLE IF EXISTS ai_agents;
DROP TABLE IF EXISTS ai_agent_scenes;
DROP TABLE IF EXISTS ai_assistant_tools;
DROP TABLE IF EXISTS ai_agent_knowledge_bases;
DROP TABLE IF EXISTS ai_knowledge_bases;
DROP TABLE IF EXISTS ai_knowledge_documents;
DROP TABLE IF EXISTS ai_knowledge_chunks;
DROP TABLE IF EXISTS ai_conversations;
DROP TABLE IF EXISTS ai_messages;
DROP TABLE IF EXISTS ai_runs;
DROP TABLE IF EXISTS ai_run_steps;

CREATE TABLE ai_engine_connections (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  engine_type VARCHAR(32) NOT NULL,
  base_url VARCHAR(512) NOT NULL,
  api_key_enc TEXT NULL,
  api_key_hint VARCHAR(32) NOT NULL DEFAULT '',
  config_json JSON NULL,
  health_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
  last_checked_at DATETIME NULL,
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  created_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_engine_connections_type_name (engine_type, name, is_del),
  KEY idx_ai_engine_connections_status (status, is_del)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI engine connection configs';

CREATE TABLE ai_apps (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  engine_connection_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(128) NOT NULL,
  code VARCHAR(128) NOT NULL,
  app_type VARCHAR(32) NOT NULL,
  engine_app_id VARCHAR(128) NOT NULL DEFAULT '',
  engine_app_api_key_enc TEXT NULL,
  engine_app_api_key_hint VARCHAR(32) NOT NULL DEFAULT '',
  default_response_mode VARCHAR(32) NOT NULL DEFAULT 'streaming',
  runtime_config_json JSON NULL,
  model_snapshot_json JSON NULL,
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  created_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_apps_code (code, is_del),
  KEY idx_ai_apps_connection (engine_connection_id, status, is_del)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI app mappings';

CREATE TABLE ai_app_bindings (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  app_id BIGINT UNSIGNED NOT NULL,
  bind_type VARCHAR(32) NOT NULL,
  bind_key VARCHAR(128) NOT NULL,
  sort INT NOT NULL DEFAULT 0,
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_app_bindings_key (app_id, bind_type, bind_key, is_del),
  KEY idx_ai_app_bindings_scope (bind_type, bind_key, status, is_del)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI app visibility bindings';

CREATE TABLE ai_conversations (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  app_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(255) NOT NULL DEFAULT '',
  engine_conversation_id VARCHAR(128) NOT NULL DEFAULT '',
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  last_message_at DATETIME NULL,
  meta_json JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ai_conversations_user (user_id, app_id, status, is_del),
  KEY idx_ai_conversations_engine (engine_conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI conversation mirror';

CREATE TABLE ai_messages (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  conversation_id BIGINT UNSIGNED NOT NULL,
  run_id BIGINT UNSIGNED NULL,
  user_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  role TINYINT UNSIGNED NOT NULL,
  content_type VARCHAR(32) NOT NULL DEFAULT 'text',
  content LONGTEXT NOT NULL,
  engine_message_id VARCHAR(128) NOT NULL DEFAULT '',
  token_input INT NOT NULL DEFAULT 0,
  token_output INT NOT NULL DEFAULT 0,
  feedback TINYINT NULL,
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  meta_json JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ai_messages_conversation (conversation_id, id, is_del),
  KEY idx_ai_messages_run (run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI message mirror';

CREATE TABLE ai_runs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_uid VARCHAR(64) NOT NULL,
  app_id BIGINT UNSIGNED NOT NULL,
  conversation_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  user_message_id BIGINT UNSIGNED NULL,
  assistant_message_id BIGINT UNSIGNED NULL,
  engine_connection_id BIGINT UNSIGNED NOT NULL,
  engine_task_id VARCHAR(128) NOT NULL DEFAULT '',
  engine_run_id VARCHAR(128) NOT NULL DEFAULT '',
  request_id VARCHAR(128) NOT NULL DEFAULT '',
  run_status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  input_snapshot_json JSON NULL,
  output_snapshot_json JSON NULL,
  usage_json JSON NULL,
  prompt_tokens INT NOT NULL DEFAULT 0,
  completion_tokens INT NOT NULL DEFAULT 0,
  total_tokens INT NOT NULL DEFAULT 0,
  cost DECIMAL(18,6) NOT NULL DEFAULT 0,
  latency_ms INT NOT NULL DEFAULT 0,
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  error_msg VARCHAR(1024) NOT NULL DEFAULT '',
  model_snapshot VARCHAR(255) NOT NULL DEFAULT '',
  meta_json JSON NULL,
  started_at DATETIME NULL,
  completed_at DATETIME NULL,
  canceled_at DATETIME NULL,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_runs_uid (run_uid),
  KEY idx_ai_runs_user (user_id, run_status, created_at),
  KEY idx_ai_runs_engine_task (engine_task_id),
  KEY idx_ai_runs_app (app_id, engine_connection_id, is_del)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI run mirror';

CREATE TABLE ai_run_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  run_id BIGINT UNSIGNED NOT NULL,
  seq BIGINT UNSIGNED NOT NULL,
  event_id VARCHAR(64) NOT NULL DEFAULT '',
  event_type VARCHAR(64) NOT NULL,
  delta_text TEXT NULL,
  payload_json JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_run_events_seq (run_id, seq),
  KEY idx_ai_run_events_type (event_type, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI durable event stream';

CREATE TABLE ai_knowledge_maps (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  engine_connection_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(128) NOT NULL,
  code VARCHAR(128) NOT NULL,
  engine_dataset_id VARCHAR(128) NOT NULL DEFAULT '',
  visibility VARCHAR(32) NOT NULL DEFAULT 'private',
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  meta_json JSON NULL,
  created_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_knowledge_maps_code (code, is_del),
  KEY idx_ai_knowledge_maps_engine (engine_connection_id, status, is_del)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI knowledge dataset maps';

CREATE TABLE ai_knowledge_documents (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  knowledge_map_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(255) NOT NULL,
  engine_document_id VARCHAR(128) NOT NULL DEFAULT '',
  engine_batch VARCHAR(128) NOT NULL DEFAULT '',
  source_type VARCHAR(32) NOT NULL,
  source_ref VARCHAR(512) NOT NULL DEFAULT '',
  content LONGTEXT NULL,
  indexing_status VARCHAR(32) NOT NULL DEFAULT 'pending',
  error_message VARCHAR(1024) NOT NULL DEFAULT '',
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  meta_json JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_ai_knowledge_documents_map (knowledge_map_id, status, is_del),
  KEY idx_ai_knowledge_documents_engine (engine_document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI knowledge document maps';

CREATE TABLE ai_tool_maps (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  engine_connection_id BIGINT UNSIGNED NOT NULL,
  app_id BIGINT UNSIGNED NULL,
  name VARCHAR(128) NOT NULL,
  code VARCHAR(128) NOT NULL,
  tool_type VARCHAR(32) NOT NULL,
  engine_tool_id VARCHAR(128) NOT NULL DEFAULT '',
  permission_code VARCHAR(128) NOT NULL DEFAULT '',
  risk_level VARCHAR(32) NOT NULL DEFAULT 'low',
  config_json JSON NULL,
  status TINYINT UNSIGNED NOT NULL DEFAULT 1,
  is_del TINYINT UNSIGNED NOT NULL DEFAULT 2,
  created_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_tool_maps_code (code, is_del),
  KEY idx_ai_tool_maps_app (app_id, status, is_del)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI tool maps';

CREATE TABLE ai_usage_daily (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  usage_date DATE NOT NULL,
  app_id BIGINT UNSIGNED NOT NULL,
  engine_connection_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  run_count INT NOT NULL DEFAULT 0,
  fail_count INT NOT NULL DEFAULT 0,
  prompt_tokens BIGINT NOT NULL DEFAULT 0,
  completion_tokens BIGINT NOT NULL DEFAULT 0,
  total_tokens BIGINT NOT NULL DEFAULT 0,
  cost DECIMAL(18,6) NOT NULL DEFAULT 0,
  latency_ms_total BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_ai_usage_daily_scope (usage_date, app_id, engine_connection_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI usage daily aggregate';

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

DELETE rp FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE p.platform = 'admin'
  AND p.is_del = 2
  AND (
      p.i18n_key IN ('menu.ai_prompts', 'menu.ai_models', 'menu.ai_agents')
      OR p.code LIKE 'ai_prompt\_%'
  );

DELETE FROM permissions
WHERE platform = 'admin'
  AND is_del = 2
  AND (
      i18n_key IN ('menu.ai_prompts', 'menu.ai_models', 'menu.ai_agents')
      OR code LIKE 'ai_prompt\_%'
  );

DROP TEMPORARY TABLE IF EXISTS tmp_ai_core_pages;
CREATE TEMPORARY TABLE tmp_ai_core_pages (
  name VARCHAR(50) NOT NULL,
  path VARCHAR(255) NOT NULL,
  component VARCHAR(255) NOT NULL,
  sort INT UNSIGNED NOT NULL,
  i18n_key VARCHAR(128) NOT NULL,
  PRIMARY KEY (i18n_key)
) ENGINE=MEMORY;

INSERT INTO tmp_ai_core_pages (name, path, component, sort, i18n_key) VALUES
('供应商配置', '/ai/providers', 'ai/providers', 1, 'menu.ai_providers'),
('智能体配置', '/ai/apps', 'ai/apps', 2, 'menu.ai_apps'),
('AI对话', '/ai/chat', 'ai/chat', 3, 'menu.ai_chat'),
('知识库', '/ai/knowledge', 'ai/knowledge', 4, 'menu.ai_knowledge'),
('运行监控', '/ai/runs', 'ai/runs', 5, 'menu.ai_runs'),
('AI工具管理', '/ai/tools', 'ai/tools', 6, 'menu.ai_tools');

INSERT INTO permissions (name, path, icon, parent_id, component, platform, type, sort, code, i18n_key, show_menu, status, is_del, created_at, updated_at)
SELECT p.name, p.path, '', @ai_parent_id, p.component, 'admin', 2, p.sort, NULL, p.i18n_key, 1, 1, 2, NOW(), NOW()
FROM tmp_ai_core_pages p
WHERE NOT EXISTS (
  SELECT 1 FROM permissions existing
  WHERE existing.platform = 'admin'
    AND existing.is_del = 2
    AND existing.i18n_key = p.i18n_key
);

UPDATE permissions
SET name = '供应商配置', path = '/ai/providers', component = 'ai/providers', parent_id = @ai_parent_id, sort = 1, show_menu = 1, status = 1, is_del = 2, updated_at = NOW()
WHERE platform = 'admin' AND i18n_key = 'menu.ai_providers';
UPDATE permissions
SET name = '智能体配置', path = '/ai/apps', component = 'ai/apps', parent_id = @ai_parent_id, sort = 2, show_menu = 1, status = 1, is_del = 2, updated_at = NOW()
WHERE platform = 'admin' AND i18n_key = 'menu.ai_apps';
UPDATE permissions
SET name = 'AI对话', path = '/ai/chat', component = 'ai/chat', parent_id = @ai_parent_id, sort = 3, show_menu = 1, status = 1, is_del = 2, updated_at = NOW()
WHERE platform = 'admin' AND i18n_key = 'menu.ai_chat';
UPDATE permissions
SET name = '知识库', path = '/ai/knowledge', component = 'ai/knowledge', parent_id = @ai_parent_id, sort = 4, show_menu = 1, status = 1, is_del = 2, updated_at = NOW()
WHERE platform = 'admin' AND i18n_key = 'menu.ai_knowledge';
UPDATE permissions
SET name = '运行监控', path = '/ai/runs', component = 'ai/runs', parent_id = @ai_parent_id, sort = 5, show_menu = 1, status = 1, is_del = 2, updated_at = NOW()
WHERE platform = 'admin' AND i18n_key = 'menu.ai_runs';
UPDATE permissions
SET name = 'AI工具管理', path = '/ai/tools', component = 'ai/tools', parent_id = @ai_parent_id, sort = 6, show_menu = 1, status = 1, is_del = 2, updated_at = NOW()
WHERE platform = 'admin' AND i18n_key = 'menu.ai_tools';

INSERT INTO role_permissions (role_id, permission_id, is_del, created_at, updated_at)
SELECT DISTINCT grant_row.role_id, page_row.id, 2, NOW(), NOW()
FROM tmp_ai_core_page_grants grant_row
JOIN permissions page_row ON page_row.platform = 'admin' AND page_row.is_del = 2
 AND (
  (grant_row.page_key = 'providers' AND page_row.i18n_key = 'menu.ai_providers')
  OR (grant_row.page_key = 'apps' AND page_row.i18n_key = 'menu.ai_apps')
  OR (grant_row.page_key = 'knowledge' AND page_row.i18n_key = 'menu.ai_knowledge')
  OR (grant_row.page_key = 'tools' AND page_row.i18n_key = 'menu.ai_tools')
  OR (grant_row.page_key = 'chat' AND page_row.i18n_key = 'menu.ai_chat')
  OR (grant_row.page_key = 'runs' AND page_row.i18n_key = 'menu.ai_runs')
 )
ON DUPLICATE KEY UPDATE is_del = VALUES(is_del), updated_at = NOW();

DROP TEMPORARY TABLE IF EXISTS tmp_ai_core_buttons;
CREATE TEMPORARY TABLE tmp_ai_core_buttons (
  page_i18n_key VARCHAR(128) NOT NULL,
  name VARCHAR(64) NOT NULL,
  code VARCHAR(128) NOT NULL,
  sort INT NOT NULL,
  PRIMARY KEY (code)
) ENGINE=MEMORY;

INSERT INTO tmp_ai_core_buttons (page_i18n_key, name, code, sort) VALUES
('menu.ai_providers', '新增供应商', 'ai_engine_add', 1),
('menu.ai_providers', '编辑供应商', 'ai_engine_edit', 2),
('menu.ai_providers', '测试连接', 'ai_engine_test', 3),
('menu.ai_providers', '供应商状态', 'ai_engine_status', 4),
('menu.ai_providers', '删除供应商', 'ai_engine_del', 5),
('menu.ai_apps', '新增智能体', 'ai_app_add', 1),
('menu.ai_apps', '编辑智能体', 'ai_app_edit', 2),
('menu.ai_apps', '测试智能体', 'ai_app_test', 3),
('menu.ai_apps', '智能体状态', 'ai_app_status', 4),
('menu.ai_apps', '删除智能体', 'ai_app_del', 5),
('menu.ai_apps', '智能体绑定', 'ai_app_binding', 6),
('menu.ai_knowledge', '知识库新增', 'ai_knowledge_add', 1),
('menu.ai_knowledge', '知识库编辑', 'ai_knowledge_edit', 2),
('menu.ai_knowledge', '知识库同步', 'ai_knowledge_sync', 3),
('menu.ai_knowledge', '知识库状态', 'ai_knowledge_status', 4),
('menu.ai_knowledge', '知识库删除', 'ai_knowledge_del', 5),
('menu.ai_knowledge', '知识库文档新增', 'ai_knowledge_document_add', 6),
('menu.ai_knowledge', '知识库文档状态', 'ai_knowledge_document_status', 7),
('menu.ai_knowledge', '知识库文档刷新', 'ai_knowledge_document_refresh', 8),
('menu.ai_knowledge', '知识库文档删除', 'ai_knowledge_document_del', 9),
('menu.ai_tools', '工具新增', 'ai_tool_add', 1),
('menu.ai_tools', '工具编辑', 'ai_tool_edit', 2),
('menu.ai_tools', '工具状态', 'ai_tool_status', 3),
('menu.ai_tools', '工具删除', 'ai_tool_del', 4);

INSERT INTO permissions (name, path, icon, parent_id, component, platform, type, sort, code, i18n_key, show_menu, status, is_del, created_at, updated_at)
SELECT b.name, '', '', p.id, NULL, 'admin', 3, b.sort, b.code, '', 2, 1, 2, NOW(), NOW()
FROM tmp_ai_core_buttons b
JOIN permissions p ON p.platform = 'admin' AND p.is_del = 2 AND p.i18n_key = b.page_i18n_key
ON DUPLICATE KEY UPDATE
  name = VALUES(name), parent_id = VALUES(parent_id), sort = VALUES(sort), show_menu = VALUES(show_menu), status = VALUES(status), is_del = VALUES(is_del), updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, is_del, created_at, updated_at)
SELECT DISTINCT page_grant.role_id, button.id, 2, NOW(), NOW()
FROM role_permissions page_grant
JOIN permissions page ON page.id = page_grant.permission_id AND page.platform = 'admin' AND page.is_del = 2
JOIN tmp_ai_core_buttons b ON b.page_i18n_key = page.i18n_key
JOIN permissions button ON button.platform = 'admin' AND button.code = b.code AND button.is_del = 2
WHERE page_grant.is_del = 2
ON DUPLICATE KEY UPDATE is_del = VALUES(is_del), updated_at = NOW();

DROP TEMPORARY TABLE IF EXISTS tmp_ai_core_buttons;
DROP TEMPORARY TABLE IF EXISTS tmp_ai_core_pages;
DROP TEMPORARY TABLE IF EXISTS tmp_ai_core_page_grants;
