CREATE INDEX `idx_ai_runs_model_created`
  ON `ai_runs` (`model_id`, `created_at`, `id`);

CREATE INDEX `idx_ai_runs_billing_created`
  ON `ai_runs` (`billing_status`, `billing_reason`, `created_at`, `id`);

CREATE INDEX `idx_ai_provider_attempts_error_run`
  ON `ai_provider_attempts` (`error_code`, `run_id`, `id`);
