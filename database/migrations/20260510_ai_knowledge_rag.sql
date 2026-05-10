-- AI Knowledge Base RAG schema and seed data.
-- Idempotent by design: CREATE TABLE IF NOT EXISTS + INSERT ... ON DUPLICATE KEY UPDATE.

SET @legacy_ai_knowledge_maps_exists := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_knowledge_maps'
);
SET @legacy_ai_knowledge_maps_target := CONCAT('ai_knowledge_maps_legacy_20260510_', DATE_FORMAT(NOW(), '%H%i%s'));
SET @rename_legacy_ai_knowledge_maps_sql := IF(
  @legacy_ai_knowledge_maps_exists > 0,
  CONCAT('RENAME TABLE `ai_knowledge_maps` TO `', @legacy_ai_knowledge_maps_target, '`'),
  'DO 0'
);
PREPARE rename_legacy_ai_knowledge_maps_stmt FROM @rename_legacy_ai_knowledge_maps_sql;
EXECUTE rename_legacy_ai_knowledge_maps_stmt;
DEALLOCATE PREPARE rename_legacy_ai_knowledge_maps_stmt;

SET @legacy_ai_knowledge_documents_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_knowledge_documents'
    AND COLUMN_NAME = 'knowledge_map_id'
);
SET @rag_ai_knowledge_documents_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'ai_knowledge_documents'
    AND COLUMN_NAME = 'knowledge_base_id'
);
SET @legacy_ai_knowledge_documents_target := CONCAT('ai_knowledge_documents_legacy_20260510_', DATE_FORMAT(NOW(), '%H%i%s'));
SET @rename_legacy_ai_knowledge_documents_sql := IF(
  @legacy_ai_knowledge_documents_exists > 0 AND @rag_ai_knowledge_documents_exists = 0,
  CONCAT('RENAME TABLE `ai_knowledge_documents` TO `', @legacy_ai_knowledge_documents_target, '`'),
  'DO 0'
);
PREPARE rename_legacy_ai_knowledge_documents_stmt FROM @rename_legacy_ai_knowledge_documents_sql;
EXECUTE rename_legacy_ai_knowledge_documents_stmt;
DEALLOCATE PREPARE rename_legacy_ai_knowledge_documents_stmt;

CREATE TABLE IF NOT EXISTS `ai_knowledge_bases` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '知识库ID',
  `name` VARCHAR(128) NOT NULL COMMENT '知识库名称，列表、绑定、监控展示',
  `code` VARCHAR(128) NOT NULL COMMENT '知识库唯一编码，用于种子幂等和人工识别',
  `description` VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '知识库说明，管理页展示和智能体绑定时辅助选择',
  `chunk_size_chars` INT UNSIGNED NOT NULL DEFAULT 1200 COMMENT '默认分块字符数，重建文档分块时使用',
  `chunk_overlap_chars` INT UNSIGNED NOT NULL DEFAULT 120 COMMENT '默认分块重叠字符数，重建文档分块时使用',
  `default_top_k` INT UNSIGNED NOT NULL DEFAULT 5 COMMENT '检索测试和智能体绑定默认召回条数',
  `default_min_score` DECIMAL(8,4) NOT NULL DEFAULT 0.1000 COMMENT '检索测试和智能体绑定默认最低分',
  `default_max_context_chars` INT UNSIGNED NOT NULL DEFAULT 6000 COMMENT '检索测试和智能体绑定默认上下文字符预算',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1启用 2禁用；运行时只读取启用知识库',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2 COMMENT '1删除 2正常；所有查询默认 is_del=2',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_knowledge_bases_code` (`code`, `is_del`),
  KEY `idx_ai_knowledge_bases_status` (`status`, `is_del`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI知识库';

CREATE TABLE IF NOT EXISTS `ai_knowledge_documents` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '文档ID',
  `knowledge_base_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_knowledge_bases.id',
  `title` VARCHAR(191) NOT NULL COMMENT '文档标题，列表、分块、监控展示',
  `source_type` VARCHAR(32) NOT NULL DEFAULT 'text' COMMENT '来源类型：text/markdown/file；第一版写 text/markdown',
  `source_ref` VARCHAR(512) NOT NULL DEFAULT '' COMMENT '来源标识，如 docs/architecture/04-go-backend-framework.md 或上传文件URL；与 knowledge_base_id、is_del 组成同来源幂等唯一键',
  `content` LONGTEXT NOT NULL COMMENT '文档原文，编辑和重建分块使用',
  `index_status` VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT 'pending/indexing/indexed/failed；分块状态展示和运行过滤',
  `error_message` VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '分块失败原因，管理页展示',
  `last_indexed_at` DATETIME NULL COMMENT '最近成功重建分块时间',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1启用 2禁用；运行时只读取启用文档',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_knowledge_documents_source` (`knowledge_base_id`, `source_ref`, `is_del`),
  KEY `idx_ai_knowledge_documents_base` (`knowledge_base_id`, `status`, `is_del`, `updated_at`),
  KEY `idx_ai_knowledge_documents_index` (`index_status`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI知识库文档';

CREATE TABLE IF NOT EXISTS `ai_knowledge_chunks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '分块ID',
  `knowledge_base_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_knowledge_bases.id，检索时直接过滤',
  `document_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_knowledge_documents.id',
  `chunk_index` INT UNSIGNED NOT NULL COMMENT '同一文档内分块序号，从1开始',
  `title` VARCHAR(191) NOT NULL DEFAULT '' COMMENT '分块标题，默认继承文档标题',
  `content` TEXT NOT NULL COMMENT '分块内容，检索和上下文注入使用',
  `content_chars` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '分块字符数，用于 max_context_chars 预算',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1启用 2禁用；运行时只读取启用分块',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_knowledge_chunks_doc_index` (`document_id`, `chunk_index`, `is_del`),
  KEY `idx_ai_knowledge_chunks_base` (`knowledge_base_id`, `status`, `is_del`, `id`),
  KEY `idx_ai_knowledge_chunks_document` (`document_id`, `status`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI知识库分块';

CREATE TABLE IF NOT EXISTS `ai_agent_knowledge_bases` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '绑定ID',
  `agent_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_agents.id',
  `knowledge_base_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_knowledge_bases.id',
  `top_k` INT UNSIGNED NOT NULL DEFAULT 5 COMMENT '本智能体对此知识库召回条数',
  `min_score` DECIMAL(8,4) NOT NULL DEFAULT 0.1000 COMMENT '本智能体对此知识库最低命中分',
  `max_context_chars` INT UNSIGNED NOT NULL DEFAULT 6000 COMMENT '本智能体对此知识库最大注入字符数',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1启用 2禁用；运行时只加载启用绑定',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_agent_knowledge_base` (`agent_id`, `knowledge_base_id`, `is_del`),
  KEY `idx_ai_agent_knowledge_agent` (`agent_id`, `status`, `is_del`),
  KEY `idx_ai_agent_knowledge_base` (`knowledge_base_id`, `status`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI智能体知识库绑定';

CREATE TABLE IF NOT EXISTS `ai_knowledge_retrievals` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '检索ID',
  `run_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_runs.id',
  `query` TEXT NOT NULL COMMENT '本轮检索查询文本，通常为用户消息正文',
  `status` VARCHAR(16) NOT NULL COMMENT 'success/failed/skipped',
  `total_hits` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '原始命中数量',
  `selected_hits` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '进入上下文的命中数量',
  `duration_ms` INT UNSIGNED NULL COMMENT '检索耗时毫秒',
  `error_message` VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '失败原因',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2 COMMENT '1删除 2正常；运行监控默认只读正常记录',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ai_knowledge_retrievals_run` (`run_id`, `is_del`, `created_at`),
  KEY `idx_ai_knowledge_retrievals_status` (`status`, `is_del`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI知识库检索记录';

CREATE TABLE IF NOT EXISTS `ai_knowledge_retrieval_hits` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '命中ID',
  `retrieval_id` BIGINT UNSIGNED NOT NULL COMMENT 'ai_knowledge_retrievals.id',
  `knowledge_base_id` BIGINT UNSIGNED NOT NULL COMMENT '命中知识库ID',
  `knowledge_base_name` VARCHAR(128) NOT NULL COMMENT '命中时知识库名称快照',
  `document_id` BIGINT UNSIGNED NOT NULL COMMENT '命中文档ID',
  `document_title` VARCHAR(191) NOT NULL COMMENT '命中时文档标题快照',
  `chunk_id` BIGINT UNSIGNED NOT NULL COMMENT '命中分块ID',
  `chunk_index` INT UNSIGNED NOT NULL COMMENT '命中分块序号快照',
  `score` DECIMAL(10,6) NOT NULL DEFAULT 0.000000 COMMENT '检索评分',
  `rank_no` INT UNSIGNED NOT NULL COMMENT '本次检索排序，从1开始',
  `content_snapshot` TEXT NOT NULL COMMENT '命中内容快照，运行监控和问题复盘使用',
  `status` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1进入上下文 2跳过',
  `skip_reason` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '跳过原因：low_score/context_limit',
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2 COMMENT '1删除 2正常',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ai_knowledge_hits_retrieval` (`retrieval_id`, `is_del`, `status`, `rank_no`),
  KEY `idx_ai_knowledge_hits_chunk` (`chunk_id`, `is_del`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI知识库检索命中';

INSERT INTO `ai_knowledge_bases` (
  `name`, `code`, `description`, `chunk_size_chars`, `chunk_overlap_chars`,
  `default_top_k`, `default_min_score`, `default_max_context_chars`, `status`, `is_del`
) VALUES (
  'admin_go 项目架构知识库',
  'admin_go_project_architecture',
  'admin_go 本地项目架构、AI 模块和前端页面结构的初始化知识库',
  1200,
  120,
  5,
  0.1000,
  6000,
  1,
  2
) ON DUPLICATE KEY UPDATE
  `name` = VALUES(`name`),
  `description` = VALUES(`description`),
  `chunk_size_chars` = VALUES(`chunk_size_chars`),
  `chunk_overlap_chars` = VALUES(`chunk_overlap_chars`),
  `default_top_k` = VALUES(`default_top_k`),
  `default_min_score` = VALUES(`default_min_score`),
  `default_max_context_chars` = VALUES(`default_max_context_chars`),
  `status` = VALUES(`status`),
  `is_del` = VALUES(`is_del`);

SET @admin_go_project_architecture_kb_id := (
  SELECT `id`
  FROM `ai_knowledge_bases`
  WHERE `code` = 'admin_go_project_architecture' AND `is_del` = 2
  LIMIT 1
);

SET @doc_content_project_principles := '项目总原则：E:\\admin_go 是 open-source-first admin rewrite workspace，不是闭门造车实验场。冷启动必须先读当前状态，再读架构、契约、测试文档，再按 agent 角色接一个窄切片，最后才改代码、跑验证、同步文档。Linus 三问固定是：这是真问题还是臆想？有更简单的方法吗？会破坏已有前端、接口、登录和权限吗？不可协商原则是代码质量、架构质量、文档真实性永远优先。没有验证证据，不准说完成；文档与运行时冲突时，以运行时为准并修正文档。';
SET @doc_content_go_backend := 'Go 后端架构：admin_back_go 采用 Gin modular monolith。顶层方向是 cmd -> bootstrap -> server -> module -> platform，业务模块内部默认 route -> handler -> service -> repository -> model。不要把 Go 写成 Java，不要制造 ServiceImpl、Manager、Factory、BO、VO、Converter everywhere。旧 PHP 只提供业务事实和兼容桥，不能污染新 REST 设计。admin-api 负责 HTTP，admin-worker 负责队列消费和定时调度。';
SET @doc_content_quality_rules := '开发质量规则：代码要简单、明确、可测、无隐藏兜底、无无主 goroutine。架构边界要清楚，职责单一，尊重既定分层。数据库迁移必须幂等，可重复执行；状态字段遵守 status=1 启用、status=2 禁用，is_del=1 删除、is_del=2 正常。前后端字段以契约和运行时事实为准，不靠猜测。触碰代码后必须给出验证命令和结果。';
SET @doc_content_ai_current_facts := 'AI 模块当前事实：AI 产品菜单包含 /ai/providers、/ai/agents、/ai/knowledge、/ai/tools、/ai/runs、/ai/chat。当前 AI 链路已有 provider、agent、chat、run、tool 基础能力。live DB 已有 ai_agents、ai_conversations、ai_messages、ai_runs、ai_run_events、ai_tools、ai_agent_tools、ai_tool_calls；知识库新设计使用 ai_knowledge_bases、ai_knowledge_documents、ai_knowledge_chunks、ai_agent_knowledge_bases、ai_knowledge_retrievals、ai_knowledge_retrieval_hits。';
SET @doc_content_ai_chat_runtime := 'AI 对话运行链路：聊天是 WebSocket-first，不走 SSE。aichat 创建 ai_runs，加载智能体和工具，调用 provider，保存助手消息并完成 run。知识库检索应该插入在第一次 StreamChat 前：根据当前会话 agent_id 读取启用绑定，对本轮用户消息执行本地检索，按 top_k、min_score、max_context_chars 筛选片段，把选中片段注入本轮 user content，并写入检索记录和命中记录。默认未绑定知识库时不能改变现有聊天行为。';
SET @doc_content_vue_ai_pages := 'Vue 前端 AI 页面结构：当前前端 AI 菜单按产品能力分为供应商配置、智能体配置、知识库、工具、运行监控、对话。知识库页面负责知识库、文档、分块和检索测试；智能体配置页负责绑定可读取的知识库以及 top_k、min_score、max_context_chars；运行监控页展示每轮检索、耗时、命中数量、选中数量、错误和命中快照；对话页只消费最终运行链路，不直接越过后端读取知识库。';

UPDATE `ai_knowledge_chunks` AS c
JOIN `ai_knowledge_documents` AS d ON d.`id` = c.`document_id`
SET
  c.`status` = 2,
  c.`is_del` = 1,
  c.`updated_at` = CURRENT_TIMESTAMP
WHERE d.`knowledge_base_id` = @admin_go_project_architecture_kb_id
  AND d.`source_ref` IN (
    CONCAT('seed', '/admin_go/project-principles'),
    CONCAT('seed', '/admin_go/go-backend-architecture'),
    CONCAT('seed', '/admin_go/development-quality-rules'),
    CONCAT('seed', '/admin_go/ai-current-facts'),
    CONCAT('seed', '/admin_go/ai-chat-runtime-flow'),
    CONCAT('seed', '/admin_go/vue-ai-page-structure')
  )
  AND c.`is_del` = 2;

UPDATE `ai_knowledge_documents`
SET
  `status` = 2,
  `is_del` = 1,
  `updated_at` = CURRENT_TIMESTAMP
WHERE `knowledge_base_id` = @admin_go_project_architecture_kb_id
  AND `source_ref` IN (
    CONCAT('seed', '/admin_go/project-principles'),
    CONCAT('seed', '/admin_go/go-backend-architecture'),
    CONCAT('seed', '/admin_go/development-quality-rules'),
    CONCAT('seed', '/admin_go/ai-current-facts'),
    CONCAT('seed', '/admin_go/ai-chat-runtime-flow'),
    CONCAT('seed', '/admin_go/vue-ai-page-structure')
  )
  AND `is_del` = 2;

INSERT INTO `ai_knowledge_documents` (
  `knowledge_base_id`, `title`, `source_type`, `source_ref`, `content`,
  `index_status`, `error_message`, `last_indexed_at`, `status`, `is_del`
) VALUES
  (@admin_go_project_architecture_kb_id, '项目总原则', 'markdown', 'docs/architecture/00-open-source-first.md', @doc_content_project_principles, 'indexed', '', CURRENT_TIMESTAMP, 1, 2),
  (@admin_go_project_architecture_kb_id, 'Go 后端架构', 'markdown', 'docs/architecture/04-go-backend-framework.md', @doc_content_go_backend, 'indexed', '', CURRENT_TIMESTAMP, 1, 2),
  (@admin_go_project_architecture_kb_id, '开发质量规则', 'markdown', 'docs/architecture/05-development-quality-rules.md', @doc_content_quality_rules, 'indexed', '', CURRENT_TIMESTAMP, 1, 2),
  (@admin_go_project_architecture_kb_id, 'AI 模块当前事实', 'markdown', 'docs/migration/current-status.md#ai', @doc_content_ai_current_facts, 'indexed', '', CURRENT_TIMESTAMP, 1, 2),
  (@admin_go_project_architecture_kb_id, 'AI 对话运行链路', 'text', 'admin_back_go/internal/module/aichat/service.go', @doc_content_ai_chat_runtime, 'indexed', '', CURRENT_TIMESTAMP, 1, 2),
  (@admin_go_project_architecture_kb_id, 'Vue 前端 AI 页面结构', 'text', 'admin_front_ts/src/views/Main/ai', @doc_content_vue_ai_pages, 'indexed', '', CURRENT_TIMESTAMP, 1, 2)
ON DUPLICATE KEY UPDATE
  `title` = VALUES(`title`),
  `source_type` = VALUES(`source_type`),
  `content` = VALUES(`content`),
  `index_status` = VALUES(`index_status`),
  `error_message` = VALUES(`error_message`),
  `last_indexed_at` = VALUES(`last_indexed_at`),
  `status` = VALUES(`status`),
  `is_del` = VALUES(`is_del`);

INSERT INTO `ai_knowledge_chunks` (
  `knowledge_base_id`, `document_id`, `chunk_index`, `title`, `content`, `content_chars`, `status`, `is_del`
)
SELECT
  `knowledge_base_id`,
  `id`,
  1,
  `title`,
  `content`,
  CHAR_LENGTH(`content`),
  1,
  2
FROM `ai_knowledge_documents`
WHERE `knowledge_base_id` = @admin_go_project_architecture_kb_id
  AND `source_ref` IN (
    'docs/architecture/00-open-source-first.md',
    'docs/architecture/04-go-backend-framework.md',
    'docs/architecture/05-development-quality-rules.md',
    'docs/migration/current-status.md#ai',
    'admin_back_go/internal/module/aichat/service.go',
    'admin_front_ts/src/views/Main/ai'
  )
  AND `is_del` = 2
ON DUPLICATE KEY UPDATE
  `knowledge_base_id` = VALUES(`knowledge_base_id`),
  `title` = VALUES(`title`),
  `content` = VALUES(`content`),
  `content_chars` = VALUES(`content_chars`),
  `status` = VALUES(`status`),
  `is_del` = VALUES(`is_del`);
