UPDATE `cron_task`
SET `status` = 2,
    `is_del` = 1,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `name` = 'clean_expired_contact_request'
  AND `is_del` = 2;

UPDATE `cron_task`
SET `handler` = CASE `name`
    WHEN 'notification_task_scheduler' THEN 'notification:dispatch-due:v1'
    WHEN 'ai_run_timeout' THEN 'ai:run-timeout:v1'
    WHEN 'payment_sync_pending_order' THEN 'payment:sync-pending-order:v1'
    WHEN 'payment_close_expired_order' THEN 'payment:close-expired-order:v1'
    ELSE `handler`
  END,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `name` IN (
    'notification_task_scheduler',
    'ai_run_timeout',
    'payment_sync_pending_order',
    'payment_close_expired_order'
  )
  AND `is_del` = 2;
