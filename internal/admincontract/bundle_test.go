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

func TestBundlePublishesCurrentAdminPlatformKernel(t *testing.T) {
	bundle := mustBuildBundle(t)

	var openAPI struct {
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &openAPI); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}

	t.Run("publishes bundle version 2", func(t *testing.T) {
		if bundle.Manifest.BundleVersion != "admin-2026-07-15.2" {
			t.Fatalf("bundle version=%q", bundle.Manifest.BundleVersion)
		}
	})

	t.Run("retires Admin Prompt transport", func(t *testing.T) {
		for path := range openAPI.Paths {
			if strings.HasPrefix(path, "/api/admin/v1/ai-prompts") {
				t.Fatalf("retired Prompt operation remains: %s", path)
			}
		}
		for name := range openAPI.Components.Schemas {
			normalized := strings.ToLower(strings.ReplaceAll(name, "_", ""))
			if strings.Contains(normalized, "aiprompt") {
				t.Fatalf("retired Prompt transport schema remains: %s", name)
			}
		}

		var permissions PermissionsDocument
		if err := json.Unmarshal(bundle.Artifacts["permissions.json"], &permissions); err != nil {
			t.Fatalf("decode permissions: %v", err)
		}
		for _, code := range permissions.PermissionCodes {
			if strings.HasPrefix(code, "ai_prompt_") {
				t.Fatalf("retired Prompt permission remains: %s", code)
			}
		}
		for _, operation := range permissions.Operations {
			if strings.HasPrefix(operation.Path, "/api/admin/v1/ai-prompts") {
				t.Fatalf("retired Prompt policy remains: %s %s", operation.Method, operation.Path)
			}
		}

		var views ViewsDocument
		if err := json.Unmarshal(bundle.Artifacts["views.json"], &views); err != nil {
			t.Fatalf("decode views: %v", err)
		}
		for _, view := range views.Views {
			if view.Path == "/ai/prompts" || view.ViewKey == "ai/prompts" || view.I18nKey == "menu.ai_prompts" {
				t.Fatalf("retired Prompt view remains: %#v", view)
			}
			for _, code := range view.PermissionCodes {
				if strings.HasPrefix(code, "ai_prompt_") {
					t.Fatalf("retired Prompt view permission remains: %s", code)
				}
			}
		}
	})

	t.Run("publishes all auth-platform management operations", func(t *testing.T) {
		expected := []struct {
			method string
			path   string
		}{
			{method: "get", path: "/api/admin/v1/auth-platforms/page-init"},
			{method: "get", path: "/api/admin/v1/auth-platforms"},
			{method: "post", path: "/api/admin/v1/auth-platforms"},
			{method: "put", path: "/api/admin/v1/auth-platforms/{id}"},
			{method: "patch", path: "/api/admin/v1/auth-platforms/{id}/status"},
			{method: "delete", path: "/api/admin/v1/auth-platforms/{id}"},
			{method: "delete", path: "/api/admin/v1/auth-platforms"},
		}
		for _, item := range expected {
			operation := openAPI.Paths[item.path][item.method]
			if operation == nil {
				t.Errorf("missing %s %s", strings.ToUpper(item.method), item.path)
				continue
			}
			if operation["x-admin-access"] == nil || operation["x-admin-audit"] == nil {
				t.Errorf("%s %s has no access/audit policy", strings.ToUpper(item.method), item.path)
			}
		}
	})

	t.Run("publishes only current registered platform enums", func(t *testing.T) {
		var decoded any
		if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &decoded); err != nil {
			t.Fatal(err)
		}
		assertNoRetiredGeneratedEnum(t, "$", decoded)

		assertSchemaPropertyStringEnum(t, openAPI.Components.Schemas, "AIRunListItem", "platform", []string{"admin"})
		assertSchemaPropertyStringEnum(t, openAPI.Components.Schemas, "AIRunDetail", "platform", []string{"admin"})
		assertSchemaPropertyStringEnum(t, openAPI.Components.Schemas, "post_api_admin_v1_permissions_Request", "platform", []string{"admin"})
		assertSchemaPropertyStringEnum(t, openAPI.Components.Schemas, "put_api_admin_v1_permissions_id_Request", "platform", []string{"admin"})
		assertSchemaPropertyStringEnum(t, openAPI.Components.Schemas, "post_api_admin_v1_notification_tasks_Request", "platform", []string{"all", "admin"})

		assertQueryStringEnum(t, openAPI.Paths["/api/admin/v1/permissions"]["get"], "platform", []string{"admin"})
		for _, path := range []string{
			"/api/admin/v1/ai-runs",
			"/api/admin/v1/ai-runs/stats",
			"/api/admin/v1/ai-runs/stats/by-agent",
			"/api/admin/v1/ai-runs/stats/by-date",
			"/api/admin/v1/ai-runs/stats/by-user",
		} {
			assertQueryStringEnum(t, openAPI.Paths[path]["get"], "platform", []string{"admin"})
		}
	})

	t.Run("retains generic platform fields", func(t *testing.T) {
		for schema, property := range map[string]string{
			"Go_internal_module_permission_PermissionDict_Output":     "permission_platform_arr",
			"Go_internal_module_permission_PermissionTreeNode_Output": "platform",
			"Go_internal_module_role_InitDict_Output":                 "permission_platform_arr",
			"Go_internal_module_auth_SessionPageInitDict_Output":      "platformArr",
			"Go_internal_module_auth_SessionListItem_Output":          "platform",
			"Go_internal_module_auth_SessionStatsResponse_Output":     "platform_distribution",
			"Go_internal_module_auth_LoginLogPageInitDict_Output":     "platformArr",
			"Go_internal_module_auth_LoginLogListItem_Output":         "platform",
			"Go_internal_module_notification_task_InitDict_Output":    "platformArr",
			"Go_internal_module_notification_task_ListItem_Output":    "platform",
		} {
			assertSchemaProperty(t, openAPI.Components.Schemas, schema, property)
		}
		assertQueryParameter(t, openAPI.Paths["/api/admin/v1/user-sessions"]["get"], "platform")
		assertQueryParameter(t, openAPI.Paths["/api/admin/v1/users/login-logs"]["get"], "platform")

		createPlatform := openAPI.Components.Schemas["post_api_admin_v1_auth_platforms_Request"]
		code := schemaProperty(t, createPlatform, "code")
		if code["pattern"] == nil || code["enum"] != nil {
			t.Fatalf("auth-platform code must remain an extensible validated string: %#v", code)
		}
	})
}

func assertNoRetiredGeneratedEnum(t *testing.T, location string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if rawEnum, ok := typed["enum"].([]any); ok {
			for _, raw := range rawEnum {
				item, _ := raw.(string)
				switch strings.ToLower(item) {
				case "app", "canvas", "desktop", "tauri", "native":
					t.Fatalf("%s publishes retired enum value %q", location, item)
				}
			}
		}
		for key, child := range typed {
			assertNoRetiredGeneratedEnum(t, location+"/"+key, child)
		}
	case []any:
		for index, child := range typed {
			assertNoRetiredGeneratedEnum(t, fmt.Sprintf("%s/%d", location, index), child)
		}
	}
}

func assertSchemaProperty(t *testing.T, schemas map[string]map[string]any, schemaName string, propertyName string) map[string]any {
	t.Helper()
	schema := schemas[schemaName]
	if schema == nil {
		t.Fatalf("missing schema %s", schemaName)
	}
	return schemaProperty(t, schema, propertyName)
}

func schemaProperty(t *testing.T, schema map[string]any, propertyName string) map[string]any {
	t.Helper()
	properties, _ := schema["properties"].(map[string]any)
	property, _ := properties[propertyName].(map[string]any)
	if property == nil {
		t.Fatalf("missing schema property %s", propertyName)
	}
	return property
}

func assertSchemaPropertyStringEnum(t *testing.T, schemas map[string]map[string]any, schemaName string, propertyName string, want []string) {
	t.Helper()
	property := assertSchemaProperty(t, schemas, schemaName, propertyName)
	assertStringEnum(t, schemaName+"."+propertyName, property, want)
}

func assertQueryParameter(t *testing.T, operation map[string]any, name string) map[string]any {
	t.Helper()
	if operation == nil {
		t.Fatalf("missing operation containing query parameter %s", name)
	}
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]any)
		if parameter["in"] == "query" && parameter["name"] == name {
			return parameter
		}
	}
	t.Fatalf("missing query parameter %s", name)
	return nil
}

func assertQueryStringEnum(t *testing.T, operation map[string]any, name string, want []string) {
	t.Helper()
	parameter := assertQueryParameter(t, operation, name)
	schema, _ := parameter["schema"].(map[string]any)
	assertStringEnum(t, "query."+name, schema, want)
}

func assertStringEnum(t *testing.T, location string, schema map[string]any, want []string) {
	t.Helper()
	raw, _ := schema["enum"].([]any)
	got := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("%s enum contains non-string value %#v", location, item)
		}
		got = append(got, value)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s enum=%v want=%v", location, got, want)
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
