-- Preserve the extensible platform kernel while permanently rejecting the two
-- retired product codes. `all` remains a notification audience only.
ALTER TABLE `authz_principal_versions`
  DROP CHECK `chk_authz_principal_platform`;

ALTER TABLE `ai_reply_commands`
  DROP CHECK `chk_ai_reply_platform`;

ALTER TABLE `ai_video_tasks`
  DROP CHECK `chk_ai_video_platform`;

ALTER TABLE `auth_platforms`
  ADD CONSTRAINT `chk_auth_platforms_code`
  CHECK (
    `code` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `code` NOT IN ('app', 'canvas')
    AND `code` <> 'all'
  );

ALTER TABLE `permissions`
  ADD CONSTRAINT `chk_permissions_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `authz_principal_versions`
  ADD CONSTRAINT `chk_authz_principal_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `user_sessions`
  ADD CONSTRAINT `chk_user_sessions_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `users_login_log`
  ADD CONSTRAINT `chk_users_login_log_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `notification_task`
  ADD CONSTRAINT `chk_notification_task_platform`
  CHECK (
    `platform` = 'all'
    OR (
      `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
      AND `platform` NOT IN ('app', 'canvas')
      AND `platform` <> 'all'
    )
  );

ALTER TABLE `notifications`
  ADD CONSTRAINT `chk_notifications_platform`
  CHECK (
    `platform` = 'all'
    OR (
      `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
      AND `platform` NOT IN ('app', 'canvas')
      AND `platform` <> 'all'
    )
  );

ALTER TABLE `export_tasks`
  ADD CONSTRAINT `chk_export_tasks_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `ai_runs`
  ADD CONSTRAINT `chk_ai_runs_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `ai_text_tasks`
  ADD CONSTRAINT `chk_ai_text_tasks_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `ai_image_tasks`
  ADD CONSTRAINT `chk_ai_image_tasks_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `ai_video_tasks`
  ADD CONSTRAINT `chk_ai_video_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );

ALTER TABLE `ai_reply_commands`
  ADD CONSTRAINT `chk_ai_reply_platform`
  CHECK (
    `platform` REGEXP '^[a-z][a-z0-9_]{1,48}$'
    AND `platform` NOT IN ('app', 'canvas')
    AND `platform` <> 'all'
  );
