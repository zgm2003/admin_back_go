package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageSchemaConvergesToSingleCapabilityTables(t *testing.T) {
	root := imageSchemaRepoRoot(t)
	migration := imageSchemaReadFile(t, filepath.Join(root, "database", "migrations", "20260607_ai_image_single_capability_convergence.sql"))

	for _, want := range []string{
		"`ai_image_tasks`",
		"`ai_image_files`",
		"`platform`",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("single image capability migration must contain %s", want)
		}
	}

	for _, forbidden := range []string{
		"CREATE TABLE `admin_ai_image_tasks`",
		"CREATE TABLE `admin_ai_image_files`",
		"CREATE TABLE `canvas_image_tasks`",
		"CREATE TABLE `canvas_image_files`",
	} {
		if strings.Contains(migration, forbidden) {
			t.Fatalf("single image capability migration must not create split table: %s", forbidden)
		}
	}
}

func TestImageSchemaDropsRetiredSplitTables(t *testing.T) {
	root := imageSchemaRepoRoot(t)
	migration := imageSchemaReadFile(t, filepath.Join(root, "database", "migrations", "20260607_ai_image_single_capability_convergence.sql"))

	for _, table := range []string{
		"`admin_ai_image_files`",
		"`admin_ai_image_tasks`",
		"`canvas_image_files`",
		"`canvas_image_tasks`",
	} {
		if !strings.Contains(migration, "DROP TABLE IF EXISTS "+table) {
			t.Fatalf("single image capability migration must drop retired split table %s", table)
		}
	}
}

func TestImageSchemaDropsRetiredGlobalAssetTables(t *testing.T) {
	root := imageSchemaRepoRoot(t)
	migration := imageSchemaReadFile(t, filepath.Join(root, "database", "migrations", "20260607_ai_image_single_capability_convergence.sql"))

	for _, table := range []string{
		"`ai_image_task_assets`",
		"`ai_image_assets`",
	} {
		if !strings.Contains(migration, "DROP TABLE IF EXISTS "+table) {
			t.Fatalf("single image capability migration must drop retired global asset table %s", table)
		}
	}
}

func TestImageSchemaMigratesRetiredImageRowsBeforeDrop(t *testing.T) {
	root := imageSchemaRepoRoot(t)
	migration := imageSchemaReadFile(t, filepath.Join(root, "database", "migrations", "20260607_ai_image_single_capability_convergence.sql"))

	for _, want := range []string{
		"tmp_ai_image_task_map",
		"tmp_ai_image_file_map",
		"INSERT IGNORE INTO `ai_image_tasks`",
		"INSERT IGNORE INTO `ai_image_files`",
		"`admin_ai_image_tasks`",
		"`canvas_image_tasks`",
		"`ai_image_task_assets`",
		"`ai_image_assets`",
		"`platform`",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("single image capability migration must preserve retired image rows before dropping tables; missing %s", want)
		}
	}
}

func TestImageModelsUseSingleCapabilityTables(t *testing.T) {
	root := imageSchemaRepoRoot(t)
	model := imageSchemaReadFile(t, filepath.Join(root, "internal", "module", "ai", "image", "model.go"))

	for _, want := range []string{
		`func (ImageTask) TableName() string { return "ai_image_tasks" }`,
		`func (ImageFile) TableName() string { return "ai_image_files" }`,
	} {
		if !strings.Contains(model, want) {
			t.Fatalf("ai/image model must contain %q", want)
		}
	}
	if !strings.Contains(model, "Platform") || !strings.Contains(model, `gorm:"column:platform"`) {
		t.Fatal("ai/image task model must own a platform column")
	}

	for _, forbidden := range []string{
		"admin_ai_image_tasks",
		"admin_ai_image_files",
		"canvas_image_tasks",
		"canvas_image_files",
		"type ImageAsset",
		"type ImageTaskAsset",
	} {
		if strings.Contains(model, forbidden) {
			t.Fatalf("ai/image model must not contain retired shape %q", forbidden)
		}
	}
}

func TestAIRunDoesNotKeepImageSourceTypes(t *testing.T) {
	root := imageSchemaRepoRoot(t)
	enumFile := imageSchemaReadFile(t, filepath.Join(root, "internal", "shared", "enum", "ai.go"))

	for _, forbidden := range []string{
		"AIRunSourceImageTask",
		`"ai_image_task"`,
		"admin_ai_image_task",
		"canvas_image_task",
	} {
		if strings.Contains(enumFile, forbidden) {
			t.Fatalf("AI run source types must be deleted from enum file; found %q", forbidden)
		}
	}
}

func imageSchemaRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		next := filepath.Dir(wd)
		if next == wd {
			t.Fatal("go.mod not found")
		}
		wd = next
	}
}

func imageSchemaReadFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
