package jobs

import (
	"encoding/json"
	"slices"
	"strings"
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

func TestContextTaskRegistrationContainsExactlyPlan04TurnHandlers(t *testing.T) {
	registry, err := NewRegistry(Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	var contextTypes []string
	for _, taskType := range registry.Types() {
		if strings.HasPrefix(taskType, "ai:context-") {
			contextTypes = append(contextTypes, taskType)
		}
	}
	want := []string{TaskContextConversationIndexV1, TaskContextDocumentIndexV1, TaskContextIndexCleanupV1, TaskContextProfileRebuildV1}
	slices.Sort(want)
	if !slices.Equal(contextTypes, want) {
		t.Fatalf("context task types=%v want=%v", contextTypes, want)
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
