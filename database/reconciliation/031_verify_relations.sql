SELECT 'rbac_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('user_role:',u.`id`) entity
  FROM `users` u LEFT JOIN `roles` r ON r.`id`=u.`role_id` AND r.`is_del`=2
  WHERE u.`is_del`=2 AND u.`role_id`<>0 AND r.`id` IS NULL
  UNION ALL
  SELECT CONCAT('role_permission_role:',rp.`id`)
  FROM `role_permissions` rp LEFT JOIN `roles` r ON r.`id`=rp.`role_id`
  WHERE rp.`is_del`=2 AND r.`id` IS NULL
  UNION ALL
  SELECT CONCAT('role_permission_permission:',rp.`id`)
  FROM `role_permissions` rp JOIN `permissions` p ON p.`id`=rp.`permission_id`
  WHERE rp.`is_del`=2 AND p.`is_del`<>2
  UNION ALL
  SELECT CONCAT('permission_parent:',p.`id`)
  FROM `permissions` p LEFT JOIN `permissions` parent ON parent.`id`=p.`parent_id`
  WHERE p.`is_del`=2 AND p.`parent_id`<>0 AND parent.`id` IS NULL
  UNION ALL
  SELECT CONCAT('principal_user:',v.`user_id`)
  FROM `authz_principal_versions` v LEFT JOIN `users` u ON u.`id`=v.`user_id`
  WHERE u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('missing_principal:',u.`id`)
  FROM `users` u LEFT JOIN `authz_principal_versions` v ON v.`user_id`=u.`id` AND v.`platform`='admin'
  WHERE u.`status`=1 AND u.`is_del`=2 AND v.`user_id` IS NULL
) bad;

SELECT 'payment_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('order_config:',o.`id`) entity
  FROM `payment_orders` o LEFT JOIN `payment_configs` c ON c.`id`=o.`config_id`
  WHERE o.`is_del`=2 AND c.`id` IS NULL
  UNION ALL
  SELECT CONCAT('recharge_user:',r.`id`)
  FROM `payment_recharges` r LEFT JOIN `users` u ON u.`id`=r.`user_id`
  WHERE r.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('recharge_order:',r.`id`)
  FROM `payment_recharges` r LEFT JOIN `payment_orders` o ON o.`id`=r.`payment_order_id`
  WHERE r.`is_del`=2 AND o.`id` IS NULL
  UNION ALL
  SELECT CONCAT('callback_order:',e.`id`)
  FROM `payment_callback_events` e LEFT JOIN `payment_orders` o ON o.`order_no`=e.`out_trade_no`
  WHERE e.`is_del`=2 AND e.`out_trade_no`<>'' AND o.`id` IS NULL
) bad;

SELECT 'wallet_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('wallet_user:',w.`id`) entity
  FROM `user_wallets` w LEFT JOIN `users` u ON u.`id`=w.`user_id`
  WHERE w.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('transaction_wallet:',t.`id`)
  FROM `wallet_transactions` t LEFT JOIN `user_wallets` w ON w.`id`=t.`wallet_id`
  WHERE t.`is_del`=2 AND w.`id` IS NULL
  UNION ALL
  SELECT CONCAT('transaction_user:',t.`id`)
  FROM `wallet_transactions` t LEFT JOIN `users` u ON u.`id`=t.`user_id`
  WHERE t.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('transaction_owner:',t.`id`)
  FROM `wallet_transactions` t JOIN `user_wallets` w ON w.`id`=t.`wallet_id`
  WHERE t.`is_del`=2 AND t.`user_id`<>w.`user_id`
) bad;

SELECT 'ai_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('conversation_user:',c.`id`) entity
  FROM `ai_conversations` c LEFT JOIN `users` u ON u.`id`=c.`user_id`
  WHERE c.`is_del`=2 AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('conversation_agent:',c.`id`)
  FROM `ai_conversations` c LEFT JOIN `ai_agents` a ON a.`id`=c.`agent_id`
  WHERE c.`is_del`=2 AND a.`id` IS NULL
  UNION ALL
  SELECT CONCAT('message_conversation:',m.`id`)
  FROM `ai_messages` m LEFT JOIN `ai_conversations` c ON c.`id`=m.`conversation_id`
  WHERE m.`is_del`=2 AND c.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_user:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `users` u ON u.`id`=r.`user_id` WHERE u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_agent:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `ai_agents` a ON a.`id`=r.`agent_id` WHERE a.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_provider:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `ai_providers` p ON p.`id`=r.`provider_id` WHERE p.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_conversation:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `ai_conversations` c ON c.`id`=r.`conversation_id`
  WHERE r.`conversation_id` IS NOT NULL AND c.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_user_message:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `ai_messages` m ON m.`id`=r.`user_message_id`
  WHERE r.`user_message_id` IS NOT NULL AND m.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_assistant_message:',r.`id`)
  FROM `ai_runs` r LEFT JOIN `ai_messages` m ON m.`id`=r.`assistant_message_id`
  WHERE r.`assistant_message_id` IS NOT NULL AND m.`id` IS NULL
  UNION ALL
  SELECT CONCAT('run_event:',e.`id`)
  FROM `ai_run_events` e LEFT JOIN `ai_runs` r ON r.`id`=e.`run_id` WHERE r.`id` IS NULL
  UNION ALL
  SELECT CONCAT('image_task_user:',t.`id`)
  FROM `ai_image_tasks` t LEFT JOIN `users` u ON u.`id`=t.`user_id` WHERE u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('image_task_agent:',t.`id`)
  FROM `ai_image_tasks` t LEFT JOIN `ai_agents` a ON a.`id`=t.`agent_id` WHERE a.`id` IS NULL
  UNION ALL
  SELECT CONCAT('image_task_provider:',t.`id`)
  FROM `ai_image_tasks` t LEFT JOIN `ai_providers` p ON p.`id`=t.`provider_id_snapshot` WHERE p.`id` IS NULL
  UNION ALL
  SELECT CONCAT('image_file_task:',f.`id`)
  FROM `ai_image_files` f LEFT JOIN `ai_image_tasks` t ON t.`id`=f.`task_id`
  WHERE t.`id` IS NULL
    AND NOT EXISTS (
      SELECT 1 FROM `ai_runs` r
      WHERE BINARY r.`request_id`=BINARY CONCAT('ai_image_task_',f.`task_id`)
    )
  UNION ALL
  SELECT CONCAT('image_file_related:',f.`id`)
  FROM `ai_image_files` f LEFT JOIN `ai_image_files` related ON related.`id`=f.`related_file_id`
  WHERE f.`related_file_id` IS NOT NULL AND related.`id` IS NULL
) bad;

SELECT 'notification_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('notification_user:',n.`id`) entity
  FROM `notifications` n LEFT JOIN `users` u ON u.`id`=n.`user_id`
  WHERE n.`is_del`=2 AND n.`platform`='admin' AND u.`id` IS NULL
  UNION ALL
  SELECT CONCAT('notification_task:',n.`id`)
  FROM `notifications` n LEFT JOIN `notification_task` t ON t.`id`=n.`source_task_id`
  WHERE n.`source_task_id` IS NOT NULL AND t.`id` IS NULL
  UNION ALL
  SELECT CONCAT('notification_identity:',n.`id`)
  FROM `notifications` n JOIN `notification_task` t ON t.`id`=n.`source_task_id`
  WHERE t.`title`<>n.`title` OR t.`content`<>n.`content` OR t.`type`<>n.`type`
     OR t.`level`<>n.`level` OR t.`link`<>n.`link` OR t.`platform`<>n.`platform`
     OR n.`created_at`<t.`created_at` OR n.`created_at`>t.`updated_at`
  UNION ALL
  SELECT CONCAT('task_creator:',t.`id`)
  FROM `notification_task` t LEFT JOIN `users` u ON u.`id`=t.`created_by`
  WHERE t.`is_del`=2 AND t.`created_by`<>0 AND u.`id` IS NULL
) bad;

SELECT 'export_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM `export_tasks` e LEFT JOIN `users` u ON u.`id`=e.`user_id`
WHERE e.`is_del`=2 AND u.`id` IS NULL;

SELECT 'mail_verification_diagnostic_orphans' AS invariant, COUNT(*) AS violations
FROM `mail_log_verification_codes` mvc
LEFT JOIN `mail_logs` ml ON ml.`id`=mvc.`mail_log_id`
WHERE ml.`id` IS NULL;

SELECT 'redeem_code_foreign_keys' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'redeem_code_batches' AS table_name, 'fk_redeem_code_batches_created_by' AS constraint_name,
    'created_by' AS column_name, 'users' AS referenced_table_name, 'id' AS referenced_column_name
  UNION ALL
  SELECT 'redeem_codes','fk_redeem_codes_batch','batch_id','redeem_code_batches','id'
  UNION ALL
  SELECT 'redeem_codes','fk_redeem_codes_used_by','used_by','users','id'
) required
LEFT JOIN (
  SELECT kcu.table_name,kcu.constraint_name,kcu.column_name,kcu.referenced_table_name,kcu.referenced_column_name,
    rc.update_rule,rc.delete_rule
  FROM information_schema.key_column_usage kcu
  JOIN information_schema.referential_constraints rc
    ON rc.constraint_schema=kcu.constraint_schema
   AND rc.table_name=kcu.table_name
   AND rc.constraint_name=kcu.constraint_name
  WHERE kcu.table_schema=DATABASE()
    AND kcu.table_name IN ('redeem_code_batches','redeem_codes')
    AND kcu.referenced_table_name IS NOT NULL
) actual
  ON actual.table_name=required.table_name
 AND actual.constraint_name=required.constraint_name
WHERE actual.constraint_name IS NULL
   OR actual.column_name<>required.column_name
   OR actual.referenced_table_name<>required.referenced_table_name
   OR actual.referenced_column_name<>required.referenced_column_name
   OR actual.update_rule<>'RESTRICT'
   OR actual.delete_rule<>'RESTRICT';

SELECT 'redeem_code_relationship_orphans' AS invariant, COUNT(*) AS violations
FROM (
  SELECT CONCAT('redeem_code_batch_creator:',batch_row.`id`) AS entity
  FROM `redeem_code_batches` AS batch_row
  LEFT JOIN `users` AS creator ON creator.`id`=batch_row.`created_by`
  WHERE creator.`id` IS NULL
  UNION ALL
  SELECT CONCAT('redeem_code_batch:',code_row.`id`)
  FROM `redeem_codes` AS code_row
  LEFT JOIN `redeem_code_batches` AS batch_row ON batch_row.`id`=code_row.`batch_id`
  WHERE batch_row.`id` IS NULL
  UNION ALL
  SELECT CONCAT('redeem_code_user:',code_row.`id`)
  FROM `redeem_codes` AS code_row
  LEFT JOIN `users` AS used_by_user ON used_by_user.`id`=code_row.`used_by`
  WHERE code_row.`used_by` IS NOT NULL AND used_by_user.`id` IS NULL
) orphan_rows;

SELECT 'redeem_code_batch_quantity_mismatch' AS invariant, COUNT(*) AS violations
FROM (
  SELECT batch_row.`id`
  FROM `redeem_code_batches` AS batch_row
  LEFT JOIN `redeem_codes` AS code_row ON code_row.`batch_id`=batch_row.`id`
  GROUP BY batch_row.`id`,batch_row.`quantity`
  HAVING COUNT(code_row.`id`)<>batch_row.`quantity`
) invalid_batches;
