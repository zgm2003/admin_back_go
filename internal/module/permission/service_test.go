package permission

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRepository struct {
	grantedIDs     []int64
	perms          []Permission
	err            error
	findRoleCalled int
}

func (f fakeRepository) PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	return f.grantedIDs, f.err
}

func (f fakeRepository) AllActivePermissions(ctx context.Context) ([]Permission, error) {
	return f.perms, f.err
}

func (f fakeRepository) ListPermissions(ctx context.Context, query PermissionListQuery) ([]Permission, error) {
	return f.perms, f.err
}

func (f fakeRepository) GetPermission(ctx context.Context, id int64) (*Permission, error) {
	for _, item := range f.perms {
		if item.ID == id {
			return &item, f.err
		}
	}
	return nil, f.err
}

func (f fakeRepository) ExistsByPlatformCode(ctx context.Context, platform string, code string, excludeID int64) (bool, error) {
	return false, f.err
}

func (f fakeRepository) ExistsByPlatformPath(ctx context.Context, platform string, path string, excludeID int64) (bool, error) {
	return false, f.err
}

func (f fakeRepository) ExistsByPlatformI18nKey(ctx context.Context, platform string, i18nKey string, excludeID int64) (bool, error) {
	return false, f.err
}

func (f fakeRepository) FindDeletedByPlatformCode(ctx context.Context, platform string, code string) (*Permission, error) {
	return nil, f.err
}

func (f fakeRepository) CreatePermission(ctx context.Context, row Permission) (int64, error) {
	return row.ID, f.err
}

func (f fakeRepository) RestoreDeletedPermission(ctx context.Context, id int64, row Permission) error {
	return f.err
}

func (f fakeRepository) UpdatePermission(ctx context.Context, id int64, fields map[string]any) error {
	return f.err
}

func (f fakeRepository) HasChildrenOutsideIDs(ctx context.Context, ids []int64) (bool, error) {
	return false, f.err
}

func (f fakeRepository) CascadeIDs(ctx context.Context, ids []int64) ([]int64, error) {
	return ids, f.err
}

func (f fakeRepository) ActiveChildren(ctx context.Context, parentID int64) ([]Permission, error) {
	return nil, f.err
}

func (f fakeRepository) DeletePermissions(ctx context.Context, ids []int64) error {
	return f.err
}

func (f fakeRepository) RoleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error) {
	return nil, f.err
}

func (f fakeRepository) UserIDsByRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	return nil, f.err
}

func TestServiceBuildContextReturnsEmptyForInvalidRoleID(t *testing.T) {
	svc := NewService(&fakeRepository{}, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 0, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	assertEmptyContext(t, got)
}

func TestServiceBuildContextAddsAncestorMenusRoutesAndButtonCodes(t *testing.T) {
	repo := &fakeRepository{
		grantedIDs: []int64{3},
		perms: []Permission{
			{ID: 1, Name: "系统", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/system", Icon: "setting", Sort: 20, ShowMenu: 1},
			{ID: 2, Name: "用户", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/system/user", Component: "/system/user/index", Icon: "user", Sort: 10, ShowMenu: 2, I18nKey: "menu.user"},
			{ID: 3, Name: "新增", ParentID: 2, Type: TypeButton, Platform: "admin", Code: "user_add", Sort: 30},
			{ID: 4, Name: "别的平台", ParentID: 0, Type: TypePage, Platform: "app", Path: "/app", Component: "/app/index", Sort: 1},
		},
	}
	svc := NewService(repo, []string{"admin", "app"})

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
	if repo.findRoleCalled != 0 {
		t.Fatalf("permission context builder must not re-query role, got %d role lookups", repo.findRoleCalled)
	}
	child := got.Permissions[0].Children[0]
	if child.Index != "2" || child.Label != "用户" || child.I18nKey != "menu.user" || child.ShowMenu != 2 || child.ParentID != 1 {
		t.Fatalf("child menu mismatch: %#v", child)
	}
}

func TestServiceBuildContextPageCodeIsButtonGrantForReadOnlyRoutes(t *testing.T) {
	repo := &fakeRepository{
		grantedIDs: []int64{2},
		perms: []Permission{
			{ID: 1, Name: "支付管理", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/payment", Sort: 1, ShowMenu: CommonYes},
			{ID: 2, Name: "支付订单", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/payment/order", Component: "payment/order", Code: "payment_order_list", Sort: 2, ShowMenu: CommonYes},
		},
	}
	svc := NewService(repo, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 7, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !reflect.DeepEqual(got.ButtonCodes, []string{"payment_order_list"}) {
		t.Fatalf("page code must be usable by PermissionCheck, got %#v", got.ButtonCodes)
	}
	if len(got.Router) != 1 || got.Router[0].Path != "/payment/order" {
		t.Fatalf("expected page route to remain, got %#v", got.Router)
	}
	if got.Router[0].ViewKey != "payment/order" {
		t.Fatalf("payment route view key must not include /index, got %#v", got.Router[0])
	}
}

func TestServiceBuildContextDoesNotExposeDirPathAsMenuRoute(t *testing.T) {
	repo := &fakeRepository{
		grantedIDs: []int64{2},
		perms: []Permission{
			{ID: 1, Name: "支付管理", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/payment", Sort: 1, ShowMenu: CommonYes},
			{ID: 2, Name: "支付订单", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/payment/order", Component: "payment/order", Code: "payment_order_list", Sort: 2, ShowMenu: CommonYes},
		},
	}
	svc := NewService(repo, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 7, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got.Permissions) != 1 {
		t.Fatalf("expected one root menu, got %#v", got.Permissions)
	}
	if got.Permissions[0].Path != "" {
		t.Fatalf("directory menu path must not leak as a route, got %q", got.Permissions[0].Path)
	}
	if len(got.Permissions[0].Children) != 1 || got.Permissions[0].Children[0].Path != "/payment/order" {
		t.Fatalf("page child route path must remain, got %#v", got.Permissions[0].Children)
	}
}

func TestServiceBuildContextKeepsRetiredPayRoutesOutOfPaymentFixtures(t *testing.T) {
	svc := NewService(&fakeRepository{
		grantedIDs: []int64{2},
		perms: []Permission{
			{ID: 1, Name: "支付管理", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/payment", Sort: 1, ShowMenu: CommonYes},
			{ID: 2, Name: "支付订单", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/payment/order", Component: "payment/order", Code: "payment_order_list", Sort: 2, ShowMenu: CommonYes},
		},
	}, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 7, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	for _, route := range got.Router {
		if route.Path == "/pay" || route.Path == "/pay/transaction" || route.Path == "/wallet" {
			t.Fatalf("retired pay/wallet route leaked into payment fixture: %#v", route)
		}
	}
	for _, code := range got.ButtonCodes {
		if code == "pay_transaction_list" {
			t.Fatalf("retired pay_ button code leaked into payment fixture: %#v", got.ButtonCodes)
		}
	}
}

func TestServiceBuildContextButtonGrantImpliesParentPageRoute(t *testing.T) {
	repo := &fakeRepository{
		grantedIDs: []int64{3},
		perms: []Permission{
			{ID: 1, Name: "权限", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/permission", Sort: 1, ShowMenu: CommonYes},
			{ID: 2, Name: "菜单", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/permission/menu", Component: "/permission/menu/index", Sort: 2, ShowMenu: CommonYes},
			{ID: 3, Name: "新增菜单", ParentID: 2, Type: TypeButton, Platform: "admin", Code: "permission_menu_add", Sort: 3},
		},
	}
	svc := NewService(repo, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 7, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !reflect.DeepEqual(got.ButtonCodes, []string{"permission_menu_add"}) {
		t.Fatalf("buttonCodes mismatch: %#v", got.ButtonCodes)
	}
	if len(got.Router) != 1 || got.Router[0].Path != "/permission/menu" {
		t.Fatalf("button grant must materialize parent page route, got %#v", got.Router)
	}
	if len(got.Permissions) != 1 || len(got.Permissions[0].Children) != 1 {
		t.Fatalf("button grant must materialize parent menu tree, got %#v", got.Permissions)
	}
}

func TestServiceBuildContextShowMenuDoesNotRemovePagePermissionTruth(t *testing.T) {
	repo := &fakeRepository{
		grantedIDs: []int64{2},
		perms: []Permission{
			{ID: 1, Name: "系统", ParentID: 0, Type: TypeDir, Platform: "admin", Path: "/system", Sort: 1, ShowMenu: CommonYes},
			{ID: 2, Name: "隐藏页", ParentID: 1, Type: TypePage, Platform: "admin", Path: "/system/hidden", Component: "/system/hidden/index", Sort: 2, ShowMenu: CommonNo},
		},
	}
	svc := NewService(repo, []string{"admin"})

	got, appErr := svc.BuildContextByRole(context.Background(), 7, "admin")

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got.Router) != 1 || got.Router[0].Path != "/system/hidden" {
		t.Fatalf("show_menu must not remove page route permission truth, got %#v", got.Router)
	}
	if len(got.Permissions) != 1 || len(got.Permissions[0].Children) != 1 {
		t.Fatalf("expected hidden page to remain in permission tree with show_menu flag, got %#v", got.Permissions)
	}
	hidden := got.Permissions[0].Children[0]
	if hidden.Index != "2" || hidden.ShowMenu != CommonNo {
		t.Fatalf("expected hidden page show_menu=%d to be preserved, got %#v", CommonNo, hidden)
	}
}

func TestServiceBuildContextKeepsChildrenWhenParentSortsBeforeChild(t *testing.T) {
	svc := NewService(&fakeRepository{
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
	svc := NewService(&fakeRepository{
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
	svc := NewService(&fakeRepository{
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
	svc := NewService(&fakeRepository{}, []string{"admin"})

	_, appErr := svc.BuildContextByRole(context.Background(), 1, "unknown")

	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected bad request app error, got %#v", appErr)
	}
}

func TestServiceBuildContextWrapsRepositoryError(t *testing.T) {
	svc := NewService(&fakeRepository{err: errors.New("db down")}, []string{"admin"})

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
