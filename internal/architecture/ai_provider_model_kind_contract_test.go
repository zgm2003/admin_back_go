package architecture

import (
	"strings"
	"testing"
)

func TestAllAgentModelJoinsPinChatKind(t *testing.T) {
	wantPredicates := map[string]int{
		"internal/module/ai/agent/repository.go":           2,
		"internal/module/ai/chat/repository.go":            1,
		"internal/module/ai/message/repository.go":         1,
		"internal/module/ai/message/history_repository.go": 1,
		"internal/module/ai/tool/repository.go":            1,
		"internal/module/ai/image/repository.go":           2,
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

func TestProviderModelKindFinalSchemaAndMigrationGuards(t *testing.T) {
	schema := mustReadRepoFile(t, "database/schema/admin.hcl")
	block := hclTableBlock(t, schema, "ai_provider_models")
	for _, required := range []string{
		`column "embedding_dimensions"`,
		`column "embedding_max_input_tokens"`,
		`column "embedding_token_counter_id"`,
		`_ascii'image'`,
		`check "chk_ai_provider_models_embedding_spec"`,
	} {
		if !strings.Contains(block, required) {
			t.Fatalf("ai_provider_models missing %q", required)
		}
	}
	backfill := mustReadRepoFile(t, "database/migrations/202608080102_ai_provider_model_kind_backfill.sql")
	for _, required := range []string{"gpt-image-2", "scenes_json", "image_generate", "ai_context_profiles", "embedding_dimensions"} {
		if !strings.Contains(backfill, required) {
			t.Fatalf("backfill missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(backfill), "DELETE FROM `AI_PROVIDER_MODELS`") {
		t.Fatal("backfill must not delete provider model rows")
	}
}
