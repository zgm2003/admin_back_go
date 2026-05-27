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
	authRoot := "internal/module/auth/"
	for _, rel := range []string{
		authRoot + "transport/admin/route.go",
		authRoot + "transport/admin/handler.go",
		authRoot + "transport/admin/handler_test.go",
		authRoot + "transport/admin/request.go",
		authRoot + "transport/admin/presenter.go",
		authRoot + "transport/app/route.go",
		authRoot + "transport/app/handler.go",
		authRoot + "transport/app/handler_test.go",
		authRoot + "transport/app/request.go",
		authRoot + "transport/app/presenter.go",
	} {
		mustExist(t, root, rel)
	}
	platformFilePrefix := "platform_"
	for _, rel := range []string{
		authRoot + "route.go",
		authRoot + "handler.go",
		authRoot + "handler_test.go",
		authRoot + "request.go",
		authRoot + platformFilePrefix + "route.go",
		authRoot + platformFilePrefix + "handler.go",
		authRoot + platformFilePrefix + "handler_test.go",
		authRoot + platformFilePrefix + "dto.go",
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

func TestAuthAdjacentModulesAreMerged(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/captcha",
		"internal/module/session",
		"internal/module/usersession",
		"internal/module/userloginlog",
	} {
		if info, err := os.Stat(filepath.Join(root, rel)); err == nil && info.IsDir() {
			t.Fatalf("expected %s to be removed (merged into auth)", rel)
		}
	}
	for _, rel := range []string{
		"internal/module/auth/captcha.go",
		"internal/module/auth/session.go",
		"internal/module/auth/loginlog.go",
	} {
		mustExist(t, root, rel)
	}
}

func TestNoImportsOfDeletedAuthAdjacentModules(t *testing.T) {
	root := backendRoot(t)
	var banned []string
	for _, name := range []string{"captcha", "session", "usersession", "userloginlog"} {
		banned = append(banned, "admin_back_go/internal/module/"+name)
	}
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
			text := string(body)
			for _, mod := range banned {
				if strings.Contains(text, mod) {
					rel, _ := filepath.Rel(root, path)
					offenders = append(offenders, filepath.ToSlash(rel)+" imports "+mod)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s go files: %v", base, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("banned auth-adjacent imports remain:\n  %s", strings.Join(offenders, "\n  "))
	}
}
