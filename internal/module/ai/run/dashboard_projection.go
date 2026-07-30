package airun

import (
	"context"
	"errors"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const terminalDashboardFactExistsSQL = `
SELECT EXISTS (
  SELECT 1 FROM ai_run_dashboard_facts WHERE run_id = ?
) AS fact_exists`

const insertTerminalDashboardFactSQL = `
INSERT INTO ai_run_dashboard_facts (
  run_id, fact_date, run_created_at, platform, model_id, model_display_name,
  agent_id, provider_id, user_id, status, prompt_tokens, completion_tokens, total_tokens, duration_ms,
  settled_runs, actual_units, released_runs, released_units, unbilled_runs,
  run_anomaly_code, billing_anomaly_code, final_error_code, ttft_ms
)
SELECT
  r.id, DATE(r.created_at), r.created_at, r.platform, r.model_id, r.model_display_name,
  r.agent_id, r.provider_id, r.user_id, r.status, r.prompt_tokens, r.completion_tokens, r.total_tokens, r.duration_ms,
  CASE WHEN r.billing_status = 'settled' AND r.billing_reason = 'settled_complete_usage'
         AND charge.status = 'settled' AND charge.finalized_at IS NOT NULL
         AND billing_class.code <> 'state_inconsistent' THEN 1 ELSE 0 END,
  CASE WHEN r.billing_status = 'settled' AND r.billing_reason = 'settled_complete_usage'
         AND charge.status = 'settled' AND charge.finalized_at IS NOT NULL
         AND billing_class.code <> 'state_inconsistent' THEN charge.actual_units ELSE 0 END,
  CASE WHEN r.billing_status = 'released'
         AND r.billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown')
         AND charge.status = 'released' AND charge.finalized_at IS NOT NULL
         AND billing_class.code <> 'state_inconsistent' THEN 1 ELSE 0 END,
  CASE WHEN r.billing_status = 'released'
         AND r.billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown')
         AND charge.status = 'released' AND charge.finalized_at IS NOT NULL
         AND billing_class.code <> 'state_inconsistent' THEN charge.held_units ELSE 0 END,
  CASE WHEN r.billing_status = 'unbilled' THEN 1 ELSE 0 END,
  CASE r.status
    WHEN 'failed' THEN 'failed'
    WHEN 'timeout' THEN 'timeout'
    WHEN 'outcome_unknown' THEN 'outcome_unknown'
    ELSE ''
  END,
  billing_class.code,
  CASE WHEN r.status IN ('failed', 'timeout', 'outcome_unknown')
    THEN COALESCE(NULLIF(TRIM(final_attempt.error_code), ''), 'unclassified') ELSE '' END,
  CASE WHEN r.status = 'success' AND final_attempt.state = 'succeeded'
         AND final_attempt.dispatched_at IS NOT NULL AND final_attempt.first_delta_at IS NOT NULL
         AND final_attempt.first_delta_at >= final_attempt.dispatched_at
    THEN TIMESTAMPDIFF(MICROSECOND, final_attempt.dispatched_at, final_attempt.first_delta_at) DIV 1000 ELSE NULL END
FROM ai_runs r
LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id
LEFT JOIN ai_provider_attempts final_attempt ON final_attempt.id = (
  SELECT attempt.id FROM ai_provider_attempts attempt
  WHERE attempt.run_id = r.id AND attempt.state IN ('succeeded', 'failed', 'canceled', 'outcome_unknown')
  ORDER BY attempt.attempt_no DESC, attempt.id DESC LIMIT 1
)
CROSS JOIN LATERAL (
  SELECT CASE
    WHEN charge.id IS NULL
      OR NOT (
        (r.billing_status = 'pending' AND r.billing_reason = 'pending' AND charge.status = 'open' AND charge.finalized_at IS NULL)
        OR (r.billing_status = 'held' AND r.billing_reason = 'held' AND charge.status = 'open' AND charge.finalized_at IS NULL)
        OR (r.billing_status = 'settled' AND r.billing_reason = 'settled_complete_usage' AND charge.status = 'settled' AND charge.finalized_at IS NOT NULL)
        OR (r.billing_status = 'released' AND r.billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown') AND charge.status = 'released' AND charge.finalized_at IS NOT NULL)
        OR (r.billing_status = 'unbilled' AND r.billing_reason IN ('legacy_unpriced', 'unbilled_usage_incomplete', 'unbilled_over_hold') AND charge.status = 'unbilled' AND charge.finalized_at IS NOT NULL)
      )
      OR (r.status = 'running' AND r.billing_status IN ('settled', 'released', 'unbilled'))
      THEN 'state_inconsistent'
    WHEN r.status IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')
      AND r.billing_status IN ('pending', 'held') AND charge.status = 'open' AND charge.finalized_at IS NULL
      THEN 'open_overdue'
    WHEN r.billing_reason <> 'legacy_unpriced' AND r.billing_status <> 'released'
      AND (charge.pricing_version IS NULL OR TRIM(charge.pricing_version) = '')
      THEN 'pricing_snapshot_missing'
    WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'legacy_unpriced' THEN 'legacy_unpriced'
    WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'unbilled_usage_incomplete' THEN 'unbilled_usage_incomplete'
    WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'unbilled_over_hold' THEN 'unbilled_over_hold'
    ELSE ''
  END AS code
) billing_class
WHERE r.id = ? AND r.status IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')`

const incrementDailyDashboardFactSQL = `
INSERT INTO ai_run_dashboard_daily_facts (
  fact_date, platform, model_id, agent_id, provider_id, user_id, status, run_anomaly_code, billing_anomaly_code, final_error_code,
  latest_run_id, latest_model_display_name, run_count, prompt_tokens, completion_tokens, total_tokens,
  settled_runs, actual_units, released_runs, released_units, unbilled_runs
)
SELECT
  fact_date, platform, model_id, agent_id, provider_id, user_id, status, run_anomaly_code, billing_anomaly_code, final_error_code,
  run_id, model_display_name, 1, prompt_tokens, completion_tokens, total_tokens,
  settled_runs, actual_units, released_runs, released_units, unbilled_runs
FROM ai_run_dashboard_facts WHERE run_id = ?
ON DUPLICATE KEY UPDATE
  latest_model_display_name = CASE WHEN VALUES(latest_run_id) > ai_run_dashboard_daily_facts.latest_run_id THEN VALUES(latest_model_display_name) ELSE ai_run_dashboard_daily_facts.latest_model_display_name END,
  latest_run_id = GREATEST(ai_run_dashboard_daily_facts.latest_run_id, VALUES(latest_run_id)),
  run_count = ai_run_dashboard_daily_facts.run_count + VALUES(run_count),
  prompt_tokens = ai_run_dashboard_daily_facts.prompt_tokens + VALUES(prompt_tokens),
  completion_tokens = ai_run_dashboard_daily_facts.completion_tokens + VALUES(completion_tokens),
  total_tokens = ai_run_dashboard_daily_facts.total_tokens + VALUES(total_tokens),
  settled_runs = ai_run_dashboard_daily_facts.settled_runs + VALUES(settled_runs),
  actual_units = ai_run_dashboard_daily_facts.actual_units + VALUES(actual_units),
  released_runs = ai_run_dashboard_daily_facts.released_runs + VALUES(released_runs),
  released_units = ai_run_dashboard_daily_facts.released_units + VALUES(released_units),
  unbilled_runs = ai_run_dashboard_daily_facts.unbilled_runs + VALUES(unbilled_runs)`

// ProjectTerminalDashboardFacts persists the immutable terminal Run projection
// in the caller's transaction. Existing facts are the replay fence. A duplicate
// key can still occur when concurrent transactions both observe a missing fact;
// only that race is accepted, while all other database errors propagate.
func ProjectTerminalDashboardFacts(ctx context.Context, tx *gorm.DB, runID int64) error {
	if tx == nil || runID <= 0 {
		return errors.New("invalid terminal dashboard projection input")
	}
	var factExists bool
	if err := tx.WithContext(ctx).Raw(terminalDashboardFactExistsSQL, runID).Scan(&factExists).Error; err != nil {
		return err
	}
	if factExists {
		return nil
	}
	inserted := tx.WithContext(ctx).Exec(insertTerminalDashboardFactSQL, runID)
	if inserted.Error != nil {
		var mysqlErr *mysqldriver.MySQLError
		if errors.As(inserted.Error, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil
		}
		return inserted.Error
	}
	return tx.WithContext(ctx).Exec(incrementDailyDashboardFactSQL, runID).Error
}
