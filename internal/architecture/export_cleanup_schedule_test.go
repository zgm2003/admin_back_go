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

func TestCronTaskReconciliationForcesUTF8ClientEncoding(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(backendRoot(t), "scripts", "database", "reconcile.ps1"))
	if err != nil {
		t.Fatalf("read reconciliation runner: %v", err)
	}
	if !strings.Contains(string(body), "--default-character-set=utf8mb4") {
		t.Fatal("reconciliation runner must force utf8mb4 when sourcing UTF-8 SQL")
	}
}

func TestCronTaskMetadataHasIdempotentUTF8RepairAndInvariant(t *testing.T) {
	root := backendRoot(t)
	repair, err := os.ReadFile(filepath.Join(root, "database", "reconciliation", "045_repair_cron_task_utf8_metadata.sql"))
	if err != nil {
		t.Fatalf("read cron task metadata repair: %v", err)
	}
	repairText := string(repair)
	for _, required := range []string{
		"export_cleanup_expired",
		"清理过期导出任务",
		"由 Worker 软删除已过期导出任务，列表与统计接口保持只读",
		"每小时",
		"realtime_event_retention_cleanup",
		"清理过期实时事件",
		"每日清理超过七天的 durable realtime events，并在同一事务推进用户 retention watermark",
		"每天 03:15",
	} {
		if !strings.Contains(repairText, required) {
			t.Fatalf("cron task metadata repair missing %q", required)
		}
	}

	runner, err := os.ReadFile(filepath.Join(root, "scripts", "database", "reconcile.ps1"))
	if err != nil {
		t.Fatalf("read reconciliation runner: %v", err)
	}
	if !strings.Contains(string(runner), "045_repair_cron_task_utf8_metadata.sql") {
		t.Fatal("cron task metadata repair must be part of reconciliation")
	}

	verifier, err := os.ReadFile(filepath.Join(root, "scripts", "database", "verify-expanded-schema.ps1"))
	if err != nil {
		t.Fatalf("read expanded schema verifier: %v", err)
	}
	if !strings.Contains(string(verifier), "037_verify_cron_task_metadata.sql") {
		t.Fatal("expanded schema verification must enforce exact cron task metadata")
	}
}
