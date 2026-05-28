package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedPackageMigrationComplete(t *testing.T) {
	root := backendRoot(t)
	packages := []string{"apperror", "response", "i18n", "enum", "validate", "dict"}
	for _, name := range packages {
		if _, err := os.Stat(filepath.Join(root, "internal", name)); !os.IsNotExist(err) {
			t.Fatalf("old shared-like package internal/%s must not exist after migration", name)
		}
		if _, err := os.Stat(filepath.Join(root, "internal", "shared", name)); err != nil {
			t.Fatalf("internal/shared/%s must exist after migration: %v", name, err)
		}
	}

	bannedPrefix := "admin_back_go/internal/"
	banned := make([]string, 0, len(packages))
	for _, name := range packages {
		banned = append(banned, bannedPrefix+name)
	}

	for _, base := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(body)
			for _, oldImport := range banned {
				if strings.Contains(text, oldImport) {
					rel, _ := filepath.Rel(root, path)
					t.Fatalf("%s still imports old shared-like package %q", filepath.ToSlash(rel), oldImport)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
}
