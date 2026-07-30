CREATE TABLE `ai_run_dashboard_facts` (
  `run_id` BIGINT UNSIGNED NOT NULL,
  `fact_date` DATE NOT NULL,
  `run_created_at` DATETIME NOT NULL,
  `platform` VARCHAR(32) NOT NULL,
  `model_id` VARCHAR(191) NOT NULL,
  `model_display_name` VARCHAR(191) NOT NULL DEFAULT '',
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `prompt_tokens` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `completion_tokens` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `total_tokens` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `duration_ms` BIGINT UNSIGNED NULL,
  `settled_runs` TINYINT UNSIGNED NOT NULL DEFAULT 0,
  `actual_units` BIGINT NOT NULL DEFAULT 0,
  `released_runs` TINYINT UNSIGNED NOT NULL DEFAULT 0,
  `released_units` BIGINT NOT NULL DEFAULT 0,
  `unbilled_runs` TINYINT UNSIGNED NOT NULL DEFAULT 0,
  `run_anomaly_code` VARCHAR(32) NOT NULL DEFAULT '',
  `billing_anomaly_code` VARCHAR(32) NOT NULL DEFAULT '',
  `final_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `ttft_ms` BIGINT UNSIGNED NULL,
  PRIMARY KEY (`run_id`),
  KEY `idx_ai_run_dashboard_facts_created` (`fact_date`, `run_id`),
  KEY `idx_ai_run_dashboard_facts_status_created` (`status`, `fact_date`, `run_id`),
  KEY `idx_ai_run_dashboard_facts_model_created` (`model_id`, `fact_date`, `run_id`),
  KEY `idx_ai_run_dashboard_facts_platform_created` (`platform`, `fact_date`, `run_id`),
  KEY `idx_ai_run_dashboard_facts_agent_created` (`agent_id`, `fact_date`, `run_id`),
  KEY `idx_ai_run_dashboard_facts_provider_created` (`provider_id`, `fact_date`, `run_id`),
  KEY `idx_ai_run_dashboard_facts_user_created` (`user_id`, `fact_date`, `run_id`),
  CONSTRAINT `fk_ai_run_dashboard_facts_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_run_dashboard_facts_status` CHECK (`status` IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')),
  CONSTRAINT `chk_ai_run_dashboard_facts_nonnegative` CHECK (
    `actual_units` >= 0 AND `released_units` >= 0
    AND `settled_runs` BETWEEN 0 AND 1 AND `released_runs` BETWEEN 0 AND 1 AND `unbilled_runs` BETWEEN 0 AND 1
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='Immutable terminal Run projection for exact AI dashboard analytics';

CREATE TABLE `ai_run_dashboard_daily_facts` (
  `fact_date` DATE NOT NULL,
  `platform` VARCHAR(32) NOT NULL,
  `model_id` VARCHAR(191) NOT NULL,
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `run_anomaly_code` VARCHAR(32) NOT NULL DEFAULT '',
  `billing_anomaly_code` VARCHAR(32) NOT NULL DEFAULT '',
  `final_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `latest_run_id` BIGINT UNSIGNED NOT NULL,
  `latest_model_display_name` VARCHAR(191) NOT NULL DEFAULT '',
  `run_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `prompt_tokens` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `completion_tokens` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `total_tokens` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `settled_runs` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `actual_units` BIGINT NOT NULL DEFAULT 0,
  `released_runs` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `released_units` BIGINT NOT NULL DEFAULT 0,
  `unbilled_runs` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (
    `fact_date`, `platform`, `model_id`, `agent_id`, `provider_id`, `user_id`, `status`,
    `run_anomaly_code`, `billing_anomaly_code`, `final_error_code`
  ),
  KEY `idx_ai_run_dashboard_daily_model_date` (`model_id`, `fact_date`),
  KEY `idx_ai_run_dashboard_daily_platform_date` (`platform`, `fact_date`),
  KEY `idx_ai_run_dashboard_daily_provider_date` (`provider_id`, `fact_date`),
  KEY `idx_ai_run_dashboard_daily_agent_date` (`agent_id`, `fact_date`),
  KEY `idx_ai_run_dashboard_daily_user_date` (`user_id`, `fact_date`),
  KEY `idx_ai_run_dashboard_daily_error_date` (`final_error_code`, `fact_date`),
  CONSTRAINT `chk_ai_run_dashboard_daily_status` CHECK (`status` IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')),
  CONSTRAINT `chk_ai_run_dashboard_daily_nonnegative` CHECK (`actual_units` >= 0 AND `released_units` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='Daily terminal Run aggregate for bounded AI dashboard analytics';

INSERT INTO `ai_run_dashboard_facts` (
  `run_id`, `fact_date`, `run_created_at`, `platform`, `model_id`, `model_display_name`,
  `agent_id`, `provider_id`, `user_id`, `status`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `duration_ms`,
  `settled_runs`, `actual_units`, `released_runs`, `released_units`, `unbilled_runs`,
  `run_anomaly_code`, `billing_anomaly_code`, `final_error_code`, `ttft_ms`
)
SELECT
  classified.run_id, classified.fact_date, classified.run_created_at, classified.platform, classified.model_id, classified.model_display_name,
  classified.agent_id, classified.provider_id, classified.user_id, classified.status,
  classified.prompt_tokens, classified.completion_tokens, classified.total_tokens, classified.duration_ms,
  CASE WHEN classified.billing_status = 'settled' AND classified.billing_reason = 'settled_complete_usage'
         AND classified.charge_status = 'settled' AND classified.charge_finalized = 1
         AND classified.billing_anomaly_code <> 'state_inconsistent' THEN 1 ELSE 0 END,
  CASE WHEN classified.billing_status = 'settled' AND classified.billing_reason = 'settled_complete_usage'
         AND classified.charge_status = 'settled' AND classified.charge_finalized = 1
         AND classified.billing_anomaly_code <> 'state_inconsistent' THEN classified.charge_actual_units ELSE 0 END,
  CASE WHEN classified.billing_status = 'released'
         AND classified.billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown')
         AND classified.charge_status = 'released' AND classified.charge_finalized = 1
         AND classified.billing_anomaly_code <> 'state_inconsistent' THEN 1 ELSE 0 END,
  CASE WHEN classified.billing_status = 'released'
         AND classified.billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown')
         AND classified.charge_status = 'released' AND classified.charge_finalized = 1
         AND classified.billing_anomaly_code <> 'state_inconsistent' THEN classified.charge_held_units ELSE 0 END,
  CASE WHEN classified.billing_status = 'unbilled' THEN 1 ELSE 0 END,
  CASE classified.status WHEN 'failed' THEN 'failed' WHEN 'timeout' THEN 'timeout' WHEN 'outcome_unknown' THEN 'outcome_unknown' ELSE '' END,
  classified.billing_anomaly_code,
  CASE WHEN classified.status IN ('failed', 'timeout', 'outcome_unknown')
    THEN COALESCE(NULLIF(TRIM(classified.error_code), ''), 'unclassified') ELSE '' END,
  CASE WHEN classified.status = 'success' AND classified.attempt_state = 'succeeded'
         AND classified.dispatched_at IS NOT NULL AND classified.first_delta_at IS NOT NULL
         AND classified.first_delta_at >= classified.dispatched_at
    THEN TIMESTAMPDIFF(MICROSECOND, classified.dispatched_at, classified.first_delta_at) DIV 1000 ELSE NULL END
FROM (
  SELECT
    r.id AS run_id, DATE(r.created_at) AS fact_date, r.created_at AS run_created_at, r.platform, r.model_id, r.model_display_name,
    r.agent_id, r.provider_id, r.user_id, r.status, r.billing_status, r.billing_reason,
    r.prompt_tokens, r.completion_tokens, r.total_tokens, r.duration_ms,
    charge.status AS charge_status, charge.actual_units AS charge_actual_units, charge.held_units AS charge_held_units,
    charge.finalized_at IS NOT NULL AS charge_finalized, final_attempt.state AS attempt_state, final_attempt.error_code,
    final_attempt.dispatched_at, final_attempt.first_delta_at,
    CASE
      WHEN charge.id IS NULL
        OR NOT (
          (r.billing_status = 'pending' AND r.billing_reason = 'pending' AND charge.status = 'open' AND charge.finalized_at IS NULL)
          OR (r.billing_status = 'held' AND r.billing_reason = 'held' AND charge.status = 'open' AND charge.finalized_at IS NULL)
          OR (r.billing_status = 'settled' AND r.billing_reason = 'settled_complete_usage' AND charge.status = 'settled' AND charge.finalized_at IS NOT NULL)
          OR (r.billing_status = 'released' AND r.billing_reason IN ('released_before_dispatch', 'released_insufficient_balance', 'released_provider_failed', 'released_outcome_unknown') AND charge.status = 'released' AND charge.finalized_at IS NOT NULL)
          OR (r.billing_status = 'unbilled' AND r.billing_reason IN ('legacy_unpriced', 'unbilled_usage_incomplete', 'unbilled_over_hold') AND charge.status = 'unbilled' AND charge.finalized_at IS NOT NULL)
        )
        OR (r.status = 'running' AND r.billing_status IN ('settled', 'released', 'unbilled')) THEN 'state_inconsistent'
      WHEN r.status IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')
        AND r.billing_status IN ('pending', 'held') AND charge.status = 'open' AND charge.finalized_at IS NULL THEN 'open_overdue'
      WHEN r.billing_reason <> 'legacy_unpriced' AND r.billing_status <> 'released'
        AND (charge.pricing_version IS NULL OR TRIM(charge.pricing_version) = '') THEN 'pricing_snapshot_missing'
      WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'legacy_unpriced' THEN 'legacy_unpriced'
      WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'unbilled_usage_incomplete' THEN 'unbilled_usage_incomplete'
      WHEN r.billing_status = 'unbilled' AND r.billing_reason = 'unbilled_over_hold' THEN 'unbilled_over_hold'
      ELSE ''
    END AS billing_anomaly_code
  FROM ai_runs r
  LEFT JOIN ai_usage_charges charge ON charge.run_id = r.id
  LEFT JOIN ai_provider_attempts final_attempt ON final_attempt.id = (
    SELECT attempt.id FROM ai_provider_attempts attempt
    WHERE attempt.run_id = r.id AND attempt.state IN ('succeeded', 'failed', 'canceled', 'outcome_unknown')
    ORDER BY attempt.attempt_no DESC, attempt.id DESC LIMIT 1
  )
  WHERE r.status IN ('success', 'failed', 'canceled', 'timeout', 'outcome_unknown')
) classified;

INSERT INTO `ai_run_dashboard_daily_facts` (
  `fact_date`, `platform`, `model_id`, `agent_id`, `provider_id`, `user_id`, `status`,
  `run_anomaly_code`, `billing_anomaly_code`, `final_error_code`, `latest_run_id`, `latest_model_display_name`,
  `run_count`, `prompt_tokens`, `completion_tokens`, `total_tokens`,
  `settled_runs`, `actual_units`, `released_runs`, `released_units`, `unbilled_runs`
)
SELECT
  fact_date, platform, model_id, agent_id, provider_id, user_id, status,
  run_anomaly_code, billing_anomaly_code, final_error_code, MAX(run_id),
  SUBSTRING(MAX(CONCAT(LPAD(run_id, 20, '0'), model_display_name)), 21),
  COUNT(*), SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens),
  SUM(settled_runs), SUM(actual_units), SUM(released_runs), SUM(released_units), SUM(unbilled_runs)
FROM `ai_run_dashboard_facts`
GROUP BY fact_date, platform, model_id, agent_id, provider_id, user_id, status,
  run_anomaly_code, billing_anomaly_code, final_error_code;
