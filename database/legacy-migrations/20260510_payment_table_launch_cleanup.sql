-- Production launch cleanup for payment-domain table data.
--
-- Current active payment tables are payment_configs and payment_orders.
-- This historical cleanup predates the 20260515 config/order rebuild; do not
-- use it as the current payment runtime map.
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
