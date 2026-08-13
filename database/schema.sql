
/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!50503 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `address` (
  `id` int unsigned NOT NULL AUTO_INCREMENT COMMENT 'Region id',
  `parent_id` int unsigned NOT NULL DEFAULT '0' COMMENT 'parent region id; 0 means root',
  `code` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '区划编码',
  `name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '区划名称',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT 'soft delete: 1 deleted 2 normal',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Created time',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Updated time',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_address_code` (`code`) USING BTREE,
  KEY `idx_address_parent` (`parent_id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='区域表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_agent_tools` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '绑定ID',
  `agent_id` bigint unsigned NOT NULL COMMENT 'ai_agents.id',
  `tool_id` bigint unsigned NOT NULL COMMENT 'ai_tools.id',
  `status` tinyint unsigned NOT NULL DEFAULT '1' COMMENT '1启用 2禁用；运行时只加载启用绑定',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_agent_tools_agent_tool` (`agent_id`,`tool_id`) USING BTREE,
  KEY `idx_ai_agent_tools_agent_status` (`agent_id`,`status`,`id`) USING BTREE,
  KEY `idx_ai_agent_tools_tool_status` (`tool_id`,`status`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_agent_tools_agent` FOREIGN KEY (`agent_id`) REFERENCES `ai_agents` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_agent_tools_tool` FOREIGN KEY (`tool_id`) REFERENCES `ai_tools` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_agent_tools_status` CHECK ((`status` in (1,2)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI智能体工具绑定';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_agents` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `provider_id` bigint unsigned NOT NULL,
  `provider_model_id` bigint unsigned NOT NULL,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `model_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `model_display_name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `billing_multiplier_ppm` bigint unsigned NOT NULL DEFAULT '1000000',
  `scenes_json` json DEFAULT NULL,
  `system_prompt` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `avatar` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `context_profile_id` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_ai_agents_provider` (`provider_id`,`status`,`is_del`) USING BTREE,
  KEY `idx_ai_agents_model` (`provider_id`,`model_id`,`status`,`is_del`) USING BTREE,
  KEY `idx_ai_agents_context_profile` (`context_profile_id`),
  KEY `idx_ai_agents_provider_model_identity` (`provider_model_id`,`provider_id`,`model_id`),
  CONSTRAINT `fk_ai_agents_context_profile` FOREIGN KEY (`context_profile_id`) REFERENCES `ai_context_profiles` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_agents_provider_model` FOREIGN KEY (`provider_model_id`, `provider_id`, `model_id`) REFERENCES `ai_provider_models` (`id`, `provider_id`, `model_id`) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI agent mappings';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_assets` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL DEFAULT '0',
  `slug` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `type` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `category` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `title` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `cover_url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `description` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `content` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `tags_json` json DEFAULT NULL,
  `status` tinyint NOT NULL DEFAULT '1',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_assets_user_slug` (`user_id`,`slug`) USING BTREE,
  KEY `idx_ai_assets_type_status` (`type`,`status`,`is_del`,`updated_at`,`id`) USING BTREE,
  KEY `idx_ai_assets_status_updated` (`status`,`is_del`,`updated_at`,`id`) USING BTREE,
  KEY `idx_ai_assets_user_status_updated` (`user_id`,`status`,`is_del`,`updated_at`,`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC COMMENT='AI素材库';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_bindings` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `agent_id` bigint unsigned NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `status` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_bindings_agent_space` (`agent_id`,`space_id`),
  KEY `idx_ai_context_bindings_agent_status` (`agent_id`,`status`,`id`),
  KEY `idx_ai_context_bindings_space_status` (`space_id`,`status`,`id`),
  CONSTRAINT `fk_ai_context_bindings_agent` FOREIGN KEY (`agent_id`) REFERENCES `ai_agents` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_bindings_space` FOREIGN KEY (`space_id`) REFERENCES `ai_context_spaces` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_bindings_status` CHECK ((`status` in (_ascii'enabled',_ascii'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_chunks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `document_version_id` bigint unsigned NOT NULL,
  `ordinal` int unsigned NOT NULL,
  `heading_path` text NOT NULL,
  `content` longtext NOT NULL,
  `content_sha256` binary(32) NOT NULL,
  `chunk_facts_sha256` binary(32) NOT NULL,
  `embedding_input_token_upper_bound` bigint unsigned NOT NULL,
  `locator_json` json NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_chunks_version_ordinal` (`document_version_id`,`ordinal`),
  KEY `idx_ai_context_chunks_version_id` (`document_version_id`,`id`),
  CONSTRAINT `fk_ai_context_chunks_version` FOREIGN KEY (`document_version_id`) REFERENCES `ai_context_document_versions` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_chunks_content` CHECK (((length(`content`) > 0) and (`embedding_input_token_upper_bound` > 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_document_versions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `document_id` bigint unsigned NOT NULL,
  `profile_id` bigint unsigned NOT NULL,
  `source_storage_provider` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_object_key` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `source_etag` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `source_size_bytes` bigint unsigned NOT NULL,
  `source_mime_type` varchar(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_filename` varchar(512) NOT NULL,
  `source_facts_sha256` binary(32) NOT NULL,
  `source_sha256` binary(32) DEFAULT NULL,
  `parser_name` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `parser_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `chunker_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `failure_stage` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `error_code` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `error_message` varchar(1024) DEFAULT NULL,
  `chunk_count` int unsigned NOT NULL DEFAULT '0',
  `embedding_input_token_upper_bound` bigint unsigned NOT NULL DEFAULT '0',
  `embedding_request_count` int unsigned NOT NULL DEFAULT '0',
  `embedding_input_tokens` bigint unsigned DEFAULT NULL,
  `started_at` datetime(6) DEFAULT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  `attempt_count` int unsigned NOT NULL DEFAULT '0',
  `lease_token` bigint unsigned DEFAULT NULL,
  `lease_expires_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_document_versions_document_id` (`document_id`,`id`),
  KEY `idx_ai_context_document_versions_document_created` (`document_id`,`created_at`,`id`),
  KEY `idx_ai_context_document_versions_profile_state` (`profile_id`,`state`,`id`),
  KEY `idx_ai_context_document_versions_lease` (`state`,`lease_expires_at`,`id`),
  CONSTRAINT `fk_ai_context_document_versions_document` FOREIGN KEY (`document_id`) REFERENCES `ai_context_documents` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_document_versions_profile` FOREIGN KEY (`profile_id`) REFERENCES `ai_context_profiles` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_document_versions_lease_pair` CHECK ((((`lease_token` is null) and (`lease_expires_at` is null)) or ((`lease_token` is not null) and (`lease_expires_at` is not null)))),
  CONSTRAINT `chk_ai_context_document_versions_source` CHECK (((`source_size_bytes` > 0) and (char_length(`source_object_key`) > 0) and (char_length(`source_etag`) > 0) and (char_length(`source_mime_type`) > 0) and (char_length(`source_filename`) > 0))),
  CONSTRAINT `chk_ai_context_document_versions_state` CHECK ((`state` in (_ascii'queued',_ascii'processing',_ascii'ready',_ascii'failed'))),
  CONSTRAINT `chk_ai_context_document_versions_terminal_shape` CHECK ((((`state` = _ascii'queued') and (`source_sha256` is null) and (`failure_stage` is null) and (`error_code` is null) and (`error_message` is null) and (`started_at` is null) and (`finished_at` is null) and (`lease_token` is null) and (`lease_expires_at` is null)) or ((`state` = _ascii'processing') and (`failure_stage` is null) and (`error_code` is null) and (`error_message` is null) and (`started_at` is not null) and (`finished_at` is null) and (`attempt_count` > 0) and (`lease_token` is not null) and (`lease_expires_at` is not null)) or ((`state` = _ascii'ready') and (`source_sha256` is not null) and (`failure_stage` is null) and (`error_code` is null) and (`error_message` is null) and (`chunk_count` > 0) and (`embedding_input_token_upper_bound` > 0) and (`embedding_request_count` > 0) and (`started_at` is not null) and (`finished_at` is not null) and (`attempt_count` > 0) and (`lease_token` is null) and (`lease_expires_at` is null)) or ((`state` = _ascii'failed') and (`failure_stage` is not null) and (char_length(`failure_stage`) > 0) and (`error_code` is not null) and (char_length(`error_code`) > 0) and ((`error_message` is null) or (char_length(`error_message`) > 0)) and (`started_at` is not null) and (`finished_at` is not null) and (`attempt_count` > 0) and (`lease_token` is null) and (`lease_expires_at` is null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_documents` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `space_id` bigint unsigned DEFAULT NULL,
  `conversation_id` int unsigned DEFAULT NULL,
  `source_message_id` bigint unsigned DEFAULT NULL,
  `source_attachment_index` int unsigned DEFAULT NULL,
  `title` varchar(512) NOT NULL,
  `active_version_id` bigint unsigned DEFAULT NULL,
  `status` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `deleted_at` datetime(6) DEFAULT NULL,
  `created_by` int unsigned NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_documents_conversation_attachment` (`conversation_id`,`source_message_id`,`source_attachment_index`),
  KEY `idx_ai_context_documents_space_status` (`space_id`,`status`,`deleted_at`,`id`),
  KEY `idx_ai_context_documents_conversation_status` (`conversation_id`,`status`,`deleted_at`,`id`),
  KEY `idx_ai_context_documents_source_message` (`source_message_id`),
  KEY `idx_ai_context_documents_active_owner` (`id`,`active_version_id`),
  KEY `idx_ai_context_documents_created_by` (`created_by`),
  KEY `idx_ai_context_documents_source_message_owner` (`source_message_id`,`conversation_id`),
  CONSTRAINT `fk_ai_context_documents_active_version` FOREIGN KEY (`id`, `active_version_id`) REFERENCES `ai_context_document_versions` (`document_id`, `id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_source_message_owner` FOREIGN KEY (`source_message_id`, `conversation_id`) REFERENCES `ai_messages` (`id`, `conversation_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_space` FOREIGN KEY (`space_id`) REFERENCES `ai_context_spaces` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_documents_owner_source` CHECK ((((`space_id` is not null) and (`conversation_id` is null) and (`source_message_id` is null) and (`source_attachment_index` is null)) or ((`space_id` is null) and (`conversation_id` is not null) and (`source_message_id` is not null) and (`source_attachment_index` is not null)))),
  CONSTRAINT `chk_ai_context_documents_status` CHECK ((`status` in (_ascii'enabled',_ascii'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_plan_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `plan_id` bigint unsigned NOT NULL,
  `ordinal` int unsigned NOT NULL,
  `block_kind` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_type` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_ref` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `source_sha256` binary(32) NOT NULL,
  `atomic_group_key` varchar(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `required` tinyint unsigned NOT NULL,
  `priority` int NOT NULL,
  `decision` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `exclusion_reason` varchar(32) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `token_upper_bound` bigint unsigned NOT NULL,
  `fusion_score` decimal(20,6) DEFAULT NULL,
  `rerank_score` decimal(20,6) DEFAULT NULL,
  `citation_key` varchar(32) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `content_snapshot` longtext,
  `metadata_json` json NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_plan_items_plan_ordinal` (`plan_id`,`ordinal`),
  UNIQUE KEY `uk_ai_context_plan_items_plan_citation` (`plan_id`,`citation_key`),
  KEY `idx_ai_context_plan_items_plan_decision` (`plan_id`,`decision`,`ordinal`),
  CONSTRAINT `fk_ai_context_plan_items_plan` FOREIGN KEY (`plan_id`) REFERENCES `ai_context_plans` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_plan_items_block_kind` CHECK ((`block_kind` in (_ascii'system_instruction',_ascii'current_user_message',_ascii'current_attachment',_ascii'recent_turn',_ascii'recalled_turn',_ascii'history_attachment',_ascii'conversation_memory',_ascii'document_evidence',_ascii'tool_definition',_ascii'tool_call',_ascii'tool_result'))),
  CONSTRAINT `chk_ai_context_plan_items_citation` CHECK (((`citation_key` is null) or ((`decision` = _ascii'selected') and (`block_kind` = _ascii'document_evidence') and regexp_like(`citation_key`,_ascii'^C[1-9][0-9]*$')))),
  CONSTRAINT `chk_ai_context_plan_items_content_snapshot` CHECK ((((`decision` = _ascii'excluded') and (`content_snapshot` is null)) or ((`decision` = _ascii'selected') and (`block_kind` in (_ascii'current_attachment',_ascii'history_attachment')) and (`content_snapshot` is null)) or ((`decision` = _ascii'selected') and (`block_kind` not in (_ascii'current_attachment',_ascii'history_attachment')) and (`content_snapshot` is not null) and (length(`content_snapshot`) > 0)))),
  CONSTRAINT `chk_ai_context_plan_items_decision` CHECK ((((`decision` = _ascii'selected') and (`exclusion_reason` is null)) or ((`decision` = _ascii'excluded') and (`exclusion_reason` is not null) and (`exclusion_reason` in (_ascii'budget_exceeded',_ascii'duplicate_content',_ascii'below_relevance_threshold',_ascii'superseded_memory',_ascii'inactive_source',_ascii'permission_changed',_ascii'unsupported_attachment'))))),
  CONSTRAINT `chk_ai_context_plan_items_required` CHECK ((`required` in (0,1)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_plans` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `run_id` bigint unsigned NOT NULL,
  `context_profile_id_snapshot` bigint unsigned DEFAULT NULL,
  `context_profile_sha256` binary(32) DEFAULT NULL,
  `context_index_generation_snapshot` bigint unsigned DEFAULT NULL,
  `policy_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `input_fingerprint_sha256` binary(32) NOT NULL,
  `plan_sha256` binary(32) DEFAULT NULL,
  `model_capability_sha256` binary(32) NOT NULL,
  `api_protocol_snapshot` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `token_counter_id_snapshot` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `context_window_tokens` bigint unsigned NOT NULL,
  `effective_output_tokens` bigint unsigned NOT NULL,
  `provider_protocol_upper_bound` bigint unsigned NOT NULL,
  `tool_continuation_input_reserve` bigint unsigned NOT NULL,
  `policy_safety_margin` bigint unsigned NOT NULL,
  `known_input_budget` bigint unsigned NOT NULL,
  `known_input_upper_bound` bigint unsigned NOT NULL,
  `budget_proof` varchar(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `retrieval_outcome` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `error_stage` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `error_code` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `error_message` varchar(1024) DEFAULT NULL,
  `metrics_json` json NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_plans_run` (`run_id`),
  UNIQUE KEY `uk_ai_context_plans_id_run` (`id`,`run_id`),
  KEY `idx_ai_context_plans_profile_generation` (`context_profile_id_snapshot`,`context_index_generation_snapshot`,`id`),
  CONSTRAINT `fk_ai_context_plans_profile` FOREIGN KEY (`context_profile_id_snapshot`) REFERENCES `ai_context_profiles` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_plans_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_plans_api_protocol` CHECK ((`api_protocol_snapshot` in (_ascii'chat_completions',_ascii'responses'))),
  CONSTRAINT `chk_ai_context_plans_budget` CHECK (((`context_window_tokens` > 0) and (`effective_output_tokens` > 0) and ((((`known_input_budget` + `effective_output_tokens`) + `provider_protocol_upper_bound`) + `policy_safety_margin`) = `context_window_tokens`) and (`tool_continuation_input_reserve` <= `provider_protocol_upper_bound`) and (`known_input_upper_bound` <= `known_input_budget`))),
  CONSTRAINT `chk_ai_context_plans_budget_proof` CHECK ((`budget_proof` in (_ascii'exact',_ascii'conservative',_ascii'opaque_attachment'))),
  CONSTRAINT `chk_ai_context_plans_profile_snapshot` CHECK ((((`context_profile_id_snapshot` is null) and (`context_profile_sha256` is null) and (`context_index_generation_snapshot` is null)) or ((`context_profile_id_snapshot` is not null) and (`context_profile_sha256` is not null) and ((`context_index_generation_snapshot` is null) or (`context_index_generation_snapshot` > 0))))),
  CONSTRAINT `chk_ai_context_plans_retrieval_outcome` CHECK ((`retrieval_outcome` in (_utf8mb4'skipped',_utf8mb4'no_hit',_utf8mb4'hit',_utf8mb4'degraded',_utf8mb4'failed'))),
  CONSTRAINT `chk_ai_context_plans_state` CHECK ((`state` in (_ascii'ready',_ascii'failed'))),
  CONSTRAINT `chk_ai_context_plans_terminal_shape` CHECK ((((`state` = _utf8mb4'ready') and (`plan_sha256` is not null) and (((`retrieval_outcome` in (_utf8mb4'skipped',_utf8mb4'no_hit',_utf8mb4'hit')) and (`error_stage` is null) and (`error_code` is null) and (`error_message` is null)) or ((`retrieval_outcome` = _utf8mb4'degraded') and (`error_stage` is not null) and (char_length(`error_stage`) > 0) and (`error_code` is not null) and (char_length(`error_code`) > 0) and ((`error_message` is null) or (char_length(`error_message`) > 0))))) or ((`state` = _utf8mb4'failed') and (`plan_sha256` is null) and (`retrieval_outcome` = _utf8mb4'failed') and (`error_stage` is not null) and (char_length(`error_stage`) > 0) and (`error_code` is not null) and (char_length(`error_code`) > 0) and ((`error_message` is null) or (char_length(`error_message`) > 0)))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_profiles` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(191) NOT NULL,
  `embedding_provider_model_id` bigint unsigned NOT NULL,
  `embedding_dimensions` int unsigned NOT NULL,
  `embedding_max_input_tokens` bigint unsigned NOT NULL,
  `embedding_token_counter_id` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `dense_distance` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `dense_min_score` decimal(20,6) NOT NULL,
  `sparse_encoder` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `sparse_encoder_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `reranker_provider_model_id` bigint unsigned DEFAULT NULL,
  `reranker_min_score` decimal(20,6) DEFAULT NULL,
  `memory_provider_model_id` bigint unsigned DEFAULT NULL,
  `status` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `active_index_generation` bigint unsigned DEFAULT NULL,
  `target_index_generation` bigint unsigned DEFAULT NULL,
  `index_state` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `index_error_code` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `index_verified_at` datetime(6) DEFAULT NULL,
  `created_by` int unsigned NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  KEY `idx_ai_context_profiles_status_state` (`status`,`index_state`,`id`),
  KEY `idx_ai_context_profiles_embedding_model` (`embedding_provider_model_id`),
  KEY `idx_ai_context_profiles_reranker_model` (`reranker_provider_model_id`),
  KEY `idx_ai_context_profiles_memory_model` (`memory_provider_model_id`),
  KEY `idx_ai_context_profiles_created_by` (`created_by`),
  CONSTRAINT `fk_ai_context_profiles_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_profiles_embedding_model` FOREIGN KEY (`embedding_provider_model_id`) REFERENCES `ai_provider_models` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_profiles_memory_model` FOREIGN KEY (`memory_provider_model_id`) REFERENCES `ai_provider_models` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_profiles_reranker_model` FOREIGN KEY (`reranker_provider_model_id`) REFERENCES `ai_provider_models` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_profiles_dense_distance` CHECK ((`dense_distance` in (_ascii'cosine',_ascii'dot',_ascii'euclid'))),
  CONSTRAINT `chk_ai_context_profiles_embedding_shape` CHECK (((`embedding_dimensions` > 0) and (`embedding_max_input_tokens` > 0))),
  CONSTRAINT `chk_ai_context_profiles_generation_order` CHECK ((((`active_index_generation` is null) or (`active_index_generation` > 0)) and ((`target_index_generation` is null) or (`target_index_generation` > 0)) and ((`active_index_generation` is null) or (`target_index_generation` is null) or (`target_index_generation` > `active_index_generation`)))),
  CONSTRAINT `chk_ai_context_profiles_generation_shape` CHECK ((((`index_state` = _ascii'provisioning') and (`active_index_generation` is null) and (`target_index_generation` is not null)) or ((`index_state` = _ascii'ready') and (`active_index_generation` is not null) and (`target_index_generation` is null)) or ((`index_state` = _ascii'rebuilding') and (`target_index_generation` is not null)) or (`index_state` = _ascii'failed'))),
  CONSTRAINT `chk_ai_context_profiles_index_error` CHECK (((`index_state` <> _ascii'failed') or ((`index_error_code` is not null) and (char_length(`index_error_code`) > 0)))),
  CONSTRAINT `chk_ai_context_profiles_index_state` CHECK ((`index_state` in (_ascii'provisioning',_ascii'ready',_ascii'rebuilding',_ascii'failed'))),
  CONSTRAINT `chk_ai_context_profiles_reranker_pair` CHECK ((((`reranker_provider_model_id` is null) and (`reranker_min_score` is null)) or ((`reranker_provider_model_id` is not null) and (`reranker_min_score` is not null)))),
  CONSTRAINT `chk_ai_context_profiles_sparse_encoder` CHECK ((`sparse_encoder` = _ascii'unicode_lexical_v1')),
  CONSTRAINT `chk_ai_context_profiles_status` CHECK ((`status` in (_ascii'enabled',_ascii'retired')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_context_spaces` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `platform` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `profile_id` bigint unsigned NOT NULL,
  `name` varchar(191) NOT NULL,
  `description` varchar(1024) NOT NULL DEFAULT '',
  `status` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `deleted_at` datetime(6) DEFAULT NULL,
  `created_by` int unsigned NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  KEY `idx_ai_context_spaces_platform_status` (`platform`,`status`,`deleted_at`,`id`),
  KEY `idx_ai_context_spaces_profile_status` (`profile_id`,`status`,`deleted_at`,`id`),
  KEY `idx_ai_context_spaces_created_by` (`created_by`),
  CONSTRAINT `fk_ai_context_spaces_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_context_spaces_profile` FOREIGN KEY (`profile_id`) REFERENCES `ai_context_profiles` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_context_spaces_platform` CHECK ((regexp_like(`platform`,_ascii'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_ascii'app',_ascii'canvas',_ascii'all')))),
  CONSTRAINT `chk_ai_context_spaces_status` CHECK ((`status` in (_ascii'enabled',_ascii'disabled')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_conversation_memories` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `conversation_id` int unsigned NOT NULL,
  `context_profile_id_snapshot` bigint unsigned NOT NULL,
  `context_profile_sha256` binary(32) NOT NULL,
  `previous_memory_id` bigint unsigned DEFAULT NULL,
  `from_message_id` bigint unsigned NOT NULL,
  `through_message_id` bigint unsigned NOT NULL,
  `source_sha256` binary(32) NOT NULL,
  `summary_sha256` binary(32) DEFAULT NULL,
  `policy_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `summary` mediumtext,
  `prompt_tokens` bigint unsigned DEFAULT NULL,
  `completion_tokens` bigint unsigned DEFAULT NULL,
  `provider_request_id` varchar(191) DEFAULT NULL,
  `state` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `error_code` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_conversation_memories_identity` (`conversation_id`,`context_profile_id_snapshot`,`through_message_id`,`source_sha256`),
  UNIQUE KEY `uk_ai_conversation_memories_owner` (`id`,`conversation_id`,`context_profile_id_snapshot`),
  KEY `idx_ai_conversation_memories_latest_ready` (`conversation_id`,`context_profile_id_snapshot`,`state`,`through_message_id`,`id`),
  KEY `idx_ai_conversation_memories_previous` (`previous_memory_id`),
  KEY `idx_ai_conversation_memories_from_message` (`from_message_id`),
  KEY `idx_ai_conversation_memories_through_message` (`through_message_id`),
  KEY `fk_ai_conversation_memories_profile` (`context_profile_id_snapshot`),
  KEY `idx_ai_conversation_memories_previous_owner` (`previous_memory_id`,`conversation_id`,`context_profile_id_snapshot`),
  KEY `idx_ai_conversation_memories_from_message_owner` (`from_message_id`,`conversation_id`),
  KEY `idx_ai_conversation_memories_through_message_owner` (`through_message_id`,`conversation_id`),
  CONSTRAINT `fk_ai_conversation_memories_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_from_message_owner` FOREIGN KEY (`from_message_id`, `conversation_id`) REFERENCES `ai_messages` (`id`, `conversation_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_previous_owner` FOREIGN KEY (`previous_memory_id`, `conversation_id`, `context_profile_id_snapshot`) REFERENCES `ai_conversation_memories` (`id`, `conversation_id`, `context_profile_id_snapshot`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_profile` FOREIGN KEY (`context_profile_id_snapshot`) REFERENCES `ai_context_profiles` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_through_message_owner` FOREIGN KEY (`through_message_id`, `conversation_id`) REFERENCES `ai_messages` (`id`, `conversation_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_conversation_memories_interval` CHECK ((`from_message_id` <= `through_message_id`)),
  CONSTRAINT `chk_ai_conversation_memories_state` CHECK ((`state` in (_ascii'ready',_ascii'failed',_ascii'invalidated'))),
  CONSTRAINT `chk_ai_conversation_memories_terminal_shape` CHECK ((((`state` = _ascii'ready') and (`summary` is not null) and (length(`summary`) > 0) and (`summary_sha256` is not null) and (`error_code` is null)) or ((`state` = _ascii'failed') and (`summary` is null) and (`summary_sha256` is null) and (`error_code` is not null) and (char_length(`error_code`) > 0)) or ((`state` = _ascii'invalidated') and (`summary` is not null) and (length(`summary`) > 0) and (`summary_sha256` is not null) and (`error_code` is null)))),
  CONSTRAINT `chk_ai_conversation_memories_usage_pair` CHECK ((((`prompt_tokens` is null) and (`completion_tokens` is null)) or ((`prompt_tokens` is not null) and (`completion_tokens` is not null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_conversations` (
  `id` int unsigned NOT NULL AUTO_INCREMENT COMMENT '会话ID',
  `user_id` int unsigned NOT NULL COMMENT '当前用户ID',
  `agent_id` bigint unsigned NOT NULL,
  `title` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '会话标题',
  `last_message_at` datetime DEFAULT NULL COMMENT '上次对话时间',
  `last_read_message_id` bigint unsigned NOT NULL DEFAULT '0',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '1删除 2正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_conversations_id_user` (`id`,`user_id`),
  KEY `idx_ai_conversations_user_agent_del_last_message` (`user_id`,`agent_id`,`is_del`,`last_message_at`,`id`) USING BTREE,
  KEY `idx_ai_conversations_agent` (`agent_id`),
  CONSTRAINT `fk_ai_conversations_agent` FOREIGN KEY (`agent_id`) REFERENCES `ai_agents` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_conversations_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI会话';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_image_files` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_id` bigint unsigned NOT NULL,
  `role` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'input/mask/output',
  `sort_order` int NOT NULL DEFAULT '0',
  `storage_provider` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `storage_key` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `storage_url` varchar(1000) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `mime_type` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `width` int NOT NULL DEFAULT '0',
  `height` int NOT NULL DEFAULT '0',
  `size_bytes` bigint NOT NULL DEFAULT '0',
  `related_file_id` bigint unsigned DEFAULT NULL,
  `revised_prompt` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_ai_image_files_task_role_sort` (`task_id`,`role`,`sort_order`) USING BTREE,
  KEY `idx_ai_image_files_related` (`related_file_id`) USING BTREE,
  CONSTRAINT `fk_ai_image_files_related` FOREIGN KEY (`related_file_id`) REFERENCES `ai_image_files` (`id`) ON DELETE SET NULL ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_image_files_task` FOREIGN KEY (`task_id`) REFERENCES `ai_image_tasks` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_image_tasks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `request_fingerprint` binary(32) NOT NULL,
  `run_id` bigint unsigned NOT NULL,
  `request_identity_status` varchar(24) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'replayable',
  `request_identity_marker` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `agent_id` bigint unsigned NOT NULL,
  `agent_name_snapshot` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `provider_id_snapshot` bigint unsigned NOT NULL DEFAULT '0',
  `provider_name_snapshot` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `model_id_snapshot` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `model_display_name_snapshot` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `prompt` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `size` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '1024x1024',
  `quality` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'auto',
  `output_format` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'png',
  `output_compression` int DEFAULT NULL,
  `moderation` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'auto',
  `n` int NOT NULL DEFAULT '1',
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `lease_owner` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `lease_token` bigint unsigned NOT NULL DEFAULT '0',
  `lease_expires_at` datetime(6) DEFAULT NULL,
  `error_message` varchar(1000) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `last_error_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `actual_params_json` json DEFAULT NULL,
  `raw_response_json` json DEFAULT NULL,
  `is_favorite` tinyint NOT NULL DEFAULT '2',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `finished_at` datetime DEFAULT NULL,
  `elapsed_ms` int NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_image_tasks_user_request` (`user_id`,`request_id`) USING BTREE,
  UNIQUE KEY `uk_ai_image_tasks_run` (`run_id`),
  KEY `idx_ai_image_tasks_platform_user_created` (`platform`,`user_id`,`created_at`) USING BTREE,
  KEY `idx_ai_image_tasks_platform_status_created` (`platform`,`status`,`created_at`) USING BTREE,
  KEY `idx_ai_image_tasks_agent_created` (`agent_id`,`created_at`) USING BTREE,
  KEY `idx_ai_image_tasks_lease` (`status`,`lease_expires_at`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_image_tasks_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_image_tasks_lease` CHECK ((((`lease_owner` is null) and (`lease_expires_at` is null)) or ((`lease_owner` is not null) and (`lease_token` > 0) and (`lease_expires_at` is not null)))),
  CONSTRAINT `chk_ai_image_tasks_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))),
  CONSTRAINT `chk_ai_image_tasks_request_identity` CHECK ((((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%'))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_messages` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '消息ID',
  `conversation_id` int unsigned NOT NULL COMMENT 'ai_conversations.id',
  `role` tinyint unsigned NOT NULL COMMENT '1用户 2助手',
  `content_type` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'text' COMMENT '内容类型，MVP只写text',
  `content` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '消息内容',
  `meta_json` json DEFAULT NULL COMMENT '消息扩展元数据：attachments/runtime_params/blocks/feedback',
  `reply_command_id` bigint unsigned DEFAULT NULL,
  `delivery_state` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '1删除 2正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_messages_id_conversation` (`id`,`conversation_id`),
  UNIQUE KEY `uk_ai_messages_reply_command` (`reply_command_id`) USING BTREE,
  KEY `idx_ai_messages_conversation_del_id` (`conversation_id`,`is_del`,`id`) USING BTREE,
  KEY `idx_ai_messages_conversation_del_role_id` (`conversation_id`,`is_del`,`role`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_messages_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_messages_delivery_state` CHECK ((((`role` = 2) and (`delivery_state` in (_utf8mb4'completed',_utf8mb4'stopped'))) or ((`role` <> 2) and (`delivery_state` is null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI消息';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_official_model_price_override_rates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `override_id` bigint unsigned NOT NULL,
  `category` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `unit` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `tier_key` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  `price_units` bigint NOT NULL,
  `unit_scale` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_official_model_price_override_rates_key` (`override_id`,`category`,`unit`,`tier_key`) USING BTREE,
  CONSTRAINT `fk_ai_official_model_price_override_rates_override` FOREIGN KEY (`override_id`) REFERENCES `ai_official_model_price_overrides` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_official_model_price_override_rates_category` CHECK ((`category` in (_utf8mb4'input',_utf8mb4'output',_utf8mb4'cache_read',_utf8mb4'cache_write',_utf8mb4'media'))),
  CONSTRAINT `chk_ai_official_model_price_override_rates_price` CHECK ((`price_units` >= 0)),
  CONSTRAINT `chk_ai_official_model_price_override_rates_scale` CHECK ((`unit_scale` > 0)),
  CONSTRAINT `chk_ai_official_model_price_override_rates_unit` CHECK ((char_length(trim(`unit`)) > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_official_model_price_overrides` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `catalog_vendor` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `model_id` varchar(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `version` bigint unsigned NOT NULL,
  `source_url` varchar(2048) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `verified_at` date NOT NULL,
  `updated_by` int unsigned NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_official_model_price_overrides_identity` (`catalog_vendor`,`model_id`) USING BTREE,
  CONSTRAINT `chk_ai_official_model_price_overrides_version` CHECK ((`version` > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_prompts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `slug` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `category` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `title` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `cover_url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `prompt` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `preview` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `tags_json` json DEFAULT NULL,
  `source_url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `status` tinyint NOT NULL DEFAULT '1',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_prompts_slug` (`slug`) USING BTREE,
  KEY `idx_ai_prompts_category_status` (`category`,`status`,`is_del`,`updated_at`,`id`) USING BTREE,
  KEY `idx_ai_prompts_status_updated` (`status`,`is_del`,`updated_at`,`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC COMMENT='AI提示词库';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_provider_attempts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `run_id` bigint unsigned NOT NULL,
  `command_id` bigint unsigned DEFAULT NULL,
  `attempt_no` int unsigned NOT NULL,
  `idempotency_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `state` varchar(24) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `prepared_request_json` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `prepared_request_sha256` binary(32) NOT NULL,
  `quote_json` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `usage_json` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `usage_status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'unavailable',
  `dispatch_state` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'not_dispatched',
  `result_candidate_json` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `provider_request_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `response_sha256` char(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `error_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `prepare_started_at` datetime(6) DEFAULT NULL,
  `dispatched_at` datetime(6) DEFAULT NULL,
  `first_delta_at` datetime(6) DEFAULT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  `context_plan_id` bigint unsigned DEFAULT NULL,
  `context_plan_sha256` binary(32) DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_attempt_key` (`idempotency_key`) USING BTREE,
  UNIQUE KEY `uk_ai_attempt_run_no` (`run_id`,`attempt_no`) USING BTREE,
  KEY `idx_ai_attempt_state` (`state`,`id`) USING BTREE,
  KEY `idx_ai_attempt_command` (`command_id`,`attempt_no`) USING BTREE,
  KEY `idx_ai_provider_attempts_error_run` (`error_code`,`run_id`,`id`) USING BTREE,
  KEY `idx_ai_provider_attempts_context_plan` (`context_plan_id`),
  KEY `idx_ai_provider_attempts_command_run` (`command_id`,`run_id`),
  KEY `idx_ai_provider_attempts_context_plan_run` (`context_plan_id`,`run_id`),
  CONSTRAINT `fk_ai_provider_attempts_command_run` FOREIGN KEY (`command_id`, `run_id`) REFERENCES `ai_reply_commands` (`id`, `run_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_provider_attempts_context_plan_run` FOREIGN KEY (`context_plan_id`, `run_id`) REFERENCES `ai_context_plans` (`id`, `run_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_provider_attempts_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_provider_attempts_context_plan_pair` CHECK ((((`context_plan_id` is null) and (`context_plan_sha256` is null)) or ((`context_plan_id` is not null) and (`context_plan_sha256` is not null)))),
  CONSTRAINT `chk_ai_provider_attempts_dispatch_state` CHECK ((`dispatch_state` in (_utf8mb4'not_dispatched',_utf8mb4'dispatched',_utf8mb4'unknown'))),
  CONSTRAINT `chk_ai_provider_attempts_state` CHECK ((`state` in (_utf8mb4'prepared',_utf8mb4'dispatched',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'outcome_unknown'))),
  CONSTRAINT `chk_ai_provider_attempts_usage_status` CHECK ((`usage_status` in (_utf8mb4'complete',_utf8mb4'unavailable')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_provider_models` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `provider_id` bigint unsigned NOT NULL,
  `model_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `model_kind` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `display_name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `official_model_id` varchar(191) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `official_catalog_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `mapping_status` varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'unmapped',
  `mapped_at` datetime(6) DEFAULT NULL,
  `embedding_dimensions` int unsigned DEFAULT NULL,
  `embedding_max_input_tokens` bigint unsigned DEFAULT NULL,
  `embedding_token_counter_id` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_provider_models_provider_model_kind` (`provider_id`,`model_id`,`model_kind`),
  UNIQUE KEY `uk_ai_provider_models_id_provider_model` (`id`,`provider_id`,`model_id`),
  KEY `idx_ai_provider_models_provider_status` (`provider_id`,`status`) USING BTREE,
  KEY `idx_ai_provider_models_official_mapping` (`mapping_status`,`official_model_id`,`status`) USING BTREE,
  CONSTRAINT `fk_ai_provider_models_provider` FOREIGN KEY (`provider_id`) REFERENCES `ai_providers` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_provider_models_embedding_spec` CHECK ((((`model_kind` = _ascii'embedding') and (((`status` = 2) and (`embedding_dimensions` is null) and (`embedding_max_input_tokens` is null) and (`embedding_token_counter_id` is null)) or ((`embedding_dimensions` is not null) and (`embedding_dimensions` > 0) and (`embedding_max_input_tokens` is not null) and (`embedding_max_input_tokens` > 0) and (`embedding_token_counter_id` is not null) and (char_length(`embedding_token_counter_id`) > 0)))) or ((`model_kind` <> _ascii'embedding') and (`embedding_dimensions` is null) and (`embedding_max_input_tokens` is null) and (`embedding_token_counter_id` is null)))),
  CONSTRAINT `chk_ai_provider_models_mapping` CHECK ((((`mapping_status` = _ascii'mapped') and (`official_model_id` is not null) and (`official_catalog_version` is not null) and (`mapped_at` is not null)) or ((`mapping_status` = _ascii'unmapped') and (`official_model_id` is null) and (`official_catalog_version` is null) and (`mapped_at` is null)))),
  CONSTRAINT `chk_ai_provider_models_mapping_status` CHECK ((`mapping_status` in (_ascii'mapped',_ascii'unmapped'))),
  CONSTRAINT `chk_ai_provider_models_model_kind` CHECK ((`model_kind` in (_ascii'chat',_ascii'embedding',_ascii'rerank',_ascii'image')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI provider enabled model catalog';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_providers` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `engine_type` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `base_url` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `api_protocol` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'chat_completions',
  `api_key_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `api_key_hint` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `health_status` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'unknown',
  `last_checked_at` datetime DEFAULT NULL,
  `last_check_error` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `last_model_sync_at` datetime DEFAULT NULL,
  `last_model_sync_status` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'unknown',
  `last_model_sync_error` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_providers_type_name` (`engine_type`,`name`,`is_del`) USING BTREE,
  KEY `idx_ai_providers_status` (`status`,`is_del`) USING BTREE,
  CONSTRAINT `chk_ai_providers_api_protocol` CHECK ((`api_protocol` in (_utf8mb4'chat_completions',_utf8mb4'responses')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI engine connection configs';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_reply_commands` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `request_fingerprint` binary(32) NOT NULL,
  `request_identity_status` varchar(24) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'replayable',
  `request_identity_marker` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `idempotency_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `user_id` int unsigned NOT NULL,
  `conversation_id` int unsigned NOT NULL,
  `run_id` bigint unsigned NOT NULL,
  `user_message_id` bigint unsigned NOT NULL,
  `assistant_message_id` bigint unsigned DEFAULT NULL,
  `request_received_at` datetime(6) DEFAULT NULL,
  `accepted_at` datetime(6) DEFAULT NULL,
  `claimed_at` datetime(6) DEFAULT NULL,
  `claim_source` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `state` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'pending',
  `attempt_count` int unsigned NOT NULL DEFAULT '0',
  `max_attempts` int unsigned NOT NULL DEFAULT '3',
  `lease_owner` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `lease_token` bigint unsigned NOT NULL DEFAULT '0',
  `lease_expires_at` datetime(6) DEFAULT NULL,
  `next_attempt_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `cancel_requested_at` datetime(6) DEFAULT NULL,
  `delivery_seq` int unsigned NOT NULL DEFAULT '0',
  `stop_delivery_seq` int unsigned DEFAULT NULL,
  `outcome_unknown_at` datetime(6) DEFAULT NULL,
  `last_error_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `last_error_message` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `started_at` datetime(6) DEFAULT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_reply_message` (`user_message_id`) USING BTREE,
  UNIQUE KEY `uk_ai_reply_idempotency` (`idempotency_key`) USING BTREE,
  UNIQUE KEY `uk_ai_reply_user_request` (`user_id`,`request_id`) USING BTREE,
  UNIQUE KEY `uk_ai_reply_commands_run` (`run_id`),
  UNIQUE KEY `uk_ai_reply_commands_id_run` (`id`,`run_id`),
  KEY `idx_ai_reply_claim` (`state`,`next_attempt_at`,`lease_expires_at`,`id`) USING BTREE,
  KEY `idx_ai_reply_commands_run_owner` (`run_id`,`user_id`,`conversation_id`,`user_message_id`,`request_id`),
  KEY `idx_ai_reply_commands_conversation_owner` (`conversation_id`,`user_id`),
  KEY `idx_ai_reply_commands_user_message_owner` (`user_message_id`,`conversation_id`),
  KEY `idx_ai_reply_commands_assistant_message_owner` (`assistant_message_id`,`conversation_id`),
  CONSTRAINT `fk_ai_reply_commands_assistant_message_owner` FOREIGN KEY (`assistant_message_id`, `conversation_id`) REFERENCES `ai_messages` (`id`, `conversation_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_reply_commands_conversation_owner` FOREIGN KEY (`conversation_id`, `user_id`) REFERENCES `ai_conversations` (`id`, `user_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_reply_commands_run_owner` FOREIGN KEY (`run_id`, `user_id`, `conversation_id`, `user_message_id`, `request_id`) REFERENCES `ai_runs` (`id`, `user_id`, `conversation_id`, `user_message_id`, `request_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_reply_commands_user_message_owner` FOREIGN KEY (`user_message_id`, `conversation_id`) REFERENCES `ai_messages` (`id`, `conversation_id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_reply_claim_source` CHECK ((`claim_source` in (_utf8mb4'',_utf8mb4'wake',_utf8mb4'poll',_utf8mb4'recovery'))),
  CONSTRAINT `chk_ai_reply_delivery_seq` CHECK ((((`cancel_requested_at` is null) and (`stop_delivery_seq` is null)) or ((`cancel_requested_at` is not null) and (`stop_delivery_seq` is not null) and (`stop_delivery_seq` <= `delivery_seq`)))),
  CONSTRAINT `chk_ai_reply_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))),
  CONSTRAINT `chk_ai_reply_request_identity` CHECK ((((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))),
  CONSTRAINT `chk_ai_reply_state` CHECK ((`state` in (_utf8mb4'pending',_utf8mb4'claimed',_utf8mb4'running',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'outcome_unknown',_utf8mb4'timed_out')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_reply_delivery_chunks` (
  `command_id` bigint unsigned NOT NULL,
  `delivery_seq` int unsigned NOT NULL,
  `delta` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`command_id`,`delivery_seq`) USING BTREE,
  CONSTRAINT `fk_ai_reply_delivery_chunks_command` FOREIGN KEY (`command_id`) REFERENCES `ai_reply_commands` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_reply_delivery_chunk_seq` CHECK ((`delivery_seq` > 0)),
  CONSTRAINT `chk_ai_reply_delivery_chunk_size` CHECK (((length(`delta`) > 0) and (length(`delta`) <= 16384)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_run_dashboard_daily_facts` (
  `fact_date` date NOT NULL,
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `model_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `agent_id` bigint unsigned NOT NULL,
  `provider_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `run_anomaly_code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `billing_anomaly_code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `final_error_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `latest_run_id` bigint unsigned NOT NULL,
  `latest_model_display_name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `run_count` bigint unsigned NOT NULL DEFAULT '0',
  `prompt_tokens` bigint unsigned NOT NULL DEFAULT '0',
  `completion_tokens` bigint unsigned NOT NULL DEFAULT '0',
  `total_tokens` bigint unsigned NOT NULL DEFAULT '0',
  `settled_runs` bigint unsigned NOT NULL DEFAULT '0',
  `actual_units` bigint NOT NULL DEFAULT '0',
  `released_runs` bigint unsigned NOT NULL DEFAULT '0',
  `released_units` bigint NOT NULL DEFAULT '0',
  `unbilled_runs` bigint unsigned NOT NULL DEFAULT '0',
  PRIMARY KEY (`fact_date`,`platform`,`model_id`,`agent_id`,`provider_id`,`user_id`,`status`,`run_anomaly_code`,`billing_anomaly_code`,`final_error_code`) USING BTREE,
  KEY `idx_ai_run_dashboard_daily_model_date` (`model_id`,`fact_date`) USING BTREE,
  KEY `idx_ai_run_dashboard_daily_platform_date` (`platform`,`fact_date`) USING BTREE,
  KEY `idx_ai_run_dashboard_daily_provider_date` (`provider_id`,`fact_date`) USING BTREE,
  KEY `idx_ai_run_dashboard_daily_agent_date` (`agent_id`,`fact_date`) USING BTREE,
  KEY `idx_ai_run_dashboard_daily_user_date` (`user_id`,`fact_date`) USING BTREE,
  KEY `idx_ai_run_dashboard_daily_error_date` (`final_error_code`,`fact_date`) USING BTREE,
  CONSTRAINT `chk_ai_run_dashboard_daily_nonnegative` CHECK (((`actual_units` >= 0) and (`released_units` >= 0))),
  CONSTRAINT `chk_ai_run_dashboard_daily_status` CHECK ((`status` in (_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'outcome_unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='Daily terminal Run aggregate for bounded AI dashboard analytics';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_run_dashboard_facts` (
  `run_id` bigint unsigned NOT NULL,
  `fact_date` date NOT NULL,
  `run_created_at` datetime NOT NULL,
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `model_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `model_display_name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `agent_id` bigint unsigned NOT NULL,
  `provider_id` bigint unsigned NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `prompt_tokens` bigint unsigned NOT NULL DEFAULT '0',
  `completion_tokens` bigint unsigned NOT NULL DEFAULT '0',
  `total_tokens` bigint unsigned NOT NULL DEFAULT '0',
  `duration_ms` bigint unsigned DEFAULT NULL,
  `settled_runs` tinyint unsigned NOT NULL DEFAULT '0',
  `actual_units` bigint NOT NULL DEFAULT '0',
  `released_runs` tinyint unsigned NOT NULL DEFAULT '0',
  `released_units` bigint NOT NULL DEFAULT '0',
  `unbilled_runs` tinyint unsigned NOT NULL DEFAULT '0',
  `run_anomaly_code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `billing_anomaly_code` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `final_error_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `ttft_ms` bigint unsigned DEFAULT NULL,
  PRIMARY KEY (`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_created` (`fact_date`,`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_status_created` (`status`,`fact_date`,`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_model_created` (`model_id`,`fact_date`,`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_platform_created` (`platform`,`fact_date`,`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_agent_created` (`agent_id`,`fact_date`,`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_provider_created` (`provider_id`,`fact_date`,`run_id`) USING BTREE,
  KEY `idx_ai_run_dashboard_facts_user_created` (`user_id`,`fact_date`,`run_id`) USING BTREE,
  CONSTRAINT `fk_ai_run_dashboard_facts_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_run_dashboard_facts_nonnegative` CHECK (((`actual_units` >= 0) and (`released_units` >= 0) and (`settled_runs` between 0 and 1) and (`released_runs` between 0 and 1) and (`unbilled_runs` between 0 and 1))),
  CONSTRAINT `chk_ai_run_dashboard_facts_status` CHECK ((`status` in (_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'outcome_unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='Immutable terminal Run projection for exact AI dashboard analytics';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_run_events` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '事件ID',
  `run_id` bigint unsigned NOT NULL COMMENT 'ai_runs.id',
  `seq` int unsigned NOT NULL COMMENT '同一run内事件序号',
  `event_type` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT 'start/completed/failed/canceled/timeout',
  `message` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '事件说明或错误原因',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '事件时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_run_events_run_seq` (`run_id`,`seq`) USING BTREE,
  KEY `idx_ai_run_events_run_id` (`run_id`,`id`) USING BTREE,
  KEY `idx_ai_run_events_type_created` (`event_type`,`created_at`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_run_events_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_run_events_type` CHECK ((`event_type` in (_utf8mb4'start',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'retry_scheduled',_utf8mb4'usage_recorded',_utf8mb4'outcome_unknown',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unbilled',_utf8mb4'file_materialized_v1')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI运行监控事件';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_runs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '运行ID',
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `conversation_id` int unsigned DEFAULT NULL COMMENT 'ai_conversations.id; chat rows only',
  `request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `request_fingerprint` binary(32) NOT NULL,
  `request_identity_status` varchar(24) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'replayable',
  `request_identity_marker` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `user_message_id` bigint unsigned DEFAULT NULL COMMENT '本轮用户消息ID; chat rows only',
  `assistant_message_id` bigint unsigned DEFAULT NULL COMMENT '完成后写入的助手消息ID; chat rows only',
  `user_id` int unsigned NOT NULL COMMENT '发起用户ID',
  `agent_id` bigint unsigned NOT NULL COMMENT 'ai_agents.id',
  `provider_id` bigint unsigned NOT NULL COMMENT 'ai_providers.id',
  `model_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '实际调用模型ID',
  `model_display_name` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '实际调用模型展示名',
  `input_snapshot` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `pricing_snapshot_json` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `idempotency_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT 'queued/running/success/failed/canceled/timeout',
  `billing_status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `billing_reason` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `prompt_tokens` int unsigned NOT NULL DEFAULT '0' COMMENT '输入token',
  `completion_tokens` int unsigned NOT NULL DEFAULT '0' COMMENT '输出token',
  `total_tokens` int unsigned NOT NULL DEFAULT '0' COMMENT '总token',
  `duration_ms` int unsigned DEFAULT NULL COMMENT '运行耗时毫秒，终态后写入',
  `error_message` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '失败/取消/超时原因',
  `started_at` datetime DEFAULT NULL COMMENT '开始调用模型时间',
  `finished_at` datetime DEFAULT NULL COMMENT '进入终态时间',
  `settled_at` datetime(6) DEFAULT NULL,
  `liked_at` datetime(6) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_runs_user_request` (`user_id`,`request_id`) USING BTREE,
  UNIQUE KEY `uk_ai_runs_user_message` (`user_message_id`) USING BTREE,
  UNIQUE KEY `uk_ai_runs_idempotency` (`idempotency_key`) USING BTREE,
  UNIQUE KEY `uk_ai_runs_command_owner` (`id`,`user_id`,`conversation_id`,`user_message_id`,`request_id`),
  KEY `idx_ai_runs_created` (`created_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_status_created` (`status`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_user_created` (`user_id`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_agent_created` (`agent_id`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_provider_created` (`provider_id`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_conversation_created` (`conversation_id`,`created_at`,`id`) USING BTREE,
  KEY `fk_ai_runs_assistant_message` (`assistant_message_id`) USING BTREE,
  KEY `idx_ai_runs_status_started` (`status`,`started_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_model_created` (`model_id`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_runs_billing_created` (`billing_status`,`billing_reason`,`created_at`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_runs_assistant_message` FOREIGN KEY (`assistant_message_id`) REFERENCES `ai_messages` (`id`) ON DELETE SET NULL ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_runs_conversation` FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_runs_user_message` FOREIGN KEY (`user_message_id`) REFERENCES `ai_messages` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_runs_billing_reason` CHECK ((`billing_reason` in (_utf8mb4'pending',_utf8mb4'held',_utf8mb4'settled_complete_usage',_utf8mb4'released_before_dispatch',_utf8mb4'released_insufficient_balance',_utf8mb4'released_provider_failed',_utf8mb4'released_outcome_unknown',_utf8mb4'unbilled_usage_incomplete',_utf8mb4'unbilled_over_hold',_utf8mb4'legacy_unpriced'))),
  CONSTRAINT `chk_ai_runs_billing_status` CHECK ((`billing_status` in (_utf8mb4'pending',_utf8mb4'held',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unbilled'))),
  CONSTRAINT `chk_ai_runs_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))),
  CONSTRAINT `chk_ai_runs_request_identity` CHECK ((((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))),
  CONSTRAINT `chk_ai_runs_status` CHECK ((`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'outcome_unknown')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI运行监控记录';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_text_tasks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `user_id` bigint unsigned NOT NULL,
  `request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `request_fingerprint` binary(32) NOT NULL,
  `run_id` bigint unsigned NOT NULL,
  `kind` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'text',
  `request_identity_status` varchar(24) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'replayable',
  `request_identity_marker` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `agent_id` bigint unsigned NOT NULL,
  `provider_id` bigint unsigned NOT NULL,
  `model_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `prompt` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `answer` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `error_message` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `last_error_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `started_at` datetime DEFAULT NULL,
  `finished_at` datetime DEFAULT NULL,
  `elapsed_ms` int unsigned NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_text_tasks_user_request` (`user_id`,`request_id`) USING BTREE,
  UNIQUE KEY `uk_ai_text_tasks_run` (`run_id`),
  KEY `idx_ai_text_tasks_user_created` (`user_id`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_text_tasks_status_created` (`status`,`created_at`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_text_tasks_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_text_tasks_kind` CHECK ((`kind` in (_utf8mb4'text',_utf8mb4'tool_draft'))),
  CONSTRAINT `chk_ai_text_tasks_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))),
  CONSTRAINT `chk_ai_text_tasks_request_identity` CHECK ((((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))),
  CONSTRAINT `chk_ai_text_tasks_status` CHECK ((`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI文本生成任务';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_tool_calls` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '工具调用ID',
  `run_id` bigint unsigned NOT NULL COMMENT 'ai_runs.id',
  `tool_id` bigint unsigned NOT NULL COMMENT 'ai_tools.id',
  `tool_code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '调用时工具编码快照',
  `tool_name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '调用时工具名称快照',
  `call_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '模型返回的tool_call_id/call_id，用于回传工具结果',
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT 'running/success/failed/timeout',
  `arguments_json` json NOT NULL COMMENT '模型传入参数',
  `result_json` json DEFAULT NULL COMMENT '工具返回结果',
  `error_message` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '失败或超时原因',
  `duration_ms` int unsigned DEFAULT NULL COMMENT '执行耗时毫秒，终态后写入',
  `started_at` datetime NOT NULL COMMENT '开始执行时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_tool_calls_run_call` (`run_id`,`call_id`) USING BTREE,
  KEY `idx_ai_tool_calls_run_id` (`run_id`,`id`) USING BTREE,
  KEY `idx_ai_tool_calls_tool_created` (`tool_id`,`created_at`,`id`) USING BTREE,
  KEY `idx_ai_tool_calls_status_created` (`status`,`created_at`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_tool_calls_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_tool_calls_tool` FOREIGN KEY (`tool_id`) REFERENCES `ai_tools` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_tool_calls_status` CHECK ((`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed',_utf8mb4'timeout')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI工具调用记录';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_tools` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '工具ID',
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '工具名称，管理页和运行监控展示',
  `code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '工具唯一编码，传给模型作为function name',
  `description` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '工具说明，传给模型作为function description',
  `parameters_json` json NOT NULL COMMENT '工具参数JSON Schema，传给模型并用于入参校验',
  `result_schema_json` json NOT NULL COMMENT '工具返回JSON Schema，用于结果校验和运行监控展示',
  `risk_level` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '风险等级：low/medium/high',
  `timeout_ms` int unsigned NOT NULL DEFAULT '3000' COMMENT '执行超时毫秒，运行时context timeout',
  `status` tinyint unsigned NOT NULL DEFAULT '1' COMMENT '1启用 2禁用',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '1删除 2正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_tools_code` (`code`) USING BTREE,
  KEY `idx_ai_tools_status_del` (`status`,`is_del`,`id`) USING BTREE,
  CONSTRAINT `chk_ai_tools_is_del` CHECK ((`is_del` in (1,2))),
  CONSTRAINT `chk_ai_tools_risk_level` CHECK ((`risk_level` in (_utf8mb4'low',_utf8mb4'medium',_utf8mb4'high'))),
  CONSTRAINT `chk_ai_tools_status` CHECK ((`status` in (1,2)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='AI工具定义';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_usage_charge_items` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `charge_id` bigint unsigned NOT NULL,
  `attempt_id` bigint unsigned NOT NULL,
  `category` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `tier_key` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `quantity` bigint NOT NULL,
  `unit` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `unit_price_units` bigint NOT NULL,
  `unit_scale` bigint NOT NULL,
  `amount_units` bigint NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_usage_charge_item_identity` (`charge_id`,`attempt_id`,`category`,`tier_key`,`unit`) USING BTREE,
  KEY `idx_ai_usage_charge_items_attempt` (`attempt_id`) USING BTREE,
  CONSTRAINT `fk_ai_usage_charge_items_attempt` FOREIGN KEY (`attempt_id`) REFERENCES `ai_provider_attempts` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_ai_usage_charge_items_charge` FOREIGN KEY (`charge_id`) REFERENCES `ai_usage_charges` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_usage_charge_items_category` CHECK ((`category` in (_utf8mb4'input',_utf8mb4'output',_utf8mb4'cache_read',_utf8mb4'cache_write',_utf8mb4'media'))),
  CONSTRAINT `chk_ai_usage_charge_items_units` CHECK (((`quantity` > 0) and (`unit_price_units` >= 0) and (`unit_scale` > 0) and (`amount_units` >= 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `ai_usage_charges` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `run_id` bigint unsigned NOT NULL,
  `user_id` bigint NOT NULL,
  `currency` char(3) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'CNY',
  `pricing_version` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `multiplier_ppm` bigint unsigned NOT NULL,
  `held_units` bigint NOT NULL DEFAULT '0',
  `actual_units` bigint NOT NULL DEFAULT '0',
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'open',
  `finalized_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_ai_usage_charges_run` (`run_id`) USING BTREE,
  KEY `idx_ai_usage_charges_user_created` (`user_id`,`created_at`,`id`) USING BTREE,
  CONSTRAINT `fk_ai_usage_charges_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_ai_usage_charges_currency` CHECK ((`currency` = _utf8mb4'CNY')),
  CONSTRAINT `chk_ai_usage_charges_status` CHECK ((`status` in (_utf8mb4'open',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unbilled'))),
  CONSTRAINT `chk_ai_usage_charges_units` CHECK (((`held_units` >= 0) and (`actual_units` >= 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `auth_platforms` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '平台标识（如 admin, app）',
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '平台名称',
  `login_types` json NOT NULL COMMENT '允许的登录方式 ["password","email","phone"]',
  `captcha_type` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'slide' COMMENT '验证码类型: slide',
  `access_ttl` int unsigned NOT NULL DEFAULT '14400' COMMENT 'access_token 有效期（秒）',
  `refresh_ttl` int unsigned NOT NULL DEFAULT '1209600' COMMENT 'refresh_token 有效期（秒）',
  `bind_platform` tinyint unsigned NOT NULL DEFAULT '1' COMMENT '绑定平台 1=是 2=否',
  `bind_device` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '绑定设备 1=是 2=否',
  `bind_ip` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '绑定IP 1=是 2=否',
  `max_sessions` int unsigned NOT NULL DEFAULT '5' COMMENT '最大会话数（0=不限）',
  `allow_register` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '允许注册 1=是 2=否',
  `status` tinyint unsigned NOT NULL DEFAULT '1' COMMENT '状态 1=启用 2=禁用',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '软删除 1=已删 2=正常',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_code` (`code`) USING BTREE,
  KEY `idx_status_del` (`status`,`is_del`) USING BTREE,
  CONSTRAINT `chk_auth_platforms_code` CHECK ((regexp_like(`code`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`code` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`code` <> _utf8mb4'all')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='认证平台管理';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `authz_principal_versions` (
  `user_id` bigint NOT NULL,
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `version` bigint unsigned NOT NULL DEFAULT '1',
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`user_id`,`platform`) USING BTREE,
  CONSTRAINT `chk_authz_principal_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `cron_task` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '任务标识（唯一）',
  `title` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '任务名称',
  `description` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '任务描述',
  `cron` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT 'Cron表达式',
  `cron_readable` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT 'Cron可读描述',
  `handler` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '处理类',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_cron_task_name` (`name`) USING BTREE,
  KEY `idx_status_del` (`status`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='定时任务配置表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `cron_task_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_id` bigint unsigned NOT NULL COMMENT '任务ID',
  `task_name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '任务标识',
  `start_time` datetime(3) NOT NULL COMMENT '开始时间',
  `end_time` datetime(3) DEFAULT NULL COMMENT '结束时间',
  `duration_ms` int unsigned DEFAULT NULL COMMENT '执行耗时(毫秒)',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `result` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci COMMENT '执行结果',
  `error_msg` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci COMMENT '错误信息',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT 'soft delete: 1 deleted 2 normal',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_task_del_id` (`task_id`,`is_del`) USING BTREE,
  KEY `idx_name_del_id` (`task_name`,`is_del`) USING BTREE,
  KEY `idx_cron_task_log_task_active_created` (`task_id`,`is_del`,`created_at` DESC,`id` DESC) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='定时任务执行日志表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `export_tasks` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL COMMENT '创建用户ID',
  `platform` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'admin' COMMENT '平台入口',
  `title` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '任务标题',
  `kind` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'user_list' COMMENT '导出类型',
  `file_name` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '文件名',
  `file_url` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '文件下载URL',
  `object_key` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT 'COS object key',
  `file_size` int unsigned DEFAULT NULL COMMENT '文件大小（字节）',
  `row_count` int unsigned DEFAULT NULL COMMENT '数据行数',
  `status` tinyint unsigned NOT NULL DEFAULT '1' COMMENT '1处理中 2成功 3失败',
  `claim_owner` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `claim_token` bigint unsigned NOT NULL DEFAULT '0',
  `claim_expires_at` datetime(6) DEFAULT NULL,
  `error_msg` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '失败原因',
  `expire_at` datetime DEFAULT NULL COMMENT '过期时间（定时任务清理）',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '2正常 1删除',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_user_status` (`user_id`,`status`,`is_del`) USING BTREE,
  KEY `idx_expire` (`expire_at`) USING BTREE,
  KEY `idx_created` (`created_at`) USING BTREE,
  KEY `idx_export_tasks_user_platform_status` (`user_id`,`platform`,`status`,`is_del`) USING BTREE,
  KEY `idx_export_tasks_user_platform_kind` (`user_id`,`platform`,`kind`,`is_del`) USING BTREE,
  KEY `idx_export_task_claim` (`status`,`is_del`,`claim_expires_at`,`id`) USING BTREE,
  KEY `idx_export_tasks_user_platform_active_id` (`user_id`,`platform`,`is_del`,`id`) USING BTREE,
  CONSTRAINT `chk_export_tasks_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='导出任务记录';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `job_history` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `job_date` date NOT NULL,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `total_items` int NOT NULL,
  `downloaded` int NOT NULL,
  `skipped` int NOT NULL,
  `errors` int NOT NULL,
  `orig_files` int NOT NULL,
  `chg_files` int NOT NULL,
  `error_message` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `started_at` datetime DEFAULT NULL,
  `completed_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `ix_job_history_job_date` (`job_date`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `mail_configs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `config_key` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'default',
  `secret_id_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `secret_id_hint` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `secret_key_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `secret_key_hint` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `region` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'ap-guangzhou',
  `endpoint` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'ses.tencentcloudapi.com',
  `from_email` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `from_name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `reply_to` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `verify_code_ttl_minutes` int unsigned NOT NULL DEFAULT '5',
  `status` tinyint unsigned NOT NULL DEFAULT '2',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `last_test_at` datetime DEFAULT NULL,
  `last_test_error` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_mail_configs_config_key` (`config_key`) USING BTREE,
  KEY `idx_mail_configs_status_del` (`status`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `mail_log_verification_codes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `mail_log_id` bigint unsigned NOT NULL,
  `key_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `code_enc` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `expires_at` datetime NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_mail_log_verification_codes_mail_log` (`mail_log_id`) USING BTREE,
  KEY `idx_mail_log_verification_codes_key_id_id` (`key_id`,`id`) USING BTREE,
  CONSTRAINT `fk_mail_log_verification_codes_mail_log` FOREIGN KEY (`mail_log_id`) REFERENCES `mail_logs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `mail_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `scene` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `template_id` bigint unsigned DEFAULT NULL,
  `to_email` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `subject` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `tencent_request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `tencent_message_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `status` tinyint unsigned NOT NULL,
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `error_code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `error_message` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `duration_ms` bigint unsigned NOT NULL DEFAULT '0',
  `sent_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_mail_logs_scene_created` (`is_del`,`scene`,`created_at`) USING BTREE,
  KEY `idx_mail_logs_status_created` (`is_del`,`status`,`created_at`) USING BTREE,
  KEY `idx_mail_logs_to_email_created` (`is_del`,`to_email`,`created_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `mail_templates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `scene` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `subject` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `tencent_template_id` bigint unsigned NOT NULL,
  `variables_json` json NOT NULL,
  `sample_variables_json` json NOT NULL,
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_mail_templates_scene` (`scene`) USING BTREE,
  KEY `idx_mail_templates_status_del` (`status`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `notice_files` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `notice_id` bigint NOT NULL,
  `file_type` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `file_name` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `file_size` bigint DEFAULT NULL,
  `download_path` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `download_date` date DEFAULT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `ix_notice_files_download_date` (`download_date`) USING BTREE,
  KEY `ix_notice_files_notice_id` (`notice_id`) USING BTREE,
  CONSTRAINT `notice_files_ibfk_1` FOREIGN KEY (`notice_id`) REFERENCES `notices` (`notice_id`) ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `notices` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `notice_id` bigint NOT NULL,
  `notice_type` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `is_change` tinyint(1) NOT NULL,
  `project_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `project_name` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `project_status` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `purchase_type` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `bid_org` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `bid_agency` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `bid_agency_addr` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `bidbook_buy_end_time` datetime DEFAULT NULL,
  `bidbook_sell_begin_time` datetime DEFAULT NULL,
  `openbid_time` datetime DEFAULT NULL,
  `openbid_addr` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `contact_person` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `contact_phone` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `contact_fax` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `email` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `pay_mode` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `project_introduce` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `change_content` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `orig_notice_id` bigint DEFAULT NULL,
  `publish_time` date DEFAULT NULL,
  `publish_org` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `source_url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `raw_json` json DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `ix_notices_notice_id` (`notice_id`) USING BTREE,
  KEY `ix_notices_publish_time` (`publish_time`) USING BTREE,
  KEY `ix_notices_orig_notice_id` (`orig_notice_id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `notification_task` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '标题',
  `content` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci COMMENT '内容',
  `type` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'type: 1 info 2 success 3 warning 4 error',
  `level` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'level: 1 normal 2 urgent',
  `link` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT '' COMMENT '跳转链接',
  `platform` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'all' COMMENT '平台 all/admin/app',
  `target_type` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'target type: 1 all 2 users 3 roles',
  `target_ids` json DEFAULT NULL COMMENT '目标ID列表',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `claim_owner` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `claim_token` bigint unsigned NOT NULL DEFAULT '0',
  `claim_expires_at` datetime(6) DEFAULT NULL,
  `total_count` int unsigned NOT NULL DEFAULT '0' COMMENT '目标用户数',
  `sent_count` int unsigned NOT NULL DEFAULT '0' COMMENT '已发送数',
  `send_at` datetime DEFAULT NULL COMMENT '定时发送时间（空=立即发送）',
  `error_msg` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '错误信息',
  `created_by` int unsigned NOT NULL COMMENT 'Creator user id',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT 'soft delete: 1 deleted 2 normal',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_status_del_send` (`status`,`is_del`,`send_at`) USING BTREE,
  KEY `idx_notification_task_claim` (`status`,`is_del`,`send_at`,`claim_expires_at`,`id`) USING BTREE,
  CONSTRAINT `chk_notification_task_platform` CHECK (((`platform` = _utf8mb4'all') or (regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `notifications` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL COMMENT '接收用户ID',
  `source_task_id` bigint DEFAULT NULL,
  `title` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '标题',
  `content` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT '' COMMENT '内容',
  `type` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'type: 1 normal 2 success 3 warning 4 error',
  `level` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'level: 1 normal 2 urgent',
  `link` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT '' COMMENT '跳转路由',
  `platform` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'all' COMMENT '平台 all/admin/app',
  `is_read` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '1 read 2 unread',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_notifications_source_user` (`source_task_id`,`user_id`) USING BTREE,
  KEY `idx_user_platform_del_id` (`user_id`,`is_del`,`id`) USING BTREE,
  KEY `idx_notifications_user_active_unread_platform` (`user_id`,`is_del`,`is_read`,`platform`,`id`) USING BTREE,
  CONSTRAINT `chk_notifications_platform` CHECK (((`platform` = _utf8mb4'all') or (regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='用户通知表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `operation_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `user_id` int unsigned NOT NULL DEFAULT '0',
  `action` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '操作行为/接口名称',
  `request_data` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci COMMENT '请求入参',
  `response_data` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci COMMENT '响应出参',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '2正常 1删除',
  `is_success` tinyint unsigned NOT NULL DEFAULT '1' COMMENT '1 success 2 fail',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_user_id` (`user_id`) USING BTREE,
  KEY `idx_action` (`action`) USING BTREE,
  KEY `idx_created_at` (`created_at`) USING BTREE,
  KEY `idx_del_created_id` (`is_del`,`created_at`,`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='操作日志表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `payment_callback_events` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `provider` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'alipay',
  `dedupe_key` binary(32) NOT NULL,
  `notify_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `out_trade_no` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `trade_no` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `trade_status` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `app_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `total_amount_cents` bigint NOT NULL DEFAULT '0',
  `signature_valid` tinyint NOT NULL DEFAULT '2',
  `process_status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `process_message` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `raw_payload_json` json DEFAULT NULL,
  `received_at` datetime NOT NULL,
  `processed_at` datetime DEFAULT NULL,
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_payment_callback_events_dedupe` (`dedupe_key`),
  KEY `idx_payment_callback_events_notify_id` (`provider`,`notify_id`) USING BTREE,
  KEY `idx_payment_callback_events_out_trade_no` (`provider`,`out_trade_no`) USING BTREE,
  KEY `idx_payment_callback_events_status_time` (`process_status`,`received_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `payment_configs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `provider` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'alipay',
  `code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `app_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `private_key_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `private_key_hint` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `app_cert_path` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `platform_cert_path` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `root_cert_path` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `notify_url` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `environment` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'sandbox',
  `enabled_methods_json` json NOT NULL,
  `sort` int NOT NULL DEFAULT '100',
  `status` tinyint NOT NULL DEFAULT '2',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_payment_configs_code` (`code`) USING BTREE,
  KEY `idx_payment_configs_provider_status` (`provider`,`status`,`is_del`) USING BTREE,
  KEY `idx_payment_configs_environment` (`environment`,`is_del`) USING BTREE,
  KEY `idx_payment_configs_provider_status_sort` (`provider`,`status`,`is_del`,`sort`,`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `payment_orders` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `order_no` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_id` bigint NOT NULL,
  `config_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `provider` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'alipay',
  `pay_method` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `subject` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount_cents` bigint NOT NULL,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `pay_url` varchar(2048) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `return_url` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `alipay_trade_no` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `alipay_trade_no_identity` varchar(64) CHARACTER SET ascii COLLATE ascii_bin DEFAULT NULL,
  `expired_at` datetime NOT NULL,
  `paid_at` datetime DEFAULT NULL,
  `closed_at` datetime DEFAULT NULL,
  `failure_reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_payment_order_no` (`order_no`) USING BTREE,
  UNIQUE KEY `uk_payment_orders_alipay_trade_identity` (`alipay_trade_no_identity`),
  KEY `idx_payment_order_status_created` (`is_del`,`status`,`created_at`) USING BTREE,
  KEY `idx_payment_order_config_created` (`config_id`,`created_at`,`is_del`) USING BTREE,
  KEY `idx_payment_orders_provider_status_expired` (`provider`,`status`,`is_del`,`expired_at`,`id`) USING BTREE,
  KEY `idx_payment_orders_status_updated` (`status`,`is_del`,`updated_at`,`id`) USING BTREE,
  CONSTRAINT `fk_payment_order_config` FOREIGN KEY (`config_id`) REFERENCES `payment_configs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_payment_orders_alipay_trade_identity` CHECK ((((`alipay_trade_no` = _utf8mb4'') and (`alipay_trade_no_identity` is null)) or ((`alipay_trade_no` <> _utf8mb4'') and (cast(`alipay_trade_no_identity` as char charset binary) = cast(`alipay_trade_no` as char charset binary)))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `payment_recharge_packages` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount_cents` bigint NOT NULL,
  `badge` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `sort` int NOT NULL DEFAULT '100',
  `status` tinyint NOT NULL DEFAULT '1',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_payment_recharge_package_code` (`code`) USING BTREE,
  KEY `idx_payment_recharge_package_status_sort` (`status`,`is_del`,`sort`,`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `payment_recharges` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `recharge_no` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `user_id` bigint NOT NULL,
  `package_code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `package_name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount_cents` bigint NOT NULL,
  `payment_order_id` bigint NOT NULL,
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `paid_at` datetime DEFAULT NULL,
  `credited_at` datetime DEFAULT NULL,
  `failure_reason` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_payment_recharge_no` (`recharge_no`) USING BTREE,
  UNIQUE KEY `uk_payment_recharge_order` (`payment_order_id`) USING BTREE,
  KEY `idx_payment_recharge_user_status_created` (`user_id`,`is_del`,`status`,`created_at`) USING BTREE,
  KEY `idx_payment_recharge_created` (`is_del`,`created_at`) USING BTREE,
  CONSTRAINT `fk_payment_recharge_order` FOREIGN KEY (`payment_order_id`) REFERENCES `payment_orders` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `permissions` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '权限名',
  `path` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT '' COMMENT '路由',
  `icon` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT '' COMMENT '图标',
  `parent_id` int unsigned NOT NULL DEFAULT '0' COMMENT 'parent permission id; 0 means root',
  `component` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '组件路径',
  `platform` varchar(10) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'admin' COMMENT '平台：admin=PC后台, app=H5/APP',
  `type` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'type: 1 dir 2 page 3 button',
  `sort` int unsigned NOT NULL DEFAULT '0' COMMENT '排序',
  `code` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '权限标识',
  `i18n_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT 'i18n键',
  `show_menu` tinyint unsigned NOT NULL DEFAULT '1' COMMENT 'show menu: 1 yes 2 no',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_permissions_platform_code` (`platform`,`code`) USING BTREE,
  KEY `idx_permissions_platform` (`platform`) USING BTREE,
  KEY `idx_permissions_parent_sort` (`parent_id`,`sort`) USING BTREE,
  KEY `idx_permissions_status_del_platform_type` (`is_del`,`status`,`platform`,`type`) USING BTREE,
  CONSTRAINT `chk_permissions_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='菜单权限表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `realtime_event_retention_watermarks` (
  `target_type` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `target_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `deleted_through_sequence` bigint unsigned NOT NULL DEFAULT '0',
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`target_type`,`target_id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `realtime_events` (
  `sequence` bigint unsigned NOT NULL AUTO_INCREMENT,
  `event_id` char(26) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `event_type` varchar(96) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `target_type` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `target_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `durability` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `payload_json` json NOT NULL,
  `occurred_at` datetime(6) NOT NULL,
  `expires_at` datetime(6) NOT NULL,
  PRIMARY KEY (`sequence`) USING BTREE,
  UNIQUE KEY `uk_realtime_event_id` (`event_id`) USING BTREE,
  KEY `idx_realtime_resume` (`target_type`,`target_id`,`sequence`) USING BTREE,
  KEY `idx_realtime_expiry` (`expires_at`,`sequence`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `redeem_code_batches` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `batch_no` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_id` varchar(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_fingerprint_version` varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_fingerprint` char(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `amount_cents` bigint NOT NULL,
  `quantity` int unsigned NOT NULL,
  `expires_at` datetime(6) DEFAULT NULL,
  `note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `created_by` int unsigned NOT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_redeem_code_batches_batch_no` (`batch_no`) USING BTREE,
  UNIQUE KEY `uk_redeem_code_batches_creator_request` (`created_by`,`request_id`) USING BTREE,
  KEY `idx_redeem_code_batches_created_at_id` (`created_at`,`id`) USING BTREE,
  KEY `idx_redeem_code_batches_expires_at_id` (`expires_at`,`id`) USING BTREE,
  CONSTRAINT `fk_redeem_code_batches_created_by` FOREIGN KEY (`created_by`) REFERENCES `users` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_redeem_code_batches_amount_cents` CHECK ((`amount_cents` between 1 and 100000000)),
  CONSTRAINT `chk_redeem_code_batches_expiry` CHECK (((`expires_at` is null) or (`expires_at` > `created_at`))),
  CONSTRAINT `chk_redeem_code_batches_quantity` CHECK ((`quantity` between 1 and 1000))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `redeem_codes` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `batch_id` bigint NOT NULL,
  `code` char(28) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `used_by` int unsigned DEFAULT NULL,
  `used_at` datetime(6) DEFAULT NULL,
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_redeem_codes_code` (`code`) USING BTREE,
  KEY `idx_redeem_codes_batch_state_id` (`batch_id`,`state`,`id`) USING BTREE,
  KEY `idx_redeem_codes_state_id` (`state`,`id`) USING BTREE,
  KEY `idx_redeem_codes_used_by_used_at_id` (`used_by`,`used_at`,`id`) USING BTREE,
  CONSTRAINT `fk_redeem_codes_batch` FOREIGN KEY (`batch_id`) REFERENCES `redeem_code_batches` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_redeem_codes_used_by` FOREIGN KEY (`used_by`) REFERENCES `users` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_redeem_codes_state` CHECK ((`state` in (_utf8mb4'unused',_utf8mb4'used',_utf8mb4'voided'))),
  CONSTRAINT `chk_redeem_codes_usage` CHECK ((((`state` = _utf8mb4'used') and (`used_by` is not null) and (`used_at` is not null)) or ((`state` in (_utf8mb4'unused',_utf8mb4'voided')) and (`used_by` is null) and (`used_at` is null))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `role_permissions` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `role_id` int unsigned NOT NULL COMMENT 'role.id',
  `permission_id` int unsigned NOT NULL COMMENT 'permission.id',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_role_permission` (`role_id`,`permission_id`) USING BTREE,
  KEY `idx_role_permissions_permission_del_role` (`permission_id`,`is_del`,`role_id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='role permission pivot';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `roles` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT 'role name',
  `is_default` tinyint unsigned NOT NULL DEFAULT '2',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_roles_name` (`name`) USING BTREE,
  KEY `idx_roles_default_del` (`is_default`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='角色';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `scheduler_settings` (
  `id` int NOT NULL,
  `enabled` tinyint(1) NOT NULL,
  `cron_schedule` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `timezone` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `sms_configs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `config_key` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'default',
  `secret_id_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `secret_id_hint` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `secret_key_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `secret_key_hint` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `sms_sdk_app_id` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `sign_name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `region` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'ap-guangzhou',
  `endpoint` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'sms.tencentcloudapi.com',
  `verify_code_ttl_minutes` int unsigned NOT NULL DEFAULT '5',
  `status` tinyint unsigned NOT NULL DEFAULT '2',
  `last_test_at` datetime DEFAULT NULL,
  `last_test_error` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_sms_configs_config_key` (`config_key`) USING BTREE,
  KEY `idx_sms_configs_status_del` (`status`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `sms_logs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `scene` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `template_id` bigint unsigned DEFAULT NULL,
  `to_phone` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `status` tinyint unsigned NOT NULL,
  `tencent_request_id` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `tencent_serial_no` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `tencent_fee` bigint unsigned NOT NULL DEFAULT '0',
  `error_code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `error_message` varchar(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `duration_ms` bigint unsigned NOT NULL DEFAULT '0',
  `sent_at` datetime DEFAULT NULL,
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_sms_logs_scene_created` (`is_del`,`scene`,`created_at`) USING BTREE,
  KEY `idx_sms_logs_status_created` (`is_del`,`status`,`created_at`) USING BTREE,
  KEY `idx_sms_logs_to_phone_created` (`is_del`,`to_phone`,`created_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `sms_templates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `scene` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `tencent_template_id` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `variables_json` json NOT NULL,
  `sample_variables_json` json NOT NULL,
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_sms_templates_scene` (`scene`) USING BTREE,
  KEY `idx_sms_templates_status_del` (`status`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `system_settings` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `setting_key` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '配置键：如 user.default_avatar',
  `setting_value` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '配置值（字符串/JSON字符串均可）',
  `value_type` tinyint unsigned NOT NULL DEFAULT '1',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '备注说明',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_setting_key` (`setting_key`) USING BTREE,
  KEY `idx_status_del` (`status`,`is_del`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='系统设置（key-value）';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `upload_driver` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `driver` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT 'cos / oss / s3 / qiniu 等',
  `secret_id_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `secret_id_hint` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `secret_key_enc` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci,
  `secret_key_hint` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  `bucket` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `region` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `appid` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT 'COS 特有',
  `endpoint` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT 'OSS/S3/AP custom domain',
  `bucket_domain` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '返回给前端用于访问的域名（可配 CDN）',
  `role_arn` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT 'OSS AssumeRole / AWS role arn',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_driver_bucket` (`driver`,`bucket`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `upload_rule` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '规则标题',
  `max_size_mb` int unsigned NOT NULL DEFAULT '5' COMMENT '最大 MB',
  `image_exts` json NOT NULL COMMENT '允许的图片扩展名',
  `file_exts` json NOT NULL COMMENT '允许的通用文件扩展名',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `upload_setting` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `driver_id` int unsigned NOT NULL,
  `rule_id` int unsigned NOT NULL,
  `status` tinyint unsigned NOT NULL DEFAULT '2',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_driver_rule` (`driver_id`,`rule_id`) USING BTREE,
  KEY `idx_status` (`status`) USING BTREE,
  KEY `idx_rule` (`rule_id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='上传设置：驱动+规则组合与启用状态';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_profiles` (
  `user_id` int unsigned NOT NULL,
  `avatar` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'https://zgm-1314542588.cos.ap-nanjing.myqcloud.com/defaultAvatar%2Favatar.jpg' COMMENT '头像',
  `bio` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci COMMENT '个人简介',
  `sex` tinyint unsigned NOT NULL DEFAULT '0' COMMENT 'sex: 0 unknown 1 male 2 female',
  `birthday` date DEFAULT NULL COMMENT '生日',
  `address_id` int unsigned DEFAULT NULL COMMENT '地址ID',
  `detail_address` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '详细地址',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT 'soft delete: 1 deleted 2 normal',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`user_id`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='用户资料表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_sessions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL,
  `refresh_token_hash` char(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT 'refresh token sha256',
  `platform` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT 'pc/h5/app/mini',
  `device_id` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '设备标识(前端生成uuid即可)',
  `ip` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '登录IP',
  `ua` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT 'User-Agent',
  `last_seen_at` datetime DEFAULT NULL COMMENT '最后活跃时间',
  `expires_at` datetime NOT NULL COMMENT 'access过期时间',
  `refresh_expires_at` datetime NOT NULL COMMENT 'refresh过期时间',
  `revoked_at` datetime DEFAULT NULL COMMENT '注销/踢下线时间',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '2 normal 1 deleted',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_refresh_hash` (`refresh_token_hash`) USING BTREE,
  KEY `idx_user_platform` (`user_id`,`platform`) USING BTREE,
  KEY `idx_expires_at` (`expires_at`) USING BTREE,
  KEY `idx_refresh_expires_at` (`refresh_expires_at`) USING BTREE,
  KEY `idx_active_stats` (`is_del`,`revoked_at`,`expires_at`,`platform`) USING BTREE,
  KEY `idx_user_sessions_user_platform_active_refresh` (`user_id`,`platform`,`is_del`,`revoked_at`,`refresh_expires_at`,`id`) USING BTREE,
  CONSTRAINT `chk_user_sessions_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='用户会话表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_wallets` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `balance_units` bigint NOT NULL DEFAULT '0',
  `total_recharge_units` bigint NOT NULL DEFAULT '0',
  `total_consume_units` bigint NOT NULL DEFAULT '0',
  `held_units` bigint NOT NULL DEFAULT '0',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_user_wallet_user` (`user_id`) USING BTREE,
  KEY `idx_user_wallet_isdel` (`is_del`) USING BTREE,
  KEY `idx_user_wallet_updated` (`is_del`,`updated_at`,`id`) USING BTREE,
  CONSTRAINT `chk_user_wallet_units_nonnegative` CHECK (((`balance_units` >= 0) and (`total_recharge_units` >= 0) and (`total_consume_units` >= 0) and (`held_units` >= 0) and (`held_units` <= `balance_units`)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `users` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `role_id` int unsigned NOT NULL DEFAULT '1',
  `username` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '用户名',
  `email` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '邮箱',
  `password` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '密码(可空: 首次第三方/邮箱免密创建)',
  `phone` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT '手机号',
  `status` tinyint unsigned NOT NULL DEFAULT '1',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uniq_users_email` (`email`) USING BTREE,
  UNIQUE KEY `uniq_users_phone` (`phone`) USING BTREE,
  KEY `idx_users_role_del` (`role_id`,`is_del`) USING BTREE,
  KEY `idx_users_active` (`is_del`,`status`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='用户表';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `users_login_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned DEFAULT NULL,
  `login_account` varchar(120) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '登录账号',
  `login_type` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'email' COMMENT '登录类型',
  `platform` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '平台',
  `ip` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT 'IP地址',
  `ua` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL COMMENT 'User-Agent',
  `is_success` tinyint unsigned NOT NULL DEFAULT '2' COMMENT '1 success 2 fail',
  `reason` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '失败原因',
  `is_del` tinyint unsigned NOT NULL DEFAULT '2' COMMENT 'soft delete: 1 deleted 2 normal',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated at',
  PRIMARY KEY (`id`) USING BTREE,
  KEY `idx_user_created` (`user_id`,`created_at` DESC) USING BTREE,
  KEY `idx_account_created` (`login_account`,`created_at` DESC) USING BTREE,
  KEY `idx_ip_created` (`ip`,`created_at` DESC) USING BTREE,
  KEY `idx_created` (`created_at` DESC) USING BTREE,
  CONSTRAINT `chk_users_login_log_platform` CHECK ((regexp_like(`platform`,_utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC COMMENT='登录日志';
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `wallet_holds` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `wallet_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `run_id` bigint unsigned NOT NULL,
  `held_units` bigint NOT NULL DEFAULT '0',
  `captured_units` bigint NOT NULL DEFAULT '0',
  `status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT 'active',
  `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_wallet_holds_run` (`run_id`) USING BTREE,
  KEY `idx_wallet_holds_wallet_status` (`wallet_id`,`status`) USING BTREE,
  CONSTRAINT `fk_wallet_holds_run` FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `fk_wallet_holds_wallet` FOREIGN KEY (`wallet_id`) REFERENCES `user_wallets` (`id`) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT `chk_wallet_holds_status` CHECK ((`status` in (_utf8mb4'active',_utf8mb4'captured',_utf8mb4'released'))),
  CONSTRAINT `chk_wallet_holds_units` CHECK ((((`status` = _utf8mb4'active') and (`held_units` > 0) and (`captured_units` = 0)) or ((`status` = _utf8mb4'captured') and (`held_units` = 0) and (`captured_units` >= 0)) or ((`status` = _utf8mb4'released') and (`held_units` = 0) and (`captured_units` = 0))))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `wallet_transactions` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `transaction_no` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `wallet_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `direction` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `amount_units` bigint NOT NULL,
  `balance_before_units` bigint NOT NULL,
  `balance_after_units` bigint NOT NULL,
  `source_type` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `source_id` bigint NOT NULL,
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `is_del` tinyint NOT NULL DEFAULT '2',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_wallet_transaction_no` (`transaction_no`) USING BTREE,
  UNIQUE KEY `uk_wallet_transaction_source` (`source_type`,`source_id`) USING BTREE,
  KEY `idx_wallet_transaction_user_created` (`user_id`,`is_del`,`created_at`) USING BTREE,
  KEY `idx_wallet_transaction_wallet_created` (`wallet_id`,`is_del`,`created_at`) USING BTREE,
  KEY `idx_wallet_tx_admin_created` (`is_del`,`created_at`,`id`) USING BTREE,
  KEY `idx_wallet_tx_admin_direction_created` (`direction`,`is_del`,`created_at`,`id`) USING BTREE,
  KEY `idx_wallet_tx_admin_source_created` (`source_type`,`is_del`,`created_at`,`id`) USING BTREE,
  CONSTRAINT `chk_wallet_transaction_units_nonnegative` CHECK (((`amount_units` >= 0) and (`balance_before_units` >= 0) and (`balance_after_units` >= 0)))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;

-- Forward migrations after baseline 202608130001 are recorded here.
CREATE TABLE `schema_migrations` (
  `version` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `checksum_sha256` char(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `applied_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`version`),
  CONSTRAINT `chk_schema_migrations_checksum` CHECK (regexp_like(`checksum_sha256`,_utf8mb4'^[0-9a-f]{64}$'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

