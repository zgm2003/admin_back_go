package admincontract

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/server/adminroute"
)

func TestOpenAPIDocumentsScopedBrowserGrantCredentials(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			SecuritySchemes map[string]map[string]any `json:"securitySchemes"`
			Schemas         map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}

	for name, want := range map[string]map[string]any{
		"queueMonitorGrantCookie": {"type": "apiKey", "in": "cookie", "name": "__Secure-admin_queue_monitor"},
		"realtimeTicket":          {"type": "apiKey", "in": "query", "name": "ticket"},
	} {
		if got := document.Components.SecuritySchemes[name]; !reflect.DeepEqual(got, want) {
			t.Fatalf("security scheme %s=%#v, want %#v", name, got, want)
		}
	}

	assertOperationSecurity := func(path, method, scheme string) {
		t.Helper()
		got := document.Paths[path][method]["security"]
		want := []any{map[string]any{scheme: []any{}}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s %s security=%#v, want %#v", method, path, got, want)
		}
	}
	assertOperationSecurity("/api/admin/v1/queue-monitor-ui", "get", "queueMonitorGrantCookie")
	assertOperationSecurity("/api/admin/v1/queue-monitor-ui", "head", "queueMonitorGrantCookie")
	assertOperationSecurity("/api/admin/v1/realtime/ws", "get", "realtimeTicket")

	assertResponseSchema := func(path, want string) {
		t.Helper()
		operation := document.Paths[path]["post"]
		responses := operation["responses"].(map[string]any)
		ok := responses["200"].(map[string]any)
		content := ok["content"].(map[string]any)
		media := content["application/json"].(map[string]any)
		schema := media["schema"].(map[string]any)
		if got := schema["$ref"]; got != "#/components/schemas/"+want {
			t.Fatalf("POST %s response schema=%#v", path, got)
		}
		if document.Components.Schemas[want] == nil {
			t.Fatalf("missing response schema %s", want)
		}
	}
	assertResponseSchema("/api/admin/v1/auth/realtime-tickets", "RealtimeTicketSuccessEnvelope")
	assertResponseSchema("/api/admin/v1/auth/queue-monitor-grants", "QueueMonitorGrantSuccessEnvelope")
}

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

func TestOpenAPIUsesDeclaredAcceptedStatus(t *testing.T) {
	document, err := buildOpenAPI([]adminroute.Definition{{
		Method:        http.MethodPost,
		Path:          "/api/admin/v1/ai-conversations/:id/messages",
		SuccessStatus: http.StatusAccepted,
		Access:        adminroute.Authenticated(),
		Audit:         adminroute.Audit("ai_message", "send", "发送AI消息"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	operation := paths["/api/admin/v1/ai-conversations/{id}/messages"].(map[string]any)["post"].(map[string]any)
	responses := operation["responses"].(map[string]any)
	if responses["202"] == nil || responses["200"] != nil {
		t.Fatalf("unexpected responses: %#v", responses)
	}
}
