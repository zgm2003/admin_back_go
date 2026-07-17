-- Add an idempotent publication identity for assistant messages produced by a
-- durable reply command. Historical messages remain NULL.
ALTER TABLE `ai_messages`
  ADD COLUMN `reply_command_id` BIGINT UNSIGNED NULL AFTER `meta_json`,
  ADD UNIQUE INDEX `uk_ai_messages_reply_command` (`reply_command_id`);
