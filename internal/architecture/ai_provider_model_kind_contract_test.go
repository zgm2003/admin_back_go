package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentModelQueriesPinTheirRequiredKinds(t *testing.T) {
	wantKinds := map[string]string{
		"internal/module/ai/chat/repository.go":            "ModelKindChat",
		"internal/module/ai/message/repository.go":         "ModelKindChat",
		"internal/module/ai/message/history_repository.go": "ModelKindChat",
		"internal/module/ai/tool/repository.go":            "ModelKindChat",
		"internal/module/ai/image/repository.go":           "ModelKindImage",
	}
	for path, wantKind := range wantKinds {
		body, err := os.ReadFile(filepath.Join(backendRoot(t), filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		if !strings.Contains(source, "model_kind = ?") {
			t.Errorf("%s does not constrain provider model kind", path)
		}
		if !strings.Contains(source, wantKind) {
			t.Errorf("%s does not bind %s", path, wantKind)
		}
	}

	agentRepository, err := os.ReadFile(filepath.Join(backendRoot(t), "internal", "module", "ai", "agent", "repository.go"))
	if err != nil {
		t.Fatal(err)
	}
	agentSource := string(agentRepository)
	for _, required := range []string{"model_kind IN ?", "ModelKindChat", "ModelKindImage", "pm.model_kind = ?"} {
		if !strings.Contains(agentSource, required) {
			t.Errorf("agent repository missing model-kind boundary %q", required)
		}
	}
}

func TestProviderModelKindFinalSchema(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(backendRoot(t), "database", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(string(body))
	for _, required := range []string{
		"`model_kind`",
		"`embedding_dimensions`",
		"`embedding_max_input_tokens`",
		"`embedding_token_counter_id`",
		"constraint `chk_ai_provider_models_model_kind`",
		"constraint `chk_ai_provider_models_embedding_spec`",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("ai_provider_models schema missing %q", required)
		}
	}
}
