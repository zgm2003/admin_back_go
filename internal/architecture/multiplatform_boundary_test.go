package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func backendRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func mustExist(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		t.Fatalf("expected %s to exist: %v", rel, err)
	}
}

func mustNotExist(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
		t.Fatalf("expected %s to be removed", rel)
	} else if !os.IsNotExist(err) {
		t.Fatalf("check %s: %v", rel, err)
	}
}

func TestAuthTransportBoundaryShape(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/auth/transport/admin/route.go",
		"internal/module/auth/transport/admin/handler.go",
		"internal/module/auth/transport/admin/handler_test.go",
		"internal/module/auth/transport/admin/request.go",
		"internal/module/auth/transport/admin/presenter.go",
		"internal/module/auth/transport/app/route.go",
		"internal/module/auth/transport/app/handler.go",
		"internal/module/auth/transport/app/handler_test.go",
		"internal/module/auth/transport/app/request.go",
		"internal/module/auth/transport/app/presenter.go",
	} {
		mustExist(t, root, rel)
	}
	for _, rel := range []string{
		"internal/module/auth/route.go",
		"internal/module/auth/handler.go",
		"internal/module/auth/handler_test.go",
		"internal/module/auth/request.go",
		"internal/module/auth/platform_route.go",
		"internal/module/auth/platform_handler.go",
		"internal/module/auth/platform_handler_test.go",
		"internal/module/auth/platform_dto.go",
	} {
		mustNotExist(t, root, rel)
	}
}

func TestNoLegacyUsersRoutesInGoRuntime(t *testing.T) {
	root := backendRoot(t)
	legacyUsersPrefix := "/api/" + "Users"
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
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
		if strings.Contains(string(body), legacyUsersPrefix) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal go files: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("legacy Users route references remain: %s", strings.Join(offenders, ", "))
	}
}

func TestAuthTransportHasNoPlatformPrefixedFiles(t *testing.T) {
	root := backendRoot(t)
	authRoot := filepath.Join(root, "internal", "module", "auth")
	var offenders []string
	err := filepath.WalkDir(authRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, "platform_") || strings.HasPrefix(name, "app_") || strings.HasPrefix(name, "admin_") {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk auth files: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("platform-prefixed auth files remain: %s", strings.Join(offenders, ", "))
	}
}
