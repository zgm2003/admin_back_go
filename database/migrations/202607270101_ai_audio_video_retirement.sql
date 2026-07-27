-- Apply only after the deployed API and Worker no longer contain audio/video
-- generation writers, routes, provider factories, or recovery handlers:
--   SET @ai_audio_video_retirement_verified = 1;

DROP TEMPORARY TABLE IF EXISTS `_ai_audio_video_retirement_guard`;
CREATE TEMPORARY TABLE `_ai_audio_video_retirement_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COALESCE(@ai_audio_video_retirement_verified, 0) = 1, 0, 1);

-- Rows containing a retired scene must be arrays. JSON columns guarantee valid
-- JSON bytes, but the legacy value could still have a non-array JSON shape.
INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents` AS agent
WHERE (
    JSON_SEARCH(agent.`scenes_json`, 'one', 'video_generate') IS NOT NULL
    OR JSON_SEARCH(agent.`scenes_json`, 'one', 'audio_generate') IS NOT NULL
  )
  AND JSON_TYPE(agent.`scenes_json`) <> 'ARRAY';

-- Do not silently discard an unknown scene while rebuilding a touched array.
INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents` AS agent
JOIN JSON_TABLE(
  CASE
    WHEN JSON_TYPE(agent.`scenes_json`) = 'ARRAY' THEN agent.`scenes_json`
    ELSE JSON_ARRAY()
  END,
  '$[*]' COLUMNS (
    `ordinality` FOR ORDINALITY,
    `scene` VARCHAR(64) PATH '$' ERROR ON ERROR
  )
) AS scene_row
WHERE (
    JSON_SEARCH(agent.`scenes_json`, 'one', 'video_generate') IS NOT NULL
    OR JSON_SEARCH(agent.`scenes_json`, 'one', 'audio_generate') IS NOT NULL
  )
  AND (
    scene_row.`scene` IS NULL
    OR scene_row.`scene` NOT IN (
      'chat', 'agent_generate', 'text_generate', 'image_generate',
      'video_generate', 'audio_generate'
    )
  );

-- Unknown and active task states fail closed. Only terminal rows may lose their
-- task-specific projection; their Run and billing evidence remains canonical.
INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_video_tasks`
WHERE `status` NOT IN ('completed', 'failed', 'cancelled');

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_audio_tasks`
WHERE `status` NOT IN ('success', 'failed', 'canceled', 'outcome_unknown');

-- A terminal task projection cannot be dropped while any linked durable fact
-- is missing or still open. This also catches inconsistent legacy task states.
INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT 'video' AS `kind`, `id`, `run_id` FROM `ai_video_tasks`
  UNION ALL
  SELECT 'audio' AS `kind`, `id`, `run_id` FROM `ai_audio_tasks`
) AS task
LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id`
LEFT JOIN `ai_usage_charges` AS charge ON charge.`run_id` = task.`run_id`
LEFT JOIN `wallet_holds` AS hold_row ON hold_row.`run_id` = task.`run_id`
LEFT JOIN `ai_provider_attempts` AS attempt ON attempt.`run_id` = task.`run_id`
WHERE run_row.`id` IS NULL
   OR run_row.`status` = 'running'
   OR run_row.`billing_status` IN ('pending', 'held')
   OR charge.`status` = 'open'
   OR hold_row.`status` = 'active'
   OR attempt.`state` IN ('prepared', 'dispatched');

-- Other tables must not point at the retiring projections. Their own FKs to
-- ai_runs are owned by the dropped tables and are intentionally not counted.
INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`KEY_COLUMN_USAGE`
WHERE `REFERENCED_TABLE_SCHEMA` = DATABASE()
  AND `REFERENCED_TABLE_NAME` IN ('ai_video_tasks', 'ai_audio_tasks')
  AND `TABLE_NAME` NOT IN ('ai_video_tasks', 'ai_audio_tasks');

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`VIEW_TABLE_USAGE`
WHERE `TABLE_SCHEMA` = DATABASE()
  AND `TABLE_NAME` IN ('ai_video_tasks', 'ai_audio_tasks');

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`TRIGGERS`
WHERE `TRIGGER_SCHEMA` = DATABASE()
  AND (
    `EVENT_OBJECT_TABLE` IN ('ai_video_tasks', 'ai_audio_tasks')
    OR `ACTION_STATEMENT` IS NULL
    OR LOWER(`ACTION_STATEMENT`) LIKE '%ai_video_tasks%'
    OR LOWER(`ACTION_STATEMENT`) LIKE '%ai_audio_tasks%'
  );

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`EVENTS`
WHERE `EVENT_SCHEMA` = DATABASE()
  AND (
    `EVENT_DEFINITION` IS NULL
    OR LOWER(`EVENT_DEFINITION`) LIKE '%ai_video_tasks%'
    OR LOWER(`EVENT_DEFINITION`) LIKE '%ai_audio_tasks%'
  );

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`ROUTINES`
WHERE `ROUTINE_SCHEMA` = DATABASE()
  AND (
    `ROUTINE_DEFINITION` IS NULL
    OR LOWER(`ROUTINE_DEFINITION`) LIKE '%ai_video_tasks%'
    OR LOWER(`ROUTINE_DEFINITION`) LIKE '%ai_audio_tasks%'
  );

DROP TEMPORARY TABLE IF EXISTS `_ai_audio_video_agent_scenes`;
CREATE TEMPORARY TABLE `_ai_audio_video_agent_scenes` (
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `retained_scenes_json` JSON NOT NULL,
  `retained_count` INT UNSIGNED NOT NULL,
  `retired_count` INT UNSIGNED NOT NULL,
  PRIMARY KEY (`agent_id`)
);

INSERT INTO `_ai_audio_video_agent_scenes` (
  `agent_id`, `retained_scenes_json`, `retained_count`, `retired_count`
)
SELECT
  agent.`id`,
  CAST(CONCAT(
    '[',
    COALESCE(GROUP_CONCAT(
      CASE
        WHEN scene_row.`scene` IN ('chat', 'agent_generate', 'text_generate', 'image_generate')
          THEN JSON_QUOTE(scene_row.`scene`)
        ELSE NULL
      END
      ORDER BY scene_row.`ordinality` SEPARATOR ','
    ), ''),
    ']'
  ) AS JSON),
  SUM(scene_row.`scene` IN ('chat', 'agent_generate', 'text_generate', 'image_generate')),
  SUM(scene_row.`scene` IN ('video_generate', 'audio_generate'))
FROM `ai_agents` AS agent
JOIN JSON_TABLE(
  agent.`scenes_json`,
  '$[*]' COLUMNS (
    `ordinality` FOR ORDINALITY,
    `scene` VARCHAR(64) PATH '$' ERROR ON ERROR
  )
) AS scene_row
WHERE JSON_TYPE(agent.`scenes_json`) = 'ARRAY'
  AND (
    JSON_SEARCH(agent.`scenes_json`, 'one', 'video_generate') IS NOT NULL
    OR JSON_SEARCH(agent.`scenes_json`, 'one', 'audio_generate') IS NOT NULL
  )
GROUP BY agent.`id`;

-- Mixed agents keep only supported scenes in their original order.
UPDATE `ai_agents` AS agent
JOIN `_ai_audio_video_agent_scenes` AS migrated ON migrated.`agent_id` = agent.`id`
SET agent.`scenes_json` = migrated.`retained_scenes_json`,
    agent.`updated_at` = CURRENT_TIMESTAMP
WHERE migrated.`retained_count` > 0;

-- Audio/video-only agents are disabled and hidden atomically. An empty JSON
-- array is deliberate; these rows must never be converted into chat agents.
UPDATE `ai_agents` AS agent
JOIN `_ai_audio_video_agent_scenes` AS migrated ON migrated.`agent_id` = agent.`id`
SET agent.`scenes_json` = JSON_ARRAY(),
    agent.`status` = 2,
    agent.`is_del` = 1,
    agent.`updated_at` = CURRENT_TIMESTAMP
WHERE migrated.`retained_count` = 0
  AND migrated.`retired_count` > 0;

-- Validate the exact migrated projection before the destructive DDL. The
-- guard table rejects the statement and leaves the task tables intact.
INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents` AS agent
JOIN `_ai_audio_video_agent_scenes` AS migrated ON migrated.`agent_id` = agent.`id`
WHERE agent.`scenes_json` IS NULL
   OR JSON_TYPE(agent.`scenes_json`) <> 'ARRAY'
   OR (migrated.`retained_count` > 0
       AND CAST(agent.`scenes_json` AS CHAR) <> CAST(migrated.`retained_scenes_json` AS CHAR))
   OR (migrated.`retained_count` = 0
       AND (agent.`status` <> 2 OR agent.`is_del` <> 1 OR JSON_LENGTH(agent.`scenes_json`) <> 0));

INSERT INTO `_ai_audio_video_retirement_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents` AS agent
WHERE agent.`is_del` = 2
  AND (
    JSON_SEARCH(agent.`scenes_json`, 'one', 'video_generate') IS NOT NULL
    OR JSON_SEARCH(agent.`scenes_json`, 'one', 'audio_generate') IS NOT NULL
  );

-- MySQL 8 atomic DDL removes both projections together. No Run, provider
-- attempt, charge, usage item, Hold, wallet transaction, or asset row is deleted.
DROP TABLE `ai_video_tasks`, `ai_audio_tasks`;

DROP TEMPORARY TABLE `_ai_audio_video_agent_scenes`;
DROP TEMPORARY TABLE `_ai_audio_video_retirement_guard`;
