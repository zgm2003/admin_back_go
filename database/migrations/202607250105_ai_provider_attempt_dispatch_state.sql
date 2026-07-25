-- Persist whether a provider request crossed the dispatch boundary. This fact
-- is required for safe retry and settlement after a process restart.
DROP TEMPORARY TABLE IF EXISTS `_ai_attempt_dispatch_state_guard`;
CREATE TEMPORARY TABLE `_ai_attempt_dispatch_state_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_attempt_dispatch_state_guard`
SELECT IF(COUNT(*) = 1, 0, 1)
FROM `ai_billing_migration_metadata`
WHERE `migration_key` = 'ai_billing_contract_v1' AND `phase` = 'complete';

SET @ai_attempt_dispatch_state_preexisting = (
  SELECT COUNT(*) FROM `ai_billing_migration_metadata`
  WHERE `migration_key` = 'ai_attempt_dispatch_state_v1'
);
INSERT INTO `_ai_attempt_dispatch_state_guard`
SELECT IF(COALESCE(@ai_attempt_dispatch_state_preexisting, 0) = 0, 0, 1);

INSERT INTO `ai_billing_migration_metadata` (
  `migration_key`, `legacy_cutover_at`, `marker_version`, `marker_sha256`,
  `phase`, `phase_started_at`, `phase_completed_at`
)
VALUES (
  'ai_attempt_dispatch_state_v1', CURRENT_TIMESTAMP(6),
  'ai_attempt_dispatch_state_v1',
  UNHEX(SHA2('ai_attempt_dispatch_state_v1', 256)),
  'started', CURRENT_TIMESTAMP(6), NULL
);

ALTER TABLE `ai_provider_attempts`
  ADD COLUMN `dispatch_state` VARCHAR(16) NOT NULL DEFAULT 'not_dispatched'
    AFTER `usage_status`;

UPDATE `ai_provider_attempts`
SET `dispatch_state` = CASE
  WHEN `state` = 'prepared' AND `dispatched_at` IS NULL THEN 'not_dispatched'
  WHEN `state` = 'outcome_unknown' THEN 'unknown'
  WHEN `dispatched_at` IS NOT NULL THEN 'dispatched'
  ELSE 'unknown'
END;

ALTER TABLE `ai_provider_attempts`
  ADD CONSTRAINT `chk_ai_provider_attempts_dispatch_state`
    CHECK (`dispatch_state` IN ('not_dispatched', 'dispatched', 'unknown'));

INSERT INTO `_ai_attempt_dispatch_state_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_provider_attempts`
WHERE `dispatch_state` NOT IN ('not_dispatched', 'dispatched', 'unknown')
   OR (`state` = 'prepared' AND `dispatched_at` IS NULL AND `dispatch_state` <> 'not_dispatched')
   OR (`state` = 'outcome_unknown' AND `dispatch_state` <> 'unknown');

UPDATE `ai_billing_migration_metadata`
SET `phase` = 'complete', `phase_completed_at` = CURRENT_TIMESTAMP(6)
WHERE `migration_key` = 'ai_attempt_dispatch_state_v1' AND `phase` = 'started';

DROP TEMPORARY TABLE `_ai_attempt_dispatch_state_guard`;
