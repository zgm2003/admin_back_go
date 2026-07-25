-- Backfill only in the declared maintenance window. This revision never creates
-- a historical Hold or Charge: legacy terminal work is explicitly unbilled.
DROP TEMPORARY TABLE IF EXISTS `_ai_billing_backfill_guard`;
CREATE TEMPORARY TABLE `_ai_billing_backfill_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

-- Backfill is resumable only at a phase boundary.  If the previous run
-- stopped after a write, the durable marker makes this script fail closed;
-- an operator must inspect the rows and explicitly record a corrective phase.
INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `ai_billing_migration_metadata`
WHERE `migration_key` = 'ai_billing_expand_v1' AND `phase` = 'complete';

SET @ai_billing_backfill_preexisting = (
  SELECT COUNT(*) FROM `ai_billing_migration_metadata`
  WHERE `migration_key` = 'ai_billing_backfill_v1'
);
INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COALESCE(@ai_billing_backfill_preexisting, 0) = 0, 0, 1);

-- Capture one durable boundary before writing any legacy identity marker. The
-- second-level precision is deliberate: legacy created_at columns are second
-- precision, so rows created at the boundary are rejected conservatively.
SET @ai_billing_legacy_cutover_at = CURRENT_TIMESTAMP(0);

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_billing_migration_metadata`
WHERE `migration_key` = 'legacy_cutover_v1';

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_runs`
WHERE `status` NOT IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')
   OR `request_id` = '';

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `user_wallets`
WHERE `balance_cents` < 0 OR `total_recharge_cents` < 0 OR `total_consume_cents` < 0
   OR `balance_cents` > 9223372036854 OR `total_recharge_cents` > 9223372036854 OR `total_consume_cents` > 9223372036854;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `wallet_transactions`
WHERE `amount_cents` < 0 OR `balance_before_cents` < 0 OR `balance_after_cents` < 0
   OR `amount_cents` > 9223372036854 OR `balance_before_cents` > 9223372036854 OR `balance_after_cents` > 9223372036854;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `user_id`, BINARY `request_id` AS `request_id`
  FROM `ai_runs`
  GROUP BY `user_id`, BINARY `request_id`
  HAVING COUNT(*) <> 1
) AS duplicate_run_identity;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `user_id`, BINARY `request_id` AS `request_id`
  FROM `ai_reply_commands`
  GROUP BY `user_id`, BINARY `request_id`
  HAVING COUNT(*) <> 1
) AS duplicate_command_identity;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT command_row.`id`
  FROM `ai_reply_commands` AS command_row
  LEFT JOIN `ai_runs` AS run_row
    ON run_row.`user_id` = command_row.`user_id`
   AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
  GROUP BY command_row.`id`
  HAVING COUNT(run_row.`id`) <> 1
) AS unmapped_commands;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT attempt.`id`
  FROM `ai_provider_attempts` AS attempt
  LEFT JOIN `ai_reply_commands` AS command_row ON command_row.`id` = attempt.`command_id`
  LEFT JOIN `ai_runs` AS run_row
    ON run_row.`user_id` = command_row.`user_id`
   AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
  GROUP BY attempt.`id`
  HAVING COUNT(run_row.`id`) <> 1
) AS unmapped_attempts;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT task.`id`
  FROM `ai_text_tasks` AS task
  LEFT JOIN `ai_runs` AS run_row
    ON run_row.`user_id` = task.`user_id`
   AND BINARY run_row.`request_id` IN (
     BINARY CONCAT('text-completion-', task.`id`),
     BINARY CONCAT('ai_text_task_', task.`id`)
   )
  GROUP BY task.`id`
  HAVING COUNT(run_row.`id`) <> 1
  UNION ALL
  SELECT task.`id`
  FROM `ai_image_tasks` AS task
  LEFT JOIN `ai_runs` AS run_row
    ON run_row.`user_id` = task.`user_id`
   AND BINARY run_row.`request_id` = BINARY CONCAT('ai_image_task_', task.`id`)
  GROUP BY task.`id`
  HAVING COUNT(run_row.`id`) <> 1
  UNION ALL
  SELECT task.`id`
  FROM `ai_video_tasks` AS task
  LEFT JOIN `ai_runs` AS run_row
    ON run_row.`id` = task.`run_id`
   AND run_row.`user_id` = task.`user_id`
  GROUP BY task.`id`
  HAVING COUNT(run_row.`id`) <> 1
) AS unmapped_paid_tasks;

-- All read-only preflight guards passed.  The next section locks writers and
-- mutates units, so journal the durable started boundary immediately before it.
INSERT INTO `ai_billing_migration_metadata` (
  `migration_key`, `legacy_cutover_at`, `marker_version`, `marker_sha256`,
  `phase`, `phase_started_at`, `phase_completed_at`
)
VALUES (
  'ai_billing_backfill_v1', CURRENT_TIMESTAMP(6), 'ai_billing_backfill_v1',
  UNHEX(SHA2('ai_billing_backfill_v1', 256)), 'started', CURRENT_TIMESTAMP(6), NULL
);

-- Wallet writers are locked across both conversion and conservation checks.
SET @ai_billing_previous_autocommit = @@autocommit;
SET autocommit = 0;
LOCK TABLES `user_wallets` WRITE, `wallet_transactions` WRITE;

UPDATE `user_wallets`
SET `balance_units` = `balance_cents` * 1000000,
    `total_recharge_units` = `total_recharge_cents` * 1000000,
    `total_consume_units` = `total_consume_cents` * 1000000,
    `held_units` = 0;

UPDATE `wallet_transactions`
SET `amount_units` = `amount_cents` * 1000000,
    `balance_before_units` = `balance_before_cents` * 1000000,
    `balance_after_units` = `balance_after_cents` * 1000000;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `wallet_transactions`
WHERE `amount_units` <> `amount_cents` * 1000000
   OR `balance_before_units` <> `balance_before_cents` * 1000000
   OR `balance_after_units` <> `balance_after_cents` * 1000000
   OR `direction` NOT IN ('in', 'out')
   OR `balance_before_units` + CASE WHEN `direction` = 'in' THEN `amount_units` ELSE -`amount_units` END <> `balance_after_units`;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `user_wallets` AS wallet
LEFT JOIN (
  SELECT `wallet_id`,
    SUM(CASE WHEN `direction` = 'in' THEN `amount_units` ELSE 0 END) AS recharge_units,
    SUM(CASE WHEN `direction` = 'out' THEN `amount_units` ELSE 0 END) AS consume_units
  FROM `wallet_transactions`
  WHERE `is_del` = 2
  GROUP BY `wallet_id`
) AS ledger ON ledger.`wallet_id` = wallet.`id`
WHERE wallet.`total_recharge_units` <> COALESCE(ledger.`recharge_units`, 0)
   OR wallet.`total_consume_units` <> COALESCE(ledger.`consume_units`, 0)
   OR wallet.`balance_units` <> wallet.`total_recharge_units` - wallet.`total_consume_units`;

COMMIT;
UNLOCK TABLES;
SET autocommit = @ai_billing_previous_autocommit;

START TRANSACTION;

INSERT INTO `ai_billing_migration_metadata` (
  `migration_key`, `legacy_cutover_at`, `marker_version`, `marker_sha256`
)
VALUES (
  'legacy_cutover_v1',
  @ai_billing_legacy_cutover_at,
  'legacy_non_replayable_v1',
  UNHEX(SHA2('legacy_non_replayable_v1', 256))
);

UPDATE `ai_runs`
SET `request_fingerprint` = UNHEX(SHA2(CONCAT('legacy_non_replayable_v1:ai_runs:', `id`), 256)),
    `request_identity_status` = 'legacy_non_replayable',
    `request_identity_marker` = CONCAT('legacy_non_replayable_v1:ai_runs:', `id`),
    `pricing_snapshot_json` = '{"version":"legacy_unpriced_v1","billable":false}',
    `billing_status` = 'unbilled',
    `billing_reason` = 'legacy_unpriced';

UPDATE `ai_reply_commands` AS command_row
JOIN `ai_runs` AS run_row
  ON run_row.`user_id` = command_row.`user_id`
 AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
SET command_row.`request_fingerprint` = run_row.`request_fingerprint`,
    command_row.`request_identity_status` = run_row.`request_identity_status`,
    command_row.`request_identity_marker` = run_row.`request_identity_marker`;

UPDATE `ai_provider_attempts` AS attempt
JOIN `ai_reply_commands` AS command_row ON command_row.`id` = attempt.`command_id`
JOIN `ai_runs` AS run_row
  ON run_row.`user_id` = command_row.`user_id`
 AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
SET attempt.`run_id` = run_row.`id`,
    attempt.`prepared_request_json` = '{"version":"legacy_unavailable_v1","replayable":false}',
    attempt.`prepared_request_sha256` = UNHEX(SHA2('{"version":"legacy_unavailable_v1","replayable":false}', 256)),
    attempt.`quote_json` = '{"version":"legacy_unpriced_v1","billable":false}',
    attempt.`usage_json` = '{"status":"unavailable","items":[]}',
    attempt.`usage_status` = 'unavailable',
    attempt.`result_candidate_json` = NULL;

UPDATE `ai_text_tasks` AS task
JOIN `ai_runs` AS run_row
  ON run_row.`user_id` = task.`user_id`
 AND BINARY run_row.`request_id` IN (
   BINARY CONCAT('text-completion-', task.`id`),
   BINARY CONCAT('ai_text_task_', task.`id`)
 )
SET task.`request_id` = run_row.`request_id`,
    task.`request_fingerprint` = run_row.`request_fingerprint`,
    task.`request_identity_status` = run_row.`request_identity_status`,
    task.`request_identity_marker` = run_row.`request_identity_marker`,
    task.`run_id` = run_row.`id`,
    task.`kind` = 'text',
    task.`last_error_code` = '';

UPDATE `ai_image_tasks` AS task
JOIN `ai_runs` AS run_row
  ON run_row.`user_id` = task.`user_id`
 AND BINARY run_row.`request_id` = BINARY CONCAT('ai_image_task_', task.`id`)
SET task.`request_id` = run_row.`request_id`,
    task.`request_fingerprint` = run_row.`request_fingerprint`,
    task.`request_identity_status` = run_row.`request_identity_status`,
    task.`request_identity_marker` = run_row.`request_identity_marker`,
    task.`run_id` = run_row.`id`,
    task.`last_error_code` = '';

UPDATE `ai_video_tasks` AS task
JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id` AND run_row.`user_id` = task.`user_id`
SET task.`request_id` = run_row.`request_id`,
    task.`request_fingerprint` = run_row.`request_fingerprint`,
    task.`request_identity_status` = run_row.`request_identity_status`,
    task.`request_identity_marker` = run_row.`request_identity_marker`,
    task.`last_error_code` = '';

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_billing_migration_metadata`
WHERE `migration_key` = 'legacy_cutover_v1'
  AND (`legacy_cutover_at` <> @ai_billing_legacy_cutover_at
    OR `marker_version` <> 'legacy_non_replayable_v1'
    OR `marker_sha256` <> UNHEX(SHA2('legacy_non_replayable_v1', 256)));

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `ai_billing_migration_metadata`
WHERE `migration_key` = 'legacy_cutover_v1';

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT command_row.`id` FROM `ai_reply_commands` AS command_row
  WHERE command_row.`request_identity_status` = 'legacy_non_replayable'
    AND command_row.`created_at` >= @ai_billing_legacy_cutover_at
  UNION ALL
  SELECT task.`id` FROM `ai_text_tasks` AS task
  WHERE task.`request_identity_status` = 'legacy_non_replayable'
    AND task.`created_at` >= @ai_billing_legacy_cutover_at
  UNION ALL
  SELECT task.`id` FROM `ai_image_tasks` AS task
  WHERE task.`request_identity_status` = 'legacy_non_replayable'
    AND task.`created_at` >= @ai_billing_legacy_cutover_at
  UNION ALL
  SELECT task.`id` FROM `ai_video_tasks` AS task
  WHERE task.`request_identity_status` = 'legacy_non_replayable'
    AND task.`created_at` >= @ai_billing_legacy_cutover_at
  UNION ALL
  SELECT attempt.`id` FROM `ai_provider_attempts` AS attempt
  JOIN `ai_runs` AS run_row ON run_row.`id` = attempt.`run_id`
  WHERE run_row.`request_identity_status` = 'legacy_non_replayable'
    AND attempt.`created_at` >= @ai_billing_legacy_cutover_at
) AS child_legacy_identity_after_cutover;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_runs`
WHERE `request_fingerprint` IS NULL OR `pricing_snapshot_json` <> '{"version":"legacy_unpriced_v1","billable":false}'
   OR JSON_VALID(`pricing_snapshot_json`) = 0 OR `billing_status` <> 'unbilled' OR `billing_reason` <> 'legacy_unpriced'
   OR `request_identity_status` <> 'legacy_non_replayable'
   OR `request_identity_marker` <> CONCAT('legacy_non_replayable_v1:ai_runs:', `id`)
   OR `request_fingerprint` <> UNHEX(SHA2(`request_identity_marker`, 256))
   OR `created_at` >= @ai_billing_legacy_cutover_at;

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_provider_attempts`
WHERE `run_id` IS NULL OR `prepared_request_json` <> '{"version":"legacy_unavailable_v1","replayable":false}'
   OR `prepared_request_sha256` <> UNHEX(SHA2('{"version":"legacy_unavailable_v1","replayable":false}', 256))
   OR `quote_json` <> '{"version":"legacy_unpriced_v1","billable":false}'
   OR `usage_json` <> '{"status":"unavailable","items":[]}' OR `usage_status` <> 'unavailable';

INSERT INTO `_ai_billing_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT command_row.`id`
  FROM `ai_reply_commands` AS command_row
  JOIN `ai_runs` AS run_row ON run_row.`user_id` = command_row.`user_id` AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
  WHERE command_row.`request_fingerprint` <> run_row.`request_fingerprint`
     OR command_row.`request_identity_status` <> 'legacy_non_replayable'
     OR command_row.`request_identity_marker` <> run_row.`request_identity_marker`
  UNION ALL
  SELECT task.`id` FROM `ai_text_tasks` AS task JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id`
  WHERE task.`request_id` IS NULL OR task.`request_fingerprint` IS NULL OR task.`run_id` IS NULL OR task.`run_id` = 0
     OR task.`request_fingerprint` <> run_row.`request_fingerprint` OR task.`request_identity_status` <> 'legacy_non_replayable' OR task.`request_identity_marker` <> run_row.`request_identity_marker`
  UNION ALL SELECT task.`id` FROM `ai_image_tasks` AS task JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id`
  WHERE task.`request_id` IS NULL OR task.`request_fingerprint` IS NULL OR task.`run_id` IS NULL OR task.`run_id` = 0
     OR task.`request_fingerprint` <> run_row.`request_fingerprint` OR task.`request_identity_status` <> 'legacy_non_replayable' OR task.`request_identity_marker` <> run_row.`request_identity_marker`
  UNION ALL SELECT task.`id` FROM `ai_video_tasks` AS task JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id`
  WHERE task.`request_id` IS NULL OR task.`request_fingerprint` IS NULL OR task.`run_id` IS NULL OR task.`run_id` = 0
     OR task.`request_fingerprint` <> run_row.`request_fingerprint` OR task.`request_identity_status` <> 'legacy_non_replayable' OR task.`request_identity_marker` <> run_row.`request_identity_marker`
) AS incomplete_task_identity;

UPDATE `ai_billing_migration_metadata`
SET `phase` = 'complete', `phase_completed_at` = CURRENT_TIMESTAMP(6)
WHERE `migration_key` = 'ai_billing_backfill_v1' AND `phase` = 'started';
UPDATE `ai_billing_migration_metadata`
SET `phase` = 'complete', `phase_completed_at` = CURRENT_TIMESTAMP(6)
WHERE `migration_key` = 'legacy_cutover_v1' AND `phase` = 'not_started';

COMMIT;
DROP TEMPORARY TABLE `_ai_billing_backfill_guard`;
