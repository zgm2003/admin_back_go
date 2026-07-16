SELECT 'unknown_platform_values' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('ai_image_task:',`id`) entity FROM `ai_image_tasks` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','canvas')
  UNION ALL
  SELECT CONCAT('ai_reply_command:',`id`) FROM `ai_reply_commands` WHERE `platform` IS NULL OR `platform`<>'admin'
  UNION ALL
  SELECT CONCAT('ai_run:',`id`) FROM `ai_runs` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','canvas')
  UNION ALL
  SELECT CONCAT('ai_text_task:',`id`) FROM `ai_text_tasks` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','canvas')
  UNION ALL
  SELECT CONCAT('ai_video_task:',`id`) FROM `ai_video_tasks` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','canvas')
  UNION ALL
  SELECT CONCAT('principal:',`user_id`) FROM `authz_principal_versions` WHERE `platform` IS NULL OR `platform`<>'admin'
  UNION ALL
  SELECT CONCAT('client_version:',`id`) FROM `client_versions` WHERE `platform` IS NULL OR `platform` NOT IN ('windows-x86_64','darwin-x86_64')
  UNION ALL
  SELECT CONCAT('export:',`id`) FROM `export_tasks` WHERE `platform` IS NULL OR `platform`<>'admin'
  UNION ALL
  SELECT CONCAT('notification_task:',`id`) FROM `notification_task` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','app','canvas','all')
  UNION ALL
  SELECT CONCAT('notification:',`id`) FROM `notifications` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','app','canvas','all')
  UNION ALL
  SELECT CONCAT('permission:',`id`) FROM `permissions` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','app','canvas')
  UNION ALL
  SELECT CONCAT('session:',`id`) FROM `user_sessions` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','app','canvas')
  UNION ALL
  SELECT CONCAT('login_log:',`id`) FROM `users_login_log` WHERE `platform` IS NULL OR `platform` NOT IN ('admin','app','canvas')
) bad;

SELECT 'duplicate_durable_identities' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('notification:',MIN(`id`)) entity
  FROM `notifications` WHERE `source_task_id` IS NOT NULL
  GROUP BY `source_task_id`,`user_id` HAVING COUNT(*)>1
  UNION ALL
  SELECT CONCAT('export_object:',MIN(`id`))
  FROM `export_tasks` WHERE `object_key` IS NOT NULL AND `object_key`<>''
  GROUP BY `object_key` HAVING COUNT(*)>1
  UNION ALL
  SELECT CONCAT('ai_run:',MIN(`id`))
  FROM `ai_runs` WHERE `idempotency_key` IS NOT NULL AND `idempotency_key`<>''
  GROUP BY `idempotency_key` HAVING COUNT(*)>1
  UNION ALL
  SELECT CONCAT('ai_reply:',MIN(`id`))
  FROM `ai_reply_commands` GROUP BY `idempotency_key` HAVING COUNT(*)>1
  UNION ALL
  SELECT CONCAT('payment_callback:',MIN(`id`))
  FROM `payment_callback_events` WHERE `is_del`=2 AND `notify_id`<>''
  GROUP BY `provider`,`notify_id` HAVING COUNT(*)>1
  UNION ALL
  SELECT CONCAT('wallet_source:',MIN(`id`))
  FROM `wallet_transactions` WHERE `is_del`=2 AND `source_type`<>'' AND `source_id`<>0
  GROUP BY `source_type`,`source_id` HAVING COUNT(*)>1
) bad;

SELECT 'active_ownership_missing' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('notification:',n.`id`) entity
  FROM `notifications` n LEFT JOIN `users` u ON u.`id`=n.`user_id`
  WHERE n.`is_del`=2 AND n.`platform`='admin' AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('export:',e.`id`)
  FROM `export_tasks` e LEFT JOIN `users` u ON u.`id`=e.`user_id`
  WHERE e.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('ai_run:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `users` u ON u.`id`=r.`user_id` WHERE u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('ai_image_task:',t.`id`)
  FROM `ai_image_tasks` t LEFT JOIN `users` u ON u.`id`=t.`user_id` WHERE u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('ai_text_task:',t.`id`)
  FROM `ai_text_tasks` t LEFT JOIN `users` u ON u.`id`=t.`user_id` WHERE u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('ai_video_task:',t.`id`)
  FROM `ai_video_tasks` t LEFT JOIN `users` u ON u.`id`=t.`user_id` WHERE t.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('ai_asset:',a.`id`)
  FROM `ai_assets` a LEFT JOIN `users` u ON u.`id`=a.`user_id`
  WHERE a.`is_del`=2 AND a.`user_id`<>0 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('session:',s.`id`)
  FROM `user_sessions` s LEFT JOIN `users` u ON u.`id`=s.`user_id`
  WHERE s.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('notification_task:',t.`id`)
  FROM `notification_task` t LEFT JOIN `users` u ON u.`id`=t.`created_by`
  WHERE t.`is_del`=2 AND t.`created_by`<>0 AND u.`id` IS NULL
) bad;
