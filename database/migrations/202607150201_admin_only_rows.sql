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

CREATE TEMPORARY TABLE `contract_retired_ai_runs` (
  `id` BIGINT UNSIGNED NOT NULL PRIMARY KEY
);

INSERT INTO `contract_retired_ai_runs` (`id`)
SELECT `id`
FROM `ai_runs`
WHERE `platform` IN ('app', 'canvas');

INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 417, 0, 1));

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
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 34, 0, 1));

DELETE FROM `role_permissions`
WHERE `id` = 723 AND `role_id` = 1 AND `permission_id` = 539 AND `is_del` = 2;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 1, 0, 1));

DELETE FROM `permissions`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 17, 0, 1));

DELETE FROM `user_sessions`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 99, 0, 1));

DELETE FROM `users_login_log`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 113, 0, 1));

DELETE FROM `authz_principal_versions`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE FROM `auth_platforms`
WHERE `code` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 2, 0, 1));

UPDATE `notification_task`
SET `platform` = 'admin'
WHERE `platform` = 'all';
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 6, 0, 1));

DELETE FROM `notification_task`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

UPDATE `notifications`
SET `platform` = 'admin'
WHERE `platform` = 'all';
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 3061, 0, 1));

DELETE FROM `notifications`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE FROM `export_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

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
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 10, 0, 1));

DELETE FROM `permissions`
WHERE `code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 5, 0, 1));

UPDATE `authz_principal_versions` AS principal_version
JOIN `users` AS user_row ON user_row.`id` = principal_version.`user_id`
SET principal_version.`version` = principal_version.`version` + 1,
    principal_version.`updated_at` = UTC_TIMESTAMP(6)
WHERE principal_version.`platform` = 'admin';
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 7, 0, 1));

DELETE FROM `ai_prompts`;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 1356, 0, 1));

DELETE hit
FROM `ai_knowledge_retrieval_hits` AS hit
JOIN `ai_knowledge_retrievals` AS retrieval ON retrieval.`id` = hit.`retrieval_id`
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE retrieval
FROM `ai_knowledge_retrievals` AS retrieval
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = retrieval.`run_id`;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE tool_call
FROM `ai_tool_calls` AS tool_call
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = tool_call.`run_id`;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE event_row
FROM `ai_run_events` AS event_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = event_row.`run_id`;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 834, 0, 1));

DELETE image_file
FROM `ai_image_files` AS image_file
JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 426, 0, 1));

DELETE image_file
FROM `ai_image_files` AS image_file
LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
WHERE image_task.`id` IS NULL;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 11, 0, 1));

DELETE FROM `ai_image_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 409, 0, 1));

DELETE FROM `ai_text_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE FROM `ai_video_tasks`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE FROM `ai_reply_commands`
WHERE `platform` IN ('app', 'canvas');
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 0, 0, 1));

DELETE run_row
FROM `ai_runs` AS run_row
JOIN `contract_retired_ai_runs` AS retired_run ON retired_run.`id` = run_row.`id`;
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 417, 0, 1));

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
INSERT INTO `_p09_contract_guard` VALUES (IF(ROW_COUNT() = 5, 0, 1));

COMMIT;

DROP TEMPORARY TABLE `contract_retired_ai_runs`;
DROP TEMPORARY TABLE `_p09_contract_guard`;
