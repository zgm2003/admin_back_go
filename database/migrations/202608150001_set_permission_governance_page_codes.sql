CREATE TEMPORARY TABLE `_permission_governance_page_code_guard` (
  `value` TINYINT NOT NULL,
  CONSTRAINT `chk_permission_governance_page_code_guard` CHECK (`value` = 1)
);

INSERT INTO `_permission_governance_page_code_guard` (`value`)
SELECT CASE WHEN
  (SELECT COUNT(*) FROM `permissions`
   WHERE id = 12 AND platform = 'admin' AND type = 2
     AND path = '/permission/permission' AND component = 'permission/permission'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'permission_permission')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'permission_permission' AND id <> 12) = 0
  AND (SELECT COUNT(*) FROM `permissions`
   WHERE id = 85 AND platform = 'admin' AND type = 2
     AND path = '/permission/authPlatform' AND component = 'permission/authPlatform'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'permission_authPlatform')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'permission_authPlatform' AND id <> 85) = 0
THEN 1 ELSE 0 END;

UPDATE `permissions`
SET code = 'permission_permission'
WHERE id = 12 AND platform = 'admin' AND type = 2
  AND path = '/permission/permission' AND component = 'permission/permission'
  AND (code IS NULL OR TRIM(code) = '');

UPDATE `permissions`
SET code = 'permission_authPlatform'
WHERE id = 85 AND platform = 'admin' AND type = 2
  AND path = '/permission/authPlatform' AND component = 'permission/authPlatform'
  AND (code IS NULL OR TRIM(code) = '');

DROP TEMPORARY TABLE `_permission_governance_page_code_guard`;
