package dict

import (
	"testing"

	"admin_back_go/internal/enum"
)

func TestAIRuntimeOptionsUseStableEnums(t *testing.T) {
	statuses := AIRunStatusOptions()
	if len(statuses) != 5 || statuses[0].Value != enum.AIRunStatusRunning || statuses[4].Value != enum.AIRunStatusTimeout {
		t.Fatalf("unexpected run status options: %#v", statuses)
	}
}
