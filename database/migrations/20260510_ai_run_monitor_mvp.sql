-- AI run monitor minimal schema.
-- Scope: token statistics only, no billing/cost, no provider task snapshot garbage.

DROP TABLE IF EXISTS `ai_run_events`;
DROP TABLE IF EXISTS `ai_runs`;

CREATE TABLE `ai_runs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '运行ID',
  `conversation_id` int unsigned NOT NULL COMMENT 'ai_conversations.id',
  `request_id` varchar(64) NOT NULL COMMENT '客户端本轮请求ID',
  `user_message_id` bigint unsigned NOT NULL COMMENT '本轮用户消息ID',
  `assistant_message_id` bigint unsigned NULL DEFAULT NULL COMMENT '完成后写入的助手消息ID',
  `user_id` int unsigned NOT NULL COMMENT '发起用户ID',
  `agent_id` bigint unsigned NOT NULL COMMENT 'ai_agents.id',
  `provider_id` bigint unsigned NOT NULL COMMENT 'ai_providers.id',
  `model_id` varchar(191) NOT NULL COMMENT '实际调用模型ID',
  `model_display_name` varchar(191) NOT NULL DEFAULT '' COMMENT '实际调用模型展示名',
  `status` varchar(16) NOT NULL COMMENT 'running/success/failed/canceled/timeout',
  `prompt_tokens` int unsigned NOT NULL DEFAULT 0 COMMENT '输入token',
  `completion_tokens` int unsigned NOT NULL DEFAULT 0 COMMENT '输出token',
  `total_tokens` int unsigned NOT NULL DEFAULT 0 COMMENT '总token',
  `duration_ms` int unsigned NULL DEFAULT NULL COMMENT '运行耗时毫秒，终态后写入',
  `error_message` varchar(1024) NOT NULL DEFAULT '' COMMENT '失败/取消/超时原因',
  `started_at` datetime NULL DEFAULT NULL COMMENT '开始调用模型时间',
  `finished_at` datetime NULL DEFAULT NULL COMMENT '进入终态时间',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_runs_conversation_request` (`conversation_id`, `request_id`),
  UNIQUE KEY `uk_ai_runs_user_message` (`user_message_id`),
  KEY `idx_ai_runs_created` (`created_at`, `id`),
  KEY `idx_ai_runs_status_created` (`status`, `created_at`, `id`),
  KEY `idx_ai_runs_user_created` (`user_id`, `created_at`, `id`),
  KEY `idx_ai_runs_agent_created` (`agent_id`, `created_at`, `id`),
  KEY `idx_ai_runs_provider_created` (`provider_id`, `created_at`, `id`),
  KEY `idx_ai_runs_conversation_created` (`conversation_id`, `created_at`, `id`),
  CONSTRAINT `fk_ai_runs_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ai_runs_user_message` FOREIGN KEY (`user_message_id`) REFERENCES `ai_messages` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ai_runs_assistant_message` FOREIGN KEY (`assistant_message_id`) REFERENCES `ai_messages` (`id`) ON DELETE SET NULL,
  CONSTRAINT `chk_ai_runs_status` CHECK (`status` IN ('running', 'success', 'failed', 'canceled', 'timeout'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI运行监控记录';

CREATE TABLE `ai_run_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '事件ID',
  `run_id` bigint unsigned NOT NULL COMMENT 'ai_runs.id',
  `seq` int unsigned NOT NULL COMMENT '同一run内事件序号',
  `event_type` varchar(32) NOT NULL COMMENT 'start/completed/failed/canceled/timeout',
  `message` varchar(1024) NOT NULL DEFAULT '' COMMENT '事件说明或错误原因',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '事件时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_run_events_run_seq` (`run_id`, `seq`),
  KEY `idx_ai_run_events_run_id` (`run_id`, `id`),
  KEY `idx_ai_run_events_type_created` (`event_type`, `created_at`, `id`),
  CONSTRAINT `fk_ai_run_events_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE CASCADE,
  CONSTRAINT `chk_ai_run_events_type` CHECK (`event_type` IN ('start', 'completed', 'failed', 'canceled', 'timeout'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI运行监控事件';

