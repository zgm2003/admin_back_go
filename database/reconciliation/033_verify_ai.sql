SELECT 'ai_run_source_identity' AS invariant, COUNT(*) AS violations
FROM (
  SELECT r.`id`
  FROM `ai_runs` r
  LEFT JOIN (
    SELECT r.`id` run_id, 'chat' source_kind
    FROM `ai_runs` r JOIN `ai_messages` m ON m.`id`=r.`user_message_id`
    WHERE r.`user_message_id` IS NOT NULL
    UNION ALL
    SELECT r.`id`, 'image'
    FROM `ai_runs` r
    WHERE r.`request_id` REGEXP '^ai_image_task_[0-9]+$'
      AND (
        EXISTS (SELECT 1 FROM `ai_image_tasks` t WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED))
        OR EXISTS (SELECT 1 FROM `ai_image_files` f WHERE f.`task_id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED))
      )
    UNION ALL
    SELECT r.`id`, 'text'
    FROM `ai_runs` r
    WHERE r.`request_id` REGEXP '^ai_text_task_[0-9]+$'
      AND EXISTS (SELECT 1 FROM `ai_text_tasks` t WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_text_task_')+1) AS UNSIGNED))
    UNION ALL
    SELECT r.`id`, 'video'
    FROM `ai_runs` r
    WHERE r.`request_id` REGEXP '^canvas_video_task_[0-9]+$'
      AND (
        EXISTS (SELECT 1 FROM `canvas_video_tasks` t WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('canvas_video_task_')+1) AS UNSIGNED))
        OR EXISTS (SELECT 1 FROM `ai_video_tasks` t WHERE t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('canvas_video_task_')+1) AS UNSIGNED))
      )
    UNION ALL
    SELECT r.`id`, 'audio' FROM `ai_runs` r WHERE r.`request_id` REGEXP '^canvas_audio_[0-9]+$'
  ) source ON source.run_id=r.`id`
  GROUP BY r.`id`
  HAVING COUNT(source.source_kind)<>1
) bad;

SELECT 'ai_run_idempotency_unique' AS invariant, COUNT(*) AS violations
FROM (
  SELECT r.`id` entity FROM `ai_runs` r WHERE r.`idempotency_key` IS NULL OR r.`idempotency_key`=''
  UNION ALL
  SELECT MIN(r.`id`) FROM `ai_runs` r
  WHERE r.`idempotency_key` IS NOT NULL AND r.`idempotency_key`<>''
  GROUP BY r.`idempotency_key` HAVING COUNT(*)>1
) bad;

SELECT 'ai_chat_source_content' AS invariant, COUNT(*) AS violations
FROM `ai_runs` r JOIN `ai_messages` m ON m.`id`=r.`user_message_id`
WHERE r.`user_message_id` IS NOT NULL
  AND (r.`platform`<>'admin' OR BINARY r.`input_snapshot`<>BINARY m.`content`);

SELECT 'ai_image_source_target_counts' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 1 entity
  WHERE (SELECT COUNT(*) FROM `ai_image_tasks`)<>
        (SELECT COUNT(*) FROM `ai_runs` r JOIN `ai_image_tasks` t ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',t.`id`))
  UNION ALL
  SELECT t.`id` FROM `ai_image_tasks` t
  LEFT JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',t.`id`)
  WHERE r.`id` IS NULL
  UNION ALL
  SELECT MIN(t.`id`) FROM `ai_image_tasks` t
  JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',t.`id`)
  GROUP BY t.`id` HAVING COUNT(*)<>1
) bad;

SELECT 'ai_image_source_target_hash' AS invariant, COUNT(*) AS violations
FROM `ai_image_tasks` t
JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',t.`id`)
WHERE BINARY SHA2(CONCAT_WS(CHAR(31),t.`platform`,t.`user_id`,t.`agent_id`,t.`provider_id_snapshot`,t.`model_id_snapshot`,t.`prompt`),256)
   <> BINARY SHA2(CONCAT_WS(CHAR(31),r.`platform`,r.`user_id`,r.`agent_id`,r.`provider_id`,r.`model_id`,r.`input_snapshot`),256);

SELECT 'retired_image_evidence_complete' AS invariant, COUNT(*) AS violations
FROM (
  SELECT f.`id` entity
  FROM `ai_image_files` f
  LEFT JOIN `ai_image_tasks` t ON t.`id`=f.`task_id`
  LEFT JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',f.`task_id`)
  WHERE t.`id` IS NULL AND r.`id` IS NULL
  UNION ALL
  SELECT r.`id`
  FROM `ai_runs` r
  LEFT JOIN `ai_image_tasks` t ON t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED)
  LEFT JOIN `ai_image_files` f ON f.`task_id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED)
  WHERE r.`request_id` REGEXP '^ai_image_task_[0-9]+$' AND t.`id` IS NULL AND f.`id` IS NULL
  UNION ALL
  SELECT MIN(r.`id`)
  FROM `ai_runs` r
  LEFT JOIN `ai_image_tasks` t ON t.`id`=CAST(SUBSTRING(r.`request_id`, LENGTH('ai_image_task_')+1) AS UNSIGNED)
  WHERE r.`request_id` REGEXP '^ai_image_task_[0-9]+$' AND t.`id` IS NULL
  GROUP BY r.`request_id` HAVING COUNT(*)<>1
) bad;

SELECT 'ai_image_file_relationships' AS invariant, COUNT(*) AS violations
FROM `ai_image_files` f
JOIN `ai_image_files` related ON related.`id`=f.`related_file_id`
WHERE f.`related_file_id` IS NOT NULL AND f.`task_id`<>related.`task_id`;

SELECT 'ai_text_source_target_hash' AS invariant, COUNT(*) AS violations
FROM (
  SELECT t.`id` entity
  FROM `ai_text_tasks` t LEFT JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_text_task_',t.`id`)
  WHERE r.`id` IS NULL OR BINARY SHA2(CONCAT_WS(CHAR(31),t.`platform`,t.`user_id`,t.`agent_id`,t.`provider_id`,t.`model_id`,t.`prompt`),256)
                     <> BINARY SHA2(CONCAT_WS(CHAR(31),r.`platform`,r.`user_id`,r.`agent_id`,r.`provider_id`,r.`model_id`,r.`input_snapshot`),256)
  UNION ALL
  SELECT MIN(t.`id`) FROM `ai_text_tasks` t
  JOIN `ai_runs` r ON BINARY r.`request_id`=BINARY CONCAT('ai_text_task_',t.`id`)
  GROUP BY t.`id` HAVING COUNT(*)<>1
) bad;

SELECT 'ai_video_source_target_hash' AS invariant, COUNT(*) AS violations
FROM (
  SELECT source.`id` entity
  FROM `canvas_video_tasks` source LEFT JOIN `ai_video_tasks` target ON target.`id`=source.`id`
  WHERE target.`id` IS NULL OR BINARY SHA2(CONCAT_WS(CHAR(31),'canvas',source.`user_id`,source.`agent_id`,source.`provider_id`,source.`model_id`,source.`prompt`,source.`duration_seconds`,source.`size`,source.`resolution_name`,source.`provider_task_id`,source.`run_id`,source.`status`,source.`error_message`,source.`is_del`),256)
                            <> BINARY SHA2(CONCAT_WS(CHAR(31),target.`platform`,target.`user_id`,target.`agent_id`,target.`provider_id`,target.`model_id`,target.`prompt`,target.`duration_seconds`,target.`size`,target.`resolution_name`,target.`provider_task_id`,target.`run_id`,target.`status`,target.`error_message`,target.`is_del`),256)
  UNION ALL
  SELECT target.`id` FROM `ai_video_tasks` target LEFT JOIN `canvas_video_tasks` source ON source.`id`=target.`id`
  WHERE target.`platform`='canvas' AND source.`id` IS NULL
) bad;

SELECT 'ai_asset_owner_relationships' AS invariant, COUNT(*) AS violations
FROM `ai_assets` a LEFT JOIN `users` u ON u.`id`=a.`user_id`
WHERE a.`is_del`=2 AND a.`user_id`<>0 AND u.`id` IS NULL;
