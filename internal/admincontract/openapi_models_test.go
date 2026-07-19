package admincontract

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"admin_back_go/internal/server/adminroute"
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

func TestBrowserOnlyCredentialContractUsesClosedCookieTransport(t *testing.T) {
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
	assertOperationParameters(t, operation, []string{"current_page", "keyword"}, []string{"current_page"}, false)

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
		"/api/admin/v1/ai-knowledge-bases",
		"/api/admin/v1/ai-knowledge-documents",
		"/api/admin/v1/ai-prompts",
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
