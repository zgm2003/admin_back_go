-- Remove the admin chat room from the active product scope.
--
-- This is intentionally destructive by product decision: remove the admin
-- chat menu grants and drop the legacy chat data tables. AI chat remains a
-- separate module and keeps the `ai_chat_images` upload folder.

DROP TEMPORARY TABLE IF EXISTS `tmp_admin_chat_permissions`;
CREATE TEMPORARY TABLE `tmp_admin_chat_permissions` (
    `id` BIGINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

DROP TEMPORARY TABLE IF EXISTS `tmp_admin_chat_parent_permissions`;
CREATE TEMPORARY TABLE `tmp_admin_chat_parent_permissions` (
    `id` BIGINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

INSERT IGNORE INTO `tmp_admin_chat_permissions` (`id`)
SELECT `id`
FROM `permissions`
WHERE `platform` = 'admin'
  AND (
      `path` = '/chat'
      OR `path` LIKE '/chat/%'
      OR `component` = 'chat'
      OR `component` LIKE 'chat/%'
      OR `i18n_key` = 'menu.chat'
      OR `code` = 'chat'
      OR `code` LIKE 'chat\_%'
  );

INSERT IGNORE INTO `tmp_admin_chat_parent_permissions` (`id`)
SELECT `id`
FROM `tmp_admin_chat_permissions`;

INSERT IGNORE INTO `tmp_admin_chat_permissions` (`id`)
SELECT child.`id`
FROM `permissions` AS child
JOIN `tmp_admin_chat_parent_permissions` AS parent ON parent.`id` = child.`parent_id`
WHERE child.`platform` = 'admin';

DELETE rp
FROM `role_permissions` AS rp
JOIN `tmp_admin_chat_permissions` AS doomed ON doomed.`id` = rp.`permission_id`;

DELETE p
FROM `permissions` AS p
JOIN `tmp_admin_chat_permissions` AS doomed ON doomed.`id` = p.`id`;

DROP TEMPORARY TABLE IF EXISTS `tmp_admin_chat_parent_permissions`;
DROP TEMPORARY TABLE IF EXISTS `tmp_admin_chat_permissions`;

DROP TABLE IF EXISTS `chat_messages`;
DROP TABLE IF EXISTS `chat_participants`;
DROP TABLE IF EXISTS `chat_contacts`;
DROP TABLE IF EXISTS `chat_conversations`;
