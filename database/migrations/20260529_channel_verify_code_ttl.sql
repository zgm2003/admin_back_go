SET NAMES utf8mb4;

SET @schema_name := DATABASE();

SET @mail_ttl_column_exists := (
  SELECT EXISTS(
    SELECT 1
    FROM information_schema.COLUMNS
    WHERE table_schema = @schema_name
      AND table_name = 'mail_configs'
      AND column_name = 'verify_code_ttl_minutes'
  )
);

SET @add_mail_ttl := (
  SELECT IF(
    @mail_ttl_column_exists,
    'SELECT 1',
    'ALTER TABLE `mail_configs` ADD COLUMN `verify_code_ttl_minutes` INT UNSIGNED NOT NULL DEFAULT 5 AFTER `reply_to`'
  )
);
PREPARE stmt FROM @add_mail_ttl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sms_ttl_column_exists := (
  SELECT EXISTS(
    SELECT 1
    FROM information_schema.COLUMNS
    WHERE table_schema = @schema_name
      AND table_name = 'sms_configs'
      AND column_name = 'verify_code_ttl_minutes'
  )
);

SET @add_sms_ttl := (
  SELECT IF(
    @sms_ttl_column_exists,
    'SELECT 1',
    'ALTER TABLE `sms_configs` ADD COLUMN `verify_code_ttl_minutes` INT UNSIGNED NOT NULL DEFAULT 5 AFTER `endpoint`'
  )
);
PREPARE stmt FROM @add_sms_ttl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @old_verify_code_ttl := (
  SELECT CASE
    WHEN `value_type` = 2
      AND `status` = 1
      AND `is_del` = 2
      AND TRIM(`setting_value`) REGEXP '^[0-9]+$'
      AND CAST(TRIM(`setting_value`) AS UNSIGNED) BETWEEN 1 AND 60
    THEN CAST(TRIM(`setting_value`) AS UNSIGNED)
    ELSE 5
  END
  FROM `system_settings`
  WHERE `setting_key` = 'auth.verify_code.ttl_minutes'
  ORDER BY `id`
  LIMIT 1
);

SET @old_verify_code_ttl := COALESCE(@old_verify_code_ttl, 5);

UPDATE `mail_configs`
SET `verify_code_ttl_minutes` = @old_verify_code_ttl,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `is_del` = 2
  AND (
    @mail_ttl_column_exists = 0
    OR `verify_code_ttl_minutes` < 1
    OR `verify_code_ttl_minutes` > 60
  );

UPDATE `sms_configs`
SET `verify_code_ttl_minutes` = @old_verify_code_ttl,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `is_del` = 2
  AND (
    @sms_ttl_column_exists = 0
    OR `verify_code_ttl_minutes` < 1
    OR `verify_code_ttl_minutes` > 60
  );

UPDATE `system_settings`
SET `status` = 2,
    `is_del` = 1,
    `remark` = '验证码有效期已迁移到 mail_configs.verify_code_ttl_minutes 和 sms_configs.verify_code_ttl_minutes',
    `updated_at` = CURRENT_TIMESTAMP
WHERE `setting_key` = 'auth.verify_code.ttl_minutes';
