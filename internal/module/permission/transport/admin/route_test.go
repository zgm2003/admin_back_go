package admin

import (
	"context"
	"testing"

	permissionmodule "admin_back_go/internal/module/permission"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestAdminPermissionRoutePermissions(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), &fakeRouteService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}

	wantPermissions := map[string]string{
		"GET /api/admin/v1/permissions/page-init":    "permission_permission",
		"GET /api/admin/v1/permissions":              "permission_permission",
		"POST /api/admin/v1/permissions":             "permission_permission_add",
		"PUT /api/admin/v1/permissions/:id":          "permission_permission_edit",
		"PATCH /api/admin/v1/permissions/:id/status": "permission_permission_status",
		"DELETE /api/admin/v1/permissions/:id":       "permission_permission_del",
		"DELETE /api/admin/v1/permissions":           "permission_permission_del",
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

type fakeRouteService struct{}

func (*fakeRouteService) PageInit(context.Context) (*permissionmodule.InitResponse, *apperror.Error) {
	return nil, nil
}

func (*fakeRouteService) List(context.Context, permissionmodule.PermissionListQuery) ([]permissionmodule.PermissionListItem, *apperror.Error) {
	return nil, nil
}

func (*fakeRouteService) Create(context.Context, permissionmodule.PermissionMutationInput) (int64, *apperror.Error) {
	return 0, nil
}

func (*fakeRouteService) Update(context.Context, int64, permissionmodule.PermissionMutationInput) *apperror.Error {
	return nil
}

func (*fakeRouteService) Delete(context.Context, []int64) *apperror.Error { return nil }

func (*fakeRouteService) ChangeStatus(context.Context, int64, int) *apperror.Error { return nil }
