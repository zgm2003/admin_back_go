CREATE TABLE IF NOT EXISTS `schema_reconciliation_runs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `stage` VARCHAR(32) NOT NULL,
  `script_name` VARCHAR(191) NOT NULL,
  `script_sha256` CHAR(64) NOT NULL,
  `source_fingerprint_sha256` CHAR(64) NOT NULL,
  `target_fingerprint_sha256` CHAR(64) NULL,
  `executor` VARCHAR(191) NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `details_json` JSON NULL,
  `started_at` DATETIME(6) NOT NULL,
  `finished_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_schema_reconciliation_script_sha` (`script_name`, `script_sha256`),
  KEY `idx_schema_reconciliation_status` (`status`, `started_at`, `id`),
  CONSTRAINT `chk_schema_reconciliation_status`
    CHECK (`status` IN ('running', 'succeeded', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
