package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestAIRuntimeOptionsUseStableEnums(t *testing.T) {
	roles := AIMessageRoleOptions()
	if len(roles) != 3 || roles[0].Value != enum.AIMessageRoleUser || roles[0].Label != "user" {
		t.Fatalf("unexpected role options: %#v", roles)
	}
	statuses := AIRunStatusOptions()
	if len(statuses) != 4 || statuses[0].Value != enum.AIRunStatusRunning || statuses[3].Value != enum.AIRunStatusCanceled {
		t.Fatalf("unexpected run status options: %#v", statuses)
	}
	stepTypes := AIRunStepTypeOptions()
	if len(stepTypes) != 7 || stepTypes[0].Value != enum.AIRunStepTypePrompt || stepTypes[6].Value != enum.AIRunStepTypeImage {
		t.Fatalf("unexpected step type options: %#v", stepTypes)
	}
	stepStatuses := AIRunStepStatusOptions()
	if len(stepStatuses) != 2 || stepStatuses[0].Value != enum.AIRunStepStatusSuccess || stepStatuses[1].Value != enum.AIRunStepStatusFail {
		t.Fatalf("unexpected step status options: %#v", stepStatuses)
	}
}
