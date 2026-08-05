package architecture

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var createTableNameRE = regexp.MustCompile("(?i)\\bCREATE\\s+TABLE\\s+`([^`]+)`")

func mustReadRepoFile(t *testing.T, relativePath string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(backendRoot(t), filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatal(err)
	}
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
	for _, name := range exact {
		exactNames[name] = struct{}{}
	}
	seen := make(map[string]struct{})
	var names []string
	for _, match := range createTableNameRE.FindAllStringSubmatch(migration, -1) {
		name := match[1]
		_, exactMatch := exactNames[name]
		if !strings.HasPrefix(name, prefix) && !exactMatch {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("duplicate CREATE TABLE %s", name)
		}
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
		if strings.Contains(migration, "`"+forbidden+"`") {
			t.Fatalf("forbidden table %s", forbidden)
		}
	}
	for _, fragment := range []string{
		"`ai_provider_models`", "`model_kind`", "`ai_agents`", "`context_profile_id`",
		"`ai_provider_attempts`", "`context_plan_id`", "`context_plan_sha256`",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("expand missing %s", fragment)
		}
	}
}

func TestAIContextExpandDoesNotRewriteLegacyKnowledgeMigration(t *testing.T) {
	migration := mustReadRepoFile(t, "database/migrations/202608020101_ai_context_expand.sql")
	if strings.Contains(migration, "20260510_ai_knowledge_rag.sql") ||
		strings.Contains(migration, "ai_knowledge_") {
		t.Fatal("Expand must not read, copy, rename, or mutate legacy Knowledge tables")
	}
}

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
		for _, match := range matches {
			got = append(got, match[1])
		}
		if !reflect.DeepEqual(got, columns) {
			t.Errorf("%s columns = %v, want %v", tableName, got, columns)
		}
	}
	for tableName, markers := range map[string][]string{
		"ai_context_profiles":          {`foreign_key "fk_ai_context_profiles_embedding_model"`, `check "chk_ai_context_profiles_generation_shape"`},
		"ai_context_documents":         {`foreign_key "fk_ai_context_documents_active_version"`, `check "chk_ai_context_documents_owner_source"`},
		"ai_context_document_versions": {`check "chk_ai_context_document_versions_terminal_shape"`},
		"ai_context_plans":             {`index "uk_ai_context_plans_run"`, `check "chk_ai_context_plans_terminal_shape"`, `check "chk_ai_context_plans_budget"`},
		"ai_context_plan_items":        {`check "chk_ai_context_plan_items_decision"`, `check "chk_ai_context_plan_items_content_snapshot"`},
		"ai_conversation_memories":     {`check "chk_ai_conversation_memories_terminal_shape"`},
	} {
		block := hclTableBlock(t, schema, tableName)
		for _, marker := range markers {
			if !strings.Contains(block, marker) {
				t.Errorf("%s missing %s", tableName, marker)
			}
		}
	}
}

func TestAIContextSchemaOptionalEnhancementWidensOnlyPlanChecks(t *testing.T) {
	migration := mustReadRepoFile(t, "database/migrations/202608050101_ai_context_optional_enhancement.sql")
	upper := strings.ToUpper(migration)
	for _, forbidden := range []string{"CREATE TABLE", "DROP TABLE", "ADD COLUMN", "DROP COLUMN", "UPDATE ", "INSERT ", "DELETE "} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("optional enhancement migration contains %q", forbidden)
		}
	}
	for _, fragment := range []string{
		"DROP CHECK `chk_ai_context_plans_retrieval_outcome`",
		"DROP CHECK `chk_ai_context_plans_terminal_shape`",
		"'skipped', 'no_hit', 'hit', 'degraded', 'failed'",
		"`retrieval_outcome` = 'degraded'",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("optional enhancement migration missing %q", fragment)
		}
	}

	planBlock := hclTableBlock(t, mustReadRepoFile(t, "database/schema/admin.hcl"), "ai_context_plans")
	for _, fragment := range []string{
		"_ascii'degraded'",
		"(`retrieval_outcome` in (_ascii'skipped',_ascii'no_hit',_ascii'hit'))",
		"(`retrieval_outcome` = _ascii'degraded')",
		"(`retrieval_outcome` = _ascii'failed')",
	} {
		if !strings.Contains(planBlock, fragment) {
			t.Fatalf("canonical context plan checks missing %q", fragment)
		}
	}
}
