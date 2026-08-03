package jobs

import (
	"encoding/json"
	"testing"

	"admin_back_go/internal/infra/taskqueue"
	"admin_back_go/internal/module/ai/contextengine"
)

func TestContextTaskRegistrationUsesClosedDocumentIndexContract(t *testing.T) {
	service := contextengine.NewDocumentIndexService(contextengine.DocumentIndexDependencies{})
	registry, err := NewRegistry(Dependencies{ContextDocumentIndex: service})
	if err != nil {
		t.Fatal(err)
	}
	task, policy, err := registry.Task(TaskContextDocumentIndexV1, mustJSON(t, ContextDocumentIndexV1{DocumentVersionID: 9}))
	if err != nil {
		t.Fatal(err)
	}
	if policy.Queue != taskqueue.QueueLow || policy.MaxRetry != ContextDocumentIndexMaxRetry || policy.Timeout != ContextDocumentIndexTimeout {
		t.Fatalf("policy=%+v", policy)
	}
	var payload map[string]any
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["document_version_id"] != float64(9) {
		t.Fatalf("payload=%v", payload)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
