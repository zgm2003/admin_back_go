DROP TEMPORARY TABLE IF EXISTS `_ai_payment_integrity_contract_guard`;
CREATE TEMPORARY TABLE `_ai_payment_integrity_contract_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

-- Identity columns must be complete and collision-free before constraints exist.
INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `payment_callback_events`
WHERE `dedupe_key` IS NULL OR OCTET_LENGTH(`dedupe_key`) <> 32;

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `dedupe_key`
  FROM `payment_callback_events`
  GROUP BY `dedupe_key`
  HAVING COUNT(*) <> 1
) AS `duplicate_callback_identity`;

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `payment_orders`
WHERE (`alipay_trade_no` = '' AND `alipay_trade_no_identity` IS NOT NULL)
   OR (`alipay_trade_no` <> '' AND (
     `alipay_trade_no_identity` IS NULL
     OR BINARY `alipay_trade_no_identity` <> BINARY `alipay_trade_no`
   ));

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `alipay_trade_no_identity`
  FROM `payment_orders`
  WHERE `alipay_trade_no_identity` IS NOT NULL
  GROUP BY `alipay_trade_no_identity`
  HAVING COUNT(*) <> 1
) AS `duplicate_trade_identity`;

-- Provider, Agent and Conversation ownership must already be exact.
INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_provider_models` AS provider_model
LEFT JOIN `ai_providers` AS provider_row ON provider_row.`id` = provider_model.`provider_id`
WHERE provider_row.`id` IS NULL;

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_agents` AS agent
LEFT JOIN `ai_provider_models` AS provider_model
  ON provider_model.`id` = agent.`provider_model_id`
 AND provider_model.`provider_id` = agent.`provider_id`
 AND BINARY provider_model.`model_id` = BINARY agent.`model_id`
 AND provider_model.`model_kind` = 'chat'
WHERE agent.`provider_model_id` IS NULL OR provider_model.`id` IS NULL;

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_conversations` AS conversation
LEFT JOIN `users` AS user_row ON user_row.`id` = conversation.`user_id`
LEFT JOIN `ai_agents` AS agent ON agent.`id` = conversation.`agent_id`
WHERE user_row.`id` IS NULL OR agent.`id` IS NULL;

-- Command is a direct one-to-one child of the Run and must carry the same owner facts.
INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_reply_commands`
WHERE `run_id` IS NULL
   OR `user_id` <= 0 OR `user_id` > 4294967295
   OR `conversation_id` <= 0 OR `conversation_id` > 4294967295
   OR `user_message_id` <= 0
   OR (`assistant_message_id` IS NOT NULL AND `assistant_message_id` <= 0);

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_reply_commands` AS command_row
LEFT JOIN `ai_runs` AS run_row
  ON run_row.`id` = command_row.`run_id`
 AND run_row.`user_id` = command_row.`user_id`
 AND run_row.`conversation_id` = command_row.`conversation_id`
 AND run_row.`user_message_id` = command_row.`user_message_id`
 AND BINARY run_row.`request_id` = BINARY command_row.`request_id`
LEFT JOIN `ai_conversations` AS conversation
  ON conversation.`id` = command_row.`conversation_id`
 AND conversation.`user_id` = command_row.`user_id`
LEFT JOIN `ai_messages` AS user_message
  ON user_message.`id` = command_row.`user_message_id`
 AND user_message.`conversation_id` = command_row.`conversation_id`
LEFT JOIN `ai_messages` AS assistant_message
  ON assistant_message.`id` = command_row.`assistant_message_id`
 AND assistant_message.`conversation_id` = command_row.`conversation_id`
WHERE run_row.`id` IS NULL
   OR conversation.`id` IS NULL
   OR user_message.`id` IS NULL
   OR (command_row.`assistant_message_id` IS NOT NULL AND assistant_message.`id` IS NULL);

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `run_id`
  FROM `ai_reply_commands`
  GROUP BY `run_id`
  HAVING COUNT(*) <> 1
) AS `duplicate_command_run`;

-- Attempt, Plan, Document and Memory relations must preserve their owner tuple.
INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_provider_attempts` AS attempt
LEFT JOIN `ai_reply_commands` AS command_row
  ON command_row.`id` = attempt.`command_id`
 AND command_row.`run_id` = attempt.`run_id`
LEFT JOIN `ai_context_plans` AS context_plan
  ON context_plan.`id` = attempt.`context_plan_id`
 AND context_plan.`run_id` = attempt.`run_id`
WHERE (attempt.`command_id` IS NOT NULL AND command_row.`id` IS NULL)
   OR (attempt.`context_plan_id` IS NOT NULL AND context_plan.`id` IS NULL);

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_context_documents` AS document
LEFT JOIN `ai_messages` AS source_message
  ON source_message.`id` = document.`source_message_id`
 AND source_message.`conversation_id` = document.`conversation_id`
WHERE document.`source_message_id` IS NOT NULL AND source_message.`id` IS NULL;

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_conversation_memories` AS memory_row
LEFT JOIN `ai_messages` AS from_message
  ON from_message.`id` = memory_row.`from_message_id`
 AND from_message.`conversation_id` = memory_row.`conversation_id`
LEFT JOIN `ai_messages` AS through_message
  ON through_message.`id` = memory_row.`through_message_id`
 AND through_message.`conversation_id` = memory_row.`conversation_id`
LEFT JOIN `ai_conversation_memories` AS previous_memory
  ON previous_memory.`id` = memory_row.`previous_memory_id`
 AND previous_memory.`conversation_id` = memory_row.`conversation_id`
 AND previous_memory.`context_profile_id_snapshot` = memory_row.`context_profile_id_snapshot`
WHERE from_message.`id` IS NULL
   OR through_message.`id` IS NULL
   OR (memory_row.`previous_memory_id` IS NOT NULL AND previous_memory.`id` IS NULL)
   OR memory_row.`previous_memory_id` = memory_row.`id`;

-- Image files and legacy text/image task projections must point at one Run only.
INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `ai_image_files` AS image_file
LEFT JOIN `ai_image_tasks` AS image_task ON image_task.`id` = image_file.`task_id`
LEFT JOIN `ai_image_files` AS related_file ON related_file.`id` = image_file.`related_file_id`
WHERE image_task.`id` IS NULL
   OR (image_file.`related_file_id` IS NOT NULL AND related_file.`id` IS NULL);

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT task.`run_id`
  FROM `ai_text_tasks` AS task
  LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id`
  WHERE run_row.`id` IS NULL
  UNION ALL
  SELECT task.`run_id`
  FROM `ai_image_tasks` AS task
  LEFT JOIN `ai_runs` AS run_row ON run_row.`id` = task.`run_id`
  WHERE run_row.`id` IS NULL
) AS `orphan_task_run`;

INSERT INTO `_ai_payment_integrity_contract_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM (
  SELECT `run_id` FROM `ai_text_tasks` GROUP BY `run_id` HAVING COUNT(*) <> 1
  UNION ALL
  SELECT `run_id` FROM `ai_image_tasks` GROUP BY `run_id` HAVING COUNT(*) <> 1
) AS `duplicate_task_run`;

ALTER TABLE `payment_callback_events`
  MODIFY COLUMN `dedupe_key` BINARY(32) NOT NULL,
  ADD UNIQUE KEY `uk_payment_callback_events_dedupe` (`dedupe_key`);

ALTER TABLE `payment_orders`
  MODIFY COLUMN `alipay_trade_no_identity` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  ADD UNIQUE KEY `uk_payment_orders_alipay_trade_identity` (`alipay_trade_no_identity`),
  ADD CONSTRAINT `chk_payment_orders_alipay_trade_identity`
    CHECK ((`alipay_trade_no` = '' AND `alipay_trade_no_identity` IS NULL)
      OR (`alipay_trade_no` <> '' AND BINARY `alipay_trade_no_identity` = BINARY `alipay_trade_no`));

ALTER TABLE `ai_provider_models`
  ADD UNIQUE KEY `uk_ai_provider_models_id_provider_model` (`id`, `provider_id`, `model_id`),
  ADD CONSTRAINT `fk_ai_provider_models_provider`
    FOREIGN KEY (`provider_id`) REFERENCES `ai_providers` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_agents`
  MODIFY COLUMN `provider_model_id` BIGINT UNSIGNED NOT NULL,
  ADD KEY `idx_ai_agents_provider_model_identity` (`provider_model_id`, `provider_id`, `model_id`),
  ADD CONSTRAINT `fk_ai_agents_provider_model`
    FOREIGN KEY (`provider_model_id`, `provider_id`, `model_id`)
    REFERENCES `ai_provider_models` (`id`, `provider_id`, `model_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_conversations`
  MODIFY COLUMN `agent_id` BIGINT UNSIGNED NOT NULL,
  ADD UNIQUE KEY `uk_ai_conversations_id_user` (`id`, `user_id`),
  ADD KEY `idx_ai_conversations_agent` (`agent_id`),
  ADD CONSTRAINT `fk_ai_conversations_user`
    FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_conversations_agent`
    FOREIGN KEY (`agent_id`) REFERENCES `ai_agents` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_messages`
  ADD UNIQUE KEY `uk_ai_messages_id_conversation` (`id`, `conversation_id`);

ALTER TABLE `ai_runs`
  ADD UNIQUE KEY `uk_ai_runs_command_owner`
    (`id`, `user_id`, `conversation_id`, `user_message_id`, `request_id`);

ALTER TABLE `ai_reply_commands`
  MODIFY COLUMN `user_id` INT UNSIGNED NOT NULL,
  MODIFY COLUMN `conversation_id` INT UNSIGNED NOT NULL,
  MODIFY COLUMN `run_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `user_message_id` BIGINT UNSIGNED NOT NULL,
  MODIFY COLUMN `assistant_message_id` BIGINT UNSIGNED NULL,
  ADD UNIQUE KEY `uk_ai_reply_commands_run` (`run_id`),
  ADD UNIQUE KEY `uk_ai_reply_commands_id_run` (`id`, `run_id`),
  ADD KEY `idx_ai_reply_commands_run_owner`
    (`run_id`, `user_id`, `conversation_id`, `user_message_id`, `request_id`),
  ADD KEY `idx_ai_reply_commands_conversation_owner` (`conversation_id`, `user_id`),
  ADD KEY `idx_ai_reply_commands_user_message_owner` (`user_message_id`, `conversation_id`),
  ADD KEY `idx_ai_reply_commands_assistant_message_owner` (`assistant_message_id`, `conversation_id`),
  ADD CONSTRAINT `fk_ai_reply_commands_run_owner`
    FOREIGN KEY (`run_id`, `user_id`, `conversation_id`, `user_message_id`, `request_id`)
    REFERENCES `ai_runs` (`id`, `user_id`, `conversation_id`, `user_message_id`, `request_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_reply_commands_conversation_owner`
    FOREIGN KEY (`conversation_id`, `user_id`)
    REFERENCES `ai_conversations` (`id`, `user_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_reply_commands_user_message_owner`
    FOREIGN KEY (`user_message_id`, `conversation_id`)
    REFERENCES `ai_messages` (`id`, `conversation_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_reply_commands_assistant_message_owner`
    FOREIGN KEY (`assistant_message_id`, `conversation_id`)
    REFERENCES `ai_messages` (`id`, `conversation_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_context_plans`
  ADD UNIQUE KEY `uk_ai_context_plans_id_run` (`id`, `run_id`);

ALTER TABLE `ai_provider_attempts`
  DROP FOREIGN KEY `fk_ai_provider_attempts_context_plan`,
  ADD KEY `idx_ai_provider_attempts_command_run` (`command_id`, `run_id`),
  ADD KEY `idx_ai_provider_attempts_context_plan_run` (`context_plan_id`, `run_id`),
  ADD CONSTRAINT `fk_ai_provider_attempts_command_run`
    FOREIGN KEY (`command_id`, `run_id`)
    REFERENCES `ai_reply_commands` (`id`, `run_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_provider_attempts_context_plan_run`
    FOREIGN KEY (`context_plan_id`, `run_id`)
    REFERENCES `ai_context_plans` (`id`, `run_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_context_documents`
  DROP FOREIGN KEY `fk_ai_context_documents_source_message`,
  ADD KEY `idx_ai_context_documents_source_message_owner` (`source_message_id`, `conversation_id`),
  ADD CONSTRAINT `fk_ai_context_documents_source_message_owner`
    FOREIGN KEY (`source_message_id`, `conversation_id`)
    REFERENCES `ai_messages` (`id`, `conversation_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_conversation_memories`
  ADD UNIQUE KEY `uk_ai_conversation_memories_owner`
    (`id`, `conversation_id`, `context_profile_id_snapshot`),
  ADD KEY `idx_ai_conversation_memories_previous_owner`
    (`previous_memory_id`, `conversation_id`, `context_profile_id_snapshot`),
  ADD KEY `idx_ai_conversation_memories_from_message_owner` (`from_message_id`, `conversation_id`),
  ADD KEY `idx_ai_conversation_memories_through_message_owner` (`through_message_id`, `conversation_id`);

ALTER TABLE `ai_conversation_memories`
  DROP FOREIGN KEY `fk_ai_conversation_memories_previous`,
  DROP FOREIGN KEY `fk_ai_conversation_memories_from_message`,
  DROP FOREIGN KEY `fk_ai_conversation_memories_through_message`,
  ADD CONSTRAINT `fk_ai_conversation_memories_previous_owner`
    FOREIGN KEY (`previous_memory_id`, `conversation_id`, `context_profile_id_snapshot`)
    REFERENCES `ai_conversation_memories` (`id`, `conversation_id`, `context_profile_id_snapshot`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_conversation_memories_from_message_owner`
    FOREIGN KEY (`from_message_id`, `conversation_id`)
    REFERENCES `ai_messages` (`id`, `conversation_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_conversation_memories_through_message_owner`
    FOREIGN KEY (`through_message_id`, `conversation_id`)
    REFERENCES `ai_messages` (`id`, `conversation_id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_image_files`
  ADD CONSTRAINT `fk_ai_image_files_task`
    FOREIGN KEY (`task_id`) REFERENCES `ai_image_tasks` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_ai_image_files_related`
    FOREIGN KEY (`related_file_id`) REFERENCES `ai_image_files` (`id`)
    ON UPDATE RESTRICT ON DELETE SET NULL;

ALTER TABLE `ai_text_tasks`
  ADD UNIQUE KEY `uk_ai_text_tasks_run` (`run_id`);
ALTER TABLE `ai_text_tasks`
  DROP INDEX `idx_ai_text_tasks_run`;

ALTER TABLE `ai_image_tasks`
  ADD UNIQUE KEY `uk_ai_image_tasks_run` (`run_id`);
ALTER TABLE `ai_image_tasks`
  DROP INDEX `idx_ai_image_tasks_run`;

DROP TEMPORARY TABLE `_ai_payment_integrity_contract_guard`;
