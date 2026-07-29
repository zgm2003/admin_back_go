package airun

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const dashboardFilteredRunColumns = `
r.id,
r.created_at,
r.started_at,
r.status,
r.billing_status,
r.billing_reason,
r.prompt_tokens,
r.completion_tokens,
r.total_tokens,
r.duration_ms`

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

func dashboardFilteredRuns(db *gorm.DB, query DashboardQuery) *gorm.DB {
	base := db.Session(&gorm.Session{NewDB: true}).Table("ai_runs r").Select(strings.TrimSpace(dashboardFilteredRunColumns))
	return applyDashboardFilters(base, query)
}
