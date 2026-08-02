# AI 上下文工程核心契约 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立九表 Expand、闭合 Context 领域类型、typed Provider 请求、可证明 Token Budget/Packer/Hash，以及 Plan 与 Provider Attempt 的持久证据关系，同时保持现有纯聊天、附件和恢复行为。

**Architecture:** 本 Slice 只增加事实和内部契约，不启用 Qdrant、不替换现有 KnowledgeRuntime，也不执行破坏性迁移。无约束 `ChatInput.Inputs` 被闭合 Message/Part/Attachment 与显式生成参数替代；Context Plan Repository 只接受一次性终局写入，后续 Slice 在报价前调用它。

**Tech Stack:** Go 1.26.5、GORM、MySQL 8.4、Atlas HCL/SQL、SHA-256、Go property/fuzz tests。

---

## Fixed File Map

```text
database/migrations/202608020101_ai_context_expand.sql   Expand only
database/schema/admin.hcl                               canonical expanded schema
internal/module/ai/contextengine/model.go               GORM persistence rows
internal/module/ai/contextengine/types.go               closed domain contracts
internal/module/ai/contextengine/errors.go              stable ai.context.* errors
internal/module/ai/contextengine/repository.go           immutable terminal Plan persistence
internal/module/ai/contextengine/hash.go                 canonical fingerprints and plan hash
internal/module/ai/contextengine/packer.go               deterministic atomic budget selection
internal/infra/ai/tokencounter.go                        registered conservative token bounds
internal/infra/ai/types.go                               typed ChatInput/Message/ContentPart
```

The module chain remains `route -> handler -> service -> repository -> model`. `contextengine` must not import Gin, Qdrant, provider-specific request structs, wallet code, or runtime composition.

### Task 1: Add the guarded nine-table Expand contract

**Files:**
- Create: `database/migrations/202608020101_ai_context_expand.sql`
- Modify: `database/schema/admin.hcl`
- Modify: `database/migrations/atlas.sum`
- Create: `internal/architecture/ai_context_schema_contract_test.go`
- Create: `scripts/tests/ai-context-expand.tests.ps1`

- [ ] **Step 1: Write the failing schema inventory test**

Add a repository test that reads the migration and HCL and locks the exact new table set plus the three existing-table expansions. Use these helpers in the same `architecture` package (imports: `os`, `path/filepath`, `reflect`, `regexp`, `sort`, `strings`, `testing`):

```go
var createTableNameRE = regexp.MustCompile("(?i)\\bCREATE\\s+TABLE\\s+`([^`]+)`")

func mustReadRepoFile(t *testing.T, relativePath string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(backendRoot(t), filepath.FromSlash(relativePath)))
	if err != nil { t.Fatal(err) }
	return string(body)
}

func sortedCreateTableNames(
	t *testing.T,
	migration string,
	prefix string,
	exact []string,
) []string {
	t.Helper()
	exactNames := make(map[string]struct{}, len(exact))
	for _, name := range exact { exactNames[name] = struct{}{} }
	seen := make(map[string]struct{})
	var names []string
	for _, match := range createTableNameRE.FindAllStringSubmatch(migration, -1) {
		name := match[1]
		_, exactMatch := exactNames[name]
		if !strings.HasPrefix(name, prefix) && !exactMatch { continue }
		if _, duplicate := seen[name]; duplicate { t.Fatalf("duplicate CREATE TABLE %s", name) }
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestAIContextExpandOwnsExactlyNineContextTables(t *testing.T) {
	want := []string{
		"ai_context_bindings", "ai_context_chunks", "ai_context_document_versions",
		"ai_context_documents", "ai_context_plan_items", "ai_context_plans",
		"ai_context_profiles", "ai_context_spaces", "ai_conversation_memories",
	}
	migration := mustReadRepoFile(t, "database/migrations/202608020101_ai_context_expand.sql")
	if got := sortedCreateTableNames(t, migration, "ai_context_", []string{"ai_conversation_memories"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("context tables = %v, want %v", got, want)
	}
	for _, forbidden := range []string{"ai_context_citations", "ai_context_retrievals", "ai_context_hits", "ai_context_jobs", "ai_context_cursors"} {
		if strings.Contains(migration, "`"+forbidden+"`") { t.Fatalf("forbidden table %s", forbidden) }
	}
	for _, fragment := range []string{
		"`ai_provider_models`", "`model_kind`", "`ai_agents`", "`context_profile_id`",
		"`ai_provider_attempts`", "`context_plan_id`", "`context_plan_sha256`",
	} {
		if !strings.Contains(migration, fragment) { t.Fatalf("expand missing %s", fragment) }
	}
}
```

Add a second test locking the forbidden legacy reference directly:

```go
func TestAIContextExpandDoesNotRewriteLegacyKnowledgeMigration(t *testing.T) {
	migration := mustReadRepoFile(t, "database/migrations/202608020101_ai_context_expand.sql")
	if strings.Contains(migration, "20260510_ai_knowledge_rag.sql") ||
		strings.Contains(migration, "ai_knowledge_") {
		t.Fatal("Expand must not read, copy, rename, or mutate legacy Knowledge tables")
	}
}
```

Use the existing `hclTableBlock` helper in package `architecture` and lock the
canonical HCL independently from the migration. This test must enumerate every
column from the inventory below; a missing or extra column is a failure:

```go
var hclColumnNameRE = regexp.MustCompile(`(?m)^\s*column "([^"]+)" \{`)

func TestAIContextExpandCanonicalHCLMatchesExactInventory(t *testing.T) {
	schema := mustReadRepoFile(t, "database/schema/admin.hcl")
	want := map[string][]string{
		"ai_context_profiles": {
			"id", "name", "embedding_provider_model_id", "embedding_dimensions",
			"embedding_max_input_tokens", "embedding_token_counter_id", "dense_distance",
			"dense_min_score", "sparse_encoder", "sparse_encoder_version",
			"reranker_provider_model_id", "reranker_min_score", "memory_provider_model_id",
			"status", "active_index_generation", "target_index_generation", "index_state",
			"index_error_code", "index_verified_at", "created_by", "created_at", "updated_at",
		},
		"ai_context_spaces": {
			"id", "platform", "profile_id", "name", "description", "status", "deleted_at",
			"created_by", "created_at", "updated_at",
		},
		"ai_context_documents": {
			"id", "space_id", "conversation_id", "source_message_id",
			"source_attachment_index", "title", "active_version_id", "status", "deleted_at",
			"created_by", "created_at", "updated_at",
		},
		"ai_context_document_versions": {
			"id", "document_id", "profile_id", "source_storage_provider", "source_object_key",
			"source_etag", "source_size_bytes", "source_mime_type", "source_filename",
			"source_facts_sha256", "source_sha256", "parser_name", "parser_version",
			"chunker_version", "state", "failure_stage", "error_code", "error_message",
			"chunk_count", "embedding_input_token_upper_bound", "embedding_request_count",
			"embedding_input_tokens", "started_at", "finished_at", "attempt_count",
			"lease_token", "lease_expires_at", "created_at", "updated_at",
		},
		"ai_context_chunks": {
			"id", "document_version_id", "ordinal", "heading_path", "content",
			"content_sha256", "chunk_facts_sha256", "embedding_input_token_upper_bound",
			"locator_json", "created_at",
		},
		"ai_context_bindings": {
			"id", "agent_id", "space_id", "status", "created_at", "updated_at",
		},
		"ai_context_plans": {
			"id", "run_id", "context_profile_id_snapshot", "context_profile_sha256",
			"context_index_generation_snapshot", "policy_version", "input_fingerprint_sha256",
			"plan_sha256", "model_capability_sha256", "api_protocol_snapshot",
			"token_counter_id_snapshot", "context_window_tokens", "effective_output_tokens",
			"provider_protocol_upper_bound", "tool_continuation_input_reserve",
			"policy_safety_margin", "known_input_budget", "known_input_upper_bound",
			"budget_proof", "retrieval_outcome", "state", "error_stage", "error_code",
			"error_message", "metrics_json", "created_at",
		},
		"ai_context_plan_items": {
			"id", "plan_id", "ordinal", "block_kind", "source_type", "source_ref",
			"source_sha256", "atomic_group_key", "required", "priority", "decision",
			"exclusion_reason", "token_upper_bound", "fusion_score", "rerank_score",
			"citation_key", "content_snapshot", "metadata_json", "created_at",
		},
		"ai_conversation_memories": {
			"id", "conversation_id", "context_profile_id_snapshot", "context_profile_sha256",
			"previous_memory_id", "from_message_id", "through_message_id", "source_sha256",
			"summary_sha256", "policy_version", "summary", "prompt_tokens",
			"completion_tokens", "provider_request_id", "state", "error_code", "created_at",
		},
	}
	for tableName, columns := range want {
		block := hclTableBlock(t, schema, tableName)
		matches := hclColumnNameRE.FindAllStringSubmatch(block, -1)
		got := make([]string, 0, len(matches))
		for _, match := range matches { got = append(got, match[1]) }
		if !reflect.DeepEqual(got, columns) {
			t.Errorf("%s columns = %v, want %v", tableName, got, columns)
		}
	}
	for tableName, markers := range map[string][]string{
		"ai_context_profiles": {`foreign_key "fk_ai_context_profiles_embedding_model"`, `check "chk_ai_context_profiles_generation_shape"`},
		"ai_context_documents": {`foreign_key "fk_ai_context_documents_active_version"`, `check "chk_ai_context_documents_owner_source"`},
		"ai_context_document_versions": {`check "chk_ai_context_document_versions_terminal_shape"`},
		"ai_context_plans": {`index "uk_ai_context_plans_run"`, `check "chk_ai_context_plans_terminal_shape"`, `check "chk_ai_context_plans_budget"`},
		"ai_context_plan_items": {`check "chk_ai_context_plan_items_decision"`, `check "chk_ai_context_plan_items_content_snapshot"`},
		"ai_conversation_memories": {`check "chk_ai_conversation_memories_terminal_shape"`},
	} {
		block := hclTableBlock(t, schema, tableName)
		for _, marker := range markers {
			if !strings.Contains(block, marker) { t.Errorf("%s missing %s", tableName, marker) }
		}
	}
}
```

- [ ] **Step 2: Run the schema test and verify RED**

Run: `go test ./internal/architecture -run AIContextExpand -count=1`

Expected: FAIL because `202608020101_ai_context_expand.sql` and the nine HCL tables do not exist.

- [ ] **Step 3: Write Expand SQL and canonical HCL**

Create the nine tables with the exact business columns from design sections 8.1-8.9. Use the repository's existing unsigned `BIGINT` IDs, `DATETIME(6)`, `utf8mb4_0900_ai_ci`, soft-delete convention only where the design declares `deleted_at`, and real foreign keys. Do not abbreviate any table definition. The migration and HCL must contain this exact column inventory:

```text
ai_context_profiles:
  id, name, embedding_provider_model_id, embedding_dimensions,
  embedding_max_input_tokens, embedding_token_counter_id, dense_distance,
  dense_min_score, sparse_encoder, sparse_encoder_version,
  reranker_provider_model_id NULL, reranker_min_score NULL,
  memory_provider_model_id NULL, status, active_index_generation NULL,
  target_index_generation NULL, index_state, index_error_code NULL,
  index_verified_at NULL, created_by, created_at, updated_at

ai_context_spaces:
  id, platform, profile_id, name, description, status, deleted_at NULL,
  created_by, created_at, updated_at

ai_context_documents:
  id, space_id NULL, conversation_id NULL, source_message_id NULL,
  source_attachment_index NULL, title, active_version_id NULL, status,
  deleted_at NULL, created_by, created_at, updated_at

ai_context_document_versions:
  id, document_id, profile_id, source_storage_provider, source_object_key,
  source_etag, source_size_bytes, source_mime_type, source_filename,
  source_facts_sha256, source_sha256 NULL, parser_name, parser_version,
  chunker_version, state, failure_stage NULL, error_code NULL,
  error_message NULL, chunk_count, embedding_input_token_upper_bound,
  embedding_request_count, embedding_input_tokens NULL, started_at NULL,
  finished_at NULL, attempt_count, lease_token NULL, lease_expires_at NULL,
  created_at, updated_at

ai_context_chunks:
  id, document_version_id, ordinal, heading_path, content, content_sha256,
  chunk_facts_sha256, embedding_input_token_upper_bound, locator_json,
  created_at

ai_context_bindings:
  id, agent_id, space_id, status, created_at, updated_at

ai_context_plans:
  id, run_id, context_profile_id_snapshot NULL,
  context_profile_sha256 NULL, context_index_generation_snapshot NULL,
  policy_version, input_fingerprint_sha256, plan_sha256 NULL,
  model_capability_sha256, api_protocol_snapshot,
  token_counter_id_snapshot, context_window_tokens,
  effective_output_tokens, provider_protocol_upper_bound,
  tool_continuation_input_reserve, policy_safety_margin,
  known_input_budget, known_input_upper_bound, budget_proof,
  retrieval_outcome, state, error_stage NULL, error_code NULL,
  error_message NULL, metrics_json, created_at

ai_context_plan_items:
  id, plan_id, ordinal, block_kind, source_type, source_ref,
  source_sha256, atomic_group_key, required, priority, decision,
  exclusion_reason NULL, token_upper_bound, fusion_score NULL,
  rerank_score NULL, citation_key NULL, content_snapshot NULL,
  metadata_json, created_at

ai_conversation_memories:
  id, conversation_id, context_profile_id_snapshot,
  context_profile_sha256, previous_memory_id NULL, from_message_id,
  through_message_id, source_sha256, summary_sha256 NULL, policy_version,
  summary NULL, prompt_tokens NULL, completion_tokens NULL,
  provider_request_id NULL, state, error_code NULL, created_at
```

Create the canonical tables with the following executable SQL; reproduce the same types, defaults, indexes, foreign keys and CHECK expressions in `database/schema/admin.hcl`:

```sql
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
```

```sql
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
```

```sql
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
```

MySQL 8.4 rejects any `CHECK` that references an `AUTO_INCREMENT` column, so
`previous_memory_id <> id` cannot be expressed in this table constraint. Keep
the executable interval CHECK above. Plan 04 Task 5 must reject preassigned new
Memory IDs and self-parenting, and must validate the locked latest parent,
Profile and continuous interval in the Memory Repository before every insert.
Do not replace that domain rule with a trigger or a different ID allocator.

The same migration must expand existing tables exactly as follows:

```sql
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
```

`DEFAULT 'chat'` is a temporary Expand compatibility bridge: an old binary may still insert `ai_provider_models` after this migration but before the new application is cut over. New code must always write an explicit closed kind. Plan 05 removes the default only after old API/Worker processes and queued commands are drained; leaving the temporary default forever is not accepted.

Add these high-value constraints in both SQL and HCL:

```text
profiles: reranker id and threshold are both NULL or both non-NULL
profiles: generation/state NULL combinations match the four legal CAS shapes
documents: exactly one of space_id/conversation_id is non-NULL
documents: conversation source message/index are both NULL or both non-NULL
versions: terminal state/output/error/lease columns are coherent
plans: ready has hash/no error; failed has NULL hash and stable error
plans: known_input_budget and known_input_upper_bound prove the budget inequality
plan_items: selected/excluded and citation_key rules are coherent
memories: ready/failed/invalidated output and error columns are coherent
```

Use composite uniqueness required by the design: Document/Version ownership, `(document_version_id, ordinal)`, `(agent_id, space_id)`, Plan `run_id`, `(plan_id, ordinal)`, non-null Plan citation key, and Memory identity. The existing Provider Model unique key must be replaced, not retained: keeping `(provider_id, model_id)` would make the new three-part identity impossible. Do not add status, retrieval, hit, job or cursor tables.

- [ ] **Step 4: Hash, then validate HCL/SQL shape without applying it**

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`

Expected: `database/migrations/atlas.sum` changes only by the new migration checksum. Atlas must hash the new file before it validates the directory.

Run: `go test ./internal/architecture -run AIContextExpand -count=1`

Expected: PASS; the test reports exactly nine new Context tables and all three existing-table expansions.

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations`

Expected: PASS; Atlas validates migration order and SQL syntax. This command validates files only and must not receive a database URL.

Run: `pwsh -NoProfile -File scripts/tests/ai-context-expand.tests.ps1 -BaselineCommit 6fcc5248a22d3cb4dc4f09e99665ff697be7e6c5`

Expected: PASS against an owned disposable `mysql:8.4.10` container. The script
must verify the baseline commit is an ancestor, apply that commit's canonical
HCL to `admin_context_before_<12hex>`, apply only migration `202608020101` with
the MySQL client, apply the immutable Expand HCL independently to
`admin_context_after_<12hex>`, and compare the two `admin-db fingerprint`
documents including columns, indexes, foreign keys and CHECK expressions. Before
the migration is committed, the Expand HCL is the worktree HCL; afterward it is
the HCL from the unique commit that introduced `202608020101`. Plan 05's cutover
script separately proves the final HCL after all three migrations. This script
must also prove an excluded Plan Item with `exclusion_reason=NULL` and an
uppercase Space platform are rejected, publish no host port beyond loopback,
and remove only its labeled container and two regex-validated schemas in
`finally`. It must never read `admin-go.env`, use the local development database,
or apply a migration to a user schema.

Run: `pwsh -NoProfile -File scripts/verify-database.ps1 -Mode empty`

Expected: PASS; the complete current HCL applies cleanly to the verifier-owned
disposable schema. This is not a substitute for the migration convergence test
above; both commands are required.

- [ ] **Step 5: Commit the schema checkpoint**

```bash
git add -- database/migrations/202608020101_ai_context_expand.sql database/migrations/atlas.sum database/schema/admin.hcl internal/architecture/ai_context_schema_contract_test.go scripts/tests/ai-context-expand.tests.ps1
git commit -m "feat(ai): add context engineering schema"
```

### Task 2: Define closed domain types and immutable Plan persistence

**Files:**
- Create: `internal/module/ai/contextengine/model.go`
- Create: `internal/module/ai/contextengine/types.go`
- Create: `internal/module/ai/contextengine/errors.go`
- Create: `internal/module/ai/contextengine/repository.go`
- Create: `internal/module/ai/contextengine/repository_test.go`
- Create: `internal/module/ai/contextengine/types_test.go`

- [ ] **Step 1: Write failing enum, validation and persistence tests**

Lock valid states and the one-Plan rule:

```go
func TestContextPlanValidateTerminalShape(t *testing.T) {
	ready := validReadyPlan()
	if err := ready.Validate(); err != nil { t.Fatal(err) }
	failed := ready
	failed.State, failed.RetrievalOutcome = PlanFailed, RetrievalFailed
	failed.PlanSHA256 = nil
	failed.ErrorStage, failed.ErrorCode = "retrieval", ErrCodeRetrievalFailed
	if err := failed.Validate(); err != nil { t.Fatal(err) }
	failed.PlanSHA256 = bytes.Repeat([]byte{1}, sha256.Size)
	if err := failed.Validate(); err == nil { t.Fatal("failed plan must not have a hash") }
}

func TestPersistTerminalPlanConcurrentLoserReloadsWinner(t *testing.T) {
	repository, guard := newPlanRepositoryFixture(t)
	left := validReadyPlanForRun(44, "left")
	right := validReadyPlanForRun(44, "right")
	results := make(chan ContextPlan, 2)
	errors := make(chan error, 2)
	for _, plan := range []ContextPlan{left, right} {
		plan := plan
		go func() {
			token := PlanCommitToken{
				RunID: plan.RunID, ReplyCommandID: 77,
				LeaseOwner: "worker-a", LeaseToken: 3,
				InputFingerprintSHA256: plan.InputFingerprintSHA256,
				AuthoritySnapshotSHA256: sha256.Sum256([]byte("authority")),
			}
			got, _, err := repository.PersistTerminal(t.Context(), plan, guard, token)
			results <- got
			errors <- err
		}()
	}
	first, second := <-results, <-results
	if err := <-errors; err != nil { t.Fatal(err) }
	if err := <-errors; err != nil { t.Fatal(err) }
	if first.ID == 0 || first.ID != second.ID || first.PlanSHA256 == nil ||
		second.PlanSHA256 == nil || *first.PlanSHA256 != *second.PlanSHA256 {
		t.Fatalf("concurrent callers returned different terminal plans: %#v %#v", first, second)
	}
}
```

Implement `newPlanRepositoryFixture` with the repository's existing MySQL integration-test harness and a transaction guard that records the exact `*gorm.DB` passed by the repository. Add table tests for illegal Profile generation transitions, unknown Block Kind, invalid Citation placement, empty-string-as-null, non-32-byte hashes, negative token bounds, and a failed Plan carrying Items.

- [ ] **Step 2: Run contextengine tests and verify RED**

Run: `go test ./internal/module/ai/contextengine -run 'Validate|PersistTerminal' -count=1`

Expected: FAIL because the package and contracts do not exist.

- [ ] **Step 3: Implement closed contracts**

Define string types with `Validate` methods and no unknown-value default branch. The central contracts are:

```go
type ContextPlan struct {
	ID                       uint64
	RunID                    uint64
	Profile                  *ProfileSnapshot
	PolicyVersion            string
	InputFingerprintSHA256   [32]byte
	PlanSHA256               *[32]byte
	ModelCapabilitySHA256    [32]byte
	APIProtocol              string
	TokenCounterID           string
	Budget                   Budget
	RetrievalOutcome         RetrievalOutcome
	State                    PlanState
	Error                    *PlanError
	Metrics                  ContextPlanMetricsV1
	Items                    []ContextPlanItem
}

type ContextPlanItem struct {
	Ordinal         uint32
	Block           ContextBlock
	Decision        Decision
	ExclusionReason *ExclusionReason
	FusionScore     *FixedScore
	RerankScore     *FixedScore
	CitationKey     *string
}
```

`FixedScore` accepts a decimal string normalized to six fractional digits, rejects NaN/Infinity at the adapter boundary, and supplies the same value for sorting, JSON persistence and hashing. JSON payloads use versioned structs (`ContextPlanMetricsV1`, `ContextBlockMetadataV1`, `ContextLocatorV1`, `RetrievalBranchesV1`), never `map[string]any`.

Expose only stable error codes from design section 15. Construct `*apperror.Error` with safe user text; raw adapter errors remain wrapped causes and never enter persisted `error_message`.

- [ ] **Step 4: Implement one terminal repository operation**

Expose a narrow API:

```go
type PlanRepository interface {
	FindTerminalByRunID(context.Context, uint64) (*ContextPlan, error)
	PersistTerminal(context.Context, ContextPlan, PlanCommitTransactionGuard, PlanCommitToken) (ContextPlan, PersistDisposition, error)
}

type PlanCommitTransactionGuard interface {
	GuardPlanCommitInTransaction(context.Context, *gorm.DB, PlanCommitToken) (PlanCommitGuardResult, error)
}

type PlanCommitGuardResult struct {
	SnapshotConflict *PlanError
}

type PlanCommitToken struct {
	RunID                   uint64
	ReplyCommandID          uint64
	LeaseOwner              string
	LeaseToken              uint64
	InputFingerprintSHA256  [32]byte
	AuthoritySnapshotSHA256 [32]byte
}

var ErrPlanCommitAborted = errors.New("context plan commit aborted by run state or lease")

const (
	PersistCreated        PersistDisposition = "created"
	PersistLoadedExisting PersistDisposition = "loaded_existing"
)
```

`PersistTerminal` validates the entire aggregate before opening a transaction and rejects a nil guard. It opens one transaction, then calls `GuardPlanCommitInTransaction` with that same non-nil `*gorm.DB`; the guard owns the `FOR UPDATE` locks on Run/Reply and all commit-time authority checks. Cancellation, timeout or lease loss returns `ErrPlanCommitAborted`, rolls back and writes no Plan. A true identity/content/authorization mismatch returns a non-nil `PlanCommitGuardResult.SnapshotConflict` with stable code `ai.context.snapshot_conflict`; the repository converts the candidate to a failed aggregate by clearing Plan Hash and Items, setting `retrieval_outcome=failed`, and persists that failed header in the same transaction. Any database/adapter error rolls back without inventing a failed business fact.

Slice 1 implements and tests this interface boundary but does not activate Context runtime. Plan 03 supplies the concrete MySQL guard without changing the repository signature or opening a second transaction around it. After an allowed result, the repository inserts Plan and Items in ordinal order and commits once. On duplicate `run_id`, it rolls back the losing transaction, discards the caller's external result, and reloads the committed winner with a fresh read. It never compares a failed Plan's NULL hash and never updates an existing Plan.

- [ ] **Step 5: Run tests and commit the domain checkpoint**

Run: `go test ./internal/module/ai/contextengine -run 'Validate|PersistTerminal' -count=1`

Expected: PASS, including concurrent winner/loser and failed-hash tests.

```bash
git add -- internal/module/ai/contextengine/model.go internal/module/ai/contextengine/types.go internal/module/ai/contextengine/errors.go internal/module/ai/contextengine/repository.go internal/module/ai/contextengine/repository_test.go internal/module/ai/contextengine/types_test.go
git commit -m "feat(ai): add immutable context plan contracts"
```

### Task 3: Replace implicit ChatInput map keys with typed messages

**Files:**
- Modify: `internal/infra/ai/types.go`
- Create: `internal/infra/ai/chat_input_test.go`
- Modify: `internal/infra/ai/openaicompat/client.go`
- Modify: `internal/infra/ai/openaicompat/responses.go`
- Modify: `internal/infra/ai/openaicompat/client_test.go`
- Modify: `internal/infra/ai/openaicompat/responses_test.go`
- Modify: `internal/infra/ai/openaicompat/file_manifest_test.go`
- Modify: `internal/module/ai/chat/service.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/runtime/ai_billing_gateway_test.go`
- Modify: `internal/runtime/ai_billing_text.go`
- Modify: `internal/runtime/ai_billing_finalizer.go`
- Modify: `internal/runtime/ai_billing_finalizer_test.go`
- Modify: `internal/runtime/providers_test.go`

- [ ] **Step 1: Write failing invalid-state and semantic request tests**

Add tests proving that Text and Attachment cannot coexist in one Part, unknown roles/kinds fail, current text can be empty only when a valid attachment exists, historical attachments survive compilation, and both API protocols retain the same role/part order:

```go
func TestChatInputRejectsAmbiguousPart(t *testing.T) {
	input := ChatInput{ModelID: "gpt-test", Messages: []Message{{
		Role: MessageRoleUser,
		Parts: []ContentPart{{Kind: ContentPartText, Text: "hello", Attachment: &AttachmentRef{Kind: AttachmentFile}}},
	}}}
	if err := input.Validate(); err == nil { t.Fatal("ambiguous content part must fail") }
}
```

In OpenAI-compatible tests decode prepared JSON and assert the exact message roles, text, image parts and file references instead of comparing implementation maps.

- [ ] **Step 2: Run typed request tests and verify RED**

Run: `go test ./internal/infra/ai ./internal/infra/ai/openaicompat ./internal/module/ai/chat ./internal/runtime -run 'ChatInput|ChatMessages|FileManifest|PreparedChat' -count=1`

Expected: FAIL because `ChatInput` still exposes `Content` and `Inputs map[string]any`.

- [ ] **Step 3: Define typed request data**

Replace `Content` and `Inputs` with explicit fields:

```go
type ChatInput struct {
	AttemptID                uint64
	IdempotencyKey           string
	AgentID                  uint64
	RunID                    uint64
	UserID                   uint64
	UserKey                  string
	ModelID                  string
	Messages                 []Message
	Temperature              *float64
	ConversationEngineID     string
	EffectiveMaxOutputTokens int
	Tools                    []ToolDefinition
	ToolCalls                []ToolCall
	ToolOutputs              []ToolOutput
	Continuation             *ChatContinuation
}

type AttachmentRef struct {
	Kind      AttachmentKind
	URL       string
	ObjectKey string
	ETag      string
	Size      int64
	MIMEType  string
	Filename  string
}
```

Closed values are `MessageRoleSystem|User|Assistant`, `ContentPartText|Attachment`, and `AttachmentImage|File`. Preserve the existing image contract: a trusted non-empty URL is sufficient, and an optional MIME value must be an image MIME when present. Requiring MIME for images would reject persisted historical attachments that are valid today. Native File requires object key, ETag, positive size, MIME and filename. Constructors normalize only syntax (trim enum/text fields); they do not invent missing values.

- [ ] **Step 4: Compile typed messages for both protocols**

Change `prepareChatMessages` and Responses preparation to iterate `input.Messages` in order. A System Message compiles to system/instructions, User and Assistant Messages keep role and ordered parts, and every Attachment becomes either an image content part or a `PreparedFileRef`. Remove `inputString`, `historyMessages`, `hasCurrentAttachments` and every compiler read of `system_prompt`, `history`, `attachments`, `model_id`, `temperature` or `max_tokens` map keys.

Change `chat.service` to construct typed system/history/current Messages from persisted rows without changing persisted message text. Keep existing history count behavior in this Slice; Plan 04 removes its effective `max_history` limit.

Change `paidChatAssembler` to set `ModelID` and `EffectiveMaxOutputTokens` on a typed copy. `clonePaidChatInput` deep-copies Messages, Parts, Attachment values, Tools, ToolCalls and ToolOutputs. Recovery of an existing prepared Attempt continues to load persisted request bytes and never recompiles them.

- [ ] **Step 5: Run semantic regression and commit**

Run: `gofmt -w internal/infra/ai internal/module/ai/chat internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/runtime/ai_billing_text.go internal/runtime/ai_billing_finalizer.go internal/runtime/ai_billing_finalizer_test.go internal/runtime/providers_test.go`

Run: `go test ./internal/infra/ai ./internal/infra/ai/openaicompat ./internal/module/ai/chat ./internal/module/ai/replycommand ./internal/runtime -count=1`

Expected: PASS; pure text, image, file, historical attachment, paid/unpaid and recovered Prepared Request tests remain green.

Run: `rg -n 'Inputs\s+map\[string\]any|input\.Inputs|input\.Content|"system_prompt"|"history"|"attachments"' internal/infra/ai internal/runtime/ai_billing_gateway.go`

Expected: no implicit ChatInput compiler keys. JSON fixture text outside the compiler is allowed only when a test is decoding an HTTP payload.

```bash
git add -- internal/infra/ai internal/module/ai/chat/service.go internal/module/ai/chat/service_test.go internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go internal/runtime/ai_billing_text.go internal/runtime/ai_billing_finalizer.go internal/runtime/ai_billing_finalizer_test.go internal/runtime/providers_test.go
git commit -m "refactor(ai): type chat provider input"
```

### Task 4: Add registered token bounds, deterministic packing and hashes

**Files:**
- Create: `internal/infra/ai/tokencounter.go`
- Create: `internal/infra/ai/tokencounter_test.go`
- Modify: `internal/module/ai/officialmodel/catalog.go`
- Modify: `internal/module/ai/officialmodel/catalog/official_models_v1.json`
- Modify: `internal/module/ai/officialmodel/catalog_test.go`
- Modify: `internal/module/ai/officialmodel/dto.go`
- Create: `internal/module/ai/contextengine/hash.go`
- Create: `internal/module/ai/contextengine/hash_test.go`
- Create: `internal/module/ai/contextengine/packer.go`
- Create: `internal/module/ai/contextengine/packer_test.go`

- [ ] **Step 1: Write failing budget, atomicity and hash property tests**

Test the actual invariants rather than individual branches:

```go
func FuzzPackerNeverSplitsAtomicGroups(f *testing.F) {
	f.Add(uint16(400), uint8(8))
	f.Fuzz(func(t *testing.T, budget uint16, count uint8) {
		plan, err := Pack(PackInput{KnownInputBudget: int64(budget), Candidates: generatedAtomicBlocks(count)})
		if err != nil { return }
		assertSelectedUpperBoundWithinBudget(t, plan)
		assertAtomicGroupsWhole(t, plan.Items)
		assertRequiredBlocksSelected(t, plan.Items)
	})
}
```

Add golden tests proving identical typed input and fixed adapter results produce identical Input Fingerprint and Plan Hash; durations, row IDs and creation time do not change hashes; source/order/decision/score changes do. Test a required overflow and tool continuation overflow as explicit errors, not truncation.

- [ ] **Step 2: Run Packer and hash tests and verify RED**

Run: `go test ./internal/infra/ai ./internal/module/ai/contextengine -run 'Token|Pack|Hash|Atomic|Overflow' -count=1`

Expected: FAIL because the counter registry, Packer and canonical hashes do not exist.

- [ ] **Step 3: Implement a versioned counter registry**

Expose:

```go
type TokenCounter interface {
	ID() string
	UpperBoundText(string) (int64, error)
	UpperBoundJSON(json.RawMessage) (int64, error)
}

const TokenCounterUTF8BytesV1 = "utf8_bytes_v1"
```

`utf8_bytes_v1` is an explicitly registered conservative bound of one token per UTF-8 byte plus the caller-supplied protocol wrapper bound. It is not a character estimate and never claims exactness. Unknown counter ID fails. Add `token_counter_id` to every official model catalog record, validate it against the registry at catalog load, and expose it in the model capability DTO. A model without a registered counter is unusable by Context Engine.

- [ ] **Step 4: Implement deterministic Packer and canonical hashing**

Use exact integer arithmetic:

```go
type Budget struct {
	ContextWindowTokens          int64
	EffectiveOutputTokens        int64
	ProviderProtocolUpperBound   int64
	ToolContinuationInputReserve int64
	PolicySafetyMargin           int64
	KnownInputBudget             int64
	KnownInputUpperBound         int64
	Proof                        BudgetProof
}

func (b Budget) Validate() error {
	want := b.ContextWindowTokens - b.EffectiveOutputTokens - b.ProviderProtocolUpperBound - b.PolicySafetyMargin
	if want < 0 || b.KnownInputBudget != want || b.ToolContinuationInputReserve > b.ProviderProtocolUpperBound || b.KnownInputUpperBound > want {
		return ErrInvalidBudget
	}
	return nil
}
```

Packer first validates all required groups fit, then applies the design's stable priority order and deterministic tie-break `(priority, relevance, source sequence/time, stable source ID)`. It selects or excludes whole `atomic_group_key` groups and assigns `C1..Cn` only after final selection of Document Evidence.

Canonical hash code writes versioned typed structs through `encoding/json`, normalizes fixed scores before sorting, and rejects maps, NaN, Infinity, empty IDs and unvalidated enums. `context_profile_sha256`, `input_fingerprint_sha256` and `plan_sha256` use the exact inclusion/exclusion boundaries in design section 7.1.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/infra/ai ./internal/module/ai/officialmodel ./internal/module/ai/contextengine -run 'Token|Catalog|Pack|Hash|Atomic|Overflow' -count=1`

Expected: PASS, including fuzz seeds and golden hashes.

```bash
git add -- internal/infra/ai/tokencounter.go internal/infra/ai/tokencounter_test.go internal/module/ai/officialmodel/catalog.go internal/module/ai/officialmodel/catalog/official_models_v1.json internal/module/ai/officialmodel/catalog_test.go internal/module/ai/officialmodel/dto.go internal/module/ai/contextengine/hash.go internal/module/ai/contextengine/hash_test.go internal/module/ai/contextengine/packer.go internal/module/ai/contextengine/packer_test.go
git commit -m "feat(ai): add context budget and plan hashing"
```

### Task 5: Make Provider Model kind and Agent Context Profile explicit

**Files:**
- Create: `internal/architecture/ai_provider_model_kind_contract_test.go`
- Modify: `internal/module/ai/provider/model.go`
- Modify: `internal/module/ai/provider/dto.go`
- Modify: `internal/module/ai/provider/repository.go`
- Modify: `internal/module/ai/provider/repository_gorm_test.go`
- Modify: `internal/module/ai/provider/service.go`
- Modify: `internal/module/ai/provider/service_test.go`
- Modify: `internal/module/ai/provider/transport/admin/request.go`
- Modify: `internal/module/ai/provider/transport/admin/handler_test.go`
- Modify: `internal/module/ai/agent/model.go`
- Modify: `internal/module/ai/agent/dto.go`
- Modify: `internal/module/ai/agent/repository.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/service_test.go`
- Modify: `internal/module/ai/agent/transport/admin/request.go`
- Modify: `internal/module/ai/chat/repository.go`
- Modify: `internal/module/ai/chat/service_test.go`
- Modify: `internal/module/ai/message/repository.go`
- Modify: `internal/module/ai/message/history_repository.go`
- Modify: `internal/module/ai/message/history_actions_test.go`
- Modify: `internal/module/ai/tool/repository.go`
- Modify: `internal/module/ai/tool/service_test.go`
- Modify: `internal/module/ai/image/repository.go`
- Modify: `internal/module/ai/image/repository_mapping_test.go`
- Modify: `internal/admincontract/openapi_ai_schemas.go`
- Modify: `internal/admincontract/openapi_test.go`

- [ ] **Step 1: Write failing model-kind isolation tests**

Add tests proving existing provider `model_ids` are stored as `chat`, new typed model entries require a closed kind, Agent create/edit/options can see only `chat`, and Context Profile resolvers reject wrong kinds. The architecture test locks every existing two-column Agent-model join; it uses `mustReadRepoFile` from Task 1:

```go
func TestAllAgentModelJoinsPinChatKind(t *testing.T) {
	wantPredicates := map[string]int{
		"internal/module/ai/agent/repository.go":          2,
		"internal/module/ai/chat/repository.go":           1,
		"internal/module/ai/message/repository.go":        1,
		"internal/module/ai/message/history_repository.go": 1,
		"internal/module/ai/tool/repository.go":           1,
		"internal/module/ai/image/repository.go":          2,
	}
	for path, want := range wantPredicates {
		source := mustReadRepoFile(t, path)
		if got := strings.Count(source, "model_kind = ?"); got < want {
			t.Errorf("%s has %d model-kind predicates, want at least %d", path, got, want)
		}
		if !strings.Contains(source, "ModelKindChat") {
			t.Errorf("%s does not bind the closed Chat model kind", path)
		}
	}
}
```

Add repository tests that put `chat`, `embedding` and `rerank` rows with the same `(provider_id, model_id)` into the fixture and prove Agent, Chat, Message, History, Tool and Image resolvers return exactly the Chat row. The Image test is mandatory because existing `image_generate` Agents also resolve historical rows now backfilled as `chat`; adding the predicate must not disable that path. Add Agent DTO tests for nullable `context_profile_id`: omitted/NULL means pure chat and round-trips as NULL, never zero. Setting a non-null Profile calls a resolver that requires `enabled` plus `index_state=ready`; clearing/changing after a Binding, ready Conversation Version, indexable complete turn or Memory exists returns conflict.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/architecture ./internal/module/ai/provider ./internal/module/ai/agent ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/tool ./internal/module/ai/image ./internal/admincontract -run 'ModelKind|ContextProfile|ChatModel|AgentModel' -count=1`

Expected: FAIL because `model_kind` and `context_profile_id` are absent.

- [ ] **Step 3: Implement closed model kinds without hiding legacy semantics**

Define:

```go
type ModelKind string

const (
	ModelKindChat      ModelKind = "chat"
	ModelKindEmbedding ModelKind = "embedding"
	ModelKindRerank    ModelKind = "rerank"
)

type ProviderModelInput struct {
	ModelID   string    `json:"model_id" binding:"required"`
	ModelKind ModelKind `json:"model_kind" binding:"required"`
}

type ModelReconcileScope string

const (
	ModelReconcileChatOnly ModelReconcileScope = "chat_only"
	ModelReconcileAll      ModelReconcileScope = "all"
)

type Repository interface {
	List(context.Context, ListQuery) ([]Provider, int64, error)
	Get(context.Context, uint64) (*Provider, error)
	ExistsByTypeName(context.Context, string, string, uint64) (bool, error)
	Create(context.Context, Provider) (uint64, error)
	Update(context.Context, uint64, map[string]any) error
	ChangeStatus(context.Context, uint64, int) error
	ListModels(context.Context, uint64) ([]ProviderModel, error)
	ListAllModels(context.Context) ([]ProviderModel, error)
	UpdateModelMapping(context.Context, uint64, officialmodel.IdentityMapping) error
	ReconcileModels(context.Context, uint64, ModelReconcileScope, []ProviderModel) error
	Delete(context.Context, uint64) error
}
```

Admin create/edit accepts typed `models`. Keep the existing typed `model_ids` field as a compatibility input whose historical meaning is exactly Chat models; reject requests that submit both forms. Responses publish only model rows with explicit kind so no consumer guesses. Identity fields `provider_id/model_id/model_kind` become immutable once referenced.

Replace delete-and-recreate `ReplaceModels` with `ReconcileModels`. In one transaction, lock existing rows in scope, index them by `(provider_id, model_id, model_kind)`, update only mutable display/mapping/status fields on matches, insert missing identities with an explicit kind, and set omitted in-scope rows to disabled. Never delete or renumber a row. `model_ids` calls `ModelReconcileChatOnly`, so it cannot disable or delete Embedding/Rerank rows; typed `models` is a complete catalog and calls `ModelReconcileAll`. Test that an existing Profile foreign key survives both paths and that retrying the same request preserves every Provider Model ID.

Every existing Agent, Chat, Message, History, Tool and Image resolver query adds a bound `model_kind = ModelKindChat` predicate. Embedding/Rerank rows must not enter chat options, pricing resolution, Agent persistence or image Agent resolution. The migration may be deployed before the new binary because its default preserves old writes, but no Embedding/Rerank row or Context Profile may be activated until every old API/Worker instance has drained; otherwise an old two-column reader can still see duplicate joins and old `ReplaceModels` can delete rows.

- [ ] **Step 4: Add explicit Agent Profile selection**

Add nullable `ContextProfileID *uint64` to model/input/output and a small resolver interface owned by Agent service:

```go
type ContextProfileResolver interface {
	RequireAssignable(context.Context, uint64) error
	RequireAgentProfileChangeAllowed(context.Context, uint64, *uint64) error
}
```

Until Plan 02 wires the concrete resolver, non-null assignment returns `ai.context.profile_unavailable`; NULL remains valid pure chat. The repository never infers Profile from a Space binding.

- [ ] **Step 5: Run contract tests and commit**

Run: `go test ./internal/architecture ./internal/module/ai/provider ./internal/module/ai/agent ./internal/module/ai/chat ./internal/module/ai/message ./internal/module/ai/tool ./internal/module/ai/image ./internal/module/ai/officialmodel ./internal/admincontract -count=1`

Expected: PASS; old provider requests still create Chat models, new kinds are closed, and pure chat Agents remain valid.

```bash
git add -- internal/architecture/ai_provider_model_kind_contract_test.go internal/module/ai/provider internal/module/ai/agent internal/module/ai/chat internal/module/ai/message internal/module/ai/tool internal/module/ai/image internal/module/ai/officialmodel internal/admincontract/openapi_ai_schemas.go internal/admincontract/openapi_test.go
git commit -m "feat(ai): type provider model purposes"
```

### Task 6: Bind Plan evidence to prepared Provider Attempts

**Files:**
- Modify: `internal/module/ai/billing/model.go`
- Modify: `internal/module/ai/chat/dto.go`
- Modify: `internal/module/ai/replycommand/attempt.go`
- Modify: `internal/module/ai/replycommand/attempt_test.go`
- Modify: `internal/module/ai/replycommand/attempt_integration_test.go`
- Modify: `internal/module/ai/aigateway/contracts.go`
- Modify: `internal/module/ai/aigateway/gateway.go`
- Modify: `internal/module/ai/aigateway/gateway_test.go`
- Modify: `internal/runtime/ai_billing_gateway.go`
- Modify: `internal/runtime/ai_billing_gateway_test.go`

- [ ] **Step 1: Write failing Plan/Attempt evidence tests**

Cover all legal shapes. First lock equality and assembly sealing so Plan evidence cannot be changed while retaining the same Prepared Request:

```go
func TestSameAttemptEvidenceIncludesContextPlan(t *testing.T) {
	hash := sha256.Sum256([]byte("plan"))
	left := ProviderAttempt{
		RunID: 44, AttemptNo: 1, IdempotencyKey: attemptKey(44, 1),
		PreparedRequest: []byte(`{"model":"gpt-test"}`),
		RequestSHA256: sha256.Sum256([]byte(`{"model":"gpt-test"}`)),
		ContextPlan: &ContextPlanEvidence{ID: 91, SHA256: hash},
		Quote: validQuote(44),
	}
	right := cloneAttempt(left)
	right.ContextPlan = &ContextPlanEvidence{ID: 92, SHA256: hash}
	if sameAttemptEvidence(left, right) {
		t.Fatal("different Context Plan IDs compared equal")
	}
}

func TestPreparedAssemblySealIncludesContextPlan(t *testing.T) {
	fingerprint := sha256.Sum256([]byte("input"))
	call := PreparedCall{
		RequestSHA256: sha256.Sum256([]byte(`{"model":"gpt-test"}`)),
		ContextPlan: &ContextPlanEvidence{ID: 91, SHA256: sha256.Sum256([]byte("plan-a"))},
		Quote: validQuote(44),
	}
	left := preparedAssemblySeal(call, 44, fingerprint)
	call.ContextPlan = &ContextPlanEvidence{ID: 91, SHA256: sha256.Sum256([]byte("plan-b"))}
	if right := preparedAssemblySeal(call, 44, fingerprint); left == right {
		t.Fatal("assembly seal ignored Context Plan hash")
	}
}
```

In `attempt_integration_test.go`, use the existing MySQL fixture to prepare an Attempt with Plan 91, then reload it and assert ID/hash equality. Reject only-one-field-present at the row adapter, wrong Run, failed Plan, wrong hash, overwriting a persisted relation, and a recovery relation whose Plan/Prepared hashes conflict. Preserve NULL/NULL for historical, non-chat and pre-activation chat Attempts.

- [ ] **Step 2: Run Attempt evidence tests and verify RED**

Run: `go test ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/runtime -run 'ContextPlan|PreparedAttempt|Recovery' -count=1`

Expected: FAIL because Attempt models and prepare inputs do not carry Plan evidence.

- [ ] **Step 3: Extend the atomic prepare contract**

Add one paired nullable value through every layer; never pass separate nullable ID/hash arguments:

```go
type ContextPlanEvidence struct {
	ID     uint64
	SHA256 [32]byte
}

type RunRequest struct {
	UserID      int64
	RunID       int64
	RequestID   string
	Identity    requestidentity.Input
	ContextPlan *ContextPlanEvidence
}

type PreparedCall struct {
	RequestBody         []byte
	RequestSHA256       [32]byte
	Quote               QuoteEvidence
	ContextPlan         *ContextPlanEvidence
	assemblyRunID       int64
	assemblyFingerprint [32]byte
	assemblySeal        [32]byte
}

type ProviderAttempt struct {
	AttemptID       uint64
	RunID           int64
	AttemptNo       uint32
	IdempotencyKey  string
	PreparedRequest []byte
	RequestSHA256   [32]byte
	Quote           QuoteEvidence
	ContextPlan     *ContextPlanEvidence
}

type PrepareAttemptInput struct {
	RunID                 int64
	CommandID             uint64
	AttemptNo             uint
	Owner                 string
	Token                 uint64
	Now                   time.Time
	PrepareStartedAt      time.Time
	IdempotencyKey        string
	PreparedRequestJSON   string
	PreparedRequestSHA256 [32]byte
	QuoteJSON             string
	ContextPlan           *aigateway.ContextPlanEvidence
}
```

`Prepare` copies `RunRequest.ContextPlan` to `PreparedCall`; `preparedAssemblySeal`, `validPreparedAssembly`, `ProviderAttempt`, `sameAttemptEvidence`, `cloneAttempt`, `PutPrepared` and recovery all include the pair. The GORM repository reads the referenced Plan inside the same prepare transaction and requires matching ID, Run, `ready` state and hash before persisting `context_plan_id/context_plan_sha256` with `prepared_request_json`, `prepared_request_sha256` and Quote. Existing Attempt recovery returns the evidence. It never reconstructs a missing hash from Plan Items and never updates the relation on conflict. A non-nil Plan with a zero ID/hash is invalid; nil remains legal only for historical, non-chat and pre-Plan-03 attempts.

Add `ContextPlan *aigateway.ContextPlanEvidence` to
`aichat.PaidChatAttemptInput` and every new-chat attempt constructor. Plan 01
only carries and seals the pair; Plan 03 makes it mandatory after Context
activation. Do not define a second chat-local evidence struct or pass ID/hash as
separate nullable fields.

Define the dispatch seam now; Plan 03 supplies the MySQL authority implementation and calls it immediately before each dispatch:

```go
type DispatchGuardInput struct {
	RunID                int64
	AttemptNo            uint32
	ContextPlan          ContextPlanEvidence
	PreparedRequestSHA256 [32]byte
}

type DispatchGuard interface {
	GuardDispatch(context.Context, DispatchGuardInput) *apperror.Error
}
```

- [ ] **Step 4: Prove old recovery remains byte-stable**

Run: `go test ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/runtime -count=1`

Expected: PASS. Existing persisted Attempts recover exact `prepared_request_json` bytes; adding Plan evidence does not call the assembler, Packer or any retrieval seam.

- [ ] **Step 5: Commit**

```bash
git add -- internal/module/ai/billing/model.go internal/module/ai/chat/dto.go internal/module/ai/replycommand/attempt.go internal/module/ai/replycommand/attempt_test.go internal/module/ai/replycommand/attempt_integration_test.go internal/module/ai/aigateway/contracts.go internal/module/ai/aigateway/gateway.go internal/module/ai/aigateway/gateway_test.go internal/runtime/ai_billing_gateway.go internal/runtime/ai_billing_gateway_test.go
git commit -m "feat(ai): bind context plans to provider attempts"
```

### Task 7: Verify the core Slice without enabling Context runtime

**Files:**
- Modify: `docs/architecture.md`
- Modify: `internal/module/README.md`

- [ ] **Step 1: Document ownership and the inactive integration boundary**

Record that `contextengine` owns Context facts and deterministic policy, `infra/contextindex` will own Qdrant, Provider compilers own protocol JSON, and runtime composition remains in `internal/platform/admin`/`internal/runtime`. State explicitly that this checkpoint has not replaced `KnowledgeRuntime`; no deploy should claim Context retrieval until Plan 03.

- [ ] **Step 2: Run formatting and targeted gates**

Run: `gofmt -w internal/module/ai/contextengine internal/infra/ai internal/module/ai/provider internal/module/ai/agent internal/module/ai/replycommand internal/module/ai/aigateway internal/module/ai/billing internal/module/ai/chat internal/module/ai/officialmodel internal/runtime`

Run: `go test ./internal/architecture ./internal/infra/ai ./internal/infra/ai/imagecompat ./internal/infra/ai/openaicompat ./internal/infra/ai/provider ./internal/module/ai/contextengine ./internal/module/ai/provider ./internal/module/ai/agent ./internal/module/ai/chat ./internal/module/ai/replycommand ./internal/module/ai/aigateway ./internal/module/ai/officialmodel ./internal/runtime -count=1`

Expected: PASS.

- [ ] **Step 3: Run static contract searches**

Run: `rg -n 'Inputs\s+map\[string\]any|input\.Inputs|input\.Content' internal/infra/ai internal/runtime`

Expected: no ChatInput map/content compiler use.

Run: `rg -n 'CREATE TABLE.*ai_context_(citation|retrieval|hit|job|cursor)' database/migrations/202608020101_ai_context_expand.sql`

Expected: no matches.

- [ ] **Step 4: Run diff checks and commit docs**

Run: `git diff --check`

Expected: no whitespace errors.

```bash
git add -- docs/architecture.md internal/module/README.md
git commit -m "docs(ai): define context engine ownership"
```

- [ ] **Step 5: Record the Slice checkpoint**

Run: `git status --short`

Expected: clean. Record `git rev-parse HEAD`, the seven commits above, RED/GREEN output, and the fact that neither a database migration nor `admin-dev` was run.
