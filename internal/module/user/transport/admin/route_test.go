package admin

import (
	"net/http"
	"reflect"
	"testing"

	usermodule "admin_back_go/internal/module/user"
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

func TestAdminUserRouteContracts(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), &fakeUserService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}

	tests := map[string]struct {
		query    any
		request  any
		response any
	}{
		"GET /api/admin/v1/users/page-init":   {response: usermodule.PageInitResponse{}},
		"GET /api/admin/v1/users":             {query: listRequest{}, response: usermodule.ListResponse{}},
		"GET /api/admin/v1/users/:id/profile": {response: usermodule.ProfileResponse{}},
		"PUT /api/admin/v1/users/:id": {
			request:  updateRequest{},
			response: adminroute.EmptyData{},
		},
		"PATCH /api/admin/v1/users/:id/status": {
			request:  statusRequest{},
			response: adminroute.EmptyData{},
		},
		"PATCH /api/admin/v1/users": {
			request:  batchProfileRequest{},
			response: adminroute.EmptyData{},
		},
		"DELETE /api/admin/v1/users/:id": {response: adminroute.EmptyData{}},
		"DELETE /api/admin/v1/users": {
			request:  deleteBatchRequest{},
			response: adminroute.EmptyData{},
		},
	}

	for route, want := range tests {
		definition, ok := definitions[route]
		if !ok {
			t.Fatalf("route %s is missing", route)
		}
		if definition.Contract == nil {
			t.Errorf("route %s contract is missing", route)
			continue
		}
		if got, expected := reflect.TypeOf(definition.Contract.Query), reflect.TypeOf(want.query); got != expected {
			t.Errorf("route %s query type=%v want %v", route, got, expected)
		}
		if got, expected := reflect.TypeOf(definition.Contract.Request), reflect.TypeOf(want.request); got != expected {
			t.Errorf("route %s request type=%v want %v", route, got, expected)
		}
		if got, expected := reflect.TypeOf(definition.Contract.Response), reflect.TypeOf(want.response); got != expected {
			t.Errorf("route %s response type=%v want %v", route, got, expected)
		}
	}
}
