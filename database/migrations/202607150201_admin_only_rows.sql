-- Retire only the product rows classified by the frozen P09 evidence.
-- The guard table converts every evidence mismatch into a hard SQL failure.
CREATE TEMPORARY TABLE `_p09_contract_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 17, 0, 1)
FROM `permissions`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 34, 0, 1)
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(
  COUNT(*) = 1
  AND MAX(role_permission.`role_id`) = 1
  AND MAX(role_permission.`permission_id`) = 539
  AND MAX(permission.`id`) IS NULL,
  0,
  1
)
FROM `role_permissions` AS role_permission
LEFT JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE role_permission.`id` = 723 AND role_permission.`is_del` = 2;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 99, 0, 1)
FROM `user_sessions`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 113, 0, 1)
FROM `users_login_log`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 2, 0, 1)
FROM `auth_platforms`
WHERE `code` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 1 AND SUM(`code` = 'admin' AND `status` = 1 AND `is_del` = 2) = 1, 0, 1)
FROM `auth_platforms`
WHERE `code` = 'admin';

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 6, 0, 1)
FROM `notification_task`
WHERE `platform` = 'all';

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `notification_task`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 3061, 0, 1)
FROM `notifications`
WHERE `platform` = 'all';

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `notifications`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `export_tasks`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 1356 AND SUM(`is_del` = 2) = 1356, 0, 1)
FROM `ai_prompts`;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 5 AND SUM(`platform` = 'admin' AND `is_del` = 2) = 5, 0, 1)
FROM `permissions`
WHERE `code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 10 AND SUM(role_permission.`is_del` = 2) = 10, 0, 1)
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`key_column_usage`
WHERE `referenced_table_schema` = DATABASE()
  AND `referenced_table_name` = 'ai_prompts';

-- P08R may already have soft-deleted these rows. The locked set is physical
-- identity-based, so cleanup must include both is_del states.
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 6, 0, 1)
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
  );

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 12, 0, 1)
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
  );

CREATE TEMPORARY TABLE `contract_retired_ai_runs` (
  `id` BIGINT UNSIGNED NOT NULL PRIMARY KEY
);

INSERT INTO `contract_retired_ai_runs` (`id`)
SELECT `id`
FROM `ai_runs`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 417, 0, 1)
FROM `contract_retired_ai_runs`;

CREATE TEMPORARY TABLE `contract_admin_principal_versions` (
  `user_id` BIGINT NOT NULL PRIMARY KEY,
  `version` BIGINT UNSIGNED NOT NULL
);

INSERT INTO `contract_admin_principal_versions` (`user_id`, `version`)
SELECT principal_version.`user_id`, principal_version.`version`
FROM `authz_principal_versions` AS principal_version
JOIN `users` AS user_row ON user_row.`id` = principal_version.`user_id`
WHERE principal_version.`platform` = 'admin'
  AND user_row.`status` = 1
  AND user_row.`is_del` = 2;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 7, 0, 1)
FROM `contract_admin_principal_versions`;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 834, 0, 1)
FROM `ai_run_events` AS event_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = event_row.`run_id`;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_tool_calls` AS tool_call
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = tool_call.`run_id`;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_knowledge_retrievals` AS retrieval
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_knowledge_retrieval_hits` AS hit
JOIN `ai_knowledge_retrievals` AS retrieval ON retrieval.`id` = hit.`retrieval_id`
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 409, 0, 1)
FROM `ai_image_tasks`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 426, 0, 1)
FROM `ai_image_files` AS image_file
JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 11, 0, 1)
FROM `ai_image_files` AS image_file
LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`id` IS NULL;

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_text_tasks`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_video_tasks`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_reply_commands`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `authz_principal_versions`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard`
SELECT IF(
  SUM(JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_text_generate'))) = 2
  AND SUM(JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_image_generate'))) = 3
  AND SUM(JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_video_generate'))) = 0
  AND SUM(JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_audio_generate'))) = 0,
  0,
  1
)
FROM `ai_agents`
WHERE JSON_VALID(`scenes_json`);

INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents`
WHERE JSON_VALID(`scenes_json`)
  AND (
    (JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_text_generate')) AND JSON_CONTAINS(`scenes_json`, JSON_QUOTE('text_generate')))
    OR (JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_image_generate')) AND JSON_CONTAINS(`scenes_json`, JSON_QUOTE('image_generate')))
    OR (JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_video_generate')) AND JSON_CONTAINS(`scenes_json`, JSON_QUOTE('video_generate')))
    OR (JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_audio_generate')) AND JSON_CONTAINS(`scenes_json`, JSON_QUOTE('audio_generate')))
  );

START TRANSACTION;

DELETE role_permission
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`platform` IN ('app', 'canvas');

DELETE FROM `role_permissions`
WHERE `id` = 723 AND `role_id` = 1 AND `permission_id` = 539 AND `is_del` = 2;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `role_permissions`
WHERE `id` = 723 AND `role_id` = 1 AND `permission_id` = 539;

DELETE FROM `permissions`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `user_sessions`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `user_sessions`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `users_login_log`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `users_login_log`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `authz_principal_versions`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `authz_principal_versions`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `auth_platforms`
WHERE `code` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `auth_platforms`
WHERE `code` IN ('app', 'canvas');

UPDATE `notification_task`
SET `platform` = 'admin'
WHERE `platform` = 'all';
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `notification_task`
WHERE `platform` = 'all';

DELETE FROM `notification_task`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `notification_task`
WHERE `platform` IN ('app', 'canvas');

UPDATE `notifications`
SET `platform` = 'admin'
WHERE `platform` = 'all';
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `notifications`
WHERE `platform` = 'all';

DELETE FROM `notifications`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `notifications`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `export_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `export_tasks`
WHERE `platform` IN ('app', 'canvas');

DELETE role_permission
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

DELETE FROM `permissions`
WHERE `code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

DELETE role_permission
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
  );
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
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
  );

DELETE FROM `permissions`
WHERE `platform` = 'admin'
  AND (
    `path` = '/system/clientVersion'
    OR `component` = 'system/clientVersion'
    OR `i18n_key` = 'menu.system_clientVersion'
    OR `code` IN (
      'system_clientVersion_add',
      'system_clientVersion_del',
      'system_clientVersion_edit',
      'system_clientVersion_forceUpdate',
      'system_clientVersion_setLatest'
    )
  );
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `platform` = 'admin'
  AND (
    `path` = '/system/clientVersion'
    OR `component` = 'system/clientVersion'
    OR `i18n_key` = 'menu.system_clientVersion'
    OR `code` IN (
      'system_clientVersion_add',
      'system_clientVersion_del',
      'system_clientVersion_edit',
      'system_clientVersion_forceUpdate',
      'system_clientVersion_setLatest'
    )
  );

UPDATE `authz_principal_versions` AS principal_version
JOIN `users` AS user_row ON user_row.`id` = principal_version.`user_id`
SET principal_version.`version` = principal_version.`version` + 1,
    principal_version.`updated_at` = UTC_TIMESTAMP(6)
WHERE principal_version.`platform` = 'admin'
  AND user_row.`status` = 1
  AND user_row.`is_del` = 2;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 7, 0, 1)
FROM `authz_principal_versions` AS principal_version
JOIN `contract_admin_principal_versions` AS before_version
  ON before_version.`user_id` = principal_version.`user_id`
WHERE principal_version.`platform` = 'admin'
  AND principal_version.`version` = before_version.`version` + 1;

DELETE FROM `ai_prompts`;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_prompts`;

DELETE hit
FROM `ai_knowledge_retrieval_hits` AS hit
JOIN `ai_knowledge_retrievals` AS retrieval ON retrieval.`id` = hit.`retrieval_id`
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_knowledge_retrieval_hits` AS hit
JOIN `ai_knowledge_retrievals` AS retrieval ON retrieval.`id` = hit.`retrieval_id`
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;

DELETE retrieval
FROM `ai_knowledge_retrievals` AS retrieval
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_knowledge_retrievals` AS retrieval
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;

DELETE tool_call
FROM `ai_tool_calls` AS tool_call
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = tool_call.`run_id`;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_tool_calls` AS tool_call
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = tool_call.`run_id`;

DELETE event_row
FROM `ai_run_events` AS event_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = event_row.`run_id`;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_run_events` AS event_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = event_row.`run_id`;

DELETE image_file
FROM `ai_image_files` AS image_file
JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_image_files` AS image_file
JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`platform` IN ('app', 'canvas');

DELETE image_file
FROM `ai_image_files` AS image_file
LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`id` IS NULL;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_image_files` AS image_file
LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`id` IS NULL;

DELETE FROM `ai_image_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_image_tasks`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `ai_text_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_text_tasks`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `ai_video_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_video_tasks`
WHERE `platform` IN ('app', 'canvas');

DELETE FROM `ai_reply_commands`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_reply_commands`
WHERE `platform` IN ('app', 'canvas');

DELETE run_row
FROM `ai_runs` AS run_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = run_row.`id`;
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_runs` AS run_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = run_row.`id`;

UPDATE `ai_agents`
SET `scenes_json` = REPLACE(REPLACE(REPLACE(REPLACE(
  `scenes_json`,
  '"canvas_text_generate"', '"text_generate"'),
  '"canvas_image_generate"', '"image_generate"'),
  '"canvas_video_generate"', '"video_generate"'),
  '"canvas_audio_generate"', '"audio_generate"')
WHERE JSON_VALID(`scenes_json`)
  AND (
    JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_text_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_image_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_video_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_audio_generate'))
  );
INSERT INTO `_p09_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents`
WHERE JSON_VALID(`scenes_json`)
  AND (
    JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_text_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_image_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_video_generate'))
    OR JSON_CONTAINS(`scenes_json`, JSON_QUOTE('canvas_audio_generate'))
  );

COMMIT;

DROP TEMPORARY TABLE `contract_retired_ai_runs`;
DROP TEMPORARY TABLE `contract_admin_principal_versions`;
DROP TEMPORARY TABLE `_p09_contract_guard`;
