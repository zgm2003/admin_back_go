CREATE TEMPORARY TABLE `_role_page_code_guard` (
  `value` TINYINT NOT NULL,
  CONSTRAINT `chk_role_page_code_guard` CHECK (`value` = 1)
);

INSERT INTO `_role_page_code_guard` (`value`)
SELECT CASE WHEN
  (SELECT COUNT(*)
   FROM `permissions`
   WHERE id = 13
     AND platform = 'admin'
     AND type = 2
     AND path = '/permission/role'
     AND component = 'permission/role'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'permission_role')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'permission_role' AND id <> 13) = 0
THEN 1 ELSE 0 END;

UPDATE `permissions`
SET code = 'permission_role'
WHERE id = 13
  AND platform = 'admin'
  AND type = 2
  AND path = '/permission/role'
  AND component = 'permission/role'
  AND (code IS NULL OR TRIM(code) = '');

DROP TEMPORARY TABLE `_role_page_code_guard`;
