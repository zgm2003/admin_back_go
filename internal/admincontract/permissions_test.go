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
	if got, want := len(document.PermissionCodes), 101; got != want {
		t.Fatalf("permission codes=%d want=%d", got, want)
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
	for _, required := range []string{"ai_agent_add", "payment_recharge_add", "payment_recharge_list", "system_mail", "devTools_queueMonitor_list"} {
		if _, exists := catalog[required]; !exists {
			t.Fatalf("missing active permission code %q", required)
		}
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
}
