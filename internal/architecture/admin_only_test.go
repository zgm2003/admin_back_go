package architecture

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func backendRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
}

func mustExist(t *testing.T, root, relative string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
		t.Fatalf("expected %s to exist: %v", relative, err)
	}
}

func mustNotExist(t *testing.T, root, relative string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
		t.Fatalf("expected %s to be absent", relative)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect %s: %v", relative, err)
	}
}

func TestAdminOnlyRuntimeHasNoRetiredProductRoutesOrImports(t *testing.T) {
	root := backendRoot(t)
	banned := []string{
		`/api/app/`,
		`/api/canvas/`,
		`admin_back_go/internal/platform/retired`,
		`admin_back_go/internal/module/canvas`,
		`/transport/app"`,
		`/transport/canvas"`,
	}
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
			relative, _ := filepath.Rel(root, path)
			for _, token := range banned {
				if strings.Contains(string(body), token) {
					offenders = append(offenders, filepath.ToSlash(relative)+" contains "+token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s runtime: %v", base, err)
		}
	}
	if len(offenders) != 0 {
		sort.Strings(offenders)
		t.Fatalf("retired product runtime remains:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestAdminOnlyRetiredTransportPackagesAreAbsent(t *testing.T) {
	root := backendRoot(t)
	for _, relative := range []string{
		"internal/server/routes_canvas.go",
		"internal/platform/retired",
		"internal/module/canvas",
		"internal/module/auth/transport/app",
		"internal/module/auth/transport/canvas",
		"internal/module/profile/transport/app",
		"internal/module/profile/transport/canvas",
		"internal/module/uploadtoken/transport/app",
		"internal/module/user/transport/app",
		"internal/module/user/transport/canvas",
		"internal/module/payment/transport/canvas",
		"internal/module/payment/wallet/transport/canvas",
		"internal/module/ai/asset/transport/canvas",
		"internal/module/ai/audio",
		"internal/module/ai/chat/transport/canvas",
		"internal/module/ai/image/transport/canvas",
		"internal/module/ai/prompt/transport/canvas",
		"internal/module/ai/video",
		"internal/module/ai/internal/canvasrequest",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			t.Errorf("retired runtime path still exists: %s", relative)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", relative, err)
		}
	}
}

func TestAdminOnlyPromptCoreRemainsWithoutAdminTransport(t *testing.T) {
	root := backendRoot(t)
	mustNotExist(t, root, "internal/module/ai/prompt/transport/admin")
	for _, relative := range []string{
		"internal/module/ai/prompt/model.go",
		"internal/module/ai/prompt/repository.go",
		"internal/module/ai/prompt/service.go",
	} {
		mustExist(t, root, relative)
	}
	model, err := os.ReadFile(filepath.Join(root, "internal", "module", "ai", "prompt", "model.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(model), `return "ai_prompts"`) {
		t.Fatal("transport-neutral Prompt model no longer retains the empty ai_prompts table")
	}

	for _, relative := range []string{
		"internal/server/routes_admin_ai.go",
		"internal/server/testdata/admin_routes_golden.txt",
		"internal/server/testdata/admin_route_policy_golden.json",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		if strings.Contains(string(body), "/api/admin/v1/ai-prompts") || strings.Contains(string(body), "ai_prompt_") {
			t.Errorf("%s still exposes retired Prompt administration", relative)
		}
	}
}

func TestAdminOnlyCompiledRouteGoldenContainsNoRetiredProductPrefix(t *testing.T) {
	root := backendRoot(t)
	for _, relative := range []string{
		"internal/server/testdata/admin_routes_golden.txt",
		"internal/server/testdata/admin_route_policy_golden.json",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, prefix := range []string{"/api/app/", "/api/canvas/"} {
			if strings.Contains(string(body), prefix) {
				t.Errorf("%s still contains %s", relative, prefix)
			}
		}
	}
}
