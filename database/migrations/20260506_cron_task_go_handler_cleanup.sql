-- Go cron task registry is the executable scheduler truth source.
-- The notification scheduler has migrated from the legacy PHP process class to
-- a versioned Asynq task type. Keep unmigrated rows as legacy provenance only.

UPDATE `cron_task`
SET `handler` = 'notification:dispatch-due:v1',
    `updated_at` = NOW()
WHERE `name` = 'notification_task_scheduler'
  AND `is_del` = 2;
