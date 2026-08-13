package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuickEntryRemovedFromActiveRuntime(t *testing.T) {
	root := backendRoot(t)
	scanRoots := []string{
		"internal",
		"scripts",
	}
	forbidden := []string{
		"users_" + "quick_entry",
		"quick_" + "entry",
		"quick" + "Entry",
		"Quick" + "Entry",
		"quick-" + "entries",
		"User" + "Quick" + "Entry" + "Service",
	}

	var offenders []string
	for _, scanRoot := range scanRoots {
		base := filepath.Join(root, scanRoot)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				if filepath.ToSlash(relative) == "scripts/tests" {
					return filepath.SkipDir
				}
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if filepath.Ext(path) != ".go" && filepath.Ext(path) != ".ps1" && filepath.Ext(path) != ".yaml" {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if rel == "internal/architecture/quickentry_removal_test.go" {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(body)
			for _, token := range forbidden {
				if strings.Contains(text, token) {
					offenders = append(offenders, rel+" contains "+token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", scanRoot, err)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("QuickEntry must be removed from active runtime:\n  %s", strings.Join(offenders, "\n  "))
	}
}
