package admincontract

import (
	"bytes"
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
