SELECT 'required_tables' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'export_tasks' name UNION ALL SELECT 'ai_runs' UNION ALL SELECT 'ai_image_tasks'
  UNION ALL SELECT 'ai_image_files' UNION ALL SELECT 'ai_text_tasks' UNION ALL SELECT 'ai_video_tasks'
  UNION ALL SELECT 'ai_assets' UNION ALL SELECT 'payment_callback_events' UNION ALL SELECT 'user_wallets'
  UNION ALL SELECT 'mail_configs' UNION ALL SELECT 'sms_configs' UNION ALL SELECT 'authz_principal_versions'
  UNION ALL SELECT 'ai_reply_commands' UNION ALL SELECT 'ai_reply_delivery_chunks' UNION ALL SELECT 'ai_provider_attempts' UNION ALL SELECT 'realtime_events'
  UNION ALL SELECT 'realtime_event_retention_watermarks'
) required
LEFT JOIN information_schema.tables t ON t.table_schema=DATABASE() AND t.table_name=required.name
WHERE t.table_name IS NULL;

SELECT 'required_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'export_tasks' t, 'platform' c UNION ALL SELECT 'export_tasks','kind' UNION ALL SELECT 'export_tasks','object_key'
  UNION ALL SELECT 'export_tasks','claim_owner' UNION ALL SELECT 'export_tasks','claim_token' UNION ALL SELECT 'export_tasks','claim_expires_at'
  UNION ALL SELECT 'ai_runs','platform' UNION ALL SELECT 'ai_runs','input_snapshot' UNION ALL SELECT 'ai_runs','idempotency_key'
  UNION ALL SELECT 'ai_runs','settled_at'
  UNION ALL SELECT 'ai_reply_commands','request_received_at' UNION ALL SELECT 'ai_reply_commands','accepted_at'
  UNION ALL SELECT 'ai_reply_commands','claimed_at' UNION ALL SELECT 'ai_reply_commands','claim_source'
  UNION ALL SELECT 'ai_reply_commands','delivery_seq' UNION ALL SELECT 'ai_reply_commands','stop_delivery_seq'
  UNION ALL SELECT 'ai_messages','delivery_state'
  UNION ALL SELECT 'ai_reply_delivery_chunks','command_id' UNION ALL SELECT 'ai_reply_delivery_chunks','delivery_seq'
  UNION ALL SELECT 'ai_reply_delivery_chunks','delta' UNION ALL SELECT 'ai_reply_delivery_chunks','created_at'
  UNION ALL SELECT 'ai_provider_attempts','prepare_started_at' UNION ALL SELECT 'ai_provider_attempts','first_delta_at'
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
  (table_name IN ('ai_runs','ai_reply_commands','ai_provider_attempts') AND
    column_name IN ('settled_at','request_received_at','accepted_at','claimed_at','prepare_started_at','first_delta_at') AND
    (column_type<>'datetime(6)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
  (table_name='ai_reply_commands' AND column_name='claim_source' AND
    (column_type<>'varchar(16)' OR is_nullable<>'NO' OR NOT (column_default <=> ''))) OR
  (table_name='ai_reply_commands' AND column_name='delivery_seq' AND
    (column_type<>'int unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> '0'))) OR
  (table_name='ai_reply_commands' AND column_name='stop_delivery_seq' AND
    (column_type<>'int unsigned' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
  (table_name='ai_messages' AND column_name='delivery_state' AND
    (column_type<>'varchar(16)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
  (table_name='ai_reply_delivery_chunks' AND column_name='command_id' AND
    (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (table_name='ai_reply_delivery_chunks' AND column_name='delivery_seq' AND
    (column_type<>'int unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (table_name='ai_reply_delivery_chunks' AND column_name='delta' AND
    (column_type<>'text' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
  (table_name='ai_reply_delivery_chunks' AND column_name='created_at' AND
    (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)'))) OR
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
  SELECT 'ai_reply_delivery_chunks','PRIMARY',0,'command_id,delivery_seq' UNION ALL
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

SELECT 'ai_reply_delivery_chunk_indexes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT table_name, COUNT(*) AS index_count
  FROM information_schema.statistics
  WHERE table_schema=DATABASE() AND table_name='ai_reply_delivery_chunks'
  GROUP BY table_name
) actual
WHERE actual.index_count<>1;

SELECT 'required_constraints' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'authz_principal_versions' t, 'chk_authz_principal_platform' c, '(regexp_like(platform,''^[a-z][a-z0-9_]{1,48}$'')and(platformnotin(''app'',''canvas''))and(platform<>''all''))' clause UNION ALL
  SELECT 'ai_reply_commands','chk_ai_reply_platform','(regexp_like(platform,''^[a-z][a-z0-9_]{1,48}$'')and(platformnotin(''app'',''canvas''))and(platform<>''all''))' UNION ALL
  SELECT 'ai_reply_commands','chk_ai_reply_state','(statein(''pending'',''claimed'',''running'',''succeeded'',''failed'',''canceled'',''outcome_unknown'',''timed_out''))' UNION ALL
  SELECT 'ai_reply_commands','chk_ai_reply_claim_source','(claim_sourcein('''',''wake'',''poll'',''recovery''))' UNION ALL
  SELECT 'ai_reply_commands','chk_ai_reply_delivery_seq','(((cancel_requested_atisnull)and(stop_delivery_seqisnull))or((cancel_requested_atisnotnull)and(stop_delivery_seqisnotnull)and(stop_delivery_seq<=delivery_seq)))' UNION ALL
  SELECT 'ai_messages','chk_ai_messages_delivery_state','(((role=2)and(delivery_statein(''completed'',''stopped'')))or((role<>2)and(delivery_stateisnull)))' UNION ALL
  SELECT 'ai_video_tasks','chk_ai_video_platform','(regexp_like(platform,''^[a-z][a-z0-9_]{1,48}$'')and(platformnotin(''app'',''canvas''))and(platform<>''all''))'
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

SELECT 'redeem_code_required_tables' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'redeem_code_batches' AS table_name
  UNION ALL SELECT 'redeem_codes'
) required
LEFT JOIN information_schema.tables actual
  ON actual.table_schema=DATABASE()
 AND actual.table_name=required.table_name
 AND actual.table_type='BASE TABLE'
WHERE actual.table_name IS NULL;

SELECT 'redeem_code_required_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'redeem_code_batches' AS table_name, 'id' AS column_name
  UNION ALL SELECT 'redeem_code_batches','batch_no'
  UNION ALL SELECT 'redeem_code_batches','request_id'
  UNION ALL SELECT 'redeem_code_batches','request_fingerprint_version'
  UNION ALL SELECT 'redeem_code_batches','request_fingerprint'
  UNION ALL SELECT 'redeem_code_batches','amount_cents'
  UNION ALL SELECT 'redeem_code_batches','quantity'
  UNION ALL SELECT 'redeem_code_batches','expires_at'
  UNION ALL SELECT 'redeem_code_batches','note'
  UNION ALL SELECT 'redeem_code_batches','created_by'
  UNION ALL SELECT 'redeem_code_batches','created_at'
  UNION ALL SELECT 'redeem_code_batches','updated_at'
  UNION ALL SELECT 'redeem_codes','id'
  UNION ALL SELECT 'redeem_codes','batch_id'
  UNION ALL SELECT 'redeem_codes','code'
  UNION ALL SELECT 'redeem_codes','state'
  UNION ALL SELECT 'redeem_codes','used_by'
  UNION ALL SELECT 'redeem_codes','used_at'
  UNION ALL SELECT 'redeem_codes','created_at'
  UNION ALL SELECT 'redeem_codes','updated_at'
) required
LEFT JOIN information_schema.columns actual
  ON actual.table_schema=DATABASE()
 AND actual.table_name=required.table_name
 AND actual.column_name=required.column_name
WHERE actual.column_name IS NULL;

SELECT 'redeem_code_column_shapes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT COUNT(*) AS column_count,
    SUM(column_name IN (
      'id','batch_no','request_id','request_fingerprint_version','request_fingerprint','amount_cents',
      'quantity','expires_at','note','created_by','created_at','updated_at'
    )) AS allowed_column_count
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='redeem_code_batches'
  HAVING column_count<>12 OR allowed_column_count<>12
  UNION ALL
  SELECT COUNT(*) AS column_count,
    SUM(column_name IN ('id','batch_id','code','state','used_by','used_at','created_at','updated_at')) AS allowed_column_count
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='redeem_codes'
  HAVING column_count<>8 OR allowed_column_count<>8
  UNION ALL
  SELECT 1, 1
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='redeem_code_batches' AND (
    (column_name='id' AND (column_type<>'bigint' OR is_nullable<>'NO' OR extra<>'auto_increment' OR NOT (column_default <=> NULL))) OR
    (column_name='batch_no' AND (column_type<>'varchar(64)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='request_id' AND (column_type<>'varchar(128)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='request_fingerprint_version' AND (column_type<>'varchar(64)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='request_fingerprint' AND (column_type<>'char(64)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='amount_cents' AND (column_type<>'bigint' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='quantity' AND (column_type<>'int unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='expires_at' AND (column_type<>'datetime(6)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
    (column_name='note' AND (column_type<>'varchar(255)' OR is_nullable<>'NO' OR NOT (column_default <=> ''))) OR
    (column_name='created_by' AND (column_type<>'int unsigned' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='created_at' AND (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)') OR extra NOT IN ('','DEFAULT_GENERATED'))) OR
    (column_name='updated_at' AND (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)') OR extra NOT IN ('on update CURRENT_TIMESTAMP(6)','DEFAULT_GENERATED on update CURRENT_TIMESTAMP(6)')))
  )
  UNION ALL
  SELECT 1, 1
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='redeem_codes' AND (
    (column_name='id' AND (column_type<>'bigint' OR is_nullable<>'NO' OR extra<>'auto_increment' OR NOT (column_default <=> NULL))) OR
    (column_name='batch_id' AND (column_type<>'bigint' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='code' AND (column_type<>'char(28)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='state' AND (column_type<>'varchar(16)' OR is_nullable<>'NO' OR NOT (column_default <=> NULL))) OR
    (column_name='used_by' AND (column_type<>'int unsigned' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
    (column_name='used_at' AND (column_type<>'datetime(6)' OR is_nullable<>'YES' OR NOT (column_default <=> NULL))) OR
    (column_name='created_at' AND (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)') OR extra NOT IN ('','DEFAULT_GENERATED'))) OR
    (column_name='updated_at' AND (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)') OR extra NOT IN ('on update CURRENT_TIMESTAMP(6)','DEFAULT_GENERATED on update CURRENT_TIMESTAMP(6)')))
  )
) invalid_columns;

SELECT 'redeem_code_indexes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'redeem_code_batches' AS table_name, 'PRIMARY' AS index_name, 0 AS non_unique, 'id' AS columns_in_order
  UNION ALL SELECT 'redeem_code_batches','uk_redeem_code_batches_batch_no',0,'batch_no'
  UNION ALL SELECT 'redeem_code_batches','uk_redeem_code_batches_creator_request',0,'created_by,request_id'
  UNION ALL SELECT 'redeem_code_batches','idx_redeem_code_batches_created_at_id',1,'created_at,id'
  UNION ALL SELECT 'redeem_code_batches','idx_redeem_code_batches_expires_at_id',1,'expires_at,id'
  UNION ALL SELECT 'redeem_codes','PRIMARY',0,'id'
  UNION ALL SELECT 'redeem_codes','uk_redeem_codes_code',0,'code'
  UNION ALL SELECT 'redeem_codes','idx_redeem_codes_batch_state_id',1,'batch_id,state,id'
  UNION ALL SELECT 'redeem_codes','idx_redeem_codes_state_id',1,'state,id'
  UNION ALL SELECT 'redeem_codes','idx_redeem_codes_used_by_used_at_id',1,'used_by,used_at,id'
) required
LEFT JOIN (
  SELECT table_name, index_name, MIN(non_unique) AS non_unique,
    GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') AS columns_in_order
  FROM information_schema.statistics
  WHERE table_schema=DATABASE() AND table_name IN ('redeem_code_batches','redeem_codes')
  GROUP BY table_name,index_name
) actual
  ON actual.table_name=required.table_name
 AND actual.index_name=required.index_name
WHERE actual.index_name IS NULL
   OR actual.non_unique<>required.non_unique
   OR actual.columns_in_order<>required.columns_in_order;

SELECT 'redeem_code_checks' AS invariant,
  IF(
    COUNT(*) = 5
    AND SUM(
      (actual.table_name='redeem_code_batches'
        AND actual.constraint_name='chk_redeem_code_batches_amount_cents'
        AND actual.normalized_clause='(amount_centsbetween1and100000000)')
      OR (actual.table_name='redeem_code_batches'
        AND actual.constraint_name='chk_redeem_code_batches_quantity'
        AND actual.normalized_clause='(quantitybetween1and1000)')
      OR (actual.table_name='redeem_code_batches'
        AND actual.constraint_name='chk_redeem_code_batches_expiry'
        AND actual.normalized_clause='((expires_atisnull)or(expires_at>created_at))')
      OR (actual.table_name='redeem_codes'
        AND actual.constraint_name='chk_redeem_codes_state'
        AND actual.normalized_clause='(statein(''unused'',''used'',''voided''))')
      OR (actual.table_name='redeem_codes'
        AND actual.constraint_name='chk_redeem_codes_usage'
        AND actual.normalized_clause='(((state=''used'')and(used_byisnotnull)and(used_atisnotnull))or((statein(''unused'',''voided''))and(used_byisnull)and(used_atisnull)))')
    ) = 5,
    0,
    1
  ) AS violations
FROM (
  SELECT tc.table_name,tc.constraint_name,
    REPLACE(
      REPLACE(
        REPLACE(
          REPLACE(
            REPLACE(
              REPLACE(
                LOWER(REGEXP_REPLACE(cc.check_clause, '[[:space:]`]+', '')),
                '_utf8mb4', ''
              ),
              '_utf8mb3', ''
            ),
            '_utf8', ''
          ),
          '_gbk', ''
        ),
        '_ascii', ''
      ),
      CHAR(92), ''
    ) AS normalized_clause
  FROM information_schema.table_constraints tc
  JOIN information_schema.check_constraints cc
    ON cc.constraint_schema=tc.constraint_schema
   AND cc.constraint_name=tc.constraint_name
  WHERE tc.constraint_schema=DATABASE()
    AND tc.table_name IN ('redeem_code_batches','redeem_codes')
    AND tc.constraint_type='CHECK'
) actual
;

SELECT 'ai_official_model_required_tables' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'ai_official_model_price_overrides' AS table_name
  UNION ALL SELECT 'ai_official_model_price_override_rates'
) required
LEFT JOIN information_schema.tables actual
  ON actual.table_schema=DATABASE()
 AND actual.table_name=required.table_name
 AND actual.table_type='BASE TABLE'
WHERE actual.table_name IS NULL;

SELECT 'ai_official_model_required_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'ai_official_model_price_overrides' AS table_name, 'id' AS column_name
  UNION ALL SELECT 'ai_official_model_price_overrides','catalog_vendor'
  UNION ALL SELECT 'ai_official_model_price_overrides','model_id'
  UNION ALL SELECT 'ai_official_model_price_overrides','version'
  UNION ALL SELECT 'ai_official_model_price_overrides','source_url'
  UNION ALL SELECT 'ai_official_model_price_overrides','verified_at'
  UNION ALL SELECT 'ai_official_model_price_overrides','updated_by'
  UNION ALL SELECT 'ai_official_model_price_overrides','created_at'
  UNION ALL SELECT 'ai_official_model_price_overrides','updated_at'
  UNION ALL SELECT 'ai_official_model_price_override_rates','id'
  UNION ALL SELECT 'ai_official_model_price_override_rates','override_id'
  UNION ALL SELECT 'ai_official_model_price_override_rates','category'
  UNION ALL SELECT 'ai_official_model_price_override_rates','unit'
  UNION ALL SELECT 'ai_official_model_price_override_rates','tier_key'
  UNION ALL SELECT 'ai_official_model_price_override_rates','price_units'
  UNION ALL SELECT 'ai_official_model_price_override_rates','unit_scale'
) required
LEFT JOIN information_schema.columns actual
  ON actual.table_schema=DATABASE()
 AND actual.table_name=required.table_name
 AND actual.column_name=required.column_name
WHERE actual.column_name IS NULL;

SELECT 'ai_official_model_column_shapes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 1 AS violation
  FROM (
    SELECT COUNT(*) AS column_count
    FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ai_official_model_price_overrides'
  ) head_count
  WHERE head_count.column_count<>9
  UNION ALL
  SELECT 1
  FROM (
    SELECT COUNT(*) AS column_count
    FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ai_official_model_price_override_rates'
  ) rate_count
  WHERE rate_count.column_count<>7
  UNION ALL
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='ai_official_model_price_overrides' AND (
    (column_name='id' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR extra<>'auto_increment')) OR
    (column_name='catalog_vendor' AND (column_type<>'varchar(32)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO')) OR
    (column_name='model_id' AND (column_type<>'varchar(191)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO')) OR
    (column_name='version' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO')) OR
    (column_name='source_url' AND (column_type<>'varchar(2048)' OR is_nullable<>'NO')) OR
    (column_name='verified_at' AND (column_type<>'date' OR is_nullable<>'NO')) OR
    (column_name='updated_by' AND (column_type<>'int unsigned' OR is_nullable<>'NO')) OR
    (column_name='created_at' AND (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)'))) OR
    (column_name='updated_at' AND (column_type<>'datetime(6)' OR is_nullable<>'NO' OR NOT (UPPER(column_default) <=> 'CURRENT_TIMESTAMP(6)') OR extra NOT IN ('on update CURRENT_TIMESTAMP(6)','DEFAULT_GENERATED on update CURRENT_TIMESTAMP(6)')))
  )
  UNION ALL
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='ai_official_model_price_override_rates' AND (
    (column_name='id' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO' OR extra<>'auto_increment')) OR
    (column_name='override_id' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO')) OR
    (column_name='category' AND (column_type<>'varchar(32)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO')) OR
    (column_name='unit' AND (column_type<>'varchar(32)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO')) OR
    (column_name='tier_key' AND (column_type<>'varchar(64)' OR character_set_name<>'ascii' OR collation_name<>'ascii_bin' OR is_nullable<>'NO' OR NOT (column_default <=> ''))) OR
    (column_name='price_units' AND (column_type<>'bigint' OR is_nullable<>'NO')) OR
    (column_name='unit_scale' AND (column_type<>'bigint unsigned' OR is_nullable<>'NO'))
  )
) invalid_columns;

SELECT 'ai_official_model_indexes' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'ai_official_model_price_overrides' AS table_name, 'PRIMARY' AS index_name, 0 AS non_unique, 'id' AS columns_in_order
  UNION ALL SELECT 'ai_official_model_price_overrides','uk_ai_official_model_price_overrides_identity',0,'catalog_vendor,model_id'
  UNION ALL SELECT 'ai_official_model_price_override_rates','PRIMARY',0,'id'
  UNION ALL SELECT 'ai_official_model_price_override_rates','uk_ai_official_model_price_override_rates_key',0,'override_id,category,unit,tier_key'
) required
LEFT JOIN (
  SELECT table_name,index_name,MIN(non_unique) AS non_unique,
    GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') AS columns_in_order
  FROM information_schema.statistics
  WHERE table_schema=DATABASE()
    AND table_name IN ('ai_official_model_price_overrides','ai_official_model_price_override_rates')
  GROUP BY table_name,index_name
) actual
  ON actual.table_name=required.table_name
 AND actual.index_name=required.index_name
WHERE actual.index_name IS NULL
   OR actual.non_unique<>required.non_unique
   OR actual.columns_in_order<>required.columns_in_order;

SELECT 'ai_official_model_checks' AS invariant,
  IF(
    COUNT(*)=5
    AND SUM(
      (actual.table_name='ai_official_model_price_overrides'
        AND actual.constraint_name='chk_ai_official_model_price_overrides_version'
        AND actual.normalized_clause='(version>0)')
      OR (actual.table_name='ai_official_model_price_override_rates'
        AND actual.constraint_name='chk_ai_official_model_price_override_rates_category'
        AND actual.normalized_clause='(categoryin(''input'',''output'',''cache_read'',''cache_write'',''media''))')
      OR (actual.table_name='ai_official_model_price_override_rates'
        AND actual.constraint_name='chk_ai_official_model_price_override_rates_unit'
        AND actual.normalized_clause='(char_length(trim(unit))>0)')
      OR (actual.table_name='ai_official_model_price_override_rates'
        AND actual.constraint_name='chk_ai_official_model_price_override_rates_price'
        AND actual.normalized_clause='(price_units>=0)')
      OR (actual.table_name='ai_official_model_price_override_rates'
        AND actual.constraint_name='chk_ai_official_model_price_override_rates_scale'
        AND actual.normalized_clause='(unit_scale>0)')
    )=5,
    0,
    1
  ) AS violations
FROM (
  SELECT tc.table_name,tc.constraint_name,
    REPLACE(
      REPLACE(
        REPLACE(
          REPLACE(
            REPLACE(
              REPLACE(LOWER(REGEXP_REPLACE(cc.check_clause, '[[:space:]`]+', '')), '_utf8mb4', ''),
              '_utf8mb3', ''
            ),
            '_utf8', ''
          ),
          '_gbk', ''
        ),
        '_ascii', ''
      ),
      CHAR(92), ''
    ) AS normalized_clause
  FROM information_schema.table_constraints tc
  JOIN information_schema.check_constraints cc
    ON cc.constraint_schema=tc.constraint_schema
   AND cc.constraint_name=tc.constraint_name
  WHERE tc.constraint_schema=DATABASE()
    AND tc.table_name IN ('ai_official_model_price_overrides','ai_official_model_price_override_rates')
    AND tc.constraint_type='CHECK'
) actual;

SELECT 'ai_official_model_provider_mapping_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'official_model_id' AS column_name
  UNION ALL SELECT 'official_catalog_version'
  UNION ALL SELECT 'mapping_status'
  UNION ALL SELECT 'mapped_at'
) required
LEFT JOIN information_schema.columns actual
  ON actual.table_schema=DATABASE()
 AND actual.table_name='ai_provider_models'
 AND actual.column_name=required.column_name
WHERE actual.column_name IS NULL;

SELECT 'ai_official_model_agent_output_column_removed' AS invariant, COUNT(*) AS violations
FROM information_schema.columns
WHERE table_schema=DATABASE()
  AND table_name='ai_agents'
  AND column_name='max_output_tokens';
