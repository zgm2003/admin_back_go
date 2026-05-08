package enum

import "testing"

func TestAIRuntimeEnumsAreStable(t *testing.T) {
	if !IsAIMessageRole(AIMessageRoleUser) || !IsAIMessageRole(AIMessageRoleAssistant) || !IsAIMessageRole(AIMessageRoleSystem) || IsAIMessageRole(9) {
		t.Fatalf("message role enum mismatch")
	}
	if !IsAIRunStatus(AIRunStatusRunning) || !IsAIRunStatus(AIRunStatusSuccess) || !IsAIRunStatus(AIRunStatusFail) || !IsAIRunStatus(AIRunStatusCanceled) || IsAIRunStatus(9) {
		t.Fatalf("run status enum mismatch")
	}
	if !IsAIRunStepType(AIRunStepTypePrompt) || !IsAIRunStepType(AIRunStepTypeFinalize) || !IsAIRunStepType(AIRunStepTypeImage) || IsAIRunStepType(99) {
		t.Fatalf("step type enum mismatch")
	}
	if !IsAIRunStepStatus(AIRunStepStatusSuccess) || !IsAIRunStepStatus(AIRunStepStatusFail) || IsAIRunStepStatus(9) {
		t.Fatalf("step status enum mismatch")
	}
}
