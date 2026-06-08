-- Remove redundant polymorphic source fields from ai_runs.
-- Canvas task tables own task identity; ai_runs is only a provider-attempt log.

ALTER TABLE `canvas_video_tasks`
  ADD COLUMN `run_id` BIGINT UNSIGNED NULL AFTER `provider_task_id`;

UPDATE `canvas_video_tasks` t
JOIN `ai_runs` r
  ON r.`source_type` = 'canvas_video_task'
 AND r.`source_id` = t.`id`
SET t.`run_id` = r.`id`
WHERE t.`run_id` IS NULL;

CREATE INDEX `idx_canvas_video_tasks_run_id` ON `canvas_video_tasks` (`run_id`);

ALTER TABLE `ai_runs`
  DROP INDEX `uk_ai_runs_source_request`,
  DROP INDEX `idx_ai_runs_platform_modality_created`,
  DROP INDEX `idx_ai_runs_source`;

ALTER TABLE `ai_runs`
  DROP CHECK `chk_ai_runs_usage_status`;

ALTER TABLE `ai_runs`
  DROP COLUMN `modality`,
  DROP COLUMN `source_type`,
  DROP COLUMN `source_id`,
  DROP COLUMN `usage_status`;
