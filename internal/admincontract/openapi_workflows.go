package admincontract

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"admin_back_go/internal/server/adminroute"
)

type workflowOperationKey struct {
	Method string
	Path   string
}

type workflowRequestBody struct {
	Schema    string
	Required  bool
	MediaType string
}

type workflowOperationContract struct {
	PathParameters  map[string]map[string]any
	QueryParameters []map[string]any
	ParameterRules  []string
	RequestBody     *workflowRequestBody
	ResponseSchema  string
}

var workflowOperationContracts = buildWorkflowOperationContracts()

func buildWorkflowOperationContracts() map[workflowOperationKey]workflowOperationContract {
	positiveID := true
	noID := false
	return map[workflowOperationKey]workflowOperationContract{
		workflowKey(http.MethodPost, "/api/admin/v1/users/export"): workflowContract("UserExportSuccessEnvelope", requiredBody("UserExportRequest"), nil, noID),

		workflowKey(http.MethodGet, "/api/admin/v1/notifications/page-init"): workflowContract("NotificationPageInitSuccessEnvelope", nil, nil, noID),
		workflowKey(http.MethodGet, "/api/admin/v1/notifications"): workflowContract("NotificationListSuccessEnvelope", nil, []map[string]any{
			queryParameter("before_id", false, positiveIntegerSchema(), "Cursor ID for records before this notification."),
			queryParameter("current_page", true, positiveIntegerSchema(), "One-based page number."),
			queryParameter("is_read", false, integerEnumSchema(1, 2), "Read-state filter: 1 read, 2 unread."),
			queryParameter("keyword", false, maxStringSchema(100), "Title/content keyword search."),
			queryParameter("level", false, integerEnumSchema(1, 2), "Notification level filter."),
			queryParameter("page_size", true, integerRangeSchema(1, 50), "Number of rows per page."),
			queryParameter("type", false, integerEnumSchema(1, 2, 3, 4), "Notification type filter."),
		}, noID),
		workflowKey(http.MethodDelete, "/api/admin/v1/notifications"):           workflowContract("EmptySuccessEnvelope", requiredBody("NotificationDeleteBatchRequest"), nil, noID),
		workflowKey(http.MethodGet, "/api/admin/v1/notifications/unread-count"): workflowContract("NotificationUnreadCountSuccessEnvelope", nil, nil, noID),
		workflowKey(http.MethodPatch, "/api/admin/v1/notifications/:id/read"):   workflowContract("EmptySuccessEnvelope", nil, nil, positiveID),
		workflowKey(http.MethodPatch, "/api/admin/v1/notifications/read"): workflowContract("EmptySuccessEnvelope", optionalBody("NotificationReadRequest"), nil, noID,
			"an absent request body or an absent/empty ids array marks all current-user notifications read"),
		workflowKey(http.MethodDelete, "/api/admin/v1/notifications/:id"): workflowContract("EmptySuccessEnvelope", nil, nil, positiveID),

		workflowKey(http.MethodGet, "/api/admin/v1/export-tasks"): workflowContract("ExportTaskListSuccessEnvelope", nil, []map[string]any{
			queryParameter("before_id", false, positiveIntegerSchema(), "Cursor ID for records before this export task."),
			queryParameter("current_page", false, schemaWith(positiveIntegerSchema(), "default", 1), "One-based page number."),
			queryParameter("file_name", false, maxStringSchema(255), "File-name search."),
			queryParameter("kind", false, maxStringSchema(64), "Export kind filter."),
			queryParameter("page_size", false, schemaWith(integerRangeSchema(1, 50), "default", 20), "Number of rows per page."),
			queryParameter("status", false, integerEnumSchema(1, 2, 3), "Export status filter."),
			queryParameter("title", false, maxStringSchema(100), "Export title search."),
		}, noID),
		workflowKey(http.MethodDelete, "/api/admin/v1/export-tasks"): workflowContract("EmptySuccessEnvelope", requiredBody("ExportTaskDeleteBatchRequest"), nil, noID),
		workflowKey(http.MethodGet, "/api/admin/v1/export-tasks/status-count"): workflowContract("ExportTaskStatusCountSuccessEnvelope", nil, []map[string]any{
			queryParameter("file_name", false, maxStringSchema(255), "File-name search."),
			queryParameter("kind", false, maxStringSchema(64), "Export kind filter."),
			queryParameter("title", false, maxStringSchema(100), "Export title search."),
		}, noID),
		workflowKey(http.MethodDelete, "/api/admin/v1/export-tasks/:id"): workflowContract("EmptySuccessEnvelope", nil, nil, positiveID),

		workflowKey(http.MethodGet, "/api/admin/v1/ai-conversations"): workflowContract("AIConversationListSuccessEnvelope", nil, []map[string]any{
			queryParameter("agent_id", false, positiveIntegerSchema(), "Agent ID filter."),
			queryParameter("before_id", false, positiveIntegerSchema(), "Conversation cursor ID; must accompany before_time."),
			queryParameter("before_time", false, schemaWith(stringSchema(), "pattern", `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`), "Conversation cursor time; must accompany before_id."),
			queryParameter("limit", false, schemaWith(integerRangeSchema(1, 100), "default", 20), "Maximum conversations returned."),
		}, noID, "before_time and before_id must either both be present or both be absent"),
		workflowKey(http.MethodPost, "/api/admin/v1/ai-conversations"):       workflowContract("AIConversationCreateSuccessEnvelope", requiredBody("AIConversationCreateRequest"), nil, noID),
		workflowKey(http.MethodGet, "/api/admin/v1/ai-conversations/:id"):    workflowContract("AIConversationDetailSuccessEnvelope", nil, nil, positiveID),
		workflowKey(http.MethodPut, "/api/admin/v1/ai-conversations/:id"):    workflowContract("EmptySuccessEnvelope", requiredBody("AIConversationUpdateRequest"), nil, positiveID),
		workflowKey(http.MethodDelete, "/api/admin/v1/ai-conversations/:id"): workflowContract("EmptySuccessEnvelope", nil, nil, positiveID),
		workflowKey(http.MethodGet, "/api/admin/v1/ai-conversations/:id/messages"): workflowContract("AIMessageListSuccessEnvelope", nil, []map[string]any{
			queryParameter("before_id", false, positiveIntegerSchema(), "Message cursor ID."),
			queryParameter("limit", false, schemaWith(integerRangeSchema(1, 100), "default", 20), "Maximum messages returned."),
		}, positiveID),
		workflowKey(http.MethodPost, "/api/admin/v1/ai-conversations/:id/messages"): workflowContract("AIMessageSendSuccessEnvelope", requiredBody("AIMessageSendRequest"), nil, positiveID,
			"trimmed content must be non-empty or attachments must contain at least one image"),
		workflowKey(http.MethodPost, "/api/admin/v1/ai-conversations/:id/messages/cancel"): workflowContract("AIMessageCancelSuccessEnvelope", requiredBody("AIMessageCancelRequest"), nil, positiveID),
		workflowKey(http.MethodPost, "/api/admin/v1/ai-conversations/:id/messages/:message_id/revisions"): withPositivePathIDs(
			workflowContract("AIMessageSendSuccessEnvelope", requiredBody("AIMessageRevisionRequest"), nil, noID,
				"canonical replay key is the authenticated (user_id, request_id); conversation and source message belong to the request fingerprint"),
			"id", "message_id",
		),
		workflowKey(http.MethodPost, "/api/admin/v1/ai-conversations/:id/messages/:message_id/regenerations"): withPositivePathIDs(
			workflowContract("AIMessageSendSuccessEnvelope", requiredBody("AIMessageRegenerationRequest"), nil, noID,
				"canonical replay key is the authenticated (user_id, request_id); conversation and source message belong to the request fingerprint"),
			"id", "message_id",
		),
		workflowKey(http.MethodDelete, "/api/admin/v1/ai-conversations/:id/messages"): workflowContract("AIMessageDeleteSuccessEnvelope", requiredBody("AIMessageDeleteRequest"), nil, positiveID),
		workflowKey(http.MethodPut, "/api/admin/v1/ai-conversations/:id/read-cursor"): workflowContract("AIConversationReadCursorSuccessEnvelope", requiredBody("AIConversationReadCursorRequest"), nil, positiveID),

		workflowKey(http.MethodGet, "/api/admin/v1/ai-runs/page-init"): workflowContract("AIRunPageInitSuccessEnvelope", nil, aiRunPageInitQueryParameters(), noID),
		workflowKey(http.MethodGet, "/api/admin/v1/ai-runs"):           workflowContract("AIRunListSuccessEnvelope", nil, aiRunListQueryParameters(), noID),
		workflowKey(http.MethodGet, "/api/admin/v1/ai-runs/dashboard"): workflowContract("AIRunDashboardSuccessEnvelope", nil, aiRunDashboardQueryParameters(), noID),
		workflowKey(http.MethodGet, "/api/admin/v1/ai-runs/:id"):       workflowContract("AIRunDetailSuccessEnvelope", nil, nil, positiveID),
		workflowKey(http.MethodGet, "/api/admin/v1/ai-runs/:id/input-attachments/:ordinal/preview"): withPositivePathIDs(
			workflowContract("AIRunInputAttachmentPreviewSuccessEnvelope", nil, nil, noID), "id", "ordinal",
		),
		workflowKey(http.MethodPut, "/api/admin/v1/ai-runs/:id/user-feedback"): workflowContract("AIRunUserFeedbackSuccessEnvelope", requiredBody("AIRunUserFeedbackRequest"), nil, positiveID),
	}
}

func workflowKey(method string, path string) workflowOperationKey {
	return workflowOperationKey{Method: strings.ToUpper(strings.TrimSpace(method)), Path: strings.TrimSpace(path)}
}

func workflowContract(response string, request *workflowRequestBody, query []map[string]any, positiveID bool, rules ...string) workflowOperationContract {
	contract := workflowOperationContract{
		QueryParameters: query,
		ParameterRules:  append([]string(nil), rules...),
		RequestBody:     request,
		ResponseSchema:  response,
	}
	if positiveID {
		contract.PathParameters = map[string]map[string]any{"id": positiveIntegerSchema()}
	}
	return contract
}

func withPositivePathIDs(contract workflowOperationContract, names ...string) workflowOperationContract {
	contract.PathParameters = make(map[string]map[string]any, len(names))
	for _, name := range names {
		contract.PathParameters[name] = positiveIntegerSchema()
	}
	return contract
}

func requiredBody(schema string) *workflowRequestBody {
	return &workflowRequestBody{Schema: schema, Required: true}
}

func optionalBody(schema string) *workflowRequestBody {
	return &workflowRequestBody{Schema: schema}
}

func queryParameter(name string, required bool, schema map[string]any, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"required":    required,
		"description": description,
		"schema":      schema,
	}
}

func aiRunListQueryParameters() []map[string]any {
	return []map[string]any{
		queryParameter("agent_id", false, positiveIntegerSchema(), "Agent ID filter."),
		queryParameter("anomaly_as_of", false, schemaWith(maxStringSchema(64), "format", "date-time"), "RFC3339 instant used to evaluate stale and overdue anomalies."),
		queryParameter("billing_anomaly", false, stringEnumSchema("state_inconsistent", "open_overdue", "pricing_snapshot_missing", "legacy_unpriced", "unbilled_usage_incomplete", "unbilled_over_hold"), "Billing anomaly drilldown filter."),
		queryParameter("billing_reason", false, stringEnumSchema(
			"pending", "held", "settled_complete_usage", "released_before_dispatch", "released_insufficient_balance",
			"released_provider_failed", "released_outcome_unknown", "unbilled_usage_incomplete", "unbilled_over_hold", "legacy_unpriced",
		), "Billing reason filter."),
		queryParameter("billing_status", false, stringEnumSchema("pending", "held", "settled", "released", "unbilled"), "Billing status filter."),
		queryParameter("current_page", false, schemaWith(positiveIntegerSchema(), "default", 1), "One-based page number."),
		aiRunDateQueryParameter("date_end"),
		aiRunDateQueryParameter("date_start"),
		queryParameter("error_code", false, maxStringSchema(128), "Final provider attempt error code filter."),
		queryParameter("model_id", false, maxStringSchema(191), "Official or historical model ID filter."),
		queryParameter("page_size", false, schemaWith(integerRangeSchema(1, 50), "default", 20), "Number of rows per page."),
		queryParameter("platform", false, registeredPlatformSchema(), "Origin platform filter."),
		queryParameter("provider_id", false, positiveIntegerSchema(), "Provider ID filter."),
		queryParameter("request_id", false, maxStringSchema(128), "Request ID search."),
		queryParameter("run_anomaly", false, stringEnumSchema("failed", "timeout", "outcome_unknown", "stale_running"), "Run anomaly drilldown filter."),
		queryParameter("status", false, stringEnumSchema("running", "success", "failed", "canceled", "timeout", "outcome_unknown"), "Run status filter."),
		queryParameter("tool_code", false, maxStringSchema(128), "Tool code drilldown filter."),
		queryParameter("user_feedback", false, stringEnumSchema("liked", "unliked"), "Persisted user feedback filter."),
		queryParameter("user_id", false, positiveIntegerSchema(), "User ID filter."),
	}
}

func aiRunDashboardQueryParameters() []map[string]any {
	return []map[string]any{
		queryParameter("agent_id", false, positiveIntegerSchema(), "Agent ID filter."),
		aiRunDateQueryParameter("date_end"),
		aiRunDateQueryParameter("date_start"),
		queryParameter("model_id", false, maxStringSchema(191), "Official or historical model ID filter."),
		queryParameter("platform", false, registeredPlatformSchema(), "Origin platform filter."),
		queryParameter("provider_id", false, positiveIntegerSchema(), "Provider ID filter."),
		queryParameter("user_id", false, positiveIntegerSchema(), "User ID filter."),
	}
}

func aiRunPageInitQueryParameters() []map[string]any {
	return []map[string]any{
		aiRunDateQueryParameter("date_end"),
		aiRunDateQueryParameter("date_start"),
	}
}

func aiRunDateQueryParameter(name string) map[string]any {
	return queryParameter(name, false,
		schemaWith(maxStringSchema(10), "format", "date", "pattern", `^\d{4}-\d{2}-\d{2}$`),
		"Inclusive Asia/Shanghai calendar date input (YYYY-MM-DD); normalized output uses an exclusive end instant.",
	)
}

func workflowOperationContractFor(method string, path string) (workflowOperationContract, bool) {
	contract, ok := workflowOperationContracts[workflowKey(method, path)]
	return contract, ok
}

func workflowOperationParameters(pathParameters []map[string]any, contract workflowOperationContract) ([]map[string]any, error) {
	parameters := make([]map[string]any, 0, len(pathParameters)+len(contract.QueryParameters))
	seenPath := make(map[string]struct{}, len(pathParameters))
	for _, parameter := range pathParameters {
		copyParameter := cloneStringAnyMap(parameter)
		name, _ := copyParameter["name"].(string)
		if schema, ok := contract.PathParameters[name]; ok {
			copyParameter["schema"] = cloneStringAnyMap(schema)
			seenPath[name] = struct{}{}
		}
		parameters = append(parameters, copyParameter)
	}
	for name := range contract.PathParameters {
		if _, ok := seenPath[name]; !ok {
			return nil, fmt.Errorf("contract path parameter %q is not present in the runtime path", name)
		}
	}
	seenQuery := make(map[string]struct{}, len(contract.QueryParameters))
	for _, parameter := range contract.QueryParameters {
		name, _ := parameter["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("query parameter name is required")
		}
		if _, duplicate := seenQuery[name]; duplicate {
			return nil, fmt.Errorf("duplicate query parameter %q", name)
		}
		seenQuery[name] = struct{}{}
		parameters = append(parameters, cloneStringAnyMap(parameter))
	}
	return parameters, nil
}

func workflowOperationRequestBody(contract workflowOperationContract) map[string]any {
	if contract.RequestBody == nil {
		return nil
	}
	mediaType := normalizedContractMediaType(contract.RequestBody.MediaType)
	return map[string]any{
		"required": contract.RequestBody.Required,
		"content": map[string]any{
			mediaType: map[string]any{
				"schema": schemaReference(contract.RequestBody.Schema),
			},
		},
	}
}

func validateWorkflowSchemaReferences(contract workflowOperationContract, schemas map[string]any) error {
	if contract.ResponseSchema == "" {
		return fmt.Errorf("response schema is required")
	}
	if schemas[contract.ResponseSchema] == nil {
		return fmt.Errorf("response schema %q is not defined", contract.ResponseSchema)
	}
	if contract.RequestBody != nil {
		if contract.RequestBody.Schema == "" {
			return fmt.Errorf("request schema is required")
		}
		if schemas[contract.RequestBody.Schema] == nil {
			return fmt.Errorf("request schema %q is not defined", contract.RequestBody.Schema)
		}
	}
	return nil
}

func validateWorkflowOperationContracts(definitions []adminroute.Definition) error {
	runtime := make(map[workflowOperationKey]adminroute.Definition, len(definitions))
	for _, definition := range definitions {
		runtime[workflowKey(definition.Method, definition.Path)] = definition
	}
	keys := make([]workflowOperationKey, 0, len(workflowOperationContracts))
	for key := range workflowOperationContracts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		if keys[i].Path != keys[j].Path {
			return keys[i].Path < keys[j].Path
		}
		return keys[i].Method < keys[j].Method
	})
	for _, key := range keys {
		contract := workflowOperationContracts[key]
		definition, ok := runtime[key]
		if !ok {
			return fmt.Errorf("%s %s has no compiled runtime operation", key.Method, key.Path)
		}
		if definition.ResponseSchema != "" && definition.ResponseSchema != contract.ResponseSchema {
			return fmt.Errorf("%s %s response schema conflicts with route definition", key.Method, key.Path)
		}
		if definition.RequestSchema != "" {
			if contract.RequestBody == nil || definition.RequestSchema != contract.RequestBody.Schema {
				return fmt.Errorf("%s %s request schema conflicts with route definition", key.Method, key.Path)
			}
		}
	}
	return nil
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
