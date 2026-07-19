package admincontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/bootstrap"
	"admin_back_go/internal/server"
	"admin_back_go/internal/server/adminroute"
)

const testBackendCommit = "0123456789abcdef0123456789abcdef01234567"

func TestBundleIsDeterministicAndAdminOnly(t *testing.T) {
	first := mustBuildBundle(t)
	second := mustBuildBundle(t)
	if !reflect.DeepEqual(first.Files(), second.Files()) {
		t.Fatal("bundle generation is not deterministic")
	}

	wantNames := []string{
		"manifest.json",
		"openapi.json",
		"permissions.json",
		"realtime/envelope.schema.json",
		"realtime/events.schema.json",
		"views.json",
	}
	if got := sortedFileNames(first.Files()); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("files=%v want=%v", got, wantNames)
	}
	for name, data := range first.Files() {
		if bytes.Contains(data, []byte("/api/app/")) || bytes.Contains(data, []byte("/api/canvas/")) {
			t.Fatalf("%s contains retired operation", name)
		}
	}
}

func TestCheckedInBundlePublishesBrowserOnlyContractAndExactHashes(t *testing.T) {
	files := readCheckedInBundleFiles(t)
	assertBrowserOnlyOpenAPI(t, files["openapi.json"])

	var permissions PermissionsDocument
	if err := json.Unmarshal(files["permissions.json"], &permissions); err != nil {
		t.Fatalf("decode checked-in permissions: %v", err)
	}
	for _, code := range permissions.PermissionCodes {
		if strings.HasPrefix(code, "system_clientVersion_") {
			t.Fatalf("checked-in permissions still publish retired code %s", code)
		}
	}
	for _, operation := range permissions.Operations {
		if strings.HasPrefix(operation.Path, "/api/admin/v1/client-versions") {
			t.Fatalf("checked-in permissions still publish retired operation %s %s", operation.Method, operation.Path)
		}
		if strings.HasPrefix(operation.Access.PermissionCode, "system_clientVersion_") {
			t.Fatalf("checked-in operation still publishes retired permission %s", operation.Access.PermissionCode)
		}
	}

	var views ViewsDocument
	if err := json.Unmarshal(files["views.json"], &views); err != nil {
		t.Fatalf("decode checked-in views: %v", err)
	}
	for _, view := range views.Views {
		if view.ViewKey == "system/clientVersion" || view.Path == "/system/clientVersion" || view.I18nKey == "menu.system_clientVersion" {
			t.Fatalf("checked-in views still publish retired view %#v", view)
		}
		for _, code := range view.PermissionCodes {
			if strings.HasPrefix(code, "system_clientVersion_") {
				t.Fatalf("checked-in view still publishes retired permission %s", code)
			}
		}
	}
	assertNoRetiredClientVersionJSONValue(t, "views.users_me.response_schema", views.UsersMe.ResponseSchema)

	var manifest Manifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode checked-in manifest: %v", err)
	}
	if len(files) != len(manifest.Artifacts)+1 {
		t.Fatalf("checked-in file count=%d manifest artifacts=%d", len(files), len(manifest.Artifacts))
	}
	for name, artifact := range manifest.Artifacts {
		data, exists := files[name]
		if !exists {
			t.Fatalf("checked-in manifest references missing artifact %s", name)
		}
		hash := sha256.Sum256(data)
		if artifact.SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatalf("checked-in manifest hash mismatch for %s", name)
		}
	}
	manifestHash := sha256.Sum256(files["manifest.json"])
	authContract, err := os.ReadFile(filepath.Join("..", "..", "docs", "contracts", "admin-browser-auth-contract.md"))
	if err != nil {
		t.Fatalf("read Browser-only activation contract: %v", err)
	}
	for _, marker := range []string{
		"bundle_version=" + manifest.BundleVersion,
		"backend_source_commit=" + manifest.BackendCommit,
		"manifest_sha256=" + hex.EncodeToString(manifestHash[:]),
	} {
		if !bytes.Contains(authContract, []byte(marker)) {
			t.Fatalf("Browser-only activation contract is missing %s", marker)
		}
	}
}

func readCheckedInBundleFiles(t *testing.T) map[string][]byte {
	t.Helper()
	root := filepath.Join("..", "..", "contracts", "admin", "v1")
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("read checked-in bundle: %v", err)
	}
	return files
}

func assertNoRetiredClientVersionJSONValue(t *testing.T, location string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			assertNoRetiredClientVersionJSONValue(t, location+"/"+key, child)
		}
	case []any:
		for index, child := range typed {
			assertNoRetiredClientVersionJSONValue(t, fmt.Sprintf("%s/%d", location, index), child)
		}
	case string:
		if typed == "system/clientVersion" || typed == "/system/clientVersion" || typed == "menu.system_clientVersion" || strings.HasPrefix(typed, "system_clientVersion_") {
			t.Fatalf("%s still publishes retired value %s", location, typed)
		}
	}
}

func TestWriteAtomicAndCheckDetectsContractDrift(t *testing.T) {
	bundle := mustBuildBundle(t)
	output := filepath.Join(t.TempDir(), "contracts", "admin", "v1")
	if err := WriteAtomic(output, bundle); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	if err := Check(output, bundle); err != nil {
		t.Fatalf("check fresh bundle: %v", err)
	}

	openAPIPath := filepath.Join(output, "openapi.json")
	data, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	data[0] ^= 1
	if err := os.WriteFile(openAPIPath, data, 0o644); err != nil {
		t.Fatalf("tamper openapi: %v", err)
	}
	if err := Check(output, bundle); err == nil || !strings.Contains(err.Error(), "openapi.json") {
		t.Fatalf("expected named byte drift, got %v", err)
	}

	if err := WriteAtomic(output, bundle); err != nil {
		t.Fatalf("restore bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(output, "unexpected.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write unexpected file: %v", err)
	}
	if err := Check(output, bundle); err == nil || !strings.Contains(err.Error(), "unexpected.json") {
		t.Fatalf("expected extra-file drift, got %v", err)
	}
}

func mustBuildBundle(t *testing.T) Bundle {
	t.Helper()
	bundle, err := Build(BuildOptions{BackendCommit: testBackendCommit})
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	return bundle
}

func runtimeContractDefinitions(t *testing.T) []adminroute.Definition {
	t.Helper()
	registry, err := bootstrap.AdminRouteRegistry()
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	if _, err := server.NewRouter(server.Dependencies{
		Core: server.CoreDependencies{RouteRegistry: registry},
	}); err != nil {
		t.Fatalf("compile runtime routes: %v", err)
	}
	definitions := make([]adminroute.Definition, 0)
	for _, definition := range registry.Definitions() {
		if isAdminContractPath(definition.Path) {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}
