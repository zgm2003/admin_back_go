package architecture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIPromptAssetConvergence(t *testing.T) {
	root := backendRoot(t)

	assertFileContains(t, filepath.Join(root, "database/migrations/20260607_ai_prompt_asset_convergence.sql"), []string{
		"CREATE TABLE IF NOT EXISTS `ai_prompts`",
		"CREATE TABLE IF NOT EXISTS `ai_assets`",
		"INSERT IGNORE INTO `ai_prompts`",
		"INSERT IGNORE INTO `ai_assets`",
	})

	assertOptionalFileContains(t, filepath.Join(root, "database/migrations/20260607_ai_prompt_asset_drop_legacy.sql"), []string{
		"DROP TABLE IF EXISTS `canvas_prompts`",
		"DROP TABLE IF EXISTS `canvas_assets`",
	})

	assertFileContains(t, filepath.Join(root, "internal/module/ai/prompt/model.go"), []string{
		`func (Prompt) TableName() string { return "ai_prompts" }`,
	})
	assertFileContains(t, filepath.Join(root, "internal/module/ai/asset/model.go"), []string{
		`func (Asset) TableName() string { return "ai_assets" }`,
	})

	assertCanvasModuleDoesNotContainAIPromptAssetLegacyTokens(t, root)
}

func assertFileContains(t *testing.T, path string, required []string) {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.ToSlash(path), err)
	}
	assertTextContains(t, filepath.ToSlash(path), string(body), required)
}

func assertOptionalFileContains(t *testing.T, path string, required []string) {
	t.Helper()

	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read %s: %v", filepath.ToSlash(path), err)
	}
	assertTextContains(t, filepath.ToSlash(path), string(body), required)
}

func assertTextContains(t *testing.T, label, text string, required []string) {
	t.Helper()

	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("%s missing %q", label, want)
		}
	}
}

func assertCanvasModuleDoesNotContainAIPromptAssetLegacyTokens(t *testing.T, root string) {
	t.Helper()

	canvasFiles, err := filepath.Glob(filepath.Join(root, "internal/module/canvas/*.go"))
	if err != nil {
		t.Fatalf("glob canvas module files: %v", err)
	}

	banned := []string{
		"type Prompt struct",
		"type Asset struct",
		"canvas_prompts",
		"canvas_assets",
		"PublicPrompts(",
		"PublicAssets(",
	}
	for _, path := range canvasFiles {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.ToSlash(path), err)
		}
		text := string(body)
		for _, bad := range banned {
			if strings.Contains(text, bad) {
				t.Fatalf("%s must not contain %q", filepath.ToSlash(path), bad)
			}
		}
	}
}
