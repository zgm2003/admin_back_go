package enum

import "testing"

func TestAIRuntimeEnumsAreStable(t *testing.T) {
	if !IsAIMessageRole(AIMessageRoleUser) || !IsAIMessageRole(AIMessageRoleAssistant) || !IsAIMessageRole(AIMessageRoleSystem) || IsAIMessageRole(9) {
		t.Fatalf("message role enum mismatch")
	}
	if !IsAIRunStatus(AIRunStatusRunning) || !IsAIRunStatus(AIRunStatusSuccess) || !IsAIRunStatus(AIRunStatusFailed) || !IsAIRunStatus(AIRunStatusCanceled) || !IsAIRunStatus(AIRunStatusTimeout) || IsAIRunStatus("queued") {
		t.Fatalf("run status enum mismatch")
	}
	if !IsAIRunEvent(AIRunEventStart) || !IsAIRunEvent(AIRunEventCompleted) || !IsAIRunEvent(AIRunEventFailed) || !IsAIRunEvent(AIRunEventCanceled) || !IsAIRunEvent(AIRunEventTimeout) || IsAIRunEvent("delta") {
		t.Fatalf("run event enum mismatch")
	}
}
