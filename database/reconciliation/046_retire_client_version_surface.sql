SET NAMES utf8mb4;
SET @browser_only_previous_group_concat_max_len := @@SESSION.group_concat_max_len;
SET SESSION group_concat_max_len = 67108864;

START TRANSACTION;

SELECT
  COUNT(*),
  SHA2(
    COALESCE(
      GROUP_CONCAT(
        SHA2(CAST(JSON_ARRAY(id,version,notes,file_url,signature,platform,file_size,is_latest,force_update,is_del,created_at,updated_at) AS CHAR),256)
        ORDER BY id SEPARATOR ''
      ),
      ''
    ),
    256
  )
INTO @browser_only_client_versions_count_before, @browser_only_client_versions_hash_before
FROM `client_versions`;

DROP TEMPORARY TABLE IF EXISTS `tmp_browser_only_retired_permission_ids`;
CREATE TEMPORARY TABLE `tmp_browser_only_retired_permission_ids` (
  `id` INT UNSIGNED NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=MEMORY;

INSERT IGNORE INTO `tmp_browser_only_retired_permission_ids` (`id`)
SELECT `id`
FROM `permissions`
WHERE `platform` = 'admin'
  AND (
    `path` = '/system/clientVersion'
    OR `component` = 'system/clientVersion'
    OR `i18n_key` = 'menu.system_clientVersion'
    OR `code` IN (
      'system_clientVersion_add',
      'system_clientVersion_del',
      'system_clientVersion_edit',
      'system_clientVersion_forceUpdate',
      'system_clientVersion_setLatest'
    )
  );

UPDATE `role_permissions` AS rp
JOIN `tmp_browser_only_retired_permission_ids` AS target
  ON target.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = UTC_TIMESTAMP(6)
WHERE rp.`is_del` = 2;
SET @browser_only_retired_role_permissions := ROW_COUNT();

UPDATE `permissions` AS p
JOIN `tmp_browser_only_retired_permission_ids` AS target
  ON target.`id` = p.`id`
SET p.`is_del` = 1,
    p.`updated_at` = UTC_TIMESTAMP(6)
WHERE p.`is_del` = 2;
SET @browser_only_retired_permissions := ROW_COUNT();

SELECT
  COUNT(*),
  SHA2(
    COALESCE(
      GROUP_CONCAT(
        SHA2(CAST(JSON_ARRAY(id,version,notes,file_url,signature,platform,file_size,is_latest,force_update,is_del,created_at,updated_at) AS CHAR),256)
        ORDER BY id SEPARATOR ''
      ),
      ''
    ),
    256
  )
INTO @browser_only_client_versions_count_after, @browser_only_client_versions_hash_after
FROM `client_versions`;

SET @browser_only_history_guard_sql := IF(
  @browser_only_client_versions_count_before <> @browser_only_client_versions_count_after
  OR BINARY @browser_only_client_versions_hash_before <> BINARY @browser_only_client_versions_hash_after,
  "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='client_versions history changed during Browser-only retirement'",
  'DO 0'
);
PREPARE browser_only_history_guard FROM @browser_only_history_guard_sql;
EXECUTE browser_only_history_guard;
DEALLOCATE PREPARE browser_only_history_guard;

COMMIT;

SELECT
  @browser_only_retired_role_permissions AS retired_role_permissions,
  @browser_only_retired_permissions AS retired_permissions,
  @browser_only_client_versions_count_after AS client_versions_count,
  @browser_only_client_versions_hash_after AS client_versions_sha256;

DROP TEMPORARY TABLE IF EXISTS `tmp_browser_only_retired_permission_ids`;
SET SESSION group_concat_max_len = @browser_only_previous_group_concat_max_len;
