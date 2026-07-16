package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIBillingRuntimeRetired(t *testing.T) {
	root := repoRoot(t)
	forbiddenActive := []string{
		"internal/module/ai/billing",
		"aibilling.",
		"AiBillingService",
		"/ai-billing-rules",
		"Charge(",
		"Refund(",
		"MarkSuccess(",
		"BillingRecord",
		"billing_record_id",
		"ai_billing_records",
		"ai_billing_rules",
	}
	allowedFiles := map[string]struct{}{
		filepath.ToSlash(filepath.Join(root, "internal/architecture/canvas_free_agent_scenes_test.go")): {},
	}
	walkActiveBillingFiles(t, root, func(path string, body string) {
		if _, ok := allowedFiles[filepath.ToSlash(path)]; ok {
			return
		}
		for _, token := range forbiddenActive {
			if strings.Contains(body, token) {
				t.Fatalf("retired AI billing token %q still present in %s", token, path)
			}
		}
	})

	migrationPath := filepath.Join(root, "database/legacy-migrations/20260601_canvas_free_agent_scenes.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	migrationText := string(migration)
	requiredSQL := []string{
		"CREATE TABLE IF NOT EXISTS `canvas_video_tasks`",
		"DROP TABLE IF EXISTS `ai_billing_records`",
		"DROP TABLE IF EXISTS `ai_billing_rules`",
		"DROP COLUMN `billing_record_id`",
	}
	for _, token := range requiredSQL {
		if !strings.Contains(migrationText, token) {
			t.Fatalf("migration missing %q", token)
		}
	}
	if strings.Contains(migrationText, "JSON_QUOTE('canvas_video_generate'))") && strings.Contains(migrationText, "JSON_ARRAY_APPEND") {
		t.Fatalf("migration must not blindly append canvas_video_generate to existing agents")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Fatalf("go.mod not found from %s", cwd)
		}
		cwd = parent
	}
}

func walkActiveBillingFiles(t *testing.T, root string, visit func(path string, body string)) {
	t.Helper()
	scanRoots := []string{
		filepath.Join(root, "internal/module/ai/agent"),
		filepath.Join(root, "internal/module/ai/image"),
		filepath.Join(root, "internal/module/canvas"),
		filepath.Join(root, "internal/bootstrap"),
		filepath.Join(root, "internal/server/router.go"),
		filepath.Join(root, "internal/server/routes_admin_ai.go"),
	}
	for _, scanRoot := range scanRoots {
		info, statErr := os.Stat(scanRoot)
		if statErr != nil {
			t.Fatalf("stat scan root %s: %v", scanRoot, statErr)
		}
		if !info.IsDir() {
			body, err := os.ReadFile(scanRoot)
			if err != nil {
				t.Fatalf("read scan file %s: %v", scanRoot, err)
			}
			visit(scanRoot, string(body))
			continue
		}
		err := filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			visit(path, string(body))
			return nil
		})
		if err != nil {
			t.Fatalf("walk go files: %v", err)
		}
	}
}
