-- Pay reconcile cron tasks now map to real Go registry task handlers.
--
-- cron_task.name/cron/status stay the DB scheduling truth. The handler column
-- records the executable Go task type for registered tasks; the scheduler never
-- executes PHP class strings.

UPDATE `cron_task`
SET `handler` = 'pay:reconcile-daily:v1',
    `updated_at` = NOW()
WHERE `name` = 'pay_reconcile_daily'
  AND `is_del` = 2;

UPDATE `cron_task`
SET `handler` = 'pay:reconcile-execute:v1',
    `updated_at` = NOW()
WHERE `name` = 'pay_reconcile_execute'
  AND `is_del` = 2;
