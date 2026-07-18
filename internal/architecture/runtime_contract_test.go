package architecture

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

func TestRuntimeCompositionRootsStayInsideRuntimePackage(t *testing.T) {
	root := backendRoot(t)
	var offenders []string
	for _, base := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, _ := filepath.Rel(root, path)
			relative = filepath.ToSlash(relative)
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(body)
			for _, banned := range []string{"bootstrap.New(", "bootstrap.NewWorker("} {
				if strings.Contains(text, banned) {
					offenders = append(offenders, relative+" uses "+banned)
				}
			}
			if strings.HasPrefix(relative, "internal/runtime/") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, body, 0)
			if err != nil {
				return err
			}
			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Recv != nil {
					continue
				}
				if function.Name.Name == "NewAPI" || function.Name.Name == "NewWorker" {
					offenders = append(offenders, relative+" declares process constructor "+function.Name.Name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("process composition must stay in internal/runtime:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestRuntimeServerDependenciesStayGrouped(t *testing.T) {
	root := backendRoot(t)
	path := filepath.Join(root, "internal", "server", "router.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse server router: %v", err)
	}
	fieldCount := -1
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Dependencies" {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatal("server.Dependencies must remain a struct")
			}
			fieldCount = 0
			for _, field := range structure.Fields.List {
				if len(field.Names) == 0 {
					fieldCount++
					continue
				}
				fieldCount += len(field.Names)
			}
		}
	}
	if fieldCount < 0 {
		t.Fatal("server.Dependencies was not found")
	}
	if fieldCount > 10 {
		t.Fatalf("server.Dependencies has %d top-level fields; maximum is 10", fieldCount)
	}
}

func TestRuntimeCapabilityServicesAndRepositoriesDoNotImportGin(t *testing.T) {
	root := backendRoot(t)
	moduleRoot := filepath.Join(root, "internal", "module")
	var offenders []string
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasPrefix(base, "service") && !strings.HasPrefix(base, "repository") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			if strings.Trim(imported.Path.Value, `"`) == "github.com/gin-gonic/gin" {
				relative, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(relative))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk capability modules: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("capability service/repository files import Gin:\n  %s", strings.Join(offenders, "\n  "))
	}
}

type serializableRuntimeCause struct {
	Secret string `json:"secret"`
}

func (cause *serializableRuntimeCause) Error() string { return "private dependency failure" }

func TestRuntimeApplicationErrorCannotSerializeRawCause(t *testing.T) {
	applicationError := apperror.Wrap(
		"dependency.test",
		apperror.CategoryDependency,
		503,
		apperror.Retryable,
		"common.dependency_unavailable",
		nil,
		"dependency unavailable",
		&serializableRuntimeCause{Secret: "raw-cause-secret"},
	)
	encoded, err := json.Marshal(applicationError)
	if err != nil {
		t.Fatalf("marshal application error: %v", err)
	}
	if bytes.Contains(encoded, []byte("raw-cause-secret")) || bytes.Contains(encoded, []byte(`"Cause"`)) {
		t.Fatalf("raw application error serialized its cause: %s", encoded)
	}
}

func TestRuntimeConfigIsSnapshottedBeforeHooksAreBuilt(t *testing.T) {
	root := backendRoot(t)
	for _, relative := range []string{"internal/runtime/api.go", "internal/runtime/worker.go"} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		if !strings.Contains(string(body), "cfg = config.Snapshot(cfg)") {
			t.Fatalf("%s must snapshot config before runtime hooks capture it", relative)
		}
	}
}

func TestAdminContractExcludesRetiredProductOperations(t *testing.T) {
	root := backendRoot(t)
	contractRoot := filepath.Join(root, "contracts", "admin")
	var offenders []string
	err := filepath.WalkDir(contractRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, retired := range []string{"/api/app/", "/api/canvas/"} {
			if bytes.Contains(body, []byte(retired)) {
				relative, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(relative)+" contains "+retired)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk Admin contract: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("retired product operations leaked into Admin contract:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestRoutePolicyRegistryHasNoNewLegacyMaps(t *testing.T) {
	root := backendRoot(t)
	allowed := map[string]struct{}{
		"internal/middleware/operation_log.go":   {}, // Compiled runtime lookup consumer.
		"internal/server/adminroute/compile.go":  {},
		"internal/server/adminroute/registry.go": {},
	}
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		if _, ok := allowed[relative]; ok {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, token := range []string{"permissionRouteRules", "operationRouteRules", "map[middleware.RouteKey]"} {
			if strings.Contains(text, token) {
				offenders = append(offenders, relative+" contains "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk route policy sources: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("route policy maps exist outside the registry boundary:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestRuntimeContractVerificationIsBlockingAndDocumented(t *testing.T) {
	root := backendRoot(t)
	runtimeScript := runtimeContractReadFile(t, root, "scripts/verify-runtime-contracts.ps1")
	for _, token := range []string{
		"-race",
		"./internal/runtime",
		"./internal/platform/admin",
		"./internal/server/...",
		"./internal/admincontract",
		"./internal/telemetry",
		"./internal/module/auth",
		"./internal/module/payment/...",
		"./internal/infra/taskqueue",
		"./internal/infra/realtime/...",
		"admin-go-race-modcache",
		"admin-go-race-buildcache",
		"scripts/check-admin-contract.ps1",
		"TestRuntime|TestAdminContract|TestRoutePolicy",
		"./cmd/admin-api",
		"./cmd/admin-worker",
	} {
		if !bytes.Contains(runtimeScript, []byte(token)) {
			t.Errorf("verify-runtime-contracts.ps1 must contain %q", token)
		}
	}
	for _, relative := range []string{"scripts/verify-backend.ps1", "scripts/verify-go-clean.ps1"} {
		body := runtimeContractReadFile(t, root, relative)
		if !bytes.Contains(body, []byte("verify-runtime-contracts.ps1")) {
			t.Errorf("%s must invoke the shared runtime contract gate", relative)
		}
		if !bytes.Contains(body, []byte("honnef.co/go/tools/cmd/staticcheck@v0.8.0-rc.1")) {
			t.Errorf("%s must pin the Go 1.26-compatible staticcheck release", relative)
		}
	}
	architecture := runtimeContractReadFile(t, root, "docs/architecture.md")
	context := runtimeContractReadFile(t, root, "CONTEXT.md")
	for name, body := range map[string][]byte{"docs/architecture.md": architecture, "CONTEXT.md": context} {
		for _, token := range []string{"verify-runtime-contracts.ps1", "config.Snapshot"} {
			if !bytes.Contains(body, []byte(token)) {
				t.Errorf("%s must document %q", name, token)
			}
		}
	}
}

func runtimeContractReadFile(t *testing.T, root string, relative string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return body
}
