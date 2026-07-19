package admincontract

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
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

func TestWorkflowOperationsUseFieldCompleteContracts(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}

	type operationExpectation struct {
		method            string
		path              string
		responseStatus    string
		responseSchema    string
		requestSchema     string
		requestRequired   bool
		queryParameters   []string
		requiredQueries   []string
		hasPositiveIDPath bool
	}
	operations := []operationExpectation{
		{method: "get", path: "/api/admin/v1/users/page-init", responseStatus: "200", responseSchema: "UserPageInitSuccessEnvelope"},
		{method: "get", path: "/api/admin/v1/users", responseStatus: "200", responseSchema: "UserListSuccessEnvelope", queryParameters: []string{"address_id", "current_page", "date", "date_end", "date_start", "detail_address", "email", "keyword", "page_size", "role_id", "sex", "username"}, requiredQueries: []string{"current_page", "page_size"}},
		{method: "patch", path: "/api/admin/v1/users", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserBatchProfileRequest", requestRequired: true},
		{method: "delete", path: "/api/admin/v1/users", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserBatchDeleteRequest", requestRequired: true},
		{method: "put", path: "/api/admin/v1/users/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserUpdateRequest", requestRequired: true, hasPositiveIDPath: true},
		{method: "delete", path: "/api/admin/v1/users/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", hasPositiveIDPath: true},
		{method: "patch", path: "/api/admin/v1/users/{id}/status", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserStatusRequest", requestRequired: true, hasPositiveIDPath: true},
		{method: "post", path: "/api/admin/v1/users/export", responseStatus: "200", responseSchema: "UserExportSuccessEnvelope", requestSchema: "UserExportRequest", requestRequired: true},

		{method: "get", path: "/api/admin/v1/notifications/page-init", responseStatus: "200", responseSchema: "NotificationPageInitSuccessEnvelope"},
		{method: "get", path: "/api/admin/v1/notifications", responseStatus: "200", responseSchema: "NotificationListSuccessEnvelope", queryParameters: []string{"before_id", "current_page", "is_read", "keyword", "level", "page_size", "type"}, requiredQueries: []string{"current_page", "page_size"}},
		{method: "delete", path: "/api/admin/v1/notifications", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "NotificationDeleteBatchRequest", requestRequired: true},
		{method: "get", path: "/api/admin/v1/notifications/unread-count", responseStatus: "200", responseSchema: "NotificationUnreadCountSuccessEnvelope"},
		{method: "patch", path: "/api/admin/v1/notifications/{id}/read", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", hasPositiveIDPath: true},
		{method: "patch", path: "/api/admin/v1/notifications/read", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "NotificationReadRequest"},
		{method: "delete", path: "/api/admin/v1/notifications/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", hasPositiveIDPath: true},

		{method: "get", path: "/api/admin/v1/export-tasks", responseStatus: "200", responseSchema: "ExportTaskListSuccessEnvelope", queryParameters: []string{"before_id", "current_page", "file_name", "kind", "page_size", "status", "title"}},
		{method: "delete", path: "/api/admin/v1/export-tasks", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "ExportTaskDeleteBatchRequest", requestRequired: true},
		{method: "get", path: "/api/admin/v1/export-tasks/status-count", responseStatus: "200", responseSchema: "ExportTaskStatusCountSuccessEnvelope", queryParameters: []string{"file_name", "kind", "title"}},
		{method: "delete", path: "/api/admin/v1/export-tasks/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", hasPositiveIDPath: true},

		{method: "get", path: "/api/admin/v1/ai-conversations", responseStatus: "200", responseSchema: "AIConversationListSuccessEnvelope", queryParameters: []string{"agent_id", "before_id", "before_time", "limit"}},
		{method: "post", path: "/api/admin/v1/ai-conversations", responseStatus: "200", responseSchema: "AIConversationCreateSuccessEnvelope", requestSchema: "AIConversationCreateRequest", requestRequired: true},
		{method: "get", path: "/api/admin/v1/ai-conversations/{id}", responseStatus: "200", responseSchema: "AIConversationDetailSuccessEnvelope", hasPositiveIDPath: true},
		{method: "put", path: "/api/admin/v1/ai-conversations/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "AIConversationUpdateRequest", requestRequired: true, hasPositiveIDPath: true},
		{method: "delete", path: "/api/admin/v1/ai-conversations/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", hasPositiveIDPath: true},
		{method: "get", path: "/api/admin/v1/ai-conversations/{id}/messages", responseStatus: "200", responseSchema: "AIMessageListSuccessEnvelope", queryParameters: []string{"before_id", "limit"}, hasPositiveIDPath: true},
		{method: "post", path: "/api/admin/v1/ai-conversations/{id}/messages", responseStatus: "202", responseSchema: "AIMessageSendSuccessEnvelope", requestSchema: "AIMessageSendRequest", requestRequired: true, hasPositiveIDPath: true},
		{method: "post", path: "/api/admin/v1/ai-conversations/{id}/messages/cancel", responseStatus: "200", responseSchema: "AIMessageCancelSuccessEnvelope", requestSchema: "AIMessageCancelRequest", requestRequired: true, hasPositiveIDPath: true},

		{method: "get", path: "/api/admin/v1/ai-runs/page-init", responseStatus: "200", responseSchema: "AIRunPageInitSuccessEnvelope"},
		{method: "get", path: "/api/admin/v1/ai-runs", responseStatus: "200", responseSchema: "AIRunListSuccessEnvelope", queryParameters: []string{"agent_id", "current_page", "date_end", "date_start", "page_size", "platform", "provider_id", "request_id", "status", "user_id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/{id}", responseStatus: "200", responseSchema: "AIRunDetailSuccessEnvelope", hasPositiveIDPath: true},
		{method: "get", path: "/api/admin/v1/ai-runs/stats", responseStatus: "200", responseSchema: "AIRunStatsSuccessEnvelope", queryParameters: []string{"agent_id", "date_end", "date_start", "platform", "provider_id", "user_id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/stats/by-date", responseStatus: "200", responseSchema: "AIRunStatsByDateSuccessEnvelope", queryParameters: []string{"agent_id", "current_page", "date_end", "date_start", "page_size", "platform", "provider_id", "user_id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/stats/by-agent", responseStatus: "200", responseSchema: "AIRunStatsByAgentSuccessEnvelope", queryParameters: []string{"agent_id", "current_page", "date_end", "date_start", "page_size", "platform", "provider_id", "user_id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/stats/by-user", responseStatus: "200", responseSchema: "AIRunStatsByUserSuccessEnvelope", queryParameters: []string{"agent_id", "current_page", "date_end", "date_start", "page_size", "platform", "provider_id", "user_id"}},
	}

	for _, expectation := range operations {
		expectation := expectation
		t.Run(expectation.method+" "+expectation.path, func(t *testing.T) {
			operation := document.Paths[expectation.path][expectation.method]
			if operation == nil {
				t.Fatalf("missing operation")
			}
			assertOperationResponseRef(t, operation, expectation.responseStatus, expectation.responseSchema)
			assertOperationRequestBody(t, operation, expectation.requestSchema, expectation.requestRequired)
			assertOperationParameters(t, operation, expectation.queryParameters, expectation.requiredQueries, expectation.hasPositiveIDPath)
		})
	}
}

func TestWorkflowSchemasCloseBusinessFieldsAndDeclareNullability(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}

	for _, name := range []string{
		"UserPageInitSuccessEnvelope", "UserListSuccessEnvelope", "UserUpdateRequest",
		"NotificationPageInitSuccessEnvelope", "NotificationListSuccessEnvelope",
		"ExportTaskListSuccessEnvelope", "ExportTaskStatusCountSuccessEnvelope",
		"AIConversationListSuccessEnvelope", "AIMessageSendRequest", "AIMessageSendSuccessEnvelope",
		"AIRunPageInitSuccessEnvelope", "AIRunListSuccessEnvelope", "AIRunDetailSuccessEnvelope",
		"AIRunStatsSuccessEnvelope", "AIRunStatsByDateSuccessEnvelope",
	} {
		if document.Components.Schemas[name] == nil {
			t.Fatalf("missing workflow schema %s", name)
		}
	}

	assertClosedSchemaWithRequired(t, document.Components.Schemas, "UserListItem", "id", "avatar", "status", "created_at")
	assertNullableProperty(t, document.Components.Schemas["UserListItem"], "avatar")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "ExportTaskItem", "id", "file_name", "file_url", "row_count", "error_msg", "expire_at")
	for _, field := range []string{"file_name", "file_url", "row_count", "error_msg", "expire_at"} {
		assertNullableProperty(t, document.Components.Schemas["ExportTaskItem"], field)
	}
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunDetail", "id", "user_message", "assistant_message", "events", "knowledge_retrievals", "tool_calls")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "user_message")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "assistant_message")

	messageItem := document.Components.Schemas["AIMessageItem"]
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageItem", "id", "role", "content_type", "content", "created_at", "updated_at")
	if containsString(anyStrings(messageItem["required"]), "meta_json") {
		t.Fatal("AIMessageItem.meta_json must remain optional")
	}

	runtimeParams := document.Components.Schemas["AIRuntimeParams"]
	if runtimeParams["additionalProperties"] != false {
		t.Fatalf("AIRuntimeParams additionalProperties=%#v", runtimeParams["additionalProperties"])
	}
	properties := runtimeParams["properties"].(map[string]any)
	if got := sortedMapKeys(properties); !reflect.DeepEqual(got, []string{"max_history", "max_tokens", "temperature"}) {
		t.Fatalf("AIRuntimeParams properties=%v", got)
	}

	sendRequest := document.Components.Schemas["AIMessageSendRequest"]
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageSendRequest", "request_id")
	if _, expandsToUnknown := sendRequest["anyOf"]; expandsToUnknown {
		t.Fatal("AIMessageSendRequest must keep its generated TypeScript shape closed; the cross-field rule belongs to the operation extension")
	}
}

func assertOperationResponseRef(t *testing.T, operation map[string]any, status string, schema string) {
	t.Helper()
	responses, _ := operation["responses"].(map[string]any)
	response, _ := responses[status].(map[string]any)
	content, _ := response["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	actual, _ := media["schema"].(map[string]any)
	if got := actual["$ref"]; got != "#/components/schemas/"+schema {
		t.Fatalf("response schema=%#v, want %s", got, schema)
	}
	if got := actual["$ref"]; got == "#/components/schemas/SuccessEnvelope" {
		t.Fatal("workflow operation fell back to SuccessEnvelope")
	}
}

func assertOperationRequestBody(t *testing.T, operation map[string]any, schema string, required bool) {
	t.Helper()
	body, exists := operation["requestBody"]
	if schema == "" {
		if exists {
			t.Fatalf("unexpected request body: %#v", body)
		}
		return
	}
	requestBody, _ := body.(map[string]any)
	if requestBody == nil {
		t.Fatalf("missing request body %s", schema)
	}
	if got, _ := requestBody["required"].(bool); got != required {
		t.Fatalf("request body required=%v, want %v", got, required)
	}
	content := requestBody["content"].(map[string]any)
	media := content["application/json"].(map[string]any)
	actual := media["schema"].(map[string]any)
	if got := actual["$ref"]; got != "#/components/schemas/"+schema {
		t.Fatalf("request schema=%#v, want %s", got, schema)
	}
	if got := actual["$ref"]; got == "#/components/schemas/GenericObject" {
		t.Fatal("workflow operation fell back to GenericObject")
	}
}

func assertOperationParameters(t *testing.T, operation map[string]any, queryNames []string, requiredQueries []string, positiveIDPath bool) {
	t.Helper()
	parameters, _ := operation["parameters"].([]any)
	queries := make(map[string]bool)
	positiveIDFound := false
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		name, _ := parameter["name"].(string)
		location, _ := parameter["in"].(string)
		if location == "query" {
			queries[name], _ = parameter["required"].(bool)
		}
		if location == "path" && name == "id" {
			schema := parameter["schema"].(map[string]any)
			positiveIDFound = schema["type"] == "integer" && schema["minimum"] == float64(1)
		}
	}
	if queryNames == nil {
		queryNames = []string{}
	}
	if got := sortedMapKeys(queries); !reflect.DeepEqual(got, queryNames) {
		t.Fatalf("query parameters=%v, want %v", got, queryNames)
	}
	for _, name := range requiredQueries {
		if !queries[name] {
			t.Fatalf("query parameter %s must be required", name)
		}
	}
	if positiveIDFound != positiveIDPath {
		t.Fatalf("positive id path=%v, want %v", positiveIDFound, positiveIDPath)
	}
}

func assertClosedSchemaWithRequired(t *testing.T, schemas map[string]map[string]any, name string, fields ...string) {
	t.Helper()
	schema := schemas[name]
	if schema == nil {
		t.Fatalf("missing schema %s", name)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema %s is not a closed object: %#v", name, schema)
	}
	required := anyStrings(schema["required"])
	for _, field := range fields {
		if !containsString(required, field) {
			t.Fatalf("schema %s does not require %s: %v", name, field, required)
		}
	}
}

func assertNullableProperty(t *testing.T, schema map[string]any, field string) {
	t.Helper()
	properties := schema["properties"].(map[string]any)
	property := properties[field].(map[string]any)
	variants, _ := property["anyOf"].([]any)
	for _, raw := range variants {
		variant := raw.(map[string]any)
		if variant["type"] == "null" {
			return
		}
	}
	t.Fatalf("property %s is not explicitly nullable: %#v", field, property)
}

func anyStrings(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func sortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
