package admin

import (
	"testing"

	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func TestAdminRoleRoutePermissions(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), &fakeHTTPService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}

	wantPermissions := map[string]string{
		"GET /api/admin/v1/roles/page-init":     "permission_role",
		"GET /api/admin/v1/roles":               "permission_role",
		"POST /api/admin/v1/roles":              "permission_role_add",
		"PUT /api/admin/v1/roles/:id":           "permission_role_edit",
		"PATCH /api/admin/v1/roles/:id/default": "permission_role_setDefault",
		"DELETE /api/admin/v1/roles/:id":        "permission_role_del",
		"DELETE /api/admin/v1/roles":            "permission_role_del",
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
