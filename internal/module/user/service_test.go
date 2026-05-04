package user

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/permission"
)

type fakeUserRepository struct {
	user                 *User
	profile              *Profile
	role                 *Role
	roleOptions          []Role
	addresses            []Address
	listRows             []ListRow
	listTotal            int64
	entries              []QuickEntry
	rolesByID            map[int64]*Role
	listQuery            ListQuery
	txCalled             bool
	updatedUserID        int64
	updatedUserFields    map[string]any
	updatedProfileUserID int64
	updatedProfileFields map[string]any
	statusUserID         int64
	statusValue          int
	deletedIDs           []int64
	batchUpdate          BatchProfileUpdate
	err                  error
}

func (f *fakeUserRepository) FindUser(ctx context.Context, userID int64) (*User, error) {
	return f.user, f.err
}

func (f *fakeUserRepository) FindProfile(ctx context.Context, userID int64) (*Profile, error) {
	return f.profile, f.err
}

func (f *fakeUserRepository) FindRole(ctx context.Context, roleID int64) (*Role, error) {
	return f.role, f.err
}

func (f *fakeUserRepository) QuickEntries(ctx context.Context, userID int64) ([]QuickEntry, error) {
	return f.entries, f.err
}

func (f *fakeUserRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	f.txCalled = true
	return fn(f)
}

func (f *fakeUserRepository) RoleOptions(ctx context.Context) ([]Role, error) {
	return f.roleOptions, f.err
}

func (f *fakeUserRepository) ActiveAddresses(ctx context.Context) ([]Address, error) {
	return f.addresses, f.err
}

func (f *fakeUserRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.listRows, f.listTotal, f.err
}

func (f *fakeUserRepository) RoleByID(ctx context.Context, id int64) (*Role, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.rolesByID != nil {
		return f.rolesByID[id], nil
	}
	if f.role != nil && f.role.ID == id {
		return f.role, nil
	}
	return nil, nil
}

func (f *fakeUserRepository) UpdateUser(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedUserID = id
	f.updatedUserFields = fields
	return f.err
}

func (f *fakeUserRepository) UpdateProfile(ctx context.Context, userID int64, fields map[string]any) error {
	f.updatedProfileUserID = userID
	f.updatedProfileFields = fields
	return f.err
}

func (f *fakeUserRepository) UpdateStatus(ctx context.Context, id int64, status int) error {
	f.statusUserID = id
	f.statusValue = status
	return f.err
}

func (f *fakeUserRepository) SoftDelete(ctx context.Context, ids []int64) error {
	f.deletedIDs = ids
	return f.err
}

func (f *fakeUserRepository) BatchUpdateProfile(ctx context.Context, input BatchProfileUpdate) error {
	f.batchUpdate = input
	return f.err
}

type fakePermissionBuilder struct {
	called   bool
	roleID   int64
	platform string
	ctx      permission.Context
	err      *apperror.Error
}

func (f *fakePermissionBuilder) BuildContextByRole(ctx context.Context, roleID int64, platform string) (permission.Context, *apperror.Error) {
	f.called = true
	f.roleID = roleID
	f.platform = platform
	return f.ctx, f.err
}

type fakeButtonCache struct {
	called bool
	key    string
	values []string
	ttl    time.Duration
	err    error
}

func (f *fakeButtonCache) Set(ctx context.Context, key string, values []string, ttl time.Duration) error {
	f.called = true
	f.key = key
	f.values = values
	f.ttl = ttl
	return f.err
}

func (f *fakeButtonCache) Delete(ctx context.Context, key string) error {
	f.called = true
	f.key = key
	f.values = nil
	f.ttl = 0
	return f.err
}

func TestServiceInitReturnsLegacyResponseAndCachesButtons(t *testing.T) {
	builder := &fakePermissionBuilder{ctx: permission.Context{
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统", Children: []permission.MenuItem{}}},
		Router:      []permission.RouteItem{{Name: "menu_2", Path: "/system/user", ViewKey: "system/user/index", Meta: map[string]string{"menuId": "2"}}},
		ButtonCodes: []string{"user_add"},
	}}
	cache := &fakeButtonCache{}
	svc := NewService(&fakeUserRepository{
		user:    &User{ID: 1, Username: "admin", RoleID: 7},
		profile: &Profile{UserID: 1, Avatar: "avatar.png"},
		role:    &Role{ID: 7, Name: "管理员"},
		entries: []QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}, builder, cache, 30*time.Minute)

	got, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if got.UserID != 1 || got.Username != "admin" || got.Avatar != "avatar.png" || got.RoleName != "管理员" {
		t.Fatalf("base response mismatch: %#v", got)
	}
	if !reflect.DeepEqual(got.ButtonCodes, []string{"user_add"}) || len(got.Permissions) != 1 || len(got.Router) != 1 {
		t.Fatalf("permission response mismatch: %#v", got)
	}
	if len(got.QuickEntry) != 1 || got.QuickEntry[0].ID != 3 || got.QuickEntry[0].PermissionID != 2 || got.QuickEntry[0].Sort != 1 {
		t.Fatalf("quick_entry mismatch: %#v", got.QuickEntry)
	}
	if builder.roleID != 7 || builder.platform != "admin" {
		t.Fatalf("permission builder input mismatch: role=%d platform=%q", builder.roleID, builder.platform)
	}
	if cache.key != "auth_perm_uid_1_admin_rbac_page_grants" || !reflect.DeepEqual(cache.values, []string{"user_add"}) || cache.ttl != 30*time.Minute {
		t.Fatalf("button cache mismatch: %#v", cache)
	}
}

func TestServiceInitIgnoresButtonCacheFailure(t *testing.T) {
	builder := &fakePermissionBuilder{ctx: permission.Context{ButtonCodes: []string{"user_add"}}}
	svc := NewService(&fakeUserRepository{
		user: &User{ID: 1, Username: "admin", RoleID: 7},
		role: &Role{ID: 7, Name: "管理员"},
	}, builder, &fakeButtonCache{err: errors.New("redis down")}, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr != nil {
		t.Fatalf("expected cache failure to be ignored, got %v", appErr)
	}
}

func TestServiceInitReturnsNotFoundWhenUserMissing(t *testing.T) {
	svc := NewService(&fakeUserRepository{}, &fakePermissionBuilder{}, nil, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 404, Platform: "admin"})

	if appErr == nil || appErr.Code != 404 {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestServiceInitSkipsPermissionBuildWhenRoleMissing(t *testing.T) {
	builder := &fakePermissionBuilder{ctx: permission.Context{ButtonCodes: []string{"user_add"}}}
	cache := &fakeButtonCache{}
	svc := NewService(&fakeUserRepository{
		user:    &User{ID: 1, Username: "admin", RoleID: 7},
		profile: &Profile{UserID: 1, Avatar: "avatar.png"},
		role:    nil,
		entries: []QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}, builder, cache, time.Minute)

	got, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr != nil {
		t.Fatalf("expected init to succeed with empty permissions, got %v", appErr)
	}
	if builder.called {
		t.Fatalf("expected permission builder to be skipped when role is missing")
	}
	if cache.called {
		t.Fatalf("expected button cache write to be skipped when role is missing")
	}
	if got.RoleName != "" || len(got.ButtonCodes) != 0 || len(got.Permissions) != 0 || len(got.Router) != 0 {
		t.Fatalf("expected empty permission payload when role is missing, got %#v", got)
	}
	if len(got.QuickEntry) != 1 || got.QuickEntry[0].ID != 3 {
		t.Fatalf("quick_entry should still be returned, got %#v", got.QuickEntry)
	}
}

func TestServiceInitWrapsRepositoryError(t *testing.T) {
	svc := NewService(&fakeUserRepository{err: errors.New("db down")}, &fakePermissionBuilder{}, nil, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr == nil || appErr.Code != 500 {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
}

func TestServiceInitPropagatesPermissionError(t *testing.T) {
	builder := &fakePermissionBuilder{err: apperror.BadRequest("无效的平台标识: unknown")}
	svc := NewService(&fakeUserRepository{
		user: &User{ID: 1, Username: "admin", RoleID: 7},
		role: &Role{ID: 7, Name: "管理员"},
	}, builder, nil, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "unknown"})

	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected permission app error, got %#v", appErr)
	}
}

func TestServicePageInitReturnsRoleSexPlatformAndAddressTree(t *testing.T) {
	svc := NewService(&fakeUserRepository{
		roleOptions: []Role{{ID: 1, Name: "管理员"}, {ID: 2, Name: "运营"}},
		addresses: []Address{
			{ID: 1, ParentID: 0, Name: "中国"},
			{ID: 2, ParentID: 1, Name: "江苏"},
			{ID: 3, ParentID: 2, Name: "南京"},
		},
	}, &fakePermissionBuilder{}, nil, time.Minute)

	got, appErr := svc.PageInit(context.Background())

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if len(got.Dict.RoleArr) != 2 || got.Dict.RoleArr[0].Value != 1 || got.Dict.RoleArr[0].Label != "管理员" {
		t.Fatalf("role dict mismatch: %#v", got.Dict.RoleArr)
	}
	if len(got.Dict.SexArr) != 3 || got.Dict.SexArr[0].Value != enum.SexUnknown || got.Dict.SexArr[1].Value != enum.SexMale || got.Dict.SexArr[2].Value != enum.SexFemale {
		t.Fatalf("sex dict mismatch: %#v", got.Dict.SexArr)
	}
	if len(got.Dict.PlatformArr) != 2 || got.Dict.PlatformArr[0].Value != enum.PlatformAdmin || got.Dict.PlatformArr[1].Value != enum.PlatformApp {
		t.Fatalf("platform dict mismatch: %#v", got.Dict.PlatformArr)
	}
	tree := got.Dict.AuthAddressTree
	if len(tree) != 1 || tree[0].Value != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].Children[0].Value != 3 {
		t.Fatalf("address tree mismatch: %#v", tree)
	}
}

func TestServiceListFormatsUserRowsAndAddressPath(t *testing.T) {
	avatar := "avatar.png"
	bio := "hello"
	detail := "玄武区"
	sex := enum.SexMale
	addressID := int64(3)
	createdAt := time.Date(2026, 5, 4, 10, 11, 12, 0, time.Local)
	repo := &fakeUserRepository{
		addresses: []Address{
			{ID: 1, ParentID: 0, Name: "中国"},
			{ID: 2, ParentID: 1, Name: "江苏"},
			{ID: 3, ParentID: 2, Name: "南京"},
		},
		listRows: []ListRow{{
			ID:            7,
			Username:      "alice",
			Email:         "alice@example.com",
			Phone:         "15600000000",
			RoleID:        2,
			RoleName:      "运营",
			Avatar:        &avatar,
			Sex:           &sex,
			AddressID:     &addressID,
			DetailAddress: &detail,
			Bio:           &bio,
			CreatedAt:     createdAt,
		}},
		listTotal: 21,
	}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute)

	got, appErr := svc.List(context.Background(), ListQuery{
		CurrentPage: 2,
		PageSize:    10,
		Username:    " alice ",
		Email:       " alice@example.com ",
		AddressIDs:  []int64{3, 3, 0},
		Sex:         &sex,
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if repo.listQuery.Username != "alice" || repo.listQuery.Email != "alice@example.com" || !reflect.DeepEqual(repo.listQuery.AddressIDs, []int64{3}) {
		t.Fatalf("query was not normalized: %#v", repo.listQuery)
	}
	if got.Page.Total != 21 || got.Page.TotalPage != 3 || got.Page.CurrentPage != 2 || got.Page.PageSize != 10 {
		t.Fatalf("page mismatch: %#v", got.Page)
	}
	if len(got.List) != 1 {
		t.Fatalf("expected one row, got %#v", got.List)
	}
	item := got.List[0]
	if item.ID != 7 || item.RoleName != "运营" || item.SexShow != "男" || item.AddressID != 3 {
		t.Fatalf("list item base mismatch: %#v", item)
	}
	if item.AddressShow != "中国-江苏-南京-玄武区" || item.CreatedAt != "2026-05-04 10:11:12" {
		t.Fatalf("formatted fields mismatch: %#v", item)
	}
}

func TestServiceUpdateUserProfileAndInvalidatesRoleCacheWhenRoleChanges(t *testing.T) {
	cache := &fakeButtonCache{}
	repo := &fakeUserRepository{
		user:      &User{ID: 9, Username: "old", RoleID: 1},
		rolesByID: map[int64]*Role{2: {ID: 2, Name: "运营"}},
	}
	svc := NewService(repo, &fakePermissionBuilder{}, cache, time.Minute)

	appErr := svc.Update(context.Background(), 9, UpdateInput{
		Username:      " new name ",
		Avatar:        "avatar.png",
		RoleID:        2,
		Sex:           enum.SexFemale,
		AddressID:     3,
		DetailAddress: "玄武区",
		Bio:           "bio",
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !repo.txCalled || repo.updatedUserID != 9 || repo.updatedUserFields["username"] != "new name" || repo.updatedUserFields["role_id"] != int64(2) {
		t.Fatalf("user update mismatch: repo=%#v fields=%#v", repo, repo.updatedUserFields)
	}
	if repo.updatedProfileUserID != 9 || repo.updatedProfileFields["address_id"] != int64(3) || repo.updatedProfileFields["sex"] != enum.SexFemale {
		t.Fatalf("profile update mismatch: %#v", repo.updatedProfileFields)
	}
	if cache.key != "auth_perm_uid_9_app_rbac_page_grants" {
		t.Fatalf("expected cache invalidation to visit sorted admin/app keys, last key=%q", cache.key)
	}
}

func TestServiceUpdateRejectsMissingRole(t *testing.T) {
	svc := NewService(&fakeUserRepository{user: &User{ID: 9, RoleID: 1}}, &fakePermissionBuilder{}, nil, time.Minute)

	appErr := svc.Update(context.Background(), 9, UpdateInput{Username: "new", RoleID: 404, Sex: enum.SexUnknown, AddressID: 1})

	if appErr == nil || appErr.Code != 404 {
		t.Fatalf("expected missing role not found, got %#v", appErr)
	}
}

func TestServiceChangeStatusAndDeleteUseNormalizedIDs(t *testing.T) {
	repo := &fakeUserRepository{user: &User{ID: 3, RoleID: 1}}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute)

	if appErr := svc.ChangeStatus(context.Background(), 3, enum.CommonNo); appErr != nil {
		t.Fatalf("expected status change to pass, got %v", appErr)
	}
	if repo.statusUserID != 3 || repo.statusValue != enum.CommonNo {
		t.Fatalf("status update mismatch: %#v", repo)
	}
	if appErr := svc.Delete(context.Background(), []int64{0, 4, 4, 5}); appErr != nil {
		t.Fatalf("expected delete to pass, got %v", appErr)
	}
	if !reflect.DeepEqual(repo.deletedIDs, []int64{4, 5}) {
		t.Fatalf("delete ids not normalized: %#v", repo.deletedIDs)
	}
}

func TestServiceBatchUpdateProfileUsesExplicitAddressIDContract(t *testing.T) {
	repo := &fakeUserRepository{}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute)

	appErr := svc.BatchUpdateProfile(context.Background(), BatchProfileUpdate{
		IDs:       []int64{3, 3, 4},
		Field:     BatchProfileFieldAddressID,
		AddressID: 8,
	})

	if appErr != nil {
		t.Fatalf("expected batch update to pass, got %v", appErr)
	}
	if repo.batchUpdate.Field != BatchProfileFieldAddressID || !reflect.DeepEqual(repo.batchUpdate.IDs, []int64{3, 4}) || repo.batchUpdate.AddressID != 8 {
		t.Fatalf("batch update mismatch: %#v", repo.batchUpdate)
	}
}
