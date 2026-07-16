-- Non-destructive retention policy for AI image tasks and their task-owned files.
SET @ddl = IF(
  EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'ai_image_tasks' AND column_name = 'is_del'
  ),
  'SELECT 1',
  'ALTER TABLE `ai_image_tasks` ADD COLUMN `is_del` TINYINT NOT NULL DEFAULT 2 AFTER `is_favorite`'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @ddl = IF(
  EXISTS(
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'ai_image_files' AND column_name = 'is_del'
  ),
  'SELECT 1',
  'ALTER TABLE `ai_image_files` ADD COLUMN `is_del` TINYINT NOT NULL DEFAULT 2 AFTER `revised_prompt`'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
