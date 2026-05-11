package user

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/exporttask"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/platform/taskqueue"
)

type fakeUserRepository struct {
	user                 *User
	profile              *Profile
	role                 *Role
	roleOptions          []Role
	addresses            []Address
	addressCalls         int
	listRows             []ListRow
	listTotal            int64
	exportRows           []ExportUserRow
	exportIDs            []int64
	entries              []QuickEntry
	rolesByID            map[int64]*Role
	emailUsed            bool
	phoneUsed            bool
	existsEmailUserID    int64
	existsEmail          string
	existsPhoneUserID    int64
	existsPhone          string
	listQuery            ListQuery
	txCalled             bool
	updatedUserID        int64
	updatedUserFields    map[string]any
	updatedProfileUserID int64
	updatedProfileFields map[string]any
	ensuredProfile       *Profile
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
	f.addressCalls++
	return f.addresses, f.err
}

func (f *fakeUserRepository) List(ctx context.Context, query ListQuery) ([]ListRow, int64, error) {
	f.listQuery = query
	return f.listRows, f.listTotal, f.err
}

func (f *fakeUserRepository) ExportUsersByIDs(ctx context.Context, ids []int64) ([]ExportUserRow, error) {
	f.exportIDs = append([]int64{}, ids...)
	return f.exportRows, f.err
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

func (f *fakeUserRepository) ExistsEmailForOtherUser(ctx context.Context, userID int64, email string) (bool, error) {
	f.existsEmailUserID = userID
	f.existsEmail = email
	return f.emailUsed, f.err
}

func (f *fakeUserRepository) ExistsPhoneForOtherUser(ctx context.Context, userID int64, phone string) (bool, error) {
	f.existsPhoneUserID = userID
	f.existsPhone = phone
	return f.phoneUsed, f.err
}

func (f *fakeUserRepository) UpdateProfile(ctx context.Context, userID int64, fields map[string]any) error {
	f.updatedProfileUserID = userID
	f.updatedProfileFields = fields
	return f.err
}

func (f *fakeUserRepository) EnsureProfile(ctx context.Context, profile Profile) error {
	f.ensuredProfile = &profile
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

type fakeAddressDictCache struct {
	snapshot    AddressDictSnapshot
	hit         bool
	getErr      error
	setErr      error
	deleteErr   error
	getCalls    int
	setCalls    int
	deleteCalls int
	saved       AddressDictSnapshot
}

func (f *fakeAddressDictCache) Get(ctx context.Context) (AddressDictSnapshot, bool, error) {
	f.getCalls++
	return f.snapshot, f.hit, f.getErr
}

func (f *fakeAddressDictCache) Set(ctx context.Context, snapshot AddressDictSnapshot) error {
	f.setCalls++
	f.saved = snapshot
	return f.setErr
}

func (f *fakeAddressDictCache) Delete(ctx context.Context) error {
	f.deleteCalls++
	return f.deleteErr
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

type fakeVerifyCodeStore struct {
	values     map[string]string
	deletedKey string
	err        error
}

func (f *fakeVerifyCodeStore) Get(ctx context.Context, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

func (f *fakeVerifyCodeStore) Delete(ctx context.Context, key string) error {
	f.deletedKey = key
	return f.err
}

func mustUserPasswordHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := hashUserPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return hash
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

func TestServiceProfileReturnsDetailDictAndSelfFlag(t *testing.T) {
	password := "$2y$10$hash"
	birthday := time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
	svc := NewService(&fakeUserRepository{
		user: &User{ID: 8, Username: "alice", Email: "alice@example.com", Phone: "15600000000", RoleID: 2, Password: &password},
		profile: &Profile{
			UserID:        8,
			Avatar:        "avatar.png",
			Sex:           enum.SexFemale,
			Birthday:      &birthday,
			AddressID:     3,
			DetailAddress: "玄武区",
			Bio:           "bio",
		},
		role: &Role{ID: 2, Name: "运营"},
		addresses: []Address{
			{ID: 1, ParentID: 0, Name: "中国"},
			{ID: 3, ParentID: 1, Name: "南京"},
		},
	}, &fakePermissionBuilder{}, nil, time.Minute)

	got, appErr := svc.Profile(context.Background(), 8, 8)

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if got.Profile.UserID != 8 || got.Profile.Username != "alice" || got.Profile.Avatar != "avatar.png" {
		t.Fatalf("profile base mismatch: %#v", got.Profile)
	}
	if got.Profile.RoleName != "运营" || got.Profile.Birthday != "2000-01-02" || !got.Profile.HasPassword || got.Profile.IsSelf != enum.CommonYes {
		t.Fatalf("profile derived fields mismatch: %#v", got.Profile)
	}
	if len(got.Dict.SexArr) != 3 || len(got.Dict.VerifyTypeArr) != 2 || got.Dict.VerifyTypeArr[0].Value != "password" || got.Dict.VerifyTypeArr[1].Value != "code" {
		t.Fatalf("profile dict mismatch: %#v", got.Dict)
	}
	if len(got.Dict.AuthAddressTree) != 1 || got.Dict.AuthAddressTree[0].Value != 1 {
		t.Fatalf("address tree mismatch: %#v", got.Dict.AuthAddressTree)
	}
}

func TestServiceUpdatePasswordWithOldPasswordWritesBcryptHash(t *testing.T) {
	hash := mustUserPasswordHash(t, "old-secret")
	repo := &fakeUserRepository{user: &User{ID: 9, Password: &hash}}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute)

	appErr := svc.UpdatePassword(context.Background(), UpdatePasswordInput{
		UserID:          9,
		VerifyType:      enum.VerifyTypePassword,
		OldPassword:     " old-secret ",
		NewPassword:     "new-secret",
		ConfirmPassword: "new-secret",
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	gotHash, ok := repo.updatedUserFields["password"].(string)
	if !ok || gotHash == "" || gotHash == hash {
		t.Fatalf("expected new password hash, got fields=%#v", repo.updatedUserFields)
	}
	if gotHash[:4] != "$2y$" {
		t.Fatalf("expected stored hash to use $2y$ prefix, got %q", gotHash[:4])
	}
	if !verifyUserPassword("new-secret", gotHash) {
		t.Fatalf("new password hash does not verify")
	}
}

func TestServiceUpdatePasswordWithCodeConsumesOwnedAccountCode(t *testing.T) {
	store := &fakeVerifyCodeStore{values: map[string]string{
		"auth:verify_code:email:change_password:c160f8cc69a4f0bf2b0362752353d060": "123456",
	}}
	repo := &fakeUserRepository{user: &User{ID: 9, Email: "alice@example.com"}}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithVerifyCodeStore(store, "auth:verify_code:"))

	appErr := svc.UpdatePassword(context.Background(), UpdatePasswordInput{
		UserID:          9,
		VerifyType:      enum.VerifyTypeCode,
		Account:         " alice@example.com ",
		Code:            "123456",
		NewPassword:     "new-secret",
		ConfirmPassword: "new-secret",
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if store.deletedKey == "" {
		t.Fatalf("expected verification code to be consumed")
	}
	if _, ok := repo.updatedUserFields["password"].(string); !ok {
		t.Fatalf("expected password update, got %#v", repo.updatedUserFields)
	}
}

func TestServiceUpdateEmailConsumesBindEmailCodeAndRejectsDuplicate(t *testing.T) {
	store := &fakeVerifyCodeStore{values: map[string]string{
		"auth:verify_code:email:bind_email:b681d72feaf8bf6a93d9a8ab86679ec3": "123456",
	}}
	repo := &fakeUserRepository{user: &User{ID: 9}}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithVerifyCodeStore(store, "auth:verify_code:"))

	appErr := svc.UpdateEmail(context.Background(), UpdateEmailInput{UserID: 9, Email: " new@example.com ", Code: "123456"})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if repo.existsEmailUserID != 9 || repo.existsEmail != "new@example.com" {
		t.Fatalf("duplicate email check mismatch: %#v", repo)
	}
	if repo.updatedUserFields["email"] != "new@example.com" || store.deletedKey == "" {
		t.Fatalf("email update/code consume mismatch: fields=%#v deleted=%q", repo.updatedUserFields, store.deletedKey)
	}

	dupRepo := &fakeUserRepository{user: &User{ID: 9}, emailUsed: true}
	dupSvc := NewService(dupRepo, &fakePermissionBuilder{}, nil, time.Minute, WithVerifyCodeStore(store, "auth:verify_code:"))
	appErr = dupSvc.UpdateEmail(context.Background(), UpdateEmailInput{UserID: 9, Email: "used@example.com", Code: "123456"})
	if appErr == nil || appErr.Message != "邮箱已被绑定" {
		t.Fatalf("expected duplicate email error, got %#v", appErr)
	}
}

func TestServiceUpdatePhoneConsumesBindPhoneCodeAndRejectsDuplicate(t *testing.T) {
	store := &fakeVerifyCodeStore{values: map[string]string{
		"auth:verify_code:phone:bind_phone:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	repo := &fakeUserRepository{user: &User{ID: 9}}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithVerifyCodeStore(store, "auth:verify_code:"))

	appErr := svc.UpdatePhone(context.Background(), UpdatePhoneInput{UserID: 9, Phone: " 15671628271 ", Code: "123456"})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if repo.existsPhoneUserID != 9 || repo.existsPhone != "15671628271" {
		t.Fatalf("duplicate phone check mismatch: %#v", repo)
	}
	if repo.updatedUserFields["phone"] != "15671628271" || store.deletedKey == "" {
		t.Fatalf("phone update/code consume mismatch: fields=%#v deleted=%q", repo.updatedUserFields, store.deletedKey)
	}

	dupRepo := &fakeUserRepository{user: &User{ID: 9}, phoneUsed: true}
	dupSvc := NewService(dupRepo, &fakePermissionBuilder{}, nil, time.Minute, WithVerifyCodeStore(store, "auth:verify_code:"))
	appErr = dupSvc.UpdatePhone(context.Background(), UpdatePhoneInput{UserID: 9, Phone: "15671628271", Code: "123456"})
	if appErr == nil || appErr.Message != "手机号已被绑定" {
		t.Fatalf("expected duplicate phone error, got %#v", appErr)
	}
}

func TestServiceProfileReturnsDefaultsWhenProfileMissing(t *testing.T) {
	repo := &fakeUserRepository{
		user: &User{ID: 9, Username: "bob", RoleID: 2},
		role: &Role{ID: 2, Name: "运营"},
	}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute)

	got, appErr := svc.Profile(context.Background(), 9, 8)

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if repo.ensuredProfile != nil {
		t.Fatalf("read path must not create missing profile: %#v", repo.ensuredProfile)
	}
	if got.Profile.IsSelf != enum.CommonNo || got.Profile.AddressID != 0 || got.Profile.Birthday != "" {
		t.Fatalf("profile defaults mismatch: %#v", got.Profile)
	}
}

func TestServiceUpdateProfileUpdatesOnlyCurrentUserSafeFields(t *testing.T) {
	birthday := "2026-05-05"
	repo := &fakeUserRepository{
		user:    &User{ID: 9, Username: "old", RoleID: 1},
		profile: &Profile{UserID: 9},
	}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute)

	appErr := svc.UpdateProfile(context.Background(), UpdateProfileInput{
		UserID:        9,
		Username:      " new name ",
		Avatar:        " avatar.png ",
		Sex:           enum.SexMale,
		Birthday:      &birthday,
		AddressID:     0,
		DetailAddress: " 玄武区 ",
		Bio:           " bio ",
	})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if !repo.txCalled || repo.updatedUserID != 9 || repo.updatedUserFields["username"] != "new name" {
		t.Fatalf("user update mismatch: %#v fields=%#v", repo, repo.updatedUserFields)
	}
	if repo.updatedProfileUserID != 9 {
		t.Fatalf("profile user id mismatch: %d", repo.updatedProfileUserID)
	}
	if repo.updatedProfileFields["avatar"] != "avatar.png" || repo.updatedProfileFields["sex"] != enum.SexMale || repo.updatedProfileFields["address_id"] != int64(0) {
		t.Fatalf("profile update fields mismatch: %#v", repo.updatedProfileFields)
	}
	if got, ok := repo.updatedProfileFields["birthday"].(*time.Time); !ok || got == nil || got.Format("2006-01-02") != "2026-05-05" {
		t.Fatalf("birthday was not parsed as date pointer: %#v", repo.updatedProfileFields["birthday"])
	}
}

func TestServiceUpdateProfileRejectsInvalidBirthday(t *testing.T) {
	birthday := "2026/05/05"
	svc := NewService(&fakeUserRepository{user: &User{ID: 9}}, &fakePermissionBuilder{}, nil, time.Minute)

	appErr := svc.UpdateProfile(context.Background(), UpdateProfileInput{
		UserID:   9,
		Username: "alice",
		Sex:      enum.SexUnknown,
		Birthday: &birthday,
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "生日格式错误" {
		t.Fatalf("expected invalid birthday error, got %#v", appErr)
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

type fakeExportTaskCreator struct {
	createdInput CreateExportTaskInput
	createdID    int64
	failedID     int64
	failedMsg    string
	err          error
}

func (f *fakeExportTaskCreator) CreatePending(ctx context.Context, input exporttask.CreatePendingInput) (int64, error) {
	f.createdInput = CreateExportTaskInput{UserID: input.UserID, Title: input.Title}
	if f.createdID == 0 {
		f.createdID = 77
	}
	return f.createdID, f.err
}

func (f *fakeExportTaskCreator) MarkFailed(ctx context.Context, id int64, message string) error {
	f.failedID = id
	f.failedMsg = message
	return nil
}

type CreateExportTaskInput struct {
	UserID int64
	Title  string
}

type fakeExportEnqueuer struct {
	tasks []taskqueue.Task
	err   error
}

func (f *fakeExportEnqueuer) Enqueue(ctx context.Context, task taskqueue.Task) (taskqueue.EnqueueResult, error) {
	f.tasks = append(f.tasks, task)
	if f.err != nil {
		return taskqueue.EnqueueResult{}, f.err
	}
	return taskqueue.EnqueueResult{ID: "export-job", Queue: task.Queue, Type: task.Type}, nil
}

func TestServiceExportRejectsEmptyIDs(t *testing.T) {
	_, appErr := NewService(&fakeUserRepository{}, nil, nil, time.Minute).Export(context.Background(), ExportInput{UserID: 9})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected bad request for empty ids, got %#v", appErr)
	}
}

func TestServiceExportNormalizesCreatesPendingAndEnqueuesLowTask(t *testing.T) {
	repo := &fakeUserRepository{exportRows: []ExportUserRow{{ID: 2, Username: "u2"}, {ID: 3, Username: "u3"}}}
	creator := &fakeExportTaskCreator{createdID: 88}
	enqueuer := &fakeExportEnqueuer{}
	got, appErr := NewService(repo, nil, nil, time.Minute, WithExportTaskCreator(creator), WithExportEnqueuer(enqueuer)).Export(context.Background(), ExportInput{UserID: 9, Platform: enum.PlatformAdmin, IDs: []int64{3, 2, 3, 0}})
	if appErr != nil {
		t.Fatalf("Export returned error: %v", appErr)
	}
	if got.ID != 88 || got.Message != "导出任务已提交，完成后将通知您" {
		t.Fatalf("unexpected export response: %#v", got)
	}
	if !reflect.DeepEqual(repo.exportIDs, []int64{2, 3}) {
		t.Fatalf("expected normalized ids, got %#v", repo.exportIDs)
	}
	if creator.createdInput.UserID != 9 || creator.createdInput.Title != "用户列表导出" {
		t.Fatalf("unexpected created task input: %#v", creator.createdInput)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].Type != exporttask.TypeRunV1 || enqueuer.tasks[0].Queue != taskqueue.QueueLow {
		t.Fatalf("unexpected enqueued task: %#v", enqueuer.tasks)
	}
	payload, err := exporttask.DecodeRunPayload(enqueuer.tasks[0].Payload)
	if err != nil {
		t.Fatalf("decode enqueued payload: %v", err)
	}
	if payload.TaskID != 88 || payload.Kind != exporttask.KindUserList || payload.UserID != 9 || !reflect.DeepEqual(payload.IDs, []int64{2, 3}) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestServiceExportMarksTaskFailedWhenEnqueueFails(t *testing.T) {
	repo := &fakeUserRepository{exportRows: []ExportUserRow{{ID: 2, Username: "u2"}}}
	creator := &fakeExportTaskCreator{createdID: 88}
	enqueuer := &fakeExportEnqueuer{err: errors.New("redis down")}
	_, appErr := NewService(repo, nil, nil, time.Minute, WithExportTaskCreator(creator), WithExportEnqueuer(enqueuer)).Export(context.Background(), ExportInput{UserID: 9, Platform: enum.PlatformAdmin, IDs: []int64{2}})
	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
	if creator.failedID != 88 || creator.failedMsg == "" {
		t.Fatalf("expected created task to be marked failed, got id=%d msg=%q", creator.failedID, creator.failedMsg)
	}
}

func TestServiceExportReturnsNotFoundWhenNoSelectedUsersExist(t *testing.T) {
	repo := &fakeUserRepository{}
	creator := &fakeExportTaskCreator{}
	enqueuer := &fakeExportEnqueuer{}
	_, appErr := NewService(repo, nil, nil, time.Minute, WithExportTaskCreator(creator), WithExportEnqueuer(enqueuer)).Export(context.Background(), ExportInput{UserID: 9, IDs: []int64{99}})
	if appErr == nil || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestExportDataProviderBuildsUserListFileData(t *testing.T) {
	repo := &fakeUserRepository{exportRows: []ExportUserRow{{ID: 2, Username: "alice", Email: "a@example.com", Phone: "15671628271", Avatar: "avatar.png", Sex: 2, RoleName: "管理员"}}}
	data, err := NewExportDataProvider(repo).BuildExportData(context.Background(), exporttask.KindUserList, []int64{2})
	if err != nil {
		t.Fatalf("BuildExportData returned error: %v", err)
	}
	if data.Prefix != "用户列表导出" || len(data.Headers) != 7 || data.Headers[0].Key != "id" || data.Headers[6].Key != "role" {
		t.Fatalf("unexpected file metadata: %#v", data)
	}
	if len(data.Rows) != 1 || data.Rows[0]["id"] != "2" || data.Rows[0]["sex"] != "女" || data.Rows[0]["role"] != "管理员" {
		t.Fatalf("unexpected export rows: %#v", data.Rows)
	}
}

func TestServiceLoadAddressDictUsesCacheHit(t *testing.T) {
	repo := &fakeUserRepository{
		addresses: []Address{{ID: 99, ParentID: 0, Name: "不应该查询"}},
	}
	cache := &fakeAddressDictCache{
		hit: true,
		snapshot: AddressDictSnapshot{
			Version:  addressDictSnapshotVersion,
			RowCount: 1,
			Tree: []AddressTreeNode{{
				ID:    1,
				Label: "中国",
				Value: 1,
			}},
			PathByID: map[int64][]string{1: []string{"中国"}},
		},
	}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithAddressDictCache(cache))

	got, err := svc.loadAddressDict(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got.Tree) != 1 || got.Tree[0].Label != "中国" {
		t.Fatalf("unexpected cached tree: %#v", got.Tree)
	}
	if repo.addressCalls != 0 {
		t.Fatalf("expected cache hit to avoid DB, got %d address calls", repo.addressCalls)
	}
	if cache.getCalls != 1 || cache.setCalls != 0 {
		t.Fatalf("cache calls mismatch: get=%d set=%d", cache.getCalls, cache.setCalls)
	}
}

func TestServiceLoadAddressDictMissRebuildsAndSavesCache(t *testing.T) {
	repo := &fakeUserRepository{
		addresses: []Address{
			{ID: 1, ParentID: 0, Name: "中国", UpdatedAt: time.Date(2026, 3, 9, 10, 56, 1, 0, time.Local)},
			{ID: 2, ParentID: 1, Name: "江苏", UpdatedAt: time.Date(2026, 3, 9, 10, 56, 1, 0, time.Local)},
		},
	}
	cache := &fakeAddressDictCache{}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithAddressDictCache(cache))

	got, err := svc.loadAddressDict(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.addressCalls != 1 {
		t.Fatalf("expected one DB address query, got %d", repo.addressCalls)
	}
	if cache.setCalls != 1 {
		t.Fatalf("expected cache Set once, got %d", cache.setCalls)
	}
	if got.RowCount != 2 || got.SourceMaxUpdated != "2026-03-09 10:56:01" {
		t.Fatalf("snapshot metadata mismatch: %#v", got)
	}
	if path := got.PathByID[2]; len(path) != 2 || path[0] != "中国" || path[1] != "江苏" {
		t.Fatalf("path_by_id mismatch: %#v", got.PathByID)
	}
	if len(cache.saved.Tree) != 1 || cache.saved.Tree[0].Children[0].Label != "江苏" {
		t.Fatalf("saved tree mismatch: %#v", cache.saved.Tree)
	}
}

func TestServiceLoadAddressDictRedisErrorFallsBackToDatabase(t *testing.T) {
	repo := &fakeUserRepository{
		addresses: []Address{{ID: 1, ParentID: 0, Name: "中国"}},
	}
	cache := &fakeAddressDictCache{getErr: errors.New("redis down")}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithAddressDictCache(cache))

	got, err := svc.loadAddressDict(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.addressCalls != 1 {
		t.Fatalf("expected fallback DB query, got %d", repo.addressCalls)
	}
	if cache.deleteCalls != 0 {
		t.Fatalf("expected redis connection error not to delete cache, got %d deletes", cache.deleteCalls)
	}
	if len(got.Tree) != 1 || got.Tree[0].Label != "中国" {
		t.Fatalf("fallback tree mismatch: %#v", got.Tree)
	}
}

func TestServiceLoadAddressDictCorruptCacheDeletesAndFallsBack(t *testing.T) {
	repo := &fakeUserRepository{
		addresses: []Address{{ID: 1, ParentID: 0, Name: "中国"}},
	}
	cache := &fakeAddressDictCache{getErr: fmt.Errorf("decode address dict: %w", ErrAddressDictCacheCorrupt)}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithAddressDictCache(cache))

	got, err := svc.loadAddressDict(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.addressCalls != 1 {
		t.Fatalf("expected fallback DB query, got %d", repo.addressCalls)
	}
	if cache.deleteCalls != 1 {
		t.Fatalf("expected corrupt cache delete once, got %d", cache.deleteCalls)
	}
	if len(got.Tree) != 1 || got.Tree[0].Value != 1 {
		t.Fatalf("fallback tree mismatch: %#v", got.Tree)
	}
}

func TestServiceLoadAddressDictDatabaseErrorReturnsErrorAndDoesNotSetCache(t *testing.T) {
	repo := &fakeUserRepository{
		err: errors.New("database down"),
	}
	cache := &fakeAddressDictCache{}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithAddressDictCache(cache))

	got, err := svc.loadAddressDict(context.Background())

	if err == nil {
		t.Fatalf("expected database error")
	}
	if got != nil {
		t.Fatalf("expected nil snapshot on database error, got %#v", got)
	}
	if repo.addressCalls != 1 {
		t.Fatalf("expected one DB address query, got %d", repo.addressCalls)
	}
	if cache.setCalls != 0 {
		t.Fatalf("expected cache Set not to run on database error, got %d", cache.setCalls)
	}
}

func TestServiceLoadAddressDictSetErrorStillReturnsDatabaseResult(t *testing.T) {
	repo := &fakeUserRepository{
		addresses: []Address{{ID: 1, ParentID: 0, Name: "中国"}},
	}
	cache := &fakeAddressDictCache{setErr: errors.New("set failed")}
	svc := NewService(repo, &fakePermissionBuilder{}, nil, time.Minute, WithAddressDictCache(cache))

	got, err := svc.loadAddressDict(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.addressCalls != 1 || cache.setCalls != 1 {
		t.Fatalf("calls mismatch: address=%d set=%d", repo.addressCalls, cache.setCalls)
	}
	if len(got.Tree) != 1 || got.Tree[0].Label != "中国" {
		t.Fatalf("tree mismatch: %#v", got.Tree)
	}
}
