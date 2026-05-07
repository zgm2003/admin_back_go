-- Payment fulfillment retry now maps to a real Go registry task type.
--
-- This replaces the legacy PHP process class string for the active cron row.

UPDATE `cron_task`
SET `handler` = 'pay:fulfillment-retry:v1',
    `updated_at` = NOW()
WHERE `name` = 'pay_fulfillment_retry'
  AND `is_del` = 2;
