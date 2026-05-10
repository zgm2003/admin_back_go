DROP TABLE IF EXISTS `ai_messages`;
DROP TABLE IF EXISTS `ai_conversations`;

CREATE TABLE `ai_conversations` (
  `id` int unsigned NOT NULL AUTO_INCREMENT COMMENT '会话ID',
  `user_id` int unsigned NOT NULL COMMENT '当前用户ID',
  `agent_id` int unsigned NOT NULL COMMENT 'ai_agents.id',
  `title` varchar(100) NOT NULL DEFAULT '' COMMENT '会话标题',
  `last_message_at` datetime NULL DEFAULT NULL COMMENT '上次对话时间',
  `is_del` tinyint unsigned NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ai_conversations_user_agent_del_last_message` (`user_id`, `agent_id`, `is_del`, `last_message_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI会话';

CREATE TABLE `ai_messages` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '消息ID',
  `conversation_id` int unsigned NOT NULL COMMENT 'ai_conversations.id',
  `role` tinyint unsigned NOT NULL COMMENT '1用户 2助手',
  `content_type` varchar(32) NOT NULL DEFAULT 'text' COMMENT '内容类型，MVP只写text',
  `content` longtext NOT NULL COMMENT '消息内容',
  `meta_json` json NULL COMMENT '消息扩展元数据：attachments/runtime_params/blocks/feedback',
  `is_del` tinyint unsigned NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ai_messages_conversation_del_id` (`conversation_id`, `is_del`, `id`),
  CONSTRAINT `fk_ai_messages_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI消息';
