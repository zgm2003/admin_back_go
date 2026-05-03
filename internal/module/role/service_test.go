package role

import (
	"context"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/module/permission"
)

type fakePermissionDict struct {
	called bool
	result *permission.InitResponse
	err    *apperror.Error
}

func (f *fakePermissionDict) Init(ctx context.Context) (*permission.InitResponse, *apperror.Error) {
	f.called = true
	return f.result, f.err
}

type fakeCacheInvalidator struct {
	keys []string
	err  error
}

func (f *fakeCacheInvalidator) Delete(ctx context.Context, key string) error {
	f.keys = append(f.keys, key)
	return f.err
}

type fakeRepository struct {
	roles               []Role
	rolesByID           map[int64]*Role
	permissions         []permission.Permission
	permissionIDsByRole map[int64][]int64
	userIDsByRoleIDs    []int64
	userCountByRoleIDs  int64
	existsByName        bool
	err                 error

	createdRoleName          string
	createdRoleID            int64
	updatedRoleID            int64
	updatedFields            map[string]any
	deletedIDs               []int64
	clearDefault             bool
	setDefaultID             int64
	syncedRoleID             int64
	syncedIDs                []int64
	deletedRolePermissionIDs []int64
	deletedRoleByName        *Role
	restoredRoleID           int64
	restoredRole             Role
	txCalled                 bool
	createdRole              Role
}

func (f *fakeRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	f.txCalled = true
	return fn(f)
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Role, int64, error) {
	return f.roles, int64(len(f.roles)), f.err
}

func (f *fakeRepository) RolesByIDs(ctx context.Context, ids []int64) (map[int64]Role, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := make(map[int64]Role, len(ids))
	for _, role := range f.roles {
		result[role.ID] = role
	}
	for id, role := range f.rolesByID {
		if role != nil {
			result[id] = *role
		}
	}
	return result, nil
}

func (f *fakeRepository) RoleByID(ctx context.Context, id int64) (*Role, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.rolesByID != nil {
		return f.rolesByID[id], nil
	}
	for _, role := range f.roles {
		if role.ID == id {
			row := role
			return &row, nil
		}
	}
	return nil, nil
}

func (f *fakeRepository) ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error) {
	return f.existsByName, f.err
}

func (f *fakeRepository) FindDeletedByName(ctx context.Context, name string) (*Role, error) {
	return f.deletedRoleByName, f.err
}

func (f *fakeRepository) Create(ctx context.Context, row Role) (int64, error) {
	f.createdRoleName = row.Name
	f.createdRole = row
	if f.createdRoleID == 0 {
		f.createdRoleID = 88
	}
	return f.createdRoleID, f.err
}

func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedRoleID = id
	f.updatedFields = fields
	return f.err
}

func (f *fakeRepository) RestoreDeleted(ctx context.Context, id int64, row Role) error {
	f.restoredRoleID = id
	f.restoredRole = row
	return f.err
}

func (f *fakeRepository) Delete(ctx context.Context, ids []int64) error {
	f.deletedIDs = append([]int64{}, ids...)
	return f.err
}

func (f *fakeRepository) HasDefaultIn(ctx context.Context, ids []int64) (bool, error) {
	for _, id := range ids {
		role := f.rolesByID[id]
		if role != nil && role.IsDefault == permission.CommonYes {
			return true, nil
		}
	}
	for _, role := range f.roles {
		if role.IsDefault == permission.CommonYes {
			return true, nil
		}
	}
	return false, f.err
}

func (f *fakeRepository) CountUsersByRoleIDs(ctx context.Context, ids []int64) (int64, error) {
	return f.userCountByRoleIDs, f.err
}

func (f *fakeRepository) ClearDefault(ctx context.Context) error {
	f.clearDefault = true
	return f.err
}

func (f *fakeRepository) SetDefault(ctx context.Context, id int64) error {
	f.setDefaultID = id
	return f.err
}

func (f *fakeRepository) PermissionIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	return f.permissionIDsByRole, f.err
}

func (f *fakeRepository) AllActivePermissions(ctx context.Context) ([]permission.Permission, error) {
	return f.permissions, f.err
}

func (f *fakeRepository) SyncPermissions(ctx context.Context, roleID int64, permissionIDs []int64) error {
	f.syncedRoleID = roleID
	f.syncedIDs = append([]int64{}, permissionIDs...)
	return f.err
}

func (f *fakeRepository) DeleteRolePermissionsByRoleIDs(ctx context.Context, roleIDs []int64) error {
	f.deletedRolePermissionIDs = append([]int64{}, roleIDs...)
	return f.err
}

func (f *fakeRepository) UserIDsByRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	return f.userIDsByRoleIDs, f.err
}

func TestServiceInitUsesPermissionDictionary(t *testing.T) {
	dict := &fakePermissionDict{result: &permission.InitResponse{Dict: permission.PermissionDict{
		PermissionTree:        []permission.PermissionTreeNode{{ID: 1, Label: "系统", Value: 1}},
		PermissionPlatformArr: []dict.Option[string]{{Label: "admin", Value: "admin"}},
	}}}
	svc := NewService(&fakeRepository{}, dict, nil, nil)

	got, appErr := svc.Init(context.Background())

	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if !dict.called {
		t.Fatalf("expected permission dictionary to be called")
	}
	if len(got.Dict.PermissionTree) != 1 || got.Dict.PermissionPlatformArr[0].Value != "admin" {
		t.Fatalf("unexpected dict: %#v", got.Dict)
	}
}

func TestServiceListNormalizesRolePermissionIDsWithPageParents(t *testing.T) {
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.Local)
	repo := &fakeRepository{
		roles:               []Role{{ID: 7, Name: "运营", IsDefault: permission.CommonNo, CreatedAt: now, UpdatedAt: now}},
		permissionIDsByRole: map[int64][]int64{7: {1, 2, 3, 999}},
		permissions: []permission.Permission{
			{ID: 1, Type: permission.TypeDir, ParentID: permission.RootParentID, Status: permission.StatusActive, IsDel: permission.CommonNo},
			{ID: 2, Type: permission.TypePage, ParentID: 1, Status: permission.StatusActive, IsDel: permission.CommonNo},
			{ID: 3, Type: permission.TypeButton, ParentID: 2, Status: permission.StatusActive, IsDel: permission.CommonNo},
		},
	}
	svc := NewService(repo, &fakePermissionDict{}, nil, nil)

	got, appErr := svc.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 50})

	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if got.Page.Total != 1 || got.Page.TotalPage != 1 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
	if !reflect.DeepEqual(got.List[0].PermissionIDs, []int64{2, 3}) {
		t.Fatalf("expected page parent + button only, got %#v", got.List[0].PermissionIDs)
	}
}

func TestServiceCreateSyncsNormalizedPermissionsInTransaction(t *testing.T) {
	repo := &fakeRepository{
		createdRoleID: 22,
		permissions: []permission.Permission{
			{ID: 2, Type: permission.TypePage, ParentID: 1, Status: permission.StatusActive, IsDel: permission.CommonNo},
			{ID: 3, Type: permission.TypeButton, ParentID: 2, Status: permission.StatusActive, IsDel: permission.CommonNo},
		},
	}
	svc := NewService(repo, &fakePermissionDict{}, nil, nil)

	id, appErr := svc.Create(context.Background(), MutationInput{Name: "运营", PermissionIDs: []int64{3}})

	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 22 || !repo.txCalled || repo.createdRoleName != "运营" {
		t.Fatalf("create flow mismatch: id=%d tx=%v name=%q", id, repo.txCalled, repo.createdRoleName)
	}
	if repo.createdRole.IsDel != permission.CommonNo {
		t.Fatalf("expected created role is_del=%d, got %d", permission.CommonNo, repo.createdRole.IsDel)
	}
	if !reflect.DeepEqual(repo.syncedIDs, []int64{2, 3}) {
		t.Fatalf("expected normalized sync ids [2 3], got %#v", repo.syncedIDs)
	}
}

func TestServiceCreateRestoresSoftDeletedRoleName(t *testing.T) {
	repo := &fakeRepository{
		deletedRoleByName: &Role{ID: 66, Name: "ces", IsDefault: permission.CommonYes},
		permissions: []permission.Permission{
			{ID: 2, Type: permission.TypePage, ParentID: 1, Status: permission.StatusActive, IsDel: permission.CommonNo},
		},
	}
	svc := NewService(repo, &fakePermissionDict{}, nil, nil)

	id, appErr := svc.Create(context.Background(), MutationInput{Name: "ces", PermissionIDs: []int64{2}})

	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 66 {
		t.Fatalf("expected to restore deleted role id 66, got %d", id)
	}
	if repo.createdRoleName != "" {
		t.Fatalf("expected restore path to skip insert, got created role %#v", repo.createdRole)
	}
	if repo.restoredRoleID != 66 || repo.restoredRole.Name != "ces" || repo.restoredRole.IsDel != permission.CommonNo {
		t.Fatalf("expected restore path to reuse deleted role, got id=%d row=%#v", repo.restoredRoleID, repo.restoredRole)
	}
}

func TestServiceUpdateInvalidatesBoundUserButtonCaches(t *testing.T) {
	repo := &fakeRepository{
		rolesByID:        map[int64]*Role{9: {ID: 9, Name: "客服", IsDefault: permission.CommonNo}},
		permissions:      []permission.Permission{{ID: 2, Type: permission.TypePage, ParentID: 1, Status: permission.StatusActive, IsDel: permission.CommonNo}},
		userIDsByRoleIDs: []int64{101, 102},
	}
	cache := &fakeCacheInvalidator{}
	svc := NewService(repo, &fakePermissionDict{}, cache, []string{"admin", "app"})

	appErr := svc.Update(context.Background(), 9, MutationInput{Name: "客服主管", PermissionIDs: []int64{2}})

	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	wantKeys := []string{
		"auth_perm_uid_101_admin_rbac_page_grants",
		"auth_perm_uid_101_app_rbac_page_grants",
		"auth_perm_uid_102_admin_rbac_page_grants",
		"auth_perm_uid_102_app_rbac_page_grants",
	}
	if !reflect.DeepEqual(cache.keys, wantKeys) {
		t.Fatalf("cache keys mismatch\nwant=%#v\n got=%#v", wantKeys, cache.keys)
	}
}

func TestServiceDeleteRejectsDefaultOrBoundRoles(t *testing.T) {
	cases := []struct {
		name string
		repo *fakeRepository
		msg  string
	}{
		{name: "default", repo: &fakeRepository{rolesByID: map[int64]*Role{1: {ID: 1, IsDefault: permission.CommonYes}}}, msg: "默认角色不能删除"},
		{name: "bound", repo: &fakeRepository{rolesByID: map[int64]*Role{2: {ID: 2, IsDefault: permission.CommonNo}}, userCountByRoleIDs: 1}, msg: "角色已绑定用户，不能删除"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(tc.repo, &fakePermissionDict{}, nil, nil)
			appErr := svc.Delete(context.Background(), []int64{tc.repo.firstRoleID()})
			if appErr == nil || appErr.Message != tc.msg {
				t.Fatalf("expected %q, got %#v", tc.msg, appErr)
			}
		})
	}
}

func (f *fakeRepository) firstRoleID() int64 {
	for id := range f.rolesByID {
		return id
	}
	return 0
}
