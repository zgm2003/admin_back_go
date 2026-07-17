package admincontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAPIContainsEveryRuntimeAdminOperation(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi=%q", document.OpenAPI)
	}
	for _, path := range []string{"/health", "/ready", "/api/admin/v1/users/me", "/api/payment/callbacks/alipay"} {
		if _, exists := document.Paths[path]; !exists {
			t.Fatalf("missing required path %s", path)
		}
	}

	operationIDs := make(map[string]struct{})
	operationCount := 0
	for path, pathItem := range document.Paths {
		if strings.HasPrefix(path, "/api/app/") || strings.HasPrefix(path, "/api/canvas/") {
			t.Fatalf("retired path present: %s", path)
		}
		for method, raw := range pathItem {
			var operation map[string]any
			if err := json.Unmarshal(raw, &operation); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
			operationID, _ := operation["operationId"].(string)
			if operationID == "" {
				t.Fatalf("missing operationId for %s %s", method, path)
			}
			if _, duplicate := operationIDs[operationID]; duplicate {
				t.Fatalf("duplicate operationId %q", operationID)
			}
			operationIDs[operationID] = struct{}{}
			if operation["x-admin-access"] == nil || operation["x-admin-audit"] == nil {
				t.Fatalf("missing policy extensions for %s %s", method, path)
			}
			operationCount++
		}
	}
	definitions := runtimeContractDefinitions(t)
	for _, definition := range definitions {
		if _, exists := operationIDs[definition.OperationID]; !exists {
			t.Fatalf("runtime operation %q is missing from OpenAPI", definition.OperationID)
		}
	}
	if want := len(definitions); operationCount != want {
		t.Fatalf("openapi operations=%d runtime definitions=%d", operationCount, want)
	}
}

func TestOpenAPIUsesStableSafeErrorEnvelope(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	raw := document.Components.Schemas["ErrorEnvelope"]
	for _, field := range []string{`"code"`, `"data"`, `"msg"`, `"error"`, `"category"`, `"retryable"`, `"request_id"`, `"trace_id"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("ErrorEnvelope missing %s: %s", field, raw)
		}
	}
	for _, forbidden := range []string{"cause", "operation", "template_data"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("ErrorEnvelope leaks internal field %q", forbidden)
		}
	}
}
