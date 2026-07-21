SELECT 'prompt_rows_remaining' AS invariant, COUNT(*) AS violations
FROM `ai_prompts`;

SELECT 'ai_prompt_permissions_remaining' AS invariant, COUNT(*) AS violations
FROM `permissions`
WHERE `code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

SELECT 'ai_prompt_role_grants_remaining' AS invariant, COUNT(*) AS violations
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

SELECT 'client_version_surface_remaining' AS invariant, COUNT(*) AS violations
FROM (
  SELECT permission.`id`
  FROM `permissions` AS permission
  WHERE permission.`platform` = 'admin'
    AND (
      permission.`path` = '/system/clientVersion'
      OR permission.`component` = 'system/clientVersion'
      OR permission.`i18n_key` = 'menu.system_clientVersion'
      OR permission.`code` IN (
        'system_clientVersion_add',
        'system_clientVersion_del',
        'system_clientVersion_edit',
        'system_clientVersion_forceUpdate',
        'system_clientVersion_setLatest'
      )
    )
  UNION ALL
  SELECT role_permission.`id`
  FROM `role_permissions` AS role_permission
  JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
  WHERE permission.`platform` = 'admin'
    AND (
      permission.`path` = '/system/clientVersion'
      OR permission.`component` = 'system/clientVersion'
      OR permission.`i18n_key` = 'menu.system_clientVersion'
      OR permission.`code` IN (
        'system_clientVersion_add',
        'system_clientVersion_del',
        'system_clientVersion_edit',
        'system_clientVersion_forceUpdate',
        'system_clientVersion_setLatest'
      )
    )
) AS active_client_version_surface;

SELECT 'retired_platform_rows_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations FROM `permissions` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `role_permissions` AS rp JOIN `permissions` AS p ON p.`id` = rp.`permission_id` WHERE p.`platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `user_sessions` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `users_login_log` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `auth_platforms` WHERE `code` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `notification_task` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `notifications` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `export_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `authz_principal_versions` WHERE `platform` IN ('app', 'canvas')
) AS retired_platform;

SELECT 'historical_all_rows_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations FROM `notification_task` WHERE `platform` = 'all'
  UNION ALL SELECT COUNT(*) FROM `notifications` WHERE `platform` = 'all'
) AS historical_all;

SELECT 'preexisting_orphan_grant_remaining' AS invariant, COUNT(*) AS violations
FROM `role_permissions`
WHERE `id` = 723 AND `role_id` = 1 AND `permission_id` = 539;

SELECT 'retired_ai_rows_remaining' AS invariant, COALESCE(SUM(`violations`), 0) AS violations
FROM (
  SELECT COUNT(*) AS violations FROM `ai_runs` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_run_events` AS event_row JOIN `ai_runs` AS run_row ON run_row.`id` = event_row.`run_id` WHERE run_row.`platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_tool_calls` AS tool_call JOIN `ai_runs` AS run_row ON run_row.`id` = tool_call.`run_id` WHERE run_row.`platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_knowledge_retrievals` AS retrieval JOIN `ai_runs` AS run_row ON run_row.`id` = retrieval.`run_id` WHERE run_row.`platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_knowledge_retrieval_hits` AS hit JOIN `ai_knowledge_retrievals` AS retrieval ON retrieval.`id` = hit.`retrieval_id` JOIN `ai_runs` AS run_row ON run_row.`id` = retrieval.`run_id` WHERE run_row.`platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_image_files` AS image_file JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id` WHERE image_task.`platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_image_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_text_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_video_tasks` WHERE `platform` IN ('app', 'canvas')
  UNION ALL SELECT COUNT(*) FROM `ai_reply_commands` WHERE `platform` IN ('app', 'canvas')
) AS retired_ai;

SELECT 'orphan_ai_image_files_remaining' AS invariant, COUNT(*) AS violations
FROM `ai_image_files` AS image_file
LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`id` IS NULL;

SELECT 'retired_scene_values_remaining' AS invariant, COUNT(*) AS violations
FROM `ai_agents`
WHERE JSON_VALID(`scenes_json`)
  AND (
    JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_text_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_image_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_video_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_audio_generate'))
  );

SELECT 'unknown_scene_values_remaining' AS invariant, COUNT(*) AS violations
FROM `ai_agents` AS agent
JOIN JSON_TABLE(
  CASE WHEN JSON_VALID(agent.`scenes_json`) THEN agent.`scenes_json` ELSE JSON_ARRAY() END,
  '$[*]' COLUMNS (`scene` VARCHAR(64) PATH '$')
) AS agent_scene
WHERE agent_scene.`scene` NOT IN (
  'chat',
  'agent_generate',
  'text_generate',
  'image_generate',
  'video_generate',
  'audio_generate'
);
