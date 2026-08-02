CREATE TABLE `ai_context_profiles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(191) NOT NULL,
  `embedding_provider_model_id` BIGINT UNSIGNED NOT NULL,
  `embedding_dimensions` INT UNSIGNED NOT NULL,
  `embedding_max_input_tokens` BIGINT UNSIGNED NOT NULL,
  `embedding_token_counter_id` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `dense_distance` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `dense_min_score` DECIMAL(20,6) NOT NULL,
  `sparse_encoder` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `sparse_encoder_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `reranker_provider_model_id` BIGINT UNSIGNED NULL,
  `reranker_min_score` DECIMAL(20,6) NULL,
  `memory_provider_model_id` BIGINT UNSIGNED NULL,
  `status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `active_index_generation` BIGINT UNSIGNED NULL,
  `target_index_generation` BIGINT UNSIGNED NULL,
  `index_state` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `index_error_code` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `index_verified_at` DATETIME(6) NULL,
  `created_by` INT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  KEY `idx_ai_context_profiles_status_state` (`status`, `index_state`, `id`),
  KEY `idx_ai_context_profiles_embedding_model` (`embedding_provider_model_id`),
  KEY `idx_ai_context_profiles_reranker_model` (`reranker_provider_model_id`),
  KEY `idx_ai_context_profiles_memory_model` (`memory_provider_model_id`),
  KEY `idx_ai_context_profiles_created_by` (`created_by`),
  CONSTRAINT `fk_ai_context_profiles_embedding_model`
    FOREIGN KEY (`embedding_provider_model_id`) REFERENCES `ai_provider_models` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_profiles_reranker_model`
    FOREIGN KEY (`reranker_provider_model_id`) REFERENCES `ai_provider_models` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_profiles_memory_model`
    FOREIGN KEY (`memory_provider_model_id`) REFERENCES `ai_provider_models` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_profiles_created_by`
    FOREIGN KEY (`created_by`) REFERENCES `users` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_profiles_embedding_shape`
    CHECK (`embedding_dimensions` > 0 AND `embedding_max_input_tokens` > 0),
  CONSTRAINT `chk_ai_context_profiles_dense_distance`
    CHECK (`dense_distance` IN (_ascii'cosine', _ascii'dot', _ascii'euclid')),
  CONSTRAINT `chk_ai_context_profiles_sparse_encoder`
    CHECK (`sparse_encoder` = _ascii'unicode_lexical_v1'),
  CONSTRAINT `chk_ai_context_profiles_reranker_pair`
    CHECK ((`reranker_provider_model_id` IS NULL AND `reranker_min_score` IS NULL)
      OR (`reranker_provider_model_id` IS NOT NULL AND `reranker_min_score` IS NOT NULL)),
  CONSTRAINT `chk_ai_context_profiles_status`
    CHECK (`status` IN (_ascii'enabled', _ascii'retired')),
  CONSTRAINT `chk_ai_context_profiles_index_state`
    CHECK (`index_state` IN (_ascii'provisioning', _ascii'ready', _ascii'rebuilding', _ascii'failed')),
  CONSTRAINT `chk_ai_context_profiles_generation_shape`
    CHECK (
      (`index_state` = _ascii'provisioning' AND `active_index_generation` IS NULL
        AND `target_index_generation` IS NOT NULL)
      OR (`index_state` = _ascii'ready' AND `active_index_generation` IS NOT NULL
        AND `target_index_generation` IS NULL)
      OR (`index_state` = _ascii'rebuilding' AND `target_index_generation` IS NOT NULL)
      OR (`index_state` = _ascii'failed')
    ),
  CONSTRAINT `chk_ai_context_profiles_generation_order`
    CHECK ((`active_index_generation` IS NULL OR `active_index_generation` > 0)
      AND (`target_index_generation` IS NULL OR `target_index_generation` > 0)
      AND (`active_index_generation` IS NULL OR `target_index_generation` IS NULL
        OR `target_index_generation` > `active_index_generation`)),
  CONSTRAINT `chk_ai_context_profiles_index_error`
    CHECK (`index_state` <> _ascii'failed'
      OR (`index_error_code` IS NOT NULL AND CHAR_LENGTH(`index_error_code`) > 0))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `ai_context_spaces` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `profile_id` BIGINT UNSIGNED NOT NULL,
  `name` VARCHAR(191) NOT NULL,
  `description` VARCHAR(1024) NOT NULL DEFAULT '',
  `status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `created_by` INT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  KEY `idx_ai_context_spaces_platform_status` (`platform`, `status`, `deleted_at`, `id`),
  KEY `idx_ai_context_spaces_profile_status` (`profile_id`, `status`, `deleted_at`, `id`),
  KEY `idx_ai_context_spaces_created_by` (`created_by`),
  CONSTRAINT `fk_ai_context_spaces_profile`
    FOREIGN KEY (`profile_id`) REFERENCES `ai_context_profiles` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_spaces_created_by`
    FOREIGN KEY (`created_by`) REFERENCES `users` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_spaces_platform`
    CHECK (`platform` REGEXP _ascii'^[a-z][a-z0-9_]{1,48}$'
      AND `platform` NOT IN (_ascii'app', _ascii'canvas', _ascii'all')),
  CONSTRAINT `chk_ai_context_spaces_status`
    CHECK (`status` IN (_ascii'enabled', _ascii'disabled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Add the active-Version foreign key only after Version owns the composite parent key.
CREATE TABLE `ai_context_documents` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `space_id` BIGINT UNSIGNED NULL,
  `conversation_id` INT UNSIGNED NULL,
  `source_message_id` BIGINT UNSIGNED NULL,
  `source_attachment_index` INT UNSIGNED NULL,
  `title` VARCHAR(512) NOT NULL,
  `active_version_id` BIGINT UNSIGNED NULL,
  `status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `created_by` INT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_documents_conversation_attachment`
    (`conversation_id`, `source_message_id`, `source_attachment_index`),
  KEY `idx_ai_context_documents_space_status` (`space_id`, `status`, `deleted_at`, `id`),
  KEY `idx_ai_context_documents_conversation_status`
    (`conversation_id`, `status`, `deleted_at`, `id`),
  KEY `idx_ai_context_documents_source_message` (`source_message_id`),
  KEY `idx_ai_context_documents_active_owner` (`id`, `active_version_id`),
  KEY `idx_ai_context_documents_created_by` (`created_by`),
  CONSTRAINT `fk_ai_context_documents_space`
    FOREIGN KEY (`space_id`) REFERENCES `ai_context_spaces` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_conversation`
    FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_source_message`
    FOREIGN KEY (`source_message_id`) REFERENCES `ai_messages` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_documents_created_by`
    FOREIGN KEY (`created_by`) REFERENCES `users` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_documents_owner_source`
    CHECK (
      (`space_id` IS NOT NULL AND `conversation_id` IS NULL
        AND `source_message_id` IS NULL AND `source_attachment_index` IS NULL)
      OR (`space_id` IS NULL AND `conversation_id` IS NOT NULL
        AND `source_message_id` IS NOT NULL AND `source_attachment_index` IS NOT NULL)
    ),
  CONSTRAINT `chk_ai_context_documents_status`
    CHECK (`status` IN (_ascii'enabled', _ascii'disabled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `ai_context_document_versions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `document_id` BIGINT UNSIGNED NOT NULL,
  `profile_id` BIGINT UNSIGNED NOT NULL,
  `source_storage_provider` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_object_key` VARCHAR(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `source_etag` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `source_size_bytes` BIGINT UNSIGNED NOT NULL,
  `source_mime_type` VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_filename` VARCHAR(512) NOT NULL,
  `source_facts_sha256` BINARY(32) NOT NULL,
  `source_sha256` BINARY(32) NULL,
  `parser_name` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `parser_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `chunker_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `failure_stage` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `error_code` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `error_message` VARCHAR(1024) NULL,
  `chunk_count` INT UNSIGNED NOT NULL DEFAULT 0,
  `embedding_input_token_upper_bound` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `embedding_request_count` INT UNSIGNED NOT NULL DEFAULT 0,
  `embedding_input_tokens` BIGINT UNSIGNED NULL,
  `started_at` DATETIME(6) NULL,
  `finished_at` DATETIME(6) NULL,
  `attempt_count` INT UNSIGNED NOT NULL DEFAULT 0,
  `lease_token` BIGINT UNSIGNED NULL,
  `lease_expires_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_document_versions_document_id` (`document_id`, `id`),
  KEY `idx_ai_context_document_versions_document_created` (`document_id`, `created_at`, `id`),
  KEY `idx_ai_context_document_versions_profile_state` (`profile_id`, `state`, `id`),
  KEY `idx_ai_context_document_versions_lease` (`state`, `lease_expires_at`, `id`),
  CONSTRAINT `fk_ai_context_document_versions_document`
    FOREIGN KEY (`document_id`) REFERENCES `ai_context_documents` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_document_versions_profile`
    FOREIGN KEY (`profile_id`) REFERENCES `ai_context_profiles` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_document_versions_source`
    CHECK (`source_size_bytes` > 0
      AND CHAR_LENGTH(`source_object_key`) > 0
      AND CHAR_LENGTH(`source_etag`) > 0
      AND CHAR_LENGTH(`source_mime_type`) > 0
      AND CHAR_LENGTH(`source_filename`) > 0),
  CONSTRAINT `chk_ai_context_document_versions_state`
    CHECK (`state` IN (_ascii'queued', _ascii'processing', _ascii'ready', _ascii'failed')),
  CONSTRAINT `chk_ai_context_document_versions_lease_pair`
    CHECK ((`lease_token` IS NULL AND `lease_expires_at` IS NULL)
      OR (`lease_token` IS NOT NULL AND `lease_expires_at` IS NOT NULL)),
  CONSTRAINT `chk_ai_context_document_versions_terminal_shape`
    CHECK (
      (`state` = _ascii'queued' AND `source_sha256` IS NULL
        AND `failure_stage` IS NULL AND `error_code` IS NULL AND `error_message` IS NULL
        AND `started_at` IS NULL AND `finished_at` IS NULL
        AND `lease_token` IS NULL AND `lease_expires_at` IS NULL)
      OR (`state` = _ascii'processing' AND `failure_stage` IS NULL
        AND `error_code` IS NULL AND `error_message` IS NULL
        AND `started_at` IS NOT NULL AND `finished_at` IS NULL
        AND `attempt_count` > 0 AND `lease_token` IS NOT NULL
        AND `lease_expires_at` IS NOT NULL)
      OR (`state` = _ascii'ready' AND `source_sha256` IS NOT NULL
        AND `failure_stage` IS NULL AND `error_code` IS NULL AND `error_message` IS NULL
        AND `chunk_count` > 0 AND `embedding_input_token_upper_bound` > 0
        AND `embedding_request_count` > 0 AND `started_at` IS NOT NULL
        AND `finished_at` IS NOT NULL AND `attempt_count` > 0
        AND `lease_token` IS NULL AND `lease_expires_at` IS NULL)
      OR (`state` = _ascii'failed' AND `failure_stage` IS NOT NULL
        AND CHAR_LENGTH(`failure_stage`) > 0 AND `error_code` IS NOT NULL
        AND CHAR_LENGTH(`error_code`) > 0
        AND (`error_message` IS NULL OR CHAR_LENGTH(`error_message`) > 0)
        AND `started_at` IS NOT NULL AND `finished_at` IS NOT NULL
        AND `attempt_count` > 0 AND `lease_token` IS NULL
        AND `lease_expires_at` IS NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE `ai_context_documents`
  ADD CONSTRAINT `fk_ai_context_documents_active_version`
    FOREIGN KEY (`id`, `active_version_id`)
    REFERENCES `ai_context_document_versions` (`document_id`, `id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

CREATE TABLE `ai_context_chunks` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `document_version_id` BIGINT UNSIGNED NOT NULL,
  `ordinal` INT UNSIGNED NOT NULL,
  `heading_path` TEXT NOT NULL,
  `content` LONGTEXT NOT NULL,
  `content_sha256` BINARY(32) NOT NULL,
  `chunk_facts_sha256` BINARY(32) NOT NULL,
  `embedding_input_token_upper_bound` BIGINT UNSIGNED NOT NULL,
  `locator_json` JSON NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_chunks_version_ordinal` (`document_version_id`, `ordinal`),
  KEY `idx_ai_context_chunks_version_id` (`document_version_id`, `id`),
  CONSTRAINT `fk_ai_context_chunks_version`
    FOREIGN KEY (`document_version_id`) REFERENCES `ai_context_document_versions` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_chunks_content`
    CHECK (OCTET_LENGTH(`content`) > 0 AND `embedding_input_token_upper_bound` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `ai_context_bindings` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `agent_id` BIGINT UNSIGNED NOT NULL,
  `space_id` BIGINT UNSIGNED NOT NULL,
  `status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_bindings_agent_space` (`agent_id`, `space_id`),
  KEY `idx_ai_context_bindings_agent_status` (`agent_id`, `status`, `id`),
  KEY `idx_ai_context_bindings_space_status` (`space_id`, `status`, `id`),
  CONSTRAINT `fk_ai_context_bindings_agent`
    FOREIGN KEY (`agent_id`) REFERENCES `ai_agents` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_bindings_space`
    FOREIGN KEY (`space_id`) REFERENCES `ai_context_spaces` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_bindings_status`
    CHECK (`status` IN (_ascii'enabled', _ascii'disabled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `ai_context_plans` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `run_id` BIGINT UNSIGNED NOT NULL,
  `context_profile_id_snapshot` BIGINT UNSIGNED NULL,
  `context_profile_sha256` BINARY(32) NULL,
  `context_index_generation_snapshot` BIGINT UNSIGNED NULL,
  `policy_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `input_fingerprint_sha256` BINARY(32) NOT NULL,
  `plan_sha256` BINARY(32) NULL,
  `model_capability_sha256` BINARY(32) NOT NULL,
  `api_protocol_snapshot` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `token_counter_id_snapshot` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `context_window_tokens` BIGINT UNSIGNED NOT NULL,
  `effective_output_tokens` BIGINT UNSIGNED NOT NULL,
  `provider_protocol_upper_bound` BIGINT UNSIGNED NOT NULL,
  `tool_continuation_input_reserve` BIGINT UNSIGNED NOT NULL,
  `policy_safety_margin` BIGINT UNSIGNED NOT NULL,
  `known_input_budget` BIGINT UNSIGNED NOT NULL,
  `known_input_upper_bound` BIGINT UNSIGNED NOT NULL,
  `budget_proof` VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `retrieval_outcome` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `error_stage` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `error_code` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `error_message` VARCHAR(1024) NULL,
  `metrics_json` JSON NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_plans_run` (`run_id`),
  KEY `idx_ai_context_plans_profile_generation`
    (`context_profile_id_snapshot`, `context_index_generation_snapshot`, `id`),
  CONSTRAINT `fk_ai_context_plans_run`
    FOREIGN KEY (`run_id`) REFERENCES `ai_runs` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_context_plans_profile`
    FOREIGN KEY (`context_profile_id_snapshot`) REFERENCES `ai_context_profiles` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_plans_profile_snapshot`
    CHECK ((`context_profile_id_snapshot` IS NULL
      AND `context_profile_sha256` IS NULL
      AND `context_index_generation_snapshot` IS NULL)
      OR (`context_profile_id_snapshot` IS NOT NULL
        AND `context_profile_sha256` IS NOT NULL
        AND (`context_index_generation_snapshot` IS NULL
          OR `context_index_generation_snapshot` > 0))),
  CONSTRAINT `chk_ai_context_plans_api_protocol`
    CHECK (`api_protocol_snapshot` IN (_ascii'chat_completions', _ascii'responses')),
  CONSTRAINT `chk_ai_context_plans_budget_proof`
    CHECK (`budget_proof` IN (_ascii'exact', _ascii'conservative', _ascii'opaque_attachment')),
  CONSTRAINT `chk_ai_context_plans_retrieval_outcome`
    CHECK (`retrieval_outcome` IN (_ascii'skipped', _ascii'no_hit', _ascii'hit', _ascii'failed')),
  CONSTRAINT `chk_ai_context_plans_state`
    CHECK (`state` IN (_ascii'ready', _ascii'failed')),
  CONSTRAINT `chk_ai_context_plans_terminal_shape`
    CHECK (
      (`state` = _ascii'ready' AND `plan_sha256` IS NOT NULL
        AND `retrieval_outcome` IN (_ascii'skipped', _ascii'no_hit', _ascii'hit')
        AND `error_stage` IS NULL AND `error_code` IS NULL AND `error_message` IS NULL)
      OR (`state` = _ascii'failed' AND `plan_sha256` IS NULL
        AND `retrieval_outcome` = _ascii'failed'
        AND `error_stage` IS NOT NULL AND CHAR_LENGTH(`error_stage`) > 0
        AND `error_code` IS NOT NULL AND CHAR_LENGTH(`error_code`) > 0
        AND (`error_message` IS NULL OR CHAR_LENGTH(`error_message`) > 0))
    ),
  CONSTRAINT `chk_ai_context_plans_budget`
    CHECK (`context_window_tokens` > 0 AND `effective_output_tokens` > 0
      AND `known_input_budget` + `effective_output_tokens`
        + `provider_protocol_upper_bound` + `policy_safety_margin`
        = `context_window_tokens`
      AND `tool_continuation_input_reserve` <= `provider_protocol_upper_bound`
      AND `known_input_upper_bound` <= `known_input_budget`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `ai_context_plan_items` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `plan_id` BIGINT UNSIGNED NOT NULL,
  `ordinal` INT UNSIGNED NOT NULL,
  `block_kind` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_type` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `source_ref` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  `source_sha256` BINARY(32) NOT NULL,
  `atomic_group_key` VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `required` TINYINT UNSIGNED NOT NULL,
  `priority` INT NOT NULL,
  `decision` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `exclusion_reason` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `token_upper_bound` BIGINT UNSIGNED NOT NULL,
  `fusion_score` DECIMAL(20,6) NULL,
  `rerank_score` DECIMAL(20,6) NULL,
  `citation_key` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `content_snapshot` LONGTEXT NULL,
  `metadata_json` JSON NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_context_plan_items_plan_ordinal` (`plan_id`, `ordinal`),
  UNIQUE KEY `uk_ai_context_plan_items_plan_citation` (`plan_id`, `citation_key`),
  KEY `idx_ai_context_plan_items_plan_decision` (`plan_id`, `decision`, `ordinal`),
  CONSTRAINT `fk_ai_context_plan_items_plan`
    FOREIGN KEY (`plan_id`) REFERENCES `ai_context_plans` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_context_plan_items_block_kind`
    CHECK (`block_kind` IN (
      _ascii'system_instruction', _ascii'current_user_message', _ascii'current_attachment',
      _ascii'recent_turn', _ascii'recalled_turn', _ascii'history_attachment',
      _ascii'conversation_memory', _ascii'document_evidence', _ascii'tool_definition',
      _ascii'tool_call', _ascii'tool_result'
    )),
  CONSTRAINT `chk_ai_context_plan_items_required`
    CHECK (`required` IN (0, 1)),
  CONSTRAINT `chk_ai_context_plan_items_decision`
    CHECK (
      (`decision` = _ascii'selected' AND `exclusion_reason` IS NULL)
      OR (`decision` = _ascii'excluded' AND `exclusion_reason` IS NOT NULL
        AND `exclusion_reason` IN (
        _ascii'budget_exceeded', _ascii'duplicate_content', _ascii'below_relevance_threshold',
        _ascii'superseded_memory', _ascii'inactive_source', _ascii'permission_changed',
        _ascii'unsupported_attachment'
      ))
    ),
  CONSTRAINT `chk_ai_context_plan_items_citation`
    CHECK (`citation_key` IS NULL
      OR (`decision` = _ascii'selected' AND `block_kind` = _ascii'document_evidence'
        AND `citation_key` REGEXP _ascii'^C[1-9][0-9]*$')),
  CONSTRAINT `chk_ai_context_plan_items_content_snapshot`
    CHECK (
      (`decision` = _ascii'excluded' AND `content_snapshot` IS NULL)
      OR (`decision` = _ascii'selected'
        AND `block_kind` IN (_ascii'current_attachment', _ascii'history_attachment')
        AND `content_snapshot` IS NULL)
      OR (`decision` = _ascii'selected'
        AND `block_kind` NOT IN (_ascii'current_attachment', _ascii'history_attachment')
        AND `content_snapshot` IS NOT NULL
        AND OCTET_LENGTH(`content_snapshot`) > 0)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `ai_conversation_memories` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `conversation_id` INT UNSIGNED NOT NULL,
  `context_profile_id_snapshot` BIGINT UNSIGNED NOT NULL,
  `context_profile_sha256` BINARY(32) NOT NULL,
  `previous_memory_id` BIGINT UNSIGNED NULL,
  `from_message_id` BIGINT UNSIGNED NOT NULL,
  `through_message_id` BIGINT UNSIGNED NOT NULL,
  `source_sha256` BINARY(32) NOT NULL,
  `summary_sha256` BINARY(32) NULL,
  `policy_version` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `summary` MEDIUMTEXT NULL,
  `prompt_tokens` BIGINT UNSIGNED NULL,
  `completion_tokens` BIGINT UNSIGNED NULL,
  `provider_request_id` VARCHAR(191) NULL,
  `state` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `error_code` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ai_conversation_memories_identity`
    (`conversation_id`, `context_profile_id_snapshot`, `through_message_id`, `source_sha256`),
  KEY `idx_ai_conversation_memories_latest_ready`
    (`conversation_id`, `context_profile_id_snapshot`, `state`, `through_message_id`, `id`),
  KEY `idx_ai_conversation_memories_previous` (`previous_memory_id`),
  KEY `idx_ai_conversation_memories_from_message` (`from_message_id`),
  KEY `idx_ai_conversation_memories_through_message` (`through_message_id`),
  CONSTRAINT `fk_ai_conversation_memories_conversation`
    FOREIGN KEY (`conversation_id`) REFERENCES `ai_conversations` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_profile`
    FOREIGN KEY (`context_profile_id_snapshot`) REFERENCES `ai_context_profiles` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_previous`
    FOREIGN KEY (`previous_memory_id`) REFERENCES `ai_conversation_memories` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_from_message`
    FOREIGN KEY (`from_message_id`) REFERENCES `ai_messages` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_ai_conversation_memories_through_message`
    FOREIGN KEY (`through_message_id`) REFERENCES `ai_messages` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_ai_conversation_memories_interval`
    CHECK (`from_message_id` <= `through_message_id`),
  CONSTRAINT `chk_ai_conversation_memories_usage_pair`
    CHECK ((`prompt_tokens` IS NULL AND `completion_tokens` IS NULL)
      OR (`prompt_tokens` IS NOT NULL AND `completion_tokens` IS NOT NULL)),
  CONSTRAINT `chk_ai_conversation_memories_state`
    CHECK (`state` IN (_ascii'ready', _ascii'failed', _ascii'invalidated')),
  CONSTRAINT `chk_ai_conversation_memories_terminal_shape`
    CHECK (
      (`state` = _ascii'ready' AND `summary` IS NOT NULL AND OCTET_LENGTH(`summary`) > 0
        AND `summary_sha256` IS NOT NULL AND `error_code` IS NULL)
      OR (`state` = _ascii'failed' AND `summary` IS NULL AND `summary_sha256` IS NULL
        AND `error_code` IS NOT NULL AND CHAR_LENGTH(`error_code`) > 0)
      OR (`state` = _ascii'invalidated' AND `summary` IS NOT NULL
        AND OCTET_LENGTH(`summary`) > 0 AND `summary_sha256` IS NOT NULL
        AND `error_code` IS NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE `ai_provider_models`
  ADD COLUMN `model_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin
    NOT NULL DEFAULT 'chat' AFTER `model_id`,
  DROP INDEX `uk_ai_provider_models_provider_model`,
  ADD UNIQUE KEY `uk_ai_provider_models_provider_model_kind`
    (`provider_id`, `model_id`, `model_kind`),
  ADD CONSTRAINT `chk_ai_provider_models_model_kind`
    CHECK (`model_kind` IN (_ascii'chat', _ascii'embedding', _ascii'rerank'));

ALTER TABLE `ai_agents`
  ADD COLUMN `context_profile_id` BIGINT UNSIGNED NULL,
  ADD KEY `idx_ai_agents_context_profile` (`context_profile_id`),
  ADD CONSTRAINT `fk_ai_agents_context_profile`
    FOREIGN KEY (`context_profile_id`) REFERENCES `ai_context_profiles` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE `ai_provider_attempts`
  ADD COLUMN `context_plan_id` BIGINT UNSIGNED NULL,
  ADD COLUMN `context_plan_sha256` BINARY(32) NULL,
  ADD KEY `idx_ai_provider_attempts_context_plan` (`context_plan_id`),
  ADD CONSTRAINT `fk_ai_provider_attempts_context_plan`
    FOREIGN KEY (`context_plan_id`) REFERENCES `ai_context_plans` (`id`)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `chk_ai_provider_attempts_context_plan_pair`
    CHECK ((`context_plan_id` IS NULL AND `context_plan_sha256` IS NULL)
      OR (`context_plan_id` IS NOT NULL AND `context_plan_sha256` IS NOT NULL));
