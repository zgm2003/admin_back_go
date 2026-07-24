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
table "ai_agent_knowledge_bases" {
  schema  = schema.admin
  comment = "AI智能体知识库绑定"
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
  column "knowledge_base_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_knowledge_bases.id"
  }
  column "top_k" {
    null     = false
    type     = int
    default  = 5
    unsigned = true
    comment  = "本智能体对此知识库召回条数"
  }
  column "min_score" {
    null     = false
    type     = decimal(8, 4)
    default  = 0.1
    unsigned = false
    comment  = "本智能体对此知识库最低命中分"
  }
  column "max_context_chars" {
    null     = false
    type     = int
    default  = 6000
    unsigned = true
    comment  = "本智能体对此知识库最大注入字符数"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1启用 2禁用；运行时只加载启用绑定"
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
  index "idx_ai_agent_knowledge_agent" {
    columns = [column.agent_id, column.status, column.is_del]
  }
  index "idx_ai_agent_knowledge_base" {
    columns = [column.knowledge_base_id, column.status, column.is_del]
  }
  index "uk_ai_agent_knowledge_base" {
    unique  = true
    columns = [column.agent_id, column.knowledge_base_id, column.is_del]
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
  primary_key {
    columns = [column.id]
  }
  index "idx_ai_agents_model" {
    columns = [column.provider_id, column.model_id, column.status, column.is_del]
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
    type     = int
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
  column "error_message" {
    null    = false
    type    = varchar(1000)
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
  index "idx_ai_image_tasks_agent_created" {
    columns = [column.agent_id, column.created_at]
  }
  index "idx_ai_image_tasks_platform_status_created" {
    columns = [column.platform, column.status, column.created_at]
  }
  index "idx_ai_image_tasks_platform_user_created" {
    columns = [column.platform, column.user_id, column.created_at]
  }
  check "chk_ai_image_tasks_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
}
table "ai_knowledge_bases" {
  schema  = schema.admin
  comment = "AI知识库"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "知识库ID"
    auto_increment = true
  }
  column "name" {
    null    = false
    type    = varchar(128)
    comment = "知识库名称，列表、绑定、监控展示"
  }
  column "code" {
    null    = false
    type    = varchar(128)
    comment = "知识库唯一编码，用于种子幂等和人工识别"
  }
  column "description" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "知识库说明，管理页展示和智能体绑定时辅助选择"
  }
  column "chunk_size_chars" {
    null     = false
    type     = int
    default  = 1200
    unsigned = true
    comment  = "默认分块字符数，重建文档分块时使用"
  }
  column "chunk_overlap_chars" {
    null     = false
    type     = int
    default  = 120
    unsigned = true
    comment  = "默认分块重叠字符数，重建文档分块时使用"
  }
  column "default_top_k" {
    null     = false
    type     = int
    default  = 5
    unsigned = true
    comment  = "检索测试和智能体绑定默认召回条数"
  }
  column "default_min_score" {
    null     = false
    type     = decimal(8, 4)
    default  = 0.1
    unsigned = false
    comment  = "检索测试和智能体绑定默认最低分"
  }
  column "default_max_context_chars" {
    null     = false
    type     = int
    default  = 6000
    unsigned = true
    comment  = "检索测试和智能体绑定默认上下文字符预算"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1启用 2禁用；运行时只读取启用知识库"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1删除 2正常；所有查询默认 is_del=2"
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
  index "idx_ai_knowledge_bases_status" {
    columns = [column.status, column.is_del, column.updated_at]
  }
  index "uk_ai_knowledge_bases_code" {
    unique  = true
    columns = [column.code, column.is_del]
  }
}
table "ai_knowledge_chunks" {
  schema  = schema.admin
  comment = "AI知识库分块"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "分块ID"
    auto_increment = true
  }
  column "knowledge_base_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_knowledge_bases.id，检索时直接过滤"
  }
  column "document_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_knowledge_documents.id"
  }
  column "chunk_index" {
    null     = false
    type     = int
    unsigned = true
    comment  = "同一文档内分块序号，从1开始"
  }
  column "title" {
    null    = false
    type    = varchar(191)
    default = ""
    comment = "分块标题，默认继承文档标题"
  }
  column "content" {
    null    = false
    type    = text
    comment = "分块内容，检索和上下文注入使用"
  }
  column "content_chars" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "分块字符数，用于 max_context_chars 预算"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1启用 2禁用；运行时只读取启用分块"
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
  index "idx_ai_knowledge_chunks_base" {
    columns = [column.knowledge_base_id, column.status, column.is_del, column.id]
  }
  index "idx_ai_knowledge_chunks_document" {
    columns = [column.document_id, column.status, column.is_del]
  }
  index "uk_ai_knowledge_chunks_doc_index" {
    unique  = true
    columns = [column.document_id, column.chunk_index, column.is_del]
  }
}
table "ai_knowledge_documents" {
  schema  = schema.admin
  comment = "AI知识库文档"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "文档ID"
    auto_increment = true
  }
  column "knowledge_base_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_knowledge_bases.id"
  }
  column "title" {
    null    = false
    type    = varchar(191)
    comment = "文档标题，列表、分块、监控展示"
  }
  column "source_type" {
    null    = false
    type    = varchar(32)
    default = "text"
    comment = "来源类型：text/markdown/file；第一版写 text/markdown"
  }
  column "source_ref" {
    null    = false
    type    = varchar(512)
    default = ""
    comment = "来源标识，如 docs/architecture/04-go-backend-framework.md 或上传文件URL；与 knowledge_base_id、is_del 组成同来源幂等唯一键"
  }
  column "content" {
    null    = false
    type    = longtext
    comment = "文档原文，编辑和重建分块使用"
  }
  column "index_status" {
    null    = false
    type    = varchar(16)
    default = "pending"
    comment = "pending/indexing/indexed/failed；分块状态展示和运行过滤"
  }
  column "error_message" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "分块失败原因，管理页展示"
  }
  column "last_indexed_at" {
    null    = true
    type    = datetime
    comment = "最近成功重建分块时间"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1启用 2禁用；运行时只读取启用文档"
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
  index "idx_ai_knowledge_documents_base" {
    columns = [column.knowledge_base_id, column.status, column.is_del, column.updated_at]
  }
  index "idx_ai_knowledge_documents_index" {
    columns = [column.index_status, column.is_del]
  }
  index "uk_ai_knowledge_documents_source" {
    unique  = true
    columns = [column.knowledge_base_id, column.source_ref, column.is_del]
  }
}
table "ai_knowledge_retrieval_hits" {
  schema  = schema.admin
  comment = "AI知识库检索命中"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "命中ID"
    auto_increment = true
  }
  column "retrieval_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_knowledge_retrievals.id"
  }
  column "knowledge_base_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "命中知识库ID"
  }
  column "knowledge_base_name" {
    null    = false
    type    = varchar(128)
    comment = "命中时知识库名称快照"
  }
  column "document_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "命中文档ID"
  }
  column "document_title" {
    null    = false
    type    = varchar(191)
    comment = "命中时文档标题快照"
  }
  column "chunk_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "命中分块ID"
  }
  column "chunk_index" {
    null     = false
    type     = int
    unsigned = true
    comment  = "命中分块序号快照"
  }
  column "score" {
    null     = false
    type     = decimal(10, 6)
    default  = 0
    unsigned = false
    comment  = "检索评分"
  }
  column "rank_no" {
    null     = false
    type     = int
    unsigned = true
    comment  = "本次检索排序，从1开始"
  }
  column "content_snapshot" {
    null    = false
    type    = text
    comment = "命中内容快照，运行监控和问题复盘使用"
  }
  column "status" {
    null     = false
    type     = tinyint
    default  = 1
    unsigned = true
    comment  = "1进入上下文 2跳过"
  }
  column "skip_reason" {
    null    = false
    type    = varchar(64)
    default = ""
    comment = "跳过原因：low_score/context_limit"
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
  index "idx_ai_knowledge_hits_chunk" {
    columns = [column.chunk_id, column.is_del]
  }
  index "idx_ai_knowledge_hits_retrieval" {
    columns = [column.retrieval_id, column.status, column.rank_no]
  }
}
table "ai_knowledge_retrievals" {
  schema  = schema.admin
  comment = "AI知识库检索记录"
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    comment        = "检索ID"
    auto_increment = true
  }
  column "run_id" {
    null     = false
    type     = bigint
    unsigned = true
    comment  = "ai_runs.id"
  }
  column "query" {
    null    = false
    type    = text
    comment = "本轮检索查询文本，通常为用户消息正文"
  }
  column "status" {
    null    = false
    type    = varchar(16)
    comment = "success/failed/skipped"
  }
  column "total_hits" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "原始命中数量"
  }
  column "selected_hits" {
    null     = false
    type     = int
    default  = 0
    unsigned = true
    comment  = "进入上下文的命中数量"
  }
  column "duration_ms" {
    null     = true
    type     = int
    unsigned = true
    comment  = "检索耗时毫秒"
  }
  column "error_message" {
    null    = false
    type    = varchar(1024)
    default = ""
    comment = "失败原因"
  }
  column "is_del" {
    null     = false
    type     = tinyint
    default  = 2
    unsigned = true
    comment  = "1删除 2正常；运行监控默认只读正常记录"
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
  index "idx_ai_knowledge_retrievals_run" {
    columns = [column.run_id, column.is_del, column.created_at]
  }
  index "idx_ai_knowledge_retrievals_status" {
    columns = [column.status, column.is_del, column.created_at]
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
  index "uk_ai_messages_reply_command" {
    unique  = true
    columns = [column.reply_command_id]
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
table "ai_provider_attempts" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    unsigned       = true
    auto_increment = true
  }
  column "command_id" {
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
  column "dispatched_at" {
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
  index "idx_ai_attempt_state" {
    columns = [column.state, column.id]
  }
  index "uk_ai_attempt_command_no" {
    unique  = true
    columns = [column.command_id, column.attempt_no]
  }
  index "uk_ai_attempt_key" {
    unique  = true
    columns = [column.idempotency_key]
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
  column "display_name" {
    null    = false
    type    = varchar(191)
    default = ""
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
  index "idx_ai_provider_models_provider_status" {
    columns = [column.provider_id, column.status]
  }
  index "uk_ai_provider_models_provider_model" {
    unique  = true
    columns = [column.provider_id, column.model_id]
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
    null = false
    type = varchar(128)
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
    null = false
    type = bigint
  }
  column "conversation_id" {
    null = false
    type = bigint
  }
  column "user_message_id" {
    null = false
    type = bigint
  }
  column "assistant_message_id" {
    null = true
    type = bigint
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
  index "idx_ai_reply_claim" {
    columns = [column.state, column.next_attempt_at, column.lease_expires_at, column.id]
  }
  index "uk_ai_reply_idempotency" {
    unique  = true
    columns = [column.idempotency_key]
  }
  index "uk_ai_reply_message" {
    unique  = true
    columns = [column.user_message_id]
  }
  index "uk_ai_reply_request" {
    unique  = true
    columns = [column.conversation_id, column.request_id]
  }
  check "chk_ai_reply_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_reply_state" {
    expr = "(`state` in (_utf8mb4'pending',_utf8mb4'claimed',_utf8mb4'running',_utf8mb4'succeeded',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'outcome_unknown',_utf8mb4'timed_out'))"
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
    comment = "start/completed/failed/canceled/timeout"
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
    expr = "(`event_type` in (_utf8mb4'start',_utf8mb4'completed',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout'))"
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
    comment = "client request identifier"
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
  column "idempotency_key" {
    null = true
    type = varchar(128)
  }
  column "status" {
    null    = false
    type    = varchar(16)
    comment = "queued/running/success/failed/canceled/timeout"
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
  index "idx_ai_runs_conversation_created" {
    columns = [column.conversation_id, column.created_at, column.id]
  }
  index "idx_ai_runs_created" {
    columns = [column.created_at, column.id]
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
  index "uk_ai_runs_conversation_request" {
    unique  = true
    columns = [column.conversation_id, column.request_id]
  }
  index "uk_ai_runs_idempotency" {
    unique  = true
    columns = [column.idempotency_key]
  }
  index "uk_ai_runs_user_message" {
    unique  = true
    columns = [column.user_message_id]
  }
  check "chk_ai_runs_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_runs_status" {
    expr = "(`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed',_utf8mb4'canceled',_utf8mb4'timeout'))"
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
  index "idx_ai_text_tasks_status_created" {
    columns = [column.status, column.created_at, column.id]
  }
  index "idx_ai_text_tasks_user_created" {
    columns = [column.user_id, column.created_at, column.id]
  }
  check "chk_ai_text_tasks_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
  }
  check "chk_ai_text_tasks_status" {
    expr = "(`status` in (_utf8mb4'running',_utf8mb4'success',_utf8mb4'failed'))"
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
table "ai_video_tasks" {
  schema = schema.admin
  column "id" {
    null           = false
    type           = bigint
    auto_increment = true
  }
  column "platform" {
    null = false
    type = varchar(32)
  }
  column "user_id" {
    null = false
    type = bigint
  }
  column "agent_id" {
    null = false
    type = bigint
  }
  column "provider_id" {
    null = false
    type = bigint
  }
  column "model_id" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "prompt" {
    null = false
    type = text
  }
  column "duration_seconds" {
    null    = false
    type    = int
    default = 0
  }
  column "size" {
    null    = false
    type    = varchar(32)
    default = ""
  }
  column "resolution_name" {
    null    = false
    type    = varchar(64)
    default = ""
  }
  column "provider_task_id" {
    null    = false
    type    = varchar(191)
    default = ""
  }
  column "run_id" {
    null    = false
    type    = bigint
    default = 0
  }
  column "status" {
    null = false
    type = varchar(32)
  }
  column "error_message" {
    null    = false
    type    = varchar(1024)
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
  column "finished_at" {
    null = true
    type = datetime
  }
  primary_key {
    columns = [column.id]
  }
  index "idx_ai_video_provider_task" {
    columns = [column.provider_id, column.provider_task_id]
  }
  index "idx_ai_video_status_created" {
    columns = [column.status, column.is_del, column.created_at, column.id]
  }
  index "idx_ai_video_user_created" {
    columns = [column.user_id, column.is_del, column.created_at, column.id]
  }
  check "chk_ai_video_platform" {
    expr = "((`platform` regexp _utf8mb4'^[a-z][a-z0-9_]{1,48}$') and (`platform` not in (_utf8mb4'app',_utf8mb4'canvas')) and (`platform` <> _utf8mb4'all'))"
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
  column "balance_cents" {
    null    = false
    type    = bigint
    default = 0
  }
  column "total_recharge_cents" {
    null    = false
    type    = bigint
    default = 0
  }
  column "total_consume_cents" {
    null    = false
    type    = bigint
    default = 0
    comment = "累计消费金额，单位分"
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
  column "amount_cents" {
    null = false
    type = bigint
  }
  column "balance_before_cents" {
    null = false
    type = bigint
  }
  column "balance_after_cents" {
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
}
schema "admin" {
  charset = "utf8mb4"
  collate = "utf8mb4_0900_ai_ci"
}
