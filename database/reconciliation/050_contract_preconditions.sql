SELECT 'active_retired_session_violations' AS invariant, COUNT(*) AS violations
FROM `user_sessions`
WHERE `platform` IN ('app', 'canvas')
  AND `is_del` = 2
  AND `revoked_at` IS NULL
  AND `refresh_expires_at` > UTC_TIMESTAMP(6);

SELECT 'unknown_platform_violations' AS invariant, COUNT(*) AS violations
FROM (
  SELECT `id` FROM `permissions` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `user_sessions` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `users_login_log` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `notification_task` WHERE `platform` NOT IN ('admin', 'app', 'canvas', 'all')
  UNION ALL
  SELECT `id` FROM `notifications` WHERE `platform` NOT IN ('admin', 'app', 'canvas', 'all')
  UNION ALL
  SELECT `id` FROM `export_tasks` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `ai_runs` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `ai_text_tasks` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `ai_image_tasks` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
  UNION ALL
  SELECT `id` FROM `ai_reply_commands` WHERE `platform` NOT IN ('admin', 'app', 'canvas')
) AS unknown_platforms;

SELECT 'unmapped_scene_violations' AS invariant, COUNT(*) AS violations
FROM `ai_agents` AS agent
JOIN JSON_TABLE(
  IF(JSON_VALID(agent.`scenes_json`), agent.`scenes_json`, JSON_ARRAY('__invalid_json__')),
  '$[*]' COLUMNS (`scene` VARCHAR(64) PATH '$')
) AS scenes
WHERE scenes.`scene` NOT IN (
  'chat',
  'agent_generate',
  'canvas_text_generate',
  'canvas_image_generate',
  'canvas_video_generate',
  'canvas_audio_generate',
  'text_generate',
  'image_generate',
  'video_generate',
  'audio_generate'
);

SELECT 'nonterminal_durable_work_violations' AS invariant, COUNT(*) AS violations
FROM `ai_reply_commands`
WHERE `state` IN ('pending', 'claimed', 'running', 'outcome_unknown');

SELECT 'nonterminal_provider_attempt_violations' AS invariant, COUNT(*) AS violations
FROM `ai_runs`
WHERE `status` IN ('pending', 'running', 'streaming');

SELECT 'duplicate_idempotency_violations' AS invariant, COUNT(*) AS violations
FROM (
  SELECT `idempotency_key`
  FROM `ai_reply_commands`
  WHERE `idempotency_key` <> ''
  GROUP BY `idempotency_key`
  HAVING COUNT(*) > 1
) AS duplicate_idempotency_keys;

SELECT 'client_version_surface_count_mismatch' AS invariant,
  IF(
    SUM(`kind` = 'permission') = 6
    AND SUM(`kind` = 'role_permission') = 12,
    0,
    1
  ) AS violations
FROM (
  SELECT 'permission' AS `kind`, permission.`id`
  FROM `permissions` AS permission
  WHERE permission.`platform` = 'admin'
    AND (
      permission.`path` = '/system/clientVersion'
      OR permission.`component` = 'system/clientVersion'
      OR permission.`i18n_key` = 'menu.system_clientVersion'
      OR permission.`code` IN (
        'system_clientVersion_add',
        'system_clientVersion_del',
        'system_clientVersion_edit',
        'system_clientVersion_forceUpdate',
        'system_clientVersion_setLatest'
      )
    )
  UNION ALL
  SELECT 'role_permission' AS `kind`, grant_row.`id`
  FROM `role_permissions` AS grant_row
  JOIN `permissions` AS permission ON permission.`id` = grant_row.`permission_id`
  WHERE permission.`platform` = 'admin'
    AND (
      permission.`path` = '/system/clientVersion'
      OR permission.`component` = 'system/clientVersion'
      OR permission.`i18n_key` = 'menu.system_clientVersion'
      OR permission.`code` IN (
        'system_clientVersion_add',
        'system_clientVersion_del',
        'system_clientVersion_edit',
        'system_clientVersion_forceUpdate',
        'system_clientVersion_setLatest'
      )
    )
) AS active_client_version_surface;

SELECT 'client_versions_count_mismatch' AS invariant,
  IF(COUNT(*) = 8, 0, 1) AS violations
FROM `client_versions`;

SELECT 'client_versions_hash_mismatch' AS invariant,
  IF(
    SHA2(
      COALESCE(
        GROUP_CONCAT(
          SHA2(
            CAST(JSON_ARRAY(
              `id`, `version`, `notes`, `file_url`, `signature`, `platform`,
              `file_size`, `is_latest`, `force_update`, `is_del`, `created_at`, `updated_at`
            ) AS CHAR),
            256
          )
          ORDER BY `id` SEPARATOR ''
        ),
        ''
      ),
      256
    ) = 'ca574b6ce101d92b05cc3571e7e138aa9bf2bc5096c04357c8d39792ba806661',
    0,
    1
  ) AS violations
FROM `client_versions`;

SELECT 'ai_prompts_count_mismatch' AS invariant,
  IF(COUNT(*) = 1356 AND SUM(`is_del` = 2) = 1356, 0, 1) AS violations
FROM `ai_prompts`;

SELECT 'ai_prompt_permission_count_mismatch' AS invariant,
  IF(
    COUNT(*) = 5
    AND SUM(`is_del` = 2) = 5
    AND SUM(`platform` = 'admin') = 5
    AND COUNT(DISTINCT `code`) = 5,
    0,
    1
  ) AS violations
FROM `permissions`
WHERE `code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

SELECT 'ai_prompt_role_grant_count_mismatch' AS invariant,
  IF(COUNT(*) = 10 AND SUM(role_permission.`is_del` = 2) = 10, 0, 1) AS violations
FROM `role_permissions` AS role_permission
JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE permission.`code` IN (
  'ai_prompt_page',
  'ai_prompt_add',
  'ai_prompt_edit',
  'ai_prompt_status',
  'ai_prompt_del'
);

SELECT 'ai_prompt_foreign_key_reference_violations' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`key_column_usage`
WHERE `referenced_table_schema` = DATABASE()
  AND `referenced_table_name` = 'ai_prompts';

SELECT 'wallet_balance_violations' AS invariant, COUNT(*) AS violations
FROM (
  SELECT wallet.`id`
  FROM `user_wallets` AS wallet
  LEFT JOIN (
    SELECT
      `wallet_id`,
      SUM(CASE WHEN `direction` = 'in' THEN `amount_cents` ELSE -`amount_cents` END) AS balance,
      SUM(CASE WHEN `direction` = 'in' AND `source_type` IN ('recharge', 'redeem_code') THEN `amount_cents` ELSE 0 END) AS recharge,
      SUM(CASE WHEN `direction` = 'out' THEN `amount_cents` ELSE 0 END) AS consume
    FROM `wallet_transactions`
    WHERE `is_del` = 2
    GROUP BY `wallet_id`
  ) AS ledger ON ledger.`wallet_id` = wallet.`id`
  WHERE wallet.`is_del` = 2
    AND (
      wallet.`balance_cents` <> COALESCE(ledger.balance, 0)
      OR wallet.`total_recharge_cents` <> COALESCE(ledger.recharge, 0)
      OR wallet.`total_consume_cents` <> COALESCE(ledger.consume, 0)
    )
) AS invalid_wallets;

SELECT 'redeem_code_used_without_transaction' AS invariant, COUNT(*) AS violations
FROM `redeem_codes` AS code_row
JOIN `redeem_code_batches` AS batch_row ON batch_row.`id` = code_row.`batch_id`
WHERE code_row.`state` = 'used'
  AND (
    (SELECT COUNT(*)
     FROM `wallet_transactions` AS transaction_row
     WHERE transaction_row.`source_type` = 'redeem_code'
       AND transaction_row.`source_id` = code_row.`id`
       AND transaction_row.`is_del` = 2) <> 1
    OR NOT EXISTS (
      SELECT 1
      FROM `wallet_transactions` AS transaction_row
      JOIN `user_wallets` AS wallet
        ON wallet.`id` = transaction_row.`wallet_id`
       AND wallet.`is_del` = 2
      WHERE transaction_row.`source_type` = 'redeem_code'
        AND transaction_row.`source_id` = code_row.`id`
        AND transaction_row.`is_del` = 2
        AND transaction_row.`user_id` = code_row.`used_by`
        AND wallet.`user_id` = code_row.`used_by`
        AND transaction_row.`amount_cents` = batch_row.`amount_cents`
        AND transaction_row.`direction` = 'in'
        AND transaction_row.`balance_before_cents` + transaction_row.`amount_cents` = transaction_row.`balance_after_cents`
    )
  );

SELECT 'redeem_code_transaction_without_used_code' AS invariant, COUNT(*) AS violations
FROM `wallet_transactions` AS transaction_row
LEFT JOIN `redeem_codes` AS code_row ON code_row.`id` = transaction_row.`source_id`
LEFT JOIN `redeem_code_batches` AS batch_row ON batch_row.`id` = code_row.`batch_id`
LEFT JOIN `user_wallets` AS wallet ON wallet.`id` = transaction_row.`wallet_id`
WHERE transaction_row.`source_type` = 'redeem_code'
  AND (
    transaction_row.`is_del` <> 2
    OR code_row.`id` IS NULL
    OR code_row.`state` <> 'used'
    OR code_row.`used_by` IS NULL
    OR code_row.`used_at` IS NULL
    OR batch_row.`id` IS NULL
    OR wallet.`id` IS NULL
    OR wallet.`is_del` <> 2
    OR transaction_row.`user_id` <> code_row.`used_by`
    OR wallet.`user_id` <> code_row.`used_by`
    OR transaction_row.`amount_cents` <> batch_row.`amount_cents`
    OR transaction_row.`direction` <> 'in'
    OR transaction_row.`balance_before_cents` + transaction_row.`amount_cents` <> transaction_row.`balance_after_cents`
  );

SELECT 'redeem_code_non_used_with_transaction' AS invariant, COUNT(*) AS violations
FROM `redeem_codes` AS code_row
WHERE code_row.`state` IN ('unused', 'voided')
  AND EXISTS (
    SELECT 1
    FROM `wallet_transactions` AS transaction_row
    WHERE transaction_row.`source_type` = 'redeem_code'
      AND transaction_row.`source_id` = code_row.`id`
  );

SELECT 'classified_orphan_role_permission_mismatch' AS invariant,
  IF(
    COUNT(*) = 1
    AND MAX(role_permission.`role_id`) = 1
    AND MAX(role_permission.`permission_id`) = 539
    AND MAX(permission.`id`) IS NULL,
    0,
    1
  ) AS violations
FROM `role_permissions` AS role_permission
LEFT JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
WHERE role_permission.`id` = 723 AND role_permission.`is_del` = 2;

SELECT 'orphan_relationship_violations' AS invariant, COUNT(*) AS violations
FROM (
  SELECT role_permission.`id`
  FROM `role_permissions` AS role_permission
  LEFT JOIN `roles` AS role_row ON role_row.`id` = role_permission.`role_id`
  LEFT JOIN `permissions` AS permission ON permission.`id` = role_permission.`permission_id`
  WHERE role_permission.`is_del` = 2
    AND role_permission.`id` <> 723
    AND (role_row.`id` IS NULL OR permission.`id` IS NULL OR permission.`is_del` <> 2)
  UNION ALL
  SELECT recharge.`id`
  FROM `payment_recharges` AS recharge
  LEFT JOIN `users` AS user_row ON user_row.`id` = recharge.`user_id`
  LEFT JOIN `payment_orders` AS payment_order ON payment_order.`id` = recharge.`payment_order_id`
  WHERE recharge.`is_del` = 2 AND (user_row.`id` IS NULL OR payment_order.`id` IS NULL)
  UNION ALL
  SELECT transaction_row.`id`
  FROM `wallet_transactions` AS transaction_row
  LEFT JOIN `user_wallets` AS wallet ON wallet.`id` = transaction_row.`wallet_id`
  LEFT JOIN `users` AS user_row ON user_row.`id` = transaction_row.`user_id`
  WHERE transaction_row.`is_del` = 2
    AND (wallet.`id` IS NULL OR user_row.`id` IS NULL OR transaction_row.`user_id` <> wallet.`user_id`)
  UNION ALL
  SELECT image_file.`id`
  FROM `ai_image_files` AS image_file
  LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
  WHERE image_task.`id` IS NULL
    AND NOT EXISTS (
      SELECT 1
      FROM `ai_runs` AS run_row
      WHERE BINARY run_row.`request_id` = BINARY CONCAT('ai_image_task_', image_file.`task_id`)
    )
  UNION ALL
  SELECT export_task.`id`
  FROM `export_tasks` AS export_task
  LEFT JOIN `users` AS user_row ON user_row.`id` = export_task.`user_id`
  WHERE export_task.`is_del` = 2 AND user_row.`id` IS NULL
) AS orphan_rows;

SELECT 'already_absent_legacy_table_violations' AS invariant, COUNT(*) AS violations
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN (
    'users_quick_entry',
    'canvas_prompts',
    'canvas_assets',
    'ai_billing_rules',
    'ai_billing_records'
  );
