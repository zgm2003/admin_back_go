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
