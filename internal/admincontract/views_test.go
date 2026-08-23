package admincontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestContextViewSeedIdentityIsPublishedForCutover(t *testing.T) {
	seedPath := filepath.Join("..", "..", "database", "seeds", "admin_permissions.sql")
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read permission seed: %v", err)
	}
	if strings.Count(string(seed), "(122, '上下文工程', '/ai/context'") != 1 {
		t.Fatal("context menu seed identity is missing or duplicated")
	}
}

func TestViewsDescribeUsersMeAndCurrentAdminViewKeys(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	if document.SchemaVersion != ViewSchemaVersion {
		t.Fatalf("schema_version=%q", document.SchemaVersion)
	}
	if got, want := len(document.Views), 35; got != want {
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
		"ai/context",
		"ai/chat",
		"component/display",
		"component/download",
		"component/effect",
		"component/form",
		"component/upload",
		"payment/recharge",
		"payment/redeem-codes",
		"system/queueMonitor",
		"user/userManager",
	} {
		if _, exists := seen[required]; !exists {
			t.Fatalf("missing active Admin view %q", required)
		}
	}
}

func TestAdminViewsPublishOfficialModelRoute(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	want := View{
		Path: "/ai/official-models", ViewKey: "ai/official-models", I18nKey: "menu.ai_official_models",
		ShowMenu: 1, PermissionCodes: []string{"ai_official_model_list"},
	}
	for _, view := range document.Views {
		if view.ViewKey == want.ViewKey {
			if !reflect.DeepEqual(view, want) {
				t.Fatalf("official model view=%#v want=%#v", view, want)
			}
			return
		}
	}
	t.Fatalf("missing official model view %q", want.ViewKey)
}

func TestViewsPublishPaymentRedeemCodeManagement(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}

	want := View{
		Path:            "/payment/redeem-codes",
		ViewKey:         "payment/redeem-codes",
		I18nKey:         "menu.payment_redeem_codes",
		ShowMenu:        1,
		PermissionCodes: []string{"payment_redeem_code_generate", "payment_redeem_code_list", "payment_redeem_code_void"},
	}
	for _, view := range document.Views {
		if view.ViewKey == want.ViewKey {
			if !reflect.DeepEqual(view, want) {
				t.Fatalf("redeem code view=%#v want=%#v", view, want)
			}
			return
		}
	}
	t.Fatalf("missing redeem code view %q", want.ViewKey)
}

func TestViewsProtectAIRunMonitoringWithListPermission(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}

	want := View{
		Path:            "/ai/runs",
		ViewKey:         "ai/runs",
		I18nKey:         "menu.ai_runs",
		ShowMenu:        1,
		PermissionCodes: []string{"ai_run_list"},
	}
	for _, view := range document.Views {
		if view.ViewKey == want.ViewKey {
			if !reflect.DeepEqual(view, want) {
				t.Fatalf("AI run view=%#v want=%#v", view, want)
			}
			return
		}
	}
	t.Fatalf("missing AI run view %q", want.ViewKey)
}

func TestViewsProtectUserManagerWithPagePermission(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}

	want := View{
		Path:            "/user/userManager",
		ViewKey:         "user/userManager",
		I18nKey:         "menu.user_userManager",
		ShowMenu:        1,
		PermissionCodes: []string{"user_userManager"},
	}
	for _, view := range document.Views {
		if view.ViewKey == want.ViewKey {
			if !reflect.DeepEqual(view, want) {
				t.Fatalf("user manager view=%#v want=%#v", view, want)
			}
			return
		}
	}
	t.Fatalf("missing user manager view %q", want.ViewKey)
}

func TestViewsProtectRoleManagerWithPagePermission(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}

	want := View{
		Path:            "/permission/role",
		ViewKey:         "permission/role",
		I18nKey:         "menu.permission_role",
		ShowMenu:        1,
		PermissionCodes: []string{"permission_role"},
	}
	for _, view := range document.Views {
		if view.ViewKey == want.ViewKey {
			if !reflect.DeepEqual(view, want) {
				t.Fatalf("role manager view=%#v want=%#v", view, want)
			}
			return
		}
	}
	t.Fatalf("missing role manager view %q", want.ViewKey)
}

func TestViewsProtectPermissionGovernancePagesWithPagePermissions(t *testing.T) {
	bundle := mustBuildBundle(t)
	var document ViewsDocument
	if err := json.Unmarshal(bundle.Artifacts["views.json"], &document); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	want := map[string]string{
		"permission/permission":   "permission_permission",
		"permission/authPlatform": "permission_authPlatform",
	}
	for _, view := range document.Views {
		code, ok := want[view.ViewKey]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(view.PermissionCodes, []string{code}) {
			t.Fatalf("view %s permission_codes=%#v want [%s]", view.ViewKey, view.PermissionCodes, code)
		}
		delete(want, view.ViewKey)
	}
	if len(want) != 0 {
		t.Fatalf("missing governance views: %#v", want)
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
	if !containsString(got, "system_mail_logView") {
		t.Fatalf("button code enum does not publish system_mail_logView")
	}
}
