package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportCleanupWorkerCommandHasReconciledSchedule(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(backendRoot(t), "database", "reconciliation", "043_register_export_cleanup.sql"))
	if err != nil {
		t.Fatalf("read export cleanup schedule: %v", err)
	}
	lower := strings.ToLower(string(body))
	for _, required := range []string{"export_cleanup_expired", "export:cleanup-expired:v1", "insert into `cron_task`", "on duplicate key update"} {
		if !strings.Contains(lower, required) {
			t.Fatalf("export cleanup schedule missing %q", required)
		}
	}
	for _, forbidden := range []string{"delete from", "drop table", "drop column"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("export cleanup schedule contains destructive SQL %q", forbidden)
		}
	}
}
