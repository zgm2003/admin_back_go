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
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
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

	for name, schema := range document.Components.Schemas {
		if strings.Contains(strings.ToLower(name), "clientvariant") {
			t.Fatalf("generated Admin OpenAPI contains retired schema %q", name)
		}
		walkParsedContract(schema, func(location map[string]any) {
			properties, ok := location["properties"].(map[string]any)
			if !ok {
				return
			}
			for _, property := range []string{"refresh_token", "refresh_expires_in"} {
				if _, exists := properties[property]; exists {
					t.Fatalf("generated Admin OpenAPI contains retired public property %q", property)
				}
			}
		})
	}
	var parsed any
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &parsed); err != nil {
		t.Fatalf("decode generic Admin OpenAPI: %v", err)
	}
	walkParsedContract(parsed, func(location map[string]any) {
		name, hasName := location["name"].(string)
		in, hasIn := location["in"].(string)
		if hasName && hasIn && strings.EqualFold(in, "header") && strings.EqualFold(name, "X-Admin-Client-Variant") {
			t.Fatal("generated Admin OpenAPI contains retired ClientVariant header parameter")
		}
	})
}

func TestBrowserOnlyAdminHasNoClientVersionRuntimeAndP09OwnsHistoryDeletion(t *testing.T) {
	root := backendRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "module", "clientversion")); !os.IsNotExist(err) {
		t.Fatalf("client-version runtime module still exists: %v", err)
	}

	bundle, err := admincontract.Build(admincontract.BuildOptions{BackendCommit: strings.Repeat("c", 40)})
	if err != nil {
		t.Fatalf("build Admin contract: %v", err)
	}
	var openAPI struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &openAPI); err != nil {
		t.Fatalf("decode Admin OpenAPI: %v", err)
	}
	for path := range openAPI.Paths {
		if strings.HasPrefix(path, "/api/admin/v1/client-versions") {
			t.Fatalf("OpenAPI still publishes retired client-version path %q", path)
		}
	}
	var permissions struct {
		PermissionCodes []string `json:"permission_codes"`
	}
	if err := json.Unmarshal(bundle.Artifacts["permissions.json"], &permissions); err != nil {
		t.Fatalf("decode Admin permissions: %v", err)
	}
	for _, code := range permissions.PermissionCodes {
		if strings.HasPrefix(code, "system_clientVersion_") {
			t.Fatalf("permissions still publish retired client-version code %q", code)
		}
	}
	var views struct {
		Views []struct {
			Path    string `json:"path"`
			ViewKey string `json:"view_key"`
			I18nKey string `json:"i18n_key"`
		} `json:"views"`
	}
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &views); err != nil {
		t.Fatalf("decode Admin views: %v", err)
	}
	for _, view := range views.Views {
		if view.Path == "/system/clientVersion" || view.ViewKey == "system/clientVersion" || view.I18nKey == "menu.system_clientVersion" {
			t.Fatalf("views still publish retired client-version view %#v", view)
		}
	}

	for _, folder := range enum.UploadFolders {
		if folder == "releases" || folder == "tauri_updater" {
			t.Fatalf("retired updater upload folder %q remains active", folder)
		}
	}

	schema, err := os.ReadFile(filepath.Join(root, "database", "schema.sql"))
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}
	if strings.Contains(strings.ToLower(string(schema)), "create table `client_versions`") {
		t.Fatal("P09 canonical schema must not retain the frozen client_versions history table")
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

func walkParsedContract(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walkParsedContract(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkParsedContract(child, visit)
		}
	}
}
