-- Add CAPTCHA policy to auth platform.
-- Existing rows use slide because Go currently only implements go-captcha slide.

SET @auth_platform_captcha_type_exists := (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'auth_platforms'
      AND COLUMN_NAME = 'captcha_type'
);

SET @auth_platform_captcha_type_sql := IF(
    @auth_platform_captcha_type_exists = 0,
    'ALTER TABLE `auth_platforms` ADD COLUMN `captcha_type` varchar(30) NOT NULL DEFAULT ''slide'' COMMENT ''验证码类型: slide'' AFTER `login_types`',
    'SELECT 1'
);

PREPARE auth_platform_captcha_type_stmt FROM @auth_platform_captcha_type_sql;
EXECUTE auth_platform_captcha_type_stmt;
DEALLOCATE PREPARE auth_platform_captcha_type_stmt;

UPDATE `auth_platforms`
SET `captcha_type` = 'slide'
WHERE `captcha_type` = '';
