ALTER TABLE `payment_callback_events`
  ADD COLUMN `dedupe_key` BINARY(32) NULL AFTER `provider`;

ALTER TABLE `payment_orders`
  ADD COLUMN `alipay_trade_no_identity` VARCHAR(64) NULL AFTER `alipay_trade_no`;

ALTER TABLE `ai_agents`
  ADD COLUMN `provider_model_id` BIGINT UNSIGNED NULL AFTER `provider_id`;

ALTER TABLE `ai_reply_commands`
  ADD COLUMN `run_id` BIGINT UNSIGNED NULL AFTER `conversation_id`;
