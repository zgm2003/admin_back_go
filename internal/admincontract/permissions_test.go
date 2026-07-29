package admincontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestPermissionsCatalogAndOperationPoliciesAreComplete(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document PermissionsDocument
	if err := json.Unmarshal(bundle.Artifacts["permissions.json"], &document); err != nil {
		t.Fatalf("decode permissions: %v", err)
	}
	if document.SchemaVersion != PermissionSchemaVersion {
		t.Fatalf("schema_version=%q", document.SchemaVersion)
	}
	wantSorted := append([]string(nil), document.PermissionCodes...)
	sort.Strings(wantSorted)
	if !reflect.DeepEqual(document.PermissionCodes, wantSorted) {
		t.Fatalf("permission codes are not sorted")
	}
	catalog := make(map[string]struct{}, len(document.PermissionCodes))
	for _, code := range document.PermissionCodes {
		if code == "" {
			t.Fatal("empty permission code")
		}
		if _, duplicate := catalog[code]; duplicate {
			t.Fatalf("duplicate permission code %q", code)
		}
		catalog[code] = struct{}{}
		if strings.HasPrefix(code, "system_clientVersion_") {
			t.Fatalf("retired client-version permission %q remains published", code)
		}
	}
	for _, required := range []string{"ai_agent_add", "ai_run_list", "ai_official_model_list", "ai_official_model_price_sync", "payment_recharge_add", "payment_recharge_list", "system_mail", "system_mail_logView", "devTools_queueMonitor_list"} {
		if _, exists := catalog[required]; !exists {
			t.Fatalf("missing active permission code %q", required)
		}
	}
	if got, want := len(document.PermissionCodes), 108; got != want {
		t.Fatalf("permission codes=%d want=%d", got, want)
	}

	if got, want := len(document.Operations), len(runtimeContractDefinitions(t)); got != want {
		t.Fatalf("operation policies=%d runtime definitions=%d", got, want)
	}
	seen := make(map[string]struct{}, len(document.Operations))
	for _, operation := range document.Operations {
		if operation.OperationID == "" || operation.Method == "" || operation.Path == "" {
			t.Fatalf("incomplete operation policy: %#v", operation)
		}
		if _, duplicate := seen[operation.OperationID]; duplicate {
			t.Fatalf("duplicate operation policy %q", operation.OperationID)
		}
		seen[operation.OperationID] = struct{}{}
		if operation.Access.Kind == "permission" {
			if _, exists := catalog[operation.Access.PermissionCode]; !exists {
				t.Fatalf("operation %s uses uncatalogued permission %q", operation.OperationID, operation.Access.PermissionCode)
			}
		}
		if !operation.Audit.Enabled && operation.Audit.Reason == "" {
			t.Fatalf("operation %s has no audit decision", operation.OperationID)
		}
	}
	for _, definition := range runtimeContractDefinitions(t) {
		if _, exists := seen[definition.OperationID]; !exists {
			t.Fatalf("runtime operation %q has no published policy", definition.OperationID)
		}
	}

	for _, expected := range []struct {
		path   string
		action string
		title  string
	}{
		{path: "/api/admin/v1/mail/logs", action: "list_logs", title: "查看邮件日志及验证码"},
		{path: "/api/admin/v1/mail/logs/:id", action: "view_log", title: "查看单条邮件日志及验证码"},
	} {
		operation, exists := findOperationPolicy(document.Operations, "GET", expected.path)
		if !exists {
			t.Fatalf("missing GET %s", expected.path)
		}
		if operation.Access.Kind != "permission" || operation.Access.PermissionCode != "system_mail_logView" {
			t.Fatalf("GET %s access=%#v", expected.path, operation.Access)
		}
		if !operation.Audit.Enabled || !operation.Audit.Required ||
			operation.Audit.Module != "mail" || operation.Audit.Action != expected.action || operation.Audit.Title != expected.title ||
			!operation.Audit.SkipRequestPayload || !operation.Audit.SkipResponsePayload {
			t.Fatalf("GET %s audit=%#v", expected.path, operation.Audit)
		}
	}
}

func TestPermissionsProtectAIRunMonitoringOperations(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document PermissionsDocument
	if err := json.Unmarshal(bundle.Artifacts["permissions.json"], &document); err != nil {
		t.Fatalf("decode permissions: %v", err)
	}

	for _, path := range []string{
		"/api/admin/v1/ai-runs/page-init",
		"/api/admin/v1/ai-runs",
		"/api/admin/v1/ai-runs/:id",
		"/api/admin/v1/ai-runs/stats",
		"/api/admin/v1/ai-runs/stats/latency",
		"/api/admin/v1/ai-runs/stats/by-date",
		"/api/admin/v1/ai-runs/stats/by-agent",
		"/api/admin/v1/ai-runs/stats/by-user",
	} {
		operation, exists := findOperationPolicy(document.Operations, "GET", path)
		if !exists {
			t.Fatalf("missing GET %s", path)
		}
		if operation.Access.Kind != "permission" || operation.Access.PermissionCode != "ai_run_list" {
			t.Fatalf("GET %s access=%#v", path, operation.Access)
		}
	}
}

func findOperationPolicy(operations []OperationPolicy, method string, path string) (OperationPolicy, bool) {
	for _, operation := range operations {
		if operation.Method == method && operation.Path == path {
			return operation, true
		}
	}
	return OperationPolicy{}, false
}
