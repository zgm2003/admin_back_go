-- Roll back the local AI core rebuild from backup tables.
-- Dify sidecar data is not touched. External Dify cleanup is an operator step.

DROP TABLE IF EXISTS ai_providers;
DROP TABLE IF EXISTS ai_agents;
DROP TABLE IF EXISTS ai_agent_bindings;
DROP TABLE IF EXISTS ai_conversations;
DROP TABLE IF EXISTS ai_messages;
DROP TABLE IF EXISTS ai_runs;
DROP TABLE IF EXISTS ai_run_events;
DROP TABLE IF EXISTS ai_knowledge_maps;
DROP TABLE IF EXISTS ai_knowledge_documents;
DROP TABLE IF EXISTS ai_tool_maps;
DROP TABLE IF EXISTS ai_usage_daily;

DELETE rp
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

DELETE FROM permissions
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

INSERT INTO permissions SELECT * FROM ai_permissions_backup_20260508;
INSERT INTO role_permissions SELECT * FROM ai_role_permissions_backup_20260508;

CREATE TABLE ai_models AS SELECT * FROM ai_models_backup_20260508;
CREATE TABLE ai_tools AS SELECT * FROM ai_tools_backup_20260508;
CREATE TABLE ai_prompts AS SELECT * FROM ai_prompts_backup_20260508;
CREATE TABLE ai_prompt AS SELECT * FROM ai_prompt_backup_20260508;
CREATE TABLE ai_agents AS SELECT * FROM ai_agents_backup_20260508;
CREATE TABLE ai_agent_scenes AS SELECT * FROM ai_agent_scenes_backup_20260508;
CREATE TABLE ai_assistant_tools AS SELECT * FROM ai_assistant_tools_backup_20260508;
CREATE TABLE ai_agent_knowledge_bases AS SELECT * FROM ai_agent_knowledge_bases_backup_20260508;
CREATE TABLE ai_knowledge_bases AS SELECT * FROM ai_knowledge_bases_backup_20260508;
CREATE TABLE ai_knowledge_documents AS SELECT * FROM ai_knowledge_documents_backup_20260508;
CREATE TABLE ai_knowledge_chunks AS SELECT * FROM ai_knowledge_chunks_backup_20260508;
CREATE TABLE ai_conversations AS SELECT * FROM ai_conversations_backup_20260508;
CREATE TABLE ai_messages AS SELECT * FROM ai_messages_backup_20260508;
CREATE TABLE ai_runs AS SELECT * FROM ai_runs_backup_20260508;
CREATE TABLE ai_run_steps AS SELECT * FROM ai_run_steps_backup_20260508;
