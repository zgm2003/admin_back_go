package admin

import (
	"net/http"
	"testing"

	"admin_back_go/internal/server/adminroute"

	"github.com/gin-gonic/gin"
)

func TestAdminUserRoutePermissions(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), &fakeUserService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}

	wantPermissions := map[string]string{
		"GET /api/admin/v1/users/page-init":    "user_userManager",
		"GET /api/admin/v1/users":              "user_userManager",
		"GET /api/admin/v1/users/:id/profile":  "user_userManager",
		"PUT /api/admin/v1/users/:id":          "user_userManager_edit",
		"PATCH /api/admin/v1/users/:id/status": "user_userManager_edit",
		"PATCH /api/admin/v1/users":            "user_userManager_batchEdit",
		"DELETE /api/admin/v1/users/:id":       "user_userManager_del",
		"DELETE /api/admin/v1/users":           "user_userManager_del",
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

	me := definitions[http.MethodGet+" /api/admin/v1/users/me"]
	if me.Access.Kind != adminroute.AccessAuthenticated || me.Access.PermissionCode != "" {
		t.Fatalf("users/me access=%#v want authenticated", me.Access)
	}
}
