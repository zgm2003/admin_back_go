ALTER TABLE `ai_providers`
  ADD COLUMN `file_input_mode` VARCHAR(32) NOT NULL DEFAULT 'disabled' AFTER `base_url`,
  ADD CONSTRAINT `chk_ai_providers_file_input_mode`
    CHECK (`file_input_mode` IN ('disabled', 'chat_completions'));
