DROP TEMPORARY TABLE IF EXISTS `p02_ai_run_sources`;
CREATE TEMPORARY TABLE `p02_ai_run_sources` (
  `run_id` BIGINT NOT NULL,
  `source_kind` VARCHAR(32) NOT NULL,
  `source_identity` VARCHAR(191) NOT NULL,
  PRIMARY KEY (`run_id`, `source_kind`)
);

INSERT INTO `p02_ai_run_sources` (`run_id`, `source_kind`, `source_identity`)
SELECT r.`id`, 'chat', CAST(r.`user_message_id` AS CHAR)
FROM `ai_runs` r JOIN `ai_messages` m ON m.`id`=r.`user_message_id`
WHERE r.`user_message_id` IS NOT NULL;

INSERT INTO `p02_ai_run_sources` (`run_id`, `source_kind`, `source_identity`)
SELECT r.`id`, 'image', SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1)
FROM `ai_runs` r
WHERE r.`request_id` REGEXP '^ai_image_task_[0-9]+$'
  AND (
    EXISTS (
      SELECT 1 FROM `ai_image_tasks` t
      WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED)
    )
    OR EXISTS (
      SELECT 1 FROM `ai_image_files` f
      WHERE f.`task_id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED)
    )
  );

INSERT INTO `p02_ai_run_sources` (`run_id`, `source_kind`, `source_identity`)
SELECT r.`id`, 'text', SUBSTRING(r.`request_id`, LENGTH('ai_text_task_')+1)
FROM `ai_runs` r
WHERE r.`request_id` REGEXP '^ai_text_task_[0-9]+$'
  AND EXISTS (
    SELECT 1 FROM `ai_text_tasks` t
    WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_text_task_')+1) AS UNSIGNED)
  );

INSERT INTO `p02_ai_run_sources` (`run_id`, `source_kind`, `source_identity`)
SELECT r.`id`, 'video', SUBSTRING(r.`request_id`, LENGTH('canvas_video_task_')+1)
FROM `ai_runs` r
WHERE r.`request_id` REGEXP '^canvas_video_task_[0-9]+$'
  AND (
    EXISTS (
      SELECT 1 FROM `canvas_video_tasks` t
      WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('canvas_video_task_')+1) AS UNSIGNED)
    )
    OR EXISTS (
      SELECT 1 FROM `ai_video_tasks` t
      WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('canvas_video_task_')+1) AS UNSIGNED)
    )
  );

INSERT INTO `p02_ai_run_sources` (`run_id`, `source_kind`, `source_identity`)
SELECT r.`id`, 'audio', r.`request_id`
FROM `ai_runs` r
WHERE r.`request_id` REGEXP '^canvas_audio_[0-9]+$';

DROP PROCEDURE IF EXISTS `p02_assert_ai_sources`;
DELIMITER //
CREATE PROCEDURE `p02_assert_ai_sources`()
BEGIN
  IF EXISTS (
    SELECT r.`id`
    FROM `ai_runs` r
    LEFT JOIN `p02_ai_run_sources` source ON source.`run_id`=r.`id`
    GROUP BY r.`id`
    HAVING COUNT(source.`source_kind`)<>1
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='AI run does not have exactly one evidence source';
  END IF;
END//
DELIMITER ;
CALL `p02_assert_ai_sources`();
DROP PROCEDURE `p02_assert_ai_sources`;

START TRANSACTION;

UPDATE `ai_runs` r
JOIN `ai_messages` m ON m.`id`=r.`user_message_id`
SET r.`platform`='admin', r.`input_snapshot`=m.`content`
WHERE r.`user_message_id` IS NOT NULL
  AND (r.`platform` IS NULL OR r.`platform`='' OR r.`input_snapshot` IS NULL OR r.`input_snapshot`='');

UPDATE `ai_runs` r
JOIN `ai_image_tasks` t ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',t.`id`)
SET r.`platform`=t.`platform`, r.`input_snapshot`=t.`prompt`
WHERE r.`platform` IS NULL OR r.`platform`='' OR r.`input_snapshot` IS NULL OR r.`input_snapshot`='';

UPDATE `ai_runs` r
JOIN `ai_text_tasks` t ON BINARY r.`request_id`=BINARY CONCAT('ai_text_task_',t.`id`)
SET r.`platform`=t.`platform`, r.`input_snapshot`=t.`prompt`
WHERE r.`platform` IS NULL OR r.`platform`='' OR r.`input_snapshot` IS NULL OR r.`input_snapshot`='';

UPDATE `ai_runs`
SET `idempotency_key`=CONCAT('legacy:ai-run:',`id`)
WHERE `idempotency_key` IS NULL OR `idempotency_key`='';

UPDATE `ai_image_tasks` t
JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',t.`id`)
SET t.`platform`=r.`platform`
WHERE t.`platform` IS NULL OR t.`platform`='';

UPDATE `ai_text_tasks` t
JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_text_task_',t.`id`)
SET t.`platform`=r.`platform`
WHERE t.`platform` IS NULL OR t.`platform`='';

INSERT INTO `ai_video_tasks` (
  `id`, `platform`, `user_id`, `agent_id`, `provider_id`, `model_id`, `prompt`,
  `duration_seconds`, `size`, `resolution_name`, `provider_task_id`, `run_id`,
  `status`, `error_message`, `is_del`, `created_at`, `updated_at`, `finished_at`
)
SELECT
  source.`id`, 'canvas', source.`user_id`, source.`agent_id`, source.`provider_id`,
  source.`model_id`, source.`prompt`, source.`duration_seconds`, source.`size`,
  source.`resolution_name`, source.`provider_task_id`, source.`run_id`, source.`status`,
  source.`error_message`, source.`is_del`, source.`created_at`, source.`updated_at`, source.`finished_at`
FROM `canvas_video_tasks` source
WHERE NOT EXISTS (SELECT 1 FROM `ai_video_tasks` target WHERE target.`id`=source.`id`);

COMMIT;

DROP TEMPORARY TABLE IF EXISTS `p02_ai_run_sources`;
