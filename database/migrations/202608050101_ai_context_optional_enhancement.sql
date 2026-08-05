ALTER TABLE `ai_context_plans`
  DROP CHECK `chk_ai_context_plans_retrieval_outcome`,
  DROP CHECK `chk_ai_context_plans_terminal_shape`,
  ADD CONSTRAINT `chk_ai_context_plans_retrieval_outcome`
    CHECK (`retrieval_outcome` IN ('skipped', 'no_hit', 'hit', 'degraded', 'failed')),
  ADD CONSTRAINT `chk_ai_context_plans_terminal_shape`
    CHECK (
      (`state` = 'ready' AND `plan_sha256` IS NOT NULL AND (
        (`retrieval_outcome` IN ('skipped', 'no_hit', 'hit')
          AND `error_stage` IS NULL AND `error_code` IS NULL AND `error_message` IS NULL)
        OR
        (`retrieval_outcome` = 'degraded'
          AND `error_stage` IS NOT NULL AND CHAR_LENGTH(`error_stage`) > 0
          AND `error_code` IS NOT NULL AND CHAR_LENGTH(`error_code`) > 0
          AND (`error_message` IS NULL OR CHAR_LENGTH(`error_message`) > 0))
      ))
      OR
      (`state` = 'failed' AND `plan_sha256` IS NULL AND `retrieval_outcome` = 'failed'
        AND `error_stage` IS NOT NULL AND CHAR_LENGTH(`error_stage`) > 0
        AND `error_code` IS NOT NULL AND CHAR_LENGTH(`error_code`) > 0
        AND (`error_message` IS NULL OR CHAR_LENGTH(`error_message`) > 0))
    );
