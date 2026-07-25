-- Invoke this destructive contract revision only after the deployed binary has
-- been verified to read/write wallet units exclusively in the maintenance window:
--   SET @ai_billing_units_only_verified = 1;
SET @ai_billing_contract_started_at = CURRENT_TIMESTAMP(6);

DROP TEMPORARY TABLE IF EXISTS `_ai_billing_contract_guard`;
CREATE TEMPORARY TABLE `_ai_billing_contract_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COALESCE(@ai_billing_units_only_verified, 0) = 1, 0, 1);

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `user_wallets`
WHERE `balance_units` IS NULL OR `total_recharge_units` IS NULL OR `total_consume_units` IS NULL OR `held_units` IS NULL
   OR `balance_units` <> `balance_cents` * 1000000
   OR `total_recharge_units` <> `total_recharge_cents` * 1000000
   OR `total_consume_units` <> `total_consume_cents` * 1000000
   OR `balance_units` < 0 OR `total_recharge_units` < 0 OR `total_consume_units` < 0 OR `held_units` < 0
   OR `held_units` > `balance_units`;

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `wallet_transactions`
WHERE `amount_units` IS NULL OR `balance_before_units` IS NULL OR `balance_after_units` IS NULL
   OR `amount_units` <> `amount_cents` * 1000000
   OR `balance_before_units` <> `balance_before_cents` * 1000000
   OR `balance_after_units` <> `balance_after_cents` * 1000000
   OR `amount_units` < 0 OR `balance_before_units` < 0 OR `balance_after_units` < 0;

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `user_wallets` AS wallet
LEFT JOIN (
  SELECT `wallet_id`, SUM(`held_units`) AS `held_units`
  FROM `wallet_holds`
  WHERE `status` = 'active'
  GROUP BY `wallet_id`
) AS active_hold ON active_hold.`wallet_id` = wallet.`id`
WHERE wallet.`held_units` <> COALESCE(active_hold.`held_units`, 0);

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_runs`
WHERE `request_fingerprint` IS NULL OR OCTET_LENGTH(`request_fingerprint`) <> 32
   OR `pricing_snapshot_json` IS NULL OR JSON_VALID(`pricing_snapshot_json`) = 0
   OR `billing_status` IS NULL OR `billing_reason` IS NULL
   OR (`created_at` >= @ai_billing_contract_started_at AND `pricing_snapshot_json` = '{"version":"legacy_unpriced_v1","billable":false}');

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `id` FROM `ai_reply_commands` WHERE `request_fingerprint` IS NULL OR OCTET_LENGTH(`request_fingerprint`) <> 32
  UNION ALL
  SELECT `id` FROM `ai_provider_attempts` WHERE `run_id` IS NULL OR `run_id` = 0 OR `prepared_request_json` IS NULL OR `prepared_request_sha256` IS NULL OR `quote_json` IS NULL OR `usage_json` IS NULL OR `usage_status` IS NULL
  UNION ALL
  SELECT `id` FROM `ai_text_tasks` WHERE `request_id` IS NULL OR `request_id` = '' OR `request_fingerprint` IS NULL OR `run_id` IS NULL OR `run_id` = 0 OR `kind` IS NULL OR `last_error_code` IS NULL
  UNION ALL
  SELECT `id` FROM `ai_image_tasks` WHERE `request_id` IS NULL OR `request_id` = '' OR `request_fingerprint` IS NULL OR `run_id` IS NULL OR `run_id` = 0 OR `last_error_code` IS NULL
  UNION ALL
  SELECT `id` FROM `ai_video_tasks` WHERE `request_id` IS NULL OR `request_id` = '' OR `request_fingerprint` IS NULL OR `run_id` IS NULL OR `run_id` = 0 OR `last_error_code` IS NULL OR `storage_provider` IS NULL OR `storage_key` IS NULL OR `content_type` IS NULL
) AS incomplete_billing_identity;

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `user_id`, BINARY `request_id` AS `request_id` FROM `ai_runs` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
  UNION ALL SELECT `user_id`, BINARY `request_id` FROM `ai_reply_commands` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
  UNION ALL SELECT `user_id`, BINARY `request_id` FROM `ai_text_tasks` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
  UNION ALL SELECT `user_id`, BINARY `request_id` FROM `ai_image_tasks` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
  UNION ALL SELECT `user_id`, BINARY `request_id` FROM `ai_video_tasks` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
) AS duplicate_canonical_identity;

INSERT INTO `_ai_billing_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT task.`id` FROM `ai_text_tasks` AS task LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id` WHERE run_row.`id` IS NULL
  UNION ALL SELECT task.`id` FROM `ai_image_tasks` AS task LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id` WHERE run_row.`id` IS NULL
  UNION ALL SELECT task.`id` FROM `ai_video_tasks` AS task LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id` WHERE run_row.`id` IS NULL
  UNION ALL SELECT attempt.`id` FROM `ai_provider_attempts` AS attempt LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = attempt.`run_id` WHERE run_row.`id` IS NULL
) AS orphan_run_owners;

ALTER TABLE `ai_runs`
  MODIFY COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  MODIFY COLUMN `request_fingerprint` BINARY(32) NOT NULL,
  MODIFY COLUMN `pricing_snapshot_json` MEDIUMTEXT NOT NULL,
  MODIFY COLUMN `billing_status` VARCHAR(16) NOT NULL,
  MODIFY COLUMN `billing_reason` VARCHAR(32) NOT NULL,
  DROP INDEX `uk_ai_runs_conversation_request`,
  ADD UNIQUE KEY `uk_ai_runs_user_request` (`user_id`, `request_id`),
  DROP CHECK `chk_ai_runs_status`,
  ADD CONSTRAINT `chk_ai_runs_status` CHECK (`status` IN ('running', 'success', 'failed', 'canceled', 'timeout', 'outcome_unknown')),
  ADD CONSTRAINT `chk_ai_runs_billing_status` CHECK (`billing_status` IN ('pending', 'held', 'settled', 'released', 'unbilled')),
  ADD CONSTRAINT `chk_ai_runs_billing_reason` CHECK (`billing_reason` IN ('pending', 'held', 'settled_complete_usage', 'released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown', 'unbilled_usage_incomplete', 'unbilled_over_hold', 'legacy_unpriced'));

ALTER TABLE `ai_reply_commands`
  MODIFY COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  MODIFY COLUMN `request_fingerprint` BINARY(32) NOT NULL,
  DROP INDEX `uk_ai_reply_request`,
  ADD UNIQUE KEY `uk_ai_reply_user_request` (`user_id`, `request_id`);

ALTER TABLE `ai_provider_attempts`
  MODIFY COLUMN `run_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `prepared_request_json` MEDIUMTEXT NOT NULL,
  MODIFY COLUMN `prepared_request_sha256` BINARY(32) NOT NULL,
  MODIFY COLUMN `quote_json` MEDIUMTEXT NOT NULL,
  MODIFY COLUMN `usage_json` MEDIUMTEXT NOT NULL,
  MODIFY COLUMN `usage_status` VARCHAR(16) NOT NULL DEFAULT 'unavailable',
  DROP INDEX `uk_ai_attempt_command_no`,
  ADD UNIQUE KEY `uk_ai_attempt_run_no` (`run_id`, `attempt_no`),
  ADD KEY `idx_ai_attempt_command` (`command_id`, `attempt_no`),
  ADD CONSTRAINT `fk_ai_provider_attempts_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `chk_ai_provider_attempts_state` CHECK (`state` IN ('prepared', 'dispatched', 'succeeded', 'failed', 'canceled', 'outcome_unknown')),
  ADD CONSTRAINT `chk_ai_provider_attempts_usage_status` CHECK (`usage_status` IN ('complete', 'unavailable'));

ALTER TABLE `ai_text_tasks`
  MODIFY COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  MODIFY COLUMN `request_fingerprint` BINARY(32) NOT NULL,
  MODIFY COLUMN `run_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `kind` VARCHAR(16) NOT NULL DEFAULT 'text',
  MODIFY COLUMN `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  ADD UNIQUE KEY `uk_ai_text_tasks_user_request` (`user_id`, `request_id`),
  ADD KEY `idx_ai_text_tasks_run` (`run_id`),
  ADD CONSTRAINT `fk_ai_text_tasks_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `chk_ai_text_tasks_kind` CHECK (`kind` IN ('text', 'tool_draft'));

ALTER TABLE `ai_image_tasks`
  MODIFY COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  MODIFY COLUMN `request_fingerprint` BINARY(32) NOT NULL,
  MODIFY COLUMN `run_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  ADD UNIQUE KEY `uk_ai_image_tasks_user_request` (`user_id`, `request_id`),
  ADD KEY `idx_ai_image_tasks_run` (`run_id`),
  ADD CONSTRAINT `fk_ai_image_tasks_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_video_tasks`
  MODIFY COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  MODIFY COLUMN `request_fingerprint` BINARY(32) NOT NULL,
  MODIFY COLUMN `run_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  MODIFY COLUMN `storage_provider` VARCHAR(32) NOT NULL DEFAULT '',
  MODIFY COLUMN `storage_key` VARCHAR(1024) NOT NULL DEFAULT '',
  MODIFY COLUMN `content_type` VARCHAR(128) NOT NULL DEFAULT '',
  ADD UNIQUE KEY `uk_ai_video_tasks_user_request` (`user_id`, `request_id`),
  ADD KEY `idx_ai_video_tasks_run` (`run_id`),
  ADD CONSTRAINT `fk_ai_video_tasks_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_run_events`
  DROP CHECK `chk_ai_run_events_type`,
  ADD CONSTRAINT `chk_ai_run_events_type` CHECK (`event_type` IN ('start', 'completed', 'failed', 'canceled', 'timeout', 'retry_scheduled', 'usage_recorded', 'outcome_unknown', 'settled', 'released', 'unbilled'));

ALTER TABLE `user_wallets`
  MODIFY COLUMN `balance_units` BIGINT NOT NULL DEFAULT 0,
  MODIFY COLUMN `total_recharge_units` BIGINT NOT NULL DEFAULT 0,
  MODIFY COLUMN `total_consume_units` BIGINT NOT NULL DEFAULT 0,
  MODIFY COLUMN `held_units` BIGINT NOT NULL DEFAULT 0,
  ADD CONSTRAINT `chk_user_wallet_units_nonnegative` CHECK (`balance_units` >= 0 AND `total_recharge_units` >= 0 AND `total_consume_units` >= 0 AND `held_units` >= 0 AND `held_units` <= `balance_units`),
  DROP COLUMN `balance_cents`,
  DROP COLUMN `total_recharge_cents`,
  DROP COLUMN `total_consume_cents`;

ALTER TABLE `wallet_transactions`
  MODIFY COLUMN `amount_units` BIGINT NOT NULL,
  MODIFY COLUMN `balance_before_units` BIGINT NOT NULL,
  MODIFY COLUMN `balance_after_units` BIGINT NOT NULL,
  ADD CONSTRAINT `chk_wallet_transaction_units_nonnegative` CHECK (`amount_units` >= 0 AND `balance_before_units` >= 0 AND `balance_after_units` >= 0),
  DROP COLUMN `amount_cents`,
  DROP COLUMN `balance_before_cents`,
  DROP COLUMN `balance_after_cents`;

DROP TEMPORARY TABLE `_ai_billing_contract_guard`;
