package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiPlatformPhase2ClosureRuntimeShape(t *testing.T) {
	root := backendRoot(t)

	required := []string{
		"internal/infra",
		"internal/shared/apperror",
		"internal/shared/response",
		"internal/shared/i18n",
		"internal/shared/enum",
		"internal/shared/validate",
		"internal/shared/dict",
		"internal/shared/setting",
		"internal/module/auth/transport/admin/route.go",
		"internal/module/auth/transport/app/route.go",
		"internal/module/profile/transport/admin/route.go",
		"internal/module/profile/transport/app/route.go",
		"internal/module/user/transport/admin/route.go",
		"internal/module/auth_platform/transport/admin/route.go",
		"internal/module/notification/task",
		"internal/module/notification/transport/admin/task_route.go",
		"internal/module/export/transport/admin/route.go",
		"internal/module/ai/provider/transport/admin/route.go",
		"internal/module/ai/agent/transport/admin/route.go",
		"internal/module/ai/tool/transport/admin/route.go",
		"internal/module/ai/image/transport/canvas/route.go",
		"internal/module/ai/knowledge/transport/admin/route.go",
		"internal/module/ai/conversation/transport/admin/route.go",
		"internal/module/ai/message/transport/admin/route.go",
		"internal/module/ai/chat/transport/admin/route.go",
		"internal/module/ai/run/transport/admin/route.go",
		"internal/module/payment/transport/admin/route.go",
		"internal/module/payment/transport/callback/route.go",
		"internal/module/payment/wallet/transport/admin/route.go",
	}
	for _, rel := range required {
		mustExist(t, root, rel)
	}

	removed := []string{
		"internal/platform",
		"internal/apperror",
		"internal/response",
		"internal/i18n",
		"internal/enum",
		"internal/validate",
		"internal/dict",
		"internal/module/captcha",
		"internal/module/session",
		"internal/module/usersession",
		"internal/module/userloginlog",
		"internal/module/userquickentry",
		"internal/module/notificationtask",
		"internal/module/exporttask",
		"internal/module/authplatform",
		"internal/module/aiprovider",
		"internal/module/aiagent",
		"internal/module/aitool",
		"internal/module/aiimage",
		"internal/module/ai/image/transport/admin/route.go",
		"internal/module/aiknowledge",
		"internal/module/aiconversation",
		"internal/module/aimessage",
		"internal/module/aichat",
		"internal/module/airun",
		"internal/module/wallet",
	}
	for _, rel := range removed {
		mustNotExist(t, root, rel)
	}
}

func TestMultiPlatformPhase2ClosureNoLegacyProductionImports(t *testing.T) {
	root := backendRoot(t)
	bannedImports := prefixedImportPaths("admin_back_go/internal/", []string{
		"platform",
		"apperror",
		"response",
		"i18n",
		"enum",
		"validate",
		"dict",
	})
	bannedImports = append(bannedImports, prefixedImportPaths("admin_back_go/internal/module/", []string{
		"captcha",
		"session",
		"usersession",
		"userloginlog",
		"userquickentry",
		"notificationtask",
		"exporttask",
		"authplatform",
		"aiprovider",
		"aiagent",
		"aitool",
		"aiimage",
		"aiknowledge",
		"aiconversation",
		"aimessage",
		"aichat",
		"airun",
		"wallet",
	})...)

	var offenders []string
	for _, base := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(body)
			for _, banned := range bannedImports {
				if strings.Contains(text, banned) {
					rel, _ := filepath.Rel(root, path)
					offenders = append(offenders, filepath.ToSlash(rel)+" references "+banned)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s go files: %v", base, err)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("legacy Phase 2 production imports remain:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func prefixedImportPaths(prefix string, names []string) []string {
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, prefix+name)
	}
	return paths
}
