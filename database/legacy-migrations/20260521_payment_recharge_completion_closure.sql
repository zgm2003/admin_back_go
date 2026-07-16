-- Payment recharge completion closure.
--
-- Adds the public Alipay callback audit table and re-enables the two Go-owned
-- payment compensation cron tasks. The business source of truth remains
-- payment_orders / payment_recharges / wallet_transactions; callback events are
-- audit-only.

CREATE TABLE IF NOT EXISTS `payment_callback_events` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `provider` VARCHAR(32) NOT NULL DEFAULT 'alipay',
  `notify_id` VARCHAR(128) NOT NULL DEFAULT '',
  `out_trade_no` VARCHAR(64) NOT NULL DEFAULT '',
  `trade_no` VARCHAR(64) NOT NULL DEFAULT '',
  `trade_status` VARCHAR(32) NOT NULL DEFAULT '',
  `app_id` VARCHAR(64) NOT NULL DEFAULT '',
  `total_amount_cents` BIGINT NOT NULL DEFAULT 0,
  `signature_valid` TINYINT NOT NULL DEFAULT 2,
  `process_status` VARCHAR(16) NOT NULL DEFAULT 'pending',
  `process_message` VARCHAR(512) NOT NULL DEFAULT '',
  `raw_payload_json` JSON NULL,
  `received_at` DATETIME NOT NULL,
  `processed_at` DATETIME NULL,
  `is_del` TINYINT NOT NULL DEFAULT 2,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_payment_callback_events_notify_id` (`provider`, `notify_id`),
  KEY `idx_payment_callback_events_out_trade_no` (`provider`, `out_trade_no`),
  KEY `idx_payment_callback_events_status_time` (`process_status`, `received_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @payment_orders_has_provider_status_expired_idx := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'payment_orders'
    AND INDEX_NAME = 'idx_payment_orders_provider_status_expired'
);

SET @payment_orders_add_provider_status_expired_idx_sql := IF(
  @payment_orders_has_provider_status_expired_idx = 0,
  'CREATE INDEX `idx_payment_orders_provider_status_expired` ON `payment_orders` (`provider`, `status`, `is_del`, `expired_at`, `id`)',
  'SELECT 1'
);

PREPARE payment_orders_add_provider_status_expired_idx_stmt FROM @payment_orders_add_provider_status_expired_idx_sql;
EXECUTE payment_orders_add_provider_status_expired_idx_stmt;
DEALLOCATE PREPARE payment_orders_add_provider_status_expired_idx_stmt;

SET @payment_orders_has_status_updated_idx := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'payment_orders'
    AND INDEX_NAME = 'idx_payment_orders_status_updated'
);

SET @payment_orders_add_status_updated_idx_sql := IF(
  @payment_orders_has_status_updated_idx = 0,
  'CREATE INDEX `idx_payment_orders_status_updated` ON `payment_orders` (`status`, `is_del`, `updated_at`, `id`)',
  'SELECT 1'
);

PREPARE payment_orders_add_status_updated_idx_stmt FROM @payment_orders_add_status_updated_idx_sql;
EXECUTE payment_orders_add_status_updated_idx_stmt;
DEALLOCATE PREPARE payment_orders_add_status_updated_idx_stmt;

INSERT INTO `cron_task` (`name`, `title`, `description`, `cron`, `cron_readable`, `handler`, `status`, `is_del`)
VALUES
  ('payment_sync_pending_order', '支付中订单补偿同步', '扫描支付中支付宝订单并补偿同步本地订单/充值/钱包状态', '0 */2 * * * *', '每2分钟', 'payment:sync-pending-order:v1', 1, 2),
  ('payment_close_expired_order', '过期支付订单关闭', '扫描过期未支付支付宝订单并关闭本地/支付宝订单', '0 */5 * * * *', '每5分钟', 'payment:close-expired-order:v1', 1, 2)
ON DUPLICATE KEY UPDATE
  `title` = VALUES(`title`),
  `description` = VALUES(`description`),
  `cron` = VALUES(`cron`),
  `cron_readable` = VALUES(`cron_readable`),
  `handler` = VALUES(`handler`),
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;
