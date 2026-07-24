-- P02 deterministic core backfill. No ownership, money, or object key is guessed.

DROP TEMPORARY TABLE IF EXISTS `p02_wallet_totals`;
CREATE TEMPORARY TABLE `p02_wallet_totals` AS
SELECT
  w.`id` AS `wallet_id`,
  w.`balance_cents` AS `stored_balance_cents`,
  w.`total_recharge_cents` AS `stored_recharge_cents`,
  COALESCE(SUM(CASE WHEN t.`direction`='in' THEN t.`amount_cents` ELSE -t.`amount_cents` END), 0) AS `ledger_balance_cents`,
  COALESCE(SUM(CASE WHEN t.`direction`='in' AND t.`source_type` IN ('recharge','redeem_code') THEN t.`amount_cents` ELSE 0 END), 0) AS `ledger_recharge_cents`,
  COALESCE(SUM(CASE WHEN t.`direction`='out' THEN t.`amount_cents` ELSE 0 END), 0) AS `ledger_consume_cents`
FROM `user_wallets` w
LEFT JOIN `wallet_transactions` t ON t.`wallet_id`=w.`id` AND t.`is_del`=2
WHERE w.`is_del`=2
GROUP BY w.`id`, w.`balance_cents`, w.`total_recharge_cents`;

DROP PROCEDURE IF EXISTS `p02_assert_core_money`;
DELIMITER //
CREATE PROCEDURE `p02_assert_core_money`()
BEGIN
  IF EXISTS (
    SELECT 1 FROM `p02_wallet_totals`
    WHERE `stored_balance_cents`<>`ledger_balance_cents`
       OR `stored_recharge_cents`<>`ledger_recharge_cents`
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='wallet balance or recharge total differs from immutable ledger';
  END IF;
END//
DELIMITER ;
CALL `p02_assert_core_money`();
DROP PROCEDURE `p02_assert_core_money`;

DROP TEMPORARY TABLE IF EXISTS `p02_enabled_cos`;
CREATE TEMPORARY TABLE `p02_enabled_cos` (
  `bucket_domain` VARCHAR(255) NOT NULL,
  PRIMARY KEY (`bucket_domain`)
);
INSERT INTO `p02_enabled_cos` (`bucket_domain`)
SELECT LOWER(TRIM(d.`bucket_domain`))
FROM `upload_setting` s
JOIN `upload_driver` d ON d.`id`=s.`driver_id` AND d.`is_del`=2 AND d.`driver`='cos'
JOIN `upload_rule` r ON r.`id`=s.`rule_id` AND r.`is_del`=2
WHERE s.`status`=1 AND s.`is_del`=2 AND TRIM(d.`bucket_domain`)<>''
ORDER BY s.`id` DESC
LIMIT 1;

DROP TEMPORARY TABLE IF EXISTS `p02_notification_matches`;
CREATE TEMPORARY TABLE `p02_notification_matches` (
  `notification_id` BIGINT NOT NULL,
  `task_id` BIGINT NOT NULL,
  `candidate_count` BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (`notification_id`)
);
INSERT INTO `p02_notification_matches` (`notification_id`, `task_id`, `candidate_count`)
SELECT n.`id`, MIN(t.`id`), COUNT(*)
FROM `notifications` n
JOIN `notification_task` t
  ON t.`title`=n.`title`
 AND t.`content`=n.`content`
 AND t.`type`=n.`type`
 AND t.`level`=n.`level`
 AND t.`link`=n.`link`
 AND t.`platform`=n.`platform`
 AND n.`created_at`>=t.`created_at`
 AND n.`created_at`<=t.`updated_at`
WHERE n.`source_task_id` IS NULL
GROUP BY n.`id`;

SET @p02_channel_ttl := (
  SELECT CASE
    WHEN s.`value_type`=2
      AND s.`status`=1
      AND s.`is_del`=2
      AND TRIM(s.`setting_value`) REGEXP '^[0-9]+$'
      AND CAST(TRIM(s.`setting_value`) AS UNSIGNED) BETWEEN 1 AND 60
    THEN CAST(TRIM(s.`setting_value`) AS UNSIGNED)
    ELSE 5
  END
  FROM `system_settings` s
  WHERE s.`setting_key`='auth.verify_code.ttl_minutes'
  ORDER BY s.`id`
  LIMIT 1
);
SET @p02_channel_ttl := COALESCE(@p02_channel_ttl, 5);

START TRANSACTION;

UPDATE `export_tasks`
SET `platform` = 'admin', `kind` = 'user_list'
WHERE `platform` IS NULL OR `platform`<>'admin' OR `kind` IS NULL OR `kind`<>'user_list';

UPDATE `export_tasks` e
JOIN `p02_enabled_cos` c
  ON LOWER(SUBSTRING_INDEX(SUBSTRING_INDEX(SUBSTRING_INDEX(SUBSTRING_INDEX(TRIM(e.`file_url`),'//',-1),'/',1),'?',1),'#',1))=c.`bucket_domain`
SET e.`object_key`=SUBSTRING_INDEX(
  SUBSTRING_INDEX(REGEXP_REPLACE(TRIM(e.`file_url`), '^https?://[^/?#]+/?', '', 1, 1, 'i'), '?', 1),
  '#',
  1
)
WHERE COALESCE(e.`object_key`,'')=''
  AND COALESCE(e.`file_url`,'')<>''
  AND REGEXP_LIKE(TRIM(e.`file_url`), '^https?://', 'i')
  AND SUBSTRING_INDEX(SUBSTRING_INDEX(REGEXP_REPLACE(TRIM(e.`file_url`), '^https?://[^/?#]+/?', '', 1, 1, 'i'), '?', 1), '#', 1)<>'';

INSERT INTO `authz_principal_versions` (`user_id`, `platform`, `version`, `updated_at`)
SELECT u.`id`, 'admin', 1, UTC_TIMESTAMP(6)
FROM `users` u
WHERE u.`status`=1 AND u.`is_del`=2
ON DUPLICATE KEY UPDATE `user_id`=`user_id`;

UPDATE `mail_configs`
SET `verify_code_ttl_minutes`=@p02_channel_ttl, `updated_at`=CURRENT_TIMESTAMP
WHERE `is_del`=2 AND (`verify_code_ttl_minutes`<1 OR `verify_code_ttl_minutes`>60);

UPDATE `sms_configs`
SET `verify_code_ttl_minutes`=@p02_channel_ttl, `updated_at`=CURRENT_TIMESTAMP
WHERE `is_del`=2 AND (`verify_code_ttl_minutes`<1 OR `verify_code_ttl_minutes`>60);

UPDATE `notifications` n
JOIN `p02_notification_matches` m ON m.`notification_id`=n.`id`
SET n.`source_task_id`=m.`task_id`
WHERE n.`source_task_id` IS NULL AND m.candidate_count = 1;

UPDATE `user_wallets` w
JOIN `p02_wallet_totals` totals ON totals.`wallet_id`=w.`id`
SET w.`total_consume_cents`=totals.`ledger_consume_cents`
WHERE w.`total_consume_cents`<>totals.`ledger_consume_cents`;

COMMIT;

DROP TEMPORARY TABLE IF EXISTS `p02_notification_matches`;
DROP TEMPORARY TABLE IF EXISTS `p02_enabled_cos`;
DROP TEMPORARY TABLE IF EXISTS `p02_wallet_totals`;
