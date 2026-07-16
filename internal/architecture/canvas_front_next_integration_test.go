package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanvasFrontNextIntegration(t *testing.T) {
	t.Run("Migration", func(t *testing.T) {
		migration := readCanvasIntegrationFile(t, "database/legacy-migrations/20260531_canvas_front_next_integration.sql")
		for _, want := range []string{
			"auth_platforms", "'canvas'", "无限画布",
			"canvas_prompts", "canvas_assets",
			"uk_canvas_prompts_slug", "uk_canvas_assets_slug",
			"canvas_pages", "canvas_buttons",
			"'/canvas'", "'canvas_page'",
			"'/image'", "'canvas_image_page'",
			"'/video'", "'canvas_video_page'",
			"'/prompts'", "'canvas_prompts_page'",
			"'/assets'", "'canvas_assets_page'",
			"'/profile'", "'canvas_profile_page'",
			"'/wallet'", "'canvas_wallet_page'",
			"'canvas_prompt_read'", "'canvas_asset_read'",
			"'canvas_ai_image_generate'", "'canvas_ai_video_generate'",
			"'canvas_recharge_add'", "'canvas_recharge_pay'",
		} {
			assertCanvasContains(t, migration, want)
		}
		for _, forbiddenPermissionShape := range []string{
			"SELECT permission_name, '', '', 0, '', 'canvas', 3",
			"'canvas_access' AS permission_code",
		} {
			assertCanvasNotContains(t, migration, forbiddenPermissionShape)
		}
		for _, forbidden := range []string{
			"CREATE TABLE `canvas_users`",
			"CREATE TABLE `canvas_credit_logs`",
			"CREATE TABLE `canvas_settings`",
			"CREATE TABLE `canvas_model_channels`",
			"CREATE TABLE `canvas_projects`",
			"CREATE TABLE `canvas_wallets`",
		} {
			assertCanvasNotContains(t, migration, forbidden)
		}
	})

	t.Run("AuthRoutesAndPlatform", func(t *testing.T) {
		routesAuth := readCanvasIntegrationFile(t, "internal/server/routes_auth.go")

		assertCanvasContains(t, routesAuth, `authcanvas.Register(router, authcanvas.Dependencies{`)
		assertCanvasNotContains(t, routesAuth, `Prefix:         "/api/canvas/v1/auth"`)
		assertCanvasPathExists(t, "internal/module/auth/transport/canvas")
	})
}

func TestCanvasRBACMigrationSeedsPageButtonRoleGrants(t *testing.T) {
	migration := readCanvasIntegrationFile(t, "database/legacy-migrations/20260531_canvas_front_next_integration.sql")

	for _, code := range []string{
		"canvas_page",
		"canvas_image_page",
		"canvas_video_page",
		"canvas_prompts_page",
		"canvas_assets_page",
		"canvas_profile_page",
		"canvas_wallet_page",
	} {
		assertCanvasContains(t, migration, "'"+code+"'")
	}
	for _, code := range []string{
		"canvas_access",
		"canvas_prompt_read",
		"canvas_asset_read",
		"canvas_ai_image_generate",
		"canvas_ai_video_generate",
		"canvas_wallet_read",
		"canvas_recharge_add",
		"canvas_recharge_pay",
	} {
		assertCanvasContains(t, migration, "'"+code+"'")
	}

	assertCanvasContains(t, migration, "INSERT INTO `role_permissions`")

	normalized := normalizeSQLForStaticCheck(migration)
	for _, want := range []string{
		"platform='canvas'",
		"typeIN(2,3)",
	} {
		assertCanvasContains(t, normalized, want)
	}
}

func TestCanvasRechargeMenuMigration(t *testing.T) {
	migration := readCanvasIntegrationFile(t, "database/legacy-migrations/20260601_canvas_profile_wallet_recharge_menu.sql")
	for _, want := range []string{"SET NAMES utf8mb4", "canvas_recharge_page", "'/recharge'", "'recharge'", "'menu.canvas_recharge'", "canvas_wallet_read", "canvas_recharge_add", "canvas_recharge_pay", "INSERT INTO `role_permissions`"} {
		assertCanvasContains(t, migration, want)
	}
	for _, forbidden := range []string{"CREATE TABLE `canvas_users`", "CREATE TABLE `canvas_wallets`", "CREATE TABLE `canvas_recharges`", "CREATE TABLE `canvas_credit_logs`"} {
		assertCanvasNotContains(t, migration, forbidden)
	}
}

func readCanvasIntegrationFile(t *testing.T, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(backendRoot(t), rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(content)
}

func assertCanvasContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected content to contain %q", needle)
	}
}

func assertCanvasNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected content not to contain %q", needle)
	}
}

func assertCanvasPathExists(t *testing.T, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(backendRoot(t), rel)); err != nil {
		t.Fatalf("path must exist: %s: %v", rel, err)
	}
}

func normalizeSQLForStaticCheck(sql string) string {
	replacer := strings.NewReplacer("`", "", " ", "", "\t", "", "\r", "", "\n", "")
	return replacer.Replace(sql)
}
