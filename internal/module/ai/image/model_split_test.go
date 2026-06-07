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

func packageRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
