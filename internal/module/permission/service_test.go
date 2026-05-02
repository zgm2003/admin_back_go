package permission

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRepository struct {
	role       *Role
	grantedIDs []int64
	perms      []Permission
	err        error
}

func (f fakeRepository) FindRole(ctx context.Context, roleID int64) (*Role, error) {
	return f.role, f.err
}

func (f fakeRepository) PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	return f.grantedIDs, f.err
}

func (f fakeRepository) AllActivePermissions(ctx context.Context) ([]Permission, error) {
	return f.perms, f.err
}

func TestServiceBuildContextReturnsEmptyWhenRoleMissing(t *testing.T) {
	svc := NewService(fakeRepository{}, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 1, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	assertEmptyContext(t, got)
}

func TestServiceBuildContextAddsAncestorMenusRoutesAndButtonCodes(t *testing.T) {
	svc := NewService(fakeRepository{
		role:       &Role{ID: 7, Name: "管理员"},
		grantedIDs: []int64{3},
		perms: []Permission{
			{ID: 1, Name: "系统", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/system", Icon: "setting", Sort: 20, ShowMenu: 1},
			{ID: 2, Name: "用户", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/system/user", Component: "/system/user/index", Icon: "user", Sort: 10, ShowMenu: 2, I18nKey: "menu.user"},
			{ID: 3, Name: "新增", ParentID: 2, Type: TypeButton, Platform: "admin", Code: "user_add", Sort: 30},
			{ID: 4, Name: "别的平台", ParentID: 0, Type: TypePage, Platform: "app", Path: "/app", Component: "/app/index", Sort: 1},
		},
	}, []string{"admin", "app"})

	got, appErr := svc.BuildContextByRole(context.Background(), 7, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !reflect.DeepEqual(got.ButtonCodes, []string{"user_add"}) {
		t.Fatalf("buttonCodes mismatch: %#v", got.ButtonCodes)
	}
	if len(got.Router) != 1 {
		t.Fatalf("expected one route, got %#v", got.Router)
	}
	if got.Router[0].Name != "menu_2" || got.Router[0].Path != "/system/user" || got.Router[0].ViewKey != "system/user/index" || got.Router[0].Meta["menuId"] != "2" {
		t.Fatalf("route mismatch: %#v", got.Router[0])
	}
	if len(got.Permissions) != 1 || got.Permissions[0].Index != "1" {
		t.Fatalf("root menu mismatch: %#v", got.Permissions)
	}
	child := got.Permissions[0].Children[0]
	if child.Index != "2" || child.Label != "用户" || child.I18nKey != "menu.user" || child.ShowMenu != 2 || child.ParentID != 1 {
		t.Fatalf("child menu mismatch: %#v", child)
	}
}

func TestServiceBuildContextKeepsChildrenWhenParentSortsBeforeChild(t *testing.T) {
	svc := NewService(fakeRepository{
		role:       &Role{ID: 9, Name: "管理员"},
		grantedIDs: []int64{3},
		perms: []Permission{
			{ID: 1, Name: "系统", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/system", Sort: 1, ShowMenu: 1},
			{ID: 2, Name: "用户", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/system/user", Component: "/system/user/index", Sort: 10, ShowMenu: 1},
			{ID: 3, Name: "新增", ParentID: 2, Type: TypeButton, Platform: "admin", Code: "user_add", Sort: 20},
		},
	}, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 9, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got.Permissions) != 1 || len(got.Permissions[0].Children) != 1 {
		t.Fatalf("expected root to keep child after tree materialization, got %#v", got.Permissions)
	}
}

func TestServiceBuildContextKeepsNestedChildrenWhenAncestorsSortBeforeDescendants(t *testing.T) {
	svc := NewService(fakeRepository{
		role:       &Role{ID: 10, Name: "管理员"},
		grantedIDs: []int64{4},
		perms: []Permission{
			{ID: 1, Name: "系统", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/system", Sort: 1, ShowMenu: 1},
			{ID: 2, Name: "权限", ParentID: 1, Type: TypeDir, Platform: "admin", Path: "/system/permission", Sort: 2, ShowMenu: 1},
			{ID: 3, Name: "角色", ParentID: 2, Type: TypePage, Platform: "admin", Path: "/system/permission/role", Component: "/system/permission/role/index", Sort: 3, ShowMenu: 1},
			{ID: 4, Name: "保存", ParentID: 3, Type: TypeButton, Platform: "admin", Code: "role_save", Sort: 4},
		},
	}, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 10, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got.Permissions) != 1 || len(got.Permissions[0].Children) != 1 || len(got.Permissions[0].Children[0].Children) != 1 {
		t.Fatalf("expected nested menu tree to keep page descendant, got %#v", got.Permissions)
	}
}
func TestServiceBuildContextAllowsRootButtonOnlyPlatform(t *testing.T) {
	svc := NewService(fakeRepository{
		role:       &Role{ID: 8, Name: "APP"},
		grantedIDs: []int64{11},
		perms: []Permission{
			{ID: 11, Name: "APP根按钮", ParentID: 0, Type: TypeButton, Platform: "app", Code: "app_root_button", Sort: 1},
		},
	}, []string{"admin", "app"})

	got, appErr := svc.BuildContextByRole(context.Background(), 8, "app")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !reflect.DeepEqual(got.ButtonCodes, []string{"app_root_button"}) {
		t.Fatalf("buttonCodes mismatch: %#v", got.ButtonCodes)
	}
	if len(got.Permissions) != 0 || len(got.Router) != 0 {
		t.Fatalf("expected pure button context, got %#v", got)
	}
}

func TestServiceBuildContextRejectsInvalidPlatform(t *testing.T) {
	svc := NewService(fakeRepository{role: &Role{ID: 1}}, []string{"admin"})

	_, appErr := svc.BuildContextByRole(context.Background(), 1, "unknown")

	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad request app error, got %#v", appErr)
	}
}

func TestServiceBuildContextWrapsRepositoryError(t *testing.T) {
	svc := NewService(fakeRepository{err: errors.New("db down")}, []string{"admin"})

	_, appErr := svc.BuildContextByRole(context.Background(), 1, "admin")

	if appErr == nil || appErr.Code != 500 {
		t.Fatalf("expected internal app error, got %#v", appErr)
	}
}

func TestButtonCacheKey(t *testing.T) {
	got := ButtonCacheKey(12, "admin")
	want := "auth_perm_uid_12_admin_rbac_page_grants"
	if got != want {
		t.Fatalf("cache key mismatch: got %q want %q", got, want)
	}
}

func assertEmptyContext(t *testing.T, ctx Context) {
	t.Helper()
	if len(ctx.Permissions) != 0 || len(ctx.Router) != 0 || len(ctx.ButtonCodes) != 0 {
		t.Fatalf("expected empty context, got %#v", ctx)
	}
}
