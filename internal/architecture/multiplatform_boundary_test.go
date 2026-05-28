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

func TestFoundationAdminTransportShells(t *testing.T) {
	root := backendRoot(t)
	for _, module := range []string{
		"system",
		"systemsetting",
		"systemlog",
		"operationlog",
		"crontask",
		"queuemonitor",
		"clientversion",
		"exporttask",
		"realtime",
	} {
		moduleRoot := "internal/module/" + module + "/"
		mustExist(t, root, moduleRoot+"transport/admin/route.go")
		mustNotExist(t, root, moduleRoot+"route.go")
		mustNotExist(t, root, moduleRoot+"handler.go")
	}
}

func TestAIAdminTransportShells(t *testing.T) {
	root := backendRoot(t)
	for _, module := range []string{
		"aiprovider",
		"aiagent",
		"aitool",
		"aiknowledge",
		"aiconversation",
		"aimessage",
		"airun",
		"aichat",
		"aiimage",
	} {
		moduleRoot := "internal/module/" + module + "/"
		mustExist(t, root, moduleRoot+"transport/admin/route.go")
		mustNotExist(t, root, moduleRoot+"route.go")
		mustNotExist(t, root, moduleRoot+"handler.go")
	}
}

func TestCommsUploadAdminTransportShells(t *testing.T) {
	root := backendRoot(t)
	for _, module := range []string{
		"mail",
		"sms",
		"notification",
		"notificationtask",
		"uploadconfig",
		"uploadtoken",
	} {
		moduleRoot := "internal/module/" + module + "/"
		mustExist(t, root, moduleRoot+"transport/admin/route.go")
		mustNotExist(t, root, moduleRoot+"route.go")
		mustNotExist(t, root, moduleRoot+"handler.go")
	}
}

func TestCommerceRBACAdminTransportShells(t *testing.T) {
	root := backendRoot(t)
	for _, module := range []string{
		"auth_platform",
		"permission",
		"role",
		"wallet",
		"payment",
	} {
		moduleRoot := "internal/module/" + module + "/"
		mustExist(t, root, moduleRoot+"transport/admin/route.go")
		mustNotExist(t, root, moduleRoot+"route.go")
		mustNotExist(t, root, moduleRoot+"handler.go")
	}
}

func TestAuthPlatformModuleUsesSnakeCaseDirectory(t *testing.T) {
	root := backendRoot(t)
	mustNotExist(t, root, "internal/module/authplatform")
	mustExist(t, root, "internal/module/auth_platform/transport/admin/route.go")
}

func TestUserProfileTransportShape(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/user/transport/admin/route.go",
		"internal/module/profile/transport/admin/route.go",
		"internal/module/profile/transport/app/route.go",
	} {
		mustExist(t, root, rel)
	}
	for _, rel := range []string{
		"internal/module/user/route.go",
		"internal/module/user/handler.go",
		"internal/module/user/app_handler.go",
		"internal/module/user/app_dto.go",
		"internal/module/userquickentry/route.go",
		"internal/module/userquickentry/handler.go",
	} {
		mustNotExist(t, root, rel)
	}
}

func TestNoModuleRootHTTPSurface(t *testing.T) {
	root := backendRoot(t)
	moduleRoot := filepath.Join(root, "internal", "module")
	bannedNames := map[string]struct{}{
		"route.go":            {},
		"handler.go":          {},
		"app_handler.go":      {},
		"platform_handler.go": {},
		"app_route_test.go":   {},
		"platform_route.go":   {},
	}
	// Root module files are reserved for business/runtime code. If a future
	// non-HTTP file trips this guard, add a tiny explicit allowlist entry here
	// with owner, reason, and removal plan instead of weakening the rule.
	allowed := map[string]string{}

	var offenders []string
	entries, err := os.ReadDir(moduleRoot)
	if err != nil {
		t.Fatalf("read internal/module: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		moduleDir := filepath.Join(moduleRoot, entry.Name())
		files, err := os.ReadDir(moduleDir)
		if err != nil {
			t.Fatalf("read module %s: %v", entry.Name(), err)
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			if _, banned := bannedNames[file.Name()]; !banned {
				continue
			}
			rel, _ := filepath.Rel(root, filepath.Join(moduleDir, file.Name()))
			rel = filepath.ToSlash(rel)
			if _, ok := allowed[rel]; ok {
				continue
			}
			offenders = append(offenders, rel)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("module root HTTP surface files must move under transport/{platform}:\n  %s", strings.Join(offenders, "\n  "))
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
