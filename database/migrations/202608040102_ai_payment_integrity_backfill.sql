DROP TEMPORARY TABLE IF EXISTS `_ai_payment_integrity_backfill_guard`;
CREATE TEMPORARY TABLE `_ai_payment_integrity_backfill_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

START TRANSACTION;

-- A callback without notify_id can only be reconstructed from the immutable
-- raw callback facts. Missing facts are corruption, not a reason to invent a key.
INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `payment_callback_events`
WHERE TRIM(`notify_id`) = ''
  AND (
    `raw_payload_json` IS NULL
    OR JSON_VALID(`raw_payload_json`) = 0
    OR JSON_TYPE(JSON_EXTRACT(`raw_payload_json`, '$.total_amount')) <> 'STRING'
  );

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `derived`.`dedupe_key`
  FROM (
    SELECT UNHEX(SHA2(CONCAT(
      'payment_callback_v1', CHAR(0), TRIM(`provider`), CHAR(0),
      CASE
        WHEN TRIM(`notify_id`) <> '' THEN CONCAT('notify_id', CHAR(0), TRIM(`notify_id`))
        ELSE CONCAT(
          'callback_facts', CHAR(0), TRIM(`out_trade_no`), CHAR(0),
          TRIM(`trade_no`), CHAR(0), TRIM(`trade_status`), CHAR(0),
          TRIM(`app_id`), CHAR(0),
          TRIM(JSON_UNQUOTE(JSON_EXTRACT(`raw_payload_json`, '$.total_amount')))
        )
      END
    ), 256)) AS `dedupe_key`
    FROM `payment_callback_events`
  ) AS `derived`
  GROUP BY `derived`.`dedupe_key`
  HAVING COUNT(*) <> 1
) AS `duplicate_callback_identity`;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `payment_orders`
WHERE BINARY `alipay_trade_no` <> BINARY TRIM(`alipay_trade_no`);

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT TRIM(`alipay_trade_no`) AS `trade_identity`
  FROM `payment_orders`
  WHERE TRIM(`alipay_trade_no`) <> ''
  GROUP BY TRIM(`alipay_trade_no`)
  HAVING COUNT(*) <> 1
) AS `duplicate_trade_identity`;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT agent.`id`
  FROM `ai_agents` AS agent
  LEFT JOIN `ai_provider_models` AS provider_model
    ON provider_model.`provider_id` = agent.`provider_id`
   AND BINARY provider_model.`model_id` = BINARY agent.`model_id`
   AND provider_model.`model_kind` = 'chat'
  GROUP BY agent.`id`
  HAVING COUNT(provider_model.`id`) <> 1
) AS `invalid_agent_model_mapping`;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT command_row.`id`
  FROM `ai_reply_commands` AS command_row
  LEFT JOIN `ai_runs` AS run_row
    ON run_row.`user_message_id` = command_row.`user_message_id`
  GROUP BY command_row.`id`
  HAVING COUNT(run_row.`id`) <> 1
) AS `invalid_command_run_mapping`;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_reply_commands` AS command_row
JOIN `ai_runs` AS run_row
  ON run_row.`user_message_id` = command_row.`user_message_id`
WHERE run_row.`user_id` <> command_row.`user_id`
   OR run_row.`conversation_id` <> command_row.`conversation_id`
   OR BINARY run_row.`request_id` <> BINARY command_row.`request_id`
   OR run_row.`request_fingerprint` <> command_row.`request_fingerprint`;

UPDATE `payment_callback_events`
SET `dedupe_key` = UNHEX(SHA2(CONCAT(
  'payment_callback_v1', CHAR(0), TRIM(`provider`), CHAR(0),
  CASE
    WHEN TRIM(`notify_id`) <> '' THEN CONCAT('notify_id', CHAR(0), TRIM(`notify_id`))
    ELSE CONCAT(
      'callback_facts', CHAR(0), TRIM(`out_trade_no`), CHAR(0),
      TRIM(`trade_no`), CHAR(0), TRIM(`trade_status`), CHAR(0),
      TRIM(`app_id`), CHAR(0),
      TRIM(JSON_UNQUOTE(JSON_EXTRACT(`raw_payload_json`, '$.total_amount')))
    )
  END
), 256));

UPDATE `payment_orders`
SET `alipay_trade_no_identity` = NULLIF(TRIM(`alipay_trade_no`), '');

UPDATE `ai_agents` AS agent
JOIN `ai_provider_models` AS provider_model
  ON provider_model.`provider_id` = agent.`provider_id`
 AND BINARY provider_model.`model_id` = BINARY agent.`model_id`
 AND provider_model.`model_kind` = 'chat'
SET agent.`provider_model_id` = provider_model.`id`;

UPDATE `ai_reply_commands` AS command_row
JOIN `ai_runs` AS run_row ON run_row.`user_message_id` = command_row.`user_message_id`
SET command_row.`run_id` = run_row.`id`;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `payment_callback_events`
WHERE `dedupe_key` IS NULL OR OCTET_LENGTH(`dedupe_key`) <> 32;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `payment_orders`
WHERE (`alipay_trade_no` = '' AND `alipay_trade_no_identity` IS NOT NULL)
   OR (`alipay_trade_no` <> '' AND (
     `alipay_trade_no_identity` IS NULL
     OR BINARY `alipay_trade_no_identity` <> BINARY `alipay_trade_no`
   ));

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents` AS agent
LEFT JOIN `ai_provider_models` AS provider_model
  ON provider_model.`id` = agent.`provider_model_id`
 AND provider_model.`provider_id` = agent.`provider_id`
 AND BINARY provider_model.`model_id` = BINARY agent.`model_id`
 AND provider_model.`model_kind` = 'chat'
WHERE agent.`provider_model_id` IS NULL OR provider_model.`id` IS NULL;

INSERT INTO `_ai_payment_integrity_backfill_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_reply_commands` AS command_row
LEFT JOIN `ai_runs` AS run_row
  ON run_row.`id` = command_row.`run_id`
 AND run_row.`user_id` = command_row.`user_id`
 AND run_row.`conversation_id` = command_row.`conversation_id`
 AND run_row.`user_message_id` = command_row.`user_message_id`
 AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
WHERE command_row.`run_id` IS NULL OR run_row.`id` IS NULL;

COMMIT;

DROP TEMPORARY TABLE `_ai_payment_integrity_backfill_guard`;
