package aiimage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageModelTablesUseSingleAICapability(t *testing.T) {
	if got := (ImageTask{}).TableName(); got != "ai_image_tasks" {
		t.Fatalf("image task table mismatch: %s", got)
	}
	if got := (ImageFile{}).TableName(); got != "ai_image_files" {
		t.Fatalf("image file table mismatch: %s", got)
	}
}

func TestImagePackageUsesTaskOwnedFilesOnly(t *testing.T) {
	root := packageRoot(t)
	for _, file := range []string{"model.go", "repository.go", "service.go", "dto.go"} {
		body, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(body)
		for _, forbidden := range []string{"type ImageAsset", "type ImageTaskAsset", "type TaskAssetRow", "RegisterAsset", "CreateTaskWithAssets", "AppendTaskAssets", "LoadTaskAssets", "LoadAssetsByIDs", "InputAssetIDs", "MaskAssetID", "MaskTargetAssetID", "admin_ai_image_tasks", "canvas_image_tasks"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("ai/image must use single task-owned ImageFile tables only; %s still contains %s", file, forbidden)
			}
		}
	}
}

func TestAdminImageWorkspaceDoesNotExposeRetiredSurfaces(t *testing.T) {
	root := packageRoot(t)
	adminTransportFiles, err := filepath.Glob(filepath.Join(root, "transport", "admin", "*.go"))
	if err != nil {
		t.Fatalf("glob admin image transport: %v", err)
	}
	if len(adminTransportFiles) > 0 {
		t.Fatalf("admin image transport must stay retired; go files still exist: %v", adminTransportFiles)
	}

	publicFiles := []string{
		"dto.go",
		"service.go",
		"repository.go",
	}
	for _, file := range publicFiles {
		assertSourceTokensAbsent(t, filepath.Join(root, filepath.FromSlash(file)), []string{
			"/api/admin/v1/ai-images",
			"/favorite",
			"Favorite(",
			"favoriteRequest",
			"is_favorite",
			"FavoriteArr",
		})
	}

	if _, err := os.Stat(filepath.Join(root, "transport", "canvas")); !os.IsNotExist(err) {
		t.Fatalf("retired Canvas image transport still exists: %v", err)
	}

	assertSourceTokensAbsent(t, filepath.Join(root, "..", "..", "..", "..", "scripts", "full-admin-smoke.ps1"), []string{
		"favorite_arr",
		"is_favorite",
		"/favorite",
	})
	assertSourceTokensAbsent(t, filepath.Join(root, "..", "..", "..", "..", "internal", "server", "testdata", "admin_route_policy_golden.json"), []string{
		"ai_image_task_favorite",
		"/api/admin/v1/ai-images/:id/favorite",
	})
	assertRetiredPermissionCleanupMigration(t, filepath.Join(root, "..", "..", "..", "..", "database", "legacy-migrations", "20260607_ai_image_retire_favorite_permission.sql"))
}

func assertRetiredPermissionCleanupMigration(t *testing.T, file string) {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read retired favorite permission cleanup migration: %v", err)
	}
	text := string(body)
	for _, token := range []string{"ai_image_task_favorite", "UPDATE `role_permissions`", "UPDATE `permissions`", "`is_del` = 1"} {
		if !strings.Contains(text, token) {
			t.Fatalf("%s must retire favorite permission and role grants; missing %q", file, token)
		}
	}
}

func assertSourceTokensAbsent(t *testing.T, file string, tokens []string) {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	text := string(body)
	for _, token := range tokens {
		if strings.Contains(text, token) {
			t.Fatalf("%s must not expose retired image favorite surface token %q", file, token)
		}
	}
}

func packageRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
