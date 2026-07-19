package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"admin_back_go/internal/admincontract"
	"admin_back_go/internal/config"
	"admin_back_go/internal/shared/enum"
)

func TestBrowserOnlyAdminRejectsVariantProductionSurface(t *testing.T) {
	root := backendRoot(t)
	banned := []string{
		"ClientVariant",
		"ClientDesktop",
		"ClientBrowser",
		"X-Admin-Client-Variant",
	}
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
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
		t.Fatalf("scan production Go: %v", err)
	}
	for _, header := range config.DefaultCORSConfig().AllowHeaders {
		if strings.EqualFold(header, "X-Admin-Client-Variant") {
			offenders = append(offenders, "DefaultCORSConfig allows X-Admin-Client-Variant")
		}
	}
	if len(offenders) != 0 {
		sort.Strings(offenders)
		t.Fatalf("Browser-only production violations:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestBrowserOnlyAdminGeneratedCredentialContract(t *testing.T) {
	bundle, err := admincontract.Build(admincontract.BuildOptions{BackendCommit: strings.Repeat("b", 40)})
	if err != nil {
		t.Fatalf("build Admin contract: %v", err)
	}
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode Admin OpenAPI: %v", err)
	}

	for _, path := range []string{"/api/admin/v1/auth/refresh", "/api/admin/v1/auth/logout"} {
		operation := document.Paths[path]["post"]
		if operation == nil {
			t.Fatalf("missing POST %s", path)
		}
		if _, exists := operation["requestBody"]; exists {
			t.Fatalf("POST %s publishes a forbidden requestBody", path)
		}
	}

	encoded := string(bundle.Artifacts["openapi.json"])
	for _, token := range []string{"ClientVariant", "X-Admin-Client-Variant", `"refresh_token"`, `"refresh_expires_in"`} {
		if strings.Contains(encoded, token) {
			t.Fatalf("generated Admin OpenAPI contains retired token %q", token)
		}
	}
}

func TestBrowserOnlyAdminHasNoClientVersionRuntimeButKeepsHistoryTable(t *testing.T) {
	root := backendRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "module", "clientversion")); !os.IsNotExist(err) {
		t.Fatalf("client-version runtime module still exists: %v", err)
	}

	bundle, err := admincontract.Build(admincontract.BuildOptions{BackendCommit: strings.Repeat("c", 40)})
	if err != nil {
		t.Fatalf("build Admin contract: %v", err)
	}
	for artifact, tokens := range map[string][]string{
		"openapi.json":     {"/api/admin/v1/client-versions"},
		"permissions.json": {"system_clientVersion_"},
		"views.json":       {"system/clientVersion", "menu.system_clientVersion"},
	} {
		body := string(bundle.Artifacts[artifact])
		for _, token := range tokens {
			if strings.Contains(body, token) {
				t.Fatalf("%s still contains retired client-version token %q", artifact, token)
			}
		}
	}

	for _, folder := range enum.UploadFolders {
		if folder == "releases" || folder == "tauri_updater" {
			t.Fatalf("retired updater upload folder %q remains active", folder)
		}
	}

	schema, err := os.ReadFile(filepath.Join(root, "database", "schema", "admin.hcl"))
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}
	if !strings.Contains(string(schema), `table "client_versions"`) {
		t.Fatal("P08R must freeze, not drop, the client_versions history table")
	}

	smoke, err := os.ReadFile(filepath.Join(root, "scripts", "full-admin-smoke.ps1"))
	if err != nil {
		t.Fatalf("read full smoke: %v", err)
	}
	if strings.Contains(string(smoke), "Assert-ClientVersion") {
		t.Fatal("full smoke still contains positive client-version contract assertions")
	}
	for _, proof := range []string{
		"client_version_unauthenticated_absence_status",
		"client_version_authenticated_absence_status",
		"route absence 404",
	} {
		if !strings.Contains(string(smoke), proof) {
			t.Fatalf("full smoke is missing retired-route proof %q", proof)
		}
	}
}
