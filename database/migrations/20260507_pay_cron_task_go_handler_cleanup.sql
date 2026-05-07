-- Payment cron runtime has migrated from legacy PHP process classes to Go
-- registry entries and versioned Asynq task types.
--
-- Keep cron_task.name/cron/status as the DB scheduling truth. The handler
-- column is display/provenance only; for registered Go tasks it must show the
-- executable task type instead of a PHP class string.

UPDATE `cron_task`
SET `handler` = 'pay:close-expired-order:v1',
    `updated_at` = NOW()
WHERE `name` = 'pay_close_expired_order'
  AND `is_del` = 2;

UPDATE `cron_task`
SET `handler` = 'pay:sync-pending-transaction:v1',
    `updated_at` = NOW()
WHERE `name` = 'pay_sync_pending_transaction'
  AND `is_del` = 2;
