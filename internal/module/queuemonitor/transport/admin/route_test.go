package admin

import (
	"testing"

	queuemonitormodule "admin_back_go/internal/module/queuemonitor"
)

func TestQueueMonitorUIPathMatchesRuntimeMonitorPath(t *testing.T) {
	if UIPath != queuemonitormodule.UIPath {
		t.Fatalf("expected queue monitor transport UI path %q to match runtime monitor path %q", UIPath, queuemonitormodule.UIPath)
	}
}
