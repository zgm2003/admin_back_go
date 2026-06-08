-- Retire Admin image playground and Admin asset management.
-- Image generation stays in Canvas; assets are Canvas user-owned.

UPDATE `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
SET rp.`is_del` = 1,
    rp.`updated_at` = CURRENT_TIMESTAMP
WHERE p.`platform` = 'admin'
  AND p.`code` IN (
    'ai_image_playground_page',
    'ai_image_asset_add',
    'ai_image_task_add',
    'ai_image_task_favorite',
    'ai_image_task_del',
    'ai_asset_page',
    'ai_asset_add',
    'ai_asset_edit',
    'ai_asset_del'
  )
  AND rp.`is_del` = 2;

UPDATE `permissions`
SET `is_del` = 1,
    `status` = 2,
    `show_menu` = 2,
    `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'admin'
  AND `code` IN (
    'ai_image_playground_page',
    'ai_image_asset_add',
    'ai_image_task_add',
    'ai_image_task_favorite',
    'ai_image_task_del',
    'ai_asset_page',
    'ai_asset_add',
    'ai_asset_edit',
    'ai_asset_del'
  );

UPDATE `ai_agents`
SET `scenes_json` = JSON_REMOVE(
      CAST(`scenes_json` AS JSON),
      JSON_UNQUOTE(JSON_SEARCH(CAST(`scenes_json` AS JSON), 'one', 'image_generate'))
    ),
    `updated_at` = CURRENT_TIMESTAMP
WHERE `is_del` = 2
  AND JSON_VALID(`scenes_json`)
  AND JSON_SEARCH(CAST(`scenes_json` AS JSON), 'one', 'image_generate') IS NOT NULL;

UPDATE `ai_agents`
SET `scenes_json` = JSON_ARRAY('chat'),
    `updated_at` = CURRENT_TIMESTAMP
WHERE `is_del` = 2
  AND JSON_VALID(`scenes_json`)
  AND JSON_LENGTH(CAST(`scenes_json` AS JSON)) = 0;
