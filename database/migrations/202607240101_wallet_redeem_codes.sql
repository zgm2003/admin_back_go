-- Add the wallet redeem-code schema. MySQL DDL commits implicitly; the guard
-- prevents applying this revision to an unexpected schema but is not rollback.
DROP TEMPORARY TABLE IF EXISTS `_wallet_redeem_code_schema_guard`;
CREATE TEMPORARY TABLE `_wallet_redeem_code_schema_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_wallet_redeem_code_schema_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`tables`
WHERE `table_schema` = DATABASE()
  AND `table_name` IN ('redeem_code_batches', 'redeem_codes');

INSERT INTO `_wallet_redeem_code_schema_guard`
SELECT IF(
  COUNT(*) = 1
  AND MAX(user_id_column.`column_type` = 'int unsigned') = 1
  AND MAX(user_id_column.`is_nullable` = 'NO') = 1
  AND MAX(user_id_column.`extra` = 'auto_increment') = 1
  AND MAX(user_primary_key.`index_name` = 'PRIMARY') = 1,
  0,
  1
)
FROM `information_schema`.`columns` AS user_id_column
LEFT JOIN `information_schema`.`statistics` AS user_primary_key
  ON user_primary_key.`table_schema` = user_id_column.`table_schema`
 AND user_primary_key.`table_name` = user_id_column.`table_name`
 AND user_primary_key.`column_name` = user_id_column.`column_name`
 AND user_primary_key.`index_name` = 'PRIMARY'
 AND user_primary_key.`seq_in_index` = 1
WHERE user_id_column.`table_schema` = DATABASE()
  AND user_id_column.`table_name` = 'users'
  AND user_id_column.`column_name` = 'id';

CREATE TABLE `redeem_code_batches` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `batch_no` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_fingerprint_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_fingerprint` CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `amount_cents` BIGINT NOT NULL,
  `quantity` INT UNSIGNED NOT NULL,
  `expires_at` DATETIME(6) NULL,
  `note` VARCHAR(255) NOT NULL DEFAULT '',
  `created_by` INT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_redeem_code_batches_batch_no` (`batch_no`),
  UNIQUE KEY `uk_redeem_code_batches_creator_request` (`created_by`, `request_id`),
  KEY `idx_redeem_code_batches_created_at_id` (`created_at`, `id`),
  KEY `idx_redeem_code_batches_expires_at_id` (`expires_at`, `id`),
  CONSTRAINT `chk_redeem_code_batches_amount_cents`
    CHECK (`amount_cents` BETWEEN 1 AND 100000000),
  CONSTRAINT `chk_redeem_code_batches_quantity`
    CHECK (`quantity` BETWEEN 1 AND 1000),
  CONSTRAINT `chk_redeem_code_batches_expiry`
    CHECK (`expires_at` IS NULL OR `expires_at` > `created_at`),
  CONSTRAINT `fk_redeem_code_batches_created_by`
    FOREIGN KEY (`created_by`) REFERENCES `users` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO `_wallet_redeem_code_schema_guard`
SELECT IF(
  COUNT(*) = 1
  AND MAX(batch_id_column.`column_type` = 'bigint') = 1
  AND MAX(batch_id_column.`is_nullable` = 'NO') = 1
  AND MAX(batch_id_column.`extra` = 'auto_increment') = 1
  AND MAX(batch_primary_key.`index_name` = 'PRIMARY') = 1,
  0,
  1
)
FROM `information_schema`.`columns` AS batch_id_column
LEFT JOIN `information_schema`.`statistics` AS batch_primary_key
  ON batch_primary_key.`table_schema` = batch_id_column.`table_schema`
 AND batch_primary_key.`table_name` = batch_id_column.`table_name`
 AND batch_primary_key.`column_name` = batch_id_column.`column_name`
 AND batch_primary_key.`index_name` = 'PRIMARY'
 AND batch_primary_key.`seq_in_index` = 1
WHERE batch_id_column.`table_schema` = DATABASE()
  AND batch_id_column.`table_name` = 'redeem_code_batches'
  AND batch_id_column.`column_name` = 'id';

CREATE TABLE `redeem_codes` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `batch_id` BIGINT NOT NULL,
  `code` CHAR(28) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` VARCHAR(16) NOT NULL,
  `used_by` INT UNSIGNED NULL,
  `used_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_redeem_codes_code` (`code`),
  KEY `idx_redeem_codes_batch_state_id` (`batch_id`, `state`, `id`),
  KEY `idx_redeem_codes_state_id` (`state`, `id`),
  KEY `idx_redeem_codes_used_by_used_at_id` (`used_by`, `used_at`, `id`),
  CONSTRAINT `chk_redeem_codes_state`
    CHECK (`state` IN ('unused', 'used', 'voided')),
  CONSTRAINT `chk_redeem_codes_usage`
    CHECK ((`state` = 'used' AND `used_by` IS NOT NULL AND `used_at` IS NOT NULL)
      OR (`state` IN ('unused', 'voided') AND `used_by` IS NULL AND `used_at` IS NULL)
    ),
  CONSTRAINT `fk_redeem_codes_batch`
    FOREIGN KEY (`batch_id`) REFERENCES `redeem_code_batches` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_redeem_codes_used_by`
    FOREIGN KEY (`used_by`) REFERENCES `users` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

DROP TEMPORARY TABLE `_wallet_redeem_code_schema_guard`;
