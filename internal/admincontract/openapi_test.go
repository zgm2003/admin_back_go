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

func TestOpenAPIRemovesRedundantSingleSessionField(t *testing.T) {
	bundle := mustBuildBundle(t)
	if strings.Contains(string(bundle.Artifacts["openapi.json"]), `"single_session"`) {
		t.Fatal("Admin OpenAPI still publishes redundant single_session field")
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
		method          string
		path            string
		operationID     string
		responseStatus  string
		responseSchema  string
		requestSchema   string
		requestRequired bool
		queryParameters []string
		requiredQueries []string
		positivePathIDs []string
	}
	operations := []operationExpectation{
		{method: "get", path: "/api/admin/v1/users/page-init", responseStatus: "200", responseSchema: "UserPageInitSuccessEnvelope"},
		{method: "get", path: "/api/admin/v1/users", responseStatus: "200", responseSchema: "UserListSuccessEnvelope", queryParameters: []string{"address_id", "current_page", "date", "date_end", "date_start", "detail_address", "email", "keyword", "page_size", "role_id", "sex", "username"}, requiredQueries: []string{"current_page", "page_size"}},
		{method: "patch", path: "/api/admin/v1/users", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserBatchProfileRequest", requestRequired: true},
		{method: "delete", path: "/api/admin/v1/users", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserBatchDeleteRequest", requestRequired: true},
		{method: "put", path: "/api/admin/v1/users/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserUpdateRequest", requestRequired: true, positivePathIDs: []string{"id"}},
		{method: "delete", path: "/api/admin/v1/users/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", positivePathIDs: []string{"id"}},
		{method: "patch", path: "/api/admin/v1/users/{id}/status", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "UserStatusRequest", requestRequired: true, positivePathIDs: []string{"id"}},
		{method: "post", path: "/api/admin/v1/users/export", responseStatus: "200", responseSchema: "UserExportSuccessEnvelope", requestSchema: "UserExportRequest", requestRequired: true},

		{method: "get", path: "/api/admin/v1/notifications/page-init", responseStatus: "200", responseSchema: "NotificationPageInitSuccessEnvelope"},
		{method: "get", path: "/api/admin/v1/notifications", responseStatus: "200", responseSchema: "NotificationListSuccessEnvelope", queryParameters: []string{"before_id", "current_page", "is_read", "keyword", "level", "page_size", "type"}, requiredQueries: []string{"current_page", "page_size"}},
		{method: "delete", path: "/api/admin/v1/notifications", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "NotificationDeleteBatchRequest", requestRequired: true},
		{method: "get", path: "/api/admin/v1/notifications/unread-count", responseStatus: "200", responseSchema: "NotificationUnreadCountSuccessEnvelope"},
		{method: "patch", path: "/api/admin/v1/notifications/{id}/read", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", positivePathIDs: []string{"id"}},
		{method: "patch", path: "/api/admin/v1/notifications/read", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "NotificationReadRequest"},
		{method: "delete", path: "/api/admin/v1/notifications/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", positivePathIDs: []string{"id"}},

		{method: "get", path: "/api/admin/v1/export-tasks", responseStatus: "200", responseSchema: "ExportTaskListSuccessEnvelope", queryParameters: []string{"before_id", "current_page", "file_name", "kind", "page_size", "status", "title"}},
		{method: "delete", path: "/api/admin/v1/export-tasks", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "ExportTaskDeleteBatchRequest", requestRequired: true},
		{method: "get", path: "/api/admin/v1/export-tasks/status-count", responseStatus: "200", responseSchema: "ExportTaskStatusCountSuccessEnvelope", queryParameters: []string{"file_name", "kind", "title"}},
		{method: "delete", path: "/api/admin/v1/export-tasks/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", positivePathIDs: []string{"id"}},

		{method: "get", path: "/api/admin/v1/ai-conversations", responseStatus: "200", responseSchema: "AIConversationListSuccessEnvelope", queryParameters: []string{"agent_id", "before_id", "before_time", "limit"}},
		{method: "post", path: "/api/admin/v1/ai-conversations", responseStatus: "200", responseSchema: "AIConversationCreateSuccessEnvelope", requestSchema: "AIConversationCreateRequest", requestRequired: true},
		{method: "get", path: "/api/admin/v1/ai-conversations/{id}", responseStatus: "200", responseSchema: "AIConversationDetailSuccessEnvelope", positivePathIDs: []string{"id"}},
		{method: "put", path: "/api/admin/v1/ai-conversations/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", requestSchema: "AIConversationUpdateRequest", requestRequired: true, positivePathIDs: []string{"id"}},
		{method: "delete", path: "/api/admin/v1/ai-conversations/{id}", responseStatus: "200", responseSchema: "EmptySuccessEnvelope", positivePathIDs: []string{"id"}},
		{method: "get", path: "/api/admin/v1/ai-conversations/{id}/messages", responseStatus: "200", responseSchema: "AIMessageListSuccessEnvelope", queryParameters: []string{"before_id", "limit"}, positivePathIDs: []string{"id"}},
		{method: "post", path: "/api/admin/v1/ai-conversations/{id}/messages", responseStatus: "202", responseSchema: "AIMessageSendSuccessEnvelope", requestSchema: "AIMessageSendRequest", requestRequired: true, positivePathIDs: []string{"id"}},
		{method: "post", path: "/api/admin/v1/ai-conversations/{id}/messages/cancel", responseStatus: "200", responseSchema: "AIMessageCancelSuccessEnvelope", requestSchema: "AIMessageCancelRequest", requestRequired: true, positivePathIDs: []string{"id"}},
		{method: "post", path: "/api/admin/v1/ai-conversations/{id}/messages/{message_id}/revisions", operationID: "post_api_admin_v1_ai_conversations_id_messages_message_id_revisions", responseStatus: "202", responseSchema: "AIMessageSendSuccessEnvelope", requestSchema: "AIMessageRevisionRequest", requestRequired: true, positivePathIDs: []string{"id", "message_id"}},
		{method: "post", path: "/api/admin/v1/ai-conversations/{id}/messages/{message_id}/regenerations", operationID: "post_api_admin_v1_ai_conversations_id_messages_message_id_regenerations", responseStatus: "202", responseSchema: "AIMessageSendSuccessEnvelope", requestSchema: "AIMessageRegenerationRequest", requestRequired: true, positivePathIDs: []string{"id", "message_id"}},
		{method: "delete", path: "/api/admin/v1/ai-conversations/{id}/messages", operationID: "delete_api_admin_v1_ai_conversations_id_messages", responseStatus: "200", responseSchema: "AIMessageDeleteSuccessEnvelope", requestSchema: "AIMessageDeleteRequest", requestRequired: true, positivePathIDs: []string{"id"}},
		{method: "put", path: "/api/admin/v1/ai-conversations/{id}/read-cursor", operationID: "put_api_admin_v1_ai_conversations_id_read_cursor", responseStatus: "200", responseSchema: "AIConversationReadCursorSuccessEnvelope", requestSchema: "AIConversationReadCursorRequest", requestRequired: true, positivePathIDs: []string{"id"}},

		{method: "get", path: "/api/admin/v1/ai-runs/page-init", responseStatus: "200", responseSchema: "AIRunPageInitSuccessEnvelope", queryParameters: []string{"date_end", "date_start"}},
		{method: "get", path: "/api/admin/v1/ai-runs", responseStatus: "200", responseSchema: "AIRunListSuccessEnvelope", queryParameters: []string{"agent_id", "anomaly_as_of", "billing_anomaly", "billing_reason", "billing_status", "current_page", "date_end", "date_start", "error_code", "model_id", "page_size", "platform", "provider_id", "request_id", "run_anomaly", "status", "tool_code", "user_feedback", "user_id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/dashboard", responseStatus: "200", responseSchema: "AIRunDashboardSuccessEnvelope", queryParameters: []string{"agent_id", "date_end", "date_start", "model_id", "platform", "provider_id", "user_id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/{id}", responseStatus: "200", responseSchema: "AIRunDetailSuccessEnvelope", positivePathIDs: []string{"id"}},
		{method: "get", path: "/api/admin/v1/ai-runs/{id}/input-attachments/{ordinal}/preview", operationID: "get_api_admin_v1_ai_runs_id_input_attachments_ordinal_preview", responseStatus: "200", responseSchema: "AIRunInputAttachmentPreviewSuccessEnvelope", positivePathIDs: []string{"id", "ordinal"}},
		{method: "put", path: "/api/admin/v1/ai-runs/{id}/user-feedback", operationID: "put_api_admin_v1_ai_runs_id_user_feedback", responseStatus: "200", responseSchema: "AIRunUserFeedbackSuccessEnvelope", requestSchema: "AIRunUserFeedbackRequest", requestRequired: true, positivePathIDs: []string{"id"}},
	}

	for _, expectation := range operations {
		expectation := expectation
		t.Run(expectation.method+" "+expectation.path, func(t *testing.T) {
			operation := document.Paths[expectation.path][expectation.method]
			if operation == nil {
				t.Fatalf("missing operation")
			}
			if expectation.operationID != "" && operation["operationId"] != expectation.operationID {
				t.Fatalf("operationId=%#v, want %s", operation["operationId"], expectation.operationID)
			}
			assertOperationResponseRef(t, operation, expectation.responseStatus, expectation.responseSchema)
			assertOperationRequestBody(t, operation, expectation.requestSchema, expectation.requestRequired)
			assertOperationParameters(t, operation, expectation.queryParameters, expectation.requiredQueries, expectation.positivePathIDs)
		})
	}
	assertQueryStringEnum(t, document.Paths["/api/admin/v1/ai-runs"]["get"], "status", []string{"running", "success", "failed", "canceled", "timeout", "outcome_unknown"})
	assertQueryStringEnum(t, document.Paths["/api/admin/v1/ai-runs"]["get"], "user_feedback", []string{"liked", "unliked"})
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
		"AIMessageRevisionRequest", "AIMessageRegenerationRequest", "AIMessageDeleteSuccessEnvelope",
		"AIConversationReadCursorSuccessEnvelope", "AIRunInputAttachmentPreviewSuccessEnvelope", "AIRunUserFeedbackSuccessEnvelope",
		"AIRunPageInitSuccessEnvelope", "AIRunListSuccessEnvelope", "AIRunDetailSuccessEnvelope",
		"AIRunDashboardSuccessEnvelope",
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
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIConversationItem", "unread_count")
	conversationProperties := document.Components.Schemas["AIConversationItem"]["properties"].(map[string]any)
	if unread := conversationProperties["unread_count"].(map[string]any); unread["type"] != "integer" || unread["minimum"] != float64(0) {
		t.Fatalf("AIConversationItem.unread_count=%#v", unread)
	}
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageItem", "paired_message_id", "run_id", "liked", "delivery_state", "settlement_pending")
	assertNullableProperty(t, document.Components.Schemas["AIMessageItem"], "paired_message_id")
	assertNullableProperty(t, document.Components.Schemas["AIMessageItem"], "run_id")
	assertNullableProperty(t, document.Components.Schemas["AIMessageItem"], "delivery_state")
	if liked := document.Components.Schemas["AIMessageItem"]["properties"].(map[string]any)["liked"].(map[string]any); liked["type"] != "boolean" {
		t.Fatalf("AIMessageItem.liked=%#v", liked)
	}
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageDeleteResult", "deleted_ids")
	deletedIDs := document.Components.Schemas["AIMessageDeleteResult"]["properties"].(map[string]any)["deleted_ids"].(map[string]any)
	if deletedIDs["uniqueItems"] != true || deletedIDs["minItems"] != float64(1) || !strings.Contains(deletedIDs["description"].(string), "ascending") {
		t.Fatalf("AIMessageDeleteResult.deleted_ids=%#v", deletedIDs)
	}
	for schemaName, fields := range map[string][]string{
		"AIMessageRegenerationRequest":    {"request_id"},
		"AIMessageDeleteRequest":          {"ids"},
		"AIMessageDeleteResult":           {"deleted_ids"},
		"AIConversationReadCursorRequest": {"message_id"},
		"AIConversationReadCursorResult":  {"conversation_id", "last_read_message_id", "unread_count"},
		"AIRunInputAttachmentPreview":     {"expires_in", "url"},
		"AIRunUserFeedbackRequest":        {"liked"},
		"AIRunUserFeedbackResult":         {"id", "liked", "liked_at"},
	} {
		assertClosedSchemaWithRequired(t, document.Components.Schemas, schemaName, fields...)
		properties := document.Components.Schemas[schemaName]["properties"].(map[string]any)
		if got := sortedMapKeys(properties); !reflect.DeepEqual(got, fields) {
			t.Fatalf("%s properties=%v want=%v", schemaName, got, fields)
		}
		if got := anyStrings(document.Components.Schemas[schemaName]["required"]); !reflect.DeepEqual(got, fields) {
			t.Fatalf("%s required=%v want=%v", schemaName, got, fields)
		}
	}
	revision := document.Components.Schemas["AIMessageRevisionRequest"]
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageRevisionRequest", "content", "request_id")
	if got := sortedMapKeys(revision["properties"].(map[string]any)); !reflect.DeepEqual(got, []string{"attachments", "content", "request_id"}) {
		t.Fatalf("AIMessageRevisionRequest properties=%v", got)
	}
	if containsString(anyStrings(revision["required"]), "attachments") {
		t.Fatal("AIMessageRevisionRequest.attachments must remain optional so omission preserves existing attachments")
	}
	assertNullableProperty(t, document.Components.Schemas["AIRunUserFeedbackResult"], "liked_at")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunDetail", "id", "user_message", "assistant_message", "events", "context_plan", "tool_calls", "diagnostic_codes")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunDetail", "liked", "liked_at")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "liked_at")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "user_message")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "assistant_message")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "context_plan")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunDetail", "billing_status", "billing_reason", "held_amount", "actual_amount", "pricing", "usage_items", "provider_attempts", "latency", "request_summary")
	assertNullableProperty(t, document.Components.Schemas["AIRunDetail"], "pricing")
	diagnosticCodes := document.Components.Schemas["AIRunDetail"]["properties"].(map[string]any)["diagnostic_codes"].(map[string]any)
	if diagnosticCodes["type"] != "array" || diagnosticCodes["items"].(map[string]any)["type"] != "string" {
		t.Fatalf("AIRunDetail.diagnostic_codes=%#v", diagnosticCodes)
	}
	for _, name := range []string{"AIRunPricing", "AIRunPricingRate", "AIRunUsageItem", "AIRunProviderAttempt", "AIRunLatencyBreakdown", "AIRunRequestSummary"} {
		assertClosedSchemaWithRequired(t, document.Components.Schemas, name)
	}
	for name, fields := range map[string][]string{
		"AIRunPricing":          {"billing_multiplier", "catalog_vendor", "max_output_tokens", "model_id", "rates", "resolved_alias", "transport_engine", "version"},
		"AIRunPricingRate":      {"category", "price", "tier_key", "unit", "unit_scale"},
		"AIRunUsageItem":        {"amount", "attempt_no", "billable", "category", "quantity", "tier_key", "unit", "unit_price", "unit_scale"},
		"AIRunProviderAttempt":  {"attempt_no", "provider_request_id", "state", "usage_status"},
		"AIRunLatencyBreakdown": {"accept_ms", "claim_source", "cos_head_ms", "cos_stream_ms", "end_to_end_ms", "prepare_ms", "provider_total_ms", "queue_ms", "settlement_ms", "ttft_ms"},
		"AIRunRequestSummary":   {"api_protocol", "attachment_count", "materialized_request_bytes", "message_count", "native_file_bytes", "native_file_count", "prepared_manifest_bytes", "prepared_request_bytes", "provider_attempt_count", "tool_call_count"},
	} {
		properties := document.Components.Schemas[name]["properties"].(map[string]any)
		if got := sortedMapKeys(properties); !reflect.DeepEqual(got, fields) {
			t.Fatalf("%s properties=%v want=%v", name, got, fields)
		}
		if got := anyStrings(document.Components.Schemas[name]["required"]); !reflect.DeepEqual(got, fields) {
			t.Fatalf("%s required=%v want=%v", name, got, fields)
		}
	}
	for _, field := range []string{"accept_ms", "queue_ms", "prepare_ms", "cos_head_ms", "cos_stream_ms", "ttft_ms", "provider_total_ms", "settlement_ms", "end_to_end_ms"} {
		assertNullableProperty(t, document.Components.Schemas["AIRunLatencyBreakdown"], field)
	}
	assertNullableProperty(t, document.Components.Schemas["AIRunRequestSummary"], "message_count")
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunLatencyBreakdown", "claim_source", []string{"", "wake", "poll", "recovery"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunRequestSummary", "api_protocol", []string{"", "chat_completions", "responses"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunDetail", "billing_status", []string{"pending", "held", "settled", "released", "unbilled"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunDetail", "billing_reason", []string{"pending", "held", "settled_complete_usage", "released_before_dispatch", "released_insufficient_balance", "released_provider_failed", "released_outcome_unknown", "unbilled_usage_incomplete", "unbilled_over_hold", "legacy_unpriced"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunProviderAttempt", "state", []string{"prepared", "dispatched", "succeeded", "failed", "canceled", "outcome_unknown"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunProviderAttempt", "usage_status", []string{"complete", "unavailable"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunUsageItem", "category", []string{"input", "output", "cache_read", "cache_write", "media"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunListItem", "status", []string{"running", "success", "failed", "canceled", "timeout", "outcome_unknown"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIRunEvent", "event_type", []string{"start", "completed", "failed", "canceled", "timeout", "retry_scheduled", "usage_recorded", "outcome_unknown", "settled", "released", "unbilled"})
	for _, field := range []string{"held_amount", "actual_amount"} {
		property := document.Components.Schemas["AIRunDetail"]["properties"].(map[string]any)[field].(map[string]any)
		if property["type"] != "string" || property["pattern"] != `^(0|[1-9][0-9]*)(\.[0-9]{0,7}[1-9])?$` {
			t.Fatalf("AIRunDetail.%s is not canonical RMB: %#v", field, property)
		}
	}
	for _, check := range []struct{ schemaName, field string }{
		{schemaName: "AIRunPricingRate", field: "price"},
		{schemaName: "AIRunUsageItem", field: "unit_price"},
		{schemaName: "AIRunUsageItem", field: "amount"},
	} {
		property := document.Components.Schemas[check.schemaName]["properties"].(map[string]any)[check.field].(map[string]any)
		if property["pattern"] != `^(0|[1-9][0-9]*)(\.[0-9]{0,7}[1-9])?$` {
			t.Fatalf("%s.%s is not canonical RMB: %#v", check.schemaName, check.field, property)
		}
	}
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageCancelRequest", "request_id", "delivered_seq")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageCancelResult", "conversation_id", "request_id", "status", "assistant_message_id", "settlement_pending")
	assertNullableProperty(t, document.Components.Schemas["AIMessageCancelResult"], "assistant_message_id")
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIMessageCancelResult", "status", []string{"stopped", "already_terminal"})
	deliveryState := document.Components.Schemas["AIMessageItem"]["properties"].(map[string]any)["delivery_state"].(map[string]any)
	deliveryVariants := deliveryState["anyOf"].([]any)
	if len(deliveryVariants) != 2 || !equalJSONValues(deliveryVariants[0].(map[string]any)["enum"], []any{"completed", "stopped"}) {
		t.Fatalf("AIMessageItem.delivery_state=%#v", deliveryState)
	}
	cancelRequestProperties := document.Components.Schemas["AIMessageCancelRequest"]["properties"].(map[string]any)
	if deliveredSeq := cancelRequestProperties["delivered_seq"].(map[string]any); deliveredSeq["type"] != "integer" || deliveredSeq["minimum"] != float64(0) {
		t.Fatalf("cancel delivered_seq=%#v", deliveredSeq)
	}
	for schemaName := range map[string]struct{}{"AIMessageCancelResult": {}, "AIMessageItem": {}} {
		properties := document.Components.Schemas[schemaName]["properties"].(map[string]any)
		if pending := properties["settlement_pending"].(map[string]any); pending["type"] != "boolean" {
			t.Fatalf("%s.settlement_pending=%#v", schemaName, pending)
		}
	}

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
	if got := sortedMapKeys(properties); !reflect.DeepEqual(got, []string{"temperature"}) {
		t.Fatalf("AIRuntimeParams properties=%v", got)
	}
	attachment := document.Components.Schemas["AIAttachmentRequest"]
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIAttachmentRequest", "type", "object_key", "mime_type", "url", "name", "size")
	if got := sortedMapKeys(attachment["properties"].(map[string]any)); !reflect.DeepEqual(got, []string{"mime_type", "name", "object_key", "size", "type", "url"}) {
		t.Fatalf("AIAttachmentRequest properties=%v", got)
	}
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIAttachmentRequest", "type", []string{"image", "file"})
	metaAttachment := document.Components.Schemas["AIMessageMetaAttachment"]
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIMessageMetaAttachment", "type", "url", "name", "size")
	if got := anyStrings(metaAttachment["required"]); !reflect.DeepEqual(got, []string{"type", "url", "name", "size"}) {
		t.Fatalf("AIMessageMetaAttachment required=%v", got)
	}
	for _, optional := range []string{"object_key", "mime_type"} {
		if containsString(anyStrings(metaAttachment["required"]), optional) {
			t.Fatalf("AIMessageMetaAttachment.%s must remain optional for historical messages", optional)
		}
	}
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIMessageMetaAttachment", "type", []string{"image", "file"})

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

func assertOperationParameters(t *testing.T, operation map[string]any, queryNames []string, requiredQueries []string, positivePathIDs []string) {
	t.Helper()
	parameters, _ := operation["parameters"].([]any)
	queries := make(map[string]bool)
	positivePaths := make(map[string]bool)
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		name, _ := parameter["name"].(string)
		location, _ := parameter["in"].(string)
		if location == "query" {
			queries[name], _ = parameter["required"].(bool)
		}
		if location == "path" {
			schema := parameter["schema"].(map[string]any)
			positivePaths[name] = schema["type"] == "integer" && schema["minimum"] == float64(1)
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
	if positivePathIDs == nil {
		positivePathIDs = []string{}
	}
	if got := sortedMapKeys(positivePaths); !reflect.DeepEqual(got, positivePathIDs) {
		t.Fatalf("positive path parameters=%v, want %v", got, positivePathIDs)
	}
	for _, name := range positivePathIDs {
		if !positivePaths[name] {
			t.Fatalf("path parameter %s must be a positive integer", name)
		}
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

func TestAIRunDashboardOpenAPIIsCompleteAndNonNullable(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}

	operation := document.Paths["/api/admin/v1/ai-runs/dashboard"]["get"]
	if operation == nil {
		t.Fatal("missing GET /api/admin/v1/ai-runs/dashboard")
	}
	assertOperationParameters(t, operation,
		[]string{"agent_id", "date_end", "date_start", "model_id", "platform", "provider_id", "user_id"}, nil, nil)
	assertOperationResponseRef(t, operation, "200", "AIRunDashboardSuccessEnvelope")

	for _, name := range []string{
		"AIRunDashboardDateRange", "AIRunDashboardSummary", "AIRunDashboardPercentile",
		"AIRunDashboardPerformance", "AIRunDashboardBilling", "AIRunDashboardAnomalyItem",
		"AIRunDashboardAnomalies", "AIRunDashboardTrendItem", "AIRunDashboardAttributionMetrics",
		"AIRunDashboardModelBreakdown", "AIRunDashboardProviderBreakdown", "AIRunDashboardAgentBreakdown",
		"AIRunDashboardUserBreakdown", "AIRunDashboardErrorBreakdown", "AIRunDashboardToolBreakdown",
		"AIRunDashboardBreakdowns", "AIRunDashboardResult", "AIRunDashboardSuccessEnvelope",
	} {
		schema := document.Components.Schemas[name]
		if schema == nil {
			t.Fatalf("missing dashboard schema %s", name)
		}
		if properties, ok := schema["properties"].(map[string]any); ok {
			if got, want := anyStrings(schema["required"]), sortedMapKeys(properties); !reflect.DeepEqual(got, want) {
				t.Fatalf("schema %s required=%v want=%v", name, got, want)
			}
			for propertyName, raw := range properties {
				assertSchemaIsNotNullable(t, name+"."+propertyName, raw)
			}
		}
	}

	assertRequiredArrayProperties(t, document.Components.Schemas["AIRunDashboardAnomalies"], "run_items", "billing_items")
	assertRequiredArrayProperties(t, document.Components.Schemas["AIRunDashboardBreakdowns"], "agents", "errors", "models", "providers", "tools", "users")
	assertRequiredArrayProperties(t, document.Components.Schemas["AIRunDashboardResult"], "trend")
	for _, check := range []struct{ schema, field string }{
		{schema: "AIRunDashboardBilling", field: "actual_amount"},
		{schema: "AIRunDashboardBilling", field: "released_amount"},
		{schema: "AIRunDashboardTrendItem", field: "actual_amount"},
		{schema: "AIRunDashboardAttributionMetrics", field: "actual_amount"},
		{schema: "AIRunDashboardModelBreakdown", field: "actual_amount"},
		{schema: "AIRunDashboardProviderBreakdown", field: "actual_amount"},
		{schema: "AIRunDashboardAgentBreakdown", field: "actual_amount"},
		{schema: "AIRunDashboardUserBreakdown", field: "actual_amount"},
	} {
		property := document.Components.Schemas[check.schema]["properties"].(map[string]any)[check.field].(map[string]any)
		if property["type"] != "string" {
			t.Fatalf("%s.%s must be a string amount: %#v", check.schema, check.field, property)
		}
	}
}

func TestLegacyAIRunStatsContractsAreAbsent(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/admin/v1/ai-runs/stats", "/api/admin/v1/ai-runs/stats/latency",
		"/api/admin/v1/ai-runs/stats/by-date", "/api/admin/v1/ai-runs/stats/by-agent",
		"/api/admin/v1/ai-runs/stats/by-user",
	} {
		if document.Paths[path] != nil {
			t.Errorf("legacy AI run stats path is still published: %s", path)
		}
	}
	for name := range document.Components.Schemas {
		if strings.HasPrefix(name, "AIRunStats") || strings.HasPrefix(name, "AIRunLatencyStats") || name == "AIRunLatencyDistribution" {
			t.Errorf("legacy AI run stats schema is still published: %s", name)
		}
	}
}

func TestAIRunPageInitAndListPublishDashboardDrilldownContract(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	pageInit := document.Paths["/api/admin/v1/ai-runs/page-init"]["get"]
	assertOperationParameters(t, pageInit, []string{"date_end", "date_start"}, nil, nil)
	for _, name := range []string{"date_start", "date_end"} {
		parameter := operationQueryParameter(t, pageInit, name)
		description, _ := parameter["description"].(string)
		if !strings.Contains(strings.ToLower(description), "inclusive") || !strings.Contains(description, "YYYY-MM-DD") {
			t.Fatalf("page-init %s description=%q", name, description)
		}
	}

	list := document.Paths["/api/admin/v1/ai-runs"]["get"]
	assertOperationParameters(t, list, []string{
		"agent_id", "anomaly_as_of", "billing_anomaly", "billing_reason", "billing_status", "current_page",
		"date_end", "date_start", "error_code", "model_id", "page_size", "platform", "provider_id",
		"request_id", "run_anomaly", "status", "tool_code", "user_feedback", "user_id",
	}, nil, nil)
	anomalyAsOf := operationQueryParameter(t, list, "anomaly_as_of")["schema"].(map[string]any)
	if anomalyAsOf["format"] != "date-time" {
		t.Fatalf("anomaly_as_of schema=%#v", anomalyAsOf)
	}

	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunPageInitModelOption", "historical", "label", "value")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunPageInitDict",
		"agentArr", "billing_reason_arr", "billing_status_arr", "model_arr", "platform_arr", "providerArr", "status_arr")
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "AIRunListItem", "billing_reason", "billing_status", "error_code", "liked", "liked_at")
	assertNullableProperty(t, document.Components.Schemas["AIRunListItem"], "liked_at")
}

func operationQueryParameter(t *testing.T, operation map[string]any, name string) map[string]any {
	t.Helper()
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if parameter["in"] == "query" && parameter["name"] == name {
			return parameter
		}
	}
	t.Fatalf("missing query parameter %s", name)
	return nil
}

func assertRequiredArrayProperties(t *testing.T, schema map[string]any, names ...string) {
	t.Helper()
	required := anyStrings(schema["required"])
	properties := schema["properties"].(map[string]any)
	for _, name := range names {
		if !containsString(required, name) {
			t.Fatalf("property %s must be required", name)
		}
		property := properties[name].(map[string]any)
		if property["type"] != "array" {
			t.Fatalf("property %s must be a non-null array: %#v", name, property)
		}
	}
}

func assertSchemaIsNotNullable(t *testing.T, name string, raw any) {
	t.Helper()
	schema, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("schema %s=%#v", name, raw)
	}
	if schema["nullable"] == true {
		t.Fatalf("schema %s is nullable", name)
	}
	if variants, ok := schema["anyOf"].([]any); ok {
		for _, variantRaw := range variants {
			variant, _ := variantRaw.(map[string]any)
			if variant["type"] == "null" {
				t.Fatalf("schema %s is nullable", name)
			}
		}
	}
}

func TestAIContextOpenAPIPublishesDegradedOutcome(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIMessageContext", "outcome", []string{"skipped", "no_hit", "hit", "degraded", "failed"})
	assertSchemaPropertyStringEnum(t, document.Components.Schemas, "AIContextPlan", "retrieval_outcome", []string{"skipped", "no_hit", "hit", "degraded", "failed"})
}

func TestAIContextPageInitPublishesTypedModelOptions(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	operation := document.Paths["/api/admin/v1/ai/context/page-init"]["get"]
	if operation == nil || operation["operationId"] != "ai_context_page_init" {
		t.Fatalf("context page-init operation = %#v", operation)
	}
	assertOperationResponseRef(t, operation, "200", "ai_context_page_init_ResponseEnvelope")
	assertClosedSchemaWithRequired(t, document.Components.Schemas,
		"Go_internal_module_ai_contextengine_ContextPageInitResponse_Output",
		"embedding_model_options", "memory_model_options", "reranker_model_options")
	assertClosedSchemaWithRequired(t, document.Components.Schemas,
		"Go_internal_module_ai_contextengine_ProviderModelOptionDTO_Output",
		"label", "model_id", "provider_name", "value")
}

func TestAIProviderModelCatalogPublishesAlternativeInputsWithoutParentRequirement(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"post_api_admin_v1_ai_providers_Request",
		"put_api_admin_v1_ai_providers_id_Request",
		"put_api_admin_v1_ai_providers_id_models_Request",
	} {
		required := anyStrings(document.Components.Schemas[name]["required"])
		if containsString(required, "model_ids") || containsString(required, "models") {
			t.Fatalf("%s requires one branch before cross-field validation: %v", name, required)
		}
	}
	assertClosedSchemaWithRequired(t, document.Components.Schemas,
		"Go_internal_module_ai_provider_ProviderModelInput_Input", "model_id", "model_kind")
}
