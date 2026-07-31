package enum

import "testing"

func TestAIRuntimeEnumsAreStable(t *testing.T) {
	if !IsAIMessageRole(AIMessageRoleUser) || !IsAIMessageRole(AIMessageRoleAssistant) || !IsAIMessageRole(AIMessageRoleSystem) || IsAIMessageRole(9) {
		t.Fatalf("message role enum mismatch")
	}
	if !IsAIRunStatus(AIRunStatusRunning) || !IsAIRunStatus(AIRunStatusSuccess) || !IsAIRunStatus(AIRunStatusFailed) || !IsAIRunStatus(AIRunStatusCanceled) || !IsAIRunStatus(AIRunStatusTimeout) || !IsAIRunStatus(AIRunStatusOutcomeUnknown) || IsAIRunStatus("queued") || IsAIRunStatus("timed_out") {
		t.Fatalf("run status enum mismatch")
	}
	if AIRunStatusTimeout == "timed_out" {
		t.Fatal("Run timeout must remain distinct from reply-command timed_out")
	}
	if !IsAIRunEvent(AIRunEventStart) || !IsAIRunEvent(AIRunEventCompleted) || !IsAIRunEvent(AIRunEventFailed) || !IsAIRunEvent(AIRunEventCanceled) || !IsAIRunEvent(AIRunEventTimeout) || !IsAIRunEvent(AIRunEventFileMaterialized) || AIRunEventFileMaterialized != "file_materialized_v1" || IsAIRunEvent("delta") {
		t.Fatalf("run event enum mismatch")
	}
}
