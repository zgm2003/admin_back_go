ALTER TABLE `ai_run_events`
  DROP CHECK `chk_ai_run_events_type`,
  ADD CONSTRAINT `chk_ai_run_events_type` CHECK (`event_type` IN (
    'start',
    'completed',
    'failed',
    'canceled',
    'timeout',
    'retry_scheduled',
    'usage_recorded',
    'outcome_unknown',
    'settled',
    'released',
    'unbilled',
    'file_materialized_v1'
  ));
