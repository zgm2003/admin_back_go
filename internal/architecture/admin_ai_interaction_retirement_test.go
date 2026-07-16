package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminAIInteractionSurfacesRetired(t *testing.T) {
	root := adminAIRetirementRepoRoot(t)

	adminAIRoutes := readAdminAIRetirementText(t, filepath.Join(root, "internal/server/routes_admin_ai.go"))
	for _, forbidden := range []string{
		`aiimageadmin "admin_back_go/internal/module/ai/image/transport/admin"`,
		`aiassetadmin "admin_back_go/internal/module/ai/asset/transport/admin"`,
		"aiimageadmin.Register",
		"aiassetadmin.RegisterRoutes",
	} {
		if strings.Contains(adminAIRoutes, forbidden) {
			t.Fatalf("admin AI routes still expose retired interactive/admin asset surface %q", forbidden)
		}
	}

	adminRouteGolden := readAdminAIRetirementText(t, filepath.Join(root, "internal/server/testdata/admin_routes_golden.txt"))
	for _, forbidden := range []string{
		"/api/admin/v1/ai-images",
		"/api/admin/v1/ai-assets",
	} {
		if strings.Contains(adminRouteGolden, forbidden) {
			t.Fatalf("admin route golden still contains retired route %q", forbidden)
		}
	}

	fullSmoke := readAdminAIRetirementText(t, filepath.Join(root, "scripts/full-admin-smoke.ps1"))
	for _, required := range []string{
		"/ai/image-playground",
		"/ai/assets",
		"ai_image_task_add",
		"ai_asset_add",
		"ai_asset_edit",
		"ai_asset_del",
	} {
		if !strings.Contains(fullSmoke, required) {
			t.Fatalf("full admin smoke must reject retired Admin AI menu/button surface %q", required)
		}
	}
	for _, forbidden := range []string{
		"sceneImageGenerate",
		`"image_generate"`,
		"'image_generate'",
		"SceneImageGenerate",
	} {
		agentService := readAdminAIRetirementText(t, filepath.Join(root, "internal/module/ai/agent/service.go"))
		if strings.Contains(agentService, forbidden) {
			t.Fatalf("AI agent service still exposes retired non-Canvas image scene %q", forbidden)
		}
		imageService := readAdminAIRetirementText(t, filepath.Join(root, "internal/module/ai/image/service.go"))
		if strings.Contains(imageService, forbidden) {
			t.Fatalf("AI image service still exposes retired non-Canvas image scene %q", forbidden)
		}
		if strings.Contains(fullSmoke, forbidden) {
			t.Fatalf("full admin smoke still probes retired non-Canvas image scene %q", forbidden)
		}
	}

	retireMigration := readAdminAIRetirementText(t, filepath.Join(root, "database/legacy-migrations/20260608_admin_ai_interaction_retirement.sql"))
	for _, required := range []string{
		"ai_image_playground_page",
		"ai_image_task_add",
		"ai_image_task_del",
		"ai_asset_page",
		"ai_asset_add",
		"ai_asset_edit",
		"ai_asset_del",
		"UPDATE `permissions`",
		"UPDATE `role_permissions`",
		"`is_del` = 1",
	} {
		if !strings.Contains(retireMigration, required) {
			t.Fatalf("retirement migration missing %q", required)
		}
	}
	if !strings.Contains(retireMigration, "JSON_REMOVE") || !strings.Contains(retireMigration, "JSON_SEARCH") || !strings.Contains(retireMigration, "'image_generate'") {
		t.Fatal("retirement migration must remove retired image_generate from ai_agents.scenes_json")
	}
}

func TestCanvasAssetsAreUserOwned(t *testing.T) {
	root := adminAIRetirementRepoRoot(t)

	ownershipMigration := readAdminAIRetirementText(t, filepath.Join(root, "database/legacy-migrations/20260608_ai_asset_user_ownership.sql"))
	for _, required := range []string{
		"ALTER TABLE `ai_assets` ADD COLUMN `user_id` BIGINT UNSIGNED NOT NULL DEFAULT 0",
		"DROP INDEX `uk_ai_assets_slug` ON `ai_assets`",
		"CREATE UNIQUE INDEX `uk_ai_assets_user_slug` ON `ai_assets` (`user_id`, `slug`)",
		"CREATE INDEX `idx_ai_assets_user_status_updated` ON `ai_assets` (`user_id`, `status`, `is_del`, `updated_at`, `id`)",
	} {
		if !strings.Contains(ownershipMigration, required) {
			t.Fatalf("asset ownership migration missing %q", required)
		}
	}

	model := readAdminAIRetirementText(t, filepath.Join(root, "internal/module/ai/asset/model.go"))
	if !strings.Contains(model, "UserID") || !strings.Contains(model, `gorm:"column:user_id"`) {
		t.Fatal("AI asset model must expose user_id ownership")
	}

	repository := readAdminAIRetirementText(t, filepath.Join(root, "internal/module/ai/asset/repository.go"))
	for _, required := range []string{
		"query.UserID",
		"Where(\"user_id = ?",
		"Where(\"id = ? AND user_id = ? AND is_del = ?",
	} {
		if !strings.Contains(repository, required) {
			t.Fatalf("AI asset repository missing user ownership guard %q", required)
		}
	}

	canvasHandler := readAdminAIRetirementText(t, filepath.Join(root, "internal/module/ai/asset/transport/canvas/handler.go"))
	for _, required := range []string{
		"middleware.ContextAuthIdentity",
		"identity.UserID",
		"UserList",
		"UserCreate",
		"UserUpdate",
		"UserDelete",
	} {
		if !strings.Contains(canvasHandler, required) {
			t.Fatalf("Canvas asset handler missing user-owned flow %q", required)
		}
	}
}

func adminAIRetirementRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("go.mod not found")
		}
		root = next
	}
}

func readAdminAIRetirementText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.ToSlash(path), err)
	}
	return string(body)
}
