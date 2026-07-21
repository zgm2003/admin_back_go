SELECT 'retired_schema_surface_remaining' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN (
    'users_quick_entry',
    'canvas_prompts',
    'canvas_assets',
    'canvas_video_tasks',
    'client_versions',
    'ai_billing_rules',
    'ai_billing_records'
  );

SELECT 'client_version_references_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations
  FROM `information_schema`.`key_column_usage`
  WHERE `referenced_table_schema` = DATABASE()
    AND `referenced_table_name` = 'client_versions'
  UNION ALL
  SELECT COUNT(*)
  FROM `information_schema`.`views`
  WHERE `table_schema` = DATABASE()
    AND LOWER(`view_definition`) LIKE '%client_versions%'
  UNION ALL
  SELECT COUNT(*)
  FROM `information_schema`.`triggers`
  WHERE `trigger_schema` = DATABASE()
    AND LOWER(`action_statement`) LIKE '%client_versions%'
  UNION ALL
  SELECT COUNT(*)
  FROM `information_schema`.`events`
  WHERE `event_schema` = DATABASE()
    AND LOWER(`event_definition`) LIKE '%client_versions%'
  UNION ALL
  SELECT COUNT(*)
  FROM `information_schema`.`routines`
  WHERE `routine_schema` = DATABASE()
    AND LOWER(COALESCE(`routine_definition`, '')) LIKE '%client_versions%'
) AS client_version_reference;

SELECT 'access_token_hash_remaining' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`columns`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'user_sessions'
  AND `column_name` = 'access_token_hash';

SELECT 'access_token_hash_index_remaining' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`statistics`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'user_sessions'
  AND `index_name` = 'uniq_access_hash';

SELECT 'ai_prompts_table_missing' AS invariant,
  IF(COUNT(*) = 1, 0, 1) AS violations
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'ai_prompts'
  AND `table_type` = 'BASE TABLE';

SELECT 'ai_prompts_rows_remaining' AS invariant, COUNT(*) AS violations
FROM `ai_prompts`;

SELECT 'ai_video_tasks_table_missing' AS invariant,
  IF(COUNT(*) = 1, 0, 1) AS violations
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'ai_video_tasks'
  AND `table_type` = 'BASE TABLE';

SELECT 'retired_ai_contract_rows_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations FROM `ai_runs` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_text_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_image_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_video_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_reply_commands` WHERE `platform` IN ('app', 'canvas')
) AS retired_ai_contract;

SELECT 'retired_scene_values_remaining' AS invariant, COUNT(*) AS violations
FROM `ai_agents`
WHERE JSON_VALID(`scenes_json`)
  AND (
    JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_text_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_image_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_video_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_audio_generate'))
  );
