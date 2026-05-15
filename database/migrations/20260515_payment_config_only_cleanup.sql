-- Payment config only cleanup.
-- After 20260515_payment_config_rebuild_v1.sql copies Alipay config into
-- payment_alipay_configs, the old channel/order/event payment runtime is no
-- longer an active source of truth.

DELETE rp
FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
WHERE p.`platform` = 'admin'
  AND (
    p.`code` LIKE 'payment_channel_%'
    OR p.`code` LIKE 'payment_order_%'
    OR p.`code` LIKE 'payment_event_%'
    OR p.`path` IN ('/payment/channel', '/payment/order', '/payment/event')
  );

DELETE FROM `permissions`
WHERE `platform` = 'admin'
  AND (
    `code` LIKE 'payment_channel_%'
    OR `code` LIKE 'payment_order_%'
    OR `code` LIKE 'payment_event_%'
    OR `path` IN ('/payment/channel', '/payment/order', '/payment/event')
  );

DELETE FROM `cron_task_log`
WHERE `task_id` IN (
  SELECT `id`
  FROM `cron_task`
  WHERE `name` IN ('payment_close_expired_order', 'payment_sync_pending_order')
     OR `handler` LIKE 'payment:%'
);

DELETE FROM `cron_task`
WHERE `name` IN ('payment_close_expired_order', 'payment_sync_pending_order')
   OR `handler` LIKE 'payment:%';

DROP TABLE IF EXISTS `payment_events`;
DROP TABLE IF EXISTS `payment_orders`;
DROP TABLE IF EXISTS `payment_channel_configs`;
DROP TABLE IF EXISTS `payment_channels`;
