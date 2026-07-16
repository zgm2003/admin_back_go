-- Accepted by before/after EXPLAIN ANALYZE evidence on the disposable restore.
CREATE INDEX `idx_user_sessions_user_platform_active_refresh`
  ON `user_sessions` (`user_id`,`platform`,`is_del`,`revoked_at`,`refresh_expires_at`,`id`);

CREATE INDEX `idx_ai_runs_status_started`
  ON `ai_runs` (`status`,`started_at`,`id`);

CREATE INDEX `idx_notifications_user_active_unread_platform`
  ON `notifications` (`user_id`,`is_del`,`is_read`,`platform`,`id`);

CREATE INDEX `idx_export_tasks_user_platform_active_id`
  ON `export_tasks` (`user_id`,`platform`,`is_del`,`id`);

CREATE INDEX `idx_cron_task_log_task_active_created`
  ON `cron_task_log` (`task_id`,`is_del`,`created_at` DESC,`id` DESC);
