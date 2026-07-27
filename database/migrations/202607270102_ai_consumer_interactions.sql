-- Extend only the existing consumer interaction facts. The temporary guard
-- fails before persistent DDL when the base shape or a target name collides.
DROP TEMPORARY TABLE IF EXISTS `_ai_consumer_interactions_guard`;
CREATE TEMPORARY TABLE `_ai_consumer_interactions_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_consumer_interactions_guard`
SELECT IF(COUNT(*) = 3, 0, 1)
FROM `information_schema`.`TABLES`
WHERE `TABLE_SCHEMA` = DATABASE()
  AND `TABLE_TYPE` = 'BASE TABLE'
  AND `TABLE_NAME` IN ('ai_conversations', 'ai_messages', 'ai_runs');

INSERT INTO `_ai_consumer_interactions_guard`
SELECT IF(COUNT(*) = 6, 0, 1)
FROM `information_schema`.`COLUMNS`
WHERE `TABLE_SCHEMA` = DATABASE()
  AND (
    (`TABLE_NAME` = 'ai_conversations' AND `COLUMN_NAME` = 'last_message_at')
    OR (`TABLE_NAME` = 'ai_messages' AND `COLUMN_NAME` IN ('conversation_id', 'is_del', 'role', 'id'))
    OR (`TABLE_NAME` = 'ai_runs' AND `COLUMN_NAME` = 'finished_at')
  );

INSERT INTO `_ai_consumer_interactions_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`COLUMNS`
WHERE `TABLE_SCHEMA` = DATABASE()
  AND (
    (`TABLE_NAME` = 'ai_conversations' AND `COLUMN_NAME` = 'last_read_message_id')
    OR (`TABLE_NAME` = 'ai_runs' AND `COLUMN_NAME` = 'liked_at')
  );

INSERT INTO `_ai_consumer_interactions_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`STATISTICS`
WHERE `TABLE_SCHEMA` = DATABASE()
  AND `TABLE_NAME` = 'ai_messages'
  AND `INDEX_NAME` = 'idx_ai_messages_conversation_del_role_id';

INSERT INTO `_ai_consumer_interactions_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `INDEX_NAME`
  FROM `information_schema`.`STATISTICS`
  WHERE `TABLE_SCHEMA` = DATABASE()
    AND `TABLE_NAME` = 'ai_messages'
  GROUP BY `INDEX_NAME`
  HAVING GROUP_CONCAT(`COLUMN_NAME` ORDER BY `SEQ_IN_INDEX` SEPARATOR ',') =
    'conversation_id,is_del,role,id'
) AS `existing_message_index`;

ALTER TABLE `ai_conversations`
  ADD COLUMN `last_read_message_id` BIGINT UNSIGNED NOT NULL DEFAULT 0
    AFTER `last_message_at`;

ALTER TABLE `ai_runs`
  ADD COLUMN `liked_at` DATETIME(6) NULL AFTER `finished_at`;

ALTER TABLE `ai_messages`
  ADD KEY `idx_ai_messages_conversation_del_role_id` (`conversation_id`, `is_del`, `role`, `id`);

DROP TEMPORARY TABLE `_ai_consumer_interactions_guard`;
