ALTER TABLE `ai_provider_models`
  ADD COLUMN `embedding_dimensions` INT UNSIGNED NULL AFTER `mapped_at`,
  ADD COLUMN `embedding_max_input_tokens` BIGINT UNSIGNED NULL AFTER `embedding_dimensions`,
  ADD COLUMN `embedding_token_counter_id` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER `embedding_max_input_tokens`,
  DROP CHECK `chk_ai_provider_models_model_kind`,
  ADD CONSTRAINT `chk_ai_provider_models_model_kind`
    CHECK (`model_kind` IN (_ascii'chat', _ascii'embedding', _ascii'rerank', _ascii'image'));
