package admin

import (
	"net/http"
	"testing"

	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func TestAdminAuthPlatformRoutePermissions(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), &fakeHTTPService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}

	wantPermissions := map[string]string{
		"GET /api/admin/v1/auth-platforms/page-init":    "permission_authPlatform",
		"GET /api/admin/v1/auth-platforms":              "permission_authPlatform",
		"POST /api/admin/v1/auth-platforms":             "permission_authPlatform_add",
		"PUT /api/admin/v1/auth-platforms/:id":          "permission_authPlatform_edit",
		"PATCH /api/admin/v1/auth-platforms/:id/status": "permission_authPlatform_status",
		"DELETE /api/admin/v1/auth-platforms/:id":       "permission_authPlatform_del",
		"DELETE /api/admin/v1/auth-platforms":           "permission_authPlatform_del",
	}
	for route, wantCode := range wantPermissions {
		definition, ok := definitions[route]
		if !ok {
			t.Fatalf("route %s is missing", route)
		}
		if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != wantCode {
			t.Fatalf("route %s access=%#v want permission %q", route, definition.Access, wantCode)
		}
	}
}

func TestAdminAuthPlatformRouteMethodsRemainStable(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), &fakeHTTPService{}, registry)

	want := map[string]struct{}{
		http.MethodGet + " /api/admin/v1/auth-platforms/page-init":    {},
		http.MethodGet + " /api/admin/v1/auth-platforms":              {},
		http.MethodPost + " /api/admin/v1/auth-platforms":             {},
		http.MethodPut + " /api/admin/v1/auth-platforms/:id":          {},
		http.MethodPatch + " /api/admin/v1/auth-platforms/:id/status": {},
		http.MethodDelete + " /api/admin/v1/auth-platforms/:id":       {},
		http.MethodDelete + " /api/admin/v1/auth-platforms":           {},
	}

	got := make(map[string]struct{}, len(registry.Definitions()))
	for _, definition := range registry.Definitions() {
		got[definition.Method+" "+definition.Path] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("route count=%d want %d: %#v", len(got), len(want), got)
	}
	for route := range want {
		if _, ok := got[route]; !ok {
			t.Fatalf("route %s is missing", route)
		}
	}
}
