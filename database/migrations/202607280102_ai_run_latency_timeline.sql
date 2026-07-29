ALTER TABLE `ai_reply_commands`
  ADD COLUMN `request_received_at` datetime(6) NULL AFTER `assistant_message_id`,
  ADD COLUMN `accepted_at` datetime(6) NULL AFTER `request_received_at`,
  ADD COLUMN `claimed_at` datetime(6) NULL AFTER `accepted_at`,
  ADD COLUMN `claim_source` varchar(16) NOT NULL DEFAULT '' AFTER `claimed_at`,
  ADD CONSTRAINT `chk_ai_reply_claim_source`
    CHECK (`claim_source` IN ('','wake','poll','recovery'));

ALTER TABLE `ai_provider_attempts`
  ADD COLUMN `prepare_started_at` datetime(6) NULL AFTER `error_code`,
  ADD COLUMN `first_delta_at` datetime(6) NULL AFTER `dispatched_at`;

ALTER TABLE `ai_runs`
  ADD COLUMN `settled_at` datetime(6) NULL AFTER `finished_at`;
