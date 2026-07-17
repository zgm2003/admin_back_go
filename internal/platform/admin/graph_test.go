package admin

import (
	"strings"
	"testing"
)

func TestGraphValidateRejectsMissingRequiredCapability(t *testing.T) {
	graph := Graph{}
	err := graph.Validate()
	if err == nil || !strings.Contains(err.Error(), "identity.auth") {
		t.Fatalf("err=%v", err)
	}
}
