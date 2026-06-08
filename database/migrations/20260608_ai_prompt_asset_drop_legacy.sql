-- Retire legacy Canvas prompt/asset tables after AI prompt/asset convergence.
-- Active readers and writers now use ai_prompts and ai_assets.

DROP TABLE IF EXISTS `canvas_prompts`;
DROP TABLE IF EXISTS `canvas_assets`;
