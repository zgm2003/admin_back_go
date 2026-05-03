package permission

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeManagementRepository struct {
	grantedIDs []int64
	perms      []Permission

	listQuery   PermissionListQuery
	listQueries []PermissionListQuery
	createRow   Permission
	updateID    int64
	updateMap   map[string]any
	deleteIDs   []int64

	existsCode    bool
	existsPath    bool
	existsI18nKey bool
	deletedByCode *Permission
	restoredID    int64
	restoredRow   Permission
	children      []Permission
	cascadeIDs    []int64
	hasChildren   bool
	err           error

	roleIDsByPermissionIDs []int64
	userIDsByRoleIDs       []int64
	rolePermissionQueryIDs []int64
	userRoleQueryIDs       []int64
}

func (f *fakeManagementRepository) PermissionIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	return f.grantedIDs, f.err
}

func (f *fakeManagementRepository) AllActivePermissions(ctx context.Context) ([]Permission, error) {
	return f.perms, f.err
}

func (f *fakeManagementRepository) ListPermissions(ctx context.Context, query PermissionListQuery) ([]Permission, error) {
	f.listQuery = query
	f.listQueries = append(f.listQueries, query)
	return f.perms, f.err
}

func (f *fakeManagementRepository) GetPermission(ctx context.Context, id int64) (*Permission, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, item := range f.perms {
		if item.ID == id {
			return &item, nil
		}
	}
	return nil, nil
}

func (f *fakeManagementRepository) ExistsByPlatformCode(ctx context.Context, platform string, code string, excludeID int64) (bool, error) {
	return f.existsCode, f.err
}

func (f *fakeManagementRepository) ExistsByPlatformPath(ctx context.Context, platform string, path string, excludeID int64) (bool, error) {
	return f.existsPath, f.err
}

func (f *fakeManagementRepository) ExistsByPlatformI18nKey(ctx context.Context, platform string, i18nKey string, excludeID int64) (bool, error) {
	return f.existsI18nKey, f.err
}

func (f *fakeManagementRepository) FindDeletedByPlatformCode(ctx context.Context, platform string, code string) (*Permission, error) {
	return f.deletedByCode, f.err
}

func (f *fakeManagementRepository) CreatePermission(ctx context.Context, row Permission) (int64, error) {
	f.createRow = row
	return 99, f.err
}

func (f *fakeManagementRepository) RestoreDeletedPermission(ctx context.Context, id int64, row Permission) error {
	f.restoredID = id
	f.restoredRow = row
	return f.err
}

func (f *fakeManagementRepository) UpdatePermission(ctx context.Context, id int64, fields map[string]any) error {
	f.updateID = id
	f.updateMap = fields
	return f.err
}

func (f *fakeManagementRepository) HasChildrenOutsideIDs(ctx context.Context, ids []int64) (bool, error) {
	f.deleteIDs = ids
	return f.hasChildren, f.err
}

func (f *fakeManagementRepository) CascadeIDs(ctx context.Context, ids []int64) ([]int64, error) {
	if len(f.cascadeIDs) > 0 {
		return f.cascadeIDs, f.err
	}
	return ids, f.err
}

func (f *fakeManagementRepository) ActiveChildren(ctx context.Context, parentID int64) ([]Permission, error) {
	return f.children, f.err
}

func (f *fakeManagementRepository) DeletePermissions(ctx context.Context, ids []int64) error {
	f.deleteIDs = ids
	return f.err
}

func (f *fakeManagementRepository) RoleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error) {
	f.rolePermissionQueryIDs = append([]int64{}, permissionIDs...)
	return f.roleIDsByPermissionIDs, f.err
}

func (f *fakeManagementRepository) UserIDsByRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	f.userRoleQueryIDs = append([]int64{}, roleIDs...)
	return f.userIDsByRoleIDs, f.err
}

type fakePermissionCacheInvalidator struct {
	keys []string
	err  error
}

func (f *fakePermissionCacheInvalidator) Delete(ctx context.Context, key string) error {
	f.keys = append(f.keys, key)
	return f.err
}

func TestServiceInitReturnsTypedDictWithoutFallbackFields(t *testing.T) {
	repo := &fakeManagementRepository{perms: []Permission{
		{ID: 1, Name: "系统", ParentID: RootParentID, Platform: "admin", Type: TypeDir},
		{ID: 2, Name: "用户", ParentID: 1, Platform: "admin", Type: TypePage},
	}}
	svc := NewService(repo, []string{"admin", "app"})

	got, appErr := svc.Init(context.Background())

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got.Dict.PermissionTypeArr) != 3 {
		t.Fatalf("expected three permission types, got %#v", got.Dict.PermissionTypeArr)
	}
	if len(got.Dict.PermissionPlatformArr) != 2 {
		t.Fatalf("expected configured platforms, got %#v", got.Dict.PermissionPlatformArr)
	}
	if got.Dict.PermissionPlatformArr[0].Label != "admin" || got.Dict.PermissionPlatformArr[0].Value != "admin" {
		t.Fatalf("expected admin platform label to stay code-like, got %#v", got.Dict.PermissionPlatformArr[0])
	}
	if got.Dict.PermissionPlatformArr[1].Label != "app" || got.Dict.PermissionPlatformArr[1].Value != "app" {
		t.Fatalf("expected app platform label to stay code-like, got %#v", got.Dict.PermissionPlatformArr[1])
	}
	if len(got.Dict.PermissionTree) != 1 || got.Dict.PermissionTree[0].Children[0].ID != 2 {
		t.Fatalf("permission tree mismatch: %#v", got.Dict.PermissionTree)
	}
}

func TestServiceListReturnsPermissionTreeForRequestedPlatform(t *testing.T) {
	repo := &fakeManagementRepository{perms: []Permission{
		{ID: 1, Name: "系统", ParentID: RootParentID, Platform: "admin", Type: TypeDir, Sort: 1, Status: CommonYes, ShowMenu: CommonYes},
		{ID: 2, Name: "用户", ParentID: 1, Platform: "admin", Type: TypePage, Path: "/user", Component: "user/index", Sort: 2, Status: CommonYes, ShowMenu: CommonYes},
		{ID: 3, Name: "APP", ParentID: RootParentID, Platform: "app", Type: TypeDir, Sort: 1, Status: CommonYes, ShowMenu: CommonYes},
	}}
	svc := NewService(repo, []string{"admin", "app"})

	got, appErr := svc.List(context.Background(), PermissionListQuery{Platform: "admin", Name: "用"})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(repo.listQueries) == 0 || repo.listQueries[0].Platform != "admin" || repo.listQueries[0].Name != "用" {
		t.Fatalf("query was not passed to repository: %#v", repo.listQueries)
	}
	if len(got) != 1 || got[0].ID != 1 || len(got[0].Children) != 1 || got[0].Children[0].ID != 2 {
		t.Fatalf("list tree mismatch: %#v", got)
	}
	if got[0].Children[0].TypeName != "页面" {
		t.Fatalf("expected typed type name, got %#v", got[0].Children[0])
	}
}

func TestServiceCreateRejectsAdminRootButton(t *testing.T) {
	svc := NewService(&fakeManagementRepository{}, []string{"admin"})

	_, appErr := svc.Create(context.Background(), PermissionMutationInput{
		Platform: "admin",
		Type:     TypeButton,
		Name:     "新增",
		Code:     "permission_add",
		Sort:     1,
	})

	if appErr == nil || appErr.Message != "按钮类型的父节点只能是页面" {
		t.Fatalf("expected admin root button rejection, got %#v", appErr)
	}
}

func TestServiceCreateWritesOnlyTypeSpecificFields(t *testing.T) {
	repo := &fakeManagementRepository{perms: []Permission{
		{ID: 1, Name: "系统", ParentID: RootParentID, Platform: "admin", Type: TypeDir},
	}}
	svc := NewService(repo, []string{"admin"})

	id, appErr := svc.Create(context.Background(), PermissionMutationInput{
		Platform:  "admin",
		Type:      TypePage,
		Name:      "用户",
		ParentID:  1,
		Icon:      "User",
		Path:      "/system/user",
		Component: "system/user/index",
		I18nKey:   "menu.system_user",
		Code:      "must_not_be_written",
		Sort:      10,
		ShowMenu:  CommonYes,
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if id != 99 {
		t.Fatalf("expected repository id 99, got %d", id)
	}
	if repo.createRow.Code != "" {
		t.Fatalf("page create must not persist button code: %#v", repo.createRow)
	}
	if repo.createRow.Path != "/system/user" || repo.createRow.Component != "system/user/index" || repo.createRow.I18nKey != "menu.system_user" {
		t.Fatalf("page fields were not persisted: %#v", repo.createRow)
	}
}

func TestServiceCreateRestoresSoftDeletedButtonCode(t *testing.T) {
	repo := &fakeManagementRepository{
		perms: []Permission{
			{ID: 12, Name: "后台菜单管理", ParentID: 2, Platform: "admin", Type: TypePage},
		},
		deletedByCode: &Permission{ID: 77, Platform: "admin", Type: TypeButton, Code: "permission_permission_export", IsDel: CommonYes},
	}
	svc := NewService(repo, []string{"admin"})

	id, appErr := svc.Create(context.Background(), PermissionMutationInput{
		Platform: "admin",
		Type:     TypeButton,
		Name:     "导出",
		ParentID: 12,
		Code:     "permission_permission_export",
		Sort:     10,
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if id != 77 || repo.restoredID != 77 {
		t.Fatalf("expected soft-deleted row to be restored, got id=%d restoredID=%d", id, repo.restoredID)
	}
	if repo.createRow.Name != "" {
		t.Fatalf("expected create to be skipped when restore is possible, got %#v", repo.createRow)
	}
	if repo.restoredRow.Name != "导出" || repo.restoredRow.ParentID != 12 || repo.restoredRow.IsDel != CommonNo {
		t.Fatalf("restored row mismatch: %#v", repo.restoredRow)
	}
}

func TestServiceUpdateRejectsMovingNodeUnderDescendant(t *testing.T) {
	repo := &fakeManagementRepository{
		perms:      []Permission{{ID: 10, Name: "系统", ParentID: RootParentID, Platform: "admin", Type: TypeDir}},
		cascadeIDs: []int64{10, 11, 12},
	}
	svc := NewService(repo, []string{"admin"})

	appErr := svc.Update(context.Background(), 10, PermissionMutationInput{
		Platform: "admin",
		Type:     TypeDir,
		Name:     "系统",
		ParentID: 12,
		I18nKey:  "menu.system",
		Sort:     1,
		ShowMenu: CommonYes,
	})

	if appErr == nil || appErr.Message != "节点不能挂到自己的后代下面" {
		t.Fatalf("expected descendant rejection, got %#v", appErr)
	}
}

func TestServiceUpdateInvalidatesUsersGrantedChangedPermissionSubtree(t *testing.T) {
	repo := &fakeManagementRepository{
		perms: []Permission{
			{ID: 1, Name: "系统", ParentID: RootParentID, Platform: "admin", Type: TypeDir},
			{ID: 2, Name: "用户", ParentID: 1, Platform: "admin", Type: TypePage},
		},
		cascadeIDs:             []int64{2, 3},
		roleIDsByPermissionIDs: []int64{9},
		userIDsByRoleIDs:       []int64{101, 102},
	}
	cache := &fakePermissionCacheInvalidator{}
	svc := NewService(repo, []string{"admin"}, WithCacheInvalidator(cache))

	appErr := svc.Update(context.Background(), 2, PermissionMutationInput{
		Platform:  "admin",
		Type:      TypePage,
		Name:      "用户管理",
		ParentID:  1,
		Path:      "/system/user",
		Component: "system/user/index",
		I18nKey:   "menu.system_user",
		Sort:      10,
		ShowMenu:  CommonYes,
	})

	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if !reflect.DeepEqual(repo.rolePermissionQueryIDs, []int64{2, 3}) {
		t.Fatalf("expected cascade permission ids to be queried, got %#v", repo.rolePermissionQueryIDs)
	}
	if !reflect.DeepEqual(repo.userRoleQueryIDs, []int64{9}) {
		t.Fatalf("expected affected role ids to be queried, got %#v", repo.userRoleQueryIDs)
	}
	wantKeys := []string{
		"auth_perm_uid_101_admin_rbac_page_grants",
		"auth_perm_uid_102_admin_rbac_page_grants",
	}
	if !reflect.DeepEqual(cache.keys, wantKeys) {
		t.Fatalf("cache keys mismatch\nwant=%#v\n got=%#v", wantKeys, cache.keys)
	}
}

func TestServiceDeleteInvalidatesUsersBeforeRolePermissionLinksAreDeleted(t *testing.T) {
	repo := &fakeManagementRepository{
		roleIDsByPermissionIDs: []int64{9},
		userIDsByRoleIDs:       []int64{101},
	}
	cache := &fakePermissionCacheInvalidator{}
	svc := NewService(repo, []string{"admin"}, WithCacheInvalidator(cache))

	appErr := svc.Delete(context.Background(), []int64{2})

	if appErr != nil {
		t.Fatalf("expected delete to succeed, got %v", appErr)
	}
	if !reflect.DeepEqual(repo.rolePermissionQueryIDs, []int64{2}) {
		t.Fatalf("expected deleted permission ids to be queried before delete, got %#v", repo.rolePermissionQueryIDs)
	}
	if !reflect.DeepEqual(repo.deleteIDs, []int64{2}) {
		t.Fatalf("expected delete ids to reach repository, got %#v", repo.deleteIDs)
	}
	if !reflect.DeepEqual(cache.keys, []string{"auth_perm_uid_101_admin_rbac_page_grants"}) {
		t.Fatalf("cache keys mismatch: %#v", cache.keys)
	}
}

func TestServiceChangeStatusInvalidatesUsersGrantedChangedPermissionSubtree(t *testing.T) {
	repo := &fakeManagementRepository{
		cascadeIDs:             []int64{2, 3},
		roleIDsByPermissionIDs: []int64{9},
		userIDsByRoleIDs:       []int64{101},
	}
	cache := &fakePermissionCacheInvalidator{}
	svc := NewService(repo, []string{"admin"}, WithCacheInvalidator(cache))

	appErr := svc.ChangeStatus(context.Background(), 2, CommonNo)

	if appErr != nil {
		t.Fatalf("expected status change to succeed, got %v", appErr)
	}
	if !reflect.DeepEqual(repo.rolePermissionQueryIDs, []int64{2, 3}) {
		t.Fatalf("expected cascade permission ids to be queried, got %#v", repo.rolePermissionQueryIDs)
	}
	if !reflect.DeepEqual(cache.keys, []string{"auth_perm_uid_101_admin_rbac_page_grants"}) {
		t.Fatalf("cache keys mismatch: %#v", cache.keys)
	}
}

func TestServiceDeleteRejectsPartialSubtree(t *testing.T) {
	repo := &fakeManagementRepository{hasChildren: true}
	svc := NewService(repo, []string{"admin"})

	appErr := svc.Delete(context.Background(), []int64{1})

	if appErr == nil || appErr.Message != "存在子节点未被勾选，不能删除" {
		t.Fatalf("expected partial subtree rejection, got %#v", appErr)
	}
}

func TestServiceWrapsManagementRepositoryError(t *testing.T) {
	svc := NewService(&fakeManagementRepository{err: errors.New("db down")}, []string{"admin"})

	_, appErr := svc.List(context.Background(), PermissionListQuery{Platform: "admin"})

	if appErr == nil || appErr.Code != 500 {
		t.Fatalf("expected internal app error, got %#v", appErr)
	}
}

func TestNormalizeIDsForMutation(t *testing.T) {
	got := normalizeIDsForMutation([]int64{0, 2, 2, 1})
	want := []int64{2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids mismatch: got %#v want %#v", got, want)
	}
}
