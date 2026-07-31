ALTER TABLE `ai_providers`
  DROP CHECK `chk_ai_providers_file_input_mode`,
  RENAME COLUMN `file_input_mode` TO `api_protocol`;

UPDATE `ai_providers`
SET `api_protocol` = CASE `api_protocol`
  WHEN 'chat_completions' THEN 'responses'
  ELSE 'chat_completions'
END;

ALTER TABLE `ai_providers`
  MODIFY COLUMN `api_protocol` VARCHAR(32) NOT NULL DEFAULT 'chat_completions' AFTER `base_url`,
  ADD CONSTRAINT `chk_ai_providers_api_protocol`
    CHECK (`api_protocol` IN ('chat_completions', 'responses'));
