-- Drop return_url from payment_configs.
--
-- return_url is not merchant configuration. It belongs to each concrete
-- payment request so different business pages can choose their own redirect.

SET @payment_configs_has_return_url := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'payment_configs'
    AND COLUMN_NAME = 'return_url'
);

SET @payment_configs_drop_return_url_sql := IF(
  @payment_configs_has_return_url > 0,
  'ALTER TABLE `payment_configs` DROP COLUMN `return_url`',
  'SELECT 1'
);

PREPARE payment_configs_drop_return_url_stmt FROM @payment_configs_drop_return_url_sql;
EXECUTE payment_configs_drop_return_url_stmt;
DEALLOCATE PREPARE payment_configs_drop_return_url_stmt;
