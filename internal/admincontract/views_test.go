package admincontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestViewsDescribeUsersMeAndCurrentAdminViewKeys(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	if document.SchemaVersion != ViewSchemaVersion {
		t.Fatalf("schema_version=%q", document.SchemaVersion)
	}
	if got, want := len(document.Views), 33; got != want {
		t.Fatalf("views=%d want=%d", got, want)
	}
	if document.UsersMe.Method != "GET" || document.UsersMe.Path != "/api/admin/v1/users/me" {
		t.Fatalf("users/me contract=%#v", document.UsersMe)
	}
	if document.UsersMe.ResponseSchema["additionalProperties"] != false {
		t.Fatalf("users/me response schema must be closed: %#v", document.UsersMe.ResponseSchema)
	}

	keys := make([]string, 0, len(document.Views))
	seen := make(map[string]struct{}, len(document.Views))
	for _, view := range document.Views {
		if view.Path == "" || view.ViewKey == "" || view.I18nKey == "" {
			t.Fatalf("incomplete view: %#v", view)
		}
		if strings.HasPrefix(view.Path, "/app/") || strings.HasPrefix(view.Path, "/canvas/") || strings.HasPrefix(view.ViewKey, "app/") || strings.HasPrefix(view.ViewKey, "canvas/") {
			t.Fatalf("retired view present: %#v", view)
		}
		if view.ViewKey == "system/clientVersion" || view.I18nKey == "menu.system_clientVersion" {
			t.Fatalf("retired client-version view remains published: %#v", view)
		}
		if _, duplicate := seen[view.ViewKey]; duplicate {
			t.Fatalf("duplicate view key %q", view.ViewKey)
		}
		seen[view.ViewKey] = struct{}{}
		keys = append(keys, view.ViewKey)
	}
	wantSorted := append([]string(nil), keys...)
	sort.Strings(wantSorted)
	if !reflect.DeepEqual(keys, wantSorted) {
		t.Fatalf("view keys are not sorted")
	}
	for _, required := range []string{
		"ai/chat",
		"component/display",
		"component/download",
		"component/effect",
		"component/form",
		"component/upload",
		"payment/recharge",
		"system/queueMonitor",
		"user/userManager",
	} {
		if _, exists := seen[required]; !exists {
			t.Fatalf("missing active Admin view %q", required)
		}
	}
}

func TestUsersMeSchemaClosesButtonCodesToPublishedPermissionCatalog(t *testing.T) {
	bundle := mustBuildBundle(t)
	var views ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &views); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	if schemaID, _ := views.UsersMe.ResponseSchema["$id"].(string); schemaID == "" {
		t.Fatal("users/me response schema is not a self-contained schema resource")
	}
	properties, ok := views.UsersMe.ResponseSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("users/me properties=%#v", views.UsersMe.ResponseSchema["properties"])
	}
	buttonCodes, ok := properties["buttonCodes"].(map[string]any)
	if !ok {
		t.Fatalf("buttonCodes schema=%#v", properties["buttonCodes"])
	}
	items, ok := buttonCodes["items"].(map[string]any)
	if !ok {
		t.Fatalf("buttonCodes items=%#v", buttonCodes["items"])
	}
	rawEnum, ok := items["enum"].([]any)
	if !ok {
		t.Fatalf("buttonCodes enum=%#v", items["enum"])
	}

	var permissions PermissionsDocument
	if err := json.Unmarshal(bundle.Artifacts["permissions.json"], &permissions); err != nil {
		t.Fatalf("decode permissions: %v", err)
	}
	got := make([]string, 0, len(rawEnum))
	for _, value := range rawEnum {
		got = append(got, value.(string))
	}
	if !reflect.DeepEqual(got, permissions.PermissionCodes) {
		t.Fatalf("button code enum does not match published permission catalog")
	}
}
