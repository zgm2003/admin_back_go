-- Go queue monitor uses Asynq lane/env config plus official asynqmon.
-- The old PHP queue monitor JSON setting must not remain editable through
-- system-settings, but the row is soft-deleted for rollback visibility.

UPDATE `system_settings`
SET `is_del` = 1,
    `updated_at` = NOW()
WHERE `setting_key` = 'devtools_queue_monitor_queues'
  AND `is_del` = 2;
