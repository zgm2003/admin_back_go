package architecture

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/shared/enum"
)

func TestPlatformKernelRegistersOnlyCurrentAdminAdapter(t *testing.T) {
	if got := enum.RegisteredPlatforms(); !reflect.DeepEqual(got, []string{enum.PlatformAdmin}) {
		t.Fatalf("current adapter registry mismatch: %#v", got)
	}
	for _, retired := range []string{"app", "canvas"} {
		if enum.IsRegisteredPlatform(retired) {
			t.Fatalf("retired platform %q is registered", retired)
		}
	}
}

func TestPlatformKernelKeepsCompleteAuthPlatformManagementSurface(t *testing.T) {
	routes := readArchitectureText(t, "internal/server/testdata/admin_routes_golden.txt")
	for _, route := range []string{
		"GET /api/admin/v1/auth-platforms/page-init",
		"GET /api/admin/v1/auth-platforms",
		"POST /api/admin/v1/auth-platforms",
		"PUT /api/admin/v1/auth-platforms/:id",
		"PATCH /api/admin/v1/auth-platforms/:id/status",
		"DELETE /api/admin/v1/auth-platforms/:id",
		"DELETE /api/admin/v1/auth-platforms",
	} {
		if !hasExactLine(routes, route) {
			t.Errorf("platform kernel route is missing: %s", route)
		}
	}
}

func TestPlatformKernelKeepsCrossModulePlatformDimensions(t *testing.T) {
	required := map[string][]string{
		"internal/module/auth_platform/dto.go": {
			"type CreateInput struct", "Code", "LoginTypes", "CaptchaType", "MaxSessions",
		},
		"internal/module/permission/dto.go": {
			"PermissionPlatformArr", "Platform string", `json:"permission_platform_arr"`,
		},
		"internal/module/role/dto.go": {
			"PermissionPlatformArr", `json:"permission_platform_arr"`,
		},
		"internal/module/auth/session_admin.go": {
			"PlatformArr", "PlatformDistribution", `json:"platform_distribution"`,
		},
		"internal/module/auth/loginlog.go": {
			"PlatformArr", "Platform", `json:"platform"`,
		},
		"internal/module/notification/task/dto.go": {
			"PlatformArr", "Platform", `json:"platform"`,
		},
	}
	for relative, tokens := range required {
		body := readArchitectureText(t, relative)
		for _, token := range tokens {
			if !strings.Contains(body, token) {
				t.Errorf("%s lost platform-kernel token %q", relative, token)
			}
		}
	}
}

func TestPlatformKernelAdminAuthIgnoresClientPlatformHeader(t *testing.T) {
	for _, relative := range []string{
		"internal/middleware/auth_token.go",
		"internal/module/auth/transport/admin/handler.go",
	} {
		body := readArchitectureText(t, relative)
		if strings.Contains(body, `GetHeader("platform")`) {
			t.Fatalf("%s trusts client-selected platform provenance", relative)
		}
	}
}

func TestPlatformKernelPrincipalRepositoryHasNoAdminOnlySQLBranch(t *testing.T) {
	body := readArchitectureText(t, "internal/module/permission/principal_repository.go")
	for _, singleton := range []string{
		"platform != enum.PlatformAdmin",
		"subject.Platform != enum.PlatformAdmin",
		`Where("platform = ?", enum.PlatformAdmin)`,
	} {
		if strings.Contains(body, singleton) {
			t.Errorf("principal repository is locked to Admin: %s", singleton)
		}
	}
}

func TestPlatformKernelDefaultManagementQueriesUseRegisteredAdapters(t *testing.T) {
	sessionRepository := readArchitectureText(t, "internal/module/auth/session_admin.go")
	if count := strings.Count(sessionRepository, `Where("us.platform IN ?", enum.RegisteredPlatforms())`); count < 2 {
		t.Fatalf("session list/stats must default to registered adapters, found %d registered scopes", count)
	}
	loginLogRepository := readArchitectureText(t, "internal/module/auth/loginlog.go")
	if !strings.Contains(loginLogRepository, `Where("l.platform IN ?", enum.RegisteredPlatforms())`) {
		t.Fatal("login-log list must default to registered adapters")
	}
}

func TestPlatformKernelSchemaRemainsExtensible(t *testing.T) {
	schema := readArchitectureText(t, "database/schema/admin.hcl")
	for _, token := range []string{
		`table "auth_platforms"`,
		`index "uk_code"`,
		`column "login_types"`,
		`column "captcha_type"`,
		`column "single_session"`,
		`column "max_sessions"`,
		`column "platform"`,
	} {
		if !strings.Contains(schema, token) {
			t.Errorf("canonical schema lost platform-kernel token %q", token)
		}
	}
	for _, singleton := range []string{
		`auth_platforms.code = 'admin'`,
		`platform = 'admin'`,
	} {
		if strings.Contains(schema, singleton) {
			t.Errorf("canonical schema was locked to a permanent singleton: %s", singleton)
		}
	}
}

func readArchitectureText(t *testing.T, relative string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(backendRoot(t), filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(body)
}

func hasExactLine(body, expected string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == expected {
			return true
		}
	}
	return false
}
