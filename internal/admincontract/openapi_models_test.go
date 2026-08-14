package admincontract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"admin_back_go/internal/module/ai/contextengine"
	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/enum"

	"github.com/google/go-cmp/cmp"
)

type modeledQuery struct {
	CurrentPage int    `form:"current_page" binding:"required,min=1"`
	Keyword     string `form:"keyword" binding:"omitempty,max=50"`
}

type modeledRequest struct {
	Name  string   `json:"name" binding:"required,min=1,max=64"`
	Tags  []string `json:"tags" binding:"omitempty,max=3,dive,min=1,max=20"`
	Note  *string  `json:"note" binding:"omitempty,max=100"`
	State int      `json:"state" binding:"required,oneof=1 2"`
}

type modeledItem struct {
	ID    int64   `json:"id"`
	Label string  `json:"label"`
	Note  *string `json:"note"`
}

type modeledResponse struct {
	List []modeledItem `json:"list"`
}

type modeledFixedScoreResponse struct {
	Score contextengine.FixedScore `json:"score"`
}

type modeledDiveRequest struct {
	Values []string `json:"values" binding:"omitempty,min=1,dive,required,max=32"`
}

func TestModelFieldRequiredStopsAtDive(t *testing.T) {
	field := reflect.TypeOf(modeledDiveRequest{}).Field(0)
	if modelFieldRequired(modelSchemaInput, field, false, modelValidationTokens(field)) {
		t.Fatal("element-level required made the parent array required")
	}
}

func TestOpenAPIGeneratesFixedScoreAsJSONString(t *testing.T) {
	document, err := buildOpenAPI([]adminroute.Definition{{
		Method:      http.MethodGet,
		Path:        "/api/admin/v1/modeled-fixed-score",
		OperationID: "get_api_admin_v1_modeled_fixed_score",
		Access:      adminroute.Authenticated(),
		Audit:       adminroute.NoAudit("contract test"),
		Contract: &adminroute.HTTPContract{
			Response: modeledFixedScoreResponse{},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rawComponents := document["components"].(map[string]any)["schemas"].(map[string]any)
	components := make(map[string]map[string]any, len(rawComponents))
	for name, raw := range rawComponents {
		components[name] = raw.(map[string]any)
	}
	score := openAPIPropertySchema(t, components, "Go_internal_admincontract_modeledFixedScoreResponse_Output", "score")
	if score["type"] != "string" {
		t.Fatalf("FixedScore JSON schema=%#v, want string", score)
	}
}

func TestUploadRuleResponsePublishesClosedExtensionEnums(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name       string
		schemaName string
		property   string
		items      bool
		want       []string
	}{
		{
			name:       "rule image extensions",
			schemaName: "Go_internal_module_uploadconfig_RuleItem_Output",
			property:   "image_exts",
			items:      true,
			want:       enum.UploadImageExts,
		},
		{
			name:       "rule file extensions",
			schemaName: "Go_internal_module_uploadconfig_RuleItem_Output",
			property:   "file_exts",
			items:      true,
			want:       enum.UploadFileExts,
		},
		{
			name:       "image option value",
			schemaName: "Go_internal_module_uploadconfig_UploadImageExtOption_Output",
			property:   "value",
			want:       enum.UploadImageExts,
		},
		{
			name:       "file option value",
			schemaName: "Go_internal_module_uploadconfig_UploadFileExtOption_Output",
			property:   "value",
			want:       enum.UploadFileExts,
		},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			schema := openAPIPropertySchema(t, document.Components.Schemas, check.schemaName, check.property)
			if check.items {
				items, ok := schema["items"]
				if !ok {
					t.Fatalf("%s.%s has no items schema: %#v", check.schemaName, check.property, schema)
				}
				schema = resolveOpenAPISchema(t, document.Components.Schemas, items)
			}
			got := openAPIStringEnum(t, document.Components.Schemas, schema)
			if diff := cmp.Diff(check.want, got); diff != "" {
				t.Fatalf("extension enum mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAIProviderAPIProtocolResponseUsesClosedEnum(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		schemaName string
		property   string
	}{
		{schemaName: "Go_internal_module_ai_provider_ProviderDTO_Output", property: "api_protocol"},
		{schemaName: "Go_internal_module_ai_provider_APIProtocolOption_Output", property: "value"},
	}
	for _, check := range checks {
		schema := openAPIPropertySchema(t, document.Components.Schemas, check.schemaName, check.property)
		got := openAPIStringEnum(t, document.Components.Schemas, schema)
		if diff := cmp.Diff(aiprovider.APIProtocols, got); diff != "" {
			t.Fatalf("file input mode enum mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestAIProviderModelKindAndAgentContextProfileContracts(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}

	wantKinds := []string{
		string(aiprovider.ModelKindChat), string(aiprovider.ModelKindEmbedding),
		string(aiprovider.ModelKindRerank), string(aiprovider.ModelKindImage),
	}
	for _, check := range []struct {
		schema   string
		property string
	}{
		{schema: "Go_internal_module_ai_provider_ProviderModelDTO_Output", property: "model_kind"},
		{schema: "Go_internal_module_ai_agent_ProviderModelDTO_Output", property: "model_kind"},
		{schema: "Go_internal_module_ai_officialmodel_OfficialModelDTO_Output", property: "model_kind"},
	} {
		kind := openAPIPropertySchema(t, document.Components.Schemas, check.schema, check.property)
		if diff := cmp.Diff(wantKinds, openAPIStringEnum(t, document.Components.Schemas, kind)); diff != "" {
			t.Fatalf("%s.%s model kind enum mismatch (-want +got):\n%s", check.schema, check.property, diff)
		}
	}

	input := document.Components.Schemas["Go_internal_module_ai_provider_ProviderModelInput_Input"]
	if input == nil {
		t.Fatal("ProviderModelInput input schema is missing")
	}
	required := anyStrings(input["required"])
	properties := input["properties"].(map[string]any)
	for _, field := range []string{
		"id", "display_name", "status", "embedding_dimensions",
		"embedding_max_input_tokens", "embedding_token_counter_id",
	} {
		if properties[field] == nil {
			t.Fatalf("ProviderModelInput does not publish optional field %s", field)
		}
		if containsString(required, field) {
			t.Fatalf("ProviderModelInput compatibility field %s is required: %v", field, required)
		}
		assertNullableProperty(t, input, field)
	}
	agent := document.Components.Schemas["Go_internal_module_ai_agent_AgentDTO_Output"]
	if agent == nil {
		t.Fatal("AgentDTO output schema is missing")
	}
	assertNullableProperty(t, agent, "context_profile_id")
}

func openAPIPropertySchema(t *testing.T, schemas map[string]map[string]any, schemaName string, propertyName string) map[string]any {
	t.Helper()
	schema, exists := schemas[schemaName]
	if !exists {
		t.Fatalf("missing schema %s", schemaName)
	}
	schema = resolveOpenAPISchema(t, schemas, schema)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema %s has no properties: %#v", schemaName, schema)
	}
	property, exists := properties[propertyName]
	if !exists {
		t.Fatalf("schema %s has no property %s", schemaName, propertyName)
	}
	return resolveOpenAPISchema(t, schemas, property)
}

func openAPIStringEnum(t *testing.T, schemas map[string]map[string]any, raw any) []string {
	t.Helper()
	schema := resolveOpenAPISchema(t, schemas, raw)
	if schema["type"] != "string" {
		t.Fatalf("enum schema is not a string: %#v", schema)
	}
	values, ok := schema["enum"].([]any)
	if !ok || len(values) == 0 {
		t.Fatalf("schema does not publish a closed enum: %#v", schema)
	}
	result := make([]string, len(values))
	for index, value := range values {
		stringValue, ok := value.(string)
		if !ok {
			t.Fatalf("enum value %d is not a string: %#v", index, value)
		}
		result[index] = stringValue
	}
	return result
}

func resolveOpenAPISchema(t *testing.T, schemas map[string]map[string]any, raw any) map[string]any {
	t.Helper()
	schema, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("schema is not an object: %#v", raw)
	}
	for {
		ref, hasRef := schema["$ref"].(string)
		if !hasRef {
			return schema
		}
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(ref, prefix) {
			t.Fatalf("unsupported schema reference %q", ref)
		}
		name := strings.TrimPrefix(ref, prefix)
		resolved, exists := schemas[name]
		if !exists {
			t.Fatalf("schema reference %q is missing", ref)
		}
		schema = resolved
	}
}

func TestBrowserOnlyCredentialContractUsesClosedCookieTransport(t *testing.T) {
	bundle := mustBuildBundle(t)
	assertBrowserOnlyOpenAPI(t, bundle.Artifacts["openapi.json"])
}

func assertBrowserOnlyOpenAPI(t *testing.T, data []byte) {
	t.Helper()
	var document struct {
		Paths      map[string]map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas    map[string]map[string]any `json:"schemas"`
			Parameters map[string]map[string]any `json:"parameters"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/admin/v1/auth/refresh", "/api/admin/v1/auth/logout"} {
		operation := document.Paths[path]["post"]
		if operation == nil {
			t.Fatalf("missing POST %s", path)
		}
		if _, exists := operation["requestBody"]; exists {
			t.Fatalf("POST %s must not publish requestBody", path)
		}
	}

	for _, path := range []string{"/api/admin/v1/auth/login", "/api/admin/v1/auth/refresh"} {
		operation := document.Paths[path]["post"]
		response := operation["responses"].(map[string]any)["200"].(map[string]any)
		envelopeRef := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"].(string)
		envelope := document.Components.Schemas[strings.TrimPrefix(envelopeRef, "#/components/schemas/")]
		dataRef := envelope["properties"].(map[string]any)["data"].(map[string]any)["$ref"].(string)
		credential := document.Components.Schemas[strings.TrimPrefix(dataRef, "#/components/schemas/")]
		properties := credential["properties"].(map[string]any)
		if len(properties) != 2 || properties["access_token"] == nil || properties["expires_in"] == nil {
			t.Fatalf("POST %s credential properties=%#v", path, properties)
		}
		if credential["additionalProperties"] != false {
			t.Fatalf("POST %s credential schema is not closed: %#v", path, credential)
		}
	}

	for path := range document.Paths {
		if strings.HasPrefix(path, "/api/admin/v1/client-versions") {
			t.Fatalf("generated OpenAPI still publishes retired path %s", path)
		}
	}
	for name, schema := range document.Components.Schemas {
		if strings.Contains(strings.ToLower(name), "clientvariant") {
			t.Fatalf("generated OpenAPI still publishes retired schema %s", name)
		}
		assertNoRetiredCredentialProperty(t, "#/components/schemas/"+name, schema)
	}

	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	assertNoRetiredVariantHeader(t, "$", decoded)
}

func assertNoRetiredCredentialProperty(t *testing.T, location string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if rawProperties, ok := typed["properties"].(map[string]any); ok {
			for _, name := range []string{"refresh_token", "refresh_expires_in"} {
				if _, exists := rawProperties[name]; exists {
					t.Fatalf("%s publishes retired public property %s", location, name)
				}
			}
		}
		for key, child := range typed {
			assertNoRetiredCredentialProperty(t, location+"/"+key, child)
		}
	case []any:
		for index, child := range typed {
			assertNoRetiredCredentialProperty(t, fmt.Sprintf("%s/%d", location, index), child)
		}
	}
}

func assertNoRetiredVariantHeader(t *testing.T, location string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		name, hasName := typed["name"].(string)
		in, hasIn := typed["in"].(string)
		if hasName && hasIn && strings.EqualFold(in, "header") && strings.EqualFold(name, "X-Admin-Client-Variant") {
			t.Fatalf("%s publishes retired ClientVariant header parameter", location)
		}
		for key, child := range typed {
			assertNoRetiredVariantHeader(t, location+"/"+key, child)
		}
	case []any:
		for index, child := range typed {
			assertNoRetiredVariantHeader(t, fmt.Sprintf("%s/%d", location, index), child)
		}
	}
}

func TestOpenAPIGeneratesFieldCompleteContractFromRuntimeModels(t *testing.T) {
	document, err := buildOpenAPI([]adminroute.Definition{{
		Method:      http.MethodPost,
		Path:        "/api/admin/v1/modeled",
		OperationID: "post_api_admin_v1_modeled",
		Access:      adminroute.Authenticated(),
		Audit:       adminroute.NoAudit("contract test"),
		Contract: &adminroute.HTTPContract{
			Query:    modeledQuery{},
			Request:  modeledRequest{},
			Response: modeledResponse{},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}

	paths := document["paths"].(map[string]any)
	operation := paths["/api/admin/v1/modeled"].(map[string]any)["post"].(map[string]any)
	assertOperationResponseRef(t, operation, "200", "post_api_admin_v1_modeled_ResponseEnvelope")
	assertOperationRequestBody(t, operation, "post_api_admin_v1_modeled_Request", true)
	assertOperationParameters(t, operation, []string{"current_page", "keyword"}, []string{"current_page"}, nil)

	rawComponents := document["components"].(map[string]any)["schemas"].(map[string]any)
	components := make(map[string]map[string]any, len(rawComponents))
	for name, raw := range rawComponents {
		components[name] = raw.(map[string]any)
	}
	request := components["post_api_admin_v1_modeled_Request"]
	assertClosedSchemaWithRequired(t, components, "post_api_admin_v1_modeled_Request", "name", "state")
	requestProperties := request["properties"].(map[string]any)
	if got := requestProperties["name"].(map[string]any)["maxLength"]; got != float64(64) {
		t.Fatalf("name maxLength=%#v", got)
	}
	if got := requestProperties["tags"].(map[string]any)["maxItems"]; got != float64(3) {
		t.Fatalf("tags maxItems=%#v", got)
	}
	if got := requestProperties["state"].(map[string]any)["enum"]; !equalJSONValues(got, []any{float64(1), float64(2)}) {
		t.Fatalf("state enum=%#v", got)
	}

	envelope := components["post_api_admin_v1_modeled_ResponseEnvelope"]
	dataReference := envelope["properties"].(map[string]any)["data"].(map[string]any)["$ref"]
	if dataReference == nil || dataReference == "" {
		t.Fatalf("response data reference=%#v", dataReference)
	}
	item := components["Go_internal_admincontract_modeledItem_Output"]
	assertClosedSchemaWithRequired(t, components, "Go_internal_admincontract_modeledItem_Output", "id", "label", "note")
	assertNullableProperty(t, item, "note")
}

func TestOpenAPIGeneratesExplicitEmptyListResponseAlternative(t *testing.T) {
	document, err := buildOpenAPI([]adminroute.Definition{{
		Method:      http.MethodGet,
		Path:        "/api/admin/v1/modeled-alternative",
		OperationID: "get_api_admin_v1_modeled_alternative",
		Access:      adminroute.Authenticated(),
		Audit:       adminroute.NoAudit("contract test"),
		Contract: &adminroute.HTTPContract{
			ResponseAlternatives: []any{modeledResponse{}, adminroute.EmptyListData{}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	envelopeName := "get_api_admin_v1_modeled_alternative_ResponseEnvelope"
	envelope := document["components"].(map[string]any)["schemas"].(map[string]any)[envelopeName].(map[string]any)
	data := envelope["properties"].(map[string]any)["data"].(map[string]any)
	oneOf := data["oneOf"].([]any)
	if len(oneOf) != 2 {
		t.Fatalf("response alternatives=%#v", oneOf)
	}
	empty := oneOf[1].(map[string]any)
	if empty["type"] != "array" || empty["maxItems"] != 0 {
		t.Fatalf("empty-list alternative=%#v", empty)
	}
}

func TestOpenAPIPublishesMailDiagnosticLogContract(t *testing.T) {
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

	for _, path := range []string{"/api/admin/v1/mail/logs", "/api/admin/v1/mail/logs/{id}"} {
		operation := document.Paths[path]["get"]
		if operation == nil {
			t.Fatalf("missing GET %s", path)
		}
		access := operation["x-admin-access"].(map[string]any)
		if access["kind"] != "permission" || access["permission_code"] != "system_mail_logView" {
			t.Fatalf("GET %s access=%#v", path, access)
		}
		audit := operation["x-admin-audit"].(map[string]any)
		if audit["required"] != true || audit["skip_request_payload"] != true || audit["skip_response_payload"] != true {
			t.Fatalf("GET %s audit=%#v", path, audit)
		}
	}

	logDTO := document.Components.Schemas["Go_internal_module_mail_LogDTO_Output"]
	assertClosedSchemaWithRequired(t, document.Components.Schemas, "Go_internal_module_mail_LogDTO_Output",
		"verification_code", "verification_code_status", "verification_code_expires_at")
	for _, field := range []string{"verification_code", "verification_code_status", "verification_code_expires_at"} {
		assertNullableProperty(t, logDTO, field)
	}
	properties := logDTO["properties"].(map[string]any)
	diagnosticNames := make([]string, 0, 3)
	for name := range properties {
		if strings.HasPrefix(name, "verification_code") {
			diagnosticNames = append(diagnosticNames, name)
		}
	}
	sort.Strings(diagnosticNames)
	if want := []string{"verification_code", "verification_code_expires_at", "verification_code_status"}; !reflect.DeepEqual(diagnosticNames, want) {
		t.Fatalf("diagnostic properties=%v want=%v", diagnosticNames, want)
	}
	assertNullableStringProperty(t, properties["verification_code"].(map[string]any), nil)
	assertNullableStringProperty(t, properties["verification_code_status"].(map[string]any), []string{"sending", "not_expired", "expired", "send_failed"})
	expiresAt := assertNullableStringProperty(t, properties["verification_code_expires_at"].(map[string]any), nil)
	if got := expiresAt["pattern"]; got != `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$` {
		t.Fatalf("verification_code_expires_at pattern=%#v", got)
	}
	for _, forbidden := range []string{"key_id", "code_enc", "ciphertext", "template_data", "provider", "body"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("LogDTO exposes forbidden property %s", forbidden)
		}
	}
}

func assertNullableStringProperty(t *testing.T, property map[string]any, enum []string) map[string]any {
	t.Helper()
	variants, _ := property["anyOf"].([]any)
	if len(variants) != 2 {
		t.Fatalf("nullable property variants=%#v", property)
	}
	stringVariant, _ := variants[0].(map[string]any)
	if stringVariant["type"] != "string" {
		t.Fatalf("nullable property string variant=%#v", property)
	}
	if enum == nil {
		if _, exists := stringVariant["enum"]; exists {
			t.Fatalf("unexpected enum=%#v", stringVariant["enum"])
		}
		return stringVariant
	}
	if !equalJSONValues(stringVariant["enum"], stringSliceAsAny(enum)) {
		t.Fatalf("enum=%#v want=%v", stringVariant["enum"], enum)
	}
	return stringVariant
}

func stringSliceAsAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func TestIdentityRoutesPublishRuntimeModelContracts(t *testing.T) {
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

	operations := []struct {
		method string
		path   string
	}{
		{method: "get", path: "/api/admin/v1/auth/login-config"},
		{method: "get", path: "/api/admin/v1/auth/captcha"},
		{method: "post", path: "/api/admin/v1/auth/send-code"},
		{method: "post", path: "/api/admin/v1/auth/forgot-password"},
		{method: "post", path: "/api/admin/v1/auth/login"},
		{method: "post", path: "/api/admin/v1/auth/refresh"},
		{method: "post", path: "/api/admin/v1/auth/logout"},
		{method: "get", path: "/api/admin/v1/profile"},
		{method: "put", path: "/api/admin/v1/profile"},
		{method: "put", path: "/api/admin/v1/profile/security/password"},
		{method: "put", path: "/api/admin/v1/profile/security/email"},
		{method: "put", path: "/api/admin/v1/profile/security/phone"},
		{method: "get", path: "/api/admin/v1/users/{id}/profile"},
		{method: "get", path: "/api/admin/v1/user-sessions/page-init"},
		{method: "get", path: "/api/admin/v1/user-sessions"},
		{method: "get", path: "/api/admin/v1/user-sessions/stats"},
		{method: "patch", path: "/api/admin/v1/user-sessions/{id}/revoke"},
		{method: "patch", path: "/api/admin/v1/user-sessions/revoke"},
		{method: "get", path: "/api/admin/v1/users/login-logs/page-init"},
		{method: "get", path: "/api/admin/v1/users/login-logs"},
		{method: "get", path: "/api/admin/v1/auth-platforms/page-init"},
		{method: "get", path: "/api/admin/v1/auth-platforms"},
		{method: "post", path: "/api/admin/v1/auth-platforms"},
		{method: "put", path: "/api/admin/v1/auth-platforms/{id}"},
		{method: "patch", path: "/api/admin/v1/auth-platforms/{id}/status"},
		{method: "delete", path: "/api/admin/v1/auth-platforms/{id}"},
		{method: "delete", path: "/api/admin/v1/auth-platforms"},
		{method: "get", path: "/api/admin/v1/permissions/page-init"},
		{method: "get", path: "/api/admin/v1/permissions"},
		{method: "post", path: "/api/admin/v1/permissions"},
		{method: "put", path: "/api/admin/v1/permissions/{id}"},
		{method: "patch", path: "/api/admin/v1/permissions/{id}/status"},
		{method: "delete", path: "/api/admin/v1/permissions/{id}"},
		{method: "delete", path: "/api/admin/v1/permissions"},
		{method: "get", path: "/api/admin/v1/roles/page-init"},
		{method: "get", path: "/api/admin/v1/roles"},
		{method: "post", path: "/api/admin/v1/roles"},
		{method: "put", path: "/api/admin/v1/roles/{id}"},
		{method: "patch", path: "/api/admin/v1/roles/{id}/default"},
		{method: "delete", path: "/api/admin/v1/roles/{id}"},
		{method: "delete", path: "/api/admin/v1/roles"},
	}
	for _, expected := range operations {
		operation := document.Paths[expected.path][expected.method]
		if operation == nil {
			t.Fatalf("missing operation %s %s", expected.method, expected.path)
		}
		responses := operation["responses"].(map[string]any)
		for status, raw := range responses {
			if status == "default" {
				continue
			}
			response := raw.(map[string]any)
			content := response["content"].(map[string]any)
			media := content["application/json"].(map[string]any)
			ref := media["schema"].(map[string]any)["$ref"]
			if ref == "#/components/schemas/SuccessEnvelope" {
				t.Fatalf("%s %s uses generic SuccessEnvelope", expected.method, expected.path)
			}
		}
	}
}

func TestUserManagementRoutesPublishRuntimeModelContracts(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}

	operations := []struct {
		method string
		path   string
	}{
		{method: "get", path: "/api/admin/v1/users/page-init"},
		{method: "get", path: "/api/admin/v1/users"},
		{method: "get", path: "/api/admin/v1/users/{id}/profile"},
		{method: "put", path: "/api/admin/v1/users/{id}"},
		{method: "patch", path: "/api/admin/v1/users/{id}/status"},
		{method: "patch", path: "/api/admin/v1/users"},
		{method: "delete", path: "/api/admin/v1/users/{id}"},
		{method: "delete", path: "/api/admin/v1/users"},
	}
	for _, expected := range operations {
		operation := document.Paths[expected.path][expected.method]
		if operation == nil {
			t.Fatalf("missing operation %s %s", expected.method, expected.path)
		}
		assertJSONOperationIsFieldComplete(t, expected.method, expected.path, operation)
	}
}

func TestSystemAndCommunicationsRoutesPublishRuntimeModelContracts(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	prefixes := []string{
		"/api/admin/v1/cron-tasks",
		"/api/admin/v1/mail",
		"/api/admin/v1/notification-tasks",
		"/api/admin/v1/operation-logs",
		"/api/admin/v1/sms",
		"/api/admin/v1/system-logs",
		"/api/admin/v1/system-settings",
		"/api/admin/v1/upload-drivers",
		"/api/admin/v1/upload-rules",
		"/api/admin/v1/upload-settings",
		"/api/admin/v1/upload-tokens",
	}
	for _, prefix := range prefixes {
		matched := 0
		for path, pathItem := range document.Paths {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			for method, operation := range pathItem {
				matched++
				assertJSONOperationIsFieldComplete(t, method, path, operation)
			}
		}
		if matched == 0 {
			t.Fatalf("no operations matched %s", prefix)
		}
	}
}

func TestPaymentAndWalletRoutesPublishRuntimeModelContracts(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	prefixes := []string{
		"/api/admin/v1/payment/certificates",
		"/api/admin/v1/payment/configs",
		"/api/admin/v1/payment/ledger",
		"/api/admin/v1/payment/recharges",
		"/api/admin/v1/payment/wallets",
		"/api/admin/v1/wallet",
	}
	for _, prefix := range prefixes {
		matched := 0
		for path, pathItem := range document.Paths {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			for method, operation := range pathItem {
				matched++
				assertJSONOperationIsFieldComplete(t, method, path, operation)
			}
		}
		if matched == 0 {
			t.Fatalf("no operations matched %s", prefix)
		}
	}
}

func TestPaymentRechargePageInitOpenAPIOmitsRecent(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}

	const schemaName = "Go_internal_module_payment_RechargePageInitResponse_Output"
	schema := document.Components.Schemas[schemaName]
	if schema == nil || schema["additionalProperties"] != false {
		t.Fatalf("%s is not a closed schema: %#v", schemaName, schema)
	}
	properties := schema["properties"].(map[string]any)
	if _, exists := properties["recent"]; exists {
		t.Fatalf("%s still publishes recent: %#v", schemaName, properties["recent"])
	}
}

func TestAIAdministrationRoutesPublishRuntimeModelContracts(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(bundle.Artifacts["openapi.json"], &document); err != nil {
		t.Fatal(err)
	}
	prefixes := []string{
		"/api/admin/v1/ai-agents",
		"/api/admin/v1/ai/context-profiles",
		"/api/admin/v1/ai/context-spaces",
		"/api/admin/v1/ai/context-documents",
		"/api/admin/v1/ai/context-evaluations",
		"/api/admin/v1/ai/context/page-init",
		"/api/admin/v1/ai-providers",
		"/api/admin/v1/ai-tools",
	}
	for _, prefix := range prefixes {
		matched := 0
		for path, pathItem := range document.Paths {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			for method, operation := range pathItem {
				matched++
				assertJSONOperationIsFieldComplete(t, method, path, operation)
			}
		}
		if matched == 0 {
			t.Fatalf("no operations matched %s", prefix)
		}
	}
}

func assertJSONOperationIsFieldComplete(t *testing.T, method string, path string, operation map[string]any) {
	t.Helper()
	responses := operation["responses"].(map[string]any)
	for status, raw := range responses {
		if status == "default" {
			continue
		}
		response := raw.(map[string]any)
		content, ok := response["content"].(map[string]any)
		if !ok {
			continue
		}
		media, ok := content["application/json"].(map[string]any)
		if !ok {
			continue
		}
		ref := media["schema"].(map[string]any)["$ref"]
		if ref == "#/components/schemas/SuccessEnvelope" {
			t.Fatalf("%s %s uses generic SuccessEnvelope", method, path)
		}
	}
	if rawBody, exists := operation["requestBody"]; exists {
		body := rawBody.(map[string]any)
		content := body["content"].(map[string]any)
		for _, rawMedia := range content {
			media := rawMedia.(map[string]any)
			ref := media["schema"].(map[string]any)["$ref"]
			if ref == "#/components/schemas/GenericObject" {
				t.Fatalf("%s %s uses generic request body", method, path)
			}
		}
	}
}

func equalJSONValues(left any, right any) bool {
	leftValues, leftOK := left.([]any)
	rightValues, rightOK := right.([]any)
	if !leftOK || !rightOK || len(leftValues) != len(rightValues) {
		return false
	}
	for index := range leftValues {
		if leftValues[index] != rightValues[index] {
			return false
		}
	}
	return true
}
