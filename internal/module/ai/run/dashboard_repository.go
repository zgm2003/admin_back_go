package airun

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const dashboardFilteredRunColumns = `
r.id,
r.created_at,
r.started_at,
r.model_id,
r.model_display_name,
r.agent_id,
r.provider_id,
r.user_id,
r.status,
r.billing_status,
r.billing_reason,
r.prompt_tokens,
r.completion_tokens,
r.total_tokens,
r.duration_ms`

type DashboardQueryStage string

const (
	DashboardStageOverview     DashboardQueryStage = "overview"
	DashboardStagePerformance  DashboardQueryStage = "performance"
	DashboardStageTrend        DashboardQueryStage = "trend"
	DashboardStageAttributions DashboardQueryStage = "attributions"
	DashboardStageErrors       DashboardQueryStage = "errors"
	DashboardStageTools        DashboardQueryStage = "tools"
)

type DashboardQueryError struct {
	Stage DashboardQueryStage
	Err   error
}

func (e *DashboardQueryError) Error() string {
	if e == nil {
		return "AI run dashboard query failed"
	}
	return fmt.Sprintf("AI run dashboard %s query failed: %v", e.Stage, e.Err)
}

func (e *DashboardQueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func applyDashboardFilters(db *gorm.DB, query DashboardQuery) *gorm.DB {
	db = db.Where("r.created_at >= ? AND r.created_at < ?", query.StartAt, query.EndExclusive)
	if query.Platform != "" {
		db = db.Where("r.platform = ?", query.Platform)
	}
	if query.ModelID != "" {
		db = db.Where("r.model_id = ?", query.ModelID)
	}
	if query.AgentID != nil {
		db = db.Where("r.agent_id = ?", *query.AgentID)
	}
	if query.ProviderID != nil {
		db = db.Where("r.provider_id = ?", *query.ProviderID)
	}
	if query.UserID != nil {
		db = db.Where("r.user_id = ?", *query.UserID)
	}
	return db
}

func dashboardRunAnomalyCaseSQL() string {
	return `CASE
WHEN r.status = 'failed' THEN 'failed'
WHEN r.status = 'timeout' THEN 'timeout'
WHEN r.status = 'outcome_unknown' THEN 'outcome_unknown'
WHEN r.status = 'running' AND r.started_at IS NOT NULL AND r.started_at < ? THEN 'stale_running'
ELSE NULL
END`
}

func dashboardBillingAnomalyCaseSQL() string {
	return `CASE
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
WHEN (
    r.status IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')
    AND r.billing_status IN ('pending', 'held')
    AND charge.status = 'open'
    AND charge.finalized_at IS NULL
  )
  OR (
    r.status = 'running'
    AND r.started_at IS NOT NULL
    AND r.started_at < ?
    AND r.billing_status IN ('pending', 'held')
    AND charge.status = 'open'
    AND charge.finalized_at IS NULL
  )
  THEN 'open_overdue'
WHEN r.billing_reason <> 'legacy_unpriced'
  AND r.billing_status <> 'released'
  AND (charge.pricing_version IS NULL OR TRIM(charge.pricing_version) = '')
  THEN 'pricing_snapshot_missing'
WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'legacy_unpriced'
  THEN 'legacy_unpriced'
WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'unbilled_usage_incomplete'
  THEN 'unbilled_usage_incomplete'
WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'unbilled_over_hold'
  THEN 'unbilled_over_hold'
ELSE NULL
END`
}

func dashboardOverviewSQL() string {
	return fmt.Sprintf(`
WITH filtered_runs AS (?),
classified_runs AS (
  SELECT
    r.id,
    r.status,
    r.billing_status,
    r.billing_reason,
    r.prompt_tokens,
    r.completion_tokens,
    r.total_tokens,
    charge.status AS charge_status,
    charge.held_units,
    charge.actual_units,
    charge.finalized_at,
    %s AS run_anomaly,
    %s AS billing_anomaly
  FROM filtered_runs r
  LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id
),
overview AS (
  SELECT
    COUNT(*) AS total_runs,
    COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0) AS running_runs,
    COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
    COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
    COALESCE(SUM(CASE WHEN status = 'canceled' THEN 1 ELSE 0 END), 0) AS canceled_runs,
    COALESCE(SUM(CASE WHEN status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_runs,
    COALESCE(SUM(CASE WHEN status = 'outcome_unknown' THEN 1 ELSE 0 END), 0) AS outcome_unknown_runs,
    COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
    COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(CASE
      WHEN billing_status = 'settled'
       AND billing_reason = 'settled_complete_usage'
       AND charge_status = 'settled'
       AND finalized_at IS NOT NULL
       AND (billing_anomaly IS NULL OR billing_anomaly <> 'state_inconsistent')
      THEN 1 ELSE 0 END), 0) AS settled_runs,
    COALESCE(SUM(CASE
      WHEN billing_status = 'settled'
       AND billing_reason = 'settled_complete_usage'
       AND charge_status = 'settled'
       AND finalized_at IS NOT NULL
       AND (billing_anomaly IS NULL OR billing_anomaly <> 'state_inconsistent')
      THEN actual_units ELSE 0 END), 0) AS actual_units,
    COALESCE(SUM(CASE
      WHEN billing_status = 'released'
       AND billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown')
       AND charge_status = 'released'
       AND finalized_at IS NOT NULL
       AND (billing_anomaly IS NULL OR billing_anomaly <> 'state_inconsistent')
      THEN 1 ELSE 0 END), 0) AS released_runs,
    COALESCE(SUM(CASE
      WHEN billing_status = 'released'
       AND billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown')
       AND charge_status = 'released'
       AND finalized_at IS NOT NULL
       AND (billing_anomaly IS NULL OR billing_anomaly <> 'state_inconsistent')
      THEN held_units ELSE 0 END), 0) AS released_units,
    COALESCE(SUM(CASE WHEN billing_status = 'unbilled' THEN 1 ELSE 0 END), 0) AS unbilled_runs
  FROM classified_runs
)
SELECT
  'summary' AS row_type,
  '' AS code,
  0 AS count_value,
  total_runs,
  running_runs,
  success_runs,
  failed_runs,
  canceled_runs,
  timeout_runs,
  outcome_unknown_runs,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  settled_runs,
  actual_units,
  released_runs,
  released_units,
  unbilled_runs
FROM overview
UNION ALL
SELECT
  'run_anomaly',
  run_anomaly,
  COUNT(*),
  0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
FROM classified_runs
WHERE run_anomaly IS NOT NULL
GROUP BY run_anomaly
UNION ALL
SELECT
  'billing_anomaly',
  billing_anomaly,
  COUNT(*),
  0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
FROM classified_runs
WHERE billing_anomaly IS NOT NULL
GROUP BY billing_anomaly`, dashboardRunAnomalyCaseSQL(), dashboardBillingAnomalyCaseSQL())
}

func dashboardPerformanceSQL() string {
	return `
WITH filtered_runs AS (?),
ranked_attempts AS (
  SELECT
    attempt.run_id,
    ROW_NUMBER() OVER (PARTITION BY attempt.run_id ORDER BY attempt.attempt_no DESC, attempt.id DESC) AS final_rank,
    CASE
      WHEN attempt.state = 'succeeded'
       AND attempt.dispatched_at IS NOT NULL
       AND attempt.first_delta_at IS NOT NULL
       AND attempt.first_delta_at >= attempt.dispatched_at
      THEN TIMESTAMPDIFF(MICROSECOND, attempt.dispatched_at, attempt.first_delta_at) DIV 1000
      ELSE NULL
    END AS ttft_ms
  FROM filtered_runs r
  JOIN ai_provider_attempts attempt ON attempt.run_id = r.id
  WHERE r.status = 'success'
),
performance_samples AS (
  SELECT 'ttft' AS metric, ttft_ms AS value_ms
  FROM ranked_attempts
  WHERE final_rank = 1 AND ttft_ms IS NOT NULL AND ttft_ms >= 0
  UNION ALL
  SELECT 'end_to_end', r.duration_ms
  FROM filtered_runs r
  WHERE r.status = 'success' AND r.duration_ms IS NOT NULL AND r.duration_ms >= 0
),
ranked_samples AS (
  SELECT
    metric,
    value_ms,
    ROW_NUMBER() OVER (PARTITION BY metric ORDER BY value_ms ASC) AS sample_rank,
    COUNT(*) OVER (PARTITION BY metric) AS sample_count
  FROM performance_samples
)
SELECT
  metric,
  sample_count,
  MAX(CASE WHEN sample_rank = CEIL(0.50 * sample_count) THEN value_ms END) AS p50_ms,
  MAX(CASE WHEN sample_rank = CEIL(0.95 * sample_count) THEN value_ms END) AS p95_ms
FROM ranked_samples
GROUP BY metric, sample_count
ORDER BY metric ASC`
}

func dashboardTrendSQL() string {
	return `
WITH filtered_runs AS (?),
daily_runs AS (
  SELECT
    DATE(r.created_at) AS run_date,
    COUNT(*) AS total_runs,
    COALESCE(SUM(CASE WHEN r.status = 'running' THEN 1 ELSE 0 END), 0) AS running_runs,
    COALESCE(SUM(CASE WHEN r.status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
    COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
    COALESCE(SUM(CASE WHEN r.status = 'canceled' THEN 1 ELSE 0 END), 0) AS canceled_runs,
    COALESCE(SUM(CASE WHEN r.status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_runs,
    COALESCE(SUM(CASE WHEN r.status = 'outcome_unknown' THEN 1 ELSE 0 END), 0) AS outcome_unknown_runs,
    COALESCE(SUM(CASE
      WHEN r.billing_status = 'settled'
       AND r.billing_reason = 'settled_complete_usage'
       AND charge.status = 'settled' AND charge.finalized_at IS NOT NULL
       AND r.status <> 'running'
      THEN charge.actual_units ELSE 0 END), 0) AS actual_units
  FROM filtered_runs r
  LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id
  GROUP BY DATE(r.created_at)
),
ranked_attempts AS (
  SELECT
    DATE(r.created_at) AS run_date,
    attempt.run_id,
    ROW_NUMBER() OVER (PARTITION BY attempt.run_id ORDER BY attempt.attempt_no DESC, attempt.id DESC) AS final_rank,
    CASE
      WHEN attempt.state = 'succeeded'
       AND attempt.dispatched_at IS NOT NULL
       AND attempt.first_delta_at IS NOT NULL
       AND attempt.first_delta_at >= attempt.dispatched_at
      THEN TIMESTAMPDIFF(MICROSECOND, attempt.dispatched_at, attempt.first_delta_at) DIV 1000
      ELSE NULL
    END AS ttft_ms
  FROM filtered_runs r
  JOIN ai_provider_attempts attempt ON attempt.run_id = r.id
  WHERE r.status = 'success'
),
trend_samples AS (
  SELECT run_date, 'ttft' AS metric, ttft_ms AS value_ms
  FROM ranked_attempts
  WHERE final_rank = 1 AND ttft_ms IS NOT NULL AND ttft_ms >= 0
  UNION ALL
  SELECT DATE(r.created_at), 'end_to_end', r.duration_ms
  FROM filtered_runs r
  WHERE r.status = 'success' AND r.duration_ms IS NOT NULL AND r.duration_ms >= 0
),
ranked_trend_samples AS (
  SELECT
    run_date,
    metric,
    value_ms,
    ROW_NUMBER() OVER (PARTITION BY run_date, metric ORDER BY value_ms ASC) AS sample_rank,
    COUNT(*) OVER (PARTITION BY run_date, metric) AS sample_count
  FROM trend_samples
),
trend_percentiles AS (
  SELECT
    run_date,
    metric,
    sample_count,
    MAX(CASE WHEN sample_rank = CEIL(0.50 * sample_count) THEN value_ms END) AS p50_ms,
    MAX(CASE WHEN sample_rank = CEIL(0.95 * sample_count) THEN value_ms END) AS p95_ms
  FROM ranked_trend_samples
  GROUP BY run_date, metric, sample_count
),
trend_performance AS (
  SELECT
    run_date,
    COALESCE(MAX(CASE WHEN metric = 'ttft' THEN sample_count END), 0) AS ttft_sample_count,
    COALESCE(MAX(CASE WHEN metric = 'ttft' THEN p50_ms END), 0) AS ttft_p50_ms,
    COALESCE(MAX(CASE WHEN metric = 'ttft' THEN p95_ms END), 0) AS ttft_p95_ms,
    COALESCE(MAX(CASE WHEN metric = 'end_to_end' THEN sample_count END), 0) AS end_to_end_sample_count,
    COALESCE(MAX(CASE WHEN metric = 'end_to_end' THEN p50_ms END), 0) AS end_to_end_p50_ms,
    COALESCE(MAX(CASE WHEN metric = 'end_to_end' THEN p95_ms END), 0) AS end_to_end_p95_ms
  FROM trend_percentiles
  GROUP BY run_date
)
SELECT
  DATE_FORMAT(daily_runs.run_date, '%Y-%m-%d') AS date,
  daily_runs.total_runs,
  daily_runs.running_runs,
  daily_runs.success_runs,
  daily_runs.failed_runs,
  daily_runs.canceled_runs,
  daily_runs.timeout_runs,
  daily_runs.outcome_unknown_runs,
  daily_runs.actual_units,
  COALESCE(trend_performance.ttft_sample_count, 0) AS ttft_sample_count,
  COALESCE(trend_performance.ttft_p50_ms, 0) AS ttft_p50_ms,
  COALESCE(trend_performance.ttft_p95_ms, 0) AS ttft_p95_ms,
  COALESCE(trend_performance.end_to_end_sample_count, 0) AS end_to_end_sample_count,
  COALESCE(trend_performance.end_to_end_p50_ms, 0) AS end_to_end_p50_ms,
  COALESCE(trend_performance.end_to_end_p95_ms, 0) AS end_to_end_p95_ms
FROM daily_runs
LEFT JOIN trend_performance ON trend_performance.run_date = daily_runs.run_date
ORDER BY daily_runs.run_date ASC
LIMIT 90`
}

func dashboardAttributionsSQL() string {
	return fmt.Sprintf(`
WITH filtered_runs AS (?),
ranked_runs AS (
  SELECT
    r.id,
    r.created_at,
    r.started_at,
    r.model_id,
    r.model_display_name,
    r.agent_id,
    r.provider_id,
    r.user_id,
    r.status,
    r.billing_status,
    r.billing_reason,
    r.total_tokens,
    ROW_NUMBER() OVER (PARTITION BY r.model_id ORDER BY r.created_at DESC, r.id DESC) AS model_name_rank
  FROM filtered_runs r
),
classified_runs AS (
  SELECT
    r.id,
    r.created_at,
    r.started_at,
    r.model_id,
    r.model_display_name,
    r.model_name_rank,
    r.agent_id,
    r.provider_id,
    r.user_id,
    r.status,
    r.billing_status,
    r.billing_reason,
    r.total_tokens,
    charge.status AS charge_status,
    charge.actual_units AS charge_actual_units,
    charge.finalized_at,
    %s AS run_anomaly,
    %s AS billing_anomaly
  FROM ranked_runs r
  LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id
),
model_attributions AS (
  SELECT
    'model' AS dimension,
    r.model_id AS stable_key,
    0 AS attribution_id,
    COALESCE(MAX(CASE WHEN r.model_name_rank = 1 THEN r.model_display_name END), '') AS attribution_name,
    COUNT(*) AS total_runs,
    COALESCE(SUM(CASE WHEN r.status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
    COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
    COALESCE(SUM(CASE WHEN r.status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_runs,
    COALESCE(SUM(CASE WHEN r.status = 'outcome_unknown' THEN 1 ELSE 0 END), 0) AS outcome_unknown_runs,
    COALESCE(SUM(r.total_tokens), 0) AS total_tokens,
    COALESCE(SUM(CASE
      WHEN r.billing_status = 'settled'
       AND r.billing_reason = 'settled_complete_usage'
       AND r.charge_status = 'settled'
       AND r.finalized_at IS NOT NULL
       AND (r.billing_anomaly IS NULL OR r.billing_anomaly <> 'state_inconsistent')
      THEN r.charge_actual_units ELSE 0 END), 0) AS actual_units,
    COALESCE(SUM(CASE WHEN r.run_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS run_anomaly_count,
    COALESCE(SUM(CASE WHEN r.billing_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS billing_anomaly_count
  FROM classified_runs r
  GROUP BY r.model_id
  ORDER BY actual_units DESC, total_runs DESC, stable_key ASC LIMIT 20
),
provider_attributions AS (
  SELECT
    'provider' AS dimension,
    CAST(r.provider_id AS CHAR) AS stable_key,
    r.provider_id AS attribution_id,
    COALESCE(MAX(provider.name), '') AS attribution_name,
    COUNT(*) AS total_runs,
    COALESCE(SUM(CASE WHEN r.status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
    COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
    COALESCE(SUM(CASE WHEN r.status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_runs,
    COALESCE(SUM(CASE WHEN r.status = 'outcome_unknown' THEN 1 ELSE 0 END), 0) AS outcome_unknown_runs,
    COALESCE(SUM(r.total_tokens), 0) AS total_tokens,
    COALESCE(SUM(CASE
      WHEN r.billing_status = 'settled'
       AND r.billing_reason = 'settled_complete_usage'
       AND r.charge_status = 'settled'
       AND r.finalized_at IS NOT NULL
       AND (r.billing_anomaly IS NULL OR r.billing_anomaly <> 'state_inconsistent')
      THEN r.charge_actual_units ELSE 0 END), 0) AS actual_units,
    COALESCE(SUM(CASE WHEN r.run_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS run_anomaly_count,
    COALESCE(SUM(CASE WHEN r.billing_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS billing_anomaly_count
  FROM classified_runs r
  LEFT JOIN ai_providers provider ON provider.id = r.provider_id
  GROUP BY r.provider_id
  ORDER BY actual_units DESC, total_runs DESC, stable_key ASC LIMIT 20
),
agent_attributions AS (
  SELECT
    'agent' AS dimension,
    CAST(r.agent_id AS CHAR) AS stable_key,
    r.agent_id AS attribution_id,
    COALESCE(MAX(agent.name), '') AS attribution_name,
    COUNT(*) AS total_runs,
    COALESCE(SUM(CASE WHEN r.status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
    COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
    COALESCE(SUM(CASE WHEN r.status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_runs,
    COALESCE(SUM(CASE WHEN r.status = 'outcome_unknown' THEN 1 ELSE 0 END), 0) AS outcome_unknown_runs,
    COALESCE(SUM(r.total_tokens), 0) AS total_tokens,
    COALESCE(SUM(CASE
      WHEN r.billing_status = 'settled'
       AND r.billing_reason = 'settled_complete_usage'
       AND r.charge_status = 'settled'
       AND r.finalized_at IS NOT NULL
       AND (r.billing_anomaly IS NULL OR r.billing_anomaly <> 'state_inconsistent')
      THEN r.charge_actual_units ELSE 0 END), 0) AS actual_units,
    COALESCE(SUM(CASE WHEN r.run_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS run_anomaly_count,
    COALESCE(SUM(CASE WHEN r.billing_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS billing_anomaly_count
  FROM classified_runs r
  LEFT JOIN ai_agents agent ON agent.id = r.agent_id
  GROUP BY r.agent_id
  ORDER BY actual_units DESC, total_runs DESC, stable_key ASC LIMIT 20
),
user_attributions AS (
  SELECT
    'user' AS dimension,
    CAST(r.user_id AS CHAR) AS stable_key,
    r.user_id AS attribution_id,
    COALESCE(MAX(user_row.username), '') AS attribution_name,
    COUNT(*) AS total_runs,
    COALESCE(SUM(CASE WHEN r.status = 'success' THEN 1 ELSE 0 END), 0) AS success_runs,
    COALESCE(SUM(CASE WHEN r.status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_runs,
    COALESCE(SUM(CASE WHEN r.status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_runs,
    COALESCE(SUM(CASE WHEN r.status = 'outcome_unknown' THEN 1 ELSE 0 END), 0) AS outcome_unknown_runs,
    COALESCE(SUM(r.total_tokens), 0) AS total_tokens,
    COALESCE(SUM(CASE
      WHEN r.billing_status = 'settled'
       AND r.billing_reason = 'settled_complete_usage'
       AND r.charge_status = 'settled'
       AND r.finalized_at IS NOT NULL
       AND (r.billing_anomaly IS NULL OR r.billing_anomaly <> 'state_inconsistent')
      THEN r.charge_actual_units ELSE 0 END), 0) AS actual_units,
    COALESCE(SUM(CASE WHEN r.run_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS run_anomaly_count,
    COALESCE(SUM(CASE WHEN r.billing_anomaly IS NOT NULL THEN 1 ELSE 0 END), 0) AS billing_anomaly_count
  FROM classified_runs r
  LEFT JOIN users user_row ON user_row.id = r.user_id
  GROUP BY r.user_id
  ORDER BY actual_units DESC, total_runs DESC, stable_key ASC LIMIT 20
)
SELECT
  dimension,
  stable_key AS attribution_key,
  attribution_id AS id,
  attribution_name AS name,
  total_runs,
  success_runs,
  failed_runs,
  timeout_runs,
  outcome_unknown_runs,
  total_tokens,
  actual_units,
  run_anomaly_count,
  billing_anomaly_count
FROM (
  SELECT dimension, stable_key, attribution_id, attribution_name, total_runs, success_runs, failed_runs, timeout_runs,
    outcome_unknown_runs, total_tokens, actual_units, run_anomaly_count, billing_anomaly_count
  FROM model_attributions
  UNION ALL
  SELECT dimension, stable_key, attribution_id, attribution_name, total_runs, success_runs, failed_runs, timeout_runs,
    outcome_unknown_runs, total_tokens, actual_units, run_anomaly_count, billing_anomaly_count
  FROM provider_attributions
  UNION ALL
  SELECT dimension, stable_key, attribution_id, attribution_name, total_runs, success_runs, failed_runs, timeout_runs,
    outcome_unknown_runs, total_tokens, actual_units, run_anomaly_count, billing_anomaly_count
  FROM agent_attributions
  UNION ALL
  SELECT dimension, stable_key, attribution_id, attribution_name, total_runs, success_runs, failed_runs, timeout_runs,
    outcome_unknown_runs, total_tokens, actual_units, run_anomaly_count, billing_anomaly_count
  FROM user_attributions
) attribution_rows
ORDER BY FIELD(dimension, 'model', 'provider', 'agent', 'user'), actual_units DESC, total_runs DESC, stable_key ASC`, dashboardRunAnomalyCaseSQL(), dashboardBillingAnomalyCaseSQL())
}

func dashboardErrorsSQL() string {
	return `
WITH filtered_runs AS (?),
ranked_terminal_attempts AS (
  SELECT
    attempt.run_id,
    attempt.error_code,
    ROW_NUMBER() OVER (PARTITION BY attempt.run_id ORDER BY attempt.attempt_no DESC, attempt.id DESC) AS final_rank
  FROM filtered_runs r
  JOIN ai_provider_attempts attempt ON attempt.run_id = r.id
  WHERE r.status IN ('failed', 'timeout', 'outcome_unknown')
    AND attempt.state IN ('succeeded', 'failed', 'canceled', 'outcome_unknown')
)
SELECT
  COALESCE(NULLIF(TRIM(error_code), ''), 'unclassified') AS error_code,
  COUNT(*) AS count
FROM ranked_terminal_attempts
WHERE final_rank = 1
GROUP BY COALESCE(NULLIF(TRIM(error_code), ''), 'unclassified')
ORDER BY count DESC, error_code ASC
LIMIT 20`
}

func dashboardToolsSQL() string {
	return `
WITH filtered_runs AS (?),
filtered_tool_calls AS (
  SELECT
    tool_call.id,
    tool_call.tool_code,
    tool_call.tool_name,
    tool_call.status,
    tool_call.duration_ms,
    tool_call.started_at,
    ROW_NUMBER() OVER (PARTITION BY tool_call.tool_code ORDER BY tool_call.started_at DESC, tool_call.id DESC) AS tool_name_rank
  FROM filtered_runs r
  JOIN ai_tool_calls tool_call ON tool_call.run_id = r.id
),
tool_totals AS (
  SELECT
    tool_code,
    COALESCE(MAX(CASE WHEN tool_name_rank = 1 THEN tool_name END), '') AS tool_name,
    COUNT(*) AS total_calls,
    COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS success_calls,
    COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_calls,
    COALESCE(SUM(CASE WHEN status = 'timeout' THEN 1 ELSE 0 END), 0) AS timeout_calls
  FROM filtered_tool_calls
  GROUP BY tool_code
),
successful_duration_samples AS (
  SELECT tool_code, duration_ms
  FROM filtered_tool_calls
  WHERE status = 'success' AND duration_ms IS NOT NULL AND duration_ms >= 0
),
ranked_duration_samples AS (
  SELECT
    tool_code,
    duration_ms,
    ROW_NUMBER() OVER (PARTITION BY tool_code ORDER BY duration_ms ASC) AS sample_rank,
    COUNT(*) OVER (PARTITION BY tool_code) AS sample_count
  FROM successful_duration_samples
),
tool_duration_percentiles AS (
  SELECT
    tool_code,
    sample_count,
    MAX(CASE WHEN sample_rank = CEIL(0.50 * sample_count) THEN duration_ms END) AS p50_ms,
    MAX(CASE WHEN sample_rank = CEIL(0.95 * sample_count) THEN duration_ms END) AS p95_ms
  FROM ranked_duration_samples
  GROUP BY tool_code, sample_count
)
SELECT
  tool_totals.tool_code,
  tool_totals.tool_name,
  tool_totals.total_calls,
  tool_totals.success_calls,
  tool_totals.failed_calls,
  tool_totals.timeout_calls,
  COALESCE(tool_duration_percentiles.sample_count, 0) AS duration_sample_count,
  COALESCE(tool_duration_percentiles.p50_ms, 0) AS duration_p50_ms,
  COALESCE(tool_duration_percentiles.p95_ms, 0) AS duration_p95_ms
FROM tool_totals
LEFT JOIN tool_duration_percentiles ON tool_duration_percentiles.tool_code = tool_totals.tool_code
ORDER BY total_calls DESC, tool_code ASC
LIMIT 20`
}

func dashboardOverviewQuery(db *gorm.DB, query DashboardQuery) *gorm.DB {
	return db.Raw(
		dashboardOverviewSQL(),
		dashboardFilteredRuns(db, query),
		query.StaleBefore,
		query.StaleBefore,
	)
}

func dashboardPerformanceQuery(db *gorm.DB, query DashboardQuery) *gorm.DB {
	return db.Raw(dashboardPerformanceSQL(), dashboardFilteredRuns(db, query))
}

func dashboardTrendQuery(db *gorm.DB, query DashboardQuery) *gorm.DB {
	return db.Raw(dashboardTrendSQL(), dashboardFilteredRuns(db, query))
}

func dashboardAttributionsQuery(db *gorm.DB, query DashboardQuery) *gorm.DB {
	return db.Raw(
		dashboardAttributionsSQL(),
		dashboardFilteredRuns(db, query),
		query.StaleBefore,
		query.StaleBefore,
	)
}

func dashboardErrorsQuery(db *gorm.DB, query DashboardQuery) *gorm.DB {
	return db.Raw(dashboardErrorsSQL(), dashboardFilteredRuns(db, query))
}

func dashboardToolsQuery(db *gorm.DB, query DashboardQuery) *gorm.DB {
	return db.Raw(dashboardToolsSQL(), dashboardFilteredRuns(db, query))
}

func (r *GormRepository) Dashboard(ctx context.Context, query DashboardQuery) (DashboardRepositoryResult, error) {
	if r == nil || r.db == nil {
		return DashboardRepositoryResult{}, ErrRepositoryNotConfigured
	}
	var result DashboardRepositoryResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return scanDashboardQueries(tx, query, &result)
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return DashboardRepositoryResult{}, err
	}
	return result, nil
}

func scanDashboardQueries(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error {
	stages := []struct {
		code DashboardQueryStage
		scan func() error
	}{
		{code: DashboardStageOverview, scan: func() error { return scanDashboardOverview(tx, query, result) }},
		{code: DashboardStagePerformance, scan: func() error { return scanDashboardPerformance(tx, query, result) }},
		{code: DashboardStageTrend, scan: func() error { return scanDashboardTrend(tx, query, result) }},
		{code: DashboardStageAttributions, scan: func() error { return scanDashboardAttributions(tx, query, result) }},
		{code: DashboardStageErrors, scan: func() error { return dashboardErrorsQuery(tx, query).Scan(&result.Errors).Error }},
		{code: DashboardStageTools, scan: func() error { return scanDashboardTools(tx, query, result) }},
	}
	for _, stage := range stages {
		if err := stage.scan(); err != nil {
			return &DashboardQueryError{Stage: stage.code, Err: err}
		}
	}
	return nil
}

type dashboardOverviewScanRow struct {
	RowType            string
	Code               string
	CountValue         int64
	TotalRuns          int64
	RunningRuns        int64
	SuccessRuns        int64
	FailedRuns         int64
	CanceledRuns       int64
	TimeoutRuns        int64
	OutcomeUnknownRuns int64
	PromptTokens       int64
	CompletionTokens   int64
	TotalTokens        int64
	SettledRuns        int64
	ActualUnits        int64
	ReleasedRuns       int64
	ReleasedUnits      int64
	UnbilledRuns       int64
}

func scanDashboardOverview(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error {
	var rows []dashboardOverviewScanRow
	if err := dashboardOverviewQuery(tx, query).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		switch row.RowType {
		case "summary":
			result.Summary = DashboardSummaryRow{
				TotalRuns: row.TotalRuns, RunningRuns: row.RunningRuns, SuccessRuns: row.SuccessRuns,
				FailedRuns: row.FailedRuns, CanceledRuns: row.CanceledRuns, TimeoutRuns: row.TimeoutRuns,
				OutcomeUnknownRuns: row.OutcomeUnknownRuns, PromptTokens: row.PromptTokens,
				CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
			}
			result.Billing = DashboardBillingRow{
				SettledRuns: row.SettledRuns, ActualUnits: row.ActualUnits, ReleasedRuns: row.ReleasedRuns,
				ReleasedUnits: row.ReleasedUnits, UnbilledRuns: row.UnbilledRuns,
			}
		case "run_anomaly":
			result.RunAnomalies = append(result.RunAnomalies, DashboardCountRow{Code: row.Code, Count: row.CountValue})
		case "billing_anomaly":
			result.BillingAnomalies = append(result.BillingAnomalies, DashboardCountRow{Code: row.Code, Count: row.CountValue})
		default:
			return fmt.Errorf("unsupported dashboard overview row type %q", row.RowType)
		}
	}
	return nil
}

type dashboardDistributionScanRow struct {
	Metric      string
	SampleCount int64
	P50MS       int64
	P95MS       int64
}

func scanDashboardPerformance(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error {
	var rows []dashboardDistributionScanRow
	if err := dashboardPerformanceQuery(tx, query).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		distribution := DashboardDistributionRow{SampleCount: row.SampleCount, P50MS: row.P50MS, P95MS: row.P95MS}
		switch row.Metric {
		case "ttft":
			result.Performance.TTFT = distribution
		case "end_to_end":
			result.Performance.EndToEnd = distribution
		default:
			return fmt.Errorf("unsupported dashboard performance metric %q", row.Metric)
		}
	}
	return nil
}

type dashboardTrendScanRow struct {
	Date                string
	TotalRuns           int64
	RunningRuns         int64
	SuccessRuns         int64
	FailedRuns          int64
	CanceledRuns        int64
	TimeoutRuns         int64
	OutcomeUnknownRuns  int64
	ActualUnits         int64
	TTFTSampleCount     int64
	TTFTP50MS           int64 `gorm:"column:ttft_p50_ms"`
	TTFTP95MS           int64 `gorm:"column:ttft_p95_ms"`
	EndToEndSampleCount int64
	EndToEndP50MS       int64
	EndToEndP95MS       int64
}

func scanDashboardTrend(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error {
	var rows []dashboardTrendScanRow
	if err := dashboardTrendQuery(tx, query).Scan(&rows).Error; err != nil {
		return err
	}
	result.Trend = make([]DashboardTrendRow, 0, len(rows))
	for _, row := range rows {
		result.Trend = append(result.Trend, DashboardTrendRow{
			Date: row.Date, TotalRuns: row.TotalRuns, RunningRuns: row.RunningRuns, SuccessRuns: row.SuccessRuns,
			FailedRuns: row.FailedRuns, CanceledRuns: row.CanceledRuns, TimeoutRuns: row.TimeoutRuns,
			OutcomeUnknownRuns: row.OutcomeUnknownRuns, ActualUnits: row.ActualUnits,
			TTFT: DashboardDistributionRow{SampleCount: row.TTFTSampleCount, P50MS: row.TTFTP50MS, P95MS: row.TTFTP95MS},
			EndToEnd: DashboardDistributionRow{
				SampleCount: row.EndToEndSampleCount, P50MS: row.EndToEndP50MS, P95MS: row.EndToEndP95MS,
			},
		})
	}
	return nil
}

type dashboardAttributionScanRow struct {
	Dimension           string
	AttributionKey      string
	ID                  int64
	Name                string
	TotalRuns           int64
	SuccessRuns         int64
	FailedRuns          int64
	TimeoutRuns         int64
	OutcomeUnknownRuns  int64
	TotalTokens         int64
	ActualUnits         int64
	RunAnomalyCount     int64
	BillingAnomalyCount int64
}

func scanDashboardAttributions(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error {
	var rows []dashboardAttributionScanRow
	if err := dashboardAttributionsQuery(tx, query).Scan(&rows).Error; err != nil {
		return err
	}
	result.Attributions = make([]DashboardAttributionRow, 0, len(rows))
	for _, row := range rows {
		result.Attributions = append(result.Attributions, DashboardAttributionRow{
			Dimension: row.Dimension, Key: row.AttributionKey, ID: row.ID, Name: row.Name, TotalRuns: row.TotalRuns,
			SuccessRuns: row.SuccessRuns, FailedRuns: row.FailedRuns, TimeoutRuns: row.TimeoutRuns,
			OutcomeUnknownRuns: row.OutcomeUnknownRuns, TotalTokens: row.TotalTokens, ActualUnits: row.ActualUnits,
			RunAnomalyCount: row.RunAnomalyCount, BillingAnomalyCount: row.BillingAnomalyCount,
		})
	}
	return nil
}

type dashboardToolScanRow struct {
	ToolCode            string
	ToolName            string
	TotalCalls          int64
	SuccessCalls        int64
	FailedCalls         int64
	TimeoutCalls        int64
	DurationSampleCount int64
	DurationP50MS       int64
	DurationP95MS       int64
}

func scanDashboardTools(tx *gorm.DB, query DashboardQuery, result *DashboardRepositoryResult) error {
	var rows []dashboardToolScanRow
	if err := dashboardToolsQuery(tx, query).Scan(&rows).Error; err != nil {
		return err
	}
	result.Tools = make([]DashboardToolRow, 0, len(rows))
	for _, row := range rows {
		result.Tools = append(result.Tools, DashboardToolRow{
			ToolCode: row.ToolCode, ToolName: row.ToolName, TotalCalls: row.TotalCalls,
			SuccessCalls: row.SuccessCalls, FailedCalls: row.FailedCalls, TimeoutCalls: row.TimeoutCalls,
			Duration: DashboardDistributionRow{
				SampleCount: row.DurationSampleCount, P50MS: row.DurationP50MS, P95MS: row.DurationP95MS,
			},
		})
	}
	return nil
}

func dashboardFilteredRuns(db *gorm.DB, query DashboardQuery) *gorm.DB {
	base := db.Session(&gorm.Session{NewDB: true}).Table("ai_runs r").Select(strings.TrimSpace(dashboardFilteredRunColumns))
	return applyDashboardFilters(base, query)
}
