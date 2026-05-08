-- Remove retired AI product modules before Go AI core migration.
--
-- Product decision: AI e-commerce script and AI cine factory are out of scope.
-- This migration removes their menu grants and module-owned tables. Core AI
-- history stays: conversations, messages, runs, run steps, and referenced agents
-- are preserved for audit by soft-deleting scene-specific selectors only.
-- Cache/ButtonGrant invalidation is intentionally not performed here.

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_permissions`;
CREATE TEMPORARY TABLE `tmp_ai_prune_permissions` (
    `id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_parent_permissions`;
CREATE TEMPORARY TABLE `tmp_ai_prune_parent_permissions` (
    `id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

INSERT IGNORE INTO `tmp_ai_prune_permissions` (`id`)
SELECT `id`
FROM `permissions`
WHERE `platform` = 'admin'
  AND (
      `path` IN ('/ai/goods', '/ai/cine')
      OR `path` LIKE '/ai/goods/%'
      OR `path` LIKE '/ai/cine/%'
      OR `component` IN ('ai/goods', 'ai/cine')
      OR `component` LIKE 'ai/goods/%'
      OR `component` LIKE 'ai/cine/%'
      OR `i18n_key` IN ('menu.ai_goods', 'menu.ai_cine')
      OR `code` LIKE 'ai_goods\_%'
      OR `code` LIKE 'ai_cine\_%'
  );

INSERT IGNORE INTO `tmp_ai_prune_parent_permissions` (`id`)
SELECT `id`
FROM `tmp_ai_prune_permissions`;

INSERT IGNORE INTO `tmp_ai_prune_permissions` (`id`)
SELECT child.`id`
FROM `permissions` AS child
JOIN `tmp_ai_prune_parent_permissions` AS parent ON parent.`id` = child.`parent_id`
WHERE child.`platform` = 'admin';

UPDATE `users_quick_entry` AS uq
JOIN `tmp_ai_prune_permissions` AS doomed ON doomed.`id` = uq.`permission_id`
SET uq.`is_del` = 1,
    uq.`updated_at` = NOW()
WHERE uq.`is_del` = 2;

DELETE rp
FROM `role_permissions` AS rp
JOIN `tmp_ai_prune_permissions` AS doomed ON doomed.`id` = rp.`permission_id`;

DELETE p
FROM `permissions` AS p
JOIN `tmp_ai_prune_permissions` AS doomed ON doomed.`id` = p.`id`;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_parent_permissions`;
DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_permissions`;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_scene_agents`;
CREATE TEMPORARY TABLE `tmp_ai_prune_scene_agents` (
    `id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

INSERT IGNORE INTO `tmp_ai_prune_scene_agents` (`id`)
SELECT `id`
FROM `ai_agents`
WHERE `scene` IN ('goods_script', 'cine_project', 'cine_keyframe');

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_tools`;
CREATE TEMPORARY TABLE `tmp_ai_prune_tools` (
    `id` INT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

INSERT IGNORE INTO `tmp_ai_prune_tools` (`id`)
SELECT `id`
FROM `ai_tools`
WHERE `code` = 'cine_generate_keyframe';

UPDATE `ai_agent_scenes`
SET `status` = 2,
    `is_del` = 1,
    `updated_at` = NOW()
WHERE `is_del` = 2
  AND `scene_code` IN ('goods_script', 'cine_project', 'cine_keyframe');

UPDATE `ai_assistant_tools` AS aat
LEFT JOIN `tmp_ai_prune_scene_agents` AS a ON a.`id` = aat.`assistant_id`
LEFT JOIN `tmp_ai_prune_tools` AS t ON t.`id` = aat.`tool_id`
SET aat.`status` = 2,
    aat.`is_del` = 1,
    aat.`updated_at` = NOW()
WHERE aat.`is_del` = 2
  AND (a.`id` IS NOT NULL OR t.`id` IS NOT NULL);

UPDATE `ai_agents` AS a
JOIN `tmp_ai_prune_scene_agents` AS doomed ON doomed.`id` = a.`id`
SET a.`status` = 2,
    a.`is_del` = 1,
    a.`updated_at` = NOW()
WHERE a.`is_del` = 2;

UPDATE `ai_tools` AS t
JOIN `tmp_ai_prune_tools` AS doomed ON doomed.`id` = t.`id`
SET t.`status` = 2,
    t.`is_del` = 1,
    t.`updated_at` = NOW()
WHERE t.`is_del` = 2;

DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_scene_agents`;
DROP TEMPORARY TABLE IF EXISTS `tmp_ai_prune_tools`;

DROP TABLE IF EXISTS `cine_assets`;
DROP TABLE IF EXISTS `cine_projects`;
DROP TABLE IF EXISTS `goods`;
