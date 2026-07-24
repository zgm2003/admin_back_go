SELECT 'required_tables' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'export_tasks' name UNION ALL SELECT 'ai_runs' UNION ALL SELECT 'ai_image_tasks'
  UNION ALL SELECT 'ai_image_files' UNION ALL SELECT 'ai_text_tasks' UNION ALL SELECT 'ai_video_tasks'
  UNION ALL SELECT 'ai_assets' UNION ALL SELECT 'payment_callback_events' UNION ALL SELECT 'user_wallets'
  UNION ALL SELECT 'mail_configs' UNION ALL SELECT 'sms_configs' UNION ALL SELECT 'authz_principal_versions'
  UNION ALL SELECT 'ai_reply_commands' UNION ALL SELECT 'ai_provider_attempts' UNION ALL SELECT 'realtime_events'
  UNION ALL SELECT 'realtime_event_retention_watermarks'
) required
LEFT JOIN information_schema.tables t ON t.table_schema=DATABASE() AND t.table_name=required.name
WHERE t.table_name IS NULL;

SELECT 'required_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'export_tasks' t, 'platform' c UNION ALL SELECT 'export_tasks','kind' UNION ALL SELECT 'export_tasks','object_key'
  UNION ALL SELECT 'export_tasks','claim_owner' UNION ALL SELECT 'export_tasks','claim_token' UNION ALL SELECT 'export_tasks','claim_expires_at'
  UNION ALL SELECT 'ai_runs','platform' UNION ALL SELECT 'ai_runs','input_snapshot' UNION ALL SELECT 'ai_runs','idempotency_key'
  UNION ALL SELECT 'ai_image_tasks','platform' UNION ALL SELECT 'user_wallets','total_consume_cents'
  UNION ALL SELECT 'mail_configs','verify_code_ttl_minutes' UNION ALL SELECT 'sms_configs','verify_code_ttl_minutes'
  UNION ALL SELECT 'notifications','source_task_id' UNION ALL SELECT 'notification_task','claim_owner'
  UNION ALL SELECT 'notification_task','claim_token' UNION ALL SELECT 'notification_task','claim_expires_at'
  UNION ALL SELECT 'realtime_event_retention_watermarks','target_type'
  UNION ALL SELECT 'realtime_event_retention_watermarks','target_id'
  UNION ALL SELECT 'realtime_event_retention_watermarks','deleted_through_sequence'
  UNION ALL SELECT 'realtime_event_retention_watermarks','updated_at'
) required
LEFT JOIN information_schema.columns c ON c.table_schema=DATABASE() AND c.table_name=required.t AND c.column_name=required.c
WHERE c.column_name IS NULL;

SELECT 'required_column_shapes' AS invariant, COUNT(*) AS violations
FROM information_schema.columns
WHERE table_schema=DATABASE() AND (
  (table_name='ai_runs' AND column_name='idempotency_key' AND
    (column_type<>'varchar(128)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
  (table_name IN ('ai_runs','ai_reply_commands') AND column_name='request_id' AND
    (column_type<>'varchar(128)' OR is_nullable<>'NO')) OR
  (table_name='realtime_events' AND column_name='request_id' AND
    (column_type<>'varchar(128)' OR is_nullable<>'YES')) OR
  (table_name='realtime_events' AND column_name='expires_at' AND
    (column_type<>'datetime(6)' OR is_nullable<>'NO')) OR
  (table_name='realtime_event_retention_watermarks' AND column_name='deleted_through_sequence' AND
    (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> '0'))) OR
  (table_name='notifications' AND column_name='source_task_id' AND
    (column_type<>'bigint' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
  (table_name IN ('notification_task','export_tasks') AND column_name='claim_owner' AND
    (column_type<>'varchar(128)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
  (table_name IN ('notification_task','export_tasks') AND column_name='claim_token' AND
    (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> '0'))) OR
  (table_name IN ('notification_task','export_tasks') AND column_name='claim_expires_at' AND
    (column_type<>'datetime(6)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL)))
);

SELECT 'required_indexes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'ai_runs' t, 'uk_ai_runs_idempotency' i, 0 non_unique, 'idempotency_key' columns_in_order UNION ALL
  SELECT 'notifications','uk_notifications_source_user',0,'source_task_id,user_id' UNION ALL
  SELECT 'notification_task','idx_notification_task_claim',1,'status,is_del,send_at,claim_expires_at,id' UNION ALL
  SELECT 'export_tasks','idx_export_task_claim',1,'status,is_del,claim_expires_at,id' UNION ALL
  SELECT 'ai_reply_commands','uk_ai_reply_request',0,'conversation_id,request_id' UNION ALL
  SELECT 'ai_reply_commands','uk_ai_reply_message',0,'user_message_id' UNION ALL
  SELECT 'ai_reply_commands','uk_ai_reply_idempotency',0,'idempotency_key' UNION ALL
  SELECT 'ai_reply_commands','idx_ai_reply_claim',1,'state,next_attempt_at,lease_expires_at,id' UNION ALL
  SELECT 'ai_provider_attempts','uk_ai_attempt_command_no',0,'command_id,attempt_no' UNION ALL
  SELECT 'ai_provider_attempts','uk_ai_attempt_key',0,'idempotency_key' UNION ALL
  SELECT 'realtime_events','uk_realtime_event_id',0,'event_id' UNION ALL
  SELECT 'realtime_events','idx_realtime_resume',1,'target_type,target_id,sequence' UNION ALL
  SELECT 'realtime_events','idx_realtime_expiry',1,'expires_at,sequence' UNION ALL
  SELECT 'realtime_event_retention_watermarks','PRIMARY',0,'target_type,target_id'
) required
LEFT JOIN (
  SELECT table_name, index_name, MIN(non_unique) non_unique,
    GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') columns_in_order
  FROM information_schema.statistics
  WHERE table_schema=DATABASE()
  GROUP BY table_name, index_name
) actual ON actual.table_name=required.t AND actual.index_name=required.i
WHERE actual.index_name IS NULL
  OR actual.non_unique<>required.non_unique
  OR actual.columns_in_order<>required.columns_in_order;

SELECT 'required_constraints' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'authz_principal_versions' t, 'chk_authz_principal_platform' c, '(platform=''admin'')' clause UNION ALL
  SELECT 'ai_reply_commands','chk_ai_reply_platform','(platform=''admin'')' UNION ALL
  SELECT 'ai_reply_commands','chk_ai_reply_state','(statein(''pending'',''claimed'',''running'',''succeeded'',''failed'',''canceled'',''outcome_unknown'',''timed_out''))' UNION ALL
  SELECT 'ai_video_tasks','chk_ai_video_platform','(platformin(''admin'',''canvas''))'
) required
LEFT JOIN (
  SELECT tc.table_name, tc.constraint_name, tc.constraint_type,
    LOWER(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(
      cc.check_clause, '`', ''), ' ', ''), '_utf8mb4', ''), '_utf8mb3', ''), '_utf8', ''), '_gbk', ''), CHAR(92), '')) normalized_clause
  FROM information_schema.table_constraints tc
  JOIN information_schema.check_constraints cc
    ON cc.constraint_schema=tc.constraint_schema AND cc.constraint_name=tc.constraint_name
  WHERE tc.constraint_schema=DATABASE()
) actual ON actual.table_name=required.t AND actual.constraint_name=required.c
WHERE actual.constraint_name IS NULL
  OR actual.constraint_type<>'CHECK'
  OR actual.normalized_clause<>required.clause;

SELECT 'mail_verification_diagnostic_table' AS invariant, COUNT(*) AS violations
FROM (
  SELECT COUNT(*) AS table_count
  FROM information_schema.tables
  WHERE table_schema=DATABASE() AND table_name='mail_log_verification_codes'
) actual
WHERE actual.table_count<>1;

SELECT 'mail_verification_diagnostic_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT COUNT(*) AS column_count,
    SUM(column_name IN ('id','mail_log_id','key_id','code_enc','expires_at','created_at')) AS allowed_column_count
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='mail_log_verification_codes'
) actual
WHERE actual.column_count<>6 OR actual.allowed_column_count<>6;

SELECT 'mail_verification_diagnostic_column_shapes' AS invariant, COUNT(*) AS violations
FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name='mail_log_verification_codes' AND (
  (column_name='id' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR extra<>'auto_increment' OR NOT (column_default <=> NULL))) OR
  (column_name='mail_log_id' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (column_name='key_id' AND (column_type<>'varchar(64)' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (column_name='code_enc' AND (column_type<>'varchar(255)' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (column_name='expires_at' AND (column_type<>'datetime' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (column_name='created_at' AND (column_type<>'datetime' OR is_nullable<>'NO' OR column_default<>'CURRENT_TIMESTAMP'))
);

SELECT 'mail_verification_diagnostic_indexes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT COUNT(DISTINCT index_name) AS index_count,
    SUM(index_name='PRIMARY' AND non_unique=0 AND columns_in_order='id') AS primary_key_count,
    SUM(index_name='uk_mail_log_verification_codes_mail_log' AND non_unique=0 AND columns_in_order='mail_log_id') AS unique_key_count,
    SUM(index_name='idx_mail_log_verification_codes_key_id_id' AND non_unique=1 AND columns_in_order='key_id,id') AS key_count
  FROM (
    SELECT index_name, MIN(non_unique) AS non_unique,
      GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') AS columns_in_order
    FROM information_schema.statistics
    WHERE table_schema=DATABASE() AND table_name='mail_log_verification_codes'
    GROUP BY index_name
  ) indexes
) actual
WHERE actual.index_count<>3 OR actual.primary_key_count<>1 OR actual.unique_key_count<>1 OR actual.key_count<>1;

SELECT 'mail_verification_diagnostic_foreign_key' AS invariant, COUNT(*) AS violations
FROM (
  SELECT COUNT(*) AS foreign_key_count,
    SUM(kcu.constraint_name='fk_mail_log_verification_codes_mail_log'
      AND kcu.column_name='mail_log_id' AND kcu.referenced_table_name='mail_logs'
      AND kcu.referenced_column_name='id' AND rc.update_rule='RESTRICT' AND rc.delete_rule='RESTRICT') AS expected_foreign_key_count
    , MAX(rc.update_rule) AS on_update, MAX(rc.delete_rule) AS on_delete
  FROM information_schema.key_column_usage kcu
  LEFT JOIN information_schema.referential_constraints rc
    ON rc.constraint_schema=kcu.constraint_schema AND rc.constraint_name=kcu.constraint_name
  WHERE kcu.table_schema=DATABASE() AND kcu.table_name='mail_log_verification_codes'
    AND kcu.referenced_table_name IS NOT NULL
) actual
WHERE actual.foreign_key_count<>1 OR actual.expected_foreign_key_count<>1
  OR actual.on_update<>'RESTRICT' OR actual.on_delete<>'RESTRICT';
