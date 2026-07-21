-- Remove only the two approved retired tables and the one proven-unused
-- session compatibility column. No COS object is read, changed, or deleted.
CREATE TEMPORARY TABLE `_p09_schema_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN (
    'users_quick_entry',
    'canvas_prompts',
    'canvas_assets',
    'ai_billing_rules',
    'ai_billing_records'
  );

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'ai_prompts'
  AND `table_type` = 'BASE TABLE';

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_prompts`;

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'ai_video_tasks'
  AND `table_type` = 'BASE TABLE';

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'canvas_video_tasks'
  AND `table_type` = 'BASE TABLE';

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `canvas_video_tasks`;

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'client_versions'
  AND `table_type` = 'BASE TABLE';

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 8, 0, 1)
FROM `client_versions`;

INSERT INTO `_p09_schema_guard`
SELECT IF(
  SHA2(
    COALESCE(
      GROUP_CONCAT(
        SHA2(
          CAST(JSON_ARRAY(
            `id`, `version`, `notes`, `file_url`, `signature`, `platform`,
            `file_size`, `is_latest`, `force_update`, `is_del`, `created_at`, `updated_at`
          ) AS CHAR),
          256
        )
        ORDER BY `id` SEPARATOR ''
      ),
      ''
    ),
    256
  ) = 'ca574b6ce101d92b05cc3571e7e138aa9bf2bc5096c04357c8d39792ba806661',
  0,
  1
)
FROM `client_versions`;

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`key_column_usage`
WHERE `referenced_table_schema` = DATABASE()
  AND `referenced_table_name` IN ('canvas_video_tasks', 'client_versions');

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`views`
WHERE `table_schema` = DATABASE()
  AND (
    LOWER(`view_definition`) LIKE '%canvas_video_tasks%'
    OR LOWER(`view_definition`) LIKE '%client_versions%'
  );

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`triggers`
WHERE `trigger_schema` = DATABASE()
  AND (
    LOWER(`action_statement`) LIKE '%canvas_video_tasks%'
    OR LOWER(`action_statement`) LIKE '%client_versions%'
  );

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`events`
WHERE `event_schema` = DATABASE()
  AND (
    LOWER(`event_definition`) LIKE '%canvas_video_tasks%'
    OR LOWER(`event_definition`) LIKE '%client_versions%'
  );

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`routines`
WHERE `routine_schema` = DATABASE()
  AND (
    LOWER(COALESCE(`routine_definition`, '')) LIKE '%canvas_video_tasks%'
    OR LOWER(COALESCE(`routine_definition`, '')) LIKE '%client_versions%'
  );

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `information_schema`.`columns`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'user_sessions'
  AND `column_name` = 'access_token_hash'
  AND `is_nullable` = 'NO'
  AND `column_type` = 'char(64)';

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM (
  SELECT `index_name`
  FROM `information_schema`.`statistics`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'user_sessions'
    AND `index_name` = 'uniq_access_hash'
  GROUP BY `index_name`
  HAVING COUNT(*) = 1
    AND MAX(`non_unique`) = 0
    AND MAX(`column_name`) = 'access_token_hash'
) AS expected_access_hash_index;

DROP TABLE `canvas_video_tasks`;
DROP TABLE `client_versions`;

ALTER TABLE `user_sessions`
  DROP INDEX `uniq_access_hash`,
  DROP COLUMN `access_token_hash`;

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN ('canvas_video_tasks', 'client_versions');

INSERT INTO `_p09_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`columns`
WHERE `table_schema` = DATABASE()
  AND `table_name` = 'user_sessions'
  AND `column_name` = 'access_token_hash';

DROP TEMPORARY TABLE `_p09_schema_guard`;
