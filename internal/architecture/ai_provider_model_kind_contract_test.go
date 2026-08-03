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
