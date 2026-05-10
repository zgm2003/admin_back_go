-- AI tool runtime MVP.
-- Scope: local function tools only. First seed: admin_user_count.
-- No placeholder columns: every column is used by management UI, agent binding,
-- model tool definitions, executor routing, runtime timeout, or run-monitor audit.

CREATE TABLE IF NOT EXISTS `ai_tools` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '工具ID',
  `name` varchar(128) NOT NULL COMMENT '工具名称，管理页和运行监控展示',
  `code` varchar(128) NOT NULL COMMENT '工具唯一编码，传给模型作为function name',
  `description` varchar(1024) NOT NULL DEFAULT '' COMMENT '工具说明，传给模型作为function description',
  `executor` varchar(64) NOT NULL COMMENT '本地执行器编码，用于Go executor registry路由',
  `parameters_json` json NOT NULL COMMENT '工具参数JSON Schema，传给模型并用于入参校验',
  `result_schema_json` json NOT NULL COMMENT '工具返回JSON Schema，用于结果校验和运行监控展示',
  `risk_level` varchar(16) NOT NULL COMMENT '风险等级：low/medium/high',
  `timeout_ms` int unsigned NOT NULL DEFAULT 3000 COMMENT '执行超时毫秒，运行时context timeout',
  `status` tinyint unsigned NOT NULL DEFAULT 1 COMMENT '1启用 2禁用',
  `is_del` tinyint unsigned NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_tools_code` (`code`),
  KEY `idx_ai_tools_status_del` (`status`, `is_del`, `id`),
  CONSTRAINT `chk_ai_tools_risk_level` CHECK (`risk_level` IN ('low', 'medium', 'high')),
  CONSTRAINT `chk_ai_tools_status` CHECK (`status` IN (1, 2)),
  CONSTRAINT `chk_ai_tools_is_del` CHECK (`is_del` IN (1, 2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI工具定义';

CREATE TABLE IF NOT EXISTS `ai_agent_tools` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '绑定ID',
  `agent_id` bigint unsigned NOT NULL COMMENT 'ai_agents.id',
  `tool_id` bigint unsigned NOT NULL COMMENT 'ai_tools.id',
  `status` tinyint unsigned NOT NULL DEFAULT 1 COMMENT '1启用 2禁用；运行时只加载启用绑定',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_agent_tools_agent_tool` (`agent_id`, `tool_id`),
  KEY `idx_ai_agent_tools_agent_status` (`agent_id`, `status`, `id`),
  KEY `idx_ai_agent_tools_tool_status` (`tool_id`, `status`, `id`),
  CONSTRAINT `fk_ai_agent_tools_agent` FOREIGN KEY (`agent_id`) REFERENCES `ai_agents` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ai_agent_tools_tool` FOREIGN KEY (`tool_id`) REFERENCES `ai_tools` (`id`) ON DELETE CASCADE,
  CONSTRAINT `chk_ai_agent_tools_status` CHECK (`status` IN (1, 2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI智能体工具绑定';

CREATE TABLE IF NOT EXISTS `ai_tool_calls` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '工具调用ID',
  `run_id` bigint unsigned NOT NULL COMMENT 'ai_runs.id',
  `tool_id` bigint unsigned NOT NULL COMMENT 'ai_tools.id',
  `tool_code` varchar(128) NOT NULL COMMENT '调用时工具编码快照',
  `tool_name` varchar(128) NOT NULL COMMENT '调用时工具名称快照',
  `call_id` varchar(128) NULL DEFAULT NULL COMMENT '模型返回的tool_call_id/call_id，用于回传工具结果',
  `status` varchar(16) NOT NULL COMMENT 'running/success/failed/timeout',
  `arguments_json` json NOT NULL COMMENT '模型传入参数',
  `result_json` json NULL COMMENT '工具返回结果',
  `error_message` varchar(1024) NOT NULL DEFAULT '' COMMENT '失败或超时原因',
  `duration_ms` int unsigned NULL DEFAULT NULL COMMENT '执行耗时毫秒，终态后写入',
  `started_at` datetime NOT NULL COMMENT '开始执行时间',
  `finished_at` datetime NULL DEFAULT NULL COMMENT '结束时间',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_tool_calls_run_call` (`run_id`, `call_id`),
  KEY `idx_ai_tool_calls_run_id` (`run_id`, `id`),
  KEY `idx_ai_tool_calls_tool_created` (`tool_id`, `created_at`, `id`),
  KEY `idx_ai_tool_calls_status_created` (`status`, `created_at`, `id`),
  CONSTRAINT `fk_ai_tool_calls_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ai_tool_calls_tool` FOREIGN KEY (`tool_id`) REFERENCES `ai_tools` (`id`) ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_tool_calls_status` CHECK (`status` IN ('running', 'success', 'failed', 'timeout'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AI工具调用记录';

INSERT INTO `ai_tools` (
  `name`,
  `code`,
  `description`,
  `executor`,
  `parameters_json`,
  `result_schema_json`,
  `risk_level`,
  `timeout_ms`,
  `status`,
  `is_del`
) VALUES (
  '查询当前用户量',
  'admin_user_count',
  '查询后台当前用户数量，只返回总数、启用数、禁用数，不返回任何用户个人信息。',
  'admin_user_count',
  CAST('{"type":"object","properties":{},"additionalProperties":false}' AS JSON),
  CAST('{"type":"object","properties":{"total_users":{"type":"integer","minimum":0},"enabled_users":{"type":"integer","minimum":0},"disabled_users":{"type":"integer","minimum":0}},"required":["total_users","enabled_users","disabled_users"],"additionalProperties":false}' AS JSON),
  'low',
  3000,
  1,
  2
) AS new_tool
ON DUPLICATE KEY UPDATE
  `name` = new_tool.`name`,
  `description` = new_tool.`description`,
  `executor` = new_tool.`executor`,
  `parameters_json` = new_tool.`parameters_json`,
  `result_schema_json` = new_tool.`result_schema_json`,
  `risk_level` = new_tool.`risk_level`,
  `timeout_ms` = new_tool.`timeout_ms`,
  `status` = new_tool.`status`,
  `is_del` = new_tool.`is_del`;

SET @admin_user_count_tool_id := (
  SELECT `id`
  FROM `ai_tools`
  WHERE `code` = 'admin_user_count' AND `is_del` = 2
  LIMIT 1
);

INSERT INTO `ai_agent_tools` (`agent_id`, `tool_id`, `status`)
SELECT a.`id`, @admin_user_count_tool_id, 1
FROM `ai_agents` AS a
WHERE @admin_user_count_tool_id IS NOT NULL
  AND a.`is_del` = 2
  AND a.`status` = 1
  AND JSON_CONTAINS(a.`scenes_json`, JSON_QUOTE('chat'))
ON DUPLICATE KEY UPDATE
  `status` = VALUES(`status`),
  `updated_at` = CURRENT_TIMESTAMP;
