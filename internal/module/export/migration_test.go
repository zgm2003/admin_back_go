package exporttask

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportRuntimeV2MigrationUsesGuardedDDL(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", "20260530_export_runtime_v2.sql"))
	if err != nil {
		t.Fatalf("read export migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"information_schema.COLUMNS",
		"information_schema.STATISTICS",
		"PREPARE export_tasks_add_kind_stmt",
		"PREPARE export_tasks_add_platform_stmt",
		"PREPARE export_tasks_add_object_key_stmt",
		"PREPARE export_tasks_add_user_platform_status_idx_stmt",
		"PREPARE export_tasks_add_user_platform_kind_idx_stmt",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("export migration must contain guarded DDL marker %q", want)
		}
	}
}
