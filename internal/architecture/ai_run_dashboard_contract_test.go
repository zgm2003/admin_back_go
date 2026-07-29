package architecture

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"admin_back_go/internal/admincontract"
)

func TestAIRunDashboardContractMatchesRuntimeSurface(t *testing.T) {
	bundle, err := admincontract.Build(admincontract.BuildOptions{BackendCommit: strings.Repeat("a", 40)})
	if err != nil {
		t.Fatal(err)
	}
	var openAPI struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &openAPI); err != nil {
		t.Fatal(err)
	}
	getPaths := make([]string, 0)
	for path, operations := range openAPI.Paths {
		if strings.HasPrefix(path, "/api/admin/v1/ai-runs") && operations["get"] != nil {
			getPaths = append(getPaths, path)
		}
	}
	sort.Strings(getPaths)
	wantPaths := []string{
		"/api/admin/v1/ai-runs",
		"/api/admin/v1/ai-runs/dashboard",
		"/api/admin/v1/ai-runs/page-init",
		"/api/admin/v1/ai-runs/{id}",
	}
	if !reflect.DeepEqual(getPaths, wantPaths) {
		t.Fatalf("AI run GET contract paths=%v want=%v", getPaths, wantPaths)
	}

	var permissions struct {
		Operations []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Access struct {
				Kind           string `json:"kind"`
				PermissionCode string `json:"permission_code"`
			} `json:"access"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(bundle.Artifacts["permissions.json"], &permissions); err != nil {
		t.Fatal(err)
	}
	for _, operation := range permissions.Operations {
		if operation.Method != "GET" || operation.Path != "/api/admin/v1/ai-runs/dashboard" {
			continue
		}
		if operation.Access.Kind != "permission" || operation.Access.PermissionCode != "ai_run_list" {
			t.Fatalf("dashboard access=%+v", operation.Access)
		}
		return
	}
	t.Fatal("dashboard permission operation is missing")
}
