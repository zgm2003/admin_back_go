table "address" {
  schema  = schema.admin
  comment = "区域表"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    comment        = "Region id"
    auto_increment = true
  }
  column "parent_id" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "parent region id; 0 means root"
  }
  column "code" {
    null    = true
    type    = varchar(255)
    comment = "区划编码"
  }
  column "name" {
    null    = true
    type    = varchar(255)
    comment = "区划名称"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "soft delete: 1 deleted 2 normal"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "Created time"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "Updated time"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_address_parent" {
    columns = [column.parent_id]
  }
  index "uniq_address_code" {
    unique  = true
    columns = [column.code]
  }
}
table "ai_agent_tools" {
  schema  = schema.admin
  comment = "AI智能体工具绑定"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "绑定ID"
    auto_increment = true
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_agents.id"
  }
  column "tool_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_tools.id"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1启用 2禁用；运行时只加载启用绑定"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_agent_tools_agent" {
    columns     = [column.agent_id]
    ref_columns = [table.ai_agents.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  foreign_key "fk_ai_agent_tools_tool" {
    columns     = [column.tool_id]
    ref_columns = [table.ai_tools.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  index "idx_ai_agent_tools_agent_status" {
    columns = [column.agent_id, column.status, column.id]
  }
  index "idx_ai_agent_tools_tool_status" {
    columns = [column.tool_id, column.status, column.id]
  }
  index "uk_ai_agent_tools_agent_tool" {
    unique  = true
    columns = [column.agent_id, column.tool_id]
  }
  check "chk_ai_agent_tools_status" {
    expr = "(`status` in (1,2))"
  }
}
table "ai_agents" {
  schema  = schema.admin
  comment = "AI agent mappings"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "provider_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "provider_model_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "name" {
    null = false
    type = varchar(128)
  }
  column "model_id" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "model_display_name" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "billing_multiplier_ppm" {
    null     = false
    type     = bigint
    unsigned = true
    default  = 1000000
  }
  column "scenes_json" {
    null = true
    type = json
  }
  column "system_prompt" {
    null = true
    type = text
  }
  column "avatar" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  column "context_profile_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_agents_context_profile" {
    columns     = [column.context_profile_id]
    ref_columns = [table.ai_context_profiles.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_agents_provider_model" {
    columns     = [column.provider_model_id, column.provider_id, column.model_id]
    ref_columns = [table.ai_provider_models.column.id, table.ai_provider_models.column.provider_id, table.ai_provider_models.column.model_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_agents_context_profile" {
    columns = [column.context_profile_id]
  }
  index "idx_ai_agents_model" {
    columns = [column.provider_id, column.model_id, column.status, column.is_del]
  }
  index "idx_ai_agents_provider_model_identity" {
    columns = [column.provider_model_id, column.provider_id, column.model_id]
  }
  index "idx_ai_agents_provider" {
    columns = [column.provider_id, column.status, column.is_del]
  }
}
table "ai_assets" {
  schema  = schema.admin
  comment = "AI素材库"
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "user_id" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "slug" {
    null = false
    type = varchar(191)
  }
  column "type" {
    null = false
    type = varchar(16)
  }
  column "category" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "title" {
    null = false
    type = varchar(191)
  }
  column "cover_url" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "description" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "content" {
    null = true
    type = text
  }
  column "url" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "tags_json" {
    null = true
    type = json
  }
  column "status" {
    null    = false
    type    = tinyint
    default = 1
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_ai_assets_status_updated" {
    columns = [column.status, column.is_del, column.updated_at, column.id]
  }
  index "idx_ai_assets_type_status" {
    columns = [column.type, column.status, column.is_del, column.updated_at, column.id]
  }
  index "idx_ai_assets_user_status_updated" {
    columns = [column.user_id, column.status, column.is_del, column.updated_at, column.id]
  }
  index "uk_ai_assets_user_slug" {
    unique  = true
    columns = [column.user_id, column.slug]
  }
}
table "ai_context_profiles" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "name" {
    null = false
    type = varchar(191)
  }
  column "embedding_provider_model_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "embedding_dimensions" {
    null     = false
    type     = int
    unsigned = true
  }
  column "embedding_max_input_tokens" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "embedding_token_counter_id" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "dense_distance" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "dense_min_score" {
    null = false
    type = decimal(20,6)
  }
  column "sparse_encoder" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "sparse_encoder_version" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "reranker_provider_model_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "reranker_min_score" {
    null = true
    type = decimal(20,6)
  }
  column "memory_provider_model_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "status" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "active_index_generation" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "target_index_generation" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "index_state" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "index_error_code" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "index_verified_at" {
    null = true
    type = datetime(6)
  }
  column "created_by" {
    null     = false
    type     = int
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_profiles_embedding_model" {
    columns     = [column.embedding_provider_model_id]
    ref_columns = [table.ai_provider_models.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_profiles_reranker_model" {
    columns     = [column.reranker_provider_model_id]
    ref_columns = [table.ai_provider_models.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_profiles_memory_model" {
    columns     = [column.memory_provider_model_id]
    ref_columns = [table.ai_provider_models.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_profiles_created_by" {
    columns     = [column.created_by]
    ref_columns = [table.users.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_context_profiles_status_state" {
    columns = [column.status, column.index_state, column.id]
  }
  index "idx_ai_context_profiles_embedding_model" {
    columns = [column.embedding_provider_model_id]
  }
  index "idx_ai_context_profiles_reranker_model" {
    columns = [column.reranker_provider_model_id]
  }
  index "idx_ai_context_profiles_memory_model" {
    columns = [column.memory_provider_model_id]
  }
  index "idx_ai_context_profiles_created_by" {
    columns = [column.created_by]
  }
  check "chk_ai_context_profiles_embedding_shape" {
    expr = "((`embedding_dimensions` > 0) and (`embedding_max_input_tokens` > 0))"
  }
  check "chk_ai_context_profiles_dense_distance" {
    expr = "(`dense_distance` in (_ascii'cosine',_ascii'dot',_ascii'euclid'))"
  }
  check "chk_ai_context_profiles_sparse_encoder" {
    expr = "(`sparse_encoder` = _ascii'unicode_lexical_v1')"
  }
  check "chk_ai_context_profiles_reranker_pair" {
    expr = "(((`reranker_provider_model_id` is null) and (`reranker_min_score` is null)) or ((`reranker_provider_model_id` is not null) and (`reranker_min_score` is not null)))"
  }
  check "chk_ai_context_profiles_status" {
    expr = "(`status` in (_ascii'enabled',_ascii'retired'))"
  }
  check "chk_ai_context_profiles_index_state" {
    expr = "(`index_state` in (_ascii'provisioning',_ascii'ready',_ascii'rebuilding',_ascii'failed'))"
  }
  check "chk_ai_context_profiles_generation_shape" {
    expr = "(((`index_state` = _ascii'provisioning') and (`active_index_generation` is null) and (`target_index_generation` is not null)) or ((`index_state` = _ascii'ready') and (`active_index_generation` is not null) and (`target_index_generation` is null)) or ((`index_state` = _ascii'rebuilding') and (`target_index_generation` is not null)) or (`index_state` = _ascii'failed'))"
  }
  check "chk_ai_context_profiles_generation_order" {
    expr = "(((`active_index_generation` is null) or (`active_index_generation` > 0)) and ((`target_index_generation` is null) or (`target_index_generation` > 0)) and ((`active_index_generation` is null) or (`target_index_generation` is null) or (`target_index_generation` > `active_index_generation`)))"
  }
  check "chk_ai_context_profiles_index_error" {
    expr = "((`index_state` <> _ascii'failed') or ((`index_error_code` is not null) and (char_length(`index_error_code`) > 0)))"
  }
}
table "ai_context_spaces" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "platform" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "profile_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "name" {
    null = false
    type = varchar(191)
  }
  column "description" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "status" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "deleted_at" {
    null = true
    type = datetime(6)
  }
  column "created_by" {
    null     = false
    type     = int
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_spaces_profile" {
    columns     = [column.profile_id]
    ref_columns = [table.ai_context_profiles.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_spaces_created_by" {
    columns     = [column.created_by]
    ref_columns = [table.users.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_context_spaces_platform_status" {
    columns = [column.platform, column.status, column.deleted_at, column.id]
  }
  index "idx_ai_context_spaces_profile_status" {
    columns = [column.profile_id, column.status, column.deleted_at, column.id]
  }
  index "idx_ai_context_spaces_created_by" {
    columns = [column.created_by]
  }
  check "chk_ai_context_spaces_platform" {
    expr = "((`platform` regexp _ascii'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_ascii'app',_ascii'canvas',_ascii'all')))"
  }
  check "chk_ai_context_spaces_status" {
    expr = "(`status` in (_ascii'enabled',_ascii'disabled'))"
  }
}
table "ai_context_documents" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "space_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "conversation_id" {
    null     = true
    type     = int
    unsigned = true
  }
  column "source_message_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "source_attachment_index" {
    null     = true
    type     = int
    unsigned = true
  }
  column "title" {
    null = false
    type = varchar(512)
  }
  column "active_version_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "status" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "deleted_at" {
    null = true
    type = datetime(6)
  }
  column "created_by" {
    null     = false
    type     = int
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_documents_space" {
    columns     = [column.space_id]
    ref_columns = [table.ai_context_spaces.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_documents_conversation" {
    columns     = [column.conversation_id]
    ref_columns = [table.ai_conversations.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_documents_source_message_owner" {
    columns     = [column.source_message_id, column.conversation_id]
    ref_columns = [table.ai_messages.column.id, table.ai_messages.column.conversation_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_documents_created_by" {
    columns     = [column.created_by]
    ref_columns = [table.users.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_documents_active_version" {
    columns     = [column.id, column.active_version_id]
    ref_columns = [table.ai_context_document_versions.column.document_id, table.ai_context_document_versions.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_context_documents_conversation_attachment" {
    unique  = true
    columns = [column.conversation_id, column.source_message_id, column.source_attachment_index]
  }
  index "idx_ai_context_documents_space_status" {
    columns = [column.space_id, column.status, column.deleted_at, column.id]
  }
  index "idx_ai_context_documents_conversation_status" {
    columns = [column.conversation_id, column.status, column.deleted_at, column.id]
  }
  index "idx_ai_context_documents_source_message" {
    columns = [column.source_message_id]
  }
  index "idx_ai_context_documents_source_message_owner" {
    columns = [column.source_message_id, column.conversation_id]
  }
  index "idx_ai_context_documents_active_owner" {
    columns = [column.id, column.active_version_id]
  }
  index "idx_ai_context_documents_created_by" {
    columns = [column.created_by]
  }
  check "chk_ai_context_documents_owner_source" {
    expr = "(((`space_id` is not null) and (`conversation_id` is null) and (`source_message_id` is null) and (`source_attachment_index` is null)) or ((`space_id` is null) and (`conversation_id` is not null) and (`source_message_id` is not null) and (`source_attachment_index` is not null)))"
  }
  check "chk_ai_context_documents_status" {
    expr = "(`status` in (_ascii'enabled',_ascii'disabled'))"
  }
}
table "ai_context_document_versions" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "document_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "profile_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "source_storage_provider" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "source_object_key" {
    null    = false
    type    = varchar(1024)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
  }
  column "source_etag" {
    null    = false
    type    = varchar(191)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
  }
  column "source_size_bytes" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "source_mime_type" {
    null    = false
    type    = varchar(191)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "source_filename" {
    null = false
    type = varchar(512)
  }
  column "source_facts_sha256" {
    null = false
    type = binary(32)
  }
  column "source_sha256" {
    null = true
    type = binary(32)
  }
  column "parser_name" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "parser_version" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "chunker_version" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "state" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "failure_stage" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "error_code" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "error_message" {
    null = true
    type = varchar(1024)
  }
  column "chunk_count" {
    null     = false
    type     = int
    unsigned = true
    default  = 0
  }
  column "embedding_input_token_upper_bound" {
    null     = false
    type     = bigint
    unsigned = true
    default  = 0
  }
  column "embedding_request_count" {
    null     = false
    type     = int
    unsigned = true
    default  = 0
  }
  column "embedding_input_tokens" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "started_at" {
    null = true
    type = datetime(6)
  }
  column "finished_at" {
    null = true
    type = datetime(6)
  }
  column "attempt_count" {
    null     = false
    type     = int
    unsigned = true
    default  = 0
  }
  column "lease_token" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "lease_expires_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_document_versions_document" {
    columns     = [column.document_id]
    ref_columns = [table.ai_context_documents.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_document_versions_profile" {
    columns     = [column.profile_id]
    ref_columns = [table.ai_context_profiles.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_context_document_versions_document_id" {
    unique  = true
    columns = [column.document_id, column.id]
  }
  index "idx_ai_context_document_versions_document_created" {
    columns = [column.document_id, column.created_at, column.id]
  }
  index "idx_ai_context_document_versions_profile_state" {
    columns = [column.profile_id, column.state, column.id]
  }
  index "idx_ai_context_document_versions_lease" {
    columns = [column.state, column.lease_expires_at, column.id]
  }
  check "chk_ai_context_document_versions_source" {
    expr = "((`source_size_bytes` > 0) and (char_length(`source_object_key`) > 0) and (char_length(`source_etag`) > 0) and (char_length(`source_mime_type`) > 0) and (char_length(`source_filename`) > 0))"
  }
  check "chk_ai_context_document_versions_state" {
    expr = "(`state` in (_ascii'queued',_ascii'processing',_ascii'ready',_ascii'failed'))"
  }
  check "chk_ai_context_document_versions_lease_pair" {
    expr = "(((`lease_token` is null) and (`lease_expires_at` is null)) or ((`lease_token` is not null) and (`lease_expires_at` is not null)))"
  }
  check "chk_ai_context_document_versions_terminal_shape" {
    expr = "(((`state` = _ascii'queued') and (`source_sha256` is null) and (`failure_stage` is null) and (`error_code` is null) and (`error_message` is null) and (`started_at` is null) and (`finished_at` is null) and (`lease_token` is null) and (`lease_expires_at` is null)) or ((`state` = _ascii'processing') and (`failure_stage` is null) and (`error_code` is null) and (`error_message` is null) and (`started_at` is not null) and (`finished_at` is null) and (`attempt_count` > 0) and (`lease_token` is not null) and (`lease_expires_at` is not null)) or ((`state` = _ascii'ready') and (`source_sha256` is not null) and (`failure_stage` is null) and (`error_code` is null) and (`error_message` is null) and (`chunk_count` > 0) and (`embedding_input_token_upper_bound` > 0) and (`embedding_request_count` > 0) and (`started_at` is not null) and (`finished_at` is not null) and (`attempt_count` > 0) and (`lease_token` is null) and (`lease_expires_at` is null)) or ((`state` = _ascii'failed') and (`failure_stage` is not null) and (char_length(`failure_stage`) > 0) and (`error_code` is not null) and (char_length(`error_code`) > 0) and ((`error_message` is null) or (char_length(`error_message`) > 0)) and (`started_at` is not null) and (`finished_at` is not null) and (`attempt_count` > 0) and (`lease_token` is null) and (`lease_expires_at` is null)))"
  }
}
table "ai_context_chunks" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "document_version_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "ordinal" {
    null     = false
    type     = int
    unsigned = true
  }
  column "heading_path" {
    null = false
    type = text
  }
  column "content" {
    null = false
    type = longtext
  }
  column "content_sha256" {
    null = false
    type = binary(32)
  }
  column "chunk_facts_sha256" {
    null = false
    type = binary(32)
  }
  column "embedding_input_token_upper_bound" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "locator_json" {
    null = false
    type = json
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_chunks_version" {
    columns     = [column.document_version_id]
    ref_columns = [table.ai_context_document_versions.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_context_chunks_version_ordinal" {
    unique  = true
    columns = [column.document_version_id, column.ordinal]
  }
  index "idx_ai_context_chunks_version_id" {
    columns = [column.document_version_id, column.id]
  }
  check "chk_ai_context_chunks_content" {
    expr = "((octet_length(`content`) > 0) and (`embedding_input_token_upper_bound` > 0))"
  }
}
table "ai_context_bindings" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "space_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "status" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_bindings_agent" {
    columns     = [column.agent_id]
    ref_columns = [table.ai_agents.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_bindings_space" {
    columns     = [column.space_id]
    ref_columns = [table.ai_context_spaces.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_context_bindings_agent_space" {
    unique  = true
    columns = [column.agent_id, column.space_id]
  }
  index "idx_ai_context_bindings_agent_status" {
    columns = [column.agent_id, column.status, column.id]
  }
  index "idx_ai_context_bindings_space_status" {
    columns = [column.space_id, column.status, column.id]
  }
  check "chk_ai_context_bindings_status" {
    expr = "(`status` in (_ascii'enabled',_ascii'disabled'))"
  }
}
table "ai_context_plans" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "context_profile_id_snapshot" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "context_profile_sha256" {
    null = true
    type = binary(32)
  }
  column "context_index_generation_snapshot" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "policy_version" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "input_fingerprint_sha256" {
    null = false
    type = binary(32)
  }
  column "plan_sha256" {
    null = true
    type = binary(32)
  }
  column "model_capability_sha256" {
    null = false
    type = binary(32)
  }
  column "api_protocol_snapshot" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "token_counter_id_snapshot" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "context_window_tokens" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "effective_output_tokens" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "provider_protocol_upper_bound" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "tool_continuation_input_reserve" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "policy_safety_margin" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "known_input_budget" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "known_input_upper_bound" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "budget_proof" {
    null    = false
    type    = varchar(24)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "retrieval_outcome" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "state" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "error_stage" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "error_code" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "error_message" {
    null = true
    type = varchar(1024)
  }
  column "metrics_json" {
    null = false
    type = json
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_plans_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_context_plans_profile" {
    columns     = [column.context_profile_id_snapshot]
    ref_columns = [table.ai_context_profiles.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_context_plans_run" {
    unique  = true
    columns = [column.run_id]
  }
  index "uk_ai_context_plans_id_run" {
    unique  = true
    columns = [column.id, column.run_id]
  }
  index "idx_ai_context_plans_profile_generation" {
    columns = [column.context_profile_id_snapshot, column.context_index_generation_snapshot, column.id]
  }
  check "chk_ai_context_plans_profile_snapshot" {
    expr = "(((`context_profile_id_snapshot` is null) and (`context_profile_sha256` is null) and (`context_index_generation_snapshot` is null)) or ((`context_profile_id_snapshot` is not null) and (`context_profile_sha256` is not null) and ((`context_index_generation_snapshot` is null) or (`context_index_generation_snapshot` > 0))))"
  }
  check "chk_ai_context_plans_api_protocol" {
    expr = "(`api_protocol_snapshot` in (_ascii'chat_completions',_ascii'responses'))"
  }
  check "chk_ai_context_plans_budget_proof" {
    expr = "(`budget_proof` in (_ascii'exact',_ascii'conservative',_ascii'opaque_attachment'))"
  }
  check "chk_ai_context_plans_retrieval_outcome" {
    expr = "(`retrieval_outcome` in (_ascii'skipped',_ascii'no_hit',_ascii'hit',_ascii'failed'))"
  }
  check "chk_ai_context_plans_state" {
    expr = "(`state` in (_ascii'ready',_ascii'failed'))"
  }
  check "chk_ai_context_plans_terminal_shape" {
    expr = "(((`state` = _ascii'ready') and (`plan_sha256` is not null) and (`retrieval_outcome` in (_ascii'skipped',_ascii'no_hit',_ascii'hit')) and (`error_stage` is null) and (`error_code` is null) and (`error_message` is null)) or ((`state` = _ascii'failed') and (`plan_sha256` is null) and (`retrieval_outcome` = _ascii'failed') and (`error_stage` is not null) and (char_length(`error_stage`) > 0) and (`error_code` is not null) and (char_length(`error_code`) > 0) and ((`error_message` is null) or (char_length(`error_message`) > 0))))"
  }
  check "chk_ai_context_plans_budget" {
    expr = "((`context_window_tokens` > 0) and (`effective_output_tokens` > 0) and ((((`known_input_budget` + `effective_output_tokens`) + `provider_protocol_upper_bound`) + `policy_safety_margin`) = `context_window_tokens`) and (`tool_continuation_input_reserve` <= `provider_protocol_upper_bound`) and (`known_input_upper_bound` <= `known_input_budget`))"
  }
}
table "ai_context_plan_items" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "plan_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "ordinal" {
    null     = false
    type     = int
    unsigned = true
  }
  column "block_kind" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "source_type" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "source_ref" {
    null    = false
    type    = varchar(512)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
  }
  column "source_sha256" {
    null = false
    type = binary(32)
  }
  column "atomic_group_key" {
    null    = false
    type    = varchar(191)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "required" {
    null     = false
    type     = tinyint
    unsigned = true
  }
  column "priority" {
    null = false
    type = int
  }
  column "decision" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "exclusion_reason" {
    null    = true
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "token_upper_bound" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "fusion_score" {
    null = true
    type = decimal(20,6)
  }
  column "rerank_score" {
    null = true
    type = decimal(20,6)
  }
  column "citation_key" {
    null    = true
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "content_snapshot" {
    null = true
    type = longtext
  }
  column "metadata_json" {
    null = false
    type = json
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_context_plan_items_plan" {
    columns     = [column.plan_id]
    ref_columns = [table.ai_context_plans.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_context_plan_items_plan_ordinal" {
    unique  = true
    columns = [column.plan_id, column.ordinal]
  }
  index "uk_ai_context_plan_items_plan_citation" {
    unique  = true
    columns = [column.plan_id, column.citation_key]
  }
  index "idx_ai_context_plan_items_plan_decision" {
    columns = [column.plan_id, column.decision, column.ordinal]
  }
  check "chk_ai_context_plan_items_block_kind" {
    expr = "(`block_kind` in (_ascii'system_instruction',_ascii'current_user_message',_ascii'current_attachment',_ascii'recent_turn',_ascii'recalled_turn',_ascii'history_attachment',_ascii'conversation_memory',_ascii'document_evidence',_ascii'tool_definition',_ascii'tool_call',_ascii'tool_result'))"
  }
  check "chk_ai_context_plan_items_required" {
    expr = "(`required` in (0,1))"
  }
  check "chk_ai_context_plan_items_decision" {
    expr = "(((`decision` = _ascii'selected') and (`exclusion_reason` is null)) or ((`decision` = _ascii'excluded') and (`exclusion_reason` is not null) and (`exclusion_reason` in (_ascii'budget_exceeded',_ascii'duplicate_content',_ascii'below_relevance_threshold',_ascii'superseded_memory',_ascii'inactive_source',_ascii'permission_changed',_ascii'unsupported_attachment'))))"
  }
  check "chk_ai_context_plan_items_citation" {
    expr = "((`citation_key` is null) or ((`decision` = _ascii'selected') and (`block_kind` = _ascii'document_evidence') and (`citation_key` regexp _ascii'^C[1-9][0-9]*$')))"
  }
  check "chk_ai_context_plan_items_content_snapshot" {
    expr = "(((`decision` = _ascii'excluded') and (`content_snapshot` is null)) or ((`decision` = _ascii'selected') and (`block_kind` in (_ascii'current_attachment',_ascii'history_attachment')) and (`content_snapshot` is null)) or ((`decision` = _ascii'selected') and (`block_kind` not in (_ascii'current_attachment',_ascii'history_attachment')) and (`content_snapshot` is not null) and (octet_length(`content_snapshot`) > 0)))"
  }
}
table "ai_conversation_memories" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "conversation_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "context_profile_id_snapshot" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "context_profile_sha256" {
    null = false
    type = binary(32)
  }
  column "previous_memory_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "from_message_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "through_message_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "source_sha256" {
    null = false
    type = binary(32)
  }
  column "summary_sha256" {
    null = true
    type = binary(32)
  }
  column "policy_version" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "summary" {
    null = true
    type = mediumtext
  }
  column "prompt_tokens" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "completion_tokens" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "provider_request_id" {
    null = true
    type = varchar(191)
  }
  column "state" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "error_code" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_conversation_memories_conversation" {
    columns     = [column.conversation_id]
    ref_columns = [table.ai_conversations.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_conversation_memories_profile" {
    columns     = [column.context_profile_id_snapshot]
    ref_columns = [table.ai_context_profiles.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_conversation_memories_previous_owner" {
    columns     = [column.previous_memory_id, column.conversation_id, column.context_profile_id_snapshot]
    ref_columns = [table.ai_conversation_memories.column.id, table.ai_conversation_memories.column.conversation_id, table.ai_conversation_memories.column.context_profile_id_snapshot]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_conversation_memories_from_message_owner" {
    columns     = [column.from_message_id, column.conversation_id]
    ref_columns = [table.ai_messages.column.id, table.ai_messages.column.conversation_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_conversation_memories_through_message_owner" {
    columns     = [column.through_message_id, column.conversation_id]
    ref_columns = [table.ai_messages.column.id, table.ai_messages.column.conversation_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_conversation_memories_owner" {
    unique  = true
    columns = [column.id, column.conversation_id, column.context_profile_id_snapshot]
  }
  index "uk_ai_conversation_memories_identity" {
    unique  = true
    columns = [column.conversation_id, column.context_profile_id_snapshot, column.through_message_id, column.source_sha256]
  }
  index "idx_ai_conversation_memories_latest_ready" {
    columns = [column.conversation_id, column.context_profile_id_snapshot, column.state, column.through_message_id, column.id]
  }
  index "idx_ai_conversation_memories_previous" {
    columns = [column.previous_memory_id]
  }
  index "idx_ai_conversation_memories_previous_owner" {
    columns = [column.previous_memory_id, column.conversation_id, column.context_profile_id_snapshot]
  }
  index "idx_ai_conversation_memories_from_message" {
    columns = [column.from_message_id]
  }
  index "idx_ai_conversation_memories_from_message_owner" {
    columns = [column.from_message_id, column.conversation_id]
  }
  index "idx_ai_conversation_memories_through_message" {
    columns = [column.through_message_id]
  }
  index "idx_ai_conversation_memories_through_message_owner" {
    columns = [column.through_message_id, column.conversation_id]
  }
  check "chk_ai_conversation_memories_interval" {
    expr = "(`from_message_id` <= `through_message_id`)"
  }
  check "chk_ai_conversation_memories_usage_pair" {
    expr = "(((`prompt_tokens` is null) and (`completion_tokens` is null)) or ((`prompt_tokens` is not null) and (`completion_tokens` is not null)))"
  }
  check "chk_ai_conversation_memories_state" {
    expr = "(`state` in (_ascii'ready',_ascii'failed',_ascii'invalidated'))"
  }
  check "chk_ai_conversation_memories_terminal_shape" {
    expr = "(((`state` = _ascii'ready') and (`summary` is not null) and (octet_length(`summary`) > 0) and (`summary_sha256` is not null) and (`error_code` is null)) or ((`state` = _ascii'failed') and (`summary` is null) and (`summary_sha256` is null) and (`error_code` is not null) and (char_length(`error_code`) > 0)) or ((`state` = _ascii'invalidated') and (`summary` is not null) and (octet_length(`summary`) > 0) and (`summary_sha256` is not null) and (`error_code` is null)))"
  }
}
table "ai_conversations" {
  schema  = schema.admin
  comment = "AI会话"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    comment        = "会话ID"
    auto_increment = true
  }
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "当前用户ID"
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_agents.id"
  }
  column "title" {
    null    = false
    type    = varchar(100)
    default = ""
    comment = "会话标题"
  }
  column "last_message_at" {
    null    = true
    type    = datetime
    comment = "上次对话时间"
  }
  column "last_read_message_id" {
    null     = false
    type     = bigint
    unsigned = true
    default  = 0
    comment  = "当前用户已读消息游标"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1删除 2正常"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_conversations_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_conversations_agent" {
    columns     = [column.agent_id]
    ref_columns = [table.ai_agents.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_conversations_id_user" {
    unique  = true
    columns = [column.id, column.user_id]
  }
  index "idx_ai_conversations_agent" {
    columns = [column.agent_id]
  }
  index "idx_ai_conversations_user_agent_del_last_message" {
    columns = [column.user_id, column.agent_id, column.is_del, column.last_message_at, column.id]
  }
}
table "ai_image_files" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "task_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "role" {
    null    = false
    type    = varchar(16)
    comment = "input/mask/output"
  }
  column "sort_order" {
    null    = false
    type    = int
    default = 0
  }
  column "storage_provider" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "storage_key" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "storage_url" {
    null    = false
    type    = varchar(1000)
    default = ""
  }
  column "mime_type" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "width" {
    null    = false
    type    = int
    default = 0
  }
  column "height" {
    null    = false
    type    = int
    default = 0
  }
  column "size_bytes" {
    null    = false
    type    = bigint
    default = 0
  }
  column "related_file_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "revised_prompt" {
    null = true
    type = text
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_image_files_task" {
    columns     = [column.task_id]
    ref_columns = [table.ai_image_tasks.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_image_files_related" {
    columns     = [column.related_file_id]
    ref_columns = [table.ai_image_files.column.id]
    on_update   = RESTRICT
    on_delete   = SET_NULL
  }
  index "idx_ai_image_files_related" {
    columns = [column.related_file_id]
  }
  index "idx_ai_image_files_task_role_sort" {
    columns = [column.task_id, column.role, column.sort_order]
  }
}
table "ai_image_tasks" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "user_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "request_id" {
    null    = false
    type    = varchar(128)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
  }
  column "request_fingerprint" {
    null = false
    type = binary(32)
  }
  column "request_identity_status" {
    null    = false
    type    = varchar(24)
    default = "replayable"
  }
  column "request_identity_marker" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "agent_name_snapshot" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "provider_id_snapshot" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "provider_name_snapshot" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "model_id_snapshot" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "model_display_name_snapshot" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "prompt" {
    null = false
    type = text
  }
  column "size" {
    null    = false
    type    = varchar(32)
    default = "1024x1024"
  }
  column "quality" {
    null    = false
    type    = varchar(16)
    default = "auto"
  }
  column "output_format" {
    null    = false
    type    = varchar(16)
    default = "png"
  }
  column "output_compression" {
    null = true
    type = int
  }
  column "moderation" {
    null    = false
    type    = varchar(16)
    default = "auto"
  }
  column "n" {
    null    = false
    type    = int
    default = 1
  }
  column "status" {
    null    = false
    type    = varchar(16)
    default = "pending"
  }
  column "lease_owner" {
    null = true
    type = varchar(128)
  }
  column "lease_token" {
    null    = false
    type    = bigint
    unsigned = true
    default = 0
  }
  column "lease_expires_at" {
    null = true
    type = datetime(6)
  }
  column "error_message" {
    null    = false
    type    = varchar(1000)
    default = ""
  }
  column "last_error_code" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "actual_params_json" {
    null = true
    type = json
  }
  column "raw_response_json" {
    null = true
    type = json
  }
  column "is_favorite" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "finished_at" {
    null = true
    type = datetime
  }
  column "elapsed_ms" {
    null    = false
    type    = int
    default = 0
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_image_tasks_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_image_tasks_agent_created" {
    columns = [column.agent_id, column.created_at]
  }
  index "idx_ai_image_tasks_platform_status_created" {
    columns = [column.platform, column.status, column.created_at]
  }
  index "idx_ai_image_tasks_platform_user_created" {
    columns = [column.platform, column.user_id, column.created_at]
  }
  index "uk_ai_image_tasks_run" {
    unique  = true
    columns = [column.run_id]
  }
  index "idx_ai_image_tasks_lease" {
    columns = [column.status, column.lease_expires_at, column.id]
  }
  index "uk_ai_image_tasks_user_request" {
    unique  = true
    columns = [column.user_id, column.request_id]
  }
  check "chk_ai_image_tasks_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_image_tasks_request_identity" {
    expr = "(((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))"
  }
  check "chk_ai_image_tasks_lease" {
    expr = "(((`lease_owner` is null) and (`lease_expires_at` is null)) or ((`lease_owner` is not null) and (`lease_token` > 0) and (`lease_expires_at` is not null)))"
  }
}
table "ai_messages" {
  schema  = schema.admin
  comment = "AI消息"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "消息ID"
    auto_increment = true
  }
  column "conversation_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "ai_conversations.id"
  }
  column "role" {
    null     = false
    type     = tinyint
    unsigned = true
    comment  = "1用户 2助手"
  }
  column "content_type" {
    null    = false
    type    = varchar(32)
    default = "text"
    comment = "内容类型，MVP只写text"
  }
  column "content" {
    null    = false
    type    = longtext
    comment = "消息内容"
  }
  column "meta_json" {
    null    = true
    type    = json
    comment = "消息扩展元数据：attachments/runtime_params/blocks/feedback"
  }
  column "reply_command_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "delivery_state" {
    null = true
    type = varchar(16)
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1删除 2正常"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_messages_conversation" {
    columns     = [column.conversation_id]
    ref_columns = [table.ai_conversations.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  index "idx_ai_messages_conversation_del_id" {
    columns = [column.conversation_id, column.is_del, column.id]
  }
  index "idx_ai_messages_conversation_del_role_id" {
    columns = [column.conversation_id, column.is_del, column.role, column.id]
  }
  index "uk_ai_messages_reply_command" {
    unique  = true
    columns = [column.reply_command_id]
  }
  index "uk_ai_messages_id_conversation" {
    unique  = true
    columns = [column.id, column.conversation_id]
  }
  check "chk_ai_messages_delivery_state" {
    expr = "(((`role` = 2) and (`delivery_state` in (_utf8mb4'completed',_utf8mb4'stopped'))) or ((`role` <> 2) and (`delivery_state` is null)))"
  }
}
table "ai_prompts" {
  schema  = schema.admin
  comment = "AI提示词库"
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "slug" {
    null = false
    type = varchar(191)
  }
  column "category" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "title" {
    null = false
    type = varchar(191)
  }
  column "cover_url" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "prompt" {
    null = false
    type = text
  }
  column "preview" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "tags_json" {
    null = true
    type = json
  }
  column "source_url" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "status" {
    null    = false
    type    = tinyint
    default = 1
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_ai_prompts_category_status" {
    columns = [column.category, column.status, column.is_del, column.updated_at, column.id]
  }
  index "idx_ai_prompts_status_updated" {
    columns = [column.status, column.is_del, column.updated_at, column.id]
  }
  index "uk_ai_prompts_slug" {
    unique  = true
    columns = [column.slug]
  }
}
table "ai_official_model_price_overrides" {
  schema  = schema.admin
  comment = "Current canonical AI model price overrides"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "catalog_vendor" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "model_id" {
    null    = false
    type    = varchar(191)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "version" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "source_url" {
    null = false
    type = varchar(2048)
  }
  column "verified_at" {
    null = false
    type = date
  }
  column "updated_by" {
    null     = false
    type     = int
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  index "uk_ai_official_model_price_overrides_identity" {
    unique  = true
    columns = [column.catalog_vendor, column.model_id]
  }
  check "chk_ai_official_model_price_overrides_version" {
    expr = "(`version` > 0)"
  }
}
table "ai_official_model_price_override_rates" {
  schema  = schema.admin
  comment = "Complete rate set for an AI model price override"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "override_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "category" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "unit" {
    null    = false
    type    = varchar(32)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "tier_key" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
    default = ""
  }
  column "price_units" {
    null = false
    type = bigint
  }
  column "unit_scale" {
    null     = false
    type     = bigint
    unsigned = true
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_official_model_price_override_rates_override" {
    columns     = [column.override_id]
    ref_columns = [table.ai_official_model_price_overrides.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  index "uk_ai_official_model_price_override_rates_key" {
    unique  = true
    columns = [column.override_id, column.category, column.unit, column.tier_key]
  }
  check "chk_ai_official_model_price_override_rates_category" {
    expr = "(`category` in (_ascii'input',_ascii'output',_ascii'cache_read',_ascii'cache_write',_ascii'media'))"
  }
  check "chk_ai_official_model_price_override_rates_unit" {
    expr = "(char_length(trim(`unit`)) > 0)"
  }
  check "chk_ai_official_model_price_override_rates_price" {
    expr = "(`price_units` >= 0)"
  }
  check "chk_ai_official_model_price_override_rates_scale" {
    expr = "(`unit_scale` > 0)"
  }
}
table "ai_provider_attempts" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "command_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "attempt_no" {
    null     = false
    type     = int
    unsigned = true
  }
  column "idempotency_key" {
    null = false
    type = varchar(128)
  }
  column "state" {
    null = false
    type = varchar(24)
  }
  column "prepared_request_json" {
    null = false
    type = mediumtext
  }
  column "prepared_request_sha256" {
    null = false
    type = binary(32)
  }
  column "quote_json" {
    null = false
    type = mediumtext
  }
  column "usage_json" {
    null = false
    type = mediumtext
  }
  column "usage_status" {
    null    = false
    type    = varchar(16)
    default = "unavailable"
  }
  column "dispatch_state" {
    null    = false
    type    = varchar(16)
    default = "not_dispatched"
  }
  column "result_candidate_json" {
    null = true
    type = mediumtext
  }
  column "provider_request_id" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "response_sha256" {
    null    = false
    type    = char(64)
    default = ""
  }
  column "error_code" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "prepare_started_at" {
    null = true
    type = datetime(6)
  }
  column "dispatched_at" {
    null = true
    type = datetime(6)
  }
  column "first_delta_at" {
    null = true
    type = datetime(6)
  }
  column "finished_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  column "context_plan_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "context_plan_sha256" {
    null = true
    type = binary(32)
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_provider_attempts_context_plan_run" {
    columns     = [column.context_plan_id, column.run_id]
    ref_columns = [table.ai_context_plans.column.id, table.ai_context_plans.column.run_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_provider_attempts_command_run" {
    columns     = [column.command_id, column.run_id]
    ref_columns = [table.ai_reply_commands.column.id, table.ai_reply_commands.column.run_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_provider_attempts_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_attempt_state" {
    columns = [column.state, column.id]
  }
  index "idx_ai_attempt_command" {
    columns = [column.command_id, column.attempt_no]
  }
  index "idx_ai_provider_attempts_error_run" {
    columns = [column.error_code, column.run_id, column.id]
  }
  index "idx_ai_provider_attempts_context_plan" {
    columns = [column.context_plan_id]
  }
  index "idx_ai_provider_attempts_context_plan_run" {
    columns = [column.context_plan_id, column.run_id]
  }
  index "idx_ai_provider_attempts_command_run" {
    columns = [column.command_id, column.run_id]
  }
  index "uk_ai_attempt_run_no" {
    unique  = true
    columns = [column.run_id, column.attempt_no]
  }
  index "uk_ai_attempt_key" {
    unique  = true
    columns = [column.idempotency_key]
  }
  check "chk_ai_provider_attempts_state" {
    expr = "(`state` in (_utf8mb4'prepared',_utf8mb4'dispatched',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'outcome_unknown'))"
  }
  check "chk_ai_provider_attempts_usage_status" {
    expr = "(`usage_status` in (_utf8mb4'complete',_utf8mb4'unavailable'))"
  }
  check "chk_ai_provider_attempts_dispatch_state" {
    expr = "(`dispatch_state` in (_utf8mb4'not_dispatched',_utf8mb4'dispatched',_utf8mb4'unknown'))"
  }
  check "chk_ai_provider_attempts_context_plan_pair" {
    expr = "(((`context_plan_id` is null) and (`context_plan_sha256` is null)) or ((`context_plan_id` is not null) and (`context_plan_sha256` is not null)))"
  }
}
table "ai_provider_models" {
  schema  = schema.admin
  comment = "AI provider enabled model catalog"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "provider_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "model_id" {
    null = false
    type = varchar(191)
  }
  column "model_kind" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "display_name" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "official_model_id" {
    null    = true
    type    = varchar(191)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "official_catalog_version" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "mapping_status" {
    null    = false
    type    = varchar(16)
    charset = "ascii"
    collate = "ascii_bin"
    default = "unmapped"
  }
  column "mapped_at" {
    null = true
    type = datetime(6)
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_provider_models_provider" {
    columns     = [column.provider_id]
    ref_columns = [table.ai_providers.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_provider_models_provider_status" {
    columns = [column.provider_id, column.status]
  }
  index "idx_ai_provider_models_official_mapping" {
    columns = [column.mapping_status, column.official_model_id, column.status]
  }
  index "uk_ai_provider_models_provider_model_kind" {
    unique  = true
    columns = [column.provider_id, column.model_id, column.model_kind]
  }
  index "uk_ai_provider_models_id_provider_model" {
    unique  = true
    columns = [column.id, column.provider_id, column.model_id]
  }
  check "chk_ai_provider_models_mapping_status" {
    expr = "(`mapping_status` in (_ascii'mapped',_ascii'unmapped'))"
  }
  check "chk_ai_provider_models_mapping" {
    expr = "(((`mapping_status` = _ascii'mapped') and (`official_model_id` is not null) and (`official_catalog_version` is not null) and (`mapped_at` is not null)) or ((`mapping_status` = _ascii'unmapped') and (`official_model_id` is null) and (`official_catalog_version` is null) and (`mapped_at` is null)))"
  }
  check "chk_ai_provider_models_model_kind" {
    expr = "(`model_kind` in (_ascii'chat',_ascii'embedding',_ascii'rerank'))"
  }
}
table "ai_providers" {
  schema  = schema.admin
  comment = "AI engine connection configs"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "name" {
    null = false
    type = varchar(128)
  }
  column "engine_type" {
    null = false
    type = varchar(32)
  }
  column "base_url" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "api_protocol" {
    null    = false
    type    = varchar(32)
    default = "chat_completions"
  }
  column "api_key_enc" {
    null = true
    type = text
  }
  column "api_key_hint" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "health_status" {
    null    = false
    type    = varchar(32)
    default = "unknown"
  }
  column "last_checked_at" {
    null = true
    type = datetime
  }
  column "last_check_error" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "last_model_sync_at" {
    null = true
    type = datetime
  }
  column "last_model_sync_status" {
    null    = false
    type    = varchar(32)
    default = "unknown"
  }
  column "last_model_sync_error" {
    null    = false
    type    = varchar(1024)
    default = ""
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_ai_providers_status" {
    columns = [column.status, column.is_del]
  }
  index "uk_ai_providers_type_name" {
    unique  = true
    columns = [column.engine_type, column.name, column.is_del]
  }
  check "chk_ai_providers_api_protocol" {
    expr = "(`api_protocol` in (_ascii'chat_completions',_ascii'responses'))"
  }
}
table "ai_reply_commands" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "request_id" {
    null    = false
    type    = varchar(128)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
  }
  column "request_fingerprint" {
    null = false
    type = binary(32)
  }
  column "request_identity_status" {
    null    = false
    type    = varchar(24)
    default = "replayable"
  }
  column "request_identity_marker" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "idempotency_key" {
    null = false
    type = varchar(128)
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "conversation_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "user_message_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "assistant_message_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "request_received_at" {
    null = true
    type = datetime(6)
  }
  column "accepted_at" {
    null = true
    type = datetime(6)
  }
  column "claimed_at" {
    null = true
    type = datetime(6)
  }
  column "claim_source" {
    null    = false
    type    = varchar(16)
    default = ""
  }
  column "state" {
    null    = false
    type    = varchar(32)
    default = "pending"
  }
  column "attempt_count" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
  }
  column "max_attempts" {
    null     = false
    type     = int
    default  = 3
    unsigned = true
  }
  column "lease_owner" {
    null = true
    type = varchar(128)
  }
  column "lease_token" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "lease_expires_at" {
    null = true
    type = datetime(6)
  }
  column "next_attempt_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "cancel_requested_at" {
    null = true
    type = datetime(6)
  }
  column "delivery_seq" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
  }
  column "stop_delivery_seq" {
    null     = true
    type     = int
    unsigned = true
  }
  column "outcome_unknown_at" {
    null = true
    type = datetime(6)
  }
  column "last_error_code" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "last_error_message" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "started_at" {
    null = true
    type = datetime(6)
  }
  column "finished_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_reply_commands_run_owner" {
    columns     = [column.run_id, column.user_id, column.conversation_id, column.user_message_id, column.request_id]
    ref_columns = [table.ai_runs.column.id, table.ai_runs.column.user_id, table.ai_runs.column.conversation_id, table.ai_runs.column.user_message_id, table.ai_runs.column.request_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_reply_commands_conversation_owner" {
    columns     = [column.conversation_id, column.user_id]
    ref_columns = [table.ai_conversations.column.id, table.ai_conversations.column.user_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_reply_commands_user_message_owner" {
    columns     = [column.user_message_id, column.conversation_id]
    ref_columns = [table.ai_messages.column.id, table.ai_messages.column.conversation_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_reply_commands_assistant_message_owner" {
    columns     = [column.assistant_message_id, column.conversation_id]
    ref_columns = [table.ai_messages.column.id, table.ai_messages.column.conversation_id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_reply_claim" {
    columns = [column.state, column.next_attempt_at, column.lease_expires_at, column.id]
  }
  index "idx_ai_reply_commands_run_owner" {
    columns = [column.run_id, column.user_id, column.conversation_id, column.user_message_id, column.request_id]
  }
  index "idx_ai_reply_commands_conversation_owner" {
    columns = [column.conversation_id, column.user_id]
  }
  index "idx_ai_reply_commands_user_message_owner" {
    columns = [column.user_message_id, column.conversation_id]
  }
  index "idx_ai_reply_commands_assistant_message_owner" {
    columns = [column.assistant_message_id, column.conversation_id]
  }
  index "uk_ai_reply_commands_run" {
    unique  = true
    columns = [column.run_id]
  }
  index "uk_ai_reply_commands_id_run" {
    unique  = true
    columns = [column.id, column.run_id]
  }
  index "uk_ai_reply_idempotency" {
    unique  = true
    columns = [column.idempotency_key]
  }
  index "uk_ai_reply_message" {
    unique  = true
    columns = [column.user_message_id]
  }
  index "uk_ai_reply_user_request" {
    unique  = true
    columns = [column.user_id, column.request_id]
  }
  check "chk_ai_reply_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_reply_state" {
    expr = "(`state` in (_utf8mb4'pending',_utf8mb4'claimed',_utf8mb4'running',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'outcome_unknown',_utf8mb4'timed_out'))"
  }
  check "chk_ai_reply_claim_source" {
    expr = "(`claim_source` in (_utf8mb4'',_utf8mb4'wake',_utf8mb4'poll',_utf8mb4'recovery'))"
  }
  check "chk_ai_reply_request_identity" {
    expr = "(((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))"
  }
  check "chk_ai_reply_delivery_seq" {
    expr = "(((`cancel_requested_at` is null) and (`stop_delivery_seq` is null)) or ((`cancel_requested_at` is not null) and (`stop_delivery_seq` is not null) and (`stop_delivery_seq` <= `delivery_seq`)))"
  }
}
table "ai_reply_delivery_chunks" {
  schema  = schema.admin
  charset = "utf8mb4"
  collate = "utf8mb4_0900_ai_ci"
  column "command_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "delivery_seq" {
    null     = false
    type     = int
    unsigned = true
  }
  column "delta" {
    null = false
    type = text
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.command_id, column.delivery_seq]
  }
  foreign_key "fk_ai_reply_delivery_chunks_command" {
    columns     = [column.command_id]
    ref_columns = [table.ai_reply_commands.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  check "chk_ai_reply_delivery_chunk_seq" {
    expr = "(`delivery_seq` > 0)"
  }
  check "chk_ai_reply_delivery_chunk_size" {
    expr = "((octet_length(`delta`) > 0) and (octet_length(`delta`) <= 16384))"
  }
}
table "ai_run_events" {
  schema  = schema.admin
  comment = "AI运行监控事件"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "事件ID"
    auto_increment = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_runs.id"
  }
  column "seq" {
    null     = false
    type     = int
    unsigned = true
    comment  = "同一run内事件序号"
  }
  column "event_type" {
    null    = false
    type    = varchar(32)
    comment = "durable Run and billing event type"
  }
  column "message" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "事件说明或错误原因"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "事件时间"
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_run_events_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  index "idx_ai_run_events_run_id" {
    columns = [column.run_id, column.id]
  }
  index "idx_ai_run_events_type_created" {
    columns = [column.event_type, column.created_at, column.id]
  }
  index "uk_ai_run_events_run_seq" {
    unique  = true
    columns = [column.run_id, column.seq]
  }
  check "chk_ai_run_events_type" {
    expr = "(`event_type` in (_utf8mb4'start',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'retry_scheduled',_utf8mb4'usage_recorded',_utf8mb4'outcome_unknown',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unbilled',_utf8mb4'file_materialized_v1'))"
  }
}
table "ai_runs" {
  schema  = schema.admin
  comment = "AI运行监控记录"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "运行ID"
    auto_increment = true
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "conversation_id" {
    null     = true
    type     = int
    unsigned = true
    comment  = "ai_conversations.id; chat rows only"
  }
  column "request_id" {
    null    = false
    type    = varchar(128)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
    comment = "client request identifier"
  }
  column "request_fingerprint" {
    null    = false
    type    = binary(32)
    comment = "SHA-256 of canonical typed request identity"
  }
  column "request_identity_status" {
    null    = false
    type    = varchar(24)
    default = "replayable"
    comment = "replayable or validated legacy_non_replayable"
  }
  column "request_identity_marker" {
    null    = false
    type    = varchar(64)
    default = ""
    comment = "stable legacy identity marker; never a canonical replay tuple"
  }
  column "user_message_id" {
    null     = true
    type     = bigint
    unsigned = true
    comment  = "本轮用户消息ID; chat rows only"
  }
  column "assistant_message_id" {
    null     = true
    type     = bigint
    unsigned = true
    comment  = "完成后写入的助手消息ID; chat rows only"
  }
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "发起用户ID"
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_agents.id"
  }
  column "provider_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_providers.id"
  }
  column "model_id" {
    null    = false
    type    = varchar(191)
    comment = "实际调用模型ID"
  }
  column "model_display_name" {
    null    = false
    type    = varchar(191)
    default = ""
    comment = "实际调用模型展示名"
  }
  column "input_snapshot" {
    null = false
    type = mediumtext
  }
  column "pricing_snapshot_json" {
    null    = false
    type    = mediumtext
    comment = "immutable Run acceptance pricing configuration"
  }
  column "idempotency_key" {
    null = true
    type = varchar(128)
  }
  column "status" {
    null    = false
    type    = varchar(16)
    comment = "running/success/failed/canceled/timeout/outcome_unknown"
  }
  column "billing_status" {
    null    = false
    type    = varchar(16)
    comment = "pending/held/settled/released/unbilled"
  }
  column "billing_reason" {
    null    = false
    type    = varchar(32)
    comment = "stable billing terminal or progress reason"
  }
  column "prompt_tokens" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "输入token"
  }
  column "completion_tokens" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "输出token"
  }
  column "total_tokens" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "总token"
  }
  column "duration_ms" {
    null     = true
    type     = int
    unsigned = true
    comment  = "运行耗时毫秒，终态后写入"
  }
  column "error_message" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "失败/取消/超时原因"
  }
  column "started_at" {
    null    = true
    type    = datetime
    comment = "开始调用模型时间"
  }
  column "finished_at" {
    null    = true
    type    = datetime
    comment = "进入终态时间"
  }
  column "settled_at" {
    null    = true
    type    = datetime(6)
    comment = "终态结算时间"
  }
  column "liked_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_runs_assistant_message" {
    columns     = [column.assistant_message_id]
    ref_columns = [table.ai_messages.column.id]
    on_update   = RESTRICT
    on_delete   = SET_NULL
  }
  foreign_key "fk_ai_runs_conversation" {
    columns     = [column.conversation_id]
    ref_columns = [table.ai_conversations.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  foreign_key "fk_ai_runs_user_message" {
    columns     = [column.user_message_id]
    ref_columns = [table.ai_messages.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  index "fk_ai_runs_assistant_message" {
    columns = [column.assistant_message_id]
  }
  index "idx_ai_runs_agent_created" {
    columns = [column.agent_id, column.created_at, column.id]
  }
  index "idx_ai_runs_billing_created" {
    columns = [column.billing_status, column.billing_reason, column.created_at, column.id]
  }
  index "idx_ai_runs_conversation_created" {
    columns = [column.conversation_id, column.created_at, column.id]
  }
  index "idx_ai_runs_created" {
    columns = [column.created_at, column.id]
  }
  index "idx_ai_runs_model_created" {
    columns = [column.model_id, column.created_at, column.id]
  }
  index "idx_ai_runs_provider_created" {
    columns = [column.provider_id, column.created_at, column.id]
  }
  index "idx_ai_runs_status_created" {
    columns = [column.status, column.created_at, column.id]
  }
  index "idx_ai_runs_status_started" {
    columns = [column.status, column.started_at, column.id]
  }
  index "idx_ai_runs_user_created" {
    columns = [column.user_id, column.created_at, column.id]
  }
  index "uk_ai_runs_user_request" {
    unique  = true
    columns = [column.user_id, column.request_id]
  }
  index "uk_ai_runs_idempotency" {
    unique  = true
    columns = [column.idempotency_key]
  }
  index "uk_ai_runs_user_message" {
    unique  = true
    columns = [column.user_message_id]
  }
  index "uk_ai_runs_command_owner" {
    unique  = true
    columns = [column.id, column.user_id, column.conversation_id, column.user_message_id, column.request_id]
  }
  check "chk_ai_runs_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_runs_status" {
    expr = "(`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'outcome_unknown'))"
  }
  check "chk_ai_runs_billing_status" {
    expr = "(`billing_status` in (_utf8mb4'pending',_utf8mb4'held',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unbilled'))"
  }
  check "chk_ai_runs_billing_reason" {
    expr = "(`billing_reason` in (_utf8mb4'pending',_utf8mb4'held',_utf8mb4'settled_complete_usage',_utf8mb4'released_before_dispatch',_utf8mb4'released_insufficient_balance',_utf8mb4'released_provider_failed',_utf8mb4'released_outcome_unknown',_utf8mb4'unbilled_usage_incomplete',_utf8mb4'unbilled_over_hold',_utf8mb4'legacy_unpriced'))"
  }
  check "chk_ai_runs_request_identity" {
    expr = "(((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))"
  }
}
table "ai_text_tasks" {
  schema  = schema.admin
  comment = "AI文本生成任务"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "user_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "request_id" {
    null    = false
    type    = varchar(128)
    charset = "utf8mb4"
    collate = "utf8mb4_bin"
  }
  column "request_fingerprint" {
    null = false
    type = binary(32)
  }
  column "request_identity_status" {
    null    = false
    type    = varchar(24)
    default = "replayable"
  }
  column "request_identity_marker" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "kind" {
    null    = false
    type    = varchar(16)
    default = "text"
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "provider_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "model_id" {
    null = false
    type = varchar(191)
  }
  column "prompt" {
    null = false
    type = mediumtext
  }
  column "answer" {
    null = true
    type = mediumtext
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "error_message" {
    null = true
    type = varchar(1024)
  }
  column "last_error_code" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "started_at" {
    null = true
    type = datetime
  }
  column "finished_at" {
    null = true
    type = datetime
  }
  column "elapsed_ms" {
    null     = false
    type     = int
    unsigned = true
  }
  column "created_at" {
    null = false
    type = datetime
  }
  column "updated_at" {
    null = false
    type = datetime
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_text_tasks_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "uk_ai_text_tasks_run" {
    unique  = true
    columns = [column.run_id]
  }
  index "idx_ai_text_tasks_status_created" {
    columns = [column.status, column.created_at, column.id]
  }
  index "idx_ai_text_tasks_user_created" {
    columns = [column.user_id, column.created_at, column.id]
  }
  index "uk_ai_text_tasks_user_request" {
    unique  = true
    columns = [column.user_id, column.request_id]
  }
  check "chk_ai_text_tasks_kind" {
    expr = "(`kind` in (_utf8mb4'text',_utf8mb4'tool_draft'))"
  }
  check "chk_ai_text_tasks_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_text_tasks_status" {
    expr = "(`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed'))"
  }
  check "chk_ai_text_tasks_request_identity" {
    expr = "(((`request_identity_status` = _utf8mb4'replayable') and (`request_identity_marker` = _utf8mb4'')) or ((`request_identity_status` = _utf8mb4'legacy_non_replayable') and (`request_identity_marker` like _utf8mb4'legacy_non_replayable_v1:ai_runs:%')))"
  }
}
table "ai_tool_calls" {
  schema  = schema.admin
  comment = "AI工具调用记录"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "工具调用ID"
    auto_increment = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_runs.id"
  }
  column "tool_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_tools.id"
  }
  column "tool_code" {
    null    = false
    type    = varchar(128)
    comment = "调用时工具编码快照"
  }
  column "tool_name" {
    null    = false
    type    = varchar(128)
    comment = "调用时工具名称快照"
  }
  column "call_id" {
    null    = true
    type    = varchar(128)
    comment = "模型返回的tool_call_id/call_id，用于回传工具结果"
  }
  column "status" {
    null    = false
    type    = varchar(16)
    comment = "running/success/failed/timeout"
  }
  column "arguments_json" {
    null    = false
    type    = json
    comment = "模型传入参数"
  }
  column "result_json" {
    null    = true
    type    = json
    comment = "工具返回结果"
  }
  column "error_message" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "失败或超时原因"
  }
  column "duration_ms" {
    null     = true
    type     = int
    unsigned = true
    comment  = "执行耗时毫秒，终态后写入"
  }
  column "started_at" {
    null    = false
    type    = datetime
    comment = "开始执行时间"
  }
  column "finished_at" {
    null    = true
    type    = datetime
    comment = "结束时间"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_tool_calls_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  foreign_key "fk_ai_tool_calls_tool" {
    columns     = [column.tool_id]
    ref_columns = [table.ai_tools.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_tool_calls_run_id" {
    columns = [column.run_id, column.id]
  }
  index "idx_ai_tool_calls_status_created" {
    columns = [column.status, column.created_at, column.id]
  }
  index "idx_ai_tool_calls_tool_created" {
    columns = [column.tool_id, column.created_at, column.id]
  }
  index "uk_ai_tool_calls_run_call" {
    unique  = true
    columns = [column.run_id, column.call_id]
  }
  check "chk_ai_tool_calls_status" {
    expr = "(`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed',_utf8mb4'timeout'))"
  }
}
table "ai_tools" {
  schema  = schema.admin
  comment = "AI工具定义"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "工具ID"
    auto_increment = true
  }
  column "name" {
    null    = false
    type    = varchar(128)
    comment = "工具名称，管理页和运行监控展示"
  }
  column "code" {
    null    = false
    type    = varchar(128)
    comment = "工具唯一编码，传给模型作为function name"
  }
  column "description" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "工具说明，传给模型作为function description"
  }
  column "parameters_json" {
    null    = false
    type    = json
    comment = "工具参数JSON Schema，传给模型并用于入参校验"
  }
  column "result_schema_json" {
    null    = false
    type    = json
    comment = "工具返回JSON Schema，用于结果校验和运行监控展示"
  }
  column "risk_level" {
    null    = false
    type    = varchar(16)
    comment = "风险等级：low/medium/high"
  }
  column "timeout_ms" {
    null     = false
    type     = int
    default  = 3000
    unsigned = true
    comment  = "执行超时毫秒，运行时context timeout"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1启用 2禁用"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1删除 2正常"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_ai_tools_status_del" {
    columns = [column.status, column.is_del, column.id]
  }
  index "uk_ai_tools_code" {
    unique  = true
    columns = [column.code]
  }
  check "chk_ai_tools_is_del" {
    expr = "(`is_del` in (1,2))"
  }
  check "chk_ai_tools_risk_level" {
    expr = "(`risk_level` in (_utf8mb4'low',_utf8mb4'medium',_utf8mb4'high'))"
  }
  check "chk_ai_tools_status" {
    expr = "(`status` in (1,2))"
  }
}
table "wallet_holds" {
  schema  = schema.admin
  comment = "Run-level wallet reservations"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "wallet_id" {
    null = false
    type = bigint
  }
  column "user_id" {
    null = false
    type = bigint
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "held_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "captured_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "status" {
    null    = false
    type    = varchar(16)
    default = "active"
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_wallet_holds_wallet" {
    columns     = [column.wallet_id]
    ref_columns = [table.user_wallets.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_wallet_holds_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_wallet_holds_wallet_status" {
    columns = [column.wallet_id, column.status]
  }
  index "uk_wallet_holds_run" {
    unique  = true
    columns = [column.run_id]
  }
  check "chk_wallet_holds_status" {
    expr = "(`status` in (_utf8mb4'active',_utf8mb4'captured',_utf8mb4'released'))"
  }
  check "chk_wallet_holds_units" {
    expr = "(((`status` = _utf8mb4'active') and (`held_units` > 0) and (`captured_units` = 0)) or ((`status` = _utf8mb4'captured') and (`held_units` = 0) and (`captured_units` >= 0)) or ((`status` = _utf8mb4'released') and (`held_units` = 0) and (`captured_units` = 0)))"
  }
}
table "ai_usage_charges" {
  schema  = schema.admin
  comment = "Immutable Run-level AI usage charges"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "user_id" {
    null = false
    type = bigint
  }
  column "currency" {
    null    = false
    type    = char(3)
    default = "CNY"
  }
  column "pricing_version" {
    null = false
    type = varchar(64)
  }
  column "multiplier_ppm" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "held_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "actual_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "status" {
    null    = false
    type    = varchar(16)
    default = "open"
  }
  column "finalized_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_usage_charges_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_usage_charges_user_created" {
    columns = [column.user_id, column.created_at, column.id]
  }
  index "uk_ai_usage_charges_run" {
    unique  = true
    columns = [column.run_id]
  }
  check "chk_ai_usage_charges_status" {
    expr = "(`status` in (_utf8mb4'open',_utf8mb4'settled',_utf8mb4'released',_utf8mb4'unbilled'))"
  }
  check "chk_ai_usage_charges_currency" {
    expr = "(`currency` = _utf8mb4'CNY')"
  }
  check "chk_ai_usage_charges_units" {
    expr = "((`held_units` >= 0) and (`actual_units` >= 0))"
  }
}
table "ai_usage_charge_items" {
  schema  = schema.admin
  comment = "Immutable categorized usage charge lines"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "charge_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "attempt_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "category" {
    null = false
    type = varchar(32)
  }
  column "tier_key" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "quantity" {
    null = false
    type = bigint
  }
  column "unit" {
    null = false
    type = varchar(32)
  }
  column "unit_price_units" {
    null = false
    type = bigint
  }
  column "unit_scale" {
    null = false
    type = bigint
  }
  column "amount_units" {
    null = false
    type = bigint
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_ai_usage_charge_items_charge" {
    columns     = [column.charge_id]
    ref_columns = [table.ai_usage_charges.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_ai_usage_charge_items_attempt" {
    columns     = [column.attempt_id]
    ref_columns = [table.ai_provider_attempts.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_usage_charge_items_attempt" {
    columns = [column.attempt_id]
  }
  index "uk_ai_usage_charge_item_identity" {
    unique  = true
    columns = [column.charge_id, column.attempt_id, column.category, column.tier_key, column.unit]
  }
  check "chk_ai_usage_charge_items_category" {
    expr = "(`category` in (_utf8mb4'input',_utf8mb4'output',_utf8mb4'cache_read',_utf8mb4'cache_write',_utf8mb4'media'))"
  }
  check "chk_ai_usage_charge_items_units" {
    expr = "((`quantity` > 0) and (`unit_price_units` >= 0) and (`unit_scale` > 0) and (`amount_units` >= 0))"
  }
}
table "atlas_schema_revisions" {
  schema  = schema.admin
  collate = "utf8mb4_bin"
  column "version" {
    null = false
    type = varchar(255)
  }
  column "description" {
    null = false
    type = varchar(255)
  }
  column "type" {
    null     = false
    type     = bigint
    default  = 2
    unsigned = true
  }
  column "applied" {
    null    = false
    type    = bigint
    default = 0
  }
  column "total" {
    null    = false
    type    = bigint
    default = 0
  }
  column "executed_at" {
    null = false
    type = timestamp
  }
  column "execution_time" {
    null = false
    type = bigint
  }
  column "error" {
    null = true
    type = longtext
  }
  column "error_stmt" {
    null = true
    type = longtext
  }
  column "hash" {
    null = false
    type = varchar(255)
  }
  column "partial_hashes" {
    null = true
    type = json
  }
  column "operator_version" {
    null = false
    type = varchar(255)
  }
  primary_key {
    columns = [column.version]
  }
}
table "auth_platforms" {
  schema  = schema.admin
  comment = "认证平台管理"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "code" {
    null    = false
    type    = varchar(50)
    comment = "平台标识（如 admin, app）"
  }
  column "name" {
    null    = false
    type    = varchar(100)
    comment = "平台名称"
  }
  column "login_types" {
    null    = false
    type    = json
    comment = "允许的登录方式 [\"password\",\"email\",\"phone\"]"
  }
  column "captcha_type" {
    null    = false
    type    = varchar(30)
    default = "slide"
    comment = "验证码类型: slide"
  }
  column "access_ttl" {
    null     = false
    type     = int
    default  = 14400
    unsigned = true
    comment  = "access_token 有效期（秒）"
  }
  column "refresh_ttl" {
    null     = false
    type     = int
    default  = 1209600
    unsigned = true
    comment  = "refresh_token 有效期（秒）"
  }
  column "bind_platform" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "绑定平台 1=是 2=否"
  }
  column "bind_device" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "绑定设备 1=是 2=否"
  }
  column "bind_ip" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "绑定IP 1=是 2=否"
  }
  column "max_sessions" {
    null     = false
    type     = int
    default  = 5
    unsigned = true
    comment  = "最大会话数（0=不限）"
  }
  column "allow_register" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "允许注册 1=是 2=否"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "状态 1=启用 2=禁用"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "软删除 1=已删 2=正常"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uk_code" {
    unique  = true
    columns = [column.code]
  }
  check "chk_auth_platforms_code" {
    expr = "((`code` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`code` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`code` <> _utf8mb4'all'))"
  }
}
table "authz_principal_versions" {
  schema = schema.admin
  column "user_id" {
    null = false
    type = bigint
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "version" {
    null     = false
    type     = bigint
    default  = 1
    unsigned = true
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.user_id, column.platform]
  }
  check "chk_authz_principal_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
}
table "cron_task" {
  schema  = schema.admin
  comment = "定时任务配置表"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "name" {
    null    = false
    type    = varchar(50)
    comment = "任务标识（唯一）"
  }
  column "title" {
    null    = false
    type    = varchar(100)
    comment = "任务名称"
  }
  column "description" {
    null    = false
    type    = varchar(255)
    default = ""
    comment = "任务描述"
  }
  column "cron" {
    null    = false
    type    = varchar(50)
    comment = "Cron表达式"
  }
  column "cron_readable" {
    null    = false
    type    = varchar(100)
    default = ""
    comment = "Cron可读描述"
  }
  column "handler" {
    null    = false
    type    = varchar(255)
    comment = "处理类"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uniq_cron_task_name" {
    unique  = true
    columns = [column.name]
  }
}
table "cron_task_log" {
  schema  = schema.admin
  comment = "定时任务执行日志表"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "task_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "任务ID"
  }
  column "task_name" {
    null    = false
    type    = varchar(50)
    comment = "任务标识"
  }
  column "start_time" {
    null    = false
    type    = datetime(3)
    comment = "开始时间"
  }
  column "end_time" {
    null    = true
    type    = datetime(3)
    comment = "结束时间"
  }
  column "duration_ms" {
    null     = true
    type     = int
    unsigned = true
    comment  = "执行耗时(毫秒)"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "result" {
    null    = true
    type    = text
    comment = "执行结果"
  }
  column "error_msg" {
    null    = true
    type    = text
    comment = "错误信息"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "soft delete: 1 deleted 2 normal"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_cron_task_log_task_active_created" {
    on {
      column = column.task_id
    }
    on {
      column = column.is_del
    }
    on {
      desc   = true
      column = column.created_at
    }
    on {
      desc   = true
      column = column.id
    }
  }
  index "idx_name_del_id" {
    columns = [column.task_name, column.is_del]
  }
  index "idx_task_del_id" {
    columns = [column.task_id, column.is_del]
  }
}
table "export_tasks" {
  schema  = schema.admin
  comment = "导出任务记录"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "创建用户ID"
  }
  column "platform" {
    null    = false
    type    = varchar(32)
    default = "admin"
    comment = "平台入口"
  }
  column "title" {
    null    = false
    type    = varchar(100)
    comment = "任务标题"
  }
  column "kind" {
    null    = false
    type    = varchar(64)
    default = "user_list"
    comment = "导出类型"
  }
  column "file_name" {
    null    = true
    type    = varchar(255)
    comment = "文件名"
  }
  column "file_url" {
    null    = true
    type    = varchar(500)
    comment = "文件下载URL"
  }
  column "object_key" {
    null    = true
    type    = varchar(500)
    comment = "COS object key"
  }
  column "file_size" {
    null     = true
    type     = int
    unsigned = true
    comment  = "文件大小（字节）"
  }
  column "row_count" {
    null     = true
    type     = int
    unsigned = true
    comment  = "数据行数"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1处理中 2成功 3失败"
  }
  column "claim_owner" {
    null = true
    type = varchar(128)
  }
  column "claim_token" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "claim_expires_at" {
    null = true
    type = datetime(6)
  }
  column "error_msg" {
    null    = true
    type    = varchar(500)
    comment = "失败原因"
  }
  column "expire_at" {
    null    = true
    type    = datetime
    comment = "过期时间（定时任务清理）"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "2正常 1删除"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_created" {
    columns = [column.created_at]
  }
  index "idx_expire" {
    columns = [column.expire_at]
  }
  index "idx_export_task_claim" {
    columns = [column.status, column.is_del, column.claim_expires_at, column.id]
  }
  index "idx_export_tasks_user_platform_active_id" {
    columns = [column.user_id, column.platform, column.is_del, column.id]
  }
  index "idx_export_tasks_user_platform_kind" {
    columns = [column.user_id, column.platform, column.kind, column.is_del]
  }
  index "idx_export_tasks_user_platform_status" {
    columns = [column.user_id, column.platform, column.status, column.is_del]
  }
  index "idx_user_status" {
    columns = [column.user_id, column.status, column.is_del]
  }
  check "chk_export_tasks_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
}
table "job_history" {
  schema  = schema.admin
  collate = "utf8mb4_general_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "job_date" {
    null = false
    type = date
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "total_items" {
    null = false
    type = int
  }
  column "downloaded" {
    null = false
    type = int
  }
  column "skipped" {
    null = false
    type = int
  }
  column "errors" {
    null = false
    type = int
  }
  column "orig_files" {
    null = false
    type = int
  }
  column "chg_files" {
    null = false
    type = int
  }
  column "error_message" {
    null = true
    type = text
  }
  column "started_at" {
    null = true
    type = datetime
  }
  column "completed_at" {
    null = true
    type = datetime
  }
  primary_key {
    columns = [column.id]
  }
  index "ix_job_history_job_date" {
    unique  = true
    columns = [column.job_date]
  }
}
table "mail_configs" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "config_key" {
    null    = false
    type    = varchar(32)
    default = "default"
  }
  column "secret_id_enc" {
    null = false
    type = text
  }
  column "secret_id_hint" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "secret_key_enc" {
    null = false
    type = text
  }
  column "secret_key_hint" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "region" {
    null    = false
    type    = varchar(64)
    default = "ap-guangzhou"
  }
  column "endpoint" {
    null    = false
    type    = varchar(128)
    default = "ses.tencentcloudapi.com"
  }
  column "from_email" {
    null = false
    type = varchar(255)
  }
  column "from_name" {
    null    = false
    type    = varchar(100)
    default = ""
  }
  column "reply_to" {
    null    = false
    type    = varchar(255)
    default = ""
  }
  column "verify_code_ttl_minutes" {
    null     = false
    type     = int
    default  = 5
    unsigned = true
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "last_test_at" {
    null = true
    type = datetime
  }
  column "last_test_error" {
    null    = false
    type    = varchar(500)
    default = ""
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_mail_configs_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uk_mail_configs_config_key" {
    unique  = true
    columns = [column.config_key]
  }
}
table "mail_logs" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "scene" {
    null = false
    type = varchar(32)
  }
  column "template_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "to_email" {
    null = false
    type = varchar(255)
  }
  column "subject" {
    null    = false
    type    = varchar(200)
    default = ""
  }
  column "tencent_request_id" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "tencent_message_id" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "status" {
    null     = false
    type     = tinyint
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "error_code" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "error_message" {
    null    = false
    type    = varchar(500)
    default = ""
  }
  column "duration_ms" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "sent_at" {
    null = true
    type = datetime
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_mail_logs_scene_created" {
    columns = [column.is_del, column.scene, column.created_at]
  }
  index "idx_mail_logs_status_created" {
    columns = [column.is_del, column.status, column.created_at]
  }
  index "idx_mail_logs_to_email_created" {
    columns = [column.is_del, column.to_email, column.created_at]
  }
}
table "mail_log_verification_codes" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "mail_log_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "key_id" {
    null = false
    type = varchar(64)
  }
  column "code_enc" {
    null = false
    type = varchar(255)
  }
  column "expires_at" {
    null = false
    type = datetime
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "uk_mail_log_verification_codes_mail_log" {
    unique  = true
    columns = [column.mail_log_id]
  }
  index "idx_mail_log_verification_codes_key_id_id" {
    columns = [column.key_id, column.id]
  }
  foreign_key "fk_mail_log_verification_codes_mail_log" {
    columns     = [column.mail_log_id]
    ref_columns = [table.mail_logs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
}
table "mail_templates" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "scene" {
    null = false
    type = varchar(32)
  }
  column "name" {
    null = false
    type = varchar(100)
  }
  column "subject" {
    null = false
    type = varchar(200)
  }
  column "tencent_template_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "variables_json" {
    null = false
    type = json
  }
  column "sample_variables_json" {
    null = false
    type = json
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_mail_templates_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uk_mail_templates_scene" {
    unique  = true
    columns = [column.scene]
  }
}
table "notice_files" {
  schema  = schema.admin
  collate = "utf8mb4_general_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "notice_id" {
    null = false
    type = bigint
  }
  column "file_type" {
    null = false
    type = varchar(32)
  }
  column "file_name" {
    null = false
    type = varchar(512)
  }
  column "file_size" {
    null = true
    type = bigint
  }
  column "download_path" {
    null = true
    type = varchar(1024)
  }
  column "download_date" {
    null = true
    type = date
  }
  column "created_at" {
    null = false
    type = datetime
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "notice_files_ibfk_1" {
    columns     = [column.notice_id]
    ref_columns = [table.notices.column.notice_id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "ix_notice_files_download_date" {
    columns = [column.download_date]
  }
  index "ix_notice_files_notice_id" {
    columns = [column.notice_id]
  }
}
table "notices" {
  schema  = schema.admin
  collate = "utf8mb4_general_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "notice_id" {
    null = false
    type = bigint
  }
  column "notice_type" {
    null = false
    type = varchar(32)
  }
  column "is_change" {
    null = false
    type = bool
  }
  column "project_code" {
    null = true
    type = varchar(64)
  }
  column "project_name" {
    null = true
    type = varchar(512)
  }
  column "project_status" {
    null = true
    type = varchar(64)
  }
  column "purchase_type" {
    null = true
    type = varchar(64)
  }
  column "bid_org" {
    null = true
    type = varchar(256)
  }
  column "bid_agency" {
    null = true
    type = varchar(256)
  }
  column "bid_agency_addr" {
    null = true
    type = varchar(512)
  }
  column "bidbook_buy_end_time" {
    null = true
    type = datetime
  }
  column "bidbook_sell_begin_time" {
    null = true
    type = datetime
  }
  column "openbid_time" {
    null = true
    type = datetime
  }
  column "openbid_addr" {
    null = true
    type = varchar(512)
  }
  column "contact_person" {
    null = true
    type = varchar(64)
  }
  column "contact_phone" {
    null = true
    type = varchar(32)
  }
  column "contact_fax" {
    null = true
    type = varchar(32)
  }
  column "email" {
    null = true
    type = varchar(128)
  }
  column "pay_mode" {
    null = true
    type = varchar(64)
  }
  column "project_introduce" {
    null = true
    type = text
  }
  column "change_content" {
    null = true
    type = text
  }
  column "orig_notice_id" {
    null = true
    type = bigint
  }
  column "publish_time" {
    null = true
    type = date
  }
  column "publish_org" {
    null = true
    type = varchar(256)
  }
  column "source_url" {
    null = true
    type = varchar(1024)
  }
  column "raw_json" {
    null = true
    type = json
  }
  column "created_at" {
    null = false
    type = datetime
  }
  column "updated_at" {
    null = false
    type = datetime
  }
  primary_key {
    columns = [column.id]
  }
  index "ix_notices_notice_id" {
    unique  = true
    columns = [column.notice_id]
  }
  index "ix_notices_orig_notice_id" {
    columns = [column.orig_notice_id]
  }
  index "ix_notices_publish_time" {
    columns = [column.publish_time]
  }
}
table "notification_task" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "title" {
    null    = false
    type    = varchar(100)
    comment = "标题"
  }
  column "content" {
    null    = true
    type    = mediumtext
    comment = "内容"
  }
  column "type" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "type: 1 info 2 success 3 warning 4 error"
  }
  column "level" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "level: 1 normal 2 urgent"
  }
  column "link" {
    null    = true
    type    = varchar(500)
    default = ""
    comment = "跳转链接"
  }
  column "platform" {
    null    = false
    type    = varchar(10)
    default = "all"
    comment = "平台 all/admin/app"
  }
  column "target_type" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "target type: 1 all 2 users 3 roles"
  }
  column "target_ids" {
    null    = true
    type    = json
    comment = "目标ID列表"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "claim_owner" {
    null = true
    type = varchar(128)
  }
  column "claim_token" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "claim_expires_at" {
    null = true
    type = datetime(6)
  }
  column "total_count" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "目标用户数"
  }
  column "sent_count" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "已发送数"
  }
  column "send_at" {
    null    = true
    type    = datetime
    comment = "定时发送时间（空=立即发送）"
  }
  column "error_msg" {
    null    = true
    type    = varchar(500)
    comment = "错误信息"
  }
  column "created_by" {
    null     = false
    type     = int
    unsigned = true
    comment  = "Creator user id"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "soft delete: 1 deleted 2 normal"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_notification_task_claim" {
    columns = [column.status, column.is_del, column.send_at, column.claim_expires_at, column.id]
  }
  index "idx_status_del_send" {
    columns = [column.status, column.is_del, column.send_at]
  }
  check "chk_notification_task_platform" {
    expr = "((`platform` = _utf8mb4'all') or ((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))"
  }
}
table "notifications" {
  schema  = schema.admin
  comment = "用户通知表"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "接收用户ID"
  }
  column "source_task_id" {
    null = true
    type = bigint
  }
  column "title" {
    null    = false
    type    = varchar(100)
    comment = "标题"
  }
  column "content" {
    null    = true
    type    = varchar(500)
    default = ""
    comment = "内容"
  }
  column "type" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "type: 1 normal 2 success 3 warning 4 error"
  }
  column "level" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "level: 1 normal 2 urgent"
  }
  column "link" {
    null    = true
    type    = varchar(200)
    default = ""
    comment = "跳转路由"
  }
  column "platform" {
    null    = false
    type    = varchar(10)
    default = "all"
    comment = "平台 all/admin/app"
  }
  column "is_read" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1 read 2 unread"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_notifications_user_active_unread_platform" {
    columns = [column.user_id, column.is_del, column.is_read, column.platform, column.id]
  }
  index "idx_user_platform_del_id" {
    columns = [column.user_id, column.is_del, column.id]
  }
  index "uk_notifications_source_user" {
    unique  = true
    columns = [column.source_task_id, column.user_id]
  }
  check "chk_notifications_platform" {
    expr = "((`platform` = _utf8mb4'all') or ((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all')))"
  }
}
table "operation_logs" {
  schema  = schema.admin
  comment = "操作日志表"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "主键"
    auto_increment = true
  }
  column "user_id" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
  }
  column "action" {
    null    = false
    type    = varchar(255)
    default = ""
    comment = "操作行为/接口名称"
  }
  column "request_data" {
    null    = true
    type    = text
    comment = "请求入参"
  }
  column "response_data" {
    null    = true
    type    = text
    comment = "响应出参"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "2正常 1删除"
  }
  column "is_success" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1 success 2 fail"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_action" {
    columns = [column.action]
  }
  index "idx_created_at" {
    columns = [column.created_at]
  }
  index "idx_del_created_id" {
    columns = [column.is_del, column.created_at, column.id]
  }
  index "idx_user_id" {
    columns = [column.user_id]
  }
}
table "payment_callback_events" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "provider" {
    null    = false
    type    = varchar(32)
    default = "alipay"
  }
  column "dedupe_key" {
    null = false
    type = binary(32)
  }
  column "notify_id" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "out_trade_no" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "trade_no" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "trade_status" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "app_id" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "total_amount_cents" {
    null    = false
    type    = bigint
    default = 0
  }
  column "signature_valid" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "process_status" {
    null    = false
    type    = varchar(16)
    default = "pending"
  }
  column "process_message" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "raw_payload_json" {
    null = true
    type = json
  }
  column "received_at" {
    null = false
    type = datetime
  }
  column "processed_at" {
    null = true
    type = datetime
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_payment_callback_events_notify_id" {
    columns = [column.provider, column.notify_id]
  }
  index "idx_payment_callback_events_out_trade_no" {
    columns = [column.provider, column.out_trade_no]
  }
  index "idx_payment_callback_events_status_time" {
    columns = [column.process_status, column.received_at]
  }
  index "uk_payment_callback_events_dedupe" {
    unique  = true
    columns = [column.dedupe_key]
  }
}
table "payment_configs" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "provider" {
    null    = false
    type    = varchar(32)
    default = "alipay"
  }
  column "code" {
    null = false
    type = varchar(64)
  }
  column "name" {
    null = false
    type = varchar(128)
  }
  column "app_id" {
    null = false
    type = varchar(64)
  }
  column "private_key_enc" {
    null = false
    type = text
  }
  column "private_key_hint" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "app_cert_path" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "platform_cert_path" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "root_cert_path" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "notify_url" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "environment" {
    null    = false
    type    = varchar(16)
    default = "sandbox"
  }
  column "enabled_methods_json" {
    null = false
    type = json
  }
  column "sort" {
    null    = false
    type    = int
    default = 100
  }
  column "status" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "remark" {
    null    = false
    type    = varchar(255)
    default = ""
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_payment_configs_environment" {
    columns = [column.environment, column.is_del]
  }
  index "idx_payment_configs_provider_status" {
    columns = [column.provider, column.status, column.is_del]
  }
  index "idx_payment_configs_provider_status_sort" {
    columns = [column.provider, column.status, column.is_del, column.sort, column.id]
  }
  index "uk_payment_configs_code" {
    unique  = true
    columns = [column.code]
  }
}
table "payment_orders" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "order_no" {
    null = false
    type = varchar(64)
  }
  column "config_id" {
    null = false
    type = bigint
  }
  column "config_code" {
    null = false
    type = varchar(64)
  }
  column "provider" {
    null    = false
    type    = varchar(32)
    default = "alipay"
  }
  column "pay_method" {
    null = false
    type = varchar(16)
  }
  column "subject" {
    null = false
    type = varchar(128)
  }
  column "amount_cents" {
    null = false
    type = bigint
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "pay_url" {
    null    = false
    type    = varchar(2048)
    default = ""
  }
  column "return_url" {
    null    = false
    type    = varchar(512)
    default = ""
  }
  column "alipay_trade_no" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "alipay_trade_no_identity" {
    null    = true
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "expired_at" {
    null = false
    type = datetime
  }
  column "paid_at" {
    null = true
    type = datetime
  }
  column "closed_at" {
    null = true
    type = datetime
  }
  column "failure_reason" {
    null    = false
    type    = varchar(255)
    default = ""
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_payment_order_config" {
    columns     = [column.config_id]
    ref_columns = [table.payment_configs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_payment_order_config_created" {
    columns = [column.config_id, column.created_at, column.is_del]
  }
  index "idx_payment_order_status_created" {
    columns = [column.is_del, column.status, column.created_at]
  }
  index "idx_payment_orders_provider_status_expired" {
    columns = [column.provider, column.status, column.is_del, column.expired_at, column.id]
  }
  index "idx_payment_orders_status_updated" {
    columns = [column.status, column.is_del, column.updated_at, column.id]
  }
  index "uk_payment_order_no" {
    unique  = true
    columns = [column.order_no]
  }
  index "uk_payment_orders_alipay_trade_identity" {
    unique  = true
    columns = [column.alipay_trade_no_identity]
  }
  check "chk_payment_orders_alipay_trade_identity" {
    expr = "(((`alipay_trade_no` = _utf8mb4'') and (`alipay_trade_no_identity` is null)) or ((`alipay_trade_no` <> _utf8mb4'') and (cast(`alipay_trade_no_identity` as binary) = cast(`alipay_trade_no` as binary))))"
  }
}
table "payment_recharge_packages" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "code" {
    null = false
    type = varchar(64)
  }
  column "name" {
    null = false
    type = varchar(128)
  }
  column "amount_cents" {
    null = false
    type = bigint
  }
  column "badge" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "sort" {
    null    = false
    type    = int
    default = 100
  }
  column "status" {
    null    = false
    type    = tinyint
    default = 1
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_payment_recharge_package_status_sort" {
    columns = [column.status, column.is_del, column.sort, column.id]
  }
  index "uk_payment_recharge_package_code" {
    unique  = true
    columns = [column.code]
  }
}
table "payment_recharges" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "recharge_no" {
    null = false
    type = varchar(64)
  }
  column "user_id" {
    null = false
    type = bigint
  }
  column "package_code" {
    null = false
    type = varchar(64)
  }
  column "package_name" {
    null = false
    type = varchar(128)
  }
  column "amount_cents" {
    null = false
    type = bigint
  }
  column "payment_order_id" {
    null = false
    type = bigint
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "paid_at" {
    null = true
    type = datetime
  }
  column "credited_at" {
    null = true
    type = datetime
  }
  column "failure_reason" {
    null    = false
    type    = varchar(255)
    default = ""
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_payment_recharge_order" {
    columns     = [column.payment_order_id]
    ref_columns = [table.payment_orders.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_payment_recharge_created" {
    columns = [column.is_del, column.created_at]
  }
  index "idx_payment_recharge_user_status_created" {
    columns = [column.user_id, column.is_del, column.status, column.created_at]
  }
  index "uk_payment_recharge_no" {
    unique  = true
    columns = [column.recharge_no]
  }
  index "uk_payment_recharge_order" {
    unique  = true
    columns = [column.payment_order_id]
  }
}
table "permissions" {
  schema  = schema.admin
  comment = "菜单权限表"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "name" {
    null    = false
    type    = varchar(50)
    default = ""
    comment = "权限名"
  }
  column "path" {
    null    = true
    type    = varchar(255)
    default = ""
    comment = "路由"
  }
  column "icon" {
    null    = true
    type    = varchar(100)
    default = ""
    comment = "图标"
  }
  column "parent_id" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "parent permission id; 0 means root"
  }
  column "component" {
    null    = true
    type    = varchar(255)
    comment = "组件路径"
  }
  column "platform" {
    null    = false
    type    = varchar(10)
    default = "admin"
    comment = "平台：admin=PC后台, app=H5/APP"
  }
  column "type" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "type: 1 dir 2 page 3 button"
  }
  column "sort" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "排序"
  }
  column "code" {
    null    = true
    type    = varchar(100)
    comment = "权限标识"
  }
  column "i18n_key" {
    null    = false
    type    = varchar(128)
    default = ""
    comment = "i18n键"
  }
  column "show_menu" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "show menu: 1 yes 2 no"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_permissions_parent_sort" {
    columns = [column.parent_id, column.sort]
  }
  index "idx_permissions_platform" {
    columns = [column.platform]
  }
  index "idx_permissions_status_del_platform_type" {
    columns = [column.is_del, column.status, column.platform, column.type]
  }
  index "uk_permissions_platform_code" {
    unique  = true
    columns = [column.platform, column.code]
  }
  check "chk_permissions_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
}
table "realtime_event_retention_watermarks" {
  schema = schema.admin
  column "target_type" {
    null = false
    type = varchar(16)
  }
  column "target_id" {
    null = false
    type = varchar(64)
  }
  column "deleted_through_sequence" {
    null     = false
    type     = bigint
    unsigned = true
    default  = 0
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.target_type, column.target_id]
  }
}
table "realtime_events" {
  schema = schema.admin
  column "sequence" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "event_id" {
    null = false
    type = char(26)
  }
  column "event_type" {
    null = false
    type = varchar(96)
  }
  column "request_id" {
    null = true
    type = varchar(128)
  }
  column "target_type" {
    null = false
    type = varchar(16)
  }
  column "target_id" {
    null = false
    type = varchar(64)
  }
  column "durability" {
    null = false
    type = varchar(16)
  }
  column "payload_json" {
    null = false
    type = json
  }
  column "occurred_at" {
    null = false
    type = datetime(6)
  }
  column "expires_at" {
    null = false
    type = datetime(6)
  }
  primary_key {
    columns = [column.sequence]
  }
  index "idx_realtime_resume" {
    columns = [column.target_type, column.target_id, column.sequence]
  }
  index "idx_realtime_expiry" {
    columns = [column.expires_at, column.sequence]
  }
  index "uk_realtime_event_id" {
    unique  = true
    columns = [column.event_id]
  }
}
table "redeem_code_batches" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "batch_no" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "request_id" {
    null    = false
    type    = varchar(128)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "request_fingerprint_version" {
    null    = false
    type    = varchar(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "request_fingerprint" {
    null    = false
    type    = char(64)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "amount_cents" {
    null = false
    type = bigint
  }
  column "quantity" {
    null     = false
    type     = int
    unsigned = true
  }
  column "expires_at" {
    null = true
    type = datetime(6)
  }
  column "note" {
    null    = false
    type    = varchar(255)
    default = ""
  }
  column "created_by" {
    null     = false
    type     = int
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_redeem_code_batches_created_by" {
    columns     = [column.created_by]
    ref_columns = [table.users.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_redeem_code_batches_created_at_id" {
    columns = [column.created_at, column.id]
  }
  index "idx_redeem_code_batches_expires_at_id" {
    columns = [column.expires_at, column.id]
  }
  index "uk_redeem_code_batches_batch_no" {
    unique  = true
    columns = [column.batch_no]
  }
  index "uk_redeem_code_batches_creator_request" {
    unique  = true
    columns = [column.created_by, column.request_id]
  }
  check "chk_redeem_code_batches_amount_cents" {
    expr = "(`amount_cents` between 1 and 100000000)"
  }
  check "chk_redeem_code_batches_expiry" {
    expr = "((`expires_at` is null) or (`expires_at` > `created_at`))"
  }
  check "chk_redeem_code_batches_quantity" {
    expr = "(`quantity` between 1 and 1000)"
  }
}
table "redeem_codes" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "batch_id" {
    null = false
    type = bigint
  }
  column "code" {
    null    = false
    type    = char(28)
    charset = "ascii"
    collate = "ascii_bin"
  }
  column "state" {
    null = false
    type = varchar(16)
  }
  column "used_by" {
    null     = true
    type     = int
    unsigned = true
  }
  column "used_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  column "updated_at" {
    null      = false
    type      = datetime(6)
    default   = sql("CURRENT_TIMESTAMP(6)")
    on_update = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_redeem_codes_batch" {
    columns     = [column.batch_id]
    ref_columns = [table.redeem_code_batches.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  foreign_key "fk_redeem_codes_used_by" {
    columns     = [column.used_by]
    ref_columns = [table.users.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_redeem_codes_batch_state_id" {
    columns = [column.batch_id, column.state, column.id]
  }
  index "idx_redeem_codes_state_id" {
    columns = [column.state, column.id]
  }
  index "idx_redeem_codes_used_by_used_at_id" {
    columns = [column.used_by, column.used_at, column.id]
  }
  index "uk_redeem_codes_code" {
    unique  = true
    columns = [column.code]
  }
  check "chk_redeem_codes_state" {
    expr = "(`state` in (_utf8mb4'unused',_utf8mb4'used',_utf8mb4'voided'))"
  }
  check "chk_redeem_codes_usage" {
    expr = "(((`state` = _utf8mb4'used') and (`used_by` is not null) and (`used_at` is not null)) or ((`state` in (_utf8mb4'unused',_utf8mb4'voided')) and (`used_by` is null) and (`used_at` is null)))"
  }
}
table "role_permissions" {
  schema  = schema.admin
  comment = "role permission pivot"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "role_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "role.id"
  }
  column "permission_id" {
    null     = false
    type     = int
    unsigned = true
    comment  = "permission.id"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_role_permissions_permission_del_role" {
    columns = [column.permission_id, column.is_del, column.role_id]
  }
  index "uniq_role_permission" {
    unique  = true
    columns = [column.role_id, column.permission_id]
  }
}
table "roles" {
  schema  = schema.admin
  comment = "角色"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "name" {
    null    = false
    type    = varchar(50)
    default = ""
    comment = "role name"
  }
  column "is_default" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_roles_default_del" {
    columns = [column.is_default, column.is_del]
  }
  index "uk_roles_name" {
    unique  = true
    columns = [column.name]
  }
}
table "scheduler_settings" {
  schema  = schema.admin
  collate = "utf8mb4_general_ci"
  column "id" {
    null = false
    type = int
  }
  column "enabled" {
    null = false
    type = bool
  }
  column "cron_schedule" {
    null = false
    type = varchar(64)
  }
  column "timezone" {
    null = false
    type = varchar(64)
  }
  column "created_at" {
    null = false
    type = datetime
  }
  column "updated_at" {
    null = false
    type = datetime
  }
  primary_key {
    columns = [column.id]
  }
}
table "schema_reconciliation_runs" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "stage" {
    null = false
    type = varchar(32)
  }
  column "script_name" {
    null = false
    type = varchar(191)
  }
  column "script_sha256" {
    null = false
    type = char(64)
  }
  column "source_fingerprint_sha256" {
    null = false
    type = char(64)
  }
  column "target_fingerprint_sha256" {
    null = true
    type = char(64)
  }
  column "executor" {
    null = false
    type = varchar(191)
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "details_json" {
    null = true
    type = json
  }
  column "started_at" {
    null = false
    type = datetime(6)
  }
  column "finished_at" {
    null = true
    type = datetime(6)
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_schema_reconciliation_status" {
    columns = [column.status, column.started_at, column.id]
  }
  index "uk_schema_reconciliation_script_sha" {
    unique  = true
    columns = [column.script_name, column.script_sha256]
  }
  check "chk_schema_reconciliation_status" {
    expr = "(`status` in (_gbk'running',_gbk'succeeded',_gbk'failed'))"
  }
}
table "sms_configs" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "config_key" {
    null    = false
    type    = varchar(32)
    default = "default"
  }
  column "secret_id_enc" {
    null = false
    type = text
  }
  column "secret_id_hint" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "secret_key_enc" {
    null = false
    type = text
  }
  column "secret_key_hint" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "sms_sdk_app_id" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "sign_name" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "region" {
    null    = false
    type    = varchar(64)
    default = "ap-guangzhou"
  }
  column "endpoint" {
    null    = false
    type    = varchar(128)
    default = "sms.tencentcloudapi.com"
  }
  column "verify_code_ttl_minutes" {
    null     = false
    type     = int
    default  = 5
    unsigned = true
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "last_test_at" {
    null = true
    type = datetime
  }
  column "last_test_error" {
    null    = false
    type    = varchar(500)
    default = ""
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_sms_configs_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uk_sms_configs_config_key" {
    unique  = true
    columns = [column.config_key]
  }
}
table "sms_logs" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "scene" {
    null = false
    type = varchar(32)
  }
  column "template_id" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "to_phone" {
    null = false
    type = varchar(32)
  }
  column "status" {
    null     = false
    type     = tinyint
    unsigned = true
  }
  column "tencent_request_id" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "tencent_serial_no" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "tencent_fee" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "error_code" {
    null    = false
    type    = varchar(128)
    default = ""
  }
  column "error_message" {
    null    = false
    type    = varchar(500)
    default = ""
  }
  column "duration_ms" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "sent_at" {
    null = true
    type = datetime
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_sms_logs_scene_created" {
    columns = [column.is_del, column.scene, column.created_at]
  }
  index "idx_sms_logs_status_created" {
    columns = [column.is_del, column.status, column.created_at]
  }
  index "idx_sms_logs_to_phone_created" {
    columns = [column.is_del, column.to_phone, column.created_at]
  }
}
table "sms_templates" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "scene" {
    null = false
    type = varchar(32)
  }
  column "name" {
    null = false
    type = varchar(100)
  }
  column "tencent_template_id" {
    null = false
    type = varchar(32)
  }
  column "variables_json" {
    null = false
    type = json
  }
  column "sample_variables_json" {
    null = false
    type = json
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_sms_templates_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uk_sms_templates_scene" {
    unique  = true
    columns = [column.scene]
  }
}
table "system_settings" {
  schema  = schema.admin
  comment = "系统设置（key-value）"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "setting_key" {
    null    = false
    type    = varchar(100)
    comment = "配置键：如 user.default_avatar"
  }
  column "setting_value" {
    null    = false
    type    = text
    comment = "配置值（字符串/JSON字符串均可）"
  }
  column "value_type" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "remark" {
    null    = false
    type    = varchar(255)
    default = ""
    comment = "备注说明"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_status_del" {
    columns = [column.status, column.is_del]
  }
  index "uniq_setting_key" {
    unique  = true
    columns = [column.setting_key]
  }
}
table "upload_driver" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "driver" {
    null    = false
    type    = varchar(20)
    comment = "cos / oss / s3 / qiniu 等"
  }
  column "secret_id_enc" {
    null = true
    type = text
  }
  column "secret_id_hint" {
    null = true
    type = varchar(20)
  }
  column "secret_key_enc" {
    null = true
    type = text
  }
  column "secret_key_hint" {
    null = true
    type = varchar(20)
  }
  column "bucket" {
    null = false
    type = varchar(255)
  }
  column "region" {
    null = false
    type = varchar(100)
  }
  column "appid" {
    null    = true
    type    = varchar(100)
    comment = "COS 特有"
  }
  column "endpoint" {
    null    = true
    type    = varchar(255)
    comment = "OSS/S3/AP custom domain"
  }
  column "bucket_domain" {
    null    = true
    type    = varchar(255)
    comment = "返回给前端用于访问的域名（可配 CDN）"
  }
  column "role_arn" {
    null    = true
    type    = varchar(255)
    comment = "OSS AssumeRole / AWS role arn"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "uniq_driver_bucket" {
    unique  = true
    columns = [column.driver, column.bucket]
  }
}
table "upload_rule" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "title" {
    null    = false
    type    = varchar(50)
    default = ""
    comment = "规则标题"
  }
  column "max_size_mb" {
    null     = false
    type     = int
    default  = 5
    unsigned = true
    comment  = "最大 MB"
  }
  column "image_exts" {
    null    = false
    type    = json
    comment = "允许的图片扩展名"
  }
  column "file_exts" {
    null    = false
    type    = json
    comment = "允许的通用文件扩展名"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
}
table "upload_setting" {
  schema  = schema.admin
  comment = "上传设置：驱动+规则组合与启用状态"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "driver_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "rule_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "remark" {
    null    = false
    type    = varchar(255)
    default = ""
    comment = "备注"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_rule" {
    columns = [column.rule_id]
  }
  index "idx_status" {
    columns = [column.status]
  }
  index "uniq_driver_rule" {
    unique  = true
    columns = [column.driver_id, column.rule_id]
  }
}
table "user_profiles" {
  schema  = schema.admin
  comment = "用户资料表"
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "avatar" {
    null    = false
    type    = varchar(255)
    default = "https://zgm-1314542588.cos.ap-nanjing.myqcloud.com/defaultAvatar%2Favatar.jpg"
    comment = "头像"
  }
  column "bio" {
    null    = true
    type    = text
    comment = "个人简介"
  }
  column "sex" {
    null     = false
    type     = tinyint
    default  = 0
    unsigned = true
    comment  = "sex: 0 unknown 1 male 2 female"
  }
  column "birthday" {
    null    = true
    type    = date
    comment = "生日"
  }
  column "address_id" {
    null     = true
    type     = int
    unsigned = true
    comment  = "地址ID"
  }
  column "detail_address" {
    null    = false
    type    = varchar(255)
    default = ""
    comment = "详细地址"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "soft delete: 1 deleted 2 normal"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "更新时间"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.user_id]
  }
}
table "user_sessions" {
  schema  = schema.admin
  comment = "用户会话表"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "user_id" {
    null     = false
    type     = int
    unsigned = true
  }
  column "refresh_token_hash" {
    null    = false
    type    = char(64)
    comment = "refresh token sha256"
  }
  column "platform" {
    null    = false
    type    = varchar(20)
    default = ""
    comment = "pc/h5/app/mini"
  }
  column "device_id" {
    null    = false
    type    = varchar(64)
    default = ""
    comment = "设备标识(前端生成uuid即可)"
  }
  column "ip" {
    null    = false
    type    = varchar(64)
    default = ""
    comment = "登录IP"
  }
  column "ua" {
    null    = true
    type    = varchar(255)
    comment = "User-Agent"
  }
  column "last_seen_at" {
    null    = true
    type    = datetime
    comment = "最后活跃时间"
  }
  column "expires_at" {
    null    = false
    type    = datetime
    comment = "access过期时间"
  }
  column "refresh_expires_at" {
    null    = false
    type    = datetime
    comment = "refresh过期时间"
  }
  column "revoked_at" {
    null    = true
    type    = datetime
    comment = "注销/踢下线时间"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "2 normal 1 deleted"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_active_stats" {
    columns = [column.is_del, column.revoked_at, column.expires_at, column.platform]
  }
  index "idx_expires_at" {
    columns = [column.expires_at]
  }
  index "idx_refresh_expires_at" {
    columns = [column.refresh_expires_at]
  }
  index "idx_user_platform" {
    columns = [column.user_id, column.platform]
  }
  index "idx_user_sessions_user_platform_active_refresh" {
    columns = [column.user_id, column.platform, column.is_del, column.revoked_at, column.refresh_expires_at, column.id]
  }
  index "uniq_refresh_hash" {
    unique  = true
    columns = [column.refresh_token_hash]
  }
  check "chk_user_sessions_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
}
table "user_wallets" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "user_id" {
    null = false
    type = bigint
  }
  column "balance_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "total_recharge_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "total_consume_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "held_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_user_wallet_isdel" {
    columns = [column.is_del]
  }
  index "idx_user_wallet_updated" {
    columns = [column.is_del, column.updated_at, column.id]
  }
  index "uk_user_wallet_user" {
    unique  = true
    columns = [column.user_id]
  }
  check "chk_user_wallet_units_nonnegative" {
    expr = "((`balance_units` >= 0) and (`total_recharge_units` >= 0) and (`total_consume_units` >= 0) and (`held_units` >= 0) and (`held_units` <= `balance_units`))"
  }
}
table "users" {
  schema  = schema.admin
  comment = "用户表"
  column "id" {
    null           = false
    type           = int
    unsigned       = true
    auto_increment = true
  }
  column "role_id" {
    null     = false
    type     = int
    default  = 1
    unsigned = true
  }
  column "username" {
    null    = false
    type    = varchar(50)
    default = ""
    comment = "用户名"
  }
  column "email" {
    null    = true
    type    = varchar(255)
    comment = "邮箱"
  }
  column "password" {
    null    = true
    type    = varchar(255)
    comment = "密码(可空: 首次第三方/邮箱免密创建)"
  }
  column "phone" {
    null    = true
    type    = varchar(20)
    comment = "手机号"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_users_active" {
    columns = [column.is_del, column.status]
  }
  index "idx_users_role_del" {
    columns = [column.role_id, column.is_del]
  }
  index "uniq_users_email" {
    unique  = true
    columns = [column.email]
  }
  index "uniq_users_phone" {
    unique  = true
    columns = [column.phone]
  }
}
table "users_login_log" {
  schema  = schema.admin
  comment = "登录日志"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "user_id" {
    null     = true
    type     = int
    unsigned = true
  }
  column "login_account" {
    null    = false
    type    = varchar(120)
    default = ""
    comment = "登录账号"
  }
  column "login_type" {
    null    = false
    type    = varchar(20)
    default = "email"
    comment = "登录类型"
  }
  column "platform" {
    null    = false
    type    = varchar(20)
    default = ""
    comment = "平台"
  }
  column "ip" {
    null    = false
    type    = varchar(64)
    default = ""
    comment = "IP地址"
  }
  column "ua" {
    null    = true
    type    = varchar(512)
    comment = "User-Agent"
  }
  column "is_success" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1 success 2 fail"
  }
  column "reason" {
    null    = false
    type    = varchar(50)
    default = ""
    comment = "失败原因"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "soft delete: 1 deleted 2 normal"
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
    comment = "创建时间"
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    comment   = "updated at"
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_account_created" {
    on {
      column = column.login_account
    }
    on {
      desc   = true
      column = column.created_at
    }
  }
  index "idx_created" {
    on {
      desc   = true
      column = column.created_at
    }
  }
  index "idx_ip_created" {
    on {
      column = column.ip
    }
    on {
      desc   = true
      column = column.created_at
    }
  }
  index "idx_user_created" {
    on {
      column = column.user_id
    }
    on {
      desc   = true
      column = column.created_at
    }
  }
  check "chk_users_login_log_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
}
table "wallet_transactions" {
  schema  = schema.admin
  collate = "utf8mb4_unicode_ci"
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "transaction_no" {
    null = false
    type = varchar(64)
  }
  column "wallet_id" {
    null = false
    type = bigint
  }
  column "user_id" {
    null = false
    type = bigint
  }
  column "direction" {
    null = false
    type = varchar(16)
  }
  column "amount_units" {
    null = false
    type = bigint
  }
  column "balance_before_units" {
    null = false
    type = bigint
  }
  column "balance_after_units" {
    null = false
    type = bigint
  }
  column "source_type" {
    null = false
    type = varchar(32)
  }
  column "source_id" {
    null = false
    type = bigint
  }
  column "remark" {
    null    = false
    type    = varchar(255)
    default = ""
  }
  column "is_del" {
    null    = false
    type    = tinyint
    default = 2
  }
  column "created_at" {
    null    = false
    type    = datetime
    default = sql("CURRENT_TIMESTAMP")
  }
  column "updated_at" {
    null      = false
    type      = datetime
    default   = sql("CURRENT_TIMESTAMP")
    on_update = sql("CURRENT_TIMESTAMP")
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_wallet_transaction_user_created" {
    columns = [column.user_id, column.is_del, column.created_at]
  }
  index "idx_wallet_transaction_wallet_created" {
    columns = [column.wallet_id, column.is_del, column.created_at]
  }
  index "idx_wallet_tx_admin_created" {
    columns = [column.is_del, column.created_at, column.id]
  }
  index "idx_wallet_tx_admin_direction_created" {
    columns = [column.direction, column.is_del, column.created_at, column.id]
  }
  index "idx_wallet_tx_admin_source_created" {
    columns = [column.source_type, column.is_del, column.created_at, column.id]
  }
  index "uk_wallet_transaction_no" {
    unique  = true
    columns = [column.transaction_no]
  }
  index "uk_wallet_transaction_source" {
    unique  = true
    columns = [column.source_type, column.source_id]
  }
  check "chk_wallet_transaction_units_nonnegative" {
    expr = "((`amount_units` >= 0) and (`balance_before_units` >= 0) and (`balance_after_units` >= 0))"
  }
}
table "ai_billing_migration_metadata" {
  schema  = schema.admin
  comment = "Persistent validation boundary for AI billing legacy identity backfill"
  column "migration_key" {
    null = false
    type = varchar(64)
  }
  column "legacy_cutover_at" {
    null = false
    type = datetime(6)
  }
  column "marker_version" {
    null = false
    type = varchar(64)
  }
  column "marker_sha256" {
    null = false
    type = binary(32)
  }
  column "phase" {
    null    = false
    type    = varchar(32)
    default = "not_started"
  }
  column "phase_started_at" {
    null = true
    type = datetime(6)
  }
  column "phase_completed_at" {
    null = true
    type = datetime(6)
  }
  column "created_at" {
    null    = false
    type    = datetime(6)
    default = sql("CURRENT_TIMESTAMP(6)")
  }
  primary_key {
    columns = [column.migration_key]
  }
}
table "ai_run_dashboard_facts" {
  schema  = schema.admin
  comment = "Immutable terminal Run projection for exact AI dashboard analytics"
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "fact_date" {
    null = false
    type = date
  }
  column "run_created_at" {
    null = false
    type = datetime
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "model_id" {
    null = false
    type = varchar(191)
  }
  column "model_display_name" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "provider_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "user_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "prompt_tokens" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "completion_tokens" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "total_tokens" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "duration_ms" {
    null     = true
    type     = bigint
    unsigned = true
  }
  column "settled_runs" {
    null     = false
    type     = tinyint
    default  = 0
    unsigned = true
  }
  column "actual_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "released_runs" {
    null     = false
    type     = tinyint
    default  = 0
    unsigned = true
  }
  column "released_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "unbilled_runs" {
    null     = false
    type     = tinyint
    default  = 0
    unsigned = true
  }
  column "run_anomaly_code" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "billing_anomaly_code" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "final_error_code" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "ttft_ms" {
    null     = true
    type     = bigint
    unsigned = true
  }
  primary_key {
    columns = [column.run_id]
  }
  foreign_key "fk_ai_run_dashboard_facts_run" {
    columns     = [column.run_id]
    ref_columns = [table.ai_runs.column.id]
    on_update   = RESTRICT
    on_delete   = RESTRICT
  }
  index "idx_ai_run_dashboard_facts_created" {
    columns = [column.fact_date, column.run_id]
  }
  index "idx_ai_run_dashboard_facts_status_created" {
    columns = [column.status, column.fact_date, column.run_id]
  }
  index "idx_ai_run_dashboard_facts_model_created" {
    columns = [column.model_id, column.fact_date, column.run_id]
  }
  index "idx_ai_run_dashboard_facts_platform_created" {
    columns = [column.platform, column.fact_date, column.run_id]
  }
  index "idx_ai_run_dashboard_facts_agent_created" {
    columns = [column.agent_id, column.fact_date, column.run_id]
  }
  index "idx_ai_run_dashboard_facts_provider_created" {
    columns = [column.provider_id, column.fact_date, column.run_id]
  }
  index "idx_ai_run_dashboard_facts_user_created" {
    columns = [column.user_id, column.fact_date, column.run_id]
  }
  check "chk_ai_run_dashboard_facts_status" {
    expr = "(`status` in (_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'outcome_unknown'))"
  }
  check "chk_ai_run_dashboard_facts_nonnegative" {
    expr = "((`actual_units` >= 0) and (`released_units` >= 0) and (`settled_runs` between 0 and 1) and (`released_runs` between 0 and 1) and (`unbilled_runs` between 0 and 1))"
  }
}
table "ai_run_dashboard_daily_facts" {
  schema  = schema.admin
  comment = "Daily terminal Run aggregate for bounded AI dashboard analytics"
  column "fact_date" {
    null = false
    type = date
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "model_id" {
    null = false
    type = varchar(191)
  }
  column "agent_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "provider_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "user_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "status" {
    null = false
    type = varchar(16)
  }
  column "run_anomaly_code" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "billing_anomaly_code" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "final_error_code" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "latest_run_id" {
    null     = false
    type     = bigint
    unsigned = true
  }
  column "latest_model_display_name" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "run_count" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "prompt_tokens" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "completion_tokens" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "total_tokens" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "settled_runs" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "actual_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "released_runs" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  column "released_units" {
    null    = false
    type    = bigint
    default = 0
  }
  column "unbilled_runs" {
    null     = false
    type     = bigint
    default  = 0
    unsigned = true
  }
  primary_key {
    columns = [column.fact_date, column.platform, column.model_id, column.agent_id, column.provider_id, column.user_id, column.status, column.run_anomaly_code, column.billing_anomaly_code, column.final_error_code]
  }
  index "idx_ai_run_dashboard_daily_model_date" {
    columns = [column.model_id, column.fact_date]
  }
  index "idx_ai_run_dashboard_daily_platform_date" {
    columns = [column.platform, column.fact_date]
  }
  index "idx_ai_run_dashboard_daily_provider_date" {
    columns = [column.provider_id, column.fact_date]
  }
  index "idx_ai_run_dashboard_daily_agent_date" {
    columns = [column.agent_id, column.fact_date]
  }
  index "idx_ai_run_dashboard_daily_user_date" {
    columns = [column.user_id, column.fact_date]
  }
  index "idx_ai_run_dashboard_daily_error_date" {
    columns = [column.final_error_code, column.fact_date]
  }
  check "chk_ai_run_dashboard_daily_status" {
    expr = "(`status` in (_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout',_utf8mb4'outcome_unknown'))"
  }
  check "chk_ai_run_dashboard_daily_nonnegative" {
    expr = "((`actual_units` >= 0) and (`released_units` >= 0))"
  }
}
schema "admin" {
  charset = "utf8mb4"
  collate = "utf8mb4_0900_ai_ci"
}
