-- Payment recharge/order closed-state consistency cleanup.
--
-- A closed payment order means the linked recharge can no longer be paid or
-- credited through the Alipay flow. Keep stale local paying/pending/failed
-- recharge rows from being auto-synced repeatedly when the cashier page opens.

UPDATE `payment_recharges` AS r
JOIN `payment_orders` AS o
  ON o.`id` = r.`payment_order_id`
SET r.`status` = 'closed',
    r.`updated_at` = CURRENT_TIMESTAMP
WHERE r.`status` IN ('pending', 'paying', 'failed')
  AND r.`is_del` = 2
  AND o.`status` = 'closed'
  AND o.`is_del` = 2;
