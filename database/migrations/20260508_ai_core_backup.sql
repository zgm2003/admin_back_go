-- Backup current AI core tables and AI menu permissions before destructive Dify sidecar rebuild.
-- This file is intentionally non-destructive. Run it before 20260508_ai_core_rebuild.sql.

CREATE TABLE IF NOT EXISTS ai_models_backup_20260508 AS SELECT * FROM ai_models;
CREATE TABLE IF NOT EXISTS ai_tools_backup_20260508 AS SELECT * FROM ai_tools;
CREATE TABLE IF NOT EXISTS ai_prompts_backup_20260508 AS SELECT * FROM ai_prompts;
CREATE TABLE IF NOT EXISTS ai_prompt_backup_20260508 AS SELECT * FROM ai_prompt;
CREATE TABLE IF NOT EXISTS ai_agents_backup_20260508 AS SELECT * FROM ai_agents;
CREATE TABLE IF NOT EXISTS ai_agent_scenes_backup_20260508 AS SELECT * FROM ai_agent_scenes;
CREATE TABLE IF NOT EXISTS ai_assistant_tools_backup_20260508 AS SELECT * FROM ai_assistant_tools;
CREATE TABLE IF NOT EXISTS ai_agent_knowledge_bases_backup_20260508 AS SELECT * FROM ai_agent_knowledge_bases;
CREATE TABLE IF NOT EXISTS ai_knowledge_bases_backup_20260508 AS SELECT * FROM ai_knowledge_bases;
CREATE TABLE IF NOT EXISTS ai_knowledge_documents_backup_20260508 AS SELECT * FROM ai_knowledge_documents;
CREATE TABLE IF NOT EXISTS ai_knowledge_chunks_backup_20260508 AS SELECT * FROM ai_knowledge_chunks;
CREATE TABLE IF NOT EXISTS ai_conversations_backup_20260508 AS SELECT * FROM ai_conversations;
CREATE TABLE IF NOT EXISTS ai_messages_backup_20260508 AS SELECT * FROM ai_messages;
CREATE TABLE IF NOT EXISTS ai_runs_backup_20260508 AS SELECT * FROM ai_runs;
CREATE TABLE IF NOT EXISTS ai_run_steps_backup_20260508 AS SELECT * FROM ai_run_steps;

CREATE TABLE IF NOT EXISTS ai_permissions_backup_20260508 AS
SELECT *
FROM permissions
WHERE platform = 'admin'
  AND (
      path = '/ai'
      OR path LIKE '/ai/%'
      OR component = 'ai'
      OR component LIKE 'ai/%'
      OR i18n_key = 'menu.ai'
      OR i18n_key LIKE 'menu.ai\_%'
      OR code LIKE 'ai\_%'
  );

CREATE TABLE IF NOT EXISTS ai_role_permissions_backup_20260508 AS
SELECT rp.*
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE p.platform = 'admin'
  AND (
      p.path = '/ai'
      OR p.path LIKE '/ai/%'
      OR p.component = 'ai'
      OR p.component LIKE 'ai/%'
      OR p.i18n_key = 'menu.ai'
      OR p.i18n_key LIKE 'menu.ai\_%'
      OR p.code LIKE 'ai\_%'
  );
