-- Add message metadata for the chat UI contract restored from the old admin
-- chat page: image attachments and per-request runtime parameters.

SET @schema_name := DATABASE();

SET @add_ai_messages_meta_json := (
  SELECT IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = @schema_name AND table_name = 'ai_messages' AND column_name = 'meta_json'),
    'SELECT 1',
    'ALTER TABLE `ai_messages` ADD COLUMN `meta_json` JSON NULL COMMENT ''消息扩展元数据：attachments/runtime_params/blocks/feedback'' AFTER `content`'
  )
);
PREPARE stmt FROM @add_ai_messages_meta_json;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
