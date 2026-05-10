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
	if len(statuses) != 5 || statuses[0].Value != enum.AIRunStatusRunning || statuses[4].Value != enum.AIRunStatusTimeout {
		t.Fatalf("unexpected run status options: %#v", statuses)
	}
}
