package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIImageSoftDeleteExpansionIsNonDestructive(t *testing.T) {
	path := filepath.Join(backendRoot(t), "database", "reconciliation", "042_add_ai_image_soft_delete.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read AI image soft-delete expansion: %v", err)
	}
	lower := strings.ToLower(string(body))
	for _, required := range []string{"ai_image_tasks", "ai_image_files", "is_del", "default 2"} {
		if !strings.Contains(lower, required) {
			t.Fatalf("AI image soft-delete expansion missing %q", required)
		}
	}
	for _, forbidden := range []string{"delete from", "drop table", "drop column"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("AI image soft-delete expansion contains destructive SQL %q", forbidden)
		}
	}
}
