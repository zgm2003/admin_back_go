CREATE TEMPORARY TABLE `_ai_stopped_delivery_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_stopped_delivery_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `id`
  FROM `ai_reply_commands`
  WHERE `state` IN ('pending', 'claimed', 'running')
  UNION ALL
  SELECT attempt.`id`
  FROM `ai_provider_attempts` AS attempt
  JOIN `ai_reply_commands` AS command_row ON command_row.`id` = attempt.`command_id`
  WHERE attempt.`state` IN ('prepared', 'dispatched')
) AS active_work;

DELETE FROM `realtime_events` WHERE `event_type` = 'ai.response.canceled.v1';

ALTER TABLE `ai_reply_commands`
  ADD COLUMN `delivery_seq` INT UNSIGNED NOT NULL DEFAULT 0 AFTER `cancel_requested_at`,
  ADD COLUMN `stop_delivery_seq` INT UNSIGNED NULL AFTER `delivery_seq`;

UPDATE `ai_reply_commands` SET `stop_delivery_seq` = 0 WHERE `cancel_requested_at` IS NOT NULL;

ALTER TABLE `ai_reply_commands`
  ADD CONSTRAINT `chk_ai_reply_delivery_seq`
    CHECK ((`cancel_requested_at` IS NULL AND `stop_delivery_seq` IS NULL)
      OR (`cancel_requested_at` IS NOT NULL AND `stop_delivery_seq` IS NOT NULL
        AND `stop_delivery_seq` <= `delivery_seq`));

ALTER TABLE `ai_messages`
  ADD COLUMN `delivery_state` VARCHAR(16) NULL AFTER `reply_command_id`;

UPDATE `ai_messages` SET `delivery_state` = 'completed' WHERE `role` = 2;

ALTER TABLE `ai_messages`
  ADD CONSTRAINT `chk_ai_messages_delivery_state`
    CHECK ((`role` = 2 AND `delivery_state` IN ('completed', 'stopped'))
      OR (`role` <> 2 AND `delivery_state` IS NULL));

CREATE TABLE `ai_reply_delivery_chunks` (
  `command_id` BIGINT UNSIGNED NOT NULL,
  `delivery_seq` INT UNSIGNED NOT NULL,
  `delta` TEXT NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`command_id`, `delivery_seq`),
  CONSTRAINT `fk_ai_reply_delivery_chunks_command`
    FOREIGN KEY (`command_id`) REFERENCES `ai_reply_commands` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_reply_delivery_chunks_seq` CHECK (`delivery_seq` > 0),
  CONSTRAINT `chk_ai_reply_delivery_chunks_delta`
    CHECK (OCTET_LENGTH(`delta`) > 0 AND OCTET_LENGTH(`delta`) <= 16384)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

DROP TEMPORARY TABLE `_ai_stopped_delivery_guard`;
