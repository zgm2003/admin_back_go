-- Grant product-facing recharge permissions to active admin roles.
--
-- The recharge cashier is a user-facing payment entry. Payment order
-- permissions remain separately controlled by payment_order_*; this migration
-- only ensures active roles can open /payment/recharge and use its row actions.

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT r.`id`, p.`id`, 2
FROM `roles` r
JOIN `permissions` p ON p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND p.`code` IN (
    'payment_recharge_list',
    'payment_recharge_add',
    'payment_recharge_pay',
    'payment_recharge_sync',
    'payment_recharge_close'
  )
WHERE r.`is_del` = 2
ON DUPLICATE KEY UPDATE
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;
