SET NAMES utf8mb4;

INSERT INTO `system_settings` (`setting_key`, `setting_value`, `value_type`, `remark`, `status`, `is_del`)
VALUES
  ('auth.captcha.ttl_minutes', '2', 2, CONVERT(UNHEX('E9AA8CE8AF81E7A081E69C89E69588E69C9FE58886E9929FE695B0') USING utf8mb4), 1, 2),
  ('auth.captcha.slide_padding', '10', 2, CONVERT(UNHEX('E6BB91E59D97E5AEB9E5B7AEE5838FE7B4A0') USING utf8mb4), 1, 2)
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
