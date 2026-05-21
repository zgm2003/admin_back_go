-- Make payment orders a product-visible payment menu again.
--
-- Payment orders are the expenditure/payment-order ledger under payment
-- management. They remain RBAC controlled, but the page itself must be visible
-- for roles that already have payment_order_list.

UPDATE `permissions`
SET `code` = 'payment_order_list',
    `show_menu` = 1,
    `sort` = 30,
    `status` = 1,
    `is_del` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `path` = '/payment/orders'
  AND `type` = 2;
