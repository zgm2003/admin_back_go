-- atlas:delimiter $$
DROP PROCEDURE IF EXISTS `_ai_provider_model_kind_contract_guard`
$$
CREATE PROCEDURE `_ai_provider_model_kind_contract_guard`()
BEGIN
  IF EXISTS (
    SELECT 1
    FROM `ai_provider_models`
    WHERE (`model_kind` = _ascii'embedding' AND (
      (`embedding_dimensions` IS NULL AND `embedding_max_input_tokens` IS NULL
        AND `embedding_token_counter_id` IS NULL AND `status` <> 2)
      OR ((`embedding_dimensions` IS NULL OR `embedding_max_input_tokens` IS NULL
          OR `embedding_token_counter_id` IS NULL)
        AND NOT (`embedding_dimensions` IS NULL AND `embedding_max_input_tokens` IS NULL
          AND `embedding_token_counter_id` IS NULL))
      OR COALESCE(`embedding_dimensions` = 0, FALSE)
      OR COALESCE(`embedding_max_input_tokens` = 0, FALSE)
      OR COALESCE(CHAR_LENGTH(`embedding_token_counter_id`) = 0, FALSE)
    )) OR (`model_kind` <> _ascii'embedding' AND (
      `embedding_dimensions` IS NOT NULL OR `embedding_max_input_tokens` IS NOT NULL
      OR `embedding_token_counter_id` IS NOT NULL
    ))
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'provider model embedding spec violates final contract';
  END IF;
END
$$
CALL `_ai_provider_model_kind_contract_guard`()
$$
DROP PROCEDURE `_ai_provider_model_kind_contract_guard`
$$

ALTER TABLE `ai_provider_models`
  ADD CONSTRAINT `chk_ai_provider_models_embedding_spec`
  CHECK (
    (`model_kind` = _ascii'embedding' AND (
      (`status` = 2 AND `embedding_dimensions` IS NULL
        AND `embedding_max_input_tokens` IS NULL AND `embedding_token_counter_id` IS NULL)
      OR (`embedding_dimensions` IS NOT NULL AND `embedding_dimensions` > 0
        AND `embedding_max_input_tokens` IS NOT NULL AND `embedding_max_input_tokens` > 0
        AND `embedding_token_counter_id` IS NOT NULL
        AND CHAR_LENGTH(`embedding_token_counter_id`) > 0)
    ))
    OR (`model_kind` <> _ascii'embedding'
      AND `embedding_dimensions` IS NULL
      AND `embedding_max_input_tokens` IS NULL
      AND `embedding_token_counter_id` IS NULL)
  )
$$
