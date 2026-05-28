package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfraRenameComplete(t *testing.T) {
	root := backendRoot(t)
	legacyImport := "admin_back_go/internal/" + "platform"

	mustNotExist(t, root, "internal/platform")
	mustExist(t, root, "internal/infra")

	var offenders []string
	for _, base := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), legacyImport) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s go files: %v", base, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("legacy internal/platform imports remain:\n  %s", strings.Join(offenders, "\n  "))
	}
}
