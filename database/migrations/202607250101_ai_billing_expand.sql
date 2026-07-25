-- Expand AI billing facts only after all legacy paid writers are stopped.
DROP TEMPORARY TABLE IF EXISTS `_ai_billing_expand_guard`;
CREATE TEMPORARY TABLE `_ai_billing_expand_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `id` FROM `ai_reply_commands` WHERE `state` IN ('pending', 'claimed', 'running')
  UNION ALL SELECT `id` FROM `ai_text_tasks` WHERE `status` = 'running'
  UNION ALL SELECT `id` FROM `ai_image_tasks` WHERE `status` IN ('pending', 'running')
  UNION ALL SELECT `id` FROM `ai_video_tasks` WHERE `status` IN ('pending', 'running')
) AS active_paid_work;

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `user_wallets`
WHERE `balance_cents` < 0 OR `total_recharge_cents` < 0 OR `total_consume_cents` < 0
   OR `balance_cents` > 9223372036854 OR `total_recharge_cents` > 9223372036854 OR `total_consume_cents` > 9223372036854;

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `wallet_transactions`
WHERE `amount_cents` < 0 OR `balance_before_cents` < 0 OR `balance_after_cents` < 0
   OR `amount_cents` > 9223372036854 OR `balance_before_cents` > 9223372036854 OR `balance_after_cents` > 9223372036854;

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (SELECT `user_id` FROM `user_wallets` GROUP BY `user_id` HAVING COUNT(*) <> 1) AS duplicate_wallet_users;

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (SELECT `source_type`, `source_id` FROM `wallet_transactions` GROUP BY `source_type`, `source_id` HAVING COUNT(*) <> 1) AS duplicate_sources;

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `user_id`, BINARY `request_id` AS `request_id` FROM `ai_runs` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
  UNION ALL
  SELECT `user_id`, BINARY `request_id` AS `request_id` FROM `ai_reply_commands` GROUP BY `user_id`, BINARY `request_id` HAVING COUNT(*) <> 1
) AS duplicate_request_identities;

INSERT INTO `_ai_billing_expand_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT attempt.`id`
  FROM `ai_provider_attempts` AS attempt
  LEFT JOIN `ai_reply_commands` AS command_row ON command_row.`id` = attempt.`command_id`
  WHERE command_row.`id` IS NULL
  UNION ALL
  SELECT task.`id` FROM `ai_text_tasks` AS task LEFT JOIN `users` AS user_row ON user_row.`id` = task.`user_id` LEFT JOIN `ai_agents` AS agent ON agent.`id` = task.`agent_id` LEFT JOIN `ai_providers` AS provider ON provider.`id` = task.`provider_id` WHERE user_row.`id` IS NULL OR agent.`id` IS NULL OR provider.`id` IS NULL
  UNION ALL
  SELECT task.`id` FROM `ai_image_tasks` AS task LEFT JOIN `users` AS user_row ON user_row.`id` = task.`user_id` LEFT JOIN `ai_agents` AS agent ON agent.`id` = task.`agent_id` LEFT JOIN `ai_providers` AS provider ON provider.`id` = task.`provider_id_snapshot` WHERE user_row.`id` IS NULL OR agent.`id` IS NULL OR provider.`id` IS NULL
  UNION ALL
  SELECT task.`id` FROM `ai_video_tasks` AS task LEFT JOIN `users` AS user_row ON user_row.`id` = task.`user_id` LEFT JOIN `ai_agents` AS agent ON agent.`id` = task.`agent_id` LEFT JOIN `ai_providers` AS provider ON provider.`id` = task.`provider_id` LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = NULLIF(task.`run_id`, 0) WHERE user_row.`id` IS NULL OR agent.`id` IS NULL OR provider.`id` IS NULL OR (task.`run_id` <> 0 AND run_row.`id` IS NULL)
  UNION ALL
  SELECT run_row.`id` FROM `ai_runs` AS run_row LEFT JOIN `users` AS user_row ON user_row.`id` = run_row.`user_id` LEFT JOIN `ai_agents` AS agent ON agent.`id` = run_row.`agent_id` LEFT JOIN `ai_providers` AS provider ON provider.`id` = run_row.`provider_id` LEFT JOIN `ai_conversations` AS conversation ON conversation.`id` = run_row.`conversation_id` LEFT JOIN `ai_messages` AS user_message ON user_message.`id` = run_row.`user_message_id` LEFT JOIN `ai_messages` AS assistant_message ON assistant_message.`id` = run_row.`assistant_message_id` WHERE user_row.`id` IS NULL OR agent.`id` IS NULL OR provider.`id` IS NULL OR (run_row.`conversation_id` IS NOT NULL AND conversation.`id` IS NULL) OR (run_row.`user_message_id` IS NOT NULL AND user_message.`id` IS NULL) OR (run_row.`assistant_message_id` IS NOT NULL AND assistant_message.`id` IS NULL)
  UNION ALL
  SELECT event_row.`id` FROM `ai_run_events` AS event_row LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = event_row.`run_id` WHERE run_row.`id` IS NULL
  UNION ALL
  SELECT command_row.`id` FROM `ai_reply_commands` AS command_row LEFT JOIN `users` AS user_row ON user_row.`id` = command_row.`user_id` LEFT JOIN `ai_conversations` AS conversation ON conversation.`id` = command_row.`conversation_id` LEFT JOIN `ai_messages` AS message_row ON message_row.`id` = command_row.`user_message_id` WHERE user_row.`id` IS NULL OR conversation.`id` IS NULL OR message_row.`id` IS NULL
  UNION ALL
  SELECT file_row.`id` FROM `ai_image_files` AS file_row LEFT JOIN `ai_image_tasks` AS task ON task.`id` = file_row.`task_id` WHERE task.`id` IS NULL
) AS orphan_ai_facts;

ALTER TABLE `ai_agents`
  ADD COLUMN `billing_multiplier_ppm` BIGINT UNSIGNED NOT NULL DEFAULT 1000000 AFTER `model_display_name`,
  ADD COLUMN `max_output_tokens` INT UNSIGNED NOT NULL DEFAULT 4096 AFTER `billing_multiplier_ppm`;

ALTER TABLE `ai_runs`
  ADD COLUMN `request_fingerprint` BINARY(32) NULL AFTER `request_id`,
  ADD COLUMN `pricing_snapshot_json` MEDIUMTEXT NULL AFTER `input_snapshot`,
  ADD COLUMN `billing_status` VARCHAR(16) NULL AFTER `status`,
  ADD COLUMN `billing_reason` VARCHAR(32) NULL AFTER `billing_status`;

ALTER TABLE `ai_reply_commands`
  ADD COLUMN `request_fingerprint` BINARY(32) NULL AFTER `request_id`;

ALTER TABLE `ai_provider_attempts`
  ADD COLUMN `run_id` BIGINT UNSIGNED NULL AFTER `id`,
  ADD COLUMN `prepared_request_json` MEDIUMTEXT NULL AFTER `state`,
  ADD COLUMN `prepared_request_sha256` BINARY(32) NULL AFTER `prepared_request_json`,
  ADD COLUMN `quote_json` MEDIUMTEXT NULL AFTER `prepared_request_sha256`,
  ADD COLUMN `usage_json` MEDIUMTEXT NULL AFTER `quote_json`,
  ADD COLUMN `usage_status` VARCHAR(16) NULL DEFAULT 'unavailable' AFTER `usage_json`,
  ADD COLUMN `result_candidate_json` MEDIUMTEXT NULL AFTER `usage_status`,
  MODIFY COLUMN `command_id` BIGINT UNSIGNED NULL;

ALTER TABLE `user_wallets`
  ADD COLUMN `balance_units` BIGINT NULL AFTER `balance_cents`,
  ADD COLUMN `total_recharge_units` BIGINT NULL AFTER `total_recharge_cents`,
  ADD COLUMN `total_consume_units` BIGINT NULL AFTER `total_consume_cents`,
  ADD COLUMN `held_units` BIGINT NULL AFTER `total_consume_units`;

ALTER TABLE `wallet_transactions`
  ADD COLUMN `amount_units` BIGINT NULL AFTER `amount_cents`,
  ADD COLUMN `balance_before_units` BIGINT NULL AFTER `balance_before_cents`,
  ADD COLUMN `balance_after_units` BIGINT NULL AFTER `balance_after_cents`;

ALTER TABLE `ai_text_tasks`
  ADD COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER `user_id`,
  ADD COLUMN `request_fingerprint` BINARY(32) NULL AFTER `request_id`,
  ADD COLUMN `run_id` BIGINT UNSIGNED NULL AFTER `request_fingerprint`,
  ADD COLUMN `kind` VARCHAR(16) NULL DEFAULT 'text' AFTER `run_id`,
  ADD COLUMN `last_error_code` VARCHAR(64) NULL DEFAULT '' AFTER `error_message`;

ALTER TABLE `ai_image_tasks`
  ADD COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER `user_id`,
  ADD COLUMN `request_fingerprint` BINARY(32) NULL AFTER `request_id`,
  ADD COLUMN `run_id` BIGINT UNSIGNED NULL AFTER `request_fingerprint`,
  ADD COLUMN `last_error_code` VARCHAR(64) NULL DEFAULT '' AFTER `error_message`;

ALTER TABLE `ai_video_tasks`
  ADD COLUMN `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER `user_id`,
  ADD COLUMN `request_fingerprint` BINARY(32) NULL AFTER `request_id`,
  ADD COLUMN `last_error_code` VARCHAR(64) NULL DEFAULT '' AFTER `error_message`,
  ADD COLUMN `storage_provider` VARCHAR(32) NULL DEFAULT '' AFTER `last_error_code`,
  ADD COLUMN `storage_key` VARCHAR(1024) NULL DEFAULT '' AFTER `storage_provider`,
  ADD COLUMN `content_type` VARCHAR(128) NULL DEFAULT '' AFTER `storage_key`;

CREATE TABLE `wallet_holds` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `wallet_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `run_id` BIGINT UNSIGNED NOT NULL,
  `held_units` BIGINT NOT NULL DEFAULT 0,
  `captured_units` BIGINT NOT NULL DEFAULT 0,
  `status` VARCHAR(16) NOT NULL DEFAULT 'active',
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_wallet_holds_run` (`run_id`),
  KEY `idx_wallet_holds_wallet_status` (`wallet_id`, `status`),
  CONSTRAINT `fk_wallet_holds_wallet` FOREIGN KEY (`wallet_id`) REFERENCES `user_wallets` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_wallet_holds_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_wallet_holds_status` CHECK (`status` IN ('active', 'captured', 'released')),
  CONSTRAINT `chk_wallet_holds_units` CHECK (`held_units` >= 0 AND `captured_units` >= 0 AND `captured_units` <= `held_units`)
);

CREATE TABLE `ai_usage_charges` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `run_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT NOT NULL,
  `currency` CHAR(3) NOT NULL DEFAULT 'CNY',
  `pricing_version` VARCHAR(64) NOT NULL,
  `multiplier_ppm` BIGINT UNSIGNED NOT NULL,
  `held_units` BIGINT NOT NULL DEFAULT 0,
  `actual_units` BIGINT NOT NULL DEFAULT 0,
  `status` VARCHAR(16) NOT NULL DEFAULT 'open',
  `finalized_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_usage_charges_run` (`run_id`),
  KEY `idx_ai_usage_charges_user_created` (`user_id`, `created_at`, `id`),
  CONSTRAINT `fk_ai_usage_charges_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_usage_charges_status` CHECK (`status` IN ('open', 'settled', 'released', 'unbilled')),
  CONSTRAINT `chk_ai_usage_charges_currency` CHECK (`currency` = 'CNY'),
  CONSTRAINT `chk_ai_usage_charges_units` CHECK (`held_units` >= 0 AND `actual_units` >= 0)
);

CREATE TABLE `ai_usage_charge_items` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `charge_id` BIGINT UNSIGNED NOT NULL,
  `attempt_id` BIGINT UNSIGNED NOT NULL,
  `category` VARCHAR(32) NOT NULL,
  `tier_key` VARCHAR(64) NOT NULL DEFAULT '',
  `quantity` BIGINT NOT NULL,
  `unit` VARCHAR(32) NOT NULL,
  `unit_price_units` BIGINT NOT NULL,
  `unit_scale` BIGINT NOT NULL,
  `amount_units` BIGINT NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_usage_charge_item_identity` (`charge_id`, `attempt_id`, `category`, `tier_key`, `unit`),
  KEY `idx_ai_usage_charge_items_attempt` (`attempt_id`),
  CONSTRAINT `fk_ai_usage_charge_items_charge` FOREIGN KEY (`charge_id`) REFERENCES `ai_usage_charges` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_usage_charge_items_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `ai_provider_attempts` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_usage_charge_items_category` CHECK (`category` IN ('input', 'output', 'cache_read', 'cache_write', 'media')),
  CONSTRAINT `chk_ai_usage_charge_items_units` CHECK (`quantity` > 0 AND `unit_price_units` >= 0 AND `unit_scale` > 0 AND `amount_units` >= 0)
);

CREATE TABLE `ai_audio_tasks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `platform` VARCHAR(32) NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `request_id` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `request_fingerprint` BINARY(32) NOT NULL,
  `run_id` BIGINT UNSIGNED NOT NULL,
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `agent_name_snapshot` VARCHAR(128) NOT NULL DEFAULT '',
  `provider_id_snapshot` BIGINT UNSIGNED NOT NULL,
  `provider_name_snapshot` VARCHAR(128) NOT NULL DEFAULT '',
  `model_id_snapshot` VARCHAR(191) NOT NULL,
  `model_display_name_snapshot` VARCHAR(191) NOT NULL DEFAULT '',
  `normalized_request_json` MEDIUMTEXT NOT NULL,
  `status` VARCHAR(24) NOT NULL DEFAULT 'pending',
  `storage_provider` VARCHAR(32) NOT NULL DEFAULT '',
  `storage_key` VARCHAR(1024) NOT NULL DEFAULT '',
  `content_type` VARCHAR(128) NOT NULL DEFAULT '',
  `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `error_message` VARCHAR(1024) NOT NULL DEFAULT '',
  `started_at` DATETIME(6) NULL,
  `finished_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_audio_tasks_user_request` (`user_id`, `request_id`),
  KEY `idx_ai_audio_tasks_run` (`run_id`),
  KEY `idx_ai_audio_tasks_status_created` (`status`, `created_at`, `id`),
  CONSTRAINT `fk_ai_audio_tasks_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_audio_tasks_status` CHECK (`status` IN ('pending', 'running', 'success', 'failed', 'canceled', 'outcome_unknown'))
);

DROP TEMPORARY TABLE `_ai_billing_expand_guard`;
