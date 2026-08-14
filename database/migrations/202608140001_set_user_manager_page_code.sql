CREATE TEMPORARY TABLE `_user_manager_page_code_guard` (
  `value` TINYINT NOT NULL,
  CONSTRAINT `chk_user_manager_page_code_guard` CHECK (`value` = 1)
);

INSERT INTO `_user_manager_page_code_guard` (`value`)
SELECT CASE WHEN
  (SELECT COUNT(*)
   FROM `permissions`
   WHERE id = 7
     AND platform = 'admin'
     AND type = 2
     AND path = '/user/userManager'
     AND component = 'user/userManager'
     AND (code IS NULL OR TRIM(code) = '' OR code = 'user_userManager')) = 1
  AND (SELECT COUNT(*) FROM `permissions` WHERE code = 'user_userManager' AND id <> 7) = 0
THEN 1 ELSE 0 END;

UPDATE `permissions`
SET code = 'user_userManager'
WHERE id = 7
  AND platform = 'admin'
  AND type = 2
  AND path = '/user/userManager'
  AND component = 'user/userManager'
  AND (code IS NULL OR TRIM(code) = '');

DROP TEMPORARY TABLE `_user_manager_page_code_guard`;
