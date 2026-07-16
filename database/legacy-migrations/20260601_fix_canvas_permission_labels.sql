-- Repair Canvas labels that were imported with a wrong client charset in local/runtime DBs.
-- The source migration is already UTF-8; this file is idempotent and only rewrites display names.

SET NAMES utf8mb4;

UPDATE `auth_platforms`
SET
  `name` = '无限画布',
  `updated_at` = CURRENT_TIMESTAMP
WHERE `code` = 'canvas';

UPDATE `permissions`
SET
  `name` = CASE `code`
    WHEN 'canvas_page' THEN '我的画布'
    WHEN 'canvas_image_page' THEN '生图工作台'
    WHEN 'canvas_video_page' THEN '视频创作台'
    WHEN 'canvas_prompts_page' THEN '提示词库'
    WHEN 'canvas_assets_page' THEN '我的素材'
    WHEN 'canvas_profile_page' THEN '个人资料'
    WHEN 'canvas_wallet_page' THEN '我的钱包'
    WHEN 'canvas_access' THEN '访问画布'
    WHEN 'canvas_ai_image_generate' THEN '图片生成'
    WHEN 'canvas_ai_video_generate' THEN '视频生成'
    WHEN 'canvas_prompt_read' THEN '读取提示词库'
    WHEN 'canvas_asset_read' THEN '读取素材库'
    WHEN 'canvas_wallet_read' THEN '读取钱包'
    WHEN 'canvas_recharge_add' THEN '创建充值'
    WHEN 'canvas_recharge_pay' THEN '支付充值'
    ELSE `name`
  END,
  `updated_at` = CURRENT_TIMESTAMP
WHERE `platform` = 'canvas'
  AND `code` IN (
    'canvas_page',
    'canvas_image_page',
    'canvas_video_page',
    'canvas_prompts_page',
    'canvas_assets_page',
    'canvas_profile_page',
    'canvas_wallet_page',
    'canvas_access',
    'canvas_ai_image_generate',
    'canvas_ai_video_generate',
    'canvas_prompt_read',
    'canvas_asset_read',
    'canvas_wallet_read',
    'canvas_recharge_add',
    'canvas_recharge_pay'
  );
