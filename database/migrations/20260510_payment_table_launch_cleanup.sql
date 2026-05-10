-- Production launch cleanup for payment-domain table data.
--
-- Runtime truth after the payment rebuild:
-- - payment_channels
-- - payment_channel_configs
-- - payment_orders
-- - payment_events
--
-- The tables below are retired legacy/backups or old recharge-wallet order
-- prototypes. Active Go/Vue runtime no longer references them; keep recovery in
-- the external mysqldump backup, not in production schema.

DROP TABLE IF EXISTS `wallet_transactions_legacy_20260508`;
DROP TABLE IF EXISTS `user_wallets_legacy_20260508`;
DROP TABLE IF EXISTS `pay_refunds_legacy_20260508`;
DROP TABLE IF EXISTS `pay_reconcile_tasks_legacy_20260508`;
DROP TABLE IF EXISTS `pay_notify_logs_legacy_20260508`;
DROP TABLE IF EXISTS `pay_transactions_legacy_20260508`;
DROP TABLE IF EXISTS `pay_channel_legacy_20260508`;
DROP TABLE IF EXISTS `order_fulfillments_legacy_20260508`;
DROP TABLE IF EXISTS `order_items`;
DROP TABLE IF EXISTS `orders`;
