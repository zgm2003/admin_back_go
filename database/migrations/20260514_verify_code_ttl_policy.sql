INSERT INTO `system_settings` (`setting_key`, `setting_value`, `value_type`, `remark`, `status`, `is_del`)
VALUES ('auth.verify_code.ttl_minutes', '5', 2, '验证码有效期分钟数，邮件和短信共用', 1, 2)
ON DUPLICATE KEY UPDATE
  `setting_value` = CASE
    WHEN `setting_value` IS NULL OR TRIM(`setting_value`) = '' THEN VALUES(`setting_value`)
    ELSE `setting_value`
  END,
  `value_type` = 2,
  `remark` = VALUES(`remark`),
  `status` = 1,
  `is_del` = 2,
  `updated_at` = CURRENT_TIMESTAMP;

UPDATE `mail_templates`
SET
  `variables_json` = JSON_ARRAY('code', 'ttl_minutes'),
  `sample_variables_json` = JSON_OBJECT('code', '123456', 'ttl_minutes', '5'),
  `updated_at` = CURRENT_TIMESTAMP
WHERE `is_del` = 2
  AND `scene` IN ('login', 'forget', 'bind_email', 'change_password');
