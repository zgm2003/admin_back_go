ALTER TABLE `ai_image_tasks`
  ADD COLUMN `lease_owner` VARCHAR(128) NULL AFTER `status`,
  ADD COLUMN `lease_token` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `lease_owner`,
  ADD COLUMN `lease_expires_at` DATETIME(6) NULL AFTER `lease_token`,
  ADD KEY `idx_ai_image_tasks_lease` (`status`, `lease_expires_at`, `id`),
  ADD CONSTRAINT `chk_ai_image_tasks_lease`
    CHECK ((`lease_owner` IS NULL AND `lease_expires_at` IS NULL)
      OR (`lease_owner` IS NOT NULL AND `lease_token` > 0 AND `lease_expires_at` IS NOT NULL));
