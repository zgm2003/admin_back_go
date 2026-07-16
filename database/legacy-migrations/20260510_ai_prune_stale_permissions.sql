-- Retire stale AI permission rows that no active route or frontend guard uses.
-- Data-only, idempotent. Does not drop live tables or touch current AI menu pages.

UPDATE `role_permissions` AS rp
JOIN `permissions` AS p ON p.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = CURRENT_TIMESTAMP
WHERE p.`code` IN ('ai_knowledge_sync', 'ai_knowledge_document_refresh', 'ai_agent_binding_del')
  AND p.`platform` = 'admin'
  AND p.`is_del` = 2
  AND rp.`is_del` = 2;

UPDATE `permissions`
SET `is_del` = 1,
    `status` = 2,
    `show_menu` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `code` IN ('ai_knowledge_sync', 'ai_knowledge_document_refresh', 'ai_agent_binding_del')
  AND `platform` = 'admin'
  AND `is_del` = 2;
