CREATE TABLE `mail_log_verification_codes` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `mail_log_id` BIGINT UNSIGNED NOT NULL,
  `key_id` VARCHAR(64) NOT NULL,
  `code_enc` VARCHAR(255) NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_mail_log_verification_codes_mail_log` (`mail_log_id`),
  KEY `idx_mail_log_verification_codes_key_id_id` (`key_id`, `id`),
  CONSTRAINT `fk_mail_log_verification_codes_mail_log`
    FOREIGN KEY (`mail_log_id`) REFERENCES `mail_logs` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO `permissions`
(`id`,`name`,`path`,`icon`,`parent_id`,`component`,`platform`,`type`,`sort`,`code`,`i18n_key`,`show_menu`,`status`,`is_del`)
VALUES (515,'查看邮件日志及验证码','','',506,NULL,'admin',3,9,'system_mail_logView','',2,1,2);
